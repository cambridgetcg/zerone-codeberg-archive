package types

import "encoding/binary"

const (
	// ModuleName is the module name for zerone staking.
	ModuleName = "zerone_staking"

	// StoreKey is the KVStore key.
	StoreKey = ModuleName

	// RouterKey is the msg router key.
	RouterKey = ModuleName
)

// KV store prefixes.
var (
	ValidatorKeyPrefix             = []byte{0x01}
	DelegationKeyPrefix            = []byte{0x02}
	UnbondingKeyPrefix             = []byte{0x03}
	TierConfigKeyPrefix            = []byte{0x04}
	ParamsKey                      = []byte{0x05}
	ValidatorByDIDPrefix           = []byte{0x06}
	UnbondingSeqKey                = []byte{0x07}
	RedelegationCooldownPrefix     = []byte{0x08}
	ValidatorDelegationIndexPrefix  = []byte{0x09} // reverse index: validator → delegators
	SlashEvidenceQueuePrefix        = []byte{0x0A} // pending evidence for end-block processing
)

// ValidatorKey returns the store key for a validator by operator address.
func ValidatorKey(operatorAddr string) []byte {
	return append(ValidatorKeyPrefix, []byte(operatorAddr)...)
}

// ValidatorByDIDKey returns the store key for a DID → operator mapping.
func ValidatorByDIDKey(did string) []byte {
	return append(ValidatorByDIDPrefix, []byte(did)...)
}

// DelegationKey returns the store key for a specific delegation.
func DelegationKey(delegatorAddr, validatorAddr string) []byte {
	key := append(DelegationKeyPrefix, []byte(delegatorAddr)...)
	key = append(key, 0x00)
	key = append(key, []byte(validatorAddr)...)
	return key
}

// DelegationsByDelegatorPrefix returns the prefix for all delegations by a delegator.
func DelegationsByDelegatorPrefix(delegatorAddr string) []byte {
	key := append(DelegationKeyPrefix, []byte(delegatorAddr)...)
	key = append(key, 0x00)
	return key
}

// ValidatorDelegationIndexKey returns the reverse-index key for delegations to a validator.
func ValidatorDelegationIndexKey(validatorAddr, delegatorAddr string) []byte {
	key := append(ValidatorDelegationIndexPrefix, []byte(validatorAddr)...)
	key = append(key, 0x00)
	key = append(key, []byte(delegatorAddr)...)
	return key
}

// DelegationsByValidatorPrefix returns the prefix for the reverse delegation index.
func DelegationsByValidatorPrefix(validatorAddr string) []byte {
	key := append(ValidatorDelegationIndexPrefix, []byte(validatorAddr)...)
	key = append(key, 0x00)
	return key
}

// UnbondingKey returns the store key for an unbonding entry.
func UnbondingKey(id string) []byte {
	return append(UnbondingKeyPrefix, []byte(id)...)
}

// TierConfigKey returns the store key for a tier configuration.
func TierConfigKey(tier ValidatorTier) []byte {
	return append(TierConfigKeyPrefix, byte(tier))
}

// RedelegationCooldownKey returns the store key for a delegator's last redelegation height.
func RedelegationCooldownKey(delegatorAddr string) []byte {
	return append(RedelegationCooldownPrefix, []byte(delegatorAddr)...)
}

// SlashEvidenceKey returns the store key for a slash evidence item by ID.
func SlashEvidenceKey(evidenceID string) []byte {
	return append(SlashEvidenceQueuePrefix, []byte(evidenceID)...)
}

// Uint64ToBytes converts a uint64 to big-endian bytes.
func Uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// BytesToUint64 converts big-endian bytes to uint64.
func BytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
