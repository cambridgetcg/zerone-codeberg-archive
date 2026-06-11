package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// enableFairSequencing sets fair_sequencing_enabled + commit_reveal_delay_blocks > 0.
func enableFairSequencing(t *testing.T, k keeper.Keeper, ctx sdk.Context) {
	t.Helper()
	p := types.DefaultParams()
	p.FairSequencingEnabled = true
	p.CommitRevealDelayBlocks = 1 // need delay > 0 so RevealSwap passes the block check
	k.SetParams(ctx, p)
}

// doCommitRevealQueued commits and reveals a swap with fair sequencing enabled.
// Returns the RevealSwapResponse (should have empty TokenOutAmount since execution is queued).
func doCommitRevealQueued(
	t *testing.T,
	ms types.MsgServer,
	ctx sdk.Context,
	commitCtx sdk.Context,
	sender, poolId, tokenInAmount string,
) *types.MsgRevealSwapResponse {
	t.Helper()
	nonce := []byte("fair-seq-nonce-" + sender)
	hash := keeper.ComputeSwapCommitmentHash(nonce, poolId, "uzrn", tokenInAmount, "", 0)
	if _, err := ms.CommitSwap(commitCtx, &types.MsgCommitSwap{
		Sender:     sender,
		PoolId:     poolId,
		Commitment: hash,
	}); err != nil {
		t.Fatalf("CommitSwap: %v", err)
	}
	// Reveal happens in a later block (ctx has higher block height than commitCtx)
	resp, err := ms.RevealSwap(ctx, &types.MsgRevealSwap{
		Sender:        sender,
		PoolId:        poolId,
		Nonce:         nonce,
		TokenInDenom:  "uzrn",
		TokenInAmount: tokenInAmount,
	})
	if err != nil {
		t.Fatalf("RevealSwap: %v", err)
	}
	return resp
}

// --- Test: queuing ---

// TestFairSequencing_QueuedOnReveal verifies that when fair_sequencing_enabled is
// true, RevealSwap queues a PendingSwapReveal instead of executing the swap.
func TestFairSequencing_QueuedOnReveal(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	enableFairSequencing(t, k, ctx)
	commitCtx := ctx.WithBlockHeader(cmtproto.Header{Height: ctx.BlockHeight() - 1})

	resp := doCommitRevealQueued(t, ms, ctx, commitCtx, testAuthority, poolId, "1000000")

	// Response should be empty (swap not yet executed)
	require.Equal(t, "", resp.TokenOutAmount)

	// PendingSwapReveal should now be stored
	reveals := k.GetAllPendingSwapReveals(ctx)
	require.Len(t, reveals, 1)
	require.Equal(t, testAuthority, reveals[0].Sender)
	require.Equal(t, poolId, reveals[0].PoolId)
	require.Equal(t, "1000000", reveals[0].TokenInAmount)
	require.Equal(t, "uzrn", reveals[0].TokenInDenom)
}

// TestFairSequencing_DisabledExecutesImmediately verifies backward compat: when
// fair_sequencing_enabled is false (default), RevealSwap executes the swap immediately.
func TestFairSequencing_DisabledExecutesImmediately(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")
	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)

	// Default params: fair sequencing disabled
	p := types.DefaultParams()
	p.CommitRevealDelayBlocks = 0
	k.SetParams(ctx, p)

	nonce := []byte("immediate-nonce")
	hash := keeper.ComputeSwapCommitmentHash(nonce, poolId, "uzrn", "1000000", "", 0)
	_, err := ms.CommitSwap(ctx, &types.MsgCommitSwap{
		Sender:     testAuthority,
		PoolId:     poolId,
		Commitment: hash,
	})
	require.NoError(t, err)

	resp, err := ms.RevealSwap(ctx, &types.MsgRevealSwap{
		Sender:        testAuthority,
		PoolId:        poolId,
		Nonce:         nonce,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	require.NoError(t, err)
	// Immediate execution → non-empty TokenOutAmount
	require.NotEmpty(t, resp.TokenOutAmount)
	require.NotEqual(t, "0", resp.TokenOutAmount)

	// No pending reveals
	require.Empty(t, k.GetAllPendingSwapReveals(ctx))
}

// --- Test: FIFO ordering ---

// TestFairSequencing_FIFOOrder verifies that ProcessFairSequencedSwaps processes
// swaps in commit-block-ascending order (earlier commits execute first).
func TestFairSequencing_FIFOOrder(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")
	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)

	enableFairSequencing(t, k, ctx)

	// Inject two PendingSwapReveals directly with different commit blocks.
	// commitBlock=50 should execute before commitBlock=100.
	k.SetPendingSwapReveal(ctx, &types.PendingSwapReveal{
		Sender:        testAuthority,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "2000000",
		CommitBlock:   100,
	})
	k.SetPendingSwapReveal(ctx, &types.PendingSwapReveal{
		Sender:        testAuthority + "early",
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
		CommitBlock:   50,
	})

	bk.setBalance(testAuthority+"early", "uzrn", 1_000_000_000_000)

	reveals := k.GetAllPendingSwapReveals(ctx)
	require.Len(t, reveals, 2)
	// Iteration should return the earlier commit block first
	require.Equal(t, uint64(50), reveals[0].CommitBlock)
	require.Equal(t, uint64(100), reveals[1].CommitBlock)
}

// --- Test: EndBlocker clears queue ---

// TestFairSequencing_EndBlockerClearsQueue verifies that ProcessFairSequencedSwaps
// deletes all queued reveals after processing them.
func TestFairSequencing_EndBlockerClearsQueue(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")
	bk.setBalance(testAuthority, "uzrn", 1_000_000_000_000)

	enableFairSequencing(t, k, ctx)
	commitCtx := ctx.WithBlockHeader(cmtproto.Header{Height: ctx.BlockHeight() - 1})
	doCommitRevealQueued(t, ms, ctx, commitCtx, testAuthority, poolId, "1000000")

	require.Len(t, k.GetAllPendingSwapReveals(ctx), 1)

	k.ProcessFairSequencedSwaps(ctx)

	// Queue should be empty after processing
	require.Empty(t, k.GetAllPendingSwapReveals(ctx))
}

// --- Test: disabled EndBlocker is no-op ---

// TestFairSequencing_EndBlockerNoOpWhenDisabled verifies that ProcessFairSequencedSwaps
// does nothing when fair_sequencing_enabled is false, even if the queue has entries.
func TestFairSequencing_EndBlockerNoOpWhenDisabled(t *testing.T) {
	_, k, ctx, _ := setupMsgServer(t)

	// Inject a record directly
	k.SetPendingSwapReveal(ctx, &types.PendingSwapReveal{
		Sender:        testAuthority,
		PoolId:        "pool-1",
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
		CommitBlock:   10,
	})

	// Default params: fair sequencing disabled
	p := types.DefaultParams()
	p.FairSequencingEnabled = false
	k.SetParams(ctx, p)

	k.ProcessFairSequencedSwaps(ctx)

	// Record should still be there — EndBlocker should not have touched it
	reveals := k.GetAllPendingSwapReveals(ctx)
	require.Len(t, reveals, 1)
}

// --- Test: genesis round-trip ---

// TestFairSequencing_GenesisRoundTrip verifies that PendingSwapReveals are exported
// and re-imported via genesis correctly.
func TestFairSequencing_GenesisRoundTrip(t *testing.T) {
	_, k, ctx, _ := setupMsgServer(t)

	k.SetPendingSwapReveal(ctx, &types.PendingSwapReveal{
		Sender:        testAuthority,
		PoolId:        "pool-1",
		TokenInDenom:  "uzrn",
		TokenInAmount: "5000000",
		CommitBlock:   77,
	})

	gs := k.ExportGenesis(ctx)
	require.Len(t, gs.PendingSwapReveals, 1)
	require.Equal(t, "5000000", gs.PendingSwapReveals[0].TokenInAmount)
	require.Equal(t, uint64(77), gs.PendingSwapReveals[0].CommitBlock)

	// Re-import into a fresh keeper
	_, k2, ctx2, _ := setupMsgServer(t)
	k2.InitGenesis(ctx2, gs)

	restored := k2.GetAllPendingSwapReveals(ctx2)
	require.Len(t, restored, 1)
	require.Equal(t, uint64(77), restored[0].CommitBlock)
	require.Equal(t, "5000000", restored[0].TokenInAmount)
}

// --- Test: Params default ---

// TestFairSequencing_DefaultParamsFalse verifies that fair_sequencing_enabled
// defaults to false (opt-in governance upgrade).
func TestFairSequencing_DefaultParamsFalse(t *testing.T) {
	p := types.DefaultParams()
	require.False(t, p.FairSequencingEnabled)
}
