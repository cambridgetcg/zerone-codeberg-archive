package keeper_test

// ─── Settlement Finality Gaps 1 and 2 ────────────────────────────────────────
//
// Gap 1 — Block Hash Anchoring (★★★★☆)
//   VerdictBlockHash is set on VerificationRound.VerdictBlockHash at CompleteRound
//   time using sdk.Context.HeaderHash(). Anchors every verdict to the specific
//   chain history that produced it — prevents long-range attack replay and provides
//   the cryptographic commitment primitive for IBC cross-chain truth attestation.
//   (settlement-finality-brief.md §Gap 1)
//
// Gap 2 — Minimum Round Duration (★★★☆☆)
//   min_round_duration_blocks Params field enforces a governance-set floor on total
//   round length (commit + reveal + aggregation phases). Prevents governance from
//   accidentally configuring rounds so short that stake can cycle faster than
//   manipulation becomes uneconomical.
//   (settlement-finality-brief.md §Gap 2)

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// minimalResult returns a simple ACCEPT result with no rewards or slashes.
func minimalResult() *keeper.VerificationResult {
	return &keeper.VerificationResult{
		Verdict:     types.Verdict_VERDICT_ACCEPT,
		Confidence:  800_000,
		Rewards:     nil,
		Slashes:     nil,
		AcceptCount: 3,
		RejectCount: 0,
	}
}

// minimalClaim creates a Claim in state with just enough fields for CompleteRound.
func minimalClaim(k keeper.Keeper, ctx sdk.Context, t *testing.T, claimID string) {
	t.Helper()
	claim := &types.Claim{
		Id:        claimID,
		Submitter: makeValidBech32Addr("submitter-" + claimID),
		Status:    types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:     "0",
		Domain:    "mathematics",
	}
	require.NoError(t, k.SetClaim(ctx, claim))
}

// ─── Gap 1: Block Hash Anchoring ─────────────────────────────────────────────

// TestVerdictBlockHash_SetOnCompletion verifies that CompleteRound stamps the
// VerdictBlockHash from sdk.Context.HeaderHash() onto the round record. This
// anchors the verdict to a specific block in chain history.
func TestVerdictBlockHash_SetOnCompletion(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	claimID := "claim-blockhash-1"
	roundID := "round-blockhash-1"

	minimalClaim(k, ctx, t, claimID)

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             claimID,
		StartedAtBlock:      95,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      110,
		RevealDeadline:      130,
		AggregationDeadline: 150,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Attach a known 32-byte block hash to the context.
	expectedHash := []byte("test-block-hash-32bytes-padded!!")
	ctxWithHash := ctx.WithHeaderHash(expectedHash)

	err := k.CompleteRound(ctxWithHash, round, minimalResult())
	require.NoError(t, err)

	// Read back from state — not from the local round pointer, which may be stale.
	stored, found := k.GetVerificationRound(ctxWithHash, roundID)
	require.True(t, found)
	require.Equal(t, expectedHash, stored.VerdictBlockHash,
		"VerdictBlockHash must equal the context header hash at CompleteRound time")
}

// TestVerdictBlockHash_EmptyBeforeCompletion verifies that a newly created round
// has no VerdictBlockHash set — it is only stamped at COMPLETE transition.
func TestVerdictBlockHash_EmptyBeforeCompletion(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-pre-complete", "claim-pre", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	stored, found := k.GetVerificationRound(ctx, round.Id)
	require.True(t, found)
	require.Empty(t, stored.VerdictBlockHash,
		"VerdictBlockHash must be empty before round completion")
}

// TestVerdictBlockHash_NilContextHashStillCompletes verifies that CompleteRound
// does not fail when the context has no header hash set (e.g. test contexts).
// The field will store nil/empty bytes — which is safe and backward-compatible.
func TestVerdictBlockHash_NilContextHashStillCompletes(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	claimID := "claim-nil-hash"
	roundID := "round-nil-hash"

	minimalClaim(k, ctx, t, claimID)

	round := &types.VerificationRound{
		Id:                  roundID,
		ClaimId:             claimID,
		StartedAtBlock:      95,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      110,
		RevealDeadline:      130,
		AggregationDeadline: 150,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// ctx has no explicit header hash — HeaderHash() returns nil.
	err := k.CompleteRound(ctx, round, minimalResult())
	require.NoError(t, err, "CompleteRound must succeed even when context header hash is nil")
}

// TestVerdictBlockHash_ProtoRoundTrip verifies that a round with a non-nil
// VerdictBlockHash serializes and deserializes through the state store
// without data loss.
func TestVerdictBlockHash_ProtoRoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	roundID := "round-roundtrip"
	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}

	round := testRound(roundID, "claim-rt", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.VerdictBlockHash = hash

	require.NoError(t, k.SetVerificationRound(ctx, round))

	stored, found := k.GetVerificationRound(ctx, roundID)
	require.True(t, found)
	require.Equal(t, hash, stored.VerdictBlockHash,
		"VerdictBlockHash must survive proto encode/decode through state store")
}

// ─── Gap 2: Minimum Round Duration ───────────────────────────────────────────

// TestMinRoundDuration_DefaultDisabled verifies that the default params have
// MinRoundDurationBlocks = 0, meaning no constraint is applied. Existing rounds
// are unaffected by this feature until governance explicitly enables it.
func TestMinRoundDuration_DefaultDisabled(t *testing.T) {
	p := types.DefaultParams()
	require.Equal(t, uint64(0), p.MinRoundDurationBlocks,
		"default MinRoundDurationBlocks must be 0 (disabled)")
}

// TestMinRoundDuration_ParamsValidate_Disabled verifies that when
// MinRoundDurationBlocks = 0, Validate() imposes no constraint on phase lengths.
func TestMinRoundDuration_ParamsValidate_Disabled(t *testing.T) {
	p := types.DefaultParams()
	// Artificially shorten all phases — should still pass because min is disabled.
	p.CommitPhaseBlocks = 1
	p.RevealPhaseBlocks = 1
	p.AggregationPhaseBlocks = 1
	p.MinRoundDurationBlocks = 0
	require.NoError(t, p.Validate(),
		"disabled MinRoundDurationBlocks must not constrain phase lengths")
}

// TestMinRoundDuration_ParamsValidate_Satisfied verifies that Validate() passes
// when the total phase duration meets or exceeds the configured minimum.
func TestMinRoundDuration_ParamsValidate_Satisfied(t *testing.T) {
	p := types.DefaultParams()
	// Default: commit=200 + reveal=200 + aggregation=50 = 450
	p.MinRoundDurationBlocks = 450
	require.NoError(t, p.Validate(),
		"MinRoundDurationBlocks exactly met must pass Validate()")

	p.MinRoundDurationBlocks = 300
	require.NoError(t, p.Validate(),
		"MinRoundDurationBlocks below total must pass Validate()")
}

// TestMinRoundDuration_ParamsValidate_Violated verifies that Validate() rejects
// params where the configured phase durations are collectively shorter than the
// minimum — preventing governance from accidentally setting a too-short round.
func TestMinRoundDuration_ParamsValidate_Violated(t *testing.T) {
	p := types.DefaultParams()
	// Total = 200+200+50 = 450, but minimum is 500 → invalid.
	p.MinRoundDurationBlocks = 500
	err := p.Validate()
	require.Error(t, err,
		"MinRoundDurationBlocks exceeding total phase duration must fail Validate()")
	require.Contains(t, err.Error(), "min_round_duration_blocks",
		"error message must identify the offending param")
}

// TestMinRoundDuration_CreateRound_Disabled verifies that when MinRoundDurationBlocks
// is 0 (default), CreateVerificationRound succeeds regardless of phase lengths.
func TestMinRoundDuration_CreateRound_Disabled(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Override params to have very short phases but min = 0.
	p, err := k.GetParams(ctx)
	require.NoError(t, err)
	p.CommitPhaseBlocks = 5
	p.RevealPhaseBlocks = 5
	p.AggregationPhaseBlocks = 5
	p.MinRoundDurationBlocks = 0
	require.NoError(t, k.SetParams(ctx, p))

	claimID := "claim-min-disabled"
	minimalClaim(k, ctx, t, claimID)
	claim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)

	_, err = k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err,
		"CreateVerificationRound must succeed when MinRoundDurationBlocks is disabled")
}

// TestMinRoundDuration_CreateRound_Satisfied verifies that round creation succeeds
// when the total phase duration meets the configured minimum.
func TestMinRoundDuration_CreateRound_Satisfied(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, err := k.GetParams(ctx)
	require.NoError(t, err)
	// commit=100, reveal=100, aggregation=50 → total=250; min=250 → exactly satisfied.
	p.CommitPhaseBlocks = 100
	p.RevealPhaseBlocks = 100
	p.AggregationPhaseBlocks = 50
	p.MinRoundDurationBlocks = 250
	require.NoError(t, k.SetParams(ctx, p))

	claimID := "claim-min-satisfied"
	minimalClaim(k, ctx, t, claimID)
	claim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)

	_, err = k.CreateVerificationRound(ctx, claim, 0)
	require.NoError(t, err,
		"CreateVerificationRound must succeed when total phases >= MinRoundDurationBlocks")
}

// TestMinRoundDuration_CreateRound_Violated verifies that CreateVerificationRound
// returns an error when the configured phase durations fall below the minimum.
// This is the belt-and-suspenders guard for pre-upgrade state.
func TestMinRoundDuration_CreateRound_Violated(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	p, err := k.GetParams(ctx)
	require.NoError(t, err)
	// Directly set params bypassing Validate() to simulate pre-upgrade state.
	// commit=10, reveal=10, aggregation=5 → total=25; min=100 → violated.
	p.CommitPhaseBlocks = 10
	p.RevealPhaseBlocks = 10
	p.AggregationPhaseBlocks = 5
	p.MinRoundDurationBlocks = 100
	// Use SetParams directly to bypass Validate() — simulates migrated state.
	require.NoError(t, k.SetParams(ctx, p))

	claimID := "claim-min-violated"
	minimalClaim(k, ctx, t, claimID)
	claim, found := k.GetClaim(ctx, claimID)
	require.True(t, found)

	_, err = k.CreateVerificationRound(ctx, claim, 0)
	require.Error(t, err,
		"CreateVerificationRound must fail when total phases < MinRoundDurationBlocks")
	require.Contains(t, err.Error(), "min_round_duration_blocks",
		"error must identify the violated parameter")
}

// ─── Context: sdk.Context with header hash ───────────────────────────────────

// Verify that sdk.Context.WithHeaderHash plumbing works as expected — that
// the hash we set is the hash we read back via HeaderHash(). Guards against
// accidental substrate change in context handling.
func TestSDKContext_WithHeaderHash_RoundTrip(t *testing.T) {
	_, ctx := setupKnowledgeKeeper(t)
	_ = cmtproto.Header{} // ensure import used

	hash := []byte("known-hash-value-for-testing-123")
	ctxWithHash := ctx.WithHeaderHash(hash)
	require.Equal(t, hash, ctxWithHash.HeaderHash(),
		"WithHeaderHash + HeaderHash must be identity")
	// Original context unaffected.
	require.Empty(t, ctx.HeaderHash(),
		"original context header hash must remain empty after WithHeaderHash on copy")
}
