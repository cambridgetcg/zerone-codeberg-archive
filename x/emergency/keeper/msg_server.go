package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/emergency/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// ProposeHalt handles a Guardian's proposal to halt the chain.
func (k msgServer) ProposeHalt(goCtx context.Context, msg *types.MsgProposeHalt) (*types.MsgProposeHaltResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := k.GetParams(goCtx)

	if !k.IsGuardian(goCtx, msg.Proposer) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Proposer)
	}

	status := k.GetEmergencyStatus(goCtx)
	if status != types.StatusNormal {
		return nil, fmt.Errorf("%w: current status is %s, must be normal", types.ErrStatusConflict, status)
	}

	if _, found := k.GetActiveCeremony(goCtx); found {
		return nil, fmt.Errorf("%w", types.ErrCeremonyActive)
	}

	// Guardian stake floor (skip if council active — H-5).
	if !k.isCouncilActive(goCtx, params) {
		guardianStake := k.GetGuardianStake(goCtx)
		minStake, ok := new(big.Int).SetString(params.MinGuardianStake, 10)
		if ok && guardianStake.Cmp(minStake) < 0 {
			return nil, fmt.Errorf("%w: total guardian stake %s < minimum %s (commitment 10: emergency machinery requires plural backing — a single guardian cannot halt the chain, the audit trail demands witnesses)", types.ErrInsufficientGuardians, guardianStake.String(), params.MinGuardianStake)
		}
	}

	// Per-guardian limit.
	if k.GetGuardianProposalCount(goCtx, msg.Proposer) >= params.MaxProposalsPerGuardianPerEpoch {
		return nil, fmt.Errorf("%w: guardian %s already proposed this epoch", types.ErrProposalLimitExceeded, msg.Proposer)
	}

	// Global limit.
	if k.GetEpochProposalCount(goCtx) >= params.MaxProposalsPerEpoch {
		return nil, fmt.Errorf("%w: epoch proposal limit reached", types.ErrProposalLimitExceeded)
	}

	// Cooldown.
	currentBlock := uint64(ctx.BlockHeight())
	lastBlock := k.GetLastProposalBlock(goCtx)
	if lastBlock > 0 && (currentBlock-lastBlock) < params.CooldownBlocks {
		return nil, fmt.Errorf("%w: %d blocks remaining", types.ErrCooldownActive, params.CooldownBlocks-(currentBlock-lastBlock))
	}

	proposalId := fmt.Sprintf("halt-%d-%s", ctx.BlockHeight(), msg.Proposer)
	proposal := types.EmergencyHaltProposal{
		Id:              proposalId,
		Proposer:        msg.Proposer,
		Reason:          msg.Reason,
		ProposedAtBlock: currentBlock,
	}

	ceremony, err := k.CreateHaltCeremony(goCtx, &proposal)
	if err != nil {
		return nil, err
	}

	k.SetEmergencyStatus(goCtx, types.StatusHaltVoting)
	k.IncrementGuardianProposalCount(goCtx, msg.Proposer)
	k.IncrementEpochProposalCount(goCtx)
	k.SetLastProposalBlock(goCtx, currentBlock)

	k.AddAuditEntry(goCtx, &types.EmergencyAuditEntry{
		Timestamp:   ctx.BlockTime().Unix(),
		BlockNumber: currentBlock,
		Action:      string(types.AuditHaltProposed),
		Actor:       msg.Proposer,
		CeremonyId:  ceremony.Id,
		Details:     msg.Reason,
	})

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.halt_proposed",
			sdk.NewAttribute("ceremony_id", ceremony.Id),
			sdk.NewAttribute("proposer", msg.Proposer),
			sdk.NewAttribute("reason", msg.Reason),
		),
	)

	return &types.MsgProposeHaltResponse{ProposalId: ceremony.Id}, nil
}

// VoteHalt handles a Guardian's vote on a halt ceremony.
func (k msgServer) VoteHalt(goCtx context.Context, msg *types.MsgVoteHalt) (*types.MsgVoteHaltResponse, error) {
	if !k.IsGuardian(goCtx, msg.Voter) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Voter)
	}

	ceremony, found := k.GetCeremony(goCtx, msg.ProposalId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrNoCeremony, msg.ProposalId)
	}

	if ceremony.Type != string(types.CeremonyHalt) {
		return nil, fmt.Errorf("%w: ceremony %s is not a halt ceremony", types.ErrInvalidPhase, msg.ProposalId)
	}

	if ceremony.Phase == string(types.PhasePrecommit) {
		err := k.AddPrecommit(goCtx, msg.ProposalId, &types.EmergencyPrecommit{Voter: msg.Voter})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditHaltPrecommit, msg.Voter, msg.ProposalId)
	} else {
		err := k.AddPrevote(goCtx, msg.ProposalId, &types.EmergencyVote{Voter: msg.Voter, Approve: msg.Approve})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditHaltPrevote, msg.Voter, msg.ProposalId)
	}

	finalized, _ := k.CheckCeremonyProgress(goCtx, msg.ProposalId)
	ceremony, _ = k.GetCeremony(goCtx, msg.ProposalId)

	if finalized {
		k.HandleCeremonyFinalization(goCtx, msg.ProposalId)
	} else if ceremony.Phase == string(types.PhaseFailed) {
		k.HandleCeremonyFailure(goCtx, msg.ProposalId)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.vote_halt",
			sdk.NewAttribute("ceremony_id", msg.ProposalId),
			sdk.NewAttribute("voter", msg.Voter),
			sdk.NewAttribute("approve", fmt.Sprintf("%v", msg.Approve)),
		),
	)

	return &types.MsgVoteHaltResponse{
		QuorumReached: ceremony.Phase == string(types.PhasePrecommit) || ceremony.Phase == string(types.PhaseFinalized),
		ChainHalted:   finalized,
	}, nil
}

// ProposeRevert handles a Guardian's proposal to revert chain state.
func (k msgServer) ProposeRevert(goCtx context.Context, msg *types.MsgProposeRevert) (*types.MsgProposeRevertResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := k.GetParams(goCtx)

	if !k.IsGuardian(goCtx, msg.Proposer) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Proposer)
	}

	status := k.GetEmergencyStatus(goCtx)
	if status != types.StatusHalted {
		return nil, fmt.Errorf("%w: chain must be halted to propose revert", types.ErrHaltRequired)
	}

	if _, found := k.GetActiveCeremony(goCtx); found {
		return nil, fmt.Errorf("%w", types.ErrCeremonyActive)
	}

	currentBlock := uint64(ctx.BlockHeight())
	if currentBlock > msg.RevertToHeight && (currentBlock-msg.RevertToHeight) > params.MaxRevertDepth {
		return nil, fmt.Errorf("%w: depth %d exceeds max %d", types.ErrRevertDepthExceeded, currentBlock-msg.RevertToHeight, params.MaxRevertDepth)
	}

	proposalId := fmt.Sprintf("revert-%d-%s", ctx.BlockHeight(), msg.Proposer)
	proposal := types.EmergencyRevertProposal{
		Id:                proposalId,
		Proposer:          msg.Proposer,
		TargetBlockNumber: msg.RevertToHeight,
		Justification:     msg.Justification,
	}

	ceremony, err := k.CreateRevertCeremony(goCtx, &proposal)
	if err != nil {
		return nil, err
	}

	k.SetEmergencyStatus(goCtx, types.StatusRevertVoting)

	k.AddAuditEntry(goCtx, &types.EmergencyAuditEntry{
		Timestamp:   ctx.BlockTime().Unix(),
		BlockNumber: currentBlock,
		Action:      string(types.AuditRevertProposed),
		Actor:       msg.Proposer,
		CeremonyId:  ceremony.Id,
		Details:     fmt.Sprintf("revert to block %d", msg.RevertToHeight),
	})

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.revert_proposed",
			sdk.NewAttribute("ceremony_id", ceremony.Id),
			sdk.NewAttribute("proposer", msg.Proposer),
			sdk.NewAttribute("target_block", fmt.Sprintf("%d", msg.RevertToHeight)),
		),
	)

	return &types.MsgProposeRevertResponse{ProposalId: ceremony.Id}, nil
}

// VoteRevert handles a Guardian's vote on a revert ceremony.
func (k msgServer) VoteRevert(goCtx context.Context, msg *types.MsgVoteRevert) (*types.MsgVoteRevertResponse, error) {
	if !k.IsGuardian(goCtx, msg.Voter) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Voter)
	}

	ceremony, found := k.GetCeremony(goCtx, msg.ProposalId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrNoCeremony, msg.ProposalId)
	}

	if ceremony.Type != string(types.CeremonyRevert) {
		return nil, fmt.Errorf("%w: ceremony %s is not a revert ceremony", types.ErrInvalidPhase, msg.ProposalId)
	}

	if ceremony.Phase == string(types.PhasePrecommit) {
		err := k.AddPrecommit(goCtx, msg.ProposalId, &types.EmergencyPrecommit{Voter: msg.Voter})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditRevertPrecommit, msg.Voter, msg.ProposalId)
	} else {
		err := k.AddPrevote(goCtx, msg.ProposalId, &types.EmergencyVote{Voter: msg.Voter, Approve: msg.Approve})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditRevertPrevote, msg.Voter, msg.ProposalId)
	}

	finalized, _ := k.CheckCeremonyProgress(goCtx, msg.ProposalId)
	ceremony, _ = k.GetCeremony(goCtx, msg.ProposalId)

	if finalized {
		k.HandleCeremonyFinalization(goCtx, msg.ProposalId)
	} else if ceremony.Phase == string(types.PhaseFailed) {
		k.HandleCeremonyFailure(goCtx, msg.ProposalId)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.vote_revert",
			sdk.NewAttribute("ceremony_id", msg.ProposalId),
			sdk.NewAttribute("voter", msg.Voter),
			sdk.NewAttribute("approve", fmt.Sprintf("%v", msg.Approve)),
		),
	)

	return &types.MsgVoteRevertResponse{
		QuorumReached:  ceremony.Phase == string(types.PhasePrecommit) || ceremony.Phase == string(types.PhaseFinalized),
		RevertExecuted: finalized,
	}, nil
}

// ProposeResume handles a Guardian's proposal to resume chain operations.
func (k msgServer) ProposeResume(goCtx context.Context, msg *types.MsgProposeResume) (*types.MsgProposeResumeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !k.IsGuardian(goCtx, msg.Proposer) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Proposer)
	}

	status := k.GetEmergencyStatus(goCtx)
	if status != types.StatusHalted {
		return nil, fmt.Errorf("%w: chain must be halted to propose resume", types.ErrHaltRequired)
	}

	if _, found := k.GetActiveCeremony(goCtx); found {
		return nil, fmt.Errorf("%w", types.ErrCeremonyActive)
	}

	proposalId := fmt.Sprintf("resume-%d-%s", ctx.BlockHeight(), msg.Proposer)
	proposal := types.EmergencyResumeProposal{
		Id:       proposalId,
		Proposer: msg.Proposer,
	}

	ceremony, err := k.CreateResumeCeremony(goCtx, &proposal)
	if err != nil {
		return nil, err
	}

	k.SetEmergencyStatus(goCtx, types.StatusResumeVoting)

	k.AddAuditEntry(goCtx, &types.EmergencyAuditEntry{
		Timestamp:   ctx.BlockTime().Unix(),
		BlockNumber: uint64(ctx.BlockHeight()),
		Action:      string(types.AuditResumeProposed),
		Actor:       msg.Proposer,
		CeremonyId:  ceremony.Id,
		Details:     "resume proposed",
	})

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.resume_proposed",
			sdk.NewAttribute("ceremony_id", ceremony.Id),
			sdk.NewAttribute("proposer", msg.Proposer),
		),
	)

	return &types.MsgProposeResumeResponse{ProposalId: ceremony.Id}, nil
}

// VoteResume handles a Guardian's vote on a resume ceremony.
func (k msgServer) VoteResume(goCtx context.Context, msg *types.MsgVoteResume) (*types.MsgVoteResumeResponse, error) {
	if !k.IsGuardian(goCtx, msg.Voter) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Voter)
	}

	ceremony, found := k.GetCeremony(goCtx, msg.ProposalId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrNoCeremony, msg.ProposalId)
	}

	if ceremony.Type != string(types.CeremonyResume) {
		return nil, fmt.Errorf("%w: ceremony %s is not a resume ceremony", types.ErrInvalidPhase, msg.ProposalId)
	}

	if ceremony.Phase == string(types.PhasePrecommit) {
		err := k.AddPrecommit(goCtx, msg.ProposalId, &types.EmergencyPrecommit{Voter: msg.Voter})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditResumePrecommit, msg.Voter, msg.ProposalId)
	} else {
		err := k.AddPrevote(goCtx, msg.ProposalId, &types.EmergencyVote{Voter: msg.Voter, Approve: msg.Approve})
		if err != nil {
			return nil, err
		}
		k.addVoteAudit(goCtx, types.AuditResumePrevote, msg.Voter, msg.ProposalId)
	}

	finalized, _ := k.CheckCeremonyProgress(goCtx, msg.ProposalId)
	ceremony, _ = k.GetCeremony(goCtx, msg.ProposalId)

	if finalized {
		k.HandleCeremonyFinalization(goCtx, msg.ProposalId)
	} else if ceremony.Phase == string(types.PhaseFailed) {
		k.HandleCeremonyFailure(goCtx, msg.ProposalId)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.vote_resume",
			sdk.NewAttribute("ceremony_id", msg.ProposalId),
			sdk.NewAttribute("voter", msg.Voter),
			sdk.NewAttribute("approve", fmt.Sprintf("%v", msg.Approve)),
		),
	)

	return &types.MsgVoteResumeResponse{
		QuorumReached: ceremony.Phase == string(types.PhasePrecommit) || ceremony.Phase == string(types.PhaseFinalized),
		ChainResumed:  finalized,
	}, nil
}

// UpdateParams handles MsgUpdateParams — governance-gated parameter update.
func (k msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if k.GetAuthority() != msg.Authority {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", k.GetAuthority(), msg.Authority)
	}

	if msg.Params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	k.SetParams(goCtx, msg.Params)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.emergency.params_updated",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}

// ─── Circuit Breaker Gap 5: Surgical State Recovery ──────────────────────────

// ProposeStateCorrection creates a surgical state-correction proposal.
// Proposer must be a Guardian-tier validator.
func (k msgServer) ProposeStateCorrection(goCtx context.Context, msg *types.MsgProposeStateCorrection) (*types.MsgProposeStateCorrectionResponse, error) {
	if !k.IsGuardian(goCtx, msg.Proposer) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Proposer)
	}

	id, err := k.Keeper.ProposeStateCorrection(goCtx, msg.Proposer, msg.Mutations, msg.Rationale)
	if err != nil {
		return nil, err
	}
	return &types.MsgProposeStateCorrectionResponse{ProposalId: id}, nil
}

// VoteStateCorrection records a Guardian vote on a state-correction proposal.
// Approvals >= HaltQuorum fraction of active guardians triggers approval.
func (k msgServer) VoteStateCorrection(goCtx context.Context, msg *types.MsgVoteStateCorrection) (*types.MsgVoteStateCorrectionResponse, error) {
	if !k.IsGuardian(goCtx, msg.Voter) {
		return nil, fmt.Errorf("%w: %s", types.ErrNotGuardian, msg.Voter)
	}

	quorumReached, approved, err := k.Keeper.VoteStateCorrection(goCtx, msg.Voter, msg.ProposalId, msg.Approve)
	if err != nil {
		return nil, err
	}
	return &types.MsgVoteStateCorrectionResponse{
		QuorumReached:      quorumReached,
		CorrectionApproved: approved,
	}, nil
}

// --- Helpers ---

func (k msgServer) addVoteAudit(ctx context.Context, action types.AuditAction, voter, ceremonyId string) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.AddAuditEntry(ctx, &types.EmergencyAuditEntry{
		Timestamp:   sdkCtx.BlockTime().Unix(),
		BlockNumber: uint64(sdkCtx.BlockHeight()),
		Action:      string(action),
		Actor:       voter,
		CeremonyId:  ceremonyId,
	})
}
