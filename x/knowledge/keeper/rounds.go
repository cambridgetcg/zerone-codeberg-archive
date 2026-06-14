package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CreateVerificationRound creates a new verification round for a claim.
// priorityTipBps is the fraction of the verifier pool (BPS) earmarked as a
// priority tip for early committers. 0 = no tip (standard behavior).
func (k Keeper) CreateVerificationRound(ctx context.Context, claim *types.Claim, priorityTipBps uint64) (*types.VerificationRound, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	// Bohm Dialogue Gap 1: SUSPENDED claims cannot have verification rounds opened.
	// A SUSPENDED claim is held in a Bohmian mirror-state — visible and queryable,
	// but no PoT panel convenes until governance explicitly resumes it.
	if claim.Status == types.ClaimStatus_CLAIM_STATUS_SUSPENDED {
		return nil, types.ErrClaimNotSuspendable.Wrapf(
			"claim %q is SUSPENDED; resume it with MsgResumeClaim before opening a verification round",
			claim.Id,
		)
	}

	// Settlement Finality Gap 2: enforce minimum round duration.
	// Params.Validate() already checks this at MsgUpdateParams time; this guard
	// is belt-and-suspenders for cases where params were set before the field
	// existed (e.g. pre-upgrade state).
	if params.MinRoundDurationBlocks > 0 {
		totalPhaseDuration := params.CommitPhaseBlocks + params.RevealPhaseBlocks + params.AggregationPhaseBlocks
		if totalPhaseDuration < params.MinRoundDurationBlocks {
			return nil, fmt.Errorf(
				"round duration (%d blocks) is below min_round_duration_blocks (%d): increase commit/reveal/aggregation phase lengths",
				totalPhaseDuration, params.MinRoundDurationBlocks,
			)
		}
	}

	roundID := GenerateRoundID(claim.Id, height)

	// Skepticism Gap 3 (Sagan's maxim): extraordinary claims require extraordinary evidence.
	// Count CONTRADICTS relations. If count ≥ threshold (and threshold > 0), scale up
	// the round's quorum requirement by the governance-configured multiplier.
	var extraordinaryMinVerifiers uint64
	if params.ExtraordinaryClaimMinContradictions > 0 {
		var contradictCount uint64
		for _, rel := range claim.Relations {
			if rel.Relation == types.RelationType_RELATION_TYPE_CONTRADICTS {
				contradictCount++
			}
		}
		if contradictCount >= params.ExtraordinaryClaimMinContradictions {
			mult := params.ExtraordinaryClaimVerifierMultiplierBps
			if mult == 0 {
				mult = 2_000_000 // default 2× if somehow unset
			}
			extraordinaryMinVerifiers = params.MinVerifiers * mult / 1_000_000
			if extraordinaryMinVerifiers < params.MinVerifiers {
				extraordinaryMinVerifiers = params.MinVerifiers // floor at global min
			}
		}
	}

	round := &types.VerificationRound{
		Id:                        roundID,
		ClaimId:                   claim.Id,
		StartedAtBlock:            height,
		Phase:                     types.VerificationPhase_VERIFICATION_PHASE_COMMIT,
		CommitDeadline:            height + params.CommitPhaseBlocks,
		RevealDeadline:            height + params.CommitPhaseBlocks + params.RevealPhaseBlocks,
		AggregationDeadline:       height + params.CommitPhaseBlocks + params.RevealPhaseBlocks + params.AggregationPhaseBlocks,
		PriorityTipBps:            priorityTipBps,
		ExtraordinaryMinVerifiers: extraordinaryMinVerifiers,
	}

	if err := k.SetVerificationRound(ctx, round); err != nil {
		return nil, err
	}

	// Track active rounds for congestion-based fee backpressure.
	k.IncrementActiveRoundsCount(ctx)

	// Update claim with round reference
	claim.VerificationRoundId = roundID
	claim.Status = types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION
	if err := k.SetClaim(ctx, claim); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.verification_round_created",
		sdk.NewAttribute("round_id", roundID),
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("phase", "COMMIT"),
	))

	return round, nil
}

// CompleteRound finalizes a verification round based on the aggregated result.
func (k Keeper) CompleteRound(ctx context.Context, round *types.VerificationRound, result *VerificationResult) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	claim, found := k.GetClaim(ctx, round.ClaimId)
	if !found {
		return fmt.Errorf("claim %s not found for round %s", round.ClaimId, round.Id)
	}

	// Methodology gates: intercept ACCEPT verdicts for claims whose methodologies
	// impose additional compliance requirements beyond the panel vote itself.
	// The panel's epistemic work (calibration, scoring) is preserved regardless —
	// only the final verdict is blocked until requirements are met.
	//
	//   Pragmatism Gap 1:    M-PRAGMATIC needs min consequence instances
	//   Contemplative Gap:   M-PHENOMENOLOGICAL needs minimum round duration
	//   Scientific Gap 1:    All claims need minimum panel organizational diversity
	if result.Verdict == types.Verdict_VERDICT_ACCEPT {
		methodologyGateParams, _ := k.GetParams(ctx)
		if gateErr := k.CheckPragmaticConsequenceGate(ctx, *claim, *methodologyGateParams); gateErr != nil {
			result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
		}
		if result.Verdict == types.Verdict_VERDICT_ACCEPT {
			if gateErr := k.CheckMethodologyTemporalGate(ctx, *claim, *round, *methodologyGateParams, height); gateErr != nil {
				result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
			}
		}
		// Scientific Method Gap 1: panel organizational independence gate.
		// Compute diversity score regardless of verdict so it is always recorded.
		// Only block ACCEPT when threshold is configured.
		if result.Verdict == types.Verdict_VERDICT_ACCEPT {
			if gateErr := k.CheckPanelDiversityGate(round.PanelDiversityScoreBps, *methodologyGateParams); gateErr != nil {
				result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
			}
		}
		// Phenomenology Brief Gap 3: cross-modal corroboration gate.
		// High-confidence M-PHENOMENOLOGICAL claims require at least one
		// M-EMPIRICAL relation. Varela's neurophenomenological principle:
		// first-person description + third-person data must mutually constrain.
		if result.Verdict == types.Verdict_VERDICT_ACCEPT {
			if !k.CheckPhenomenologicalCorroborationGate(ctx, claim, result.Confidence, *methodologyGateParams) {
				result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
			}
		}
		// Contemplative Traditions Gap 4: cross-tradition convergence gate.
		// M-PHENOMENOLOGICAL only. Independent confirmation across lineages is
		// stronger evidence than same-tradition consensus.
		if result.Verdict == types.Verdict_VERDICT_ACCEPT {
			if gateErr := k.CheckCrossTraditionGate(ctx, claim, round.CrossTraditionScoreBps, *methodologyGateParams); gateErr != nil {
				result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
			}
		}
	}

	// Always compute and store the panel diversity scores so governance can audit
	// the organizational and cross-tradition independence of every panel.
	round.PanelDiversityScoreBps = k.ComputePanelDiversityScore(ctx, round)
	round.CrossTraditionScoreBps = k.ComputeCrossTraditionScore(ctx, round)

	round.Verdict = result.Verdict
	round.VerdictBlock = height
	round.VerdictBlockHash = sdkCtx.HeaderHash() // Settlement Finality Gap 1: anchors verdict to specific chain history
	round.Phase = types.VerificationPhase_VERIFICATION_PHASE_COMPLETE

	// Settlement Finality Gap 4: compute finality receipt hash.
	// receipt = sha256(round_id | "|" | verdict_string | "|" | verdict_block_BE | "|" | verdict_block_hash)
	// This deterministic commitment allows any IBC light client on a remote chain to
	// prove a specific zerone verdict via a standard Cosmos Merkle state proof.
	round.FinalityReceiptHash = computeFinalityReceiptHash(round.Id, round.Verdict.String(), height, round.VerdictBlockHash)

	// Hegelian Dialectic — Determinate Negation (Gap 1):
	// When the verdict is REJECT, aggregate the verifiers' negation_type diagnoses
	// and store the plurality on the round. "The negation is a determinate negation;
	// it has a content." — Hegel, Science of Logic §54.
	// This guides submitters toward what kind of more adequate successor claim to build.
	if result.Verdict == types.Verdict_VERDICT_REJECT {
		round.DeterminateNegationType = aggregateDeterminateNegationType(round.Reveals)
	}

	// Wave 15b: pay the invitation bonus BEFORE any verdict-specific side
	// effects consume the invitation state. handleChallengeSurvival will
	// bump LastCorroboratedBlock on REJECT, which makes the invitation
	// look stale retroactively. Check here, while the state still
	// reflects what the challenger saw when they submitted.
	if claim.ProvisionalFactId != "" {
		paramsEarly, _ := k.GetParams(ctx)
		k.payInvitationBonus(ctx, claim, paramsEarly)
	}

	// Record submitter calibration (Phase 5 — feedback loop). Every round
	// outcome updates the submitter's track record. Challenge claims are
	// handled below with their own challenger-side recording.
	k.RecordSubmissionOutcome(ctx, claim.Submitter, ResolveMethodId(claim.MethodId), result.Verdict)

	// Wave 4: record Brier-score verifier calibration for each participant
	// that reported a confidence value. Only binary verdicts (ACCEPT/REJECT)
	// produce a clear outcome to score against; skip MALFORMED/INCONCLUSIVE.
	if result.Verdict == types.Verdict_VERDICT_ACCEPT || result.Verdict == types.Verdict_VERDICT_REJECT {
		outcomeAccept := result.Verdict == types.Verdict_VERDICT_ACCEPT
		verifierMethodId := ResolveMethodId(claim.MethodId)
		for _, reveal := range round.Reveals {
			if reveal.ConfidenceBps == 0 {
				// Pre-Wave-4 reveal or verifier declined to report confidence.
				continue
			}
			votedAccept := reveal.Vote == "accept"
			k.RecordVerifierOutcome(ctx, reveal.Verifier, verifierMethodId, reveal.ConfidenceBps, votedAccept, outcomeAccept)
		}
		// Bayesian Gap 2: Submitter Prior-Calibration Brier Scoring.
		// Score the submitter's stated prior against the actual verdict.
		// No-op when prior_probability_bps == 0 (no prior stated).
		k.RecordSubmitterPriorOutcome(ctx, claim.Submitter, claim.PriorProbabilityBps, outcomeAccept)
	}

	var factId string
	switch result.Verdict {
	case types.Verdict_VERDICT_ACCEPT:
		// Create fact from accepted claim
		var err error
		factId, err = k.createFactFromClaim(ctx, claim, round, result.Confidence)
		if err != nil {
			return err
		}
		// Review fee already distributed at submission time — no refund.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_ACCEPTED
		// Base-rate tracking: record binary outcome for this domain's prior (Gap 3).
		k.RecordDomainVerdict(ctx, claim.Domain, true)

	case types.Verdict_VERDICT_REJECT:
		// Review fee already distributed at submission time — the fee IS the cost of rejection.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_REJECTED
		// Base-rate tracking: record binary outcome for this domain's prior (Gap 3).
		k.RecordDomainVerdict(ctx, claim.Domain, false)
		// Null-result credit: reward submitters who pre-declared falsifiability conditions.
		// Operationalizes Peirce's self-correcting community (Scientific Method Gap 5).
		k.maybeAwardNullResultCredit(ctx, claim)
		// If this was a challenge claim, the original fact survived — energy boost
		// + Falsificationism Gap 1: pass rejection confidence so severity score accumulates.
		k.handleChallengeSurvival(ctx, claim, result.Confidence)
		// Challenge lost: record on the challenger's side of the ledger (Phase 5).
		if claim.ProvisionalFactId != "" {
			k.RecordChallengeOutcome(ctx, claim.Submitter, false)
		}
		// Contradicting claim was rejected — undo its CONTESTED side-effect on target facts (T-i4).
		k.reverseContradictionsFromClaim(ctx, claim)
		// Hegelian Dialectic Gap 2: Aufhebung (sublation).
		// Record the link between this rejected claim and the accepted Fact that
		// "sublates" it — absorbing its content while transcending its inadequacy.
		// Hegel: "The negation is a determinate negation; it has a content" — and
		// that content is not lost; it is preserved in the superior determination.
		k.recordSublation(ctx, claim)

	case types.Verdict_VERDICT_MALFORMED:
		// Review fee already distributed at submission time — no additional slashing needed.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_MALFORMED
		k.reverseContradictionsFromClaim(ctx, claim)

	case types.Verdict_VERDICT_INCONCLUSIVE:
		// Review fee is non-refundable — verifiers still did work even if inconclusive.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT
		// Pragmatism Gap 2: INCONCLUSIVE as epistemic difficulty signal.
		// Track how many times this claim has resisted a verdict. Claims with
		// high inconclusive_count name exactly what the community of inquiry
		// cannot resolve — evidence sparse, claim framing unobservable, or
		// diversity gates preventing acceptance. INCONCLUSIVE is data, not failure.
		claim.InconclusiveCount++
		if claim.FirstInconclusiveAtBlock == 0 {
			claim.FirstInconclusiveAtBlock = uint64(sdkCtx.BlockHeight())
		}
		// Socratic Dialectic Gap 2: Aporia — productive perplexity.
		// When the claim has now resisted resolution N times (≥ aporia_threshold),
		// mark it as a named site of epistemic difficulty so future inquiry can
		// attend to it with fresh instruments rather than repeating the impasse.
		k.maybeMarkAporia(ctx, claim)
		k.reverseContradictionsFromClaim(ctx, claim)
	}

	// Structured Deliberation Gap 3: Dung labelling — grounded semantics.
	// Assign the one-step label that seeds the argumentation graph: ACCEPT → IN,
	// REJECT/MALFORMED → OUT, INCONCLUSIVE → UNDECIDED. The label is immutable;
	// full extension semantics (preferred, stable) are computed offline by walking
	// the attack graph (ClaimRelation.relation == ATTACKS + FactRelation.attack_target).
	claim.DungLabel = dungLabelForVerdict(result.Verdict)

	if err := k.SetClaim(ctx, claim); err != nil {
		return err
	}
	if err := k.SetVerificationRound(ctx, round); err != nil {
		return err
	}

	// Distribute verifier rewards from the verifier pool (including priority tip if set).
	k.distributeVerifierRewardsFromPool(ctx, claim, round, result)

	// Slash loop: route vindication-eligible slashes to escrow only when a fact was
	// created (ACCEPT verdict) — non-ACCEPT verdicts have no fact that can later be
	// disproven, so vindication is structurally unreachable and any escrowed tokens
	// would be orphaned (T-i3).
	params, _ := k.GetParams(ctx)
	var vindicationEntries []types.VindicationEntry
	canVindicate := params.VindicationRefundEnabled && result.Verdict == types.Verdict_VERDICT_ACCEPT && factId != ""

	for _, slash := range result.Slashes {
		if k.stakingKeeper == nil {
			continue
		}
		if slash.VindicationEligible && canVindicate {
			slashedAmt, err := k.stakingKeeper.SlashValidatorToModule(ctx, slash.Verifier, slash.SlashBps, types.VindicationEscrowModuleName)
			if err == nil && slashedAmt.IsPositive() {
				vote := ""
				for _, reveal := range round.Reveals {
					if reveal.Verifier == slash.Verifier {
						vote = reveal.Vote
						break
					}
				}
				vindicationEntries = append(vindicationEntries, types.VindicationEntry{
					Verifier:    slash.Verifier,
					Vote:        vote,
					SlashAmount: slashedAmt.String(),
					SlashBps:    slash.SlashBps,
					RoundId:     round.Id,
					Height:      height,
				})
			}
		} else {
			_ = k.stakingKeeper.SlashValidator(ctx, slash.Verifier, slash.SlashBps)
		}
	}

	// Store vindication entries, always associated with the created fact.
	if len(vindicationEntries) > 0 {
		for i := range vindicationEntries {
			vindicationEntries[i].FactId = factId
		}
		_ = k.SetVindicationPending(ctx, factId, vindicationEntries)
	}

	// If this was a challenge claim that was ACCEPTED, the original fact is disproven.
	// This triggers vindication for the original fact's minority voters.
	if result.Verdict == types.Verdict_VERDICT_ACCEPT && claim.ProvisionalFactId != "" {
		k.handleChallengeDisproven(ctx, claim, factId)
	}

	// Settle the challenger's staked escrow. Legitimate falsification is the
	// engine of truth-discovery on this chain; if successful challenges earn
	// nothing and failed challenges confiscate the full stake, no rational
	// actor challenges bad facts. Wire the SuccessfulChallengeRewardBps /
	// FailedChallengeSlashBps params that were defined in Wave ~2 but never
	// connected. See also the Wave 14b moat-integrity audit.
	if claim.ProvisionalFactId != "" {
		k.settleChallengeStake(ctx, claim, result.Verdict, params)
	}

	// Record verification outcomes for domain qualification tracking (R26-3).
	// Rewarded verifiers voted correctly; slashed verifiers voted incorrectly.
	if k.domainQualificationKeeper != nil && claim.Domain != "" &&
		result.Verdict != types.Verdict_VERDICT_INCONCLUSIVE {
		for _, reward := range result.Rewards {
			if err := k.domainQualificationKeeper.RecordVerificationOutcome(ctx, reward.Verifier, claim.Domain, true); err != nil {
				k.Logger(ctx).Debug("failed to record correct verification outcome", "verifier", reward.Verifier, "error", err)
			}
		}
		for _, slash := range result.Slashes {
			if err := k.domainQualificationKeeper.RecordVerificationOutcome(ctx, slash.Verifier, claim.Domain, false); err != nil {
				k.Logger(ctx).Debug("failed to record incorrect verification outcome", "verifier", slash.Verifier, "error", err)
			}
		}
	}

	// Feed verification history to capture defense (R28-8).
	if k.captureDefenseKeeper != nil {
		verdictVote := verdictToVoteString(result.Verdict)
		roundValidators := make([]string, 0, len(round.Reveals))
		roundVerdicts := make([]bool, 0, len(round.Reveals))
		for _, reveal := range round.Reveals {
			roundValidators = append(roundValidators, reveal.Verifier)
			roundVerdicts = append(roundVerdicts, reveal.Vote == verdictVote)
		}
		k.captureDefenseKeeper.RecordVerificationHistory(ctx, claim.Domain, round.Id, roundValidators, roundVerdicts, nil)

		// Update reputations — get stratum for domain context.
		stratum := ""
		if k.ontologyKeeper != nil {
			stratum, _ = k.ontologyKeeper.GetStratumForDomain(ctx, claim.Domain)
		}
		for _, reveal := range round.Reveals {
			wasCorrect := reveal.Vote == verdictVote
			k.captureDefenseKeeper.UpdateReputation(ctx, reveal.Verifier, claim.Domain, stratum, wasCorrect)
		}
	}

	// Record round diversity metrics (R28-2)
	if err := k.RecordRoundDiversity(ctx, round.Id, claim.Domain, result.AcceptCount, result.RejectCount); err != nil {
		k.Logger(ctx).Error("failed to record round diversity", "round_id", round.Id, "error", err)
	}

	// Update validator independence for each revealed voter
	majorityVote := verdictToVoteString(result.Verdict)
	if majorityVote != "" {
		for _, reveal := range round.Reveals {
			if err := k.UpdateValidatorIndependence(ctx, reveal.Verifier, reveal.Vote, majorityVote); err != nil {
				k.Logger(ctx).Debug("failed to update validator independence", "verifier", reveal.Verifier, "error", err)
			}
		}
	}

	// Index completed round for window-based metrics (R31-2)
	hasDissent := roundHasDissent(round)
	duration := height - round.StartedAtBlock
	completionMeta := &types.CompletedRoundMeta{
		Domain:         claim.Domain,
		HasDissent:     hasDissent,
		DurationBlocks: duration,
	}
	if idxErr := k.IndexCompletedRound(ctx, height, round.Id, completionMeta); idxErr != nil {
		k.Logger(ctx).Debug("failed to index completed round", "round", round.Id, "error", idxErr)
	}

	// Track active rounds for congestion-based fee backpressure.
	k.DecrementActiveRoundsCount(ctx)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.verification_round_completed",
		sdk.NewAttribute("round_id", round.Id),
		sdk.NewAttribute("claim_id", round.ClaimId),
		sdk.NewAttribute("verdict", round.Verdict.String()),
		// Settlement Finality Gap 4: include finality receipt for IBC light client proofs.
		sdk.NewAttribute("finality_receipt", hex.EncodeToString(round.FinalityReceiptHash)),
	))

	return nil
}

// computeFinalityReceiptHash computes the sha256 finality receipt for a completed round.
// Format: sha256(roundId || "|" || verdictStr || "|" || verdictBlock_BE_8 || "|" || verdictBlockHash)
//
// The "|" separators prevent ambiguous concatenation. verdictBlock is encoded as 8-byte
// big-endian to make the hash independent of decimal string representation.
// This is the canonical Settlement Finality Gap 4 receipt format.
func computeFinalityReceiptHash(roundId, verdictStr string, verdictBlock uint64, verdictBlockHash []byte) []byte {
	h := sha256.New()
	h.Write([]byte(roundId))
	h.Write([]byte("|"))
	h.Write([]byte(verdictStr))
	h.Write([]byte("|"))
	var blockBE [8]byte
	binary.BigEndian.PutUint64(blockBE[:], verdictBlock)
	h.Write(blockBE[:])
	h.Write([]byte("|"))
	h.Write(verdictBlockHash)
	return h.Sum(nil)
}

// distributeVerifierRewardsFromPool distributes the verifier pool among correct verifiers.
//
// Split mechanics (Gap 2 — Priority Tip Pool):
//
//	If round.PriorityTipBps > 0:
//	  tipPool   = poolAmount × PriorityTipBps / 1_000_000
//	  basePool  = poolAmount − tipPool
//	  tipPool is shared equally among correct verifiers who committed before the
//	  commit-phase midpoint (StartedAtBlock + CommitDeadline) / 2.
//	  If no early correct verifiers exist, tipPool merges back into basePool.
//	  basePool is shared equally among all correct verifiers.
//	If PriorityTipBps == 0 (default): poolAmount is treated as basePool entirely —
//	identical to pre-Gap-2 behavior.
//
// Each verifier's base-pool share is further modulated by their historical
// independence score (T3): crowd-followers receive less, with the withheld
// amount flowing to the development fund.
func (k Keeper) distributeVerifierRewardsFromPool(ctx context.Context, claim *types.Claim, round *types.VerificationRound, result *VerificationResult) {
	if k.bankKeeper == nil || len(result.Rewards) == 0 {
		return
	}

	params, _ := k.GetParams(ctx)

	// Calculate pool: verifierPoolBps% of the original review fee.
	feeAmt, ok := new(big.Int).SetString(claim.Stake, 10)
	if !ok || feeAmt.Sign() <= 0 {
		return
	}
	poolAmount := verifierPoolFromFee(feeAmt.Uint64(), params)
	if poolAmount == 0 {
		return
	}

	// ── Priority tip split (Gap 2) ──────────────────────────────────────────
	var tipPool, basePool uint64
	if round != nil && round.PriorityTipBps > 0 {
		tipPool = safeMulDiv(poolAmount, round.PriorityTipBps, 1_000_000)
		basePool = poolAmount - tipPool
	} else {
		basePool = poolAmount
	}

	// Build early-committer set for tip eligibility.
	// "Early" = committed at or before the commit-phase midpoint.
	// midpoint = (round.StartedAtBlock + round.CommitDeadline) / 2
	earlySet := map[string]struct{}{}
	if tipPool > 0 && round != nil {
		midpoint := (round.StartedAtBlock + round.CommitDeadline) / 2
		for _, c := range round.Commits {
			if c.CommittedAtBlock <= midpoint {
				earlySet[c.Verifier] = struct{}{}
			}
		}
	}

	// Identify early correct verifiers (intersection of rewarded + early).
	var earlyRewards []VerifierReward
	if tipPool > 0 {
		for _, r := range result.Rewards {
			if _, ok := earlySet[r.Verifier]; ok {
				earlyRewards = append(earlyRewards, r)
			}
		}
		// If no early correct verifiers, tip merges into base pool.
		if len(earlyRewards) == 0 {
			basePool += tipPool
			tipPool = 0
		}
	}

	// Distribute tip pool equally among early correct verifiers.
	if tipPool > 0 && len(earlyRewards) > 0 {
		perEarly := tipPool / uint64(len(earlyRewards))
		tipRemainder := tipPool - perEarly*uint64(len(earlyRewards))
		for i, reward := range earlyRewards {
			amount := perEarly
			if i == 0 {
				amount += tipRemainder // first early verifier absorbs dust
			}
			k.distributeVerifierReward(ctx, reward.Verifier, amount)
		}
	}

	// Distribute base pool: Brier-score-proportional (proper scoring rule) when
	// params.ProperScoringRuleEnabled is true; equal-share otherwise (existing logic).
	//
	// Proper scoring rule (Calibrated Forecasting Gap 1):
	//   Each correct verifier's share ∝ brierCorrectnessBps(reveal, outcome).
	//   brierCorrectnessBps = 1_000_000 − (p_accept − outcome)² / 1_000_000.
	//   ConfidenceBps == 0 (not reported) → neutral position (500_000).
	var withheldTotal uint64
	properScoringApplied := false

	if params != nil && params.ProperScoringRuleEnabled && round != nil &&
		(result.Verdict == types.Verdict_VERDICT_ACCEPT || result.Verdict == types.Verdict_VERDICT_REJECT) {

		// Build O(1) reveal lookup.
		revealMap := make(map[string]*types.RevealEntry, len(round.Reveals))
		for _, rev := range round.Reveals {
			revealMap[rev.Verifier] = rev
		}
		outcomeAcceptBps := uint64(0)
		if result.Verdict == types.Verdict_VERDICT_ACCEPT {
			outcomeAcceptBps = 1_000_000
		}

		// Compute Brier correctness score per correct verifier.
		scores := make([]uint64, len(result.Rewards))
		var totalScore uint64
		for i, reward := range result.Rewards {
			scores[i] = brierCorrectnessBps(revealMap[reward.Verifier], outcomeAcceptBps)
			totalScore += scores[i]
		}

		if totalScore > 0 {
			properScoringApplied = true
			var distributed uint64
			for i, reward := range result.Rewards {
				amount := safeMulDiv(basePool, scores[i], totalScore)
				modulated := k.applyIndependenceMultiplier(ctx, reward.Verifier, amount, params)
				if modulated < amount {
					withheldTotal += amount - modulated
				}
				k.distributeVerifierReward(ctx, reward.Verifier, modulated)
				distributed += amount
			}
			// Rounding remainder → first verifier.
			if rem := basePool - distributed; rem > 0 {
				k.distributeVerifierReward(ctx, result.Rewards[0].Verifier, rem)
			}
		}
		// If totalScore == 0 (shouldn't happen: correct verifiers always score > 0
		// unless all said exactly 100% against their correct vote, which is impossible
		// by definition), fall through to equal-share.
	}

	if !properScoringApplied {
		// Equal-share distribution among all correct verifiers.
		perVerifier := basePool / uint64(len(result.Rewards))
		remainder := basePool - (perVerifier * uint64(len(result.Rewards)))
		for i, reward := range result.Rewards {
			amount := perVerifier
			if i == 0 {
				amount += remainder // first verifier absorbs dust
			}
			modulated := k.applyIndependenceMultiplier(ctx, reward.Verifier, amount, params)
			if modulated < amount {
				withheldTotal += amount - modulated
			}
			k.distributeVerifierReward(ctx, reward.Verifier, modulated)
		}
	}

	// Withheld portion (independence modulation) flows to development fund.
	if withheldTotal > 0 {
		k.forwardWithheldToDevelopmentFund(ctx, withheldTotal)
	}
}

// BrierCorrectnessBpsForTest is an exported thin wrapper for use in unit tests
// (package keeper_test cannot access unexported brierCorrectnessBps directly).
func BrierCorrectnessBpsForTest(reveal *types.RevealEntry, outcomeAcceptBps uint64) uint64 {
	return brierCorrectnessBps(reveal, outcomeAcceptBps)
}

// brierCorrectnessBps computes the Brier correctness score for a single verifier
// reveal against a known outcome, scaled to BPS (0..1_000_000).
//
//   brierCorrectness = 1_000_000 − brierError
//   brierError       = (p_accept_bps − outcome_accept_bps)² / 1_000_000
//
// outcomeAcceptBps is 1_000_000 for ACCEPT, 0 for REJECT.
// ConfidenceBps == 0 (pre-Wave-4 or not reported) → neutral 50% (500_000).
//
// A perfectly confident correct verifier (ConfidenceBps=1_000_000, correct direction)
// scores 1_000_000.  A neutral verifier (ConfidenceBps=0) scores 750_000.
// A perfectly confident wrong verifier would score 0, but correct verifiers
// always have brierCorrectness > 0 by definition.
func brierCorrectnessBps(reveal *types.RevealEntry, outcomeAcceptBps uint64) uint64 {
	var pAcceptBps uint64
	if reveal == nil || reveal.ConfidenceBps == 0 {
		pAcceptBps = 500_000 // neutral: no confidence reported
	} else if reveal.Vote == "accept" {
		pAcceptBps = reveal.ConfidenceBps
	} else {
		// REJECT vote: complement maps ConfidenceBps to P(accept).
		if reveal.ConfidenceBps > 1_000_000 {
			pAcceptBps = 0
		} else {
			pAcceptBps = 1_000_000 - reveal.ConfidenceBps
		}
	}
	diff := int64(pAcceptBps) - int64(outcomeAcceptBps)
	brierError := uint64(diff*diff) / 1_000_000
	if brierError > 1_000_000 {
		brierError = 1_000_000
	}
	return 1_000_000 - brierError
}

// applyIndependenceMultiplier returns the reward amount after modulation.
// Formula: amount × (1 - conformity_bps × strength_bps / BPS²).
// New voters (no history) get full reward. Strength=0 disables the mechanism.
func (k Keeper) applyIndependenceMultiplier(ctx context.Context, verifier string, amount uint64, params *types.Params) uint64 {
	if amount == 0 || params == nil || params.IndependenceRewardStrengthBps == 0 {
		return amount
	}
	rec, found, err := k.GetValidatorIndependence(ctx, verifier)
	if err != nil || !found || rec.TotalVotes == 0 {
		return amount
	}
	const bps uint64 = 1_000_000
	// conformity_bps = (total - minority) × BPS / total
	majority := rec.TotalVotes - rec.MinorityVotes
	conformityBps := safeMulDiv(majority, bps, rec.TotalVotes)
	// penalty_bps = conformity_bps × strength_bps / BPS
	penaltyBps := safeMulDiv(conformityBps, params.IndependenceRewardStrengthBps, bps)
	if penaltyBps >= bps {
		penaltyBps = bps - 1
	}
	// amount × (BPS - penalty) / BPS
	return safeMulDiv(amount, bps-penaltyBps, bps)
}

// forwardWithheldToDevelopmentFund sends withheld reward tokens to the development fund.
func (k Keeper) forwardWithheldToDevelopmentFund(ctx context.Context, amount uint64) {
	if k.bankKeeper == nil || amount == 0 {
		return
	}
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(new(big.Int).SetUint64(amount))))
	_ = k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, developmentFundModule, coins)
}

// createFactFromClaim creates a new Fact from an accepted claim.
// Returns the generated factID and any error.
func (k Keeper) createFactFromClaim(ctx context.Context, claim *types.Claim, round *types.VerificationRound, confidence uint64) (string, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	factID := GenerateFactID(claim.Id, height)

	// Calculate fitness epoch from current height
	params, _ := k.GetParams(ctx)
	epochBorn := uint64(0)
	if params.FitnessEpochBlocks > 0 {
		epochBorn = height / params.FitnessEpochBlocks
	}

	// Freeze the submitter's calibration at acceptance time. This is the
	// TVW snapshot that Popper-weights the fact's future revenue. Reading
	// it fresh at query time would let a submitter inflate calibration
	// after the fact lands and retroactively harvest more training value
	// — the gate has to bite at creation, not at payout.
	var calibrationSnapshot uint64
	if cal, ok := k.GetAgentCalibration(ctx, claim.Submitter); ok && cal != nil {
		calibrationSnapshot = cal.CalibrationScoreBps
	}
	if calibrationSnapshot == 0 {
		calibrationSnapshot = 500_000 // neutral — no reward, no penalty
	}

	fact := &types.Fact{
		Id:                factID,
		Content:           claim.FactContent,
		Domain:            claim.Domain,
		Category:          claim.Category,
		Confidence:        confidence,
		Submitter:         claim.Submitter,
		SubmittedAtBlock:  claim.SubmittedAtBlock,
		VerifiedAtBlock:   height,
		LastVerifiedBlock: height,
		References:        claim.References,
		Status:            types.FactStatus_FACT_STATUS_VERIFIED,
		ClaimId:           claim.Id,
		ClaimType:         claim.ClaimType,
		Structure:         claim.Structure,
		CanonicalForm:     claim.CanonicalForm,
		CanonicalHash:     claim.CanonicalHash,
		// Methodology (Phase 1): copy from claim; default to M-LEGACY if unset.
		MethodId: ResolveMethodId(claim.MethodId),
		// Reasoning trace (Phase 9): structured derivation flows through to
		// the accepted Fact as first-class training data.
		ReasoningTrace: claim.ReasoningTrace,
		// Theory-ladenness (Scientific Method Gap 4): theoretical paradigm
		// flows through to the Fact for inter-paradigm corpus analysis.
		TheoreticalFramework: claim.TheoreticalFramework,
		// Phenomenological method (Phenomenology Brief Gap 2): method name
		// flows through so Fact corpus can be queried by specific method
		// (e.g. "which neurophenomenological facts have cross-modal support?").
		PhenomenologicalMethod: claim.PhenomenologicalMethod,
		// Structured Deliberation Gap 1: Toulmin argument structure flows
		// through to the accepted Fact for argument-graph queries and
		// Dung-style attack targeting at the warrant layer.
		Warrant:      claim.Warrant,
		Backing:      claim.Backing,
		QualifierBps: claim.QualifierBps,
		// Falsificationism Gap 1: initial corroboration severity = acceptance
		// panel confidence. A 95%-confident ACCEPT is a severer test than a
		// 55%-confident one; the severity score captures this asymmetry from
		// the moment of creation.
		CorroborationSeverityScoreBps: confidence,
		// Fitness fields
		FitnessScore:       params.FitnessInitialScore,
		FitnessUpdatedBlock: height,
		EpochBorn:          epochBorn,
		// Metabolism fields
		Energy:           params.MetabolismInitialEnergy,
		EnergyCap:        params.MetabolismEnergyCap,
		EnergyLastUpdated: height,
		// Popper-weighted TVW: calibration snapshot frozen here. Any future
		// calibration drift has no retroactive effect on this fact's TVW.
		SubmitterCalibrationSnapshotBps: calibrationSnapshot,
		// Bayesian Gap 1: stated prior flows through from claim. Enables
		// post-hoc update magnitude analysis: |ConfidenceBps − StatedPriorBps|.
		// Zero when no prior was declared at submission.
		StatedPriorBps: claim.PriorProbabilityBps,
	}

	// Apply domain carrying capacity birth pressure (R29-1)
	fact.Energy = k.ApplyBirthPressure(ctx, claim.Domain, fact.Energy)

	// Apply role bonus — claim type × account type (R28-5)
	accountType := k.getAccountType(ctx, claim.Submitter)
	fact.Confidence = ApplyRoleBonusToConfidence(fact.Confidence, claim.ClaimType, accountType, params)

	// Apply dual validation bonus for partnership claims (R28-5)
	// Scale by weaker role's accuracy in the domain (R29-3)
	if claim.PartnershipId != "" {
		agentAcc, humanAcc := k.GetRoleAccuracies(ctx, claim.Domain)
		weakerAccuracy := agentAcc
		if humanAcc < weakerAccuracy || weakerAccuracy == 0 {
			weakerAccuracy = humanAcc
		}
		if weakerAccuracy > 0 {
			scaledBonusBps := safeMulDiv(params.DualValidationBonusBps, weakerAccuracy, BPS)
			fact.Confidence = safeMulDiv(fact.Confidence, 1_000_000+scaledBonusBps, 1_000_000)
		} else {
			// No track record yet — use full static bonus
			fact.Confidence = ApplyDualValidationBonus(fact.Confidence, params)
		}
	}

	// Apply confidence ceiling (stratum + global MaxConfidence hard cap)
	fact.Confidence = k.ClampConfidence(ctx, fact.Confidence, claim.Domain)
	if k.ontologyKeeper != nil && claim.Domain != "" {
		stratum, err := k.ontologyKeeper.GetStratumForDomain(ctx, claim.Domain)
		if err == nil && stratum != "" {
			fact.Stratum = stratum
		}
	}

	// ─── Epistemic provenance (ToK Wave 2) ─────────────────────────────
	// axiom_distance = min(cited_facts.axiom_distance) + 1, or 0 if no cites
	// (the fact is foundational). dependency_confidence_floor inherits the
	// weakest cited support's effective confidence so proof chains can't
	// claim more confidence than their foundations.
	dist, floor := k.computeProvenance(ctx, claim)
	fact.AxiomDistance = dist
	fact.DependencyConfidenceFloor = floor
	// Clamp the fact's own confidence to the inherited floor if it exists.
	if floor > 0 && fact.Confidence > floor {
		fact.Confidence = floor
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.confidence_clamped_to_floor",
			sdk.NewAttribute("fact_id", factID),
			sdk.NewAttribute("dependency_floor_bps", fmt.Sprintf("%d", floor)),
			sdk.NewAttribute("axiom_distance", fmt.Sprintf("%d", dist)),
		))
	}

	if err := k.SetFact(ctx, fact); err != nil {
		return "", err
	}

	// Update domain stats for carrying capacity (R29-1)
	k.IncrementDomainFactCount(ctx, fact.Domain, true, fact.Energy)

	// Index fact by structured subject and tags
	if fact.Structure != nil {
		if err := k.IndexFactBySubject(ctx, fact); err != nil {
			return "", fmt.Errorf("failed to index fact by subject: %w", err)
		}
	}

	// ─── Niche registration (competitive dynamics) ─────────────────────
	nicheKey := k.ComputeNicheKey(fact)
	fact.NicheKey = nicheKey
	fact.NicheLeader = true // new fact starts as leader-candidate
	fact.NicheRank = 1
	fact.NicheSize = 1
	currentLeader, hasLeader := k.GetNicheLeader(ctx, nicheKey)
	if hasLeader {
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.niche_challenger",
			sdk.NewAttribute("new_fact", fact.Id),
			sdk.NewAttribute("current_leader", currentLeader.Id),
			sdk.NewAttribute("niche", nicheKey),
			sdk.NewAttribute("domain", fact.Domain),
		))
		fact.NicheLeader = false // not leader yet — competition will decide
		fact.NicheSize = uint64(len(k.GetNicheMembers(ctx, nicheKey))) + 1
	}
	if err := k.UpdateNicheIndex(ctx, fact); err != nil {
		return "", fmt.Errorf("failed to update niche index: %w", err)
	}
	// Re-save fact with niche fields set
	if err := k.SetFact(ctx, fact); err != nil {
		return "", fmt.Errorf("failed to save fact with niche fields: %w", err)
	}

	// Index fact by canonical hash
	if fact.CanonicalHash != "" {
		if err := k.SetCanonicalHash(ctx, fact.CanonicalHash, fact.Id); err != nil {
			return "", fmt.Errorf("failed to index fact by canonical hash: %w", err)
		}
	}

	// Convert claim relations to fact relations and store in graph index.
	// Inference type/strength (ToK Wave 1) propagate from ClaimRelation to
	// FactRelation so proof-tree auditors can see HOW each edge was derived.
	for _, claimRel := range claim.Relations {
		factRel := &types.FactRelation{
			SourceFactId:             factID,
			TargetFactId:             claimRel.TargetFactId,
			Relation:                 claimRel.Relation,
			CreatedAtBlock:           height,
			Creator:                  claim.Submitter,
			Inference:                claimRel.Inference,
			InferenceStrengthBps:     claimRel.InferenceStrengthBps,
		}
		if err := k.SetFactRelation(ctx, factRel); err != nil {
			return "", fmt.Errorf("failed to store fact relation: %w", err)
		}

		// Track new citation for metabolism energy income
		k.IncrementNewCitationEpoch(ctx, claimRel.TargetFactId)
		// Track downstream utility — Peirce: facts that ground accepted claims are
		// epistemically stronger than facts that nothing builds on.
		k.IncrementIncomingCitationCount(ctx, claimRel.TargetFactId)

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.fact_relation_created",
			sdk.NewAttribute("source", factRel.SourceFactId),
			sdk.NewAttribute("target", factRel.TargetFactId),
			sdk.NewAttribute("relation", factRel.Relation.String()),
		))
	}

	// ─── Proof certificate premise citations ─────────────────────────────────
	// When a formal proof certificate lists premise_fact_ids, each premise fact
	// contributed to this derivation. Record the incoming citation so the fitness
	// system can recognise foundational facts that ground many formal proofs.
	if claim.ProofCertificate != nil {
		for _, premiseID := range claim.ProofCertificate.PremiseFactIds {
			k.IncrementIncomingCitationCount(ctx, premiseID)
		}
	}

	// ─── Lineage registration (reproduction) ──────────────────────────────
	// Check for reproductive relations (REFINES or GENERALIZES imply parent-child)
	for _, rel := range claim.Relations {
		if rel.Relation == types.RelationType_RELATION_TYPE_REFINES ||
			rel.Relation == types.RelationType_RELATION_TYPE_GENERALIZES {
			parentFact, found := k.GetFact(ctx, rel.TargetFactId)
			if !found {
				continue
			}

			// Check max children
			if uint64(len(parentFact.ChildFactIds)) >= params.ReproductionMaxChildren {
				k.Logger(ctx).Info("parent at max children", "parent", parentFact.Id)
				continue
			}

			// Set lineage on child
			fact.ParentFactId = parentFact.Id
			fact.LineageDepth = parentFact.LineageDepth + 1
			fact.LineageRootId = parentFact.LineageRootId
			if fact.LineageRootId == "" {
				fact.LineageRootId = parentFact.Id // parent is the root
			}

			// Inherit fitness from parent
			inheritedFitness := safeMulDiv(parentFact.FitnessScore, params.ReproductionChildFitnessInheritanceBps, 1_000_000)
			fact.FitnessScore = inheritedFitness

			// Update parent: add child, bump progeny, energy bonus
			parentFact.ChildFactIds = append(parentFact.ChildFactIds, fact.Id)
			parentFact.ProgenyCount++
			parentFact.Energy += params.ReproductionParentEnergyBonus
			if parentFact.Energy > params.MetabolismEnergyCap {
				parentFact.Energy = params.MetabolismEnergyCap
			}
			_ = k.SetFact(ctx, parentFact)

			// Propagate progeny count up the lineage
			k.PropagateProgenyCount(ctx, parentFact.ParentFactId)

			// Save updated child fact with lineage fields
			_ = k.SetFact(ctx, fact)

			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.lineage_created",
				sdk.NewAttribute("child_fact_id", fact.Id),
				sdk.NewAttribute("parent_fact_id", parentFact.Id),
				sdk.NewAttribute("lineage_depth", fmt.Sprintf("%d", fact.LineageDepth)),
				sdk.NewAttribute("inherited_fitness", fmt.Sprintf("%d", inheritedFitness)),
			))

			break // Only one parent relationship
		}
	}

	// Route submitter reward: through partnership split or direct vesting (R26-4)
	if claim.PartnershipId != "" && k.partnershipKeeper != nil {
		stakeAmt, ok := new(big.Int).SetString(claim.Stake, 10)
		if ok && stakeAmt.Sign() > 0 {
			rewardCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
			err := k.partnershipKeeper.DistributeReward(ctx, claim.PartnershipId, rewardCoins, "knowledge_verification")
			if err != nil {
				// Fallback to direct vesting on partnership error
				k.Logger(ctx).Error("partnership reward routing failed, falling back to vesting",
					"partnership_id", claim.PartnershipId, "err", err)
				if k.vestingRewardsKeeper != nil {
					_ = k.vestingRewardsKeeper.CreateVestingScheduleFromKnowledge(
						ctx, claim.Id, factID, claim.Submitter, claim.Stake, claim.Category,
					)
				}
			}
		}
	} else if k.vestingRewardsKeeper != nil {
		// Direct vesting (no partnership — existing behavior)
		_ = k.vestingRewardsKeeper.CreateVestingScheduleFromKnowledge(
			ctx, claim.Id, factID, claim.Submitter, claim.Stake, claim.Category,
		)
	}

	// Check if this fact fills an active knowledge bounty
	k.ClaimBountyForFact(ctx, fact, claim)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.fact_created",
		sdk.NewAttribute("fact_id", factID),
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("domain", claim.Domain),
		sdk.NewAttribute("confidence", fmt.Sprintf("%d", fact.Confidence)),
		sdk.NewAttribute("stated_prior_bps", fmt.Sprintf("%d", fact.StatedPriorBps)),
	))

	return factID, nil
}

// reverseContradictionsFromClaim restores any target facts this claim flipped to
// CONTESTED via its CONTRADICTS relations, when the claim itself is not accepted.
// Only restores when no other live claim (PENDING / IN_VERIFICATION) still
// contradicts the same target fact (T-i4).
func (k Keeper) reverseContradictionsFromClaim(ctx context.Context, claim *types.Claim) {
	for _, rel := range claim.Relations {
		if rel.Relation != types.RelationType_RELATION_TYPE_CONTRADICTS {
			continue
		}
		targetFact, found := k.GetFact(ctx, rel.TargetFactId)
		if !found || targetFact.Status != types.FactStatus_FACT_STATUS_CONTESTED {
			continue
		}
		if k.hasOtherLiveContradiction(ctx, claim.Id, rel.TargetFactId) {
			continue
		}
		targetFact.Status = types.FactStatus_FACT_STATUS_VERIFIED
		_ = k.SetFact(ctx, targetFact)
		sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.contradiction_reversed",
			sdk.NewAttribute("fact_id", rel.TargetFactId),
			sdk.NewAttribute("reverted_by_claim", claim.Id),
		))
	}
}

// hasOtherLiveContradiction reports whether any claim other than excludeClaimId
// with PENDING or IN_VERIFICATION status has a CONTRADICTS relation to targetFactId.
func (k Keeper) hasOtherLiveContradiction(ctx context.Context, excludeClaimId, targetFactId string) bool {
	found := false
	k.IterateClaims(ctx, func(c *types.Claim) bool {
		if c.Id == excludeClaimId {
			return false
		}
		if c.Status != types.ClaimStatus_CLAIM_STATUS_PENDING &&
			c.Status != types.ClaimStatus_CLAIM_STATUS_PENDING_EVALUATION &&
			c.Status != types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION {
			return false
		}
		for _, rel := range c.Relations {
			if rel.Relation == types.RelationType_RELATION_TYPE_CONTRADICTS && rel.TargetFactId == targetFactId {
				found = true
				return true
			}
		}
		return false
	})
	return found
}

// computeProvenance walks the claim's citations (References + Relations) and
// returns the axiom distance and dependency confidence floor for the new
// fact (ToK Waves 2 + 6).
//
//   - axiom_distance: min of cited facts' axiom_distance, plus 1. Facts with no
//     cites become foundational (distance 0). Missing cites are ignored.
//
//   - dependency_confidence_floor: inference-weighted minimum. Each edge carries
//     an inference strength (BPS). For each cited fact the edge contribution is
//     target.effective_confidence × strength / BPS. A deductive edge at full
//     strength preserves confidence; an inductive edge at 70% strength weakens
//     the contribution by 30%. The floor is the minimum contribution across
//     all edges. References-only cites have no declared strength and default
//     to full BPS (pure citation preserves the cited fact's confidence).
//     Facts with no cites have floor 0 (= no cap).
func (k Keeper) computeProvenance(ctx context.Context, claim *types.Claim) (uint32, uint64) {
	const bps uint64 = 1_000_000

	// Per-edge contributions: (factID → strengthBps). References default to
	// full strength; Relations use declared strength (UNSPECIFIED or 0 → BPS).
	edges := make(map[string]uint64)
	for _, ref := range claim.References {
		if ref == "" {
			continue
		}
		// Plain reference = full-strength preservation.
		if prev, ok := edges[ref]; !ok || bps > prev {
			edges[ref] = bps
		}
	}
	for _, rel := range claim.Relations {
		if rel.Relation == types.RelationType_RELATION_TYPE_CONTRADICTS {
			continue
		}
		if rel.TargetFactId == "" {
			continue
		}
		strength := rel.InferenceStrengthBps
		if strength == 0 || strength > bps {
			strength = bps
		}
		// If the same fact is cited via both References and Relations, keep
		// the stronger edge (less weakening) — the submitter declared a
		// stronger claim somewhere.
		if prev, ok := edges[rel.TargetFactId]; !ok || strength > prev {
			edges[rel.TargetFactId] = strength
		}
	}

	if len(edges) == 0 {
		return 0, 0
	}

	var minDist uint32 = ^uint32(0)
	var minFloor uint64
	var floorInitialized bool

	for factID, strength := range edges {
		target, found := k.GetFact(ctx, factID)
		if !found {
			continue
		}
		if target.AxiomDistance < minDist {
			minDist = target.AxiomDistance
		}
		// target.effective_confidence = min(own, inherited floor)
		eff := target.Confidence
		if target.DependencyConfidenceFloor > 0 && target.DependencyConfidenceFloor < eff {
			eff = target.DependencyConfidenceFloor
		}
		// Weaken by inference strength (ToK Wave 6).
		contribution := safeMulDiv(eff, strength, bps)
		if !floorInitialized || contribution < minFloor {
			minFloor = contribution
			floorInitialized = true
		}
	}

	if minDist == ^uint32(0) {
		return 0, 0
	}
	return minDist + 1, minFloor
}

// handleChallengeSurvival restores a challenged fact and grants survival energy
// when a challenge claim is rejected (the original fact survived).
// rejectionConfidenceBps is the panel's confidence in the REJECT verdict —
// higher confidence = severer test survived (Falsificationism Gap 1).
func (k Keeper) handleChallengeSurvival(ctx context.Context, challengeClaim *types.Claim, rejectionConfidenceBps uint64) {
	if challengeClaim.ProvisionalFactId == "" {
		return
	}
	originalFact, found := k.GetFact(ctx, challengeClaim.ProvisionalFactId)
	if !found {
		return
	}
	params, _ := k.GetParams(ctx)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	// Energy boost for surviving a challenge — always credit, even if the
	// corroboration counter is rate-limited below. Energy reflects demand-
	// side engagement; the counter reflects epistemic hardening.
	originalFact.Energy += params.MetabolismEnergyChallengeSurvival
	if originalFact.Energy > params.MetabolismEnergyCap {
		originalFact.Energy = params.MetabolismEnergyCap
	}
	// Popperian corroboration with rate limit. Inverted challenge
	// economics make high-confidence facts cheap to probe, which is the
	// point — we want them stress-tested. But cheap probes also make
	// collusive corroboration-farming cheap: an attacker controlling N
	// addresses could fire N rapid challenges on their own fact to pump
	// BaseWeight. The cooldown caps the counter to one survival per
	// CorroborationCooldownBlocks per fact. Attackers can still probe
	// cheaply; they just can't farm credit for it. Organic probing from
	// independent challengers spread over time remains fully credited.
	const corroborationCooldownBlocks uint64 = 1_000
	eligible := originalFact.LastCorroboratedBlock == 0 ||
		height >= originalFact.LastCorroboratedBlock+corroborationCooldownBlocks
	if eligible {
		originalFact.CorroborationCount++
		originalFact.LastCorroboratedBlock = height
		// Phase 5: credit submitter only when the counter moves.
		k.RecordCorroborationForSubmitter(ctx, originalFact.Submitter, originalFact.MethodId)
	}
	// Falsificationism Gap 1: accumulate severity UNCONDITIONALLY (no cooldown).
	// Each challenge carries its own severity signal regardless of timing.
	// The severity score is the sum of rejection confidences — a series of
	// 90%-confident REJECTs is epistemically stronger than a series of 55%
	// ones. Farm-attackers cannot inflate this easily: calibrated verifiers
	// hedge on planted challenges, keeping rejection confidence low.
	originalFact.CorroborationSeverityScoreBps += rejectionConfidenceBps

	// Restore from challenged status regardless of cooldown — the fact
	// survived the probe either way.
	originalFact.Status = types.FactStatus_FACT_STATUS_ACTIVE
	originalFact.AtRiskSinceEpoch = 0
	_ = k.SetFact(ctx, originalFact)

	// Popper, not popularity: a fact's standing comes from how many
	// serious challenges it has survived. This event records one more
	// survival — the counter that BaseWeight reads to decide TVW.
	// See TRUTH_SEEKING.md commitment 3.
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.corroboration_incremented",
		sdk.NewAttribute("fact_id", originalFact.Id),
		sdk.NewAttribute("challenge_claim_id", challengeClaim.Id),
		sdk.NewAttribute("new_count", fmt.Sprintf("%d", originalFact.CorroborationCount)),
		sdk.NewAttribute("severity_score_bps", fmt.Sprintf("%d", originalFact.CorroborationSeverityScoreBps)),
		sdk.NewAttribute("rejection_confidence_bps", fmt.Sprintf("%d", rejectionConfidenceBps)),
		sdk.NewAttribute("block_height", fmt.Sprintf("%d", height)),
		sdk.NewAttribute("counter_incremented", fmt.Sprintf("%t", eligible)),
		sdk.NewAttribute("creed_commitment", "3"),
	))
}

// settleChallengeStake applies the challenge economic parameters to a
// finalized challenge claim. The verifier reward pool (55% of the stake)
// was already paid out in distributeVerifierRewardsFromPool; this function
// handles the remaining 45% that otherwise sits orphaned in the module
// account.
//
//	VERDICT_ACCEPT (challenge succeeded, bad fact disproven):
//	  - refund the 45% remainder to the challenger
//	  - pay SuccessfulChallengeRewardBps × stake as bonus, drawn from
//	    protocol treasury — the reward-for-finding-bad-facts signal
//	VERDICT_REJECT (challenge failed, fact survived):
//	  - route the 45% remainder to protocol treasury instead of leaving it
//	    stranded; this is the FailedChallengeSlashBps the fact's defender
//	    pays via the verifier pool plus what the chain absorbs from the
//	    collusion-farming attacker
//	other verdicts: funds stay in the knowledge module account as a no-op.
func (k Keeper) settleChallengeStake(ctx context.Context, claim *types.Claim, verdict types.Verdict, params *types.Params) {
	if k.bankKeeper == nil || claim == nil || claim.Stake == "" {
		return
	}
	stakeAmt, ok := new(big.Int).SetString(claim.Stake, 10)
	if !ok || stakeAmt.Sign() <= 0 {
		return
	}
	// Verifier pool consumed params.VerifierPoolBps of the stake. Remainder
	// is what the module still holds for this challenge.
	verifierPool := safeMulDiv(stakeAmt.Uint64(), params.VerifierPoolBps, 1_000_000)
	remainder := new(big.Int).Sub(stakeAmt, new(big.Int).SetUint64(verifierPool))
	if remainder.Sign() <= 0 {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	switch verdict {
	case types.Verdict_VERDICT_ACCEPT:
		challengerAddr, err := sdk.AccAddressFromBech32(claim.Submitter)
		if err != nil {
			return
		}
		refund := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(remainder)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, refund); err != nil {
			k.Logger(ctx).Error("challenge refund failed", "claim", claim.Id, "err", err)
			return
		}
		// Popperian amplification: the more confident the community was
		// in the disproven fact, the larger the paradigm shift and the
		// bigger the reward. Disproving a 90%-confidence claim is the
		// rarest, most-valuable signal the substrate produces; disproving
		// a 10% fact is routine cleanup. Reward scales with target
		// confidence.
		//
		// Reward is computed on the BASE MinChallengeStake, not the
		// (discounted) effective stake the challenger paid. Otherwise the
		// high-confidence discount would swallow the amplification: the
		// challenger who probed cheaply would also be rewarded cheaply
		// and the incentive to probe trusted facts would vanish.
		//
		//   effective_bonus_bps = base_reward + amp_bps × target_conf / BPS
		//   bonus = base_min_challenge_stake × effective_bonus_bps
		//
		// At target_conf = 0  → bonus = base × 30%      (routine cleanup)
		// At target_conf = 1  → bonus = base × 230%     (paradigm shift)
		const bonusAmplificationBps uint64 = 2_000_000
		const bps uint64 = 1_000_000
		rewardBps := params.SuccessfulChallengeRewardBps
		if rewardBps > 0 {
			effectiveBonusBps := rewardBps
			if claim.ProvisionalFactId != "" {
				if disproven, found := k.GetFact(ctx, claim.ProvisionalFactId); found && disproven != nil {
					conf := disproven.Confidence
					if conf > bps {
						conf = bps
					}
					amp := safeMulDiv(conf, bonusAmplificationBps, bps)
					effectiveBonusBps = rewardBps + amp
				}
			}
			// Compute the bonus from the base stake so the amplification
			// is measured against the un-discounted cost of probing.
			baseStake, ok := new(big.Int).SetString(params.MinChallengeStake, 10)
			if ok && baseStake.Sign() > 0 {
				bonusAmt := new(big.Int).Mul(baseStake, new(big.Int).SetUint64(effectiveBonusBps))
				bonusAmt.Div(bonusAmt, new(big.Int).SetUint64(bps))
				if bonusAmt.Sign() > 0 {
					// Wave 15: draw from the dedicated probe bounty
					// pool (per-block-minted audit budget) first; only
					// fall back to protocol treasury for any remainder
					// the pool can't cover. Purpose-built funding path
					// for continuous epistemic auditing.
					paid := k.PayProbeBountyFromPool(ctx, challengerAddr, bonusAmt)
					remaining := new(big.Int).Sub(bonusAmt, paid)
					if remaining.Sign() > 0 {
						bonusCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(remaining)))
						if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, protocolTreasuryModule, challengerAddr, bonusCoins); err != nil {
							k.Logger(ctx).Info("challenge bonus partially unfunded; probe pool paid what it could",
								"claim", claim.Id, "pool_paid", paid.String(), "treasury_short", remaining.String())
						}
					}
				}
			}
			// Substrate stress-tests its own truth: the chain
			// announces every challenge resolution because the
			// resolution itself is the audit. See TRUTH_SEEKING.md
			// commitment 4.
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.challenge_settled",
				sdk.NewAttribute("claim_id", claim.Id),
				sdk.NewAttribute("challenger", claim.Submitter),
				sdk.NewAttribute("outcome", "accepted"),
				sdk.NewAttribute("refund", remainder.String()),
				sdk.NewAttribute("reward_bps", fmt.Sprintf("%d", effectiveBonusBps)),
				sdk.NewAttribute("creed_commitment", "4"),
			))
			break
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.challenge_settled",
			sdk.NewAttribute("claim_id", claim.Id),
			sdk.NewAttribute("challenger", claim.Submitter),
			sdk.NewAttribute("outcome", "accepted"),
			sdk.NewAttribute("refund", remainder.String()),
			sdk.NewAttribute("reward_bps", "0"),
			sdk.NewAttribute("creed_commitment", "4"),
		))

	case types.Verdict_VERDICT_REJECT:
		// Probe participation reward: even a failed probe is an audit of
		// the fact. Return 15% of the original stake to the challenger to
		// encourage stress-testing — the chain thanks you for trying, even
		// when the fact holds. The remaining 30% of the original stake
		// flows to protocol treasury (the 55% verifier pool already paid
		// out in distributeVerifierRewardsFromPool).
		//
		//   Before: 55% verifiers,  0% challenger, 45% treasury
		//   After:  55% verifiers, 15% challenger, 30% treasury
		//
		// This turns the economic shape from "gamble that confiscates
		// stake on loss" to "participation-rewarded audit," which under
		// the inverted challenge pricing makes probing high-confidence
		// facts EV-positive for any challenger with even weak doubt.
		const probeParticipationBps uint64 = 150_000
		participation := new(big.Int).Mul(stakeAmt, new(big.Int).SetUint64(probeParticipationBps))
		participation.Div(participation, new(big.Int).SetUint64(1_000_000))
		toTreasury := new(big.Int).Sub(remainder, participation)

		if participation.Sign() > 0 {
			challengerAddr, err := sdk.AccAddressFromBech32(claim.Submitter)
			if err == nil {
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(participation)))
				if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, coins); err != nil {
					k.Logger(ctx).Error("probe participation refund failed", "claim", claim.Id, "err", err)
					// Fall through to treasury-route the full remainder if refund fails.
					toTreasury = remainder
				}
			} else {
				toTreasury = remainder
			}
		}

		if toTreasury.Sign() > 0 {
			coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(toTreasury)))
			if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, protocolTreasuryModule, coins); err != nil {
				k.Logger(ctx).Error("failed-challenge stake → treasury failed", "claim", claim.Id, "err", err)
				return
			}
		}

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.challenge_settled",
			sdk.NewAttribute("claim_id", claim.Id),
			sdk.NewAttribute("challenger", claim.Submitter),
			sdk.NewAttribute("outcome", "rejected"),
			sdk.NewAttribute("participation_refund", participation.String()),
			sdk.NewAttribute("to_treasury", toTreasury.String()),
			sdk.NewAttribute("creed_commitment", "4"),
		))
	}
}

// distributeVerifierReward sends a verification reward to a verifier.
func (k Keeper) distributeVerifierReward(ctx context.Context, verifier string, amount uint64) {
	if k.bankKeeper == nil || amount == 0 {
		return
	}
	addr, err := sdk.AccAddressFromBech32(verifier)
	if err != nil {
		return
	}
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(new(big.Int).SetUint64(amount))))
	_ = k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, coins)
}

// roundHasDissent checks if a round had any verifier dissent (mixed accept/reject votes).
func roundHasDissent(round *types.VerificationRound) bool {
	hasAccept, hasReject := false, false
	for _, reveal := range round.Reveals {
		switch reveal.Vote {
		case "accept":
			hasAccept = true
		case "reject":
			hasReject = true
		}
		if hasAccept && hasReject {
			return true
		}
	}
	return false
}

// payInvitationBonus pays a flat invitation bonus from the probe bounty
// pool when the prober answered a chain-issued invitation on the target
// fact. The invitation is current if the fact's ProbeInvitedAtBlock is
// set AND hasn't been superseded by a corroboration. Fires regardless
// of verdict — the chain pays for participation in answering its
// audit calls, not just for winning.
//
// Emits zerone.knowledge.invitation_bonus_paid on success.
func (k Keeper) payInvitationBonus(ctx context.Context, claim *types.Claim, params *types.Params) {
	if params == nil || claim == nil || claim.ProvisionalFactId == "" {
		return
	}
	amountStr := params.InvitationBonusAmount
	if amountStr == "" || amountStr == "0" {
		return
	}
	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || amount.Sign() <= 0 {
		return
	}
	fact, found := k.GetFact(ctx, claim.ProvisionalFactId)
	if !found || fact == nil {
		return
	}
	// Invitation must be current: stamped and not already superseded
	// by a corroboration (which resets the audit clock).
	if fact.ProbeInvitedAtBlock == 0 {
		return
	}
	if fact.LastCorroboratedBlock > fact.ProbeInvitedAtBlock {
		return
	}
	challengerAddr, err := sdk.AccAddressFromBech32(claim.Submitter)
	if err != nil {
		return
	}
	paid := k.PayProbeBountyFromPool(ctx, challengerAddr, amount)
	if paid.Sign() <= 0 {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// The chain manufactures probe demand (commitment 5) and pays for
	// its own audit (commitment 12): this event is the observable
	// proof that the bounty pool actually paid. Without it, "probes
	// are paid" is rhetoric. See TRUTH_SEEKING.md.
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.invitation_bonus_paid",
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("challenger", claim.Submitter),
		sdk.NewAttribute("fact_id", claim.ProvisionalFactId),
		sdk.NewAttribute("amount", paid.String()),
		sdk.NewAttribute("creed_commitment", "5,12"),
	))
}
