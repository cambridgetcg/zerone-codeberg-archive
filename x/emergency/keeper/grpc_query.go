package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/zerone-chain/zerone/x/emergency/types"
)

type queryServer struct {
	types.UnimplementedQueryServer
	Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{Keeper: keeper}
}

var _ types.QueryServer = queryServer{}

// Status returns the current emergency status.
func (q queryServer) Status(goCtx context.Context, _ *types.QueryStatusRequest) (*types.QueryStatusResponse, error) {
	return &types.QueryStatusResponse{
		Status:               string(q.GetEmergencyStatus(goCtx)),
		IsHalted:             q.IsHalted(goCtx),
		ActiveHaltCeremonyId: q.GetActiveHaltCeremonyId(goCtx),
	}, nil
}

// ActiveCeremony returns the currently active ceremony, if any.
func (q queryServer) ActiveCeremony(goCtx context.Context, _ *types.QueryActiveCeremonyRequest) (*types.QueryActiveCeremonyResponse, error) {
	ceremony, found := q.GetActiveCeremony(goCtx)
	if !found {
		return &types.QueryActiveCeremonyResponse{Found: false}, nil
	}
	return &types.QueryActiveCeremonyResponse{
		Ceremony: ceremony,
		Found:    true,
	}, nil
}

// CompletedCeremonies returns completed (finalized/failed) ceremonies.
func (q queryServer) CompletedCeremonies(goCtx context.Context, req *types.QueryCompletedCeremoniesRequest) (*types.QueryCompletedCeremoniesResponse, error) {
	limit := req.Limit
	if limit == 0 || limit > 100 {
		limit = 100
	}

	var completed []*types.EmergencyCeremony
	var total uint64

	q.IterateCeremonies(goCtx, func(c *types.EmergencyCeremony) bool {
		if c.Phase == string(types.PhaseFinalized) || c.Phase == string(types.PhaseFailed) {
			total++
			if total > uint64(req.Offset) && uint32(len(completed)) < limit {
				completed = append(completed, c)
			}
		}
		return false
	})

	return &types.QueryCompletedCeremoniesResponse{
		Ceremonies: completed,
		Total:      total,
	}, nil
}

// AuditLog returns the emergency audit log.
func (q queryServer) AuditLog(goCtx context.Context, req *types.QueryAuditLogRequest) (*types.QueryAuditLogResponse, error) {
	limit := req.Limit
	if limit == 0 || limit > 100 {
		limit = 100
	}

	entries := q.GetAuditLog(goCtx)
	total := uint64(len(entries))

	start := uint32(0)
	if req.Offset < uint32(len(entries)) {
		start = req.Offset
	} else {
		start = uint32(len(entries))
	}
	end := start + limit
	if end > uint32(len(entries)) {
		end = uint32(len(entries))
	}

	return &types.QueryAuditLogResponse{
		Entries: entries[start:end],
		Total:   total,
	}, nil
}

// Params returns the emergency module parameters.
func (q queryServer) Params(goCtx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params := q.GetParams(goCtx)
	return &types.QueryParamsResponse{Params: params}, nil
}

// ProtocolHealth returns a leading-indicator health snapshot for monitoring
// dashboards (Circuit Breakers Gap 3). All fields sourced from emergency module
// state — no cross-module queries required.
//
// Planned extensions: active_rounds_ratio (knowledge), slash_rate (staking).
func (q queryServer) ProtocolHealth(goCtx context.Context, _ *types.QueryProtocolHealthRequest) (*types.QueryProtocolHealthResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentBlock := uint64(ctx.BlockHeight())
	params := q.GetParams(goCtx)

	// Oracle freshness — from P-21b LastOracleUpdateBlockKey.
	lastOracleBlock := q.GetLastOracleUpdateBlock(goCtx)
	var freshness uint64
	if lastOracleBlock > 0 && currentBlock >= lastOracleBlock {
		freshness = currentBlock - lastOracleBlock
	}

	threshold := params.CircuitOracleStalenessBlocks
	isStale := threshold > 0 && lastOracleBlock > 0 && freshness > threshold

	// Halt duration — from HaltStartBlockKey.
	haltStart := q.GetHaltStartBlock(goCtx)
	var blocksSinceHalt uint64
	if haltStart > 0 && q.IsHalted(goCtx) && currentBlock >= haltStart {
		blocksSinceHalt = currentBlock - haltStart
	}

	return &types.QueryProtocolHealthResponse{
		EmergencyStatus:          string(q.GetEmergencyStatus(goCtx)),
		OracleFreshnessBlocks:    freshness,
		OracleStalenessThreshold: threshold,
		IsOracleStale:            isStale,
		HaltStartBlock:           haltStart,
		BlocksSinceHalt:          blocksSinceHalt,
		MaxHaltDurationBlocks:    params.MaxHaltDurationBlocks,
		CircuitBreakersEnabled:   threshold > 0, // true iff oracle staleness circuit breaker is active
		SnapshotBlockHeight:      currentBlock,
	}, nil
}
