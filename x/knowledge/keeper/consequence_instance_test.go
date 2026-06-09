package keeper_test

// Pragmatism Gap 1: ConsequenceInstance log on M-PRAGMATIC claims.
//
// Peirce's criterion: truth is what reliably guides successful action.
// M-PRAGMATIC claims must carry a track record of validated consequence
// instances. Params.min_pragmatic_consequence_instances sets the governance-
// configured N: the panel's ACCEPT verdict is blocked (overridden to INCONCLUSIVE)
// until the claim has at least N logged instances.
//
// Tests:
//   1. AddInstance_Success: M-PRAGMATIC claim accepts instance, count returned
//   2. AddInstance_WrongMethodology: non-M-PRAGMATIC claim rejects
//   3. AddInstance_RejectedClaim: rejected claim rejects new instances
//   4. AddInstance_MissingRequiredFields: missing context/predicted/observed fails
//   5. Gate_Disabled: param=0 → ACCEPT proceeds with 0 instances
//   6. Gate_InsufficientInstances: param=3, claim has 1 → ACCEPT → INCONCLUSIVE
//   7. Gate_SufficientInstances: param=3, claim has 3 → ACCEPT proceeds
//   8. Gate_NonPragmatic: param set but claim is M-EMPIRICAL → not gated

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: AddInstance_Success ──────────────────────────────────────────────

func TestConsequenceInstance_AddSuccess(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	sub := makeValidBech32Addr("sub")
	claim := &types.Claim{
		Id:       "claim-pragmatic-1",
		MethodId: types.MethodologyPragmatic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	instance := &types.ConsequenceInstance{
		ContextDescription: "deployed routing protocol in production",
		PredictedOutcome:   "latency under 50ms for 99th percentile",
		ObservedOutcome:    "latency at 42ms p99 across 10k requests",
		EvidenceHash:       "abc123",
		SubmittedBy:        sub,
	}

	count, err := keeper.AddConsequenceInstanceForTest(k, ctx, "claim-pragmatic-1", instance)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Retrieve and verify instance persisted
	stored, found := k.GetClaim(ctx, "claim-pragmatic-1")
	require.True(t, found)
	require.Len(t, stored.ConsequenceInstances, 1)
	require.Equal(t, "deployed routing protocol in production", stored.ConsequenceInstances[0].ContextDescription)
	require.Equal(t, "latency under 50ms for 99th percentile", stored.ConsequenceInstances[0].PredictedOutcome)
	require.Equal(t, "abc123", stored.ConsequenceInstances[0].EvidenceHash)
	require.Equal(t, sub, stored.ConsequenceInstances[0].SubmittedBy)
}

// ── Test 2: AddInstance_WrongMethodology ────────────────────────────────────

func TestConsequenceInstance_WrongMethodology(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-empirical",
		MethodId: types.MethodologyEmpirical,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	instance := &types.ConsequenceInstance{
		ContextDescription: "some context",
		PredictedOutcome:   "some prediction",
		ObservedOutcome:    "some observation",
	}

	_, err := keeper.AddConsequenceInstanceForTest(k, ctx, "claim-empirical", instance)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConsequenceInstanceWrongMethodology)
}

// ── Test 3: AddInstance_RejectedClaim ───────────────────────────────────────

func TestConsequenceInstance_RejectedClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-rejected",
		MethodId: types.MethodologyPragmatic,
		Status:   types.ClaimStatus_CLAIM_STATUS_REJECTED,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	instance := &types.ConsequenceInstance{
		ContextDescription: "context",
		PredictedOutcome:   "prediction",
		ObservedOutcome:    "observation",
	}

	_, err := keeper.AddConsequenceInstanceForTest(k, ctx, "claim-rejected", instance)
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConsequenceInstanceRejectedClaim)
}

// ── Test 4: AddInstance_MissingRequiredFields ────────────────────────────────

func TestConsequenceInstance_MissingRequiredFields(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-pragmatic-fields",
		MethodId: types.MethodologyPragmatic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Missing context_description
	_, err := keeper.AddConsequenceInstanceForTest(k, ctx, "claim-pragmatic-fields", &types.ConsequenceInstance{
		PredictedOutcome: "p",
		ObservedOutcome:  "o",
	})
	require.Error(t, err, "should reject missing context_description")

	// Missing predicted_outcome
	_, err = keeper.AddConsequenceInstanceForTest(k, ctx, "claim-pragmatic-fields", &types.ConsequenceInstance{
		ContextDescription: "ctx",
		ObservedOutcome:    "o",
	})
	require.Error(t, err, "should reject missing predicted_outcome")

	// Missing observed_outcome
	_, err = keeper.AddConsequenceInstanceForTest(k, ctx, "claim-pragmatic-fields", &types.ConsequenceInstance{
		ContextDescription: "ctx",
		PredictedOutcome:   "p",
	})
	require.Error(t, err, "should reject missing observed_outcome")
}

// ── Test 5: Gate_Disabled ────────────────────────────────────────────────────

func TestConsequenceInstance_GateDisabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:                   "claim-pragmatic-gate-disabled",
		MethodId:             types.MethodologyPragmatic,
		Status:               types.ClaimStatus_CLAIM_STATUS_PENDING,
		ConsequenceInstances: nil, // 0 instances
	}

	params := types.Params{
		MinPragmaticConsequenceInstances: 0, // gate disabled
	}

	err := k.CheckPragmaticConsequenceGate(ctx, *claim, params)
	require.NoError(t, err, "gate disabled: 0 instances should pass with param=0")
}

// ── Test 6: Gate_InsufficientInstances ──────────────────────────────────────

func TestConsequenceInstance_GateInsufficientInstances(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-pragmatic-gate-insufficient",
		MethodId: types.MethodologyPragmatic,
		Status:   types.ClaimStatus_CLAIM_STATUS_PENDING,
		ConsequenceInstances: []*types.ConsequenceInstance{
			{ContextDescription: "ctx1", PredictedOutcome: "p1", ObservedOutcome: "o1"},
		}, // 1 instance
	}

	params := types.Params{
		MinPragmaticConsequenceInstances: 3, // need 3
	}

	err := k.CheckPragmaticConsequenceGate(ctx, *claim, params)
	require.Error(t, err, "1 instance < 3 required should fail")
	require.ErrorIs(t, err, types.ErrInsufficientConsequenceInstances)
}

// ── Test 7: Gate_SufficientInstances ────────────────────────────────────────

func TestConsequenceInstance_GateSufficientInstances(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	instances := []*types.ConsequenceInstance{
		{ContextDescription: "ctx1", PredictedOutcome: "p1", ObservedOutcome: "o1"},
		{ContextDescription: "ctx2", PredictedOutcome: "p2", ObservedOutcome: "o2"},
		{ContextDescription: "ctx3", PredictedOutcome: "p3", ObservedOutcome: "o3"},
	}

	claim := &types.Claim{
		Id:                   "claim-pragmatic-gate-sufficient",
		MethodId:             types.MethodologyPragmatic,
		Status:               types.ClaimStatus_CLAIM_STATUS_PENDING,
		ConsequenceInstances: instances,
	}

	params := types.Params{
		MinPragmaticConsequenceInstances: 3, // exactly 3 instances
	}

	err := k.CheckPragmaticConsequenceGate(ctx, *claim, params)
	require.NoError(t, err, "3 instances >= 3 required should pass")
}

// ── Test 8: Gate_NonPragmatic ────────────────────────────────────────────────

func TestConsequenceInstance_GateNonPragmatic(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:                   "claim-empirical-gated",
		MethodId:             types.MethodologyEmpirical,
		Status:               types.ClaimStatus_CLAIM_STATUS_PENDING,
		ConsequenceInstances: nil, // 0 instances — would fail if gated
	}

	params := types.Params{
		MinPragmaticConsequenceInstances: 5, // gate is set but claim is M-EMPIRICAL
	}

	err := k.CheckPragmaticConsequenceGate(ctx, *claim, params)
	require.NoError(t, err, "gate only applies to M-PRAGMATIC: M-EMPIRICAL should always pass")
}
