package keeper

import (
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/partnerships/types"
)

const (
	exitInitiatorForfeitBps    = 7700
	exitNonInitiatorForfeitBps = 5500
)

var exitBpsDenom = big.NewInt(10000)

// CalculateExitSettlement computes payouts when a partnership dissolves.
func CalculateExitSettlement(
	potBalance *big.Int,
	lockTier uint32,
	initiator string,
	humanAddr string,
) (humanPayout, agentPayout, burned *big.Int) {
	if potBalance.Sign() <= 0 {
		return big.NewInt(0), big.NewInt(0), big.NewInt(0)
	}

	halfPot := new(big.Int).Div(potBalance, big.NewInt(2))

	tier := lockTier
	if tier > 5 {
		tier = 5
	}
	tierPenaltyBps := new(big.Int).SetUint64(types.LockTiers[tier].ExitPenaltyBps)
	tierDenom := big.NewInt(1000000)

	isHumanInitiator := initiator == humanAddr

	initiatorHalf := new(big.Int).Set(halfPot)
	nonInitiatorHalf := new(big.Int).Set(halfPot)

	// Initiator penalty: tier penalty * 77% forfeiture
	initiatorPenalty := new(big.Int).Mul(initiatorHalf, tierPenaltyBps)
	initiatorPenalty.Div(initiatorPenalty, tierDenom)
	initiatorForfeited := new(big.Int).Mul(initiatorPenalty, big.NewInt(exitInitiatorForfeitBps))
	initiatorForfeited.Div(initiatorForfeited, exitBpsDenom)
	initiatorPay := new(big.Int).Sub(initiatorHalf, initiatorForfeited)

	// Non-initiator penalty: tier penalty * 55% forfeiture
	nonInitiatorPenalty := new(big.Int).Mul(nonInitiatorHalf, tierPenaltyBps)
	nonInitiatorPenalty.Div(nonInitiatorPenalty, tierDenom)
	nonInitiatorForfeited := new(big.Int).Mul(nonInitiatorPenalty, big.NewInt(exitNonInitiatorForfeitBps))
	nonInitiatorForfeited.Div(nonInitiatorForfeited, exitBpsDenom)
	nonInitiatorPay := new(big.Int).Sub(nonInitiatorHalf, nonInitiatorForfeited)

	if initiatorPay.Sign() < 0 {
		initiatorPay = big.NewInt(0)
	}
	if nonInitiatorPay.Sign() < 0 {
		nonInitiatorPay = big.NewInt(0)
	}

	totalBurned := new(big.Int).Add(initiatorForfeited, nonInitiatorForfeited)

	if isHumanInitiator {
		return initiatorPay, nonInitiatorPay, totalBurned
	}
	return nonInitiatorPay, initiatorPay, totalBurned
}

// HandleExit processes a partnership exit/dissolution.
func (k Keeper) HandleExit(ctx sdk.Context, partnershipId string, initiator string) (uint64, error) {
	params := k.GetParams(ctx)
	currentBlock := uint64(ctx.BlockHeight())

	partnership, found := k.GetPartnership(ctx, partnershipId)
	if !found {
		return 0, types.ErrPartnershipNotFound
	}

	if partnership.Status == types.StatusDissolved {
		return 0, types.ErrAlreadyDissolved
	}
	if partnership.Status == types.StatusCooling {
		return 0, types.ErrExitInProgress
	}

	// Check lock expiry: must be expired OR partnership must be frozen/suspended
	if partnership.Status == types.StatusActive && partnership.LockExpiresAt > currentBlock {
		return 0, types.ErrLockNotExpired
	}

	potBalance := new(big.Int)
	if partnership.CommonPotBalance != "" {
		potBalance.SetString(partnership.CommonPotBalance, 10)
	}

	humanPayout, agentPayout, burned := CalculateExitSettlement(
		potBalance,
		partnership.LockTier,
		initiator,
		partnership.HumanAddr,
	)

	// Execute bank transfers
	if humanPayout.Sign() > 0 {
		humanAddr, err := sdk.AccAddressFromBech32(partnership.HumanAddr)
		if err == nil {
			coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(humanPayout)))
			if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, humanAddr, coins); err != nil {
				k.Logger(ctx).Error("failed to send human exit payout", "err", err)
			}
		}
	}
	if agentPayout.Sign() > 0 {
		agentAddr, err := sdk.AccAddressFromBech32(partnership.AgentAddr)
		if err == nil {
			coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(agentPayout)))
			if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, agentAddr, coins); err != nil {
				k.Logger(ctx).Error("failed to send agent exit payout", "err", err)
			}
		}
	}
	if burned.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(burned)))
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
			k.Logger(ctx).Error("failed to burn forfeited exit amount", "err", err)
		}
	}

	cooldownEnd := currentBlock + params.CoolingPeriodBlocks
	partnership.Status = types.StatusCooling
	partnership.ExitState = &types.ExitState{
		InitiatedBy: initiator,
		InitiatedAt: currentBlock,
		CooldownEnd: cooldownEnd,
	}
	partnership.CommonPotBalance = "0"
	k.SetPartnership(ctx, partnership)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.partnerships.exit_initiated",
			sdk.NewAttribute("partnership_id", partnershipId),
			sdk.NewAttribute("initiator", initiator),
			sdk.NewAttribute("cooldown_ends_at", fmt.Sprintf("%d", cooldownEnd)),
			sdk.NewAttribute("human_payout", humanPayout.String()),
			sdk.NewAttribute("agent_payout", agentPayout.String()),
			sdk.NewAttribute("burned", burned.String()),
		),
	)

	return cooldownEnd, nil
}

// SettleCoolingPartnerships dissolves partnerships that have completed their cooling period.
// Emits social_benefit_lost/achieved events when dissolution changes domain status (R31-5).
func (k Keeper) SettleCoolingPartnerships(ctx sdk.Context) {
	currentBlock := uint64(ctx.BlockHeight())

	// R31-5: Snapshot pre-dissolution social benefit status for affected domains.
	affectedDomains := make(map[string]bool)

	k.IteratePartnerships(ctx, func(p *types.Partnership) bool {
		if p.Status == types.StatusCooling && p.ExitState != nil && p.ExitState.CooldownEnd <= currentBlock {
			for _, addr := range []string{p.HumanAddr, p.AgentAddr} {
				for _, m := range k.GetMentorshipsByMentor(ctx, addr) {
					affectedDomains[m.Domain] = true
				}
				for _, m := range k.GetMentorshipsByMentee(ctx, addr) {
					affectedDomains[m.Domain] = true
				}
			}
		}
		return false
	})

	preBenefitStatus := make(map[string]bool)
	for domain := range affectedDomains {
		preBenefitStatus[domain] = k.GetDomainSocialBenefitStatus(ctx, domain)
	}

	// Dissolve partnerships.
	k.IteratePartnerships(ctx, func(p *types.Partnership) bool {
		if p.Status == types.StatusCooling && p.ExitState != nil && p.ExitState.CooldownEnd <= currentBlock {
			p.Status = types.StatusDissolved
			k.SetPartnership(ctx, p)
		}
		return false
	})

	// R31-5: Water → Fire — emit events when social benefit status changes.
	for domain := range affectedDomains {
		postBenefit := k.GetDomainSocialBenefitStatus(ctx, domain)
		if preBenefitStatus[domain] && !postBenefit {
			density := k.GetDomainPartnershipDensity(ctx, domain)
			params := k.GetParams(ctx)
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.partnerships.social_benefit_lost",
				sdk.NewAttribute("domain", domain),
				sdk.NewAttribute("density", fmt.Sprintf("%d", density)),
				sdk.NewAttribute("threshold", fmt.Sprintf("%d", params.SocialSaturationThreshold)),
			))
		} else if !preBenefitStatus[domain] && postBenefit {
			density := k.GetDomainPartnershipDensity(ctx, domain)
			params := k.GetParams(ctx)
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.partnerships.social_benefit_achieved",
				sdk.NewAttribute("domain", domain),
				sdk.NewAttribute("density", fmt.Sprintf("%d", density)),
				sdk.NewAttribute("threshold", fmt.Sprintf("%d", params.SocialSaturationThreshold)),
			))
		}
	}
}
