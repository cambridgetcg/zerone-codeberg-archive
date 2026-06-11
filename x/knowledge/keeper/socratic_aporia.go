package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Socratic Dialectic Gap 2: Aporia ────────────────────────────────────────
//
// Aporia (Greek ἀπορία — "without passage") is the Socratic condition of
// productive perplexity: the moment when inquiry has genuinely exhausted its
// current instruments and a claim stands as a named site of difficulty rather
// than mere ignorance. In the Theaetetus (210b), Socrates acknowledges that
// not-knowing-what-knowledge-is is the beginning of understanding, not its
// failure.
//
// A claim in aporia (in_aporia = true) is distinct from INSUFFICIENT:
//   - INSUFFICIENT: a single round failed to reach quorum.
//   - APORIA: the claim has now resisted resolution N times (≥ aporia_threshold)
//     — the community of inquiry has genuinely been stumped.
//
// The chain marks aporia so that future submitters and verifiers can attend to
// it with fresh instruments: better evidence, finer methodology, or a reformed
// question frame. It is a signal to epistemically interesting territory, not a
// condemnation.
//
// Wiring: called from both INCONCLUSIVE increment paths:
//   1. CompleteRound (panel INCONCLUSIVE verdict) — rounds.go
//   2. AdvanceRoundPhases insufficient-reveals expiry — phases.go
//
// In both paths the claim is SetClaim'd immediately after, so aporia fields
// are persisted atomically with the inconclusive count update.

// maybeMarkAporia checks whether the claim has reached the aporia threshold
// and, if so, marks it in_aporia and emits zerone.knowledge.claim_in_aporia.
//
// Called after claim.InconclusiveCount has already been incremented — so the
// check `>= threshold` catches the moment of crossing, not the state before.
//
// Idempotent: no-op if claim.InAporia is already true.
// Fail-open: if params cannot be fetched, silently skips.
func (k Keeper) maybeMarkAporia(ctx context.Context, claim *types.Claim) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return
	}
	if params.AporiaThreshold == 0 {
		return // disabled — backward-compatible default
	}
	if claim.InAporia {
		return // already in aporia — idempotent
	}
	if claim.InconclusiveCount < params.AporiaThreshold {
		return // threshold not yet reached
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	claim.InAporia = true
	claim.AporiaReachedAtBlock = uint64(sdkCtx.BlockHeight())

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.claim_in_aporia",
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("inconclusive_count", fmt.Sprintf("%d", claim.InconclusiveCount)),
		sdk.NewAttribute("aporia_threshold", fmt.Sprintf("%d", params.AporiaThreshold)),
	))
}

// MaybeMarkAporiaForTest exports maybeMarkAporia for white-box unit tests.
func (k Keeper) MaybeMarkAporiaForTest(ctx context.Context, claim *types.Claim) {
	k.maybeMarkAporia(ctx, claim)
}
