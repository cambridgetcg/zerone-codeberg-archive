package keeper

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// DomainStats tracks the population of a domain for carrying capacity calculations.
type DomainStats struct {
	Domain                string `json:"domain"`
	ActiveCount           uint64 `json:"active_count"`
	AtRiskCount           uint64 `json:"at_risk_count"`
	TotalEnergy           uint64 `json:"total_energy"`
	LastUpdated           uint64 `json:"last_updated"`            // block height
	MentorshipGraduations uint64 `json:"mentorship_graduations"` // R31-5: Water → Wood
	// Base-rate tracking (Calibrated Forecasting Gap 3).
	// DomainAccepted + DomainRejected count all binary verdicts in this domain.
	// The prior probability P(ACCEPT|domain) = DomainAccepted / (DomainAccepted + DomainRejected).
	DomainAccepted uint64 `json:"domain_accepted"`
	DomainRejected uint64 `json:"domain_rejected"`
}

func (k Keeper) SetDomainStats(ctx context.Context, stats *DomainStats) {
	store := k.storeService.OpenKVStore(ctx)
	bz, _ := json.Marshal(stats)
	_ = store.Set(types.DomainStatsKey(stats.Domain), bz)
}

func (k Keeper) GetDomainStats(ctx context.Context, domain string) (*DomainStats, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.DomainStatsKey(domain))
	if err != nil || bz == nil {
		return &DomainStats{Domain: domain}, false
	}
	var stats DomainStats
	if err := json.Unmarshal(bz, &stats); err != nil {
		return &DomainStats{Domain: domain}, false
	}
	return &stats, true
}

func (k Keeper) IncrementDomainFactCount(ctx context.Context, domain string, isActive bool, energy uint64) {
	stats, _ := k.GetDomainStats(ctx, domain)
	stats.Domain = domain
	if isActive {
		stats.ActiveCount++
	} else {
		stats.AtRiskCount++
	}
	stats.TotalEnergy += energy
	k.SetDomainStats(ctx, stats)
}

func (k Keeper) DecrementDomainFactCount(ctx context.Context, domain string, wasActive bool, energy uint64) {
	stats, _ := k.GetDomainStats(ctx, domain)
	stats.Domain = domain
	if wasActive {
		if stats.ActiveCount > 0 {
			stats.ActiveCount--
		}
	} else {
		if stats.AtRiskCount > 0 {
			stats.AtRiskCount--
		}
	}
	if energy > stats.TotalEnergy {
		stats.TotalEnergy = 0
	} else {
		stats.TotalEnergy -= energy
	}
	k.SetDomainStats(ctx, stats)
}

// RecordDomainVerdict increments the lifetime accept/reject counters for a domain.
// Called from CompleteRound for ACCEPT and REJECT verdicts. MALFORMED and
// INCONCLUSIVE are excluded: they reflect claim quality issues rather than the
// domain's inherent acceptance prior (Calibrated Forecasting Gap 3).
func (k Keeper) RecordDomainVerdict(ctx context.Context, domain string, accepted bool) {
	if domain == "" {
		return
	}
	stats, _ := k.GetDomainStats(ctx, domain)
	stats.Domain = domain
	if accepted {
		stats.DomainAccepted++
	} else {
		stats.DomainRejected++
	}
	k.SetDomainStats(ctx, stats)
}

// DomainAcceptanceRateBps returns the domain's prior acceptance rate in BPS.
// Returns 0 if no binary verdicts have been recorded yet.
func DomainAcceptanceRateBps(stats *DomainStats) uint64 {
	total := stats.DomainAccepted + stats.DomainRejected
	if total == 0 {
		return 0
	}
	return stats.DomainAccepted * 1_000_000 / total
}

// ApplyMentorshipDividend adds energy to a domain and increments its graduation
// counter when a mentorship graduates (R31-5: Water → Wood).
func (k Keeper) ApplyMentorshipDividend(ctx context.Context, domain, mentor, mentee string) {
	params, _ := k.GetParams(ctx)
	dividendEnergy := params.MentorshipDividendEnergy
	if dividendEnergy == 0 {
		dividendEnergy = 50_000 // safety default
	}

	stats, _ := k.GetDomainStats(ctx, domain)
	stats.Domain = domain
	stats.MentorshipGraduations++
	stats.TotalEnergy += dividendEnergy

	// Cap at MetabolismEnergyCap
	energyCap := params.MetabolismEnergyCap
	if energyCap == 0 {
		energyCap = 1_000_000
	}
	if stats.TotalEnergy > energyCap {
		stats.TotalEnergy = energyCap
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	stats.LastUpdated = uint64(sdkCtx.BlockHeight())
	k.SetDomainStats(ctx, stats)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.mentorship_dividend_applied",
		sdk.NewAttribute("domain", domain),
		sdk.NewAttribute("mentor", mentor),
		sdk.NewAttribute("mentee", mentee),
		sdk.NewAttribute("energy_added", fmt.Sprintf("%d", dividendEnergy)),
		sdk.NewAttribute("total_energy", fmt.Sprintf("%d", stats.TotalEnergy)),
		sdk.NewAttribute("graduations", fmt.Sprintf("%d", stats.MentorshipGraduations)),
	))
}

// TransitionDomainFactStatus updates stats when a fact moves between active/at-risk.
func (k Keeper) TransitionDomainFactStatus(ctx context.Context, domain string, toActive bool) {
	stats, _ := k.GetDomainStats(ctx, domain)
	stats.Domain = domain
	if toActive {
		if stats.AtRiskCount > 0 {
			stats.AtRiskCount--
		}
		stats.ActiveCount++
	} else {
		if stats.ActiveCount > 0 {
			stats.ActiveCount--
		}
		stats.AtRiskCount++
	}
	k.SetDomainStats(ctx, stats)
}

// ─── Carrying capacity and pressure ─────────────────────────────────────────

const BPSCapacity = 1_000_000

func (k Keeper) GetDomainCarryingCapacity(ctx context.Context, domain string) uint64 {
	params, _ := k.GetParams(ctx)
	base := params.DomainBaseCapacity
	if base == 0 {
		base = 1000 // safety default
	}
	inbound := k.GetInboundCrossDomainCitationCount(ctx, domain)
	bonus := inbound * params.DomainCapacityGrowthPerCitation

	// R31-5: Water → Wood — mentorship graduations expand carrying capacity.
	stats, _ := k.GetDomainStats(ctx, domain)
	mentorshipBonus := stats.MentorshipGraduations * params.MentorshipCapacityBonus

	total := base + bonus + mentorshipBonus

	// Metal controls Wood: capture flag penalty reduces capacity (R31-1)
	if k.captureDefenseKeeper != nil {
		flagged, penaltyBps := k.captureDefenseKeeper.GetDomainCapturePenalty(ctx, domain)
		if flagged {
			reduction := safeMulDiv(total, penaltyBps, BPSCapacity)
			if reduction >= total {
				return 1 // minimum capacity — can't go to zero
			}
			total = total - reduction
		}
	}

	// R31-4: Metal controls Wood — stratum depth constrains carrying capacity.
	stratumMultiplier := k.getStratumCapacityMultiplier(ctx, domain)
	if stratumMultiplier < BPSCapacity {
		effectiveCapacity := total * stratumMultiplier / BPSCapacity

		sdkCtx := sdk.UnwrapSDKContext(ctx)
		depth := uint32(1)
		if k.ontologyKeeper != nil {
			if d, err := k.ontologyKeeper.GetDepthForDomain(ctx, domain); err == nil {
				depth = d
			}
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.stratum_capacity_applied",
			sdk.NewAttribute("domain", domain),
			sdk.NewAttribute("stratum_depth", fmt.Sprintf("%d", depth)),
			sdk.NewAttribute("capacity_multiplier_bps", fmt.Sprintf("%d", stratumMultiplier)),
			sdk.NewAttribute("effective_capacity", fmt.Sprintf("%d", effectiveCapacity)),
		))

		return effectiveCapacity
	}

	return total
}

// getStratumCapacityMultiplier returns a BPS multiplier based on domain depth.
// Deeper strata (higher depth) have naturally lower carrying capacity (R31-4: Metal controls Wood).
// Depth 1 (root): 100%, Depth 2: 80%, Depth 3: 60%, Depth 4+: 50%.
func (k Keeper) getStratumCapacityMultiplier(ctx context.Context, domain string) uint64 {
	if k.ontologyKeeper == nil {
		return BPSCapacity // 1x — no ontology module wired
	}

	depth, err := k.ontologyKeeper.GetDepthForDomain(ctx, domain)
	if err != nil {
		return BPSCapacity // 1x on error (domain not found, etc.)
	}

	switch {
	case depth <= 1:
		return BPSCapacity // 100%
	case depth == 2:
		return 800_000 // 80%
	case depth == 3:
		return 600_000 // 60%
	default:
		return 500_000 // 50% floor for depth 4+
	}
}

func (k Keeper) GetDomainPressure(ctx context.Context, domain string) uint64 {
	stats, _ := k.GetDomainStats(ctx, domain)
	capacity := k.GetDomainCarryingCapacity(ctx, domain)
	if capacity == 0 {
		return BPSCapacity
	}
	population := stats.ActiveCount + stats.AtRiskCount
	return safeMulDiv(population, BPSCapacity, capacity)
}

// GetInboundCrossDomainCitationCount counts citations FROM other domains TO facts in this domain.
func (k Keeper) GetInboundCrossDomainCitationCount(ctx context.Context, domain string) uint64 {
	count := uint64(0)
	k.IterateFactsByDomain(ctx, domain, func(factID string) bool {
		incoming, err := k.GetIncomingRelations(ctx, factID)
		if err != nil {
			return false
		}
		for _, rel := range incoming {
			sourceFact, found := k.GetFact(ctx, rel.SourceFactId)
			if found && sourceFact.Domain != domain {
				count++
			}
		}
		return false
	})
	return count
}

// ─── Birth and death pressure ───────────────────────────────────────────────

// GetDeathPressureMultiplier returns the decay multiplier for a domain.
// >1M BPS = faster decay (overcrowded), <1M BPS = slower decay (sparse), 1M = normal.
func (k Keeper) GetDeathPressureMultiplier(ctx context.Context, domain string) uint64 {
	params, err := k.GetParams(ctx)
	if err != nil || params.DomainBaseCapacity == 0 {
		return BPSCapacity
	}
	pressure := k.GetDomainPressure(ctx, domain)

	if pressure > BPSCapacity {
		// Overcrowded: accelerate decay
		excess := pressure - BPSCapacity
		return BPSCapacity + safeMulDiv(excess, params.OvercrowdingDecayMultiplierBps-BPSCapacity, BPSCapacity)
	} else if pressure < BPSCapacity/2 {
		// Very sparse (< 50% capacity): slow decay to 75%
		return BPSCapacity * 3 / 4
	}
	return BPSCapacity // normal range
}

// ApplyBirthPressure adjusts initial energy based on domain pressure.
// Sparse domains give an energy bonus; overcrowded domains give no bonus.
func (k Keeper) ApplyBirthPressure(ctx context.Context, domain string, baseEnergy uint64) uint64 {
	params, err := k.GetParams(ctx)
	if err != nil || params.DomainBaseCapacity == 0 {
		return baseEnergy
	}
	pressure := k.GetDomainPressure(ctx, domain)
	if pressure >= BPSCapacity {
		return baseEnergy // at or over capacity — no bonus
	}
	// Under capacity: bonus proportional to sparseness
	sparseness := BPSCapacity - pressure
	bonus := safeMulDiv(
		safeMulDiv(baseEnergy, sparseness, BPSCapacity),
		params.UnderpopulationBirthBonusBps,
		BPSCapacity,
	)
	return baseEnergy + bonus
}

// PressureCategory returns a human-readable category for the pressure level.
func PressureCategory(pressureBps uint64) string {
	switch {
	case pressureBps < 250_000:
		return "sparse"
	case pressureBps < 750_000:
		return "normal"
	case pressureBps <= BPSCapacity:
		return "crowded"
	default:
		return "overcrowded"
	}
}

// ─── Pending verification ratio (R31-1: Wood controls Earth) ────────────────

// GetPendingVerificationRatio returns pending claims / active facts in BPS.
// Used by alignment module to detect knowledge growth pressure.
func (k Keeper) GetPendingVerificationRatio(ctx context.Context) uint64 {
	var pending, active uint64
	k.IterateClaims(ctx, func(claim *types.Claim) bool {
		switch claim.Status {
		case types.ClaimStatus_CLAIM_STATUS_PENDING,
			types.ClaimStatus_CLAIM_STATUS_PENDING_EVALUATION,
			types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION:
			pending++
		case types.ClaimStatus_CLAIM_STATUS_ACCEPTED:
			active++
		}
		return false
	})
	if active == 0 {
		if pending > 0 {
			return BPSCapacity * 2 // infinite ratio capped at 200%
		}
		return BPSCapacity // 1:1 when both zero
	}
	return safeMulDiv(pending, BPSCapacity, active)
}

// ─── Events ─────────────────────────────────────────────────────────────────

// EmitCapacityPenaltyEvent emits a capacity_penalty_applied event when capture penalty reduces capacity (R31-1).
func (k Keeper) EmitCapacityPenaltyEvent(ctx context.Context, domain string) {
	if k.captureDefenseKeeper == nil {
		return
	}
	flagged, penaltyBps := k.captureDefenseKeeper.GetDomainCapturePenalty(ctx, domain)
	if !flagged {
		return
	}

	params, _ := k.GetParams(ctx)
	base := params.DomainBaseCapacity
	if base == 0 {
		base = 1000
	}
	inbound := k.GetInboundCrossDomainCitationCount(ctx, domain)
	bonus := inbound * params.DomainCapacityGrowthPerCitation
	baseTotal := base + bonus
	effective := k.GetDomainCarryingCapacity(ctx, domain)

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.capacity_penalty_applied",
		sdk.NewAttribute("domain", domain),
		sdk.NewAttribute("base_capacity", fmt.Sprintf("%d", baseTotal)),
		sdk.NewAttribute("effective_capacity", fmt.Sprintf("%d", effective)),
		sdk.NewAttribute("capture_penalty_bps", fmt.Sprintf("%d", penaltyBps)),
		sdk.NewAttribute("reason", "capture_flagged"),
	))
}

// EmitDomainPressureEvent emits a domain_pressure_changed event.
func (k Keeper) EmitDomainPressureEvent(ctx context.Context, domain string) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	stats, _ := k.GetDomainStats(ctx, domain)
	capacity := k.GetDomainCarryingCapacity(ctx, domain)
	pressure := k.GetDomainPressure(ctx, domain)
	category := PressureCategory(pressure)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.domain_pressure_changed",
		sdk.NewAttribute("domain", domain),
		sdk.NewAttribute("active_count", fmt.Sprintf("%d", stats.ActiveCount)),
		sdk.NewAttribute("capacity", fmt.Sprintf("%d", capacity)),
		sdk.NewAttribute("pressure_bps", fmt.Sprintf("%d", pressure)),
		sdk.NewAttribute("category", category),
	))
}
