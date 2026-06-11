package keeper_test

// Scientific Method Gap 2: Methodology Governance
//
// MsgUpdateMethodology enables governance to amend registered methodology
// definitions — the Kuhn "revolutionary science" path. Key properties:
//
//   - Authority-gated: only the module's governance authority may amend.
//   - Update-only: methodologies must already exist; creation is genesis-only.
//   - Rationale required: every amendment must carry a human-readable reason.
//   - Version auto-incremented: each amendment bumps version by 1.
//   - RatifiedAtBlock auto-set: stamps the chain height at amendment time.
//   - All content fields (compliance_criteria, falsification_paths,
//     cross_method_discount_bps, min_qualification_weight,
//     required_account_types, is_transitional) are fully replaceable.
//
// The PrivilegedAction log records every amendment for audit. Events are
// emitted on-chain for indexing.

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

const (
	testAuthority   = "zrn1authority"
	testNotAuth     = "zrn1notauthority"
	testRationale   = "Scientific consensus on verifier requirements has evolved — updating."
	testMethodId    = types.MethodologyEmpirical
	testMethodIdAlt = types.MethodologyFormal
)

// seedMethodology stores a minimal methodology in the registry for use in tests.
func seedMethodology(t *testing.T, k keeper.Keeper, ctx sdk.Context, id string) {
	t.Helper()
	m := &types.Methodology{
		Id:      id,
		Name:    "Test Methodology",
		Version: 1,
	}
	require.NoError(t, k.SetMethodology(ctx, m))
}

// TestUpdateMethodology_Success verifies the happy path:
// the amended methodology is stored, version is incremented,
// and ratified_at_block is set to the current block height.
func TestUpdateMethodology_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId)

	ms := keeper.NewMsgServerImpl(k)
	resp, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:                   testMethodId,
			Name:                 "Updated Empirical Methodology",
			Description:          "Revised for wave 3 epistemics.",
			ComplianceCriteria:   []string{"peer review", "replication"},
			FalsificationPaths:   []string{"single counter-example refutes universal claims"},
			MinQualificationWeight: 150_000,
			RequiredAccountTypes: []string{"human"},
			IsTransitional:       false,
		},
		Rationale: testRationale,
	})
	require.NoError(t, err)
	require.Equal(t, testMethodId, resp.MethodologyId)
	require.Equal(t, uint64(2), resp.NewVersion, "version should increment from 1 to 2")

	// Verify the stored methodology reflects the full update.
	stored, found := k.GetMethodology(ctx, testMethodId)
	require.True(t, found)
	require.Equal(t, "Updated Empirical Methodology", stored.Name)
	require.Equal(t, uint64(2), stored.Version)
	require.Equal(t, uint64(ctx.BlockHeight()), stored.RatifiedAtBlock)
	require.Equal(t, []string{"peer review", "replication"}, stored.ComplianceCriteria)
	require.Equal(t, []string{"single counter-example refutes universal claims"}, stored.FalsificationPaths)
	require.Equal(t, uint64(150_000), stored.MinQualificationWeight)
	require.Equal(t, []string{"human"}, stored.RequiredAccountTypes)
}

// TestUpdateMethodology_Unauthorized verifies that a non-authority caller
// is rejected with a clear error message.
func TestUpdateMethodology_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId)

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testNotAuth,
		Methodology: &types.Methodology{
			Id:   testMethodId,
			Name: "Unauthorized change",
		},
		Rationale: "should fail",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// TestUpdateMethodology_NotFound verifies that attempting to update a
// methodology that doesn't exist is rejected. Creation is genesis-only.
func TestUpdateMethodology_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Do not seed any methodology.

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:   "M-NONEXISTENT",
			Name: "phantom",
		},
		Rationale: testRationale,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestUpdateMethodology_RationaleRequired verifies that an empty rationale
// is rejected — every governance amendment must justify itself.
func TestUpdateMethodology_RationaleRequired(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId)

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:   testMethodId,
			Name: "missing rationale update",
		},
		Rationale: "", // empty — should fail
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rationale required")
}

// TestUpdateMethodology_NilMsg verifies nil-safety.
func TestUpdateMethodology_NilMsg(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UpdateMethodology(ctx, nil)
	require.Error(t, err)
}

// TestUpdateMethodology_VersionIncrement verifies that consecutive amendments
// each increment the version monotonically. Three amendments → version = 4.
func TestUpdateMethodology_VersionIncrement(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId) // version starts at 1

	ms := keeper.NewMsgServerImpl(k)
	for i := 2; i <= 4; i++ {
		resp, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
			Authority:   testAuthority,
			Methodology: &types.Methodology{Id: testMethodId, Name: "evolving"},
			Rationale:   testRationale,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(i), resp.NewVersion, "version should be %d after amendment %d", i, i-1)
	}

	stored, found := k.GetMethodology(ctx, testMethodId)
	require.True(t, found)
	require.Equal(t, uint64(4), stored.Version)
}

// TestUpdateMethodology_CrossMethodDiscountBps verifies that the
// cross_method_discount_bps map is fully replaced on amendment.
func TestUpdateMethodology_CrossMethodDiscountBps(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Seed with initial cross-method discount.
	initial := &types.Methodology{
		Id:      testMethodId,
		Name:    "Initial",
		Version: 1,
		CrossMethodDiscountBps: map[string]uint64{
			testMethodIdAlt: 800_000,
		},
	}
	require.NoError(t, k.SetMethodology(ctx, initial))

	ms := keeper.NewMsgServerImpl(k)
	// Amend with a different discount map.
	resp, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:   testMethodId,
			Name: "Updated discounts",
			CrossMethodDiscountBps: map[string]uint64{
				testMethodIdAlt:              500_000, // reduced
				types.MethodologyPragmatic:   750_000, // newly added
			},
		},
		Rationale: "Cross-method discount revision after empirical study of consensus drift",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), resp.NewVersion)

	stored, found := k.GetMethodology(ctx, testMethodId)
	require.True(t, found)
	require.Equal(t, uint64(500_000), stored.CrossMethodDiscountBps[testMethodIdAlt])
	require.Equal(t, uint64(750_000), stored.CrossMethodDiscountBps[types.MethodologyPragmatic])
}

// TestUpdateMethodology_IsTransitionalRetirement verifies that governance can
// retire a methodology by setting is_transitional=true (the M-LEGACY sunset path).
func TestUpdateMethodology_IsTransitionalRetirement(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId)

	ms := keeper.NewMsgServerImpl(k)
	resp, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:             testMethodId,
			Name:           "Retiring methodology",
			IsTransitional: true,
		},
		Rationale: "This methodology has been superseded. Setting is_transitional=true to enter sunset.",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), resp.NewVersion)

	stored, found := k.GetMethodology(ctx, testMethodId)
	require.True(t, found)
	require.True(t, stored.IsTransitional, "is_transitional must be set by governance retirement")
}

// TestUpdateMethodology_RequiredAccountTypesCleared verifies that governance
// can REMOVE restrictions by setting required_account_types to an empty slice.
// This represents the reverse operation from the phenomenology gate test:
// widening the panel composition policy rather than narrowing it.
func TestUpdateMethodology_RequiredAccountTypesCleared(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Start with human-only restriction.
	restricted := &types.Methodology{
		Id:                   types.MethodologyPhenomenologic,
		Name:                 "Phenomenological",
		Version:              1,
		RequiredAccountTypes: []string{"human"},
	}
	require.NoError(t, k.SetMethodology(ctx, restricted))

	ms := keeper.NewMsgServerImpl(k)
	// Governance removes the restriction (empty required_account_types = no gate).
	resp, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority: testAuthority,
		Methodology: &types.Methodology{
			Id:                   types.MethodologyPhenomenologic,
			Name:                 "Phenomenological (gate lifted)",
			RequiredAccountTypes: []string{}, // explicit empty = no restriction
		},
		Rationale: "AI verifiers have now passed the phenomenological qualification standard.",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), resp.NewVersion)

	stored, found := k.GetMethodology(ctx, types.MethodologyPhenomenologic)
	require.True(t, found)
	require.Empty(t, stored.RequiredAccountTypes, "restriction should be cleared")
}

// TestUpdateMethodology_RatifiedAtBlockUpdated verifies that each amendment
// stamps the block height at which it was ratified. Two amendments at
// different heights should have different RatifiedAtBlock values.
func TestUpdateMethodology_RatifiedAtBlockUpdated(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	seedMethodology(t, k, ctx, testMethodId)

	ms := keeper.NewMsgServerImpl(k)

	// First amendment at the default context height.
	_, err := ms.UpdateMethodology(ctx, &types.MsgUpdateMethodology{
		Authority:   testAuthority,
		Methodology: &types.Methodology{Id: testMethodId, Name: "v2"},
		Rationale:   testRationale,
	})
	require.NoError(t, err)

	v2, _ := k.GetMethodology(ctx, testMethodId)
	blockV2 := v2.RatifiedAtBlock

	// Second amendment at a later block.
	ctx2 := ctx.WithBlockHeight(999)
	_, err = ms.UpdateMethodology(ctx2, &types.MsgUpdateMethodology{
		Authority:   testAuthority,
		Methodology: &types.Methodology{Id: testMethodId, Name: "v3"},
		Rationale:   testRationale,
	})
	require.NoError(t, err)

	v3, _ := k.GetMethodology(ctx2, testMethodId)
	require.Greater(t, v3.RatifiedAtBlock, blockV2,
		"each amendment must stamp the block at which it was ratified")
	require.Equal(t, uint64(999), v3.RatifiedAtBlock)
}
