package keeper

import (
	"encoding/json"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/research/types"
)

// ---------- Params ----------

func (k Keeper) SetParams(ctx sdk.Context, params *types.Params) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := proto.Marshal(params)
	_ = kvStore.Set(types.ParamsKey, bz)
}

func (k Keeper) GetParams(ctx sdk.Context) *types.Params {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.ParamsKey)
	if err != nil || bz == nil {
		p := types.DefaultParams()
		return &p
	}
	var params types.Params
	proto.Unmarshal(bz, &params)
	return &params
}

// ---------- Research Submissions ----------

func submissionKey(id string) []byte {
	return append(types.SubmissionKeyPrefix, []byte(id)...)
}

func (k Keeper) SetResearch(ctx sdk.Context, r *types.Research) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := proto.Marshal(r)
	_ = kvStore.Set(submissionKey(r.Id), bz)
}

func (k Keeper) GetResearch(ctx sdk.Context, id string) (*types.Research, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(submissionKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var r types.Research
	proto.Unmarshal(bz, &r)
	return &r, true
}

func (k Keeper) DeleteResearch(ctx sdk.Context, id string) {
	kvStore := k.storeService.OpenKVStore(ctx)
	_ = kvStore.Delete(submissionKey(id))
}

func (k Keeper) IterateResearches(ctx sdk.Context, cb func(*types.Research) bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	iter, err := kvStore.Iterator(types.SubmissionKeyPrefix, prefixEndBytes(types.SubmissionKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var r types.Research
		proto.Unmarshal(iter.Value(), &r)
		if cb(&r) {
			break
		}
	}
}

func (k Keeper) GetResearchesByStatus(ctx sdk.Context, status types.ResearchStatus) []*types.Research {
	var results []*types.Research
	k.IterateResearches(ctx, func(r *types.Research) bool {
		if r.Status == string(status) {
			results = append(results, r)
		}
		return false
	})
	return results
}

func (k Keeper) GetResearchesByDomain(ctx sdk.Context, domain string) []*types.Research {
	var results []*types.Research
	k.IterateResearches(ctx, func(r *types.Research) bool {
		if r.Domain == domain {
			results = append(results, r)
		}
		return false
	})
	return results
}

// ---------- Bounties ----------

func bountyKey(id string) []byte {
	return append(types.BountyKeyPrefix, []byte(id)...)
}

func (k Keeper) SetBounty(ctx sdk.Context, b *types.Bounty) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := proto.Marshal(b)
	_ = kvStore.Set(bountyKey(b.Id), bz)
}

func (k Keeper) GetBounty(ctx sdk.Context, id string) (*types.Bounty, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(bountyKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var b types.Bounty
	proto.Unmarshal(bz, &b)
	return &b, true
}

func (k Keeper) DeleteBounty(ctx sdk.Context, id string) {
	kvStore := k.storeService.OpenKVStore(ctx)
	_ = kvStore.Delete(bountyKey(id))
}

func (k Keeper) IterateBounties(ctx sdk.Context, cb func(*types.Bounty) bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	iter, err := kvStore.Iterator(types.BountyKeyPrefix, prefixEndBytes(types.BountyKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var b types.Bounty
		proto.Unmarshal(iter.Value(), &b)
		if cb(&b) {
			break
		}
	}
}

func (k Keeper) GetActiveBounties(ctx sdk.Context) []*types.Bounty {
	var results []*types.Bounty
	k.IterateBounties(ctx, func(b *types.Bounty) bool {
		if b.Status == string(types.BountyStatusOpen) || b.Status == string(types.BountyStatusClaimed) {
			results = append(results, b)
		}
		return false
	})
	return results
}

// ExpireBounties expires bounties past deadline and returns locked rewards to creators.
// Claimed bounties past deadline are reopened so another agent can claim them.
func (k Keeper) ExpireBounties(ctx sdk.Context) {
	currentBlock := uint64(ctx.BlockHeight())
	k.IterateBounties(ctx, func(b *types.Bounty) bool {
		if currentBlock <= b.DeadlineHeight {
			return false
		}

		switch b.Status {
		case string(types.BountyStatusOpen):
			rewardInt := new(big.Int)
			if _, ok := rewardInt.SetString(b.Reward, 10); ok && rewardInt.Sign() > 0 {
				if creatorAddr, err := sdk.AccAddressFromBech32(b.Creator); err == nil {
					returnCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(rewardInt)))
					if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, creatorAddr, returnCoins); err != nil {
						k.Logger(ctx).Error("failed to return expired bounty reward",
							"bounty_id", b.Id, "error", err)
					}
				}
			}

			b.Status = string(types.BountyStatusExpired)
			k.SetBounty(ctx, b)

			ctx.EventManager().EmitEvent(
				sdk.NewEvent(
					"zerone.research.bounty_expired",
					sdk.NewAttribute("bounty_id", b.Id),
					sdk.NewAttribute("reward_returned", b.Reward),
					sdk.NewAttribute("creator", b.Creator),
				),
			)

		case string(types.BountyStatusClaimed):
			formerClaimer := b.ClaimedBy
			b.Status = string(types.BountyStatusOpen)
			b.ClaimedBy = ""
			k.SetBounty(ctx, b)

			ctx.EventManager().EmitEvent(
				sdk.NewEvent(
					"zerone.research.bounty_claim_expired",
					sdk.NewAttribute("bounty_id", b.Id),
					sdk.NewAttribute("former_claimer", formerClaimer),
				),
			)
		}

		return false
	})
}

// ---------- Peer Reviews ----------

func reviewKey(researchId, reviewId string) []byte {
	return append(types.PeerReviewKeyPrefix, []byte(researchId+"/"+reviewId)...)
}

func reviewPrefixForResearch(researchId string) []byte {
	return append(types.PeerReviewKeyPrefix, []byte(researchId+"/")...)
}

func (k Keeper) SetPeerReview(ctx sdk.Context, r *types.PeerReview) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := proto.Marshal(r)
	_ = kvStore.Set(reviewKey(r.ResearchId, r.Id), bz)
}

func (k Keeper) GetPeerReview(ctx sdk.Context, researchId, reviewId string) (*types.PeerReview, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(reviewKey(researchId, reviewId))
	if err != nil || bz == nil {
		return nil, false
	}
	var r types.PeerReview
	proto.Unmarshal(bz, &r)
	return &r, true
}

func (k Keeper) GetReviewsForResearch(ctx sdk.Context, researchId string) []*types.PeerReview {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := reviewPrefixForResearch(researchId)
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var results []*types.PeerReview
	for ; iter.Valid(); iter.Next() {
		var r types.PeerReview
		proto.Unmarshal(iter.Value(), &r)
		results = append(results, &r)
	}
	return results
}

func (k Keeper) HasReviewerReviewed(ctx sdk.Context, researchId, reviewer string) bool {
	reviews := k.GetReviewsForResearch(ctx, researchId)
	for _, r := range reviews {
		if r.Reviewer == reviewer {
			return true
		}
	}
	return false
}

// ---------- Treasury ----------

func (k Keeper) SetTreasuryBalance(ctx sdk.Context, balance string) {
	kvStore := k.storeService.OpenKVStore(ctx)
	tb := types.TreasuryBalance{Balance: balance}
	bz, _ := proto.Marshal(&tb)
	_ = kvStore.Set(types.TreasuryKeyPrefix, bz)
}

// GetTreasuryBalance returns the explicitly funded treasury balance.
// This tracks deposits made via FundResearch — it is DISTINCT from the
// stake escrow held in the same module account (those are not treasury funds).
//
// For the consolidated view including billing's research_fund allocation, use
// GetConsolidatedResearchBalance which reads both sources from the bank.
func (k Keeper) GetTreasuryBalance(ctx sdk.Context) string {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.TreasuryKeyPrefix)
	if err != nil || bz == nil {
		return "0"
	}
	var tb types.TreasuryBalance
	proto.Unmarshal(bz, &tb)
	return tb.Balance
}

// GetBillingResearchFundBalance returns the balance of the billing module's
// research_fund account — the ResearchBps allocation from each billing payment.
// This was previously invisible to x/research (Gap 6 — dual accounting).
func (k Keeper) GetBillingResearchFundBalance(ctx sdk.Context) string {
	coin := k.bankKeeper.GetModuleBalance(ctx, "research_fund", "uzrn")
	return coin.Amount.String()
}

// GetConsolidatedResearchBalance returns the sum of explicit treasury funds
// (FundResearch deposits) and the billing research_fund allocation.
// This is the true picture of research funding across both sources — it
// eliminates the dual-accounting gap where research_fund was invisible to
// the research module (Gap 6).
func (k Keeper) GetConsolidatedResearchBalance(ctx sdk.Context) string {
	explicit, ok := sdkmath.NewIntFromString(k.GetTreasuryBalance(ctx))
	if !ok {
		explicit = sdkmath.ZeroInt()
	}
	billing := k.bankKeeper.GetModuleBalance(ctx, "research_fund", "uzrn").Amount
	return explicit.Add(billing).String()
}

// ---------- Counters ----------

var (
	researchCounterKey = []byte("research_counter")
	bountyCounterKey   = []byte("bounty_counter")
)

func (k Keeper) nextResearchId(ctx sdk.Context) string {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := kvStore.Get(researchCounterKey)
	var counter uint64
	if bz != nil {
		json.Unmarshal(bz, &counter)
	}
	counter++
	bz, _ = json.Marshal(counter)
	_ = kvStore.Set(researchCounterKey, bz)
	return fmt.Sprintf("RES-%d", counter)
}

func (k Keeper) nextBountyId(ctx sdk.Context) string {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, _ := kvStore.Get(bountyCounterKey)
	var counter uint64
	if bz != nil {
		json.Unmarshal(bz, &counter)
	}
	counter++
	bz, _ = json.Marshal(counter)
	_ = kvStore.Set(bountyCounterKey, bz)
	return fmt.Sprintf("BOUNTY-%d", counter)
}

func (k Keeper) nextReviewId(ctx sdk.Context) string {
	kvStore := k.storeService.OpenKVStore(ctx)
	key := []byte("review_counter")
	bz, _ := kvStore.Get(key)
	var counter uint64
	if bz != nil {
		json.Unmarshal(bz, &counter)
	}
	counter++
	bz, _ = json.Marshal(counter)
	_ = kvStore.Set(key, bz)
	return fmt.Sprintf("REV-%d", counter)
}
