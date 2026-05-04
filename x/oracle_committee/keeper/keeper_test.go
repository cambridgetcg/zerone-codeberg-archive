package keeper_test

import (
	"context"
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

	"github.com/zerone-chain/zerone/x/oracle_committee/keeper"
	"github.com/zerone-chain/zerone/x/oracle_committee/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

// ── Mock bank keeper ──────────────────────────────────────────────

type mockBankKeeper struct {
	moduleBalance map[string]sdkmath.Int // module → total uzrn held
}

func newMockBankKeeper() *mockBankKeeper {
	return &mockBankKeeper{moduleBalance: make(map[string]sdkmath.Int)}
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ sdk.Context, _ sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	prev := m.moduleBalance[recipientModule]
	if prev.IsNil() {
		prev = sdkmath.ZeroInt()
	}
	for _, c := range amt {
		if c.Denom == "uzrn" {
			prev = prev.Add(c.Amount)
		}
	}
	m.moduleBalance[recipientModule] = prev
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ sdk.Context, senderModule string, _ sdk.AccAddress, amt sdk.Coins) error {
	prev := m.moduleBalance[senderModule]
	if prev.IsNil() {
		prev = sdkmath.ZeroInt()
	}
	for _, c := range amt {
		if c.Denom == "uzrn" {
			prev = prev.Sub(c.Amount)
		}
	}
	m.moduleBalance[senderModule] = prev
	return nil
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(_ sdk.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	for _, c := range amt {
		if c.Denom != "uzrn" {
			continue
		}
		s := m.moduleBalance[senderModule]
		if s.IsNil() {
			s = sdkmath.ZeroInt()
		}
		r := m.moduleBalance[recipientModule]
		if r.IsNil() {
			r = sdkmath.ZeroInt()
		}
		m.moduleBalance[senderModule] = s.Sub(c.Amount)
		m.moduleBalance[recipientModule] = r.Add(c.Amount)
	}
	return nil
}

// contextKey is used to prevent unused import warning.
var _ context.Context = (*sdk.Context)(nil)

// ── Test setup ────────────────────────────────────────────────────

const (
	testAuthority = "zrn1authority"
	testChainID   = "zerone-test-1"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *mockBankKeeper) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	mockBK := newMockBankKeeper()
	storeService := runtime.NewKVStoreService(storeKey)
	k := keeper.NewKeeper(cdc, storeService, mockBK, testAuthority)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100, ChainID: testChainID}, false, log.NewNopLogger())
	return k, ctx, mockBK
}

// validOperatorAddr returns a deterministic bech32 test address.
// Index 1-9 produce distinct addresses.
func validOperatorAddr(t *testing.T, idx int) string {
	t.Helper()
	// Use the authority address with a suffix embedded in the 20-byte addr slice.
	b := make([]byte, 20)
	b[19] = byte(idx)
	return sdk.AccAddress(b).String()
}

// registerN registers n operators with 5_000_000 uzrn each. Returns their addresses.
func registerN(t *testing.T, k keeper.Keeper, ctx sdk.Context, n int) []string {
	t.Helper()
	var addrs []string
	for i := 1; i <= n; i++ {
		addr := validOperatorAddr(t, i)
		uri := "https://oracle" + string(rune('0'+i)) + ".example.com"
		if err := k.RegisterOperator(ctx, addr, uri, "5000000"); err != nil {
			t.Fatalf("register operator %d: %v", i, err)
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

// ── Unit tests: Aggregate (pure function) ────────────────────────

// TestAggregate_EmptyInput: no evaluations → uncertain, confidence 0.
func TestAggregate_EmptyInput(t *testing.T) {
	agg := keeper.Aggregate("claim1", nil, nil, 100)
	if agg.FinalVerdict != "uncertain" {
		t.Errorf("want uncertain, got %s", agg.FinalVerdict)
	}
	if agg.OperatorCount != 0 {
		t.Errorf("want 0 operators, got %d", agg.OperatorCount)
	}
}

// TestAggregate_UnanimousTrue: all operators submit "true" → final verdict "true".
func TestAggregate_UnanimousTrue(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "1000000"},
		{Address: "op2", Stake: "1000000"},
		{Address: "op3", Stake: "1000000"},
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c1", Operator: "op1", Verdict: "true", ConfidenceBps: 8000},
		{ClaimId: "c1", Operator: "op2", Verdict: "true", ConfidenceBps: 9000},
		{ClaimId: "c1", Operator: "op3", Verdict: "true", ConfidenceBps: 7000},
	}
	agg := keeper.Aggregate("c1", evals, ops, 200)
	if agg.FinalVerdict != "true" {
		t.Errorf("want true, got %s", agg.FinalVerdict)
	}
	if agg.OperatorCount != 3 {
		t.Errorf("want 3, got %d", agg.OperatorCount)
	}
}

// TestAggregate_MajorityFalse: 2-of-3 vote false → final verdict "false".
func TestAggregate_MajorityFalse(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "1000000"},
		{Address: "op2", Stake: "1000000"},
		{Address: "op3", Stake: "1000000"},
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c2", Operator: "op1", Verdict: "false", ConfidenceBps: 8000},
		{ClaimId: "c2", Operator: "op2", Verdict: "false", ConfidenceBps: 7000},
		{ClaimId: "c2", Operator: "op3", Verdict: "true", ConfidenceBps: 9000},
	}
	agg := keeper.Aggregate("c2", evals, ops, 200)
	if agg.FinalVerdict != "false" {
		t.Errorf("want false, got %s", agg.FinalVerdict)
	}
}

// TestAggregate_TieGoesToUncertain: equal stake split → "uncertain".
func TestAggregate_TieGoesToUncertain(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "1000000"},
		{Address: "op2", Stake: "1000000"},
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c3", Operator: "op1", Verdict: "true", ConfidenceBps: 9000},
		{ClaimId: "c3", Operator: "op2", Verdict: "false", ConfidenceBps: 9000},
	}
	agg := keeper.Aggregate("c3", evals, ops, 200)
	if agg.FinalVerdict != "uncertain" {
		t.Errorf("want uncertain on tie, got %s", agg.FinalVerdict)
	}
}

// TestAggregate_StakeWeightedMajority: one high-stake operator outweighs two low-stake operators.
func TestAggregate_StakeWeightedMajority(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "10000000"}, // 10M — heavy
		{Address: "op2", Stake: "1000000"},  // 1M
		{Address: "op3", Stake: "1000000"},  // 1M
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c4", Operator: "op1", Verdict: "true", ConfidenceBps: 8000},
		{ClaimId: "c4", Operator: "op2", Verdict: "false", ConfidenceBps: 9000},
		{ClaimId: "c4", Operator: "op3", Verdict: "false", ConfidenceBps: 9000},
	}
	agg := keeper.Aggregate("c4", evals, ops, 200)
	// op1 stake 10M > op2+op3 combined 2M for "false"
	if agg.FinalVerdict != "true" {
		t.Errorf("want true (stake-weighted), got %s", agg.FinalVerdict)
	}
}

// TestAggregate_MedianConfidence: three evals → median picks the middle value.
func TestAggregate_MedianConfidence(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "1000000"},
		{Address: "op2", Stake: "1000000"},
		{Address: "op3", Stake: "1000000"},
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c5", Operator: "op1", Verdict: "true", ConfidenceBps: 3000},
		{ClaimId: "c5", Operator: "op2", Verdict: "true", ConfidenceBps: 7000},
		{ClaimId: "c5", Operator: "op3", Verdict: "true", ConfidenceBps: 9000},
	}
	agg := keeper.Aggregate("c5", evals, ops, 200)
	// sorted: 3000, 7000, 9000. Equal weights → median at cumulative >= 50%: 7000.
	if agg.AggregatedConfidenceBps != 7000 {
		t.Errorf("want median confidence 7000, got %d", agg.AggregatedConfidenceBps)
	}
}

// TestAggregate_SingleEval: single evaluator → their verdict and confidence.
func TestAggregate_SingleEval(t *testing.T) {
	ops := []*types.OracleOperator{
		{Address: "op1", Stake: "1000000"},
	}
	evals := []*types.OracleEvaluationRecord{
		{ClaimId: "c6", Operator: "op1", Verdict: "false", ConfidenceBps: 6500},
	}
	agg := keeper.Aggregate("c6", evals, ops, 200)
	if agg.FinalVerdict != "false" {
		t.Errorf("want false, got %s", agg.FinalVerdict)
	}
	if agg.AggregatedConfidenceBps != 6500 {
		t.Errorf("want 6500, got %d", agg.AggregatedConfidenceBps)
	}
}

// ── Keeper tests: registration ────────────────────────────────────

// TestRegisterOperator_Basic: registers and retrieves.
func TestRegisterOperator_Basic(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addr := validOperatorAddr(t, 1)
	if err := k.RegisterOperator(ctx, addr, "https://oracle.example.com", "5000000"); err != nil {
		t.Fatalf("register: %v", err)
	}
	op, found := k.GetOperator(ctx, addr)
	if !found {
		t.Fatal("operator not found after register")
	}
	if !op.Active {
		t.Error("want active=true after register")
	}
	if op.Uri != "https://oracle.example.com" {
		t.Errorf("want URI set, got %s", op.Uri)
	}
	if op.Stake != "5000000" {
		t.Errorf("want stake 5000000, got %s", op.Stake)
	}
}

// TestRegisterOperator_Duplicate: registering the same address twice fails.
func TestRegisterOperator_Duplicate(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addr := validOperatorAddr(t, 1)
	_ = k.RegisterOperator(ctx, addr, "https://a.com", "5000000")
	err := k.RegisterOperator(ctx, addr, "https://b.com", "5000000")
	if err != types.ErrOperatorAlreadyExists {
		t.Errorf("want ErrOperatorAlreadyExists, got %v", err)
	}
}

// TestRegisterOperator_InsufficientStake: stake below minimum is rejected.
func TestRegisterOperator_InsufficientStake(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	// Default min stake is 1_000_000. Submit 999_999.
	addr := validOperatorAddr(t, 1)
	err := k.RegisterOperator(ctx, addr, "https://a.com", "999999")
	if err != types.ErrInsufficientStake {
		t.Errorf("want ErrInsufficientStake, got %v", err)
	}
}

// TestDeregisterOperator: register then deregister; operator removed.
func TestDeregisterOperator(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addr := validOperatorAddr(t, 1)
	_ = k.RegisterOperator(ctx, addr, "https://a.com", "5000000")
	if err := k.DeregisterOperator(ctx, addr); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	_, found := k.GetOperator(ctx, addr)
	if found {
		t.Error("operator still present after deregister")
	}
}

// TestDeregisterOperator_NotFound: deregistering unknown operator fails.
func TestDeregisterOperator_NotFound(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	err := k.DeregisterOperator(ctx, validOperatorAddr(t, 99))
	if err != types.ErrOperatorNotFound {
		t.Errorf("want ErrOperatorNotFound, got %v", err)
	}
}

// ── Keeper tests: evaluation submission ──────────────────────────

// TestSubmitEvaluation_NoQuorum: 3 operators, only 1 submits — no quorum (60% of 3 = 2).
func TestSubmitEvaluation_NoQuorum(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	agg, quorum, err := k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 8000, "first", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if quorum {
		t.Error("want no quorum after 1-of-3 submission")
	}
	if agg != nil {
		t.Error("want nil aggregation before quorum")
	}
}

// TestSubmitEvaluation_QuorumReached: 3 operators, 2 submit (≥ 60%) → quorum.
func TestSubmitEvaluation_QuorumReached(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 8000, "first", "")
	agg, quorum, err := k.SubmitEvaluation(ctx, addrs[1], "claim1", "true", 9000, "second", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !quorum {
		t.Error("want quorum after 2-of-3 submission (60%)")
	}
	if agg == nil {
		t.Fatal("want aggregation after quorum")
	}
	if agg.FinalVerdict != "true" {
		t.Errorf("want true, got %s", agg.FinalVerdict)
	}
}

// TestSubmitEvaluation_DuplicateRejected: same operator submits twice.
func TestSubmitEvaluation_DuplicateRejected(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 8000, "first", "")
	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 9000, "duplicate", "")
	if err != types.ErrDuplicateEvaluation {
		t.Errorf("want ErrDuplicateEvaluation, got %v", err)
	}
}

// TestSubmitEvaluation_InvalidVerdict: unsupported verdict string.
func TestSubmitEvaluation_InvalidVerdict(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claim1", "maybe", 8000, "", "")
	if err != types.ErrInvalidVerdict {
		t.Errorf("want ErrInvalidVerdict, got %v", err)
	}
}

// TestSubmitEvaluation_ConfidenceOutOfRange: confidence > 10000.
func TestSubmitEvaluation_ConfidenceOutOfRange(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 10001, "", "")
	if err != types.ErrInvalidConfidence {
		t.Errorf("want ErrInvalidConfidence, got %v", err)
	}
}

// TestSubmitEvaluation_AfterAggregation: submitting after quorum already reached.
func TestSubmitEvaluation_AfterAggregation(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	// Reach quorum.
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 8000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[1], "claim1", "true", 9000, "", "")
	// Third operator tries to submit after aggregation exists.
	_, _, err := k.SubmitEvaluation(ctx, addrs[2], "claim1", "true", 7000, "", "")
	if err != types.ErrAggregationExists {
		t.Errorf("want ErrAggregationExists, got %v", err)
	}
}

// TestSubmitEvaluation_InactiveOperator: deregistered operator cannot submit.
func TestSubmitEvaluation_InactiveOperator(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_ = k.DeregisterOperator(ctx, addrs[0])
	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claim1", "true", 8000, "", "")
	if err != types.ErrOperatorNotFound {
		t.Errorf("want ErrOperatorNotFound for deregistered operator, got %v", err)
	}
}

// ── Keeper tests: quorum calculation ─────────────────────────────

// TestQuorum_ExactlyAtThreshold: ceiling(60% of 5) = 3 → need 3 submissions.
func TestQuorum_ExactlyAtThreshold(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 5)
	// Submit 2 (not enough).
	_, q1, _ := k.SubmitEvaluation(ctx, addrs[0], "claim5", "true", 8000, "", "")
	_, q2, _ := k.SubmitEvaluation(ctx, addrs[1], "claim5", "true", 8000, "", "")
	if q1 || q2 {
		t.Error("want no quorum after 2-of-5 submissions (need ceiling(60%*5)=3)")
	}
	// Submit 3rd — quorum.
	agg, q3, err := k.SubmitEvaluation(ctx, addrs[2], "claim5", "true", 8000, "", "")
	if err != nil || !q3 || agg == nil {
		t.Errorf("want quorum at 3-of-5, q3=%v err=%v agg=%v", q3, err, agg)
	}
}

// ── Keeper tests: slashing ────────────────────────────────────────

// TestSlashOperator_WrongConfidentVerdict: confident + wrong → slashed.
func TestSlashOperator_WrongConfidentVerdict(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	// Module has stake locked from registration (3 operators × 5_000_000).
	initialModuleBalance := bk.moduleBalance[types.ModuleName]

	// op1 submits "true" with high confidence.
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "slashclaim", "true", 8000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[1], "slashclaim", "true", 9000, "", "")

	// Panel adjudicates: final verdict is "false". op1 was confident and wrong.
	if err := k.SlashOperator(ctx, addrs[0], "slashclaim", "false", "distribution"); err != nil {
		t.Fatalf("slash: %v", err)
	}

	op, _ := k.GetOperator(ctx, addrs[0])
	// stake was 5_000_000; slash = 5% = 250_000
	if op.Stake == "5000000" {
		t.Error("want stake reduced after slash")
	}
	if op.SlashCount != 1 {
		t.Errorf("want slash_count 1, got %d", op.SlashCount)
	}

	// Module balance should have decreased by slash amount.
	afterBalance := bk.moduleBalance[types.ModuleName]
	if !afterBalance.LT(initialModuleBalance) {
		t.Error("want module balance reduced after slash")
	}
}

// TestSlashOperator_CorrectVerdict: correct verdict → no slash.
func TestSlashOperator_CorrectVerdict(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "slashclaim2", "true", 8000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[1], "slashclaim2", "true", 9000, "", "")

	// Panel verdict matches: no slash.
	_ = k.SlashOperator(ctx, addrs[0], "slashclaim2", "true", "distribution")

	op, _ := k.GetOperator(ctx, addrs[0])
	if op.Stake != "5000000" {
		t.Errorf("want stake unchanged for correct verdict, got %s", op.Stake)
	}
	if op.SlashCount != 0 {
		t.Errorf("want slash_count 0, got %d", op.SlashCount)
	}
}

// TestSlashOperator_LowConfidenceNoSlash: wrong verdict but low confidence (≤5000) → no slash.
func TestSlashOperator_LowConfidenceNoSlash(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "slashclaim3", "true", 4000, "", "") // low confidence
	_, _, _ = k.SubmitEvaluation(ctx, addrs[1], "slashclaim3", "true", 9000, "", "")

	_ = k.SlashOperator(ctx, addrs[0], "slashclaim3", "false", "distribution")

	op, _ := k.GetOperator(ctx, addrs[0])
	if op.Stake != "5000000" {
		t.Errorf("want stake unchanged for low-confidence wrong verdict, got %s", op.Stake)
	}
}

// ── Keeper tests: params ──────────────────────────────────────────

// TestDefaultParams: default params are returned when not set.
func TestDefaultParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	p := k.GetParams(ctx)
	if p.MinQuorumBps != 6000 {
		t.Errorf("want MinQuorumBps 6000, got %d", p.MinQuorumBps)
	}
	if p.MinCommitteeSize != 3 {
		t.Errorf("want MinCommitteeSize 3, got %d", p.MinCommitteeSize)
	}
	if p.MinStake != "1000000" {
		t.Errorf("want MinStake 1000000, got %s", p.MinStake)
	}
}

// TestSetParams: can update params.
func TestSetParams(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	p := &types.OracleCommitteeParams{
		MinQuorumBps:           7500,
		MinCommitteeSize:       5,
		EvaluationWindowBlocks: 100,
		MaxSlashFractionBps:    1000,
		MinStake:               "2000000",
	}
	if err := k.SetParams(ctx, p); err != nil {
		t.Fatalf("set params: %v", err)
	}
	got := k.GetParams(ctx)
	if got.MinQuorumBps != 7500 {
		t.Errorf("want 7500, got %d", got.MinQuorumBps)
	}
	if got.MinStake != "2000000" {
		t.Errorf("want 2000000, got %s", got.MinStake)
	}
}

// TestCountActiveOperators: counts only active operators.
func TestCountActiveOperators(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 4)
	// Deregister one.
	_ = k.DeregisterOperator(ctx, addrs[0])
	count := k.CountActiveOperators(ctx)
	if count != 3 {
		t.Errorf("want 3 active operators, got %d", count)
	}
}

// ── Keeper tests: GetAllOperators ────────────────────────────────

// TestGetAllOperators: returns correct count.
func TestGetAllOperators(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	registerN(t, k, ctx, 5)
	all := k.GetAllOperators(ctx)
	if len(all) != 5 {
		t.Errorf("want 5 operators, got %d", len(all))
	}
}

// TestGetEvaluationsForClaim: correct evaluation count per claim.
func TestGetEvaluationsForClaim(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 5)
	// Submit 3 evaluations for claim A, 1 for claim B.
	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "claimA", "true", 8000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[1], "claimA", "false", 7000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[2], "claimA", "true", 9000, "", "")
	_, _, _ = k.SubmitEvaluation(ctx, addrs[3], "claimB", "uncertain", 5000, "", "")

	evalsA := k.GetEvaluationsForClaim(ctx, "claimA")
	if len(evalsA) != 3 {
		t.Errorf("want 3 evals for claimA, got %d", len(evalsA))
	}
	evalsB := k.GetEvaluationsForClaim(ctx, "claimB")
	if len(evalsB) != 1 {
		t.Errorf("want 1 eval for claimB, got %d", len(evalsB))
	}
	evalsC := k.GetEvaluationsForClaim(ctx, "claimC")
	if len(evalsC) != 0 {
		t.Errorf("want 0 evals for claimC, got %d", len(evalsC))
	}
}

// ── Gap 4: Oracle Request freshness token tests ───────────────────

// TestGenerateRequestId_Deterministic: same inputs → same nonce.
func TestGenerateRequestId_Deterministic(t *testing.T) {
	id1 := keeper.GenerateRequestId("zerone-1", 100, "claim42", "zrn1abc")
	id2 := keeper.GenerateRequestId("zerone-1", 100, "claim42", "zrn1abc")
	if id1 != id2 {
		t.Errorf("want deterministic nonce, got %s and %s", id1, id2)
	}
}

// TestGenerateRequestId_UniquePerClaim: different claim IDs → different nonces.
func TestGenerateRequestId_UniquePerClaim(t *testing.T) {
	id1 := keeper.GenerateRequestId("zerone-1", 100, "claimA", "zrn1abc")
	id2 := keeper.GenerateRequestId("zerone-1", 100, "claimB", "zrn1abc")
	if id1 == id2 {
		t.Error("want distinct nonces for different claim IDs")
	}
}

// TestGenerateRequestId_UniquePerBlock: different block heights → different nonces.
func TestGenerateRequestId_UniquePerBlock(t *testing.T) {
	id1 := keeper.GenerateRequestId("zerone-1", 100, "claim1", "zrn1abc")
	id2 := keeper.GenerateRequestId("zerone-1", 101, "claim1", "zrn1abc")
	if id1 == id2 {
		t.Error("want distinct nonces for different block heights")
	}
}

// TestGenerateRequestId_IsHexSHA256: nonce is a 64-char hex string (sha256 output).
func TestGenerateRequestId_IsHexSHA256(t *testing.T) {
	id := keeper.GenerateRequestId("zerone-1", 100, "claim1", "zrn1abc")
	if len(id) != 64 {
		t.Errorf("want 64-char hex (sha256), got len=%d val=%s", len(id), id)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("nonce contains non-hex character: %c", c)
			break
		}
	}
}

// TestOpenOracleRequest_StoresNonce: opened request is retrievable.
func TestOpenOracleRequest_StoresNonce(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	requestor := validOperatorAddr(t, 1)
	nonce, err := k.OpenOracleRequest(ctx, "claim99", requestor)
	if err != nil {
		t.Fatalf("OpenOracleRequest: %v", err)
	}
	if nonce == "" {
		t.Fatal("want non-empty nonce")
	}

	req, found := k.GetOracleRequest(ctx, "claim99")
	if !found {
		t.Fatal("request not found after open")
	}
	if req.Nonce != nonce {
		t.Errorf("stored nonce %s != returned nonce %s", req.Nonce, nonce)
	}
	if req.ClaimId != "claim99" {
		t.Errorf("want claim_id 'claim99', got %s", req.ClaimId)
	}
	if req.OpenedAtBlock != 100 {
		t.Errorf("want opened_at_block 100, got %d", req.OpenedAtBlock)
	}
}

// TestOpenOracleRequest_DuplicateRejected: opening twice for the same claim fails.
func TestOpenOracleRequest_DuplicateRejected(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	requestor := validOperatorAddr(t, 1)
	_, _ = k.OpenOracleRequest(ctx, "claimDup", requestor)
	_, err := k.OpenOracleRequest(ctx, "claimDup", requestor)
	if err == nil {
		t.Error("want error on duplicate OpenOracleRequest")
	}
}

// TestSubmitEvaluation_ValidNonce: when request exists, matching nonce is accepted.
func TestSubmitEvaluation_ValidNonce(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	requestor := validOperatorAddr(t, 99)
	nonce, err := k.OpenOracleRequest(ctx, "claimFresh", requestor)
	if err != nil {
		t.Fatalf("OpenOracleRequest: %v", err)
	}

	_, _, err = k.SubmitEvaluation(ctx, addrs[0], "claimFresh", "true", 8000, "reasoning", nonce)
	if err != nil {
		t.Errorf("want no error with valid nonce, got %v", err)
	}
}

// TestSubmitEvaluation_MissingNonce: when request exists, empty nonce is rejected.
func TestSubmitEvaluation_MissingNonce(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	requestor := validOperatorAddr(t, 99)
	_, _ = k.OpenOracleRequest(ctx, "claimFresh2", requestor)

	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claimFresh2", "true", 8000, "", "")
	if err != types.ErrInvalidOracleRequestId {
		t.Errorf("want ErrInvalidOracleRequestId for empty nonce, got %v", err)
	}
}

// TestSubmitEvaluation_WrongNonce: when request exists, wrong nonce is rejected.
func TestSubmitEvaluation_WrongNonce(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	requestor := validOperatorAddr(t, 99)
	_, _ = k.OpenOracleRequest(ctx, "claimFresh3", requestor)

	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claimFresh3", "true", 8000, "", "deadbeef")
	if err != types.ErrInvalidOracleRequestId {
		t.Errorf("want ErrInvalidOracleRequestId for wrong nonce, got %v", err)
	}
}

// TestSubmitEvaluation_NoRequest_Passes: when no request exists, empty nonce is allowed
// (backward compat — old verification paths that don't call OpenOracleRequest).
func TestSubmitEvaluation_NoRequest_Passes(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	// No OpenOracleRequest call — old-style submission without nonce.
	_, _, err := k.SubmitEvaluation(ctx, addrs[0], "claimOld", "true", 8000, "", "")
	if err != nil {
		t.Errorf("want no error when no request exists (backward compat), got %v", err)
	}
}

// TestSubmitEvaluation_NoncePersisted: nonce is stored in the evaluation record.
func TestSubmitEvaluation_NoncePersisted(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	addrs := registerN(t, k, ctx, 3)

	requestor := validOperatorAddr(t, 99)
	nonce, _ := k.OpenOracleRequest(ctx, "claimAudit", requestor)

	_, _, _ = k.SubmitEvaluation(ctx, addrs[0], "claimAudit", "true", 8000, "reasoning", nonce)

	eval, found := k.GetEvaluation(ctx, "claimAudit", addrs[0])
	if !found {
		t.Fatal("evaluation not found")
	}
	if eval.OracleRequestId != nonce {
		t.Errorf("want nonce %s stored in record, got %s", nonce, eval.OracleRequestId)
	}
}
