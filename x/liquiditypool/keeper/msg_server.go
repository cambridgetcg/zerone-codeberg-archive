package keeper

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/liquiditypool/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	Keeper
}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// CreatePool creates a new constant-product AMM pool.
// Only governance (authority) can create pools.
func (m msgServer) CreatePool(goCtx context.Context, msg *types.MsgCreatePool) (*types.MsgCreatePoolResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	params := m.Keeper.GetParams(ctx)

	// Authority check: only governance can create pools
	if m.Keeper.GetAuthority() != msg.Creator {
		return nil, types.ErrUnauthorized
	}

	// Max pools check (0 = unlimited)
	if params.MaxPools > 0 && m.Keeper.CountPools(ctx) >= params.MaxPools {
		return nil, types.ErrMaxPoolsReached
	}

	// Check no existing pool for this denom pair
	if existing := m.Keeper.GetPoolByDenoms(ctx, msg.DenomA, msg.DenomB); existing != nil {
		return nil, types.ErrPoolAlreadyExists
	}

	amountA := new(big.Int)
	if _, ok := amountA.SetString(msg.AmountA, 10); !ok || amountA.Sign() <= 0 {
		return nil, types.ErrZeroAmount
	}
	amountB := new(big.Int)
	if _, ok := amountB.SetString(msg.AmountB, 10); !ok || amountB.Sign() <= 0 {
		return nil, types.ErrZeroAmount
	}

	// Minimum initial liquidity check
	minLiq := new(big.Int)
	minLiq.SetString(params.MinInitialLiquidity, 10)
	if amountA.Cmp(minLiq) < 0 && amountB.Cmp(minLiq) < 0 {
		return nil, types.ErrInsufficientLiquidity
	}

	feeBps := msg.SwapFeeBps
	if feeBps == 0 {
		feeBps = params.DefaultSwapFeeBps
	}

	// Validate per-pool swap fraction override (Market Microstructure Gap 4).
	// 0 = use global default (allowed); 1–1_000_000 = valid override.
	if msg.PoolMaxSwapFractionBps > 1_000_000 {
		return nil, types.ErrInvalidPoolSwapFraction
	}

	// Transfer initial liquidity from creator to module
	creatorAddr, _ := sdk.AccAddressFromBech32(msg.Creator)
	coinsA := sdk.NewCoins(sdk.NewCoin(msg.DenomA, sdkmath.NewIntFromBigInt(amountA)))
	coinsB := sdk.NewCoins(sdk.NewCoin(msg.DenomB, sdkmath.NewIntFromBigInt(amountB)))

	if m.Keeper.bankKeeper != nil {
		if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, creatorAddr, types.ModuleName, coinsA); err != nil {
			return nil, fmt.Errorf("transfer denom_a failed: %w", err)
		}
		if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, creatorAddr, types.ModuleName, coinsB); err != nil {
			return nil, fmt.Errorf("transfer denom_b failed: %w", err)
		}
	}

	// Assign pool ID
	counter := m.Keeper.IncrementPoolCounter(ctx)
	poolId := fmt.Sprintf("pool-%d", counter)

	// Initial LP tokens = sqrt(amountA * amountB)
	lpTokens := CalculateLPTokensForDeposit(amountA, amountB, amountA, amountB, new(big.Int))

	pool := &types.Pool{
		PoolId:                  poolId,
		DenomA:                  msg.DenomA,
		DenomB:                  msg.DenomB,
		ReserveA:                amountA.String(),
		ReserveB:                amountB.String(),
		SwapFeeBps:              feeBps,
		LpTokenSupply:           lpTokens.String(),
		LpDenom:                 types.LPDenom(poolId),
		Creator:                 msg.Creator,
		CreatedAtBlock:          uint64(ctx.BlockHeight()),
		PoolMaxSwapFractionBps:  msg.PoolMaxSwapFractionBps,
	}
	m.Keeper.SetPool(ctx, pool)

	// Mint LP tokens to creator
	if m.Keeper.bankKeeper != nil {
		lpDenom := types.LPDenom(poolId)
		lpCoins := sdk.NewCoins(sdk.NewCoin(lpDenom, sdkmath.NewIntFromBigInt(lpTokens)))
		if err := m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to mint LP tokens: %w", err)
		}
		if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, creatorAddr, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to send LP tokens: %w", err)
		}
	}

	// Initialize TWAP accumulator
	m.Keeper.SetTWAPAccumulator(ctx, &types.TWAPAccumulator{
		PoolId:       poolId,
		LastBlock:    uint64(ctx.BlockHeight()),
		CumPriceAToB: "0",
		CumPriceBToA: "0",
	})

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.pool_created",
		sdk.NewAttribute("pool_id", poolId),
		sdk.NewAttribute("denom_a", msg.DenomA),
		sdk.NewAttribute("denom_b", msg.DenomB),
		sdk.NewAttribute("reserve_a", amountA.String()),
		sdk.NewAttribute("reserve_b", amountB.String()),
		sdk.NewAttribute("lp_tokens", lpTokens.String()),
	))

	return &types.MsgCreatePoolResponse{PoolId: poolId}, nil
}

// Swap executes a constant-product swap through a pool.
func (m msgServer) Swap(goCtx context.Context, msg *types.MsgSwap) (*types.MsgSwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	pool, found := m.Keeper.GetPool(ctx, msg.PoolId)
	if !found {
		return nil, types.ErrPoolNotFound
	}

	if pool.Locked {
		return nil, types.ErrPoolLocked
	}

	// Deadline check: if DeadlineBlock is set, the swap must execute at or before that block
	if msg.DeadlineBlock > 0 && uint64(ctx.BlockHeight()) > msg.DeadlineBlock {
		return nil, types.ErrSwapExpired
	}

	// Determine swap direction
	tokenIn := new(big.Int)
	if _, ok := tokenIn.SetString(msg.TokenInAmount, 10); !ok || tokenIn.Sign() <= 0 {
		return nil, types.ErrZeroAmount
	}

	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)

	var reserveIn, reserveOut *big.Int
	var denomOut string

	switch msg.TokenInDenom {
	case pool.DenomA:
		reserveIn = reserveA
		reserveOut = reserveB
		denomOut = pool.DenomB
	case pool.DenomB:
		reserveIn = reserveB
		reserveOut = reserveA
		denomOut = pool.DenomA
	default:
		return nil, types.ErrDenomNotInPool
	}

	// Max swap fraction check — per-pool override takes precedence over global.
	// Precedence: pool.PoolMaxSwapFractionBps (if > 0) > Params.MaxSwapFractionBps.
	// Both 0 = check disabled (Market Microstructure Gap 4 + existing Gap 5).
	params := m.Keeper.GetParams(ctx)
	effectiveFractionBps := params.MaxSwapFractionBps
	if pool.PoolMaxSwapFractionBps > 0 {
		effectiveFractionBps = pool.PoolMaxSwapFractionBps
	}
	if effectiveFractionBps > 0 {
		maxIn := new(big.Int).Mul(reserveIn, new(big.Int).SetUint64(effectiveFractionBps))
		maxIn.Div(maxIn, big.NewInt(1_000_000))
		if tokenIn.Cmp(maxIn) > 0 {
			return nil, types.ErrSwapFractionExceeded
		}
	}

	// TWAP deviation circuit breaker (Oracle Resilience Gap 5).
	// Blocks swaps when spot price has diverged significantly from the TWAP,
	// bounding TWAP manipulation cost. If the TWAP has no history yet the check
	// is skipped (fail-open — pool must be usable before TWAP history accumulates).
	// Gate: disabled when max_twap_deviation_bps == 0 (default).
	if params.MaxTwapDeviationBps > 0 {
		twap, _, twapErr := m.Keeper.GetTWAP(ctx, pool.PoolId, pool.DenomA, 0)
		if twapErr == nil && twap != nil && twap.Sign() > 0 {
			// Spot price of DenomA in DenomB terms, same 1e6 scale as GetTWAP output.
			spotScaled := new(big.Int).Mul(reserveB, big.NewInt(1_000_000))
			spotScaled.Div(spotScaled, reserveA)

			// deviation = |spot - twap| * 1_000_000 / twap  (result in 1M-BPS units)
			diff := new(big.Int).Sub(spotScaled, twap)
			if diff.Sign() < 0 {
				diff.Neg(diff)
			}
			deviationBps := new(big.Int).Mul(diff, big.NewInt(1_000_000))
			deviationBps.Div(deviationBps, twap)

			if deviationBps.Uint64() > params.MaxTwapDeviationBps {
				return nil, types.ErrTWAPDeviationExceeded
			}
		}
	}

	// Lock pool for this transaction
	pool.Locked = true
	m.Keeper.SetPool(ctx, pool)

	// Calculate swap output
	tokenOut, feeAmount := CalculateSwapOutput(reserveIn, reserveOut, tokenIn, pool.SwapFeeBps)

	// Slippage check
	minOut := new(big.Int)
	if msg.MinTokenOut != "" {
		minOut.SetString(msg.MinTokenOut, 10)
	}
	if minOut.Sign() > 0 && tokenOut.Cmp(minOut) < 0 {
		// Unlock and return error
		pool.Locked = false
		m.Keeper.SetPool(ctx, pool)
		return nil, types.ErrSlippageExceeded
	}

	// Min reserve check after swap
	newReserveOut := new(big.Int).Sub(reserveOut, tokenOut)
	if params.MinReserve != "" {
		minReserve := new(big.Int)
		minReserve.SetString(params.MinReserve, 10)
		if newReserveOut.Cmp(minReserve) < 0 {
			pool.Locked = false
			m.Keeper.SetPool(ctx, pool)
			return nil, types.ErrReserveBelowMinimum
		}
	}

	// Transfer tokens
	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)
	if m.Keeper.bankKeeper != nil {
		// Sender -> module (input)
		inCoins := sdk.NewCoins(sdk.NewCoin(msg.TokenInDenom, sdkmath.NewIntFromBigInt(tokenIn)))
		if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, inCoins); err != nil {
			pool.Locked = false
			m.Keeper.SetPool(ctx, pool)
			return nil, fmt.Errorf("input transfer failed: %w", err)
		}
		// Module -> sender (output)
		outCoins := sdk.NewCoins(sdk.NewCoin(denomOut, sdkmath.NewIntFromBigInt(tokenOut)))
		if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, outCoins); err != nil {
			pool.Locked = false
			m.Keeper.SetPool(ctx, pool)
			return nil, fmt.Errorf("output transfer failed: %w", err)
		}
	}

	// Update reserves: input reserve increases by tokenIn, output reserve decreases by tokenOut
	newReserveIn := new(big.Int).Add(reserveIn, tokenIn)

	if msg.TokenInDenom == pool.DenomA {
		pool.ReserveA = newReserveIn.String()
		pool.ReserveB = newReserveOut.String()
	} else {
		pool.ReserveB = newReserveIn.String()
		pool.ReserveA = newReserveOut.String()
	}

	// Unlock
	pool.Locked = false
	m.Keeper.SetPool(ctx, pool)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.swap",
		sdk.NewAttribute("pool_id", msg.PoolId),
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("token_in", msg.TokenInDenom),
		sdk.NewAttribute("amount_in", tokenIn.String()),
		sdk.NewAttribute("token_out", denomOut),
		sdk.NewAttribute("amount_out", tokenOut.String()),
		sdk.NewAttribute("fee", feeAmount.String()),
	))

	return &types.MsgSwapResponse{
		TokenOutAmount: tokenOut.String(),
		FeeAmount:      feeAmount.String(),
	}, nil
}

// AddLiquidity adds liquidity to an existing pool proportionally.
func (m msgServer) AddLiquidity(goCtx context.Context, msg *types.MsgAddLiquidity) (*types.MsgAddLiquidityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	pool, found := m.Keeper.GetPool(ctx, msg.PoolId)
	if !found {
		return nil, types.ErrPoolNotFound
	}

	desiredA := new(big.Int)
	desiredA.SetString(msg.AmountA, 10)
	desiredB := new(big.Int)
	desiredB.SetString(msg.AmountB, 10)

	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)
	totalSupply := new(big.Int)
	totalSupply.SetString(pool.LpTokenSupply, 10)

	// Calculate proportional deposit
	actualA, actualB := CalculateProportionalDeposit(reserveA, reserveB, desiredA, desiredB)

	// Calculate LP tokens
	lpTokens := CalculateLPTokensForDeposit(reserveA, reserveB, actualA, actualB, totalSupply)

	// Min LP check
	if msg.MinLpTokens != "" {
		minLP := new(big.Int)
		minLP.SetString(msg.MinLpTokens, 10)
		if minLP.Sign() > 0 && lpTokens.Cmp(minLP) < 0 {
			return nil, types.ErrSlippageExceeded
		}
	}

	// Settle liquidity mining rewards BEFORE LP balance changes.
	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)
	if m.Keeper.bankKeeper != nil {
		lpDenomForMining := types.LPDenom(pool.PoolId)
		currentLpBal := m.Keeper.bankKeeper.GetBalance(ctx, senderAddr, lpDenomForMining)
		m.Keeper.SettleMinerRewards(ctx, pool.PoolId, msg.Sender, currentLpBal.Amount.BigInt(), pool)
	}

	// Record deposit block for anti-JIT cooldown (Market Liquidity Gap 5).
	// Updated on every AddLiquidity call — the clock resets to the latest deposit.
	m.Keeper.SetLPDepositBlock(ctx, pool.PoolId, msg.Sender, uint64(ctx.BlockHeight()))

	// Record deposit price for IL insurance on the FIRST AddLiquidity only (Liquidity Gap 3).
	// Price = reserveB * 1e12 / reserveA using pre-deposit reserves.
	// Only set for pools containing uzrn; only set once (subsequent deposits do not change it).
	if (pool.DenomA == "uzrn" || pool.DenomB == "uzrn") && reserveA.Sign() > 0 {
		pos := m.Keeper.GetLpMinerPosition(ctx, pool.PoolId, msg.Sender)
		if pos.DepositPriceAPerB == "" {
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil)
			depositPrice := new(big.Int).Mul(reserveB, scale)
			depositPrice.Div(depositPrice, reserveA)
			pos.DepositPriceAPerB = depositPrice.String()
			m.Keeper.SetLpMinerPosition(ctx, pos)
		}
	}

	// Transfer tokens
	if m.Keeper.bankKeeper != nil {
		if actualA.Sign() > 0 {
			coinsA := sdk.NewCoins(sdk.NewCoin(pool.DenomA, sdkmath.NewIntFromBigInt(actualA)))
			if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, coinsA); err != nil {
				return nil, fmt.Errorf("transfer denom_a failed: %w", err)
			}
		}
		if actualB.Sign() > 0 {
			coinsB := sdk.NewCoins(sdk.NewCoin(pool.DenomB, sdkmath.NewIntFromBigInt(actualB)))
			if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, coinsB); err != nil {
				return nil, fmt.Errorf("transfer denom_b failed: %w", err)
			}
		}
	}

	// Update reserves and LP supply
	newReserveA := new(big.Int).Add(reserveA, actualA)
	newReserveB := new(big.Int).Add(reserveB, actualB)
	newSupply := new(big.Int).Add(totalSupply, lpTokens)

	pool.ReserveA = newReserveA.String()
	pool.ReserveB = newReserveB.String()
	pool.LpTokenSupply = newSupply.String()
	m.Keeper.SetPool(ctx, pool)

	// Mint LP tokens
	if m.Keeper.bankKeeper != nil && lpTokens.Sign() > 0 {
		lpDenom := types.LPDenom(pool.PoolId)
		lpCoins := sdk.NewCoins(sdk.NewCoin(lpDenom, sdkmath.NewIntFromBigInt(lpTokens)))
		if err := m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to mint LP tokens: %w", err)
		}
		if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to send LP tokens: %w", err)
		}
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.liquidity_added",
		sdk.NewAttribute("pool_id", msg.PoolId),
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("amount_a", actualA.String()),
		sdk.NewAttribute("amount_b", actualB.String()),
		sdk.NewAttribute("lp_tokens", lpTokens.String()),
	))

	return &types.MsgAddLiquidityResponse{
		LpTokensMinted: lpTokens.String(),
		ActualA:        actualA.String(),
		ActualB:        actualB.String(),
	}, nil
}

// RemoveLiquidity burns LP tokens and returns underlying assets.
func (m msgServer) RemoveLiquidity(goCtx context.Context, msg *types.MsgRemoveLiquidity) (*types.MsgRemoveLiquidityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	pool, found := m.Keeper.GetPool(ctx, msg.PoolId)
	if !found {
		return nil, types.ErrPoolNotFound
	}

	lpTokens := new(big.Int)
	if _, ok := lpTokens.SetString(msg.LpTokens, 10); !ok || lpTokens.Sign() <= 0 {
		return nil, types.ErrZeroAmount
	}

	// Anti-JIT cooldown check (Liquidity Gap 5).
	// If MinLiquidityDurationBlocks > 0, the sender must have waited at least that
	// many blocks since their last AddLiquidity before they can remove liquidity.
	// Missing deposit record (depositBlock == 0) means position pre-dates the param
	// — cooldown does not apply (backward compatible).
	params := m.Keeper.GetParams(ctx)
	if params.MinLiquidityDurationBlocks > 0 {
		depositBlock, recorded := m.Keeper.GetLPDepositBlock(ctx, pool.PoolId, msg.Sender)
		if recorded {
			elapsed := uint64(ctx.BlockHeight()) - depositBlock
			if elapsed < params.MinLiquidityDurationBlocks {
				return nil, types.ErrWithdrawalTooEarly
			}
		}
	}

	totalSupply := new(big.Int)
	totalSupply.SetString(pool.LpTokenSupply, 10)

	if lpTokens.Cmp(totalSupply) > 0 {
		return nil, types.ErrInsufficientLP
	}

	reserveA := new(big.Int)
	reserveA.SetString(pool.ReserveA, 10)
	reserveB := new(big.Int)
	reserveB.SetString(pool.ReserveB, 10)

	amountA, amountB := CalculateWithdrawalAmounts(reserveA, reserveB, lpTokens, totalSupply)

	// Slippage checks
	if msg.MinAmountA != "" {
		minA := new(big.Int)
		minA.SetString(msg.MinAmountA, 10)
		if minA.Sign() > 0 && amountA.Cmp(minA) < 0 {
			return nil, types.ErrSlippageExceeded
		}
	}
	if msg.MinAmountB != "" {
		minB := new(big.Int)
		minB.SetString(msg.MinAmountB, 10)
		if minB.Sign() > 0 && amountB.Cmp(minB) < 0 {
			return nil, types.ErrSlippageExceeded
		}
	}

	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)

	// Settle liquidity mining rewards BEFORE LP balance changes.
	// lpTokens = amount being removed; full balance = lpTokens at minimum (may be more).
	// Use actual bank balance to capture any transfers since last settlement.
	if m.Keeper.bankKeeper != nil {
		lpDenomForMining := types.LPDenom(pool.PoolId)
		currentLpBal := m.Keeper.bankKeeper.GetBalance(ctx, senderAddr, lpDenomForMining)
		m.Keeper.SettleMinerRewards(ctx, pool.PoolId, msg.Sender, currentLpBal.Amount.BigInt(), pool)
	}

	// Burn LP tokens: user -> module -> burn
	if m.Keeper.bankKeeper != nil {
		lpDenom := types.LPDenom(pool.PoolId)
		lpCoins := sdk.NewCoins(sdk.NewCoin(lpDenom, sdkmath.NewIntFromBigInt(lpTokens)))
		if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to collect LP tokens: %w", err)
		}
		if err := m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, lpCoins); err != nil {
			return nil, fmt.Errorf("failed to burn LP tokens: %w", err)
		}

		// Return underlying assets
		if amountA.Sign() > 0 {
			coinsA := sdk.NewCoins(sdk.NewCoin(pool.DenomA, sdkmath.NewIntFromBigInt(amountA)))
			if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, coinsA); err != nil {
				return nil, fmt.Errorf("failed to return denom_a: %w", err)
			}
		}
		if amountB.Sign() > 0 {
			coinsB := sdk.NewCoins(sdk.NewCoin(pool.DenomB, sdkmath.NewIntFromBigInt(amountB)))
			if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, coinsB); err != nil {
				return nil, fmt.Errorf("failed to return denom_b: %w", err)
			}
		}
	}

	// MEV reserve payout: pro-rata uzrn from pool.MevReserveBalance.
	// lpMevShare = mevReserve × lpTokens / totalSupply.
	// Sent to LP recipient; reserve balance updated before pool save.
	mevShare := new(big.Int)
	if pool.MevReserveBalance != "" && pool.MevReserveBalance != "0" && m.Keeper.bankKeeper != nil {
		mevReserve := new(big.Int)
		if _, ok := mevReserve.SetString(pool.MevReserveBalance, 10); ok && mevReserve.Sign() > 0 {
			// integer division: floor(mevReserve × lpTokens / totalSupply)
			mevShare.Mul(mevReserve, lpTokens)
			mevShare.Div(mevShare, totalSupply)
			if mevShare.Sign() > 0 {
				mevCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(mevShare)))
				if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, senderAddr, mevCoins); err != nil {
					return nil, fmt.Errorf("failed to distribute MEV reserve to LP: %w", err)
				}
				pool.MevReserveBalance = new(big.Int).Sub(mevReserve, mevShare).String()
			}
		}
	}

	// IL insurance payout — computed from pre-withdrawal pool reserves (Liquidity Gap 3).
	// MaybePayILInsurance reads pool.ReserveA/B for the current price, so it must be
	// called BEFORE the pool state is updated below.  Fail-silent: insurance failure
	// never aborts a valid withdrawal.
	ilPayout := m.Keeper.MaybePayILInsurance(ctx, pool, senderAddr, amountA, amountB)

	// Update pool state
	newReserveA := new(big.Int).Sub(reserveA, amountA)
	newReserveB := new(big.Int).Sub(reserveB, amountB)
	newSupply := new(big.Int).Sub(totalSupply, lpTokens)

	pool.ReserveA = newReserveA.String()
	pool.ReserveB = newReserveB.String()
	pool.LpTokenSupply = newSupply.String()
	m.Keeper.SetPool(ctx, pool)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.liquidity_removed",
		sdk.NewAttribute("pool_id", msg.PoolId),
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("lp_tokens_burned", lpTokens.String()),
		sdk.NewAttribute("amount_a", amountA.String()),
		sdk.NewAttribute("amount_b", amountB.String()),
		sdk.NewAttribute("mev_reserve_payout", mevShare.String()),
		sdk.NewAttribute("il_insurance_payout", ilPayout.String()),
	))

	return &types.MsgRemoveLiquidityResponse{
		AmountA: amountA.String(),
		AmountB: amountB.String(),
	}, nil
}

// FundILInsurance deposits uzrn into the IL insurance fund. Any address may top up.
// Liquidity Gap 3.
func (m msgServer) FundILInsurance(goCtx context.Context, msg *types.MsgFundILInsurance) (*types.MsgFundILInsuranceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	amount := new(big.Int)
	if _, ok := amount.SetString(msg.Amount, 10); !ok || amount.Sign() <= 0 {
		return nil, types.ErrILInsuranceInvalidAmount
	}

	senderAddr, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	// Transfer amount from sender to module account
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amount)))
	if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to fund IL insurance: %w", err)
	}

	// Increase fund balance
	current := m.Keeper.GetILInsuranceFundBalance(ctx)
	newBalance := new(big.Int).Add(current, amount)
	m.Keeper.SetILInsuranceFundBalance(ctx, newBalance)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.liquiditypool.il_insurance_funded",
		sdk.NewAttribute("sender", msg.Sender),
		sdk.NewAttribute("amount", amount.String()),
		sdk.NewAttribute("new_balance", newBalance.String()),
	))

	return &types.MsgFundILInsuranceResponse{NewBalance: newBalance.String()}, nil
}

// UpdateParams updates module parameters. Only governance can call.
func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.Keeper.GetAuthority() != msg.Authority {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.Keeper.GetAuthority(), msg.Authority)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	m.Keeper.SetParams(ctx, msg.Params)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.liquiditypool.update_params",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}

// CreateLiquidityMiningPeriod creates a new emission schedule for LP token holders.
// Only governance (authority) may call this. At most one active period per pool.
func (m msgServer) CreateLiquidityMiningPeriod(goCtx context.Context, msg *types.MsgCreateLiquidityMiningPeriod) (*types.MsgCreateLiquidityMiningPeriodResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.Authority != m.Keeper.GetAuthority() {
		return nil, types.ErrUnauthorized
	}

	// Pool must exist
	if _, found := m.Keeper.GetPool(ctx, msg.PoolId); !found {
		return nil, types.ErrPoolNotFound
	}

	period, err := m.Keeper.CreateMiningPeriod(ctx, msg.PoolId, msg.Authority, msg.AmountPerBlock, msg.StartBlock, msg.EndBlock)
	if err != nil {
		return nil, err
	}

	return &types.MsgCreateLiquidityMiningPeriodResponse{PeriodId: period.Id}, nil
}

// ClaimLiquidityRewards settles and pays out accrued mining rewards for the caller.
func (m msgServer) ClaimLiquidityRewards(goCtx context.Context, msg *types.MsgClaimLiquidityRewards) (*types.MsgClaimLiquidityRewardsResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	pool, found := m.Keeper.GetPool(ctx, msg.PoolId)
	if !found {
		return nil, types.ErrPoolNotFound
	}

	claimed, err := m.Keeper.ClaimPendingRewards(ctx, msg.PoolId, msg.Sender, pool)
	if err != nil {
		return nil, err
	}

	return &types.MsgClaimLiquidityRewardsResponse{ClaimedAmount: claimed.String()}, nil
}

// CommitSwap stores a sha256 commitment for a swap — the parameters stay hidden until RevealSwap.
// This breaks the front-running window: by the time the reveal is visible in the mempool, the
// commit is already included in a block and the validator cannot sandwich the swap.
func (m msgServer) CommitSwap(goCtx context.Context, msg *types.MsgCommitSwap) (*types.MsgCommitSwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Pool must exist
	if _, found := m.Keeper.GetPool(ctx, msg.PoolId); !found {
		return nil, types.ErrPoolNotFound
	}

	// Commitment must be exactly 32 bytes (sha256 output)
	if len(msg.Commitment) != 32 {
		return nil, types.ErrInvalidCommitment
	}

	// Store (overwrite allowed — user may re-commit if they change their mind)
	m.Keeper.SetSwapCommitment(ctx, &types.SwapCommitment{
		Sender:      msg.Sender,
		PoolId:      msg.PoolId,
		Commitment:  msg.Commitment,
		CommitBlock: uint64(ctx.BlockHeight()),
	})

	return &types.MsgCommitSwapResponse{}, nil
}

// RevealSwap verifies a commitment and executes the swap.
// The nonce and all swap parameters must reproduce the stored sha256 hash exactly.
// One-use: the commitment is deleted after a successful reveal.
func (m msgServer) RevealSwap(goCtx context.Context, msg *types.MsgRevealSwap) (*types.MsgRevealSwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Load commitment
	commit, found := m.Keeper.GetSwapCommitment(ctx, msg.Sender, msg.PoolId)
	if !found {
		return nil, types.ErrNoCommitFound
	}

	// Delay check: if commit-reveal delay is configured, enforce it
	params := m.Keeper.GetParams(ctx)
	if params.CommitRevealDelayBlocks > 0 {
		revealBlock := uint64(ctx.BlockHeight())
		if revealBlock < commit.CommitBlock+params.CommitRevealDelayBlocks {
			return nil, types.ErrRevealTooEarly
		}
	}

	// Verify commitment: recompute hash from reveal parameters and compare
	expected := ComputeSwapCommitmentHash(
		msg.Nonce,
		msg.PoolId,
		msg.TokenInDenom,
		msg.TokenInAmount,
		msg.MinTokenOut,
		msg.DeadlineBlock,
	)
	if !bytes.Equal(expected, commit.Commitment) {
		return nil, types.ErrCommitmentMismatch
	}

	// Delete commitment before execution (one-use only; prevents replay even on error)
	m.Keeper.DeleteSwapCommitment(ctx, msg.Sender, msg.PoolId)

	// Fair sequencing (Market Microstructure Gap 5): if enabled, queue the swap
	// for EndBlocker execution in commit-block-ascending (FIFO) order rather than
	// executing it immediately in the current block's transaction ordering.
	if params.FairSequencingEnabled {
		m.Keeper.SetPendingSwapReveal(ctx, &types.PendingSwapReveal{
			Sender:          msg.Sender,
			PoolId:          msg.PoolId,
			TokenInDenom:    msg.TokenInDenom,
			TokenInAmount:   msg.TokenInAmount,
			MinTokenOut:     msg.MinTokenOut,
			DeadlineBlock:   msg.DeadlineBlock,
			CommitBlock:     commit.CommitBlock,
			PriorityTipUzrn: msg.PriorityTipUzrn,
		})
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.liquiditypool.swap_reveal_queued",
			sdk.NewAttribute("sender", msg.Sender),
			sdk.NewAttribute("pool_id", msg.PoolId),
			sdk.NewAttribute("commit_block", fmt.Sprintf("%d", commit.CommitBlock)),
		))
		return &types.MsgRevealSwapResponse{}, nil
	}

	// Execute the underlying swap
	swapResp, err := m.Swap(goCtx, &types.MsgSwap{
		Sender:        msg.Sender,
		PoolId:        msg.PoolId,
		TokenInDenom:  msg.TokenInDenom,
		TokenInAmount: msg.TokenInAmount,
		MinTokenOut:   msg.MinTokenOut,
		DeadlineBlock: msg.DeadlineBlock,
	})
	if err != nil {
		return nil, err
	}

	// Process MEV priority tip (Market Microstructure Gap 3).
	// The tip is NOT part of the commitment hash — users choose it at reveal time.
	// mev_fee_share_bps of the tip goes to the pool's LP MEV reserve; the rest is burned.
	mevLpShare := new(big.Int)
	if msg.PriorityTipUzrn != "" && msg.PriorityTipUzrn != "0" {
		tip := new(big.Int)
		if _, ok := tip.SetString(msg.PriorityTipUzrn, 10); !ok || tip.Sign() < 0 {
			return nil, types.ErrInvalidMEVTip
		}
		if tip.Sign() > 0 && m.Keeper.bankKeeper != nil {
			senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)
			tipCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(tip)))

			// Transfer tip from sender to module.
			if err := m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, tipCoins); err != nil {
				return nil, fmt.Errorf("failed to collect MEV priority tip: %w", err)
			}

			// Compute LP share and protocol share.
			params := m.Keeper.GetParams(ctx)
			if params.MevFeeShareBps > 0 {
				mevLpShare.Mul(tip, new(big.Int).SetUint64(params.MevFeeShareBps))
				mevLpShare.Div(mevLpShare, new(big.Int).SetUint64(1_000_000))
			}
			protocolShare := new(big.Int).Sub(tip, mevLpShare)

			// LP share: accumulate in pool MEV reserve.
			if mevLpShare.Sign() > 0 {
				pool, found := m.Keeper.GetPool(ctx, msg.PoolId)
				if found {
					reserve := new(big.Int)
					if pool.MevReserveBalance != "" {
						reserve.SetString(pool.MevReserveBalance, 10)
					}
					reserve.Add(reserve, mevLpShare)
					pool.MevReserveBalance = reserve.String()
					m.Keeper.SetPool(ctx, pool)
				}
			}

			// Protocol share: burn.
			if protocolShare.Sign() > 0 {
				protocolCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(protocolShare)))
				// Best-effort burn — swap already succeeded; don't fail it for protocol accounting.
				_ = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, protocolCoins)
			}

			ctx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.liquiditypool.mev_tip_processed",
				sdk.NewAttribute("pool_id", msg.PoolId),
				sdk.NewAttribute("sender", msg.Sender),
				sdk.NewAttribute("tip_total", tip.String()),
				sdk.NewAttribute("lp_share", mevLpShare.String()),
				sdk.NewAttribute("protocol_share", protocolShare.String()),
			))
		}
	}

	return &types.MsgRevealSwapResponse{
		TokenOutAmount: swapResp.TokenOutAmount,
		FeeAmount:      swapResp.FeeAmount,
		MevLpShare:     mevLpShare.String(),
	}, nil
}
