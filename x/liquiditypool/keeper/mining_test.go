package keeper_test

// mining_test.go — Tests for liquidity mining: accumulator model, settlement,
// claim, CreateLiquidityMiningPeriod, and BeginBlock accumulation.

import (
	"math/big"
	"testing"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

const (
	testPool   = "pool-mining-1"
	addrMiner1 = "zrn1q3syrs7qmr9mrtqvrqtkaf5d5smxkmuav70hwx"
	addrMiner2 = "zrn1ahpgpu4cdeytmjunfyuy6t74y3n9xxf2tlypwq"
)

// seedPool creates a pool with the given lp_token_supply for mining tests.
func seedPool(t *testing.T, k keeper.Keeper, ctx sdk.Context, poolId string, lpSupply string) {
	t.Helper()
	pool := &types.Pool{
		PoolId:        poolId,
		DenomA:        "uzrn",
		DenomB:        "uatom",
		ReserveA:      "1000000",
		ReserveB:      "1000000",
		LpTokenSupply: lpSupply,
		LpDenom:       types.LPDenom(poolId),
		Creator:       testAuthority,
	}
	k.SetPool(ctx, pool)
}

// ---------- GenerateMiningPeriodID ----------

func TestGenerateMiningPeriodID_Deterministic(t *testing.T) {
	id1 := keeper.GenerateMiningPeriodID("pool-1", 100, 200)
	id2 := keeper.GenerateMiningPeriodID("pool-1", 100, 200)
	id3 := keeper.GenerateMiningPeriodID("pool-1", 100, 300)

	if id1 != id2 {
		t.Fatal("same inputs must produce same id")
	}
	if id1 == id3 {
		t.Fatal("different inputs must produce different ids")
	}
	if len(id1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(id1))
	}
}

// ---------- LiquidityMiningPeriod CRUD ----------

func TestMiningPeriod_SetGet(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	id := keeper.GenerateMiningPeriodID(testPool, 100, 200)
	period := &types.LiquidityMiningPeriod{
		Id:             id,
		PoolId:         testPool,
		AmountPerBlock: "1000",
		StartBlock:     100,
		EndBlock:       200,
		Active:         true,
		TotalMined:     "0",
		Creator:        testAuthority,
	}
	k.SetLiquidityMiningPeriod(ctx, period)

	got := k.GetLiquidityMiningPeriod(ctx, id)
	if got == nil {
		t.Fatal("expected period, got nil")
	}
	if got.AmountPerBlock != "1000" {
		t.Fatalf("expected amount 1000, got %s", got.AmountPerBlock)
	}
	if !got.Active {
		t.Fatal("expected active")
	}
}

func TestMiningPeriod_NotFound(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	got := k.GetLiquidityMiningPeriod(ctx, "nonexistent")
	if got != nil {
		t.Fatal("expected nil for nonexistent period")
	}
}

func TestGetActiveMiningPeriodByPool(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	id := keeper.GenerateMiningPeriodID(testPool, 100, 200)
	period := &types.LiquidityMiningPeriod{
		Id: id, PoolId: testPool, AmountPerBlock: "500", StartBlock: 100, EndBlock: 200, Active: true,
	}
	k.SetLiquidityMiningPeriod(ctx, period)
	// No index yet — should be nil
	got := k.GetActiveMiningPeriodByPool(ctx, testPool)
	if got != nil {
		t.Fatal("expected nil before index is set")
	}

	// Create via keeper.CreateMiningPeriod to wire index
	seedPool(t, k, ctx, testPool, "1000")
	p, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "1000", 100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got = k.GetActiveMiningPeriodByPool(ctx, testPool)
	if got == nil {
		t.Fatal("expected active period after CreateMiningPeriod")
	}
	if got.Id != p.Id {
		t.Fatalf("expected id %s, got %s", p.Id, got.Id)
	}
}

// ---------- LpMinerPosition CRUD ----------

func TestLpMinerPosition_SetGet(t *testing.T) {
	k, ctx, _ := setupKeeper(t)

	pos := &types.LpMinerPosition{
		PoolId:         testPool,
		MinerAddr:      addrMiner1,
		RewardDebt:     "1000000",
		PendingRewards: "500",
	}
	k.SetLpMinerPosition(ctx, pos)

	got := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	if got.RewardDebt != "1000000" {
		t.Fatalf("expected debt 1000000, got %s", got.RewardDebt)
	}
	if got.PendingRewards != "500" {
		t.Fatalf("expected pending 500, got %s", got.PendingRewards)
	}
}

func TestLpMinerPosition_Default(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	// Not stored — should return zeroed default
	got := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	if got == nil {
		t.Fatal("expected default position, got nil")
	}
	if got.RewardDebt != "0" {
		t.Fatalf("expected debt 0, got %s", got.RewardDebt)
	}
	if got.PendingRewards != "0" {
		t.Fatalf("expected pending 0, got %s", got.PendingRewards)
	}
}

// ---------- AccumulateMiningRewards ----------

func TestAccumulateMiningRewards_Basic(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000") // 1000 LP tokens outstanding

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id:             "period-1",
		PoolId:         testPool,
		AmountPerBlock: "1000", // 1000 uzrn per block
		StartBlock:     100,
		EndBlock:       200,
		Active:         true,
		TotalMined:     "0",
	}

	k.AccumulateMiningRewards(ctx, pool, period)

	// delta = 1000 × 1e18 / 1000 = 1e18 (i.e., 1 uzrn per LP token)
	pool, _ = k.GetPool(ctx, testPool)
	expectedAcc := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil).String()
	if pool.AccumulatedRewardPerLp != expectedAcc {
		t.Fatalf("expected acc %s, got %s", expectedAcc, pool.AccumulatedRewardPerLp)
	}

	// Module balance should be 1000 uzrn
	modBal := bk.moduleBalances[types.ModuleName]["uzrn"]
	if modBal != 1000 {
		t.Fatalf("expected module balance 1000, got %d", modBal)
	}

	// TotalMined updated
	p := k.GetLiquidityMiningPeriod(ctx, "period-1")
	if p.TotalMined != "1000" {
		t.Fatalf("expected total_mined 1000, got %s", p.TotalMined)
	}
}

func TestAccumulateMiningRewards_ZeroSupply(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "0") // no LP holders

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "period-1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}

	k.AccumulateMiningRewards(ctx, pool, period)

	// No mint, no accumulation
	pool, _ = k.GetPool(ctx, testPool)
	if pool.AccumulatedRewardPerLp != "" && pool.AccumulatedRewardPerLp != "0" {
		t.Fatalf("expected no accumulation for zero supply, got %s", pool.AccumulatedRewardPerLp)
	}
	if bk.moduleBalances[types.ModuleName]["uzrn"] != 0 {
		t.Fatal("expected no mint for zero supply")
	}
}

func TestAccumulateMiningRewards_Accumulates(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "period-1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.SetLiquidityMiningPeriod(ctx, period)

	// Accumulate 3 blocks
	for i := 0; i < 3; i++ {
		pool, _ = k.GetPool(ctx, testPool)
		p := k.GetLiquidityMiningPeriod(ctx, "period-1")
		k.AccumulateMiningRewards(ctx, pool, p)
	}

	pool, _ = k.GetPool(ctx, testPool)
	// After 3 blocks: 3 × 1e18
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	expected := new(big.Int).Mul(big.NewInt(3), scale)
	acc := new(big.Int)
	acc.SetString(pool.AccumulatedRewardPerLp, 10)
	if acc.Cmp(expected) != 0 {
		t.Fatalf("expected %s, got %s", expected.String(), acc.String())
	}
	if bk.moduleBalances[types.ModuleName]["uzrn"] != 3000 {
		t.Fatalf("expected 3000 uzrn in module, got %d", bk.moduleBalances[types.ModuleName]["uzrn"])
	}
}

// ---------- SettleMinerRewards ----------

func TestSettleMinerRewards_Basic(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "period-1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.AccumulateMiningRewards(ctx, pool, period)

	pool, _ = k.GetPool(ctx, testPool)
	lpBal := big.NewInt(500) // miner holds 500/1000 = 50% of LP
	k.SettleMinerRewards(ctx, testPool, addrMiner1, lpBal, pool)

	pos := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	// pending = (1e18 - 0) × 500 / 1e18 = 500
	if pos.PendingRewards != "500" {
		t.Fatalf("expected pending 500, got %s", pos.PendingRewards)
	}
	// debt = pool acc
	if pos.RewardDebt != pool.AccumulatedRewardPerLp {
		t.Fatalf("expected debt == pool acc")
	}
	_ = bk
}

func TestSettleMinerRewards_ZeroBalance(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "period-1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.AccumulateMiningRewards(ctx, pool, period)

	pool, _ = k.GetPool(ctx, testPool)
	// miner has 0 LP
	k.SettleMinerRewards(ctx, testPool, addrMiner1, big.NewInt(0), pool)

	pos := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	// No pending even though acc advanced — miner had no LP
	if pos.PendingRewards != "0" {
		t.Fatalf("expected 0 pending for zero LP, got %s", pos.PendingRewards)
	}
	// Debt still advanced to current acc
	if pos.RewardDebt != pool.AccumulatedRewardPerLp {
		t.Fatal("debt must advance even for zero balance")
	}
	_ = bk
}

func TestSettleMinerRewards_Accumulates(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "p1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.SetLiquidityMiningPeriod(ctx, period)

	// Accumulate 2 blocks
	for i := 0; i < 2; i++ {
		pool, _ = k.GetPool(ctx, testPool)
		p := k.GetLiquidityMiningPeriod(ctx, "p1")
		k.AccumulateMiningRewards(ctx, pool, p)
	}
	pool, _ = k.GetPool(ctx, testPool)

	// Settle for miner holding all 1000 LP
	k.SettleMinerRewards(ctx, testPool, addrMiner1, big.NewInt(1000), pool)

	pos := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	// 2 blocks × 1000 uzrn/block × 1000/1000 = 2000
	if pos.PendingRewards != "2000" {
		t.Fatalf("expected 2000, got %s", pos.PendingRewards)
	}
	_ = bk
}

// ---------- CreateMiningPeriod ----------

func TestCreateMiningPeriod_Success(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "0")

	p, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "500", 100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Active {
		t.Fatal("expected active period")
	}
	if p.AmountPerBlock != "500" {
		t.Fatalf("expected 500, got %s", p.AmountPerBlock)
	}

	// Index wired
	got := k.GetActiveMiningPeriodByPool(ctx, testPool)
	if got == nil || got.Id != p.Id {
		t.Fatal("pool→period index not wired")
	}
}

func TestCreateMiningPeriod_InvalidAmount(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "0")

	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "0", 100, 200)
	if err != types.ErrMiningPeriodInvalid {
		t.Fatalf("expected ErrMiningPeriodInvalid, got %v", err)
	}
}

func TestCreateMiningPeriod_InvalidBlocks(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "0")

	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "1000", 200, 100)
	if err != types.ErrMiningPeriodInvalid {
		t.Fatalf("expected ErrMiningPeriodInvalid, got %v", err)
	}
}

func TestCreateMiningPeriod_DuplicateActive(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "0")

	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "500", 100, 200)
	if err != nil {
		t.Fatalf("first period: unexpected error: %v", err)
	}

	_, err = k.CreateMiningPeriod(ctx, testPool, testAuthority, "1000", 300, 400)
	if err != types.ErrMiningPeriodExists {
		t.Fatalf("expected ErrMiningPeriodExists, got %v", err)
	}
}

// ---------- ClaimPendingRewards ----------

func TestClaimPendingRewards_Success(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	// Give addrMiner1 500 LP in mock bank
	bk.setBalance(addrMiner1, types.LPDenom(testPool), 500)

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "p1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.AccumulateMiningRewards(ctx, pool, period) // mint 1000 to module

	pool, _ = k.GetPool(ctx, testPool)
	claimed, err := k.ClaimPendingRewards(ctx, testPool, addrMiner1, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 500/1000 × 1000 = 500
	if claimed.Int64() != 500 {
		t.Fatalf("expected 500 claimed, got %s", claimed.String())
	}

	// Module paid out 500
	if bk.moduleBalances[types.ModuleName]["uzrn"] != 500 {
		t.Fatalf("expected module balance 500 remaining, got %d", bk.moduleBalances[types.ModuleName]["uzrn"])
	}

	// User received 500
	if bk.balances[addrMiner1]["uzrn"] != 500 {
		t.Fatalf("expected user balance 500, got %d", bk.balances[addrMiner1]["uzrn"])
	}

	// Position cleared
	pos := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	if pos.PendingRewards != "0" {
		t.Fatalf("expected pending cleared, got %s", pos.PendingRewards)
	}
}

func TestClaimPendingRewards_NothingToClaim(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	pool, _ := k.GetPool(ctx, testPool)
	_, err := k.ClaimPendingRewards(ctx, testPool, addrMiner1, pool)
	if err != types.ErrNoRewardsToClaim {
		t.Fatalf("expected ErrNoRewardsToClaim, got %v", err)
	}
	_ = bk
}

// ---------- ProcessMiningBeginBlock ----------

func TestProcessMiningBeginBlock_ActivePeriod(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "2000")

	// Create period starting at block 100 (ctx is already at 100)
	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "2000", 100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	k.ProcessMiningBeginBlock(ctx)

	pool, _ := k.GetPool(ctx, testPool)
	// delta = 2000 × 1e18 / 2000 = 1e18
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	acc := new(big.Int)
	acc.SetString(pool.AccumulatedRewardPerLp, 10)
	if acc.Cmp(scale) != 0 {
		t.Fatalf("expected 1e18, got %s", acc.String())
	}
	_ = bk
}

func TestProcessMiningBeginBlock_PeriodExpiry(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	// Create a period that ends before the current block (100)
	// startBlock=50, endBlock=99 → at block 100, currentBlock > endBlock → expired
	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "1000", 50, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	k.ProcessMiningBeginBlock(ctx)

	// Period should be deactivated
	got := k.GetActiveMiningPeriodByPool(ctx, testPool)
	if got != nil {
		t.Fatal("expected no active period after expiry")
	}
	_ = bk
}

func TestProcessMiningBeginBlock_BeforeStart(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	// Period starts in future (block 200, ctx at 100) — set directly without CreateMiningPeriod
	// so we can bypass index (index check is only needed for duplicate guard)
	period := &types.LiquidityMiningPeriod{
		Id:             "p-future",
		PoolId:         testPool,
		AmountPerBlock: "1000",
		StartBlock:     200, // future
		EndBlock:       300,
		Active:         true,
		TotalMined:     "0",
	}
	k.SetLiquidityMiningPeriod(ctx, period)

	k.ProcessMiningBeginBlock(ctx)

	pool, _ := k.GetPool(ctx, testPool)
	if pool.AccumulatedRewardPerLp != "" && pool.AccumulatedRewardPerLp != "0" {
		t.Fatalf("expected no accumulation before start, got %s", pool.AccumulatedRewardPerLp)
	}
	_ = bk
}

// ---------- MsgServer: CreateLiquidityMiningPeriod ----------

func TestMsg_CreateLiquidityMiningPeriod_Success(t *testing.T) {
	srv, k, ctx, _ := setupMsgServer(t)
	seedPool(t, k, ctx, testPool, "0")

	resp, err := srv.CreateLiquidityMiningPeriod(ctx, &types.MsgCreateLiquidityMiningPeriod{
		Authority:      testAuthority,
		PoolId:         testPool,
		AmountPerBlock: "1000",
		StartBlock:     100,
		EndBlock:       200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PeriodId == "" {
		t.Fatal("expected non-empty period_id")
	}
}

func TestMsg_CreateLiquidityMiningPeriod_Unauthorized(t *testing.T) {
	srv, k, ctx, _ := setupMsgServer(t)
	seedPool(t, k, ctx, testPool, "0")

	_, err := srv.CreateLiquidityMiningPeriod(ctx, &types.MsgCreateLiquidityMiningPeriod{
		Authority:      "zrn1notauthority",
		PoolId:         testPool,
		AmountPerBlock: "1000",
		StartBlock:     100,
		EndBlock:       200,
	})
	if err != types.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestMsg_CreateLiquidityMiningPeriod_PoolNotFound(t *testing.T) {
	srv, _, ctx, _ := setupMsgServer(t)

	_, err := srv.CreateLiquidityMiningPeriod(ctx, &types.MsgCreateLiquidityMiningPeriod{
		Authority:      testAuthority,
		PoolId:         "nonexistent",
		AmountPerBlock: "1000",
		StartBlock:     100,
		EndBlock:       200,
	})
	if err != types.ErrPoolNotFound {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

// ---------- MsgServer: ClaimLiquidityRewards ----------

func TestMsg_ClaimLiquidityRewards_Success(t *testing.T) {
	srv, k, ctx, bk := setupMsgServer(t)
	seedPool(t, k, ctx, testPool, "1000")
	bk.setBalance(addrMiner1, types.LPDenom(testPool), 1000)

	// Create and process one block of rewards
	_, _ = srv.CreateLiquidityMiningPeriod(ctx, &types.MsgCreateLiquidityMiningPeriod{
		Authority: testAuthority, PoolId: testPool, AmountPerBlock: "1000", StartBlock: 100, EndBlock: 200,
	})
	k.ProcessMiningBeginBlock(ctx)

	resp, err := srv.ClaimLiquidityRewards(ctx, &types.MsgClaimLiquidityRewards{
		Sender: addrMiner1, PoolId: testPool,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedAmount != "1000" {
		t.Fatalf("expected 1000, got %s", resp.ClaimedAmount)
	}
}

func TestMsg_ClaimLiquidityRewards_PoolNotFound(t *testing.T) {
	srv, _, ctx, _ := setupMsgServer(t)

	_, err := srv.ClaimLiquidityRewards(ctx, &types.MsgClaimLiquidityRewards{
		Sender: addrMiner1, PoolId: "nonexistent",
	})
	if err != types.ErrPoolNotFound {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

// ---------- Two-miner proportional split ----------

func TestMining_TwoMiners_ProportionalSplit(t *testing.T) {
	k, ctx, bk := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	// Miner1: 750 LP, Miner2: 250 LP
	bk.setBalance(addrMiner1, types.LPDenom(testPool), 750)
	bk.setBalance(addrMiner2, types.LPDenom(testPool), 250)

	pool, _ := k.GetPool(ctx, testPool)
	period := &types.LiquidityMiningPeriod{
		Id: "p1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	k.AccumulateMiningRewards(ctx, pool, period)

	pool, _ = k.GetPool(ctx, testPool)

	// Settle both
	k.SettleMinerRewards(ctx, testPool, addrMiner1, big.NewInt(750), pool)
	k.SettleMinerRewards(ctx, testPool, addrMiner2, big.NewInt(250), pool)

	pos1 := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	pos2 := k.GetLpMinerPosition(ctx, testPool, addrMiner2)

	// 75% + 25% = 1000 (may lose 1 in integer division, assert sum)
	p1, _ := new(big.Int).SetString(pos1.PendingRewards, 10)
	p2, _ := new(big.Int).SetString(pos2.PendingRewards, 10)
	total := new(big.Int).Add(p1, p2)

	if total.Int64() < 999 || total.Int64() > 1000 {
		t.Fatalf("expected 999-1000 total pending, got %s", total.String())
	}
	if p1.Int64() != 750 {
		t.Fatalf("expected 750 for miner1, got %s", p1.String())
	}
	if p2.Int64() != 250 {
		t.Fatalf("expected 250 for miner2, got %s", p2.String())
	}
}

// ---------- Genesis round-trip ----------

func TestMining_GenesisRoundTrip(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	seedPool(t, k, ctx, testPool, "1000")

	_, err := k.CreateMiningPeriod(ctx, testPool, testAuthority, "500", 100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pos := &types.LpMinerPosition{
		PoolId: testPool, MinerAddr: addrMiner1, RewardDebt: "5000", PendingRewards: "100",
	}
	k.SetLpMinerPosition(ctx, pos)

	// Export
	gs := k.ExportGenesis(ctx)
	if len(gs.MiningPeriods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(gs.MiningPeriods))
	}
	if len(gs.MinerPositions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(gs.MinerPositions))
	}

	// Restore into fresh keeper
	k2, ctx2, _ := setupKeeper(t)
	// Need the pool too
	gs.Pools = []*types.Pool{{
		PoolId: testPool, LpTokenSupply: "1000", LpDenom: types.LPDenom(testPool),
	}}
	k2.InitGenesis(ctx2, gs)

	// Mining period restored
	got := k2.GetActiveMiningPeriodByPool(ctx2, testPool)
	if got == nil {
		t.Fatal("expected active period after genesis restore")
	}

	// Position restored
	p2 := k2.GetLpMinerPosition(ctx2, testPool, addrMiner1)
	if p2.RewardDebt != "5000" {
		t.Fatalf("expected debt 5000, got %s", p2.RewardDebt)
	}
}

// ---------- AddLiquidity settles mining rewards ----------

func TestAddLiquidity_SettlesMiningRewards(t *testing.T) {
	srv, k, ctx, bk := setupMsgServer(t)

	// Seed pool directly
	pool := &types.Pool{
		PoolId:        testPool,
		DenomA:        "uzrn",
		DenomB:        "uatom",
		ReserveA:      "1000000",
		ReserveB:      "1000000",
		LpTokenSupply: "1000",
		LpDenom:       types.LPDenom(testPool),
		Creator:       testAuthority,
	}
	k.SetPool(ctx, pool)

	// Give addrMiner1 500 LP and some underlying tokens to add
	bk.setBalance(addrMiner1, types.LPDenom(testPool), 500)
	bk.setBalance(addrMiner1, "uzrn", 100000)
	bk.setBalance(addrMiner1, "uatom", 100000)

	// Accumulate one block of rewards
	period := &types.LiquidityMiningPeriod{
		Id: "p1", PoolId: testPool, AmountPerBlock: "1000", Active: true, TotalMined: "0",
	}
	pool, _ = k.GetPool(ctx, testPool)
	k.AccumulateMiningRewards(ctx, pool, period)

	// AddLiquidity should settle 500/1000 × 1000 = 500 pending rewards
	_, err := srv.AddLiquidity(ctx, &types.MsgAddLiquidity{
		Sender:  addrMiner1,
		PoolId:  testPool,
		AmountA: "1000",
		AmountB: "1000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pos := k.GetLpMinerPosition(ctx, testPool, addrMiner1)
	pending, _ := new(big.Int).SetString(pos.PendingRewards, 10)
	if pending.Int64() < 490 || pending.Int64() > 510 {
		// Allow some rounding: should be ~500
		t.Fatalf("expected ~500 pending rewards after AddLiquidity settlement, got %s", pos.PendingRewards)
	}
}

// setupBlock returns a new context at the specified block height.
func setupBlock(ctx sdk.Context, height int64) sdk.Context {
	return ctx.WithBlockHeader(cmtproto.Header{Height: height, ChainID: testChainID})
}
