package keeper

// ─── Scientific Method Gap 3: Submission Bias / Pre-registration ─────────────
//
// Pre-registration corrects the p-hacking problem for on-chain claims.
// A claim submitter can test many candidate claims off-chain before submitting
// the one most likely to pass verification. The chain sees only the "published"
// claim, not the distribution of candidates. If verification pass rates are
// correlated with truth but not equal to it, selection on "likely to pass"
// introduces bias: the corpus overrepresents claims that LOOK true under the
// verification methodology, not claims that ARE true.
//
// The scientific method's solution (pre-registration):
//   Declare the hypothesis BEFORE collecting evidence. The temporal anchor is
//   unforgeable — the on-chain block height at pre-registration time proves
//   the hypothesis predated the evidence. When the claim is later submitted
//   referencing the pre-registration, any observer can verify the claim intent
//   was committed before the result was known.
//
// Bond mechanics:
//   Skin-in-the-game for pre-registration honesty. Without a bond, submitters
//   can pre-register everything speculatively and discard failed hypotheses.
//   With a bond: forfeiture on expiry makes selective disclosure costly.
//
// The analogy: ClinicalTrials.gov for knowledge verification.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmath "cosmossdk.io/math"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── State accessors ──────────────────────────────────────────────────────────

// SetPreRegistration stores a PreRegistration record and maintains the expiry
// index when expiry_block > 0.
func (k Keeper) SetPreRegistration(ctx context.Context, pr *types.PreRegistration) error {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := k.cdc.Marshal(pr)
	if err != nil {
		return fmt.Errorf("marshal PreRegistration: %w", err)
	}
	if err := store.Set(types.PreRegistrationKey(pr.Id), bz); err != nil {
		return fmt.Errorf("store PreRegistration: %w", err)
	}
	// Maintain expiry index (only when expiry is configured).
	if pr.ExpiryBlock > 0 && !pr.Fulfilled && !pr.Expired {
		return store.Set(types.PreRegistrationExpiryKey(pr.ExpiryBlock, pr.Id), []byte{1})
	}
	return nil
}

// GetPreRegistration retrieves a PreRegistration by id.
func (k Keeper) GetPreRegistration(ctx context.Context, id string) (*types.PreRegistration, bool) {
	store := k.storeService.OpenKVStore(ctx)
	bz, err := store.Get(types.PreRegistrationKey(id))
	if err != nil || bz == nil {
		return nil, false
	}
	var pr types.PreRegistration
	if err := k.cdc.Unmarshal(bz, &pr); err != nil {
		return nil, false
	}
	return &pr, true
}

// DeletePreRegistrationExpiryIndex removes the expiry index entry for a
// pre-registration. Called when marking fulfilled or expired.
func (k Keeper) DeletePreRegistrationExpiryIndex(ctx context.Context, expiryBlock uint64, id string) {
	if expiryBlock == 0 {
		return
	}
	store := k.storeService.OpenKVStore(ctx)
	_ = store.Delete(types.PreRegistrationExpiryKey(expiryBlock, id))
}

// GetAllPreRegistrations iterates all pre-registration records. Used for
// genesis export.
func (k Keeper) GetAllPreRegistrations(ctx context.Context) []*types.PreRegistration {
	store := k.storeService.OpenKVStore(ctx)
	iter, err := store.Iterator(types.PreRegistrationKeyPrefix, prefixEndBytes(types.PreRegistrationKeyPrefix))
	if err != nil {
		return nil
	}
	defer iter.Close()

	var result []*types.PreRegistration
	for ; iter.Valid(); iter.Next() {
		var pr types.PreRegistration
		if err := k.cdc.Unmarshal(iter.Value(), &pr); err != nil {
			continue
		}
		result = append(result, &pr)
	}
	return result
}

// ── ID generation ────────────────────────────────────────────────────────────

// generatePreRegistrationID creates a unique, deterministic pre-registration ID.
// Format: "prereg-{first8hexOfHash}" where hash = sha256(submitter || block).
func generatePreRegistrationID(submitter string, blockHeight uint64) string {
	h := sha256.New()
	h.Write([]byte(submitter))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockHeight)
	h.Write(buf[:])
	digest := h.Sum(nil)
	return "prereg-" + hex.EncodeToString(digest[:8])
}

// ── Expiry processing ────────────────────────────────────────────────────────

// ProcessExpiredPreRegistrations is called from BeginBlocker. Scans the expiry
// index for pre-registrations whose expiry_block <= current block height,
// forfeits bonds to the research fund, and marks them expired.
func (k Keeper) ProcessExpiredPreRegistrations(ctx context.Context) {
	params, err := k.GetParams(ctx)
	if err != nil || params.PreRegistrationExpiryBlocks == 0 {
		return // expiry disabled
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentBlock := uint64(sdkCtx.BlockHeight())

	store := k.storeService.OpenKVStore(ctx)

	// Scan expiry index: prefix 0x7B, all entries with expiryBlock ≤ currentBlock.
	// Keys are sorted big-endian by block, so we iterate from the beginning and
	// stop as soon as we hit an entry with block > currentBlock.
	iter, err := store.Iterator(
		types.PreRegistrationExpiryIndexPrefix,
		prefixEndBytes(types.PreRegistrationExpiryIndexPrefix),
	)
	if err != nil {
		return
	}

	// Collect IDs to process (avoid modifying state while iterating).
	type expiryEntry struct {
		block uint64
		id    string
	}
	var toExpire []expiryEntry

	for ; iter.Valid(); iter.Next() {
		rawKey := iter.Key()
		// Key layout after prefix: 8 bytes expiryBlock + id string
		if len(rawKey) < 8 {
			continue
		}
		expiryBlock := binary.BigEndian.Uint64(rawKey[:8])
		if expiryBlock > currentBlock {
			break // sorted — no more expired entries
		}
		id := string(rawKey[8:])
		toExpire = append(toExpire, expiryEntry{block: expiryBlock, id: id})
	}
	iter.Close()

	for _, entry := range toExpire {
		pr, found := k.GetPreRegistration(ctx, entry.id)
		if !found || pr.Fulfilled || pr.Expired {
			// Already handled — clean up dangling index entry.
			k.DeletePreRegistrationExpiryIndex(ctx, entry.block, entry.id)
			continue
		}

		// Forfeit bond to research_fund if bond was posted.
		if pr.BondAmount != "" && pr.BondAmount != "0" && k.bankKeeper != nil {
			bondInt, ok := sdkmath.NewIntFromString(pr.BondAmount)
			if ok && bondInt.IsPositive() {
				bondCoins := sdk.NewCoins(sdk.NewCoin("uzrn", bondInt))
				// knowledge module account → research_fund
				if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, "research_fund", bondCoins); err != nil {
					k.Logger(ctx).Error("pre-registration bond forfeiture failed",
						"pre_registration_id", pr.Id,
						"bond", pr.BondAmount,
						"error", err,
					)
					// Non-fatal: mark expired anyway.
				}
			}
		}

		// Mark expired.
		pr.Expired = true
		_ = k.SetPreRegistration(ctx, pr)

		// Remove expiry index entry.
		k.DeletePreRegistrationExpiryIndex(ctx, entry.block, entry.id)

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.pre_registration_expired",
			sdk.NewAttribute("pre_registration_id", pr.Id),
			sdk.NewAttribute("submitter", pr.Submitter),
			sdk.NewAttribute("bond_forfeited", pr.BondAmount),
		))

		k.Logger(ctx).Info("pre-registration expired",
			"pre_registration_id", pr.Id,
			"submitter", pr.Submitter,
			"bond_forfeited", pr.BondAmount,
		)
	}
}

// ── Message handler ──────────────────────────────────────────────────────────

// PreRegisterClaim handles MsgPreRegisterClaim — creates an on-chain anchor
// declaring the claim intent before evidence is collected.
func (m *msgServer) PreRegisterClaim(ctx context.Context, msg *types.MsgPreRegisterClaim) (*types.MsgPreRegisterClaimResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if msg.Submitter == "" {
		return nil, fmt.Errorf("submitter required")
	}
	if msg.ClaimIntent == "" {
		return nil, fmt.Errorf("claim_intent required: the hypothesis must be declared explicitly")
	}
	if msg.Domain == "" {
		return nil, fmt.Errorf("domain required")
	}

	params, err := m.keeper.GetParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load params: %w", err)
	}

	// Validate domain.
	if _, found := m.keeper.GetDomain(ctx, msg.Domain); !found {
		return nil, fmt.Errorf("unknown domain %q: pre-registration domain must match a registered genesis domain", msg.Domain)
	}

	// Validate methodology_id (if provided — empty is allowed).
	if msg.MethodologyId != "" {
		if _, found := m.keeper.GetMethodology(ctx, msg.MethodologyId); !found {
			return nil, fmt.Errorf("methodology %q not found in governance registry", msg.MethodologyId)
		}
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	// Compute expiry block.
	var expiryBlock uint64
	if params.PreRegistrationExpiryBlocks > 0 {
		expiryBlock = height + params.PreRegistrationExpiryBlocks
	}

	// Collect bond (if configured).
	bondAmount := "0"
	if params.PreRegistrationBondUzrn != "" && params.PreRegistrationBondUzrn != "0" && m.keeper.bankKeeper != nil {
		bondInt, ok := sdkmath.NewIntFromString(params.PreRegistrationBondUzrn)
		if ok && bondInt.IsPositive() {
			submitterAddr, err := sdk.AccAddressFromBech32(msg.Submitter)
			if err != nil {
				return nil, fmt.Errorf("invalid submitter address: %w", err)
			}
			bondCoins := sdk.NewCoins(sdk.NewCoin("uzrn", bondInt))
			if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, submitterAddr, types.ModuleName, bondCoins); err != nil {
				return nil, fmt.Errorf("failed to collect pre-registration bond: %w", err)
			}
			bondAmount = params.PreRegistrationBondUzrn
		}
	}

	// Generate ID.
	id := generatePreRegistrationID(msg.Submitter, height)

	pr := &types.PreRegistration{
		Id:             id,
		Submitter:      msg.Submitter,
		ClaimIntent:    msg.ClaimIntent,
		MethodologyId:  msg.MethodologyId,
		Domain:         msg.Domain,
		ExpiryBlock:    expiryBlock,
		BondAmount:     bondAmount,
		CreatedAtBlock: height,
		Fulfilled:      false,
		Expired:        false,
	}

	if err := m.keeper.SetPreRegistration(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to store pre-registration: %w", err)
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.claim_pre_registered",
		sdk.NewAttribute("pre_registration_id", id),
		sdk.NewAttribute("submitter", msg.Submitter),
		sdk.NewAttribute("domain", msg.Domain),
		sdk.NewAttribute("methodology_id", msg.MethodologyId),
		sdk.NewAttribute("expiry_block", fmt.Sprintf("%d", expiryBlock)),
		sdk.NewAttribute("bond_amount", bondAmount),
	))

	return &types.MsgPreRegisterClaimResponse{
		PreRegistrationId: id,
		ExpiryBlock:       expiryBlock,
		BondAmount:        bondAmount,
	}, nil
}

// ── Fulfillment (called from msg_server SubmitClaim) ─────────────────────────

// FulfillPreRegistration marks a pre-registration as fulfilled and returns
// the bond to the submitter. Called when MsgSubmitClaim provides a valid
// pre_registration_id.
//
// Returns (createdAtBlock, error). The createdAtBlock is stored on the Claim
// as an audit anchor proving temporal priority.
func (k Keeper) FulfillPreRegistration(ctx context.Context, preRegID, submitter string, params *types.Params) (createdAtBlock uint64, err error) {
	pr, found := k.GetPreRegistration(ctx, preRegID)
	if !found {
		return 0, fmt.Errorf("pre-registration %q not found", preRegID)
	}
	if pr.Submitter != submitter {
		return 0, fmt.Errorf("pre-registration %q belongs to %s, not %s", preRegID, pr.Submitter, submitter)
	}
	if pr.Fulfilled {
		return 0, fmt.Errorf("pre-registration %q already fulfilled by claim %s", preRegID, pr.FulfilledClaimId)
	}
	if pr.Expired {
		return 0, fmt.Errorf("pre-registration %q has expired", preRegID)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentBlock := uint64(sdkCtx.BlockHeight())
	if pr.ExpiryBlock > 0 && currentBlock > pr.ExpiryBlock {
		return 0, fmt.Errorf("pre-registration %q expired at block %d (current: %d)", preRegID, pr.ExpiryBlock, currentBlock)
	}

	// Return bond to submitter.
	if pr.BondAmount != "" && pr.BondAmount != "0" && k.bankKeeper != nil {
		bondInt, ok := sdkmath.NewIntFromString(pr.BondAmount)
		if ok && bondInt.IsPositive() {
			submitterAddr, addrErr := sdk.AccAddressFromBech32(submitter)
			if addrErr != nil {
				return 0, fmt.Errorf("invalid submitter address: %w", addrErr)
			}
			bondCoins := sdk.NewCoins(sdk.NewCoin("uzrn", bondInt))
			if sendErr := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, submitterAddr, bondCoins); sendErr != nil {
				return 0, fmt.Errorf("failed to return pre-registration bond: %w", sendErr)
			}
		}
	}

	// Remove expiry index entry.
	k.DeletePreRegistrationExpiryIndex(ctx, pr.ExpiryBlock, pr.Id)

	// Mark fulfilled (claim_id set by caller after this returns).
	pr.Fulfilled = true
	_ = k.SetPreRegistration(ctx, pr)

	return pr.CreatedAtBlock, nil
}

// UpdatePreRegistrationClaimID sets the fulfilled_claim_id on a pre-registration.
// Called after the claim is stored, once the claim ID is known.
func (k Keeper) UpdatePreRegistrationClaimID(ctx context.Context, preRegID, claimID string) {
	pr, found := k.GetPreRegistration(ctx, preRegID)
	if !found {
		return
	}
	pr.FulfilledClaimId = claimID
	_ = k.SetPreRegistration(ctx, pr)
}

// ApplyPreRegistrationDiscount returns the discounted effective minimum review
// fee for a pre-registered claim. If discount is 0 or disabled, returns the
// original min fee unchanged.
func ApplyPreRegistrationDiscount(effectiveMinFee string, discountBps uint64) string {
	if discountBps == 0 || discountBps >= 1_000_000 {
		return effectiveMinFee
	}
	minFeeInt, ok := sdkmath.NewIntFromString(effectiveMinFee)
	if !ok || !minFeeInt.IsPositive() {
		return effectiveMinFee
	}
	// discountedMin = minFee × (1_000_000 - discountBps) / 1_000_000
	discounted := minFeeInt.Mul(sdkmath.NewInt(int64(1_000_000 - discountBps))).Quo(sdkmath.NewInt(1_000_000))
	return discounted.String()
}
