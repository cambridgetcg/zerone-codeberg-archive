package keeper

// defeater.go — Skepticism Gaps 1 + 5: Pollock Defeater Tracking.
//
// John Pollock (1986) distinguished two kinds of defeaters for a belief B
// inferred from evidence E:
//
//   REBUTTING DEFEATER  — directly contradicts B.
//   UNDERCUTTING DEFEATER — does not contradict B, but attacks the
//     evidential connection between E and B. It says "E is not good
//     evidence for B in this situation" without asserting ¬B.
//
// ─── Skepticism Gap 1: Undercutting Defeater ─────────────────────────────
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
// ─── Skepticism Gap 5: Rebutting Defeater ────────────────────────────────
// On-chain analogue:
//   When a claim carrying RELATION_TYPE_CONTRADICTS pointing to an existing
//   fact is ACCEPTED, the accepted contradicting fact is a rebutting defeater.
//   The chain now holds two directly contradictory accepted facts; the older
//   fact's epistemic standing must be reduced.
//
// Implementation:
//   - Triggered from createFactFromClaim for each CONTRADICTS relation in the
//     newly accepted fact.
//   - Increments Fact.RebuttingDefeaterCount and recomputes EffectiveConfidenceBps.
//   - Emits zerone.knowledge.rebutting_defeater_recorded.
//   - Disabled by default (rebutting_defeater_discount_bps=0).
//
// ─── Combined EffectiveConfidenceBps formula ─────────────────────────────
//
//	total_discount = (defeater_count × defeater_per_count_discount_bps
//	               + rebutting_defeater_count × rebutting_defeater_discount_bps)
//	effective = base_confidence × max(0, BPS − total_discount) / BPS
//
// The base_confidence is the fact's stored Confidence field. EffectiveConfidenceBps
// is a separate epistemic readout — it does NOT mutate Confidence.

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

	// Recompute effective confidence accounting for both undercutting and
	// any existing rebutting defeaters (Gap 5).
	fact.EffectiveConfidenceBps = computeCombinedEffectiveConfidence(
		fact.Confidence,
		fact.DefeaterCount, params.DefeaterPerCountDiscountBps,
		fact.RebuttingDefeaterCount, params.RebuttingDefeaterDiscountBps,
	)

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

// recordRebuttingDefeater records a rebutting defeater (Skepticism Gap 5)
// on targetFactID when an accepted fact directly contradicts it.
//
// A rebutting defeater (Pollock, 1986) asserts the negation of a belief B —
// it directly contradicts rather than merely attacking evidential support.
// When the verification panel accepts a claim with RELATION_TYPE_CONTRADICTS
// pointing to an existing fact, that fact now has an on-chain accepted
// contradiction: its epistemic standing must be reduced.
//
// The function is a no-op when:
//   - RebuttingDefeaterDiscountBps == 0 (feature disabled)
//   - targetFactID is not found in state
func (k Keeper) recordRebuttingDefeater(ctx context.Context, targetFactID, contradictingFactID string) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return
	}
	discountBps := params.RebuttingDefeaterDiscountBps
	if discountBps == 0 {
		// Feature disabled — rebutting defeaters do not reduce effective confidence.
		return
	}

	fact, found := k.GetFact(ctx, targetFactID)
	if !found {
		return
	}

	// Record the rebutting defeater.
	fact.RebuttingDefeaterCount++

	// Recompute effective confidence accounting for both undercutting (Gap 1)
	// and rebutting (Gap 5) defeaters.
	fact.EffectiveConfidenceBps = computeCombinedEffectiveConfidence(
		fact.Confidence,
		fact.DefeaterCount, params.DefeaterPerCountDiscountBps,
		fact.RebuttingDefeaterCount, discountBps,
	)

	if err := k.SetFact(ctx, fact); err != nil {
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.rebutting_defeater_recorded",
		sdk.NewAttribute("fact_id", targetFactID),
		sdk.NewAttribute("contradicting_fact_id", contradictingFactID),
		sdk.NewAttribute("rebutting_defeater_count", fmt.Sprintf("%d", fact.RebuttingDefeaterCount)),
		sdk.NewAttribute("effective_confidence_bps", fmt.Sprintf("%d", fact.EffectiveConfidenceBps)),
		sdk.NewAttribute("pollock_criterion", "rebutting defeater: accepted fact directly contradicts target"),
	))
}

// computeCombinedEffectiveConfidence applies both undercutting and rebutting
// defeater discounts to a fact's base confidence.
//
//	total_discount = (undercuttingCount × undercuttingDiscountBps
//	               + rebuttingCount    × rebuttingDiscountBps)
//	effective = confidence × max(0, BPS − total_discount) / BPS
//
// Either discount component is 0 when the corresponding feature is disabled.
func computeCombinedEffectiveConfidence(
	confidenceBps,
	undercuttingCount, undercuttingDiscountBps,
	rebuttingCount, rebuttingDiscountBps uint64,
) uint64 {
	undercuttingTotal := undercuttingCount * undercuttingDiscountBps
	rebuttingTotal := rebuttingCount * rebuttingDiscountBps
	totalDiscount := undercuttingTotal + rebuttingTotal
	if totalDiscount == 0 {
		return confidenceBps
	}
	if totalDiscount >= BPS {
		return 0
	}
	return confidenceBps * (BPS - totalDiscount) / BPS
}

// computeEffectiveConfidence applies the undercutting-only defeater discount:
//
//	effective = confidence × (1 − defeaterCount × discountBps / 1_000_000)
//	clamped to [0, confidence].
//
// Preserved for backward compatibility with existing callers and tests.
// New code should prefer computeCombinedEffectiveConfidence.
func computeEffectiveConfidence(confidenceBps, defeaterCount, discountBps uint64) uint64 {
	return computeCombinedEffectiveConfidence(confidenceBps, defeaterCount, discountBps, 0, 0)
}

// RecordDefeaterIfApplicableForTest exports recordDefeaterIfApplicable
// for use in unit tests outside the keeper package.
func (k Keeper) RecordDefeaterIfApplicableForTest(ctx context.Context, factID string, rejectionConfidenceBps uint64) {
	k.recordDefeaterIfApplicable(ctx, factID, rejectionConfidenceBps)
}

// RecordRebuttingDefeaterForTest exports recordRebuttingDefeater
// for use in unit tests outside the keeper package.
func (k Keeper) RecordRebuttingDefeaterForTest(ctx context.Context, targetFactID, contradictingFactID string) {
	k.recordRebuttingDefeater(ctx, targetFactID, contradictingFactID)
}

// ComputeEffectiveConfidenceForTest exports computeEffectiveConfidence
// for direct unit testing of the formula.
func ComputeEffectiveConfidenceForTest(confidenceBps, defeaterCount, discountBps uint64) uint64 {
	return computeEffectiveConfidence(confidenceBps, defeaterCount, discountBps)
}

// ComputeCombinedEffectiveConfidenceForTest exports computeCombinedEffectiveConfidence
// for direct unit testing of the combined formula.
func ComputeCombinedEffectiveConfidenceForTest(
	confidenceBps,
	undercuttingCount, undercuttingDiscountBps,
	rebuttingCount, rebuttingDiscountBps uint64,
) uint64 {
	return computeCombinedEffectiveConfidence(
		confidenceBps,
		undercuttingCount, undercuttingDiscountBps,
		rebuttingCount, rebuttingDiscountBps,
	)
}
