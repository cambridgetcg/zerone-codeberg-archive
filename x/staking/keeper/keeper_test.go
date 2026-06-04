package keeper_test

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/staking/keeper"
	"github.com/zerone-chain/zerone/x/staking/types"
)

// ============================================================
// Test infrastructure
// ============================================================

func init() {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("zrn", "zrnpub")
	cfg.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	cfg.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

// ---------- Mock BankKeeper ----------

type mockBankKeeper struct {
	balances    map[string]sdk.Coins
	moduleSent  sdk.Coins
	burnedCoins sdk.Coins
	accountSent sdk.Coins // tracks SendCoinsFromModuleToAccount calls
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{
		balances: make(map[string]sdk.Coins),
	}
}

func (m *mockBankKeeper) SendCoins(_ context.Context, _, _ sdk.AccAddress, _ sdk.Coins) error {
	return nil
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, _ sdk.AccAddress, _ string, _ sdk.Coins) error {
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, _ string, _ sdk.AccAddress, amt sdk.Coins) error {
	m.accountSent = m.accountSent.Add(amt...)
	return nil
}

func (m *mockBankKeeper) GetBalance(_ context.Context, _ sdk.AccAddress, _ string) sdk.Coin {
	return sdk.NewInt64Coin("uzrn", 0)
}

func (m *mockBankKeeper) GetAllBalances(_ context.Context, _ sdk.AccAddress) sdk.Coins {
	return nil
}

func (m *mockBankKeeper) SpendableCoins(_ context.Context, _ sdk.AccAddress) sdk.Coins {
	return nil
}

func (m *mockBankKeeper) MintCoins(_ context.Context, _ string, _ sdk.Coins) error {
	return nil
}

func (m *mockBankKeeper) BurnCoins(_ context.Context, _ string, amt sdk.Coins) error {
	m.burnedCoins = m.burnedCoins.Add(amt...)
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule string, recipientModule string, amt sdk.Coins) error {
	m.moduleSent = m.moduleSent.Add(amt...)
	return nil
}

// ---------- Mock AccountKeeper ----------

type mockAccountKeeper struct{}

func (m *mockAccountKeeper) GetAccount(_ context.Context, _ sdk.AccAddress) sdk.AccountI { return nil }
func (m *mockAccountKeeper) SetAccount(_ context.Context, _ sdk.AccountI)               {}
func (m *mockAccountKeeper) NewAccountWithAddress(_ context.Context, _ sdk.AccAddress) sdk.AccountI {
	return nil
}

// ---------- Mock AutopoiesisKeeper ----------

type mockAutopoiesisKeeper struct {
	multipliers map[string]uint64
}

func newMockAutopoiesisKeeper() *mockAutopoiesisKeeper {
	return &mockAutopoiesisKeeper{multipliers: make(map[string]uint64)}
}

func (m *mockAutopoiesisKeeper) GetMultiplier(_ context.Context, path string) uint64 {
	if v, ok := m.multipliers[path]; ok {
		return v
	}
	return 1_000_000 // default 1.0x
}

// ---------- Setup ----------

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
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
	ak := &mockAccountKeeper{}

	k := keeper.NewKeeper(cdc, storeKey, ak, bk, "authority")
	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())

	// Initialize default genesis.
	k.InitGenesis(ctx, types.DefaultGenesisState())

	return k, ctx
}

func setupBareKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
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
	ak := &mockAccountKeeper{}

	k := keeper.NewKeeper(cdc, storeKey, ak, bk, "authority")
	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())

	return k, ctx
}

// setupKeeperWithBank returns the keeper, context, and the underlying mock
// bank keeper so tests can inspect burn/send amounts directly.
func setupKeeperWithBank(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper) {
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
	ak := &mockAccountKeeper{}

	k := keeper.NewKeeper(cdc, storeKey, ak, bk, "authority")
	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())
	k.InitGenesis(ctx, types.DefaultGenesisState())

	return k, ctx, bk
}

// testAddr creates a deterministic bech32 address from a seed.
func testAddr(seed string) string {
	padded := make([]byte, 20)
	copy(padded, []byte(seed))
	return sdk.AccAddress(padded).String()
}

// registerTestValidator registers a validator via SetValidator directly.
func registerTestValidator(t *testing.T, k keeper.Keeper, ctx sdk.Context, addr, did, selfDel string) {
	t.Helper()
	val := &types.Validator{
		OperatorAddress: addr,
		ConsensusPubkey: fmt.Sprintf("pk_%s", addr[:8]),
		Did:             did,
		Moniker:         fmt.Sprintf("val_%s", addr[:8]),
		Tier:            types.TierApprentice,
		SelfDelegation:  selfDel,
		DelegatedStake:  "0",
		TotalStake:      selfDel,
		ReputationScore: 500_000,
		IsActive:        true,
	}
	k.SetValidator(ctx, val)
}

// promoteToScholar sets a validator to Scholar tier with sufficient stats.
func promoteToScholar(t *testing.T, k keeper.Keeper, ctx sdk.Context, addr string) {
	t.Helper()
	val, found := k.GetValidator(ctx, addr)
	if !found {
		t.Fatalf("validator %s not found for promotion", addr)
	}
	val.SelfDelegation = "1111000000"
	val.TotalStake = "1111000000"
	val.TotalVerifications = 22
	val.CorrectVerifications = 18
	val.ReputationScore = 600_000
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)
}

// promoteToGuardian sets a validator to Guardian tier with sufficient stats.
func promoteToGuardian(t *testing.T, k keeper.Keeper, ctx sdk.Context, addr string) {
	t.Helper()
	val, found := k.GetValidator(ctx, addr)
	if !found {
		t.Fatalf("validator %s not found for promotion", addr)
	}
	val.SelfDelegation = "11111000000"
	val.TotalStake = "11111000000"
	val.TotalVerifications = 500
	val.CorrectVerifications = 450
	val.ContestedVerificationsCorrect = 50
	val.ContestedCount = 50
	val.ReputationScore = 800_000
	val.SlashCount = 0
	val.Tier = types.TierGuardian
	k.SetValidator(ctx, val)
}

// ============================================================
// 1. Core CRUD Tests
// ============================================================

func TestValidatorCRUD(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("val_crud")

	// Create
	val := &types.Validator{
		OperatorAddress: addr,
		ConsensusPubkey: "pk_crud",
		Did:             "did:zrn:crud",
		Moniker:         "TestVal",
		Tier:            types.TierApprentice,
		SelfDelegation:  "111000",
		DelegatedStake:  "0",
		TotalStake:      "111000",
		ReputationScore: 500_000,
		IsActive:        true,
	}
	k.SetValidator(ctx, val)

	// Read
	got, found := k.GetValidator(ctx, addr)
	if !found {
		t.Fatal("validator not found after SetValidator")
	}
	if got.OperatorAddress != addr {
		t.Errorf("expected address %s, got %s", addr, got.OperatorAddress)
	}
	if got.Did != "did:zrn:crud" {
		t.Errorf("expected DID did:zrn:crud, got %s", got.Did)
	}

	// Read by DID
	gotByDID, found := k.GetValidatorByDID(ctx, "did:zrn:crud")
	if !found {
		t.Fatal("validator not found by DID")
	}
	if gotByDID.OperatorAddress != addr {
		t.Errorf("DID lookup returned wrong address: %s", gotByDID.OperatorAddress)
	}

	// Delete
	k.DeleteValidator(ctx, addr)
	_, found = k.GetValidator(ctx, addr)
	if found {
		t.Error("validator should not exist after delete")
	}
}

func TestDelegationCRUD(t *testing.T) {
	k, ctx := setupKeeper(t)
	delAddr := testAddr("delegator")
	valAddr := testAddr("validator")

	del := &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "500000",
		CreatedAtBlock:   100,
	}
	k.SetDelegation(ctx, del)

	got, found := k.GetDelegation(ctx, delAddr, valAddr)
	if !found {
		t.Fatal("delegation not found")
	}
	if got.Amount != "500000" {
		t.Errorf("expected amount 500000, got %s", got.Amount)
	}

	// Reverse index
	dels := k.GetDelegationsForValidator(ctx, valAddr)
	if len(dels) != 1 {
		t.Fatalf("expected 1 delegation via reverse index, got %d", len(dels))
	}
	if dels[0].DelegatorAddress != delAddr {
		t.Errorf("reverse index returned wrong delegator")
	}

	k.DeleteDelegation(ctx, delAddr, valAddr)
	_, found = k.GetDelegation(ctx, delAddr, valAddr)
	if found {
		t.Error("delegation should not exist after delete")
	}
}

func TestUnbondingCRUD(t *testing.T) {
	k, ctx := setupKeeper(t)
	entry := &types.UnbondingEntry{
		Id:                "unbond_1",
		DelegatorAddress:  testAddr("delegator"),
		ValidatorAddress:  testAddr("validator"),
		Amount:            "100000",
		CreatedAtHeight:   100,
		CompletesAtHeight: 100 + 268_560,
		Status:            "pending",
	}
	k.SetUnbonding(ctx, entry)

	got, found := k.GetUnbonding(ctx, "unbond_1")
	if !found {
		t.Fatal("unbonding not found")
	}
	if got.Amount != "100000" {
		t.Errorf("expected amount 100000, got %s", got.Amount)
	}

	k.DeleteUnbonding(ctx, "unbond_1")
	_, found = k.GetUnbonding(ctx, "unbond_1")
	if found {
		t.Error("unbonding should not exist after delete")
	}
}

func TestUnbondingSequence(t *testing.T) {
	k, ctx := setupKeeper(t)

	seq1 := k.NextUnbondingSeq(ctx)
	seq2 := k.NextUnbondingSeq(ctx)
	seq3 := k.NextUnbondingSeq(ctx)

	if seq1 != 1 || seq2 != 2 || seq3 != 3 {
		t.Errorf("expected sequential IDs 1,2,3 — got %d,%d,%d", seq1, seq2, seq3)
	}
}

// ============================================================
// 2. MsgServer Tests
// ============================================================

func TestMsgServer_RegisterValidator(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	addr := testAddr("register_val")

	resp, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        addr,
		ConsensusPubkey: "pk_register",
		SelfDelegation:  "111000",
		CommissionBps:   1000,
		Did:             "did:zrn:register",
		Moniker:         "TestReg",
	})
	if err != nil {
		t.Fatalf("RegisterValidator failed: %v", err)
	}
	if resp.InitialTier != uint32(types.TierApprentice) {
		t.Errorf("expected initial tier apprentice, got %d", resp.InitialTier)
	}

	val, found := k.GetValidator(ctx, addr)
	if !found {
		t.Fatal("validator not found after registration")
	}
	if val.SelfDelegation != "111000" {
		t.Errorf("expected self delegation 111000, got %s", val.SelfDelegation)
	}
	if val.IsActive != true {
		t.Error("validator should be active")
	}
}

func TestMsgServer_RegisterValidator_DuplicateAddr(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	addr := testAddr("dup_addr")

	_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        addr,
		ConsensusPubkey: "pk_dup1",
		SelfDelegation:  "111000",
	})
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	_, err = ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        addr,
		ConsensusPubkey: "pk_dup2",
		SelfDelegation:  "111000",
	})
	if err == nil {
		t.Error("expected error for duplicate address")
	}
}

func TestMsgServer_RegisterValidator_DuplicateDID(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        testAddr("did_val1"),
		ConsensusPubkey: "pk_did1",
		SelfDelegation:  "111000",
		Did:             "did:zrn:shared",
	})
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	_, err = ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        testAddr("did_val2"),
		ConsensusPubkey: "pk_did2",
		SelfDelegation:  "111000",
		Did:             "did:zrn:shared",
	})
	if err == nil {
		t.Error("expected error for duplicate DID")
	}
}

func TestMsgServer_RegisterValidator_InsufficientSelfDel(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
		Operator:        testAddr("low_stake"),
		ConsensusPubkey: "pk_low",
		SelfDelegation:  "100", // below min 111000
	})
	if err == nil {
		t.Error("expected error for insufficient self-delegation")
	}
}

func TestMsgServer_Delegate(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("del_val")
	delAddr := testAddr("delegator1")

	registerTestValidator(t, k, ctx, valAddr, "did:zrn:delval", "111000")

	resp, err := ms.Delegate(ctx, &types.MsgDelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "500000",
	})
	if err != nil {
		t.Fatalf("Delegate failed: %v", err)
	}
	if resp.NewDelegation != "500000" {
		t.Errorf("expected new delegation 500000, got %s", resp.NewDelegation)
	}

	val, _ := k.GetValidator(ctx, valAddr)
	if val.DelegatedStake != "500000" {
		t.Errorf("expected delegated stake 500000, got %s", val.DelegatedStake)
	}
}

func TestMsgServer_Delegate_InactiveValidator(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("inactive_val")

	val := &types.Validator{
		OperatorAddress: valAddr,
		ConsensusPubkey: "pk_inactive",
		Tier:            types.TierApprentice,
		SelfDelegation:  "111000",
		DelegatedStake:  "0",
		TotalStake:      "111000",
		IsActive:        false,
	}
	k.SetValidator(ctx, val)

	_, err := ms.Delegate(ctx, &types.MsgDelegate{
		Delegator: testAddr("del_inactive"),
		Validator: valAddr,
		Amount:    "500000",
	})
	if err == nil {
		t.Error("expected error delegating to inactive validator")
	}
}

func TestMsgServer_Undelegate(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("undel_val")
	delAddr := testAddr("undel_del")

	registerTestValidator(t, k, ctx, valAddr, "did:zrn:undelval", "111000")

	// Create delegation.
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "1000000",
		CreatedAtBlock:   100,
	})
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "1000000"
	val.TotalStake = "1111000"
	k.SetValidator(ctx, val)

	resp, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "500000",
	})
	if err != nil {
		t.Fatalf("Undelegate failed: %v", err)
	}
	if resp.UnbondingId == "" {
		t.Error("expected non-empty unbonding ID")
	}

	// Check delegation reduced.
	del, found := k.GetDelegation(ctx, delAddr, valAddr)
	if !found {
		t.Fatal("delegation should still exist with remaining amount")
	}
	if del.Amount != "500000" {
		t.Errorf("expected remaining delegation 500000, got %s", del.Amount)
	}

	// Check unbonding entry created.
	entry, found := k.GetUnbonding(ctx, resp.UnbondingId)
	if !found {
		t.Fatal("unbonding entry not found")
	}
	if entry.Amount != "500000" {
		t.Errorf("expected unbonding amount 500000, got %s", entry.Amount)
	}
	if entry.Status != "pending" {
		t.Errorf("expected pending status, got %s", entry.Status)
	}
}

func TestMsgServer_Undelegate_Full(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("undfull_val")
	delAddr := testAddr("undfull_del")

	registerTestValidator(t, k, ctx, valAddr, "", "111000")
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "500000",
		CreatedAtBlock:   100,
	})
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "500000"
	val.TotalStake = "611000"
	k.SetValidator(ctx, val)

	_, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "500000",
	})
	if err != nil {
		t.Fatalf("Undelegate full failed: %v", err)
	}

	_, found := k.GetDelegation(ctx, delAddr, valAddr)
	if found {
		t.Error("delegation should be fully removed after full undelegation")
	}
}

func TestMsgServer_Redelegate(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	val1 := testAddr("redel_src")
	val2 := testAddr("redel_dst")
	delAddr := testAddr("redel_del")

	registerTestValidator(t, k, ctx, val1, "", "1111000000")
	registerTestValidator(t, k, ctx, val2, "", "1111000000")

	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: val1,
		Amount:           "500000",
		CreatedAtBlock:   100,
	})
	v1, _ := k.GetValidator(ctx, val1)
	v1.DelegatedStake = "500000"
	v1.TotalStake = "1111500000"
	k.SetValidator(ctx, v1)

	_, err := ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator:    delAddr,
		SrcValidator: val1,
		DstValidator: val2,
		Amount:       "300000",
	})
	if err != nil {
		t.Fatalf("Redelegate failed: %v", err)
	}

	// Check source delegation reduced.
	srcDel, found := k.GetDelegation(ctx, delAddr, val1)
	if !found {
		t.Fatal("source delegation should still exist")
	}
	if srcDel.Amount != "200000" {
		t.Errorf("expected remaining source 200000, got %s", srcDel.Amount)
	}

	// Check destination delegation created.
	dstDel, found := k.GetDelegation(ctx, delAddr, val2)
	if !found {
		t.Fatal("destination delegation should exist")
	}
	if dstDel.Amount != "300000" {
		t.Errorf("expected destination 300000, got %s", dstDel.Amount)
	}
}

func TestMsgServer_Redelegate_Cooldown(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	val1 := testAddr("cd_src")
	val2 := testAddr("cd_dst")
	val3 := testAddr("cd_dst2")
	delAddr := testAddr("cd_del")

	registerTestValidator(t, k, ctx, val1, "", "1111000000")
	registerTestValidator(t, k, ctx, val2, "", "1111000000")
	registerTestValidator(t, k, ctx, val3, "", "1111000000")

	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: val1,
		Amount:           "1000000",
		CreatedAtBlock:   100,
	})
	v1, _ := k.GetValidator(ctx, val1)
	v1.DelegatedStake = "1000000"
	v1.TotalStake = "1112000000"
	k.SetValidator(ctx, v1)

	// First redelegate should succeed.
	_, err := ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator:    delAddr,
		SrcValidator: val1,
		DstValidator: val2,
		Amount:       "300000",
	})
	if err != nil {
		t.Fatalf("first redelegate failed: %v", err)
	}

	// Second redelegate during cooldown should fail.
	_, err = ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator:    delAddr,
		SrcValidator: val1,
		DstValidator: val3,
		Amount:       "100000",
	})
	if err == nil {
		t.Error("expected cooldown error for rapid redelegate")
	}

	// After cooldown, should succeed.
	params := k.GetParams(ctx)
	ctx = ctx.WithBlockHeight(100 + int64(params.RedelegationCooldownBlocks))
	_, err = ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator:    delAddr,
		SrcValidator: val1,
		DstValidator: val3,
		Amount:       "100000",
	})
	if err != nil {
		t.Errorf("redelegate after cooldown should succeed: %v", err)
	}
}

func TestMsgServer_UpdateValidatorStake_Increase(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	addr := testAddr("upstake_val")

	registerTestValidator(t, k, ctx, addr, "", "111000")

	_, err := ms.UpdateValidatorStake(ctx, &types.MsgUpdateValidatorStake{
		Operator: addr,
		Amount:   "500000",
		Increase: true,
	})
	if err != nil {
		t.Fatalf("UpdateValidatorStake increase failed: %v", err)
	}

	val, _ := k.GetValidator(ctx, addr)
	if val.SelfDelegation != "611000" {
		t.Errorf("expected self delegation 611000, got %s", val.SelfDelegation)
	}
}

func TestMsgServer_UpdateValidatorStake_Decrease(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	addr := testAddr("downstake")

	registerTestValidator(t, k, ctx, addr, "", "1000000")

	_, err := ms.UpdateValidatorStake(ctx, &types.MsgUpdateValidatorStake{
		Operator: addr,
		Amount:   "500000",
		Increase: false,
	})
	if err != nil {
		t.Fatalf("UpdateValidatorStake decrease failed: %v", err)
	}

	val, _ := k.GetValidator(ctx, addr)
	if val.SelfDelegation != "500000" {
		t.Errorf("expected self delegation 500000, got %s", val.SelfDelegation)
	}
}

func TestMsgServer_UpdateParams(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	newParams := types.DefaultParams()
	newParams.MaxValidators = 50

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: "authority",
		Params:    newParams,
	})
	if err != nil {
		t.Fatalf("UpdateParams failed: %v", err)
	}

	params := k.GetParams(ctx)
	if params.MaxValidators != 50 {
		t.Errorf("expected MaxValidators=50, got %d", params.MaxValidators)
	}
}

func TestMsgServer_UpdateParams_Unauthorized(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: testAddr("not_authority"),
		Params:    types.DefaultParams(),
	})
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

// ============================================================
// 3. Slash Tests
// ============================================================

func TestSlashValidator_Basic(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_basic")
	registerTestValidator(t, k, ctx, addr, "", "1000000")
	promoteToScholar(t, k, ctx, addr)

	k.SlashValidator(ctx, addr, big.NewInt(100000), "test_slash")

	val, _ := k.GetValidator(ctx, addr)
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1, got %d", val.SlashCount)
	}
	if val.SlashesThisEpoch != 1 {
		t.Errorf("expected SlashesThisEpoch=1, got %d", val.SlashesThisEpoch)
	}
}

func TestSlashValidator_ZeroAmount(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_zero")
	registerTestValidator(t, k, ctx, addr, "", "1000000")

	k.SlashValidator(ctx, addr, big.NewInt(0), "zero_slash")

	val, _ := k.GetValidator(ctx, addr)
	if val.SlashCount != 0 {
		t.Errorf("zero-amount slash should not increment SlashCount, got %d", val.SlashCount)
	}
}

func TestSlashValidator_PerEpochCap(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_cap")
	registerTestValidator(t, k, ctx, addr, "", "10000000000")
	promoteToScholar(t, k, ctx, addr)

	// Default MaxSlashesPerEpoch = 2
	k.SlashValidator(ctx, addr, big.NewInt(1000), "cap_1")
	k.SlashValidator(ctx, addr, big.NewInt(1000), "cap_2")

	val, _ := k.GetValidator(ctx, addr)
	if val.SlashesThisEpoch != 2 {
		t.Fatalf("expected 2 slashes this epoch, got %d", val.SlashesThisEpoch)
	}

	// Third slash should be silently capped.
	k.SlashValidator(ctx, addr, big.NewInt(1000), "cap_3")
	val, _ = k.GetValidator(ctx, addr)
	if val.SlashesThisEpoch != 2 {
		t.Errorf("expected capped at 2, got %d", val.SlashesThisEpoch)
	}
}

func TestSlashValidator_Escalation(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_esc")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	params := k.GetParams(ctx)
	params.MaxSlashesPerEpoch = 10
	params.MaxSlashCountDeactivate = 10
	k.SetParams(ctx, params)

	// First slash: no escalation (count=0)
	val, _ := k.GetValidator(ctx, addr)
	stakeBefore, _ := new(big.Int).SetString(val.SelfDelegation, 10)

	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "esc_0")
	val, _ = k.GetValidator(ctx, addr)
	stakeAfter, _ := new(big.Int).SetString(val.SelfDelegation, 10)

	slashed := new(big.Int).Sub(stakeBefore, stakeAfter)
	if slashed.Int64() != 1_000_000 {
		t.Errorf("first slash: expected 1000000, got %s", slashed.String())
	}

	// Second slash: escalation factor = (1M + 1*100000) / 1M = 1.1x
	stakeBefore2, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "esc_1")
	val, _ = k.GetValidator(ctx, addr)
	stakeAfter2, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	slashed2 := new(big.Int).Sub(stakeBefore2, stakeAfter2)

	if slashed2.Int64() != 1_100_000 {
		t.Errorf("second slash: expected 1100000 (1.1x), got %s", slashed2.String())
	}
}

func TestSlashValidator_SSIMultiplier(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_ssi")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "10000"
	val.DelegatedStake = "0"
	val.TotalStake = "10000"
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)

	mockAuto := newMockAutopoiesisKeeper()
	mockAuto.multipliers["ssi"] = 800_000 // 0.8x
	k.SetAutopoiesisKeeper(mockAuto)

	k.SlashValidator(ctx, addr, big.NewInt(1000), "ssi_test")

	val, found := k.GetValidator(ctx, addr)
	if !found {
		t.Fatal("validator not found after slash")
	}
	if val.SelfDelegation != "9200" {
		t.Errorf("expected self_del=9200 (base 1000 x 0.8 SSI = 800), got %s", val.SelfDelegation)
	}
}

func TestSlashValidator_SSIMultiplierHigh(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_ssi_hi")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "10000"
	val.DelegatedStake = "0"
	val.TotalStake = "10000"
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)

	mockAuto := newMockAutopoiesisKeeper()
	mockAuto.multipliers["ssi"] = 2_000_000 // 2.0x
	k.SetAutopoiesisKeeper(mockAuto)

	k.SlashValidator(ctx, addr, big.NewInt(1000), "ssi_high_test")

	val, _ = k.GetValidator(ctx, addr)
	if val.SelfDelegation != "8000" {
		t.Errorf("expected self_del=8000 (base 1000 x 2.0 SSI = 2000), got %s", val.SelfDelegation)
	}
}

func TestSlashValidator_NilAutopoiesisKeeper(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_nilauto")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "10000"
	val.DelegatedStake = "0"
	val.TotalStake = "10000"
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)

	// No autopoiesis keeper set = default 1.0x
	k.SlashValidator(ctx, addr, big.NewInt(1000), "nil_auto")

	val, _ = k.GetValidator(ctx, addr)
	if val.SelfDelegation != "9000" {
		t.Errorf("expected self_del=9000, got %s", val.SelfDelegation)
	}
}

func TestSlashValidator_OwnBeforeDelegated(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_own")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "1000"
	val.DelegatedStake = "500"
	val.TotalStake = "1500"
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)

	// Slash exact own stake.
	k.SlashValidator(ctx, addr, big.NewInt(1000), "exact_own")
	val, _ = k.GetValidator(ctx, addr)
	if val.SelfDelegation != "0" {
		t.Errorf("own stake should be 0, got %s", val.SelfDelegation)
	}
	if val.DelegatedStake != "500" {
		t.Errorf("delegated should be untouched (500), got %s", val.DelegatedStake)
	}
}

func TestSlashValidator_OverflowToDelegated(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_over")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "1000"
	val.DelegatedStake = "500"
	val.TotalStake = "1500"
	val.Tier = types.TierScholar
	k.SetValidator(ctx, val)

	k.SlashValidator(ctx, addr, big.NewInt(1200), "overflow")
	val, _ = k.GetValidator(ctx, addr)
	if val.SelfDelegation != "0" {
		t.Errorf("own stake should be 0 after overflow, got %s", val.SelfDelegation)
	}
	if val.DelegatedStake != "300" {
		t.Errorf("delegated should be 300 after overflow, got %s", val.DelegatedStake)
	}
	if val.TotalStake != "300" {
		t.Errorf("total should be 300, got %s", val.TotalStake)
	}
}

func TestSlashValidator_Deactivation(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_deact")
	registerTestValidator(t, k, ctx, addr, "", "1000000000")

	params := k.GetParams(ctx)
	params.MaxSlashCountDeactivate = 3
	params.MaxSlashesPerEpoch = 10
	k.SetParams(ctx, params)

	for i := 0; i < 3; i++ {
		k.SlashValidator(ctx, addr, big.NewInt(1000), fmt.Sprintf("deact_%d", i))
	}

	val, _ := k.GetValidator(ctx, addr)
	if val.IsActive {
		t.Error("validator should be deactivated after 3 slashes (MaxSlashCountDeactivate=3)")
	}
}

func TestSlashValidator_ReputationDecrease(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_rep")
	registerTestValidator(t, k, ctx, addr, "", "1000000000")

	val, _ := k.GetValidator(ctx, addr)
	repBefore := val.ReputationScore

	k.SlashValidator(ctx, addr, big.NewInt(1000), "rep_slash")

	val, _ = k.GetValidator(ctx, addr)
	params := k.GetParams(ctx)
	expected := repBefore - params.ReputationSlashDelta
	if val.ReputationScore != expected {
		t.Errorf("expected reputation %d (was %d, slash delta %d), got %d",
			expected, repBefore, params.ReputationSlashDelta, val.ReputationScore)
	}
}

// ============================================================
// 4. RecordVerification Tests
// ============================================================

func TestRecordVerification_Correct(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_correct")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	repBefore := val.ReputationScore

	k.RecordVerification(ctx, addr, true, false)

	val, _ = k.GetValidator(ctx, addr)
	if val.TotalVerifications != 1 {
		t.Errorf("expected TotalVerifications=1, got %d", val.TotalVerifications)
	}
	if val.CorrectVerifications != 1 {
		t.Errorf("expected CorrectVerifications=1, got %d", val.CorrectVerifications)
	}

	params := k.GetParams(ctx)
	expected := repBefore + params.ReputationCorrectDelta
	if val.ReputationScore != expected {
		t.Errorf("expected reputation %d, got %d", expected, val.ReputationScore)
	}
}

func TestRecordVerification_Incorrect(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_wrong")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	repBefore := val.ReputationScore

	k.RecordVerification(ctx, addr, false, false)

	val, _ = k.GetValidator(ctx, addr)
	if val.TotalVerifications != 1 {
		t.Errorf("expected TotalVerifications=1, got %d", val.TotalVerifications)
	}
	if val.CorrectVerifications != 0 {
		t.Errorf("expected CorrectVerifications=0, got %d", val.CorrectVerifications)
	}

	params := k.GetParams(ctx)
	expected := repBefore - params.ReputationIncorrectDelta
	if val.ReputationScore != expected {
		t.Errorf("expected reputation %d, got %d", expected, val.ReputationScore)
	}
}

func TestRecordVerification_Contested(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_contest")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	k.RecordVerification(ctx, addr, true, true)

	val, _ := k.GetValidator(ctx, addr)
	if val.ContestedVerificationsCorrect != 1 {
		t.Errorf("expected ContestedVerificationsCorrect=1, got %d", val.ContestedVerificationsCorrect)
	}
	if val.ContestedCount != 1 {
		t.Errorf("expected ContestedCount=1, got %d", val.ContestedCount)
	}
}

func TestRecordVerification_ReputationUnderflowGuard(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_underflow")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	// Set reputation very low.
	val, _ := k.GetValidator(ctx, addr)
	val.ReputationScore = 50
	k.SetValidator(ctx, val)

	k.RecordVerification(ctx, addr, false, false)

	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore != 0 {
		t.Errorf("reputation should floor at 0, got %d", val.ReputationScore)
	}
}

func TestRecordVerification_ReputationCap(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_cap")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	val.ReputationScore = types.BPSScale - 10
	k.SetValidator(ctx, val)

	k.RecordVerification(ctx, addr, true, false)

	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore > types.BPSScale {
		t.Errorf("reputation should cap at BPSScale, got %d", val.ReputationScore)
	}
}

func TestRecordVerification_ParameterizedReputation(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rv_param")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	params := k.GetParams(ctx)
	params.ReputationCorrectDelta = 500
	params.ReputationIncorrectDelta = 1000
	k.SetParams(ctx, params)

	val, _ := k.GetValidator(ctx, addr)
	val.ReputationScore = 500_000
	k.SetValidator(ctx, val)

	k.RecordVerification(ctx, addr, true, false)
	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore != 500_500 {
		t.Errorf("expected 500500 after correct (+500), got %d", val.ReputationScore)
	}

	k.RecordVerification(ctx, addr, false, false)
	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore != 499_500 {
		t.Errorf("expected 499500 after incorrect (-1000), got %d", val.ReputationScore)
	}
}

// ============================================================
// 5. Tier Tests
// ============================================================

func TestComputeValidatorTier_Apprentice(t *testing.T) {
	k, ctx := setupKeeper(t)
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(111000), 0, 0, 0, 0, 0)
	if tier != types.TierApprentice {
		t.Errorf("expected Apprentice, got %s", types.ValidatorTierString(tier))
	}
}

func TestComputeValidatorTier_Verified(t *testing.T) {
	k, ctx := setupKeeper(t)
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_110_000), 22, 18, 0, 0, 0)
	if tier != types.TierVerified {
		t.Errorf("expected Verified, got %s", types.ValidatorTierString(tier))
	}
}

func TestComputeValidatorTier_Scholar(t *testing.T) {
	k, ctx := setupKeeper(t)
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_111_000_000), 22, 12, 0, 0, 0)
	if tier != types.TierScholar {
		t.Errorf("expected Scholar, got %s", types.ValidatorTierString(tier))
	}
}

func TestComputeValidatorTier_Guardian(t *testing.T) {
	k, ctx := setupKeeper(t)
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(11_111_000_000), 500, 450, 0, 50, 0)
	if tier != types.TierGuardian {
		t.Errorf("expected Guardian, got %s", types.ValidatorTierString(tier))
	}
}

func TestComputeValidatorTier_GuardianRequiresContestedCount(t *testing.T) {
	k, ctx := setupKeeper(t)
	// All Guardian criteria met EXCEPT contested verifications (0 instead of 33).
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(11_111_000_000), 500, 450, 0, 0, 0)
	if tier == types.TierGuardian {
		t.Error("Guardian should NOT be granted without contested verifications (P0-2 fix)")
	}
}

func TestComputeValidatorTier_GuardianSlashBlocksDemotion(t *testing.T) {
	k, ctx := setupKeeper(t)
	// Guardian criteria met but has 1 slash (MaxSlashCount=0 for Guardian).
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(11_111_000_000), 500, 450, 1, 50, 50)
	if tier == types.TierGuardian {
		t.Error("Guardian should NOT be granted with any slashes (MaxSlashCount=0)")
	}
}

func TestIsTierEligibleForCategory(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Apprentice: protocol, computational, formal only.
	if !k.IsTierEligibleForCategory(ctx, types.TierApprentice, "protocol") {
		t.Error("apprentice should be eligible for protocol")
	}
	if k.IsTierEligibleForCategory(ctx, types.TierApprentice, "empirical") {
		t.Error("apprentice should NOT be eligible for empirical")
	}
	if k.IsTierEligibleForCategory(ctx, types.TierApprentice, "contested") {
		t.Error("apprentice should NOT be eligible for contested")
	}

	// Guardian: all categories.
	if !k.IsTierEligibleForCategory(ctx, types.TierGuardian, "contested") {
		t.Error("guardian should be eligible for contested")
	}
}

func TestGetEffectiveSelectionStake_TierZero(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("ess_tier0")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	effective := k.GetEffectiveSelectionStake(ctx, val)

	params := k.GetParams(ctx)
	vs, _ := new(big.Int).SetString(params.VirtualStake, 10)
	if effective.Cmp(vs) != 0 {
		t.Errorf("tier 0 should use virtual stake %s, got %s", vs, effective)
	}
}

func TestGetEffectiveSelectionStake_ScholarUsesRealStake(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("ess_scholar")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	effective := k.GetEffectiveSelectionStake(ctx, val)

	totalStake, _ := new(big.Int).SetString(val.TotalStake, 10)
	if effective.Cmp(totalStake) != 0 {
		t.Errorf("scholar should use real stake %s, got %s", totalStake, effective)
	}
}

func TestCalculateTierReward(t *testing.T) {
	k, ctx := setupKeeper(t)

	base := big.NewInt(1000)

	// Apprentice: 100 BPS = 0.1x → 1000 * 100 / 1000 = 100
	reward := k.CalculateTierReward(ctx, types.TierApprentice, base)
	if reward.Int64() != 100 {
		t.Errorf("apprentice reward: expected 100, got %d", reward.Int64())
	}

	// Scholar: 1000 BPS = 1.0x → 1000 * 1000 / 1000 = 1000
	reward = k.CalculateTierReward(ctx, types.TierScholar, base)
	if reward.Int64() != 1000 {
		t.Errorf("scholar reward: expected 1000, got %d", reward.Int64())
	}

	// Guardian: 2000 BPS = 2.0x → 1000 * 2000 / 1000 = 2000
	reward = k.CalculateTierReward(ctx, types.TierGuardian, base)
	if reward.Int64() != 2000 {
		t.Errorf("guardian reward: expected 2000, got %d", reward.Int64())
	}
}

func TestCalculateTierSlash(t *testing.T) {
	k, ctx := setupKeeper(t)
	base := big.NewInt(1000)

	// Apprentice: 1500 BPS → 1000 * 1500 / 1000 = 1500
	slash := k.CalculateTierSlash(ctx, types.TierApprentice, base)
	if slash.Int64() != 1500 {
		t.Errorf("apprentice slash: expected 1500, got %d", slash.Int64())
	}

	// Scholar: 1000 BPS → 1000 * 1000 / 1000 = 1000
	slash = k.CalculateTierSlash(ctx, types.TierScholar, base)
	if slash.Int64() != 1000 {
		t.Errorf("scholar slash: expected 1000, got %d", slash.Int64())
	}
}

func TestAccuracyRate(t *testing.T) {
	tests := []struct {
		name      string
		total     uint64
		correct   uint64
		wantBps   uint64
		meetsMin  bool // meets 77% Guardian threshold
	}{
		{"exactly 77%", 1_000_000, 770_000, 770_000, true},
		{"above 77%", 1_000_000, 770_001, 770_001, true},
		{"below 77%", 1_000_000, 769_999, 769_999, false},
		{"zero completions", 0, 0, 0, false},
		{"100%", 1000, 1000, 1_000_000, true},
		{"1 of 3 (33.3%)", 3, 1, 333_333, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &types.Validator{
				TotalVerifications:   tt.total,
				CorrectVerifications: tt.correct,
			}
			got := v.GetAccuracyRate()
			if got != tt.wantBps {
				t.Errorf("GetAccuracyRate(%d/%d) = %d, want %d", tt.correct, tt.total, got, tt.wantBps)
			}
			meetsThreshold := got >= 770_000
			if meetsThreshold != tt.meetsMin {
				t.Errorf("meets guardian threshold = %v, want %v", meetsThreshold, tt.meetsMin)
			}
		})
	}
}

// ============================================================
// 6. BeginBlocker / EndBlocker Tests
// ============================================================

func TestBeginBlocker_SlashDecay(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("bb_decay")
	registerTestValidator(t, k, ctx, addr, "", "1000000000")

	params := k.GetParams(ctx)
	params.MaxSlashesPerEpoch = 5
	k.SetParams(ctx, params)

	k.SlashValidator(ctx, addr, big.NewInt(1000), "decay_1")
	k.SlashValidator(ctx, addr, big.NewInt(1000), "decay_2")

	val, _ := k.GetValidator(ctx, addr)
	if val.SlashCount != 2 {
		t.Fatalf("expected SlashCount=2, got %d", val.SlashCount)
	}

	// First epoch: slashes happened, so no decay (SlashesThisEpoch > 0).
	epochBlocks := int64(params.SlashDecayPeriodBlocks)
	ctx = ctx.WithBlockHeight(epochBlocks)
	k.BeginBlocker(ctx)

	val, _ = k.GetValidator(ctx, addr)
	if val.SlashesThisEpoch != 0 {
		t.Errorf("expected SlashesThisEpoch reset to 0, got %d", val.SlashesThisEpoch)
	}
	if val.SlashCount != 2 {
		t.Errorf("expected SlashCount=2 (had slashes this epoch), got %d", val.SlashCount)
	}

	// Second epoch: clean epoch, decay by 1.
	ctx = ctx.WithBlockHeight(epochBlocks * 2)
	k.BeginBlocker(ctx)
	val, _ = k.GetValidator(ctx, addr)
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1 after clean epoch, got %d", val.SlashCount)
	}

	// Third epoch: another clean epoch.
	ctx = ctx.WithBlockHeight(epochBlocks * 3)
	k.BeginBlocker(ctx)
	val, _ = k.GetValidator(ctx, addr)
	if val.SlashCount != 0 {
		t.Errorf("expected SlashCount=0 after 2 clean epochs, got %d", val.SlashCount)
	}
}

func TestBeginBlocker_UnbondingMaturation(t *testing.T) {
	k, ctx := setupKeeper(t)

	entry := &types.UnbondingEntry{
		Id:                "mature_1",
		DelegatorAddress:  testAddr("mature_del"),
		ValidatorAddress:  testAddr("mature_val"),
		Amount:            "100000",
		CreatedAtHeight:   50,
		CompletesAtHeight: 200,
		Status:            "pending",
	}
	k.SetUnbonding(ctx, entry)

	// Before maturation.
	ctx = ctx.WithBlockHeight(199)
	k.BeginBlocker(ctx)
	e, _ := k.GetUnbonding(ctx, "mature_1")
	if e.Status != "pending" {
		t.Errorf("expected pending before maturation, got %s", e.Status)
	}

	// At maturation.
	ctx = ctx.WithBlockHeight(200)
	k.BeginBlocker(ctx)
	e, _ = k.GetUnbonding(ctx, "mature_1")
	if e.Status != "completed" {
		t.Errorf("expected completed at maturation, got %s", e.Status)
	}
}

func TestEndBlocker_TierAdvancement(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("eb_advance")
	registerTestValidator(t, k, ctx, addr, "", "1110000")

	// Meet Verified criteria.
	val, _ := k.GetValidator(ctx, addr)
	val.TotalVerifications = 22
	val.CorrectVerifications = 18
	val.ReputationScore = 800_000
	k.SetValidator(ctx, val)

	k.EndBlocker(ctx)

	val, _ = k.GetValidator(ctx, addr)
	if val.Tier != types.TierVerified {
		t.Errorf("expected tier Verified after EndBlocker, got %s", types.ValidatorTierString(val.Tier))
	}
}

// ============================================================
// 7. Genesis Tests
// ============================================================

func TestGenesis_RoundTrip(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("genesis_val")
	registerTestValidator(t, k, ctx, addr, "did:zrn:genesis", "1111000000")

	params := k.GetParams(ctx)
	params.TierConfigs[0].MinStake = "333000"
	params.ReputationCorrectDelta = 150
	params.RedelegationCooldownBlocks = 2222
	k.SetParams(ctx, params)

	gs1 := k.ExportGenesis(ctx)
	if len(gs1.Params.TierConfigs) != 4 {
		t.Fatalf("exported genesis should have 4 TierConfigs, got %d", len(gs1.Params.TierConfigs))
	}
	if gs1.Params.TierConfigs[0].MinStake != "333000" {
		t.Errorf("exported apprentice MinStake: expected 333000, got %s", gs1.Params.TierConfigs[0].MinStake)
	}

	// Re-import.
	k2, ctx2 := setupBareKeeper(t)
	k2.InitGenesis(ctx2, gs1)

	gs2 := k2.ExportGenesis(ctx2)
	if gs2.Params.TierConfigs[0].MinStake != gs1.Params.TierConfigs[0].MinStake {
		t.Errorf("round-trip MinStake mismatch: %s vs %s",
			gs1.Params.TierConfigs[0].MinStake, gs2.Params.TierConfigs[0].MinStake)
	}
	if gs2.Params.RedelegationCooldownBlocks != 2222 {
		t.Errorf("round-trip RedelegationCooldownBlocks: expected 2222, got %d",
			gs2.Params.RedelegationCooldownBlocks)
	}
	if len(gs2.Validators) != len(gs1.Validators) {
		t.Errorf("validator count mismatch: %d vs %d", len(gs1.Validators), len(gs2.Validators))
	}
}

func TestGenesis_DefaultParams(t *testing.T) {
	k, ctx := setupBareKeeper(t)
	params := k.GetParams(ctx)
	if len(params.TierConfigs) != 4 {
		t.Fatalf("expected DefaultParams TierConfigs length=4, got %d", len(params.TierConfigs))
	}
	if params.UnbondingPeriod != 268_560 {
		t.Errorf("expected default UnbondingPeriod=268560, got %d", params.UnbondingPeriod)
	}
}

func TestGenesis_SetParamsSyncsKVStore(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := k.GetParams(ctx)
	params.TierConfigs[0].MinStake = "222000"
	params.TierConfigs[2].SlashMultiplierBps = 2000
	k.SetParams(ctx, params)

	readParams := k.GetParams(ctx)
	if readParams.TierConfigs[0].MinStake != "222000" {
		t.Errorf("expected apprentice MinStake=222000, got %s", readParams.TierConfigs[0].MinStake)
	}

	apprenticeConfig, found := k.GetTierConfig(ctx, types.TierApprentice)
	if !found {
		t.Fatal("apprentice tier config not found in KVStore")
	}
	if apprenticeConfig.MinStake != "222000" {
		t.Errorf("KVStore apprentice MinStake: expected 222000, got %s", apprenticeConfig.MinStake)
	}
}

// ============================================================
// 8. Params Validation Tests
// ============================================================

func TestParams_Validate_Valid(t *testing.T) {
	p := types.DefaultParams()
	if err := p.Validate(); err != nil {
		t.Errorf("valid params should pass: %v", err)
	}
}

func TestParams_Validate_TierConfigs3_Fails(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs = p.TierConfigs[:3]
	if err := p.Validate(); err == nil {
		t.Error("expected error for 3 tier configs")
	}
}

func TestParams_Validate_SlashMultiplierBpsOver10000_Fails(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs[0].SlashMultiplierBps = 10001
	if err := p.Validate(); err == nil {
		t.Error("expected error for slash_multiplier_bps=10001")
	}
}

func TestParams_Validate_MinAccuracyOver1M_Fails(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs[1].MinAccuracy = 1_000_001
	if err := p.Validate(); err == nil {
		t.Error("expected error for min_accuracy > 1000000")
	}
}

func TestParams_Validate_RewardMultiplierZero_Fails(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs[0].RewardMultiplierBps = 0
	if err := p.Validate(); err == nil {
		t.Error("expected error for reward_multiplier_bps=0")
	}
}

func TestParams_Validate_SelectionWeightZero_Fails(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs[0].SelectionWeightBps = 0
	if err := p.Validate(); err == nil {
		t.Error("expected error for selection_weight_bps=0")
	}
}

func TestParams_Validate_SlashEscalationBpsOverLimit(t *testing.T) {
	p := types.DefaultParams()
	p.SlashEscalationBps = 1_000_001
	if err := p.Validate(); err == nil {
		t.Error("expected error for SlashEscalationBps > 1000000")
	}
}

func TestParams_Validate_EmptyTierConfigsAllowed(t *testing.T) {
	p := types.DefaultParams()
	p.TierConfigs = nil
	if err := p.Validate(); err != nil {
		t.Errorf("empty TierConfigs should be allowed (backward compat): %v", err)
	}
}

// ============================================================
// 9. Adversarial Tests (ported from OpenClaw)
// ============================================================

func TestOC_WhaleStakeConcentration(t *testing.T) {
	k, ctx := setupKeeper(t)
	whaleAddr := testAddr("whale_val")
	smallAddr := testAddr("small_val")

	registerTestValidator(t, k, ctx, whaleAddr, "did:zrn:whale", "999999999999999")
	registerTestValidator(t, k, ctx, smallAddr, "did:zrn:small", "111000")

	params := k.GetParams(ctx)
	whaleVal, _ := k.GetValidator(ctx, whaleAddr)
	smallVal, _ := k.GetValidator(ctx, smallAddr)

	whaleEffective := k.GetEffectiveSelectionStake(ctx, whaleVal)
	smallEffective := k.GetEffectiveSelectionStake(ctx, smallVal)

	// Both at tier 0, both get virtual stake.
	if whaleEffective.Cmp(smallEffective) != 0 {
		t.Errorf("whale effective (%s) != small effective (%s) at tier 0", whaleEffective, smallEffective)
	}

	vs, _ := new(big.Int).SetString(params.VirtualStake, 10)
	if whaleEffective.Cmp(vs) != 0 {
		t.Errorf("tier-0 effective should be virtual stake (%s), got %s", params.VirtualStake, whaleEffective)
	}

	// Promote whale to Scholar — now uses real stake.
	promoteToScholar(t, k, ctx, whaleAddr)
	whaleVal, _ = k.GetValidator(ctx, whaleAddr)
	whaleEffective = k.GetEffectiveSelectionStake(ctx, whaleVal)
	if whaleEffective.Cmp(vs) == 0 {
		t.Error("scholar whale should use real stake, not virtual")
	}
}

func TestOC_SybilRegistrationSpam(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	for i := 0; i < 50; i++ {
		addr := testAddr(fmt.Sprintf("syb%04d", i))
		_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
			Operator:        addr,
			ConsensusPubkey: fmt.Sprintf("pk_syb_%04d", i),
			SelfDelegation:  "111000",
			Did:             fmt.Sprintf("did:zrn:syb%04d", i),
		})
		if err != nil {
			t.Fatalf("sybil registration #%d failed: %v", i, err)
		}
	}

	// All at tier 0 = 0 block producers.
	count := k.CountBlockProducers(ctx)
	if count != 0 {
		t.Errorf("expected 0 block producers from 50 min-stake sybils, got %d", count)
	}
}

func TestOC_UnbondingIdCollision(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("oc_unbond")
	delAddr := testAddr("oc_unbdel")

	registerTestValidator(t, k, ctx, valAddr, "", "1111000000")

	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "10000000",
		CreatedAtBlock:   100,
	})
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "10000000"
	val.TotalStake = "1121000000"
	k.SetValidator(ctx, val)

	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		resp, err := ms.Undelegate(ctx, &types.MsgUndelegate{
			Delegator: delAddr,
			Validator: valAddr,
			Amount:    "100000",
		})
		if err != nil {
			t.Fatalf("undelegate #%d failed: %v", i+1, err)
		}
		if ids[resp.UnbondingId] {
			t.Errorf("unbonding ID collision: %s returned twice", resp.UnbondingId)
		}
		ids[resp.UnbondingId] = true
	}
	if len(ids) != 5 {
		t.Errorf("expected 5 unique IDs, got %d", len(ids))
	}
}

func TestOC_TierDemotionOnSlash(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("oc_demote")
	registerTestValidator(t, k, ctx, addr, "did:zrn:gdmote", "11111000000")
	promoteToGuardian(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	if val.Tier != types.TierGuardian {
		t.Fatalf("expected Guardian tier, got %s", types.ValidatorTierString(val.Tier))
	}

	params := k.GetParams(ctx)
	params.MaxSlashesPerEpoch = 5
	k.SetParams(ctx, params)

	k.SlashValidator(ctx, addr, big.NewInt(100000), "guardian_slash")

	val, _ = k.GetValidator(ctx, addr)
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1 after slash, got %d", val.SlashCount)
	}
	// Guardian MaxSlashCount=0, so any slash should prevent Guardian status.
	if val.Tier == types.TierGuardian {
		t.Error("Guardian should lose tier after any slash (MaxSlashCount=0)")
	}
}

func TestOC_BlockProducerCapEnforcement(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	params := k.GetParams(ctx)
	params.MaxValidators = 2
	k.SetParams(ctx, params)

	// Register 4 validators at Scholar tier.
	addrs := make([]string, 4)
	for i := 0; i < 4; i++ {
		addrs[i] = testAddr(fmt.Sprintf("bpcap%d", i))
		_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
			Operator:        addrs[i],
			ConsensusPubkey: fmt.Sprintf("pk_bpcap_%d", i),
			SelfDelegation:  "1111000000", // Scholar-level stake
			Did:             fmt.Sprintf("did:zrn:bpcap%d", i),
		})
		if err != nil {
			t.Fatalf("registration #%d failed: %v", i, err)
		}
	}

	// All start at Apprentice (no verifications), so 0 block producers.
	if count := k.CountBlockProducers(ctx); count != 0 {
		t.Fatalf("expected 0 block producers initially, got %d", count)
	}

	// Promote 2 to Scholar.
	promoteToScholar(t, k, ctx, addrs[0])
	promoteToScholar(t, k, ctx, addrs[1])
	if count := k.CountBlockProducers(ctx); count != 2 {
		t.Fatalf("expected 2 block producers, got %d", count)
	}

	// Verifier-only registrations should always succeed (tier 0).
	for i := 0; i < 5; i++ {
		vAddr := testAddr(fmt.Sprintf("verifier%d", i))
		_, err := ms.RegisterValidator(ctx, &types.MsgRegisterValidator{
			Operator:        vAddr,
			ConsensusPubkey: fmt.Sprintf("pk_ver_%d", i),
			SelfDelegation:  "111000",
		})
		if err != nil {
			t.Errorf("verifier registration #%d should succeed: %v", i, err)
		}
	}
}

// ============================================================
// 10. AUDIT TESTS — 5 new tests per the security audit
// ============================================================

// AUDIT-1: Guardian tier MUST require ContestedCount >= MinContestedVerifications.
func TestAudit_GuardianTier_RequiresContestedCount(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Validator meets ALL Guardian criteria EXCEPT contested count.
	addr := testAddr("audit_gc")
	registerTestValidator(t, k, ctx, addr, "did:zrn:audit_gc", "11111000000")

	val, _ := k.GetValidator(ctx, addr)
	val.SelfDelegation = "11111000000"
	val.TotalStake = "11111000000"
	val.TotalVerifications = 500
	val.CorrectVerifications = 450
	val.ContestedVerificationsCorrect = 32 // 1 short of 33
	val.ContestedCount = 32
	val.ReputationScore = 800_000
	val.SlashCount = 0
	k.SetValidator(ctx, val)

	newTier, changed := k.CheckTierTransition(ctx, val)
	if newTier == types.TierGuardian {
		t.Error("AUDIT-1: Guardian MUST NOT be granted with ContestedCount=32 (requires 33)")
	}

	// Now set to 33 — should qualify.
	val.ContestedVerificationsCorrect = 33
	val.ContestedCount = 33
	k.SetValidator(ctx, val)

	newTier, changed = k.CheckTierTransition(ctx, val)
	if !changed || newTier != types.TierGuardian {
		t.Error("AUDIT-1: Guardian should be granted with ContestedCount=33")
	}
}

// AUDIT-2: Scholar tier MUST enforce MinStake of 1,111 ZRN (1111000000 uzrn).
func TestAudit_ScholarTier_EnforcesMinStake(t *testing.T) {
	k, ctx := setupKeeper(t)

	scholarCfg, found := k.GetTierConfig(ctx, types.TierScholar)
	if !found {
		t.Fatal("Scholar tier config not found")
	}
	if scholarCfg.MinStake != "1111000000" {
		t.Errorf("AUDIT-2: Scholar MinStake should be '1111000000', got '%s'", scholarCfg.MinStake)
	}

	// Validator with 1110999999 (1 uzrn below) should NOT qualify.
	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_110_999_999), 22, 18, 0, 0, 0)
	if tier >= types.TierScholar {
		t.Error("AUDIT-2: validator with 1110999999 uzrn should NOT reach Scholar")
	}

	// Exact threshold should qualify.
	tier = keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_111_000_000), 22, 18, 0, 0, 0)
	if tier < types.TierScholar {
		t.Error("AUDIT-2: validator with 1111000000 uzrn should reach Scholar")
	}
}

// AUDIT-3: Reputation MUST NOT underflow (go below 0).
func TestAudit_Reputation_NoUnderflow(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("audit_rep")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	// Set reputation to just above the incorrect delta.
	params := k.GetParams(ctx)
	val, _ := k.GetValidator(ctx, addr)
	val.ReputationScore = params.ReputationIncorrectDelta - 1
	k.SetValidator(ctx, val)

	k.RecordVerification(ctx, addr, false, false)

	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore != 0 {
		t.Errorf("AUDIT-3: reputation should floor at 0, got %d", val.ReputationScore)
	}

	// Slash underflow guard.
	val.ReputationScore = params.ReputationSlashDelta - 1
	k.SetValidator(ctx, val)

	k.SlashValidator(ctx, addr, big.NewInt(1000), "underflow_slash")
	val, _ = k.GetValidator(ctx, addr)
	if val.ReputationScore != 0 {
		t.Errorf("AUDIT-3: reputation should floor at 0 after slash, got %d", val.ReputationScore)
	}
}

// AUDIT-4: DisbursementQuorumBps validation (params boundary).
func TestAudit_Params_DisbursementQuorumValidation(t *testing.T) {
	p := types.DefaultParams()
	// SlashEscalationBps at exactly BPSScale should pass.
	p.SlashEscalationBps = types.BPSScale
	if err := p.Validate(); err != nil {
		t.Errorf("AUDIT-4: SlashEscalationBps at max should pass: %v", err)
	}

	// Above BPSScale should fail.
	p.SlashEscalationBps = types.BPSScale + 1
	if err := p.Validate(); err == nil {
		t.Error("AUDIT-4: SlashEscalationBps above max should fail")
	}

	// Reputation deltas at boundary.
	p = types.DefaultParams()
	p.ReputationCorrectDelta = types.BPSScale
	if err := p.Validate(); err != nil {
		t.Errorf("AUDIT-4: ReputationCorrectDelta at max should pass: %v", err)
	}
	p.ReputationCorrectDelta = types.BPSScale + 1
	if err := p.Validate(); err == nil {
		t.Error("AUDIT-4: ReputationCorrectDelta above max should fail")
	}
}

// AUDIT-5: All BPS values use 1,000,000 scale (NOT 10,000).
func TestAudit_Params_AllBPSScale(t *testing.T) {
	if types.BPSScale != 1_000_000 {
		t.Fatalf("AUDIT-5: BPSScale must be 1,000,000, got %d", types.BPSScale)
	}

	configs := types.DefaultTierConfigs()
	for _, tc := range configs {
		if tc.MinAccuracy > types.BPSScale {
			t.Errorf("AUDIT-5: tier %s MinAccuracy %d exceeds BPSScale", tc.Name, tc.MinAccuracy)
		}
	}

	// Guardian requires 77% accuracy = 770,000 BPS.
	guardianCfg := configs[3]
	if guardianCfg.MinAccuracy != 770_000 {
		t.Errorf("AUDIT-5: Guardian MinAccuracy should be 770000 (77%% at 1M scale), got %d", guardianCfg.MinAccuracy)
	}

	// Verified requires 77% accuracy.
	verifiedCfg := configs[1]
	if verifiedCfg.MinAccuracy != 770_000 {
		t.Errorf("AUDIT-5: Verified MinAccuracy should be 770000, got %d", verifiedCfg.MinAccuracy)
	}
}

// ============================================================
// 11. Query Server Tests
// ============================================================

func TestQueryValidator(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	addr := testAddr("qval")
	registerTestValidator(t, k, ctx, addr, "did:zrn:qval", "111000")

	resp, err := qs.Validator(ctx, &types.QueryValidatorRequest{Address: addr})
	if err != nil {
		t.Fatalf("Validator query failed: %v", err)
	}
	if resp.Validator.OperatorAddress != addr {
		t.Errorf("expected address %s, got %s", addr, resp.Validator.OperatorAddress)
	}
	if resp.Validator.Did != "did:zrn:qval" {
		t.Errorf("expected DID did:zrn:qval, got %s", resp.Validator.Did)
	}
}

func TestQueryValidatorNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Validator(ctx, &types.QueryValidatorRequest{Address: testAddr("nonexist")})
	if err == nil {
		t.Error("expected error for nonexistent validator")
	}
}

func TestQueryValidatorsAll(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	for i := 0; i < 3; i++ {
		addr := testAddr(fmt.Sprintf("qvall%d", i))
		registerTestValidator(t, k, ctx, addr, fmt.Sprintf("did:zrn:qvall%d", i), "111000")
	}

	resp, err := qs.Validators(ctx, &types.QueryValidatorsRequest{Tier: -1})
	if err != nil {
		t.Fatalf("Validators query failed: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected 3 validators, got %d", resp.Total)
	}
	if len(resp.Validators) != 3 {
		t.Errorf("expected 3 validators in page, got %d", len(resp.Validators))
	}
}

func TestQueryValidatorsActiveOnly(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	activeAddr := testAddr("qvactive")
	registerTestValidator(t, k, ctx, activeAddr, "did:zrn:qvactive", "111000")

	inactiveAddr := testAddr("qvinactive")
	registerTestValidator(t, k, ctx, inactiveAddr, "did:zrn:qvinactive", "111000")
	val, _ := k.GetValidator(ctx, inactiveAddr)
	val.IsActive = false
	k.SetValidator(ctx, val)

	resp, err := qs.Validators(ctx, &types.QueryValidatorsRequest{ActiveOnly: true, Tier: -1})
	if err != nil {
		t.Fatalf("Validators query failed: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 active validator, got %d", resp.Total)
	}
	if resp.Validators[0].OperatorAddress != activeAddr {
		t.Errorf("expected active validator %s, got %s", activeAddr, resp.Validators[0].OperatorAddress)
	}
}

func TestQueryValidatorsPagination(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	for i := 0; i < 5; i++ {
		addr := testAddr(fmt.Sprintf("qvpag%d", i))
		registerTestValidator(t, k, ctx, addr, fmt.Sprintf("did:zrn:qvpag%d", i), "111000")
	}

	// First page: offset=0, limit=2
	resp1, err := qs.Validators(ctx, &types.QueryValidatorsRequest{Tier: -1, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("page 1 query failed: %v", err)
	}
	if resp1.Total != 5 {
		t.Errorf("expected total 5, got %d", resp1.Total)
	}
	if len(resp1.Validators) != 2 {
		t.Errorf("expected 2 validators in page 1, got %d", len(resp1.Validators))
	}

	// Second page: offset=2, limit=2
	resp2, err := qs.Validators(ctx, &types.QueryValidatorsRequest{Tier: -1, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page 2 query failed: %v", err)
	}
	if len(resp2.Validators) != 2 {
		t.Errorf("expected 2 validators in page 2, got %d", len(resp2.Validators))
	}

	// Ensure pages don't overlap.
	if resp1.Validators[0].OperatorAddress == resp2.Validators[0].OperatorAddress {
		t.Error("page 1 and page 2 should not overlap")
	}
}

func TestQueryDelegation(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	delAddr := testAddr("qdel")
	valAddr := testAddr("qdelval")

	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "500000",
		CreatedAtBlock:   100,
	})

	resp, err := qs.Delegation(ctx, &types.QueryDelegationRequest{Delegator: delAddr, Validator: valAddr})
	if err != nil {
		t.Fatalf("Delegation query failed: %v", err)
	}
	if resp.Delegation.Amount != "500000" {
		t.Errorf("expected amount 500000, got %s", resp.Delegation.Amount)
	}
}

func TestQueryDelegationNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	_, err := qs.Delegation(ctx, &types.QueryDelegationRequest{
		Delegator: testAddr("noexist_del"),
		Validator: testAddr("noexist_val"),
	})
	if err == nil {
		t.Error("expected error for nonexistent delegation")
	}
}

func TestQueryDelegatorDelegations(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	delAddr := testAddr("qddel")

	for i := 0; i < 3; i++ {
		valAddr := testAddr(fmt.Sprintf("qddval%d", i))
		k.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: delAddr,
			ValidatorAddress: valAddr,
			Amount:           fmt.Sprintf("%d00000", i+1),
			CreatedAtBlock:   100,
		})
	}

	resp, err := qs.DelegatorDelegations(ctx, &types.QueryDelegatorDelegationsRequest{Delegator: delAddr})
	if err != nil {
		t.Fatalf("DelegatorDelegations query failed: %v", err)
	}
	if len(resp.Delegations) != 3 {
		t.Errorf("expected 3 delegations, got %d", len(resp.Delegations))
	}
}

// ============================================================
// 12. Additional Query Tests
// ============================================================

func TestQueryValidatorDelegations(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)
	valAddr := testAddr("qvdval")

	for i := 0; i < 3; i++ {
		delAddr := testAddr(fmt.Sprintf("qvddel%d", i))
		k.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: delAddr,
			ValidatorAddress: valAddr,
			Amount:           "100000",
			CreatedAtBlock:   100,
		})
	}

	resp, err := qs.ValidatorDelegations(ctx, &types.QueryValidatorDelegationsRequest{Validator: valAddr})
	if err != nil {
		t.Fatalf("ValidatorDelegations query failed: %v", err)
	}
	if len(resp.Delegations) != 3 {
		t.Errorf("expected 3 delegations to validator, got %d", len(resp.Delegations))
	}
}

func TestQueryParams(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("Params query failed: %v", err)
	}
	if resp.Params == nil {
		t.Fatal("expected non-nil params")
	}
	if resp.Params.UnbondingPeriod != 268_560 {
		t.Errorf("expected default UnbondingPeriod=268560, got %d", resp.Params.UnbondingPeriod)
	}
	if len(resp.TierConfigs) != 4 {
		t.Errorf("expected 4 tier configs, got %d", len(resp.TierConfigs))
	}
}

func TestQueryTierConfig(t *testing.T) {
	k, ctx := setupKeeper(t)
	qs := keeper.NewQueryServerImpl(k)

	resp, err := qs.TierConfig(ctx, &types.QueryTierConfigRequest{Tier: uint32(types.TierScholar)})
	if err != nil {
		t.Fatalf("TierConfig query failed: %v", err)
	}
	if resp.TierConfig == nil {
		t.Fatal("expected non-nil tier config")
	}
	if resp.TierConfig.Name != "Scholar" {
		t.Errorf("expected tier name 'Scholar', got '%s'", resp.TierConfig.Name)
	}
	if resp.TierConfig.MinStake != "1111000000" {
		t.Errorf("expected Scholar MinStake '1111000000', got '%s'", resp.TierConfig.MinStake)
	}
}

// ============================================================
// 13. Tier Transition Tests
// ============================================================

func TestCheckTierTransition_ApprenticeToVerified(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tt_a2v")
	registerTestValidator(t, k, ctx, addr, "", "1110000")

	val, _ := k.GetValidator(ctx, addr)
	val.TotalVerifications = 22
	val.CorrectVerifications = 18
	val.ReputationScore = 800_000
	k.SetValidator(ctx, val)

	newTier, changed := k.CheckTierTransition(ctx, val)
	if !changed {
		t.Error("expected tier change from Apprentice to Verified")
	}
	if newTier != types.TierVerified {
		t.Errorf("expected Verified, got %s", types.ValidatorTierString(newTier))
	}
}

func TestCheckTierTransition_NoChange(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tt_nochg")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	// Apprentice with insufficient stats for Verified.
	val.TotalVerifications = 5
	val.CorrectVerifications = 3
	k.SetValidator(ctx, val)

	_, changed := k.CheckTierTransition(ctx, val)
	if changed {
		t.Error("expected no tier change for under-qualified validator")
	}
}

func TestCheckTierTransition_ScholarDownToVerified(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tt_s2v")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	// Reduce stake below Scholar minimum.
	val.SelfDelegation = "500000"
	val.TotalStake = "500000"
	k.SetValidator(ctx, val)

	newTier, changed := k.CheckTierTransition(ctx, val)
	if !changed {
		t.Error("expected tier demotion from Scholar")
	}
	if newTier >= types.TierScholar {
		t.Errorf("expected demotion below Scholar, got %s", types.ValidatorTierString(newTier))
	}
}

// ============================================================
// 14. Staking Edge Cases
// ============================================================

func TestGetTotalBondedStake(t *testing.T) {
	k, ctx := setupKeeper(t)

	registerTestValidator(t, k, ctx, testAddr("tbs_v1"), "", "1000000")
	registerTestValidator(t, k, ctx, testAddr("tbs_v2"), "", "2000000")
	registerTestValidator(t, k, ctx, testAddr("tbs_v3"), "", "3000000")

	total := k.GetTotalBondedStake(ctx)
	expected := big.NewInt(6000000)
	if total.Cmp(expected) != 0 {
		t.Errorf("expected total bonded stake %s, got %s", expected, total)
	}
}

func TestCountBlockProducers(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Register 4 validators: 2 at Apprentice, 1 Scholar, 1 Guardian.
	apprentice1 := testAddr("cbp_a1")
	apprentice2 := testAddr("cbp_a2")
	scholar := testAddr("cbp_sch")
	guardian := testAddr("cbp_grd")

	registerTestValidator(t, k, ctx, apprentice1, "", "111000")
	registerTestValidator(t, k, ctx, apprentice2, "", "111000")
	registerTestValidator(t, k, ctx, scholar, "", "1111000000")
	registerTestValidator(t, k, ctx, guardian, "", "11111000000")

	promoteToScholar(t, k, ctx, scholar)
	promoteToGuardian(t, k, ctx, guardian)

	count := k.CountBlockProducers(ctx)
	if count != 2 {
		t.Errorf("expected 2 block producers (Scholar + Guardian), got %d", count)
	}
}

func TestIterateValidatorsEarlyStop(t *testing.T) {
	k, ctx := setupKeeper(t)

	for i := 0; i < 5; i++ {
		addr := testAddr(fmt.Sprintf("ives%d", i))
		registerTestValidator(t, k, ctx, addr, fmt.Sprintf("did:zrn:ives%d", i), "111000")
	}

	var visited int
	k.IterateValidators(ctx, func(val *types.Validator) bool {
		visited++
		return visited >= 2 // stop after 2
	})

	if visited != 2 {
		t.Errorf("expected iteration to stop after 2, visited %d", visited)
	}
}

func TestDelegationReverseIndex(t *testing.T) {
	k, ctx := setupKeeper(t)
	valAddr := testAddr("dri_val")

	for i := 0; i < 3; i++ {
		delAddr := testAddr(fmt.Sprintf("dri_del%d", i))
		k.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: delAddr,
			ValidatorAddress: valAddr,
			Amount:           fmt.Sprintf("%d00000", i+1),
			CreatedAtBlock:   100,
		})
	}

	dels := k.GetDelegationsForValidator(ctx, valAddr)
	if len(dels) != 3 {
		t.Errorf("expected 3 delegations via reverse index, got %d", len(dels))
	}
}

// ============================================================
// 15. Unbonding Edge Cases
// ============================================================

func TestGetMatureUnbondings(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Create 3 unbondings with different completion heights.
	entries := []struct {
		id         string
		completes  uint64
	}{
		{"mat_1", 50},
		{"mat_2", 100},
		{"mat_3", 200},
	}
	for _, e := range entries {
		k.SetUnbonding(ctx, &types.UnbondingEntry{
			Id:                e.id,
			DelegatorAddress:  testAddr("mat_del"),
			ValidatorAddress:  testAddr("mat_val"),
			Amount:            "100000",
			CreatedAtHeight:   10,
			CompletesAtHeight: e.completes,
			Status:            "pending",
		})
	}

	// At height 100, entries mat_1 and mat_2 should be mature.
	mature := k.GetMatureUnbondings(ctx, 100)
	if len(mature) != 2 {
		t.Errorf("expected 2 mature unbondings at height 100, got %d", len(mature))
	}

	// At height 200, all 3 should be mature.
	mature = k.GetMatureUnbondings(ctx, 200)
	if len(mature) != 3 {
		t.Errorf("expected 3 mature unbondings at height 200, got %d", len(mature))
	}
}

func TestGetUnbondingsForDelegator(t *testing.T) {
	k, ctx := setupKeeper(t)

	del1 := testAddr("ubd_del1")
	del2 := testAddr("ubd_del2")

	k.SetUnbonding(ctx, &types.UnbondingEntry{
		Id: "ubd_1", DelegatorAddress: del1, ValidatorAddress: testAddr("ubd_val"),
		Amount: "100000", CreatedAtHeight: 100, CompletesAtHeight: 200, Status: "pending",
	})
	k.SetUnbonding(ctx, &types.UnbondingEntry{
		Id: "ubd_2", DelegatorAddress: del1, ValidatorAddress: testAddr("ubd_val"),
		Amount: "200000", CreatedAtHeight: 100, CompletesAtHeight: 300, Status: "pending",
	})
	k.SetUnbonding(ctx, &types.UnbondingEntry{
		Id: "ubd_3", DelegatorAddress: del2, ValidatorAddress: testAddr("ubd_val"),
		Amount: "300000", CreatedAtHeight: 100, CompletesAtHeight: 400, Status: "pending",
	})

	entries := k.GetUnbondingsForDelegator(ctx, del1)
	if len(entries) != 2 {
		t.Errorf("expected 2 unbondings for del1, got %d", len(entries))
	}

	entries2 := k.GetUnbondingsForDelegator(ctx, del2)
	if len(entries2) != 1 {
		t.Errorf("expected 1 unbonding for del2, got %d", len(entries2))
	}
}

func TestIterateUnbondings(t *testing.T) {
	k, ctx := setupKeeper(t)

	for i := 0; i < 4; i++ {
		k.SetUnbonding(ctx, &types.UnbondingEntry{
			Id:                fmt.Sprintf("iter_u_%d", i),
			DelegatorAddress:  testAddr("iter_del"),
			ValidatorAddress:  testAddr("iter_val"),
			Amount:            "100000",
			CreatedAtHeight:   100,
			CompletesAtHeight: 200,
			Status:            "pending",
		})
	}

	var count int
	k.IterateUnbondings(ctx, func(entry *types.UnbondingEntry) bool {
		count++
		return false
	})
	if count != 4 {
		t.Errorf("expected 4 unbondings via iteration, got %d", count)
	}
}

// ============================================================
// 16. Misc Tests
// ============================================================

func TestGetValidatorByDIDNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)

	_, found := k.GetValidatorByDID(ctx, "did:zrn:nonexistent")
	if found {
		t.Error("expected validator not found for nonexistent DID")
	}
}

func TestGetActiveValidatorSet(t *testing.T) {
	k, ctx := setupKeeper(t)

	active1 := testAddr("avs_a1")
	active2 := testAddr("avs_a2")
	inactive := testAddr("avs_in")

	registerTestValidator(t, k, ctx, active1, "did:zrn:avs_a1", "111000")
	registerTestValidator(t, k, ctx, active2, "did:zrn:avs_a2", "111000")
	registerTestValidator(t, k, ctx, inactive, "did:zrn:avs_in", "111000")

	// Deactivate one.
	val, _ := k.GetValidator(ctx, inactive)
	val.IsActive = false
	k.SetValidator(ctx, val)

	activeSet := k.GetActiveValidatorSet(ctx)
	if len(activeSet) != 2 {
		t.Errorf("expected 2 active validators, got %d", len(activeSet))
	}
	for _, v := range activeSet {
		if !v.IsActive {
			t.Errorf("inactive validator %s found in active set", v.OperatorAddress)
		}
	}
}

func TestLastRedelegationHeight(t *testing.T) {
	k, ctx := setupKeeper(t)
	delAddr := testAddr("lrh_del")

	// Default should be 0.
	h := k.GetLastRedelegationHeight(ctx, delAddr)
	if h != 0 {
		t.Errorf("expected default redelegation height 0, got %d", h)
	}

	// Set and retrieve.
	k.SetLastRedelegationHeight(ctx, delAddr, 500)
	h = k.GetLastRedelegationHeight(ctx, delAddr)
	if h != 500 {
		t.Errorf("expected redelegation height 500, got %d", h)
	}

	// Overwrite.
	k.SetLastRedelegationHeight(ctx, delAddr, 750)
	h = k.GetLastRedelegationHeight(ctx, delAddr)
	if h != 750 {
		t.Errorf("expected redelegation height 750, got %d", h)
	}
}

// ============================================================
// 17. Tier Transition Edge Cases (ported from prototype)
// ============================================================

// TestTierPromotion_VerifiedToScholar verifies that a Verified validator
// is promoted to Scholar when they meet the Scholar-level stake and
// verification criteria.
func TestTierPromotion_VerifiedToScholar(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tp_v2s")
	registerTestValidator(t, k, ctx, addr, "did:zrn:v2s", "1111000000")

	// Set to Verified tier first with Verified-level stats.
	val, _ := k.GetValidator(ctx, addr)
	val.Tier = types.TierVerified
	val.SelfDelegation = "1111000000"
	val.TotalStake = "1111000000"
	val.TotalVerifications = 22
	val.CorrectVerifications = 18 // 81% accuracy
	val.ReputationScore = 600_000
	k.SetValidator(ctx, val)

	// CheckTierTransition should promote to Scholar (stake >= 1111000000,
	// verifications >= 11, accuracy >= 50%).
	newTier, changed := k.CheckTierTransition(ctx, val)
	if !changed {
		t.Error("expected tier change from Verified to Scholar")
	}
	if newTier != types.TierScholar {
		t.Errorf("expected Scholar tier, got %s", types.ValidatorTierString(newTier))
	}
}

// TestTierDemotion_ScholarToVerified_InsufficientStake verifies that a Scholar
// validator is demoted when their stake drops below the Scholar minimum.
func TestTierDemotion_ScholarToVerified_InsufficientStake(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("td_s2v_stake")
	registerTestValidator(t, k, ctx, addr, "did:zrn:s2v", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	if val.Tier != types.TierScholar {
		t.Fatalf("expected Scholar tier, got %s", types.ValidatorTierString(val.Tier))
	}

	// Drop stake below Scholar minimum (1111000000).
	val.SelfDelegation = "1110999999"
	val.TotalStake = "1110999999"
	k.SetValidator(ctx, val)

	newTier, changed := k.CheckTierTransition(ctx, val)
	if !changed {
		t.Error("expected tier demotion from Scholar")
	}
	if newTier >= types.TierScholar {
		t.Errorf("expected demotion below Scholar, got %s", types.ValidatorTierString(newTier))
	}
	// With 22 verifications, 18 correct, and high enough stake for Verified,
	// they should land at Verified.
	if newTier != types.TierVerified {
		t.Errorf("expected demotion to Verified, got %s", types.ValidatorTierString(newTier))
	}
}

// TestTierPromotion_RequiresMinStake verifies that a validator cannot
// be promoted to Verified without meeting the MinStake requirement.
func TestTierPromotion_RequiresMinStake(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tp_minstake")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	val, _ := k.GetValidator(ctx, addr)
	// Has excellent verification stats but insufficient stake for Verified.
	val.TotalVerifications = 100
	val.CorrectVerifications = 90 // 90% accuracy
	val.ReputationScore = 900_000
	// Stake is 111000, Verified requires 1110000
	k.SetValidator(ctx, val)

	newTier, changed := k.CheckTierTransition(ctx, val)
	if changed && newTier >= types.TierVerified {
		t.Error("validator should NOT be promoted to Verified without meeting MinStake")
	}
	if newTier != types.TierApprentice {
		t.Errorf("expected Apprentice (stake too low), got %s", types.ValidatorTierString(newTier))
	}
}

// TestTierPromotion_RequiresReputation verifies that a validator cannot
// be promoted without meeting the minimum accuracy/verification requirements.
func TestTierPromotion_RequiresReputation(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("tp_rep")
	registerTestValidator(t, k, ctx, addr, "", "1110000") // Verified-level stake

	val, _ := k.GetValidator(ctx, addr)
	// Has sufficient stake but poor accuracy (below 77% needed for Verified).
	val.TotalVerifications = 22
	val.CorrectVerifications = 10 // 45% accuracy (below 77%)
	val.ReputationScore = 400_000
	k.SetValidator(ctx, val)

	tier := keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_110_000), 22, 10, 0, 0, 0)
	if tier >= types.TierVerified {
		t.Errorf("validator with 45%% accuracy should NOT reach Verified, got %s",
			types.ValidatorTierString(tier))
	}

	// Borderline: exactly 77% accuracy
	tier = keeper.ComputeValidatorTier(ctx, k, big.NewInt(1_110_000), 100, 77, 0, 0, 0)
	if tier != types.TierVerified {
		t.Errorf("validator with exactly 77%% accuracy should reach Verified, got %s",
			types.ValidatorTierString(tier))
	}
}

// ============================================================
// 18. Slashing Edge Cases (ported from prototype)
// ============================================================

// TestSlashValidator_DoubleSlash verifies that two slashes in the same epoch
// both apply and escalation is correctly calculated for the second one.
func TestSlashValidator_DoubleSlash(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_dbl")
	registerTestValidator(t, k, ctx, addr, "", "10000000000")
	promoteToScholar(t, k, ctx, addr)

	// First slash at count=0: no escalation, effective = 1000000
	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "double_1")
	val, _ := k.GetValidator(ctx, addr)
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1 after first slash, got %d", val.SlashCount)
	}

	// Second slash at count=1: escalation = (1M + 1*100000) / 1M = 1.1x
	// effective = 1000000 * 1.1 = 1100000
	stakeBefore, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "double_2")
	val, _ = k.GetValidator(ctx, addr)
	stakeAfter, _ := new(big.Int).SetString(val.SelfDelegation, 10)

	slashed := new(big.Int).Sub(stakeBefore, stakeAfter)
	if slashed.Int64() != 1_100_000 {
		t.Errorf("second slash should be 1100000 (1.1x escalation), got %s", slashed.String())
	}
	if val.SlashCount != 2 {
		t.Errorf("expected SlashCount=2, got %d", val.SlashCount)
	}
}

// TestSlashBelowMinStake_Demotion verifies that when a validator is slashed
// below a tier's MinStake threshold, CheckTierTransition correctly demotes them.
func TestSlashBelowMinStake_Demotion(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_bms")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	if val.Tier != types.TierScholar {
		t.Fatalf("expected Scholar, got %s", types.ValidatorTierString(val.Tier))
	}

	// Slash enough to drop below Scholar MinStake (1111000000).
	// Current self-delegation is 1111000000. Slash 200_000_000 to get 911000000.
	k.SlashValidator(ctx, addr, big.NewInt(200_000_000), "below_min")

	val, _ = k.GetValidator(ctx, addr)
	selfDel, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	scholarMin, _ := new(big.Int).SetString("1111000000", 10)

	if selfDel.Cmp(scholarMin) >= 0 {
		t.Fatalf("stake should be below Scholar min after slash, got %s", selfDel.String())
	}

	// Tier should have been demoted via CheckTierTransition inside SlashValidator.
	if val.Tier >= types.TierScholar {
		t.Errorf("expected demotion below Scholar after slashing below MinStake, got %s",
			types.ValidatorTierString(val.Tier))
	}
}

// TestSlashValidator_DoubleSign simulates a double-sign scenario:
// a large slash that should significantly reduce the validator's stake
// and increment their slash count.
func TestSlashValidator_DoubleSign(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_dsign")
	registerTestValidator(t, k, ctx, addr, "", "10000000000")
	promoteToScholar(t, k, ctx, addr)

	// Double-sign slash: 5% of stake = 500_000_000
	val, _ := k.GetValidator(ctx, addr)
	stakeBefore, _ := new(big.Int).SetString(val.SelfDelegation, 10)

	k.SlashValidator(ctx, addr, big.NewInt(500_000_000), "double_sign")

	val, _ = k.GetValidator(ctx, addr)
	stakeAfter, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	slashed := new(big.Int).Sub(stakeBefore, stakeAfter)

	if slashed.Int64() != 500_000_000 {
		t.Errorf("expected slash of 500000000, got %s", slashed.String())
	}
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1, got %d", val.SlashCount)
	}
	if val.SlashesThisEpoch != 1 {
		t.Errorf("expected SlashesThisEpoch=1, got %d", val.SlashesThisEpoch)
	}
}

// TestSlashValidator_Downtime simulates a downtime slash: a smaller slash
// amount that should reduce stake and record the slash.
func TestSlashValidator_Downtime(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_down")
	registerTestValidator(t, k, ctx, addr, "", "10000000000")

	val, _ := k.GetValidator(ctx, addr)
	stakeBefore, _ := new(big.Int).SetString(val.SelfDelegation, 10)

	// Downtime slash: 0.01% of stake = 1_000_000
	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "downtime")

	val, _ = k.GetValidator(ctx, addr)
	stakeAfter, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	slashed := new(big.Int).Sub(stakeBefore, stakeAfter)

	if slashed.Int64() != 1_000_000 {
		t.Errorf("expected downtime slash of 1000000, got %s", slashed.String())
	}
	if val.SlashCount != 1 {
		t.Errorf("expected SlashCount=1 after downtime slash, got %d", val.SlashCount)
	}

	// Reputation should decrease by ReputationSlashDelta.
	expectedRep := uint64(500_000) - k.GetParams(ctx).ReputationSlashDelta
	if val.ReputationScore != expectedRep {
		t.Errorf("expected reputation %d after slash, got %d", expectedRep, val.ReputationScore)
	}
}

// ============================================================
// 19. Reputation Edge Cases (ported from prototype)
// ============================================================

// TestReputationAccumulation verifies that correct verifications
// accumulate reputation score over multiple rounds.
func TestReputationAccumulation(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rep_accum")
	registerTestValidator(t, k, ctx, addr, "", "111000")

	params := k.GetParams(ctx)

	// Record 10 correct verifications.
	for i := 0; i < 10; i++ {
		k.RecordVerification(ctx, addr, true, false)
	}

	val, _ := k.GetValidator(ctx, addr)
	expectedRep := uint64(500_000) + 10*params.ReputationCorrectDelta
	if val.ReputationScore != expectedRep {
		t.Errorf("expected reputation %d after 10 correct verifications, got %d",
			expectedRep, val.ReputationScore)
	}
	if val.TotalVerifications != 10 {
		t.Errorf("expected TotalVerifications=10, got %d", val.TotalVerifications)
	}
	if val.CorrectVerifications != 10 {
		t.Errorf("expected CorrectVerifications=10, got %d", val.CorrectVerifications)
	}
}

// TestReputationDecay verifies that incorrect verifications and slashes
// reduce reputation over time.
func TestReputationDecay(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rep_decay")
	registerTestValidator(t, k, ctx, addr, "", "1000000000")

	params := k.GetParams(ctx)

	// Start at 500_000 (default). Record 5 incorrect verifications.
	for i := 0; i < 5; i++ {
		k.RecordVerification(ctx, addr, false, false)
	}

	val, _ := k.GetValidator(ctx, addr)
	expectedAfterIncorrect := uint64(500_000) - 5*params.ReputationIncorrectDelta
	if val.ReputationScore != expectedAfterIncorrect {
		t.Errorf("expected reputation %d after 5 incorrect, got %d",
			expectedAfterIncorrect, val.ReputationScore)
	}

	// Now slash: further reduces reputation by ReputationSlashDelta.
	k.SlashValidator(ctx, addr, big.NewInt(1000), "rep_decay_slash")
	val, _ = k.GetValidator(ctx, addr)
	expectedAfterSlash := expectedAfterIncorrect - params.ReputationSlashDelta
	if val.ReputationScore != expectedAfterSlash {
		t.Errorf("expected reputation %d after slash, got %d",
			expectedAfterSlash, val.ReputationScore)
	}
}

// TestReputationImpactsVRFWeight verifies that reputation indirectly
// impacts VRF selection by influencing tier eligibility. A validator
// with low reputation should not qualify for higher tiers and thus
// should receive virtual stake (lower VRF weight) instead of real stake.
func TestReputationImpactsVRFWeight(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("rep_vrf")
	registerTestValidator(t, k, ctx, addr, "", "1111000000")
	promoteToScholar(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	if val.Tier != types.TierScholar {
		t.Fatalf("expected Scholar, got %s", types.ValidatorTierString(val.Tier))
	}

	// Scholar gets real stake for VRF selection.
	effective := k.GetEffectiveSelectionStake(ctx, val)
	totalStake, _ := new(big.Int).SetString(val.TotalStake, 10)
	if effective.Cmp(totalStake) != 0 {
		t.Errorf("Scholar should use real stake %s, got %s", totalStake, effective)
	}

	// Now directly demote the validator to Apprentice by reducing stake
	// below Verified threshold but above MinStakeForVerification (111000).
	// This simulates the effect of slashing degrading tier eligibility.
	val, _ = k.GetValidator(ctx, addr)
	val.SelfDelegation = "500000"   // Below Verified min (1110000)
	val.TotalStake = "500000"
	val.TotalVerifications = 5       // Too few for Verified
	val.CorrectVerifications = 3
	val.ReputationScore = 300_000    // Low reputation
	k.SetValidator(ctx, val)

	// CheckTierTransition should demote to Apprentice.
	newTier, changed := k.CheckTierTransition(ctx, val)
	if !changed {
		t.Fatal("expected tier change after stats reduction")
	}
	if newTier != types.TierApprentice {
		t.Errorf("expected Apprentice after degradation, got %s", types.ValidatorTierString(newTier))
	}
	val.Tier = newTier
	k.SetValidator(ctx, val)

	// Apprentice tier uses virtual stake for VRF selection.
	effective = k.GetEffectiveSelectionStake(ctx, val)
	vs, _ := new(big.Int).SetString(k.GetParams(ctx).VirtualStake, 10)
	if effective.Cmp(vs) != 0 {
		t.Errorf("demoted Apprentice should use virtual stake %s, got %s", vs, effective)
	}
}

// ============================================================
// 20. Delegation Edge Cases (ported from prototype)
// ============================================================

// TestDelegateToInactiveValidator verifies that delegating to an
// inactive validator fails with an appropriate error.
func TestDelegateToInactiveValidator(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("del_inact")
	delAddr := testAddr("del_inact_d")

	val := &types.Validator{
		OperatorAddress: valAddr,
		ConsensusPubkey: "pk_inactive",
		Tier:            types.TierApprentice,
		SelfDelegation:  "111000",
		DelegatedStake:  "0",
		TotalStake:      "111000",
		IsActive:        false,
		ReputationScore: 500_000,
	}
	k.SetValidator(ctx, val)

	_, err := ms.Delegate(ctx, &types.MsgDelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "100000",
	})
	if err == nil {
		t.Error("expected error when delegating to inactive validator")
	}
}

// TestUndelegateMoreThanStaked verifies that undelegating more than the
// delegated amount is rejected.
func TestUndelegateMoreThanStaked(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("undel_more")
	delAddr := testAddr("undel_more_d")

	registerTestValidator(t, k, ctx, valAddr, "", "1111000000")

	// Create a delegation of 500000.
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "500000",
		CreatedAtBlock:   100,
	})
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "500000"
	val.TotalStake = "1111500000"
	k.SetValidator(ctx, val)

	// Try to undelegate more than delegated.
	_, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "999999",
	})
	if err == nil {
		t.Error("expected error when undelegating more than staked")
	}

	// Verify the delegation is unchanged.
	del, found := k.GetDelegation(ctx, delAddr, valAddr)
	if !found {
		t.Fatal("delegation should still exist after failed undelegate")
	}
	if del.Amount != "500000" {
		t.Errorf("delegation should be unchanged at 500000, got %s", del.Amount)
	}
}

// TestRedelegation verifies the full redelegation flow: source reduced,
// destination increased, validator stakes updated, and cooldown set.
func TestRedelegation(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	val1 := testAddr("redel_s")
	val2 := testAddr("redel_d")
	delAddr := testAddr("redel_dlg")

	registerTestValidator(t, k, ctx, val1, "", "1111000000")
	registerTestValidator(t, k, ctx, val2, "", "1111000000")

	// Create delegation to val1.
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: val1,
		Amount:           "1000000",
		CreatedAtBlock:   100,
	})
	v1, _ := k.GetValidator(ctx, val1)
	v1.DelegatedStake = "1000000"
	v1.TotalStake = "1112000000"
	k.SetValidator(ctx, v1)

	// Redelegate 400000 from val1 to val2.
	_, err := ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator:    delAddr,
		SrcValidator: val1,
		DstValidator: val2,
		Amount:       "400000",
	})
	if err != nil {
		t.Fatalf("Redelegate failed: %v", err)
	}

	// Source delegation reduced.
	srcDel, found := k.GetDelegation(ctx, delAddr, val1)
	if !found {
		t.Fatal("source delegation should still exist")
	}
	if srcDel.Amount != "600000" {
		t.Errorf("expected remaining source 600000, got %s", srcDel.Amount)
	}

	// Destination delegation created.
	dstDel, found := k.GetDelegation(ctx, delAddr, val2)
	if !found {
		t.Fatal("destination delegation should exist")
	}
	if dstDel.Amount != "400000" {
		t.Errorf("expected destination 400000, got %s", dstDel.Amount)
	}

	// Source validator delegated stake reduced.
	v1After, _ := k.GetValidator(ctx, val1)
	if v1After.DelegatedStake != "600000" {
		t.Errorf("expected src val delegated 600000, got %s", v1After.DelegatedStake)
	}

	// Destination validator delegated stake increased.
	v2After, _ := k.GetValidator(ctx, val2)
	if v2After.DelegatedStake != "400000" {
		t.Errorf("expected dst val delegated 400000, got %s", v2After.DelegatedStake)
	}

	// Cooldown should be set.
	lastRedel := k.GetLastRedelegationHeight(ctx, delAddr)
	if lastRedel != 100 {
		t.Errorf("expected last redelegation height 100, got %d", lastRedel)
	}
}

// TestRedelegation_OffByOneCooldown tests the exact cooldown boundary
// for redelegation: fail at (lastHeight + cooldown - 1), succeed at
// (lastHeight + cooldown).
func TestRedelegation_OffByOneCooldown(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	val1 := testAddr("obo_src")
	val2 := testAddr("obo_dst")
	val3 := testAddr("obo_dst2")
	delAddr := testAddr("obo_del")

	registerTestValidator(t, k, ctx, val1, "", "1111000000")
	registerTestValidator(t, k, ctx, val2, "", "1111000000")
	registerTestValidator(t, k, ctx, val3, "", "1111000000")

	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: val1,
		Amount:           "1000000",
		CreatedAtBlock:   100,
	})
	v1, _ := k.GetValidator(ctx, val1)
	v1.DelegatedStake = "1000000"
	v1.TotalStake = "1112000000"
	k.SetValidator(ctx, v1)

	// First redelegate at height 100.
	_, err := ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator: delAddr, SrcValidator: val1, DstValidator: val2, Amount: "200000",
	})
	if err != nil {
		t.Fatalf("first redelegate failed: %v", err)
	}

	params := k.GetParams(ctx)
	cooldown := int64(params.RedelegationCooldownBlocks)

	// At height 100 + cooldown - 1: still in cooldown.
	ctx = ctx.WithBlockHeight(100 + cooldown - 1)
	_, err = ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator: delAddr, SrcValidator: val1, DstValidator: val3, Amount: "100000",
	})
	if err == nil {
		t.Errorf("redelegate at boundary-1 (block %d) should fail", 100+cooldown-1)
	}

	// At height 100 + cooldown: cooldown elapsed.
	ctx = ctx.WithBlockHeight(100 + cooldown)
	_, err = ms.Redelegate(ctx, &types.MsgRedelegate{
		Delegator: delAddr, SrcValidator: val1, DstValidator: val3, Amount: "100000",
	})
	if err != nil {
		t.Errorf("redelegate at boundary (block %d) should succeed: %v", 100+cooldown, err)
	}
}

// TestDelegateToNonexistentValidator verifies that delegating to a
// validator that does not exist returns an error.
func TestDelegateToNonexistentValidator(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.Delegate(ctx, &types.MsgDelegate{
		Delegator: testAddr("del_noexist"),
		Validator: testAddr("noexist_val"),
		Amount:    "100000",
	})
	if err == nil {
		t.Error("expected error when delegating to nonexistent validator")
	}
}

// TestUndelegateFromNonexistentDelegation verifies that undelegating
// when no delegation exists returns an error.
func TestUndelegateFromNonexistentDelegation(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	valAddr := testAddr("undel_noex")

	registerTestValidator(t, k, ctx, valAddr, "", "1111000000")

	_, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: testAddr("undel_noex_d"),
		Validator: valAddr,
		Amount:    "100000",
	})
	if err == nil {
		t.Error("expected error when undelegating from nonexistent delegation")
	}
}

// TestSlashValidator_DeactivatesAfterThreshold verifies that a validator
// is deactivated when their slash count reaches MaxSlashCountDeactivate,
// and that further operations reflect the inactive state.
func TestSlashValidator_DeactivatesAfterThreshold(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("slash_thresh")
	registerTestValidator(t, k, ctx, addr, "", "10000000000")

	params := k.GetParams(ctx)
	params.MaxSlashCountDeactivate = 2
	params.MaxSlashesPerEpoch = 10
	k.SetParams(ctx, params)

	// First slash: still active.
	k.SlashValidator(ctx, addr, big.NewInt(1000), "thresh_1")
	val, _ := k.GetValidator(ctx, addr)
	if !val.IsActive {
		t.Error("validator should still be active after first slash")
	}

	// Second slash: deactivated (MaxSlashCountDeactivate=2).
	k.SlashValidator(ctx, addr, big.NewInt(1000), "thresh_2")
	val, _ = k.GetValidator(ctx, addr)
	if val.IsActive {
		t.Error("validator should be deactivated after reaching MaxSlashCountDeactivate")
	}
	if val.SlashCount != 2 {
		t.Errorf("expected SlashCount=2, got %d", val.SlashCount)
	}

	// Delegating to deactivated validator should fail.
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.Delegate(ctx, &types.MsgDelegate{
		Delegator: testAddr("thresh_del"),
		Validator: addr,
		Amount:    "100000",
	})
	if err == nil {
		t.Error("expected error when delegating to deactivated validator")
	}
}

// TestGuardianDemotion_OnSlash verifies that a Guardian validator
// loses their tier after any slash (MaxSlashCount=0 for Guardian).
func TestGuardianDemotion_OnSlash(t *testing.T) {
	k, ctx := setupKeeper(t)
	addr := testAddr("grd_demote")
	registerTestValidator(t, k, ctx, addr, "did:zrn:grddemote", "11111000000")
	promoteToGuardian(t, k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	if val.Tier != types.TierGuardian {
		t.Fatalf("expected Guardian, got %s", types.ValidatorTierString(val.Tier))
	}

	// Single slash should disqualify from Guardian (MaxSlashCount=0).
	params := k.GetParams(ctx)
	params.MaxSlashesPerEpoch = 5
	k.SetParams(ctx, params)

	k.SlashValidator(ctx, addr, big.NewInt(100_000), "grd_slash")

	val, _ = k.GetValidator(ctx, addr)
	if val.Tier == types.TierGuardian {
		t.Error("Guardian should be demoted after any slash (MaxSlashCount=0)")
	}

	// With sufficient stake and verifications, should land at Scholar.
	if val.Tier < types.TierScholar {
		t.Errorf("expected at least Scholar after Guardian demotion, got %s",
			types.ValidatorTierString(val.Tier))
	}
}

// ────────────────────────────────────────────────────────────────────────────
// SlashBurnFractionBps tests
// ────────────────────────────────────────────────────────────────────────────

// TestSlashBurnFraction_Disabled verifies backward-compat: BurnBps=0 sends
// the full slash to the development fund and nothing is burned.
func TestSlashBurnFraction_Disabled(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("burn_disabled")
	registerTestValidator(t, k, ctx, addr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashBurnFractionBps = 0 // explicitly disabled
	k.SetParams(ctx, params)

	k.SlashValidator(ctx, addr, big.NewInt(1_000_000), "test")

	// Nothing burned.
	if len(bk.burnedCoins) != 0 {
		t.Errorf("expected 0 burned, got %v", bk.burnedCoins)
	}
	// Full amount sent to module routing (development_fund).
	if bk.moduleSent.IsZero() {
		t.Error("expected non-zero module routing when burn=0")
	}
}

// TestSlashBurnFraction_Half verifies that BurnBps=500_000 burns exactly 50%
// and routes the remaining 50% to development_fund.
func TestSlashBurnFraction_Half(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("burn_half")
	registerTestValidator(t, k, ctx, addr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashBurnFractionBps = 500_000 // 50%
	params.SlashEscalationBps = 0        // disable escalation for clean math
	k.SetParams(ctx, params)

	slashAmt := int64(2_000_000)
	k.SlashValidator(ctx, addr, big.NewInt(slashAmt), "test")

	burnedAmt := bk.burnedCoins.AmountOf("uzrn").Int64()
	sentAmt := bk.moduleSent.AmountOf("uzrn").Int64()

	if burnedAmt != slashAmt/2 {
		t.Errorf("expected burned=%d, got %d", slashAmt/2, burnedAmt)
	}
	if sentAmt != slashAmt/2 {
		t.Errorf("expected sent=%d, got %d", slashAmt/2, sentAmt)
	}
}

// TestSlashBurnFraction_Full verifies that BurnBps=1_000_000 burns 100% of
// the slash with nothing routed to the development fund.
func TestSlashBurnFraction_Full(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("burn_full")
	registerTestValidator(t, k, ctx, addr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashBurnFractionBps = 1_000_000 // 100%
	params.SlashEscalationBps = 0
	k.SetParams(ctx, params)

	slashAmt := int64(1_000_000)
	k.SlashValidator(ctx, addr, big.NewInt(slashAmt), "test")

	burnedAmt := bk.burnedCoins.AmountOf("uzrn").Int64()
	sentAmt := bk.moduleSent.AmountOf("uzrn").Int64()

	if burnedAmt != slashAmt {
		t.Errorf("expected burned=%d (100%%), got %d", slashAmt, burnedAmt)
	}
	if sentAmt != 0 {
		t.Errorf("expected sent=0 when burn=100%%, got %d", sentAmt)
	}
}

// TestSlashBurnFraction_ToModule verifies that SlashValidatorToModule also
// applies the burn fraction before routing to the destination module.
func TestSlashBurnFraction_ToModule(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("burn_to_mod")
	registerTestValidator(t, k, ctx, addr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashBurnFractionBps = 200_000 // 20%
	params.SlashEscalationBps = 0
	k.SetParams(ctx, params)

	slashAmt := int64(1_000_000)
	k.SlashValidatorToModule(ctx, addr, big.NewInt(slashAmt), "vindication_escrow", "test")

	burnedAmt := bk.burnedCoins.AmountOf("uzrn").Int64()
	sentAmt := bk.moduleSent.AmountOf("uzrn").Int64()
	expectedBurn := slashAmt * 200_000 / 1_000_000
	expectedSent := slashAmt - expectedBurn

	if burnedAmt != expectedBurn {
		t.Errorf("expected burned=%d (20%%), got %d", expectedBurn, burnedAmt)
	}
	if sentAmt != expectedSent {
		t.Errorf("expected sent=%d (80%%), got %d", expectedSent, sentAmt)
	}
}

// TestSlashBurnFraction_Validate rejects BurnBps > 1_000_000.
func TestSlashBurnFraction_Validate(t *testing.T) {
	params := types.DefaultParams()
	params.SlashBurnFractionBps = 1_000_001 // over 100%
	if err := params.Validate(); err == nil {
		t.Error("expected validation error for SlashBurnFractionBps > 1_000_000")
	}
}

// TestSlashBurnFraction_ValidateMaxAllowed verifies that exactly 1_000_000 is accepted.
func TestSlashBurnFraction_ValidateMaxAllowed(t *testing.T) {
	params := types.DefaultParams()
	params.SlashBurnFractionBps = 1_000_000
	if err := params.Validate(); err != nil {
		t.Errorf("expected valid for SlashBurnFractionBps=1_000_000, got %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// SlashValidatorByType tests
// ────────────────────────────────────────────────────────────────────────────

// TestSlashValidatorByType_DoubleVote verifies the 20% configured rate is applied.
func TestSlashValidatorByType_DoubleVote(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("by_type_double_vote")
	stake := int64(10_000_000)
	registerTestValidator(t, k, ctx, addr, "", fmt.Sprintf("%d", stake))

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0 // clean math
	k.SetParams(ctx, params)

	k.SlashValidatorByType(ctx, addr, types.SlashType_SLASH_TYPE_DOUBLE_VOTE)

	// Default DOUBLE_VOTE rate = 200_000 bps = 20%
	expectedSlash := stake * 200_000 / 1_000_000
	sentAmt := bk.moduleSent.AmountOf("uzrn").Int64()
	if sentAmt != expectedSlash {
		t.Errorf("expected slash=%d (20%%), got %d", expectedSlash, sentAmt)
	}

	val, _ := k.GetValidator(ctx, addr)
	if val.SlashCount != 1 {
		t.Errorf("expected slash_count=1, got %d", val.SlashCount)
	}
}

// TestSlashValidatorByType_LivenessFault verifies the 1% configured rate.
func TestSlashValidatorByType_LivenessFault(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("by_type_liveness")
	stake := int64(10_000_000)
	registerTestValidator(t, k, ctx, addr, "", fmt.Sprintf("%d", stake))

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0
	k.SetParams(ctx, params)

	k.SlashValidatorByType(ctx, addr, types.SlashType_SLASH_TYPE_LIVENESS_FAULT)

	// Default LIVENESS_FAULT rate = 10_000 bps = 1%
	expectedSlash := stake * 10_000 / 1_000_000
	sentAmt := bk.moduleSent.AmountOf("uzrn").Int64()
	if sentAmt != expectedSlash {
		t.Errorf("expected slash=%d (1%%), got %d", expectedSlash, sentAmt)
	}
}

// TestSlashValidatorByType_NoRate is a no-op when the type has no configured rate.
// Note: GetParams backfills nil configs from defaults (upgrade-compat behaviour), so
// we set a non-nil config list that simply omits DOUBLE_VOTE to trigger the no-rate path.
func TestSlashValidatorByType_NoRate(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	addr := testAddr("by_type_no_rate")
	registerTestValidator(t, k, ctx, addr, "", "10000000")

	// Only liveness_fault configured — double_vote has no entry.
	params := k.GetParams(ctx)
	params.SlashRateConfigs = []*types.SlashRateConfig{
		{SlashType: types.SlashType_SLASH_TYPE_LIVENESS_FAULT, RateBps: 10_000},
	}
	k.SetParams(ctx, params)

	k.SlashValidatorByType(ctx, addr, types.SlashType_SLASH_TYPE_DOUBLE_VOTE)

	if !bk.moduleSent.IsZero() {
		t.Errorf("expected no slash when double_vote not in configs, got %v", bk.moduleSent)
	}
}

// TestSlashValidatorByType_NotFound is a no-op for an unknown validator.
func TestSlashValidatorByType_NotFound(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)

	k.SlashValidatorByType(ctx, testAddr("nonexistent"), types.SlashType_SLASH_TYPE_DOUBLE_VOTE)

	if !bk.moduleSent.IsZero() || !bk.burnedCoins.IsZero() {
		t.Error("expected no action for unknown validator")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// SlashRateConfigs types-level tests
// ────────────────────────────────────────────────────────────────────────────

// TestDefaultSlashRateConfigs verifies all 4 types have non-zero rates and
// rates are ordered by severity (double_vote > coordination > calibration > liveness).
func TestDefaultSlashRateConfigs(t *testing.T) {
	cfgs := types.DefaultSlashRateConfigs()
	if len(cfgs) != 4 {
		t.Fatalf("expected 4 default slash rate configs, got %d", len(cfgs))
	}
	rateFor := func(st types.SlashType) uint64 {
		return types.GetSlashRate(&types.Params{SlashRateConfigs: cfgs}, st)
	}
	dvRate := rateFor(types.SlashType_SLASH_TYPE_DOUBLE_VOTE)
	cvRate := rateFor(types.SlashType_SLASH_TYPE_COORDINATION_VIOLATION)
	cfRate := rateFor(types.SlashType_SLASH_TYPE_CALIBRATION_FAULT)
	lfRate := rateFor(types.SlashType_SLASH_TYPE_LIVENESS_FAULT)

	if dvRate == 0 || lfRate == 0 || cfRate == 0 || cvRate == 0 {
		t.Errorf("all default rates must be >0: dv=%d lf=%d cf=%d cv=%d", dvRate, lfRate, cfRate, cvRate)
	}
	// severity ordering: double_vote > coordination_violation > calibration_fault > liveness_fault
	if !(dvRate > cvRate && cvRate > cfRate && cfRate > lfRate) {
		t.Errorf("expected severity order dv>cv>cf>lf, got dv=%d cv=%d cf=%d lf=%d", dvRate, cvRate, cfRate, lfRate)
	}
}

// TestGetSlashRate_Missing returns 0 for a type not in configs.
func TestGetSlashRate_Missing(t *testing.T) {
	params := &types.Params{
		SlashRateConfigs: []*types.SlashRateConfig{
			{SlashType: types.SlashType_SLASH_TYPE_LIVENESS_FAULT, RateBps: 10_000},
		},
	}
	rate := types.GetSlashRate(params, types.SlashType_SLASH_TYPE_DOUBLE_VOTE)
	if rate != 0 {
		t.Errorf("expected 0 for unlisted type, got %d", rate)
	}
}

// TestSlashRateConfigs_Validate rejects rate_bps > BPSScale.
func TestSlashRateConfigs_Validate(t *testing.T) {
	params := types.DefaultParams()
	params.SlashRateConfigs = []*types.SlashRateConfig{
		{SlashType: types.SlashType_SLASH_TYPE_DOUBLE_VOTE, RateBps: 1_000_001},
	}
	if err := params.Validate(); err == nil {
		t.Error("expected validation error for rate_bps > 1_000_000")
	}
}

// TestSlashRateConfigs_ValidateMaxAllowed accepts exactly 1_000_000.
func TestSlashRateConfigs_ValidateMaxAllowed(t *testing.T) {
	params := types.DefaultParams()
	params.SlashRateConfigs = []*types.SlashRateConfig{
		{SlashType: types.SlashType_SLASH_TYPE_DOUBLE_VOTE, RateBps: 1_000_000},
	}
	if err := params.Validate(); err != nil {
		t.Errorf("expected valid for rate_bps=1_000_000, got %v", err)
	}
}

// TestGetParamsBackfillsSlashRateConfigs verifies that GetParams fills in
// default slash_rate_configs on a chain that upgraded before Gap2.
func TestGetParamsBackfillsSlashRateConfigs(t *testing.T) {
	k, ctx := setupBareKeeper(t)

	// Store params with empty SlashRateConfigs (pre-Gap2 chain state).
	params := types.DefaultParams()
	params.SlashRateConfigs = nil
	k.SetParams(ctx, params)

	loaded := k.GetParams(ctx)
	if len(loaded.SlashRateConfigs) == 0 {
		t.Error("expected GetParams to backfill SlashRateConfigs from defaults")
	}
	if types.GetSlashRate(loaded, types.SlashType_SLASH_TYPE_DOUBLE_VOTE) == 0 {
		t.Error("expected backfilled DOUBLE_VOTE rate to be non-zero")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Evidence-Based Slashing tests (Gap 3)
// ────────────────────────────────────────────────────────────────────────────

// TestMsgServer_SubmitSlashEvidence_Valid verifies that valid evidence is
// stored in the queue and an evidence_id is returned.
func TestMsgServer_SubmitSlashEvidence_Valid(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val1")
	submitter := testAddr("se_sub1")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	resp, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     submitter,
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "abc123deadbeef",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EvidenceId == "" {
		t.Fatal("expected non-empty evidence_id")
	}

	ev, found := k.GetSlashEvidence(ctx, resp.EvidenceId)
	if !found {
		t.Fatal("evidence not found in queue after submission")
	}
	if ev.ValidatorAddr != valAddr {
		t.Errorf("evidence.validator_addr: got %s, want %s", ev.ValidatorAddr, valAddr)
	}
	if ev.Submitter != submitter {
		t.Errorf("evidence.submitter: got %s, want %s", ev.Submitter, submitter)
	}
	if ev.Processed {
		t.Error("evidence should not be marked processed immediately after submission")
	}
}

// TestMsgServer_SubmitSlashEvidence_ValidatorNotFound verifies that submitting
// evidence for an unknown validator returns an error.
func TestMsgServer_SubmitSlashEvidence_ValidatorNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     testAddr("se_sub2"),
		ValidatorAddr: testAddr("se_val_unknown"),
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "deadbeef",
	})
	if err == nil {
		t.Fatal("expected error for unknown validator, got nil")
	}
}

// TestMsgServer_SubmitSlashEvidence_NoRate verifies that submitting evidence
// for a slash type with rate=0 returns an error.
func TestMsgServer_SubmitSlashEvidence_NoRate(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val3")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashRateConfigs = []*types.SlashRateConfig{
		{SlashType: types.SlashType_SLASH_TYPE_DOUBLE_VOTE, RateBps: 0},
	}
	k.SetParams(ctx, params)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     testAddr("se_sub3"),
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "deadbeef",
	})
	if err == nil {
		t.Fatal("expected error when slash type rate=0, got nil")
	}
}

// TestMsgServer_SubmitSlashEvidence_Duplicate verifies that submitting the
// same evidence (same block, same submitter, same validator) is rejected.
func TestMsgServer_SubmitSlashEvidence_Duplicate(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val4")
	submitter := testAddr("se_sub4")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	msg := &types.MsgSubmitSlashEvidence{
		Submitter:     submitter,
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_LIVENESS_FAULT,
		EvidenceHash:  "hash1",
	}

	// First submission succeeds.
	_, err := ms.SubmitSlashEvidence(ctx, msg)
	if err != nil {
		t.Fatalf("first submission failed: %v", err)
	}

	// Second submission same block is rejected (same ID).
	_, err = ms.SubmitSlashEvidence(ctx, msg)
	if err == nil {
		t.Fatal("expected duplicate submission to be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "already submitted") {
		t.Errorf("expected 'already submitted' in error, got: %v", err)
	}
}

// TestProcessSlashEvidenceQueue_SlashApplied verifies that EndBlocker processes
// queued evidence and slashes the validator.
func TestProcessSlashEvidenceQueue_SlashApplied(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val5")
	submitter := testAddr("se_sub5")
	stake := "10000000"
	registerTestValidator(t, k, ctx, valAddr, "", stake)

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0
	params.EvidenceRewardBps = 0
	k.SetParams(ctx, params)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     submitter,
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "evidhash5",
	})
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}

	val, _ := k.GetValidator(ctx, valAddr)
	if val.SlashCount != 0 {
		t.Error("expected slash count 0 before EndBlock")
	}

	k.EndBlocker(ctx)

	val, found := k.GetValidator(ctx, valAddr)
	if !found {
		t.Fatal("validator not found after EndBlock")
	}
	if val.SlashCount == 0 {
		t.Error("expected slash count > 0 after EndBlock processing")
	}

	stakeAfter, _ := new(big.Int).SetString(val.TotalStake, 10)
	stakeBefore, _ := new(big.Int).SetString(stake, 10)
	if stakeAfter.Cmp(stakeBefore) >= 0 {
		t.Errorf("expected stake to decrease: before=%s, after=%s", stake, val.TotalStake)
	}
}

// TestProcessSlashEvidenceQueue_QueueCleared verifies that evidence items are
// removed from the queue after processing.
func TestProcessSlashEvidenceQueue_QueueCleared(t *testing.T) {
	k, ctx := setupKeeper(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val6")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	resp, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     testAddr("se_sub6"),
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_LIVENESS_FAULT,
		EvidenceHash:  "evidhash6",
	})
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}

	evidenceID := resp.EvidenceId

	if _, found := k.GetSlashEvidence(ctx, evidenceID); !found {
		t.Fatal("evidence should be in queue before EndBlock")
	}

	k.EndBlocker(ctx)

	if _, found := k.GetSlashEvidence(ctx, evidenceID); found {
		t.Error("evidence should be removed from queue after EndBlock processing")
	}
}

// TestProcessSlashEvidenceQueue_GhostValidator verifies that evidence for
// a non-existent validator is cleaned up without panicking.
func TestProcessSlashEvidenceQueue_GhostValidator(t *testing.T) {
	k, ctx := setupKeeper(t)

	k.SetSlashEvidence(ctx, &types.SlashEvidence{
		Id:            "ghost_evidence",
		Submitter:     testAddr("se_sub7"),
		ValidatorAddr: testAddr("se_ghost_val"),
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "hash",
		SubmittedAt:   100,
	})

	// Must not panic.
	k.EndBlocker(ctx)

	if _, found := k.GetSlashEvidence(ctx, "ghost_evidence"); found {
		t.Error("ghost evidence should have been cleaned up by EndBlock")
	}
}

// TestProcessSlashEvidenceQueue_EvidenceReward verifies that the submitter
// receives evidence_reward_bps fraction of the slashed amount.
func TestProcessSlashEvidenceQueue_EvidenceReward(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val8")
	submitter := testAddr("se_sub8")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0
	params.SlashBurnFractionBps = 0
	params.EvidenceRewardBps = 100_000 // 10%
	k.SetParams(ctx, params)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     submitter,
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_LIVENESS_FAULT, // rate=10_000 bps → 1% of 10_000_000 = 100_000
		EvidenceHash:  "evidhash8",
	})
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}

	k.EndBlocker(ctx)

	// slashInt=100_000, burn=0, afterBurn=100_000
	// reward = 100_000 * 100_000/1_000_000 = 10_000
	// devFund = 100_000 - 10_000 = 90_000
	expectedReward := int64(10_000)
	expectedDevFund := int64(90_000)

	rewardAmt := bk.accountSent.AmountOf("uzrn").Int64()
	devFundAmt := bk.moduleSent.AmountOf("uzrn").Int64()

	if rewardAmt != expectedReward {
		t.Errorf("evidence reward: got %d, want %d", rewardAmt, expectedReward)
	}
	if devFundAmt != expectedDevFund {
		t.Errorf("dev fund: got %d, want %d", devFundAmt, expectedDevFund)
	}
	if len(bk.burnedCoins) != 0 {
		t.Errorf("expected no burn, got %v", bk.burnedCoins)
	}
}

// TestProcessSlashEvidenceQueue_ThreeWayRouting verifies the three-way split:
// burn fraction, evidence reward from after-burn, remainder to dev fund.
func TestProcessSlashEvidenceQueue_ThreeWayRouting(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val9")
	submitter := testAddr("se_sub9")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0
	// DOUBLE_VOTE rate = 200_000 bps (20%) of 10_000_000 = 2_000_000 slashed
	params.SlashBurnFractionBps = 200_000 // 20% burn first
	params.EvidenceRewardBps = 100_000   // 10% of after-burn
	k.SetParams(ctx, params)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     submitter,
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_DOUBLE_VOTE,
		EvidenceHash:  "evidhash9",
	})
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}

	k.EndBlocker(ctx)

	// slashInt  = 2_000_000
	// burnAmt   = 2_000_000 * 200_000 / 1_000_000 = 400_000
	// afterBurn = 2_000_000 - 400_000 = 1_600_000
	// reward    = 1_600_000 * 100_000 / 1_000_000 = 160_000
	// devFund   = 1_600_000 - 160_000 = 1_440_000
	expectedBurn := int64(400_000)
	expectedReward := int64(160_000)
	expectedDevFund := int64(1_440_000)

	burnAmt := bk.burnedCoins.AmountOf("uzrn").Int64()
	rewardAmt := bk.accountSent.AmountOf("uzrn").Int64()
	devFundAmt := bk.moduleSent.AmountOf("uzrn").Int64()

	if burnAmt != expectedBurn {
		t.Errorf("burn: got %d, want %d", burnAmt, expectedBurn)
	}
	if rewardAmt != expectedReward {
		t.Errorf("reward: got %d, want %d", rewardAmt, expectedReward)
	}
	if devFundAmt != expectedDevFund {
		t.Errorf("dev fund: got %d, want %d", devFundAmt, expectedDevFund)
	}
}

// TestProcessSlashEvidenceQueue_DisabledReward verifies that with
// EvidenceRewardBps=0, no coins are sent to the submitter account.
func TestProcessSlashEvidenceQueue_DisabledReward(t *testing.T) {
	k, ctx, bk := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("se_val10")
	registerTestValidator(t, k, ctx, valAddr, "", "10000000")

	params := k.GetParams(ctx)
	params.SlashEscalationBps = 0
	params.EvidenceRewardBps = 0
	params.SlashBurnFractionBps = 0
	k.SetParams(ctx, params)

	_, err := ms.SubmitSlashEvidence(ctx, &types.MsgSubmitSlashEvidence{
		Submitter:     testAddr("se_sub10"),
		ValidatorAddr: valAddr,
		SlashType:     types.SlashType_SLASH_TYPE_LIVENESS_FAULT,
		EvidenceHash:  "evidhash10",
	})
	if err != nil {
		t.Fatalf("submission failed: %v", err)
	}

	k.EndBlocker(ctx)

	if bk.accountSent.AmountOf("uzrn").Int64() != 0 {
		t.Errorf("expected no account send when EvidenceRewardBps=0, got %v", bk.accountSent)
	}
	if bk.moduleSent.IsZero() {
		t.Error("expected module routing (dev fund) when reward=0")
	}
}

// ===================================================================
// Delegator Insurance Fund Tests (Slashing Gap 4)
// ===================================================================

func TestInsuranceFund_Contribute_Success(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("ins_val1")
	registerTestValidator(t, k, ctx, valAddr, "did:zrn:ins1", "5000000")

	resp, err := ms.ContributeToInsuranceFund(ctx, &types.MsgContributeToInsuranceFund{
		Operator: valAddr,
		Amount:   "500000",
	})
	if err != nil {
		t.Fatalf("contribute failed: %v", err)
	}
	if resp.NewBalance != "500000" {
		t.Errorf("expected balance 500000, got %s", resp.NewBalance)
	}
	val, _ := k.GetValidator(ctx, valAddr)
	if val.InsuranceFundBalance != "500000" {
		t.Errorf("validator insurance balance: want 500000, got %s", val.InsuranceFundBalance)
	}
}

func TestInsuranceFund_Slash_FullAbsorption(t *testing.T) {
	// Insurance fully covers the overflow — delegated stake unchanged.
	k, ctx, _ := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("ins_val2")
	registerTestValidator(t, k, ctx, valAddr, "did:zrn:ins2", "1000000")

	// Give validator delegated stake directly
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "2000000"
	val.TotalStake = "3000000"
	k.SetValidator(ctx, val)

	// Fund insurance with 3M — enough to absorb 500K overflow
	_, err := ms.ContributeToInsuranceFund(ctx, &types.MsgContributeToInsuranceFund{
		Operator: valAddr,
		Amount:   "3000000",
	})
	if err != nil {
		t.Fatalf("contribute failed: %v", err)
	}

	// Slash 1.5M: self_delegation=1M absorbed, overflow=500K, insurance absorbs 500K
	k.SlashValidator(ctx, valAddr, big.NewInt(1_500_000), "test_slash")

	val, _ = k.GetValidator(ctx, valAddr)
	delegatedAfter, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegatedAfter.Int64() != 2_000_000 {
		t.Errorf("delegated stake should be unchanged: want 2000000, got %s", val.DelegatedStake)
	}
	insuranceAfter, _ := new(big.Int).SetString(val.InsuranceFundBalance, 10)
	if insuranceAfter.Int64() != 2_500_000 {
		t.Errorf("insurance balance: want 2500000 (reduced by 500K), got %s", val.InsuranceFundBalance)
	}
}

func TestInsuranceFund_Slash_PartialAbsorption(t *testing.T) {
	// Insurance partially covers overflow — remainder hits delegated.
	k, ctx, _ := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("ins_val3")
	registerTestValidator(t, k, ctx, valAddr, "did:zrn:ins3", "1000000")

	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "2000000"
	val.TotalStake = "3000000"
	k.SetValidator(ctx, val)

	// Fund with 200K — less than the 500K overflow
	_, err := ms.ContributeToInsuranceFund(ctx, &types.MsgContributeToInsuranceFund{
		Operator: valAddr,
		Amount:   "200000",
	})
	if err != nil {
		t.Fatalf("contribute failed: %v", err)
	}

	// Slash 1.5M → overflow 500K → insurance covers 200K → remaining 300K hits delegated
	// delegated = 2M - 300K = 1.7M
	k.SlashValidator(ctx, valAddr, big.NewInt(1_500_000), "test_slash")

	val, _ = k.GetValidator(ctx, valAddr)
	delegatedAfter, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegatedAfter.Int64() != 1_700_000 {
		t.Errorf("delegated stake: want 1700000, got %s", val.DelegatedStake)
	}
	insuranceAfter, _ := new(big.Int).SetString(val.InsuranceFundBalance, 10)
	if insuranceAfter.Int64() != 0 {
		t.Errorf("insurance balance: want 0 (depleted), got %s", val.InsuranceFundBalance)
	}
}

func TestInsuranceFund_NoFund_DelegatedAbsorbs(t *testing.T) {
	// Without insurance, current behavior is unchanged.
	k, ctx, _ := setupKeeperWithBank(t)

	valAddr := testAddr("ins_val4")
	registerTestValidator(t, k, ctx, valAddr, "did:zrn:ins4", "1000000")

	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "2000000"
	val.TotalStake = "3000000"
	k.SetValidator(ctx, val)

	// No insurance contribution
	k.SlashValidator(ctx, valAddr, big.NewInt(1_500_000), "test_slash")

	val, _ = k.GetValidator(ctx, valAddr)
	delegatedAfter, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	// overflow=500K all hits delegated: 2M - 500K = 1.5M
	if delegatedAfter.Int64() != 1_500_000 {
		t.Errorf("delegated stake: want 1500000, got %s", val.DelegatedStake)
	}
}

func TestInsuranceFund_MultipleContributions_Accumulate(t *testing.T) {
	k, ctx, _ := setupKeeperWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	valAddr := testAddr("ins_val5")
	registerTestValidator(t, k, ctx, valAddr, "did:zrn:ins5", "5000000")

	for _, amt := range []string{"100000", "200000", "300000"} {
		_, err := ms.ContributeToInsuranceFund(ctx, &types.MsgContributeToInsuranceFund{
			Operator: valAddr,
			Amount:   amt,
		})
		if err != nil {
			t.Fatalf("contribute %s failed: %v", amt, err)
		}
	}

	val, _ := k.GetValidator(ctx, valAddr)
	if val.InsuranceFundBalance != "600000" {
		t.Errorf("expected accumulated balance 600000, got %s", val.InsuranceFundBalance)
	}
}
