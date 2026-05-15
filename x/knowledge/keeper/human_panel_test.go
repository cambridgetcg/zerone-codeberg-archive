package keeper_test

// P-13: Human-only panel gate tests.
//
// When Claim.RequiresHumanVerifiers is true, AI agent accounts are blocked
// from joining the verification panel. This is a hard constraint set by the
// claim submitter — no fallback, no governance threshold. The purpose is to
// reserve certain panels (constitutional facts, legal interpretations,
// governance precedents) for human judgment exclusively.
//
// Note: mockZeroneAuthKeeper is declared in role_bonus_test.go (same package).
// Tests here rely on that shared mock; we add a setType helper via a local
// helper function to keep test code readable.

import (
	"crypto/sha256"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// setupKeeperWithAuth wires a mockZeroneAuthKeeper into the knowledge keeper.
func setupKeeperWithAuth(t *testing.T) (keeper.Keeper, sdk.Context, *trackingBankKeeper, *mockZeroneAuthKeeper) {
	t.Helper()
	k, ctx, bk, _ := setupKnowledgeTestFull(t)
	auth := newMockZeroneAuthKeeper()
	k.SetZeroneAuthKeeper(auth)
	return k, ctx, bk, auth
}

// setAccountType sets the account type for a given address in the mock keeper.
func setAccountType(auth *mockZeroneAuthKeeper, addr, accountType string) {
	auth.accounts[addr] = accountType
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// A human verifier on a human-only claim: should be accepted.
func TestSubmitCommitment_HumanPanel_HumanVerifier_Accepted(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	verifier := makeValidBech32Addr("human-verifier")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, verifier, "human")

	// Create a human-only claim.
	claim, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("submitter-human"),
		"constitutional claim", "", "governance", "1000000")
	claim.RequiresHumanVerifiers = true
	require.NoError(t, k.SetClaim(ctx, claim))

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("human-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "human verifier must be accepted on a human-only claim")
}

// An agent verifier on a human-only claim: must be rejected.
func TestSubmitCommitment_HumanPanel_AgentVerifier_Rejected(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	verifier := makeValidBech32Addr("agent-verifier")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, verifier, "agent")

	claim, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("submitter-for-agent"),
		"constitutional claim 2", "", "governance", "1000000")
	claim.RequiresHumanVerifiers = true
	require.NoError(t, k.SetClaim(ctx, claim))

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.ErrorIs(t, err, types.ErrAgentNotAllowedOnHumanPanel,
		"agent verifier must be rejected on a human-only claim")
}

// Agent verifier on a normal (non-human-only) claim: should be accepted.
func TestSubmitCommitment_NormalClaim_AgentVerifier_Accepted(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	verifier := makeValidBech32Addr("agent-normal-claim")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, verifier, "agent")

	// Normal claim: RequiresHumanVerifiers defaults to false.
	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("submitter-normal"),
		"factual claim about gravity", "", "scientific", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-normal-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "agent verifier must be accepted on a normal (non-human-only) claim")
}

// Unknown account type (not registered in auth keeper) on a human-only claim:
// should be accepted — the gate only blocks "agent", not unknowns.
func TestSubmitCommitment_HumanPanel_UnknownAccountType_Accepted(t *testing.T) {
	k, ctx, bk, _ := setupKeeperWithAuth(t)

	verifier := makeValidBech32Addr("unknown-type-verifier")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	// Not registered → GetAccountType returns "", false → getAccountType returns ""

	claim, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("submitter-unknown"),
		"constitutional claim 3", "", "governance", "1000000")
	claim.RequiresHumanVerifiers = true
	require.NoError(t, k.SetClaim(ctx, claim))

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("unknown-type-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err,
		"unknown account type must be accepted on human-only claim (gate only blocks 'agent')")
}

// MsgSubmitClaim propagates RequiresHumanVerifiers to the stored Claim.
func TestSubmitClaim_PropagatesRequiresHumanVerifiers(t *testing.T) {
	k, ctx, _, _ := setupKeeperWithAuth(t)

	submitter := makeValidBech32Addr("submitter-propagate")

	msgServer := keeper.NewMsgServerImpl(k)
	resp, err := msgServer.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:              submitter,
		FactContent:            "human-only constitutional fact that is long enough to pass content validation",
		Category:               "governance",
		Stake:                  "1000000",
		RequiresHumanVerifiers: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	// Fetch the stored claim and verify the flag was persisted.
	claim, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.True(t, claim.RequiresHumanVerifiers,
		"RequiresHumanVerifiers must be propagated from MsgSubmitClaim to stored Claim")
}
