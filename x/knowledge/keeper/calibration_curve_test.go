package keeper_test

import (
	"testing"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// TestCalibrationCurve_Initialization verifies that the 10 buckets are created on first call.
func TestCalibrationCurve_Initialization(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verifier := "cosmos1verifier0000000000000000000"
	k.RecordVerifierOutcome(ctx, verifier, "test_method", 800_000, true, true)

	c, found := k.GetAgentCalibration(ctx, verifier)
	if !found {
		t.Fatal("AgentCalibration not found")
	}
	if len(c.CalibrationCurve) != 10 {
		t.Fatalf("expected 10 calibration buckets, got %d", len(c.CalibrationCurve))
	}
	// Verify bucket boundaries
	for i, b := range c.CalibrationCurve {
		expected := uint64(i) * 100_000
		if b.MinConfidenceBps != expected {
			t.Errorf("bucket %d: expected min=%d, got %d", i, expected, b.MinConfidenceBps)
		}
	}
}

// TestCalibrationCurve_CorrectBucket verifies predictions land in the right bucket.
func TestCalibrationCurve_CorrectBucket(t *testing.T) {
	tests := []struct {
		pAcceptBps    uint64 // P(ACCEPT)
		expectedBucket int
	}{
		{0, 0},              // 0% → bucket 0
		{50_000, 0},         // 5% → bucket 0
		{100_000, 1},        // 10% → bucket 1
		{350_000, 3},        // 35% → bucket 3
		{500_000, 5},        // 50% → bucket 5
		{750_000, 7},        // 75% → bucket 7
		{999_999, 9},        // 99.9999% → bucket 9
		{1_000_000, 9},      // 100% → bucket 9 (clamped)
	}

	for _, tt := range tests {
		k, ctx := setupKnowledgeTest(t)
		verifier := "cosmos1verifier0000000000000000000"

		// We need to drive pAcceptBps = tt.pAcceptBps. Since votedAccept=true means pAcceptBps=confidenceBps,
		// set confidenceBps directly when votedAccept=true.
		var confidenceBps uint64
		var votedAccept bool
		if tt.pAcceptBps >= 500_000 {
			confidenceBps = tt.pAcceptBps
			votedAccept = true
		} else {
			confidenceBps = 1_000_000 - tt.pAcceptBps
			votedAccept = false
		}

		k.RecordVerifierOutcome(ctx, verifier, "m", confidenceBps, votedAccept, true)

		c, found := k.GetAgentCalibration(ctx, verifier)
		if !found {
			t.Fatalf("pAccept=%d: AgentCalibration not found", tt.pAcceptBps)
		}
		if len(c.CalibrationCurve) != 10 {
			t.Fatalf("pAccept=%d: expected 10 buckets", tt.pAcceptBps)
		}
		if c.CalibrationCurve[tt.expectedBucket].TotalCount != 1 {
			t.Errorf("pAccept=%d: expected 1 in bucket %d, got %d",
				tt.pAcceptBps, tt.expectedBucket, c.CalibrationCurve[tt.expectedBucket].TotalCount)
		}
		// All other buckets should be zero
		for i, b := range c.CalibrationCurve {
			if i == tt.expectedBucket {
				continue
			}
			if b.TotalCount != 0 {
				t.Errorf("pAccept=%d: bucket %d should be 0, got %d", tt.pAcceptBps, i, b.TotalCount)
			}
		}
	}
}

// TestCalibrationCurve_HitRate verifies correct_count vs total_count tracking.
func TestCalibrationCurve_HitRate(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	verifier := "cosmos1verifier0000000000000000000"

	// 3 predictions in the 80% bucket (P(ACCEPT) = 80% → confidenceBps=800_000, votedAccept=true)
	// Outcomes: ACCEPT, ACCEPT, REJECT
	k.RecordVerifierOutcome(ctx, verifier, "m", 800_000, true, true)  // hit
	k.RecordVerifierOutcome(ctx, verifier, "m", 800_000, true, true)  // hit
	k.RecordVerifierOutcome(ctx, verifier, "m", 800_000, true, false) // miss

	c, _ := k.GetAgentCalibration(ctx, verifier)
	bucket8 := c.CalibrationCurve[8] // [800_000, 900_000)
	if bucket8.TotalCount != 3 {
		t.Errorf("expected TotalCount=3, got %d", bucket8.TotalCount)
	}
	if bucket8.CorrectCount != 2 {
		t.Errorf("expected CorrectCount=2 (ACCEPT outcomes), got %d", bucket8.CorrectCount)
	}
}

// TestCalibrationCurve_ZeroConfidenceSkipped verifies confidence=0 skips the curve.
func TestCalibrationCurve_ZeroConfidenceSkipped(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	verifier := "cosmos1verifier0000000000000000000"

	// confidenceBps=0 is skipped by the early-return guard in RecordVerifierOutcome
	k.RecordVerifierOutcome(ctx, verifier, "m", 0, true, true)

	_, found := k.GetAgentCalibration(ctx, verifier)
	if found {
		t.Error("expected no calibration record for zero-confidence verifier")
	}
}

// TestCalibrationCurve_MultipleBuckets verifies predictions spread across multiple buckets.
func TestCalibrationCurve_MultipleBuckets(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	verifier := "cosmos1verifier0000000000000000000"

	// bucket 2: P(ACCEPT)=20% → confidenceBps=800_000, votedAccept=false
	//           pAcceptBps = 1_000_000 - 800_000 = 200_000 → bucket 2
	k.RecordVerifierOutcome(ctx, verifier, "m", 800_000, false, false) // miss (REJECT predicted, REJECT happened: pAcceptBps=200_000, outcome=false → miss for ACCEPT bucket)
	// bucket 9: P(ACCEPT)=100%
	k.RecordVerifierOutcome(ctx, verifier, "m", 1_000_000, true, true) // hit

	c, _ := k.GetAgentCalibration(ctx, verifier)
	if c.CalibrationCurve[2].TotalCount != 1 {
		t.Errorf("bucket 2: expected TotalCount=1, got %d", c.CalibrationCurve[2].TotalCount)
	}
	if c.CalibrationCurve[9].TotalCount != 1 {
		t.Errorf("bucket 9: expected TotalCount=1, got %d", c.CalibrationCurve[9].TotalCount)
	}
}

// TestRecordCalibrationBucket_StandaloneUnit tests the pure helper directly.
func TestRecordCalibrationBucket_StandaloneUnit(t *testing.T) {
	c := &types.AgentCalibration{}

	// First call should initialize 10 buckets
	keeper.RecordCalibrationBucketForTest(c, 500_000, true)
	if len(c.CalibrationCurve) != 10 {
		t.Fatalf("expected 10 buckets after first record")
	}
	// bucket 5: [500_000, 600_000), outcome ACCEPT → CorrectCount=1
	if c.CalibrationCurve[5].CorrectCount != 1 || c.CalibrationCurve[5].TotalCount != 1 {
		t.Errorf("bucket 5: expected correct=1/total=1, got %d/%d",
			c.CalibrationCurve[5].CorrectCount, c.CalibrationCurve[5].TotalCount)
	}

	// Second call: same bucket, outcome REJECT
	keeper.RecordCalibrationBucketForTest(c, 550_000, false)
	if c.CalibrationCurve[5].CorrectCount != 1 || c.CalibrationCurve[5].TotalCount != 2 {
		t.Errorf("bucket 5 after REJECT: expected correct=1/total=2, got %d/%d",
			c.CalibrationCurve[5].CorrectCount, c.CalibrationCurve[5].TotalCount)
	}
}
