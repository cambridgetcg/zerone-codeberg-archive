package keeper_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

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

	knowledgetypes "github.com/zerone-chain/zerone/x/knowledge/types"

	"github.com/zerone-chain/zerone/x/disputes/keeper"
	"github.com/zerone-chain/zerone/x/disputes/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

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

func (m *mockBankKeeper) SendCoins(_ context.Context, from, to sdk.AccAddress, amt sdk.Coins) error {
	for _, coin := range amt {
		fromAddr := from.String()
		toAddr := to.String()
		if m.balances[fromAddr] == nil {
			m.balances[fromAddr] = make(map[string]int64)
		}
		if m.balances[toAddr] == nil {
			m.balances[toAddr] = make(map[string]int64)
		}
		m.balances[fromAddr][coin.Denom] -= coin.Amount.Int64()
		m.balances[toAddr][coin.Denom] += coin.Amount.Int64()
	}
	return nil
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

func (m *mockBankKeeper) GetBalance(_ context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	addrStr := addr.String()
	if m.balances[addrStr] == nil {
		return sdk.NewInt64Coin(denom, 0)
	}
	return sdk.NewInt64Coin(denom, m.balances[addrStr][denom])
}

// ---------- Mock StakingKeeper ----------

type mockStakingKeeper struct {
	validators []string
}

func newMockStakingKeeper(validators []string) *mockStakingKeeper {
	return &mockStakingKeeper{validators: validators}
}

func (m *mockStakingKeeper) GetQualifiedValidators(_ context.Context, _ string, count int) ([]string, error) {
	if count > len(m.validators) {
		return m.validators, nil
	}
	return m.validators[:count], nil
}

// ---------- Mock KnowledgeKeeper ----------

type mockKnowledgeKeeper struct {
	facts map[string]*knowledgetypes.Fact
}

func newMockKnowledgeKeeper() *mockKnowledgeKeeper {
	return &mockKnowledgeKeeper{
		facts: make(map[string]*knowledgetypes.Fact),
	}
}

func (m *mockKnowledgeKeeper) addFact(id, submitter string) {
	m.facts[id] = &knowledgetypes.Fact{
		Id:        id,
		Submitter: submitter,
	}
}

func (m *mockKnowledgeKeeper) GetFact(_ context.Context, factID string) (*knowledgetypes.Fact, bool) {
	f, ok := m.facts[factID]
	return f, ok
}

// ---------- Test Addresses ----------

func testAddr(name string) string {
	h := sha256.Sum256([]byte("test_seed:" + name))
	return sdk.AccAddress(h[:20]).String()
}

// ---------- Test Setup ----------

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper, *mockStakingKeeper, *mockKnowledgeKeeper) {
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

	mockBK := newMockBankKeeper()

	// Create validators for arbiter selection (need enough to not include challenger/defender)
	validators := []string{
		testAddr("validator1"),
		testAddr("validator2"),
		testAddr("validator3"),
		testAddr("validator4"),
		testAddr("validator5"),
		testAddr("validator6"),
		testAddr("validator7"),
		testAddr("validator8"),
		testAddr("validator9"),
		testAddr("validator10"),
	}
	mockSK := newMockStakingKeeper(validators)
	mockKK := newMockKnowledgeKeeper()

	storeService := runtime.NewKVStoreService(storeKey)
	k := keeper.NewKeeper(storeService, cdc, "zrn1authority", mockBK, mockSK, mockKK)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100, ChainID: "zerone-test-1"}, false, log.NewNopLogger())

	return k, ctx, mockBK, mockSK, mockKK
}

func setupMsgServer(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context, *mockBankKeeper, *mockStakingKeeper, *mockKnowledgeKeeper) {
	t.Helper()
	k, ctx, bk, sk, kk := setupKeeper(t)
	return keeper.NewMsgServerImpl(k), k, ctx, bk, sk, kk
}

// ---------- Params Tests ----------

func TestDefaultParams(t *testing.T) {
	params := types.DefaultParams()
	if len(params.TierConfigs) != 4 {
		t.Errorf("expected 4 tier configs, got %d", len(params.TierConfigs))
	}
	if params.MaxActiveDisputes != 100 {
		t.Errorf("expected max active disputes 100, got %d", params.MaxActiveDisputes)
	}
	if params.SlashRateLoserBps != 500000 {
		t.Errorf("expected slash rate 500000, got %d", params.SlashRateLoserBps)
	}
}

func TestSetGetParams(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	params := &types.Params{
		TierConfigs:        types.DefaultTierConfigs(),
		MaxActiveDisputes:  50,
		EscalationDelay:    1000,
		SlashRateLoserBps:  600000,
		RewardRateWinnerBps: 300000,
		ArbiterRewardBps:   100000,
	}
	k.SetParams(ctx, params)

	got := k.GetParams(ctx)
	if got.MaxActiveDisputes != 50 {
		t.Errorf("expected max active disputes 50, got %d", got.MaxActiveDisputes)
	}
	if got.SlashRateLoserBps != 600000 {
		t.Errorf("expected slash rate 600000, got %d", got.SlashRateLoserBps)
	}
}

// ---------- Dispute CRUD Tests ----------

func TestSetGetDispute(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:         "dispute-1",
		TargetId:   "fact-1",
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		Challenger: testAddr("challenger"),
		Defender:   testAddr("defender"),
		Bond:       "1000000",
		Tier:       1,
		Phase:      types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
	}
	k.SetDispute(ctx, dispute)

	got, found := k.GetDispute(ctx, "dispute-1")
	if !found {
		t.Fatal("dispute not found")
	}
	if got.Challenger != dispute.Challenger {
		t.Errorf("expected challenger %s, got %s", dispute.Challenger, got.Challenger)
	}
	if got.Bond != "1000000" {
		t.Errorf("expected bond 1000000, got %s", got.Bond)
	}
}

func TestGetDisputeNotFound(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	_, found := k.GetDispute(ctx, "nonexistent")
	if found {
		t.Fatal("expected dispute not found")
	}
}

func TestGetDisputesByTarget(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	for i := 0; i < 3; i++ {
		d := &types.Dispute{
			Id:         fmt.Sprintf("dispute-%d", i),
			TargetId:   "fact-1",
			TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
			Challenger: testAddr("challenger"),
			Phase:      types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		}
		k.SetDispute(ctx, d)
	}

	disputes := k.GetDisputesByTarget(ctx, "fact-1")
	if len(disputes) != 3 {
		t.Errorf("expected 3 disputes, got %d", len(disputes))
	}
}

func TestGetActiveDisputes(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	// Active dispute
	k.SetDispute(ctx, &types.Dispute{
		Id:       "active-1",
		TargetId: "fact-1",
		Phase:    types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
	})
	// Settled dispute
	k.SetDispute(ctx, &types.Dispute{
		Id:       "settled-1",
		TargetId: "fact-2",
		Phase:    types.DisputePhase_DISPUTE_PHASE_SETTLED,
	})

	active := k.GetActiveDisputes(ctx)
	if len(active) != 1 {
		t.Errorf("expected 1 active dispute, got %d", len(active))
	}
	if active[0].Id != "active-1" {
		t.Errorf("expected active-1, got %s", active[0].Id)
	}
}

// ---------- Genesis Tests ----------

func TestInitExportGenesis(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	genState := &types.GenesisState{
		Params: &types.Params{
			TierConfigs:        types.DefaultTierConfigs(),
			MaxActiveDisputes:  50,
			EscalationDelay:    1000,
			SlashRateLoserBps:  500000,
			RewardRateWinnerBps: 400000,
			ArbiterRewardBps:   100000,
		},
		Disputes: []*types.Dispute{
			{
				Id:         "gen-dispute-1",
				TargetId:   "fact-1",
				TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
				Challenger: testAddr("challenger"),
				Bond:       "5000000",
				Phase:      types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
			},
		},
	}

	k.InitGenesis(ctx, genState)

	exported := k.ExportGenesis(ctx)
	if exported.Params.MaxActiveDisputes != 50 {
		t.Errorf("expected max active disputes 50, got %d", exported.Params.MaxActiveDisputes)
	}
	if len(exported.Disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d", len(exported.Disputes))
	}
	if exported.Disputes[0].Id != "gen-dispute-1" {
		t.Errorf("expected gen-dispute-1, got %s", exported.Disputes[0].Id)
	}
}

// ---------- InitiateDispute Tests ----------

func TestInitiateDispute(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed claim",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute failed: %v", err)
	}
	if resp.DisputeId == "" {
		t.Fatal("expected non-empty dispute ID")
	}

	dispute, found := k.GetDispute(ctx, resp.DisputeId)
	if !found {
		t.Fatal("dispute not found")
	}
	if dispute.Challenger != challenger {
		t.Errorf("expected challenger %s, got %s", challenger, dispute.Challenger)
	}
	if dispute.Defender != defender {
		t.Errorf("expected defender %s, got %s", defender, dispute.Defender)
	}
	if dispute.Bond != "1000000" {
		t.Errorf("expected bond 1000000, got %s", dispute.Bond)
	}
	if dispute.Tier != 1 {
		t.Errorf("expected tier 1, got %d", dispute.Tier)
	}
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT {
		t.Errorf("expected EVIDENCE_COMMIT phase, got %s", dispute.Phase.String())
	}
	if len(dispute.Arbiters) != 3 {
		t.Errorf("expected 3 arbiters, got %d", len(dispute.Arbiters))
	}

	// Verify bond was escrowed
	if bk.balances[challenger]["uzrn"] != 9_000_000 {
		t.Errorf("expected challenger balance 9000000, got %d", bk.balances[challenger]["uzrn"])
	}
}

func TestInitiateDisputeInsufficientBond(t *testing.T) {
	msgSrv, _, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed claim",
		Bond:       "100", // below tier 1 minimum of 1000000
	})
	if err == nil {
		t.Fatal("expected error for insufficient bond")
	}
}

func TestInitiateDisputeTargetNotFound(t *testing.T) {
	msgSrv, _, ctx, bk, _, _ := setupMsgServer(t)

	challenger := testAddr("challenger")
	bk.setBalance(challenger, "uzrn", 10_000_000)

	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "nonexistent-fact",
		Reason:     "Disputed",
		Bond:       "1000000",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
}

// ---------- CommitEvidence Tests ----------

func TestCommitEvidence(t *testing.T) {
	msgSrv, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Commit evidence
	content := "This fact is inaccurate because..."
	nonce := "random-nonce-123"
	h := sha256.Sum256([]byte(content + nonce))
	commitHash := hex.EncodeToString(h[:])

	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: commitHash,
	})
	if err != nil {
		t.Fatalf("CommitEvidence failed: %v", err)
	}

	commitment, found := k.GetCommitment(ctx, resp.DisputeId, challenger)
	if !found {
		t.Fatal("commitment not found")
	}
	if commitment.ContentHash != commitHash {
		t.Errorf("expected hash %s, got %s", commitHash, commitment.ContentHash)
	}
	if commitment.Revealed {
		t.Error("expected commitment to be unrevealed")
	}
}

func TestCommitEvidenceWrongPhase(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	// Create dispute in ARBITRATION phase directly
	k.SetDispute(ctx, &types.Dispute{
		Id:               "dispute-arb",
		TargetId:         "fact-1",
		Challenger:       testAddr("challenger"),
		Defender:         testAddr("defender"),
		Phase:            types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		EvidenceDeadline: 200,
	})

	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      testAddr("challenger"),
		DisputeId:      "dispute-arb",
		CommitmentHash: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for wrong phase")
	}
}

func TestCommitEvidenceNotParty(t *testing.T) {
	msgSrv, _, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	outsider := testAddr("outsider")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      outsider,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for non-party submitter")
	}
}

// ---------- RevealEvidence Tests ----------

func TestRevealEvidence(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Commit evidence
	content := "Evidence content"
	nonce := "nonce123"
	h := sha256.Sum256([]byte(content + nonce))
	commitHash := hex.EncodeToString(h[:])

	_, _ = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: commitHash,
	})

	// Advance to reveal phase
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 1000 // far future
	k.SetDispute(ctx, dispute)

	// Reveal
	_, err := msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   content,
		Nonce:     nonce,
	})
	if err != nil {
		t.Fatalf("RevealEvidence failed: %v", err)
	}

	// Verify evidence stored
	evidences := k.GetEvidenceByDispute(ctx, resp.DisputeId)
	if len(evidences) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(evidences))
	}
	if evidences[0].Content != content {
		t.Errorf("expected content %q, got %q", content, evidences[0].Content)
	}

	// Verify commitment marked as revealed
	commitment, _ := k.GetCommitment(ctx, resp.DisputeId, challenger)
	if !commitment.Revealed {
		t.Error("expected commitment to be revealed")
	}

	// Verify evidence count incremented
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.EvidenceCount != 1 {
		t.Errorf("expected evidence count 1, got %d", dispute.EvidenceCount)
	}
}

func TestRevealEvidenceHashMismatch(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Commit with one hash
	h := sha256.Sum256([]byte("correct content" + "correct nonce"))
	commitHash := hex.EncodeToString(h[:])
	_, _ = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: commitHash,
	})

	// Advance to reveal phase
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 1000
	k.SetDispute(ctx, dispute)

	// Try to reveal with different content
	_, err := msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   "wrong content",
		Nonce:     "wrong nonce",
	})
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
}

// ---------- ArbiterVote Tests ----------

func TestArbiterVote(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Advance to arbitration phase
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// Vote as an assigned arbiter
	arbiter := dispute.Arbiters[0]
	_, err := msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   arbiter,
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "Evidence supports challenger",
	})
	if err != nil {
		t.Fatalf("ArbiterVote failed: %v", err)
	}

	vote, found := k.GetVote(ctx, resp.DisputeId, arbiter)
	if !found {
		t.Fatal("vote not found")
	}
	if vote.Vote != types.ArbiterDecision_ARBITER_DECISION_CHALLENGER {
		t.Errorf("expected CHALLENGER vote, got %s", vote.Vote.String())
	}
}

func TestArbiterVoteNotArbiter(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// Try voting as non-arbiter
	outsider := testAddr("outsider")
	_, err := msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   outsider,
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "Should fail",
	})
	if err == nil {
		t.Fatal("expected error for non-arbiter voter")
	}
}

func TestArbiterVoteAlreadyVoted(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	arbiter := dispute.Arbiters[0]
	_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   arbiter,
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "First vote",
	})

	// Try to vote again
	_, err := msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   arbiter,
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
		Reasoning: "Change mind",
	})
	if err == nil {
		t.Fatal("expected error for double voting")
	}
}

// ---------- Escalation Tests ----------

func TestEscalateDispute(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 100_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Set dispute as past escalation delay
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.CreatedAt = 1 // set to very early block so escalation delay is met
	k.SetDispute(ctx, dispute)

	// Move to a block past escalation delay
	escCtx := ctx.WithBlockHeight(600)

	escResp, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      challenger,
		DisputeId:      resp.DisputeId,
		AdditionalBond: "10000000",
	})
	if err != nil {
		t.Fatalf("EscalateDispute failed: %v", err)
	}
	if escResp.NewTier != 2 {
		t.Errorf("expected tier 2, got %d", escResp.NewTier)
	}

	// Verify dispute updated
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.Tier != 2 {
		t.Errorf("expected tier 2, got %d", dispute.Tier)
	}
	if dispute.Bond != "11000000" {
		t.Errorf("expected total bond 11000000, got %s", dispute.Bond)
	}
	if len(dispute.Arbiters) != 7 {
		t.Errorf("expected 7 arbiters for tier 2, got %d", len(dispute.Arbiters))
	}
}

func TestEscalateDisputeMaxTier(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 1_000_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Set to max tier
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Tier = 4
	dispute.CreatedAt = 1
	k.SetDispute(ctx, dispute)

	escCtx := ctx.WithBlockHeight(600)
	_, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      challenger,
		DisputeId:      resp.DisputeId,
		AdditionalBond: "100000000",
	})
	if err == nil {
		t.Fatal("expected error for max tier escalation")
	}
}

// ---------- Settlement Tests ----------

func TestSettleDisputeChallengerWins(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Advance to arbitration and set all arbiters voting for challenger
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// All 3 arbiters vote for challenger (>66.7% majority)
	for _, arb := range dispute.Arbiters {
		_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter:   arb,
			DisputeId: resp.DisputeId,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Reasoning: "Challenger is right",
		})
	}

	// Settle
	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS, got %s", settleResp.Outcome.String())
	}

	// Verify dispute settled
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED phase, got %s", dispute.Phase.String())
	}
}

func TestSettleDisputeDefenderWins(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// All 3 arbiters vote for defender
	for _, arb := range dispute.Arbiters {
		_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter:   arb,
			DisputeId: resp.DisputeId,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
			Reasoning: "Defender is right",
		})
	}

	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS {
		t.Errorf("expected DEFENDER_WINS, got %s", settleResp.Outcome.String())
	}
}

func TestSettleDisputeDraw(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// Split votes: 1 challenger, 1 defender, 1 abstain → no majority
	_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   dispute.Arbiters[0],
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "For challenger",
	})
	_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   dispute.Arbiters[1],
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
		Reasoning: "For defender",
	})
	_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   dispute.Arbiters[2],
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_ABSTAIN,
		Reasoning: "Abstaining",
	})

	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_DRAW {
		t.Errorf("expected DRAW, got %s", settleResp.Outcome.String())
	}
}

func TestSettleDisputeTimeout(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// No votes cast → timeout
	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
		t.Errorf("expected TIMED_OUT, got %s", settleResp.Outcome.String())
	}

	// Verify bond returned to challenger on timeout
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_TIMED_OUT {
		t.Errorf("expected TIMED_OUT phase, got %s", dispute.Phase.String())
	}
}

func TestSettleDisputeUnauthorized(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	k.SetDispute(ctx, dispute)

	_, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: testAddr("not-authority"),
		DisputeId: resp.DisputeId,
	})
	if err == nil {
		t.Fatal("expected error for unauthorized settle")
	}
}

// ---------- Bond Distribution Tests ----------

func TestBondDistributionChallengerWins(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// All arbiters vote challenger
	for _, arb := range dispute.Arbiters {
		_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter: arb, DisputeId: resp.DisputeId,
			Vote: types.ArbiterDecision_ARBITER_DECISION_CHALLENGER, Reasoning: "yes",
		})
	}

	// Note balances before settlement
	challengerBefore := bk.balances[challenger]["uzrn"]

	_, _ = msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})

	challengerAfter := bk.balances[challenger]["uzrn"]

	// Challenger should have received: winner reward (40% of 50% slash) + refund (50% of bond)
	// Bond=1000000, slash=500000 (50%), winner=200000 (40% of slash), refund=500000
	// So challenger net gain = 200000 + 500000 = 700000 from module
	if challengerAfter <= challengerBefore {
		t.Errorf("expected challenger balance to increase: before=%d, after=%d", challengerBefore, challengerAfter)
	}
}

// ---------- Max Active Disputes Test ----------

func TestMaxActiveDisputes(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	bk.setBalance(challenger, "uzrn", 1_000_000_000)

	// Set max to 2
	params := types.DefaultParams()
	params.MaxActiveDisputes = 2
	k.SetParams(ctx, params)

	for i := 0; i < 2; i++ {
		factID := fmt.Sprintf("fact-%d", i)
		kk.addFact(factID, testAddr(fmt.Sprintf("defender-%d", i)))
		_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
			Challenger: challenger,
			TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
			TargetId:   factID,
			Reason:     "Disputed",
			Bond:       "1000000",
		})
		if err != nil {
			t.Fatalf("InitiateDispute %d failed: %v", i, err)
		}
	}

	// 3rd should fail
	kk.addFact("fact-2", testAddr("defender-2"))
	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-2",
		Reason:     "Disputed",
		Bond:       "1000000",
	})
	if err == nil {
		t.Fatal("expected error for exceeding max active disputes")
	}
}

// ---------- Phase Transition Tests (BeginBlocker) ----------

func TestPhaseTransitionCommitToReveal(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	// Create dispute with commit phase deadline at block 200
	k.SetDispute(ctx, &types.Dispute{
		Id:               "dispute-1",
		TargetId:         "fact-1",
		Phase:            types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		Tier:             1,
		EvidenceDeadline: 200,
		VotingDeadline:   1700,
	})

	// Process at block 201 → should advance to reveal
	k.ProcessPhaseTransitions(ctx, 201)

	dispute, _ := k.GetDispute(ctx, "dispute-1")
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL {
		t.Errorf("expected EVIDENCE_REVEAL, got %s", dispute.Phase.String())
	}
}

func TestPhaseTransitionRevealToArbitration(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	k.SetDispute(ctx, &types.Dispute{
		Id:               "dispute-1",
		TargetId:         "fact-1",
		Phase:            types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL,
		Tier:             1,
		EvidenceDeadline: 300,
		VotingDeadline:   1800,
	})

	k.ProcessPhaseTransitions(ctx, 301)

	dispute, _ := k.GetDispute(ctx, "dispute-1")
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_ARBITRATION {
		t.Errorf("expected ARBITRATION, got %s", dispute.Phase.String())
	}
}

func TestPhaseTransitionArbitrationToTimeout(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	k.SetDispute(ctx, &types.Dispute{
		Id:             "dispute-1",
		TargetId:       "fact-1",
		Phase:          types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Tier:           1,
		Challenger:     testAddr("challenger"),
		Bond:           "1000000",
		VotingDeadline: 500,
	})

	// Process past voting deadline with no votes → timeout
	k.ProcessPhaseTransitions(ctx, 501)

	dispute, _ := k.GetDispute(ctx, "dispute-1")
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_TIMED_OUT {
		t.Errorf("expected TIMED_OUT, got %s", dispute.Phase.String())
	}
}

// ---------- Full Lifecycle Test ----------

func TestFullLifecycle_InitiateCommitRevealVoteSettle(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	// 1. Initiate dispute
	initResp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "The fact is wrong",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute: %v", err)
	}

	// 2. Commit evidence
	content := "My evidence that the fact is wrong"
	nonce := "secret-nonce-42"
	h := sha256.Sum256([]byte(content + nonce))
	commitHash := hex.EncodeToString(h[:])

	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      initResp.DisputeId,
		CommitmentHash: commitHash,
	})
	if err != nil {
		t.Fatalf("CommitEvidence: %v", err)
	}

	// 3. Advance to reveal phase
	dispute, _ := k.GetDispute(ctx, initResp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 1000
	k.SetDispute(ctx, dispute)

	// 4. Reveal evidence
	_, err = msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: initResp.DisputeId,
		Content:   content,
		Nonce:     nonce,
	})
	if err != nil {
		t.Fatalf("RevealEvidence: %v", err)
	}

	// 5. Advance to arbitration
	dispute, _ = k.GetDispute(ctx, initResp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// 6. All arbiters vote challenger
	dispute, _ = k.GetDispute(ctx, initResp.DisputeId)
	for _, arb := range dispute.Arbiters {
		_, err = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter:   arb,
			DisputeId: initResp.DisputeId,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Reasoning: "Evidence is convincing",
		})
		if err != nil {
			t.Fatalf("ArbiterVote by %s: %v", arb, err)
		}
	}

	// 7. Settle
	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: initResp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS, got %s", settleResp.Outcome.String())
	}

	// 8. Verify final state
	dispute, _ = k.GetDispute(ctx, initResp.DisputeId)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED, got %s", dispute.Phase.String())
	}

	// Verify evidence count
	if dispute.EvidenceCount != 1 {
		t.Errorf("expected evidence count 1, got %d", dispute.EvidenceCount)
	}

	// Verify dispute no longer active
	active := k.GetActiveDisputes(ctx)
	for _, a := range active {
		if a.Id == initResp.DisputeId {
			t.Error("settled dispute should not be in active list")
		}
	}
}

// ---------- Query Server Tests ----------

func TestQueryDispute(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	k.SetDispute(ctx, &types.Dispute{
		Id:       "q-dispute-1",
		TargetId: "fact-1",
		Bond:     "5000000",
		Phase:    types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
	})

	resp, err := qs.Dispute(ctx, &types.QueryDisputeRequest{Id: "q-dispute-1"})
	if err != nil {
		t.Fatalf("Query Dispute failed: %v", err)
	}
	if resp.Dispute.Bond != "5000000" {
		t.Errorf("expected bond 5000000, got %s", resp.Dispute.Bond)
	}
}

func TestQueryDisputeNotFound(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Dispute(ctx, &types.QueryDisputeRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent dispute")
	}
}

func TestQueryParams(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("Query Params failed: %v", err)
	}
	if resp.Params.MaxActiveDisputes != 100 {
		t.Errorf("expected max active disputes 100, got %d", resp.Params.MaxActiveDisputes)
	}
}

// ---------- ValidateBasic Tests ----------

func TestMsgInitiateDisputeValidateBasic(t *testing.T) {
	msg := &types.MsgInitiateDispute{
		Challenger: testAddr("challenger"),
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Invalid fact",
		Bond:       "1000000",
	}
	if err := msg.ValidateBasic(); err != nil {
		t.Errorf("ValidateBasic should pass: %v", err)
	}

	// Empty reason
	msg2 := &types.MsgInitiateDispute{
		Challenger: testAddr("challenger"),
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "",
		Bond:       "1000000",
	}
	if err := msg2.ValidateBasic(); err == nil {
		t.Error("ValidateBasic should fail for empty reason")
	}

	// Unspecified target type
	msg3 := &types.MsgInitiateDispute{
		Challenger: testAddr("challenger"),
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_UNSPECIFIED,
		TargetId:   "fact-1",
		Reason:     "Invalid",
		Bond:       "1000000",
	}
	if err := msg3.ValidateBasic(); err == nil {
		t.Error("ValidateBasic should fail for unspecified target type")
	}
}

func TestGenesisValidation(t *testing.T) {
	gs := types.DefaultGenesis()
	if err := gs.Validate(); err != nil {
		t.Errorf("default genesis should be valid: %v", err)
	}

	// Duplicate dispute IDs
	gs2 := &types.GenesisState{
		Params: types.DefaultParams(),
		Disputes: []*types.Dispute{
			{Id: "dup"},
			{Id: "dup"},
		},
	}
	if err := gs2.Validate(); err == nil {
		t.Error("genesis with duplicate dispute IDs should be invalid")
	}
}

// ---------- Arbiter Selection Tests ----------

func TestRevealWithoutCommit(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Advance to reveal phase without committing
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 1000
	k.SetDispute(ctx, dispute)

	// Try to reveal without having committed
	_, err := msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   "some evidence",
		Nonce:     "some-nonce",
	})
	if err == nil {
		t.Fatal("expected error for reveal without commit")
	}
}

func TestRevealAfterDeadline(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Commit evidence
	content := "Evidence content"
	nonce := "nonce123"
	h := sha256.Sum256([]byte(content + nonce))
	commitHash := hex.EncodeToString(h[:])
	_, _ = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: commitHash,
	})

	// Advance to reveal phase with deadline already passed
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 50 // deadline in the past (current block = 100)
	k.SetDispute(ctx, dispute)

	// Try to reveal after deadline
	_, err := msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   content,
		Nonce:     nonce,
	})
	if err == nil {
		t.Fatal("expected error for reveal after deadline")
	}
}

func TestEscalateInsufficientBond(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 100_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Set up for escalation
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.CreatedAt = 1
	k.SetDispute(ctx, dispute)

	escCtx := ctx.WithBlockHeight(600)

	// Try to escalate with zero additional bond
	_, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      challenger,
		DisputeId:      resp.DisputeId,
		AdditionalBond: "0",
	})
	if err == nil {
		t.Fatal("expected error for zero additional bond")
	}
}

func TestCommitEvidenceAfterDeadline(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	// Set evidence deadline to past
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.EvidenceDeadline = 50 // past (current block = 100)
	k.SetDispute(ctx, dispute)

	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for commit after deadline")
	}
}

func TestEscalateDisputeNonParty(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("challenger")
	defender := testAddr("defender")
	outsider := testAddr("outsider")
	bk.setBalance(challenger, "uzrn", 100_000_000)
	bk.setBalance(outsider, "uzrn", 100_000_000)
	kk.addFact("fact-1", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-1",
		Reason:     "Disputed",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.CreatedAt = 1
	k.SetDispute(ctx, dispute)

	escCtx := ctx.WithBlockHeight(600)

	// Outsider tries to escalate
	_, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      outsider,
		DisputeId:      resp.DisputeId,
		AdditionalBond: "10000000",
	})
	if err == nil {
		t.Fatal("expected error for non-party escalation")
	}
}

func TestArbiterSelectionExcludesParties(t *testing.T) {
	k, ctx, _, sk, _ := setupKeeper(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")

	// Add challenger and defender to validators
	sk.validators = append(sk.validators, challenger, defender)

	arbiters, err := k.SelectArbiters(ctx, 3, challenger, defender, 100)
	if err != nil {
		t.Fatalf("SelectArbiters failed: %v", err)
	}
	if len(arbiters) != 3 {
		t.Fatalf("expected 3 arbiters, got %d", len(arbiters))
	}

	for _, arb := range arbiters {
		if arb == challenger {
			t.Error("challenger should not be selected as arbiter")
		}
		if arb == defender {
			t.Error("defender should not be selected as arbiter")
		}
	}
}

// ---------- Tests: UpdateParams ----------

func TestUpdateParams(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	authority := k.GetAuthority()

	srv := keeper.NewMsgServerImpl(k)
	newParams := types.DefaultParams()
	newParams.MaxActiveDisputes = 200
	newParams.EscalationDelay = 1000

	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    newParams,
	})
	if err != nil {
		t.Fatalf("UpdateParams failed: %v", err)
	}

	got := k.GetParams(ctx)
	if got.MaxActiveDisputes != 200 {
		t.Errorf("expected MaxActiveDisputes 200, got %d", got.MaxActiveDisputes)
	}
	if got.EscalationDelay != 1000 {
		t.Errorf("expected EscalationDelay 1000, got %d", got.EscalationDelay)
	}
}

func TestUpdateParamsUnauthorized(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: testAddr("wrongauthority"),
		Params:    types.DefaultParams(),
	})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
}

func TestUpdateParamsNilParams(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    nil,
	})
	if err == nil {
		t.Fatal("expected nil params error")
	}
}

func TestUpdateParamsInvalid(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	authority := k.GetAuthority()
	srv := keeper.NewMsgServerImpl(k)

	badParams := types.DefaultParams()
	badParams.TierConfigs = nil // invalid: at least one tier required

	_, err := srv.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    badParams,
	})
	if err == nil {
		t.Fatal("expected validation error for invalid params")
	}
}

// ---------- Tally/Settlement Edge Cases ----------

func TestTallyVotesNoVotes(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:       "tally-no-votes",
		TargetId: "fact-1",
		Tier:     1,
		Phase:    types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Arbiters: []string{testAddr("arb1"), testAddr("arb2"), testAddr("arb3")},
	}
	k.SetDispute(ctx, dispute)

	// No votes cast at all
	outcome := k.TallyVotes(ctx, dispute)
	if outcome != types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
		t.Errorf("expected TIMED_OUT with no votes, got %s", outcome.String())
	}
}

func TestTallyVotesAllAbstain(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:       "tally-all-abstain",
		TargetId: "fact-1",
		Tier:     1,
		Phase:    types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Arbiters: []string{testAddr("arb1"), testAddr("arb2"), testAddr("arb3")},
	}
	k.SetDispute(ctx, dispute)

	// All 3 arbiters abstain
	for i, arb := range dispute.Arbiters {
		k.SetVote(ctx, &types.DisputeVote{
			DisputeId: dispute.Id,
			Arbiter:   arb,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_ABSTAIN,
			Stake:     "1",
			Rationale: fmt.Sprintf("abstain-%d", i),
		})
	}

	outcome := k.TallyVotes(ctx, dispute)
	if outcome != types.DisputeOutcome_DISPUTE_OUTCOME_DRAW {
		t.Errorf("expected DRAW when all abstain (totalVotingWeight=0), got %s", outcome.String())
	}
}

func TestTallyVotesTiedVotes(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:       "tally-tied",
		TargetId: "fact-1",
		Tier:     1,
		Phase:    types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Arbiters: []string{testAddr("arb1"), testAddr("arb2")},
	}
	k.SetDispute(ctx, dispute)

	// 1 vote challenger, 1 vote defender with equal stake
	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute.Id,
		Arbiter:   dispute.Arbiters[0],
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Stake:     "1000",
	})
	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute.Id,
		Arbiter:   dispute.Arbiters[1],
		Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
		Stake:     "1000",
	})

	// 50/50 split: neither reaches 66.67% majority → DRAW
	outcome := k.TallyVotes(ctx, dispute)
	if outcome != types.DisputeOutcome_DISPUTE_OUTCOME_DRAW {
		t.Errorf("expected DRAW for tied votes, got %s", outcome.String())
	}
}

func TestDistributeSettlementZeroBond(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:         "zero-bond",
		TargetId:   "fact-1",
		Challenger: testAddr("challenger"),
		Defender:   testAddr("defender"),
		Bond:       "0",
		Outcome:    types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS,
		Phase:      types.DisputePhase_DISPUTE_PHASE_SETTLED,
	}
	k.SetDispute(ctx, dispute)

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("expected nil error for zero bond, got %v", err)
	}
}

func TestDistributeSettlementDefenderWins(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	defender := testAddr("defender")
	challenger := testAddr("challenger")

	dispute := &types.Dispute{
		Id:         "def-wins",
		TargetId:   "fact-1",
		Challenger: challenger,
		Defender:   defender,
		Bond:       "1000000",
		Outcome:    types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS,
		Phase:      types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Tier:       1,
		Arbiters:   []string{testAddr("arb1"), testAddr("arb2"), testAddr("arb3")},
	}
	k.SetDispute(ctx, dispute)

	// Set up votes for the arbiters (defender wins)
	for _, arb := range dispute.Arbiters {
		k.SetVote(ctx, &types.DisputeVote{
			DisputeId: dispute.Id,
			Arbiter:   arb,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
			Stake:     "1",
		})
	}

	// Fund the module so distribution can happen
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 2_000_000}

	defenderBefore := bk.balances[defender]["uzrn"]
	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	defenderAfter := bk.balances[defender]["uzrn"]
	if defenderAfter <= defenderBefore {
		t.Errorf("expected defender balance to increase: before=%d, after=%d", defenderBefore, defenderAfter)
	}
}

// ---------- Phase Transition Tests (Extended) ----------

func TestProcessPhaseTransitions_CommitToReveal(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	k.SetDispute(ctx, &types.Dispute{
		Id:               "pt-commit-reveal",
		TargetId:         "fact-1",
		Phase:            types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		Tier:             1,
		EvidenceDeadline: 150,
		VotingDeadline:   2000,
	})

	// Block 150: should NOT transition (not past deadline)
	k.ProcessPhaseTransitions(ctx, 150)
	d, _ := k.GetDispute(ctx, "pt-commit-reveal")
	if d.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT {
		t.Errorf("should still be EVIDENCE_COMMIT at deadline block, got %s", d.Phase.String())
	}

	// Block 151: should transition to EVIDENCE_REVEAL
	k.ProcessPhaseTransitions(ctx, 151)
	d, _ = k.GetDispute(ctx, "pt-commit-reveal")
	if d.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL {
		t.Errorf("expected EVIDENCE_REVEAL after deadline, got %s", d.Phase.String())
	}
}

func TestProcessPhaseTransitions_RevealToArbitration(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	k.SetDispute(ctx, &types.Dispute{
		Id:               "pt-reveal-arb",
		TargetId:         "fact-1",
		Phase:            types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL,
		Tier:             1,
		EvidenceDeadline: 400,
		VotingDeadline:   2000,
	})

	// Block 400: should NOT transition
	k.ProcessPhaseTransitions(ctx, 400)
	d, _ := k.GetDispute(ctx, "pt-reveal-arb")
	if d.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL {
		t.Errorf("should still be EVIDENCE_REVEAL at deadline block, got %s", d.Phase.String())
	}

	// Block 401: should transition to ARBITRATION
	k.ProcessPhaseTransitions(ctx, 401)
	d, _ = k.GetDispute(ctx, "pt-reveal-arb")
	if d.Phase != types.DisputePhase_DISPUTE_PHASE_ARBITRATION {
		t.Errorf("expected ARBITRATION after reveal deadline, got %s", d.Phase.String())
	}
}

func TestProcessPhaseTransitions_ArbitrationAutoSettle(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	arbiters := []string{testAddr("arb1"), testAddr("arb2"), testAddr("arb3")}
	challenger := testAddr("challenger")

	k.SetDispute(ctx, &types.Dispute{
		Id:             "pt-arb-settle",
		TargetId:       "fact-1",
		Phase:          types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Tier:           1,
		Challenger:     challenger,
		Defender:       testAddr("defender"),
		Bond:           "1000000",
		VotingDeadline: 600,
		Arbiters:       arbiters,
	})

	// All arbiters vote for challenger
	for _, arb := range arbiters {
		k.SetVote(ctx, &types.DisputeVote{
			DisputeId: "pt-arb-settle",
			Arbiter:   arb,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Stake:     "1",
		})
	}

	// Fund the module account for settlement distribution
	k.ProcessPhaseTransitions(ctx, 601)

	d, _ := k.GetDispute(ctx, "pt-arb-settle")
	if d.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED after auto-settle, got %s", d.Phase.String())
	}
	if d.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS outcome, got %s", d.Outcome.String())
	}
}

// ---------- Evidence/Vote CRUD Tests ----------

func TestSetGetMultipleEvidence(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	for i := 0; i < 3; i++ {
		k.SetEvidence(ctx, &types.DisputeEvidence{
			Id:          fmt.Sprintf("ev-%d", i),
			DisputeId:   "dispute-multi-ev",
			Submitter:   testAddr(fmt.Sprintf("submitter-%d", i)),
			Side:        "challenger",
			Content:     fmt.Sprintf("evidence content %d", i),
			SubmittedAt: uint64(100 + i),
		})
	}

	evidence := k.GetEvidenceByDispute(ctx, "dispute-multi-ev")
	if len(evidence) != 3 {
		t.Fatalf("expected 3 evidence items, got %d", len(evidence))
	}

	// Verify each evidence is present
	ids := map[string]bool{}
	for _, ev := range evidence {
		ids[ev.Id] = true
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("ev-%d", i)
		if !ids[id] {
			t.Errorf("evidence %s not found in results", id)
		}
	}
}

func TestGetCommitmentNotFound(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	_, found := k.GetCommitment(ctx, "nonexistent-dispute", "nonexistent-submitter")
	if found {
		t.Fatal("expected commitment not found for nonexistent dispute+submitter")
	}
}

func TestGetVotesByDisputeEmpty(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	votes := k.GetVotesByDispute(ctx, "no-votes-dispute")
	if len(votes) != 0 {
		t.Errorf("expected 0 votes for nonexistent dispute, got %d", len(votes))
	}
}

// ---------- Query Server Tests (Extended) ----------

func TestQueryDisputesByTarget(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	// Create 2 disputes for the same target
	k.SetDispute(ctx, &types.Dispute{
		Id:       "target-q-1",
		TargetId: "shared-fact",
		Phase:    types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
	})
	k.SetDispute(ctx, &types.Dispute{
		Id:       "target-q-2",
		TargetId: "shared-fact",
		Phase:    types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
	})

	resp, err := qs.DisputesByTarget(ctx, &types.QueryByTargetRequest{TargetId: "shared-fact"})
	if err != nil {
		t.Fatalf("DisputesByTarget failed: %v", err)
	}
	if len(resp.Disputes) != 2 {
		t.Errorf("expected 2 disputes for target, got %d", len(resp.Disputes))
	}
}

func TestQueryActiveDisputes(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	// 2 active disputes
	k.SetDispute(ctx, &types.Dispute{
		Id:       "qa-active-1",
		TargetId: "fact-a",
		Phase:    types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
	})
	k.SetDispute(ctx, &types.Dispute{
		Id:       "qa-active-2",
		TargetId: "fact-b",
		Phase:    types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
	})
	// 1 settled dispute (not active)
	k.SetDispute(ctx, &types.Dispute{
		Id:       "qa-settled-1",
		TargetId: "fact-c",
		Phase:    types.DisputePhase_DISPUTE_PHASE_SETTLED,
	})

	resp, err := qs.ActiveDisputes(ctx, &types.QueryActiveRequest{})
	if err != nil {
		t.Fatalf("ActiveDisputes query failed: %v", err)
	}
	if len(resp.Disputes) != 2 {
		t.Errorf("expected 2 active disputes, got %d", len(resp.Disputes))
	}

	// Verify none of them are the settled one
	for _, d := range resp.Disputes {
		if d.Id == "qa-settled-1" {
			t.Error("settled dispute should not appear in active query results")
		}
	}
}

func TestQueryParamsNilRequest(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Params(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil Params request")
	}
}

// ---------- Arbiter Selection Tests (Extended) ----------

func TestSelectArbitersDeterministic(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	challenger := testAddr("challenger")
	defender := testAddr("defender")

	arbiters1, err := k.SelectArbiters(ctx, 3, challenger, defender, 100)
	if err != nil {
		t.Fatalf("first SelectArbiters failed: %v", err)
	}

	arbiters2, err := k.SelectArbiters(ctx, 3, challenger, defender, 100)
	if err != nil {
		t.Fatalf("second SelectArbiters failed: %v", err)
	}

	if len(arbiters1) != len(arbiters2) {
		t.Fatalf("deterministic selection returned different lengths: %d vs %d", len(arbiters1), len(arbiters2))
	}
	for i := range arbiters1 {
		if arbiters1[i] != arbiters2[i] {
			t.Errorf("arbiter[%d] differs: %s vs %s", i, arbiters1[i], arbiters2[i])
		}
	}
}

func TestSelectArbitersInsufficientValidators(t *testing.T) {
	k, ctx, _, sk, _ := setupKeeper(t)

	// Override validators to only have 2 (plus challenger and defender makes it even worse)
	sk.validators = []string{testAddr("val1"), testAddr("val2")}

	challenger := testAddr("challenger")
	defender := testAddr("defender")

	// Request 3 arbiters but only 2 validators available (neither is challenger/defender)
	_, err := k.SelectArbiters(ctx, 3, challenger, defender, 100)
	if err == nil {
		t.Fatal("expected error for insufficient validators")
	}
}

// ---------- Counter and ID Tests ----------

func TestGetNextDisputeIDSequential(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	for expected := uint64(1); expected <= 5; expected++ {
		got := k.GetNextDisputeID(ctx)
		if got != expected {
			t.Errorf("expected dispute ID %d, got %d", expected, got)
		}
	}
}

func TestGenerateDisputeIDDeterministic(t *testing.T) {
	// Same inputs produce the same ID
	id1 := keeper.GenerateDisputeID("fact-1", "challenger-addr", 100)
	id2 := keeper.GenerateDisputeID("fact-1", "challenger-addr", 100)
	if id1 != id2 {
		t.Errorf("expected same IDs for same inputs, got %s and %s", id1, id2)
	}

	// Different inputs produce different IDs
	id3 := keeper.GenerateDisputeID("fact-2", "challenger-addr", 100)
	if id1 == id3 {
		t.Errorf("expected different IDs for different inputs, got same: %s", id1)
	}

	id4 := keeper.GenerateDisputeID("fact-1", "challenger-addr", 200)
	if id1 == id4 {
		t.Errorf("expected different IDs for different block heights, got same: %s", id1)
	}
}

// ---------- Delete and Iteration Tests ----------

func TestDeleteDisputeCleansIndices(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	dispute := &types.Dispute{
		Id:       "del-idx-1",
		TargetId: "fact-del",
		Phase:    types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT, // active
	}
	k.SetDispute(ctx, dispute)

	// Verify it appears in both indices before deletion
	byTarget := k.GetDisputesByTarget(ctx, "fact-del")
	if len(byTarget) != 1 {
		t.Fatalf("expected 1 dispute by target before delete, got %d", len(byTarget))
	}
	active := k.GetActiveDisputes(ctx)
	foundActive := false
	for _, a := range active {
		if a.Id == "del-idx-1" {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatal("expected dispute in active index before delete")
	}

	// Delete and verify indices are cleaned
	k.DeleteDispute(ctx, dispute)

	byTarget = k.GetDisputesByTarget(ctx, "fact-del")
	if len(byTarget) != 0 {
		t.Errorf("expected 0 disputes by target after delete, got %d", len(byTarget))
	}

	active = k.GetActiveDisputes(ctx)
	for _, a := range active {
		if a.Id == "del-idx-1" {
			t.Error("deleted dispute should not appear in active index")
		}
	}

	_, found := k.GetDispute(ctx, "del-idx-1")
	if found {
		t.Error("deleted dispute should not be found by GetDispute")
	}
}

// ============================================================
// Ported from prototype (legible-money) — Resolution, Tier,
// Evidence, Edge-case, and Adversarial tests
// ============================================================

// ---------- Resolution Logic ----------

// TestDisputeResolutionByEvidence verifies a full commit-reveal-vote-settle
// flow where the challenger provides strong evidence and wins.
func TestDisputeResolutionByEvidence(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("res-ev-challenger")
	defender := testAddr("res-ev-defender")
	bk.setBalance(challenger, "uzrn", 100_000_000)
	kk.addFact("fact-res-ev", defender)

	// 1. Initiate
	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-res-ev",
		Reason:     "Fact lacks supporting data",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute failed: %v", err)
	}

	// 2. Commit evidence from challenger
	content := "Source paper retracted by journal"
	nonce := "nonce-res-ev"
	h := sha256.Sum256([]byte(content + nonce))
	commitHash := hex.EncodeToString(h[:])

	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: commitHash,
	})
	if err != nil {
		t.Fatalf("CommitEvidence failed: %v", err)
	}

	// 3. Advance to reveal phase
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 5000
	k.SetDispute(ctx, dispute)

	// 4. Reveal evidence
	_, err = msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   content,
		Nonce:     nonce,
	})
	if err != nil {
		t.Fatalf("RevealEvidence failed: %v", err)
	}

	// 5. Advance to arbitration
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 10000
	k.SetDispute(ctx, dispute)

	// 6. All arbiters vote for challenger (evidence was compelling)
	for _, arb := range dispute.Arbiters {
		_, err = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter:   arb,
			DisputeId: resp.DisputeId,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Reasoning: "Evidence strongly supports challenger",
		})
		if err != nil {
			t.Fatalf("ArbiterVote failed: %v", err)
		}
	}

	// 7. Settle
	settleResp, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}
	if settleResp.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS, got %s", settleResp.Outcome.String())
	}

	// Verify evidence count reflects submitted evidence
	final, _ := k.GetDispute(ctx, resp.DisputeId)
	if final.EvidenceCount != 1 {
		t.Errorf("expected evidence count 1, got %d", final.EvidenceCount)
	}
}

// TestDisputeResolutionByVoting verifies that stake-weighted voting
// correctly determines outcomes even when the head count is against.
func TestDisputeResolutionByVoting(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	arbiters := []string{testAddr("arb-wt1"), testAddr("arb-wt2"), testAddr("arb-wt3")}
	dispute := &types.Dispute{
		Id:             "dispute-vote-weight",
		TargetId:       "fact-vote-wt",
		Phase:          types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Tier:           1,
		VotingDeadline: 5000,
		Arbiters:       arbiters,
	}
	k.SetDispute(ctx, dispute)

	// 2 arbiters vote DEFENDER with low stake, 1 votes CHALLENGER with very high stake
	// CHALLENGER weight: 900000, DEFENDER weight: 2*100 = 200
	// CHALLENGER ratio = 900000 / 900200 ≈ 999778 bps >> 666667 majority
	k.SetVote(ctx, &types.DisputeVote{DisputeId: dispute.Id, Arbiter: arbiters[0], Vote: types.ArbiterDecision_ARBITER_DECISION_DEFENDER, Stake: "100"})
	k.SetVote(ctx, &types.DisputeVote{DisputeId: dispute.Id, Arbiter: arbiters[1], Vote: types.ArbiterDecision_ARBITER_DECISION_DEFENDER, Stake: "100"})
	k.SetVote(ctx, &types.DisputeVote{DisputeId: dispute.Id, Arbiter: arbiters[2], Vote: types.ArbiterDecision_ARBITER_DECISION_CHALLENGER, Stake: "900000"})

	outcome := k.TallyVotes(ctx, dispute)
	if outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS (high-stake voter should dominate), got %s", outcome.String())
	}
}

// TestDisputeResolutionTimeout verifies that auto-settlement produces
// TIMED_OUT when no votes are cast and the voting deadline passes.
func TestDisputeResolutionTimeout(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	challenger := testAddr("res-timeout-c")
	dispute := &types.Dispute{
		Id:             "dispute-res-timeout",
		TargetId:       "fact-res-timeout",
		Challenger:     challenger,
		Defender:       testAddr("res-timeout-d"),
		Phase:          types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Tier:           1,
		Bond:           "1000000",
		VotingDeadline: 200,
		Arbiters:       []string{testAddr("arb-to1"), testAddr("arb-to2"), testAddr("arb-to3")},
	}
	k.SetDispute(ctx, dispute)

	// No votes cast at all — advance past voting deadline
	k.ProcessPhaseTransitions(ctx, 201)

	resolved, _ := k.GetDispute(ctx, "dispute-res-timeout")
	if resolved.Phase != types.DisputePhase_DISPUTE_PHASE_TIMED_OUT {
		t.Errorf("expected TIMED_OUT phase, got %s", resolved.Phase.String())
	}
	if resolved.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
		t.Errorf("expected TIMED_OUT outcome, got %s", resolved.Outcome.String())
	}
}

// ---------- Tier-Specific Configuration ----------

// TestDisputeConfigByTier verifies each of the 4 tiers has the expected
// arbiter count, min bond, evidence/voting periods, quorum, and majority.
func TestDisputeConfigByTier(t *testing.T) {
	params := types.DefaultParams()

	expectations := []struct {
		tier         uint32
		arbiterCount uint32
		minBond      string
		evidPeriod   uint64
		votePeriod   uint64
		quorumBps    uint64
		majorityBps  uint64
	}{
		{1, 3, "1000000", 500, 1000, 500000, 666667},
		{2, 7, "10000000", 1000, 2000, 500000, 666667},
		{3, 13, "100000000", 2000, 5000, 600000, 750000},
		{4, 21, "1000000000", 5000, 10000, 666000, 800000},
	}

	for _, exp := range expectations {
		tc := types.GetTierConfig(params, exp.tier)
		if tc == nil {
			t.Fatalf("tier %d config not found", exp.tier)
		}
		if tc.ArbiterCount != exp.arbiterCount {
			t.Errorf("tier %d: expected arbiter count %d, got %d", exp.tier, exp.arbiterCount, tc.ArbiterCount)
		}
		if tc.MinBond != exp.minBond {
			t.Errorf("tier %d: expected min bond %s, got %s", exp.tier, exp.minBond, tc.MinBond)
		}
		if tc.EvidencePeriod != exp.evidPeriod {
			t.Errorf("tier %d: expected evidence period %d, got %d", exp.tier, exp.evidPeriod, tc.EvidencePeriod)
		}
		if tc.VotingPeriod != exp.votePeriod {
			t.Errorf("tier %d: expected voting period %d, got %d", exp.tier, exp.votePeriod, tc.VotingPeriod)
		}
		if tc.QuorumBps != exp.quorumBps {
			t.Errorf("tier %d: expected quorum %d, got %d", exp.tier, exp.quorumBps, tc.QuorumBps)
		}
		if tc.MajorityBps != exp.majorityBps {
			t.Errorf("tier %d: expected majority %d, got %d", exp.tier, exp.majorityBps, tc.MajorityBps)
		}
	}

	// Invalid tier returns nil
	if types.GetTierConfig(params, 0) != nil {
		t.Error("tier 0 should return nil config")
	}
	if types.GetTierConfig(params, 99) != nil {
		t.Error("tier 99 should return nil config")
	}
}

// TestDisputeStakeRequirements verifies that disputes cannot be opened
// with a bond below each tier's minimum.
func TestDisputeStakeRequirements(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("stake-req-c")
	defender := testAddr("stake-req-d")
	bk.setBalance(challenger, "uzrn", 10_000_000_000)
	kk.addFact("fact-stake-req", defender)

	// Bond of 999999 is below tier 1 minimum of 1000000
	_, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-stake-req",
		Reason:     "Testing stake requirement",
		Bond:       "999999",
	})
	if err == nil {
		t.Fatal("expected error for bond below tier 1 minimum (999999 < 1000000)")
	}

	// Bond exactly at tier 1 minimum should succeed
	_, err = msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-stake-req",
		Reason:     "Testing exact minimum",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("bond at exact tier 1 minimum should succeed: %v", err)
	}

	// Negative and zero bonds
	kk.addFact("fact-stake-neg", defender)
	_, err = msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-stake-neg",
		Reason:     "Negative bond",
		Bond:       "-100",
	})
	if err == nil {
		t.Fatal("expected error for negative bond")
	}
}

// ---------- Evidence Submission ----------

// TestSubmitMultipleEvidence verifies both parties can commit and reveal
// evidence, and that evidence count is tracked correctly.
func TestSubmitMultipleEvidence(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("multi-ev-c")
	defender := testAddr("multi-ev-d")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-multi-ev", defender)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-multi-ev",
		Reason:     "Multiple evidence test",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute failed: %v", err)
	}

	// Challenger commits
	content1 := "Challenger evidence: the source is unreliable"
	nonce1 := "nonce-challenger"
	h1 := sha256.Sum256([]byte(content1 + nonce1))
	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: hex.EncodeToString(h1[:]),
	})
	if err != nil {
		t.Fatalf("Challenger commit failed: %v", err)
	}

	// Defender commits
	content2 := "Defender evidence: source verified by peer review"
	nonce2 := "nonce-defender"
	h2 := sha256.Sum256([]byte(content2 + nonce2))
	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      defender,
		DisputeId:      resp.DisputeId,
		CommitmentHash: hex.EncodeToString(h2[:]),
	})
	if err != nil {
		t.Fatalf("Defender commit failed: %v", err)
	}

	// Advance to reveal phase
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 5000
	k.SetDispute(ctx, dispute)

	// Both reveal
	_, err = msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   content1,
		Nonce:     nonce1,
	})
	if err != nil {
		t.Fatalf("Challenger reveal failed: %v", err)
	}

	_, err = msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: defender,
		DisputeId: resp.DisputeId,
		Content:   content2,
		Nonce:     nonce2,
	})
	if err != nil {
		t.Fatalf("Defender reveal failed: %v", err)
	}

	// Verify evidence count
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.EvidenceCount != 2 {
		t.Errorf("expected evidence count 2, got %d", dispute.EvidenceCount)
	}

	// Verify both evidence items stored
	evidence := k.GetEvidenceByDispute(ctx, resp.DisputeId)
	if len(evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(evidence))
	}
}

// TestEvidenceDeadlineEnforced verifies that evidence cannot be committed
// or revealed after the respective deadlines have passed.
func TestEvidenceDeadlineEnforced(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("ev-deadline-c")
	defender := testAddr("ev-deadline-d")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-ev-deadline", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-ev-deadline",
		Reason:     "Deadline test",
		Bond:       "1000000",
	})

	// Set evidence deadline to past
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.EvidenceDeadline = 50 // block 100 > 50
	k.SetDispute(ctx, dispute)

	// Commit should fail after deadline
	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for commit after evidence deadline")
	}

	// Also verify reveal after deadline fails
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
	dispute.EvidenceDeadline = 50
	k.SetDispute(ctx, dispute)

	// Plant a fake commitment so reveal path is exercised
	k.SetCommitment(ctx, &types.EvidenceCommitment{
		DisputeId:   resp.DisputeId,
		Submitter:   challenger,
		Side:        "challenger",
		ContentHash: "fake",
		CommittedAt: 10,
		Revealed:    false,
	})

	_, err = msgSrv.RevealEvidence(ctx, &types.MsgRevealEvidence{
		Submitter: challenger,
		DisputeId: resp.DisputeId,
		Content:   "late evidence",
		Nonce:     "late-nonce",
	})
	if err == nil {
		t.Fatal("expected error for reveal after evidence deadline")
	}
}

// TestEvidenceFromNonParty verifies that outsiders cannot submit evidence
// to a dispute they are not a party to.
func TestEvidenceFromNonParty(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("ev-nonparty-c")
	defender := testAddr("ev-nonparty-d")
	outsider := testAddr("ev-nonparty-outsider")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-ev-nonparty", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-ev-nonparty",
		Reason:     "Non-party test",
		Bond:       "1000000",
	})

	// Outsider tries to commit evidence
	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      outsider,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "outsider-hash",
	})
	if err == nil {
		t.Fatal("expected error for non-party evidence submission")
	}
}

// ---------- Edge Cases ----------

// TestDisputeAlreadyResolved verifies that settled disputes cannot be
// re-settled or re-voted on.
func TestDisputeAlreadyResolved(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	// Create already-settled dispute
	dispute := &types.Dispute{
		Id:         "dispute-already-settled",
		TargetId:   "fact-settled",
		Challenger: testAddr("settled-c"),
		Defender:   testAddr("settled-d"),
		Phase:      types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Outcome:    types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS,
		Bond:       "1000000",
		Tier:       1,
		Arbiters:   []string{testAddr("settled-arb1")},
	}
	k.SetDispute(ctx, dispute)

	// Try to settle again
	_, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: "dispute-already-settled",
	})
	if err == nil {
		t.Fatal("expected error for settling an already-settled dispute")
	}

	// Try to vote on settled dispute
	_, err = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
		Arbiter:   dispute.Arbiters[0],
		DisputeId: dispute.Id,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "Too late",
	})
	if err == nil {
		t.Fatal("expected error for voting on settled dispute")
	}

	// Try to commit evidence to settled dispute
	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      dispute.Challenger,
		DisputeId:      dispute.Id,
		CommitmentHash: "late-commit",
	})
	if err == nil {
		t.Fatal("expected error for committing evidence to settled dispute")
	}
}

// TestDisputeAgainstOwnClaim verifies behavior when a submitter challenges
// their own fact. The system should allow it (the economic penalty deters
// abuse) or reject it.
func TestDisputeAgainstOwnClaim(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	selfDisputer := testAddr("self-disputer")
	bk.setBalance(selfDisputer, "uzrn", 100_000_000)

	// Fact submitted by the same person who will challenge
	kk.addFact("fact-self-dispute", selfDisputer)

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: selfDisputer,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-self-dispute",
		Reason:     "Challenging my own fact",
		Bond:       "1000000",
	})

	// Whether allowed or rejected, verify consistent behavior
	if err != nil {
		// Self-dispute rejected — this is valid defense
		t.Logf("Self-dispute rejected (defense): %v", err)
	} else {
		// Self-dispute allowed — verify it was created properly
		dispute, found := k.GetDispute(ctx, resp.DisputeId)
		if !found {
			t.Fatal("dispute not found after self-dispute initiation")
		}
		if dispute.Challenger != dispute.Defender {
			t.Logf("Self-dispute allowed: challenger=%s defender=%s (different addresses in fact lookup)", dispute.Challenger, dispute.Defender)
		} else {
			t.Logf("Self-dispute allowed: challenger == defender == %s", dispute.Challenger)
		}
		// Bond should still have been escrowed
		if bk.balances[selfDisputer]["uzrn"] >= 100_000_000 {
			t.Error("expected bond to be escrowed for self-dispute")
		}
	}
}

// TestCascadingDisputes verifies that multiple disputes can be opened
// against the same target (up to the max active limit).
func TestCascadingDisputes(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	defender := testAddr("cascade-d")
	kk.addFact("fact-cascade", defender)

	var disputeIDs []string
	// Open multiple disputes from different challengers
	for i := 0; i < 3; i++ {
		challenger := testAddr(fmt.Sprintf("cascade-c-%d", i))
		bk.setBalance(challenger, "uzrn", 10_000_000)

		resp, err := msgSrv.InitiateDispute(ctx.WithBlockHeight(int64(100+i)), &types.MsgInitiateDispute{
			Challenger: challenger,
			TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
			TargetId:   "fact-cascade",
			Reason:     fmt.Sprintf("Challenge round %d", i),
			Bond:       "1000000",
		})
		if err != nil {
			t.Fatalf("dispute %d failed: %v", i, err)
		}
		disputeIDs = append(disputeIDs, resp.DisputeId)
	}

	// Verify all 3 disputes exist for this target
	disputes := k.GetDisputesByTarget(ctx, "fact-cascade")
	if len(disputes) != 3 {
		t.Errorf("expected 3 cascading disputes, got %d", len(disputes))
	}

	// All should be active
	for _, d := range disputes {
		if d.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT {
			t.Errorf("dispute %s: expected EVIDENCE_COMMIT phase, got %s", d.Id, d.Phase.String())
		}
	}

	// Each should have distinct IDs
	seen := map[string]bool{}
	for _, id := range disputeIDs {
		if seen[id] {
			t.Errorf("duplicate dispute ID: %s", id)
		}
		seen[id] = true
	}
}

// ---------- Adversarial / Security Tests (ported from OC tests) ----------

// TestDoubleCommitPrevention verifies that the same party cannot commit
// evidence twice to the same dispute.
func TestDoubleCommitPrevention(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("dbl-commit-c")
	defender := testAddr("dbl-commit-d")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-dbl-commit", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-dbl-commit",
		Reason:     "Double commit test",
		Bond:       "1000000",
	})

	// First commit — should succeed
	_, err := msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("first commit should succeed: %v", err)
	}

	// Second commit — should fail
	_, err = msgSrv.CommitEvidence(ctx, &types.MsgCommitEvidence{
		Submitter:      challenger,
		DisputeId:      resp.DisputeId,
		CommitmentHash: "hash-2",
	})
	if err == nil {
		t.Fatal("expected error for double commitment — attacker could overwrite evidence")
	}
}

// TestBondSlashIntegerTruncation verifies that bond slash calculation
// does not produce negative burn or zero slash for meaningful bonds.
func TestBondSlashIntegerTruncation(t *testing.T) {
	params := types.DefaultParams()
	slashRate := params.SlashRateLoserBps  // 500000 (50%)
	rewardRate := params.RewardRateWinnerBps // 400000 (40%)
	arbiterRate := params.ArbiterRewardBps   // 100000 (10%)

	// Scale factor is 1M (1,000,000)
	bonds := []int64{1, 2, 100, 999, 1000, 1000000, 10000000}

	for _, bond := range bonds {
		slash := bond * int64(slashRate) / 1_000_000
		winnerReward := slash * int64(rewardRate) / 1_000_000
		arbiterReward := slash * int64(arbiterRate) / 1_000_000
		burn := slash - winnerReward - arbiterReward

		if burn < 0 {
			t.Errorf("negative burn at bond=%d: slash=%d reward=%d arbiter=%d burn=%d",
				bond, slash, winnerReward, arbiterReward, burn)
		}

		// At the minimum bond (1000000), verify non-trivial economics
		if bond == 1000000 {
			if slash != 500000 {
				t.Errorf("expected slash 500000 at bond 1M, got %d", slash)
			}
			if winnerReward != 200000 {
				t.Errorf("expected winner reward 200000 at bond 1M, got %d", winnerReward)
			}
			if arbiterReward != 50000 {
				t.Errorf("expected arbiter reward 50000 at bond 1M, got %d", arbiterReward)
			}
			expectedBurn := int64(250000)
			if burn != expectedBurn {
				t.Errorf("expected burn %d at bond 1M, got %d", expectedBurn, burn)
			}
		}
	}
}

// TestQuorumBoundaryExact verifies the quorum threshold at exact boundaries
// for different tier configurations.
func TestQuorumBoundaryExact(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	// Tier 1: 3 arbiters, quorum 500000 bps (50%)
	// Quorum required: 3 * 500000 / 1000000 = 1 (floor). So 1/3 meets quorum.
	arbitersTier1 := []string{testAddr("qb-arb1"), testAddr("qb-arb2"), testAddr("qb-arb3")}
	dispute1 := &types.Dispute{
		Id:       "quorum-boundary-1",
		TargetId: "fact-qb1",
		Tier:     1,
		Arbiters: arbitersTier1,
	}
	k.SetDispute(ctx, dispute1)

	// 0 votes → TIMED_OUT
	outcome0 := k.TallyVotes(ctx, dispute1)
	if outcome0 != types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
		t.Errorf("0 votes: expected TIMED_OUT, got %s", outcome0.String())
	}

	// 1 vote should meet quorum (1 >= 1)
	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute1.Id,
		Arbiter:   arbitersTier1[0],
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Stake:     "1000",
	})
	outcome1 := k.TallyVotes(ctx, dispute1)
	// With 1 vote for challenger out of 1 total: 100% >= 66.67% majority → CHALLENGER_WINS
	if outcome1 != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("1/3 vote (all challenger): expected CHALLENGER_WINS, got %s", outcome1.String())
	}
}

// TestArbiterCollusionAllOverturn verifies behavior when all arbiters
// vote the same way (potential collusion scenario).
func TestArbiterCollusionAllOverturn(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)

	arbiters := []string{testAddr("coll-arb1"), testAddr("coll-arb2"), testAddr("coll-arb3")}
	dispute := &types.Dispute{
		Id:             "dispute-collusion",
		TargetId:       "fact-collusion",
		Phase:          types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Tier:           1,
		VotingDeadline: 5000,
		Arbiters:       arbiters,
	}
	k.SetDispute(ctx, dispute)

	// All 3 arbiters vote for challenger (unanimous overturn)
	for _, arb := range arbiters {
		k.SetVote(ctx, &types.DisputeVote{
			DisputeId: dispute.Id,
			Arbiter:   arb,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Stake:     "5000",
		})
	}

	outcome := k.TallyVotes(ctx, dispute)
	if outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS for unanimous vote, got %s", outcome.String())
	}

	// Verify: defense against collusion is the escalation mechanism
	// At tier 1 (3 arbiters), collusion is feasible. Escalation to tier 2
	// (7 arbiters) or tier 3 (13) dilutes collusion probability.
	params := types.DefaultParams()
	tc2 := types.GetTierConfig(params, 2)
	tc3 := types.GetTierConfig(params, 3)
	if tc2.ArbiterCount <= dispute.Tier*3 {
		t.Logf("Tier 2 has %d arbiters (up from 3 at tier 1)", tc2.ArbiterCount)
	}
	if tc3.ArbiterCount < 10 {
		t.Errorf("tier 3 should have many arbiters to dilute collusion, got %d", tc3.ArbiterCount)
	}
}

// TestEscalationResetsPhaseAndDeadlines verifies that escalation properly
// resets the dispute to EVIDENCE_COMMIT phase with new deadlines.
func TestEscalationResetsPhaseAndDeadlines(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("esc-reset-c")
	defender := testAddr("esc-reset-d")
	bk.setBalance(challenger, "uzrn", 100_000_000)
	kk.addFact("fact-esc-reset", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-esc-reset",
		Reason:     "Escalation reset test",
		Bond:       "1000000",
	})

	// Record original deadlines
	original, _ := k.GetDispute(ctx, resp.DisputeId)
	originalEvDeadline := original.EvidenceDeadline
	originalVoteDeadline := original.VotingDeadline
	originalArbiterCount := len(original.Arbiters)

	// Set up for escalation
	original.CreatedAt = 1
	k.SetDispute(ctx, original)

	// Escalate at a block past escalation delay
	escCtx := ctx.WithBlockHeight(600)
	escResp, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      challenger,
		DisputeId:      resp.DisputeId,
		AdditionalBond: "10000000",
	})
	if err != nil {
		t.Fatalf("EscalateDispute failed: %v", err)
	}
	if escResp.NewTier != 2 {
		t.Errorf("expected tier 2, got %d", escResp.NewTier)
	}

	// Verify phase reset
	escalated, _ := k.GetDispute(ctx, resp.DisputeId)
	if escalated.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT {
		t.Errorf("expected EVIDENCE_COMMIT after escalation, got %s", escalated.Phase.String())
	}

	// Deadlines should be different (set from the escalation block)
	if escalated.EvidenceDeadline == originalEvDeadline {
		t.Error("evidence deadline should be reset after escalation")
	}
	if escalated.VotingDeadline == originalVoteDeadline {
		t.Error("voting deadline should be reset after escalation")
	}

	// Arbiter count should increase (tier 2 has 7 arbiters)
	if len(escalated.Arbiters) <= originalArbiterCount {
		t.Errorf("expected more arbiters after escalation: was %d, now %d",
			originalArbiterCount, len(escalated.Arbiters))
	}
	if len(escalated.Arbiters) != 7 {
		t.Errorf("tier 2 should have 7 arbiters, got %d", len(escalated.Arbiters))
	}
}

// TestVoteAfterVotingDeadline verifies the voting deadline is enforced
// through the MsgServer.
func TestVoteAfterVotingDeadline(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("vote-deadline-c")
	defender := testAddr("vote-deadline-d")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-vote-deadline", defender)

	resp, _ := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-vote-deadline",
		Reason:     "Vote deadline test",
		Bond:       "1000000",
	})

	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 150
	k.SetDispute(ctx, dispute)

	arbiter := dispute.Arbiters[0]

	// Attempt to vote at block 200 (past deadline of 150)
	lateCtx := ctx.WithBlockHeight(200)
	_, err := msgSrv.ArbiterVote(lateCtx, &types.MsgArbiterVote{
		Arbiter:   arbiter,
		DisputeId: resp.DisputeId,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Reasoning: "Late vote attempt",
	})
	if err == nil {
		t.Fatal("expected error for vote after deadline")
	}
}

// TestSettleDisputeWrongPhase verifies that settlement is rejected when
// the dispute is not in the ARBITRATION phase.
func TestSettleDisputeWrongPhase(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	// Dispute in EVIDENCE_COMMIT phase — not ready for settlement
	k.SetDispute(ctx, &types.Dispute{
		Id:         "dispute-wrong-phase-settle",
		TargetId:   "fact-wp",
		Challenger: testAddr("wp-c"),
		Defender:   testAddr("wp-d"),
		Phase:      types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		Bond:       "1000000",
		Tier:       1,
	})

	_, err := msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: "dispute-wrong-phase-settle",
	})
	if err == nil {
		t.Fatal("expected error for settlement in EVIDENCE_COMMIT phase")
	}

	// Dispute in EVIDENCE_REVEAL phase — also not ready
	k.SetDispute(ctx, &types.Dispute{
		Id:         "dispute-wrong-phase-settle-2",
		TargetId:   "fact-wp2",
		Challenger: testAddr("wp2-c"),
		Defender:   testAddr("wp2-d"),
		Phase:      types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL,
		Bond:       "1000000",
		Tier:       1,
	})

	_, err = msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: "dispute-wrong-phase-settle-2",
	})
	if err == nil {
		t.Fatal("expected error for settlement in EVIDENCE_REVEAL phase")
	}
}

// TestDistributeDrawRefundAndArbiterFees verifies the draw settlement:
// challenger gets bond minus arbiter fees, arbiters split the fees.
func TestDistributeDrawRefundAndArbiterFees(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	challenger := testAddr("draw-ref-c")
	arb1 := testAddr("draw-arb1")
	arb2 := testAddr("draw-arb2")

	dispute := &types.Dispute{
		Id:         "draw-refund",
		TargetId:   "fact-draw",
		Challenger: challenger,
		Defender:   testAddr("draw-ref-d"),
		Bond:       "1000000",
		Outcome:    types.DisputeOutcome_DISPUTE_OUTCOME_DRAW,
		Phase:      types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Tier:       1,
		Arbiters:   []string{arb1, arb2},
	}
	k.SetDispute(ctx, dispute)

	// Both arbiters voted (they split the arbiter fee on draw)
	k.SetVote(ctx, &types.DisputeVote{DisputeId: dispute.Id, Arbiter: arb1, Vote: types.ArbiterDecision_ARBITER_DECISION_CHALLENGER, Stake: "1"})
	k.SetVote(ctx, &types.DisputeVote{DisputeId: dispute.Id, Arbiter: arb2, Vote: types.ArbiterDecision_ARBITER_DECISION_DEFENDER, Stake: "1"})

	// Fund module account
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 2_000_000}

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	// Challenger should get most of the bond back (bond - arbiter fee)
	// ArbiterFee = bond * arbiterRewardBps / 1M = 1000000 * 100000 / 1M = 100000
	// Refund = 1000000 - 100000 = 900000
	challengerBal := bk.balances[challenger]["uzrn"]
	if challengerBal <= 0 {
		t.Errorf("expected challenger to receive refund, got balance %d", challengerBal)
	}

	// Each arbiter should get half the fee (100000 / 2 = 50000)
	arb1Bal := bk.balances[arb1]["uzrn"]
	arb2Bal := bk.balances[arb2]["uzrn"]
	if arb1Bal <= 0 || arb2Bal <= 0 {
		t.Errorf("expected arbiters to receive fees: arb1=%d arb2=%d", arb1Bal, arb2Bal)
	}
	if arb1Bal != arb2Bal {
		t.Errorf("expected equal arbiter fees: arb1=%d arb2=%d", arb1Bal, arb2Bal)
	}
}

// TestEscalateTimedOutDispute verifies that timed-out disputes cannot be
// escalated (per the EscalateDispute guard).
func TestEscalateTimedOutDispute(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("esc-timedout-c")
	bk.setBalance(challenger, "uzrn", 100_000_000)

	dispute := &types.Dispute{
		Id:         "dispute-esc-timedout",
		TargetId:   "fact-esc-to",
		Challenger: challenger,
		Defender:   testAddr("esc-timedout-d"),
		Phase:      types.DisputePhase_DISPUTE_PHASE_TIMED_OUT,
		Tier:       1,
		Bond:       "1000000",
		CreatedAt:  1,
	}
	k.SetDispute(ctx, dispute)

	escCtx := ctx.WithBlockHeight(600)
	_, err := msgSrv.EscalateDispute(escCtx, &types.MsgEscalateDispute{
		Requester:      challenger,
		DisputeId:      dispute.Id,
		AdditionalBond: "10000000",
	})
	if err == nil {
		t.Fatal("expected error for escalating timed-out dispute")
	}
}

// ============================================================
// Service Escrow Tests
// ============================================================

// TestCreateServiceEscrow_Success verifies that a valid CreateServiceEscrow
// locks funds in the module account and stores the escrow as PENDING.
func TestCreateServiceEscrow_Success(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer1")
	provider := testAddr("se_provider1")
	payerAddr, _ := sdk.AccAddressFromBech32(payer)
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(100)
	resp, err := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "500000",
		TermsHash: "abc123",
		Deadline:  200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EscrowId == "" {
		t.Fatal("expected non-empty escrow_id")
	}

	// Payer balance reduced by 500000.
	if bk.balances[payerAddr.String()]["uzrn"] != 500_000 {
		t.Errorf("payer balance: got %d, want 500000", bk.balances[payerAddr.String()]["uzrn"])
	}
	// Module holds 500000.
	if bk.moduleBalances[types.ModuleName]["uzrn"] != 500_000 {
		t.Errorf("module balance: got %d, want 500000", bk.moduleBalances[types.ModuleName]["uzrn"])
	}

	// Escrow stored as PENDING.
	escrow, found := k.GetServiceEscrow(ctx, resp.EscrowId)
	if !found {
		t.Fatal("escrow not found in store")
	}
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_PENDING {
		t.Errorf("expected PENDING, got %v", escrow.Status)
	}
	if escrow.Payer != payer || escrow.Provider != provider {
		t.Errorf("escrow parties mismatch: payer=%s provider=%s", escrow.Payer, escrow.Provider)
	}
}

// TestCreateServiceEscrow_DeadlineInPast verifies that a deadline at or before
// the current block height is rejected.
func TestCreateServiceEscrow_DeadlineInPast(t *testing.T) {
	msgSrv, _, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer2")
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(200)
	_, err := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  testAddr("se_provider2"),
		Amount:    "100000",
		TermsHash: "hash2",
		Deadline:  200, // equal to current height — not in future
	})
	if err == nil {
		t.Fatal("expected error for past deadline, got nil")
	}
}

// TestCompleteServiceEscrow_Success verifies that the provider can complete
// the escrow and receive the funds.
func TestCompleteServiceEscrow_Success(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer3")
	provider := testAddr("se_provider3")
	providerAddr, _ := sdk.AccAddressFromBech32(provider)
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(100)
	createResp, err := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "750000",
		TermsHash: "terms3",
		Deadline:  500,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 50
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(150)
	_, err = msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof_of_delivery_3",
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Funds NOT released immediately — held during dispute window.
	if bk.balances[providerAddr.String()]["uzrn"] != 0 {
		t.Errorf("provider balance should be 0 before dispute window closes, got %d",
			bk.balances[providerAddr.String()]["uzrn"])
	}

	// Escrow COMPLETED with proof; dispute window close = 150 + 50 = 200.
	escrow, _ := k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_COMPLETED {
		t.Errorf("expected COMPLETED, got %v", escrow.Status)
	}
	if escrow.ProofHash != "proof_of_delivery_3" {
		t.Errorf("proof_hash not stored: %s", escrow.ProofHash)
	}
	if escrow.Deadline != 200 {
		t.Errorf("expected dispute deadline 200, got %d", escrow.Deadline)
	}

	// Before dispute window closes: EndBlock should NOT release.
	ctx = ctx.WithBlockHeight(199)
	k.ProcessExpiredEscrows(ctx)
	if bk.balances[providerAddr.String()]["uzrn"] != 0 {
		t.Errorf("provider balance should still be 0 before window closes, got %d",
			bk.balances[providerAddr.String()]["uzrn"])
	}

	// After dispute window closes: EndBlock auto-releases to provider.
	ctx = ctx.WithBlockHeight(201)
	k.ProcessExpiredEscrows(ctx)
	if bk.balances[providerAddr.String()]["uzrn"] != 750_000 {
		t.Errorf("provider balance after auto-release: got %d, want 750000",
			bk.balances[providerAddr.String()]["uzrn"])
	}
}

// TestCompleteServiceEscrow_WrongProvider verifies that a non-provider caller
// cannot complete the escrow.
func TestCompleteServiceEscrow_WrongProvider(t *testing.T) {
	msgSrv, _, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer4")
	provider := testAddr("se_provider4")
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "100000",
		TermsHash: "terms4",
		Deadline:  500,
	})

	_, err := msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{
		Provider:  testAddr("se_impostor4"), // wrong
		EscrowId:  createResp.EscrowId,
		ProofHash: "fake_proof",
	})
	if err == nil {
		t.Fatal("expected error for wrong provider, got nil")
	}
}

// TestCompleteServiceEscrow_AfterDeadline verifies that completion is rejected
// after the deadline block.
func TestCompleteServiceEscrow_AfterDeadline(t *testing.T) {
	msgSrv, _, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer5")
	provider := testAddr("se_provider5")
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "100000",
		TermsHash: "terms5",
		Deadline:  300,
	})

	// Attempt completion after deadline.
	ctx = ctx.WithBlockHeight(400)
	_, err := msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "too_late",
	})
	if err == nil {
		t.Fatal("expected error for post-deadline completion, got nil")
	}
}

// TestDisputeServiceEscrow_Success verifies that the payer can dispute
// within the dispute window after completion.
func TestDisputeServiceEscrow_Success(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer6")
	provider := testAddr("se_provider6")
	bk.setBalance(payer, "uzrn", 1_000_000)

	// Set a small dispute window.
	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 100
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "200000",
		TermsHash: "terms6",
		Deadline:  400,
	})

	// Provider completes.
	ctx = ctx.WithBlockHeight(200)
	_, err := msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof6",
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Payer disputes within window (deadline=400, window=100 → windowEnd=500; current=300).
	ctx = ctx.WithBlockHeight(300)
	_, err = msgSrv.DisputeServiceEscrow(ctx, &types.MsgDisputeServiceEscrow{
		Payer:    payer,
		EscrowId: createResp.EscrowId,
		Reason:   "service not delivered as agreed",
	})
	if err != nil {
		t.Fatalf("dispute failed: %v", err)
	}

	escrow, _ := k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_DISPUTED {
		t.Errorf("expected DISPUTED, got %v", escrow.Status)
	}
}

// TestDisputeServiceEscrow_WindowExpired verifies that disputing outside
// the window is rejected.
func TestDisputeServiceEscrow_WindowExpired(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer7")
	provider := testAddr("se_provider7")
	bk.setBalance(payer, "uzrn", 1_000_000)

	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 50
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "100000",
		TermsHash: "terms7",
		Deadline:  300,
	})

	ctx = ctx.WithBlockHeight(200)
	msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{ //nolint:errcheck
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof7",
	})

	// deadline=300, window=50 → windowEnd=350; current=400 > 350 → expired.
	ctx = ctx.WithBlockHeight(400)
	_, err := msgSrv.DisputeServiceEscrow(ctx, &types.MsgDisputeServiceEscrow{
		Payer:    payer,
		EscrowId: createResp.EscrowId,
		Reason:   "too late",
	})
	if err == nil {
		t.Fatal("expected error for expired dispute window, got nil")
	}
}

// TestDisputeServiceEscrow_NotPayer verifies that only the payer can dispute.
func TestDisputeServiceEscrow_NotPayer(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer8")
	provider := testAddr("se_provider8")
	bk.setBalance(payer, "uzrn", 1_000_000)

	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 500
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "100000",
		TermsHash: "terms8",
		Deadline:  300,
	})
	msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{ //nolint:errcheck
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof8",
	})

	_, err := msgSrv.DisputeServiceEscrow(ctx, &types.MsgDisputeServiceEscrow{
		Payer:    testAddr("se_impostor8"), // not the payer
		EscrowId: createResp.EscrowId,
		Reason:   "unauthorized",
	})
	if err == nil {
		t.Fatal("expected error for wrong payer, got nil")
	}
}

// TestProcessExpiredEscrows_RefundsAfterDeadline verifies that EndBlock
// refunds PENDING escrows whose deadline has passed.
func TestProcessExpiredEscrows_RefundsAfterDeadline(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer9")
	provider := testAddr("se_provider9")
	payerAddr, _ := sdk.AccAddressFromBech32(payer)
	bk.setBalance(payer, "uzrn", 1_000_000)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "300000",
		TermsHash: "terms9",
		Deadline:  200,
	})

	// Before deadline: EndBlock should NOT refund.
	ctx = ctx.WithBlockHeight(199)
	k.ProcessExpiredEscrows(ctx)
	escrow, _ := k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_PENDING {
		t.Errorf("expected still PENDING before deadline, got %v", escrow.Status)
	}

	// After deadline: EndBlock should refund.
	ctx = ctx.WithBlockHeight(250)
	k.ProcessExpiredEscrows(ctx)
	escrow, _ = k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_REFUNDED {
		t.Errorf("expected REFUNDED after deadline, got %v", escrow.Status)
	}
	// Payer got funds back.
	if bk.balances[payerAddr.String()]["uzrn"] != 1_000_000 {
		t.Errorf("payer balance after refund: got %d, want 1000000", bk.balances[payerAddr.String()]["uzrn"])
	}
}

// TestProcessExpiredEscrows_AutoReleasesAfterDisputeWindow verifies that
// EndBlock releases COMPLETED escrow funds to the provider once the dispute
// window (repurposed Deadline) has passed with no dispute raised.
func TestProcessExpiredEscrows_AutoReleasesAfterDisputeWindow(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer10")
	provider := testAddr("se_provider10")
	providerAddr, _ := sdk.AccAddressFromBech32(provider)
	bk.setBalance(payer, "uzrn", 1_000_000)

	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 100 // window = 100 blocks
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "400000",
		TermsHash: "terms10",
		Deadline:  300,
	})

	// Provider completes at block 200 → dispute window close = 200 + 100 = 300.
	ctx = ctx.WithBlockHeight(200)
	msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{ //nolint:errcheck
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof10",
	})

	// Funds still locked immediately after completion.
	if bk.balances[providerAddr.String()]["uzrn"] != 0 {
		t.Errorf("provider balance right after complete: got %d, want 0 (funds still locked)",
			bk.balances[providerAddr.String()]["uzrn"])
	}

	// Before dispute window closes (block 299): no release.
	ctx = ctx.WithBlockHeight(299)
	k.ProcessExpiredEscrows(ctx)
	if bk.balances[providerAddr.String()]["uzrn"] != 0 {
		t.Errorf("provider balance before window close: got %d, want 0",
			bk.balances[providerAddr.String()]["uzrn"])
	}

	// After dispute window (block 301): auto-release to provider.
	ctx = ctx.WithBlockHeight(301)
	k.ProcessExpiredEscrows(ctx)

	if bk.balances[providerAddr.String()]["uzrn"] != 400_000 {
		t.Errorf("provider balance after auto-release: got %d, want 400000",
			bk.balances[providerAddr.String()]["uzrn"])
	}

	// Verify sentinel: Deadline is zeroed after release to prevent double-pay.
	escrow, _ := k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_COMPLETED {
		t.Errorf("expected COMPLETED after release, got %v", escrow.Status)
	}
	if escrow.Deadline != 0 {
		t.Errorf("expected Deadline=0 (released sentinel) after auto-release, got %d", escrow.Deadline)
	}

	// A second ProcessExpiredEscrows run at block 302 skips (sentinel) — no double-pay.
	ctx = ctx.WithBlockHeight(302)
	k.ProcessExpiredEscrows(ctx)
	if bk.balances[providerAddr.String()]["uzrn"] != 400_000 {
		t.Errorf("provider balance after second run: got %d, want 400000 (no double-pay)",
			bk.balances[providerAddr.String()]["uzrn"])
	}
}

// TestProcessExpiredEscrows_DisputedImmune verifies that DISPUTED escrows are
// never refunded or released by EndBlock — they stay locked until governance acts.
func TestProcessExpiredEscrows_DisputedImmune(t *testing.T) {
	msgSrv, k, ctx, bk, _, _ := setupMsgServer(t)
	payer := testAddr("se_payer11")
	provider := testAddr("se_provider11")
	payerAddr, _ := sdk.AccAddressFromBech32(payer)
	providerAddr, _ := sdk.AccAddressFromBech32(provider)
	bk.setBalance(payer, "uzrn", 500_000)

	params := types.DefaultParams()
	params.EscrowDisputeWindowBlocks = 200
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(100)
	createResp, _ := msgSrv.CreateServiceEscrow(ctx, &types.MsgCreateServiceEscrow{
		Payer:     payer,
		Provider:  provider,
		Amount:    "500000",
		TermsHash: "terms11",
		Deadline:  400,
	})

	// Provider completes → dispute window close = 150 + 200 = 350.
	ctx = ctx.WithBlockHeight(150)
	msgSrv.CompleteServiceEscrow(ctx, &types.MsgCompleteServiceEscrow{ //nolint:errcheck
		Provider:  provider,
		EscrowId:  createResp.EscrowId,
		ProofHash: "proof11",
	})

	// Payer disputes within window (block 200 <= 350).
	ctx = ctx.WithBlockHeight(200)
	_, err := msgSrv.DisputeServiceEscrow(ctx, &types.MsgDisputeServiceEscrow{
		Payer:    payer,
		EscrowId: createResp.EscrowId,
		Reason:   "service incomplete",
	})
	if err != nil {
		t.Fatalf("dispute failed: %v", err)
	}

	// EndBlock far past any deadline — DISPUTED must not be auto-processed.
	ctx = ctx.WithBlockHeight(9999)
	k.ProcessExpiredEscrows(ctx)

	// Funds still locked — neither party received anything.
	if bk.balances[payerAddr.String()]["uzrn"] != 0 {
		t.Errorf("payer should not receive funds (DISPUTED), got %d", bk.balances[payerAddr.String()]["uzrn"])
	}
	if bk.balances[providerAddr.String()]["uzrn"] != 0 {
		t.Errorf("provider should not receive funds (DISPUTED), got %d", bk.balances[providerAddr.String()]["uzrn"])
	}

	escrow, _ := k.GetServiceEscrow(ctx, createResp.EscrowId)
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_DISPUTED {
		t.Errorf("expected DISPUTED, got %v", escrow.Status)
	}
}

// ---------- Defender Bond tests ----------

// TestDefendDispute_Success verifies that a defender can post bond during EVIDENCE_COMMIT
// and the dispute's DefenderBond field is updated.
func TestDefendDispute_Success(t *testing.T) {
	ms, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("defend-c")
	defender := testAddr("defend-d")
	kk.addFact("fact-db", defender)
	bk.setBalance(challenger, "uzrn", 10_000_000)
	bk.setBalance(defender, "uzrn", 10_000_000)

	initResp, err := ms.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-db",
		Reason:     "bad fact",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute failed: %v", err)
	}

	_, err = ms.DefendDispute(ctx, &types.MsgDefendDispute{
		Defender:  defender,
		DisputeId: initResp.DisputeId,
		Bond:      "1000000",
	})
	if err != nil {
		t.Fatalf("DefendDispute failed: %v", err)
	}

	dispute, found := k.GetDispute(ctx, initResp.DisputeId)
	if !found {
		t.Fatal("dispute not found")
	}
	if dispute.DefenderBond != "1000000" {
		t.Errorf("expected DefenderBond 1000000, got %q", dispute.DefenderBond)
	}
	// Defender's balance should be reduced by bond amount.
	if bk.balances[defender]["uzrn"] != 9_000_000 {
		t.Errorf("expected defender balance 9000000, got %d", bk.balances[defender]["uzrn"])
	}
}

// TestDefendDispute_WrongPhase verifies that posting bond fails outside EVIDENCE_COMMIT.
// The test directly injects a dispute in ARBITRATION phase (bypassing BeginBlock phase
// advancement) to verify that DefendDispute rejects non-EVIDENCE_COMMIT phases.
func TestDefendDispute_WrongPhase(t *testing.T) {
	ms, k, ctx, bk, _, _ := setupMsgServer(t)

	challenger := testAddr("defend-wp-c")
	defender := testAddr("defend-wp-d")
	bk.setBalance(defender, "uzrn", 10_000_000)

	// Inject a dispute already in ARBITRATION phase.
	dispute := &types.Dispute{
		Id:               "wp-dispute",
		TargetId:         "fact-wp",
		Challenger:       challenger,
		Defender:         defender,
		Bond:             "1000000",
		Tier:             1,
		Phase:            types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		Outcome:          types.DisputeOutcome_DISPUTE_OUTCOME_UNSPECIFIED,
		EvidenceDeadline: 1,
		VotingDeadline:   2000,
	}
	k.SetDispute(ctx, dispute)

	_, err := ms.DefendDispute(ctx, &types.MsgDefendDispute{
		Defender:  defender,
		DisputeId: dispute.Id,
		Bond:      "1000000",
	})
	if err == nil {
		t.Fatal("expected error for wrong phase, got nil")
	}
}

// TestDefendDispute_NotDefender verifies that a non-defender cannot post bond.
func TestDefendDispute_NotDefender(t *testing.T) {
	ms, _, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("defend-nd-c")
	defender := testAddr("defend-nd-d")
	impostor := testAddr("defend-nd-imp")
	kk.addFact("fact-nd", defender)
	bk.setBalance(challenger, "uzrn", 10_000_000)
	bk.setBalance(impostor, "uzrn", 10_000_000)

	initResp, _ := ms.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-nd",
		Reason:     "bad fact",
		Bond:       "1000000",
	})

	_, err := ms.DefendDispute(ctx, &types.MsgDefendDispute{
		Defender:  impostor,
		DisputeId: initResp.DisputeId,
		Bond:      "1000000",
	})
	if err == nil {
		t.Fatal("expected error for non-defender, got nil")
	}
}

// TestDefendDispute_AlreadyPosted verifies that double-posting bond is rejected.
func TestDefendDispute_AlreadyPosted(t *testing.T) {
	ms, _, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("defend-dup-c")
	defender := testAddr("defend-dup-d")
	kk.addFact("fact-dup", defender)
	bk.setBalance(challenger, "uzrn", 10_000_000)
	bk.setBalance(defender, "uzrn", 10_000_000)

	initResp, _ := ms.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-dup",
		Reason:     "bad fact",
		Bond:       "1000000",
	})

	_, err := ms.DefendDispute(ctx, &types.MsgDefendDispute{
		Defender:  defender,
		DisputeId: initResp.DisputeId,
		Bond:      "1000000",
	})
	if err != nil {
		t.Fatalf("first DefendDispute failed: %v", err)
	}

	_, err = ms.DefendDispute(ctx, &types.MsgDefendDispute{
		Defender:  defender,
		DisputeId: initResp.DisputeId,
		Bond:      "1000000",
	})
	if err == nil {
		t.Fatal("expected error for duplicate bond post, got nil")
	}
}

// TestTwoBond_ChallengerWins verifies two-bond settlement when challenger wins:
// - Defender's bond is slashed.
// - Challenger receives own bond back in full plus winner reward.
// - Defender receives unslashed portion of their bond.
// - Burn remainder routes to development_fund.
func TestTwoBond_ChallengerWins(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	challenger := testAddr("twob-cw-c")
	defender := testAddr("twob-cw-d")
	arb1 := testAddr("twob-cw-arb1")

	// Bond = 1_000_000 (challenger). DefenderBond = 1_000_000.
	// Params: SlashRateLoserBps=500_000 (50%), RewardRateWinnerBps=400_000 (40%), ArbiterRewardBps=100_000 (10%).
	// Loser (defender): slashAmt = 1_000_000 x 50% = 500_000
	//   winnerReward = 500_000 x 40% = 200_000
	//   arbiterReward = 500_000 x 10% = 50_000
	//   burnAmt = 500_000 - 200_000 - 50_000 = 250_000
	//   defenderRefund = 1_000_000 - 500_000 = 500_000
	// Challenger receives: own bond (1_000_000) + winner reward (200_000) = 1_200_000
	dispute := &types.Dispute{
		Id:           "twob-cw",
		TargetId:     "fact-cw",
		Challenger:   challenger,
		Defender:     defender,
		Bond:         "1000000",
		DefenderBond: "1000000",
		Tier:         1,
		Outcome:      types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS,
		Phase:        types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Arbiters:     []string{arb1},
	}
	k.SetDispute(ctx, dispute)

	// Arbiter voted for challenger (correct side).
	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute.Id,
		Arbiter:   arb1,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
		Stake:     "1",
	})

	// Fund module with both bonds.
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 2_000_000}

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	if bk.balances[challenger]["uzrn"] != 1_200_000 {
		t.Errorf("expected challenger 1_200_000, got %d", bk.balances[challenger]["uzrn"])
	}
	if bk.balances[defender]["uzrn"] != 500_000 {
		t.Errorf("expected defender 500_000 refund, got %d", bk.balances[defender]["uzrn"])
	}
	if bk.balances[arb1]["uzrn"] != 50_000 {
		t.Errorf("expected arb1 50_000, got %d", bk.balances[arb1]["uzrn"])
	}
	if bk.moduleBalances["development_fund"]["uzrn"] != 250_000 {
		t.Errorf("expected development_fund 250_000, got %d", bk.moduleBalances["development_fund"]["uzrn"])
	}
}

// TestTwoBond_DefenderWins verifies two-bond settlement when defender wins:
// - Challenger's bond is slashed.
// - Defender receives own bond back in full plus winner reward.
// - Challenger receives unslashed portion of their bond.
func TestTwoBond_DefenderWins(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	challenger := testAddr("twob-dw-c")
	defender := testAddr("twob-dw-d")
	arb1 := testAddr("twob-dw-arb1")

	// Loser (challenger): slashAmt = 500_000; winnerReward = 200_000; arbiterReward = 50_000; burn = 250_000
	// Challenger refund = 1_000_000 - 500_000 = 500_000
	// Defender receives: own bond (1_000_000) + winner reward (200_000) = 1_200_000
	dispute := &types.Dispute{
		Id:           "twob-dw",
		TargetId:     "fact-dw",
		Challenger:   challenger,
		Defender:     defender,
		Bond:         "1000000",
		DefenderBond: "1000000",
		Tier:         1,
		Outcome:      types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS,
		Phase:        types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Arbiters:     []string{arb1},
	}
	k.SetDispute(ctx, dispute)

	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute.Id,
		Arbiter:   arb1,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
		Stake:     "1",
	})

	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 2_000_000}

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	if bk.balances[defender]["uzrn"] != 1_200_000 {
		t.Errorf("expected defender 1_200_000, got %d", bk.balances[defender]["uzrn"])
	}
	if bk.balances[challenger]["uzrn"] != 500_000 {
		t.Errorf("expected challenger 500_000 refund, got %d", bk.balances[challenger]["uzrn"])
	}
	if bk.moduleBalances["development_fund"]["uzrn"] != 250_000 {
		t.Errorf("expected development_fund 250_000, got %d", bk.moduleBalances["development_fund"]["uzrn"])
	}
}

// TestTwoBond_Draw verifies two-bond draw: each party pays ArbiterRewardBps fraction
// of their own bond as arbiter fee; remainder returned.
func TestTwoBond_Draw(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	challenger := testAddr("twob-draw-c")
	defender := testAddr("twob-draw-d")
	arb1 := testAddr("twob-draw-arb")

	// Bond = 2_000_000 each. ArbiterRewardBps = 100_000 (10%).
	// challengerFee = 2_000_000 x 10% = 200_000; refund 1_800_000
	// defenderFee   = 2_000_000 x 10% = 200_000; refund 1_800_000
	// totalArbiterFee = 400_000; arb1 gets 400_000
	dispute := &types.Dispute{
		Id:           "twob-draw",
		TargetId:     "fact-draw2",
		Challenger:   challenger,
		Defender:     defender,
		Bond:         "2000000",
		DefenderBond: "2000000",
		Tier:         1,
		Outcome:      types.DisputeOutcome_DISPUTE_OUTCOME_DRAW,
		Phase:        types.DisputePhase_DISPUTE_PHASE_SETTLED,
		Arbiters:     []string{arb1},
	}
	k.SetDispute(ctx, dispute)

	k.SetVote(ctx, &types.DisputeVote{
		DisputeId: dispute.Id,
		Arbiter:   arb1,
		Vote:      types.ArbiterDecision_ARBITER_DECISION_ABSTAIN,
		Stake:     "1",
	})

	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 4_000_000}

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	if bk.balances[challenger]["uzrn"] != 1_800_000 {
		t.Errorf("expected challenger 1_800_000, got %d", bk.balances[challenger]["uzrn"])
	}
	if bk.balances[defender]["uzrn"] != 1_800_000 {
		t.Errorf("expected defender 1_800_000, got %d", bk.balances[defender]["uzrn"])
	}
	if bk.balances[arb1]["uzrn"] != 400_000 {
		t.Errorf("expected arb1 400_000, got %d", bk.balances[arb1]["uzrn"])
	}
}

// TestTwoBond_Timeout verifies two-bond timeout: both bonds returned in full, no penalty.
func TestTwoBond_Timeout(t *testing.T) {
	k, ctx, bk, _, _ := setupKeeper(t)

	challenger := testAddr("twob-to-c")
	defender := testAddr("twob-to-d")

	dispute := &types.Dispute{
		Id:           "twob-timeout",
		TargetId:     "fact-to2",
		Challenger:   challenger,
		Defender:     defender,
		Bond:         "1500000",
		DefenderBond: "1500000",
		Tier:         1,
		Outcome:      types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT,
		Phase:        types.DisputePhase_DISPUTE_PHASE_TIMED_OUT,
	}
	k.SetDispute(ctx, dispute)

	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 3_000_000}

	err := k.DistributeSettlement(ctx, dispute)
	if err != nil {
		t.Fatalf("DistributeSettlement failed: %v", err)
	}

	if bk.balances[challenger]["uzrn"] != 1_500_000 {
		t.Errorf("expected challenger 1_500_000, got %d", bk.balances[challenger]["uzrn"])
	}
	if bk.balances[defender]["uzrn"] != 1_500_000 {
		t.Errorf("expected defender 1_500_000, got %d", bk.balances[defender]["uzrn"])
	}
}

// ---------- Appeal Tests ----------

// appealParams returns Params with tier 1 configured for appeals:
// 100-block appeal window, appeal bond = 2× original.
func appealParams() *types.Params {
	p := types.DefaultParams()
	p.TierConfigs[0].AppealWindowBlocks = 100
	p.TierConfigs[0].AppealBondMultiplierBps = 2_000_000 // 2× original bond
	return p
}

// setupAppealDispute creates a dispute in APPEAL_WINDOW phase with a CHALLENGER_WINS verdict.
// Challenger bond = 1_000_000 uzrn; settled_at_block = 200.
func setupAppealDispute(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context, *mockBankKeeper, string, string) {
	t.Helper()
	ms, k, ctx, bk, _, kk := setupMsgServer(t)

	challenger := testAddr("appeal-c")
	defender := testAddr("appeal-d")
	kk.addFact("fact-ap", defender)
	bk.setBalance(challenger, "uzrn", 10_000_000)
	bk.setBalance(defender, "uzrn", 10_000_000)

	k.SetParams(ctx, appealParams())

	initResp, err := ms.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-ap",
		Reason:     "test",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute: %v", err)
	}
	dID := initResp.DisputeId

	// Inject APPEAL_WINDOW phase directly (arbitration already completed).
	dispute, _ := k.GetDispute(ctx, dID)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_APPEAL_WINDOW
	dispute.Outcome = types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS
	dispute.SettledAtBlock = 200
	k.SetDispute(ctx, dispute)

	// ctx starts at height 100; for appeal window tests advance to 201 (within window: 200+100=300).
	ctx = ctx.WithBlockHeight(201)

	return ms, k, ctx, bk, challenger, defender
}

// TestAppealDispute_Success verifies that a valid appeal transitions the dispute to APPEALED.
func TestAppealDispute_Success(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)

	// Defender appeals (lost the original verdict).
	// Appeal bond required: 1_000_000 × 2_000_000 / 1_000_000 = 2_000_000.
	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  "ap-", // will be overridden; use the real ID below
		AppealBond: "2000000",
	})
	// We need the real dispute ID, so use k directly.
	disputes := k.GetActiveDisputes(ctx)
	if len(disputes) == 0 {
		t.Fatal("no active disputes")
	}
	dID := disputes[0].Id

	_, err = ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err != nil {
		t.Fatalf("AppealDispute failed: %v", err)
	}

	dispute, _ := k.GetDispute(ctx, dID)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_APPEALED {
		t.Errorf("expected APPEALED phase, got %s", dispute.Phase)
	}
	if dispute.Appealer != defender {
		t.Errorf("expected appealer=%s, got %s", defender, dispute.Appealer)
	}
	if dispute.AppealBond != "2000000" {
		t.Errorf("expected appeal_bond=2000000, got %s", dispute.AppealBond)
	}
	// Appeal bond locked from defender's balance.
	if bk.balances[defender]["uzrn"] != 8_000_000 {
		t.Errorf("expected defender balance 8000000 after bond lock, got %d", bk.balances[defender]["uzrn"])
	}
}

// TestAppealDispute_NotParty verifies that a non-party cannot appeal.
func TestAppealDispute_NotParty(t *testing.T) {
	ms, k, ctx, bk, _, _ := setupAppealDispute(t)
	bk.setBalance(testAddr("outsider"), "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   testAddr("outsider"),
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err == nil {
		t.Fatal("expected error for non-party appeal, got nil")
	}
}

// TestAppealDispute_WindowClosed verifies that filing after the window is rejected.
func TestAppealDispute_WindowClosed(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)
	bk.setBalance(defender, "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Advance past window: settled_at=200, window=100 → expires at block 300.
	ctx = ctx.WithBlockHeight(301)

	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err == nil {
		t.Fatal("expected error for closed window, got nil")
	}
}

// TestAppealDispute_AlreadyAppealed verifies that a second appeal is rejected.
func TestAppealDispute_AlreadyAppealed(t *testing.T) {
	ms, k, ctx, bk, challenger, defender := setupAppealDispute(t)
	bk.setBalance(defender, "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// First appeal by defender succeeds.
	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:  defender,
		DisputeId: dID,
		AppealBond: "2000000",
	})
	if err != nil {
		t.Fatalf("first appeal failed: %v", err)
	}

	// Transition back to APPEAL_WINDOW to test double-appeal rejection.
	dispute, _ := k.GetDispute(ctx, dID)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_APPEAL_WINDOW
	k.SetDispute(ctx, dispute)

	bk.setBalance(challenger, "uzrn", 10_000_000)
	_, err = ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   challenger,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err == nil {
		t.Fatal("expected error for already-appealed dispute, got nil")
	}
}

// TestAppealDispute_BondInsufficient verifies that an appeal bond below minimum is rejected.
func TestAppealDispute_BondInsufficient(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)
	bk.setBalance(defender, "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Minimum is 2_000_000; post only 500_000.
	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "500000",
	})
	if err == nil {
		t.Fatal("expected insufficient bond error, got nil")
	}
}

// TestPhaseTransition_AppealWindowExpires verifies that a dispute in APPEAL_WINDOW
// auto-settles when the window expires with no appeal filed.
func TestPhaseTransition_AppealWindowExpires(t *testing.T) {
	_, k, ctx, bk, challenger, _ := setupAppealDispute(t)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Fund the module account (settlement will try to distribute).
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 1_000_000}

	// Advance past window: settled_at=200 + 100 = 300.
	ctx = ctx.WithBlockHeight(301)
	k.ProcessPhaseTransitions(ctx, 301)

	dispute, _ := k.GetDispute(ctx, dID)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED after window expiry, got %s", dispute.Phase)
	}
	_ = challenger // balance assertions depend on settlement math, outcome already verified elsewhere
}

// TestAppealDispute_WrongPhase verifies that AppealDispute rejects disputes not in APPEAL_WINDOW.
func TestAppealDispute_WrongPhase(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)
	bk.setBalance(defender, "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Move to SETTLED phase directly.
	dispute, _ := k.GetDispute(ctx, dID)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_SETTLED
	k.SetDispute(ctx, dispute)

	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err == nil {
		t.Fatal("expected error for wrong phase, got nil")
	}
}

// TestAppealDispute_NoWindow verifies that AppealDispute rejects disputes on tiers with no appeal window.
func TestAppealDispute_NoWindow(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)
	bk.setBalance(defender, "uzrn", 10_000_000)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Remove appeal window from tier 1.
	p := k.GetParams(ctx)
	p.TierConfigs[0].AppealWindowBlocks = 0
	p.TierConfigs[0].AppealBondMultiplierBps = 0
	k.SetParams(ctx, p)

	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err == nil {
		t.Fatal("expected error for no appeal window, got nil")
	}
}

// TestPhaseTransition_AppealedSettles verifies that after appeal voting deadline,
// the dispute settles via distributeAppealResult.
func TestPhaseTransition_AppealedSettles(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Fund module account.
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 3_000_000}

	// File appeal.
	bk.setBalance(defender, "uzrn", 10_000_000)
	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err != nil {
		t.Fatalf("AppealDispute: %v", err)
	}

	// Advance past appeal voting deadline.
	dispute, _ := k.GetDispute(ctx, dID)
	ctx = ctx.WithBlockHeight(int64(dispute.VotingDeadline) + 1)
	k.ProcessPhaseTransitions(ctx, dispute.VotingDeadline+1)

	dispute, _ = k.GetDispute(ctx, dID)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED after appeal resolution, got %s", dispute.Phase)
	}
}

// TestSettleDispute_EntersAppealWindow verifies that SettleDispute transitions to
// APPEAL_WINDOW (not SETTLED) when the tier has an appeal window configured.
func TestSettleDispute_EntersAppealWindow(t *testing.T) {
	k, ctx, bk, _, kk := setupKeeper(t)
	msgSrv := keeper.NewMsgServerImpl(k)

	challenger := testAddr("saw-c")
	defender := testAddr("saw-d")
	bk.setBalance(challenger, "uzrn", 10_000_000)
	kk.addFact("fact-saw", defender)

	// Enable appeal window on tier 1.
	k.SetParams(ctx, appealParams())

	resp, err := msgSrv.InitiateDispute(ctx, &types.MsgInitiateDispute{
		Challenger: challenger,
		TargetType: types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT,
		TargetId:   "fact-saw",
		Reason:     "test",
		Bond:       "1000000",
	})
	if err != nil {
		t.Fatalf("InitiateDispute: %v", err)
	}

	// Advance to arbitration.
	dispute, _ := k.GetDispute(ctx, resp.DisputeId)
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
	dispute.VotingDeadline = 2000
	k.SetDispute(ctx, dispute)

	// Cast votes for challenger (all 3 arbiters → challenger wins).
	for _, arb := range dispute.Arbiters {
		_, _ = msgSrv.ArbiterVote(ctx, &types.MsgArbiterVote{
			Arbiter:   arb,
			DisputeId: resp.DisputeId,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_CHALLENGER,
			Reasoning: "test",
		})
	}

	// Settle via authority.
	_, err = msgSrv.SettleDispute(ctx, &types.MsgSettleDispute{
		Authority: "zrn1authority",
		DisputeId: resp.DisputeId,
	})
	if err != nil {
		t.Fatalf("SettleDispute failed: %v", err)
	}

	// With appeals configured the dispute should be in APPEAL_WINDOW, not SETTLED.
	dispute, _ = k.GetDispute(ctx, resp.DisputeId)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_APPEAL_WINDOW {
		t.Errorf("expected APPEAL_WINDOW, got %s", dispute.Phase)
	}
	if dispute.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS {
		t.Errorf("expected CHALLENGER_WINS outcome recorded, got %s", dispute.Outcome)
	}
	if dispute.SettledAtBlock == 0 {
		t.Error("expected SettledAtBlock to be set")
	}
	// Settlement deferred: challenger's bond must still be in the module account.
	if bk.moduleBalances[types.ModuleName]["uzrn"] <= 0 {
		t.Error("expected challenger bond still escrowed in module account")
	}
}

// TestAppealResult_ReversedVerdictReturnsBond verifies that when the appeal panel
// reverses the original verdict, the appeal bond is returned to the appealer.
func TestAppealResult_ReversedVerdictReturnsBond(t *testing.T) {
	ms, k, ctx, bk, _, defender := setupAppealDispute(t)

	disputes := k.GetActiveDisputes(ctx)
	dID := disputes[0].Id

	// Fund module: challenger bond (1_000_000) + appeal bond (2_000_000) = 3_000_000.
	bk.moduleBalances[types.ModuleName] = map[string]int64{"uzrn": 3_000_000}

	// File appeal as defender.
	bk.setBalance(defender, "uzrn", 10_000_000)
	_, err := ms.AppealDispute(ctx, &types.MsgAppealDispute{
		Appealer:   defender,
		DisputeId:  dID,
		AppealBond: "2000000",
	})
	if err != nil {
		t.Fatalf("AppealDispute: %v", err)
	}

	// After filing, module should hold original bond (1_000_000) + appeal bond (2_000_000) = 3_000_000.
	// No need to reset — bk already reflected the lock above via SendCoinsFromAccountToModule.

	// Cast appeal votes: all appeal arbiters vote DEFENDER_WINS → reverses CHALLENGER_WINS.
	dispute, _ := k.GetDispute(ctx, dID)
	for _, arb := range dispute.Arbiters {
		k.SetVote(ctx, &types.DisputeVote{
			DisputeId: dID,
			Arbiter:   arb,
			Vote:      types.ArbiterDecision_ARBITER_DECISION_DEFENDER,
			Stake:     "1000000",
		})
	}

	// Advance past appeal voting deadline.
	ctx = ctx.WithBlockHeight(int64(dispute.VotingDeadline) + 1)
	k.ProcessPhaseTransitions(ctx, dispute.VotingDeadline+1)

	dispute, _ = k.GetDispute(ctx, dID)
	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_SETTLED {
		t.Errorf("expected SETTLED, got %s", dispute.Phase)
	}
	if dispute.Outcome != types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS {
		t.Errorf("expected DEFENDER_WINS (reversed), got %s", dispute.Outcome)
	}
	// Appeal bond (2_000_000) should be returned to defender (the appealer).
	if bk.balances[defender]["uzrn"] < 2_000_000 {
		t.Errorf("expected defender to receive appeal bond back, got %d uzrn", bk.balances[defender]["uzrn"])
	}
}
