package keeper

import (
	"context"
	"encoding/binary"
	"math/big"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/billing/types"
	"github.com/zerone-chain/zerone/x/common/pricing"
)

// uzrnPerZRN is 1,000,000 — the conversion factor from ZRN to uzrn.
var uzrnPerZRN = big.NewInt(1_000_000)

// priceChangeThresholdBps is the minimum price change (in basis points of 1M)
// required to emit a new oracle_price_update event. 50_000 = 5%.
const priceChangeThresholdBps = 50_000

// getZRNPriceUSD returns the ZRN price in 6-decimal USD using a weighted median
// of up to three independent sources:
//
//  1. TWAP from the liquidity pool keeper (manipulation-resistant but ZRN-circular)
//  2. External price feed (independent off-chain oracle, reduces circular dependency)
//  3. Manual governance override (emergency fallback, full governance control)
//
// The weighted median is computed from whichever sources return valid prices.
// With all three sources active, single-source manipulation cannot shift the
// median — an attacker must corrupt 2/3 sources simultaneously.
//
// Returns zero when no source is available → caller falls back to fixed BaseQueryPrice.
func (k Keeper) getZRNPriceUSD(ctx context.Context, cfg *types.DynamicPricingConfig) *big.Int {
	var sources []*big.Int

	// Source 1: TWAP from liquidity pool
	if k.liquidityPoolKeeper != nil {
		twap, err := k.liquidityPoolKeeper.GetTWAP(ctx, "uzrn", cfg.TwapWindowBlocks)
		if err == nil && twap != nil && twap.Sign() > 0 {
			lastUpdate := k.liquidityPoolKeeper.GetLastPriceUpdateHeight(ctx)
			sdkCtx := extractBlockHeight(ctx)
			if sdkCtx <= lastUpdate+cfg.StalenessBlocks {
				sources = append(sources, twap)
			}
		}
	}

	// Source 2: External price feed (when enabled and keeper wired)
	if cfg.ExternalPriceFeedEnabled && cfg.ExternalPriceFeedAddress != "" && k.externalPriceFeedKeeper != nil {
		extPrice, err := k.externalPriceFeedKeeper.GetLatestPrice(ctx, cfg.ExternalPriceFeedAddress)
		if err == nil && extPrice != nil && extPrice.Sign() > 0 {
			sources = append(sources, extPrice)
		}
	}

	// Source 3: Manual governance override
	if cfg.ManualZrnPriceUsd != "" && cfg.ManualZrnPriceUsd != "0" {
		manual := new(big.Int)
		if _, ok := manual.SetString(cfg.ManualZrnPriceUsd, 10); ok && manual.Sign() > 0 {
			sources = append(sources, manual)
		}
	}

	price := WeightedMedianPrice(sources)
	if price == nil || price.Sign() <= 0 {
		return big.NewInt(0)
	}

	// Determine source label for events.
	var sourceLabel string
	switch len(sources) {
	case 1:
		if cfg.ManualZrnPriceUsd != "" && cfg.ManualZrnPriceUsd != "0" {
			sourceLabel = "manual"
		} else if cfg.ExternalPriceFeedEnabled && len(sources) > 0 {
			sourceLabel = "external"
		} else {
			sourceLabel = "twap"
		}
	default:
		sourceLabel = "median"
	}
	k.maybeEmitPriceEvent(ctx, price, sourceLabel)
	return price
}

// WeightedMedianPrice returns the median of the provided price sources.
//
// Exported for testing and future calibration. With equal weights
// (current implementation), the median is the middle value when sorted.
// For even-count slices, the lower of the two middle values is returned
// (conservative: errs toward lower prices, protecting buyers).
//
// The median of N values requires an attacker to corrupt ceil(N/2) sources —
// with N=3, two sources must be compromised simultaneously.
func WeightedMedianPrice(sources []*big.Int) *big.Int {
	if len(sources) == 0 {
		return nil
	}
	if len(sources) == 1 {
		return sources[0]
	}

	// Sort ascending (simple insertion sort — N is always ≤3 in practice).
	sorted := make([]*big.Int, len(sources))
	copy(sorted, sources)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Cmp(sorted[j]) > 0 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Middle value. For even count, use lower middle (conservative).
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		mid-- // lower of the two middle values
	}
	return sorted[mid]
}

// calculateDynamicBaseCost computes the per-fact base cost in uzrn using
// the dynamic pricing engine.
//
// Formula: baseCostUzrn = targetCostUSD * 1_000_000 / zrnPriceUSD
// Result is clamped to [MinCostPerFact, MaxCostPerFact].
// Returns 0 if no price is available → signals fallback to fixed BaseQueryPrice.
func (k Keeper) calculateDynamicBaseCost(ctx context.Context) *big.Int {
	params := k.GetParams(ctx)
	cfg := params.DynamicPricing
	if cfg == nil || !cfg.Enabled {
		return big.NewInt(0)
	}

	zrnPrice := k.getZRNPriceUSD(ctx, cfg)
	if zrnPrice.Sign() == 0 {
		return big.NewInt(0) // no price → fallback
	}

	targetCost := new(big.Int)
	if _, ok := targetCost.SetString(cfg.TargetQueryCostUsd, 10); !ok || targetCost.Sign() <= 0 {
		return big.NewInt(0)
	}

	// Fast path: delegate to shared utility when all values fit uint64
	if zrnPrice.IsUint64() && targetCost.IsUint64() {
		minCost, _ := strconv.ParseUint(cfg.MinCostPerFact, 10, 64)
		maxCost, _ := strconv.ParseUint(cfg.MaxCostPerFact, 10, 64)
		if minCost > 0 && maxCost > 0 {
			result := pricing.CalculateUSDStablePrice(
				targetCost.Uint64(), zrnPrice.Uint64(), minCost, maxCost,
			)
			if result > 0 {
				return new(big.Int).SetUint64(result)
			}
		}
	}

	// Fallback: big.Int arithmetic for extreme values
	baseCost := new(big.Int).Mul(targetCost, uzrnPerZRN)
	baseCost.Div(baseCost, zrnPrice)

	// Clamp to [min, max]
	minCost := new(big.Int)
	minCost.SetString(cfg.MinCostPerFact, 10)

	maxCost := new(big.Int)
	maxCost.SetString(cfg.MaxCostPerFact, 10)

	if baseCost.Cmp(minCost) < 0 {
		baseCost.Set(minCost)
	}
	if baseCost.Cmp(maxCost) > 0 {
		baseCost.Set(maxCost)
	}

	return baseCost
}

// ---------- Price feed events ----------

// maybeEmitPriceEvent emits an oracle_price_update event if the price has
// changed more than 5% from the last emitted price.
func (k Keeper) maybeEmitPriceEvent(ctx context.Context, newPrice *big.Int, source string) {
	if newPrice == nil || newPrice.Sign() <= 0 {
		return
	}

	lastPrice := k.getLastEmittedPrice(ctx)

	// Always emit on first observation (no previous price stored)
	if lastPrice == 0 {
		k.emitPriceEvent(ctx, newPrice, source)
		k.setLastEmittedPrice(ctx, newPrice.Uint64())
		return
	}

	// Check if change exceeds threshold (5%)
	newU64 := newPrice.Uint64()
	var diff uint64
	if newU64 > lastPrice {
		diff = newU64 - lastPrice
	} else {
		diff = lastPrice - newU64
	}

	// diff / lastPrice > 5%  ⟺  diff * 1_000_000 / lastPrice > 50_000
	changeBps := diff * 1_000_000 / lastPrice
	if changeBps > priceChangeThresholdBps {
		k.emitPriceEvent(ctx, newPrice, source)
		k.setLastEmittedPrice(ctx, newU64)
	}
}

// emitPriceEvent emits the oracle_price_update SDK event.
func (k Keeper) emitPriceEvent(ctx context.Context, price *big.Int, source string) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.billing.oracle_price_update",
		sdk.NewAttribute("denom", "uzrn"),
		sdk.NewAttribute("price_usd", price.String()),
		sdk.NewAttribute("source", source),
	))
}

// getLastEmittedPrice reads the last emitted price from the KV store.
// Returns 0 if no price has been emitted yet.
func (k Keeper) getLastEmittedPrice(ctx context.Context) uint64 {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.LastEmittedPriceKey)
	if err != nil || bz == nil || len(bz) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// setLastEmittedPrice writes the last emitted price to the KV store.
func (k Keeper) setLastEmittedPrice(ctx context.Context, price uint64) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, price)
	_ = kvStore.Set(types.LastEmittedPriceKey, bz)
}

// extractBlockHeight extracts the block height from context.
func extractBlockHeight(ctx context.Context) uint64 {
	// Use SDK context to get block height
	sdkCtx, ok := ctx.(interface{ BlockHeight() int64 })
	if ok {
		return uint64(sdkCtx.BlockHeight())
	}
	return 0
}
