package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Fact CRUD ───────────────────────────────────────────────────────────────

func TestFact_SetAndGet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:        "fact-001",
		Content:   "Water boils at 100C at standard pressure",
		Domain:    "physics",
		Category:  "empirical",
		Submitter: "zrn1submitter1",
		Status:    types.FactStatus_FACT_STATUS_VERIFIED,
	}
	require.NoError(t, k.SetFact(ctx, fact))

	got, found := k.GetFact(ctx, "fact-001")
	require.True(t, found)
	require.Equal(t, "fact-001", got.Id)
	require.Equal(t, "Water boils at 100C at standard pressure", got.Content)
	require.Equal(t, "physics", got.Domain)
	require.Equal(t, "empirical", got.Category)
	require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, got.Status)
}

func TestFact_GetNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	_, found := k.GetFact(ctx, "nonexistent")
	require.False(t, found)
}

func TestFact_Delete(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:        "fact-del",
		Content:   "Delete me",
		Domain:    "general",
		Submitter: "zrn1sub1",
	}
	require.NoError(t, k.SetFact(ctx, fact))

	_, found := k.GetFact(ctx, "fact-del")
	require.True(t, found)

	require.NoError(t, k.DeleteFact(ctx, "fact-del"))

	_, found = k.GetFact(ctx, "fact-del")
	require.False(t, found)
}

func TestFact_DeleteNonExistent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Deleting a fact that doesn't exist should not error
	require.NoError(t, k.DeleteFact(ctx, "nonexistent-fact"))
}

func TestFact_Update(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:         "fact-upd",
		Content:    "Original content",
		Domain:     "physics",
		Confidence: 500_000,
		Status:     types.FactStatus_FACT_STATUS_VERIFIED,
	}
	require.NoError(t, k.SetFact(ctx, fact))

	// Update confidence
	fact.Confidence = 800_000
	fact.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, fact))

	got, found := k.GetFact(ctx, "fact-upd")
	require.True(t, found)
	require.Equal(t, uint64(800_000), got.Confidence)
	require.Equal(t, types.FactStatus_FACT_STATUS_ACTIVE, got.Status)
}

func TestFact_Iterate(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	facts := []*types.Fact{
		{Id: "fact-a", Content: "A", Domain: "physics"},
		{Id: "fact-b", Content: "B", Domain: "mathematics"},
		{Id: "fact-c", Content: "C", Domain: "physics"},
	}
	for _, f := range facts {
		require.NoError(t, k.SetFact(ctx, f))
	}

	var collected []string
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		collected = append(collected, fact.Id)
		return false
	})
	require.Len(t, collected, 3)
}

func TestFact_IterateEarlyBreak(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, k.SetFact(ctx, &types.Fact{
			Id:      fmt.Sprintf("fact-%d", i),
			Content: fmt.Sprintf("Content %d", i),
			Domain:  "general",
		}))
	}

	var count int
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		count++
		return count >= 2 // stop after 2
	})
	require.Equal(t, 2, count)
}

func TestFact_IterateByDomain(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f1", Content: "A", Domain: "physics"}))
	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f2", Content: "B", Domain: "mathematics"}))
	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f3", Content: "C", Domain: "physics"}))

	var physicsFacts []string
	k.IterateFactsByDomain(ctx, "physics", func(factID string) bool {
		physicsFacts = append(physicsFacts, factID)
		return false
	})
	require.Len(t, physicsFacts, 2)
	require.Contains(t, physicsFacts, "f1")
	require.Contains(t, physicsFacts, "f3")
}

func TestFact_IterateBySubmitter(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f1", Content: "A", Submitter: "zrn1alice", Domain: "general"}))
	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f2", Content: "B", Submitter: "zrn1bob", Domain: "general"}))
	require.NoError(t, k.SetFact(ctx, &types.Fact{Id: "f3", Content: "C", Submitter: "zrn1alice", Domain: "general"}))

	var aliceFacts []string
	k.IterateFactsBySubmitter(ctx, "zrn1alice", func(factID string) bool {
		aliceFacts = append(aliceFacts, factID)
		return false
	})
	require.Len(t, aliceFacts, 2)
	require.Contains(t, aliceFacts, "f1")
	require.Contains(t, aliceFacts, "f3")
}

func TestFact_IterateByDomain_Empty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	var facts []string
	k.IterateFactsByDomain(ctx, "nonexistent_domain", func(factID string) bool {
		facts = append(facts, factID)
		return false
	})
	require.Empty(t, facts)
}

func TestFact_WithReferences(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ref := &types.Fact{Id: "ref-fact", Content: "Reference", Domain: "mathematics"}
	require.NoError(t, k.SetFact(ctx, ref))

	fact := &types.Fact{
		Id:         "citing-fact",
		Content:    "Cites reference",
		Domain:     "mathematics",
		References: []string{"ref-fact"},
	}
	require.NoError(t, k.SetFact(ctx, fact))

	got, found := k.GetFact(ctx, "citing-fact")
	require.True(t, found)
	require.Equal(t, []string{"ref-fact"}, got.References)
}

func TestFact_AllStatuses(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	statuses := []types.FactStatus{
		types.FactStatus_FACT_STATUS_PENDING,
		types.FactStatus_FACT_STATUS_PROVISIONAL,
		types.FactStatus_FACT_STATUS_VERIFIED,
		types.FactStatus_FACT_STATUS_ACTIVE,
		types.FactStatus_FACT_STATUS_CONTESTED,
		types.FactStatus_FACT_STATUS_CHALLENGED,
		types.FactStatus_FACT_STATUS_SUPERSEDED,
		types.FactStatus_FACT_STATUS_EXPIRED,
		types.FactStatus_FACT_STATUS_DISPROVEN,
		types.FactStatus_FACT_STATUS_REVOKED,
		types.FactStatus_FACT_STATUS_AT_RISK,
		types.FactStatus_FACT_STATUS_PRUNED,
	}

	for i, s := range statuses {
		id := fmt.Sprintf("fact-status-%d", i)
		require.NoError(t, k.SetFact(ctx, &types.Fact{
			Id:      id,
			Content: fmt.Sprintf("Status test %d", i),
			Domain:  "general",
			Status:  s,
		}))
		got, found := k.GetFact(ctx, id)
		require.True(t, found)
		require.Equal(t, s, got.Status)
	}
}

func TestFact_ConfidenceRange(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Test boundary values: 0 and 1,000,000
	for _, conf := range []uint64{0, 1, 500_000, 999_999, 1_000_000} {
		id := fmt.Sprintf("fact-conf-%d", conf)
		require.NoError(t, k.SetFact(ctx, &types.Fact{
			Id:         id,
			Content:    "Confidence test",
			Domain:     "general",
			Confidence: conf,
		}))
		got, found := k.GetFact(ctx, id)
		require.True(t, found)
		require.Equal(t, conf, got.Confidence)
	}
}

func TestFact_Patronage(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:                   "fact-patron",
		Content:              "Patronized fact",
		Domain:               "general",
		PatronageAmount:      "5000000",
		PatronageExpiryBlock: 1000,
	}
	require.NoError(t, k.SetFact(ctx, fact))

	got, found := k.GetFact(ctx, "fact-patron")
	require.True(t, found)
	require.Equal(t, "5000000", got.PatronageAmount)
	require.Equal(t, uint64(1000), got.PatronageExpiryBlock)
}

// ─── Content Hash / Dedup ────────────────────────────────────────────────────

func TestComputeClaimContentHash_Deterministic(t *testing.T) {
	h1 := keeper.ComputeClaimContentHash("Water boils at 100C", "physics")
	h2 := keeper.ComputeClaimContentHash("Water boils at 100C", "physics")
	require.Equal(t, h1, h2)
}

func TestComputeClaimContentHash_DomainSeparation(t *testing.T) {
	h1 := keeper.ComputeClaimContentHash("Water boils at 100C", "physics")
	h2 := keeper.ComputeClaimContentHash("Water boils at 100C", "chemistry")
	require.NotEqual(t, h1, h2)
}

func TestComputeClaimContentHash_ContentDifference(t *testing.T) {
	h1 := keeper.ComputeClaimContentHash("Water boils at 100C", "physics")
	h2 := keeper.ComputeClaimContentHash("Water freezes at 0C", "physics")
	require.NotEqual(t, h1, h2)
}

func TestClaimByContentHash_Dedup(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-dup",
		FactContent: "Unique content for dedup test",
		Domain:      "physics",
		ContentHash: "abc123hash",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	id, found := k.GetClaimByContentHash(ctx, "abc123hash")
	require.True(t, found)
	require.Equal(t, "claim-dup", id)
}

func TestClaimByContentHash_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	_, found := k.GetClaimByContentHash(ctx, "nonexistent")
	require.False(t, found)
}

// ─── ID Generation ───────────────────────────────────────────────────────────

func TestGenerateClaimID_Deterministic(t *testing.T) {
	id1 := keeper.GenerateClaimID("zrn1sub", "hash123", 100)
	id2 := keeper.GenerateClaimID("zrn1sub", "hash123", 100)
	require.Equal(t, id1, id2)
}

func TestGenerateClaimID_DifferentInputs(t *testing.T) {
	id1 := keeper.GenerateClaimID("zrn1sub", "hash123", 100)
	id2 := keeper.GenerateClaimID("zrn1sub", "hash456", 100)
	id3 := keeper.GenerateClaimID("zrn1sub", "hash123", 101)
	require.NotEqual(t, id1, id2)
	require.NotEqual(t, id1, id3)
}

func TestGenerateClaimID_Length(t *testing.T) {
	id := keeper.GenerateClaimID("zrn1sub", "hash123", 100)
	require.Len(t, id, 32, "claim ID must be 32 hex characters")
}

func TestGenerateFactID_Deterministic(t *testing.T) {
	id1 := keeper.GenerateFactID("claim-1", 200)
	id2 := keeper.GenerateFactID("claim-1", 200)
	require.Equal(t, id1, id2)
}

func TestGenerateFactID_Length(t *testing.T) {
	id := keeper.GenerateFactID("claim-1", 200)
	require.Len(t, id, 32)
}

func TestGenerateRoundID_Deterministic(t *testing.T) {
	id1 := keeper.GenerateRoundID("claim-1", 100)
	id2 := keeper.GenerateRoundID("claim-1", 100)
	require.Equal(t, id1, id2)
}

func TestGenerateRoundID_DifferentBlock(t *testing.T) {
	id1 := keeper.GenerateRoundID("claim-1", 100)
	id2 := keeper.GenerateRoundID("claim-1", 101)
	require.NotEqual(t, id1, id2)
}

// ─── Claim CRUD ──────────────────────────────────────────────────────────────

func TestClaim_SetAndGet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-001",
		FactContent: "Water boils at 100C at standard pressure",
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   "zrn1submitter1",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	got, found := k.GetClaim(ctx, "claim-001")
	require.True(t, found)
	require.Equal(t, "claim-001", got.Id)
	require.Equal(t, "physics", got.Domain)
	require.Equal(t, "1000000", got.Stake)
}

func TestClaim_GetNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	_, found := k.GetClaim(ctx, "nonexistent-claim")
	require.False(t, found)
}

func TestClaim_Delete(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	require.NoError(t, k.SetClaim(ctx, &types.Claim{Id: "claim-del", FactContent: "Delete me"}))
	_, found := k.GetClaim(ctx, "claim-del")
	require.True(t, found)

	require.NoError(t, k.DeleteClaim(ctx, "claim-del"))
	_, found = k.GetClaim(ctx, "claim-del")
	require.False(t, found)
}

func TestClaim_Iterate(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, k.SetClaim(ctx, &types.Claim{
			Id:          fmt.Sprintf("claim-%d", i),
			FactContent: fmt.Sprintf("Claim %d content for iteration", i),
		}))
	}

	var count int
	k.IterateClaims(ctx, func(claim *types.Claim) bool {
		count++
		return false
	})
	require.Equal(t, 5, count)
}

func TestClaim_AllStatuses(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	statuses := []types.ClaimStatus{
		types.ClaimStatus_CLAIM_STATUS_PENDING,
		types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		types.ClaimStatus_CLAIM_STATUS_ACCEPTED,
		types.ClaimStatus_CLAIM_STATUS_REJECTED,
		types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT,
		types.ClaimStatus_CLAIM_STATUS_CHALLENGED,
		types.ClaimStatus_CLAIM_STATUS_CONTESTED,
	}

	for i, s := range statuses {
		id := fmt.Sprintf("claim-status-%d", i)
		require.NoError(t, k.SetClaim(ctx, &types.Claim{
			Id:          id,
			FactContent: fmt.Sprintf("Status test %d with enough chars", i),
			Status:      s,
		}))
		got, found := k.GetClaim(ctx, id)
		require.True(t, found)
		require.Equal(t, s, got.Status)
	}
}

func TestClaim_RoundIndex(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-with-round",
		FactContent: "This claim will have a verification round",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub1",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// Look up round by claim ID
	got, found := k.GetRoundByClaimID(ctx, "claim-with-round")
	require.True(t, found)
	require.Equal(t, round.Id, got.Id)
}
