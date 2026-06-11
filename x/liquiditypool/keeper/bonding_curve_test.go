package keeper_test

// bonding_curve_test.go — Liquidity Gap 4: Bonding Curve (Single-Asset Pools).
//
// Tests cover:
//
// Pure function unit tests (BondingCurveBuyOut / BondingCurveSellOut):
//  1. BuyBasic: buy from fresh pool  (supply=1000, reserve=1000 → 1:1 price)
//  2. BuyIncreasesReserveAndSupply: linear curve preserves price after buy
//  3. SellBasic: sell half back — receive proportional ZRN
//  4. CannotSellMoreThanSupply
//  5. UnsupportedCRR: CRR ≠ 1_000_000 → error
//  6. DepositTooSmall: output rounds to 0 → error
//  7. InitialMint: supply=0 → tokensOut = depositAmount (1:1 bootstrap)
//
// MsgServer integration tests (zero-fee env, min-liq = 1):
//  8. CreateBondingCurvePool_Success
//  9. CreateBondingCurvePool_WrongCRR
// 10. CreateBondingCurvePool_NonZeroAmountB
// 11. BuySwap: buy continuous tokens with denom_a
// 12. SellSwap: sell continuous tokens back for denom_a
// 13. ConstantProductUnaffected: POOL_TYPE_CONSTANT_PRODUCT pools unaffected

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// ── Pure function unit tests ──────────────────────────────────────────────────

func TestBondingCurve_BuyBasic(t *testing.T) {
	// reserve=1000, supply=1000, deposit=100 → tokensOut = 1000×100/1000 = 100
	out, err := keeper.BondingCurveBuyOut("1000", "1000", "100", 1_000_000)
	require.NoError(t, err)
	require.Equal(t, "100", out)
}

func TestBondingCurve_BuyIncreasesReserveAndSupply(t *testing.T) {
	// Linear bonding curve (CRR=100%): spot price = reserveA/supplyB = constant 1.
	// buy 500 from initial (reserve=1000, supply=1000) → out = 1000×500/1000 = 500
	out1, err := keeper.BondingCurveBuyOut("1000", "1000", "500", 1_000_000)
	require.NoError(t, err)
	require.Equal(t, "500", out1)

	// New state: reserve=1500, supply=1500 — spot price still 1.
	// buy another 100 → out = 1500×100/1500 = 100
	out2, err := keeper.BondingCurveBuyOut("1500", "1500", "100", 1_000_000)
	require.NoError(t, err)
	require.Equal(t, "100", out2)
}

func TestBondingCurve_SellBasic(t *testing.T) {
	// reserve=1000, supply=1000: sell 100 continuous → reserveOut = 1000×100/1000 = 100
	out, err := keeper.BondingCurveSellOut("1000", "1000", "100", 1_000_000)
	require.NoError(t, err)
	require.Equal(t, "100", out)
}

func TestBondingCurve_CannotSellMoreThanSupply(t *testing.T) {
	_, err := keeper.BondingCurveSellOut("1000", "1000", "1001", 1_000_000)
	require.Error(t, err, "should reject selling more than total supply")
}

func TestBondingCurve_UnsupportedCRR(t *testing.T) {
	_, err := keeper.BondingCurveBuyOut("1000", "1000", "100", 500_000)
	require.Error(t, err, "only CRR=1_000_000 supported in v1")

	_, err = keeper.BondingCurveSellOut("1000", "1000", "100", 500_000)
	require.Error(t, err, "only CRR=1_000_000 supported in v1")
}

func TestBondingCurve_DepositTooSmall(t *testing.T) {
	// supply=1, large reserve → supply×deposit/reserve = 1×1/1_000_000_000 = 0 (floor) → error
	_, err := keeper.BondingCurveBuyOut("1000000000", "1", "1", 1_000_000)
	require.Error(t, err, "deposit should be rejected when output rounds to 0")
}

func TestBondingCurve_InitialMintZeroSupply(t *testing.T) {
	// supply=0 → bootstrap: tokensOut = depositAmount (1:1, no division)
	out, err := keeper.BondingCurveBuyOut("1000", "0", "500", 1_000_000)
	require.NoError(t, err)
	require.Equal(t, "500", out, "with zero supply, deposit = initial token mint (1:1)")
}

// ── MsgServer integration tests ───────────────────────────────────────────────

// setupBCTest creates a test env with zero fees and relaxed min-liquidity — both
// appropriate for unit-level bonding curve tests that focus on the math, not policy.
func setupBCTest(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context, *mockBankKeeper) {
	t.Helper()
	ms, k, ctx, bk := setupMsgServer(t)
	params := k.GetParams(ctx)
	params.DefaultSwapFeeBps = 0   // no fee: test pure bonding curve math
	params.MinInitialLiquidity = "1" // allow small test amounts
	k.SetParams(ctx, params)
	return ms, k, ctx, bk
}

func TestBondingCurvePool_CreateSuccess(t *testing.T) {
	ms, k, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)

	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                 testAuthority,
		DenomA:                  "uzrn",
		DenomB:                  "continuous-token",
		AmountA:                 "1000000",
		AmountB:                 "0",
		SwapFeeBps:              0,
		PoolType:                types.PoolType_POOL_TYPE_BONDING_CURVE,
		ConstantReserveRatioBps: 1_000_000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.PoolId)

	pool, found := k.GetPool(ctx, resp.PoolId)
	require.True(t, found)
	require.Equal(t, types.PoolType_POOL_TYPE_BONDING_CURVE, pool.PoolType)
	require.Equal(t, uint64(1_000_000), pool.ConstantReserveRatioBps)
	require.Equal(t, "1000000", pool.ReserveA)
	require.Equal(t, "0", pool.ReserveB)
	require.Equal(t, "1000000", pool.LpTokenSupply, "initial continuous token supply = amountA (1:1)")
}

func TestBondingCurvePool_CreateWrongCRR(t *testing.T) {
	ms, _, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)

	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                 testAuthority,
		DenomA:                  "uzrn",
		DenomB:                  "tok",
		AmountA:                 "1000000",
		AmountB:                 "0",
		PoolType:                types.PoolType_POOL_TYPE_BONDING_CURVE,
		ConstantReserveRatioBps: 500_000, // 50% — not supported in v1
	})
	require.ErrorIs(t, err, types.ErrBondingCurveNoCRR)
}

func TestBondingCurvePool_CreateNonZeroAmountB(t *testing.T) {
	ms, _, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)

	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                 testAuthority,
		DenomA:                  "uzrn",
		DenomB:                  "tok",
		AmountA:                 "1000000",
		AmountB:                 "500000", // must be 0 for single-asset pool
		PoolType:                types.PoolType_POOL_TYPE_BONDING_CURVE,
		ConstantReserveRatioBps: 1_000_000,
	})
	require.ErrorIs(t, err, types.ErrBondingCurveRequiresOnlyA)
}

func TestBondingCurvePool_BuySwap(t *testing.T) {
	ms, k, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)

	// Create bonding curve pool: reserve=1000000, supply=1000000, price=1
	createResp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                 testAuthority,
		DenomA:                  "uzrn",
		DenomB:                  "ctoken",
		AmountA:                 "1000000",
		AmountB:                 "0",
		PoolType:                types.PoolType_POOL_TYPE_BONDING_CURVE,
		ConstantReserveRatioBps: 1_000_000,
	})
	require.NoError(t, err)
	poolId := createResp.PoolId

	// Buy 100000 uzrn worth of ctoken (zero fee env → 1:1 price → tokensOut = 100000).
	buyer := bcTestAddr("bc-buyer-xxxxxxxxxxx") // 20 chars exactly
	bk.setBalance(buyer.String(), "uzrn", 1_000_000_000)

	swapResp, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        buyer.String(),
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "100000",
	})
	require.NoError(t, err)
	// tokensOut = 1000000 × 100000 / 1000000 = 100000 (fee = 0)
	require.Equal(t, "100000", swapResp.TokenOutAmount)

	pool, found := k.GetPool(ctx, poolId)
	require.True(t, found)
	require.Equal(t, "1100000", pool.ReserveA, "reserve should increase by deposit")
	require.Equal(t, "1100000", pool.LpTokenSupply, "continuous supply should increase by minted tokens")
}

func TestBondingCurvePool_SellSwap(t *testing.T) {
	ms, k, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)

	createResp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                 testAuthority,
		DenomA:                  "uzrn",
		DenomB:                  "stoken",
		AmountA:                 "1000000",
		AmountB:                 "0",
		PoolType:                types.PoolType_POOL_TYPE_BONDING_CURVE,
		ConstantReserveRatioBps: 1_000_000,
	})
	require.NoError(t, err)
	poolId := createResp.PoolId

	// Sell 200000 stoken back for uzrn (zero fee → reserveOut = 200000).
	seller := bcTestAddr("bc-seller-xxxxxxxxxxx") // 20 chars exactly
	bk.setBalance(seller.String(), "stoken", 1_000_000_000)

	swapResp, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        seller.String(),
		PoolId:        poolId,
		TokenInDenom:  "stoken",
		TokenInAmount: "200000",
	})
	require.NoError(t, err)
	// reserveOut = 1000000 × 200000 / 1000000 = 200000 (fee = 0)
	require.Equal(t, "200000", swapResp.TokenOutAmount)

	pool, found := k.GetPool(ctx, poolId)
	require.True(t, found)
	require.Equal(t, "800000", pool.ReserveA, "reserve should decrease by returned ZRN")
	require.Equal(t, "800000", pool.LpTokenSupply, "continuous supply should decrease by burned tokens")
}

func TestBondingCurvePool_ConstantProductUnaffected(t *testing.T) {
	ms, k, ctx, bk := setupBCTest(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)
	bk.setBalance(testAuthority, "atom", 1_000_000_000)

	// Create a standard constant-product pool (small amounts OK with min-liq = 1).
	createResp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:    testAuthority,
		DenomA:     "uzrn",
		DenomB:     "atom",
		AmountA:    "1000000",
		AmountB:    "1000000",
		SwapFeeBps: 0,
		// PoolType defaults to POOL_TYPE_CONSTANT_PRODUCT
	})
	require.NoError(t, err)
	poolId := createResp.PoolId

	pool, found := k.GetPool(ctx, poolId)
	require.True(t, found)
	require.Equal(t, types.PoolType_POOL_TYPE_CONSTANT_PRODUCT, pool.PoolType,
		"default pool type must be CONSTANT_PRODUCT")

	// Verify a standard swap still works.
	swapper := bcTestAddr("cp-swapper-xxxxxxxxxx") // 20 chars exactly
	bk.setBalance(swapper.String(), "uzrn", 1_000_000_000)

	swapResp, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        swapper.String(),
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "100000",
	})
	require.NoError(t, err)
	require.NotEmpty(t, swapResp.TokenOutAmount)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// bcTestAddr creates a deterministic 20-byte sdk.AccAddress from a string seed.
// The seed is copied into a zero-padded 20-byte buffer — safe for any seed ≤ 20 chars.
func bcTestAddr(seed string) sdk.AccAddress {
	addr := make([]byte, 20)
	copy(addr, []byte(seed))
	return sdk.AccAddress(addr)
}
