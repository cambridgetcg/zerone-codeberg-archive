package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

// Keeper manages the disputes module's state.
type Keeper struct {
	storeService    store.KVStoreService
	cdc             codec.BinaryCodec
	authority       string
	bankKeeper      types.BankKeeper
	stakingKeeper   types.StakingKeeper
	knowledgeKeeper types.KnowledgeKeeper
}

// NewKeeper creates a new disputes module Keeper.
func NewKeeper(
	storeService store.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	bk types.BankKeeper,
	sk types.StakingKeeper,
	kk types.KnowledgeKeeper,
) Keeper {
	return Keeper{
		storeService:    storeService,
		cdc:             cdc,
		authority:       authority,
		bankKeeper:      bk,
		stakingKeeper:   sk,
		knowledgeKeeper: kk,
	}
}

// Logger returns a module-scoped logger.
func (k Keeper) Logger(ctx context.Context) log.Logger {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return sdkCtx.Logger().With("module", "x/"+types.ModuleName)
}

// GetAuthority returns the module authority address.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// prefixEndBytes returns the end key for prefix iteration (exclusive).
func prefixEndBytes(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil
}

// ---------- Params ----------

// SetParams sets module parameters.
func (k Keeper) SetParams(ctx context.Context, params *types.Params) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal params: %v", err))
	}
	_ = kvStore.Set(types.ParamsKey, bz)
}

// GetParams returns module parameters.
func (k Keeper) GetParams(ctx context.Context) *types.Params {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.ParamsKey)
	if err != nil || bz == nil {
		return types.DefaultParams()
	}
	var params types.Params
	if err := proto.Unmarshal(bz, &params); err != nil {
		return types.DefaultParams()
	}
	return &params
}

// ---------- Genesis ----------

// InitGenesis initializes the module's state from genesis.
func (k Keeper) InitGenesis(ctx context.Context, genState *types.GenesisState) {
	if genState.Params != nil {
		k.SetParams(ctx, genState.Params)
	}
	for _, dispute := range genState.Disputes {
		k.SetDispute(ctx, dispute)
	}
}

// ExportGenesis exports the module's state.
func (k Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	params := k.GetParams(ctx)
	disputes := k.GetAllDisputes(ctx)
	return &types.GenesisState{
		Params:   params,
		Disputes: disputes,
	}
}

// ---------- Dispute Counter ----------

// GetNextDisputeID returns the next dispute sequence and increments the counter.
func (k Keeper) GetNextDisputeID(ctx context.Context) uint64 {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.DisputeCounterKey)
	if err != nil || bz == nil {
		bz = make([]byte, 8)
	}
	counter := binary.BigEndian.Uint64(bz)
	counter++
	newBz := make([]byte, 8)
	binary.BigEndian.PutUint64(newBz, counter)
	_ = kvStore.Set(types.DisputeCounterKey, newBz)
	return counter
}

// ---------- Dispute CRUD ----------

// SetDispute stores a dispute and maintains indices.
func (k Keeper) SetDispute(ctx context.Context, dispute *types.Dispute) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(dispute)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal dispute: %v", err))
	}
	_ = kvStore.Set(types.DisputeKey(dispute.Id), bz)

	// Target index
	_ = kvStore.Set(types.TargetIndexKey(dispute.TargetId, dispute.Id), []byte{1})

	// Active index: add if still active
	if isActivePhase(dispute.Phase) {
		_ = kvStore.Set(types.ActiveIndexKey(dispute.Id), []byte{1})
	} else {
		_ = kvStore.Delete(types.ActiveIndexKey(dispute.Id))
	}
}

// GetDispute retrieves a dispute by ID.
func (k Keeper) GetDispute(ctx context.Context, id string) (*types.Dispute, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.DisputeKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var dispute types.Dispute
	if err := proto.Unmarshal(bz, &dispute); err != nil {
		return nil, false
	}
	return &dispute, true
}

// DeleteDispute removes a dispute and its indices.
func (k Keeper) DeleteDispute(ctx context.Context, dispute *types.Dispute) {
	kvStore := k.storeService.OpenKVStore(ctx)
	_ = kvStore.Delete(types.DisputeKey(dispute.Id))
	_ = kvStore.Delete(types.TargetIndexKey(dispute.TargetId, dispute.Id))
	_ = kvStore.Delete(types.ActiveIndexKey(dispute.Id))
}

// GetAllDisputes returns all disputes.
func (k Keeper) GetAllDisputes(ctx context.Context) []*types.Dispute {
	var disputes []*types.Dispute
	k.IterateDisputes(ctx, func(d *types.Dispute) bool {
		disputes = append(disputes, d)
		return false
	})
	return disputes
}

// IterateDisputes iterates all disputes.
func (k Keeper) IterateDisputes(ctx context.Context, cb func(*types.Dispute) bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	iter, err := kvStore.Iterator(types.DisputeKeyPrefix, prefixEndBytes(types.DisputeKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var dispute types.Dispute
		if err := proto.Unmarshal(iter.Value(), &dispute); err != nil {
			continue
		}
		if cb(&dispute) {
			break
		}
	}
}

// GetDisputesByTarget returns all disputes for a given target.
func (k Keeper) GetDisputesByTarget(ctx context.Context, targetID string) []*types.Dispute {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := types.TargetIndexKeyPrefix(targetID)
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var disputes []*types.Dispute
	for ; iter.Valid(); iter.Next() {
		// Extract dispute ID from key: prefix + disputeID
		key := iter.Key()
		disputeID := string(key[len(prefix):])
		if d, found := k.GetDispute(ctx, disputeID); found {
			disputes = append(disputes, d)
		}
	}
	return disputes
}

// GetActiveDisputes returns all active disputes.
func (k Keeper) GetActiveDisputes(ctx context.Context) []*types.Dispute {
	kvStore := k.storeService.OpenKVStore(ctx)
	iter, err := kvStore.Iterator(types.ActiveIndexPrefix, prefixEndBytes(types.ActiveIndexPrefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var disputes []*types.Dispute
	for ; iter.Valid(); iter.Next() {
		key := iter.Key()
		disputeID := string(key[len(types.ActiveIndexPrefix):])
		if d, found := k.GetDispute(ctx, disputeID); found {
			disputes = append(disputes, d)
		}
	}
	return disputes
}

// CountActiveDisputes returns the number of active disputes.
func (k Keeper) CountActiveDisputes(ctx context.Context) int {
	kvStore := k.storeService.OpenKVStore(ctx)
	iter, err := kvStore.Iterator(types.ActiveIndexPrefix, prefixEndBytes(types.ActiveIndexPrefix))
	if err != nil {
		return 0
	}
	defer iter.Close()
	count := 0
	for ; iter.Valid(); iter.Next() {
		count++
	}
	return count
}

// ---------- Evidence CRUD ----------

// SetEvidence stores dispute evidence.
func (k Keeper) SetEvidence(ctx context.Context, evidence *types.DisputeEvidence) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(evidence)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal evidence: %v", err))
	}
	_ = kvStore.Set(types.EvidenceKey(evidence.DisputeId, evidence.Id), bz)
}

// GetEvidenceByDispute returns all evidence for a dispute.
func (k Keeper) GetEvidenceByDispute(ctx context.Context, disputeID string) []*types.DisputeEvidence {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := types.EvidenceByDisputePrefix(disputeID)
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var items []*types.DisputeEvidence
	for ; iter.Valid(); iter.Next() {
		var ev types.DisputeEvidence
		if err := proto.Unmarshal(iter.Value(), &ev); err != nil {
			continue
		}
		items = append(items, &ev)
	}
	return items
}

// ---------- Commitment CRUD ----------

// SetCommitment stores an evidence commitment.
func (k Keeper) SetCommitment(ctx context.Context, commitment *types.EvidenceCommitment) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(commitment)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal commitment: %v", err))
	}
	_ = kvStore.Set(types.CommitmentKey(commitment.DisputeId, commitment.Submitter), bz)
}

// GetCommitment retrieves a commitment by dispute+submitter.
func (k Keeper) GetCommitment(ctx context.Context, disputeID, submitter string) (*types.EvidenceCommitment, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.CommitmentKey(disputeID, submitter))
	if err != nil || bz == nil {
		return nil, false
	}
	var commitment types.EvidenceCommitment
	if err := proto.Unmarshal(bz, &commitment); err != nil {
		return nil, false
	}
	return &commitment, true
}

// ---------- Vote CRUD ----------

// SetVote stores an arbiter vote.
func (k Keeper) SetVote(ctx context.Context, vote *types.DisputeVote) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := proto.Marshal(vote)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal vote: %v", err))
	}
	_ = kvStore.Set(types.VoteKey(vote.DisputeId, vote.Arbiter), bz)
}

// GetVote retrieves a vote by dispute+arbiter.
func (k Keeper) GetVote(ctx context.Context, disputeID, arbiter string) (*types.DisputeVote, bool) {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.VoteKey(disputeID, arbiter))
	if err != nil || bz == nil {
		return nil, false
	}
	var vote types.DisputeVote
	if err := proto.Unmarshal(bz, &vote); err != nil {
		return nil, false
	}
	return &vote, true
}

// GetVotesByDispute returns all votes for a dispute.
func (k Keeper) GetVotesByDispute(ctx context.Context, disputeID string) []*types.DisputeVote {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := types.VoteByDisputePrefix(disputeID)
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var votes []*types.DisputeVote
	for ; iter.Valid(); iter.Next() {
		var v types.DisputeVote
		if err := proto.Unmarshal(iter.Value(), &v); err != nil {
			continue
		}
		votes = append(votes, &v)
	}
	return votes
}

// ---------- Arbiter Selection ----------

// SelectArbiters selects arbiters for a dispute from qualified validators.
// It deterministically selects `count` arbiters, excluding the challenger and defender.
func (k Keeper) SelectArbiters(ctx context.Context, count int, challenger, defender string, blockHeight uint64) ([]string, error) {
	// Get qualified validators (domain "disputes" for general qualification)
	candidates, err := k.stakingKeeper.GetQualifiedValidators(ctx, "disputes", count*3) // request more than needed
	if err != nil {
		return nil, fmt.Errorf("failed to get qualified validators: %w", err)
	}

	// Filter out parties
	var eligible []string
	for _, addr := range candidates {
		if addr != challenger && addr != defender {
			eligible = append(eligible, addr)
		}
	}

	if len(eligible) < count {
		return nil, fmt.Errorf("%w: need %d, have %d", types.ErrInsufficientArbiters, count, len(eligible))
	}

	// Deterministic pseudo-random selection using block height as seed
	seed := sha256.Sum256([]byte(fmt.Sprintf("zerone.disputes.arbiter.v1:%d:%s:%s", blockHeight, challenger, defender)))

	// Score each candidate deterministically
	type scored struct {
		addr  string
		score [32]byte
	}
	var scored_candidates []scored
	for _, addr := range eligible {
		h := sha256.Sum256(append(seed[:], []byte(addr)...))
		scored_candidates = append(scored_candidates, scored{addr: addr, score: h})
	}

	// Sort by score (deterministic ordering)
	sort.Slice(scored_candidates, func(i, j int) bool {
		for k := 0; k < 32; k++ {
			if scored_candidates[i].score[k] != scored_candidates[j].score[k] {
				return scored_candidates[i].score[k] < scored_candidates[j].score[k]
			}
		}
		return false
	})

	// Take top N
	result := make([]string, count)
	for i := 0; i < count; i++ {
		result[i] = scored_candidates[i].addr
	}
	return result, nil
}

// ---------- Settlement Logic ----------

// TallyVotes tallies votes for a dispute and returns the outcome.
func (k Keeper) TallyVotes(ctx context.Context, dispute *types.Dispute) types.DisputeOutcome {
	params := k.GetParams(ctx)
	tierCfg := types.GetTierConfig(params, dispute.Tier)
	if tierCfg == nil {
		return types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT
	}

	votes := k.GetVotesByDispute(ctx, dispute.Id)
	if len(votes) == 0 {
		return types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT
	}

	// Quorum check: votes cast vs required arbiters
	quorumRequired := uint64(tierCfg.ArbiterCount) * tierCfg.QuorumBps / 1000000
	if quorumRequired == 0 {
		quorumRequired = 1
	}
	if uint64(len(votes)) < quorumRequired {
		return types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT
	}

	// Stake-weighted tally
	challengerWeight := new(big.Int)
	defenderWeight := new(big.Int)
	totalVotingWeight := new(big.Int)

	for _, v := range votes {
		stake := new(big.Int)
		if _, ok := stake.SetString(v.Stake, 10); !ok || stake.Sign() <= 0 {
			stake.SetInt64(1) // default 1 for unweighted
		}
		switch v.Vote {
		case types.ArbiterDecision_ARBITER_DECISION_CHALLENGER:
			challengerWeight.Add(challengerWeight, stake)
			totalVotingWeight.Add(totalVotingWeight, stake)
		case types.ArbiterDecision_ARBITER_DECISION_DEFENDER:
			defenderWeight.Add(defenderWeight, stake)
			totalVotingWeight.Add(totalVotingWeight, stake)
		case types.ArbiterDecision_ARBITER_DECISION_ABSTAIN:
			// Abstentions don't count toward majority
		}
	}

	if totalVotingWeight.Sign() == 0 {
		return types.DisputeOutcome_DISPUTE_OUTCOME_DRAW
	}

	// Calculate ratios in BPS (1M scale)
	scale := new(big.Int).SetUint64(1000000)
	challengerRatio := new(big.Int).Mul(challengerWeight, scale)
	challengerRatio.Div(challengerRatio, totalVotingWeight)

	defenderRatio := new(big.Int).Mul(defenderWeight, scale)
	defenderRatio.Div(defenderRatio, totalVotingWeight)

	majorityBps := new(big.Int).SetUint64(tierCfg.MajorityBps)

	if challengerRatio.Cmp(majorityBps) >= 0 {
		return types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS
	}
	if defenderRatio.Cmp(majorityBps) >= 0 {
		return types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS
	}
	return types.DisputeOutcome_DISPUTE_OUTCOME_DRAW
}

// DistributeSettlement distributes bonds based on dispute outcome.
// When the defender has posted a bond (DefenderBond != ""), two-bond math applies:
// the loser's bond is slashed and the winner's bond is returned whole.
// When only the challenger has bonded, single-bond math applies (backward compatible).
func (k Keeper) DistributeSettlement(ctx context.Context, dispute *types.Dispute) error {
	params := k.GetParams(ctx)

	challengerBond := new(big.Int)
	if _, ok := challengerBond.SetString(dispute.Bond, 10); !ok {
		challengerBond.SetInt64(0)
	}
	if challengerBond.Sign() <= 0 {
		return nil // no bond to distribute
	}

	// Determine if defender also posted a bond
	defenderBond := new(big.Int)
	hasTwoBonds := false
	if dispute.DefenderBond != "" && dispute.DefenderBond != "0" {
		if _, ok := defenderBond.SetString(dispute.DefenderBond, 10); ok && defenderBond.Sign() > 0 {
			hasTwoBonds = true
		}
	}

	switch dispute.Outcome {
	case types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS:
		if hasTwoBonds {
			return k.distributeWinTwoBonds(ctx, dispute, params, dispute.Challenger, challengerBond, defenderBond)
		}
		return k.distributeWin(ctx, dispute, params, dispute.Challenger, challengerBond)
	case types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS:
		if hasTwoBonds {
			return k.distributeWinTwoBonds(ctx, dispute, params, dispute.Defender, challengerBond, defenderBond)
		}
		return k.distributeWin(ctx, dispute, params, dispute.Defender, challengerBond)
	case types.DisputeOutcome_DISPUTE_OUTCOME_DRAW:
		if hasTwoBonds {
			return k.distributeDrawTwoBonds(ctx, dispute, params, challengerBond, defenderBond)
		}
		return k.distributeDraw(ctx, dispute, params, challengerBond)
	case types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT:
		if hasTwoBonds {
			return k.distributeTimeoutTwoBonds(ctx, dispute, challengerBond, defenderBond)
		}
		return k.distributeTimeout(ctx, dispute, challengerBond)
	}
	return nil
}

// distributeWin handles bond distribution when one side wins.
// Loser's slash: winner gets winner_rate, arbiters get arbiter_rate, remainder burned.
func (k Keeper) distributeWin(ctx context.Context, dispute *types.Dispute, params *types.Params, winner string, bond *big.Int) error {
	scale := new(big.Int).SetUint64(1000000)

	// Calculate slash amount
	slashAmt := new(big.Int).Mul(bond, new(big.Int).SetUint64(params.SlashRateLoserBps))
	slashAmt.Div(slashAmt, scale)

	// Winner reward from slash pool
	winnerReward := new(big.Int).Mul(slashAmt, new(big.Int).SetUint64(params.RewardRateWinnerBps))
	winnerReward.Div(winnerReward, scale)

	// Arbiter reward from slash pool
	arbiterReward := new(big.Int).Mul(slashAmt, new(big.Int).SetUint64(params.ArbiterRewardBps))
	arbiterReward.Div(arbiterReward, scale)

	// Burn remainder
	burnAmt := new(big.Int).Sub(slashAmt, winnerReward)
	burnAmt.Sub(burnAmt, arbiterReward)

	// Return unslashed portion to challenger (bond always comes from challenger)
	refund := new(big.Int).Sub(bond, slashAmt)

	// Distribute winner reward
	if winnerReward.Sign() > 0 {
		winnerAddr, err := sdk.AccAddressFromBech32(winner)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(winnerReward)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, winnerAddr, coins); err != nil {
			return err
		}
	}

	// Distribute arbiter rewards (split equally among arbiters who voted correctly)
	if arbiterReward.Sign() > 0 && len(dispute.Arbiters) > 0 {
		votes := k.GetVotesByDispute(ctx, dispute.Id)
		var correctVoters []string
		for _, v := range votes {
			if (dispute.Outcome == types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS && v.Vote == types.ArbiterDecision_ARBITER_DECISION_CHALLENGER) ||
				(dispute.Outcome == types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS && v.Vote == types.ArbiterDecision_ARBITER_DECISION_DEFENDER) {
				correctVoters = append(correctVoters, v.Arbiter)
			}
		}
		if len(correctVoters) > 0 {
			perArbiter := new(big.Int).Div(arbiterReward, big.NewInt(int64(len(correctVoters))))
			for _, arb := range correctVoters {
				arbAddr, err := sdk.AccAddressFromBech32(arb)
				if err != nil {
					continue
				}
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(perArbiter)))
				if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, arbAddr, coins); err != nil {
					k.Logger(ctx).Error("failed to distribute arbiter reward", "arbiter", arb, "error", err)
				}
			}
		}
	}

	// Refund unslashed portion to challenger
	if refund.Sign() > 0 {
		challengerAddr, err := sdk.AccAddressFromBech32(dispute.Challenger)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(refund)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, coins); err != nil {
			return err
		}
	}

	// Route remainder to development fund
	if burnAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(burnAmt)))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "development_fund", coins); err != nil {
			return err
		}
	}

	return nil
}

// distributeDraw handles bond return minus arbiter fees on draw.
func (k Keeper) distributeDraw(ctx context.Context, dispute *types.Dispute, params *types.Params, bond *big.Int) error {
	scale := new(big.Int).SetUint64(1000000)

	// Small arbiter fee on draw
	arbiterFee := new(big.Int).Mul(bond, new(big.Int).SetUint64(params.ArbiterRewardBps))
	arbiterFee.Div(arbiterFee, scale)

	// Return bond minus fees to challenger
	refund := new(big.Int).Sub(bond, arbiterFee)
	if refund.Sign() > 0 {
		challengerAddr, err := sdk.AccAddressFromBech32(dispute.Challenger)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(refund)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, coins); err != nil {
			return err
		}
	}

	// Distribute arbiter fee to all voters equally
	if arbiterFee.Sign() > 0 {
		votes := k.GetVotesByDispute(ctx, dispute.Id)
		if len(votes) > 0 {
			perArbiter := new(big.Int).Div(arbiterFee, big.NewInt(int64(len(votes))))
			for _, v := range votes {
				arbAddr, err := sdk.AccAddressFromBech32(v.Arbiter)
				if err != nil {
					continue
				}
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(perArbiter)))
				_ = k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, arbAddr, coins)
			}
		}
	}

	return nil
}

// distributeTimeout returns bonds on timeout.
func (k Keeper) distributeTimeout(ctx context.Context, dispute *types.Dispute, bond *big.Int) error {
	if bond.Sign() <= 0 {
		return nil
	}
	challengerAddr, err := sdk.AccAddressFromBech32(dispute.Challenger)
	if err != nil {
		return err
	}
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(bond)))
	return k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, coins)
}

// ---------- Two-Bond Settlement (P-16a: Defender Bond) ----------

// distributeWinTwoBonds handles settlement when both parties have posted bonds.
// The loser's bond is slashed; the winner's bond is returned whole plus the winner reward.
// Fund conservation: challengerBond + defenderBond flows out in full.
func (k Keeper) distributeWinTwoBonds(ctx context.Context, dispute *types.Dispute, params *types.Params, winner string, challengerBond, defenderBond *big.Int) error {
	scale := new(big.Int).SetUint64(1000000)

	// Identify loser and winner
	var loserBond, winnerBond *big.Int
	var loserAddr, winnerAddr string
	if winner == dispute.Challenger {
		loserBond = new(big.Int).Set(defenderBond)
		loserAddr = dispute.Defender
		winnerBond = new(big.Int).Set(challengerBond)
		winnerAddr = dispute.Challenger
	} else {
		loserBond = new(big.Int).Set(challengerBond)
		loserAddr = dispute.Challenger
		winnerBond = new(big.Int).Set(defenderBond)
		winnerAddr = dispute.Defender
	}

	// Slash loser's bond
	slashAmt := new(big.Int).Mul(loserBond, new(big.Int).SetUint64(params.SlashRateLoserBps))
	slashAmt.Div(slashAmt, scale)

	// Winner reward from loser's slash pool
	winnerReward := new(big.Int).Mul(slashAmt, new(big.Int).SetUint64(params.RewardRateWinnerBps))
	winnerReward.Div(winnerReward, scale)

	// Arbiter reward from loser's slash pool
	arbiterReward := new(big.Int).Mul(slashAmt, new(big.Int).SetUint64(params.ArbiterRewardBps))
	arbiterReward.Div(arbiterReward, scale)

	// Burn loser's slash remainder
	burnAmt := new(big.Int).Sub(slashAmt, winnerReward)
	burnAmt.Sub(burnAmt, arbiterReward)

	// Loser gets back unslashed portion
	loserRefund := new(big.Int).Sub(loserBond, slashAmt)

	// Return winner's full bond + their reward
	wAddr, err := sdk.AccAddressFromBech32(winnerAddr)
	if err != nil {
		return err
	}
	winnerTotal := new(big.Int).Add(winnerBond, winnerReward)
	if winnerTotal.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(winnerTotal)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, wAddr, coins); err != nil {
			return err
		}
	}

	// Return loser's unslashed portion
	if loserRefund.Sign() > 0 {
		lAddr, err := sdk.AccAddressFromBech32(loserAddr)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(loserRefund)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, lAddr, coins); err != nil {
			return err
		}
	}

	// Distribute arbiter rewards to correct voters
	if arbiterReward.Sign() > 0 && len(dispute.Arbiters) > 0 {
		votes := k.GetVotesByDispute(ctx, dispute.Id)
		var correctVoters []string
		for _, v := range votes {
			if (dispute.Outcome == types.DisputeOutcome_DISPUTE_OUTCOME_CHALLENGER_WINS && v.Vote == types.ArbiterDecision_ARBITER_DECISION_CHALLENGER) ||
				(dispute.Outcome == types.DisputeOutcome_DISPUTE_OUTCOME_DEFENDER_WINS && v.Vote == types.ArbiterDecision_ARBITER_DECISION_DEFENDER) {
				correctVoters = append(correctVoters, v.Arbiter)
			}
		}
		if len(correctVoters) > 0 {
			perArbiter := new(big.Int).Div(arbiterReward, big.NewInt(int64(len(correctVoters))))
			for _, arb := range correctVoters {
				arbAddr, err := sdk.AccAddressFromBech32(arb)
				if err != nil {
					continue
				}
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(perArbiter)))
				if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, arbAddr, coins); err != nil {
					k.Logger(ctx).Error("failed to distribute arbiter reward (two-bond)", "arbiter", arb, "error", err)
				}
			}
		}
	}

	// Route loser's slash remainder to development fund (same as single-bond path;
	// P-18b-a will migrate all paths to BurnCoins once merged).
	if burnAmt.Sign() > 0 {
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(burnAmt)))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "development_fund", coins); err != nil {
			return err
		}
	}

	return nil
}

// distributeDrawTwoBonds handles draw when both parties have posted bonds.
// Each party pays an equal arbiter fee; the remainder is returned to each.
func (k Keeper) distributeDrawTwoBonds(ctx context.Context, dispute *types.Dispute, params *types.Params, challengerBond, defenderBond *big.Int) error {
	scale := new(big.Int).SetUint64(1000000)

	// Arbiter fee taken from each party's bond proportionally (same rate)
	arbiterFeeChallenger := new(big.Int).Mul(challengerBond, new(big.Int).SetUint64(params.ArbiterRewardBps))
	arbiterFeeChallenger.Div(arbiterFeeChallenger, scale)

	arbiterFeeDefender := new(big.Int).Mul(defenderBond, new(big.Int).SetUint64(params.ArbiterRewardBps))
	arbiterFeeDefender.Div(arbiterFeeDefender, scale)

	// Return challenger net
	challengerRefund := new(big.Int).Sub(challengerBond, arbiterFeeChallenger)
	if challengerRefund.Sign() > 0 {
		cAddr, err := sdk.AccAddressFromBech32(dispute.Challenger)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(challengerRefund)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, cAddr, coins); err != nil {
			return err
		}
	}

	// Return defender net
	defenderRefund := new(big.Int).Sub(defenderBond, arbiterFeeDefender)
	if defenderRefund.Sign() > 0 {
		dAddr, err := sdk.AccAddressFromBech32(dispute.Defender)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(defenderRefund)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, dAddr, coins); err != nil {
			return err
		}
	}

	// Distribute combined arbiter fee to all voters equally
	totalArbiterFee := new(big.Int).Add(arbiterFeeChallenger, arbiterFeeDefender)
	if totalArbiterFee.Sign() > 0 {
		votes := k.GetVotesByDispute(ctx, dispute.Id)
		if len(votes) > 0 {
			perArbiter := new(big.Int).Div(totalArbiterFee, big.NewInt(int64(len(votes))))
			for _, v := range votes {
				arbAddr, err := sdk.AccAddressFromBech32(v.Arbiter)
				if err != nil {
					continue
				}
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(perArbiter)))
				_ = k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, arbAddr, coins)
			}
		}
	}

	return nil
}

// distributeTimeoutTwoBonds returns both bonds in full on timeout.
func (k Keeper) distributeTimeoutTwoBonds(ctx context.Context, dispute *types.Dispute, challengerBond, defenderBond *big.Int) error {
	// Return challenger bond
	if challengerBond.Sign() > 0 {
		cAddr, err := sdk.AccAddressFromBech32(dispute.Challenger)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(challengerBond)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, cAddr, coins); err != nil {
			return err
		}
	}

	// Return defender bond
	if defenderBond.Sign() > 0 {
		dAddr, err := sdk.AccAddressFromBech32(dispute.Defender)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(defenderBond)))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, dAddr, coins); err != nil {
			return err
		}
	}

	return nil
}

// ---------- BeginBlocker Logic ----------

// ProcessPhaseTransitions advances dispute phases based on block deadlines.
func (k Keeper) ProcessPhaseTransitions(ctx context.Context, currentBlock uint64) {
	active := k.GetActiveDisputes(ctx)
	for _, dispute := range active {
		switch dispute.Phase {
		case types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT:
			if currentBlock > dispute.EvidenceDeadline {
				// Advance to reveal phase: evidence_deadline + half of evidence period
				params := k.GetParams(ctx)
				tierCfg := types.GetTierConfig(params, dispute.Tier)
				if tierCfg != nil {
					dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL
					// Reveal phase gets half the evidence period
					dispute.EvidenceDeadline = currentBlock + tierCfg.EvidencePeriod/2
					k.SetDispute(ctx, dispute)
				}
			}
		case types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL:
			if currentBlock > dispute.EvidenceDeadline {
				// Advance to arbitration
				dispute.Phase = types.DisputePhase_DISPUTE_PHASE_ARBITRATION
				k.SetDispute(ctx, dispute)
			}
		case types.DisputePhase_DISPUTE_PHASE_ARBITRATION:
			if currentBlock > dispute.VotingDeadline {
				// Auto-settle: tally votes
				outcome := k.TallyVotes(ctx, dispute)
				dispute.Outcome = outcome
				dispute.Phase = types.DisputePhase_DISPUTE_PHASE_SETTLED
				if outcome == types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
					dispute.Phase = types.DisputePhase_DISPUTE_PHASE_TIMED_OUT
				}
				k.SetDispute(ctx, dispute)
				if err := k.DistributeSettlement(ctx, dispute); err != nil {
					k.Logger(ctx).Error("failed to distribute settlement", "dispute_id", dispute.Id, "error", err)
				}
			}
		}
	}
}

// GenerateDisputeID creates a deterministic dispute ID.
func GenerateDisputeID(targetID, challenger string, blockHeight uint64) string {
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, blockHeight)
	h := sha256.Sum256(append(append([]byte(targetID), []byte(challenger)...), heightBytes...))
	return hex.EncodeToString(h[:16])
}

// isActivePhase returns true if the dispute phase is considered active.
func isActivePhase(phase types.DisputePhase) bool {
	switch phase {
	case types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL,
		types.DisputePhase_DISPUTE_PHASE_ARBITRATION,
		types.DisputePhase_DISPUTE_PHASE_ESCALATED:
		return true
	}
	return false
}
