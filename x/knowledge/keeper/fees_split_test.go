package keeper_test

// ─── Fee Markets Gap 3: Governance-Adjustable Revenue Split ──────────────────
//
// Tests that the revenue split (verifier / protocol / development / research) is
// read from Params at distribution time — not from compile-time constants —
// allowing governance to adjust incentives without a chain upgrade.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// TestSplit_DefaultValuesUnchanged verifies that the default param split
// produces exactly the same amounts as the previous hardcoded constants.
// This is the regression guard: Gap 3 must not change behaviour at defaults.
func TestSplit_DefaultValuesUnchanged(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("split-default"),
		FactContent: "Default split regression guard: verifier/protocol/dev/research unchanged",
		Domain:      "mathematics",
		Category:    "formal",
		Stake:       "1000000", // 1 ZRN
	})
	require.NoError(t, err)

	// Default split: 55%/22%/19.67%/~3.33% (unchanged from previous hardcoded constants).
	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt = call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt = call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(220_000), protocolAmt, "protocol: default 22%% of 1_000_000")
	require.Equal(t, int64(196_700), devAmt, "development: default 19.67%% of 1_000_000")
	require.Equal(t, int64(33_300), researchAmt, "research: default remainder ~3.33%%")
}

// TestSplit_GovernanceIncreasesVerifierShare verifies that raising VerifierPoolBps
// reduces the remainder going to research without changing protocol or dev amounts
// (which use their own BPS).
func TestSplit_GovernanceIncreasesVerifierShare(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Governance raises verifier share from 55% to 70%, keeps protocol/dev unchanged.
	// Research gets: 1_000_000 - 700_000 - 220_000 - 196_700 = -116_700 → invalid.
	// Use a valid split: verifier=60%, protocol=20%, dev=15%, research=5%.
	p, _ := k.GetParams(ctx)
	p.VerifierPoolBps = 600_000    // 60%
	p.ProtocolTreasuryBps = 200_000 // 20%
	p.DevelopmentFundBps = 150_000  // 15%
	// research = remainder = 50_000 (5%)
	_ = k.SetParams(ctx, p)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("split-gov-verifier"),
		FactContent: "Governance-adjusted split: higher verifier share for bootstrap incentive",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000", // 1 ZRN
	})
	require.NoError(t, err)

	// With stake=1_000_000:
	//   verifier pool = 600_000 (stays in module — no send)
	//   protocol = 200_000
	//   development = 150_000
	//   research = 1_000_000 - 600_000 - 200_000 - 150_000 = 50_000
	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt = call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt = call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(200_000), protocolAmt, "protocol: 20%% of 1_000_000 = 200_000")
	require.Equal(t, int64(150_000), devAmt, "development: 15%% of 1_000_000 = 150_000")
	require.Equal(t, int64(50_000), researchAmt, "research: remainder 5%% = 50_000")
}

// TestSplit_GovernanceMinimizesProtocolShare verifies routing when governance
// dramatically cuts the protocol share (e.g. during bootstrap: maximize verifiers).
func TestSplit_GovernanceMinimizesProtocolShare(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Protocol share cut to 1%, rest goes to verifiers and development.
	p, _ := k.GetParams(ctx)
	p.VerifierPoolBps = 750_000   // 75%
	p.ProtocolTreasuryBps = 10_000 // 1%
	p.DevelopmentFundBps = 200_000 // 20%
	// research = 40_000 (4%)
	_ = k.SetParams(ctx, p)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("split-min-protocol"),
		FactContent: "Governance-adjusted split: minimized protocol share for verifier bootstrap",
		Domain:      "computer_science",
		Category:    "formal",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt = call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt = call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(10_000), protocolAmt, "protocol: 1%% of 1_000_000 = 10_000")
	require.Equal(t, int64(200_000), devAmt, "development: 20%% of 1_000_000 = 200_000")
	require.Equal(t, int64(40_000), researchAmt, "research: 4%% remainder = 40_000")
}

// TestSplit_ProportionalAtLargeStake verifies the split scales correctly for
// a 10 ZRN stake using non-default governance params.
func TestSplit_ProportionalAtLargeStake(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	p, _ := k.GetParams(ctx)
	p.VerifierPoolBps = 500_000    // 50%
	p.ProtocolTreasuryBps = 250_000 // 25%
	p.DevelopmentFundBps = 200_000  // 20%
	// research = 50_000 (5%)
	_ = k.SetParams(ctx, p)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("split-large-stake"),
		FactContent: "Large stake proportional distribution test with governance-set split",
		Domain:      "economics",
		Category:    "empirical",
		Stake:       "10000000", // 10 ZRN
	})
	require.NoError(t, err)

	// 10 ZRN = 10_000_000 uzrn:
	//   verifier pool = 5_000_000 (stays in module)
	//   protocol      = 2_500_000
	//   development   = 2_000_000
	//   research      = 500_000
	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt = call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt = call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(2_500_000), protocolAmt, "protocol: 25%% of 10 ZRN")
	require.Equal(t, int64(2_000_000), devAmt, "development: 20%% of 10 ZRN")
	require.Equal(t, int64(500_000), researchAmt, "research: 5%% of 10 ZRN")
}

// TestSplit_Validate_SumEqualToOneMillion verifies that a split summing to
// exactly 1_000_000 is rejected (research fund would receive 0).
func TestSplit_Validate_SumEqualToOneMillion(t *testing.T) {
	p := types.DefaultParams()
	p.VerifierPoolBps = 550_000
	p.ProtocolTreasuryBps = 220_000
	p.DevelopmentFundBps = 230_000 // 550+220+230 = 1_000_000 → research = 0
	err := p.Validate()
	require.Error(t, err, "sum == 1_000_000 must be rejected (research fund = 0)")
	require.Contains(t, err.Error(), "must be < 1,000,000")
}

// TestSplit_Validate_SumExceedsOneMillion verifies that a split summing above
// 1_000_000 is rejected (research fund would overflow to negative).
func TestSplit_Validate_SumExceedsOneMillion(t *testing.T) {
	p := types.DefaultParams()
	p.VerifierPoolBps = 600_000
	p.ProtocolTreasuryBps = 300_000
	p.DevelopmentFundBps = 200_000 // 600+300+200 = 1_100_000 → overflow
	err := p.Validate()
	require.Error(t, err, "sum > 1_000_000 must be rejected")
	require.Contains(t, err.Error(), "must be < 1,000,000")
}

// TestSplit_Validate_IndividualFieldAtOneMillion verifies that a single field
// at 1_000_000 is rejected even before the sum check.
func TestSplit_Validate_IndividualFieldAtOneMillion(t *testing.T) {
	p := types.DefaultParams()
	p.VerifierPoolBps = 1_000_000 // 100% to verifiers — research, protocol, dev get 0
	err := p.Validate()
	require.Error(t, err, "VerifierPoolBps == 1_000_000 must be rejected")
	require.Contains(t, err.Error(), "verifier_pool_bps must be < 1,000,000")

	p = types.DefaultParams()
	p.ProtocolTreasuryBps = 1_000_000
	err = p.Validate()
	require.Error(t, err, "ProtocolTreasuryBps == 1_000_000 must be rejected")
	require.Contains(t, err.Error(), "protocol_treasury_bps must be < 1,000,000")

	p = types.DefaultParams()
	p.DevelopmentFundBps = 1_000_000
	err = p.Validate()
	require.Error(t, err, "DevelopmentFundBps == 1_000_000 must be rejected")
	require.Contains(t, err.Error(), "development_fund_bps must be < 1,000,000")
}

// TestSplit_Validate_DefaultParamsValid verifies that default params pass validation.
// This is the sanity check: shipping new defaults must not break genesis.
func TestSplit_Validate_DefaultParamsValid(t *testing.T) {
	p := types.DefaultParams()
	err := p.Validate()
	require.NoError(t, err, "default params must pass validation (550_000 + 220_000 + 196_700 < 1_000_000)")
}

// TestSplit_Validate_MaximumLegalSplit verifies a split that is just under the
// 1_000_000 limit is valid.
func TestSplit_Validate_MaximumLegalSplit(t *testing.T) {
	p := types.DefaultParams()
	// 999_999 total for three fields → 1 uzrn goes to research (valid).
	p.VerifierPoolBps = 700_000
	p.ProtocolTreasuryBps = 200_000
	p.DevelopmentFundBps = 99_999 // sum = 999_999, research = 1
	err := p.Validate()
	require.NoError(t, err, "sum of 999_999 is valid (research receives 1 BPS)")
}

// TestSplit_RoundCompletionUsesGovernanceSplit verifies that verifier payout
// on round completion reflects the governance-configured pool share, not the
// old hardcoded 55%.
func TestSplit_RoundCompletionUsesGovernanceSplit(t *testing.T) {
	k, ctx, bk, sk := setupKnowledgeTestFull(t)

	// Set verifier share to 70% via governance.
	p, _ := k.GetParams(ctx)
	p.VerifierPoolBps = 700_000    // 70% to verifiers
	p.ProtocolTreasuryBps = 150_000 // 15%
	p.DevelopmentFundBps = 100_000  // 10%
	// research = 50_000 (5%)
	_ = k.SetParams(ctx, p)

	verifier1 := makeValidBech32Addr("verifier-gov-split")
	sk.addValidator(verifier1, 100_000, "bonded")

	claim := &types.Claim{
		Id:          "claim-gov-split",
		FactContent: "Claim whose round completion pays 70%% of fee to verifier",
		Domain:      "biology",
		Category:    "empirical",
		Submitter:   "zrn1sub",
		Stake:       "1000000", // 1 ZRN → 70% pool = 700_000 uzrn
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := makeRoundInPhase("r-gov-split", "claim-gov-split", types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, 80)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	initialSendCount := len(bk.sendCalls)
	result := &keeper.VerificationResult{
		Verdict:    types.Verdict_VERDICT_ACCEPT,
		Confidence: 900_000,
		Rewards:    []keeper.VerifierReward{{Verifier: verifier1, Amount: 0}},
	}
	require.NoError(t, k.CompleteRound(ctx, round, result))

	// Verifier should receive 70% of 1_000_000 = 700_000 uzrn.
	var verifierReceivedAmt int64
	for _, call := range bk.sendCalls[initialSendCount:] {
		if call.from == "knowledge" {
			verifierReceivedAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(700_000), verifierReceivedAmt,
		"verifier should receive 70%% of stake (700_000 uzrn) per governance-configured split")
	_ = sk
}

// ─── Fee Markets Gap 4: Fee Burn ─────────────────────────────────────────────
//
// Tests that when BurnBps > 0, distributeReviewFee burns the configured fraction
// before distributing the remainder. BurnBps=0 (default) must not burn anything —
// no behaviour change until governance enables it.

// TestBurn_DefaultZeroNoBurn verifies that BurnBps=0 (default) does not trigger
// any BurnCoins call. Existing behaviour is fully preserved.
func TestBurn_DefaultZeroNoBurn(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Default params: BurnBps = 0.
	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("burn-default-zero"),
		FactContent: "Default burn=0: no coins burned, existing split unchanged",
		Domain:      "mathematics",
		Category:    "formal",
		Stake:       "1000000",
	})
	require.NoError(t, err)

	// No burn should have occurred.
	require.True(t, bk.burned.IsZero(), "BurnBps=0 must not burn any coins; got %s", bk.burned)
}

// TestBurn_GovernanceEnablesBurn verifies that setting BurnBps=50_000 (5%) causes
// exactly 5% of the fee to be burned and the research remainder to shrink accordingly.
func TestBurn_GovernanceEnablesBurn(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Governance enables 5% burn. Remaining split: verifier=55%, protocol=22%, dev=19.67%, research=~(100-5-55-22-19.67)%=-1.67%
	// That's invalid. Use valid split: burn=5%, verifier=50%, protocol=20%, dev=15% → research=10%.
	p, _ := k.GetParams(ctx)
	p.BurnBps = 50_000      // 5%
	p.VerifierPoolBps = 500_000 // 50%
	p.ProtocolTreasuryBps = 200_000 // 20%
	p.DevelopmentFundBps = 150_000  // 15%
	// research = 1_000_000 - 50_000 - 500_000 - 200_000 - 150_000 = 100_000 (10%)
	_ = k.SetParams(ctx, p)

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("burn-governance-5pct"),
		FactContent: "Governance burn 5%: EIP-1559 analogue applied to knowledge fee market",
		Domain:      "physics",
		Category:    "empirical",
		Stake:       "1000000", // 1 ZRN = 1_000_000 uzrn
	})
	require.NoError(t, err)

	// burn = 50_000 uzrn (5% of 1_000_000)
	burnedAmt := bk.burned.AmountOf("uzrn").Int64()
	require.Equal(t, int64(50_000), burnedAmt, "5%% of 1_000_000 must be burned")

	// protocol = 200_000, dev = 150_000, research = remainder
	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt = call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt = call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt = call.coins.AmountOf("uzrn").Int64()
		}
	}
	require.Equal(t, int64(200_000), protocolAmt, "protocol: 20%% of 1_000_000")
	require.Equal(t, int64(150_000), devAmt, "development: 15%% of 1_000_000")
	require.Equal(t, int64(100_000), researchAmt, "research: remainder 10%% after burn")
}

// TestBurn_TotalConserved verifies that burn + verifierPool + protocol + dev + research
// sums to exactly feeAmount — no coins created or destroyed beyond the burn intent.
func TestBurn_TotalConserved(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// burn=3%, verifier=55%, protocol=20%, dev=15% → research=7%.
	p, _ := k.GetParams(ctx)
	p.BurnBps = 30_000       // 3%
	p.VerifierPoolBps = 550_000
	p.ProtocolTreasuryBps = 200_000
	p.DevelopmentFundBps = 150_000
	// sum check: 30_000+550_000+200_000+150_000 = 930_000 < 1_000_000 ✓
	_ = k.SetParams(ctx, p)

	const feeAmount = int64(2_500_000) // 2.5 ZRN — non-round to expose rounding issues

	_, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   makeValidBech32Addr("burn-conservation"),
		FactContent: "Burn conservation: all coins accounted for across burn+split+research",
		Domain:      "biology",
		Category:    "empirical",
		Stake:       "2500000",
	})
	require.NoError(t, err)

	burnedAmt := bk.burned.AmountOf("uzrn").Int64()

	var protocolAmt, devAmt, researchAmt int64
	for _, call := range bk.sendCalls {
		switch call.to {
		case "protocol_treasury":
			protocolAmt += call.coins.AmountOf("uzrn").Int64()
		case "development_fund":
			devAmt += call.coins.AmountOf("uzrn").Int64()
		case "research_fund":
			researchAmt += call.coins.AmountOf("uzrn").Int64()
		}
	}

	// verifierPool stays in module (no send), so total sent+burned = feeAmount.
	// verifierPool = 55% of 2_500_000 = 1_375_000
	verifierPool := int64(2_500_000) * 550_000 / 1_000_000

	total := burnedAmt + verifierPool + protocolAmt + devAmt + researchAmt
	require.Equal(t, feeAmount, total,
		"burn(%d)+verifier(%d)+protocol(%d)+dev(%d)+research(%d) must equal fee(%d)",
		burnedAmt, verifierPool, protocolAmt, devAmt, researchAmt, feeAmount)
}

// TestBurn_ValidationRejectsOverflow verifies that Params.Validate() rejects
// a BurnBps value that would push the total sum ≥ 1_000_000 (leaving no remainder
// for the research fund).
func TestBurn_ValidationRejectsOverflow(t *testing.T) {
	p := types.DefaultParams()
	// Default split: verifier=55%, protocol=22%, dev=19.67%, research=~3.33%.
	// Set burn=4% → total = 4+55+22+19.67 = 100.67% > 100% → invalid.
	p.BurnBps = 40_000 // 4%
	// sum: 40_000 + 550_000 + 220_000 + 196_700 = 1_006_700 ≥ 1_000_000
	err := p.Validate()
	require.Error(t, err, "BurnBps that overflows the 1_000_000 invariant must be rejected")
	require.Contains(t, err.Error(), "burn_bps")
}

// TestBurn_ValidationAcceptsValidBurnBps verifies that a valid BurnBps (within invariant)
// is accepted by Params.Validate().
func TestBurn_ValidationAcceptsValidBurnBps(t *testing.T) {
	p := types.DefaultParams()
	// Reduce verifier share to make room for burn.
	p.BurnBps = 30_000       // 3%
	p.VerifierPoolBps = 520_000  // 52%  (was 55%)
	p.ProtocolTreasuryBps = 220_000
	p.DevelopmentFundBps = 196_700
	// sum: 30_000+520_000+220_000+196_700 = 966_700 < 1_000_000 ✓
	require.NoError(t, p.Validate())
}

// TestBurn_MaxLegal verifies the boundary: sum == 999_999 is legal (1 uzrn goes to research).
func TestBurn_MaxLegalBurnBps(t *testing.T) {
	p := types.DefaultParams()
	// burn=100_000, verifier=400_000, protocol=200_000, dev=299_999 → sum=999_999 → research=1
	p.BurnBps = 100_000
	p.VerifierPoolBps = 400_000
	p.ProtocolTreasuryBps = 200_000
	p.DevelopmentFundBps = 299_999
	// sum: 100_000+400_000+200_000+299_999 = 999_999 < 1_000_000 ✓
	require.NoError(t, p.Validate(), "sum=999_999 is the maximum legal configuration")
}

// TestBurn_ExactlyOneMillion verifies the boundary: sum == 1_000_000 is rejected
// (research fund would receive nothing).
func TestBurn_ExactlyOneMillion(t *testing.T) {
	p := types.DefaultParams()
	p.BurnBps = 100_000
	p.VerifierPoolBps = 400_000
	p.ProtocolTreasuryBps = 200_000
	p.DevelopmentFundBps = 300_000
	// sum: 100_000+400_000+200_000+300_000 = 1_000_000 — invalid
	err := p.Validate()
	require.Error(t, err, "sum=1_000_000 must be rejected")
}
