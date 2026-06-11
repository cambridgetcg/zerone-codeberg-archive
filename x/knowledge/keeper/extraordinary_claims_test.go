package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ────────────────────────────────────────────────────────────────────────────
// Skepticism Gap 3 — Extraordinary Claims Gate (Sagan's Maxim)
//
// "Extraordinary claims require extraordinary evidence."
// A claim that contradicts many established facts faces a higher panel
// requirement than one that is consistent with the existing corpus.
//
// Implementation:
//   - Params.extraordinary_claim_min_contradictions (field 190): threshold
//   - Params.extraordinary_claim_verifier_multiplier_bps (field 191): scale
//   - VerificationRound.extraordinary_min_verifiers (field 19): per-round gate
//
// Extraordinary gate: if CONTRADICTS-relation count ≥ threshold, the round's
// quorum requirement = min_verifiers × multiplier / 1_000_000.
// ────────────────────────────────────────────────────────────────────────────

// makeExtraordinaryParams sets up params with the extraordinary claims gate enabled.
func makeExtraordinaryParams(base types.Params, minContradictions, multiplierBps uint64) types.Params {
	base.ExtraordinaryClaimMinContradictions = minContradictions
	base.ExtraordinaryClaimVerifierMultiplierBps = multiplierBps
	return base
}

// TestExtraordinaryClaims_Disabled verifies that with min_contradictions=0
// (disabled), ordinary min_verifiers applies regardless of contradictions.
func TestExtraordinaryClaims_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := types.DefaultParams()
	p.MinVerifiers = 3
	p.ExtraordinaryClaimMinContradictions = 0 // disabled
	k.SetParams(ctx, &p)

	// Create claim with many contradictions
	claimID := "claim-disabled"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "disabled gate test claim",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-dis"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "fact-1", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "fact-2", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "fact-3", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// ExtraordinaryMinVerifiers must be 0 (disabled)
	require.Equal(t, uint64(0), round.ExtraordinaryMinVerifiers,
		"disabled gate must not set extraordinary_min_verifiers")
}

// TestExtraordinaryClaims_BelowThreshold verifies that a claim with fewer
// contradictions than the threshold uses standard min_verifiers.
func TestExtraordinaryClaims_BelowThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := makeExtraordinaryParams(types.DefaultParams(), 5 /* threshold */, 2_000_000 /* 2× */)
	p.MinVerifiers = 3
	k.SetParams(ctx, &p)

	// Only 2 CONTRADICTS — below threshold of 5
	claimID := "claim-below"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "below threshold claim test",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-below"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "fact-1", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "fact-2", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), round.ExtraordinaryMinVerifiers,
		"below-threshold claim must use standard quorum (extraordinary_min_verifiers=0)")
}

// TestExtraordinaryClaims_AtThreshold verifies that reaching the threshold
// triggers the extraordinary gate with the correct scaled quorum.
func TestExtraordinaryClaims_AtThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := makeExtraordinaryParams(types.DefaultParams(), 3 /* threshold */, 3_000_000 /* 3× */)
	p.MinVerifiers = 3
	k.SetParams(ctx, &p)

	// Exactly 3 CONTRADICTS — meets threshold
	claimID := "claim-at"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "at threshold claim test",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-at"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "fact-1", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "fact-2", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "fact-3", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// 3× min_verifiers = 3 × 3 = 9
	require.Equal(t, uint64(9), round.ExtraordinaryMinVerifiers,
		"at-threshold claim must require 3× the standard panel (3×3=9)")
}

// TestExtraordinaryClaims_AboveThreshold verifies that exceeding the threshold
// also triggers the extraordinary gate.
func TestExtraordinaryClaims_AboveThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := makeExtraordinaryParams(types.DefaultParams(), 2 /* threshold */, 2_000_000 /* 2× */)
	p.MinVerifiers = 3
	k.SetParams(ctx, &p)

	// 5 CONTRADICTS — well above threshold of 2
	claimID := "claim-above"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "above threshold claim test",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-above"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "f1", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "f2", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "f3", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "f4", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
			{TargetFactId: "f5", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// 2× min_verifiers = 2 × 3 = 6
	require.Equal(t, uint64(6), round.ExtraordinaryMinVerifiers,
		"above-threshold claim must require 2× the standard panel (2×3=6)")
}

// TestExtraordinaryClaims_NonContradictRelations verifies that non-CONTRADICTS
// relations (SUPPORTS, ELABORATES, etc.) do not count toward the threshold.
func TestExtraordinaryClaims_NonContradictRelations(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := makeExtraordinaryParams(types.DefaultParams(), 2, 2_000_000)
	p.MinVerifiers = 3
	k.SetParams(ctx, &p)

	// Many SUPPORTS but zero CONTRADICTS
	claimID := "claim-supports"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "non-contradict relations test",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-sup"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "f1", Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
			{TargetFactId: "f2", Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
			{TargetFactId: "f3", Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), round.ExtraordinaryMinVerifiers,
		"SUPPORTS relations must not count toward extraordinary gate")
}

// TestExtraordinaryClaims_QuorumEnforced verifies that AggregateVerificationResult
// returns INCONCLUSIVE when reveals < extraordinary_min_verifiers even if
// reveals ≥ global min_verifiers.
func TestExtraordinaryClaims_QuorumEnforced(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Global min_verifiers = 2, extraordinary threshold = 1 contradict, 3× multiplier
	// → extraordinary panel = 2 × 3 = 6. With only 2 reveals: INCONCLUSIVE.
	p := makeExtraordinaryParams(types.DefaultParams(), 1, 3_000_000)
	p.MinVerifiers = 2
	k.SetParams(ctx, &p)

	round := &types.VerificationRound{
		Id:                        "round-quorum",
		ClaimId:                   "claim-quorum",
		StartedAtBlock:            1,
		Phase:                     types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:            100,
		RevealDeadline:            200,
		AggregationDeadline:       300,
		ExtraordinaryMinVerifiers: 6, // set as-if this claim is extraordinary
		Reveals: []*types.RevealEntry{
			{Verifier: makeValidBech32Addr("v1"), Vote: "accept", ConfidenceBps: 800_000},
			{Verifier: makeValidBech32Addr("v2"), Vote: "accept", ConfidenceBps: 800_000},
		},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, result.Verdict,
		"2 reveals < extraordinary_min_verifiers(6) must produce INCONCLUSIVE verdict")
}

// TestExtraordinaryClaims_FloorAtMinVerifiers verifies that the extraordinary
// requirement never falls below the global min_verifiers floor.
func TestExtraordinaryClaims_FloorAtMinVerifiers(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Pathological config: multiplier < 1 (500_000 = 0.5×), but floor must apply
	// extraordinary_min_verifiers must be ≥ min_verifiers (5)
	p := makeExtraordinaryParams(types.DefaultParams(), 1 /* threshold */, 500_000 /* 0.5× */)
	p.MinVerifiers = 5
	k.SetParams(ctx, &p)

	claimID := "claim-floor"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "floor at min verifiers test",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-floor"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "f1", Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	// 0.5 × 5 = 2, but floor = min_verifiers = 5
	require.Equal(t, uint64(5), round.ExtraordinaryMinVerifiers,
		"extraordinary_min_verifiers must be floored at global min_verifiers")
}

// TestExtraordinaryClaims_AggregatePassesWithSufficientPanel verifies that
// an extraordinary claim with enough verifiers gets a normal ACCEPT verdict.
func TestExtraordinaryClaims_AggregatePassesWithSufficientPanel(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := makeExtraordinaryParams(types.DefaultParams(), 1, 2_000_000)
	p.MinVerifiers = 3
	k.SetParams(ctx, &p)

	// Extraordinary claim with extraordinary_min_verifiers = 6
	// Provide 6 reveals → should pass quorum and produce ACCEPT
	reveals := make([]*types.RevealEntry, 6)
	for i := range reveals {
		reveals[i] = &types.RevealEntry{
			Verifier:      makeValidBech32Addr("verifier" + string(rune('a'+i))),
			Vote:          "accept",
			ConfidenceBps: 900_000,
		}
	}

	round := &types.VerificationRound{
		Id:                        "round-pass",
		ClaimId:                   "claim-pass",
		StartedAtBlock:            1,
		Phase:                     types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:            100,
		RevealDeadline:            200,
		AggregationDeadline:       300,
		ExtraordinaryMinVerifiers: 6,
		Reveals:                   reveals,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// First set the claim (needed by CompleteRound for fact creation)
	require.NoError(t, k.SetClaim(ctx, &types.Claim{
		Id:               "claim-pass",
		FactContent:      "extraordinary claim sufficient panel",
		Domain:           "physics",
		Submitter:        makeValidBech32Addr("sub-pass"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
	}))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict,
		"6 reveals on extraordinary round with 6-verifier requirement must produce ACCEPT")
}
