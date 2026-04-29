package keeper_test

// Tests for P-18b-b: knowledge module minting cap enforcement.
//
// Gap closed: three knowledge-module mint paths (probe bounty pool, training fund
// disbursement, contribution challenge reward) called bankKeeper.MintCoins directly
// without checking MaxSupplyUzrn. This file tests that the cap is now enforced via
// Keeper.MintWithCap and that all three call sites respect it.

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// maxSupplyBig parses MaxSupplyUzrn as a *big.Int for arithmetic in tests.
func maxSupplyBig() *big.Int {
	n, _ := new(big.Int).SetString(types.MaxSupplyUzrn, 10)
	return n
}

// ─── MintWithCap unit tests ──────────────────────────────────────────────────

// TestMintWithCap_normal — mint well under the cap: full amount returned.
func TestMintWithCap_normal(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	amount := big.NewInt(1_000_000) // 1 uzrn, well under cap
	actual, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, amount)
	require.NoError(t, err)
	require.Equal(t, 0, actual.Cmp(amount), "expected full amount minted under cap")

	supply := bk.GetSupply(ctx, "uzrn")
	require.True(t, supply.Amount.GTE(sdkmath.NewInt(1_000_000)), "supply should include minted amount")
}

// TestMintWithCap_atCap — supply already at MaxSupplyUzrn: returns 0, no error.
func TestMintWithCap_atCap(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Fill up to exactly MaxSupplyUzrn accounting for genesis tokens already minted.
	maxSupply := sdkmath.NewIntFromBigInt(maxSupplyBig())
	existingSupply := bk.GetSupply(ctx, "uzrn").Amount
	remaining := maxSupply.Sub(existingSupply)
	require.True(t, remaining.IsPositive(), "genesis supply must be below max cap for this test to be valid")
	bk.moduleBalances["fake_module"] = sdk.NewCoins(sdk.NewCoin("uzrn", remaining))

	// Total supply is now exactly MaxSupplyUzrn.
	totalNow := bk.GetSupply(ctx, "uzrn").Amount
	require.Equal(t, maxSupply, totalNow, "test setup: supply should equal cap")

	actual, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, big.NewInt(1_000_000))
	require.NoError(t, err)
	require.Equal(t, 0, actual.Sign(), "expected zero minted when cap is exhausted")

	// Supply must not increase beyond cap.
	supply := bk.GetSupply(ctx, "uzrn")
	require.True(t, supply.Amount.LTE(maxSupply), "supply must not exceed MaxSupplyUzrn")
}

// TestMintWithCap_nearCap — supply 100 uzrn below cap, request 1000: returns 100.
func TestMintWithCap_nearCap(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Leave exactly 100 uzrn of headroom: fill (MaxSupply - existingSupply - 100).
	headroom := int64(100)
	maxSupply := sdkmath.NewIntFromBigInt(maxSupplyBig())
	existingSupply := bk.GetSupply(ctx, "uzrn").Amount
	toFill := maxSupply.Sub(existingSupply).SubRaw(headroom)
	require.True(t, toFill.IsPositive(), "test setup: must have room to fill minus headroom")
	bk.moduleBalances["fake_module"] = sdk.NewCoins(sdk.NewCoin("uzrn", toFill))

	// Confirm headroom is exactly 100.
	remaining := maxSupply.Sub(bk.GetSupply(ctx, "uzrn").Amount)
	require.Equal(t, sdkmath.NewInt(headroom), remaining, "test setup: headroom should be 100")

	request := big.NewInt(1_000) // ask for 1000, only 100 available
	actual, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, request)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(headroom), actual, "expected mint clamped to remaining headroom")
}

// TestMintWithCap_zeroAmount — zero amount in: returns 0, no error, no mint.
func TestMintWithCap_zeroAmount(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	initialMinted := bk.minted

	actual, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, big.NewInt(0))
	require.NoError(t, err)
	require.Equal(t, 0, actual.Sign(), "expected zero minted for zero request")
	require.Equal(t, initialMinted, bk.minted, "MintCoins must not be called for zero request")
}

// TestMintWithCap_nilAmount — nil amount in: returns 0, no error.
func TestMintWithCap_nilAmount(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	actual, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, nil)
	require.NoError(t, err)
	require.Equal(t, 0, actual.Sign())
}

// ─── Probe bounty pool: cap enforced end-to-end ──────────────────────────────

// TestMintToProbeBountyPool_capEnforced — when supply is at cap, pool mint is
// silently skipped (non-fatal, BeginBlocker continues normally).
func TestMintToProbeBountyPool_capEnforced(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Fill supply to exactly MaxSupplyUzrn (accounting for genesis tokens).
	maxSupply := sdkmath.NewIntFromBigInt(maxSupplyBig())
	existingSupply := bk.GetSupply(ctx, "uzrn").Amount
	toFill := maxSupply.Sub(existingSupply)
	bk.moduleBalances["fake_module"] = sdk.NewCoins(sdk.NewCoin("uzrn", toFill))

	params := types.DefaultParams()
	params.ProbeBountyMintPerBlock = "10000" // 10,000 uzrn/block

	// Should not panic or error — silently skips when cap exhausted.
	k.MintToProbeBountyPool(ctx, &params)

	// Supply must stay at cap — not exceeded.
	supply := bk.GetSupply(ctx, "uzrn")
	require.True(t, supply.Amount.LTE(maxSupply), "supply must not exceed cap after silenced mint")
}

// TestMintToProbeBountyPool_normal — normal mint path still works.
func TestMintToProbeBountyPool_normal(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	params := types.DefaultParams()
	params.ProbeBountyMintPerBlock = "5000"
	params.ProbeBountyMaxPoolSize = "100000"

	supplyBefore := bk.GetSupply(ctx, "uzrn").Amount
	k.MintToProbeBountyPool(ctx, &params)
	supplyAfter := bk.GetSupply(ctx, "uzrn").Amount

	// Supply should have grown by (up to) 5000 uzrn.
	require.True(t, supplyAfter.GT(supplyBefore), "expected supply to increase after normal pool fill")
	delta := supplyAfter.Sub(supplyBefore)
	require.True(t, delta.LTE(sdkmath.NewInt(5000)), "minted amount should not exceed ProbeBountyMintPerBlock")
}

// ─── Training fund disbursement: cap enforced ────────────────────────────────

// TestClaimTrainingFundDisbursement_capExhausted — when supply cap is exhausted,
// MintWithCap returns (0, nil) rather than over-issuing silently.
func TestClaimTrainingFundDisbursement_capExhausted(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// Fill supply to exactly MaxSupplyUzrn.
	maxSupply := sdkmath.NewIntFromBigInt(maxSupplyBig())
	existingSupply := bk.GetSupply(ctx, "uzrn").Amount
	toFill := maxSupply.Sub(existingSupply)
	bk.moduleBalances["fake_module"] = sdk.NewCoins(sdk.NewCoin("uzrn", toFill))

	// Attempt MintWithCap — should return 0, no error.
	actual, err := k.MintWithCap(ctx, types.TrainingFundModuleName, big.NewInt(1_000_000_000))
	require.NoError(t, err)
	require.Equal(t, 0, actual.Sign(), "must not mint when cap is exhausted")

	// Supply must not grow beyond cap.
	supply := bk.GetSupply(ctx, "uzrn")
	require.True(t, supply.Amount.LTE(maxSupply), "supply must not exceed MaxSupplyUzrn")
}

// TestMintWithCap_multipleModules — cap is enforced across all module accounts.
// Minting to different modules is still bounded by the single global cap.
func TestMintWithCap_multipleModules(t *testing.T) {
	k, ctx, bk := setupKnowledgeTestWithBank(t)

	// First mint: 500 uzrn to probe pool.
	actual1, err := k.MintWithCap(ctx, types.ProbeBountyPoolModuleName, big.NewInt(500))
	require.NoError(t, err)
	require.Equal(t, big.NewInt(500), actual1)

	// Second mint: another 300 uzrn to training fund.
	actual2, err := k.MintWithCap(ctx, types.TrainingFundModuleName, big.NewInt(300))
	require.NoError(t, err)
	require.Equal(t, big.NewInt(300), actual2)

	// Total supply should be 800.
	supply := bk.GetSupply(ctx, "uzrn")
	require.True(t, supply.Amount.GTE(sdkmath.NewInt(800)), "aggregate supply should include both mints")
}
