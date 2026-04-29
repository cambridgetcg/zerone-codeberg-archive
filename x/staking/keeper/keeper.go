package keeper

import (
	"encoding/json"
	"fmt"
	"math/big"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/staking/types"
)


// Keeper manages the zerone_staking module state.
type Keeper struct {
	cdc               codec.Codec
	storeKey          *storetypes.KVStoreKey
	accountKeeper     types.AccountKeeper
	bankKeeper        types.BankKeeper
	authKeeper        types.ZeroneAuthKeeper
	autopoiesisKeeper types.AutopoiesisKeeper
	emergencyKeeper   types.EmergencyKeeper // nil-safe; set via SetEmergencyKeeper
	authority         string
	logger            log.Logger
}

// NewKeeper creates a new Keeper.
func NewKeeper(
	cdc codec.Codec,
	storeKey *storetypes.KVStoreKey,
	ak types.AccountKeeper,
	bk types.BankKeeper,
	authority string,
) Keeper {
	return Keeper{
		cdc:          cdc,
		storeKey:     storeKey,
		accountKeeper: ak,
		bankKeeper:   bk,
		authority:    authority,
		logger:       log.NewNopLogger(),
	}
}

// SetAutopoiesisKeeper sets the autopoiesis keeper (post-init to break circular dep).
func (k *Keeper) SetAutopoiesisKeeper(ak types.AutopoiesisKeeper) {
	k.autopoiesisKeeper = ak
}

// SetAuthKeeper sets the zerone auth keeper.
func (k *Keeper) SetAuthKeeper(ak types.ZeroneAuthKeeper) {
	k.authKeeper = ak
}

// SetEmergencyKeeper sets the emergency keeper (post-init to break circular dep).
// The emergency keeper receives a RecordSlash notification on every confirmed slash
// so it can monitor the slash rate for circuit breaker purposes.
func (k *Keeper) SetEmergencyKeeper(ek types.EmergencyKeeper) {
	k.emergencyKeeper = ek
}

// GetAuthority returns the module authority address.
func (k Keeper) GetAuthority() string { return k.authority }

// ---------- Store accessor ----------

func (k Keeper) getStore(ctx sdk.Context) storetypes.KVStore {
	return ctx.KVStore(k.storeKey)
}

// ---------- Validator CRUD ----------

// SetValidator stores a validator by operator address and updates the DID index.
func (k Keeper) SetValidator(ctx sdk.Context, val *types.Validator) {
	store := k.getStore(ctx)
	bz, err := json.Marshal(val)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal validator: %v", err))
	}
	store.Set(types.ValidatorKey(val.OperatorAddress), bz)
	if val.Did != "" {
		store.Set(types.ValidatorByDIDKey(val.Did), []byte(val.OperatorAddress))
	}
}

// GetValidator retrieves a validator by operator address.
func (k Keeper) GetValidator(ctx sdk.Context, operatorAddr string) (*types.Validator, bool) {
	store := k.getStore(ctx)
	bz := store.Get(types.ValidatorKey(operatorAddr))
	if bz == nil {
		return nil, false
	}
	var val types.Validator
	if err := json.Unmarshal(bz, &val); err != nil {
		return nil, false
	}
	return &val, true
}

// GetValidatorByDID retrieves a validator by DID.
func (k Keeper) GetValidatorByDID(ctx sdk.Context, did string) (*types.Validator, bool) {
	store := k.getStore(ctx)
	operatorBz := store.Get(types.ValidatorByDIDKey(did))
	if operatorBz == nil {
		return nil, false
	}
	return k.GetValidator(ctx, string(operatorBz))
}

// DeleteValidator removes a validator from the store.
func (k Keeper) DeleteValidator(ctx sdk.Context, operatorAddr string) {
	val, found := k.GetValidator(ctx, operatorAddr)
	if !found {
		return
	}
	store := k.getStore(ctx)
	store.Delete(types.ValidatorKey(operatorAddr))
	if val.Did != "" {
		store.Delete(types.ValidatorByDIDKey(val.Did))
	}
}

// IterateValidators iterates over all validators.
func (k Keeper) IterateValidators(ctx sdk.Context, cb func(val *types.Validator) bool) {
	store := k.getStore(ctx)
	iter := storetypes.KVStorePrefixIterator(store, types.ValidatorKeyPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Validator
		if err := json.Unmarshal(iter.Value(), &val); err != nil {
			continue
		}
		if cb(&val) {
			break
		}
	}
}

// GetActiveValidatorSet returns all active validators.
func (k Keeper) GetActiveValidatorSet(ctx sdk.Context) []*types.Validator {
	var active []*types.Validator
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		if val.IsActive {
			active = append(active, val)
		}
		return false
	})
	return active
}

// GetTotalBondedStake returns the total bonded stake across active validators.
func (k Keeper) GetTotalBondedStake(ctx sdk.Context) *big.Int {
	total := new(big.Int)
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		if val.IsActive {
			stake, ok := new(big.Int).SetString(val.TotalStake, 10)
			if ok {
				total.Add(total, stake)
			}
		}
		return false
	})
	return total
}

// CountBlockProducers counts validators at tier Scholar or above.
func (k Keeper) CountBlockProducers(ctx sdk.Context) uint64 {
	var count uint64
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		if val.IsActive && val.Tier >= types.TierScholar {
			count++
		}
		return false
	})
	return count
}

// ---------- Delegation CRUD ----------

// SetDelegation stores a delegation and maintains the reverse index.
func (k Keeper) SetDelegation(ctx sdk.Context, del *types.Delegation) {
	store := k.getStore(ctx)
	bz, err := json.Marshal(del)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal delegation: %v", err))
	}
	store.Set(types.DelegationKey(del.DelegatorAddress, del.ValidatorAddress), bz)
	// Reverse index: validator → delegator (P1-1 fix)
	store.Set(types.ValidatorDelegationIndexKey(del.ValidatorAddress, del.DelegatorAddress), []byte{0x01})
}

// GetDelegation retrieves a delegation.
func (k Keeper) GetDelegation(ctx sdk.Context, delegatorAddr, validatorAddr string) (*types.Delegation, bool) {
	store := k.getStore(ctx)
	bz := store.Get(types.DelegationKey(delegatorAddr, validatorAddr))
	if bz == nil {
		return nil, false
	}
	var del types.Delegation
	if err := json.Unmarshal(bz, &del); err != nil {
		return nil, false
	}
	return &del, true
}

// GetDelegationsForValidator returns all delegations for a validator using the reverse index.
func (k Keeper) GetDelegationsForValidator(ctx sdk.Context, validatorAddr string) []*types.Delegation {
	store := k.getStore(ctx)
	prefix := types.DelegationsByValidatorPrefix(validatorAddr)
	iter := storetypes.KVStorePrefixIterator(store, prefix)
	defer iter.Close()

	var delegations []*types.Delegation
	for ; iter.Valid(); iter.Next() {
		// Extract delegator address from key: [prefix][validatorAddr][0x00][delegatorAddr]
		key := iter.Key()
		delegatorAddr := string(key[len(prefix):])
		del, found := k.GetDelegation(ctx, delegatorAddr, validatorAddr)
		if found {
			delegations = append(delegations, del)
		}
	}
	return delegations
}

// DeleteDelegation removes a delegation and its reverse index.
func (k Keeper) DeleteDelegation(ctx sdk.Context, delegatorAddr, validatorAddr string) {
	store := k.getStore(ctx)
	store.Delete(types.DelegationKey(delegatorAddr, validatorAddr))
	store.Delete(types.ValidatorDelegationIndexKey(validatorAddr, delegatorAddr))
}

// IterateDelegations iterates over all delegations.
func (k Keeper) IterateDelegations(ctx sdk.Context, cb func(del *types.Delegation) bool) {
	store := k.getStore(ctx)
	iter := storetypes.KVStorePrefixIterator(store, types.DelegationKeyPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var del types.Delegation
		if err := json.Unmarshal(iter.Value(), &del); err != nil {
			continue
		}
		if cb(&del) {
			break
		}
	}
}

// ---------- Unbonding CRUD ----------

// SetUnbonding stores an unbonding entry.
func (k Keeper) SetUnbonding(ctx sdk.Context, entry *types.UnbondingEntry) {
	store := k.getStore(ctx)
	bz, err := json.Marshal(entry)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal unbonding: %v", err))
	}
	store.Set(types.UnbondingKey(entry.Id), bz)
}

// GetUnbonding retrieves an unbonding entry by ID.
func (k Keeper) GetUnbonding(ctx sdk.Context, id string) (*types.UnbondingEntry, bool) {
	store := k.getStore(ctx)
	bz := store.Get(types.UnbondingKey(id))
	if bz == nil {
		return nil, false
	}
	var entry types.UnbondingEntry
	if err := json.Unmarshal(bz, &entry); err != nil {
		return nil, false
	}
	return &entry, true
}

// DeleteUnbonding removes an unbonding entry.
func (k Keeper) DeleteUnbonding(ctx sdk.Context, id string) {
	store := k.getStore(ctx)
	store.Delete(types.UnbondingKey(id))
}

// GetMatureUnbondings returns unbondings that have completed.
func (k Keeper) GetMatureUnbondings(ctx sdk.Context, currentHeight uint64) []*types.UnbondingEntry {
	var mature []*types.UnbondingEntry
	k.IterateUnbondings(ctx, func(entry *types.UnbondingEntry) bool {
		if entry.Status == "pending" && entry.CompletesAtHeight <= currentHeight {
			mature = append(mature, entry)
		}
		return false
	})
	return mature
}

// GetUnbondingsForDelegator returns all unbondings for a delegator.
func (k Keeper) GetUnbondingsForDelegator(ctx sdk.Context, delegatorAddr string) []*types.UnbondingEntry {
	var entries []*types.UnbondingEntry
	k.IterateUnbondings(ctx, func(entry *types.UnbondingEntry) bool {
		if entry.DelegatorAddress == delegatorAddr {
			entries = append(entries, entry)
		}
		return false
	})
	return entries
}

// IterateUnbondings iterates over all unbonding entries.
func (k Keeper) IterateUnbondings(ctx sdk.Context, cb func(entry *types.UnbondingEntry) bool) {
	store := k.getStore(ctx)
	iter := storetypes.KVStorePrefixIterator(store, types.UnbondingKeyPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var entry types.UnbondingEntry
		if err := json.Unmarshal(iter.Value(), &entry); err != nil {
			continue
		}
		if cb(&entry) {
			break
		}
	}
}

// ---------- Unbonding Sequence ----------

// NextUnbondingSeq returns the next unbonding sequence ID.
func (k Keeper) NextUnbondingSeq(ctx sdk.Context) uint64 {
	seq := k.GetUnbondingSeq(ctx) + 1
	k.SetUnbondingSeq(ctx, seq)
	return seq
}

// GetUnbondingSeq returns the current unbonding sequence.
func (k Keeper) GetUnbondingSeq(ctx sdk.Context) uint64 {
	store := k.getStore(ctx)
	bz := store.Get(types.UnbondingSeqKey)
	if bz == nil {
		return 0
	}
	return types.BytesToUint64(bz)
}

// SetUnbondingSeq stores the unbonding sequence.
func (k Keeper) SetUnbondingSeq(ctx sdk.Context, seq uint64) {
	store := k.getStore(ctx)
	store.Set(types.UnbondingSeqKey, types.Uint64ToBytes(seq))
}

// ---------- Tier Config ----------

// SetTierConfig stores a tier configuration.
func (k Keeper) SetTierConfig(ctx sdk.Context, tc *types.TierConfig) {
	store := k.getStore(ctx)
	bz, err := json.Marshal(tc)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal tier config: %v", err))
	}
	store.Set(types.TierConfigKey(tc.Tier), bz)
}

// GetTierConfig retrieves a tier configuration.
func (k Keeper) GetTierConfig(ctx sdk.Context, tier types.ValidatorTier) (*types.TierConfig, bool) {
	store := k.getStore(ctx)
	bz := store.Get(types.TierConfigKey(tier))
	if bz == nil {
		return nil, false
	}
	var tc types.TierConfig
	if err := json.Unmarshal(bz, &tc); err != nil {
		return nil, false
	}
	return &tc, true
}

// GetAllTierConfigs returns all 4 tier configurations.
func (k Keeper) GetAllTierConfigs(ctx sdk.Context) []*types.TierConfig {
	var configs []*types.TierConfig
	for _, tier := range []types.ValidatorTier{types.TierApprentice, types.TierVerified, types.TierScholar, types.TierGuardian} {
		tc, found := k.GetTierConfig(ctx, tier)
		if found {
			configs = append(configs, tc)
		}
	}
	return configs
}

// ---------- Redelegation Cooldown ----------

// SetLastRedelegationHeight stores the last redelegation block height for a delegator.
func (k Keeper) SetLastRedelegationHeight(ctx sdk.Context, delegatorAddr string, height uint64) {
	store := k.getStore(ctx)
	store.Set(types.RedelegationCooldownKey(delegatorAddr), types.Uint64ToBytes(height))
}

// GetLastRedelegationHeight returns the last redelegation block height for a delegator.
func (k Keeper) GetLastRedelegationHeight(ctx sdk.Context, delegatorAddr string) uint64 {
	store := k.getStore(ctx)
	bz := store.Get(types.RedelegationCooldownKey(delegatorAddr))
	if bz == nil {
		return 0
	}
	return types.BytesToUint64(bz)
}

// ---------- Params ----------

// SetParams stores module parameters and syncs tier configs to the KVStore.
func (k Keeper) SetParams(ctx sdk.Context, params *types.Params) {
	store := k.getStore(ctx)
	bz, err := json.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal params: %v", err))
	}
	store.Set(types.ParamsKey, bz)

	// Sync tier configs to KVStore for per-tier lookups.
	for _, tc := range params.TierConfigs {
		k.SetTierConfig(ctx, tc)
	}
}

// GetParams returns the module parameters, falling back to defaults for missing fields.
func (k Keeper) GetParams(ctx sdk.Context) *types.Params {
	store := k.getStore(ctx)
	bz := store.Get(types.ParamsKey)
	if bz == nil {
		return types.DefaultParams()
	}
	var params types.Params
	if err := json.Unmarshal(bz, &params); err != nil {
		return types.DefaultParams()
	}

	// Fill tier configs from KVStore if not in params (backward compat).
	if len(params.TierConfigs) == 0 {
		params.TierConfigs = k.GetAllTierConfigs(ctx)
	}
	if len(params.TierConfigs) == 0 {
		params.TierConfigs = types.DefaultTierConfigs()
	}

	// Fill zero-value reputation deltas from defaults.
	defaults := types.DefaultParams()
	if params.ReputationCorrectDelta == 0 {
		params.ReputationCorrectDelta = defaults.ReputationCorrectDelta
	}
	if params.ReputationIncorrectDelta == 0 {
		params.ReputationIncorrectDelta = defaults.ReputationIncorrectDelta
	}
	if params.ReputationSlashDelta == 0 {
		params.ReputationSlashDelta = defaults.ReputationSlashDelta
	}
	if params.RedelegationCooldownBlocks == 0 {
		params.RedelegationCooldownBlocks = defaults.RedelegationCooldownBlocks
	}

	return &params
}

// ---------- Performance Tracking ----------

// RecordVerification records a verification result and adjusts reputation (P0-2 fix).
func (k Keeper) RecordVerification(ctx sdk.Context, validatorAddr string, correct, contested bool) {
	val, found := k.GetValidator(ctx, validatorAddr)
	if !found {
		return
	}
	params := k.GetParams(ctx)

	val.TotalVerifications++
	if correct {
		val.CorrectVerifications++
		if contested {
			val.ContestedVerificationsCorrect++
			val.ContestedCount++
		}
		// Reputation increase (capped at BPSScale)
		val.ReputationScore += params.ReputationCorrectDelta
		if val.ReputationScore > types.BPSScale {
			val.ReputationScore = types.BPSScale
		}
	} else {
		// Reputation decrease (P0-1 fix: underflow guard)
		if val.ReputationScore >= params.ReputationIncorrectDelta {
			val.ReputationScore -= params.ReputationIncorrectDelta
		} else {
			val.ReputationScore = 0
		}
	}

	// Check for tier transitions.
	newTier, changed := k.CheckTierTransition(ctx, val)
	if changed {
		val.Tier = newTier
	}

	k.SetValidator(ctx, val)
}

// SlashValidator slashes a validator with progressive escalation.
func (k Keeper) SlashValidator(ctx sdk.Context, validatorAddr string, amount *big.Int, reason string) {
	val, found := k.GetValidator(ctx, validatorAddr)
	if !found {
		return
	}
	params := k.GetParams(ctx)

	// R2: per-epoch slash cap
	if params.MaxSlashesPerEpoch > 0 && val.SlashesThisEpoch >= params.MaxSlashesPerEpoch {
		return
	}

	// Progressive escalation: adjusted = amount * (1M + slashCount * escalationBps) / 1M
	escalationFactor := new(big.Int).SetUint64(types.BPSScale + val.SlashCount*params.SlashEscalationBps)
	adjustedAmount := new(big.Int).Mul(amount, escalationFactor)
	adjustedAmount.Div(adjustedAmount, new(big.Int).SetUint64(types.BPSScale))

	// Autopoiesis SSI multiplier
	if k.autopoiesisKeeper != nil {
		sdkCtx := ctx
		multiplier := k.autopoiesisKeeper.GetMultiplier(sdkCtx, "ssi")
		if multiplier != 0 && multiplier != types.BPSScale {
			adjustedAmount.Mul(adjustedAmount, new(big.Int).SetUint64(multiplier))
			adjustedAmount.Div(adjustedAmount, new(big.Int).SetUint64(types.BPSScale))
		}
	}

	// Slash from own stake first, delegated absorbs overflow.
	selfStake, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStake == nil {
		selfStake = new(big.Int)
	}
	slashInt := new(big.Int).Set(adjustedAmount)
	if slashInt.Cmp(selfStake) > 0 {
		slashInt.Set(selfStake)
		// Overflow to delegated
		overflow := new(big.Int).Sub(adjustedAmount, selfStake)
		delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
		if delegated == nil {
			delegated = new(big.Int)
		}
		if overflow.Cmp(delegated) > 0 {
			overflow.Set(delegated)
		}
		delegated.Sub(delegated, overflow)
		val.DelegatedStake = delegated.String()
		slashInt.Add(slashInt, overflow)
	}

	// Route slashed tokens to development fund
	if slashInt.Sign() > 0 {
		slashCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(slashInt)))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "development_fund", slashCoins); err != nil {
			// Abort slash on routing failure
			return
		}

		selfStake.Sub(selfStake, new(big.Int).Sub(slashInt, new(big.Int)))
		if selfStake.Sign() < 0 {
			selfStake.SetInt64(0)
		}
	}

	// R4: only increment slash count if actual slash > 0
	if slashInt.Sign() > 0 {
		val.SlashCount++
		val.SlashesThisEpoch++
		val.LastSlashHeight = uint64(ctx.BlockHeight())
	}

	// Notify emergency module for circuit breaker monitoring.
	if slashInt.Sign() > 0 && k.emergencyKeeper != nil {
		k.emergencyKeeper.RecordSlash(ctx)
	}

	// Update stake values
	selfStakeNew, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStakeNew == nil {
		selfStakeNew = new(big.Int)
	}
	selfStakeNew.Sub(selfStakeNew, slashInt)
	if selfStakeNew.Sign() < 0 {
		selfStakeNew.SetInt64(0)
	}
	val.SelfDelegation = selfStakeNew.String()

	// Recalculate total
	delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegated == nil {
		delegated = new(big.Int)
	}
	total := new(big.Int).Add(selfStakeNew, delegated)
	val.TotalStake = total.String()

	// Reputation decrease
	if val.ReputationScore >= params.ReputationSlashDelta {
		val.ReputationScore -= params.ReputationSlashDelta
	} else {
		val.ReputationScore = 0
	}

	// Deactivate if excessive slashes
	if params.MaxSlashCountDeactivate > 0 && val.SlashCount >= params.MaxSlashCountDeactivate {
		val.IsActive = false
	}

	// Check tier transition
	newTier, changed := k.CheckTierTransition(ctx, val)
	if changed {
		val.Tier = newTier
	}

	k.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.validator_slashed",
		sdk.NewAttribute("validator", validatorAddr),
		sdk.NewAttribute("amount", slashInt.String()),
		sdk.NewAttribute("reason", reason),
	))
}

// SlashValidatorToModule slashes a validator with progressive escalation and routes
// slashed tokens to a specified module account (instead of hardcoded development_fund).
// Returns the actual slashed amount.
func (k Keeper) SlashValidatorToModule(ctx sdk.Context, validatorAddr string, amount *big.Int, destModule string, reason string) *big.Int {
	val, found := k.GetValidator(ctx, validatorAddr)
	if !found {
		return new(big.Int)
	}
	params := k.GetParams(ctx)

	// R2: per-epoch slash cap
	if params.MaxSlashesPerEpoch > 0 && val.SlashesThisEpoch >= params.MaxSlashesPerEpoch {
		return new(big.Int)
	}

	// Progressive escalation: adjusted = amount * (1M + slashCount * escalationBps) / 1M
	escalationFactor := new(big.Int).SetUint64(types.BPSScale + val.SlashCount*params.SlashEscalationBps)
	adjustedAmount := new(big.Int).Mul(amount, escalationFactor)
	adjustedAmount.Div(adjustedAmount, new(big.Int).SetUint64(types.BPSScale))

	// Autopoiesis SSI multiplier
	if k.autopoiesisKeeper != nil {
		sdkCtx := ctx
		multiplier := k.autopoiesisKeeper.GetMultiplier(sdkCtx, "ssi")
		if multiplier != 0 && multiplier != types.BPSScale {
			adjustedAmount.Mul(adjustedAmount, new(big.Int).SetUint64(multiplier))
			adjustedAmount.Div(adjustedAmount, new(big.Int).SetUint64(types.BPSScale))
		}
	}

	// Slash from own stake first, delegated absorbs overflow.
	selfStake, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStake == nil {
		selfStake = new(big.Int)
	}
	slashInt := new(big.Int).Set(adjustedAmount)
	if slashInt.Cmp(selfStake) > 0 {
		slashInt.Set(selfStake)
		// Overflow to delegated
		overflow := new(big.Int).Sub(adjustedAmount, selfStake)
		delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
		if delegated == nil {
			delegated = new(big.Int)
		}
		if overflow.Cmp(delegated) > 0 {
			overflow.Set(delegated)
		}
		delegated.Sub(delegated, overflow)
		val.DelegatedStake = delegated.String()
		slashInt.Add(slashInt, overflow)
	}

	// Route slashed tokens to the specified destination module
	if slashInt.Sign() > 0 {
		slashCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(slashInt)))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, destModule, slashCoins); err != nil {
			// Abort slash on routing failure
			return new(big.Int)
		}
	}

	// R4: only increment slash count if actual slash > 0
	if slashInt.Sign() > 0 {
		val.SlashCount++
		val.SlashesThisEpoch++
		val.LastSlashHeight = uint64(ctx.BlockHeight())
	}

	// Notify emergency module for circuit breaker monitoring.
	if slashInt.Sign() > 0 && k.emergencyKeeper != nil {
		k.emergencyKeeper.RecordSlash(ctx)
	}

	// Update stake values
	selfStakeNew, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStakeNew == nil {
		selfStakeNew = new(big.Int)
	}
	selfStakeNew.Sub(selfStakeNew, slashInt)
	if selfStakeNew.Sign() < 0 {
		selfStakeNew.SetInt64(0)
	}
	val.SelfDelegation = selfStakeNew.String()

	// Recalculate total
	delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegated == nil {
		delegated = new(big.Int)
	}
	total := new(big.Int).Add(selfStakeNew, delegated)
	val.TotalStake = total.String()

	// Reputation decrease
	if val.ReputationScore >= params.ReputationSlashDelta {
		val.ReputationScore -= params.ReputationSlashDelta
	} else {
		val.ReputationScore = 0
	}

	// Deactivate if excessive slashes
	if params.MaxSlashCountDeactivate > 0 && val.SlashCount >= params.MaxSlashCountDeactivate {
		val.IsActive = false
	}

	// Check tier transition
	newTier, changed := k.CheckTierTransition(ctx, val)
	if changed {
		val.Tier = newTier
	}

	k.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.validator_slashed",
		sdk.NewAttribute("validator", validatorAddr),
		sdk.NewAttribute("amount", slashInt.String()),
		sdk.NewAttribute("reason", reason),
		sdk.NewAttribute("dest_module", destModule),
	))

	return slashInt
}

// ---------- Genesis ----------

// InitGenesis initializes the module state from genesis.
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) {
	k.SetParams(ctx, gs.Params)
	for _, val := range gs.Validators {
		k.SetValidator(ctx, val)
	}
	for _, del := range gs.Delegations {
		k.SetDelegation(ctx, del)
	}
	for _, entry := range gs.UnbondingEntries {
		k.SetUnbonding(ctx, entry)
	}
	if gs.UnbondingSeq > 0 {
		k.SetUnbondingSeq(ctx, gs.UnbondingSeq)
	}
}

// ExportGenesis exports the module state.
func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	params := k.GetParams(ctx)

	var validators []*types.Validator
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		validators = append(validators, val)
		return false
	})

	var delegations []*types.Delegation
	k.IterateDelegations(ctx, func(del *types.Delegation) bool {
		delegations = append(delegations, del)
		return false
	})

	var unbondings []*types.UnbondingEntry
	k.IterateUnbondings(ctx, func(entry *types.UnbondingEntry) bool {
		unbondings = append(unbondings, entry)
		return false
	})

	return &types.GenesisState{
		Params:           params,
		Validators:       validators,
		Delegations:      delegations,
		UnbondingEntries: unbondings,
		UnbondingSeq:     k.GetUnbondingSeq(ctx),
	}
}

// ---------- Begin/End Blocker ----------

// BeginBlocker runs at the start of each block: slash decay + unbonding maturation.
func (k Keeper) BeginBlocker(ctx sdk.Context) {
	params := k.GetParams(ctx)
	currentHeight := uint64(ctx.BlockHeight())

	// Slash decay: at every epoch boundary, reset SlashesThisEpoch and decay SlashCount.
	if params.SlashDecayPeriodBlocks > 0 && currentHeight%params.SlashDecayPeriodBlocks == 0 {
		k.IterateValidators(ctx, func(val *types.Validator) bool {
			changed := false
			// R1: decay slash count if no slashes this epoch
			if val.SlashesThisEpoch == 0 && val.SlashCount > 0 {
				val.SlashCount--
				changed = true
			}
			// R2: reset per-epoch counter
			if val.SlashesThisEpoch > 0 {
				val.SlashesThisEpoch = 0
				changed = true
			}
			if changed {
				k.SetValidator(ctx, val)
			}
			return false
		})
	}

	// Process mature unbondings
	mature := k.GetMatureUnbondings(ctx, currentHeight)
	for _, entry := range mature {
		recipientAddr, err := sdk.AccAddressFromBech32(entry.DelegatorAddress)
		if err != nil {
			continue
		}
		amt, ok := new(big.Int).SetString(entry.Amount, 10)
		if !ok || amt.Sign() <= 0 {
			continue
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, recipientAddr, coins); err != nil {
			continue
		}
		entry.Status = "completed"
		k.SetUnbonding(ctx, entry)

		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.staking.unbonding_completed",
			sdk.NewAttribute("delegator", entry.DelegatorAddress),
			sdk.NewAttribute("amount", entry.Amount),
		))
	}
}

// EndBlocker runs at the end of each block: tier advancement checks.
func (k Keeper) EndBlocker(ctx sdk.Context) {
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		newTier, changed := k.CheckTierTransition(ctx, val)
		if changed {
			oldTier := val.Tier
			val.Tier = newTier
			k.SetValidator(ctx, val)
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.staking.validator_tier_changed",
				sdk.NewAttribute("validator", val.OperatorAddress),
				sdk.NewAttribute("old_tier", types.ValidatorTierString(oldTier)),
				sdk.NewAttribute("new_tier", types.ValidatorTierString(newTier)),
			))
		}
		return false
	})
}
