package types

import "encoding/binary"

const (
	// ModuleName is the name of the knowledge module.
	ModuleName = "knowledge"
	// StoreKey is the store key for the knowledge module.
	StoreKey = ModuleName
	// MemStoreKey is the in-memory store key.
	MemStoreKey = "mem_knowledge"
	// PortID is the IBC port ID.
	PortID = "knowledge"
	// Version is the IBC channel version.
	Version = "zrn-knowledge-1"
	// RouterKey is the message routing key.
	RouterKey = ModuleName
	// BootstrapFundModuleName is the module account that holds the knowledge bootstrap fund.
	BootstrapFundModuleName = "knowledge_bootstrap_fund"
	// TrainingFundModuleName is the module account that holds augmentation
	// escrow, training fund disbursements (with vesting), and research fund
	// forfeitures collected under Route B Wave 4.
	TrainingFundModuleName = "knowledge_training_fund"
	// ProbeBountyPoolModuleName is the module account that holds the
	// probe-bounty stream introduced in Wave 15. Each block the chain
	// mints a small amount into this pool (capped); successful-challenge
	// bonuses draw from it before falling back to the protocol treasury.
	// Purpose-built funding stream for continuous epistemic auditing —
	// decouples probe rewards from general governance.
	ProbeBountyPoolModuleName = "knowledge_probe_bounty_pool"
)

// Store key prefixes — one byte per sub-namespace.
// KV prefix ranges for knowledge module state.
var (
	// ─── Core state ──────────────────────────────────────────────────────────
	FactKeyPrefix              = []byte{0x01} // factID → Fact
	ClaimKeyPrefix             = []byte{0x02} // claimID → Claim
	VerificationRoundKeyPrefix = []byte{0x03} // roundID → VerificationRound
	FactReferenceKeyPrefix     = []byte{0x04} // factID:refID → exists
	DomainFactIndexPrefix      = []byte{0x05} // domain/factID → exists
	ParamsKey                  = []byte{0x06} // singleton Params
	DomainKeyPrefix            = []byte{0x07} // domainName → Domain
	IncomingRefIndexPrefix     = []byte{0x08} // toFactID:fromFactID → exists
	ContentHashIndexPrefix     = []byte{0x09} // contentHash → claimID (dedup)
	ClaimRoundIndexPrefix      = []byte{0x0a} // claimID → roundID
	EquivocationKeyPrefix      = []byte{0x0b} // roundID:validator → evidence

	// ─── Adversarial verification ────────────────────────────────────────────
	ProvisionalChallengeKeyPrefix = []byte{0x0c}
	ChallengerCooldownKeyPrefix   = []byte{0x0d}
	PendingEvalClaimIndexPrefix   = []byte{0x0e}
	SubmitterCalibrationPrefix    = []byte{0x0f}

	// ─── Negative knowledge ──────────────────────────────────────────────────
	CounterFactKeyPrefix             = []byte{0x10}
	CounterFactByFactIndexPrefix     = []byte{0x11}
	CounterFactByDomainIndexPrefix   = []byte{0x12}
	CounterFactByHeightIndexPrefix   = []byte{0x13}
	FactNegationLinkPrefix           = []byte{0x14}
	ContradictionCooldownPrefix      = []byte{0x15}
	FalsificationEpochPaidPrefix     = []byte{0x16}
	FalsificationCarryForwardPrefix  = []byte{0x17}
	CounterFactChallengeKeyPrefix    = []byte{0x18}
	CounterFactChallengeWindowPrefix = []byte{0x19}
	ExtendedParamsKey                = []byte{0x1a} // singleton JSON ExtendedParams
	PatronageRecordPrefix            = []byte{0x1b}
	PruningQueuePrefix               = []byte{0x1c}
	VerifierConformityPrefix         = []byte{0x1d} // FARM-1
	ValidatorParticipationPrefix     = []byte{0x1e} // FARM-8

	// ─── Secondary query indexes ─────────────────────────────────────────────
	FactBySubmitterIndexPrefix = []byte{0x1f} // submitter/factID → exists
	FactByDomainIndexPrefix    = []byte{0x20} // domain/factID → exists (mirror of 0x05)
	ActiveRoundIndexPrefix     = []byte{0x21} // roundID → exists

	// ─── Citation and domain strata ──────────────────────────────────────────
	CitationSourcePrefix = []byte{0x27} // FARM-11 citation-source tracking
	DomainStratumPrefix  = []byte{0x28} // FARM-12 domain-to-stratum mapping

	// ─── Research fund governance ────────────────────────────────────────────
	ResearchProposalPrefix  = []byte{0x29}
	ResearchVotePrefix      = []byte{0x2a}
	ResearchFundStatsPrefix = []byte{0x2b}

	// ─── Partnership citation stats ──────────────────────────────────────────
	PartnershipCitationStatsPrefix = []byte{0x2c}

	// ─── Semantic relations (knowledge graph edges) ──────────────────────────
	FactRelationPrefix        = []byte{0x30} // 0x30 | source_fact_id / target_fact_id → FactRelation
	FactRelationReversePrefix = []byte{0x31} // 0x31 | target_fact_id / source_fact_id → FactRelation (reverse index)

	// ─── Structured claim indexes ────────────────────────────────────────────
	FactSubjectPrefix = []byte{0x32} // 0x32 | domain / subject_hash → fact_id
	FactTagPrefix     = []byte{0x33} // 0x33 | tag / fact_id → []byte{1}

	// ─── Canonical form dedup ────────────────────────────────────────────────
	CanonicalHashPrefix = []byte{0x34} // 0x34 | canonical_hash → claim_id/fact_id

	// ─── Bootstrap fund tracking ─────────────────────────────────────────────
	BootstrapClaimCountPrefix = []byte{0x35} // 0x35 | address → uint64 (lifetime sponsored count)
	BootstrapEpochCountPrefix = []byte{0x36} // 0x36 | epoch_number → uint64 (epoch-wide count)

	// ─── Metabolism tracking ────────────────────────────────────────────────
	NewCitationsEpochPrefix = []byte{0x37} // 0x37 | fact_id → uint64 (new citations this epoch)

	// ─── Novelty detection ──────────────────────────────────────────────────
	CommonKnowledgePrefix = []byte{0x38} // 0x38 | domain / subject_hash → CommonKnowledgeEntry

	// ─── Agent demand tracking ─────────────────────────────────────────
	DemandSignalPrefix          = []byte{0x39} // domain / subject_hash → DemandSignal
	BountyPrefix                = []byte{0x3a} // bounty_id → KnowledgeBounty
	BountyByDomainSubjectPrefix = []byte{0x3b} // domain / subject_hash → bounty_id (active index)

	// ─── Niche competition ──────────────────────────────────────────────
	NicheIndexPrefix   = []byte{0x3c} // 0x3c | niche_key | fact_id → []byte{1}
	NicheMembersPrefix = []byte{0x3d} // 0x3d | niche_key → []byte{1} (niche existence)

	// ─── Query satisfaction ─────────────────────────────────────────────
	QueryReceiptPrefix = []byte{0x3e} // 0x3e | rater / fact_id → block height (query receipt)

	// ─── Consensus diversity (R28-2) ────────────────────────────────────
	RoundDiversityPrefix         = []byte{0x40} // 0x40 | roundID → RoundDiversity (JSON)
	DomainDiversityPrefix        = []byte{0x41} // 0x41 | domain / epoch_bytes → DomainDiversityScore (JSON)
	ValidatorIndependencePrefix  = []byte{0x42} // 0x42 | validatorAddr → ValidatorIndependence (JSON)
	ConformityStreakPrefix       = []byte{0x43} // 0x43 | domain → ConformityStreak (JSON)
	DomainEpochRoundIndexPrefix = []byte{0x44} // 0x44 | domain / epoch_bytes / roundID → 0x01

	// ─── Retroactive vindication (R28-1) ────────────────────────────────
	VindicationPendingPrefix = []byte{0x50} // 0x50 | factID → []VindicationEntry (JSON)
	VindicationRecordPrefix  = []byte{0x51} // 0x51 | factID / verifier → VindicationRecord (JSON)

	// ─── Capture defense overrides (R28-8) ──────────────────────────────
	VerificationThresholdOverrideKeyPrefix = []byte{0x52} // 0x52 | domain → threshold override (binary)

	// ─── Epistemic temperature (R29-2) ─────────────────────────────────
	EpistemicStatePrefix = []byte{0x53} // 0x53 | domain → DomainEpistemicState (JSON)

	// ─── Domain carrying capacity (R29-1) ──────────────────────────────
	DomainStatsPrefix = []byte{0x54} // 0x54 | domain → DomainStats (JSON)

	// ─── Domain role elasticity (R29-3) ────────────────────────────────
	DomainRoleRecordPrefix = []byte{0x55} // 0x55 | domain → DomainRoleRecord (JSON)

	// ─── Adaptive pacing (R29-6) ───────────────────────────────────────
	LastClaimHeightKeyPrefix = []byte{0x56} // 0x56 | submitter → uint64 (last claim block height)

	// ─── Completion index (R31-2: Fire activity metrics) ──────────────
	CompletedRoundIndexPrefix = []byte{0x57} // 0x57 | verdictBlock(8) | roundID → CompletedRoundMeta (proto)

	// ─── Methodology registry (Phase 1: methodology over statement) ──
	MethodologyKeyPrefix = []byte{0x58} // 0x58 | methodID → Methodology (proto)

	// ─── Normative commitment registry (Phase 6: is-ought wall) ──────
	NormativeCommitmentKeyPrefix = []byte{0x59} // 0x59 | commitmentID → NormativeCommitment (proto)

	// ─── Agent calibration (Phase 5: feedback loop) ──────────────────
	AgentCalibrationKeyPrefix = []byte{0x5A} // 0x5A | address → AgentCalibration (proto)

	// ─── Route B: model training infrastructure ──────────────────────
	TokenizerSpecKey               = []byte{0x5B} // singleton: current TokenizerSpec
	TokenizerSpecHistoryKeyPrefix  = []byte{0x5C} // 0x5C | be64(version) → TokenizerSpec (historical)
	TrainingPipelineKeyPrefix      = []byte{0x5D} // 0x5D | pipelineID → TrainingPipeline
	ModelCardKeyPrefix             = []byte{0x5E} // 0x5E | modelID → ModelCard
	TrainingAttestationKeyPrefix   = []byte{0x5F} // 0x5F | pipelineID → TrainingAttestation
	ContributionByModelKeyPrefix   = []byte{0x60} // 0x60 | modelID → ContributionRecord
	ContributionByFactKeyPrefix    = []byte{0x61} // 0x61 | factID | modelID → 1 byte marker
	AugmentationBountyKeyPrefix    = []byte{0x62} // 0x62 | bountyID → AugmentationBounty
	AugmentationKeyPrefix          = []byte{0x63} // 0x63 | augID → Augmentation
	AugmentationByFactKeyPrefix    = []byte{0x64} // 0x64 | factID | augID → marker
	AugmentationByBountyKeyPrefix  = []byte{0x65} // 0x65 | bountyID | augID → marker

	// ─── Route B Wave 4: economic realignment ──────────────────────────
	ContributionChallengeKeyPrefix          = []byte{0x66} // 0x66 | challenge_id → ContributionChallenge
	ContributionChallengeByModelKeyPrefix   = []byte{0x67} // 0x67 | model_id | challenge_id → 1 (reverse index)
	OpenContributionChallengeKeyPrefix      = []byte{0x68} // 0x68 | challenge_id → 1 (open-only set)
	TrainingFundDisbursementKeyPrefix       = []byte{0x69} // 0x69 | disbursement_id → TrainingFundDisbursement
	TrainingFundDisbursementByModelPrefix   = []byte{0x6A} // 0x6A | model_id | disbursement_id → 1
	TrainingFundEscrowLockedKeyPrefix       = []byte{0x6B} // 0x6B | bounty_id → uzrn string (redundant bookkeeping for fast totals)
	TrainingFundVestingKeyPrefix            = []byte{0x6C} // 0x6C | disbursement_id → uzrn string (redundant)

	// ─── Route B Wave 5: unified training data format ─────────────────
	TraceSchemaKey                = []byte{0x6D} // singleton: current TraceSchema
	TraceSchemaHistoryKeyPrefix   = []byte{0x6E} // 0x6E | be64(version) → TraceSchema (historical)

	// ─── Route B Wave 7: training-run manifests ──────────────────────
	TrainingManifestKeyPrefix            = []byte{0x6F} // 0x6F | manifest_id → TrainingManifest
	TrainingManifestByPipelineKeyPrefix  = []byte{0x70} // 0x70 | pipeline_id | manifest_id → 1 (reverse index)
	TrainingManifestByCreatorKeyPrefix   = []byte{0x71} // 0x71 | creator | manifest_id → 1 (reverse index)
	TrainingManifestByStatusKeyPrefix    = []byte{0x72} // 0x72 | be8(status) | manifest_id → 1 (reverse index)

	// ─── Route B Wave 11: incident response ──────────────────────────
	IncidentRecordKeyPrefix         = []byte{0x73} // 0x73 | incident_id → IncidentRecord
	IncidentBySeverityKeyPrefix     = []byte{0x74} // 0x74 | be8(severity) | incident_id → 1
	IncidentByStatusKeyPrefix       = []byte{0x75} // 0x75 | be8(status) | incident_id → 1
	OpenIncidentKeyPrefix           = []byte{0x76} // 0x76 | incident_id → 1 (open-only set)

	// ─── Route B Wave 12: circuit breakers ───────────────────────────
	ModulePauseKeyPrefix            = []byte{0x77} // 0x77 | module_name → ModulePause

	// ─── Wave 14: privileged-action audit log ────────────────────────
	PrivilegedActionKeyPrefix       = []byte{0x78} // 0x78 | be64(seq) → PrivilegedAction
	PrivilegedActionSeqKey          = []byte{0x79} // singleton: next-seq counter (uvarint)

	// ─── Scientific Method Gap 3: pre-registration ───────────────────
	PreRegistrationKeyPrefix         = []byte{0x7A} // 0x7A | preRegID → PreRegistration (proto)
	PreRegistrationExpiryIndexPrefix = []byte{0x7B} // 0x7B | be64(expiryBlock) | preRegID → 1

	// ─── Wave 16: guardian-veto pending fact-injection queue ────────
	PendingFactInjectionKeyPrefix          = []byte{0x7A} // 0x7A | id → PendingFactInjection
	PendingFactInjectionByExecuteKeyPrefix = []byte{0x7B} // 0x7B | be64(execute_at) | id → 1 (time index for BeginBlocker scan)

	// ─── Fee markets: congestion counter ────────────────────────────
	ActiveRoundsCountKey = []byte{0x7C} // singleton: number of currently active verification rounds (uint64, big-endian)
)

// PendingFactInjectionKey returns the store key for a pending injection.
func PendingFactInjectionKey(id string) []byte {
	return append(append([]byte{}, PendingFactInjectionKeyPrefix...), []byte(id)...)
}

// PendingFactInjectionByExecuteKey returns the time-index key for a
// pending injection. Sorted by execute_at_block (big-endian) so the
// BeginBlocker can scan-and-stop at the first record whose deadline
// has not yet passed.
func PendingFactInjectionByExecuteKey(executeAt uint64, id string) []byte {
	out := append([]byte{}, PendingFactInjectionByExecuteKeyPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, executeAt)
	out = append(out, buf...)
	out = append(out, []byte(id)...)
	return out
}

// MethodologyKey returns the store key for a methodology by ID.
func MethodologyKey(id string) []byte {
	return append(append([]byte{}, MethodologyKeyPrefix...), []byte(id)...)
}

// NormativeCommitmentKey returns the store key for a normative commitment.
func NormativeCommitmentKey(id string) []byte {
	return append(append([]byte{}, NormativeCommitmentKeyPrefix...), []byte(id)...)
}

// AgentCalibrationKey returns the store key for a submitter's calibration record.
func AgentCalibrationKey(addr string) []byte {
	return append(append([]byte{}, AgentCalibrationKeyPrefix...), []byte(addr)...)
}

// TokenizerSpecHistoryKey returns the store key for a historical tokenizer spec.
func TokenizerSpecHistoryKey(version uint64) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, TokenizerSpecHistoryKeyPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, version)
	return append(key, buf...)
}

// TrainingPipelineKey returns the store key for a training pipeline record.
func TrainingPipelineKey(id string) []byte {
	return append(append([]byte{}, TrainingPipelineKeyPrefix...), []byte(id)...)
}

// ModelCardKey returns the store key for a model card record.
func ModelCardKey(id string) []byte {
	return append(append([]byte{}, ModelCardKeyPrefix...), []byte(id)...)
}

// TrainingAttestationKey returns the store key for a training attestation.
func TrainingAttestationKey(pipelineID string) []byte {
	return append(append([]byte{}, TrainingAttestationKeyPrefix...), []byte(pipelineID)...)
}

// ContributionByModelKey returns the store key for a model's contribution record.
func ContributionByModelKey(modelID string) []byte {
	return append(append([]byte{}, ContributionByModelKeyPrefix...), []byte(modelID)...)
}

// ContributionByFactKey returns the reverse-index key for fact → model.
func ContributionByFactKey(factID, modelID string) []byte {
	out := append([]byte{}, ContributionByFactKeyPrefix...)
	out = append(out, []byte(factID)...)
	out = append(out, 0) // separator
	out = append(out, []byte(modelID)...)
	return out
}

// ContributionByFactPrefix returns the iteration prefix for all models that used factID.
func ContributionByFactPrefix(factID string) []byte {
	out := append([]byte{}, ContributionByFactKeyPrefix...)
	out = append(out, []byte(factID)...)
	out = append(out, 0)
	return out
}

// AugmentationBountyKey returns the store key for an augmentation bounty.
func AugmentationBountyKey(id string) []byte {
	return append(append([]byte{}, AugmentationBountyKeyPrefix...), []byte(id)...)
}

// AugmentationKey returns the store key for an augmentation.
func AugmentationKey(id string) []byte {
	return append(append([]byte{}, AugmentationKeyPrefix...), []byte(id)...)
}

// AugmentationByFactKey returns the reverse-index key for fact → augmentation.
func AugmentationByFactKey(factID, augID string) []byte {
	out := append([]byte{}, AugmentationByFactKeyPrefix...)
	out = append(out, []byte(factID)...)
	out = append(out, 0)
	out = append(out, []byte(augID)...)
	return out
}

// AugmentationByFactPrefix returns the iteration prefix for all augmentations of a fact.
func AugmentationByFactPrefix(factID string) []byte {
	out := append([]byte{}, AugmentationByFactKeyPrefix...)
	out = append(out, []byte(factID)...)
	out = append(out, 0)
	return out
}

// AugmentationByBountyKey returns the reverse-index key for bounty → augmentation.
func AugmentationByBountyKey(bountyID, augID string) []byte {
	out := append([]byte{}, AugmentationByBountyKeyPrefix...)
	out = append(out, []byte(bountyID)...)
	out = append(out, 0)
	out = append(out, []byte(augID)...)
	return out
}

// AugmentationByBountyPrefix returns the iteration prefix for all augmentations of a bounty.
func AugmentationByBountyPrefix(bountyID string) []byte {
	out := append([]byte{}, AugmentationByBountyKeyPrefix...)
	out = append(out, []byte(bountyID)...)
	out = append(out, 0)
	return out
}

// ─── Key constructors ─────────────────────────────────────────────────────────

// FactKey returns the store key for a fact.
func FactKey(id string) []byte {
	return append(append([]byte{}, FactKeyPrefix...), []byte(id)...)
}

// ClaimKey returns the store key for a claim.
func ClaimKey(id string) []byte {
	return append(append([]byte{}, ClaimKeyPrefix...), []byte(id)...)
}

// RoundKey returns the store key for a verification round.
func RoundKey(id string) []byte {
	return append(append([]byte{}, VerificationRoundKeyPrefix...), []byte(id)...)
}

// DomainKey returns the store key for a domain.
func DomainKey(name string) []byte {
	return append(append([]byte{}, DomainKeyPrefix...), []byte(name)...)
}

// ContentHashKey returns the index key for a content hash.
func ContentHashKey(hash string) []byte {
	return append(append([]byte{}, ContentHashIndexPrefix...), []byte(hash)...)
}

// ClaimRoundIndexKey returns the index key mapping a claim to its round.
func ClaimRoundIndexKey(claimID string) []byte {
	return append(append([]byte{}, ClaimRoundIndexPrefix...), []byte(claimID)...)
}

// FactBySubmitterKey returns the composite index key for facts by submitter.
func FactBySubmitterKey(submitter, factID string) []byte {
	key := append(append([]byte{}, FactBySubmitterIndexPrefix...), []byte(submitter)...)
	key = append(key, '/')
	return append(key, []byte(factID)...)
}

// FactByDomainKey returns the composite index key for facts by domain.
func FactByDomainKey(domain, factID string) []byte {
	key := append(append([]byte{}, DomainFactIndexPrefix...), []byte(domain)...)
	key = append(key, '/')
	return append(key, []byte(factID)...)
}

// FactRelationKey returns the forward index key for a fact relation.
func FactRelationKey(sourceFactID, targetFactID string) []byte {
	key := append(append([]byte{}, FactRelationPrefix...), []byte(sourceFactID)...)
	key = append(key, '/')
	return append(key, []byte(targetFactID)...)
}

// FactRelationReverseKey returns the reverse index key for a fact relation.
func FactRelationReverseKey(targetFactID, sourceFactID string) []byte {
	key := append(append([]byte{}, FactRelationReversePrefix...), []byte(targetFactID)...)
	key = append(key, '/')
	return append(key, []byte(sourceFactID)...)
}

// FactRelationsBySourcePrefix returns the prefix for iterating all relations from a source fact.
func FactRelationsBySourcePrefix(sourceFactID string) []byte {
	key := append(append([]byte{}, FactRelationPrefix...), []byte(sourceFactID)...)
	return append(key, '/')
}

// FactRelationsByTargetPrefix returns the prefix for iterating all relations to a target fact.
func FactRelationsByTargetPrefix(targetFactID string) []byte {
	key := append(append([]byte{}, FactRelationReversePrefix...), []byte(targetFactID)...)
	return append(key, '/')
}

// FactSubjectKey returns the index key for a fact's subject within a domain.
// Subject is stored as a SHA-256 hash to normalize and bound key length.
func FactSubjectKey(domain, subjectHash string) []byte {
	key := append(append([]byte{}, FactSubjectPrefix...), []byte(domain)...)
	key = append(key, '/')
	return append(key, []byte(subjectHash)...)
}

// FactTagKey returns the index key for a fact tagged with a given tag.
func FactTagKey(tag, factID string) []byte {
	key := append(append([]byte{}, FactTagPrefix...), []byte(tag)...)
	key = append(key, '/')
	return append(key, []byte(factID)...)
}

// FactTagsByTagPrefix returns the prefix for iterating all facts with a given tag.
func FactTagsByTagPrefix(tag string) []byte {
	key := append(append([]byte{}, FactTagPrefix...), []byte(tag)...)
	return append(key, '/')
}

// CanonicalHashKey returns the index key for a canonical form hash.
func CanonicalHashKey(hash string) []byte {
	return append(append([]byte{}, CanonicalHashPrefix...), []byte(hash)...)
}

// CommonKnowledgeKey returns the store key for a common knowledge entry by domain and subject hash.
func CommonKnowledgeKey(domain, subjectHash string) []byte {
	key := append(append([]byte{}, CommonKnowledgePrefix...), []byte(domain)...)
	key = append(key, '/')
	return append(key, []byte(subjectHash)...)
}

// CommonKnowledgeByDomainPrefix returns the prefix for iterating all common knowledge entries in a domain.
func CommonKnowledgeByDomainPrefix(domain string) []byte {
	key := append(append([]byte{}, CommonKnowledgePrefix...), []byte(domain)...)
	return append(key, '/')
}

// DemandSignalKey returns the store key for a demand signal.
func DemandSignalKey(domain, subjectHash string) []byte {
	key := append(append([]byte{}, DemandSignalPrefix...), []byte(domain)...)
	key = append(key, '/')
	return append(key, []byte(subjectHash)...)
}

// BountyKey returns the store key for a bounty.
func BountyKey(id string) []byte {
	return append(append([]byte{}, BountyPrefix...), []byte(id)...)
}

// BountyByDomainSubjectKey returns the index key for active bounties by domain/subject.
func BountyByDomainSubjectKey(domain, subjectHash string) []byte {
	key := append(append([]byte{}, BountyByDomainSubjectPrefix...), []byte(domain)...)
	key = append(key, '/')
	return append(key, []byte(subjectHash)...)
}

// NicheIndexKey returns the composite index key for a fact within a niche.
func NicheIndexKey(nicheKey, factID string) []byte {
	key := append(append([]byte{}, NicheIndexPrefix...), []byte(nicheKey)...)
	key = append(key, '/')
	return append(key, []byte(factID)...)
}

// NicheIndexByNichePrefix returns the prefix for iterating all facts in a niche.
func NicheIndexByNichePrefix(nicheKey string) []byte {
	key := append(append([]byte{}, NicheIndexPrefix...), []byte(nicheKey)...)
	return append(key, '/')
}

// NicheMembersKey returns the key for registering a niche's existence.
func NicheMembersKey(nicheKey string) []byte {
	return append(append([]byte{}, NicheMembersPrefix...), []byte(nicheKey)...)
}

// QueryReceiptKey returns the key for a query receipt: 0x3e | rater / factID.
func QueryReceiptKey(rater, factID string) []byte {
	key := append(append([]byte{}, QueryReceiptPrefix...), []byte(rater)...)
	key = append(key, '/')
	return append(key, []byte(factID)...)
}

// RoundDiversityKey returns the store key for a round's diversity data.
func RoundDiversityKey(roundID string) []byte {
	return append(append([]byte{}, RoundDiversityPrefix...), []byte(roundID)...)
}

// DomainDiversityKey returns the store key for a domain's epoch diversity score.
func DomainDiversityKey(domain string, epoch uint64) []byte {
	key := append(append([]byte{}, DomainDiversityPrefix...), []byte(domain)...)
	key = append(key, '/')
	epochBz := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBz, epoch)
	return append(key, epochBz...)
}

// DomainDiversityByDomainPrefix returns the prefix for iterating all epochs for a domain.
func DomainDiversityByDomainPrefix(domain string) []byte {
	key := append(append([]byte{}, DomainDiversityPrefix...), []byte(domain)...)
	return append(key, '/')
}

// ValidatorIndependenceKey returns the store key for a validator's independence score.
func ValidatorIndependenceKey(validator string) []byte {
	return append(append([]byte{}, ValidatorIndependencePrefix...), []byte(validator)...)
}

// ConformityStreakKey returns the store key for a domain's conformity streak.
func ConformityStreakKey(domain string) []byte {
	return append(append([]byte{}, ConformityStreakPrefix...), []byte(domain)...)
}

// DomainEpochRoundKey returns the index key for a round in a domain's epoch.
func DomainEpochRoundKey(domain string, epoch uint64, roundID string) []byte {
	key := append(append([]byte{}, DomainEpochRoundIndexPrefix...), []byte(domain)...)
	key = append(key, '/')
	epochBz := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBz, epoch)
	key = append(key, epochBz...)
	key = append(key, '/')
	return append(key, []byte(roundID)...)
}

// DomainEpochRoundPrefix returns the prefix for iterating all rounds in a domain's epoch.
func DomainEpochRoundPrefix(domain string, epoch uint64) []byte {
	key := append(append([]byte{}, DomainEpochRoundIndexPrefix...), []byte(domain)...)
	key = append(key, '/')
	epochBz := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBz, epoch)
	key = append(key, epochBz...)
	return append(key, '/')
}

// VindicationPendingKey returns the store key for pending vindications for a fact.
func VindicationPendingKey(factId string) []byte {
	return append(append([]byte{}, VindicationPendingPrefix...), []byte(factId)...)
}

// VindicationRecordKey returns the store key for a vindication record.
func VindicationRecordKey(factId, verifier string) []byte {
	key := append([]byte{}, VindicationRecordPrefix...)
	key = append(key, []byte(factId)...)
	key = append(key, '/')
	key = append(key, []byte(verifier)...)
	return key
}

// VindicationRecordPrefixForFact returns the prefix for iterating all records for a fact.
func VindicationRecordPrefixForFact(factId string) []byte {
	key := append([]byte{}, VindicationRecordPrefix...)
	key = append(key, []byte(factId)...)
	key = append(key, '/')
	return key
}

// EpistemicStateKey returns the store key for a domain's epistemic state.
func EpistemicStateKey(domain string) []byte {
	return append(append([]byte{}, EpistemicStatePrefix...), []byte(domain)...)
}

// DomainStatsKey returns the store key for a domain's population stats.
func DomainStatsKey(domain string) []byte {
	return append(append([]byte{}, DomainStatsPrefix...), []byte(domain)...)
}

// DomainRoleRecordKey returns the store key for a domain's role track record.
func DomainRoleRecordKey(domain string) []byte {
	return append(append([]byte{}, DomainRoleRecordPrefix...), []byte(domain)...)
}

// LastClaimHeightKey returns the store key for a submitter's last claim height.
func LastClaimHeightKey(submitter string) []byte {
	return append(append([]byte{}, LastClaimHeightKeyPrefix...), []byte(submitter)...)
}

// CompletedRoundKey returns the index key for a completed round by verdict block.
func CompletedRoundKey(verdictBlock uint64, roundID string) []byte {
	key := make([]byte, 0, 1+8+len(roundID))
	key = append(key, CompletedRoundIndexPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, verdictBlock)
	key = append(key, buf...)
	key = append(key, []byte(roundID)...)
	return key
}

// CompletedRoundBlockPrefix returns the prefix for iterating completed rounds starting at a block.
// Use with start=CompletedRoundBlockPrefix(startBlock) and end=CompletedRoundBlockPrefix(endBlock+1).
func CompletedRoundBlockPrefix(block uint64) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, CompletedRoundIndexPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, block)
	key = append(key, buf...)
	return key
}

// ─── Route B Wave 4: economic realignment key constructors ────────────────

// ContributionChallengeKey returns the store key for a challenge by id.
func ContributionChallengeKey(id string) []byte {
	return append(append([]byte{}, ContributionChallengeKeyPrefix...), []byte(id)...)
}

// ContributionChallengeByModelKey returns the (model, challenge) reverse key.
func ContributionChallengeByModelKey(modelID, challengeID string) []byte {
	out := append([]byte{}, ContributionChallengeByModelKeyPrefix...)
	out = append(out, []byte(modelID)...)
	out = append(out, 0)
	out = append(out, []byte(challengeID)...)
	return out
}

// ContributionChallengeByModelPrefix returns the iteration prefix for a model.
func ContributionChallengeByModelPrefix(modelID string) []byte {
	out := append([]byte{}, ContributionChallengeByModelKeyPrefix...)
	out = append(out, []byte(modelID)...)
	out = append(out, 0)
	return out
}

// OpenContributionChallengeKey returns the set key marking an open challenge.
func OpenContributionChallengeKey(id string) []byte {
	return append(append([]byte{}, OpenContributionChallengeKeyPrefix...), []byte(id)...)
}

// TrainingFundDisbursementKey returns the store key for a disbursement by id.
func TrainingFundDisbursementKey(id string) []byte {
	return append(append([]byte{}, TrainingFundDisbursementKeyPrefix...), []byte(id)...)
}

// TrainingFundDisbursementByModelKey is the reverse index.
func TrainingFundDisbursementByModelKey(modelID, disbursementID string) []byte {
	out := append([]byte{}, TrainingFundDisbursementByModelPrefix...)
	out = append(out, []byte(modelID)...)
	out = append(out, 0)
	out = append(out, []byte(disbursementID)...)
	return out
}

// TrainingFundEscrowLockedKey returns the bookkeeping key for an active escrow.
func TrainingFundEscrowLockedKey(bountyID string) []byte {
	return append(append([]byte{}, TrainingFundEscrowLockedKeyPrefix...), []byte(bountyID)...)
}

// TrainingFundVestingKey returns the bookkeeping key for a vesting amount.
func TrainingFundVestingKey(disbursementID string) []byte {
	return append(append([]byte{}, TrainingFundVestingKeyPrefix...), []byte(disbursementID)...)
}

// TraceSchemaHistoryKey returns the store key for a historical trace schema.
func TraceSchemaHistoryKey(version uint64) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, TraceSchemaHistoryKeyPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, version)
	return append(key, buf...)
}

// ─── Route B Wave 7: training-manifest key constructors ───────────────────

// TrainingManifestKey returns the store key for a manifest by id.
func TrainingManifestKey(id string) []byte {
	return append(append([]byte{}, TrainingManifestKeyPrefix...), []byte(id)...)
}

// TrainingManifestByPipelineKey returns the (pipeline, manifest) reverse key.
func TrainingManifestByPipelineKey(pipelineID, manifestID string) []byte {
	out := append([]byte{}, TrainingManifestByPipelineKeyPrefix...)
	out = append(out, []byte(pipelineID)...)
	out = append(out, 0)
	out = append(out, []byte(manifestID)...)
	return out
}

// TrainingManifestByPipelinePrefix returns the iteration prefix for a pipeline.
func TrainingManifestByPipelinePrefix(pipelineID string) []byte {
	out := append([]byte{}, TrainingManifestByPipelineKeyPrefix...)
	out = append(out, []byte(pipelineID)...)
	out = append(out, 0)
	return out
}

// TrainingManifestByCreatorKey returns the (creator, manifest) reverse key.
func TrainingManifestByCreatorKey(creator, manifestID string) []byte {
	out := append([]byte{}, TrainingManifestByCreatorKeyPrefix...)
	out = append(out, []byte(creator)...)
	out = append(out, 0)
	out = append(out, []byte(manifestID)...)
	return out
}

// TrainingManifestByStatusKey returns the (status, manifest) reverse key.
// status is encoded as a single byte.
func TrainingManifestByStatusKey(status byte, manifestID string) []byte {
	out := append([]byte{}, TrainingManifestByStatusKeyPrefix...)
	out = append(out, status)
	out = append(out, []byte(manifestID)...)
	return out
}

// ─── Route B Wave 11: incident response key constructors ──────────────────

// IncidentRecordKey returns the store key for an incident by id.
func IncidentRecordKey(id string) []byte {
	return append(append([]byte{}, IncidentRecordKeyPrefix...), []byte(id)...)
}

// IncidentBySeverityKey returns the (severity, incident_id) reverse key.
func IncidentBySeverityKey(severity byte, incidentID string) []byte {
	out := append([]byte{}, IncidentBySeverityKeyPrefix...)
	out = append(out, severity)
	out = append(out, []byte(incidentID)...)
	return out
}

// IncidentByStatusKey returns the (status, incident_id) reverse key.
func IncidentByStatusKey(status byte, incidentID string) []byte {
	out := append([]byte{}, IncidentByStatusKeyPrefix...)
	out = append(out, status)
	out = append(out, []byte(incidentID)...)
	return out
}

// OpenIncidentKey returns the set key marking an incident as live (OPEN or MITIGATING).
func OpenIncidentKey(id string) []byte {
	return append(append([]byte{}, OpenIncidentKeyPrefix...), []byte(id)...)
}

// ─── Route B Wave 12: module circuit breaker key ─────────────────────────

// ModulePauseKey returns the store key for a module's pause marker.
// Absence of the record == module is not paused.
func ModulePauseKey(moduleName string) []byte {
	return append(append([]byte{}, ModulePauseKeyPrefix...), []byte(moduleName)...)
}

// ─── Wave 14: privileged action audit log key ───────────────────────────

// PrivilegedActionKey returns the store key for a PrivilegedAction by seq.
func PrivilegedActionKey(seq uint64) []byte {
	key := make([]byte, 0, 1+8)
	key = append(key, PrivilegedActionKeyPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, seq)
	return append(key, buf...)
}

// ─── Scientific Method Gap 3: pre-registration key constructors ─────────────

// PreRegistrationKey returns the store key for a PreRegistration by id.
func PreRegistrationKey(id string) []byte {
	return append(append([]byte{}, PreRegistrationKeyPrefix...), []byte(id)...)
}

// PreRegistrationExpiryKey returns the expiry-index key for a PreRegistration.
// Sorted by expiryBlock (big-endian) then id — allows range scan for expired records.
func PreRegistrationExpiryKey(expiryBlock uint64, id string) []byte {
	key := make([]byte, 0, 1+8+len(id))
	key = append(key, PreRegistrationExpiryIndexPrefix...)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, expiryBlock)
	key = append(key, buf...)
	key = append(key, []byte(id)...)
	return key
}

// PreRegistrationExpiryPrefixUpTo returns the prefix for scanning all expiry
// index entries with expiryBlock <= upToBlock (inclusive).
func PreRegistrationExpiryPrefixUpTo(upToBlock uint64) []byte {
	// We scan the full prefix range [0x7B][0x00...] → [0x7B][be64(upToBlock+1)]
	// Callers iterate this range and stop at entries > upToBlock.
	return append([]byte{}, PreRegistrationExpiryIndexPrefix...)
}
