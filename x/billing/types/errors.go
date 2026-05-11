package types

import "cosmossdk.io/errors"

var (
	ErrProviderNotFound   = errors.Register(ModuleName, 2, "provider not found")
	ErrProviderExists     = errors.Register(ModuleName, 3, "provider already exists")
	ErrInsufficientStake  = errors.Register(ModuleName, 4, "insufficient stake")
	ErrProviderNotActive  = errors.Register(ModuleName, 5, "provider not active")
	ErrInvalidAmount      = errors.Register(ModuleName, 6, "invalid amount")
	ErrQuoteExpired       = errors.Register(ModuleName, 7, "quote expired")
	ErrUnauthorized       = errors.Register(ModuleName, 8, "unauthorized")
	ErrInsufficientFunds  = errors.Register(ModuleName, 9, "insufficient funds")
	ErrFactNotFound       = errors.Register(ModuleName, 10, "fact not found")
	ErrNoFactsRequested   = errors.Register(ModuleName, 11, "no facts requested")
	ErrInvalidDistribution          = errors.Register(ModuleName, 12, "invalid distribution")
	ErrZeroPayment                  = errors.Register(ModuleName, 13, "zero payment")
	ErrSpendingLimitExceeded        = errors.Register(ModuleName, 14, "amount exceeds development fund spending limit")
	ErrSpendingDisabled             = errors.Register(ModuleName, 15, "development fund spending is disabled (limit is 0)")
	ErrInsufficientDevelopmentFund  = errors.Register(ModuleName, 16, "development fund has insufficient balance")
)
