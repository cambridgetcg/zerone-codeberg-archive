package keeper

import (
	"context"
	"encoding/binary"
	"math/big"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

const pacingBPS = uint64(1_000_000)

// GetEffectiveCooldown returns the claim cooldown adjusted by global health pacing
// and domain carrying capacity pressure.
//
// When the network is healthy (creationBps == 1,000,000), cooldown equals the base param.
// When degraded (creationBps < 1,000,000), cooldown increases proportionally — fewer claims
// are accepted per unit time, matching reduced network throughput.
// Domain pressure can only tighten (increase) the cooldown further.
func (k Keeper) GetEffectiveCooldown(ctx context.Context, domain string) uint64 {
	params, err := k.GetParams(ctx)
	if err != nil {
		return 50 // safe fallback
	}
	baseCooldown := params.ClaimCooldownBlocks

	effectiveCooldown := baseCooldown
	if k.pacingKeeper != nil {
		creationPacing, _ := k.pacingKeeper.GetGlobalPacingMultiplier(ctx)
		if creationPacing > 0 && creationPacing != pacingBPS {
			// Inverse relationship: lower pacing → higher cooldown.
			// e.g. creationPacing = 750_000 (75%) → cooldown = base * 1_000_000 / 750_000 ≈ 133%
			effectiveCooldown = safeMulDiv(baseCooldown, pacingBPS, creationPacing)
		}
	}

	// Domain carrying capacity override: can only tighten (increase) cooldown.
	pressure := k.GetDomainPressure(ctx, domain)
	if pressure > pacingBPS {
		domainCooldown := safeMulDiv(effectiveCooldown, pressure, pacingBPS)
		if domainCooldown > effectiveCooldown {
			effectiveCooldown = domainCooldown
		}
	}

	return effectiveCooldown
}

// GetEffectiveMinReviewFee returns the minimum review fee adjusted by pacing
// and congestion.
//
// Two independent adjustments may increase the fee above the base MinReviewFee:
//
//  1. Pacing: when the network is degraded (creationBps < 1,000,000), the fee
//     rises inversely to discourage low-value submissions during constrained
//     periods. Example: 75% pacing → 133% of base fee.
//
//  2. Congestion: when active verification rounds exceed CongestionThresholdRounds,
//     each excess round adds CongestionMultiplierBps/1M to the fee multiplier.
//     Applied on top of the pacing-adjusted fee. Example: threshold=100,
//     active=150, multiplierBps=10_000 → excess=50, +50% of pacing-adjusted fee.
//     Disabled when CongestionMultiplierBps == 0 or CongestionThresholdRounds == 0.
func (k Keeper) GetEffectiveMinReviewFee(ctx context.Context) string {
	params, err := k.GetParams(ctx)
	if err != nil {
		return "100000" // safe fallback
	}
	baseStr := params.MinReviewFee
	fee, ok := new(big.Int).SetString(baseStr, 10)
	if !ok || fee.Sign() <= 0 {
		return baseStr
	}

	// Layer 1: pacing adjustment (existing behaviour).
	if k.pacingKeeper != nil {
		creationPacing, _ := k.pacingKeeper.GetGlobalPacingMultiplier(ctx)
		if creationPacing > 0 && creationPacing < pacingBPS {
			// Inverse relationship: lower pacing → higher fee.
			adjusted := new(big.Int).Mul(fee, new(big.Int).SetUint64(pacingBPS))
			adjusted.Div(adjusted, new(big.Int).SetUint64(creationPacing))
			fee = adjusted
		}
	}

	// Layer 2: congestion adjustment (EIP-1559-style backpressure).
	if params.CongestionMultiplierBps > 0 && params.CongestionThresholdRounds > 0 {
		activeRounds := k.GetActiveRoundsCount(ctx)
		if activeRounds > params.CongestionThresholdRounds {
			excess := activeRounds - params.CongestionThresholdRounds
			// effectiveFee = fee × (1 + excess × multiplierBps / 1_000_000)
			// Rearranged to avoid float: fee + fee×excess×multiplierBps/1_000_000
			addend := new(big.Int).SetUint64(excess)
			addend.Mul(addend, new(big.Int).SetUint64(params.CongestionMultiplierBps))
			addend.Mul(addend, fee)
			addend.Div(addend, new(big.Int).SetUint64(1_000_000))
			fee = new(big.Int).Add(fee, addend)
		}
	}

	return fee.String()
}

// GetActiveRoundsCount returns the number of currently active verification
// rounds. Used by congestion-based fee backpressure.
func (k Keeper) GetActiveRoundsCount(ctx context.Context) uint64 {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.ActiveRoundsCountKey)
	if err != nil || len(bz) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// SetActiveRoundsCount writes the active-rounds counter to state.
func (k Keeper) SetActiveRoundsCount(ctx context.Context, count uint64) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, count)
	_ = kvStore.Set(types.ActiveRoundsCountKey, bz)
}

// IncrementActiveRoundsCount increments the active-rounds counter by 1.
// Called when a new verification round is created.
func (k Keeper) IncrementActiveRoundsCount(ctx context.Context) {
	k.SetActiveRoundsCount(ctx, k.GetActiveRoundsCount(ctx)+1)
}

// DecrementActiveRoundsCount decrements the active-rounds counter by 1,
// saturating at 0 to prevent underflow.
// Called when a verification round is completed.
func (k Keeper) DecrementActiveRoundsCount(ctx context.Context) {
	current := k.GetActiveRoundsCount(ctx)
	if current > 0 {
		k.SetActiveRoundsCount(ctx, current-1)
	}
}

// GetLastClaimHeight returns the block height at which a submitter last submitted a claim.
// Returns 0 if the submitter has never submitted.
func (k Keeper) GetLastClaimHeight(ctx context.Context, submitter string) uint64 {
	kvStore := k.storeService.OpenKVStore(ctx)
	key := types.LastClaimHeightKey(submitter)
	bz, err := kvStore.Get(key)
	if err != nil || len(bz) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// SetLastClaimHeight records the block height of a submitter's latest claim.
func (k Keeper) SetLastClaimHeight(ctx context.Context, submitter string, height uint64) {
	kvStore := k.storeService.OpenKVStore(ctx)
	key := types.LastClaimHeightKey(submitter)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, height)
	_ = kvStore.Set(key, bz)
}
