package cross_stack_test

// Test_OracleResilienceChain_AllFiveGaps exercises the five oracle resilience
// safeguards in one sequential test. Each section is labeled by gap number so
// failures pinpoint which safeguard regressed.
//
// Gaps and commits:
//
//	Gap 1 (29b2aa9) — External price feed + weighted median (billing/keeper)
//	Gap 2 (c81de93) — OracleCommittee N-of-M quorum (oracle_committee/keeper)
//	Gap 3 (012b15c) — GetTWAPSafe staleness guard (liquiditypool/keeper)
//	Gap 4 (0431112) — On-chain freshness token / nonce (oracle_committee/keeper)
//	Gap 5 (ce27158) — MaxSwapFractionBps enforcement (liquiditypool/keeper)
//
// oracle_committee is not yet wired into the app, so both module keepers are
// set up independently (same pattern as their respective keeper_test.go files).

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

	billingkeeper "github.com/zerone-chain/zerone/x/billing/keeper"
	ockeeper "github.com/zerone-chain/zerone/x/oracle_committee/keeper"
	octypes "github.com/zerone-chain/zerone/x/oracle_committee/types"
	lpkeeper "github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	lptypes "github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// ── Mock bank keeper: oracle_committee (sdk.Context interface) ────────────────

// ocMockBankKeeper satisfies the oracle_committee keeper.BankKeeper interface,
// which takes sdk.Context (not context.Context).
type ocMockBankKeeper struct {
	moduleBalance map[string]sdkmath.Int
}

func newOCMockBankKeeper() *ocMockBankKeeper {
	return &ocMockBankKeeper{moduleBalance: make(map[string]sdkmath.Int)}
}

func (m *ocMockBankKeeper) SendCoinsFromAccountToModule(_ sdk.Context, _ sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
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

func (m *ocMockBankKeeper) SendCoinsFromModuleToAccount(_ sdk.Context, senderModule string, _ sdk.AccAddress, amt sdk.Coins) error {
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

func (m *ocMockBankKeeper) SendCoinsFromModuleToModule(_ sdk.Context, senderModule, recipientModule string, amt sdk.Coins) error {
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

// ── Mock bank keeper: liquiditypool (context.Context interface) ───────────────

// lpMockBankKeeper satisfies the liquiditypool types.BankKeeper interface,
// which takes context.Context.
type lpMockBankKeeper struct {
	balances       map[string]map[string]int64
	moduleBalances map[string]map[string]int64
}

func newLPMockBankKeeper() *lpMockBankKeeper {
	return &lpMockBankKeeper{
		balances:       make(map[string]map[string]int64),
		moduleBalances: make(map[string]map[string]int64),
	}
}

func (m *lpMockBankKeeper) setBalance(addr, denom string, amount int64) {
	if m.balances[addr] == nil {
		m.balances[addr] = make(map[string]int64)
	}
	m.balances[addr][denom] = amount
}

func (m *lpMockBankKeeper) SendCoins(_ context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error {
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

func (m *lpMockBankKeeper) GetBalance(_ context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	if bal, ok := m.balances[addr.String()]; ok {
		if amt, exists := bal[denom]; exists {
			return sdk.NewCoin(denom, sdkmath.NewInt(amt))
		}
	}
	return sdk.NewCoin(denom, sdkmath.ZeroInt())
}

func (m *lpMockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
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

func (m *lpMockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
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

func (m *lpMockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
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

func (m *lpMockBankKeeper) MintCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	if m.moduleBalances[moduleName] == nil {
		m.moduleBalances[moduleName] = make(map[string]int64)
	}
	for _, coin := range amt {
		m.moduleBalances[moduleName][coin.Denom] += coin.Amount.Int64()
	}
	return nil
}

func (m *lpMockBankKeeper) BurnCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	if m.moduleBalances[moduleName] == nil {
		m.moduleBalances[moduleName] = make(map[string]int64)
	}
	for _, coin := range amt {
		m.moduleBalances[moduleName][coin.Denom] -= coin.Amount.Int64()
	}
	return nil
}

// ── Setup helpers ─────────────────────────────────────────────────────────────

const oracleResilienceAuthority = "zrn1oracleauthority000000000000"

// setupOracleCommitteeKeeper creates an isolated oracle_committee keeper at block 100.
func setupOracleCommitteeKeeper(t *testing.T) (ockeeper.Keeper, sdk.Context, *ocMockBankKeeper) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(octypes.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := ms.LoadLatestVersion(); err != nil {
		t.Fatalf("oracle_committee: failed to load store: %v", err)
	}
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	bk := newOCMockBankKeeper()
	k := ockeeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), bk, oracleResilienceAuthority)
	ctx := sdk.NewContext(ms, cmtproto.Header{Height: 100, ChainID: testChainID}, false, log.NewNopLogger())
	return k, ctx, bk
}

// setupLiquidityPoolKeeper creates an isolated liquiditypool keeper at block 100.
func setupLiquidityPoolKeeper(t *testing.T) (lpkeeper.Keeper, lptypes.MsgServer, sdk.Context, *lpMockBankKeeper) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(lptypes.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := ms.LoadLatestVersion(); err != nil {
		t.Fatalf("liquiditypool: failed to load store: %v", err)
	}
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	bk := newLPMockBankKeeper()
	k := lpkeeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), bk, oracleResilienceAuthority)
	ctx := sdk.NewContext(ms, cmtproto.Header{Height: 100, ChainID: testChainID}, false, log.NewNopLogger())
	return k, lpkeeper.NewMsgServerImpl(k), ctx, bk
}

// ocAddr returns a deterministic bech32 operator address for the given index.
func ocAddr(idx int) string {
	b := make([]byte, 20)
	b[19] = byte(idx)
	return sdk.AccAddress(b).String()
}

// ── The integration test ──────────────────────────────────────────────────────

// Test_OracleResilienceChain_AllFiveGaps exercises all five oracle resilience
// safeguards in sequence. A failure in any section names the regressed gap.
func Test_OracleResilienceChain_AllFiveGaps(t *testing.T) {

	// ══════════════════════════════════════════════════════════════════════
	// GAP 1 — External price feed: weighted median of 3 independent sources.
	//
	// Attack model: a single corrupted source cannot shift the median.
	// With sources [90, 100, 110]: median = 100.
	// With one attacker driving source 1 to 200: sources [200, 100, 110]
	// → sorted [100, 110, 200] → median = 110 (bounded, not 200).
	// ══════════════════════════════════════════════════════════════════════

	t.Run("Gap1_WeightedMedian_BoundsAttacker", func(t *testing.T) {
		sources := []*big.Int{
			big.NewInt(90_000_000),
			big.NewInt(100_000_000),
			big.NewInt(110_000_000),
		}
		median := billingkeeper.WeightedMedianPrice(sources)
		if median == nil || median.Cmp(big.NewInt(100_000_000)) != 0 {
			t.Errorf("gap1: want median=100_000_000, got %v", median)
		}

		// Attacker corrupts source 0: drives it to 200_000_000.
		corrupted := []*big.Int{
			big.NewInt(200_000_000), // attacker
			big.NewInt(100_000_000),
			big.NewInt(110_000_000),
		}
		medianCorrupted := billingkeeper.WeightedMedianPrice(corrupted)
		// Sorted: [100, 110, 200] → median = 110 (index 1 of 3, lower-middle).
		if medianCorrupted == nil || medianCorrupted.Cmp(big.NewInt(110_000_000)) != 0 {
			t.Errorf("gap1: with one corrupted source, want bounded median=110_000_000, got %v", medianCorrupted)
		}

		// Single-source: returns the source directly.
		single := billingkeeper.WeightedMedianPrice([]*big.Int{big.NewInt(50_000_000)})
		if single == nil || single.Cmp(big.NewInt(50_000_000)) != 0 {
			t.Errorf("gap1: single source, want 50_000_000, got %v", single)
		}

		// Empty: returns nil (caller falls back to fixed price).
		if billingkeeper.WeightedMedianPrice(nil) != nil {
			t.Errorf("gap1: empty sources should return nil")
		}
	})

	// ══════════════════════════════════════════════════════════════════════
	// GAP 2 — OracleCommittee quorum: 3 operators, 60% quorum = 2 needed.
	//
	// Sequence:
	//   1. Register 3 operators (meets MinCommitteeSize=3).
	//   2. Submit eval from op1 → quorum not reached (1 < 2).
	//   3. Submit eval from op2 → quorum reached (2 >= 2) → aggregation fires.
	//   4. Submit eval from op3 → ErrAggregationExists (already finalized).
	// ══════════════════════════════════════════════════════════════════════

	t.Run("Gap2_OracleCommittee_QuorumFiresAtTwo", func(t *testing.T) {
		k, ctx, _ := setupOracleCommitteeKeeper(t)

		op1 := ocAddr(1)
		op2 := ocAddr(2)
		op3 := ocAddr(3)

		for _, addr := range []string{op1, op2, op3} {
			if err := k.RegisterOperator(ctx, addr, "https://oracle.example.com", "5000000"); err != nil {
				t.Fatalf("gap2: register operator %s: %v", addr, err)
			}
		}

		claimID := "gap2-claim-quorum"

		// Op1 submits → quorum not yet reached (1/2 needed).
		agg1, fired1, err1 := k.SubmitEvaluation(ctx, op1, claimID, "true", 8000, "gap2-reasoning", "")
		if err1 != nil {
			t.Fatalf("gap2: op1 submit: %v", err1)
		}
		if fired1 || agg1 != nil {
			t.Errorf("gap2: quorum should NOT fire after first evaluation; fired=%v", fired1)
		}

		// Op2 submits → quorum reached; aggregation fires.
		agg2, fired2, err2 := k.SubmitEvaluation(ctx, op2, claimID, "true", 9000, "gap2-reasoning", "")
		if err2 != nil {
			t.Fatalf("gap2: op2 submit: %v", err2)
		}
		if !fired2 || agg2 == nil {
			t.Errorf("gap2: quorum SHOULD fire after second evaluation; fired=%v", fired2)
		}
		if agg2.FinalVerdict != "true" {
			t.Errorf("gap2: want final verdict 'true', got %q", agg2.FinalVerdict)
		}
		if agg2.OperatorCount != 2 {
			t.Errorf("gap2: want 2 operators in aggregation, got %d", agg2.OperatorCount)
		}

		// Op3 submits → aggregation already exists; error.
		_, _, err3 := k.SubmitEvaluation(ctx, op3, claimID, "true", 7000, "gap2-reasoning", "")
		if err3 != octypes.ErrAggregationExists {
			t.Errorf("gap2: op3 submit after quorum: want ErrAggregationExists, got %v", err3)
		}
	})

	// ══════════════════════════════════════════════════════════════════════
	// GAP 4 — Freshness token: oracle request nonce must match exactly.
	//
	// Sequence:
	//   1. Open oracle request → receive nonce.
	//   2. Submit with wrong nonce → ErrInvalidOracleRequestId.
	//   3. Submit with correct nonce → succeeds.
	//
	// Note: Gaps 2 and 4 interact naturally (nonce flows through evaluation
	// submission), so Gap 4 is tested in its own claim context to keep
	// assertions clean.
	// ══════════════════════════════════════════════════════════════════════

	t.Run("Gap4_FreshnessToken_NonceEnforcement", func(t *testing.T) {
		k, ctx, _ := setupOracleCommitteeKeeper(t)

		// Register enough operators to meet MinCommitteeSize=3.
		for i := 1; i <= 3; i++ {
			if err := k.RegisterOperator(ctx, ocAddr(i), "https://oracle.example.com", "5000000"); err != nil {
				t.Fatalf("gap4: register op%d: %v", i, err)
			}
		}

		requestor := ocAddr(99)
		claimID := "gap4-claim-nonce"

		nonce, err := k.OpenOracleRequest(ctx, claimID, requestor)
		if err != nil {
			t.Fatalf("gap4: OpenOracleRequest: %v", err)
		}
		if nonce == "" {
			t.Fatal("gap4: OpenOracleRequest returned empty nonce")
		}

		// Wrong nonce → rejected.
		_, _, errWrong := k.SubmitEvaluation(ctx, ocAddr(1), claimID, "true", 8000, "", "deadbeef-wrong-nonce")
		if errWrong != octypes.ErrInvalidOracleRequestId {
			t.Errorf("gap4: wrong nonce: want ErrInvalidOracleRequestId, got %v", errWrong)
		}

		// Missing nonce (empty string) → rejected.
		_, _, errEmpty := k.SubmitEvaluation(ctx, ocAddr(1), claimID, "true", 8000, "", "")
		if errEmpty != octypes.ErrInvalidOracleRequestId {
			t.Errorf("gap4: empty nonce: want ErrInvalidOracleRequestId, got %v", errEmpty)
		}

		// Correct nonce → accepted.
		_, _, errOK := k.SubmitEvaluation(ctx, ocAddr(1), claimID, "true", 8000, "gap4-reasoning", nonce)
		if errOK != nil {
			t.Errorf("gap4: correct nonce: want no error, got %v", errOK)
		}

		// Verify nonce is persisted on the evaluation record.
		rec, found := k.GetEvaluation(ctx, claimID, ocAddr(1))
		if !found {
			t.Fatal("gap4: evaluation record not found after submission")
		}
		if rec.OracleRequestId != nonce {
			t.Errorf("gap4: evaluation record nonce mismatch: want %q, got %q", nonce, rec.OracleRequestId)
		}

		// Backward compat: no oracle request opened → empty nonce passes.
		claimOld := "gap4-claim-old-compat"
		_, _, errOld := k.SubmitEvaluation(ctx, ocAddr(1), claimOld, "true", 8000, "", "")
		if errOld != nil {
			t.Errorf("gap4: backward compat (no oracle request): want no error, got %v", errOld)
		}
	})

	// ══════════════════════════════════════════════════════════════════════
	// GAP 3 — GetTWAPSafe staleness guard.
	//
	// Sequence:
	//   1. Create pool, update TWAP accumulator at block 100.
	//   2. At block 100: maxStaleness=50 → fresh (delta=0 ≤ 50). Succeeds.
	//   3. At block 200: maxStaleness=50 → stale (delta=100 > 50). Error.
	//   4. maxStaleness=0: staleness check disabled regardless of age.
	// ══════════════════════════════════════════════════════════════════════

	t.Run("Gap3_GetTWAPSafe_StalenessGuard", func(t *testing.T) {
		k, msgSrv, ctx, bk := setupLiquidityPoolKeeper(t)

		// Fund authority for pool creation.
		bk.setBalance(oracleResilienceAuthority, "uzrn", 1_000_000_000_000_000)
		bk.setBalance(oracleResilienceAuthority, "uatom", 1_000_000_000_000_000)

		resp, err := msgSrv.CreatePool(ctx, &lptypes.MsgCreatePool{
			Creator: oracleResilienceAuthority,
			DenomA:  "uzrn",
			DenomB:  "uatom",
			AmountA: "100000000000",
			AmountB: "200000000000",
		})
		if err != nil {
			t.Fatalf("gap3: CreatePool: %v", err)
		}
		poolID := resp.PoolId

		pool, found := k.GetPool(ctx, poolID)
		if !found {
			t.Fatalf("gap3: pool %q not found after creation", poolID)
		}

		// Seed TWAP accumulator at block 100.
		k.UpdateTWAPAccumulator(ctx, pool)

		// Block 100, maxStaleness=50: fresh (delta=0). Should succeed.
		price, _, err := k.GetTWAPSafe(ctx, poolID, "uzrn", 0, 50)
		if err != nil {
			t.Errorf("gap3: fresh TWAP (block 100, maxStaleness=50): want no error, got %v", err)
		}
		if price == nil || price.Sign() <= 0 {
			t.Errorf("gap3: fresh TWAP: want positive price, got %v", price)
		}

		// Block 200, maxStaleness=50: stale (delta=100 > 50). Should fail.
		laterCtx := ctx.WithBlockHeight(200)
		_, _, errStale := k.GetTWAPSafe(laterCtx, poolID, "uzrn", 0, 50)
		if errStale == nil {
			t.Errorf("gap3: stale TWAP (block 200, maxStaleness=50): want ErrTWAPStale, got nil")
		}

		// maxStaleness=0: staleness check disabled — same stale context should succeed.
		priceUnbounded, _, errUnbounded := k.GetTWAPSafe(laterCtx, poolID, "uzrn", 0, 0)
		if errUnbounded != nil {
			t.Errorf("gap3: maxStaleness=0 (disabled): want no error, got %v", errUnbounded)
		}
		if priceUnbounded == nil || priceUnbounded.Sign() <= 0 {
			t.Errorf("gap3: maxStaleness=0: want positive price, got %v", priceUnbounded)
		}
	})

	// ══════════════════════════════════════════════════════════════════════
	// GAP 5 — MaxSwapFractionBps: large swaps blocked; small swaps pass.
	//
	// Pool: 100_000_000_000 uzrn : 100_000_000_000 uatom.
	// Cap: 100_000 bps = 10% → max input = 10_000_000_000 uzrn.
	//
	// Sequence:
	//   1. Swap 15_000_000_000 (15%) → ErrSwapFractionExceeded.
	//   2. Swap  8_000_000_000  (8%) → succeeds.
	//   3. MaxSwapFractionBps=0: check disabled → 50% swap passes.
	// ══════════════════════════════════════════════════════════════════════

	t.Run("Gap5_MaxSwapFractionBps_BlocksLargeSwap", func(t *testing.T) {
		k, msgSrv, ctx, bk := setupLiquidityPoolKeeper(t)

		bk.setBalance(oracleResilienceAuthority, "uzrn", 1_000_000_000_000_000)
		bk.setBalance(oracleResilienceAuthority, "uatom", 1_000_000_000_000_000)

		resp, err := msgSrv.CreatePool(ctx, &lptypes.MsgCreatePool{
			Creator: oracleResilienceAuthority,
			DenomA:  "uzrn",
			DenomB:  "uatom",
			AmountA: "100000000000",
			AmountB: "100000000000",
		})
		if err != nil {
			t.Fatalf("gap5: CreatePool: %v", err)
		}
		poolID := resp.PoolId

		// Set 10% cap.
		params := lptypes.DefaultParams()
		params.MaxSwapFractionBps = 100_000
		k.SetParams(ctx, params)

		sender := "zrn1swapsender0000000000000000000"
		bk.setBalance(sender, "uzrn", 100_000_000_000)

		// 15% swap → blocked.
		_, errBlocked := msgSrv.Swap(ctx, &lptypes.MsgSwap{
			Sender:        sender,
			PoolId:        poolID,
			TokenInDenom:  "uzrn",
			TokenInAmount: "15000000000",
		})
		if errBlocked == nil {
			t.Errorf("gap5: 15%% swap: want ErrSwapFractionExceeded, got nil")
		}

		// 8% swap → allowed.
		_, errAllowed := msgSrv.Swap(ctx, &lptypes.MsgSwap{
			Sender:        sender,
			PoolId:        poolID,
			TokenInDenom:  "uzrn",
			TokenInAmount: "8000000000",
		})
		if errAllowed != nil {
			t.Errorf("gap5: 8%% swap: want success, got %v", errAllowed)
		}

		// MaxSwapFractionBps=0 → check disabled; large swap passes.
		params.MaxSwapFractionBps = 0
		params.MinReserve = "1"
		k.SetParams(ctx, params)

		bk.setBalance(sender, "uzrn", 100_000_000_000) // replenish

		// Create a fresh pool with a different denom pair so reserves are clean.
		bk.setBalance(oracleResilienceAuthority, "uusdc", 1_000_000_000_000_000)
		resp2, err2 := msgSrv.CreatePool(ctx, &lptypes.MsgCreatePool{
			Creator: oracleResilienceAuthority,
			DenomA:  "uzrn",
			DenomB:  "uusdc",
			AmountA: "100000000000",
			AmountB: "100000000000",
		})
		if err2 != nil {
			t.Fatalf("gap5: CreatePool (disabled-check): %v", err2)
		}
		_, errDisabled := msgSrv.Swap(ctx, &lptypes.MsgSwap{
			Sender:        sender,
			PoolId:        resp2.PoolId,
			TokenInDenom:  "uzrn",
			TokenInAmount: "50000000000", // 50% — allowed when disabled
		})
		if errDisabled != nil {
			t.Errorf("gap5: 50%% swap with MaxSwapFractionBps=0 (disabled): want success, got %v", errDisabled)
		}
	})
}
