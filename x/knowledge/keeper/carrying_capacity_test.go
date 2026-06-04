package keeper_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Mock OntologyKeeper for stratum depth tests ─────────────────────────────

// capacityMockOntologyKeeper implements types.OntologyKeeper for carrying capacity tests.
type capacityMockOntologyKeeper struct {
	depths map[string]uint32
}

func newCapacityMockOntologyKeeper() *capacityMockOntologyKeeper {
	return &capacityMockOntologyKeeper{depths: make(map[string]uint32)}
}

func (m *capacityMockOntologyKeeper) setDepth(domain string, depth uint32) {
	m.depths[domain] = depth
}

func (m *capacityMockOntologyKeeper) GetDepthForDomain(_ context.Context, domainName string) (uint32, error) {
	d, ok := m.depths[domainName]
	if !ok {
		return 0, fmt.Errorf("domain %s not found", domainName)
	}
	return d, nil
}

func (m *capacityMockOntologyKeeper) GetConfidenceCeiling(_ context.Context, _ string) (uint64, error) {
	return 1_000_000, nil
}

func (m *capacityMockOntologyKeeper) IsValidLogicZone(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (m *capacityMockOntologyKeeper) AcknowledgesIncompleteness(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *capacityMockOntologyKeeper) GetStratumForDomain(_ context.Context, _ string) (string, error) {
	return "empirical", nil
}

func TestDomainStats_SetGet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 5, AtRiskCount: 2, TotalEnergy: 500000, LastUpdated: 100}
	k.SetDomainStats(ctx, &stats)
	got, found := k.GetDomainStats(ctx, "physics")
	require.True(t, found)
	require.Equal(t, uint64(5), got.ActiveCount)
	require.Equal(t, uint64(2), got.AtRiskCount)
	require.Equal(t, uint64(500000), got.TotalEnergy)
	require.Equal(t, uint64(100), got.LastUpdated)
}

func TestDomainStats_GetNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	got, found := k.GetDomainStats(ctx, "nonexistent")
	require.False(t, found)
	require.Equal(t, "nonexistent", got.Domain)
	require.Equal(t, uint64(0), got.ActiveCount)
}

func TestDomainStats_IncrementDecrement(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	k.IncrementDomainFactCount(ctx, "physics", true, 500000)  // active
	k.IncrementDomainFactCount(ctx, "physics", true, 300000)  // active
	k.IncrementDomainFactCount(ctx, "physics", false, 100000) // at-risk
	got, found := k.GetDomainStats(ctx, "physics")
	require.True(t, found)
	require.Equal(t, uint64(2), got.ActiveCount)
	require.Equal(t, uint64(1), got.AtRiskCount)
	require.Equal(t, uint64(900000), got.TotalEnergy)

	k.DecrementDomainFactCount(ctx, "physics", true, 500000)
	got, _ = k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(1), got.ActiveCount)
	require.Equal(t, uint64(400000), got.TotalEnergy)
}

func TestDomainStats_DecrementUnderflow(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Decrement when already at zero — should not underflow
	k.DecrementDomainFactCount(ctx, "physics", true, 100)
	got, _ := k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(0), got.ActiveCount)
	require.Equal(t, uint64(0), got.TotalEnergy)
}

func TestDomainStats_Transition(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	k.IncrementDomainFactCount(ctx, "physics", true, 500000)
	k.IncrementDomainFactCount(ctx, "physics", true, 300000)

	// Transition one from active → at-risk
	k.TransitionDomainFactStatus(ctx, "physics", false)
	got, _ := k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(1), got.ActiveCount)
	require.Equal(t, uint64(1), got.AtRiskCount)

	// Transition it back: at-risk → active
	k.TransitionDomainFactStatus(ctx, "physics", true)
	got, _ = k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(2), got.ActiveCount)
	require.Equal(t, uint64(0), got.AtRiskCount)
}

// ─── Pressure calculation tests ─────────────────────────────────────────────

func TestCarryingCapacity_Base(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	cap := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), cap) // DomainBaseCapacity default
}

func TestDomainPressure_Empty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	pressure := k.GetDomainPressure(ctx, "physics")
	require.Equal(t, uint64(0), pressure) // no facts = zero pressure
}

func TestDomainPressure_AtCapacity(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 1000, AtRiskCount: 0}
	k.SetDomainStats(ctx, &stats)
	pressure := k.GetDomainPressure(ctx, "physics")
	require.Equal(t, uint64(1_000_000), pressure) // exactly at capacity
}

func TestDomainPressure_Overcrowded(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 2000, AtRiskCount: 0}
	k.SetDomainStats(ctx, &stats)
	pressure := k.GetDomainPressure(ctx, "physics")
	require.Equal(t, uint64(2_000_000), pressure) // 2x capacity
}

func TestDomainPressure_HalfCapacity(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 500, AtRiskCount: 0}
	k.SetDomainStats(ctx, &stats)
	pressure := k.GetDomainPressure(ctx, "physics")
	require.Equal(t, uint64(500_000), pressure) // 50%
}

func TestPressureCategory(t *testing.T) {
	require.Equal(t, "sparse", keeper.PressureCategory(100_000))
	require.Equal(t, "sparse", keeper.PressureCategory(0))
	require.Equal(t, "normal", keeper.PressureCategory(500_000))
	require.Equal(t, "crowded", keeper.PressureCategory(900_000))
	require.Equal(t, "crowded", keeper.PressureCategory(1_000_000))
	require.Equal(t, "overcrowded", keeper.PressureCategory(1_500_000))
}

// ─── Birth pressure tests ───────────────────────────────────────────────────

func TestBirthPressure_SparseBonus(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Empty domain = sparse = energy bonus
	params, _ := k.GetParams(ctx)
	energy := k.ApplyBirthPressure(ctx, "physics", params.MetabolismInitialEnergy)
	require.Greater(t, energy, params.MetabolismInitialEnergy)
}

func TestBirthPressure_OvercrowdedNoBonus(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 2000}
	k.SetDomainStats(ctx, &stats)
	params, _ := k.GetParams(ctx)
	energy := k.ApplyBirthPressure(ctx, "physics", params.MetabolismInitialEnergy)
	require.Equal(t, params.MetabolismInitialEnergy, energy) // no bonus when overcrowded
}

func TestBirthPressure_AtCapacityNoBonus(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 1000}
	k.SetDomainStats(ctx, &stats)
	params, _ := k.GetParams(ctx)
	energy := k.ApplyBirthPressure(ctx, "physics", params.MetabolismInitialEnergy)
	require.Equal(t, params.MetabolismInitialEnergy, energy) // no bonus at exact capacity
}

func TestBirthPressure_HalfCapacity(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 500}
	k.SetDomainStats(ctx, &stats)
	params, _ := k.GetParams(ctx)
	energy := k.ApplyBirthPressure(ctx, "physics", params.MetabolismInitialEnergy)
	// 50% sparseness, 20% bonus cap → ~10% bonus
	require.Greater(t, energy, params.MetabolismInitialEnergy)
	// But less than full sparse bonus
	fullSparseEnergy := k.ApplyBirthPressure(ctx, "theology", params.MetabolismInitialEnergy) // empty domain
	require.Less(t, energy, fullSparseEnergy)
}

// ─── Death pressure tests ───────────────────────────────────────────────────

func TestDeathPressure_Overcrowded(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 2000}
	k.SetDomainStats(ctx, &stats)
	multiplier := k.GetDeathPressureMultiplier(ctx, "physics")
	require.Greater(t, multiplier, uint64(keeper.BPSCapacity)) // > 100% = faster decay
}

func TestDeathPressure_Sparse(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 100}
	k.SetDomainStats(ctx, &stats)
	multiplier := k.GetDeathPressureMultiplier(ctx, "physics")
	require.Less(t, multiplier, uint64(keeper.BPSCapacity)) // < 100% = slower decay
}

func TestDeathPressure_Normal(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 600}
	k.SetDomainStats(ctx, &stats)
	multiplier := k.GetDeathPressureMultiplier(ctx, "physics")
	require.Equal(t, uint64(keeper.BPSCapacity), multiplier) // 100% = normal
}

func TestDeathPressure_Empty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Empty domain = 0 pressure < 50% threshold → 75% decay
	multiplier := k.GetDeathPressureMultiplier(ctx, "physics")
	require.Equal(t, uint64(750_000), multiplier) // 75% = slower decay
}

// ─── Integration test ───────────────────────────────────────────────────────

func TestCarryingCapacity_Integration(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	params, _ := k.GetParams(ctx)

	// 1. Create 1200 facts in "physics" (over capacity of 1000)
	for i := 0; i < 1200; i++ {
		factID := fmt.Sprintf("fact-%04d", i)
		fact := &types.Fact{
			Id:                factID,
			Content:           fmt.Sprintf("Physics fact number %d with enough content", i),
			Domain:            "physics",
			Category:          "observation",
			Confidence:        500_000,
			Submitter:         "zrn1test",
			SubmittedAtBlock:  100,
			VerifiedAtBlock:   100,
			LastVerifiedBlock: 100,
			Status:            types.FactStatus_FACT_STATUS_ACTIVE,
			Energy:            params.MetabolismInitialEnergy,
			EnergyCap:         params.MetabolismEnergyCap,
		}
		require.NoError(t, k.SetFact(ctx, fact))
		k.IncrementDomainFactCount(ctx, fact.Domain, true, fact.Energy)
	}

	// 2. Verify pressure > 1M BPS (overcrowded)
	pressure := k.GetDomainPressure(ctx, "physics")
	require.Greater(t, pressure, uint64(keeper.BPSCapacity))
	require.Equal(t, "overcrowded", keeper.PressureCategory(pressure))

	// 3. Verify death pressure multiplier > 1M (faster decay)
	multiplier := k.GetDeathPressureMultiplier(ctx, "physics")
	require.Greater(t, multiplier, uint64(keeper.BPSCapacity))

	// 4. Verify birth pressure gives no bonus
	energy := k.ApplyBirthPressure(ctx, "physics", params.MetabolismInitialEnergy)
	require.Equal(t, params.MetabolismInitialEnergy, energy)

	// 5. Create fact in sparse domain — verify bonus energy
	sparseEnergy := k.ApplyBirthPressure(ctx, "theology", params.MetabolismInitialEnergy)
	require.Greater(t, sparseEnergy, params.MetabolismInitialEnergy)

	// 6. Verify stats are correct
	stats, found := k.GetDomainStats(ctx, "physics")
	require.True(t, found)
	require.Equal(t, uint64(1200), stats.ActiveCount)
	require.Equal(t, uint64(0), stats.AtRiskCount)

	// 7. Verify capacity
	cap := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), cap)

	// 8. Verify sparse domain
	sparseStats, found := k.GetDomainStats(ctx, "theology")
	require.False(t, found) // not yet populated
	require.Equal(t, uint64(0), sparseStats.ActiveCount)
	sparsePressure := k.GetDomainPressure(ctx, "theology")
	require.Equal(t, uint64(0), sparsePressure)
	require.Equal(t, "sparse", keeper.PressureCategory(sparsePressure))
}

// ─── R31-4: Metal→Wood stratum depth constrains carrying capacity ───────────

func TestStratumCapacity_NoOntologyKeeper(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Without ontology keeper, capacity should be unchanged (base default)
	cap := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), cap) // DomainBaseCapacity default, no reduction
}

func TestStratumCapacity_Depth1_FullCapacity(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("physics", 1) // root domain
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), cap) // 100% of base — no reduction for depth 1
}

func TestStratumCapacity_Depth2_80Percent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("quantum_physics", 2)
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "quantum_physics")
	// 1000 * 800_000 / 1_000_000 = 800
	require.Equal(t, uint64(800), cap)
}

func TestStratumCapacity_Depth3_60Percent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("quantum_chromodynamics", 3)
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "quantum_chromodynamics")
	// 1000 * 600_000 / 1_000_000 = 600
	require.Equal(t, uint64(600), cap)
}

func TestStratumCapacity_Depth4_50PercentFloor(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("deep_subdomain", 4)
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "deep_subdomain")
	// 1000 * 500_000 / 1_000_000 = 500
	require.Equal(t, uint64(500), cap)
}

func TestStratumCapacity_Depth5_50PercentFloor(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("very_deep", 5) // max depth — same 50% floor as depth 4
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "very_deep")
	// 1000 * 500_000 / 1_000_000 = 500
	require.Equal(t, uint64(500), cap)
}

func TestStratumCapacity_ErrorFallback(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	// Don't set depth for "unknown_domain" — GetDepthForDomain returns error
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "unknown_domain")
	require.Equal(t, uint64(1000), cap) // Falls back to full capacity on error
}

func TestStratumCapacity_PressureAffectedByDepth(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("deep_domain", 3) // 60% capacity → 600 effective capacity
	k.SetOntologyKeeper(ok)

	// Set 600 active facts — should be exactly at capacity for depth-3 domain
	stats := keeper.DomainStats{Domain: "deep_domain", ActiveCount: 600}
	k.SetDomainStats(ctx, &stats)

	pressure := k.GetDomainPressure(ctx, "deep_domain")
	require.Equal(t, uint64(1_000_000), pressure) // exactly at capacity

	// Same 600 facts in a root domain (depth 1, capacity 1000) = only 60% pressure
	ok.setDepth("root_domain", 1)
	stats2 := keeper.DomainStats{Domain: "root_domain", ActiveCount: 600}
	k.SetDomainStats(ctx, &stats2)

	pressure2 := k.GetDomainPressure(ctx, "root_domain")
	require.Equal(t, uint64(600_000), pressure2) // 60% of capacity
}

func TestStratumCapacity_DeepDomainOvercrowdsFaster(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("shallow", 1) // capacity: 1000
	ok.setDepth("deep", 3)    // capacity: 600
	k.SetOntologyKeeper(ok)

	// 800 facts in each domain
	for _, domain := range []string{"shallow", "deep"} {
		stats := keeper.DomainStats{Domain: domain, ActiveCount: 800}
		k.SetDomainStats(ctx, &stats)
	}

	shallowPressure := k.GetDomainPressure(ctx, "shallow")
	deepPressure := k.GetDomainPressure(ctx, "deep")

	// Shallow: 800/1000 = 80% — normal
	require.Equal(t, uint64(800_000), shallowPressure)
	require.Equal(t, "crowded", keeper.PressureCategory(shallowPressure))

	// Deep: 800/600 > 100% — overcrowded
	require.Greater(t, deepPressure, uint64(keeper.BPSCapacity))
	require.Equal(t, "overcrowded", keeper.PressureCategory(deepPressure))
}

func TestStratumCapacity_EventEmitted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("deep_domain", 3)
	k.SetOntologyKeeper(ok)

	// Call GetDomainCarryingCapacity — should emit stratum_capacity_applied event
	cap := k.GetDomainCarryingCapacity(ctx, "deep_domain")
	require.Equal(t, uint64(600), cap)

	// Check that the event was emitted
	events := ctx.EventManager().Events()
	found := false
	for _, event := range events {
		if event.Type == "zerone.knowledge.stratum_capacity_applied" {
			found = true
			attrs := make(map[string]string)
			for _, attr := range event.Attributes {
				attrs[attr.Key] = attr.Value
			}
			require.Equal(t, "deep_domain", attrs["domain"])
			require.Equal(t, "3", attrs["stratum_depth"])
			require.Equal(t, "600000", attrs["capacity_multiplier_bps"])
			require.Equal(t, "600", attrs["effective_capacity"])
		}
	}
	require.True(t, found, "stratum_capacity_applied event should be emitted for depth > 1")
}

func TestStratumCapacity_NoEventForDepth1(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ok := newCapacityMockOntologyKeeper()
	ok.setDepth("root_domain", 1)
	k.SetOntologyKeeper(ok)

	cap := k.GetDomainCarryingCapacity(ctx, "root_domain")
	require.Equal(t, uint64(1000), cap)

	// No event should be emitted for depth 1 (no reduction applied)
	events := ctx.EventManager().Events()
	for _, event := range events {
		require.NotEqual(t, "zerone.knowledge.stratum_capacity_applied", event.Type,
			"no event should be emitted when depth=1 (no capacity reduction)")
	}
}

// ─── Capture penalty tests (R31-1: Metal controls Wood) ────────────────────

type mockCaptureDefenseForCapacity struct {
	flagged   map[string]bool
	penalties map[string]uint64
}

func (m *mockCaptureDefenseForCapacity) RecordVerificationHistory(_ context.Context, _, _ string, _ []string, _ []bool, _ []uint64) {
}
func (m *mockCaptureDefenseForCapacity) UpdateReputation(_ context.Context, _, _, _ string, _ bool) {
}
func (m *mockCaptureDefenseForCapacity) GetDomainCapturePenalty(_ context.Context, domain string) (bool, uint64) {
	if m.flagged[domain] {
		return true, m.penalties[domain]
	}
	return false, 0
}

func TestCarryingCapacity_CapturePenalty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	mockCD := &mockCaptureDefenseForCapacity{
		flagged:   map[string]bool{"physics": true},
		penalties: map[string]uint64{"physics": 400_000},
	}
	k.SetCaptureDefenseKeeper(mockCD)

	// Base capacity is 1000 (default), no citations = no bonus
	// Penalty: 1000 * 400_000 / 1_000_000 = 400
	// Effective: 1000 - 400 = 600
	capacity := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(600), capacity)
}

func TestCarryingCapacity_CapturePenaltyMonopoly(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	mockCD := &mockCaptureDefenseForCapacity{
		flagged:   map[string]bool{"physics": true},
		penalties: map[string]uint64{"physics": 1_000_000},
	}
	k.SetCaptureDefenseKeeper(mockCD)

	// Full penalty would be 1000 * 1_000_000 / 1_000_000 = 1000
	// But minimum capacity is 1
	capacity := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1), capacity)
}

func TestCarryingCapacity_NoPenaltyWhenNotFlagged(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	mockCD := &mockCaptureDefenseForCapacity{
		flagged:   map[string]bool{},
		penalties: map[string]uint64{},
	}
	k.SetCaptureDefenseKeeper(mockCD)

	capacity := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), capacity)
}

func TestCarryingCapacity_UnflagRestoresCapacity(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	mockCD := &mockCaptureDefenseForCapacity{
		flagged:   map[string]bool{"physics": true},
		penalties: map[string]uint64{"physics": 400_000},
	}
	k.SetCaptureDefenseKeeper(mockCD)

	require.Equal(t, uint64(600), k.GetDomainCarryingCapacity(ctx, "physics"))

	mockCD.flagged["physics"] = false

	require.Equal(t, uint64(1000), k.GetDomainCarryingCapacity(ctx, "physics"))
}

func TestCarryingCapacity_CapturePenaltyEvent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	mockCD := &mockCaptureDefenseForCapacity{
		flagged:   map[string]bool{"physics": true},
		penalties: map[string]uint64{"physics": 400_000},
	}
	k.SetCaptureDefenseKeeper(mockCD)

	k.EmitCapacityPenaltyEvent(ctx, "physics")

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	events := sdkCtx.EventManager().Events()
	found := false
	for _, e := range events {
		if e.Type == "zerone.knowledge.capacity_penalty_applied" {
			found = true
			for _, attr := range e.Attributes {
				switch attr.Key {
				case "domain":
					require.Equal(t, "physics", attr.Value)
				case "base_capacity":
					require.Equal(t, "1000", attr.Value)
				case "effective_capacity":
					require.Equal(t, "600", attr.Value)
				case "capture_penalty_bps":
					require.Equal(t, "400000", attr.Value)
				case "reason":
					require.Equal(t, "capture_flagged", attr.Value)
				}
			}
		}
	}
	require.True(t, found, "expected capacity_penalty_applied event")
}

// ─── Pending verification ratio tests (R31-1: Wood controls Earth) ──────────

func TestGetPendingVerificationRatio_NoClaims(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ratio := k.GetPendingVerificationRatio(ctx)
	require.Equal(t, uint64(1_000_000), ratio) // both zero → 1:1
}

func TestGetPendingVerificationRatio_OnlyPending(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, k.SetClaim(ctx, &types.Claim{
			Id:     fmt.Sprintf("claim-%d", i),
			Status: types.ClaimStatus_CLAIM_STATUS_PENDING,
			Domain: "physics",
		}))
	}
	ratio := k.GetPendingVerificationRatio(ctx)
	require.Equal(t, uint64(2_000_000), ratio) // capped at 200%
}

func TestGetPendingVerificationRatio_HealthyRatio(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	for i := 0; i < 2; i++ {
		require.NoError(t, k.SetClaim(ctx, &types.Claim{
			Id:     fmt.Sprintf("pending-%d", i),
			Status: types.ClaimStatus_CLAIM_STATUS_PENDING,
			Domain: "physics",
		}))
	}
	for i := 0; i < 10; i++ {
		require.NoError(t, k.SetClaim(ctx, &types.Claim{
			Id:     fmt.Sprintf("active-%d", i),
			Status: types.ClaimStatus_CLAIM_STATUS_ACCEPTED,
			Domain: "physics",
		}))
	}
	ratio := k.GetPendingVerificationRatio(ctx)
	require.Equal(t, uint64(200_000), ratio) // 2/10 = 20%
}

func TestStratumCapacity_AllDepthMultipliers(t *testing.T) {
	tests := []struct {
		depth    uint32
		expected uint64 // expected BPS multiplier
	}{
		{0, 1_000_000}, // depth 0 treated as <= 1
		{1, 1_000_000}, // root: 100%
		{2, 800_000},   // 80%
		{3, 600_000},   // 60%
		{4, 500_000},   // 50% floor
		{5, 500_000},   // 50% floor
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("depth_%d", tt.depth), func(t *testing.T) {
			k, ctx := setupKnowledgeTest(t)
			ok := newCapacityMockOntologyKeeper()
			domain := fmt.Sprintf("domain_d%d", tt.depth)
			ok.setDepth(domain, tt.depth)
			k.SetOntologyKeeper(ok)

			cap := k.GetDomainCarryingCapacity(ctx, domain)
			expectedCap := uint64(1000) * tt.expected / keeper.BPSCapacity
			require.Equal(t, expectedCap, cap,
				"depth %d: expected capacity %d, got %d", tt.depth, expectedCap, cap)
		})
	}
}

// ─── R31-5: Mentorship graduations (Water → Wood) ───────────────────────────

func TestDomainStats_MentorshipGraduations(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	stats := keeper.DomainStats{
		Domain:                "philosophy",
		ActiveCount:           10,
		MentorshipGraduations: 3,
		TotalEnergy:           100000,
		LastUpdated:           50,
	}
	k.SetDomainStats(ctx, &stats)

	got, found := k.GetDomainStats(ctx, "philosophy")
	require.True(t, found)
	require.Equal(t, uint64(3), got.MentorshipGraduations)
}

func TestApplyMentorshipDividend(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	stats := keeper.DomainStats{Domain: "physics", ActiveCount: 5, TotalEnergy: 100000}
	k.SetDomainStats(ctx, &stats)

	k.ApplyMentorshipDividend(ctx, "physics", "mentor1", "mentee1")

	got, found := k.GetDomainStats(ctx, "physics")
	require.True(t, found)
	require.Equal(t, uint64(1), got.MentorshipGraduations)
	require.Equal(t, uint64(150000), got.TotalEnergy)

	k.ApplyMentorshipDividend(ctx, "physics", "mentor2", "mentee2")
	got, _ = k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(2), got.MentorshipGraduations)
	require.Equal(t, uint64(200000), got.TotalEnergy)
}

func TestApplyMentorshipDividend_EnergyCap(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	stats := keeper.DomainStats{Domain: "physics", TotalEnergy: 990000}
	k.SetDomainStats(ctx, &stats)

	k.ApplyMentorshipDividend(ctx, "physics", "mentor1", "mentee1")

	got, _ := k.GetDomainStats(ctx, "physics")
	require.Equal(t, uint64(1000000), got.TotalEnergy)
}

func TestGetDomainCarryingCapacity_MentorshipBonus(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	baseCapacity := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1000), baseCapacity)

	stats := keeper.DomainStats{Domain: "physics", MentorshipGraduations: 3}
	k.SetDomainStats(ctx, &stats)

	// 1000 + 3 * 5 = 1015
	capacity := k.GetDomainCarryingCapacity(ctx, "physics")
	require.Equal(t, uint64(1015), capacity)
}

// ─── Base Rate Tracking (Calibrated Forecasting Gap 3) ───────────────────────

func TestRecordDomainVerdict_AcceptIncrementsAccepted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.RecordDomainVerdict(ctx, "physics", true)

	stats, found := k.GetDomainStats(ctx, "physics")
	require.True(t, found)
	require.Equal(t, uint64(1), stats.DomainAccepted)
	require.Equal(t, uint64(0), stats.DomainRejected)
}

func TestRecordDomainVerdict_RejectIncrementsRejected(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.RecordDomainVerdict(ctx, "biology", false)

	stats, found := k.GetDomainStats(ctx, "biology")
	require.True(t, found)
	require.Equal(t, uint64(0), stats.DomainAccepted)
	require.Equal(t, uint64(1), stats.DomainRejected)
}

func TestRecordDomainVerdict_Cumulative(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.RecordDomainVerdict(ctx, "math", true)
	k.RecordDomainVerdict(ctx, "math", true)
	k.RecordDomainVerdict(ctx, "math", false)

	stats, _ := k.GetDomainStats(ctx, "math")
	require.Equal(t, uint64(2), stats.DomainAccepted)
	require.Equal(t, uint64(1), stats.DomainRejected)
}

func TestRecordDomainVerdict_EmptyDomainSkipped(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Should not panic — empty domain is a no-op
	k.RecordDomainVerdict(ctx, "", true)

	// No records created
	_, found := k.GetDomainStats(ctx, "")
	require.False(t, found)
}

func TestDomainAcceptanceRateBps_NoVerdicts(t *testing.T) {
	stats := &keeper.DomainStats{Domain: "empty"}
	rate := keeper.DomainAcceptanceRateBps(stats)
	require.Equal(t, uint64(0), rate)
}

func TestDomainAcceptanceRateBps_AllAccepted(t *testing.T) {
	stats := &keeper.DomainStats{Domain: "d", DomainAccepted: 10, DomainRejected: 0}
	rate := keeper.DomainAcceptanceRateBps(stats)
	require.Equal(t, uint64(1_000_000), rate) // 100%
}

func TestDomainAcceptanceRateBps_AllRejected(t *testing.T) {
	stats := &keeper.DomainStats{Domain: "d", DomainAccepted: 0, DomainRejected: 5}
	rate := keeper.DomainAcceptanceRateBps(stats)
	require.Equal(t, uint64(0), rate) // 0%
}

func TestDomainAcceptanceRateBps_Mixed(t *testing.T) {
	// 3 accepted, 1 rejected → 75%
	stats := &keeper.DomainStats{Domain: "d", DomainAccepted: 3, DomainRejected: 1}
	rate := keeper.DomainAcceptanceRateBps(stats)
	require.Equal(t, uint64(750_000), rate) // 75%
}

func TestRecordDomainVerdict_DoesNotAffectOtherFields(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Pre-seed some carrying capacity stats
	k.IncrementDomainFactCount(ctx, "chemistry", true, 100)
	k.IncrementDomainFactCount(ctx, "chemistry", false, 50)

	k.RecordDomainVerdict(ctx, "chemistry", true)

	stats, _ := k.GetDomainStats(ctx, "chemistry")
	require.Equal(t, uint64(1), stats.ActiveCount)
	require.Equal(t, uint64(1), stats.AtRiskCount)
	require.Equal(t, uint64(150), stats.TotalEnergy)
	require.Equal(t, uint64(1), stats.DomainAccepted)
}

func TestDomainCapacityQuery_ExposesBaseRate(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	k.RecordDomainVerdict(ctx, "physics", true)
	k.RecordDomainVerdict(ctx, "physics", true)
	k.RecordDomainVerdict(ctx, "physics", false)

	stats, _ := k.GetDomainStats(ctx, "physics")
	rate := keeper.DomainAcceptanceRateBps(stats)

	// 2 accepted / 3 total = 66.666...% → 666_666 BPS
	require.Equal(t, uint64(666_666), rate)
	require.Equal(t, uint64(2), stats.DomainAccepted)
	require.Equal(t, uint64(1), stats.DomainRejected)
}
