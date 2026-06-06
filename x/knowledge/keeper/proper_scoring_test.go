package keeper_test

// ─── Calibrated Forecasting Gap 1: Proper Scoring Rule ────────────────────────
//
// Tests for ProperScoringRuleEnabled param: when enabled, verifier reward
// shares within the correct-verifier pool are proportional to individual
// Brier correctness scores instead of equal shares.
//
// Key invariants:
//   - ProperScoringRuleEnabled=false (default) → equal shares (existing behaviour).
//   - ProperScoringRuleEnabled=true, ACCEPT/REJECT outcome → Brier-proportional.
//   - MALFORMED/INCONCLUSIVE outcomes → equal shares even when enabled.
//   - ConfidenceBps=0 (not reported) → treated as 500_000 (neutral 50%).
//   - Higher correct confidence → larger share.
//   - All same confidence → equal shares (proportional collapses to equal).
//   - Single correct verifier → gets whole pool regardless.
//   - Total distributed always equals verifier pool (conservation).
//
// Brier correctness math (BPS scale):
//   pAcceptBps  = confidenceBps if vote==accept; 1_000_000 - confidenceBps if vote==reject
//   diff        = pAcceptBps - outcomeAcceptBps   (outcomeAcceptBps = 1M on ACCEPT, 0 on REJECT)
//   brierError  = diff² / 1_000_000
//   correctness = 1_000_000 − brierError
//
// Examples (ACCEPT outcome, 1M pool, 55% verifier pool = 550_000 uzrn):
//   confidence=900_000 accept: diff=-100_000, error=10_000,  correctness=990_000
//   confidence=600_000 accept: diff=-400_000, error=160_000, correctness=840_000
//   confidence=0 (neutral):   pAccept=500_000, diff=-500_000, error=250_000, correctness=750_000

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Unit tests for the pure brierCorrectnessBps function ─────────────────────

// TestBrierCorrectnessBps_NilReveal verifies that nil or zero-confidence
// reveals score at the neutral position (500_000 pAccept → correctness 750_000
// on ACCEPT outcome).
func TestBrierCorrectnessBps_NilReveal(t *testing.T) {
	// nil → pAccept=500_000, outcome=1_000_000 (ACCEPT)
	// diff = 500_000 - 1_000_000 = -500_000
	// error = 500_000² / 1_000_000 = 250_000
	// correctness = 750_000
	got := keeper.BrierCorrectnessBpsForTest(nil, 1_000_000)
	require.Equal(t, uint64(750_000), got, "nil reveal → neutral correctness")
}

// TestBrierCorrectnessBps_ZeroConfidence verifies that ConfidenceBps=0 maps
// to the same neutral score as nil.
func TestBrierCorrectnessBps_ZeroConfidence(t *testing.T) {
	rev := &types.RevealEntry{Verifier: "v1", Vote: "accept", ConfidenceBps: 0}
	got := keeper.BrierCorrectnessBpsForTest(rev, 1_000_000)
	require.Equal(t, uint64(750_000), got, "zero confidence → neutral (50%) correctness")
}

// TestBrierCorrectnessBps_PerfectAccept verifies that vote=accept with max
// confidence against an ACCEPT outcome scores 1_000_000.
func TestBrierCorrectnessBps_PerfectAccept(t *testing.T) {
	rev := &types.RevealEntry{Verifier: "v1", Vote: "accept", ConfidenceBps: 1_000_000}
	got := keeper.BrierCorrectnessBpsForTest(rev, 1_000_000)
	require.Equal(t, uint64(1_000_000), got, "perfect accept confidence → 1_000_000")
}

// TestBrierCorrectnessBps_PerfectReject verifies that vote=reject with max
// confidence against a REJECT outcome scores 1_000_000.
func TestBrierCorrectnessBps_PerfectReject(t *testing.T) {
	// REJECT outcome: outcomeAcceptBps=0, pAcceptBps = 1_000_000 - 1_000_000 = 0
	// diff = 0 - 0 = 0, error=0, correctness=1_000_000
	rev := &types.RevealEntry{Verifier: "v1", Vote: "reject", ConfidenceBps: 1_000_000}
	got := keeper.BrierCorrectnessBpsForTest(rev, 0)
	require.Equal(t, uint64(1_000_000), got, "perfect reject confidence → 1_000_000")
}

// TestBrierCorrectnessBps_HighConfidence verifies scoring for 90% confidence
// on a correct accept vote (outcome=ACCEPT).
func TestBrierCorrectnessBps_HighConfidence(t *testing.T) {
	// pAccept=900_000, outcome=1_000_000, diff=-100_000
	// error = 100_000² / 1_000_000 = 10_000
	// correctness = 990_000
	rev := &types.RevealEntry{Verifier: "v1", Vote: "accept", ConfidenceBps: 900_000}
	got := keeper.BrierCorrectnessBpsForTest(rev, 1_000_000)
	require.Equal(t, uint64(990_000), got)
}

// TestBrierCorrectnessBps_LowConfidence verifies scoring for 60% confidence
// on a correct accept vote (outcome=ACCEPT).
func TestBrierCorrectnessBps_LowConfidence(t *testing.T) {
	// pAccept=600_000, outcome=1_000_000, diff=-400_000
	// error = 400_000² / 1_000_000 = 160_000
	// correctness = 840_000
	rev := &types.RevealEntry{Verifier: "v1", Vote: "accept", ConfidenceBps: 600_000}
	got := keeper.BrierCorrectnessBpsForTest(rev, 1_000_000)
	require.Equal(t, uint64(840_000), got)
}

// ─── Integration tests: reward distribution with proper scoring ────────────────

// TestProperScoring_Disabled_EqualShares verifies backward compatibility:
// with ProperScoringRuleEnabled=false (default), verifiers with different
// confidence levels still receive equal shares.
func TestProperScoring_Disabled_EqualShares(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Default params: ProperScoringRuleEnabled = false
	addrA := makeValidBech32Addr("scoring-disabled-a")
	addrB := makeValidBech32Addr("scoring-disabled-b")

	claim := &types.Claim{
		Id:          "claim-scoring-disabled",
		FactContent: "Proper scoring disabled: should equal-share",
		Domain:      "mathematics",
		Submitter:   makeValidBech32Addr("submitter-sd"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-sd", "claim-scoring-disabled", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept", ConfidenceBps: 900_000}, // high confidence
		{Verifier: addrB, Vote: "accept", ConfidenceBps: 600_000}, // low confidence
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// pool = 55% of 1_000_000 = 550_000; equal split = 275_000 each
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, uint64(275_000), aTotal, "disabled: A gets equal share (275_000)")
	require.Equal(t, uint64(275_000), bTotal, "disabled: B gets equal share (275_000)")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation: total equals verifier pool")
}

// TestProperScoring_HighVsLow_UnequalShares verifies that with proper scoring
// enabled, a verifier with higher confidence receives a larger share.
func TestProperScoring_HighVsLow_UnequalShares(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("scoring-high-verifier")
	addrB := makeValidBech32Addr("scoring-low-verifier9")

	claim := &types.Claim{
		Id:          "claim-scoring-hl",
		FactContent: "High vs low confidence verifier payout test",
		Domain:      "physics",
		Submitter:   makeValidBech32Addr("submitter-hl"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-hl", "claim-scoring-hl", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept", ConfidenceBps: 900_000}, // correctness=990_000
		{Verifier: addrB, Vote: "accept", ConfidenceBps: 600_000}, // correctness=840_000
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// pool = 550_000
	// totalScore = 990_000 + 840_000 = 1_830_000
	// A share = safeMulDiv(550_000, 990_000, 1_830_000) = 297_540; +1 remainder = 297_541
	// B share = safeMulDiv(550_000, 840_000, 1_830_000) = 252_459
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, uint64(297_541), aTotal, "high confidence: gets larger share")
	require.Equal(t, uint64(252_459), bTotal, "low confidence: gets smaller share")
	require.Greater(t, aTotal, bTotal, "high confidence verifier earns more")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation: total equals verifier pool")
}

// TestProperScoring_EqualConfidence_EqualShares verifies that when all verifiers
// report the same confidence, proportional distribution collapses to equal shares.
func TestProperScoring_EqualConfidence_EqualShares(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("eq-conf-verifier-aa")
	addrB := makeValidBech32Addr("eq-conf-verifier-bb")

	claim := &types.Claim{
		Id:          "claim-eq-conf",
		FactContent: "Equal confidence verifiers: proportional collapses to equal",
		Domain:      "biology",
		Submitter:   makeValidBech32Addr("submitter-eq"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-eq", "claim-eq-conf", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept", ConfidenceBps: 800_000},
		{Verifier: addrB, Vote: "accept", ConfidenceBps: 800_000},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 800_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, aTotal, bTotal, "equal confidence → equal shares")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation")
}

// TestProperScoring_NoConfidence_NeutralScore verifies that a verifier with
// ConfidenceBps=0 receives the neutral (50%) score — equivalent to 500_000
// pAccept — and the distribution reflects that.
func TestProperScoring_NoConfidence_NeutralScore(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("no-conf-verifier-a1")
	addrB := makeValidBech32Addr("no-conf-verifier-b2")

	claim := &types.Claim{
		Id:          "claim-no-conf",
		FactContent: "One verifier with no confidence: neutral score",
		Domain:      "chemistry",
		Submitter:   makeValidBech32Addr("submitter-nc"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-nc", "claim-no-conf", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept", ConfidenceBps: 0},       // neutral (50%) → correctness=750_000
		{Verifier: addrB, Vote: "accept", ConfidenceBps: 500_000}, // explicit 50% → same score
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 750_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Both score 750_000 (ConfidenceBps=0 maps to 500_000 pAccept, same as explicit 500_000).
	// Equal scores → equal shares.
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, aTotal, bTotal, "zero confidence maps to neutral (50%) → equal shares")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation")
}

// TestProperScoring_RejectOutcome verifies proper scoring on a REJECT verdict.
// A verifier who voted reject with 80% confidence should score higher than
// one with 60% confidence.
func TestProperScoring_RejectOutcome(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("reject-hi-verifier1")
	addrB := makeValidBech32Addr("reject-lo-verifier2")

	claim := &types.Claim{
		Id:          "claim-reject-scoring",
		FactContent: "Proper scoring on reject outcome",
		Domain:      "logic",
		Submitter:   makeValidBech32Addr("submitter-rj"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-rj", "claim-reject-scoring", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	// Both voted reject; REJECT outcome
	// A: confidence=800_000 reject → pAccept=200_000, outcome=0, diff=200_000, error=40_000, correctness=960_000
	// B: confidence=600_000 reject → pAccept=400_000, outcome=0, diff=400_000, error=160_000, correctness=840_000
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "reject", ConfidenceBps: 800_000},
		{Verifier: addrB, Vote: "reject", ConfidenceBps: 600_000},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_REJECT,
		Confidence: 800_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Greater(t, aTotal, bTotal, "higher-confidence reject verifier earns more")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation")
}

// TestProperScoring_Malformed_FallsBackToEqual verifies that MALFORMED verdicts
// use equal-share distribution even when ProperScoringRuleEnabled=true.
func TestProperScoring_Malformed_FallsBackToEqual(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("malformed-verifier-a")
	addrB := makeValidBech32Addr("malformed-verifier-b")

	claim := &types.Claim{
		Id:          "claim-malformed-ps",
		FactContent: "Malformed claim proper scoring fallback test",
		Domain:      "linguistics",
		Submitter:   makeValidBech32Addr("submitter-mf"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-mf", "claim-malformed-ps", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "malformed", ConfidenceBps: 900_000},
		{Verifier: addrB, Vote: "malformed", ConfidenceBps: 600_000},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_MALFORMED,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA},
			{Verifier: addrB},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// MALFORMED → non-binary verdict → equal shares
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, aTotal, bTotal, "MALFORMED verdict: equal shares despite different confidence")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "conservation")
}

// TestProperScoring_SingleVerifier_GetsWholePool verifies the edge case where
// one correct verifier receives the entire pool regardless of their confidence.
func TestProperScoring_SingleVerifier_GetsWholePool(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.ProperScoringRuleEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	addrA := makeValidBech32Addr("single-verifier-xyz")

	claim := &types.Claim{
		Id:          "claim-single-verif",
		FactContent: "Single correct verifier receives full pool",
		Domain:      "mathematics",
		Submitter:   makeValidBech32Addr("submitter-sv"),
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-sv", "claim-single-verif", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.Reveals = []*types.RevealEntry{
		{Verifier: addrA, Vote: "accept", ConfidenceBps: 700_000},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 700_000,
		Rewards:    []keeper.VerifierReward{{Verifier: addrA}},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	aTotal := sumSentToAddr(bk, addrA)
	require.Equal(t, uint64(550_000), aTotal, "single verifier: receives full verifier pool")
}
