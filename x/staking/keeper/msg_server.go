package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/staking/types"
)

type msgServer struct {
	Keeper
	types.UnimplementedMsgServer
}

// NewMsgServerImpl returns a MsgServer implementation.
func NewMsgServerImpl(k Keeper) types.MsgServer {
	return &msgServer{Keeper: k}
}

var _ types.MsgServer = &msgServer{}

// RegisterValidator registers a new validator.
func (ms *msgServer) RegisterValidator(goCtx context.Context, msg *types.MsgRegisterValidator) (*types.MsgRegisterValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	operatorAddr, err := sdk.AccAddressFromBech32(msg.Operator)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrap("invalid operator address")
	}

	// Check not already registered.
	if _, found := ms.GetValidator(ctx, msg.Operator); found {
		return nil, types.ErrValidatorAlreadyExists
	}

	// Check DID not already registered.
	if msg.Did != "" {
		if _, found := ms.GetValidatorByDID(ctx, msg.Did); found {
			return nil, types.ErrDIDAlreadyRegistered
		}
	}

	// Parse and enforce MinSelfDelegation.
	selfDel := new(big.Int)
	if msg.SelfDelegation != "" {
		var ok bool
		selfDel, ok = selfDel.SetString(msg.SelfDelegation, 10)
		if !ok {
			return nil, types.ErrInvalidAmount.Wrap("invalid self delegation amount")
		}
	}

	params := ms.GetParams(ctx)
	minSelfDel, _ := new(big.Int).SetString(params.MinSelfDelegation, 10)
	if minSelfDel != nil && selfDel.Cmp(minSelfDel) < 0 {
		return nil, types.ErrInsufficientSelfDelegation.Wrapf("minimum %s uzrn, got %s", params.MinSelfDelegation, selfDel.String())
	}

	// Compute initial tier.
	initialTier := ComputeValidatorTier(ctx, ms.Keeper, selfDel, 0, 0, 0, 0, 0)

	// If tier >= Scholar (block producer), check MaxValidators cap.
	if initialTier >= types.TierScholar {
		if ms.CountBlockProducers(ctx) >= params.MaxValidators {
			return nil, types.ErrMaxValidatorsReached
		}
	}

	// Transfer self-delegation to staking module.
	if selfDel.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(selfDel)))
		if err := ms.bankKeeper.SendCoinsFromAccountToModule(ctx, operatorAddr, types.ModuleName, coins); err != nil {
			return nil, err
		}
	}

	// Create validator.
	val := &types.Validator{
		OperatorAddress:  msg.Operator,
		ConsensusPubkey:  msg.ConsensusPubkey,
		Did:              msg.Did,
		Moniker:          msg.Moniker,
		Tier:             initialTier,
		SelfDelegation:   selfDel.String(),
		DelegatedStake:   "0",
		TotalStake:       selfDel.String(),
		ReputationScore:  500_000, // start at 50%
		JoinedAtBlock:    uint64(ctx.BlockHeight()),
		IsActive:         true,
		CommissionBps:    msg.CommissionBps,
		Website:          msg.Website,
		Details:          msg.Details,
	}

	ms.SetValidator(ctx, val)

	// Create self-delegation record.
	if selfDel.Sign() > 0 {
		ms.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: msg.Operator,
			ValidatorAddress: msg.Operator,
			Amount:           selfDel.String(),
			CreatedAtBlock:   uint64(ctx.BlockHeight()),
		})
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.validator_registered",
		sdk.NewAttribute("operator", msg.Operator),
		sdk.NewAttribute("tier", types.ValidatorTierString(initialTier)),
		sdk.NewAttribute("self_delegation", selfDel.String()),
	))

	return &types.MsgRegisterValidatorResponse{
		InitialTier: uint32(initialTier),
	}, nil
}

// Delegate delegates tokens to a validator.
func (ms *msgServer) Delegate(goCtx context.Context, msg *types.MsgDelegate) (*types.MsgDelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	delegatorAddr, err := sdk.AccAddressFromBech32(msg.Delegator)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	val, found := ms.GetValidator(ctx, msg.Validator)
	if !found {
		return nil, types.ErrValidatorNotFound
	}
	if !val.IsActive {
		return nil, types.ErrValidatorInactive
	}

	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	// Transfer tokens to staking module.
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
	if err := ms.bankKeeper.SendCoinsFromAccountToModule(ctx, delegatorAddr, types.ModuleName, coins); err != nil {
		return nil, err
	}

	// Upsert delegation.
	existingDel, found := ms.GetDelegation(ctx, msg.Delegator, msg.Validator)
	var newTotal *big.Int
	if found {
		existing, _ := new(big.Int).SetString(existingDel.Amount, 10)
		if existing == nil {
			existing = new(big.Int)
		}
		newTotal = new(big.Int).Add(existing, amt)
		existingDel.Amount = newTotal.String()
		ms.SetDelegation(ctx, existingDel)
	} else {
		newTotal = new(big.Int).Set(amt)
		ms.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: msg.Delegator,
			ValidatorAddress: msg.Validator,
			Amount:           amt.String(),
			CreatedAtBlock:   uint64(ctx.BlockHeight()),
		})
	}

	// Update validator's delegated stake and total stake.
	delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegated == nil {
		delegated = new(big.Int)
	}
	delegated.Add(delegated, amt)
	val.DelegatedStake = delegated.String()

	selfStake, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStake == nil {
		selfStake = new(big.Int)
	}
	val.TotalStake = new(big.Int).Add(selfStake, delegated).String()

	if newTier, changed := ms.CheckTierTransition(ctx, val); changed {
		ms.ApplyTierTransition(ctx, val, newTier, "stake_delegate")
	}
	ms.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.delegation_created",
		sdk.NewAttribute("delegator", msg.Delegator),
		sdk.NewAttribute("validator", msg.Validator),
		sdk.NewAttribute("amount", msg.Amount),
	))

	return &types.MsgDelegateResponse{
		NewDelegation: newTotal.String(),
	}, nil
}

// Undelegate initiates unbonding.
func (ms *msgServer) Undelegate(goCtx context.Context, msg *types.MsgUndelegate) (*types.MsgUndelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Tombstone gate: a tombstoned validator's operator cannot self-unbond.
	// External delegators can still exit — only the operator's self-stake is held.
	// Governance must call MsgUntombstoneValidator before the operator can withdraw.
	if val, found := ms.GetValidator(ctx, msg.Validator); found && val.Tombstoned {
		if msg.Delegator == val.OperatorAddress {
			return nil, types.ErrTombstonedCannotSelfUnbond.Wrapf(
				"validator %s is tombstoned; operator must obtain governance approval via MsgUntombstoneValidator",
				msg.Validator)
		}
	}

	del, found := ms.GetDelegation(ctx, msg.Delegator, msg.Validator)
	if !found {
		return nil, types.ErrDelegationNotFound
	}

	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	existing, _ := new(big.Int).SetString(del.Amount, 10)
	if existing == nil || existing.Cmp(amt) < 0 {
		return nil, types.ErrInsufficientDelegation
	}

	params := ms.GetParams(ctx)
	currentHeight := uint64(ctx.BlockHeight())
	completesAt := currentHeight + params.UnbondingPeriod

	// Create unbonding entry.
	seq := ms.NextUnbondingSeq(ctx)
	unbondingID := fmt.Sprintf("%s_%s_%d_%d", msg.Delegator, msg.Validator, currentHeight, seq)
	entry := &types.UnbondingEntry{
		Id:               unbondingID,
		DelegatorAddress: msg.Delegator,
		ValidatorAddress: msg.Validator,
		Amount:           msg.Amount,
		CreatedAtHeight:  currentHeight,
		CompletesAtHeight: completesAt,
		Status:           "pending",
	}
	ms.SetUnbonding(ctx, entry)

	// Update delegation.
	remaining := new(big.Int).Sub(existing, amt)
	if remaining.Sign() == 0 {
		ms.DeleteDelegation(ctx, msg.Delegator, msg.Validator)
	} else {
		del.Amount = remaining.String()
		ms.SetDelegation(ctx, del)
	}

	// Update validator stake.
	val, found := ms.GetValidator(ctx, msg.Validator)
	if found {
		delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
		if delegated == nil {
			delegated = new(big.Int)
		}
		delegated.Sub(delegated, amt)
		if delegated.Sign() < 0 {
			delegated.SetInt64(0)
		}
		val.DelegatedStake = delegated.String()

		selfStake, _ := new(big.Int).SetString(val.SelfDelegation, 10)
		if selfStake == nil {
			selfStake = new(big.Int)
		}
		val.TotalStake = new(big.Int).Add(selfStake, delegated).String()

		if newTier, changed := ms.CheckTierTransition(ctx, val); changed {
			ms.ApplyTierTransition(ctx, val, newTier, "stake_undelegate")
		}
		ms.SetValidator(ctx, val)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.delegation_unbonding",
		sdk.NewAttribute("delegator", msg.Delegator),
		sdk.NewAttribute("validator", msg.Validator),
		sdk.NewAttribute("amount", msg.Amount),
		sdk.NewAttribute("completes_at", fmt.Sprintf("%d", completesAt)),
	))

	return &types.MsgUndelegateResponse{
		UnbondingId:       unbondingID,
		CompletesAtHeight: completesAt,
	}, nil
}

// Redelegate moves a delegation between validators.
func (ms *msgServer) Redelegate(goCtx context.Context, msg *types.MsgRedelegate) (*types.MsgRedelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentHeight := uint64(ctx.BlockHeight())

	// Check cooldown.
	params := ms.GetParams(ctx)
	lastRedel := ms.GetLastRedelegationHeight(ctx, msg.Delegator)
	if lastRedel > 0 && currentHeight < lastRedel+params.RedelegationCooldownBlocks {
		return nil, types.ErrRedelegationCooldown
	}

	// Check source delegation.
	srcDel, found := ms.GetDelegation(ctx, msg.Delegator, msg.SrcValidator)
	if !found {
		return nil, types.ErrDelegationNotFound
	}

	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	srcAmt, _ := new(big.Int).SetString(srcDel.Amount, 10)
	if srcAmt == nil || srcAmt.Cmp(amt) < 0 {
		return nil, types.ErrInsufficientDelegation
	}

	// Check destination validator.
	dstVal, found := ms.GetValidator(ctx, msg.DstValidator)
	if !found {
		return nil, types.ErrValidatorNotFound
	}
	if !dstVal.IsActive {
		return nil, types.ErrValidatorInactive
	}

	// Update source delegation.
	remaining := new(big.Int).Sub(srcAmt, amt)
	if remaining.Sign() == 0 {
		ms.DeleteDelegation(ctx, msg.Delegator, msg.SrcValidator)
	} else {
		srcDel.Amount = remaining.String()
		ms.SetDelegation(ctx, srcDel)
	}

	// Upsert destination delegation.
	dstDel, found := ms.GetDelegation(ctx, msg.Delegator, msg.DstValidator)
	if found {
		dstAmt, _ := new(big.Int).SetString(dstDel.Amount, 10)
		if dstAmt == nil {
			dstAmt = new(big.Int)
		}
		dstDel.Amount = new(big.Int).Add(dstAmt, amt).String()
		ms.SetDelegation(ctx, dstDel)
	} else {
		ms.SetDelegation(ctx, &types.Delegation{
			DelegatorAddress: msg.Delegator,
			ValidatorAddress: msg.DstValidator,
			Amount:           amt.String(),
			CreatedAtBlock:   currentHeight,
		})
	}

	// Update source validator.
	srcVal, found := ms.GetValidator(ctx, msg.SrcValidator)
	if found {
		delegated, _ := new(big.Int).SetString(srcVal.DelegatedStake, 10)
		if delegated == nil {
			delegated = new(big.Int)
		}
		delegated.Sub(delegated, amt)
		if delegated.Sign() < 0 {
			delegated.SetInt64(0)
		}
		srcVal.DelegatedStake = delegated.String()
		selfStake, _ := new(big.Int).SetString(srcVal.SelfDelegation, 10)
		if selfStake == nil {
			selfStake = new(big.Int)
		}
		srcVal.TotalStake = new(big.Int).Add(selfStake, delegated).String()
		if newTier, changed := ms.CheckTierTransition(ctx, srcVal); changed {
			ms.ApplyTierTransition(ctx, srcVal, newTier, "redelegate_src")
		}
		ms.SetValidator(ctx, srcVal)
	}

	// Update destination validator.
	delegated, _ := new(big.Int).SetString(dstVal.DelegatedStake, 10)
	if delegated == nil {
		delegated = new(big.Int)
	}
	delegated.Add(delegated, amt)
	dstVal.DelegatedStake = delegated.String()
	selfStake, _ := new(big.Int).SetString(dstVal.SelfDelegation, 10)
	if selfStake == nil {
		selfStake = new(big.Int)
	}
	dstVal.TotalStake = new(big.Int).Add(selfStake, delegated).String()
	if newTier, changed := ms.CheckTierTransition(ctx, dstVal); changed {
		ms.ApplyTierTransition(ctx, dstVal, newTier, "redelegate_dst")
	}
	ms.SetValidator(ctx, dstVal)

	// Record cooldown.
	ms.SetLastRedelegationHeight(ctx, msg.Delegator, currentHeight)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.delegation_redelegated",
		sdk.NewAttribute("delegator", msg.Delegator),
		sdk.NewAttribute("src_validator", msg.SrcValidator),
		sdk.NewAttribute("dst_validator", msg.DstValidator),
		sdk.NewAttribute("amount", msg.Amount),
	))

	return &types.MsgRedelegateResponse{}, nil
}

// UpdateValidatorStake increases or decreases a validator's self-delegation.
func (ms *msgServer) UpdateValidatorStake(goCtx context.Context, msg *types.MsgUpdateValidatorStake) (*types.MsgUpdateValidatorStakeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	operatorAddr, err := sdk.AccAddressFromBech32(msg.Operator)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	val, found := ms.GetValidator(ctx, msg.Operator)
	if !found {
		return nil, types.ErrValidatorNotFound
	}

	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return nil, types.ErrInvalidAmount
	}

	selfStake, _ := new(big.Int).SetString(val.SelfDelegation, 10)
	if selfStake == nil {
		selfStake = new(big.Int)
	}

	if msg.Increase {
		// Increase: transfer tokens to module.
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
		if err := ms.bankKeeper.SendCoinsFromAccountToModule(ctx, operatorAddr, types.ModuleName, coins); err != nil {
			return nil, err
		}
		selfStake.Add(selfStake, amt)
		val.SelfDelegation = selfStake.String()

		// Upsert self-delegation record.
		del, found := ms.GetDelegation(ctx, msg.Operator, msg.Operator)
		if found {
			existing, _ := new(big.Int).SetString(del.Amount, 10)
			if existing == nil {
				existing = new(big.Int)
			}
			del.Amount = new(big.Int).Add(existing, amt).String()
			ms.SetDelegation(ctx, del)
		} else {
			ms.SetDelegation(ctx, &types.Delegation{
				DelegatorAddress: msg.Operator,
				ValidatorAddress: msg.Operator,
				Amount:           amt.String(),
				CreatedAtBlock:   uint64(ctx.BlockHeight()),
			})
		}
	} else {
		// Decrease: create unbonding entry.
		if selfStake.Cmp(amt) < 0 {
			return nil, types.ErrInsufficientSelfDelegation
		}
		selfStake.Sub(selfStake, amt)
		val.SelfDelegation = selfStake.String()

		// Create unbonding.
		params := ms.GetParams(ctx)
		currentHeight := uint64(ctx.BlockHeight())
		seq := ms.NextUnbondingSeq(ctx)
		unbondingID := fmt.Sprintf("%s_%s_%d_%d", msg.Operator, msg.Operator, currentHeight, seq)
		ms.SetUnbonding(ctx, &types.UnbondingEntry{
			Id:               unbondingID,
			DelegatorAddress: msg.Operator,
			ValidatorAddress: msg.Operator,
			Amount:           msg.Amount,
			CreatedAtHeight:  currentHeight,
			CompletesAtHeight: currentHeight + params.UnbondingPeriod,
			Status:           "pending",
		})

		// Update self-delegation record.
		del, found := ms.GetDelegation(ctx, msg.Operator, msg.Operator)
		if found {
			existing, _ := new(big.Int).SetString(del.Amount, 10)
			if existing == nil {
				existing = new(big.Int)
			}
			remaining := new(big.Int).Sub(existing, amt)
			if remaining.Sign() <= 0 {
				ms.DeleteDelegation(ctx, msg.Operator, msg.Operator)
			} else {
				del.Amount = remaining.String()
				ms.SetDelegation(ctx, del)
			}
		}

		// Deactivate if tier 0, zero stake, and insufficient verifications.
		if selfStake.Sign() == 0 && val.Tier == types.TierApprentice && val.TotalVerifications < 22 {
			val.IsActive = false
		}
	}

	// Recalculate total stake.
	delegated, _ := new(big.Int).SetString(val.DelegatedStake, 10)
	if delegated == nil {
		delegated = new(big.Int)
	}
	val.TotalStake = new(big.Int).Add(selfStake, delegated).String()

	if newTier, changed := ms.CheckTierTransition(ctx, val); changed {
		ms.ApplyTierTransition(ctx, val, newTier, "update_validator_stake")
	}
	ms.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.staking.update_validator_stake",
			sdk.NewAttribute("operator", msg.Operator),
			sdk.NewAttribute("new_stake", selfStake.String()),
		),
	)

	return &types.MsgUpdateValidatorStakeResponse{}, nil
}

// SubmitSlashEvidence queues on-chain evidence of validator misbehavior.
// The evidence is processed at EndBlock; if valid, the validator is slashed
// and the submitter receives evidence_reward_bps fraction of the slashed amount.
func (ms *msgServer) SubmitSlashEvidence(goCtx context.Context, msg *types.MsgSubmitSlashEvidence) (*types.MsgSubmitSlashEvidenceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Validator must exist
	if _, found := ms.GetValidator(ctx, msg.ValidatorAddr); !found {
		return nil, fmt.Errorf("%w: %s", types.ErrValidatorNotFound, msg.ValidatorAddr)
	}

	// Slash type must have a configured rate (otherwise this will be a no-op at end-block)
	params := ms.GetParams(ctx)
	if types.GetSlashRate(params, msg.SlashType) == 0 {
		return nil, fmt.Errorf("%w: no configured rate for slash type %s", types.ErrValidatorNotSlashable, msg.SlashType.String())
	}

	// Generate deterministic evidence ID from submitter + validator + block height
	currentBlock := uint64(ctx.BlockHeight())
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, currentBlock)
	h := sha256.Sum256(append(append([]byte(msg.Submitter), []byte(msg.ValidatorAddr)...), heightBytes...))
	evidenceID := hex.EncodeToString(h[:16])

	// Idempotency: reject duplicate evidence from same submitter for same validator same block
	if _, exists := ms.GetSlashEvidence(ctx, evidenceID); exists {
		return nil, fmt.Errorf("%w: evidence_id %s", types.ErrEvidenceAlreadySubmitted, evidenceID)
	}

	evidence := &types.SlashEvidence{
		Id:           evidenceID,
		Submitter:    msg.Submitter,
		ValidatorAddr: msg.ValidatorAddr,
		SlashType:    msg.SlashType,
		EvidenceHash: msg.EvidenceHash,
		WitnessData:  msg.WitnessData,
		SubmittedAt:  currentBlock,
		Processed:    false,
	}
	ms.SetSlashEvidence(ctx, evidence)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.slash_evidence_submitted",
		sdk.NewAttribute("evidence_id", evidenceID),
		sdk.NewAttribute("submitter", msg.Submitter),
		sdk.NewAttribute("validator", msg.ValidatorAddr),
		sdk.NewAttribute("slash_type", msg.SlashType.String()),
		sdk.NewAttribute("evidence_hash", msg.EvidenceHash),
	))

	return &types.MsgSubmitSlashEvidenceResponse{EvidenceId: evidenceID}, nil
}

// UpdateParams updates module parameters (governance-gated).
func (ms *msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if ms.GetAuthority() != msg.Authority {
		return nil, types.ErrUnauthorized.Wrapf("expected %s, got %s", ms.GetAuthority(), msg.Authority)
	}

	ms.SetParams(ctx, msg.Params)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.params_updated",
		sdk.NewAttribute("authority", msg.Authority),
	))

	return &types.MsgUpdateParamsResponse{}, nil
}

// ContributeToInsuranceFund deposits uzrn from the operator into the per-validator
// delegator insurance pool. Funds in this pool are used to absorb slash overflow
// before it propagates to delegated stake.
func (ms *msgServer) ContributeToInsuranceFund(goCtx context.Context, msg *types.MsgContributeToInsuranceFund) (*types.MsgContributeToInsuranceFundResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	val, found := ms.GetValidator(ctx, msg.Operator)
	if !found {
		return nil, types.ErrValidatorNotFound
	}

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrInvalidInsuranceAmount
	}

	// Transfer from operator to module
	operatorAddr, err := sdk.AccAddressFromBech32(msg.Operator)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amount)))
	if ms.bankKeeper != nil {
		if err := ms.bankKeeper.SendCoinsFromAccountToModule(ctx, operatorAddr, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("insurance fund contribution transfer failed: %w", err)
		}
	}

	// Credit the validator's insurance fund balance
	current, _ := new(big.Int).SetString(val.InsuranceFundBalance, 10)
	if current == nil {
		current = new(big.Int)
	}
	current.Add(current, amount)
	val.InsuranceFundBalance = current.String()
	ms.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.insurance_fund_contributed",
		sdk.NewAttribute("operator", msg.Operator),
		sdk.NewAttribute("amount", amount.String()),
		sdk.NewAttribute("new_balance", current.String()),
	))

	return &types.MsgContributeToInsuranceFundResponse{NewBalance: current.String()}, nil
}

// UntombstoneValidator lifts a permanent tombstone from a validator.
// This is governance-gated — only the module authority address can call it.
// Clearing the tombstone does NOT automatically re-activate the validator;
// governance must also update the validator's IsActive status separately.
func (ms *msgServer) UntombstoneValidator(goCtx context.Context, msg *types.MsgUntombstoneValidator) (*types.MsgUntombstoneValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Governance authority gate.
	if msg.Authority != ms.GetAuthority() {
		return nil, types.ErrUnauthorized.Wrapf(
			"only the governance module authority %s may untombstone validators; got %s",
			ms.GetAuthority(), msg.Authority)
	}

	val, found := ms.GetValidator(ctx, msg.ValidatorAddress)
	if !found {
		return nil, types.ErrValidatorNotFound
	}

	if !val.Tombstoned {
		return nil, types.ErrNotTombstoned.Wrapf(
			"validator %s is not tombstoned", msg.ValidatorAddress)
	}

	// Clear tombstone.
	val.Tombstoned = false
	val.TombstonedAt = 0
	ms.SetValidator(ctx, val)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.staking.validator_untombstoned",
		sdk.NewAttribute("validator", msg.ValidatorAddress),
		sdk.NewAttribute("authority", msg.Authority),
		sdk.NewAttribute("reason", msg.Reason),
	))

	return &types.MsgUntombstoneValidatorResponse{}, nil
}
