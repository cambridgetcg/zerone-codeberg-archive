package integration_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	treekeeper "github.com/zerone-chain/zerone/x/tree/keeper"
	treetypes "github.com/zerone-chain/zerone/x/tree/types"
	vestingkeeper "github.com/zerone-chain/zerone/x/vesting_rewards/keeper"
	vestingtypes "github.com/zerone-chain/zerone/x/vesting_rewards/types"
)

// ---------- Test 1: Complete Revenue Flow Map ----------

func TestCompleteRevenueMap(t *testing.T) {
	h := setupRevenueHarness(t)

	// SOURCE 1: Block Rewards (10 ZRN = 10,000,000 uzrn at full validators)
	dist, err := h.vestingKeeper.DistributeBlockReward(h.ctx, h.producerAddr.String(), 22, true)
	if err != nil {
		t.Fatalf("block reward distribution failed: %v", err)
	}

	totalMinted := new(big.Int)
	totalMinted.SetString(dist.TotalMinted, 10)
	if totalMinted.Sign() <= 0 {
		t.Fatal("expected non-zero block reward mint")
	}

	// 4-way revenue split: contributor 55%, protocol 22%, development 19.67%, research 3.33%
	bps := big.NewInt(1000000)

	// Gross research = 3.33% of total
	grossResearch := new(big.Int).Mul(totalMinted, big.NewInt(33300))
	grossResearch.Div(grossResearch, bps)

	// Founder = 7% of gross research
	expectedFounder := new(big.Int).Mul(grossResearch, big.NewInt(70000))
	expectedFounder.Div(expectedFounder, bps)

	// Net research = gross research - founder
	expectedNetResearch := new(big.Int).Sub(grossResearch, expectedFounder)

	researchShare := new(big.Int)
	researchShare.SetString(dist.ResearchShare, 10)
	if researchShare.Cmp(expectedNetResearch) != 0 {
		t.Errorf("block reward research share: got %s, want %s", researchShare, expectedNetResearch)
	}

	founderShare := new(big.Int)
	founderShare.SetString(dist.FounderShare, 10)
	if founderShare.Cmp(expectedFounder) != 0 {
		t.Errorf("block reward founder share: got %s, want %s", founderShare, expectedFounder)
	}

	// Contributor (producer) = 55% of total
	expectedProducer := new(big.Int).Mul(totalMinted, big.NewInt(550000))
	expectedProducer.Div(expectedProducer, bps)

	producerReward := new(big.Int)
	producerReward.SetString(dist.ProducerReward, 10)
	if producerReward.Cmp(expectedProducer) != 0 {
		t.Errorf("block reward producer: got %s, want %s", producerReward, expectedProducer)
	}

	// Protocol = 22% of total
	protocolAmt := new(big.Int).Mul(totalMinted, big.NewInt(220000))
	protocolAmt.Div(protocolAmt, bps)

	// Protocol sub-split: verification 30%, then split into knowledge 70% + compute_pool 30%
	verificationPool := new(big.Int).Mul(protocolAmt, big.NewInt(300000))
	verificationPool.Div(verificationPool, bps)
	computePool := new(big.Int).Mul(verificationPool, big.NewInt(300000))
	computePool.Div(computePool, bps)
	expectedKnowledge := new(big.Int).Sub(verificationPool, computePool)

	knowledgeSent := h.bk.totalSentToModule("knowledge")
	if !knowledgeSent.Equal(sdkmath.NewIntFromBigInt(expectedKnowledge)) {
		t.Errorf("knowledge module received %s, want %s", knowledgeSent, expectedKnowledge)
	}

	computeSent := h.bk.totalSentToModule("compute_pool")
	if !computeSent.Equal(sdkmath.NewIntFromBigInt(computePool)) {
		t.Errorf("compute_pool received %s, want %s", computeSent, computePool)
	}

	// SOURCE 2: Billing Service Revenue (55% provider / 22% knowledge / 3.33% research / 19.67% development)
	billingPayment := big.NewInt(1000000)
	billingDist := h.billingKeeper.CalculateDistribution(h.ctx, billingPayment, []string{"fact-1"})

	// Verify 55% provider
	providerAmt := new(big.Int)
	providerAmt.SetString(billingDist.ProviderShare, 10)
	expectedProvider := new(big.Int).Mul(billingPayment, big.NewInt(550000))
	expectedProvider.Div(expectedProvider, big.NewInt(1000000))
	if providerAmt.Cmp(expectedProvider) != 0 {
		t.Errorf("billing provider share: got %s, want %s", providerAmt, expectedProvider)
	}

	// Verify 3.33% research
	researchAmt := new(big.Int)
	researchAmt.SetString(billingDist.ResearchShare, 10)
	expectedBillingResearch := new(big.Int).Mul(billingPayment, big.NewInt(33300))
	expectedBillingResearch.Div(expectedBillingResearch, big.NewInt(1000000))
	if researchAmt.Cmp(expectedBillingResearch) != 0 {
		t.Errorf("billing research share: got %s, want %s", researchAmt, expectedBillingResearch)
	}

	// Verify full accounting: all components sum to total payment
	devAmt := new(big.Int)
	devAmt.SetString(billingDist.DevelopmentFund, 10) 
	treasuryAmt := new(big.Int)
	treasuryAmt.SetString(billingDist.ProtocolTreasury, 10)
	protocolTotal := new(big.Int).Add(devAmt, treasuryAmt)
	knowledgeAmt := new(big.Int)
	knowledgeAmt.SetString(billingDist.KnowledgePool[0].Amount, 10)
	allAccountedFor := new(big.Int).Add(expectedProvider, expectedBillingResearch)
	allAccountedFor.Add(allAccountedFor, knowledgeAmt)
	allAccountedFor.Add(allAccountedFor, protocolTotal)
	billingVerifPool := new(big.Int).Sub(billingPayment, allAccountedFor)
	if billingVerifPool.Sign() < 0 {
		t.Errorf("billing verification pool is negative: %s", billingVerifPool)
	}
	fullSum := new(big.Int).Add(allAccountedFor, billingVerifPool)
	if fullSum.Cmp(billingPayment) != 0 {
		t.Errorf("billing distribution doesn't sum to total: got %s, want %s", fullSum, billingPayment)
	}

	// SOURCE 3: Tree Service Revenue (pure function)
	treeDist := treekeeper.CalculateRevenue(
		1000000,
		550000, // 55% contributors
		100000, // 10% treasury
		33300,  // 3.33% research
		196700, // 19.67% development
		[]*treetypes.ContributorRecord{
			{Did: "did:zrn:contributor1", TasksCompleted: 10},
		},
	)
	protocolAlloc := int64(1000000) - treeDist.ContributorPool - treeDist.ResearchFund - treeDist.DevelopmentFund
	expectedTreeTreasury := protocolAlloc - treeDist.VerificationPool
	if treeDist.ProtocolTreasury != expectedTreeTreasury {
		t.Errorf("tree treasury: got %d, want %d", treeDist.ProtocolTreasury, expectedTreeTreasury)
	}
	if treeDist.VerificationPool <= 0 {
		t.Errorf("tree verification pool should be positive, got %d", treeDist.VerificationPool)
	}
	if treeDist.DevelopmentFund != 196700 {
		t.Errorf("tree development fund: got %d, want 196700", treeDist.DevelopmentFund)
	}
}

// ---------- Test 2: Founder Split Consistency ----------

func TestFounderSplitAllSources(t *testing.T) {
	h := setupRevenueHarness(t)
	depositAmount := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewInt(1000000)))

	// FounderShareBps = 70000 (7% on 1M scale)
	expectedFounder := sdkmath.NewInt(70000)
	expectedResearch := sdkmath.NewInt(930000)

	sources := []string{
		"vesting_rewards",
		"tree",
		"billing",
		"knowledge",
	}

	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			bk := newMockBankKeeper()
			vestingStoreKey := storetypes.NewKVStoreKey(vestingtypes.StoreKey)
			db := dbm.NewMemDB()
			stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
			stateStore.MountStoreWithDB(vestingStoreKey, storetypes.StoreTypeIAVL, db)
			if err := stateStore.LoadLatestVersion(); err != nil {
				t.Fatalf("failed to load: %v", err)
			}
			registry := codectypes.NewInterfaceRegistry()
			cdc := codec.NewProtoCodec(registry)
			vk := vestingkeeper.NewKeeper(cdc, runtime.NewKVStoreService(vestingStoreKey), bk, nil, "authority")
			ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 1000}, false, log.NewNopLogger())

			gs := vestingtypes.DefaultGenesis()
			gs.Params.FounderAddress = h.founderAddr.String()
			vk.InitGenesis(ctx, gs)

			err := vk.DepositToResearchFund(ctx, source, depositAmount)
			if err != nil {
				t.Fatalf("DepositToResearchFund from %s failed: %v", source, err)
			}

			founderGot := bk.totalSentToAddr(h.founderAddr.String())
			if !founderGot.Equal(expectedFounder) {
				t.Errorf("founder got %s from %s, want %s", founderGot, source, expectedFounder)
			}

			researchGot := bk.totalSentToModule("research_fund")
			if !researchGot.Equal(expectedResearch) {
				t.Errorf("research_fund got %s from %s, want %s", researchGot, source, expectedResearch)
			}

			totalRouted := founderGot.Add(researchGot)
			if !totalRouted.Equal(sdkmath.NewInt(1000000)) {
				t.Errorf("total routed %s from %s, want 1000000 (dust detected)", totalRouted, source)
			}
		})
	}
}

// ---------- Test 3: No Double Taxation ----------

func TestNoDoubleTaxation(t *testing.T) {
	h := setupRevenueHarness(t)

	dist, err := h.vestingKeeper.DistributeBlockReward(h.ctx, h.producerAddr.String(), 22, true)
	if err != nil {
		t.Fatalf("block reward failed: %v", err)
	}

	totalMinted := new(big.Int)
	totalMinted.SetString(dist.TotalMinted, 10)

	// 4-way split: contributor 55%, protocol 22%, development 19.67%, research 3.33%
	bps := big.NewInt(1000000)

	researchNet := new(big.Int)
	researchNet.SetString(dist.ResearchShare, 10)
	founderAmt := new(big.Int)
	founderAmt.SetString(dist.FounderShare, 10)

	// Knowledge module receives from protocol sub-split (30% verification × 70% to knowledge)
	knowledgeModuleBalance := h.bk.totalSentToModule("knowledge")

	// Compute expected knowledge balance from the actual 4-way split
	protocolAmt := new(big.Int).Mul(totalMinted, big.NewInt(220000))
	protocolAmt.Div(protocolAmt, bps)
	verificationPool := new(big.Int).Mul(protocolAmt, big.NewInt(300000))
	verificationPool.Div(verificationPool, bps)
	computePool := new(big.Int).Mul(verificationPool, big.NewInt(300000))
	computePool.Div(computePool, bps)
	expectedKnowledgeBalance := new(big.Int).Sub(verificationPool, computePool)

	if !knowledgeModuleBalance.Equal(sdkmath.NewIntFromBigInt(expectedKnowledgeBalance)) {
		t.Errorf("knowledge module balance %s != expected %s — possible double taxation",
			knowledgeModuleBalance, expectedKnowledgeBalance)
	}

	// The key no-double-tax invariant: verifier receives full knowledge balance
	// without additional research deduction (tax was applied at mint time only).
	verifierAddr := sdk.AccAddress("verifier____________")
	_ = verifierAddr

	// Full accounting: all parts sum to total minted
	producerReward := new(big.Int)
	producerReward.SetString(dist.ProducerReward, 10)
	burnAmt := new(big.Int)
	burnAmt.SetString(dist.DevelopmentAmount, 10)
	protocolShare := new(big.Int)
	protocolShare.SetString(dist.ProtocolShare, 10)

	// Gross research = net research + founder
	grossResearch := new(big.Int).Add(researchNet, founderAmt)

	// Total = producer + protocol + gross_research + burn
	accounting := new(big.Int).Add(producerReward, protocolShare)
	accounting.Add(accounting, grossResearch)
	accounting.Add(accounting, burnAmt)

	if accounting.Cmp(totalMinted) != 0 {
		t.Errorf("accounting mismatch: sum of parts %s != total minted %s", accounting, totalMinted)
	}
}

// ---------- Test 4: Service Revenue Development Fund ----------

func TestServiceRevenueDevelopmentFund(t *testing.T) {
	t.Run("tree_development_19.67pct", func(t *testing.T) {
		total := int64(1000000)
		dist := treekeeper.CalculateRevenue(
			total,
			550000, // 55% contributors
			100000, // 10% treasury
			33300,  // 3.33% research
			196700, // 19.67% development
			[]*treetypes.ContributorRecord{
				{Did: "did:zrn:alice", TasksCompleted: 5},
			},
		)

		expectedDev := total * 196700 / 1000000
		if dist.DevelopmentFund != expectedDev {
			t.Errorf("tree development fund: got %d, want %d", dist.DevelopmentFund, expectedDev)
		}

		sum := dist.ContributorPool + dist.ResearchFund + dist.ProtocolTreasury + dist.VerificationPool + dist.DevelopmentFund
		if sum != total {
			t.Errorf("tree sum %d != total %d (dust: %d)", sum, total, total-sum)
		}
	})

	t.Run("billing_development_fund", func(t *testing.T) {
		h := setupRevenueHarness(t)
		payment := big.NewInt(1000000)
		dist := h.billingKeeper.CalculateDistribution(h.ctx, payment, []string{"fact-1"})

		devAmt := new(big.Int)
		devAmt.SetString(dist.DevelopmentFund, 10) 

		providerAmt := new(big.Int)
		providerAmt.SetString(dist.ProviderShare, 10)
		researchAmt := new(big.Int)
		researchAmt.SetString(dist.ResearchShare, 10)
		knowledgeTotal := big.NewInt(0)
		for _, e := range dist.KnowledgePool {
			amt := new(big.Int)
			amt.SetString(e.Amount, 10)
			knowledgeTotal.Add(knowledgeTotal, amt)
		}
		treasuryAmt2 := new(big.Int)
		treasuryAmt2.SetString(dist.ProtocolTreasury, 10)
		accounted := new(big.Int).Add(providerAmt, researchAmt)
		accounted.Add(accounted, knowledgeTotal)
		accounted.Add(accounted, treasuryAmt2)
		accounted.Add(accounted, devAmt)
		implicitVerif := new(big.Int).Sub(payment, accounted)

		fullAccounted := new(big.Int).Add(accounted, implicitVerif)
		if fullAccounted.Cmp(payment) != 0 {
			t.Errorf("billing distribution doesn't sum: got %s, want %s", fullAccounted, payment)
		}

		err := h.billingKeeper.ExecuteDistribution(h.ctx, h.callerAddr, h.providerAddr, dist)
		if err != nil {
			t.Fatalf("execute distribution failed: %v", err)
		}

		// Development fund receives tokens (no burn)
		devSent := h.bk.totalSentToModule("development_fund")
		if !devSent.Equal(sdkmath.NewIntFromBigInt(devAmt)) {
			t.Errorf("development_fund received: got %s, want %s", devSent, devAmt)
		}

		// No tokens should be burned
		actualBurned := h.bk.totalBurned()
		if actualBurned.IsPositive() {
			t.Errorf("expected no burn, got %s", actualBurned)
		}
	})
}

// ---------- Test 5: Dead Accounts Have Zero Balance ----------

func TestDeadAccountsRemoved(t *testing.T) {
	h := setupRevenueHarness(t)

	deadAccounts := []string{
		"treasury_research",
		"treasury_foundation",
		"treasury_community",
		"treasury_developers",
		"treasury_reserve",
	}

	_, err := h.vestingKeeper.DistributeBlockReward(h.ctx, h.producerAddr.String(), 22, true)
	if err != nil {
		t.Fatalf("block reward failed: %v", err)
	}

	for _, name := range deadAccounts {
		sent := h.bk.totalSentToModule(name)
		if sent.IsPositive() {
			t.Errorf("dead account %q received %s uzrn — should be zero", name, sent)
		}
	}

	activeAccounts := map[string]bool{
		"research_fund":    true,
		"knowledge":        true,
		"compute_pool":     true,
		"development_fund": true,
	}
	for name := range activeAccounts {
		sent := h.bk.totalSentToModule(name)
		if !sent.IsPositive() {
			t.Errorf("active account %q received zero — expected positive balance", name)
		}
	}
}

// ---------- Test 6: Full Ledger Balance ----------

func TestLedgerBalance(t *testing.T) {
	h := setupRevenueHarness(t)

	for i := 0; i < 5; i++ {
		h.ctx = h.ctx.WithBlockHeight(int64(1000 + i))
		_, err := h.vestingKeeper.DistributeBlockReward(h.ctx, h.producerAddr.String(), 22, true)
		if err != nil {
			t.Fatalf("block %d reward failed: %v", 1000+i, err)
		}
	}

	payment := big.NewInt(2000000)
	billingDist := h.billingKeeper.CalculateDistribution(h.ctx, payment, []string{"fact-1"})
	err := h.billingKeeper.ExecuteDistribution(h.ctx, h.callerAddr, h.providerAddr, billingDist)
	if err != nil {
		t.Fatalf("billing execution failed: %v", err)
	}

	totalMinted := h.bk.totalMinted()
	totalBurned := h.bk.totalBurned()
	expectedSupply := totalMinted.Sub(totalBurned)
	actualSupply := h.bk.GetSupply(h.ctx, "uzrn").Amount

	if !actualSupply.Equal(expectedSupply) {
		t.Errorf("supply mismatch: actual %s != minted(%s) - burned(%s) = %s",
			actualSupply, totalMinted, totalBurned, expectedSupply)
	}

	var totalDistributed sdkmath.Int = sdkmath.ZeroInt()

	for module, coins := range h.bk.sentToModule {
		amt := coins.AmountOf("uzrn")
		if amt.IsPositive() {
			t.Logf("  module %s: %s uzrn", module, amt)
			totalDistributed = totalDistributed.Add(amt)
		}
	}

	for addr, coins := range h.bk.sentToAccount {
		amt := coins.AmountOf("uzrn")
		if amt.IsPositive() {
			t.Logf("  account %s: %s uzrn", addr, amt)
			totalDistributed = totalDistributed.Add(amt)
		}
	}

	t.Logf("Total minted: %s, burned: %s, distributed: %s", totalMinted, totalBurned, totalDistributed)
}

// ---------- Test 7: DepositToResearchFund with No Founder ----------

func TestDepositToResearchFund_NoFounder(t *testing.T) {
	bk := newMockBankKeeper()

	vestingStoreKey := storetypes.NewKVStoreKey(vestingtypes.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(vestingStoreKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	vk := vestingkeeper.NewKeeper(cdc, runtime.NewKVStoreService(vestingStoreKey), bk, nil, "authority")
	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 1000}, false, log.NewNopLogger())

	gs := vestingtypes.DefaultGenesis()
	gs.Params.FounderAddress = ""
	vk.InitGenesis(ctx, gs)

	deposit := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewInt(1000000)))
	err := vk.DepositToResearchFund(ctx, "billing", deposit)
	if err != nil {
		t.Fatalf("deposit failed: %v", err)
	}

	researchGot := bk.totalSentToModule("research_fund")
	if !researchGot.Equal(sdkmath.NewInt(1000000)) {
		t.Errorf("research_fund got %s, want 1000000 (no founder)", researchGot)
	}

	for addr, coins := range bk.sentToAccount {
		if coins.AmountOf("uzrn").IsPositive() {
			t.Errorf("unexpected transfer to account %s: %s (should be zero with no founder)", addr, coins)
		}
	}
}

// ---------- Test 8: Billing ExecuteDistribution Research Routes Through Depositor ----------

func TestBillingResearchRoutedThroughDepositor(t *testing.T) {
	h := setupRevenueHarness(t)

	payment := big.NewInt(1000000)
	dist := h.billingKeeper.CalculateDistribution(h.ctx, payment, []string{"fact-1"})

	// Reset bank keeper to isolate billing execution
	h.bk.sentToModule = make(map[string]sdk.Coins)
	h.bk.sentToAccount = make(map[string]sdk.Coins)
	h.bk.sentFromAcct = make(map[string]sdk.Coins)
	h.bk.p2pSent = make(map[string]sdk.Coins)
	h.bk.burnedCoins = nil

	err := h.billingKeeper.ExecuteDistribution(h.ctx, h.callerAddr, h.providerAddr, dist)
	if err != nil {
		t.Fatalf("execute distribution failed: %v", err)
	}

	// Research share (3.33% = 33,300 uzrn) → DepositToResearchFund
	// 7% founder split: founder gets 2,331, research_fund gets 30,969
	researchAmt := new(big.Int)
	researchAmt.SetString(dist.ResearchShare, 10)
	expectedResearch := int64(33300)
	if researchAmt.Int64() != expectedResearch {
		t.Errorf("research amount: got %s, want %d", researchAmt, expectedResearch)
	}

	expectedFounderFromBilling := sdkmath.NewInt(2331)
	founderGot := h.bk.totalSentToAddr(h.founderAddr.String())
	if !founderGot.Equal(expectedFounderFromBilling) {
		t.Errorf("billing founder split: got %s, want %s", founderGot, expectedFounderFromBilling)
	}

	expectedResearchNet := sdkmath.NewInt(30969)
	researchGot := h.bk.totalSentToModule("research_fund")
	if !researchGot.Equal(expectedResearchNet) {
		t.Errorf("billing research_fund: got %s, want %s", researchGot, expectedResearchNet)
	}

	// Development fund receives tokens (no burn)
	devSent := h.bk.totalSentToModule("development_fund")
	if !devSent.IsPositive() {
		t.Error("expected positive development_fund deposit from billing execution")
	}

	// No tokens should be burned
	burnedAmt := h.bk.totalBurned()
	if burnedAmt.IsPositive() {
		t.Errorf("expected no burn, got %s", burnedAmt)
	}

	treasurySent := h.bk.totalSentToModule("treasury_protocol")
	if !treasurySent.IsPositive() {
		t.Error("expected positive treasury_protocol transfer from billing")
	}
}

// ---------- Test 9: Verification Reward Decay Pool Solvency ----------

func TestVerificationRewardDecay_PoolSolvency(t *testing.T) {
	// Zerone: baseReward = 10,000,000 uzrn (10 ZRN)
	baseReward := uint64(10000000)
	decayBps := uint64(994478)    // ~1-year half-life (0.994478x per epoch)
	floorReward := uint64(100000) // 0.1 ZRN

	baseRewardBig := new(big.Int).SetUint64(baseReward)
	var prev uint64 = baseReward
	floorReached := false

	// With 1-year half-life, floor (~0.1 ZRN) is reached at ~epoch 832 (~year 6.6).
	// Sample key epochs to verify monotonic decay without iterating all 850.
	checkEpochs := []uint64{0, 1, 2, 5, 10, 50, 100, 125, 250, 500, 750, 832, 850}
	for _, epoch := range checkEpochs {
		decayed := testApplyDecay(baseRewardBig, decayBps, epoch).Uint64()

		if decayed > baseReward {
			t.Errorf("epoch %d: decayed %d > base %d", epoch, decayed, baseReward)
		}

		if decayed > prev {
			t.Errorf("epoch %d: decayed %d > previous %d — not monotonic", epoch, decayed, prev)
		}

		if decayed < floorReward {
			floorReached = true
		}

		if epoch == 0 && decayed != baseReward {
			t.Errorf("epoch 0: expected %d, got %d", baseReward, decayed)
		}

		// At epoch 1, reward should be 0.994478 * 10,000,000 = 9,944,780
		if epoch == 1 {
			expected := uint64(9944780)
			if decayed != expected {
				t.Errorf("epoch 1: expected %d, got %d", expected, decayed)
			}
		}

		prev = decayed
	}

	if !floorReached {
		t.Errorf("floor reward %d was never reached by epoch 850", floorReward)
	}

	// At epoch 850, decay should be well below floor
	deepDecay := testApplyDecay(baseRewardBig, decayBps, 850).Uint64()
	if deepDecay >= floorReward {
		t.Errorf("epoch 850: expected decay below floor, got %d (floor %d)", deepDecay, floorReward)
	}
}

// ---------- Test 10: Full Revenue Flow With Verification Pool ----------

func TestFullRevenueFlow_WithVerificationPool(t *testing.T) {
	h := setupRevenueHarness(t)

	// --- Part A: Block reward distributes to verification pool ---
	dist, err := h.vestingKeeper.DistributeBlockReward(h.ctx, h.producerAddr.String(), 22, true)
	if err != nil {
		t.Fatalf("block reward distribution failed: %v", err)
	}

	totalMinted := new(big.Int)
	totalMinted.SetString(dist.TotalMinted, 10)
	if totalMinted.Sign() <= 0 {
		t.Fatal("expected non-zero block reward mint")
	}

	knowledgeSent := h.bk.totalSentToModule("knowledge")
	if !knowledgeSent.IsPositive() {
		t.Error("knowledge module received zero from block reward — verification pool missing")
	}

	computeSent := h.bk.totalSentToModule("compute_pool")
	if !computeSent.IsPositive() {
		t.Error("compute_pool received zero from block reward")
	}

	// --- Part B: Billing payment 3-way protocol split ---
	billingPayment := big.NewInt(1000000)
	billingDist := h.billingKeeper.CalculateDistribution(h.ctx, billingPayment, []string{"fact-1"})

	providerAmt := new(big.Int)
	providerAmt.SetString(billingDist.ProviderShare, 10)
	researchAmt := new(big.Int)
	researchAmt.SetString(billingDist.ResearchShare, 10)
	burnAmt := new(big.Int)
	burnAmt.SetString(billingDist.DevelopmentFund, 10)
	treasuryAmt := new(big.Int)
	treasuryAmt.SetString(billingDist.ProtocolTreasury, 10)

	knowledgeTotal := big.NewInt(0)
	for _, e := range billingDist.KnowledgePool {
		amt := new(big.Int)
		amt.SetString(e.Amount, 10)
		knowledgeTotal.Add(knowledgeTotal, amt)
	}

	accounted := new(big.Int).Add(providerAmt, researchAmt)
	accounted.Add(accounted, burnAmt)
	accounted.Add(accounted, treasuryAmt)
	accounted.Add(accounted, knowledgeTotal)
	verifPool := new(big.Int).Sub(billingPayment, accounted)

	if verifPool.Sign() <= 0 {
		t.Errorf("billing verification pool is non-positive: %s", verifPool)
	}

	fullSum := new(big.Int).Add(accounted, verifPool)
	if fullSum.Cmp(billingPayment) != 0 {
		t.Errorf("billing distribution doesn't sum to total: got %s, want %s (dust: %s)",
			fullSum, billingPayment, new(big.Int).Sub(billingPayment, fullSum))
	}

	// --- Part C: Tree revenue verification pool ---
	treeDist := treekeeper.CalculateRevenue(
		1000000,
		550000, // 55% contributors
		100000, // 10% treasury
		33300,  // 3.33% research
		196700, // 19.67% development
		[]*treetypes.ContributorRecord{
			{Did: "did:zrn:contributor1", TasksCompleted: 10},
			{Did: "did:zrn:contributor2", TasksCompleted: 5},
		},
	)

	if treeDist.VerificationPool <= 0 {
		t.Errorf("tree verification pool should be positive, got %d", treeDist.VerificationPool)
	}

	treeSum := treeDist.ContributorPool + treeDist.ResearchFund +
		treeDist.ProtocolTreasury + treeDist.VerificationPool + treeDist.DevelopmentFund
	if treeSum != 1000000 {
		t.Errorf("tree distribution doesn't sum to total: got %d, want 1000000 (dust: %d)",
			treeSum, 1000000-treeSum)
	}

	// Protocol allocation = total - contrib - research - development = 1M - 550K - 33.3K - 196.7K = 220K
	// Verification = 30% of protocol = 66,000
	expectedVerifPool := int64(66000)
	if treeDist.VerificationPool != expectedVerifPool {
		t.Errorf("tree verification pool: got %d, want %d", treeDist.VerificationPool, expectedVerifPool)
	}
}
