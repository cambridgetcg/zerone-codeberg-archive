package types

import "fmt"

// DefaultParams returns the default module parameters.
func DefaultParams() *Params {
	return &Params{
		UnbondingPeriod:          268_560,         // ~7 days at 2521ms blocks
		VirtualStake:             "11000000",      // 11 ZRN (uzrn)
		MaxValidators:            100,
		MinSelfDelegation:        "111000",        // 0.111 ZRN (uzrn)
		MaxSlashesPerEpoch:       2,
		SlashDecayPeriodBlocks:   34_272,          // ~1 day
		MaxSlashCountDeactivate:  3,
		MinStakeForVerification:  "111000",        // 0.111 ZRN
		SlashEscalationBps:       100_000,         // 10%
		ReputationCorrectDelta:   100,             // +0.01%
		ReputationIncorrectDelta: 200,             // -0.02%
		ReputationSlashDelta:     10_000,          // -1%
		RedelegationCooldownBlocks: 1_111,         // ~46 minutes
		TierConfigs:              DefaultTierConfigs(),
		SlashBurnFractionBps:     0,              // default: no burn (all routes to dev fund)
		SlashRateConfigs:         DefaultSlashRateConfigs(),
	}
}

// DefaultGenesisState returns the default genesis state.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:          DefaultParams(),
		Validators:      nil,
		Delegations:     nil,
		UnbondingEntries: nil,
		UnbondingSeq:    0,
	}
}

// Validate validates the genesis state.
func (gs *GenesisState) Validate() error {
	if gs.Params == nil {
		return fmt.Errorf("params cannot be nil")
	}
	if gs.Params.UnbondingPeriod == 0 {
		return fmt.Errorf("unbonding period must be positive")
	}
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Validate tier configs
	if len(gs.Params.TierConfigs) != 4 {
		return fmt.Errorf("params must have exactly 4 tier configs, got %d", len(gs.Params.TierConfigs))
	}

	// Check for duplicate operator addresses
	seen := make(map[string]bool)
	for _, v := range gs.Validators {
		if seen[v.OperatorAddress] {
			return fmt.Errorf("duplicate validator: %s", v.OperatorAddress)
		}
		seen[v.OperatorAddress] = true
	}

	return nil
}
