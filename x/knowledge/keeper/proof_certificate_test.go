package keeper_test

// Formal Logic Gap 1: Proof Certificates — tests for VerifyProofCertificate
// and MaybeAutoAcceptCertifiedClaim.
//
// A ProofCertificate carries a machine-verifiable derivation for M-FORMAL claims.
// Format CANONICAL_CHAIN enables on-chain premise verification: all premise_fact_ids
// must be accepted facts. When Params.proof_certificate_bypass_enabled is true and
// the certificate is verified, the claim is auto-accepted at MaxConfidence without
// a Proof-of-Truth panel (Tier 0 verification).
//
// Other formats (LEAN4, HASH_ONLY) are stored-only commitments — the chain
// never verifies them and the bypass never fires for them.
//
// Operationalises Formal Logic Brief Gap 1 (★★★★★).

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── VerifyProofCertificate ─────────────────────────────────────────────────

// TestProofCert_NilCertReturnsFlase checks the nil-safe guard.
func TestProofCert_NilCertReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.False(t, k.RecordProofCertificateForTest(ctx, nil))
}

// TestProofCert_NonCanonicalChainReturnsFalse verifies that LEAN4 / HASH_ONLY
// formats are never chain-verified — they are stored commitments only.
func TestProofCert_NonCanonicalChainReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Seed an accepted fact so premises would be valid if format were CANONICAL_CHAIN.
	fact := makeTestFact(t, k, ctx, "fact-lean4-premise", "some axiom", "math", "logic", makeValidBech32Addr("sub"), 800_000)
	fact.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, fact))

	lean4cert := &types.ProofCertificate{
		Format:          types.ProofFormat_PROOF_FORMAT_LEAN4,
		CertificateBytes: []byte("some lean4 term"),
		FormalSystem:    "LEAN4",
		PremiseFactIds:  []string{fact.Id},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, lean4cert),
		"LEAN4 format should never be chain-verified")

	hashCert := &types.ProofCertificate{
		Format:         types.ProofFormat_PROOF_FORMAT_HASH_ONLY,
		CertificateBytes: make([]byte, 32),
		FormalSystem:   "SHA256",
		PremiseFactIds: []string{fact.Id},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, hashCert),
		"HASH_ONLY format should never be chain-verified")
}

// TestProofCert_EmptyPremisesReturnsFalse checks the guard against zero-premise proofs.
func TestProofCert_EmptyPremisesReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("step1: conclude X"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   nil, // no premises — not a valid derivation
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, cert),
		"certificate with no premises should not verify")
}

// TestProofCert_UnknownPremiseReturnsFalse verifies that referencing a
// non-existent fact ID causes verification to fail.
func TestProofCert_UnknownPremiseReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive: X from unknown"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{"fact-does-not-exist"},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, cert),
		"unknown premise fact should fail verification")
}

// TestProofCert_PendingPremiseReturnsFalse ensures PENDING facts (not yet accepted)
// cannot serve as proof premises — only ACTIVE / VERIFIED are valid.
func TestProofCert_PendingPremiseReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Seed a PENDING fact (not yet verified).
	fact := makeTestFact(t, k, ctx, "fact-pending-premise", "unverified assertion", "math", "logic", makeValidBech32Addr("sub-pend"), 0)
	fact.Status = types.FactStatus_FACT_STATUS_PENDING
	require.NoError(t, k.SetFact(ctx, fact))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive B from A"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{fact.Id},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, cert),
		"PENDING premise should not be accepted as proof foundation")
}

// TestProofCert_ContestedPremiseReturnsFalse checks that a CONTESTED fact
// (currently under dispute) cannot anchor a new proof.
func TestProofCert_ContestedPremiseReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := makeTestFact(t, k, ctx, "fact-contested-premise", "disputed assertion", "math", "logic", makeValidBech32Addr("sub-cont"), 700_000)
	fact.Status = types.FactStatus_FACT_STATUS_CONTESTED
	require.NoError(t, k.SetFact(ctx, fact))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive C from disputed"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{fact.Id},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, cert),
		"CONTESTED premise should not be accepted as proof foundation")
}

// TestProofCert_AllActivePremisesReturnsTrue is the positive case:
// all premises are ACTIVE on-chain facts → certificate verified.
func TestProofCert_AllActivePremisesReturnsTrue(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	f1 := makeTestFact(t, k, ctx, "fact-active-1", "axiom A", "math", "logic", makeValidBech32Addr("sub-a1"), 880_000)
	f1.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, f1))

	f2 := makeTestFact(t, k, ctx, "fact-active-2", "axiom B", "math", "logic", makeValidBech32Addr("sub-a2"), 880_000)
	f2.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, f2))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("from A and B, derive A∧B"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{f1.Id, f2.Id},
	}
	require.True(t, k.RecordProofCertificateForTest(ctx, cert),
		"all-ACTIVE premises → certificate should verify")
}

// TestProofCert_VerifiedStatusAlsoAccepted checks that FACT_STATUS_VERIFIED
// (in addition to ACTIVE) is accepted as a valid premise.
func TestProofCert_VerifiedStatusAlsoAccepted(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// makeTestFact creates VERIFIED status by default.
	f1 := makeTestFact(t, k, ctx, "fact-verified-1", "axiom C", "physics", "theory", makeValidBech32Addr("sub-v1"), 700_000)
	// Status is already FACT_STATUS_VERIFIED from makeTestFact.

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive D from C"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{f1.Id},
	}
	require.True(t, k.RecordProofCertificateForTest(ctx, cert),
		"FACT_STATUS_VERIFIED premise should verify")
}

// TestProofCert_MixedPremisesOneBadReturnsFalse: if any premise fails,
// the whole certificate fails regardless of others.
func TestProofCert_MixedPremisesOneBadReturnsFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	good := makeTestFact(t, k, ctx, "fact-good", "axiom E", "math", "logic", makeValidBech32Addr("sub-g"), 880_000)
	good.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, good))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("derive F from E and unknown"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{good.Id, "fact-not-on-chain"},
	}
	require.False(t, k.RecordProofCertificateForTest(ctx, cert),
		"one bad premise → whole certificate invalid")
}

// ── MaybeAutoAcceptCertifiedClaim ─────────────────────────────────────────

// TestProofCert_BypassDisabledNoAutoAccept: even with a verified certificate,
// if bypass is disabled the claim proceeds to PoT.
func TestProofCert_BypassDisabledNoAutoAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Default: ProofCertificateBypassEnabled = false.
	claim := &types.Claim{
		Id:                  "claim-bypass-off",
		FactContent:         "all primes > 2 are odd",
		Domain:              "math",
		Category:            "number theory",
		Submitter:           makeValidBech32Addr("submitter-bypass"),
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		MethodId:            "M-FORMAL",
		CertificateVerified: true, // pretend verified
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	accepted, err := k.MaybeAutoAcceptCertifiedClaim(ctx, claim)
	require.NoError(t, err)
	require.False(t, accepted,
		"bypass disabled → MaybeAutoAcceptCertifiedClaim should return false")
}

// TestProofCert_NonFormalNoAutoAccept: bypass only fires for M-FORMAL claims.
func TestProofCert_NonFormalNoAutoAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProofCertificateBypassEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:                  "claim-non-formal",
		FactContent:         "water is wet",
		Domain:              "chemistry",
		Category:            "properties",
		Submitter:           makeValidBech32Addr("submitter-nf"),
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		MethodId:            "M-EMPIRICAL", // not M-FORMAL
		CertificateVerified: true,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	accepted, err := k.MaybeAutoAcceptCertifiedClaim(ctx, claim)
	require.NoError(t, err)
	require.False(t, accepted,
		"only M-FORMAL claims get Tier 0 bypass")
}

// TestProofCert_NotVerifiedNoAutoAccept: CertificateVerified=false → no bypass.
func TestProofCert_NotVerifiedNoAutoAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProofCertificateBypassEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:                  "claim-not-verified",
		FactContent:         "P implies Q",
		Domain:              "logic",
		Category:            "implications",
		Submitter:           makeValidBech32Addr("submitter-nv"),
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		MethodId:            "M-FORMAL",
		CertificateVerified: false, // not verified
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	accepted, err := k.MaybeAutoAcceptCertifiedClaim(ctx, claim)
	require.NoError(t, err)
	require.False(t, accepted,
		"unverified certificate → no bypass")
}

// TestProofCert_BypassEnabledAutoAccepts: the happy path — M-FORMAL claim with
// verified certificate and bypass enabled is auto-accepted at Tier 0.
func TestProofCert_BypassEnabledAutoAccepts(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProofCertificateBypassEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// Seed a premise fact for the certificate.
	premise := makeTestFact(t, k, ctx, "fact-formal-premise", "axiom: A ∨ ¬A", "logic", "tautologies", makeValidBech32Addr("sub-premise"), 880_000)
	premise.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, premise))

	cert := &types.ProofCertificate{
		Format:           types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN,
		CertificateBytes: []byte("from excluded middle, derive: ¬¬A ↔ A"),
		FormalSystem:     "CANONICAL_FORM_V1",
		PremiseFactIds:   []string{premise.Id},
	}

	claim := &types.Claim{
		Id:                  "claim-tier0-accept",
		FactContent:         "double negation elimination: ¬¬A ↔ A",
		Domain:              "logic",
		Category:            "propositional",
		Submitter:           makeValidBech32Addr("submitter-tier0"),
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		MethodId:            "M-FORMAL",
		CertificateVerified: true,
		ProofCertificate:    cert,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	accepted, err := k.MaybeAutoAcceptCertifiedClaim(ctx, claim)
	require.NoError(t, err)
	require.True(t, accepted, "bypass enabled + M-FORMAL + verified → auto-accept")

	// The claim should now be ACCEPTED.
	updated, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, updated.Status,
		"claim status should be ACCEPTED after Tier 0 bypass")

	// A fact should have been created from the claim.
	var created bool
	k.IterateFacts(ctx, func(f *types.Fact) bool {
		if f.ClaimId == claim.Id {
			created = true
			require.Equal(t, "M-FORMAL", f.MethodId)
			return true // stop
		}
		return false
	})
	require.True(t, created, "a fact should be created from the auto-accepted claim")
}
