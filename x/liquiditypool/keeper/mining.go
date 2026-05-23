package keeper

// mining.go — Liquidity mining state management and reward distribution.
//
// Reward model: MasterChef/Synthetix accumulator pattern.
//   Pool.AccumulatedRewardPerLp (×1e18) — monotonically increasing global accumulator.
//   LpMinerPosition.RewardDebt  (×1e18) — snapshot of accumulator at last settlement.
//   Pending = (acc - debt) × lpBalance / 1e18
//
// Invariant: all bigint operations use math/big; no floating point.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// MiningScale is 1e18 — precision multiplier for accumulated_reward_per_lp.
var MiningScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// GenerateMiningPeriodID produces a deterministic ID for a LiquidityMiningPeriod.
func GenerateMiningPeriodID(poolId string, startBlock, endBlock uint64) string {
	data := fmt.Sprintf("ZRN.mining.v1:%s:%d:%d", poolId, startBlock, endBlock)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ---------- LiquidityMiningPeriod CRUD ----------

// SetLiquidityMiningPeriod stores a mining period.
func (k Keeper) SetLiquidityMiningPeriod(ctx sdk.Context, period *types.LiquidityMiningPeriod) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(period)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal LiquidityMiningPeriod: %v", err))
	}
	if err := store.Set(types.MiningPeriodKey(period.Id), bz); err != nil {
		panic(fmt.Sprintf("failed to set mining period: %v", err))
	}
}

// GetLiquidityMiningPeriod retrieves a mining period by id. Returns nil if not found.
func (k Keeper) GetLiquidityMiningPeriod(ctx sdk.Context, periodId string) *types.LiquidityMiningPeriod {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.MiningPeriodKey(periodId))
	if err != nil || bz == nil {
		return nil
	}
	var period types.LiquidityMiningPeriod
	if err := proto.Unmarshal(bz, &period); err != nil {
		return nil
	}
	return &period
}

// GetActiveMiningPeriodByPool retrieves the active mining period for a pool, or nil.
func (k Keeper) GetActiveMiningPeriodByPool(ctx sdk.Context, poolId string) *types.LiquidityMiningPeriod {
	store := k.storeService.OpenKVStore(ctx)
	idBz, err := store.Get(types.PoolMiningIndexKey(poolId))
	if err != nil || idBz == nil {
		return nil
	}
	return k.GetLiquidityMiningPeriod(ctx, string(idBz))
}

// setPoolMiningIndex writes or deletes the pool → mining period index.
func (k Keeper) setPoolMiningIndex(ctx sdk.Context, poolId, periodId string) {
	store := k.storeService.OpenKVStore(ctx)
	if err := store.Set(types.PoolMiningIndexKey(poolId), []byte(periodId)); err != nil {
		panic(fmt.Sprintf("failed to set pool mining index: %v", err))
	}
}

// deletePoolMiningIndex removes the pool → mining period index entry.
func (k Keeper) deletePoolMiningIndex(ctx sdk.Context, poolId string) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.PoolMiningIndexKey(poolId))
}

// IterateMiningPeriods iterates over all stored mining periods.
func (k Keeper) IterateMiningPeriods(ctx sdk.Context, cb func(*types.LiquidityMiningPeriod) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.MiningPeriodKeyPrefix, prefixEndBytes(types.MiningPeriodKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var period types.LiquidityMiningPeriod
		if err := proto.Unmarshal(iter.Value(), &period); err != nil {
			continue
		}
		if cb(&period) {
			break
		}
	}
}

// ---------- LpMinerPosition CRUD ----------

// SetLpMinerPosition stores a miner position.
func (k Keeper) SetLpMinerPosition(ctx sdk.Context, pos *types.LpMinerPosition) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(pos)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal LpMinerPosition: %v", err))
	}
	if err := store.Set(types.LpMinerPositionKey(pos.PoolId, pos.MinerAddr), bz); err != nil {
		panic(fmt.Sprintf("failed to set miner position: %v", err))
	}
}

// GetLpMinerPosition retrieves a miner position, or an empty zeroed position if not found.
func (k Keeper) GetLpMinerPosition(ctx sdk.Context, poolId, addr string) *types.LpMinerPosition {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.LpMinerPositionKey(poolId, addr))
	if err != nil || bz == nil {
		return &types.LpMinerPosition{
			PoolId:        poolId,
			MinerAddr:     addr,
			RewardDebt:    "0",
			PendingRewards: "0",
		}
	}
	var pos types.LpMinerPosition
	if err := proto.Unmarshal(bz, &pos); err != nil {
		return &types.LpMinerPosition{
			PoolId:        poolId,
			MinerAddr:     addr,
			RewardDebt:    "0",
			PendingRewards: "0",
		}
	}
	// Normalise empty strings to "0"
	if pos.RewardDebt == "" {
		pos.RewardDebt = "0"
	}
	if pos.PendingRewards == "" {
		pos.PendingRewards = "0"
	}
	return &pos
}

// IterateLpMinerPositions iterates all miner positions. Return true from cb to stop.
func (k Keeper) IterateLpMinerPositions(ctx sdk.Context, cb func(*types.LpMinerPosition) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.LpMinerPositionPrefix, prefixEndBytes(types.LpMinerPositionPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var pos types.LpMinerPosition
		if err := proto.Unmarshal(iter.Value(), &pos); err != nil {
			continue
		}
		if cb(&pos) {
			break
		}
	}
}

// ---------- Core Distribution Logic ----------

// AccumulateMiningRewards is called from BeginBlocker for each active mining period.
// It mints amountPerBlock uzrn to the module account and increments the pool's
// accumulated_reward_per_lp accumulator proportionally.
//
// If lp_token_supply is 0, rewards are skipped (no holders to distribute to).
func (k Keeper) AccumulateMiningRewards(ctx sdk.Context, pool *types.Pool, period *types.LiquidityMiningPeriod) {
	amountPerBlock := new(big.Int)
	if _, ok := amountPerBlock.SetString(period.AmountPerBlock, 10); !ok || amountPerBlock.Sign() <= 0 {
		return
	}

	totalSupply := new(big.Int)
	if _, ok := totalSupply.SetString(pool.LpTokenSupply, 10); !ok || totalSupply.Sign() <= 0 {
		// No LP holders — skip (rewards are not minted; they are lost for this block).
		return
	}

	// Mint amountPerBlock to module account for later distribution.
	if k.bankKeeper != nil {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amountPerBlock)))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			k.Logger(ctx).Error("liquidity mining: failed to mint rewards", "error", err, "period", period.Id)
			return
		}
	}

	// delta = amountPerBlock × MiningScale / totalSupply
	delta := new(big.Int).Mul(amountPerBlock, MiningScale)
	delta.Div(delta, totalSupply)

	// accumulator += delta
	acc := new(big.Int)
	if pool.AccumulatedRewardPerLp != "" && pool.AccumulatedRewardPerLp != "0" {
		acc.SetString(pool.AccumulatedRewardPerLp, 10)
	}
	acc.Add(acc, delta)
	pool.AccumulatedRewardPerLp = acc.String()
	k.SetPool(ctx, pool)

	// Track total mined
	totalMined := new(big.Int)
	if period.TotalMined != "" {
		totalMined.SetString(period.TotalMined, 10)
	}
	totalMined.Add(totalMined, amountPerBlock)
	period.TotalMined = totalMined.String()
	k.SetLiquidityMiningPeriod(ctx, period)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.mining_reward_accumulated",
		sdk.NewAttribute("pool_id", pool.PoolId),
		sdk.NewAttribute("period_id", period.Id),
		sdk.NewAttribute("amount", amountPerBlock.String()),
		sdk.NewAttribute("accumulated_reward_per_lp", acc.String()),
	))
}

// SettleMinerRewards computes and records pending rewards for a miner BEFORE their
// LP balance changes. Call this in AddLiquidity and RemoveLiquidity handlers.
// lpBalance is the miner's current LP token balance (before the pending tx changes it).
func (k Keeper) SettleMinerRewards(ctx sdk.Context, poolId, minerAddr string, lpBalance *big.Int, pool *types.Pool) {
	pos := k.GetLpMinerPosition(ctx, poolId, minerAddr)

	currentAcc := new(big.Int)
	if pool.AccumulatedRewardPerLp != "" && pool.AccumulatedRewardPerLp != "0" {
		currentAcc.SetString(pool.AccumulatedRewardPerLp, 10)
	}

	debt := new(big.Int)
	if pos.RewardDebt != "" && pos.RewardDebt != "0" {
		debt.SetString(pos.RewardDebt, 10)
	}

	// pending = (currentAcc - debt) × lpBalance / MiningScale
	if currentAcc.Cmp(debt) > 0 && lpBalance.Sign() > 0 {
		diff := new(big.Int).Sub(currentAcc, debt)
		pending := new(big.Int).Mul(diff, lpBalance)
		pending.Div(pending, MiningScale)

		if pending.Sign() > 0 {
			existing := new(big.Int)
			if pos.PendingRewards != "" && pos.PendingRewards != "0" {
				existing.SetString(pos.PendingRewards, 10)
			}
			existing.Add(existing, pending)
			pos.PendingRewards = existing.String()
		}
	}

	// Update debt to current accumulator regardless (sync on every interaction)
	pos.RewardDebt = currentAcc.String()
	k.SetLpMinerPosition(ctx, pos)
}

// ClaimPendingRewards settles and pays out all pending rewards for a miner.
// Returns the amount claimed (uzrn bigint). Returns 0 if nothing to claim.
func (k Keeper) ClaimPendingRewards(ctx sdk.Context, poolId, minerAddr string, pool *types.Pool) (*big.Int, error) {
	// Get current LP balance for final settlement
	minerAccAddr, err := sdk.AccAddressFromBech32(minerAddr)
	if err != nil {
		return big.NewInt(0), err
	}

	lpDenom := types.LPDenom(poolId)
	var lpBalance *big.Int
	if k.bankKeeper != nil {
		coin := k.bankKeeper.GetBalance(ctx, minerAccAddr, lpDenom)
		lpBalance = coin.Amount.BigInt()
	} else {
		lpBalance = big.NewInt(0)
	}

	// Settle any accrued rewards since last interaction
	k.SettleMinerRewards(ctx, poolId, minerAddr, lpBalance, pool)

	pos := k.GetLpMinerPosition(ctx, poolId, minerAddr)
	pending := new(big.Int)
	if pos.PendingRewards != "" && pos.PendingRewards != "0" {
		pending.SetString(pos.PendingRewards, 10)
	}

	if pending.Sign() <= 0 {
		return big.NewInt(0), types.ErrNoRewardsToClaim
	}

	// Transfer from module account to user
	if k.bankKeeper != nil {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(pending)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, minerAccAddr, coins); err != nil {
			return big.NewInt(0), fmt.Errorf("failed to pay mining rewards: %w", err)
		}
	}

	// Clear pending
	pos.PendingRewards = "0"
	k.SetLpMinerPosition(ctx, pos)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.mining_rewards_claimed",
		sdk.NewAttribute("pool_id", poolId),
		sdk.NewAttribute("miner", minerAddr),
		sdk.NewAttribute("amount", pending.String()),
	))

	return pending, nil
}

// CreateMiningPeriod validates and stores a new mining period.
// Returns ErrMiningPeriodExists if the pool already has an active period.
func (k Keeper) CreateMiningPeriod(ctx sdk.Context, poolId, authority, amountPerBlock string, startBlock, endBlock uint64) (*types.LiquidityMiningPeriod, error) {
	// Validate
	amount := new(big.Int)
	if _, ok := amount.SetString(amountPerBlock, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrMiningPeriodInvalid
	}
	if endBlock <= startBlock {
		return nil, types.ErrMiningPeriodInvalid
	}

	// Check no active period exists
	existing := k.GetActiveMiningPeriodByPool(ctx, poolId)
	if existing != nil && existing.Active {
		return nil, types.ErrMiningPeriodExists
	}

	periodId := GenerateMiningPeriodID(poolId, startBlock, endBlock)
	period := &types.LiquidityMiningPeriod{
		Id:             periodId,
		PoolId:         poolId,
		AmountPerBlock: amountPerBlock,
		StartBlock:     startBlock,
		EndBlock:       endBlock,
		Active:         true,
		TotalMined:     "0",
		Creator:        authority,
	}

	k.SetLiquidityMiningPeriod(ctx, period)
	k.setPoolMiningIndex(ctx, poolId, periodId)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.mining_period_created",
		sdk.NewAttribute("period_id", periodId),
		sdk.NewAttribute("pool_id", poolId),
		sdk.NewAttribute("amount_per_block", amountPerBlock),
		sdk.NewAttribute("start_block", fmt.Sprintf("%d", startBlock)),
		sdk.NewAttribute("end_block", fmt.Sprintf("%d", endBlock)),
	))

	return period, nil
}

// ProcessMiningBeginBlock is the BeginBlocker hook for all active mining periods.
// Called once per block. Iterates active periods and accumulates rewards.
func (k Keeper) ProcessMiningBeginBlock(ctx sdk.Context) {
	currentBlock := uint64(ctx.BlockHeight())

	k.IterateMiningPeriods(ctx, func(period *types.LiquidityMiningPeriod) bool {
		if !period.Active {
			return false
		}

		if currentBlock < period.StartBlock {
			return false // not started yet
		}

		if currentBlock > period.EndBlock {
			// Deactivate expired period
			period.Active = false
			k.SetLiquidityMiningPeriod(ctx, period)
			k.deletePoolMiningIndex(ctx, period.PoolId)
			return false
		}

		pool, found := k.GetPool(ctx, period.PoolId)
		if !found {
			return false
		}

		k.AccumulateMiningRewards(ctx, pool, period)
		return false
	})
}
