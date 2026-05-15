package keeper

import (
	"encoding/json"
	"fmt"
	"math/big"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// Keeper manages the liquiditypool module's state.
type Keeper struct {
	cdc              codec.Codec
	storeService     store.KVStoreService
	bankKeeper       types.BankKeeper
	authority        string
	emergencyKeeper  types.EmergencyKeeper // optional; nil until SetEmergencyKeeper called
}

// NewKeeper creates a new liquiditypool module Keeper.
func NewKeeper(
	cdc codec.Codec,
	storeService store.KVStoreService,
	bk types.BankKeeper,
	authority string,
) Keeper {
	return Keeper{
		cdc:          cdc,
		storeService: storeService,
		bankKeeper:   bk,
		authority:    authority,
	}
}

// SetEmergencyKeeper wires the optional emergency keeper for the oracle
// staleness circuit breaker (P-21b). Call this from app.go after both keepers
// are constructed. The liquiditypool keeper is nil-safe: if never called, the
// circuit breaker simply does not receive oracle liveness signals.
func (k *Keeper) SetEmergencyKeeper(ek types.EmergencyKeeper) {
	k.emergencyKeeper = ek
}

func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// --- Params ---

func (k Keeper) GetParams(ctx sdk.Context) *types.Params {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.ParamsKey)
	if err != nil || bz == nil {
		return types.DefaultParams()
	}
	var params types.Params
	if err := proto.Unmarshal(bz, &params); err != nil {
		return types.DefaultParams()
	}
	return &params
}

func (k Keeper) SetParams(ctx sdk.Context, params *types.Params) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal params: %v", err))
	}
	if err := kvStore.Set(types.ParamsKey, bz); err != nil {
		panic(fmt.Sprintf("failed to set params: %v", err))
	}
}

// --- Genesis ---

func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) {
	if gs.Params != nil {
		k.SetParams(ctx, gs.Params)
	}
	for _, pool := range gs.Pools {
		if pool != nil {
			k.SetPool(ctx, pool)
		}
	}
	for _, acc := range gs.TwapAccumulators {
		if acc != nil {
			k.SetTWAPAccumulator(ctx, acc)
		}
	}
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	var pools []*types.Pool
	k.IteratePools(ctx, func(p *types.Pool) bool {
		pools = append(pools, p)
		return false
	})
	var accs []*types.TWAPAccumulator
	k.IterateTWAPAccumulators(ctx, func(a *types.TWAPAccumulator) bool {
		accs = append(accs, a)
		return false
	})
	return &types.GenesisState{
		Params:           k.GetParams(ctx),
		Pools:            pools,
		TwapAccumulators: accs,
	}
}

func (k Keeper) ExportGenesisJSON(ctx sdk.Context) json.RawMessage {
	gs := k.ExportGenesis(ctx)
	bz, err := json.Marshal(gs)
	if err != nil {
		panic("failed to marshal genesis: " + err.Error())
	}
	return bz
}

// --- Pool Counter ---

func (k Keeper) GetNextPoolId(ctx sdk.Context) uint64 {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.PoolCounterKey)
	if err != nil || bz == nil {
		return 1
	}
	counter := new(big.Int).SetBytes(bz)
	return counter.Uint64()
}

func (k Keeper) IncrementPoolCounter(ctx sdk.Context) uint64 {
	current := k.GetNextPoolId(ctx)
	next := current + 1
	kvStore := k.storeService.OpenKVStore(ctx)
	bz := new(big.Int).SetUint64(next).Bytes()
	if err := kvStore.Set(types.PoolCounterKey, bz); err != nil {
		panic(fmt.Sprintf("failed to set pool counter: %v", err))
	}
	return current
}

// --- Cross-module price oracle ---

// GetZRNPrice returns the current ZRN price in the quote denom, scaled by 1e6.
// Returns (price, error). Price = reserve_quote * 1e6 / reserve_zrn.
func (k Keeper) GetZRNPrice(ctx sdk.Context, quoteDenom string) (sdkmath.Int, error) {
	pool := k.GetPoolByDenoms(ctx, "uzrn", quoteDenom)
	if pool == nil {
		return sdkmath.ZeroInt(), types.ErrNoPool
	}
	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)

	if reserveA.Sign() == 0 {
		return sdkmath.ZeroInt(), fmt.Errorf("zero reserve for uzrn")
	}

	// Determine which reserve is uzrn
	var zrnReserve, quoteReserve *big.Int
	if pool.DenomA == "uzrn" {
		zrnReserve = reserveA
		quoteReserve = reserveB
	} else {
		zrnReserve = reserveB
		quoteReserve = reserveA
	}

	// price = quoteReserve * 1e6 / zrnReserve
	price := new(big.Int).Mul(quoteReserve, big.NewInt(1_000_000))
	price.Div(price, zrnReserve)
	return sdkmath.NewIntFromBigInt(price), nil
}

// prefixEndBytes returns the end key for a prefix scan.
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
