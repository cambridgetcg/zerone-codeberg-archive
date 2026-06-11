package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Hegelian Dialectic Gap 2: Aufhebung (Sublation) ─────────────────────────
//
// Hegel (Science of Logic): "That which is sublated is thus something at the
// same time preserved, something that has lost its immediacy but has not come
// to nothing for that."
//
// When a claim is REJECTED, its content is not simply lost. If an accepted Fact
// stands in contradiction to the rejected claim, that Fact is the higher-order
// determination that absorbed the claim's content — the Aufhebung. The chain
// records this link:
//   - Claim.sublation_successor_fact_id → the Fact that achieved the Aufhebung
//   - Claim.sublation_at_block          → when the link was established
//   - Fact.sublated_claim_ids           → inverse link (which claims it sublated)
//
// Detection priority:
//  1. Claim.conflicting_fact_id (canonical-form opposition from Formal Logic Gap 3)
//  2. Claim.self_contradiction_fact_id (Socratic submitter-scope contradiction)
//  3. Scan Claim.relations for CONTRADICTS → accepted/verified Fact

// findSublationSuccessor returns the ID of the accepted Fact that best represents
// the Hegelian sublation of this rejected claim, or "" if none is found.
func (k Keeper) findSublationSuccessor(ctx context.Context, claim *types.Claim) string {
	// Priority 1: canonical-form conflict detected by constraint check (Gap 3).
	// This is the most precise sublation: the rejected claim's canonical form
	// is the logical negation of the successor fact's canonical form.
	if claim.ConflictingFactId != "" {
		if fact, found := k.GetFact(ctx, claim.ConflictingFactId); found && fact != nil {
			if isAcceptedFact(fact) {
				return fact.Id
			}
		}
	}

	// Priority 2: Socratic self-contradiction (same submitter, opposing canonical form).
	// The rejected claim's submitter had previously established the contradicting fact.
	if claim.SelfContradictionFactId != "" {
		if fact, found := k.GetFact(ctx, claim.SelfContradictionFactId); found && fact != nil {
			if isAcceptedFact(fact) {
				return fact.Id
			}
		}
	}

	// Priority 3: scan Claim.relations for explicit CONTRADICTS edges pointing to
	// accepted facts. This is the broadest detection — any explicitly-declared
	// contradiction whose target has been accepted.
	for _, rel := range claim.Relations {
		if rel.Relation != types.RelationType_RELATION_TYPE_CONTRADICTS {
			continue
		}
		if rel.TargetFactId == "" {
			continue
		}
		if fact, found := k.GetFact(ctx, rel.TargetFactId); found && fact != nil {
			if isAcceptedFact(fact) {
				return fact.Id
			}
		}
	}

	return ""
}

// recordSublation wires the Hegelian sublation link on the rejected claim and
// the inverse link on the successor Fact. Called from CompleteRound REJECT path.
//
// Non-blocking: errors updating the successor Fact are silently ignored so they
// do not surface as claim-level failures (the rejection is already committed).
func (k Keeper) recordSublation(ctx context.Context, claim *types.Claim) {
	successorID := k.findSublationSuccessor(ctx, claim)
	if successorID == "" {
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	claim.SublationSuccessorFactId = successorID
	claim.SublationAtBlock = uint64(sdkCtx.BlockHeight())

	// Reverse link: the successor Fact records which claim it sublated.
	// Hegelian Gap 3 (Progressive Enrichment): also increment the fact's
	// enrichment_count and update last_enriched_at_block. Each sublation is an
	// enrichment event — the rejected claim's determination was not lost; its
	// content was preserved and elevated in the successor Fact.
	if successor, found := k.GetFact(ctx, successorID); found && successor != nil {
		successor.SublatedClaimIds = append(successor.SublatedClaimIds, claim.Id)
		successor.EnrichmentCount++
		successor.LastEnrichedAtBlock = uint64(sdkCtx.BlockHeight())
		_ = k.SetFact(ctx, successor) // best-effort; main REJECT flow is unaffected
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.hegelian_sublation",
		sdk.NewAttribute("rejected_claim_id", claim.Id),
		sdk.NewAttribute("sublation_successor_fact_id", successorID),
	))
}

// FindSublationSuccessorForTest exports findSublationSuccessor for white-box tests.
func (k Keeper) FindSublationSuccessorForTest(ctx context.Context, claim *types.Claim) string {
	return k.findSublationSuccessor(ctx, claim)
}
