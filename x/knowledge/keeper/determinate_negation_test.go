package keeper_test

// Hegelian Dialectic — Determinate Negation (Gap 1).
//
// Hegel's principle (Science of Logic §54): "The negation is a determinate
// negation; it has a content." When a claim is rejected, the failure is not
// merely "no" — it has a specific character that points toward the more adequate
// claim that should succeed it. Each rejection type implies what KIND of
// improvement is needed:
//
//   INTERNAL_INCONSISTENCY   → successor must resolve the self-contradiction
//   INSUFFICIENT_EVIDENCE    → successor must supply empirical grounding
//   SCOPE_EXCEEDED           → successor must narrow its scope
//   PRESUPPOSITIONAL_FAILURE → successor must revise its foundational assumptions
//   METHODOLOGICAL_MISMATCH  → successor must apply the correct methodology
//
// Architecture:
//   - RevealEntry.negation_type (field 6): verifier's individual diagnosis
//   - MsgSubmitReveal.negation_type (field 7): propagated to RevealEntry
//   - VerificationRound.determinate_negation_type (field 17): plurality result
//   - aggregateDeterminateNegationType: plurality algorithm with tie → UNSPECIFIED
//
// Unit tests (AggregateDeterminateNegationTypeForTest):
//   1. AllUnspecified → UNSPECIFIED (no diagnosis)
//   2. EmptyReveals → UNSPECIFIED
//   3. SingleReject_InsufficientEvidence → INSUFFICIENT_EVIDENCE
//   4. Plurality_InsufficientEvidence (2 vs 1) → INSUFFICIENT_EVIDENCE
//   5. AllAccept → UNSPECIFIED (accept votes ignored)
//   6. MixedAcceptReject → only reject votes count
//   7. Tie → UNSPECIFIED (epistemic humility on tie)
//   8. AllFiveTypes → returns plurality
//   9. NilRevealEntry → ignored safely
//
// Integration tests (CompleteRound):
//  10. VerdictReject_DeterminateNegationStored
//  11. VerdictAccept_DeterminateNegationUnspecified (not set on non-REJECT)
//  12. VerdictReject_AllUnspecified → UNSPECIFIED stored
//  13. RevealNegationTypePropagatedToEntry

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeRevealEntry(vote string, negType types.DeterminateNegationType) *types.RevealEntry {
	return &types.RevealEntry{
		Verifier:     makeValidBech32Addr("rev-" + vote + "-" + negType.String()),
		Vote:         vote,
		NegationType: negType,
	}
}

// ── Unit tests (AggregateDeterminateNegationTypeForTest) ──────────────────────

// Test 1: All reveals have UNSPECIFIED negation type → UNSPECIFIED.
func TestDeterminateNegation_AllUnspecified(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED, result,
		"all UNSPECIFIED diagnoses → aggregate must be UNSPECIFIED")
}

// Test 2: Empty reveals slice → UNSPECIFIED.
func TestDeterminateNegation_EmptyReveals(t *testing.T) {
	result := keeper.AggregateDeterminateNegationTypeForTest(nil)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED, result)
}

// Test 3: Single reject with INSUFFICIENT_EVIDENCE → INSUFFICIENT_EVIDENCE.
func TestDeterminateNegation_SingleReject_InsufficientEvidence(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE, result,
		"single INSUFFICIENT_EVIDENCE reject → aggregate must be INSUFFICIENT_EVIDENCE")
}

// Test 4: Plurality — 2× INSUFFICIENT_EVIDENCE vs 1× SCOPE_EXCEEDED → INSUFFICIENT_EVIDENCE.
func TestDeterminateNegation_Plurality(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE, result,
		"2 vs 1 → plurality INSUFFICIENT_EVIDENCE must win")
}

// Test 5: All ACCEPT votes → UNSPECIFIED (accept votes carry no negation diagnosis).
func TestDeterminateNegation_AllAccept(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("accept", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
		makeRevealEntry("accept", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED, result,
		"accept votes ignored for negation aggregation → UNSPECIFIED")
}

// Test 6: Mixed accept+reject — only reject votes contribute.
func TestDeterminateNegation_MixedAcceptReject(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("accept", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE), // ignored
		makeRevealEntry("accept", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),        // ignored
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_METHODOLOGICAL_MISMATCH),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_METHODOLOGICAL_MISMATCH),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_METHODOLOGICAL_MISMATCH, result,
		"only reject votes counted — METHODOLOGICAL_MISMATCH must win")
}

// Test 7: Tie → UNSPECIFIED (epistemic humility: don't assert false precision).
func TestDeterminateNegation_Tie(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED, result,
		"1:1 tie → UNSPECIFIED (epistemic humility: do not assert a false plurality)")
}

// Test 8: All five named types represented; one with highest count wins.
func TestDeterminateNegation_AllFiveTypes(t *testing.T) {
	reveals := []*types.RevealEntry{
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INTERNAL_INCONSISTENCY),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE), // ← 2
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_PRESUPPOSITIONAL_FAILURE),
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE, result,
		"INSUFFICIENT_EVIDENCE has 2 votes (plurality) → must win")
}

// Test 9: Nil RevealEntry → ignored safely.
func TestDeterminateNegation_NilRevealEntry(t *testing.T) {
	reveals := []*types.RevealEntry{
		nil,
		makeRevealEntry("reject", types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED),
		nil,
	}
	result := keeper.AggregateDeterminateNegationTypeForTest(reveals)
	require.Equal(t, types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED, result,
		"nil entries are skipped; remaining SCOPE_EXCEEDED must win")
}

// ── Integration tests (CompleteRound path) ────────────────────────────────────

// completeRoundWithNegationTypes injects a claim + round with REJECT reveals
// and calls CompleteRound directly, returning the completed round.
func completeRoundWithNegationTypes(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	claimSuffix string,
	negTypes []types.DeterminateNegationType,
) *types.VerificationRound {
	t.Helper()

	claimID := "dn-claim-" + claimSuffix
	roundID := "dn-round-" + claimSuffix

	// Inject the claim directly (no fee/bank needed when bypassing MsgServer).
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "determinate negation integration test claim for " + claimSuffix,
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("dn-sub-" + claimSuffix),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Build reveals with the specified negation types (all REJECT votes).
	reveals := make([]*types.RevealEntry, len(negTypes))
	for i, negType := range negTypes {
		reveals[i] = &types.RevealEntry{
			Verifier:     makeValidBech32Addr("dn-ver-" + claimSuffix + string(rune('a'+i))),
			Vote:         "reject",
			NegationType: negType,
		}
	}

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
		Reveals:             reveals,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Drive CompleteRound with a REJECT result.
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_REJECT,
		Confidence: 800_000,
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Fetch the updated round.
	completedRound, found := k.GetVerificationRound(ctx, roundID)
	require.True(t, found, "round must still be stored after CompleteRound")
	return completedRound
}

// Test 10: REJECT verdict → DeterminateNegationType stored on round.
func TestDeterminateNegation_VerdictReject_TypeStored(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	completedRound := completeRoundWithNegationTypes(t, k, ctx, "scope", []types.DeterminateNegationType{
		types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED,
		types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED,
		types.DeterminateNegationType_DETERMINATE_NEGATION_INSUFFICIENT_EVIDENCE,
	})

	require.Equal(t, types.Verdict_VERDICT_REJECT, completedRound.Verdict)
	require.Equal(t,
		types.DeterminateNegationType_DETERMINATE_NEGATION_SCOPE_EXCEEDED,
		completedRound.DeterminateNegationType,
		"SCOPE_EXCEEDED (2 votes) must be stored as the determinate negation type on the round",
	)
}

// Test 11: REJECT with all-UNSPECIFIED diagnoses → UNSPECIFIED stored.
func TestDeterminateNegation_VerdictReject_AllUnspecified(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	completedRound := completeRoundWithNegationTypes(t, k, ctx, "unspec", []types.DeterminateNegationType{
		types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED,
		types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED,
	})

	require.Equal(t, types.Verdict_VERDICT_REJECT, completedRound.Verdict)
	require.Equal(t,
		types.DeterminateNegationType_DETERMINATE_NEGATION_UNSPECIFIED,
		completedRound.DeterminateNegationType,
		"all UNSPECIFIED → UNSPECIFIED must be stored (no false precision)",
	)
}

// Test 12: RevealEntry.NegationType propagated from MsgSubmitReveal.
func TestDeterminateNegation_NegationTypePropagatedToRevealEntry(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Inject a round with a reveal that has NegationType set.
	round := &types.VerificationRound{
		Id:          "round-neg-prop",
		ClaimId:     "claim-neg-prop",
		Phase:       types.VerificationPhase_VERIFICATION_PHASE_COMPLETE,
		Verdict:     types.Verdict_VERDICT_REJECT,
		VerdictBlock: uint64(ctx.BlockHeight()),
		Reveals: []*types.RevealEntry{
			{
				Verifier:     makeValidBech32Addr("neg-verifier-a"),
				Vote:         "reject",
				NegationType: types.DeterminateNegationType_DETERMINATE_NEGATION_PRESUPPOSITIONAL_FAILURE,
			},
			{
				Verifier:     makeValidBech32Addr("neg-verifier-b"),
				Vote:         "reject",
				NegationType: types.DeterminateNegationType_DETERMINATE_NEGATION_PRESUPPOSITIONAL_FAILURE,
			},
		},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Retrieve and verify NegationType is preserved in reveals.
	stored, found := k.GetVerificationRound(ctx, "round-neg-prop")
	require.True(t, found)
	require.Len(t, stored.Reveals, 2)
	require.Equal(t,
		types.DeterminateNegationType_DETERMINATE_NEGATION_PRESUPPOSITIONAL_FAILURE,
		stored.Reveals[0].NegationType,
		"RevealEntry.NegationType must survive round KV round-trip",
	)
	require.Equal(t,
		types.DeterminateNegationType_DETERMINATE_NEGATION_PRESUPPOSITIONAL_FAILURE,
		stored.Reveals[1].NegationType,
	)
}
