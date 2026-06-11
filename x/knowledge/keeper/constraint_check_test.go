package keeper_test

// constraint_check_test.go — Formal Logic Gap 3: Automated Constraint Satisfaction Checking.
//
// Tests for the on-chain assert ↔ negate canonical form conflict detection.
// When Params.ConstraintCheckEnabled is true and a new claim has an assert()
// or negate() canonical form, the keeper checks if a verified fact with the
// opposing form exists and sets Claim.ConflictingFactId if found.
//
// Tests:
//  1. DisabledByDefault: constraint_check_enabled=false (default) → ConflictingFactId never set
//  2. NoCanonicalForm: no canonical form → no conflict possible
//  3. AssertNoOpposingFact: assert claim, no opposing negate fact → empty
//  4. AssertVsNegateConflict: assert opposing verified negate fact → conflict set
//  5. NegateVsAssertConflict: negate opposing verified assert fact → conflict set
//  6. ConstrainNoOpposing: constrain() has no direct opposing form → empty
//  7. PendingClaimNotConflict: opposing form exists as pending claim, not Fact → empty
//  8. ComputeOpposingCanonicalForm: pure function coverage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: DisabledByDefault ─────────────────────────────────────────────────

func TestConstraintCheck_DisabledByDefault(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Inject a verified negate fact.
	negateNorm := types.NormalizeCanonicalForm(
		`negate("physics", "energy conservation", "holds in isolated systems")`)
	negateHash := types.HashCanonicalForm(negateNorm)
	negateFact := &types.Fact{
		Id:            "fact-negate-d1",
		Content:       "energy conservation does not hold in isolated systems",
		Domain:        "physics",
		CanonicalForm: negateNorm,
		CanonicalHash: negateHash,
		Confidence:    800_000,
		Status:        types.FactStatus_FACT_STATUS_ACTIVE,
	}
	require.NoError(t, k.SetFact(ctx, negateFact))
	require.NoError(t, k.SetCanonicalHash(ctx, negateHash, negateFact.Id))

	// Default params: ConstraintCheckEnabled = false.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.False(t, params.ConstraintCheckEnabled, "must be disabled by default")

	// Submit opposing assert claim — should NOT trigger conflict (feature disabled).
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc1-sub"),
		FactContent:   "energy conservation holds in isolated systems",
		Domain:        "physics",
		Category:      "thermodynamics",
		Stake:         "1000000",
		CanonicalForm: `assert("physics", "energy conservation", "holds in isolated systems")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, stored.ConflictingFactId,
		"ConflictingFactId must be empty when constraint_check_enabled=false (default)")
}

// ── Test 2: NoCanonicalForm ───────────────────────────────────────────────────

func TestConstraintCheck_NoCanonicalForm(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// Claim has no canonical form — detection cannot fire.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("cc2-sub"),
		FactContent: "water boils at 100 degrees Celsius at sea level",
		Domain:      "chemistry",
		Category:    "physical",
		Stake:       "1000000",
		// no CanonicalForm
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, stored.ConflictingFactId,
		"no canonical form → no conflict check possible")
}

// ── Test 3: AssertNoOpposingFact ──────────────────────────────────────────────

func TestConstraintCheck_AssertNoOpposingFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// No opposing negate fact for this subject exists anywhere.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc3-sub"),
		FactContent:   "mitochondria is the powerhouse of the cell",
		Domain:        "biology",
		Category:      "cell biology",
		Stake:         "1000000",
		CanonicalForm: `assert("biology", "mitochondria", "is the powerhouse of the cell")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, stored.ConflictingFactId,
		"no opposing negate fact exists → ConflictingFactId must be empty")
}

// ── Test 4: AssertVsNegateConflict ────────────────────────────────────────────

func TestConstraintCheck_AssertVsNegateConflict(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// Inject verified negate fact: "faster-than-light travel is NOT physically possible"
	negateNorm := types.NormalizeCanonicalForm(
		`negate("physics", "faster-than-light travel", "is physically possible")`)
	negateHash := types.HashCanonicalForm(negateNorm)
	negateFact := &types.Fact{
		Id:            "fact-ftl-negate",
		Content:       "faster-than-light travel is not physically possible",
		Domain:        "physics",
		CanonicalForm: negateNorm,
		CanonicalHash: negateHash,
		Confidence:    880_000,
		Status:        types.FactStatus_FACT_STATUS_ACTIVE,
	}
	require.NoError(t, k.SetFact(ctx, negateFact))
	require.NoError(t, k.SetCanonicalHash(ctx, negateHash, negateFact.Id))

	// Submit assert claim — directly opposes the verified negate fact.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc4-sub"),
		FactContent:   "faster-than-light travel is physically possible",
		Domain:        "physics",
		Category:      "relativity",
		Stake:         "1000000",
		CanonicalForm: `assert("physics", "faster-than-light travel", "is physically possible")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "fact-ftl-negate", stored.ConflictingFactId,
		"assert claim opposing a verified negate fact must set ConflictingFactId to the fact's ID")
}

// ── Test 5: NegateVsAssertConflict ────────────────────────────────────────────

func TestConstraintCheck_NegateVsAssertConflict(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// Inject verified assert fact: "water is composed of H2O"
	assertNorm := types.NormalizeCanonicalForm(
		`assert("chemistry", "water", "is composed of H2O")`)
	assertHash := types.HashCanonicalForm(assertNorm)
	assertFact := &types.Fact{
		Id:            "fact-water-assert",
		Content:       "water is composed of H2O",
		Domain:        "chemistry",
		CanonicalForm: assertNorm,
		CanonicalHash: assertHash,
		Confidence:    880_000,
		Status:        types.FactStatus_FACT_STATUS_ACTIVE,
	}
	require.NoError(t, k.SetFact(ctx, assertFact))
	require.NoError(t, k.SetCanonicalHash(ctx, assertHash, assertFact.Id))

	// Submit negate claim — directly opposes the verified assert fact.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc5-sub"),
		FactContent:   "water is not composed of H2O",
		Domain:        "chemistry",
		Category:      "composition",
		Stake:         "1000000",
		CanonicalForm: `negate("chemistry", "water", "is composed of H2O")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "fact-water-assert", stored.ConflictingFactId,
		"negate claim opposing a verified assert fact must set ConflictingFactId")
}

// ── Test 6: ConstrainNoOpposing ───────────────────────────────────────────────

func TestConstraintCheck_ConstrainNoOpposing(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// constrain() has no direct logical inverse in this language.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc6-sub"),
		FactContent:   "energy is conserved in isolated systems (constraint)",
		Domain:        "physics",
		Category:      "thermodynamics",
		Stake:         "1000000",
		CanonicalForm: `constrain("physics", "energy", "is conserved in isolated systems")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, stored.ConflictingFactId,
		"constrain() has no direct opposing form → ConflictingFactId must be empty")
}

// ── Test 7: PendingClaimNotConflict ───────────────────────────────────────────

func TestConstraintCheck_PendingClaimNotConflict(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ConstraintCheckEnabled = true
	require.NoError(t, k.SetParams(ctx, params))

	// Register a PENDING negate claim in the canonical hash index.
	// This simulates a claim that has been submitted but not yet accepted.
	negateNorm := types.NormalizeCanonicalForm(
		`negate("mathematics", "Riemann hypothesis", "is true")`)
	negateHash := types.HashCanonicalForm(negateNorm)
	negateClaim := &types.Claim{
		Id:            "claim-riemann-negate",
		FactContent:   "the Riemann hypothesis is false",
		Domain:        "mathematics",
		Category:      "number theory",
		Submitter:     makeValidBech32Addr("rh-sub"),
		Status:        types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:         "1000000",
		CanonicalForm: negateNorm,
		CanonicalHash: negateHash,
	}
	require.NoError(t, k.SetClaim(ctx, negateClaim))
	// The canonical hash index entry points to the claim ID (not a fact ID).
	require.NoError(t, k.SetCanonicalHash(ctx, negateHash, negateClaim.Id))

	// Submit the opposing assert claim.
	// Opposing canonical hash exists in index, but maps to a Claim — not a Fact.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:     makeValidBech32Addr("cc7-sub"),
		FactContent:   "the Riemann hypothesis is true",
		Domain:        "mathematics",
		Category:      "number theory",
		Stake:         "1000000",
		CanonicalForm: `assert("mathematics", "Riemann hypothesis", "is true")`,
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Empty(t, stored.ConflictingFactId,
		"opposing form exists only as a pending claim (not a verified Fact) — no conflict")
}

// ── Test 8: ComputeOpposingCanonicalForm pure function ────────────────────────

func TestConstraintCheck_ComputeOpposingCanonicalForm(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantFunc string // empty = expect "" result
	}{
		{
			name:     "assert → negate",
			input:    `assert("physics", "energy conservation", "holds in isolated systems")`,
			wantFunc: "negate",
		},
		{
			name:     "negate → assert",
			input:    `negate("physics", "energy conservation", "holds in isolated systems")`,
			wantFunc: "assert",
		},
		{
			name:     "assert with scope → negate preserves scope",
			input:    `assert("biology", "evolution", "occurs via natural selection", "current scientific consensus")`,
			wantFunc: "negate",
		},
		{
			name:     "relate: no opposing form",
			input:    `relate("biology", "carbon", "is-a", "organic element")`,
			wantFunc: "",
		},
		{
			name:     "constrain: no opposing form",
			input:    `constrain("physics", "energy", "is conserved in isolated systems")`,
			wantFunc: "",
		},
		{
			name:     "define: no opposing form",
			input:    `define("mathematics", "prime", "natural number > 1 with no positive divisors other than 1 and itself")`,
			wantFunc: "",
		},
		{
			name:     "observe: no opposing form",
			input:    `observe("astronomy", "Andromeda galaxy", "visible to naked eye", "2026-01")`,
			wantFunc: "",
		},
		{
			name:     "empty string: no opposing form",
			input:    "",
			wantFunc: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := types.ComputeOpposingCanonicalForm(tc.input)
			if tc.wantFunc == "" {
				require.Empty(t, got)
				return
			}
			// The result must start with the expected function name followed by '('.
			gotNorm := types.NormalizeCanonicalForm(got)
			require.NotEmpty(t, gotNorm)
			require.True(t, strings.HasPrefix(gotNorm, tc.wantFunc+"("),
				"expected opposing form to start with %s(, got: %s", tc.wantFunc, gotNorm)

			// The inner args must be identical to the original (only function name flips).
			// Strip the original function name prefix to get the arg-list (including parens).
			inputNorm := types.NormalizeCanonicalForm(tc.input)
			inputInner := inputNorm[strings.Index(inputNorm, "("):]
			gotInner := gotNorm[strings.Index(gotNorm, "("):]
			require.Equal(t, inputInner, gotInner,
				"opposing form must preserve all arguments unchanged")
		})
	}
}
