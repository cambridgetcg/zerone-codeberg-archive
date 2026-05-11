package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/billing/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// RegisterProvider registers a knowledge API provider with staked tokens.
func (k msgServer) RegisterProvider(goCtx context.Context, msg *types.MsgRegisterProvider) (*types.MsgRegisterProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	senderAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	if _, found := k.GetProvider(ctx, msg.Sender); found {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderExists, msg.Sender)
	}

	stakeAmt := new(big.Int)
	if _, ok := stakeAmt.SetString(msg.Stake, 10); !ok || stakeAmt.Sign() <= 0 {
		return nil, fmt.Errorf("%w: %s", types.ErrInvalidAmount, msg.Stake)
	}

	params := k.GetParams(ctx)
	minStake := new(big.Int)
	minStake.SetString(params.MinProviderStake, 10)
	if stakeAmt.Cmp(minStake) < 0 {
		return nil, fmt.Errorf("%w: need %s, got %s", types.ErrInsufficientStake, params.MinProviderStake, msg.Stake)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to stake: %w", err)
	}

	provider := &types.Provider{
		Address:      msg.Sender,
		Name:         msg.Name,
		Domains:      msg.Domains,
		StakeAmount:  msg.Stake,
		Active:       true,
		TotalQueries: 0,
		TotalRevenue: "0",
		RegisteredAt: uint64(ctx.BlockHeight()),
	}
	k.SetProvider(ctx, provider)

	for _, domain := range msg.Domains {
		k.SetDomainIndex(ctx, domain, msg.Sender)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.provider_registered",
			sdk.NewAttribute("address", msg.Sender),
			sdk.NewAttribute("stake", msg.Stake),
			sdk.NewAttribute("domains", fmt.Sprintf("%v", msg.Domains)),
		),
	)

	return &types.MsgRegisterProviderResponse{}, nil
}

// DeregisterProvider removes a provider and refunds their stake.
func (k msgServer) DeregisterProvider(goCtx context.Context, msg *types.MsgDeregisterProvider) (*types.MsgDeregisterProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	senderAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	provider, found := k.GetProvider(ctx, msg.Sender)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotFound, msg.Sender)
	}

	stakeAmt := new(big.Int)
	stakeAmt.SetString(provider.StakeAmount, 10)
	if stakeAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, coins); err != nil {
			return nil, fmt.Errorf("failed to refund stake: %w", err)
		}
	}

	for _, domain := range provider.Domains {
		k.DeleteDomainIndex(ctx, domain, msg.Sender)
	}

	k.DeleteProvider(ctx, msg.Sender)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.provider_deregistered",
			sdk.NewAttribute("address", msg.Sender),
			sdk.NewAttribute("stake_refunded", provider.StakeAmount),
		),
	)

	return &types.MsgDeregisterProviderResponse{}, nil
}

// QueryFact handles a single fact query: quote + pay + distribute in one tx.
func (k msgServer) QueryFact(goCtx context.Context, msg *types.MsgQueryFact) (*types.MsgQueryFactResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	callerAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	provider, found := k.GetProvider(ctx, msg.Provider)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotFound, msg.Provider)
	}
	if !provider.Active {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotActive, msg.Provider)
	}

	providerAddr, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, fmt.Errorf("invalid provider address: %w", err)
	}

	currentBlock := uint64(ctx.BlockHeight())
	factIds := []string{msg.FactId}

	// Calculate pricing
	totalPrice, breakdown := k.CalculateQueryPrice(ctx, factIds, currentBlock)
	if totalPrice.Sign() == 0 {
		return nil, types.ErrZeroPayment
	}

	// Calculate distribution
	distribution := k.CalculateDistribution(ctx, totalPrice, factIds)

	// Execute distribution atomically
	if err := k.ExecuteDistribution(ctx, callerAddr, providerAddr, distribution); err != nil {
		return nil, fmt.Errorf("failed to execute distribution: %w", err)
	}

	// Increment citation count
	if err := k.knowledgeKeeper.IncrementCitationCount(ctx, msg.FactId); err != nil {
		k.Logger(ctx).Error("failed to increment citation count", "fact_id", msg.FactId, "error", err)
	}

	// Update provider stats
	provider.TotalQueries++
	revenue := new(big.Int)
	revenue.SetString(provider.TotalRevenue, 10)
	providerShare := new(big.Int)
	providerShare.SetString(distribution.ProviderShare, 10)
	revenue.Add(revenue, providerShare)
	provider.TotalRevenue = revenue.String()
	k.SetProvider(ctx, provider)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.fact_queried",
			sdk.NewAttribute("caller", msg.Sender),
			sdk.NewAttribute("provider", msg.Provider),
			sdk.NewAttribute("fact_id", msg.FactId),
			sdk.NewAttribute("total_price", totalPrice.String()),
		),
	)

	// Build quote response
	var quote *types.QueryQuote
	if len(breakdown) > 0 {
		quote = &types.QueryQuote{
			FactId:         msg.FactId,
			BasePrice:      breakdown[0].BaseCost,
			EffectivePrice: breakdown[0].TotalPrice,
		}
	}

	return &types.MsgQueryFactResponse{Quote: quote}, nil
}

// BatchQueryFacts handles batch fact queries: quote + pay + distribute in one tx.
func (k msgServer) BatchQueryFacts(goCtx context.Context, msg *types.MsgBatchQueryFacts) (*types.MsgBatchQueryFactsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	callerAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	provider, found := k.GetProvider(ctx, msg.Provider)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotFound, msg.Provider)
	}
	if !provider.Active {
		return nil, fmt.Errorf("%w: %s", types.ErrProviderNotActive, msg.Provider)
	}

	providerAddr, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, fmt.Errorf("invalid provider address: %w", err)
	}

	currentBlock := uint64(ctx.BlockHeight())

	totalPrice, breakdown := k.CalculateQueryPrice(ctx, msg.FactIds, currentBlock)
	if totalPrice.Sign() == 0 {
		return nil, types.ErrZeroPayment
	}

	distribution := k.CalculateDistribution(ctx, totalPrice, msg.FactIds)

	if err := k.ExecuteDistribution(ctx, callerAddr, providerAddr, distribution); err != nil {
		return nil, fmt.Errorf("failed to execute distribution: %w", err)
	}

	// Increment citation counts
	for _, factId := range msg.FactIds {
		if err := k.knowledgeKeeper.IncrementCitationCount(ctx, factId); err != nil {
			k.Logger(ctx).Error("failed to increment citation count", "fact_id", factId, "error", err)
		}
	}

	// Update provider stats
	provider.TotalQueries += uint64(len(msg.FactIds))
	revenue := new(big.Int)
	revenue.SetString(provider.TotalRevenue, 10)
	providerShare := new(big.Int)
	providerShare.SetString(distribution.ProviderShare, 10)
	revenue.Add(revenue, providerShare)
	provider.TotalRevenue = revenue.String()
	k.SetProvider(ctx, provider)

	// Build quote responses
	var quotes []*types.QueryQuote
	for _, b := range breakdown {
		quotes = append(quotes, &types.QueryQuote{
			FactId:         b.FactId,
			BasePrice:      b.BaseCost,
			EffectivePrice: b.TotalPrice,
		})
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.batch_facts_queried",
			sdk.NewAttribute("caller", msg.Sender),
			sdk.NewAttribute("provider", msg.Provider),
			sdk.NewAttribute("fact_count", fmt.Sprintf("%d", len(msg.FactIds))),
			sdk.NewAttribute("total_price", totalPrice.String()),
		),
	)

	return &types.MsgBatchQueryFactsResponse{
		Quotes:     quotes,
		TotalPrice: totalPrice.String(),
	}, nil
}

// UpdateParams handles governance-gated parameter update.
func (k msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if k.GetAuthority() != msg.Authority {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", k.GetAuthority(), msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if msg.Params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}
	k.SetParams(ctx, msg.Params)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.params_updated",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}

// DevelopmentFundSpend handles governance-gated disbursement from the development_fund.
//
// Security invariants enforced:
//  1. Only governance authority may call (prevents arbitrary account drain).
//  2. DevelopmentFundSpendingLimitBps caps the fraction of current balance per proposal
//     (prevents single-proposal drain even if governance is captured briefly).
//  3. Amount must not exceed current fund balance (no overdraft).
func (k msgServer) DevelopmentFundSpend(goCtx context.Context, msg *types.MsgDevelopmentFundSpend) (*types.MsgDevelopmentFundSpendResponse, error) {
	if k.GetAuthority() != msg.Authority {
		return nil, types.ErrUnauthorized.Wrapf("expected %s, got %s", k.GetAuthority(), msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	params := k.GetParams(ctx)

	// Enforce spending gate: limit of 0 means spending is disabled.
	if params.DevelopmentFundSpendingLimitBps == 0 {
		return nil, types.ErrSpendingDisabled
	}

	// Parse requested amount.
	amtBig := new(big.Int)
	if _, ok := amtBig.SetString(msg.Amount, 10); !ok || amtBig.Sign() <= 0 {
		return nil, types.ErrInvalidAmount.Wrapf("amount %q is not a positive integer", msg.Amount)
	}

	// Get current development_fund balance.
	fundBalance := k.bankKeeper.GetModuleBalance(ctx, DevelopmentFund, "uzrn")
	balanceBig := fundBalance.Amount.BigInt()

	// Check balance sufficiency.
	if amtBig.Cmp(balanceBig) > 0 {
		return nil, types.ErrInsufficientDevelopmentFund.Wrapf(
			"requested %s uzrn but fund holds %s uzrn", amtBig, balanceBig,
		)
	}

	// Enforce per-proposal spending limit: amount <= balance * limitBps / 1_000_000.
	limitNumerator := new(big.Int).Mul(balanceBig, big.NewInt(int64(params.DevelopmentFundSpendingLimitBps)))
	limitBig := new(big.Int).Div(limitNumerator, big.NewInt(1_000_000))
	if amtBig.Cmp(limitBig) > 0 {
		return nil, types.ErrSpendingLimitExceeded.Wrapf(
			"requested %s uzrn exceeds limit %s uzrn (%d bps of %s balance)",
			amtBig, limitBig, params.DevelopmentFundSpendingLimitBps, balanceBig,
		)
	}

	// Enforce protocol treasury reserve floor before spending.
	// If MinProtocolReserveBps == 0 the check is disabled.
	if params.MinProtocolReserveBps > 0 {
		protocolBal := k.bankKeeper.GetModuleBalance(ctx, ProtocolTreasury, "uzrn")
		protocolBig := protocolBal.Amount.BigInt()
		// combined = protocol_treasury + development_fund (current, before the spend)
		combined := new(big.Int).Add(protocolBig, balanceBig)
		if combined.Sign() > 0 {
			// ratio = protocolBig * 1_000_000 / combined
			ratioBig := new(big.Int).Mul(protocolBig, big.NewInt(1_000_000))
			ratioBig.Div(ratioBig, combined)
			minReserve := big.NewInt(int64(params.MinProtocolReserveBps))
			if ratioBig.Cmp(minReserve) < 0 {
				return nil, types.ErrProtocolReserveBreach.Wrapf(
					"protocol treasury is %s uzrn (%s bps of combined %s uzrn); minimum is %d bps",
					protocolBig, ratioBig, combined, params.MinProtocolReserveBps,
				)
			}
		}
	}

	// Transfer from development_fund to recipient.
	recipientAddr, err := sdk.AccAddressFromBech32(msg.Recipient)
	if err != nil {
		return nil, types.ErrInvalidAmount.Wrapf("invalid recipient address: %s", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amtBig)))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, DevelopmentFund, recipientAddr, coins); err != nil {
		return nil, fmt.Errorf("development fund transfer failed: %w", err)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.billing.development_fund_spend",
			sdk.NewAttribute("authority", msg.Authority),
			sdk.NewAttribute("recipient", msg.Recipient),
			sdk.NewAttribute("amount", msg.Amount),
			sdk.NewAttribute("description", msg.Description),
			sdk.NewAttribute("fund_balance_before", balanceBig.String()),
			sdk.NewAttribute("spending_limit_bps", fmt.Sprintf("%d", params.DevelopmentFundSpendingLimitBps)),
		),
	)

	return &types.MsgDevelopmentFundSpendResponse{AmountSpent: msg.Amount}, nil
}
