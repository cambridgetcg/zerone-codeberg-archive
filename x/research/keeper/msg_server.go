package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/research/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	Keeper
}

// NewMsgServerImpl returns a msg server implementation.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// SubmitResearch handles a new research submission.
func (m msgServer) SubmitResearch(
	goCtx context.Context,
	msg *types.MsgSubmitResearch,
) (*types.MsgSubmitResearchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := m.Keeper.GetParams(ctx)

	// Validate stake meets minimum
	stakeInt := new(big.Int)
	if _, ok := stakeInt.SetString(msg.Stake, 10); !ok {
		return nil, fmt.Errorf("invalid stake amount")
	}
	minStake := new(big.Int)
	minStake.SetString(params.MinResearchStake, 10)
	if stakeInt.Cmp(minStake) < 0 {
		return nil, fmt.Errorf("%w: need %s, got %s", types.ErrInsufficientStake, params.MinResearchStake, msg.Stake)
	}

	// Transfer stake to module
	submitterAddr, err := sdk.AccAddressFromBech32(msg.Submitter)
	if err != nil {
		return nil, fmt.Errorf("invalid submitter address: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeInt)))
	if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, submitterAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock stake: %w", err)
	}

	// Create research submission
	researchId := m.Keeper.nextResearchId(ctx)
	research := &types.Research{
		Id:          researchId,
		Submitter:   msg.Submitter,
		Title:       msg.Title,
		Description: msg.Description,
		Domain:      msg.Domain,
		Stake:       msg.Stake,
		Status:      string(types.ResearchStatusSubmitted),
		CreatedAt:   uint64(ctx.BlockHeight()),
		UpdatedAt:   uint64(ctx.BlockHeight()),
	}
	m.Keeper.SetResearch(ctx, research)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.research_submitted",
			sdk.NewAttribute("research_id", researchId),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("domain", msg.Domain),
			sdk.NewAttribute("stake", msg.Stake),
		),
	)

	return &types.MsgSubmitResearchResponse{ResearchId: researchId}, nil
}

// ChallengeResearch challenges an existing research submission.
func (m msgServer) ChallengeResearch(
	goCtx context.Context,
	msg *types.MsgChallengeResearch,
) (*types.MsgChallengeResearchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := m.Keeper.GetParams(ctx)

	// Fetch research
	research, found := m.Keeper.GetResearch(ctx, msg.ResearchId)
	if !found {
		return nil, types.ErrSubmissionNotFound
	}

	// Only submitted or under_review can be challenged
	if research.Status != string(types.ResearchStatusSubmitted) && research.Status != string(types.ResearchStatusUnderReview) {
		return nil, types.ErrResearchNotChallengeable
	}

	// Validate stake
	stakeInt := new(big.Int)
	stakeInt.SetString(msg.Stake, 10)
	minStake := new(big.Int)
	minStake.SetString(params.MinChallengeStake, 10)
	if stakeInt.Cmp(minStake) < 0 {
		return nil, fmt.Errorf("%w: need %s, got %s", types.ErrInsufficientStake, params.MinChallengeStake, msg.Stake)
	}

	// Lock challenger's stake
	challengerAddr, _ := sdk.AccAddressFromBech32(msg.Challenger)
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeInt)))
	if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock challenge stake: %w", err)
	}

	// Update status
	research.Status = string(types.ResearchStatusChallenged)
	research.UpdatedAt = uint64(ctx.BlockHeight())
	m.Keeper.SetResearch(ctx, research)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.research_challenged",
			sdk.NewAttribute("research_id", msg.ResearchId),
			sdk.NewAttribute("challenger", msg.Challenger),
			sdk.NewAttribute("reason", msg.Reason),
		),
	)

	return &types.MsgChallengeResearchResponse{}, nil
}

// ReviewResearch submits a peer review for a research submission.
func (m msgServer) ReviewResearch(
	goCtx context.Context,
	msg *types.MsgReviewResearch,
) (*types.MsgReviewResearchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Fetch research
	research, found := m.Keeper.GetResearch(ctx, msg.ResearchId)
	if !found {
		return nil, types.ErrSubmissionNotFound
	}

	// Must be submitted or under_review to accept reviews
	if research.Status != string(types.ResearchStatusSubmitted) && research.Status != string(types.ResearchStatusUnderReview) {
		return nil, types.ErrResearchNotUnderReview
	}

	// Check duplicate review
	if m.Keeper.HasReviewerReviewed(ctx, msg.ResearchId, msg.Reviewer) {
		return nil, types.ErrAlreadyReviewed
	}

	// Map proto enum verdict to string for storage
	var verdictStr string
	switch msg.Verdict {
	case types.ReviewVerdict_REVIEW_VERDICT_APPROVE:
		verdictStr = "approve"
	case types.ReviewVerdict_REVIEW_VERDICT_REJECT:
		verdictStr = "reject"
	case types.ReviewVerdict_REVIEW_VERDICT_REVISE:
		verdictStr = "revise"
	default:
		verdictStr = "unspecified"
	}

	// Create peer review
	reviewId := m.Keeper.nextReviewId(ctx)
	review := &types.PeerReview{
		Id:         reviewId,
		ResearchId: msg.ResearchId,
		Reviewer:   msg.Reviewer,
		Verdict:    verdictStr,
		Score:      msg.QualityScore,
		Reasoning:  msg.Reasoning,
		ReviewedAt: uint64(ctx.BlockHeight()),
	}
	m.Keeper.SetPeerReview(ctx, review)

	// Update research review counts
	research.ReviewCount++
	switch msg.Verdict {
	case types.ReviewVerdict_REVIEW_VERDICT_APPROVE:
		research.ApproveCount++
	case types.ReviewVerdict_REVIEW_VERDICT_REJECT:
		research.RejectCount++
	case types.ReviewVerdict_REVIEW_VERDICT_REVISE:
		research.ReviseCount++
	}

	// Transition to under_review after first review
	if research.Status == string(types.ResearchStatusSubmitted) {
		research.Status = string(types.ResearchStatusUnderReview)
	}

	// Update aggregate score (simple average of all review scores)
	reviews := m.Keeper.GetReviewsForResearch(ctx, msg.ResearchId)
	var totalScore uint32
	for _, r := range reviews {
		totalScore += r.Score
	}
	research.AggregateScore = totalScore / uint32(len(reviews))

	research.UpdatedAt = uint64(ctx.BlockHeight())
	m.Keeper.SetResearch(ctx, research)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.research_reviewed",
			sdk.NewAttribute("research_id", msg.ResearchId),
			sdk.NewAttribute("reviewer", msg.Reviewer),
			sdk.NewAttribute("verdict", verdictStr),
			sdk.NewAttribute("score", fmt.Sprintf("%d", msg.QualityScore)),
		),
	)

	return &types.MsgReviewResearchResponse{}, nil
}

// ResolveResearch resolves a research submission after sufficient reviews.
func (m msgServer) ResolveResearch(
	goCtx context.Context,
	msg *types.MsgResolveResearch,
) (*types.MsgResolveResearchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Authority check
	if msg.Authority != m.Keeper.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	params := m.Keeper.GetParams(ctx)

	// Fetch research
	research, found := m.Keeper.GetResearch(ctx, msg.ResearchId)
	if !found {
		return nil, types.ErrSubmissionNotFound
	}

	if research.Status != string(types.ResearchStatusUnderReview) {
		return nil, types.ErrResearchNotUnderReview
	}

	// Need minimum reviews
	if research.ReviewCount < params.MinReviewerCount {
		return nil, fmt.Errorf("%w: need %d, have %d", types.ErrInsufficientReviews, params.MinReviewerCount, research.ReviewCount)
	}

	// Determine outcome from aggregate score
	var outcome types.ResearchOutcome
	if research.AggregateScore >= params.AcceptanceScoreThreshold {
		outcome = types.ResearchOutcome_RESEARCH_OUTCOME_ACCEPTED
		research.Status = string(types.ResearchStatusAccepted)

		// Return stake to submitter
		stakeInt := new(big.Int)
		stakeInt.SetString(research.Stake, 10)
		submitterAddr, _ := sdk.AccAddressFromBech32(research.Submitter)
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeInt)))
		m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, submitterAddr, coins)
	} else {
		outcome = types.ResearchOutcome_RESEARCH_OUTCOME_REJECTED
		research.Status = string(types.ResearchStatusRejected)

		// Slash stake
		stakeInt := new(big.Int)
		stakeInt.SetString(research.Stake, 10)
		slashRate := new(big.Int).SetUint64(params.RejectionSlashBps)
		slashAmount := new(big.Int).Mul(stakeInt, slashRate)
		slashAmount.Div(slashAmount, new(big.Int).SetUint64(1000000))

		// Route slashed amount to development fund
		if slashAmount.Sign() > 0 {
			slashCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(slashAmount)))
			m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "development_fund", slashCoins)
		}

		// Return remainder to submitter
		remainder := new(big.Int).Sub(stakeInt, slashAmount)
		if remainder.Sign() > 0 {
			submitterAddr, _ := sdk.AccAddressFromBech32(research.Submitter)
			returnCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(remainder)))
			m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, submitterAddr, returnCoins)
		}
	}

	research.UpdatedAt = uint64(ctx.BlockHeight())
	m.Keeper.SetResearch(ctx, research)

	var outcomeStr string
	if outcome == types.ResearchOutcome_RESEARCH_OUTCOME_ACCEPTED {
		outcomeStr = "accepted"
	} else {
		outcomeStr = "rejected"
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.research_resolved",
			sdk.NewAttribute("research_id", msg.ResearchId),
			sdk.NewAttribute("outcome", outcomeStr),
			sdk.NewAttribute("aggregate_score", fmt.Sprintf("%d", research.AggregateScore)),
		),
	)

	return &types.MsgResolveResearchResponse{Outcome: outcome}, nil
}

// CreateBounty creates a new research bounty.
func (m msgServer) CreateBounty(
	goCtx context.Context,
	msg *types.MsgCreateBounty,
) (*types.MsgCreateBountyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := m.Keeper.GetParams(ctx)

	// Validate deadline
	if msg.DeadlineHeight <= uint64(ctx.BlockHeight())+params.BountyMinDeadlineBlocks {
		return nil, fmt.Errorf("%w: deadline must be at least %d blocks in the future",
			types.ErrDeadlineTooSoon, params.BountyMinDeadlineBlocks)
	}

	// Validate reward
	rewardInt := new(big.Int)
	rewardInt.SetString(msg.Reward, 10)
	maxReward := new(big.Int)
	maxReward.SetString(params.MaxBountyReward, 10)
	if rewardInt.Cmp(maxReward) > 0 {
		return nil, types.ErrExceedsMaxReward
	}

	// Lock reward from creator
	creatorAddr, _ := sdk.AccAddressFromBech32(msg.Creator)
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(rewardInt)))
	if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, creatorAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock bounty reward: %w", err)
	}

	bountyId := m.Keeper.nextBountyId(ctx)
	bounty := &types.Bounty{
		Id:             bountyId,
		Creator:        msg.Creator,
		Title:          msg.Title,
		Description:    msg.Description,
		Reward:         msg.Reward,
		DeadlineHeight: msg.DeadlineHeight,
		Status:         string(types.BountyStatusOpen),
		CreatedAt:      uint64(ctx.BlockHeight()),
	}
	m.Keeper.SetBounty(ctx, bounty)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.bounty_created",
			sdk.NewAttribute("bounty_id", bountyId),
			sdk.NewAttribute("creator", msg.Creator),
			sdk.NewAttribute("reward", msg.Reward),
		),
	)

	return &types.MsgCreateBountyResponse{BountyId: bountyId}, nil
}

// ClaimBounty claims an open bounty.
func (m msgServer) ClaimBounty(
	goCtx context.Context,
	msg *types.MsgClaimBounty,
) (*types.MsgClaimBountyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bounty, found := m.Keeper.GetBounty(ctx, msg.BountyId)
	if !found {
		return nil, types.ErrBountyNotFound
	}

	if bounty.Status != string(types.BountyStatusOpen) {
		return nil, types.ErrBountyNotOpen
	}

	// Check deadline
	if uint64(ctx.BlockHeight()) > bounty.DeadlineHeight {
		return nil, types.ErrBountyExpired
	}

	bounty.Status = string(types.BountyStatusClaimed)
	bounty.ClaimedBy = msg.Claimer
	bounty.ClaimedAt = uint64(ctx.BlockHeight())
	m.Keeper.SetBounty(ctx, bounty)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.bounty_claimed",
			sdk.NewAttribute("bounty_id", msg.BountyId),
			sdk.NewAttribute("claimer", msg.Claimer),
		),
	)

	return &types.MsgClaimBountyResponse{}, nil
}

// FulfillBounty marks a bounty as fulfilled and pays the reward.
func (m msgServer) FulfillBounty(
	goCtx context.Context,
	msg *types.MsgFulfillBounty,
) (*types.MsgFulfillBountyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Authority check
	if msg.Authority != m.Keeper.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	bounty, found := m.Keeper.GetBounty(ctx, msg.BountyId)
	if !found {
		return nil, types.ErrBountyNotFound
	}

	if bounty.Status != string(types.BountyStatusClaimed) {
		return nil, types.ErrBountyNotClaimed
	}

	// Pay reward to claimer
	rewardInt := new(big.Int)
	rewardInt.SetString(bounty.Reward, 10)
	claimerAddr, _ := sdk.AccAddressFromBech32(bounty.ClaimedBy)
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(rewardInt)))
	if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, claimerAddr, coins); err != nil {
		return nil, fmt.Errorf("failed to pay bounty reward: %w", err)
	}

	bounty.Status = string(types.BountyStatusFulfilled)
	bounty.FulfilledBy = bounty.ClaimedBy
	m.Keeper.SetBounty(ctx, bounty)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.bounty_fulfilled",
			sdk.NewAttribute("bounty_id", msg.BountyId),
			sdk.NewAttribute("fulfilled_by", bounty.ClaimedBy),
			sdk.NewAttribute("reward", bounty.Reward),
		),
	)

	return &types.MsgFulfillBountyResponse{}, nil
}

// FundResearch adds funds to the research treasury.
func (m msgServer) FundResearch(
	goCtx context.Context,
	msg *types.MsgFundResearch,
) (*types.MsgFundResearchResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	amountInt := new(big.Int)
	amountInt.SetString(msg.Amount, 10)

	// Transfer to module account
	funderAddr, _ := sdk.AccAddressFromBech32(msg.Funder)
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amountInt)))
	if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, funderAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to fund research treasury: %w", err)
	}

	// Update explicit treasury balance tracker.
	currentBalance := new(big.Int)
	currentBalance.SetString(m.Keeper.GetTreasuryBalance(ctx), 10)
	newBalance := new(big.Int).Add(currentBalance, amountInt)
	m.Keeper.SetTreasuryBalance(ctx, newBalance.String())

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.research_funded",
			sdk.NewAttribute("funder", msg.Funder),
			sdk.NewAttribute("amount", msg.Amount),
			sdk.NewAttribute("new_treasury_balance", newBalance.String()),
		),
	)

	return &types.MsgFundResearchResponse{}, nil
}

// UpdateParams updates module parameters.
func (m msgServer) UpdateParams(
	goCtx context.Context,
	msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != m.Keeper.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	if msg.Params != nil {
		m.Keeper.SetParams(ctx, msg.Params)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.research.params_updated",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}
