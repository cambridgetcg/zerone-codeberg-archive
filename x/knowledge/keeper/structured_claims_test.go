package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── SubmitClaim with Structure ──────────────────────────────────────────────

func TestSubmitClaim_WithStructure(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Entropy of a closed system cannot decrease spontaneously",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Subject:       "entropy of a closed system",
			Predicate:     "cannot decrease spontaneously",
			Object:        "second law of thermodynamics",
			Scope:         "classical thermodynamics, isolated systems",
			TemporalScope: "",
			Negatable:     true,
			Tags:          []string{"thermodynamics", "entropy", "physics"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	// Verify structure is stored on claim
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.NotNil(t, claim.Structure)
	require.Equal(t, "entropy of a closed system", claim.Structure.Subject)
	require.Equal(t, "cannot decrease spontaneously", claim.Structure.Predicate)
	require.Equal(t, "second law of thermodynamics", claim.Structure.Object)
	require.Equal(t, "classical thermodynamics, isolated systems", claim.Structure.Scope)
	require.True(t, claim.Structure.Negatable)
	require.Equal(t, []string{"thermodynamics", "entropy", "physics"}, claim.Structure.Tags)
}

func TestSubmitClaim_WithoutStructure_StillWorks(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Water boils at 100 degrees Celsius at standard pressure",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Nil(t, claim.Structure) // No structure = nil
}

// ─── Structure Validation ────────────────────────────────────────────────────

func TestSubmitClaim_StructureValidation_MissingSubject(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has structure but missing subject field",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Predicate: "is true",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subject is required")
}

func TestSubmitClaim_StructureValidation_MissingPredicate(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has structure but missing predicate",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Subject: "entropy",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "predicate is required")
}

func TestSubmitClaim_StructureValidation_TooManyTags(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	tags := make([]string, 11)
	for i := range tags {
		tags[i] = "tag"
	}

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has way too many tags assigned",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "increases",
			Tags:      tags,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max 10 tags")
}

func TestSubmitClaim_StructureValidation_TagTooLong(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	longTag := make([]byte, 51)
	for i := range longTag {
		longTag[i] = 'x'
	}

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has a tag that is way too long",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "increases",
			Tags:      []string{string(longTag)},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tag too long")
}

// ─── Fact Propagation ────────────────────────────────────────────────────────

func TestCreateFactFromClaim_PropagatesStructure(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	structure := &types.ClaimStructure{
		Subject:   "entropy",
		Predicate: "cannot decrease in closed systems",
		Scope:     "thermodynamics",
		Negatable: true,
		Tags:      []string{"physics", "entropy"},
	}

	// Create a claim with structure
	contentHash := keeper.ComputeClaimContentHash("Entropy cannot decrease in closed systems", "physics")
	claimID := keeper.GenerateClaimID("zrn1sub", contentHash, uint64(ctx.BlockHeight()))

	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "Entropy cannot decrease in closed systems",
		Domain:           "physics",
		Category:         "empirical",
		Submitter:        "zrn1sub",
		SubmittedAtBlock: uint64(ctx.BlockHeight()),
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000000",
		ContentHash:      contentHash,
		ClaimType:        types.ClaimType_CLAIM_TYPE_ASSERTION,
		Structure:        structure,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// Complete the round with ACCEPT verdict
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 750_000,
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Find the created fact
	var createdFact *types.Fact
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == claimID {
			createdFact = fact
			return true
		}
		return false
	})
	require.NotNil(t, createdFact)
	require.NotNil(t, createdFact.Structure)
	require.Equal(t, "entropy", createdFact.Structure.Subject)
	require.Equal(t, "cannot decrease in closed systems", createdFact.Structure.Predicate)
	require.Equal(t, "thermodynamics", createdFact.Structure.Scope)
	require.True(t, createdFact.Structure.Negatable)
	require.Equal(t, []string{"physics", "entropy"}, createdFact.Structure.Tags)
}

// ─── Subject Index ───────────────────────────────────────────────────────────

func TestSubjectIndex_StoreAndRetrieve(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:      "subj-fact-1",
		Content: "Entropy cannot decrease",
		Domain:  "physics",
		Status:  types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "cannot decrease",
			Tags:      []string{"thermodynamics", "physics"},
		},
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.IndexFactBySubject(ctx, fact))

	// Retrieve by subject
	foundID := k.FindFactBySubjectPredicate(ctx, "physics", "entropy", "")
	require.Equal(t, "subj-fact-1", foundID)

	// Retrieve by subject+predicate
	foundID = k.FindFactBySubjectPredicate(ctx, "physics", "entropy", "cannot decrease")
	require.Equal(t, "subj-fact-1", foundID)

	// Non-matching predicate returns empty
	foundID = k.FindFactBySubjectPredicate(ctx, "physics", "entropy", "always increases")
	require.Empty(t, foundID)

	// Non-matching domain returns empty
	foundID = k.FindFactBySubjectPredicate(ctx, "mathematics", "entropy", "")
	require.Empty(t, foundID)
}

func TestSubjectIndex_Normalization(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:      "norm-fact-1",
		Content: "Entropy test normalization",
		Domain:  "physics",
		Status:  types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject:   "Entropy",
			Predicate: "test",
		},
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.IndexFactBySubject(ctx, fact))

	// Case-insensitive lookup
	require.Equal(t, "norm-fact-1", k.FindFactBySubjectPredicate(ctx, "physics", "entropy", ""))
	require.Equal(t, "norm-fact-1", k.FindFactBySubjectPredicate(ctx, "physics", "ENTROPY", ""))
	require.Equal(t, "norm-fact-1", k.FindFactBySubjectPredicate(ctx, "physics", " entropy ", ""))
}

// ─── Tag Index ───────────────────────────────────────────────────────────────

func TestFindFactsByTag(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact1 := &types.Fact{
		Id: "tag-fact-1", Content: "Fact one for tags", Domain: "physics",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject: "a", Predicate: "b",
			Tags: []string{"thermodynamics", "entropy"},
		},
	}
	fact2 := &types.Fact{
		Id: "tag-fact-2", Content: "Fact two for tags", Domain: "physics",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject: "c", Predicate: "d",
			Tags: []string{"thermodynamics", "heat"},
		},
	}
	fact3 := &types.Fact{
		Id: "tag-fact-3", Content: "Fact three no tags", Domain: "physics",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
	}

	require.NoError(t, k.SetFact(ctx, fact1))
	require.NoError(t, k.IndexFactBySubject(ctx, fact1))
	require.NoError(t, k.SetFact(ctx, fact2))
	require.NoError(t, k.IndexFactBySubject(ctx, fact2))
	require.NoError(t, k.SetFact(ctx, fact3))

	// Tag "thermodynamics" should return both
	ids, err := k.FindFactsByTag(ctx, "thermodynamics")
	require.NoError(t, err)
	require.Len(t, ids, 2)
	require.Contains(t, ids, "tag-fact-1")
	require.Contains(t, ids, "tag-fact-2")

	// Tag "entropy" should return only fact1
	ids, err = k.FindFactsByTag(ctx, "entropy")
	require.NoError(t, err)
	require.Len(t, ids, 1)
	require.Equal(t, "tag-fact-1", ids[0])

	// Tag "heat" should return only fact2
	ids, err = k.FindFactsByTag(ctx, "heat")
	require.NoError(t, err)
	require.Len(t, ids, 1)
	require.Equal(t, "tag-fact-2", ids[0])

	// Unknown tag should return empty
	ids, err = k.FindFactsByTag(ctx, "unknown")
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestFindFactsByTag_CaseInsensitive(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id: "tag-ci-1", Content: "Case insensitive tag", Domain: "physics",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject: "a", Predicate: "b",
			Tags: []string{"Physics"},
		},
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.IndexFactBySubject(ctx, fact))

	ids, err := k.FindFactsByTag(ctx, "physics")
	require.NoError(t, err)
	require.Len(t, ids, 1)

	ids, err = k.FindFactsByTag(ctx, "PHYSICS")
	require.NoError(t, err)
	require.Len(t, ids, 1)
}

// ─── Subject Dedup Warning ───────────────────────────────────────────────────

func TestSubjectDedup_EmitsWarning(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// First: create a fact with subject "entropy" in physics domain
	fact := &types.Fact{
		Id: "dedup-fact-1", Content: "Entropy always increases in closed systems",
		Domain: "physics", Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "always increases",
		},
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.IndexFactBySubject(ctx, fact))

	submitter := makeValidBech32Addr("submitter1")

	// Submit a new claim with the same subject+predicate
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Entropy always increases in an isolated system without exceptions",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "always increases",
			Tags:      []string{"thermodynamics"},
		},
	})
	// Should NOT reject — just emit warning
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	// Verify warning event was emitted
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	events := sdkCtx.EventManager().Events()
	var foundWarning bool
	for _, e := range events {
		if e.Type == "zerone.knowledge.duplicate_subject_warning" {
			foundWarning = true
			break
		}
	}
	require.True(t, foundWarning, "expected duplicate_subject_warning event to be emitted")
}

// ─── Query: FactsBySubject ───────────────────────────────────────────────────

func TestQuery_FactsBySubject(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	qs := keeper.NewQueryServerImpl(k)

	fact := &types.Fact{
		Id: "qsubj-1", Content: "Entropy fact for query", Domain: "physics",
		Category: "empirical", Confidence: 800_000,
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject:   "entropy",
			Predicate: "always increases",
		},
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.IndexFactBySubject(ctx, fact))

	resp, err := qs.FactsBySubject(ctx, &types.QueryFactsBySubjectRequest{
		Domain:  "physics",
		Subject: "entropy",
	})
	require.NoError(t, err)
	require.Len(t, resp.Facts, 1)
	require.Equal(t, "qsubj-1", resp.Facts[0].Id)
}

func TestQuery_FactsBySubject_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.FactsBySubject(ctx, &types.QueryFactsBySubjectRequest{
		Domain:  "physics",
		Subject: "nonexistent",
	})
	require.NoError(t, err)
	require.Empty(t, resp.Facts)
}

func TestQuery_FactsBySubject_MissingArgs(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.FactsBySubject(ctx, &types.QueryFactsBySubjectRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "domain is required")

	_, err = qs.FactsBySubject(ctx, &types.QueryFactsBySubjectRequest{Domain: "physics"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subject is required")
}

// ─── Query: FactsByTag ───────────────────────────────────────────────────────

func TestQuery_FactsByTag(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	qs := keeper.NewQueryServerImpl(k)

	fact1 := &types.Fact{
		Id: "qtag-1", Content: "Tagged fact one content", Domain: "physics",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject: "a", Predicate: "b",
			Tags: []string{"thermodynamics", "entropy"},
		},
	}
	fact2 := &types.Fact{
		Id: "qtag-2", Content: "Tagged fact two content", Domain: "chemistry",
		Status: types.FactStatus_FACT_STATUS_VERIFIED,
		Structure: &types.ClaimStructure{
			Subject: "c", Predicate: "d",
			Tags: []string{"thermodynamics"},
		},
	}
	require.NoError(t, k.SetFact(ctx, fact1))
	require.NoError(t, k.IndexFactBySubject(ctx, fact1))
	require.NoError(t, k.SetFact(ctx, fact2))
	require.NoError(t, k.IndexFactBySubject(ctx, fact2))

	resp, err := qs.FactsByTag(ctx, &types.QueryFactsByTagRequest{Tag: "thermodynamics"})
	require.NoError(t, err)
	require.Len(t, resp.Facts, 2)

	resp, err = qs.FactsByTag(ctx, &types.QueryFactsByTagRequest{Tag: "entropy"})
	require.NoError(t, err)
	require.Len(t, resp.Facts, 1)
	require.Equal(t, "qtag-1", resp.Facts[0].Id)
}

func TestQuery_FactsByTag_EmptyTag(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.FactsByTag(ctx, &types.QueryFactsByTagRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tag is required")
}
