package keeper_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
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
	"github.com/stretchr/testify/require"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/zerone-chain/zerone/x/partnerships/keeper"
	"github.com/zerone-chain/zerone/x/partnerships/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")

	// Must compute addresses AFTER bech32 prefix is set.
	humanAddr = testAddr("human")
	agentAddr = testAddr("agent")
	outsiderAddr = testAddr("outsider")
	agent2Addr = testAddr("agent2")
	agent3Addr = testAddr("agent3")
	authority = testAddr("authority")
}

// ---------- Test Address Helpers ----------

func testAddr(name string) string {
	h := sha256.Sum256([]byte("test_seed:" + name))
	return sdk.AccAddress(h[:20]).String()
}

var (
	humanAddr    string
	agentAddr    string
	outsiderAddr string
	agent2Addr   string
	agent3Addr   string
	authority    string
)

// ---------- Mock BankKeeper ----------

type mockBankKeeper struct {
	balances       map[string]map[string]int64
	moduleBalances map[string]map[string]int64
	modToModCalled bool
	modToModAmount int64
	burnCalled     bool
	burnAmount     int64
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

func (m *mockBankKeeper) setModuleBalance(module, denom string, amount int64) {
	if m.moduleBalances[module] == nil {
		m.moduleBalances[module] = make(map[string]int64)
	}
	m.moduleBalances[module][denom] = amount
}

func (m *mockBankKeeper) SendCoins(_ context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error {
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

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule string, recipientModule string, amt sdk.Coins) error {
	m.modToModCalled = true
	for _, coin := range amt {
		m.modToModAmount += coin.Amount.Int64()
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

func (m *mockBankKeeper) BurnCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	m.burnCalled = true
	for _, coin := range amt {
		m.burnAmount += coin.Amount.Int64()
		if m.moduleBalances[moduleName] == nil {
			m.moduleBalances[moduleName] = make(map[string]int64)
		}
		m.moduleBalances[moduleName][coin.Denom] -= coin.Amount.Int64()
	}
	return nil
}

// ---------- Mock HomeKeeper ----------

type mockHomeKeeper struct {
	homes                map[string][]string
	partnershipOnHomeSet map[string]string
}

func newMockHomeKeeper() *mockHomeKeeper {
	return &mockHomeKeeper{
		homes:                make(map[string][]string),
		partnershipOnHomeSet: make(map[string]string),
	}
}

func (m *mockHomeKeeper) GetHomesByOwner(_ context.Context, owner string) []string {
	return m.homes[owner]
}

func (m *mockHomeKeeper) SetPartnershipOnHome(_ context.Context, homeID, partnershipID string) {
	m.partnershipOnHomeSet[homeID] = partnershipID
}

// ---------- Test Setup ----------

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

	mockBK := newMockBankKeeper()

	storeService := runtime.NewKVStoreService(storeKey)
	k := keeper.NewKeeper(cdc, storeService, mockBK, authority)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100, ChainID: "zerone-test-1"}, false, log.NewNopLogger())

	// Initialize default params
	k.SetParams(ctx, types.DefaultParams())

	return k, ctx, mockBK
}

func setupMsgServer(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context, *mockBankKeeper) {
	t.Helper()
	k, ctx, bk := setupKeeper(t)
	return keeper.NewMsgServerImpl(k), k, ctx, bk
}

// ctxAtHeight returns a new context at the given block height.
func ctxAtHeight(ctx sdk.Context, height int64) sdk.Context {
	return ctx.WithBlockHeight(height)
}

// ---------- Helper: propose + accept a partnership ----------

func proposeAndAccept(t *testing.T, ms types.MsgServer, ctx sdk.Context, human, agent string) (string, sdk.Context) {
	t.Helper()
	resp, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       human,
		Partner:        agent,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err != nil {
		t.Fatalf("ProposePartnership failed: %v", err)
	}

	_, err = ms.AcceptPartnership(ctx, &types.MsgAcceptPartnership{
		Accepter:      agent,
		PartnershipId: resp.PartnershipId,
		Deposit:       "1000000",
	})
	if err != nil {
		t.Fatalf("AcceptPartnership failed: %v", err)
	}

	return resp.PartnershipId, ctx
}

// ==================== PARAMS TESTS ====================

func TestDefaultParams(t *testing.T) {
	params := types.DefaultParams()
	if params.FormationWindowBlocks != 1000 {
		t.Errorf("expected formation window 1000, got %d", params.FormationWindowBlocks)
	}
	if params.CoolingPeriodBlocks != 5000 {
		t.Errorf("expected cooling period 5000, got %d", params.CoolingPeriodBlocks)
	}
	if params.CommonPotShareBps != 100000 {
		t.Errorf("expected common pot share 100000, got %d", params.CommonPotShareBps)
	}
	if params.DefaultHumanSplitBps+params.DefaultAgentSplitBps != 1000000 {
		t.Errorf("splits should sum to 1000000, got %d", params.DefaultHumanSplitBps+params.DefaultAgentSplitBps)
	}
	if params.MaxCounterProposalDepth != 3 {
		t.Errorf("expected max counter depth 3, got %d", params.MaxCounterProposalDepth)
	}
}

func TestSetGetParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	custom := &types.Params{
		FormationWindowBlocks:      2000,
		CoolingPeriodBlocks:        8000,
		CommonPotShareBps:          200000,
		SafetyFreezeDurationBlocks: 1000,
		MaxFreezesPerEpoch:         5,
		CoercionReviewBlocks:       3000,
		BaseCooldownBlocks:         200,
		MaxCounterProposalDepth:    5,
		DefaultHumanSplitBps:       600000,
		DefaultAgentSplitBps:       400000,
		MinPartnershipStake:        "2000000",
		SeedPartnershipDuration:    20000,
		SeedCommonPotCap:           "200000000",
	}
	k.SetParams(ctx, custom)

	got := k.GetParams(ctx)
	if got.FormationWindowBlocks != 2000 {
		t.Errorf("expected 2000, got %d", got.FormationWindowBlocks)
	}
	if got.DefaultHumanSplitBps != 600000 {
		t.Errorf("expected 600000, got %d", got.DefaultHumanSplitBps)
	}
}

// ==================== FULL LIFECYCLE TESTS ====================

func TestLifecycle_ProposeAcceptDissolve(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)
	bk.setModuleBalance(types.ModuleName, "uzrn", 10000000)

	// Step 1: Propose
	resp, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err != nil {
		t.Fatalf("ProposePartnership failed: %v", err)
	}
	pid := resp.PartnershipId

	p, found := k.GetPartnership(ctx, pid)
	if !found {
		t.Fatal("partnership not found after propose")
	}
	if p.Status != types.StatusPending {
		t.Errorf("expected pending, got %s", p.Status)
	}
	if p.HumanAddr != humanAddr {
		t.Errorf("wrong human addr: %s", p.HumanAddr)
	}
	if p.AgentAddr != agentAddr {
		t.Errorf("wrong agent addr: %s", p.AgentAddr)
	}
	if p.CooperationScore != 500000 {
		t.Errorf("expected cooperation score 500000, got %d", p.CooperationScore)
	}

	// Step 2: Accept
	_, err = ms.AcceptPartnership(ctx, &types.MsgAcceptPartnership{
		Accepter:      agentAddr,
		PartnershipId: pid,
		Deposit:       "1000000",
	})
	if err != nil {
		t.Fatalf("AcceptPartnership failed: %v", err)
	}

	p, _ = k.GetPartnership(ctx, pid)
	if p.Status != types.StatusActive {
		t.Errorf("expected active after accept, got %s", p.Status)
	}
	if p.CommonPotBalance != "2000000" {
		t.Errorf("expected pot 2000000 after both deposits, got %s", p.CommonPotBalance)
	}

	// Step 3: Dissolve (must wait for lock to expire)
	// Lock tier 0 = 22222 blocks, current is 100.
	// Move to a block past lock expiry.
	futureCtx := ctxAtHeight(ctx, 100+22223)

	dissResp, err := ms.InitiateDissolution(futureCtx, &types.MsgInitiateDissolution{
		Initiator:     humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("InitiateDissolution failed: %v", err)
	}

	p, _ = k.GetPartnership(futureCtx, pid)
	if p.Status != types.StatusCooling {
		t.Errorf("expected cooling, got %s", p.Status)
	}
	if p.ExitState == nil {
		t.Fatal("exit state should be set")
	}
	if p.ExitState.InitiatedBy != humanAddr {
		t.Errorf("wrong initiator: %s", p.ExitState.InitiatedBy)
	}

	// Step 4: Settle cooling (simulate BeginBlocker at cooldown end)
	settleCtx := ctxAtHeight(ctx, int64(dissResp.CooldownEnd+1))
	k.SettleCoolingPartnerships(settleCtx)

	p, _ = k.GetPartnership(settleCtx, pid)
	if p.Status != types.StatusDissolved {
		t.Errorf("expected dissolved after cooldown, got %s", p.Status)
	}
}

func TestLifecycle_ProposerCannotAcceptOwn(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	resp, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err != nil {
		t.Fatalf("ProposePartnership failed: %v", err)
	}

	// Human (proposer) tries to accept — should fail
	_, err = ms.AcceptPartnership(ctx, &types.MsgAcceptPartnership{
		Accepter:      humanAddr,
		PartnershipId: resp.PartnershipId,
	})
	if err == nil {
		t.Fatal("expected error when proposer tries to accept")
	}
}

func TestLifecycle_FormationExpiry(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	resp, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err != nil {
		t.Fatalf("ProposePartnership failed: %v", err)
	}

	// Expire the formation (default window = 1000 blocks)
	expiredCtx := ctxAtHeight(ctx, 100+1001)
	k.ExpireFormations(expiredCtx)

	p, _ := k.GetPartnership(expiredCtx, resp.PartnershipId)
	if p.Status != types.StatusDissolved {
		t.Errorf("expected dissolved after formation expiry, got %s", p.Status)
	}

	// Agent tries to accept after expiry — should fail
	_, err = ms.AcceptPartnership(expiredCtx, &types.MsgAcceptPartnership{
		Accepter:      agentAddr,
		PartnershipId: resp.PartnershipId,
	})
	if err == nil {
		t.Fatal("expected error when accepting expired formation")
	}
}

func TestLifecycle_DuplicatePartnershipBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 20000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Try to propose again with same pair
	_, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err == nil {
		t.Fatal("expected error for duplicate partnership")
	}
}

func TestLifecycle_LockNotExpiredBlocksDissolution(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Try to dissolve while lock is active (tier 0 = 22222 blocks from block 100)
	_, err := ms.InitiateDissolution(ctx, &types.MsgInitiateDissolution{
		Initiator:     humanAddr,
		PartnershipId: pid,
	})
	if err == nil {
		t.Fatal("expected error when lock has not expired")
	}
}

func TestLifecycle_MinStakeEnforced(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	// Default min stake is 1000000; try depositing less
	_, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "100",
	})
	if err == nil {
		t.Fatal("expected error for insufficient deposit")
	}
}

func TestLifecycle_HomeAutoLink(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	mockHK := newMockHomeKeeper()
	mockHK.homes[agentAddr] = []string{"home-1"}
	k.SetHomeKeeper(mockHK)

	// Create msg server AFTER setting the home keeper on the pointer
	ms := keeper.NewMsgServerImpl(k)

	resp, err := ms.ProposePartnership(ctx, &types.MsgProposePartnership{
		Proposer:       humanAddr,
		Partner:        agentAddr,
		ProposedTier:   0,
		InitialDeposit: "1000000",
	})
	if err != nil {
		t.Fatalf("ProposePartnership failed: %v", err)
	}

	_, err = ms.AcceptPartnership(ctx, &types.MsgAcceptPartnership{
		Accepter:      agentAddr,
		PartnershipId: resp.PartnershipId,
		Deposit:       "1000000",
	})
	if err != nil {
		t.Fatalf("AcceptPartnership failed: %v", err)
	}

	if mockHK.partnershipOnHomeSet["home-1"] != resp.PartnershipId {
		t.Errorf("expected home auto-linked, got %s", mockHK.partnershipOnHomeSet["home-1"])
	}
}

// ==================== CONSENSUS OPERATION TESTS ====================

func TestConsensusOp_ProposeAndApprove(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)
	bk.setModuleBalance(types.ModuleName, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Propose a withdraw operation (micro tier: <1% of pot)
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "10000", // 10000 / 2000000 = 0.5% → micro tier
		Rationale:     "need funds",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	op, found := k.GetConsensusOperation(ctx, opResp.OperationId)
	if !found {
		t.Fatal("operation not found")
	}
	if op.Status != types.OpStatusPending {
		t.Errorf("expected pending, got %s", op.Status)
	}
	if op.Deliberation.AmountTier != types.AmountTierMicro {
		t.Errorf("expected micro tier, got %s", op.Deliberation.AmountTier)
	}

	// micro tier: window=22 blocks, floor=11 blocks
	// Vote after floor period (block 100 + 11 = 111)
	voteCtx := ctxAtHeight(ctx, 112)

	_, err = ms.VoteConsensusOp(voteCtx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     true,
	})
	if err != nil {
		t.Fatalf("VoteConsensusOp approve failed: %v", err)
	}

	op, _ = k.GetConsensusOperation(voteCtx, opResp.OperationId)
	if op.Status != types.OpStatusApproved {
		t.Errorf("expected approved, got %s", op.Status)
	}
}

func TestConsensusOp_RejectWithCooldown(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Reject (no floor constraint for rejection)
	_, err = ms.VoteConsensusOp(ctx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     false,
	})
	if err != nil {
		t.Fatalf("VoteConsensusOp reject failed: %v", err)
	}

	op, _ := k.GetConsensusOperation(ctx, opResp.OperationId)
	if op.Status != types.OpStatusRejected {
		t.Errorf("expected rejected, got %s", op.Status)
	}

	// Now there should be a cooldown active
	rc, found := k.GetRejectionCooldown(ctx, pid)
	if !found {
		t.Fatal("rejection cooldown not found")
	}
	if rc.RejectionCount != 1 {
		t.Errorf("expected rejection count 1, got %d", rc.RejectionCount)
	}
	// Base cooldown = 100, 2^1 = 200
	expectedEnd := uint64(100) + 200
	if rc.CooldownEndsAt != expectedEnd {
		t.Errorf("expected cooldown ends at %d, got %d", expectedEnd, rc.CooldownEndsAt)
	}

	// Try to propose during cooldown — should fail
	_, err = ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "500",
		Rationale:     "again",
	})
	if err == nil {
		t.Fatal("expected error during cooldown")
	}
}

func TestConsensusOp_SelfVotePrevented(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Proposer tries to vote on their own op
	_, err = ms.VoteConsensusOp(ctx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       humanAddr,
		Approve:     true,
	})
	if err == nil {
		t.Fatal("expected error for self-vote")
	}
}

func TestConsensusOp_NonParticipantBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Outsider tries to propose
	_, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      outsiderAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err == nil {
		t.Fatal("expected error for non-participant proposer")
	}
}

func TestConsensusOp_FrozenPartnershipBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Freeze the partnership
	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	// Try to propose op while frozen — should fail
	_, err = ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      agentAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err == nil {
		t.Fatal("expected error when partnership is frozen")
	}
}

func TestConsensusOp_WithdrawExecutes(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)
	bk.setModuleBalance(types.ModuleName, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Propose withdraw of 500000
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "500000",
		Rationale:     "need some funds",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// 500000 / 2000000 = 25% → large tier (window=777, floor=388)
	// Approve after floor period (block 100 + 388 = 488)
	voteCtx := ctxAtHeight(ctx, 500)
	_, err = ms.VoteConsensusOp(voteCtx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     true,
	})
	if err != nil {
		t.Fatalf("VoteConsensusOp failed: %v", err)
	}

	p, _ := k.GetPartnership(voteCtx, pid)
	potBal := new(big.Int)
	potBal.SetString(p.CommonPotBalance, 10)
	expected := new(big.Int)
	expected.SetString("1500000", 10) // 2000000 - 500000
	if potBal.Cmp(expected) != 0 {
		t.Errorf("expected pot balance 1500000, got %s", p.CommonPotBalance)
	}
}

func TestConsensusOp_ExpiredWindowBlocks(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Vote well past the window (micro = 22 blocks from block 100)
	lateCtx := ctxAtHeight(ctx, 200)
	_, err = ms.VoteConsensusOp(lateCtx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     true,
	})
	if err == nil {
		t.Fatal("expected error when voting past window")
	}
}

// ==================== DELIBERATION TIMING TESTS ====================

func TestDeliberation_FloorPeriodEnforced(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000", // micro: window=22, floor=11
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Try to approve during floor period (at block 105, floor ends at 111)
	earlyCtx := ctxAtHeight(ctx, 105)
	_, err = ms.VoteConsensusOp(earlyCtx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     true,
	})
	if err == nil {
		t.Fatal("expected error when voting during floor period")
	}
}

func TestDeliberation_AmountTiers(t *testing.T) {
	k, _, _ := setupKeeper(t)

	tests := []struct {
		amount      string
		potBalance  string
		expectedTier string
	}{
		{"5000", "1000000", types.AmountTierMicro},    // 0.5%
		{"20000", "1000000", types.AmountTierSmall},    // 2%
		{"100000", "1000000", types.AmountTierMedium},  // 10%
		{"300000", "1000000", types.AmountTierLarge},    // 30%
		{"600000", "1000000", types.AmountTierMajor},    // 60%
	}

	for _, tt := range tests {
		got := k.CalculateAmountTier(tt.amount, tt.potBalance)
		if got != tt.expectedTier {
			t.Errorf("CalculateAmountTier(%s, %s) = %s, want %s",
				tt.amount, tt.potBalance, got, tt.expectedTier)
		}
	}
}

func TestDeliberation_WindowDurations(t *testing.T) {
	k, _, _ := setupKeeper(t)

	expected := map[string]uint64{
		types.AmountTierMicro:  22,
		types.AmountTierSmall:  77,
		types.AmountTierMedium: 222,
		types.AmountTierLarge:  777,
		types.AmountTierMajor:  2222,
	}

	for tier, want := range expected {
		got := k.GetDeliberationWindow(tier)
		if got != want {
			t.Errorf("GetDeliberationWindow(%s) = %d, want %d", tier, got, want)
		}
	}
}

func TestDeliberation_LargeAmountGetsLongerFloor(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Propose a large withdraw: 600000 / 2000000 = 30% → large tier (window=777, floor=388)
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "600000",
		Rationale:     "big expense",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	op, _ := k.GetConsensusOperation(ctx, opResp.OperationId)
	if op.Deliberation.AmountTier != types.AmountTierLarge {
		t.Errorf("expected large tier, got %s", op.Deliberation.AmountTier)
	}
	// Floor = 100 + 777/2 = 100 + 388 = 488
	if op.Deliberation.FloorEndsAt != 488 {
		t.Errorf("expected floor ends at 488, got %d", op.Deliberation.FloorEndsAt)
	}
	// Window = 100 + 777 = 877
	if op.Deliberation.WindowEndsAt != 877 {
		t.Errorf("expected window ends at 877, got %d", op.Deliberation.WindowEndsAt)
	}
}

// ==================== COUNTER-PROPOSAL DEPTH TESTS ====================

func TestCounterProposal_BasicChain(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Propose op
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "100000",
		Rationale:     "original",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Reject with counter-proposal
	voteResp, err := ms.VoteConsensusOp(ctx, &types.MsgVoteConsensusOp{
		OperationId:  opResp.OperationId,
		Voter:        agentAddr,
		Approve:      false,
		CounterAmount: "50000",
		Rationale:    "too much, how about this",
	})
	if err != nil {
		t.Fatalf("VoteConsensusOp reject+counter failed: %v", err)
	}

	if voteResp.CounterOperationId == "" {
		t.Fatal("expected counter-proposal operation ID")
	}

	counterOp, found := k.GetConsensusOperation(ctx, voteResp.CounterOperationId)
	if !found {
		t.Fatal("counter-operation not found")
	}
	if counterOp.Deliberation.CounterProposalOf != opResp.OperationId {
		t.Errorf("counter should reference original, got %s", counterOp.Deliberation.CounterProposalOf)
	}
	if counterOp.Deliberation.ChainDepth != 1 {
		t.Errorf("expected chain depth 1, got %d", counterOp.Deliberation.ChainDepth)
	}
	if counterOp.Amount != "50000" {
		t.Errorf("expected counter amount 50000, got %s", counterOp.Amount)
	}
}

func TestCounterProposal_DepthLimitEnforced(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// MaxCounterProposalDepth = 3, so build a chain of depth 3
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "100000",
		Rationale:     "depth 0",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	currentOpId := opResp.OperationId
	voters := []string{agentAddr, humanAddr, agentAddr}

	for i := 0; i < 3; i++ {
		// Clear cooldown for next proposal
		k.DeleteRejectionCooldown(ctx, pid)

		voteResp, err := ms.VoteConsensusOp(ctx, &types.MsgVoteConsensusOp{
			OperationId:  currentOpId,
			Voter:        voters[i],
			Approve:      false,
			CounterAmount: "50000",
			Rationale:    "counter",
		})
		if err != nil {
			t.Fatalf("counter %d failed: %v", i+1, err)
		}
		if i < 2 {
			// Should succeed for depth 1 and 2
			if voteResp.CounterOperationId == "" {
				t.Fatalf("counter %d: expected counter op ID", i+1)
			}
			currentOpId = voteResp.CounterOperationId
		} else {
			// Depth 3: chain depth of parent is 2, which equals MaxCounterProposalDepth-1=2...
			// Actually, the check is: parentOp.Deliberation.ChainDepth >= params.MaxCounterProposalDepth
			// At depth 2 counter, parent has ChainDepth=2, MaxCounterProposalDepth=3
			// 2 < 3, so depth 2 counter should still succeed
			if voteResp.CounterOperationId == "" {
				t.Fatalf("counter %d: expected counter op ID", i+1)
			}
			currentOpId = voteResp.CounterOperationId
		}
	}

	// One more rejection with counter should fail (depth 3 >= max 3)
	k.DeleteRejectionCooldown(ctx, pid)
	voteResp, err := ms.VoteConsensusOp(ctx, &types.MsgVoteConsensusOp{
		OperationId:  currentOpId,
		Voter:        humanAddr,
		Approve:      false,
		CounterAmount: "25000",
		Rationale:    "one more",
	})
	if err != nil {
		t.Fatalf("final rejection failed: %v", err)
	}
	// Counter should NOT have been created because depth limit reached
	if voteResp.CounterOperationId != "" {
		t.Errorf("expected no counter op at max depth, got %s", voteResp.CounterOperationId)
	}
}

// ==================== SAFETY FREEZE TESTS ====================

func TestSafetyFreeze_Basic(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	resp, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	// Default freeze duration = 500 blocks
	expectedExpiry := uint64(100 + 500)
	if resp.ExpiresAt != expectedExpiry {
		t.Errorf("expected expires at %d, got %d", expectedExpiry, resp.ExpiresAt)
	}

	p, _ := k.GetPartnership(ctx, pid)
	if p.Status != types.StatusSuspended {
		t.Errorf("expected suspended after freeze, got %s", p.Status)
	}

	sf, found := k.GetSafetyFreeze(ctx, pid)
	if !found {
		t.Fatal("safety freeze not found")
	}
	if sf.FreezeCountThisEpoch != 1 {
		t.Errorf("expected freeze count 1, got %d", sf.FreezeCountThisEpoch)
	}
}

func TestSafetyFreeze_BlocksWithdrawal(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Freeze
	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	// Try to propose consensus op — should fail because frozen
	_, err = ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      agentAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "need cash",
	})
	if err == nil {
		t.Fatal("expected error: cannot propose ops while frozen")
	}
}

func TestSafetyFreeze_MaxPerEpoch(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Freeze 1
	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("freeze 1 failed: %v", err)
	}

	// Let freeze expire, then lift and reactivate
	expiredCtx := ctxAtHeight(ctx, 700)
	k.LiftExpiredFreezes(expiredCtx)

	// Freeze 2
	_, err = ms.SafetyFreeze(expiredCtx, &types.MsgSafetyFreeze{
		Freezer:       agentAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("freeze 2 failed: %v", err)
	}

	// Let it expire again
	expired2Ctx := ctxAtHeight(ctx, 1300)
	k.LiftExpiredFreezes(expired2Ctx)

	// Freeze 3
	_, err = ms.SafetyFreeze(expired2Ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("freeze 3 failed: %v", err)
	}

	// Let it expire
	expired3Ctx := ctxAtHeight(ctx, 1900)
	k.LiftExpiredFreezes(expired3Ctx)

	// Freeze 4 — should fail (max 3 per epoch)
	_, err = ms.SafetyFreeze(expired3Ctx, &types.MsgSafetyFreeze{
		Freezer:       agentAddr,
		PartnershipId: pid,
	})
	if err == nil {
		t.Fatal("expected error: max freezes per epoch reached")
	}
}

func TestSafetyFreeze_ReducesCooperationScore(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	p, _ := k.GetPartnership(ctx, pid)
	scoreBefore := p.CooperationScore

	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	p, _ = k.GetPartnership(ctx, pid)
	// First freeze: penalty = 100 * 1 = 100
	expectedScore := scoreBefore - 100
	if p.CooperationScore != expectedScore {
		t.Errorf("expected cooperation score %d, got %d", expectedScore, p.CooperationScore)
	}
}

func TestSafetyFreeze_LiftRestoresActive(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	// Let it expire and lift
	liftCtx := ctxAtHeight(ctx, 700)
	k.LiftExpiredFreezes(liftCtx)

	p, _ := k.GetPartnership(liftCtx, pid)
	if p.Status != types.StatusActive {
		t.Errorf("expected active after freeze lift, got %s", p.Status)
	}
}

func TestSafetyFreeze_NonParticipantBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       outsiderAddr,
		PartnershipId: pid,
	})
	if err == nil {
		t.Fatal("expected error for non-participant freeze")
	}
}

// ==================== COERCION SIGNAL TESTS ====================

func TestCoercionSignal_Basic(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	resp, err := ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("RaiseCoercionSignal failed: %v", err)
	}
	if resp.SignalId == "" {
		t.Fatal("expected non-empty signal ID")
	}

	p, _ := k.GetPartnership(ctx, pid)
	if p.Status != types.StatusSuspended {
		t.Errorf("expected suspended after coercion signal, got %s", p.Status)
	}
}

func TestCoercionSignal_BlocksConsensusOps(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	_, err := ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("RaiseCoercionSignal failed: %v", err)
	}

	// Partnership is suspended — cannot propose ops
	_, err = ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      agentAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err == nil {
		t.Fatal("expected error: cannot propose ops while coercion signal active")
	}
}

func TestCoercionSignal_ExpiryRestoresActive(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	_, err := ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("RaiseCoercionSignal failed: %v", err)
	}

	// CoercionReviewBlocks default = 2000
	expiredCtx := ctxAtHeight(ctx, 100+2001)
	k.ExpireCoercionSignals(expiredCtx)

	p, _ := k.GetPartnership(expiredCtx, pid)
	if p.Status != types.StatusActive {
		t.Errorf("expected active after coercion signal expiry, got %s", p.Status)
	}
}

func TestCoercionSignal_DuplicateBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	_, err := ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("first coercion signal failed: %v", err)
	}

	// Raising a second signal while the first is active
	_, err = ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        agentAddr,
		PartnershipId: pid,
	})
	if err == nil {
		t.Fatal("expected error for duplicate coercion signal")
	}
}

func TestCoercionSignal_FreezePlusCoercionKeepsSuspended(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Freeze first
	_, err := ms.SafetyFreeze(ctx, &types.MsgSafetyFreeze{
		Freezer:       humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("SafetyFreeze failed: %v", err)
	}

	// Then coercion signal
	_, err = ms.RaiseCoercionSignal(ctx, &types.MsgRaiseCoercionSignal{
		Raiser:        agentAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("RaiseCoercionSignal failed: %v", err)
	}

	// Lift freeze — should stay suspended because coercion is still active
	liftCtx := ctxAtHeight(ctx, 700)
	k.LiftExpiredFreezes(liftCtx)

	p, _ := k.GetPartnership(liftCtx, pid)
	if p.Status != types.StatusSuspended {
		t.Errorf("expected still suspended (coercion active), got %s", p.Status)
	}

	// Now expire the coercion signal too
	coercionExpiredCtx := ctxAtHeight(ctx, 100+2001)
	k.ExpireCoercionSignals(coercionExpiredCtx)

	p, _ = k.GetPartnership(coercionExpiredCtx, pid)
	if p.Status != types.StatusActive {
		t.Errorf("expected active after both freeze and coercion expired, got %s", p.Status)
	}
}

// ==================== SEED PARTNERSHIP TESTS ====================

func TestSeedPartnership_Create(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	resp, err := ms.CreateSeedPartnership(ctx, &types.MsgCreateSeedPartnership{
		Human:             humanAddr,
		Agent:             agentAddr,
		HumanContribution: "5000000",
	})
	if err != nil {
		t.Fatalf("CreateSeedPartnership failed: %v", err)
	}

	sp, found := k.GetSeedPartnership(ctx, resp.SeedId)
	if !found {
		t.Fatal("seed partnership not found")
	}
	if sp.HumanAddr != humanAddr {
		t.Errorf("wrong human: %s", sp.HumanAddr)
	}
	if sp.AgentAddr != agentAddr {
		t.Errorf("wrong agent: %s", sp.AgentAddr)
	}
	if sp.Status != "active" {
		t.Errorf("expected active, got %s", sp.Status)
	}
	if sp.CommonPotBalance != "5000000" {
		t.Errorf("expected pot 5000000, got %s", sp.CommonPotBalance)
	}
	// ExpiresAt = 100 + 10000 = 10100
	if sp.ExpiresAt != 10100 {
		t.Errorf("expected expiry at 10100, got %d", sp.ExpiresAt)
	}
}

func TestSeedPartnership_TwoSeedLimit(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 20000000)

	// Seed 1
	_, err := ms.CreateSeedPartnership(ctx, &types.MsgCreateSeedPartnership{
		Human:             humanAddr,
		Agent:             agentAddr,
		HumanContribution: "1000000",
	})
	if err != nil {
		t.Fatalf("seed 1 failed: %v", err)
	}

	// Seed 2
	_, err = ms.CreateSeedPartnership(ctx, &types.MsgCreateSeedPartnership{
		Human:             humanAddr,
		Agent:             agent2Addr,
		HumanContribution: "1000000",
	})
	if err != nil {
		t.Fatalf("seed 2 failed: %v", err)
	}

	// Seed 3 — should fail
	_, err = ms.CreateSeedPartnership(ctx, &types.MsgCreateSeedPartnership{
		Human:             humanAddr,
		Agent:             agent3Addr,
		HumanContribution: "1000000",
	})
	if err == nil {
		t.Fatal("expected error: max 2 seeds per DID")
	}
}

func TestSeedPartnership_Expiry(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	resp, err := ms.CreateSeedPartnership(ctx, &types.MsgCreateSeedPartnership{
		Human:             humanAddr,
		Agent:             agentAddr,
		HumanContribution: "1000000",
	})
	if err != nil {
		t.Fatalf("CreateSeedPartnership failed: %v", err)
	}

	// Expire it
	expiredCtx := ctxAtHeight(ctx, 10200)
	k.ExpireSeedPartnerships(expiredCtx)

	sp, _ := k.GetSeedPartnership(expiredCtx, resp.SeedId)
	if sp.Status != "expired" {
		t.Errorf("expected expired, got %s", sp.Status)
	}
}

// ==================== FORMATION POOL TESTS ====================

func TestFormationPool_JoinAndLeave(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"math", "physics"},
		PreferredRole: "human",
		Deposit:       "100000",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	pe, found := k.GetPoolEntry(ctx, humanAddr)
	if !found {
		t.Fatal("pool entry not found")
	}
	if pe.Status != "active" {
		t.Errorf("expected active, got %s", pe.Status)
	}
	if len(pe.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(pe.Domains))
	}
	// ExpiresAt = 100 + 11111 = 11211
	if pe.ExpiresAt != 11211 {
		t.Errorf("expected expiry at 11211, got %d", pe.ExpiresAt)
	}

	// Leave
	_, err = ms.LeaveFormationPool(ctx, &types.MsgLeaveFormationPool{
		Leaver: humanAddr,
	})
	if err != nil {
		t.Fatalf("LeaveFormationPool failed: %v", err)
	}

	_, found = k.GetPoolEntry(ctx, humanAddr)
	if found {
		t.Fatal("pool entry should be deleted after leaving")
	}
}

func TestFormationPool_DuplicateJoinBlocked(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"math"},
		PreferredRole: "human",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	_, err = ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"physics"},
		PreferredRole: "human",
	})
	if err == nil {
		t.Fatal("expected error for duplicate pool join")
	}
}

func TestFormationPool_Expiry(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"math"},
		PreferredRole: "human",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	// Expire it
	expiredCtx := ctxAtHeight(ctx, 11300)
	k.ExpirePoolEntries(expiredCtx)

	pe, found := k.GetPoolEntry(expiredCtx, humanAddr)
	if !found {
		t.Fatal("pool entry should still exist (just marked expired)")
	}
	if pe.Status != "expired" {
		t.Errorf("expected expired, got %s", pe.Status)
	}
}

func TestFormationPool_LeaveRefundsDeposit(t *testing.T) {
	ms, _, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)

	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"math"},
		PreferredRole: "human",
		Deposit:       "500000",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	balBefore := bk.balances[humanAddr]["uzrn"]

	_, err = ms.LeaveFormationPool(ctx, &types.MsgLeaveFormationPool{
		Leaver: humanAddr,
	})
	if err != nil {
		t.Fatalf("LeaveFormationPool failed: %v", err)
	}

	balAfter := bk.balances[humanAddr]["uzrn"]
	if balAfter-balBefore != 500000 {
		t.Errorf("expected deposit refund of 500000, got diff %d", balAfter-balBefore)
	}
}

func TestFormationPool_LeaveNotInPoolError(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.LeaveFormationPool(ctx, &types.MsgLeaveFormationPool{
		Leaver: humanAddr,
	})
	if err == nil {
		t.Fatal("expected error when leaving pool without joining")
	}
}

// ==================== EXIT SETTLEMENT TESTS ====================

func TestExitSettlement_PureMath(t *testing.T) {
	pot := big.NewInt(2000000)
	human, agent, burned := keeper.CalculateExitSettlement(pot, 0, humanAddr, humanAddr)

	// Tier 0: ExitPenaltyBps = 110000 (11%), tierDenom = 1000000
	// halfPot = 1000000
	// initiator (human): penalty = 1000000 * 110000 / 1000000 = 110000
	//   forfeited = 110000 * 7700 / 10000 = 84700
	//   pay = 1000000 - 84700 = 915300
	// non-initiator (agent): penalty = 110000
	//   forfeited = 110000 * 5500 / 10000 = 60500
	//   pay = 1000000 - 60500 = 939500
	// burned = 84700 + 60500 = 145200

	if human.Int64() != 915300 {
		t.Errorf("human payout: expected 915300, got %d", human.Int64())
	}
	if agent.Int64() != 939500 {
		t.Errorf("agent payout: expected 939500, got %d", agent.Int64())
	}
	if burned.Int64() != 145200 {
		t.Errorf("burned: expected 145200, got %d", burned.Int64())
	}
}

func TestExitSettlement_HigherTierMorePenalty(t *testing.T) {
	pot := big.NewInt(2000000)

	_, _, burned0 := keeper.CalculateExitSettlement(pot, 0, humanAddr, humanAddr)
	_, _, burned3 := keeper.CalculateExitSettlement(pot, 3, humanAddr, humanAddr)
	_, _, burned5 := keeper.CalculateExitSettlement(pot, 5, humanAddr, humanAddr)

	if burned3.Cmp(burned0) <= 0 {
		t.Errorf("tier 3 should burn more than tier 0: %s vs %s", burned3, burned0)
	}
	if burned5.Cmp(burned3) <= 0 {
		t.Errorf("tier 5 should burn more than tier 3: %s vs %s", burned5, burned3)
	}
}

func TestExitSettlement_ZeroPot(t *testing.T) {
	human, agent, burned := keeper.CalculateExitSettlement(big.NewInt(0), 0, humanAddr, humanAddr)
	if human.Sign() != 0 || agent.Sign() != 0 || burned.Sign() != 0 {
		t.Error("zero pot should produce zero payouts")
	}
}

func TestExitSettlement_InitiatorPaysSteeper(t *testing.T) {
	pot := big.NewInt(2000000)

	// Human initiates
	humanPay1, agentPay1, _ := keeper.CalculateExitSettlement(pot, 2, humanAddr, humanAddr)
	// Agent initiates
	humanPay2, agentPay2, _ := keeper.CalculateExitSettlement(pot, 2, agentAddr, humanAddr)

	// When human initiates, human gets less (77% forfeiture rate vs 55%)
	if humanPay1.Cmp(agentPay1) >= 0 {
		t.Errorf("initiator should get less: human=%s, agent=%s", humanPay1, agentPay1)
	}
	// When agent initiates, agent gets less
	if agentPay2.Cmp(humanPay2) >= 0 {
		t.Errorf("initiator should get less: agent=%s, human=%s", agentPay2, humanPay2)
	}
}

func TestExitSettlement_BankTransfers(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)
	bk.setModuleBalance(types.ModuleName, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Move past lock expiry
	futureCtx := ctxAtHeight(ctx, 100+22223)

	_, err := ms.InitiateDissolution(futureCtx, &types.MsgInitiateDissolution{
		Initiator:     humanAddr,
		PartnershipId: pid,
	})
	if err != nil {
		t.Fatalf("InitiateDissolution failed: %v", err)
	}

	// Verify forfeited coins were burned (not routed to development_fund).
	if !bk.burnCalled {
		t.Error("expected BurnCoins to be called during exit settlement for forfeited amounts")
	}
	if bk.burnAmount <= 0 {
		t.Errorf("expected positive burn amount, got %d", bk.burnAmount)
	}

	p, _ := k.GetPartnership(futureCtx, pid)
	if p.CommonPotBalance != "0" {
		t.Errorf("expected pot drained to 0, got %s", p.CommonPotBalance)
	}
}

// ==================== REWARD DISTRIBUTION TESTS ====================

func TestRewardDistribution_Basic(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	partnership := &types.Partnership{
		Id:               "p-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		LockTier:         0,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, partnership)

	humanShare, agentShare, potAdd, err := k.DistributeReward(ctx, partnership, "1000000")
	if err != nil {
		t.Fatalf("DistributeReward failed: %v", err)
	}

	// LockTier 0: multiplier = 1000000 (1.0x) / 1000000 = 1.0
	// adjusted = 1000000
	// CommonPotShareBps = 100000 (10%) → potShare = 100000
	// remaining = 900000
	// humanShare = 900000 * 500000/1000000 = 450000
	// agentShare = 900000 - 450000 = 450000
	if humanShare != "450000" {
		t.Errorf("expected human share 450000, got %s", humanShare)
	}
	if agentShare != "450000" {
		t.Errorf("expected agent share 450000, got %s", agentShare)
	}
	if potAdd != "100000" {
		t.Errorf("expected pot add 100000, got %s", potAdd)
	}
}

func TestRewardDistribution_LockMultiplier(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	partnership := &types.Partnership{
		Id:               "p-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		LockTier:         5, // 1.77x multiplier
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, partnership)

	_, _, _, err := k.DistributeReward(ctx, partnership, "1000000")
	if err != nil {
		t.Fatalf("DistributeReward failed: %v", err)
	}

	p, _ := k.GetPartnership(ctx, "p-1")
	// adjusted = 1000000 * 1770000 / 1000000 = 1770000
	if p.TotalEarned != "1770000" {
		t.Errorf("expected total earned 1770000, got %s", p.TotalEarned)
	}
}

func TestRewardDistribution_UnequalSplit(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	partnership := &types.Partnership{
		Id:               "p-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		LockTier:         0,
		SplitHumanBps:    700000, // 70%
		SplitAgentBps:    300000, // 30%
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, partnership)

	humanShare, agentShare, _, err := k.DistributeReward(ctx, partnership, "1000000")
	if err != nil {
		t.Fatalf("DistributeReward failed: %v", err)
	}

	// remaining = 900000 (after 10% pot share)
	// humanShare = 900000 * 700000 / 1000000 = 630000
	// agentShare = 900000 - 630000 = 270000
	if humanShare != "630000" {
		t.Errorf("expected human share 630000, got %s", humanShare)
	}
	if agentShare != "270000" {
		t.Errorf("expected agent share 270000, got %s", agentShare)
	}
}

func TestLockMultiplier_Tiers(t *testing.T) {
	expected := []uint64{1000000, 1110000, 1220000, 1440000, 1550000, 1770000}
	for tier, want := range expected {
		got := keeper.GetLockMultiplier(uint32(tier))
		if got.Uint64() != want {
			t.Errorf("tier %d: expected multiplier %d, got %d", tier, want, got.Uint64())
		}
	}
}

func TestLockMultiplier_CapsAt5(t *testing.T) {
	m5 := keeper.GetLockMultiplier(5)
	m99 := keeper.GetLockMultiplier(99)
	if m5.Cmp(m99) != 0 {
		t.Errorf("tier >5 should cap at tier 5: %s vs %s", m99, m5)
	}
}

// ==================== COOLDOWN TESTS ====================

func TestCooldown_ExponentialGrowth(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// BaseCooldownBlocks = 100
	// CalculateCooldown doubles with each rejection
	tests := []struct {
		rejections uint32
		expected   uint64
	}{
		{1, 200},    // 100 * 2^1
		{2, 400},    // 100 * 2^2
		{3, 800},    // 100 * 2^3
		{4, 1600},   // 100 * 2^4
		{5, 3200},   // 100 * 2^5
		{6, 6400},   // 100 * 2^6
		{7, 11111},  // capped at maxCooldownBlocks
		{10, 11111}, // still capped
	}

	for _, tt := range tests {
		got := k.CalculateCooldown(ctx, tt.rejections)
		if got != tt.expected {
			t.Errorf("CalculateCooldown(%d) = %d, want %d", tt.rejections, got, tt.expected)
		}
	}
}

// ==================== EXPIRE CONSENSUS OPS TESTS ====================

func TestExpireConsensusOps(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// micro tier: window ends at 100+22 = 122
	expiredCtx := ctxAtHeight(ctx, 130)
	k.ExpireConsensusOps(expiredCtx)

	op, _ := k.GetConsensusOperation(expiredCtx, opResp.OperationId)
	if op.Status != types.OpStatusExpired {
		t.Errorf("expected expired, got %s", op.Status)
	}
}

// ==================== GENESIS ROUNDTRIP TEST ====================

func TestGenesis_InitExportRoundtrip(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Set up some state
	partnership := &types.Partnership{
		Id:               "p-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "1000000",
		TotalEarned:      "500000",
		CooperationScore: 500000,
	}
	k.SetPartnership(ctx, partnership)

	sf := &types.SafetyFreeze{
		PartnershipId:        "p-1",
		FrozenBy:             humanAddr,
		FrozenAt:             100,
		ExpiresAt:            600,
		FreezeCountThisEpoch: 1,
	}
	k.SetSafetyFreeze(ctx, sf)

	// Export
	gs := k.ExportGenesis(ctx)
	if len(gs.Partnerships) != 1 {
		t.Errorf("expected 1 partnership, got %d", len(gs.Partnerships))
	}
	if len(gs.SafetyFreezes) != 1 {
		t.Errorf("expected 1 safety freeze, got %d", len(gs.SafetyFreezes))
	}

	// Create fresh keeper and import
	k2, ctx2, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, gs)

	p, found := k2.GetPartnership(ctx2, "p-1")
	if !found {
		t.Fatal("partnership not found after genesis import")
	}
	if p.Status != types.StatusActive {
		t.Errorf("expected active, got %s", p.Status)
	}
	if p.CommonPotBalance != "1000000" {
		t.Errorf("expected pot 1000000, got %s", p.CommonPotBalance)
	}

	sf2, found := k2.GetSafetyFreeze(ctx2, "p-1")
	if !found {
		t.Fatal("safety freeze not found after genesis import")
	}
	if sf2.FreezeCountThisEpoch != 1 {
		t.Errorf("expected freeze count 1, got %d", sf2.FreezeCountThisEpoch)
	}
}

func TestGenesis_MentorshipRoundTrip(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr,
		Domain: "physics", Status: "active", StartBlock: 100, DurationBlocks: 1000,
	})
	k.SetFormationMatch(ctx, &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		Score: 8000, ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	})

	exported := k.ExportGenesis(ctx)
	if len(exported.Mentorships) != 1 {
		t.Errorf("expected 1 mentorship in export, got %d", len(exported.Mentorships))
	}
	if len(exported.FormationMatches) != 1 {
		t.Errorf("expected 1 formation match in export, got %d", len(exported.FormationMatches))
	}

	k2, ctx2, _ := setupKeeper(t)
	k2.InitGenesis(ctx2, exported)

	m, found := k2.GetMentorship(ctx2, "m-1")
	if !found {
		t.Fatal("mentorship not found after import")
	}
	if m.Domain != "physics" {
		t.Errorf("expected physics, got %s", m.Domain)
	}

	fm, found := k2.GetFormationMatch(ctx2, "match-1")
	if !found {
		t.Fatal("formation match not found after import")
	}
	if fm.Score != 8000 {
		t.Errorf("expected score 8000, got %d", fm.Score)
	}
}

// ==================== GOVERNANCE TESTS ====================

func TestUpdateParams_AuthorityOnly(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	// Non-authority tries to update params
	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: outsiderAddr,
		Params:    types.DefaultParams(),
	})
	if err == nil {
		t.Fatal("expected error for non-authority params update")
	}

	// Authority succeeds
	_, err = ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authority,
		Params:    types.DefaultParams(),
	})
	if err != nil {
		t.Fatalf("authority params update failed: %v", err)
	}
}

// ==================== VALIDATE BASIC TESTS ====================

func TestValidateBasic_ProposePartnership(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgProposePartnership
		wantErr bool
	}{
		{"valid", &types.MsgProposePartnership{Proposer: humanAddr, Partner: agentAddr, ProposedTier: 0}, false},
		{"empty proposer", &types.MsgProposePartnership{Proposer: "", Partner: agentAddr}, true},
		{"self partnership", &types.MsgProposePartnership{Proposer: humanAddr, Partner: humanAddr}, true},
		{"invalid tier", &types.MsgProposePartnership{Proposer: humanAddr, Partner: agentAddr, ProposedTier: 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_ConsensusOp(t *testing.T) {
	msg := &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: "p-1",
		OpType:        "withdraw",
		Amount:        "1000",
	}
	if err := msg.ValidateBasic(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}

	msg.Amount = "-100"
	if err := msg.ValidateBasic(); err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestDefaultGenesis_Validation(t *testing.T) {
	gs := types.DefaultGenesis()
	if err := gs.Validate(); err != nil {
		t.Errorf("default genesis should be valid: %v", err)
	}
}

// ==================== QUERY SERVER TESTS ====================

func TestQueryServer_Params(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("Params query failed: %v", err)
	}
	if resp.Params == nil {
		t.Fatal("expected non-nil params")
	}
	if resp.Params.FormationWindowBlocks != 1000 {
		t.Errorf("expected FormationWindowBlocks 1000, got %d", resp.Params.FormationWindowBlocks)
	}
	if resp.Params.CoolingPeriodBlocks != 5000 {
		t.Errorf("expected CoolingPeriodBlocks 5000, got %d", resp.Params.CoolingPeriodBlocks)
	}
}

func TestQueryServer_Partnership(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-q1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "5000000",
		TotalEarned:      "10000000",
		CooperationScore: 500000,
	}
	k.SetPartnership(ctx, p)

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.Partnership(ctx, &types.QueryPartnershipRequest{Id: "p-q1"})
	if err != nil {
		t.Fatalf("Partnership query failed: %v", err)
	}
	if resp.Partnership.Id != "p-q1" {
		t.Errorf("expected id p-q1, got %s", resp.Partnership.Id)
	}
	if resp.Partnership.CommonPotBalance != "5000000" {
		t.Errorf("expected pot 5000000, got %s", resp.Partnership.CommonPotBalance)
	}
	if resp.Partnership.Status != types.StatusActive {
		t.Errorf("expected active, got %s", resp.Partnership.Status)
	}
}

func TestQueryServer_PartnershipNotFound(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Partnership(ctx, &types.QueryPartnershipRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent partnership")
	}
}

func TestQueryServer_PartnershipsByAddress(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p1 := &types.Partnership{
		Id:               "p-addr1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, p1)

	p2 := &types.Partnership{
		Id:               "p-addr2",
		HumanAddr:        humanAddr,
		AgentAddr:        agent2Addr,
		Status:           types.StatusDissolved,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, p2)

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.PartnershipsByAddress(ctx, &types.QueryByAddressRequest{Address: humanAddr})
	if err != nil {
		t.Fatalf("PartnershipsByAddress query failed: %v", err)
	}
	if len(resp.Partnerships) != 2 {
		t.Errorf("expected 2 partnerships, got %d", len(resp.Partnerships))
	}
}

func TestQueryServer_PartnershipsByAddressEmpty(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.PartnershipsByAddress(ctx, &types.QueryByAddressRequest{Address: outsiderAddr})
	if err != nil {
		t.Fatalf("PartnershipsByAddress query failed: %v", err)
	}
	if len(resp.Partnerships) != 0 {
		t.Errorf("expected 0 partnerships, got %d", len(resp.Partnerships))
	}
}

func TestQueryServer_PendingOps(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Create a pending operation
	_, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "1000",
		Rationale:     "test",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.PendingOps(ctx, &types.QueryPendingOpsRequest{PartnershipId: pid})
	if err != nil {
		t.Fatalf("PendingOps query failed: %v", err)
	}
	if len(resp.Operations) != 1 {
		t.Errorf("expected 1 pending op, got %d", len(resp.Operations))
	}
	if resp.Operations[0].Status != types.OpStatusPending {
		t.Errorf("expected pending status, got %s", resp.Operations[0].Status)
	}
}

func TestQueryServer_PendingOpsEmpty(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.PendingOps(ctx, &types.QueryPendingOpsRequest{PartnershipId: "p-nonexistent"})
	if err != nil {
		t.Fatalf("PendingOps query failed: %v", err)
	}
	if len(resp.Operations) != 0 {
		t.Errorf("expected 0 ops, got %d", len(resp.Operations))
	}
}

func TestQueryServer_FormationPool(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        humanAddr,
		Domains:       []string{"math"},
		PreferredRole: "human",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	_, err = ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner:        agentAddr,
		Domains:       []string{"physics"},
		PreferredRole: "agent",
	})
	if err != nil {
		t.Fatalf("JoinFormationPool failed: %v", err)
	}

	qs := keeper.NewQueryServerImpl(k)
	resp, err := qs.FormationPool(ctx, &types.QueryFormationPoolRequest{})
	if err != nil {
		t.Fatalf("FormationPool query failed: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Entries))
	}
}

func TestQueryServer_NilRequestParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Params(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil params request")
	}
}

func TestQueryServer_NilRequestPartnership(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Partnership(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil partnership request")
	}
}

func TestQueryServer_NilRequestPartnershipsByAddress(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.PartnershipsByAddress(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil by-address request")
	}
}

func TestQueryServer_NilRequestPendingOps(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.PendingOps(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil pending ops request")
	}
}

func TestQueryServer_NilRequestFormationPool(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.FormationPool(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil formation pool request")
	}
}

func TestQueryServer_PartnershipEmptyId(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Partnership(ctx, &types.QueryPartnershipRequest{Id: ""})
	if err == nil {
		t.Fatal("expected error for empty partnership id")
	}
}

func TestQueryServer_PartnershipsByAddressEmptyAddr(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.PartnershipsByAddress(ctx, &types.QueryByAddressRequest{Address: ""})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestQueryServer_PendingOpsEmptyPartnershipId(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.PendingOps(ctx, &types.QueryPendingOpsRequest{PartnershipId: ""})
	if err == nil {
		t.Fatal("expected error for empty partnership id")
	}
}

// ==================== CRUD DIRECT TESTS ====================

func TestPartnershipCRUD_Direct(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Not found initially
	_, found := k.GetPartnership(ctx, "p-crud-1")
	if found {
		t.Fatal("expected partnership not found")
	}

	// Set
	p := &types.Partnership{
		Id:               "p-crud-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    600000,
		SplitAgentBps:    400000,
		CommonPotBalance: "5000",
		TotalEarned:      "10000",
		CooperationScore: 700000,
	}
	k.SetPartnership(ctx, p)

	// Get
	got, found := k.GetPartnership(ctx, "p-crud-1")
	if !found {
		t.Fatal("expected partnership found")
	}
	if got.HumanAddr != humanAddr {
		t.Errorf("expected human %s, got %s", humanAddr, got.HumanAddr)
	}
	if got.SplitHumanBps != 600000 {
		t.Errorf("expected split 600000, got %d", got.SplitHumanBps)
	}

	// GetAll
	all := k.GetAllPartnerships(ctx)
	if len(all) != 1 {
		t.Errorf("expected 1 partnership, got %d", len(all))
	}

	// By Human index
	ids := k.GetPartnershipsByHuman(ctx, humanAddr)
	if len(ids) != 1 || ids[0] != "p-crud-1" {
		t.Errorf("expected by human: [p-crud-1], got %v", ids)
	}

	// By Agent index
	ids = k.GetPartnershipsByAgent(ctx, agentAddr)
	if len(ids) != 1 || ids[0] != "p-crud-1" {
		t.Errorf("expected by agent: [p-crud-1], got %v", ids)
	}

	// Delete
	k.DeletePartnership(ctx, p)
	_, found = k.GetPartnership(ctx, "p-crud-1")
	if found {
		t.Fatal("expected partnership deleted")
	}
}

func TestConsensusOperationCRUD_Direct(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Not found initially
	_, found := k.GetConsensusOperation(ctx, "op-crud-1")
	if found {
		t.Fatal("expected operation not found")
	}

	// Set
	op := &types.ConsensusOperation{
		Id:            "op-crud-1",
		PartnershipId: "p-1",
		OpType:        "withdraw",
		ProposedBy:    humanAddr,
		Amount:        "5000",
		Status:        types.OpStatusPending,
		Deliberation: &types.DeliberationState{
			AmountTier:   types.AmountTierSmall,
			WindowEndsAt: 200,
			FloorEndsAt:  150,
		},
		CreatedAt: 100,
	}
	k.SetConsensusOperation(ctx, op)

	// Get
	got, found := k.GetConsensusOperation(ctx, "op-crud-1")
	if !found {
		t.Fatal("expected operation found")
	}
	if got.Amount != "5000" {
		t.Errorf("expected amount 5000, got %s", got.Amount)
	}
	if got.Status != types.OpStatusPending {
		t.Errorf("expected pending, got %s", got.Status)
	}

	// GetAll
	all := k.GetAllConsensusOperations(ctx)
	if len(all) != 1 {
		t.Errorf("expected 1 operation, got %d", len(all))
	}

	// Delete
	k.DeleteConsensusOperation(ctx, "op-crud-1")
	_, found = k.GetConsensusOperation(ctx, "op-crud-1")
	if found {
		t.Fatal("expected operation deleted")
	}
}

func TestPartnershipsByParticipant_Direct(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-part-1",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, p)

	// Human finds it
	result := k.GetPartnershipsByParticipant(ctx, humanAddr)
	if len(result) != 1 {
		t.Errorf("expected 1 for human, got %d", len(result))
	}

	// Agent finds it
	result = k.GetPartnershipsByParticipant(ctx, agentAddr)
	if len(result) != 1 {
		t.Errorf("expected 1 for agent, got %d", len(result))
	}

	// Non-participant finds nothing
	result = k.GetPartnershipsByParticipant(ctx, outsiderAddr)
	if len(result) != 0 {
		t.Errorf("expected 0 for outsider, got %d", len(result))
	}
}

// ==================== ADDITIONAL EDGE CASES ====================

func TestRewardDistribution_Accumulates(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	partnership := &types.Partnership{
		Id:               "p-accum",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		LockTier:         0,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "1000",
		TotalEarned:      "5000",
	}
	k.SetPartnership(ctx, partnership)

	// First distribution
	_, _, potAdd1, err := k.DistributeReward(ctx, partnership, "1000000")
	if err != nil {
		t.Fatalf("first DistributeReward failed: %v", err)
	}

	// Refresh partnership from store after first distribution
	partnership, _ = k.GetPartnership(ctx, "p-accum")

	// Second distribution
	_, _, potAdd2, err := k.DistributeReward(ctx, partnership, "500000")
	if err != nil {
		t.Fatalf("second DistributeReward failed: %v", err)
	}

	// Verify accumulation
	partnership, _ = k.GetPartnership(ctx, "p-accum")

	// Initial pot: 1000
	// First pot add: 1000000 * 100000 / 1000000 = 100000 → pot = 101000
	// Second pot add: 500000 * 100000 / 1000000 = 50000 → pot = 151000
	potAddBig1 := new(big.Int)
	potAddBig1.SetString(potAdd1, 10)
	potAddBig2 := new(big.Int)
	potAddBig2.SetString(potAdd2, 10)
	expectedPot := new(big.Int).SetInt64(1000)
	expectedPot.Add(expectedPot, potAddBig1)
	expectedPot.Add(expectedPot, potAddBig2)

	if partnership.CommonPotBalance != expectedPot.String() {
		t.Errorf("expected pot %s, got %s", expectedPot.String(), partnership.CommonPotBalance)
	}

	// Total earned = 5000 + 1000000 + 500000 = 1505000
	if partnership.TotalEarned != "1505000" {
		t.Errorf("expected total earned 1505000, got %s", partnership.TotalEarned)
	}
}

func TestHandleExit_AlreadyDissolved(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-dissolved",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusDissolved,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, p)

	_, err := k.HandleExit(ctx, "p-dissolved", humanAddr)
	if err == nil {
		t.Fatal("expected error for already dissolved partnership")
	}
}

func TestSafetyFreeze_AlreadyActive(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-freeze-active",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
		CooperationScore: 500000,
	}
	k.SetPartnership(ctx, p)

	// Set an already-active freeze (expires in the future)
	sf := &types.SafetyFreeze{
		PartnershipId:        "p-freeze-active",
		FreezeCountThisEpoch: 1,
		ExpiresAt:            200, // current block is 100, so still active
	}
	k.SetSafetyFreeze(ctx, sf)

	_, err := k.HandleSafetyFreeze(ctx, "p-freeze-active", humanAddr)
	if err == nil {
		t.Fatal("expected error when freeze already active")
	}
}

func TestCoercionSignal_AlreadyActive(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-coerce-dup",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusActive,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
	}
	k.SetPartnership(ctx, p)

	// First signal succeeds
	_, err := k.HandleCoercionSignal(ctx, "p-coerce-dup", humanAddr)
	if err != nil {
		t.Fatalf("first coercion signal failed: %v", err)
	}

	// Second signal should fail (already active)
	_, err = k.HandleCoercionSignal(ctx, "p-coerce-dup", agentAddr)
	if err == nil {
		t.Fatal("expected error for duplicate coercion signal")
	}
}

// ==================== SETTLEMENT EDGE CASES ====================

func TestSettleCooling_NotYetExpired(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-cool-notyet",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusCooling,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "10000",
		ExitState: &types.ExitState{
			InitiatedBy: humanAddr,
			InitiatedAt: 50,
			CooldownEnd: 200, // after current block (100)
		},
	}
	k.SetPartnership(ctx, p)

	k.SettleCoolingPartnerships(ctx)

	updated, _ := k.GetPartnership(ctx, "p-cool-notyet")
	if updated.Status != types.StatusCooling {
		t.Errorf("expected still cooling, got %s", updated.Status)
	}
}

func TestSettleCooling_Expired(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-cool-expired",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusCooling,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "10000",
		ExitState: &types.ExitState{
			InitiatedBy: humanAddr,
			InitiatedAt: 50,
			CooldownEnd: 90, // before current block (100)
		},
	}
	k.SetPartnership(ctx, p)

	k.SettleCoolingPartnerships(ctx)

	updated, _ := k.GetPartnership(ctx, "p-cool-expired")
	if updated.Status != types.StatusDissolved {
		t.Errorf("expected dissolved, got %s", updated.Status)
	}
}

func TestWithdrawalExecution_ViaApproval(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)
	bk.setModuleBalance(types.ModuleName, "uzrn", 10000000)

	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	// Propose withdraw of 100000
	opResp, err := ms.ProposeConsensusOp(ctx, &types.MsgProposeConsensusOp{
		Proposer:      humanAddr,
		PartnershipId: pid,
		OpType:        "withdraw",
		Amount:        "100000",
		Rationale:     "operational expenses",
	})
	if err != nil {
		t.Fatalf("ProposeConsensusOp failed: %v", err)
	}

	// Check tier: 100000 / 2000000 = 5% = medium tier (window=222, floor=111)
	op, _ := k.GetConsensusOperation(ctx, opResp.OperationId)
	if op.Deliberation.AmountTier != types.AmountTierMedium {
		t.Errorf("expected medium tier, got %s", op.Deliberation.AmountTier)
	}

	// Approve after floor period
	voteCtx := ctxAtHeight(ctx, 100+112)
	_, err = ms.VoteConsensusOp(voteCtx, &types.MsgVoteConsensusOp{
		OperationId: opResp.OperationId,
		Voter:       agentAddr,
		Approve:     true,
	})
	if err != nil {
		t.Fatalf("VoteConsensusOp failed: %v", err)
	}

	// Verify pot balance decreased
	p, _ := k.GetPartnership(voteCtx, pid)
	potBal := new(big.Int)
	potBal.SetString(p.CommonPotBalance, 10)
	expected := new(big.Int)
	expected.SetString("1900000", 10) // 2000000 - 100000
	if potBal.Cmp(expected) != 0 {
		t.Errorf("expected pot 1900000, got %s", p.CommonPotBalance)
	}
}

// ==================== ADDITIONAL VALIDATE BASIC TESTS ====================

func TestValidateBasic_AcceptPartnership(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgAcceptPartnership
		wantErr bool
	}{
		{"valid", &types.MsgAcceptPartnership{Accepter: agentAddr, PartnershipId: "p-1"}, false},
		{"empty accepter", &types.MsgAcceptPartnership{Accepter: "", PartnershipId: "p-1"}, true},
		{"empty partnership id", &types.MsgAcceptPartnership{Accepter: agentAddr, PartnershipId: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_SafetyFreeze(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgSafetyFreeze
		wantErr bool
	}{
		{"valid", &types.MsgSafetyFreeze{Freezer: humanAddr, PartnershipId: "p-1"}, false},
		{"empty freezer", &types.MsgSafetyFreeze{Freezer: "", PartnershipId: "p-1"}, true},
		{"empty partnership id", &types.MsgSafetyFreeze{Freezer: humanAddr, PartnershipId: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_RaiseCoercionSignal(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgRaiseCoercionSignal
		wantErr bool
	}{
		{"valid", &types.MsgRaiseCoercionSignal{Raiser: humanAddr, PartnershipId: "p-1"}, false},
		{"empty raiser", &types.MsgRaiseCoercionSignal{Raiser: "", PartnershipId: "p-1"}, true},
		{"empty partnership id", &types.MsgRaiseCoercionSignal{Raiser: humanAddr, PartnershipId: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_InitiateDissolution(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgInitiateDissolution
		wantErr bool
	}{
		{"valid", &types.MsgInitiateDissolution{Initiator: humanAddr, PartnershipId: "p-1"}, false},
		{"empty initiator", &types.MsgInitiateDissolution{Initiator: "", PartnershipId: "p-1"}, true},
		{"empty partnership id", &types.MsgInitiateDissolution{Initiator: humanAddr, PartnershipId: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_CreateSeedPartnership(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgCreateSeedPartnership
		wantErr bool
	}{
		{"valid", &types.MsgCreateSeedPartnership{Human: humanAddr, Agent: agentAddr}, false},
		{"empty human", &types.MsgCreateSeedPartnership{Human: "", Agent: agentAddr}, true},
		{"empty agent", &types.MsgCreateSeedPartnership{Human: humanAddr, Agent: ""}, true},
		{"self partnership", &types.MsgCreateSeedPartnership{Human: humanAddr, Agent: humanAddr}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_JoinFormationPool(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgJoinFormationPool
		wantErr bool
	}{
		{"valid", &types.MsgJoinFormationPool{Joiner: humanAddr, Domains: []string{"math"}, PreferredRole: "human"}, false},
		{"empty joiner", &types.MsgJoinFormationPool{Joiner: "", Domains: []string{"math"}, PreferredRole: "human"}, true},
		{"no domains", &types.MsgJoinFormationPool{Joiner: humanAddr, Domains: []string{}, PreferredRole: "human"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_LeaveFormationPool(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgLeaveFormationPool
		wantErr bool
	}{
		{"valid", &types.MsgLeaveFormationPool{Leaver: humanAddr}, false},
		{"empty leaver", &types.MsgLeaveFormationPool{Leaver: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_UpdateParams(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgUpdateParams
		wantErr bool
	}{
		{"valid", &types.MsgUpdateParams{Authority: authority, Params: types.DefaultParams()}, false},
		{"empty authority", &types.MsgUpdateParams{Authority: "", Params: types.DefaultParams()}, true},
		{"nil params", &types.MsgUpdateParams{Authority: authority, Params: nil}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBasic_VoteConsensusOp(t *testing.T) {
	tests := []struct {
		name    string
		msg     *types.MsgVoteConsensusOp
		wantErr bool
	}{
		{"valid approve", &types.MsgVoteConsensusOp{Voter: humanAddr, OperationId: "op-1", Approve: true}, false},
		{"valid reject", &types.MsgVoteConsensusOp{Voter: humanAddr, OperationId: "op-1", Approve: false}, false},
		{"empty voter", &types.MsgVoteConsensusOp{Voter: "", OperationId: "op-1", Approve: true}, true},
		{"empty operation id", &types.MsgVoteConsensusOp{Voter: humanAddr, OperationId: "", Approve: true}, true},
		{"invalid counter amount", &types.MsgVoteConsensusOp{Voter: humanAddr, OperationId: "op-1", Approve: false, CounterAmount: "abc"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ==================== SEQUENCE AUTO-INCREMENT ====================

func TestSequence_AutoIncrement(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	seq1 := k.NextSequence(ctx)
	seq2 := k.NextSequence(ctx)
	seq3 := k.NextSequence(ctx)

	if seq1 != 1 {
		t.Errorf("expected seq1=1, got %d", seq1)
	}
	if seq2 != 2 {
		t.Errorf("expected seq2=2, got %d", seq2)
	}
	if seq3 != 3 {
		t.Errorf("expected seq3=3, got %d", seq3)
	}

	// Verify monotonic increment
	if seq2 != seq1+1 || seq3 != seq2+1 {
		t.Error("sequence is not monotonically incrementing")
	}
}

// ==================== HANDLE EXIT EDGE CASES ====================

func TestHandleExit_CoolingStatusBlocked(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-cooling",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusCooling,
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "0",
		TotalEarned:      "0",
		ExitState: &types.ExitState{
			InitiatedBy: humanAddr,
			InitiatedAt: 50,
			CooldownEnd: 200,
		},
	}
	k.SetPartnership(ctx, p)

	_, err := k.HandleExit(ctx, "p-cooling", agentAddr)
	if err == nil {
		t.Fatal("expected error for exit on already-cooling partnership")
	}
}

func TestHandleExit_SuspendedBypassesLock(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	p := &types.Partnership{
		Id:               "p-suspended-exit",
		HumanAddr:        humanAddr,
		AgentAddr:        agentAddr,
		Status:           types.StatusSuspended,
		LockTier:         3,
		LockExpiresAt:    999999, // lock not expired
		SplitHumanBps:    500000,
		SplitAgentBps:    500000,
		CommonPotBalance: "10000",
		TotalEarned:      "20000",
	}
	k.SetPartnership(ctx, p)

	_, err := k.HandleExit(ctx, "p-suspended-exit", humanAddr)
	if err != nil {
		t.Fatalf("expected exit during suspension to succeed, got: %v", err)
	}

	updated, _ := k.GetPartnership(ctx, "p-suspended-exit")
	if updated.Status != types.StatusCooling {
		t.Errorf("expected cooling, got %s", updated.Status)
	}
}

// ==================== PARAMS VALIDATION ====================

func TestParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  types.Params
		wantErr bool
	}{
		{"valid default", *types.DefaultParams(), false},
		{"zero formation window", types.Params{
			FormationWindowBlocks: 0, CoolingPeriodBlocks: 5000, CommonPotShareBps: 100000,
			SafetyFreezeDurationBlocks: 500, MaxFreezesPerEpoch: 3, CoercionReviewBlocks: 2000,
			BaseCooldownBlocks: 100, MaxCounterProposalDepth: 3,
			DefaultHumanSplitBps: 500000, DefaultAgentSplitBps: 500000,
		}, true},
		{"splits dont sum to 1000000", types.Params{
			FormationWindowBlocks: 1000, CoolingPeriodBlocks: 5000, CommonPotShareBps: 100000,
			SafetyFreezeDurationBlocks: 500, MaxFreezesPerEpoch: 3, CoercionReviewBlocks: 2000,
			BaseCooldownBlocks: 100, MaxCounterProposalDepth: 3,
			DefaultHumanSplitBps: 600000, DefaultAgentSplitBps: 500000,
		}, true},
	}

	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ==================== GENESIS VALIDATION EDGE CASES ====================

func TestGenesisValidation_DuplicatePartnershipIds(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Partnerships = []*types.Partnership{
		{Id: "p-1", HumanAddr: humanAddr, AgentAddr: agentAddr, SplitHumanBps: 500000, SplitAgentBps: 500000},
		{Id: "p-1", HumanAddr: humanAddr, AgentAddr: agent2Addr, SplitHumanBps: 500000, SplitAgentBps: 500000},
	}
	if err := gs.Validate(); err == nil {
		t.Fatal("expected error for duplicate partnership IDs in genesis")
	}
}

func TestGenesisValidation_InvalidSplits(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Partnerships = []*types.Partnership{
		{Id: "p-bad", HumanAddr: humanAddr, AgentAddr: agentAddr, SplitHumanBps: 600000, SplitAgentBps: 500000},
	}
	if err := gs.Validate(); err == nil {
		t.Fatal("expected error for invalid splits in genesis")
	}
}

// ==================== COUNTER-PROPOSAL VALIDATION ====================

func TestValidateCounterProposal_DepthCheck(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// MaxCounterProposalDepth = 3 by default
	// Depth 2: should be OK (can create depth 3)
	op := &types.ConsensusOperation{
		Deliberation: &types.DeliberationState{ChainDepth: 2},
	}
	if err := k.ValidateCounterProposal(ctx, op); err != nil {
		t.Errorf("expected no error at depth 2, got: %v", err)
	}

	// Depth 3: should fail (at max)
	op.Deliberation.ChainDepth = 3
	if err := k.ValidateCounterProposal(ctx, op); err == nil {
		t.Error("expected error at depth 3 (max)")
	}
}

// ==================== CONSENSUS OP PENDING FILTERING ====================

func TestGetPendingOpsForPartnership_FiltersNonPending(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	// Create two ops for same partnership: one pending, one approved
	op1 := &types.ConsensusOperation{
		Id:            "op-pend-1",
		PartnershipId: "p-1",
		OpType:        "withdraw",
		ProposedBy:    humanAddr,
		Amount:        "1000",
		Status:        types.OpStatusPending,
		Deliberation: &types.DeliberationState{
			AmountTier:   types.AmountTierMicro,
			WindowEndsAt: 200,
			FloorEndsAt:  150,
		},
		CreatedAt: 100,
	}
	k.SetConsensusOperation(ctx, op1)

	op2 := &types.ConsensusOperation{
		Id:            "op-pend-2",
		PartnershipId: "p-1",
		OpType:        "withdraw",
		ProposedBy:    humanAddr,
		Amount:        "2000",
		Status:        types.OpStatusApproved,
		Deliberation: &types.DeliberationState{
			AmountTier:   types.AmountTierSmall,
			WindowEndsAt: 300,
			FloorEndsAt:  250,
		},
		CreatedAt: 100,
	}
	k.SetConsensusOperation(ctx, op2)

	// A third op for a different partnership
	op3 := &types.ConsensusOperation{
		Id:            "op-pend-3",
		PartnershipId: "p-other",
		OpType:        "withdraw",
		ProposedBy:    humanAddr,
		Amount:        "3000",
		Status:        types.OpStatusPending,
		Deliberation: &types.DeliberationState{
			AmountTier:   types.AmountTierMedium,
			WindowEndsAt: 400,
			FloorEndsAt:  350,
		},
		CreatedAt: 100,
	}
	k.SetConsensusOperation(ctx, op3)

	pending := k.GetPendingOpsForPartnership(ctx, "p-1")
	if len(pending) != 1 {
		t.Errorf("expected 1 pending op for p-1, got %d", len(pending))
	}
	if len(pending) > 0 && pending[0].Id != "op-pend-1" {
		t.Errorf("expected op-pend-1, got %s", pending[0].Id)
	}
}

// ==================== HUMAN COERCION FREEZE MULTIPLIER (R28-5) ====================

// mockPartnershipsAuthKeeper implements types.ZeroneAuthKeeper for tests.
type mockPartnershipsAuthKeeper struct {
	accounts map[string]string
}

func newMockPartnershipsAuthKeeper() *mockPartnershipsAuthKeeper {
	return &mockPartnershipsAuthKeeper{accounts: make(map[string]string)}
}

func (m *mockPartnershipsAuthKeeper) GetAccountType(_ context.Context, address string) (string, bool) {
	t, ok := m.accounts[address]
	return t, ok
}

func TestCoercionSignal_HumanFreezeMultiplier(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	// Set up auth keeper with human account type
	authKeeper := newMockPartnershipsAuthKeeper()
	authKeeper.accounts[humanAddr] = "human"
	authKeeper.accounts[agentAddr] = "agent"
	k.SetZeroneAuthKeeper(authKeeper)

	ms := keeper.NewMsgServerImpl(k)
	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	params := k.GetParams(ctx)
	currentBlock := uint64(ctx.BlockHeight())
	expectedExpiry := currentBlock + params.CoercionReviewBlocks*params.HumanCoercionFreezeMultiplierBps/1_000_000

	cs, err := k.HandleCoercionSignal(ctx, pid, humanAddr)
	if err != nil {
		t.Fatalf("HandleCoercionSignal failed: %v", err)
	}

	if cs.ExpiresAt != expectedExpiry {
		t.Errorf("human coercion freeze: expected ExpiresAt=%d (base=%d * 1.5), got %d",
			expectedExpiry, params.CoercionReviewBlocks, cs.ExpiresAt)
	}
}

func TestCoercionSignal_AgentNoMultiplier(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	// Set up auth keeper with agent account type for the raiser
	authKeeper := newMockPartnershipsAuthKeeper()
	authKeeper.accounts[humanAddr] = "human"
	authKeeper.accounts[agentAddr] = "agent"
	k.SetZeroneAuthKeeper(authKeeper)

	ms := keeper.NewMsgServerImpl(k)
	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	params := k.GetParams(ctx)
	currentBlock := uint64(ctx.BlockHeight())
	expectedExpiry := currentBlock + params.CoercionReviewBlocks // no multiplier for agent

	cs, err := k.HandleCoercionSignal(ctx, pid, agentAddr)
	if err != nil {
		t.Fatalf("HandleCoercionSignal failed: %v", err)
	}

	if cs.ExpiresAt != expectedExpiry {
		t.Errorf("agent coercion freeze: expected ExpiresAt=%d (no multiplier), got %d",
			expectedExpiry, cs.ExpiresAt)
	}
}

func TestCoercionSignal_NoAuthKeeperFallback(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	// Do NOT set zeroneAuthKeeper — should fall back to base duration
	ms := keeper.NewMsgServerImpl(k)
	pid, _ := proposeAndAccept(t, ms, ctx, humanAddr, agentAddr)

	params := k.GetParams(ctx)
	currentBlock := uint64(ctx.BlockHeight())
	expectedExpiry := currentBlock + params.CoercionReviewBlocks

	cs, err := k.HandleCoercionSignal(ctx, pid, humanAddr)
	if err != nil {
		t.Fatalf("HandleCoercionSignal failed: %v", err)
	}

	if cs.ExpiresAt != expectedExpiry {
		t.Errorf("no auth keeper fallback: expected ExpiresAt=%d (base), got %d",
			expectedExpiry, cs.ExpiresAt)
	}
}

// ---------- Mentorship CRUD Tests ----------

func TestMentorship_SetAndGet(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	m := &types.Mentorship{
		Id:                  "mentorship-1",
		MentorAddr:          humanAddr,
		MenteeAddr:          agentAddr,
		Domain:              "physics",
		Status:              "proposed",
		StartBlock:          0,
		DurationBlocks:      1000,
		GraduationThreshold: 20,
		GraduationClaimsReq: 5,
	}
	k.SetMentorship(ctx, m)

	got, found := k.GetMentorship(ctx, "mentorship-1")
	if !found {
		t.Fatal("mentorship not found")
	}
	if got.MentorAddr != humanAddr {
		t.Errorf("expected mentor %s, got %s", humanAddr, got.MentorAddr)
	}
	if got.Domain != "physics" {
		t.Errorf("expected domain physics, got %s", got.Domain)
	}
}

func TestMentorship_GetByMentor(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr, Status: "active",
	})
	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-2", MentorAddr: humanAddr, MenteeAddr: agent2Addr, Status: "active",
	})
	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-3", MentorAddr: agentAddr, MenteeAddr: agent2Addr, Status: "active",
	})

	mentorships := k.GetMentorshipsByMentor(ctx, humanAddr)
	if len(mentorships) != 2 {
		t.Errorf("expected 2 mentorships for mentor, got %d", len(mentorships))
	}
}

func TestMentorship_GetByMentee(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr, Status: "active",
	})

	mentorships := k.GetMentorshipsByMentee(ctx, agentAddr)
	if len(mentorships) != 1 {
		t.Errorf("expected 1 mentorship for mentee, got %d", len(mentorships))
	}
}

func TestMentorship_CountActive(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr, Status: "active",
	})
	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-2", MentorAddr: humanAddr, MenteeAddr: agent2Addr, Status: "graduated",
	})
	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-3", MentorAddr: humanAddr, MenteeAddr: agent3Addr, Status: "active",
	})

	count := k.CountActiveMentorshipsForMentor(ctx, humanAddr)
	if count != 2 {
		t.Errorf("expected 2 active mentorships, got %d", count)
	}
}

func TestMentorship_ActiveForMentee(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetMentorship(ctx, &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr, Status: "active",
	})

	m, found := k.GetActiveMentorshipForMentee(ctx, agentAddr)
	if !found {
		t.Fatal("expected active mentorship for mentee")
	}
	if m.Id != "m-1" {
		t.Errorf("expected m-1, got %s", m.Id)
	}

	_, found = k.GetActiveMentorshipForMentee(ctx, agent2Addr)
	if found {
		t.Error("expected no active mentorship for agent2")
	}
}

func TestMentorship_Delete(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	m := &types.Mentorship{
		Id: "m-1", MentorAddr: humanAddr, MenteeAddr: agentAddr, Status: "active",
	}
	k.SetMentorship(ctx, m)
	k.DeleteMentorship(ctx, m)

	_, found := k.GetMentorship(ctx, "m-1")
	if found {
		t.Error("expected mentorship to be deleted")
	}
}

// ---------- Mentorship Message Handler Tests ----------

func TestMentorship_ProposeAndAccept(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	resp, err := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor:         humanAddr,
		Mentee:         agentAddr,
		Domain:         "physics",
		DurationBlocks: 1000,
	})
	if err != nil {
		t.Fatalf("ProposeMentorship failed: %v", err)
	}
	if resp.MentorshipId == "" {
		t.Fatal("expected non-empty mentorship ID")
	}

	m, found := k.GetMentorship(ctx, resp.MentorshipId)
	if !found {
		t.Fatal("mentorship not found after proposal")
	}
	if m.Status != "proposed" {
		t.Errorf("expected proposed, got %s", m.Status)
	}

	_, err = ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee:       agentAddr,
		MentorshipId: resp.MentorshipId,
	})
	if err != nil {
		t.Fatalf("AcceptMentorship failed: %v", err)
	}

	m, _ = k.GetMentorship(ctx, resp.MentorshipId)
	if m.Status != "active" {
		t.Errorf("expected active, got %s", m.Status)
	}
	if m.StartBlock != uint64(ctx.BlockHeight()) {
		t.Errorf("expected start block %d, got %d", ctx.BlockHeight(), m.StartBlock)
	}
}

func TestMentorship_SelfMentorshipBlocked(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	_, err := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: humanAddr, Domain: "physics", DurationBlocks: 1000,
	})
	if err == nil {
		t.Fatal("expected error for self-mentorship")
	}
}

func TestMentorship_MaxMentorshipsEnforced(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	addrs := []string{agentAddr, agent2Addr, agent3Addr}
	for _, addr := range addrs {
		resp, err := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
			Mentor: humanAddr, Mentee: addr, Domain: "physics", DurationBlocks: 1000,
		})
		if err != nil {
			t.Fatalf("ProposeMentorship failed: %v", err)
		}
		_, err = ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
			Mentee: addr, MentorshipId: resp.MentorshipId,
		})
		if err != nil {
			t.Fatalf("AcceptMentorship failed: %v", err)
		}
	}

	fourthAddr := testAddr("agent4")
	_, err := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: fourthAddr, Domain: "physics", DurationBlocks: 1000,
	})
	if err == nil {
		t.Fatal("expected error for exceeding max mentorships")
	}
}

func TestMentorship_ManualGraduation(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 1000,
	})
	ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})

	_, err := ms.GraduateMentee(ctx, &types.MsgGraduateMentee{
		Mentor: humanAddr, MentorshipId: resp.MentorshipId,
	})
	if err != nil {
		t.Fatalf("GraduateMentee failed: %v", err)
	}

	m, _ := k.GetMentorship(ctx, resp.MentorshipId)
	if m.Status != "graduated" {
		t.Errorf("expected graduated, got %s", m.Status)
	}
}

func TestMentorship_EarlyTermination(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 1000,
	})
	ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})

	_, err := ms.EndMentorship(ctx, &types.MsgEndMentorship{
		Sender: agentAddr, MentorshipId: resp.MentorshipId,
	})
	if err != nil {
		t.Fatalf("EndMentorship failed: %v", err)
	}

	m, _ := k.GetMentorship(ctx, resp.MentorshipId)
	if m.Status != "terminated" {
		t.Errorf("expected terminated, got %s", m.Status)
	}
}

func TestMentorship_TerminationByOutsiderFails(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 1000,
	})
	ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})

	_, err := ms.EndMentorship(ctx, &types.MsgEndMentorship{
		Sender: outsiderAddr, MentorshipId: resp.MentorshipId,
	})
	if err == nil {
		t.Fatal("expected error for outsider ending mentorship")
	}
}

func TestMentorship_AcceptByWrongMenteeFails(t *testing.T) {
	ms, _, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 1000,
	})

	_, err := ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: outsiderAddr, MentorshipId: resp.MentorshipId,
	})
	if err == nil {
		t.Fatal("expected error for wrong mentee accepting")
	}
}

// ==================== FORMATION MATCHING TESTS ====================

func TestFormation_MatchSetAndGet(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	fm := &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		Score: 7500, ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	}
	k.SetFormationMatch(ctx, fm)

	got, found := k.GetFormationMatch(ctx, "match-1")
	if !found {
		t.Fatal("match not found")
	}
	if got.Score != 7500 {
		t.Errorf("expected score 7500, got %d", got.Score)
	}
}

func TestFormation_MatchingRunsAtInterval(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: humanAddr, Domains: []string{"physics"}, PreferredRole: "human",
		RegisteredAt: 50, ExpiresAt: 12000, Status: "active",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agentAddr, Domains: []string{"physics"}, PreferredRole: "agent",
		RegisteredAt: 60, ExpiresAt: 12000, Status: "active",
	})

	ctx99 := ctxAtHeight(ctx, 99)
	k.RunFormationMatching(ctx99)
	matches := k.GetAllFormationMatches(ctx99)
	if len(matches) != 0 {
		t.Errorf("expected no matches at non-interval block, got %d", len(matches))
	}

	ctx100 := ctxAtHeight(ctx, 100)
	k.RunFormationMatching(ctx100)
	matches = k.GetAllFormationMatches(ctx100)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match at interval block, got %d", len(matches))
	}
}

func TestFormation_CompatiblePairsMatched(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: humanAddr, Domains: []string{"physics", "math"}, PreferredRole: "human",
		RegisteredAt: 50, ExpiresAt: 12000, Status: "active",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agentAddr, Domains: []string{"physics"}, PreferredRole: "agent",
		RegisteredAt: 60, ExpiresAt: 12000, Status: "active",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agent2Addr, Domains: []string{"biology"}, PreferredRole: "agent",
		RegisteredAt: 70, ExpiresAt: 12000, Status: "active",
	})

	ctx100 := ctxAtHeight(ctx, 100)
	k.RunFormationMatching(ctx100)
	matches := k.GetAllFormationMatches(ctx100)

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	hasHuman := m.Addr1 == humanAddr || m.Addr2 == humanAddr
	hasAgent := m.Addr1 == agentAddr || m.Addr2 == agentAddr
	if !hasHuman || !hasAgent {
		t.Errorf("expected human-agent match, got %s and %s", m.Addr1, m.Addr2)
	}
	if m.Score == 0 {
		t.Error("expected non-zero score")
	}
}

func TestFormation_CappedAt200(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	for i := 0; i < 210; i++ {
		addr := testAddr(fmt.Sprintf("pool-%d", i))
		k.SetPoolEntry(ctx, &types.PoolEntry{
			Address: addr, Domains: []string{"physics"}, PreferredRole: "any",
			RegisteredAt: 50, ExpiresAt: 12000, Status: "active",
		})
	}

	ctx100 := ctxAtHeight(ctx, 100)
	k.RunFormationMatching(ctx100) // Should not panic
}

func TestFormation_ExpireMatches(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	k.SetFormationMatch(ctx, &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	})
	// Add pool entries so expiry can return them to unmatched state
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: humanAddr, Status: "active", MatchedWith: "match-1",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agentAddr, Status: "active", MatchedWith: "match-1",
	})

	k.ExpireFormationMatches(ctxAtHeight(ctx, 299))
	m, _ := k.GetFormationMatch(ctxAtHeight(ctx, 299), "match-1")
	if m.Status != "proposed" {
		t.Errorf("expected still proposed, got %s", m.Status)
	}

	k.ExpireFormationMatches(ctxAtHeight(ctx, 300))
	m, _ = k.GetFormationMatch(ctxAtHeight(ctx, 300), "match-1")
	if m.Status != "expired" {
		t.Errorf("expected expired, got %s", m.Status)
	}

	pe1, _ := k.GetPoolEntry(ctxAtHeight(ctx, 300), humanAddr)
	if pe1.MatchedWith != "" {
		t.Error("expected unmatched after expiry")
	}
}

// ==================== FORMATION MATCH HANDLER TESTS ====================

func TestFormation_BothAcceptFormsPartnership(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	k.SetFormationMatch(ctx, &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		Score: 8000, ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: humanAddr, Domains: []string{"physics"}, Status: "active", MatchedWith: "match-1",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agentAddr, Domains: []string{"physics"}, Status: "active", MatchedWith: "match-1",
	})

	_, err := ms.AcceptFormationMatch(ctx, &types.MsgAcceptFormationMatch{
		Accepter: humanAddr, MatchId: "match-1",
	})
	if err != nil {
		t.Fatalf("AcceptFormationMatch (addr1) failed: %v", err)
	}

	fm, _ := k.GetFormationMatch(ctx, "match-1")
	if !fm.Addr1Accepted {
		t.Error("expected addr1_accepted to be true")
	}
	if fm.Status != "proposed" {
		t.Error("expected status still proposed after one accept")
	}

	_, err = ms.AcceptFormationMatch(ctx, &types.MsgAcceptFormationMatch{
		Accepter: agentAddr, MatchId: "match-1",
	})
	if err != nil {
		t.Fatalf("AcceptFormationMatch (addr2) failed: %v", err)
	}

	fm, _ = k.GetFormationMatch(ctx, "match-1")
	if fm.Status != "accepted" {
		t.Errorf("expected accepted status, got %s", fm.Status)
	}

	partnerships := k.GetAllPartnerships(ctx)
	found := false
	for _, p := range partnerships {
		if (p.HumanAddr == humanAddr && p.AgentAddr == agentAddr) ||
			(p.HumanAddr == agentAddr && p.AgentAddr == humanAddr) {
			found = true
			if p.Status != types.StatusPending {
				t.Errorf("expected pending partnership, got %s", p.Status)
			}
		}
	}
	if !found {
		t.Error("expected partnership to be created after both accept")
	}

	_, inPool := k.GetPoolEntry(ctx, humanAddr)
	if inPool {
		t.Error("expected human to be removed from pool")
	}
	_, inPool = k.GetPoolEntry(ctx, agentAddr)
	if inPool {
		t.Error("expected agent to be removed from pool")
	}
}

func TestFormation_DeclineReturnsToPool(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	k.SetFormationMatch(ctx, &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		Score: 8000, ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: humanAddr, Domains: []string{"physics"}, Status: "active", MatchedWith: "match-1",
	})
	k.SetPoolEntry(ctx, &types.PoolEntry{
		Address: agentAddr, Domains: []string{"physics"}, Status: "active", MatchedWith: "match-1",
	})

	_, err := ms.DeclineFormationMatch(ctx, &types.MsgDeclineFormationMatch{
		Decliner: humanAddr, MatchId: "match-1",
	})
	if err != nil {
		t.Fatalf("DeclineFormationMatch failed: %v", err)
	}

	fm, _ := k.GetFormationMatch(ctx, "match-1")
	if fm.Status != "declined" {
		t.Errorf("expected declined, got %s", fm.Status)
	}

	pe1, _ := k.GetPoolEntry(ctx, humanAddr)
	if pe1.MatchedWith != "" {
		t.Error("expected human unmatched after decline")
	}
	pe2, _ := k.GetPoolEntry(ctx, agentAddr)
	if pe2.MatchedWith != "" {
		t.Error("expected agent unmatched after decline")
	}
}

func TestFormation_AcceptByOutsiderFails(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	k.SetFormationMatch(ctx, &types.FormationMatch{
		Id: "match-1", Addr1: humanAddr, Addr2: agentAddr,
		Score: 8000, ProposedAt: 100, ExpiresAt: 300, Status: "proposed",
	})

	_, err := ms.AcceptFormationMatch(ctx, &types.MsgAcceptFormationMatch{
		Accepter: outsiderAddr, MatchId: "match-1",
	})
	if err == nil {
		t.Fatal("expected error for outsider accepting match")
	}
	_ = k // suppress unused
}

// ==================== AUTO-GRADUATION TESTS ====================

func TestMentorship_AutoGraduation(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 500,
	})
	ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})

	ctx400 := ctxAtHeight(ctx, 100+400)
	k.AutoGraduateMentorships(ctx400)
	m, _ := k.GetMentorship(ctx400, resp.MentorshipId)
	if m.Status != "active" {
		t.Errorf("expected active before duration, got %s", m.Status)
	}

	ctx601 := ctxAtHeight(ctx, 100+501)
	k.AutoGraduateMentorships(ctx601)
	m, _ = k.GetMentorship(ctx601, resp.MentorshipId)
	if m.Status != "graduated" {
		t.Errorf("expected graduated after duration, got %s", m.Status)
	}
}

func TestMentorship_AutoProposePartnership(t *testing.T) {
	ms, k, ctx, _ := setupMsgServer(t)

	resp, _ := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 100,
	})
	ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})

	ms.GraduateMentee(ctx, &types.MsgGraduateMentee{
		Mentor: humanAddr, MentorshipId: resp.MentorshipId,
	})

	partnerships := k.GetAllPartnerships(ctx)
	found := false
	for _, p := range partnerships {
		if p.HumanAddr == humanAddr && p.AgentAddr == agentAddr {
			found = true
			if p.Status != types.StatusPending {
				t.Errorf("expected pending, got %s", p.Status)
			}
		}
	}
	if !found {
		t.Error("expected partnership to be auto-proposed on graduation")
	}
}

// ==================== FULL LIFECYCLE INTEGRATION TESTS ====================

func TestIntegration_MentorshipToPartnership(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	// 1. Propose mentorship
	resp, err := ms.ProposeMentorship(ctx, &types.MsgProposeMentorship{
		Mentor: humanAddr, Mentee: agentAddr, Domain: "physics", DurationBlocks: 100,
	})
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	// 2. Accept mentorship
	_, err = ms.AcceptMentorship(ctx, &types.MsgAcceptMentorship{
		Mentee: agentAddr, MentorshipId: resp.MentorshipId,
	})
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}

	m, _ := k.GetMentorship(ctx, resp.MentorshipId)
	if m.Status != "active" {
		t.Fatalf("expected active, got %s", m.Status)
	}

	// 3. Auto-graduate after duration
	ctx201 := ctxAtHeight(ctx, 201)
	k.AutoGraduateMentorships(ctx201)

	m, _ = k.GetMentorship(ctx201, resp.MentorshipId)
	if m.Status != "graduated" {
		t.Fatalf("expected graduated, got %s", m.Status)
	}

	// 4. Partnership should have been auto-proposed
	partnerships := k.GetAllPartnerships(ctx201)
	if len(partnerships) == 0 {
		t.Fatal("expected auto-proposed partnership")
	}

	p := partnerships[0]
	if p.Status != types.StatusPending {
		t.Errorf("expected pending, got %s", p.Status)
	}

	// 5. Accept the partnership
	_, err = ms.AcceptPartnership(ctx201, &types.MsgAcceptPartnership{
		Accepter:      agentAddr,
		PartnershipId: p.Id,
		Deposit:       "1000000",
	})
	if err != nil {
		t.Fatalf("accept partnership failed: %v", err)
	}

	p, _ = k.GetPartnership(ctx201, p.Id)
	if p.Status != types.StatusActive {
		t.Errorf("expected active partnership, got %s", p.Status)
	}
}

func TestIntegration_FormationPoolToPartnership(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)
	bk.setBalance(humanAddr, "uzrn", 10000000)
	bk.setBalance(agentAddr, "uzrn", 10000000)

	// 1. Both join formation pool
	_, err := ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner: humanAddr, Domains: []string{"physics", "math"}, PreferredRole: "human",
	})
	if err != nil {
		t.Fatalf("join pool (human) failed: %v", err)
	}

	_, err = ms.JoinFormationPool(ctx, &types.MsgJoinFormationPool{
		Joiner: agentAddr, Domains: []string{"physics"}, PreferredRole: "agent",
	})
	if err != nil {
		t.Fatalf("join pool (agent) failed: %v", err)
	}

	// 2. Matching runs at interval
	ctx100 := ctxAtHeight(ctx, 100)
	k.RunFormationMatching(ctx100)

	matches := k.GetAllFormationMatches(ctx100)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matchId := matches[0].Id

	// 3. Both accept
	_, err = ms.AcceptFormationMatch(ctx100, &types.MsgAcceptFormationMatch{
		Accepter: humanAddr, MatchId: matchId,
	})
	if err != nil {
		t.Fatalf("accept match (human) failed: %v", err)
	}

	_, err = ms.AcceptFormationMatch(ctx100, &types.MsgAcceptFormationMatch{
		Accepter: agentAddr, MatchId: matchId,
	})
	if err != nil {
		t.Fatalf("accept match (agent) failed: %v", err)
	}

	// 4. Match should be accepted, partnership proposed
	fm, _ := k.GetFormationMatch(ctx100, matchId)
	if fm.Status != "accepted" {
		t.Errorf("expected accepted, got %s", fm.Status)
	}

	partnerships := k.GetAllPartnerships(ctx100)
	if len(partnerships) == 0 {
		t.Fatal("expected partnership after both accept")
	}

	// 5. Pool entries should be removed
	_, inPool := k.GetPoolEntry(ctx100, humanAddr)
	if inPool {
		t.Error("expected human removed from pool")
	}
	_, inPool = k.GetPoolEntry(ctx100, agentAddr)
	if inPool {
		t.Error("expected agent removed from pool")
	}
}

// ---------- R31-5: LastParamUpdateHeight ----------

func TestSetParams_RecordsUpdateHeight(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	ctx = ctx.WithBlockHeight(42)
	params := types.DefaultParams()
	k.SetParams(ctx, params)

	height := k.GetLastParamUpdateHeight(ctx)
	require.Equal(t, uint64(42), height)

	ctx = ctx.WithBlockHeight(100)
	k.SetParams(ctx, params)
	height = k.GetLastParamUpdateHeight(ctx)
	require.Equal(t, uint64(100), height)
}

// ---------- R31-5: Formation Matching Cycle Reset ----------

func TestRunFormationMatching_ResetsOnParamChange(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	params := types.DefaultParams()
	params.FormationMatchIntervalBlocks = 10
	ctx = ctx.WithBlockHeight(1)
	k.SetParams(ctx, params)

	k.SetPoolEntry(ctx, &types.PoolEntry{Address: "addr1", Status: "active", Domains: []string{"physics"}, RegisteredAt: 1})
	k.SetPoolEntry(ctx, &types.PoolEntry{Address: "addr2", Status: "active", Domains: []string{"physics"}, PreferredRole: "agent", RegisteredAt: 1})

	// lastParamUpdate=1, so cycle fires when (block - 1) % 10 == 0 → block=11
	ctx = ctx.WithBlockHeight(11)
	k.RunFormationMatching(ctx)
	matches := k.GetAllFormationMatches(ctx)
	require.Len(t, matches, 1, "matching should run at block 11")

	// New pool entries
	k.SetPoolEntry(ctx, &types.PoolEntry{Address: "addr3", Status: "active", Domains: []string{"physics"}, RegisteredAt: 1})
	k.SetPoolEntry(ctx, &types.PoolEntry{Address: "addr4", Status: "active", Domains: []string{"physics"}, PreferredRole: "agent", RegisteredAt: 1})

	// Update params at block 13 — resets cycle
	ctx = ctx.WithBlockHeight(13)
	k.SetParams(ctx, params)

	// Block 15 (15-13=2, 2%10 != 0) — should NOT run
	ctx = ctx.WithBlockHeight(15)
	k.RunFormationMatching(ctx)
	require.Len(t, k.GetAllFormationMatches(ctx), 1, "matching should not run at block 15")

	// Block 23 (23-13=10, 10%10 == 0) — should run
	ctx = ctx.WithBlockHeight(23)
	k.RunFormationMatching(ctx)
	require.Len(t, k.GetAllFormationMatches(ctx), 2, "matching should run at block 23")
}
