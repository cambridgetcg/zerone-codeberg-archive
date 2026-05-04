package types

import "fmt"

func DefaultParams() *Params {
	return &Params{
		DefaultSwapFeeBps:   3000,          // 0.3%
		MaxPools:            0,             // 0 = unlimited
		MinInitialLiquidity: "10000000000", // 10,000 ZRN in uzrn
		TwapWindowBlocks:    1000,          // ~42 minutes at 2.521s blocks
		ProtocolFeeBps:      450_000,       // 45% of swap fees go to protocol
		MinReserve:          "1",           // minimum reserve after swap
		MaxSwapFractionBps:  100_000,       // 10% of reserve per swap; 0 = disabled
	}
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:           DefaultParams(),
		Pools:            []*Pool{},
		TwapAccumulators: []*TWAPAccumulator{},
	}
}

func (gs *GenesisState) Validate() error {
	if gs.Params == nil {
		return fmt.Errorf("params cannot be nil")
	}
	return gs.Params.Validate()
}

func (p *Params) Validate() error {
	if p.DefaultSwapFeeBps > 1_000_000 {
		return fmt.Errorf("default_swap_fee_bps cannot exceed 1000000")
	}
	// MaxPools 0 = unlimited; no validation needed
	if p.TwapWindowBlocks == 0 {
		return fmt.Errorf("twap_window_blocks must be positive")
	}
	if p.ProtocolFeeBps > 1_000_000 {
		return fmt.Errorf("protocol_fee_bps cannot exceed 1000000")
	}
	if p.MaxSwapFractionBps > 1_000_000 {
		return fmt.Errorf("max_swap_fraction_bps cannot exceed 1000000")
	}
	return nil
}
