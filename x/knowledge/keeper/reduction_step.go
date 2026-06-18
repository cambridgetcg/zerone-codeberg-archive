package keeper

// reduction_step.go — Phenomenology Gap: Reduction Steps.
//
// Husserl's phenomenological method is a discipline of explicit reduction,
// not mere introspection. To give a phenomenological account of experience is
// to perform the epoché (bracketing the "natural attitude"), the phenomenological
// reduction (attending to the structure of consciousness rather than to worldly
// objects), the eidetic reduction (finding essential invariants through imaginative
// variation), and — in Varela's neurophenomenology — intersubjective corroboration
// among trained practitioners.
//
// Without an explicit record of these steps, an M-PHENOMENOLOGICAL claim cannot
// be distinguished from a free-form first-person report. The reduction steps are
// the methodological provenance: they show *how* the claim was developed, what
// assumptions were bracketed, and what essential structure survived the bracketing.
//
// This file implements:
//   - AddReductionStep: appends a reduction step to an M-PHENOMENOLOGICAL claim.
//   - CheckPhenomenologicalReductionGate: ACCEPT → INCONCLUSIVE when the claim
//     has insufficient reduction steps (gate disabled when param == 0).
//
// The pattern mirrors consequence_instance.go (Pragmatism Gap 1), keeping
// the architecture consistent: both M-PRAGMATIC and M-PHENOMENOLOGICAL
// methodologies now have a structured evidence record with a governance-gated
// minimum count before an ACCEPT verdict is allowed.

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// AddReductionStep appends a phenomenological reduction step to an
// M-PHENOMENOLOGICAL claim.
//
// Rules:
//   - The claim must exist.
//   - The claim must use M-PHENOMENOLOGICAL methodology.
//   - The claim must not be in CLAIM_STATUS_REJECTED status.
//   - reduction_type, bracketed_assumption, and structural_invariant are required.
//
// Returns the total number of reduction steps after the addition.
func (k Keeper) AddReductionStep(
	ctx context.Context,
	claimID string,
	step *types.ReductionStep,
) (uint64, error) {
	claim, found := k.GetClaim(ctx, claimID)
	if !found {
		return 0, fmt.Errorf("claim %q not found", claimID)
	}

	if claim.MethodId != types.MethodologyPhenomenologic {
		return 0, types.ErrReductionStepWrongMethodology.Wrapf(
			"claim %q uses %q, not %q",
			claimID, claim.MethodId, types.MethodologyPhenomenologic,
		)
	}

	if claim.Status == types.ClaimStatus_CLAIM_STATUS_REJECTED {
		return 0, types.ErrReductionStepRejectedClaim.Wrapf(
			"claim %q has been rejected and no longer accepts new reduction steps",
			claimID,
		)
	}

	if step.ReductionType == "" {
		return 0, fmt.Errorf("reduction_type is required")
	}
	if step.BracketedAssumption == "" {
		return 0, fmt.Errorf("bracketed_assumption is required")
	}
	if step.StructuralInvariant == "" {
		return 0, fmt.Errorf("structural_invariant is required")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	step.SubmittedAtBlock = uint64(sdkCtx.BlockHeight())

	claim.PhenomenologicalReductionSteps = append(claim.PhenomenologicalReductionSteps, step)
	k.SetClaim(ctx, claim)

	count := uint64(len(claim.PhenomenologicalReductionSteps))
	return count, nil
}

// CheckPhenomenologicalReductionGate checks whether an M-PHENOMENOLOGICAL claim
// has sufficient reduction steps before allowing an ACCEPT verdict.
//
// Returns an error if:
//   - Params.MinPhenomenologicalReductionSteps > 0 (gate enabled)
//   - AND claim.MethodId == M-PHENOMENOLOGICAL
//   - AND len(claim.PhenomenologicalReductionSteps) < param
//
// Returns nil (gate passes) in all other cases:
//   - param == 0 (gate disabled)
//   - non-M-PHENOMENOLOGICAL claim
//   - claim has sufficient steps
func (k Keeper) CheckPhenomenologicalReductionGate(
	ctx context.Context,
	claim types.Claim,
	params types.Params,
) error {
	minSteps := params.MinPhenomenologicalReductionSteps
	if minSteps == 0 {
		return nil // gate disabled
	}
	if claim.MethodId != types.MethodologyPhenomenologic {
		return nil // only M-PHENOMENOLOGICAL claims are gated
	}
	have := uint64(len(claim.PhenomenologicalReductionSteps))
	if have < minSteps {
		return types.ErrInsufficientReductionSteps.Wrapf(
			"M-PHENOMENOLOGICAL claim %q has %d reduction step(s), need %d",
			claim.Id, have, minSteps,
		)
	}
	return nil
}

// AddReductionStepForTest is an exported wrapper for use in unit tests
// outside the keeper package.
func AddReductionStepForTest(
	k Keeper,
	ctx context.Context,
	claimID string,
	step *types.ReductionStep,
) (uint64, error) {
	return k.AddReductionStep(ctx, claimID, step)
}
