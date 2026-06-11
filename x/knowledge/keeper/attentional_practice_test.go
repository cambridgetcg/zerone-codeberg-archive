package keeper_test

// Contemplative Traditions — Attentional Training Metric.
//
// Varela's neurophenomenological requirement: verifiers of M-PHENOMENOLOGICAL
// claims must have cultivated the attentional instrument, not just the domain
// vocabulary. A meditator with 500 hours of vipassanā has trained the instrument
// of observation itself — a structurally different epistemic access than a
// scholar who has read Husserl. The chain tracks this via self-declared or
// authority-verified practice hours, gated at MsgSubmitCommitment.
//
// Tests:
//   1. SelfDeclaration: sender == verifier → hours stored, verified_by empty
//   2. AuthorityOverride: governance authority sets hours, verified_by = authority
//   3. AuthorityOverride_RequiresRationale: authority without rationale → error
//   4. Unauthorized: third party cannot set another verifier's hours
//   5. Gate_Disabled: param=0 → all verifiers pass regardless of hours
//   6. Gate_InsufficientHours: param=100, hours=50 → rejected
//   7. Gate_SufficientHours: param=100, hours=100 → passes (inclusive boundary)
//   8. Gate_NonPhenomenological: M-EMPIRICAL claim not gated regardless of param
//   9. Gate_NoRecord: param>0, no calibration record → rejected (0 hours)
//  10. Gate_ExportedFunction: direct keeper function round-trip

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Test 1: SelfDeclaration ──────────────────────────────────────────────────

func TestSetAttentionalPractice_SelfDeclaration(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("attentional-self")
	resp, err := ms.SetAttentionalPractice(ctx, &types.MsgSetAttentionalPractice{
		Sender:          verifier,
		VerifierAddress: verifier,
		PracticeHours:   200,
	})
	require.NoError(t, err)
	require.Equal(t, verifier, resp.VerifierAddress)
	require.Equal(t, uint64(200), resp.PracticeHours)
	require.False(t, resp.Verified, "self-declaration should not mark verified=true")

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, uint64(200), cal.AttentionalPracticeHours)
	require.Empty(t, cal.AttentionalPracticeVerifiedBy, "self-declaration leaves verified_by empty")
}

// ── Test 2: AuthorityOverride ────────────────────────────────────────────────

func TestSetAttentionalPractice_AuthorityOverride(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("attentional-override-target")
	_, err := ms.SetAttentionalPractice(ctx, &types.MsgSetAttentionalPractice{
		Sender:          "zrn1authority",
		VerifierAddress: verifier,
		PracticeHours:   500,
		Rationale:       "confirmed via three-year retreat verification program",
	})
	require.NoError(t, err)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, uint64(500), cal.AttentionalPracticeHours)
	require.Equal(t, "zrn1authority", cal.AttentionalPracticeVerifiedBy, "authority override sets verified_by")
}

// ── Test 3: AuthorityOverride_RequiresRationale ──────────────────────────────

func TestSetAttentionalPractice_AuthorityOverride_RequiresRationale(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("attentional-rationale-req")
	_, err := ms.SetAttentionalPractice(ctx, &types.MsgSetAttentionalPractice{
		Sender:          "zrn1authority",
		VerifierAddress: verifier,
		PracticeHours:   300,
		Rationale:       "", // missing — should be rejected
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rationale required")
}

// ── Test 4: Unauthorized ─────────────────────────────────────────────────────

func TestSetAttentionalPractice_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("attentional-target-unauth")
	attacker := makeValidBech32Addr("attentional-attacker")
	_, err := ms.SetAttentionalPractice(ctx, &types.MsgSetAttentionalPractice{
		Sender:          attacker,
		VerifierAddress: verifier,
		PracticeHours:   999,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// ── Test 5: Gate_Disabled ────────────────────────────────────────────────────

func TestAttentionalPracticeGate_Disabled(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:       "phenom-no-gate",
		MethodId: types.MethodologyPhenomenologic,
	}
	params := &types.Params{
		MinPhenomenologicalPracticeHours: 0, // gate disabled
	}
	// No calibration record at all — but gate is off
	err := k.CheckAttentionalPracticeGate(ctx, claim, makeValidBech32Addr("no-record-verifier"), params)
	require.NoError(t, err, "gate disabled: passes with no record")
}

// ── Test 6: Gate_InsufficientHours ──────────────────────────────────────────

func TestAttentionalPracticeGate_InsufficientHours(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verifier := makeValidBech32Addr("phenom-insufficient-hours")
	require.NoError(t, k.SetAttentionalPracticeHours(ctx, verifier, 50, ""))

	claim := &types.Claim{
		Id:       "phenom-gate-test",
		MethodId: types.MethodologyPhenomenologic,
	}
	params := &types.Params{
		MinPhenomenologicalPracticeHours: 100,
	}
	err := k.CheckAttentionalPracticeGate(ctx, claim, verifier, params)
	require.Error(t, err, "50 hours < 100 required should fail")
	require.ErrorIs(t, err, types.ErrInsufficientAttentionalPractice)
}

// ── Test 7: Gate_SufficientHours (inclusive boundary) ───────────────────────

func TestAttentionalPracticeGate_SufficientHours(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verifier := makeValidBech32Addr("phenom-sufficient-hours")
	require.NoError(t, k.SetAttentionalPracticeHours(ctx, verifier, 100, ""))

	claim := &types.Claim{
		Id:       "phenom-gate-sufficient",
		MethodId: types.MethodologyPhenomenologic,
	}
	params := &types.Params{
		MinPhenomenologicalPracticeHours: 100, // exact boundary
	}
	err := k.CheckAttentionalPracticeGate(ctx, claim, verifier, params)
	require.NoError(t, err, "hours == threshold should pass (inclusive boundary)")
}

// ── Test 8: Gate_NonPhenomenological ────────────────────────────────────────

func TestAttentionalPracticeGate_NonPhenomenological(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verifier := makeValidBech32Addr("empirical-not-gated")
	// Deliberately no practice hours set

	claim := &types.Claim{
		Id:       "empirical-claim",
		MethodId: types.MethodologyEmpirical,
	}
	params := &types.Params{
		MinPhenomenologicalPracticeHours: 500, // high threshold — but M-EMPIRICAL is exempt
	}
	err := k.CheckAttentionalPracticeGate(ctx, claim, verifier, params)
	require.NoError(t, err, "M-EMPIRICAL should not be gated by phenomenological practice hours")
}

// ── Test 9: Gate_NoRecord ────────────────────────────────────────────────────

func TestAttentionalPracticeGate_NoRecord(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// No calibration record for this verifier
	verifier := makeValidBech32Addr("phenom-no-record")

	claim := &types.Claim{
		Id:       "phenom-gate-no-record",
		MethodId: types.MethodologyPhenomenologic,
	}
	params := &types.Params{
		MinPhenomenologicalPracticeHours: 10, // any non-zero threshold
	}
	err := k.CheckAttentionalPracticeGate(ctx, claim, verifier, params)
	require.Error(t, err, "no calibration record = 0 hours; should fail when gate > 0")
	require.ErrorIs(t, err, types.ErrInsufficientAttentionalPractice)
}

// ── Test 10: Gate_ExportedFunction round-trip ────────────────────────────────

func TestSetAttentionalPracticeHours_RoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verifier := makeValidBech32Addr("attentional-round-trip")
	require.NoError(t, k.SetAttentionalPracticeHours(ctx, verifier, 750, "zrn1authority"))

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, uint64(750), cal.AttentionalPracticeHours)
	require.Equal(t, "zrn1authority", cal.AttentionalPracticeVerifiedBy)
}
