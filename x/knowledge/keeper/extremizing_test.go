package keeper_test

// ─── Calibrated Forecasting Gap 5: Aggregate Extremizing ──────────────────────
//
// Tests for AggregationExtremizingAlpha param: when enabled, the stake-weighted
// vote-ratio confidence is pushed away from 50% using Tetlock's formula:
//
//	extremized = p^α / (p^α + (1−p)^α)
//
// where p is the raw aggregate confidence (0..1) and α > 1 pushes away from 0.5.
//
// Storage: alphaMilli = α × 1000 → 1000 = α=1.0 (identity), 1500 = α=1.5.
// Default: 0 = disabled. Only applied to ACCEPT/REJECT; MALFORMED/INCONCLUSIVE
// unchanged. Results clamped by MaxConfidence as usual.
//
// Key invariants:
//   - alphaMilli=0 (disabled): confidence unchanged.
//   - alphaMilli=1000 (α=1.0, identity): confidence unchanged.
//   - p=0.5 (500_000 BPS): fixed point — extremizing 50% → 50%.
//   - p>0.5, α>1: pushes higher (toward 1).
//   - p<0.5, α>1: pushes lower (toward 0).
//   - p=0: returns 0. p=1: returns 1_000_000.
//   - MALFORMED verdict: extremizing NOT applied.
//   - MaxConfidence cap still applies after extremizing.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Unit tests: pure extremizeConfidence function ────────────────────────────

// TestExtremize_Disabled verifies that alphaMilli=0 returns input unchanged.
func TestExtremize_Disabled(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(700_000, 0)
	require.Equal(t, uint64(700_000), got, "disabled: confidence unchanged")
}

// TestExtremize_Identity verifies that alphaMilli=1000 (α=1.0) is identity.
func TestExtremize_Identity(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(700_000, 1000)
	require.Equal(t, uint64(700_000), got, "α=1.0: identity")
}

// TestExtremize_FixedPoint verifies that 50% (500_000) is the fixed point.
func TestExtremize_FixedPoint(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(500_000, 1500)
	require.Equal(t, uint64(500_000), got, "50%% is the fixed point for any α")
}

// TestExtremize_FixedPoint_Alpha2 verifies 50% is fixed point for α=2.0.
func TestExtremize_FixedPoint_Alpha2(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(500_000, 2000)
	require.Equal(t, uint64(500_000), got, "50%% is the fixed point for α=2.0")
}

// TestExtremize_HighConfidence_PushesHigher verifies that α=1.5 pushes 70% upward.
// p=0.7, α=1.5: 0.7^1.5 ≈ 0.5857, 0.3^1.5 ≈ 0.1643
// extremized = 0.5857 / (0.5857 + 0.1643) ≈ 0.7812 → 781_200 BPS
func TestExtremize_HighConfidence_PushesHigher(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(700_000, 1500)
	// Must be > input (pushed away from 50%)
	require.Greater(t, got, uint64(700_000), "α=1.5 pushes 70%% upward")
	// Expected ≈ 781_200 BPS (within ±500 for floating-point rounding)
	require.InDelta(t, float64(781_200), float64(got), 500, "α=1.5 on 70%%: ≈78.1%%")
}

// TestExtremize_LowConfidence_PushesLower verifies that α=1.5 pushes 30% downward.
// Symmetric to above: 30% → ≈21.9%.
func TestExtremize_LowConfidence_PushesLower(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(300_000, 1500)
	require.Less(t, got, uint64(300_000), "α=1.5 pushes 30%% downward")
	require.InDelta(t, float64(218_800), float64(got), 500, "α=1.5 on 30%%: ≈21.9%%")
}

// TestExtremize_ZeroInput verifies that p=0 returns 0.
func TestExtremize_ZeroInput(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(0, 1500)
	require.Equal(t, uint64(0), got, "p=0 → 0")
}

// TestExtremize_FullInput verifies that p=1_000_000 returns 1_000_000.
func TestExtremize_FullInput(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(1_000_000, 1500)
	require.Equal(t, uint64(1_000_000), got, "p=1.0 → 1.0")
}

// TestExtremize_HighAlpha verifies that large α (e.g. 5.0) approaches binary.
// p=0.9, α=5.0: 0.9^5 = 0.59049, 0.1^5 = 0.00001
// extremized ≈ 0.59049 / 0.59050 ≈ 0.99998 → very close to 1_000_000.
func TestExtremize_HighAlpha(t *testing.T) {
	got := keeper.ExtremizeConfidenceForTest(900_000, 5000)
	require.Greater(t, got, uint64(990_000), "α=5.0 on 90%%: approaches 1.0")
}

// TestExtremize_Symmetry verifies that extremize(p) + extremize(1-p) = 1.
// The formula is symmetric: if 70% → X, then 30% → (1-X).
func TestExtremize_Symmetry(t *testing.T) {
	hi := keeper.ExtremizeConfidenceForTest(700_000, 1500)
	lo := keeper.ExtremizeConfidenceForTest(300_000, 1500)
	sum := hi + lo
	// Should be 1_000_000 (±1 for rounding)
	require.InDelta(t, float64(1_000_000), float64(sum), 1, "symmetry: extremize(p) + extremize(1-p) ≈ 1")
}

// ─── Integration tests: extremizing wired into AggregateVerificationResult ────

// TestExtremizing_Disabled_ConfidenceUnchanged verifies that with
// AggregationExtremizingAlpha=0 (default), the aggregate confidence is
// the raw stake-weighted vote ratio (no extremizing applied).
// Uses 4 accept + 1 reject (5 total) → ratio = 800_000, safely above
// ConfidenceThreshold (770_000) and MinHeadcountAgreement (3 accept).
func TestExtremizing_Disabled_ConfidenceUnchanged(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Default params: AggregationExtremizingAlpha = 0 (disabled).
	addrA := makeValidBech32Addr("extremize-dis-v1111")
	addrB := makeValidBech32Addr("extremize-dis-v2222")
	addrC := makeValidBech32Addr("extremize-dis-v3333")
	addrD := makeValidBech32Addr("extremize-dis-v4444")
	addrE := makeValidBech32Addr("extremize-dis-v5555")

	claim := &types.Claim{
		Id:          "claim-extremize-dis",
		FactContent: "Extremizing disabled: raw vote ratio as confidence",
		Domain:      "physics",
		Submitter:   makeValidBech32Addr("submitter-ex-dis0"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-ex-dis", "claim-extremize-dis", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept"},
		{Verifier: addrB, Vote: "accept"},
		{Verifier: addrC, Vote: "accept"},
		{Verifier: addrD, Vote: "accept"},
		{Verifier: addrE, Vote: "reject"},
	}
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA}, {Verifier: addrB}, {Verifier: addrC},
		{Verifier: addrD}, {Verifier: addrE},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict)
	// 4/5 accept → raw ratio = 800_000 BPS (stake=1 each, equal weight).
	// No extremizing → confidence stays at 800_000 (below MaxConfidence cap 880_000).
	require.Equal(t, uint64(800_000), result.Confidence, "disabled: confidence = raw vote ratio")
}

// TestExtremizing_Alpha1500_HighConfidence verifies that α=1.5 pushes
// high confidence (80%) upward when enabled.
// Uses 4 accept + 1 reject (5 total) → raw ratio = 800_000.
// extremize(800_000, 1500): p=0.8, 0.8^1.5≈0.716, 0.2^1.5≈0.089
// extremized ≈ 0.716/(0.716+0.089) ≈ 0.889 → 889_000 → capped at MaxConfidence (880_000).
func TestExtremizing_Alpha1500_HighConfidence(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 1500 // α=1.5
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("extremize-hi-conf-a1")
	addrB := makeValidBech32Addr("extremize-hi-conf-b2")
	addrC := makeValidBech32Addr("extremize-hi-conf-c3")
	addrD := makeValidBech32Addr("extremize-hi-conf-d4")
	addrE := makeValidBech32Addr("extremize-hi-conf-e5")

	claim := &types.Claim{
		Id:          "claim-extremize-hi",
		FactContent: "Extremizing α=1.5: high confidence pushed higher",
		Domain:      "mathematics",
		Submitter:   makeValidBech32Addr("submitter-ex-hi00"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-ex-hi", "claim-extremize-hi", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept"},
		{Verifier: addrB, Vote: "accept"},
		{Verifier: addrC, Vote: "accept"},
		{Verifier: addrD, Vote: "accept"},
		{Verifier: addrE, Vote: "reject"},
	}
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA}, {Verifier: addrB}, {Verifier: addrC},
		{Verifier: addrD}, {Verifier: addrE},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict)
	// Raw ratio = 800_000; extremized ≈ 889_000, capped at MaxConfidence (880_000).
	// Confidence must be > raw ratio (extremizing pushed it higher).
	require.Greater(t, result.Confidence, uint64(800_000), "α=1.5: confidence pushed above raw ratio")
}

// TestExtremizing_FixedPoint_Unanimous verifies that near-unanimous panels
// (confidence near 100%) remain near 100% after extremizing.
func TestExtremizing_FixedPoint_Unanimous(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 2000 // α=2.0
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("extremize-unan-a111")
	addrB := makeValidBech32Addr("extremize-unan-b222")
	addrC := makeValidBech32Addr("extremize-unan-c333")

	claim := &types.Claim{
		Id:          "claim-extremize-un",
		FactContent: "Extremizing: unanimous panel stays near 100%",
		Domain:      "logic",
		Submitter:   makeValidBech32Addr("submitter-ex-un"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-ex-un", "claim-extremize-un", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept"},
		{Verifier: addrB, Vote: "accept"},
		{Verifier: addrC, Vote: "accept"},
	}
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA}, {Verifier: addrB}, {Verifier: addrC},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict)
	// Unanimous → raw ratio = 1_000_000 → extremized = 1_000_000, capped by MaxConfidence
	// (MaxConfidence default = 880_000)
	p2, _ := k.GetParams(ctx)
	expectedCap := p2.MaxConfidence
	require.Equal(t, expectedCap, result.Confidence, "unanimous: confidence at MaxConfidence cap")
}

// TestExtremizing_Malformed_NotApplied verifies that MALFORMED verdicts
// do not have extremizing applied (non-binary probability).
func TestExtremizing_Malformed_NotApplied(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 1500 // α=1.5 enabled
	p.ConfidenceThreshold = 600_000      // lower threshold for test
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("extremize-malform-a")
	addrB := makeValidBech32Addr("extremize-malform-b")
	addrC := makeValidBech32Addr("extremize-malform-c")

	claim := &types.Claim{
		Id:          "claim-extremize-mf",
		FactContent: "Extremizing NOT applied to MALFORMED verdict",
		Domain:      "linguistics",
		Submitter:   makeValidBech32Addr("submitter-ex-mf"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-ex-mf", "claim-extremize-mf", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "malformed"},
		{Verifier: addrB, Vote: "malformed"},
		{Verifier: addrC, Vote: "malformed"},
	}
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA}, {Verifier: addrB}, {Verifier: addrC},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_MALFORMED, result.Verdict, "verdict is MALFORMED")
	// Confidence is raw ratio (1_000_000), capped by MaxConfidence (880_000).
	// Extremizing NOT applied to MALFORMED — result.Confidence should equal
	// raw ratio capped by MaxConfidence (same as if extremizing were disabled).
	p2, _ := k.GetParams(ctx)
	require.Equal(t, p2.MaxConfidence, result.Confidence, "MALFORMED: confidence = MaxConfidence cap, no extremizing")
}

// TestExtremizing_ParamsValidate_TooSmall verifies Validate rejects α < 1.0
// (alphaMilli > 0 && < 1000).
func TestExtremizing_ParamsValidate_TooSmall(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 500 // α=0.5 — compresses, invalid
	err := p.Validate()
	require.Error(t, err, "α < 1.0 should be rejected by Validate")
	_ = k // keeper not used; context kept for test helper compat
	_ = ctx
}

// TestExtremizing_ParamsValidate_TooLarge verifies Validate rejects α > 10.0
// (alphaMilli > 10_000).
func TestExtremizing_ParamsValidate_TooLarge(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 10_001 // α=10.001 — out of range
	err := p.Validate()
	require.Error(t, err, "α > 10.0 should be rejected by Validate")
	_ = k
	_ = ctx
}

// TestExtremizing_ParamsValidate_Valid verifies Validate accepts valid α values.
func TestExtremizing_ParamsValidate_Valid(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	p, _ := k.GetParams(ctx)
	p.AggregationExtremizingAlpha = 1500 // α=1.5 — valid
	err := p.Validate()
	require.NoError(t, err, "α=1.5 is valid")
	_ = k
	_ = ctx
}
