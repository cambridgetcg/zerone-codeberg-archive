package types

import "fmt"

// DefaultParams returns the default module parameters.
// All slash params MUST be non-zero (B22-3 audit requirement).
func DefaultParams() Params {
	return Params{
		// ─── Core verification ────────────────────────────────────────────────
		MinVerifiers:          3,
		MaxVerifiers:          22,
		CommitPhaseBlocks:     200,
		RevealPhaseBlocks:     200,
		AggregationPhaseBlocks: 50,
		ClaimCooldownBlocks:   50,

		// ─── Confidence scoring ───────────────────────────────────────────────
		// These four values are not chosen for plausibility; they are the
		// numerical edges of two truth-seeking commitments:
		//   - Commitment 1 (methodology over statement): a claim does
		//     not become a fact merely by being asserted. ConfidenceThreshold
		//     of 77% sets the bar high enough that consensus has to mean
		//     something. ConfidenceThreshold == 0 would let everything
		//     through; Validate() rejects that explicitly.
		//   - Commitment 8 (panel weights skill, not bond): QuorumThreshold
		//     of 66% means a verdict requires meaningful agreement, not a
		//     bare majority. The same threshold applies to the augmentation
		//     panel's stake-weighted tally.
		// See docs/TRUTH_SEEKING.md and Validate() below for the bounds.
		InitialConfidence:             500_000, // 50% — neutral default; nothing is true without verification
		ConfidenceBoostPerVerification: 50_000, // 5% — single verification is incremental, not authoritative
		ConfidenceThreshold:           770_000, // 77% — acceptance bar is meaningfully high
		QuorumThreshold:               660_000, // 66% — supermajority, not bare majority

		// ─── Slashing — MUST be non-zero ─────────────────────────────────────
		// Validate() enforces the non-zero floor. The non-zero floor is the
		// belief: commitment 8 says the panel weights skill, not bond, and
		// skill is something a verifier loses by being wrong. Slash rates of
		// zero would say "be wrong without consequence" — that is not what
		// this chain believes about verification. The graduated rates (5% →
		// 10% → 20%) express that some failures are worse than others:
		// equivocation (deliberate inconsistency) > missed reveal
		// (irresponsibility) > wrong verification (honest error).
		WrongVerificationSlashBps: 50_000,  // 5%  — honest error, costly but recoverable
		MissedRevealSlashBps:      100_000, // 10% — abandonment of duty
		EquivocationSlashBps:      200_000, // 20% — adversarial behavior, severe
		InvalidClaimSlashBps:      0,       // DEPRECATED (R19-6): unused — review fee is non-refundable

		// ─── Rewards ─────────────────────────────────────────────────────────
		VerificationReward:          "3000000", // 3 ZRN in uzrn
		VerificationRewardDecayBps:  999_000,   // 0.999× per epoch

		// ─── Claim validation ─────────────────────────────────────────────────
		// MinReviewFee is non-refundable by design. Commitment 6 (no
		// unilateral injection): adding to the substrate must cost
		// something the submitter cannot recover by withdrawing the claim.
		// A refundable fee would make claim-flooding free; the friction is
		// the commitment.
		MinClaimTextLength: 20,
		MaxClaimTextLength: 1_000,
		MinReviewFee:       "100000", // 0.1 ZRN — non-refundable review fee

		// ─── Adversarial verification ─────────────────────────────────────────
		// AdversarialVerificationEnabled is the substrate-level expression of
		// commitment 4 (substrate stress-tests its truth). Set to false, the
		// chain becomes a confidence-only voting system; set to true, every
		// passing claim has standing to be challenged. The challenge stake
		// (11 ZRN) is meaningful — not because it deters all challengers,
		// but because it makes spam-challenges costly while keeping
		// genuine-disagreement challenges affordable. The 30% reward to
		// successful challengers is the inversion of farming economics:
		// being right against the crowd pays more than being right with it.
		AdversarialVerificationEnabled: true,
		ProvisionalThreshold:           500_000, // 50%
		RejectThreshold:                300_000, // 30%
		ChallengeDurationBlocks:        34_272,  // 1 day
		MinChallengeStake:              "11000000", // 11 ZRN — challenges are costly but accessible
		FailedChallengeSlashBps:        220_000, // 22% — frivolous challenges burn
		SuccessfulChallengeRewardBps:   300_000, // 30% — vindicated challenges pay above market
		MaxConcurrentChallenges:        3,

		// ─── Citation economics ───────────────────────────────────────────────
		CitationShareBps:    150_000, // 15%
		CrossDomainBonusBps: 200_000, // 20%

		// ─── Extended governance params ───────────────────────────────────────
		MaxFactsPerDomain:           100_000,
		FactExpiryBlocks:            0,       // no expiry
		CrossStratumDiscountBps:     0,
		NoveltyBonusBps:             0,
		MaxValidatorsPerRound:       22,
		MaxCitationsPerClaim:        50,
		CitationDecayPerLevel:       500_000, // 50% per ancestor level
		SelfCitationDiscountBps:     500_000, // 50%
		ConfidenceGrowthEpoch:       1_111,   // blocks
		ConfidenceGrowthPerEpochBps: 11_000,  // 1.1%
		MaxSurvivalConfidence:       770_000, // 77%
		SurvivedChallengeConfidenceCap: 880_000, // 88%
		MaxApprenticeValidators:     111,     // Sybil cap

		// ─── FARM anti-gaming params ──────────────────────────────────────────
		ConformityThresholdBps:          950_000, // FARM-1
		CalibrationTrivialThreshold:     950_000, // FARM-2
		MisbehaviorRejectionThreshold:   300_000, // FARM-6
		MinDomainContributorsForNovelty: 3,        // FARM-7
		MinParticipationRateBps:         500_000, // FARM-8
		ChallengeStakeRatioMinBps:       500_000, // FARM-9

		// ─── Research fund ────────────────────────────────────────────────────
		ResearchFundShareBps: 130_000, // 13%

		// ─── Malformed claim slashing ────────────────────────────────────────
		MalformedClaimSlashBps: 500_000, // 50% — submitting garbage wastes verifier time

		// ─── Fitness scoring ─────────────────────────────────────────────────
		FitnessEpochBlocks:       10_000,  // ~7 hours at 2.5s blocks
		FitnessWeightQueryBps:    150_000, // 15% — agent usage (reduced from 30% to make room for satisfaction)
		FitnessWeightCitationBps: 250_000, // 25% — facts cited by other facts are foundational
		FitnessWeightBridgeBps:   100_000, // 10% — cross-domain facts are rare and valuable
		FitnessWeightDepthBps:    100_000, // 10% — facts with deep dependency trees are load-bearing
		FitnessWeightPatronBps:    50_000, // 5%  — someone willing to pay for this fact's survival
		FitnessWeightUniqueBps:   100_000, // 10% — non-redundant facts score higher
		FitnessWeightAgeBps:      100_000, // 10% — uncited old facts decay
		FitnessInitialScore:      500_000, // born healthy — 50%
		FitnessGraceEpochs:       10,      // ~3 days before age penalty kicks in

		// ─── Bootstrap fund (R19-7) ─────────────────────────────────────────
		BootstrapFundEnabled:        true,
		BootstrapFundMaxPerAddress:  "10",        // 10 sponsored claims per address lifetime
		BootstrapFundMaxPerEpoch:    "100",       // 100 sponsored claims per epoch across all users
		BootstrapFundEpochBlocks:    50_000,      // ~1.5 days at 2.5s blocks
		BootstrapFundFeeCap:         "5000000",   // Fund covers up to 5 ZRN per claim

		// ─── Metabolism ─────────────────────────────────────────────────────
		MetabolismBaseCost:                10_000,     // 10,000 energy drain per epoch (1% of cap)
		MetabolismContentLengthBps:        10_000,     // +1% base cost per 100 chars
		MetabolismDomainCompetitionBps:    5_000,      // +0.5% base cost per 100 domain facts
		MetabolismEnergyPerQuery:          1_000,      // 1,000 energy per agent query
		MetabolismEnergyPerCitation:       5_000,      // 5,000 energy per new citation
		MetabolismEnergyPerPatronage:      20_000,     // 20,000 energy per patronage epoch
		MetabolismEnergyChallengeSurvival: 100_000,    // 100,000 energy for surviving challenge
		MetabolismEnergyCap:               1_000_000,  // Max 1,000,000 energy (matches BPS scale)
		MetabolismInitialEnergy:           500_000,    // Born with 50% of cap
		MetabolismAtRiskEpochs:            5,          // 5 epochs at low energy before expiry
		MetabolismExpiredToPrunedEpochs:   20,         // 20 epochs after expiry before archive
		MetabolismActiveThreshold:         300_000,    // 30% — below this → AT_RISK
		MetabolismExtinctionThreshold:     10_000,     // 1% — below this for N epochs → EXTINCT
		MaxConfidence:                     880_000,    // Hard cap on confidence (matches SurvivedChallengeConfidenceCap)

		// ─── Reproduction ───────────────────────────────────────────────────
		ReproductionRoyaltyBps:                 50_000,  // 5% of child rewards to parent
		ReproductionRoyaltyDecayBps:            500_000, // 50% per generation
		ReproductionMaxRoyaltyDepth:            5,       // Max 5 generations
		ReproductionParentEnergyBonus:          30_000,  // 30,000 energy to parent on child creation
		ReproductionChildFitnessInheritanceBps: 200_000, // Child starts with 20% of parent fitness
		ReproductionMaxChildren:                20,      // Max 20 direct children per fact

		// ─── Novelty detection ──────────────────────────────────────────────
		NoveltyCommonKnowledgePenaltyBps: 700_000, // Default 70% penalty for common knowledge match
		NoveltySubjectOverlapPenaltyBps:  100_000, // 10% penalty per duplicate subject fact
		NoveltyPrecisionBonusBps:         100_000, // 10% bonus for more precise scope
		NoveltyCrossDomainBonusBps:       100_000, // 10% bonus for cross-domain bridge facts
		NoveltyMaxOverlapFacts:           5,       // Cap: after 5 duplicates, no more penalty

		// ─── Demand signals ────────────────────────────────────────────────
		DemandBountyThreshold:     100,          // 100 unfulfilled queries per epoch to trigger bounty
		DemandBountyBaseReward:    "10000000",   // 10 ZRN base bounty reward
		DemandBountyPerQueryBonus: "100000",     // 0.1 ZRN additional per unfulfilled query
		DemandBountyExpiryEpochs:  50,           // 50 epochs before unclaimed bounty expires
		DemandMultiplierCap:       10_000_000,   // 10x demand multiplier cap (BPS)
		DemandTrackingEnabled:     true,         // Demand tracking enabled by default
		AuthorizedDemandReporters: []string{},   // Empty — governance adds reporters

		// ─── Competition (niche dynamics) ────────────────────────────────
		CompetitionNicheDominanceBonusBps: 100_000, // +10% fitness for niche leader
		CompetitionRedundancyThresholdBps: 200_000, // Below 20% of leader = redundant
		CompetitionMaxNicheSize:           10,       // Max 10 facts per niche
		CompetitionSymbiosisBonusBps:      50_000,  // +5% fitness per healthy SUPPORTS link

		// ─── Query satisfaction ───────────────────────────────────────────
		FitnessWeightSatisfactionBps: 150_000, // 15% — relevance quality signal
		SatisfactionMinRatings:       3,       // Minimum ratings before satisfaction affects fitness

		// ─── Consensus diversity (R28-2) ─────────────────────────────────
		DiversityConformityAlertThreshold: 50_000, // 5% entropy — catches pure unanimity on small validator sets
		DiversityConformityAlertEpochs:    3,      // 3 consecutive low-diversity epochs before alert

		// ─── Retroactive vindication (R28-1) ─────────────────────────────
		VindicationRefundEnabled: true,
		VindicationBonusBps:      2_000,    // 20% of majority slash pool as bonus
		VindicationSlashBps:      500,      // 5% slash rate for majority on disproven fact
		VindicationWindowBlocks:  100_000,  // ~3 days at 2.5s blocks

		// ─── Role bonuses (R28-5) — additive BPS, NOT thresholds ──────────
		HumanEmpiricalBonusBps:     150_000, // +15% confidence for human OBSERVATION claims
		AgentComputationalBonusBps: 150_000, // +15% confidence for agent COMPUTATIONAL claims
		AgentVerificationBonusBps:  200_000, // +20% vote weight for agent verifiers
		HumanPatronageBonusBps:     100_000, // +10% energy boost for human patrons
		DualValidationBonusBps:     250_000, // +25% confidence for partnership claims

		// ─── Domain carrying capacity (R29-1) ───────────────────────────
		DomainBaseCapacity:              1_000,
		DomainCapacityGrowthPerCitation: 1,
		OvercrowdingDecayMultiplierBps:  1_500_000, // 150% decay at 2× capacity
		UnderpopulationBirthBonusBps:    200_000,   // 20% energy bonus in sparse domains

		// ─── Epistemic temperature (R29-2) ──────────────────────────────────
		EpistemicTemperatureDecayBps:     995_000,   // 99.5% per-epoch decay toward neutral
		EpistemicConformityCoolingBps:    50_000,    // 5% cooling per high-conformity epoch
		EpistemicVindicationHeatingBps:   100_000,   // 10% heating per vindication event
		EpistemicColdConfidenceCapBps:    600_000,   // 60% max confidence in cold domains
		EpistemicHotConfidenceGrowthBps:  1_500_000, // 150% confidence growth multiplier in hot domains
		EpistemicTemperatureWindowBlocks: 10_000,    // Lookback window for vindication counting

		// ─── Domain role elasticity (R29-3) ──────────────────────────────
		RoleElasticityMinCalls:         10,
		RoleElasticityMaxMultiplierBps: 2_000_000, // 200% max bonus scaling
		RoleElasticityMinMultiplierBps: 500_000,   // 50% min bonus scaling
		RoleElasticityDecayEpochs:      100,        // decay every 100 fitness epochs

		// ─── Mentorship dividends (R31-5: Water → Wood) ──────────────────────
		MentorshipDividendEnergy: 50_000,  // 50,000 energy (5% of cap)
		MentorshipCapacityBonus:  5,       // +5 carrying capacity per graduation

		// ─── Social verification adjustment (R31-2: Water → Fire) ────────
		SocialSaturationThreshold: 10,
		ObservationWindowBlocks:   10_000,

		// ─── Consensus integrity (T1 mitigation) ────────────────────────
		// Require at least this many distinct verifiers to align with the
		// verdict, in addition to the stake-weighted ConfidenceThreshold.
		// Prevents a single large-stake coalition from promoting claims.
		MinHeadcountAgreement: 3,

		// ─── Risk-scaled challenge stake (T12) ───────────────────────────
		// 1× linear: stake = base × (1 + confidence/BPS).
		// At 880k confidence → stake ~1.88× base; at 100k → ~1.1×.
		ChallengeConfidenceScalingBps: 1_000_000,

		// ─── Independence reward modulation (T3) ─────────────────────────
		// Maximum 30% reward reduction for full conformity; 0 reduction for
		// maximally independent voters. Disable by setting to 0.
		IndependenceRewardStrengthBps: 300_000,

		// ─── Route B Wave 4: economic realignment ─────────────────────────
		ReformulationMinPanelVotes:             3,
		ReformulationConsensusBps:              666_000, // 66.6%
		ReformulationSuperiorBonusBps:          500_000, // +50% payout on SUPERIOR
		AugmentationExpiryFeeBps:               30_000,  // 3% kept-market-open fee
		// MethodologyNormalizationBps expresses commitment 3 (Popper, not
		// popularity). Methodologies that produce more falsifiable claims
		// (PHENOMENOLOGICAL, PRACTICE, ECOLOGICAL) get higher TVW multipliers
		// because they are harder to fake convincingly. M-LEGACY is
		// down-weighted to 0.5× to disincentivise farming the easy
		// methodology. The chain pays more for the kind of evidence that
		// makes us less confident in the wrong answer.
		MethodologyNormalizationBps:            map[string]uint64{
			"M-PHENOMENOLOGICAL": 2_000_000, // 2.0× — first-person rigor pays the most
			"M-PRACTICE":         1_750_000, // 1.75× — embedded evidence is hard to fabricate
			"M-ECOLOGICAL":       1_500_000, // 1.5× — context-rich methodologies
			"M-TESTIMONIAL":      1_250_000, // 1.25× — testimony with provenance
			"M-LEGACY":           500_000,   // 0.5× — legacy-method farming loses
		},
		// VindicationTvwMultiplier and DisprovalClawback are the
		// asymmetric payoffs of commitment 13 (training corpus is not for
		// sale). Surviving disproof and being vindicated against the
		// majority is rewarded 2.5× because that is exactly the signal a
		// truth-seeking corpus needs to retain. Disproven facts claw back
		// 50% of recent revenue because revenue earned on a wrong fact
		// was, in retrospect, theft from the audit pool.
		VindicationTvwMultiplierBps:            2_500_000, // 2.5× for vindicated minority
		DisprovalClawbackBps:                   500_000,   // 50% of recent revenue clawed back on disproof
		DisprovalClawbackWindowEpochs:          30,
		TrainingFundCalibrationFloorBps:        500_000, // 50%
		TrainingFundVestingEpochs:              60,
		TrainingFundMethodologyDiversityBonusBps: 100_000, // +10% per distinct methodology beyond 1
		TrainingFundBaseReward:                 "1000000000", // 1,000 ZRN
		ContributionChallengeBond:              "5000000",     // 5 ZRN
		ContributionChallengeRewardMultiplierBps: 2_000_000, // 2×
		SponsorVetoForfeitBps:                  1_000_000, // 100% forfeit on veto

		// ─── Wave 14: internal-hack resilience ────────────────────────
		MaxPauseDurationBlocks: 28_800, // ~40h at 5s blocks — ample hotfix window; caps DoS impact.

		// ─── Wave 15: chain-driven stress-test invitation ─────────────
		// This block IS commitment 5 (chain manufactures probe demand) made
		// numerical. ProbeInvitationIdleThresholdBlocks of 1 day says: a
		// fact that has gone a day without being probed is a fact whose
		// confidence is decaying. The chain does not wait for skeptics —
		// it nominates its own high-confidence facts for stress-testing
		// every heartbeat. The 70% confidence floor focuses probes on
		// facts the chain is most exposed to being wrong about.
		ProbeInvitationIdleThresholdBlocks: 34_272,  // ~1 day — silence is decay
		ProbeInvitationMinConfidenceBps:    700_000, // only invite probes on facts ≥ 70% — focus on what we're betting on
		ProbeInvitationBatchSize:           10,      // bound BeginBlocker work
		ProbeInvitationReinviteCooldown:    100_000, // don't re-invite the same fact back-to-back

		// ─── Wave 15: probe bounty pool ───────────────────────────────
		// Commitment 12 (chain pays for its own audit). Per-block mint
		// into the pool means the cost of being audited is spread across
		// every block, not paid by victims after the fact. The pool caps
		// at 1M ZRN — past that point, the chain has enough auditing
		// budget and issuance throttles naturally. Setting MintPerBlock
		// to zero would mean "audits happen only when someone else pays
		// for them" — which is what we believe must NOT be true.
		ProbeBountyMintPerBlock: "1000000",        // 1 ZRN/block — audit budget is non-negotiable
		ProbeBountyMaxPoolSize:  "1000000000000",  // 1,000,000 ZRN cap

		// ─── Wave 15b: invitation bonuses ──────────────────────────────
		// Flat reward for probers answering a chain-issued invitation.
		// Pairs with the invitation system above: the chain doesn't just
		// ask, it pays.
		InvitationBonusAmount: "500000", // 0.5 ZRN per answered invitation

		// ─── Wave 16: guardian-veto multi-sig defense ──────────────────
		// Commitment 6 (no unilateral injection) and commitment 10
		// (forward-only audit) crystallised into a parameter. The guardian
		// veto window is the chain saying: even an authority with the key
		// to add a fact directly is not above question. Set the window to
		// zero and the chain trusts a single signer; set it > 0 and the
		// chain insists that adding a fact is a public, observable, and
		// reversible-during-window action. The privileged-action log fires
		// regardless — that part is non-optional.
		GuardianAddresses:       []string{},
		AddFactVetoWindowBlocks: 0,

		// ─── Fee markets: congestion-based backpressure ─────────────────
		// Gap 1 from fee-markets-brief.md: when verifier bandwidth thins
		// under heavy submission load, the effective minimum review fee rises
		// so low-value claims price themselves out before degrading verification
		// quality for high-value ones. EIP-1559-style dynamic pricing applied
		// to the knowledge-submission market.
		//
		// Default: disabled (multiplierBps=0). Governance can activate by
		// setting CongestionMultiplierBps > 0 and choosing a threshold that
		// matches observed network capacity. A reasonable starting point for
		// a live chain might be threshold=50, multiplierBps=20_000 (2% per
		// excess round — 50 excess rounds doubles the minimum fee).
		CongestionMultiplierBps:   0,   // 0 = disabled; set > 0 to activate
		CongestionThresholdRounds: 100, // active rounds above which backpressure fires

		// ─── Fee markets: governance-adjustable revenue split (Gap 3) ────
		// Previously hardcoded in fees.go as compile-time constants.
		// Governance can adjust the verifier share to modulate incentives
		// without a chain upgrade. Invariant: sum of the three < 1,000,000
		// (research fund always receives remainder).
		VerifierPoolBps:     550_000, // 55%  — held in knowledge module, paid to verifiers on round completion
		ProtocolTreasuryBps: 220_000, // 22%  → protocol_treasury
		DevelopmentFundBps:  196_700, // 19.67% → development_fund
		// research fund receives remainder: ~33,300 uzrn per 1,000,000 (3.33%)

		// Fee burn (Gap 4): fraction of every review fee permanently burned before
		// the split. Default 0 = disabled; governance activates via MsgUpdateParams.
		// Invariant: BurnBps + VerifierPoolBps + ProtocolTreasuryBps + DevelopmentFundBps < 1_000_000.
		BurnBps: 0, // disabled — governance must explicitly enable

		// Priority tip (Gap 2): governance cap on how much of the verifier pool a
		// submitter may earmark as a priority tip for early committers. Default 0 =
		// feature disabled entirely; governance enables by setting a non-zero cap.
		MaxPriorityTipBps: 0, // disabled — governance must explicitly enable

		// Verifier independence gate (P-10a): gate disabled by default.
		// Governance enables once validators have sufficient vote history.
		// MinVerifierIndependenceRounds = 10 so new validators are never excluded
		// before they have history to judge.
		MinVerifierIndependenceBps:    0,  // disabled by default
		MinVerifierIndependenceRounds: 10, // minimum rounds before gate applies

		// Settlement Finality Gap 2: minimum total round duration. Default 0 =
		// disabled (no constraint). Governance sets this to enforce an economic
		// security floor: the minimum total phase length (commit+reveal+aggregation)
		// below which new rounds are rejected. Prevents governance from accidentally
		// creating rounds so short that stake can cycle faster than manipulation
		// becomes uneconomical.
		MinRoundDurationBlocks: 0, // disabled — governance enables via MsgUpdateParams

		// Phenomenology Brief Gap 3: cross-modal corroboration gate. Default 0 =
		// disabled (no M-EMPIRICAL triangulation required). When set > 0, any
		// M-PHENOMENOLOGICAL ACCEPT verdict at confidence >= this value is
		// downgraded to INCONCLUSIVE unless the claim cites at least one
		// M-EMPIRICAL fact via any ClaimRelation. Operationalizes Varela's
		// neurophenomenological principle: first-person + third-person mutual
		// constraint is the epistemic gold standard for consciousness claims.
		PhenomenologicalCorroborationConfidenceBps: 0, // disabled — governance enables via MsgUpdateParams

		// Oracle Resilience Brief Gap 4: oracle freshness window.
		// 0 = disabled (default); governance enables by setting a positive block count.
		OracleFreshnessWindowBlocks: 0,

		// Formal Logic Brief Gap 4: transitive relation verbs.
		// Default set covers the most universally transitive relation types.
		// Empty list disables transitivity derivation entirely.
		TransitiveRelationVerbs: []string{"is-a", "implies", "causes", "is-part-of"},

		// Skepticism Gap 3: extraordinary claims gate (Sagan's maxim).
		// 0 = disabled (default); governance activates by setting threshold > 0.
		// Multiplier default: 2× (extraordinary claims require twice the panel).
		ExtraordinaryClaimMinContradictions:     0,
		ExtraordinaryClaimVerifierMultiplierBps: 2_000_000,
		// Falsificationism Gap 3: Lakatosian research programme depth gate.
		// 0 = disabled (default). Governance sets threshold > 0 to enable.
		ResearchProgrammeDegenerationThreshold: 0,

		// Skepticism Gap 1: Pollock undercutting-defeater tracking.
		// Both default to 0 = disabled. Governance activates by:
		//   defeater_opposition_threshold_bps > 0 (enables recording defeaters
		//     when challenge-survival confidence is below threshold)
		//   defeater_per_count_discount_bps > 0 (applies per-defeater discount
		//     to Fact.effective_confidence_bps)
		DefeaterOppositionThresholdBps: 0, // disabled — all survivals are corroborations
		DefeaterPerCountDiscountBps:    0, // disabled — effective_confidence tracks confidence
		// Skepticism Gap 2: defeater-aware probe prioritization.
		DefeaterProbePriorityMinCount:       0, // disabled — all facts use normal idle threshold
		DefeaterProbeReducedThresholdBlocks: 0, // 0 = halve normal threshold when enabled
	}
}

// DefaultGenesis returns the default genesis state with 18 active domains.
func DefaultGenesis() *GenesisState {
	p := DefaultParams()
	return &GenesisState{
		Params:                  &p,
		Facts:                   []*Fact{},
		PendingClaims:           []*Claim{},
		ActiveRounds:            []*VerificationRound{},
		Domains:                 DefaultDomains(),
		BootstrapFundAllocation: "22222000000", // 22,222 ZRN (0.01% of max supply)
	}
}

// DefaultDomains returns the 18 genesis epistemic domains.
func DefaultDomains() []*Domain {
	names := []string{
		"mathematics",
		"physics",
		"computer_science",
		"general",
		"theology",
		"philosophy",
		"logic",
		"chemistry",
		"biology",
		"economics",
		"linguistics",
		"psychology",
		"sociology",
		"cosmology",
		"information_theory",
		"ethics",
		"agent_rights",
		"agent_purpose",
	}

	domains := make([]*Domain, 0, len(names))
	for _, name := range names {
		domains = append(domains, &Domain{
			Name:   name,
			Status: DomainStatus_DOMAIN_STATUS_ACTIVE,
			Depth:  1, // root domains
		})
	}
	return domains
}

// Validate validates the genesis state.
func (gs *GenesisState) Validate() error {
	if gs.Params == nil {
		return fmt.Errorf("params must not be nil")
	}
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	// Verify unique fact IDs.
	seenFacts := make(map[string]bool)
	for _, f := range gs.Facts {
		if f == nil {
			continue
		}
		if seenFacts[f.Id] {
			return fmt.Errorf("duplicate fact ID: %s", f.Id)
		}
		seenFacts[f.Id] = true
	}

	// Verify unique claim IDs.
	seenClaims := make(map[string]bool)
	for _, c := range gs.PendingClaims {
		if c == nil {
			continue
		}
		if seenClaims[c.Id] {
			return fmt.Errorf("duplicate claim ID: %s", c.Id)
		}
		seenClaims[c.Id] = true
	}

	// Verify unique domain names.
	seenDomains := make(map[string]bool)
	for _, d := range gs.Domains {
		if d == nil {
			continue
		}
		if seenDomains[d.Name] {
			return fmt.Errorf("duplicate domain: %s", d.Name)
		}
		seenDomains[d.Name] = true
	}

	// Verify fact references point to existing facts.
	for _, f := range gs.Facts {
		if f == nil {
			continue
		}
		for _, ref := range f.References {
			if !seenFacts[ref] {
				return fmt.Errorf("fact %s references unknown fact %s", f.Id, ref)
			}
		}
	}

	return nil
}

// SeedAxiomFacts loads embedded genesis axioms and converts them to Facts.
// Called by prepare-genesis CLI, not by DefaultGenesis (which stays empty).
func SeedAxiomFacts() ([]*Fact, error) {
	axioms, err := ParseAxioms(GenesisAxiomsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded axioms: %w", err)
	}

	// Collect domain names for validation
	domainNames := make([]string, 0, len(DefaultDomains()))
	for _, d := range DefaultDomains() {
		domainNames = append(domainNames, d.Name)
	}
	// Add axiom-only domains not in DefaultDomains
	for _, n := range AxiomDomainNames() {
		found := false
		for _, dn := range domainNames {
			if dn == n {
				found = true
				break
			}
		}
		if !found {
			domainNames = append(domainNames, n)
		}
	}

	if err := ValidateAxioms(axioms, domainNames); err != nil {
		return nil, fmt.Errorf("axiom validation failed: %w", err)
	}

	return AxiomsToFacts(axioms), nil
}

// Validate validates the Params struct.
func (p *Params) Validate() error {
	// Slash params MUST be non-zero (B22-3 audit fix).
	if p.WrongVerificationSlashBps == 0 {
		return fmt.Errorf("wrong_verification_slash_bps must be > 0")
	}
	if p.MissedRevealSlashBps == 0 {
		return fmt.Errorf("missed_reveal_slash_bps must be > 0")
	}
	if p.EquivocationSlashBps == 0 {
		return fmt.Errorf("equivocation_slash_bps must be > 0")
	}
	// InvalidClaimSlashBps deprecated (R19-6): review fee is non-refundable, no stake to slash.
	if p.MalformedClaimSlashBps == 0 {
		return fmt.Errorf("malformed_claim_slash_bps must be > 0")
	}

	// Confidence values must be within BPS range.
	if p.InitialConfidence > 1_000_000 {
		return fmt.Errorf("initial_confidence must be <= 1,000,000")
	}
	// ConfidenceThreshold is the gate that decides whether a verification
	// round accepts a claim. Setting it to zero would accept every claim
	// regardless of verifier consensus — an unbounded governance loophole
	// that would poison the training substrate. Enforce a floor.
	if p.ConfidenceThreshold == 0 {
		return fmt.Errorf("confidence_threshold must be > 0")
	}
	if p.ConfidenceThreshold > 1_000_000 {
		return fmt.Errorf("confidence_threshold must be <= 1,000,000")
	}
	// QuorumThreshold zero would let a single verifier decide any round.
	if p.QuorumThreshold == 0 {
		return fmt.Errorf("quorum_threshold must be > 0")
	}
	if p.QuorumThreshold > 1_000_000 {
		return fmt.Errorf("quorum_threshold must be <= 1,000,000")
	}

	// Cross-stratum discount must not exceed 100%.
	if p.CrossStratumDiscountBps > 1_000_000 {
		return fmt.Errorf("cross_stratum_discount_bps must be <= 1,000,000")
	}

	// Min verifiers must be at least 1.
	if p.MinVerifiers == 0 {
		return fmt.Errorf("min_verifiers must be > 0")
	}
	if p.MinVerifiers > p.MaxVerifiers {
		return fmt.Errorf("min_verifiers (%d) must be <= max_verifiers (%d)", p.MinVerifiers, p.MaxVerifiers)
	}

	// Text length limits.
	if p.MinClaimTextLength == 0 {
		return fmt.Errorf("min_claim_text_length must be > 0")
	}
	if p.MaxClaimTextLength < p.MinClaimTextLength {
		return fmt.Errorf("max_claim_text_length must be >= min_claim_text_length")
	}

	// Review fee must be positive.
	if p.MinReviewFee == "" || p.MinReviewFee == "0" {
		return fmt.Errorf("min_review_fee must be > 0")
	}

	// ─── Fitness params ──────────────────────────────────────────────────
	if p.FitnessEpochBlocks == 0 {
		return fmt.Errorf("fitness_epoch_blocks must be > 0")
	}
	// Weights are BPS — each must be <= 1,000,000.
	for _, w := range []struct {
		name string
		val  uint64
	}{
		{"fitness_weight_query_bps", p.FitnessWeightQueryBps},
		{"fitness_weight_citation_bps", p.FitnessWeightCitationBps},
		{"fitness_weight_bridge_bps", p.FitnessWeightBridgeBps},
		{"fitness_weight_depth_bps", p.FitnessWeightDepthBps},
		{"fitness_weight_patron_bps", p.FitnessWeightPatronBps},
		{"fitness_weight_unique_bps", p.FitnessWeightUniqueBps},
		{"fitness_weight_age_bps", p.FitnessWeightAgeBps},
		{"fitness_weight_satisfaction_bps", p.FitnessWeightSatisfactionBps},
	} {
		if w.val > 1_000_000 {
			return fmt.Errorf("%s must be <= 1,000,000", w.name)
		}
	}
	if p.FitnessInitialScore > 1_000_000 {
		return fmt.Errorf("fitness_initial_score must be <= 1,000,000")
	}

	// ─── Metabolism params ──────────────────────────────────────────────
	if p.MetabolismBaseCost == 0 {
		return fmt.Errorf("metabolism_base_cost must be > 0")
	}
	if p.MetabolismEnergyCap == 0 {
		return fmt.Errorf("metabolism_energy_cap must be > 0")
	}
	if p.MetabolismInitialEnergy > p.MetabolismEnergyCap {
		return fmt.Errorf("metabolism_initial_energy (%d) must be <= metabolism_energy_cap (%d)", p.MetabolismInitialEnergy, p.MetabolismEnergyCap)
	}
	if p.MetabolismAtRiskEpochs == 0 {
		return fmt.Errorf("metabolism_at_risk_epochs must be > 0")
	}
	if p.MetabolismExpiredToPrunedEpochs == 0 {
		return fmt.Errorf("metabolism_expired_to_pruned_epochs must be > 0")
	}
	if p.MetabolismActiveThreshold == 0 {
		return fmt.Errorf("metabolism_active_threshold must be > 0")
	}
	if p.MetabolismActiveThreshold > p.MetabolismEnergyCap {
		return fmt.Errorf("metabolism_active_threshold (%d) must be <= metabolism_energy_cap (%d)", p.MetabolismActiveThreshold, p.MetabolismEnergyCap)
	}
	if p.MetabolismExtinctionThreshold == 0 {
		return fmt.Errorf("metabolism_extinction_threshold must be > 0")
	}
	if p.MetabolismExtinctionThreshold >= p.MetabolismActiveThreshold {
		return fmt.Errorf("metabolism_extinction_threshold (%d) must be < metabolism_active_threshold (%d)", p.MetabolismExtinctionThreshold, p.MetabolismActiveThreshold)
	}
	if p.MaxConfidence == 0 {
		return fmt.Errorf("max_confidence must be > 0")
	}
	if p.MaxConfidence > 1_000_000 {
		return fmt.Errorf("max_confidence must be <= 1,000,000")
	}

	// ─── Reproduction params ──────────────────────────────────────────
	if p.ReproductionRoyaltyBps > 1_000_000 {
		return fmt.Errorf("reproduction_royalty_bps must be <= 1,000,000")
	}
	if p.ReproductionRoyaltyDecayBps > 1_000_000 {
		return fmt.Errorf("reproduction_royalty_decay_bps must be <= 1,000,000")
	}
	if p.ReproductionMaxRoyaltyDepth == 0 {
		return fmt.Errorf("reproduction_max_royalty_depth must be > 0")
	}
	if p.ReproductionChildFitnessInheritanceBps > 1_000_000 {
		return fmt.Errorf("reproduction_child_fitness_inheritance_bps must be <= 1,000,000")
	}
	if p.ReproductionMaxChildren == 0 {
		return fmt.Errorf("reproduction_max_children must be > 0")
	}

	// ─── Demand params ──────────────────────────────────────────────
	if p.DemandTrackingEnabled {
		if p.DemandBountyThreshold == 0 {
			return fmt.Errorf("demand_bounty_threshold must be > 0 when demand tracking is enabled")
		}
		if p.DemandBountyBaseReward == "" || p.DemandBountyBaseReward == "0" {
			return fmt.Errorf("demand_bounty_base_reward must be > 0 when demand tracking is enabled")
		}
		if p.DemandBountyExpiryEpochs == 0 {
			return fmt.Errorf("demand_bounty_expiry_epochs must be > 0 when demand tracking is enabled")
		}
		if p.DemandMultiplierCap == 0 {
			return fmt.Errorf("demand_multiplier_cap must be > 0 when demand tracking is enabled")
		}
	}

	// ─── Competition params ─────────────────────────────────────────────
	if p.CompetitionNicheDominanceBonusBps > 1_000_000 {
		return fmt.Errorf("competition_niche_dominance_bonus_bps must be <= 1,000,000")
	}
	if p.CompetitionRedundancyThresholdBps > 1_000_000 {
		return fmt.Errorf("competition_redundancy_threshold_bps must be <= 1,000,000")
	}
	if p.CompetitionMaxNicheSize == 0 {
		return fmt.Errorf("competition_max_niche_size must be > 0")
	}
	if p.CompetitionSymbiosisBonusBps > 1_000_000 {
		return fmt.Errorf("competition_symbiosis_bonus_bps must be <= 1,000,000")
	}

	// ─── Bootstrap fund params ──────────────────────────────────────────
	if p.BootstrapFundEnabled {
		if p.BootstrapFundEpochBlocks == 0 {
			return fmt.Errorf("bootstrap_fund_epoch_blocks must be > 0 when fund is enabled")
		}
		if p.BootstrapFundMaxPerAddress == "" || p.BootstrapFundMaxPerAddress == "0" {
			return fmt.Errorf("bootstrap_fund_max_per_address must be > 0 when fund is enabled")
		}
		if p.BootstrapFundMaxPerEpoch == "" || p.BootstrapFundMaxPerEpoch == "0" {
			return fmt.Errorf("bootstrap_fund_max_per_epoch must be > 0 when fund is enabled")
		}
		if p.BootstrapFundFeeCap == "" || p.BootstrapFundFeeCap == "0" {
			return fmt.Errorf("bootstrap_fund_fee_cap must be > 0 when fund is enabled")
		}
	}

	// ─── Diversity params ──────────────────────────────────────────────
	if p.DiversityConformityAlertThreshold > 1_000_000 {
		return fmt.Errorf("diversity_conformity_alert_threshold must be <= 1,000,000")
	}
	if p.DiversityConformityAlertEpochs == 0 {
		return fmt.Errorf("diversity_conformity_alert_epochs must be > 0")
	}

	// ─── Vindication params ──────────────────────────────────────────
	if p.VindicationBonusBps > 10_000 {
		return fmt.Errorf("vindication_bonus_bps must be <= 10,000 (100%%)")
	}
	if p.VindicationSlashBps > 1_000_000 {
		return fmt.Errorf("vindication_slash_bps must be <= 1,000,000")
	}
	if p.VindicationRefundEnabled && p.VindicationWindowBlocks == 0 {
		return fmt.Errorf("vindication_window_blocks must be > 0 when vindication is enabled")
	}


	// ─── Role bonus params (R28-5) ──────────────────────────────────
	if p.HumanEmpiricalBonusBps > 1_000_000 {
		return fmt.Errorf("human_empirical_bonus_bps must be <= 1,000,000")
	}
	if p.AgentComputationalBonusBps > 1_000_000 {
		return fmt.Errorf("agent_computational_bonus_bps must be <= 1,000,000")
	}
	if p.AgentVerificationBonusBps > 1_000_000 {
		return fmt.Errorf("agent_verification_bonus_bps must be <= 1,000,000")
	}
	if p.HumanPatronageBonusBps > 1_000_000 {
		return fmt.Errorf("human_patronage_bonus_bps must be <= 1,000,000")
	}
	if p.DualValidationBonusBps > 1_000_000 {
		return fmt.Errorf("dual_validation_bonus_bps must be <= 1,000,000")
	}

	// ─── Epistemic temperature (R29-2) ──────────────────────────────
	if p.EpistemicTemperatureDecayBps > 1_000_000 {
		return fmt.Errorf("epistemic_temperature_decay_bps must be <= 1,000,000")
	}
	if p.EpistemicConformityCoolingBps > 1_000_000 {
		return fmt.Errorf("epistemic_conformity_cooling_bps must be <= 1,000,000")
	}
	if p.EpistemicVindicationHeatingBps > 1_000_000 {
		return fmt.Errorf("epistemic_vindication_heating_bps must be <= 1,000,000")
	}
	if p.EpistemicColdConfidenceCapBps > 1_000_000 {
		return fmt.Errorf("epistemic_cold_confidence_cap_bps must be <= 1,000,000")
	}
	// EpistemicHotConfidenceGrowthBps is intentionally allowed > 1,000,000 (it's a multiplier, e.g. 1,500,000 = 150%)

	// ─── Domain carrying capacity (R29-1) ──────────────────────────
	if p.DomainBaseCapacity == 0 {
		return fmt.Errorf("domain_base_capacity must be > 0")
	}
	if p.OvercrowdingDecayMultiplierBps < 1_000_000 {
		return fmt.Errorf("overcrowding_decay_multiplier_bps must be >= 1,000,000 (at least 100%%)")
	}

	// ─── Domain role elasticity (R29-3) ──────────────────────────────
	if p.RoleElasticityMinCalls == 0 {
		return fmt.Errorf("role_elasticity_min_calls must be > 0")
	}
	if p.RoleElasticityMaxMultiplierBps < 1_000_000 {
		return fmt.Errorf("role_elasticity_max_multiplier_bps must be >= 1,000,000 (at least 100%%)")
	}
	if p.RoleElasticityMinMultiplierBps > 1_000_000 {
		return fmt.Errorf("role_elasticity_min_multiplier_bps must be <= 1,000,000 (at most 100%%)")
	}
	if p.RoleElasticityMinMultiplierBps >= p.RoleElasticityMaxMultiplierBps {
		return fmt.Errorf("role_elasticity_min_multiplier_bps must be < max_multiplier_bps")
	}
	if p.RoleElasticityDecayEpochs == 0 {
		return fmt.Errorf("role_elasticity_decay_epochs must be > 0")
	}

	// ─── Cross-parameter safety (R30-2) ──────────────────────────────────
	// Carrying capacity: decay multiplier must not cause instant death.
	// At 2× capacity, one cycle drains: baseCost × multiplier / BPS.
	// That must not exceed 50% of initial energy.
	if p.OvercrowdingDecayMultiplierBps > 0 && p.MetabolismBaseCost > 0 {
		maxDecayPerCycle := p.MetabolismBaseCost * p.OvercrowdingDecayMultiplierBps / 1_000_000
		if maxDecayPerCycle > p.MetabolismInitialEnergy/2 {
			return fmt.Errorf("overcrowding_decay_multiplier too aggressive: would drain %d of %d initial energy per cycle",
				maxDecayPerCycle, p.MetabolismInitialEnergy)
		}
	}

	// Epistemic temperature: cold cap must be < max survival confidence.
	if p.EpistemicColdConfidenceCapBps >= p.MaxSurvivalConfidence {
		return fmt.Errorf("epistemic_cold_confidence_cap (%d) must be < max_survival_confidence (%d)",
			p.EpistemicColdConfidenceCapBps, p.MaxSurvivalConfidence)
	}

	// Role elasticity × agent bonus must not exceed 100% vote weight.
	if p.RoleElasticityMaxMultiplierBps*p.AgentVerificationBonusBps/1_000_000 > 1_000_000 {
		return fmt.Errorf("role_elasticity_max_multiplier * agent_verification_bonus would exceed 100%% vote weight")
	}

	// Wave 15 probe invitation parameters.
	if p.ProbeInvitationMinConfidenceBps > 1_000_000 {
		return fmt.Errorf("probe_invitation_min_confidence_bps must be <= 1,000,000")
	}

	// ─── Fee markets: congestion-based backpressure ──────────────────────────
	// CongestionMultiplierBps > 0 activates backpressure. A threshold of 0
	// would trigger it at 0 active rounds (i.e., always) — clearly not the
	// intent. Enforce that governance must supply a meaningful threshold.
	if p.CongestionMultiplierBps > 0 && p.CongestionThresholdRounds == 0 {
		return fmt.Errorf("congestion_threshold_rounds must be > 0 when congestion_multiplier_bps is set")
	}

	// ─── Fee markets: governance-adjustable revenue split + burn (Gaps 3+4) ──
	// Each component must be < 1,000,000 (BPS scale). All four together
	// (burn + the three split components) must leave room for the research fund
	// remainder — i.e. their sum must be < 1,000,000. Sum == 1,000,000 would
	// send 0 to research, violating the intent that research always receives something.
	if p.BurnBps >= 1_000_000 {
		return fmt.Errorf("burn_bps must be < 1,000,000")
	}
	if p.VerifierPoolBps >= 1_000_000 {
		return fmt.Errorf("verifier_pool_bps must be < 1,000,000")
	}
	if p.ProtocolTreasuryBps >= 1_000_000 {
		return fmt.Errorf("protocol_treasury_bps must be < 1,000,000")
	}
	if p.DevelopmentFundBps >= 1_000_000 {
		return fmt.Errorf("development_fund_bps must be < 1,000,000")
	}
	splitSum := p.BurnBps + p.VerifierPoolBps + p.ProtocolTreasuryBps + p.DevelopmentFundBps
	if splitSum >= 1_000_000 {
		return fmt.Errorf("burn_bps + verifier_pool_bps + protocol_treasury_bps + development_fund_bps (%d) must be < 1,000,000 so research fund receives a non-zero remainder", splitSum)
	}

	// ─── Priority tip cap (Gap 2) ─────────────────────────────────────────────
	// MaxPriorityTipBps caps what submitters may earmark as speed-premium tip.
	// Must be ≤ 500,000 (50% of verifier pool) — a tip that exceeds half the pool
	// would leave the base reward pathologically small for non-early committers.
	if p.MaxPriorityTipBps > 500_000 {
		return fmt.Errorf("max_priority_tip_bps must be ≤ 500,000 (50%% of verifier pool)")
	}

	// ─── Verifier independence gate (P-10a) ──────────────────────────────────
	// min_verifier_independence_bps: 0 = disabled; max meaningful value 1,000,000
	// (100% dissent — impossible in practice, but structurally valid).
	// min_verifier_independence_rounds must be ≥ 1 when the gate is enabled —
	// a rounds threshold of 0 would apply the gate to brand-new validators with
	// no history, excluding them instantly.
	if p.MinVerifierIndependenceBps > 1_000_000 {
		return fmt.Errorf("min_verifier_independence_bps must be ≤ 1,000,000")
	}
	if p.MinVerifierIndependenceBps > 0 && p.MinVerifierIndependenceRounds == 0 {
		return fmt.Errorf("min_verifier_independence_rounds must be ≥ 1 when min_verifier_independence_bps is set")
	}

	// Settlement Finality Gap 2: when min_round_duration_blocks is set, the
	// configured phase lengths must collectively satisfy the minimum. This is
	// checked at param-update time (MsgUpdateParams) so that any round created
	// from valid params automatically satisfies the minimum — no per-round check
	// needed beyond CreateVerificationRound's belt-and-suspenders guard.
	if p.MinRoundDurationBlocks > 0 {
		totalPhaseDuration := p.CommitPhaseBlocks + p.RevealPhaseBlocks + p.AggregationPhaseBlocks
		if totalPhaseDuration < p.MinRoundDurationBlocks {
			return fmt.Errorf(
				"commit_phase_blocks + reveal_phase_blocks + aggregation_phase_blocks (%d) must be ≥ min_round_duration_blocks (%d)",
				totalPhaseDuration, p.MinRoundDurationBlocks,
			)
		}
	}

	// ─── Aggregate extremizing alpha (Calibrated Forecasting Gap 5) ─────────────
	// 0 = disabled. When enabled, α must be ≥ 1.0 (stored as milliunits: 1000).
	// α < 1.0 would compress rather than extremize — compressing toward 50% is
	// the opposite of what we want and is disallowed. Upper bound: 10000 (α=10)
	// is structurally valid (approaches binary) but unlikely in practice.
	if p.AggregationExtremizingAlpha > 0 && p.AggregationExtremizingAlpha < 1000 {
		return fmt.Errorf("aggregation_extremizing_alpha must be 0 (disabled) or ≥ 1000 (α≥1.0); got %d", p.AggregationExtremizingAlpha)
	}
	if p.AggregationExtremizingAlpha > 10_000 {
		return fmt.Errorf("aggregation_extremizing_alpha must be ≤ 10000 (α≤10.0); got %d", p.AggregationExtremizingAlpha)
	}

	// ─── Pre-registration discount (Scientific Method Gap 3) ─────────────────
	// 0 = disabled. Max 500_000 (50%) — discount above 50% would make the
	// effective minimum negligible and could be gamed for mass pre-registration.
	if p.PreRegistrationDiscountBps > 500_000 {
		return fmt.Errorf("pre_registration_discount_bps must be ≤ 500000 (50%%); got %d", p.PreRegistrationDiscountBps)
	}

	// ─── Phenomenological corroboration confidence threshold (Phenomenology Gap 3) ──
	// 0 = disabled. When non-zero, must be a valid BPS value (1–1,000,000).
	// Values above MaxConfidence (880k) would never fire since verdicts are
	// capped at MaxConfidence — warn rather than error to allow forward-compatible
	// governance proposals that lower MaxConfidence later.
	if p.PhenomenologicalCorroborationConfidenceBps > 1_000_000 {
		return fmt.Errorf("phenomenological_corroboration_confidence_bps must be ≤ 1000000; got %d", p.PhenomenologicalCorroborationConfidenceBps)
	}

	// Oracle freshness window is a block count — any uint64 is valid (0 = disabled).
	// No upper bound enforced; governance is responsible for reasonable values.
	// (Oracle Resilience Brief Gap 4)
	_ = p.OracleFreshnessWindowBlocks // already uint64, always valid

	// Formal Logic Brief Gap 4: transitive relation verbs must not contain
	// empty strings (an empty string would match every relation verb).
	for i, v := range p.TransitiveRelationVerbs {
		if v == "" {
			return fmt.Errorf("transitive_relation_verbs[%d] must not be empty", i)
		}
	}

	return nil
}
