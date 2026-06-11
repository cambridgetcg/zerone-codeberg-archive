package keeper

import (
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// aggregateDeterminateNegationType computes the plurality DeterminateNegationType
// from all REJECT-voting reveals in a completed round.
//
// Hegelian principle (Science of Logic §54): "The negation is a determinate
// negation; it has a content." When a claim is rejected, the specific character
// of the failure shapes the more adequate claim that should follow. This function
// distils the panel's collective diagnosis into a single determinate type.
//
// Algorithm:
//  1. Iterate all reveals with vote = "reject".
//  2. Count occurrences of each non-UNSPECIFIED DeterminateNegationType.
//  3. Return the plurality type (highest count > 0); UNSPECIFIED if tied or all unspecified.
//
// Tie-breaking: UNSPECIFIED wins on tie (epistemic humility — if verifiers
// disagree about the failure mode, the chain does not assert a false precision).
// If all reject voters used UNSPECIFIED (or left the field empty), UNSPECIFIED
// is returned.
//
// Non-reject reveals (ACCEPT, MALFORMED) are ignored — their negation_type
// field carries no semantic content.
func aggregateDeterminateNegationType(reveals []*types.RevealEntry) types.DeterminateNegationType {
	counts := make(map[types.DeterminateNegationType]int)
	for _, r := range reveals {
		if r == nil {
			continue
		}
		if r.Vote != "reject" {
			continue // only REJECT votes contribute to negation diagnosis
		}
		if r.NegationType == types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED {
			continue // UNSPECIFIED is the default and does not contribute a positive diagnosis
		}
		counts[r.NegationType]++
	}

	if len(counts) == 0 {
		return types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED
	}

	// Find the plurality (highest count); on tie, return UNSPECIFIED.
	var bestType types.DeterminateNegationType
	bestCount := 0
	tied := false
	for negType, count := range counts {
		if count > bestCount {
			bestType = negType
			bestCount = count
			tied = false
		} else if count == bestCount {
			tied = true
		}
	}

	if tied {
		return types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED
	}
	return bestType
}

// AggregateDeterminateNegationTypeForTest exports aggregateDeterminateNegationType
// for white-box unit tests.
func AggregateDeterminateNegationTypeForTest(reveals []*types.RevealEntry) types.DeterminateNegationType {
	return aggregateDeterminateNegationType(reveals)
}
