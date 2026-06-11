package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/billing/keeper"
	"github.com/zerone-chain/zerone/x/billing/types"
)

// ============================================================
// QueryProtocolReserves — Treasury Gap 5
// ============================================================
//
// Aggregates all protocol fund balances into one read-only query so
// governance participants don't need seven separate balance lookups.

// seedModuleBalance seeds a module account in the mock bank keeper.
func seedModuleBalance(bk *mockBankKeeper, module string, amountUzrn int64) {
	key := module + "/uzrn"
	bk.balances[key] = sdkmath.NewInt(amountUzrn)
}

// TestProtocolReserves_AllFundsPresent verifies that each of the seven
// module accounts is reported correctly when all have positive balances.
func TestProtocolReserves_AllFundsPresent(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	seedModuleBalance(bk, "treasury_protocol", 1_000_000)
	seedModuleBalance(bk, "protocol_treasury", 2_000_000)
	seedModuleBalance(bk, "research_fund", 3_000_000)
	seedModuleBalance(bk, "development_fund", 4_000_000)
	seedModuleBalance(bk, "vindication_escrow", 500_000)
	seedModuleBalance(bk, "knowledge", 750_000)
	seedModuleBalance(bk, "disputes", 250_000)

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.ProtocolReserves(ctx, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)

	require.Equal(t, "1000000", resp.TreasuryProtocolUzrn)
	require.Equal(t, "2000000", resp.ProtocolTreasuryUzrn)
	require.Equal(t, "3000000", resp.ResearchFundUzrn)
	require.Equal(t, "4000000", resp.DevelopmentFundUzrn)
	require.Equal(t, "500000", resp.VindicationEscrowUzrn)
	require.Equal(t, "750000", resp.KnowledgePoolUzrn)
	require.Equal(t, "250000", resp.DisputesEscrowUzrn)
	require.Equal(t, uint64(100), resp.SnapshotBlockHeight) // ctx is at height 100
}

// TestProtocolReserves_EmptyFunds verifies that uninitialized module accounts
// report "0" rather than an error or empty string.
func TestProtocolReserves_EmptyFunds(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	// No balances seeded — all should read as 0.

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.ProtocolReserves(ctx, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)

	require.Equal(t, "0", resp.TreasuryProtocolUzrn, "treasury_protocol should be 0 when empty")
	require.Equal(t, "0", resp.ProtocolTreasuryUzrn, "protocol_treasury should be 0 when empty")
	require.Equal(t, "0", resp.ResearchFundUzrn, "research_fund should be 0 when empty")
	require.Equal(t, "0", resp.DevelopmentFundUzrn, "development_fund should be 0 when empty")
	require.Equal(t, "0", resp.VindicationEscrowUzrn, "vindication_escrow should be 0 when empty")
	require.Equal(t, "0", resp.KnowledgePoolUzrn, "knowledge should be 0 when empty")
	require.Equal(t, "0", resp.DisputesEscrowUzrn, "disputes should be 0 when empty")
}

// TestProtocolReserves_PartialFunds verifies that the query handles a mix of
// funded and unfunded accounts (typical early-stage protocol state).
func TestProtocolReserves_PartialFunds(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	// Only development_fund and research_fund have balances.
	seedModuleBalance(bk, "development_fund", 100_000_000)
	seedModuleBalance(bk, "research_fund", 50_000_000)

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.ProtocolReserves(ctx, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)

	require.Equal(t, "0", resp.TreasuryProtocolUzrn)
	require.Equal(t, "0", resp.ProtocolTreasuryUzrn)
	require.Equal(t, "50000000", resp.ResearchFundUzrn)
	require.Equal(t, "100000000", resp.DevelopmentFundUzrn)
	require.Equal(t, "0", resp.VindicationEscrowUzrn)
	require.Equal(t, "0", resp.KnowledgePoolUzrn)
	require.Equal(t, "0", resp.DisputesEscrowUzrn)
}

// TestProtocolReserves_SnapshotBlockHeight verifies that SnapshotBlockHeight
// reflects the actual context block height, not a hardcoded value.
func TestProtocolReserves_SnapshotBlockHeight(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	// Default ctx is height 100; advance it.
	ctx100 := ctx
	ctx500 := ctx.WithBlockHeight(500)

	qs := keeper.NewQueryServerImpl(k)

	resp100, err := qs.ProtocolReserves(ctx100, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(100), resp100.SnapshotBlockHeight)

	resp500, err := qs.ProtocolReserves(ctx500, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(500), resp500.SnapshotBlockHeight)
}

// TestProtocolReserves_NilRequest verifies that a nil request returns an error.
func TestProtocolReserves_NilRequest(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	_, err := qs.ProtocolReserves(ctx, nil)
	require.Error(t, err, "nil request should return error")
}

// TestProtocolReserves_DistinctAccounts verifies that "treasury_protocol" and
// "protocol_treasury" are reported as separate accounts with independent balances,
// documenting the known naming inconsistency.
func TestProtocolReserves_DistinctAccounts(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	seedModuleBalance(bk, "treasury_protocol", 111_000)
	seedModuleBalance(bk, "protocol_treasury", 222_000)

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.ProtocolReserves(ctx, &types.QueryProtocolReservesRequest{})
	require.NoError(t, err)

	// The two treasury accounts are distinct — different balances prove they're different keys.
	require.Equal(t, "111000", resp.TreasuryProtocolUzrn, "billing treasury_protocol")
	require.Equal(t, "222000", resp.ProtocolTreasuryUzrn, "knowledge/toolbox protocol_treasury")
	require.NotEqual(t, resp.TreasuryProtocolUzrn, resp.ProtocolTreasuryUzrn,
		"the two protocol treasury accounts are distinct — this is a known naming inconsistency")
}
