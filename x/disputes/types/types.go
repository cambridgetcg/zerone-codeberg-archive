package types

import (
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultTierConfigs returns the 4 default tier configurations.
func DefaultTierConfigs() []*TierConfig {
	return []*TierConfig{
		{Tier: 1, ArbiterCount: 3, MinBond: "1000000", EvidencePeriod: 500, VotingPeriod: 1000, QuorumBps: 500000, MajorityBps: 666667},
		{Tier: 2, ArbiterCount: 7, MinBond: "10000000", EvidencePeriod: 1000, VotingPeriod: 2000, QuorumBps: 500000, MajorityBps: 666667},
		{Tier: 3, ArbiterCount: 13, MinBond: "100000000", EvidencePeriod: 2000, VotingPeriod: 5000, QuorumBps: 600000, MajorityBps: 750000},
		{Tier: 4, ArbiterCount: 21, MinBond: "1000000000", EvidencePeriod: 5000, VotingPeriod: 10000, QuorumBps: 666000, MajorityBps: 800000},
	}
}

// DefaultParams returns the default disputes module parameters.
//
// These values express commitment 3 (Popper, not popularity) and
// commitment 10 (forward-only audit). See doc.go for the contract;
// the values below price the falsification opportunity and the
// asymmetric payoffs of being right against a verified fact.
func DefaultParams() *Params {
	return &Params{
		TierConfigs:        DefaultTierConfigs(),

		// MaxActiveDisputes caps concurrent dispute load. 100 is
		// enough that the panel pipeline doesn't bottleneck
		// adjudication, low enough that nuisance flooding is bounded.
		MaxActiveDisputes:  100,

		// EscalationDelay (~21 minutes) is the cooling period before
		// a tier-1 dispute can be escalated. Purpose: deliberation
		// between tiers, not race-to-tier-4.
		EscalationDelay:    500,

		// Asymmetric payoffs (commitment 3): the loser of a dispute
		// loses 50% of their bond; the winner takes 40%; arbiters
		// share 10%. The slash exceeds the reward because the
		// substrate cares more about discouraging frivolous bonds
		// than maximally rewarding correct ones — the chain's
		// epistemic interest is sharper bond-allocation than louder
		// payouts.
		SlashRateLoserBps:  500000,  // 50% — frivolous bonds burn meaningfully
		RewardRateWinnerBps: 400000, // 40% — correct disputes pay above market
		ArbiterRewardBps:   100000,  // 10% — adjudicators are paid for their attention

		// EscrowDisputeWindowBlocks: blocks after CompleteServiceEscrow
		// during which the payer may raise a dispute. ~10 minutes at 2.5s/block.
		EscrowDisputeWindowBlocks: 250,
	}
}

// Validate validates the disputes module parameters.
func (p *Params) Validate() error {
	if len(p.TierConfigs) == 0 {
		return fmt.Errorf("at least one tier config is required")
	}
	for _, tc := range p.TierConfigs {
		if tc.Tier == 0 || tc.Tier > 4 {
			return fmt.Errorf("tier must be 1-4, got %d", tc.Tier)
		}
		if tc.ArbiterCount == 0 {
			return fmt.Errorf("tier %d: arbiter_count must be positive", tc.Tier)
		}
		bond := new(big.Int)
		if _, ok := bond.SetString(tc.MinBond, 10); !ok || bond.Sign() <= 0 {
			return fmt.Errorf("tier %d: invalid min_bond: %s", tc.Tier, tc.MinBond)
		}
		if tc.EvidencePeriod == 0 {
			return fmt.Errorf("tier %d: evidence_period must be positive", tc.Tier)
		}
		if tc.VotingPeriod == 0 {
			return fmt.Errorf("tier %d: voting_period must be positive", tc.Tier)
		}
		if tc.QuorumBps > 1000000 {
			return fmt.Errorf("tier %d: quorum_bps must be <= 1000000", tc.Tier)
		}
		if tc.MajorityBps > 1000000 {
			return fmt.Errorf("tier %d: majority_bps must be <= 1000000", tc.Tier)
		}
	}
	if p.SlashRateLoserBps > 1000000 {
		return fmt.Errorf("slash_rate_loser_bps must be <= 1000000")
	}
	if p.RewardRateWinnerBps > 1000000 {
		return fmt.Errorf("reward_rate_winner_bps must be <= 1000000")
	}
	if p.ArbiterRewardBps > 1000000 {
		return fmt.Errorf("arbiter_reward_bps must be <= 1000000")
	}
	if p.RewardRateWinnerBps+p.ArbiterRewardBps > 1000000 {
		return fmt.Errorf("reward_rate_winner_bps + arbiter_reward_bps must be <= 1000000")
	}
	// EscrowDisputeWindowBlocks=0 means dispute window is disabled (payer cannot dispute after completion).
	// Any non-zero value is valid.
	return nil
}

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:   DefaultParams(),
		Disputes: []*Dispute{},
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
	for _, d := range gs.Disputes {
		if seen[d.Id] {
			return fmt.Errorf("duplicate dispute id: %s", d.Id)
		}
		seen[d.Id] = true
	}
	return nil
}

// GetTierConfig returns the config for a given tier, or nil.
func GetTierConfig(params *Params, tier uint32) *TierConfig {
	for _, tc := range params.TierConfigs {
		if tc.Tier == tier {
			return tc
		}
	}
	return nil
}

// ---- GetSigners / ValidateBasic for Msg types ----

func (msg *MsgInitiateDispute) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Challenger)
	return []sdk.AccAddress{addr}
}

func (msg *MsgInitiateDispute) ValidateBasic() error {
	if msg.Challenger == "" {
		return fmt.Errorf("challenger cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Challenger); err != nil {
		return fmt.Errorf("invalid challenger address: %w", err)
	}
	if msg.TargetType == DisputeTargetType_DISPUTE_TARGET_TYPE_UNSPECIFIED {
		return fmt.Errorf("target_type cannot be unspecified")
	}
	if msg.TargetId == "" {
		return fmt.Errorf("target_id cannot be empty")
	}
	if msg.Reason == "" {
		return fmt.Errorf("reason cannot be empty")
	}
	bond := new(big.Int)
	if _, ok := bond.SetString(msg.Bond, 10); !ok || bond.Sign() <= 0 {
		return fmt.Errorf("bond must be a positive integer")
	}
	return nil
}

func (msg *MsgCommitEvidence) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Submitter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgCommitEvidence) ValidateBasic() error {
	if msg.Submitter == "" {
		return fmt.Errorf("submitter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	if msg.CommitmentHash == "" {
		return fmt.Errorf("commitment_hash cannot be empty")
	}
	return nil
}

func (msg *MsgRevealEvidence) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Submitter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgRevealEvidence) ValidateBasic() error {
	if msg.Submitter == "" {
		return fmt.Errorf("submitter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Submitter); err != nil {
		return fmt.Errorf("invalid submitter address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	if msg.Content == "" {
		return fmt.Errorf("content cannot be empty")
	}
	if msg.Nonce == "" {
		return fmt.Errorf("nonce cannot be empty")
	}
	return nil
}

func (msg *MsgArbiterVote) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Arbiter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgArbiterVote) ValidateBasic() error {
	if msg.Arbiter == "" {
		return fmt.Errorf("arbiter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Arbiter); err != nil {
		return fmt.Errorf("invalid arbiter address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	if msg.Vote == ArbiterDecision_ARBITER_DECISION_UNSPECIFIED {
		return fmt.Errorf("vote cannot be unspecified")
	}
	return nil
}

func (msg *MsgEscalateDispute) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Requester)
	return []sdk.AccAddress{addr}
}

func (msg *MsgEscalateDispute) ValidateBasic() error {
	if msg.Requester == "" {
		return fmt.Errorf("requester cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Requester); err != nil {
		return fmt.Errorf("invalid requester address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	bond := new(big.Int)
	if _, ok := bond.SetString(msg.AdditionalBond, 10); !ok || bond.Sign() <= 0 {
		return fmt.Errorf("additional_bond must be a positive integer")
	}
	return nil
}

func (msg *MsgSettleDispute) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{addr}
}

func (msg *MsgSettleDispute) ValidateBasic() error {
	if msg.Authority == "" {
		return fmt.Errorf("authority cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	return nil
}

// ---- Service Escrow Msg types ----

func (msg *MsgCreateServiceEscrow) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Payer)
	return []sdk.AccAddress{addr}
}

func (msg *MsgCreateServiceEscrow) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Payer); err != nil {
		return fmt.Errorf("invalid payer address: %w", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Provider); err != nil {
		return fmt.Errorf("invalid provider address: %w", err)
	}
	if msg.Payer == msg.Provider {
		return fmt.Errorf("payer and provider must be different addresses")
	}
	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return fmt.Errorf("amount must be a positive integer")
	}
	if msg.TermsHash == "" {
		return fmt.Errorf("terms_hash cannot be empty")
	}
	if msg.Deadline == 0 {
		return fmt.Errorf("deadline must be a positive block height")
	}
	return nil
}

func (msg *MsgCompleteServiceEscrow) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Provider)
	return []sdk.AccAddress{addr}
}

func (msg *MsgCompleteServiceEscrow) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Provider); err != nil {
		return fmt.Errorf("invalid provider address: %w", err)
	}
	if msg.EscrowId == "" {
		return fmt.Errorf("escrow_id cannot be empty")
	}
	if msg.ProofHash == "" {
		return fmt.Errorf("proof_hash cannot be empty")
	}
	return nil
}

func (msg *MsgDisputeServiceEscrow) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Payer)
	return []sdk.AccAddress{addr}
}

func (msg *MsgDisputeServiceEscrow) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Payer); err != nil {
		return fmt.Errorf("invalid payer address: %w", err)
	}
	if msg.EscrowId == "" {
		return fmt.Errorf("escrow_id cannot be empty")
	}
	return nil
}

// ---- Defender Bond Msg types ----

func (msg *MsgDefendDispute) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Defender)
	return []sdk.AccAddress{addr}
}

func (msg *MsgDefendDispute) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Defender); err != nil {
		return fmt.Errorf("invalid defender address: %w", err)
	}
	if msg.DisputeId == "" {
		return fmt.Errorf("dispute_id cannot be empty")
	}
	bond := new(big.Int)
	if _, ok := bond.SetString(msg.Bond, 10); !ok || bond.Sign() <= 0 {
		return fmt.Errorf("bond must be a positive integer")
	}
	return nil
}
