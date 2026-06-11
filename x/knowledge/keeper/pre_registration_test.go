package keeper_test

// Scientific Method Gap 3: Submission Bias / Pre-registration
//
// Pre-registration corrects p-hacking by anchoring the claim hypothesis
// on-chain BEFORE evidence collection. The temporal anchor is unforgeable.
//
// Key properties:
//   - MsgPreRegisterClaim: creates a PreRegistration anchor; optional bond
//   - MsgSubmitClaim.pre_registration_id: fulfills the pre-registration, returns bond,
//     applies review fee discount, sets Claim.pre_registration_id + pre_registered_at_block
//   - Expiry: expired unfulfilled pre-registrations forfeit bond to research_fund
//   - ProcessExpiredPreRegistrations: BeginBlocker-driven, reads expiry index
//   - ApplyPreRegistrationDiscount: discountedMin = min × (1M - bps) / 1M
//   - Fail-open: disabled params (0/"") → no restrictions

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// preRegAddr returns a deterministic bech32 address for pre-registration tests.
func preRegAddr(seed string) string {
	return makeValidBech32Addr("prereg-" + seed)
}

// ── Unit tests: ApplyPreRegistrationDiscount ─────────────────────────────────

// TestPreRegDiscount_Disabled verifies that 0 discount returns the original fee.
func TestPreRegDiscount_Disabled(t *testing.T) {
	result := keeper.ApplyPreRegistrationDiscount("1000000", 0)
	require.Equal(t, "1000000", result, "0 discount must return original fee")
}

// TestPreRegDiscount_10Percent verifies 10% discount reduces min fee correctly.
func TestPreRegDiscount_10Percent(t *testing.T) {
	// 1_000_000 × (1_000_000 - 100_000) / 1_000_000 = 900_000
	result := keeper.ApplyPreRegistrationDiscount("1000000", 100_000)
	require.Equal(t, "900000", result, "10%% discount of 1000000 = 900000")
}

// TestPreRegDiscount_50Percent verifies the max allowed discount.
func TestPreRegDiscount_50Percent(t *testing.T) {
	// 2_000_000 × (1_000_000 - 500_000) / 1_000_000 = 1_000_000
	result := keeper.ApplyPreRegistrationDiscount("2000000", 500_000)
	require.Equal(t, "1000000", result, "50%% discount of 2000000 = 1000000")
}

// TestPreRegDiscount_InvalidFee verifies that unparseable fees are returned unchanged.
func TestPreRegDiscount_InvalidFee(t *testing.T) {
	result := keeper.ApplyPreRegistrationDiscount("not-a-number", 100_000)
	require.Equal(t, "not-a-number", result, "unparseable fee must be returned unchanged")
}

// TestPreRegDiscount_ZeroFee verifies that zero/non-positive fees are returned unchanged.
func TestPreRegDiscount_ZeroFee(t *testing.T) {
	result := keeper.ApplyPreRegistrationDiscount("0", 100_000)
	require.Equal(t, "0", result, "zero fee must be returned unchanged")
}

// ── Unit tests: state CRUD ────────────────────────────────────────────────────

// TestPreRegistration_SetGet verifies basic storage round-trip.
func TestPreRegistration_SetGet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pr := &types.PreRegistration{
		Id:             "prereg-abc12345",
		Submitter:      preRegAddr("alice"),
		ClaimIntent:    "The speed of light is constant in vacuum",
		MethodologyId:  "M-EMPIRICAL",
		Domain:         "physics",
		ExpiryBlock:    1000,
		BondAmount:     "5000",
		CreatedAtBlock: 50,
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))

	got, found := k.GetPreRegistration(ctx, "prereg-abc12345")
	require.True(t, found)
	require.Equal(t, pr.Id, got.Id)
	require.Equal(t, pr.Submitter, got.Submitter)
	require.Equal(t, pr.ClaimIntent, got.ClaimIntent)
	require.Equal(t, pr.CreatedAtBlock, got.CreatedAtBlock)
}

// TestPreRegistration_NotFound verifies missing records return false.
func TestPreRegistration_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	_, found := k.GetPreRegistration(ctx, "does-not-exist")
	require.False(t, found)
}

// ── Unit tests: FulfillPreRegistration ───────────────────────────────────────

// TestPreRegistration_Fulfill_Success verifies that FulfillPreRegistration
// marks the record as fulfilled and returns the correct createdAtBlock.
func TestPreRegistration_Fulfill_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	pr := &types.PreRegistration{
		Id:             "prereg-fulfill-ok",
		Submitter:      preRegAddr("fulfill-submitter"),
		ClaimIntent:    "Photons have zero rest mass",
		Domain:         "physics",
		ExpiryBlock:    height + 100, // not yet expired
		BondAmount:     "0",          // no bond — no bank mock needed
		CreatedAtBlock: height - 10,
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))
	params, _ := k.GetParams(ctx)

	createdAt, err := k.FulfillPreRegistration(ctx, pr.Id, pr.Submitter, params)
	require.NoError(t, err)
	require.Equal(t, pr.CreatedAtBlock, createdAt)

	// Must be marked fulfilled.
	got, found := k.GetPreRegistration(ctx, pr.Id)
	require.True(t, found)
	require.True(t, got.Fulfilled)
}

// TestPreRegistration_Fulfill_NotFound verifies error on missing pre-registration.
func TestPreRegistration_Fulfill_NotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	params, _ := k.GetParams(ctx)
	_, err := k.FulfillPreRegistration(ctx, "nonexistent", preRegAddr("x"), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestPreRegistration_Fulfill_WrongSubmitter verifies error when submitter mismatch.
func TestPreRegistration_Fulfill_WrongSubmitter(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pr := &types.PreRegistration{
		Id:          "prereg-wrong-sub",
		Submitter:   preRegAddr("real-owner"),
		ClaimIntent: "Water is H2O",
		Domain:      "chemistry",
		BondAmount:  "0",
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))
	params, _ := k.GetParams(ctx)

	_, err := k.FulfillPreRegistration(ctx, pr.Id, preRegAddr("attacker"), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), preRegAddr("real-owner"))
}

// TestPreRegistration_Fulfill_AlreadyFulfilled verifies idempotency protection.
func TestPreRegistration_Fulfill_AlreadyFulfilled(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pr := &types.PreRegistration{
		Id:               "prereg-already-done",
		Submitter:        preRegAddr("done-sub"),
		ClaimIntent:      "something",
		Domain:           "physics",
		BondAmount:       "0",
		Fulfilled:        true,
		FulfilledClaimId: "claim-xyz",
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))
	params, _ := k.GetParams(ctx)

	_, err := k.FulfillPreRegistration(ctx, pr.Id, pr.Submitter, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already fulfilled")
}

// TestPreRegistration_Fulfill_Expired verifies error when pre-registration is expired.
func TestPreRegistration_Fulfill_Expired(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	pr := &types.PreRegistration{
		Id:          "prereg-expired",
		Submitter:   preRegAddr("expired-sub"),
		ClaimIntent: "expired claim",
		Domain:      "physics",
		BondAmount:  "0",
		Expired:     true,
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))
	params, _ := k.GetParams(ctx)

	_, err := k.FulfillPreRegistration(ctx, pr.Id, pr.Submitter, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}

// TestPreRegistration_Fulfill_PastExpiryBlock verifies error when expiry block passed.
func TestPreRegistration_Fulfill_PastExpiryBlock(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	pr := &types.PreRegistration{
		Id:          "prereg-past-expiry",
		Submitter:   preRegAddr("past-sub"),
		ClaimIntent: "past expiry claim",
		Domain:      "physics",
		BondAmount:  "0",
		ExpiryBlock: height - 1, // already past
	}
	require.NoError(t, k.SetPreRegistration(ctx, pr))
	params, _ := k.GetParams(ctx)

	_, err := k.FulfillPreRegistration(ctx, pr.Id, pr.Submitter, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired at block")
}

// ── Integration tests: MsgPreRegisterClaim ────────────────────────────────────

// TestPreRegisterClaim_Success verifies basic pre-registration creation.
func TestPreRegisterClaim_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	submitter := preRegAddr("good-sub")
	resp, err := ms.PreRegisterClaim(ctx, &types.MsgPreRegisterClaim{
		Submitter:    submitter,
		ClaimIntent:  "Electrons carry negative charge",
		MethodologyId: "",
		Domain:       "physics",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.PreRegistrationId, "pre-registration ID must be set")
	require.Equal(t, uint64(0), resp.ExpiryBlock, "expiry disabled (param=0)")
	require.Equal(t, "0", resp.BondAmount, "no bond configured")

	// Verify stored.
	pr, found := k.GetPreRegistration(ctx, resp.PreRegistrationId)
	require.True(t, found)
	require.Equal(t, submitter, pr.Submitter)
	require.Equal(t, "physics", pr.Domain)
	require.False(t, pr.Fulfilled)
	require.False(t, pr.Expired)
}

// TestPreRegisterClaim_NilRequest verifies nil-safety.
func TestPreRegisterClaim_NilRequest(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.PreRegisterClaim(ctx, nil)
	require.Error(t, err)
}

// TestPreRegisterClaim_EmptyClaimIntent verifies that empty claim_intent is rejected.
func TestPreRegisterClaim_EmptyClaimIntent(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.PreRegisterClaim(ctx, &types.MsgPreRegisterClaim{
		Submitter:   preRegAddr("empty-intent"),
		ClaimIntent: "",
		Domain:      "physics",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claim_intent required")
}

// TestPreRegisterClaim_UnknownDomain verifies that unknown domain is rejected.
func TestPreRegisterClaim_UnknownDomain(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.PreRegisterClaim(ctx, &types.MsgPreRegisterClaim{
		Submitter:   preRegAddr("bad-domain"),
		ClaimIntent: "something",
		Domain:      "not-a-real-domain-xyz",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown domain")
}

// TestPreRegisterClaim_InvalidMethodology verifies rejection of unknown methodology.
func TestPreRegisterClaim_InvalidMethodology(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.PreRegisterClaim(ctx, &types.MsgPreRegisterClaim{
		Submitter:    preRegAddr("bad-method"),
		ClaimIntent:  "something real",
		Domain:       "physics",
		MethodologyId: "M-NONEXISTENT",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found in governance registry")
}

// TestPreRegisterClaim_ExpiryBlockSet verifies expiry block is computed correctly.
func TestPreRegisterClaim_ExpiryBlockSet(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Enable expiry.
	p, _ := k.GetParams(ctx)
	p.PreRegistrationExpiryBlocks = 500
	_ = k.SetParams(ctx, p)

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	resp, err := ms.PreRegisterClaim(ctx, &types.MsgPreRegisterClaim{
		Submitter:   preRegAddr("expiry-sub"),
		ClaimIntent: "carbon is an element",
		Domain:      "chemistry",
	})
	require.NoError(t, err)
	require.Equal(t, height+500, resp.ExpiryBlock, "expiry_block = created_block + expiry_param")
}

// TestParamsValidate_DiscountTooHigh verifies that discount above 50% is rejected.
func TestParamsValidate_DiscountTooHigh(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.PreRegistrationDiscountBps = 600_000 // 60% — above max
	err := p.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "pre_registration_discount_bps")
}

// TestParamsValidate_DiscountValid verifies that valid discount passes.
func TestParamsValidate_DiscountValid(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.PreRegistrationDiscountBps = 200_000 // 20% — valid
	err := p.Validate()
	require.NoError(t, err)
}

// TestGetAllPreRegistrations_Genesis verifies that GetAllPreRegistrations works
// for genesis export/import round-trip.
func TestGetAllPreRegistrations_Genesis(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	prs := []*types.PreRegistration{
		{Id: "prereg-genesis-1", Submitter: preRegAddr("g1"), ClaimIntent: "A", Domain: "physics", BondAmount: "0"},
		{Id: "prereg-genesis-2", Submitter: preRegAddr("g2"), ClaimIntent: "B", Domain: "chemistry", BondAmount: "0"},
	}
	for _, pr := range prs {
		require.NoError(t, k.SetPreRegistration(ctx, pr))
	}

	all := k.GetAllPreRegistrations(ctx)
	require.Len(t, all, 2, "all pre-registrations must be exported")

	ids := make(map[string]bool)
	for _, pr := range all {
		ids[pr.Id] = true
	}
	require.True(t, ids["prereg-genesis-1"])
	require.True(t, ids["prereg-genesis-2"])
}
