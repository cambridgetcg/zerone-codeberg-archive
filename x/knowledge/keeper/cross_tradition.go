package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Contemplative Traditions Gap 4: Cross-Tradition Convergence ─────────────
//
// Independent confirmation from practitioners across lineages is treated as
// stronger evidence than same-tradition consensus — the replication logic from
// Scientific Method Gap 1 (panel_diversity_score_bps) applied to the
// contemplative domain.
//
// Score = distinct_tradition_count / total_verifiers × 1_000_000
// Empty/unknown tradition → unique singleton (fail-open: never penalises
// verifiers who have not declared a tradition).
//
// Gate: M-PHENOMENOLOGICAL claims only. Params.min_cross_tradition_diversity_bps
// > 0 → ACCEPT becomes INCONCLUSIVE when score < threshold.

// ComputeCrossTraditionScore returns the panel cross-tradition diversity score
// in BPS (1_000_000 = one distinct tradition per verifier).
//
// Empty tradition → treated as unique singleton so unregistered verifiers are
// never penalised.
func (k Keeper) ComputeCrossTraditionScore(ctx context.Context, round *types.VerificationRound) uint64 {
	verifiers := revealVerifiers(round) // reuse from panel_diversity.go
	total := len(verifiers)
	if total == 0 {
		return 1_000_000 // fail-open: no verifiers = full diversity
	}

	tradSeen := make(map[string]struct{}, total)
	for i, addr := range verifiers {
		cal, found := k.GetAgentCalibration(ctx, addr)
		var tradKey string
		if found && cal.ContemplativeTradition != "" {
			tradKey = "trad:" + cal.ContemplativeTradition
		} else {
			// Unknown tradition: treat as own unique singleton so verifiers
			// without declared traditions never reduce the diversity score.
			tradKey = fmt.Sprintf("unknown:%d:%s", i, addr)
		}
		tradSeen[tradKey] = struct{}{}
	}

	distinct := len(tradSeen)
	return uint64(distinct) * 1_000_000 / uint64(total)
}

// CheckCrossTraditionGate returns an error if the cross-tradition score falls
// below the configured threshold on an M-PHENOMENOLOGICAL claim.
//
// Gate is fail-open:
//   - min_cross_tradition_diversity_bps = 0 → disabled
//   - non-PHENOMENOLOGICAL claim         → skip
//   - score >= threshold                 → pass
//   - score < threshold && threshold > 0 → INCONCLUSIVE
func (k Keeper) CheckCrossTraditionGate(ctx context.Context, claim *types.Claim, crossTradScore uint64, params types.Params) error {
	threshold := params.MinCrossTraditionDiversityBps
	if threshold == 0 {
		return nil // disabled
	}
	// Only M-PHENOMENOLOGICAL claims are gated.
	if claim == nil || claim.MethodId != "M-PHENOMENOLOGICAL" {
		return nil
	}
	if crossTradScore >= threshold {
		return nil
	}
	return fmt.Errorf(
		"cross-tradition score %d bps < required %d bps for M-PHENOMENOLOGICAL claim: "+
			"panel lacks sufficient cross-lineage diversity; a round with verifiers from "+
			"more distinct contemplative traditions is required",
		crossTradScore, threshold,
	)
}

// SetContemplativeTradition stores a verifier's tradition declaration in their
// AgentCalibration record. Creates the record if it does not exist.
func (k Keeper) SetContemplativeTradition(ctx context.Context, verifierAddr, tradition string) error {
	cal := k.getOrInitCalibration(ctx, verifierAddr)
	cal.ContemplativeTradition = tradition
	return k.SetAgentCalibration(ctx, cal)
}

// MsgSetContemplativeTradition handler — self-declaration or governance override.
func (m *msgServer) SetContemplativeTradition(ctx context.Context, msg *types.MsgSetContemplativeTradition) (*types.MsgSetContemplativeTraditionResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if msg.VerifierAddress == "" {
		return nil, fmt.Errorf("verifier_address required")
	}
	if msg.Sender == "" {
		return nil, fmt.Errorf("sender required")
	}
	if msg.Tradition == "" {
		return nil, fmt.Errorf("tradition required")
	}

	isAuthority := msg.Sender == m.keeper.GetAuthority()
	isSelf := msg.Sender == msg.VerifierAddress

	if !isSelf && !isAuthority {
		return nil, fmt.Errorf(
			"unauthorized: %s may only set their own contemplative_tradition; "+
				"to set another verifier's tradition use the governance authority address",
			msg.Sender,
		)
	}

	if isAuthority && !isSelf && msg.Rationale == "" {
		return nil, fmt.Errorf("rationale required for authority override of another verifier's contemplative_tradition")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := m.keeper.SetContemplativeTradition(ctx, msg.VerifierAddress, msg.Tradition); err != nil {
		return nil, fmt.Errorf("failed to store contemplative_tradition: %w", err)
	}

	action := "self-declared"
	if isAuthority && !isSelf {
		action = "authority-override"
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.contemplative_tradition_set",
		sdk.NewAttribute("verifier", msg.VerifierAddress),
		sdk.NewAttribute("tradition", msg.Tradition),
		sdk.NewAttribute("action", action),
		sdk.NewAttribute("sender", msg.Sender),
	))

	return &types.MsgSetContemplativeTraditionResponse{
		VerifierAddress: msg.VerifierAddress,
		Tradition:       msg.Tradition,
	}, nil
}

// ComputeCrossTraditionScoreForTest exports ComputeCrossTraditionScore for
// white-box tests.
func (k Keeper) ComputeCrossTraditionScoreForTest(ctx context.Context, round *types.VerificationRound) uint64 {
	return k.ComputeCrossTraditionScore(ctx, round)
}
