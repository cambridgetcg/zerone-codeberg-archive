package types

import (
	"context"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AccountKeeper defines the expected account keeper interface.
type AccountKeeper interface {
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
}

// BankKeeper defines the expected bank keeper interface.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// StakingKeeper defines the expected staking keeper interface.
type StakingKeeper interface {
	// GetActiveValidatorInfos returns all validators eligible for selection.
	GetActiveValidatorInfos(ctx context.Context) ([]ValidatorInfo, error)
	// GetValidatorInfo returns info for a specific validator.
	GetValidatorInfo(ctx context.Context, addr string) (*ValidatorInfo, error)
	// GetEffectiveStake returns the effective stake for a validator (including virtual stake).
	GetEffectiveStake(ctx context.Context, addr string) (uint64, error)
	// GetTotalStake returns the total staked amount across all validators.
	GetTotalStake(ctx context.Context) (uint64, error)
	// SlashValidator slashes a validator by the given BPS amount.
	SlashValidator(ctx context.Context, addr string, slashBps uint64) error
	// SlashValidatorToModule slashes a validator and routes tokens to a specific module account.
	// Returns the actual slashed amount.
	SlashValidatorToModule(ctx context.Context, addr string, slashBps uint64, destModule string) (sdkmath.Int, error)
}

// OntologyKeeper defines the expected ontology keeper interface.
// Provides confidence ceilings, logic zones, and stratum definitions.
type OntologyKeeper interface {
	// GetConfidenceCeiling returns the max confidence for a stratum (0-1,000,000).
	GetConfidenceCeiling(ctx context.Context, stratum string) (uint64, error)
	// IsValidLogicZone checks if a domain is within a valid logic zone.
	IsValidLogicZone(ctx context.Context, domain string) (bool, error)
	// AcknowledgesIncompleteness checks if a domain acknowledges Godelian limits.
	AcknowledgesIncompleteness(ctx context.Context, domain string) (bool, error)
	// GetStratumForDomain returns the stratum associated with a domain.
	GetStratumForDomain(ctx context.Context, domain string) (string, error)
	// GetDepthForDomain returns the tree depth of a domain (R31-4: Metal→Wood).
	// Root domains have depth 1; deeper strata have higher depth values.
	GetDepthForDomain(ctx context.Context, domainName string) (uint32, error)
}

// DomainQualificationKeeper defines the expected domain qualification keeper interface.
type DomainQualificationKeeper interface {
	// IsQualified checks if a validator is qualified for a domain.
	IsQualified(ctx context.Context, validatorAddr, domain string) (bool, error)
	// GetQualificationWeight returns the qualification weight for a validator in a domain.
	GetQualificationWeight(ctx context.Context, validatorAddr, domain string) (uint64, error)
	// GetQualifiedValidators returns all validators qualified for a domain.
	GetQualifiedValidators(ctx context.Context, domain string) ([]string, error)
	// RecordVerificationOutcome records the outcome of a verification for reputation.
	RecordVerificationOutcome(ctx context.Context, validatorAddr, domain string, accepted bool) error
}

// VestingRewardsKeeper defines the expected vesting rewards keeper interface.
// Signatures match the actual x/vesting_rewards/keeper implementations.
type VestingRewardsKeeper interface {
	// CreateVestingScheduleFromKnowledge creates a vesting schedule for a knowledge reward.
	CreateVestingScheduleFromKnowledge(ctx context.Context, claimID, factID, recipient, totalAmount, epistemicCategory string) error
	// DistributeFalsificationReward distributes a falsification reward.
	DistributeFalsificationReward(ctx context.Context, counterFactID, targetFactID, recipient, amount string) error
	// GetEpochBlockRewardPool returns the estimated reward pool for an epoch (in uzrn).
	GetEpochBlockRewardPool(ctx context.Context, epoch uint64) uint64
	// PauseVestingByClaimId pauses vesting schedules associated with a claim.
	PauseVestingByClaimId(ctx context.Context, claimID string) error
	// PauseAllVestingByRecipient pauses all vesting schedules for a recipient.
	PauseAllVestingByRecipient(ctx context.Context, recipient string) int
	// DepositToResearchFund deposits an amount into the research fund.
	DepositToResearchFund(ctx context.Context, sourceModule string, amount sdk.Coins) error
}

// AutopoiesisKeeper defines the expected autopoiesis keeper interface.
type AutopoiesisKeeper interface {
	// GetMultiplier returns the autopoiesis reward multiplier for a path (BPS, 1,000,000 = 1.0x).
	GetMultiplier(ctx context.Context, path string) (uint64, error)
}

// PartnershipKeeper defines the expected partnership keeper interface.
type PartnershipKeeper interface {
	// IsActive checks if a partnership exists and is active (not frozen, dissolved, etc).
	IsActive(ctx context.Context, partnershipId string) (bool, error)
	// IsParticipant checks if an address is a participant in a partnership.
	IsParticipant(ctx context.Context, partnershipId string, address string) (bool, error)
	// IsSuspended checks if a partnership is suspended (coercion freeze).
	IsSuspended(ctx context.Context, partnershipId string) (bool, error)
	// DistributeReward distributes a reward through the partnership split.
	DistributeReward(ctx context.Context, partnershipId string, amount sdk.Coins, source string) error
	// GetDomainPartnershipDensity returns the count of unique partnership participants in a domain (R31-2).
	GetDomainPartnershipDensity(ctx context.Context, domain string) uint64
}

// ZeroneAuthKeeper defines the expected zerone auth keeper interface (R28-5).
// Used to look up account types (human/agent/contract) for role bonuses.
type ZeroneAuthKeeper interface {
	// GetAccountType returns the account type ("human", "agent", "contract", "system")
	// for a given bech32 address. Returns "" and false if not found.
	GetAccountType(ctx context.Context, address string) (string, bool)
}

// CaptureDefenseKeeper feeds verification history and reputation updates to capture defense.
type CaptureDefenseKeeper interface {
	RecordVerificationHistory(ctx context.Context, domain, roundId string, validators []string, verdicts []bool, submitBlocks []uint64)
	UpdateReputation(ctx context.Context, validator string, domain string, stratum string, approved bool)
	// GetDomainCapturePenalty returns whether a domain is flagged and its HHI-based penalty (R31-1).
	GetDomainCapturePenalty(ctx context.Context, domain string) (flagged bool, penaltyBps uint64)
}

// PacingKeeper provides global pacing signals from the alignment module (R29-6).
type PacingKeeper interface {
	GetGlobalPacingMultiplier(ctx context.Context) (creationBps, analysisBps uint64)
}

// CounterexampleKeeper exposes the alignment-by-structure read used
// by ComputeTrainingValueWeight. The contract is intentionally narrow:
// "does this fact have at least one validated counterexample, and if
// so, what BPS multiplier should TVW apply?" Anything richer (the
// counterexamples themselves, validations, error type) belongs to
// queries against x/counterexamples directly.
type CounterexampleKeeper interface {
	HasValidatedCounterexample(ctx context.Context, factID string) bool
	GetTvwMultiplierBps(ctx context.Context) uint64
}
