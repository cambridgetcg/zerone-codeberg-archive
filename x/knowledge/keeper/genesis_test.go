package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ─── InitGenesis ─────────────────────────────────────────────────────────────

func TestInitGenesis_DefaultGenesis(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(3), params.MinVerifiers)
	require.Equal(t, uint64(22), params.MaxVerifiers)
}

func TestInitGenesis_CustomParams(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	custom := types.DefaultParams()
	custom.MinVerifiers = 7
	custom.MaxVerifiers = 44
	custom.CommitPhaseBlocks = 10

	gs := &types.GenesisState{
		Params:  &custom,
		Domains: types.DefaultDomains(),
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	got, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(7), got.MinVerifiers)
	require.Equal(t, uint64(44), got.MaxVerifiers)
	require.Equal(t, uint64(10), got.CommitPhaseBlocks)
}

func TestInitGenesis_18DefaultDomains(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	var count int
	k.IterateDomains(ctx, func(_ *types.Domain) bool {
		count++
		return false
	})
	require.Equal(t, 18, count)
}

func TestInitGenesis_WithPreexistingFacts(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	fact := &types.Fact{
		Id:      "genesis-fact-1",
		Content: "Water boils at 100C at sea level",
		Domain:  "physics",
		Status:  types.FactStatus_FACT_STATUS_VERIFIED,
	}

	gs := &types.GenesisState{
		Params:  func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains: types.DefaultDomains(),
		Facts:   []*types.Fact{fact},
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	got, found := k.GetFact(ctx, "genesis-fact-1")
	require.True(t, found)
	require.Equal(t, "Water boils at 100C at sea level", got.Content)
}

func TestInitGenesis_WithPreexistingClaims(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	claim := &types.Claim{
		Id:          "genesis-claim-1",
		FactContent: "Test genesis claim content here",
		Domain:      "mathematics",
		Submitter:   "zrn1sub",
		Status:      types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:       "1000000",
	}

	gs := &types.GenesisState{
		Params:        func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains:       types.DefaultDomains(),
		PendingClaims: []*types.Claim{claim},
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	got, found := k.GetClaim(ctx, "genesis-claim-1")
	require.True(t, found)
	require.Equal(t, "Test genesis claim content here", got.FactContent)
}

func TestInitGenesis_SkipsNilEntries(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	gs := &types.GenesisState{
		Params:        func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains:       []*types.Domain{nil, {Name: "valid_domain", Status: types.DomainStatus_DOMAIN_STATUS_ACTIVE}, nil},
		Facts:         []*types.Fact{nil},
		PendingClaims: []*types.Claim{nil},
		ActiveRounds:  []*types.VerificationRound{nil},
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	_, found := k.GetDomain(ctx, "valid_domain")
	require.True(t, found)
}

// ─── ExportGenesis ──────────────────────────────────────────────────────────

func TestExportGenesis_RoundTrip(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Add extra state
	makeTestFact(t, k, ctx, "export-fact-1", "Export test content", "physics", "empirical", "zrn1sub", 800_000)

	// Export
	exported := k.ExportGenesis(ctx)
	require.NotNil(t, exported)
	require.NotNil(t, exported.Params)
	require.GreaterOrEqual(t, len(exported.Domains), 18)

	// Verify fact is in export
	var foundFact bool
	for _, f := range exported.Facts {
		if f.Id == "export-fact-1" {
			foundFact = true
			require.Equal(t, "Export test content", f.Content)
		}
	}
	require.True(t, foundFact)
}

func TestExportGenesis_PreservesParams(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	params := types.DefaultParams()
	params.MinVerifiers = 11
	require.NoError(t, k.SetParams(ctx, &params))

	exported := k.ExportGenesis(ctx)
	require.Equal(t, uint64(11), exported.Params.MinVerifiers)
}

func TestExportGenesis_IncludesActiveRounds(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	round := makeRoundInPhase("export-round", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 100)
	require.NoError(t, k.SetVerificationRound(ctx, round))

	exported := k.ExportGenesis(ctx)
	var foundRound bool
	for _, r := range exported.ActiveRounds {
		if r.Id == "export-round" {
			foundRound = true
		}
	}
	require.True(t, foundRound)
}

// ─── GenesisState Validation ────────────────────────────────────────────────

func TestGenesisValidate_Default(t *testing.T) {
	gs := types.DefaultGenesis()
	require.NoError(t, gs.Validate())
}

func TestGenesisValidate_NilParams(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Params = nil
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_InvalidParams(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Params.MinVerifiers = 0 // invalid
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_DuplicateFactIDs(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Facts = []*types.Fact{
		{Id: "same-id", Content: "First"},
		{Id: "same-id", Content: "Second"},
	}
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_DuplicateClaimIDs(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.PendingClaims = []*types.Claim{
		{Id: "dup-claim"},
		{Id: "dup-claim"},
	}
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_DuplicateDomainNames(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Domains = append(gs.Domains, &types.Domain{Name: "mathematics"})
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_InvalidReference(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Facts = []*types.Fact{
		{Id: "fact-1", Content: "Valid", References: []string{"nonexistent"}},
	}
	require.Error(t, gs.Validate())
}

func TestGenesisValidate_ValidReferences(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Facts = []*types.Fact{
		{Id: "fact-a", Content: "Base fact"},
		{Id: "fact-b", Content: "Depends on A", References: []string{"fact-a"}},
	}
	require.NoError(t, gs.Validate())
}

// ─── ActiveRoundsCount genesis seeding ──────────────────────────────────────

func TestInitGenesis_ActiveRoundsCount_SeededFromImport(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Import a genesis state with 3 explicit active rounds.
	gs := &types.GenesisState{
		Params:  func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains: types.DefaultDomains(),
		ActiveRounds: []*types.VerificationRound{
			makeRoundInPhase("cong-r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50),
			makeRoundInPhase("cong-r2", "c2", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 60),
			makeRoundInPhase("cong-r3", "c3", types.VerificationPhase_VERIFICATION_PHASE_REVEAL, 70),
		},
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	// The counter must reflect the 3 non-nil rounds — not 0.
	require.Equal(t, uint64(3), k.GetActiveRoundsCount(ctx),
		"ActiveRoundsCount must be seeded from imported active rounds")
}

func TestInitGenesis_ActiveRoundsCount_NilRoundsNotCounted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	gs := &types.GenesisState{
		Params:  func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains: types.DefaultDomains(),
		ActiveRounds: []*types.VerificationRound{
			nil, // nil — must be skipped
			makeRoundInPhase("cong-r1", "c1", types.VerificationPhase_VERIFICATION_PHASE_COMMIT, 50),
			nil, // nil — must be skipped
		},
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	// Only 1 non-nil round — counter must be 1, not 3.
	require.Equal(t, uint64(1), k.GetActiveRoundsCount(ctx),
		"nil active rounds must not inflate the ActiveRoundsCount")
}

func TestInitGenesis_ActiveRoundsCount_EmptyRounds(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	gs := &types.GenesisState{
		Params:       func() *types.Params { p := types.DefaultParams(); return &p }(),
		Domains:      types.DefaultDomains(),
		ActiveRounds: []*types.VerificationRound{}, // empty
	}
	require.NoError(t, k.InitGenesis(ctx, gs))

	require.Equal(t, uint64(0), k.GetActiveRoundsCount(ctx),
		"empty active rounds slice should seed counter to 0")
}

// ─── Params validation: congestion ───────────────────────────────────────────

func TestGenesisValidate_CongestionMultiplierWithZeroThreshold(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Params.CongestionMultiplierBps = 10_000   // enabled
	gs.Params.CongestionThresholdRounds = 0      // zero threshold — must be rejected
	require.Error(t, gs.Validate(),
		"non-zero multiplier with zero threshold must fail validation")
}

func TestGenesisValidate_CongestionMultiplierWithNonZeroThreshold(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Params.CongestionMultiplierBps = 10_000
	gs.Params.CongestionThresholdRounds = 100
	require.NoError(t, gs.Validate(),
		"non-zero multiplier with non-zero threshold must pass validation")
}

func TestGenesisValidate_CongestionDisabled_ZeroThresholdOK(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Params.CongestionMultiplierBps = 0   // disabled
	gs.Params.CongestionThresholdRounds = 0 // zero threshold is fine when multiplier is 0
	require.NoError(t, gs.Validate(),
		"zero multiplier with zero threshold must pass validation (disabled state)")
}
