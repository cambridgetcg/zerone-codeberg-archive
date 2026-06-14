package keeper_test

import (
	"context"
	"fmt"
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

	"github.com/zerone-chain/zerone/x/research/keeper"
	"github.com/zerone-chain/zerone/x/research/types"
)

// -----------------------------------------------------------------------
// Mock BankKeeper
// -----------------------------------------------------------------------

type mockBankKeeper struct {
	balances   map[string]sdkmath.Int
	moduleSent sdkmath.Int
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{
		balances:   make(map[string]sdkmath.Int),
		moduleSent: sdkmath.ZeroInt(),
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
		senderKey := senderModule + "/" + coin.Denom
		senderBal, ok := m.balances[senderKey]
		if !ok {
			senderBal = sdkmath.ZeroInt()
		}
		if senderBal.LT(coin.Amount) {
			return fmt.Errorf("insufficient module balance to send")
		}
		m.balances[senderKey] = senderBal.Sub(coin.Amount)

		recipientKey := recipientModule + "/" + coin.Denom
		recipientBal, ok := m.balances[recipientKey]
		if !ok {
			recipientBal = sdkmath.ZeroInt()
		}
		m.balances[recipientKey] = recipientBal.Add(coin.Amount)
		m.moduleSent = m.moduleSent.Add(coin.Amount)
	}
	return nil
}

// GetModuleBalance returns the mock balance for a module name + denom.
// Key format matches SendCoinsFromAccountToModule (moduleName + "/" + denom).
func (m *mockBankKeeper) GetModuleBalance(_ context.Context, moduleName string, denom string) sdk.Coin {
	key := moduleName + "/" + denom
	bal, ok := m.balances[key]
	if !ok {
		return sdk.NewCoin(denom, sdkmath.ZeroInt())
	}
	return sdk.NewCoin(denom, bal)
}

// setModuleBalance is a test helper to pre-populate a module account balance.
func (m *mockBankKeeper) setModuleBalance(moduleName string, denom string, amount sdkmath.Int) {
	m.balances[moduleName+"/"+denom] = amount
}

// -----------------------------------------------------------------------
// Setup
// -----------------------------------------------------------------------

var testAuthority = sdk.AccAddress([]byte("authority-addr------")).String()

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper) {
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

	k := keeper.NewKeeper(runtime.NewKVStoreService(storeKey), cdc, testAuthority, bk)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())

	return k, ctx, bk
}

func testAddr(i int) sdk.AccAddress {
	return sdk.AccAddress([]byte(fmt.Sprintf("test-addr-%010d", i)))
}

func testAddrStr(i int) string {
	return testAddr(i).String()
}

// -----------------------------------------------------------------------
// Tests: Research Submission
// -----------------------------------------------------------------------

func TestSubmitResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, err := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Test Research",
		Description: "Testing research submission",
		Domain:      "physics",
		Stake:       "1000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ResearchId == "" {
		t.Fatal("expected non-empty research_id")
	}

	// Verify stored
	research, found := k.GetResearch(ctx, resp.ResearchId)
	if !found {
		t.Fatal("research not found after submission")
	}
	if research.Status != "submitted" {
		t.Errorf("expected status 'submitted', got %q", research.Status)
	}
	if research.Submitter != submitter.String() {
		t.Errorf("expected submitter %s, got %s", submitter.String(), research.Submitter)
	}
	if research.Stake != "1000000" {
		t.Errorf("expected stake 1000000, got %s", research.Stake)
	}

	// Verify stake was deducted
	remaining := bk.balances[submitter.String()+"/uzrn"]
	if !remaining.Equal(sdkmath.NewInt(4000000)) {
		t.Errorf("expected remaining balance 4000000, got %s", remaining.String())
	}
}

func TestSubmitResearchInsufficientStake(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(500000)) // less than min stake

	msgServer := keeper.NewMsgServerImpl(k)
	_, err := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Test Research",
		Description: "Testing",
		Domain:      "physics",
		Stake:       "500000", // below 1000000 minimum
	})
	if err == nil {
		t.Fatal("expected error for insufficient stake, got nil")
	}
}

// -----------------------------------------------------------------------
// Tests: Review
// -----------------------------------------------------------------------

func TestReviewResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	// Submit research
	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Test Research",
		Description: "Testing",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Review
	reviewer := testAddrStr(2)
	_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     reviewer,
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good work",
		QualityScore: 80,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify research updated
	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.ReviewCount != 1 {
		t.Errorf("expected review_count 1, got %d", research.ReviewCount)
	}
	if research.ApproveCount != 1 {
		t.Errorf("expected approve_count 1, got %d", research.ApproveCount)
	}
	if research.AggregateScore != 80 {
		t.Errorf("expected aggregate_score 80, got %d", research.AggregateScore)
	}
	if research.Status != "under_review" {
		t.Errorf("expected status 'under_review', got %q", research.Status)
	}
}

func TestReviewResearchDuplicate(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Test",
		Description: "Testing",
		Domain:      "physics",
		Stake:       "1000000",
	})

	reviewer := testAddrStr(2)
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     reviewer,
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "OK",
		QualityScore: 70,
	})

	// Second review by same reviewer should fail
	_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     reviewer,
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_REJECT,
		Reasoning:    "Changed my mind",
		QualityScore: 30,
	})
	if err == nil {
		t.Fatal("expected error for duplicate review, got nil")
	}
}

// -----------------------------------------------------------------------
// Tests: Resolution
// -----------------------------------------------------------------------

func submitAndReview(t *testing.T, k keeper.Keeper, ctx sdk.Context, bk *mockBankKeeper, scores []uint32) string {
	t.Helper()
	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(10000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, err := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research for Resolution",
		Description: "Testing resolution",
		Domain:      "math",
		Stake:       "1000000",
	})
	if err != nil {
		t.Fatalf("submit error: %v", err)
	}

	for i, score := range scores {
		reviewer := testAddrStr(100 + i)
		_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			Reviewer:     reviewer,
			ResearchId:   resp.ResearchId,
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			Reasoning:    "Review",
			QualityScore: score,
		})
		if err != nil {
			t.Fatalf("review %d error: %v", i, err)
		}
	}
	return resp.ResearchId
}

func TestResolveResearchAccepted(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	// 3 reviews with scores averaging >= 70 (threshold)
	researchId := submitAndReview(t, k, ctx, bk, []uint32{80, 75, 70})

	// Pre-resolution: check module has stake
	moduleBal := bk.balances[types.ModuleName+"/uzrn"]
	if moduleBal.LT(sdkmath.NewInt(1000000)) {
		t.Fatal("module should hold stake before resolution")
	}

	submitterBefore := bk.balances[testAddr(1).String()+"/uzrn"]

	msgServer := keeper.NewMsgServerImpl(k)
	resolveResp, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAuthority,
		ResearchId: researchId,
	})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if resolveResp.Outcome != types.ResearchOutcome_RESEARCH_OUTCOME_ACCEPTED {
		t.Errorf("expected ACCEPTED outcome, got %v", resolveResp.Outcome)
	}

	// Check research status
	research, _ := k.GetResearch(ctx, researchId)
	if research.Status != "accepted" {
		t.Errorf("expected status 'accepted', got %q", research.Status)
	}

	// Stake returned to submitter
	submitterAfter := bk.balances[testAddr(1).String()+"/uzrn"]
	returned := submitterAfter.Sub(submitterBefore)
	if !returned.Equal(sdkmath.NewInt(1000000)) {
		t.Errorf("expected 1000000 returned, got %s", returned.String())
	}
}

func TestResolveResearchRejected(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	// 3 reviews with scores averaging < 70 (threshold)
	researchId := submitAndReview(t, k, ctx, bk, []uint32{50, 60, 55})

	bk.moduleSent = sdkmath.ZeroInt()

	msgServer := keeper.NewMsgServerImpl(k)
	resolveResp, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAuthority,
		ResearchId: researchId,
	})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if resolveResp.Outcome != types.ResearchOutcome_RESEARCH_OUTCOME_REJECTED {
		t.Errorf("expected REJECTED outcome, got %v", resolveResp.Outcome)
	}

	// Check research status
	research, _ := k.GetResearch(ctx, researchId)
	if research.Status != "rejected" {
		t.Errorf("expected status 'rejected', got %q", research.Status)
	}

	// Some amount sent to development fund (33% of 1000000 = 330000)
	if bk.moduleSent.IsZero() {
		t.Error("expected some tokens to be sent to development fund on rejection")
	}
}

// -----------------------------------------------------------------------
// Tests: Bounty
// -----------------------------------------------------------------------

func TestCreateBounty(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, err := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Find error in paper X",
		Description:    "Investigate",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000, // well past minimum
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BountyId == "" {
		t.Fatal("expected non-empty bounty_id")
	}

	bounty, found := k.GetBounty(ctx, resp.BountyId)
	if !found {
		t.Fatal("bounty not found after creation")
	}
	if bounty.Status != "open" {
		t.Errorf("expected status 'open', got %q", bounty.Status)
	}
	if bounty.Reward != "5000000" {
		t.Errorf("expected reward 5000000, got %s", bounty.Reward)
	}

	// Reward locked from creator
	remaining := bk.balances[creator.String()+"/uzrn"]
	if !remaining.Equal(sdkmath.NewInt(95000000)) {
		t.Errorf("expected 95000000 remaining, got %s", remaining.String())
	}
}

func TestClaimBounty(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	claimer := testAddrStr(2)
	_, err := msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  claimer,
		BountyId: createResp.BountyId,
	})
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}

	bounty, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty.Status != "claimed" {
		t.Errorf("expected status 'claimed', got %q", bounty.Status)
	}
	if bounty.ClaimedBy != claimer {
		t.Errorf("expected claimed_by %s, got %s", claimer, bounty.ClaimedBy)
	}
}

func TestFulfillBounty(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	claimer := testAddr(2)
	msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  claimer.String(),
		BountyId: createResp.BountyId,
	})

	_, err := msgServer.FulfillBounty(ctx, &types.MsgFulfillBounty{
		Authority: testAuthority,
		BountyId:  createResp.BountyId,
		Claimer:   claimer.String(),
	})
	if err != nil {
		t.Fatalf("fulfill error: %v", err)
	}

	bounty, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty.Status != "fulfilled" {
		t.Errorf("expected status 'fulfilled', got %q", bounty.Status)
	}

	// Reward paid to claimer
	claimerBal := bk.balances[claimer.String()+"/uzrn"]
	if !claimerBal.Equal(sdkmath.NewInt(5000000)) {
		t.Errorf("expected claimer to have 5000000, got %s", claimerBal.String())
	}
}

// -----------------------------------------------------------------------
// Tests: Bounty Expiry
// -----------------------------------------------------------------------

func TestBountyExpiry(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Also create a claimed bounty
	createResp2, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty 2",
		Description:    "Test 2",
		Reward:         "3000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	claimer := testAddrStr(2)
	msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  claimer,
		BountyId: createResp2.BountyId,
	})

	creatorBalBefore := bk.balances[creator.String()+"/uzrn"]

	// Advance past deadline
	ctx = ctx.WithBlockHeight(int64(uint64(ctx.BlockHeight()) + 50001))
	k.ExpireBounties(ctx)

	// Open bounty should be expired, reward returned
	bounty1, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty1.Status != "expired" {
		t.Errorf("expected status 'expired', got %q", bounty1.Status)
	}

	creatorBalAfter := bk.balances[creator.String()+"/uzrn"]
	returned := creatorBalAfter.Sub(creatorBalBefore)
	if !returned.Equal(sdkmath.NewInt(5000000)) {
		t.Errorf("expected 5000000 returned to creator, got %s", returned.String())
	}

	// Claimed bounty should be reopened
	bounty2, _ := k.GetBounty(ctx, createResp2.BountyId)
	if bounty2.Status != "open" {
		t.Errorf("expected claimed bounty reopened to 'open', got %q", bounty2.Status)
	}
	if bounty2.ClaimedBy != "" {
		t.Errorf("expected claimed_by cleared, got %q", bounty2.ClaimedBy)
	}
}

// -----------------------------------------------------------------------
// Tests: Genesis Import/Export
// -----------------------------------------------------------------------

func TestGenesisImportExport(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	// Create some state
	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)

	// Submit research
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Genesis Test",
		Description: "Testing genesis",
		Domain:      "math",
		Stake:       "1000000",
	})

	// Review it
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(2),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 85,
	})

	// Create a bounty
	msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        submitter.String(),
		Title:          "Test Bounty",
		Description:    "Testing",
		Reward:         "2000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Fund treasury
	bk.setBalance(testAddr(3), "uzrn", sdkmath.NewInt(50000000))
	msgServer.FundResearch(ctx, &types.MsgFundResearch{
		Funder: testAddrStr(3),
		Amount: "5000000",
	})

	// Export genesis
	exportedState := k.ExportGenesis(ctx)

	if exportedState.Params == nil {
		t.Fatal("exported params is nil")
	}
	if len(exportedState.Researches) != 1 {
		t.Errorf("expected 1 research, got %d", len(exportedState.Researches))
	}
	if len(exportedState.Bounties) != 1 {
		t.Errorf("expected 1 bounty, got %d", len(exportedState.Bounties))
	}
	if len(exportedState.PeerReviews) != 1 {
		t.Errorf("expected 1 peer review, got %d", len(exportedState.PeerReviews))
	}
	if exportedState.TreasuryBalance == nil || exportedState.TreasuryBalance.Balance != "5000000" {
		t.Errorf("expected treasury balance 5000000, got %v", exportedState.TreasuryBalance)
	}

	// Re-import into fresh keeper
	k2, ctx2, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, exportedState)

	// Verify imported state
	research, found := k2.GetResearch(ctx2, resp.ResearchId)
	if !found {
		t.Fatal("imported research not found")
	}
	if research.Title != "Genesis Test" {
		t.Errorf("expected title 'Genesis Test', got %q", research.Title)
	}

	treasury := k2.GetTreasuryBalance(ctx2)
	if treasury != "5000000" {
		t.Errorf("expected imported treasury 5000000, got %s", treasury)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Submission Edge Cases
// -----------------------------------------------------------------------

func TestSubmitResearchSequentialIds(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	msg := &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	}

	resp1, err := msgServer.SubmitResearch(ctx, msg)
	if err != nil {
		t.Fatalf("submit 1 error: %v", err)
	}
	resp2, err := msgServer.SubmitResearch(ctx, msg)
	if err != nil {
		t.Fatalf("submit 2 error: %v", err)
	}
	resp3, err := msgServer.SubmitResearch(ctx, msg)
	if err != nil {
		t.Fatalf("submit 3 error: %v", err)
	}

	if resp1.ResearchId != "RES-1" || resp2.ResearchId != "RES-2" || resp3.ResearchId != "RES-3" {
		t.Errorf("expected sequential IDs RES-1,RES-2,RES-3, got %s,%s,%s",
			resp1.ResearchId, resp2.ResearchId, resp3.ResearchId)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Review Edge Cases
// -----------------------------------------------------------------------

func TestReviewResearchMultipleReviewers(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Three distinct reviewers
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(10),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 80,
	})
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(11),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "OK",
		QualityScore: 70,
	})
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(12),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_REJECT,
		Reasoning:    "Weak",
		QualityScore: 40,
	})

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.ReviewCount != 3 {
		t.Errorf("expected review_count 3, got %d", research.ReviewCount)
	}
	if research.ApproveCount != 2 {
		t.Errorf("expected approve_count 2, got %d", research.ApproveCount)
	}
	if research.RejectCount != 1 {
		t.Errorf("expected reject_count 1, got %d", research.RejectCount)
	}
	// (80+70+40)/3 = 63
	if research.AggregateScore != 63 {
		t.Errorf("expected aggregate_score 63, got %d", research.AggregateScore)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Resolution Edge Cases
// -----------------------------------------------------------------------

func TestResolveResearchInsufficientReviews(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Only 1 review (need 3)
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(10),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 80,
	})

	_, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAuthority,
		ResearchId: resp.ResearchId,
	})
	if err == nil {
		t.Fatal("expected error for insufficient reviews, got nil")
	}
}

func TestResolveResearchUnauthorized(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// non-authority tries to resolve
	_, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAddrStr(99),
		ResearchId: resp.ResearchId,
	})
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Challenge System
// -----------------------------------------------------------------------

func TestChallengeResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	challenger := testAddr(2)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))
	bk.setBalance(challenger, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	_, err := msgServer.ChallengeResearch(ctx, &types.MsgChallengeResearch{
		Challenger: challenger.String(),
		ResearchId: resp.ResearchId,
		Reason:     "Methodology flawed",
		Stake:      "1000000",
	})
	if err != nil {
		t.Fatalf("ChallengeResearch failed: %v", err)
	}

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "challenged" {
		t.Errorf("expected status 'challenged', got %q", research.Status)
	}
}

func TestChallengeResearchNotFound(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	challenger := testAddr(2)
	bk.setBalance(challenger, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	_, err := msgServer.ChallengeResearch(ctx, &types.MsgChallengeResearch{
		Challenger: challenger.String(),
		ResearchId: "RES-999",
		Reason:     "Does not exist",
		Stake:      "1000000",
	})
	if err == nil {
		t.Fatal("expected error for non-existent research, got nil")
	}
}

func TestChallengeResearchAlreadyAccepted(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	challenger := testAddr(2)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))
	bk.setBalance(challenger, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Manually set status to accepted
	research, _ := k.GetResearch(ctx, resp.ResearchId)
	research.Status = "accepted"
	k.SetResearch(ctx, research)

	_, err := msgServer.ChallengeResearch(ctx, &types.MsgChallengeResearch{
		Challenger: challenger.String(),
		ResearchId: resp.ResearchId,
		Reason:     "Too late",
		Stake:      "1000000",
	})
	if err == nil {
		t.Fatal("expected error challenging accepted research, got nil")
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Bounty Edge Cases
// -----------------------------------------------------------------------

func TestCreateBountyDeadlineTooSoon(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	_, err := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Too soon",
		Description:    "Desc",
		Reward:         "5000000",
		DeadlineHeight: 200, // not far enough (need 100 + 34272 + 1)
	})
	if err == nil {
		t.Fatal("expected deadline too soon error, got nil")
	}
}

func TestCreateBountyExceedsMaxReward(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(1000000000000))

	msgServer := keeper.NewMsgServerImpl(k)
	_, err := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Too rich",
		Description:    "Desc",
		Reward:         "99999000000000", // way over max of 10000000000
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})
	if err == nil {
		t.Fatal("expected exceeds max reward error, got nil")
	}
}

func TestClaimBountyNotOpen(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Claim once
	msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  testAddrStr(2),
		BountyId: createResp.BountyId,
	})

	// Try to claim again
	_, err := msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  testAddrStr(3),
		BountyId: createResp.BountyId,
	})
	if err == nil {
		t.Fatal("expected bounty not open error, got nil")
	}
}

func TestClaimBountyExpired(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Advance past deadline
	expiredCtx := ctx.WithBlockHeight(int64(uint64(ctx.BlockHeight()) + 50001))

	_, err := msgServer.ClaimBounty(expiredCtx, &types.MsgClaimBounty{
		Claimer:  testAddrStr(2),
		BountyId: createResp.BountyId,
	})
	if err == nil {
		t.Fatal("expected bounty expired error, got nil")
	}
}

func TestFulfillBountyNotClaimed(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Try to fulfill without claiming
	_, err := msgServer.FulfillBounty(ctx, &types.MsgFulfillBounty{
		Authority: testAuthority,
		BountyId:  createResp.BountyId,
	})
	if err == nil {
		t.Fatal("expected bounty not claimed error, got nil")
	}
}

func TestFulfillBountyUnauthorized(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Bounty",
		Description:    "Test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	claimer := testAddr(2)
	msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  claimer.String(),
		BountyId: createResp.BountyId,
	})

	// Non-authority tries to fulfill
	_, err := msgServer.FulfillBounty(ctx, &types.MsgFulfillBounty{
		Authority: testAddrStr(99),
		BountyId:  createResp.BountyId,
	})
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Treasury
// -----------------------------------------------------------------------

func TestFundResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	funder := testAddr(1)
	bk.setBalance(funder, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	_, err := msgServer.FundResearch(ctx, &types.MsgFundResearch{
		Funder: funder.String(),
		Amount: "5000000",
	})
	if err != nil {
		t.Fatalf("FundResearch failed: %v", err)
	}

	bal := k.GetTreasuryBalance(ctx)
	if bal != "5000000" {
		t.Errorf("expected treasury balance 5000000, got %s", bal)
	}

	// Fund again
	_, err = msgServer.FundResearch(ctx, &types.MsgFundResearch{
		Funder: funder.String(),
		Amount: "3000000",
	})
	if err != nil {
		t.Fatalf("FundResearch (2nd) failed: %v", err)
	}

	bal = k.GetTreasuryBalance(ctx)
	if bal != "8000000" {
		t.Errorf("expected treasury balance 8000000, got %s", bal)
	}
}

func TestTreasuryBalance(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	bal := k.GetTreasuryBalance(ctx)
	if bal != "0" {
		t.Errorf("expected initial treasury 0, got %s", bal)
	}

	k.SetTreasuryBalance(ctx, "12345")
	bal = k.GetTreasuryBalance(ctx)
	if bal != "12345" {
		t.Errorf("expected treasury 12345, got %s", bal)
	}
}

// TestConsolidatedResearchBalance verifies Treasury Gap 6 fix:
// GetConsolidatedResearchBalance surfaces BOTH explicit treasury funds
// (FundResearch deposits tracked in KV) AND billing research_fund allocation
// (previously invisible to x/research). The dual accounting risk is now
// addressed by making both sources visible in one query.
func TestConsolidatedResearchBalance(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	// Initially both sources are zero.
	consolidated := k.GetConsolidatedResearchBalance(ctx)
	if consolidated != "0" {
		t.Errorf("expected consolidated 0, got %s", consolidated)
	}

	// Simulate explicit FundResearch deposit (KV tracker).
	k.SetTreasuryBalance(ctx, "5000000")
	consolidated = k.GetConsolidatedResearchBalance(ctx)
	if consolidated != "5000000" {
		t.Errorf("expected consolidated 5000000 (KV only), got %s", consolidated)
	}

	// Simulate billing research_fund allocation arriving via bank.
	bk.setModuleBalance("research_fund", "uzrn", sdkmath.NewInt(2000000))
	consolidated = k.GetConsolidatedResearchBalance(ctx)
	if consolidated != "7000000" {
		t.Errorf("expected consolidated 7000000 (KV + billing), got %s", consolidated)
	}

	// GetBillingResearchFundBalance reads billing source independently.
	billingBal := k.GetBillingResearchFundBalance(ctx)
	if billingBal != "2000000" {
		t.Errorf("expected billing fund 2000000, got %s", billingBal)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: State CRUD
// -----------------------------------------------------------------------

func TestSetGetParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	params := k.GetParams(ctx)

	if params.MinReviewerCount != 3 {
		t.Errorf("expected min_reviewer_count 3, got %d", params.MinReviewerCount)
	}
	if params.AcceptanceScoreThreshold != 70 {
		t.Errorf("expected acceptance_score_threshold 70, got %d", params.AcceptanceScoreThreshold)
	}
	if params.RejectionSlashBps != 330000 {
		t.Errorf("expected rejection_slash_bps 330000, got %d", params.RejectionSlashBps)
	}
	if params.ReviewPeriodBlocks != 68544 {
		t.Errorf("expected review_period_blocks 68544, got %d", params.ReviewPeriodBlocks)
	}
	if params.BountyMinDeadlineBlocks != 34272 {
		t.Errorf("expected bounty_min_deadline_blocks 34272, got %d", params.BountyMinDeadlineBlocks)
	}

	// Roundtrip: set modified params and re-read
	params.MinReviewerCount = 5
	params.AcceptanceScoreThreshold = 80
	k.SetParams(ctx, params)

	params2 := k.GetParams(ctx)
	if params2.MinReviewerCount != 5 {
		t.Errorf("expected modified min_reviewer_count 5, got %d", params2.MinReviewerCount)
	}
	if params2.AcceptanceScoreThreshold != 80 {
		t.Errorf("expected modified acceptance_score_threshold 80, got %d", params2.AcceptanceScoreThreshold)
	}
}

func TestIterateResearches(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Add 5 research submissions directly
	for i := 1; i <= 5; i++ {
		k.SetResearch(ctx, &types.Research{
			Id:     fmt.Sprintf("RES-%d", i),
			Domain: "test",
			Status: "submitted",
		})
	}

	var count int
	k.IterateResearches(ctx, func(r *types.Research) bool {
		count++
		return false
	})
	if count != 5 {
		t.Errorf("expected 5 researches iterated, got %d", count)
	}
}

func TestGetResearchesByStatus(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetResearch(ctx, &types.Research{Id: "RES-1", Status: "submitted"})
	k.SetResearch(ctx, &types.Research{Id: "RES-2", Status: "accepted"})
	k.SetResearch(ctx, &types.Research{Id: "RES-3", Status: "submitted"})

	submitted := k.GetResearchesByStatus(ctx, types.ResearchStatusSubmitted)
	if len(submitted) != 2 {
		t.Errorf("expected 2 submitted, got %d", len(submitted))
	}

	accepted := k.GetResearchesByStatus(ctx, types.ResearchStatusAccepted)
	if len(accepted) != 1 {
		t.Errorf("expected 1 accepted, got %d", len(accepted))
	}
}

func TestGetResearchesByDomain(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetResearch(ctx, &types.Research{Id: "RES-1", Domain: "physics"})
	k.SetResearch(ctx, &types.Research{Id: "RES-2", Domain: "math"})
	k.SetResearch(ctx, &types.Research{Id: "RES-3", Domain: "physics"})

	physics := k.GetResearchesByDomain(ctx, "physics")
	if len(physics) != 2 {
		t.Errorf("expected 2 physics, got %d", len(physics))
	}

	math := k.GetResearchesByDomain(ctx, "math")
	if len(math) != 1 {
		t.Errorf("expected 1 math, got %d", len(math))
	}
}

func TestGetActiveBounties(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetBounty(ctx, &types.Bounty{Id: "B-1", Status: "open"})
	k.SetBounty(ctx, &types.Bounty{Id: "B-2", Status: "claimed"})
	k.SetBounty(ctx, &types.Bounty{Id: "B-3", Status: "expired"})
	k.SetBounty(ctx, &types.Bounty{Id: "B-4", Status: "fulfilled"})

	active := k.GetActiveBounties(ctx)
	if len(active) != 2 {
		t.Errorf("expected 2 active bounties (open+claimed), got %d", len(active))
	}
}

func TestDeleteResearch(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetResearch(ctx, &types.Research{Id: "RES-1", Domain: "test"})
	_, found := k.GetResearch(ctx, "RES-1")
	if !found {
		t.Fatal("expected research to exist")
	}

	k.DeleteResearch(ctx, "RES-1")
	_, found = k.GetResearch(ctx, "RES-1")
	if found {
		t.Fatal("expected research to be deleted")
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Query Server
// -----------------------------------------------------------------------

func TestQueryResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Query Test",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Add a review
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(10),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 75,
	})

	qs := keeper.NewQueryServerImpl(k)
	qResp, err := qs.Research(ctx, &types.QueryResearchRequest{ResearchId: resp.ResearchId})
	if err != nil {
		t.Fatalf("Query Research failed: %v", err)
	}
	if qResp.Research.Id != resp.ResearchId {
		t.Errorf("expected id %s, got %s", resp.ResearchId, qResp.Research.Id)
	}
	if len(qResp.Reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(qResp.Reviews))
	}
}

func TestQueryResearchNotFound(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	qs := keeper.NewQueryServerImpl(k)
	_, err := qs.Research(ctx, &types.QueryResearchRequest{ResearchId: "RES-999"})
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
}

func TestQuerySubmissions(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	for _, domain := range []string{"physics", "physics", "math"} {
		msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
			Submitter:   submitter.String(),
			Title:       "Research " + domain,
			Description: "Desc",
			Domain:      domain,
			Stake:       "1000000",
		})
	}

	qs := keeper.NewQueryServerImpl(k)

	// Query all
	allResp, _ := qs.Submissions(ctx, &types.QuerySubmissionsRequest{})
	if len(allResp.Submissions) != 3 {
		t.Errorf("expected 3 submissions, got %d", len(allResp.Submissions))
	}

	// Filter by domain
	physResp, _ := qs.Submissions(ctx, &types.QuerySubmissionsRequest{Domain: "physics"})
	if len(physResp.Submissions) != 2 {
		t.Errorf("expected 2 physics submissions, got %d", len(physResp.Submissions))
	}

	// Filter by status
	statusResp, _ := qs.Submissions(ctx, &types.QuerySubmissionsRequest{Status: "submitted"})
	if len(statusResp.Submissions) != 3 {
		t.Errorf("expected 3 submitted, got %d", len(statusResp.Submissions))
	}
}

func TestQueryBounty(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Query Bounty",
		Description:    "Desc",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	qs := keeper.NewQueryServerImpl(k)
	qResp, err := qs.Bounty(ctx, &types.QueryBountyRequest{BountyId: createResp.BountyId})
	if err != nil {
		t.Fatalf("Query Bounty failed: %v", err)
	}
	if qResp.Bounty.Title != "Query Bounty" {
		t.Errorf("expected title 'Query Bounty', got %q", qResp.Bounty.Title)
	}
}

func TestQueryParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	qs := keeper.NewQueryServerImpl(k)
	qResp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("Query Params failed: %v", err)
	}
	if qResp.Params.MinReviewerCount != 3 {
		t.Errorf("expected min_reviewer_count 3, got %d", qResp.Params.MinReviewerCount)
	}
	if qResp.TreasuryBalance == nil || qResp.TreasuryBalance.Balance != "0" {
		bal := ""
		if qResp.TreasuryBalance != nil {
			bal = qResp.TreasuryBalance.Balance
		}
		t.Errorf("expected treasury_balance 0, got %s", bal)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Full Lifecycle Integration
// -----------------------------------------------------------------------

func TestFullResearchLifecycle(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)

	// 1. Submit
	resp, err := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Full Lifecycle",
		Description: "Complete test",
		Domain:      "physics",
		Stake:       "1000000",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	researchId := resp.ResearchId

	// 2. Review (3 reviewers, all approve with score >= 70)
	for i := 100; i < 103; i++ {
		_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			Reviewer:     testAddrStr(i),
			ResearchId:   researchId,
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			Reasoning:    "Excellent",
			QualityScore: 85,
		})
		if err != nil {
			t.Fatalf("review %d failed: %v", i, err)
		}
	}

	// 3. Verify under review status
	research, _ := k.GetResearch(ctx, researchId)
	if research.Status != "under_review" {
		t.Fatalf("expected status 'under_review', got %q", research.Status)
	}

	// 4. Resolve (should accept with score 85 >= 70)
	resolveResp, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAuthority,
		ResearchId: researchId,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolveResp.Outcome != types.ResearchOutcome_RESEARCH_OUTCOME_ACCEPTED {
		t.Errorf("expected ACCEPTED outcome, got %v", resolveResp.Outcome)
	}

	// 5. Verify final state
	research, _ = k.GetResearch(ctx, researchId)
	if research.Status != "accepted" {
		t.Errorf("expected status 'accepted', got %q", research.Status)
	}
}

func TestFullBountyLifecycle(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	claimer := testAddr(2)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)

	// 1. Create
	createResp, err := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Full Bounty",
		Description:    "Complete test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})
	if err != nil {
		t.Fatalf("create bounty failed: %v", err)
	}

	// 2. Claim
	_, err = msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		Claimer:  claimer.String(),
		BountyId: createResp.BountyId,
	})
	if err != nil {
		t.Fatalf("claim bounty failed: %v", err)
	}

	// 3. Fulfill
	_, err = msgServer.FulfillBounty(ctx, &types.MsgFulfillBounty{
		Authority: testAuthority,
		BountyId:  createResp.BountyId,
		Claimer:   claimer.String(),
	})
	if err != nil {
		t.Fatalf("fulfill bounty failed: %v", err)
	}

	// 4. Verify final state
	bounty, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty.Status != "fulfilled" {
		t.Errorf("expected status 'fulfilled', got %q", bounty.Status)
	}

	// Verify reward paid to claimer
	claimerBal := bk.balances[claimer.String()+"/uzrn"]
	if !claimerBal.Equal(sdkmath.NewInt(5000000)) {
		t.Errorf("expected claimer balance 5000000, got %s", claimerBal.String())
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Genesis with PeerReviews
// -----------------------------------------------------------------------

func TestGenesisWithPeerReviews(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Genesis Review Test",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})
	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     testAddrStr(10),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 75,
	})

	// Export
	gs := k.ExportGenesis(ctx)
	if len(gs.PeerReviews) != 1 {
		t.Fatalf("expected 1 peer review in export, got %d", len(gs.PeerReviews))
	}

	// Re-init into fresh keeper
	k2, ctx2, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, gs)

	// Verify review survived round-trip
	reviews := k2.GetReviewsForResearch(ctx2, resp.ResearchId)
	if len(reviews) != 1 {
		t.Errorf("expected 1 review after re-init, got %d", len(reviews))
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Bounty Expiry Edge Cases
// -----------------------------------------------------------------------

func TestExpireBountiesNotExpiredYet(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Not Yet",
		Description:    "Desc",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Don't advance past deadline
	k.ExpireBounties(ctx)

	bounty, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty.Status != "open" {
		t.Errorf("expected status 'open' (not expired yet), got %q", bounty.Status)
	}
}

// -----------------------------------------------------------------------
// Ported Tests: Adversarial (OpenClaw)
// -----------------------------------------------------------------------

func TestDoubleReviewAttack(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	reviewer := testAddr(10)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Test Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// First review
	_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     reviewer.String(),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		Reasoning:    "Good",
		QualityScore: 80,
	})
	if err != nil {
		t.Fatalf("first review failed: %v", err)
	}

	// ATTACK: Same reviewer submits again
	_, err = msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		Reviewer:     reviewer.String(),
		ResearchId:   resp.ResearchId,
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_REJECT,
		Reasoning:    "Changed mind",
		QualityScore: 10,
	})
	if err == nil {
		t.Fatal("double review attack succeeded -- should have been rejected")
	}

	// Verify original review preserved
	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.ReviewCount != 1 {
		t.Errorf("expected review_count 1, got %d", research.ReviewCount)
	}
	if research.AggregateScore != 80 {
		t.Errorf("expected aggregate_score 80, got %d", research.AggregateScore)
	}
}

func TestUnauthorizedResearchResolution(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Add enough reviews to make resolvable
	for i := 100; i < 103; i++ {
		msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			Reviewer:     testAddrStr(i),
			ResearchId:   resp.ResearchId,
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			Reasoning:    "Good",
			QualityScore: 80,
		})
	}

	// ATTACK: Random user tries to resolve
	_, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAddrStr(99),
		ResearchId: resp.ResearchId,
	})
	if err == nil {
		t.Fatal("unauthorized resolution succeeded -- should have been rejected")
	}

	// Verify still under review
	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status == "accepted" || research.Status == "rejected" {
		t.Errorf("research was resolved by unauthorized party: %s", research.Status)
	}
}

func TestChallengeAcceptedResearch(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	submitter := testAddr(1)
	challenger := testAddr(2)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(50000000))
	bk.setBalance(challenger, "uzrn", sdkmath.NewInt(50000000))

	msgServer := keeper.NewMsgServerImpl(k)
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Research",
		Description: "Desc",
		Domain:      "physics",
		Stake:       "1000000",
	})

	// Manually accept
	research, _ := k.GetResearch(ctx, resp.ResearchId)
	research.Status = "accepted"
	k.SetResearch(ctx, research)

	// ATTACK: Challenge accepted research
	_, err := msgServer.ChallengeResearch(ctx, &types.MsgChallengeResearch{
		Challenger: challenger.String(),
		ResearchId: resp.ResearchId,
		Reason:     "Retroactive challenge",
		Stake:      "1000000",
	})
	if err == nil {
		t.Fatal("challenge on accepted research succeeded -- should have been rejected")
	}

	// Verify status unchanged
	research, _ = k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "accepted" {
		t.Errorf("expected status 'accepted' preserved, got %q", research.Status)
	}
}

func TestBountyClaimAfterExpiry(t *testing.T) {
	k, ctx, bk := setupKeeper(t)

	creator := testAddr(1)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(100000000))

	msgServer := keeper.NewMsgServerImpl(k)
	createResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Find error",
		Description:    "Desc",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 50000,
	})

	// Advance past deadline
	expiredCtx := ctx.WithBlockHeight(int64(uint64(ctx.BlockHeight()) + 50001))

	// ATTACK: Claim expired bounty
	_, err := msgServer.ClaimBounty(expiredCtx, &types.MsgClaimBounty{
		Claimer:  testAddrStr(2),
		BountyId: createResp.BountyId,
	})
	if err == nil {
		t.Fatal("claiming expired bounty succeeded -- should have been rejected")
	}

	// Verify bounty status unchanged
	bounty, _ := k.GetBounty(ctx, createResp.BountyId)
	if bounty.ClaimedBy != "" {
		t.Errorf("expected no claimer, got %q", bounty.ClaimedBy)
	}
}

// -----------------------------------------------------------------------
// Tests: Auto-Resolution
// -----------------------------------------------------------------------

func TestAutoResolveResearchAccepted(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.ReviewPeriodBlocks = 10
	params.MinReviewerCount = 2
	params.AcceptanceScoreThreshold = 70
	k.SetParams(ctx, &params)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))

	resp, err := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Auto-Resolve Test",
		Description: "Testing auto-resolution",
		Domain:      "physics",
		Stake:       "1000000",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	for i := 2; i <= 3; i++ {
		_, err := msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			ResearchId:   resp.ResearchId,
			Reviewer:     testAddrStr(i),
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			QualityScore: 80,
			Reasoning:    "Good work",
		})
		if err != nil {
			t.Fatalf("review %d: %v", i, err)
		}
	}

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "under_review" {
		t.Fatalf("expected under_review, got %s", research.Status)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 11)

	err = k.AutoResolveResearch(ctx)
	if err != nil {
		t.Fatalf("auto-resolve: %v", err)
	}

	research, _ = k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "accepted" {
		t.Fatalf("expected accepted, got %s", research.Status)
	}

	bal := bk.balances[submitter.String()+"/uzrn"]
	if !bal.Equal(sdkmath.NewInt(5000000)) {
		t.Fatalf("expected stake returned, balance: %s", bal)
	}
}

func TestAutoResolveResearchRejected(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.ReviewPeriodBlocks = 10
	params.MinReviewerCount = 2
	params.AcceptanceScoreThreshold = 70
	params.RejectionSlashBps = 330000
	k.SetParams(ctx, &params)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Low Score Research",
		Description: "Will be rejected",
		Domain:      "physics",
		Stake:       "1000000",
	})

	for i := 2; i <= 3; i++ {
		msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			ResearchId:   resp.ResearchId,
			Reviewer:     testAddrStr(i),
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_REJECT,
			QualityScore: 30,
			Reasoning:    "Poor quality",
		})
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 11)
	k.AutoResolveResearch(ctx)

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "rejected" {
		t.Fatalf("expected rejected, got %s", research.Status)
	}

	// Submitter: started 5M, staked 1M (balance 4M), gets back 1M - 33% = 670000 → 4670000
	bal := bk.balances[submitter.String()+"/uzrn"]
	if !bal.Equal(sdkmath.NewInt(4670000)) {
		t.Fatalf("expected 4670000 after slash, got %s", bal)
	}
}

func TestAutoResolveResearchInsufficientReviews(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.ReviewPeriodBlocks = 10
	params.MinReviewerCount = 3
	k.SetParams(ctx, &params)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Not Enough Reviews",
		Description: "Only 1 review",
		Domain:      "physics",
		Stake:       "1000000",
	})

	msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
		ResearchId:   resp.ResearchId,
		Reviewer:     testAddrStr(2),
		Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
		QualityScore: 90,
		Reasoning:    "Great",
	})

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 11)
	k.AutoResolveResearch(ctx)

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "under_review" {
		t.Fatalf("expected under_review (insufficient reviews), got %s", research.Status)
	}
}

func TestAutoResolveResearchWithinPeriod(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.ReviewPeriodBlocks = 100
	params.MinReviewerCount = 2
	k.SetParams(ctx, &params)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Too Early",
		Description: "Not enough time",
		Domain:      "physics",
		Stake:       "1000000",
	})

	for i := 2; i <= 3; i++ {
		msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			ResearchId:   resp.ResearchId,
			Reviewer:     testAddrStr(i),
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			QualityScore: 90,
			Reasoning:    "Excellent",
		})
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 5)
	k.AutoResolveResearch(ctx)

	research, _ := k.GetResearch(ctx, resp.ResearchId)
	if research.Status != "under_review" {
		t.Fatalf("expected under_review (within period), got %s", research.Status)
	}
}

// -----------------------------------------------------------------------
// Tests: Auto-Fulfillment
// -----------------------------------------------------------------------

func TestAutoFulfillBountyAccepted(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.BountyFulfillmentPeriodBlocks = 10
	params.BountyMinDeadlineBlocks = 5
	k.SetParams(ctx, &params)

	creator := testAddr(1)
	claimer := testAddr(2)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(50000000))

	bResp, err := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Test Bounty",
		Description:    "Auto-fulfill test",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 1000,
	})
	if err != nil {
		t.Fatalf("create bounty: %v", err)
	}

	_, err = msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		BountyId: bResp.BountyId,
		Claimer:  claimer.String(),
	})
	if err != nil {
		t.Fatalf("claim bounty: %v", err)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 11)

	err = k.AutoFulfillBounties(ctx)
	if err != nil {
		t.Fatalf("auto-fulfill: %v", err)
	}

	bounty, _ := k.GetBounty(ctx, bResp.BountyId)
	if bounty.Status != "fulfilled" {
		t.Fatalf("expected fulfilled, got %s", bounty.Status)
	}
	if bounty.FulfilledBy != claimer.String() {
		t.Fatalf("expected fulfilled_by = %s, got %s", claimer.String(), bounty.FulfilledBy)
	}

	bal := bk.balances[claimer.String()+"/uzrn"]
	if !bal.Equal(sdkmath.NewInt(5000000)) {
		t.Fatalf("expected claimer to have 5000000, got %s", bal)
	}
}

func TestAutoFulfillBountyWithinPeriod(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.BountyFulfillmentPeriodBlocks = 100
	params.BountyMinDeadlineBlocks = 5
	k.SetParams(ctx, &params)

	creator := testAddr(1)
	claimer := testAddr(2)
	bk.setBalance(creator, "uzrn", sdkmath.NewInt(50000000))

	bResp, _ := msgServer.CreateBounty(ctx, &types.MsgCreateBounty{
		Creator:        creator.String(),
		Title:          "Too Early Bounty",
		Description:    "Within period",
		Reward:         "5000000",
		DeadlineHeight: uint64(ctx.BlockHeight()) + 1000,
	})

	msgServer.ClaimBounty(ctx, &types.MsgClaimBounty{
		BountyId: bResp.BountyId,
		Claimer:  claimer.String(),
	})

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 5)
	k.AutoFulfillBounties(ctx)

	bounty, _ := k.GetBounty(ctx, bResp.BountyId)
	if bounty.Status != "claimed" {
		t.Fatalf("expected claimed (within period), got %s", bounty.Status)
	}
}

func TestGovernanceOverrideStillWorks(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	msgServer := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.ReviewPeriodBlocks = 1000
	params.MinReviewerCount = 2
	k.SetParams(ctx, &params)

	submitter := testAddr(1)
	bk.setBalance(submitter, "uzrn", sdkmath.NewInt(5000000))
	resp, _ := msgServer.SubmitResearch(ctx, &types.MsgSubmitResearch{
		Submitter:   submitter.String(),
		Title:       "Governance Override",
		Description: "Force resolve via authority",
		Domain:      "physics",
		Stake:       "1000000",
	})

	for i := 2; i <= 3; i++ {
		msgServer.ReviewResearch(ctx, &types.MsgReviewResearch{
			ResearchId:   resp.ResearchId,
			Reviewer:     testAddrStr(i),
			Verdict:      types.ReviewVerdict_REVIEW_VERDICT_APPROVE,
			QualityScore: 80,
			Reasoning:    "Good",
		})
	}

	// Fund the module account so SendCoinsFromModuleToAccount works
	bk.balances["research/uzrn"] = sdkmath.NewInt(1000000)

	resolveResp, err := msgServer.ResolveResearch(ctx, &types.MsgResolveResearch{
		Authority:  testAuthority,
		ResearchId: resp.ResearchId,
	})
	if err != nil {
		t.Fatalf("governance resolve: %v", err)
	}
	if resolveResp.Outcome != types.ResearchOutcome_RESEARCH_OUTCOME_ACCEPTED {
		t.Fatal("expected accepted via governance override")
	}
}
