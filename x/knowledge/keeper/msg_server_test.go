package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── SubmitClaim ────────────────────────────────────────────────────────────

func TestMsgServer_SubmitClaim_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "The speed of light is approximately 3e8 m/s",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	// Verify claim was stored
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION, claim.Status)
	require.Equal(t, "physics", claim.Domain)
}

func TestMsgServer_SubmitClaim_TextTooShort(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "Too short",
		Domain:      "physics",
		Stake:       "1000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "too short")
}

func TestMsgServer_SubmitClaim_TextTooLong(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	longText := make([]byte, 10_001)
	for i := range longText {
		longText[i] = 'a'
	}

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: string(longText),
		Domain:      "physics",
		Stake:       "1000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "too long")
}

func TestMsgServer_SubmitClaim_InvalidDomain(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This is a valid length claim text for testing",
		Domain:      "nonexistent_domain",
		Stake:       "1000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist")
}

func TestMsgServer_SubmitClaim_StakeBelowMinimum(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has stake below the minimum",
		Domain:      "physics",
		Stake:       "100", // below MinReviewFee of 100000
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "below minimum")
}

func TestMsgServer_SubmitClaim_InvalidStake(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has an invalid stake amount",
		Domain:      "physics",
		Stake:       "not_a_number",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid review fee")
}

func TestMsgServer_SubmitClaim_ZeroStake(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has a zero stake amount",
		Domain:      "physics",
		Stake:       "0",
	})
	require.Error(t, err)
}

func TestMsgServer_SubmitClaim_DuplicateContent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	msg := &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "This content will be submitted twice for dedup test",
		Domain:      "physics",
		Stake:       "1000000",
	}

	_, err := ms.SubmitClaim(ctx, msg)
	require.NoError(t, err)

	// Second submission with same content should fail
	_, err = ms.SubmitClaim(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestMsgServer_SubmitClaim_CreatesVerificationRound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim should create a verification round",
		Domain:      "physics",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	// Verify a round was created for this claim
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)

	round, found := k.GetRoundByClaimID(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, round.Phase)
}

func TestMsgServer_SubmitClaim_EmptyDomainAllowed(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("submitter1"),
		FactContent: "This claim has no domain specified",
		Domain:      "",
		Stake:       "1000000",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)
}

// ─── SubmitClaim: RefutationConditions (P-07) ───────────────────────────────

func TestMsgServer_SubmitClaim_WithRefutationConditions(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	conditions := []string{
		"If a peer-reviewed experiment measures c > 3.1e8 m/s under controlled conditions",
		"If multiple independent labs reproduce the measurement with calibrated instruments",
	}
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("submitter1"),
		FactContent:          "The speed of light is approximately 3e8 m/s in vacuum",
		Domain:               "physics",
		Stake:                "1000000",
		RefutationConditions: conditions,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	// Verify refutation conditions are stored on the claim
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, conditions, claim.RefutationConditions)
}

func TestMsgServer_SubmitClaim_RefutationConditions_TooMany(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	conditions := make([]string, 11)
	for i := range conditions {
		conditions[i] = "some refutation condition"
	}
	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("submitter1"),
		FactContent:          "The speed of light is approximately 3e8 m/s in vacuum",
		Domain:               "physics",
		Stake:                "1000000",
		RefutationConditions: conditions,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max 10 entries")
}

func TestMsgServer_SubmitClaim_RefutationConditions_EmptyString(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("submitter1"),
		FactContent:          "The speed of light is approximately 3e8 m/s in vacuum",
		Domain:               "physics",
		Stake:                "1000000",
		RefutationConditions: []string{"valid condition", ""},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty string not allowed")
}

func TestMsgServer_SubmitClaim_RefutationConditions_TooLong(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	tooLong := make([]byte, 501)
	for i := range tooLong {
		tooLong[i] = 'x'
	}
	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:            makeValidBech32Addr("submitter1"),
		FactContent:          "The speed of light is approximately 3e8 m/s in vacuum",
		Domain:               "physics",
		Stake:                "1000000",
		RefutationConditions: []string{string(tooLong)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "too long")
}

// ─── SubmitCommitment ───────────────────────────────────────────────────────

func TestMsgServer_SubmitCommitment_Success(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Fund verifier with sufficient balance (100 ZRN = 100_000_000 uzrn)
	verifier := makeValidBech32Addr("validator1")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))

	round := makeRoundInPhase("commit-round-1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	hash := computeMsgServerCommitHash("commit-round-1", "accept", 0, []byte("salt123"))

	resp, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   verifier,
		RoundId:    "commit-round-1",
		CommitHash: hash,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify commitment was stored
	updated, found := k.GetVerificationRound(ctx, "commit-round-1")
	require.True(t, found)
	require.Len(t, updated.Commits, 1)
	require.Equal(t, verifier, updated.Commits[0].Verifier)
}

func TestMsgServer_SubmitCommitment_RoundNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   "zrn1validator1",
		RoundId:    "nonexistent",
		CommitHash: []byte("hash"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestMsgServer_SubmitCommitment_WrongPhase(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	round := makeRoundInPhase("reveal-round", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   "zrn1validator1",
		RoundId:    "reveal-round",
		CommitHash: []byte("hash"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in COMMIT phase")
}

func TestMsgServer_SubmitCommitment_PastDeadline(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Manually construct round with commit deadline in the past (94 < 100)
	round := &types.VerificationRound{
		Id:                  "expired-commit",
		ClaimId:             "c1",
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_COMMIT,
		StartedAtBlock:      90,
		CommitDeadline:      94,
		RevealDeadline:      298,
		AggregationDeadline: 348,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   "zrn1validator1",
		RoundId:    "expired-commit",
		CommitHash: []byte("hash"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ended")
}

func TestMsgServer_SubmitCommitment_DuplicateVerifier(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("validator1")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))

	round := makeRoundInPhase("dup-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	hash := computeMsgServerCommitHash("dup-commit", "accept", 0, []byte("salt1"))

	_, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   verifier,
		RoundId:    "dup-commit",
		CommitHash: hash,
	})
	require.NoError(t, err)

	_, err = ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   verifier,
		RoundId:    "dup-commit",
		CommitHash: hash,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate commitment")
}

func TestMsgServer_SubmitCommitment_InsufficientBalance(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Verifier with only 50 ZRN (below 100 ZRN minimum)
	verifier := makeValidBech32Addr("poorval1")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 50_000_000))

	round := makeRoundInPhase("bal-gate-round", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	hash := computeMsgServerCommitHash("bal-gate-round", "accept", 0, []byte("salt1"))

	_, err := ms.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		Verifier:   verifier,
		RoundId:    "bal-gate-round",
		CommitHash: hash,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "verifier does not meet minimum balance requirement")
}

// ─── SubmitReveal ───────────────────────────────────────────────────────────

func TestMsgServer_SubmitReveal_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	vote := "accept"
	salt := []byte("test-salt-reveal")
	hash := computeMsgServerCommitHash("reveal-1", vote, 0, salt)

	// Reveal deadline = 98+8=106 > 100
	round := makeRoundInPhase("reveal-1", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1validator1", CommitHash: hash, CommittedAtBlock: 99},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	resp, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-1",
		Vote:     vote,
		Salt:     salt,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	updated, found := k.GetVerificationRound(ctx, "reveal-1")
	require.True(t, found)
	require.Len(t, updated.Reveals, 1)
	require.Equal(t, "accept", updated.Reveals[0].Vote)
}

func TestMsgServer_SubmitReveal_HashMismatch(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	hash := computeMsgServerCommitHash("reveal-mismatch", "accept", 0, []byte("original-salt"))

	round := makeRoundInPhase("reveal-mismatch", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1validator1", CommitHash: hash, CommittedAtBlock: 99},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-mismatch",
		Vote:     "accept",
		Salt:     []byte("wrong-salt"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match")
}

func TestMsgServer_SubmitReveal_WrongPhase(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	round := makeRoundInPhase("reveal-wrong-phase", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-wrong-phase",
		Vote:     "accept",
		Salt:     []byte("salt"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in REVEAL phase")
}

func TestMsgServer_SubmitReveal_NoCommitment(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	round := makeRoundInPhase("reveal-no-commit", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-no-commit",
		Vote:     "accept",
		Salt:     []byte("salt"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no commitment")
}

func TestMsgServer_SubmitReveal_InvalidVote(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	vote := "maybe"
	salt := []byte("salt")
	hash := computeMsgServerCommitHash("reveal-invalid-vote", vote, 0, salt)

	round := makeRoundInPhase("reveal-invalid-vote", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1validator1", CommitHash: hash, CommittedAtBlock: 99},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-invalid-vote",
		Vote:     vote,
		Salt:     salt,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid vote")
}

func TestMsgServer_SubmitReveal_DuplicateReveal(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	vote := "accept"
	salt := []byte("salt-dup")
	hash := computeMsgServerCommitHash("reveal-dup", vote, 0, salt)

	round := makeRoundInPhase("reveal-dup", "c1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 98)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1validator1", CommitHash: hash, CommittedAtBlock: 99},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1validator1", Vote: vote, Salt: salt, RevealedAtBlock: 100},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	_, err := ms.SubmitReveal(ctx, &types.MsgSubmitReveal{
		Verifier: "zrn1validator1",
		RoundId:  "reveal-dup",
		Vote:     vote,
		Salt:     salt,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate reveal")
}

// ─── AddFact (authority-only) ───────────────────────────────────────────────

func TestMsgServer_AddFact_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.AddFact(ctx, &types.MsgAddFact{
		Authority:  "zrn1authority",
		Content:    "Governance-injected fact content for testing",
		Domain:     "mathematics",
		Category:   "formal",
		Confidence: 900_000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.FactId)

	fact, found := k.GetFact(ctx, resp.FactId)
	require.True(t, found)
	require.Equal(t, types.FactStatus_FACT_STATUS_VERIFIED, fact.Status)
	require.Equal(t, uint64(880_000), fact.Confidence, "900k input clamped to MaxConfidence (880,000)")
}

func TestMsgServer_AddFact_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.AddFact(ctx, &types.MsgAddFact{
		Authority: "zrn1notauthority",
		Content:   "Unauthorized governance fact injection",
		Domain:    "mathematics",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// ─── UpdateParams ───────────────────────────────────────────────────────────

func TestMsgServer_UpdateParams_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	newParams := types.DefaultParams()
	newParams.MinVerifiers = 5
	newParams.MaxVerifiers = 50

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "zrn1authority",
		Params:    &newParams,
	})
	require.NoError(t, err)

	got, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(5), got.MinVerifiers)
	require.Equal(t, uint64(50), got.MaxVerifiers)
}

func TestMsgServer_UpdateParams_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "zrn1notauthority",
		Params:    &params,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

func TestMsgServer_UpdateParams_NilParams(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "zrn1authority",
		Params:    nil,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestMsgServer_UpdateParams_InvalidParams(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	invalid := types.DefaultParams()
	invalid.MinVerifiers = 0

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "zrn1authority",
		Params:    &invalid,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

// ─── ChallengeFact ──────────────────────────────────────────────────────────

func TestMsgServer_ChallengeFact_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	makeTestFact(t, k, ctx, "challenge-target", "Fact to be challenged here", "physics", "empirical", "zrn1sub", 800_000)

	challenger := makeValidBech32Addr("challenger1")

	// Fact confidence 800k → risk-scaled stake = 11 ZRN × (1 + 0.8) = 19.8 ZRN.
	resp, err := ms.ChallengeFact(ctx, &types.MsgChallengeFact{
		Challenger: challenger,
		FactId:     "challenge-target",
		Stake:      "19800000",
		Reason:     "Evidence contradicts this claim",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.RoundId)

	fact, found := k.GetFact(ctx, "challenge-target")
	require.True(t, found)
	require.Equal(t, types.FactStatus_FACT_STATUS_CHALLENGED, fact.Status)
}

func TestMsgServer_ChallengeFact_BelowRiskScaledStake(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Low-confidence facts still require the full base stake under the
	// inverted-Popperian challenge economics: the discount only applies
	// once the community has invested confidence worth stress-testing.
	// Nothing to prove = pay full price.
	makeTestFact(t, k, ctx, "challenge-lowconf", "unproven claim", "physics", "empirical", "zrn1sub", 0)

	_, err := ms.ChallengeFact(ctx, &types.MsgChallengeFact{
		Challenger: makeValidBech32Addr("challenger1"),
		FactId:     "challenge-lowconf",
		Stake:      "5000000", // below the 11 ZRN base stake
		Reason:     "under-priced challenge attempt",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "below effective minimum")
}

func TestMsgServer_ChallengeFact_HighConfidenceCheaperToProbe(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Truth stands firm under challenge: a 90%-confidence fact is cheaper
	// to probe than a 0%-confidence fact. The same stake that fails on
	// an unproven claim succeeds on a highly-trusted one. Scaling=1.0 at
	// conf=0.9 gives an effective minimum of base × 0.1 = 1.1 ZRN.
	makeTestFact(t, k, ctx, "challenge-highconf", "highly-trusted claim", "physics", "empirical", "zrn1sub", 900_000)

	_, err := ms.ChallengeFact(ctx, &types.MsgChallengeFact{
		Challenger: makeValidBech32Addr("challenger2"),
		FactId:     "challenge-highconf",
		Stake:      "2000000", // 2 ZRN — clears the ~1.1 ZRN minimum
		Reason:     "stress-testing a confident claim",
	})
	require.NoError(t, err)
}

func TestMsgServer_ChallengeFact_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.ChallengeFact(ctx, &types.MsgChallengeFact{
		Challenger: makeValidBech32Addr("challenger1"),
		FactId:     "nonexistent",
		Stake:      "11000000",
		Reason:     "No such fact",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestMsgServer_ChallengeFact_NotChallengeable(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	fact := &types.Fact{
		Id:      "unchallengeable",
		Content: "Revoked fact cannot be challenged",
		Domain:  "physics",
		Status:  types.FactStatus_FACT_STATUS_REVOKED,
	}
	require.NoError(t, k.SetFact(ctx, fact))

	_, err := ms.ChallengeFact(ctx, &types.MsgChallengeFact{
		Challenger: makeValidBech32Addr("challenger1"),
		FactId:     "unchallengeable",
		Stake:      "11000000",
		Reason:     "Should fail",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in a challengeable state")
}

// ─── ProposeDomain ──────────────────────────────────────────────────────────

func TestMsgServer_ProposeDomain_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	proposer := makeValidBech32Addr("proposer1")

	resp, err := ms.ProposeDomain(ctx, &types.MsgProposeDomain{
		Proposer:    proposer,
		Name:        "astrobiology",
		Description: "Study of life in the universe",
		Stratum:     "empirical",
		Stake:       "1000000",
	})
	require.NoError(t, err)
	require.Equal(t, "astrobiology", resp.ProposalId)

	domain, found := k.GetDomain(ctx, "astrobiology")
	require.True(t, found)
	require.Equal(t, types.DomainStatus_DOMAIN_STATUS_PROPOSED, domain.Status)
	require.Equal(t, proposer, domain.Proposer)
}

func TestMsgServer_ProposeDomain_AlreadyExists(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.ProposeDomain(ctx, &types.MsgProposeDomain{
		Proposer:    makeValidBech32Addr("proposer1"),
		Name:        "physics",
		Description: "Duplicate",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

// ─── EndorseDomainProposal ──────────────────────────────────────────────────

func TestMsgServer_EndorseDomain_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:     "new_domain",
		Status:   types.DomainStatus_DOMAIN_STATUS_PROPOSED,
		Proposer: "zrn1proposer1",
	}))

	_, err := ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
		Endorser:   "zrn1endorser1",
		ProposalId: "new_domain",
	})
	require.NoError(t, err)

	domain, found := k.GetDomain(ctx, "new_domain")
	require.True(t, found)
	require.Contains(t, domain.Endorsers, "zrn1endorser1")
}

func TestMsgServer_EndorseDomain_AutoActivateAt3(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:     "activate_me",
		Status:   types.DomainStatus_DOMAIN_STATUS_PROPOSED,
		Proposer: "zrn1proposer1",
	}))

	for i := 1; i <= 3; i++ {
		_, err := ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
			Endorser:   makeValidatorAddr(i),
			ProposalId: "activate_me",
		})
		require.NoError(t, err)
	}

	domain, found := k.GetDomain(ctx, "activate_me")
	require.True(t, found)
	require.Equal(t, types.DomainStatus_DOMAIN_STATUS_ACTIVE, domain.Status)
	require.Len(t, domain.Endorsers, 3)
}

func TestMsgServer_EndorseDomain_DuplicateEndorser(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:     "dup_endorse",
		Status:   types.DomainStatus_DOMAIN_STATUS_PROPOSED,
		Proposer: "zrn1proposer1",
	}))

	_, err := ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
		Endorser:   "zrn1endorser1",
		ProposalId: "dup_endorse",
	})
	require.NoError(t, err)

	_, err = ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
		Endorser:   "zrn1endorser1",
		ProposalId: "dup_endorse",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already endorsed")
}

func TestMsgServer_EndorseDomain_NotProposed(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
		Endorser:   "zrn1endorser1",
		ProposalId: "physics",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in PROPOSED status")
}

func TestMsgServer_EndorseDomain_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.EndorseDomainProposal(ctx, &types.MsgEndorseDomainProposal{
		Endorser:   "zrn1endorser1",
		ProposalId: "nonexistent_domain",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// ─── RegisterStratum ────────────────────────────────────────────────────────

func TestMsgServer_RegisterStratum_Authority(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.RegisterStratum(ctx, &types.MsgRegisterStratum{
		Authority:         "zrn1authority",
		Name:              "analytic",
		ConfidenceCeiling: 900_000,
	})
	require.NoError(t, err)
}

func TestMsgServer_RegisterStratum_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.RegisterStratum(ctx, &types.MsgRegisterStratum{
		Authority:         "zrn1nobody",
		Name:              "analytic",
		ConfidenceCeiling: 900_000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// ─── UpdateExtendedParams ───────────────────────────────────────────────────

func TestMsgServer_UpdateExtendedParams_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.UpdateExtendedParams(ctx, &types.MsgUpdateExtendedParams{
		Authority:  "zrn1authority",
		ParamsJson: `{"custom_field": true}`,
	})
	require.NoError(t, err)
}

func TestMsgServer_UpdateExtendedParams_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.UpdateExtendedParams(ctx, &types.MsgUpdateExtendedParams{
		Authority:  "zrn1nobody",
		ParamsJson: `{}`,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// ─── ClaimType Tests ────────────────────────────────────────────────────────

func TestMsgServer_SubmitClaim_DefaultType(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	// Submit without specifying claim_type (should default to ASSERTION)
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Untyped claim defaults to assertion type",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, types.ClaimType_CLAIM_TYPE_ASSERTION, claim.ClaimType)
}

func TestMsgServer_SubmitClaim_ExplicitType(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	testCases := []struct {
		name      string
		claimType types.ClaimType
		content   string
	}{
		{"assertion", types.ClaimType_CLAIM_TYPE_ASSERTION, "Water freezes at zero degrees Celsius"},
		{"relation", types.ClaimType_CLAIM_TYPE_RELATION, "Thermodynamics relates to statistical mechanics via entropy"},
		{"definition", types.ClaimType_CLAIM_TYPE_DEFINITION, "Entropy means the measure of disorder in a system"},
		{"constraint", types.ClaimType_CLAIM_TYPE_CONSTRAINT, "Energy must be conserved in all physical processes"},
		{"negation", types.ClaimType_CLAIM_TYPE_NEGATION, "Perpetual motion machines are NOT possible"},
		{"observation", types.ClaimType_CLAIM_TYPE_OBSERVATION, "BTC was observed at fifty thousand on 2026-01-01"},
	}

	for _, tc := range testCases {
		ctx = advanceBlocks(ctx, 51) // exceed default cooldown of 50 (R29-6)
		t.Run(tc.name, func(t *testing.T) {
			resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
				Submitter:   submitter,
				FactContent: tc.content,
				Domain:      "physics",
				Category:    "empirical",
				Stake:       "1000000",
				ClaimType:   tc.claimType,
			})
			require.NoError(t, err)

			claim, found := k.GetClaim(ctx, resp.ClaimId)
			require.True(t, found)
			require.Equal(t, tc.claimType, claim.ClaimType)
		})
	}
}

func TestMsgServer_CreateFactFromClaim_PropagatesType(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := makeValidBech32Addr("submitter1")

	// Submit a definition-typed claim
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   submitter,
		FactContent: "Entropy means the measure of disorder in a thermodynamic system",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000",
		ClaimType:   types.ClaimType_CLAIM_TYPE_DEFINITION,
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, types.ClaimType_CLAIM_TYPE_DEFINITION, claim.ClaimType)

	// Get the verification round
	round, found := k.GetVerificationRound(ctx, claim.VerificationRoundId)
	require.True(t, found)

	// Complete the round with ACCEPT verdict
	err = k.CompleteRound(ctx, round, &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 800_000,
	})
	require.NoError(t, err)

	// Find the fact created from this claim
	var createdFact *types.Fact
	k.IterateFacts(ctx, func(fact *types.Fact) bool {
		if fact.ClaimId == claim.Id {
			createdFact = fact
			return true
		}
		return false
	})
	require.NotNil(t, createdFact, "expected a fact to be created from the accepted claim")
	require.Equal(t, types.ClaimType_CLAIM_TYPE_DEFINITION, createdFact.ClaimType)
}
