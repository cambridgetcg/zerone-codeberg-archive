package keeper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"google.golang.org/protobuf/proto"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// marshalOpts forces deterministic proto encoding for store writes.
var marshalOpts = proto.MarshalOptions{Deterministic: true}

// ─── Params ──────────────────────────────────────────────────────────────────

func (k Keeper) SetParams(ctx context.Context, params *types.Params) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	return store.Set(types.ParamsKey, bz)
}

func (k Keeper) GetParams(ctx context.Context) (*types.Params, error) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ParamsKey)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		p := types.DefaultParams()
		return &p, nil
	}
	var params types.Params
	if err := proto.Unmarshal(bz, &params); err != nil {
		p := types.DefaultParams()
		return &p, nil
	}
	return &params, nil
}

// ─── Fact CRUD ───────────────────────────────────────────────────────────────

func (k Keeper) SetFact(ctx context.Context, fact *types.Fact) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(fact)
	if err != nil {
		return fmt.Errorf("failed to marshal fact: %w", err)
	}
	if err := store.Set(types.FactKey(fact.Id), bz); err != nil {
		return err
	}
	// Secondary indexes
	if fact.Submitter != "" {
		_ = store.Set(types.FactBySubmitterKey(fact.Submitter, fact.Id), []byte{0x01})
	}
	if fact.Domain != "" {
		_ = store.Set(types.FactByDomainKey(fact.Domain, fact.Id), []byte{0x01})
	}
	return nil
}

func (k Keeper) GetFact(ctx context.Context, id string) (*types.Fact, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.FactKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var fact types.Fact
	if err := proto.Unmarshal(bz, &fact); err != nil {
		return nil, false
	}
	return &fact, true
}

func (k Keeper) DeleteFact(ctx context.Context, id string) error {
	fact, found := k.GetFact(ctx, id)
	if !found {
		return nil
	}
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.FactKey(id))
	if fact.Submitter != "" {
		_ = store.Delete(types.FactBySubmitterKey(fact.Submitter, id))
	}
	if fact.Domain != "" {
		_ = store.Delete(types.FactByDomainKey(fact.Domain, id))
	}
	return nil
}

func (k Keeper) IterateFacts(ctx context.Context, cb func(fact *types.Fact) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.FactKeyPrefix, prefixEndBytes(types.FactKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var fact types.Fact
		if err := proto.Unmarshal(iter.Value(), &fact); err != nil {
			continue
		}
		if cb(&fact) {
			break
		}
	}
}

func (k Keeper) IterateFactsByDomain(ctx context.Context, domain string, cb func(factID string) bool) {
	store := k.storeService.OpenKVStore(ctx)
	pfx := append(append([]byte{}, types.DomainFactIndexPrefix...), []byte(domain+"/")...)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		// Key is prefix+domain+"/"+factID — extract factID from after prefix
		key := iter.Key()
		factID := string(key[len(pfx):])
		if cb(factID) {
			break
		}
	}
}

func (k Keeper) IterateFactsBySubmitter(ctx context.Context, submitter string, cb func(factID string) bool) {
	store := k.storeService.OpenKVStore(ctx)
	pfx := append(append([]byte{}, types.FactBySubmitterIndexPrefix...), []byte(submitter+"/")...)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key := iter.Key()
		factID := string(key[len(pfx):])
		if cb(factID) {
			break
		}
	}
}

// ─── Claim CRUD ──────────────────────────────────────────────────────────────

func (k Keeper) SetClaim(ctx context.Context, claim *types.Claim) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(claim)
	if err != nil {
		return fmt.Errorf("failed to marshal claim: %w", err)
	}
	if err := store.Set(types.ClaimKey(claim.Id), bz); err != nil {
		return err
	}
	// Content hash dedup index
	if claim.ContentHash != "" {
		_ = store.Set(types.ContentHashKey(claim.ContentHash), []byte(claim.Id))
	}
	// Canonical hash dedup index
	if claim.CanonicalHash != "" {
		_ = store.Set(types.CanonicalHashKey(claim.CanonicalHash), []byte(claim.Id))
	}
	return nil
}

func (k Keeper) GetClaim(ctx context.Context, id string) (*types.Claim, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ClaimKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var claim types.Claim
	if err := proto.Unmarshal(bz, &claim); err != nil {
		return nil, false
	}
	return &claim, true
}

func (k Keeper) DeleteClaim(ctx context.Context, id string) error {
	store := k.storeService.OpenKVStore(ctx)
	return store.Delete(types.ClaimKey(id))
}

func (k Keeper) IterateClaims(ctx context.Context, cb func(claim *types.Claim) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.ClaimKeyPrefix, prefixEndBytes(types.ClaimKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var claim types.Claim
		if err := proto.Unmarshal(iter.Value(), &claim); err != nil {
			continue
		}
		if cb(&claim) {
			break
		}
	}
}

// GetClaimByContentHash looks up a claim ID by its content hash (dedup).
func (k Keeper) GetClaimByContentHash(ctx context.Context, hash string) (string, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ContentHashKey(hash))
	if err != nil || bz == nil {
		return "", false
	}
	return string(bz), true
}

// ─── VerificationRound CRUD ─────────────────────────────────────────────────

func (k Keeper) SetVerificationRound(ctx context.Context, round *types.VerificationRound) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(round)
	if err != nil {
		return fmt.Errorf("failed to marshal round: %w", err)
	}
	if err := store.Set(types.RoundKey(round.Id), bz); err != nil {
		return err
	}
	// Claim→round index
	if round.ClaimId != "" {
		_ = store.Set(types.ClaimRoundIndexKey(round.ClaimId), []byte(round.Id))
	}
	// Active round index (for BeginBlocker iteration)
	activeKey := activeRoundKey(round.Id)
	if round.Phase != types.VerificationPhase_VERIFICATION_PHASE_COMPLETE &&
		round.Phase != types.VerificationPhase_VERIFICATION_PHASE_EXPIRED {
		_ = store.Set(activeKey, []byte{0x01})
	} else {
		_ = store.Delete(activeKey)
	}
	return nil
}

func (k Keeper) GetVerificationRound(ctx context.Context, id string) (*types.VerificationRound, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.RoundKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var round types.VerificationRound
	if err := proto.Unmarshal(bz, &round); err != nil {
		return nil, false
	}
	return &round, true
}

func (k Keeper) DeleteVerificationRound(ctx context.Context, id string) error {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(activeRoundKey(id))
	return store.Delete(types.RoundKey(id))
}

func (k Keeper) GetRoundByClaimID(ctx context.Context, claimID string) (*types.VerificationRound, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.ClaimRoundIndexKey(claimID))
	if err != nil || bz == nil {
		return nil, false
	}
	return k.GetVerificationRound(ctx, string(bz))
}

func (k Keeper) IterateActiveRounds(ctx context.Context, cb func(round *types.VerificationRound) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.ActiveRoundIndexPrefix, prefixEndBytes(types.ActiveRoundIndexPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		roundID := string(iter.Key()[len(types.ActiveRoundIndexPrefix):])
		round, found := k.GetVerificationRound(ctx, roundID)
		if !found {
			continue
		}
		if cb(round) {
			break
		}
	}
}

// ─── Domain CRUD ─────────────────────────────────────────────────────────────

func (k Keeper) SetDomain(ctx context.Context, domain *types.Domain) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(domain)
	if err != nil {
		return fmt.Errorf("failed to marshal domain: %w", err)
	}
	return store.Set(types.DomainKey(domain.Name), bz)
}

func (k Keeper) GetDomain(ctx context.Context, name string) (*types.Domain, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.DomainKey(name))
	if err != nil || bz == nil {
		return nil, false
	}
	var domain types.Domain
	if err := proto.Unmarshal(bz, &domain); err != nil {
		return nil, false
	}
	return &domain, true
}

func (k Keeper) IterateDomains(ctx context.Context, cb func(domain *types.Domain) bool) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.DomainKeyPrefix, prefixEndBytes(types.DomainKeyPrefix))
	if err != nil {
		return
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var domain types.Domain
		if err := proto.Unmarshal(iter.Value(), &domain); err != nil {
			continue
		}
		if cb(&domain) {
			break
		}
	}
}

// ─── ID Generation ───────────────────────────────────────────────────────────

// ComputeClaimContentHash returns the SHA-256 hex of domain-tagged claim content.
func ComputeClaimContentHash(content, domain string) string {
	h := sha256.New()
	h.Write([]byte("ZRN.claim.v1:"))
	h.Write([]byte(domain))
	h.Write([]byte(":"))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateClaimID generates a deterministic claim ID from submitter, content hash, and block.
func GenerateClaimID(submitter, contentHash string, height uint64) string {
	h := sha256.New()
	h.Write([]byte("ZRN.claim.id.v1:"))
	h.Write([]byte(submitter))
	h.Write([]byte(":"))
	h.Write([]byte(contentHash))
	h.Write([]byte(fmt.Sprintf(":%d", height)))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// GenerateFactID generates a deterministic fact ID from claim ID and block.
func GenerateFactID(claimID string, height uint64) string {
	h := sha256.New()
	h.Write([]byte("ZRN.fact.id.v1:"))
	h.Write([]byte(claimID))
	h.Write([]byte(fmt.Sprintf(":%d", height)))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// GenerateRoundID generates a deterministic round ID from claim ID and block.
func GenerateRoundID(claimID string, height uint64) string {
	h := sha256.New()
	h.Write([]byte("ZRN.round.id.v1:"))
	h.Write([]byte(claimID))
	h.Write([]byte(fmt.Sprintf(":%d", height)))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// ─── Commit/Reveal helpers ───────────────────────────────────────────────────

func findCommitByVerifier(commits []*types.CommitEntry, verifier string) *types.CommitEntry {
	for _, c := range commits {
		if c.Verifier == verifier {
			return c
		}
	}
	return nil
}

func findRevealByVerifier(reveals []*types.RevealEntry, verifier string) *types.RevealEntry {
	for _, r := range reveals {
		if r.Verifier == verifier {
			return r
		}
	}
	return nil
}

// ─── ABCI commit/reveal storage ──────────────────────────────────────────

// StoreCommitmentInRound stores a commitment entry in a verification round.
// Idempotent for duplicate commitments (same hash); returns ErrEquivocation
// for conflicting commitments from the same verifier.
func (k Keeper) StoreCommitmentInRound(ctx context.Context, roundID string, commit *types.CommitEntry) error {
	round, found := k.GetVerificationRound(ctx, roundID)
	if !found {
		return types.ErrRoundNotFound
	}
	if round.Phase != types.VerificationPhase_VERIFICATION_PHASE_COMMIT {
		return types.ErrRoundNotInCommitPhase
	}

	existing := findCommitByVerifier(round.Commits, commit.Verifier)
	if existing != nil {
		if bytes.Equal(existing.CommitHash, commit.CommitHash) {
			return types.ErrDuplicateCommitment
		}
		return types.ErrEquivocation
	}

	round.Commits = append(round.Commits, commit)

	// Add to selected verifiers if new
	isNew := true
	for _, v := range round.SelectedVerifiers {
		if v == commit.Verifier {
			isNew = false
			break
		}
	}
	if isNew {
		round.SelectedVerifiers = append(round.SelectedVerifiers, commit.Verifier)
	}

	return k.SetVerificationRound(ctx, round)
}

// StoreRevealInRound stores a reveal entry in a verification round.
// Verifies the reveal matches the prior commitment hash using the provided confidence,
// then persists confidence in reveal.ConfidenceBps for downstream Brier-score
// verifier calibration (Wave 4). Confidence is part of the commitment hash preimage,
// so it is verified here against the commitment before being stored.
func (k Keeper) StoreRevealInRound(ctx context.Context, roundID string, reveal *types.RevealEntry, confidence uint64) error {
	round, found := k.GetVerificationRound(ctx, roundID)
	if !found {
		return types.ErrRoundNotFound
	}
	if round.Phase != types.VerificationPhase_VERIFICATION_PHASE_REVEAL {
		return types.ErrRoundNotInRevealPhase
	}

	// Find matching commit
	commit := findCommitByVerifier(round.Commits, reveal.Verifier)
	if commit == nil {
		return types.ErrNoCommitment
	}

	// Verify reveal matches commitment hash
	if !types.VerifyCommitmentHash(commit.CommitHash, roundID, reveal.Vote, confidence, reveal.Salt) {
		return types.ErrRevealMismatch
	}

	// Check for existing reveal
	existing := findRevealByVerifier(round.Reveals, reveal.Verifier)
	if existing != nil {
		if existing.Vote == reveal.Vote && bytes.Equal(existing.Salt, reveal.Salt) {
			return types.ErrDuplicateReveal
		}
		return types.ErrEquivocation
	}

	// Persist confidence in the reveal entry now that it has been verified
	// against the commitment hash. This enables downstream Brier-score
	// verifier calibration (Wave 4).
	reveal.ConfidenceBps = confidence

	round.Reveals = append(round.Reveals, reveal)
	return k.SetVerificationRound(ctx, round)
}

// GetActiveRounds returns all active (non-complete/expired) verification rounds.
func (k Keeper) GetActiveRounds(ctx context.Context) []*types.VerificationRound {
	var rounds []*types.VerificationRound
	k.IterateActiveRounds(ctx, func(round *types.VerificationRound) bool {
		rounds = append(rounds, round)
		return false
	})
	return rounds
}

// ─── Fact Relation CRUD ──────────────────────────────────────────────────────

// SetFactRelation stores a fact relation with dual-write (forward + reverse index).
func (k Keeper) SetFactRelation(ctx context.Context, rel *types.FactRelation) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := marshalOpts.Marshal(rel)
	if err != nil {
		return fmt.Errorf("failed to marshal fact relation: %w", err)
	}
	// Forward index: source → target
	if err := store.Set(types.FactRelationKey(rel.SourceFactId, rel.TargetFactId), bz); err != nil {
		return err
	}
	// Reverse index: target → source
	return store.Set(types.FactRelationReverseKey(rel.TargetFactId, rel.SourceFactId), bz)
}

// GetFactRelations returns all outgoing relations from a fact.
func (k Keeper) GetFactRelations(ctx context.Context, factID string) ([]*types.FactRelation, error) {
	return k.iterateRelationsWithPrefix(ctx, types.FactRelationsBySourcePrefix(factID))
}

// GetIncomingRelations returns all incoming relations pointing to a fact.
func (k Keeper) GetIncomingRelations(ctx context.Context, factID string) ([]*types.FactRelation, error) {
	return k.iterateRelationsWithPrefix(ctx, types.FactRelationsByTargetPrefix(factID))
}

// GetRelationsByType returns outgoing relations from a fact filtered by type.
func (k Keeper) GetRelationsByType(ctx context.Context, factID string, relType types.RelationType) ([]*types.FactRelation, error) {
	all, err := k.GetFactRelations(ctx, factID)
	if err != nil {
		return nil, err
	}
	var filtered []*types.FactRelation
	for _, rel := range all {
		if rel.Relation == relType {
			filtered = append(filtered, rel)
		}
	}
	return filtered, nil
}

func (k Keeper) iterateRelationsWithPrefix(ctx context.Context, pfx []byte) ([]*types.FactRelation, error) {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var relations []*types.FactRelation
	for ; iter.Valid(); iter.Next() {
		var rel types.FactRelation
		if err := proto.Unmarshal(iter.Value(), &rel); err != nil {
			continue
		}
		relations = append(relations, &rel)
	}
	return relations, nil
}

// ─── Structured claim indexes ────────────────────────────────────────────────

// normalizeSubject lowercases and trims a subject for consistent indexing.
func normalizeSubject(subject string) string {
	return strings.ToLower(strings.TrimSpace(subject))
}

// subjectHash returns a hex SHA-256 hash of the normalized subject for use as a store key.
func subjectHash(subject string) string {
	h := sha256.Sum256([]byte(normalizeSubject(subject)))
	return hex.EncodeToString(h[:])
}

// IndexFactBySubject indexes a fact by its structured subject and tags.
// Called after creating a fact from a claim that has structure.
func (k Keeper) IndexFactBySubject(ctx context.Context, fact *types.Fact) error {
	if fact.Structure == nil {
		return nil
	}
	store := k.storeService.OpenKVStore(ctx)

	// Subject index: domain/subject_hash → fact_id
	if fact.Structure.Subject != "" {
		key := types.FactSubjectKey(fact.Domain, subjectHash(fact.Structure.Subject))
		if err := store.Set(key, []byte(fact.Id)); err != nil {
			return err
		}
	}

	// Tag index: tag/fact_id → 0x01
	for _, tag := range fact.Structure.Tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		key := types.FactTagKey(normalized, fact.Id)
		if err := store.Set(key, []byte{0x01}); err != nil {
			return err
		}
	}
	return nil
}

// FindFactBySubjectPredicate finds an existing fact with the same subject in a domain.
// Returns the fact ID if found, empty string otherwise.
// Note: only matches by subject hash (not predicate) since the index is subject-based.
// Predicate matching is done by loading the fact and comparing.
func (k Keeper) FindFactBySubjectPredicate(ctx context.Context, domain, subject, predicate string) string {
	store := k.storeService.OpenKVStore(ctx)
	key := types.FactSubjectKey(domain, subjectHash(subject))
	bz, err := store.Get(key)
	if err != nil || bz == nil {
		return ""
	}
	factID := string(bz)

	// If predicate matching is requested, verify the fact's predicate matches
	if predicate != "" {
		fact, found := k.GetFact(ctx, factID)
		if !found || fact.Structure == nil {
			return ""
		}
		if normalizeSubject(fact.Structure.Predicate) != normalizeSubject(predicate) {
			return ""
		}
	}
	return factID
}

// FindFactsByTag returns all fact IDs tagged with the given tag.
func (k Keeper) FindFactsByTag(ctx context.Context, tag string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(tag))
	if normalized == "" {
		return nil, nil
	}
	store := k.storeService.OpenKVStore(ctx)
	pfx := types.FactTagsByTagPrefix(normalized)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var factIDs []string
	for ; iter.Valid(); iter.Next() {
		factID := string(iter.Key()[len(pfx):])
		factIDs = append(factIDs, factID)
	}
	return factIDs, nil
}

// ─── Canonical hash index ─────────────────────────────────────────────────

// SetCanonicalHash stores a canonical hash → id mapping for dedup.
func (k Keeper) SetCanonicalHash(ctx context.Context, hash string, id string) error {
	store := k.storeService.OpenKVStore(ctx)
	return store.Set(types.CanonicalHashKey(hash), []byte(id))
}

// GetClaimByCanonicalHash looks up a claim/fact ID by its canonical hash.
func (k Keeper) GetClaimByCanonicalHash(ctx context.Context, hash string) (string, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.CanonicalHashKey(hash))
	if err != nil || bz == nil {
		return "", false
	}
	return string(bz), true
}

// ─── Bootstrap fund tracking (R19-7) ─────────────────────────────────────────

// GetBootstrapClaimCount returns the lifetime count of sponsored claims for an address.
func (k Keeper) GetBootstrapClaimCount(ctx context.Context, address string) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	key := append(append([]byte{}, types.BootstrapClaimCountPrefix...), []byte(address)...)
	bz, err := store.Get(key)
	if err != nil || bz == nil || len(bz) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// IncrementBootstrapClaimCount increments the lifetime sponsored claim count for an address.
func (k Keeper) IncrementBootstrapClaimCount(ctx context.Context, address string) error {
	store := k.storeService.OpenKVStore(ctx)
	key := append(append([]byte{}, types.BootstrapClaimCountPrefix...), []byte(address)...)
	count := k.GetBootstrapClaimCount(ctx, address) + 1
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, count)
	return store.Set(key, bz)
}

// GetBootstrapEpochCount returns the number of sponsored claims in a given epoch.
func (k Keeper) GetBootstrapEpochCount(ctx context.Context, epoch uint64) uint64 {
	store := k.storeService.OpenKVStore(ctx)
	epochBz := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBz, epoch)
	key := append(append([]byte{}, types.BootstrapEpochCountPrefix...), epochBz...)
	bz, err := store.Get(key)
	if err != nil || bz == nil || len(bz) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(bz)
}

// IncrementBootstrapEpochCount increments the sponsored claim count for a given epoch.
func (k Keeper) IncrementBootstrapEpochCount(ctx context.Context, epoch uint64) error {
	store := k.storeService.OpenKVStore(ctx)
	epochBz := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBz, epoch)
	key := append(append([]byte{}, types.BootstrapEpochCountPrefix...), epochBz...)
	count := k.GetBootstrapEpochCount(ctx, epoch) + 1
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, count)
	return store.Set(key, bz)
}

// GetBootstrapFundBalance returns the current balance of the bootstrap fund module account.
func (k Keeper) GetBootstrapFundBalance(ctx context.Context) sdk.Coin {
	if k.bankKeeper == nil {
		return sdk.NewInt64Coin("uzrn", 0)
	}
	addr := authtypes.NewModuleAddress(types.BootstrapFundModuleName)
	return k.bankKeeper.GetBalance(ctx, addr, "uzrn")
}

// CurrentEpoch returns the current epoch number based on block height and epoch length.
func (k Keeper) CurrentEpoch(ctx context.Context) uint64 {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	params, err := k.GetParams(ctx)
	if err != nil || params.BootstrapFundEpochBlocks == 0 {
		return 0
	}
	return height / params.BootstrapFundEpochBlocks
}

// ─── Store helpers ───────────────────────────────────────────────────────────

func activeRoundKey(roundID string) []byte {
	return append(append([]byte{}, types.ActiveRoundIndexPrefix...), []byte(roundID)...)
}

// ─── Niche Index CRUD ─────────────────────────────────────────────────────────

// ComputeNicheKey returns hash(domain + subject + claim_type) for a fact.
// Facts without structure are each in their own niche (no competition).
func (k Keeper) ComputeNicheKey(fact *types.Fact) string {
	if fact.Structure == nil || fact.Structure.Subject == "" {
		// Unstructured facts: solo niche keyed by fact ID
		return "solo:" + fact.Id
	}
	h := sha256.New()
	h.Write([]byte("ZRN.niche.v1:"))
	h.Write([]byte(fact.Domain))
	h.Write([]byte(":"))
	h.Write([]byte(normalizeSubject(fact.Structure.Subject)))
	h.Write([]byte(":"))
	h.Write([]byte(fact.ClaimType.String()))
	// Scope differentiates sub-niches
	if fact.Structure.Scope != "" {
		h.Write([]byte(":"))
		h.Write([]byte(strings.ToLower(strings.TrimSpace(fact.Structure.Scope))))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// UpdateNicheIndex registers a fact in its niche index.
func (k Keeper) UpdateNicheIndex(ctx context.Context, fact *types.Fact) error {
	nicheKey := k.ComputeNicheKey(fact)
	fact.NicheKey = nicheKey
	store := k.storeService.OpenKVStore(ctx)
	// Register fact in niche
	if err := store.Set(types.NicheIndexKey(nicheKey, fact.Id), []byte{0x01}); err != nil {
		return err
	}
	// Register niche existence
	if err := store.Set(types.NicheMembersKey(nicheKey), []byte{0x01}); err != nil {
		return err
	}
	return nil
}

// RemoveFromNicheIndex removes a fact from its niche index.
func (k Keeper) RemoveFromNicheIndex(ctx context.Context, nicheKey, factID string) {
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.NicheIndexKey(nicheKey, factID))
}

// GetNicheMembers returns all active facts in a niche, sorted by fitness descending.
func (k Keeper) GetNicheMembers(ctx context.Context, nicheKey string) []*types.Fact {
	store := k.storeService.OpenKVStore(ctx)
	pfx := types.NicheIndexByNichePrefix(nicheKey)
	iter, err := store.Iterator(pfx, prefixEndBytes(pfx))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var members []*types.Fact
	for ; iter.Valid(); iter.Next() {
		factID := string(iter.Key()[len(pfx):])
		fact, found := k.GetFact(ctx, factID)
		if !found {
			continue
		}
		// Only include active/verified/provisional facts
		if fact.Status == types.FactStatus_FACT_STATUS_VERIFIED ||
			fact.Status == types.FactStatus_FACT_STATUS_ACTIVE ||
			fact.Status == types.FactStatus_FACT_STATUS_PROVISIONAL ||
			fact.Status == types.FactStatus_FACT_STATUS_AT_RISK {
			members = append(members, fact)
		}
	}
	return members
}

// GetNicheLeader returns the highest-fitness fact in a niche.
func (k Keeper) GetNicheLeader(ctx context.Context, nicheKey string) (*types.Fact, bool) {
	members := k.GetNicheMembers(ctx, nicheKey)
	if len(members) == 0 {
		return nil, false
	}
	// Find highest fitness
	leader := members[0]
	for _, m := range members[1:] {
		if m.FitnessScore > leader.FitnessScore {
			leader = m
		}
	}
	return leader, true
}

// GetAllNiches returns all registered niche keys.
func (k Keeper) GetAllNiches(ctx context.Context) []string {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.NicheMembersPrefix, prefixEndBytes(types.NicheMembersPrefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var niches []string
	for ; iter.Valid(); iter.Next() {
		nicheKey := string(iter.Key()[len(types.NicheMembersPrefix):])
		niches = append(niches, nicheKey)
	}
	return niches
}

// prefixEndBytes returns the exclusive end key for prefix iteration.
func prefixEndBytes(pfx []byte) []byte {
	if len(pfx) == 0 {
		return nil
	}
	end := make([]byte, len(pfx))
	copy(end, pfx)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil // overflow: 0xFF...FF → nil means no upper bound
}
