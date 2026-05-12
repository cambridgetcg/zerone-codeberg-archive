package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Store-level relation tests ──────────────────────────────────────────────

func TestGetFactRelations_Bidirectional(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Create two facts
	factA := makeTestFact(t, k, ctx, "fact-a", "Fact A", "physics", "empirical", makeSubmitterAddr(1), 900_000)
	factB := makeTestFact(t, k, ctx, "fact-b", "Fact B", "physics", "empirical", makeSubmitterAddr(1), 800_000)

	// Create a relation: A supports B
	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   factB.Id,
		Relation:       types.RelationType_RELATION_TYPE_SUPPORTS,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeSubmitterAddr(1),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	// Outgoing from A should return the relation
	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, factA.Id, outgoing[0].SourceFactId)
	require.Equal(t, factB.Id, outgoing[0].TargetFactId)
	require.Equal(t, types.RelationType_RELATION_TYPE_SUPPORTS, outgoing[0].Relation)

	// Incoming to B should return the same relation
	incoming, err := k.GetIncomingRelations(ctx, factB.Id)
	require.NoError(t, err)
	require.Len(t, incoming, 1)
	require.Equal(t, factA.Id, incoming[0].SourceFactId)
	require.Equal(t, factB.Id, incoming[0].TargetFactId)

	// No outgoing from B
	outB, err := k.GetFactRelations(ctx, factB.Id)
	require.NoError(t, err)
	require.Empty(t, outB)

	// No incoming to A
	inA, err := k.GetIncomingRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Empty(t, inA)
}

func TestGetRelationsByType(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	factA := makeTestFact(t, k, ctx, "fact-a", "Fact A", "physics", "empirical", makeSubmitterAddr(1), 900_000)
	factB := makeTestFact(t, k, ctx, "fact-b", "Fact B", "physics", "empirical", makeSubmitterAddr(1), 800_000)
	factC := makeTestFact(t, k, ctx, "fact-c", "Fact C", "physics", "empirical", makeSubmitterAddr(1), 700_000)

	// A supports B
	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factA.Id, TargetFactId: factB.Id,
		Relation: types.RelationType_RELATION_TYPE_SUPPORTS, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))
	// A requires C
	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factA.Id, TargetFactId: factC.Id,
		Relation: types.RelationType_RELATION_TYPE_REQUIRES, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))

	// Filter by SUPPORTS — should return only B
	supports, err := k.GetRelationsByType(ctx, factA.Id, types.RelationType_RELATION_TYPE_SUPPORTS)
	require.NoError(t, err)
	require.Len(t, supports, 1)
	require.Equal(t, factB.Id, supports[0].TargetFactId)

	// Filter by REQUIRES — should return only C
	requires, err := k.GetRelationsByType(ctx, factA.Id, types.RelationType_RELATION_TYPE_REQUIRES)
	require.NoError(t, err)
	require.Len(t, requires, 1)
	require.Equal(t, factC.Id, requires[0].TargetFactId)

	// Filter by CONTRADICTS — should return empty
	contradicts, err := k.GetRelationsByType(ctx, factA.Id, types.RelationType_RELATION_TYPE_CONTRADICTS)
	require.NoError(t, err)
	require.Empty(t, contradicts)
}

// ─── Message-level relation tests ────────────────────────────────────────────

func TestSubmitClaim_WithRelations(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	msgServer := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")
	bk.balances[submitter] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))

	// Create domain and target fact
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))
	targetFact := makeTestFact(t, k, ctx, "target-fact", "E=mc²", "physics", "empirical", submitter, 900_000)

	// Submit claim with a supports relation
	resp, err := msgServer.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Mass-energy equivalence confirmed experimentally",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Relations: []*types.ClaimRelation{
			{TargetFactId: targetFact.Id, Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
		},
	})
	require.NoError(t, err)

	// Verify claim has relations stored
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Len(t, claim.Relations, 1)
	require.Equal(t, targetFact.Id, claim.Relations[0].TargetFactId)
	require.Equal(t, types.RelationType_RELATION_TYPE_SUPPORTS, claim.Relations[0].Relation)
}

func TestSubmitClaim_InvalidRelationTarget(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	msgServer := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")
	bk.balances[submitter] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Submit claim referencing non-existent fact
	_, err := msgServer.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "This claim references a nonexistent fact for testing",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Relations: []*types.ClaimRelation{
			{TargetFactId: "nonexistent-fact", Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "relation target fact nonexistent-fact not found")
}

func TestSubmitClaim_UnspecifiedRelationType(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	msgServer := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")
	bk.balances[submitter] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))
	targetFact := makeTestFact(t, k, ctx, "target-fact", "E=mc²", "physics", "empirical", submitter, 900_000)

	// Submit claim with UNSPECIFIED relation type
	_, err := msgServer.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "This claim has an unspecified relation type for testing",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Relations: []*types.ClaimRelation{
			{TargetFactId: targetFact.Id, Relation: types.RelationType_RELATION_TYPE_UNSPECIFIED},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "relation type must be specified")
}

func TestSubmitClaim_ContradictionAutoContests(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	msgServer := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")
	bk.balances[submitter] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Create a verified target fact
	targetFact := makeTestFact(t, k, ctx, "target-fact", "E=mc²", "physics", "empirical", submitter, 900_000)
	require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, targetFact.Status)

	// Submit claim that CONTRADICTS the target
	_, err := msgServer.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Energy and mass are not equivalent",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Relations: []*types.ClaimRelation{
			{TargetFactId: targetFact.Id, Relation: types.RelationType_RELATION_TYPE_CONTRADICTS},
		},
	})
	require.NoError(t, err)

	// Target fact should now be CONTESTED
	updated, found := k.GetFact(ctx, targetFact.Id)
	require.True(t, found)
	require.Equal(t, types.FactStatus_FACT_STATUS_CONTESTED, updated.Status)
}

func TestCreateFactFromClaim_PropagatesRelations(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Create target facts
	targetA := makeTestFact(t, k, ctx, "target-a", "Target A", "physics", "empirical", makeSubmitterAddr(1), 900_000)
	targetB := makeTestFact(t, k, ctx, "target-b", "Target B", "physics", "empirical", makeSubmitterAddr(1), 800_000)

	// Create claim with relations
	height := uint64(ctx.BlockHeight())
	contentHash := keeper.ComputeClaimContentHash("New fact supporting A and requiring B", "physics")
	claimID := keeper.GenerateClaimID(makeSubmitterAddr(1), contentHash, height)

	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "New fact supporting A and requiring B",
		Domain:           "physics",
		Category:         "empirical",
		Submitter:        makeSubmitterAddr(1),
		SubmittedAtBlock: height,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000000",
		ContentHash:      contentHash,
		Relations: []*types.ClaimRelation{
			{TargetFactId: targetA.Id, Relation: types.RelationType_RELATION_TYPE_SUPPORTS},
			{TargetFactId: targetB.Id, Relation: types.RelationType_RELATION_TYPE_REQUIRES},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// Complete the round with ACCEPT verdict
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 850_000,
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Find the created fact
	claim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, claim.Status)

	// Get the fact ID from the claim
	var createdFact *types.Fact
	k.IterateFacts(ctx, func(f *types.Fact) bool {
		if f.ClaimId == claimID {
			createdFact = f
			return true
		}
		return false
	})
	require.NotNil(t, createdFact)

	// Verify relations were stored in the graph index
	outgoing, err := k.GetFactRelations(ctx, createdFact.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 2)

	// Verify incoming relations on targets
	incomingA, err := k.GetIncomingRelations(ctx, targetA.Id)
	require.NoError(t, err)
	require.Len(t, incomingA, 1)
	require.Equal(t, createdFact.Id, incomingA[0].SourceFactId)
	require.Equal(t, types.RelationType_RELATION_TYPE_SUPPORTS, incomingA[0].Relation)

	incomingB, err := k.GetIncomingRelations(ctx, targetB.Id)
	require.NoError(t, err)
	require.Len(t, incomingB, 1)
	require.Equal(t, createdFact.Id, incomingB[0].SourceFactId)
	require.Equal(t, types.RelationType_RELATION_TYPE_REQUIRES, incomingB[0].Relation)
}

// ─── Query-level relation tests ──────────────────────────────────────────────

func TestQueryFactRelations(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	queryServer := keeper.NewQueryServerImpl(k)

	factA := makeTestFact(t, k, ctx, "fact-a", "Fact A", "physics", "empirical", makeSubmitterAddr(1), 900_000)
	factB := makeTestFact(t, k, ctx, "fact-b", "Fact B", "physics", "empirical", makeSubmitterAddr(1), 800_000)
	factC := makeTestFact(t, k, ctx, "fact-c", "Fact C", "physics", "empirical", makeSubmitterAddr(1), 700_000)

	// A supports B, A contradicts C
	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factA.Id, TargetFactId: factB.Id,
		Relation: types.RelationType_RELATION_TYPE_SUPPORTS, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))
	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factA.Id, TargetFactId: factC.Id,
		Relation: types.RelationType_RELATION_TYPE_CONTRADICTS, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))

	// Query all relations for A (both directions, no type filter)
	resp, err := queryServer.FactRelations(ctx, &types.QueryFactRelationsRequest{
		FactId:    factA.Id,
		Direction: "both",
	})
	require.NoError(t, err)
	require.Len(t, resp.Relations, 2)

	// Query outgoing only
	resp, err = queryServer.FactRelations(ctx, &types.QueryFactRelationsRequest{
		FactId:    factA.Id,
		Direction: "outgoing",
	})
	require.NoError(t, err)
	require.Len(t, resp.Relations, 2)

	// Query incoming to B — should be 1 (from A)
	resp, err = queryServer.FactRelations(ctx, &types.QueryFactRelationsRequest{
		FactId:    factB.Id,
		Direction: "incoming",
	})
	require.NoError(t, err)
	require.Len(t, resp.Relations, 1)
	require.Equal(t, factA.Id, resp.Relations[0].SourceFactId)

	// Query with type filter — only SUPPORTS from A
	resp, err = queryServer.FactRelations(ctx, &types.QueryFactRelationsRequest{
		FactId:    factA.Id,
		Relation:  types.RelationType_RELATION_TYPE_SUPPORTS,
		Direction: "outgoing",
	})
	require.NoError(t, err)
	require.Len(t, resp.Relations, 1)
	require.Equal(t, factB.Id, resp.Relations[0].TargetFactId)

	// Query nonexistent fact
	_, err = queryServer.FactRelations(ctx, &types.QueryFactRelationsRequest{
		FactId: "nonexistent",
	})
	require.Error(t, err)
}

// ─── Graph traversal tests ───────────────────────────────────────────────────

func TestGraphTraversal_TwoHop(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Create chain: A requires B, B requires C
	factA := makeTestFact(t, k, ctx, "fact-a", "Theorem", "math", "derived", makeSubmitterAddr(1), 900_000)
	factB := makeTestFact(t, k, ctx, "fact-b", "Lemma", "math", "derived", makeSubmitterAddr(1), 800_000)
	factC := makeTestFact(t, k, ctx, "fact-c", "Axiom", "math", "axiomatic", makeSubmitterAddr(1), 1_000_000)

	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factA.Id, TargetFactId: factB.Id,
		Relation: types.RelationType_RELATION_TYPE_REQUIRES, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))
	require.NoError(t, k.SetFactRelation(ctx, &types.FactRelation{
		SourceFactId: factB.Id, TargetFactId: factC.Id,
		Relation: types.RelationType_RELATION_TYPE_REQUIRES, CreatedAtBlock: 100, Creator: makeSubmitterAddr(1),
	}))

	// From A, outgoing → B
	outA, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outA, 1)
	require.Equal(t, factB.Id, outA[0].TargetFactId)

	// From B, outgoing → C
	outB, err := k.GetFactRelations(ctx, outA[0].TargetFactId)
	require.NoError(t, err)
	require.Len(t, outB, 1)
	require.Equal(t, factC.Id, outB[0].TargetFactId)

	// Collect all facts reachable from A via requires (2-hop)
	reachable := map[string]bool{factA.Id: true}
	queue := []string{factA.Id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		rels, err := k.GetRelationsByType(ctx, current, types.RelationType_RELATION_TYPE_REQUIRES)
		require.NoError(t, err)
		for _, rel := range rels {
			if !reachable[rel.TargetFactId] {
				reachable[rel.TargetFactId] = true
				queue = append(queue, rel.TargetFactId)
			}
		}
	}

	// All three facts should be reachable
	require.True(t, reachable[factA.Id])
	require.True(t, reachable[factB.Id])
	require.True(t, reachable[factC.Id])
	require.Len(t, reachable, 3)
}
