package keeper

import (
	"context"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CheckAdHocRescue implements Popper's criterion for distinguishing legitimate
// from ad-hoc theory modification (Falsificationism Brief Gap 2).
//
// A rescue is potentially ad-hoc if:
//  1. The rescuesClaimId refers to a REJECTED claim, AND
//  2. The new claim's RefutationConditions are NOT a proper superset of the
//     original's — i.e., the submitter has not committed to new falsifiability
//     conditions beyond those the original already carried.
//
// Returns true (ad-hoc) when:
//   - The target claim exists AND its status is REJECTED
//   - AND newConditions ⊄ originalConditions (not a strict superset)
//
// Returns false (legitimate or undecidable) when:
//   - The target claim does not exist (stale reference — ignore, non-blocking)
//   - The target claim was NOT rejected (not a rescue of a failure)
//   - The new claim's conditions are a proper superset of the original's
//
// The check is informational and never blocks claim submission.
func (k Keeper) CheckAdHocRescue(
	ctx context.Context,
	rescuesClaimId string,
	newConditions []string,
) bool {
	if rescuesClaimId == "" {
		return false
	}

	targetClaim, found := k.GetClaim(ctx, rescuesClaimId)
	if !found {
		return false // stale reference — not an error
	}

	// Only flag as ad-hoc if the rescued claim was actually rejected.
	if targetClaim.Status != types.ClaimStatus_CLAIM_STATUS_REJECTED {
		return false
	}

	// Popper's criterion: legitimate rescue must add NEW testable predictions.
	// newConditions must be a *proper superset* of originalConditions.
	//
	// Proper superset means:
	//   1. len(new) > len(original), AND
	//   2. every condition in original is also in new
	//
	// If either condition fails → the rescue is potentially ad-hoc.
	originalConditions := targetClaim.RefutationConditions

	// Edge case: original had zero conditions — any conditions in the new
	// claim are expansions. Rescue is legitimate if new has at least one.
	if len(originalConditions) == 0 {
		return len(newConditions) == 0 // ad-hoc only if new also declares nothing
	}

	// Build a set from new conditions for O(1) lookup.
	newSet := make(map[string]struct{}, len(newConditions))
	for _, c := range newConditions {
		newSet[c] = struct{}{}
	}

	// All original conditions must be covered by new.
	for _, orig := range originalConditions {
		if _, ok := newSet[orig]; !ok {
			return true // new conditions drop an original → ad-hoc
		}
	}

	// New must have strictly more conditions than original.
	if len(newConditions) <= len(originalConditions) {
		return true // same or fewer conditions → no genuine expansion → ad-hoc
	}

	// Proper superset confirmed: legitimate rescue.
	return false
}

// CheckAdHocRescueForTest exports CheckAdHocRescue for white-box unit tests.
func CheckAdHocRescueForTest(k Keeper, ctx context.Context, rescuesClaimId string, newConditions []string) bool {
	return k.CheckAdHocRescue(ctx, rescuesClaimId, newConditions)
}
