package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	researchtypes "github.com/zerone-chain/zerone/x/research/types"
)

// sdkBankKeeperForResearch is the minimal subset of the Cosmos SDK bank keeper
// used by ResearchBankAdapter. Satisfied by bankkeeper.Keeper.
type sdkBankKeeperForResearch interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoins(ctx context.Context, from sdk.AccAddress, to sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, sender string, recipient string, amt sdk.Coins) error
}

// ResearchBankAdapter adapts the Cosmos SDK bank keeper to satisfy
// researchtypes.BankKeeper by adding GetModuleBalance.
type ResearchBankAdapter struct {
	inner sdkBankKeeperForResearch
}

// NewResearchBankAdapter wraps the Cosmos SDK bank keeper for use in x/research.
func NewResearchBankAdapter(bk sdkBankKeeperForResearch) *ResearchBankAdapter {
	return &ResearchBankAdapter{inner: bk}
}

// Compile-time interface check.
var _ researchtypes.BankKeeper = (*ResearchBankAdapter)(nil)

// GetModuleBalance returns the balance of a module account by module name.
// Derives the module address from the name — no separate KV tracker needed.
func (a *ResearchBankAdapter) GetModuleBalance(ctx context.Context, moduleName string, denom string) sdk.Coin {
	addr := authtypes.NewModuleAddress(moduleName)
	return a.inner.GetBalance(ctx, addr, denom)
}

func (a *ResearchBankAdapter) SendCoins(ctx context.Context, from sdk.AccAddress, to sdk.AccAddress, amt sdk.Coins) error {
	return a.inner.SendCoins(ctx, from, to, amt)
}

func (a *ResearchBankAdapter) SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error {
	return a.inner.SendCoinsFromAccountToModule(ctx, sender, module, amt)
}

func (a *ResearchBankAdapter) SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error {
	return a.inner.SendCoinsFromModuleToAccount(ctx, senderModule, recipient, amt)
}

func (a *ResearchBankAdapter) SendCoinsFromModuleToModule(ctx context.Context, sender string, recipient string, amt sdk.Coins) error {
	return a.inner.SendCoinsFromModuleToModule(ctx, sender, recipient, amt)
}
