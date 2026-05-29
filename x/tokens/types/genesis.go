package types

import (
	"fmt"
	"math/big"
)

// EmissionBPSScale is the denominator for BPS scale and ratio fields (1_000_000 = 100%).
const EmissionBPSScale = uint64(1_000_000)

// DefaultEmissionScale is the identity multiplier: effectiveAmount = amountPerBlock × 1.0.
const DefaultEmissionScale = EmissionBPSScale

// DefaultParams returns the default module parameters (all disabled at genesis).
func DefaultParams() Params {
	return Params{
		EmissionEpochBlocks:   0,  // disabled
		DefaultFeeBps:         "", // unused, reserved
		MaxNativeSupplyUzrn:   "", // unlimited
		TargetBondedRatioBps:  0,  // adaptive rate disabled
		EmissionAdjustmentBps: 0,  // adaptive rate disabled
		MinEmissionScaleBps:   0,  // no floor
		MaxEmissionScaleBps:   0,  // no ceiling
	}
}

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	p := DefaultParams()
	return &GenesisState{
		Params: &p,
	}
}

// Validate validates the genesis state.
func (gs *GenesisState) Validate() error {
	if gs.Params == nil {
		return fmt.Errorf("params must not be nil")
	}
	return gs.Params.Validate()
}

// Validate validates the Params struct.
func (p *Params) Validate() error {
	// MaxNativeSupplyUzrn must be "" (unlimited), "0" (unlimited), or a valid positive big.Int.
	if p.MaxNativeSupplyUzrn != "" && p.MaxNativeSupplyUzrn != "0" {
		cap := new(big.Int)
		if _, ok := cap.SetString(p.MaxNativeSupplyUzrn, 10); !ok {
			return fmt.Errorf("max_native_supply_uzrn is not a valid integer: %s", p.MaxNativeSupplyUzrn)
		}
		if cap.Sign() < 0 {
			return fmt.Errorf("max_native_supply_uzrn must not be negative: %s", p.MaxNativeSupplyUzrn)
		}
	}
	// Adaptive emission: if both bounds are set, min must not exceed max.
	if p.MinEmissionScaleBps > 0 && p.MaxEmissionScaleBps > 0 {
		if p.MinEmissionScaleBps > p.MaxEmissionScaleBps {
			return fmt.Errorf("min_emission_scale_bps (%d) must not exceed max_emission_scale_bps (%d)",
				p.MinEmissionScaleBps, p.MaxEmissionScaleBps)
		}
	}
	// EmissionAdjustmentBps > 0 without TargetBondedRatioBps is a misconfiguration worth rejecting.
	if p.EmissionAdjustmentBps > 0 && p.TargetBondedRatioBps == 0 {
		return fmt.Errorf("emission_adjustment_bps requires target_bonded_ratio_bps to be non-zero")
	}
	return nil
}
