package keeper_test

// ─── Fee Markets Gap 5: QueryNetworkCapacity ─────────────────────────────────
//
// Tests for the NetworkCapacity query handler. The query is read-only: it
// surfaces active_rounds_count, total_active_verifiers, and
// estimated_panel_formation_blocks so submitters can calibrate priority_tip_bps.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// TestNetworkCapacity_EmptyState verifies that a fresh chain returns zeros.
func TestNetworkCapacity_EmptyState(t *testing.T) {
	k, ctx, _, _ := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, uint64(0), resp.ActiveRoundsCount, "no active rounds")
	require.Equal(t, uint64(0), resp.TotalActiveVerifiers, "no active verifiers")
	require.Equal(t, uint64(100), resp.SnapshotBlockHeight, "snapshot at ctx block height")
}

// TestNetworkCapacity_ActiveRoundsCount verifies the count increments
// as verification rounds are created and decrements as they complete.
func TestNetworkCapacity_ActiveRoundsCount(t *testing.T) {
	k, ctx, _, _ := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	// Initially zero.
	resp, _ := qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.Equal(t, uint64(0), resp.ActiveRoundsCount)

	// Create two rounds — count should be 2.
	claim1, _ := makeTestClaim(t, k, ctx, "zrn1sub1", "fact one", "mathematics", "formal", "1000000")
	claim2, _ := makeTestClaim(t, k, ctx, "zrn1sub2", "fact two different content", "physics", "empirical", "1000000")
	_ = claim1
	_ = claim2

	resp, _ = qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.Equal(t, uint64(2), resp.ActiveRoundsCount)
}

// TestNetworkCapacity_TotalActiveVerifiers verifies that the sum of
// selected_verifiers across active rounds is reported correctly.
func TestNetworkCapacity_TotalActiveVerifiers(t *testing.T) {
	k, ctx, _, _ := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	// Build two rounds manually with selected_verifiers set.
	round1 := makeRoundInPhase("r-v1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)
	round1.SelectedVerifiers = []string{"zrn1v1", "zrn1v2", "zrn1v3"}
	require.NoError(t, k.SetVerificationRound(ctx, round1))
	k.IncrementActiveRoundsCount(ctx)

	round2 := makeRoundInPhase("r-v2", "c2", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)
	round2.SelectedVerifiers = []string{"zrn1v4", "zrn1v5"}
	require.NoError(t, k.SetVerificationRound(ctx, round2))
	k.IncrementActiveRoundsCount(ctx)

	resp, err := qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(2), resp.ActiveRoundsCount)
	require.Equal(t, uint64(5), resp.TotalActiveVerifiers, "3 + 2 selected verifiers")
}

// TestNetworkCapacity_EstimatedPanelBlocks verifies the formula:
// CommitPhaseBlocks × MinVerifiers / bondedValidatorCount.
func TestNetworkCapacity_EstimatedPanelBlocks(t *testing.T) {
	k, ctx, _, sk := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	// Add bonded validators.
	sk.addValidator("zrn1v1", 100_000, "bonded")
	sk.addValidator("zrn1v2", 100_000, "bonded")
	sk.addValidator("zrn1v3", 100_000, "bonded")
	sk.addValidator("zrn1v4", 100_000, "bonded")
	sk.addValidator("zrn1v5", 100_000, "bonded")
	sk.addValidator("zrn1v6", 100_000, "bonded")
	// 6 bonded validators

	// Default params: CommitPhaseBlocks=50 (from genesis default), MinVerifiers=3.
	// EstimatedPanelBlocks = 50 × 3 / 6 = 25.
	resp, err := qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.NoError(t, err)

	p, _ := k.GetParams(ctx)
	expected := p.CommitPhaseBlocks * p.MinVerifiers / 6
	require.Equal(t, expected, resp.EstimatedPanelFormationBlocks, "formula: CommitPhaseBlocks × MinVerifiers / validatorCount")
}

// TestNetworkCapacity_EstimatedPanelBlocks_FewerValidatorsThanPanel verifies
// that when bondedCount < MinVerifiers, EstimatedPanelFormationBlocks = CommitPhaseBlocks.
func TestNetworkCapacity_EstimatedPanelBlocks_FewerValidatorsThanPanel(t *testing.T) {
	k, ctx, _, sk := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	// Add only 2 validators but MinVerifiers defaults to 3 — can't form panel.
	sk.addValidator("zrn1v1", 100_000, "bonded")
	sk.addValidator("zrn1v2", 100_000, "bonded")

	resp, err := qs.NetworkCapacity(ctx, &types.QueryNetworkCapacityRequest{})
	require.NoError(t, err)

	p, _ := k.GetParams(ctx)
	require.Equal(t, p.CommitPhaseBlocks, resp.EstimatedPanelFormationBlocks,
		"fewer validators than panel size: return full commit window as estimate")
}

// TestNetworkCapacity_NilRequest verifies a nil request is handled gracefully.
func TestNetworkCapacity_NilRequest(t *testing.T) {
	k, ctx, _, _ := setupKnowledgeTestFull(t)
	qs := keeper.NewQueryServerImpl(k)

	// nil request should not panic
	resp, err := qs.NetworkCapacity(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
}
