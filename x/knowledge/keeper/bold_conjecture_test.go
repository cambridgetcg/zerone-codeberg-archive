package keeper_test

// Falsificationism Gap 4 — Bold Conjecture Score + Challenge Breadth.
//
// Popper, Section 4 (Degrees of Falsifiability): "A theory's empirical
// content is measured by how much it forbids." A claim that contradicts
// many established facts at submission is a "bold conjecture" — it takes
// real epistemic risk. A fact that has been challenged many times carries
// a richer corroboration history than one that has never been challenged.
//
// bold_conjecture_score on Claim:
//   Computed at SubmitClaim. Counts how many VERIFIED or ACTIVE facts the
//   new claim contradicts via RELATION_TYPE_CONTRADICTS. Zero if no such
//   targets. Non-zero triggers zerone.knowledge.bold_conjecture_submitted.
//
// challenge_breadth_count on Fact:
//   Incremented at SubmitClaim for each CONTRADICTS relation targeting the
//   fact (only when fact is VERIFIED or ACTIVE at submission time).
//
// Together with corroboration_severity_score_bps:
//   bold_conjecture_score:         how exposed the claim was at entry
//   challenge_breadth_count:       how many distinct challengers
//   corroboration_severity_score:  severity-weighted survival score
//
// Tests:
//   1. ZeroScoreNoRelations: no CONTRADICTS → bold_conjecture_score = 0
//   2. ZeroScoreContradictsPending: targeting PENDING fact → score = 0
//   3. ScoreOneVerifiedFact: contradicting 1 VERIFIED fact → score = 1
//   4. ScoreThreeVerifiedFacts: contradicting 3 VERIFIED facts → score = 3
//   5. ActiveFactCountsToo: ACTIVE fact targeted → score = 1
//   6. MixedStatuses: 2 VERIFIED + 1 PENDING → score = 2 (pending excluded)
//   7. ChallengeBreadthAccumulates: two claims challenge same fact → breadth = 2
//   8. BreadthOnlyForVerifiedActive: PENDING target → breadth unchanged
//   9. BoldEventEmitted: non-zero score → event with correct attrs
//  10. NonBoldNoEvent: zero score → no bold_conjecture_submitted event

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// makeActiveFact creates a Fact with FACT_STATUS_ACTIVE (not VERIFIED).
func makeActiveFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, id, content string) *types.Fact {
	t.Helper()
	fact := &types.Fact{
		Id:               id,
		Content:          content,
		Domain:           "physics",
		Category:         "mechanics",
		Confidence:       800_000,
		Submitter:        makeValidBech32Addr("active-fact-sub-" + id),
		SubmittedAtBlock: uint64(ctx.BlockHeight()),
		VerifiedAtBlock:  uint64(ctx.BlockHeight()),
		Status:           types.FactStatus_FACT_STATUS_ACTIVE,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return fact
}

// makePendingFact creates a Fact with FACT_STATUS_PENDING.
func makePendingFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, id, content string) *types.Fact {
	t.Helper()
	fact := &types.Fact{
		Id:               id,
		Content:          content,
		Domain:           "physics",
		Category:         "mechanics",
		Confidence:       500_000,
		Submitter:        makeValidBech32Addr("pending-fact-sub-" + id),
		SubmittedAtBlock: uint64(ctx.BlockHeight()),
		Status:           types.FactStatus_FACT_STATUS_PENDING,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return fact
}

// submitBoldClaim submits a claim via MsgServer with the given CONTRADICTS targets.
// Returns the claim ID on success.
func submitBoldClaim(
	t *testing.T,
	ms types.MsgServer,
	ctx sdk.Context,
	submitterSeed string,
	targetFactIDs []string,
) string {
	t.Helper()
	var rels []*types.ClaimRelation
	for _, id := range targetFactIDs {
		rels = append(rels, &types.ClaimRelation{
			TargetFactId: id,
			Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS,
		})
	}
	sub := makeValidBech32Addr(submitterSeed)
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   sub,
		FactContent: "bold claim by " + submitterSeed,
		Domain:      "physics",
		Category:    "mechanics",
		Stake:       "1000000",
		Relations:   rels,
	})
	require.NoError(t, err)
	return resp.ClaimId
}

// ─── Test 1: No Relations → score = 0 ────────────────────────────────────────

func TestBoldConjecture_ZeroScoreNoRelations(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("no-rel-sub"),
		FactContent: "claim with no relations",
		Domain:      "physics",
		Category:    "mechanics",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(0), stored.BoldConjectureScore,
		"claim with no CONTRADICTS must have bold_conjecture_score = 0")
}

// ─── Test 2: CONTRADICTS targeting PENDING fact → score = 0 ─────────────────

func TestBoldConjecture_ZeroScoreContradictsPending(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Create a PENDING fact — should not count toward bold score.
	pending := makePendingFact(t, k, ctx, "fact-pending-target", "pending knowledge")

	claimID := submitBoldClaim(t, ms, ctx, "pending-contra-sub", []string{pending.Id})

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(0), stored.BoldConjectureScore,
		"contradicting a PENDING fact must not increase bold_conjecture_score")

	// ChallengeBreadthCount on the PENDING fact must also remain 0.
	updatedPending, found := k.GetFact(ctx, pending.Id)
	require.True(t, found)
	require.Equal(t, uint64(0), updatedPending.ChallengeBreadthCount,
		"PENDING fact must not have challenge_breadth_count incremented")
}

// ─── Test 3: Contradicting 1 VERIFIED fact → score = 1 ──────────────────────

func TestBoldConjecture_ScoreOneVerifiedFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	verified := makeTestFact(t, k, ctx, "fact-v1", "Newton's first law", "physics", "mechanics",
		makeValidBech32Addr("fact-sub-v1"), 900_000)

	claimID := submitBoldClaim(t, ms, ctx, "bold-sub-1v", []string{verified.Id})

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(1), stored.BoldConjectureScore,
		"contradicting 1 VERIFIED fact must set bold_conjecture_score = 1")

	// ChallengeBreadthCount on the targeted fact must be 1.
	updatedFact, found := k.GetFact(ctx, verified.Id)
	require.True(t, found)
	require.Equal(t, uint64(1), updatedFact.ChallengeBreadthCount,
		"VERIFIED fact challenged once must have challenge_breadth_count = 1")
}

// ─── Test 4: Contradicting 3 VERIFIED facts → score = 3 ─────────────────────

func TestBoldConjecture_ScoreThreeVerifiedFacts(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	f1 := makeTestFact(t, k, ctx, "fact-3a", "law A", "physics", "mechanics", makeValidBech32Addr("sub-3a"), 900_000)
	f2 := makeTestFact(t, k, ctx, "fact-3b", "law B", "physics", "mechanics", makeValidBech32Addr("sub-3b"), 800_000)
	f3 := makeTestFact(t, k, ctx, "fact-3c", "law C", "physics", "mechanics", makeValidBech32Addr("sub-3c"), 700_000)

	claimID := submitBoldClaim(t, ms, ctx, "bold-sub-3v", []string{f1.Id, f2.Id, f3.Id})

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(3), stored.BoldConjectureScore,
		"contradicting 3 VERIFIED facts must set bold_conjecture_score = 3")

	// Each targeted fact must have breadth = 1.
	for _, id := range []string{f1.Id, f2.Id, f3.Id} {
		fact, found := k.GetFact(ctx, id)
		require.True(t, found)
		require.Equal(t, uint64(1), fact.ChallengeBreadthCount,
			"each challenged VERIFIED fact must have challenge_breadth_count = 1: "+id)
	}
}

// ─── Test 5: ACTIVE fact counts toward bold score ────────────────────────────

func TestBoldConjecture_ActiveFactCountsToo(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	active := makeActiveFact(t, k, ctx, "fact-active-bold", "active established law")

	claimID := submitBoldClaim(t, ms, ctx, "bold-sub-active", []string{active.Id})

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(1), stored.BoldConjectureScore,
		"contradicting an ACTIVE fact must count toward bold_conjecture_score")

	updatedFact, found := k.GetFact(ctx, active.Id)
	require.True(t, found)
	require.Equal(t, uint64(1), updatedFact.ChallengeBreadthCount,
		"ACTIVE fact challenged must have challenge_breadth_count = 1")
}

// ─── Test 6: Mixed statuses — only VERIFIED and ACTIVE count ─────────────────

func TestBoldConjecture_MixedStatusesOnlyVerifiedAndActive(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	verified := makeTestFact(t, k, ctx, "fact-mix-v", "verified law", "physics", "mechanics", makeValidBech32Addr("sub-mix-v"), 900_000)
	active := makeActiveFact(t, k, ctx, "fact-mix-a", "active law")
	pending := makePendingFact(t, k, ctx, "fact-mix-p", "pending hypothesis")

	claimID := submitBoldClaim(t, ms, ctx, "bold-sub-mix",
		[]string{verified.Id, active.Id, pending.Id})

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(2), stored.BoldConjectureScore,
		"only VERIFIED and ACTIVE targets count: expected score = 2")

	// VERIFIED and ACTIVE breadth incremented; PENDING unchanged.
	vFact, _ := k.GetFact(ctx, verified.Id)
	require.Equal(t, uint64(1), vFact.ChallengeBreadthCount, "verified fact breadth = 1")
	aFact, _ := k.GetFact(ctx, active.Id)
	require.Equal(t, uint64(1), aFact.ChallengeBreadthCount, "active fact breadth = 1")
	pFact, _ := k.GetFact(ctx, pending.Id)
	require.Equal(t, uint64(0), pFact.ChallengeBreadthCount, "pending fact breadth unchanged = 0")
}

// ─── Test 7: Two distinct challengers → breadth = 2 ─────────────────────────

func TestBoldConjecture_ChallengeBreadthAccumulatesAcrossSubmitters(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	target := makeTestFact(t, k, ctx, "fact-breadth-acc", "well-established law",
		"physics", "mechanics", makeValidBech32Addr("sub-breadth-orig"), 950_000)

	// First challenger.
	submitBoldClaim(t, ms, ctx, "breadth-challenger-1", []string{target.Id})
	fact, _ := k.GetFact(ctx, target.Id)
	require.Equal(t, uint64(1), fact.ChallengeBreadthCount, "after first challenge: breadth = 1")

	// Second challenger — different submitter, same target.
	submitBoldClaim(t, ms, ctx, "breadth-challenger-2", []string{target.Id})
	fact, _ = k.GetFact(ctx, target.Id)
	require.Equal(t, uint64(2), fact.ChallengeBreadthCount, "after second challenge: breadth = 2")
}

// ─── Test 8: PENDING target → breadth unchanged ──────────────────────────────

func TestBoldConjecture_BreadthNotIncrementedForPendingTarget(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	pending := makePendingFact(t, k, ctx, "fact-pend-breadth", "unverified hypothesis")

	submitBoldClaim(t, ms, ctx, "pend-breadth-sub", []string{pending.Id})

	fact, found := k.GetFact(ctx, pending.Id)
	require.True(t, found)
	require.Equal(t, uint64(0), fact.ChallengeBreadthCount,
		"PENDING target must not have challenge_breadth_count incremented")
}

// ─── Test 9: Bold event emitted when score > 0 ───────────────────────────────

func TestBoldConjecture_EventEmittedWhenBold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	verified := makeTestFact(t, k, ctx, "fact-event-v", "Newton's law", "physics", "mechanics",
		makeValidBech32Addr("sub-event-v"), 900_000)

	claimID := submitBoldClaim(t, ms, sdkCtx, "bold-event-sub", []string{verified.Id})

	// Find the bold_conjecture_submitted event in the context events.
	events := sdkCtx.EventManager().Events()
	var found bool
	for _, ev := range events {
		if ev.Type == "zerone.knowledge.bold_conjecture_submitted" {
			found = true
			attrs := make(map[string]string)
			for _, attr := range ev.Attributes {
				attrs[attr.Key] = attr.Value
			}
			require.Equal(t, claimID, attrs["claim_id"],
				"bold event must carry the claim_id")
			require.Equal(t, "1", attrs["bold_conjecture_score"],
				"bold event must carry bold_conjecture_score = 1")
			require.NotEmpty(t, attrs["submitter"],
				"bold event must carry submitter address")
			break
		}
	}
	require.True(t, found, "zerone.knowledge.bold_conjecture_submitted event must be emitted")
}

// ─── Test 10: Non-bold claim → no bold event emitted ─────────────────────────

func TestBoldConjecture_NoBoldEventWhenScoreZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// No CONTRADICTS relations.
	_, err := ms.SubmitClaim(sdkCtx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("non-bold-sub"),
		FactContent: "timid claim that contradicts nothing",
		Domain:      "physics",
		Category:    "mechanics",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	// Confirm no bold_conjecture_submitted event.
	for _, ev := range sdkCtx.EventManager().Events() {
		require.NotEqual(t, "zerone.knowledge.bold_conjecture_submitted", ev.Type,
			"no bold event must be emitted when bold_conjecture_score = 0")
	}
}
