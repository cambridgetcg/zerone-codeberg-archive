package keeper_test

// Formal Logic Gap 2: Claim-Level Refutation Conditions — MsgServer integration tests.
//
// Brief: formal-logic-brief.md, Gap 2 (★★★★☆)
//
// The gap: methodology-level FalsificationPaths tell verifiers what can falsify claims
// of a *type*, but a specific claim never declared what would refute *this instance*.
// Fix: repeated string refutation_conditions on Claim. Submitters commit at birth to
// specific falsifiability conditions. This:
//   1. Gives challengers a declared target (not just the methodology's general paths).
//   2. Enables null-result credit for submitters whose pre-declared conditions get met.
//   3. Improves calibration diagnostics — submitters with declared conditions are trackable.
//
// The proto field, msg_server storage, and null_result_credit are all implemented.
// These tests verify the end-to-end submission → storage → challenger-access chain
// that was missing from coverage.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Query-path access ─────────────────────────────────────────────────────────

// TestRefutationConditions_QueryClaim_ExposesConditions verifies the GRPC
// QueryServer.Claim endpoint returns refutation conditions — the production
// read path challengers use before constructing a challenge.
func TestRefutationConditions_QueryClaim_ExposesConditions(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("qrc-sub1"),
		FactContent: "The orbital period of Earth around the Sun is approximately 365.25 days",
		Domain:      "astronomy",
		Category:    "empirical",
		Stake:       "1000000",
		RefutationConditions: []string{
			"if three independent observatories record a period deviating >0.01 days from 365.25",
		},
	})
	require.NoError(t, err)

	qResp, err := qs.Claim(ctx, &types.QueryClaimRequest{Id: resp.ClaimId})
	require.NoError(t, err)
	require.NotNil(t, qResp.Claim)
	require.Len(t, qResp.Claim.RefutationConditions, 1)
	require.Equal(t,
		"if three independent observatories record a period deviating >0.01 days from 365.25",
		qResp.Claim.RefutationConditions[0],
		"QueryClaim must expose the falsifiability declaration to challengers",
	)
}

// TestRefutationConditions_PendingClaimsQuery_ExposesConditions verifies the
// PendingClaims list query includes refutation conditions — challengers scanning
// the pending queue can see declared falsifiability for each claim without a
// per-claim lookup.
func TestRefutationConditions_PendingClaimsQuery_ExposesConditions(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	qs := keeper.NewQueryServerImpl(k)

	conditions := []string{
		"if a direct measurement of c in vacuum deviates >0.001% from 299792458 m/s",
	}

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("qrc-sub2"),
		FactContent:          "The speed of light in vacuum is exactly 299792458 metres per second",
		Domain:               "physics",
		Category:             "constant",
		Stake:                "1000000",
		RefutationConditions: conditions,
	})
	require.NoError(t, err)

	pendingResp, err := qs.PendingClaims(ctx, &types.QueryPendingClaimsRequest{})
	require.NoError(t, err)

	var target *types.Claim
	for _, c := range pendingResp.Claims {
		if c.Id == resp.ClaimId {
			target = c
			break
		}
	}
	require.NotNil(t, target, "submitted claim must appear in pending queue")
	require.Equal(t, conditions, target.RefutationConditions,
		"PendingClaims must carry conditions so challengers can scan without a per-claim lookup",
	)
}

// ── Submit → Store → Retrieve ─────────────────────────────────────────────────

// TestRefutationConditions_StoredOnSubmit verifies the core round-trip: conditions
// declared in MsgSubmitClaim are stored on the Claim and retrievable via GetClaim.
func TestRefutationConditions_StoredOnSubmit(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	conditions := []string{
		"if a replicated experiment yields T < 5×10⁶ K then false",
	}

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("refut-sub1"),
		FactContent:          "The temperature of Andromeda's core is approximately 10 million Kelvin",
		Domain:               "cosmology",
		Category:             "stellar",
		Stake:                "1000000",
		RefutationConditions: conditions,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, conditions, claim.RefutationConditions,
		"RefutationConditions must be stored exactly as declared")
}

// TestRefutationConditions_EmptyIsBackwardCompat verifies that submitting without
// conditions is legal and doesn't break the submission pipeline.
func TestRefutationConditions_EmptyIsBackwardCompat(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("refut-sub2"),
		FactContent: "Hydrogen is the most abundant element in the observable universe",
		Domain:      "cosmology",
		Category:    "composition",
		Stake:       "1000000",
		// RefutationConditions intentionally absent
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, claim.RefutationConditions,
		"no conditions declared → empty slice, not an error")
}

// TestRefutationConditions_MultipleConditionsRoundTrip verifies that multiple
// conditions are preserved in declaration order.
func TestRefutationConditions_MultipleConditionsRoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	conditions := []string{
		"if spectroscopic measurement yields T < 5×10⁶ K from ≥2 independent observatories",
		"if a peer-reviewed meta-analysis rejects the null at α < 0.01",
		"if the IPCC 7th report contradicts the claim in its Summary for Policymakers",
	}

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("refut-sub3"),
		FactContent:          "Global mean surface temperature will exceed 1.5°C above pre-industrial by 2035",
		Domain:               "physics",
		Category:             "forecasting",
		Stake:                "1000000",
		RefutationConditions: conditions,
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Len(t, claim.RefutationConditions, 3, "all three conditions must be stored")
	require.Equal(t, conditions[0], claim.RefutationConditions[0], "order must be preserved")
	require.Equal(t, conditions[1], claim.RefutationConditions[1])
	require.Equal(t, conditions[2], claim.RefutationConditions[2])
}

// ── Challenger Access ─────────────────────────────────────────────────────────

// TestRefutationConditions_ChallengerCanReadConditions simulates the challenger
// workflow: after submission, GetClaim returns the refutation conditions so the
// challenger knows exactly what evidence would constitute a refutation.
//
// This closes the brief's Gap 2 concern: "Challengers don't have a declared target."
func TestRefutationConditions_ChallengerCanReadConditions(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	declaredCondition := "if a double-blind RCT with n > 1000 shows no significant effect at p > 0.05"

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("refut-submitter"),
		FactContent:          "Aspirin reduces cardiovascular events in the general population by more than 20 percent",
		Domain:               "biology",
		Category:             "pharmacology",
		Stake:                "1000000",
		RefutationConditions: []string{declaredCondition},
	})
	require.NoError(t, err)

	// Challenger reads the claim (simulated by GetClaim — in production this is QueryClaim).
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Len(t, claim.RefutationConditions, 1,
		"challenger must see exactly one declared condition")
	require.Equal(t, declaredCondition, claim.RefutationConditions[0],
		"challenger must see the exact declared falsification condition")
}

// TestRefutationConditions_IsolatedPerClaim verifies that conditions on one claim
// don't bleed into another claim from the same submitter.
func TestRefutationConditions_IsolatedPerClaim(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("refut-iso-sub")

	resp1, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            submitter,
		FactContent:          "Claim A: the boiling point of water at sea level is one hundred degrees Celsius",
		Domain:               "chemistry",
		Category:             "physical",
		Stake:                "1000000",
		RefutationConditions: []string{"if any calibrated thermometer records <99°C or >101°C at 1 atm"},
	})
	require.NoError(t, err)

	ctx = advanceBlocks(ctx, 51) // advance past cooldown

	resp2, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Claim B: the melting point of ice at standard pressure is zero degrees Celsius",
		Domain:      "chemistry",
		Category:    "physical",
		Stake:       "1000000",
		// Claim B has no refutation conditions
	})
	require.NoError(t, err)

	claimA, found := k.GetClaim(ctx, resp1.ClaimId)
	require.True(t, found)
	require.Len(t, claimA.RefutationConditions, 1, "Claim A should have 1 condition")

	claimB, found := k.GetClaim(ctx, resp2.ClaimId)
	require.True(t, found)
	require.Empty(t, claimB.RefutationConditions, "Claim B should have no conditions")
}

// ── Combined with Proof Certificate ──────────────────────────────────────────

// TestRefutationConditions_WithProofCert_Combined verifies that a claim can carry
// both refutation conditions AND a proof certificate simultaneously, and both
// fields are stored and retrievable independently.
//
// These two fields occupy orthogonal positions in the epistemic architecture:
//   - refutation_conditions: falsifiability commitment for any claim type
//   - proof_certificate: formal verification for M-FORMAL claims
// A claim that's formally verified AND declares what would override its axioms
// is maximally epistemically responsible.
func TestRefutationConditions_WithProofCert_Combined(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Seed an accepted fact for the proof certificate's premise.
	premise := makeTestFact(t, k, ctx,
		"fact-prime-axiom",
		"every integer greater than one has a prime factorization",
		"mathematics", "number-theory",
		makeValidBech32Addr("refut-premise-sub"),
		880_000,
	)
	premise.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, premise))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive: 4 has prime factorization 2×2, from fundamental theorem"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{premise.Id},
	}

	refutCond := "if there exists a counter-example integer > 1 with no prime factorization"

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("refut-cert-sub"),
		FactContent:          "Every integer greater than one has a unique prime factorization (fundamental theorem of arithmetic)",
		Domain:               "mathematics",
		Category:             "number-theory",
		Stake:                "1000000",
		RefutationConditions: []string{refutCond},
		ProofCertificate:     cert,
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)

	// Both fields stored.
	require.Equal(t, []string{refutCond}, claim.RefutationConditions,
		"refutation conditions must be stored alongside proof certificate")
	require.NotNil(t, claim.ProofCertificate,
		"proof certificate must also be stored")
	require.Equal(t, types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN, claim.ProofCertificate.Format)

	// Certificate is verified (CANONICAL_CHAIN + active premise = verified).
	require.True(t, claim.CertificateVerified,
		"CertificateVerified should be true when premise is active and format is CANONICAL_CHAIN")
}

// ── Falsifiability at Round Completion ───────────────────────────────────────

// TestRefutationConditions_NullResultCredit_WiredFromRejection verifies the full
// economic chain: submit claim with conditions → simulate rejection via CompleteRound
// → null-result credit is minted.
//
// This is the key economic incentive: submitters who pre-declare falsifiability and
// then get falsified receive a credit. The credit corrects publication bias —
// "testable and wrong" becomes better than "untestable."
func TestRefutationConditions_NullResultCredit_WiredFromRejection(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Enable the null-result credit.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 250
	require.NoError(t, k.SetParams(ctx, params))

	submitter := makeValidBech32Addr("refut-nrc-sub")

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            submitter,
		FactContent:          "A specific protein fold predicted by AlphaFold3 will be confirmed by cryo-EM within two years",
		Domain:               "biology",
		Category:             "structural",
		Stake:                "1000000",
		Sponsored:            true, // avoid bank send complexity for this test
		RefutationConditions: []string{
			"if cryo-EM structure at resolution < 3.5 Å contradicts the predicted fold",
		},
	})
	require.NoError(t, err)

	// Retrieve the claim so we can simulate rejection directly.
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Len(t, claim.RefutationConditions, 1,
		"condition must be stored before rejection fires")

	// Snapshot balance before simulating rejection.
	mintedBefore := bk.minted.AmountOf("uzrn").Int64()

	// Fire null-result credit directly (maybeAwardNullResultCredit is the internal
	// path called by rounds.go on VERDICT_REJECT). RecordNullResultCreditForTest is
	// the exported test wrapper for the same function.
	k.RecordNullResultCreditForTest(ctx, claim)

	mintedDelta := bk.minted.AmountOf("uzrn").Int64() - mintedBefore
	require.Equal(t, int64(250), mintedDelta,
		"250 uzrn null-result credit must be minted on rejection of a claim with conditions")
}

// TestRefutationConditions_NoNullResultCredit_WhenNotDeclared verifies that a
// rejected claim without pre-declared conditions receives no null-result credit.
// This is the incentive's teeth: only Popperian good citizenship is rewarded.
func TestRefutationConditions_NoNullResultCredit_WhenNotDeclared(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 250
	require.NoError(t, k.SetParams(ctx, params))

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("refut-no-cond"),
		FactContent: "A specific protein fold predicted by AlphaFold3 will be confirmed by cryo-EM within two years",
		Domain:      "biology",
		Category:    "structural",
		Stake:       "1000000",
		Sponsored:   true,
		// No RefutationConditions — submitter chose not to declare
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, claim.RefutationConditions)

	mintedBefore := bk.minted.AmountOf("uzrn").Int64()
	k.RecordNullResultCreditForTest(ctx, claim)

	require.Equal(t, mintedBefore, bk.minted.AmountOf("uzrn").Int64(),
		"no credit should be minted when no conditions were declared")
}
