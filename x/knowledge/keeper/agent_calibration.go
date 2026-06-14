package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// Calibration score formula constants. These are intentionally conservative
// under Phase 5 — the score rewards consistency over volume and penalizes
// post-acceptance disprovals, matching the training-loop intuition that a
// model's reliability is measured by how often its outputs survive.
const (
	// Max corroboration bonus contribution (BPS). 10% added to a perfect
	// acceptance-rate score if the submitter's facts earn corroborations.
	CalibrationCorroborationBonusCapBps uint64 = 100_000
	// Per-corroboration-per-acceptance scaling into the bonus. Tuned so that
	// 1 corroboration per accepted fact approaches the cap.
	CalibrationCorroborationScaleBps uint64 = 100_000
)

// ─── CRUD ────────────────────────────────────────────────────────────────

// SetAgentCalibration stores the calibration record.
func (k Keeper) SetAgentCalibration(ctx context.Context, c *types.AgentCalibration) error {
	if c == nil || c.Address == "" {
		return nil
	}
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(c)
	if err != nil {
		return err
	}
	return store.Set(types.AgentCalibrationKey(c.Address), bz)
}

// GetAgentCalibration fetches a calibration record, returning a zero-valued
// struct if absent (so callers can treat "never submitted" as "empty stats").
func (k Keeper) GetAgentCalibration(ctx context.Context, addr string) (*types.AgentCalibration, bool) {
	if addr == "" {
		return nil, false
	}
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.AgentCalibrationKey(addr))
	if err != nil || bz == nil {
		return nil, false
	}
	var c types.AgentCalibration
	if err := proto.Unmarshal(bz, &c); err != nil {
		return nil, false
	}
	return &c, true
}

// IterateAgentCalibrations yields every calibration record.
func (k Keeper) IterateAgentCalibrations(ctx context.Context, cb func(*types.AgentCalibration) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.AgentCalibrationKeyPrefix, prefixEndBytes(types.AgentCalibrationKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var c types.AgentCalibration
		if err := proto.Unmarshal(iter.Value(), &c); err != nil {
			continue
		}
		if cb(&c) {
			return
		}
	}
}

// getOrInit returns the stored calibration or a fresh record bound to addr.
// Callers should call SetAgentCalibration after mutation.
func (k Keeper) getOrInitCalibration(ctx context.Context, addr string) *types.AgentCalibration {
	if c, found := k.GetAgentCalibration(ctx, addr); found {
		return c
	}
	accountType := ""
	if k.zeroneAuthKeeper != nil {
		accountType = k.getAccountType(ctx, addr)
	}
	return &types.AgentCalibration{
		Address:     addr,
		AccountType: accountType,
		PerMethod:   map[string]*types.AgentMethodStats{},
	}
}

// ensureMethodStats returns the mutable per-method slot for a calibration,
// creating it if needed.
func ensureMethodStats(c *types.AgentCalibration, methodId string) *types.AgentMethodStats {
	if c.PerMethod == nil {
		c.PerMethod = map[string]*types.AgentMethodStats{}
	}
	stats, ok := c.PerMethod[methodId]
	if !ok {
		stats = &types.AgentMethodStats{}
		c.PerMethod[methodId] = stats
	}
	return stats
}

// ─── Update sites ────────────────────────────────────────────────────────

// RecordSubmissionOutcome is called from CompleteRound after a verdict is
// determined. Updates the submitter's lifetime + per-method stats.
func (k Keeper) RecordSubmissionOutcome(
	ctx context.Context,
	submitter string,
	methodId string,
	verdict types.Verdict,
) {
	if submitter == "" {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, submitter)
	if c.FirstSubmissionBlock == 0 {
		c.FirstSubmissionBlock = height
	}
	c.LastSubmissionBlock = height
	c.TotalSubmissions++

	stats := ensureMethodStats(c, methodId)
	stats.Submissions++

	switch verdict {
	case types.Verdict_VERDICT_ACCEPT:
		c.Accepted++
		stats.Accepted++
	case types.Verdict_VERDICT_REJECT:
		c.Rejected++
		stats.Rejected++
	case types.Verdict_VERDICT_MALFORMED:
		c.Malformed++
	case types.Verdict_VERDICT_INCONCLUSIVE:
		c.Inconclusive++
	}

	c.CalibrationScoreBps = ComputeAgentCalibrationScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
}

// RecordCorroborationForSubmitter is called from handleChallengeSurvival
// (a challenge was rejected → the cited fact survived).
func (k Keeper) RecordCorroborationForSubmitter(
	ctx context.Context,
	submitter string,
	methodId string,
) {
	if submitter == "" {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, submitter)
	c.CorroborationsEarned++
	stats := ensureMethodStats(c, methodId)
	stats.CorroborationsEarned++

	c.CalibrationScoreBps = ComputeAgentCalibrationScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
}

// RecordDisprovalForSubmitter is called from cascadeFalsification / direct
// disproof — the submitter's previously-accepted fact was invalidated.
func (k Keeper) RecordDisprovalForSubmitter(
	ctx context.Context,
	submitter string,
	methodId string,
) {
	if submitter == "" {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, submitter)
	c.DisprovenCount++
	stats := ensureMethodStats(c, methodId)
	stats.Disproven++

	c.CalibrationScoreBps = ComputeAgentCalibrationScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
}

// RecordChallengeOutcome tracks the challenger's side of the ledger. A
// challenge is "successful" when the target fact is marked DISPROVEN.
func (k Keeper) RecordChallengeOutcome(
	ctx context.Context,
	challenger string,
	succeeded bool,
) {
	if challenger == "" {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, challenger)
	c.ChallengesIssued++
	if succeeded {
		c.ChallengesSucceeded++
	} else {
		c.ChallengesFailed++
	}
	c.CalibrationScoreBps = ComputeAgentCalibrationScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
}

// ─── Score computation ──────────────────────────────────────────────────

// ComputeAgentCalibrationScore returns a BPS score in [0, BPS] summarising a
// submitter's track record. Formula:
//
//	acceptance_rate = accepted / total_submissions        (zero if no subs)
//	corr_bonus      = min(cap, corroborations × scale / accepted)
//	disproval_pen   = disproven × BPS / accepted
//	score           = acceptance_rate + corr_bonus - disproval_pen
//	                  clamped to [0, BPS]
//
// The score is intentionally coarse — it is a public signal, not a
// governance-critical parameter. Training pipelines can weight sampling by
// it; selection / reward logic should NOT depend on it until Phase 5
// iterations validate the formula under adversarial load.
func ComputeAgentCalibrationScore(c *types.AgentCalibration) uint64 {
	if c == nil || c.TotalSubmissions == 0 {
		return 0
	}
	const bps uint64 = 1_000_000

	acceptanceBps := c.Accepted * bps / c.TotalSubmissions

	var corrBonus uint64
	if c.Accepted > 0 {
		corrBonus = c.CorroborationsEarned * CalibrationCorroborationScaleBps / c.Accepted
		if corrBonus > CalibrationCorroborationBonusCapBps {
			corrBonus = CalibrationCorroborationBonusCapBps
		}
	}

	var disprovalPenalty uint64
	if c.Accepted > 0 {
		disprovalPenalty = c.DisprovenCount * bps / c.Accepted
	}

	// Compose — signed-ish math in uint64 space.
	score := acceptanceBps + corrBonus
	if disprovalPenalty >= score {
		return 0
	}
	score -= disprovalPenalty
	if score > bps {
		score = bps
	}
	return score
}

// ─── Verifier Brier scoring (Wave 4) ────────────────────────────────────────

// RecordVerifierOutcome updates Brier-score calibration for a verifier after a
// round resolves to a binary verdict (ACCEPT or REJECT). Called from
// CompleteRound for each reveal that carries a non-zero ConfidenceBps.
//
// The Brier score for a single prediction is (p − o)² where:
//   - p = stated probability that the claim is TRUE, in [0, 1]
//   - o = actual outcome: 1 if claim was accepted, 0 if rejected
//
// The function converts vote+confidence to P(ACCEPT) so that confidence
// always expresses belief about the underlying claim, not about the verifier's
// own vote:
//   - votedAccept=true:  P(ACCEPT) = confidenceBps / 1_000_000
//   - votedAccept=false: P(ACCEPT) = 1 - (confidenceBps / 1_000_000)
//
// The running mean Brier score is stored as BPS (0..1_000_000) and inverted
// in ComputeVerifierCalibrationScore to produce a score where 1_000_000 is
// perfect calibration.
func (k Keeper) RecordVerifierOutcome(
	ctx context.Context,
	verifier string,
	methodId string,
	confidenceBps uint64,
	votedAccept bool,
	outcomeAccept bool,
) {
	if verifier == "" || confidenceBps == 0 {
		// Skip absent confidence — pre-Wave-4 reveals or confidence not reported.
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, verifier)
	stats := ensureMethodStats(c, methodId)

	// Convert vote+confidence → P(claim is TRUE) in BPS.
	var pAcceptBps uint64
	if votedAccept {
		pAcceptBps = confidenceBps
	} else {
		// Confidence in REJECT vote → complement expresses belief in ACCEPT.
		if confidenceBps > 1_000_000 {
			confidenceBps = 1_000_000
		}
		pAcceptBps = 1_000_000 - confidenceBps
	}

	// Outcome in BPS (1_000_000 = accepted, 0 = rejected).
	var oAcceptBps int64
	if outcomeAccept {
		oAcceptBps = 1_000_000
	}

	// Brier error = (P_bps − O_bps)² / 1_000_000, normalised back to BPS scale.
	// diff ∈ [−1_000_000, 1_000_000] → diff² ≤ 10^12 → fits uint64.
	diff := int64(pAcceptBps) - oAcceptBps
	brierBps := uint64(diff*diff) / 1_000_000

	c.VerifierRounds++
	c.VerifierSumBrierBps += brierBps
	stats.VerifierRounds++
	stats.VerifierSumBrierBps += brierBps

	// Calibration curve: update per-confidence-bin hit-rate tracking.
	// Bucket k covers P(ACCEPT) ∈ [k×100_000, (k+1)×100_000).
	recordCalibrationBucket(c, pAcceptBps, outcomeAccept)

	c.VerifierCalibrationScoreBps = ComputeVerifierCalibrationScore(c)
	c.VerifierResolutionBps = ComputeVerifierResolutionScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
}

// recordCalibrationBucket increments the calibration curve bucket for a single
// verifier outcome. The curve has 10 equal-width buckets of 100_000 BPS each.
// Bucket index k covers P(ACCEPT) ∈ [k×100_000, (k+1)×100_000); bucket 9 is
// [900_000, 1_000_000] (inclusive for pAcceptBps = 1_000_000).
func recordCalibrationBucket(c *types.AgentCalibration, pAcceptBps uint64, outcomeAccept bool) {
	// Initialize 10 buckets if this is the first verifier outcome.
	if len(c.CalibrationCurve) != 10 {
		c.CalibrationCurve = make([]*types.CalibrationBucket, 10)
		for i := 0; i < 10; i++ {
			c.CalibrationCurve[i] = &types.CalibrationBucket{
				MinConfidenceBps: uint64(i) * 100_000,
				MaxConfidenceBps: uint64(i+1) * 100_000,
			}
		}
		// Bucket 9 is inclusive at 1_000_000.
		c.CalibrationCurve[9].MaxConfidenceBps = 1_000_001 // sentinel: > max possible
	}

	// Determine bucket index.
	bucketIdx := pAcceptBps / 100_000
	if bucketIdx >= 10 {
		bucketIdx = 9 // clamp 1_000_000 into the last bucket
	}

	c.CalibrationCurve[bucketIdx].TotalCount++
	if outcomeAccept {
		c.CalibrationCurve[bucketIdx].CorrectCount++
	}
}

// RecordCalibrationBucketForTest is an exported thin wrapper for use in unit tests
// that want to test bucket logic without setting up a full keeper context.
func RecordCalibrationBucketForTest(c *types.AgentCalibration, pAcceptBps uint64, outcomeAccept bool) {
	recordCalibrationBucket(c, pAcceptBps, outcomeAccept)
}

// ComputeVerifierCalibrationScore derives a BPS calibration score in [0, 1_000_000]
// from accumulated Brier errors. Higher is better; 1_000_000 = perfect calibration.
// Returns 0 if the verifier has no rounds recorded.
func ComputeVerifierCalibrationScore(c *types.AgentCalibration) uint64 {
	if c == nil || c.VerifierRounds == 0 {
		return 0
	}
	avgBrierBps := c.VerifierSumBrierBps / c.VerifierRounds
	if avgBrierBps >= 1_000_000 {
		return 0
	}
	return 1_000_000 - avgBrierBps
}

// ComputeVerifierResolutionScore computes Murphy's resolution from the calibration curve.
//
// Resolution measures how much information the verifier's confidence reports carry —
// do they distinguish hard cases (likely-reject) from easy ones (likely-accept)?
//
// Murphy decomposition of the Brier score:
//
//	Resolution = Σ_k (n_k / N) × (hit_rate_k − overall_hit_rate)²
//
// where n_k = predictions in bucket k, N = total rounds, hit_rate_k = correct_k/n_k,
// overall_hit_rate = Σ correct_k / N.
//
// The maximum value is 0.25 (when half the predictions are at 0% and half at 100%,
// each correct). We scale by 4_000_000 to map [0, 0.25] → [0, 1_000_000] BPS.
//
// Returns 0 if the verifier has no calibration curve entries.
func ComputeVerifierResolutionScore(c *types.AgentCalibration) uint64 {
	if c == nil || len(c.CalibrationCurve) == 0 {
		return 0
	}

	var totalCount, totalCorrect uint64
	for _, b := range c.CalibrationCurve {
		totalCount += b.TotalCount
		totalCorrect += b.CorrectCount
	}
	if totalCount == 0 {
		return 0
	}

	// overall_hit_rate in BPS (scaled to 1_000_000)
	overallHitBps := totalCorrect * 1_000_000 / totalCount

	// Σ_k (n_k/N) × (hit_rate_k − overall_hit_rate)²
	// All values computed in BPS² space; divide by 1_000_000 to keep in BPS range.
	var resolutionBps2 uint64 // accumulates in BPS² / 1_000_000 = BPS units
	for _, b := range c.CalibrationCurve {
		if b.TotalCount == 0 {
			continue
		}
		hitRateBps := b.CorrectCount * 1_000_000 / b.TotalCount
		var diffBps uint64
		if hitRateBps >= overallHitBps {
			diffBps = hitRateBps - overallHitBps
		} else {
			diffBps = overallHitBps - hitRateBps
		}
		// weight × diff² in BPS² space; keep precision by dividing 1_000_000 once
		// (n_k / N) × diff² scaled: (n_k × diff² / 1_000_000) / totalCount
		resolutionBps2 += b.TotalCount * (diffBps * diffBps / 1_000_000)
	}
	resolutionBps2 /= totalCount

	// Scale from [0, 250_000] (max variance ≈ 0.25 × 1_000_000) to [0, 1_000_000]
	// by multiplying by 4.
	score := resolutionBps2 * 4
	if score > 1_000_000 {
		score = 1_000_000
	}
	return score
}

// ─── Bayesian Gap 2: Submitter Prior-Calibration Brier Scoring ───────────────
//
// When a claim resolves to a binary verdict (ACCEPT or REJECT) and the
// submitter stated a prior (prior_probability_bps != 0), the chain computes
// the submitter's Brier contribution:
//
//	contribution = (prior_bps − outcome_bps)² / 1_000_000
//
// where outcome_bps = 1_000_000 for ACCEPT, 0 for REJECT.
//
// A perfectly calibrated submitter who declares 70% on claims accepted 70%
// of the time will have a low Brier score. An overconfident submitter who
// always declares 99% on rejected claims will have a high score.
// Lower = better calibration.
//
// Distinct from verifier Brier scoring (RecordVerifierOutcome): this scores
// the *submitter* role — how well they know in advance whether their claim
// will be accepted. Well-calibrated submitters provide more epistemic signal
// even when both calibrated and poorly-calibrated submitters submit true claims.
//
// Fields updated: AgentCalibration.SubmitterPriorCount,
// SubmitterBrierSumBps, SubmitterBrierScoreBps (fields 29–31).
//
// Wiring: called from CompleteRound within the binary verdict block,
// immediately after RecordVerifierOutcome for all reveals.

// RecordSubmitterPriorOutcome updates the submitter's Bayesian prior-calibration
// Brier score after a binary verdict.
//
// priorBps: Claim.PriorProbabilityBps — 0 means no prior stated (skip).
// outcomeAccept: true if verdict was ACCEPT, false if REJECT.
func (k Keeper) RecordSubmitterPriorOutcome(
	ctx context.Context,
	submitter string,
	priorBps uint64,
	outcomeAccept bool,
) {
	if submitter == "" || priorBps == 0 {
		// priorBps == 0: no prior stated — backward compatible; nothing to score.
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	c := k.getOrInitCalibration(ctx, submitter)

	// Outcome in BPS: 1_000_000 for ACCEPT, 0 for REJECT.
	var oAcceptBps int64
	if outcomeAccept {
		oAcceptBps = 1_000_000
	}

	// Brier error = (prior_bps − outcome_bps)² / 1_000_000, normalised to BPS.
	// diff ∈ [−1_000_000, 1_000_000] → diff² ≤ 10^12 → fits int64/uint64.
	diff := int64(priorBps) - oAcceptBps
	brierBps := uint64(diff*diff) / 1_000_000

	c.SubmitterPriorCount++
	c.SubmitterBrierSumBps += brierBps
	c.SubmitterBrierScoreBps = c.SubmitterBrierSumBps / c.SubmitterPriorCount

	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	// EmitCalibrationUpdated fires from RecordSubmissionOutcome in the same
	// CompleteRound call — no duplicate event needed here.
}

// RecordSubmitterPriorOutcomeForTest exports RecordSubmitterPriorOutcome for
// white-box unit tests.
func (k Keeper) RecordSubmitterPriorOutcomeForTest(
	ctx context.Context,
	submitter string,
	priorBps uint64,
	outcomeAccept bool,
) {
	k.RecordSubmitterPriorOutcome(ctx, submitter, priorBps, outcomeAccept)
}

// EmitCalibrationUpdated emits an event for off-chain observers (training
// pipelines, dashboards) to track calibration changes without polling.
func (k Keeper) EmitCalibrationUpdated(ctx context.Context, c *types.AgentCalibration) {
	if c == nil {
		return
	}
	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.agent_calibration_updated",
		sdk.NewAttribute("address", c.Address),
		sdk.NewAttribute("account_type", c.AccountType),
		sdk.NewAttribute("total_submissions", fmt.Sprintf("%d", c.TotalSubmissions)),
		sdk.NewAttribute("accepted", fmt.Sprintf("%d", c.Accepted)),
		sdk.NewAttribute("disproven_count", fmt.Sprintf("%d", c.DisprovenCount)),
		sdk.NewAttribute("corroborations_earned", fmt.Sprintf("%d", c.CorroborationsEarned)),
		sdk.NewAttribute("calibration_score_bps", fmt.Sprintf("%d", c.CalibrationScoreBps)),
		sdk.NewAttribute("verifier_rounds", fmt.Sprintf("%d", c.VerifierRounds)),
		sdk.NewAttribute("verifier_calibration_score_bps", fmt.Sprintf("%d", c.VerifierCalibrationScoreBps)),
		sdk.NewAttribute("verifier_resolution_bps", fmt.Sprintf("%d", c.VerifierResolutionBps)),
	))
}
