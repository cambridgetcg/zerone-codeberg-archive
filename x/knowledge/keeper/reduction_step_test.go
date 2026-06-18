package keeper_test

// Phenomenology Gap: Reduction Steps as Structured Fields.
//
// Husserl's epoché is not introspection — it is a disciplined methodology that
// brackets the "natural attitude" and attends to the structure of consciousness
// itself. Without an explicit record of each reduction step, an M-PHENOMENOLOGICAL
// claim cannot be distinguished from a free-form first-person report.
//
// Tests:
//   1. AddStep_Success: M-PHENOMENOLOGICAL claim accepts step, count returned
//   2. AddStep_WrongMethodology: non-M-PHENOMENOLOGICAL claim rejects
//   3. AddStep_RejectedClaim: rejected claim rejects new steps
//   4. AddStep_MissingRequiredFields: missing reduction_type/bracketed_assumption/structural_invariant fails
//   5. Gate_Disabled: param=0 → ACCEPT proceeds with 0 steps
//   6. Gate_InsufficientSteps: param=3, claim has 1 → gate blocks
//   7. Gate_SufficientSteps: param=3, claim has 3 → gate passes
//   8. Gate_NonPhenomenological: param set but claim is M-EMPIRICAL → not gated

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: AddStep_Success ──────────────────────────────────────────────────

func TestReductionStep_AddSuccess(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub")
	claim := &types.Claim{
		Id:       "claim-pheno-1",
		MethodId: types.MethodologyPhenomenologic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	step := &types.ReductionStep{
		ReductionType:       "epoche",
		BracketedAssumption: "the chair exists independently of any observer",
		StructuralInvariant: "givenness of the chair as perspectival appearance",
		SubmittedBy:         sub,
	}

	count, err := keeper.AddReductionStepForTest(k, ctx, "claim-pheno-1", step)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Retrieve and verify step persisted
	stored, found := k.GetClaim(ctx, "claim-pheno-1")
	require.True(t, found)
	require.Len(t, stored.PhenomenologicalReductionSteps, 1)
	require.Equal(t, "epoche", stored.PhenomenologicalReductionSteps[0].ReductionType)
	require.Equal(t, "the chair exists independently of any observer", stored.PhenomenologicalReductionSteps[0].BracketedAssumption)
	require.Equal(t, "givenness of the chair as perspectival appearance", stored.PhenomenologicalReductionSteps[0].StructuralInvariant)
	require.Equal(t, sub, stored.PhenomenologicalReductionSteps[0].SubmittedBy)
}

// ── Test 2: AddStep_WrongMethodology ────────────────────────────────────────

func TestReductionStep_WrongMethodology(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-empirical-rs",
		MethodId: types.MethodologyEmpirical,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	step := &types.ReductionStep{
		ReductionType:       "epoche",
		BracketedAssumption: "some assumption",
		StructuralInvariant: "some invariant",
	}

	_, err := keeper.AddReductionStepForTest(k, ctx, "claim-empirical-rs", step)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrReductionStepWrongMethodology)
}

// ── Test 3: AddStep_RejectedClaim ───────────────────────────────────────────

func TestReductionStep_RejectedClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-rejected-rs",
		MethodId: types.MethodologyPhenomenologic,
		Status:   types.ClaimStatus_CLAIM_STATUS_REJECTED,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	step := &types.ReductionStep{
		ReductionType:       "epoche",
		BracketedAssumption: "some assumption",
		StructuralInvariant: "some invariant",
	}

	_, err := keeper.AddReductionStepForTest(k, ctx, "claim-rejected-rs", step)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrReductionStepRejectedClaim)
}

// ── Test 4: AddStep_MissingRequiredFields ────────────────────────────────────

func TestReductionStep_MissingRequiredFields(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-pheno-fields",
		MethodId: types.MethodologyPhenomenologic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Missing reduction_type
	_, err := keeper.AddReductionStepForTest(k, ctx, "claim-pheno-fields", &types.ReductionStep{
		BracketedAssumption: "some assumption",
		StructuralInvariant: "some invariant",
	})
	require.Error(t, err, "should reject missing reduction_type")

	// Missing bracketed_assumption
	_, err = keeper.AddReductionStepForTest(k, ctx, "claim-pheno-fields", &types.ReductionStep{
		ReductionType:       "epoche",
		StructuralInvariant: "some invariant",
	})
	require.Error(t, err, "should reject missing bracketed_assumption")

	// Missing structural_invariant
	_, err = keeper.AddReductionStepForTest(k, ctx, "claim-pheno-fields", &types.ReductionStep{
		ReductionType:       "epoche",
		BracketedAssumption: "some assumption",
	})
	require.Error(t, err, "should reject missing structural_invariant")
}

// ── Test 5: Gate_Disabled ────────────────────────────────────────────────────

func TestReductionStep_GateDisabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:                             "claim-pheno-gate-disabled",
		MethodId:                       types.MethodologyPhenomenologic,
		Status:                         types.ClaimStatus_CLAIM_STATUS_PENDING,
		PhenomenologicalReductionSteps: nil, // 0 steps
	}

	params := types.Params{
		MinPhenomenologicalReductionSteps: 0, // gate disabled
	}

	err := k.CheckPhenomenologicalReductionGate(ctx, *claim, params)
	require.NoError(t, err, "gate disabled: 0 steps should pass with param=0")
}

// ── Test 6: Gate_InsufficientSteps ──────────────────────────────────────────

func TestReductionStep_GateInsufficientSteps(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-pheno-gate-insufficient",
		MethodId: types.MethodologyPhenomenologic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
		PhenomenologicalReductionSteps: []*types.ReductionStep{
			{ReductionType: "epoche", BracketedAssumption: "a1", StructuralInvariant: "i1"},
		}, // 1 step
	}

	params := types.Params{
		MinPhenomenologicalReductionSteps: 3, // need 3
	}

	err := k.CheckPhenomenologicalReductionGate(ctx, *claim, params)
	require.Error(t, err, "1 step < 3 required should fail")
	require.ErrorIs(t, err, types.ErrInsufficientReductionSteps)
}

// ── Test 7: Gate_SufficientSteps ────────────────────────────────────────────

func TestReductionStep_GateSufficientSteps(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	steps := []*types.ReductionStep{
		{ReductionType: "epoche", BracketedAssumption: "a1", StructuralInvariant: "i1"},
		{ReductionType: "phenomenological-reduction", BracketedAssumption: "a2", StructuralInvariant: "i2"},
		{ReductionType: "eidetic-reduction", BracketedAssumption: "a3", StructuralInvariant: "i3"},
	}

	claim := &types.Claim{
		Id:                             "claim-pheno-gate-sufficient",
		MethodId:                       types.MethodologyPhenomenologic,
		Status:                         types.ClaimStatus_CLAIM_STATUS_PENDING,
		PhenomenologicalReductionSteps: steps,
	}

	params := types.Params{
		MinPhenomenologicalReductionSteps: 3, // exactly 3 steps
	}

	err := k.CheckPhenomenologicalReductionGate(ctx, *claim, params)
	require.NoError(t, err, "3 steps >= 3 required should pass")
}

// ── Test 8: Gate_NonPhenomenological ────────────────────────────────────────

func TestReductionStep_GateNonPhenomenological(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:                             "claim-empirical-gated-rs",
		MethodId:                       types.MethodologyEmpirical,
		Status:                         types.ClaimStatus_CLAIM_STATUS_PENDING,
		PhenomenologicalReductionSteps: nil, // 0 steps — would fail if gated
	}

	params := types.Params{
		MinPhenomenologicalReductionSteps: 5, // gate is set but claim is M-EMPIRICAL
	}

	err := k.CheckPhenomenologicalReductionGate(ctx, *claim, params)
	require.NoError(t, err, "gate only applies to M-PHENOMENOLOGICAL: M-EMPIRICAL should always pass")
}
