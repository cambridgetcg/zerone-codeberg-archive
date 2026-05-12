package keeper

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// Module account names for fee routing.
const (
	protocolTreasuryModule = "protocol_treasury"
	developmentFundModule  = "development_fund"
)

// distributeReviewFee distributes a non-refundable review fee using the governance-configured
// revenue split from params. The split is stored as BPS values (1,000,000 = 100%):
//
//	params.VerifierPoolBps      → held in knowledge module account, paid to verifiers on round completion
//	params.ProtocolTreasuryBps  → protocol_treasury module account
//	params.DevelopmentFundBps   → development_fund module account
//	remainder                   → research fund (absorbs rounding dust)
//
// Default split: 55% / 22% / 19.67% / ~3.33%.
func (k Keeper) distributeReviewFee(ctx context.Context, feeAmount uint64, params *types.Params) error {
	if k.bankKeeper == nil || feeAmount == 0 {
		return nil
	}

	verifierPool := safeMulDiv(feeAmount, params.VerifierPoolBps, 1_000_000)
	protocolAmt := safeMulDiv(feeAmount, params.ProtocolTreasuryBps, 1_000_000)
	devAmt := safeMulDiv(feeAmount, params.DevelopmentFundBps, 1_000_000)
	researchAmt := feeAmount - verifierPool - protocolAmt - devAmt // remainder absorbs rounding dust

	// verifierPool (55%) stays in knowledge module account — distributed to verifiers on round completion.

	// Send protocol share to treasury.
	if protocolAmt > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(new(big.Int).SetUint64(protocolAmt))))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, protocolTreasuryModule, coins); err != nil {
			return fmt.Errorf("review fee → protocol treasury: %w", err)
		}
	}

	// Send development share.
	if devAmt > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(new(big.Int).SetUint64(devAmt))))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, developmentFundModule, coins); err != nil {
			return fmt.Errorf("review fee → development fund: %w", err)
		}
	}

	// Send research share via canonical depositor (handles founder split).
	if researchAmt > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(new(big.Int).SetUint64(researchAmt))))
		if k.vestingRewardsKeeper != nil {
			if err := k.vestingRewardsKeeper.DepositToResearchFund(ctx, types.ModuleName, coins); err != nil {
				return fmt.Errorf("review fee → research fund: %w", err)
			}
		} else {
			// Fallback: send directly to research_fund module account.
			if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "research_fund", coins); err != nil {
				return fmt.Errorf("review fee → research fund (fallback): %w", err)
			}
		}
	}

	return nil
}

// verifierPoolFromFee calculates the verifier reward pool for a given fee amount,
// using the governance-configured split from params.
func verifierPoolFromFee(feeAmount uint64, params *types.Params) uint64 {
	return safeMulDiv(feeAmount, params.VerifierPoolBps, 1_000_000)
}

// validateAndPayFromBootstrapFund validates sponsorship eligibility and pays the review fee
// from the bootstrap fund module account instead of the submitter.
func (k Keeper) validateAndPayFromBootstrapFund(ctx context.Context, submitter string, stakeAmt *big.Int, feeCoins sdk.Coins, params *types.Params) error {
	// Check fund is enabled
	if !params.BootstrapFundEnabled {
		return fmt.Errorf("bootstrap fund sponsorship is disabled")
	}

	// Check fee cap
	feeCap, _ := new(big.Int).SetString(params.BootstrapFundFeeCap, 10)
	if feeCap != nil && stakeAmt.Cmp(feeCap) > 0 {
		return fmt.Errorf("review fee %s exceeds bootstrap fund cap %s", stakeAmt.String(), params.BootstrapFundFeeCap)
	}

	// Check per-address lifetime limit
	addressCount := k.GetBootstrapClaimCount(ctx, submitter)
	maxPerAddr, _ := strconv.ParseUint(params.BootstrapFundMaxPerAddress, 10, 64)
	if addressCount >= maxPerAddr {
		return fmt.Errorf("address has used all %d bootstrap fund claims", maxPerAddr)
	}

	// Check per-epoch rate limit
	epoch := k.CurrentEpoch(ctx)
	epochCount := k.GetBootstrapEpochCount(ctx, epoch)
	maxPerEpoch, _ := strconv.ParseUint(params.BootstrapFundMaxPerEpoch, 10, 64)
	if epochCount >= maxPerEpoch {
		return fmt.Errorf("bootstrap fund epoch limit reached (%d/%d)", epochCount, maxPerEpoch)
	}

	// Check fund has sufficient balance
	fundBalance := k.GetBootstrapFundBalance(ctx)
	if fundBalance.Amount.LT(sdkmath.NewIntFromBigInt(stakeAmt)) {
		return fmt.Errorf("bootstrap fund insufficient: has %s, need %s", fundBalance.Amount, stakeAmt)
	}

	// Pay fee from bootstrap fund → knowledge module
	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.BootstrapFundModuleName, types.ModuleName, feeCoins); err != nil {
		return fmt.Errorf("failed to draw from bootstrap fund: %w", err)
	}

	// Track usage
	_ = k.IncrementBootstrapClaimCount(ctx, submitter)
	_ = k.IncrementBootstrapEpochCount(ctx, epoch)

	return nil
}
