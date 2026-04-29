package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	knowledgetypes "github.com/zerone-chain/zerone/x/knowledge/types"
)

// BankKeeper defines the expected bank module interface.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule string, recipientModule string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// StakingKeeper defines the expected staking module interface for arbiter selection.
type StakingKeeper interface {
	GetQualifiedValidators(ctx context.Context, domain string, count int) ([]string, error)
}

// KnowledgeKeeper defines the expected knowledge module interface.
type KnowledgeKeeper interface {
	GetFact(ctx context.Context, factID string) (*knowledgetypes.Fact, bool)
}
