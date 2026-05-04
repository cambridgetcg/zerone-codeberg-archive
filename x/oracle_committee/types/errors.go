package types

import "cosmossdk.io/errors"

var (
	ErrOperatorNotFound       = errors.Register(ModuleName, 2, "oracle operator not found")
	ErrOperatorAlreadyExists  = errors.Register(ModuleName, 3, "oracle operator already registered")
	ErrOperatorInactive       = errors.Register(ModuleName, 4, "oracle operator is inactive")
	ErrInsufficientStake      = errors.Register(ModuleName, 5, "stake below minimum required")
	ErrInvalidVerdict         = errors.Register(ModuleName, 6, "verdict must be 'true', 'false', or 'uncertain'")
	ErrInvalidConfidence      = errors.Register(ModuleName, 7, "confidence_bps must be 0-10000")
	ErrDuplicateEvaluation    = errors.Register(ModuleName, 8, "operator already submitted evaluation for this claim")
	ErrAggregationExists      = errors.Register(ModuleName, 9, "aggregation already finalized for this claim")
	ErrCommitteeTooSmall      = errors.Register(ModuleName, 10, "active operator count below min_committee_size")
	ErrUnauthorized           = errors.Register(ModuleName, 11, "unauthorized")
)
