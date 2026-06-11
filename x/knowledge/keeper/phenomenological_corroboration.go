package keeper

import (
	"context"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CheckPhenomenologicalCorroborationGate enforces Varela's neurophenomenological
// principle: high-confidence M-PHENOMENOLOGICAL claims require cross-modal
// grounding in M-EMPIRICAL evidence. When params.PhenomenologicalCorroborationConfidenceBps
// is non-zero and the claim's M-PHENOMENOLOGICAL verdict would be ACCEPT at
// confidence >= threshold, this gate verifies that at least one of the claim's
// ClaimRelations points to an accepted M-EMPIRICAL fact.
//
// Returns true (gate passed) if:
//   - The gate is disabled (param == 0)
//   - The claim methodology is not M-PHENOMENOLOGICAL
//   - The confidence is below the threshold
//   - At least one ClaimRelation targets an accepted M-EMPIRICAL fact
//
// Returns false (downgrade to INCONCLUSIVE) if:
//   - The claim is M-PHENOMENOLOGICAL, confidence >= threshold,
//     AND no ClaimRelation targets an M-EMPIRICAL fact.
//
// Phenomenology Brief Gap 3 (★★★☆☆): cross-modal corroboration requirement.
// Varela, Thompson, Rosch (1991) — "The Embodied Mind": first-person
// phenomenological descriptions and third-person neural/behavioral data should
// mutually constrain each other. Neither alone is sufficient for high-confidence
// acceptance. This gate encodes that principle as a governance-activatable rule.
func (k Keeper) CheckPhenomenologicalCorroborationGate(
	ctx context.Context,
	claim *types.Claim,
	confidence uint64,
	params types.Params,
) bool {
	threshold := params.PhenomenologicalCorroborationConfidenceBps

	// Gate disabled: always pass.
	if threshold == 0 {
		return true
	}

	// Not a phenomenological claim: gate does not apply.
	if claim.MethodId != types.MethodologyPhenomenologic {
		return true
	}

	// Below confidence threshold: no corroboration required.
	if confidence < threshold {
		return true
	}

	// High-confidence M-PHENOMENOLOGICAL: require at least one M-EMPIRICAL relation.
	for _, rel := range claim.Relations {
		if rel.TargetFactId == "" {
			continue
		}
		fact, found := k.GetFact(ctx, rel.TargetFactId)
		if !found {
			continue
		}
		if fact.MethodId == types.MethodologyEmpirical {
			return true // found M-EMPIRICAL corroboration
		}
	}

	// No M-EMPIRICAL relation found: gate fails → downgrade to INCONCLUSIVE.
	return false
}

// CheckPhenomenologicalCorroborationGateForTest is exported for unit tests.
// Wraps CheckPhenomenologicalCorroborationGate with test-friendly parameter passing.
func CheckPhenomenologicalCorroborationGateForTest(
	k Keeper,
	ctx context.Context,
	claim *types.Claim,
	confidence uint64,
	params types.Params,
) bool {
	return k.CheckPhenomenologicalCorroborationGate(ctx, claim, confidence, params)
}
