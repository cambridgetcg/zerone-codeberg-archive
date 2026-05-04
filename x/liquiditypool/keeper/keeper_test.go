package keeper_test

import (
	"context"
	"math/big"
	"testing"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

const (
	testAuthority = "zrn1authority"
	testChainID   = "zerone-test-1"
)

// ---------- Mock BankKeeper ----------

type mockBankKeeper struct {
	balances       map[string]map[string]int64
	moduleBalances map[string]map[string]int64
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{
		balances:       make(map[string]map[string]int64),
		moduleBalances: make(map[string]map[string]int64),
	}
}

func (m *mockBankKeeper) setBalance(addr, denom string, amount int64) {
	if m.balances[addr] == nil {
		m.balances[addr] = make(map[string]int64)
	}
	m.balances[addr][denom] = amount
}

func (m *mockBankKeeper) SendCoins(_ context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error {
	for _, coin := range amt {
		from := fromAddr.String()
		to := toAddr.String()
		if m.balances[from] == nil {
			m.balances[from] = make(map[string]int64)
		}
		if m.balances[to] == nil {
			m.balances[to] = make(map[string]int64)
		}
		m.balances[from][coin.Denom] -= coin.Amount.Int64()
		m.balances[to][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *mockBankKeeper) GetBalance(_ context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	if bal, ok := m.balances[addr.String()]; ok {
		if amt, exists := bal[denom]; exists {
			return sdk.NewCoin(denom, sdkmath.NewInt(amt))
		}
	}
	return sdk.NewCoin(denom, sdkmath.ZeroInt())
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	for _, coin := range amt {
		from := senderAddr.String()
		if m.balances[from] == nil {
			m.balances[from] = make(map[string]int64)
		}
		if m.moduleBalances[recipientModule] == nil {
			m.moduleBalances[recipientModule] = make(map[string]int64)
		}
		m.balances[from][coin.Denom] -= coin.Amount.Int64()
		m.moduleBalances[recipientModule][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	for _, coin := range amt {
		to := recipientAddr.String()
		if m.moduleBalances[senderModule] == nil {
			m.moduleBalances[senderModule] = make(map[string]int64)
		}
		if m.balances[to] == nil {
			m.balances[to] = make(map[string]int64)
		}
		m.moduleBalances[senderModule][coin.Denom] -= coin.Amount.Int64()
		m.balances[to][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	for _, coin := range amt {
		if m.moduleBalances[senderModule] == nil {
			m.moduleBalances[senderModule] = make(map[string]int64)
		}
		if m.moduleBalances[recipientModule] == nil {
			m.moduleBalances[recipientModule] = make(map[string]int64)
		}
		m.moduleBalances[senderModule][coin.Denom] -= coin.Amount.Int64()
		m.moduleBalances[recipientModule][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *mockBankKeeper) MintCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	if m.moduleBalances[moduleName] == nil {
		m.moduleBalances[moduleName] = make(map[string]int64)
	}
	for _, coin := range amt {
		m.moduleBalances[moduleName][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *mockBankKeeper) BurnCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	if m.moduleBalances[moduleName] == nil {
		m.moduleBalances[moduleName] = make(map[string]int64)
	}
	for _, coin := range amt {
		m.moduleBalances[moduleName][coin.Denom] -= coin.Amount.Int64()
	}
	return nil
}

// ---------- Test Setup ----------

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	err := stateStore.LoadLatestVersion()
	if err != nil {
		t.Fatalf("failed to load latest version: %v", err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	mockBK := newMockBankKeeper()

	storeService := runtime.NewKVStoreService(storeKey)
	k := keeper.NewKeeper(cdc, storeService, mockBK, testAuthority)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100, ChainID: testChainID}, false, log.NewNopLogger())

	return k, ctx, mockBK
}

func setupMsgServer(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context, *mockBankKeeper) {
	t.Helper()
	k, ctx, bk := setupKeeper(t)
	return keeper.NewMsgServerImpl(k), k, ctx, bk
}

// createTestPool creates a pool via the msg server.
func createTestPool(t *testing.T, ms types.MsgServer, ctx sdk.Context, bk *mockBankKeeper, denomA, denomB, amountA, amountB string) string {
	t.Helper()
	bk.setBalance(testAuthority, denomA, 1_000_000_000_000_000)
	bk.setBalance(testAuthority, denomB, 1_000_000_000_000_000)

	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: testAuthority,
		DenomA:  denomA,
		DenomB:  denomB,
		AmountA: amountA,
		AmountB: amountB,
	})
	if err != nil {
		t.Fatalf("createTestPool failed: %v", err)
	}
	return resp.PoolId
}

// ===================================================================
// AMM Pure Math Tests
// ===================================================================

func TestCalculateSwapOutput_Basic(t *testing.T) {
	reserveIn := big.NewInt(1_000_000)
	reserveOut := big.NewInt(1_000_000)
	tokenIn := big.NewInt(10_000)

	// 30% fee = 300,000 bps
	tokenOut, feeAmt := keeper.CalculateSwapOutput(reserveIn, reserveOut, tokenIn, 300_000)

	// fee = 10000 * 300000 / 1000000 = 3000
	if feeAmt.Int64() != 3000 {
		t.Errorf("expected fee 3000, got %s", feeAmt.String())
	}

	// effectiveIn = 10000 - 3000 = 7000
	// tokenOut = 1000000 * 7000 / (1000000 + 7000) = 7000000000 / 1007000 ~ 6950
	if tokenOut.Int64() < 6940 || tokenOut.Int64() > 6960 {
		t.Errorf("expected tokenOut ~6950, got %s", tokenOut.String())
	}
}

func TestCalculateSwapOutput_ZeroInputs(t *testing.T) {
	out, fee := keeper.CalculateSwapOutput(big.NewInt(0), big.NewInt(1000), big.NewInt(100), 30000)
	if out.Sign() != 0 || fee.Sign() != 0 {
		t.Error("expected zero output for zero reserve in")
	}

	out, fee = keeper.CalculateSwapOutput(big.NewInt(1000), big.NewInt(0), big.NewInt(100), 30000)
	if out.Sign() != 0 || fee.Sign() != 0 {
		t.Error("expected zero output for zero reserve out")
	}

	out, fee = keeper.CalculateSwapOutput(big.NewInt(1000), big.NewInt(1000), big.NewInt(0), 30000)
	if out.Sign() != 0 || fee.Sign() != 0 {
		t.Error("expected zero output for zero token in")
	}
}

func TestCalculateSwapOutput_ConstantProductHolds(t *testing.T) {
	reserveIn := big.NewInt(1_000_000)
	reserveOut := big.NewInt(2_000_000)
	tokenIn := big.NewInt(100_000)

	tokenOut, feeAmt := keeper.CalculateSwapOutput(reserveIn, reserveOut, tokenIn, 30_000) // 3% fee

	// After swap: newReserveIn = reserveIn + effectiveIn, newReserveOut = reserveOut - tokenOut
	effectiveIn := new(big.Int).Sub(tokenIn, feeAmt)
	newIn := new(big.Int).Add(reserveIn, effectiveIn)
	newOut := new(big.Int).Sub(reserveOut, tokenOut)

	kBefore := new(big.Int).Mul(reserveIn, reserveOut)
	kAfter := new(big.Int).Mul(newIn, newOut)

	if kAfter.Cmp(kBefore) < 0 {
		t.Errorf("k decreased: before=%s after=%s", kBefore.String(), kAfter.String())
	}
}

func TestCalculateSwapOutput_LargeSwapPriceImpact(t *testing.T) {
	reserve := big.NewInt(1_000_000)
	tokenIn := big.NewInt(500_000)
	tokenOut, _ := keeper.CalculateSwapOutput(reserve, reserve, tokenIn, 0) // no fee

	halfReserve := big.NewInt(500_000)
	if tokenOut.Cmp(halfReserve) >= 0 {
		t.Errorf("large swap should suffer price impact: got %s >= %s", tokenOut.String(), halfReserve.String())
	}
	// dy = 1M * 500K / (1M + 500K) = 500B / 1.5M ~ 333333
	if tokenOut.Int64() < 333000 || tokenOut.Int64() > 334000 {
		t.Errorf("expected ~333333, got %d", tokenOut.Int64())
	}
}

func TestCalculateLPTokens_InitialDeposit(t *testing.T) {
	amountA := big.NewInt(1_000_000)
	amountB := big.NewInt(4_000_000)
	lp := keeper.CalculateLPTokensForDeposit(amountA, amountB, amountA, amountB, big.NewInt(0))

	// sqrt(1M * 4M) = sqrt(4 * 10^12) = 2 * 10^6 = 2000000
	if lp.Int64() != 2_000_000 {
		t.Errorf("expected 2000000 LP tokens, got %s", lp.String())
	}
}

func TestCalculateLPTokens_SubsequentDeposit(t *testing.T) {
	reserveA := big.NewInt(1_000_000)
	reserveB := big.NewInt(2_000_000)
	totalSupply := big.NewInt(1_414_213) // ~sqrt(1M * 2M)

	// Deposit proportional: 100K A, 200K B
	lp := keeper.CalculateLPTokensForDeposit(reserveA, reserveB, big.NewInt(100_000), big.NewInt(200_000), totalSupply)
	// LP = totalSupply * min(100K/1M, 200K/2M) = 1414213 * 0.1 = 141421
	if lp.Int64() != 141421 {
		t.Errorf("expected 141421 LP tokens, got %s", lp.String())
	}
}

func TestCalculateWithdrawalAmounts(t *testing.T) {
	reserveA := big.NewInt(1_000_000)
	reserveB := big.NewInt(2_000_000)
	totalSupply := big.NewInt(1_000_000)
	lpTokens := big.NewInt(100_000) // 10%

	outA, outB := keeper.CalculateWithdrawalAmounts(reserveA, reserveB, lpTokens, totalSupply)

	if outA.Int64() != 100_000 {
		t.Errorf("expected 100000 A, got %s", outA.String())
	}
	if outB.Int64() != 200_000 {
		t.Errorf("expected 200000 B, got %s", outB.String())
	}
}

func TestCalculateProportionalDeposit(t *testing.T) {
	reserveA := big.NewInt(1_000_000)
	reserveB := big.NewInt(2_000_000)

	// Want to deposit 50K A, 200K B — B is excessive, A is binding
	actualA, actualB := keeper.CalculateProportionalDeposit(reserveA, reserveB, big.NewInt(50_000), big.NewInt(200_000))
	if actualA.Int64() != 50_000 {
		t.Errorf("expected actualA=50000, got %s", actualA.String())
	}
	if actualB.Int64() != 100_000 {
		t.Errorf("expected actualB=100000, got %s (reserve ratio 1:2)", actualB.String())
	}

	// Want to deposit 200K A, 200K B — A is excessive, B is binding
	actualA, actualB = keeper.CalculateProportionalDeposit(reserveA, reserveB, big.NewInt(200_000), big.NewInt(200_000))
	if actualA.Int64() != 100_000 {
		t.Errorf("expected actualA=100000, got %s", actualA.String())
	}
	if actualB.Int64() != 200_000 {
		t.Errorf("expected actualB=200000, got %s", actualB.String())
	}
}

func TestCalculatePriceImpactBps(t *testing.T) {
	reserveIn := big.NewInt(1_000_000)
	reserveOut := big.NewInt(1_000_000)
	tokenIn := big.NewInt(100_000)
	// No fee: tokenOut = 1M * 100K / (1M + 100K) = 90909
	tokenOut, _ := keeper.CalculateSwapOutput(reserveIn, reserveOut, tokenIn, 0)
	impact := keeper.CalculatePriceImpactBps(reserveIn, reserveOut, tokenIn, tokenOut)
	// ideal = 100K * 1M / 1M = 100K, actual ~ 90909
	// impact = (100K - 90909) / 100K * 1M ~ 90910 bps (9.09%)
	if impact < 80000 || impact > 100000 {
		t.Errorf("expected impact ~90910 bps, got %d", impact)
	}
}

// ===================================================================
// Keeper State Tests
// ===================================================================

func TestPoolCRUD(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:        "pool-1",
		DenomA:        "uzrn",
		DenomB:        "uatom",
		ReserveA:      "1000000",
		ReserveB:      "2000000",
		SwapFeeBps:    30_000,
		LpTokenSupply: "1414213",
		Creator:       testAuthority,
	}

	k.SetPool(ctx, pool)

	got, found := k.GetPool(ctx, "pool-1")
	if !found {
		t.Fatal("pool not found")
	}
	if got.DenomA != "uzrn" || got.ReserveA != "1000000" {
		t.Errorf("pool data mismatch: %+v", got)
	}

	// GetPoolByDenoms (lexicographic sorting)
	byDenom := k.GetPoolByDenoms(ctx, "uatom", "uzrn")
	if byDenom == nil || byDenom.PoolId != "pool-1" {
		t.Error("GetPoolByDenoms failed")
	}

	// CountPools
	if count := k.CountPools(ctx); count != 1 {
		t.Errorf("expected 1 pool, got %d", count)
	}

	// DeletePool
	k.DeletePool(ctx, pool)
	_, found = k.GetPool(ctx, "pool-1")
	if found {
		t.Error("pool should be deleted")
	}
}

func TestTWAPAccumulatorCRUD(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	acc := &types.TWAPAccumulator{
		PoolId:       "pool-1",
		LastBlock:    100,
		CumPriceAToB: "5000000000",
		CumPriceBToA: "2500000000",
	}

	k.SetTWAPAccumulator(ctx, acc)

	got, found := k.GetTWAPAccumulator(ctx, "pool-1")
	if !found {
		t.Fatal("TWAP accumulator not found")
	}
	if got.LastBlock != 100 || got.CumPriceAToB != "5000000000" {
		t.Errorf("TWAP data mismatch: %+v", got)
	}
}

func TestParamsGetSet(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Default params
	params := k.GetParams(ctx)
	if params.DefaultSwapFeeBps != 3_000 {
		t.Errorf("expected default fee 3000, got %d", params.DefaultSwapFeeBps)
	}

	// Set custom params
	custom := types.DefaultParams()
	custom.MaxPools = 10
	custom.DefaultSwapFeeBps = 5_000
	k.SetParams(ctx, custom)

	got := k.GetParams(ctx)
	if got.MaxPools != 10 || got.DefaultSwapFeeBps != 5_000 {
		t.Errorf("params not updated: %+v", got)
	}
}

func TestPoolCounter(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	id1 := k.GetNextPoolId(ctx)
	if id1 != 1 {
		t.Errorf("expected first pool ID 1, got %d", id1)
	}

	got := k.IncrementPoolCounter(ctx)
	if got != 1 {
		t.Errorf("expected IncrementPoolCounter to return 1, got %d", got)
	}

	id2 := k.GetNextPoolId(ctx)
	if id2 != 2 {
		t.Errorf("expected next pool ID 2, got %d", id2)
	}
}

func TestGenesisExportImport(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:        "pool-1",
		DenomA:        "uzrn",
		DenomB:        "uatom",
		ReserveA:      "1000000",
		ReserveB:      "2000000",
		SwapFeeBps:    30_000,
		LpTokenSupply: "1414213",
	}
	k.SetPool(ctx, pool)

	acc := &types.TWAPAccumulator{
		PoolId:       "pool-1",
		LastBlock:    100,
		CumPriceAToB: "5000000000",
		CumPriceBToA: "2500000000",
	}
	k.SetTWAPAccumulator(ctx, acc)

	// Export
	gs := k.ExportGenesis(ctx)
	if len(gs.Pools) != 1 {
		t.Fatalf("expected 1 pool in genesis, got %d", len(gs.Pools))
	}
	if len(gs.TwapAccumulators) != 1 {
		t.Fatalf("expected 1 TWAP accumulator in genesis, got %d", len(gs.TwapAccumulators))
	}

	// Import into fresh keeper
	k2, ctx2, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, gs)

	got, found := k2.GetPool(ctx2, "pool-1")
	if !found {
		t.Fatal("pool not found after import")
	}
	if got.ReserveA != "1000000" {
		t.Errorf("reserve mismatch after import")
	}

	gotAcc, found := k2.GetTWAPAccumulator(ctx2, "pool-1")
	if !found {
		t.Fatal("TWAP accumulator not found after import")
	}
	if gotAcc.CumPriceAToB != "5000000000" {
		t.Errorf("TWAP data mismatch after import")
	}
}

// ===================================================================
// Msg Server Tests: CreatePool
// ===================================================================

func TestCreatePool_Success(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	if poolId != "pool-1" {
		t.Errorf("expected pool-1, got %s", poolId)
	}

	pool, found := k.GetPool(ctx, poolId)
	if !found {
		t.Fatal("pool not found after creation")
	}
	if pool.ReserveA != "100000000000" || pool.ReserveB != "200000000000" {
		t.Errorf("reserves mismatch: %s/%s", pool.ReserveA, pool.ReserveB)
	}

	// LP tokens = sqrt(100B * 200B) = sqrt(2 * 10^22) ~ 141421356237
	lp := new(big.Int)
	lp.SetString(pool.LpTokenSupply, 10)
	if lp.Int64() < 141_421_000_000 || lp.Int64() > 141_422_000_000 {
		t.Errorf("expected LP ~141421356237, got %s", pool.LpTokenSupply)
	}
}

func TestCreatePool_OnlyAuthority(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: "zrn1unauthorized0000000000000000",
		DenomA:  "uzrn",
		DenomB:  "uatom",
		AmountA: "1000000",
		AmountB: "1000000",
	})
	if err == nil {
		t.Error("expected error for unauthorized creator")
	}
}

func TestCreatePool_MaxPoolsReached(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	// Set max pools to 1
	params := types.DefaultParams()
	params.MaxPools = 1
	k.SetParams(ctx, params)

	createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Second pool should fail
	bk.setBalance(testAuthority, "uosmo", 1_000_000_000)
	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: testAuthority,
		DenomA:  "uzrn",
		DenomB:  "uosmo",
		AmountA: "1000000",
		AmountB: "1000000",
	})
	if err == nil {
		t.Error("expected error for max pools reached")
	}
}

func TestCreatePool_DuplicateDenomPair(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Same denom pair again
	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: testAuthority,
		DenomA:  "uzrn",
		DenomB:  "uatom",
		AmountA: "1000000",
		AmountB: "1000000",
	})
	if err == nil {
		t.Error("expected error for duplicate denom pair")
	}
}

func TestCreatePool_ZeroAmount(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: testAuthority,
		DenomA:  "uzrn",
		DenomB:  "uatom",
		AmountA: "0",
		AmountB: "1000000",
	})
	if err == nil {
		t.Error("expected error for zero amount")
	}
}

// ===================================================================
// Msg Server Tests: Swap
// ===================================================================

func TestSwap_Success(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)

	resp, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("swap failed: %v", err)
	}

	outAmt := new(big.Int)
	outAmt.SetString(resp.TokenOutAmount, 10)
	if outAmt.Sign() <= 0 {
		t.Error("expected positive output")
	}

	// Verify reserves updated
	pool, _ := k.GetPool(ctx, poolId)
	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	if reserveA.Int64() <= 100_000_000_000 {
		t.Errorf("expected reserve A to increase above 100B, got %s", pool.ReserveA)
	}
}

func TestSwap_SlippageProtection(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)

	// Set impossibly high minimum output
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
		MinTokenOut:   "999999999",
	})
	if err == nil {
		t.Error("expected slippage error")
	}
}

func TestSwap_PoolNotFound(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        "zrn1sender0000000000000000000000",
		PoolId:        "nonexistent",
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err == nil {
		t.Error("expected pool not found error")
	}
}

func TestSwap_WrongDenom(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uosmo", 100_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uosmo",
		TokenInAmount: "10000",
	})
	if err == nil {
		t.Error("expected denom not in pool error")
	}
}

func TestSwap_LockedPool(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	// Lock the pool directly
	pool, _ := k.GetPool(ctx, poolId)
	pool.Locked = true
	k.SetPool(ctx, pool)

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err == nil {
		t.Error("expected pool locked error")
	}
}

func TestSwap_UnlocksAfterCompletion(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("swap failed: %v", err)
	}

	// Pool should be unlocked after swap
	pool, _ := k.GetPool(ctx, poolId)
	if pool.Locked {
		t.Error("pool should be unlocked after successful swap")
	}
}

func TestSwap_BothDirections(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)
	bk.setBalance(senderAddr, "uatom", 100_000)

	resp1, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("A->B swap failed: %v", err)
	}

	resp2, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uatom",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("B->A swap failed: %v", err)
	}

	out1 := new(big.Int)
	out1.SetString(resp1.TokenOutAmount, 10)
	out2 := new(big.Int)
	out2.SetString(resp2.TokenOutAmount, 10)

	if out1.Sign() <= 0 || out2.Sign() <= 0 {
		t.Error("both directions should produce positive output")
	}

	pool, _ := k.GetPool(ctx, poolId)
	if pool.Locked {
		t.Error("pool should be unlocked after all swaps")
	}
}

func TestSwap_MinReserveCheck(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	// Create a pool with small reserves to test min_reserve
	params := types.DefaultParams()
	params.MinReserve = "1000"
	params.MinInitialLiquidity = "1" // allow small pools for this test
	k.SetParams(ctx, params)

	bk.setBalance(testAuthority, "uzrn", 1_000_000_000)
	bk.setBalance(testAuthority, "uatom", 1_000_000_000)

	resp, err := ms.CreatePool(ctx, &types.MsgCreatePool{
		Creator: testAuthority,
		DenomA:  "uzrn",
		DenomB:  "uatom",
		AmountA: "2000",
		AmountB: "2000",
	})
	if err != nil {
		t.Fatalf("create pool failed: %v", err)
	}

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 10_000)

	// Try to swap nearly all of one side
	_, err = ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        resp.PoolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err == nil {
		t.Error("expected reserve below minimum error")
	}
}

// ===================================================================
// Msg Server Tests: AddLiquidity
// ===================================================================

func TestAddLiquidity_Success(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 1_000_000)
	bk.setBalance(senderAddr, "uatom", 2_000_000)

	resp, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  senderAddr,
		PoolId:  poolId,
		AmountA: "100000",
		AmountB: "200000",
	})
	if err != nil {
		t.Fatalf("add liquidity failed: %v", err)
	}

	lpMinted := new(big.Int)
	lpMinted.SetString(resp.LpTokensMinted, 10)
	if lpMinted.Sign() <= 0 {
		t.Error("expected positive LP tokens minted")
	}

	// Verify reserves increased
	pool, _ := k.GetPool(ctx, poolId)
	rA := new(big.Int)
	rA.SetString(pool.ReserveA, 10)
	expected := new(big.Int).SetInt64(100_000_100_000)
	if rA.Cmp(expected) != 0 {
		t.Errorf("expected reserve A %s, got %s", expected.String(), pool.ReserveA)
	}
}

func TestAddLiquidity_SlippageCheck(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 1_000_000)
	bk.setBalance(senderAddr, "uatom", 2_000_000)

	_, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:      senderAddr,
		PoolId:      poolId,
		AmountA:     "100000",
		AmountB:     "200000",
		MinLpTokens: "999999999",
	})
	if err == nil {
		t.Error("expected slippage error for impossibly high min LP")
	}
}

func TestAddLiquidity_ProportionalDeposit(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 1_000_000)
	bk.setBalance(senderAddr, "uatom", 2_000_000)

	// Disproportionate deposit (too much B). Should use A as binding constraint.
	resp, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  senderAddr,
		PoolId:  poolId,
		AmountA: "100000",
		AmountB: "500000", // more than needed for 1:2 ratio
	})
	if err != nil {
		t.Fatalf("add liquidity failed: %v", err)
	}

	// Actual B should be ~200000 (100K * 200B/100B)
	actualB := new(big.Int)
	actualB.SetString(resp.ActualB, 10)
	if actualB.Int64() != 200_000 {
		t.Errorf("expected actualB=200000, got %s", resp.ActualB)
	}
}

// ===================================================================
// Msg Server Tests: RemoveLiquidity
// ===================================================================

func TestRemoveLiquidity_Success(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)
	totalLP := pool.LpTokenSupply

	senderAddr := "zrn1sender0000000000000000000000"
	lpDenom := types.LPDenom(poolId)

	// Parse total LP and take 10%
	totalLPBig := new(big.Int)
	totalLPBig.SetString(totalLP, 10)
	tenPercent := new(big.Int).Div(totalLPBig, big.NewInt(10))
	bk.setBalance(senderAddr, lpDenom, tenPercent.Int64())

	resp, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   senderAddr,
		PoolId:   poolId,
		LpTokens: tenPercent.String(),
	})
	if err != nil {
		t.Fatalf("remove liquidity failed: %v", err)
	}

	outA := new(big.Int)
	outA.SetString(resp.AmountA, 10)
	outB := new(big.Int)
	outB.SetString(resp.AmountB, 10)

	// 10% of 100B ~ 10B, 10% of 200B ~ 20B
	expectedA := int64(10_000_000_000)
	expectedB := int64(20_000_000_000)
	if outA.Int64() < expectedA-1 || outA.Int64() > expectedA+1 {
		t.Errorf("expected ~%d A, got %s", expectedA, resp.AmountA)
	}
	if outB.Int64() < expectedB-1 || outB.Int64() > expectedB+1 {
		t.Errorf("expected ~%d B, got %s", expectedB, resp.AmountB)
	}
}

func TestRemoveLiquidity_SlippageCheckA(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)
	totalLPBig := new(big.Int)
	totalLPBig.SetString(pool.LpTokenSupply, 10)
	tenPercent := new(big.Int).Div(totalLPBig, big.NewInt(10))

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, types.LPDenom(poolId), tenPercent.Int64())

	_, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:     senderAddr,
		PoolId:     poolId,
		LpTokens:   tenPercent.String(),
		MinAmountA: "999999999999999",
	})
	if err == nil {
		t.Error("expected slippage error for high minAmountA")
	}
}

func TestRemoveLiquidity_ExceedsSupply(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, types.LPDenom(poolId), 999_999_999)

	_, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   senderAddr,
		PoolId:   poolId,
		LpTokens: "999999999999",
	})
	if err == nil {
		t.Error("expected error for LP exceeding supply")
	}
}

// ===================================================================
// Msg Server Tests: UpdateParams
// ===================================================================

func TestUpdateParams_Success(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	newParams := types.DefaultParams()
	newParams.MaxPools = 50
	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: testAuthority,
		Params:    newParams,
	})
	if err != nil {
		t.Fatalf("update params failed: %v", err)
	}

	got := k.GetParams(ctx)
	if got.MaxPools != 50 {
		t.Errorf("expected MaxPools=50, got %d", got.MaxPools)
	}
}

func TestUpdateParams_Unauthorized(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "zrn1unauthorized0000000000000000",
		Params:    types.DefaultParams(),
	})
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

// ===================================================================
// TWAP Tests
// ===================================================================

func TestUpdateTWAPAccumulator(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:   "pool-1",
		DenomA:   "uzrn",
		DenomB:   "uatom",
		ReserveA: "1000000",
		ReserveB: "2000000",
	}
	k.SetPool(ctx, pool)

	// First update at block 100 — initializes accumulator
	k.UpdateTWAPAccumulator(ctx, pool)

	acc, found := k.GetTWAPAccumulator(ctx, "pool-1")
	if !found {
		t.Fatal("TWAP accumulator not found")
	}
	if acc.LastBlock != 100 {
		t.Errorf("expected last block 100, got %d", acc.LastBlock)
	}

	// Advance 10 blocks
	ctx = ctx.WithBlockHeight(110)
	k.UpdateTWAPAccumulator(ctx, pool)

	acc, _ = k.GetTWAPAccumulator(ctx, "pool-1")
	if acc.LastBlock != 110 {
		t.Errorf("expected last block 110, got %d", acc.LastBlock)
	}

	// CumPriceAToB should be positive (reserveB/reserveA * blocksDelta * scale)
	cumAtoB := new(big.Int)
	cumAtoB.SetString(acc.CumPriceAToB, 10)
	if cumAtoB.Sign() <= 0 {
		t.Errorf("expected positive cumulative price A->B, got %s", acc.CumPriceAToB)
	}
}

func TestUpdateTWAPAccumulator_SameBlock(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:   "pool-1",
		DenomA:   "uzrn",
		DenomB:   "uatom",
		ReserveA: "1000000",
		ReserveB: "2000000",
	}
	k.SetPool(ctx, pool)

	k.UpdateTWAPAccumulator(ctx, pool)
	acc1, _ := k.GetTWAPAccumulator(ctx, "pool-1")
	cum1 := acc1.CumPriceAToB

	// Same block — should not update
	k.UpdateTWAPAccumulator(ctx, pool)
	acc2, _ := k.GetTWAPAccumulator(ctx, "pool-1")

	if acc2.CumPriceAToB != cum1 {
		t.Error("TWAP should not change on same block")
	}
}

func TestGetSpotPrice(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:   "pool-1",
		DenomA:   "uzrn",
		DenomB:   "uatom",
		ReserveA: "1000000",
		ReserveB: "2000000",
	}
	k.SetPool(ctx, pool)

	// Spot price of uzrn in uatom = 2M * 1e6 / 1M = 2_000_000
	price, err := k.GetSpotPrice(ctx, "pool-1", "uzrn")
	if err != nil {
		t.Fatalf("GetSpotPrice failed: %v", err)
	}
	if price.Int64() != 2_000_000 {
		t.Errorf("expected spot price 2000000, got %s", price.String())
	}

	// Spot price of uatom in uzrn = 1M * 1e6 / 2M = 500_000
	price, err = k.GetSpotPrice(ctx, "pool-1", "uatom")
	if err != nil {
		t.Fatalf("GetSpotPrice failed: %v", err)
	}
	if price.Int64() != 500_000 {
		t.Errorf("expected spot price 500000, got %s", price.String())
	}
}

func TestGetZRNPrice(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pool := &types.Pool{
		PoolId:   "pool-1",
		DenomA:   "uzrn",
		DenomB:   "uusdc",
		ReserveA: "1000000",
		ReserveB: "5000000",
	}
	k.SetPool(ctx, pool)

	price, err := k.GetZRNPrice(ctx, "uusdc")
	if err != nil {
		t.Fatalf("GetZRNPrice failed: %v", err)
	}
	// price = quoteReserve * 1e6 / zrnReserve = 5M * 1e6 / 1M = 5_000_000
	if price.Int64() != 5_000_000 {
		t.Errorf("expected ZRN price 5000000, got %s", price.String())
	}
}

func TestGetZRNPrice_NoPool(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	_, err := k.GetZRNPrice(ctx, "uusdc")
	if err == nil {
		t.Error("expected error when no pool exists")
	}
}

// ===================================================================
// Query Server Tests
// ===================================================================

func TestQueryPool(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	pool := &types.Pool{
		PoolId:   "pool-1",
		DenomA:   "uzrn",
		DenomB:   "uatom",
		ReserveA: "1000000",
		ReserveB: "2000000",
	}
	k.SetPool(ctx, pool)

	resp, err := qs.Pool(ctx, &types.QueryPoolRequest{PoolId: "pool-1"})
	if err != nil {
		t.Fatalf("query pool failed: %v", err)
	}
	if resp.Pool.ReserveA != "1000000" {
		t.Errorf("reserve mismatch in query response")
	}
}

func TestQueryPool_NotFound(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Pool(ctx, &types.QueryPoolRequest{PoolId: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent pool")
	}
}

func TestQueryPools(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	k.SetPool(ctx, &types.Pool{PoolId: "pool-1", DenomA: "uzrn", DenomB: "uatom", ReserveA: "1000000", ReserveB: "2000000"})
	k.SetPool(ctx, &types.Pool{PoolId: "pool-2", DenomA: "uzrn", DenomB: "uosmo", ReserveA: "500000", ReserveB: "800000"})

	resp, err := qs.Pools(ctx, &types.QueryPoolsRequest{})
	if err != nil {
		t.Fatalf("query pools failed: %v", err)
	}
	if len(resp.Pools) != 2 {
		t.Errorf("expected 2 pools, got %d", len(resp.Pools))
	}
}

func TestQueryParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("query params failed: %v", err)
	}
	if resp.Params.DefaultSwapFeeBps != 3_000 {
		t.Errorf("expected default fee 3000, got %d", resp.Params.DefaultSwapFeeBps)
	}
}

func TestQuerySimulateSwap(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	pool := &types.Pool{
		PoolId:     "pool-1",
		DenomA:     "uzrn",
		DenomB:     "uatom",
		ReserveA:   "1000000",
		ReserveB:   "2000000",
		SwapFeeBps: 3_000,
	}
	k.SetPool(ctx, pool)

	resp, err := qs.SimulateSwap(ctx, &types.QuerySimulateSwapRequest{
		PoolId:        "pool-1",
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("simulate swap failed: %v", err)
	}
	if resp.Result.TokenOutDenom != "uatom" {
		t.Errorf("expected output denom uatom, got %s", resp.Result.TokenOutDenom)
	}
	outAmt := new(big.Int)
	outAmt.SetString(resp.Result.TokenOutAmount, 10)
	if outAmt.Sign() <= 0 {
		t.Error("expected positive simulated output")
	}
}

// ===================================================================
// Integration-Style Tests
// ===================================================================

func TestSwap_FeeAccruesToPool(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)
	rA0 := new(big.Int)
	rA0.SetString(pool.ReserveA, 10)
	rB0 := new(big.Int)
	rB0.SetString(pool.ReserveB, 10)
	k0 := new(big.Int).Mul(rA0, rB0) // initial k

	// Execute a swap
	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 100_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000",
	})
	if err != nil {
		t.Fatalf("swap failed: %v", err)
	}

	// After swap: k should increase (fees stay in pool)
	pool, _ = k.GetPool(ctx, poolId)
	rA1 := new(big.Int)
	rA1.SetString(pool.ReserveA, 10)
	rB1 := new(big.Int)
	rB1.SetString(pool.ReserveB, 10)
	k1 := new(big.Int).Mul(rA1, rB1)

	if k1.Cmp(k0) < 0 {
		t.Errorf("k should not decrease after swap: before=%s after=%s", k0.String(), k1.String())
	}
}

func TestRoundTrip_AddRemoveLiquidity(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 1_000_000)
	bk.setBalance(senderAddr, "uatom", 2_000_000)

	// Add liquidity
	addResp, err := ms.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  senderAddr,
		PoolId:  poolId,
		AmountA: "100000",
		AmountB: "200000",
	})
	if err != nil {
		t.Fatalf("add liquidity failed: %v", err)
	}

	lpMinted := addResp.LpTokensMinted

	// Give LP tokens to sender for removal
	lpAmt := new(big.Int)
	lpAmt.SetString(lpMinted, 10)
	bk.setBalance(senderAddr, types.LPDenom(poolId), lpAmt.Int64())

	// Remove the same LP tokens
	rmResp, err := ms.RemoveLiquidity(ctx, &types.MsgRemoveLiquidity{
		Sender:   senderAddr,
		PoolId:   poolId,
		LpTokens: lpMinted,
	})
	if err != nil {
		t.Fatalf("remove liquidity failed: %v", err)
	}

	// Should get back approximately same amounts (may lose 1 due to rounding)
	gotA := new(big.Int)
	gotA.SetString(rmResp.AmountA, 10)
	gotB := new(big.Int)
	gotB.SetString(rmResp.AmountB, 10)

	if gotA.Int64() < 99_000 || gotA.Int64() > 100_001 {
		t.Errorf("expected ~100000 A back, got %s", rmResp.AmountA)
	}
	if gotB.Int64() < 199_000 || gotB.Int64() > 200_001 {
		t.Errorf("expected ~200000 B back, got %s", rmResp.AmountB)
	}

	// Pool reserves should return to approximately original values (100B)
	pool, _ := k.GetPool(ctx, poolId)
	rA := new(big.Int)
	rA.SetString(pool.ReserveA, 10)
	origA := new(big.Int).SetInt64(100_000_000_000)
	diff := new(big.Int).Sub(rA, origA)
	diff.Abs(diff)
	if diff.Int64() > 1 {
		t.Errorf("reserve A should be ~100000000000, got %s", pool.ReserveA)
	}
}

func TestMultipleSwaps_PriceMovement(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Get initial spot price (should be 1:1 = 1_000_000)
	price0, err := k.GetSpotPrice(ctx, poolId, "uzrn")
	if err != nil {
		t.Fatalf("GetSpotPrice failed: %v", err)
	}

	// Execute multiple swaps buying uatom with uzrn — should push price down
	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 10_000_000)

	for i := 0; i < 5; i++ {
		_, err := ms.Swap(ctx, &types.MsgSwap{
			Sender:        senderAddr,
			PoolId:        poolId,
			TokenInDenom:  "uzrn",
			TokenInAmount: "50000",
		})
		if err != nil {
			t.Fatalf("swap %d failed: %v", i, err)
		}
	}

	// Price of uzrn in uatom should have decreased (more uzrn, less uatom in pool)
	price1, err := k.GetSpotPrice(ctx, poolId, "uzrn")
	if err != nil {
		t.Fatalf("GetSpotPrice failed: %v", err)
	}

	if price1.Cmp(price0) >= 0 {
		t.Errorf("price should decrease after buying uatom: initial=%s final=%s", price0.String(), price1.String())
	}
}

// ===================================================================
// GetTWAPSafe Tests
// ===================================================================

func TestGetTWAPSafe_FreshAccumulator(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)

	// Seed the TWAP accumulator at block 100 (the test context height).
	k.UpdateTWAPAccumulator(ctx, pool)

	// maxStalenessBlocks=50 — accumulator is at block 100, current block is 100 → fresh.
	price, _, err := k.GetTWAPSafe(ctx, poolId, "uzrn", 0, 50)
	if err != nil {
		t.Fatalf("expected no error for fresh accumulator: %v", err)
	}
	if price == nil || price.Sign() <= 0 {
		t.Fatalf("expected positive price, got %v", price)
	}
}

func TestGetTWAPSafe_StaleAccumulator(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)

	// Seed the accumulator at block 100.
	k.UpdateTWAPAccumulator(ctx, pool)

	// Advance block height to 200 — accumulator was last updated at 100.
	// maxStalenessBlocks=50, delta=100 → stale.
	laterCtx := ctx.WithBlockHeight(200)
	_, _, err := k.GetTWAPSafe(laterCtx, poolId, "uzrn", 0, 50)
	if err == nil {
		t.Fatal("expected ErrTWAPStale but got nil")
	}
}

func TestGetTWAPSafe_ZeroMaxStaleness_SkipsCheck(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)

	// Seed at block 100, advance to block 1000.
	k.UpdateTWAPAccumulator(ctx, pool)
	laterCtx := ctx.WithBlockHeight(1000)

	// maxStalenessBlocks=0 → check skipped regardless of staleness.
	price, _, err := k.GetTWAPSafe(laterCtx, poolId, "uzrn", 0, 0)
	if err != nil {
		t.Fatalf("expected no error with maxStalenessBlocks=0: %v", err)
	}
	if price == nil || price.Sign() <= 0 {
		t.Fatalf("expected positive price, got %v", price)
	}
}

func TestGetTWAPSafe_NoAccumulator_ReturnsSpotPrice(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	// No UpdateTWAPAccumulator call → no accumulator in store.
	// GetTWAPSafe should fall through to GetTWAP which returns spot price.
	price, _, err := k.GetTWAPSafe(ctx, poolId, "uzrn", 0, 50)
	if err != nil {
		t.Fatalf("expected no error when no accumulator exists: %v", err)
	}
	if price == nil || price.Sign() <= 0 {
		t.Fatalf("expected positive spot price, got %v", price)
	}
}

func TestGetTWAPSafe_ExactStalenessThreshold(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "200000000000")

	pool, _ := k.GetPool(ctx, poolId)
	k.UpdateTWAPAccumulator(ctx, pool) // block 100

	// delta = maxStalenessBlocks exactly → not stale (delta <= maxStalenessBlocks is fresh,
	// delta > maxStalenessBlocks is stale).
	at150 := ctx.WithBlockHeight(150)
	_, _, err := k.GetTWAPSafe(at150, poolId, "uzrn", 0, 50)
	if err != nil {
		t.Fatalf("expected fresh at exact threshold: %v", err)
	}

	// One block beyond threshold → stale.
	at151 := ctx.WithBlockHeight(151)
	_, _, err = k.GetTWAPSafe(at151, poolId, "uzrn", 0, 50)
	if err == nil {
		t.Fatal("expected ErrTWAPStale one block past threshold")
	}
}

// ===================================================================
// Msg Server Tests: MaxSwapFractionBps enforcement (Gap 5)
// ===================================================================

// TestSwap_MaxFractionBps_BlocksLargeSwap verifies that a swap exceeding
// max_swap_fraction_bps of the input reserve is rejected.
func TestSwap_MaxFractionBps_BlocksLargeSwap(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	// Reserve: 100_000_000_000 uzrn : 100_000_000_000 uatom (standard test amount).
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Cap: 10% of reserveIn = 10_000_000_000 uzrn.
	params := types.DefaultParams()
	params.MaxSwapFractionBps = 100_000 // 10%
	k.SetParams(ctx, params)

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 20_000_000_000)

	// Attempt to swap 15_000_000_000 (15% > 10%) — must be rejected.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "15000000000",
	})
	if err == nil {
		t.Fatal("expected ErrSwapFractionExceeded for oversized swap")
	}
}

// TestSwap_MaxFractionBps_AllowsExactBoundary verifies that a swap equal to
// max_swap_fraction_bps of the input reserve is allowed.
func TestSwap_MaxFractionBps_AllowsExactBoundary(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	params := types.DefaultParams()
	params.MaxSwapFractionBps = 100_000 // 10% → max 10_000_000_000 uzrn
	k.SetParams(ctx, params)

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 20_000_000_000)

	// Exactly 10% of reserveIn = 10_000_000_000.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "10000000000",
	})
	if err != nil {
		t.Fatalf("expected swap at exact boundary to succeed: %v", err)
	}
}

// TestSwap_MaxFractionBps_ZeroMeansDisabled verifies that setting
// max_swap_fraction_bps = 0 disables the check and permits large swaps.
func TestSwap_MaxFractionBps_ZeroMeansDisabled(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	params := types.DefaultParams()
	params.MaxSwapFractionBps = 0 // disabled
	params.MinReserve = "1"
	k.SetParams(ctx, params)

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 60_000_000_000)

	// Swap 50% of reserve — should be allowed when check is disabled.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "50000000000",
	})
	if err != nil {
		t.Fatalf("expected large swap to succeed when max_swap_fraction_bps=0: %v", err)
	}
}

// TestSwap_MaxFractionBps_DefaultValue verifies the default param value
// is 10% (100_000 bps) and that it is enforced.
func TestSwap_MaxFractionBps_DefaultValue(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	// Default params already have MaxSwapFractionBps = 100_000 (10%).
	k.SetParams(ctx, types.DefaultParams())

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	params := k.GetParams(ctx)
	if params.MaxSwapFractionBps != 100_000 {
		t.Fatalf("expected default MaxSwapFractionBps 100000, got %d", params.MaxSwapFractionBps)
	}

	senderAddr := "zrn1sender0000000000000000000000"
	bk.setBalance(senderAddr, "uzrn", 20_000_000_000)

	// Swap at 11% = 11_000_000_000 > 10% cap → blocked.
	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        senderAddr,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "11000000000",
	})
	if err == nil {
		t.Fatal("default 10% cap should block 11% swap")
	}
}

// TestParams_MaxFractionBps_InvalidExceedsScale verifies that setting
// max_swap_fraction_bps > 1_000_000 fails Validate().
func TestParams_MaxFractionBps_InvalidExceedsScale(t *testing.T) {
	p := types.DefaultParams()
	p.MaxSwapFractionBps = 1_000_001
	if err := p.Validate(); err == nil {
		t.Fatal("expected Validate to fail for max_swap_fraction_bps > 1000000")
	}
}

// TestParams_MaxFractionBps_MaxScale verifies that exactly 1_000_000 bps
// (100%) is valid (though extreme).
func TestParams_MaxFractionBps_MaxScale(t *testing.T) {
	p := types.DefaultParams()
	p.MaxSwapFractionBps = 1_000_000
	if err := p.Validate(); err != nil {
		t.Fatalf("expected Validate to pass for max_swap_fraction_bps=1000000: %v", err)
	}
}
