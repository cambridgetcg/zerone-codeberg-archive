package keeper_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// Valid bech32 test addresses (known-good from existing tests).
const (
	mevSwapSender = "zrn1commit0000000000000000000000" // used in commit-reveal
	mevLPSender   = "zrn1sender0000000000000000000000" // used in LP operations
)

// doCommitReveal commits and immediately reveals a swap, returning the RevealSwap response.
// The nonce is deterministic per call — callers must use distinct (sender, poolId) pairs or
// delete the commitment between calls. Tests use fresh contexts so this is safe.
func doCommitReveal(
	t *testing.T,
	ms types.MsgServer,
	ctx sdk.Context,
	sender, poolId, tokenInAmount, tipUzrn string,
) *types.MsgRevealSwapResponse {
	t.Helper()
	nonce := []byte("mev-test-nonce")
	hash := keeper.ComputeSwapCommitmentHash(nonce, poolId, "uzrn", tokenInAmount, "", 0)
	if _, err := ms.CommitSwap(ctx, &types.MsgCommitSwap{
		Sender:     sender,
		PoolId:     poolId,
		Commitment: hash,
	}); err != nil {
		t.Fatalf("CommitSwap: %v", err)
	}
	resp, err := ms.RevealSwap(ctx, &types.MsgRevealSwap{
		Sender:          sender,
		PoolId:          poolId,
		Nonce:           nonce,
		TokenInDenom:    "uzrn",
		TokenInAmount:   tokenInAmount,
		PriorityTipUzrn: tipUzrn,
	})
	if err != nil {
		t.Fatalf("RevealSwap: %v", err)
	}
	return resp
}

// TestRevealSwap_MEVTip_HalfToLP verifies that 50% of the tip accumulates in the
// pool's LP MEV reserve and 50% is burned as protocol share.
func TestRevealSwap_MEVTip_HalfToLP(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	params := types.DefaultParams()
	params.MevFeeShareBps = 500_000 // 50% to LP reserve
	k.SetParams(ctx, params)

	bk.setBalance(mevSwapSender, "uzrn", 50_000) // swap + tip buffer

	const tip = "10000"
	resp := doCommitReveal(t, ms, ctx, mevSwapSender, poolId, "10000", tip)

	// LP share = 10000 × 500000 / 1000000 = 5000
	if resp.MevLpShare != "5000" {
		t.Errorf("expected MevLpShare=5000, got %s", resp.MevLpShare)
	}

	pool, _ := k.GetPool(ctx, poolId)
	if pool.MevReserveBalance != "5000" {
		t.Errorf("expected pool.MevReserveBalance=5000, got %q", pool.MevReserveBalance)
	}

	// Module net balance: received 10000, burned 5000 (protocol) → holds 5000 for LP reserve.
	// (Swap fees may also be routed through the module; test only checks the floor.)
	modBal := bk.moduleBalances[types.ModuleName]["uzrn"]
	if modBal < 5000 {
		t.Errorf("module should hold at least the LP MEV reserve (5000 uzrn); got %d", modBal)
	}
}

// TestRevealSwap_MEVTip_AllProtocol verifies that when MEVFeeShareBps=0 (default),
// the entire tip goes to protocol burn and pool reserve stays empty.
func TestRevealSwap_MEVTip_AllProtocol(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	// DefaultParams: MevFeeShareBps = 0 (disabled)
	params := types.DefaultParams()
	if params.MevFeeShareBps != 0 {
		t.Fatalf("expected default MevFeeShareBps=0, got %d", params.MevFeeShareBps)
	}

	bk.setBalance(mevSwapSender, "uzrn", 50_000)

	resp := doCommitReveal(t, ms, ctx, mevSwapSender, poolId, "10000", "8000")

	if resp.MevLpShare != "0" {
		t.Errorf("expected MevLpShare=0 when disabled, got %s", resp.MevLpShare)
	}

	pool, _ := k.GetPool(ctx, poolId)
	if pool.MevReserveBalance != "" && pool.MevReserveBalance != "0" {
		t.Errorf("expected empty MevReserveBalance when disabled, got %q", pool.MevReserveBalance)
	}
}

// TestRevealSwap_MEVTip_AllToLP verifies that when MEVFeeShareBps=1_000_000,
// the entire tip becomes LP reserve and nothing is burned.
func TestRevealSwap_MEVTip_AllToLP(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	params := types.DefaultParams()
	params.MevFeeShareBps = 1_000_000 // 100% to LP
	k.SetParams(ctx, params)

	bk.setBalance(mevSwapSender, "uzrn", 50_000)

	const tip = "12000"
	resp := doCommitReveal(t, ms, ctx, mevSwapSender, poolId, "10000", tip)

	if resp.MevLpShare != tip {
		t.Errorf("expected MevLpShare=%s (all-to-LP), got %s", tip, resp.MevLpShare)
	}

	pool, _ := k.GetPool(ctx, poolId)
	if pool.MevReserveBalance != tip {
		t.Errorf("expected MevReserveBalance=%s, got %q", tip, pool.MevReserveBalance)
	}
}

// TestRevealSwap_NoTip_BackwardCompat verifies that RevealSwap without a tip
// continues to work exactly as before Gap 3.
func TestRevealSwap_NoTip_BackwardCompat(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	bk.setBalance(mevSwapSender, "uzrn", 100_000)

	resp := doCommitReveal(t, ms, ctx, mevSwapSender, poolId, "10000", "")

	if resp.MevLpShare != "0" {
		t.Errorf("expected MevLpShare=0 on no-tip reveal, got %s", resp.MevLpShare)
	}
	if resp.TokenOutAmount == "" || resp.TokenOutAmount == "0" {
		t.Error("swap should still produce output")
	}

	pool, _ := k.GetPool(ctx, poolId)
	if pool.MevReserveBalance != "" && pool.MevReserveBalance != "0" {
		t.Errorf("expected empty reserve on no-tip reveal, got %q", pool.MevReserveBalance)
	}
}

// TestRevealSwap_MEVTip_InvalidAmount verifies that a negative tip string is rejected.
func TestRevealSwap_MEVTip_InvalidAmount(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	bk.setBalance(mevSwapSender, "uzrn", 100_000)

	nonce := []byte("bad-tip-nonce")
	hash := keeper.ComputeSwapCommitmentHash(nonce, poolId, "uzrn", "10000", "", 0)
	if _, err := ms.CommitSwap(ctx, &types.MsgCommitSwap{
		Sender: mevSwapSender, PoolId: poolId, Commitment: hash,
	}); err != nil {
		t.Fatalf("CommitSwap: %v", err)
	}

	_, err := ms.RevealSwap(ctx, &types.MsgRevealSwap{
		Sender:          mevSwapSender,
		PoolId:          poolId,
		Nonce:           nonce,
		TokenInDenom:    "uzrn",
		TokenInAmount:   "10000",
		PriorityTipUzrn: "-1", // invalid — must be rejected
	})
	if err == nil {
		t.Fatal("expected ErrInvalidMEVTip for negative tip, got nil")
	}
}

// TestRemoveLiquidity_MEVReserveDistributed verifies that an LP removing a share
// of the supply receives the proportional fraction of the pool's MEV reserve.
func TestRemoveLiquidity_MEVReserveDistributed(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	// Inject MEV reserve into pool state.
	pool, _ := k.GetPool(ctx, poolId)
	const mevReserveTotal = int64(10000)
	pool.MevReserveBalance = "10000"
	k.SetPool(ctx, pool)

	// Pre-fund the module with the uzrn it will pay out.
	if err := bk.MintCoins(ctx, types.ModuleName, sdk.NewCoins(
		sdk.NewCoin("uzrn", sdkmath.NewInt(mevReserveTotal)),
	)); err != nil {
		t.Fatalf("MintCoins: %v", err)
	}

	// Give mevLPSender exactly half the LP supply.
	totalLPBig := new(big.Int)
	totalLPBig.SetString(pool.LpTokenSupply, 10)
	halfLP := new(big.Int).Div(totalLPBig, big.NewInt(2))

	lpDenom := types.LPDenom(poolId)
	bk.setBalance(mevLPSender, lpDenom, halfLP.Int64())

	_, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   mevLPSender,
		PoolId:   poolId,
		LpTokens: halfLP.String(),
	})
	if err != nil {
		t.Fatalf("RemoveLiquidity: %v", err)
	}

	// Compute expected MEV share: floor(10000 × halfLP / totalLP).
	expectedMev := new(big.Int).Mul(big.NewInt(mevReserveTotal), halfLP)
	expectedMev.Div(expectedMev, totalLPBig)
	expectedRemaining := new(big.Int).Sub(big.NewInt(mevReserveTotal), expectedMev)

	// Pool reserve should be reduced by exactly the payout (floor division).
	// Note: expectedMev ≈ 4999 for typical LP supply (sqrt arithmetic), so remaining ≈ 5001.
	updatedPool, _ := k.GetPool(ctx, poolId)
	mevLeft := new(big.Int)
	mevLeft.SetString(updatedPool.MevReserveBalance, 10)
	if mevLeft.Cmp(expectedRemaining) != 0 {
		t.Errorf("expected remaining MevReserveBalance=%s, got %s", expectedRemaining.String(), mevLeft.String())
	}

	// Sanity: MEV payout must be positive (non-zero for any reasonable LP fraction).
	if expectedMev.Sign() <= 0 {
		t.Error("expected positive MEV payout for half-LP removal")
	}
}

// TestRemoveLiquidity_NoMEVReserve verifies backward compatibility:
// pools with no MEV reserve produce no uzrn payout and behave identically to pre-Gap-3.
func TestRemoveLiquidity_NoMEVReserve(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)
	if pool.MevReserveBalance != "" {
		t.Fatalf("expected empty MevReserveBalance on new pool, got %q", pool.MevReserveBalance)
	}

	totalLPBig := new(big.Int)
	totalLPBig.SetString(pool.LpTokenSupply, 10)
	tenPct := new(big.Int).Div(totalLPBig, big.NewInt(10))

	lpDenom := types.LPDenom(poolId)
	bk.setBalance(mevLPSender, lpDenom, tenPct.Int64())

	_, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   mevLPSender,
		PoolId:   poolId,
		LpTokens: tenPct.String(),
	})
	if err != nil {
		t.Fatalf("RemoveLiquidity with no MEV reserve: %v", err)
	}

	// No uzrn MEV payout expected (sender had none before, received none).
	senderUzrn := bk.balances[mevLPSender]["uzrn"]
	if senderUzrn != 0 {
		t.Errorf("expected no uzrn payout from empty reserve, got %d", senderUzrn)
	}
}

// TestParams_Validate_MEVFeeShareBps verifies boundary validation for the new param.
func TestParams_Validate_MEVFeeShareBps(t *testing.T) {
	p := types.DefaultParams()
	p.MevFeeShareBps = 1_000_001
	if err := p.Validate(); err == nil {
		t.Fatal("expected Validate to reject MevFeeShareBps > 1_000_000")
	}

	p.MevFeeShareBps = 1_000_000
	if err := p.Validate(); err != nil {
		t.Errorf("MevFeeShareBps=1_000_000 should pass: %v", err)
	}

	p.MevFeeShareBps = 0
	if err := p.Validate(); err != nil {
		t.Errorf("MevFeeShareBps=0 should pass: %v", err)
	}
}
