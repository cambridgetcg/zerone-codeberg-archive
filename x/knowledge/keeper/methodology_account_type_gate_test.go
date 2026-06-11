package keeper_test

// Methodology-level account type gate tests (Phenomenology Brief).
//
// Methodology.required_account_types enforces panel composition at the
// methodology layer — stronger than P-13's per-claim flag. When a methodology
// declares required_account_types = ["human"], only verifiers whose
// account_type is "human" may join panels for claims under that methodology.
//
// Gate is fail-open: empty list, missing methodology, or empty method_id all
// allow any verifier through.

import (
	"crypto/sha256"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// seedPhenomenologicalMethodology stores M-PHENOMENOLOGICAL with
// RequiredAccountTypes = ["human"] into the test keeper's registry.
func seedPhenomenologicalMethodology(t *testing.T, k keeper.Keeper, ctx sdk.Context) {
	t.Helper()
	m := &types.Methodology{
		Id:                   types.MethodologyPhenomenologic,
		Name:                 "Phenomenological description (test)",
		RequiredAccountTypes: []string{"human"},
	}
	require.NoError(t, k.SetMethodology(ctx, m))
}

// makeClaimWithMethodology creates a claim tagged with a specific methodology_id.
func makeClaimWithMethodology(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	submitter, methodId string,
) (*types.Claim, *types.VerificationRound) {
	t.Helper()
	claim, round := makeTestClaim(t, k, ctx, submitter,
		"a phenomenological claim about the structure of conscious experience", "", "philosophy", "1000000")
	claim.MethodId = methodId
	require.NoError(t, k.SetClaim(ctx, claim))
	return claim, round
}

// ─── Agent blocked on M-PHENOMENOLOGICAL ─────────────────────────────────────

// TestMethodologyGate_AgentOnPhenomenological_Blocked verifies that an agent
// cannot commit to a round for a M-PHENOMENOLOGICAL claim when the methodology
// declares required_account_types = ["human"].
func TestMethodologyGate_AgentOnPhenomenological_Blocked(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)
	seedPhenomenologicalMethodology(t, k, ctx)

	agent := makeValidBech32Addr("agent-phenom")
	bk.balances[agent] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, agent, "agent")

	_, round := makeClaimWithMethodology(t, k, ctx, makeValidBech32Addr("submitter-phenom"), types.MethodologyPhenomenologic)

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-phenom-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   agent,
		CommitHash: hash[:],
	})
	require.Error(t, err, "agent must be blocked by methodology-level required_account_types gate")
	require.ErrorIs(t, err, types.ErrMethodologyRequiresAccountType)
}

// ─── Human allowed on M-PHENOMENOLOGICAL ─────────────────────────────────────

// TestMethodologyGate_HumanOnPhenomenological_Allowed verifies that a human
// verifier is accepted on a M-PHENOMENOLOGICAL claim.
func TestMethodologyGate_HumanOnPhenomenological_Allowed(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)
	seedPhenomenologicalMethodology(t, k, ctx)

	human := makeValidBech32Addr("human-phenom")
	bk.balances[human] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, human, "human")

	_, round := makeClaimWithMethodology(t, k, ctx, makeValidBech32Addr("submitter-phenom-2"), types.MethodologyPhenomenologic)

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("human-phenom-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   human,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "human verifier must be allowed on M-PHENOMENOLOGICAL claim")
}

// ─── Fail-open: no required types ────────────────────────────────────────────

// TestMethodologyGate_NoRequiredTypes_AllAllowed verifies that when a
// methodology has an empty required_account_types list, agents are allowed.
func TestMethodologyGate_NoRequiredTypes_AllAllowed(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	// Seed a methodology with NO required_account_types.
	m := &types.Methodology{
		Id:                   "M-EMPIRICAL",
		Name:                 "Empirical method (test)",
		RequiredAccountTypes: nil, // empty = no restriction
	}
	require.NoError(t, k.SetMethodology(ctx, m))

	agent := makeValidBech32Addr("agent-empirical")
	bk.balances[agent] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, agent, "agent")

	claim, round := makeClaimWithMethodology(t, k, ctx, makeValidBech32Addr("submitter-empirical"), "M-EMPIRICAL")
	_ = claim

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-empirical-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   agent,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "empty required_account_types must allow all verifiers (fail-open)")
}

// ─── Fail-open: methodology not in registry ───────────────────────────────────

// TestMethodologyGate_MethodologyNotFound_FailOpen verifies that when the
// claim's method_id is not in the registry, the gate is a no-op.
func TestMethodologyGate_MethodologyNotFound_FailOpen(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)
	// Do NOT seed any methodology — simulate unregistered methodology_id.

	agent := makeValidBech32Addr("agent-nomethod")
	bk.balances[agent] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, agent, "agent")

	claim, round := makeClaimWithMethodology(t, k, ctx, makeValidBech32Addr("submitter-nomethod"), "M-NONEXISTENT")
	_ = claim

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-nomethod-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   agent,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "unregistered methodology must not block any verifier (fail-open)")
}

// ─── Fail-open: claim has no method_id ────────────────────────────────────────

// TestMethodologyGate_NoMethodId_FailOpen verifies that a claim with an empty
// method_id bypasses the methodology gate entirely.
func TestMethodologyGate_NoMethodId_FailOpen(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	agent := makeValidBech32Addr("agent-nomethid")
	bk.balances[agent] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	setAccountType(auth, agent, "agent")

	// makeTestClaim creates a claim with empty method_id.
	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("submitter-nomethid"),
		"a claim with no methodology declared", "", "philosophy", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("agent-nomethid-vote"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   agent,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "empty method_id must bypass the methodology gate (fail-open)")
}

// ─── Multiple allowed types ───────────────────────────────────────────────────

// TestMethodologyGate_MultipleAllowedTypes verifies that a methodology can
// permit multiple account types and all of them pass the gate.
func TestMethodologyGate_MultipleAllowedTypes(t *testing.T) {
	k, ctx, bk, auth := setupKeeperWithAuth(t)

	// Seed a methodology allowing both "human" and "agent".
	const testMethodId = "M-TEST-MULTI"
	m := &types.Methodology{
		Id:                   testMethodId,
		Name:                 "Multi-type test methodology",
		RequiredAccountTypes: []string{"human", "agent"},
	}
	require.NoError(t, k.SetMethodology(ctx, m))

	for _, accountType := range []string{"human", "agent"} {
		verifier := makeValidBech32Addr("verifier-multi-" + accountType)
		bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
		setAccountType(auth, verifier, accountType)

		submitter := makeValidBech32Addr("submitter-multi-" + accountType)
		_, round := makeClaimWithMethodology(t, k, ctx, submitter, testMethodId)

		msgServer := keeper.NewMsgServerImpl(k)
		hash := sha256.Sum256([]byte("multi-" + accountType + "-vote"))
		_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
			RoundId:    round.Id,
			Verifier:   verifier,
			CommitHash: hash[:],
		})
		require.NoError(t, err, "account type %q should be allowed by multi-type methodology", accountType)
	}
}

// ─── DefaultMethodologies seed: M-PHENOMENOLOGICAL ships with human gate ─────

// TestDefaultMethodologies_PhenomenologicalRequiresHuman verifies that the
// default methodology registry seeds M-PHENOMENOLOGICAL with
// RequiredAccountTypes = ["human"].
func TestDefaultMethodologies_PhenomenologicalRequiresHuman(t *testing.T) {
	k, ctx, _, _ := setupKeeperWithAuth(t)

	// Seed all default methodologies (as InitGenesis does).
	require.NoError(t, k.SeedDefaultMethodologies(ctx))

	m, found := k.GetMethodology(ctx, types.MethodologyPhenomenologic)
	require.True(t, found)
	require.Equal(t, []string{"human"}, m.RequiredAccountTypes,
		"M-PHENOMENOLOGICAL must ship with required_account_types = [\"human\"] by default")
}
