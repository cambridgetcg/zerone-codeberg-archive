package keeper

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Route B Wave 4 msg handlers ─────────────────────────────────────────
//
// Alignment invariant: the payer never judges, and the claimant never
// self-authorises. Every payout is gated on verifier-panel consensus,
// existing PoT adjudication, or realized live calibration — signals the
// chain already computes. See docs/ROUTE_B_WAVE4.md for the full design
// narrative.

// VoteOnAugmentation — a verifier records their verdict on a reformulation.
// Not callable by the submitter or by the sponsor (is-judge separation).
// On consensus, the verdict is finalised and — for EQUIVALENT/SUPERIOR —
// escrow is released to the submitter.
func (m *msgServer) VoteOnAugmentation(ctx context.Context, msg *types.MsgVoteOnAugmentation) (*types.MsgVoteOnAugmentationResponse, error) {
	if msg == nil || msg.AugmentationId == "" {
		return nil, fmt.Errorf("augmentation_id required")
	}
	if msg.Verifier == "" {
		return nil, fmt.Errorf("verifier required")
	}

	finalized, verdict, err := m.keeper.RecordAugmentationVote(ctx, msg.AugmentationId, msg.Verifier, msg.Vote)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.augmentation_vote_cast",
		sdk.NewAttribute("augmentation_id", msg.AugmentationId),
		sdk.NewAttribute("verifier", msg.Verifier),
		sdk.NewAttribute("vote", msg.Vote.String()),
		sdk.NewAttribute("finalized", fmt.Sprintf("%t", finalized)),
	))

	if finalized {
		if err := m.keeper.ApplyFinalizedAugmentationVerdict(ctx, msg.AugmentationId, verdict); err != nil {
			return nil, err
		}
	}
	return &types.MsgVoteOnAugmentationResponse{
		VerdictFinalized:  finalized,
		FinalizedVerdict:  verdict,
	}, nil
}

// SponsorVetoAugmentation — sponsor rejects a passing verdict, forfeiting
// the deposit to the research fund. The only way for a sponsor to block a
// verdict; prevents silent rejection of legitimate variants.
func (m *msgServer) SponsorVetoAugmentation(ctx context.Context, msg *types.MsgSponsorVetoAugmentation) (*types.MsgSponsorVetoAugmentationResponse, error) {
	if msg == nil || msg.AugmentationId == "" {
		return nil, fmt.Errorf("augmentation_id required")
	}
	aug, ok := m.keeper.GetAugmentation(ctx, msg.AugmentationId)
	if !ok {
		return nil, fmt.Errorf("augmentation %s not found", msg.AugmentationId)
	}
	if aug.BountyId == "" {
		return nil, fmt.Errorf("volunteer augmentations have no sponsor to veto")
	}
	bounty, ok := m.keeper.GetAugmentationBounty(ctx, aug.BountyId)
	if !ok {
		return nil, fmt.Errorf("bounty %s vanished", aug.BountyId)
	}
	if bounty.SponsorAddress != msg.Sponsor {
		return nil, fmt.Errorf("only the sponsor may veto")
	}
	passing := aug.Verdict == types.AugmentationVerdict_AUGMENTATION_VERDICT_EQUIVALENT ||
		aug.Verdict == types.AugmentationVerdict_AUGMENTATION_VERDICT_SUPERIOR
	if !passing {
		return nil, fmt.Errorf("veto only applies to passing verdicts")
	}
	if aug.SponsorVetoed {
		return nil, fmt.Errorf("already vetoed")
	}
	aug.SponsorVetoed = true
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Forfeit the payout amount to the research fund. If payout was not yet
	// applied (e.g. veto before finalize's payout step), use the bounty's
	// reward_per_variant as the forfeited amount.
	forfeitAmt, ok := sdkmath.NewIntFromString(aug.PayoutAmount)
	if !ok || forfeitAmt.IsZero() {
		forfeitAmt = sdkmath.NewIntFromUint64(bounty.RewardPerVariant)
	}
	if err := m.keeper.ForfeitAugmentationDeposit(ctx, aug.BountyId, forfeitAmt); err != nil {
		return nil, err
	}

	// If payout already went out, we don't recall it. The sponsor loses the
	// equivalent amount from remaining escrow (fairness: the submitter is
	// whole; the sponsor bears the cost of an indefensible veto).
	aug.PayoutAmount = forfeitAmt.String() + " (forfeited)"
	if err := m.keeper.SetAugmentation(ctx, aug); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.augmentation_sponsor_vetoed",
		sdk.NewAttribute("augmentation_id", aug.Id),
		sdk.NewAttribute("sponsor", msg.Sponsor),
		sdk.NewAttribute("forfeited_amount", forfeitAmt.String()),
		sdk.NewAttribute("reason", msg.Reason),
	))
	return &types.MsgSponsorVetoAugmentationResponse{}, nil
}

// ChallengeContribution — a fact submitter disputes a ContributionRecord.
// "missing" asserts the model trained on the fact but the owner omitted it;
// "fraudulent" asserts the listed fact wasn't actually used. Bond is locked
// in the TrainingFund and paid out to the winner on resolution.
func (m *msgServer) ChallengeContribution(ctx context.Context, msg *types.MsgChallengeContribution) (*types.MsgChallengeContributionResponse, error) {
	if msg == nil || msg.Id == "" {
		return nil, fmt.Errorf("challenge id required")
	}
	if msg.Challenger == "" || msg.ModelId == "" || msg.DisputedFactId == "" {
		return nil, fmt.Errorf("challenger, model_id, disputed_fact_id required")
	}
	if msg.DisputeType != "missing" && msg.DisputeType != "fraudulent" {
		return nil, fmt.Errorf(`dispute_type must be "missing" or "fraudulent"`)
	}
	if _, exists := m.keeper.GetContributionChallenge(ctx, msg.Id); exists {
		return nil, fmt.Errorf("challenge %s already exists", msg.Id)
	}
	if _, ok := m.keeper.GetContributionRecord(ctx, msg.ModelId); !ok {
		return nil, fmt.Errorf("no contribution record for model %s", msg.ModelId)
	}

	params, _ := m.keeper.GetParams(ctx)
	bondStr := params.ContributionChallengeBond
	if bondStr == "" {
		bondStr = "5000000"
	}
	bondAmt, ok := sdkmath.NewIntFromString(bondStr)
	if !ok {
		return nil, fmt.Errorf("invalid bond param")
	}

	challengerAddr, err := sdk.AccAddressFromBech32(msg.Challenger)
	if err != nil {
		return nil, fmt.Errorf("invalid challenger address: %w", err)
	}
	if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.TrainingFundModuleName, sdk.NewCoins(sdk.NewCoin("uzrn", bondAmt))); err != nil {
		return nil, fmt.Errorf("bond escrow failed: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	challenge := &types.ContributionChallenge{
		Id:              msg.Id,
		ModelId:         msg.ModelId,
		Challenger:      msg.Challenger,
		DisputedFactId:  msg.DisputedFactId,
		DisputeType:     msg.DisputeType,
		Bond:            bondAmt.String(),
		CreatedAtBlock:  uint64(sdkCtx.BlockHeight()),
		Evidence:        msg.Evidence,
		Status:          "open",
	}
	if err := m.keeper.SetContributionChallenge(ctx, challenge); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.contribution_challenge_opened",
		sdk.NewAttribute("challenge_id", challenge.Id),
		sdk.NewAttribute("model_id", challenge.ModelId),
		sdk.NewAttribute("challenger", challenge.Challenger),
		sdk.NewAttribute("dispute_type", challenge.DisputeType),
		sdk.NewAttribute("bond", challenge.Bond),
	))
	return &types.MsgChallengeContributionResponse{BondEscrowed: challenge.Bond}, nil
}

// ResolveContributionChallenge — settles a challenge. For now the resolver
// must be the governance authority (a verifier-panel hook can be added once
// a ContributionRound type is wired). On uphold, challenger gets bond × N;
// on reject, challenger forfeits to the research fund.
func (m *msgServer) ResolveContributionChallenge(ctx context.Context, msg *types.MsgResolveContributionChallenge) (*types.MsgResolveContributionChallengeResponse, error) {
	if msg == nil || msg.ChallengeId == "" {
		return nil, fmt.Errorf("challenge_id required")
	}
	if msg.Resolver != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("only governance authority may resolve (verifier panel binding pending)")
	}
	challenge, ok := m.keeper.GetContributionChallenge(ctx, msg.ChallengeId)
	if !ok {
		return nil, fmt.Errorf("challenge %s not found", msg.ChallengeId)
	}
	if challenge.Status != "open" {
		return nil, fmt.Errorf("challenge %s is not open", msg.ChallengeId)
	}

	params, _ := m.keeper.GetParams(ctx)
	mult := params.ContributionChallengeRewardMultiplierBps
	if mult == 0 {
		mult = 2_000_000 // 2×
	}
	bond, ok := sdkmath.NewIntFromString(challenge.Bond)
	if !ok {
		return nil, fmt.Errorf("invalid bond amount on challenge")
	}
	payout := bond.Mul(sdkmath.NewIntFromUint64(mult)).Quo(sdkmath.NewIntFromUint64(1_000_000))

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	var paidOut sdkmath.Int
	if msg.Uphold {
		// Winner = challenger: refund bond + reward. The fund already holds
		// the bond; the reward portion is minted from the training-fund
		// Minter permission (the loser is the model owner, who didn't post
		// a bond; slashing them directly would require auth the model
		// registry doesn't yet expose — use mint-as-reward until that lands).
		rewardPortion := payout.Sub(bond)
		if rewardPortion.IsPositive() {
			// Mint through the global supply cap (P-18b-b). If the cap is
			// exhausted, pay out from bond only — challenger still recovers
			// their stake, just without the mint-as-reward portion.
			actualBig, mintErr := m.keeper.MintWithCap(ctx, types.TrainingFundModuleName, rewardPortion.BigInt())
			if mintErr != nil {
				return nil, mintErr
			}
			if actualBig.Sign() > 0 {
				minted := sdkmath.NewIntFromBigInt(actualBig)
				if minted.LT(rewardPortion) {
					// Partial cap headroom — reduce payout to bond + actual minted.
					payout = bond.Add(minted)
				}
				// If minted == rewardPortion, payout stays as calculated.
			} else {
				// Cap exhausted — reward from bond only.
				payout = bond
			}
		}
		challengerAddr, err := sdk.AccAddressFromBech32(challenge.Challenger)
		if err != nil {
			return nil, err
		}
		if err := m.keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.TrainingFundModuleName, challengerAddr, sdk.NewCoins(sdk.NewCoin("uzrn", payout))); err != nil {
			return nil, err
		}
		paidOut = payout
		challenge.Status = "upheld"
	} else {
		// Challenger forfeits bond to research fund.
		if m.keeper.vestingRewardsKeeper != nil {
			if err := m.keeper.vestingRewardsKeeper.DepositToResearchFund(ctx, types.TrainingFundModuleName, sdk.NewCoins(sdk.NewCoin("uzrn", bond))); err != nil {
				return nil, err
			}
		}
		paidOut = sdkmath.ZeroInt()
		challenge.Status = "rejected"
	}
	challenge.ResolvedAtBlock = uint64(sdkCtx.BlockHeight())
	challenge.Resolver = msg.Resolver
	challenge.ResolutionNote = msg.Note
	if err := m.keeper.SetContributionChallenge(ctx, challenge); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.contribution_challenge_resolved",
		sdk.NewAttribute("challenge_id", challenge.Id),
		sdk.NewAttribute("status", challenge.Status),
		sdk.NewAttribute("payout", paidOut.String()),
		sdk.NewAttribute("resolver", msg.Resolver),
	))
	return &types.MsgResolveContributionChallengeResponse{PayoutToWinner: paidOut.String()}, nil
}

// ClaimTrainingFundDisbursement — post-hoc, calibration-gated reward. The
// claimant is the pipeline's operator. Amount is computed mechanically from
// on-chain signals (calibration score + methodology diversity + base).
// 50% paid immediately; 50% vests over TrainingFundVestingEpochs and is
// clawed back if calibration drops.
func (m *msgServer) ClaimTrainingFundDisbursement(ctx context.Context, msg *types.MsgClaimTrainingFundDisbursement) (*types.MsgClaimTrainingFundDisbursementResponse, error) {
	if msg == nil || msg.Id == "" || msg.ModelId == "" || msg.Claimant == "" {
		return nil, fmt.Errorf("id, model_id, claimant required")
	}
	if _, exists := m.keeper.GetTrainingFundDisbursement(ctx, msg.Id); exists {
		return nil, fmt.Errorf("disbursement %s already exists", msg.Id)
	}
	card, ok := m.keeper.GetModelCard(ctx, msg.ModelId)
	if !ok {
		return nil, fmt.Errorf("model %s not found", msg.ModelId)
	}
	if !card.Active {
		return nil, fmt.Errorf("model %s is retired — no disbursement", msg.ModelId)
	}
	pipeline, ok := m.keeper.GetTrainingPipeline(ctx, card.PipelineId)
	if !ok {
		return nil, fmt.Errorf("pipeline %s not found", card.PipelineId)
	}
	if pipeline.OperatorAddress != msg.Claimant {
		return nil, fmt.Errorf("only the pipeline operator may claim")
	}

	// Pull the deployed model's calibration as the primary signal.
	params, _ := m.keeper.GetParams(ctx)
	floor := params.TrainingFundCalibrationFloorBps
	if floor == 0 {
		floor = 500_000
	}
	cal, calOk := m.keeper.GetAgentCalibration(ctx, card.DeploymentAddress)
	if !calOk || cal.CalibrationScoreBps < floor {
		return nil, fmt.Errorf("calibration %d below floor %d — disbursement gated", func() uint64 { if cal == nil { return 0 }; return cal.CalibrationScoreBps }(), floor)
	}

	// Base reward.
	baseStr := params.TrainingFundBaseReward
	if baseStr == "" {
		baseStr = "1000000000"
	}
	base, ok := sdkmath.NewIntFromString(baseStr)
	if !ok || base.IsZero() {
		return nil, fmt.Errorf("invalid base reward param")
	}

	// Scale by calibration proportion above floor (linear up to BPS=1.0).
	excess := cal.CalibrationScoreBps - floor
	ceiling := uint64(1_000_000) - floor
	if ceiling == 0 {
		ceiling = 1
	}
	calFactor := uint64(1_000_000)
	if excess < ceiling {
		// scale 1.0× at floor → 2.0× at BPS=1.0
		calFactor = 1_000_000 + (excess*1_000_000)/ceiling
	} else {
		calFactor = 2_000_000
	}
	scaled := base.Mul(sdkmath.NewIntFromUint64(calFactor)).Quo(sdkmath.NewIntFromUint64(1_000_000))

	// Methodology diversity — bonus per distinct methodology beyond 1.
	diversity := m.keeper.countPipelineMethodologyDiversity(ctx, card.PipelineId)
	diversityBonusBps := params.TrainingFundMethodologyDiversityBonusBps
	if diversity > 1 && diversityBonusBps > 0 {
		extra := scaled.Mul(sdkmath.NewIntFromUint64(uint64(diversity-1) * diversityBonusBps)).Quo(sdkmath.NewIntFromUint64(1_000_000))
		scaled = scaled.Add(extra)
	}

	// Mint into the training fund through the global supply cap (P-18b-b).
	// If the cap is exhausted, minting is suppressed rather than silently
	// over-issuing — the disbursement fails so callers can retry later.
	actualBig, err := m.keeper.MintWithCap(ctx, types.TrainingFundModuleName, scaled.BigInt())
	if err != nil {
		return nil, fmt.Errorf("mint to training fund failed: %w", err)
	}
	if actualBig.Sign() <= 0 {
		return nil, fmt.Errorf("supply cap exhausted — training fund disbursement suppressed")
	}
	scaled = sdkmath.NewIntFromBigInt(actualBig) // use actual minted (may be < requested)
	released := scaled.Quo(sdkmath.NewInt(2))
	vesting := scaled.Sub(released)

	claimantAddr, err := sdk.AccAddressFromBech32(msg.Claimant)
	if err != nil {
		return nil, err
	}
	if !released.IsZero() {
		if err := m.keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.TrainingFundModuleName, claimantAddr, sdk.NewCoins(sdk.NewCoin("uzrn", released))); err != nil {
			return nil, err
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	vestingEpochs := params.TrainingFundVestingEpochs
	if vestingEpochs == 0 {
		vestingEpochs = 60
	}
	fitnessEpoch := params.FitnessEpochBlocks
	if fitnessEpoch == 0 {
		fitnessEpoch = 1_111
	}
	vestEnd := uint64(sdkCtx.BlockHeight()) + vestingEpochs*fitnessEpoch

	d := &types.TrainingFundDisbursement{
		Id:                          msg.Id,
		ModelId:                     msg.ModelId,
		PipelineId:                  card.PipelineId,
		Claimant:                    msg.Claimant,
		ClaimedAtBlock:              uint64(sdkCtx.BlockHeight()),
		TotalAmount:                 scaled.String(),
		ReleasedAmount:              released.String(),
		VestingAmount:               vesting.String(),
		VestingEndBlock:             vestEnd,
		CalibrationScoreAtClaimBps:  cal.CalibrationScoreBps,
		MethodologyDiversityCount:   uint64(diversity),
		ReproducibilityProofPresent: false, // reserved for verification axis
	}
	if err := m.keeper.SetTrainingFundDisbursement(ctx, d); err != nil {
		return nil, err
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.training_fund_disbursed",
		sdk.NewAttribute("disbursement_id", d.Id),
		sdk.NewAttribute("model_id", d.ModelId),
		sdk.NewAttribute("claimant", d.Claimant),
		sdk.NewAttribute("released", d.ReleasedAmount),
		sdk.NewAttribute("vesting", d.VestingAmount),
		sdk.NewAttribute("vesting_end_block", fmt.Sprintf("%d", d.VestingEndBlock)),
	))
	return &types.MsgClaimTrainingFundDisbursementResponse{
		TotalAmount:     d.TotalAmount,
		ReleasedAmount:  d.ReleasedAmount,
		VestingAmount:   d.VestingAmount,
		VestingEndBlock: d.VestingEndBlock,
	}, nil
}

// countPipelineMethodologyDiversity — walks THIS pipeline's finalized /
// attested manifests and counts the distinct methodology_ids across the
// facts those manifests actually committed to. The earlier implementation
// ignored the pipelineID and scanned every GOLD-tier fact globally, which
// made the diversity-bonus a public good that every pipeline harvested
// equally regardless of what it trained on — a farming vector by default.
// Scoping to the pipeline's own manifests makes the bonus proportional to
// the operator's real methodological breadth.
func (k Keeper) countPipelineMethodologyDiversity(ctx context.Context, pipelineID string) uint32 {
	if pipelineID == "" {
		return 0
	}
	methods := make(map[string]struct{})
	k.IterateTrainingManifests(ctx, func(m *types.TrainingManifest) bool {
		if m == nil || m.PipelineId != pipelineID {
			return false
		}
		if m.Status != types.ManifestStatus_MANIFEST_STATUS_FINALIZED &&
			m.Status != types.ManifestStatus_MANIFEST_STATUS_ATTESTED {
			return false
		}
		for _, factID := range m.IncludedFactIds {
			fact, ok := k.GetFact(ctx, factID)
			if !ok || fact == nil || fact.MethodId == "" {
				continue
			}
			methods[fact.MethodId] = struct{}{}
		}
		return false
	})
	if len(methods) > int(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(len(methods))
}
