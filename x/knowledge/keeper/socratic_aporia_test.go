package keeper_test

// Socratic Dialectic Gap 2 — Aporia (ἀπορία — "without passage").
//
// Plato, Theaetetus 210b: "I don't know what knowledge is" — the moment when
// inquiry has exhausted its current instruments. In Socratic practice, aporia
// is not the endpoint of dialogue but its hinge: what one genuinely does not
// know is the beginning of real understanding.
//
// When a claim has accumulated inconclusive_count >= Params.aporia_threshold,
// the chain marks it in_aporia = true and emits zerone.knowledge.claim_in_aporia.
// This is distinct from INSUFFICIENT (single-round quorum failure): aporia
// represents repeated failure across N rounds — the community of inquiry has
// been genuinely stumped.
//
// A claim in aporia is not condemned. It is named as epistemically interesting
// territory: a site where fresh instruments, better evidence, or a reformed
// question frame is needed. The flag is a signal, not a verdict.
//
// Unit tests (MaybeMarkAporiaForTest):
//   1. Disabled_NoAporia — threshold=0 → no marking regardless of count
//   2. BelowThreshold_NoAporia — count=2, threshold=3 → no marking
//   3. AtThreshold_MarksAporia — count=3, threshold=3 → in_aporia=true, block set
//   4. AboveThreshold_MarksAporia — count=5, threshold=3 → still marks (>=, not ==)
//   5. AlreadyInAporia_Idempotent — already in_aporia=true → block not overwritten
//
// Integration tests (CompleteRound INCONCLUSIVE path):
//   6. CompleteRound_ReachesThreshold — 3 INCONCLUSIVE rounds, threshold=3 → aporia
//   7. CompleteRound_BelowThreshold — 2 INCONCLUSIVE rounds, threshold=3 → no aporia
//   8. AcceptVerdict_NoAporia — ACCEPT verdict → aporia never triggered
//   9. RejectVerdict_NoAporia — REJECT verdict → aporia never triggered
//
// Integration test (phases.go insufficient-reveals path):
//  10. InsufficientReveals_TriggersAporia — expired round (no quorum) also marks aporia

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// setAporiaThreshold writes aporia_threshold into the keeper params.
func setAporiaThreshold(t *testing.T, k keeper.Keeper, ctx sdk.Context, threshold uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.AporiaThreshold = threshold
	require.NoError(t, k.SetParams(ctx, params))
}

// aporiaClaim builds a minimal claim for aporia unit tests.
// inconclusive_count is set directly so we can exercise the threshold logic
// without running multiple full rounds.
func aporiaClaim(id string, inconclusiveCount uint64) *types.Claim {
	return &types.Claim{
		Id:               id,
		FactContent:      "aporia test claim " + id,
		Domain:           "physics",
		Category:         "empirical",
		Submitter:        makeValidBech32Addr("aporia-sub-" + id),
		Status:           types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT,
		InconclusiveCount: inconclusiveCount,
	}
}

// ── Unit tests ────────────────────────────────────────────────────────────────

// Test 1: threshold=0 (disabled) → no aporia even with high inconclusive count.
func TestAporia_Disabled_NoAporia(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	// Default threshold=0 — disabled.

	claim := aporiaClaim("ap-disabled", 10)
	k.MaybeMarkAporiaForTest(ctx, claim)

	require.False(t, claim.InAporia, "threshold=0 must not mark aporia")
	require.Zero(t, claim.AporiaReachedAtBlock)
}

// Test 2: count below threshold → no aporia.
func TestAporia_BelowThreshold_NoAporia(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	setAporiaThreshold(t, k, ctx, 3)

	claim := aporiaClaim("ap-below", 2) // count=2, threshold=3
	k.MaybeMarkAporiaForTest(ctx, claim)

	require.False(t, claim.InAporia, "count < threshold must not mark aporia")
	require.Zero(t, claim.AporiaReachedAtBlock)
}

// Test 3: count exactly at threshold → aporia marked.
func TestAporia_AtThreshold_MarksAporia(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	setAporiaThreshold(t, k, ctx, 3)

	advCtx := ctx.WithBlockHeight(500)
	claim := aporiaClaim("ap-at", 3) // count=3, threshold=3
	k.MaybeMarkAporiaForTest(advCtx, claim)

	require.True(t, claim.InAporia, "count == threshold must mark aporia")
	require.Equal(t, uint64(500), claim.AporiaReachedAtBlock,
		"aporia_reached_at_block must be set to the current block height")
}

// Test 4: count above threshold → still marks (>= check, not ==).
func TestAporia_AboveThreshold_MarksAporia(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	setAporiaThreshold(t, k, ctx, 3)

	advCtx := ctx.WithBlockHeight(600)
	claim := aporiaClaim("ap-above", 5) // count=5, threshold=3
	k.MaybeMarkAporiaForTest(advCtx, claim)

	require.True(t, claim.InAporia, "count > threshold must also mark aporia")
	require.Equal(t, uint64(600), claim.AporiaReachedAtBlock)
}

// Test 5: already in aporia → idempotent; aporia_reached_at_block not overwritten.
func TestAporia_AlreadyInAporia_Idempotent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	setAporiaThreshold(t, k, ctx, 3)

	// Simulate claim already in aporia (set at block 100).
	claim := aporiaClaim("ap-idem", 5)
	claim.InAporia = true
	claim.AporiaReachedAtBlock = 100

	advCtx := ctx.WithBlockHeight(999) // different block
	k.MaybeMarkAporiaForTest(advCtx, claim)

	require.True(t, claim.InAporia)
	require.Equal(t, uint64(100), claim.AporiaReachedAtBlock,
		"aporia_reached_at_block must not be overwritten when already in aporia")
}

// ── Integration tests (CompleteRound) ────────────────────────────────────────

// driveInconclusiveRound runs one INCONCLUSIVE CompleteRound on the claim,
// using a fresh round ID each time (suffix must be unique per call).
func driveInconclusiveRound(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	claimID string,
	roundSuffix string,
	blockHeight int64,
) {
	t.Helper()

	round := incRound("round-ap-"+roundSuffix, claimID)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	r, found := k.GetVerificationRound(ctx, "round-ap-"+roundSuffix)
	require.True(t, found)

	advCtx := ctx.WithBlockHeight(blockHeight)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_INCONCLUSIVE}
	require.NoError(t, k.CompleteRound(advCtx, r, result))
}

// Test 6: three INCONCLUSIVE rounds with threshold=3 → aporia after the third.
func TestAporia_CompleteRound_ReachesThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setAporiaThreshold(t, k, ctx, 3)

	claim := incClaim("ap-cr-reach")
	require.NoError(t, k.SetClaim(ctx, claim))

	driveInconclusiveRound(t, k, ctx, "ap-cr-reach", "r1", 400)
	driveInconclusiveRound(t, k, ctx, "ap-cr-reach", "r2", 401)

	// After two rounds: not in aporia yet.
	updated, found := k.GetClaim(ctx.WithBlockHeight(401), "ap-cr-reach")
	require.True(t, found)
	require.False(t, updated.InAporia, "must not be in aporia before threshold")
	require.Equal(t, uint64(2), updated.InconclusiveCount)

	driveInconclusiveRound(t, k, ctx, "ap-cr-reach", "r3", 402)

	// After three rounds: aporia reached.
	updated, found = k.GetClaim(ctx.WithBlockHeight(402), "ap-cr-reach")
	require.True(t, found)
	require.True(t, updated.InAporia, "must be in aporia after third INCONCLUSIVE round")
	require.Equal(t, uint64(3), updated.InconclusiveCount)
	require.Equal(t, uint64(402), updated.AporiaReachedAtBlock,
		"aporia_reached_at_block must equal the block of the threshold-crossing round")
}

// Test 7: two INCONCLUSIVE rounds with threshold=3 → not yet in aporia.
func TestAporia_CompleteRound_BelowThreshold(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setAporiaThreshold(t, k, ctx, 3)

	claim := incClaim("ap-cr-below")
	require.NoError(t, k.SetClaim(ctx, claim))

	driveInconclusiveRound(t, k, ctx, "ap-cr-below", "b1", 400)
	driveInconclusiveRound(t, k, ctx, "ap-cr-below", "b2", 401)

	updated, found := k.GetClaim(ctx.WithBlockHeight(401), "ap-cr-below")
	require.True(t, found)
	require.False(t, updated.InAporia, "two rounds below threshold must not mark aporia")
	require.Equal(t, uint64(2), updated.InconclusiveCount)
	require.Zero(t, updated.AporiaReachedAtBlock)
}

// Test 8: ACCEPT verdict → aporia never triggered regardless of prior inconclusive count.
func TestAporia_AcceptVerdict_NoAporia(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setAporiaThreshold(t, k, ctx, 1) // threshold=1 so any single inconclusive would fire

	claim := incClaim("ap-accept")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-ap-accept", "ap-accept")))

	r, _ := k.GetVerificationRound(ctx, "round-ap-accept")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "ap-accept")
	require.True(t, found)
	require.False(t, updated.InAporia, "ACCEPT verdict must never trigger aporia")
	require.Zero(t, updated.AporiaReachedAtBlock)
	require.Zero(t, updated.InconclusiveCount)
}

// Test 9: REJECT verdict → aporia never triggered.
func TestAporia_RejectVerdict_NoAporia(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setAporiaThreshold(t, k, ctx, 1)

	claim := incClaim("ap-reject")
	require.NoError(t, k.SetClaim(ctx, claim))
	require.NoError(t, k.SetVerificationRound(ctx, incRound("round-ap-reject", "ap-reject")))

	r, _ := k.GetVerificationRound(ctx, "round-ap-reject")
	advCtx := ctx.WithBlockHeight(400)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: 800_000}
	require.NoError(t, k.CompleteRound(advCtx, r, result))

	updated, found := k.GetClaim(advCtx, "ap-reject")
	require.True(t, found)
	require.False(t, updated.InAporia, "REJECT verdict must never trigger aporia")
	require.Zero(t, updated.AporiaReachedAtBlock)
	require.Zero(t, updated.InconclusiveCount)
}

// Test 10: insufficient-reveals path (phases.go) also triggers aporia.
// Uses AdvanceRoundPhases with an expired round that has fewer reveals than
// MinVerifiers — mirrors the exact path in phases.go.
func TestAporia_InsufficientReveals_TriggersAporia(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	setAporiaThreshold(t, k, ctx, 1) // threshold=1: first insufficient-reveals → aporia

	claim := &types.Claim{
		Id:          "ap-insufficient",
		FactContent: "claim that will expire with too few reveals",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   makeValidBech32Addr("sub-ap-ins"),
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Round in COMMIT phase with 1 reveal.
	// MinVerifiers default=3 — one reveal is insufficient for quorum.
	round := testRound("round-ap-ins", "ap-insufficient", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Reveals = []*types.RevealEntry{
		{
			Verifier:        makeValidBech32Addr("verifier-ap-ins"),
			Vote:            "accept",
			Salt:            []byte("s1"),
			RevealedAtBlock: 155,
		},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance past AggregationDeadline (250) — triggers insufficient-reveals path.
	advCtx := ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(advCtx))

	updated, found := k.GetClaim(advCtx, "ap-insufficient")
	require.True(t, found)
	require.Equal(t, uint64(1), updated.InconclusiveCount,
		"insufficient-reveals path must increment inconclusive_count")
	require.True(t, updated.InAporia,
		"insufficient-reveals path must trigger aporia when threshold reached")
	require.Equal(t, uint64(300), updated.AporiaReachedAtBlock,
		"aporia_reached_at_block must equal the expiry block height")
}
