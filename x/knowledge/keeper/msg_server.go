package keeper

import (
	"context"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

type msgServer struct {
	keeper Keeper
	types.UnimplementedMsgServer
}

// NewMsgServerImpl returns a types.MsgServer backed by the given Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{keeper: keeper}
}

// ─── Core PoT handlers ──────────────────────────────────────────────────────

func (m *msgServer) SubmitClaim(ctx context.Context, msg *types.MsgSubmitClaim) (*types.MsgSubmitClaimResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	params, err := m.keeper.GetParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get params: %w", err)
	}

	// Validate text length
	textLen := uint64(len(msg.FactContent))
	if textLen < params.MinClaimTextLength {
		return nil, fmt.Errorf("claim text too short: %d < %d", textLen, params.MinClaimTextLength)
	}
	if textLen > params.MaxClaimTextLength {
		return nil, fmt.Errorf("claim text too long: %d > %d", textLen, params.MaxClaimTextLength)
	}

	// Validate domain exists
	if msg.Domain != "" {
		if _, found := m.keeper.GetDomain(ctx, msg.Domain); !found {
			return nil, fmt.Errorf("domain %s does not exist", msg.Domain)
		}
	}

	// Validate partnership_id if provided (R26-4)
	if msg.PartnershipId != "" {
		if m.keeper.partnershipKeeper == nil {
			return nil, types.ErrInvalidPartnership.Wrap("partnership module not available")
		}
		// Check partnership exists and is active
		active, err := m.keeper.partnershipKeeper.IsActive(ctx, msg.PartnershipId)
		if err != nil || !active {
			return nil, types.ErrInvalidPartnership.Wrapf(
				"partnership %s is not active", msg.PartnershipId)
		}
		// Check submitter is a participant
		isParticipant, err := m.keeper.partnershipKeeper.IsParticipant(ctx, msg.PartnershipId, msg.Submitter)
		if err != nil || !isParticipant {
			return nil, types.ErrInvalidPartnership.Wrapf(
				"%s is not a participant in partnership %s", msg.Submitter, msg.PartnershipId)
		}
		// Reject claims through frozen/suspended partnerships
		suspended, err := m.keeper.partnershipKeeper.IsSuspended(ctx, msg.PartnershipId)
		if err == nil && suspended {
			return nil, types.ErrPartnershipFrozen.Wrapf(
				"partnership %s is frozen due to coercion signal", msg.PartnershipId)
		}
	}

	// Validate review fee
	stakeAmt, ok := new(big.Int).SetString(msg.Stake, 10)
	if !ok || stakeAmt.Sign() <= 0 {
		return nil, fmt.Errorf("invalid review fee amount: %s", msg.Stake)
	}
	effectiveMinFee := m.keeper.GetEffectiveMinReviewFee(ctx)
	minFee, _ := new(big.Int).SetString(effectiveMinFee, 10)
	if minFee != nil && stakeAmt.Cmp(minFee) < 0 {
		return nil, fmt.Errorf("review fee %s below minimum %s (effective)", msg.Stake, effectiveMinFee)
	}

	// Validate typed relations — target facts must exist
	for _, rel := range msg.Relations {
		if rel.Relation == types.RelationType_RELATION_TYPE_UNSPECIFIED {
			return nil, fmt.Errorf("relation type must be specified")
		}
		if _, found := m.keeper.GetFact(ctx, rel.TargetFactId); !found {
			return nil, fmt.Errorf("relation target fact %s not found", rel.TargetFactId)
		}
	}

	// Validate structure if provided
	if msg.Structure != nil {
		if msg.Structure.Subject == "" {
			return nil, fmt.Errorf("claim structure: subject is required when structure is provided")
		}
		if msg.Structure.Predicate == "" {
			return nil, fmt.Errorf("claim structure: predicate is required when structure is provided")
		}
		if len(msg.Structure.Tags) > 10 {
			return nil, fmt.Errorf("claim structure: max 10 tags allowed")
		}
		for _, tag := range msg.Structure.Tags {
			if len(tag) > 50 {
				return nil, fmt.Errorf("claim structure: tag too long (max 50 chars): %s", tag)
			}
		}
	}

	// Auto-derive or normalize canonical form
	canonicalForm := msg.CanonicalForm
	if canonicalForm == "" && msg.Structure != nil {
		canonicalForm = types.BuildCanonicalForm(msg.ClaimType, msg.Structure, msg.Domain)
	}
	var canonicalHash string
	if canonicalForm != "" {
		canonicalForm = types.NormalizeCanonicalForm(canonicalForm)
		canonicalHash = types.HashCanonicalForm(canonicalForm)

		// Dedup against canonical hash (stronger than content_hash)
		if existingID, exists := m.keeper.GetClaimByCanonicalHash(ctx, canonicalHash); exists {
			return nil, fmt.Errorf("canonical duplicate: matches existing claim %s", existingID)
		}
	}

	// Check content hash dedup
	contentHash := ComputeClaimContentHash(msg.FactContent, msg.Domain)
	if existingID, exists := m.keeper.GetClaimByContentHash(ctx, contentHash); exists {
		return nil, fmt.Errorf("duplicate claim: content hash matches existing claim %s", existingID)
	}

	// Subject-based dedup warning (structured claims only)
	if msg.Structure != nil && msg.Structure.Subject != "" {
		if existingFactID := m.keeper.FindFactBySubjectPredicate(ctx, msg.Domain, msg.Structure.Subject, msg.Structure.Predicate); existingFactID != "" {
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				"zerone.knowledge.duplicate_subject_warning",
				sdk.NewAttribute("existing_fact_id", existingFactID),
				sdk.NewAttribute("subject", msg.Structure.Subject),
			))
		}
	}

	// Adaptive cooldown check (R29-6)
	effectiveCooldown := m.keeper.GetEffectiveCooldown(ctx, msg.Domain)
	if effectiveCooldown > 0 {
		lastClaimHeight := m.keeper.GetLastClaimHeight(ctx, msg.Submitter)
		if lastClaimHeight > 0 && height-lastClaimHeight < effectiveCooldown {
			return nil, fmt.Errorf("claim cooldown active: %d blocks remaining (effective cooldown: %d)",
				effectiveCooldown-(height-lastClaimHeight), effectiveCooldown)
		}
	}

	// Collect non-refundable review fee and distribute immediately via revenue split.
	sponsored := msg.Sponsored
	feeAmount := stakeAmt.Uint64()

	if m.keeper.bankKeeper != nil {
		feeCoins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))

		if sponsored {
			// ─── Bootstrap fund sponsored path (R19-7) ──────────────────────
			if err := m.keeper.validateAndPayFromBootstrapFund(ctx, msg.Submitter, stakeAmt, feeCoins, params); err != nil {
				return nil, err
			}
		} else {
			// ─── Normal path: submitter pays fee directly ───────────────────
			submitterAddr, err := sdk.AccAddressFromBech32(msg.Submitter)
			if err != nil {
				return nil, fmt.Errorf("invalid submitter address: %w", err)
			}
			if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, submitterAddr, types.ModuleName, feeCoins); err != nil {
				return nil, fmt.Errorf("failed to collect review fee: %w", err)
			}
		}

		// Distribute fee via revenue split (same path regardless of who paid)
		if err := m.keeper.distributeReviewFee(ctx, feeAmount, params); err != nil {
			m.keeper.Logger(ctx).Error("failed to distribute review fee", "error", err)
		}
	}

	// Generate claim ID
	claimID := GenerateClaimID(msg.Submitter, contentHash, height)

	// Default unspecified to assertion (backward compat)
	claimType := msg.ClaimType
	if claimType == types.ClaimType_CLAIM_TYPE_UNSPECIFIED {
		claimType = types.ClaimType_CLAIM_TYPE_ASSERTION
	}

	claim := &types.Claim{
		Id:               claimID,
		FactContent:      msg.FactContent,
		Domain:           msg.Domain,
		Category:         msg.Category,
		Submitter:        msg.Submitter,
		SubmittedAtBlock: height,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		References:       msg.References,
		Stake:            msg.Stake,
		PartnershipId:    msg.PartnershipId,
		ContentHash:      contentHash,
		ClaimType:        claimType,
		Relations:        msg.Relations,
		Structure:        msg.Structure,
		CanonicalForm:    canonicalForm,
		CanonicalHash:    canonicalHash,
	}

	if err := m.keeper.SetClaim(ctx, claim); err != nil {
		return nil, err
	}

	// Record last claim height for adaptive cooldown (R29-6)
	m.keeper.SetLastClaimHeight(ctx, msg.Submitter, height)

	// Contradiction detection: auto-mark target facts as CONTESTED
	for _, rel := range msg.Relations {
		if rel.Relation == types.RelationType_RELATION_TYPE_CONTRADICTS {
			targetFact, found := m.keeper.GetFact(ctx, rel.TargetFactId)
			if found && (targetFact.Status == types.FactStatus_FACT_STATUS_VERIFIED ||
				targetFact.Status == types.FactStatus_FACT_STATUS_ACTIVE) {
				targetFact.Status = types.FactStatus_FACT_STATUS_CONTESTED
				_ = m.keeper.SetFact(ctx, targetFact)
			}
		}
	}

	// Validate priority tip BPS (Gap 2): must not exceed governance cap.
	// MaxPriorityTipBps == 0 means the feature is disabled entirely.
	if msg.PriorityTipBps > 0 {
		if params.MaxPriorityTipBps == 0 {
			return nil, types.ErrPriorityTipExceedsMax.Wrap("priority tip is disabled (max_priority_tip_bps == 0)")
		}
		if msg.PriorityTipBps > params.MaxPriorityTipBps {
			return nil, types.ErrPriorityTipExceedsMax.Wrapf(
				"priority_tip_bps %d exceeds governance maximum %d", msg.PriorityTipBps, params.MaxPriorityTipBps)
		}
	}

	// Create verification round (carries priority tip config for reward distribution).
	if _, err := m.keeper.CreateVerificationRound(ctx, claim, msg.PriorityTipBps); err != nil {
		return nil, fmt.Errorf("failed to create verification round: %w", err)
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.submit_claim",
			sdk.NewAttribute("claim_id", claimID),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("domain", msg.Domain),
			sdk.NewAttribute("review_fee", msg.Stake),
			sdk.NewAttribute("content_hash", contentHash),
			sdk.NewAttribute("claim_type", claimType.String()),
			sdk.NewAttribute("sponsored", fmt.Sprintf("%t", sponsored)),
		),
	)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.review_fee_distributed",
			sdk.NewAttribute("claim_id", claimID),
			sdk.NewAttribute("fee_amount", msg.Stake),
			sdk.NewAttribute("verifier_pool", fmt.Sprintf("%d", verifierPoolFromFee(feeAmount, params))),
			sdk.NewAttribute("protocol", fmt.Sprintf("%d", safeMulDiv(feeAmount, params.ProtocolTreasuryBps, 1_000_000))),
			sdk.NewAttribute("development", fmt.Sprintf("%d", safeMulDiv(feeAmount, params.DevelopmentFundBps, 1_000_000))),
			sdk.NewAttribute("research", fmt.Sprintf("%d", feeAmount-verifierPoolFromFee(feeAmount, params)-safeMulDiv(feeAmount, params.ProtocolTreasuryBps, 1_000_000)-safeMulDiv(feeAmount, params.DevelopmentFundBps, 1_000_000))),
		),
	)

	if sponsored {
		addressCount := m.keeper.GetBootstrapClaimCount(ctx, msg.Submitter)
		fundBalance := m.keeper.GetBootstrapFundBalance(ctx)
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"zerone.knowledge.bootstrap_sponsored",
			sdk.NewAttribute("claim_id", claimID),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("fee_amount", msg.Stake),
			sdk.NewAttribute("fund_balance_after", fundBalance.Amount.String()),
			sdk.NewAttribute("address_claims_used", fmt.Sprintf("%d", addressCount)),
		))
	}

	return &types.MsgSubmitClaimResponse{ClaimId: claimID}, nil
}

func (m *msgServer) SubmitCommitment(ctx context.Context, msg *types.MsgSubmitCommitment) (*types.MsgSubmitCommitmentResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	round, found := m.keeper.GetVerificationRound(ctx, msg.RoundId)
	if !found {
		return nil, fmt.Errorf("verification round %s not found", msg.RoundId)
	}

	// Validate phase
	if round.Phase != types.VerificationPhase_VERIFICATION_PHASE_COMMIT {
		return nil, fmt.Errorf("round %s is not in COMMIT phase (current: %s)", msg.RoundId, round.Phase)
	}

	// Check deadline
	if height > round.CommitDeadline {
		return nil, fmt.Errorf("commit phase has ended at block %d", round.CommitDeadline)
	}

	// Verifier minimum balance gate (stopgap until full qualification module)
	if m.keeper.bankKeeper != nil {
		verifierAddr, err := sdk.AccAddressFromBech32(msg.Verifier)
		if err != nil {
			return nil, fmt.Errorf("invalid verifier address: %w", err)
		}
		bal := m.keeper.bankKeeper.GetBalance(ctx, verifierAddr, "uzrn")
		minBalance := sdkmath.NewInt(100_000_000) // 100 ZRN
		if bal.Amount.LT(minBalance) {
			return nil, fmt.Errorf("verifier does not meet minimum balance requirement")
		}
	}

	// Check domain qualification
	if m.keeper.domainQualificationKeeper != nil {
		claim, found := m.keeper.GetClaim(ctx, round.ClaimId)
		if !found {
			return nil, fmt.Errorf("claim %s not found for round %s", round.ClaimId, msg.RoundId)
		}
		if claim.Domain != "" {
			qualified, err := m.keeper.domainQualificationKeeper.IsQualified(ctx, msg.Verifier, claim.Domain)
			if err != nil {
				return nil, fmt.Errorf("qualification check failed: %w", err)
			}
			if !qualified {
				// Check if fallback applies: if no qualified validators exist for this domain,
				// allow unqualified ones through
				qualifiedVals, _ := m.keeper.domainQualificationKeeper.GetQualifiedValidators(ctx, claim.Domain)
				params, _ := m.keeper.GetParams(ctx)
				if uint64(len(qualifiedVals)) >= params.MinVerifiers {
					return nil, types.ErrUnqualifiedVerifier.Wrapf(
						"validator %s is not qualified for domain %s", msg.Verifier, claim.Domain)
				}
				// Insufficient qualified validators — allow through with warning
				m.keeper.Logger(ctx).Warn("allowing unqualified verifier due to insufficient qualified validators",
					"verifier", msg.Verifier, "domain", claim.Domain,
					"qualified_count", len(qualifiedVals), "min_verifiers", params.MinVerifiers)
			}
		}
	}

	// Verifier independence gate (P-10a): block validators who consistently
	// agree with the majority if governance has enabled a minimum independence
	// score. Fallback applies if not enough independent validators exist to
	// form a panel — same design as the domain qualification fallback above.
	if params2, err2 := m.keeper.GetParams(ctx); err2 == nil && params2.MinVerifierIndependenceBps > 0 {
		rec, found, _ := m.keeper.GetValidatorIndependence(ctx, msg.Verifier)
		if found && rec.TotalVotes >= params2.MinVerifierIndependenceRounds {
			score := uint64(0)
			if rec.TotalVotes > 0 {
				score = rec.MinorityVotes * 1_000_000 / rec.TotalVotes
			}
			if score < params2.MinVerifierIndependenceBps {
				// Check fallback: count validators with sufficient independence.
				independentCount := m.keeper.CountValidatorsWithSufficientIndependence(
					ctx, params2.MinVerifierIndependenceBps, params2.MinVerifierIndependenceRounds)
				if uint64(independentCount) >= params2.MinVerifiers {
					// Enough independent validators exist — enforce the gate.
					return nil, types.ErrLowVerifierIndependence.Wrapf(
						"validator %s has independence score %d bps (minimum: %d bps, total_votes: %d)",
						msg.Verifier, score, params2.MinVerifierIndependenceBps, rec.TotalVotes)
				}
				// Insufficient independent validators — allow through with warning.
				m.keeper.Logger(ctx).Warn("allowing low-independence verifier due to insufficient independent validators",
					"verifier", msg.Verifier,
					"score_bps", score,
					"required_bps", params2.MinVerifierIndependenceBps,
					"independent_count", independentCount,
					"min_verifiers", params2.MinVerifiers)
			}
		}
	}

	// Delegate storage to the unified helper so the tx path and the vote-extension
	// path share equivocation detection and SelectedVerifiers tracking.
	if err := m.keeper.StoreCommitmentInRound(ctx, msg.RoundId, &types.CommitEntry{
		Verifier:         msg.Verifier,
		CommitHash:       msg.CommitHash,
		CommittedAtBlock: height,
	}); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.submit_commitment",
			sdk.NewAttribute("round_id", msg.RoundId),
			sdk.NewAttribute("verifier", msg.Verifier),
			sdk.NewAttribute("committed_at_block", fmt.Sprintf("%d", height)),
		),
	)

	return &types.MsgSubmitCommitmentResponse{}, nil
}

func (m *msgServer) SubmitReveal(ctx context.Context, msg *types.MsgSubmitReveal) (*types.MsgSubmitRevealResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	round, found := m.keeper.GetVerificationRound(ctx, msg.RoundId)
	if !found {
		return nil, fmt.Errorf("verification round %s not found", msg.RoundId)
	}

	// Validate phase
	if round.Phase != types.VerificationPhase_VERIFICATION_PHASE_REVEAL {
		return nil, fmt.Errorf("round %s is not in REVEAL phase (current: %s)", msg.RoundId, round.Phase)
	}

	// Check deadline
	if height > round.RevealDeadline {
		return nil, fmt.Errorf("reveal phase has ended at block %d", round.RevealDeadline)
	}

	// Validate vote value before delegating (tx-level friendliness).
	if msg.Vote != "accept" && msg.Vote != "reject" && msg.Vote != "malformed" {
		return nil, fmt.Errorf("invalid vote: must be 'accept', 'reject', or 'malformed'")
	}

	// Delegate to the unified helper which uses ComputeCommitmentHash
	// (round_id + vote + confidence + salt) — matches the vote-extension path.
	if err := m.keeper.StoreRevealInRound(ctx, msg.RoundId, &types.RevealEntry{
		Verifier:        msg.Verifier,
		Vote:            msg.Vote,
		Salt:            msg.Salt,
		RevealedAtBlock: height,
	}, msg.Confidence); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.submit_reveal",
			sdk.NewAttribute("round_id", msg.RoundId),
			sdk.NewAttribute("verifier", msg.Verifier),
			sdk.NewAttribute("vote", msg.Vote),
			sdk.NewAttribute("revealed_at_block", fmt.Sprintf("%d", height)),
		),
	)

	return &types.MsgSubmitRevealResponse{}, nil
}

func (m *msgServer) AddFact(ctx context.Context, msg *types.MsgAddFact) (*types.MsgAddFactResponse, error) {
	// Authority-only: governance fact injection
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	factID := GenerateFactID(msg.Content, height)

	params, err := m.keeper.GetParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get params: %w", err)
	}

	// AddFact bypasses the verifier-panel trust chain. The Wave 16
	// guardian-veto window protects this path from single-key abuse:
	// when a guardian set is configured AND a veto window is active,
	// the proposed fact is queued instead of materializing immediately.
	// During the window any guardian can call MsgVetoFactInjection to
	// cancel. After the window the BeginBlocker materializes the fact
	// (existing behavior). Without configuration, the immediate path
	// remains for genesis seeding / corrective injection on a still-
	// trusted authority.
	if m.keeper.AddFactVetoEnabled(ctx) {
		executeAt := height + params.AddFactVetoWindowBlocks
		pending := &types.PendingFactInjection{
			Id:              factID,
			Content:         msg.Content,
			Domain:          msg.Domain,
			Category:        msg.Category,
			Confidence:      msg.Confidence,
			References:      msg.References,
			Proposer:        msg.Authority,
			ProposedAtBlock: height,
			ExecuteAtBlock:  executeAt,
		}
		if err := m.keeper.SetPendingFactInjection(ctx, pending); err != nil {
			return nil, err
		}
		m.keeper.RecordPrivilegedAction(ctx,
			types.PrivilegedActionType_PRIVILEGED_ACTION_TYPE_FACT_AUTHORITY_INJECT,
			msg.Authority,
			factID,
			"",
			fmt.Sprintf("PROPOSED domain=%s category=%s veto_until=%d", msg.Domain, msg.Category, executeAt),
		)
		sdkCtx.EventManager().EmitEvent(
			sdk.NewEvent("zerone.knowledge.add_fact_proposed",
				sdk.NewAttribute("pending_id", factID),
				sdk.NewAttribute("authority", msg.Authority),
				sdk.NewAttribute("domain", msg.Domain),
				sdk.NewAttribute("category", msg.Category),
				sdk.NewAttribute("execute_at_block", fmt.Sprintf("%d", executeAt)),
			),
		)
		return &types.MsgAddFactResponse{FactId: factID}, nil
	}

	// ── Immediate path (no guardian veto configured) ──
	fact := &types.Fact{
		Id:                factID,
		Content:           msg.Content,
		Domain:            msg.Domain,
		Category:          msg.Category,
		Confidence:        m.keeper.ClampConfidence(ctx, msg.Confidence, msg.Domain),
		Submitter:         msg.Authority,
		SubmittedAtBlock:  height,
		VerifiedAtBlock:   height,
		LastVerifiedBlock: height,
		References:        msg.References,
		Status:            types.FactStatus_FACT_STATUS_VERIFIED,
		// Initialize metabolism fields
		Energy:            params.MetabolismInitialEnergy,
		EnergyCap:         params.MetabolismEnergyCap,
		EnergyLastUpdated: height,
	}

	// Apply domain carrying capacity birth pressure (R29-1)
	fact.Energy = m.keeper.ApplyBirthPressure(ctx, msg.Domain, fact.Energy)

	if err := m.keeper.SetFact(ctx, fact); err != nil {
		return nil, err
	}

	// Update domain stats for carrying capacity (R29-1)
	m.keeper.IncrementDomainFactCount(ctx, fact.Domain, true, fact.Energy)

	// Privileged-action log captures every authority-gated injection so
	// compromised-authority abuse is queryable rather than buried in the
	// generic block event stream.
	m.keeper.RecordPrivilegedAction(ctx,
		types.PrivilegedActionType_PRIVILEGED_ACTION_TYPE_FACT_AUTHORITY_INJECT,
		msg.Authority,
		factID,
		"",
		fmt.Sprintf("domain=%s category=%s", msg.Domain, msg.Category),
	)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.add_fact",
			sdk.NewAttribute("fact_id", factID),
			sdk.NewAttribute("authority", msg.Authority),
			sdk.NewAttribute("domain", msg.Domain),
			sdk.NewAttribute("category", msg.Category),
			sdk.NewAttribute("status", types.FactStatus_FACT_STATUS_VERIFIED.String()),
		),
	)

	return &types.MsgAddFactResponse{FactId: factID}, nil
}

// VetoFactInjection — Wave 16 multi-sig defense. A registered guardian
// cancels a pending authority-injected fact during the veto window.
// Removes the entry from the pending queue; the fact never materializes.
// The privileged-action log records the veto for audit.
func (m *msgServer) VetoFactInjection(ctx context.Context, msg *types.MsgVetoFactInjection) (*types.MsgVetoFactInjectionResponse, error) {
	if msg == nil || msg.PendingId == "" {
		return nil, fmt.Errorf("pending_id required")
	}
	if !m.keeper.IsGuardian(ctx, msg.Guardian) {
		return nil, fmt.Errorf("unauthorized: %s is not a registered guardian", msg.Guardian)
	}
	pending, ok := m.keeper.GetPendingFactInjection(ctx, msg.PendingId)
	if !ok {
		return nil, fmt.Errorf("no pending fact injection with id %s", msg.PendingId)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	if height >= pending.ExecuteAtBlock {
		return nil, fmt.Errorf("veto window closed: pending %s materializes at block %d (now %d) (commitment 6: the veto window is the chain's promise that authority injection is reviewable; once closed the promise has run its course)",
			msg.PendingId, pending.ExecuteAtBlock, height)
	}
	if err := m.keeper.DeletePendingFactInjection(ctx, msg.PendingId); err != nil {
		return nil, err
	}
	m.keeper.RecordPrivilegedAction(ctx,
		types.PrivilegedActionType_PRIVILEGED_ACTION_TYPE_FACT_AUTHORITY_INJECT,
		msg.Guardian,
		pending.Id,
		"",
		fmt.Sprintf("VETOED proposer=%s reason=%s", pending.Proposer, msg.Reason),
	)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.fact_injection_vetoed",
		sdk.NewAttribute("pending_id", pending.Id),
		sdk.NewAttribute("guardian", msg.Guardian),
		sdk.NewAttribute("proposer", pending.Proposer),
		sdk.NewAttribute("reason", msg.Reason),
	))
	return &types.MsgVetoFactInjectionResponse{}, nil
}

// ─── Param update handlers ──────────────────────────────────────────────────

func (m *msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}
	if msg.Params == nil {
		return nil, fmt.Errorf("params must not be nil")
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := m.keeper.SetParams(ctx, msg.Params); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.update_params",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}

func (m *msgServer) UpdateExtendedParams(ctx context.Context, msg *types.MsgUpdateExtendedParams) (*types.MsgUpdateExtendedParamsResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}
	// Store extended params as raw JSON blob
	store := m.keeper.storeService.OpenKVStore(ctx)
	if err := store.Set(types.ExtendedParamsKey, []byte(msg.ParamsJson)); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.update_extended_params",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateExtendedParamsResponse{}, nil
}

// ─── Challenge/contradiction handlers ────────────────────────────────────────

func (m *msgServer) ChallengeFact(ctx context.Context, msg *types.MsgChallengeFact) (*types.MsgChallengeFactResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	fact, found := m.keeper.GetFact(ctx, msg.FactId)
	if !found {
		return nil, fmt.Errorf("fact %s not found", msg.FactId)
	}

	// Fact must be in a challengeable state
	if fact.Status != types.FactStatus_FACT_STATUS_VERIFIED &&
		fact.Status != types.FactStatus_FACT_STATUS_ACTIVE {
		return nil, fmt.Errorf("fact %s is not in a challengeable state (status: %s) (commitment 4: only verified or active facts accept probes; degraded facts are already moving)", msg.FactId, fact.Status)
	}

	// Risk-scaled stake check (T12): higher-confidence facts require more stake
	// to challenge, proportional to params.ChallengeConfidenceScalingBps.
	params, err := m.keeper.GetParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load params: %w", err)
	}
	stakeAmt, ok := new(big.Int).SetString(msg.Stake, 10)
	if !ok || stakeAmt.Sign() <= 0 {
		return nil, fmt.Errorf("invalid stake amount: %s", msg.Stake)
	}
	effectiveMinStake := EffectiveMinChallengeStake(params, fact.Confidence)
	if stakeAmt.Cmp(effectiveMinStake) < 0 {
		return nil, fmt.Errorf("challenge stake %s below effective minimum %s (fact confidence %d) (commitment 4: probe cost scales with confidence — high-confidence facts must be the cheapest to test relative to what is at stake)",
			msg.Stake, effectiveMinStake.String(), fact.Confidence)
	}

	// Lock challenge stake
	if m.keeper.bankKeeper != nil {
		challengerAddr, err := sdk.AccAddressFromBech32(msg.Challenger)
		if err != nil {
			return nil, fmt.Errorf("invalid challenger address: %w", err)
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
		if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("failed to lock challenge stake: %w", err)
		}
	}

	// Mark fact as challenged
	fact.Status = types.FactStatus_FACT_STATUS_CHALLENGED
	if err := m.keeper.SetFact(ctx, fact); err != nil {
		return nil, err
	}

	// Create a challenge claim and round
	challengeClaimID := GenerateClaimID(msg.Challenger, msg.FactId, height)
	challengeClaim := &types.Claim{
		Id:                challengeClaimID,
		FactContent:       fmt.Sprintf("Challenge of fact %s: %s", msg.FactId, msg.Reason),
		Domain:            fact.Domain,
		Category:          fact.Category,
		Submitter:         msg.Challenger,
		SubmittedAtBlock:  height,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             msg.Stake,
		ProvisionalFactId: msg.FactId, // Track challenged fact for resolution
	}
	if err := m.keeper.SetClaim(ctx, challengeClaim); err != nil {
		return nil, err
	}

	round, err := m.keeper.CreateVerificationRound(ctx, challengeClaim, 0)
	if err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.challenge_fact",
			sdk.NewAttribute("fact_id", msg.FactId),
			sdk.NewAttribute("challenger", msg.Challenger),
			sdk.NewAttribute("round_id", round.Id),
			sdk.NewAttribute("stake", msg.Stake),
			sdk.NewAttribute("reason", msg.Reason),
		),
	)

	return &types.MsgChallengeFactResponse{RoundId: round.Id}, nil
}

func (m *msgServer) ChallengeProvisionalFact(ctx context.Context, msg *types.MsgChallengeProvisionalFact) (*types.MsgChallengeProvisionalFactResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	fact, found := m.keeper.GetFact(ctx, msg.FactId)
	if !found {
		return nil, fmt.Errorf("fact %s not found", msg.FactId)
	}

	if fact.Status != types.FactStatus_FACT_STATUS_PROVISIONAL {
		return nil, fmt.Errorf("fact %s is not provisional (status: %s)", msg.FactId, fact.Status)
	}

	// Lock stake
	if m.keeper.bankKeeper != nil {
		stakeAmt, ok := new(big.Int).SetString(msg.Stake, 10)
		if !ok || stakeAmt.Sign() <= 0 {
			return nil, fmt.Errorf("invalid stake amount: %s", msg.Stake)
		}
		challengerAddr, err := sdk.AccAddressFromBech32(msg.Challenger)
		if err != nil {
			return nil, err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
		if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("failed to lock challenge stake: %w", err)
		}
	}

	fact.Status = types.FactStatus_FACT_STATUS_CHALLENGED
	_ = m.keeper.SetFact(ctx, fact)

	challengeClaimID := GenerateClaimID(msg.Challenger, msg.FactId, height)
	challengeClaim := &types.Claim{
		Id:                challengeClaimID,
		FactContent:       fmt.Sprintf("Provisional challenge of fact %s: %s", msg.FactId, msg.Reason),
		Domain:            fact.Domain,
		Category:          fact.Category,
		Submitter:         msg.Challenger,
		SubmittedAtBlock:  height,
		Status:            types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:             msg.Stake,
		ProvisionalFactId: msg.FactId, // Track challenged fact for resolution
	}
	_ = m.keeper.SetClaim(ctx, challengeClaim)
	round, err := m.keeper.CreateVerificationRound(ctx, challengeClaim, 0)
	if err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.challenge_provisional_fact",
			sdk.NewAttribute("fact_id", msg.FactId),
			sdk.NewAttribute("challenger", msg.Challenger),
			sdk.NewAttribute("challenge_id", round.Id),
			sdk.NewAttribute("stake", msg.Stake),
			sdk.NewAttribute("reason", msg.Reason),
		),
	)

	return &types.MsgChallengeProvisionalFactResponse{ChallengeId: round.Id}, nil
}

func (m *msgServer) SubmitContradiction(ctx context.Context, msg *types.MsgSubmitContradiction) (*types.MsgSubmitContradictionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	// Validate target fact exists
	targetFact, found := m.keeper.GetFact(ctx, msg.FactId)
	if !found {
		return nil, fmt.Errorf("target fact %s not found", msg.FactId)
	}

	// Lock stake
	if m.keeper.bankKeeper != nil {
		stakeAmt, ok := new(big.Int).SetString(msg.Stake, 10)
		if !ok || stakeAmt.Sign() <= 0 {
			return nil, fmt.Errorf("invalid stake amount: %s", msg.Stake)
		}
		submitterAddr, err := sdk.AccAddressFromBech32(msg.Submitter)
		if err != nil {
			return nil, err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
		if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, submitterAddr, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("failed to lock contradiction stake: %w", err)
		}
	}

	// Create counter-claim
	domain := msg.Domain
	if domain == "" {
		domain = targetFact.Domain
	}
	contentHash := ComputeClaimContentHash(msg.CounterClaim, domain)
	counterClaimID := GenerateClaimID(msg.Submitter, contentHash, height)

	claim := &types.Claim{
		Id:               counterClaimID,
		FactContent:      msg.CounterClaim,
		Domain:           domain,
		Category:         msg.Category,
		Submitter:        msg.Submitter,
		SubmittedAtBlock: height,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            msg.Stake,
		ContentHash:      contentHash,
	}
	if err := m.keeper.SetClaim(ctx, claim); err != nil {
		return nil, err
	}

	if _, err := m.keeper.CreateVerificationRound(ctx, claim, 0); err != nil {
		return nil, err
	}

	// Mark target fact as contested
	targetFact.Status = types.FactStatus_FACT_STATUS_CONTESTED
	_ = m.keeper.SetFact(ctx, targetFact)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.submit_contradiction",
			sdk.NewAttribute("fact_id", msg.FactId),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("counter_claim_id", counterClaimID),
			sdk.NewAttribute("domain", domain),
			sdk.NewAttribute("stake", msg.Stake),
		),
	)

	return &types.MsgSubmitContradictionResponse{CounterFactId: counterClaimID}, nil
}

// ─── Domain handlers ─────────────────────────────────────────────────────────

func (m *msgServer) ProposeDomain(ctx context.Context, msg *types.MsgProposeDomain) (*types.MsgProposeDomainResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	// Check domain doesn't already exist
	if _, found := m.keeper.GetDomain(ctx, msg.Name); found {
		return nil, fmt.Errorf("domain %s already exists", msg.Name)
	}

	// Lock proposal stake
	if m.keeper.bankKeeper != nil && msg.Stake != "" {
		stakeAmt, ok := new(big.Int).SetString(msg.Stake, 10)
		if ok && stakeAmt.Sign() > 0 {
			proposerAddr, err := sdk.AccAddressFromBech32(msg.Proposer)
			if err != nil {
				return nil, err
			}
			coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(stakeAmt)))
			if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, proposerAddr, types.ModuleName, coins); err != nil {
				return nil, fmt.Errorf("failed to lock proposal stake: %w", err)
			}
		}
	}

	domain := &types.Domain{
		Name:           msg.Name,
		Description:    msg.Description,
		Status:         types.DomainStatus_DOMAIN_STATUS_PROPOSED,
		CreatedAtBlock: height,
		Proposer:       msg.Proposer,
		Stratum:        msg.Stratum,
	}

	if err := m.keeper.SetDomain(ctx, domain); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.domain_proposed",
		sdk.NewAttribute("name", msg.Name),
		sdk.NewAttribute("proposer", msg.Proposer),
	))

	return &types.MsgProposeDomainResponse{ProposalId: msg.Name}, nil
}

func (m *msgServer) EndorseDomainProposal(ctx context.Context, msg *types.MsgEndorseDomainProposal) (*types.MsgEndorseDomainProposalResponse, error) {
	domain, found := m.keeper.GetDomain(ctx, msg.ProposalId)
	if !found {
		return nil, fmt.Errorf("domain proposal %s not found", msg.ProposalId)
	}

	if domain.Status != types.DomainStatus_DOMAIN_STATUS_PROPOSED {
		return nil, fmt.Errorf("domain %s is not in PROPOSED status", msg.ProposalId)
	}

	// Check for duplicate endorsement
	for _, e := range domain.Endorsers {
		if e == msg.Endorser {
			return nil, fmt.Errorf("already endorsed by %s", msg.Endorser)
		}
	}

	domain.Endorsers = append(domain.Endorsers, msg.Endorser)

	// Auto-activate if threshold met (3 endorsers)
	if len(domain.Endorsers) >= 3 {
		domain.Status = types.DomainStatus_DOMAIN_STATUS_ACTIVE
	}

	if err := m.keeper.SetDomain(ctx, domain); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.endorse_domain_proposal",
			sdk.NewAttribute("proposal_id", msg.ProposalId),
			sdk.NewAttribute("endorser", msg.Endorser),
			sdk.NewAttribute("endorser_count", fmt.Sprintf("%d", len(domain.Endorsers))),
			sdk.NewAttribute("status", domain.Status.String()),
		),
	)

	return &types.MsgEndorseDomainProposalResponse{}, nil
}

func (m *msgServer) ChallengeDomainProposal(ctx context.Context, msg *types.MsgChallengeDomainProposal) (*types.MsgChallengeDomainProposalResponse, error) {
	domain, found := m.keeper.GetDomain(ctx, msg.ProposalId)
	if !found {
		return nil, fmt.Errorf("domain proposal %s not found", msg.ProposalId)
	}

	if domain.Status != types.DomainStatus_DOMAIN_STATUS_PROPOSED {
		return nil, fmt.Errorf("domain %s is not in PROPOSED status", msg.ProposalId)
	}

	// For now, emit event — full challenge resolution deferred
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.domain_proposal_challenged",
		sdk.NewAttribute("domain", msg.ProposalId),
		sdk.NewAttribute("challenger", msg.Challenger),
		sdk.NewAttribute("reason", msg.Reason),
	))

	return &types.MsgChallengeDomainProposalResponse{}, nil
}

func (m *msgServer) RegisterStratum(ctx context.Context, msg *types.MsgRegisterStratum) (*types.MsgRegisterStratumResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}
	// Stratum registration is deferred to ontology integration.
	// For now, emit event only.
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.stratum_registered",
		sdk.NewAttribute("name", msg.Name),
		sdk.NewAttribute("confidence_ceiling", fmt.Sprintf("%d", msg.ConfidenceCeiling)),
	))
	return &types.MsgRegisterStratumResponse{}, nil
}

// ─── Economic handlers ───────────────────────────────────────────────────────

func (m *msgServer) PatronizeFact(ctx context.Context, msg *types.MsgPatronizeFact) (*types.MsgPatronizeFactResponse, error) {
	fact, found := m.keeper.GetFact(ctx, msg.FactId)
	if !found {
		return nil, fmt.Errorf("fact %s not found", msg.FactId)
	}

	// Lock patronage amount
	if m.keeper.bankKeeper != nil {
		amtBig, ok := new(big.Int).SetString(msg.Amount, 10)
		if !ok || amtBig.Sign() <= 0 {
			return nil, fmt.Errorf("invalid patronage amount: %s", msg.Amount)
		}
		patronAddr, err := sdk.AccAddressFromBech32(msg.Patron)
		if err != nil {
			return nil, err
		}
		coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amtBig)))
		if err := m.keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, patronAddr, types.ModuleName, coins); err != nil {
			return nil, fmt.Errorf("failed to lock patronage: %w", err)
		}
	}

	// Update fact patronage
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())
	fact.PatronageAmount = msg.Amount
	fact.PatronageExpiryBlock = height + msg.DurationBlocks

	// Apply immediate energy boost (saves fact internally)
	m.keeper.ApplyPatronageEnergyBoost(ctx, fact, msg.DurationBlocks, msg.Patron)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.patronize_fact",
			sdk.NewAttribute("fact_id", msg.FactId),
			sdk.NewAttribute("patron", msg.Patron),
			sdk.NewAttribute("amount", msg.Amount),
			sdk.NewAttribute("duration_blocks", fmt.Sprintf("%d", msg.DurationBlocks)),
			sdk.NewAttribute("expiry_block", fmt.Sprintf("%d", fact.PatronageExpiryBlock)),
		),
	)

	return &types.MsgPatronizeFactResponse{}, nil
}

func (m *msgServer) ProposeResearchFund(ctx context.Context, msg *types.MsgProposeResearchFund) (*types.MsgProposeResearchFundResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	proposalID := GenerateClaimID(msg.Proposer, msg.Title, height)

	// Store proposal in ExtendedParams key space (using research proposal prefix)
	store := m.keeper.storeService.OpenKVStore(ctx)
	proposalKey := append(append([]byte{}, types.ResearchProposalPrefix...), []byte(proposalID)...)
	proposalData := fmt.Sprintf(`{"id":"%s","proposer":"%s","title":"%s","amount":"%s","recipient":"%s","voting_end":%d,"status":"active"}`,
		proposalID, msg.Proposer, msg.Title, msg.Amount, msg.Recipient, height+msg.VotingPeriodBlocks)
	if err := store.Set(proposalKey, []byte(proposalData)); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.propose_research_fund",
			sdk.NewAttribute("proposal_id", proposalID),
			sdk.NewAttribute("proposer", msg.Proposer),
			sdk.NewAttribute("title", msg.Title),
			sdk.NewAttribute("amount", msg.Amount),
			sdk.NewAttribute("recipient", msg.Recipient),
			sdk.NewAttribute("voting_end_block", fmt.Sprintf("%d", height+msg.VotingPeriodBlocks)),
		),
	)

	return &types.MsgProposeResearchFundResponse{ProposalId: proposalID}, nil
}

func (m *msgServer) VoteResearchProposal(ctx context.Context, msg *types.MsgVoteResearchProposal) (*types.MsgVoteResearchProposalResponse, error) {
	// Store vote
	store := m.keeper.storeService.OpenKVStore(ctx)
	voteKey := append(append([]byte{}, types.ResearchVotePrefix...), []byte(msg.ProposalId+"/"+msg.Voter)...)
	voteVal := "no"
	if msg.Vote {
		voteVal = "yes"
	}
	if err := store.Set(voteKey, []byte(voteVal)); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent("zerone.knowledge.vote_research_proposal",
			sdk.NewAttribute("proposal_id", msg.ProposalId),
			sdk.NewAttribute("voter", msg.Voter),
			sdk.NewAttribute("vote", voteVal),
		),
	)

	return &types.MsgVoteResearchProposalResponse{}, nil
}

func (m *msgServer) ExecuteResearchProposal(ctx context.Context, msg *types.MsgExecuteResearchProposal) (*types.MsgExecuteResearchProposalResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	// Full tally and execution deferred to governance integration.
	// For now, emit event.
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.research_proposal_executed",
		sdk.NewAttribute("proposal_id", msg.ProposalId),
	))

	return &types.MsgExecuteResearchProposalResponse{}, nil
}

// ─── Common knowledge registry governance ─────────────────────────────────────

func (m msgServer) AddCommonKnowledge(ctx context.Context, msg *types.MsgAddCommonKnowledge) (*types.MsgAddCommonKnowledgeResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	if msg.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if msg.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if msg.PenaltyBps > 1_000_000 {
		return nil, fmt.Errorf("penalty_bps must be <= 1,000,000")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	id := commonKnowledgeID(msg.Domain, msg.Subject)

	entry := &types.CommonKnowledgeEntry{
		Id:          id,
		Domain:      msg.Domain,
		Subject:     msg.Subject,
		Description: msg.Description,
		PenaltyBps:  msg.PenaltyBps,
		AddedBlock:  uint64(sdkCtx.BlockHeight()),
	}

	if err := m.keeper.SetCommonKnowledgeEntry(ctx, entry); err != nil {
		return nil, fmt.Errorf("failed to set common knowledge entry: %w", err)
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.common_knowledge_added",
		sdk.NewAttribute("id", id),
		sdk.NewAttribute("domain", msg.Domain),
		sdk.NewAttribute("subject", msg.Subject),
		sdk.NewAttribute("penalty_bps", fmt.Sprintf("%d", msg.PenaltyBps)),
	))

	return &types.MsgAddCommonKnowledgeResponse{Id: id}, nil
}

func (m msgServer) RemoveCommonKnowledge(ctx context.Context, msg *types.MsgRemoveCommonKnowledge) (*types.MsgRemoveCommonKnowledgeResponse, error) {
	if msg.Authority != m.keeper.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.keeper.GetAuthority(), msg.Authority)
	}

	if msg.Id == "" {
		return nil, fmt.Errorf("id is required")
	}

	// Find entry by ID to get domain/subject for store key
	entry, found := m.keeper.FindCommonKnowledgeByID(ctx, msg.Id)
	if !found {
		return nil, fmt.Errorf("common knowledge entry not found: %s", msg.Id)
	}

	if err := m.keeper.DeleteCommonKnowledgeEntry(ctx, entry.Domain, entry.Subject); err != nil {
		return nil, fmt.Errorf("failed to delete common knowledge entry: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.common_knowledge_removed",
		sdk.NewAttribute("id", msg.Id),
		sdk.NewAttribute("domain", entry.Domain),
		sdk.NewAttribute("subject", entry.Subject),
	))

	return &types.MsgRemoveCommonKnowledgeResponse{}, nil
}

// ─── Agent demand handlers ───────────────────────────────────────────────────

func (m *msgServer) ReportDemand(ctx context.Context, msg *types.MsgReportDemand) (*types.MsgReportDemandResponse, error) {
	if !m.keeper.IsAuthorizedDemandReporter(ctx, msg.Reporter) {
		return nil, fmt.Errorf("unauthorized demand reporter: %s", msg.Reporter)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := uint64(sdkCtx.BlockHeight())

	for _, report := range msg.Reports {
		signal, _ := m.keeper.GetOrCreateDemandSignal(ctx, report.Domain, report.Subject)
		signal.QueryCount += report.Queries
		signal.FulfilledCount += report.Fulfilled
		signal.UnfulfilledCount += report.Unfulfilled
		signal.EpochQueryCount += report.Queries
		signal.EpochUnfulfilled += report.Unfulfilled
		signal.LastQueryBlock = height
		if err := m.keeper.SetDemandSignal(ctx, signal); err != nil {
			return nil, fmt.Errorf("failed to store demand signal: %w", err)
		}
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.demand_reported",
		sdk.NewAttribute("reporter", msg.Reporter),
		sdk.NewAttribute("report_count", fmt.Sprintf("%d", len(msg.Reports))),
	))

	return &types.MsgReportDemandResponse{}, nil
}

// ─── Query satisfaction handlers ────────────────────────────────────────────

func (m *msgServer) RateFact(ctx context.Context, msg *types.MsgRateFact) (*types.MsgRateFactResponse, error) {
	// Validate memo length
	if len(msg.Memo) > 256 {
		return nil, fmt.Errorf("memo exceeds 256 characters")
	}

	// Verify fact exists
	fact, found := m.keeper.GetFact(ctx, msg.FactId)
	if !found {
		return nil, fmt.Errorf("fact not found: %s", msg.FactId)
	}

	// Verify query receipt (proof-of-query)
	if !m.keeper.HasQueryReceipt(ctx, msg.Rater, msg.FactId) {
		return nil, fmt.Errorf("no query receipt: you must query a fact before rating it")
	}

	// Prevent double-rating: consume the receipt
	if err := m.keeper.ConsumeQueryReceipt(ctx, msg.Rater, msg.FactId); err != nil {
		return nil, fmt.Errorf("failed to consume receipt: %w", err)
	}

	// Apply rating
	if msg.Useful {
		fact.SatisfactionUp++
		fact.SatisfactionUpEpoch++
	} else {
		fact.SatisfactionDown++
		fact.SatisfactionDownEpoch++
	}

	if err := m.keeper.SetFact(ctx, fact); err != nil {
		return nil, fmt.Errorf("failed to update fact: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.fact_rated",
		sdk.NewAttribute("fact_id", msg.FactId),
		sdk.NewAttribute("rater", msg.Rater),
		sdk.NewAttribute("useful", fmt.Sprintf("%t", msg.Useful)),
	))

	return &types.MsgRateFactResponse{}, nil
}
