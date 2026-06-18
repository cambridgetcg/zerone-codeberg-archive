package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ────────────────────────────────────────────────────────────────────────────
// Skepticism Gap 4 — Regress-Depth Probe Prioritization
//
// Facts far from axioms have more potential failure points in their derivation
// chain (Regress Tracking / Doubt Provenance, §5.4 of the skepticism brief).
// When enabled, facts with AxiomDistance ≥ RegressDepthProbeThreshold use a
// reduced idle threshold — they surface in the probe queue sooner.
//
// The feature is structurally parallel to Skepticism Gap 2 (defeater-aware
// probe prioritization) but operates on the regress-depth axis rather than
// the epistemic-stress (DefeaterCount) axis.
//
// New params:
//   - RegressDepthProbeThreshold (field 199): minimum AxiomDistance; 0 = disabled
//   - RegressDepthProbeReducedThresholdBlocks (field 200): reduced idle threshold;
//     0 = halve the normal threshold
//
// New event attribute on zerone.knowledge.probe_invited:
//   - axiom_distance
//
// New IdleFact field:
//   - axiom_distance (proto field 9)
//
// When both Gap 2 (defeater) and Gap 4 (regress depth) are active and a fact
// qualifies under both, the minimum of the two reduced thresholds applies —
// the chain uses whichever signal indicates higher urgency.
// ────────────────────────────────────────────────────────────────────────────

// makeDeepFact creates and stores a fact with the given AxiomDistance.
func makeDeepFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, id string, axiomDistance uint32) *types.Fact {
	t.Helper()
	height := uint64(ctx.BlockHeight())
	fact := &types.Fact{
		Id:                id,
		Content:           "deep-lineage test fact: " + id,
		Domain:            "physics",
		Category:          "epistemics",
		Confidence:        800_000,
		Submitter:         makeValidBech32Addr("sub-" + id),
		SubmittedAtBlock:  1,
		VerifiedAtBlock:   1,
		LastVerifiedBlock: 1,
		Status:            types.FactStatus_FACT_STATUS_VERIFIED,
		AxiomDistance:     axiomDistance,
		LastCorroboratedBlock: 0,
	}
	// Place the probe clock at block 1 so the idleness calculation works.
	fact.VerifiedAtBlock = 1
	require.NoError(t, k.SetFact(ctx, fact))
	_ = height
	return fact
}

// setRegressParams configures params for regress-depth prioritization tests.
func setRegressParams(t *testing.T, k keeper.Keeper, ctx sdk.Context,
	idleThreshold, minConf uint64,
	regressThreshold, regressReducedThreshold uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProbeInvitationBatchSize = 10
	params.ProbeInvitationIdleThresholdBlocks = idleThreshold
	params.ProbeInvitationMinConfidenceBps = minConf
	params.ProbeInvitationReinviteCooldown = 0
	params.DefeaterProbePriorityMinCount = 0          // Gap 2 disabled for Gap 4 tests
	params.DefeaterProbeReducedThresholdBlocks = 0
	params.RegressDepthProbeThreshold = regressThreshold
	params.RegressDepthProbeReducedThresholdBlocks = regressReducedThreshold
	require.NoError(t, k.SetParams(ctx, params))
}

// hasProbeEventForFact returns the probe_invited event attrs for a specific fact_id.
func hasProbeEventForFact(ctx sdk.Context, factID string) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.probe_invited" {
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

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestRegressProbe_Disabled: RegressDepthProbeThreshold=0 means all facts use
// the normal idle threshold regardless of AxiomDistance.
func TestRegressProbe_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	const idleThreshold = uint64(100)
	setRegressParams(t, k, ctx, idleThreshold, 0, 0, 0)

	// Deep fact (AxiomDistance=10) at height=1.
	deepFact := makeDeepFact(t, k, ctx, "regress-disabled-deep", 10)

	// At height=55 (< 100 threshold) — should NOT be invited (feature disabled).
	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, _ := hasProbeEventForFact(ctx55, deepFact.Id)
	require.False(t, fired,
		"threshold=0 (disabled): deep fact must use normal idle threshold → not invited at height 55")
}

// TestRegressProbe_DeepFactInvitedEarly: a fact with AxiomDistance above the
// threshold is invited before it would be under the normal idle threshold.
func TestRegressProbe_DeepFactInvitedEarly(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// idleThreshold=100, regressThreshold=5, regressReduced=50
	setRegressParams(t, k, ctx, 100, 0, 5, 50)

	// Deep fact: AxiomDistance=7 (>= threshold=5).
	deepFact := makeDeepFact(t, k, ctx, "regress-deep-early", 7)

	// At height=55: 55 - 1 (verifiedAtBlock) = 54 < 100 (normal) but ≥ 50 (reduced).
	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, _ := hasProbeEventForFact(ctx55, deepFact.Id)
	require.True(t, fired,
		"deep fact (AxiomDistance=7 >= threshold=5) must be invited at height 55 (< normal but ≥ reduced=50)")
}

// TestRegressProbe_ShallowFactNotEarly: a fact with AxiomDistance below the
// threshold still uses the normal idle threshold.
func TestRegressProbe_ShallowFactNotEarly(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// idleThreshold=100, regressThreshold=5, regressReduced=50
	setRegressParams(t, k, ctx, 100, 0, 5, 50)

	// Shallow fact: AxiomDistance=2 (< threshold=5).
	shallowFact := makeDeepFact(t, k, ctx, "regress-shallow", 2)

	// At height=55: eligible for reduced (50) but AxiomDistance=2 < threshold=5 → normal threshold applies.
	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, _ := hasProbeEventForFact(ctx55, shallowFact.Id)
	require.False(t, fired,
		"shallow fact (AxiomDistance=2 < threshold=5) must NOT be invited at height 55 (normal threshold=100)")
}

// TestRegressProbe_DefaultReducedThreshold_Halves: when
// RegressDepthProbeReducedThresholdBlocks=0, the effective reduced threshold
// is half the normal idle threshold.
func TestRegressProbe_DefaultReducedThreshold_Halves(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// idleThreshold=100, regressThreshold=3, regressReduced=0 (default: halve to 50)
	setRegressParams(t, k, ctx, 100, 0, 3, 0)

	deepFact := makeDeepFact(t, k, ctx, "regress-default-halve", 4) // AxiomDistance=4 >= 3

	// At height=55 (>= 50 halved but < 100 normal).
	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, _ := hasProbeEventForFact(ctx55, deepFact.Id)
	require.True(t, fired,
		"default halving: reduced threshold = 50; deep fact must be invited at height 55")
}

// TestRegressProbe_ExplicitReducedThreshold: governance can set an explicit
// reduced threshold other than half the normal.
func TestRegressProbe_ExplicitReducedThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// idleThreshold=200, regressThreshold=3, regressReduced=30 (explicit override)
	setRegressParams(t, k, ctx, 200, 0, 3, 30)

	deepFact := makeDeepFact(t, k, ctx, "regress-explicit", 5) // AxiomDistance=5 >= 3

	// At height=35 (35 - 1 = 34 >= 30 but < 200 normal and < 100 halved).
	ctx35 := ctx.WithBlockHeight(35).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx35, 35, params)

	fired, _ := hasProbeEventForFact(ctx35, deepFact.Id)
	require.True(t, fired,
		"explicit reduced threshold=30: deep fact must be invited at height 35")
}

// TestRegressProbe_ProbeEventAxiomDistanceAttribute: the probe_invited event
// carries axiom_distance as an attribute.
func TestRegressProbe_ProbeEventAxiomDistanceAttribute(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	setRegressParams(t, k, ctx, 100, 0, 3, 50)
	const axiomDist = uint32(8)
	deepFact := makeDeepFact(t, k, ctx, "regress-event-attr", axiomDist)

	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, attrs := hasProbeEventForFact(ctx55, deepFact.Id)
	require.True(t, fired)
	require.Equal(t, fmt.Sprintf("%d", axiomDist), attrs["axiom_distance"],
		"probe_invited event must carry axiom_distance attribute")
}

// TestRegressProbe_ProbeEventPriorityTrue: when a fact qualifies under Gap 4,
// priority_probe attribute is "true".
func TestRegressProbe_ProbeEventPriorityTrue(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	setRegressParams(t, k, ctx, 100, 0, 5, 50)
	deepFact := makeDeepFact(t, k, ctx, "regress-priority-true", 6) // AxiomDistance=6 >= 5

	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, attrs := hasProbeEventForFact(ctx55, deepFact.Id)
	require.True(t, fired)
	require.Equal(t, "true", attrs["priority_probe"],
		"deep fact qualifying under Gap 4 must have priority_probe=true")
}

// TestRegressProbe_ProbeEventPriorityFalse: a non-deep fact has priority_probe=false.
func TestRegressProbe_ProbeEventPriorityFalse(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Normal threshold=50 so a shallow fact (AxiomDistance=1) gets invited at height=55.
	setRegressParams(t, k, ctx, 50, 0, 5, 25) // regressThreshold=5, but AxiomDistance=1 below it

	shallowFact := makeDeepFact(t, k, ctx, "regress-priority-false", 1) // AxiomDistance=1 < 5

	ctx55 := ctx.WithBlockHeight(55).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx55, 55, params)

	fired, attrs := hasProbeEventForFact(ctx55, shallowFact.Id)
	require.True(t, fired, "shallow fact invited via normal threshold (55-1=54 >= 50)")
	require.Equal(t, "false", attrs["priority_probe"],
		"shallow fact not qualifying under Gap 4 must have priority_probe=false")
}

// TestRegressProbe_IdleFactsForProbing_IncludesAxiomDistance: the
// IdleFactsForProbing query response includes axiom_distance on each IdleFact.
func TestRegressProbe_IdleFactsForProbing_IncludesAxiomDistance(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Build a fact with a known AxiomDistance and stamp ProbeInvitedAtBlock.
	const expectedAxiomDist = uint32(5)
	fact := makeDeepFact(t, k, ctx, "regress-idle-query", expectedAxiomDist)
	fact.ProbeInvitedAtBlock = 10
	require.NoError(t, k.SetFact(ctx, fact))

	ctx100 := ctx.WithBlockHeight(100)
	results := k.IdleFactsForProbing(ctx100, "physics", 10)

	require.NotEmpty(t, results)
	found := false
	for _, r := range results {
		if r.Id == fact.Id {
			require.Equal(t, expectedAxiomDist, r.AxiomDistance,
				"IdleFactsForProbing must include axiom_distance on each IdleFact")
			found = true
		}
	}
	require.True(t, found, "fact must appear in IdleFactsForProbing results")
}

// TestRegressProbe_NotInvited_BelowReducedThreshold: deep fact exists but height
// is below even the reduced threshold — not invited.
func TestRegressProbe_NotInvited_BelowReducedThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// idleThreshold=100, regressThreshold=3, regressReduced=60
	setRegressParams(t, k, ctx, 100, 0, 3, 60)

	deepFact := makeDeepFact(t, k, ctx, "regress-below-reduced", 5) // AxiomDistance=5 >= 3

	// height=30 → 30-1=29 < 60 reduced threshold → NOT invited.
	ctx30 := ctx.WithBlockHeight(30).WithEventManager(sdk.NewEventManager())
	params, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx30, 30, params)

	fired, _ := hasProbeEventForFact(ctx30, deepFact.Id)
	require.False(t, fired,
		"deep fact must not be invited when height < reduced threshold (29 < 60)")
}

// TestRegressProbe_BothGapsActive_MinThresholdApplied: when both Gap 2 (defeater)
// and Gap 4 (regress depth) are active and a fact qualifies under both, the
// minimum of the two reduced thresholds is used.
func TestRegressProbe_BothGapsActive_MinThresholdApplied(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name: "physics", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProbeInvitationBatchSize = 10
	params.ProbeInvitationIdleThresholdBlocks = 100
	params.ProbeInvitationMinConfidenceBps = 0
	params.ProbeInvitationReinviteCooldown = 0
	// Gap 2: defeater priority with reduced threshold = 70
	params.DefeaterProbePriorityMinCount = 2
	params.DefeaterProbeReducedThresholdBlocks = 70
	// Gap 4: regress depth priority with reduced threshold = 40
	params.RegressDepthProbeThreshold = 5
	params.RegressDepthProbeReducedThresholdBlocks = 40
	require.NoError(t, k.SetParams(ctx, params))

	// Fact qualifies under both: DefeaterCount=3 (>= 2), AxiomDistance=6 (>= 5).
	height := uint64(ctx.BlockHeight())
	fact := &types.Fact{
		Id:            "both-gaps-active",
		Content:       "qualifies under both Gap 2 and Gap 4",
		Domain:        "physics",
		Category:      "epistemics",
		Confidence:    800_000,
		Submitter:     makeValidBech32Addr("both-gaps-sub"),
		VerifiedAtBlock: 1,
		Status:        types.FactStatus_FACT_STATUS_VERIFIED,
		DefeaterCount: 3,
		AxiomDistance: 6,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	_ = height

	// At height=45: 45-1=44 >= min(70, 40)=40 → should be invited.
	ctx45 := ctx.WithBlockHeight(45).WithEventManager(sdk.NewEventManager())
	p, _ := k.GetParams(ctx)
	k.InviteIdleFactsForProbing(ctx45, 45, p)

	fired, attrs := hasProbeEventForFact(ctx45, fact.Id)
	require.True(t, fired,
		"fact qualifying under both gaps must be invited at height 45 (min threshold = 40)")
	require.Equal(t, "true", attrs["priority_probe"])
	require.Equal(t, "6", attrs["axiom_distance"])
}
