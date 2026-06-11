package types

import (
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultParams returns the default qualification module parameters.
//
// These values are not tuning knobs; they are this module's
// expression of two truth-seeking commitments. See doc.go and
// docs/TRUTH_SEEKING.md for the binding tests.
func DefaultParams() *Params {
	return &Params{
		// MinStakeAmount + MinVerifications + MinAccuracyBps form the
		// initial qualification bar. Commitment 7 (skill is current):
		// no validator earns qualification by stake alone — they must
		// have done the work and done it accurately. 100 ZRN is the
		// price of admission; 100 verifications and 80% accuracy is
		// the demonstration.
		MinStakeAmount:               "100000000", // 100 ZRN — admission, not skill
		StakeLockPeriod:              100800,      // ~7 days — stake must commit before counting
		MinVerifications:             100,         // demonstrate the work, not just pay for entry
		MinAccuracyBps:              800000, // 80% — initial bar; decay below this triggers state machine

		// MinReputationScore + EndorsementMaxOverlapBps are the
		// commitment 8 (panel weights skill, not bond) shape: stake +
		// reputation + endorsement-network-shape, all bounded so that
		// no single dimension can carry a validator past the bar.
		// EndorsementMaxOverlapBps caps how much endorsement weight
		// can come from a tightly-clustered ring; without that cap,
		// a small group of colluding validators could amplify each
		// other's qualification with no external signal.
		MinReputationScore:          500000, // 50%
		QualificationPeriod:         1209600, // ~84 days at 6s blocks
		ProbationPeriod:             302400,  // ~21 days
		RenewalWindow:               100800,  // ~7 days before expiry
		MaxEndorsements:             50,
		CrossRefMinWeight:           30,
		CrossRefWeightDiscountBps:   200000, // 20% discount
		InheritanceWeightDiscountBps: 300000, // 30% discount
		EndorsementMaxOverlapBps:    600000, // 60% — anti-ring guard (L3) for commitment 8

		// Wave 16: accuracy-based qualification decay. Commitment 7
		// (skill is current, not historical) made into a state
		// machine. A validator who was excellent two years ago and
		// has since been wrong on every recent vote should not still
		// be voting with full weight today. The thresholds form a
		// hysteresis loop: drop below 60% → PROBATIONARY (warning,
		// reduced weight); drop below 40% → SUSPENDED (no weight);
		// climb back above 75% → ACTIVE. The gap between probation
		// (60%) and recovery (75%) prevents flapping at the boundary.
		// DecayMinSamples is the floor against decay-from-noise —
		// 20 samples is the smallest count where accuracy is a real
		// statistic rather than a recent streak.
		DecayCheckIntervalBlocks: 10_000,  // ~7 hours at 2.5s blocks — frequent enough to react
		DecayMinSamples:          20,      // 20 samples — decay is a statistic, not a streak
		DecayProbationBps:        600_000, // 60% — fall below: warning state, weight reduced
		DecaySuspensionBps:       400_000, // 40% — fall below: skill is gone, weight is zero
		DecayRecoveryBps:         750_000, // 75% — climb above: skill is current again
	}
}

// Validate validates the qualification module parameters.
func (p *Params) Validate() error {
	minStake := new(big.Int)
	if _, ok := minStake.SetString(p.MinStakeAmount, 10); !ok || minStake.Sign() <= 0 {
		return fmt.Errorf("invalid min_stake_amount: %s", p.MinStakeAmount)
	}
	if p.MinAccuracyBps > 1000000 {
		return fmt.Errorf("min_accuracy_bps cannot exceed 1000000, got %d", p.MinAccuracyBps)
	}
	// Wave 16 decay-threshold ordering must hold for the state machine
	// to make sense: SUSPENSION ≤ PROBATION ≤ RECOVERY.
	if p.DecaySuspensionBps > p.DecayProbationBps {
		return fmt.Errorf("decay_suspension_bps (%d) must be <= decay_probation_bps (%d)",
			p.DecaySuspensionBps, p.DecayProbationBps)
	}
	if p.DecayProbationBps > p.DecayRecoveryBps && p.DecayRecoveryBps > 0 {
		return fmt.Errorf("decay_probation_bps (%d) must be <= decay_recovery_bps (%d)",
			p.DecayProbationBps, p.DecayRecoveryBps)
	}
	if p.DecayRecoveryBps > 1_000_000 {
		return fmt.Errorf("decay_recovery_bps cannot exceed 1,000,000, got %d", p.DecayRecoveryBps)
	}
	if p.CrossRefWeightDiscountBps > 1000000 {
		return fmt.Errorf("cross_ref_weight_discount_bps cannot exceed 1000000, got %d", p.CrossRefWeightDiscountBps)
	}
	if p.InheritanceWeightDiscountBps > 1000000 {
		return fmt.Errorf("inheritance_weight_discount_bps cannot exceed 1000000, got %d", p.InheritanceWeightDiscountBps)
	}
	// Validate domain-specific decay overrides (Scientific Method Gap 5).
	seenDomains := make(map[string]struct{})
	for i, ov := range p.DomainDecayOverrides {
		if ov == nil {
			return fmt.Errorf("domain_decay_overrides[%d]: nil entry", i)
		}
		if ov.Domain == "" {
			return fmt.Errorf("domain_decay_overrides[%d]: domain must not be empty", i)
		}
		if _, dup := seenDomains[ov.Domain]; dup {
			return fmt.Errorf("domain_decay_overrides[%d]: duplicate domain %q", i, ov.Domain)
		}
		seenDomains[ov.Domain] = struct{}{}
		// Threshold ordering must hold when both sides are non-zero.
		if ov.DecaySuspensionBps > 0 && ov.DecayProbationBps > 0 &&
			ov.DecaySuspensionBps > ov.DecayProbationBps {
			return fmt.Errorf("domain_decay_overrides[%d] (%q): decay_suspension_bps (%d) must be <= decay_probation_bps (%d)",
				i, ov.Domain, ov.DecaySuspensionBps, ov.DecayProbationBps)
		}
		if ov.DecayProbationBps > 0 && ov.DecayRecoveryBps > 0 &&
			ov.DecayProbationBps > ov.DecayRecoveryBps {
			return fmt.Errorf("domain_decay_overrides[%d] (%q): decay_probation_bps (%d) must be <= decay_recovery_bps (%d)",
				i, ov.Domain, ov.DecayProbationBps, ov.DecayRecoveryBps)
		}
		if ov.DecayRecoveryBps > 1_000_000 {
			return fmt.Errorf("domain_decay_overrides[%d] (%q): decay_recovery_bps cannot exceed 1,000,000, got %d",
				i, ov.Domain, ov.DecayRecoveryBps)
		}
	}
	return nil
}

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:            DefaultParams(),
		Qualifications:    []*DomainQualification{},
		Endorsements:      []*QualificationEndorsement{},
		NextEndorsementId: 1,
	}
}

// Validate validates the genesis state.
func (gs *GenesisState) Validate() error {
	if gs.Params != nil {
		if err := gs.Params.Validate(); err != nil {
			return fmt.Errorf("invalid params: %w", err)
		}
	}
	seen := make(map[string]bool)
	for _, q := range gs.Qualifications {
		key := q.Validator + "/" + q.Domain
		if seen[key] {
			return fmt.Errorf("duplicate qualification: %s", key)
		}
		seen[key] = true
	}
	return nil
}

// QualificationPenalty represents a temporary reduction in qualification weight.
type QualificationPenalty struct {
	Validator    string `json:"validator"`
	Domain       string `json:"domain"`
	ReductionBps uint64 `json:"reduction_bps"`
	ExpiryHeight uint64 `json:"expiry_height"`
	CreatedAt    uint64 `json:"created_at"`
}

// ---- GetSigners / ValidateBasic for Msg types ----

func (msg *MsgQualifyByStake) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgQualifyByStake) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	amt := new(big.Int)
	if _, ok := amt.SetString(msg.StakeAmount, 10); !ok || amt.Sign() <= 0 {
		return fmt.Errorf("stake_amount must be a positive integer")
	}
	return nil
}

func (msg *MsgQualifyByTrackRecord) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgQualifyByTrackRecord) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	return nil
}

func (msg *MsgQualifyByCrossReference) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgQualifyByCrossReference) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.TargetDomain == "" {
		return fmt.Errorf("target_domain cannot be empty")
	}
	if msg.SourceDomain == "" {
		return fmt.Errorf("source_domain cannot be empty")
	}
	if msg.TargetDomain == msg.SourceDomain {
		return fmt.Errorf("target_domain and source_domain cannot be the same")
	}
	return nil
}

func (msg *MsgQualifyByInheritance) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgQualifyByInheritance) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.TargetDomain == "" {
		return fmt.Errorf("target_domain cannot be empty")
	}
	if msg.ParentDomain == "" {
		return fmt.Errorf("parent_domain cannot be empty")
	}
	if msg.TargetDomain == msg.ParentDomain {
		return fmt.Errorf("target_domain and parent_domain cannot be the same")
	}
	return nil
}

func (msg *MsgEndorseQualification) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Endorser)
	return []sdk.AccAddress{addr}
}

func (msg *MsgEndorseQualification) ValidateBasic() error {
	if msg.Endorser == "" {
		return fmt.Errorf("endorser cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Endorser); err != nil {
		return fmt.Errorf("invalid endorser address: %w", err)
	}
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if msg.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if msg.Weight == 0 || msg.Weight > 100 {
		return fmt.Errorf("weight must be 1-100, got %d", msg.Weight)
	}
	return nil
}

func (msg *MsgRenewQualification) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgRenewQualification) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	return nil
}

func (msg *MsgWithdrawQualification) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Validator)
	return []sdk.AccAddress{addr}
}

func (msg *MsgWithdrawQualification) ValidateBasic() error {
	if msg.Validator == "" {
		return fmt.Errorf("validator cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Validator); err != nil {
		return fmt.Errorf("invalid validator address: %w", err)
	}
	if msg.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	return nil
}

func (msg *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{addr}
}

func (msg *MsgUpdateParams) ValidateBasic() error {
	if msg.Authority == "" {
		return fmt.Errorf("authority cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if msg.Params != nil {
		if err := msg.Params.Validate(); err != nil {
			return fmt.Errorf("invalid params: %w", err)
		}
	}
	return nil
}
