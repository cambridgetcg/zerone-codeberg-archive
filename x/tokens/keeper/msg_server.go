package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/tokens/types"
)

type msgServer struct {
	Keeper
	types.UnimplementedMsgServer
}

// NewMsgServerImpl returns an implementation of the MsgServer interface.
// 15 handlers: 8 core ZRN-20 token ops, 2 delegation, 2 wrap, 2 emission, 1 governance.
func NewMsgServerImpl(keeper Keeper) *msgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = (*msgServer)(nil)

// CreateToken creates a new ZRN-20 secondary token.
func (s *msgServer) CreateToken(goCtx context.Context, msg *types.MsgCreateToken) (*types.MsgCreateTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !isValidSymbol(msg.Symbol) {
		return nil, types.ErrInvalidSymbol
	}

	if len(msg.Name) == 0 || len(msg.Name) > 64 {
		return nil, types.ErrInvalidName
	}

	if existing := s.GetTokenBySymbol(ctx, msg.Symbol); existing != nil {
		return nil, types.ErrTokenAlreadyExists
	}

	initialSupply := new(big.Int)
	if msg.InitialSupply != "" {
		if _, ok := initialSupply.SetString(msg.InitialSupply, 10); !ok {
			return nil, types.ErrInvalidAmount
		}
	}

	maxSupply := new(big.Int)
	if msg.MaxSupply != "" {
		if _, ok := maxSupply.SetString(msg.MaxSupply, 10); !ok {
			return nil, types.ErrInvalidAmount
		}
	}

	if maxSupply.Sign() > 0 && initialSupply.Cmp(maxSupply) > 0 {
		return nil, types.ErrSupplyExceeded
	}

	tokenId := GenerateTokenID(msg.Creator, msg.Symbol, ctx.BlockHeight())

	features := msg.Features
	if features == nil {
		features = &types.TokenFeatures{}
	}

	token := &types.TokenDefinition{
		Id:          tokenId,
		Creator:     msg.Creator,
		Name:        msg.Name,
		Symbol:      msg.Symbol,
		Decimals:    msg.Decimals,
		TotalSupply: initialSupply.String(),
		MaxSupply:   maxSupply.String(),
		Features:    features,
		Paused:      false,
		CreatedAt:   uint64(ctx.BlockHeight()),
	}

	s.SetToken(ctx, token)

	if initialSupply.Sign() > 0 {
		s.SetBalance(ctx, tokenId, msg.Creator, initialSupply)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_created",
		sdk.NewAttribute("token_id", tokenId),
		sdk.NewAttribute("creator", msg.Creator),
		sdk.NewAttribute("symbol", msg.Symbol),
		sdk.NewAttribute("initial_supply", initialSupply.String()),
	))

	return &types.MsgCreateTokenResponse{TokenId: tokenId}, nil
}

// MintToken mints new tokens to a recipient. Only creator or governance can mint.
func (s *msgServer) MintToken(goCtx context.Context, msg *types.MsgMintToken) (*types.MsgMintTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if token.Features == nil || !token.Features.Mintable {
		return nil, types.ErrNotMintable
	}

	if msg.Authority != token.Creator && msg.Authority != s.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	totalSupply := new(big.Int)
	totalSupply.SetString(token.TotalSupply, 10)
	newSupply := new(big.Int).Add(totalSupply, amount)

	maxSupply := new(big.Int)
	maxSupply.SetString(token.MaxSupply, 10)
	if maxSupply.Sign() > 0 && newSupply.Cmp(maxSupply) > 0 {
		return nil, types.ErrSupplyExceeded
	}

	token.TotalSupply = newSupply.String()
	s.SetToken(ctx, token)

	bal := s.GetBalance(ctx, msg.TokenId, msg.To)
	bal.Add(bal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.To, bal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_minted",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("to", msg.To),
		sdk.NewAttribute("amount", amount.String()),
	))

	return &types.MsgMintTokenResponse{}, nil
}

// BurnToken burns tokens from the burner's own balance.
func (s *msgServer) BurnToken(goCtx context.Context, msg *types.MsgBurnToken) (*types.MsgBurnTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if token.Features == nil || !token.Features.Burnable {
		return nil, types.ErrNotBurnable
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	undelegated := s.GetUndelegatedBalance(ctx, msg.TokenId, msg.Burner)
	if undelegated.Cmp(amount) < 0 {
		return nil, types.ErrInsufficientUndelegatedBalance
	}

	bal := s.GetBalance(ctx, msg.TokenId, msg.Burner)
	bal.Sub(bal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.Burner, bal)

	totalSupply := new(big.Int)
	totalSupply.SetString(token.TotalSupply, 10)
	totalSupply.Sub(totalSupply, amount)
	token.TotalSupply = totalSupply.String()
	s.SetToken(ctx, token)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_burned",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("burner", msg.Burner),
		sdk.NewAttribute("amount", amount.String()),
	))

	return &types.MsgBurnTokenResponse{}, nil
}

// TransferToken transfers tokens from sender to recipient.
func (s *msgServer) TransferToken(goCtx context.Context, msg *types.MsgTransferToken) (*types.MsgTransferTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if msg.Sender == msg.To {
		return nil, types.ErrSelfTransfer
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	undelegated := s.GetUndelegatedBalance(ctx, msg.TokenId, msg.Sender)
	if undelegated.Cmp(amount) < 0 {
		return nil, types.ErrInsufficientUndelegatedBalance
	}

	senderBal := s.GetBalance(ctx, msg.TokenId, msg.Sender)
	senderBal.Sub(senderBal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.Sender, senderBal)

	recipientBal := s.GetBalance(ctx, msg.TokenId, msg.To)
	recipientBal.Add(recipientBal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.To, recipientBal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_transferred",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("from", msg.Sender),
		sdk.NewAttribute("to", msg.To),
		sdk.NewAttribute("amount", amount.String()),
	))

	return &types.MsgTransferTokenResponse{}, nil
}

// ApproveToken sets an allowance for a spender. Amount=0 revokes.
func (s *msgServer) ApproveToken(goCtx context.Context, msg *types.MsgApproveToken) (*types.MsgApproveTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if msg.Owner == msg.Spender {
		return nil, types.ErrSelfTransfer
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() < 0 {
		return nil, types.ErrInvalidAmount
	}

	s.SetAllowance(ctx, msg.TokenId, msg.Owner, msg.Spender, amount)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_approved",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("owner", msg.Owner),
		sdk.NewAttribute("spender", msg.Spender),
		sdk.NewAttribute("amount", amount.String()),
	))

	return &types.MsgApproveTokenResponse{}, nil
}

// TransferFrom transfers tokens on behalf of the owner using an allowance.
func (s *msgServer) TransferFrom(goCtx context.Context, msg *types.MsgTransferFrom) (*types.MsgTransferFromResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if msg.From == msg.To {
		return nil, types.ErrSelfTransfer
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	allowance := s.GetAllowance(ctx, msg.TokenId, msg.From, msg.Spender)
	if allowance.Cmp(amount) < 0 {
		return nil, types.ErrAllowanceExceeded
	}

	undelegated := s.GetUndelegatedBalance(ctx, msg.TokenId, msg.From)
	if undelegated.Cmp(amount) < 0 {
		return nil, types.ErrInsufficientUndelegatedBalance
	}
	fromBal := s.GetBalance(ctx, msg.TokenId, msg.From)

	allowance.Sub(allowance, amount)
	s.SetAllowance(ctx, msg.TokenId, msg.From, msg.Spender, allowance)

	fromBal.Sub(fromBal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.From, fromBal)

	toBal := s.GetBalance(ctx, msg.TokenId, msg.To)
	toBal.Add(toBal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.To, toBal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.transfer_from",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("spender", msg.Spender),
		sdk.NewAttribute("from", msg.From),
		sdk.NewAttribute("to", msg.To),
		sdk.NewAttribute("amount", amount.String()),
	))

	return &types.MsgTransferFromResponse{}, nil
}

// PauseToken pauses a token. Only creator or governance can pause.
func (s *msgServer) PauseToken(goCtx context.Context, msg *types.MsgPauseToken) (*types.MsgPauseTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Features == nil || !token.Features.Pausable {
		return nil, types.ErrNotPausable
	}

	if msg.Authority != token.Creator && msg.Authority != s.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	token.Paused = true
	s.SetToken(ctx, token)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_paused",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgPauseTokenResponse{}, nil
}

// UnpauseToken unpauses a paused token. Only creator or governance can unpause.
func (s *msgServer) UnpauseToken(goCtx context.Context, msg *types.MsgUnpauseToken) (*types.MsgUnpauseTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Features == nil || !token.Features.Pausable {
		return nil, types.ErrNotPausable
	}

	if msg.Authority != token.Creator && msg.Authority != s.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	if !token.Paused {
		return nil, types.ErrTokenNotPaused
	}

	token.Paused = false
	s.SetToken(ctx, token)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_unpaused",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgUnpauseTokenResponse{}, nil
}

// ---------- Delegation Handlers ----------

// DelegatePower delegates governance voting power for a ZRN-20 token.
// Sets the delegation to the specified amount (replaces previous amount).
func (s *msgServer) DelegatePower(goCtx context.Context, msg *types.MsgDelegatePower) (*types.MsgDelegatePowerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if msg.Delegator == msg.Delegate {
		return nil, types.ErrSelfDelegation
	}

	newAmount := new(big.Int)
	if _, ok := newAmount.SetString(msg.Amount, 10); !ok || newAmount.Sign() < 0 {
		return nil, types.ErrInvalidAmount
	}

	currentDelegation := s.GetDelegation(ctx, msg.TokenId, msg.Delegator, msg.Delegate)
	currentTotal := s.GetDelegatorTotal(ctx, msg.TokenId, msg.Delegator)

	if newAmount.Cmp(currentDelegation) > 0 {
		delta := new(big.Int).Sub(newAmount, currentDelegation)
		undelegated := s.GetUndelegatedBalance(ctx, msg.TokenId, msg.Delegator)
		if undelegated.Cmp(delta) < 0 {
			return nil, types.ErrInsufficientUndelegatedBalance
		}
		currentTotal.Add(currentTotal, delta)
	} else if newAmount.Cmp(currentDelegation) < 0 {
		delta := new(big.Int).Sub(currentDelegation, newAmount)
		currentTotal.Sub(currentTotal, delta)
	} else {
		return &types.MsgDelegatePowerResponse{}, nil
	}

	s.SetDelegation(ctx, msg.TokenId, msg.Delegator, msg.Delegate, newAmount)
	s.SetDelegatorTotal(ctx, msg.TokenId, msg.Delegator, currentTotal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.power_delegated",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("delegator", msg.Delegator),
		sdk.NewAttribute("delegate", msg.Delegate),
		sdk.NewAttribute("amount", newAmount.String()),
	))

	return &types.MsgDelegatePowerResponse{}, nil
}

// UndelegatePower removes a specific amount of delegated voting power.
func (s *msgServer) UndelegatePower(goCtx context.Context, msg *types.MsgUndelegatePower) (*types.MsgUndelegatePowerResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if msg.Delegator == msg.Delegate {
		return nil, types.ErrSelfDelegation
	}

	undelegateAmount := new(big.Int)
	if _, ok := undelegateAmount.SetString(msg.Amount, 10); !ok || undelegateAmount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	currentDelegation := s.GetDelegation(ctx, msg.TokenId, msg.Delegator, msg.Delegate)
	if currentDelegation.Cmp(undelegateAmount) < 0 {
		return nil, types.ErrInsufficientUndelegatedBalance
	}

	newDelegation := new(big.Int).Sub(currentDelegation, undelegateAmount)
	s.SetDelegation(ctx, msg.TokenId, msg.Delegator, msg.Delegate, newDelegation)

	currentTotal := s.GetDelegatorTotal(ctx, msg.TokenId, msg.Delegator)
	currentTotal.Sub(currentTotal, undelegateAmount)
	s.SetDelegatorTotal(ctx, msg.TokenId, msg.Delegator, currentTotal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.power_undelegated",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("delegator", msg.Delegator),
		sdk.NewAttribute("delegate", msg.Delegate),
		sdk.NewAttribute("amount", undelegateAmount.String()),
	))

	return &types.MsgUndelegatePowerResponse{}, nil
}

// ---------- Wrap/Unwrap Handlers ----------

// WrapToken wraps a ZRN-20 secondary token into an IBC-compatible sdk.Coin.
func (s *msgServer) WrapToken(goCtx context.Context, msg *types.MsgWrapToken) (*types.MsgWrapTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	token := s.GetToken(ctx, msg.TokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	if token.Features == nil || !token.Features.Wrappable {
		return nil, types.ErrNotWrappable
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	undelegated := s.GetUndelegatedBalance(ctx, msg.TokenId, msg.Sender)
	if undelegated.Cmp(amount) < 0 {
		return nil, types.ErrInsufficientUndelegatedBalance
	}

	bal := s.GetBalance(ctx, msg.TokenId, msg.Sender)
	bal.Sub(bal, amount)
	s.SetBalance(ctx, msg.TokenId, msg.Sender, bal)

	wrappedDenom := WrappedDenom(msg.TokenId)
	s.SetWrapRecord(ctx, msg.TokenId, wrappedDenom)

	senderAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin(wrappedDenom, sdkmath.NewIntFromBigInt(amount)))
	if err := s.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to mint wrapped coins: %w", err)
	}
	if err := s.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, coins); err != nil {
		return nil, fmt.Errorf("failed to send wrapped coins: %w", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_wrapped",
		sdk.NewAttribute("token_id", msg.TokenId),
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("amount", amount.String()),
		sdk.NewAttribute("wrapped_denom", wrappedDenom),
	))

	return &types.MsgWrapTokenResponse{WrappedDenom: wrappedDenom}, nil
}

// UnwrapToken unwraps an IBC-compatible sdk.Coin back into the original ZRN-20 token.
func (s *msgServer) UnwrapToken(goCtx context.Context, msg *types.MsgUnwrapToken) (*types.MsgUnwrapTokenResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	tokenId := s.GetTokenIdByWrappedDenom(ctx, msg.WrappedDenom)
	if tokenId == "" {
		return nil, types.ErrWrapRecordNotFound
	}

	token := s.GetToken(ctx, tokenId)
	if token == nil {
		return nil, types.ErrTokenNotFound
	}

	if token.Paused {
		return nil, types.ErrTokenPaused
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	senderAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	coin := s.bankKeeper.GetBalance(ctx, senderAddr, msg.WrappedDenom)
	if coin.Amount.BigInt().Cmp(amount) < 0 {
		return nil, types.ErrInsufficientBalance
	}

	coins := sdk.NewCoins(sdk.NewCoin(msg.WrappedDenom, sdkmath.NewIntFromBigInt(amount)))
	if err := s.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to send wrapped coins to module: %w", err)
	}
	if err := s.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to burn wrapped coins: %w", err)
	}

	bal := s.GetBalance(ctx, tokenId, msg.Sender)
	bal.Add(bal, amount)
	s.SetBalance(ctx, tokenId, msg.Sender, bal)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.token_unwrapped",
		sdk.NewAttribute("token_id", tokenId),
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("amount", amount.String()),
		sdk.NewAttribute("wrapped_denom", msg.WrappedDenom),
	))

	return &types.MsgUnwrapTokenResponse{TokenId: tokenId}, nil
}

// ---------- Emission Handlers ----------

// CreateEmissionPeriod creates a scheduled token emission. Governance-gated.
func (s *msgServer) CreateEmissionPeriod(goCtx context.Context, msg *types.MsgCreateEmissionPeriod) (*types.MsgCreateEmissionPeriodResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != s.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", s.GetAuthority(), msg.Authority)
	}

	if msg.EndBlock <= msg.StartBlock {
		return nil, types.ErrInvalidEmissionRange
	}

	amountPerBlock := new(big.Int)
	if _, ok := amountPerBlock.SetString(msg.AmountPerBlock, 10); !ok || amountPerBlock.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	// Gap 5: Validate multi-recipient splits when provided.
	if len(msg.Recipients) > 0 {
		var total uint64
		for _, split := range msg.Recipients {
			if split == nil {
				return nil, fmt.Errorf("recipient split must not be nil")
			}
			if _, err := sdk.AccAddressFromBech32(split.Address); err != nil {
				return nil, fmt.Errorf("invalid recipient address %q: %w", split.Address, err)
			}
			total += split.Bps
		}
		if total != types.EmissionBPSScale {
			return nil, fmt.Errorf("recipients bps must sum to %d, got %d", types.EmissionBPSScale, total)
		}
	}

	emissionId := GenerateEmissionID(msg.Authority, msg.StartBlock, msg.EndBlock)

	emission := &types.EmissionPeriod{
		Id:             emissionId,
		StartBlock:     msg.StartBlock,
		EndBlock:       msg.EndBlock,
		AmountPerBlock: msg.AmountPerBlock,
		Recipient:      msg.Recipient,
		Recipients:     msg.Recipients,
		Active:         true,
		TotalEmitted:   "0",
		Creator:        msg.Authority,
	}

	s.SetEmissionPeriod(ctx, emission)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.emission_period_created",
		sdk.NewAttribute("emission_id", emissionId),
		sdk.NewAttribute("start_block", fmt.Sprintf("%d", msg.StartBlock)),
		sdk.NewAttribute("end_block", fmt.Sprintf("%d", msg.EndBlock)),
		sdk.NewAttribute("amount_per_block", msg.AmountPerBlock),
		sdk.NewAttribute("recipient", msg.Recipient),
		sdk.NewAttribute("recipient_count", fmt.Sprintf("%d", len(msg.Recipients))),
	))

	return &types.MsgCreateEmissionPeriodResponse{EmissionId: emissionId}, nil
}

// CancelEmissionPeriod deactivates an emission period. Governance-gated.
func (s *msgServer) CancelEmissionPeriod(goCtx context.Context, msg *types.MsgCancelEmissionPeriod) (*types.MsgCancelEmissionPeriodResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != s.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", s.GetAuthority(), msg.Authority)
	}

	emission := s.GetEmissionPeriod(ctx, msg.EmissionId)
	if emission == nil {
		return nil, types.ErrEmissionNotFound
	}

	if !emission.Active {
		return nil, types.ErrEmissionInactive
	}

	emission.Active = false
	s.SetEmissionPeriod(ctx, emission)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.emission_period_cancelled",
		sdk.NewAttribute("emission_id", msg.EmissionId),
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgCancelEmissionPeriodResponse{}, nil
}

// ---------- Governance ----------

// UpdateParams updates the tokens module parameters. Only callable by governance.
func (s *msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != s.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", s.GetAuthority(), msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Params != nil {
		if err := msg.Params.Validate(); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		s.SetParams(ctx, msg.Params)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.tokens.params_updated",
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgUpdateParamsResponse{}, nil
}
