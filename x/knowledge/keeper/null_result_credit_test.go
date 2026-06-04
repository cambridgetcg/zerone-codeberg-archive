package keeper_test

// Scientific Method Gap 5: Null Result Credit — tests for maybeAwardNullResultCredit.
//
// Peirce's self-correcting community: submitters who pre-declare what would
// falsify their claim and then get falsified receive a small credit. This
// corrects publication bias — "testable and wrong" becomes better than
// "untestable". Default disabled (NullResultCreditUzrn=0).

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// TestNullResultCredit_AwardedWhenConditionsDeclared verifies that a credit is
// minted and sent when the claim has RefutationConditions and the param > 0.
func TestNullResultCredit_AwardedWhenConditionsDeclared(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Enable the credit.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 500
	require.NoError(t, k.SetParams(ctx, params))

	submitter := makeValidBech32Addr("submitter-rc-1")

	claim := &types.Claim{
		Id:                   "claim-rc-001",
		Submitter:            submitter,
		RefutationConditions: []string{"if temp > 100 then false"},
	}

	// Snapshot minted before call.
	mintedBefore := bk.minted.AmountOf("uzrn").Int64()
	sendsBefore := len(bk.sendCalls)

	k.RecordNullResultCreditForTest(ctx, claim)

	mintedDelta := bk.minted.AmountOf("uzrn").Int64() - mintedBefore
	require.Equal(t, int64(500), mintedDelta,
		"500 uzrn should have been minted for the null result credit")

	require.Equal(t, sendsBefore+1, len(bk.sendCalls), "one send call expected")
	require.Equal(t, types.ModuleName, bk.sendCalls[len(bk.sendCalls)-1].from)
	require.Equal(t, submitter, bk.sendCalls[len(bk.sendCalls)-1].to)
}

// TestNullResultCredit_NotAwardedWithoutConditions verifies that no credit is
// minted when the claim has no RefutationConditions.
func TestNullResultCredit_NotAwardedWithoutConditions(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 500
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:                   "claim-rc-002",
		Submitter:            makeValidBech32Addr("submitter-rc-2"),
		RefutationConditions: nil, // no pre-declared falsifiability
	}

	mintedBefore := bk.minted.AmountOf("uzrn").Int64()
	sendsBefore := len(bk.sendCalls)

	k.RecordNullResultCreditForTest(ctx, claim)

	require.Equal(t, mintedBefore, bk.minted.AmountOf("uzrn").Int64(),
		"no mint expected when no RefutationConditions")
	require.Equal(t, sendsBefore, len(bk.sendCalls), "no send expected")
}

// TestNullResultCredit_DisabledWhenParamZero verifies that no credit is minted
// when NullResultCreditUzrn = 0 (the default).
func TestNullResultCredit_DisabledWhenParamZero(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Default params have NullResultCreditUzrn = 0 — don't override.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), params.NullResultCreditUzrn, "default should be 0")

	claim := &types.Claim{
		Id:                   "claim-rc-003",
		Submitter:            makeValidBech32Addr("submitter-rc-3"),
		RefutationConditions: []string{"if X then not Y"},
	}

	mintedBefore := bk.minted.AmountOf("uzrn").Int64()
	sendsBefore := len(bk.sendCalls)

	k.RecordNullResultCreditForTest(ctx, claim)

	require.Equal(t, mintedBefore, bk.minted.AmountOf("uzrn").Int64(),
		"no mint expected when param is 0")
	require.Equal(t, sendsBefore, len(bk.sendCalls), "no send expected")
}

// TestNullResultCredit_NilClaimNoOp verifies nil claim is a safe no-op.
func TestNullResultCredit_NilClaimNoOp(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 500
	require.NoError(t, k.SetParams(ctx, params))

	mintedBefore := bk.minted.AmountOf("uzrn").Int64()

	k.RecordNullResultCreditForTest(ctx, nil)

	require.Equal(t, mintedBefore, bk.minted.AmountOf("uzrn").Int64(),
		"no mint expected for nil claim")
}

// TestNullResultCredit_MultipleConditions verifies that the number of
// RefutationConditions does not affect the amount — it's a flat credit per rejection.
func TestNullResultCredit_MultipleConditions(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.NullResultCreditUzrn = 1000
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:        "claim-rc-005",
		Submitter: makeValidBech32Addr("submitter-rc-5"),
		RefutationConditions: []string{
			"condition A",
			"condition B",
			"condition C",
		},
	}

	mintedBefore := bk.minted.AmountOf("uzrn").Int64()

	k.RecordNullResultCreditForTest(ctx, claim)

	// Flat credit regardless of condition count.
	mintedDelta := bk.minted.AmountOf("uzrn").Int64() - mintedBefore
	require.Equal(t, int64(1000), mintedDelta,
		"flat credit should equal NullResultCreditUzrn regardless of condition count")
}
