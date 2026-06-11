package keeper_test

// Phenomenology Brief Gap 2 + Gap 3 — tests.
//
// Gap 2 (★★★☆☆): M-PHENOMENOLOGICAL claim schema — phenomenological_method
// declaration is required for M-PHENOMENOLOGICAL submissions. Enables verifiers
// to assess whether the stated method (epoché, hermeneutic reduction,
// neurophenomenological protocol) was actually applied.
//
// Gap 3 (★★★☆☆): Cross-modal corroboration gate — high-confidence
// M-PHENOMENOLOGICAL ACCEPT verdicts require at least one ClaimRelation pointing
// to an accepted M-EMPIRICAL fact. Encodes Varela, Thompson & Rosch (1991):
// first-person phenomenological description and third-person neural/behavioral
// data must mutually constrain each other for high-confidence acceptance.
//
// Both gaps are fail-open (disabled at default params) and governance-activatable.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Gap 2: method declaration gate (MsgSubmitClaim) ─────────────────────────

// TestPhenomGap2_MissingMethod_Fails verifies that an M-PHENOMENOLOGICAL claim
// submitted without a phenomenological_method returns ErrMissingPhenomenologicalMethod.
func TestPhenomGap2_MissingMethod_Fails(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("phenom-submitter"),
		FactContent: "Attending to the pre-reflective givenness of duration reveals a non-thetic temporal arc that resists objectification",
		Domain:      "philosophy",
		Category:    "phenomenological-analysis",
		Stake:       "1000000",
		MethodId:    types.MethodologyPhenomenologic, // M-PHENOMENOLOGICAL
		// PhenomenologicalMethod intentionally omitted
	})

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrMissingPhenomenologicalMethod)
}

// TestPhenomGap2_WithMethod_Stored verifies that an M-PHENOMENOLOGICAL claim with
// phenomenological_method set is accepted and the field is stored on the Claim.
func TestPhenomGap2_WithMethod_Stored(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:              makeValidBech32Addr("phenom-submitter-2"),
		FactContent:            "Attending to the pre-reflective givenness of duration reveals a non-thetic temporal arc that resists objectification",
		Domain:                 "philosophy",
		Category:               "phenomenological-analysis",
		Stake:                  "1000000",
		MethodId:               types.MethodologyPhenomenologic,
		PhenomenologicalMethod: "husserl-time-consciousness",
	})

	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, types.MethodologyPhenomenologic, claim.MethodId)
	require.Equal(t, "husserl-time-consciousness", claim.PhenomenologicalMethod)
}

// TestPhenomGap2_NonPhenomNoMethod_Allowed verifies that a non-M-PHENOMENOLOGICAL
// claim submitted without phenomenological_method is accepted (gate is M-PHENOM-only).
func TestPhenomGap2_NonPhenomNoMethod_Allowed(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("empirical-submitter"),
		FactContent: "The boiling point of water at sea level is 100 degrees Celsius under standard atmospheric pressure",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		MethodId:    types.MethodologyEmpirical,
		// PhenomenologicalMethod not set — fine for non-M-PHENOMENOLOGICAL
	})

	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)
}

// TestPhenomGap2_EmptyMethodId_Allowed verifies backward-compatibility: an empty
// method_id (M-LEGACY) does not trigger the phenomenological_method requirement.
func TestPhenomGap2_EmptyMethodId_Allowed(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("legacy-submitter"),
		FactContent: "Gravity causes objects to fall toward the Earth at approximately 9.8 m/s squared near the surface",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		// MethodId intentionally empty (M-LEGACY)
		// PhenomenologicalMethod intentionally empty
	})

	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)
}

// TestPhenomGap2_MethodIdPropagatedToFact verifies that on an accepted claim,
// method_id propagates to the Fact (via rounds.go createFactFromClaim path).
// We test via direct store injection since acceptance requires full round machinery.
func TestPhenomGap2_PhenomenologicalMethodPropagatedToClaim(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:              makeValidBech32Addr("phenom-sub-3"),
		FactContent:            "The epoché reveals the constitutive function of intentional consciousness prior to any naturalistic presupposition about its substrate",
		Domain:                 "philosophy",
		Category:               "phenomenological-analysis",
		Stake:                  "1000000",
		MethodId:               types.MethodologyPhenomenologic,
		PhenomenologicalMethod: "varela-neurophenomenological",
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found, "claim must be stored")
	require.Equal(t, "varela-neurophenomenological", claim.PhenomenologicalMethod,
		"phenomenological_method must be propagated to Claim")
	require.Equal(t, types.MethodologyPhenomenologic, claim.MethodId,
		"method_id must be propagated to Claim")
}

// ── Gap 3: cross-modal corroboration gate (CheckPhenomenologicalCorroborationGate) ─

// TestPhenomGap3_Disabled verifies fail-open behaviour: when the corroboration
// threshold param is 0 (default), the gate always passes regardless of methodology
// or confidence.
func TestPhenomGap3_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-phenom-disabled",
		MethodId: types.MethodologyPhenomenologic,
		// No relations to M-EMPIRICAL facts — would fail if gate were enabled
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 0, // disabled
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 999_000, params)
	require.True(t, result, "disabled gate must always pass")
}

// TestPhenomGap3_NotPhenomenological verifies that the gate is a no-op for
// non-M-PHENOMENOLOGICAL claims even when the threshold is configured.
func TestPhenomGap3_NotPhenomenological(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-empirical-1",
		MethodId: types.MethodologyEmpirical, // not M-PHENOMENOLOGICAL
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 700_000,
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 900_000, params)
	require.True(t, result, "gate must not apply to non-M-PHENOMENOLOGICAL claims")
}

// TestPhenomGap3_BelowThreshold verifies that a low-confidence M-PHENOMENOLOGICAL
// verdict passes the gate without requiring M-EMPIRICAL corroboration.
func TestPhenomGap3_BelowThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:       "claim-phenom-low-conf",
		MethodId: types.MethodologyPhenomenologic,
		// No M-EMPIRICAL relation — but confidence is below threshold
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 800_000,
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 600_000, params)
	require.True(t, result, "confidence below threshold must pass without corroboration")
}

// TestPhenomGap3_HasEmpiricalRelation_Passes verifies that a high-confidence
// M-PHENOMENOLOGICAL claim with an accepted M-EMPIRICAL relation passes the gate.
// This is the canonically correct case per Varela's neurophenomenological requirement.
func TestPhenomGap3_HasEmpiricalRelation_Passes(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Seed an accepted M-EMPIRICAL fact.
	empiricalFact := makeTestFact(t, k, ctx,
		"fact-empirical-neural-1",
		"BOLD signal in the insula correlates with interoceptive awareness reports",
		"neuroscience", "empirical",
		makeValidBech32Addr("neuro-sub"),
		900_000,
	)
	empiricalFact.MethodId = types.MethodologyEmpirical
	empiricalFact.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, empiricalFact))

	claim := &types.Claim{
		Id:       "claim-phenom-with-empirical",
		MethodId: types.MethodologyPhenomenologic,
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: "fact-empirical-neural-1",
				Relation:     types.RelationType_RELATION_TYPE_SUPPORTS,
			},
		},
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 700_000,
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 850_000, params)
	require.True(t, result, "M-PHENOMENOLOGICAL claim with M-EMPIRICAL relation must pass")
}

// TestPhenomGap3_NoEmpiricalRelation_Fails verifies that a high-confidence
// M-PHENOMENOLOGICAL claim with no M-EMPIRICAL relation fails the gate and should
// be downgraded to INCONCLUSIVE by CompleteRound.
func TestPhenomGap3_NoEmpiricalRelation_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Seed an accepted fact — but it's M-PHENOMENOLOGICAL, not M-EMPIRICAL.
	phenomFact := makeTestFact(t, k, ctx,
		"fact-phenom-only-1",
		"Husserl's time-constituting flow is itself non-temporal in the ordinary sense",
		"philosophy", "phenomenological-analysis",
		makeValidBech32Addr("phenom-sub-x"),
		800_000,
	)
	phenomFact.MethodId = types.MethodologyPhenomenologic
	phenomFact.Status = types.FactStatus_FACT_STATUS_ACTIVE
	require.NoError(t, k.SetFact(ctx, phenomFact))

	claim := &types.Claim{
		Id:       "claim-phenom-no-empirical",
		MethodId: types.MethodologyPhenomenologic,
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: "fact-phenom-only-1",
				// Relation is SUPPORTS but target is M-PHENOMENOLOGICAL, not M-EMPIRICAL
				Relation: types.RelationType_RELATION_TYPE_SUPPORTS,
			},
		},
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 700_000,
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 850_000, params)
	require.False(t, result,
		"M-PHENOMENOLOGICAL claim without M-EMPIRICAL relation must fail (→ INCONCLUSIVE)")
}

// TestPhenomGap3_NoRelationsAtAll_Fails verifies that a claim with no relations
// whatsoever fails the gate when threshold is active.
func TestPhenomGap3_NoRelationsAtAll_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:        "claim-phenom-isolated",
		MethodId:  types.MethodologyPhenomenologic,
		Relations: nil, // no relations at all
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 700_000,
	}

	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 850_000, params)
	require.False(t, result,
		"isolated M-PHENOMENOLOGICAL claim with no relations must fail the corroboration gate")
}

// TestPhenomGap3_AtBoundaryConfidence_Passes verifies the strict < threshold
// boundary: confidence exactly at threshold should be blocked (≥ threshold fails).
func TestPhenomGap3_AtBoundaryConfidence_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:        "claim-phenom-boundary",
		MethodId:  types.MethodologyPhenomenologic,
		Relations: nil,
	}
	params := types.Params{
		PhenomenologicalCorroborationConfidenceBps: 700_000,
	}

	// Exactly at threshold — should fail (gate fires at confidence >= threshold)
	result := keeper.CheckPhenomenologicalCorroborationGateForTest(k, ctx, claim, 700_000, params)
	require.False(t, result, "confidence == threshold with no M-EMPIRICAL relation must fail")
}

// TestPhenomGap3_ParamsValidate_TooHigh verifies that setting the param above
// 1_000_000 (100%) is rejected by Params.Validate().
func TestPhenomGap3_ParamsValidate_TooHigh(t *testing.T) {
	params := types.DefaultParams()
	params.PhenomenologicalCorroborationConfidenceBps = 1_000_001 // > 1M BPS

	err := params.Validate()
	require.Error(t, err,
		"PhenomenologicalCorroborationConfidenceBps > 1_000_000 must fail validation")
}

// TestPhenomGap3_ParamsValidate_Valid verifies that valid threshold values pass
// Params.Validate() (0 = disabled, 500k = 50%, 1M = 100%).
func TestPhenomGap3_ParamsValidate_Valid(t *testing.T) {
	for _, bps := range []uint64{0, 500_000, 1_000_000} {
		params := types.DefaultParams()
		params.PhenomenologicalCorroborationConfidenceBps = bps
		require.NoError(t, params.Validate(),
			"PhenomenologicalCorroborationConfidenceBps=%d should be valid", bps)
	}
}
