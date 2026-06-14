package keeper

// defeater.go — Skepticism Gap 1: Pollock Undercutting Defeater Tracking.
//
// John Pollock (1986) distinguished two kinds of defeaters for a belief B
// inferred from evidence E:
//
//   REBUTTING DEFEATER  — directly contradicts B.
//   UNDERCUTTING DEFEATER — does not contradict B, but attacks the
//     evidential connection between E and B. It says "E is not good
//     evidence for B in this situation" without asserting ¬B.
//
// On-chain analogue:
//   When a challenge claim is REJECTED (the original fact survives), the
//   rejection verdict is evidence FOR the fact. But the strength of that
//   evidence depends on HOW convincingly the challenge was rejected.
//
//   High rejection confidence (e.g. 90%) = strong evidence; the challenge
//   was decisively defeated.
//
//   Low rejection confidence (e.g. 52%) = weak evidence; a meaningful
//   minority doubted the fact's validity. The challenge survived by a slim
//   margin. This slim margin is an undercutting defeater: the evidence (the
//   rejection) is present but is not robustly supporting.
//
// Implementation:
//   - Triggered from handleChallengeSurvival when rejectionConfidenceBps
//     is below Params.DefeaterOppositionThresholdBps.
//   - Increments Fact.DefeaterCount and recomputes Fact.EffectiveConfidenceBps.
//   - Emits zerone.knowledge.defeater_recorded.
//   - Disabled by default (threshold=0 means all survivals are corroborations).
//
// EffectiveConfidenceBps formula:
//
//	effective = base_confidence × (1 − defeater_count × discount_bps / 1_000_000)
//	clamped to [0, base_confidence].
//
// The base_confidence is the fact's stored Confidence field (the PoT panel
// confidence at acceptance time, potentially raised by corroboration events
// via confidence growth). EffectiveConfidenceBps is NOT used to mutate
// Confidence — it is a separate epistemic readout for query engines, training
// pipelines, and analytics that want Pollock-adjusted scores.

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// recordDefeaterIfApplicable checks whether a fact-survival event should be
// recorded as an undercutting defeater and, if so, updates the surviving fact.
//
// Parameters:
//   - factID: the surviving fact's ID
//   - rejectionConfidenceBps: the panel's confidence in the REJECT verdict
//     (higher = more decisive rejection = stronger corroboration)
//
// The function is a no-op when:
//   - DefeaterOppositionThresholdBps == 0 (feature disabled)
//   - The fact cannot be found in state
//   - rejectionConfidenceBps >= threshold (strong rejection = pure corroboration)
func (k Keeper) recordDefeaterIfApplicable(ctx context.Context, factID string, rejectionConfidenceBps uint64) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return
	}
	threshold := params.DefeaterOppositionThresholdBps
	if threshold == 0 {
		// Feature disabled — all challenge survivals are pure corroborations.
		return
	}
	if rejectionConfidenceBps >= threshold {
		// Rejection was decisive — no undercutting signal.
		return
	}

	fact, found := k.GetFact(ctx, factID)
	if !found {
		return
	}

	// Record the undercutting defeater.
	fact.DefeaterCount++

	// Recompute effective confidence.
	discountBps := params.DefeaterPerCountDiscountBps
	fact.EffectiveConfidenceBps = computeEffectiveConfidence(fact.Confidence, fact.DefeaterCount, discountBps)

	if err := k.SetFact(ctx, fact); err != nil {
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.defeater_recorded",
		sdk.NewAttribute("fact_id", factID),
		sdk.NewAttribute("defeater_count", fmt.Sprintf("%d", fact.DefeaterCount)),
		sdk.NewAttribute("rejection_confidence_bps", fmt.Sprintf("%d", rejectionConfidenceBps)),
		sdk.NewAttribute("threshold_bps", fmt.Sprintf("%d", threshold)),
		sdk.NewAttribute("effective_confidence_bps", fmt.Sprintf("%d", fact.EffectiveConfidenceBps)),
		sdk.NewAttribute("pollock_criterion", "undercutting defeater: rejection confidence below opposition threshold"),
	))
}

// computeEffectiveConfidence applies the defeater discount formula:
//
//	effective = confidence × (1 − defeaterCount × discountBps / 1_000_000)
//	clamped to [0, confidence].
//
// When discountBps == 0, effective == confidence (no discount).
func computeEffectiveConfidence(confidenceBps, defeaterCount, discountBps uint64) uint64 {
	if discountBps == 0 || defeaterCount == 0 {
		return confidenceBps
	}
	// Total discount fraction = defeaterCount × discountBps (in BPS units).
	totalDiscountBps := defeaterCount * discountBps
	if totalDiscountBps >= BPS {
		// Fully discounted — effective confidence is zero.
		return 0
	}
	// effective = confidence × (BPS − totalDiscount) / BPS
	return confidenceBps * (BPS - totalDiscountBps) / BPS
}

// RecordDefeaterIfApplicableForTest exports recordDefeaterIfApplicable
// for use in unit tests outside the keeper package.
func (k Keeper) RecordDefeaterIfApplicableForTest(ctx context.Context, factID string, rejectionConfidenceBps uint64) {
	k.recordDefeaterIfApplicable(ctx, factID, rejectionConfidenceBps)
}

// ComputeEffectiveConfidenceForTest exports computeEffectiveConfidence
// for direct unit testing of the formula.
func ComputeEffectiveConfidenceForTest(confidenceBps, defeaterCount, discountBps uint64) uint64 {
	return computeEffectiveConfidence(confidenceBps, defeaterCount, discountBps)
}
