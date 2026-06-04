package keeper

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// RecordNullResultCreditForTest is an exported wrapper for unit tests.
// Production code calls maybeAwardNullResultCredit directly from rounds.go.
func (k Keeper) RecordNullResultCreditForTest(ctx context.Context, claim *types.Claim) {
	k.maybeAwardNullResultCredit(ctx, claim)
}

// maybeAwardNullResultCredit mints a small credit to the submitter of a
// REJECTED claim if and only if:
//   1. The claim had pre-declared RefutationConditions (Claim.refutation_conditions is non-empty)
//   2. NullResultCreditUzrn > 0 in governance params
//
// This operationalizes Peirce's self-correcting community (Scientific Method Gap 5):
// submitters who pre-declare what would falsify their claim and then get falsified are
// rewarded rather than penalised. The incentive corrects publication bias — it makes
// "testable and wrong" better than "untestable".
//
// The credit is minted from the knowledge module account and is subject to the chain's
// global supply cap via governance configuration of NullResultCreditUzrn. Default = 0
// (disabled) so existing economic behaviour is unchanged until governance explicitly enables.
func (k Keeper) maybeAwardNullResultCredit(ctx context.Context, claim *types.Claim) {
	if claim == nil || claim.Submitter == "" {
		return
	}
	// Only apply when RefutationConditions were declared upfront.
	if len(claim.RefutationConditions) == 0 {
		return
	}

	params, err := k.GetParams(ctx)
	if err != nil || params == nil || params.NullResultCreditUzrn == 0 {
		return
	}

	creditAmt := sdkmath.NewIntFromUint64(params.NullResultCreditUzrn)
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", creditAmt))

	// Mint into the knowledge module account then transfer to submitter.
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		k.Logger(ctx).Error("null result credit: mint failed",
			"submitter", claim.Submitter,
			"amount", creditAmt.String(),
			"err", err,
		)
		return
	}

	submitterAddr, err := sdk.AccAddressFromBech32(claim.Submitter)
	if err != nil {
		// Bech32 parse failed — burn the just-minted coins via an intentional no-op
		// (they will sit in the module account). Log and return; don't panic.
		k.Logger(ctx).Error("null result credit: invalid submitter address",
			"submitter", claim.Submitter,
			"err", err,
		)
		return
	}

	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, submitterAddr, coins); err != nil {
		k.Logger(ctx).Error("null result credit: send failed",
			"submitter", claim.Submitter,
			"amount", creditAmt.String(),
			"err", err,
		)
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.null_result_credit_awarded",
		sdk.NewAttribute("submitter", claim.Submitter),
		sdk.NewAttribute("amount_uzrn", fmt.Sprintf("%d", params.NullResultCreditUzrn)),
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("refutation_conditions_count", fmt.Sprintf("%d", len(claim.RefutationConditions))),
	))
}
