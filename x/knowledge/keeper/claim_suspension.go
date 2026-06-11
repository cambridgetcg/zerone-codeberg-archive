package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Bohm Dialogue Gap 1: Claim Suspension ────────────────────────────────────
//
// Encodes David Bohm's practice of suspension: "hold the assumption before
// oneself like an object in a mirror — visible, examined, but not acted upon."
// (*On Dialogue*, 1996, p.22.)
//
// A SUSPENDED claim is held in a mirror-state:
//   - Fully queryable (canonical form, relations, contradiction flags all visible)
//   - No verification round may be opened on it (CreateVerificationRound rejects)
//   - Neither suppressed (REJECTED) nor asserted (ACCEPTED)
//
// This is structurally distinct from REJECTED (terminal) and EXPIRED (timeout).
// SUSPENDED is a reversible, deliberate pause — the chain "holds" the claim
// in examination while the epistemic community deliberates.
//
// Only governance authority may suspend or resume claims.
// Requires Params.SuspensionEnabled = true.

// suspendableStatuses lists the claim statuses from which a claim may be
// suspended. Terminal statuses (ACCEPTED, REJECTED, EXPIRED, MALFORMED) cannot
// be suspended — they have already resolved.
var suspendableStatuses = map[types.ClaimStatus]bool{
	types.ClaimStatus_CLAIM_STATUS_PENDING:             true,
	types.ClaimStatus_CLAIM_STATUS_PENDING_EVALUATION:  true,
	types.ClaimStatus_CLAIM_STATUS_EVALUATED:           true,
	types.ClaimStatus_CLAIM_STATUS_PROVISIONAL:         true,
	types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION:     true,
	types.ClaimStatus_CLAIM_STATUS_CHALLENGED:          true,
	types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT:        true,
	types.ClaimStatus_CLAIM_STATUS_CONTESTED:           true,
}

// SuspendClaim places a claim into CLAIM_STATUS_SUSPENDED. Authority-gated.
// Records suspended_at_block and suspension_reason on the claim.
// Emits zerone.knowledge.claim_suspended.
func (m *msgServer) SuspendClaim(ctx context.Context, msg *types.MsgSuspendClaim) (*types.MsgSuspendClaimResponse, error) {
	// Authority check.
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, types.ErrUnauthorized.Wrapf("only %s may suspend claims; got %s",
			m.keeper.GetAuthority(), msg.Authority)
	}

	params, err := m.keeper.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if !params.SuspensionEnabled {
		return nil, types.ErrSuspensionDisabled
	}

	if msg.Reason == "" {
		return nil, types.ErrInvalidClaim.Wrap("suspension reason is required (name the assumption being held in mirror-state)")
	}

	claim, found := m.keeper.GetClaim(ctx, msg.ClaimId)
	if !found {
		return nil, types.ErrClaimNotFound.Wrapf("claim %q does not exist", msg.ClaimId)
	}

	if claim.Status == types.ClaimStatus_CLAIM_STATUS_SUSPENDED {
		return nil, types.ErrClaimAlreadySuspended.Wrapf("claim %q is already SUSPENDED", msg.ClaimId)
	}

	if !suspendableStatuses[claim.Status] {
		return nil, types.ErrClaimNotSuspendable.Wrapf("claim %q has terminal status %s; cannot suspend",
			msg.ClaimId, claim.Status)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	claim.Status = types.ClaimStatus_CLAIM_STATUS_SUSPENDED
	claim.SuspendedAtBlock = uint64(sdkCtx.BlockHeight())
	claim.SuspensionReason = msg.Reason

	if err := m.keeper.SetClaim(ctx, claim); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.claim_suspended",
		sdk.NewAttribute("claim_id", msg.ClaimId),
		sdk.NewAttribute("authority", msg.Authority),
		sdk.NewAttribute("reason", msg.Reason),
	))

	return &types.MsgSuspendClaimResponse{
		ClaimId:          msg.ClaimId,
		SuspendedAtBlock: claim.SuspendedAtBlock,
	}, nil
}

// ResumeClaim lifts a SUSPENDED claim back to IN_VERIFICATION status.
// Clears suspended_at_block and suspension_reason. Authority-gated.
// Emits zerone.knowledge.claim_resumed.
func (m *msgServer) ResumeClaim(ctx context.Context, msg *types.MsgResumeClaim) (*types.MsgResumeClaimResponse, error) {
	// Authority check.
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, types.ErrUnauthorized.Wrapf("only %s may resume claims; got %s",
			m.keeper.GetAuthority(), msg.Authority)
	}

	claim, found := m.keeper.GetClaim(ctx, msg.ClaimId)
	if !found {
		return nil, types.ErrClaimNotFound.Wrapf("claim %q does not exist", msg.ClaimId)
	}

	if claim.Status != types.ClaimStatus_CLAIM_STATUS_SUSPENDED {
		return nil, types.ErrClaimNotSuspended.Wrapf("claim %q has status %s; only SUSPENDED claims may be resumed",
			msg.ClaimId, claim.Status)
	}

	claim.Status = types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION
	claim.SuspendedAtBlock = 0
	claim.SuspensionReason = ""

	if err := m.keeper.SetClaim(ctx, claim); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.claim_resumed",
		sdk.NewAttribute("claim_id", msg.ClaimId),
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgResumeClaimResponse{
		ClaimId: msg.ClaimId,
	}, nil
}
