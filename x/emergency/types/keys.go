package types

const (
	// ModuleName is the emergency module's name.
	ModuleName = "emergency"

	// StoreKey is the store key for the emergency module.
	StoreKey = ModuleName
)

// KV store key prefixes.
var (
	ParamsKey                   = []byte{0x01}
	CeremonyKeyPrefix           = []byte{0x02}
	AuditLogKeyPrefix           = []byte{0x03}
	HaltStatusKey               = []byte{0x04}
	GuardianProposalCountPrefix = []byte{0x05}
	EpochProposalCountKey       = []byte{0x06}
	LastProposalBlockKey        = []byte{0x07}
	ActiveHaltCeremonyIdKey     = []byte{0x08}
	HaltStartBlockKey           = []byte{0x09}
	RevertTargetHeightKey       = []byte{0x0A}
	RevertTargetHashKey         = []byte{0x0B}
	RevertCeremonyIdKey         = []byte{0x0C}

	// LastOracleUpdateBlockKey stores the block height of the most recent TWAP
	// update received from the liquiditypool module. Used by the oracle staleness
	// circuit breaker (P-21b) to detect oracle failure.
	LastOracleUpdateBlockKey = []byte{0x0D}

	// StateCorrectionKeyPrefix is the key prefix for StateCorrectionProposal records.
	// Keyed by proposal ID string. Circuit Breaker Gap 5.
	StateCorrectionKeyPrefix = []byte{0x0E}
)

// CeremonyKey returns the store key for a ceremony by ID.
func CeremonyKey(id string) []byte {
	return append(CeremonyKeyPrefix, []byte(id)...)
}

// GuardianProposalCountKey returns the store key for a guardian's proposal count.
func GuardianProposalCountKey(addr string) []byte {
	return append(GuardianProposalCountPrefix, []byte(addr)...)
}

// StateCorrectionKey returns the store key for a StateCorrectionProposal by ID.
func StateCorrectionKey(id string) []byte {
	return append(StateCorrectionKeyPrefix, []byte(id)...)
}

// AuditLogKey returns the store key for an audit entry by height and index.
func AuditLogKey(height uint64, index uint32) []byte {
	key := make([]byte, len(AuditLogKeyPrefix)+12)
	copy(key, AuditLogKeyPrefix)
	// Big-endian height for ordered iteration.
	key[len(AuditLogKeyPrefix)] = byte(height >> 56)
	key[len(AuditLogKeyPrefix)+1] = byte(height >> 48)
	key[len(AuditLogKeyPrefix)+2] = byte(height >> 40)
	key[len(AuditLogKeyPrefix)+3] = byte(height >> 32)
	key[len(AuditLogKeyPrefix)+4] = byte(height >> 24)
	key[len(AuditLogKeyPrefix)+5] = byte(height >> 16)
	key[len(AuditLogKeyPrefix)+6] = byte(height >> 8)
	key[len(AuditLogKeyPrefix)+7] = byte(height)
	// Big-endian index.
	key[len(AuditLogKeyPrefix)+8] = byte(index >> 24)
	key[len(AuditLogKeyPrefix)+9] = byte(index >> 16)
	key[len(AuditLogKeyPrefix)+10] = byte(index >> 8)
	key[len(AuditLogKeyPrefix)+11] = byte(index)
	return key
}
