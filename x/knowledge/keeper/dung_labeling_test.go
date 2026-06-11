package keeper_test

// Structured Deliberation Gap 3 — Dung Labelling.
//
// Phan Minh Dung (1995): "On the acceptability of arguments and its fundamental
// role in nonmonotonic reasoning, logic programming and n-person games."
// Artificial Intelligence 77, 321–357.
//
// An argumentation framework AF = ⟨A, R⟩ labels each argument as:
//   in  — every attacker is OUT (argument is acceptable)
//   out — some attacker is IN (argument is defeated)
//   undecided — neither (unresolved)
//
// Zerone's on-chain approximation seeds the grounded semantics with the PoT
// verification verdict:
//   ACCEPT    → DUNG_LABEL_IN       (survived adversarial scrutiny)
//   REJECT    → DUNG_LABEL_OUT      (defeated)
//   MALFORMED → DUNG_LABEL_OUT      (not truth-apt — ruled out)
//   INCONCLUSIVE → DUNG_LABEL_UNDECIDED (inquiry unresolved)
//
// The label is immutable after assignment; full extension semantics can be
// computed offline by traversing the claim/fact attack graph.
//
// Tests:
//  1. AcceptVerdict_AssignsIN        — ACCEPT → DUNG_LABEL_IN
//  2. RejectVerdict_AssignsOUT       — REJECT → DUNG_LABEL_OUT
//  3. MalformedVerdict_AssignsOUT    — MALFORMED → DUNG_LABEL_OUT
//  4. InconclusiveVerdict_AssignsUNDECIDED — INCONCLUSIVE → DUNG_LABEL_UNDECIDED
//  5. PreVerdict_LabelUnspecified    — fresh claim has UNSPECIFIED label (zero value)
//  6. LabelPersistedInState          — label persisted by SetClaim / GetClaim round-trip
//  7. MultipleRoundsOneLabel         — second INCONCLUSIVE round on same claim: label stays UNDECIDED

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// dungClaim builds a minimal claim for Dung label tests.
func dungClaim(id string) *types.Claim {
	return &types.Claim{
		Id:          id,
		FactContent: "dung label test claim " + id,
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   makeValidBech32Addr("dung-sub-" + id),
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:       "1000uzrn",
	}
}

// completeDungRound runs one CompleteRound on the claim and returns the stored claim.
func completeDungRound(
	t *testing.T,
	k keeper.Keeper,
	ctx interface{ WithBlockHeight(int64) interface{ BlockHeight() int64 } },
	claimID, roundSuffix string,
	verdict types.Verdict,
	confidence uint64,
) *types.Claim {
	t.Helper()
	return nil // replaced by the actual calls below
}

// ── Tests ────────────────────────────────────────────────────────────────────

// Test 1: ACCEPT verdict → DUNG_LABEL_IN.
func TestDungLabel_AcceptVerdict_AssignsIN(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-accept")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-accept", "dung-accept")))

	r, _ := k.GetVerificationRound(ctx, "round-dung-accept")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "dung-accept")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_IN, updated.DungLabel,
		"ACCEPT verdict must assign DUNG_LABEL_IN")
}

// Test 2: REJECT verdict → DUNG_LABEL_OUT.
func TestDungLabel_RejectVerdict_AssignsOUT(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-reject")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-reject", "dung-reject")))

	r, _ := k.GetVerificationRound(ctx, "round-dung-reject")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "dung-reject")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_OUT, updated.DungLabel,
		"REJECT verdict must assign DUNG_LABEL_OUT")
}

// Test 3: MALFORMED verdict → DUNG_LABEL_OUT (not truth-apt = ruled out).
func TestDungLabel_MalformedVerdict_AssignsOUT(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-malformed")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-malformed", "dung-malformed")))

	r, _ := k.GetVerificationRound(ctx, "round-dung-malformed")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_MALFORMED, Confidence: 600_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "dung-malformed")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_OUT, updated.DungLabel,
		"MALFORMED verdict must assign DUNG_LABEL_OUT — not truth-apt arguments are ruled out")
}

// Test 4: INCONCLUSIVE verdict → DUNG_LABEL_UNDECIDED.
func TestDungLabel_InconclusiveVerdict_AssignsUNDECIDED(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-inconclusive")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-inc", "dung-inconclusive")))

	r, _ := k.GetVerificationRound(ctx, "round-dung-inc")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_INCONCLUSIVE}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "dung-inconclusive")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_UNDECIDED, updated.DungLabel,
		"INCONCLUSIVE verdict must assign DUNG_LABEL_UNDECIDED")
}

// Test 5: Fresh claim has UNSPECIFIED label (zero value — no verdict yet).
func TestDungLabel_PreVerdict_LabelUnspecified(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := dungClaim("dung-fresh")
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "dung-fresh")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_UNSPECIFIED, stored.DungLabel,
		"claim before any verdict must have DUNG_LABEL_UNSPECIFIED (zero value)")
}

// Test 6: Label is persisted via SetClaim / GetClaim round-trip.
func TestDungLabel_LabelPersistedInState(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-persist")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-persist", "dung-persist")))

	r, _ := k.GetVerificationRound(ctx, "round-dung-persist")
	advCtx := ctx.WithBlockHeight(500)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 900_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	// Read back from a fresh context (same block) to confirm persistence.
	persisted, found := k.GetClaim(advCtx, "dung-persist")
	require.True(t, found)
	require.Equal(t, types.DungLabel_DUNG_LABEL_IN, persisted.DungLabel,
		"Dung label must survive a SetClaim/GetClaim round-trip")
}

// Test 7: Second INCONCLUSIVE round on same claim — label stays UNDECIDED.
// The label is based on the most recent verdict; repeated inconclusiveness
// does not degrade the label further.
func TestDungLabel_MultipleRounds_LabelTracksLastVerdict(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := dungClaim("dung-multi")
	require.NoError(t, k.SetClaim(ctx, claim))

	// Round 1: INCONCLUSIVE → UNDECIDED.
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-multi-1", "dung-multi")))
	r1, _ := k.GetVerificationRound(ctx, "round-dung-multi-1")
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), r1, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_INCONCLUSIVE,
	}))

	after1, _ := k.GetClaim(ctx.WithBlockHeight(400), "dung-multi")
	require.Equal(t, types.DungLabel_DUNG_LABEL_UNDECIDED, after1.DungLabel,
		"after first INCONCLUSIVE: UNDECIDED")

	// Round 2: INCONCLUSIVE again → label remains UNDECIDED.
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-dung-multi-2", "dung-multi")))
	r2, _ := k.GetVerificationRound(ctx, "round-dung-multi-2")
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(401), r2, &keeper.VerificationResult{
		Verdict: types.Verdict_VERDICT_INCONCLUSIVE,
	}))

	after2, _ := k.GetClaim(ctx.WithBlockHeight(401), "dung-multi")
	require.Equal(t, types.DungLabel_DUNG_LABEL_UNDECIDED, after2.DungLabel,
		"after second INCONCLUSIVE: still UNDECIDED")
}
