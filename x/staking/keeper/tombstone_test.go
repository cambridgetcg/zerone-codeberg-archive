package keeper_test

import (
	"math/big"
	"testing"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/staking/keeper"
	"github.com/zerone-chain/zerone/x/staking/types"
)

// slashAmt is a convenience helper: slash a validator by a small fixed amount.
func slashAmt(k keeper.Keeper, ctx sdk.Context, addr string) {
	k.SlashValidator(ctx, addr, big.NewInt(1000), "test_slash")
}

// ============================================================
// Tombstoning — Slashing Gap 1
// ============================================================
//
// Tombstoning is a PERMANENT ban distinct from jailing (P-19a).
// SlashCount >= tombstone_threshold → val.Tombstoned = true.
// Governance can lift via MsgUntombstoneValidator (authority-gated).
// Tombstoned operators cannot self-unbond.

// setupTombstoneKeeper returns a keeper and context with a
// governance-enabled tombstone threshold wired into params.
func setupTombstoneKeeper(t *testing.T, threshold uint64) (keeper.Keeper, sdk.Context) {
	t.Helper()
	k, ctx := setupKeeper(t)

	p := k.GetParams(ctx)
	p.TombstoneThreshold = threshold
	p.MaxSlashCountDeactivate = 1 // deactivate after 1 slash; tombstone after threshold
	p.MaxSlashesPerEpoch = 0      // 0 = unlimited (no per-epoch cap for tombstone tests)
	k.SetParams(ctx, p)

	return k, ctx
}

// seedTombstoneValidator creates a validator with the given slash count and stake.
func seedTombstoneValidator(t *testing.T, k keeper.Keeper, ctx sdk.Context, addr string, slashCount uint64) {
	t.Helper()
	val := &types.Validator{
		OperatorAddress: addr,
		ConsensusPubkey: "pk_tomb",
		Moniker:         "tombval",
		Tier:            types.TierApprentice,
		SelfDelegation:  "10000000",
		DelegatedStake:  "0",
		TotalStake:      "10000000",
		ReputationScore: 500_000,
		IsActive:        true,
		SlashCount:      slashCount,
	}
	k.SetValidator(ctx, val)
}

// TestTombstone_FiresAtThreshold verifies that a validator is tombstoned
// when slash_count reaches the configured tombstone_threshold.
func TestTombstone_FiresAtThreshold(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 3) // tombstone at 3 slashes
	addr := testAddr("tomb_fires")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	// Slash twice — below threshold, not tombstoned.
	slashAmt(k, ctx, addr)
	slashAmt(k, ctx, addr)
	val, _ := k.GetValidator(ctx, addr)
	require.False(t, val.Tombstoned, "should not be tombstoned at 2 slashes (threshold=3)")

	// Third slash — crosses threshold.
	slashAmt(k, ctx, addr)
	val, _ = k.GetValidator(ctx, addr)
	require.True(t, val.Tombstoned, "should be tombstoned at slash_count >= threshold (3)")
	require.Equal(t, uint64(ctx.BlockHeight()), val.TombstonedAt)
	require.False(t, val.IsActive, "tombstoned validator must be inactive")
}

// TestTombstone_Disabled verifies that setting tombstone_threshold=0
// disables automatic tombstoning entirely.
func TestTombstone_Disabled(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 0) // threshold=0 → disabled
	addr := testAddr("tomb_disabled")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	// Slash many times — tombstoning should never fire.
	for i := 0; i < 10; i++ {
		slashAmt(k, ctx, addr)
	}
	val, _ := k.GetValidator(ctx, addr)
	require.False(t, val.Tombstoned, "tombstone_threshold=0 must never tombstone")
}

// TestTombstone_Idempotent verifies that a validator already tombstoned
// does not have its TombstonedAt clobbered by subsequent slashes.
func TestTombstone_Idempotent(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 2)
	addr := testAddr("tomb_idem")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	// Slash to tombstone at height 100.
	slashAmt(k, ctx, addr)
	slashAmt(k, ctx, addr)
	val, _ := k.GetValidator(ctx, addr)
	require.True(t, val.Tombstoned)
	firstTombstonedAt := val.TombstonedAt

	// Advance height and slash again.
	ctx2 := ctx.WithBlockHeader(cmtproto.Header{Height: 200})
	slashAmt(k, ctx2, addr)
	val2, _ := k.GetValidator(ctx2, addr)

	require.True(t, val2.Tombstoned)
	require.Equal(t, firstTombstonedAt, val2.TombstonedAt, "TombstonedAt must not be overwritten")
}

// TestTombstone_OperatorCannotSelfUnbond verifies that a tombstoned operator
// cannot undelegate their own stake until governance lifts the tombstone.
func TestTombstone_OperatorCannotSelfUnbond(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 1)
	addr := testAddr("tomb_unbond")

	// Seed with SlashCount already at threshold so next slash tombstones immediately.
	seedTombstoneValidator(t, k, ctx, addr, 0)
	// Add a delegation record for the operator.
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: addr,
		ValidatorAddress: addr,
		Amount:           "10000000",
	})

	slashAmt(k, ctx, addr) // threshold=1 → tombstoned now

	val, _ := k.GetValidator(ctx, addr)
	require.True(t, val.Tombstoned)

	// Attempt self-unbond — must fail.
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: addr,
		Validator: addr,
		Amount:    "1000",
	})
	require.ErrorIs(t, err, types.ErrTombstonedCannotSelfUnbond,
		"tombstoned operator must not be able to self-unbond")
}

// TestTombstone_DelegatorCanUnbond verifies that a non-operator delegator
// CAN still undelegate from a tombstoned validator (only operator is blocked).
func TestTombstone_DelegatorCanUnbond(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 1)
	valAddr := testAddr("tomb_delval")
	delAddr := testAddr("tomb_delgtr")

	seedTombstoneValidator(t, k, ctx, valAddr, 0)
	// Seed a delegation from a third-party delegator.
	k.SetDelegation(ctx, &types.Delegation{
		DelegatorAddress: delAddr,
		ValidatorAddress: valAddr,
		Amount:           "5000000",
	})
	val, _ := k.GetValidator(ctx, valAddr)
	val.DelegatedStake = "5000000"
	val.TotalStake = "15000000"
	k.SetValidator(ctx, val)

	slashAmt(k, ctx, valAddr) // tombstone fires

	val, _ = k.GetValidator(ctx, valAddr)
	require.True(t, val.Tombstoned)

	// Third-party delegator undelegate — must succeed.
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.Undelegate(ctx, &types.MsgUndelegate{
		Delegator: delAddr,
		Validator: valAddr,
		Amount:    "1000",
	})
	require.NoError(t, err, "non-operator delegator must be able to undelegate from tombstoned validator")
}

// TestUntombstoneValidator_Success verifies that governance can lift a tombstone.
func TestUntombstoneValidator_Success(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 1)
	addr := testAddr("untomb_ok")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	slashAmt(k, ctx, addr)

	val, _ := k.GetValidator(ctx, addr)
	require.True(t, val.Tombstoned)

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UntombstoneValidator(ctx, &types.MsgUntombstoneValidator{
		Authority:        "authority", // matches keeper authority
		ValidatorAddress: addr,
		Reason:           "governance review complete",
	})
	require.NoError(t, err)

	val, _ = k.GetValidator(ctx, addr)
	require.False(t, val.Tombstoned, "tombstone must be cleared after UntombstoneValidator")
	require.Equal(t, uint64(0), val.TombstonedAt, "TombstonedAt must reset to 0")
	// IsActive is NOT automatically restored — governance decides separately.
}

// TestUntombstoneValidator_NotTombstoned verifies that untombstoning a
// non-tombstoned validator returns ErrNotTombstoned.
func TestUntombstoneValidator_NotTombstoned(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 3)
	addr := testAddr("untomb_notset")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UntombstoneValidator(ctx, &types.MsgUntombstoneValidator{
		Authority:        "authority",
		ValidatorAddress: addr,
		Reason:           "mistake",
	})
	require.ErrorIs(t, err, types.ErrNotTombstoned)
}

// TestUntombstoneValidator_Unauthorized verifies that a non-authority caller
// cannot untombstone a validator.
func TestUntombstoneValidator_Unauthorized(t *testing.T) {
	k, ctx := setupTombstoneKeeper(t, 1)
	addr := testAddr("untomb_unauth")
	seedTombstoneValidator(t, k, ctx, addr, 0)

	slashAmt(k, ctx, addr)

	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.UntombstoneValidator(ctx, &types.MsgUntombstoneValidator{
		Authority:        testAddr("not_authority"),
		ValidatorAddress: addr,
		Reason:           "unauthorized attempt",
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// TestTombstoneParams_Validate verifies that tombstone_threshold must exceed
// max_slash_count_deactivate when both are non-zero.
func TestTombstoneParams_Validate(t *testing.T) {
	k, ctx := setupKeeper(t)
	base := k.GetParams(ctx)

	// threshold == deactivate: invalid (no window between deactivate and tombstone).
	bad := base
	bad.MaxSlashCountDeactivate = 3
	bad.TombstoneThreshold = 3
	require.Error(t, bad.Validate(), "threshold == deactivate must be invalid")

	// threshold < deactivate: also invalid.
	bad2 := base
	bad2.MaxSlashCountDeactivate = 5
	bad2.TombstoneThreshold = 3
	require.Error(t, bad2.Validate(), "threshold < deactivate must be invalid")

	// threshold > deactivate: valid hierarchy.
	good := base
	good.MaxSlashCountDeactivate = 2
	good.TombstoneThreshold = 5
	require.NoError(t, good.Validate(), "threshold > deactivate must be valid")

	// threshold=0: disabled regardless of deactivate value.
	disabled := base
	disabled.MaxSlashCountDeactivate = 3
	disabled.TombstoneThreshold = 0
	require.NoError(t, disabled.Validate(), "threshold=0 (disabled) must always be valid")

	// Both zero: valid (tombstoning and deactivation both disabled).
	both0 := base
	both0.MaxSlashCountDeactivate = 0
	both0.TombstoneThreshold = 0
	require.NoError(t, both0.Validate(), "both zero must be valid")
}
