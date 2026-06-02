package types

import "cosmossdk.io/errors"

var (
	ErrDisputeNotFound      = errors.Register(ModuleName, 2, "dispute not found")
	ErrInvalidBond          = errors.Register(ModuleName, 3, "invalid bond amount")
	ErrInsufficientBond     = errors.Register(ModuleName, 4, "bond below tier minimum")
	ErrInvalidTargetType    = errors.Register(ModuleName, 5, "invalid dispute target type")
	ErrTargetNotFound       = errors.Register(ModuleName, 6, "dispute target not found")
	ErrMaxActiveDisputes    = errors.Register(ModuleName, 7, "max active disputes reached")
	ErrWrongPhase           = errors.Register(ModuleName, 8, "dispute not in required phase")
	ErrDeadlinePassed       = errors.Register(ModuleName, 9, "deadline has passed")
	ErrDeadlineNotPassed    = errors.Register(ModuleName, 10, "deadline has not passed")
	ErrNotParty             = errors.Register(ModuleName, 11, "sender not a dispute party")
	ErrCommitmentExists     = errors.Register(ModuleName, 12, "evidence commitment already exists")
	ErrCommitmentNotFound   = errors.Register(ModuleName, 13, "evidence commitment not found")
	ErrHashMismatch         = errors.Register(ModuleName, 14, "revealed content hash does not match commitment")
	ErrAlreadyRevealed      = errors.Register(ModuleName, 15, "commitment already revealed")
	ErrNotArbiter           = errors.Register(ModuleName, 16, "sender not an assigned arbiter")
	ErrAlreadyVoted         = errors.Register(ModuleName, 17, "arbiter already voted")
	ErrMaxTierReached       = errors.Register(ModuleName, 18, "dispute already at maximum tier")
	ErrEscalationTooEarly   = errors.Register(ModuleName, 19, "escalation delay not elapsed")
	ErrNoQuorum             = errors.Register(ModuleName, 20, "quorum not reached")
	ErrUnauthorized         = errors.Register(ModuleName, 21, "unauthorized")
	ErrInsufficientArbiters = errors.Register(ModuleName, 22, "insufficient qualified arbiters")

	// Service Escrow errors
	ErrServiceEscrowNotFound = errors.Register(ModuleName, 23, "service escrow not found")
	ErrEscrowExpired         = errors.Register(ModuleName, 24, "service escrow deadline has passed")
	ErrEscrowNotPending      = errors.Register(ModuleName, 25, "service escrow is not in pending status")
	ErrEscrowNotCompleted    = errors.Register(ModuleName, 26, "service escrow is not in completed status")
	ErrUnauthorizedEscrow    = errors.Register(ModuleName, 27, "sender is not authorized for this escrow action")
	ErrInvalidEscrowAmount   = errors.Register(ModuleName, 28, "invalid escrow amount")
	ErrEscrowDisputeWindow   = errors.Register(ModuleName, 29, "dispute window has closed for this escrow")

	// Defender Bond errors
	ErrDefenderBondExists    = errors.Register(ModuleName, 30, "defender bond already posted")
	ErrNotDefender           = errors.Register(ModuleName, 31, "sender is not the dispute defender")

	// Appeal errors
	ErrNoAppealWindow        = errors.Register(ModuleName, 32, "appeal window not configured for this tier")
	ErrAppealWindowClosed    = errors.Register(ModuleName, 33, "appeal window has closed")
	ErrAlreadyAppealed       = errors.Register(ModuleName, 34, "dispute has already been appealed")
	ErrAppealBondInsufficient = errors.Register(ModuleName, 35, "appeal bond below required minimum")
	ErrNotPartyToDispute     = errors.Register(ModuleName, 36, "appealer is not a party to the dispute")

	// Collateral Registry errors
	ErrLeverageExceeded = errors.Register(ModuleName, 37, "collateral leverage limit exceeded: total locked would exceed max_leverage_bps of spendable balance")
)
