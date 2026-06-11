package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Scientific Method Gap 2: Methodology Governance ──────────────────────────
//
// UpdateMethodology — authority-gated amendment of a registered methodology.
// Implements Kuhn's "revolutionary science" path: the on-chain methodology
// registry is not static. As the epistemic community's understanding evolves,
// governance can update compliance criteria, falsification paths, cross-method
// discounts, minimum qualification weights, and required account types.
//
// The methodology must already exist in the registry — this handler cannot
// create new methodologies (creation is genesis-only, ensuring the bootstrap
// set goes through the genesis parameter review process).
//
// Every amendment auto-increments the methodology's version and stamps
// ratified_at_block to the current chain height, providing a complete
// amendment audit trail queryable via the Methodologies endpoint.
//
// The rationale is required and stored in the privileged-action log, ensuring
// that every governance-driven change to the epistemic rule set is accompanied
// by a human-readable justification — a requirement Kuhn himself would approve.
func (m *msgServer) UpdateMethodology(ctx context.Context, msg *types.MsgUpdateMethodology) (*types.MsgUpdateMethodologyResponse, error) {
	if msg == nil || msg.Methodology == nil {
		return nil, fmt.Errorf("methodology required")
	}
	if msg.Methodology.Id == "" {
		return nil, fmt.Errorf("methodology.id required")
	}
	if msg.Rationale == "" {
		return nil, fmt.Errorf("rationale required: every methodology amendment must state why")
	}
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s",
			m.keeper.GetAuthority(), msg.Authority)
	}

	// The methodology must already exist. We do not allow governance to create
	// new methodologies via this path — creation is genesis-only.
	current, found := m.keeper.GetMethodology(ctx, msg.Methodology.Id)
	if !found {
		return nil, fmt.Errorf("methodology %q not found: UpdateMethodology amends existing methodologies; new methodologies must be declared in genesis", msg.Methodology.Id)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Auto-manage version and ratified_at_block. The caller supplies the
	// complete desired state for all other fields; we never let a caller
	// regress the version or forge a past ratification block.
	newMethodology := msg.Methodology
	newMethodology.Version = current.Version + 1
	newMethodology.RatifiedAtBlock = uint64(sdkCtx.BlockHeight())

	if err := m.keeper.SetMethodology(ctx, newMethodology); err != nil {
		return nil, fmt.Errorf("failed to store updated methodology: %w", err)
	}

	// Record in the privileged-action log so every amendment is queryable
	// and auditable. The rationale travels in the note field.
	m.keeper.RecordPrivilegedAction(ctx,
		types.PrivilegedActionType_PRIVILEGED_ACTION_TYPE_METHODOLOGY_AMEND,
		msg.Authority,
		fmt.Sprintf("%s@v%d", newMethodology.Id, newMethodology.Version),
		"",
		msg.Rationale,
	)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.methodology_amended",
		sdk.NewAttribute("methodology_id", newMethodology.Id),
		sdk.NewAttribute("new_version", fmt.Sprintf("%d", newMethodology.Version)),
		sdk.NewAttribute("ratified_at_block", fmt.Sprintf("%d", newMethodology.RatifiedAtBlock)),
		sdk.NewAttribute("authority", msg.Authority),
		sdk.NewAttribute("rationale", msg.Rationale),
	))

	return &types.MsgUpdateMethodologyResponse{
		MethodologyId: newMethodology.Id,
		NewVersion:    newMethodology.Version,
	}, nil
}
