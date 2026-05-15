package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

type msgServer struct {
	types.UnimplementedMsgServer
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// InitiateDispute creates a new dispute.
func (m msgServer) InitiateDispute(goCtx context.Context, msg *types.MsgInitiateDispute) (*types.MsgInitiateDisputeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	challengerAddr, err := sdk.AccAddressFromBech32(msg.Challenger)
	if err != nil {
		return nil, fmt.Errorf("invalid challenger address: %w", err)
	}

	// Validate target exists (only for FACT type currently)
	var defender string
	if msg.TargetType == types.DisputeTargetType_DISPUTE_TARGET_TYPE_FACT {
		if m.knowledgeKeeper != nil {
			fact, found := m.knowledgeKeeper.GetFact(ctx, msg.TargetId)
			if !found {
				return nil, fmt.Errorf("%w: fact %s", types.ErrTargetNotFound, msg.TargetId)
			}
			defender = fact.GetSubmitter()
		}
	}
	if defender == "" {
		defender = "unknown"
	}

	// Check max active disputes
	params := m.GetParams(ctx)
	if uint32(m.CountActiveDisputes(ctx)) >= params.MaxActiveDisputes {
		return nil, types.ErrMaxActiveDisputes
	}

	// Validate bond meets tier minimum
	bond := new(big.Int)
	if _, ok := bond.SetString(msg.Bond, 10); !ok || bond.Sign() <= 0 {
		return nil, fmt.Errorf("%w: %s", types.ErrInvalidBond, msg.Bond)
	}

	tier := uint32(1) // start at tier 1
	tierCfg := types.GetTierConfig(params, tier)
	if tierCfg == nil {
		return nil, fmt.Errorf("tier 1 config not found")
	}

	minBond := new(big.Int)
	minBond.SetString(tierCfg.MinBond, 10)
	if bond.Cmp(minBond) < 0 {
		return nil, fmt.Errorf("%w: need %s, got %s", types.ErrInsufficientBond, tierCfg.MinBond, msg.Bond)
	}

	// Escrow bond from challenger
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(bond)))
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to escrow bond: %w", err)
	}

	// Select arbiters
	currentBlock := uint64(ctx.BlockHeight())
	arbiters, err := m.SelectArbiters(ctx, int(tierCfg.ArbiterCount), msg.Challenger, defender, currentBlock)
	if err != nil {
		// Refund bond on arbiter selection failure
		_ = m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challengerAddr, coins)
		return nil, err
	}

	// Generate dispute ID
	disputeID := GenerateDisputeID(msg.TargetId, msg.Challenger, currentBlock)

	dispute := &types.Dispute{
		Id:               disputeID,
		TargetId:         msg.TargetId,
		TargetType:       msg.TargetType,
		Challenger:       msg.Challenger,
		Defender:         defender,
		Reason:           msg.Reason,
		Bond:             msg.Bond,
		Tier:             tier,
		Phase:            types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT,
		Outcome:          types.DisputeOutcome_DISPUTE_OUTCOME_UNSPECIFIED,
		CreatedAt:        currentBlock,
		EvidenceDeadline: currentBlock + tierCfg.EvidencePeriod,
		VotingDeadline:   currentBlock + tierCfg.EvidencePeriod + tierCfg.VotingPeriod,
		Arbiters:         arbiters,
		EvidenceCount:    0,
	}

	m.SetDispute(ctx, dispute)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.dispute_initiated",
			sdk.NewAttribute("dispute_id", disputeID),
			sdk.NewAttribute("challenger", msg.Challenger),
			sdk.NewAttribute("defender", defender),
			sdk.NewAttribute("target_id", msg.TargetId),
			sdk.NewAttribute("bond", msg.Bond),
			sdk.NewAttribute("tier", fmt.Sprintf("%d", tier)),
		),
	)

	return &types.MsgInitiateDisputeResponse{DisputeId: disputeID}, nil
}

// CommitEvidence stores a hash commitment for evidence (commit phase).
func (m msgServer) CommitEvidence(goCtx context.Context, msg *types.MsgCommitEvidence) (*types.MsgCommitEvidenceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	dispute, found := m.GetDispute(ctx, msg.DisputeId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrDisputeNotFound, msg.DisputeId)
	}

	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT {
		return nil, fmt.Errorf("%w: expected EVIDENCE_COMMIT, got %s", types.ErrWrongPhase, dispute.Phase.String())
	}

	currentBlock := uint64(ctx.BlockHeight())
	if currentBlock > dispute.EvidenceDeadline {
		return nil, fmt.Errorf("%w: evidence commit deadline passed", types.ErrDeadlinePassed)
	}

	// Verify submitter is a party
	if msg.Submitter != dispute.Challenger && msg.Submitter != dispute.Defender {
		return nil, fmt.Errorf("%w: %s is not challenger or defender", types.ErrNotParty, msg.Submitter)
	}

	// Check no existing commitment
	if _, exists := m.GetCommitment(ctx, msg.DisputeId, msg.Submitter); exists {
		return nil, fmt.Errorf("%w: %s already committed", types.ErrCommitmentExists, msg.Submitter)
	}

	side := "challenger"
	if msg.Submitter == dispute.Defender {
		side = "defender"
	}

	commitment := &types.EvidenceCommitment{
		DisputeId:   msg.DisputeId,
		Submitter:   msg.Submitter,
		Side:        side,
		ContentHash: msg.CommitmentHash,
		CommittedAt: currentBlock,
		Revealed:    false,
	}
	m.SetCommitment(ctx, commitment)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.evidence_committed",
			sdk.NewAttribute("dispute_id", msg.DisputeId),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("side", side),
		),
	)

	return &types.MsgCommitEvidenceResponse{}, nil
}

// RevealEvidence reveals previously committed evidence content (reveal phase).
func (m msgServer) RevealEvidence(goCtx context.Context, msg *types.MsgRevealEvidence) (*types.MsgRevealEvidenceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	dispute, found := m.GetDispute(ctx, msg.DisputeId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrDisputeNotFound, msg.DisputeId)
	}

	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_EVIDENCE_REVEAL {
		return nil, fmt.Errorf("%w: expected EVIDENCE_REVEAL, got %s", types.ErrWrongPhase, dispute.Phase.String())
	}

	currentBlock := uint64(ctx.BlockHeight())
	if currentBlock > dispute.EvidenceDeadline {
		return nil, fmt.Errorf("%w: evidence reveal deadline passed", types.ErrDeadlinePassed)
	}

	// Get commitment
	commitment, exists := m.GetCommitment(ctx, msg.DisputeId, msg.Submitter)
	if !exists {
		return nil, fmt.Errorf("%w: no commitment found for %s", types.ErrCommitmentNotFound, msg.Submitter)
	}
	if commitment.Revealed {
		return nil, fmt.Errorf("%w: commitment already revealed", types.ErrAlreadyRevealed)
	}

	// Verify hash: SHA256(content + nonce) must match commitment
	h := sha256.Sum256([]byte(msg.Content + msg.Nonce))
	revealedHash := hex.EncodeToString(h[:])
	if revealedHash != commitment.ContentHash {
		return nil, fmt.Errorf("%w: expected %s, got %s", types.ErrHashMismatch, commitment.ContentHash, revealedHash)
	}

	// Mark commitment as revealed
	commitment.Revealed = true
	m.SetCommitment(ctx, commitment)

	// Store evidence
	evidenceID := fmt.Sprintf("%s-%s", msg.DisputeId, msg.Submitter)
	evidence := &types.DisputeEvidence{
		Id:          evidenceID,
		DisputeId:   msg.DisputeId,
		Submitter:   msg.Submitter,
		Side:        commitment.Side,
		Content:     msg.Content,
		SubmittedAt: currentBlock,
	}
	m.SetEvidence(ctx, evidence)

	// Increment evidence count
	dispute.EvidenceCount++
	m.SetDispute(ctx, dispute)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.evidence_revealed",
			sdk.NewAttribute("dispute_id", msg.DisputeId),
			sdk.NewAttribute("submitter", msg.Submitter),
			sdk.NewAttribute("evidence_id", evidenceID),
		),
	)

	return &types.MsgRevealEvidenceResponse{}, nil
}

// ArbiterVote records an arbiter's vote during the arbitration phase.
func (m msgServer) ArbiterVote(goCtx context.Context, msg *types.MsgArbiterVote) (*types.MsgArbiterVoteResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	dispute, found := m.GetDispute(ctx, msg.DisputeId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrDisputeNotFound, msg.DisputeId)
	}

	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_ARBITRATION {
		return nil, fmt.Errorf("%w: expected ARBITRATION, got %s", types.ErrWrongPhase, dispute.Phase.String())
	}

	currentBlock := uint64(ctx.BlockHeight())
	if currentBlock > dispute.VotingDeadline {
		return nil, fmt.Errorf("%w: voting deadline passed", types.ErrDeadlinePassed)
	}

	// Verify arbiter is assigned
	isArbiter := false
	for _, arb := range dispute.Arbiters {
		if arb == msg.Arbiter {
			isArbiter = true
			break
		}
	}
	if !isArbiter {
		return nil, fmt.Errorf("%w: %s", types.ErrNotArbiter, msg.Arbiter)
	}

	// Check not already voted
	if _, exists := m.GetVote(ctx, msg.DisputeId, msg.Arbiter); exists {
		return nil, fmt.Errorf("%w: %s", types.ErrAlreadyVoted, msg.Arbiter)
	}

	vote := &types.DisputeVote{
		DisputeId: msg.DisputeId,
		Arbiter:   msg.Arbiter,
		Vote:      msg.Vote,
		Stake:     "1000000", // default stake weight (can be enhanced with actual stake lookup)
		Rationale: msg.Reasoning,
		VotedAt:   currentBlock,
	}
	m.SetVote(ctx, vote)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.arbiter_voted",
			sdk.NewAttribute("dispute_id", msg.DisputeId),
			sdk.NewAttribute("arbiter", msg.Arbiter),
			sdk.NewAttribute("vote", msg.Vote.String()),
		),
	)

	return &types.MsgArbiterVoteResponse{}, nil
}

// EscalateDispute moves a dispute to a higher tier with additional bond.
func (m msgServer) EscalateDispute(goCtx context.Context, msg *types.MsgEscalateDispute) (*types.MsgEscalateDisputeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	dispute, found := m.GetDispute(ctx, msg.DisputeId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrDisputeNotFound, msg.DisputeId)
	}

	// Can only escalate from settled/draw outcomes or active arbitration
	if dispute.Phase == types.DisputePhase_DISPUTE_PHASE_TIMED_OUT {
		return nil, fmt.Errorf("%w: cannot escalate timed-out dispute", types.ErrWrongPhase)
	}

	// Check max tier
	if dispute.Tier >= 4 {
		return nil, types.ErrMaxTierReached
	}

	// Check escalation delay
	params := m.GetParams(ctx)
	currentBlock := uint64(ctx.BlockHeight())
	if currentBlock < dispute.CreatedAt+params.EscalationDelay {
		return nil, fmt.Errorf("%w: must wait until block %d", types.ErrEscalationTooEarly, dispute.CreatedAt+params.EscalationDelay)
	}

	// Verify requester is a party
	if msg.Requester != dispute.Challenger && msg.Requester != dispute.Defender {
		return nil, fmt.Errorf("%w: %s", types.ErrNotParty, msg.Requester)
	}

	// Escrow additional bond
	additionalBond := new(big.Int)
	if _, ok := additionalBond.SetString(msg.AdditionalBond, 10); !ok || additionalBond.Sign() <= 0 {
		return nil, fmt.Errorf("%w: %s", types.ErrInvalidBond, msg.AdditionalBond)
	}

	requesterAddr, err := sdk.AccAddressFromBech32(msg.Requester)
	if err != nil {
		return nil, fmt.Errorf("invalid requester address: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(additionalBond)))
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, requesterAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to escrow additional bond: %w", err)
	}

	// Upgrade tier
	newTier := dispute.Tier + 1
	newTierCfg := types.GetTierConfig(params, newTier)
	if newTierCfg == nil {
		return nil, fmt.Errorf("tier %d config not found", newTier)
	}

	// Select new arbiters for the escalated tier
	newArbiters, err := m.SelectArbiters(ctx, int(newTierCfg.ArbiterCount), dispute.Challenger, dispute.Defender, currentBlock)
	if err != nil {
		// Refund additional bond on failure
		_ = m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, requesterAddr, coins)
		return nil, err
	}

	// Update bond total
	existingBond := new(big.Int)
	existingBond.SetString(dispute.Bond, 10)
	existingBond.Add(existingBond, additionalBond)

	dispute.Tier = newTier
	dispute.Bond = existingBond.String()
	dispute.Arbiters = newArbiters
	dispute.Phase = types.DisputePhase_DISPUTE_PHASE_EVIDENCE_COMMIT
	dispute.EvidenceDeadline = currentBlock + newTierCfg.EvidencePeriod
	dispute.VotingDeadline = currentBlock + newTierCfg.EvidencePeriod + newTierCfg.VotingPeriod
	m.SetDispute(ctx, dispute)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.dispute_escalated",
			sdk.NewAttribute("dispute_id", msg.DisputeId),
			sdk.NewAttribute("new_tier", fmt.Sprintf("%d", newTier)),
			sdk.NewAttribute("additional_bond", msg.AdditionalBond),
			sdk.NewAttribute("total_bond", dispute.Bond),
		),
	)

	return &types.MsgEscalateDisputeResponse{NewTier: newTier}, nil
}

// SettleDispute manually triggers vote tallying and settlement (authority-gated).
func (m msgServer) SettleDispute(goCtx context.Context, msg *types.MsgSettleDispute) (*types.MsgSettleDisputeResponse, error) {
	if m.GetAuthority() != msg.Authority {
		return nil, fmt.Errorf("%w: expected %s, got %s", types.ErrUnauthorized, m.GetAuthority(), msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	dispute, found := m.GetDispute(ctx, msg.DisputeId)
	if !found {
		return nil, fmt.Errorf("%w: %s", types.ErrDisputeNotFound, msg.DisputeId)
	}

	if dispute.Phase != types.DisputePhase_DISPUTE_PHASE_ARBITRATION {
		return nil, fmt.Errorf("%w: expected ARBITRATION, got %s", types.ErrWrongPhase, dispute.Phase.String())
	}

	// Tally votes
	outcome := m.TallyVotes(ctx, dispute)
	dispute.Outcome = outcome
	if outcome == types.DisputeOutcome_DISPUTE_OUTCOME_TIMED_OUT {
		dispute.Phase = types.DisputePhase_DISPUTE_PHASE_TIMED_OUT
	} else {
		dispute.Phase = types.DisputePhase_DISPUTE_PHASE_SETTLED
	}
	m.SetDispute(ctx, dispute)

	// Distribute bonds
	if err := m.DistributeSettlement(ctx, dispute); err != nil {
		return nil, fmt.Errorf("failed to distribute settlement: %w", err)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.dispute_settled",
			sdk.NewAttribute("dispute_id", msg.DisputeId),
			sdk.NewAttribute("outcome", outcome.String()),
		),
	)

	return &types.MsgSettleDisputeResponse{Outcome: outcome}, nil
}

// UpdateParams handles MsgUpdateParams — governance-gated parameter update.
func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.GetAuthority() != msg.Authority {
		return nil, fmt.Errorf("unauthorized: expected %s, got %s", m.GetAuthority(), msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if msg.Params == nil {
		return nil, fmt.Errorf("params cannot be nil")
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	m.SetParams(ctx, msg.Params)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"zerone.disputes.params_updated",
			sdk.NewAttribute("authority", msg.Authority),
		),
	)

	return &types.MsgUpdateParamsResponse{}, nil
}

// ---------- Service Escrow handlers ----------

// CreateServiceEscrow locks payer funds into the disputes module account.
func (m msgServer) CreateServiceEscrow(goCtx context.Context, msg *types.MsgCreateServiceEscrow) (*types.MsgCreateServiceEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentHeight := uint64(ctx.BlockHeight())

	if msg.Deadline <= currentHeight {
		return nil, types.ErrEscrowExpired.Wrap("deadline must be in the future")
	}

	payerAddr, err := sdk.AccAddressFromBech32(msg.Payer)
	if err != nil {
		return nil, types.ErrInvalidEscrowAmount.Wrap("invalid payer address")
	}

	amt, ok := new(big.Int).SetString(msg.Amount, 10)
	if !ok || amt.Sign() <= 0 {
		return nil, types.ErrInvalidEscrowAmount
	}

	// Lock funds: payer account → disputes module account.
	coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(amt)))
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, payerAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock escrow funds: %w", err)
	}

	escrowID := GenerateServiceEscrowID(msg.Payer, msg.Provider, currentHeight)
	escrow := &types.ServiceEscrow{
		Id:        escrowID,
		Payer:     msg.Payer,
		Provider:  msg.Provider,
		Amount:    msg.Amount,
		TermsHash: msg.TermsHash,
		Deadline:  msg.Deadline,
		Status:    types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_PENDING,
		CreatedAt: currentHeight,
	}
	m.SetServiceEscrow(ctx, escrow)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.disputes.service_escrow_created",
		sdk.NewAttribute("escrow_id", escrowID),
		sdk.NewAttribute("payer", msg.Payer),
		sdk.NewAttribute("provider", msg.Provider),
		sdk.NewAttribute("amount", msg.Amount),
		sdk.NewAttribute("deadline", fmt.Sprintf("%d", msg.Deadline)),
		sdk.NewAttribute("terms_hash", msg.TermsHash),
	))

	return &types.MsgCreateServiceEscrowResponse{EscrowId: escrowID}, nil
}

// CompleteServiceEscrow is called by the provider to submit proof of delivery.
// Funds are NOT released immediately — they remain in the module account while the
// payer's dispute window is open. EndBlock auto-releases to the provider once
// the dispute window (Deadline) passes without a dispute.
//
// State transition: PENDING → COMPLETED
// Deadline field is repurposed: PENDING Deadline = provider's submission deadline;
// after COMPLETED, Deadline = dispute window close (currentBlock + EscrowDisputeWindowBlocks).
func (m msgServer) CompleteServiceEscrow(goCtx context.Context, msg *types.MsgCompleteServiceEscrow) (*types.MsgCompleteServiceEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentHeight := uint64(ctx.BlockHeight())

	escrow, found := m.GetServiceEscrow(ctx, msg.EscrowId)
	if !found {
		return nil, types.ErrServiceEscrowNotFound
	}
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_PENDING {
		return nil, types.ErrEscrowNotPending
	}
	if currentHeight > escrow.Deadline {
		return nil, types.ErrEscrowExpired
	}
	if escrow.Provider != msg.Provider {
		return nil, types.ErrUnauthorizedEscrow
	}

	params := m.GetParams(ctx)

	// Record proof, advance to COMPLETED, open dispute window.
	// Deadline is repurposed as dispute_window_close.
	escrow.ProofHash = msg.ProofHash
	escrow.Status = types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_COMPLETED
	escrow.Deadline = currentHeight + params.EscrowDisputeWindowBlocks
	m.SetServiceEscrow(ctx, escrow)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.disputes.service_escrow_completed",
		sdk.NewAttribute("escrow_id", escrow.Id),
		sdk.NewAttribute("provider", msg.Provider),
		sdk.NewAttribute("proof_hash", msg.ProofHash),
		sdk.NewAttribute("dispute_window_closes", fmt.Sprintf("%d", escrow.Deadline)),
	))

	return &types.MsgCompleteServiceEscrowResponse{}, nil
}

// DisputeServiceEscrow is called by the payer to contest a completed escrow within
// the dispute window. Funds remain locked in the module account pending governance
// resolution; EndBlock will NOT auto-release a DISPUTED escrow.
//
// State transition: COMPLETED → DISPUTED
func (m msgServer) DisputeServiceEscrow(goCtx context.Context, msg *types.MsgDisputeServiceEscrow) (*types.MsgDisputeServiceEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	currentHeight := uint64(ctx.BlockHeight())

	escrow, found := m.GetServiceEscrow(ctx, msg.EscrowId)
	if !found {
		return nil, types.ErrServiceEscrowNotFound
	}
	if escrow.Status != types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_COMPLETED {
		return nil, types.ErrEscrowNotCompleted
	}
	if escrow.Payer != msg.Payer {
		return nil, types.ErrUnauthorizedEscrow
	}
	// Deadline (after CompleteServiceEscrow) is the dispute window close block.
	if currentHeight > escrow.Deadline {
		return nil, types.ErrEscrowDisputeWindow
	}

	escrow.Status = types.ServiceEscrowStatus_SERVICE_ESCROW_STATUS_DISPUTED
	m.SetServiceEscrow(ctx, escrow)

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.disputes.service_escrow_disputed",
		sdk.NewAttribute("escrow_id", escrow.Id),
		sdk.NewAttribute("payer", msg.Payer),
		sdk.NewAttribute("provider", escrow.Provider),
		sdk.NewAttribute("amount", escrow.Amount),
		sdk.NewAttribute("reason", msg.Reason),
	))

	return &types.MsgDisputeServiceEscrowResponse{}, nil
}
