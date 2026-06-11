package keeper_test

// Contemplative Traditions Gap 4 — Cross-Tradition Convergence.
//
// Independent confirmation from practitioners across lineages is stronger
// evidence than same-tradition consensus — the replication logic from
// Scientific Method Gap 1 (panel_diversity_score_bps) applied to the
// contemplative domain.
//
// AgentCalibration.contemplative_tradition (field 28): self-declared lineage.
// VerificationRound.cross_tradition_score_bps (field 18): computed at aggregation.
// Params.min_cross_tradition_diversity_bps (field 189): governance gate.
//
// Tests:
//  1. SelfDeclaration: sender == verifier → tradition stored
//  2. AuthorityOverride: governance authority sets tradition, rationale required
//  3. AuthorityOverride_RequiresRationale: authority without rationale → error
//  4. Unauthorized: third party cannot set another verifier's tradition
//  5. EmptyTradition_Rejected: empty tradition string → error
//  6. Score_AllSameTradition: 3 verifiers from "zen" → score = 1_000_000 / 3
//  7. Score_AllDistinct: 3 distinct traditions → score = 1_000_000
//  8. Score_EmptyTradition: unknown treated as unique singleton
//  9. Score_EmptyPanel: no verifiers → score = 1_000_000 (fail-open)
// 10. Gate_Disabled: param=0 → no gate regardless of score
// 11. Gate_NonPhenomenological: M-EMPIRICAL not gated
// 12. Gate_Passes: score >= threshold → accept unchanged
// 13. Gate_Fails: score < threshold → INCONCLUSIVE

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// setCrossTraditionParam sets min_cross_tradition_diversity_bps in the module.
func setCrossTraditionParam(t *testing.T, k keeper.Keeper, ctx sdk.Context, bps uint64) {
	t.Helper()
	p, err := k.GetParams(ctx)
	require.NoError(t, err)
	p.MinCrossTraditionDiversityBps = bps
	require.NoError(t, k.SetParams(ctx, p))
}

// setTradition stores a tradition declaration directly on a verifier's calibration.
func setTradition(t *testing.T, k keeper.Keeper, ctx sdk.Context, verifier, tradition string) {
	t.Helper()
	require.NoError(t, k.SetContemplativeTradition(ctx, verifier, tradition))
}

// ── Tests 1–5: MsgSetContemplativeTradition ──────────────────────────────────

func TestSetContemplativeTradition_SelfDeclaration(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("trad-self")
	resp, err := ms.SetContemplativeTradition(ctx, &types.MsgSetContemplativeTradition{
		Sender:          verifier,
		VerifierAddress: verifier,
		Tradition:       "zen",
	})
	require.NoError(t, err)
	require.Equal(t, verifier, resp.VerifierAddress)
	require.Equal(t, "zen", resp.Tradition)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, "zen", cal.ContemplativeTradition)
}

func TestSetContemplativeTradition_AuthorityOverride(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("trad-verifier")
	authority := k.GetAuthority()
	_, err := ms.SetContemplativeTradition(ctx, &types.MsgSetContemplativeTradition{
		Sender:          authority,
		VerifierAddress: verifier,
		Tradition:       "theravada",
		Rationale:       "confirmed via retreat records",
	})
	require.NoError(t, err)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, "theravada", cal.ContemplativeTradition)
}

func TestSetContemplativeTradition_AuthorityOverride_RequiresRationale(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("trad-rationale")
	authority := k.GetAuthority()
	_, err := ms.SetContemplativeTradition(ctx, &types.MsgSetContemplativeTradition{
		Sender:          authority,
		VerifierAddress: verifier,
		Tradition:       "tibetan",
		// Rationale intentionally empty
	})
	require.Error(t, err, "authority override without rationale must be rejected")
	require.Contains(t, err.Error(), "rationale required")
}

func TestSetContemplativeTradition_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("trad-victim")
	thirdParty := makeValidBech32Addr("trad-attacker")
	_, err := ms.SetContemplativeTradition(ctx, &types.MsgSetContemplativeTradition{
		Sender:          thirdParty,
		VerifierAddress: verifier,
		Tradition:       "sufi",
	})
	require.Error(t, err, "third party must not be able to set another verifier's tradition")
	require.Contains(t, err.Error(), "unauthorized")
}

func TestSetContemplativeTradition_EmptyTradition_Rejected(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("trad-empty")
	_, err := ms.SetContemplativeTradition(ctx, &types.MsgSetContemplativeTradition{
		Sender:          verifier,
		VerifierAddress: verifier,
		Tradition:       "",
	})
	require.Error(t, err, "empty tradition string must be rejected")
	require.Contains(t, err.Error(), "tradition required")
}

// ── Tests 6–9: ComputeCrossTraditionScore ────────────────────────────────────

func buildRoundWithTraditions(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	traditions []string, // one per verifier; "" = no declaration
) *types.VerificationRound {
	t.Helper()
	reveals := make([]*types.RevealEntry, 0, len(traditions))
	for i, trad := range traditions {
		addr := makeValidBech32Addr("trad-v-score-" + string(rune('a'+i)))
		if trad != "" {
			setTradition(t, k, ctx, addr, trad)
		}
		reveals = append(reveals, &types.RevealEntry{
			Verifier:      addr,
			Vote:          "accept",
			ConfidenceBps: 800_000,
		})
	}
	return &types.VerificationRound{
		Id:      "score-round-" + t.Name(),
		ClaimId: "score-claim-" + t.Name(),
		Reveals: reveals,
	}
}

func TestCrossTraditionScore_AllSameTradition(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	round := buildRoundWithTraditions(t, k, ctx, []string{"zen", "zen", "zen"})
	score := k.ComputeCrossTraditionScoreForTest(ctx, round)

	// distinct=1, total=3 → 1 * 1_000_000 / 3 = 333_333
	require.Equal(t, uint64(333_333), score,
		"three verifiers from same tradition: score = 1/3 × 1_000_000")
}

func TestCrossTraditionScore_AllDistinct(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	round := buildRoundWithTraditions(t, k, ctx, []string{"zen", "theravada", "tibetan"})
	score := k.ComputeCrossTraditionScoreForTest(ctx, round)

	require.Equal(t, uint64(1_000_000), score,
		"three distinct traditions: score = 1_000_000")
}

func TestCrossTraditionScore_EmptyTradition_TreatedAsSingleton(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Two declared "zen" + one unknown → distinct={zen, unknown-a} = 2
	round := buildRoundWithTraditions(t, k, ctx, []string{"zen", "zen", ""})
	score := k.ComputeCrossTraditionScoreForTest(ctx, round)

	// distinct=2, total=3 → 2 * 1_000_000 / 3 = 666_666
	require.Equal(t, uint64(666_666), score,
		"unknown tradition treated as unique singleton: score = 2/3 × 1_000_000")
}

func TestCrossTraditionScore_EmptyPanel(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	round := &types.VerificationRound{
		Id:      "empty-panel-round",
		ClaimId: "empty-panel-claim",
		Reveals: nil,
	}
	score := k.ComputeCrossTraditionScoreForTest(ctx, round)
	require.Equal(t, uint64(1_000_000), score,
		"empty panel → fail-open: full diversity score")
}

// ── Tests 10–13: CheckCrossTraditionGate ─────────────────────────────────────

func TestCrossTraditionGate_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, _ := k.GetParams(ctx)
	params.MinCrossTraditionDiversityBps = 0 // disabled

	claim := &types.Claim{MethodId: "M-PHENOMENOLOGICAL"}
	err := k.CheckCrossTraditionGate(ctx, claim, 100_000, *params) // very low score
	require.NoError(t, err, "gate disabled → always pass regardless of score")
}

func TestCrossTraditionGate_NonPhenomenological(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, _ := k.GetParams(ctx)
	params.MinCrossTraditionDiversityBps = 500_000 // enabled

	claim := &types.Claim{MethodId: "M-EMPIRICAL"}
	err := k.CheckCrossTraditionGate(ctx, claim, 0, *params) // zero score
	require.NoError(t, err, "non-phenomenological claim must not be gated")
}

func TestCrossTraditionGate_Passes(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, _ := k.GetParams(ctx)
	params.MinCrossTraditionDiversityBps = 500_000

	claim := &types.Claim{MethodId: "M-PHENOMENOLOGICAL"}
	err := k.CheckCrossTraditionGate(ctx, claim, 500_000, *params) // exactly at threshold
	require.NoError(t, err, "score == threshold must pass (boundary inclusive)")
}

func TestCrossTraditionGate_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params, _ := k.GetParams(ctx)
	params.MinCrossTraditionDiversityBps = 500_000

	claim := &types.Claim{MethodId: "M-PHENOMENOLOGICAL"}
	err := k.CheckCrossTraditionGate(ctx, claim, 333_333, *params) // same-tradition panel
	require.Error(t, err, "score < threshold on M-PHENOMENOLOGICAL must fail")
	require.Contains(t, err.Error(), "cross-tradition score")
}
