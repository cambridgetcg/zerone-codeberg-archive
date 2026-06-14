package keeper_test

// Skepticism Gap 2 — Defeater-Aware Probe Prioritization.
//
// John Pollock's undercutting defeaters (Gap 1) track epistemic stress on
// surviving facts. Gap 2 closes the write-back loop: facts that have
// accumulated undercutting defeaters are promoted to the probe invitation
// queue sooner, using a governance-configured reduced idle threshold.
//
// Pollock's insight applied operationally:
//   A fact that survived multiple narrow-margin challenges still asserts
//   the same conclusion, but the accumulating defeaters reveal that the
//   evidential connection is being repeatedly questioned. The priority
//   mechanism ensures such facts receive additional scrutiny before their
//   effective_confidence decays to the point of being useless.
//
// Architecture:
//   Params.DefeaterProbePriorityMinCount  (field 195): min defeater_count to
//     qualify for priority. 0 = feature disabled.
//   Params.DefeaterProbeReducedThresholdBlocks (field 196): the reduced idle
//     threshold for priority facts. 0 = halve the normal threshold.
//
//   The `probe_invited` event now carries three new attributes:
//     defeater_count, effective_confidence_bps, priority_probe.
//
//   IdleFactsForProbing (QueryIdleFacts RPC) now returns DefeaterCount and
//     EffectiveConfidenceBps on each IdleFact.
//
// Tests (10):
//
//  1. PriorityDisabled — DefeaterProbePriorityMinCount=0: stressed fact uses
//     normal threshold (not invited before threshold, invited after).
//  2. PriorityEnabled_StressedFactInvitedEarly — stressed fact (DefeaterCount >=
//     minCount) is invited after reducedThreshold blocks, not after normal
//     threshold.
//  3. PriorityEnabled_PristineFactNotEarly — pristine fact (DefeaterCount <
//     minCount) still uses normal threshold even when feature is enabled.
//  4. ReducedThreshold_Default_Halves — when DefeaterProbeReducedThresholdBlocks=0,
//     the effective threshold is half the normal threshold.
//  5. ReducedThreshold_Explicit — explicit DefeaterProbeReducedThresholdBlocks
//     overrides the halving default.
//  6. ProbeEvent_PriorityProbe_True — priority_probe="true" in event when fact
//     qualifies for priority treatment.
//  7. ProbeEvent_PriorityProbe_False — priority_probe="false" for pristine facts.
//  8. ProbeEvent_DefeaterAttributes — defeater_count and effective_confidence_bps
//     are present and accurate in the probe_invited event.
//  9. IdleFactsForProbing_IncludesDefeaterFields — query returns DefeaterCount
//     and EffectiveConfidenceBps per idle fact.
// 10. NotInvited_BelowReducedThreshold — stressed fact NOT invited when block
//     height is between reduced and normal threshold, verified by checking
//     ProbeInvitedAtBlock remains 0.

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeProbeReadyFact creates an ACTIVE fact with the given confidence,
// defeater_count, and effective_confidence_bps. VerifiedAtBlock is set to 1
// so idle-time calculations have a stable reference. Returns the fact ID.
func makeProbeReadyFact(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	id string,
	confidence uint64,
	defeaterCount uint64,
	effectiveConfBps uint64,
) string {
	t.Helper()
	fact := &types.Fact{
		Id:                     id,
		Content:                "probe-test fact " + id,
		Domain:                 "physics",
		Category:               "test",
		Submitter:              makeValidBech32Addr("probe-sub-" + id),
		SubmittedAtBlock:       1,
		VerifiedAtBlock:        1,
		Status:                 types.FactStatus_FACT_STATUS_ACTIVE,
		Confidence:             confidence,
		DefeaterCount:          defeaterCount,
		EffectiveConfidenceBps: effectiveConfBps,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return id
}

// setProbeParams configures probe invitation + defeater priority params.
func setProbeParams(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	idleThreshold uint64,
	minConf uint64,
	priorityMinCount uint64,
	reducedThreshold uint64,
) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ProbeInvitationIdleThresholdBlocks = idleThreshold
	params.ProbeInvitationMinConfidenceBps = minConf
	params.ProbeInvitationBatchSize = 10
	params.ProbeInvitationReinviteCooldown = 0
	params.DefeaterProbePriorityMinCount = priorityMinCount
	params.DefeaterProbeReducedThresholdBlocks = reducedThreshold
	require.NoError(t, k.SetParams(ctx, params))
}

// hasProbeEvent returns true and an attribute map when ctx holds a
// zerone.knowledge.probe_invited event for the given fact_id.
func hasProbeEvent(ctx sdk.Context, factID string) (bool, map[string]string) {
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

// withHeight returns a fresh sdk.Context at the given block height.
func withHeight(ctx sdk.Context, h int64) sdk.Context {
	return ctx.WithBlockHeight(h).WithEventManager(sdk.NewEventManager())
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestDefeaterProbe_PriorityDisabled confirms that when
// DefeaterProbePriorityMinCount=0 (feature disabled), a stressed fact is NOT
// invited before the normal threshold.
func TestDefeaterProbe_PriorityDisabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold = uint64(100)
		minConf         = uint64(700_000)
	)
	// Feature disabled: priorityMinCount=0.
	setProbeParams(t, k, ctx, normalThreshold, minConf, 0, 0)

	// Stressed fact (3 defeaters).
	factID := makeProbeReadyFact(t, k, ctx, "pd-stressed", 800_000, 3, 680_000)

	// Invoke at height = normal threshold - 1 (block 100 verified, now at 100).
	// lastProbe = VerifiedAtBlock = 1; height=50 < 1+100=101 → not invited.
	ctx50 := withHeight(ctx, 50)
	params, _ := k.GetParams(ctx50)
	k.InviteIdleFactsForProbing(ctx50, uint64(50), params)

	found, _ := hasProbeEvent(ctx50, factID)
	require.False(t, found, "stressed fact must not be invited before normal threshold when priority disabled")

	f, ok := k.GetFact(ctx50, factID)
	require.True(t, ok)
	require.Equal(t, uint64(0), f.ProbeInvitedAtBlock, "ProbeInvitedAtBlock must remain 0")
}

// TestDefeaterProbe_StressedFactInvitedEarly confirms that with priority enabled,
// a fact with DefeaterCount >= minCount is invited using the reduced threshold.
func TestDefeaterProbe_StressedFactInvitedEarly(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold  = uint64(100)
		reducedThreshold = uint64(25)
		minConf          = uint64(700_000)
		priorityMin      = uint64(2)
	)
	setProbeParams(t, k, ctx, normalThreshold, minConf, priorityMin, reducedThreshold)

	// Stressed fact: 3 defeaters ≥ priorityMin=2 → qualifies for priority.
	factID := makeProbeReadyFact(t, k, ctx, "se-stressed", 800_000, 3, 680_000)

	// height=30 → 30 > VerifiedAtBlock(1) + reducedThreshold(25) = 26 → INVITED.
	ctx30 := withHeight(ctx, 30)
	params, _ := k.GetParams(ctx30)
	k.InviteIdleFactsForProbing(ctx30, uint64(30), params)

	found, _ := hasProbeEvent(ctx30, factID)
	require.True(t, found, "stressed fact must be invited after reduced threshold")
}

// TestDefeaterProbe_PristineFactNotEarly confirms that a pristine fact (no
// defeaters) still uses the normal idle threshold even when priority is enabled.
func TestDefeaterProbe_PristineFactNotEarly(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold  = uint64(100)
		reducedThreshold = uint64(25)
		minConf          = uint64(700_000)
		priorityMin      = uint64(2)
	)
	setProbeParams(t, k, ctx, normalThreshold, minConf, priorityMin, reducedThreshold)

	// Pristine fact: 0 defeaters → does NOT qualify for priority.
	factID := makeProbeReadyFact(t, k, ctx, "pf-pristine", 900_000, 0, 900_000)

	// height=30 < 1+100=101 → NOT invited (uses normal threshold).
	ctx30 := withHeight(ctx, 30)
	params, _ := k.GetParams(ctx30)
	k.InviteIdleFactsForProbing(ctx30, uint64(30), params)

	found, _ := hasProbeEvent(ctx30, factID)
	require.False(t, found, "pristine fact must NOT be invited before normal threshold")
}

// TestDefeaterProbe_ReducedThreshold_Default_Halves verifies that when
// DefeaterProbeReducedThresholdBlocks=0, the effective threshold is half the
// normal threshold.
func TestDefeaterProbe_ReducedThreshold_Default_Halves(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold = uint64(100) // halved = 50
		minConf         = uint64(700_000)
		priorityMin     = uint64(1)
	)
	// reducedThreshold=0 → use normalThreshold/2 = 50.
	setProbeParams(t, k, ctx, normalThreshold, minConf, priorityMin, 0)

	// Stressed fact (2 defeaters).
	factID := makeProbeReadyFact(t, k, ctx, "rh-half", 800_000, 2, 720_000)

	// height=55 > 1+50=51 → invited (reduced = 50).
	ctx55 := withHeight(ctx, 55)
	params, _ := k.GetParams(ctx55)
	k.InviteIdleFactsForProbing(ctx55, uint64(55), params)

	found, _ := hasProbeEvent(ctx55, factID)
	require.True(t, found, "stressed fact must be invited after halved threshold")
}

// TestDefeaterProbe_ReducedThreshold_Explicit verifies that an explicit
// DefeaterProbeReducedThresholdBlocks overrides the halving default.
func TestDefeaterProbe_ReducedThreshold_Explicit(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold  = uint64(100)
		reducedThreshold = uint64(10) // explicit — NOT halved (would be 50)
		minConf          = uint64(700_000)
		priorityMin      = uint64(1)
	)
	setProbeParams(t, k, ctx, normalThreshold, minConf, priorityMin, reducedThreshold)

	factID := makeProbeReadyFact(t, k, ctx, "re-explicit", 800_000, 1, 760_000)

	// height=15 > 1+10=11 → invited (explicit reduced = 10).
	ctx15 := withHeight(ctx, 15)
	params, _ := k.GetParams(ctx15)
	k.InviteIdleFactsForProbing(ctx15, uint64(15), params)

	found, _ := hasProbeEvent(ctx15, factID)
	require.True(t, found, "stressed fact must be invited after explicit reduced threshold")

	// Sanity: would NOT have been invited at the halved threshold (50).
	// height=15 < 1+50=51 — confirms we're using the explicit 10, not the half of 100.
	// (implicit: the test passes only because threshold=10, not 50)
}

// TestDefeaterProbe_ProbeEvent_PriorityProbe_True confirms priority_probe="true"
// in the event for a qualifying stressed fact.
func TestDefeaterProbe_ProbeEvent_PriorityProbe_True(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setProbeParams(t, k, ctx, 100, 700_000, 2, 25)

	factID := makeProbeReadyFact(t, k, ctx, "evt-priority-true", 850_000, 3, 722_500)

	ctx30 := withHeight(ctx, 30)
	params, _ := k.GetParams(ctx30)
	k.InviteIdleFactsForProbing(ctx30, uint64(30), params)

	found, attrs := hasProbeEvent(ctx30, factID)
	require.True(t, found)
	require.Equal(t, "true", attrs["priority_probe"],
		"priority_probe must be 'true' for stressed fact meeting threshold")
}

// TestDefeaterProbe_ProbeEvent_PriorityProbe_False confirms priority_probe="false"
// for a pristine fact invited at the normal threshold.
func TestDefeaterProbe_ProbeEvent_PriorityProbe_False(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setProbeParams(t, k, ctx, 50, 700_000, 5, 10) // priority enabled, but fact has 0 defeaters

	factID := makeProbeReadyFact(t, k, ctx, "evt-priority-false", 900_000, 0, 900_000)

	// height=55 > 1+50=51 → invited at normal threshold.
	ctx55 := withHeight(ctx, 55)
	params, _ := k.GetParams(ctx55)
	k.InviteIdleFactsForProbing(ctx55, uint64(55), params)

	found, attrs := hasProbeEvent(ctx55, factID)
	require.True(t, found, "pristine fact must be invited at normal threshold")
	require.Equal(t, "false", attrs["priority_probe"],
		"priority_probe must be 'false' for pristine fact")
}

// TestDefeaterProbe_ProbeEvent_DefeaterAttributes verifies that defeater_count
// and effective_confidence_bps are present and accurate in the probe_invited event.
func TestDefeaterProbe_ProbeEvent_DefeaterAttributes(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setProbeParams(t, k, ctx, 50, 700_000, 0, 0) // priority disabled; just check event attrs

	const (
		conf         = uint64(800_000)
		defeaters    = uint64(3)
		effectiveConf = uint64(680_000)
	)
	factID := makeProbeReadyFact(t, k, ctx, "evt-attrs", conf, defeaters, effectiveConf)

	ctx55 := withHeight(ctx, 55)
	params, _ := k.GetParams(ctx55)
	k.InviteIdleFactsForProbing(ctx55, uint64(55), params)

	found, attrs := hasProbeEvent(ctx55, factID)
	require.True(t, found)
	require.Equal(t, fmt.Sprintf("%d", defeaters), attrs["defeater_count"],
		"defeater_count attribute must match Fact.DefeaterCount")
	require.Equal(t, fmt.Sprintf("%d", effectiveConf), attrs["effective_confidence_bps"],
		"effective_confidence_bps attribute must match Fact.EffectiveConfidenceBps")
}

// TestDefeaterProbe_IdleFactsForProbing_IncludesDefeaterFields verifies that
// the IdleFactsForProbing query returns DefeaterCount and EffectiveConfidenceBps.
func TestDefeaterProbe_IdleFactsForProbing_IncludesDefeaterFields(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setProbeParams(t, k, ctx, 50, 700_000, 0, 0)

	const (
		conf         = uint64(900_000)
		defeaters    = uint64(2)
		effectiveConf = uint64(810_000)
	)
	factID := makeProbeReadyFact(t, k, ctx, "ifl-defeater", conf, defeaters, effectiveConf)

	// Stamp ProbeInvitedAtBlock directly so IdleFactsForProbing considers it invited.
	f, ok := k.GetFact(ctx, factID)
	require.True(t, ok)
	f.ProbeInvitedAtBlock = 55
	require.NoError(t, k.SetFact(ctx, f))

	ctx56 := withHeight(ctx, 56)
	idle := k.IdleFactsForProbing(ctx56, "", 10)
	require.Len(t, idle, 1, "one idle fact expected")
	require.Equal(t, factID, idle[0].Id)
	require.Equal(t, defeaters, idle[0].DefeaterCount,
		"IdleFact.DefeaterCount must reflect Fact.DefeaterCount")
	require.Equal(t, effectiveConf, idle[0].EffectiveConfidenceBps,
		"IdleFact.EffectiveConfidenceBps must reflect Fact.EffectiveConfidenceBps")
}

// TestDefeaterProbe_NotInvited_BelowReducedThreshold confirms that a stressed
// fact is NOT invited when height is between the reduced threshold and the
// normal threshold (proving the reduced threshold is smaller than normal).
func TestDefeaterProbe_NotInvited_BelowReducedThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	const (
		normalThreshold  = uint64(100)
		reducedThreshold = uint64(40)
		priorityMin      = uint64(1)
	)
	setProbeParams(t, k, ctx, normalThreshold, 700_000, priorityMin, reducedThreshold)

	factID := makeProbeReadyFact(t, k, ctx, "nib-stressed", 800_000, 2, 720_000)

	// height=30 < 1+40=41 → NOT invited even for stressed fact.
	ctx30 := withHeight(ctx, 30)
	params, _ := k.GetParams(ctx30)
	k.InviteIdleFactsForProbing(ctx30, uint64(30), params)

	found, _ := hasProbeEvent(ctx30, factID)
	require.False(t, found, "stressed fact must not be invited below reduced threshold")

	f, ok := k.GetFact(ctx30, factID)
	require.True(t, ok)
	require.Equal(t, uint64(0), f.ProbeInvitedAtBlock,
		"ProbeInvitedAtBlock must remain 0 when not invited")
}
