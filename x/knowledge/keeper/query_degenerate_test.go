package keeper_test

// DegeneratingProgrammes query — Falsificationism Gap 3 (query surface).
//
// The degeneration signal already exists on-chain via Claim.ResearchProgrammeDepth
// and the zerone.knowledge.degenerating_research_programme event. The gap this
// test suite closes: until now, discovering which claims are deeply entangled in
// rescue chains required scanning ALL claims. DegeneratingProgrammes exposes a
// query surface that governance tooling can call directly.
//
// Coverage matrix:
//   1. Empty corpus → empty result list, threshold echoed.
//   2. Single deep claim, threshold=0 → returned (depth > 0 is the gate when
//      threshold=0 in both request and params).
//   3. Threshold from Params used when request.MinDepthOverride=0.
//   4. Request min_depth_override beats Params threshold (explicit override).
//   5. Domain filter: only matching domain returned.
//   6. Domain filter: empty = all domains.
//   7. Results sorted deepest first.
//   8. Limit applied (cap at request.Limit).
//   9. Limit=0 → default cap (100) applied.
//  10. Limit > 500 → hard cap at 500.
//  11. depth=0 claims (no rescue parent) never returned.
//  12. SnapshotBlockHeight is populated from block context.
//  13. DegenerationThreshold in response echoes effective threshold.
//  14. FactContentPreview truncated at 256 chars.
//  15. All claim statuses included (not filtered by status).

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// storeClaimAtDepth stores a claim with the given depth and domain directly via
// keeper.SetClaim — bypasses msg_server so we control depth precisely.
func storeClaimAtDepth(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	id string,
	depth uint64,
	domain string,
	rescuesID string,
	status types.ClaimStatus,
	content string,
) {
	t.Helper()
	if content == "" {
		content = "claim content for " + id
	}
	claim := &types.Claim{
		Id:                     id,
		FactContent:            content,
		Domain:                 domain,
		Category:               "test",
		Submitter:              makeValidBech32Addr("degen-sub-" + id),
		SubmittedAtBlock:       1,
		Status:                 status,
		Stake:                  "1000uzrn",
		ResearchProgrammeDepth: depth,
		RescuesClaimId:         rescuesID,
	}
	require.NoError(t, k.SetClaim(ctx, claim))
}

// setupDegenQuery returns a queryServer backed by a fresh test keeper.
func setupDegenQuery(t *testing.T) (types.QueryServer, keeper.Keeper, sdk.Context) {
	t.Helper()
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	qs := keeper.NewQueryServerImpl(k)
	return qs, k, ctx
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// 1. Empty corpus → no results, threshold echoed.
func TestDegeneratingProgrammes_EmptyCorpus(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	// Set a non-zero threshold so we can verify it echoes.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 3
	require.NoError(t, k.SetParams(ctx, params))

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Empty(t, resp.Programmes, "empty corpus must return no programmes")
	require.Equal(t, uint64(3), resp.DegenerationThreshold, "response must echo effective threshold")
}

// 2. Single deep claim, threshold=0 in both request and params → returned
// because gate is depth > 0.
func TestDegeneratingProgrammes_ThresholdZero_AllDepthReturned(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	// Keep params threshold = 0 (default).
	storeClaimAtDepth(t, k, ctx, "rescue-1", 2, "physics", "parent-1",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	// depth=0 claim — must NOT appear.
	storeClaimAtDepth(t, k, ctx, "root-1", 0, "physics", "",
		types.ClaimStatus_CLAIM_STATUS_ACCEPTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 1)
	require.Equal(t, "rescue-1", resp.Programmes[0].ClaimId)
	require.Equal(t, uint64(0), resp.DegenerationThreshold,
		"threshold=0: any depth > 0 included, echoed threshold stays 0")
}

// 3. Threshold from Params used when request.MinDepthOverride=0.
func TestDegeneratingProgrammes_UsesParamsThreshold(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 5
	require.NoError(t, k.SetParams(ctx, params))

	// Depth 3 — below threshold → excluded.
	storeClaimAtDepth(t, k, ctx, "shallow", 3, "bio", "p0",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	// Depth 5 — equal to threshold → included (>= floor).
	storeClaimAtDepth(t, k, ctx, "at-threshold", 5, "bio", "p1",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	// Depth 7 — above threshold → included.
	storeClaimAtDepth(t, k, ctx, "deep", 7, "bio", "p2",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, p := range resp.Programmes {
		ids[p.ClaimId] = true
	}
	require.True(t, ids["at-threshold"], "depth==threshold (5): must be included")
	require.True(t, ids["deep"], "depth>threshold (7): must be included")
	require.False(t, ids["shallow"], "depth<threshold (3): must be excluded")
	require.Equal(t, uint64(5), resp.DegenerationThreshold)
}

// 4. Request MinDepthOverride beats Params threshold.
func TestDegeneratingProgrammes_OverrideBeatsParams(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	// Params threshold = 2 (would include depth>=2).
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 2
	require.NoError(t, k.SetParams(ctx, params))

	storeClaimAtDepth(t, k, ctx, "d2", 2, "chem", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "d6", 6, "chem", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	// Override to 5 — only d6 should appear.
	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{
		MinDepthOverride: 5,
	})
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, p := range resp.Programmes {
		ids[p.ClaimId] = true
	}
	require.False(t, ids["d2"], "d2 (depth=2) excluded by override=5")
	require.True(t, ids["d6"], "d6 (depth=6) included by override=5")
	require.Equal(t, uint64(5), resp.DegenerationThreshold, "response echoes override, not params")
}

// 5. Domain filter: only matching domain returned.
func TestDegeneratingProgrammes_DomainFilter(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	storeClaimAtDepth(t, k, ctx, "physics-1", 3, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "chem-1", 4, "chemistry", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "physics-2", 5, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{
		DomainFilter: "physics",
	})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 2)
	for _, p := range resp.Programmes {
		require.Equal(t, "physics", p.Domain, "only physics claims should appear")
	}
}

// 6. Domain filter empty → all domains included.
func TestDegeneratingProgrammes_DomainFilterEmpty_AllDomains(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	storeClaimAtDepth(t, k, ctx, "a", 2, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "b", 3, "chemistry", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "c", 4, "biology", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 3, "all domains returned when filter is empty")
}

// 7. Results sorted deepest first.
func TestDegeneratingProgrammes_SortedDeepestFirst(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	storeClaimAtDepth(t, k, ctx, "d1", 1, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "d9", 9, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "d5", 5, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	storeClaimAtDepth(t, k, ctx, "d3", 3, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 4)

	depths := make([]uint64, len(resp.Programmes))
	for i, p := range resp.Programmes {
		depths[i] = p.ResearchProgrammeDepth
	}
	require.Equal(t, uint64(9), depths[0], "deepest first: 9")
	require.Equal(t, uint64(5), depths[1], "second: 5")
	require.Equal(t, uint64(3), depths[2], "third: 3")
	require.Equal(t, uint64(1), depths[3], "shallowest last: 1")
}

// 8. Limit applied: request.Limit=2 caps at 2 results (deepest).
func TestDegeneratingProgrammes_LimitApplied(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	for i := uint64(1); i <= 5; i++ {
		id := strings.Repeat("x", int(i)) // unique IDs by content
		storeClaimAtDepth(t, k, ctx, id, i, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	}

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{
		Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 2, "limit=2 → at most 2 results")
	// Deepest first after sort+truncate → top 2 should be depth=5 and depth=4.
	require.Equal(t, uint64(5), resp.Programmes[0].ResearchProgrammeDepth)
	require.Equal(t, uint64(4), resp.Programmes[1].ResearchProgrammeDepth)
}

// 9. Limit=0 → default cap (100): with 5 claims, all 5 returned.
func TestDegeneratingProgrammes_DefaultLimit(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	for i := 1; i <= 5; i++ {
		id := strings.Repeat("a", i)
		storeClaimAtDepth(t, k, ctx, id, uint64(i+1), "physics", "p",
			types.ClaimStatus_CLAIM_STATUS_REJECTED, "")
	}

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{Limit: 0})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 5,
		"limit=0 uses default (100); 5 claims < 100 so all returned")
}

// 10. depth=0 claims never returned regardless of threshold.
func TestDegeneratingProgrammes_DepthZeroExcluded(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	// All root claims (no rescue parent) at depth=0.
	for i := 0; i < 5; i++ {
		id := strings.Repeat("r", i+1)
		storeClaimAtDepth(t, k, ctx, id, 0, "physics", "",
			types.ClaimStatus_CLAIM_STATUS_ACCEPTED, "")
	}

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Empty(t, resp.Programmes, "depth=0 claims (root claims) must never appear")
}

// 11. SnapshotBlockHeight is populated from block context.
func TestDegeneratingProgrammes_SnapshotBlockHeight(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	storeClaimAtDepth(t, k, ctx, "d3", 3, "physics", "p", types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	// Set a specific block height on the context.
	ctx = ctx.WithBlockHeight(42)
	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	_ = k // suppress unused
	require.Equal(t, uint64(42), resp.SnapshotBlockHeight,
		"SnapshotBlockHeight must echo the block context")
}

// 12. DegenerationThreshold in response echoes the effective threshold used.
func TestDegeneratingProgrammes_ThresholdEchoed(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.ResearchProgrammeDegenerationThreshold = 7
	require.NoError(t, k.SetParams(ctx, params))

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(7), resp.DegenerationThreshold,
		"response must echo params threshold when no override set")

	// With override: echoes override, not params.
	resp2, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{
		MinDepthOverride: 3,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), resp2.DegenerationThreshold,
		"response must echo override when set")
}

// 13. FactContentPreview truncated at 256 chars.
func TestDegeneratingProgrammes_ContentPreviewTruncated(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	longContent := strings.Repeat("x", 512) // 512 chars
	storeClaimAtDepth(t, k, ctx, "long-claim", 3, "physics", "p",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, longContent)

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 1)
	require.Equal(t, 256, len(resp.Programmes[0].FactContentPreview),
		"preview must be truncated to 256 chars")
}

// 14. Short content preserved verbatim (no truncation).
func TestDegeneratingProgrammes_ShortContentNotTruncated(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	shortContent := "A brief rescue claim."
	storeClaimAtDepth(t, k, ctx, "short-claim", 2, "chemistry", "p",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, shortContent)

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 1)
	require.Equal(t, shortContent, resp.Programmes[0].FactContentPreview,
		"short content must be preserved verbatim, not padded or truncated")
}

// 15. All claim statuses included (not filtered by status).
func TestDegeneratingProgrammes_AllStatusesIncluded(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	statuses := []types.ClaimStatus{
		types.ClaimStatus_CLAIM_STATUS_PENDING,
		types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		types.ClaimStatus_CLAIM_STATUS_ACCEPTED,
		types.ClaimStatus_CLAIM_STATUS_REJECTED,
	}
	for i, st := range statuses {
		id := strings.Repeat("s", i+1)
		storeClaimAtDepth(t, k, ctx, id, uint64(i+2), "physics", "p", st, "")
	}

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 4,
		"all claim statuses must be included — degeneration signal is status-independent")
}

// 16. RescuesClaimId propagated correctly in summary.
func TestDegeneratingProgrammes_RescuesClaimIdPropagated(t *testing.T) {
	qs, k, ctx := setupDegenQuery(t)

	storeClaimAtDepth(t, k, ctx, "child-claim", 3, "physics", "parent-claim-id",
		types.ClaimStatus_CLAIM_STATUS_REJECTED, "")

	resp, err := qs.DegeneratingProgrammes(ctx, &types.QueryDegeneratingProgrammesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Programmes, 1)
	require.Equal(t, "parent-claim-id", resp.Programmes[0].RescuesClaimId,
		"RescuesClaimId must identify the direct rescue parent")
}

// 17. Nil request treated as empty request (no panic).
func TestDegeneratingProgrammes_NilRequest(t *testing.T) {
	qs, _, ctx := setupDegenQuery(t)

	resp, err := qs.DegeneratingProgrammes(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, resp.Programmes, "nil request = empty request, returns empty result")
}
