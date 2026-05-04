// Package keeper implements the oracle_committee module state machine.
//
// The oracle committee is a on-chain registry of independent oracle operators
// who evaluate claims. When a verification round requests evaluation, registered
// active operators independently submit verdicts. Once quorum is reached, the
// keeper aggregates via stake-weighted median and finalizes an OracleAggregation.
//
// Aggregation algorithm:
//  1. Collect all OracleEvaluationRecord entries for the claim.
//  2. Separate by verdict group ("true", "false", "uncertain").
//  3. The final_verdict is the majority group by cumulative stake weight.
//     Tie → "uncertain".
//  4. aggregated_confidence_bps is the stake-weighted median confidence
//     across all submissions in the winning verdict group.
//
// Slashing (called by external panels after adjudication):
//   - If an operator's verdict != final_panel_verdict AND confidence_bps > 5000
//     (the operator was confident and wrong), slash max_slash_fraction_bps of
//     their stake.
//   - Slashed tokens are transferred to the community pool.
package keeper

import (
	"fmt"
	"math/big"
	"sort"

	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/oracle_committee/types"
)

// evalWithWeight pairs a confidence_bps value with a stake weight for median computation.
type evalWithWeight struct {
	confidence uint32
	stake      *big.Int
}

// BankKeeper defines the bank operations used by the oracle_committee keeper.
type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx sdk.Context, senderModule, recipientModule string, amt sdk.Coins) error
}

// Keeper manages the oracle_committee module's state.
type Keeper struct {
	cdc          codec.Codec
	storeService store.KVStoreService
	bankKeeper   BankKeeper
	// governance authority — address allowed to call UpdateOracleCommitteeParams
	authority string
}

// NewKeeper creates a new oracle_committee module Keeper.
func NewKeeper(
	cdc codec.Codec,
	storeService store.KVStoreService,
	bk BankKeeper,
	authority string,
) Keeper {
	return Keeper{
		cdc:          cdc,
		storeService: storeService,
		bankKeeper:   bk,
		authority:    authority,
	}
}

func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// ── Params ───────────────────────────────────────────────────────

// DefaultParams returns the default oracle committee parameters.
func DefaultParams() *types.OracleCommitteeParams {
	return &types.OracleCommitteeParams{
		MinQuorumBps:            6000,  // 60%
		MinCommitteeSize:        3,
		EvaluationWindowBlocks:  50,    // ~5 minutes at 6s/block
		MaxSlashFractionBps:     500,   // 5%
		MinStake:                "1000000", // 1 ZRN in uzrn
	}
}

// GetParams returns the current oracle committee params.
// Falls back to DefaultParams if not set.
func (k Keeper) GetParams(ctx sdk.Context) *types.OracleCommitteeParams {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ParamsKey)
	if err != nil || bz == nil {
		return DefaultParams()
	}
	var p types.OracleCommitteeParams
	if err := k.cdc.Unmarshal(bz, &p); err != nil {
		return DefaultParams()
	}
	return &p
}

// SetParams persists oracle committee params.
func (k Keeper) SetParams(ctx sdk.Context, p *types.OracleCommitteeParams) error {
	bz, err := k.cdc.Marshal(p)
	if err != nil {
		return err
	}
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.ParamsKey, bz)
}

// ── Operators ────────────────────────────────────────────────────

// GetOperator returns the OracleOperator with the given address, or (nil, false).
func (k Keeper) GetOperator(ctx sdk.Context, address string) (*types.OracleOperator, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.OperatorKey(address))
	if err != nil || bz == nil {
		return nil, false
	}
	var op types.OracleOperator
	if err := k.cdc.Unmarshal(bz, &op); err != nil {
		return nil, false
	}
	return &op, true
}

// SetOperator persists an OracleOperator.
func (k Keeper) SetOperator(ctx sdk.Context, op *types.OracleOperator) error {
	bz, err := k.cdc.Marshal(op)
	if err != nil {
		return err
	}
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.OperatorKey(op.Address), bz)
}

// DeleteOperator removes an OracleOperator from the store.
func (k Keeper) DeleteOperator(ctx sdk.Context, address string) error {
	store := k.storeService.OpenKVStore(ctx)
	return store.Delete(types.OperatorKey(address))
}

// GetAllOperators returns all registered oracle operators.
func (k Keeper) GetAllOperators(ctx sdk.Context) []*types.OracleOperator {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.OperatorKeyPrefix, append(types.OperatorKeyPrefix, 0xFF))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var ops []*types.OracleOperator
	for ; iter.Valid(); iter.Next() {
		var op types.OracleOperator
		if err := k.cdc.Unmarshal(iter.Value(), &op); err == nil {
			ops = append(ops, &op)
		}
	}
	return ops
}

// CountActiveOperators returns the number of currently active operators.
func (k Keeper) CountActiveOperators(ctx sdk.Context) int {
	count := 0
	for _, op := range k.GetAllOperators(ctx) {
		if op.Active {
			count++
		}
	}
	return count
}

// ── Evaluations ──────────────────────────────────────────────────

// GetEvaluation returns the evaluation record for (claimID, operator), or (nil, false).
func (k Keeper) GetEvaluation(ctx sdk.Context, claimID, operator string) (*types.OracleEvaluationRecord, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.EvaluationKey(claimID, operator))
	if err != nil || bz == nil {
		return nil, false
	}
	var rec types.OracleEvaluationRecord
	if err := k.cdc.Unmarshal(bz, &rec); err != nil {
		return nil, false
	}
	return &rec, true
}

// SetEvaluation persists an OracleEvaluationRecord.
func (k Keeper) SetEvaluation(ctx sdk.Context, rec *types.OracleEvaluationRecord) error {
	bz, err := k.cdc.Marshal(rec)
	if err != nil {
		return err
	}
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.EvaluationKey(rec.ClaimId, rec.Operator), bz)
}

// GetEvaluationsForClaim returns all evaluation records for a given claim.
func (k Keeper) GetEvaluationsForClaim(ctx sdk.Context, claimID string) []*types.OracleEvaluationRecord {
	store := k.storeService.OpenKVStore(ctx)
	prefix := types.EvaluationByClaimPrefix(claimID)
	iter, err := store.Iterator(prefix, append(prefix, 0xFF))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var recs []*types.OracleEvaluationRecord
	for ; iter.Valid(); iter.Next() {
		var rec types.OracleEvaluationRecord
		if err := k.cdc.Unmarshal(iter.Value(), &rec); err == nil {
			recs = append(recs, &rec)
		}
	}
	return recs
}

// ── Aggregations ─────────────────────────────────────────────────

// GetAggregation returns the finalized OracleAggregation for a claim, or (nil, false).
func (k Keeper) GetAggregation(ctx sdk.Context, claimID string) (*types.OracleAggregation, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.AggregationKey(claimID))
	if err != nil || bz == nil {
		return nil, false
	}
	var agg types.OracleAggregation
	if err := k.cdc.Unmarshal(bz, &agg); err != nil {
		return nil, false
	}
	return &agg, true
}

// SetAggregation persists an OracleAggregation.
func (k Keeper) SetAggregation(ctx sdk.Context, agg *types.OracleAggregation) error {
	bz, err := k.cdc.Marshal(agg)
	if err != nil {
		return err
	}
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.AggregationKey(agg.ClaimId), bz)
}

// ── Core Logic ────────────────────────────────────────────────────

// RegisterOperator registers a new oracle operator, locking their stake.
func (k Keeper) RegisterOperator(ctx sdk.Context, address, uri, stakeStr string) error {
	if _, exists := k.GetOperator(ctx, address); exists {
		return types.ErrOperatorAlreadyExists
	}
	params := k.GetParams(ctx)

	// Validate stake meets minimum.
	stake, ok := new(big.Int).SetString(stakeStr, 10)
	if !ok || stake.Sign() <= 0 {
		return types.ErrInsufficientStake
	}
	minStake, _ := new(big.Int).SetString(params.MinStake, 10)
	if stake.Cmp(minStake) < 0 {
		return types.ErrInsufficientStake
	}

	// Lock stake: operator → module.
	operatorAddr, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		return fmt.Errorf("invalid operator address: %w", err)
	}
	stakeInt, ok2 := sdkmath.NewIntFromString(stakeStr)
	if !ok2 {
		return types.ErrInsufficientStake
	}
	stakeCoins := sdk.NewCoins(sdk.NewCoin("uzrn", stakeInt))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, operatorAddr, types.ModuleName, stakeCoins); err != nil {
		return fmt.Errorf("failed to lock stake: %w", err)
	}

	op := &types.OracleOperator{
		Address:      address,
		Uri:          uri,
		Stake:        stakeStr,
		Active:       true,
		RegisteredAt: uint64(ctx.BlockHeight()),
		SlashCount:   0,
	}
	return k.SetOperator(ctx, op)
}

// DeregisterOperator removes an operator and returns their stake.
func (k Keeper) DeregisterOperator(ctx sdk.Context, address string) error {
	op, found := k.GetOperator(ctx, address)
	if !found {
		return types.ErrOperatorNotFound
	}

	// Return stake: module → operator.
	operatorAddr, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		return fmt.Errorf("invalid operator address: %w", err)
	}
	stakeInt, ok := sdkmath.NewIntFromString(op.Stake)
	if !ok {
		return fmt.Errorf("invalid stake stored for operator")
	}
	stakeCoins := sdk.NewCoins(sdk.NewCoin("uzrn", stakeInt))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, operatorAddr, stakeCoins); err != nil {
		return fmt.Errorf("failed to return stake: %w", err)
	}

	return k.DeleteOperator(ctx, address)
}

// SubmitEvaluation records one operator's evaluation. If quorum is reached,
// fires aggregation and returns (aggregation, true). Otherwise (nil, false).
func (k Keeper) SubmitEvaluation(
	ctx sdk.Context,
	operator, claimID, verdict string,
	confidenceBps uint32,
	reasoning string,
) (*types.OracleAggregation, bool, error) {
	// Validation.
	op, found := k.GetOperator(ctx, operator)
	if !found {
		return nil, false, types.ErrOperatorNotFound
	}
	if !op.Active {
		return nil, false, types.ErrOperatorInactive
	}
	if verdict != "true" && verdict != "false" && verdict != "uncertain" {
		return nil, false, types.ErrInvalidVerdict
	}
	if confidenceBps > 10000 {
		return nil, false, types.ErrInvalidConfidence
	}
	if _, exists := k.GetEvaluation(ctx, claimID, operator); exists {
		return nil, false, types.ErrDuplicateEvaluation
	}
	if _, exists := k.GetAggregation(ctx, claimID); exists {
		return nil, false, types.ErrAggregationExists
	}

	// Store the evaluation.
	rec := &types.OracleEvaluationRecord{
		ClaimId:       claimID,
		Operator:      operator,
		Verdict:       verdict,
		ConfidenceBps: confidenceBps,
		Reasoning:     reasoning,
		SubmittedAt:   uint64(ctx.BlockHeight()),
	}
	if err := k.SetEvaluation(ctx, rec); err != nil {
		return nil, false, err
	}

	// Check quorum.
	params := k.GetParams(ctx)
	activeCount := k.CountActiveOperators(ctx)
	if activeCount < int(params.MinCommitteeSize) {
		return nil, false, nil // not enough operators yet; no quorum possible
	}

	evals := k.GetEvaluationsForClaim(ctx, claimID)
	quorumThreshold := (activeCount*int(params.MinQuorumBps) + 9999) / 10000 // ceiling division
	if len(evals) < quorumThreshold {
		return nil, false, nil // quorum not yet reached
	}

	// Quorum reached — aggregate.
	agg := k.aggregate(ctx, claimID, evals)
	if err := k.SetAggregation(ctx, agg); err != nil {
		return nil, false, err
	}
	return agg, true, nil
}

// SlashOperator slashes an operator whose verdict diverged from the final panel
// verdict (called by an external adjudication module post-panel).
//
// Only slashes if:
//   - operator's submitted verdict != panelVerdict
//   - operator's confidence_bps > 5000 (confident and wrong, not merely uncertain)
//
// Slashed tokens are sent to the community pool module.
func (k Keeper) SlashOperator(ctx sdk.Context, operator, claimID, panelVerdict, communityPoolModule string) error {
	op, found := k.GetOperator(ctx, operator)
	if !found {
		return types.ErrOperatorNotFound
	}

	eval, found := k.GetEvaluation(ctx, claimID, operator)
	if !found {
		return nil // operator didn't evaluate this claim; no slash
	}

	if eval.Verdict == panelVerdict || eval.ConfidenceBps <= 5000 {
		return nil // correct verdict OR low confidence — no slash
	}

	params := k.GetParams(ctx)
	stake, ok := new(big.Int).SetString(op.Stake, 10)
	if !ok || stake.Sign() <= 0 {
		return nil
	}

	// slashAmount = stake * maxSlashFractionBps / 10000
	slashBps := new(big.Int).SetUint64(uint64(params.MaxSlashFractionBps))
	slashAmount := new(big.Int).Mul(stake, slashBps)
	slashAmount.Div(slashAmount, big.NewInt(10000))

	if slashAmount.Sign() <= 0 {
		return nil
	}

	slashInt, _ := sdkmath.NewIntFromString(slashAmount.String())
	slashCoins := sdk.NewCoins(sdk.NewCoin("uzrn", slashInt))

	// Transfer slashed amount from module to community pool.
	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, communityPoolModule, slashCoins); err != nil {
		return fmt.Errorf("slash transfer failed: %w", err)
	}

	// Reduce stored stake and increment slash count.
	newStake := new(big.Int).Sub(stake, slashAmount)
	op.Stake = newStake.String()
	op.SlashCount++
	return k.SetOperator(ctx, op)
}

// ── Internal ─────────────────────────────────────────────────────

// aggregate computes the final verdict and confidence from a set of evaluation
// records. It uses stake-weighted majority for verdict selection and
// stake-weighted median for confidence aggregation.
//
// Exported for testing.
func (k Keeper) aggregate(ctx sdk.Context, claimID string, evals []*types.OracleEvaluationRecord) *types.OracleAggregation {
	return Aggregate(claimID, evals, k.GetAllOperators(ctx), uint64(ctx.BlockHeight()))
}

// ── Genesis ──────────────────────────────────────────────────────

// ExportGenesis exports the current oracle committee state.
func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	ops := k.GetAllOperators(ctx)
	// Collect all evaluations (scan full prefix).
	store := k.storeService.OpenKVStore(ctx)
	evalIter, _ := store.Iterator(types.EvaluationKeyPrefix, append(types.EvaluationKeyPrefix, 0xFF))
	var evals []*types.OracleEvaluationRecord
	if evalIter != nil {
		defer evalIter.Close()
		for ; evalIter.Valid(); evalIter.Next() {
			var rec types.OracleEvaluationRecord
			if err := k.cdc.Unmarshal(evalIter.Value(), &rec); err == nil {
				evals = append(evals, &rec)
			}
		}
	}
	aggIter, _ := store.Iterator(types.AggregationKeyPrefix, append(types.AggregationKeyPrefix, 0xFF))
	var aggs []*types.OracleAggregation
	if aggIter != nil {
		defer aggIter.Close()
		for ; aggIter.Valid(); aggIter.Next() {
			var agg types.OracleAggregation
			if err := k.cdc.Unmarshal(aggIter.Value(), &agg); err == nil {
				aggs = append(aggs, &agg)
			}
		}
	}
	return &types.GenesisState{
		Params:      k.GetParams(ctx),
		Operators:   ops,
		Evaluations: evals,
		Aggregations: aggs,
	}
}

// InitGenesis initializes oracle committee state from a genesis state.
func (k Keeper) InitGenesis(ctx sdk.Context, gs *types.GenesisState) {
	if gs.Params != nil {
		_ = k.SetParams(ctx, gs.Params)
	}
	for _, op := range gs.Operators {
		_ = k.SetOperator(ctx, op)
	}
	for _, eval := range gs.Evaluations {
		_ = k.SetEvaluation(ctx, eval)
	}
	for _, agg := range gs.Aggregations {
		_ = k.SetAggregation(ctx, agg)
	}
}

// ── JSON helpers for tests ────────────────────────────────────────

// stakeForOperator returns the stored stake for an operator address, or "0".
func stakeForOperator(ops []*types.OracleOperator, address string) *big.Int {
	for _, op := range ops {
		if op.Address == address {
			v, _ := new(big.Int).SetString(op.Stake, 10)
			return v
		}
	}
	return big.NewInt(0)
}

// Aggregate computes the final verdict and aggregated confidence from a set
// of evaluation records, using the provided operator list for stake weights.
//
// Exported (capital A) so unit tests can call it without a live SDK context.
//
// Algorithm:
//  1. Group evaluations by verdict. Sum stake weight per group.
//  2. The verdict group with the highest total stake weight wins.
//     Tie between groups → "uncertain".
//  3. Collect confidence_bps values from the winning group (or all if uncertain).
//     Compute stake-weighted median confidence.
func Aggregate(claimID string, evals []*types.OracleEvaluationRecord, ops []*types.OracleOperator, blockHeight uint64) *types.OracleAggregation {
	if len(evals) == 0 {
		return &types.OracleAggregation{
			ClaimId:                 claimID,
			FinalVerdict:            "uncertain",
			AggregatedConfidenceBps: 0,
			OperatorCount:           0,
			FinalizedAt:             blockHeight,
		}
	}

	// Sum stake weights per verdict group.
	groups := map[string][]evalWithWeight{
		"true":      {},
		"false":     {},
		"uncertain": {},
	}
	totalStake := map[string]*big.Int{
		"true":      big.NewInt(0),
		"false":     big.NewInt(0),
		"uncertain": big.NewInt(0),
	}
	for _, ev := range evals {
		s := stakeForOperator(ops, ev.Operator)
		if s.Sign() == 0 {
			s = big.NewInt(1) // uniform weight if no stake data
		}
		v := ev.Verdict
		if _, ok := groups[v]; !ok {
			v = "uncertain"
		}
		groups[v] = append(groups[v], evalWithWeight{confidence: ev.ConfidenceBps, stake: s})
		totalStake[v] = new(big.Int).Add(totalStake[v], s)
	}

	// Select winning verdict: highest total stake. Tie → uncertain.
	winVerdict := "uncertain"
	winStake := big.NewInt(-1)
	for _, verdict := range []string{"true", "false", "uncertain"} {
		s := totalStake[verdict]
		cmp := s.Cmp(winStake)
		if cmp > 0 {
			winVerdict = verdict
			winStake = s
		} else if cmp == 0 && winVerdict != "uncertain" {
			winVerdict = "uncertain" // tie → uncertain
		}
	}

	// Stake-weighted median confidence from winning group's evaluations.
	// (If uncertain by tie, use all evaluations.)
	var candidateEvals []evalWithWeight
	if winVerdict == "uncertain" && totalStake["true"].Cmp(big.NewInt(0)) > 0 || totalStake["false"].Cmp(big.NewInt(0)) > 0 {
		// There was a contested tie — pull all evals.
		for _, g := range groups {
			candidateEvals = append(candidateEvals, g...)
		}
	} else {
		candidateEvals = groups[winVerdict]
	}
	medianConf := stakeWeightedMedianConfidence(candidateEvals)

	return &types.OracleAggregation{
		ClaimId:                 claimID,
		FinalVerdict:            winVerdict,
		AggregatedConfidenceBps: medianConf,
		OperatorCount:           uint32(len(evals)),
		FinalizedAt:             blockHeight,
	}
}

// stakeWeightedMedianConfidence returns the stake-weighted median confidence_bps.
//
// Sort by confidence ascending. Walk from lowest to highest, accumulating
// stake. The median is the confidence value at which cumulative stake >= 50%
// of total stake.
func stakeWeightedMedianConfidence(evals []evalWithWeight) uint32 {
	var items []evalWithWeight
	total := big.NewInt(0)
	for _, e := range evals {
		s := e.stake
		if s == nil || s.Sign() <= 0 {
			s = big.NewInt(1)
		}
		items = append(items, evalWithWeight{confidence: e.confidence, stake: new(big.Int).Set(s)})
		total.Add(total, s)
	}
	if len(items) == 0 || total.Sign() == 0 {
		return 5000 // default: uncertain confidence
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].confidence < items[j].confidence
	})

	// Walk to cumulative 50% threshold.
	half := new(big.Int).Rsh(total, 1) // total / 2
	cum := big.NewInt(0)
	for _, item := range items {
		cum.Add(cum, item.stake)
		if cum.Cmp(half) >= 0 {
			return item.confidence
		}
	}
	return items[len(items)-1].confidence
}

