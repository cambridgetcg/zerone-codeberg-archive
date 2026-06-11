package keeper

import (
	"context"
	"math"
	"math/big"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// VerificationResult holds the aggregated outcome of a verification round.
// This is a local struct (not proto) used only within the keeper.
type VerificationResult struct {
	Verdict    types.Verdict
	Confidence uint64 // 0-1,000,000
	Rewards    []VerifierReward
	Slashes    []VerifierSlash

	AcceptCount uint64 // raw headcount (not stake-weighted) for diversity
	RejectCount uint64 // raw headcount (not stake-weighted) for diversity
}

// VerifierReward records a reward amount for a correct verifier.
type VerifierReward struct {
	Verifier string
	Amount   uint64
}

// VerifierSlash records a slash for an incorrect or absent verifier.
type VerifierSlash struct {
	Verifier            string
	SlashBps            uint64
	VindicationEligible bool // true for wrong-vote slashes, false for missed-reveal/equivocation
}

// AggregateVerificationResult performs stake-weighted vote aggregation.
func (k Keeper) AggregateVerificationResult(ctx context.Context, round *types.VerificationRound) (*VerificationResult, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	// Count stake-weighted accept/reject/malformed votes from reveals
	var acceptStake, rejectStake, malformedStake, totalVoteStake uint64

	for _, reveal := range round.Reveals {
		var stake uint64
		if k.stakingKeeper != nil {
			s, err := k.stakingKeeper.GetEffectiveStake(ctx, reveal.Verifier)
			if err == nil {
				stake = s
			}
		}
		if stake == 0 {
			stake = 1 // minimum weight for unknown validators
		}

		// Apply agent verification bonus (R28-5), modulated by domain role elasticity (R29-3)
		if params.AgentVerificationBonusBps > 0 {
			accountType := k.getAccountType(ctx, reveal.Verifier)
			if accountType == "agent" {
				domain := k.getDomainForRound(ctx, round)
				agentBonus, _ := k.GetRoleElasticity(ctx, domain)
				stake = safeMulDiv(stake, 1_000_000+agentBonus, 1_000_000)
			}
		}

		totalVoteStake += stake
		switch reveal.Vote {
		case "accept":
			acceptStake += stake
		case "reject":
			rejectStake += stake
		case "malformed":
			malformedStake += stake
		}
	}

	// Count raw headcounts for diversity (1 validator = 1 signal)
	var rawAccept, rawReject uint64
	for _, reveal := range round.Reveals {
		switch reveal.Vote {
		case "accept":
			rawAccept++
		case "reject":
			rawReject++
		}
	}

	result := &VerificationResult{}

	// Check quorum: total reveals must meet the effective quorum requirement.
	// Skepticism Gap 3: extraordinary claims use round.ExtraordinaryMinVerifiers
	// (set at round creation) when it exceeds the global min_verifiers.
	effectiveMinVerifiers := params.MinVerifiers
	if round.ExtraordinaryMinVerifiers > effectiveMinVerifiers {
		effectiveMinVerifiers = round.ExtraordinaryMinVerifiers
	}
	if uint64(len(round.Reveals)) < effectiveMinVerifiers {
		result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
		result.Confidence = 0
		return result, nil
	}

	// Calculate vote ratios (BPS scale)
	var acceptRatio, rejectRatio, malformedRatio uint64
	if totalVoteStake > 0 {
		acceptRatio = safeMulDiv(acceptStake, 1_000_000, totalVoteStake)
		rejectRatio = safeMulDiv(rejectStake, 1_000_000, totalVoteStake)
		malformedRatio = safeMulDiv(malformedStake, 1_000_000, totalVoteStake)
	}

	// Count raw headcount by vote for the T1 headcount-floor check below.
	var acceptCount, rejectCount, malformedCount uint64
	for _, reveal := range round.Reveals {
		switch reveal.Vote {
		case "accept":
			acceptCount++
		case "reject":
			rejectCount++
		case "malformed":
			malformedCount++
		}
	}

	// Determine verdict — check malformed FIRST: a malformed claim should
	// never become a fact regardless of accept votes.
	// Each branch must also satisfy MinHeadcountAgreement so a stake-heavy
	// coalition cannot promote a claim past the threshold alone (T1).
	// Cap at MinVerifiers so chain configs requiring fewer verifiers than
	// the default headcount still produce verdicts.
	headcountFloor := params.MinHeadcountAgreement
	if headcountFloor > params.MinVerifiers {
		headcountFloor = params.MinVerifiers
	}
	switch {
	case malformedRatio >= params.ConfidenceThreshold && malformedCount >= headcountFloor:
		result.Verdict = types.Verdict_VERDICT_MALFORMED
		result.Confidence = malformedRatio
	case acceptRatio >= params.ConfidenceThreshold && acceptCount >= headcountFloor:
		result.Verdict = types.Verdict_VERDICT_ACCEPT
		result.Confidence = acceptRatio
	case rejectRatio >= params.ConfidenceThreshold && rejectCount >= headcountFloor:
		result.Verdict = types.Verdict_VERDICT_REJECT
		result.Confidence = rejectRatio
	default:
		result.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
		result.Confidence = 0
	}

	// Apply aggregate extremizing (Calibrated Forecasting Gap 5).
	// Tetlock's formula corrects for under-confidence at the aggregate level:
	// individual probability estimates share evidence, so the simple average
	// under-weights their combined signal. Extremizing pushes the result away
	// from 50% by a governance-configured factor α.
	// Only applied to binary (ACCEPT/REJECT) verdicts; MALFORMED/INCONCLUSIVE
	// are not probability estimates and are left unchanged.
	if params.AggregationExtremizingAlpha >= 1000 &&
		(result.Verdict == types.Verdict_VERDICT_ACCEPT || result.Verdict == types.Verdict_VERDICT_REJECT) {
		result.Confidence = extremizeConfidence(result.Confidence, params.AggregationExtremizingAlpha)
	}

	// Apply stratum ceiling to confidence
	if k.ontologyKeeper != nil && result.Verdict == types.Verdict_VERDICT_ACCEPT {
		claim, found := k.GetClaim(ctx, round.ClaimId)
		if found && claim.Domain != "" {
			stratum, err := k.ontologyKeeper.GetStratumForDomain(ctx, claim.Domain)
			if err == nil && stratum != "" {
				ceiling, err := k.ontologyKeeper.GetConfidenceCeiling(ctx, stratum)
				if err == nil && ceiling > 0 && result.Confidence > ceiling {
					result.Confidence = ceiling
				}
			}
		}
	}

	// Apply global MaxConfidence hard cap
	if params.MaxConfidence > 0 && result.Confidence > params.MaxConfidence {
		result.Confidence = params.MaxConfidence
	}

	// Calculate rewards and slashes
	k.calculateRewardsAndSlashes(ctx, round, result, params)

	result.AcceptCount = rawAccept
	result.RejectCount = rawReject

	return result, nil
}

// calculateRewardsAndSlashes determines rewards for correct voters and slashes for incorrect ones.
func (k Keeper) calculateRewardsAndSlashes(ctx context.Context, round *types.VerificationRound, result *VerificationResult, params *types.Params) {
	// Determine which vote is "correct" based on verdict
	var correctVote string
	var partialRewardVote string      // vote that gets reduced reward instead of slash
	var partialRewardRatio uint64     // BPS ratio of base reward for partial (0 = no partial)
	switch result.Verdict {
	case types.Verdict_VERDICT_ACCEPT:
		correctVote = "accept"
	case types.Verdict_VERDICT_REJECT:
		correctVote = "reject"
	case types.Verdict_VERDICT_MALFORMED:
		correctVote = "malformed"
		// "reject" voters are directionally correct but imprecise — half reward
		partialRewardVote = "reject"
		partialRewardRatio = 500_000 // 50% of base reward
	default:
		// Inconclusive — no rewards or slashes
		return
	}

	// Parse base reward
	baseReward := new(big.Int)
	if params.VerificationReward != "" {
		baseReward.SetString(params.VerificationReward, 10)
	}
	if baseReward.Sign() <= 0 {
		return
	}

	// Build reveal map for lookup
	revealMap := make(map[string]string) // verifier → vote
	for _, reveal := range round.Reveals {
		revealMap[reveal.Verifier] = reveal.Vote
	}

	// Process all committed verifiers
	for _, commit := range round.Commits {
		vote, revealed := revealMap[commit.Verifier]

		if !revealed {
			// Committed but did not reveal — slash for missed reveal
			result.Slashes = append(result.Slashes, VerifierSlash{
				Verifier: commit.Verifier,
				SlashBps: params.MissedRevealSlashBps,
			})
			continue
		}

		if vote == correctVote {
			// Correct vote — full reward
			if baseReward.IsUint64() {
				result.Rewards = append(result.Rewards, VerifierReward{
					Verifier: commit.Verifier,
					Amount:   baseReward.Uint64(),
				})
			}
		} else if partialRewardVote != "" && vote == partialRewardVote {
			// Partially correct vote — reduced reward
			if baseReward.IsUint64() {
				partialAmt := safeMulDiv(baseReward.Uint64(), partialRewardRatio, 1_000_000)
				if partialAmt > 0 {
					result.Rewards = append(result.Rewards, VerifierReward{
						Verifier: commit.Verifier,
						Amount:   partialAmt,
					})
				}
			}
		} else {
			// Incorrect vote — slash (vindication-eligible: may be refunded if fact later disproven)
			result.Slashes = append(result.Slashes, VerifierSlash{
				Verifier:            commit.Verifier,
				SlashBps:            params.WrongVerificationSlashBps,
				VindicationEligible: true,
			})
		}
	}
}

// ClampConfidence enforces the MaxConfidence hard cap and optional stratum ceiling.
func (k Keeper) ClampConfidence(ctx context.Context, confidence uint64, domain string) uint64 {
	params, err := k.GetParams(ctx)
	if err != nil {
		return confidence
	}

	// Apply stratum ceiling if ontology keeper is available
	if k.ontologyKeeper != nil && domain != "" {
		stratum, err := k.ontologyKeeper.GetStratumForDomain(ctx, domain)
		if err == nil && stratum != "" {
			ceiling, err := k.ontologyKeeper.GetConfidenceCeiling(ctx, stratum)
			if err == nil && ceiling > 0 && confidence > ceiling {
				confidence = ceiling
			}
		}
	}

	// Apply epistemic temperature cap modulation (R29-2)
	if domain != "" {
		epistemicState, found, err := k.GetDomainEpistemicState(ctx, domain)
		if err == nil && found {
			effectiveCap := params.MaxConfidence
			if effectiveCap == 0 {
				effectiveCap = 880_000
			}

			// Cold domains: lower cap — untested consensus shouldn't be highly confident
			if epistemicState.Temperature < 300_000 && params.EpistemicColdConfidenceCapBps > 0 {
				if params.EpistemicColdConfidenceCapBps < effectiveCap {
					effectiveCap = params.EpistemicColdConfidenceCapBps
				}
			}

			// Very hot domains: allow up to SurvivedChallengeConfidenceCap
			if epistemicState.Temperature > 800_000 && params.SurvivedChallengeConfidenceCap > effectiveCap {
				effectiveCap = params.SurvivedChallengeConfidenceCap
			}

			if confidence > effectiveCap {
				confidence = effectiveCap
			}
			return confidence
		}
	}

	// Fallback: apply global hard cap (no epistemic state found)
	if params.MaxConfidence > 0 && confidence > params.MaxConfidence {
		confidence = params.MaxConfidence
	}

	return confidence
}

// safeMulDiv computes (a * b / c) using big.Int to prevent overflow.
func safeMulDiv(a, b, c uint64) uint64 {
	if c == 0 {
		return 0
	}
	result := new(big.Int).SetUint64(a)
	result.Mul(result, new(big.Int).SetUint64(b))
	result.Div(result, new(big.Int).SetUint64(c))
	if !result.IsUint64() {
		return ^uint64(0)
	}
	return result.Uint64()
}

// ChallengeStakeFloorBps bounds how cheap a probe can get. A single
// nonzero floor prevents axiom-level facts from being challengeable for
// dust while preserving the Popperian invitation: everyone should probe
// our most-trusted claims the most.
const ChallengeStakeFloorBps uint64 = 100_000 // 10% of base

// EffectiveMinChallengeStake computes the confidence-weighted minimum
// challenge stake for a target fact.
//
// Popperian antifragility: truth stands firm under challenge because of
// its nature, so the substrate must invite probing of high-confidence
// claims rather than tax it. This function scales the stake INVERSELY
// with the target fact's confidence — the more the community trusts a
// claim, the cheaper it is to stress-test it. Low-confidence facts pay
// the full base stake (they're easy pickings; no subsidy needed). High-
// confidence facts approach the ChallengeStakeFloorBps floor.
//
// Formula: base × max(floor, 1 - confidence × scaling / BPS) / BPS
//
//	confidence=0            → 1.00× base  (full price; nothing to prove)
//	confidence=0.5, scale=1 → 0.50× base  (half-price; worth testing)
//	confidence=0.9, scale=1 → 0.10× base  (floor; probe aggressively)
//	confidence=1.0, scale=1 → floor       (clamped; still costs something)
//
// A param ChallengeConfidenceScalingBps = 0 disables the discount (all
// probes cost the full base stake) — provides a governance escape hatch
// if this antifragile posture needs temporary tightening during an
// active attack.
func EffectiveMinChallengeStake(params *types.Params, targetConfidence uint64) *big.Int {
	if params == nil {
		return big.NewInt(0)
	}
	baseStr := params.MinChallengeStake
	base, ok := new(big.Int).SetString(baseStr, 10)
	if !ok || base.Sign() <= 0 {
		return big.NewInt(0)
	}
	if params.ChallengeConfidenceScalingBps == 0 || targetConfidence == 0 {
		return base
	}
	const bps uint64 = 1_000_000
	discountBps := safeMulDiv(targetConfidence, params.ChallengeConfidenceScalingBps, bps)
	if discountBps > bps {
		discountBps = bps
	}
	multiplierBps := bps - discountBps
	if multiplierBps < ChallengeStakeFloorBps {
		multiplierBps = ChallengeStakeFloorBps
	}
	scaled := new(big.Int).Mul(base, new(big.Int).SetUint64(multiplierBps))
	scaled.Div(scaled, new(big.Int).SetUint64(bps))
	return scaled
}

// extremizeConfidence applies Tetlock's aggregate-extremizing formula to a
// panel confidence score, pushing it away from 50% (500_000 BPS).
//
//	extremized = p^α / (p^α + (1−p)^α)
//
// alphaMilli is α stored as milliunits: 1000 = α=1.0 (identity), 1500 = α=1.5.
// 50% is the fixed point: extremizing 500_000 yields 500_000.
// Results ≤0 clamp to 0; results ≥1_000_000 clamp to 1_000_000.
//
// Exported for testing via ExtremeizeConfidenceForTest.
func extremizeConfidence(confidenceBps uint64, alphaMilli uint64) uint64 {
	if alphaMilli == 0 || alphaMilli == 1000 {
		return confidenceBps // disabled or identity
	}
	if confidenceBps == 0 {
		return 0
	}
	if confidenceBps >= 1_000_000 {
		return 1_000_000
	}
	p := float64(confidenceBps) / 1_000_000.0
	alpha := float64(alphaMilli) / 1000.0
	pAlpha := math.Pow(p, alpha)
	qAlpha := math.Pow(1.0-p, alpha)
	denom := pAlpha + qAlpha
	if denom == 0 {
		return confidenceBps // degenerate: shouldn't happen for p ∈ (0,1)
	}
	result := pAlpha / denom
	if result < 0 {
		return 0
	}
	if result > 1.0 {
		return 1_000_000
	}
	return uint64(math.Round(result * 1_000_000))
}

// ExtremizeConfidenceForTest is an exported thin wrapper for unit tests
// (package keeper_test cannot access unexported extremizeConfidence directly).
func ExtremizeConfidenceForTest(confidenceBps uint64, alphaMilli uint64) uint64 {
	return extremizeConfidence(confidenceBps, alphaMilli)
}
