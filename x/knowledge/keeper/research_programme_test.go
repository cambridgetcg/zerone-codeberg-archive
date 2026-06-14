package keeper_test

// Falsificationism Gap 3 — Lakatosian Research Programme Depth Tracking.
//
// Lakatos extended Popper by recognising that science operates through
// Research Programmes — structured sequences of theories connected by a
// protective belt of auxiliary hypotheses. A programme is:
//
//	PROGRESSIVE  — each rescue generates novel, independently testable
//	               predictions that were subsequently confirmed.
//	DEGENERATING — rescues are purely defensive; no new predictions are
//	               generated or confirmed.
//
// On-chain encoding:
//   - Claim.ResearchProgrammeDepth (field 51): depth of the rescue chain
//     this claim sits in. depth=0 = no rescue parent declared.
//     depth=N = this claim is N rescue steps deep.
//   - Claim.ResearchProgrammeDepth is set at submission time by inheriting
//     rescued_claim.ResearchProgrammeDepth + 1.
//   - Params.ResearchProgrammeDegenerationThreshold (field 192): governance
//     gate. 0 = disabled (default). When > 0, submitting a rescue claim
//     whose resulting depth exceeds the threshold emits event
//     zerone.knowledge.degenerating_research_programme.
//   - The signal is non-blocking — the claim is still accepted for
//     verification. Governance uses threshold as a visibility parameter,
//     not a censorship one.
//
// Tests:
//   1. No rescue declared → depth=0 on new claim.
//   2. Single rescue of a depth-0 claim → new claim depth=1.
//   3. Chained rescue (rescued.depth=1) → new claim depth=2.
//   4. Orphaned rescue_id (rescued claim not found) → depth stays 0.
//   5. Threshold disabled (param=0) → no degenerating event at any depth.
//   6. Depth at threshold (depth == threshold) → no event (condition is >).
//   7. Depth one over threshold → degenerating event emitted with all attributes.
//   8. Depth many over threshold → event emitted once per rescue (not repeatedly).

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// setClaimWithDepth stores a claim with the given ResearchProgrammeDepth.
// The claim is stored as REJECTED so it can be used as a rescue target.
func setClaimWithDepth(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	id string,
	depth uint64,
) {
	t.Helper()
	claim := &types.Claim{
		Id:                     id,
		FactContent:            "theory at depth " + id,
		Domain:                 "physics",
		Category:               "test",
		Submitter:              makeValidBech32Addr("depth-sub-" + id),
		SubmittedAtBlock:       1,
		Status:                 types.ClaimStatus_CLAIM_STATUS_REJECTED,
		Stake:                  "1000uzrn",
		ResearchProgrammeDepth: depth,
	}
	require.NoError(t, k.SetClaim(ctx, claim))
}

// hasDegeneratingEvent returns true when the SDK context contains at least one
// zerone.knowledge.degenerating_research_programme event. It also returns the
// attribute map of the first matching event for further assertions.
func hasDegeneratingEvent(ctx sdk.Context) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.degenerating_research_programme" {
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

// TestResearchProgramme_NoRescue_DepthIsZero verifies that a claim submitted
// without a RescuesClaimId carries depth=0.
func TestResearchProgramme_NoRescue_DepthIsZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	sub := makeValidBech32Addr("rp-no-rescue")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   sub,
		FactContent: "standalone claim with no rescue ancestor",
		Domain:      "physics",
		Category:    "mechanics",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(0), stored.ResearchProgrammeDepth,
		"no rescue declared → depth must be 0")
}

// TestResearchProgramme_SingleRescue_DepthIsOne verifies that rescuing a
// depth-0 claim produces depth=1 on the new claim.
func TestResearchProgramme_SingleRescue_DepthIsOne(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "chemistry",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Depth-0 rejected claim.
	setClaimWithDepth(t, k, ctx, "root-claim", 0)

	sub := makeValidBech32Addr("rp-single-rescue")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "first rescue of root claim",
		Domain:         "chemistry",
		Category:       "combustion",
		Stake:          "1000000",
		RescuesClaimId: "root-claim",
		RefutationConditions: []string{"new testable prediction"},
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(1), stored.ResearchProgrammeDepth,
		"rescue of depth-0 claim → new claim must have depth=1")
}

// TestResearchProgramme_ChainedRescue_DepthIncrements verifies depth
// propagation through a two-step rescue chain: depth=1 → depth=2.
func TestResearchProgramme_ChainedRescue_DepthIncrements(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// First step: a claim at depth=1 (one rescue already in the chain).
	setClaimWithDepth(t, k, ctx, "depth-1-claim", 1)

	sub := makeValidBech32Addr("rp-chain-rescue")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "second rescue in a Lakatosian chain",
		Domain:         "physics",
		Category:       "mechanics",
		Stake:          "1000000",
		RescuesClaimId: "depth-1-claim",
		RefutationConditions: []string{"additional testable prediction"},
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(2), stored.ResearchProgrammeDepth,
		"rescue of depth-1 claim → new claim must have depth=2")
}

// TestResearchProgramme_OrphanedRescueId_DepthZero verifies that when the
// rescued claim ID doesn't exist in state, the new claim receives depth=0
// (graceful degradation — non-blocking).
func TestResearchProgramme_OrphanedRescueId_DepthZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "biology",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	sub := makeValidBech32Addr("rp-orphan-rescue")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "rescue of a claim that no longer exists",
		Domain:         "biology",
		Category:       "genetics",
		Stake:          "1000000",
		RescuesClaimId: "nonexistent-claim-xyz",
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(0), stored.ResearchProgrammeDepth,
		"orphaned rescue_id: rescued claim not found → depth must stay 0")
}

// TestResearchProgramme_ThresholdDisabled_NoEvent verifies that when the
// degeneration threshold param is 0 (default/disabled), no
// degenerating_research_programme event is emitted even for very deep chains.
func TestResearchProgramme_ThresholdDisabled_NoEvent(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Ensure threshold is 0 (disabled).
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 0
	require.NoError(t, k.SetParams(ctx, params))

	// A very deep rescue chain (depth=50) should not fire the event.
	setClaimWithDepth(t, k, ctx, "deep-claim", 50)

	sub := makeValidBech32Addr("rp-disabled-threshold")
	_, err = ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "rescue at depth 51 but threshold disabled",
		Domain:         "physics",
		Category:       "mechanics",
		Stake:          "1000000",
		RescuesClaimId: "deep-claim",
	})
	require.NoError(t, err)

	found, _ := hasDegeneratingEvent(ctx)
	require.False(t, found,
		"threshold=0 (disabled) → no degenerating_research_programme event at any depth")
}

// TestResearchProgramme_AtThreshold_NoEvent verifies the boundary condition:
// depth == threshold does NOT emit the event. The condition is strictly >.
func TestResearchProgramme_AtThreshold_NoEvent(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Threshold = 3. Rescuing a depth-2 claim → new depth = 3 = threshold.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 3
	require.NoError(t, k.SetParams(ctx, params))

	setClaimWithDepth(t, k, ctx, "depth-2-boundary", 2)

	sub := makeValidBech32Addr("rp-at-threshold")
	_, err = ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "rescue that hits exactly the threshold",
		Domain:         "physics",
		Category:       "mechanics",
		Stake:          "1000000",
		RescuesClaimId: "depth-2-boundary",
	})
	require.NoError(t, err)

	found, _ := hasDegeneratingEvent(ctx)
	require.False(t, found,
		"depth == threshold: condition is strictly > threshold → no event at boundary")
}

// TestResearchProgramme_OverThreshold_EventEmitted verifies that depth one step
// over the threshold triggers the degenerating_research_programme event with
// all required attributes.
func TestResearchProgramme_OverThreshold_EventEmitted(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Threshold = 3. Rescuing a depth-3 claim → new depth = 4 > threshold.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 3
	require.NoError(t, k.SetParams(ctx, params))

	setClaimWithDepth(t, k, ctx, "depth-3-claim", 3)

	sub := makeValidBech32Addr("rp-over-threshold")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "rescue that exceeds the degeneration threshold",
		Domain:         "physics",
		Category:       "mechanics",
		Stake:          "1000000",
		RescuesClaimId: "depth-3-claim",
	})
	require.NoError(t, err)

	// Depth must be stored correctly.
	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(4), stored.ResearchProgrammeDepth,
		"depth must be rescued.depth+1 = 4")

	// Degeneration event must be emitted.
	evFound, attrs := hasDegeneratingEvent(ctx)
	require.True(t, evFound,
		"depth=4 > threshold=3 → degenerating_research_programme event must be emitted")

	// Verify required attributes.
	require.Equal(t, resp.ClaimId, attrs["claim_id"],
		"event must identify the new claim")
	require.Equal(t, "depth-3-claim", attrs["rescues_claim_id"],
		"event must identify the rescued claim")
	require.Equal(t, "4", attrs["research_programme_depth"],
		"event must carry the computed depth")
	require.Equal(t, "3", attrs["degeneration_threshold"],
		"event must carry the configured threshold")
	require.Equal(t, sub, attrs["submitter"],
		"event must identify the submitter")
	require.NotEmpty(t, attrs["lakatos_criterion"],
		"event must carry the Lakatos criterion description")
}

// TestResearchProgramme_ManyOverThreshold_EmitsOnce verifies that even when
// the chain is very deep (far over threshold), the event is emitted exactly
// once per SubmitClaim call — not accumulated.
func TestResearchProgramme_ManyOverThreshold_EmitsOnce(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "chemistry",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Threshold = 2. Rescuing a depth-10 claim → new depth = 11 >> threshold.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 2
	require.NoError(t, k.SetParams(ctx, params))

	setClaimWithDepth(t, k, ctx, "depth-10-claim", 10)

	sub := makeValidBech32Addr("rp-many-over")
	_, err = ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      sub,
		FactContent:    "rescue far over the degeneration threshold",
		Domain:         "chemistry",
		Category:       "combustion",
		Stake:          "1000000",
		RescuesClaimId: "depth-10-claim",
	})
	require.NoError(t, err)

	// Count degenerating events — must be exactly 1.
	count := 0
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.degenerating_research_programme" {
			count++
		}
	}
	require.Equal(t, 1, count,
		"exactly one degenerating_research_programme event per SubmitClaim call, regardless of how far over threshold")
}
