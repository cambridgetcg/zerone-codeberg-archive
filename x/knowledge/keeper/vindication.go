package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── Vindication Pending (per-fact minority entries) ─────────────────────────

// SetVindicationPending stores the pending vindication entries for a fact as JSON.
func (k Keeper) SetVindicationPending(ctx context.Context, factId string, entries []types.VindicationEntry) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return store.Set(types.VindicationPendingKey(factId), bz)
}

// GetVindicationPending retrieves pending vindication entries for a fact.
// Returns nil if no entries are found.
func (k Keeper) GetVindicationPending(ctx context.Context, factId string) []types.VindicationEntry {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.VindicationPendingKey(factId))
	if err != nil || bz == nil {
		return nil
	}
	var entries []types.VindicationEntry
	if err := json.Unmarshal(bz, &entries); err != nil {
		return nil
	}
	return entries
}

// DeleteVindicationPending removes all pending vindication entries for a fact.
func (k Keeper) DeleteVindicationPending(ctx context.Context, factId string) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.VindicationPendingKey(factId))
}

// GetAllVindicationPending iterates all pending vindication entries across all facts.
// Returns a map of factId -> entries. Used by pruning logic in BeginBlocker.
// NOTE: Map iteration order is non-deterministic. Safe for pruning (each entry is
// processed independently), but do not use where ordering affects consensus state.
func (k Keeper) GetAllVindicationPending(ctx context.Context) map[string][]types.VindicationEntry {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.VindicationPendingPrefix, prefixEndBytes(types.VindicationPendingPrefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	result := make(map[string][]types.VindicationEntry)
	for ; iter.Valid(); iter.Next() {
		factId := string(iter.Key()[len(types.VindicationPendingPrefix):])
		var entries []types.VindicationEntry
		if err := json.Unmarshal(iter.Value(), &entries); err != nil {
			continue
		}
		result[factId] = entries
	}
	return result
}

// ─── Vindication Records (immutable per-verifier outcomes) ───────────────────

// SetVindicationRecord stores a vindication record for a specific fact/verifier pair.
func (k Keeper) SetVindicationRecord(ctx context.Context, factId string, record types.VindicationRecord) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return store.Set(types.VindicationRecordKey(factId, record.Verifier), bz)
}

// GetVindicationRecord retrieves a vindication record for a specific fact and verifier.
// Returns the record and true if found, or a zero-value record and false if not.
func (k Keeper) GetVindicationRecord(ctx context.Context, factId, verifier string) (types.VindicationRecord, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.VindicationRecordKey(factId, verifier))
	if err != nil || bz == nil {
		return types.VindicationRecord{}, false
	}
	var record types.VindicationRecord
	if err := json.Unmarshal(bz, &record); err != nil {
		return types.VindicationRecord{}, false
	}
	return record, true
}

// IterateAllVindicationRecords yields every vindication record across all
// facts. Used by the training corpus exporter (Phase 9) — correct-dissent
// records are the rarest positive training signal on-chain.
func (k Keeper) IterateAllVindicationRecords(ctx context.Context, cb func(*types.VindicationRecord) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.VindicationRecordPrefix, prefixEndBytes(types.VindicationRecordPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var record types.VindicationRecord
		if err := json.Unmarshal(iter.Value(), &record); err != nil {
			continue
		}
		if cb(&record) {
			return
		}
	}
}

// GetVindicationRecordsForFact returns all vindication records for a given fact.
func (k Keeper) GetVindicationRecordsForFact(ctx context.Context, factId string) []types.VindicationRecord {
	store := k.storeService.OpenKVStore(ctx)
	pfx := types.VindicationRecordPrefixForFact(factId)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var records []types.VindicationRecord
	for ; iter.Valid(); iter.Next() {
		var record types.VindicationRecord
		if err := json.Unmarshal(iter.Value(), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	return records
}

// ─── Pruning ─────────────────────────────────────────────────────────────────

// PruneExpiredVindications removes pending vindication entries older than the window.
// Expired escrowed tokens are sent to protocol treasury.
func (k Keeper) PruneExpiredVindications(ctx context.Context, currentHeight, windowBlocks uint64) {
	allPending := k.GetAllVindicationPending(ctx)

	for factId, entries := range allPending {
		if len(entries) == 0 {
			continue
		}
		// All entries for a fact are created atomically in CompleteRound at the same
		// block height, so checking entries[0].Height is sufficient.
		entryHeight := entries[0].Height
		if currentHeight <= entryHeight || currentHeight-entryHeight <= windowBlocks {
			continue // still within window (or defensive guard against underflow)
		}

		// Expired: transfer escrowed tokens to treasury
		if k.bankKeeper != nil {
			totalEscrowed := new(big.Int)
			for _, entry := range entries {
				amt, _ := new(big.Int).SetString(entry.SlashAmount, 10)
				if amt != nil {
					totalEscrowed.Add(totalEscrowed, amt)
				}
			}
			if totalEscrowed.Sign() > 0 {
				coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(totalEscrowed)))
				if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.VindicationEscrowModuleName, "development_fund", coins); err != nil {
					k.Logger(ctx).Error("failed to transfer expired escrow to treasury", "fact_id", factId, "error", err)
				}
			}
		}

		k.DeleteVindicationPending(ctx, factId)

		sdkCtx := sdk.UnwrapSDKContext(ctx)
		// Forward-only audit: vindication windows close on schedule;
		// expiry is part of the immutable audit trail. See
		// TRUTH_SEEKING.md commitment 10.
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.vindication_expired",
			sdk.NewAttribute("fact_id", factId),
			sdk.NewAttribute("entry_count", fmt.Sprintf("%d", len(entries))),
			sdk.NewAttribute("creed_commitment", "10"),
		))
	}
}

// ─── Challenge Disproven Transition ──────────────────────────────────────────

// handleChallengeDisproven transitions the challenged fact to DISPROVEN
// when a challenge claim is accepted. Triggers vindication for the original
// fact's minority voters who were slashed during its verification round.
func (k Keeper) handleChallengeDisproven(ctx context.Context, challengeClaim *types.Claim, newFactId string) {
	if challengeClaim.ProvisionalFactId == "" {
		return
	}

	originalFact, found := k.GetFact(ctx, challengeClaim.ProvisionalFactId)
	if !found {
		return
	}

	// Contradiction check: same domain + explicit challenge link
	if originalFact.Domain != challengeClaim.Domain {
		return
	}

	// Transition to DISPROVEN.
	// Hegelian Dialectic Gap 4 — Finite Determination as Self-Negating:
	// set the forward pointer to the more adequate determination that replaced
	// this one. Every finite determination carries within itself the seed of
	// its own supersession; the forward link makes the succession chain
	// traversable (DISPROVEN → superseded_by → new Fact → ...).
	originalFact.Status = types.FactStatus_FACT_STATUS_DISPROVEN
	if newFactId != "" {
		originalFact.SupersededByFactId = newFactId
	}
	_ = k.SetFact(ctx, originalFact)

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// Popper, not popularity: a fact stops being a fact when a serious
	// attempt to disprove it succeeds. The chain announces the verdict
	// publicly so future training and citation runs can route around it.
	// See TRUTH_SEEKING.md commitment 3.
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.fact_disproven",
		sdk.NewAttribute("fact_id", originalFact.Id),
		sdk.NewAttribute("disproven_by", newFactId),
		sdk.NewAttribute("challenge_claim_id", challengeClaim.Id),
		sdk.NewAttribute("creed_commitment", "3"),
	))

	// Phase 5 feedback loop:
	//  · the disproven fact's submitter accrues a disproven_count
	//  · the challenger is credited with a successful challenge
	k.RecordDisprovalForSubmitter(ctx, originalFact.Submitter, originalFact.MethodId)
	k.RecordChallengeOutcome(ctx, challengeClaim.Submitter, true)

	// Falsification cascade (ToK Wave 5): mark direct descendants as CONTESTED
	// so they'll be re-examined rather than continuing to pose as validated.
	// Only first-hop descendants — transitive cascading is not done automatically
	// to avoid runaway invalidation. Governance can trigger deeper cascade via
	// a later message if needed.
	k.cascadeFalsification(ctx, originalFact.Id, challengeClaim.Id)

	// Trigger vindication for the ORIGINAL fact's minority voters
	k.ExecuteVindication(ctx, originalFact.Id, newFactId)
}

// cascadeFalsification marks every fact that directly supports its reasoning
// on the disproven fact as CONTESTED, so operators know the proof chain is
// compromised. Scope: only first-hop descendants via support-bearing edges
// (SUPPORTS / REQUIRES / REFINES / GENERALIZES / CITES). CONTRADICTS edges
// are ignored — they express disagreement with the disproven fact, which is
// now vindicated.
func (k Keeper) cascadeFalsification(ctx context.Context, disprovenFactId, challengeClaimId string) {
	incoming, err := k.GetIncomingRelations(ctx, disprovenFactId)
	if err != nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	affectedCount := 0
	for _, rel := range incoming {
		switch rel.Relation {
		case types.RelationType_RELATION_TYPE_SUPPORTS,
			types.RelationType_RELATION_TYPE_REQUIRES,
			types.RelationType_RELATION_TYPE_REFINES,
			types.RelationType_RELATION_TYPE_GENERALIZES,
			types.RelationType_RELATION_TYPE_CITES:
		default:
			continue
		}
		descendant, ok := k.GetFact(ctx, rel.SourceFactId)
		if !ok {
			continue
		}
		// Only flip VERIFIED/ACTIVE/AT_RISK facts. Don't re-contest already
		// CONTESTED/DISPROVEN/CHALLENGED facts — their status is informative.
		switch descendant.Status {
		case types.FactStatus_FACT_STATUS_VERIFIED,
			types.FactStatus_FACT_STATUS_ACTIVE,
			types.FactStatus_FACT_STATUS_AT_RISK:
		default:
			continue
		}
		descendant.Status = types.FactStatus_FACT_STATUS_CONTESTED
		_ = k.SetFact(ctx, descendant)
		affectedCount++
		// Popper, not popularity: when a fact falls, the chain
		// announces which facts inherited its provisional truth and
		// must be re-examined. The cascade is the substrate
		// admitting that what depended on a wrong fact also might
		// be wrong. See TRUTH_SEEKING.md commitment 3.
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.falsification_cascade",
			sdk.NewAttribute("descendant_fact_id", descendant.Id),
			sdk.NewAttribute("disproven_fact_id", disprovenFactId),
			sdk.NewAttribute("challenge_claim_id", challengeClaimId),
			sdk.NewAttribute("edge_relation", rel.Relation.String()),
			sdk.NewAttribute("creed_commitment", "3"),
		))
	}
	if affectedCount > 0 {
		// Aggregate cascade announcement for off-chain observers
		// that want one summary record per disproof. Same commitment
		// as per-descendant events above.
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.falsification_cascade_summary",
			sdk.NewAttribute("disproven_fact_id", disprovenFactId),
			sdk.NewAttribute("descendants_contested", fmt.Sprintf("%d", affectedCount)),
			sdk.NewAttribute("creed_commitment", "3"),
		))
	}
}

// ─── Execute Vindication ─────────────────────────────────────────────────────

// ExecuteVindication refunds minority voters from escrow, slashes the majority,
// distributes a bonus from the majority slash pool, and records immutable
// vindication records. Called when a fact is disproven via challenge.
func (k Keeper) ExecuteVindication(ctx context.Context, factId, disprovenBy string) {
	pending := k.GetVindicationPending(ctx, factId)
	if len(pending) == 0 {
		return
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	params, _ := k.GetParams(ctx)

	// Find the original round to identify majority voters
	roundId := pending[0].RoundId
	round, found := k.GetVerificationRound(ctx, roundId)
	if !found {
		return
	}

	// Build minority set
	minoritySet := make(map[string]bool)
	for _, entry := range pending {
		minoritySet[entry.Verifier] = true
	}

	// Identify majority voters: revealed voters NOT in minority
	var majorityVoters []string
	for _, reveal := range round.Reveals {
		if !minoritySet[reveal.Verifier] {
			majorityVoters = append(majorityVoters, reveal.Verifier)
		}
	}

	// Slash majority at VindicationSlashBps
	totalMajoritySlash := sdkmath.ZeroInt()
	if k.stakingKeeper != nil && params.VindicationSlashBps > 0 {
		for _, voter := range majorityVoters {
			slashed, err := k.stakingKeeper.SlashValidatorToModule(ctx, voter, params.VindicationSlashBps, types.ModuleName)
			if err == nil {
				totalMajoritySlash = totalMajoritySlash.Add(slashed)
			}
		}
	}

	// Calculate bonus pool: VindicationBonusBps percent of majority slash goes to minority
	bonusPool := sdkmath.ZeroInt()
	remainder := sdkmath.ZeroInt()
	if totalMajoritySlash.IsPositive() && params.VindicationBonusBps > 0 {
		bonusPool = totalMajoritySlash.MulRaw(int64(params.VindicationBonusBps)).QuoRaw(10_000)
		remainder = totalMajoritySlash.Sub(bonusPool)
	} else {
		remainder = totalMajoritySlash
	}

	// Send remainder to treasury (development_fund)
	if remainder.IsPositive() && k.bankKeeper != nil {
		treasuryCoins := sdk.NewCoins(sdk.NewCoin("uzrn", remainder))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "development_fund", treasuryCoins); err != nil {
			k.Logger(ctx).Error("failed to send vindication remainder to treasury", "fact_id", factId, "error", err)
		}
	}

	// Calculate total minority slash for proportional bonus distribution
	totalMinoritySlash := new(big.Int)
	for _, entry := range pending {
		amt, ok := new(big.Int).SetString(entry.SlashAmount, 10)
		if ok && amt != nil {
			totalMinoritySlash.Add(totalMinoritySlash, amt)
		}
	}

	// Refund each minority voter from escrow + distribute proportional bonus
	for _, entry := range pending {
		refundAmt, ok := new(big.Int).SetString(entry.SlashAmount, 10)
		if !ok || refundAmt == nil || refundAmt.Sign() <= 0 {
			continue
		}

		// Refund original slash from escrow back to the validator
		if k.bankKeeper != nil {
			addr, addrErr := sdk.AccAddressFromBech32(entry.Verifier)
			if addrErr == nil {
				refundCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(refundAmt)))
				if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.VindicationEscrowModuleName, addr, refundCoins); err != nil {
					k.Logger(ctx).Error("failed to refund escrowed slash", "verifier", entry.Verifier, "amount", refundAmt.String(), "error", err)
				}
			}
		}

		// Calculate proportional share of bonus pool
		bonusAmt := new(big.Int)
		if bonusPool.IsPositive() && totalMinoritySlash.Sign() > 0 {
			bonusAmt.Mul(bonusPool.BigInt(), refundAmt)
			bonusAmt.Div(bonusAmt, totalMinoritySlash)
			if bonusAmt.Sign() > 0 && k.bankKeeper != nil {
				addr, addrErr := sdk.AccAddressFromBech32(entry.Verifier)
				if addrErr == nil {
					bonusCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(bonusAmt)))
					if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, bonusCoins); err != nil {
						k.Logger(ctx).Error("failed to distribute vindication bonus", "verifier", entry.Verifier, "amount", bonusAmt.String(), "error", err)
					}
				}
			}
		}

		// Record immutable vindication outcome
		_ = k.SetVindicationRecord(ctx, factId, types.VindicationRecord{
			Verifier:     entry.Verifier,
			FactId:       factId,
			RefundAmount: refundAmt.String(),
			BonusAmount:  bonusAmt.String(),
			VindicatedAt: height,
			DisprovenBy:  disprovenBy,
			RoundId:      entry.RoundId,
		})
	}

	// Clean up pending entries — vindication is complete
	k.DeleteVindicationPending(ctx, factId)

	// Record role impact — the majority was wrong (R29-3)
	k.RecordVindicationRoleImpact(ctx, round, k.getDomainForFact(ctx, factId))

	// Emit summary event
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.vindication_executed",
		sdk.NewAttribute("fact_id", factId),
		sdk.NewAttribute("disproven_by", disprovenBy),
		sdk.NewAttribute("minority_count", fmt.Sprintf("%d", len(pending))),
		sdk.NewAttribute("majority_slashed", totalMajoritySlash.String()),
		sdk.NewAttribute("bonus_pool", bonusPool.String()),
	))
}
