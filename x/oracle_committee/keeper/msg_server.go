package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/oracle_committee/types"
)

// MsgServer implements the oracle_committee Msg service.
type MsgServer struct {
	Keeper
}

// NewMsgServer returns a new MsgServer backed by the given Keeper.
func NewMsgServer(k Keeper) *MsgServer {
	return &MsgServer{Keeper: k}
}

// RegisterOracleOperator handles MsgRegisterOracleOperator.
func (s *MsgServer) RegisterOracleOperator(goCtx context.Context, msg *types.MsgRegisterOracleOperator) (*types.MsgRegisterOracleOperatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.Keeper.RegisterOperator(ctx, msg.Operator, msg.Uri, msg.Stake); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_operator_registered",
		sdk.NewAttribute("operator", msg.Operator),
		sdk.NewAttribute("uri", msg.Uri),
		sdk.NewAttribute("stake", msg.Stake),
	))
	return &types.MsgRegisterOracleOperatorResponse{}, nil
}

// SubmitOracleEvaluation handles MsgSubmitOracleEvaluation.
func (s *MsgServer) SubmitOracleEvaluation(goCtx context.Context, msg *types.MsgSubmitOracleEvaluation) (*types.MsgSubmitOracleEvaluationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	agg, quorumReached, err := s.Keeper.SubmitEvaluation(
		ctx,
		msg.Operator,
		msg.ClaimId,
		msg.Verdict,
		msg.ConfidenceBps,
		msg.Reasoning,
		msg.OracleRequestId,
	)
	if err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_evaluation_submitted",
		sdk.NewAttribute("operator", msg.Operator),
		sdk.NewAttribute("claim_id", msg.ClaimId),
		sdk.NewAttribute("verdict", msg.Verdict),
		sdk.NewAttribute("quorum_reached", fmt.Sprintf("%v", quorumReached)),
	))
	if quorumReached && agg != nil {
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"oracle_committee_quorum_reached",
			sdk.NewAttribute("claim_id", msg.ClaimId),
			sdk.NewAttribute("final_verdict", agg.FinalVerdict),
		))
	}
	return &types.MsgSubmitOracleEvaluationResponse{QuorumReached: quorumReached}, nil
}

// DeregisterOracleOperator handles MsgDeregisterOracleOperator.
func (s *MsgServer) DeregisterOracleOperator(goCtx context.Context, msg *types.MsgDeregisterOracleOperator) (*types.MsgDeregisterOracleOperatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.Keeper.DeregisterOperator(ctx, msg.Operator); err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"oracle_operator_deregistered",
		sdk.NewAttribute("operator", msg.Operator),
	))
	return &types.MsgDeregisterOracleOperatorResponse{}, nil
}

// UpdateOracleCommitteeParams handles MsgUpdateOracleCommitteeParams (governance-only).
func (s *MsgServer) UpdateOracleCommitteeParams(goCtx context.Context, msg *types.MsgUpdateOracleCommitteeParams) (*types.MsgUpdateOracleCommitteeParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if msg.Authority != s.Keeper.GetAuthority() {
		return nil, types.ErrUnauthorized
	}
	if msg.Params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}
	if err := s.Keeper.SetParams(ctx, msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateOracleCommitteeParamsResponse{}, nil
}
