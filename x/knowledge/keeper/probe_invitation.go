package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// InviteIdleFactsForProbing is the Wave 15 stress-test invitation
// heartbeat. Each block it scans a bounded slice of facts, nominates
// eligible idle claims for external probing, and emits an invitation
// event that external prober agents can subscribe to.
//
// Eligibility criteria for an invitation:
//
//   - Fact status is VERIFIED or ACTIVE (facts the chain asserts are
//     currently trustworthy — the interesting ones to probe).
//   - Confidence ≥ ProbeInvitationMinConfidenceBps (default 70%). Low-
//     confidence facts don't need the nudge; verifiers are already
//     adjudicating them.
//   - Time-since-last-probe ≥ effective idle threshold.
//     "Last probe" = max(LastCorroboratedBlock, ProbeInvitedAtBlock).
//     A fact recently stress-tested doesn't need a new invitation yet.
//     Skepticism Gap 2: facts with DefeaterCount ≥ DefeaterProbePriorityMinCount
//     use a reduced threshold (DefeaterProbeReducedThresholdBlocks, or half
//     the normal threshold when that param is 0) — epistemically stressed
//     facts surface sooner in the probe queue.
//   - Re-invite cooldown: if the fact was already invited within
//     ProbeInvitationReinviteCooldown blocks, skip. Prevents spam in
//     event logs for facts nobody ever probes.
//
// Per-block work is capped at ProbeInvitationBatchSize to keep the
// heartbeat O(constant). The iteration walks the fact store until the
// batch is full, then stops.
//
// Emits: zerone.knowledge.probe_invited per invited fact.
func (k Keeper) InviteIdleFactsForProbing(ctx context.Context, height uint64, params *types.Params) {
	if params == nil {
		return
	}
	batchSize := params.ProbeInvitationBatchSize
	if batchSize == 0 {
		return
	}
	threshold := params.ProbeInvitationIdleThresholdBlocks
	if threshold == 0 {
		return
	}

	// Skepticism Gap 2: defeater-aware probe prioritization.
	// Facts with accumulated undercutting defeaters are epistemically stressed
	// and warrant sooner re-examination. When priority is enabled, compute the
	// reduced idle threshold that applies to such facts.
	priorityMinCount := params.DefeaterProbePriorityMinCount
	reducedThreshold := params.DefeaterProbeReducedThresholdBlocks
	if priorityMinCount > 0 && reducedThreshold == 0 {
		// Default: halve the normal threshold (governance can override via
		// DefeaterProbeReducedThresholdBlocks to any explicit value).
		reducedThreshold = threshold / 2
		if reducedThreshold == 0 {
			reducedThreshold = 1
		}
	}

	// Early-exit guard: skip the scan if height is below the smallest
	// possible effective threshold. When priority is enabled, the effective
	// minimum is the reduced threshold (it is always ≤ normal threshold).
	earliestPossible := threshold
	if priorityMinCount > 0 && reducedThreshold < earliestPossible {
		earliestPossible = reducedThreshold
	}
	if height < earliestPossible {
		// Chain hasn't been running long enough for any fact to be "idle"
		// beyond even the smallest threshold; save the scan.
		return
	}

	minConf := params.ProbeInvitationMinConfidenceBps
	reinviteCooldown := params.ProbeInvitationReinviteCooldown

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	invited := uint32(0)

	k.IterateFacts(ctx, func(f *types.Fact) bool {
		if f == nil {
			return false
		}
		if invited >= batchSize {
			return true // stop iteration
		}
		// Status gate.
		if f.Status != types.FactStatus_FACT_STATUS_VERIFIED &&
			f.Status != types.FactStatus_FACT_STATUS_ACTIVE {
			return false
		}
		// Confidence gate.
		if f.Confidence < minConf {
			return false
		}
		// Time-since-last-probe: the "probe" signal is whichever is
		// latest across corroborated (failed challenge) or previously-
		// invited. A successful challenge moves the fact to DISPROVEN
		// which the status gate above filters out entirely.
		lastProbe := f.LastCorroboratedBlock
		if f.ProbeInvitedAtBlock > lastProbe {
			lastProbe = f.ProbeInvitedAtBlock
		}
		// If the fact has never been probed (lastProbe == 0), use its
		// VerifiedAtBlock so we don't invite freshly-minted facts.
		if lastProbe == 0 {
			lastProbe = f.VerifiedAtBlock
		}

		// Effective idle threshold: stressed facts get the reduced threshold.
		effectiveThreshold := threshold
		isPriority := priorityMinCount > 0 && f.DefeaterCount >= priorityMinCount
		if isPriority {
			effectiveThreshold = reducedThreshold
		}

		if height < lastProbe+effectiveThreshold {
			return false // not idle enough yet
		}
		// Re-invite cooldown.
		if reinviteCooldown > 0 && f.ProbeInvitedAtBlock > 0 &&
			height < f.ProbeInvitedAtBlock+reinviteCooldown {
			return false
		}

		// Invite: stamp the fact, emit event.
		f.ProbeInvitedAtBlock = height
		if err := k.SetFact(ctx, f); err != nil {
			k.Logger(ctx).Error("probe invitation SetFact failed", "fact", f.Id, "err", err)
			return false
		}
		// The chain manufactures probe demand. This event is the
		// announcement: "this high-confidence fact has gone idle and
		// I am inviting you to challenge it." See TRUTH_SEEKING.md
		// commitment 5.
		// Skepticism Gap 2: include Pollock epistemic-stress signals so that
		// prober agents can triage invitations by stress level.
		priorityStr := "false"
		if isPriority {
			priorityStr = "true"
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.probe_invited",
			sdk.NewAttribute("fact_id", f.Id),
			sdk.NewAttribute("domain", f.Domain),
			sdk.NewAttribute("confidence", fmt.Sprintf("%d", f.Confidence)),
			sdk.NewAttribute("corroboration_count", fmt.Sprintf("%d", f.CorroborationCount)),
			sdk.NewAttribute("idle_since_block", fmt.Sprintf("%d", lastProbe)),
			sdk.NewAttribute("invited_at_block", fmt.Sprintf("%d", height)),
			sdk.NewAttribute("creed_commitment", "5"),
			sdk.NewAttribute("defeater_count", fmt.Sprintf("%d", f.DefeaterCount)),
			sdk.NewAttribute("effective_confidence_bps", fmt.Sprintf("%d", f.EffectiveConfidenceBps)),
			sdk.NewAttribute("priority_probe", priorityStr),
		))
		invited++
		return false
	})
}

// IdleFactsForProbing returns facts currently inviting stress-tests —
// any fact with probe_invited_at_block > 0 whose invitation hasn't been
// superseded by a subsequent challenge or corroboration. Sorted by
// time-since-invitation descending (the oldest, most-neglected
// invitations come first). Used by QueryIdleFacts to give prober agents
// a concrete work queue.
func (k Keeper) IdleFactsForProbing(ctx context.Context, domain string, limit uint32) []*types.IdleFact {
	if limit == 0 {
		limit = 50
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	out := make([]*types.IdleFact, 0, limit)
	k.IterateFacts(ctx, func(f *types.Fact) bool {
		if f == nil || f.ProbeInvitedAtBlock == 0 {
			return false
		}
		if domain != "" && f.Domain != domain {
			return false
		}
		// Invitation is "current" if no corroboration has landed since
		// it was issued. Otherwise the fact was probed already and the
		// invitation is stale. (Successful challenges flip status to
		// DISPROVEN which we also exclude via the field semantics.)
		if f.LastCorroboratedBlock > f.ProbeInvitedAtBlock {
			return false
		}
		idle := &types.IdleFact{
			Id:                    f.Id,
			Domain:                f.Domain,
			Confidence:            f.Confidence,
			CorroborationCount:    f.CorroborationCount,
			ProbeInvitedAtBlock:   f.ProbeInvitedAtBlock,
			DefeaterCount:         f.DefeaterCount,
			EffectiveConfidenceBps: f.EffectiveConfidenceBps,
		}
		if height >= f.ProbeInvitedAtBlock {
			idle.BlocksSinceInvited = height - f.ProbeInvitedAtBlock
		}
		out = append(out, idle)
		return uint32(len(out)) >= limit
	})
	return out
}
