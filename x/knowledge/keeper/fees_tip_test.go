package keeper_test

// ─── Fee Markets Gap 2: Priority Tip Pool ─────────────────────────────────────
//
// Tests for the priority tip mechanism: a fraction of the verifier pool that
// is routed to early committers (committed before commit-phase midpoint).
//
// Key invariants:
//   - PriorityTipBps=0 (default) → identical to pre-Gap-2 distribution.
//   - Feature is disabled when MaxPriorityTipBps==0; any non-zero request is rejected.
//   - Submissions with priority_tip_bps > MaxPriorityTipBps are rejected.
//   - tipPool = poolAmount × PriorityTipBps / 1_000_000.
//   - Early correct verifiers (committed ≤ midpoint) share tipPool equally.
//   - If no early correct verifiers: tipPool merges into basePool.
//   - basePool is shared equally among ALL correct verifiers (same as before).
//   - MaxPriorityTipBps > 500_000 fails Params.Validate().

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// TestPriorityTip_DisabledByDefault verifies that non-zero priority_tip_bps is
// rejected when MaxPriorityTipBps == 0 (the default — feature off).
func TestPriorityTip_DisabledByDefault(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Default params: MaxPriorityTipBps = 0 (disabled)
	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      makeValidBech32Addr("tip-submitter-a"),
		FactContent:    "Priority tip test: disabled by default",
		Domain:         "mathematics",
		Category:       "formal",
		Stake:          "1000000",
		PriorityTipBps: 50_000, // 5% — should be rejected
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrPriorityTipExceedsMax)
}

// TestPriorityTip_ExceedsMax verifies that priority_tip_bps > MaxPriorityTipBps is rejected.
func TestPriorityTip_ExceedsMax(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 100_000 // governance allows up to 10%
	require.NoError(t, k.SetParams(ctx, p))

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      makeValidBech32Addr("tip-exceeds-max"),
		FactContent:    "Priority tip test: exceeds governance maximum",
		Domain:         "physics",
		Category:       "empirical",
		Stake:          "1000000",
		PriorityTipBps: 150_000, // 15% — exceeds 10% cap
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrPriorityTipExceedsMax)
}

// TestPriorityTip_ZeroBps_IdenticalToBaseline verifies that priority_tip_bps=0
// produces exactly the same verifier distribution as before Gap 2.
// Pool (default 55%) is shared equally among all correct verifiers.
func TestPriorityTip_ZeroBps_IdenticalToBaseline(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// round.PriorityTipBps = 0 (default) — standard distribution
	claim := &types.Claim{
		Id:          "claim-baseline",
		FactContent: "Baseline: no priority tip",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// startBlock=100, CommitDeadline=300, midpoint=200
	addrA := makeValidBech32Addr("verifier-a-1234")
	addrB := makeValidBech32Addr("verifier-b-5678")

	round := makeRoundInPhase("r-baseline", "claim-baseline", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.PriorityTipBps = 0
	// Both verifiers commit before midpoint — irrelevant when tip is off
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA, CommitHash: []byte("ha"), CommittedAtBlock: 150},
		{Verifier: addrB, CommitHash: []byte("hb"), CommittedAtBlock: 250},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA, Amount: 0},
			{Verifier: addrB, Amount: 0},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// pool = 55% of 1_000_000 = 550_000
	// With PriorityTipBps=0: tipPool=0, basePool=550_000
	// Each verifier gets 550_000 / 2 = 275_000 (+ dust to first)
	addrASends := sumSentToAddr(bk, addrA)
	addrBSends := sumSentToAddr(bk, addrB)
	total := addrASends + addrBSends
	require.Equal(t, uint64(550_000), total, "total pool distributed must equal verifier pool")
	// Equal split (A gets dust = 0 since 550_000/2=275_000 exact)
	require.Equal(t, uint64(275_000), addrASends, "verifier A: equal base share")
	require.Equal(t, uint64(275_000), addrBSends, "verifier B: equal base share")
}

// TestPriorityTip_EarlyCommitterGetsExtra verifies that when PriorityTipBps > 0,
// a verifier who committed before the midpoint receives the tip on top of their base.
func TestPriorityTip_EarlyCommitterGetsExtra(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Enable priority tip up to 20%
	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 200_000
	require.NoError(t, k.SetParams(ctx, p))

	claim := &types.Claim{
		Id:          "claim-early",
		FactContent: "Early committer gets priority tip",
		Domain:      "physics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// startBlock=100, CommitDeadline=300, midpoint=200
	addrEarly := makeValidBech32Addr("early-verifier-xy")
	addrLate := makeValidBech32Addr("late-verifier-z99")

	round := makeRoundInPhase("r-early", "claim-early", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.PriorityTipBps = 100_000 // 10% tip
	round.Commits = []*types.CommitEntry{
		{Verifier: addrEarly, CommitHash: []byte("he"), CommittedAtBlock: 150}, // before midpoint 200 → early
		{Verifier: addrLate, CommitHash: []byte("hl"), CommittedAtBlock: 250},  // after midpoint → late
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrEarly, Amount: 0},
			{Verifier: addrLate, Amount: 0},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// pool = 55% of 1_000_000 = 550_000
	// tipPool = 550_000 × 100_000 / 1_000_000 = 55_000
	// basePool = 550_000 - 55_000 = 495_000
	// earlyRewards: [addrEarly] → tipPool / 1 = 55_000
	// basePool / 2 = 247_500 each
	// addrEarly total = 55_000 (tip) + 247_500 (base) = 302_500
	// addrLate total  = 247_500 (base only)
	earlyTotal := sumSentToAddr(bk, addrEarly)
	lateTotal := sumSentToAddr(bk, addrLate)
	require.Equal(t, uint64(302_500), earlyTotal, "early committer: tip + base share")
	require.Equal(t, uint64(247_500), lateTotal, "late committer: base share only")
	require.Equal(t, uint64(550_000), earlyTotal+lateTotal, "total distributed equals verifier pool")
}

// TestPriorityTip_NoEarlyCommitters_TipMergesIntoBase verifies that when all
// committers missed the early window, the tip merges back into basePool.
func TestPriorityTip_NoEarlyCommitters_TipMergesIntoBase(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 200_000
	require.NoError(t, k.SetParams(ctx, p))

	claim := &types.Claim{
		Id:          "claim-late-all",
		FactContent: "All committers late: tip merges into base pool",
		Domain:      "computer_science",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	addrA := makeValidBech32Addr("late-only-verifier-1")
	addrB := makeValidBech32Addr("late-only-verifier-2")

	// startBlock=100, CommitDeadline=300, midpoint=200
	round := makeRoundInPhase("r-late-all", "claim-late-all", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.PriorityTipBps = 100_000 // 10% tip, but no early committers
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA, CommitHash: []byte("ha"), CommittedAtBlock: 210}, // after midpoint
		{Verifier: addrB, CommitHash: []byte("hb"), CommittedAtBlock: 290}, // after midpoint
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA, Amount: 0},
			{Verifier: addrB, Amount: 0},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// No early committers → tipPool (55_000) merges into basePool (495_000 + 55_000 = 550_000)
	// Each verifier gets 550_000 / 2 = 275_000
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, uint64(275_000), aTotal, "all-late A: full pool / 2")
	require.Equal(t, uint64(275_000), bTotal, "all-late B: full pool / 2")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "total distributed equals verifier pool")
}

// TestPriorityTip_AllEarlyCommitters_FullTipShared verifies that when all correct
// verifiers committed early, all receive both tip and base shares.
func TestPriorityTip_AllEarlyCommitters_FullTipShared(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 200_000
	require.NoError(t, k.SetParams(ctx, p))

	claim := &types.Claim{
		Id:          "claim-all-early",
		FactContent: "All verifiers early: tip shared among all",
		Domain:      "general",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	addrA := makeValidBech32Addr("early-all-verifier-1")
	addrB := makeValidBech32Addr("early-all-verifier-2")

	round := makeRoundInPhase("r-all-early", "claim-all-early", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.PriorityTipBps = 100_000 // 10%
	round.Commits = []*types.CommitEntry{
		{Verifier: addrA, CommitHash: []byte("ha"), CommittedAtBlock: 130}, // early
		{Verifier: addrB, CommitHash: []byte("hb"), CommittedAtBlock: 180}, // early
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrA, Amount: 0},
			{Verifier: addrB, Amount: 0},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// pool = 550_000, tipPool = 55_000, basePool = 495_000
	// Both early → tip split: 55_000 / 2 = 27_500 each
	// Base split: 495_000 / 2 = 247_500 each
	// Each verifier: 27_500 + 247_500 = 275_000
	aTotal := sumSentToAddr(bk, addrA)
	bTotal := sumSentToAddr(bk, addrB)
	require.Equal(t, uint64(275_000), aTotal, "all-early A: tip/2 + base/2")
	require.Equal(t, uint64(275_000), bTotal, "all-early B: tip/2 + base/2")
	require.Equal(t, uint64(550_000), aTotal+bTotal, "total distributed equals verifier pool")
}

// TestPriorityTip_MaxValidation verifies that Params.Validate() rejects
// MaxPriorityTipBps > 500_000 (no tip can exceed half the verifier pool).
func TestPriorityTip_MaxValidation(t *testing.T) {
	p := types.DefaultParams()
	p.MaxPriorityTipBps = 500_001 // just over 50%
	err := p.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_priority_tip_bps")
}

// TestPriorityTip_MaxValidation_BoundaryOK verifies that exactly 500_000 (50%) is accepted.
func TestPriorityTip_MaxValidation_BoundaryOK(t *testing.T) {
	p := types.DefaultParams()
	p.MaxPriorityTipBps = 500_000 // exactly 50% — at boundary
	require.NoError(t, p.Validate())
}

// TestPriorityTip_StoredInRound verifies that PriorityTipBps is persisted
// in the VerificationRound at creation time.
func TestPriorityTip_StoredInRound(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Enable priority tip
	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 100_000
	require.NoError(t, k.SetParams(ctx, p))

	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:      makeValidBech32Addr("tip-stored-submitter"),
		FactContent:    "Priority tip should be stored in the created round",
		Domain:         "mathematics",
		Category:       "formal",
		Stake:          "1000000",
		PriorityTipBps: 80_000, // 8%
	})
	require.NoError(t, err)

	// Look up the claim to find its round ID
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.NotEmpty(t, claim.VerificationRoundId)

	round, found := k.GetVerificationRound(ctx, claim.VerificationRoundId)
	require.True(t, found)
	require.Equal(t, uint64(80_000), round.PriorityTipBps, "PriorityTipBps must be stored in round")
}

// TestPriorityTip_ExactlyAtMidpoint treats midpoint as inclusive for early commits.
func TestPriorityTip_ExactlyAtMidpoint(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	p, _ := k.GetParams(ctx)
	p.MaxPriorityTipBps = 200_000
	require.NoError(t, k.SetParams(ctx, p))

	claim := &types.Claim{
		Id:          "claim-midpoint",
		FactContent: "Midpoint boundary: CommittedAtBlock == midpoint is early",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Stake:       "1000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// startBlock=100, CommitDeadline=300, midpoint=200
	addrMid := makeValidBech32Addr("midpoint-verifier-abc")
	addrPost := makeValidBech32Addr("post-mid-verifier-xyz")

	round := makeRoundInPhase("r-midpoint", "claim-midpoint", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 100)
	round.PriorityTipBps = 100_000
	round.Commits = []*types.CommitEntry{
		{Verifier: addrMid, CommitHash: []byte("hm"), CommittedAtBlock: 200},  // exactly at midpoint → early
		{Verifier: addrPost, CommitHash: []byte("hp"), CommittedAtBlock: 201}, // one block after → late
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards: []keeper.VerifierReward{
			{Verifier: addrMid, Amount: 0},
			{Verifier: addrPost, Amount: 0},
		},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// tipPool = 55_000 (10% of 550_000), all to addrMid (only early)
	// basePool = 495_000, split 247_500 each
	midTotal := sumSentToAddr(bk, addrMid)
	postTotal := sumSentToAddr(bk, addrPost)
	require.Equal(t, uint64(302_500), midTotal, "at-midpoint: tip + base/2")
	require.Equal(t, uint64(247_500), postTotal, "post-midpoint: base/2 only")
}

// ─── helper ─────────────────────────────────────────────────────────────────

// sumSentToAddr sums all bank sends from the knowledge module to the given bech32 address.
func sumSentToAddr(bk *trackingBankKeeper, addr string) uint64 {
	var total uint64
	for _, c := range bk.sendCalls {
		if c.to == addr {
			total += uint64(c.coins.AmountOf("uzrn").Int64())
		}
	}
	return total
}
