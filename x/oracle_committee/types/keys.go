package types

const (
	// ModuleName is the oracle_committee module's name.
	ModuleName = "oracle_committee"

	// StoreKey is the store key for the oracle_committee module.
	StoreKey = ModuleName
)

// KV store key prefixes.
var (
	ParamsKey             = []byte{0x00}
	OperatorKeyPrefix     = []byte{0x01}
	EvaluationKeyPrefix   = []byte{0x02}
	AggregationKeyPrefix  = []byte{0x03}
)

// OperatorKey returns the store key for an oracle operator by address.
func OperatorKey(address string) []byte {
	return append(OperatorKeyPrefix, []byte(address)...)
}

// EvaluationKey returns the store key for an evaluation by (claimId, operator).
func EvaluationKey(claimID, operator string) []byte {
	key := append(EvaluationKeyPrefix, []byte(claimID)...)
	key = append(key, '/')
	key = append(key, []byte(operator)...)
	return key
}

// EvaluationByClaimPrefix returns the prefix for all evaluations of a claim.
func EvaluationByClaimPrefix(claimID string) []byte {
	key := append(EvaluationKeyPrefix, []byte(claimID)...)
	key = append(key, '/')
	return key
}

// AggregationKey returns the store key for the final aggregation of a claim.
func AggregationKey(claimID string) []byte {
	return append(AggregationKeyPrefix, []byte(claimID)...)
}
