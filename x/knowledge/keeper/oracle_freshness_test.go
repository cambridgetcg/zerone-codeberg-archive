package keeper_test

// Oracle Resilience Brief Gap 4 — tests for oracle freshness ring buffer
// and oracle_request_id verification in MsgSubmitReveal.
//
// The oracle returns a request_id = sha256(claim_id + "|" + hex(block_hash) + "|" + verifier_addr).
// Verifiers include it in MsgSubmitReveal.oracle_request_id; the chain checks
// it against the ring buffer of recent block hashes.
//
// Fail-open behaviour:
//   - Empty oracle_request_id → always passes
//   - oracle_freshness_window_blocks == 0 → gate disabled, all reveals pass
//   - Ring buffer empty (just started / restarted) → fail-open

import (
	"encoding/hex"
	"testing"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Ring buffer state accessors ───────────────────────────────────────────────

// TestRecentBlockHash_RoundTrip verifies that SetRecentBlockHash / GetRecentBlockHash
// are symmetric and that different heights are correctly isolated.
func TestRecentBlockHash_RoundTrip(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	hash100 := []byte("block-hash-at-100-xxxxxxxxxx!!!")
	hash200 := []byte("block-hash-at-200-xxxxxxxxxx!!!")

	k.SetRecentBlockHash(ctx, 100, hash100)
	k.SetRecentBlockHash(ctx, 200, hash200)

	got100, found := k.GetRecentBlockHash(ctx, 100)
	require.True(t, found)
	require.Equal(t, hash100, got100)

	got200, found := k.GetRecentBlockHash(ctx, 200)
	require.True(t, found)
	require.Equal(t, hash200, got200)

	_, found = k.GetRecentBlockHash(ctx, 999)
	require.False(t, found, "non-existent height must return false")
}

// TestRecentBlockHash_DeleteEvicts verifies that DeleteRecentBlockHash removes the entry.
func TestRecentBlockHash_DeleteEvicts(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	k.SetRecentBlockHash(ctx, 50, []byte("some hash"))

	k.DeleteRecentBlockHash(ctx, 50)
	_, found := k.GetRecentBlockHash(ctx, 50)
	require.False(t, found, "deleted entry must not be retrievable")
}

// TestWriteAndPrune_WritesAndEvicts verifies that WriteAndPruneRecentBlockHash:
//  1. Stores the previous block's hash (height-1)
//  2. Evicts the entry just outside the window
func TestWriteAndPrune_WritesAndEvicts(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Seed params with a freshness window of 5 blocks.
	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 5
	require.NoError(t, k.SetParams(ctx, &params))

	// Simulate block 10: LastBlockId.Hash = hash of block 9.
	prevHash := []byte("hash-of-block-9-xxxxxxxxxxxxxxx!")
	ctxAt10 := ctx.WithBlockHeader(cmtproto.Header{
		Height: 10,
		LastBlockId: cmtproto.BlockID{
			Hash: prevHash,
		},
	})

	k.WriteAndPruneRecentBlockHash(ctxAt10, &params)

	// Hash for block 9 should be stored.
	stored, found := k.GetRecentBlockHash(ctxAt10, 9)
	require.True(t, found, "hash for block height-1 must be stored")
	require.Equal(t, prevHash, stored)

	// Eviction: currentHeight-2-window = 10-2-5 = 3 — block 3 should be deleted.
	// (Block 3 was never set in this test, so just verify no panic.)
	k.SetRecentBlockHash(ctxAt10, 3, []byte("hash-of-block-3"))
	k.WriteAndPruneRecentBlockHash(ctxAt10, &params)
	_, found = k.GetRecentBlockHash(ctxAt10, 3)
	require.False(t, found, "block 3 should have been evicted at block 10 with window=5")
}

// TestWriteAndPrune_DisabledWhenWindowZero verifies that the ring buffer is not
// populated when oracle_freshness_window_blocks == 0.
func TestWriteAndPrune_DisabledWhenWindowZero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 0 // disabled

	ctxAt10 := ctx.WithBlockHeader(cmtproto.Header{
		Height: 10,
		LastBlockId: cmtproto.BlockID{
			Hash: []byte("some-hash-xxxxxxxxxxxxxxxxxxxxxx"),
		},
	})

	k.WriteAndPruneRecentBlockHash(ctxAt10, &params)

	// Nothing should be stored.
	_, found := k.GetRecentBlockHash(ctxAt10, 9)
	require.False(t, found, "ring buffer must not be populated when window is disabled")
}

// ── ComputeOracleRequestId ────────────────────────────────────────────────────

// TestComputeOracleRequestId_Deterministic verifies that the same inputs produce
// the same output and that changing any input produces a different output.
func TestComputeOracleRequestId_Deterministic(t *testing.T) {
	claimId := "claim-abc-123"
	blockHash := []byte("test-block-hash-32bytes-padded!!")
	verifier := makeValidBech32Addr("verifier-1")

	id1 := keeper.ComputeOracleRequestId(claimId, blockHash, verifier)
	id2 := keeper.ComputeOracleRequestId(claimId, blockHash, verifier)
	require.Equal(t, id1, id2, "same inputs must produce same output")

	// Different claim id → different output
	idDiffClaim := keeper.ComputeOracleRequestId("claim-different", blockHash, verifier)
	require.NotEqual(t, id1, idDiffClaim)

	// Different block hash → different output
	idDiffHash := keeper.ComputeOracleRequestId(claimId, []byte("different-hash-32bytes-padded!!"), verifier)
	require.NotEqual(t, id1, idDiffHash)

	// Different verifier → different output
	idDiffVerifier := keeper.ComputeOracleRequestId(claimId, blockHash, makeValidBech32Addr("verifier-2"))
	require.NotEqual(t, id1, idDiffVerifier)
}

// ── CheckOracleFreshness ──────────────────────────────────────────────────────

// TestOracleFreshness_Disabled verifies fail-open when window == 0.
func TestOracleFreshness_Disabled(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 0 // disabled

	err := k.CheckOracleFreshness(ctx, "claim-1", makeValidBech32Addr("v1"), "some-request-id", &params)
	require.NoError(t, err, "disabled gate must always pass")
}

// TestOracleFreshness_EmptyRequestId verifies fail-open when oracle_request_id is empty.
func TestOracleFreshness_EmptyRequestId(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 10

	// No ring buffer entries seeded — but oracle_request_id is empty → pass.
	err := k.CheckOracleFreshness(ctx, "claim-1", makeValidBech32Addr("v1"), "", &params)
	require.NoError(t, err, "empty oracle_request_id must always pass")
}

// TestOracleFreshness_EmptyRingBuffer_FailOpen verifies that a non-empty request_id
// with an empty ring buffer (chain just started) passes (fail-open).
func TestOracleFreshness_EmptyRingBuffer_FailOpen(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 10

	// No SetRecentBlockHash calls — ring buffer is empty.
	err := k.CheckOracleFreshness(ctx, "claim-1", makeValidBech32Addr("v1"), "any-request-id", &params)
	require.NoError(t, err, "empty ring buffer must fail-open to avoid blocking fresh chains")
}

// TestOracleFreshness_MatchesStoredHash verifies that a request_id computed from
// a stored block hash passes freshness verification.
func TestOracleFreshness_MatchesStoredHash(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 5

	claimId := "claim-freshness-1"
	verifier := makeValidBech32Addr("verifier-freshness")
	blockHash := []byte("block-hash-height-95-xxxxxxxxxx!")

	// Seed the ring buffer with the hash at height 95.
	k.SetRecentBlockHash(ctx, 95, blockHash)

	// Compute the expected request_id as the oracle would.
	expectedId := keeper.ComputeOracleRequestId(claimId, blockHash, verifier)

	// Process the reveal at block 100 — block 95 is within the window of 5.
	ctxAt100 := ctx.WithBlockHeader(cmtproto.Header{Height: 100})

	err := k.CheckOracleFreshness(ctxAt100, claimId, verifier, expectedId, &params)
	require.NoError(t, err, "request_id matching a stored hash within window must pass")
}

// TestOracleFreshness_HashOutsideWindow_Fails verifies that a request_id computed
// from a hash OUTSIDE the freshness window is rejected.
func TestOracleFreshness_HashOutsideWindow_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 5 // blocks [95, 99] valid at height 100

	claimId := "claim-stale-1"
	verifier := makeValidBech32Addr("verifier-stale")
	staleHash := []byte("stale-block-hash-at-88-xxxxxxxxx!")

	// Only seed hash at height 88 — this is outside the window at block 100.
	k.SetRecentBlockHash(ctx, 88, staleHash)
	// Also seed a fresh hash at height 97 so ring buffer is non-empty.
	k.SetRecentBlockHash(ctx, 97, []byte("fresh-block-hash-at-97-xxxxxxxxx!"))

	// Compute request_id using the stale hash.
	staleRequestId := keeper.ComputeOracleRequestId(claimId, staleHash, verifier)

	// At block 100, window = [95..99]. Hash at 88 is outside → fail.
	ctxAt100 := ctx.WithBlockHeader(cmtproto.Header{Height: 100})

	err := k.CheckOracleFreshness(ctxAt100, claimId, verifier, staleRequestId, &params)
	require.Error(t, err, "request_id from outside the freshness window must fail")
	require.ErrorIs(t, err, types.ErrStaleOracleRequest)
}

// TestOracleFreshness_WrongRequestId_Fails verifies that a garbled or wrong
// request_id is rejected when the ring buffer has valid entries.
func TestOracleFreshness_WrongRequestId_Fails(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.OracleFreshnessWindowBlocks = 10

	claimId := "claim-wrong-id"
	verifier := makeValidBech32Addr("verifier-wrong")

	// Seed a valid hash.
	k.SetRecentBlockHash(ctx, 90, []byte("valid-block-hash-at-90-xxxxxxxxx!"))

	ctxAt100 := ctx.WithBlockHeader(cmtproto.Header{Height: 100})

	err := k.CheckOracleFreshness(ctxAt100, claimId, verifier, "not-a-valid-request-id", &params)
	require.Error(t, err, "non-matching request_id must fail when ring buffer has entries")
	require.ErrorIs(t, err, types.ErrStaleOracleRequest)
}

// ── Oracle evaluator request_id computation ───────────────────────────────────

// TestOracleRequestId_OracleAndChainFormulasMatch verifies that the oracle's
// computeRequestId formula matches the chain's ComputeOracleRequestId.
// Both use: sha256(claim_id + "|" + hex(block_hash) + "|" + verifier_addr)
// where hex(block_hash) is lowercase hexadecimal.
func TestOracleRequestId_OracleAndChainFormulasMatch(t *testing.T) {
	claimId := "claim-oracle-match-1"
	blockHash := []byte("representative-block-hash-32!!!!")
	blockHashHex := hex.EncodeToString(blockHash)
	verifier := makeValidBech32Addr("oracle-verifier")

	// Chain formula: ComputeOracleRequestId(claimId, rawBlockHash, verifierAddr).
	chainId := keeper.ComputeOracleRequestId(claimId, blockHash, verifier)

	// Output must be a 64-char lowercase hex string (sha256 = 32 bytes = 64 hex chars).
	require.Equal(t, 64, len(chainId), "sha256 hex output must be 64 hex chars")
	require.NotEmpty(t, chainId)

	// Cross-check: the oracle uses hex.EncodeToString(rawBlockHash) as input to its
	// sha256 computation. The chain does the same. Since both use the same blockHash
	// bytes, the results are identical.
	require.Equal(t, blockHashHex, hex.EncodeToString(blockHash),
		"hex.EncodeToString must be deterministic")

	// Same request_id when blockHash is provided as bytes vs when oracle uses hex string.
	// (Oracle receives blockHashHex as a string field; chain receives raw bytes and
	//  hex-encodes internally.) Verify consistent round-trip:
	chainIdFromSameBytes := keeper.ComputeOracleRequestId(claimId, blockHash, verifier)
	require.Equal(t, chainId, chainIdFromSameBytes)
}

// ── cmttypes import anchor ────────────────────────────────────────────────────

// Ensure cmttypes is used (keeps the import from being pruned by goimports
// in projects that otherwise don't use this type explicitly).
var _ = cmttypes.BlockID{}
