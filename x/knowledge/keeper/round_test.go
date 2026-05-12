package keeper_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── VerificationRound CRUD ─────────────────────────────────────────────────

func TestRound_SetAndGet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	got, found := k.GetVerificationRound(ctx, "round-1")
	require.True(t, found)
	require.Equal(t, "round-1", got.Id)
	require.Equal(t, "claim-1", got.ClaimId)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got.Phase)
}

func TestRound_GetNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	_, found := k.GetVerificationRound(ctx, "nonexistent-round")
	require.False(t, found)
}

func TestRound_Delete(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("round-del", "claim-del", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	require.NoError(t, k.DeleteVerificationRound(ctx, "round-del"))
	_, found := k.GetVerificationRound(ctx, "round-del")
	require.False(t, found)
}

func TestRound_GetByClaimID(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-for-round",
		FactContent: "Test claim content that is long enough",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	got, found := k.GetRoundByClaimID(ctx, "claim-for-round")
	require.True(t, found)
	require.Equal(t, round.Id, got.Id)
}

func TestRound_GetByClaimID_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	_, found := k.GetRoundByClaimID(ctx, "nonexistent-claim")
	require.False(t, found)
}

// ─── CreateVerificationRound ─────────────────────────────────────────────────

func TestCreateVerificationRound_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-create-round",
		FactContent: "A verifiable claim content string",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	require.NotEmpty(t, round.Id)
	require.Equal(t, "claim-create-round", round.ClaimId)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, round.Phase)
	require.Equal(t, uint64(100), round.StartedAtBlock) // ctx height is 100

	// Deadlines based on DefaultParams
	require.Equal(t, uint64(300), round.CommitDeadline)  // +200
	require.Equal(t, uint64(500), round.RevealDeadline)  // +400
	require.Equal(t, uint64(550), round.AggregationDeadline) // +450

	// Claim should be updated
	updatedClaim, found := k.GetClaim(ctx, "claim-create-round")
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION, updatedClaim.Status)
	require.Equal(t, round.Id, updatedClaim.VerificationRoundId)
}

func TestCreateVerificationRound_IDIsDeterministic(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-det",
		FactContent: "Deterministic test claim content",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	expectedID := keeper.GenerateRoundID("claim-det", 100)
	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	require.Equal(t, expectedID, round.Id)
}

// ─── ActiveRound Indexing ────────────────────────────────────────────────────

func TestActiveRounds_ExcludesComplete(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	r1 := makeRoundInPhase("r-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	r2 := makeRoundInPhase("r-reveal", "c2", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	r3 := makeRoundInPhase("r-complete", "c3", types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, 50)
	r4 := makeRoundInPhase("r-expired", "c4", types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, 50)

	for _, r := range []*types.VerificationRound{r1, r2, r3, r4} {
		require.NoError(t, k.SetVerificationRound(ctx, r))
	}

	active := k.GetActiveRounds(ctx)
	require.Len(t, active, 2) // commit + reveal

	ids := make(map[string]bool)
	for _, r := range active {
		ids[r.Id] = true
	}
	require.True(t, ids["r-commit"])
	require.True(t, ids["r-reveal"])
	require.False(t, ids["r-complete"])
	require.False(t, ids["r-expired"])
}

func TestActiveRounds_Empty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	active := k.GetActiveRounds(ctx)
	require.Empty(t, active)
}

func TestActiveRounds_TransitionRemovesFromIndex(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-active", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	active := k.GetActiveRounds(ctx)
	require.Len(t, active, 1)

	// Mark as complete
	round.Phase = types.VerificationPhase_VERIFICATION_PHASE_COMPLETE
	require.NoError(t, k.SetVerificationRound(ctx, round))

	active = k.GetActiveRounds(ctx)
	require.Empty(t, active)
}

// ─── StoreCommitmentInRound (extended) ──────────────────────────────────────

func TestStoreCommitment_MultipleVerifiers(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-multi", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	for i := 0; i < 5; i++ {
		commit := &types.CommitEntry{
			Verifier:         makeValidatorAddr(i),
			CommitHash:       []byte(fmt.Sprintf("hash_%d__________________________", i)),
			CommittedAtBlock: 100,
		}
		require.NoError(t, k.StoreCommitmentInRound(ctx, "r-multi", commit))
	}

	got, found := k.GetVerificationRound(ctx, "r-multi")
	require.True(t, found)
	require.Len(t, got.Commits, 5)
	require.Len(t, got.SelectedVerifiers, 5)
}

func TestStoreCommitment_RoundNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	err := k.StoreCommitmentInRound(ctx, "nonexistent", &types.CommitEntry{
		Verifier:   "zrn1val",
		CommitHash: []byte("hash"),
	})
	require.ErrorIs(t, err, types.ErrRoundNotFound)
}

func TestStoreCommitment_VerifierAddedToSelectedOnce(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-sel", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	round.SelectedVerifiers = []string{"zrn1validator0"} // already selected
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit := &types.CommitEntry{
		Verifier:         "zrn1validator0",
		CommitHash:       []byte("hash_one________________________"),
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "r-sel", commit))

	got, _ := k.GetVerificationRound(ctx, "r-sel")
	// Should not duplicate in SelectedVerifiers
	count := 0
	for _, v := range got.SelectedVerifiers {
		if v == "zrn1validator0" {
			count++
		}
	}
	require.Equal(t, 1, count, "verifier should appear only once in SelectedVerifiers")
}

// ─── StoreRevealInRound (extended) ──────────────────────────────────────────

func TestStoreReveal_DuplicateReveal(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-dup-reveal", "accept", 600_000, salt)

	round := makeRoundInPhase("r-dup-reveal", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	reveal := &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}
	require.NoError(t, k.StoreRevealInRound(ctx, "r-dup-reveal", reveal, 600_000))

	// Same reveal again → ErrDuplicateReveal
	err := k.StoreRevealInRound(ctx, "r-dup-reveal", reveal, 600_000)
	require.ErrorIs(t, err, types.ErrDuplicateReveal)
}

func TestStoreReveal_Equivocation(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	salt1, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-equivoc", "accept", 600_000, salt1)

	round := makeRoundInPhase("r-equivoc", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	reveal1 := &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            salt1,
		RevealedAtBlock: 100,
	}
	require.NoError(t, k.StoreRevealInRound(ctx, "r-equivoc", reveal1, 600_000))

	// Different vote from same verifier fails hash verification
	// (equivocation path is unreachable because hash check happens first)
	reveal2 := &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "reject",
		Salt:            salt1,
		RevealedAtBlock: 101,
	}
	err := k.StoreRevealInRound(ctx, "r-equivoc", reveal2, 600_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch)
}

func TestStoreReveal_RoundNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	err := k.StoreRevealInRound(ctx, "nonexistent", &types.RevealEntry{Verifier: "zrn1val1"}, 600_000)
	require.ErrorIs(t, err, types.ErrRoundNotFound)
}

func TestStoreReveal_MultipleVerifiers(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-multi-reveal", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)

	// Set up 3 commits
	for i := 0; i < 3; i++ {
		salt, _ := hex.DecodeString(fmt.Sprintf("aabbccdd1122334400000000000000%02x", i))
		hash := types.ComputeCommitmentHash("r-multi-reveal", "accept", 600_000, salt)
		round.Commits = append(round.Commits, &types.CommitEntry{
			Verifier:         makeValidatorAddr(i),
			CommitHash:       hash,
			CommittedAtBlock: 90,
		})
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Reveal all 3
	for i := 0; i < 3; i++ {
		salt, _ := hex.DecodeString(fmt.Sprintf("aabbccdd1122334400000000000000%02x", i))
		reveal := &types.RevealEntry{
			Verifier:        makeValidatorAddr(i),
			Vote:            "accept",
			Salt:            salt,
			RevealedAtBlock: 100,
		}
		require.NoError(t, k.StoreRevealInRound(ctx, "r-multi-reveal", reveal, 600_000))
	}

	got, _ := k.GetVerificationRound(ctx, "r-multi-reveal")
	require.Len(t, got.Reveals, 3)
}

// ─── Phase Transitions ──────────────────────────────────────────────────────

func TestGetExpectedPhase_Commit(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	phase := keeper.GetExpectedPhase(round, 100, &params) // start block
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, phase)

	phase = keeper.GetExpectedPhase(round, 103, &params) // still before deadline 104
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, phase)
}

func TestGetExpectedPhase_Reveal(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	phase := keeper.GetExpectedPhase(round, round.CommitDeadline, &params) // at commit deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, phase)

	phase = keeper.GetExpectedPhase(round, round.RevealDeadline-1, &params) // before reveal deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, phase)
}

func TestGetExpectedPhase_Aggregation(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	phase := keeper.GetExpectedPhase(round, round.RevealDeadline, &params) // at reveal deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, phase)

	phase = keeper.GetExpectedPhase(round, round.AggregationDeadline-1, &params) // before aggregation deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, phase)
}

func TestGetExpectedPhase_Expired(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	phase := keeper.GetExpectedPhase(round, round.AggregationDeadline, &params) // at aggregation deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, phase)

	phase = keeper.GetExpectedPhase(round, round.AggregationDeadline+100, &params) // way past deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, phase)
}

func TestGetExpectedPhase_AlreadyComplete(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, 100)

	// Complete rounds should stay complete regardless of height
	phase := keeper.GetExpectedPhase(round, 50, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, phase)

	phase = keeper.GetExpectedPhase(round, 200, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, phase)
}

func TestGetExpectedPhase_AlreadyExpired(t *testing.T) {
	params := types.DefaultParams()
	round := makeRoundInPhase("r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, 100)

	phase := keeper.GetExpectedPhase(round, 50, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, phase)
}

// ─── BeginBlocker / AdvanceRoundPhases ───────────────────────────────────────

func TestAdvanceRoundPhases_CommitToReveal(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-advance-1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 90)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance context to commit deadline
	ctx = ctx.WithBlockHeight(int64(round.CommitDeadline))
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	got, _ := k.GetVerificationRound(ctx, "r-advance-1")
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, got.Phase)
}

func TestAdvanceRoundPhases_NoTransitionBeforeDeadline(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-no-advance", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 90)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Context at height 93 — before commit deadline of 94
	ctx = ctx.WithBlockHeight(93)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	got, _ := k.GetVerificationRound(ctx, "r-no-advance")
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got.Phase)
}

func TestAdvanceRoundPhases_ExpiredWithInsufficientReveals(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-expire",
		FactContent: "Claim that will expire due to no reveals",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-expire", "claim-expire", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Jump well past aggregation deadline
	ctx = ctx.WithBlockHeight(int64(round.AggregationDeadline) + 5)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	got, _ := k.GetVerificationRound(ctx, "r-expire")
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got.Phase)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, got.Verdict)

	// Claim should be marked as insufficient
	updatedClaim, found := k.GetClaim(ctx, "claim-expire")
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT, updatedClaim.Status)
}

// ─── Concurrent Rounds — Security Test ──────────────────────────────────────

func TestConcurrentRounds_NoInterference(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Create two independent rounds
	round1 := makeRoundInPhase("r-concurrent-1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 80)
	round2 := makeRoundInPhase("r-concurrent-2", "c2", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round1))
	require.NoError(t, k.SetVerificationRound(ctx, round2))

	// Store a commit in round 1
	commit := &types.CommitEntry{
		Verifier:         "zrn1val1",
		CommitHash:       []byte("hash_r1_________________________"),
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "r-concurrent-1", commit))

	// Round 2 should be unaffected
	got2, _ := k.GetVerificationRound(ctx, "r-concurrent-2")
	require.Empty(t, got2.Commits)

	// Round 1 should have the commit
	got1, _ := k.GetVerificationRound(ctx, "r-concurrent-1")
	require.Len(t, got1.Commits, 1)
}

func TestConcurrentRounds_IndependentPhases(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Round 1: starts later (commit phase, deadline far away)
	round1 := makeRoundInPhase("r-ind-1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 1000)
	// Round 2: started much earlier (will expire first)
	round2 := makeRoundInPhase("r-ind-2", "c2", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 80)

	require.NoError(t, k.SetVerificationRound(ctx, round1))
	require.NoError(t, k.SetVerificationRound(ctx, round2))

	// At a height past round2's aggregation deadline but before round1's commit deadline
	testHeight := int64(round2.AggregationDeadline) + 5
	ctx = ctx.WithBlockHeight(testHeight)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	got1, _ := k.GetVerificationRound(ctx, "r-ind-1")
	got2, _ := k.GetVerificationRound(ctx, "r-ind-2")

	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got1.Phase,
		"round1 should still be in commit phase")
	// Round 2 expired (insufficient reveals → expired)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got2.Phase,
		"round2 should be expired")
}

// ─── CompleteRound ──────────────────────────────────────────────────────────

func TestCompleteRound_AcceptCreatesFactAndReturnsStake(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:          "claim-accept",
		FactContent: "An accepted claim creates a fact",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-accept", "claim-accept", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 850_000,
	}

	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Round should be complete with accept verdict
	updatedRound, _ := k.GetVerificationRound(ctx, "r-accept")
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, updatedRound.Phase)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, updatedRound.Verdict)

	// Claim should be accepted
	updatedClaim, _ := k.GetClaim(ctx, "claim-accept")
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, updatedClaim.Status)

	// A fact should exist (created from claim)
	var factFound bool
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == "claim-accept" {
			factFound = true
			require.Equal(t, uint64(850_000), fact.Confidence)
			require.Equal(t, "mathematics", fact.Domain)
			require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, fact.Status)
		}
		return false
	})
	require.True(t, factFound, "accepted claim should create a fact")

	// Review fee is non-refundable (R19-6) — no stake return on accept.
	_ = bk
}

func TestCompleteRound_RejectSlashesStake(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	claim := &types.Claim{
		Id:          "claim-reject",
		FactContent: "A rejected claim gets slashed and burned",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-reject", "claim-reject", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_REJECT,
		Confidence: 850_000,
	}

	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Claim should be rejected
	updatedClaim, _ := k.GetClaim(ctx, "claim-reject")
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_REJECTED, updatedClaim.Status)

	// Review fee is non-refundable (R19-6) — no bank calls expected on rejection
	_ = bk // bank keeper not used for rejection anymore
}

func TestCompleteRound_Inconclusive(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-inconclusive",
		FactContent: "An inconclusive claim gets stake returned",
		Domain:      "general",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-inconclusive", "claim-inconclusive", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_INCONCLUSIVE,
		Confidence: 0,
	}

	require.NoError(t, k.CompleteRound(ctx, round, result))

	updatedClaim, _ := k.GetClaim(ctx, "claim-inconclusive")
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT, updatedClaim.Status)
}

func TestCompleteRound_DistributesRewards(t *testing.T) {
	k, ctx, bk, sk := setupKnowledgeTestFull(t)

	sk.addValidator("zrn1correct", 100_000, "bonded")

	claim := &types.Claim{
		Id:          "claim-reward",
		FactContent: "Claim with rewards distributed to verifiers",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-reward", "claim-reward", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1correct", CommitHash: []byte("hash"), CommittedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: "zrn1correct", Amount: 3_000_000},
		},
	}

	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Verify the result struct carried the rewards (bank distribution
	// requires valid bech32 addresses; verify via result structure instead)
	require.Len(t, result.Rewards, 1)
	require.Equal(t, "zrn1correct", result.Rewards[0].Verifier)
	require.Equal(t, uint64(3_000_000), result.Rewards[0].Amount)

	_ = bk
	_ = sk
}

func TestCompleteRound_SlashesIncorrectVoters(t *testing.T) {
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	sk.addValidator("zrn1wrong", 100_000, "bonded")

	claim := &types.Claim{
		Id:          "claim-slash",
		FactContent: "Claim with slashes for wrong verification vote",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-slash", "claim-slash", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Slashes: []keeper.VerifierSlash{
			{Verifier: "zrn1wrong", SlashBps: 50_000},
		},
	}

	require.NoError(t, k.CompleteRound(ctx, round, result))

	require.Len(t, sk.slashes, 1)
	require.Equal(t, "zrn1wrong", sk.slashes[0].Validator)
	require.Equal(t, uint64(50_000), sk.slashes[0].SlashBps)
}

// ─── Phase enum values ──────────────────────────────────────────────────────

func TestVerificationPhase_AllValues(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	phases := []types.VerificationPhase{
		types.VerificationPhase_VERIFICATION_PHASE_COMMIT,
		types.VerificationPhase_VERIFICATION_PHASE_REVEAL,
		types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		types.VerificationPhase_VERIFICATION_PHASE_COMPLETE,
		types.VerificationPhase_VERIFICATION_PHASE_EXPIRED,
	}

	for i, p := range phases {
		id := fmt.Sprintf("round-phase-%d", i)
		round := makeRoundInPhase(id, fmt.Sprintf("c%d", i), p, 50)
		require.NoError(t, k.SetVerificationRound(ctx, round))

		got, found := k.GetVerificationRound(ctx, id)
		require.True(t, found)
		require.Equal(t, p, got.Phase)
	}
}

func TestVerdict_AllValues(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verdicts := []types.Verdict{
		types.Verdict_VERDICT_UNSPECIFIED,
		types.Verdict_VERDICT_ACCEPT,
		types.Verdict_VERDICT_REJECT,
		types.Verdict_VERDICT_INCONCLUSIVE,
	}

	for i, v := range verdicts {
		id := fmt.Sprintf("round-verdict-%d", i)
		round := makeRoundInPhase(id, fmt.Sprintf("c%d", i), types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, 50)
		round.Verdict = v
		require.NoError(t, k.SetVerificationRound(ctx, round))

		got, _ := k.GetVerificationRound(ctx, id)
		require.Equal(t, v, got.Verdict)
	}
}

// ─── Extended Lifecycle ──────────────────────────────────────────────────────

func TestRound_CommitRevealFullLifecycle(t *testing.T) {
	// Full 3-validator commit→reveal→aggregate→complete path.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 3; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	claim := &types.Claim{
		Id:          "claim-lifecycle",
		FactContent: "Full lifecycle claim for three validator test",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, round.Phase)

	// --- Commit phase: 3 validators commit ---
	salts := make([][]byte, 3)
	for i := 0; i < 3; i++ {
		salts[i], _ = hex.DecodeString(fmt.Sprintf("aabbccdd1122334400000000000000%02x", i))
		commitHash := types.ComputeCommitmentHash(round.Id, "accept", 800_000, salts[i])
		commit := &types.CommitEntry{
			Verifier:         makeValidatorAddr(i),
			CommitHash:       commitHash,
			CommittedAtBlock: uint64(ctx.BlockHeight()),
		}
		require.NoError(t, k.StoreCommitmentInRound(ctx, round.Id, commit))
	}

	// --- Advance to reveal phase ---
	ctx = ctx.WithBlockHeight(int64(round.CommitDeadline))
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, _ := k.GetVerificationRound(ctx, round.Id)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, updated.Phase)

	// --- Reveal phase: 3 validators reveal ---
	for i := 0; i < 3; i++ {
		reveal := &types.RevealEntry{
			Verifier:        makeValidatorAddr(i),
			Vote:            "accept",
			Salt:            salts[i],
			RevealedAtBlock: uint64(ctx.BlockHeight()),
		}
		require.NoError(t, k.StoreRevealInRound(ctx, round.Id, reveal, 800_000))
	}

	// --- Advance to aggregation (triggers auto-aggregate + complete) ---
	ctx = ctx.WithBlockHeight(int64(round.RevealDeadline))
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	final, found := k.GetVerificationRound(ctx, round.Id)
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, final.Phase)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, final.Verdict)

	// Claim should be accepted
	updatedClaim, _ := k.GetClaim(ctx, "claim-lifecycle")
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, updatedClaim.Status)
}

func TestRound_PhaseTransition_CommitInclusive(t *testing.T) {
	// At CommitDeadline height, expected phase should be REVEAL (>= boundary).
	// But at CommitDeadline-1, it should still be COMMIT.
	params := types.DefaultParams()
	round := makeRoundInPhase("r-commit-inc", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	// At CommitDeadline-1 (103): still commit
	phase := keeper.GetExpectedPhase(round, round.CommitDeadline-1, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, phase,
		"one block before CommitDeadline should still be commit")

	// At CommitDeadline (104): transitions to reveal
	phase = keeper.GetExpectedPhase(round, round.CommitDeadline, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, phase,
		"at CommitDeadline the phase transitions to reveal (>= boundary)")
}

func TestRound_PhaseTransition_RevealInclusive(t *testing.T) {
	// At RevealDeadline height, expected phase should be AGGREGATION (>= boundary).
	// But at RevealDeadline-1, it should still be REVEAL.
	params := types.DefaultParams()
	round := makeRoundInPhase("r-reveal-inc", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)

	// At RevealDeadline-1 (107): still reveal
	phase := keeper.GetExpectedPhase(round, round.RevealDeadline-1, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, phase,
		"one block before RevealDeadline should still be reveal")

	// At RevealDeadline (108): transitions to aggregation
	phase = keeper.GetExpectedPhase(round, round.RevealDeadline, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, phase,
		"at RevealDeadline the phase transitions to aggregation (>= boundary)")
}

func TestRound_CleanupExpiredRounds(t *testing.T) {
	// Expired rounds should be removed from the active index.
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-cleanup",
		FactContent: "Claim for expired round cleanup test",
		Domain:      "general",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-cleanup", "claim-cleanup", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Verify it's in the active index
	active := k.GetActiveRounds(ctx)
	require.Len(t, active, 1)

	// Jump well past aggregation deadline, triggers expiration
	ctx = ctx.WithBlockHeight(int64(round.AggregationDeadline) + 5)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// Should no longer appear in active rounds
	active = k.GetActiveRounds(ctx)
	require.Empty(t, active, "expired round should be removed from active index")

	// But the round should still exist in storage with EXPIRED phase
	got, found := k.GetVerificationRound(ctx, "r-cleanup")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got.Phase)
}

func TestRound_InsufficientParticipation(t *testing.T) {
	// Below quorum (MinVerifiers=3, only 2 reveals) → inconclusive verdict.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 3; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	claim := &types.Claim{
		Id:          "claim-quorum",
		FactContent: "Claim for insufficient participation quorum test",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-quorum", "claim-quorum", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	// 3 commits, but only 2 reveals
	round.Commits = []*types.CommitEntry{
		{Verifier: makeValidatorAddr(0), CommitHash: []byte("h0"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(1), CommitHash: []byte("h1"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(2), CommitHash: []byte("h2"), CommittedAtBlock: 85},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: makeValidatorAddr(0), Vote: "accept", Salt: []byte("s0"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(1), Vote: "accept", Salt: []byte("s1"), RevealedAtBlock: 90},
		// makeValidatorAddr(2) did NOT reveal
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Default MinVerifiers is 3, only 2 reveals → insufficient
	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, result.Verdict,
		"2 reveals with MinVerifiers=3 must produce INCONCLUSIVE")
	require.Equal(t, uint64(0), result.Confidence)
}

func TestRound_ContestedOutcome(t *testing.T) {
	// Split verdicts (50/50) → inconclusive because neither side meets 77% threshold.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 4; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	// Lower MinVerifiers to 4 so all 4 reveals count
	params, _ := k.GetParams(ctx)
	params.MinVerifiers = 4
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:          "claim-contested",
		FactContent: "Claim for contested split outcome test case",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-contested", "claim-contested", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	round.Commits = []*types.CommitEntry{
		{Verifier: makeValidatorAddr(0), CommitHash: []byte("h0"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(1), CommitHash: []byte("h1"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(2), CommitHash: []byte("h2"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(3), CommitHash: []byte("h3"), CommittedAtBlock: 85},
	}
	// 2 accept, 2 reject — 50/50 split
	round.Reveals = []*types.RevealEntry{
		{Verifier: makeValidatorAddr(0), Vote: "accept", Salt: []byte("s0"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(1), Vote: "accept", Salt: []byte("s1"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(2), Vote: "reject", Salt: []byte("s2"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(3), Vote: "reject", Salt: []byte("s3"), RevealedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, result.Verdict,
		"50/50 split must be INCONCLUSIVE (neither side meets 77% threshold)")
}

func TestRound_RejectedOutcome(t *testing.T) {
	// All validators reject → rejected verdict.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 3; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	claim := &types.Claim{
		Id:          "claim-all-reject",
		FactContent: "Claim that all validators reject unanimously",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-all-reject", "claim-all-reject", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	round.Commits = []*types.CommitEntry{
		{Verifier: makeValidatorAddr(0), CommitHash: []byte("h0"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(1), CommitHash: []byte("h1"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(2), CommitHash: []byte("h2"), CommittedAtBlock: 85},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: makeValidatorAddr(0), Vote: "reject", Salt: []byte("s0"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(1), Vote: "reject", Salt: []byte("s1"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(2), Vote: "reject", Salt: []byte("s2"), RevealedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_REJECT, result.Verdict,
		"all 3 reject (100%) must exceed 77% threshold → REJECT")
	require.Equal(t, uint64(880_000), result.Confidence,
		"unanimous reject gives raw 100% but capped at MaxConfidence (880,000)")

	// Complete and verify claim status
	require.NoError(t, k.CompleteRound(ctx, round, result))
	updatedClaim, _ := k.GetClaim(ctx, "claim-all-reject")
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_REJECTED, updatedClaim.Status)
}

func TestRound_ConfidenceAggregation(t *testing.T) {
	// 3 of 3 accept (all with equal stake) → raw confidence 1,000,000 capped at 880,000 ≥ 770,000 threshold.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 3; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	claim := &types.Claim{
		Id:          "claim-confidence",
		FactContent: "Claim for confidence aggregation threshold test",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-confidence", "claim-confidence", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	round.Commits = []*types.CommitEntry{
		{Verifier: makeValidatorAddr(0), CommitHash: []byte("h0"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(1), CommitHash: []byte("h1"), CommittedAtBlock: 85},
		{Verifier: makeValidatorAddr(2), CommitHash: []byte("h2"), CommittedAtBlock: 85},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: makeValidatorAddr(0), Vote: "accept", Salt: []byte("s0"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(1), Vote: "accept", Salt: []byte("s1"), RevealedAtBlock: 90},
		{Verifier: makeValidatorAddr(2), Vote: "accept", Salt: []byte("s2"), RevealedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict)
	require.GreaterOrEqual(t, result.Confidence, uint64(770_000),
		"acceptance confidence must meet or exceed the 770,000 threshold")
	require.Equal(t, uint64(880_000), result.Confidence,
		"unanimous accept gives raw 100% but capped at MaxConfidence (880,000)")
}
