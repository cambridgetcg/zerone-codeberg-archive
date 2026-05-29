package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	zeronetokenstypes "github.com/zerone-chain/zerone/x/tokens/types"
)

// TokensStakingAdapter wraps the zerone staking Keeper to satisfy
// zeronetokenstypes.StakingKeeper (GetTotalBondedStake).
// Used by x/tokens BeginBlocker for adaptive emission rate (Gap 2).
type TokensStakingAdapter struct {
	k Keeper
}

// NewTokensStakingAdapter returns an adapter providing bonded stake data to x/tokens.
func NewTokensStakingAdapter(k Keeper) *TokensStakingAdapter {
	return &TokensStakingAdapter{k: k}
}

// Compile-time interface check.
var _ zeronetokenstypes.StakingKeeper = (*TokensStakingAdapter)(nil)

// GetTotalBondedStake returns total bonded stake across active validators in uzrn.
func (a *TokensStakingAdapter) GetTotalBondedStake(ctx sdk.Context) *big.Int {
	return a.k.GetTotalBondedStake(ctx)
}
