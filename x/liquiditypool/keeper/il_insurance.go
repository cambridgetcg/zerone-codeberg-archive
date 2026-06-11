package keeper

import (
	"math"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// ────────────────────────────────────────────────────────────────────────────
// IL Insurance Fund — KV state helpers
// ────────────────────────────────────────────────────────────────────────────

// GetILInsuranceFundBalance reads the current fund balance from the KV store.
// Returns big.Int(0) when the key is absent (fund not yet seeded).
func (k Keeper) GetILInsuranceFundBalance(ctx sdk.Context) *big.Int {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.ILInsuranceFundKey)
	if err != nil || len(bz) == 0 {
		return new(big.Int)
	}
	n := new(big.Int)
	n.SetString(string(bz), 10)
	return n
}

// SetILInsuranceFundBalance writes the fund balance to the KV store.
// amount must be ≥ 0.
func (k Keeper) SetILInsuranceFundBalance(ctx sdk.Context, amount *big.Int) {
	kvStore := k.storeService.OpenKVStore(ctx)
	if err := kvStore.Set(types.ILInsuranceFundKey, []byte(amount.String())); err != nil {
		panic("SetILInsuranceFundBalance: " + err.Error())
	}
}

// ────────────────────────────────────────────────────────────────────────────
// IL computation — CPAMM mathematics
// ────────────────────────────────────────────────────────────────────────────

// ComputeILBps calculates impermanent loss for a CPAMM LP in 1M-BPS scale.
//
// IL_factor = 2*sqrt(r)/(1+r) - 1   (≤ 0 for r ≠ 1)
// Returns |IL_factor| × 1_000_000 as uint64.
//
// depositPrice and currentPrice are both in 1e12-scaled bigint units
// (consistent with LpMinerPosition.deposit_price_a_per_b and TWAP accumulator).
// Returns 0 if either price is zero (no IL computable).
func ComputeILBps(depositPrice, currentPrice *big.Int) uint64 {
	if depositPrice == nil || currentPrice == nil {
		return 0
	}
	if depositPrice.Sign() <= 0 || currentPrice.Sign() <= 0 {
		return 0
	}

	// r = currentPrice / depositPrice  (float64 is fine — we only need ~6 sig-fig precision)
	dp := new(big.Float).SetInt(depositPrice)
	cp := new(big.Float).SetInt(currentPrice)
	rFl, _ := new(big.Float).Quo(cp, dp).Float64()
	if rFl <= 0 {
		return 0
	}

	// IL_factor = 2*sqrt(r)/(1+r) - 1
	ilFactor := 2*math.Sqrt(rFl)/(1+rFl) - 1
	// ilFactor is ≤ 0; we want the magnitude
	if ilFactor >= 0 {
		return 0 // no IL (r == 1)
	}
	ilBps := uint64((-ilFactor) * 1_000_000)
	if ilBps > 1_000_000 {
		ilBps = 1_000_000 // cap at 100%
	}
	return ilBps
}

// ────────────────────────────────────────────────────────────────────────────
// IL payout — called from RemoveLiquidity
// ────────────────────────────────────────────────────────────────────────────

// MaybePayILInsurance computes and distributes IL compensation to senderAddr.
//
// Prerequisites (checked internally, fail-silent when not met):
//   - Params.IlInsuranceThresholdBps > 0 (insurance enabled)
//   - LpMinerPosition.DepositPriceAPerB is set and the pool has uzrn on one side
//   - Computed IL ≥ threshold
//   - Fund balance > 0
//
// The payout is min(IL × maxCoverage, fundBalance), transferred from the
// module account to senderAddr.
//
// pool is the pool being withdrawn from; senderAddr the withdrawing LP.
// amountA / amountB are the raw amounts being returned (before this call modifies state).
//
// Returns the payout amount in uzrn (0 if nothing was paid).
func (k Keeper) MaybePayILInsurance(
	ctx sdk.Context,
	pool *types.Pool,
	senderAddr sdk.AccAddress,
	amountA, amountB *big.Int,
) *big.Int {
	zeroPayout := new(big.Int)

	params := k.GetParams(ctx)
	if params.IlInsuranceThresholdBps == 0 {
		return zeroPayout // disabled
	}

	// Only meaningful for uzrn-containing pools
	uzrn := "uzrn"
	var amountUZRN *big.Int
	switch {
	case pool.DenomA == uzrn:
		amountUZRN = new(big.Int).Set(amountA)
	case pool.DenomB == uzrn:
		amountUZRN = new(big.Int).Set(amountB)
	default:
		return zeroPayout // non-uzrn pool: IL insurance not applicable
	}

	if amountUZRN.Sign() <= 0 {
		return zeroPayout
	}

	// Load deposit price from LP miner position
	pos := k.GetLpMinerPosition(ctx, pool.PoolId, senderAddr.String())
	if pos == nil || pos.DepositPriceAPerB == "" {
		return zeroPayout // no deposit price recorded (pre-IL-insurance LP or genesis LP)
	}
	depositPrice := new(big.Int)
	if _, ok := depositPrice.SetString(pos.DepositPriceAPerB, 10); !ok || depositPrice.Sign() <= 0 {
		return zeroPayout
	}

	// Current price: reserveB * 1e12 / reserveA
	resA := new(big.Int)
	resB := new(big.Int)
	resA.SetString(pool.ReserveA, 10)
	resB.SetString(pool.ReserveB, 10)
	if resA.Sign() <= 0 {
		return zeroPayout
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil)
	currentPrice := new(big.Int).Mul(resB, scale)
	currentPrice.Div(currentPrice, resA)
	if currentPrice.Sign() <= 0 {
		return zeroPayout
	}

	// IL in BPS
	ilBps := ComputeILBps(depositPrice, currentPrice)
	if ilBps < params.IlInsuranceThresholdBps {
		return zeroPayout // below threshold
	}

	// IL payout = amountUZRN × 2 × ilBps / 1_000_000  × maxCoverage / 1_000_000
	// (amountUZRN is one side; total withdrawal value ≈ 2×amountUZRN in balanced pool)
	totalValueUZRN := new(big.Int).Mul(amountUZRN, big.NewInt(2))
	maxCov := params.IlInsuranceMaxCoverageBps
	if maxCov == 0 {
		maxCov = 1_000_000 // default: 100%
	}

	// payout = totalValueUZRN × ilBps × maxCov / (1_000_000 × 1_000_000)
	payout := new(big.Int).Mul(totalValueUZRN, big.NewInt(int64(ilBps)))
	payout.Mul(payout, big.NewInt(int64(maxCov)))
	payout.Div(payout, new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1_000_000)))

	if payout.Sign() <= 0 {
		return zeroPayout
	}

	// Cap at fund balance
	fundBalance := k.GetILInsuranceFundBalance(ctx)
	if fundBalance.Sign() <= 0 {
		return zeroPayout // empty fund
	}
	if payout.Cmp(fundBalance) > 0 {
		payout = new(big.Int).Set(fundBalance)
	}

	// Transfer payout from module account to sender
	payoutCoins := sdk.NewCoins(sdk.NewCoin(uzrn, sdkmath.NewIntFromBigInt(payout)))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, payoutCoins); err != nil {
		// Fail silently — don't abort the withdrawal over insurance
		return zeroPayout
	}

	// Deduct from fund balance
	newBalance := new(big.Int).Sub(fundBalance, payout)
	k.SetILInsuranceFundBalance(ctx, newBalance)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.il_insurance_payout",
		sdk.NewAttribute("pool_id", pool.PoolId),
		sdk.NewAttribute("recipient", senderAddr.String()),
		sdk.NewAttribute("payout_uzrn", payout.String()),
		sdk.NewAttribute("il_bps", sdkmath.NewIntFromUint64(uint64(ilBps)).String()),
		sdk.NewAttribute("fund_balance_after", newBalance.String()),
	))

	return payout
}

// ILInsuranceFundBalanceForTest exports fund balance for white-box unit tests.
func ILInsuranceFundBalanceForTest(k Keeper, ctx sdk.Context) *big.Int {
	return k.GetILInsuranceFundBalance(ctx)
}
