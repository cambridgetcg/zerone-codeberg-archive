package keeper_test

// Bohm Dialogue Gap 1 — SUSPENDED claim status.
//
// David Bohm's practice of suspension: "hold the assumption before oneself
// like an object in a mirror — visible, examined, but not acted upon."
// (*On Dialogue*, 1996, p.22.)
//
// A SUSPENDED claim is a Bohmian mirror-state:
//   - Fully queryable (relations, canonical form, contradiction flags preserved)
//   - No verification round may be opened (CreateVerificationRound rejects)
//   - Neither terminal (REJECTED/EXPIRED) nor asserted (ACCEPTED)
//   - Resumable — governance can lift suspension back to IN_VERIFICATION
//
// Tests:
//   1. SuspendClaim_Success: valid in-progress claim → SUSPENDED, fields set
//   2. SuspendClaim_Disabled: SuspensionEnabled=false → ErrSuspensionDisabled
//   3. SuspendClaim_Unauthorized: non-authority → ErrUnauthorized
//   4. SuspendClaim_ClaimNotFound: unknown ID → ErrClaimNotFound
//   5. SuspendClaim_AlreadySuspended: SUSPENDED claim → ErrClaimAlreadySuspended
//   6. SuspendClaim_TerminalStatus: ACCEPTED claim → ErrClaimNotSuspendable
//   7. SuspendClaim_EmptyReason: empty reason → ErrInvalidClaim
//   8. ResumeClaim_Success: SUSPENDED → IN_VERIFICATION, fields cleared
//   9. ResumeClaim_Unauthorized: non-authority → ErrUnauthorized
//  10. ResumeClaim_NotSuspended: IN_VERIFICATION claim → ErrClaimNotSuspended
//  11. ResumeClaim_ClaimNotFound: unknown ID → ErrClaimNotFound
//  12. CreateVerificationRound_BlockedWhileSuspended: gate fires for SUSPENDED claim
//  13. SuspendResumeCycle: full lifecycle — suspend then resume then round allowed

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// enableSuspension fetches params and sets SuspensionEnabled = true.
func enableSuspension(t *testing.T, k keeper.Keeper, ctx sdk.Context) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.SuspensionEnabled = true
	require.NoError(t, k.SetParams(ctx, params))
}

// makePendingClaim injects a PENDING claim for suspension tests.
func makePendingClaim(t *testing.T, k keeper.Keeper, ctx sdk.Context, suffix string) *types.Claim {
	t.Helper()
	claim := &types.Claim{
		Id:               "suspend-claim-" + suffix,
		FactContent:      "Bohm suspension test claim: " + suffix,
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("submitter-" + suffix),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))
	return claim
}

// Test 1: SuspendClaim_Success
func TestSuspendClaim_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	claim := makePendingClaim(t, k, ctx, "success")
	authority := k.GetAuthority()

	resp, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "assumption requires collective examination before verification",
	})
	require.NoError(t, err)
	require.Equal(t, claim.Id, resp.ClaimId)
	require.NotZero(t, resp.SuspendedAtBlock)

	// Verify state in store.
	stored, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_SUSPENDED, stored.Status)
	require.NotZero(t, stored.SuspendedAtBlock)
	require.Equal(t, "assumption requires collective examination before verification", stored.SuspensionReason)
}

// Test 2: SuspendClaim_Disabled
func TestSuspendClaim_Disabled(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	// SuspensionEnabled is false by default — do NOT call enableSuspension.

	claim := makePendingClaim(t, k, ctx, "disabled")
	authority := k.GetAuthority()

	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "some reason",
	})
	require.ErrorIs(t, err, types.ErrSuspensionDisabled)
}

// Test 3: SuspendClaim_Unauthorized
func TestSuspendClaim_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	claim := makePendingClaim(t, k, ctx, "unauth")
	attacker := makeValidBech32Addr("attacker-suspend")

	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: attacker,
		ClaimId:   claim.Id,
		Reason:    "unauthorized suspension attempt",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// Test 4: SuspendClaim_ClaimNotFound
func TestSuspendClaim_ClaimNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	authority := k.GetAuthority()
	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   "nonexistent-claim-id",
		Reason:    "some reason",
	})
	require.ErrorIs(t, err, types.ErrClaimNotFound)
}

// Test 5: SuspendClaim_AlreadySuspended
func TestSuspendClaim_AlreadySuspended(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	// Inject a claim that is already SUSPENDED.
	claim := &types.Claim{
		Id:               "already-suspended-claim",
		FactContent:      "already in mirror-state",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("submitter-already-sus"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_SUSPENDED,
		SuspendedAtBlock: 5,
		SuspensionReason: "prior suspension reason",
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	authority := k.GetAuthority()
	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "double suspension attempt",
	})
	require.ErrorIs(t, err, types.ErrClaimAlreadySuspended)
}

// Test 6: SuspendClaim_TerminalStatus — ACCEPTED claim cannot be suspended.
func TestSuspendClaim_TerminalStatus(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	claim := &types.Claim{
		Id:               "accepted-claim-suspend",
		FactContent:      "already accepted",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("submitter-accepted"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_ACCEPTED,
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	authority := k.GetAuthority()
	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "trying to suspend accepted claim",
	})
	require.ErrorIs(t, err, types.ErrClaimNotSuspendable)
}

// Test 7: SuspendClaim_EmptyReason
func TestSuspendClaim_EmptyReason(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	claim := makePendingClaim(t, k, ctx, "empty-reason")
	authority := k.GetAuthority()

	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "", // empty reason — the Bohmian mirror requires naming the assumption
	})
	require.ErrorIs(t, err, types.ErrInvalidClaim)
}

// Test 8: ResumeClaim_Success
func TestResumeClaim_Success(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	// First suspend the claim.
	claim := makePendingClaim(t, k, ctx, "resume-success")
	authority := k.GetAuthority()

	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "under examination",
	})
	require.NoError(t, err)

	// Verify suspended.
	stored, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_SUSPENDED, stored.Status)

	// Now resume.
	resumeResp, err := ms.ResumeClaim(ctx, &types.MsgResumeClaim{
		Authority: authority,
		ClaimId:   claim.Id,
	})
	require.NoError(t, err)
	require.Equal(t, claim.Id, resumeResp.ClaimId)

	// Verify IN_VERIFICATION + fields cleared.
	resumed, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION, resumed.Status)
	require.Zero(t, resumed.SuspendedAtBlock, "SuspendedAtBlock must be cleared on resume")
	require.Empty(t, resumed.SuspensionReason, "SuspensionReason must be cleared on resume")
}

// Test 9: ResumeClaim_Unauthorized
func TestResumeClaim_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Inject a SUSPENDED claim directly.
	claim := &types.Claim{
		Id:               "claim-resume-unauth",
		FactContent:      "suspended claim",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("submitter-resume-unauth"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_SUSPENDED,
		SuspendedAtBlock: 10,
		SuspensionReason: "under examination",
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	attacker := makeValidBech32Addr("resume-attacker")
	_, err := ms.ResumeClaim(ctx, &types.MsgResumeClaim{
		Authority: attacker,
		ClaimId:   claim.Id,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// Test 10: ResumeClaim_NotSuspended
func TestResumeClaim_NotSuspended(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	// Inject a PENDING (not suspended) claim.
	claim := makePendingClaim(t, k, ctx, "not-suspended")
	authority := k.GetAuthority()

	_, err := ms.ResumeClaim(ctx, &types.MsgResumeClaim{
		Authority: authority,
		ClaimId:   claim.Id,
	})
	require.ErrorIs(t, err, types.ErrClaimNotSuspended)
}

// Test 11: ResumeClaim_ClaimNotFound
func TestResumeClaim_ClaimNotFound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	authority := k.GetAuthority()
	_, err := ms.ResumeClaim(ctx, &types.MsgResumeClaim{
		Authority: authority,
		ClaimId:   "ghost-claim-for-resume",
	})
	require.ErrorIs(t, err, types.ErrClaimNotFound)
}

// Test 12: CreateVerificationRound_BlockedWhileSuspended
// A suspended claim must not be able to enter verification.
func TestCreateVerificationRound_BlockedWhileSuspended(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Inject a claim with SUSPENDED status.
	claim := &types.Claim{
		Id:               "suspended-round-gate",
		FactContent:      "suspended gate test",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("submitter-gate"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_SUSPENDED,
		SuspendedAtBlock: 5,
		SuspensionReason: "under Bohmian examination",
		Stake:            "1000uzrn",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	_, err := k.CreateVerificationRound(ctx, claim, 0)
	require.ErrorIs(t, err, types.ErrClaimNotSuspendable,
		"SUSPENDED claim must be blocked from entering verification")
}

// Test 13: SuspendResumeCycle — full lifecycle
// Suspend → verify gate fires → Resume → CreateVerificationRound succeeds.
func TestSuspendResumeCycle(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)
	enableSuspension(t, k, ctx)

	authority := k.GetAuthority()
	claim := makePendingClaim(t, k, ctx, "full-cycle")

	// Step 1: suspend.
	_, err := ms.SuspendClaim(ctx, &types.MsgSuspendClaim{
		Authority: authority,
		ClaimId:   claim.Id,
		Reason:    "held for deliberation — Bohm dialogue in progress",
	})
	require.NoError(t, err)

	// Step 2: verify round gate fires.
	suspended, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	_, err = k.CreateVerificationRound(ctx, suspended, 0)
	require.ErrorIs(t, err, types.ErrClaimNotSuspendable, "round must be blocked while SUSPENDED")

	// Step 3: resume.
	_, err = ms.ResumeClaim(ctx, &types.MsgResumeClaim{
		Authority: authority,
		ClaimId:   claim.Id,
	})
	require.NoError(t, err)

	// Step 4: CreateVerificationRound succeeds on resumed claim.
	resumed, found := k.GetClaim(ctx, claim.Id)
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION, resumed.Status)
	round, err := k.CreateVerificationRound(ctx, resumed, 0)
	require.NoError(t, err, "round creation must succeed after claim is resumed")
	require.NotEmpty(t, round.Id)
}
