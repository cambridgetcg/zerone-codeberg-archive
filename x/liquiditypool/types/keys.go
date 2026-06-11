package types

const (
	ModuleName   = "liquiditypool"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	MemStoreKey  = "mem_" + ModuleName
	QuerierRoute = ModuleName
)

var (
	PoolKeyPrefix         = []byte{0x01}
	TWAPKeyPrefix         = []byte{0x02}
	ParamsKey             = []byte{0x03}
	PoolCounterKey        = []byte{0x04}
	DenomIndexPrefix        = []byte{0x10} // index: denom pair -> pool ID
	MiningPeriodKeyPrefix   = []byte{0x05} // LiquidityMiningPeriod by period id
	PoolMiningIndexPrefix   = []byte{0x06} // index: pool_id -> active mining period id
	LpMinerPositionPrefix   = []byte{0x07} // LpMinerPosition by pool_id+"/"+addr
	SwapCommitmentKeyPrefix    = []byte{0x08} // SwapCommitment by sender+"/"+pool_id
	LPDepositBlockKeyPrefix    = []byte{0x09} // LP deposit block by pool_id+"/"+addr (anti-JIT cooldown)
	PendingSwapRevealKeyPrefix = []byte{0x0A} // PendingSwapReveal by bigEndian(commitBlock)+"/"+sender+"/"+poolId
)

func PoolKey(poolId string) []byte {
	return append(PoolKeyPrefix, []byte(poolId)...)
}

func TWAPKey(poolId string) []byte {
	return append(TWAPKeyPrefix, []byte(poolId)...)
}

// DenomPairKey returns the index key for a denom pair (sorted lexicographically).
func DenomPairKey(denomA, denomB string) []byte {
	if denomA > denomB {
		denomA, denomB = denomB, denomA
	}
	return append(DenomIndexPrefix, []byte(denomA+"/"+denomB)...)
}

// LPDenom returns the LP token denom for a pool.
func LPDenom(poolId string) string {
	return "lp/" + poolId
}

// MiningPeriodKey returns the storage key for a LiquidityMiningPeriod.
func MiningPeriodKey(periodId string) []byte {
	return append(MiningPeriodKeyPrefix, []byte(periodId)...)
}

// PoolMiningIndexKey returns the index key mapping pool_id → active mining period id.
func PoolMiningIndexKey(poolId string) []byte {
	return append(PoolMiningIndexPrefix, []byte(poolId)...)
}

// LpMinerPositionKey returns the storage key for a LpMinerPosition (pool_id + "/" + addr).
func LpMinerPositionKey(poolId, addr string) []byte {
	return append(LpMinerPositionPrefix, []byte(poolId+"/"+addr)...)
}

// LpMinerPositionPoolPrefix returns the prefix for iterating all positions in a pool.
func LpMinerPositionPoolPrefix(poolId string) []byte {
	return append(LpMinerPositionPrefix, []byte(poolId+"/")...)
}

// SwapCommitmentKey returns the storage key for a SwapCommitment (sender + "/" + pool_id).
func SwapCommitmentKey(sender, poolId string) []byte {
	return append(SwapCommitmentKeyPrefix, []byte(sender+"/"+poolId)...)
}

// LPDepositBlockKey returns the storage key for an LP's last deposit block (pool_id + "/" + addr).
func LPDepositBlockKey(poolId, addr string) []byte {
	return append(LPDepositBlockKeyPrefix, []byte(poolId+"/"+addr)...)
}

// PendingSwapRevealKey returns the storage key for a PendingSwapReveal.
// The big-endian commitBlock prefix ensures lexicographic order == chronological order (FIFO).
// Key layout: PendingSwapRevealKeyPrefix + bigEndian8(commitBlock) + "/" + sender + "/" + poolId
func PendingSwapRevealKey(commitBlock uint64, sender, poolId string) []byte {
	b := make([]byte, 8)
	b[0] = byte(commitBlock >> 56)
	b[1] = byte(commitBlock >> 48)
	b[2] = byte(commitBlock >> 40)
	b[3] = byte(commitBlock >> 32)
	b[4] = byte(commitBlock >> 24)
	b[5] = byte(commitBlock >> 16)
	b[6] = byte(commitBlock >> 8)
	b[7] = byte(commitBlock)
	suffix := append(b, []byte("/"+sender+"/"+poolId)...)
	return append(PendingSwapRevealKeyPrefix, suffix...)
}
