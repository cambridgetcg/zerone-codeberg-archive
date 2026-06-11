package keeper_test

// domain_decay_test.go — Scientific Method Gap 5: domain-specific decay rates.
//
// Tests that DomainDecayOverride entries in Params cause RunAccuracyDecay to
// apply per-domain thresholds instead of the global ones, and that the
// Params.Validate() guards reject malformed overrides.

import (
	"testing"

	"github.com/zerone-chain/zerone/x/qualification/types"
)

// makeActiveQ creates a minimal ACTIVE qualification with the given accuracy.
func makeActiveQ(validator, domain string, accuracyBps uint64) *types.DomainQualification {
	return &types.DomainQualification{
		Validator: validator,
		Domain:    domain,
		Status:    types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE,
		Weight:    50,
		Metrics: &types.QualificationMetrics{
			TotalVerifications:  100,
			CorrectVerifications: uint64(accuracyBps / 10_000),
			AccuracyBps:         accuracyBps,
		},
	}
}

func makeProbationary(validator, domain string, accuracyBps uint64) *types.DomainQualification {
	q := makeActiveQ(validator, domain, accuracyBps)
	q.Status = types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY
	return q
}

// paramsWithOverride returns a Params struct with only the decay fields set,
// plus one domain override entry.
func paramsWithOverride(
	globalProb, globalSusp, globalRec, globalMin uint64,
	overrideDomain string, ovProb, ovSusp, ovRec, ovMin uint64,
) *types.Params {
	return &types.Params{
		MinStakeAmount:  "100000000",
		DecayMinSamples: globalMin,
		DecayProbationBps: globalProb,
		DecaySuspensionBps: globalSusp,
		DecayRecoveryBps: globalRec,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{
				Domain:             overrideDomain,
				DecayProbationBps:  ovProb,
				DecaySuspensionBps: ovSusp,
				DecayRecoveryBps:   ovRec,
				DecayMinSamples:    ovMin,
			},
		},
	}
}

// TestDomainDecay_GlobalFallback — no override for domain; global thresholds apply.
func TestDomainDecay_GlobalFallback(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// Global: probation at 600k, no override for "physics"
	params := &types.Params{
		MinStakeAmount:     "100000000",
		DecayMinSamples:    10,
		DecayProbationBps:  600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:   750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "economics", DecayProbationBps: 800_000},
		},
	}
	k.SetParams(ctx, params)

	// Accuracy 550_000 is below global probation (600_000) → should become PROBATIONARY
	q := makeActiveQ(testAddr("val1"), "physics", 550_000)
	k.SetQualification(ctx, q)

	k.RunAccuracyDecay(ctx, 1000, params)

	got, found := k.GetQualification(ctx, q.Validator, q.Domain)
	if !found {
		t.Fatal("qualification not found after decay")
	}
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY {
		t.Errorf("expected PROBATIONARY, got %v", got.Status)
	}
}

// TestDomainDecay_OverrideTighter — override has tighter threshold; validator
// that would survive global threshold is demoted by domain-specific one.
func TestDomainDecay_OverrideTighter(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// Global probation at 600_000, but "technology" override at 800_000.
	// Accuracy 650_000 > global (survives global) but < override (demoted by override).
	params := paramsWithOverride(
		600_000, 400_000, 750_000, 10, // global
		"technology", 800_000, 500_000, 900_000, 0, // override
	)
	k.SetParams(ctx, params)

	q := makeActiveQ(testAddr("val2"), "technology", 650_000)
	k.SetQualification(ctx, q)

	k.RunAccuracyDecay(ctx, 1000, params)

	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY {
		t.Errorf("technology validator at 65%% should be PROBATIONARY under tighter override (80%% threshold), got %v", got.Status)
	}
}

// TestDomainDecay_OverrideLooser — override has looser threshold; validator
// that would be demoted globally survives under domain-specific override.
func TestDomainDecay_OverrideLooser(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// Global probation at 600_000, but "mathematics" override at 400_000 (looser).
	// Accuracy 550_000 < global (would normally be demoted) but > override (survives).
	params := paramsWithOverride(
		600_000, 400_000, 750_000, 10, // global
		"mathematics", 400_000, 200_000, 500_000, 0, // override
	)
	k.SetParams(ctx, params)

	q := makeActiveQ(testAddr("val3"), "mathematics", 550_000)
	k.SetQualification(ctx, q)

	k.RunAccuracyDecay(ctx, 1000, params)

	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE {
		t.Errorf("mathematics validator at 55%% should remain ACTIVE under looser override (40%% threshold), got %v", got.Status)
	}
}

// TestDomainDecay_PartialOverride_OnlyProbation — override sets only
// DecayProbationBps; suspension and recovery fall back to global.
func TestDomainDecay_PartialOverride_OnlyProbation(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// "chemistry" override: probation at 700_000, suspension/recovery = global.
	params := paramsWithOverride(
		600_000, 400_000, 750_000, 10, // global
		"chemistry", 700_000, 0, 0, 0, // override: only probation raised
	)
	k.SetParams(ctx, params)

	// Accuracy 650_000: above global (600k) but below override (700k) → PROBATIONARY
	q := makeActiveQ(testAddr("val4"), "chemistry", 650_000)
	k.SetQualification(ctx, q)
	k.RunAccuracyDecay(ctx, 1000, params)
	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY {
		t.Errorf("expected PROBATIONARY (override 700k threshold), got %v", got.Status)
	}

	// Now test suspension: put q2 in PROBATIONARY with accuracy 350_000.
	// Suspension falls back to global 400_000 — 350k < 400k → SUSPENDED.
	q2 := makeProbationary(testAddr("val5"), "chemistry", 350_000)
	k.SetQualification(ctx, q2)
	k.RunAccuracyDecay(ctx, 1000, params)
	got2, _ := k.GetQualification(ctx, q2.Validator, q2.Domain)
	if got2.Status != types.QualificationStatus_QUALIFICATION_STATUS_SUSPENDED {
		t.Errorf("expected SUSPENDED (global suspension 400k threshold), got %v", got2.Status)
	}
}

// TestDomainDecay_MinSamplesOverride — domain-specific min_samples; validator
// below override min is skipped even if above global min.
func TestDomainDecay_MinSamplesOverride(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// "formal-logic" override: min_samples = 50, global min = 10.
	params := paramsWithOverride(
		600_000, 400_000, 750_000, 10, // global min_samples = 10
		"formal-logic", 600_000, 400_000, 750_000, 50, // override min_samples = 50
	)
	k.SetParams(ctx, params)

	// accuracy 100_000 is well below probation threshold — would be demoted
	// globally. But total_verifications = 20 < override min_samples 50 → skipped.
	q := &types.DomainQualification{
		Validator: testAddr("val6"),
		Domain:    "formal-logic",
		Status:    types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE,
		Weight:    50,
		Metrics: &types.QualificationMetrics{
			TotalVerifications:  20, // below override min (50) but above global (10)
			CorrectVerifications: 2,
			AccuracyBps:         100_000, // 10% — would normally trigger probation
		},
	}
	k.SetQualification(ctx, q)
	k.RunAccuracyDecay(ctx, 1000, params)

	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE {
		t.Errorf("should remain ACTIVE (not enough samples for domain override), got %v", got.Status)
	}
}

// TestDomainDecay_MultipleOverrides_OnlyMatchingApplied — multiple overrides;
// only the one matching the domain is used.
func TestDomainDecay_MultipleOverrides_OnlyMatchingApplied(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	params := &types.Params{
		MinStakeAmount:     "100000000",
		DecayMinSamples:    10,
		DecayProbationBps:  600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:   750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "technology", DecayProbationBps: 800_000},
			{Domain: "mathematics", DecayProbationBps: 300_000},
			{Domain: "biology", DecayProbationBps: 700_000},
		},
	}
	k.SetParams(ctx, params)

	// accuracy 650_000: survives math (300k) and global (600k), demoted by technology (800k) and bio (700k).
	// We test "mathematics" domain: should SURVIVE (300k threshold).
	qMath := makeActiveQ(testAddr("val7"), "mathematics", 650_000)
	k.SetQualification(ctx, qMath)
	k.RunAccuracyDecay(ctx, 1000, params)
	gotMath, _ := k.GetQualification(ctx, qMath.Validator, qMath.Domain)
	if gotMath.Status != types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE {
		t.Errorf("mathematics: expected ACTIVE (override 300k), got %v", gotMath.Status)
	}

	// Same accuracy on "technology" domain: should be DEMOTED (800k threshold).
	qTech := makeActiveQ(testAddr("val8"), "technology", 650_000)
	k.SetQualification(ctx, qTech)
	k.RunAccuracyDecay(ctx, 1000, params)
	gotTech, _ := k.GetQualification(ctx, qTech.Validator, qTech.Domain)
	if gotTech.Status != types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY {
		t.Errorf("technology: expected PROBATIONARY (override 800k), got %v", gotTech.Status)
	}
}

// TestDomainDecay_NoOverrides_GlobalBehaviourPreserved — empty
// DomainDecayOverrides: existing behaviour unchanged.
func TestDomainDecay_NoOverrides_GlobalBehaviourPreserved(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	params := &types.Params{
		MinStakeAmount:       "100000000",
		DecayMinSamples:      10,
		DecayProbationBps:    600_000,
		DecaySuspensionBps:   400_000,
		DecayRecoveryBps:     750_000,
		DomainDecayOverrides: nil, // no overrides at all
	}
	k.SetParams(ctx, params)

	q := makeActiveQ(testAddr("val9"), "any-domain", 550_000)
	k.SetQualification(ctx, q)
	k.RunAccuracyDecay(ctx, 1000, params)

	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY {
		t.Errorf("expected PROBATIONARY (global 600k threshold, accuracy 550k), got %v", got.Status)
	}
}

// ─── Validate() tests ────────────────────────────────────────────────────────

func TestDomainDecayOverride_Validate_EmptyOverrides(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:       "100000000",
		DecayMinSamples:      20,
		DecayProbationBps:    600_000,
		DecaySuspensionBps:   400_000,
		DecayRecoveryBps:     750_000,
		DomainDecayOverrides: nil,
	}
	if err := p.Validate(); err != nil {
		t.Errorf("empty overrides should be valid, got: %v", err)
	}
}

func TestDomainDecayOverride_Validate_ValidOverrides(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayMinSamples:   20,
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "physics", DecayProbationBps: 700_000, DecaySuspensionBps: 500_000, DecayRecoveryBps: 800_000},
			{Domain: "economics", DecayProbationBps: 800_000},
		},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("valid overrides should pass, got: %v", err)
	}
}

func TestDomainDecayOverride_Validate_DuplicateDomain(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "physics", DecayProbationBps: 700_000},
			{Domain: "physics", DecayProbationBps: 800_000}, // duplicate
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("duplicate domain should fail validation")
	}
}

func TestDomainDecayOverride_Validate_EmptyDomain(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "", DecayProbationBps: 700_000},
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("empty domain name should fail validation")
	}
}

func TestDomainDecayOverride_Validate_SuspensionExceedsProbation(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "physics", DecayProbationBps: 500_000, DecaySuspensionBps: 600_000}, // susp > prob
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("suspension_bps > probation_bps should fail validation")
	}
}

func TestDomainDecayOverride_Validate_ProbationExceedsRecovery(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "economics", DecayProbationBps: 800_000, DecayRecoveryBps: 700_000}, // prob > rec
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("probation_bps > recovery_bps should fail validation")
	}
}

func TestDomainDecayOverride_Validate_RecoveryExceeds1M(t *testing.T) {
	p := &types.Params{
		MinStakeAmount:    "100000000",
		DecayProbationBps: 600_000,
		DecaySuspensionBps: 400_000,
		DecayRecoveryBps:  750_000,
		DomainDecayOverrides: []*types.DomainDecayOverride{
			{Domain: "biology", DecayProbationBps: 800_000, DecayRecoveryBps: 1_100_000}, // > 1M
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("recovery_bps > 1,000,000 should fail validation")
	}
}

// TestDomainDecay_RecoveryWithOverride — probationary validator in an
// override domain recovers when accuracy exceeds the override recovery threshold.
func TestDomainDecay_RecoveryWithOverride(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)

	// "sociology" override: recovery at 600_000 (lower than global 750_000).
	params := paramsWithOverride(
		600_000, 400_000, 750_000, 10, // global
		"sociology", 600_000, 400_000, 600_000, 0, // override: recovery at 600k
	)
	k.SetParams(ctx, params)

	ctx = ctx.WithBlockHeight(1000)
	// Accuracy 620_000: above override recovery (600k) → should recover from PROBATIONARY → ACTIVE.
	q := makeProbationary(testAddr("val10"), "sociology", 620_000)
	k.SetQualification(ctx, q)
	k.RunAccuracyDecay(ctx, 1000, params)

	got, _ := k.GetQualification(ctx, q.Validator, q.Domain)
	if got.Status != types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE {
		t.Errorf("should recover under override recovery threshold (600k), got %v", got.Status)
	}
}

