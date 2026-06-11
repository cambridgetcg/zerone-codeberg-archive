package keeper_test

// Socratic Dialectic Gap 1 — Elenctic Self-Consistency.
//
// The Socratic elenchus traps interlocutors by their own prior commitments.
// Socrates forced each person to acknowledge how their new claim contradicted
// what they themselves had already agreed to be true. On-chain analogue:
// if submitter X has an accepted Fact with canonical form assert(D, S, P)
// and now submits a new Claim with negate(D, S, P), the chain detects the
// self-contradiction and flags it.
//
// This is distinct from Formal Logic Gap 3 (ConflictingFactId) which flags
// conflict with ANY submitter's fact. Socratic check is scoped to the SAME
// submitter — you are caught by your own prior commitments, not by others'.
//
// Mechanism:
//   - Requires Params.SocraticCheckEnabled = true (default false)
//   - Requires canonical_form on the new claim (assert/negate pair only)
//   - CheckSocraticConsistency: finds opposing canonical hash → checks submitter match
//   - Sets Claim.IsSelfContradictory + Claim.SelfContradictionFactId
//   - Emits zerone.knowledge.socratic_self_contradiction_detected event
//   - Non-blocking: claim proceeds to PoT regardless
//
// Unit tests (CheckSocraticConsistencyForTest):
//   1.  EmptyCanonicalForm → (false, "")
//   2.  EmptySubmitter → (false, "")
//   3.  NoOpposingHashExists → (false, "") [assert with no negate on-chain]
//   4.  NonAssertNegateForm → (false, "") [relate, define: no mechanical inverse]
//   5.  OpposingHashExistsButDifferentSubmitter → (false, "") [other's fact = no elenchus]
//   6.  AssertNegateConflict_SameSubmitter → (true, factID)
//   7.  NegateAssertConflict_SameSubmitter → (true, factID) [symmetric]
//   8.  OpposingHashMapsToClaimNotFact → (false, "") [pending claim, not accepted fact]
//
// Integration tests (MsgServer.SubmitClaim):
//   9.  DisabledByDefault → IsSelfContradictory=false always
//  10.  EnabledNoCanonicalForm → IsSelfContradictory=false [no form, no check]
//  11.  EnabledNoOpposingFact → IsSelfContradictory=false [no prior commitment]
//  12.  SelfContradictionDetected → IsSelfContradictory=true, SelfContradictionFactId set
//  13.  OtherSubmitterConflict_NotSocratic → IsSelfContradictory=false [different submitter]
//  14.  SelfContradictionPropagated → SelfContradictionFactId correctly stored

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// injectAcceptedFact stores a Fact with the given canonical form and registers
// its canonical hash in the KV index (simulating an ACCEPT verdict path).
func injectAcceptedFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, factID, submitter, canonicalForm string) {
	t.Helper()
	norm := types.NormalizeCanonicalForm(canonicalForm)
	hash := types.HashCanonicalForm(norm)
	fact := &types.Fact{
		Id:            factID,
		Content:       "accepted fact for socratic test: " + factID,
		Domain:        "physics",
		Category:      "test",
		Submitter:     submitter,
		Confidence:    900_000,
		Status:        types.FactStatus_FACT_STATUS_ACTIVE,
		CanonicalForm: norm,
		CanonicalHash: hash,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	require.NoError(t, k.SetCanonicalHash(ctx, hash, factID))
}

// enableSocraticCheck fetches params and enables the Socratic check.
func enableSocraticCheck(t *testing.T, k keeper.Keeper, ctx sdk.Context) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.SocraticCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))
}

// ── Unit tests (CheckSocraticConsistencyForTest) ──────────────────────────────

// Test 1: Empty canonical form → always (false, "").
func TestSocraticConsistency_EmptyCanonicalForm(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-1")

	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub, "")
	require.False(t, isSelf)
	require.Empty(t, factID)
}

// Test 2: Empty submitter → always (false, "").
func TestSocraticConsistency_EmptySubmitter(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, "",
		`assert("physics", "light", "travels at c in vacuum")`)
	require.False(t, isSelf)
	require.Empty(t, factID)
}

// Test 3: No opposing hash exists on-chain → (false, "").
func TestSocraticConsistency_NoOpposingHashExists(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-3")

	// No negate(...) fact exists for this assertion.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub,
		`assert("chemistry", "water", "is composed of H2O")`)
	require.False(t, isSelf)
	require.Empty(t, factID)
}

// Test 4: Non-assert/negate canonical form → (false, "") [no mechanical inverse].
func TestSocraticConsistency_RelateFormNoInverse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-4")

	// relate() has no opposing form.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub,
		`relate("biology", "DNA", "encodes", "proteins")`)
	require.False(t, isSelf)
	require.Empty(t, factID)
}

// Test 5: Opposing hash exists but belongs to a DIFFERENT submitter → (false, "").
// This is NOT Socratic (you're not caught by your own commitments).
func TestSocraticConsistency_DifferentSubmitter_NotSocratic(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	submitterA := makeValidBech32Addr("soc-sub-A")
	submitterB := makeValidBech32Addr("soc-sub-B")

	// submitterA has an accepted negate fact.
	injectAcceptedFact(t, k, ctx, "fact-diff-sub",
		submitterA,
		`negate("physics", "light", "has mass")`,
	)

	// submitterB submits the opposing assert — different submitter, not Socratic.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, submitterB,
		`assert("physics", "light", "has mass")`)
	require.False(t, isSelf,
		"opposing fact was submitted by a different person — not a Socratic self-contradiction")
	require.Empty(t, factID)
}

// Test 6: Assert claim when same submitter has accepted negate fact → (true, factID).
func TestSocraticConsistency_AssertVsNegate_SameSubmitter(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-6")

	// sub previously had negate("physics", "electrons", "have negative charge") accepted.
	injectAcceptedFact(t, k, ctx, "fact-electrons-neg",
		sub,
		`negate("physics", "electrons", "have negative charge")`,
	)

	// Now sub submits assert("physics", "electrons", "have negative charge") — self-contradiction.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub,
		`assert("physics", "electrons", "have negative charge")`)
	require.True(t, isSelf,
		"same submitter asserts what they previously negated — Socratic self-contradiction")
	require.Equal(t, "fact-electrons-neg", factID)
}

// Test 7: Negate claim when same submitter has accepted assert fact → (true, factID).
// Symmetric direction: previously asserted, now negating.
func TestSocraticConsistency_NegateVsAssert_SameSubmitter(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-7")

	// sub has an accepted assert fact.
	injectAcceptedFact(t, k, ctx, "fact-gravity-pos",
		sub,
		`assert("physics", "gravity", "attracts masses proportional to their product")`,
	)

	// sub now submits the negation — trapped by their own prior commitment.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub,
		`negate("physics", "gravity", "attracts masses proportional to their product")`)
	require.True(t, isSelf,
		"same submitter negates what they previously asserted — Socratic self-contradiction")
	require.Equal(t, "fact-gravity-pos", factID)
}

// Test 8: Opposing hash maps to a pending Claim (not an accepted Fact) → (false, "").
// The elenchus requires the submitter's ESTABLISHED commitment — a pending claim
// that hasn't been verified yet is not a prior commitment.
func TestSocraticConsistency_OpposingHashMapsToClaim_NotFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	sub := makeValidBech32Addr("soc-sub-8")

	// Store a PENDING claim (not a Fact) with the opposing canonical hash.
	negateNorm := types.NormalizeCanonicalForm(`negate("physics", "photons", "carry momentum")`)
	negateHash := types.HashCanonicalForm(negateNorm)
	pendingClaim := &types.Claim{
		Id:            "claim-pending-oppose",
		FactContent:   "photons do not carry momentum (pending claim)",
		Domain:        "physics",
		Category:      "test",
		Submitter:     sub,
		SubmittedAtBlock: 1,
		Status:        types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:         "1000uzrn",
		CanonicalForm: negateNorm,
		CanonicalHash: negateHash,
	}
	require.NoError(t, k.SetClaim(ctx, pendingClaim))
	// Register the hash → claimID (same as SetClaim does automatically via state.go)
	// Note: SetClaim indexes the canonical hash. The Socratic check only flags
	// when GetFact returns the entity — a Claim returns false from GetFact.

	// assert claim by same submitter — opposing hash maps to a Claim, not a Fact.
	isSelf, factID := keeper.CheckSocraticConsistencyForTest(k, ctx, sub,
		`assert("physics", "photons", "carry momentum")`)
	require.False(t, isSelf,
		"opposing hash maps to a pending Claim (unverified) — not an established commitment; no elenchus")
	require.Empty(t, factID)
}

// ── Integration tests (MsgServer.SubmitClaim) ─────────────────────────────────

// Test 9: DisabledByDefault → IsSelfContradictory always false.
func TestSocraticConsistency_DisabledByDefault(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)
	sub := makeValidBech32Addr("soc-int-9")

	// Inject accepted negate fact for this submitter.
	injectAcceptedFact(t, k, ctx, "fact-disabled-test", sub,
		`negate("physics", "kinetic energy", "is proportional to velocity squared")`)

	// Default params: SocraticCheckEnabled = false.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.False(t, params.SocraticCheckEnabled, "must be disabled by default")

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     sub,
		FactContent:   "kinetic energy is proportional to velocity squared",
		Domain:        "physics",
		Category:      "mechanics",
		Stake:         "1000000",
		CanonicalForm: `assert("physics", "kinetic energy", "is proportional to velocity squared")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.False(t, stored.IsSelfContradictory,
		"IsSelfContradictory must be false when socratic_check_enabled=false (default)")
	require.Empty(t, stored.SelfContradictionFactId)
}

// Test 10: Enabled, no canonical form → IsSelfContradictory = false.
func TestSocraticConsistency_EnabledNoCanonicalForm(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	enableSocraticCheck(t, k, ctx)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("soc-int-10"),
		FactContent: "the specific heat of water is 4.18 J per gram per Kelvin",
		Domain:      "chemistry",
		Category:    "thermodynamics",
		Stake:       "1000000",
		// no CanonicalForm
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.False(t, stored.IsSelfContradictory,
		"no canonical form → Socratic check cannot fire")
}

// Test 11: Enabled, no opposing fact → IsSelfContradictory = false.
func TestSocraticConsistency_EnabledNoOpposingFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	enableSocraticCheck(t, k, ctx)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("soc-int-11"),
		FactContent:   "the speed of light is constant in all inertial frames",
		Domain:        "physics",
		Category:      "special relativity",
		Stake:         "1000000",
		CanonicalForm: `assert("physics", "speed of light", "is constant in all inertial frames")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.False(t, stored.IsSelfContradictory,
		"no prior opposing fact exists → IsSelfContradictory must be false")
}

// Test 12: Self-contradiction detected — full integration path.
func TestSocraticConsistency_SelfContradictionDetected(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)
	sub := makeValidBech32Addr("soc-int-12")

	enableSocraticCheck(t, k, ctx)

	// Inject prior accepted fact: this submitter asserted that entropy always increases.
	injectAcceptedFact(t, k, ctx, "fact-entropy-prior", sub,
		`assert("physics", "entropy", "increases in isolated systems")`)

	// Now the same submitter submits the negation — trapped by their own prior commitment.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     sub,
		FactContent:   "entropy does not always increase in isolated systems",
		Domain:        "physics",
		Category:      "thermodynamics",
		Stake:         "1000000",
		CanonicalForm: `negate("physics", "entropy", "increases in isolated systems")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.True(t, stored.IsSelfContradictory,
		"submitter's new claim contradicts their own prior accepted fact — Socratic elenchus")
	require.Equal(t, "fact-entropy-prior", stored.SelfContradictionFactId,
		"SelfContradictionFactId must point to the submitter's prior accepted fact")
}

// Test 13: Opposing fact by a DIFFERENT submitter → IsSelfContradictory = false.
func TestSocraticConsistency_OtherSubmitterConflict_NotSocratic(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)
	subA := makeValidBech32Addr("soc-int-13a")
	subB := makeValidBech32Addr("soc-int-13b")

	enableSocraticCheck(t, k, ctx)

	// subA has a negate fact.
	injectAcceptedFact(t, k, ctx, "fact-other-sub-negate", subA,
		`negate("biology", "viruses", "are living organisms")`)

	// subB submits the opposing assert — NOT a Socratic self-contradiction.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     subB,
		FactContent:   "viruses are living organisms",
		Domain:        "biology",
		Category:      "classification",
		Stake:         "1000000",
		CanonicalForm: `assert("biology", "viruses", "are living organisms")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.False(t, stored.IsSelfContradictory,
		"conflict with another submitter's fact = NOT Socratic (elenchus requires own prior commitments)")
	require.Empty(t, stored.SelfContradictionFactId)
}

// Test 14: SelfContradictionFactId and IsSelfContradictory are both correctly propagated.
func TestSocraticConsistency_FieldPropagation(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)
	sub := makeValidBech32Addr("soc-int-14")

	enableSocraticCheck(t, k, ctx)

	// Prior fact by sub.
	injectAcceptedFact(t, k, ctx, "fact-wave-particle", sub,
		`assert("physics", "light", "exhibits wave-particle duality")`)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     sub,
		FactContent:   "light does not exhibit wave-particle duality",
		Domain:        "physics",
		Category:      "quantum mechanics",
		Stake:         "1000000",
		CanonicalForm: `negate("physics", "light", "exhibits wave-particle duality")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	// Both fields must be set together consistently.
	require.True(t, stored.IsSelfContradictory)
	require.Equal(t, "fact-wave-particle", stored.SelfContradictionFactId)
	// Verify the fields are non-empty and non-default.
	require.NotEmpty(t, stored.SelfContradictionFactId,
		"SelfContradictionFactId must be the ID of the prior accepted fact")
	// Verify the claim itself still proceeded (not blocked).
	require.NotEmpty(t, stored.Id, "claim must still be stored (non-blocking detection)")
}
