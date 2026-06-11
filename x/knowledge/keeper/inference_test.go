package keeper_test

// Formal Logic Brief Gap 4 — Inference Rules for Canonical Form Language.
//
// The canonical form language has four claim types: assert, relate, define, constrain.
// Accepted facts are the corpus; inference rules extend the corpus with what follows
// logically without requiring a new PoT round.
//
// Two rules implemented here:
//   Transitivity:            relate(D, A, r, B) + relate(D, B, r, C) where r is
//                            in Params.TransitiveRelationVerbs → relate(D, A, r, C).
//   Definition substitution: define(D, term, meaning) + assert(D, term, pred)
//                            → assert(D, meaning, pred).
//
// Off-chain engines call QueryDerivedConsequences, then submit derived facts as
// M-COMPUTATIONAL claims citing the source fact IDs via ClaimRelation.
//
// Tests:
//  1.  Transitivity_Direct             — A→B + B→C using "is-a" → A→C
//  2.  Transitivity_NonTransitiveVerb  — "loves" not in transitive list → empty
//  3.  Transitivity_MismatchedDomains  — A→B in domain1, B→C in domain2 → empty
//  4.  Transitivity_EmptyVerbsList     — disables transitivity for all verbs
//  5.  Transitivity_NoPair             — A→B with no B→C → empty
//  6.  Transitivity_NonAcceptedPremise — second premise not accepted → empty
//  7.  Transitivity_SourceNotAccepted  — source not accepted → empty
//  8.  Transitivity_ChainImplies       — "implies" is transitive, A implies B+C → A implies C
//  9.  DefinitionSubstitution_Basic    — define(cat,feline)+assert(cat,meows)→assert(feline,meows)
//  10. DefinitionSubstitution_NoMatch  — no assertion uses the defined term → empty
//  11. DefinitionSubstitution_Multiple — two assertions → two derived consequences
//  12. Params_ValidateEmptyVerb        — empty string in verbs list is rejected
//  13. Params_DefaultVerbs             — DefaultParams has expected transitive verbs

import (
	"testing"

	"github.com/stretchr/testify/require"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

// setTransitiveVerbsInf updates Params.TransitiveRelationVerbs.
func setTransitiveVerbsInf(k keeper.Keeper, ctx sdk.Context, verbs []string) {
	p := types.DefaultParams()
	p.TransitiveRelationVerbs = verbs
	if err := k.SetParams(ctx, &p); err != nil {
		panic(err)
	}
}

func makeRelationFact(k keeper.Keeper, ctx sdk.Context, id, domain, subject, verb, object string) *types.Fact {
	f := &types.Fact{
		Id:        id,
		Content:   subject + " " + verb + " " + object,
		Domain:    domain,
		Category:  "derived",
		Status:    types.FactStatus_FACT_STATUS_VERIFIED,
		ClaimType: types.ClaimType_CLAIM_TYPE_RELATION,
		Structure: &types.ClaimStructure{
			Subject:   subject,
			Predicate: verb,
			Object:    object,
		},
	}
	f.CanonicalForm = types.BuildCanonicalForm(f.ClaimType, f.Structure, domain)
	if err := k.SetFact(ctx, f); err != nil {
		panic(err)
	}
	return f
}

func makeAssertFact(k keeper.Keeper, ctx sdk.Context, id, domain, subject, predicate string) *types.Fact {
	f := &types.Fact{
		Id:        id,
		Content:   subject + " " + predicate,
		Domain:    domain,
		Category:  "derived",
		Status:    types.FactStatus_FACT_STATUS_VERIFIED,
		ClaimType: types.ClaimType_CLAIM_TYPE_ASSERTION,
		Structure: &types.ClaimStructure{
			Subject:   subject,
			Predicate: predicate,
		},
	}
	if err := k.SetFact(ctx, f); err != nil {
		panic(err)
	}
	return f
}

func makeDefineFact(k keeper.Keeper, ctx sdk.Context, id, domain, term, meaning string) *types.Fact {
	f := &types.Fact{
		Id:        id,
		Content:   term + " means " + meaning,
		Domain:    domain,
		Category:  "derived",
		Status:    types.FactStatus_FACT_STATUS_VERIFIED,
		ClaimType: types.ClaimType_CLAIM_TYPE_DEFINITION,
		Structure: &types.ClaimStructure{
			Subject:   term,
			Predicate: meaning,
		},
	}
	if err := k.SetFact(ctx, f); err != nil {
		panic(err)
	}
	return f
}

// ─── Transitivity ─────────────────────────────────────────────────────────────

func TestDeriveConsequences_Transitivity_Direct(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	bc := makeRelationFact(k, ctx, "bc", "biology", "mammal", "is-a", "animal")

	results := k.DeriveConsequences(ctx, ab.Id)

	require.Len(t, results, 1)
	d := results[0]
	require.Equal(t, "transitivity", d.RuleName)
	require.Equal(t, types.ClaimType_CLAIM_TYPE_RELATION, d.DerivedClaimType)
	require.Equal(t, "dog", d.DerivedStructure.Subject)
	require.Equal(t, "is-a", d.DerivedStructure.Predicate)
	require.Equal(t, "animal", d.DerivedStructure.Object)
	require.Contains(t, d.SourceFactIds, ab.Id)
	require.Contains(t, d.SourceFactIds, bc.Id)
	require.NotEmpty(t, d.DerivedCanonicalForm)
}

func TestDeriveConsequences_Transitivity_NonTransitiveVerb(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "emotions", "alice", "loves", "bob")
	makeRelationFact(k, ctx, "bc", "emotions", "bob", "loves", "carol")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_MismatchedDomains(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	// Different domain — not found by IterateFactsByDomain("biology")
	makeRelationFact(k, ctx, "bc", "chemistry", "mammal", "is-a", "animal")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_EmptyVerbsList(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	setTransitiveVerbsInf(k, ctx, []string{}) // disable

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	makeRelationFact(k, ctx, "bc", "biology", "mammal", "is-a", "animal")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_NoPair(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	makeRelationFact(k, ctx, "unrelated", "biology", "fish", "is-a", "vertebrate")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_NonAcceptedPremise(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	bc := makeRelationFact(k, ctx, "bc", "biology", "mammal", "is-a", "animal")
	// Demote bc to pending
	bc.Status = types.FactStatus_FACT_STATUS_PENDING
	_ = k.SetFact(ctx, bc)

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_SourceNotAccepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "biology", "dog", "is-a", "mammal")
	ab.Status = types.FactStatus_FACT_STATUS_PENDING
	_ = k.SetFact(ctx, ab)
	makeRelationFact(k, ctx, "bc", "biology", "mammal", "is-a", "animal")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_Transitivity_ChainImplies(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ab := makeRelationFact(k, ctx, "ab", "medicine", "fever", "implies", "inflammation")
	bc := makeRelationFact(k, ctx, "bc", "medicine", "inflammation", "implies", "infection-risk")

	results := k.DeriveConsequences(ctx, ab.Id)
	require.Len(t, results, 1)
	require.Equal(t, "fever", results[0].DerivedStructure.Subject)
	require.Equal(t, "implies", results[0].DerivedStructure.Predicate)
	require.Equal(t, "infection-risk", results[0].DerivedStructure.Object)
	require.Contains(t, results[0].SourceFactIds, ab.Id)
	require.Contains(t, results[0].SourceFactIds, bc.Id)
}

// ─── Definition substitution ─────────────────────────────────────────────────

func TestDeriveConsequences_DefinitionSubstitution_Basic(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	def := makeDefineFact(k, ctx, "def1", "biology", "cat", "feline")
	assrt := makeAssertFact(k, ctx, "a1", "biology", "cat", "meows")

	results := k.DeriveConsequences(ctx, def.Id)

	require.Len(t, results, 1)
	d := results[0]
	require.Equal(t, "definition_substitution", d.RuleName)
	require.Equal(t, types.ClaimType_CLAIM_TYPE_ASSERTION, d.DerivedClaimType)
	require.Equal(t, "feline", d.DerivedStructure.Subject)
	require.Equal(t, "meows", d.DerivedStructure.Predicate)
	require.Contains(t, d.SourceFactIds, def.Id)
	require.Contains(t, d.SourceFactIds, assrt.Id)
	require.NotEmpty(t, d.DerivedCanonicalForm)
}

func TestDeriveConsequences_DefinitionSubstitution_NoMatch(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	def := makeDefineFact(k, ctx, "def1", "biology", "cat", "feline")
	makeAssertFact(k, ctx, "a1", "biology", "dog", "barks")

	results := k.DeriveConsequences(ctx, def.Id)
	require.Empty(t, results)
}

func TestDeriveConsequences_DefinitionSubstitution_MultipleAssertions(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	def := makeDefineFact(k, ctx, "def1", "biology", "cat", "feline")
	makeAssertFact(k, ctx, "a1", "biology", "cat", "meows")
	makeAssertFact(k, ctx, "a2", "biology", "cat", "purrs")

	results := k.DeriveConsequences(ctx, def.Id)
	require.Len(t, results, 2)
	for _, d := range results {
		require.Equal(t, "feline", d.DerivedStructure.Subject)
		require.Equal(t, "definition_substitution", d.RuleName)
	}
}

// ─── Params validation ────────────────────────────────────────────────────────

func TestDeriveConsequences_Params_ValidateEmptyVerb(t *testing.T) {
	p := types.DefaultParams()
	p.TransitiveRelationVerbs = []string{"is-a", ""}
	err := p.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "transitive_relation_verbs[1]")
}

func TestDeriveConsequences_Params_DefaultVerbs(t *testing.T) {
	p := types.DefaultParams()
	require.Contains(t, p.TransitiveRelationVerbs, "is-a")
	require.Contains(t, p.TransitiveRelationVerbs, "implies")
	require.Contains(t, p.TransitiveRelationVerbs, "causes")
	require.Contains(t, p.TransitiveRelationVerbs, "is-part-of")
}

// ─── gRPC query handler ───────────────────────────────────────────────────────

// TestDeriveConsequences_QueryHandler verifies the full gRPC round-trip via
// QueryServer.DerivedConsequences using "causes" as the transitive verb.
func TestDeriveConsequences_QueryHandler(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	p := types.DefaultParams()
	p.TransitiveRelationVerbs = []string{"causes"}
	require.NoError(t, k.SetParams(ctx, &p))

	f1 := makeRelationFact(k, ctx, "heat-exp", "chemistry", "heat", "causes", "expansion")
	makeRelationFact(k, ctx, "exp-pres", "chemistry", "expansion", "causes", "pressure")

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.DerivedConsequences(ctx, &types.QueryDerivedConsequencesRequest{
		FactId: f1.Id,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, f1.Id, resp.SourceFactId)
	require.Len(t, resp.Consequences, 1)

	dc := resp.Consequences[0]
	require.Equal(t, "transitivity", dc.RuleName)
	require.Equal(t, "heat", dc.DerivedStructure.Subject)
	require.Equal(t, "causes", dc.DerivedStructure.Predicate)
	require.Equal(t, "pressure", dc.DerivedStructure.Object)
}

// TestDeriveConsequences_QueryHandler_NilRequest verifies that a nil request
// returns an error.
func TestDeriveConsequences_QueryHandler_NilRequest(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	qs := keeper.NewQueryServerImpl(k)
	_, err := qs.DerivedConsequences(ctx, nil)
	require.Error(t, err)
}
