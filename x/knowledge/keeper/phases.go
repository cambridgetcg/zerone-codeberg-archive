package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// BeginBlocker runs knowledge module begin-block logic.
// Advances verification round phases by deadline and triggers fitness epoch updates.
func (k Keeper) BeginBlocker(ctx context.Context) error {
	// Route B Wave 8: the heartbeat. Self-maintenance of the training
	// infrastructure — bounty expiry / vesting release / manifest
	// supersession. Runs first so a round-advance error cannot silently
	// starve Route B lifecycle progression. Non-fatal: errors logged inside.
	k.ProcessRouteBLifecycle(ctx)

	if err := k.AdvanceRoundPhases(ctx); err != nil {
		return err
	}

	// Check if we're at a fitness epoch boundary
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	params, err := k.GetParams(ctx)
	if err != nil {
		return nil // non-fatal: don't block consensus for param read failure
	}
	// Prune expired vindication entries every 1000 blocks
	if height > 0 && height%1000 == 0 && params.VindicationRefundEnabled {
		k.PruneExpiredVindications(ctx, height, params.VindicationWindowBlocks)
	}

	if params.FitnessEpochBlocks > 0 && height > 0 && height%params.FitnessEpochBlocks == 0 {
		epoch := height / params.FitnessEpochBlocks

		// Order matters:
		// 1. Update fitness scores (current usage data)
		if err := k.UpdateAllFitnessScores(ctx); err != nil {
			k.Logger(ctx).Error("fitness update failed", "error", err)
		}
		// 2. Process competition (uses fitness to rank niches)
		if err := k.ProcessCompetition(ctx, epoch); err != nil {
			k.Logger(ctx).Error("competition processing failed", "epoch", epoch, "error", err)
		}
		// 3. Process symbiosis (adjusts fitness based on relationships)
		k.ProcessSymbiosis(ctx, params)
		// 4. Process metabolism (uses fitness + competition tax to drain/replenish energy)
		if err := k.ProcessMetabolism(ctx, epoch); err != nil {
			k.Logger(ctx).Error("metabolism processing failed", "epoch", epoch, "error", err)
		}
		// 5. Process agent demand bounties
		if err := k.ProcessDemandBounties(ctx, epoch); err != nil {
			k.Logger(ctx).Error("demand bounty processing failed", "epoch", epoch, "error", err)
		}
		// 6. Clean up expired bounties
		k.ProcessExpiredBounties(ctx)
		// 7. Clear query receipts (bound receipt storage to one epoch)
		k.ClearQueryReceipts(ctx)
		// 8. Aggregate diversity metrics and check conformity alerts (R28-2)
		if err := k.ProcessDiversity(ctx, epoch); err != nil {
			k.Logger(ctx).Error("diversity processing failed", "epoch", epoch, "error", err)
		}
		// 9. Update epistemic temperature for all domains (R29-2)
		k.IterateDomains(ctx, func(domain *types.Domain) bool {
			if dErr := k.UpdateEpistemicTemperature(ctx, domain.Name); dErr != nil {
				k.Logger(ctx).Error("epistemic temperature update failed", "domain", domain.Name, "error", dErr)
			}
			return false
		})
		// 10. Decay domain role elasticity records (R29-3)
		if params.RoleElasticityDecayEpochs > 0 && epoch%params.RoleElasticityDecayEpochs == 0 {
			k.DecayRoleRecords(ctx)
		}
	}

	// Advance fact confidence at ConfidenceGrowthEpoch intervals (R29-2)
	if params.ConfidenceGrowthEpoch > 0 && height > 0 && height%params.ConfidenceGrowthEpoch == 0 {
		if err := k.AdvanceConfidence(ctx); err != nil {
			k.Logger(ctx).Error("confidence growth failed", "error", err)
		}
	}

	// Wave 15: stress-test invitation heartbeat. Chain-driven demand for
	// probes on idle high-confidence facts. Bounded to ProbeInvitationBatchSize
	// per block so heartbeat cost stays O(constant) regardless of fact count.
	if params.ProbeInvitationIdleThresholdBlocks > 0 {
		k.InviteIdleFactsForProbing(ctx, height, params)
	}

	// Wave 15: probe bounty pool mint. Per-block issuance into the pool
	// that funds successful-probe bonuses, capped at ProbeBountyMaxPoolSize
	// so minting throttles naturally.
	if params.ProbeBountyMintPerBlock != "" && params.ProbeBountyMintPerBlock != "0" {
		k.MintToProbeBountyPool(ctx, params)
	}

	// Wave 16: materialize pending fact injections whose guardian-veto
	// window has expired without a veto. Until materialization the fact
	// does not exist in state — guardians can still cancel.
	k.MaterializeMaturedFactInjections(ctx, height)

	// Scientific Method Gap 3: process expired pre-registrations.
	// Bond forfeiture to research_fund; marks expired records so they
	// cannot be fulfilled. Non-fatal — never blocks consensus.
	k.ProcessExpiredPreRegistrations(ctx)

	return nil
}

// AdvanceRoundPhases iterates all active rounds and transitions phases by deadline.
func (k Keeper) AdvanceRoundPhases(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Collect rounds to process (avoid modifying store during iteration)
	var roundsToProcess []*types.VerificationRound
	k.IterateActiveRounds(ctx, func(round *types.VerificationRound) bool {
		roundsToProcess = append(roundsToProcess, round)
		return false
	})

	for _, round := range roundsToProcess {
		expectedPhase := GetExpectedPhase(round, height, params)

		if expectedPhase == round.Phase {
			continue // no transition needed
		}

		switch expectedPhase {
		case types.VerificationPhase_VERIFICATION_PHASE_REVEAL:
			// COMMIT → REVEAL transition
			round.Phase = types.VerificationPhase_VERIFICATION_PHASE_REVEAL
			if err := k.SetVerificationRound(ctx, round); err != nil {
				k.Logger(ctx).Error("failed to transition round to REVEAL", "round_id", round.Id, "error", err)
			}
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.round_phase_changed",
				sdk.NewAttribute("round_id", round.Id),
				sdk.NewAttribute("phase", "REVEAL"),
			))

		case types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION:
			// REVEAL → AGGREGATION transition
			round.Phase = types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION
			if err := k.SetVerificationRound(ctx, round); err != nil {
				k.Logger(ctx).Error("failed to transition round to AGGREGATION", "round_id", round.Id, "error", err)
				continue
			}
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.round_phase_changed",
				sdk.NewAttribute("round_id", round.Id),
				sdk.NewAttribute("phase", "AGGREGATION"),
				sdk.NewAttribute("reveal_count", fmt.Sprintf("%d", len(round.Reveals))),
			))
			// Perform aggregation immediately
			if err := k.performAggregation(ctx, round); err != nil {
				k.Logger(ctx).Error("aggregation failed", "round_id", round.Id, "error", err)
			}

		case types.VerificationPhase_VERIFICATION_PHASE_EXPIRED:
			// Round has expired — check if we can still aggregate
			if uint64(len(round.Reveals)) >= params.MinVerifiers {
				// Enough reveals — aggregate
				round.Phase = types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION
				if err := k.SetVerificationRound(ctx, round); err != nil {
					continue
				}
				if err := k.performAggregation(ctx, round); err != nil {
					k.Logger(ctx).Error("late aggregation failed", "round_id", round.Id, "error", err)
				}
			} else {
				// Insufficient reveals — mark as expired
				round.Phase = types.VerificationPhase_VERIFICATION_PHASE_EXPIRED
				round.Verdict = types.Verdict_VERDICT_INCONCLUSIVE
				if err := k.SetVerificationRound(ctx, round); err != nil {
					continue
				}
				// Review fee is non-refundable — mark claim as insufficient
				claim, found := k.GetClaim(ctx, round.ClaimId)
				if found {
					claim.Status = types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT
					// Pragmatism Gap 2: INCONCLUSIVE as epistemic difficulty signal.
					// Procedural inconclusiveness (insufficient reveals) signals the
					// same thing as panel-level inconclusiveness: this claim resists
					// resolution. Track it alongside CompleteRound's INCONCLUSIVE path.
					claim.InconclusiveCount++
					if claim.FirstInconclusiveAtBlock == 0 {
						claim.FirstInconclusiveAtBlock = uint64(sdkCtx.BlockHeight())
					}
					_ = k.SetClaim(ctx, claim)
				}

				// P-24b: Decrement the active-rounds counter.
				// CompleteRound normally does this; the insufficient-reveals path
				// bypasses CompleteRound, so the counter must be decremented here.
				// Without this, every abandoned round permanently inflates
				// ActiveRoundsCount, mis-signalling congestion and triggering
				// premature fee backpressure across the entire knowledge market.
				k.DecrementActiveRoundsCount(ctx)

				// Slash committed-but-not-revealed verifiers for MissedRevealSlashBps.
				// They made a commitment (creating an obligation to reveal) but the
				// round failed to gather enough reveals. The same penalty applies here
				// as in the normal aggregation path — abandoning a commitment degrades
				// the verifier pool's reliability regardless of whether the round
				// ultimately reaches quorum.
				if k.stakingKeeper != nil && params.MissedRevealSlashBps > 0 && len(round.Commits) > 0 {
					revealSet := make(map[string]struct{}, len(round.Reveals))
					for _, rev := range round.Reveals {
						revealSet[rev.Verifier] = struct{}{}
					}
					for _, commit := range round.Commits {
						if _, revealed := revealSet[commit.Verifier]; !revealed {
							_ = k.stakingKeeper.SlashValidator(ctx, commit.Verifier, params.MissedRevealSlashBps)
						}
					}
				}

				sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
					"zerone.knowledge.round_expired",
					sdk.NewAttribute("round_id", round.Id),
					sdk.NewAttribute("reveals", fmt.Sprintf("%d", len(round.Reveals))),
					sdk.NewAttribute("commits", fmt.Sprintf("%d", len(round.Commits))),
				))
			}
		}
	}

	return nil
}

// GetExpectedPhase is a pure function that maps a block height to the expected phase.
func GetExpectedPhase(round *types.VerificationRound, height uint64, params *types.Params) types.VerificationPhase {
	if round.Phase == types.VerificationPhase_VERIFICATION_PHASE_COMPLETE ||
		round.Phase == types.VerificationPhase_VERIFICATION_PHASE_EXPIRED {
		return round.Phase
	}

	if height >= round.AggregationDeadline {
		return types.VerificationPhase_VERIFICATION_PHASE_EXPIRED
	}
	if height >= round.RevealDeadline {
		return types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION
	}
	if height >= round.CommitDeadline {
		return types.VerificationPhase_VERIFICATION_PHASE_REVEAL
	}
	return types.VerificationPhase_VERIFICATION_PHASE_COMMIT
}

// performAggregation aggregates votes and completes the round.
func (k Keeper) performAggregation(ctx context.Context, round *types.VerificationRound) error {
	result, err := k.AggregateVerificationResult(ctx, round)
	if err != nil {
		return err
	}
	return k.CompleteRound(ctx, round, result)
}
