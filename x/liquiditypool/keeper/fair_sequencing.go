package keeper

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// ---------- PendingSwapReveal CRUD ----------

// SetPendingSwapReveal stores a PendingSwapReveal for fair-sequencing.
// Key encodes commitBlock as big-endian uint64 so iteration is FIFO by commit block.
func (k Keeper) SetPendingSwapReveal(ctx sdk.Context, r *types.PendingSwapReveal) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(r)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal PendingSwapReveal: %v", err))
	}
	key := types.PendingSwapRevealKey(r.CommitBlock, r.Sender, r.PoolId)
	if err := store.Set(key, bz); err != nil {
		panic(fmt.Sprintf("failed to set PendingSwapReveal: %v", err))
	}
}

// DeletePendingSwapReveal removes a queued swap reveal.
func (k Keeper) DeletePendingSwapReveal(ctx sdk.Context, commitBlock uint64, sender, poolId string) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.PendingSwapRevealKey(commitBlock, sender, poolId))
}

// IteratePendingSwapReveals iterates all queued reveals in FIFO (commit block ascending) order.
// Callback receives the raw key (for deletion) and the reveal. Return true to stop.
func (k Keeper) IteratePendingSwapReveals(ctx sdk.Context, cb func(r *types.PendingSwapReveal) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(
		types.PendingSwapRevealKeyPrefix,
		prefixEndBytes(types.PendingSwapRevealKeyPrefix),
	)
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var r types.PendingSwapReveal
		if err := proto.Unmarshal(iter.Value(), &r); err != nil {
			continue
		}
		if cb(&r) {
			break
		}
	}
}

// GetAllPendingSwapReveals returns all queued swap reveals (for genesis export).
func (k Keeper) GetAllPendingSwapReveals(ctx sdk.Context) []*types.PendingSwapReveal {
	var out []*types.PendingSwapReveal
	k.IteratePendingSwapReveals(ctx, func(r *types.PendingSwapReveal) bool {
		cp := r
		out = append(out, cp)
		return false
	})
	return out
}

// ---------- EndBlocker processor ----------

// ProcessFairSequencedSwaps executes all queued PendingSwapReveal records in
// commit-block-ascending (FIFO) order.  Called from module EndBlock when
// Params.FairSequencingEnabled is true.
//
// Failed swaps (slippage, expired deadline, pool locked, etc.) are silently
// discarded — the commitment was already consumed at RevealSwap time.
func (k Keeper) ProcessFairSequencedSwaps(ctx context.Context) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(sdkCtx)
	if !params.FairSequencingEnabled {
		return
	}

	// Collect all pending reveals in FIFO order.
	var reveals []*types.PendingSwapReveal
	k.IteratePendingSwapReveals(sdkCtx, func(r *types.PendingSwapReveal) bool {
		cp := *r
		reveals = append(reveals, &cp)
		return false
	})

	ms := msgServer{Keeper: k}
	for _, r := range reveals {
		// Delete before execution — prevents replay even if execution panics/errors.
		k.DeletePendingSwapReveal(sdkCtx, r.CommitBlock, r.Sender, r.PoolId)

		// Execute the underlying swap. Errors are discarded — the user's commitment
		// was consumed; they must re-commit if they want to retry.
		_, _ = ms.Swap(ctx, &types.MsgSwap{
			Sender:          r.Sender,
			PoolId:          r.PoolId,
			TokenInDenom:    r.TokenInDenom,
			TokenInAmount:   r.TokenInAmount,
			MinTokenOut:     r.MinTokenOut,
			DeadlineBlock:   r.DeadlineBlock,
		})

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.liquiditypool.fair_sequenced_swap_executed",
			sdk.NewAttribute("sender", r.Sender),
			sdk.NewAttribute("pool_id", r.PoolId),
			sdk.NewAttribute("commit_block", fmt.Sprintf("%d", r.CommitBlock)),
		))
	}
}
