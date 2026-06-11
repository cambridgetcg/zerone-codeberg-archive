package keeper

// ─── Pragmatism: Downstream Utility Tracking ─────────────────────────────────
//
// Peirce's insight that truth is tracked through its downstream consequences
// extends to facts: a fact that nothing ever builds on is epistemically inert.
// A fact that grounds many accepted downstream claims is epistemically vital.
//
// Fact.incoming_citation_count (field 23): incremented every time an accepted
// claim cites this fact as a dependency — either via:
//   - claim.relations (the general fact-to-fact citation graph), or
//   - claim.proof_certificate.premise_fact_ids (formal proof dependencies).
//
// This operationalises Dewey's instrumentalism at the fact layer: a fact's
// epistemic value is partly determined by how much downstream inquiry it enables.
// The fitness scoring (FactFitness) already reads IncomingCitationCount and uses
// it to slow age-related energy decay for well-cited facts — now it has live data.

import "context"

// IncrementIncomingCitationCount records that the given fact has been cited as
// a dependency in a newly accepted claim. No-op if the fact is not found.
// Called from createFactFromClaim for each cited target.
func (k Keeper) IncrementIncomingCitationCount(ctx context.Context, factID string) {
	fact, found := k.GetFact(ctx, factID)
	if !found {
		return // unknown premise — silently skip
	}
	fact.IncomingCitationCount++
	_ = k.SetFact(ctx, fact)
}
