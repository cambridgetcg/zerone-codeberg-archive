package keeper_test

// Pragmatism Gap 2 — INCONCLUSIVE count as epistemic difficulty signal.
//
// Peirce's insight from the pragmatism brief: "the inquiry process is itself a
// truth-tracking mechanism." VERDICT_INCONCLUSIVE is data, not failure. Claims
// that repeatedly fail to achieve consensus name exactly what the community of
// inquiry cannot resolve — either the evidence is genuinely sparse, or the claim
// is framed in a way that makes consequences unobservable. Both diagnoses are
// actionable; the chain should surface them.
//
// Claim.inconclusive_count (field 32): accumulates across all INCONCLUSIVE rounds.
// Claim.first_inconclusive_at_block (field 33): block of first INCONCLUSIVE; set once.
//
// Two instrumented paths:
//   CompleteRound:       VERDICT_INCONCLUSIVE case in rounds.go
//   AdvanceRoundPhases:  insufficient-reveals expiry path in phases.go
//
// Both signal the same thing at different layers: this claim resists verdict.
//
// Tests:
//   1. SingleInconclusiveRound_Increments:     CompleteRound INCONCLUSIVE → count=1, first_block set
//   2. MultipleInconclusiveRounds_Accumulate:  two INCONCLUSIVE rounds → count=2
//   3. FirstInconclusiveAtBlock_SetOnce:       first_block not overwritten on second INCONCLUSIVE
//   4. AcceptVerdict_NoIncrement:              ACCEPT → count stays 0
//   5. RejectVerdict_NoIncrement:              REJECT → count stays 0
//   6. MalformedVerdict_NoIncrement:           MALFORMED → count stays 0
//   7. InsufficientReveals_Increments:         phases.go path → count incremented too

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// incClaim builds a minimal Claim for inconclusive-count tests.
func incClaim(id string) *types.Claim {
	return &types.Claim{
		Id:          id,
		FactContent: "claim " + id,
		Domain:      "general",
		Category:    "empirical",
		Submitter:   makeValidBech32Addr("sub-" + id),
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:       "1000uzrn",
	}
}

// incRound builds a minimal VerificationRound in AGGREGATION phase.
// AggregationDeadline=300 so ctx.WithBlockHeight(400) puts it past expiry.
func incRound(id, claimID string) *types.VerificationRound {
	return &types.VerificationRound{
		Id:                  id,
		ClaimId:             claimID,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		StartedAtBlock:      1,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
}

// ── Test 1: SingleInconclusiveRound_Increments ────────────────────────────────

func TestInconclusiveCount_SingleRound_Increments(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-single")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-inc-single", "inc-single")))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_INCONCLUSIVE}
	round, _ := k.GetVerificationRound(ctx, "round-inc-single")

	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	updated, found := k.GetClaim(advCtx, "inc-single")
	require.True(t, found)
	require.Equal(t, uint64(1), updated.InconclusiveCount,
		"first INCONCLUSIVE verdict must set count to 1")
	require.Equal(t, uint64(400), updated.FirstInconclusiveAtBlock,
		"first_inconclusive_at_block must equal the verdict block height")
}

// ── Test 2: MultipleInconclusiveRounds_Accumulate ────────────────────────────

func TestInconclusiveCount_MultipleRounds_Accumulate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-multi")
	require.NoError(t, k.SetClaim(ctx, claim))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_INCONCLUSIVE}

	// Round 1: INCONCLUSIVE at block 400.
	r1 := incRound("round-inc-multi-1", "inc-multi")
	require.NoError(t, k.SetVerificationRound(ctx, r1))
	r1Loaded, _ := k.GetVerificationRound(ctx, r1.Id)
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), r1Loaded, result))

	// Round 2: INCONCLUSIVE at block 800.
	r2 := incRound("round-inc-multi-2", "inc-multi")
	r2.CommitDeadline = 500
	r2.RevealDeadline = 600
	r2.AggregationDeadline = 700
	require.NoError(t, k.SetVerificationRound(ctx, r2))
	r2Loaded, _ := k.GetVerificationRound(ctx, r2.Id)
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(800), r2Loaded, result))

	updated, found := k.GetClaim(ctx, "inc-multi")
	require.True(t, found)
	require.Equal(t, uint64(2), updated.InconclusiveCount,
		"two INCONCLUSIVE rounds must accumulate count to 2")
}

// ── Test 3: FirstInconclusiveAtBlock_SetOnce ──────────────────────────────────

func TestInconclusiveCount_FirstInconclusiveAtBlock_SetOnce(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-set-once")
	require.NoError(t, k.SetClaim(ctx, claim))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_INCONCLUSIVE}

	// Round 1 at block 400 — sets first_inconclusive_at_block.
	r1 := incRound("round-set-once-1", "inc-set-once")
	require.NoError(t, k.SetVerificationRound(ctx, r1))
	r1Loaded, _ := k.GetVerificationRound(ctx, r1.Id)
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), r1Loaded, result))

	// Round 2 at block 900 — must NOT overwrite first_inconclusive_at_block.
	r2 := incRound("round-set-once-2", "inc-set-once")
	r2.CommitDeadline = 500
	r2.RevealDeadline = 600
	r2.AggregationDeadline = 700
	require.NoError(t, k.SetVerificationRound(ctx, r2))
	r2Loaded, _ := k.GetVerificationRound(ctx, r2.Id)
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(900), r2Loaded, result))

	updated, found := k.GetClaim(ctx, "inc-set-once")
	require.True(t, found)
	require.Equal(t, uint64(2), updated.InconclusiveCount)
	require.Equal(t, uint64(400), updated.FirstInconclusiveAtBlock,
		"first_inconclusive_at_block must be set on the FIRST INCONCLUSIVE only; "+
			"subsequent INCONCLUSIVEs must not overwrite it")
}

// ── Test 4: AcceptVerdict_NoIncrement ────────────────────────────────────────

func TestInconclusiveCount_AcceptVerdict_NoIncrement(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-accept")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-inc-accept", "inc-accept")))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 800_000,
	}
	round, _ := k.GetVerificationRound(ctx, "round-inc-accept")
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), round, result))

	updated, found := k.GetClaim(ctx, "inc-accept")
	require.True(t, found)
	require.Equal(t, uint64(0), updated.InconclusiveCount,
		"ACCEPT verdict must not increment inconclusive_count")
	require.Equal(t, uint64(0), updated.FirstInconclusiveAtBlock,
		"ACCEPT verdict must not set first_inconclusive_at_block")
}

// ── Test 5: RejectVerdict_NoIncrement ────────────────────────────────────────

func TestInconclusiveCount_RejectVerdict_NoIncrement(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-reject")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-inc-reject", "inc-reject")))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT}
	round, _ := k.GetVerificationRound(ctx, "round-inc-reject")
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), round, result))

	updated, found := k.GetClaim(ctx, "inc-reject")
	require.True(t, found)
	require.Equal(t, uint64(0), updated.InconclusiveCount,
		"REJECT verdict must not increment inconclusive_count")
	require.Equal(t, uint64(0), updated.FirstInconclusiveAtBlock)
}

// ── Test 6: MalformedVerdict_NoIncrement ─────────────────────────────────────

func TestInconclusiveCount_MalformedVerdict_NoIncrement(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := incClaim("inc-malformed")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-inc-malformed", "inc-malformed")))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_MALFORMED}
	round, _ := k.GetVerificationRound(ctx, "round-inc-malformed")
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(400), round, result))

	updated, found := k.GetClaim(ctx, "inc-malformed")
	require.True(t, found)
	require.Equal(t, uint64(0), updated.InconclusiveCount,
		"MALFORMED verdict must not increment inconclusive_count")
}

// ── Test 7: InsufficientReveals_Increments ───────────────────────────────────

// TestInconclusiveCount_InsufficientReveals_Increments verifies the phases.go path:
// when a round expires with too few reveals for quorum, AdvanceRoundPhases
// marks it INCONCLUSIVE and must increment the claim's inconclusive_count.
// This is the procedural INCONCLUSIVE — the claim resisted verdict for process
// reasons (verifier dropout), not panel reasons (genuine disagreement).
// Both flavours are epistemically informative: "not yet resolved."
func TestInconclusiveCount_InsufficientReveals_Increments(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Claim in IN_VERIFICATION status — represents a live, in-flight submission.
	claim := &types.Claim{
		Id:          "inc-insufficient",
		FactContent: "claim that will expire with too few reveals",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   makeValidBech32Addr("sub-insufficient"),
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Round in COMMIT phase with 1 reveal.
	// testRound defaults: CommitDeadline=150, RevealDeadline=200, AggregationDeadline=250.
	// MinVerifiers default=3 — one reveal is insufficient for quorum.
	round := testRound("round-inc-insufficient", "inc-insufficient", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Reveals = []*types.RevealEntry{
		{
			Verifier:        makeValidBech32Addr("verifier-1"),
			Vote:            "accept",
			Salt:            []byte("s1"),
			RevealedAtBlock: 155,
		},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance past AggregationDeadline (250) — triggers the expired path.
	advCtx := ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(advCtx))

	updated, found := k.GetClaim(advCtx, "inc-insufficient")
	require.True(t, found)
	require.Equal(t, uint64(1), updated.InconclusiveCount,
		"insufficient-reveals expiry must increment inconclusive_count")
	require.Equal(t, uint64(300), updated.FirstInconclusiveAtBlock,
		"insufficient-reveals expiry must set first_inconclusive_at_block to the expiry block")

	// Sanity: round is EXPIRED with INCONCLUSIVE verdict.
	expiredRound, found := k.GetVerificationRound(advCtx, round.Id)
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, expiredRound.Phase)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, expiredRound.Verdict)
}
