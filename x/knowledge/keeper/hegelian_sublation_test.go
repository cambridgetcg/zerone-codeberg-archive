package keeper_test

// Hegelian Dialectic Gap 2 — Aufhebung (Sublation).
//
// Hegel (Science of Logic): "That which is sublated is thus something at the
// same time preserved, something that has lost its immediacy but has not come
// to nothing for that."
//
// When a claim is REJECTED and an accepted Fact stands in contradiction,
// that Fact is the higher-order determination. The chain records:
//   - Claim.sublation_successor_fact_id → the Fact that achieved the Aufhebung
//   - Fact.sublated_claim_ids           → inverse: which rejected claims it sublated
//
// Detection priority:
//   1. Claim.ConflictingFactId (canonical-form opposition from Formal Logic Gap 3)
//   2. Claim.SelfContradictionFactId (submitter-scope Socratic contradiction)
//   3. Claim.Relations with CONTRADICTS → accepted Fact
//
// Unit tests (FindSublationSuccessorForTest):
//   1. NoConflict → "" (no sublation, no CONTRADICTS relation)
//   2. ConflictingFactId_Accepted → returns conflicting fact ID (priority 1)
//   3. ConflictingFactId_NotAccepted → skips (fact is PENDING/REJECTED)
//   4. SelfContradiction_Accepted → returns self_contradiction_fact_id (priority 2)
//   5. Relations_CONTRADICTS_Accepted → returns via Relations scan (priority 3)
//   6. Relations_Non-CONTRADICTS → ignored
//   7. Priority: ConflictingFactId wins over Relations (priority 1 > 3)
//
// Integration tests (CompleteRound REJECT path):
//   8. VerdictReject_SublationStored — conflicting_fact_id set → sublation link recorded
//   9. VerdictAccept_NoSublation — ACCEPT verdict → no sublation link
//  10. ReverseLink_SublatedClaimIds — Fact.sublated_claim_ids updated on sublation
//  11. VerdictReject_NoConflict → no sublation (no contradicting accepted Fact)
//  12. MultipleRejections_Accumulate — multiple rejected claims → Fact accumulates sublated list

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── unit helpers ─────────────────────────────────────────────────────────────

// makeSublationFact injects an ACTIVE (accepted) fact for sublation tests.
func makeSublationFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, suffix string) *types.Fact {
	t.Helper()
	return makeTestFact(t, k, ctx,
		"suf-fact-"+suffix,
		"sublation target fact "+suffix,
		"physics",
		"test",
		makeValidBech32Addr("sub-creator-"+suffix),
		800_000,
	)
}

// ── Unit tests (FindSublationSuccessorForTest) ─────────────────────────────────

// Test 1: no conflict anywhere → ""
func TestSublation_NoConflict(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "stest-no-conflict",
		FactContent: "no conflict claim",
		Domain:      "physics",
		Status:      types.ClaimStatus_CLAIM_STATUS_REJECTED,
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Empty(t, result, "no conflicting fact → no sublation successor")
}

// Test 2: ConflictingFactId points to ACTIVE fact → returns that ID.
func TestSublation_ConflictingFactId_Accepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	fact := makeSublationFact(t, k, ctx, "conf-accepted")

	claim := &types.Claim{
		Id:                "stest-conf-accepted",
		FactContent:       "conflicting claim",
		Domain:            "physics",
		ConflictingFactId: fact.Id,
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Equal(t, fact.Id, result, "ConflictingFactId pointing to accepted Fact must return that Fact")
}

// Test 3: ConflictingFactId points to a fact that is not accepted (PENDING) → skip.
func TestSublation_ConflictingFactId_NotAccepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Inject a CONTESTED (non-active) fact.
	pendingFact := &types.Fact{
		Id:       "stest-pending-fact",
		Content:  "pending fact",
		Domain:   "physics",
		Category: "test",
		Status:   types.FactStatus_FACT_STATUS_CONTESTED,
	}
	require.NoError(t, k.SetFact(ctx, pendingFact))

	claim := &types.Claim{
		Id:                "stest-conf-not-accepted",
		ConflictingFactId: pendingFact.Id,
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Empty(t, result, "non-accepted ConflictingFactId must be skipped")
}

// Test 4: SelfContradictionFactId → priority 2 fires.
func TestSublation_SelfContradiction_Accepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	fact := makeSublationFact(t, k, ctx, "self-accepted")

	claim := &types.Claim{
		Id:                       "stest-self-accepted",
		SelfContradictionFactId:  fact.Id,
		IsSelfContradictory:      true,
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Equal(t, fact.Id, result, "SelfContradictionFactId pointing to accepted Fact must return that Fact")
}

// Test 5: Relations CONTRADICTS → accepted Fact → priority 3 fires.
func TestSublation_Relations_CONTRADICTS_Accepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	fact := makeSublationFact(t, k, ctx, "rel-accepted")

	claim := &types.Claim{
		Id:          "stest-rel-accepted",
		FactContent: "claim with explicit CONTRADICTS relation",
		Domain:      "physics",
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: fact.Id,
				Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS,
			},
		},
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Equal(t, fact.Id, result, "CONTRADICTS relation to accepted Fact must return that Fact")
}

// Test 6: Relations with non-CONTRADICTS relation → not a sublation target.
func TestSublation_Relations_NonContradicts_Ignored(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	fact := makeSublationFact(t, k, ctx, "non-contr")

	claim := &types.Claim{
		Id: "stest-non-contradicts",
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: fact.Id,
				Relation:     types.RelationType_RELATION_TYPE_SUPPORTS, // not CONTRADICTS
			},
		},
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Empty(t, result, "SUPPORTS relation must not trigger sublation")
}

// Test 7: ConflictingFactId takes priority over Relations scan.
func TestSublation_Priority_ConflictingFactId_Wins(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	highPriorityFact := makeSublationFact(t, k, ctx, "hipri")
	lowPriorityFact := makeSublationFact(t, k, ctx, "lopri")

	claim := &types.Claim{
		Id:                "stest-priority",
		ConflictingFactId: highPriorityFact.Id, // priority 1
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: lowPriorityFact.Id,
				Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS, // priority 3
			},
		},
	}
	result := k.FindSublationSuccessorForTest(ctx, claim)
	require.Equal(t, highPriorityFact.Id, result, "ConflictingFactId (priority 1) must beat Relations scan (priority 3)")
}

// ── Integration tests (CompleteRound REJECT path) ─────────────────────────────

// completeRoundWithConflict injects claim+round and drives CompleteRound with
// REJECT verdict, then returns the stored claim and successor fact.
func completeRoundWithConflict(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	suffix string,
	conflictingFactID string,
) (*types.Claim, *types.Fact) {
	t.Helper()

	claimID := "sf-claim-" + suffix
	roundID := "sf-round-" + suffix

	claim := &types.Claim{
		Id:                claimID,
		FactContent:       "sublation integration test: " + suffix,
		Domain:            "physics",
		Category:          "test",
		Submitter:         makeValidBech32Addr("sf-sub-" + suffix),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ConflictingFactId: conflictingFactID,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  roundID,
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
		Confidence: 800_000,
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Fetch updated claim.
	storedClaim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)

	// Fetch the conflicting fact (may have been updated with reverse link).
	var storedFact *types.Fact
	if conflictingFactID != "" {
		f, ok := k.GetFact(ctx, conflictingFactID)
		if ok {
			storedFact = f
		}
	}
	return storedClaim, storedFact
}

// Test 8: REJECT verdict with ConflictingFactId → sublation link stored on claim.
func TestSublation_VerdictReject_SublationStored(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := makeSublationFact(t, k, ctx, "int-stored")
	storedClaim, _ := completeRoundWithConflict(t, k, ctx, "int-stored", fact.Id)

	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_REJECTED, storedClaim.Status)
	require.Equal(t, fact.Id, storedClaim.SublationSuccessorFactId,
		"REJECT with ConflictingFactId must set sublation_successor_fact_id")
	require.NotZero(t, storedClaim.SublationAtBlock,
		"sublation_at_block must be set when sublation is recorded")
}

// Test 9: ACCEPT verdict → no sublation link.
func TestSublation_VerdictAccept_NoSublation(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := makeSublationFact(t, k, ctx, "accept-no-sub")

	claimID := "sf-accept-claim"
	claim := &types.Claim{
		Id:                claimID,
		FactContent:       "accepted claim — should not be sublated",
		Domain:            "physics",
		Category:          "test",
		Submitter:         makeValidBech32Addr("sf-accept-sub"),
		SubmittedAtBlock:  1,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ConflictingFactId: fact.Id,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "sf-accept-round",
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
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Sublation link must NOT be set for accepted claims.
	storedClaim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Empty(t, storedClaim.SublationSuccessorFactId,
		"ACCEPT verdict must not set sublation_successor_fact_id")
	require.Zero(t, storedClaim.SublationAtBlock)
}

// Test 10: Reverse link — Fact.sublated_claim_ids updated on sublation.
func TestSublation_ReverseLink_SublatedClaimIds(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := makeSublationFact(t, k, ctx, "reverse-link")
	storedClaim, storedFact := completeRoundWithConflict(t, k, ctx, "reverse-link", fact.Id)

	require.Equal(t, fact.Id, storedClaim.SublationSuccessorFactId)
	require.NotNil(t, storedFact)
	require.Contains(t, storedFact.SublatedClaimIds, storedClaim.Id,
		"Fact.sublated_claim_ids must contain the rejected claim's ID")
}

// Test 11: REJECT with no conflict → no sublation.
func TestSublation_VerdictReject_NoConflict(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	storedClaim, _ := completeRoundWithConflict(t, k, ctx, "no-conflict", "")

	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_REJECTED, storedClaim.Status)
	require.Empty(t, storedClaim.SublationSuccessorFactId,
		"REJECT with no conflicting fact → no sublation")
	require.Zero(t, storedClaim.SublationAtBlock)
}

// Test 12: Multiple rejections accumulate sublated_claim_ids on the same Fact.
func TestSublation_MultipleRejections_Accumulate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := makeSublationFact(t, k, ctx, "multi-accum")

	// Reject two different claims, both with the same ConflictingFactId.
	storedClaim1, _ := completeRoundWithConflict(t, k, ctx, "multi-1", fact.Id)
	storedClaim2, storedFact := completeRoundWithConflict(t, k, ctx, "multi-2", fact.Id)

	require.Equal(t, fact.Id, storedClaim1.SublationSuccessorFactId)
	require.Equal(t, fact.Id, storedClaim2.SublationSuccessorFactId)

	require.NotNil(t, storedFact)
	require.Len(t, storedFact.SublatedClaimIds, 2,
		"Fact must accumulate all sublated claim IDs without overwriting")
	require.Contains(t, storedFact.SublatedClaimIds, storedClaim1.Id)
	require.Contains(t, storedFact.SublatedClaimIds, storedClaim2.Id)
}
