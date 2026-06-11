package keeper_test

// Hegelian Dialectic Gap 3 — Progressive Enrichment.
//
// Hegel, Science of Logic: "the higher unity is the determinate unity, the
// unity that contains within itself as moments the very distinctions it
// supersedes." A Fact that has been the Hegelian successor of multiple
// rejected claims is epistemically richer than one that arrived at its
// determination directly: it carries the content of all those prior
// inadequate attempts.
//
// enrichment_count: increments each time a claim is sublated into this Fact.
// last_enriched_at_block: set to the block of the most recent sublation.
//
// Both fields are set in recordSublation (hegelian_sublation.go), alongside
// the existing sublated_claim_ids append.
//
// Tests:
//  1. FirstSublation_EnrichmentCountOne    — first sublation → count=1, block set
//  2. MultipleSublations_Accumulate        — three sublations → count=3, block updated
//  3. NoConflict_NoEnrichment              — REJECT with no ConflictingFactId → enrichment=0
//  4. AcceptVerdict_NoEnrichment           — ACCEPT verdict does not trigger enrichment
//  5. EnrichmentCountEqualsLen_SublatedIds — enrichment_count == len(sublated_claim_ids) invariant
//  6. LastEnrichedAtBlock_UpdatesEachTime  — block advances with each sublation

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// enrichFact injects an ACTIVE fact that can be enriched by sublation.
func enrichFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, suffix string) *types.Fact {
	t.Helper()
	return makeTestFact(t, k, ctx,
		"ef-fact-"+suffix,
		"enrichment target fact "+suffix,
		"physics",
		"empirical",
		makeValidBech32Addr("ef-creator-"+suffix),
		900_000,
	)
}

// enrichRound drives a single REJECT round against the fact, returning the
// updated claim and fact. Suffix must be unique per call.
func enrichRound(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	factID, suffix string,
	blockHeight int64,
) (*types.Claim, *types.Fact) {
	t.Helper()
	storedClaim, storedFact := completeRoundWithConflict(
		t, k, ctx.WithBlockHeight(blockHeight),
		"ef-"+suffix,
		factID,
	)
	return storedClaim, storedFact
}

// ── Tests ──────────────────────────────────────────────────────────────────────

// Test 1: first sublation → enrichment_count=1, last_enriched_at_block set.
func TestEnrichment_FirstSublation_CountOne(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := enrichFact(t, k, ctx, "first")
	_, storedFact := enrichRound(t, k, ctx, fact.Id, "first-r1", 500)

	require.NotNil(t, storedFact)
	require.Equal(t, uint64(1), storedFact.EnrichmentCount,
		"first sublation must set enrichment_count to 1")
	require.Equal(t, uint64(500), storedFact.LastEnrichedAtBlock,
		"last_enriched_at_block must equal the sublation block height")
}

// Test 2: three sublations → enrichment_count=3, last_enriched_at_block updated.
func TestEnrichment_MultipleSublations_Accumulate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := enrichFact(t, k, ctx, "multi")

	enrichRound(t, k, ctx, fact.Id, "multi-r1", 500)
	enrichRound(t, k, ctx, fact.Id, "multi-r2", 501)
	_, storedFact := enrichRound(t, k, ctx, fact.Id, "multi-r3", 502)

	require.NotNil(t, storedFact)
	require.Equal(t, uint64(3), storedFact.EnrichmentCount,
		"three sublations must accumulate enrichment_count to 3")
	require.Equal(t, uint64(502), storedFact.LastEnrichedAtBlock,
		"last_enriched_at_block must track the latest sublation block")
}

// Test 3: REJECT with no ConflictingFactId → enrichment fields stay zero.
func TestEnrichment_NoConflict_NoEnrichment(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Drive a REJECT with no conflicting fact.
	claimID := "ef-no-conflict"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "no conflict claim",
		Domain:           "physics",
		Category:         "empirical",
		Submitter:        makeValidBech32Addr("ef-sub-no-conflict"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// ConflictingFactId intentionally empty
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "ef-round-no-conflict",
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_REJECT,
		Confidence: 700_000,
	}
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), round, result))

	// No fact was involved — nothing to check enrichment on.
	// Confirm the claim has no sublation link (sanity).
	updated, found := k.GetClaim(ctx.WithBlockHeight(400), claimID)
	require.True(t, found)
	require.Empty(t, updated.SublationSuccessorFactId,
		"no conflicting fact → no sublation → no enrichment")
}

// Test 4: ACCEPT verdict → no enrichment (the fact IS the accepted claim; it's new).
func TestEnrichment_AcceptVerdict_NoEnrichment(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := enrichFact(t, k, ctx, "accept-no-enrich")

	claimID := "ef-accept-claim"
	claim := &types.Claim{
		Id:                claimID,
		FactContent:       "accepted claim — should not enrich the conflict fact",
		Domain:            "physics",
		Category:          "empirical",
		Submitter:         makeValidBech32Addr("ef-accept-sub"),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ConflictingFactId: fact.Id,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "ef-accept-round",
		ClaimId:             claimID,
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
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), round, result))

	// The conflict fact was not enriched — sublation only fires on REJECT.
	storedFact, found := k.GetFact(ctx.WithBlockHeight(400), fact.Id)
	require.True(t, found)
	require.Zero(t, storedFact.EnrichmentCount,
		"ACCEPT verdict must not enrich the conflicting fact")
	require.Zero(t, storedFact.LastEnrichedAtBlock)
}

// Test 5: enrichment_count == len(sublated_claim_ids) invariant holds.
func TestEnrichment_CountEqualsLenSublatedClaimIds(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := enrichFact(t, k, ctx, "invariant")

	_, f1 := enrichRound(t, k, ctx, fact.Id, "inv-r1", 500)
	_, f2 := enrichRound(t, k, ctx, fact.Id, "inv-r2", 501)
	_ = f1

	require.NotNil(t, f2)
	require.Equal(t, uint64(len(f2.SublatedClaimIds)), f2.EnrichmentCount,
		"enrichment_count must equal len(sublated_claim_ids) — both maintained in sync")
}

// Test 6: last_enriched_at_block updates each time, tracking the latest sublation.
func TestEnrichment_LastEnrichedAtBlock_UpdatesEachTime(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := enrichFact(t, k, ctx, "block-update")

	_, f1 := enrichRound(t, k, ctx, fact.Id, "bu-r1", 100)
	require.NotNil(t, f1)
	require.Equal(t, uint64(100), f1.LastEnrichedAtBlock)

	_, f2 := enrichRound(t, k, ctx, fact.Id, "bu-r2", 200)
	require.NotNil(t, f2)
	require.Equal(t, uint64(200), f2.LastEnrichedAtBlock,
		"last_enriched_at_block must update to the most recent sublation block")
}
