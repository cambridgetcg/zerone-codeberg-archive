package keeper_test

// Liquidity Brief Gap 5: LP Withdrawal Delay (anti-JIT liquidity attack)
//
// MinLiquidityDurationBlocks in Params sets a per-depositor cooldown:
// after AddLiquidity the LP must wait at least N blocks before RemoveLiquidity.
//
// Precedence:
//   param == 0 → cooldown disabled entirely (default, backward compat)
//   deposit not recorded → cooldown does not apply (pre-param positions)
//   param > 0 and recorded → enforced on the depositor
//
// The deposit block is updated (reset) on every AddLiquidity call.

import (
	"testing"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// TestWithdrawal_BeforeCooldown_Blocked verifies that a withdrawal attempted
// before MinLiquidityDurationBlocks have elapsed is rejected.
func TestWithdrawal_BeforeCooldown_Blocked(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MinLiquidityDurationBlocks = 100 // 100 block cooldown
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Deposit at block 1 (default test context)
	sender := "zrn1jit00000000000000000000000"
	bk.setBalance(sender, "uzrn", 10_000_000_000)
	bk.setBalance(sender, "uatom", 10_000_000_000)
	_, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  sender,
		PoolId:  poolId,
		AmountA: "1000000000",
		AmountB: "1000000000",
	})
	if err != nil {
		t.Fatalf("AddLiquidity: %v", err)
	}

	// Attempt withdrawal immediately — should fail (elapsed = 0 < 100)
	_, err = ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   sender,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err == nil {
		t.Fatal("expected ErrWithdrawalTooEarly for withdrawal before cooldown")
	}
}

// TestWithdrawal_AfterCooldown_Allowed verifies that a withdrawal after
// MinLiquidityDurationBlocks have elapsed is permitted.
func TestWithdrawal_AfterCooldown_Allowed(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MinLiquidityDurationBlocks = 10
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	sender := "zrn1after0000000000000000000000"
	bk.setBalance(sender, "uzrn", 10_000_000_000)
	bk.setBalance(sender, "uatom", 10_000_000_000)
	_, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  sender,
		PoolId:  poolId,
		AmountA: "1000000000",
		AmountB: "1000000000",
	})
	if err != nil {
		t.Fatalf("AddLiquidity: %v", err)
	}

	// Advance context to block 11 (deposit was at block 1, cooldown = 10)
	laterCtx := ctx.WithBlockHeight(ctx.BlockHeight() + 10)

	_, err = ms.RemoveLiquidity(laterCtx, &types.MsgRemoveLiquidity{
		Sender:   sender,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err != nil {
		t.Fatalf("expected withdrawal to succeed after cooldown: %v", err)
	}
}

// TestWithdrawal_Cooldown_Disabled verifies that when MinLiquidityDurationBlocks==0
// the check is skipped entirely (backward compatible default).
func TestWithdrawal_Cooldown_Disabled(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MinLiquidityDurationBlocks = 0 // disabled
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	sender := "zrn1disabled00000000000000000000"
	bk.setBalance(sender, "uzrn", 10_000_000_000)
	bk.setBalance(sender, "uatom", 10_000_000_000)
	_, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  sender,
		PoolId:  poolId,
		AmountA: "1000000000",
		AmountB: "1000000000",
	})
	if err != nil {
		t.Fatalf("AddLiquidity: %v", err)
	}

	// Immediate withdrawal — should be allowed when cooldown disabled
	_, err = ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   sender,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err != nil {
		t.Fatalf("expected withdrawal to succeed when cooldown disabled: %v", err)
	}
}

// TestWithdrawal_NoDepositRecord_Allowed verifies that a sender with no recorded
// deposit block (pre-param position, or LP tokens received via transfer) can
// always withdraw regardless of cooldown param. Backward compatible.
func TestWithdrawal_NoDepositRecord_Allowed(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MinLiquidityDurationBlocks = 1000 // long cooldown
	k.SetParams(ctx, params)

	// Create pool with direct authority deposit (which writes deposit record)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Simulate pre-param LP position: inject LP balance without going through AddLiquidity
	legacy := "zrn1legacy0000000000000000000000"
	bk.setBalance(legacy, types.LPDenom(poolId), 50_000_000_000)

	// No deposit record for legacy sender → cooldown does NOT apply
	_, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   legacy,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err != nil {
		t.Fatalf("expected withdrawal for unrecorded position to succeed: %v", err)
	}
}

// TestWithdrawal_DepositResetsCooldown verifies that a second AddLiquidity
// resets the deposit clock, blocking withdrawal until the new cooldown elapses.
func TestWithdrawal_DepositResetsCooldown(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MinLiquidityDurationBlocks = 50
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	sender := "zrn1reset0000000000000000000000"
	bk.setBalance(sender, "uzrn", 20_000_000_000)
	bk.setBalance(sender, "uatom", 20_000_000_000)

	// First deposit at block 1
	_, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  sender,
		PoolId:  poolId,
		AmountA: "500000000",
		AmountB: "500000000",
	})
	if err != nil {
		t.Fatalf("first AddLiquidity: %v", err)
	}

	// Advance to block 40 (cooldown = 50, not yet elapsed from block 1)
	ctx40 := ctx.WithBlockHeight(ctx.BlockHeight() + 39)

	// Second deposit at block 40 — resets clock to block 40
	_, err = ms.AddLiquidity(ctx40, &types.MsgAddLiquidity{
		Sender:  sender,
		PoolId:  poolId,
		AmountA: "500000000",
		AmountB: "500000000",
	})
	if err != nil {
		t.Fatalf("second AddLiquidity: %v", err)
	}

	// At block 60 (20 blocks since second deposit) — still blocked (need 50 since block 40)
	ctx60 := ctx.WithBlockHeight(ctx.BlockHeight() + 59)
	_, err = ms.RemoveLiquidity(ctx60, &types.MsgRemoveLiquidity{
		Sender:   sender,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err == nil {
		t.Fatal("expected ErrWithdrawalTooEarly: clock was reset by second deposit")
	}

	// At block 91 (51 blocks since second deposit at block 40) — now allowed
	ctx91 := ctx.WithBlockHeight(ctx.BlockHeight() + 90)
	_, err = ms.RemoveLiquidity(ctx91, &types.MsgRemoveLiquidity{
		Sender:   sender,
		PoolId:   poolId,
		LpTokens: "1000",
	})
	if err != nil {
		t.Fatalf("expected withdrawal to succeed after clock reset + elapsed: %v", err)
	}
}

// TestGetSetLPDepositBlock_RoundTrip verifies the raw state accessor.
func TestGetSetLPDepositBlock_RoundTrip(t *testing.T) {
	_, k, ctx, _ := setupMsgServer(t)

	// Not recorded initially
	if _, found := k.GetLPDepositBlock(ctx, "pool-1", "zrn1abc"); found {
		t.Fatal("expected no deposit block before any set")
	}

	// Round-trip
	k.SetLPDepositBlock(ctx, "pool-1", "zrn1abc", 12345)
	block, found := k.GetLPDepositBlock(ctx, "pool-1", "zrn1abc")
	if !found {
		t.Fatal("expected deposit block after set")
	}
	if block != 12345 {
		t.Errorf("GetLPDepositBlock: got %d, want 12345", block)
	}

	// Different pool — isolated
	if _, found := k.GetLPDepositBlock(ctx, "pool-2", "zrn1abc"); found {
		t.Fatal("deposit block should not bleed across pools")
	}
}
