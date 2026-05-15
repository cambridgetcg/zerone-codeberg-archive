package types

const (
	// ModuleName is the disputes module's name.
	ModuleName = "disputes"

	// StoreKey is the store key for the disputes module.
	StoreKey = ModuleName
)

// KV store key prefixes.
var (
	ParamsKey             = []byte{0x00}
	DisputeKeyPrefix      = []byte{0x01}
	EvidenceKeyPrefix     = []byte{0x02}
	CommitmentKeyPrefix   = []byte{0x03}
	VoteKeyPrefix         = []byte{0x04}
	TargetIndexPrefix     = []byte{0x05}
	ActiveIndexPrefix     = []byte{0x06}
	DisputeCounterKey     = []byte{0x07}

	// Service Escrow
	ServiceEscrowKeyPrefix = []byte{0x08}
)

// DisputeKey returns the store key for a dispute by ID.
func DisputeKey(id string) []byte {
	return append(DisputeKeyPrefix, []byte(id)...)
}

// EvidenceKey returns the store key for evidence by dispute+evidence ID.
func EvidenceKey(disputeID, evidenceID string) []byte {
	key := append(EvidenceKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	key = append(key, []byte(evidenceID)...)
	return key
}

// EvidenceByDisputePrefix returns the prefix for all evidence of a dispute.
func EvidenceByDisputePrefix(disputeID string) []byte {
	key := append(EvidenceKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	return key
}

// CommitmentKey returns the store key for an evidence commitment.
func CommitmentKey(disputeID, submitter string) []byte {
	key := append(CommitmentKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	key = append(key, []byte(submitter)...)
	return key
}

// CommitmentByDisputePrefix returns the prefix for all commitments of a dispute.
func CommitmentByDisputePrefix(disputeID string) []byte {
	key := append(CommitmentKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	return key
}

// VoteKey returns the store key for a vote by dispute+arbiter.
func VoteKey(disputeID, arbiter string) []byte {
	key := append(VoteKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	key = append(key, []byte(arbiter)...)
	return key
}

// VoteByDisputePrefix returns the prefix for all votes of a dispute.
func VoteByDisputePrefix(disputeID string) []byte {
	key := append(VoteKeyPrefix, []byte(disputeID)...)
	key = append(key, '/')
	return key
}

// TargetIndexKey returns the index key for disputes by target.
func TargetIndexKey(targetID, disputeID string) []byte {
	key := append(TargetIndexPrefix, []byte(targetID)...)
	key = append(key, '/')
	key = append(key, []byte(disputeID)...)
	return key
}

// TargetIndexKeyPrefix returns the prefix for all disputes of a target.
func TargetIndexKeyPrefix(targetID string) []byte {
	key := append(TargetIndexPrefix, []byte(targetID)...)
	key = append(key, '/')
	return key
}

// ActiveIndexKey returns the index key for active disputes.
func ActiveIndexKey(disputeID string) []byte {
	return append(ActiveIndexPrefix, []byte(disputeID)...)
}

// ServiceEscrowKey returns the store key for a service escrow by ID.
func ServiceEscrowKey(id string) []byte {
	return append(ServiceEscrowKeyPrefix, []byte(id)...)
}
