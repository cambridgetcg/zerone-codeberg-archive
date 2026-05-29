package keeper_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/tokens/types"
)

// -----------------------------------------------------------------------
// Gap 4: Hard ZRN Supply Cap Tests
// -----------------------------------------------------------------------

// TestSupplyCap_Disabled: max_native_supply_uzrn="" → unlimited, mints normally.
func TestSupplyCap_Disabled(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Default params have no cap
	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "cap_disabled",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "1000",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx) // block 100

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	if !coin.Amount.Equal(sdkmath.NewInt(1000)) {
		t.Fatalf("expected 1000 uzrn minted with no cap, got %s", coin.Amount)
	}

	em := k.GetEmissionPeriod(ctx, "cap_disabled")
	if !em.Active {
		t.Fatal("emission should still be active")
	}
}

// TestSupplyCap_BelowCap: supply well below cap → mints normally.
func TestSupplyCap_BelowCap(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Set cap to 1,000,000 uzrn; current supply is 0
	params := k.GetParams(ctx)
	params.MaxNativeSupplyUzrn = "1000000"
	k.SetParams(ctx, params)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "cap_below",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "500",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx)

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	if !coin.Amount.Equal(sdkmath.NewInt(500)) {
		t.Fatalf("expected 500 uzrn, got %s", coin.Amount)
	}
}

// TestSupplyCap_AtCap: supply already at cap → skip and deactivate.
func TestSupplyCap_AtCap(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Set cap to 1000. Pre-fund recipient so GetSupply returns 1000.
	params := k.GetParams(ctx)
	params.MaxNativeSupplyUzrn = "1000"
	k.SetParams(ctx, params)

	// Inject supply by pre-minting into the mock bank directly
	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[recipientAddr.String()] = map[string]sdkmath.Int{
		"uzrn": sdkmath.NewInt(1000),
	}

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "cap_at",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "500",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx)

	// Balance must not increase
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	if !coin.Amount.Equal(sdkmath.NewInt(1000)) {
		t.Fatalf("expected balance unchanged at 1000, got %s", coin.Amount)
	}

	// Period must be deactivated
	em := k.GetEmissionPeriod(ctx, "cap_at")
	if em.Active {
		t.Fatal("emission should be deactivated when at cap")
	}
}

// TestSupplyCap_PartialMint: amount would exceed cap → clamp to headroom + deactivate.
func TestSupplyCap_PartialMint(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Cap=1000, current supply=700, amount_per_block=500 → only 300 should mint
	params := k.GetParams(ctx)
	params.MaxNativeSupplyUzrn = "1000"
	k.SetParams(ctx, params)

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[recipientAddr.String()] = map[string]sdkmath.Int{
		"uzrn": sdkmath.NewInt(700),
	}

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "cap_partial",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "500",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx)

	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	if !coin.Amount.Equal(sdkmath.NewInt(1000)) {
		t.Fatalf("expected balance clamped to 1000 (700+300), got %s", coin.Amount)
	}

	em := k.GetEmissionPeriod(ctx, "cap_partial")
	if em.Active {
		t.Fatal("emission should be deactivated after partial mint fills cap")
	}
}

// TestSupplyCap_Validate_InvalidString: invalid MaxNativeSupplyUzrn is rejected by Validate.
func TestSupplyCap_Validate_InvalidString(t *testing.T) {
	p := types.DefaultParams()
	p.MaxNativeSupplyUzrn = "not-a-number"
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error for non-integer max_native_supply_uzrn")
	}
}

// TestSupplyCap_Validate_Negative: negative MaxNativeSupplyUzrn is rejected.
func TestSupplyCap_Validate_Negative(t *testing.T) {
	p := types.DefaultParams()
	p.MaxNativeSupplyUzrn = "-1000"
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error for negative max_native_supply_uzrn")
	}
}

// TestSupplyCap_Validate_ZeroIsUnlimited: "0" is valid (treated as unlimited).
func TestSupplyCap_Validate_ZeroIsUnlimited(t *testing.T) {
	p := types.DefaultParams()
	p.MaxNativeSupplyUzrn = "0"
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid for max_native_supply_uzrn=0: %v", err)
	}
}

// TestSupplyCap_Validate_EmptyIsUnlimited: "" is valid (unlimited).
func TestSupplyCap_Validate_EmptyIsUnlimited(t *testing.T) {
	p := types.DefaultParams()
	p.MaxNativeSupplyUzrn = ""
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid for empty max_native_supply_uzrn: %v", err)
	}
}

// -----------------------------------------------------------------------
// Gap 3: Emission Halving Tests
// -----------------------------------------------------------------------

// TestHalving_Disabled_NoInterval: halving_interval_blocks=0 → no halving, normal emission.
func TestHalving_Disabled_NoInterval(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_none",
		StartBlock:            100,
		EndBlock:              300,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 0, // disabled
		HalvingFactorBps:      5000,
	})

	// Run many blocks — amount should never change
	for h := int64(100); h <= 110; h++ {
		k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: h}))
	}

	em := k.GetEmissionPeriod(ctx, "halv_none")
	if em.AmountPerBlock != "1000" {
		t.Fatalf("expected amount_per_block unchanged at 1000, got %s", em.AmountPerBlock)
	}
	if em.HalvingCount != 0 {
		t.Fatalf("expected 0 halvings applied, got %d", em.HalvingCount)
	}

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	// 11 blocks × 1000 = 11000
	if !coin.Amount.Equal(sdkmath.NewInt(11000)) {
		t.Fatalf("expected 11000 minted across 11 blocks, got %s", coin.Amount)
	}
}

// TestHalving_FirstHalvingFires: fires at startBlock + halvingIntervalBlocks.
func TestHalving_FirstHalvingFires(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Start=100, interval=10, factor=5000 (50% reduction)
	// First halving at block 100+10=110 → amount becomes 500
	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_first",
		StartBlock:            100,
		EndBlock:              300,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      5000, // 50%
	})

	// Blocks 100-109: no halving yet (blocksSinceStart < interval)
	for h := int64(100); h <= 109; h++ {
		k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: h}))
	}
	em := k.GetEmissionPeriod(ctx, "halv_first")
	if em.AmountPerBlock != "1000" {
		t.Fatalf("expected 1000 before first halving, got %s", em.AmountPerBlock)
	}
	if em.HalvingCount != 0 {
		t.Fatalf("expected 0 halvings before interval, got %d", em.HalvingCount)
	}

	// Block 110: first halving fires (blocksSinceStart=10, expectedHalvings=1)
	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 110}))
	em = k.GetEmissionPeriod(ctx, "halv_first")
	if em.AmountPerBlock != "500" {
		t.Fatalf("expected 500 after first halving, got %s", em.AmountPerBlock)
	}
	if em.HalvingCount != 1 {
		t.Fatalf("expected 1 halving applied, got %d", em.HalvingCount)
	}

	// Check minted: 10 blocks × 1000 + 1 block × 500 = 10500
	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	if !coin.Amount.Equal(sdkmath.NewInt(10500)) {
		t.Fatalf("expected 10500 minted, got %s", coin.Amount)
	}
}

// TestHalving_MultipleHalvings: second halving fires at startBlock + 2×interval.
func TestHalving_MultipleHalvings(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_multi",
		StartBlock:            100,
		EndBlock:              500,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      5000,
	})

	// Block 110: first halving → 500
	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 110}))
	em := k.GetEmissionPeriod(ctx, "halv_multi")
	if em.AmountPerBlock != "500" || em.HalvingCount != 1 {
		t.Fatalf("after 1st halving: amount=%s count=%d", em.AmountPerBlock, em.HalvingCount)
	}

	// Block 119: still 500
	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 119}))
	em = k.GetEmissionPeriod(ctx, "halv_multi")
	if em.AmountPerBlock != "500" {
		t.Fatalf("expected 500 before second halving, got %s", em.AmountPerBlock)
	}

	// Block 120: second halving → 250
	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 120}))
	em = k.GetEmissionPeriod(ctx, "halv_multi")
	if em.AmountPerBlock != "250" || em.HalvingCount != 2 {
		t.Fatalf("after 2nd halving: amount=%s count=%d", em.AmountPerBlock, em.HalvingCount)
	}

	_ = bk // suppress unused warning
}

// TestHalving_ZeroFactor_Disabled: halving_factor_bps=0 → no halving applied.
func TestHalving_ZeroFactor_Disabled(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_zfac",
		StartBlock:            100,
		EndBlock:              300,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      0, // disabled — zero factor
	})

	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 110}))
	em := k.GetEmissionPeriod(ctx, "halv_zfac")
	if em.AmountPerBlock != "1000" {
		t.Fatalf("expected amount unchanged with factor=0, got %s", em.AmountPerBlock)
	}
}

// TestHalving_AmountReducesToZero_Deactivates: when halved amount rounds to 0, period deactivates.
func TestHalving_AmountReducesToZero_Deactivates(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	// amount=1, factor=3000 (30%) → 1 * 3000 / 10000 = 0 → deactivate
	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_zero",
		StartBlock:            100,
		EndBlock:              300,
		AmountPerBlock:        "1",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      3000, // 30% — 1 * 0.3 = 0 integer
	})

	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 110}))
	em := k.GetEmissionPeriod(ctx, "halv_zero")
	if em.Active {
		t.Fatal("expected period to be deactivated when halved amount reduces to 0")
	}
}

// TestHalving_Genesis_RoundTrip: halving fields survive export + import.
func TestHalving_Genesis_RoundTrip(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_gen",
		StartBlock:            100,
		EndBlock:              500,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 50,
		HalvingFactorBps:      7500,
		HalvingCount:          3,
	})

	exported := k.ExportGenesisJSON(ctx)

	k2, ctx2 := setupKeeper(t)
	k2.InitGenesisEmissions(ctx2, exported)

	em := k2.GetEmissionPeriod(ctx2, "halv_gen")
	if em == nil {
		t.Fatal("emission period not found after genesis import")
	}
	if em.HalvingIntervalBlocks != 50 {
		t.Fatalf("HalvingIntervalBlocks: want 50 got %d", em.HalvingIntervalBlocks)
	}
	if em.HalvingFactorBps != 7500 {
		t.Fatalf("HalvingFactorBps: want 7500 got %d", em.HalvingFactorBps)
	}
	if em.HalvingCount != 3 {
		t.Fatalf("HalvingCount: want 3 got %d", em.HalvingCount)
	}
	if em.AmountPerBlock != "1000" {
		t.Fatalf("AmountPerBlock: want 1000 got %s", em.AmountPerBlock)
	}
}

// TestHalvingAndCap_Combined: halving fires first (reducing amount), then cap check applies.
func TestHalvingAndCap_Combined(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Cap=600, current supply=500, halving at block 110 reduces 1000→500.
	// After halving: amount=500; supply=500; headroom=100.
	// So only 100 should mint (partial mint), then deactivate.
	params := k.GetParams(ctx)
	params.MaxNativeSupplyUzrn = "600"
	k.SetParams(ctx, params)

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[recipientAddr.String()] = map[string]sdkmath.Int{
		"uzrn": sdkmath.NewInt(500),
	}

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_cap",
		StartBlock:            100,
		EndBlock:              300,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      5000, // 50% → 500
	})

	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 110}))

	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	// Expected: supply was 500, cap=600, headroom=100; halveed amount=500 > headroom → clamp to 100
	expected := sdkmath.NewInt(600)
	if !coin.Amount.Equal(expected) {
		t.Fatalf("expected balance %s, got %s", expected, coin.Amount)
	}

	em := k.GetEmissionPeriod(ctx, "halv_cap")
	if em.Active {
		t.Fatal("expected period deactivated after cap fill")
	}

	// Verify halving was applied (HalvingCount incremented)
	if em.HalvingCount != 1 {
		t.Fatalf("expected halving_count=1, got %d", em.HalvingCount)
	}
}

// TestSupplyCap_DefaultParams: verify default params have no cap.
func TestSupplyCap_DefaultParams(t *testing.T) {
	p := types.DefaultParams()
	if p.MaxNativeSupplyUzrn != "" {
		t.Fatalf("expected empty MaxNativeSupplyUzrn by default, got %q", p.MaxNativeSupplyUzrn)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("default params should be valid: %v", err)
	}
}

// TestHalving_CatchupOnMissedBlocks: if emission starts past interval boundary, all pending halvings applied.
func TestHalving_CatchupOnMissedBlocks(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	// Period starts at block 100, interval=10. First block processed is 130.
	// expectedHalvings = (130-100)/10 = 3 → three halvings applied: 1000→500→250→125
	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:                    "halv_catchup",
		StartBlock:            100,
		EndBlock:              500,
		AmountPerBlock:        "1000",
		Recipient:             testCreator,
		Active:                true,
		HalvingIntervalBlocks: 10,
		HalvingFactorBps:      5000,
	})

	k.BeginBlocker(ctx.WithBlockHeader(cmtproto.Header{Height: 130}))

	em := k.GetEmissionPeriod(ctx, "halv_catchup")
	expectedAmount := new(big.Int).SetInt64(125) // 1000 → 500 → 250 → 125
	actualAmount, _ := new(big.Int).SetString(em.AmountPerBlock, 10)
	if actualAmount.Cmp(expectedAmount) != 0 {
		t.Fatalf("expected amount_per_block=%s after 3 halvings, got %s", expectedAmount, em.AmountPerBlock)
	}
	if em.HalvingCount != 3 {
		t.Fatalf("expected halving_count=3, got %d", em.HalvingCount)
	}
}

// -----------------------------------------------------------------------
// Gap 2: Adaptive Emission Rate Tests
// -----------------------------------------------------------------------

// mockStakingKeeper implements types.StakingKeeper with a configurable bonded total.
type mockStakingKeeper struct {
	totalBonded *big.Int
}

func (m *mockStakingKeeper) GetTotalBondedStake(_ sdk.Context) *big.Int {
	if m.totalBonded == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(m.totalBonded)
}

// setupKeeperWithBankAndStaking wraps setupKeeperWithBank and attaches a mock staking keeper.
func setupKeeperWithBankAndStaking(t *testing.T, totalBonded int64) (interface{ GetEmissionScale(sdk.Context) uint64; SetEmissionScale(sdk.Context, uint64); BeginBlocker(sdk.Context); SetParams(sdk.Context, *types.Params); GetParams(sdk.Context) *types.Params; SetEmissionPeriod(sdk.Context, *types.EmissionPeriod); GetEmissionPeriod(sdk.Context, string) *types.EmissionPeriod }, sdk.Context, *mockBankKeeper) {
	t.Helper()
	k, ctx, bk := setupKeeperWithBank(t)
	sk := &mockStakingKeeper{totalBonded: big.NewInt(totalBonded)}
	k.SetStakingKeeper(sk)
	return &k, ctx, bk
}

// TestEmissionScale_DefaultIsIdentity: unset scale returns 1_000_000.
func TestEmissionScale_DefaultIsIdentity(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	scale := k.GetEmissionScale(ctx)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected default scale %d, got %d", types.DefaultEmissionScale, scale)
	}
}

// TestEmissionScale_SetGet: round-trip.
func TestEmissionScale_SetGet(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)

	k.SetEmissionScale(ctx, 750_000) // 0.75×
	got := k.GetEmissionScale(ctx)
	if got != 750_000 {
		t.Fatalf("expected 750000, got %d", got)
	}

	k.SetEmissionScale(ctx, 1_500_000) // 1.5×
	got = k.GetEmissionScale(ctx)
	if got != 1_500_000 {
		t.Fatalf("expected 1500000, got %d", got)
	}
}

// TestAdaptiveEmission_DisabledWhenTargetZero: target=0 → scale never adjusts.
func TestAdaptiveEmission_DisabledWhenTargetZero(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)} // 10% bonded
	k.SetStakingKeeper(sk)

	// Set large supply via bank pre-fund so ratio = 100k/1M = 10%
	// Target=0 means disabled — scale must not change.
	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 0            // disabled
	params.EmissionAdjustmentBps = 1_000
	params.MinEmissionScaleBps = 500_000
	params.MaxEmissionScaleBps = 2_000_000
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected scale unchanged at %d, got %d", types.DefaultEmissionScale, scale)
	}
}

// TestAdaptiveEmission_DisabledWhenAdjustmentZero: adjustment=0 → scale never adjusts.
func TestAdaptiveEmission_DisabledWhenAdjustmentZero(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	// Pre-fund to set supply
	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)}
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000     // 67% target
	params.EmissionAdjustmentBps = 0           // disabled
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected scale unchanged at %d, got %d", types.DefaultEmissionScale, scale)
	}
}

// TestAdaptiveEmission_DisabledWhenNoEpoch: epoch_blocks=0 → no adjustment even at block 0.
func TestAdaptiveEmission_DisabledWhenNoEpoch(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)}
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 0 // no epochs
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 1_000
	k.SetParams(ctx, params)

	// Block 100 is divisible by 0... which Go would panic. But we guard with > 0.
	// Scale must not change.
	k.BeginBlocker(ctx) // ctx block = 100

	scale := k.GetEmissionScale(ctx)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected scale unchanged at %d, got %d", types.DefaultEmissionScale, scale)
	}
}

// TestAdaptiveEmission_ScaleIncreasesWhenBelowTarget: low bonded ratio → scale up.
func TestAdaptiveEmission_ScaleIncreasesWhenBelowTarget(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Supply = 1_000_000 uzrn, bonded = 100_000 → ratio = 10% < 67% target
	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)}
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000 // 67%
	params.EmissionAdjustmentBps = 5_000  // 0.5% step
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	expected := types.DefaultEmissionScale + 5_000 // 1_005_000
	if scale != expected {
		t.Fatalf("expected scale %d (increased), got %d", expected, scale)
	}
}

// TestAdaptiveEmission_ScaleDecreasesWhenAboveTarget: high bonded ratio → scale down.
func TestAdaptiveEmission_ScaleDecreasesWhenAboveTarget(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Supply = 1_000_000 uzrn, bonded = 900_000 → ratio = 90% > 67% target
	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(900_000)}
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 5_000
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	expected := types.DefaultEmissionScale - 5_000 // 995_000
	if scale != expected {
		t.Fatalf("expected scale %d (decreased), got %d", expected, scale)
	}
}

// TestAdaptiveEmission_ScaleHoldsAtTarget: bonded ratio exactly at target → no change.
func TestAdaptiveEmission_ScaleHoldsAtTarget(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Supply = 1_000_000 uzrn, bonded = 670_000 → ratio = exactly 67%
	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(670_000)}
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 5_000
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected scale unchanged at %d, got %d", types.DefaultEmissionScale, scale)
	}
}

// TestAdaptiveEmission_FloorClamp: scale cannot drop below MinEmissionScaleBps.
func TestAdaptiveEmission_FloorClamp(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(900_000)} // above target → decrease

	k.SetStakingKeeper(sk)

	// Set current scale very close to min
	k.SetEmissionScale(ctx, 800_010)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 50_000   // large step
	params.MinEmissionScaleBps = 800_000    // floor at 0.8×
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	if scale != 800_000 {
		t.Fatalf("expected scale clamped to floor 800000, got %d", scale)
	}
}

// TestAdaptiveEmission_CeilingClamp: scale cannot exceed MaxEmissionScaleBps.
func TestAdaptiveEmission_CeilingClamp(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)} // below target → increase

	k.SetStakingKeeper(sk)

	// Set current scale close to max
	k.SetEmissionScale(ctx, 1_490_000)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 10
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 50_000     // large step
	params.MaxEmissionScaleBps = 1_500_000    // ceiling at 1.5×
	k.SetParams(ctx, params)

	ctx = sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 10}, false, ctx.Logger())
	k.BeginBlocker(ctx)

	scale := k.GetEmissionScale(ctx)
	if scale != 1_500_000 {
		t.Fatalf("expected scale clamped to ceiling 1500000, got %d", scale)
	}
}

// TestAdaptiveEmission_MintScaledDown: scale < 1.0× reduces effective mint.
func TestAdaptiveEmission_MintScaledDown(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Pre-set scale to 0.5× (500_000 BPS)
	k.SetEmissionScale(ctx, 500_000)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "scale_down",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "1000",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx) // block 100

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	// effective = 1000 * 500_000 / 1_000_000 = 500
	if !coin.Amount.Equal(sdkmath.NewInt(500)) {
		t.Fatalf("expected 500 uzrn (0.5× scale), got %s", coin.Amount)
	}
}

// TestAdaptiveEmission_MintScaledUp: scale > 1.0× increases effective mint.
func TestAdaptiveEmission_MintScaledUp(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	// Pre-set scale to 1.5× (1_500_000 BPS)
	k.SetEmissionScale(ctx, 1_500_000)

	k.SetEmissionPeriod(ctx, &types.EmissionPeriod{
		Id:             "scale_up",
		StartBlock:     100,
		EndBlock:       200,
		AmountPerBlock: "1000",
		Recipient:      testCreator,
		Active:         true,
	})

	k.BeginBlocker(ctx) // block 100

	recipientAddr, _ := sdk.AccAddressFromBech32(testCreator)
	coin := bk.GetBalance(ctx, recipientAddr, "uzrn")
	// effective = 1000 * 1_500_000 / 1_000_000 = 1500
	if !coin.Amount.Equal(sdkmath.NewInt(1500)) {
		t.Fatalf("expected 1500 uzrn (1.5× scale), got %s", coin.Amount)
	}
}

// TestAdaptiveEmission_EpochBoundaryTriggersAdjust: scale adjusts at epoch block, not between.
func TestAdaptiveEmission_EpochBoundaryTriggersAdjust(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	addr, _ := sdk.AccAddressFromBech32(testCreator)
	bk.balances[addr.String()] = map[string]sdkmath.Int{"uzrn": sdkmath.NewInt(1_000_000)}
	sk := &mockStakingKeeper{totalBonded: big.NewInt(100_000)} // below target → would increase
	k.SetStakingKeeper(sk)

	params := k.GetParams(ctx)
	params.EmissionEpochBlocks = 50
	params.TargetBondedRatioBps = 670_000
	params.EmissionAdjustmentBps = 10_000
	k.SetParams(ctx, params)

	// Non-epoch block (101 % 50 != 0)
	ctx101 := sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 101}, false, ctx.Logger())
	k.BeginBlocker(ctx101)
	scale := k.GetEmissionScale(ctx101)
	if scale != types.DefaultEmissionScale {
		t.Fatalf("expected no adjustment at block 101, got scale %d", scale)
	}

	// Epoch block (150 % 50 == 0)
	ctx150 := sdk.NewContext(ctx.MultiStore(), cmtproto.Header{Height: 150}, false, ctx.Logger())
	k.BeginBlocker(ctx150)
	scale = k.GetEmissionScale(ctx150)
	expected := types.DefaultEmissionScale + 10_000
	if scale != expected {
		t.Fatalf("expected scale %d at epoch block 150, got %d", expected, scale)
	}
}

// TestAdaptiveEmission_Validate_MinExceedsMax: Validate rejects min > max.
func TestAdaptiveEmission_Validate_MinExceedsMax(t *testing.T) {
	p := types.DefaultParams()
	p.TargetBondedRatioBps = 670_000
	p.EmissionAdjustmentBps = 1_000
	p.MinEmissionScaleBps = 1_500_000
	p.MaxEmissionScaleBps = 800_000 // less than min → invalid
	if err := p.Validate(); err == nil {
		t.Fatal("expected Validate to fail when min > max")
	}
}

// TestAdaptiveEmission_Validate_AdjustmentWithoutTarget: adjustment > 0 but target = 0 → invalid.
func TestAdaptiveEmission_Validate_AdjustmentWithoutTarget(t *testing.T) {
	p := types.DefaultParams()
	p.TargetBondedRatioBps = 0
	p.EmissionAdjustmentBps = 1_000 // target missing
	if err := p.Validate(); err == nil {
		t.Fatal("expected Validate to fail when adjustment set but target is zero")
	}
}

// TestAdaptiveEmission_Validate_ValidConfig: well-formed params pass.
func TestAdaptiveEmission_Validate_ValidConfig(t *testing.T) {
	p := types.DefaultParams()
	p.EmissionEpochBlocks = 100
	p.TargetBondedRatioBps = 670_000
	p.EmissionAdjustmentBps = 1_000
	p.MinEmissionScaleBps = 500_000
	p.MaxEmissionScaleBps = 2_000_000
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid params to pass, got: %v", err)
	}
}
