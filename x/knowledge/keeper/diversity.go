package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// BPS is the basis-point scale: 1,000,000 = 100%.
const BPS = 1_000_000

// NeutralBPS is the neutral diversity score returned when no data exists.
const NeutralBPS = 500_000

// ─── Fixed-point log2 lookup table ───────────────────────────────────────────
//
// Maps pBPS (probability in BPS) → -log2(p / BPS) * BPS.
// Values are pre-computed to avoid floating-point in consensus.
// Between points we use linear interpolation.

type log2Entry struct {
	pBPS uint64 // probability in BPS (0..1_000_000)
	val  uint64 // -log2(p/BPS) * BPS
}

// log2Table is sorted ascending by pBPS.
var log2Table = []log2Entry{
	{10_000, 6_643_856},   // p=0.01
	{20_000, 5_643_856},   // p=0.02
	{50_000, 4_321_928},   // p=0.05
	{75_000, 3_736_966},   // p=0.075
	{100_000, 3_321_928},  // p=0.10
	{125_000, 3_000_000},  // p=0.125
	{150_000, 2_736_966},  // p=0.15
	{200_000, 2_321_928},  // p=0.20
	{250_000, 2_000_000},  // p=0.25
	{300_000, 1_736_966},  // p=0.30
	{350_000, 1_514_573},  // p=0.35
	{400_000, 1_321_928},  // p=0.40
	{450_000, 1_152_003},  // p=0.45
	{500_000, 1_000_000},  // p=0.50
	{600_000, 736_966},    // p=0.60
	{700_000, 514_573},    // p=0.70
	{750_000, 415_037},    // p=0.75
	{800_000, 321_928},    // p=0.80
	{900_000, 152_003},    // p=0.90
	{1_000_000, 0},        // p=1.00
}

// log2BPS returns -log2(pBPS / BPS) * BPS via table lookup with linear interpolation.
// Returns 0 for pBPS >= BPS, and a very large value for pBPS near 0.
func log2BPS(pBPS uint64) uint64 {
	if pBPS == 0 {
		return 0 // Convention: 0 * log2(0) = 0 in Shannon entropy
	}
	if pBPS >= BPS {
		return 0
	}

	// Binary search for the bracketing entries
	n := len(log2Table)

	// Below smallest table entry: extrapolate from first two points
	if pBPS < log2Table[0].pBPS {
		// Linear extrapolation from first two points
		p0, v0 := log2Table[0].pBPS, log2Table[0].val
		p1, v1 := log2Table[1].pBPS, log2Table[1].val
		// v0 > v1 since smaller p → larger -log2(p)
		slope := (v0 - v1) // per (p1 - p0) units of pBPS
		dp := p0 - pBPS
		denom := p1 - p0
		return v0 + (slope*dp)/denom
	}

	// Find bracketing interval
	idx := sort.Search(n, func(i int) bool {
		return log2Table[i].pBPS >= pBPS
	})

	// Exact match
	if idx < n && log2Table[idx].pBPS == pBPS {
		return log2Table[idx].val
	}

	// Interpolate between idx-1 and idx
	if idx == 0 {
		return log2Table[0].val
	}
	if idx >= n {
		return 0
	}

	lo := log2Table[idx-1]
	hi := log2Table[idx]

	// Linear interpolation: val decreases as pBPS increases
	dpTotal := hi.pBPS - lo.pBPS
	dpActual := pBPS - lo.pBPS
	dvTotal := lo.val - hi.val // positive since lo.val > hi.val

	return lo.val - (dvTotal*dpActual)/dpTotal
}

// ─── ComputeRoundEntropy ─────────────────────────────────────────────────────

// ComputeRoundEntropy computes Shannon entropy for a binary vote in BPS.
//
// H = pAccept * (-log2(pAccept)) + pReject * (-log2(pReject))
//
// All arithmetic is in BPS. Returns 0 for unanimous or empty votes.
// Result is capped at BPS (1,000,000).
func ComputeRoundEntropy(acceptCount, rejectCount uint64) uint64 {
	total := acceptCount + rejectCount
	if total == 0 {
		return 0
	}
	if acceptCount == 0 || rejectCount == 0 {
		return 0 // unanimous
	}

	// Probabilities in BPS
	pAccept := (acceptCount * BPS) / total
	pReject := (rejectCount * BPS) / total

	// H = pAccept/BPS * log2BPS(pAccept) + pReject/BPS * log2BPS(pReject)
	// To stay in integer BPS, compute: (pAccept * log2BPS(pAccept) + pReject * log2BPS(pReject)) / BPS
	term1 := pAccept * log2BPS(pAccept)
	term2 := pReject * log2BPS(pReject)

	entropy := (term1 + term2) / BPS

	if entropy > BPS {
		return BPS
	}
	return entropy
}

// ─── Record types (keeper-local, JSON-encoded) ───────────────────────────────

// RoundDiversityRecord stores per-round vote entropy and raw headcounts.
type RoundDiversityRecord struct {
	RoundID     string `json:"round_id"`
	Entropy     uint64 `json:"entropy"`
	AcceptCount uint64 `json:"accept_count"`
	RejectCount uint64 `json:"reject_count"`
	TotalVoters uint64 `json:"total_voters"`
	Domain      string `json:"domain"`
	Epoch       uint64 `json:"epoch"`
}

// DomainDiversityRecord stores per-domain, per-epoch aggregated diversity.
type DomainDiversityRecord struct {
	Domain         string `json:"domain"`
	Epoch          uint64 `json:"epoch"`
	AvgEntropy     uint64 `json:"avg_entropy"`
	RoundCount     uint64 `json:"round_count"`
	UnanimousCount uint64 `json:"unanimous_count"`
}

// ValidatorIndependenceRecord tracks how often a validator dissents from the majority.
type ValidatorIndependenceRecord struct {
	Validator     string `json:"validator"`
	TotalVotes    uint64 `json:"total_votes"`
	MinorityVotes uint64 `json:"minority_votes"`
	LastEpoch     uint64 `json:"last_epoch"`
}

// ConformityStreakRecord tracks consecutive low-diversity epochs for a domain.
type ConformityStreakRecord struct {
	Domain            string `json:"domain"`
	ConsecutiveEpochs uint64 `json:"consecutive_epochs"`
	LastEpoch         uint64 `json:"last_epoch"`
}

// ─── CRUD: RoundDiversity ────────────────────────────────────────────────────

func (k Keeper) SetRoundDiversity(ctx context.Context, roundID string, rec RoundDiversityRecord) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal RoundDiversityRecord: %w", err)
	}
	return store.Set(types.RoundDiversityKey(roundID), bz)
}

func (k Keeper) GetRoundDiversity(ctx context.Context, roundID string) (RoundDiversityRecord, bool, error) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.RoundDiversityKey(roundID))
	if err != nil {
		return RoundDiversityRecord{}, false, err
	}
	if bz == nil {
		return RoundDiversityRecord{}, false, nil
	}
	var rec RoundDiversityRecord
	if err := json.Unmarshal(bz, &rec); err != nil {
		return RoundDiversityRecord{}, false, fmt.Errorf("failed to unmarshal RoundDiversityRecord: %w", err)
	}
	return rec, true, nil
}

// ─── CRUD: DomainDiversity ───────────────────────────────────────────────────

func (k Keeper) SetDomainDiversity(ctx context.Context, domain string, epoch uint64, rec DomainDiversityRecord) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal DomainDiversityRecord: %w", err)
	}
	return store.Set(types.DomainDiversityKey(domain, epoch), bz)
}

func (k Keeper) GetDomainDiversity(ctx context.Context, domain string, epoch uint64) (DomainDiversityRecord, bool, error) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.DomainDiversityKey(domain, epoch))
	if err != nil {
		return DomainDiversityRecord{}, false, err
	}
	if bz == nil {
		return DomainDiversityRecord{}, false, nil
	}
	var rec DomainDiversityRecord
	if err := json.Unmarshal(bz, &rec); err != nil {
		return DomainDiversityRecord{}, false, fmt.Errorf("failed to unmarshal DomainDiversityRecord: %w", err)
	}
	return rec, true, nil
}

// ─── CRUD: ValidatorIndependence ─────────────────────────────────────────────

func (k Keeper) SetValidatorIndependence(ctx context.Context, validator string, rec ValidatorIndependenceRecord) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal ValidatorIndependenceRecord: %w", err)
	}
	return store.Set(types.ValidatorIndependenceKey(validator), bz)
}

func (k Keeper) GetValidatorIndependence(ctx context.Context, validator string) (ValidatorIndependenceRecord, bool, error) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ValidatorIndependenceKey(validator))
	if err != nil {
		return ValidatorIndependenceRecord{}, false, err
	}
	if bz == nil {
		return ValidatorIndependenceRecord{}, false, nil
	}
	var rec ValidatorIndependenceRecord
	if err := json.Unmarshal(bz, &rec); err != nil {
		return ValidatorIndependenceRecord{}, false, fmt.Errorf("failed to unmarshal ValidatorIndependenceRecord: %w", err)
	}
	return rec, true, nil
}

// CountValidatorsWithSufficientIndependence returns the number of validators
// whose independence score meets or exceeds minIndependenceBps and who have
// at least minRounds completed verification rounds. Used by the verifier
// independence gate (P-10a) to determine whether the fallback should apply.
//
// Score = (MinorityVotes / TotalVotes) × 1_000_000.
// A validator who always votes with the majority scores 0.
func (k Keeper) CountValidatorsWithSufficientIndependence(
	ctx context.Context,
	minIndependenceBps uint64,
	minRounds uint64,
) int {
	store := k.storeService.OpenKVStore(ctx)
	pfx := types.ValidatorIndependencePrefix
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return 0
	}
	defer iter.Close()

	count := 0
	for ; iter.Valid(); iter.Next() {
		var rec ValidatorIndependenceRecord
		if err := json.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if rec.TotalVotes < minRounds {
			continue // not enough history
		}
		score := rec.MinorityVotes * 1_000_000 / rec.TotalVotes
		if score >= minIndependenceBps {
			count++
		}
	}
	return count
}

// ─── CRUD: ConformityStreak ──────────────────────────────────────────────────

func (k Keeper) SetConformityStreak(ctx context.Context, domain string, rec ConformityStreakRecord) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal ConformityStreakRecord: %w", err)
	}
	return store.Set(types.ConformityStreakKey(domain), bz)
}

func (k Keeper) GetConformityStreak(ctx context.Context, domain string) (ConformityStreakRecord, bool, error) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ConformityStreakKey(domain))
	if err != nil {
		return ConformityStreakRecord{}, false, err
	}
	if bz == nil {
		return ConformityStreakRecord{}, false, nil
	}
	var rec ConformityStreakRecord
	if err := json.Unmarshal(bz, &rec); err != nil {
		return ConformityStreakRecord{}, false, fmt.Errorf("failed to unmarshal ConformityStreakRecord: %w", err)
	}
	return rec, true, nil
}

// ─── High-level operations ───────────────────────────────────────────────────

// RecordRoundDiversity computes entropy for a round's votes, stores the record,
// and indexes it in the domain-epoch index.
func (k Keeper) RecordRoundDiversity(ctx context.Context, roundID, domain string, acceptCount, rejectCount uint64) error {
	entropy := ComputeRoundEntropy(acceptCount, rejectCount)

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	var epoch uint64
	if params.FitnessEpochBlocks > 0 {
		epoch = height / params.FitnessEpochBlocks
	}

	rec := RoundDiversityRecord{
		RoundID:     roundID,
		Entropy:     entropy,
		AcceptCount: acceptCount,
		RejectCount: rejectCount,
		TotalVoters: acceptCount + rejectCount,
		Domain:      domain,
		Epoch:       epoch,
	}

	if err := k.SetRoundDiversity(ctx, roundID, rec); err != nil {
		return err
	}

	return k.indexRoundInDomainEpoch(ctx, domain, epoch, roundID)
}

// UpdateValidatorIndependence increments a validator's vote counters.
// If the validator's vote differs from the majority vote, it counts as a minority vote.
func (k Keeper) UpdateValidatorIndependence(ctx context.Context, validator, vote, majorityVote string) error {
	rec, found, err := k.GetValidatorIndependence(ctx, validator)
	if err != nil {
		return err
	}
	if !found {
		rec = ValidatorIndependenceRecord{
			Validator: validator,
		}
	}

	rec.TotalVotes++
	if vote != majorityVote {
		rec.MinorityVotes++
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.FitnessEpochBlocks > 0 {
		rec.LastEpoch = height / params.FitnessEpochBlocks
	}

	return k.SetValidatorIndependence(ctx, validator, rec)
}

// AggregateDomainDiversity iterates all rounds in a domain's epoch,
// computes the mean entropy, and stores the DomainDiversityRecord.
func (k Keeper) AggregateDomainDiversity(ctx context.Context, domain string, epoch uint64) error {
	store := k.storeService.OpenKVStore(ctx)
	pfx := types.DomainEpochRoundPrefix(domain, epoch)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return err
	}
	defer iter.Close()

	var totalEntropy uint64
	var roundCount uint64
	var unanimousCount uint64

	for ; iter.Valid(); iter.Next() {
		// Extract roundID from key
		roundID := string(iter.Key()[len(pfx):])
		rec, found, err := k.GetRoundDiversity(ctx, roundID)
		if err != nil || !found {
			continue
		}

		totalEntropy += rec.Entropy
		roundCount++
		if rec.Entropy == 0 {
			unanimousCount++
		}
	}

	if roundCount == 0 {
		return nil // nothing to aggregate
	}

	avgEntropy := totalEntropy / roundCount

	domRec := DomainDiversityRecord{
		Domain:         domain,
		Epoch:          epoch,
		AvgEntropy:     avgEntropy,
		RoundCount:     roundCount,
		UnanimousCount: unanimousCount,
	}

	return k.SetDomainDiversity(ctx, domain, epoch, domRec)
}

// IncrementConformityStreak increments the conformity streak for a domain.
func (k Keeper) IncrementConformityStreak(ctx context.Context, domain string, epoch uint64) error {
	rec, found, err := k.GetConformityStreak(ctx, domain)
	if err != nil {
		return err
	}
	if !found {
		rec = ConformityStreakRecord{
			Domain: domain,
		}
	}

	rec.ConsecutiveEpochs++
	rec.LastEpoch = epoch

	return k.SetConformityStreak(ctx, domain, rec)
}

// ResetConformityStreak resets the conformity streak for a domain to zero.
func (k Keeper) ResetConformityStreak(ctx context.Context, domain string) error {
	rec, _, err := k.GetConformityStreak(ctx, domain)
	if err != nil {
		return err
	}

	rec.Domain = domain
	rec.ConsecutiveEpochs = 0

	return k.SetConformityStreak(ctx, domain, rec)
}

// CheckConformityAlert checks if a domain's average entropy is below the
// conformity alert threshold for enough consecutive epochs, and emits an event.
func (k Keeper) CheckConformityAlert(ctx context.Context, domain string, avgEntropy, epoch uint64) error {
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	threshold := params.DiversityConformityAlertThreshold
	requiredEpochs := params.DiversityConformityAlertEpochs

	if avgEntropy < threshold {
		if err := k.IncrementConformityStreak(ctx, domain, epoch); err != nil {
			return err
		}

		rec, found, err := k.GetConformityStreak(ctx, domain)
		if err != nil {
			return err
		}
		if found && rec.ConsecutiveEpochs >= requiredEpochs {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.conformity_alert",
				sdk.NewAttribute("domain", domain),
				sdk.NewAttribute("consecutive_epochs", fmt.Sprintf("%d", rec.ConsecutiveEpochs)),
				sdk.NewAttribute("avg_entropy", fmt.Sprintf("%d", avgEntropy)),
				sdk.NewAttribute("threshold", fmt.Sprintf("%d", threshold)),
			))
		}
	} else {
		if err := k.ResetConformityStreak(ctx, domain); err != nil {
			return err
		}
	}

	return nil
}

// ProcessDiversity iterates all domains, aggregates diversity for the epoch,
// and checks conformity alerts.
func (k Keeper) ProcessDiversity(ctx context.Context, epoch uint64) error {
	var processErr error

	k.IterateDomains(ctx, func(domain *types.Domain) bool {
		if err := k.AggregateDomainDiversity(ctx, domain.Name, epoch); err != nil {
			processErr = err
			return true
		}

		rec, found, err := k.GetDomainDiversity(ctx, domain.Name, epoch)
		if err != nil {
			processErr = err
			return true
		}

		if found {
			if err := k.CheckConformityAlert(ctx, domain.Name, rec.AvgEntropy, epoch); err != nil {
				processErr = err
				return true
			}
		}

		return false
	})

	return processErr
}

// GetGlobalConsensusDiversity computes the average diversity across all domains
// for the most recent epoch that has data. Returns NeutralBPS (500,000) if no data.
// Uses O(domains) direct epoch lookup instead of scanning all historical epochs.
func (k Keeper) GetGlobalConsensusDiversity(ctx context.Context) uint64 {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	params, err := k.GetParams(ctx)
	if err != nil || params.FitnessEpochBlocks == 0 || height == 0 {
		return NeutralBPS
	}

	epoch := height / params.FitnessEpochBlocks

	// Try current epoch, then previous epoch (data may not exist yet for current)
	for try := uint64(0); try < 2; try++ {
		if epoch < try {
			break
		}
		checkEpoch := epoch - try

		var totalEntropy, domainCount uint64
		k.IterateDomains(ctx, func(domain *types.Domain) bool {
			rec, found, getErr := k.GetDomainDiversity(ctx, domain.Name, checkEpoch)
			if getErr == nil && found && rec.RoundCount > 0 {
				totalEntropy += rec.AvgEntropy
				domainCount++
			}
			return false
		})

		if domainCount > 0 {
			return totalEntropy / domainCount
		}
	}

	return NeutralBPS
}

// verdictToVoteString maps a verdict enum to the corresponding vote string.
func verdictToVoteString(v types.Verdict) string {
	switch v {
	case types.Verdict_VERDICT_ACCEPT:
		return "accept"
	case types.Verdict_VERDICT_REJECT:
		return "reject"
	case types.Verdict_VERDICT_MALFORMED:
		return "malformed"
	default:
		return ""
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// indexRoundInDomainEpoch stores roundID in the domain/epoch/roundID index.
func (k Keeper) indexRoundInDomainEpoch(ctx context.Context, domain string, epoch uint64, roundID string) error {
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.DomainEpochRoundKey(domain, epoch, roundID), []byte{0x01})
}
