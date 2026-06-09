package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// AddConsequenceInstance appends a pragmatic consequence instance to an M-PRAGMATIC claim.
//
// Rules:
//   - The claim must exist.
//   - The claim must use M-PRAGMATIC methodology.
//   - The claim must not be in CLAIM_STATUS_REJECTED status.
//   - context_description, predicted_outcome, and observed_outcome are required.
//
// Returns the total number of consequence instances after the addition.
func (k Keeper) AddConsequenceInstance(
	ctx context.Context,
	claimID string,
	instance *types.ConsequenceInstance,
) (uint64, error) {
	claim, found := k.GetClaim(ctx, claimID)
	if !found {
		return 0, fmt.Errorf("claim %q not found", claimID)
	}

	if claim.MethodId != types.MethodologyPragmatic {
		return 0, types.ErrConsequenceInstanceWrongMethodology.Wrapf(
			"claim %q uses %q, not %q",
			claimID, claim.MethodId, types.MethodologyPragmatic,
		)
	}

	if claim.Status == types.ClaimStatus_CLAIM_STATUS_REJECTED {
		return 0, types.ErrConsequenceInstanceRejectedClaim.Wrapf(
			"claim %q has been rejected and no longer accepts new instances",
			claimID,
		)
	}

	if instance.ContextDescription == "" {
		return 0, fmt.Errorf("context_description is required")
	}
	if instance.PredictedOutcome == "" {
		return 0, fmt.Errorf("predicted_outcome is required")
	}
	if instance.ObservedOutcome == "" {
		return 0, fmt.Errorf("observed_outcome is required")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	instance.SubmittedAtBlock = uint64(sdkCtx.BlockHeight())

	claim.ConsequenceInstances = append(claim.ConsequenceInstances, instance)
	k.SetClaim(ctx, claim)

	count := uint64(len(claim.ConsequenceInstances))
	return count, nil
}

// CheckPragmaticConsequenceGate checks whether an M-PRAGMATIC claim has sufficient
// consequence instances before allowing an ACCEPT verdict.
//
// Returns an error if:
//   - Params.MinPragmaticConsequenceInstances > 0 (gate enabled)
//   - AND claim.MethodId == M-PRAGMATIC
//   - AND len(claim.ConsequenceInstances) < param
//
// Returns nil (gate passes) in all other cases:
//   - param == 0 (gate disabled)
//   - non-M-PRAGMATIC claim
//   - claim has sufficient instances
func (k Keeper) CheckPragmaticConsequenceGate(
	ctx context.Context,
	claim types.Claim,
	params types.Params,
) error {
	minInstances := params.MinPragmaticConsequenceInstances
	if minInstances == 0 {
		return nil // gate disabled
	}
	if claim.MethodId != types.MethodologyPragmatic {
		return nil // only M-PRAGMATIC claims are gated
	}
	have := uint64(len(claim.ConsequenceInstances))
	if have < minInstances {
		return types.ErrInsufficientConsequenceInstances.Wrapf(
			"M-PRAGMATIC claim %q has %d consequence instance(s), need %d",
			claim.Id, have, minInstances,
		)
	}
	return nil
}

// AddConsequenceInstanceForTest is an exported wrapper for use in unit tests.
func AddConsequenceInstanceForTest(
	k Keeper,
	ctx context.Context,
	claimID string,
	instance *types.ConsequenceInstance,
) (uint64, error) {
	return k.AddConsequenceInstance(ctx, claimID, instance)
}
