package keeper_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/billing/keeper"
	"github.com/zerone-chain/zerone/x/billing/types"
)

// -----------------------------------------------------------------------
// Mock BankKeeper
// -----------------------------------------------------------------------

type mockBankKeeper struct {
	balances map[string]sdkmath.Int
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{
		balances: make(map[string]sdkmath.Int),
	}
}

func (m *mockBankKeeper) setBalance(addr sdk.AccAddress, denom string, amount sdkmath.Int) {
	m.balances[addr.String()+"/"+denom] = amount
}

func (m *mockBankKeeper) SendCoins(_ context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error {
	for _, coin := range amt {
		fromKey := fromAddr.String() + "/" + coin.Denom
		toKey := toAddr.String() + "/" + coin.Denom

		fromBal, ok := m.balances[fromKey]
		if !ok {
			fromBal = sdkmath.ZeroInt()
		}
		if fromBal.LT(coin.Amount) {
			return fmt.Errorf("insufficient balance")
		}
		m.balances[fromKey] = fromBal.Sub(coin.Amount)

		toBal, ok := m.balances[toKey]
		if !ok {
			toBal = sdkmath.ZeroInt()
		}
		m.balances[toKey] = toBal.Add(coin.Amount)
	}
	return nil
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	for _, coin := range amt {
		key := senderAddr.String() + "/" + coin.Denom
		bal, ok := m.balances[key]
		if !ok {
			bal = sdkmath.ZeroInt()
		}
		if bal.LT(coin.Amount) {
			return fmt.Errorf("insufficient balance for send to module")
		}
		m.balances[key] = bal.Sub(coin.Amount)
		modKey := recipientModule + "/" + coin.Denom
		modBal, ok := m.balances[modKey]
		if !ok {
			modBal = sdkmath.ZeroInt()
		}
		m.balances[modKey] = modBal.Add(coin.Amount)
	}
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	for _, coin := range amt {
		modKey := senderModule + "/" + coin.Denom
		modBal, ok := m.balances[modKey]
		if !ok {
			modBal = sdkmath.ZeroInt()
		}
		if modBal.LT(coin.Amount) {
			return fmt.Errorf("insufficient module balance")
		}
		m.balances[modKey] = modBal.Sub(coin.Amount)

		key := recipientAddr.String() + "/" + coin.Denom
		bal, ok := m.balances[key]
		if !ok {
			bal = sdkmath.ZeroInt()
		}
		m.balances[key] = bal.Add(coin.Amount)
	}
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule string, recipientModule string, amt sdk.Coins) error {
	for _, coin := range amt {
		fromKey := senderModule + "/" + coin.Denom
		toKey := recipientModule + "/" + coin.Denom

		fromBal, ok := m.balances[fromKey]
		if !ok {
			fromBal = sdkmath.ZeroInt()
		}
		if fromBal.LT(coin.Amount) {
			return fmt.Errorf("insufficient module balance")
		}
		m.balances[fromKey] = fromBal.Sub(coin.Amount)

		toBal, ok := m.balances[toKey]
		if !ok {
			toBal = sdkmath.ZeroInt()
		}
		m.balances[toKey] = toBal.Add(coin.Amount)
	}
	return nil
}

// GetModuleBalance returns the balance of a module account keyed by module name.
// Matches the BankKeeper interface used by DevelopmentFundSpend.
func (m *mockBankKeeper) GetModuleBalance(_ context.Context, moduleName string, denom string) sdk.Coin {
	key := moduleName + "/" + denom
	if bal, ok := m.balances[key]; ok {
		return sdk.NewCoin(denom, bal)
	}
	return sdk.NewCoin(denom, sdkmath.ZeroInt())
}

// -----------------------------------------------------------------------
// Mock KnowledgeKeeper
// -----------------------------------------------------------------------

type mockKnowledgeKeeper struct {
	facts map[string]mockFact
}

type mockFact struct {
	confidence    uint64
	citationCount uint64
	submitter     string
	createdBlock  uint64
}

func newMockKnowledgeKeeper() *mockKnowledgeKeeper {
	return &mockKnowledgeKeeper{
		facts: make(map[string]mockFact),
	}
}

func (m *mockKnowledgeKeeper) addFact(factId string, confidence, citations, createdBlock uint64, submitter string) {
	m.facts[factId] = mockFact{
		confidence:    confidence,
		citationCount: citations,
		submitter:     submitter,
		createdBlock:  createdBlock,
	}
}

func (m *mockKnowledgeKeeper) GetFactConfidence(_ context.Context, factId string) (uint64, bool) {
	f, ok := m.facts[factId]
	if !ok {
		return 0, false
	}
	return f.confidence, true
}

func (m *mockKnowledgeKeeper) GetFactCitationCount(_ context.Context, factId string) (uint64, bool) {
	f, ok := m.facts[factId]
	if !ok {
		return 0, false
	}
	return f.citationCount, true
}

func (m *mockKnowledgeKeeper) GetFactSubmitter(_ context.Context, factId string) (string, bool) {
	f, ok := m.facts[factId]
	if !ok {
		return "", false
	}
	return f.submitter, true
}

func (m *mockKnowledgeKeeper) GetFactCreatedBlock(_ context.Context, factId string) (uint64, bool) {
	f, ok := m.facts[factId]
	if !ok {
		return 0, false
	}
	return f.createdBlock, true
}

func (m *mockKnowledgeKeeper) IncrementCitationCount(_ context.Context, factId string) error {
	f, ok := m.facts[factId]
	if !ok {
		return fmt.Errorf("fact not found: %s", factId)
	}
	f.citationCount++
	m.facts[factId] = f
	return nil
}

// -----------------------------------------------------------------------
// Mock ResearchFundDepositor
// -----------------------------------------------------------------------

type mockResearchFundDepositor struct {
	bk *mockBankKeeper
}

func (d *mockResearchFundDepositor) DepositToResearchFund(_ context.Context, sourceModule string, amount sdk.Coins) error {
	return d.bk.SendCoinsFromModuleToModule(context.Background(), sourceModule, "research_fund", amount)
}

// -----------------------------------------------------------------------
// Mock LiquidityPoolKeeper
// -----------------------------------------------------------------------

type mockLiquidityPoolKeeper struct {
	twap            *big.Int
	twapErr         error
	lastUpdateBlock uint64
}

func (m *mockLiquidityPoolKeeper) GetTWAP(_ context.Context, _ string, _ uint64) (*big.Int, error) {
	return m.twap, m.twapErr
}

func (m *mockLiquidityPoolKeeper) GetLastPriceUpdateHeight(_ context.Context) uint64 {
	return m.lastUpdateBlock
}

// mockExternalPriceFeedKeeper returns a fixed price for any feedAddress.
type mockExternalPriceFeedKeeper struct {
	price *big.Int
	err   error
}

func (m *mockExternalPriceFeedKeeper) GetLatestPrice(_ context.Context, _ string) (*big.Int, error) {
	return m.price, m.err
}

// -----------------------------------------------------------------------
// Test Setup
// -----------------------------------------------------------------------

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper, *mockKnowledgeKeeper) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatalf("failed to load latest version: %v", err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	bk := newMockBankKeeper()
	kk := newMockKnowledgeKeeper()
	rfd := &mockResearchFundDepositor{bk: bk}

	authority := sdk.AccAddress([]byte("authority-addr------")).String()
	k := keeper.NewKeeper(runtime.NewKVStoreService(storeKey), cdc, authority, bk, kk, rfd)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())

	return k, ctx, bk, kk
}

func testAddr(i int) string {
	addr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", i)))
	return addr.String()
}

// -----------------------------------------------------------------------
// Tests: Params
// -----------------------------------------------------------------------

func TestParamsDefault(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	params := k.GetParams(ctx)
	if params.BaseQueryPrice != "1000000" {
		t.Errorf("expected BaseQueryPrice 1000000, got %s", params.BaseQueryPrice)
	}
	if params.QuoteValidityBlocks != 100 {
		t.Errorf("expected QuoteValidityBlocks 100, got %d", params.QuoteValidityBlocks)
	}
	if params.ConfidenceThreshold != 500000 {
		t.Errorf("expected ConfidenceThreshold 500000, got %d", params.ConfidenceThreshold)
	}
	if params.RevenueSplit == nil {
		t.Fatal("expected non-nil RevenueSplit")
	}
	if params.RevenueSplit.ContributorBps != 550000 {
		t.Errorf("expected ContributorBps 550000, got %d", params.RevenueSplit.ContributorBps)
	}
}

func TestParamsSetGet(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	custom := types.DefaultParams()
	custom.BaseQueryPrice = "2000000"
	custom.ConfidenceThreshold = 600000
	custom.ConfidenceWeightBps = 300000
	k.SetParams(ctx, custom)

	got := k.GetParams(ctx)
	if got.BaseQueryPrice != "2000000" {
		t.Errorf("expected BaseQueryPrice 2000000, got %s", got.BaseQueryPrice)
	}
	if got.ConfidenceThreshold != 600000 {
		t.Errorf("expected ConfidenceThreshold 600000, got %d", got.ConfidenceThreshold)
	}
}

// -----------------------------------------------------------------------
// Tests: Provider CRUD
// -----------------------------------------------------------------------

func TestProviderCRUD(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	addr := testAddr(1)

	_, found := k.GetProvider(ctx, addr)
	if found {
		t.Error("expected provider not found")
	}

	provider := &types.Provider{
		Address:      addr,
		Name:         "TestProvider",
		Domains:      []string{"mathematics", "physics"},
		StakeAmount:  "100000000",
		Active:       true,
		TotalQueries: 0,
		TotalRevenue: "0",
		RegisteredAt: 100,
	}
	k.SetProvider(ctx, provider)

	got, found := k.GetProvider(ctx, addr)
	if !found {
		t.Fatal("expected provider to be found")
	}
	if got.StakeAmount != "100000000" {
		t.Errorf("expected stake 100000000, got %s", got.StakeAmount)
	}
	if len(got.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(got.Domains))
	}

	k.DeleteProvider(ctx, addr)
	_, found = k.GetProvider(ctx, addr)
	if found {
		t.Error("expected provider to be deleted")
	}
}

func TestGetAllProviders(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	for i := 0; i < 3; i++ {
		k.SetProvider(ctx, &types.Provider{
			Address:      testAddr(i),
			Domains:      []string{"test"},
			StakeAmount:  "100000000",
			Active:       true,
			TotalRevenue: "0",
		})
	}
	all := k.GetAllProviders(ctx)
	if len(all) != 3 {
		t.Errorf("expected 3 providers, got %d", len(all))
	}
}

// -----------------------------------------------------------------------
// Tests: Distribution
// -----------------------------------------------------------------------

func TestDistribution_RevenueSplit(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	submitter1 := testAddr(10)
	submitter2 := testAddr(11)
	kk.addFact("fact-1", 800000, 100, 50, submitter1)
	kk.addFact("fact-2", 300000, 10, 80, submitter2)

	totalPayment := big.NewInt(10000000) // 10 ZRN

	distribution := k.CalculateDistribution(ctx, totalPayment, []string{"fact-1", "fact-2"})

	providerShare := new(big.Int)
	providerShare.SetString(distribution.ProviderShare, 10)
	expectedProvider := big.NewInt(5500000) // 55%
	if providerShare.Cmp(expectedProvider) != 0 {
		t.Errorf("expected provider share %s, got %s", expectedProvider, providerShare)
	}

	researchShare := new(big.Int)
	researchShare.SetString(distribution.ResearchShare, 10)
	expectedResearch := big.NewInt(333000) // 3.33%
	if researchShare.Cmp(expectedResearch) != 0 {
		t.Errorf("expected research share %s, got %s", expectedResearch, researchShare)
	}

	devAmt := new(big.Int)
	devAmt.SetString(distribution.DevelopmentFund, 10)
	expectedDev := big.NewInt(1967000) // 19.67% development fund
	if devAmt.Cmp(expectedDev) != 0 {
		t.Errorf("expected development fund %s, got %s", expectedDev, devAmt)
	}
}

func TestDistribution_DevelopmentFund(t *testing.T) {
	k, ctx, bk, kk := setupKeeper(t)

	callerAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 1)))
	providerAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 2)))
	submitterAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 10)))

	kk.addFact("fact-1", 500000, 10, 50, submitterAddr.String())

	totalPayment := big.NewInt(10000000)
	bk.setBalance(callerAddr, "uzrn", sdkmath.NewIntFromBigInt(totalPayment))

	distribution := k.CalculateDistribution(ctx, totalPayment, []string{"fact-1"})
	err := k.ExecuteDistribution(ctx, callerAddr, providerAddr, distribution)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Billing module should have zero balance after routing to development_fund
	billingBal := bk.balances["billing/uzrn"]
	if billingBal.IsPositive() {
		t.Errorf("expected billing module balance 0 after development fund transfer, got %s", billingBal)
	}

	// Development fund should have received the allocation
	devBal := bk.balances[keeper.DevelopmentFund+"/uzrn"]
	expectedDev := sdkmath.NewInt(1967000) // 19.67% of 10M
	if !devBal.Equal(expectedDev) {
		t.Errorf("expected development_fund balance %s, got %s", expectedDev, devBal)
	}
}

func TestDistribution_KnowledgePool(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	submitter1 := testAddr(10)
	submitter2 := testAddr(11)
	kk.addFact("fact-1", 800000, 100, 50, submitter1)
	kk.addFact("fact-2", 100000, 1, 80, submitter2)

	distribution := k.CalculateDistribution(ctx, big.NewInt(10000000), []string{"fact-1", "fact-2"})

	if len(distribution.KnowledgePool) != 2 {
		t.Fatalf("expected 2 knowledge pool entries, got %d", len(distribution.KnowledgePool))
	}

	w1 := new(big.Int)
	w1.SetString(distribution.KnowledgePool[0].Weight, 10)
	w2 := new(big.Int)
	w2.SetString(distribution.KnowledgePool[1].Weight, 10)

	if w1.Cmp(w2) <= 0 {
		t.Errorf("expected fact-1 weight > fact-2 weight, got %s vs %s", w1, w2)
	}

	sum := new(big.Int)
	for _, entry := range distribution.KnowledgePool {
		amt := new(big.Int)
		amt.SetString(entry.Amount, 10)
		sum.Add(sum, amt)
	}
	expected := big.NewInt(1100000) // 11% of 10M (50% of 22%)
	if sum.Cmp(expected) != 0 {
		t.Errorf("expected knowledge pool sum %s, got %s", expected, sum)
	}
}

func TestDistribution_ModuleAccounts(t *testing.T) {
	k, ctx, bk, kk := setupKeeper(t)

	callerAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 1)))
	providerAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 2)))
	submitterAddr := sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", 10)))

	kk.addFact("fact-1", 500000, 10, 50, submitterAddr.String())

	totalPayment := big.NewInt(10000000)
	bk.setBalance(callerAddr, "uzrn", sdkmath.NewIntFromBigInt(totalPayment))

	distribution := k.CalculateDistribution(ctx, totalPayment, []string{"fact-1"})
	err := k.ExecuteDistribution(ctx, callerAddr, providerAddr, distribution)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	researchBal := bk.balances[keeper.ResearchFund+"/uzrn"]
	expectedResearch := sdkmath.NewInt(333000) // 3.33% of 10M
	if !researchBal.Equal(expectedResearch) {
		t.Errorf("expected research_fund balance %s, got %s", expectedResearch, researchBal)
	}

	treasuryBal := bk.balances[keeper.ProtocolTreasury+"/uzrn"]
	expectedTreasury := sdkmath.NewInt(440000)
	if !treasuryBal.Equal(expectedTreasury) {
		t.Errorf("expected treasury_protocol balance %s, got %s", expectedTreasury, treasuryBal)
	}

	knowledgeBal := bk.balances[keeper.KnowledgeModuleName+"/uzrn"]
	expectedVerification := sdkmath.NewInt(660000)
	if !knowledgeBal.Equal(expectedVerification) {
		t.Errorf("expected knowledge module balance %s, got %s", expectedVerification, knowledgeBal)
	}
}

func TestRevenueSplit_FromParams(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 500000, 10, 50, testAddr(10))

	// Default split: 55/22/3.33/19.67 → change to 60/20/10/10
	params := types.DefaultParams()
	params.RevenueSplit.ContributorBps = 600000
	params.RevenueSplit.ProtocolBps = 200000
	params.RevenueSplit.ResearchBps = 100000
	params.RevenueSplit.DevelopmentBps = 100000
	k.SetParams(ctx, params)

	distribution := k.CalculateDistribution(ctx, big.NewInt(10000000), []string{"fact-1"})

	providerShare := new(big.Int)
	providerShare.SetString(distribution.ProviderShare, 10)
	expected := big.NewInt(6000000) // 60%
	if providerShare.Cmp(expected) != 0 {
		t.Errorf("expected provider share %s with custom split, got %s", expected, providerShare)
	}

	researchShare := new(big.Int)
	researchShare.SetString(distribution.ResearchShare, 10)
	expectedResearch := big.NewInt(1000000) // 10%
	if researchShare.Cmp(expectedResearch) != 0 {
		t.Errorf("expected research share %s with custom split, got %s", expectedResearch, researchShare)
	}
}

// -----------------------------------------------------------------------
// Tests: RegisterProvider
// -----------------------------------------------------------------------

func TestRegisterProvider(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	addr := testAddr(1)
	accAddr, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(accAddr, "uzrn", sdkmath.NewInt(200000000))

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RegisterProvider(ctx, &types.MsgRegisterProvider{
		Sender:  addr,
		Name:    "TestProvider",
		Domains: []string{"mathematics", "physics"},
		Stake:   "100000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	provider, found := k.GetProvider(ctx, addr)
	if !found {
		t.Fatal("expected provider to be found")
	}
	if !provider.Active {
		t.Error("expected provider to be active")
	}
	if provider.StakeAmount != "100000000" {
		t.Errorf("expected stake 100000000, got %s", provider.StakeAmount)
	}

	mathProviders := k.GetProvidersByDomain(ctx, "mathematics")
	if len(mathProviders) != 1 {
		t.Errorf("expected 1 math provider, got %d", len(mathProviders))
	}
}

func TestRegisterProviderInsufficientStake(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	addr := testAddr(1)
	accAddr, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(accAddr, "uzrn", sdkmath.NewInt(200000000))

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RegisterProvider(ctx, &types.MsgRegisterProvider{
		Sender:  addr,
		Domains: []string{"mathematics"},
		Stake:   "1000",
	})
	if err == nil {
		t.Error("expected insufficient stake error")
	}
}

func TestRegisterProviderDuplicate(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	addr := testAddr(1)
	accAddr, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(accAddr, "uzrn", sdkmath.NewInt(500000000))

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RegisterProvider(ctx, &types.MsgRegisterProvider{
		Sender:  addr,
		Domains: []string{"mathematics"},
		Stake:   "100000000",
	})
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	_, err = srv.RegisterProvider(ctx, &types.MsgRegisterProvider{
		Sender:  addr,
		Domains: []string{"physics"},
		Stake:   "100000000",
	})
	if err == nil {
		t.Error("expected duplicate provider error")
	}
}

// -----------------------------------------------------------------------
// Tests: DeregisterProvider
// -----------------------------------------------------------------------

func TestDeregisterProvider(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)

	addr := testAddr(1)
	accAddr, _ := sdk.AccAddressFromBech32(addr)
	bk.setBalance(accAddr, "uzrn", sdkmath.NewInt(200000000))

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RegisterProvider(ctx, &types.MsgRegisterProvider{
		Sender:  addr,
		Domains: []string{"mathematics"},
		Stake:   "100000000",
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	_, err = srv.DeregisterProvider(ctx, &types.MsgDeregisterProvider{
		Sender: addr,
	})
	if err != nil {
		t.Fatalf("deregistration failed: %v", err)
	}

	_, found := k.GetProvider(ctx, addr)
	if found {
		t.Error("expected provider to be deleted")
	}

	mathProviders := k.GetProvidersByDomain(ctx, "mathematics")
	if len(mathProviders) != 0 {
		t.Errorf("expected 0 math providers after deregister, got %d", len(mathProviders))
	}
}

// -----------------------------------------------------------------------
// Tests: Pricing
// -----------------------------------------------------------------------

func TestPricingBasic(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-basic", 500000, 0, 1, testAddr(10))

	totalPrice, breakdown := k.CalculateQueryPrice(ctx, []string{"fact-basic"}, 2000)

	if totalPrice.Sign() <= 0 {
		t.Error("expected positive total price")
	}
	if len(breakdown) != 1 {
		t.Fatalf("expected 1 breakdown entry, got %d", len(breakdown))
	}

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)
	if totalPrice.Cmp(baseCost) != 0 {
		t.Errorf("expected base cost %s, got %s", baseCost, totalPrice)
	}
}

func TestPricing_ConfidenceCurve(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	// Low confidence: surcharge
	kk.addFact("fact-lo", 200000, 0, 1, testAddr(10))
	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-lo"}, 2000)

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)
	adj := new(big.Int).Mul(baseCost, big.NewInt(200000))
	adj.Div(adj, big.NewInt(1000000))
	expected := new(big.Int).Add(baseCost, adj)

	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected %s with low-confidence surcharge, got %s", expected, totalPrice)
	}
}

func TestPricing_NoveltyCurve(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-cited", 600000, 100, 1, testAddr(10))

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-cited"}, 2000)

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)

	discount := new(big.Int).Mul(baseCost, big.NewInt(6))
	discount.Div(discount, big.NewInt(10))
	halfBase := new(big.Int).Div(baseCost, big.NewInt(2))
	if discount.Cmp(halfBase) > 0 {
		discount.Set(halfBase)
	}
	expected := new(big.Int).Sub(baseCost, discount)

	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected %s with novelty discount, got %s", expected, totalPrice)
	}
}

func TestPricing_FreshnessCurve(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-fresh", 600000, 0, 50, testAddr(10))

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-fresh"}, 100)

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)
	premium := new(big.Int).Mul(baseCost, big.NewInt(100000))
	premium.Div(premium, big.NewInt(1000000))
	expected := new(big.Int).Add(baseCost, premium)

	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected %s with freshness premium, got %s", expected, totalPrice)
	}
}

func TestPricingPeakValueZone(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-mid", 600000, 0, 1, testAddr(10))

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-mid"}, 2000)

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)
	if totalPrice.Cmp(baseCost) != 0 {
		t.Errorf("expected base cost %s in peak value zone, got %s", baseCost, totalPrice)
	}
}

func TestPricingLowConfidence(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-lo", 200000, 0, 1, testAddr(10))

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-lo"}, 2000)

	baseCost := new(big.Int)
	baseCost.SetString("1000000", 10)
	adj := new(big.Int).Mul(baseCost, big.NewInt(200000))
	adj.Div(adj, big.NewInt(1000000))
	expected := new(big.Int).Add(baseCost, adj)

	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected %s with low-confidence surcharge, got %s", expected, totalPrice)
	}
}

// -----------------------------------------------------------------------
// Tests: Oracle
// -----------------------------------------------------------------------

func TestOracle_ManualOverride(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",   // $0.01
		ManualZrnPriceUsd:  "1000000", // $1.00
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(10000) // 10000 * 1_000_000 / 1_000_000
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected dynamic base cost %s at $1/ZRN, got %s", expected, totalPrice)
	}
}

func TestOracle_TWAPFallback(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "0", // disabled
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	// Set up TWAP keeper with price
	lpk := &mockLiquidityPoolKeeper{
		twap:            big.NewInt(2000000), // $2.00
		lastUpdateBlock: 99,                  // recent (block 100 - 99 = 1 < 5000 staleness)
	}
	k.SetLiquidityPoolKeeper(lpk)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// baseCost = 10000 * 1_000_000 / 2_000_000 = 5000
	expected := big.NewInt(5000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected TWAP-based cost %s, got %s", expected, totalPrice)
	}
}

func TestOracle_StalenessCutoff(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "0",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5, // very short staleness window
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	// TWAP is stale (last update at block 10, current block 100 > 10+5)
	lpk := &mockLiquidityPoolKeeper{
		twap:            big.NewInt(1000000),
		lastUpdateBlock: 10,
	}
	k.SetLiquidityPoolKeeper(lpk)

	// Should fall back to fixed base cost since TWAP is stale
	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(1000000) // fallback to BaseQueryPrice
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected fallback to fixed base cost %s, got %s", expected, totalPrice)
	}
}

func TestOracle_Unavailable(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "0",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	// No liquidity pool keeper set → falls back to fixed
	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(1000000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected fallback to fixed base cost %s, got %s", expected, totalPrice)
	}
}

// -----------------------------------------------------------------------
// Tests: Queries
// -----------------------------------------------------------------------

func TestQueryParams(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Params.BaseQueryPrice != "1000000" {
		t.Errorf("expected 1000000, got %s", resp.Params.BaseQueryPrice)
	}
}

func TestQueryProvider(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	addr := testAddr(1)
	k.SetProvider(ctx, &types.Provider{
		Address:      addr,
		Domains:      []string{"test"},
		StakeAmount:  "100000000",
		Active:       true,
		TotalRevenue: "0",
	})

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.Provider(ctx, &types.QueryProviderRequest{Address: addr})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider.StakeAmount != "100000000" {
		t.Errorf("expected stake 100000000, got %s", resp.Provider.StakeAmount)
	}
}

func TestQueryProviderNotFound(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	_, err := qs.Provider(ctx, &types.QueryProviderRequest{Address: testAddr(99)})
	if err == nil {
		t.Error("expected not found error")
	}
}

func TestQueryQuote(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)
	kk.addFact("fact-1", 500000, 10, 50, testAddr(10))

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.Quote(ctx, &types.QueryQuoteRequest{FactId: "fact-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Quote.FactId != "fact-1" {
		t.Errorf("expected fact-1, got %s", resp.Quote.FactId)
	}
	price := new(big.Int)
	price.SetString(resp.Quote.EffectivePrice, 10)
	if price.Sign() <= 0 {
		t.Error("expected positive effective price")
	}
}

func TestQueryBatchQuote(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)
	kk.addFact("fact-1", 500000, 0, 1, testAddr(10))
	kk.addFact("fact-2", 600000, 10, 50, testAddr(11))

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.BatchQuote(ctx, &types.QueryBatchQuoteRequest{FactIds: []string{"fact-1", "fact-2"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Quotes) != 2 {
		t.Errorf("expected 2 quotes, got %d", len(resp.Quotes))
	}
	totalPrice := new(big.Int)
	totalPrice.SetString(resp.TotalPrice, 10)
	if totalPrice.Sign() <= 0 {
		t.Error("expected positive total price")
	}
}

// -----------------------------------------------------------------------
// Tests: UpdateParams
// -----------------------------------------------------------------------

func TestUpdateParams(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	authority := k.GetAuthority()

	srv := keeper.NewMsgServerImpl(k)
	newParams := types.DefaultParams()
	newParams.BaseQueryPrice = "5000000"
	newParams.ConfidenceThreshold = 700000

	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    newParams,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := k.GetParams(ctx)
	if got.BaseQueryPrice != "5000000" {
		t.Errorf("expected 5000000, got %s", got.BaseQueryPrice)
	}
}

func TestUpdateParamsUnauthorized(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: testAddr(99),
		Params:    types.DefaultParams(),
	})
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

// -----------------------------------------------------------------------
// Tests: Genesis
// -----------------------------------------------------------------------

func TestGenesis(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	addr1 := testAddr(1)
	addr2 := testAddr(2)
	k.SetProvider(ctx, &types.Provider{
		Address: addr1, Domains: []string{"math"}, StakeAmount: "100000000",
		Active: true, TotalRevenue: "0",
	})
	k.SetProvider(ctx, &types.Provider{
		Address: addr2, Domains: []string{"physics"}, StakeAmount: "200000000",
		Active: true, TotalRevenue: "0",
	})

	genState := k.ExportGenesis(ctx)
	if len(genState.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(genState.Providers))
	}

	k2, ctx2, _, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, genState)

	got := k2.ExportGenesis(ctx2)
	if len(got.Providers) != 2 {
		t.Errorf("expected 2 providers after import, got %d", len(got.Providers))
	}
}

func TestGenesisValidation(t *testing.T) {
	valid := types.DefaultGenesis()
	if err := valid.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	dup := types.DefaultGenesis()
	dup.Providers = []*types.Provider{
		{Address: "zrn1abc"}, {Address: "zrn1abc"},
	}
	if err := dup.Validate(); err == nil {
		t.Error("expected duplicate provider error")
	}
}

// -----------------------------------------------------------------------
// Tests: Dynamic Pricing
// -----------------------------------------------------------------------

func TestDynamicPricingConfigCRUD(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	params := k.GetParams(ctx)
	cfg := params.DynamicPricing
	if cfg == nil {
		t.Fatal("expected non-nil dynamic pricing config")
	}
	if cfg.Enabled {
		t.Error("expected default config to be disabled")
	}
	if cfg.TargetQueryCostUsd != "10000" {
		t.Errorf("expected default TargetQueryCostUsd 10000, got %s", cfg.TargetQueryCostUsd)
	}

	// Set custom config via params
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "20000",
		ManualZrnPriceUsd:  "1000000",
		TwapWindowBlocks:   2000,
		StalenessBlocks:    10000,
		MinCostPerFact:     "500",
		MaxCostPerFact:     "50000000",
	}
	k.SetParams(ctx, params)

	got := k.GetParams(ctx)
	if !got.DynamicPricing.Enabled {
		t.Error("expected config to be enabled")
	}
	if got.DynamicPricing.TargetQueryCostUsd != "20000" {
		t.Errorf("expected TargetQueryCostUsd 20000, got %s", got.DynamicPricing.TargetQueryCostUsd)
	}
}

func TestDynamicPricingDisabled(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(1000000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected fixed base cost %s with dynamic pricing disabled, got %s", expected, totalPrice)
	}
}

func TestDynamicPricingManualPrice(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "1000000",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(10000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected dynamic base cost %s at $1/ZRN, got %s", expected, totalPrice)
	}
}

func TestDynamicPricingHighZRNPrice(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "10000000",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "100",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(1000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected dynamic base cost %s at $10/ZRN, got %s", expected, totalPrice)
	}
}

func TestDynamicPricingLowZRNPrice(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "1000",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(10000000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected dynamic base cost %s at $0.001/ZRN, got %s", expected, totalPrice)
	}
}

func TestDynamicPricingFloor(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "1000000000",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "5000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(5000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected floor-clamped cost %s, got %s", expected, totalPrice)
	}
}

func TestDynamicPricingCeiling(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "100",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "50000000",
	}
	k.SetParams(ctx, params)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	expected := big.NewInt(50000000)
	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("expected ceiling-clamped cost %s, got %s", expected, totalPrice)
	}
}

// -----------------------------------------------------------------------
// Tests: Price Feed Events
// -----------------------------------------------------------------------

func setupKeeperWithLPK(t *testing.T, twapPrice *big.Int, lastUpdate uint64) (keeper.Keeper, sdk.Context, *mockBankKeeper, *mockKnowledgeKeeper) {
	t.Helper()
	k, ctx, bk, kk := setupKeeper(t)

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:            true,
		TargetQueryCostUsd: "10000",
		ManualZrnPriceUsd:  "0",
		TwapWindowBlocks:   1000,
		StalenessBlocks:    5000,
		MinCostPerFact:     "1000",
		MaxCostPerFact:     "100000000",
	}
	k.SetParams(ctx, params)

	if twapPrice != nil {
		lpk := &mockLiquidityPoolKeeper{
			twap:            twapPrice,
			lastUpdateBlock: lastUpdate,
		}
		k.SetLiquidityPoolKeeper(lpk)
	}

	return k, ctx, bk, kk
}

func TestPriceEvent_FirstEmission(t *testing.T) {
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(1000000), 99)
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	// First price query should emit an event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	events := ctx.EventManager().Events()
	found := false
	for _, ev := range events {
		if ev.Type == "zerone.billing.oracle_price_update" {
			found = true
			for _, attr := range ev.Attributes {
				if attr.Key == "price_usd" && attr.Value != "1000000" {
					t.Errorf("expected price_usd=1000000, got %s", attr.Value)
				}
				if attr.Key == "source" && attr.Value != "twap" {
					t.Errorf("expected source=twap, got %s", attr.Value)
				}
			}
		}
	}
	if !found {
		t.Error("expected oracle_price_update event on first emission")
	}
}

func TestPriceEvent_BelowThreshold(t *testing.T) {
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(1000000), 99)
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	// First query — emits event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// Change price by 3% (below 5% threshold)
	lpk2 := &mockLiquidityPoolKeeper{
		twap:            big.NewInt(1030000), // +3%
		lastUpdateBlock: 99,
	}
	k.SetLiquidityPoolKeeper(lpk2)

	// Second query — should NOT emit another event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	count := 0
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.billing.oracle_price_update" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 oracle_price_update event (below threshold), got %d", count)
	}
}

func TestPriceEvent_AboveThreshold(t *testing.T) {
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(1000000), 99)
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	// First query — emits event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// Change price by 10% (above 5% threshold)
	lpk2 := &mockLiquidityPoolKeeper{
		twap:            big.NewInt(1100000), // +10%
		lastUpdateBlock: 99,
	}
	k.SetLiquidityPoolKeeper(lpk2)

	// Second query — should emit another event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	count := 0
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.billing.oracle_price_update" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 oracle_price_update events (above threshold), got %d", count)
	}
}

func TestPriceEvent_OracleUnavailable(t *testing.T) {
	k, ctx, _, kk := setupKeeperWithLPK(t, nil, 0) // no LPK
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	// Query with no oracle — should fall back to fixed price, no event
	k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.billing.oracle_price_update" {
			t.Error("expected no oracle_price_update event when oracle is unavailable")
		}
	}
}

// -----------------------------------------------------------------------
// Tests: Batch
// -----------------------------------------------------------------------

func TestBatchQuery_PricingCorrect(t *testing.T) {
	k, ctx, _, kk := setupKeeper(t)

	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))
	kk.addFact("fact-2", 600000, 0, 1, testAddr(11))

	totalPrice, breakdown := k.CalculateQueryPrice(ctx, []string{"fact-1", "fact-2"}, 2000)

	if len(breakdown) != 2 {
		t.Fatalf("expected 2 breakdowns, got %d", len(breakdown))
	}

	// Both facts should have the same price (same params, same confidence)
	if breakdown[0].TotalPrice != breakdown[1].TotalPrice {
		t.Errorf("expected equal prices, got %s vs %s", breakdown[0].TotalPrice, breakdown[1].TotalPrice)
	}

	// Total should be 2x single fact
	singlePrice := new(big.Int)
	singlePrice.SetString(breakdown[0].TotalPrice, 10)
	expectedTotal := new(big.Int).Mul(singlePrice, big.NewInt(2))
	if totalPrice.Cmp(expectedTotal) != 0 {
		t.Errorf("expected batch total %s, got %s", expectedTotal, totalPrice)
	}
}

// -----------------------------------------------------------------------
// Tests: WeightedMedianPrice (pure function)
// -----------------------------------------------------------------------

func TestWeightedMedianPrice_Empty(t *testing.T) {
	result := keeper.WeightedMedianPrice(nil)
	if result != nil {
		t.Errorf("want nil for empty sources, got %s", result)
	}
}

func TestWeightedMedianPrice_Single(t *testing.T) {
	price := big.NewInt(1_000_000)
	result := keeper.WeightedMedianPrice([]*big.Int{price})
	if result.Cmp(price) != 0 {
		t.Errorf("want %s, got %s", price, result)
	}
}

func TestWeightedMedianPrice_TwoSources_ReturnsLower(t *testing.T) {
	// Even count → return lower middle (conservative)
	low := big.NewInt(800_000)
	high := big.NewInt(1_200_000)
	result := keeper.WeightedMedianPrice([]*big.Int{high, low}) // unsorted input
	if result.Cmp(low) != 0 {
		t.Errorf("want lower middle %s for 2-source even count, got %s", low, result)
	}
}

func TestWeightedMedianPrice_ThreeSources_ReturnsMiddle(t *testing.T) {
	// Standard case: median of 3 = middle value after sorting.
	// sorted: [1_000_000, 1_500_000, 2_000_000] → middle (index 1) = 1_500_000
	sources := []*big.Int{
		big.NewInt(1_500_000),
		big.NewInt(1_000_000),
		big.NewInt(2_000_000),
	}
	result := keeper.WeightedMedianPrice(sources)
	want := big.NewInt(1_500_000) // middle of sorted list
	if result.Cmp(want) != 0 {
		t.Errorf("want median %s, got %s", want, result)
	}
}

func TestWeightedMedianPrice_AttackerManipulatesOneSource(t *testing.T) {
	// Attacker controls TWAP (raised to 10x). External + manual are accurate.
	// Median should return the accurate middle, not the manipulated high.
	attackerTWAP := big.NewInt(10_000_000) // 10x normal
	externalFeed := big.NewInt(1_000_000)  // accurate: $1.00
	manualOverride := big.NewInt(1_050_000) // accurate: $1.05

	result := keeper.WeightedMedianPrice([]*big.Int{attackerTWAP, externalFeed, manualOverride})
	// Sorted: 1_000_000, 1_050_000, 10_000_000 → median = 1_050_000
	want := big.NewInt(1_050_000)
	if result.Cmp(want) != 0 {
		t.Errorf("want unmanipulated median %s, got %s (attacker TWAP should not shift median)", want, result)
	}
}

// -----------------------------------------------------------------------
// Tests: external price feed integration
// -----------------------------------------------------------------------

func TestOracle_ExternalFeedOnly(t *testing.T) {
	// No TWAP, no manual — only external feed provides price.
	k, ctx, _, kk := setupKeeper(t)
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:                    true,
		TargetQueryCostUsd:         "10000",
		ManualZrnPriceUsd:          "0",
		TwapWindowBlocks:           1000,
		StalenessBlocks:            5000,
		MinCostPerFact:             "1000",
		MaxCostPerFact:             "100000000",
		ExternalPriceFeedEnabled:   true,
		ExternalPriceFeedAddress:   "oracle_feed_zrn_usd",
	}
	k.SetParams(ctx, params)

	// Only external feed returns a price ($0.50 per ZRN)
	epfk := &mockExternalPriceFeedKeeper{price: big.NewInt(500_000)}
	k.SetExternalPriceFeedKeeper(epfk)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// baseCost = 10000 * 1_000_000 / 500_000 = 20000
	want := big.NewInt(20000)
	if totalPrice.Cmp(want) != 0 {
		t.Errorf("want external-feed-based cost %s, got %s", want, totalPrice)
	}
}

func TestOracle_ThreeSourceMedian(t *testing.T) {
	// All three sources active: TWAP, external feed, manual override.
	// Median of three prices should be computed.
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(1_000_000), 99) // TWAP $1.00
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:                    true,
		TargetQueryCostUsd:         "10000",
		ManualZrnPriceUsd:          "1200000", // $1.20 (governance override)
		TwapWindowBlocks:           1000,
		StalenessBlocks:            5000,
		MinCostPerFact:             "1000",
		MaxCostPerFact:             "100000000",
		ExternalPriceFeedEnabled:   true,
		ExternalPriceFeedAddress:   "oracle_feed_zrn_usd",
	}
	k.SetParams(ctx, params)

	// External feed: $1.10
	epfk := &mockExternalPriceFeedKeeper{price: big.NewInt(1_100_000)}
	k.SetExternalPriceFeedKeeper(epfk)

	// Three sources: TWAP=$1.00, external=$1.10, manual=$1.20
	// Median = $1.10
	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// baseCost = 10000 * 1_000_000 / 1_100_000 ≈ 9090
	// (The exact value depends on big.Int division, no float rounding)
	baseUSD := big.NewInt(10000)
	scale := big.NewInt(1_000_000)
	medianPrice := big.NewInt(1_100_000)
	expected := new(big.Int).Div(new(big.Int).Mul(baseUSD, scale), medianPrice)

	if totalPrice.Cmp(expected) != 0 {
		t.Errorf("want 3-source median cost %s, got %s", expected, totalPrice)
	}
}

func TestOracle_ExternalFeedUnavailable_FallsBackToTWAP(t *testing.T) {
	// External feed enabled but returns error. Should use TWAP alone.
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(2_000_000), 99) // TWAP $2.00
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:                    true,
		TargetQueryCostUsd:         "10000",
		ManualZrnPriceUsd:          "0",
		TwapWindowBlocks:           1000,
		StalenessBlocks:            5000,
		MinCostPerFact:             "1000",
		MaxCostPerFact:             "100000000",
		ExternalPriceFeedEnabled:   true,
		ExternalPriceFeedAddress:   "oracle_feed_zrn_usd",
	}
	k.SetParams(ctx, params)

	// External feed fails.
	epfk := &mockExternalPriceFeedKeeper{price: nil, err: fmt.Errorf("feed unavailable")}
	k.SetExternalPriceFeedKeeper(epfk)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// Only TWAP available at $2.00: baseCost = 10000 * 1_000_000 / 2_000_000 = 5000
	want := big.NewInt(5000)
	if totalPrice.Cmp(want) != 0 {
		t.Errorf("want TWAP fallback cost %s when external feed unavailable, got %s", want, totalPrice)
	}
}

func TestOracle_ExternalFeedDisabled(t *testing.T) {
	// ExternalPriceFeedEnabled=false: external keeper should not be consulted.
	k, ctx, _, kk := setupKeeperWithLPK(t, big.NewInt(1_000_000), 99) // TWAP $1.00
	kk.addFact("fact-1", 600000, 0, 1, testAddr(10))

	params := types.DefaultParams()
	params.DynamicPricing = &types.DynamicPricingConfig{
		Enabled:                    true,
		TargetQueryCostUsd:         "10000",
		ManualZrnPriceUsd:          "0",
		TwapWindowBlocks:           1000,
		StalenessBlocks:            5000,
		MinCostPerFact:             "1000",
		MaxCostPerFact:             "100000000",
		ExternalPriceFeedEnabled:   false, // disabled
		ExternalPriceFeedAddress:   "oracle_feed_zrn_usd",
	}
	k.SetParams(ctx, params)

	// Wire up external feed with a different price — should be ignored.
	epfk := &mockExternalPriceFeedKeeper{price: big.NewInt(500_000)} // $0.50
	k.SetExternalPriceFeedKeeper(epfk)

	totalPrice, _ := k.CalculateQueryPrice(ctx, []string{"fact-1"}, 2000)

	// Only TWAP: 10000 * 1_000_000 / 1_000_000 = 10000
	want := big.NewInt(10000)
	if totalPrice.Cmp(want) != 0 {
		t.Errorf("want TWAP-only cost %s when external feed disabled, got %s", want, totalPrice)
	}
}

// -----------------------------------------------------------------------
// Tests: DevelopmentFundSpend
// -----------------------------------------------------------------------

// seedDevFund seeds the development_fund module account balance in the mock.
func seedDevFund(bk *mockBankKeeper, amount sdkmath.Int) {
	bk.balances[keeper.DevelopmentFund+"/uzrn"] = amount
}

func TestDevelopmentFundSpend_Success(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	// Seed development fund with 10M uzrn, limit is 10% = 1M max per proposal.
	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 100_000 // 10%
	k.SetParams(ctx, params)

	recipientAddr := testAddr(5)

	resp, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   recipientAddr,
		Amount:      "1000000", // exactly 10% of 10M — at the limit
		Description: "test spend",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AmountSpent != "1000000" {
		t.Errorf("expected AmountSpent 1000000, got %s", resp.AmountSpent)
	}

	// Development fund balance should decrease.
	devBal := bk.balances[keeper.DevelopmentFund+"/uzrn"]
	expectedDevBal := sdkmath.NewInt(9_000_000) // 10M - 1M
	if !devBal.Equal(expectedDevBal) {
		t.Errorf("expected dev fund balance %s after spend, got %s", expectedDevBal, devBal)
	}

	// Recipient should have received funds.
	recipientAccAddr, _ := sdk.AccAddressFromBech32(recipientAddr)
	recipientBal := bk.balances[recipientAccAddr.String()+"/uzrn"]
	expectedRecipient := sdkmath.NewInt(1_000_000)
	if !recipientBal.Equal(expectedRecipient) {
		t.Errorf("expected recipient balance %s, got %s", expectedRecipient, recipientBal)
	}
}

func TestDevelopmentFundSpend_BelowLimit(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	// Seed development fund with 10M uzrn.
	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 100_000 // 10% = 1M max
	k.SetParams(ctx, params)

	// Spend 500k — well under 10% limit.
	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "500000",
		Description: "small spend",
	})
	if err != nil {
		t.Fatalf("unexpected error for amount within limit: %v", err)
	}
}

func TestDevelopmentFundSpend_Unauthorized(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)

	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   testAddr(99), // not governance authority
		Recipient:   testAddr(5),
		Amount:      "1000000",
		Description: "unauthorized",
	})
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

func TestDevelopmentFundSpend_SpendingDisabled(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	// Disable spending by setting limit to 0.
	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 0
	k.SetParams(ctx, params)

	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "1000000",
		Description: "blocked",
	})
	if err == nil {
		t.Error("expected ErrSpendingDisabled")
	}
}

func TestDevelopmentFundSpend_ExceedsSpendingLimit(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	// 10M uzrn, 10% limit = 1M max.
	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 100_000 // 10%
	k.SetParams(ctx, params)

	// Try to spend 1.1M (11% of 10M — over the 10% limit).
	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "1100000",
		Description: "over limit",
	})
	if err == nil {
		t.Error("expected ErrSpendingLimitExceeded")
	}
}

func TestDevelopmentFundSpend_InsufficientFund(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	// Fund has 500k, limit is 100% — but still can't spend 1M.
	seedDevFund(bk, sdkmath.NewInt(500_000))

	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 1_000_000 // 100%
	k.SetParams(ctx, params)

	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "1000000", // more than the 500k in fund
		Description: "overdraft",
	})
	if err == nil {
		t.Error("expected ErrInsufficientDevelopmentFund")
	}
}

func TestDevelopmentFundSpend_InvalidAmount(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	// Zero amount.
	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "0",
		Description: "zero spend",
	})
	if err == nil {
		t.Error("expected ErrInvalidAmount for zero amount")
	}

	// Non-numeric amount.
	_, err = srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "not-a-number",
		Description: "bad amount",
	})
	if err == nil {
		t.Error("expected ErrInvalidAmount for non-numeric amount")
	}
}

func TestDevelopmentFundSpend_InvalidRecipient(t *testing.T) {
	k, ctx, bk, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	seedDevFund(bk, sdkmath.NewInt(10_000_000))

	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   "not-a-valid-address",
		Amount:      "1000000",
		Description: "bad recipient",
	})
	if err == nil {
		t.Error("expected error for invalid recipient address")
	}
}

func TestDevelopmentFundSpend_EmptyFund(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	// No balance seeded — fund is empty.
	params := types.DefaultParams()
	params.DevelopmentFundSpendingLimitBps = 100_000
	k.SetParams(ctx, params)

	_, err := srv.DevelopmentFundSpend(ctx, &types.MsgDevelopmentFundSpend{
		Authority:   authority,
		Recipient:   testAddr(5),
		Amount:      "1000000",
		Description: "empty fund",
	})
	if err == nil {
		t.Error("expected error when development fund is empty")
	}
}
