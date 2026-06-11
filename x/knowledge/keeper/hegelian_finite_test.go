package keeper_test

// Hegelian Dialectic Gap 4 — Finite Determination as Self-Negating.
//
// Hegel, Science of Logic: "Every finite determination is its own sublation —
// its very finitude ensures that it will pass beyond itself." No fact on-chain
// is ultimate; each carries within itself the seed of its supersession.
//
// When a challenge claim is ACCEPTED against a fact, the fact transitions to
// FACT_STATUS_DISPROVEN. This gap adds Fact.superseded_by_fact_id (field 84):
// the forward pointer to the more adequate determination that replaced it.
//
// Together with enrichment_count (Gap 3) and sublated_claim_ids (Gap 2), this
// builds the full Hegelian succession chain:
//
//   FactA (DISPROVEN) → superseded_by_fact_id → FactB (ACTIVE)
//
// Tests:
//  1. ChallengeAccepted_SupersededBySet    — challenge ACCEPT → forward pointer set
//  2. ChallengeRejected_NoSupersession     — challenge REJECT → original fact untouched
//  3. NoChallengeLink_NoSupersession       — plain ACCEPT (no ProvisionalFactId) → no effect
//  4. StatusDISPROVEN_WithSupersession     — status + superseded_by invariant hold together

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Tests ────────────────────────────────────────────────────────────────────

// Test 1: challenge ACCEPT → superseded_by_fact_id set to new fact ID.
func TestFinite_ChallengeAccepted_SupersededBySet(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	origFact := makeTestFact(t, k, ctx,
		"finite-orig-1", "original determination", "physics", "empirical",
		makeValidBech32Addr("finite-orig-1"), 900_000)

	claimID := "finite-ch-1"
	challenge := &types.Claim{
		Id:                claimID,
		FactContent:       "challenge: the original determination is inadequate",
		Domain:            "physics",
		Category:          "empirical",
		Submitter:         makeValidBech32Addr("finite-challenger-1"),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ProvisionalFactId: origFact.Id,
	}
	require.NoError(t, k.SetClaim(ctx, challenge))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("finite-round-1", claimID)))

	r, _ := k.GetVerificationRound(ctx, "finite-round-1")
	advCtx := ctx.WithBlockHeight(500)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 850_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updatedOrig, found := k.GetFact(advCtx, origFact.Id)
	require.True(t, found)
	require.Equal(t, types.FactStatus_FACT_STATUS_DISPROVEN, updatedOrig.Status,
		"original fact must be DISPROVEN after successful challenge")
	require.NotEmpty(t, updatedOrig.SupersededByFactId,
		"superseded_by_fact_id must be set to the new fact ID")

	// The new fact must exist and be accessible.
	newFact, foundNew := k.GetFact(advCtx, updatedOrig.SupersededByFactId)
	require.True(t, foundNew, "the superseding fact must exist in state")
	require.NotNil(t, newFact)
}

// Test 2: challenge REJECT → original fact untouched (no supersession).
func TestFinite_ChallengeRejected_NoSupersession(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	origFact := makeTestFact(t, k, ctx,
		"finite-orig-2", "original determination 2", "physics", "empirical",
		makeValidBech32Addr("finite-orig-2"), 900_000)

	claimID := "finite-ch-2"
	challenge := &types.Claim{
		Id:                claimID,
		FactContent:       "challenge that will fail",
		Domain:            "physics",
		Category:          "empirical",
		Submitter:         makeValidBech32Addr("finite-challenger-2"),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ProvisionalFactId: origFact.Id,
	}
	require.NoError(t, k.SetClaim(ctx, challenge))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("finite-round-2", claimID)))

	r, _ := k.GetVerificationRound(ctx, "finite-round-2")
	advCtx := ctx.WithBlockHeight(500)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updatedOrig, found := k.GetFact(advCtx, origFact.Id)
	require.True(t, found)
	require.NotEqual(t, types.FactStatus_FACT_STATUS_DISPROVEN, updatedOrig.Status,
		"original fact must NOT be disproven after failed challenge")
	require.Empty(t, updatedOrig.SupersededByFactId,
		"superseded_by_fact_id must be empty when challenge was REJECTED")
}

// Test 3: plain ACCEPT (no ProvisionalFactId) — handleChallengeDisproven is skipped.
func TestFinite_NoChallengeLink_NoSupersession(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Simple accept — not a challenge claim.
	claimID := "finite-plain-3"
	claim := &types.Claim{
		Id:          claimID,
		FactContent: "new claim, not a challenge",
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   makeValidBech32Addr("finite-plain-3"),
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:       "1000uzrn",
		// ProvisionalFactId intentionally empty
	}
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("finite-round-3", claimID)))

	r, _ := k.GetVerificationRound(ctx, "finite-round-3")
	advCtx := ctx.WithBlockHeight(500)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	// Accepted claim creates a new fact. Confirm the new fact is ACTIVE (not DISPROVEN).
	updatedClaim, found := k.GetClaim(advCtx, claimID)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, updatedClaim.Status)
	// No original fact was challenged — just verify no panic and claim accepted normally.
}

// Test 4: status + superseded_by invariant hold together after successful challenge.
func TestFinite_StatusDISPROVEN_WithSupersession(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	origFact := makeTestFact(t, k, ctx,
		"finite-orig-4", "original fact 4", "physics", "empirical",
		makeValidBech32Addr("finite-orig-4"), 800_000)

	claimID := "finite-ch-4"
	challenge := &types.Claim{
		Id:                claimID,
		FactContent:       "challenge 4 — successful",
		Domain:            "physics",
		Category:          "empirical",
		Submitter:         makeValidBech32Addr("finite-challenger-4"),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ProvisionalFactId: origFact.Id,
	}
	require.NoError(t, k.SetClaim(ctx, challenge))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("finite-round-4", claimID)))

	r, _ := k.GetVerificationRound(ctx, "finite-round-4")
	advCtx := ctx.WithBlockHeight(600)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 950_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updatedOrig, found := k.GetFact(advCtx, origFact.Id)
	require.True(t, found)
	// Both invariants hold together.
	require.Equal(t, types.FactStatus_FACT_STATUS_DISPROVEN, updatedOrig.Status)
	require.NotEmpty(t, updatedOrig.SupersededByFactId,
		"DISPROVEN fact must carry the superseded_by_fact_id forward pointer")
	require.NotEqual(t, origFact.Id, updatedOrig.SupersededByFactId,
		"superseded_by_fact_id must point to a different (newer) fact")
}
