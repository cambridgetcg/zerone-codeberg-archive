package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// twapScale is the precision scale for TWAP accumulators (1e12).
var twapScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil)

// UpdateTWAPAccumulator records the current spot prices into the TWAP accumulator.
// Called at each block via BeginBlock for all active pools.
func (k Keeper) UpdateTWAPAccumulator(ctx sdk.Context, pool *types.Pool) {
	currentBlock := uint64(ctx.BlockHeight())

	acc, found := k.GetTWAPAccumulator(ctx, pool.PoolId)
	if !found {
		acc = &types.TWAPAccumulator{
			PoolId:       pool.PoolId,
			LastBlock:    currentBlock,
			CumPriceAToB: "0",
			CumPriceBToA: "0",
		}
		k.SetTWAPAccumulator(ctx, acc)
		return // initialized; no delta to accumulate yet
	}

	if currentBlock <= acc.LastBlock {
		return // already updated this block
	}

	blocksDelta := currentBlock - acc.LastBlock

	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)

	if reserveA.Sign() <= 0 || reserveB.Sign() <= 0 {
		return
	}

	// cumPriceAToB += (reserveB / reserveA) * blocksDelta, scaled by 1e12
	priceAtoB := new(big.Int).Mul(reserveB, twapScale)
	priceAtoB.Div(priceAtoB, reserveA)
	priceAtoB.Mul(priceAtoB, new(big.Int).SetUint64(blocksDelta))

	priceBtoA := new(big.Int).Mul(reserveA, twapScale)
	priceBtoA.Div(priceBtoA, reserveB)
	priceBtoA.Mul(priceBtoA, new(big.Int).SetUint64(blocksDelta))

	cumAtoB := new(big.Int)
	cumAtoB.SetString(acc.CumPriceAToB, 10)
	cumAtoB.Add(cumAtoB, priceAtoB)

	cumBtoA := new(big.Int)
	cumBtoA.SetString(acc.CumPriceBToA, 10)
	cumBtoA.Add(cumBtoA, priceBtoA)

	acc.CumPriceAToB = cumAtoB.String()
	acc.CumPriceBToA = cumBtoA.String()
	acc.LastBlock = currentBlock

	k.SetTWAPAccumulator(ctx, acc)
}

// GetTWAP computes the time-weighted average price over a window of blocks.
// Returns the TWAP for baseDenom (price of baseDenom in terms of quote), scaled by 1e6.
// Falls back to spot price if insufficient history.
func (k Keeper) GetTWAP(ctx sdk.Context, poolId string, baseDenom string, window uint64) (*big.Int, uint64, error) {
	pool, found := k.GetPool(ctx, poolId)
	if !found {
		return nil, 0, types.ErrPoolNotFound
	}

	acc, found := k.GetTWAPAccumulator(ctx, poolId)
	if !found {
		// No TWAP history — return spot price scaled by 1e6
		return k.getSpotPrice(pool, baseDenom)
	}

	currentBlock := uint64(ctx.BlockHeight())
	if window == 0 {
		params := k.GetParams(ctx)
		window = params.TwapWindowBlocks
	}

	// TWAP is the accumulator delta / block delta
	startBlock := acc.LastBlock
	if currentBlock > window && startBlock < currentBlock-window {
		startBlock = currentBlock - window
	}
	actualWindow := currentBlock - startBlock
	if actualWindow == 0 {
		return k.getSpotPrice(pool, baseDenom)
	}

	// Use the current accumulator value divided by blocks elapsed
	var cumPrice *big.Int
	if baseDenom == pool.DenomA {
		cumPrice = new(big.Int)
		cumPrice.SetString(acc.CumPriceAToB, 10)
	} else {
		cumPrice = new(big.Int)
		cumPrice.SetString(acc.CumPriceBToA, 10)
	}

	if cumPrice.Sign() == 0 {
		return k.getSpotPrice(pool, baseDenom)
	}

	// TWAP = cumPrice * 1e6 / (blocksDelta * twapScale)
	blocksDelta := acc.LastBlock // total blocks accumulated
	if blocksDelta == 0 {
		return k.getSpotPrice(pool, baseDenom)
	}

	scale1e6 := big.NewInt(1_000_000)
	twap := new(big.Int).Mul(cumPrice, scale1e6)
	divisor := new(big.Int).Mul(new(big.Int).SetUint64(blocksDelta), twapScale)
	twap.Div(twap, divisor)

	return twap, actualWindow, nil
}

// getSpotPrice returns spot price of baseDenom in quote terms, scaled by 1e6.
func (k Keeper) getSpotPrice(pool *types.Pool, baseDenom string) (*big.Int, uint64, error) {
	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)

	if reserveA.Sign() == 0 || reserveB.Sign() == 0 {
		return big.NewInt(0), 0, nil
	}

	scale := big.NewInt(1_000_000)
	var price *big.Int
	if baseDenom == pool.DenomA {
		// price = reserveB * 1e6 / reserveA
		price = new(big.Int).Mul(reserveB, scale)
		price.Div(price, reserveA)
	} else {
		// price = reserveA * 1e6 / reserveB
		price = new(big.Int).Mul(reserveA, scale)
		price.Div(price, reserveB)
	}
	return price, 0, nil
}

// GetTWAPSafe is the staleness-aware variant of GetTWAP.
//
// It returns ErrTWAPStale if the TWAP accumulator has not been updated within
// maxStalenessBlocks of the current block. Callers should use GetTWAPSafe in
// preference to GetTWAP whenever they need a freshness guarantee — this
// centralises the staleness check so individual consumers cannot accidentally
// read a stale price.
//
// If maxStalenessBlocks is 0 the check is skipped (equivalent to GetTWAP).
func (k Keeper) GetTWAPSafe(ctx sdk.Context, poolId string, baseDenom string, window uint64, maxStalenessBlocks uint64) (*big.Int, uint64, error) {
	if maxStalenessBlocks > 0 {
		acc, found := k.GetTWAPAccumulator(ctx, poolId)
		if found {
			currentBlock := uint64(ctx.BlockHeight())
			if currentBlock > acc.LastBlock && currentBlock-acc.LastBlock > maxStalenessBlocks {
				return nil, 0, types.ErrTWAPStale
			}
		}
		// If no accumulator exists yet, fall through to GetTWAP which returns spot price.
	}
	return k.GetTWAP(ctx, poolId, baseDenom, window)
}

// GetSpotPrice returns the spot price for external use (e.g., billing module).
func (k Keeper) GetSpotPrice(ctx sdk.Context, poolId, baseDenom string) (*big.Int, error) {
	pool, found := k.GetPool(ctx, poolId)
	if !found {
		return nil, types.ErrPoolNotFound
	}
	price, _, err := k.getSpotPrice(pool, baseDenom)
	return price, err
}
