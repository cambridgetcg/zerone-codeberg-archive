package keeper_test

// Market Microstructure Gap 4: Per-pool MaxSwapFractionBps
//
// The global Params.MaxSwapFractionBps (Gap 5, existing) sets a network-wide
// ceiling on single-swap size. Gap 4 adds per-pool override: each pool can
// declare its own limit via Pool.PoolMaxSwapFractionBps.
//
// Precedence: pool-level (if > 0) overrides global.
//   pool > 0 → use pool value
//   pool == 0 → use global value
//   both == 0 → check disabled
//
// This lets pools with thin liquidity or volatile pairs enforce tighter caps
// without waiting for governance to change the global parameter.

import (
	"testing"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// TestCreatePool_PerPoolSwapFraction_Stored verifies that PoolMaxSwapFractionBps
// declared at pool creation is stored on the Pool struct.
func TestCreatePool_PerPoolSwapFraction_Stored(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)
	bk.setBalance(testAuthority, "uatom", 1_000_000_000_000)

	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                testAuthority,
		DenomA:                 "uzrn",
		DenomB:                 "uatom",
		AmountA:                "100000000000",
		AmountB:                "100000000000",
		PoolMaxSwapFractionBps: 50_000, // 5% pool-level cap
	})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}

	pool, found := k.GetPool(ctx, resp.PoolId)
	if !found {
		t.Fatal("pool not found after creation")
	}
	if pool.PoolMaxSwapFractionBps != 50_000 {
		t.Errorf("PoolMaxSwapFractionBps: got %d, want 50000", pool.PoolMaxSwapFractionBps)
	}
}

// TestCreatePool_PerPoolSwapFraction_Zero_Allowed verifies that 0 (use global) is
// accepted and stored (backward compatible with existing pool creation).
func TestCreatePool_PerPoolSwapFraction_Zero_Allowed(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	pool, found := k.GetPool(ctx, poolId)
	if !found {
		t.Fatal("pool not found")
	}
	if pool.PoolMaxSwapFractionBps != 0 {
		t.Errorf("default PoolMaxSwapFractionBps should be 0, got %d", pool.PoolMaxSwapFractionBps)
	}
}

// TestCreatePool_PerPoolSwapFraction_InvalidExceedsMax verifies that
// PoolMaxSwapFractionBps > 1_000_000 is rejected at pool creation.
func TestCreatePool_PerPoolSwapFraction_InvalidExceedsMax(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)
	bk.setBalance(testAuthority, "uatom", 1_000_000_000_000)

	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                testAuthority,
		DenomA:                 "uzrn",
		DenomB:                 "uatom",
		AmountA:                "100000000000",
		AmountB:                "100000000000",
		PoolMaxSwapFractionBps: 1_000_001, // > 100% — invalid
	})
	if err == nil {
		t.Fatal("expected ErrInvalidPoolSwapFraction for cap > 1_000_000")
	}
}

// TestSwap_PerPoolFraction_OverridesGlobal verifies that a pool-level cap tighter
// than the global cap is enforced: swap blocked by pool cap even when global allows it.
func TestSwap_PerPoolFraction_OverridesGlobal(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	// Global: 15% cap.
	params := types.DefaultParams()
	params.MaxSwapFractionBps = 150_000 // 15%
	k.SetParams(ctx, params)

	// Per-pool: 5% cap (tighter than global).
	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)
	bk.setBalance(testAuthority, "uatom", 1_000_000_000_000)
	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                testAuthority,
		DenomA:                 "uzrn",
		DenomB:                 "uatom",
		AmountA:                "100000000000",
		AmountB:                "100000000000",
		PoolMaxSwapFractionBps: 50_000, // 5%
	})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	poolId := resp.PoolId

	sender := "zrn1perpool0000000000000000000"
	bk.setBalance(sender, "uzrn", 20_000_000_000)

	// Swap 10% of reserve — allowed by global (15%) but blocked by pool (5%).
	_, err = ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000000000", // 10% of 100_000_000_000
	})
	if err == nil {
		t.Fatal("expected ErrSwapFractionExceeded: pool-level cap (5%) should block 10% swap")
	}
}

// TestSwap_PerPoolFraction_LooserThanGlobal verifies that a pool-level cap looser
// than the global cap still enforces the pool cap, not the global.
func TestSwap_PerPoolFraction_LooserThanGlobal(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	// Global: 5% cap.
	params := types.DefaultParams()
	params.MaxSwapFractionBps = 50_000 // 5%
	k.SetParams(ctx, params)

	// Per-pool: 20% cap (looser than global).
	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)
	bk.setBalance(testAuthority, "uatom", 1_000_000_000_000)
	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator:                testAuthority,
		DenomA:                 "uzrn",
		DenomB:                 "uatom",
		AmountA:                "100000000000",
		AmountB:                "100000000000",
		PoolMaxSwapFractionBps: 200_000, // 20%
	})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	poolId := resp.PoolId

	sender := "zrn1looser00000000000000000000"
	bk.setBalance(sender, "uzrn", 20_000_000_000)

	// Swap 10% of reserve — blocked by global (5%) in the normal case, but pool
	// overrides to 20%, so 10% should be allowed.
	_, err = ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000000000", // 10% of 100_000_000_000
	})
	if err != nil {
		t.Fatalf("expected swap at 10%% to succeed with 20%% per-pool override: %v", err)
	}
}

// TestSwap_PerPoolFraction_Zero_FallsBackToGlobal verifies that a pool with
// PoolMaxSwapFractionBps == 0 uses the global Params.MaxSwapFractionBps.
func TestSwap_PerPoolFraction_Zero_FallsBackToGlobal(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	// Global: 5% cap.
	params := types.DefaultParams()
	params.MaxSwapFractionBps = 50_000 // 5%
	k.SetParams(ctx, params)

	// Pool-level: 0 (use global).
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	sender := "zrn1fallback0000000000000000000"
	bk.setBalance(sender, "uzrn", 20_000_000_000)

	// Swap 10% — should be blocked by global (5%) since pool has no override.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000000000", // 10% of 100_000_000_000
	})
	if err == nil {
		t.Fatal("expected ErrSwapFractionExceeded: global cap (5%) should block 10% swap when pool-level is 0")
	}
}

// TestSwap_PerPoolFraction_BothZero_Disabled verifies that when both pool-level
// and global max_swap_fraction_bps are 0, the check is disabled entirely.
func TestSwap_PerPoolFraction_BothZero_Disabled(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxSwapFractionBps = 0 // global disabled
	params.MinReserve = "1"
	k.SetParams(ctx, params)

	// Pool-level: 0 (use global, which is also 0).
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	sender := "zrn1bothzero0000000000000000000"
	bk.setBalance(sender, "uzrn", 60_000_000_000)

	// Swap 50% — should be allowed when both caps are 0.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "50000000000", // 50% of 100_000_000_000
	})
	if err != nil {
		t.Fatalf("expected swap to succeed when both fraction caps are 0: %v", err)
	}
}
