package keeper_test

// Scientific Method Gap 4 — Theory-Ladenness: theoretical_framework field.
//
// Kuhn's observation: all observation is theory-laden. The chain should record
// which theory is doing the loading. Claim.theoretical_framework (field 29) and
// Fact.theoretical_framework (field 75) carry the paradigm declaration through
// the PoT lifecycle.
//
// Tests:
//   1. StoredOnClaim: theoretical_framework persists via SetClaim/GetClaim
//   2. EmptyIsBackwardCompat: empty framework is valid (optional field)
//   3. ClaimToFact_Propagates: accepted claim's framework copied to Fact on ACCEPT
//   4. ClaimToFact_EmptyFramework: empty framework propagates as empty (not error)

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: StoredOnClaim ────────────────────────────────────────────────────

func TestTheoreticalFramework_StoredOnClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub-theory")
	claim := &types.Claim{
		Id:                   "claim-with-framework",
		FactContent:          "vaccines protect against measles",
		Domain:               "biology",
		Category:             "medicine",
		Submitter:            sub,
		SubmittedAtBlock:     1,
		Status:               types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:                "1000uzrn",
		TheoreticalFramework: "germ theory of disease",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-with-framework")
	require.True(t, found)
	require.Equal(t, "germ theory of disease", stored.TheoreticalFramework)
}

// ── Test 2: EmptyIsBackwardCompat ────────────────────────────────────────────

func TestTheoreticalFramework_EmptyIsBackwardCompat(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub-no-theory")
	claim := &types.Claim{
		Id:               "claim-no-framework",
		FactContent:      "water is H2O",
		Domain:           "chemistry",
		Category:         "composition",
		Submitter:        sub,
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// TheoreticalFramework intentionally empty
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-no-framework")
	require.True(t, found)
	require.Equal(t, "", stored.TheoreticalFramework, "empty framework should be empty, not error")
}

// ── Test 3: ClaimToFact_Propagates ──────────────────────────────────────────

func TestTheoreticalFramework_ClaimToFactPropagates(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub-claim-fact")
	claim := &types.Claim{
		Id:                   "claim-evolve",
		FactContent:          "natural selection drives speciation",
		Domain:               "biology",
		Category:             "evolution",
		Submitter:            sub,
		SubmittedAtBlock:     1,
		Status:               types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:                "1000uzrn",
		TheoreticalFramework: "Darwinian evolutionary theory",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-evolve",
		ClaimId:             "claim-evolve",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 800_000,
	}

	advancedCtx := ctx.WithBlockHeight(400)
	err := k.CompleteRound(advancedCtx, round, result)
	require.NoError(t, err)

	// Find the accepted fact and verify theoretical_framework propagated.
	var factFound bool
	k.IterateFacts(advancedCtx, func(f *types.Fact) bool {
		if f.Content == "natural selection drives speciation" {
			require.Equal(t, "Darwinian evolutionary theory", f.TheoreticalFramework,
				"theoretical_framework must propagate from claim to fact on ACCEPT")
			factFound = true
			return true
		}
		return false
	})
	require.True(t, factFound, "accepted fact should exist")
}

// ── Test 4: ClaimToFact_EmptyFramework ──────────────────────────────────────

func TestTheoreticalFramework_EmptyFrameworkPropagates(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub-empty-fw")
	claim := &types.Claim{
		Id:               "claim-helio",
		FactContent:      "the earth orbits the sun",
		Domain:           "cosmology",
		Category:         "astronomy",
		Submitter:        sub,
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// TheoreticalFramework intentionally empty
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-helio",
		ClaimId:             "claim-helio",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 800_000,
	}

	advancedCtx := ctx.WithBlockHeight(400)
	err := k.CompleteRound(advancedCtx, round, result)
	require.NoError(t, err)

	var factFound bool
	k.IterateFacts(advancedCtx, func(f *types.Fact) bool {
		if f.Content == "the earth orbits the sun" {
			require.Equal(t, "", f.TheoreticalFramework,
				"empty framework should propagate as empty, not cause errors")
			factFound = true
			return true
		}
		return false
	})
	require.True(t, factFound, "accepted fact should exist")
}
