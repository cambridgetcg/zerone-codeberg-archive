package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Ring buffer: recent block hashes ─────────────────────────────────────────
//
// Oracle Resilience Brief Gap 4: to verify oracle_request_id freshness, the
// chain stores a rolling window of recent block hashes. BeginBlock writes the
// previous block's header hash; reveals can then check that their oracle was
// queried within the last oracle_freshness_window_blocks blocks.

// SetRecentBlockHash stores the block hash for the given height.
// Called from BeginBlock: SetRecentBlockHash(ctx, height-1, lastBlockID.Hash).
func (k Keeper) SetRecentBlockHash(ctx context.Context, height uint64, hash []byte) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Set(types.RecentBlockHashKey(height), hash)
}

// GetRecentBlockHash retrieves the stored block hash for the given height.
// Returns (hash, true) if found, (nil, false) if not in the ring buffer.
func (k Keeper) GetRecentBlockHash(ctx context.Context, height uint64) ([]byte, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.RecentBlockHashKey(height))
	if err != nil || len(bz) == 0 {
		return nil, false
	}
	return bz, true
}

// DeleteRecentBlockHash removes the hash entry for the given height.
// Called from BeginBlock to evict entries that have fallen outside the window.
func (k Keeper) DeleteRecentBlockHash(ctx context.Context, height uint64) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.RecentBlockHashKey(height))
}

// WriteAndPruneRecentBlockHash is called from BeginBlock every block.
// It stores the previous block's header hash and evicts the entry that just
// fell outside the freshness window.
//
// Invariant: at the start of block N, the ring buffer contains hashes for
// blocks [N-1-window, N-1]. Reveals in block N can check against any of these.
func (k Keeper) WriteAndPruneRecentBlockHash(ctx context.Context, params *types.Params) {
	if params.OracleFreshnessWindowBlocks == 0 {
		return // gate disabled — don't maintain the ring buffer
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentHeight := uint64(sdkCtx.BlockHeight())
	if currentHeight < 2 {
		return // genesis / first block — no prev hash available
	}

	prevHeight := currentHeight - 1
	prevHash := sdkCtx.BlockHeader().LastBlockId.Hash
	if len(prevHash) > 0 {
		k.SetRecentBlockHash(ctx, prevHeight, prevHash)
	}

	// Evict the entry that just fell outside the window.
	// After writing height (N-1), the oldest we should keep is N-1-window.
	// So delete N-2-window = (currentHeight-2) - window.
	if currentHeight >= 2+params.OracleFreshnessWindowBlocks {
		evict := currentHeight - 2 - params.OracleFreshnessWindowBlocks
		k.DeleteRecentBlockHash(ctx, evict)
	}
}

// ── Oracle request ID ─────────────────────────────────────────────────────────

// ComputeOracleRequestId computes the canonical oracle request fingerprint:
//
//	sha256(claim_id + "|" + hex(block_hash) + "|" + verifier_addr)
//
// This is the same formula the oracle process uses when returning a request_id
// in its EvaluateResponse. The hex encoding uses lowercase digits.
func ComputeOracleRequestId(claimID string, blockHash []byte, verifierAddr string) string {
	h := sha256.New()
	h.Write([]byte(claimID))
	h.Write([]byte("|"))
	h.Write([]byte(hex.EncodeToString(blockHash)))
	h.Write([]byte("|"))
	h.Write([]byte(verifierAddr))
	return hex.EncodeToString(h.Sum(nil))
}

// CheckOracleFreshness verifies that msg.OracleRequestId was computed using a
// block hash from within the last params.OracleFreshnessWindowBlocks blocks.
//
// Returns nil (pass) when:
//   - params.OracleFreshnessWindowBlocks == 0 (gate disabled)
//   - msg.OracleRequestId is empty (verifier opted out)
//   - A matching stored hash is found in the ring buffer
//   - The ring buffer contains NO stored hashes at all (fail-open for fresh chains
//     and restarts: if there's nothing to check against, don't block)
//
// Returns ErrStaleOracleRequest when:
//   - The ring buffer has at least one stored hash AND none match the request_id
func (k Keeper) CheckOracleFreshness(
	ctx context.Context,
	claimID string,
	verifierAddr string,
	oracleRequestId string,
	params *types.Params,
) error {
	if params.OracleFreshnessWindowBlocks == 0 {
		return nil // gate disabled
	}
	if oracleRequestId == "" {
		return nil // verifier did not include a request_id — fail-open
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentHeight := uint64(sdkCtx.BlockHeight())

	// Scan the ring buffer: check hashes for blocks in [currentHeight-window, currentHeight-1].
	var anyHashFound bool
	window := params.OracleFreshnessWindowBlocks
	start := uint64(0)
	if currentHeight > window {
		start = currentHeight - window
	}

	for h := start; h < currentHeight; h++ {
		blockHash, found := k.GetRecentBlockHash(ctx, h)
		if !found {
			continue
		}
		anyHashFound = true
		expected := ComputeOracleRequestId(claimID, blockHash, verifierAddr)
		if expected == oracleRequestId {
			return nil // freshness verified
		}
	}

	if !anyHashFound {
		// Ring buffer is empty (chain just started or restarted within window) — fail-open.
		return nil
	}

	// Hashes were present but none matched.
	return types.ErrStaleOracleRequest.Wrapf(
		"oracle_request_id %q does not match any of the last %d block hashes for claim %q / verifier %q",
		oracleRequestId, window, claimID, verifierAddr,
	)
}

// ── BeginBlock helper: uint64 big-endian ─────────────────────────────────────

// blockHeightFromKey extracts the uint64 height from a RecentBlockHashKey.
// For diagnostic use only. Not called in hot paths.
func blockHeightFromKey(key []byte) uint64 {
	if len(key) < 9 {
		return 0
	}
	return binary.BigEndian.Uint64(key[1:9])
}
