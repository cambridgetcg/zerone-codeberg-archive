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

	c.VerifierCalibrationScoreBps = ComputeVerifierCalibrationScore(c)
	c.LastUpdatedBlock = height
	_ = k.SetAgentCalibration(ctx, c)
	k.EmitCalibrationUpdated(ctx, c)
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
	))
}
