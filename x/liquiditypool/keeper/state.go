package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// --- Pool CRUD ---

func (k Keeper) SetPool(ctx sdk.Context, pool *types.Pool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(pool)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal pool: %v", err))
	}
	if err := store.Set(types.PoolKey(pool.PoolId), bz); err != nil {
		panic(fmt.Sprintf("failed to set pool: %v", err))
	}
	// Update denom pair index
	indexKey := types.DenomPairKey(pool.DenomA, pool.DenomB)
	if err := store.Set(indexKey, []byte(pool.PoolId)); err != nil {
		panic(fmt.Sprintf("failed to set denom index: %v", err))
	}
}

func (k Keeper) GetPool(ctx sdk.Context, poolId string) (*types.Pool, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.PoolKey(poolId))
	if err != nil || bz == nil {
		return nil, false
	}
	var pool types.Pool
	if err := proto.Unmarshal(bz, &pool); err != nil {
		return nil, false
	}
	return &pool, true
}

func (k Keeper) GetPoolByDenoms(ctx sdk.Context, denomA, denomB string) *types.Pool {
	store := k.storeService.OpenKVStore(ctx)
	indexKey := types.DenomPairKey(denomA, denomB)
	bz, err := store.Get(indexKey)
	if err != nil || bz == nil {
		return nil
	}
	poolId := string(bz)
	pool, found := k.GetPool(ctx, poolId)
	if !found {
		return nil
	}
	return pool
}

func (k Keeper) DeletePool(ctx sdk.Context, pool *types.Pool) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.PoolKey(pool.PoolId))
	_ = store.Delete(types.DenomPairKey(pool.DenomA, pool.DenomB))
}

func (k Keeper) IteratePools(ctx sdk.Context, cb func(*types.Pool) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.PoolKeyPrefix, prefixEndBytes(types.PoolKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var pool types.Pool
		if err := proto.Unmarshal(iter.Value(), &pool); err != nil {
			continue
		}
		if cb(&pool) {
			break
		}
	}
}

func (k Keeper) CountPools(ctx sdk.Context) uint64 {
	var count uint64
	k.IteratePools(ctx, func(_ *types.Pool) bool {
		count++
		return false
	})
	return count
}

// --- TWAP Accumulator CRUD ---

func (k Keeper) SetTWAPAccumulator(ctx sdk.Context, acc *types.TWAPAccumulator) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(acc)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal TWAP accumulator: %v", err))
	}
	if err := store.Set(types.TWAPKey(acc.PoolId), bz); err != nil {
		panic(fmt.Sprintf("failed to set TWAP accumulator: %v", err))
	}
}

func (k Keeper) GetTWAPAccumulator(ctx sdk.Context, poolId string) (*types.TWAPAccumulator, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.TWAPKey(poolId))
	if err != nil || bz == nil {
		return nil, false
	}
	var acc types.TWAPAccumulator
	if err := proto.Unmarshal(bz, &acc); err != nil {
		return nil, false
	}
	return &acc, true
}

func (k Keeper) IterateTWAPAccumulators(ctx sdk.Context, cb func(*types.TWAPAccumulator) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.TWAPKeyPrefix, prefixEndBytes(types.TWAPKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var acc types.TWAPAccumulator
		if err := proto.Unmarshal(iter.Value(), &acc); err != nil {
			continue
		}
		if cb(&acc) {
			break
		}
	}
}

// --- LP Deposit Block (anti-JIT cooldown) ---

// SetLPDepositBlock records the block at which addr last deposited into poolId.
// Called by AddLiquidity to start the cooldown clock.
func (k Keeper) SetLPDepositBlock(ctx sdk.Context, poolId, addr string, block uint64) {
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	bz[0] = byte(block >> 56)
	bz[1] = byte(block >> 48)
	bz[2] = byte(block >> 40)
	bz[3] = byte(block >> 32)
	bz[4] = byte(block >> 24)
	bz[5] = byte(block >> 16)
	bz[6] = byte(block >> 8)
	bz[7] = byte(block)
	_ = store.Set(types.LPDepositBlockKey(poolId, addr), bz)
}

// GetLPDepositBlock returns the last deposit block for addr in poolId.
// Returns (0, false) when no deposit has been recorded (cooldown does not apply).
func (k Keeper) GetLPDepositBlock(ctx sdk.Context, poolId, addr string) (uint64, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.LPDepositBlockKey(poolId, addr))
	if err != nil || len(bz) < 8 {
		return 0, false
	}
	block := uint64(bz[0])<<56 | uint64(bz[1])<<48 | uint64(bz[2])<<40 | uint64(bz[3])<<32 |
		uint64(bz[4])<<24 | uint64(bz[5])<<16 | uint64(bz[6])<<8 | uint64(bz[7])
	return block, true
}
