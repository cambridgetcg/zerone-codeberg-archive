package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	billingtypes "github.com/zerone-chain/zerone/x/billing/types"
	toolboxtypes "github.com/zerone-chain/zerone/x/toolbox/types"
)

// -----------------------------------------------------------------------
// BillingBankAdapter
// -----------------------------------------------------------------------

// sdkBankKeeper is the minimal subset of the Cosmos SDK bank keeper used by
// BillingBankAdapter. Satisfied by bankkeeper.Keeper.
type sdkBankKeeper interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoins(ctx context.Context, from sdk.AccAddress, to sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, sender string, recipient string, amt sdk.Coins) error
}

// BillingBankAdapter adapts the Cosmos SDK bank keeper to satisfy
// billingtypes.BankKeeper by adding GetModuleBalance.
type BillingBankAdapter struct {
	inner sdkBankKeeper
}

// NewBillingBankAdapter wraps the Cosmos SDK bank keeper for use in billing.
func NewBillingBankAdapter(bk sdkBankKeeper) *BillingBankAdapter {
	return &BillingBankAdapter{inner: bk}
}

// Compile-time interface check.
var _ billingtypes.BankKeeper = (*BillingBankAdapter)(nil)

func (a *BillingBankAdapter) GetModuleBalance(ctx context.Context, moduleName string, denom string) sdk.Coin {
	addr := authtypes.NewModuleAddress(moduleName)
	return a.inner.GetBalance(ctx, addr, denom)
}

func (a *BillingBankAdapter) SendCoins(ctx context.Context, from sdk.AccAddress, to sdk.AccAddress, amt sdk.Coins) error {
	return a.inner.SendCoins(ctx, from, to, amt)
}

func (a *BillingBankAdapter) SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error {
	return a.inner.SendCoinsFromAccountToModule(ctx, sender, module, amt)
}

func (a *BillingBankAdapter) SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error {
	return a.inner.SendCoinsFromModuleToAccount(ctx, senderModule, recipient, amt)
}

func (a *BillingBankAdapter) SendCoinsFromModuleToModule(ctx context.Context, sender string, recipient string, amt sdk.Coins) error {
	return a.inner.SendCoinsFromModuleToModule(ctx, sender, recipient, amt)
}

// ToolboxBillingAdapter wraps the billing Keeper to satisfy
// toolboxtypes.BillingKeeper interface.
type ToolboxBillingAdapter struct {
	k Keeper
}

// NewToolboxBillingAdapter returns an adapter that bridges the billing keeper
// to the toolbox module's expected BillingKeeper interface.
func NewToolboxBillingAdapter(k Keeper) *ToolboxBillingAdapter {
	return &ToolboxBillingAdapter{k: k}
}

// Ensure compile-time interface compliance.
var _ toolboxtypes.BillingKeeper = (*ToolboxBillingAdapter)(nil)

// GetZRNPriceUSD returns the current ZRN price in micro-USD.
// The billing keeper's GetZRNPriceUSD returns uint64 (no error),
// so we wrap it to match the toolbox interface (uint64, error).
func (a *ToolboxBillingAdapter) GetZRNPriceUSD(ctx context.Context) (uint64, error) {
	return a.k.GetZRNPriceUSD(ctx), nil
}
