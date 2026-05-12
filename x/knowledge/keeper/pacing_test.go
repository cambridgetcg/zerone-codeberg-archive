package keeper_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// ─── Mock PacingKeeper ──────────────────────────────────────────────────────

type mockPacingKeeper struct {
	creationBps uint64
	analysisBps uint64
}

func (m *mockPacingKeeper) GetGlobalPacingMultiplier(_ context.Context) (creationBps, analysisBps uint64) {
	return m.creationBps, m.analysisBps
}

// ─── GetEffectiveCooldown tests ─────────────────────────────────────────────

func TestPacing_EffectiveCooldown_NoPacingKeeper(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Without pacing keeper, cooldown should equal the base param (50).
	cooldown := k.GetEffectiveCooldown(ctx, "physics")
	require.Equal(t, uint64(50), cooldown, "no pacing keeper should return base cooldown")
}

func TestPacing_EffectiveCooldown_HealthyNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pk := &mockPacingKeeper{creationBps: 1_000_000, analysisBps: 1_000_000}
	k.SetPacingKeeper(pk)

	cooldown := k.GetEffectiveCooldown(ctx, "physics")
	require.Equal(t, uint64(50), cooldown, "healthy pacing should return base cooldown")
}

func TestPacing_EffectiveCooldown_DegradedNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// 75% pacing → cooldown = 50 * 1_000_000 / 750_000 = 66
	pk := &mockPacingKeeper{creationBps: 750_000, analysisBps: 750_000}
	k.SetPacingKeeper(pk)

	cooldown := k.GetEffectiveCooldown(ctx, "physics")
	require.Equal(t, uint64(66), cooldown, "degraded pacing (75%%) should increase cooldown to 66")
}

func TestPacing_EffectiveCooldown_CriticalNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// 50% pacing → cooldown = 50 * 1_000_000 / 500_000 = 100
	pk := &mockPacingKeeper{creationBps: 500_000, analysisBps: 500_000}
	k.SetPacingKeeper(pk)

	cooldown := k.GetEffectiveCooldown(ctx, "physics")
	require.Equal(t, uint64(100), cooldown, "critical pacing (50%%) should double cooldown to 100")
}

func TestPacing_EffectiveCooldown_DomainPressure(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pk := &mockPacingKeeper{creationBps: 1_000_000, analysisBps: 1_000_000}
	k.SetPacingKeeper(pk)

	// Overcrowd the physics domain: add more facts than base capacity (1000).
	for i := 0; i < 1500; i++ {
		k.IncrementDomainFactCount(ctx, "physics", true, 500_000)
	}

	cooldown := k.GetEffectiveCooldown(ctx, "physics")
	require.Greater(t, cooldown, uint64(50),
		"overcrowded domain should increase cooldown above base")
}

// ─── GetEffectiveMinReviewFee tests ─────────────────────────────────────────

func TestPacing_EffectiveMinReviewFee_NoPacingKeeper(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "no pacing keeper should return base fee")
}

func TestPacing_EffectiveMinReviewFee_HealthyNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pk := &mockPacingKeeper{creationBps: 1_000_000, analysisBps: 1_000_000}
	k.SetPacingKeeper(pk)

	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "healthy pacing should return base fee")
}

func TestPacing_EffectiveMinReviewFee_DegradedNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// 75% pacing → fee = 100000 * 1_000_000 / 750_000 = 133333
	pk := &mockPacingKeeper{creationBps: 750_000, analysisBps: 750_000}
	k.SetPacingKeeper(pk)

	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "133333", fee, "degraded pacing (75%%) should increase fee to 133333")
}

func TestPacing_EffectiveMinReviewFee_CriticalNetwork(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// 50% pacing → fee = 100000 * 1_000_000 / 500_000 = 200000
	pk := &mockPacingKeeper{creationBps: 500_000, analysisBps: 500_000}
	k.SetPacingKeeper(pk)

	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "200000", fee, "critical pacing (50%%) should double fee to 200000")
}

// ─── LastClaimHeight tests ──────────────────────────────────────────────────

func TestPacing_LastClaimHeight_RoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	submitter := "zrn1submitter_pacing"

	// Initially zero
	height := k.GetLastClaimHeight(ctx, submitter)
	require.Equal(t, uint64(0), height, "should return 0 for unknown submitter")

	// Set and retrieve
	k.SetLastClaimHeight(ctx, submitter, 42)
	height = k.GetLastClaimHeight(ctx, submitter)
	require.Equal(t, uint64(42), height, "should return 42 after set")

	// Overwrite
	k.SetLastClaimHeight(ctx, submitter, 999)
	height = k.GetLastClaimHeight(ctx, submitter)
	require.Equal(t, uint64(999), height, "should return 999 after overwrite")
}

func TestPacing_LastClaimHeight_IsolatedPerSubmitter(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.SetLastClaimHeight(ctx, "alice", 100)
	k.SetLastClaimHeight(ctx, "bob", 200)

	require.Equal(t, uint64(100), k.GetLastClaimHeight(ctx, "alice"))
	require.Equal(t, uint64(200), k.GetLastClaimHeight(ctx, "bob"))
	require.Equal(t, uint64(0), k.GetLastClaimHeight(ctx, "carol"), "unknown submitter should be 0")
}

// ─── ActiveRoundsCount tests ────────────────────────────────────────────────

func TestCongestion_ActiveRoundsCount_InitialZero(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	require.Equal(t, uint64(0), k.GetActiveRoundsCount(ctx), "initial active rounds should be zero")
}

func TestCongestion_ActiveRoundsCount_IncrementDecrement(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.IncrementActiveRoundsCount(ctx)
	k.IncrementActiveRoundsCount(ctx)
	k.IncrementActiveRoundsCount(ctx)
	require.Equal(t, uint64(3), k.GetActiveRoundsCount(ctx), "three increments should give 3")

	k.DecrementActiveRoundsCount(ctx)
	require.Equal(t, uint64(2), k.GetActiveRoundsCount(ctx), "one decrement should give 2")
}

func TestCongestion_ActiveRoundsCount_SaturatesAtZero(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Decrement when already zero — should not underflow.
	k.DecrementActiveRoundsCount(ctx)
	require.Equal(t, uint64(0), k.GetActiveRoundsCount(ctx), "decrement from zero should stay zero (no underflow)")

	k.IncrementActiveRoundsCount(ctx)
	k.DecrementActiveRoundsCount(ctx)
	k.DecrementActiveRoundsCount(ctx) // extra
	require.Equal(t, uint64(0), k.GetActiveRoundsCount(ctx), "extra decrement should saturate at zero")
}

func TestCongestion_ActiveRoundsCount_SetRoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.SetActiveRoundsCount(ctx, 42)
	require.Equal(t, uint64(42), k.GetActiveRoundsCount(ctx))

	k.SetActiveRoundsCount(ctx, 0)
	require.Equal(t, uint64(0), k.GetActiveRoundsCount(ctx))
}

// ─── GetEffectiveMinReviewFee — congestion layer tests ────────────────────────

func TestCongestion_EffectiveMinReviewFee_Disabled_MultiplierZero(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// CongestionMultiplierBps=0 (default) → no congestion pricing regardless of active rounds.
	p, _ := k.GetParams(ctx)
	p.CongestionMultiplierBps = 0
	p.CongestionThresholdRounds = 10
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 500) // well above threshold
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "zero multiplier should disable congestion pricing")
}

func TestCongestion_EffectiveMinReviewFee_Disabled_ThresholdZero(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// CongestionThresholdRounds=0 → disabled even if multiplier is set.
	p, _ := k.GetParams(ctx)
	p.CongestionMultiplierBps = 10_000
	p.CongestionThresholdRounds = 0
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 500)
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "zero threshold should disable congestion pricing")
}

func TestCongestion_EffectiveMinReviewFee_BelowThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, _ := k.GetParams(ctx)
	p.CongestionMultiplierBps = 10_000 // 1% per excess round
	p.CongestionThresholdRounds = 100
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 99) // one below threshold
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "below threshold: no congestion premium")
}

func TestCongestion_EffectiveMinReviewFee_AtThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, _ := k.GetParams(ctx)
	p.CongestionMultiplierBps = 10_000
	p.CongestionThresholdRounds = 100
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 100) // exactly at threshold — no excess
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "100000", fee, "at threshold: no congestion premium (excess=0)")
}

func TestCongestion_EffectiveMinReviewFee_OneAboveThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, _ := k.GetParams(ctx)
	// 1% per excess round (10_000 / 1_000_000 = 1%)
	p.CongestionMultiplierBps = 10_000
	p.CongestionThresholdRounds = 100
	_ = k.SetParams(ctx, p)

	// excess=1 → fee = 100000 + 100000 × 1 × 10_000 / 1_000_000 = 100000 + 1000 = 101000
	k.SetActiveRoundsCount(ctx, 101)
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "101000", fee, "1 above threshold with 1%% multiplier → 101000")
}

func TestCongestion_EffectiveMinReviewFee_FiftyAboveThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, _ := k.GetParams(ctx)
	// 2% per excess round (20_000 / 1_000_000 = 2%)
	// excess=50 → fee = 100000 × (1 + 50×20_000/1_000_000) = 100000 × 2.0 = 200000
	p.CongestionMultiplierBps = 20_000
	p.CongestionThresholdRounds = 100
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 150) // excess = 50
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "200000", fee, "50 excess rounds at 2%% multiplier should double the fee")
}

func TestCongestion_EffectiveMinReviewFee_CombinedWithPacing(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Pacing at 75% → fee = 100000 * 1_000_000 / 750_000 = 133333 (base after pacing)
	// Then congestion: threshold=100, active=110, multiplier=10_000 (1%), excess=10
	// congestion addend = 133333 × 10 × 10_000 / 1_000_000 = 133333 × 0.1 = 13333
	// final fee = 133333 + 13333 = 146666
	pk := &mockPacingKeeper{creationBps: 750_000, analysisBps: 1_000_000}
	k.SetPacingKeeper(pk)

	p, _ := k.GetParams(ctx)
	p.CongestionMultiplierBps = 10_000
	p.CongestionThresholdRounds = 100
	_ = k.SetParams(ctx, p)

	k.SetActiveRoundsCount(ctx, 110) // excess = 10
	fee := k.GetEffectiveMinReviewFee(ctx)
	require.Equal(t, "146666", fee, "pacing (75%%) + congestion (10 excess rounds at 1%%) should compound")
}
