package keeper_test

// Falsificationism Gap 1 — Corroboration Severity Score.
//
// Popper's key insight: scientific robustness comes from surviving *severe* tests,
// not from accumulating confirmations. A claim that survived one decisive 95%-
// confident challenge is epistemically stronger than one that survived ten
// sycophantic 55%-confident challenges. Quantity and severity are different.
//
// This gap adds Fact.corroboration_severity_score_bps (field 80):
//   - Set to acceptance_confidence_bps at ACCEPT verdict (severity of the
//     original PoT panel: a 95%-confident ACCEPT is a more severe test than
//     a 55%-confident one).
//   - Incremented by rejection_confidence_bps each time a CONTRADICTS
//     challenge against this fact is REJECTED (how convincingly the panel
//     found the challenger wrong).
//   - No cooldown — unlike CorroborationCount, each challenge carries an
//     independent severity signal regardless of timing.
//
// Distinct from corroboration_count (raw count). This field encodes HOW
// CONVINCINGLY tests were survived, not merely how many.
//
// Tests:
//   1. InitialSeverityOnAccept: severity set to acceptance confidence at ACCEPT
//   2. ZeroConfidenceGivesZeroSeverity: 0-confidence ACCEPT → severity = 0
//   3. SeverityAccumulatesOnChallengeRejected: REJECT adds rejection panel confidence
//   4. SeverityAccumulatesUncooled: multiple fast challenges all add severity (no cooldown)
//   5. CountAndSeverityAreDistinct: count is cooldown-limited; severity is not
//   6. HighConfidenceAddsMoreThanLow: 95% rejection adds more than 55%
//   7. RejectNonChallengeNoop: REJECT on non-challenge claim leaves fact unchanged
//   8. StoredAfterSurvival: severity persists in KV after SetFact

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// acceptClaimSeverity creates a claim, accepts it via CompleteRound, and returns the Fact.
func acceptClaimSeverity(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	claimID string,
	content string,
	conf uint64,
	roundID string,
	acceptBlock int64,
) *types.Fact {
	t.Helper()

	claim := &types.Claim{
		Id:               claimID,
		FactContent:      content,
		Domain:           "physics",
		Category:         "mechanics",
		Submitter:        makeValidBech32Addr(claimID + "-sub"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	advCtx := ctx.WithBlockHeight(acceptBlock)
	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: conf}
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found *types.Fact
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == content {
			fc := *f
			found = &fc
			return true
		}
		return false
	})
	require.NotNil(t, found, "accepted fact must exist")
	return found
}

// rejectChallengeSeverity creates a challenge claim and rejects it, simulating a failed challenge.
func rejectChallengeSeverity(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	challClaimID string,
	content string,
	factID string,
	rejectConf uint64,
	roundID string,
	submitBlock int64,
	rejectBlock int64,
) {
	t.Helper()

	challClaim := &types.Claim{
		Id:                challClaimID,
		FactContent:       content,
		Domain:            "physics",
		Category:          "mechanics",
		Submitter:         makeValidBech32Addr(challClaimID + "-chall"),
		SubmittedAtBlock:  uint64(submitBlock),
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             "1000uzrn",
		ProvisionalFactId: factID,
	}
	require.NoError(t, k.SetClaim(ctx.WithBlockHeight(submitBlock), challClaim))

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             challClaimID,
		StartedAtBlock:      uint64(submitBlock),
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      uint64(submitBlock) + 100,
		RevealDeadline:      uint64(submitBlock) + 200,
		AggregationDeadline: uint64(submitBlock) + 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx.WithBlockHeight(submitBlock), round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: rejectConf}
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(rejectBlock), round, result))
}

// ── Test 1: InitialSeverityOnAccept ──────────────────────────────────────────

func TestCorroborationSeverity_InitialSeverityOnAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	acceptConf := uint64(920_000)
	fact := acceptClaimSeverity(t, k, ctx, "cs-initial", "light speed in vacuum", acceptConf, "cs-initial-round", 400)

	require.Equal(t, acceptConf, fact.CorroborationSeverityScoreBps,
		"initial severity must equal acceptance panel confidence")
}

// ── Test 2: ZeroConfidenceGivesZeroSeverity ──────────────────────────────────

func TestCorroborationSeverity_ZeroConfidenceGivesZeroSeverity(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	fact := acceptClaimSeverity(t, k, ctx, "cs-zero", "zero confidence test claim", 0, "cs-zero-round", 400)

	require.Equal(t, uint64(0), fact.CorroborationSeverityScoreBps)
}

// ── Test 3: SeverityAccumulatesOnChallengeRejected ───────────────────────────

func TestCorroborationSeverity_AccumulatesOnChallengeRejected(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	acceptConf := uint64(800_000)
	origFact := acceptClaimSeverity(t, k, ctx, "cs-accum", "original fact for accum test", acceptConf, "cs-accum-round", 400)

	rejectConf := uint64(900_000)
	rejectChallengeSeverity(t, k, ctx, "cs-accum-chall", "challenge to accum fact",
		origFact.Id, rejectConf, "cs-accum-chall-round", 500, 900)

	updatedFact, found := k.GetFact(ctx.WithBlockHeight(1000), origFact.Id)
	require.True(t, found)
	require.Equal(t, acceptConf+rejectConf, updatedFact.CorroborationSeverityScoreBps,
		"severity = initial + rejection confidence")
}

// ── Test 4: SeverityAccumulatesUncooled ──────────────────────────────────────

func TestCorroborationSeverity_AccumulatesUncooled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	acceptConf := uint64(700_000)
	origFact := acceptClaimSeverity(t, k, ctx, "cs-uncooled", "original fact for uncooled test", acceptConf, "cs-uncooled-round", 400)

	// Two fast challenges within the count-cooldown window (< 1000 blocks between them)
	conf1 := uint64(800_000)
	rejectChallengeSeverity(t, k, ctx, "cs-uncooled-c1", "challenge 1 to uncooled",
		origFact.Id, conf1, "cs-uncooled-c1-round", 500, 900)

	conf2 := uint64(600_000)
	rejectChallengeSeverity(t, k, ctx, "cs-uncooled-c2", "challenge 2 to uncooled",
		origFact.Id, conf2, "cs-uncooled-c2-round", 502, 902)

	updatedFact, found := k.GetFact(ctx.WithBlockHeight(1100), origFact.Id)
	require.True(t, found)

	// All three severities accumulate regardless of timing
	require.Equal(t, acceptConf+conf1+conf2, updatedFact.CorroborationSeverityScoreBps,
		"severity must accumulate for all challenges regardless of timing (no cooldown)")
}

// ── Test 5: CountAndSeverityAreDistinct ──────────────────────────────────────

func TestCorroborationSeverity_CountAndSeverityAreDistinct(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	acceptConf := uint64(800_000)
	origFact := acceptClaimSeverity(t, k, ctx, "cs-distinct", "original fact for distinct test", acceptConf, "cs-distinct-round", 400)

	// Challenge 1 REJECT at block 900 (first after initial; clears cooldown from 0)
	conf1 := uint64(700_000)
	rejectChallengeSeverity(t, k, ctx, "cs-distinct-c1", "challenge 1 to distinct",
		origFact.Id, conf1, "cs-distinct-c1-round", 500, 900)

	// Challenge 2 REJECT at block 902 — within cooldown (< 1000 blocks later) → count NOT incremented
	conf2 := uint64(650_000)
	rejectChallengeSeverity(t, k, ctx, "cs-distinct-c2", "challenge 2 to distinct",
		origFact.Id, conf2, "cs-distinct-c2-round", 901, 902)

	updatedFact, found := k.GetFact(ctx.WithBlockHeight(1100), origFact.Id)
	require.True(t, found)

	// Count is cooldown-limited: challenge 1 cleared cooldown (0 → 900); challenge 2 at 902 within window
	require.Equal(t, uint64(1), updatedFact.CorroborationCount,
		"count must respect the 1000-block cooldown")
	// Severity accumulates for both regardless
	require.Equal(t, acceptConf+conf1+conf2, updatedFact.CorroborationSeverityScoreBps,
		"severity accumulates for all challenges, no cooldown")
}

// ── Test 6: HighConfidenceAddsMoreThanLow ────────────────────────────────────

func TestCorroborationSeverity_HighConfidenceAddsMoreThanLow(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Fact A: rejected with 90% confidence
	origFactA := acceptClaimSeverity(t, k, ctx, "cs-high", "original A for high test", 800_000, "cs-high-round", 400)
	rejectChallengeSeverity(t, k, ctx, "cs-high-c1", "challenge to A",
		origFactA.Id, 900_000, "cs-high-c1-round", 500, 900)

	// Fact B: rejected with 55% confidence
	origFactB := acceptClaimSeverity(t, k, ctx, "cs-low", "original B for low test", 800_000, "cs-low-round", 401)
	rejectChallengeSeverity(t, k, ctx, "cs-low-c1", "challenge to B",
		origFactB.Id, 550_000, "cs-low-c1-round", 500, 900)

	factA, foundA := k.GetFact(ctx.WithBlockHeight(1100), origFactA.Id)
	factB, foundB := k.GetFact(ctx.WithBlockHeight(1100), origFactB.Id)
	require.True(t, foundA)
	require.True(t, foundB)

	require.Greater(t, factA.CorroborationSeverityScoreBps, factB.CorroborationSeverityScoreBps,
		"90%% rejection should yield higher severity than 55%% rejection")
}

// ── Test 7: RejectNonChallengeNoop ───────────────────────────────────────────

func TestCorroborationSeverity_RejectNonChallengeNoop(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Accept original fact
	origFact := acceptClaimSeverity(t, k, ctx, "cs-noop", "original fact noop test", 750_000, "cs-noop-round", 400)
	initialSeverity := origFact.CorroborationSeverityScoreBps

	// REJECT a completely separate non-challenge claim (no ProvisionalFactId)
	unrelated := &types.Claim{
		Id:               "cs-noop-unrelated",
		FactContent:      "unrelated claim that gets rejected",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("cs-noop-other"),
		SubmittedAtBlock: 500,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// ProvisionalFactId intentionally empty — this is NOT a challenge
	}
	require.NoError(t, k.SetClaim(ctx, unrelated))

	unrelatedRound := &types.VerificationRound{
		Id: "cs-noop-unrelated-round", ClaimId: "cs-noop-unrelated",
		StartedAtBlock: 500, Phase: types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline: 600, RevealDeadline: 700, AggregationDeadline: 800,
	}
	require.NoError(t, k.SetVerificationRound(ctx, unrelatedRound))
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(900), unrelatedRound,
		&keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: 900_000}))

	// Original fact severity must be unchanged
	updatedFact, found := k.GetFact(ctx.WithBlockHeight(900), origFact.Id)
	require.True(t, found)
	require.Equal(t, initialSeverity, updatedFact.CorroborationSeverityScoreBps,
		"REJECT of non-challenge claim must not affect original fact's severity")
}

// ── Test 8: StoredAfterSurvival ───────────────────────────────────────────────

func TestCorroborationSeverity_StoredAfterSurvival(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	acceptConf := uint64(850_000)
	rejectConf := uint64(880_000)

	origFact := acceptClaimSeverity(t, k, ctx, "cs-stored", "original fact stored test", acceptConf, "cs-stored-round", 400)
	rejectChallengeSeverity(t, k, ctx, "cs-stored-c1", "challenge to stored",
		origFact.Id, rejectConf, "cs-stored-c1-round", 500, 900)

	// Fetch fresh from KV
	storedFact, found := k.GetFact(ctx.WithBlockHeight(1000), origFact.Id)
	require.True(t, found)
	require.Equal(t, acceptConf+rejectConf, storedFact.CorroborationSeverityScoreBps,
		"corroboration severity must persist to KV after SetFact")
}
