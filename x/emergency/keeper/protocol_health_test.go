package keeper_test

// Circuit Breakers Gap 3: ProtocolHealth query (leading indicator monitoring)
//
// QueryProtocolHealth returns a health snapshot for dashboards and
// auto-halt condition evaluation. All fields sourced from emergency module
// state — no cross-module queries.
//
// Metrics:
//   emergency_status     — current chain status
//   oracle_freshness_blocks — blocks since last TWAP update
//   oracle_staleness_threshold — circuit_oracle_staleness_blocks param
//   is_oracle_stale      — freshness > threshold (when both > 0)
//   halt_start_block     — block when halt began (0 if not halted)
//   blocks_since_halt    — elapsed since halt (0 if not halted)
//   max_halt_duration_blocks — governance halt cap
//   circuit_breakers_enabled — oracle circuit breaker active
//   snapshot_block_height — current block

import (
	"testing"

	"github.com/zerone-chain/zerone/x/emergency/keeper"
	"github.com/zerone-chain/zerone/x/emergency/types"
)

// TestProtocolHealth_NormalOperation verifies the health snapshot under
// normal conditions: no halt, no oracle staleness, circuit breakers disabled.
func TestProtocolHealth_NormalOperation(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if resp.EmergencyStatus != string(types.StatusNormal) {
		t.Errorf("EmergencyStatus: got %q, want %q", resp.EmergencyStatus, types.StatusNormal)
	}
	if resp.IsOracleStale {
		t.Error("IsOracleStale should be false when threshold = 0 (disabled)")
	}
	if resp.CircuitBreakersEnabled {
		t.Error("CircuitBreakersEnabled should be false when threshold = 0")
	}
	if resp.HaltStartBlock != 0 {
		t.Errorf("HaltStartBlock should be 0 when not halted, got %d", resp.HaltStartBlock)
	}
	if resp.BlocksSinceHalt != 0 {
		t.Errorf("BlocksSinceHalt should be 0 when not halted, got %d", resp.BlocksSinceHalt)
	}
	if resp.SnapshotBlockHeight == 0 {
		t.Error("SnapshotBlockHeight should be non-zero")
	}
}

// TestProtocolHealth_OracleFreshness verifies that oracle freshness is
// computed from LastOracleUpdateBlock.
func TestProtocolHealth_OracleFreshness(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	// Record oracle update at block 50; context is at block 100.
	ctx50 := ctx.WithBlockHeight(50)
	k.RecordOracleUpdate(ctx50)

	// Query at block 100 — freshness = 100 - 50 = 50
	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if resp.OracleFreshnessBlocks != 50 {
		t.Errorf("OracleFreshnessBlocks: got %d, want 50", resp.OracleFreshnessBlocks)
	}
}

// TestProtocolHealth_OracleStale verifies is_oracle_stale when freshness
// exceeds the staleness threshold.
func TestProtocolHealth_OracleStale(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	// Set threshold = 30 blocks
	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 30
	k.SetParams(ctx, params)

	// Oracle last updated at block 50; current block = 100; freshness = 50 > 30 → stale
	ctx50 := ctx.WithBlockHeight(50)
	k.RecordOracleUpdate(ctx50)

	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if !resp.IsOracleStale {
		t.Error("expected IsOracleStale=true when freshness > threshold")
	}
	if !resp.CircuitBreakersEnabled {
		t.Error("CircuitBreakersEnabled should be true when threshold > 0")
	}
	if resp.OracleStalenessThreshold != 30 {
		t.Errorf("OracleStalenessThreshold: got %d, want 30", resp.OracleStalenessThreshold)
	}
}

// TestProtocolHealth_OracleFresh_NotStale verifies is_oracle_stale=false when
// freshness is within the threshold.
func TestProtocolHealth_OracleFresh_NotStale(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 100
	k.SetParams(ctx, params)

	// Oracle updated at block 90; current = 100; freshness = 10 < 100 → fresh
	ctx90 := ctx.WithBlockHeight(90)
	k.RecordOracleUpdate(ctx90)

	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if resp.IsOracleStale {
		t.Errorf("expected IsOracleStale=false: freshness=%d threshold=%d",
			resp.OracleFreshnessBlocks, resp.OracleStalenessThreshold)
	}
}

// TestProtocolHealth_WhileHalted verifies that halt_start_block and
// blocks_since_halt are populated when the chain is halted.
func TestProtocolHealth_WhileHalted(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	// Manually set halted state
	k.SetEmergencyStatus(ctx, types.StatusHalted)
	k.SetHaltStartBlock(ctx, 80) // halted since block 80; current = 100

	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if resp.EmergencyStatus != string(types.StatusHalted) {
		t.Errorf("EmergencyStatus: got %q, want %q", resp.EmergencyStatus, types.StatusHalted)
	}
	if resp.HaltStartBlock != 80 {
		t.Errorf("HaltStartBlock: got %d, want 80", resp.HaltStartBlock)
	}
	if resp.BlocksSinceHalt != 20 {
		t.Errorf("BlocksSinceHalt: got %d, want 20", resp.BlocksSinceHalt)
	}
}

// TestProtocolHealth_MaxHaltDuration verifies that max_halt_duration_blocks
// is surfaced from params.
func TestProtocolHealth_MaxHaltDuration(t *testing.T) {
	k, _, ctx := setupKeeper(t)
	querySvr := keeper.NewQueryServerImpl(k)

	params := k.GetParams(ctx)
	params.MaxHaltDurationBlocks = 500
	k.SetParams(ctx, params)

	resp, err := querySvr.ProtocolHealth(ctx, &types.QueryProtocolHealthRequest{})
	if err != nil {
		t.Fatalf("ProtocolHealth: %v", err)
	}

	if resp.MaxHaltDurationBlocks != 500 {
		t.Errorf("MaxHaltDurationBlocks: got %d, want 500", resp.MaxHaltDurationBlocks)
	}
}
