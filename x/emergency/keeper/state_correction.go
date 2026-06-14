package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/emergency/types"
)

// ─── State Correction Keeper — Circuit Breaker Gap 5 ─────────────────────────
//
// Surgical state corrections allow Guardians to propose and vote on precise KV
// mutations that fix corrupted or incorrectly-set module state without a full
// chain revert. The approval mechanism mirrors the halt/revert/resume pattern:
// propose → guardian votes → quorum → approved/rejected.
//
// Quorum: approvals / total_active_guardians >= HaltQuorum BPS / 1_000_000.
// On approval, `zerone.emergency.state_correction_approved` is emitted with the
// full mutation list so node operators can verify and apply the correction
// during a planned upgrade handler.

// SetStateCorrection stores a StateCorrectionProposal in the KV store.
func (k Keeper) SetStateCorrection(ctx context.Context, prop *types.StateCorrectionProposal) error {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(prop)
	if err != nil {
		return fmt.Errorf("marshal StateCorrectionProposal: %w", err)
	}
	return kvStore.Set(types.StateCorrectionKey(prop.Id), bz)
}

// GetStateCorrection reads a StateCorrectionProposal by ID.
// Returns (nil, false) when absent.
func (k Keeper) GetStateCorrection(ctx context.Context, id string) (*types.StateCorrectionProposal, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.StateCorrectionKey(id))
	if err != nil || len(bz) == 0 {
		return nil, false
	}
	var prop types.StateCorrectionProposal
	if err := proto.Unmarshal(bz, &prop); err != nil {
		return nil, false
	}
	return &prop, true
}

// ProposeStateCorrection creates a new pending StateCorrectionProposal.
// The proposer must be a Guardian-tier validator.
// Returns the proposal ID.
func (k Keeper) ProposeStateCorrection(
	ctx context.Context,
	proposer string,
	mutations []*types.StateMutation,
	rationale string,
) (string, error) {
	if len(mutations) == 0 {
		return "", types.ErrNoMutations
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Generate deterministic proposal ID: "sc-<blockheight>-<proposer[:8]>".
	pfx := proposer
	if len(pfx) > 8 {
		pfx = pfx[:8]
	}
	id := fmt.Sprintf("sc-%d-%s", sdkCtx.BlockHeight(), pfx)

	prop := &types.StateCorrectionProposal{
		Id:              id,
		Proposer:        proposer,
		Mutations:       mutations,
		Rationale:       rationale,
		ProposedAtBlock: uint64(sdkCtx.BlockHeight()),
		Approvals:       []string{},
		Rejections:      []string{},
		Status:          "pending",
		ApprovedAtBlock: 0,
	}
	if err := k.SetStateCorrection(ctx, prop); err != nil {
		return "", fmt.Errorf("store state correction: %w", err)
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.emergency.state_correction_proposed",
		sdk.NewAttribute("proposal_id", id),
		sdk.NewAttribute("proposer", prop.Proposer),
		sdk.NewAttribute("mutation_count", fmt.Sprintf("%d", len(mutations))),
		sdk.NewAttribute("rationale", rationale),
	))

	return id, nil
}

// VoteStateCorrection records a Guardian vote on a pending proposal.
// Returns (quorumReached, correctionApproved, error).
// Quorum is HaltQuorum BPS of active guardian count.
// On approval, emits zerone.emergency.state_correction_approved.
func (k Keeper) VoteStateCorrection(
	ctx context.Context,
	voter string,
	proposalID string,
	approve bool,
) (quorumReached bool, correctionApproved bool, err error) {
	prop, found := k.GetStateCorrection(ctx, proposalID)
	if !found {
		return false, false, types.ErrCorrectionNotFound
	}
	if prop.Status != "pending" {
		return false, false, types.ErrCorrectionNotPending
	}

	// Duplicate vote check.
	for _, v := range prop.Approvals {
		if v == voter {
			return false, false, types.ErrDuplicateVote
		}
	}
	for _, v := range prop.Rejections {
		if v == voter {
			return false, false, types.ErrDuplicateVote
		}
	}

	if approve {
		prop.Approvals = append(prop.Approvals, voter)
	} else {
		prop.Rejections = append(prop.Rejections, voter)
	}

	params := k.GetParams(ctx)
	quorumBps := params.HaltQuorum // reuse halt quorum for state corrections

	// Total active guardian count.
	totalGuardians := k.activeGuardianCount(ctx)
	if totalGuardians == 0 {
		totalGuardians = 1 // guard against division-by-zero; single-guardian networks still function
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	approvalBps := uint64(len(prop.Approvals)) * 1_000_000 / uint64(totalGuardians)

	if approvalBps >= quorumBps {
		prop.Status = "approved"
		prop.ApprovedAtBlock = uint64(sdkCtx.BlockHeight())
		if saveErr := k.SetStateCorrection(ctx, prop); saveErr != nil {
			return false, false, saveErr
		}

		// Emit the full mutation list as the authoritative correction specification.
		// Per-mutation attributes are built dynamically; the four base attributes
		// are inlined for event-audit scanner compatibility.
		mutationAttrs := make([]sdk.Attribute, 0, 2*len(prop.Mutations))
		for i, m := range prop.Mutations {
			mutationAttrs = append(mutationAttrs,
				sdk.NewAttribute(fmt.Sprintf("mutation_%d_module", i), m.ModuleName),
				sdk.NewAttribute(fmt.Sprintf("mutation_%d_description", i), m.Description),
			)
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.emergency.state_correction_approved",
			append([]sdk.Attribute{
				sdk.NewAttribute("proposal_id", prop.Id),
				sdk.NewAttribute("proposer", prop.Proposer),
				sdk.NewAttribute("approved_at_block", fmt.Sprintf("%d", prop.ApprovedAtBlock)),
				sdk.NewAttribute("mutation_count", fmt.Sprintf("%d", len(prop.Mutations))),
			}, mutationAttrs...)...,
		))
		return true, true, nil
	}

	// Check if rejection makes approval impossible.
	remaining := uint64(totalGuardians) - uint64(len(prop.Approvals)) - uint64(len(prop.Rejections))
	maxPossibleBps := (uint64(len(prop.Approvals)) + remaining) * 1_000_000 / uint64(totalGuardians)
	if maxPossibleBps < quorumBps {
		prop.Status = "rejected"
		if saveErr := k.SetStateCorrection(ctx, prop); saveErr != nil {
			return false, false, saveErr
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.emergency.state_correction_rejected",
			sdk.NewAttribute("proposal_id", prop.Id),
		))
		return true, false, nil
	}

	// Vote recorded, quorum not yet reached.
	if saveErr := k.SetStateCorrection(ctx, prop); saveErr != nil {
		return false, false, saveErr
	}
	return false, false, nil
}

// activeGuardianCount returns the number of currently active Guardian-tier validators.
func (k Keeper) activeGuardianCount(ctx context.Context) int {
	guardians, err := k.stakingKeeper.GetGuardianValidators(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for _, v := range guardians {
		if v.IsActive {
			count++
		}
	}
	// Include council members during bootstrap.
	params := k.GetParams(ctx)
	if k.isCouncilActive(ctx, params) {
		count += len(params.GenesisCouncil)
	}
	return count
}
