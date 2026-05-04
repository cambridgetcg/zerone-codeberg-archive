package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/oracle_committee/types"
)

// QueryServer implements the oracle_committee Query service.
type QueryServer struct {
	Keeper
}

// NewQueryServer returns a new QueryServer backed by the given Keeper.
func NewQueryServer(k Keeper) *QueryServer {
	return &QueryServer{Keeper: k}
}

// OracleOperators returns all registered oracle operators.
func (q *QueryServer) OracleOperators(goCtx context.Context, _ *types.QueryOracleOperatorsRequest) (*types.QueryOracleOperatorsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryOracleOperatorsResponse{Operators: q.Keeper.GetAllOperators(ctx)}, nil
}

// OracleOperator returns a single oracle operator by address.
func (q *QueryServer) OracleOperator(goCtx context.Context, req *types.QueryOracleOperatorRequest) (*types.QueryOracleOperatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	op, found := q.Keeper.GetOperator(ctx, req.Address)
	if !found {
		return nil, types.ErrOperatorNotFound
	}
	return &types.QueryOracleOperatorResponse{Operator: op}, nil
}

// OracleEvaluations returns all evaluations for a given claim.
func (q *QueryServer) OracleEvaluations(goCtx context.Context, req *types.QueryOracleEvaluationsRequest) (*types.QueryOracleEvaluationsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	evals := q.Keeper.GetEvaluationsForClaim(ctx, req.ClaimId)
	return &types.QueryOracleEvaluationsResponse{Evaluations: evals}, nil
}

// OracleAggregation returns the finalized aggregation for a given claim.
func (q *QueryServer) OracleAggregation(goCtx context.Context, req *types.QueryOracleAggregationRequest) (*types.QueryOracleAggregationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	agg, found := q.Keeper.GetAggregation(ctx, req.ClaimId)
	if !found {
		return nil, types.ErrOperatorNotFound // reuse; "aggregation not found"
	}
	return &types.QueryOracleAggregationResponse{Aggregation: agg}, nil
}

// Params returns the current oracle committee params.
func (q *QueryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return &types.QueryParamsResponse{Params: q.Keeper.GetParams(ctx)}, nil
}
