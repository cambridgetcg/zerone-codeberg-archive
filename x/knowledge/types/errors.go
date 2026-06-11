package types

import "cosmossdk.io/errors"

// Knowledge module sentinel errors.
var (
	// ─── Core PoT lifecycle (2–38) ───────────────────────────────────────────
	ErrClaimNotFound            = errors.Register(ModuleName, 2, "claim not found")
	ErrFactNotFound             = errors.Register(ModuleName, 3, "fact not found")
	ErrRoundNotFound            = errors.Register(ModuleName, 4, "round not found")
	ErrInvalidConfidence        = errors.Register(ModuleName, 5, "invalid confidence value")
	ErrInvalidClaim             = errors.Register(ModuleName, 6, "invalid claim")
	ErrClaimTooShort            = errors.Register(ModuleName, 7, "claim text too short")
	ErrInsufficientStake        = errors.Register(ModuleName, 8, "insufficient stake")
	ErrRoundNotInCommitPhase    = errors.Register(ModuleName, 9, "round not in commit phase")
	ErrRoundNotInRevealPhase    = errors.Register(ModuleName, 10, "round not in reveal phase")
	ErrCommitmentMismatch       = errors.Register(ModuleName, 11, "commitment hash mismatch")
	ErrNotSelectedValidator     = errors.Register(ModuleName, 12, "validator not selected for this round")
	ErrAlreadyCommitted         = errors.Register(ModuleName, 13, "validator already committed")
	ErrAlreadyRevealed          = errors.Register(ModuleName, 14, "validator already revealed")
	ErrInvalidDomain            = errors.Register(ModuleName, 15, "invalid domain")
	ErrDuplicateClaim           = errors.Register(ModuleName, 16, "duplicate claim content")
	ErrFactAlreadyChallenged    = errors.Register(ModuleName, 17, "fact already challenged")
	ErrCannotChallengeFalsified = errors.Register(ModuleName, 18, "cannot challenge a falsified fact")
	ErrInvalidIBCVersion        = errors.Register(ModuleName, 19, "invalid IBC version")
	ErrQueryRateLimited         = errors.Register(ModuleName, 20, "query rate limited")
	ErrWrongPhase               = errors.Register(ModuleName, 21, "wrong verification phase")
	ErrDeadlinePassed           = errors.Register(ModuleName, 22, "verification deadline has passed")
	ErrDuplicateCommitment      = errors.Register(ModuleName, 23, "duplicate commitment")
	ErrInvalidCommitment        = errors.Register(ModuleName, 24, "invalid commitment hash")
	ErrNoCommitment             = errors.Register(ModuleName, 25, "no commitment found")
	ErrRevealMismatch           = errors.Register(ModuleName, 26, "reveal does not match commitment")
	ErrDuplicateReveal          = errors.Register(ModuleName, 27, "duplicate reveal")
	ErrInvalidChallenge         = errors.Register(ModuleName, 28, "invalid challenge")
	ErrDomainNotFound           = errors.Register(ModuleName, 29, "domain not found")
	ErrDomainExists             = errors.Register(ModuleName, 30, "domain already exists")
	ErrInvalidVRFProof          = errors.Register(ModuleName, 31, "invalid VRF proof")
	ErrVRFSelectionFailed       = errors.Register(ModuleName, 32, "VRF validator selection failed")
	ErrValidatorNotEligible     = errors.Register(ModuleName, 33, "validator not eligible for this domain")
	ErrEquivocation             = errors.Register(ModuleName, 34, "equivocation detected")
	ErrInvalidCategory          = errors.Register(ModuleName, 35, "invalid epistemic category")
	ErrClaimStakeInsufficient   = errors.Register(ModuleName, 36, "claim stake below minimum")
	ErrRoundExpired             = errors.Register(ModuleName, 37, "verification round has expired")
	ErrUnauthorized             = errors.Register(ModuleName, 38, "unauthorized")

	// ─── Adversarial verification (40–47) ───────────────────────────────────
	ErrNotProvisional        = errors.Register(ModuleName, 40, "fact is not in provisional state")
	ErrChallengeWindowClosed = errors.Register(ModuleName, 41, "challenge window has closed")
	ErrSelfChallenge         = errors.Register(ModuleName, 42, "cannot challenge own fact")
	ErrMaxChallengesReached  = errors.Register(ModuleName, 43, "maximum concurrent challenges reached")
	ErrChallengeCooldown     = errors.Register(ModuleName, 44, "challenger is in cooldown period")
	ErrChallengeNotFound     = errors.Register(ModuleName, 45, "challenge not found")
	ErrAdversarialDisabled   = errors.Register(ModuleName, 46, "adversarial verification is disabled")
	ErrEvidenceRequired      = errors.Register(ModuleName, 47, "evidence is required for this operation")

	// ─── Negative knowledge (50–57) ─────────────────────────────────────────
	ErrCounterFactNotFound          = errors.Register(ModuleName, 50, "counter-fact not found")
	ErrCannotContradictSelf         = errors.Register(ModuleName, 51, "cannot contradict own fact")
	ErrContradictionStakeTooLow     = errors.Register(ModuleName, 52, "contradiction stake too low")
	ErrContradictionRateLimited     = errors.Register(ModuleName, 53, "contradiction rate limited")
	ErrGoedelIncompletenessRequired = errors.Register(ModuleName, 54, "Gödelian incompleteness acknowledgment required")
	ErrCannotContradictCounterFact  = errors.Register(ModuleName, 55, "cannot contradict a counter-fact")
	ErrFactAlreadyDisproven         = errors.Register(ModuleName, 56, "fact is already disproven")
	ErrCounterFactAlreadyExists     = errors.Register(ModuleName, 57, "counter-fact already exists for this fact")

	// ─── Knowledge pruning (58–66) ──────────────────────────────────────────
	ErrFactNotAtRisk         = errors.Register(ModuleName, 58, "fact is not at-risk")
	ErrFactImmune            = errors.Register(ModuleName, 59, "fact is immune from pruning")
	ErrInsufficientPatronage = errors.Register(ModuleName, 60, "insufficient patronage amount")
	ErrFactAlreadyPruned     = errors.Register(ModuleName, 61, "fact is already pruned")
	ErrPatronageExpired      = errors.Register(ModuleName, 62, "patronage has expired")
	ErrPruningDisabled       = errors.Register(ModuleName, 63, "knowledge pruning is disabled")
	ErrDuplicateChallenge    = errors.Register(ModuleName, 64, "challenge already exists")
	ErrStratumNotFound       = errors.Register(ModuleName, 65, "stratum not found")
	ErrProposalNotFound      = errors.Register(ModuleName, 66, "proposal not found")

	// ─── Domain qualification (70) ────────────────────────────────────────
	ErrUnqualifiedVerifier = errors.Register(ModuleName, 70, "verifier not qualified for domain")

	// ─── Partnership validation (80–81) ──────────────────────────────────
	ErrInvalidPartnership  = errors.Register(ModuleName, 80, "invalid partnership")
	ErrPartnershipFrozen   = errors.Register(ModuleName, 81, "partnership is frozen")

	// ─── Priority tip validation (82) ────────────────────────────────────
	ErrPriorityTipExceedsMax = errors.Register(ModuleName, 82, "priority_tip_bps exceeds governance maximum")

	// ─── Verifier independence gate (83) — P-10a ─────────────────────────
	ErrLowVerifierIndependence = errors.Register(ModuleName, 83, "verifier independence score below governance minimum")

	// ─── Human-only panel gate (84) — P-13 ───────────────────────────────
	ErrAgentNotAllowedOnHumanPanel = errors.Register(ModuleName, 84, "agent accounts may not verify human-only claims")

	// ─── Pragmatic consequence instances (85–87) — Pragmatism Gap 1 ──────
	ErrConsequenceInstanceWrongMethodology = errors.Register(ModuleName, 85, "consequence instances only valid on M-PRAGMATIC claims")
	ErrConsequenceInstanceRejectedClaim    = errors.Register(ModuleName, 86, "cannot add consequence instance to a rejected claim")
	ErrInsufficientConsequenceInstances    = errors.Register(ModuleName, 87, "M-PRAGMATIC claim has insufficient consequence instances for ACCEPT verdict")

	// ─── Temporal verification window (88) — Contemplative Traditions Gap ─
	ErrVerificationWindowTooShort = errors.Register(ModuleName, 88, "verification round has not run long enough for this methodology's temporal requirement")

	// ─── Methodology-level account type gate (89) — Phenomenology Brief ──
	ErrMethodologyRequiresAccountType = errors.Register(ModuleName, 89, "verifier account type not permitted by this methodology's required_account_types")

	// ─── Attentional practice gate (90) — Contemplative Traditions Gap ───
	ErrInsufficientAttentionalPractice = errors.Register(ModuleName, 90, "verifier attentional practice hours below governance minimum for M-PHENOMENOLOGICAL panels")
)
