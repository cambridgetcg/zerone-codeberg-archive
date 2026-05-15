package keeper_test

import (
	"context"
	"encoding/hex"
	"testing"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

func init() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("zrn", "zrnpub")
	config.SetBech32PrefixForValidator("zrnvaloper", "zrnvaloperpub")
	config.SetBech32PrefixForConsensusNode("zrnvalcons", "zrnvalconspub")
}

// ---------- Mock Keepers ----------

type mockBankKeeper struct{}

func (bk *mockBankKeeper) SendCoins(_ context.Context, _, _ sdk.AccAddress, _ sdk.Coins) error {
	return nil
}
func (bk *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, _ string, _ sdk.AccAddress, _ sdk.Coins) error {
	return nil
}
func (bk *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, _ sdk.AccAddress, _ string, _ sdk.Coins) error {
	return nil
}
func (bk *mockBankKeeper) SendCoinsFromModuleToModule(_ context.Context, _, _ string, _ sdk.Coins) error {
	return nil
}
func (bk *mockBankKeeper) GetBalance(_ context.Context, _ sdk.AccAddress, _ string) sdk.Coin {
	return sdk.NewInt64Coin("uzrn", 0)
}
func (bk *mockBankKeeper) MintCoins(_ context.Context, _ string, _ sdk.Coins) error {
	return nil
}
func (bk *mockBankKeeper) BurnCoins(_ context.Context, _ string, _ sdk.Coins) error {
	return nil
}

type mockStakingKeeper struct {
	validators map[string]*types.ValidatorInfo
	totalStake uint64
}

func newMockStakingKeeper() *mockStakingKeeper {
	return &mockStakingKeeper{
		validators: make(map[string]*types.ValidatorInfo),
		totalStake: 1_000_000,
	}
}

func (sk *mockStakingKeeper) GetActiveValidatorInfos(_ context.Context) ([]types.ValidatorInfo, error) {
	vals := make([]types.ValidatorInfo, 0, len(sk.validators))
	for _, v := range sk.validators {
		vals = append(vals, types.ValidatorInfo{
			Address:           v.Address,
			Stake:             v.Stake,
			Tier:              v.Tier,
			VerificationCount: v.VerificationCount,
			AccuracyBps:       v.AccuracyBps,
		})
	}
	return vals, nil
}

func (sk *mockStakingKeeper) GetValidatorInfo(_ context.Context, addr string) (*types.ValidatorInfo, error) {
	v, ok := sk.validators[addr]
	if !ok {
		return &types.ValidatorInfo{Address: addr, Stake: 100_000}, nil
	}
	return v, nil
}

func (sk *mockStakingKeeper) GetEffectiveStake(_ context.Context, addr string) (uint64, error) {
	if v, ok := sk.validators[addr]; ok {
		return v.Stake, nil
	}
	return 100_000, nil
}

func (sk *mockStakingKeeper) GetTotalStake(_ context.Context) (uint64, error) {
	return sk.totalStake, nil
}

func (sk *mockStakingKeeper) SlashValidator(_ context.Context, _ string, _ uint64) error {
	return nil
}

func (sk *mockStakingKeeper) SlashValidatorToModule(_ context.Context, _ string, _ uint64, _ string) (sdkmath.Int, error) {
	return sdkmath.ZeroInt(), nil
}

// ---------- Test Setup ----------

func setupKnowledgeKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	bk := &mockBankKeeper{}
	sk := newMockStakingKeeper()

	k := keeper.NewKeeper(
		runtime.NewKVStoreService(storeKey),
		cdc,
		"authority",
		bk,
		sk,
	)

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger())

	// Initialize default params
	require.NoError(t, k.InitGenesis(ctx, types.DefaultGenesis()))

	return k, ctx
}

// testRound creates a round in the given phase.
func testRound(id, claimID string, phase types.VerificationPhase) *types.VerificationRound {
	return &types.VerificationRound{
		Id:                  id,
		ClaimId:             claimID,
		StartedAtBlock:      50,
		Phase:               phase,
		SelectedVerifiers:   nil,
		Commits:             nil,
		Reveals:             nil,
		CommitDeadline:      150,
		RevealDeadline:      200,
		AggregationDeadline: 250,
	}
}

// ---------- StoreCommitmentInRound Tests ----------

func TestStoreCommitmentInRound_Success(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit := &types.CommitEntry{
		Verifier:         "zrn1validator1",
		CommitHash:       []byte("abcdef1234567890abcdef1234567890"),
		CommittedAtBlock: 100,
	}

	err := k.StoreCommitmentInRound(ctx, "round-1", commit)
	require.NoError(t, err)

	// Verify round was updated
	updated, found := k.GetVerificationRound(ctx, "round-1")
	require.True(t, found)
	require.Len(t, updated.Commits, 1)
	require.Equal(t, "zrn1validator1", updated.Commits[0].Verifier)
	require.Contains(t, updated.SelectedVerifiers, "zrn1validator1")
}

func TestStoreCommitmentInRound_WrongPhase(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit := &types.CommitEntry{
		Verifier:         "zrn1validator1",
		CommitHash:       []byte("abcdef1234567890"),
		CommittedAtBlock: 100,
	}

	err := k.StoreCommitmentInRound(ctx, "round-1", commit)
	require.ErrorIs(t, err, types.ErrRoundNotInCommitPhase)
}

func TestStoreCommitmentInRound_Duplicate(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	commitHash := []byte("abcdef1234567890abcdef1234567890")
	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit := &types.CommitEntry{
		Verifier:         "zrn1validator1",
		CommitHash:       commitHash,
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "round-1", commit))

	// Same hash from same verifier → duplicate (idempotent)
	err := k.StoreCommitmentInRound(ctx, "round-1", commit)
	require.ErrorIs(t, err, types.ErrDuplicateCommitment)
}

func TestStoreCommitmentInRound_Equivocation(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	commit1 := &types.CommitEntry{
		Verifier:         "zrn1validator1",
		CommitHash:       []byte("hash_one_xxxxxxxxxxxxxxxxxxxxxxxx"),
		CommittedAtBlock: 100,
	}
	require.NoError(t, k.StoreCommitmentInRound(ctx, "round-1", commit1))

	// Different hash from same verifier → equivocation
	commit2 := &types.CommitEntry{
		Verifier:         "zrn1validator1",
		CommitHash:       []byte("hash_two_xxxxxxxxxxxxxxxxxxxxxxxx"),
		CommittedAtBlock: 101,
	}
	err := k.StoreCommitmentInRound(ctx, "round-1", commit2)
	require.ErrorIs(t, err, types.ErrEquivocation)
}

// ---------- StoreRevealInRound Tests ----------

func TestStoreRevealInRound_Success(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	salt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("round-1", "accept", 600_000, salt)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	round.Commits = []*types.CommitEntry{
		{
			Verifier:         "zrn1validator1",
			CommitHash:       commitHash,
			CommittedAtBlock: 90,
		},
	}
	round.SelectedVerifiers = []string{"zrn1validator1"}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	reveal := &types.RevealEntry{
		Verifier:        "zrn1validator1",
		Vote:            "accept",
		Salt:            salt,
		RevealedAtBlock: 100,
	}

	err := k.StoreRevealInRound(ctx, "round-1", reveal, 600_000)
	require.NoError(t, err)

	updated, found := k.GetVerificationRound(ctx, "round-1")
	require.True(t, found)
	require.Len(t, updated.Reveals, 1)
	require.Equal(t, "accept", updated.Reveals[0].Vote)
}

func TestStoreRevealInRound_WrongPhase(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	reveal := &types.RevealEntry{
		Verifier:        "zrn1validator1",
		Vote:            "accept",
		Salt:            []byte("salt"),
		RevealedAtBlock: 100,
	}

	err := k.StoreRevealInRound(ctx, "round-1", reveal, 600_000)
	require.ErrorIs(t, err, types.ErrRoundNotInRevealPhase)
}

func TestStoreRevealInRound_NoCommitment(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	reveal := &types.RevealEntry{
		Verifier:        "zrn1validator1",
		Vote:            "accept",
		Salt:            []byte("salt"),
		RevealedAtBlock: 100,
	}

	err := k.StoreRevealInRound(ctx, "round-1", reveal, 600_000)
	require.ErrorIs(t, err, types.ErrNoCommitment)
}

func TestStoreRevealInRound_HashMismatch(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	correctSalt, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	commitHash := types.ComputeCommitmentHash("round-1", "accept", 600_000, correctSalt)

	round := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	round.Commits = []*types.CommitEntry{
		{
			Verifier:         "zrn1validator1",
			CommitHash:       commitHash,
			CommittedAtBlock: 90,
		},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	wrongSalt, _ := hex.DecodeString("1122334455667788aabbccddeeff0011")
	reveal := &types.RevealEntry{
		Verifier:        "zrn1validator1",
		Vote:            "accept",
		Salt:            wrongSalt,
		RevealedAtBlock: 100,
	}

	err := k.StoreRevealInRound(ctx, "round-1", reveal, 600_000)
	require.ErrorIs(t, err, types.ErrRevealMismatch)
}

// ---------- GetActiveRounds Test ----------

func TestGetActiveRounds(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round1 := testRound("round-1", "claim-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round2 := testRound("round-2", "claim-2", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	round3 := testRound("round-3", "claim-3", types.VerificationPhase_VERIFICATION_PHASE_COMPLETE)

	require.NoError(t, k.SetVerificationRound(ctx, round1))
	require.NoError(t, k.SetVerificationRound(ctx, round2))
	require.NoError(t, k.SetVerificationRound(ctx, round3))

	active := k.GetActiveRounds(ctx)
	require.Len(t, active, 2)

	ids := make(map[string]bool)
	for _, r := range active {
		ids[r.Id] = true
	}
	require.True(t, ids["round-1"])
	require.True(t, ids["round-2"])
	require.False(t, ids["round-3"])
}

// ========== GetExpectedPhase Tests (pure function — no store setup) ==========

func TestGetExpectedPhase_CommitPhase(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	params := types.DefaultParams()

	// height 100 < commitDeadline 150 → still COMMIT
	got := keeper.GetExpectedPhase(round, 100, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got)

	// height 0 (genesis) → still COMMIT
	got = keeper.GetExpectedPhase(round, 0, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got)

	// height 149 (one before commitDeadline) → still COMMIT
	got = keeper.GetExpectedPhase(round, 149, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, got)
}

func TestGetExpectedPhase_RevealPhase(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	params := types.DefaultParams()

	// commitDeadline=150, revealDeadline=200
	// height 150 → REVEAL (at boundary)
	got := keeper.GetExpectedPhase(round, 150, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, got)

	// height 175 (mid-reveal) → REVEAL
	got = keeper.GetExpectedPhase(round, 175, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, got)

	// height 199 (one before revealDeadline) → REVEAL
	got = keeper.GetExpectedPhase(round, 199, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, got)
}

func TestGetExpectedPhase_AggregationPhase(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	params := types.DefaultParams()

	// revealDeadline=200, aggregationDeadline=250
	// height 200 → AGGREGATION
	got := keeper.GetExpectedPhase(round, 200, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, got)

	// height 225 (mid-aggregation) → AGGREGATION
	got = keeper.GetExpectedPhase(round, 225, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, got)

	// height 249 (one before aggregationDeadline) → AGGREGATION
	got = keeper.GetExpectedPhase(round, 249, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, got)
}

func TestGetExpectedPhase_ExpiredPhase(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	params := types.DefaultParams()

	// aggregationDeadline=250
	// height 250 → EXPIRED
	got := keeper.GetExpectedPhase(round, 250, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)

	// height 1000 (far past) → EXPIRED
	got = keeper.GetExpectedPhase(round, 1000, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)
}

func TestGetExpectedPhase_CompleteStaysComplete(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_COMPLETE)
	params := types.DefaultParams()

	// Even with height past all deadlines, COMPLETE stays COMPLETE
	got := keeper.GetExpectedPhase(round, 0, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, got)

	got = keeper.GetExpectedPhase(round, 100, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, got)

	got = keeper.GetExpectedPhase(round, 500, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, got)
}

func TestGetExpectedPhase_ExpiredStaysExpired(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_EXPIRED)
	params := types.DefaultParams()

	// EXPIRED stays EXPIRED regardless of height
	got := keeper.GetExpectedPhase(round, 0, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)

	got = keeper.GetExpectedPhase(round, 100, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)

	got = keeper.GetExpectedPhase(round, 999, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)
}

func TestGetExpectedPhase_ExactDeadlineBoundary(t *testing.T) {
	round := testRound("r-1", "c-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	params := types.DefaultParams()

	// At exact commitDeadline (150) → REVEAL
	got := keeper.GetExpectedPhase(round, round.CommitDeadline, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, got)

	// At exact revealDeadline (200) → AGGREGATION
	got = keeper.GetExpectedPhase(round, round.RevealDeadline, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION, got)

	// At exact aggregationDeadline (250) → EXPIRED
	got = keeper.GetExpectedPhase(round, round.AggregationDeadline, &params)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, got)
}

// ========== AdvanceRoundPhases / BeginBlocker Tests ==========

func TestAdvanceRoundPhases_CommitToRevealTransition(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-adv-1", "claim-adv-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	// commitDeadline=150, context height=100 → not past yet
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past commitDeadline (150) but before revealDeadline (200)
	ctx = ctx.WithBlockHeight(160)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-adv-1")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, updated.Phase)
}

func TestAdvanceRoundPhases_RevealToAggregation(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create a claim for the round (needed for aggregation → CompleteRound)
	claim := &types.Claim{
		Id:          "claim-agg-1",
		FactContent: "Water boils at 100 degrees Celsius at 1 atm",
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round in REVEAL phase with enough reveals (MinVerifiers default = 3)
	salt1, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	salt2, _ := hex.DecodeString("11223344aabbccdd11223344aabbccdd")
	salt3, _ := hex.DecodeString("aabb112233445566aabb112233445566")

	round := testRound("round-agg-1", "claim-agg-1", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-agg-1", "accept", 800000, salt1), CommittedAtBlock: 60},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-agg-1", "accept", 800000, salt2), CommittedAtBlock: 61},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-agg-1", "accept", 800000, salt3), CommittedAtBlock: 62},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt1, RevealedAtBlock: 155},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt2, RevealedAtBlock: 156},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt3, RevealedAtBlock: 157},
	}
	round.SelectedVerifiers = []string{
		"zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past revealDeadline (200) but before aggregationDeadline (250)
	ctx = ctx.WithBlockHeight(210)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-agg-1")
	require.True(t, found)
	// Should have transitioned to COMPLETE (aggregation triggers CompleteRound)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, updated.Phase)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, updated.Verdict)
}

func TestAdvanceRoundPhases_NoTransitionNeeded(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-no-tr", "claim-no-tr", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	// commitDeadline=150, context height=100 → no transition needed
	require.NoError(t, k.SetVerificationRound(ctx, round))

	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-no-tr")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMMIT, updated.Phase)
}

func TestAdvanceRoundPhases_ExpiredWithSufficientReveals(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create claim for the round
	claim := &types.Claim{
		Id:          "claim-exp-suf",
		FactContent: "The speed of light in vacuum is approximately 3e8 m/s",
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round still in COMMIT phase but with reveals already stored
	// (simulates a scenario where reveals were added while phase was behind)
	salt1, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	salt2, _ := hex.DecodeString("11223344aabbccdd11223344aabbccdd")
	salt3, _ := hex.DecodeString("aabb112233445566aabb112233445566")

	round := testRound("round-exp-suf", "claim-exp-suf", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-exp-suf", "accept", 800000, salt1), CommittedAtBlock: 60},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-exp-suf", "accept", 800000, salt2), CommittedAtBlock: 61},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-exp-suf", "accept", 800000, salt3), CommittedAtBlock: 62},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt1, RevealedAtBlock: 155},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt2, RevealedAtBlock: 156},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: salt3, RevealedAtBlock: 157},
	}
	round.SelectedVerifiers = []string{
		"zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past aggregationDeadline (250) → EXPIRED, but has enough reveals → aggregates
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-exp-suf")
	require.True(t, found)
	// Should have aggregated and completed despite being past deadline
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, updated.Phase)
	require.Equal(t, types.Verdict_VERDICT_ACCEPT, updated.Verdict)
}

func TestAdvanceRoundPhases_ExpiredInsufficientReveals(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create claim for the round
	claim := &types.Claim{
		Id:          "claim-exp-ins",
		FactContent: "Insufficient reveals will cause expiration",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round in COMMIT phase with only 1 reveal (MinVerifiers default = 3)
	round := testRound("round-exp-ins", "claim-exp-ins", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: []byte("salt1"), RevealedAtBlock: 155},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past aggregationDeadline (250) → EXPIRED with insufficient reveals
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-exp-ins")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, updated.Phase)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, updated.Verdict)
}

func TestAdvanceRoundPhases_MultipleRounds(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Round 1: COMMIT phase, should transition to REVEAL at height 160
	round1 := testRound("round-multi-1", "claim-multi-1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round1))

	// Round 2: COMMIT phase with no reveals, should expire at height 300
	round2 := testRound("round-multi-2", "claim-multi-2", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	// Make round2 expire sooner: aggregationDeadline=120 (before current height 160)
	round2.CommitDeadline = 80
	round2.RevealDeadline = 100
	round2.AggregationDeadline = 120
	require.NoError(t, k.SetVerificationRound(ctx, round2))

	// Create claim for round2 (needed for expiration handling)
	claim2 := &types.Claim{
		Id:        "claim-multi-2",
		Submitter: "zrn1submitter000000000000000000000",
		Status:    types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim2))

	// Set height to 160: round1 should → REVEAL, round2 should → EXPIRED
	ctx = ctx.WithBlockHeight(160)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated1, found1 := k.GetVerificationRound(ctx, "round-multi-1")
	require.True(t, found1)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, updated1.Phase)

	updated2, found2 := k.GetVerificationRound(ctx, "round-multi-2")
	require.True(t, found2)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, updated2.Phase)
}

func TestAdvanceRoundPhases_CompleteSkipped(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-complete", "claim-complete", types.VerificationPhase_VERIFICATION_PHASE_COMPLETE)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height far past all deadlines
	ctx = ctx.WithBlockHeight(500)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// COMPLETE rounds are not in active index, so AdvanceRoundPhases won't iterate them.
	// Verify it's unchanged.
	updated, found := k.GetVerificationRound(ctx, "round-complete")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, updated.Phase)
}

func TestAdvanceRoundPhases_ExpiredReturnsClaim(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create claim with IN_VERIFICATION status
	claim := &types.Claim{
		Id:          "claim-return",
		FactContent: "This claim will expire and stake will return",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "5000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round with 0 reveals → insufficient
	round := testRound("round-return", "claim-return", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance past aggregationDeadline
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// Verify claim status was updated to INSUFFICIENT
	updatedClaim, found := k.GetClaim(ctx, "claim-return")
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_INSUFFICIENT, updatedClaim.Status)

	// Verify round is EXPIRED with INCONCLUSIVE verdict
	updatedRound, found := k.GetVerificationRound(ctx, "round-return")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, updatedRound.Phase)
	require.Equal(t, types.Verdict_VERDICT_INCONCLUSIVE, updatedRound.Verdict)
}

func TestBeginBlocker_DelegatesToAdvance(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create a round that should transition at height 160
	round := testRound("round-bb", "claim-bb", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past commitDeadline
	ctx = ctx.WithBlockHeight(160)

	// BeginBlocker delegates to AdvanceRoundPhases
	require.NoError(t, k.BeginBlocker(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-bb")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_REVEAL, updated.Phase)
}

// ========== GetEligibleValidators Tests ==========

func TestGetEligibleValidators_ReturnsAll(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// The mock staking keeper is set up by setupKnowledgeKeeper.
	// Access via the keeper's staking keeper interface and add validators.
	sk := k.GetStakingKeeper().(*mockStakingKeeper)
	sk.validators["zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx"] = &types.ValidatorInfo{
		Address:           "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Stake:             500_000,
		Tier:              "bonded",
		VerificationCount: 100,
		AccuracyBps:       900_000,
	}
	sk.validators["zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx"] = &types.ValidatorInfo{
		Address:           "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Stake:             300_000,
		Tier:              "verified",
		VerificationCount: 50,
		AccuracyBps:       850_000,
	}

	vals, err := k.GetEligibleValidators(ctx, "")
	require.NoError(t, err)
	require.Len(t, vals, 2)

	// Verify all validators returned
	addrs := make(map[string]bool)
	for i := range vals {
		addrs[vals[i].Address] = true
	}
	require.True(t, addrs["zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx"])
	require.True(t, addrs["zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx"])
}

func TestGetEligibleValidators_Empty(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Mock staking keeper has no validators by default
	vals, err := k.GetEligibleValidators(ctx, "")
	require.NoError(t, err)
	require.Empty(t, vals)
}

func TestGetEligibleValidators_PreservesValidatorInfo(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	sk := k.GetStakingKeeper().(*mockStakingKeeper)
	sk.validators["zrn1guardian00000000000000000000000"] = &types.ValidatorInfo{
		Address:           "zrn1guardian00000000000000000000000",
		Stake:             11_111_000_000,
		Tier:              "guardian",
		VerificationCount: 2000,
		AccuracyBps:       950_000,
	}

	vals, err := k.GetEligibleValidators(ctx, "")
	require.NoError(t, err)
	require.Len(t, vals, 1)

	require.Equal(t, "zrn1guardian00000000000000000000000", vals[0].Address)
	require.Equal(t, uint64(11_111_000_000), vals[0].Stake)
	require.Equal(t, "guardian", vals[0].Tier)
	require.Equal(t, uint64(2000), vals[0].VerificationCount)
	require.Equal(t, uint64(950_000), vals[0].AccuracyBps)
}

// ========== SlashMissedVerification Tests ==========

func TestSlashMissedVerification_DelegatesSlash(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// SlashValidator on mock staking keeper returns nil → no error
	err := k.SlashMissedVerification(ctx, "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", 100_000)
	require.NoError(t, err)
}

func TestSlashMissedVerification_ZeroBps(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Slash with 0 BPS is still valid (no-op slash)
	err := k.SlashMissedVerification(ctx, "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", 0)
	require.NoError(t, err)
}

// ========== VerifyValidatorVRFSelection Tests ==========

func TestVerifyVRFSelection_RoundNotFound(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	selected, err := k.VerifyValidatorVRFSelection(
		ctx, "nonexistent-round", "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		[]byte("vrf-output"), []byte("vrf-proof"),
	)
	require.NoError(t, err)
	require.False(t, selected)
}

func TestVerifyVRFSelection_RoundExists(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	round := testRound("round-vrf", "claim-vrf", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// The VRF selection depends on the output bytes and stake.
	// With mock staking keeper returning 100_000 stake and 1_000_000 total,
	// selection depends on VRF output relative to threshold.
	// We just verify the function runs without error and returns a boolean.
	selected, err := k.VerifyValidatorVRFSelection(
		ctx, "round-vrf", "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		make([]byte, 32), []byte("proof"),
	)
	require.NoError(t, err)
	// selected is deterministic based on vrfOutput bytes — just verify no error
	_ = selected
}

// ========== Edge case / integration tests ==========

func TestAdvanceRoundPhases_ExpiredRoundRemovedFromActive(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	claim := &types.Claim{
		Id:        "claim-active-rm",
		Submitter: "zrn1submitter000000000000000000000",
		Status:    types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := testRound("round-active-rm", "claim-active-rm", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Verify round is in active list
	active := k.GetActiveRounds(ctx)
	require.Len(t, active, 1)

	// Expire the round
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// After expiration, round should be removed from active list
	active = k.GetActiveRounds(ctx)
	require.Empty(t, active)
}

func TestAdvanceRoundPhases_EmptyRounds(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// No active rounds at all → AdvanceRoundPhases is a no-op
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	active := k.GetActiveRounds(ctx)
	require.Empty(t, active)
}

func TestAdvanceRoundPhases_RevealToAggregation_Reject(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Create a claim for the round
	claim := &types.Claim{
		Id:          "claim-rej",
		FactContent: "This claim will be rejected by verifiers",
		Domain:      "physics",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round in REVEAL phase with 3 reject votes
	salt1, _ := hex.DecodeString("aabbccdd11223344aabbccdd11223344")
	salt2, _ := hex.DecodeString("11223344aabbccdd11223344aabbccdd")
	salt3, _ := hex.DecodeString("aabb112233445566aabb112233445566")

	round := testRound("round-rej", "claim-rej", types.VerificationPhase_VERIFICATION_PHASE_REVEAL)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-rej", "reject", 800000, salt1), CommittedAtBlock: 60},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-rej", "reject", 800000, salt2), CommittedAtBlock: 61},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", CommitHash: types.ComputeCommitmentHash("round-rej", "reject", 800000, salt3), CommittedAtBlock: 62},
	}
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "reject", Salt: salt1, RevealedAtBlock: 155},
		{Verifier: "zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "reject", Salt: salt2, RevealedAtBlock: 156},
		{Verifier: "zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "reject", Salt: salt3, RevealedAtBlock: 157},
	}
	round.SelectedVerifiers = []string{
		"zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val2xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"zrn1val3xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Set height past revealDeadline but before aggregationDeadline
	ctx = ctx.WithBlockHeight(210)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	updated, found := k.GetVerificationRound(ctx, "round-rej")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_COMPLETE, updated.Phase)
	require.Equal(t, types.Verdict_VERDICT_REJECT, updated.Verdict)

	// Claim should be REJECTED
	updatedClaim, found := k.GetClaim(ctx, "claim-rej")
	require.True(t, found)
	require.Equal(t, types.ClaimStatus_CLAIM_STATUS_REJECTED, updatedClaim.Status)
}

// ── P-24b: Abandoned round finality ─────────────────────────────────────────
//
// Gap: rounds that expire with insufficient reveals bypassed CompleteRound,
// so ActiveRoundsCount was never decremented and committed-but-not-revealed
// verifiers escaped the MissedRevealSlashBps penalty. Both are now fixed in
// phases.go (DecrementActiveRoundsCount + missed-reveal slash loop).

// TestExpiredRound_DecrementsActiveRoundsCount verifies that an abandoned round
// (expired with too few reveals to reach quorum) correctly decrements the
// ActiveRoundsCount singleton. Before the P-24b fix, each abandoned round
// inflated the counter permanently, corrupting the congestion-based fee market.
func TestExpiredRound_DecrementsActiveRoundsCount(t *testing.T) {
	k, ctx := setupKnowledgeKeeper(t)

	// Seed counter at 3 (simulates 3 prior active rounds in flight).
	k.SetActiveRoundsCount(ctx, 3)

	// Create a claim to be abandoned.
	claim := &types.Claim{
		Id:          "claim-p24b-counter",
		FactContent: "Abandoned round — no quorum, counter must drop",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Create round: 1 reveal, MinVerifiers default = 3 → insufficient.
	round := testRound("round-p24b-counter", "claim-p24b-counter", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Reveals = []*types.RevealEntry{
		{Verifier: "zrn1val1xxxxxxxxxxxxxxxxxxxxxxxxxxxx", Vote: "accept", Salt: []byte("s1"), RevealedAtBlock: 155},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance to past AggregationDeadline.
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// Counter must have been decremented by exactly 1.
	require.Equal(t, uint64(2), k.GetActiveRoundsCount(ctx),
		"ActiveRoundsCount must drop by 1 when a round expires with insufficient reveals (P-24b)")

	// Sanity: round is EXPIRED.
	updated, found := k.GetVerificationRound(ctx, "round-p24b-counter")
	require.True(t, found)
	require.Equal(t, types.VerificationPhase_VERIFICATION_PHASE_EXPIRED, updated.Phase)
}

// TestExpiredRound_SlashesMissedRevealers verifies that verifiers who committed
// to a round but never revealed are slashed at MissedRevealSlashBps when the
// round expires with insufficient participation. Before P-24b, these verifiers
// escaped the penalty entirely because the insufficient-reveals path bypassed
// the normal aggregation / slash pipeline.
func TestExpiredRound_SlashesMissedRevealers(t *testing.T) {
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	// Enable missed-reveal penalty.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MissedRevealSlashBps = 50_000 // 5%
	require.NoError(t, k.SetParams(ctx, params))

	// Create a claim.
	claim := &types.Claim{
		Id:          "claim-p24b-slash",
		FactContent: "Abandoned round — committed verifiers must be slashed",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	// Two verifiers commit; zero reveal. (MinVerifiers = 3 → insufficient.)
	committer1 := "zrn1committer1xxxxxxxxxxxxxxxxxxxxxx"
	committer2 := "zrn1committer2xxxxxxxxxxxxxxxxxxxxxx"

	round := testRound("round-p24b-slash", "claim-p24b-slash", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Commits = []*types.CommitEntry{
		{Verifier: committer1, CommitHash: []byte("h1"), CommittedAtBlock: 110},
		{Verifier: committer2, CommitHash: []byte("h2"), CommittedAtBlock: 115},
	}
	// No reveals.
	require.NoError(t, k.SetVerificationRound(ctx, round))

	// Advance past AggregationDeadline.
	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	// Both committers must have been slashed at MissedRevealSlashBps.
	require.Len(t, sk.slashes, 2,
		"both committers must be slashed for missed reveal (P-24b)")
	slashMap := make(map[string]uint64)
	for _, s := range sk.slashes {
		slashMap[s.Validator] = s.SlashBps
	}
	require.Equal(t, uint64(50_000), slashMap[committer1],
		"committer1 must be slashed at MissedRevealSlashBps")
	require.Equal(t, uint64(50_000), slashMap[committer2],
		"committer2 must be slashed at MissedRevealSlashBps")
}

// TestExpiredRound_NoSlash_WhenMissedRevealSlashBpsIsZero verifies that when
// MissedRevealSlashBps = 0 (disabled), no slashes are issued on round expiry.
func TestExpiredRound_NoSlash_WhenMissedRevealSlashBpsIsZero(t *testing.T) {
	k, ctx, _, sk := setupKnowledgeTestFull(t)

	// Explicitly disable missed-reveal penalty.
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.MissedRevealSlashBps = 0
	require.NoError(t, k.SetParams(ctx, params))

	claim := &types.Claim{
		Id:          "claim-p24b-noslash",
		FactContent: "No slash when MissedRevealSlashBps is zero",
		Domain:      "general",
		Category:    "empirical",
		Submitter:   "zrn1submitter000000000000000000000",
		Status:      types.ClaimStatus_CLAIM_STATUS_IN_VERIFICATION,
		Stake:       "1000000",
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	round := testRound("round-p24b-noslash", "claim-p24b-noslash", types.VerificationPhase_VERIFICATION_PHASE_COMMIT)
	round.Commits = []*types.CommitEntry{
		{Verifier: "zrn1committer1xxxxxxxxxxxxxxxxxxxxxx", CommitHash: []byte("h1"), CommittedAtBlock: 110},
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	ctx = ctx.WithBlockHeight(300)
	require.NoError(t, k.AdvanceRoundPhases(ctx))

	require.Empty(t, sk.slashes, "no slashes should be issued when MissedRevealSlashBps = 0")
}
