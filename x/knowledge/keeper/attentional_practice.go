package keeper

// ─── Contemplative Traditions: Attentional Training Metric ───────────────────
//
// Varela's neurophenomenological requirement: verifiers of phenomenal-state
// claims must have cultivated the attentional instrument, not just the
// domain vocabulary. A verifier who has read Husserl and can apply epoché
// is methodologically qualified; a verifier with 500 hours of vipassanā has
// also trained the instrument of observation itself — a structurally different
// epistemic access. The chain should track both.
//
// AgentCalibration.attentional_practice_hours (field 26): self-declared hours.
// AgentCalibration.attentional_practice_verified_by (field 27): authority verifier.
// Params.min_phenomenological_practice_hours (field 181): governance gate.
//
// Gate: if min_phenomenological_practice_hours > 0 and claim.MethodId == M-PHENOMENOLOGICAL,
// verifiers with attentional_practice_hours < minimum are rejected at MsgSubmitCommitment.

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// SetAttentionalPracticeHours stores a verifier's declared contemplative practice
// hours in their AgentCalibration record. Creates the record if absent.
// verifiedBy should be "" for self-declaration, or the authority address for
// governance-confirmed practice depth.
func (k Keeper) SetAttentionalPracticeHours(ctx context.Context, verifierAddr string, hours uint64, verifiedBy string) error {
	cal := k.getOrInitCalibration(ctx, verifierAddr)
	cal.AttentionalPracticeHours = hours
	cal.AttentionalPracticeVerifiedBy = verifiedBy
	return k.SetAgentCalibration(ctx, cal)
}

// CheckAttentionalPracticeGate returns an error if the given verifier's declared
// practice hours fall below the governance-configured minimum for M-PHENOMENOLOGICAL
// panels. Gate is fail-open: if the param is 0 or the claim is not M-PHENOMENOLOGICAL,
// this is a no-op.
//
// Called from MsgSubmitCommitment after existing methodology gates.
func (k Keeper) CheckAttentionalPracticeGate(ctx context.Context, claim *types.Claim, verifierAddr string, params *types.Params) error {
	min := params.MinPhenomenologicalPracticeHours
	if min == 0 {
		return nil // gate disabled
	}
	if claim.MethodId != types.MethodologyPhenomenologic {
		return nil // gate applies to M-PHENOMENOLOGICAL only
	}

	cal, found := k.GetAgentCalibration(ctx, verifierAddr)
	if !found {
		// No calibration record → practice hours = 0. If gate > 0, reject.
		return types.ErrInsufficientAttentionalPractice.Wrapf(
			"verifier %s has no attentional practice record; %d hours required for M-PHENOMENOLOGICAL panels",
			verifierAddr, min,
		)
	}
	if cal.AttentionalPracticeHours < min {
		return types.ErrInsufficientAttentionalPractice.Wrapf(
			"verifier %s has %d hours of declared attentional practice; %d required for M-PHENOMENOLOGICAL panels",
			verifierAddr, cal.AttentionalPracticeHours, min,
		)
	}
	return nil
}

// MsgSetAttentionalPractice handler — self-declaration or governance-confirmed
// attentional practice hours for M-PHENOMENOLOGICAL panel qualification.
func (m *msgServer) SetAttentionalPractice(ctx context.Context, msg *types.MsgSetAttentionalPractice) (*types.MsgSetAttentionalPracticeResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if msg.VerifierAddress == "" {
		return nil, fmt.Errorf("verifier_address required")
	}
	if msg.Sender == "" {
		return nil, fmt.Errorf("sender required")
	}

	// Authorization: self-declaration or governance authority.
	isAuthority := msg.Sender == m.keeper.GetAuthority()
	isSelf := msg.Sender == msg.VerifierAddress

	if !isSelf && !isAuthority {
		return nil, fmt.Errorf(
			"unauthorized: %s may only declare their own attentional practice hours; "+
				"to set another verifier's hours use the governance authority address",
			msg.Sender,
		)
	}

	// Authority overrides require rationale for audit trail.
	if isAuthority && !isSelf && msg.Rationale == "" {
		return nil, fmt.Errorf("rationale required for authority override of another verifier's attentional practice hours")
	}

	// verifiedBy is the authority address for governance-confirmed records,
	// empty for self-declarations.
	verifiedBy := ""
	if isAuthority && !isSelf {
		verifiedBy = msg.Sender
	}

	if err := m.keeper.SetAttentionalPracticeHours(ctx, msg.VerifierAddress, msg.PracticeHours, verifiedBy); err != nil {
		return nil, fmt.Errorf("failed to store attentional practice hours: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	action := "self-declared"
	if isAuthority && !isSelf {
		action = "authority-verified"
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.attentional_practice_set",
		sdk.NewAttribute("verifier", msg.VerifierAddress),
		sdk.NewAttribute("practice_hours", fmt.Sprintf("%d", msg.PracticeHours)),
		sdk.NewAttribute("action", action),
		sdk.NewAttribute("sender", msg.Sender),
	))

	return &types.MsgSetAttentionalPracticeResponse{
		VerifierAddress: msg.VerifierAddress,
		PracticeHours:   msg.PracticeHours,
		Verified:        isAuthority && !isSelf,
	}, nil
}
