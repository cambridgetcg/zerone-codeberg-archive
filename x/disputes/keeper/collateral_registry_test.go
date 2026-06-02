package keeper_test

// ─── Counterparty Escrow Gap 4: Cross-Module Collateral Registry ──────────────
//
// Gap 4 (★★★☆☆) — counterparty-escrow-brief.md §Gap 4
//
// Bond requirements in disputes, claim stakes in knowledge, LP positions in
// liquiditypool, and validator stakes in staking are tracked independently.
// Without a unified view, an agent can over-commit capital across modules.
//
// This file tests:
//   - CollateralLock CRUD (register, get, iterate, release)
//   - GetTotalLockedCollateral sums across multiple concurrent disputes
//   - CheckLeverageLimit enforces MaxLeverageBps ceiling
//   - InitiateDispute rejects bonds that exceed the leverage limit
//   - DistributeSettlement releases the challenger's lock
//   - Params.Validate() rejects MaxLeverageBps > 1_000_000

import (
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

// ─── Collateral Lock CRUD ─────────────────────────────────────────────────────

// TestCollateralRegistry_Empty — no locks → total = 0.
func TestCollateralRegistry_Empty(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-empty")
	total := k.GetTotalLockedCollateral(ctx, addr)
	require.Zero(t, total.Sign(), "no locks registered → total must be 0")
}

// TestCollateralRegistry_RegisterAndGet — register a single lock and read it back.
func TestCollateralRegistry_RegisterAndGet(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-register")

	k.RegisterCollateralLock(ctx, addr, "lock-1", "5000000")
	got := k.GetCollateralLock(ctx, addr, "lock-1")
	require.Equal(t, int64(5_000_000), got.Int64())
}

// TestCollateralRegistry_TotalAggregates — two concurrent locks sum correctly.
func TestCollateralRegistry_TotalAggregates(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-agg")

	k.RegisterCollateralLock(ctx, addr, "lock-a", "3000000")
	k.RegisterCollateralLock(ctx, addr, "lock-b", "2000000")

	total := k.GetTotalLockedCollateral(ctx, addr)
	require.Equal(t, int64(5_000_000), total.Int64(),
		"total must be sum of all locks for the address")
}

// TestCollateralRegistry_ReleaseOne — releasing one of two locks decreases total correctly.
func TestCollateralRegistry_ReleaseOne(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-release-one")

	k.RegisterCollateralLock(ctx, addr, "lock-x", "4000000")
	k.RegisterCollateralLock(ctx, addr, "lock-y", "1000000")

	k.ReleaseCollateralLock(ctx, addr, "lock-x")
	total := k.GetTotalLockedCollateral(ctx, addr)
	require.Equal(t, int64(1_000_000), total.Int64(),
		"after releasing lock-x (4M), only lock-y (1M) should remain")
}

// TestCollateralRegistry_ReleaseAll — releasing all locks returns total to 0.
func TestCollateralRegistry_ReleaseAll(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-release-all")

	k.RegisterCollateralLock(ctx, addr, "lock-p", "2000000")
	k.RegisterCollateralLock(ctx, addr, "lock-q", "3000000")

	k.ReleaseCollateralLock(ctx, addr, "lock-p")
	k.ReleaseCollateralLock(ctx, addr, "lock-q")

	total := k.GetTotalLockedCollateral(ctx, addr)
	require.Zero(t, total.Sign(), "after releasing all locks total must be 0")
}

// TestCollateralRegistry_ReleaseIdempotent — releasing a non-existent lock is a no-op.
func TestCollateralRegistry_ReleaseIdempotent(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("user-idempotent")

	// Release on an empty registry should not panic or corrupt state.
	require.NotPanics(t, func() {
		k.ReleaseCollateralLock(ctx, addr, "does-not-exist")
	})

	k.RegisterCollateralLock(ctx, addr, "lock-1", "1000000")
	k.ReleaseCollateralLock(ctx, addr, "lock-1")
	// Second release on same ID should also be a no-op.
	require.NotPanics(t, func() {
		k.ReleaseCollateralLock(ctx, addr, "lock-1")
	})
	total := k.GetTotalLockedCollateral(ctx, addr)
	require.Zero(t, total.Sign())
}

// TestCollateralRegistry_MultiAddress — locks for different addresses are independent.
func TestCollateralRegistry_MultiAddress(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	alice := testAddr("alice")
	bob := testAddr("bob")

	k.RegisterCollateralLock(ctx, alice, "lock-alice", "7000000")
	k.RegisterCollateralLock(ctx, bob, "lock-bob", "3000000")

	aliceTotal := k.GetTotalLockedCollateral(ctx, alice)
	bobTotal := k.GetTotalLockedCollateral(ctx, bob)

	require.Equal(t, int64(7_000_000), aliceTotal.Int64(), "alice total must be independent of bob")
	require.Equal(t, int64(3_000_000), bobTotal.Int64(), "bob total must be independent of alice")

	k.ReleaseCollateralLock(ctx, alice, "lock-alice")
	require.Zero(t, k.GetTotalLockedCollateral(ctx, alice).Sign(), "alice total after release")
	require.Equal(t, int64(3_000_000), k.GetTotalLockedCollateral(ctx, bob).Int64(), "bob total unaffected by alice release")
}

// ─── Leverage Limit ───────────────────────────────────────────────────────────

// TestLeverageLimit_Disabled — MaxLeverageBps = 0 disables the check entirely.
func TestLeverageLimit_Disabled(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	addr := testAddr("user-disabled")
	addrSDK, err := sdk.AccAddressFromBech32(addr)
	require.NoError(t, err)
	bk.setBalance(addr, "uzrn", 1_000) // very small balance

	params := types.DefaultParams()
	params.MaxLeverageBps = 0 // disabled

	// Even a bond larger than the balance should pass when disabled.
	import_big := int64(1_000_000)
	bigBond := bigIntFromTestInt(import_big)

	err = k.CheckLeverageLimit(ctx, addrSDK, bigBond, params)
	require.NoError(t, err, "MaxLeverageBps=0 must disable the leverage check")
}

// TestLeverageLimit_BelowLimit — bond well within the leverage ceiling is allowed.
func TestLeverageLimit_BelowLimit(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	addr := testAddr("user-below")
	addrSDK, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(addr, "uzrn", 10_000_000) // 10M balance

	params := types.DefaultParams()
	params.MaxLeverageBps = 500_000 // 50% max

	bond := bigIntFromTestInt(1_000_000) // 1M = 10% of 10M
	err := k.CheckLeverageLimit(ctx, addrSDK, bond, params)
	require.NoError(t, err, "10%% used of 50%% limit must pass")
}

// TestLeverageLimit_AtLimit — bond exactly at the leverage ceiling is allowed (≤ not <).
func TestLeverageLimit_AtLimit(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	addr := testAddr("user-at-limit")
	addrSDK, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(addr, "uzrn", 2_000_000) // 2M balance

	params := types.DefaultParams()
	params.MaxLeverageBps = 500_000 // 50% max

	bond := bigIntFromTestInt(1_000_000) // 1M = exactly 50% of 2M
	err := k.CheckLeverageLimit(ctx, addrSDK, bond, params)
	require.NoError(t, err, "exactly 50%% of 50%% limit must pass (≤)")
}

// TestLeverageLimit_ExceedsLimit — bond just over the ceiling is rejected.
func TestLeverageLimit_ExceedsLimit(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	addr := testAddr("user-exceeds")
	addrSDK, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(addr, "uzrn", 1_900_000) // 1.9M balance

	params := types.DefaultParams()
	params.MaxLeverageBps = 500_000 // 50% max; 1M/1.9M ≈ 52.6% > 50%

	bond := bigIntFromTestInt(1_000_000)
	err := k.CheckLeverageLimit(ctx, addrSDK, bond, params)
	require.ErrorIs(t, err, types.ErrLeverageExceeded,
		"1M bond against 1.9M balance at 50%% limit must be rejected")
}

// TestLeverageLimit_CumulativeBlocks — existing locks are included in the check.
func TestLeverageLimit_CumulativeBlocks(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	addr := testAddr("user-cumulative")
	addrSDK, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(addr, "uzrn", 10_000_000)

	params := types.DefaultParams()
	params.MaxLeverageBps = 500_000 // 50% max = 5M

	// Register existing 4M lock (represents a prior dispute).
	k.RegisterCollateralLock(ctx, addr, "prior-dispute", "4000000")

	// New bond of 1M → total would be 5M = exactly 50% → allowed.
	bond1 := bigIntFromTestInt(1_000_000)
	err := k.CheckLeverageLimit(ctx, addrSDK, bond1, params)
	require.NoError(t, err, "4M existing + 1M new = 5M = 50%% → must pass")

	// New bond of 1.1M → total would be 5.1M > 50% → blocked.
	bond2 := bigIntFromTestInt(1_100_000)
	err = k.CheckLeverageLimit(ctx, addrSDK, bond2, params)
	require.ErrorIs(t, err, types.ErrLeverageExceeded,
		"4M existing + 1.1M new = 5.1M > 50%% → must be rejected")
}

// ─── Integration: InitiateDispute + leverage enforcement ─────────────────────

// TestInitiateDispute_LeverageCheck_DefaultDisabled — default params have MaxLeverageBps=0,
// so the leverage check is disabled and disputes proceed normally.
func TestInitiateDispute_LeverageCheck_DefaultDisabled(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger-no-limit")
	defender := testAddr("defender-no-limit")
	bk.setBalance(challenger, "uzrn", 1_500_000) // barely enough for bond
	kk.addFact("fact-disabled", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-disabled",
		Reason:     "test",
		Bond:       "1000000", // 66% of balance, but limit is disabled
	})
	require.NoError(t, err, "leverage check disabled by default → dispute must succeed")
	require.NotEmpty(t, resp.DisputeId)

	// Collateral lock registered.
	total := k.GetTotalLockedCollateral(ctx, challenger)
	require.Equal(t, int64(1_000_000), total.Int64(),
		"challenger's bond must be registered in collateral registry")
}

// TestInitiateDispute_LeverageCheck_Enforced — when MaxLeverageBps is set,
// a bond that exceeds the limit is rejected before escrow.
func TestInitiateDispute_LeverageCheck_Enforced(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger-enforced")
	defender := testAddr("defender-enforced")
	// Balance = 4M; bond = 1M; MaxLeverageBps = 200_000 (20%).
	// 1M / 4M = 25% > 20% → should be rejected.
	bk.setBalance(challenger, "uzrn", 4_000_000)
	kk.addFact("fact-enforced", defender)

	// Set MaxLeverageBps on params.
	params := types.DefaultParams()
	params.MaxLeverageBps = 200_000 // 20% max
	k.SetParams(ctx, params)

	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-enforced",
		Reason:     "test",
		Bond:       "1000000",
	})
	require.ErrorIs(t, err, types.ErrLeverageExceeded,
		"bond of 25%% of balance against 20%% limit must be rejected")

	// No funds should have been escrowed.
	require.Equal(t, int64(4_000_000), bk.balances[challenger]["uzrn"],
		"challenger balance must be unchanged when leverage check fails")

	// No collateral lock registered.
	total := k.GetTotalLockedCollateral(ctx, challenger)
	require.Zero(t, total.Sign(), "no lock must be registered on rejected dispute")
}

// TestInitiateDispute_LeverageCheck_AtLimit — bond exactly at the limit is accepted.
func TestInitiateDispute_LeverageCheck_AtLimit(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger-at-limit")
	defender := testAddr("defender-at-limit")
	// Balance = 5M; bond = 1M; MaxLeverageBps = 200_000 (20%).
	// 1M / 5M = 20% = exactly the limit → must be allowed (≤ not <).
	bk.setBalance(challenger, "uzrn", 5_000_000)
	kk.addFact("fact-at-limit", defender)

	params := types.DefaultParams()
	params.MaxLeverageBps = 200_000 // 20% max
	k.SetParams(ctx, params)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-at-limit",
		Reason:     "test",
		Bond:       "1000000",
	})
	require.NoError(t, err, "bond exactly at limit must be accepted")
	require.NotEmpty(t, resp.DisputeId)
}

// ─── Integration: DistributeSettlement releases lock ─────────────────────────

// TestDistributeSettlement_ReleasesCollateralLock — after settlement the
// challenger's collateral lock is removed, allowing future disputes.
func TestDistributeSettlement_ReleasesCollateralLock(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger-settle")
	defender := testAddr("defender-settle")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	bk.setBalance(defender, "uzrn", 5_000_000)
	kk.addFact("fact-settle", defender)

	// Open a dispute.
	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-settle",
		Reason:     "settle test",
		Bond:       "1000000",
	})
	require.NoError(t, err)
	disputeID := resp.DisputeId

	// Verify lock registered.
	require.Equal(t, int64(1_000_000), k.GetTotalLockedCollateral(ctx, challenger).Int64(),
		"collateral lock must be registered after InitiateDispute")

	// Set outcome and settle.
	dispute, found := k.GetDispute(ctx, disputeID)
	require.True(t, found)
	dispute.Outcome = types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT
	k.SetDispute(ctx, dispute)

	err = k.DistributeSettlement(ctx, dispute)
	require.NoError(t, err)

	// Lock must be released after settlement.
	total := k.GetTotalLockedCollateral(ctx, challenger)
	require.Zero(t, total.Sign(),
		"collateral lock must be released after DistributeSettlement")
}

// ─── Params validation ────────────────────────────────────────────────────────

// TestValidate_MaxLeverageBps_Valid — 0 (disabled) and values <= 1_000_000 pass.
func TestValidate_MaxLeverageBps_Valid(t *testing.T) {
	p := types.DefaultParams()
	p.MaxLeverageBps = 0
	require.NoError(t, p.Validate(), "0 (disabled) must pass Validate")

	p.MaxLeverageBps = 500_000
	require.NoError(t, p.Validate(), "500_000 (50%%) must pass Validate")

	p.MaxLeverageBps = 1_000_000
	require.NoError(t, p.Validate(), "1_000_000 (100%%) must pass Validate")
}

// TestValidate_MaxLeverageBps_TooHigh — values > 1_000_000 are rejected.
func TestValidate_MaxLeverageBps_TooHigh(t *testing.T) {
	p := types.DefaultParams()
	p.MaxLeverageBps = 1_000_001
	err := p.Validate()
	require.Error(t, err, "MaxLeverageBps > 1_000_000 must fail Validate")
	require.Contains(t, err.Error(), "max_leverage_bps")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// bigIntFromTestInt constructs a *big.Int from an int64 for test readability.
func bigIntFromTestInt(n int64) *big.Int {
	return big.NewInt(n)
}
