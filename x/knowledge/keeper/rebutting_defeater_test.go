package keeper_test

// Skepticism Gap 5 — Pollock Rebutting Defeater Tracking.
//
// John Pollock (1986) distinguished two kinds of defeaters:
//
//   UNDERCUTTING DEFEATER (Gap 1): attacks the evidential link between
//     evidence E and belief B without asserting ¬B.
//
//   REBUTTING DEFEATER (Gap 5): directly contradicts B — it asserts ¬B.
//     Epistemically stronger than an undercutting defeater.
//
// On-chain encoding:
//
//	When an accepted fact carries a RELATION_TYPE_CONTRADICTS edge pointing
//	to an existing fact, the existing fact receives a rebutting defeater.
//	The chain now holds two accepted facts that directly contradict each other.
//	The older fact's epistemic standing is reduced via its EffectiveConfidenceBps.
//
//	Result:
//	  - TargetFact.RebuttingDefeaterCount is incremented.
//	  - TargetFact.EffectiveConfidenceBps is recomputed:
//	      total_discount = (undercutting_count × undercutting_discount
//	                      + rebutting_count × rebutting_discount)
//	      effective = confidence × max(0, BPS − total_discount) / BPS
//	  - Event zerone.knowledge.rebutting_defeater_recorded is emitted.
//	  - TargetFact.Confidence is NOT modified.
//
// Tests:
//
//  1. Disabled (discount=0) — no rebutting defeater recorded.
//  2. Accepted contradicting fact → target receives rebutting defeater.
//  3. Effective confidence reduced correctly.
//  4. Unknown target fact ID — no panic, no-op.
//  5. Accumulates — multiple accepted contradicting facts compound correctly.
//  6. Full combined discount (≥ BPS) — effective clamps to 0.
//  7. Combined formula: both undercutting AND rebutting defeaters apply.
//  8. Nominal Confidence is never modified by rebutting defeater recording.
//  9. Event attributes are complete and correct.
// 10. computeCombinedEffectiveConfidence formula unit tests.
// 11. Backward compatibility: computeEffectiveConfidence unchanged (undercutting-only).

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// setRebuttingDefeaterParam configures rebutting_defeater_discount_bps.
func setRebuttingDefeaterParam(t *testing.T, k keeper.Keeper, ctx sdk.Context, discountBps uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.RebuttingDefeaterDiscountBps = discountBps
	require.NoError(t, k.SetParams(ctx, params))
}

// setAllDefeaterParams sets both undercutting and rebutting discount params.
func setAllDefeaterParams(t *testing.T, k keeper.Keeper, ctx sdk.Context,
	undercuttingThreshold, undercuttingDiscount, rebuttingDiscount uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.DefeaterOppositionThresholdBps = undercuttingThreshold
	params.DefeaterPerCountDiscountBps = undercuttingDiscount
	params.RebuttingDefeaterDiscountBps = rebuttingDiscount
	require.NoError(t, k.SetParams(ctx, params))
}

// makeTargetFact creates an accepted fact with the given confidence.
func makeTargetFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, id string, confidence uint64) string {
	t.Helper()
	fact := &types.Fact{
		Id:                     id,
		Content:                "target fact " + id,
		Domain:                 "physics",
		Category:               "test",
		Submitter:              makeValidBech32Addr("ret-sub-" + id),
		SubmittedAtBlock:       1,
		VerifiedAtBlock:        1,
		Status:                 types.FactStatus_FACT_STATUS_ACTIVE,
		Confidence:             confidence,
		EffectiveConfidenceBps: confidence,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return id
}

// hasRebuttingDefeaterEvent returns true and the attribute map when the event manager
// holds at least one zerone.knowledge.rebutting_defeater_recorded event.
func hasRebuttingDefeaterEvent(ctx sdk.Context) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.rebutting_defeater_recorded" {
			attrs := make(map[string]string)
			for _, a := range ev.Attributes {
				attrs[a.Key] = a.Value
			}
			return true, attrs
		}
	}
	return false, nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestRebuttingDefeater_Disabled verifies that when rebutting_defeater_discount_bps==0,
// calling recordRebuttingDefeater is a complete no-op.
func TestRebuttingDefeater_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	targetID := makeTargetFact(t, k, ctx, "target-disabled", 900_000)

	// discount=0 → disabled
	setRebuttingDefeaterParam(t, k, ctx, 0)
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contradicting-fact-x")

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, uint64(0), fact.RebuttingDefeaterCount, "disabled: count stays 0")
	require.Equal(t, uint64(900_000), fact.EffectiveConfidenceBps,
		"disabled: effective confidence unchanged")
	found2, _ := hasRebuttingDefeaterEvent(ctx)
	require.False(t, found2, "disabled: no event emitted")
}

// TestRebuttingDefeater_RecordsOnContradiction verifies that a rebutting defeater
// is recorded and the effective confidence is reduced.
func TestRebuttingDefeater_RecordsOnContradiction(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const baseConf = uint64(800_000)
	const discount = uint64(100_000) // 10% per rebutting defeater

	targetID := makeTargetFact(t, k, ctx, "target-rebut", baseConf)
	setRebuttingDefeaterParam(t, k, ctx, discount)

	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contradicting-fact-1")

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, uint64(1), fact.RebuttingDefeaterCount)
	// effective = 800_000 × (1 − 100_000/1_000_000) = 800_000 × 900_000/1_000_000 = 720_000
	require.Equal(t, uint64(720_000), fact.EffectiveConfidenceBps)
	require.Equal(t, baseConf, fact.Confidence, "nominal confidence must not change")
}

// TestRebuttingDefeater_UnknownTarget verifies no panic when target fact doesn't exist.
func TestRebuttingDefeater_UnknownTarget(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setRebuttingDefeaterParam(t, k, ctx, 100_000)

	// Must not panic
	require.NotPanics(t, func() {
		k.RecordRebuttingDefeaterForTest(ctx, "nonexistent-fact", "contradicting-fact-x")
	})
}

// TestRebuttingDefeater_Accumulates verifies that multiple accepted contradictions
// compound correctly.
func TestRebuttingDefeater_Accumulates(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const baseConf = uint64(1_000_000)
	const discount = uint64(100_000) // 10% each

	targetID := makeTargetFact(t, k, ctx, "target-accum", baseConf)
	setRebuttingDefeaterParam(t, k, ctx, discount)

	// Three accepted contradictions
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-1")
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-2")
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-3")

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, uint64(3), fact.RebuttingDefeaterCount)
	// total_discount = 3 × 100_000 = 300_000
	// effective = 1_000_000 × (1_000_000 − 300_000) / 1_000_000 = 700_000
	require.Equal(t, uint64(700_000), fact.EffectiveConfidenceBps)
}

// TestRebuttingDefeater_FullDiscount verifies that when combined discount ≥ BPS,
// effective confidence clamps to 0.
func TestRebuttingDefeater_FullDiscount(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const baseConf = uint64(900_000)
	const discount = uint64(200_000) // 20% each → 5 defeaters = 100%

	targetID := makeTargetFact(t, k, ctx, "target-full", baseConf)
	setRebuttingDefeaterParam(t, k, ctx, discount)

	// 5 × 200_000 = 1_000_000 = BPS — fully discounted
	for i := 0; i < 5; i++ {
		k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-full")
	}

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, uint64(5), fact.RebuttingDefeaterCount)
	require.Equal(t, uint64(0), fact.EffectiveConfidenceBps, "fully discounted → clamp to 0")
	require.Equal(t, baseConf, fact.Confidence, "nominal confidence must not change")
}

// TestRebuttingDefeater_CombinedWithUndercutting verifies that both undercutting
// and rebutting defeaters apply to EffectiveConfidenceBps simultaneously.
func TestRebuttingDefeater_CombinedWithUndercutting(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const baseConf = uint64(1_000_000)

	targetID := makeTargetFact(t, k, ctx, "target-combined", baseConf)

	// Enable both types: undercutting threshold=700_000, discount=100_000;
	// rebutting discount=150_000.
	setAllDefeaterParams(t, k, ctx, 700_000, 100_000, 150_000)

	// Record one undercutting defeater (challenge rejected at 60% < 70% threshold)
	k.RecordDefeaterIfApplicableForTest(ctx, targetID, 600_000)

	// Record one rebutting defeater
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-combined")

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, uint64(1), fact.DefeaterCount)
	require.Equal(t, uint64(1), fact.RebuttingDefeaterCount)
	// total_discount = 1 × 100_000 + 1 × 150_000 = 250_000
	// effective = 1_000_000 × (1_000_000 − 250_000) / 1_000_000 = 750_000
	require.Equal(t, uint64(750_000), fact.EffectiveConfidenceBps)
}

// TestRebuttingDefeater_NominalConfidenceUnchanged verifies that Fact.Confidence
// is never mutated by rebutting defeater recording.
func TestRebuttingDefeater_NominalConfidenceUnchanged(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const baseConf = uint64(850_000)
	targetID := makeTargetFact(t, k, ctx, "target-nominal", baseConf)
	setRebuttingDefeaterParam(t, k, ctx, 200_000)

	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-nom")
	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contra-nom2")

	fact, found := k.GetFact(ctx, targetID)
	require.True(t, found)
	require.Equal(t, baseConf, fact.Confidence,
		"Fact.Confidence must not be modified by rebutting defeater recording")
}

// TestRebuttingDefeater_EventAttributes verifies that the emitted event carries
// all required attributes with correct values.
func TestRebuttingDefeater_EventAttributes(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	targetID := makeTargetFact(t, k, ctx, "target-event", 900_000)
	setRebuttingDefeaterParam(t, k, ctx, 100_000)

	k.RecordRebuttingDefeaterForTest(ctx, targetID, "contradicting-fact-ev")

	found, attrs := hasRebuttingDefeaterEvent(ctx)
	require.True(t, found, "event must be emitted")
	require.Equal(t, targetID, attrs["fact_id"])
	require.Equal(t, "contradicting-fact-ev", attrs["contradicting_fact_id"])
	require.Equal(t, "1", attrs["rebutting_defeater_count"])
	require.NotEmpty(t, attrs["effective_confidence_bps"])
	require.Contains(t, attrs["pollock_criterion"], "rebutting defeater")
}

// ── Formula unit tests ────────────────────────────────────────────────────────

// TestComputeCombined_BothZero verifies the formula returns confidence unchanged
// when both discount terms are zero.
func TestComputeCombined_BothZero(t *testing.T) {
	got := keeper.ComputeCombinedEffectiveConfidenceForTest(800_000, 3, 0, 2, 0)
	require.Equal(t, uint64(800_000), got)
}

// TestComputeCombined_OnlyUndercutting verifies the combined formula reduces
// to the undercutting-only formula when rebutting discount is 0.
func TestComputeCombined_OnlyUndercutting(t *testing.T) {
	// base=1_000_000, undercutting=2×100_000=200_000, rebutting=0
	got := keeper.ComputeCombinedEffectiveConfidenceForTest(1_000_000, 2, 100_000, 0, 0)
	require.Equal(t, uint64(800_000), got)
	// Matches old computeEffectiveConfidence(1_000_000, 2, 100_000)
	old := keeper.ComputeEffectiveConfidenceForTest(1_000_000, 2, 100_000)
	require.Equal(t, old, got, "combined with rebutting=0 must match old undercutting-only formula")
}

// TestComputeCombined_OnlyRebutting verifies the formula when only rebutting
// discount is active.
func TestComputeCombined_OnlyRebutting(t *testing.T) {
	// base=1_000_000, rebutting=3×50_000=150_000
	got := keeper.ComputeCombinedEffectiveConfidenceForTest(1_000_000, 0, 0, 3, 50_000)
	// effective = 1_000_000 × (1_000_000 − 150_000) / 1_000_000 = 850_000
	require.Equal(t, uint64(850_000), got)
}

// TestComputeCombined_BothActive verifies combined math with both terms active.
func TestComputeCombined_BothActive(t *testing.T) {
	// base=1_000_000, undercutting=1×200_000, rebutting=2×100_000 → total=400_000
	got := keeper.ComputeCombinedEffectiveConfidenceForTest(1_000_000, 1, 200_000, 2, 100_000)
	// effective = 1_000_000 × 600_000 / 1_000_000 = 600_000
	require.Equal(t, uint64(600_000), got)
}

// TestComputeCombined_ExceedsBPS verifies combined discount ≥ BPS clamps to 0.
func TestComputeCombined_ExceedsBPS(t *testing.T) {
	// undercutting=5×100_000=500_000; rebutting=5×100_000=500_000 → total=1_000_000
	got := keeper.ComputeCombinedEffectiveConfidenceForTest(900_000, 5, 100_000, 5, 100_000)
	require.Equal(t, uint64(0), got)
}
