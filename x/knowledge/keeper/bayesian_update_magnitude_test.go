package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ────────────────────────────────────────────────────────────────────────────
// Bayesian Gap 3 — Bayesian Update Magnitude
//
// When a claim is accepted and the submitter declared a prior probability, the
// chain computes:
//
//	bayesian_update_magnitude_bps = |final_confidence_bps − stated_prior_bps|
//
// This measures epistemic surprise: how much did the evidence actually move
// the community's belief relative to what the submitter expected?
//
// A near-zero magnitude means the evidence merely confirmed the prior.
// A magnitude near 1_000_000 means the evidence completely overturned it.
//
// The field is stored on Fact (proto field 90) at ACCEPT time via
// createFactFromClaim. When Params.high_bayesian_update_threshold_bps > 0
// (field 198), a zerone.knowledge.high_bayesian_update event is emitted for
// every accepted fact whose magnitude meets or exceeds the threshold.
// ────────────────────────────────────────────────────────────────────────────

// bayesianMagnitudeFactID helps each test create a uniquely-IDed claim.
const bayesianMagnitudeClaimPrefix = "claim-bayes-magnitude-"

// makeBayesianClaim builds a minimal Claim for testing createFactFromClaim.
func makeBayesianClaim(id string, domain string, priorBps uint64) *types.Claim {
	return &types.Claim{
		Id:                  id,
		FactContent:         "Bayesian update magnitude test: " + id,
		Domain:              domain,
		Category:            "epistemics",
		Submitter:           makeValidBech32Addr("bayes-sub-" + id),
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		PriorProbabilityBps: priorBps,
	}
}

// minimalRound builds the smallest VerificationRound that won't crash
// createFactFromClaim (round is only read for its Id in standard paths).
func minimalRound(id, claimID string) *types.VerificationRound {
	return &types.VerificationRound{
		Id:      id,
		ClaimId: claimID,
	}
}

// findBayesianUpdateEvent searches ctx events for
// zerone.knowledge.high_bayesian_update and returns attrs when found.
func findBayesianUpdateEvent(ctx sdk.Context, factID string) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.high_bayesian_update" {
			attrs := make(map[string]string)
			for _, a := range ev.Attributes {
				attrs[a.Key] = a.Value
			}
			if attrs["fact_id"] == factID {
				return true, attrs
			}
		}
	}
	return false, nil
}

// findFactCreatedMagnitudeAttr looks up the bayesian_update_magnitude_bps
// attribute on the zerone.knowledge.fact_created event for the given fact_id.
func findFactCreatedMagnitudeAttr(ctx sdk.Context, factID string) (string, bool) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.fact_created" {
			attrs := make(map[string]string)
			for _, a := range ev.Attributes {
				attrs[a.Key] = a.Value
			}
			if attrs["fact_id"] == factID {
				v, ok := attrs["bayesian_update_magnitude_bps"]
				return v, ok
			}
		}
	}
	return "", false
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestBayesianMagnitude_NoPrior: when no prior was declared (PriorProbabilityBps=0),
// bayesian_update_magnitude_bps must remain 0 — nothing to compute.
func TestBayesianMagnitude_NoPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"no-prior", "physics", 0)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-no-prior", claim.Id), 800_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Zero(t, fact.BayesianUpdateMagnitudeBps,
		"no prior declared → magnitude must be 0")
}

// TestBayesianMagnitude_PriorEqualsConfidence: stated prior exactly matches
// final confidence → magnitude = 0 (no update, prediction perfectly calibrated).
func TestBayesianMagnitude_PriorEqualsConfidence(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	const priorBps = uint64(750_000)
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"equal", "physics", priorBps)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-equal", claim.Id), priorBps)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Zero(t, fact.BayesianUpdateMagnitudeBps,
		"prior == confidence → magnitude must be 0")
}

// TestBayesianMagnitude_PositiveUpdate: confidence exceeds prior →
// magnitude = confidence - prior (the evidence was better than expected).
func TestBayesianMagnitude_PositiveUpdate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 40%, final confidence = 85% → magnitude = 45% = 450_000 BPS
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"positive", "physics", 400_000)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-positive", claim.Id), 850_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(450_000), fact.BayesianUpdateMagnitudeBps,
		"|850_000 − 400_000| = 450_000")
}

// TestBayesianMagnitude_NegativeUpdate: prior exceeds confidence →
// magnitude = prior - confidence (the evidence was weaker than expected).
func TestBayesianMagnitude_NegativeUpdate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 90%, final confidence = 65% → magnitude = 25% = 250_000 BPS
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"negative", "physics", 900_000)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-negative", claim.Id), 650_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(250_000), fact.BayesianUpdateMagnitudeBps,
		"|650_000 − 900_000| = 250_000")
}

// TestBayesianMagnitude_MaxSurprise: prior = 1 BPS (near-zero), evidence pushes
// to maximum confidence. The magnitude = |final_confidence − 1|. We verify the
// relationship holds rather than hardcoding a specific value, because
// createFactFromClaim applies role bonuses and MaxConfidence clamping that
// modify the final stored confidence from the input value.
func TestBayesianMagnitude_MaxSurprise(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	const priorBps = uint64(1) // near-zero prior
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"max-surprise", "physics", priorBps)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-max-surprise", claim.Id), 1_000_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Greater(t, fact.Confidence, priorBps,
		"post-adjustment confidence must exceed near-zero prior")
	// The magnitude must equal the absolute difference of the *stored* confidence and the prior.
	require.Equal(t, fact.Confidence-priorBps, fact.BayesianUpdateMagnitudeBps,
		"magnitude = |final_confidence − stated_prior| = final_confidence − 1")
}

// TestBayesianMagnitude_HighUpdateEvent_Disabled: when
// high_bayesian_update_threshold_bps = 0, no event is emitted regardless of
// how large the magnitude is.
func TestBayesianMagnitude_HighUpdateEvent_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Ensure threshold is 0 (default — disabled).
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighBayesianUpdateThresholdBps = 0
	require.NoError(t, k.SetParams(ctx, params))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Large surprise: prior=1, confidence=1_000_000 → magnitude=999_999
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"ev-disabled", "physics", 1)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-ev-disabled", claim.Id), 1_000_000)
	require.NoError(t, err)

	fired, _ := findBayesianUpdateEvent(ctx, factID)
	require.False(t, fired, "threshold=0 → no high_bayesian_update event regardless of magnitude")
}

// TestBayesianMagnitude_HighUpdateEvent_BelowThreshold: magnitude < threshold
// → no event emitted.
func TestBayesianMagnitude_HighUpdateEvent_BelowThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighBayesianUpdateThresholdBps = 500_000 // threshold = 50%
	require.NoError(t, k.SetParams(ctx, params))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 600_000, confidence = 800_000 → magnitude = 200_000 (< 500_000 threshold)
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"below-thresh", "physics", 600_000)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-below-thresh", claim.Id), 800_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(200_000), fact.BayesianUpdateMagnitudeBps)

	fired, _ := findBayesianUpdateEvent(ctx, factID)
	require.False(t, fired, "magnitude 200_000 < threshold 500_000 → no event")
}

// TestBayesianMagnitude_HighUpdateEvent_AtThreshold: magnitude == threshold
// → event IS emitted (>= check, not >).
func TestBayesianMagnitude_HighUpdateEvent_AtThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighBayesianUpdateThresholdBps = 300_000 // threshold = 30%
	require.NoError(t, k.SetParams(ctx, params))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 400_000, confidence = 700_000 → magnitude = 300_000 (== threshold)
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"at-thresh", "physics", 400_000)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-at-thresh", claim.Id), 700_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(300_000), fact.BayesianUpdateMagnitudeBps)

	fired, attrs := findBayesianUpdateEvent(ctx, factID)
	require.True(t, fired, "magnitude == threshold → event must fire (>= boundary)")
	require.Equal(t, "300000", attrs["bayesian_update_magnitude_bps"])
	require.Equal(t, "300000", attrs["threshold_bps"])
}

// TestBayesianMagnitude_HighUpdateEvent_AboveThreshold: magnitude > threshold
// → event emitted with correct attributes including fact_id, prior, confidence,
// magnitude, and threshold. We set a low threshold (10%) so the magnitude of
// any reasonable update will exceed it, and verify attribute values from the
// stored fact (which may differ from the input confidence after clamping).
func TestBayesianMagnitude_HighUpdateEvent_AboveThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.HighBayesianUpdateThresholdBps = 100_000 // threshold = 10%
	require.NoError(t, k.SetParams(ctx, params))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 200_000 (20%), input confidence = 900_000. After clamping the
	// stored confidence will be ≤ MaxConfidence (880_000) but still >> 200_000,
	// so magnitude >> 100_000 threshold. Event should always fire.
	const priorBps = uint64(200_000)
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"above-thresh", "physics", priorBps)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-above-thresh", claim.Id), 900_000)
	require.NoError(t, err)

	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	// Verify magnitude computed from the *stored* (post-adjustment) confidence.
	require.Equal(t, fact.Confidence-priorBps, fact.BayesianUpdateMagnitudeBps,
		"magnitude = stored_confidence − prior_bps")
	require.Greater(t, fact.BayesianUpdateMagnitudeBps, uint64(100_000),
		"magnitude must exceed the 10% threshold so the event fires")

	fired, attrs := findBayesianUpdateEvent(ctx, factID)
	require.True(t, fired, "magnitude exceeds threshold → high_bayesian_update event must fire")
	require.Equal(t, "200000", attrs["stated_prior_bps"])
	require.Equal(t, "100000", attrs["threshold_bps"])
	require.Equal(t, "physics", attrs["domain"])
	// Event magnitude attribute must match the stored field.
	require.Equal(t,
		fmt.Sprintf("%d", fact.BayesianUpdateMagnitudeBps),
		attrs["bayesian_update_magnitude_bps"],
		"event attribute must echo stored BayesianUpdateMagnitudeBps")
}

// TestBayesianMagnitude_FactCreatedEventIncludesMagnitude: the standard
// zerone.knowledge.fact_created event now carries bayesian_update_magnitude_bps
// as an attribute — verify it is present and correct even when the
// high_bayesian_update threshold is not met.
func TestBayesianMagnitude_FactCreatedEventIncludesMagnitude(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	ctx = ctx.WithEventManager(sdk.NewEventManager())
	// Prior = 500_000, confidence = 800_000 → magnitude = 300_000
	claim := makeBayesianClaim(bayesianMagnitudeClaimPrefix+"fact-created-attr", "physics", 500_000)
	require.NoError(t, k.SetClaim(ctx, claim))

	factID, err := k.CreateFactFromClaimForTest(ctx, claim, minimalRound("r-fc-attr", claim.Id), 800_000)
	require.NoError(t, err)

	val, found := findFactCreatedMagnitudeAttr(ctx, factID)
	require.True(t, found, "bayesian_update_magnitude_bps must appear in fact_created event")
	require.Equal(t, "300000", val)
}
