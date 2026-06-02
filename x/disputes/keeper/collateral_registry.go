package keeper

import (
	"context"
	"encoding/binary"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/disputes/types"
)

// ─── Collateral Registry ─────────────────────────────────────────────────────
//
// Tracks locked capital per (address, lock-ID) pair within the disputes module.
// lock-ID is typically the dispute ID or service escrow ID that locked the funds.
//
// Each lock stores the amount as a uint64 big-endian value in the KV store.
// Amounts are in uzrn (micro-ZRN), matching the bond denomination.
//
// This registry is the foundation for Gap 4 of the Counterparty Escrow brief:
// cross-module collateral aggregation with a governance-set leverage ceiling.
// Other modules (knowledge, liquiditypool, staking) can register locks via this
// pattern when cross-module wiring is added in a future session.

// RegisterCollateralLock records that `addr` has locked `amount` uzrn under `lockID`.
// If a lock with the same (addr, lockID) already exists, it is overwritten.
// amount must be a valid positive decimal string (e.g. "1000").
func (k Keeper) RegisterCollateralLock(ctx context.Context, addr, lockID, amount string) {
	a := new(big.Int)
	if _, ok := a.SetString(amount, 10); !ok || a.Sign() <= 0 {
		return // invalid amount: no-op
	}

	kvStore := k.storeService.OpenKVStore(ctx)
	key := types.CollateralLockKey(addr, lockID)

	// Store as 16-byte big-endian (supports amounts up to ~1.8e19 uzrn).
	// big.Int.Bytes() is variable length; we pad to 16 bytes for fixed width.
	bz := padBigInt(a, 16)
	_ = kvStore.Set(key, bz)
}

// ReleaseCollateralLock removes the lock entry for (addr, lockID).
// Idempotent: if no lock exists, this is a no-op.
func (k Keeper) ReleaseCollateralLock(ctx context.Context, addr, lockID string) {
	kvStore := k.storeService.OpenKVStore(ctx)
	_ = kvStore.Delete(types.CollateralLockKey(addr, lockID))
}

// GetCollateralLock returns the locked amount for (addr, lockID).
// Returns zero if no lock exists.
func (k Keeper) GetCollateralLock(ctx context.Context, addr, lockID string) *big.Int {
	kvStore := k.storeService.OpenKVStore(ctx)
	bz, err := kvStore.Get(types.CollateralLockKey(addr, lockID))
	if err != nil || len(bz) == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).SetBytes(bz)
}

// GetTotalLockedCollateral returns the sum of all locked amounts for addr.
// Iterates all (addr, *) entries in the collateral registry.
func (k Keeper) GetTotalLockedCollateral(ctx context.Context, addr string) *big.Int {
	kvStore := k.storeService.OpenKVStore(ctx)
	prefix := types.CollateralLocksByAddressPrefix(addr)
	iter, err := kvStore.Iterator(prefix, prefixEndBytes(prefix))
	if err != nil {
		return big.NewInt(0)
	}
	defer iter.Close()

	total := big.NewInt(0)
	for ; iter.Valid(); iter.Next() {
		v := iter.Value()
		if len(v) == 0 {
			continue
		}
		total.Add(total, new(big.Int).SetBytes(v))
	}
	return total
}

// CheckLeverageLimit returns ErrLeverageExceeded if adding newBond uzrn to addr's
// total locked collateral would exceed params.MaxLeverageBps of their spendable balance.
// Returns nil if MaxLeverageBps == 0 (disabled) or the check passes.
func (k Keeper) CheckLeverageLimit(ctx context.Context, addr sdk.AccAddress, newBond *big.Int, params *types.Params) error {
	if params.MaxLeverageBps == 0 {
		return nil // disabled
	}

	// Spendable balance at the moment the dispute is opened (before escrowing).
	balance := k.bankKeeper.GetBalance(ctx, addr, "uzrn")
	balanceInt := balance.Amount.BigInt()

	// Total locked after this bond is added.
	currentLocked := k.GetTotalLockedCollateral(ctx, addr.String())
	newTotal := new(big.Int).Add(currentLocked, newBond)

	// Leverage check: newTotal / balance > maxLeverageBps / 1_000_000
	// ↔ newTotal * 1_000_000 > balance * maxLeverageBps
	lhs := new(big.Int).Mul(newTotal, big.NewInt(1_000_000))
	rhs := new(big.Int).Mul(balanceInt, new(big.Int).SetUint64(params.MaxLeverageBps))

	if lhs.Cmp(rhs) > 0 {
		return types.ErrLeverageExceeded
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// padBigInt returns a fixed-width big-endian encoding of n, padded to `width` bytes.
// If n.Bytes() is longer than width, only the first `width` bytes are used (truncation
// only occurs for astronomically large values, not in normal operation).
func padBigInt(n *big.Int, width int) []byte {
	src := n.Bytes()
	if len(src) >= width {
		return src[:width]
	}
	out := make([]byte, width)
	copy(out[width-len(src):], src)
	return out
}

// bigIntFromBytes decodes a big-endian byte slice to *big.Int.
// Kept for symmetry; used in test helpers.
func bigIntFromBytes(bz []byte) *big.Int {
	_ = binary.BigEndian // ensure import used
	return new(big.Int).SetBytes(bz)
}
