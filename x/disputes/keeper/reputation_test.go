package keeper_test

// ─── Counterparty Escrow Gap 5: Reputation-Adjusted Bond Requirements ─────────
//
// Gap 5 (★★★☆☆) — counterparty-escrow-brief.md §Gap 5
//
// Bond requirements are fixed per tier. A challenger with a strong track record
// of successful challenges should post a smaller bond; a challenger with a poor
// track record should post a larger bond. This file tests the full reputation
// system from profile CRUD through integration with InitiateDispute and
// DistributeSettlement.

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

// ─── DisputerProfile CRUD ─────────────────────────────────────────────────────

// TestDisputerProfile_EmptyIsZero — a profile that has never been written returns zeros.
func TestDisputerProfile_EmptyIsZero(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("profile-empty")
	p := k.GetDisputerProfile(ctx, addr)
	require.Equal(t, addr, p.Address)
	require.Zero(t, p.WinCount)
	require.Zero(t, p.TotalDisputes)
}

// TestDisputerProfile_SetAndGet — written profile is retrieved correctly.
func TestDisputerProfile_SetAndGet(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("profile-set")

	k.SetDisputerProfile(ctx, &types.DisputerProfile{
		Address:       addr,
		WinCount:      7,
		TotalDisputes: 10,
	})

	p := k.GetDisputerProfile(ctx, addr)
	require.Equal(t, uint64(7), p.WinCount)
	require.Equal(t, uint64(10), p.TotalDisputes)
}

// ─── RecordDisputeOutcome ─────────────────────────────────────────────────────

// TestRecordOutcome_Win — CHALLENGER_WINS increments both win_count and total.
func TestRecordOutcome_Win(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("record-win")

	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS)
	p := k.GetDisputerProfile(ctx, addr)
	require.Equal(t, uint64(1), p.WinCount)
	require.Equal(t, uint64(1), p.TotalDisputes)
}

// TestRecordOutcome_Loss — DEFENDER_WINS increments only total_disputes.
func TestRecordOutcome_Loss(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("record-loss")

	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS)
	p := k.GetDisputerProfile(ctx, addr)
	require.Zero(t, p.WinCount, "DEFENDER_WINS must not increment win_count")
	require.Equal(t, uint64(1), p.TotalDisputes)
}

// TestRecordOutcome_Draw — DRAW increments only total_disputes.
func TestRecordOutcome_Draw(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("record-draw")

	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_DRAW)
	p := k.GetDisputerProfile(ctx, addr)
	require.Zero(t, p.WinCount)
	require.Equal(t, uint64(1), p.TotalDisputes)
}

// TestRecordOutcome_Timeout — TIMED_OUT increments only total_disputes.
func TestRecordOutcome_Timeout(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("record-timeout")

	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT)
	p := k.GetDisputerProfile(ctx, addr)
	require.Zero(t, p.WinCount)
	require.Equal(t, uint64(1), p.TotalDisputes)
}

// TestRecordOutcome_Cumulative — multiple outcomes accumulate correctly.
func TestRecordOutcome_Cumulative(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("record-cumulative")

	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS) // win 1
	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS) // win 2
	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS)   // loss
	k.RecordDisputeOutcome(ctx, addr, types.DisputeOutcome_DISPUTE_OUTCOME_DRAW)            // draw

	p := k.GetDisputerProfile(ctx, addr)
	require.Equal(t, uint64(2), p.WinCount, "two CHALLENGER_WINS outcomes")
	require.Equal(t, uint64(4), p.TotalDisputes, "4 total outcomes")
}

// ─── ComputeEffectiveMinBond ──────────────────────────────────────────────────

func reputationParams() *types.Params {
	p := types.DefaultParams()
	p.ReputationMinRounds = 10
	p.ReputationHighThresholdBps = 800_000 // 80%
	p.ReputationLowThresholdBps = 200_000  // 20%
	p.ReputationDiscountBps = 200_000      // 20% off for high rep
	p.ReputationPenaltyMultiplierBps = 2_000_000 // 2× for low rep
	return p
}

// TestEffectiveMinBond_Disabled — ReputationMinRounds=0 disables the system.
func TestEffectiveMinBond_Disabled(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-disabled")
	// Give the address 20 wins out of 20 — would normally trigger discount.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 20, TotalDisputes: 20})

	params := types.DefaultParams()
	params.ReputationMinRounds = 0 // disabled

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, params)
	require.Equal(t, int64(1_000_000), got.Int64(),
		"disabled: effective min must equal tier minimum regardless of profile")
}

// TestEffectiveMinBond_BelowMinRounds — not enough history: standard bond applies.
func TestEffectiveMinBond_BelowMinRounds(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-below-min-rounds")
	// Only 5 disputes — below the 10-round gate.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 5, TotalDisputes: 5})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(1_000_000), got.Int64(),
		"below min_rounds: effective min must equal tier minimum (no reward/penalty yet)")
}

// TestEffectiveMinBond_HighRep_Discount — win rate > 80% triggers 20% discount.
func TestEffectiveMinBond_HighRep_Discount(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-high-rep")
	// 9 wins out of 10 = 90% > 80% → 20% discount on 1_000_000 → 800_000.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 9, TotalDisputes: 10})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(800_000), got.Int64(),
		"90%% win rate with 20%% discount: effective min must be 800_000")
}

// TestEffectiveMinBond_LowRep_Penalty — win rate < 20% triggers 2× penalty.
func TestEffectiveMinBond_LowRep_Penalty(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-low-rep")
	// 1 win out of 10 = 10% < 20% → 2× penalty on 1_000_000 → 2_000_000.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 1, TotalDisputes: 10})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(2_000_000), got.Int64(),
		"10%% win rate with 2× penalty: effective min must be 2_000_000")
}

// TestEffectiveMinBond_MiddleBand — win rate between thresholds: standard bond.
func TestEffectiveMinBond_MiddleBand(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-middle")
	// 5 wins out of 10 = 50% — above low threshold (20%), below high (80%).
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 5, TotalDisputes: 10})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(1_000_000), got.Int64(),
		"50%% win rate in middle band: effective min must equal tier minimum")
}

// TestEffectiveMinBond_ExactHighThreshold — exactly at high threshold: no discount (> not >=).
func TestEffectiveMinBond_ExactHighThreshold(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-exact-high")
	// 8 wins out of 10 = 80% = exactly the high threshold → middle band, no discount.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 8, TotalDisputes: 10})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(1_000_000), got.Int64(),
		"exactly 80%% win rate: condition is >, not >=, so no discount")
}

// TestEffectiveMinBond_ExactLowThreshold — exactly at low threshold: no penalty (< not <=).
func TestEffectiveMinBond_ExactLowThreshold(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-exact-low")
	// 2 wins out of 10 = 20% = exactly the low threshold → middle band, no penalty.
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr, WinCount: 2, TotalDisputes: 10})

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(1_000_000), got.Int64(),
		"exactly 20%% win rate: condition is <, not <=, so no penalty")
}

// TestEffectiveMinBond_NoProfile — challenger with no history gets standard bond.
func TestEffectiveMinBond_NoProfile(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr := testAddr("eff-no-profile")
	// No profile written — TotalDisputes=0 < ReputationMinRounds=10 → standard bond.

	tierMin := big.NewInt(1_000_000)
	got := k.ComputeEffectiveMinBond(ctx, addr, tierMin, reputationParams())
	require.Equal(t, int64(1_000_000), got.Int64(),
		"no profile: must not panic, must return tier minimum")
}

// ─── Integration: InitiateDispute + reputation ────────────────────────────────

// TestInitiateDispute_Reputation_DefaultDisabled — reputation system is off by default.
// A new challenger can open a dispute with the standard tier minimum bond.
func TestInitiateDispute_Reputation_DefaultDisabled(t *testing.T) {
	msgSrv, _, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-rep-default")
	defender := testAddr("def-rep-default")
	bk.setBalance(challenger, "uzrn", 2_000_000)
	kk.addFact("fact-rep-default", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-rep-default",
		Reason:     "reputation default disabled",
		Bond:       "1000000", // exactly the tier minimum
	})
	require.NoError(t, err, "default params: reputation system disabled, tier minimum bond must suffice")
	require.NotEmpty(t, resp.DisputeId)
}

// TestInitiateDispute_Reputation_HighRep_CanPostLessThanTierMin — a high-reputation
// challenger may post less than the tier minimum because the effective minimum is lower.
func TestInitiateDispute_Reputation_HighRep_CanPostLessThanTierMin(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-high-rep")
	defender := testAddr("def-high-rep")
	bk.setBalance(challenger, "uzrn", 2_000_000)
	kk.addFact("fact-high-rep", defender)

	// Enable reputation system and give challenger a 90% win rate.
	params := reputationParams()
	k.SetParams(ctx, params)
	k.SetDisputerProfile(ctx, &types.DisputerProfile{
		Address: challenger, WinCount: 9, TotalDisputes: 10,
	})

	// Tier minimum = 1_000_000. With 20% discount, effective min = 800_000.
	// Posting 800_000 must succeed.
	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-high-rep",
		Reason:     "high-rep discount test",
		Bond:       "800000", // below tier min but within discounted effective min
	})
	require.NoError(t, err, "90%% win rate with 20%% discount: 800_000 bond must be accepted")
	require.NotEmpty(t, resp.DisputeId)
}

// TestInitiateDispute_Reputation_HighRep_BelowEffectiveMin — a high-reputation
// challenger posting less than the discounted effective minimum is still rejected.
func TestInitiateDispute_Reputation_HighRep_BelowEffectiveMin(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-high-rep-below")
	defender := testAddr("def-high-rep-below")
	bk.setBalance(challenger, "uzrn", 2_000_000)
	kk.addFact("fact-high-rep-below", defender)

	params := reputationParams()
	k.SetParams(ctx, params)
	k.SetDisputerProfile(ctx, &types.DisputerProfile{
		Address: challenger, WinCount: 9, TotalDisputes: 10,
	})

	// Effective min = 800_000; posting 799_999 must fail.
	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-high-rep-below",
		Reason:     "high-rep below effective min",
		Bond:       "799999",
	})
	require.Error(t, err, "799_999 < effective min 800_000 must be rejected")
}

// TestInitiateDispute_Reputation_LowRep_PenaltyEnforced — a low-reputation
// challenger must post more than the tier minimum.
func TestInitiateDispute_Reputation_LowRep_PenaltyEnforced(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-low-rep")
	defender := testAddr("def-low-rep")
	bk.setBalance(challenger, "uzrn", 5_000_000)
	kk.addFact("fact-low-rep", defender)

	params := reputationParams()
	k.SetParams(ctx, params)
	k.SetDisputerProfile(ctx, &types.DisputerProfile{
		Address: challenger, WinCount: 1, TotalDisputes: 10, // 10% win rate
	})

	// Effective min = 2_000_000 (2× penalty).
	// Posting tier min 1_000_000 must fail.
	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-low-rep",
		Reason:     "low-rep penalty test",
		Bond:       "1000000", // tier min — not enough after 2× penalty
	})
	require.Error(t, err, "tier minimum bond with low-rep penalty active must be rejected")
}

// TestInitiateDispute_Reputation_LowRep_SufficientBond — same low-rep challenger
// posting the penalized effective minimum succeeds.
func TestInitiateDispute_Reputation_LowRep_SufficientBond(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-low-rep-ok")
	defender := testAddr("def-low-rep-ok")
	bk.setBalance(challenger, "uzrn", 5_000_000)
	kk.addFact("fact-low-rep-ok", defender)

	params := reputationParams()
	k.SetParams(ctx, params)
	k.SetDisputerProfile(ctx, &types.DisputerProfile{
		Address: challenger, WinCount: 1, TotalDisputes: 10, // 10% win rate → 2× = 2_000_000
	})

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-low-rep-ok",
		Reason:     "low-rep sufficient bond",
		Bond:       "2000000", // exactly the penalized effective min
	})
	require.NoError(t, err, "posting exactly the penalized effective min must succeed")
	require.NotEmpty(t, resp.DisputeId)
}

// ─── Integration: DistributeSettlement updates reputation ─────────────────────

// TestDistributeSettlement_UpdatesReputation_Win — after a challenger wins,
// their profile's win_count and total_disputes both increment.
func TestDistributeSettlement_UpdatesReputation_Win(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-rep-win")
	defender := testAddr("def-rep-win")
	bk.setBalance(challenger, "uzrn", 5_000_000)
	bk.setBalance(defender, "uzrn", 5_000_000)
	kk.addFact("fact-rep-win", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-rep-win",
		Reason:     "rep win test",
		Bond:       "1000000",
	})
	require.NoError(t, err)

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Outcome = types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS
	k.SetDispute(ctx, dispute)

	err = k.DistributeSettlement(ctx, dispute)
	require.NoError(t, err)

	p := k.GetDisputerProfile(ctx, challenger)
	require.Equal(t, uint64(1), p.WinCount, "CHALLENGER_WINS must increment win_count")
	require.Equal(t, uint64(1), p.TotalDisputes, "must increment total_disputes")
}

// TestDistributeSettlement_UpdatesReputation_Loss — after a challenger loses,
// only total_disputes increments (win_count stays 0).
func TestDistributeSettlement_UpdatesReputation_Loss(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("chall-rep-loss")
	defender := testAddr("def-rep-loss")
	bk.setBalance(challenger, "uzrn", 5_000_000)
	bk.setBalance(defender, "uzrn", 5_000_000)
	kk.addFact("fact-rep-loss", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-rep-loss",
		Reason:     "rep loss test",
		Bond:       "1000000",
	})
	require.NoError(t, err)

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Outcome = types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS
	k.SetDispute(ctx, dispute)

	err = k.DistributeSettlement(ctx, dispute)
	require.NoError(t, err)

	p := k.GetDisputerProfile(ctx, challenger)
	require.Zero(t, p.WinCount, "DEFENDER_WINS must not increment win_count")
	require.Equal(t, uint64(1), p.TotalDisputes, "must increment total_disputes")
}

// ─── Params validation ────────────────────────────────────────────────────────

// TestValidate_Reputation_Disabled — ReputationMinRounds=0: other fields not validated.
func TestValidate_Reputation_Disabled(t *testing.T) {
	p := types.DefaultParams()
	p.ReputationMinRounds = 0
	// Set invalid values — they must be ignored when the system is disabled.
	p.ReputationDiscountBps = 1_500_000      // would be invalid if enabled
	p.ReputationPenaltyMultiplierBps = 0     // would be invalid if enabled
	p.ReputationHighThresholdBps = 2_000_000 // would be invalid if enabled
	require.NoError(t, p.Validate(), "disabled reputation: invalid param values must be ignored")
}

// TestValidate_Reputation_Valid — a fully configured reputation system passes.
func TestValidate_Reputation_Valid(t *testing.T) {
	require.NoError(t, reputationParams().Validate(), "valid reputation params must pass Validate")
}

// TestValidate_Reputation_DiscountTooHigh — discount >= 1_000_000 rejected.
func TestValidate_Reputation_DiscountTooHigh(t *testing.T) {
	p := reputationParams()
	p.ReputationDiscountBps = 1_000_000
	require.Error(t, p.Validate(), "reputation_discount_bps == 1_000_000 must fail Validate")
}

// TestValidate_Reputation_PenaltyZero — penalty multiplier = 0 rejected.
func TestValidate_Reputation_PenaltyZero(t *testing.T) {
	p := reputationParams()
	p.ReputationPenaltyMultiplierBps = 0
	require.Error(t, p.Validate(), "reputation_penalty_multiplier_bps == 0 must fail Validate")
}

// TestValidate_Reputation_HighThresholdTooHigh — high threshold > 1_000_000 rejected.
func TestValidate_Reputation_HighThresholdTooHigh(t *testing.T) {
	p := reputationParams()
	p.ReputationHighThresholdBps = 1_000_001
	require.Error(t, p.Validate(), "reputation_high_threshold_bps > 1_000_000 must fail Validate")
}

// TestValidate_Reputation_LowGeqHigh — low threshold >= high threshold rejected.
func TestValidate_Reputation_LowGeqHigh(t *testing.T) {
	p := reputationParams()
	p.ReputationLowThresholdBps = p.ReputationHighThresholdBps // equal → must fail
	require.Error(t, p.Validate(), "reputation_low_threshold_bps == high must fail Validate")

	p.ReputationLowThresholdBps = p.ReputationHighThresholdBps + 1 // above → must also fail
	require.Error(t, p.Validate(), "reputation_low_threshold_bps > high must fail Validate")
}

// ─── Genesis round-trip ───────────────────────────────────────────────────────

// TestGenesis_DisputerProfiles_RoundTrip — disputer profiles survive export/import.
func TestGenesis_DisputerProfiles_RoundTrip(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	addr1 := testAddr("genesis-profile-a")
	addr2 := testAddr("genesis-profile-b")

	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr1, WinCount: 8, TotalDisputes: 10})
	k.SetDisputerProfile(ctx, &types.DisputerProfile{Address: addr2, WinCount: 3, TotalDisputes: 15})

	exported := k.ExportGenesis(ctx)
	require.Len(t, exported.DisputerProfiles, 2, "two profiles must be exported")

	// Re-import into a fresh keeper.
	k2, ctx2, _, _, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, exported)

	p1 := k2.GetDisputerProfile(ctx2, addr1)
	require.Equal(t, uint64(8), p1.WinCount)
	require.Equal(t, uint64(10), p1.TotalDisputes)

	p2 := k2.GetDisputerProfile(ctx2, addr2)
	require.Equal(t, uint64(3), p2.WinCount)
	require.Equal(t, uint64(15), p2.TotalDisputes)
}
