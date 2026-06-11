package types

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Emergency status constants.
type EmergencyStatus string

const (
	StatusNormal       EmergencyStatus = "normal"
	StatusHaltVoting   EmergencyStatus = "halt_voting"
	StatusHalted       EmergencyStatus = "halted"
	StatusRevertVoting EmergencyStatus = "revert_voting"
	StatusReverting    EmergencyStatus = "reverting"
	StatusResumeVoting EmergencyStatus = "resume_voting"
)

// Ceremony type constants.
type CeremonyType string

const (
	CeremonyHalt   CeremonyType = "halt"
	CeremonyRevert CeremonyType = "revert"
	CeremonyResume CeremonyType = "resume"
)

// Ceremony phase constants.
type CeremonyPhase string

const (
	PhasePrevote   CeremonyPhase = "prevote"
	PhasePrecommit CeremonyPhase = "precommit"
	PhaseFinalized CeremonyPhase = "finalized"
	PhaseFailed    CeremonyPhase = "failed"
)

// Audit action constants.
type AuditAction string

const (
	AuditHaltProposed    AuditAction = "halt_proposed"
	AuditHaltPrevote     AuditAction = "halt_prevote"
	AuditHaltPrecommit   AuditAction = "halt_precommit"
	AuditHaltExecuted    AuditAction = "halt_executed"
	AuditHaltFailed      AuditAction = "halt_failed"
	AuditRevertProposed  AuditAction = "revert_proposed"
	AuditRevertPrevote   AuditAction = "revert_prevote"
	AuditRevertPrecommit AuditAction = "revert_precommit"
	AuditRevertExecuted  AuditAction = "revert_executed"
	AuditRevertFailed    AuditAction = "revert_failed"
	AuditResumeProposed  AuditAction = "resume_proposed"
	AuditResumePrevote   AuditAction = "resume_prevote"
	AuditResumePrecommit AuditAction = "resume_precommit"
	AuditResumeExecuted  AuditAction = "resume_executed"
	AuditResumeFailed    AuditAction = "resume_failed"
)

// Guardian tier constant (must match staking module TierGuardian value = 4).
const TierGuardian = uint32(4)

// --- Ceremony helpers ---

// GetPrevote returns the prevote for a voter, if it exists.
func (c *EmergencyCeremony) GetPrevote(voter string) (*EmergencyVote, bool) {
	for _, entry := range c.Prevotes {
		if entry.Key == voter {
			return entry.Value, true
		}
	}
	return nil, false
}

// SetPrevote adds or updates a prevote for a voter.
func (c *EmergencyCeremony) SetPrevote(voter string, vote *EmergencyVote) {
	for i, entry := range c.Prevotes {
		if entry.Key == voter {
			c.Prevotes[i].Value = vote
			return
		}
	}
	c.Prevotes = append(c.Prevotes, &PrevoteEntry{Key: voter, Value: vote})
}

// GetPrecommit returns the precommit for a voter, if it exists.
func (c *EmergencyCeremony) GetPrecommit(voter string) (*EmergencyPrecommit, bool) {
	for _, entry := range c.Precommits {
		if entry.Key == voter {
			return entry.Value, true
		}
	}
	return nil, false
}

// SetPrecommit adds or updates a precommit for a voter.
func (c *EmergencyCeremony) SetPrecommit(voter string, precommit *EmergencyPrecommit) {
	for i, entry := range c.Precommits {
		if entry.Key == voter {
			c.Precommits[i].Value = precommit
			return
		}
	}
	c.Precommits = append(c.Precommits, &PrecommitEntry{Key: voter, Value: precommit})
}

// --- Message validation ---

func (msg *MsgProposeHalt) ValidateBasic() error {
	if msg.Proposer == "" {
		return fmt.Errorf("proposer cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Proposer); err != nil {
		return fmt.Errorf("invalid proposer address: %w", err)
	}
	if msg.Reason == "" {
		return fmt.Errorf("reason cannot be empty")
	}
	return nil
}

func (msg *MsgProposeHalt) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Proposer)
	return []sdk.AccAddress{addr}
}

func (msg *MsgVoteHalt) ValidateBasic() error {
	if msg.Voter == "" {
		return fmt.Errorf("voter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Voter); err != nil {
		return fmt.Errorf("invalid voter address: %w", err)
	}
	if msg.ProposalId == "" {
		return fmt.Errorf("proposal_id cannot be empty")
	}
	return nil
}

func (msg *MsgVoteHalt) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Voter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgProposeRevert) ValidateBasic() error {
	if msg.Proposer == "" {
		return fmt.Errorf("proposer cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Proposer); err != nil {
		return fmt.Errorf("invalid proposer address: %w", err)
	}
	if msg.RevertToHeight == 0 {
		return fmt.Errorf("revert_to_height must be > 0")
	}
	return nil
}

func (msg *MsgProposeRevert) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Proposer)
	return []sdk.AccAddress{addr}
}

func (msg *MsgVoteRevert) ValidateBasic() error {
	if msg.Voter == "" {
		return fmt.Errorf("voter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Voter); err != nil {
		return fmt.Errorf("invalid voter address: %w", err)
	}
	if msg.ProposalId == "" {
		return fmt.Errorf("proposal_id cannot be empty")
	}
	return nil
}

func (msg *MsgVoteRevert) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Voter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgProposeResume) ValidateBasic() error {
	if msg.Proposer == "" {
		return fmt.Errorf("proposer cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Proposer); err != nil {
		return fmt.Errorf("invalid proposer address: %w", err)
	}
	return nil
}

func (msg *MsgProposeResume) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Proposer)
	return []sdk.AccAddress{addr}
}

func (msg *MsgVoteResume) ValidateBasic() error {
	if msg.Voter == "" {
		return fmt.Errorf("voter cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Voter); err != nil {
		return fmt.Errorf("invalid voter address: %w", err)
	}
	if msg.ProposalId == "" {
		return fmt.Errorf("proposal_id cannot be empty")
	}
	return nil
}

func (msg *MsgVoteResume) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Voter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgUpdateParams) ValidateBasic() error {
	if msg.Authority == "" {
		return fmt.Errorf("authority cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return fmt.Errorf("invalid authority address: %w", err)
	}
	return nil
}

func (msg *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{addr}
}

// --- Codec registration ---

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgProposeHalt{}, "emergency/MsgProposeHalt", nil)
	cdc.RegisterConcrete(&MsgVoteHalt{}, "emergency/MsgVoteHalt", nil)
	cdc.RegisterConcrete(&MsgProposeRevert{}, "emergency/MsgProposeRevert", nil)
	cdc.RegisterConcrete(&MsgVoteRevert{}, "emergency/MsgVoteRevert", nil)
	cdc.RegisterConcrete(&MsgProposeResume{}, "emergency/MsgProposeResume", nil)
	cdc.RegisterConcrete(&MsgVoteResume{}, "emergency/MsgVoteResume", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "emergency/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgProposeStateCorrection{}, "emergency/MsgProposeStateCorrection", nil)
	cdc.RegisterConcrete(&MsgVoteStateCorrection{}, "emergency/MsgVoteStateCorrection", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgProposeHalt{},
		&MsgVoteHalt{},
		&MsgProposeRevert{},
		&MsgVoteRevert{},
		&MsgProposeResume{},
		&MsgVoteResume{},
		&MsgUpdateParams{},
		&MsgProposeStateCorrection{},
		&MsgVoteStateCorrection{},
	)
}

// IsEmergencyMsg returns true if the given message is an emergency module transaction.
func IsEmergencyMsg(msg sdk.Msg) bool {
	switch msg.(type) {
	case *MsgProposeHalt, *MsgVoteHalt,
		*MsgProposeRevert, *MsgVoteRevert,
		*MsgProposeResume, *MsgVoteResume:
		return true
	default:
		return false
	}
}
