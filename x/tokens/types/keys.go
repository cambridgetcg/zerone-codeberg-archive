package types

const (
	// ModuleName defines the module name.
	ModuleName = "tokens"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// RouterKey defines the routing key.
	RouterKey = ModuleName

	// MemStoreKey defines the in-memory store key.
	MemStoreKey = "mem_" + ModuleName

	// QuerierRoute defines the querier route.
	QuerierRoute = ModuleName
)

// Store key prefixes.
var (
	ParamsKey             = []byte{0x00}
	TokenDefKeyPrefix     = []byte{0x01} // {tokenId} -> TokenDefinition (proto)
	BalanceKeyPrefix      = []byte{0x02} // {tokenId}/{ownerAddr} -> big.Int string
	AllowanceKeyPrefix    = []byte{0x03} // {tokenId}/{ownerAddr}/{spenderAddr} -> big.Int string
	OwnerTokenIndexPrefix = []byte{0x04} // {ownerAddr}/{tokenId} -> []byte{1} (exists flag)
	SymbolIndexPrefix     = []byte{0x05} // {symbol} -> tokenId string

	// Delegation prefixes
	DelegationKeyPrefix     = []byte{0x06} // {tokenId}/{delegator}/{delegate} -> amount string
	DelegatorTotalKeyPrefix = []byte{0x07} // {tokenId}/{delegator} -> total delegated string

	// Wrap prefixes
	WrapRecordKeyPrefix    = []byte{0x08} // {tokenId} -> wrappedDenom string
	WrapReverseIndexPrefix = []byte{0x09} // {wrappedDenom} -> tokenId string

	// Emission prefixes
	EmissionPeriodKeyPrefix = []byte{0x0A} // {emissionId} -> EmissionPeriod (proto)

	// Adaptive emission scale singleton (Gap 2)
	EmissionScaleKey = []byte{0x0B} // uint64 BPS; default 1_000_000 (1.0×) when not set
)

// TokenDefKey returns the store key for a token definition.
func TokenDefKey(tokenId string) []byte {
	return append(TokenDefKeyPrefix, []byte(tokenId)...)
}

// BalanceKey returns the store key for a specific balance.
func BalanceKey(tokenId, ownerAddr string) []byte {
	return append(BalanceKeyPrefix, []byte(tokenId+"/"+ownerAddr)...)
}

// BalancesByTokenPrefix returns the prefix for iterating all balances of a token.
func BalancesByTokenPrefix(tokenId string) []byte {
	return append(BalanceKeyPrefix, []byte(tokenId+"/")...)
}

// AllowanceKey returns the store key for a specific allowance.
func AllowanceKey(tokenId, ownerAddr, spenderAddr string) []byte {
	return append(AllowanceKeyPrefix, []byte(tokenId+"/"+ownerAddr+"/"+spenderAddr)...)
}

// AllowancesByOwnerPrefix returns the prefix for iterating all allowances granted by an owner for a token.
func AllowancesByOwnerPrefix(tokenId, ownerAddr string) []byte {
	return append(AllowanceKeyPrefix, []byte(tokenId+"/"+ownerAddr+"/")...)
}

// OwnerTokenIndexKey returns the store key for the owner-token index entry.
func OwnerTokenIndexKey(ownerAddr, tokenId string) []byte {
	return append(OwnerTokenIndexPrefix, []byte(ownerAddr+"/"+tokenId)...)
}

// OwnerTokensPrefix returns the prefix for iterating all tokens owned by an address.
func OwnerTokensPrefix(ownerAddr string) []byte {
	return append(OwnerTokenIndexPrefix, []byte(ownerAddr+"/")...)
}

// SymbolIndexKey returns the store key for the symbol-to-tokenId index.
func SymbolIndexKey(symbol string) []byte {
	return append(SymbolIndexPrefix, []byte(symbol)...)
}

// DelegationKey returns the store key for a specific delegation.
func DelegationKey(tokenId, delegator, delegate string) []byte {
	return append(DelegationKeyPrefix, []byte(tokenId+"/"+delegator+"/"+delegate)...)
}

// DelegationsByDelegatorPrefix returns the prefix for iterating all delegations by a delegator for a token.
func DelegationsByDelegatorPrefix(tokenId, delegator string) []byte {
	return append(DelegationKeyPrefix, []byte(tokenId+"/"+delegator+"/")...)
}

// DelegationsByTokenPrefix returns the prefix for iterating all delegations for a token.
func DelegationsByTokenPrefix(tokenId string) []byte {
	return append(DelegationKeyPrefix, []byte(tokenId+"/")...)
}

// DelegatorTotalKey returns the store key for a delegator's total delegated amount for a token.
func DelegatorTotalKey(tokenId, delegator string) []byte {
	return append(DelegatorTotalKeyPrefix, []byte(tokenId+"/"+delegator)...)
}

// DelegatorTotalsByTokenPrefix returns the prefix for iterating all delegator totals for a token.
func DelegatorTotalsByTokenPrefix(tokenId string) []byte {
	return append(DelegatorTotalKeyPrefix, []byte(tokenId+"/")...)
}

// WrapRecordKey returns the store key for a wrap record (tokenId -> wrappedDenom).
func WrapRecordKey(tokenId string) []byte {
	return append(WrapRecordKeyPrefix, []byte(tokenId)...)
}

// WrapReverseIndexKey returns the store key for the reverse index (wrappedDenom -> tokenId).
func WrapReverseIndexKey(wrappedDenom string) []byte {
	return append(WrapReverseIndexPrefix, []byte(wrappedDenom)...)
}

// EmissionPeriodKey returns the store key for an emission period.
func EmissionPeriodKey(emissionId string) []byte {
	return append(EmissionPeriodKeyPrefix, []byte(emissionId)...)
}
