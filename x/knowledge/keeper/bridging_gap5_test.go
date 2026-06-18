package keeper_test

// Structured Deliberation Gap 5 — High-Confidence Bridging Requirement.
//
// Habermas Machine / Fishkin deliberative polling insight: single-cluster
// agreement cannot certify high epistemic confidence. The Habermas Machine
// (Tessler et al., Science 2024) shows that AI-mediated cross-cluster consensus
// statements receive higher endorsement and reduce polarization — because they
// bridge organizational perspective gaps. Fishkin's deliberative polling shows
// that opinion change under structured exposure to opposing views is the signal
// of genuine deliberation.
//
// On-chain encoding:
//
//	When a fact is accepted, its bridging_org_count (Gap 4) tracks how many
//	distinct organizational clusters agreed. Gap 5 adds a confidence gate:
//	when high_confidence_bridging_min_orgs > 0, facts at or above
//	high_confidence_bridging_threshold_bps have their confidence capped to
//	(threshold - 1) unless bridging_org_count >= min_orgs.
//
//	The chain enforces: "you cannot be 90%-confident in something that only
//	one organizational cluster agreed on."
//
// Tests:
//
//  1. Disabled_ConfidenceUncapped — min_orgs=0 → cap never fires regardless of confidence
//  2. ThresholdBelowConfidence_NoFire — confidence below threshold → cap does not fire
//  3. ThresholdMet_CapApplied — confidence at threshold + insufficient bridging → capped
//  4. SufficientBridging_NoCap — bridging_org_count >= min_orgs → confidence unchanged
//  5. ExactThreshold_CapApplied — confidence == threshold exactly → capped
//  6. OneBelow_NoCap — confidence == threshold-1 → cap does not fire
//  7. EventEmitted_WhenCapped — cap applied → zerone.knowledge.high_confidence_bridging_cap_applied event
//  8. NoEvent_WhenNotCapped — sufficient bridging → no cap event
//  9. Validate_ThresholdZeroWhenEnabled — min_orgs > 0 but threshold = 0 → Validate() error
// 10. Validate_ThresholdAboveMillion — threshold > 1_000_000 → Validate() error
// 11. Validate_Valid — valid enabled config → no error

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── pure unit tests for maybeCapHighConfidenceBridging ───────────────────────

func TestHighConfidenceBridgingGap5_Disabled_ConfidenceUncapped(t *testing.T) {
	// min_orgs = 0 → gate disabled; confidence 950k (above any threshold) → unchanged.
	fact := &types.Fact{Confidence: 950_000}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      0, // disabled
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 1 /* one org, insufficient */, params)
	require.False(t, capped, "gate disabled — cap should not fire")
	require.Equal(t, uint64(950_000), fact.Confidence, "confidence should be unchanged")
}

func TestHighConfidenceBridgingGap5_ThresholdBelowConfidence_NoFire(t *testing.T) {
	// confidence 800k < threshold 900k → below threshold, cap does not fire.
	fact := &types.Fact{Confidence: 800_000}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      3,
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 1, params)
	require.False(t, capped)
	require.Equal(t, uint64(800_000), fact.Confidence)
}

func TestHighConfidenceBridgingGap5_ThresholdMet_CapApplied(t *testing.T) {
	// confidence 950k >= threshold 900k, bridging count 1 < min_orgs 3 → cap to 899999.
	fact := &types.Fact{Confidence: 950_000}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      3,
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 1, params)
	require.True(t, capped, "cap should fire")
	require.Equal(t, uint64(899_999), fact.Confidence, "confidence capped to threshold-1")
}

func TestHighConfidenceBridgingGap5_SufficientBridging_NoCap(t *testing.T) {
	// confidence 950k >= threshold, but bridging count 3 >= min_orgs 3 → no cap.
	fact := &types.Fact{Confidence: 950_000}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      3,
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 3, params)
	require.False(t, capped, "sufficient bridging — cap should not fire")
	require.Equal(t, uint64(950_000), fact.Confidence, "confidence unchanged")
}

func TestHighConfidenceBridgingGap5_ExactThreshold_CapApplied(t *testing.T) {
	// confidence == threshold exactly → >= condition → capped.
	fact := &types.Fact{Confidence: 900_000}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      2,
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 1, params)
	require.True(t, capped)
	require.Equal(t, uint64(899_999), fact.Confidence)
}

func TestHighConfidenceBridgingGap5_OneBelow_NoCap(t *testing.T) {
	// confidence = threshold - 1 → strictly below → no cap.
	fact := &types.Fact{Confidence: 899_999}
	params := types.Params{
		HighConfidenceBridgingMinOrgs:      2,
		HighConfidenceBridgingThresholdBps: 900_000,
	}
	capped := keeper.MaybeCapHighConfidenceBridgingForTest(fact, 0, params)
	require.False(t, capped)
	require.Equal(t, uint64(899_999), fact.Confidence)
}

// ── integration tests via RecordBridgingOrgCountForTest ──────────────────────

func TestHighConfidenceBridgingGap5_EventEmitted_WhenCapped(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Set up: fact with high confidence, single-org round, min_orgs=2 threshold=900k.
	factID := makeBridgingFact(t, k, ctx, "hcb-fact-1")

	// Give the fact high confidence before calling recordBridgingOrgCount.
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	fact.Confidence = 950_000
	require.NoError(t, k.SetFact(ctx, fact))

	setBridgingOrg(t, k, ctx, makeValidBech32Addr("hcb-v1"), "org-alpha")
	setBridgingOrg(t, k, ctx, makeValidBech32Addr("hcb-v2"), "org-alpha") // same org
	setBridgingMinOrgs(t, k, ctx, 0) // Gap 4 event disabled

	// Set Gap 5 params.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighConfidenceBridgingMinOrgs = 2       // require 2 distinct orgs
	params.HighConfidenceBridgingThresholdBps = 900_000
	require.NoError(t, k.SetParams(ctx, params))

	round := &types.VerificationRound{
		ClaimId: "claim-hcb-1",
		Reveals: []*types.RevealEntry{
			{Verifier: makeValidBech32Addr("hcb-v1"), Vote: "accept"},
			{Verifier: makeValidBech32Addr("hcb-v2"), Vote: "accept"},
		},
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	keeper.RecordBridgingOrgCountForTest(k, sdkCtx, factID, round)

	// Fact should have confidence capped.
	after, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(899_999), after.Confidence, "confidence capped to threshold-1")
	require.Equal(t, uint64(1), after.BridgingOrgCount, "bridging count = 1 org")

	// Event should be present.
	events := sdkCtx.EventManager().Events()
	found = false
	for _, e := range events {
		if e.Type == "zerone.knowledge.high_confidence_bridging_cap_applied" {
			found = true
			break
		}
	}
	require.True(t, found, "high_confidence_bridging_cap_applied event should be emitted")
}

func TestHighConfidenceBridgingGap5_NoEvent_WhenSufficientBridging(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	factID := makeBridgingFact(t, k, ctx, "hcb-fact-2")
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	fact.Confidence = 950_000
	require.NoError(t, k.SetFact(ctx, fact))

	setBridgingOrg(t, k, ctx, makeValidBech32Addr("hcb2-v1"), "org-beta")
	setBridgingOrg(t, k, ctx, makeValidBech32Addr("hcb2-v2"), "org-gamma") // different org

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighConfidenceBridgingMinOrgs = 2
	params.HighConfidenceBridgingThresholdBps = 900_000
	require.NoError(t, k.SetParams(ctx, params))

	round := &types.VerificationRound{
		ClaimId: "claim-hcb-2",
		Reveals: []*types.RevealEntry{
			{Verifier: makeValidBech32Addr("hcb2-v1"), Vote: "accept"},
			{Verifier: makeValidBech32Addr("hcb2-v2"), Vote: "accept"},
		},
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	keeper.RecordBridgingOrgCountForTest(k, sdkCtx, factID, round)

	after, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(950_000), after.Confidence, "confidence unchanged — sufficient bridging")
	require.Equal(t, uint64(2), after.BridgingOrgCount)

	events := sdkCtx.EventManager().Events()
	for _, e := range events {
		require.NotEqual(t, "zerone.knowledge.high_confidence_bridging_cap_applied", e.Type,
			"cap event should NOT be emitted when bridging is sufficient")
	}
}

// ── validation tests ──────────────────────────────────────────────────────────

func TestHighConfidenceBridgingGap5_Validate_ThresholdZeroWhenEnabled(t *testing.T) {
	// Start from DefaultParams (always valid) and override gap 5 fields only.
	p := types.DefaultParams()
	p.HighConfidenceBridgingMinOrgs = 2
	p.HighConfidenceBridgingThresholdBps = 0 // invalid: min_orgs > 0 requires threshold > 0
	err := p.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "high_confidence_bridging_threshold_bps must be > 0")
}

func TestHighConfidenceBridgingGap5_Validate_ThresholdAboveMillion(t *testing.T) {
	p := types.DefaultParams()
	p.HighConfidenceBridgingMinOrgs = 2
	p.HighConfidenceBridgingThresholdBps = 1_000_001
	err := p.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "high_confidence_bridging_threshold_bps must be ≤ 1000000")
}

func TestHighConfidenceBridgingGap5_Validate_Valid(t *testing.T) {
	p := types.DefaultParams()
	p.HighConfidenceBridgingMinOrgs = 3
	p.HighConfidenceBridgingThresholdBps = 900_000
	require.NoError(t, p.Validate(), "valid enabled config should pass Validate()")
}
