package keeper_test

// Oracle Resilience Gap 5: TWAP Deviation Circuit Breaker
//
// MaxTwapDeviationBps in Params caps the allowed divergence between
// the current spot price and the TWAP. Swaps that arrive when spot has
// drifted too far from the TWAP — a signal of manipulation or an extreme
// price shock — are rejected with ErrTWAPDeviationExceeded.
//
// Design:
//   params.max_twap_deviation_bps == 0  → check disabled (default)
//   TWAP unavailable (no history yet)   → fail-open, swap allowed
//   |spot − twap| * 1_000_000 / twap > max_twap_deviation_bps → rejected
//
// The check fires in the Swap path after the max-swap-fraction check and
// before the reentrancy pool.Locked guard, so it cannot be bypassed by
// concurrent calls.
//
// TWAP accumulator construction (for tests):
//   twap = CumPriceAToB * 1e6 / (acc.LastBlock * 1e12)
//   → CumPriceAToB = twap * acc.LastBlock * 1e6   (for acc.LastBlock blocks)
//
// Context: setupKeeper uses block height 100. Inject accumulator with
// LastBlock=50 so actualWindow = 100−50 = 50 > 0 (avoids spot-price fallback).

import (
	"testing"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

// TestTWAPDeviation_Disabled verifies that when MaxTwapDeviationBps == 0
// the circuit breaker is entirely disabled, even with a large spot/TWAP gap.
func TestTWAPDeviation_Disabled(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 0 // disabled
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// Inject a TWAP 2× above spot — would be 50% deviation if check were active.
	// CumPriceAToB for TWAP=2_000_000 at LastBlock=50:
	//   = 2_000_000 * 50 * 1_000_000 = 100_000_000_000_000
	k.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    50,
		CumPriceAToB: "100000000000000", // TWAP = 2_000_000 (2:1)
		CumPriceBToA: "100000000000000",
	})

	sender := "zrn1dev0000000000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err != nil {
		t.Fatalf("expected swap to succeed when deviation check disabled: %v", err)
	}
}

// TestTWAPDeviation_NoHistory verifies fail-open behaviour: when the pool
// has no accumulated TWAP history (CumPrice == 0), GetTWAP falls back to
// spot price, deviation is zero, and the swap is allowed.
func TestTWAPDeviation_NoHistory(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 300_000 // 30% threshold
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")
	// After createTestPool, accumulator has LastBlock=100, CumPrice="0".
	// GetTWAP: CumPrice==0 → falls back to spot price → deviation=0 → allowed.

	sender := "zrn1nohist000000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err != nil {
		t.Fatalf("expected swap to succeed when no TWAP history (fail-open): %v", err)
	}
}

// TestTWAPDeviation_WithinThreshold verifies that a swap is allowed when
// the spot/TWAP deviation is below the configured threshold.
//
// Pool reserves:    100B uzrn : 100B uatom  → spot = 1_000_000 (1:1 × 1e6)
// Injected TWAP:    900_000  (10% below spot — pool drifted slightly up)
// Actual deviation: |1_000_000 − 900_000| × 1_000_000 / 900_000 ≈ 111_111 (11.1%)
// Threshold:        200_000 (20%)  →  111_111 < 200_000  →  ALLOWED
func TestTWAPDeviation_WithinThreshold(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 200_000 // 20%
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// TWAP = 900_000 → CumPriceAToB = 900_000 * 50 * 1_000_000 = 45_000_000_000_000
	k.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    50,
		CumPriceAToB: "45000000000000", // TWAP ≈ 900_000
		CumPriceBToA: "45000000000000",
	})

	sender := "zrn1within000000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err != nil {
		t.Fatalf("expected swap to succeed when deviation (≈11%%) within threshold (20%%): %v", err)
	}
}

// TestTWAPDeviation_ExceedsThreshold verifies that a swap is blocked when
// the spot/TWAP deviation exceeds the configured threshold.
//
// Pool reserves:    100B uzrn : 100B uatom  → spot = 1_000_000 (1:1 × 1e6)
// Injected TWAP:    2_000_000  (TWAP recorded 2:1 price history)
// Actual deviation: |1_000_000 − 2_000_000| × 1_000_000 / 2_000_000 = 500_000 (50%)
// Threshold:        300_000 (30%)  →  500_000 > 300_000  →  BLOCKED
func TestTWAPDeviation_ExceedsThreshold(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 300_000 // 30%
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	// TWAP = 2_000_000 → CumPriceAToB = 2_000_000 * 50 * 1_000_000 = 100_000_000_000_000
	k.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    50,
		CumPriceAToB: "100000000000000", // TWAP = 2_000_000
		CumPriceBToA: "100000000000000",
	})

	sender := "zrn1exceeds00000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err == nil {
		t.Fatal("expected ErrTWAPDeviationExceeded: 50% deviation exceeds 30% threshold")
	}
}

// TestTWAPDeviation_AtBoundary_Allowed verifies the strict-greater comparison:
// deviation exactly equal to the threshold is ALLOWED (not blocked).
//
// spot = 1_000_000, TWAP = 2_000_000, deviation = 500_000 (50%)
// threshold = 500_000 (50%)  →  500_000 > 500_000 is FALSE  →  ALLOWED
func TestTWAPDeviation_AtBoundary_Allowed(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 500_000 // exactly 50%
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	k.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    50,
		CumPriceAToB: "100000000000000", // TWAP = 2_000_000 → deviation = 500_000 = threshold
		CumPriceBToA: "100000000000000",
	})

	sender := "zrn1boundary0000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err != nil {
		t.Fatalf("expected swap allowed when deviation equals threshold (strict >): %v", err)
	}
}

// TestTWAPDeviation_JustOverBoundary_Blocked verifies that one BPS above the
// threshold triggers the circuit breaker.
//
// spot = 1_000_000, TWAP = 2_000_000, deviation = 500_000
// threshold = 499_999  →  500_000 > 499_999 is TRUE  →  BLOCKED
func TestTWAPDeviation_JustOverBoundary_Blocked(t *testing.T) {
	ms, k, ctx, bk := setupMsgServer(t)

	params := types.DefaultParams()
	params.MaxTwapDeviationBps = 499_999 // 1 BPS below the actual 50% deviation
	k.SetParams(ctx, params)

	poolId := createTestPool(t, ms, ctx, bk, "uzrn", "uatom", "100000000000", "100000000000")

	k.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    50,
		CumPriceAToB: "100000000000000", // TWAP = 2_000_000 → deviation = 500_000 > 499_999
		CumPriceBToA: "100000000000000",
	})

	sender := "zrn1justover0000000000000000000"
	bk.setBalance(sender, "uzrn", 1_000_000_000)

	_, err := ms.Swap(ctx, &types.MsgSwap{
		Sender:        sender,
		PoolId:        poolId,
		TokenInDenom:  "uzrn",
		TokenInAmount: "1000000",
	})
	if err == nil {
		t.Fatal("expected ErrTWAPDeviationExceeded: deviation (500_000) just exceeds threshold (499_999)")
	}
}
