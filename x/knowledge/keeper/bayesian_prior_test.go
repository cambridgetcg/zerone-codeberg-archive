package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ────────────────────────────────────────────────────────────────────────────
// Bayesian Gap 1 — Prior at Submission
//
// A claim submitted with prior_probability_bps carries the submitter's stated
// probability BEFORE the verification panel weighs in. On ACCEPT the prior
// flows to Fact.stated_prior_bps. This enables update-magnitude analysis:
//   |ConfidenceBps − StatedPriorBps| = Bayesian update magnitude.
// ────────────────────────────────────────────────────────────────────────────

// submitterForBayesian returns a valid bech32 address with a seeded balance.
func submitterForBayesian(t *testing.T, bk *trackingBankKeeper) string {
	t.Helper()
	addr := makeValidBech32Addr("bayesian-submitter")
	bk.balances[addr] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))
	return addr
}

// setupBayesianDomain registers the "physics" domain needed for claim submission.
func setupBayesianDomain(t *testing.T, k keeper.Keeper, ctx sdk.Context) {
	t.Helper()
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))
}

// TestBayesianPrior_StoredOnSubmit verifies that a stated prior is persisted
// on the Claim as prior_probability_bps.
func TestBayesianPrior_StoredOnSubmit(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := submitterForBayesian(t, bk)
	setupBayesianDomain(t, k, ctx)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:           submitter,
		FactContent:         "Bayesian claim with prior",
		Domain:              "physics",
		Category:            "mechanics",
		Stake:               "1000000",
		PriorProbabilityBps: 600_000, // submitter thinks 60% likely
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(600_000), claim.PriorProbabilityBps,
		"stated prior must be stored on the Claim")
}

// TestBayesianPrior_NoPrior verifies that omitting prior stores 0 (not stated).
func TestBayesianPrior_NoPrior(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := submitterForBayesian(t, bk)
	setupBayesianDomain(t, k, ctx)

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:           submitter,
		FactContent:         "Claim without explicit prior",
		Domain:              "physics",
		Category:            "mechanics",
		Stake:               "1000000",
		PriorProbabilityBps: 0, // not stated
	})
	require.NoError(t, err)

	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, uint64(0), claim.PriorProbabilityBps,
		"unset prior must default to 0 (not stated)")
}

// TestBayesianPrior_BoundaryValues verifies valid boundary values [1, 999_999].
func TestBayesianPrior_BoundaryValues(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	setupBayesianDomain(t, k, ctx)

	cases := []struct {
		priorBps uint64
		content  string
		seed     string
	}{
		{1, "boundary prior at minimum one bps", "sub-bound-1"},
		{500_000, "boundary prior at fifty percent midpoint", "sub-bound-2"},
		{999_999, "boundary prior at maximum 999999 bps value", "sub-bound-3"},
	}
	for _, tc := range cases {
		addr := makeValidBech32Addr(tc.seed)
		bk.balances[addr] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 100_000_000))
		resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
			Submitter:           addr,
			FactContent:         tc.content,
			Domain:              "physics",
			Category:            "mechanics",
			Stake:               "1000000",
			PriorProbabilityBps: tc.priorBps,
		})
		require.NoError(t, err, "prior %d should be accepted", tc.priorBps)
		claim, found := k.GetClaim(ctx, resp.ClaimId)
		require.True(t, found)
		require.Equal(t, tc.priorBps, claim.PriorProbabilityBps)
	}
}

// TestBayesianPrior_InvalidRange verifies that dogmatic certainty (≥1_000_000)
// is rejected — a 100% prior means no evidence can update belief.
func TestBayesianPrior_InvalidRange(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := submitterForBayesian(t, bk)
	setupBayesianDomain(t, k, ctx)

	for _, badPrior := range []uint64{1_000_000, 1_500_000, ^uint64(0)} {
		_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
			Submitter:           submitter,
			FactContent:         "invalid prior claim content",
			Domain:              "physics",
			Category:            "mechanics",
			Stake:               "1000000",
			PriorProbabilityBps: badPrior,
		})
		require.Error(t, err, "prior %d should be rejected", badPrior)
		require.ErrorIs(t, err, types.ErrInvalidPriorProbability)
	}
}

// TestBayesianPrior_PropagatedToFact verifies that on ACCEPT verdict, the
// stated prior flows from Claim.prior_probability_bps to Fact.stated_prior_bps.
func TestBayesianPrior_PropagatedToFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	const prior = uint64(400_000) // submitter thinks 40% likely
	claimID := "claim-prior-propagate"
	content := "prior propagation test claim"

	claim := &types.Claim{
		Id:                  claimID,
		FactContent:         content,
		Domain:              "physics",
		Category:            "mechanics",
		Submitter:           makeValidBech32Addr("sub-propagate"),
		SubmittedAtBlock:    1,
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:               "1000uzrn",
		PriorProbabilityBps: prior,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-prior-propagate",
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Panel accepts with 85% confidence — much higher than the 40% prior
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 850_000,
	}
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(10), round, result))

	var found *types.Fact
	k.IterateFacts(ctx.WithBlockHeight(10), func(f *types.Fact) bool {
		if f.Content == content {
			fc := *f
			found = &fc
			return true
		}
		return false
	})
	require.NotNil(t, found, "accepted fact must exist in store")

	// Prior must propagate from Claim to Fact
	require.Equal(t, prior, found.StatedPriorBps,
		"Fact.stated_prior_bps must equal the claim's prior_probability_bps")
}

// TestBayesianPrior_NoPrior_FactZero verifies that when no prior was stated,
// Fact.stated_prior_bps is 0.
func TestBayesianPrior_NoPrior_FactZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claimID := "claim-no-prior"
	content := "no prior fact"

	claim := &types.Claim{
		Id:               claimID,
		FactContent:      content,
		Domain:           "physics",
		Category:         "mechanics",
		Submitter:        makeValidBech32Addr("sub-no-prior"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// PriorProbabilityBps: 0 (omitted = zero value)
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-no-prior",
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
	}
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(10), round, result))

	var found *types.Fact
	k.IterateFacts(ctx.WithBlockHeight(10), func(f *types.Fact) bool {
		if f.Content == content {
			fc := *f
			found = &fc
			return true
		}
		return false
	})
	require.NotNil(t, found)
	require.Equal(t, uint64(0), found.StatedPriorBps,
		"Fact.stated_prior_bps must be 0 when no prior was declared")
}

// TestBayesianPrior_RejectedClaimNoPropagation verifies that REJECT verdict
// does not create a Fact, so the prior remains only on the Claim record.
func TestBayesianPrior_RejectedClaimNoPropagation(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	const prior = uint64(700_000)
	claimID := "claim-prior-reject"
	content := "rejected prior claim"

	claim := &types.Claim{
		Id:                  claimID,
		FactContent:         content,
		Domain:              "physics",
		Category:            "mechanics",
		Submitter:           makeValidBech32Addr("sub-reject"),
		SubmittedAtBlock:    1,
		Status:              types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:               "1000uzrn",
		PriorProbabilityBps: prior,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-prior-reject",
		ClaimId:             claimID,
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_REJECT,
		Confidence: 800_000,
	}
	require.NoError(t, k.CompleteRound(ctx.WithBlockHeight(10), round, result))

	// No fact should exist for the rejected claim
	var factFound bool
	k.IterateFacts(ctx.WithBlockHeight(10), func(f *types.Fact) bool {
		if f.Content == content {
			factFound = true
			return true
		}
		return false
	})
	require.False(t, factFound, "REJECT verdict must not create a Fact")

	// Prior is still on the Claim
	storedClaim, claimFound := k.GetClaim(ctx, claimID)
	require.True(t, claimFound)
	require.Equal(t, prior, storedClaim.PriorProbabilityBps,
		"prior must remain on the Claim record after rejection")
}
