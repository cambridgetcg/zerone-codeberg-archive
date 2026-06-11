package keeper

import (
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Structured Deliberation Gap 3: Dung Labelling ───────────────────────────
//
// Phan Minh Dung (1995): "On the acceptability of arguments and its fundamental
// role in nonmonotonic reasoning, logic programming and n-person games."
// Artificial Intelligence 77, 321–357.
//
// An argumentation framework AF = ⟨A, R⟩ assigns labels from {in, out, undecided}
// to each argument A under a semantics:
//   - A is IN  iff every argument B such that (B,A) ∈ R is OUT
//   - A is OUT iff there exists argument B such that (B,A) ∈ R and B is IN
//   - A is UNDECIDED otherwise
//
// Zerone's on-chain approximation — one-step grounded semantics seeded by the
// PoT verification verdict:
//
//   ACCEPT    → DUNG_LABEL_IN        (argument survived adversarial scrutiny)
//   REJECT    → DUNG_LABEL_OUT       (argument was defeated)
//   MALFORMED → DUNG_LABEL_OUT       (argument is not truth-apt — ruled out)
//   INCONCLUSIVE/INSUFFICIENT → DUNG_LABEL_UNDECIDED
//
// The label is assigned at CompleteRound and is immutable (re-submissions create
// new claim IDs; the original verdict stands). Full extension semantics (preferred,
// stable, complete) can be computed offline by traversing the attack graph:
// ClaimRelation.relation == ATTACKS + FactRelation.attack_target.

// dungLabelForVerdict maps a PoT verdict to the corresponding Dung label.
func dungLabelForVerdict(verdict types.Verdict) types.DungLabel {
	switch verdict {
	case types.Verdict_VERDICT_ACCEPT:
		return types.DungLabel_DUNG_LABEL_IN
	case types.Verdict_VERDICT_REJECT, types.Verdict_VERDICT_MALFORMED:
		return types.DungLabel_DUNG_LABEL_OUT
	case types.Verdict_VERDICT_INCONCLUSIVE:
		return types.DungLabel_DUNG_LABEL_UNDECIDED
	default:
		return types.DungLabel_DUNG_LABEL_UNSPECIFIED
	}
}
