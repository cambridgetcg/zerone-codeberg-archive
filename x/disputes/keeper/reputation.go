package keeper

import (
	"context"
	"math/big"

	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

// ─── Disputer Profile (Gap 5: Reputation-Adjusted Bond Requirements) ──────────
//
// A DisputerProfile records the closed-dispute history for each challenger:
//   - WinCount      — disputes where outcome was CHALLENGER_WINS
//   - TotalDisputes — all closed disputes (wins + losses + draws + timeouts)
//
// ComputeEffectiveMinBond applies a discount for high win-rate challengers and
// a penalty for low win-rate challengers, modifying the tier minimum bond.
// The system is disabled when params.ReputationMinRounds == 0.

// GetDisputerProfile returns the reputation profile for addr.
// Returns a zero-value profile if none exists yet.
func (k Keeper) GetDisputerProfile(ctx context.Context, addr string) *types.DisputerProfile {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.DisputerProfileKey(addr))
	if err != nil || len(bz) == 0 {
		return &types.DisputerProfile{Address: addr}
	}
	profile := &types.DisputerProfile{}
	if err := proto.Unmarshal(bz, profile); err != nil {
		return &types.DisputerProfile{Address: addr}
	}
	return profile
}

// SetDisputerProfile persists a reputation profile.
func (k Keeper) SetDisputerProfile(ctx context.Context, profile *types.DisputerProfile) {
	bz, err := proto.Marshal(profile)
	if err != nil {
		return
	}
	kvStore := k.storeService.OpenKVStore(ctx)
	_ = kvStore.Set(types.DisputerProfileKey(profile.Address), bz)
}

// RecordDisputeOutcome updates the challenger's reputation profile based on
// the dispute outcome. Called by DistributeSettlement after verdict is final.
//
// Outcome mapping:
//   - CHALLENGER_WINS → win_count++, total_disputes++
//   - DEFENDER_WINS / DRAW / TIMED_OUT → total_disputes++ (no win)
func (k Keeper) RecordDisputeOutcome(ctx context.Context, challenger string, outcome types.DisputeOutcome) {
	profile := k.GetDisputerProfile(ctx, challenger)
	profile.TotalDisputes++
	if outcome == types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		profile.WinCount++
	}
	k.SetDisputerProfile(ctx, profile)
}

// ComputeEffectiveMinBond returns the minimum bond required for challenger to
// open a new dispute, adjusted by their reputation.
//
// Logic (when reputation system is enabled, i.e. params.ReputationMinRounds > 0):
//   - If profile.TotalDisputes < ReputationMinRounds: return tierMinBond unchanged
//     (not enough history to apply reputation adjustments).
//   - win_rate_bps = WinCount * 1_000_000 / TotalDisputes
//   - If win_rate_bps > ReputationHighThresholdBps:
//     effective = tierMinBond × (1_000_000 − ReputationDiscountBps) / 1_000_000
//   - If win_rate_bps < ReputationLowThresholdBps:
//     effective = tierMinBond × ReputationPenaltyMultiplierBps / 1_000_000
//   - Otherwise: return tierMinBond unchanged.
//
// The returned value is always ≥ 1 uzrn (no zero-bond edge case).
func (k Keeper) ComputeEffectiveMinBond(ctx context.Context, challenger string, tierMinBond *big.Int, params *types.Params) *big.Int {
	if params.ReputationMinRounds == 0 {
		return new(big.Int).Set(tierMinBond) // system disabled
	}

	profile := k.GetDisputerProfile(ctx, challenger)
	if profile.TotalDisputes < params.ReputationMinRounds {
		return new(big.Int).Set(tierMinBond) // insufficient history
	}

	// win_rate in BPS (1M scale): integer arithmetic, no division by zero (TotalDisputes >= ReputationMinRounds > 0).
	winRateBps := profile.WinCount * 1_000_000 / profile.TotalDisputes

	if winRateBps > params.ReputationHighThresholdBps {
		// Discount: effective = tierMinBond × (1_000_000 - discountBps) / 1_000_000
		numerator := new(big.Int).Mul(tierMinBond, big.NewInt(int64(1_000_000-params.ReputationDiscountBps)))
		result := new(big.Int).Div(numerator, big.NewInt(1_000_000))
		if result.Sign() <= 0 {
			result = big.NewInt(1)
		}
		return result
	}

	if winRateBps < params.ReputationLowThresholdBps {
		// Penalty: effective = tierMinBond × penaltyMultiplierBps / 1_000_000
		numerator := new(big.Int).Mul(tierMinBond, new(big.Int).SetUint64(params.ReputationPenaltyMultiplierBps))
		return new(big.Int).Div(numerator, big.NewInt(1_000_000))
	}

	return new(big.Int).Set(tierMinBond) // middle band: unchanged
}

// GetAllDisputerProfiles iterates all disputer profiles in the KV store.
// Used by ExportGenesis for state snapshot.
func (k Keeper) GetAllDisputerProfiles(ctx context.Context) []*types.DisputerProfile {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := types.DisputerProfileKeyPrefix
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var profiles []*types.DisputerProfile
	for ; iter.Valid(); iter.Next() {
		profile := &types.DisputerProfile{}
		if err := proto.Unmarshal(iter.Value(), profile); err == nil {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}
