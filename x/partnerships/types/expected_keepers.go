package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper defines the expected Cosmos SDK bank keeper.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule string, recipientModule string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// HomeKeeper defines the expected home module interface.
type HomeKeeper interface {
	GetHomesByOwner(ctx context.Context, owner string) []string
	SetPartnershipOnHome(ctx context.Context, homeID, partnershipID string)
}

// ZeroneAuthKeeper defines the expected zerone auth keeper interface (R28-5).
type ZeroneAuthKeeper interface {
	GetAccountType(ctx context.Context, address string) (string, bool)
}

// CaptureDefenseKeeper provides access to capture_defense module for structural immunity (R29-5).
type CaptureDefenseKeeper interface {
	IsDomainFlagged(ctx context.Context, domain string) bool
}

// PacingKeeper provides global pacing signals for adaptive formation timing (R29-6).
type PacingKeeper interface {
	GetGlobalPacingMultiplier(ctx context.Context) (creationBps, analysisBps uint64)
}

// OntologyKeeper provides stratum information for cross-stratum partnership matching (R31-4).
type OntologyKeeper interface {
	GetRelatedStrata(ctx context.Context, domain string) []string
}

// KnowledgeKeeper provides access to knowledge module for mentorship dividends (R31-5).
type KnowledgeKeeper interface {
	ApplyMentorshipDividend(ctx context.Context, domain, mentor, mentee string)
}
