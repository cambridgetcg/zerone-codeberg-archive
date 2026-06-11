package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/billing/types"
)

type queryServer struct {
	types.UnimplementedQueryServer
	Keeper
}

// NewQueryServerImpl returns an implementation of the QueryServer interface.
func NewQueryServerImpl(keeper Keeper) types.QueryServer {
	return &queryServer{Keeper: keeper}
}

var _ types.QueryServer = &queryServer{}

func (k queryServer) Params(goCtx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := k.GetParams(ctx)
	return &types.QueryParamsResponse{Params: params}, nil
}

func (k queryServer) Provider(goCtx context.Context, req *types.QueryProviderRequest) (*types.QueryProviderResponse, error) {
	if req == nil || req.Address == "" {
		return nil, fmt.Errorf("address is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, found := k.GetProvider(ctx, req.Address)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotFound, req.Address)
	}
	return &types.QueryProviderResponse{Provider: provider}, nil
}

func (k queryServer) Providers(goCtx context.Context, req *types.QueryProvidersRequest) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	providers := k.GetAllProviders(ctx)
	return &types.QueryProvidersResponse{Providers: providers}, nil
}

func (k queryServer) Quote(goCtx context.Context, req *types.QueryQuoteRequest) (*types.QueryQuoteResponse, error) {
	if req == nil || req.FactId == "" {
		return nil, fmt.Errorf("fact_id is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentBlock := uint64(ctx.BlockHeight())

	totalPrice, breakdown := k.CalculateQueryPrice(ctx, []string{req.FactId}, currentBlock)

	var quote *types.QueryQuote
	if len(breakdown) > 0 {
		quote = &types.QueryQuote{
			FactId:         req.FactId,
			BasePrice:      breakdown[0].BaseCost,
			EffectivePrice: breakdown[0].TotalPrice,
		}
	} else {
		quote = &types.QueryQuote{
			FactId:         req.FactId,
			EffectivePrice: totalPrice.String(),
		}
	}

	return &types.QueryQuoteResponse{Quote: quote}, nil
}

func (k queryServer) BatchQuote(goCtx context.Context, req *types.QueryBatchQuoteRequest) (*types.QueryBatchQuoteResponse, error) {
	if req == nil || len(req.FactIds) == 0 {
		return nil, fmt.Errorf("at least one fact_id is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentBlock := uint64(ctx.BlockHeight())

	totalPrice, breakdown := k.CalculateQueryPrice(ctx, req.FactIds, currentBlock)

	var quotes []*types.QueryQuote
	for _, b := range breakdown {
		quotes = append(quotes, &types.QueryQuote{
			FactId:         b.FactId,
			BasePrice:      b.BaseCost,
			EffectivePrice: b.TotalPrice,
		})
	}

	return &types.QueryBatchQuoteResponse{
		Quotes:     quotes,
		TotalPrice: totalPrice.String(),
	}, nil
}

func (k queryServer) ZRNPriceUSD(goCtx context.Context, req *types.QueryZRNPriceUSDRequest) (*types.QueryZRNPriceUSDResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := k.GetParams(ctx)
	cfg := params.DynamicPricing
	if cfg == nil {
		cfg = types.DefaultDynamicPricingConfig()
	}

	price := k.getZRNPriceUSD(ctx, cfg)

	source := "unavailable"
	if price.Sign() > 0 {
		// Determine source
		if cfg.ManualZrnPriceUsd != "" && cfg.ManualZrnPriceUsd != "0" {
			manual := new(big.Int)
			if _, ok := manual.SetString(cfg.ManualZrnPriceUsd, 10); ok && manual.Cmp(price) == 0 {
				source = "manual"
			} else {
				source = "twap"
			}
		} else {
			source = "twap"
		}
	}

	return &types.QueryZRNPriceUSDResponse{
		PriceUsd: price.String(),
		Source:   source,
	}, nil
}

// ProtocolReserves returns a unified snapshot of all protocol fund balances.
// All amounts are in uzrn. This endpoint is governance-facing: it aggregates
// the seven main protocol fund accounts into a single response so governance
// participants can assess the full financial picture without issuing seven
// separate balance queries.
//
// NOTE: "treasury_protocol" and "protocol_treasury" are distinct module accounts
// due to an existing naming inconsistency between the billing and knowledge routing
// paths. Both are included so governance can see the complete picture.
func (k queryServer) ProtocolReserves(goCtx context.Context, req *types.QueryProtocolReservesRequest) (*types.QueryProtocolReservesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	denom := "uzrn"

	get := func(module string) string {
		return k.bankKeeper.GetModuleBalance(ctx, module, denom).Amount.String()
	}

	return &types.QueryProtocolReservesResponse{
		TreasuryProtocolUzrn:  get("treasury_protocol"),
		ProtocolTreasuryUzrn:  get("protocol_treasury"),
		ResearchFundUzrn:      get("research_fund"),
		DevelopmentFundUzrn:   get("development_fund"),
		VindicationEscrowUzrn: get("vindication_escrow"),
		KnowledgePoolUzrn:     get("knowledge"),
		DisputesEscrowUzrn:    get("disputes"),
		SnapshotBlockHeight:   uint64(ctx.BlockHeight()),
	}, nil
}
