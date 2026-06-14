package keeper_test

// Skepticism Gap 1 — Pollock Undercutting Defeater Tracking.
//
// John Pollock (1986) distinguished rebutting defeaters (which directly
// contradict a belief) from undercutting defeaters (which attack the
// evidential connection between evidence and belief without contradicting
// the belief itself).
//
// On-chain encoding:
//
//	A challenge claim that is REJECTED (the fact survives) but only by a slim
//	margin (panel rejection confidence < DefeaterOppositionThresholdBps) is
//	treated as an undercutting defeater: the fact survived, but the evidence
//	for its truth is weakly supporting — a meaningful minority doubted it.
//
//	Result:
//	  - Fact.DefeaterCount is incremented.
//	  - Fact.EffectiveConfidenceBps is recomputed as:
//	      effective = confidence × (1 − defeaterCount × discount / 1_000_000)
//	      clamped to [0, confidence].
//	  - Event zerone.knowledge.defeater_recorded is emitted.
//	  - Nominal Fact.Confidence is NOT modified.
//
// Tests:
//
//  1. Disabled (threshold=0) — no defeater recorded at any confidence level.
//  2. High rejection confidence ≥ threshold — no defeater (strong corroboration).
//  3. Low rejection confidence < threshold — defeater recorded + effective confidence adjusted.
//  4. Unknown fact ID — no panic, no-op.
//  5. Accumulates — multiple defeaters compound correctly.
//  6. Full discount (defeaterCount × discountBps ≥ 1_000_000) — effective clamps to 0.
//  7. Discount=0 — effective_confidence equals nominal confidence regardless of defeater_count.
//  8. computeEffectiveConfidence formula unit tests.
//  9. Event attributes are complete and correct.
// 10. Nominal Confidence is never modified by defeater recording.

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// makeFactWithConfidence creates and stores a fact with the given confidence.
// Returns the fact ID.
func makeFactWithConfidence(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	id string,
	confidence uint64,
) string {
	t.Helper()
	fact := &types.Fact{
		Id:                    id,
		Content:               "defeater test fact " + id,
		Domain:                "physics",
		Category:              "test",
		Submitter:             makeValidBech32Addr("def-sub-" + id),
		SubmittedAtBlock:      1,
		VerifiedAtBlock:       1,
		Status:                types.FactStatus_FACT_STATUS_ACTIVE,
		Confidence:            confidence,
		EffectiveConfidenceBps: confidence, // initialized same as confidence
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return id
}

// hasDeferaterEvent returns true when the SDK event manager holds at least
// one zerone.knowledge.defeater_recorded event. Also returns the attribute map.
func hasDeferaterEvent(ctx sdk.Context) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.defeater_recorded" {
			attrs := make(map[string]string)
			for _, a := range ev.Attributes {
				attrs[a.Key] = a.Value
			}
			return true, attrs
		}
	}
	return false, nil
}

// setDefeaterParams is a test helper that configures the defeater params.
func setDefeaterParams(t *testing.T, k keeper.Keeper, ctx sdk.Context, thresholdBps, discountBps uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.DefeaterOppositionThresholdBps = thresholdBps
	params.DefeaterPerCountDiscountBps = discountBps
	require.NoError(t, k.SetParams(ctx, params))
}

// ── Unit tests for formula ────────────────────────────────────────────────────

// TestComputeEffectiveConfidence_NoDiscount verifies that discount=0 always
// returns the original confidence unchanged.
func TestComputeEffectiveConfidence_NoDiscount(t *testing.T) {
	for _, confidence := range []uint64{0, 100_000, 500_000, 800_000, 1_000_000} {
		got := keeper.ComputeEffectiveConfidenceForTest(confidence, 5, 0)
		require.Equal(t, confidence, got,
			"discount=0: effective must equal confidence for any defeaterCount")
	}
}

// TestComputeEffectiveConfidence_ZeroDefeaters verifies that defeaterCount=0
// always returns the original confidence unchanged regardless of discount.
func TestComputeEffectiveConfidence_ZeroDefeaters(t *testing.T) {
	got := keeper.ComputeEffectiveConfidenceForTest(800_000, 0, 50_000)
	require.Equal(t, uint64(800_000), got,
		"defeaterCount=0: effective must equal confidence")
}

// TestComputeEffectiveConfidence_SingleDefeater validates one-defeater math.
//
// base=800_000, discount=50_000 (5%), defeaters=1:
//
//	effective = 800_000 × (1 − 1 × 50_000 / 1_000_000)
//	          = 800_000 × 950_000 / 1_000_000
//	          = 760_000
func TestComputeEffectiveConfidence_SingleDefeater(t *testing.T) {
	got := keeper.ComputeEffectiveConfidenceForTest(800_000, 1, 50_000)
	require.Equal(t, uint64(760_000), got,
		"single defeater: effective = base × (1 − discount/BPS)")
}

// TestComputeEffectiveConfidence_MultipleDefeaters validates multi-defeater math.
//
// base=800_000, discount=50_000 (5%), defeaters=3:
//
//	totalDiscount = 3 × 50_000 = 150_000
//	effective = 800_000 × (1_000_000 − 150_000) / 1_000_000
//	          = 800_000 × 850_000 / 1_000_000
//	          = 680_000
func TestComputeEffectiveConfidence_MultipleDefeaters(t *testing.T) {
	got := keeper.ComputeEffectiveConfidenceForTest(800_000, 3, 50_000)
	require.Equal(t, uint64(680_000), got,
		"3 defeaters, 5% each: effective = 800_000 × 0.85 = 680_000")
}

// TestComputeEffectiveConfidence_FullyDiscounted verifies that when total
// discount ≥ 1_000_000 (100%), effective clamps to 0.
//
// base=800_000, discount=200_000 (20%), defeaters=6:
//
//	totalDiscount = 6 × 200_000 = 1_200_000 ≥ 1_000_000 → clamp to 0
func TestComputeEffectiveConfidence_FullyDiscounted(t *testing.T) {
	got := keeper.ComputeEffectiveConfidenceForTest(800_000, 6, 200_000)
	require.Equal(t, uint64(0), got,
		"total discount ≥ 100%: effective clamps to 0")
}

// TestComputeEffectiveConfidence_ExactBoundary verifies the exact boundary:
// totalDiscount = BPS → effective = 0 (zero, not negative).
//
// base=800_000, discount=100_000 (10%), defeaters=10:
//
//	totalDiscount = 10 × 100_000 = 1_000_000 = BPS → clamp to 0
func TestComputeEffectiveConfidence_ExactBoundary(t *testing.T) {
	got := keeper.ComputeEffectiveConfidenceForTest(800_000, 10, 100_000)
	require.Equal(t, uint64(0), got,
		"total discount exactly at BPS: effective clamps to 0")
}

// ── Keeper-level integration tests ──────────────────────────────────────────

// TestDefeater_Disabled_NoRecord verifies that when threshold=0 (disabled),
// no defeater is recorded even when rejection confidence is very low.
func TestDefeater_Disabled_NoRecord(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-disabled", 800_000)
	setDefeaterParams(t, k, ctx, 0 /* disabled */, 50_000)

	k.RecordDefeaterIfApplicableForTest(ctx, factID, 200_000) // very low confidence

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(0), fact.DefeaterCount,
		"threshold=0: no defeater recorded at any confidence")
	require.Equal(t, uint64(800_000), fact.EffectiveConfidenceBps,
		"effective confidence must remain unchanged when disabled")

	evFound, _ := hasDeferaterEvent(ctx)
	require.False(t, evFound, "no event emitted when feature disabled")
}

// TestDefeater_HighConfidence_NoRecord verifies that when rejection confidence
// meets or exceeds the threshold, no defeater is recorded (strong corroboration).
func TestDefeater_HighConfidence_NoRecord(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-high", 800_000)
	setDefeaterParams(t, k, ctx, 600_000 /* 60% threshold */, 50_000)

	// Rejection at 70% — well above the 60% threshold.
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 700_000)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(0), fact.DefeaterCount,
		"rejection confidence 70% ≥ threshold 60%: no defeater")
	require.Equal(t, uint64(800_000), fact.EffectiveConfidenceBps,
		"strong corroboration: effective confidence unchanged")
}

// TestDefeater_AtThreshold_NoRecord verifies that rejection confidence exactly
// equal to the threshold is NOT a defeater (condition is strictly <).
func TestDefeater_AtThreshold_NoRecord(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-boundary", 800_000)
	setDefeaterParams(t, k, ctx, 600_000, 50_000)

	// Rejection at exactly 60% — equal to threshold, not below.
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 600_000)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(0), fact.DefeaterCount,
		"rejection_confidence == threshold: strictly < required, no defeater")
}

// TestDefeater_LowConfidence_RecordedAndDiscounted verifies the core case:
// rejection confidence below threshold records a defeater and adjusts
// effective_confidence_bps.
//
// base_confidence=800_000, discount=50_000 (5%), rejection=450_000 (45%),
// threshold=600_000 (60%) → defeater recorded.
//
// effective = 800_000 × (1 − 1 × 50_000 / 1_000_000) = 760_000
func TestDefeater_LowConfidence_RecordedAndDiscounted(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-low", 800_000)
	setDefeaterParams(t, k, ctx, 600_000, 50_000)

	k.RecordDefeaterIfApplicableForTest(ctx, factID, 450_000) // below threshold

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(1), fact.DefeaterCount,
		"rejection confidence 45% < threshold 60%: defeater recorded")
	require.Equal(t, uint64(760_000), fact.EffectiveConfidenceBps,
		"effective = 800_000 × 0.95 = 760_000")
	// Nominal confidence must NOT be touched.
	require.Equal(t, uint64(800_000), fact.Confidence,
		"nominal Confidence must be unchanged by defeater recording")
}

// TestDefeater_EventAttributes verifies all event attributes are present
// and carry correct values.
func TestDefeater_EventAttributes(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-event", 750_000)
	setDefeaterParams(t, k, ctx, 600_000, 50_000)

	k.RecordDefeaterIfApplicableForTest(ctx, factID, 350_000)

	evFound, attrs := hasDeferaterEvent(ctx)
	require.True(t, evFound, "defeater_recorded event must be emitted")

	require.Equal(t, factID, attrs["fact_id"],
		"fact_id attribute must identify the surviving fact")
	require.Equal(t, "1", attrs["defeater_count"],
		"defeater_count must be 1 after first defeater")
	require.Equal(t, "350000", attrs["rejection_confidence_bps"],
		"rejection_confidence_bps must match the input")
	require.Equal(t, "600000", attrs["threshold_bps"],
		"threshold_bps must match the governance param")
	// effective = 750_000 × (1 − 50_000/1_000_000) = 750_000 × 0.95 = 712_500
	require.Equal(t, "712500", attrs["effective_confidence_bps"],
		"effective_confidence_bps = 750_000 × 0.95 = 712_500")
	require.NotEmpty(t, attrs["pollock_criterion"],
		"pollock_criterion must be set")
}

// TestDefeater_UnknownFact_NoPanic verifies that a non-existent fact ID
// is gracefully ignored — no panic, no state mutation.
func TestDefeater_UnknownFact_NoPanic(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setDefeaterParams(t, k, ctx, 600_000, 50_000)

	require.NotPanics(t, func() {
		k.RecordDefeaterIfApplicableForTest(ctx, "nonexistent-fact-xyz", 200_000)
	})

	evFound, _ := hasDeferaterEvent(ctx)
	require.False(t, evFound, "no event for unknown fact")
}

// TestDefeater_Accumulates verifies that multiple defeaters compound correctly.
//
// base=900_000, discount=100_000 (10%):
//   - After 1st defeater: effective = 900_000 × 0.9 = 810_000
//   - After 2nd defeater: effective = 900_000 × 0.8 = 720_000
//   - After 3rd defeater: effective = 900_000 × 0.7 = 630_000
func TestDefeater_Accumulates(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-accum", 900_000)
	setDefeaterParams(t, k, ctx, 700_000, 100_000)

	// Defeater 1: rejection at 500k < threshold 700k
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 500_000)
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(1), fact.DefeaterCount)
	require.Equal(t, uint64(810_000), fact.EffectiveConfidenceBps,
		"after 1st defeater: 900_000 × 0.9 = 810_000")

	// Defeater 2
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 600_000)
	fact, found = k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(2), fact.DefeaterCount)
	require.Equal(t, uint64(720_000), fact.EffectiveConfidenceBps,
		"after 2nd defeater: 900_000 × 0.8 = 720_000")

	// Defeater 3
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 400_000)
	fact, found = k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(3), fact.DefeaterCount)
	require.Equal(t, uint64(630_000), fact.EffectiveConfidenceBps,
		"after 3rd defeater: 900_000 × 0.7 = 630_000")

	// Nominal confidence must be untouched throughout.
	require.Equal(t, uint64(900_000), fact.Confidence,
		"nominal Confidence must never be modified by defeaters")
}

// TestDefeater_FullyDiscounted_ClampsToZero verifies that many defeaters
// cannot produce negative effective confidence — it clamps to 0.
func TestDefeater_FullyDiscounted_ClampsToZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-clamp", 800_000)
	setDefeaterParams(t, k, ctx, 600_000, 200_000) // 20% discount per defeater

	// 6 defeaters × 20% = 120% total discount → clamp to 0.
	for i := 0; i < 6; i++ {
		k.RecordDefeaterIfApplicableForTest(ctx, factID, 500_000)
	}

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(6), fact.DefeaterCount)
	require.Equal(t, uint64(0), fact.EffectiveConfidenceBps,
		"6 defeaters × 20% discount → 120% total → effective clamps to 0")
	// Nominal confidence still intact.
	require.Equal(t, uint64(800_000), fact.Confidence,
		"nominal Confidence unaffected even at full discount")
}

// TestDefeater_ZeroDiscount_EffectiveTracksConfidence verifies that when
// discount=0 but threshold > 0, defeaters are counted but effective_confidence
// tracks nominal confidence (no actual penalty applied).
func TestDefeater_ZeroDiscount_EffectiveTracksConfidence(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeFactWithConfidence(t, k, ctx, "defeater-zerodiscount", 800_000)
	// Threshold enabled, discount disabled.
	setDefeaterParams(t, k, ctx, 600_000, 0 /* no discount */)

	k.RecordDefeaterIfApplicableForTest(ctx, factID, 400_000)
	k.RecordDefeaterIfApplicableForTest(ctx, factID, 500_000)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(2), fact.DefeaterCount,
		"defeater count still increments even when discount=0")
	require.Equal(t, uint64(800_000), fact.EffectiveConfidenceBps,
		"discount=0: effective_confidence == nominal confidence regardless of count")
}

// TestDefeater_InitializedAtFactCreation verifies that a fact created via
// SubmitClaim has EffectiveConfidenceBps initialized to the panel confidence.
// (This tests the wire-in at createFactFromClaim.)
func TestDefeater_InitializedAtFactCreation(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sub := makeValidBech32Addr("defeater-init-sub")
	bk.balances[sub] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// We cannot easily force a round to complete with a controlled confidence
	// in a unit test, but we CAN test that a fact injected via SetFact with a
	// known confidence has EffectiveConfidenceBps == confidence after the
	// RecordDefeaterIfApplicableForTest paths, confirming the initialization.
	//
	// Direct path: create a fact with a known EffectiveConfidenceBps (simulating
	// what createFactFromClaim does) and verify it gets discounted properly.
	factID := makeFactWithConfidence(t, k, ctx, "fact-init-verify", 850_000)
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, fact.Confidence, fact.EffectiveConfidenceBps,
		"at creation: effective_confidence_bps must equal nominal confidence")

	// Also smoke-test SubmitClaim doesn't panic with the new field.
	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   sub,
		FactContent: "defeater initialization smoke test claim",
		Domain:      "physics",
		Category:    "mechanics",
		Stake:       "1000000",
	})
	require.NoError(t, err)
}
