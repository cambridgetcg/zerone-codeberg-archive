package types

import (
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Tier aliases for convenience.
const (
	TierApprentice = ValidatorTier_VALIDATOR_TIER_APPRENTICE
	TierVerified   = ValidatorTier_VALIDATOR_TIER_VERIFIED
	TierScholar    = ValidatorTier_VALIDATOR_TIER_SCHOLAR
	TierGuardian   = ValidatorTier_VALIDATOR_TIER_GUARDIAN
)

// BPS scale constants.
const (
	BPSScale  = uint64(1_000_000)
	MaxBPS    = uint64(1_000_000)
	HalfScale = uint64(500_000) // 50%
)

// ValidatorTierFromString converts a tier name to its enum value.
func ValidatorTierFromString(s string) ValidatorTier {
	switch s {
	case "apprentice":
		return TierApprentice
	case "verified":
		return TierVerified
	case "scholar":
		return TierScholar
	case "guardian":
		return TierGuardian
	default:
		return ValidatorTier_VALIDATOR_TIER_UNSPECIFIED
	}
}

// ValidatorTierString returns the human-readable name for a tier.
func ValidatorTierString(t ValidatorTier) string {
	switch t {
	case TierApprentice:
		return "apprentice"
	case TierVerified:
		return "verified"
	case TierScholar:
		return "scholar"
	case TierGuardian:
		return "guardian"
	default:
		return "unspecified"
	}
}

// GetAccuracyRate returns the accuracy rate in BPS (1,000,000 scale).
func (v *Validator) GetAccuracyRate() uint64 {
	if v.TotalVerifications == 0 {
		return 0
	}
	return (v.CorrectVerifications * BPSScale) / v.TotalVerifications
}

// DefaultTierConfigs returns the default tier configurations.
func DefaultTierConfigs() []*TierConfig {
	return []*TierConfig{
		{
			Tier:                            TierApprentice,
			Name:                            "Apprentice",
			MinStake:                        "111000",     // 0.111 ZRN
			MinReputation:                   0,
			MinVerifications:                0,
			MinAccuracy:                     0,
			MaxSlashCount:                   -1, // unlimited
			AllowedCategories:               []string{"protocol", "computational", "formal"},
			RewardMultiplierBps:             100,  // 0.1x
			SelectionWeightBps:              100,  // 0.1x
			SlashWindowEpochs:               0,
			MinContestedVerifications:        0,
			ContestedVerificationMultiplier: 1,
			SlashMultiplierBps:              1500, // 1.5x
		},
		{
			Tier:                            TierVerified,
			Name:                            "Verified",
			MinStake:                        "1110000",    // 1.11 ZRN
			MinReputation:                   770_000,      // 77%
			MinVerifications:                22,
			MinAccuracy:                     770_000,      // 77%
			MaxSlashCount:                   -1,
			AllowedCategories:               []string{"protocol", "computational", "formal", "empirical"},
			RewardMultiplierBps:             500,  // 0.5x
			SelectionWeightBps:              500,  // 0.5x
			SlashWindowEpochs:               0,
			MinContestedVerifications:        0,
			ContestedVerificationMultiplier: 1,
			SlashMultiplierBps:              1200, // 1.2x
		},
		{
			Tier:                            TierScholar,
			Name:                            "Scholar",
			MinStake:                        "1111000000",  // 1,111 ZRN
			MinReputation:                   500_000,       // 50%
			MinVerifications:                11,
			MinAccuracy:                     500_000,       // 50%
			MaxSlashCount:                   -1,
			AllowedCategories:               []string{"protocol", "computational", "formal", "empirical", "oracle", "attestation"},
			RewardMultiplierBps:             1000, // 1.0x
			SelectionWeightBps:              1000, // 1.0x
			SlashWindowEpochs:               0,
			MinContestedVerifications:        0,
			ContestedVerificationMultiplier: 1,
			SlashMultiplierBps:              1000, // 1.0x
		},
		{
			Tier:                            TierGuardian,
			Name:                            "Guardian",
			MinStake:                        "11111000000", // 11,111 ZRN
			MinReputation:                   770_000,       // 77%
			MinVerifications:                333,
			MinAccuracy:                     770_000,       // 77%
			MaxSlashCount:                   0,             // zero tolerance
			AllowedCategories:               []string{"protocol", "computational", "formal", "empirical", "oracle", "attestation", "predictive", "social", "contested"},
			RewardMultiplierBps:             2000, // 2.0x
			SelectionWeightBps:              1500, // 1.5x
			SlashWindowEpochs:               10,
			MinContestedVerifications:        33,  // MUST be checked (P0-2 fix)
			ContestedVerificationMultiplier: 3,
			SlashMultiplierBps:              1000, // 1.0x
		},
	}
}

// ---------- SlashType helpers ----------

// DefaultSlashRateConfigs returns the governance-seeded per-type slash rates.
// These align with the knowledge module's BPS scale (wrong=5%, equivocation=20%).
func DefaultSlashRateConfigs() []*SlashRateConfig {
	return []*SlashRateConfig{
		{SlashType: SlashType_SLASH_TYPE_DOUBLE_VOTE, RateBps: 200_000},            // 20% — Byzantine, severe
		{SlashType: SlashType_SLASH_TYPE_LIVENESS_FAULT, RateBps: 10_000},          // 1%  — missed blocks, mild
		{SlashType: SlashType_SLASH_TYPE_CALIBRATION_FAULT, RateBps: 50_000},       // 5%  — confidence miscalibration
		{SlashType: SlashType_SLASH_TYPE_COORDINATION_VIOLATION, RateBps: 100_000}, // 10% — VRF / coordination breach
	}
}

// GetSlashRate returns the configured rate_bps for a SlashType.
// Returns 0 if the type is not found (caller decides whether to skip or error).
func GetSlashRate(params *Params, st SlashType) uint64 {
	for _, cfg := range params.SlashRateConfigs {
		if cfg.SlashType == st {
			return cfg.RateBps
		}
	}
	return 0
}

// SlashTypeString returns the human-readable name for a SlashType.
func SlashTypeString(st SlashType) string {
	switch st {
	case SlashType_SLASH_TYPE_DOUBLE_VOTE:
		return "double_vote"
	case SlashType_SLASH_TYPE_LIVENESS_FAULT:
		return "liveness_fault"
	case SlashType_SLASH_TYPE_CALIBRATION_FAULT:
		return "calibration_fault"
	case SlashType_SLASH_TYPE_COORDINATION_VIOLATION:
		return "coordination_violation"
	default:
		return "unspecified"
	}
}

// ---------- ValidateBasic for Msg types ----------

func (m *MsgRegisterValidator) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return ErrInvalidAddress.Wrap("invalid operator address")
	}
	if m.ConsensusPubkey == "" {
		return ErrInvalidPubkey.Wrap("consensus pubkey required")
	}
	if m.CommissionBps > 10_000 {
		return ErrInvalidCommission.Wrap("commission exceeds 100%")
	}
	if len(m.Moniker) > 70 {
		return ErrInvalidDescription.Wrap("moniker too long (max 70)")
	}
	if len(m.Did) > 0 && len(m.Did) > 128 {
		return ErrInvalidDID.Wrap("DID too long (max 128)")
	}
	if len(m.Website) > 140 {
		return ErrInvalidDescription.Wrap("website too long (max 140)")
	}
	if len(m.Details) > 2000 {
		return ErrInvalidDescription.Wrap("details too long (max 2000)")
	}
	return nil
}

func (m *MsgDelegate) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Delegator); err != nil {
		return ErrInvalidAddress.Wrap("invalid delegator address")
	}
	if _, err := sdk.AccAddressFromBech32(m.Validator); err != nil {
		return ErrInvalidAddress.Wrap("invalid validator address")
	}
	amt, ok := new(big.Int).SetString(m.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return ErrInvalidAmount.Wrap("amount must be positive")
	}
	return nil
}

func (m *MsgUndelegate) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Delegator); err != nil {
		return ErrInvalidAddress.Wrap("invalid delegator address")
	}
	if _, err := sdk.AccAddressFromBech32(m.Validator); err != nil {
		return ErrInvalidAddress.Wrap("invalid validator address")
	}
	amt, ok := new(big.Int).SetString(m.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return ErrInvalidAmount.Wrap("amount must be positive")
	}
	return nil
}

func (m *MsgRedelegate) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Delegator); err != nil {
		return ErrInvalidAddress.Wrap("invalid delegator address")
	}
	if _, err := sdk.AccAddressFromBech32(m.SrcValidator); err != nil {
		return ErrInvalidAddress.Wrap("invalid source validator address")
	}
	if _, err := sdk.AccAddressFromBech32(m.DstValidator); err != nil {
		return ErrInvalidAddress.Wrap("invalid destination validator address")
	}
	if m.SrcValidator == m.DstValidator {
		return ErrSameValidator
	}
	amt, ok := new(big.Int).SetString(m.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return ErrInvalidAmount.Wrap("amount must be positive")
	}
	return nil
}

func (m *MsgUpdateValidatorStake) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return ErrInvalidAddress.Wrap("invalid operator address")
	}
	if m.Amount == "" || m.Amount == "0" {
		return ErrInvalidAmount.Wrap("amount required")
	}
	amt, ok := new(big.Int).SetString(m.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return ErrInvalidAmount.Wrap("amount must be positive")
	}
	return nil
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if m.Authority == "" {
		return ErrUnauthorized.Wrap("authority required")
	}
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return ErrInvalidAddress.Wrap("invalid authority address")
	}
	if m.Params == nil {
		return ErrInvalidParams.Wrap("params required")
	}
	return m.Params.Validate()
}

// ---------- Params validation ----------

func (p *Params) Validate() error {
	if p.UnbondingPeriod == 0 {
		return fmt.Errorf("unbonding period must be positive")
	}
	if p.MaxValidators == 0 {
		return fmt.Errorf("max validators must be positive")
	}
	vs, ok := new(big.Int).SetString(p.VirtualStake, 10)
	if !ok || vs.Sign() <= 0 {
		return fmt.Errorf("virtual stake must be positive")
	}
	ms, ok := new(big.Int).SetString(p.MinSelfDelegation, 10)
	if !ok || ms.Sign() <= 0 {
		return fmt.Errorf("min self delegation must be positive")
	}
	if p.SlashEscalationBps > BPSScale {
		return fmt.Errorf("slash escalation bps exceeds maximum (%d)", BPSScale)
	}
	if p.ReputationCorrectDelta > BPSScale {
		return fmt.Errorf("reputation correct delta exceeds maximum")
	}
	if p.ReputationIncorrectDelta > BPSScale {
		return fmt.Errorf("reputation incorrect delta exceeds maximum")
	}
	if p.ReputationSlashDelta > BPSScale {
		return fmt.Errorf("reputation slash delta exceeds maximum")
	}
	if p.SlashBurnFractionBps > BPSScale {
		return fmt.Errorf("slash burn fraction bps exceeds maximum (%d)", BPSScale)
	}
	if p.EvidenceRewardBps > BPSScale {
		return fmt.Errorf("evidence reward bps exceeds maximum (%d)", BPSScale)
	}
	// tombstone_threshold must form a meaningful two-level hierarchy:
	// the tombstone threshold must be strictly greater than max_slash_count_deactivate
	// so there is a window in which a validator is deactivated but not yet permanently banned.
	// 0 = disabled (no automatic tombstoning).
	if p.TombstoneThreshold > 0 && p.MaxSlashCountDeactivate > 0 &&
		p.TombstoneThreshold <= p.MaxSlashCountDeactivate {
		return fmt.Errorf(
			"tombstone_threshold (%d) must exceed max_slash_count_deactivate (%d) to form a meaningful two-level hierarchy; set tombstone_threshold=0 to disable",
			p.TombstoneThreshold, p.MaxSlashCountDeactivate,
		)
	}

	// Validate slash_rate_configs: each rate must fit within BPS scale.
	for i, cfg := range p.SlashRateConfigs {
		if cfg == nil {
			return fmt.Errorf("slash_rate_configs[%d]: nil entry", i)
		}
		if cfg.RateBps > BPSScale {
			return fmt.Errorf("slash_rate_configs[%d]: rate_bps %d exceeds maximum (%d)", i, cfg.RateBps, BPSScale)
		}
	}

	if len(p.TierConfigs) > 0 {
		if len(p.TierConfigs) != 4 {
			return fmt.Errorf("tier configs must have exactly 4 entries, got %d", len(p.TierConfigs))
		}
		for i, tc := range p.TierConfigs {
			s, ok := new(big.Int).SetString(tc.MinStake, 10)
			if !ok || s.Sign() < 0 {
				return fmt.Errorf("tier %d: min stake must be non-negative", i)
			}
			if tc.MinAccuracy > BPSScale {
				return fmt.Errorf("tier %d: min accuracy exceeds maximum", i)
			}
			if tc.RewardMultiplierBps == 0 {
				return fmt.Errorf("tier %d: reward multiplier must be non-zero", i)
			}
			if tc.SelectionWeightBps == 0 {
				return fmt.Errorf("tier %d: selection weight must be non-zero", i)
			}
			if tc.SlashMultiplierBps < 1 || tc.SlashMultiplierBps > 10_000 {
				return fmt.Errorf("tier %d: slash multiplier bps must be 1-10000", i)
			}
		}
	}

	return nil
}

// ---------- GetSigners for Msg types ----------

func (m *MsgRegisterValidator) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Operator)
	return []sdk.AccAddress{addr}
}

func (m *MsgDelegate) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Delegator)
	return []sdk.AccAddress{addr}
}

func (m *MsgUndelegate) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Delegator)
	return []sdk.AccAddress{addr}
}

func (m *MsgRedelegate) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Delegator)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateValidatorStake) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Operator)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m *MsgSubmitSlashEvidence) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{addr}
}

func (m *MsgSubmitSlashEvidence) ValidateBasic() error {
	if m.Submitter == "" {
		return fmt.Errorf("submitter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if m.ValidatorAddr == "" {
		return fmt.Errorf("validator_addr cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(m.ValidatorAddr); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if m.SlashType == SlashType_SLASH_TYPE_UNSPECIFIED {
		return fmt.Errorf("slash_type cannot be unspecified")
	}
	if m.EvidenceHash == "" {
		return fmt.Errorf("evidence_hash cannot be empty")
	}
	if len(m.WitnessData) > 512 {
		return fmt.Errorf("witness_data exceeds maximum length of 512 characters")
	}
	return nil
}

func (m *MsgUntombstoneValidator) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m *MsgUntombstoneValidator) ValidateBasic() error {
	if m.Authority == "" {
		return ErrUnauthorized.Wrap("authority required")
	}
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return ErrInvalidAddress.Wrap("invalid authority address")
	}
	if m.ValidatorAddress == "" {
		return ErrInvalidAddress.Wrap("validator_address required")
	}
	if _, err := sdk.AccAddressFromBech32(m.ValidatorAddress); err != nil {
		return ErrInvalidAddress.Wrap("invalid validator address")
	}
	return nil
}
