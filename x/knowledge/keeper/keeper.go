package keeper

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// Keeper holds module state for the knowledge module.
type Keeper struct {
	storeService store.KVStoreService
	cdc          codec.BinaryCodec
	authority    string // governance authority address

	// External keeper dependencies (core — set at construction)
	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper

	// External keeper dependencies (post-init setters to break circular deps)
	ontologyKeeper            types.OntologyKeeper
	vestingRewardsKeeper      types.VestingRewardsKeeper
	domainQualificationKeeper types.DomainQualificationKeeper // nil until R6-5
	autopoiesisKeeper         types.AutopoiesisKeeper         // nil until R7-1
	partnershipKeeper         types.PartnershipKeeper         // nil until R26-4
	zeroneAuthKeeper           types.ZeroneAuthKeeper           // nil until R28-5
	captureDefenseKeeper       types.CaptureDefenseKeeper       // nil until R28-8
	pacingKeeper               types.PacingKeeper               // nil until R29-6
	counterexampleKeeper       types.CounterexampleKeeper       // nil until counterexamples wired
}

// NewKeeper creates a new knowledge Keeper.
func NewKeeper(
	storeService store.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
) Keeper {
	return Keeper{
		storeService:  storeService,
		cdc:           cdc,
		authority:     authority,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,
	}
}

// GetAuthority returns the module's governance authority address.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// GetStakingKeeper returns the staking keeper dependency.
func (k Keeper) GetStakingKeeper() types.StakingKeeper {
	return k.stakingKeeper
}

// SetOntologyKeeper sets the ontology keeper (post-init to break circular dep).
func (k *Keeper) SetOntologyKeeper(ok types.OntologyKeeper) {
	k.ontologyKeeper = ok
}

// SetVestingRewardsKeeper sets the vesting rewards keeper (post-init).
func (k *Keeper) SetVestingRewardsKeeper(vk types.VestingRewardsKeeper) {
	k.vestingRewardsKeeper = vk
}

// SetDomainQualificationKeeper sets the domain qualification keeper (post-init, R6-5).
func (k *Keeper) SetDomainQualificationKeeper(dk types.DomainQualificationKeeper) {
	k.domainQualificationKeeper = dk
}

// SetAutopoiesisKeeper sets the autopoiesis keeper (post-init, R7-1).
func (k *Keeper) SetAutopoiesisKeeper(ak types.AutopoiesisKeeper) {
	k.autopoiesisKeeper = ak
}

// SetPartnershipKeeper sets the partnership keeper (post-init, R26-4).
func (k *Keeper) SetPartnershipKeeper(pk types.PartnershipKeeper) {
	k.partnershipKeeper = pk
}

// SetZeroneAuthKeeper sets the zerone auth keeper (post-init, R28-5).
func (k *Keeper) SetZeroneAuthKeeper(ak types.ZeroneAuthKeeper) {
	k.zeroneAuthKeeper = ak
}

// SetCaptureDefenseKeeper sets the capture defense keeper post-initialization.
func (k *Keeper) SetCaptureDefenseKeeper(cdk types.CaptureDefenseKeeper) {
	k.captureDefenseKeeper = cdk
}

// SetCounterexampleKeeper wires the counterexamples module so TVW
// can apply the alignment-by-structure multiplier (commitment 15).
func (k *Keeper) SetCounterexampleKeeper(ck types.CounterexampleKeeper) {
	k.counterexampleKeeper = ck
}

// SetPacingKeeper sets the pacing keeper for adaptive timing (R29-6).
func (k *Keeper) SetPacingKeeper(pk types.PacingKeeper) {
	k.pacingKeeper = pk
}

// Logger returns a module-scoped logger.
func (k Keeper) Logger(ctx context.Context) log.Logger {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return sdkCtx.Logger().With("module", "x/"+types.ModuleName)
}

// MintWithCap mints ZRN tokens into the target module account, capped at
// the global MaxSupplyUzrn covenant (222,222,222 ZRN = 222,222,222,000,000 uzrn).
//
// This enforces the same supply cap as x/vesting_rewards.MintWithCap, closing
// the gap identified in P-18b-b: three knowledge-module mint paths (probe bounty
// pool, training fund disbursement, contribution challenge reward) previously
// called bankKeeper.MintCoins directly without checking the global cap.
//
// Returns the actual amount minted (may be less than requested when cap is near).
// Returns (0, nil) — not an error — when the cap is already exhausted.
// The caller decides whether to proceed with partial funding or return an error.
func (k Keeper) MintWithCap(ctx context.Context, moduleName string, amount *big.Int) (*big.Int, error) {
	if k.bankKeeper == nil || amount == nil || amount.Sign() <= 0 {
		return new(big.Int), nil
	}

	maxSupply := new(big.Int)
	maxSupply.SetString(types.MaxSupplyUzrn, 10)

	supply := k.bankKeeper.GetSupply(ctx, "uzrn")
	currentSupply := supply.Amount.BigInt()

	remaining := new(big.Int).Sub(maxSupply, currentSupply)
	if remaining.Sign() <= 0 {
		return new(big.Int), nil // supply cap exhausted — not an error, just no new tokens
	}

	actual := new(big.Int).Set(amount)
	if actual.Cmp(remaining) > 0 {
		actual.Set(remaining) // clamp to available headroom
	}

	mintCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(actual)))
	if err := k.bankKeeper.MintCoins(ctx, moduleName, mintCoins); err != nil {
		return nil, fmt.Errorf("mintWithCap: mint to %s failed: %w", moduleName, err)
	}

	return actual, nil
}

// IncreaseVerificationThreshold temporarily requires more verifiers for a domain.
// The override is stored as 20 bytes: additionalVerifiers (4) + expiryHeight (8) + createdAt (8).
func (k Keeper) IncreaseVerificationThreshold(ctx context.Context, domain string, additionalVerifiers uint32, expiryHeight uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kvStore := k.storeService.OpenKVStore(ctx)

	key := append(append([]byte{}, types.VerificationThresholdOverrideKeyPrefix...), []byte(domain)...)
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[0:4], additionalVerifiers)
	binary.BigEndian.PutUint64(buf[4:12], expiryHeight)
	binary.BigEndian.PutUint64(buf[12:20], uint64(sdkCtx.BlockHeight()))
	return kvStore.Set(key, buf)
}
