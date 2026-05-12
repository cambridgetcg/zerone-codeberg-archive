package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CreateVerificationRound creates a new verification round for a claim.
func (k Keeper) CreateVerificationRound(ctx context.Context, claim *types.Claim) (*types.VerificationRound, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	roundID := GenerateRoundID(claim.Id, height)

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             claim.Id,
		StartedAtBlock:      height,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_COMMIT,
		CommitDeadline:      height + params.CommitPhaseBlocks,
		RevealDeadline:      height + params.CommitPhaseBlocks + params.RevealPhaseBlocks,
		AggregationDeadline: height + params.CommitPhaseBlocks + params.RevealPhaseBlocks + params.AggregationPhaseBlocks,
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

	round.Verdict = result.Verdict
	round.VerdictBlock = height
	round.Phase = types.VerificationPhase_VERIFICATION_PHASE_COMPLETE

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

	case types.Verdict_VERDICT_REJECT:
		// Review fee already distributed at submission time — the fee IS the cost of rejection.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_REJECTED
		// If this was a challenge claim, the original fact survived — energy boost
		k.handleChallengeSurvival(ctx, claim)
		// Challenge lost: record on the challenger's side of the ledger (Phase 5).
		if claim.ProvisionalFactId != "" {
			k.RecordChallengeOutcome(ctx, claim.Submitter, false)
		}
		// Contradicting claim was rejected — undo its CONTESTED side-effect on target facts (T-i4).
		k.reverseContradictionsFromClaim(ctx, claim)

	case types.Verdict_VERDICT_MALFORMED:
		// Review fee already distributed at submission time — no additional slashing needed.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_MALFORMED
		k.reverseContradictionsFromClaim(ctx, claim)

	case types.Verdict_VERDICT_INCONCLUSIVE:
		// Review fee is non-refundable — verifiers still did work even if inconclusive.
		claim.Status = types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT
		k.reverseContradictionsFromClaim(ctx, claim)
	}

	if err := k.SetClaim(ctx, claim); err != nil {
		return err
	}
	if err := k.SetVerificationRound(ctx, round); err != nil {
		return err
	}

	// Distribute verifier rewards from the 55% fee pool
	k.distributeVerifierRewardsFromPool(ctx, claim, result)

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
	))

	return nil
}

// distributeVerifierRewardsFromPool distributes the 55% verifier pool among correct verifiers.
// Each verifier's share is modulated by their historical independence score (T3):
// persistent crowd-followers receive up to IndependenceRewardStrengthBps less than
// they otherwise would, with the withheld amount flowing to the development fund.
func (k Keeper) distributeVerifierRewardsFromPool(ctx context.Context, claim *types.Claim, result *VerificationResult) {
	if k.bankKeeper == nil || len(result.Rewards) == 0 {
		return
	}

	// Calculate pool: 55% of the original review fee
	feeAmt, ok := new(big.Int).SetString(claim.Stake, 10)
	if !ok || feeAmt.Sign() <= 0 {
		return
	}
	poolAmount := verifierPoolFromFee(feeAmt.Uint64())
	if poolAmount == 0 {
		return
	}

	params, _ := k.GetParams(ctx)

	// Divide pool equally among rewarded verifiers
	perVerifier := poolAmount / uint64(len(result.Rewards))
	remainder := poolAmount - (perVerifier * uint64(len(result.Rewards)))

	var withheldTotal uint64
	for i, reward := range result.Rewards {
		amount := perVerifier
		if i == 0 {
			amount += remainder // first verifier gets dust
		}
		modulated := k.applyIndependenceMultiplier(ctx, reward.Verifier, amount, params)
		if modulated < amount {
			withheldTotal += amount - modulated
		}
		k.distributeVerifierReward(ctx, reward.Verifier, modulated)
	}

	// Withheld portion flows to development fund so the budget doesn't compound
	// for crowd-followers in the verifier pool.
	if withheldTotal > 0 {
		k.forwardWithheldToDevelopmentFund(ctx, withheldTotal)
	}
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

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.fact_relation_created",
			sdk.NewAttribute("source", factRel.SourceFactId),
			sdk.NewAttribute("target", factRel.TargetFactId),
			sdk.NewAttribute("relation", factRel.Relation.String()),
		))
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
func (k Keeper) handleChallengeSurvival(ctx context.Context, challengeClaim *types.Claim) {
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
	// Verifier pool consumed reviewFeeContributorBps of the stake. Remainder
	// is what the module still holds for this challenge.
	verifierPool := safeMulDiv(stakeAmt.Uint64(), reviewFeeContributorBps, 1_000_000)
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
