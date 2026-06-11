package keeper

import (
	"context"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CheckSocraticConsistency implements the Socratic elenctic self-consistency
// check (Socratic Dialectic Brief Gap 1).
//
// The Socratic elenchus traps interlocutors by their own prior commitments:
// rather than attacking from outside, Socrates forced each person to confront
// how their new claim contradicted what they had already agreed to be true.
//
// On-chain analogue: when a submitter provides a canonical_form for their new
// claim, this function checks whether the same submitter already has an accepted
// Fact whose canonical form is the logical OPPOSITE. If so, the submitter is
// committing to something they have already publicly refuted under their own
// authority — a self-contradiction in the Socratic sense.
//
// Implementation:
//  1. Compute the opposing canonical hash via ComputeOpposingCanonicalHash.
//  2. Look up that hash in the canonical hash index → returns a Fact or Claim ID.
//  3. Fetch the entity; if it is a Fact and the submitter matches → self-contradiction.
//  4. (Claims that were accepted but not yet fact-indexed are covered when they
//     become Facts; checking only Facts ensures we flag established commitments.)
//
// Returns (isSelfContradictory bool, selfContradictionFactId string).
// Non-blocking: callers store the result and emit an event, but do not halt
// claim submission. Returns (false, "") when:
//   - The canonical form is empty (detection disabled)
//   - No opposing canonical form exists for this claim type
//   - No fact exists for the opposing hash
//   - The opposing fact was submitted by a different submitter
func (k Keeper) CheckSocraticConsistency(
	ctx context.Context,
	submitter string,
	canonicalForm string,
) (bool, string) {
	if canonicalForm == "" || submitter == "" {
		return false, ""
	}

	opposingHash := types.ComputeOpposingCanonicalHash(canonicalForm)
	if opposingHash == "" {
		return false, "" // canonical form has no computable opposite
	}

	entityID, exists := k.GetClaimByCanonicalHash(ctx, opposingHash)
	if !exists {
		return false, "" // no entity with the opposing canonical hash on-chain
	}

	// Only flag as self-contradictory if the opposing entity is an accepted Fact
	// (not a pending Claim), and the submitter matches.
	fact, factFound := k.GetFact(ctx, entityID)
	if !factFound {
		return false, "" // opposing hash maps to a Claim (pending/rejected), not a Fact
	}

	if fact.Submitter != submitter {
		return false, "" // the contradiction is with another submitter — not Socratic
	}

	return true, entityID
}

// CheckSocraticConsistencyForTest exports CheckSocraticConsistency for white-box tests.
func CheckSocraticConsistencyForTest(
	k Keeper,
	ctx context.Context,
	submitter string,
	canonicalForm string,
) (bool, string) {
	return k.CheckSocraticConsistency(ctx, submitter, canonicalForm)
}
