package keeper_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── PoT Security Tests ─────────────────────────────────────────────────────
//
// These tests exercise the Proof-of-Truth commit-reveal scheme at the keeper
// layer, focusing on hash integrity, replay resistance, equivocation
// detection, and aggregation determinism.

func TestSecurity_HappyPath_CommitRevealFinalize(t *testing.T) {
	// Full PoT happy path: commit→reveal→aggregate→complete with fact creation.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 3; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	claim := &types.Claim{
		Id:          "claim-sec-happy",
		FactContent: "Security happy path claim for commit reveal finalize test",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round, err := k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err)

	// Commit phase
	salts := make([][]byte, 3)
	for i := 0; i < 3; i++ {
		salts[i], _ = hex.DecodeString(fmt.Sprintf("deadbeef1122334400000000000000%02x", i))
		commitHash := types.ComputeCommitmentHash(round.Id, "accept", 850_000, salts[i])
		require.NoError(t, k.StoreCommitmentInRound(ctx, round.Id, &types.CommitEntry{
			Verifier:         makeValidatorAddr(i),
			CommitHash:       commitHash,
			CommittedAtBlock: uint64(ctx.BlockHeight()),
		}))
	}

	// Transition to reveal
	ctx = ctx.WithBlockHeight(int64(round.CommitDeadline))
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// Reveal phase
	for i := 0; i < 3; i++ {
		require.NoError(t, k.StoreRevealInRound(ctx, round.Id, &types.RevealEntry{
			Verifier:        makeValidatorAddr(i),
			Vote:            "accept",
			Salt:            salts[i],
			RevealedAtBlock: uint64(ctx.BlockHeight()),
		}, 850_000))
	}

	// Transition to aggregation (auto-aggregates and completes)
	ctx = ctx.WithBlockHeight(int64(round.RevealDeadline))
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	final, found := k.GetVerificationRound(ctx, round.Id)
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, final.Phase)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, final.Verdict)

	// Verify a fact was created
	var factFound bool
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == "claim-sec-happy" {
			factFound = true
			require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, fact.Status)
		}
		return false
	})
	require.True(t, factFound, "accepted claim must create a verified fact")
}

func TestSecurity_TamperedReveal_VerdictChange(t *testing.T) {
	// Commit "accept" but reveal "reject" — hash check must reject.
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-tamper-vote", "accept", 800_000, salt)

	round := makeRoundInPhase("r-tamper-vote", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Reveal with changed vote ("reject" instead of "accept")
	err := k.StoreRevealInRound(ctx, "r-tamper-vote", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "reject",
		Salt:            salt,
		RevealedAtBlock: 100,
	}, 800_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch,
		"changed vote must fail hash verification")
}

func TestSecurity_TamperedReveal_ConfidenceChange(t *testing.T) {
	// Commit with confidence 800_000 but reveal with 999_000 — hash check must reject.
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-tamper-conf", "accept", 800_000, salt)

	round := makeRoundInPhase("r-tamper-conf", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Reveal with tampered confidence (999_000 instead of 800_000)
	err := k.StoreRevealInRound(ctx, "r-tamper-conf", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}, 999_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch,
		"changed confidence must fail hash verification")
}

func TestSecurity_TamperedReveal_WrongSalt(t *testing.T) {
	// Commit with one salt but reveal with different salt — hash check must reject.
	k, ctx := setupKnowledgeTest(t)

	originalSalt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	wrongSalt, _ := hex.DecodeString("1122334455667788aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-tamper-salt", "accept", 800_000, originalSalt)

	round := makeRoundInPhase("r-tamper-salt", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Reveal with wrong salt
	err := k.StoreRevealInRound(ctx, "r-tamper-salt", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            wrongSalt,
		RevealedAtBlock: 100,
	}, 800_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch,
		"wrong salt must fail hash verification")
}

func TestSecurity_TamperedReveal_AllChanged(t *testing.T) {
	// All reveal fields tampered simultaneously — hash check must reject.
	k, ctx := setupKnowledgeTest(t)

	originalSalt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	wrongSalt, _ := hex.DecodeString("1122334455667788aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("r-tamper-all", "accept", 800_000, originalSalt)

	round := makeRoundInPhase("r-tamper-all", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: commitHash, CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Reveal with all fields changed
	err := k.StoreRevealInRound(ctx, "r-tamper-all", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "reject",
		Salt:            wrongSalt,
		RevealedAtBlock: 100,
	}, 999_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch,
		"all fields tampered must fail hash verification")
}

func TestSecurity_CrossRoundReplay(t *testing.T) {
	// Commitment hash includes roundID — reusing a commit from one round in
	// another must fail verification (domain separation).
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")

	// Create commit for round A
	hashForRoundA := types.ComputeCommitmentHash("round-A", "accept", 800_000, salt)
	hashForRoundB := types.ComputeCommitmentHash("round-B", "accept", 800_000, salt)

	// The hashes must differ since the roundID is domain-separated
	require.NotEqual(t, hashForRoundA, hashForRoundB,
		"commitment hashes for different rounds must differ (domain separation)")

	// Now prove the actual keeper rejects: commit with round-A's hash in round-B
	roundB := makeRoundInPhase("round-B", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	roundB.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: hashForRoundA, CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, roundB))

	// Reveal with correct vote/salt/confidence for round-B, but commit was round-A's hash
	err := k.StoreRevealInRound(ctx, "round-B", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}, 800_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch,
		"cross-round replayed commitment must fail hash verification")
}

func TestSecurity_DuplicateCommit_Idempotent(t *testing.T) {
	// Same verifier submitting same hash twice returns ErrDuplicateCommitment (not equivocation).
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-dup-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	hash := []byte("consistent_hash_for_idempotency_")
	commit := &types.CommitEntry{
		Verifier:         "zrn1val1",
		CommitHash:       hash,
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "r-dup-commit", commit))

	err := k.StoreCommitmentInRound(ctx, "r-dup-commit", commit)
	require.ErrorIs(t, err, types.ErrDuplicateCommitment,
		"same hash from same verifier must be ErrDuplicateCommitment")
	require.NotErrorIs(t, err, types.ErrEquivocation,
		"same hash must NOT be treated as equivocation")
}

func TestSecurity_DuplicateCommit_Equivocation(t *testing.T) {
	// Same verifier submitting different hash = ErrEquivocation.
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-equivoc-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit1 := &types.CommitEntry{
		Verifier:         "zrn1val1",
		CommitHash:       []byte("hash_version_one____________________"),
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "r-equivoc-commit", commit1))

	commit2 := &types.CommitEntry{
		Verifier:         "zrn1val1",
		CommitHash:       []byte("hash_version_two____________________"),
		CommittedAtBlock: 101,
	}
	err := k.StoreCommitmentInRound(ctx, "r-equivoc-commit", commit2)
	require.ErrorIs(t, err, types.ErrEquivocation,
		"different hash from same verifier must be ErrEquivocation")
}

func TestSecurity_RoundCompletionIdempotent(t *testing.T) {
	// Double CompleteRound should produce no additional side effects.
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-double-complete",
		FactContent: "Claim for double round completion idempotency test",
		Domain:      "general",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-double-complete", "claim-double-complete", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
	}

	// First completion
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Count facts after first completion
	factCount1 := 0
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == "claim-double-complete" {
			factCount1++
		}
		return false
	})

	// Second completion — round is already COMPLETE
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Fact count should remain the same (idempotent, or at most a duplicate
	// fact is harmless since the same ID will be generated)
	factCount2 := 0
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == "claim-double-complete" {
			factCount2++
		}
		return false
	})
	require.Equal(t, factCount1, factCount2,
		"double CompleteRound must not create duplicate facts")
}

func TestSecurity_AggregationDeterminism(t *testing.T) {
	// Reordered reveals must produce the same aggregation result.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 4; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	params, _ := k.GetParams(ctx)
	params.MinVerifiers = 4
	require.NoError(t, k.SetParams(ctx, params))

	makeRound := func(id, claimID string, revealOrder []int) *types.VerificationRound {
		claim := &types.Claim{
			Id:          claimID,
			FactContent: "Determinism test claim content for order " + id,
			Domain:      "physics",
			Submitter:   "zrn1sub",
			Stake:       "1000000",
			Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		}
		require.NoError(t, k.SetClaim(ctx, claim))

		round := makeRoundInPhase(id, claimID, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
		for _, idx := range revealOrder {
			round.Commits = append(round.Commits, &types.CommitEntry{
				Verifier: makeValidatorAddr(idx), CommitHash: []byte(fmt.Sprintf("h%d", idx)), CommittedAtBlock: 85,
			})
			vote := "accept"
			if idx == 3 {
				vote = "reject"
			}
			round.Reveals = append(round.Reveals, &types.RevealEntry{
				Verifier: makeValidatorAddr(idx), Vote: vote, Salt: []byte(fmt.Sprintf("s%d", idx)), RevealedAtBlock: 90,
			})
		}
		require.NoError(t, k.SetVerificationRound(ctx, round))
		return round
	}

	// Order A: 0,1,2,3
	roundA := makeRound("r-det-a", "claim-det-a", []int{0, 1, 2, 3})
	resultA, err := k.AggregateVerificationResult(ctx, roundA)
	require.NoError(t, err)

	// Order B: 3,2,1,0 (reversed)
	roundB := makeRound("r-det-b", "claim-det-b", []int{3, 2, 1, 0})
	resultB, err := k.AggregateVerificationResult(ctx, roundB)
	require.NoError(t, err)

	require.Equal(t, resultA.Verdict, resultB.Verdict,
		"reordered reveals must produce same verdict")
	require.Equal(t, resultA.Confidence, resultB.Confidence,
		"reordered reveals must produce same confidence")
}

func TestSecurity_ProposerCensorship(t *testing.T) {
	// Missing validator votes reduce participation — if a proposer censors
	// some reveals, the participation count drops and may cause INCONCLUSIVE.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	for i := 0; i < 5; i++ {
		sk.addValidator(makeValidatorAddr(i), 100_000, "bonded")
	}

	// Set MinVerifiers=4 so 5 commits but only 3 reveals is insufficient
	params, _ := k.GetParams(ctx)
	params.MinVerifiers = 4
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:          "claim-censor",
		FactContent: "Claim for proposer censorship participation test",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-censor", "claim-censor", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	for i := 0; i < 5; i++ {
		round.Commits = append(round.Commits, &types.CommitEntry{
			Verifier: makeValidatorAddr(i), CommitHash: []byte(fmt.Sprintf("h%d", i)), CommittedAtBlock: 85,
		})
	}
	// Only 3 of 5 reveal (censored 2)
	for i := 0; i < 3; i++ {
		round.Reveals = append(round.Reveals, &types.RevealEntry{
			Verifier: makeValidatorAddr(i), Vote: "accept", Salt: []byte(fmt.Sprintf("s%d", i)), RevealedAtBlock: 90,
		})
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, result.Verdict,
		"3 reveals with MinVerifiers=4 must produce INCONCLUSIVE due to censorship")
}

func TestSecurity_RevealWithoutCommit(t *testing.T) {
	// Reveal from a verifier who never committed must return ErrNoCommitment.
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")

	round := makeRoundInPhase("r-no-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	// No commits at all
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.StoreRevealInRound(ctx, "r-no-commit", &types.RevealEntry{
		Verifier:        "zrn1attacker",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}, 800_000)
	require.ErrorIs(t, err, types.ErrNoCommitment,
		"reveal without prior commit must return ErrNoCommitment")
}

func TestSecurity_SlashOnMissedReveal(t *testing.T) {
	// Committed but no reveal → slashed with MissedRevealSlashBps during aggregation.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	sk.addValidator("zrn1revealer", 100_000, "bonded")
	sk.addValidator("zrn1skipper", 100_000, "bonded")
	sk.addValidator("zrn1revealer2", 100_000, "bonded")

	claim := &types.Claim{
		Id:          "claim-missed-reveal",
		FactContent: "Claim for missed reveal slash security test",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Lower MinVerifiers so 2 reveals is sufficient for aggregation
	params, _ := k.GetParams(ctx)
	params.MinVerifiers = 2
	require.NoError(t, k.SetParams(ctx, params))

	round := makeRoundInPhase("r-missed-reveal", "claim-missed-reveal", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1revealer", CommitHash: []byte("h1"), CommittedAtBlock: 85},
		{Verifier: "zrn1skipper", CommitHash: []byte("h2"), CommittedAtBlock: 85},
		{Verifier: "zrn1revealer2", CommitHash: []byte("h3"), CommittedAtBlock: 85},
	}
	// Only 2 of 3 reveal; zrn1skipper does NOT
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1revealer", Vote: "accept", Salt: []byte("s1"), RevealedAtBlock: 90},
		{Verifier: "zrn1revealer2", Vote: "accept", Salt: []byte("s2"), RevealedAtBlock: 90},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)

	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Verify the skipper was slashed
	// Re-fetch params for assertion (params already declared above)
	params, _ = k.GetParams(ctx)
	var skipperSlashed bool
	for _, s := range sk.slashes {
		if s.Validator == "zrn1skipper" {
			skipperSlashed = true
			require.Equal(t, params.MissedRevealSlashBps, s.SlashBps,
				"missed reveal slash must use MissedRevealSlashBps param")
		}
	}
	require.True(t, skipperSlashed,
		"committed-but-not-revealed verifier must be slashed")
}

func TestSecurity_FactCreatedOnAcceptance(t *testing.T) {
	// Accepted claim must create a verified fact with correct metadata.
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "claim-fact-creation",
		FactContent: "Security test claim that should create a fact on acceptance",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-fact-create", "claim-fact-creation", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 850_000,
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Verify a fact was created
	var factFound bool
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == "claim-fact-creation" {
			factFound = true
			require.Equal(t, "mathematics", fact.Domain)
			require.Equal(t, "formal", fact.Category)
			require.Equal(t, uint64(850_000), fact.Confidence)
			require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, fact.Status)
			require.NotEmpty(t, fact.Id)
		}
		return false
	})
	require.True(t, factFound, "accepted claim must produce a verified fact")

	// Verify claim status is accepted
	updatedClaim, found := k.GetClaim(ctx, "claim-fact-creation")
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_ACCEPTED, updatedClaim.Status)
}

func TestSecurity_CommitAfterRevealPhase(t *testing.T) {
	// Attempting to commit when the round is in reveal phase must fail.
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("r-commit-late", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 50)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.StoreCommitmentInRound(ctx, "r-commit-late", &types.CommitEntry{
		Verifier:         "zrn1val1",
		CommitHash:       []byte("late_commitment_hash________________"),
		CommittedAtBlock: 100,
	})
	require.ErrorIs(t, err, types.ErrRoundNotInCommitPhase,
		"commit during reveal phase must return ErrRoundNotInCommitPhase")
}

func TestSecurity_RevealInCommitPhase(t *testing.T) {
	// Attempting to reveal when the round is in commit phase must fail.
	k, ctx := setupKnowledgeTest(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")

	round := makeRoundInPhase("r-reveal-early", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1", CommitHash: []byte("some_hash_______________________"), CommittedAtBlock: 55},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	err := k.StoreRevealInRound(ctx, "r-reveal-early", &types.RevealEntry{
		Verifier:        "zrn1val1",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}, 800_000)
	require.ErrorIs(t, err, types.ErrRoundNotInRevealPhase,
		"reveal during commit phase must return ErrRoundNotInRevealPhase")
}

func TestMalformed_SybilCostAnalysis(t *testing.T) {
	// N sybils voting malformed on a legitimate claim: verify they all get slashed
	// when the legitimate claim passes ACCEPT from honest validators.
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	// 5 honest validators with high stake
	for i := 0; i < 5; i++ {
		sk.addValidator(fmt.Sprintf("zrn1honest%d", i), 200_000, "bonded")
	}
	// 3 sybil validators with low stake
	for i := 0; i < 3; i++ {
		sk.addValidator(fmt.Sprintf("zrn1sybil%d", i), 10_000, "apprentice")
	}

	params, _ := k.GetParams(ctx)
	params.MinVerifiers = 8
	params.ConfidenceThreshold = 600_000 // 60%
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:          "claim-sybil-mal",
		FactContent: "Legitimate claim that sybils try to mark malformed",
		Domain:      "mathematics",
		Category:    "formal",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-sybil-mal", "claim-sybil-mal", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	for i := 0; i < 5; i++ {
		round.Commits = append(round.Commits, &types.CommitEntry{
			Verifier: fmt.Sprintf("zrn1honest%d", i), CommitHash: []byte(fmt.Sprintf("h%d", i)), CommittedAtBlock: 85,
		})
		round.Reveals = append(round.Reveals, &types.RevealEntry{
			Verifier: fmt.Sprintf("zrn1honest%d", i), Vote: "accept", Salt: []byte(fmt.Sprintf("s%d", i)), RevealedAtBlock: 90,
		})
	}
	for i := 0; i < 3; i++ {
		round.Commits = append(round.Commits, &types.CommitEntry{
			Verifier: fmt.Sprintf("zrn1sybil%d", i), CommitHash: []byte(fmt.Sprintf("sh%d", i)), CommittedAtBlock: 85,
		})
		round.Reveals = append(round.Reveals, &types.RevealEntry{
			Verifier: fmt.Sprintf("zrn1sybil%d", i), Vote: "malformed", Salt: []byte(fmt.Sprintf("ss%d", i)), RevealedAtBlock: 90,
		})
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result, err := k.AggregateVerificationResult(ctx, round)
	require.NoError(t, err)

	// Honest: 5 × 200k = 1,000k accept stake
	// Sybil: 3 × 10k = 30k malformed stake
	// Total: 1,030k
	// Accept ratio = 1,000k/1,030k ≈ 970,874 bps → above 60% threshold
	// Malformed ratio = 30k/1,030k ≈ 29,126 bps → well below threshold
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, result.Verdict,
		"sybil malformed votes must not override honest accept majority")

	// All sybils should be slashed for wrong vote
	slashMap := make(map[string]uint64)
	for _, s := range result.Slashes {
		slashMap[s.Verifier] = s.SlashBps
	}
	for i := 0; i < 3; i++ {
		sybilAddr := fmt.Sprintf("zrn1sybil%d", i)
		require.Contains(t, slashMap, sybilAddr,
			"sybil %s must be slashed for wrong malformed vote", sybilAddr)
		require.Equal(t, params.WrongVerificationSlashBps, slashMap[sybilAddr],
			"sybil slash must use WrongVerificationSlashBps")
	}

	// All honest validators should be rewarded
	rewardMap := make(map[string]uint64)
	for _, r := range result.Rewards {
		rewardMap[r.Verifier] = r.Amount
	}
	for i := 0; i < 5; i++ {
		honestAddr := fmt.Sprintf("zrn1honest%d", i)
		require.Contains(t, rewardMap, honestAddr,
			"honest validator %s must be rewarded", honestAddr)
		require.Equal(t, uint64(3_000_000), rewardMap[honestAddr],
			"honest validator reward must match VerificationReward")
	}
}
