package keeper

import (
	"context"
	"fmt"
	"math"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/billing/types"
)

// Module account names for revenue distribution.
const (
	ProtocolTreasury    = "treasury_protocol"
	ResearchFund        = "research_fund"
	DevelopmentFund     = "development_fund"
	KnowledgeModuleName = "knowledge"

	// Protocol allocation sub-split (bps on 1M scale, relative to the protocol allocation).
	KnowledgePoolShareOfProtocolBps    = 500000 // 50% of protocol → citation rewards
	VerificationPoolShareOfProtocolBps = 300000 // 30% of protocol → verification pool
	// Remaining 20% → protocol treasury (governance, operations)
)

// CalculateDistribution computes the payment distribution using governance-adjustable RevenueSplit.
func (k Keeper) CalculateDistribution(ctx context.Context, totalPayment *big.Int, factIds []string) *types.PaymentDistribution {
	params := k.GetParams(ctx)
	split := params.RevenueSplit
	if split == nil {
		split = types.DefaultRevenueSplit()
	}

	// Top-level split using 1M bps basis
	providerShare := new(big.Int).Mul(totalPayment, big.NewInt(int64(split.ContributorBps)))
	providerShare.Div(providerShare, bpsBasis)

	protocolAllocation := new(big.Int).Mul(totalPayment, big.NewInt(int64(split.ProtocolBps)))
	protocolAllocation.Div(protocolAllocation, bpsBasis)

	researchShare := new(big.Int).Mul(totalPayment, big.NewInt(int64(split.ResearchBps)))
	researchShare.Div(researchShare, bpsBasis)

	// Burn = remainder (absorbs rounding dust)
	burnTotal := new(big.Int).Set(totalPayment)
	burnTotal.Sub(burnTotal, providerShare)
	burnTotal.Sub(burnTotal, protocolAllocation)
	burnTotal.Sub(burnTotal, researchShare)

	// Split protocol allocation into 3 sub-pools
	citationPoolTotal := new(big.Int).Mul(protocolAllocation, big.NewInt(KnowledgePoolShareOfProtocolBps))
	citationPoolTotal.Div(citationPoolTotal, bpsBasis)

	verificationPoolTotal := new(big.Int).Mul(protocolAllocation, big.NewInt(VerificationPoolShareOfProtocolBps))
	verificationPoolTotal.Div(verificationPoolTotal, bpsBasis)

	protocolTreasury := new(big.Int).Set(protocolAllocation)
	protocolTreasury.Sub(protocolTreasury, citationPoolTotal)
	protocolTreasury.Sub(protocolTreasury, verificationPoolTotal)

	// Knowledge pool (citation rewards) weighted by confidence * log2(citations+1)
	knowledgePool := k.calculateKnowledgePoolDistribution(ctx, citationPoolTotal, factIds)

	return &types.PaymentDistribution{
		TotalPayment:     totalPayment.String(),
		ResearchShare:    researchShare.String(),
		ProviderShare:    providerShare.String(),
		KnowledgePool:    knowledgePool,
		ProtocolBurn:     burnTotal.String(),
		ProtocolTreasury: protocolTreasury.String(),
	}
}

// calculateKnowledgePoolDistribution distributes the knowledge pool share
// weighted by confidence * log2(citations+1) for each fact.
func (k Keeper) calculateKnowledgePoolDistribution(ctx context.Context, poolTotal *big.Int, factIds []string) []*types.KnowledgePoolEntry {
	type factWeight struct {
		factId    string
		submitter string
		weight    *big.Int
	}

	var entries []factWeight
	totalWeight := new(big.Int)

	for _, factId := range factIds {
		submitter, found := k.knowledgeKeeper.GetFactSubmitter(ctx, factId)
		if !found {
			continue
		}

		confidence, _ := k.knowledgeKeeper.GetFactConfidence(ctx, factId)
		citations, _ := k.knowledgeKeeper.GetFactCitationCount(ctx, factId)

		// weight = confidence * floor(log2(citations+1) + 1)
		logFactor := uint64(math.Log2(float64(citations+1))) + 1
		weight := new(big.Int).Mul(big.NewInt(int64(confidence)), big.NewInt(int64(logFactor)))

		if weight.Sign() <= 0 {
			weight.SetInt64(1) // minimum weight
		}

		entries = append(entries, factWeight{
			factId:    factId,
			submitter: submitter,
			weight:    weight,
		})
		totalWeight.Add(totalWeight, weight)
	}

	var pool []*types.KnowledgePoolEntry

	if totalWeight.Sign() == 0 || len(entries) == 0 {
		// No eligible facts; entire knowledge pool goes to protocol treasury
		pool = append(pool, &types.KnowledgePoolEntry{
			FactId:    "",
			Submitter: ProtocolTreasury,
			Amount:    poolTotal.String(),
			Weight:    "0",
		})
		return pool
	}

	distributed := new(big.Int)
	for i, entry := range entries {
		var amount *big.Int
		if i == len(entries)-1 {
			// Last entry gets remainder to avoid rounding loss
			amount = new(big.Int).Sub(poolTotal, distributed)
		} else {
			amount = new(big.Int).Mul(poolTotal, entry.weight)
			amount.Div(amount, totalWeight)
		}
		distributed.Add(distributed, amount)

		pool = append(pool, &types.KnowledgePoolEntry{
			FactId:    entry.factId,
			Submitter: entry.submitter,
			Amount:    amount.String(),
			Weight:    entry.weight.String(),
		})
	}

	return pool
}

// ExecuteDistribution performs the atomic transfers for a payment distribution.
func (k Keeper) ExecuteDistribution(ctx context.Context, callerAddr sdk.AccAddress, providerAddr sdk.AccAddress, distribution *types.PaymentDistribution) error {
	// 1. Research fund — route through depositor for founder auto-split
	researchAmt := new(big.Int)
	researchAmt.SetString(distribution.ResearchShare, 10)
	if researchAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(researchAmt)))
		// Two-step: escrow to billing module, then route through depositor
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, callerAddr, types.ModuleName, coins); err != nil {
			return fmt.Errorf("research fund escrow failed: %w", err)
		}
		if err := k.researchFundDepositor.DepositToResearchFund(ctx, types.ModuleName, coins); err != nil {
			return fmt.Errorf("research fund deposit failed: %w", err)
		}
	}

	// 2. Provider share directly
	providerAmt := new(big.Int)
	providerAmt.SetString(distribution.ProviderShare, 10)
	if providerAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(providerAmt)))
		if err := k.bankKeeper.SendCoins(ctx, callerAddr, providerAddr, coins); err != nil {
			return fmt.Errorf("provider payment failed: %w", err)
		}
	}

	// 3. Knowledge pool to fact submitters
	for _, entry := range distribution.KnowledgePool {
		amt := new(big.Int)
		amt.SetString(entry.Amount, 10)
		if amt.Sign() <= 0 {
			continue
		}
		if entry.Submitter == ProtocolTreasury {
			coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
			if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, callerAddr, ProtocolTreasury, coins); err != nil {
				return fmt.Errorf("knowledge pool fallback failed: %w", err)
			}
			continue
		}
		submitterAddr, err := sdk.AccAddressFromBech32(entry.Submitter)
		if err != nil {
			continue // skip invalid addresses
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
		if err := k.bankKeeper.SendCoins(ctx, callerAddr, submitterAddr, coins); err != nil {
			return fmt.Errorf("knowledge pool payment to %s failed: %w", entry.Submitter, err)
		}
	}

	// 4. Protocol burn (rounding dust — escrow into billing then destroy).
	devAmt := new(big.Int)
	devAmt.SetString(distribution.ProtocolBurn, 10)
	if devAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(devAmt)))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, callerAddr, types.ModuleName, coins); err != nil {
			return fmt.Errorf("protocol burn escrow failed: %w", err)
		}
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
			return fmt.Errorf("protocol burn failed: %w", err)
		}
	}

	// 5. Protocol treasury
	treasuryAmt := new(big.Int)
	treasuryAmt.SetString(distribution.ProtocolTreasury, 10)
	if treasuryAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(treasuryAmt)))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, callerAddr, ProtocolTreasury, coins); err != nil {
			return fmt.Errorf("treasury transfer failed: %w", err)
		}
	}

	// 6. Verification pool → knowledge module account
	totalAmt := new(big.Int)
	totalAmt.SetString(distribution.TotalPayment, 10)

	knowledgePoolSum := new(big.Int)
	for _, entry := range distribution.KnowledgePool {
		amt := new(big.Int)
		amt.SetString(entry.Amount, 10)
		knowledgePoolSum.Add(knowledgePoolSum, amt)
	}

	verificationAmt := new(big.Int).Set(totalAmt)
	verificationAmt.Sub(verificationAmt, providerAmt)
	verificationAmt.Sub(verificationAmt, researchAmt)
	verificationAmt.Sub(verificationAmt, knowledgePoolSum)
	verificationAmt.Sub(verificationAmt, devAmt)
	verificationAmt.Sub(verificationAmt, treasuryAmt)

	if verificationAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(verificationAmt)))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, callerAddr, KnowledgeModuleName, coins); err != nil {
			return fmt.Errorf("verification pool transfer failed: %w", err)
		}
	}

	return nil
}
