package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Scientific Method Gap 1: Verifier Independence ──────────────────────────
//
// Panel diversity measures organizational independence of a verification panel.
// Diversity score = (distinct_org_count / total_verifiers) × 1_000_000.
//
// The replication crisis lesson: three verifiers from the same institution or
// trained on the same corpus are not three independent assessments, even if
// they commit blindly through the commit-reveal scheme. Structural independence
// is a prerequisite for genuine epistemic independence.

// ComputePanelDiversityScore returns the panel diversity score in BPS
// (1_000_000 = maximum diversity, one distinct org per verifier).
//
// Algorithm:
//   - For each verifier in reveals, fetch their AgentCalibration.OrganizationId
//   - Empty organization_id → treated as a unique singleton (distinct from all others)
//   - Score = distinct_org_count / total_count × 1_000_000
//   - Returns 1_000_000 for empty panels (fail-open)
func (k Keeper) ComputePanelDiversityScore(ctx context.Context, round *types.VerificationRound) uint64 {
	verifiers := revealVerifiers(round)
	total := len(verifiers)
	if total == 0 {
		return 1_000_000 // fail-open: no verifiers = full diversity (nothing to measure)
	}

	// Count distinct organization identifiers.
	// Verifiers with empty org are each counted as a unique singleton org
	// (identified by their address), so they never reduce the diversity score.
	orgSeen := make(map[string]struct{}, total)
	for i, addr := range verifiers {
		cal, found := k.GetAgentCalibration(ctx, addr)
		var orgKey string
		if found && cal.OrganizationId != "" {
			orgKey = "org:" + cal.OrganizationId
		} else {
			// Unknown affiliation: treat as own unique singleton
			// to avoid penalizing unregistered verifiers.
			orgKey = fmt.Sprintf("unknown:%d:%s", i, addr)
		}
		orgSeen[orgKey] = struct{}{}
	}

	distinct := len(orgSeen)
	return uint64(distinct) * 1_000_000 / uint64(total)
}

// revealVerifiers returns the addresses of verifiers who submitted reveals in
// the given round.
func revealVerifiers(round *types.VerificationRound) []string {
	addrs := make([]string, 0, len(round.Reveals))
	for _, r := range round.Reveals {
		if r.Verifier != "" {
			addrs = append(addrs, r.Verifier)
		}
	}
	return addrs
}

// CheckPanelDiversityGate returns an error if the panel's diversity score
// falls below the configured threshold. Called from CompleteRound for
// ACCEPT verdicts.
//
// Gate is fail-open:
//   - min_panel_diversity_bps = 0 → gate disabled
//   - diversity_score >= threshold → pass
//   - diversity_score < threshold AND threshold > 0 → INCONCLUSIVE
func (k Keeper) CheckPanelDiversityGate(diversityScore uint64, params types.Params) error {
	threshold := params.MinPanelDiversityBps
	if threshold == 0 {
		return nil // disabled
	}
	if diversityScore >= threshold {
		return nil
	}
	return fmt.Errorf(
		"panel diversity score %d bps < required %d bps: too many verifiers from the same organization; "+
			"a new round with a more organizationally diverse panel is required",
		diversityScore, threshold,
	)
}

// SetVerifierOrganization stores a verifier's organizational affiliation in
// their AgentCalibration record. Creates the record if it does not exist.
func (k Keeper) SetVerifierOrganization(ctx context.Context, verifierAddr, organizationId string) error {
	cal := k.getOrInitCalibration(ctx, verifierAddr)
	cal.OrganizationId = organizationId
	return k.SetAgentCalibration(ctx, cal)
}

// MsgSetVerifierOrganization handler — self-declaration of organizational affiliation.
func (m *msgServer) SetVerifierOrganization(ctx context.Context, msg *types.MsgSetVerifierOrganization) (*types.MsgSetVerifierOrganizationResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if msg.VerifierAddress == "" {
		return nil, fmt.Errorf("verifier_address required")
	}
	if msg.Sender == "" {
		return nil, fmt.Errorf("sender required")
	}

	// Authorization: either self-declaration (sender == verifier) or authority override.
	isAuthority := msg.Sender == m.keeper.GetAuthority()
	isSelf := msg.Sender == msg.VerifierAddress

	if !isSelf && !isAuthority {
		return nil, fmt.Errorf(
			"unauthorized: %s may only set their own organization_id; "+
				"to set another verifier's affiliation use the governance authority address",
			msg.Sender,
		)
	}

	// Authority overrides require a rationale for audit purposes.
	if isAuthority && !isSelf && msg.Rationale == "" {
		return nil, fmt.Errorf("rationale required for authority override of another verifier's organization_id")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := m.keeper.SetVerifierOrganization(ctx, msg.VerifierAddress, msg.OrganizationId); err != nil {
		return nil, fmt.Errorf("failed to store organization_id: %w", err)
	}

	action := "self-declared"
	if isAuthority && !isSelf {
		action = "authority-override"
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.verifier_organization_set",
		sdk.NewAttribute("verifier", msg.VerifierAddress),
		sdk.NewAttribute("organization_id", msg.OrganizationId),
		sdk.NewAttribute("action", action),
		sdk.NewAttribute("sender", msg.Sender),
	))

	return &types.MsgSetVerifierOrganizationResponse{
		VerifierAddress: msg.VerifierAddress,
		OrganizationId:  msg.OrganizationId,
	}, nil
}

// ─── Structured Deliberation Gap 4: Polis Bridging ───────────────────────────
//
// Polis (pol.is, Audrey Tang / vTaiwan): cross-organizational consensus is
// stronger epistemic signal than within-cluster agreement. "Bridging statements"
// in Polis are those that achieve high agreement *across* ideologically distinct
// groups — not merely among a homogeneous cohort.
//
// On-chain analog: verifier organizational affiliations are the structural
// proxy for Polis clusters. A fact accepted by verifiers from N distinct
// organizations has bridged N organizational perspectives.

// RecordBridgingOrgCountForTest exports recordBridgingOrgCount for white-box unit tests.
func RecordBridgingOrgCountForTest(k Keeper, ctx context.Context, factId string, round *types.VerificationRound) {
	k.recordBridgingOrgCount(ctx, factId, round)
}

// ComputeBridgingOrgCount returns the number of distinct organizations among
// verifiers who voted "accept" in the given round.
//
// Algorithm (mirrors ComputePanelDiversityScore but restricted to ACCEPT voters):
//   - Skips reveals with Vote != "accept" or empty Verifier
//   - Empty organization_id → treated as unique singleton (distinct from all)
//   - Returns 0 when no verifiers voted "accept"
func (k Keeper) ComputeBridgingOrgCount(ctx context.Context, round *types.VerificationRound) uint64 {
	orgSeen := make(map[string]struct{})
	singletonIdx := 0
	for _, r := range round.Reveals {
		if r.Verifier == "" || r.Vote != "accept" {
			continue
		}
		cal, found := k.GetAgentCalibration(ctx, r.Verifier)
		var orgKey string
		if found && cal.OrganizationId != "" {
			orgKey = "org:" + cal.OrganizationId
		} else {
			// Unknown affiliation: treat as unique singleton so no diversity penalty.
			orgKey = fmt.Sprintf("singleton:%d:%s", singletonIdx, r.Verifier)
			singletonIdx++
		}
		orgSeen[orgKey] = struct{}{}
	}
	return uint64(len(orgSeen))
}

// recordBridgingOrgCount computes and stores the bridging org count on the
// accepted fact identified by factId. Emits zerone.knowledge.bridging_claim_accepted
// when the count meets or exceeds Params.BridgingMinOrgs (if non-zero).
//
// Called from CompleteRound on ACCEPT verdicts only, after createFactFromClaim.
// Fail-open: any error is silently absorbed to avoid blocking round completion.
func (k Keeper) recordBridgingOrgCount(ctx context.Context, factId string, round *types.VerificationRound) {
	if factId == "" {
		return
	}
	fact, found := k.GetFact(ctx, factId)
	if !found {
		return
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	fact.BridgingOrgCount = count
	if err := k.SetFact(ctx, fact); err != nil {
		return
	}

	// Threshold-gated event emission.
	params, err := k.GetParams(ctx)
	if err != nil || params.BridgingMinOrgs == 0 || count < params.BridgingMinOrgs {
		return
	}

	// Count total accept voters for the event attribute.
	acceptCount := uint64(0)
	for _, r := range round.Reveals {
		if r.Vote == "accept" {
			acceptCount++
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.bridging_claim_accepted",
		sdk.NewAttribute("fact_id", factId),
		sdk.NewAttribute("claim_id", round.ClaimId),
		sdk.NewAttribute("bridging_org_count", fmt.Sprintf("%d", count)),
		sdk.NewAttribute("accept_voter_count", fmt.Sprintf("%d", acceptCount)),
	))
}
