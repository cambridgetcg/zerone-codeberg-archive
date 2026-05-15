package keeper

import (
	"context"
	"encoding/binary"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/emergency/types"
)

// --- Emergency Status ---

// GetEmergencyStatus returns the current emergency status.
func (k Keeper) GetEmergencyStatus(ctx context.Context) types.EmergencyStatus {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.HaltStatusKey)
	if err != nil || bz == nil {
		return types.StatusNormal
	}
	return types.EmergencyStatus(bz)
}

// SetEmergencyStatus sets the current emergency status.
func (k Keeper) SetEmergencyStatus(ctx context.Context, status types.EmergencyStatus) {
	store := k.storeService.OpenKVStore(ctx)
	if err := store.Set(types.HaltStatusKey, []byte(status)); err != nil {
		panic("failed to set emergency status: " + err.Error())
	}
}

// IsHalted returns true if the chain is in any halted state.
func (k Keeper) IsHalted(ctx context.Context) bool {
	status := k.GetEmergencyStatus(ctx)
	switch status {
	case types.StatusHalted, types.StatusRevertVoting, types.StatusReverting, types.StatusResumeVoting:
		return true
	default:
		return false
	}
}

// --- Ceremonies ---

// SetCeremony stores a ceremony by ID.
func (k Keeper) SetCeremony(ctx context.Context, ceremony *types.EmergencyCeremony) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(ceremony)
	if err != nil {
		return fmt.Errorf("failed to marshal emergency ceremony: %w", err)
	}
	if err := store.Set(types.CeremonyKey(ceremony.Id), bz); err != nil {
		return fmt.Errorf("failed to set emergency ceremony: %w", err)
	}
	return nil
}

// GetCeremony retrieves a ceremony by ID.
func (k Keeper) GetCeremony(ctx context.Context, id string) (*types.EmergencyCeremony, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.CeremonyKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var ceremony types.EmergencyCeremony
	if err := proto.Unmarshal(bz, &ceremony); err != nil {
		return nil, false
	}
	return &ceremony, true
}

// GetActiveCeremony returns the currently active (non-terminal) ceremony, if any.
func (k Keeper) GetActiveCeremony(ctx context.Context) (*types.EmergencyCeremony, bool) {
	var active *types.EmergencyCeremony
	k.IterateCeremonies(ctx, func(c *types.EmergencyCeremony) bool {
		if c.Phase != string(types.PhaseFinalized) && c.Phase != string(types.PhaseFailed) {
			active = c
			return true
		}
		return false
	})
	if active == nil {
		return nil, false
	}
	return active, true
}

// GetAllCeremonies returns all stored ceremonies.
func (k Keeper) GetAllCeremonies(ctx context.Context) []*types.EmergencyCeremony {
	var ceremonies []*types.EmergencyCeremony
	k.IterateCeremonies(ctx, func(c *types.EmergencyCeremony) bool {
		ceremonies = append(ceremonies, c)
		return false
	})
	return ceremonies
}

// IterateCeremonies iterates over all ceremonies. Return true from cb to stop.
func (k Keeper) IterateCeremonies(ctx context.Context, cb func(*types.EmergencyCeremony) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.CeremonyKeyPrefix, prefixEndBytes(types.CeremonyKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var ceremony types.EmergencyCeremony
		if err := proto.Unmarshal(iter.Value(), &ceremony); err != nil {
			continue
		}
		if cb(&ceremony) {
			break
		}
	}
}

// --- Active Halt Ceremony ID ---

// GetActiveHaltCeremonyId returns the ID of the current active halt ceremony.
func (k Keeper) GetActiveHaltCeremonyId(ctx context.Context) string {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ActiveHaltCeremonyIdKey)
	if err != nil || bz == nil {
		return ""
	}
	return string(bz)
}

// SetActiveHaltCeremonyId stores the active halt ceremony ID.
func (k Keeper) SetActiveHaltCeremonyId(ctx context.Context, id string) {
	store := k.storeService.OpenKVStore(ctx)
	if id == "" {
		_ = store.Delete(types.ActiveHaltCeremonyIdKey)
	} else {
		_ = store.Set(types.ActiveHaltCeremonyIdKey, []byte(id))
	}
}

// --- Halt Start Block ---

// GetHaltStartBlock returns the block at which the current halt began.
func (k Keeper) GetHaltStartBlock(ctx context.Context) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.HaltStartBlockKey)
	if err != nil || bz == nil {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// SetHaltStartBlock records the block at which the halt began.
func (k Keeper) SetHaltStartBlock(ctx context.Context, block uint64) {
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, block)
	_ = store.Set(types.HaltStartBlockKey, bz)
}

// ClearHaltStartBlock removes the halt start block (called on resume).
func (k Keeper) ClearHaltStartBlock(ctx context.Context) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.HaltStartBlockKey)
}

// --- Audit Log ---

// AddAuditEntry appends an audit entry to the log.
func (k Keeper) AddAuditEntry(ctx context.Context, entry *types.EmergencyAuditEntry) {
	store := k.storeService.OpenKVStore(ctx)
	height := entry.BlockNumber
	var index uint32
	for {
		key := types.AuditLogKey(height, index)
		bz, err := store.Get(key)
		if err != nil || bz == nil {
			data, err := proto.Marshal(entry)
			if err != nil {
				return
			}
			_ = store.Set(key, data)
			return
		}
		index++
	}
}

// GetAuditLog returns all audit entries.
func (k Keeper) GetAuditLog(ctx context.Context) []*types.EmergencyAuditEntry {
	var entries []*types.EmergencyAuditEntry
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.AuditLogKeyPrefix, prefixEndBytes(types.AuditLogKeyPrefix))
	if err != nil {
		return entries
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		var entry types.EmergencyAuditEntry
		if err := proto.Unmarshal(iter.Value(), &entry); err != nil {
			continue
		}
		entries = append(entries, &entry)
	}
	return entries
}

// --- Anti-Abuse Tracking ---

// GetGuardianProposalCount returns how many proposals a guardian has made this epoch.
func (k Keeper) GetGuardianProposalCount(ctx context.Context, addr string) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.GuardianProposalCountKey(addr))
	if err != nil || bz == nil {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// IncrementGuardianProposalCount increments a guardian's proposal count.
func (k Keeper) IncrementGuardianProposalCount(ctx context.Context, addr string) {
	count := k.GetGuardianProposalCount(ctx, addr) + 1
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, count)
	_ = store.Set(types.GuardianProposalCountKey(addr), bz)
}

// GetEpochProposalCount returns the global proposal count for this epoch.
func (k Keeper) GetEpochProposalCount(ctx context.Context) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.EpochProposalCountKey)
	if err != nil || bz == nil {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// IncrementEpochProposalCount increments the global epoch proposal count.
func (k Keeper) IncrementEpochProposalCount(ctx context.Context) {
	count := k.GetEpochProposalCount(ctx) + 1
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, count)
	_ = store.Set(types.EpochProposalCountKey, bz)
}

// GetLastProposalBlock returns the block height of the last proposal.
func (k Keeper) GetLastProposalBlock(ctx context.Context) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.LastProposalBlockKey)
	if err != nil || bz == nil {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// SetLastProposalBlock stores the block height of the last proposal.
func (k Keeper) SetLastProposalBlock(ctx context.Context, block uint64) {
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, block)
	_ = store.Set(types.LastProposalBlockKey, bz)
}

// ResetEpochCounters clears all per-epoch anti-abuse tracking.
func (k Keeper) ResetEpochCounters(ctx context.Context) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.EpochProposalCountKey)
	iter, err := store.Iterator(types.GuardianProposalCountPrefix, prefixEndBytes(types.GuardianProposalCountPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	var keys [][]byte
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, iter.Key())
	}
	for _, key := range keys {
		_ = store.Delete(key)
	}
}

// --- Revert Target ---

// RevertTarget holds the guardian-agreed state rollback target.
type RevertTarget struct {
	Height     uint64 `json:"height"`
	BlockHash  string `json:"block_hash"`
	CeremonyId string `json:"ceremony_id"`
}

// SetRevertTarget stores the revert target agreed upon by guardians.
func (k Keeper) SetRevertTarget(ctx context.Context, height uint64, blockHash string, ceremonyId string) {
	store := k.storeService.OpenKVStore(ctx)
	heightBz := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBz, height)
	_ = store.Set(types.RevertTargetHeightKey, heightBz)
	_ = store.Set(types.RevertTargetHashKey, []byte(blockHash))
	_ = store.Set(types.RevertCeremonyIdKey, []byte(ceremonyId))
}

// GetRevertTarget returns the current revert target, if set.
func (k Keeper) GetRevertTarget(ctx context.Context) (RevertTarget, bool) {
	store := k.storeService.OpenKVStore(ctx)
	heightBz, err := store.Get(types.RevertTargetHeightKey)
	if err != nil || heightBz == nil {
		return RevertTarget{}, false
	}
	height := binary.BigEndian.Uint64(heightBz)
	hashBz, _ := store.Get(types.RevertTargetHashKey)
	ceremonyBz, _ := store.Get(types.RevertCeremonyIdKey)
	return RevertTarget{
		Height:     height,
		BlockHash:  string(hashBz),
		CeremonyId: string(ceremonyBz),
	}, true
}

// ClearRevertTarget removes the revert target (called on resume after rollback).
func (k Keeper) ClearRevertTarget(ctx context.Context) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.RevertTargetHeightKey)
	_ = store.Delete(types.RevertTargetHashKey)
	_ = store.Delete(types.RevertCeremonyIdKey)
}

// --- Params ---

// GetParams returns the emergency module parameters.
func (k Keeper) GetParams(ctx context.Context) *types.Params {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ParamsKey)
	if err != nil || bz == nil {
		p := types.DefaultParams()
		return &p
	}
	var params types.Params
	if err := proto.Unmarshal(bz, &params); err != nil {
		p := types.DefaultParams()
		return &p
	}
	return &params
}

// SetParams stores the emergency module parameters.
func (k Keeper) SetParams(ctx context.Context, params *types.Params) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(params)
	if err != nil {
		panic("failed to marshal emergency params: " + err.Error())
	}
	if err := store.Set(types.ParamsKey, bz); err != nil {
		panic("failed to set emergency params: " + err.Error())
	}
}

// sdkContext is a helper to get sdk.Context from context.Context for event emission.
func sdkContext(ctx context.Context) sdk.Context {
	return sdk.UnwrapSDKContext(ctx)
}

// --- Oracle staleness circuit breaker state (P-21b) ---

// GetLastOracleUpdateBlock returns the block height of the most recent oracle
// (TWAP) update recorded by the emergency module. Returns 0 if no update has
// been recorded yet (fresh chain or oracle keeper not wired).
func (k Keeper) GetLastOracleUpdateBlock(ctx context.Context) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.LastOracleUpdateBlockKey)
	if err != nil || len(bz) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// SetLastOracleUpdateBlock stores the block height of the most recent oracle
// (TWAP) update.
func (k Keeper) SetLastOracleUpdateBlock(ctx context.Context, height uint64) {
	store := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, height)
	if err := store.Set(types.LastOracleUpdateBlockKey, bz); err != nil {
		panic("failed to set last oracle update block: " + err.Error())
	}
}
