package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ────────────────────────────────────────────────────────────────────────────
// Bayesian Gap 2 — Submitter Prior-Calibration Brier Scoring
//
// When a claim resolves to ACCEPT or REJECT and the submitter stated a prior
// (prior_probability_bps != 0), the chain scores the submitter's prediction
// quality via Brier scoring:
//
//	brierBps = (prior_bps − outcome_bps)² / 1_000_000
//
// outcome_bps = 1_000_000 for ACCEPT, 0 for REJECT.
//
// Lower Brier score = better calibration.
//
// Fields updated: AgentCalibration.SubmitterPriorCount (29),
// SubmitterBrierSumBps (30), SubmitterBrierScoreBps (31).
//
// This closes the loop opened by Bayesian Gap 1 (prior at submission):
// Gap 1 stored what the submitter believed; Gap 2 measures how right they were.
// ────────────────────────────────────────────────────────────────────────────

const bayesianPriorSubAddr = "cosmos1bayesian00000000000000000000000prioraddr"

// TestSubmitterPrior_NoPrior verifies that prior_probability_bps == 0
// (no prior stated) results in no scoring — backward compatible.
func TestSubmitterPrior_NoPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 0, true)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	// Either not found (never touched) or count == 0.
	if found && cal != nil {
		require.Zero(t, cal.SubmitterPriorCount,
			"no prior stated: SubmitterPriorCount must stay 0")
	}
}

// TestSubmitterPrior_EmptySubmitter verifies that an empty submitter address
// is silently skipped.
func TestSubmitterPrior_EmptySubmitter(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Must not panic.
	require.NotPanics(t, func() {
		k.RecordSubmitterPriorOutcomeForTest(ctx, "", 500_000, true)
	})
}

// TestSubmitterPrior_AcceptWithHighPrior verifies low Brier error when the
// submitter stated a high prior (90%) and the claim was ACCEPT'd.
//
// prior = 900_000, outcome = 1_000_000 (ACCEPT)
// diff = 900_000 - 1_000_000 = -100_000
// brier = 100_000² / 1_000_000 = 10_000 BPS (1%)
func TestSubmitterPrior_AcceptWithHighPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 900_000, true)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	require.True(t, found)
	require.Equal(t, uint64(1), cal.SubmitterPriorCount)

	expectedBrier := uint64(10_000) // (100_000)² / 1_000_000
	require.Equal(t, expectedBrier, cal.SubmitterBrierSumBps)
	require.Equal(t, expectedBrier, cal.SubmitterBrierScoreBps,
		"score = sum / count with count=1")
}

// TestSubmitterPrior_AcceptWithLowPrior verifies high Brier error when the
// submitter stated a low prior (10%) but the claim was ACCEPT'd.
//
// prior = 100_000, outcome = 1_000_000 (ACCEPT)
// diff = 100_000 - 1_000_000 = -900_000
// brier = 900_000² / 1_000_000 = 810_000 BPS (81%)
func TestSubmitterPrior_AcceptWithLowPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 100_000, true)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	require.True(t, found)
	require.Equal(t, uint64(1), cal.SubmitterPriorCount)

	expectedBrier := uint64(810_000) // (900_000)² / 1_000_000
	require.Equal(t, expectedBrier, cal.SubmitterBrierSumBps)
}

// TestSubmitterPrior_RejectWithLowPrior verifies low Brier error when the
// submitter stated a low prior (10%) and the claim was REJECT'd.
//
// prior = 100_000, outcome = 0 (REJECT)
// diff = 100_000 - 0 = 100_000
// brier = 100_000² / 1_000_000 = 10_000 BPS (1%)
func TestSubmitterPrior_RejectWithLowPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 100_000, false)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	require.True(t, found)
	require.Equal(t, uint64(1), cal.SubmitterPriorCount)

	expectedBrier := uint64(10_000) // (100_000)² / 1_000_000
	require.Equal(t, expectedBrier, cal.SubmitterBrierSumBps)
}

// TestSubmitterPrior_RejectWithHighPrior verifies high Brier error when the
// submitter stated a high prior (90%) but the claim was REJECT'd.
//
// prior = 900_000, outcome = 0 (REJECT)
// diff = 900_000 - 0 = 900_000
// brier = 900_000² / 1_000_000 = 810_000 BPS (81%)
func TestSubmitterPrior_RejectWithHighPrior(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 900_000, false)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	require.True(t, found)
	require.Equal(t, uint64(1), cal.SubmitterPriorCount)

	expectedBrier := uint64(810_000) // (900_000)² / 1_000_000
	require.Equal(t, expectedBrier, cal.SubmitterBrierSumBps)
}

// TestSubmitterPrior_Accumulates verifies that multiple scored events
// accumulate correctly: count increments, sum accumulates, score = sum/count.
//
// Event 1: prior=700_000, ACCEPT → brier=(300_000)²/1e6 = 90_000
// Event 2: prior=600_000, REJECT → brier=(600_000)²/1e6 = 360_000
// After 2: count=2, sum=450_000, score=225_000
func TestSubmitterPrior_Accumulates(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 700_000, true)
	k.RecordSubmitterPriorOutcomeForTest(ctx, bayesianPriorSubAddr, 600_000, false)

	cal, found := k.GetAgentCalibration(ctx, bayesianPriorSubAddr)
	require.True(t, found)
	require.Equal(t, uint64(2), cal.SubmitterPriorCount)

	// (300_000)² / 1_000_000 = 90_000
	// (600_000)² / 1_000_000 = 360_000
	expectedSum := uint64(90_000 + 360_000) // 450_000
	require.Equal(t, expectedSum, cal.SubmitterBrierSumBps)
	require.Equal(t, expectedSum/2, cal.SubmitterBrierScoreBps,
		"score = sum / count = 450_000 / 2 = 225_000")
}

// TestSubmitterPrior_WiredInCompleteRound verifies end-to-end that stating a
// prior on a claim causes SubmitterPriorCount to increment after the verdict.
func TestSubmitterPrior_WiredInCompleteRound(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sub := makeValidBech32Addr("bayesian-sub-wired")
	bk.balances[sub] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	require.NoError(t, k.SetDomain(ctx, &types.Domain{
		Name:   "physics",
		Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE,
	}))

	// Submit claim with stated prior.
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:           sub,
		FactContent:         "Bayesian gap-2 wiring test claim",
		Domain:              "physics",
		Category:            "mechanics",
		Stake:               "1000000",
		PriorProbabilityBps: 750_000, // 75% prior
	})
	require.NoError(t, err)
	claimID := resp.ClaimId

	claim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Equal(t, uint64(750_000), claim.PriorProbabilityBps)

	// Retrieve calibration *before* verdict — no prior scoring yet.
	calBefore, foundBefore := k.GetAgentCalibration(ctx, sub)
	var countBefore uint64
	if foundBefore && calBefore != nil {
		countBefore = calBefore.SubmitterPriorCount
	}

	// Simulate ACCEPT: RecordSubmitterPriorOutcome should increment count.
	k.RecordSubmitterPriorOutcomeForTest(ctx, sub, claim.PriorProbabilityBps, true)

	calAfter, found := k.GetAgentCalibration(ctx, sub)
	require.True(t, found)
	require.Equal(t, countBefore+1, calAfter.SubmitterPriorCount,
		"count must increment exactly once per scored verdict")

	// Verify Brier math: prior=750_000, outcome=1_000_000 (ACCEPT)
	// diff = -250_000; brier = (250_000)² / 1_000_000 = 62_500
	require.Equal(t, uint64(62_500), calAfter.SubmitterBrierSumBps)
}
