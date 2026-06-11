package keeper

import (
	"context"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// defaultTransitiveRelationVerbs are the relation verbs for which transitivity
// is assumed if Params.TransitiveRelationVerbs is empty.
var defaultTransitiveRelationVerbs = []string{"is-a", "implies", "causes", "is-part-of"}

// DeriveConsequences returns the logical consequences that follow from an
// accepted fact via the chain's inference rules:
//
//   - Transitivity: relate(D, A, r, B) + relate(D, B, r, C) where r is in
//     Params.TransitiveRelationVerbs → derive relate(D, A, r, C).
//
//   - Definition substitution: define(D, term, meaning) + assert(D, term, pred)
//     → derive assert(D, meaning, pred).
//
// The source fact must be accepted. Facts used as the second premise must also
// be accepted. Returns an empty slice (never nil) when nothing can be derived.
// isAcceptedFact returns true if the fact has a status that indicates it has
// passed verification (VERIFIED or ACTIVE). Facts can be in either state after
// passing a PoT round — VERIFIED transitions to ACTIVE under Metabolism.
func isAcceptedFact(fact *types.Fact) bool {
	return fact.Status == types.FactStatus_FACT_STATUS_VERIFIED ||
		fact.Status == types.FactStatus_FACT_STATUS_ACTIVE
}

func (k Keeper) DeriveConsequences(ctx context.Context, factID string) []*types.DerivedConsequence {
	source, found := k.GetFact(ctx, factID)
	if !found || !isAcceptedFact(source) {
		return []*types.DerivedConsequence{}
	}
	if source.Structure == nil {
		return []*types.DerivedConsequence{}
	}

	params, err := k.GetParams(ctx)
	if err != nil || params == nil {
		return []*types.DerivedConsequence{}
	}

	var results []*types.DerivedConsequence

	switch source.ClaimType {
	case types.ClaimType_CLAIM_TYPE_RELATION:
		results = append(results, k.deriveTransitivity(ctx, source, *params)...)
	case types.ClaimType_CLAIM_TYPE_DEFINITION:
		results = append(results, k.deriveDefinitionSubstitution(ctx, source)...)
	}

	if results == nil {
		return []*types.DerivedConsequence{}
	}
	return results
}

// deriveTransitivity applies the transitivity rule to a RELATION fact.
// Given relate(D, A, r, B) where r is transitive, finds all accepted RELATION
// facts relate(D, B, r, C) and derives relate(D, A, r, C).
func (k Keeper) deriveTransitivity(ctx context.Context, source *types.Fact, params types.Params) []*types.DerivedConsequence { //nolint:gocritic
	verb := source.Structure.Predicate
	if verb == "" {
		return nil
	}

	transitiveVerbs := params.TransitiveRelationVerbs
	if len(transitiveVerbs) == 0 {
		// Empty list means disabled — no transitivity derivations.
		return nil
	}

	// Is this verb transitive under current params?
	transitive := false
	for _, tv := range transitiveVerbs {
		if tv == verb {
			transitive = true
			break
		}
	}
	if !transitive {
		return nil
	}

	// source: relate(D, A, r, B) — find all relate(D, B, r, C)
	subjectA := source.Structure.Subject
	objectB := source.Structure.Object
	domain := source.Domain

	if objectB == "" {
		return nil
	}

	// Guard against self-loops at the fact level (A == B).
	if subjectA == objectB {
		return nil
	}

	var derived []*types.DerivedConsequence

	k.IterateFactsByDomain(ctx, domain, func(candidateID string) bool {
		if candidateID == source.Id {
			return false // skip self
		}
		candidate, found := k.GetFact(ctx, candidateID)
		if !found {
			return false
		}
		if !isAcceptedFact(candidate) {
			return false
		}
		if candidate.ClaimType != types.ClaimType_CLAIM_TYPE_RELATION {
			return false
		}
		if candidate.Structure == nil {
			return false
		}
		// Match: relate(D, B, r, C) — same domain, same verb, subject == objectB
		if candidate.Domain != domain {
			return false
		}
		if candidate.Structure.Predicate != verb {
			return false
		}
		if candidate.Structure.Subject != objectB {
			return false
		}
		objectC := candidate.Structure.Object
		if objectC == "" {
			return false
		}
		// Skip if A == C (trivial loop)
		if subjectA == objectC {
			return false
		}

		derivedStructure := &types.ClaimStructure{
			Subject:   subjectA,
			Predicate: verb,
			Object:    objectC,
		}
		canonical := types.BuildCanonicalForm(
			types.ClaimType_CLAIM_TYPE_RELATION,
			derivedStructure,
			domain,
		)
		derived = append(derived, &types.DerivedConsequence{
			RuleName:             "transitivity",
			DerivedCanonicalForm: canonical,
			DerivedClaimType:     types.ClaimType_CLAIM_TYPE_RELATION,
			DerivedStructure:     derivedStructure,
			SourceFactIds:        []string{source.Id, candidateID},
		})
		return false
	})

	return derived
}

// deriveDefinitionSubstitution applies the definition substitution rule.
// Given define(D, term, meaning), finds all accepted ASSERTION facts
// assert(D, term, pred) and derives assert(D, meaning, pred).
func (k Keeper) deriveDefinitionSubstitution(ctx context.Context, source *types.Fact) []*types.DerivedConsequence {
	term := source.Structure.Subject    // the term being defined
	meaning := source.Structure.Predicate // what it means
	domain := source.Domain

	if term == "" || meaning == "" {
		return nil
	}

	var derived []*types.DerivedConsequence

	k.IterateFactsByDomain(ctx, domain, func(candidateID string) bool {
		if candidateID == source.Id {
			return false
		}
		candidate, found := k.GetFact(ctx, candidateID)
		if !found {
			return false
		}
		if !isAcceptedFact(candidate) {
			return false
		}
		if candidate.ClaimType != types.ClaimType_CLAIM_TYPE_ASSERTION {
			return false
		}
		if candidate.Structure == nil {
			return false
		}
		if candidate.Domain != domain {
			return false
		}
		// Match: assert(D, term, pred) — subject == term
		if candidate.Structure.Subject != term {
			return false
		}
		pred := candidate.Structure.Predicate
		if pred == "" {
			return false
		}
		// Skip if meaning == term (substitution produces no new fact)
		if meaning == term {
			return false
		}

		derivedStructure := &types.ClaimStructure{
			Subject:   meaning,
			Predicate: pred,
		}
		canonical := types.BuildCanonicalForm(
			types.ClaimType_CLAIM_TYPE_ASSERTION,
			derivedStructure,
			domain,
		)
		derived = append(derived, &types.DerivedConsequence{
			RuleName:             "definition_substitution",
			DerivedCanonicalForm: canonical,
			DerivedClaimType:     types.ClaimType_CLAIM_TYPE_ASSERTION,
			DerivedStructure:     derivedStructure,
			SourceFactIds:        []string{source.Id, candidateID},
		})
		return false
	})

	return derived
}
