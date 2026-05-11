package types

import (
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	commontypes "github.com/zerone-chain/zerone/x/common/types"
)

// DefaultRevenueSplit returns the default 55/22/3.33/19.67 revenue split.
func DefaultRevenueSplit() *commontypes.RevenueSplit {
	return &commontypes.RevenueSplit{
		ContributorBps: 550000,  // 55%
		ProtocolBps:    220000,  // 22%
		ResearchBps:    33300,   // 3.33%
		DevelopmentBps: 196700,  // 19.67%
	}
}

// DefaultDynamicPricingConfig returns a disabled dynamic pricing config.
func DefaultDynamicPricingConfig() *DynamicPricingConfig {
	return &DynamicPricingConfig{
		Enabled:            false,
		TargetQueryCostUsd: "10000",     // $0.01 in 6-decimal USD
		ManualZrnPriceUsd:  "0",         // disabled
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",      // 0.001 ZRN floor
		MaxCostPerFact:     "100000000", // 100 ZRN ceiling
	}
}

// DefaultParams returns the default billing module parameters.
func DefaultParams() *Params {
	return &Params{
		BaseQueryPrice:       "1000000",   // 1 ZRN per fact query
		ConfidenceWeightBps:  200000,      // 20% adjustment range
		NoveltyWeightBps:     0,           // informational
		FreshnessWeightBps:   100000,      // 10% freshness premium
		RevenueSplit:         DefaultRevenueSplit(),
		DynamicPricing:       DefaultDynamicPricingConfig(),
		MinProviderStake:     "100000000", // 100 ZRN
		ConfidenceThreshold:  500000,      // 50% on 1M scale
		FreshnessWindowBlocks: 1000,
		QuoteValidityBlocks:  100,
		// 10% per proposal — governance can spend up to 10% of development_fund balance
		// in a single MsgDevelopmentFundSpend. Prevents single-block drain.
		DevelopmentFundSpendingLimitBps: 100000,
		// 0 = disabled. When non-zero, spending is frozen if protocol treasury
		// balance falls below this fraction of combined (protocol + development) funds.
		MinProtocolReserveBps: 0,
	}
}

// Validate validates the billing module parameters.
func (p *Params) Validate() error {
	base := new(big.Int)
	if _, ok := base.SetString(p.BaseQueryPrice, 10); !ok || base.Sign() <= 0 {
		return fmt.Errorf("invalid base_query_price: %s", p.BaseQueryPrice)
	}
	if p.ConfidenceThreshold > 1000000 {
		return fmt.Errorf("confidence_threshold must be <= 1000000 bps")
	}
	if p.ConfidenceWeightBps > 1000000 {
		return fmt.Errorf("confidence_weight_bps must be <= 1000000 bps")
	}
	if p.FreshnessWindowBlocks == 0 {
		return fmt.Errorf("freshness_window_blocks must be positive")
	}
	if p.FreshnessWeightBps > 1000000 {
		return fmt.Errorf("freshness_weight_bps must be <= 1000000 bps")
	}
	if p.QuoteValidityBlocks == 0 {
		return fmt.Errorf("quote_validity_blocks must be positive")
	}
	stake := new(big.Int)
	if _, ok := stake.SetString(p.MinProviderStake, 10); !ok || stake.Sign() <= 0 {
		return fmt.Errorf("invalid min_provider_stake: %s", p.MinProviderStake)
	}
	if p.RevenueSplit != nil {
		sum := p.RevenueSplit.ContributorBps + p.RevenueSplit.ProtocolBps + p.RevenueSplit.ResearchBps + p.RevenueSplit.DevelopmentBps
		if sum != 1000000 {
			return fmt.Errorf("revenue_split bps must sum to 1000000, got %d", sum)
		}
	}
	if p.DevelopmentFundSpendingLimitBps > 1000000 {
		return fmt.Errorf("development_fund_spending_limit_bps must be <= 1000000 bps")
	}
	if p.MinProtocolReserveBps > 1000000 {
		return fmt.Errorf("min_protocol_reserve_bps must be <= 1000000 bps")
	}
	return nil
}

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:    DefaultParams(),
		Providers: []*Provider{},
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
	for _, p := range gs.Providers {
		if seen[p.Address] {
			return fmt.Errorf("duplicate provider address: %s", p.Address)
		}
		seen[p.Address] = true
	}
	return nil
}

// ---- GetSigners / ValidateBasic for Msg types ----

func (msg *MsgRegisterProvider) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Sender)
	return []sdk.AccAddress{addr}
}

func (msg *MsgRegisterProvider) ValidateBasic() error {
	if msg.Sender == "" {
		return fmt.Errorf("sender cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Sender); err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	if len(msg.Domains) == 0 {
		return fmt.Errorf("at least one domain is required")
	}
	amt := new(big.Int)
	if _, ok := amt.SetString(msg.Stake, 10); !ok || amt.Sign() <= 0 {
		return fmt.Errorf("stake amount must be a positive integer")
	}
	return nil
}

func (msg *MsgDeregisterProvider) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Sender)
	return []sdk.AccAddress{addr}
}

func (msg *MsgDeregisterProvider) ValidateBasic() error {
	if msg.Sender == "" {
		return fmt.Errorf("sender cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Sender); err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	return nil
}

func (msg *MsgQueryFact) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Sender)
	return []sdk.AccAddress{addr}
}

func (msg *MsgQueryFact) ValidateBasic() error {
	if msg.Sender == "" {
		return fmt.Errorf("sender cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Sender); err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	if msg.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Provider); err != nil {
		return fmt.Errorf("invalid provider address: %w", err)
	}
	if msg.FactId == "" {
		return fmt.Errorf("fact_id cannot be empty")
	}
	return nil
}

func (msg *MsgBatchQueryFacts) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Sender)
	return []sdk.AccAddress{addr}
}

func (msg *MsgBatchQueryFacts) ValidateBasic() error {
	if msg.Sender == "" {
		return fmt.Errorf("sender cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Sender); err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	if msg.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Provider); err != nil {
		return fmt.Errorf("invalid provider address: %w", err)
	}
	if len(msg.FactIds) == 0 {
		return fmt.Errorf("at least one fact_id is required")
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
	if msg.Params == nil {
		return fmt.Errorf("params cannot be nil")
	}
	return msg.Params.Validate()
}

func (msg *MsgDevelopmentFundSpend) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{addr}
}

func (msg *MsgDevelopmentFundSpend) ValidateBasic() error {
	if msg.Authority == "" {
		return fmt.Errorf("authority cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if msg.Recipient == "" {
		return fmt.Errorf("recipient cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Recipient); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	amt := new(big.Int)
	if _, ok := amt.SetString(msg.Amount, 10); !ok || amt.Sign() <= 0 {
		return fmt.Errorf("amount must be a positive integer, got %q", msg.Amount)
	}
	return nil
}
