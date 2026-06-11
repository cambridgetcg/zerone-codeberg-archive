package keeper_test

// Pragmatism — Downstream Utility Tracking.
//
// Dewey's instrumentalism: the value of a fact is partly determined by how
// much downstream inquiry it enables. Fact.incoming_citation_count (field 23)
// is incremented when an accepted claim cites the fact as a dependency — either
// via claim.relations or via proof_certificate.premise_fact_ids.
//
// The field existed in the proto and was read by fitness scoring, but was never
// written by any code path. This test suite verifies both write paths.
//
// Tests:
//   1. RelationCitation: claim with REQUIRES relation → target fact.incoming_citation_count++
//   2. MultipleCitations: claim with two relations → both targets incremented
//   3. NoCitations: claim with no relations → no increments
//   4. CitationNotIncrementedOnReject: REJECT verdict → no fact created → no increment
//   5. ProofCertPremise: accepted claim with ProofCertificate.premise_fact_ids → premise fact incremented
//   6. PremiseAndRelation: both paths fire together → additive
//   7. UnknownTargetSkipped: relation to non-existent fact → no panic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func duFact(id, domain, content string) *types.Fact {
	return &types.Fact{
		Id:      id,
		Domain:  domain,
		Content: content,
		Status:  types.FactStatus_FACT_STATUS_ACTIVE,
	}
}

func duClaim(id, content string, rels []*types.ClaimRelation) *types.Claim {
	return &types.Claim{
		Id:           id,
		FactContent:  content,
		Domain:       "physics",
		Submitter:    makeValidBech32Addr("du-submitter"),
		SubmittedAtBlock: 1,
		Status:       types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:        "1000uzrn",
		Relations:    rels,
	}
}

func duRound(id, claimID string) *types.VerificationRound {
	return &types.VerificationRound{
		Id:                  id,
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
}

// ── Test 1: RelationCitation ─────────────────────────────────────────────────

func TestDownstreamUtility_RelationCitation(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Store a pre-existing fact that will be cited.
	premise := duFact("premise-fact-1", "physics", "conservation of energy holds in closed systems")
	require.NoError(t, k.SetFact(ctx, premise))

	// Claim that REQUIRES the premise fact.
	claim := duClaim("du-claim-1", "entropy increases in isolated systems", []*types.ClaimRelation{
		{
			TargetFactId: "premise-fact-1",
			Relation:     types.RelationType_RELATION_TYPE_REQUIRES,
		},
	})
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-1", "du-claim-1")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Accept the claim.
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 700_000,
	}
	err := k.CompleteRound(ctx.WithBlockHeight(400), round, result)
	require.NoError(t, err)

	// Premise fact should have its incoming_citation_count incremented.
	updated, found := k.GetFact(ctx, "premise-fact-1")
	require.True(t, found)
	require.Equal(t, uint64(1), updated.IncomingCitationCount, "premise fact should have 1 incoming citation")
}

// ── Test 2: MultipleCitations ────────────────────────────────────────────────

func TestDownstreamUtility_MultipleCitations(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Two premise facts.
	require.NoError(t, k.SetFact(ctx, duFact("multi-premise-a", "physics", "Newton's first law")))
	require.NoError(t, k.SetFact(ctx, duFact("multi-premise-b", "physics", "Newton's second law")))

	claim := duClaim("du-claim-multi", "F=ma is the net force law", []*types.ClaimRelation{
		{TargetFactId: "multi-premise-a", Relation: types.RelationType_RELATION_TYPE_REQUIRES},
		{TargetFactId: "multi-premise-b", Relation: types.RelationType_RELATION_TYPE_REQUIRES},
	})
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-multi", "du-claim-multi")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 700_000,
	})
	require.NoError(t, err)

	factA, _ := k.GetFact(ctx, "multi-premise-a")
	factB, _ := k.GetFact(ctx, "multi-premise-b")
	require.Equal(t, uint64(1), factA.IncomingCitationCount, "premise A should have 1 incoming citation")
	require.Equal(t, uint64(1), factB.IncomingCitationCount, "premise B should have 1 incoming citation")
}

// ── Test 3: NoCitations ──────────────────────────────────────────────────────

func TestDownstreamUtility_NoCitations(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// A standalone fact with no relations.
	standalone := duFact("standalone-fact", "physics", "the speed of light is constant")
	standalone.IncomingCitationCount = 7 // set a known baseline
	require.NoError(t, k.SetFact(ctx, standalone))

	// Claim with no relations.
	claim := duClaim("du-claim-norel", "vacuum permittivity is constant", nil)
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-norel", "du-claim-norel")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 700_000,
	})
	require.NoError(t, err)

	// Standalone fact should be unchanged.
	unchanged, _ := k.GetFact(ctx, "standalone-fact")
	require.Equal(t, uint64(7), unchanged.IncomingCitationCount, "unrelated fact count unchanged")
}

// ── Test 4: CitationNotIncrementedOnReject ───────────────────────────────────

func TestDownstreamUtility_CitationNotIncrementedOnReject(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	premise := duFact("reject-premise", "physics", "phlogiston causes combustion")
	require.NoError(t, k.SetFact(ctx, premise))

	claim := duClaim("du-claim-reject", "aether exists", []*types.ClaimRelation{
		{TargetFactId: "reject-premise", Relation: types.RelationType_RELATION_TYPE_REQUIRES},
	})
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-reject", "du-claim-reject")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// REJECT verdict — no fact is created, no citation should fire.
	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_REJECT, Confidence: 700_000,
	})
	require.NoError(t, err)

	unchanged, _ := k.GetFact(ctx, "reject-premise")
	require.Equal(t, uint64(0), unchanged.IncomingCitationCount, "rejected claim must not increment premise citation count")
}

// ── Test 5: ProofCertPremise ─────────────────────────────────────────────────

func TestDownstreamUtility_ProofCertPremise(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// A premise fact that the proof certificate depends on.
	axiom := duFact("cert-axiom-1", "mathematics", "Peano axioms define the natural numbers")
	axiom.Status = types.FactStatus_FACT_STATUS_VERIFIED
	require.NoError(t, k.SetFact(ctx, axiom))

	// Claim with a proof certificate (CANONICAL_CHAIN format — verified path).
	claim := duClaim("du-claim-cert", "0 is a natural number", nil)
	claim.MethodId = "M-FORMAL"
	claim.ProofCertificate = &types.ProofCertificate{
		Format:         types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		PremiseFactIds: []string{"cert-axiom-1"},
	}
	claim.CertificateVerified = true // simulates post-VerifyProofCertificate state
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-cert", "du-claim-cert")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 700_000,
	})
	require.NoError(t, err)

	// Axiom fact should have its incoming citation count incremented.
	updated, found := k.GetFact(ctx, "cert-axiom-1")
	require.True(t, found)
	require.Equal(t, uint64(1), updated.IncomingCitationCount, "proof cert premise should have 1 incoming citation")
}

// ── Test 6: PremiseAndRelation (additive) ────────────────────────────────────

func TestDownstreamUtility_PremiseAndRelation(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Fact cited by both a relation AND a proof certificate.
	foundation := duFact("dual-premise", "mathematics", "the empty set exists")
	foundation.Status = types.FactStatus_FACT_STATUS_VERIFIED
	require.NoError(t, k.SetFact(ctx, foundation))

	claim := duClaim("du-claim-dual", "the singleton set exists", []*types.ClaimRelation{
		{TargetFactId: "dual-premise", Relation: types.RelationType_RELATION_TYPE_REQUIRES},
	})
	claim.MethodId = "M-FORMAL"
	claim.ProofCertificate = &types.ProofCertificate{
		Format:         types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		PremiseFactIds: []string{"dual-premise"},
	}
	claim.CertificateVerified = true
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-dual", "du-claim-dual")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 700_000,
	})
	require.NoError(t, err)

	// Both paths should fire: relation + proof cert = 2 citations.
	updated, found := k.GetFact(ctx, "dual-premise")
	require.True(t, found)
	require.Equal(t, uint64(2), updated.IncomingCitationCount,
		"cited via both relation and proof cert = 2 incoming citations")
}

// ── Test 7: UnknownTargetSkipped ─────────────────────────────────────────────

func TestDownstreamUtility_UnknownTargetSkipped(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Claim references a non-existent fact as a relation target.
	claim := duClaim("du-claim-ghost", "something derived from nothing", []*types.ClaimRelation{
		{TargetFactId: "nonexistent-fact-xyz", Relation: types.RelationType_RELATION_TYPE_REQUIRES},
	})
	require.NoError(t, k.SetClaim(ctx, claim))
	round := duRound("du-round-ghost", "du-claim-ghost")
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Should not panic; should accept normally.
	err := k.CompleteRound(ctx.WithBlockHeight(400), round, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 700_000,
	})
	require.NoError(t, err, "unknown target fact should not cause an error")
}
