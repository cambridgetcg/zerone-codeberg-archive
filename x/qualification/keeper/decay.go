package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/qualification/types"
)

// effectiveDecayParams returns the decay thresholds applicable to domain d.
// It checks Params.DomainDecayOverrides for an exact match; any zero field
// in the override falls back to the corresponding global Params value.
// Returns (probationBps, suspensionBps, recoveryBps, minSamples).
func effectiveDecayParams(params *types.Params, domain string) (uint64, uint64, uint64, uint64) {
	prob := params.DecayProbationBps
	susp := params.DecaySuspensionBps
	rec := params.DecayRecoveryBps
	min := params.DecayMinSamples

	for _, ov := range params.DomainDecayOverrides {
		if ov == nil || ov.Domain != domain {
			continue
		}
		if ov.DecayProbationBps > 0 {
			prob = ov.DecayProbationBps
		}
		if ov.DecaySuspensionBps > 0 {
			susp = ov.DecaySuspensionBps
		}
		if ov.DecayRecoveryBps > 0 {
			rec = ov.DecayRecoveryBps
		}
		if ov.DecayMinSamples > 0 {
			min = ov.DecayMinSamples
		}
		break
	}
	return prob, susp, rec, min
}

// RunAccuracyDecay scans every qualification with sufficient sample
// size and transitions state based on the current AccuracyBps. The
// transitions form a self-correcting feedback loop on top of the
// per-domain panel's RecordVerificationOutcome writes:
//
//	ACTIVE       + accuracy <  decay_probation_bps  → PROBATIONARY
//	PROBATIONARY + accuracy <  decay_suspension_bps → SUSPENDED
//	PROBATIONARY + accuracy >= decay_recovery_bps   → ACTIVE
//
// Domain-specific thresholds are applied when DomainDecayOverrides
// contains an entry for the qualification's domain (Scientific Method
// Gap 5). Fast-moving domains (technology, economics) decay faster;
// stable domains (mathematics, formal logic) decay slower.
//
// Skipped qualifications:
//   - TotalVerifications < minSamples (insufficient signal yet)
//   - Status not in {ACTIVE, PROBATIONARY} (already EXPIRED / SUSPENDED /
//     REVOKED — those have their own paths back)
//
// Cost: O(n × |domain_decay_overrides|) on qualifications. Caller
// (BeginBlocker) gates the scan to once per DecayCheckIntervalBlocks.
func (k Keeper) RunAccuracyDecay(ctx context.Context, currentBlock uint64, params *types.Params) {
	if params == nil {
		return
	}
	if params.DecayMinSamples == 0 {
		return
	}
	// Fast-path: decay fully disabled when all global thresholds are zero
	// and there are no per-domain overrides.
	if params.DecayProbationBps == 0 && params.DecaySuspensionBps == 0 &&
		params.DecayRecoveryBps == 0 && len(params.DomainDecayOverrides) == 0 {
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	k.IterateQualifications(ctx, func(q *types.DomainQualification) bool {
		if q == nil || q.Metrics == nil {
			return false
		}

		probationBps, suspensionBps, recoveryBps, minSamples := effectiveDecayParams(params, q.Domain)

		if q.Metrics.TotalVerifications < minSamples {
			return false
		}
		accuracy := q.Metrics.AccuracyBps

		switch q.Status {
		// All three decay transitions below announce commitment 7:
		// skill is current, not historical. Each event marks a
		// validator's qualification weight changing because their
		// recent accuracy crossed a threshold. See TRUTH_SEEKING.md.

		case types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE:
			if probationBps > 0 && accuracy < probationBps {
				q.Status = types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY
				// Set probation deadline: standard probation period from
				// the moment of demotion. If the voter doesn't recover
				// within this window, the existing PROBATIONARY → expiry
				// path handles the rest.
				if params.ProbationPeriod > 0 {
					q.ProbationUntil = currentBlock + params.ProbationPeriod
				}
				k.SetQualification(ctx, q)
				sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
					"zerone.qualification.decay_probation",
					sdk.NewAttribute("validator", q.Validator),
					sdk.NewAttribute("domain", q.Domain),
					sdk.NewAttribute("accuracy_bps", fmt.Sprintf("%d", accuracy)),
					sdk.NewAttribute("threshold_bps", fmt.Sprintf("%d", probationBps)),
					sdk.NewAttribute("creed_commitment", "7"),
				))
			}

		case types.QualificationStatus_QUALIFICATION_STATUS_PROBATIONARY:
			if suspensionBps > 0 && accuracy < suspensionBps {
				q.Status = types.QualificationStatus_QUALIFICATION_STATUS_SUSPENDED
				q.ProbationUntil = 0
				k.SetQualification(ctx, q)
				sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
					"zerone.qualification.decay_suspension",
					sdk.NewAttribute("validator", q.Validator),
					sdk.NewAttribute("domain", q.Domain),
					sdk.NewAttribute("accuracy_bps", fmt.Sprintf("%d", accuracy)),
					sdk.NewAttribute("threshold_bps", fmt.Sprintf("%d", suspensionBps)),
					sdk.NewAttribute("creed_commitment", "7"),
				))
				return false
			}
			if recoveryBps > 0 && accuracy >= recoveryBps {
				q.Status = types.QualificationStatus_QUALIFICATION_STATUS_ACTIVE
				q.ProbationUntil = 0
				k.SetQualification(ctx, q)
				sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
					"zerone.qualification.decay_recovered",
					sdk.NewAttribute("validator", q.Validator),
					sdk.NewAttribute("domain", q.Domain),
					sdk.NewAttribute("accuracy_bps", fmt.Sprintf("%d", accuracy)),
					sdk.NewAttribute("threshold_bps", fmt.Sprintf("%d", recoveryBps)),
					sdk.NewAttribute("creed_commitment", "7"),
				))
			}
		}
		return false
	})
}
