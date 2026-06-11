package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// claimTypeFuncName maps ClaimType enum values to canonical form function names.
var claimTypeFuncName = map[ClaimType]string{
	ClaimType_CLAIM_TYPE_UNSPECIFIED: "assert",
	ClaimType_CLAIM_TYPE_ASSERTION:  "assert",
	ClaimType_CLAIM_TYPE_RELATION:   "relate",
	ClaimType_CLAIM_TYPE_DEFINITION: "define",
	ClaimType_CLAIM_TYPE_CONSTRAINT: "constrain",
	ClaimType_CLAIM_TYPE_NEGATION:   "negate",
	ClaimType_CLAIM_TYPE_OBSERVATION: "observe",
}

// BuildCanonicalForm constructs a canonical form from a claim's type and structure.
// Returns empty string if structure is insufficient (nil or missing subject).
//
// Grammar:
//
//	ASSERTION:   assert(domain, subject, predicate[, scope])
//	RELATION:    relate(domain, subject, relation_verb, object[, scope])
//	DEFINITION:  define(domain, term, meaning)
//	CONSTRAINT:  constrain(domain, subject, constraint[, scope])
//	NEGATION:    negate(domain, subject, predicate[, scope])
//	OBSERVATION: observe(domain, subject, predicate, temporal_scope)
func BuildCanonicalForm(claimType ClaimType, structure *ClaimStructure, domain string) string {
	if structure == nil || structure.Subject == "" {
		return ""
	}

	funcName, ok := claimTypeFuncName[claimType]
	if !ok {
		funcName = "assert"
	}

	var args []string
	args = append(args, quoteArg(domain))

	switch claimType {
	case ClaimType_CLAIM_TYPE_DEFINITION:
		// define(domain, term, meaning)
		args = append(args, quoteArg(structure.Subject))
		if structure.Predicate != "" {
			args = append(args, quoteArg(structure.Predicate))
		}

	case ClaimType_CLAIM_TYPE_RELATION:
		// relate(domain, subject, relation_verb, object[, scope])
		args = append(args, quoteArg(structure.Subject))
		if structure.Predicate != "" {
			args = append(args, quoteArg(structure.Predicate))
		}
		if structure.Object != "" {
			args = append(args, quoteArg(structure.Object))
		}
		if structure.Scope != "" {
			args = append(args, quoteArg(structure.Scope))
		}

	case ClaimType_CLAIM_TYPE_OBSERVATION:
		// observe(domain, subject, predicate, temporal_scope)
		args = append(args, quoteArg(structure.Subject))
		if structure.Predicate != "" {
			args = append(args, quoteArg(structure.Predicate))
		}
		if structure.TemporalScope != "" {
			args = append(args, quoteArg(structure.TemporalScope))
		}

	default:
		// assert, constrain, negate: (domain, subject, predicate[, scope])
		args = append(args, quoteArg(structure.Subject))
		if structure.Predicate != "" {
			args = append(args, quoteArg(structure.Predicate))
		}
		if structure.Scope != "" {
			args = append(args, quoteArg(structure.Scope))
		}
	}

	form := fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))
	return NormalizeCanonicalForm(form)
}

// NormalizeCanonicalForm applies normalization rules to a canonical form string:
// 1. All strings lowercased
// 2. Leading/trailing whitespace trimmed
// 3. Internal whitespace collapsed to single space
// 4. UTF-8 normalized to NFC
func NormalizeCanonicalForm(form string) string {
	// NFC normalization
	form = norm.NFC.String(form)
	// Lowercase
	form = strings.ToLower(form)
	// Trim
	form = strings.TrimSpace(form)
	// Collapse internal whitespace
	form = collapseWhitespace(form)
	return form
}

// HashCanonicalForm returns the SHA-256 hex digest of a normalized canonical form.
func HashCanonicalForm(form string) string {
	h := sha256.Sum256([]byte(form))
	return hex.EncodeToString(h[:])
}

// ComputeOpposingCanonicalForm returns the logically opposing canonical form.
//
// The only direct logical contradiction in the canonical form language is:
//
//	assert(D, S, P[, scope]) ↔ negate(D, S, P[, scope])
//
// Both carry the same domain, subject, predicate, and optional scope — one
// asserts the predicate holds, the other asserts it does not. If a verified
// fact exists with the opposing form, the new claim contradicts established
// chain knowledge.
//
// For all other canonical forms (relate, define, constrain, observe) the
// function returns "" — no direct mechanical inverse exists in this language.
// Constraint violations against constrain() facts require semantic reasoning
// beyond what string-level form inversion can provide (off-chain oracle
// territory, not on-chain detection).
//
// The returned opposing form is already normalized — suitable for direct
// comparison or hashing via HashCanonicalForm.
func ComputeOpposingCanonicalForm(canonicalForm string) string {
	normalized := NormalizeCanonicalForm(canonicalForm)
	if strings.HasPrefix(normalized, "assert(") {
		return "negate(" + normalized[len("assert("):]
	}
	if strings.HasPrefix(normalized, "negate(") {
		return "assert(" + normalized[len("negate("):]
	}
	return ""
}

// ComputeOpposingCanonicalHash returns the SHA-256 hex digest of the opposing
// canonical form, or "" if no opposing form exists.
// Convenience wrapper: ComputeOpposingCanonicalForm + HashCanonicalForm.
func ComputeOpposingCanonicalHash(canonicalForm string) string {
	opposing := ComputeOpposingCanonicalForm(canonicalForm)
	if opposing == "" {
		return ""
	}
	return HashCanonicalForm(opposing)
}

// quoteArg wraps a canonical form argument in double quotes.
func quoteArg(s string) string {
	s = strings.TrimSpace(s)
	return fmt.Sprintf("%q", s)
}

// collapseWhitespace replaces runs of whitespace with a single space,
// except inside quoted strings.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inQuote := false
	prevSpace := false
	for _, r := range s {
		if r == '"' {
			inQuote = !inQuote
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !inQuote && unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}
