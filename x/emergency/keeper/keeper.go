package keeper

import (
	"context"
	"fmt"
	"math/big"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/emergency/types"
)

// Keeper manages the emergency module's state.
type Keeper struct {
	storeService  store.KVStoreService
	cdc           codec.BinaryCodec
	authority     string
	stakingKeeper types.StakingKeeper
}

// NewKeeper creates a new emergency module Keeper.
func NewKeeper(
	storeService store.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	stakingKeeper types.StakingKeeper,
) Keeper {
	return Keeper{
		storeService:  storeService,
		cdc:           cdc,
		authority:     authority,
		stakingKeeper: stakingKeeper,
	}
}

// prefixEndBytes returns the end key for prefix iteration (exclusive).
func prefixEndBytes(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil
}

// Logger returns a module-scoped logger.
func (k Keeper) Logger(ctx context.Context) log.Logger {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return sdkCtx.Logger().With("module", "x/"+types.ModuleName)
}

// GetAuthority returns the module authority address.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// IsGuardian checks if an address is a Guardian-tier validator or genesis council member.
func (k Keeper) IsGuardian(ctx context.Context, operatorAddr string) bool {
	val, found := k.stakingKeeper.GetValidator(ctx, operatorAddr)
	if found && val.Tier == types.TierGuardian && val.IsActive {
		return true
	}
	// H-5: Check genesis emergency council.
	params := k.GetParams(ctx)
	if k.isCouncilActive(ctx, params) {
		return k.isCouncilMember(operatorAddr, params)
	}
	return false
}

// GetGuardianStake returns the total effective stake of all active Guardians.
func (k Keeper) GetGuardianStake(ctx context.Context) *big.Int {
	total := new(big.Int)
	guardians, err := k.stakingKeeper.GetGuardianValidators(ctx)
	if err != nil {
		return total
	}
	for _, v := range guardians {
		if v.IsActive {
			stake, ok := new(big.Int).SetString(v.TotalStake, 10)
			if ok {
				total.Add(total, stake)
			}
		}
	}
	// H-5: Add council virtual stake during bootstrap.
	params := k.GetParams(ctx)
	if k.isCouncilActive(ctx, params) && len(params.GenesisCouncil) > 0 {
		virtualPerMember, ok := new(big.Int).SetString(params.CouncilVirtualStake, 10)
		if ok {
			councilTotal := new(big.Int).Mul(virtualPerMember, big.NewInt(int64(len(params.GenesisCouncil))))
			total.Add(total, councilTotal)
		}
	}
	return total
}

// GetGuardianEffectiveStake returns a single guardian's effective stake.
func (k Keeper) GetGuardianEffectiveStake(ctx context.Context, operatorAddr string) *big.Int {
	val, found := k.stakingKeeper.GetValidator(ctx, operatorAddr)
	if found && val.Tier == types.TierGuardian && val.IsActive {
		stake, ok := new(big.Int).SetString(val.TotalStake, 10)
		if ok {
			return stake
		}
	}
	// H-5: Council member virtual stake.
	params := k.GetParams(ctx)
	if k.isCouncilActive(ctx, params) && k.isCouncilMember(operatorAddr, params) {
		virtualStake, ok := new(big.Int).SetString(params.CouncilVirtualStake, 10)
		if ok {
			return virtualStake
		}
	}
	return new(big.Int)
}

// isCouncilMember checks if an address is in the genesis emergency council.
func (k Keeper) isCouncilMember(operatorAddr string, params *types.Params) bool {
	for _, member := range params.GenesisCouncil {
		if member == operatorAddr {
			return true
		}
	}
	return false
}

// isCouncilActive checks if the genesis council is still active.
func (k Keeper) isCouncilActive(ctx context.Context, params *types.Params) bool {
	if params.CouncilExpiryBlock == 0 || len(params.GenesisCouncil) == 0 {
		return false
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return uint64(sdkCtx.BlockHeight()) < params.CouncilExpiryBlock
}

// --- Genesis ---

// InitGenesis initializes the module's state from genesis.
func (k Keeper) InitGenesis(ctx context.Context, genState *types.GenesisState) {
	if genState.Params != nil {
		k.SetParams(ctx, genState.Params)
	}
	k.SetEmergencyStatus(ctx, types.EmergencyStatus(genState.Status))
	for _, ceremony := range genState.Ceremonies {
		if ceremony == nil {
			continue
		}
		if err := k.SetCeremony(ctx, ceremony); err != nil {
			panic("failed to init genesis ceremony: " + err.Error())
		}
	}
	for _, entry := range genState.AuditLog {
		if entry == nil {
			continue
		}
		k.AddAuditEntry(ctx, entry)
	}
}

// ExportGenesis exports the module's state.
func (k Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	ceremonies := k.GetAllCeremonies(ctx)
	auditLog := k.GetAuditLog(ctx)
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params:     params,
		Status:     string(k.GetEmergencyStatus(ctx)),
		Ceremonies: ceremonies,
		AuditLog:   auditLog,
	}
}

// --- Oracle staleness circuit breaker (P-21b) ---

// RecordOracleUpdate is called by the liquiditypool module whenever a TWAP
// accumulator is updated (BeginBlock, per active pool). It stamps the current
// block height as the last known oracle liveness signal. The call is designed
// to be idempotent within a block — multiple pools updating the same block is
// fine; only the first write per block matters structurally, though subsequent
// same-block writes are harmless overwrites.
func (k Keeper) RecordOracleUpdate(ctx context.Context) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.SetLastOracleUpdateBlock(ctx, uint64(sdkCtx.BlockHeight()))
}

// CheckOracleStaleness is called from the emergency module's BeginBlock. It
// compares the current block height against the last recorded oracle update and
// halts the chain automatically if the oracle has been silent for longer than
// CircuitOracleStalenessBlocks.
//
// The check is a no-op when:
//   - CircuitOracleStalenessBlocks == 0 (disabled, the default)
//   - The chain is already halted (no point double-halting)
//   - LastOracleUpdateBlock == 0 (oracle keeper not yet wired or chain just started)
func (k Keeper) CheckOracleStaleness(ctx context.Context) {
	params := k.GetParams(ctx)
	if params.CircuitOracleStalenessBlocks == 0 {
		return // disabled
	}
	if k.IsHalted(ctx) {
		return // already halted; nothing to do
	}

	lastUpdate := k.GetLastOracleUpdateBlock(ctx)
	if lastUpdate == 0 {
		return // no oracle update ever recorded — oracle not yet wired; skip
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	current := uint64(sdkCtx.BlockHeight())
	if current <= lastUpdate {
		return // same block or before — not stale
	}

	staleness := current - lastUpdate
	if staleness <= params.CircuitOracleStalenessBlocks {
		return // within tolerance
	}

	// Oracle is stale beyond the threshold — halt the chain.
	k.autoHaltOracleStaleness(ctx, sdkCtx, staleness)
}

// autoHaltOracleStaleness halts the chain due to oracle staleness and records
// an audit entry explaining why.
func (k Keeper) autoHaltOracleStaleness(ctx context.Context, sdkCtx sdk.Context, staleBlocks uint64) {
	k.SetEmergencyStatus(ctx, types.StatusHalted)

	reason := fmt.Sprintf(
		"automatic halt: oracle TWAP not updated for %d blocks (threshold: %d)",
		staleBlocks, k.GetParams(ctx).CircuitOracleStalenessBlocks,
	)
	k.AddAuditEntry(ctx, &types.EmergencyAuditEntry{
		BlockNumber: uint64(sdkCtx.BlockHeight()),
		Action:      "circuit_breaker_oracle_staleness",
		Details:     reason,
	})

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			"emergency_circuit_breaker",
			sdk.NewAttribute("type", "oracle_staleness"),
			sdk.NewAttribute("stale_blocks", fmt.Sprintf("%d", staleBlocks)),
			sdk.NewAttribute("status", string(types.StatusHalted)),
		),
	)

	k.Logger(ctx).Error(reason)
}
