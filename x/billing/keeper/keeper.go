package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/billing/types"
)

// Keeper manages the billing module's state.
type Keeper struct {
	storeService store.KVStoreService
	cdc          codec.BinaryCodec
	authority    string

	bankKeeper              types.BankKeeper
	knowledgeKeeper         types.KnowledgeKeeper
	researchFundDepositor   types.ResearchFundDepositor
	liquidityPoolKeeper     types.LiquidityPoolKeeper     // nil-safe, set post-init
	externalPriceFeedKeeper types.ExternalPriceFeedKeeper // nil-safe, set post-init
}

// NewKeeper creates a new billing module Keeper.
func NewKeeper(
	storeService store.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	bk types.BankKeeper,
	kk types.KnowledgeKeeper,
	rfd types.ResearchFundDepositor,
) Keeper {
	if rfd == nil {
		panic("billing: ResearchFundDepositor is required")
	}
	return Keeper{
		storeService:          storeService,
		cdc:                   cdc,
		authority:             authority,
		bankKeeper:            bk,
		knowledgeKeeper:       kk,
		researchFundDepositor: rfd,
	}
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

// SetLiquidityPoolKeeper sets the liquidity pool keeper post-initialization.
func (k *Keeper) SetLiquidityPoolKeeper(lpk types.LiquidityPoolKeeper) {
	k.liquidityPoolKeeper = lpk
}

// SetExternalPriceFeedKeeper sets the external price feed keeper post-initialization.
// When set and ExternalPriceFeedEnabled is true in DynamicPricingConfig, this
// keeper is consulted as the third source in the 3-source weighted median.
func (k *Keeper) SetExternalPriceFeedKeeper(epfk types.ExternalPriceFeedKeeper) {
	k.externalPriceFeedKeeper = epfk
}

// GetZRNPriceUSD returns the current ZRN price in micro-USD (6-decimal).
// Public wrapper for cross-module use. Returns 0 if no price is available.
func (k Keeper) GetZRNPriceUSD(ctx context.Context) uint64 {
	params := k.GetParams(ctx)
	cfg := params.DynamicPricing
	if cfg == nil {
		cfg = types.DefaultDynamicPricingConfig()
	}
	price := k.getZRNPriceUSD(ctx, cfg)
	if price == nil || price.Sign() <= 0 {
		return 0
	}
	if !price.IsUint64() {
		return ^uint64(0) // math.MaxUint64
	}
	return price.Uint64()
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

// ---------- Params ----------

// SetParams sets module parameters.
func (k Keeper) SetParams(ctx context.Context, params *types.Params) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal params: %v", err))
	}
	_ = kvStore.Set(types.ParamsKey, bz)
}

// GetParams returns module parameters.
func (k Keeper) GetParams(ctx context.Context) *types.Params {
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

// ---------- Genesis ----------

// InitGenesis initializes the module's state from genesis.
func (k Keeper) InitGenesis(ctx context.Context, genState *types.GenesisState) {
	if genState.Params != nil {
		k.SetParams(ctx, genState.Params)
	}
	for _, provider := range genState.Providers {
		k.SetProvider(ctx, provider)
	}
}

// ExportGenesis exports the module's state.
func (k Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	params := k.GetParams(ctx)
	providers := k.GetAllProviders(ctx)
	return &types.GenesisState{
		Params:    params,
		Providers: providers,
	}
}
