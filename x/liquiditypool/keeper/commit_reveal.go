package keeper

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// ---------- SwapCommitment CRUD ----------

// SetSwapCommitment stores a swap commitment for (sender, poolId).
// Overwrites any existing commitment for the same pair.
func (k Keeper) SetSwapCommitment(ctx sdk.Context, c *types.SwapCommitment) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal SwapCommitment: %v", err))
	}
	if err := store.Set(types.SwapCommitmentKey(c.Sender, c.PoolId), bz); err != nil {
		panic(fmt.Sprintf("failed to set SwapCommitment: %v", err))
	}
}

// GetSwapCommitment retrieves a pending commitment for (sender, poolId).
// Returns (zero-value, false) if not found.
func (k Keeper) GetSwapCommitment(ctx sdk.Context, sender, poolId string) (*types.SwapCommitment, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.SwapCommitmentKey(sender, poolId))
	if err != nil || bz == nil {
		return nil, false
	}
	var c types.SwapCommitment
	if err := proto.Unmarshal(bz, &c); err != nil {
		return nil, false
	}
	return &c, true
}

// DeleteSwapCommitment removes a commitment for (sender, poolId).
func (k Keeper) DeleteSwapCommitment(ctx sdk.Context, sender, poolId string) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.SwapCommitmentKey(sender, poolId))
}

// IterateSwapCommitments iterates all stored swap commitments.
// Callback returns true to stop iteration.
func (k Keeper) IterateSwapCommitments(ctx sdk.Context, cb func(*types.SwapCommitment) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.SwapCommitmentKeyPrefix, prefixEndBytes(types.SwapCommitmentKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var c types.SwapCommitment
		if err := proto.Unmarshal(iter.Value(), &c); err != nil {
			continue
		}
		if cb(&c) {
			break
		}
	}
}

// ---------- Commitment hash ----------

// ComputeSwapCommitmentHash returns the canonical sha256 preimage hash for the given swap parameters.
//
// Canonical preimage (length-prefixed nonce to avoid collision):
//
//	sha256(uint32_bigendian(len(nonce)) || nonce || "|" || pool_id || "|" ||
//	       token_in_denom || "|" || token_in_amount || "|" ||
//	       min_token_out || "|" || strconv.FormatUint(deadline_block, 10))
func ComputeSwapCommitmentHash(nonce []byte, poolId, tokenInDenom, tokenInAmount, minTokenOut string, deadlineBlock uint64) []byte {
	var buf bytes.Buffer
	// length-prefix nonce (4-byte big-endian) prevents nonce bytes from being
	// confused with the "|" separator that follows.
	lenPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lenPrefix, uint32(len(nonce)))
	buf.Write(lenPrefix)
	buf.Write(nonce)
	buf.WriteByte('|')
	buf.WriteString(poolId)
	buf.WriteByte('|')
	buf.WriteString(tokenInDenom)
	buf.WriteByte('|')
	buf.WriteString(tokenInAmount)
	buf.WriteByte('|')
	buf.WriteString(minTokenOut)
	buf.WriteByte('|')
	buf.WriteString(strconv.FormatUint(deadlineBlock, 10))
	h := sha256.Sum256(buf.Bytes())
	return h[:]
}
