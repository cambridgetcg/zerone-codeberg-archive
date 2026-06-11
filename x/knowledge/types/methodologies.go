package types

// Methodology ID constants. Stable identifiers referenced on-chain by every
// claim and fact submitted under the "methodology over statement" model.
//
// The bootstrap methodologies are grounded in recognized epistemic traditions:
//
//   M-FORMAL          — foundationalism (Frege, Russell, Hilbert): truth-
//                       preserving inference from stated premises in a named
//                       formal system.
//   M-EMPIRICAL       — logical empiricism + Popper: hypothesis, prediction,
//                       test, replication. Corroboration as failed falsification.
//   M-COMPUTATIONAL   — algorithmic verification: Turing-decidable claims.
//   M-TESTIMONIAL     — social epistemology (Goldman, Fricker): trust in
//                       independent primary sources with lineage.
//   M-ANALOGICAL      — structural mapping (Hofstadter, Gentner): invariant-
//                       preserving analogies with counterexample testing.
//   M-DIALECTICAL     — Habermasian discourse + Popper: survival under
//                       adversarial challenge over time.
//   M-PRAGMATIC       — pragmatism (Peirce, Dewey, James): truth as what
//                       generates reliable practical consequences.
//   M-COHERENTIST     — coherentism (BonJour, Lehrer): truth by fit in a web
//                       of mutually-supporting beliefs.
//   M-PHENOMENOLOGICAL — phenomenology (Husserl, Merleau-Ponty, Varela):
//                       first-person structured description of experience.
//   M-HISTORICAL      — historical method (Ranke, Collingwood): truth by
//                       documented primary-source record with provenance.
//   M-ECOLOGICAL      — situated knowing (Haraway, Longino, TEK): relational
//                       truth explicit about its context.
//   M-PRACTICE        — tacit / consensus-of-practitioners (Polanyi,
//                       Wittgenstein, Kuhn's "normal science"): valid in a
//                       domain if engaged practitioners agree.
const (
	MethodologyFormal         = "M-FORMAL"
	MethodologyEmpirical      = "M-EMPIRICAL"
	MethodologyComputational  = "M-COMPUTATIONAL"
	MethodologyTestimonial    = "M-TESTIMONIAL"
	MethodologyAnalogical     = "M-ANALOGICAL"
	MethodologyDialectical    = "M-DIALECTICAL"
	MethodologyPragmatic      = "M-PRAGMATIC"
	MethodologyCoherentist    = "M-COHERENTIST"
	MethodologyPhenomenologic = "M-PHENOMENOLOGICAL"
	MethodologyHistorical     = "M-HISTORICAL"
	MethodologyEcological     = "M-ECOLOGICAL"
	MethodologyPractice       = "M-PRACTICE"
	MethodologyLegacy         = "M-LEGACY" // transitional; sunsetted
)

const BPS uint64 = 1_000_000

// DefaultMethodologies returns the seven bootstrap methodologies seeded at
// genesis. Each names the rule of compliance, what evidence proves compliance,
// and what would falsify a claim made under it. The chain's "bedrock" under
// the methodology-over-statement model.
//
// Cross-method discounts are stated per pair: when a claim under method A
// cites evidence from method B, the cited contribution is capped at
// cross_method_discount_bps[B] of its effective confidence. Missing entries
// default to full strength (BPS).
func DefaultMethodologies() []*Methodology {
	return []*Methodology{
		{
			Id:          MethodologyFormal,
			Name:        "Formal derivation",
			Description: "Claim derives from stated premises via declared sound inference rules in a named formal system (e.g. ZFC, Peano, Euclidean). Each step must name a rule of that system.",
			ComplianceCriteria: []string{
				"The formal system is named explicitly",
				"Each inference step cites a rule of the stated system",
				"No gap in the derivation chain from premises to conclusion",
			},
			FalsificationPaths: []string{
				"An inference step fails to match any rule of the stated system",
				"A contradiction with a previously verified theorem in the same system",
				"An undeclared axiom is required for the derivation to close",
			},
			CrossMethodDiscountBps: map[string]uint64{
				// Formal citing formal: full. Formal citing empirical is a
				// "formal under empirical assumption" — capped at 90%.
				MethodologyEmpirical:     900_000,
				MethodologyComputational: 1_000_000, // algorithmic verification is strict
				MethodologyTestimonial:   500_000,   // testimony cannot prove a formal claim
				MethodologyAnalogical:    500_000,
			},
			MinQualificationWeight: 50,
			Version:                1,
		},
		{
			Id:          MethodologyEmpirical,
			Name:        "Empirical investigation",
			Description: "Hypothesis → prediction → experiment → independent replication. Claim is accepted when the experimental protocol has been replicated by at least N independent parties.",
			ComplianceCriteria: []string{
				"A hypothesis is stated with testable prediction",
				"Experimental protocol is published",
				"At least 3 independent replications with declared methodology",
				"Control conditions documented",
			},
			FalsificationPaths: []string{
				"The prediction fails under the stated protocol",
				"Replication fails under the stated protocol",
				"A confound is identified that invalidates the controls",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        1_000_000, // math as premise is fine
				MethodologyComputational: 1_000_000,
				MethodologyTestimonial:   700_000,
				MethodologyAnalogical:    400_000,
			},
			MinQualificationWeight: 50,
			Version:                1,
		},
		{
			Id:          MethodologyComputational,
			Name:        "Algorithmic verification",
			Description: "Claim is decidable by a stated deterministic program on stated inputs. The program and inputs are submitted; verification re-executes and checks the output.",
			ComplianceCriteria: []string{
				"Program submitted (with source or deterministic bytecode)",
				"Inputs are fully specified",
				"Output matches the stated claim under re-execution",
			},
			FalsificationPaths: []string{
				"Re-execution produces a different output",
				"The program contains a bug relevant to the claim",
				"The program is non-deterministic on the stated inputs",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:    1_000_000,
				MethodologyEmpirical: 1_000_000,
			},
			MinQualificationWeight: 40,
			Version:                1,
		},
		{
			Id:          MethodologyTestimonial,
			Name:        "Multi-source corroboration",
			Description: "Claim is supported by at least N independent primary sources with documented lineage. Independence means the sources did not derive from each other.",
			ComplianceCriteria: []string{
				"At least 3 primary sources are cited",
				"Each source's lineage is documented (why it is primary)",
				"Independence statement: sources do not share a common origin",
			},
			FalsificationPaths: []string{
				"A primary source repudiates the claim",
				"Two or more sources are shown to share a common origin (not independent)",
				"A source's primary-status is contested and not defended",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        300_000, // testimony cannot ground math
				MethodologyEmpirical:     500_000,
				MethodologyComputational: 300_000,
				MethodologyAnalogical:    700_000,
			},
			MinQualificationWeight: 30,
			Version:                1,
		},
		{
			Id:          MethodologyAnalogical,
			Name:        "Structural analogy",
			Description: "Claim extends a relationship from one domain to another via an explicit structural mapping. The mapping's preserved invariants must be stated; counterexamples considered.",
			ComplianceCriteria: []string{
				"Both domains are stated explicitly",
				"The mapping between them is specified",
				"Preserved invariants are listed",
				"At least one counterexample-consideration is documented",
			},
			FalsificationPaths: []string{
				"A counterexample where the mapping's stated invariant fails",
				"An unacknowledged disanalogy that breaks the inference",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        300_000, // analogy cannot prove a formal claim
				MethodologyEmpirical:     500_000,
				MethodologyComputational: 300_000,
				MethodologyTestimonial:   600_000,
			},
			MinQualificationWeight: 30,
			Version:                1,
		},
		{
			Id:          MethodologyDialectical,
			Name:        "Challenge-survival",
			Description: "Claim earns dialectical robustness by surviving declared challenges from adversarial reviewers. Robustness accrues over time and distinct challenges.",
			ComplianceCriteria: []string{
				"Claim has survived at least N declared challenges (N set per domain)",
				"Each challenge and its rebuttal is logged on-chain",
				"No unrebutted challenge within the current survival window",
			},
			FalsificationPaths: []string{
				"An unrebutted challenge within the survival window",
				"A successful challenge that causes the claim to be marked DISPROVEN",
			},
			CrossMethodDiscountBps: map[string]uint64{
				// Dialectical is a meta-method that confers robustness on top of
				// another method. Cross-citation is discounted because pure
				// challenge-survival doesn't bear on formal or empirical truth
				// independently.
				MethodologyFormal:        500_000,
				MethodologyEmpirical:     700_000,
				MethodologyComputational: 500_000,
			},
			MinQualificationWeight: 40,
			Version:                1,
		},
		// ─── Pragmatism (Peirce, Dewey, James) ──────────────────────────────
		{
			Id:          MethodologyPragmatic,
			Name:        "Pragmatic consequence",
			Description: "Claim is validated by the reliability of its practical consequences across contexts. A true belief is one whose implications hold up in use. Distinct from M-EMPIRICAL: no controlled experiment is required; observable downstream behaviour over time is the test.",
			ComplianceCriteria: []string{
				"The claim names its stated practical consequences",
				"Consequences are observable in the on-chain or off-chain record",
				"A track record of at least N instances where the predicted consequence held",
			},
			FalsificationPaths: []string{
				"A documented instance where the predicted consequence failed",
				"A counter-practice where the claim's use produced an opposite result",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        400_000, // pragmatic claims cannot ground formal proofs
				MethodologyEmpirical:     800_000,
				MethodologyComputational: 500_000,
				MethodologyTestimonial:   900_000,
			},
			MinQualificationWeight: 35,
			Version:                1,
		},
		// ─── Coherentism (BonJour, Lehrer) ──────────────────────────────────
		{
			Id:          MethodologyCoherentist,
			Name:        "Coherentist fit",
			Description: "Claim earns support by fitting into a web of mutually-supporting beliefs, rather than by derivation from foundations. The test is mutual coherence with an existing, independently-verified body of claims.",
			ComplianceCriteria: []string{
				"Claim cites at least N facts that also cite each other (a mutual cluster)",
				"No cited fact contradicts another cited fact",
				"The web's coherence score exceeds the stated threshold",
			},
			FalsificationPaths: []string{
				"A coherence-breaking contradiction within the cited web",
				"Discovery that the web isolates itself from independent verification",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:     500_000, // coherence cannot ground formal proof
				MethodologyEmpirical:  700_000,
				MethodologyHistorical: 800_000,
			},
			MinQualificationWeight: 35,
			Version:                1,
		},
		// ─── Phenomenology (Husserl, Merleau-Ponty, Varela) ─────────────────
		{
			Id:          MethodologyPhenomenologic,
			Name:        "Phenomenological description",
			Description: "First-person structured description of experience following a stated phenomenological method (e.g. epoché + eidetic reduction). Used for claims about the structure of subjective experience that cannot be accessed third-person.",
			ComplianceCriteria: []string{
				"A specific phenomenological method is named (classical, hermeneutic, neurophenomenological, etc.)",
				"The description follows the method's structured protocol",
				"Intersubjective corroboration by at least N independent phenomenologists",
			},
			FalsificationPaths: []string{
				"Systematic failure to replicate the described structure under the stated method",
				"Demonstrated methodological error in the reduction",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        200_000, // phenomenology cannot ground formal proof
				MethodologyEmpirical:     500_000,
				MethodologyComputational: 200_000,
				MethodologyTestimonial:   700_000,
			},
			MinQualificationWeight: 40,
			Version:               1,
			// M-PHENOMENOLOGICAL requires human verifiers at the methodology layer.
			// First-person phenomenal experience cannot be assessed by agents lacking
			// phenomenal consciousness. Governance-amendable but defaults human-only.
			RequiredAccountTypes: []string{"human"},
		},
		// ─── Historical method (Ranke, Collingwood) ─────────────────────────
		{
			Id:          MethodologyHistorical,
			Name:        "Historical method",
			Description: "Truth by primary-source documentation with explicit provenance chain. Distinct from M-TESTIMONIAL: relies on documented records, not live witnesses. Standard tools of historiography (textual criticism, provenance auditing, source triangulation) apply.",
			ComplianceCriteria: []string{
				"At least N primary documentary sources cited with location and access path",
				"Provenance chain documented for each source",
				"Source dating and authenticity addressed",
			},
			FalsificationPaths: []string{
				"A primary source is shown to be forged or misdated",
				"A previously unknown source materially contradicts the claim",
				"The provenance chain contains a gap that cannot be closed",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:      400_000,
				MethodologyEmpirical:   700_000,
				MethodologyTestimonial: 900_000,
			},
			MinQualificationWeight: 40,
			Version:                1,
		},
		// ─── Ecological / situated (Haraway, Longino, TEK) ──────────────────
		{
			Id:          MethodologyEcological,
			Name:        "Situated / relational",
			Description: "Knowledge that is inseparable from its context and the relationships within it. Honours traditional ecological knowledge, indigenous epistemologies, and feminist standpoint theory without conflating or appropriating them. The claim is valid within the stated relational context.",
			ComplianceCriteria: []string{
				"The relational context is stated explicitly (community, practice, place, tradition)",
				"The relationship between the claimant and the context is named",
				"Intersection with other methodologies is documented, not erased",
			},
			FalsificationPaths: []string{
				"The stated context disowns the claim",
				"A context-independent test consistent with the claim yields a contradictory result and the context-dependence does not explain it",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        200_000, // situated knowledge is not a substitute for formal proof
				MethodologyEmpirical:     400_000,
				MethodologyComputational: 200_000,
				MethodologyPhenomenologic: 900_000,
				MethodologyHistorical:    700_000,
			},
			MinQualificationWeight: 30,
			Version:                1,
		},
		// ─── Consensus of practice (Polanyi, Wittgenstein, Kuhn) ────────────
		{
			Id:          MethodologyPractice,
			Name:        "Practitioner consensus",
			Description: "Valid in a domain if practitioners actively engaged in the practice agree that it is. Captures tacit knowledge and the knowledge-in-practice that cannot be fully explicated. Weaker than count-based consensus: requires demonstrated practice, not just opinion.",
			ComplianceCriteria: []string{
				"Practitioners identified with demonstrable engagement in the practice",
				"At least N practitioners agree the claim is valid within the practice",
				"The practice itself is named and bounded",
			},
			FalsificationPaths: []string{
				"The practice boundary shifts such that the claim no longer applies",
				"Engaged practitioners who hold the claim as invalid exceed the agreement threshold",
			},
			CrossMethodDiscountBps: map[string]uint64{
				MethodologyFormal:        300_000,
				MethodologyEmpirical:     500_000,
				MethodologyComputational: 400_000,
				MethodologyTestimonial:   700_000,
			},
			MinQualificationWeight: 30,
			Version:                1,
		},
		{
			Id:          MethodologyLegacy,
			Name:        "Legacy (transitional)",
			Description: "Pre-Phase-1 claims and claims that did not declare a methodology. Adjudicated under permissive rules. Sunsetted — after the sunset window, new claims cannot use this method.",
			ComplianceCriteria: []string{
				"Any evidence considered — transitional permissiveness",
			},
			FalsificationPaths: []string{
				"Standard challenge mechanism applies",
			},
			CrossMethodDiscountBps: map[string]uint64{
				// Legacy claims discount when cited by any method — don't let
				// pre-methodology facts prop up post-methodology claims at full
				// strength.
				MethodologyFormal:        700_000,
				MethodologyEmpirical:     700_000,
				MethodologyComputational: 700_000,
				MethodologyTestimonial:   800_000,
				MethodologyAnalogical:    800_000,
				MethodologyDialectical:   800_000,
			},
			MinQualificationWeight: 20,
			Version:                1,
			IsTransitional:         true,
		},
	}
}

// IsValidMethodologyID reports whether the given id names a bootstrap
// methodology. Governance-added methodologies are validated via the keeper's
// registry check; this function is a fast-path for the defaults.
func IsValidMethodologyID(id string) bool {
	switch id {
	case MethodologyFormal, MethodologyEmpirical, MethodologyComputational,
		MethodologyTestimonial, MethodologyAnalogical, MethodologyDialectical,
		MethodologyPragmatic, MethodologyCoherentist, MethodologyPhenomenologic,
		MethodologyHistorical, MethodologyEcological, MethodologyPractice,
		MethodologyLegacy:
		return true
	}
	return false
}
