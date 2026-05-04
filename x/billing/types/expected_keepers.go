package types

import (
	"context"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper defines the expected bank module interface.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule string, recipientModule string, amt sdk.Coins) error
}

// KnowledgeKeeper defines the expected knowledge module interface for billing.
type KnowledgeKeeper interface {
	GetFactConfidence(ctx context.Context, factId string) (uint64, bool)
	GetFactCitationCount(ctx context.Context, factId string) (uint64, bool)
	GetFactSubmitter(ctx context.Context, factId string) (string, bool)
	GetFactCreatedBlock(ctx context.Context, factId string) (uint64, bool)
	IncrementCitationCount(ctx context.Context, factId string) error
}

// ResearchFundDepositor routes deposits to the research fund with founder auto-split.
// Satisfied by VestingRewardsKeeper adapter.
type ResearchFundDepositor interface {
	DepositToResearchFund(ctx context.Context, sourceModule string, amount sdk.Coins) error
}

// LiquidityPoolKeeper provides TWAP oracle data for dynamic pricing.
// Nil-safe — set post-init when x/liquiditypool is wired.
type LiquidityPoolKeeper interface {
	GetTWAP(ctx context.Context, denom string, windowBlocks uint64) (*big.Int, error)
	GetLastPriceUpdateHeight(ctx context.Context) uint64
}

// ExternalPriceFeedKeeper provides prices from external oracle sources.
// Nil-safe — set post-init when an oracle feed module is wired.
//
// GetLatestPrice returns the current ZRN/USD price (6-decimal micro-USD)
// for the given feed address. Returns (nil, err) if the feed is unavailable
// or the price is stale. Implementations may include:
//   - An oracle committee module that aggregates operator submissions
//   - A Chainlink/Pyth adapter that reads from ABCI vote extensions
//   - A governance-managed price store
type ExternalPriceFeedKeeper interface {
	GetLatestPrice(ctx context.Context, feedAddress string) (*big.Int, error)
}
