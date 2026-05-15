package keeper_test

// P-10a: Verifier Independence Pre-Check Gate tests.
//
// The independence gate blocks validators who consistently vote with the
// majority from joining new verification panels, preserving epistemic
// diversity. Default off (min_verifier_independence_bps = 0); governance
// enables when validators have sufficient vote history.

import (
	"crypto/sha256"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Helper: seed independence records ───────────────────────────────────────

// seedIndependence sets a validator's independence record directly.
// minorityFraction is e.g. 0.10 meaning 10% minority votes → score 100_000 bps.
func seedIndependence(t *testing.T, k keeper.Keeper, ctx sdk.Context, validator string, totalVotes, minorityVotes uint64) {
	t.Helper()
	err := k.SetValidatorIndependence(ctx, validator, keeper.ValidatorIndependenceRecord{
		Validator:     validator,
		TotalVotes:    totalVotes,
		MinorityVotes: minorityVotes,
	})
	require.NoError(t, err)
}

// ─── CountValidatorsWithSufficientIndependence ───────────────────────────────

func TestCountValidatorsWithSufficientIndependence_Empty(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	count := k.CountValidatorsWithSufficientIndependence(ctx, 100_000, 10)
	require.Equal(t, 0, count, "empty store should return 0")
}

func TestCountValidatorsWithSufficientIndependence_CountsCorrectly(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// val1: 20% dissent (200_000 bps) — above threshold of 100_000
	seedIndependence(t, k, ctx, "zrn1val1", 100, 20)
	// val2: 5% dissent (50_000 bps) — below threshold of 100_000
	seedIndependence(t, k, ctx, "zrn1val2", 100, 5)
	// val3: 15% dissent (150_000 bps) — above threshold
	seedIndependence(t, k, ctx, "zrn1val3", 100, 15)
	// val4: only 5 rounds — below minRounds=10, excluded
	seedIndependence(t, k, ctx, "zrn1val4", 5, 5)

	count := k.CountValidatorsWithSufficientIndependence(ctx, 100_000, 10)
	require.Equal(t, 2, count, "only val1 and val3 meet both threshold and minRounds")
}

func TestCountValidatorsWithSufficientIndependence_ZeroMinRounds(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// val with only 1 round — would be excluded at minRounds=10 but not at 0
	seedIndependence(t, k, ctx, "zrn1val1", 1, 1) // 100% dissent
	count := k.CountValidatorsWithSufficientIndependence(ctx, 100_000, 0)
	require.Equal(t, 1, count)
}

// ─── MsgSubmitCommitment independence gate tests ─────────────────────────────

func TestSubmitCommitment_IndependenceGate_DisabledByDefault(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	verifier := makeValidBech32Addr("verifier-low-ind")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))

	// Verifier has terrible independence score: 0 minority votes in 50 rounds
	seedIndependence(t, k, ctx, verifier, 50, 0)

	// Default params: gate disabled (MinVerifierIndependenceBps = 0)
	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub1"),
		"some claim", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("reveal" + "salt"))
	_, err := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err, "gate disabled: low-independence verifier must be accepted")
}

func TestSubmitCommitment_IndependenceGate_HighScoreAccepted(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	// Enable the gate at 50_000 bps (5% minimum dissent).
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MinVerifierIndependenceBps = 50_000
	params.MinVerifierIndependenceRounds = 10
	params.MinVerifiers = 3
	require.NoError(t, k.SetParams(ctx, params))

	verifier := makeValidBech32Addr("verifier-good-ind")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))

	// 20% dissent = 200_000 bps → above threshold of 50_000
	seedIndependence(t, k, ctx, verifier, 50, 10)

	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub2"),
		"another claim", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("high-ind-hash"))
	_, err2 := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err2, "high-independence verifier must be accepted")
}

func TestSubmitCommitment_IndependenceGate_LowScoreRejected_WhenEnoughIndependentValidators(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	// Enable gate.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MinVerifierIndependenceBps = 100_000 // 10% minimum dissent
	params.MinVerifierIndependenceRounds = 10
	params.MinVerifiers = 3
	require.NoError(t, k.SetParams(ctx, params))

	// Plant 3 high-independence validators in the store (→ fallback won't trigger).
	seedIndependence(t, k, ctx, "zrn1independent1", 100, 20) // 20% → 200_000 bps
	seedIndependence(t, k, ctx, "zrn1independent2", 100, 15) // 15% → 150_000 bps
	seedIndependence(t, k, ctx, "zrn1independent3", 100, 12) // 12% → 120_000 bps

	// The verifier under test has 2% dissent = 20_000 bps → below 100_000.
	verifier := makeValidBech32Addr("verifier-low")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	seedIndependence(t, k, ctx, verifier, 50, 1) // 1/50 = 20_000 bps

	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub3"),
		"claim needing diverse panel", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("low-ind-hash"))
	_, err2 := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.ErrorIs(t, err2, types.ErrLowVerifierIndependence,
		"low-independence verifier must be rejected when enough independent validators exist")
}

func TestSubmitCommitment_IndependenceGate_LowScoreAllowed_WhenInsufficientIndependentValidators(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	// Enable gate at 10% minimum.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MinVerifierIndependenceBps = 100_000
	params.MinVerifierIndependenceRounds = 10
	params.MinVerifiers = 3
	require.NoError(t, k.SetParams(ctx, params))

	// Only 2 high-independence validators in the store — fewer than MinVerifiers (3).
	seedIndependence(t, k, ctx, "zrn1independent1", 100, 20)
	seedIndependence(t, k, ctx, "zrn1independent2", 100, 15)

	verifier := makeValidBech32Addr("verifier-low-fallback")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	seedIndependence(t, k, ctx, verifier, 50, 1) // 20_000 bps — below threshold

	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub4"),
		"claim with sparse validators", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("fallback-hash"))
	_, err2 := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err2,
		"low-independence verifier must be allowed when insufficient independent validators exist (fallback)")
}

func TestSubmitCommitment_IndependenceGate_NewValidator_NotGated(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	// Enable gate.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MinVerifierIndependenceBps = 100_000
	params.MinVerifierIndependenceRounds = 10 // need 10 rounds before gated
	require.NoError(t, k.SetParams(ctx, params))

	// New validator — only 3 rounds completed, below MinVerifierIndependenceRounds.
	verifier := makeValidBech32Addr("verifier-new")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))
	seedIndependence(t, k, ctx, verifier, 3, 0) // 3 rounds, 0% dissent

	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub5"),
		"claim for new validator", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("new-val-hash"))
	_, err2 := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err2,
		"new validator with fewer rounds than MinVerifierIndependenceRounds must NOT be gated")
}

func TestSubmitCommitment_IndependenceGate_NoRecordAtAll_NotGated(t *testing.T) {
	k, ctx, bk, _ := setupKnowledgeTestFull(t)

	// Enable gate.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MinVerifierIndependenceBps = 100_000
	params.MinVerifierIndependenceRounds = 10
	require.NoError(t, k.SetParams(ctx, params))

	// Verifier with no independence record at all (brand new).
	verifier := makeValidBech32Addr("verifier-brand-new")
	bk.balances[verifier] = sdk.NewCoins(sdk.NewInt64Coin("uzrn", 200_000_000))

	_, round := makeTestClaim(t, k, ctx, makeValidBech32Addr("sub6"),
		"claim for brand-new verifier", "", "computational", "1000000")

	msgServer := keeper.NewMsgServerImpl(k)
	hash := sha256.Sum256([]byte("brand-new-hash"))
	_, err2 := msgServer.SubmitCommitment(ctx, &types.MsgSubmitCommitment{
		RoundId:    round.Id,
		Verifier:   verifier,
		CommitHash: hash[:],
	})
	require.NoError(t, err2,
		"verifier with no independence record (brand new) must NOT be gated — record not found → skip check")
}
