package types

import "cosmossdk.io/errors"

var (
	ErrPoolNotFound          = errors.Register(ModuleName, 2, "pool not found")
	ErrPoolAlreadyExists     = errors.Register(ModuleName, 3, "pool already exists for this denom pair")
	ErrMaxPoolsReached       = errors.Register(ModuleName, 4, "maximum active pools reached")
	ErrInsufficientLiquidity = errors.Register(ModuleName, 5, "insufficient initial liquidity")
	ErrInvalidDenom          = errors.Register(ModuleName, 6, "invalid denom")
	ErrSameDenom             = errors.Register(ModuleName, 7, "cannot create pool with same denom on both sides")
	ErrSlippageExceeded      = errors.Register(ModuleName, 8, "output below minimum (slippage exceeded)")
	ErrPoolLocked            = errors.Register(ModuleName, 9, "pool is locked (swap in progress)")
	ErrZeroAmount            = errors.Register(ModuleName, 10, "amount must be positive")
	ErrInvalidSwapFee        = errors.Register(ModuleName, 11, "swap fee must be between 0 and 1000000 bps")
	ErrUnauthorized          = errors.Register(ModuleName, 12, "unauthorized: only governance can create pools")
	ErrDenomNotInPool        = errors.Register(ModuleName, 13, "denom not found in this pool")
	ErrInsufficientLP        = errors.Register(ModuleName, 14, "insufficient LP tokens")
	ErrNoPool                = errors.Register(ModuleName, 15, "no pool found for denom pair")
	ErrReserveBelowMinimum   = errors.Register(ModuleName, 16, "reserve would fall below minimum")
	ErrTWAPStale             = errors.Register(ModuleName, 17, "TWAP accumulator is stale")
)
