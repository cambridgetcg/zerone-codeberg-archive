package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AccountKeeper defines the expected auth module keeper.
type AccountKeeper interface {
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
	SetAccount(ctx context.Context, acc sdk.AccountI)
	NewAccountWithAddress(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
}

// BankKeeper defines the expected bank module keeper.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	GetAllBalances(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule string, recipientModule string, amt sdk.Coins) error
}

// AutopoiesisKeeper defines the expected autopoiesis module keeper.
// Set via post-init setter to break circular dependency.
type AutopoiesisKeeper interface {
	GetMultiplier(ctx context.Context, path string) uint64
}

// ZeroneAuthKeeper defines the expected Zerone auth module keeper.
// Reserved for future use.
type ZeroneAuthKeeper interface{}

// EmergencyKeeper defines the staking module's interface to the emergency module.
// Set via post-init setter (SetEmergencyKeeper) to break the circular dependency
// between staking and emergency. The staking module calls RecordSlash on every
// confirmed slash so that the emergency module can monitor the slash rate.
type EmergencyKeeper interface {
	RecordSlash(ctx context.Context)
}
