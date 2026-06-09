package keeper_test

// Contemplative Traditions — Temporal Verification Window.
//
// M-PHENOMENOLOGICAL claims require sustained observation: claims about jhāna
// states, phenomenological realization, or attentional training effects cannot
// be validly assessed in a flash panel. Params.min_phenomenological_window_blocks
// sets the governance-configured minimum: if the verification round elapsed
// (VerdictBlock - StartedAtBlock) is less than this, ACCEPT is overridden to
// INCONCLUSIVE. A new round with a longer observation window must be initiated.
//
// Tests:
//   1. Gate_Disabled: param=0 → ACCEPT allowed at any block
//   2. Gate_WindowTooShort: param=100, elapsed=50 → error
//   3. Gate_WindowSufficient: param=100, elapsed=100 → passes
//   4. Gate_WindowJustEnough: elapsed exactly equals param → passes (inclusive boundary)
//   5. Gate_NonPhenomenological: M-EMPIRICAL not gated regardless of param
//   6. Gate_PhenomPlus: elapsed > param → passes (more time is fine)

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: Gate_Disabled ────────────────────────────────────────────────────

func TestTemporalWindow_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "phenom-disabled",
		MethodId: types.MethodologyPhenomenologic,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 1000,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 0, // disabled
	}
	// Block 1001: only 1 block elapsed, but gate is off
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 1001)
	require.NoError(t, err, "gate disabled: any elapsed time should pass")
}

// ── Test 2: Gate_WindowTooShort ─────────────────────────────────────────────

func TestTemporalWindow_TooShort(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "phenom-too-short",
		MethodId: types.MethodologyPhenomenologic,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 1000,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 100,
	}
	// Block 1050: 50 blocks elapsed, need 100
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 1050)
	require.Error(t, err, "50 blocks elapsed < 100 required should fail")
	require.ErrorIs(t, err, types.ErrVerificationWindowTooShort)
}

// ── Test 3: Gate_WindowSufficient ───────────────────────────────────────────

func TestTemporalWindow_Sufficient(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "phenom-sufficient",
		MethodId: types.MethodologyPhenomenologic,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 1000,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 100,
	}
	// Block 1150: 150 blocks elapsed, need 100
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 1150)
	require.NoError(t, err, "150 blocks elapsed >= 100 required should pass")
}

// ── Test 4: Gate_WindowJustEnough (inclusive boundary) ──────────────────────

func TestTemporalWindow_ExactBoundary(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "phenom-exact",
		MethodId: types.MethodologyPhenomenologic,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 1000,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 100,
	}
	// Block 1100: exactly 100 blocks elapsed — inclusive boundary should pass
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 1100)
	require.NoError(t, err, "elapsed == threshold should pass (inclusive boundary)")
}

// ── Test 5: Gate_NonPhenomenological ────────────────────────────────────────

func TestTemporalWindow_NonPhenomenological(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "empirical-not-gated",
		MethodId: types.MethodologyEmpirical,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 1000,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 1000, // high threshold, but M-EMPIRICAL shouldn't be gated
	}
	// Block 1001: only 1 block elapsed, but not M-PHENOMENOLOGICAL
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 1001)
	require.NoError(t, err, "M-EMPIRICAL should not be gated by phenomenological window param")
}

// ── Test 6: Gate_MoreThanEnough ──────────────────────────────────────────────

func TestTemporalWindow_MoreThanEnough(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := types.Claim{
		Id:       "phenom-long",
		MethodId: types.MethodologyPhenomenologic,
	}
	round := types.VerificationRound{
		Id:             "round-1",
		StartedAtBlock: 100,
	}
	params := types.Params{
		MinPhenomenologicalWindowBlocks: 50,
	}
	// Block 10000: 9900 blocks elapsed, far above 50
	err := keeper.CheckMethodologyTemporalGateForTest(k, ctx, claim, round, params, 10000)
	require.NoError(t, err, "elapsed >> threshold should pass")
}
