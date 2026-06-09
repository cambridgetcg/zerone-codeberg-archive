package keeper

import (
	"context"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// CheckMethodologyTemporalGate checks whether a verification round has run long
// enough for the claim's methodology before allowing an ACCEPT verdict.
//
// Currently enforces one methodology-specific window:
//   - M-PHENOMENOLOGICAL: Params.MinPhenomenologicalWindowBlocks
//
// The gate fires if:
//   - The relevant param > 0 (gate is enabled for this methodology)
//   - The claim uses that methodology
//   - elapsed = verdictBlock - round.StartedAtBlock < param
//
// Returns nil (gate passes) when:
//   - param == 0 (gate disabled)
//   - claim uses a different methodology
//   - elapsed >= param
//
// Intent: contemplative-traditions brief temporal verification requirement.
// Claims about states requiring sustained observation (phenomenological realizations,
// meditative states, attentional training effects) cannot be validly assessed in a
// flash panel. The minimum window ensures the verification round spans enough
// chain time to constitute a genuine temporal observation.
func (k Keeper) CheckMethodologyTemporalGate(
	ctx context.Context,
	claim types.Claim,
	round types.VerificationRound,
	params types.Params,
	verdictBlock uint64,
) error {
	// M-PHENOMENOLOGICAL temporal window gate.
	if claim.MethodId == types.MethodologyPhenomenologic {
		minWindow := params.MinPhenomenologicalWindowBlocks
		if minWindow > 0 {
			if verdictBlock < round.StartedAtBlock {
				// Defensive: should not happen, but treat as zero elapsed.
				return types.ErrVerificationWindowTooShort.Wrapf(
					"M-PHENOMENOLOGICAL claim %q: verdict block %d predates round start %d",
					claim.Id, verdictBlock, round.StartedAtBlock,
				)
			}
			elapsed := verdictBlock - round.StartedAtBlock
			if elapsed < minWindow {
				return types.ErrVerificationWindowTooShort.Wrapf(
					"M-PHENOMENOLOGICAL claim %q: round elapsed %d blocks, need %d",
					claim.Id, elapsed, minWindow,
				)
			}
		}
	}
	return nil
}

// CheckMethodologyTemporalGateForTest is an exported wrapper for use in unit tests.
func CheckMethodologyTemporalGateForTest(
	k Keeper,
	ctx context.Context,
	claim types.Claim,
	round types.VerificationRound,
	params types.Params,
	verdictBlock uint64,
) error {
	return k.CheckMethodologyTemporalGate(ctx, claim, round, params, verdictBlock)
}
