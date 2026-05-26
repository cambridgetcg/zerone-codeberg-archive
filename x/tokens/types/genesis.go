package types

import (
	"fmt"
	"math/big"
)

// DefaultParams returns the default module parameters (all disabled at genesis).
func DefaultParams() Params {
	return Params{
		EmissionEpochBlocks:  0,  // disabled
		DefaultFeeBps:        "", // unused, reserved
		MaxNativeSupplyUzrn:  "", // unlimited
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
	return nil
}
