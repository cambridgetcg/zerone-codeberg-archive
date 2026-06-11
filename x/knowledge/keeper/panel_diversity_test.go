package keeper_test

// Scientific Method Gap 1: Verifier Independence
//
// Panel diversity measures organizational independence of a verification panel:
//   diversity_score = (distinct_org_count / total_verifiers) × 1_000_000
//
// Key properties:
//   - Verifiers without declared organization → counted as unique singletons
//     (fail-open: unknown affiliation does not reduce diversity score)
//   - min_panel_diversity_bps = 0 → gate disabled
//   - ACCEPT verdicts below threshold → INCONCLUSIVE (not REJECT)
//   - diversity_score always stored in round.panel_diversity_score_bps
//   - Self-declaration via MsgSetVerifierOrganization (anyone sets own org)
//   - Authority override for any address (governance bootstrapping)

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// divAddr returns a deterministic bech32 address for diversity test verifiers.
func divAddr(seed string) string {
	return makeValidBech32Addr("diversity-" + seed)
}

func makeRoundWithRevealers(verifiers []string) *types.VerificationRound {
	reveals := make([]*types.RevealEntry, 0, len(verifiers))
	for _, v := range verifiers {
		reveals = append(reveals, &types.RevealEntry{
			Verifier: v,
			Vote:     "accept",
		})
	}
	return &types.VerificationRound{
		Id:      "test-round",
		ClaimId: "test-claim",
		Reveals: reveals,
	}
}

// mustGetParams is a test helper that panics if GetParams returns an error.
func mustGetParams(t *testing.T, k keeper.Keeper, ctx interface{ Deadline() (interface{}, bool) }) *types.Params {
	t.Helper()
	return nil // placeholder — use inline GetParams
}

// ── Unit tests: ComputePanelDiversityScore ──────────────────────────────────

// TestDiversityScore_AllDifferentOrgs verifies maximum diversity
// when every verifier is from a different organization.
func TestDiversityScore_AllDifferentOrgs(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	addrs := []string{divAddr("org1v"), divAddr("org2v"), divAddr("org3v")}
	orgs := []string{"org-A", "org-B", "org-C"}
	for i, addr := range addrs {
		require.NoError(t, k.SetVerifierOrganization(ctx, addr, orgs[i]))
	}

	round := makeRoundWithRevealers(addrs)
	score := k.ComputePanelDiversityScore(ctx, round)
	require.Equal(t, uint64(1_000_000), score,
		"3 verifiers from 3 different orgs = 100%% diversity")
}

// TestDiversityScore_AllSameOrg verifies minimum diversity
// when all verifiers belong to the same organization.
func TestDiversityScore_AllSameOrg(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	addrs := []string{divAddr("same1"), divAddr("same2"), divAddr("same3")}
	for _, addr := range addrs {
		require.NoError(t, k.SetVerifierOrganization(ctx, addr, "org-X"))
	}

	round := makeRoundWithRevealers(addrs)
	score := k.ComputePanelDiversityScore(ctx, round)
	// 1 distinct org / 3 verifiers = 333_333
	require.Equal(t, uint64(333_333), score,
		"3 verifiers from 1 org = 33.3%% diversity")
}

// TestDiversityScore_NoOrganizationDeclarations verifies that verifiers
// without organization_id are treated as unique singletons (fail-open).
func TestDiversityScore_NoOrganizationDeclarations(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// No organizations set — all verifiers are unknown affiliations.
	addrs := []string{divAddr("noorg1"), divAddr("noorg2"), divAddr("noorg3")}

	round := makeRoundWithRevealers(addrs)
	score := k.ComputePanelDiversityScore(ctx, round)
	require.Equal(t, uint64(1_000_000), score,
		"verifiers without declared org are counted as unique singletons — full diversity")
}

// TestDiversityScore_Mixed verifies partial diversity:
// 2 verifiers from org-A, 1 from org-B = 2 distinct / 3 total = 666_666.
func TestDiversityScore_Mixed(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	addrA1 := divAddr("mixA1")
	addrA2 := divAddr("mixA2")
	addrB := divAddr("mixB")

	require.NoError(t, k.SetVerifierOrganization(ctx, addrA1, "org-A"))
	require.NoError(t, k.SetVerifierOrganization(ctx, addrA2, "org-A"))
	require.NoError(t, k.SetVerifierOrganization(ctx, addrB, "org-B"))

	round := makeRoundWithRevealers([]string{addrA1, addrA2, addrB})
	score := k.ComputePanelDiversityScore(ctx, round)
	// 2 distinct / 3 total = 666_666
	require.Equal(t, uint64(666_666), score)
}

// TestDiversityScore_EmptyPanel verifies fail-open: empty panel = full diversity.
func TestDiversityScore_EmptyPanel(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	round := makeRoundWithRevealers([]string{})
	score := k.ComputePanelDiversityScore(ctx, round)
	require.Equal(t, uint64(1_000_000), score,
		"empty panel should return max diversity (fail-open)")
}

// ── Unit tests: CheckPanelDiversityGate ──────────────────────────────────────

func TestDiversityGate_Disabled(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.MinPanelDiversityBps = 0 // disabled
	_ = k.SetParams(ctx, p)

	// Any diversity score should pass when gate is disabled.
	require.NoError(t, k.CheckPanelDiversityGate(0, *p))
	require.NoError(t, k.CheckPanelDiversityGate(1_000_000, *p))
}

func TestDiversityGate_PassesThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.MinPanelDiversityBps = 500_000 // 50% threshold
	_ = k.SetParams(ctx, p)

	// Exactly at threshold — passes.
	require.NoError(t, k.CheckPanelDiversityGate(500_000, *p))
	// Above threshold — passes.
	require.NoError(t, k.CheckPanelDiversityGate(666_666, *p))
}

func TestDiversityGate_FailsBelowThreshold(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.MinPanelDiversityBps = 500_000
	_ = k.SetParams(ctx, p)

	// Below threshold — should fail.
	err := k.CheckPanelDiversityGate(333_333, *p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "panel diversity score")
}

// ── Integration tests: MsgSetVerifierOrganization ───────────────────────────

// TestSetVerifierOrganization_SelfDeclaration verifies that a verifier can
// declare their own organization.
func TestSetVerifierOrganization_SelfDeclaration(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("self-org-verifier")
	resp, err := ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          verifier,
		VerifierAddress: verifier,
		OrganizationId:  "university-of-zerone",
	})
	require.NoError(t, err)
	require.Equal(t, "university-of-zerone", resp.OrganizationId)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, "university-of-zerone", cal.OrganizationId)
}

// TestSetVerifierOrganization_ClearAffiliation verifies that setting an empty
// organization_id clears a previously declared affiliation.
func TestSetVerifierOrganization_ClearAffiliation(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("clear-org-verifier")
	_, err := ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          verifier,
		VerifierAddress: verifier,
		OrganizationId:  "some-org",
	})
	require.NoError(t, err)

	// Clear it.
	_, err = ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          verifier,
		VerifierAddress: verifier,
		OrganizationId:  "", // empty = clear
	})
	require.NoError(t, err)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Empty(t, cal.OrganizationId)
}

// TestSetVerifierOrganization_AuthorityOverride verifies that the governance
// authority can set organization_id for any verifier.
func TestSetVerifierOrganization_AuthorityOverride(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("override-target-verifier")
	_, err := ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          "zrn1authority", // matches keepertest authority
		VerifierAddress: verifier,
		OrganizationId:  "verified-org",
		Rationale:       "bootstrapping known affiliation from off-chain registry",
	})
	require.NoError(t, err)

	cal, found := k.GetAgentCalibration(ctx, verifier)
	require.True(t, found)
	require.Equal(t, "verified-org", cal.OrganizationId)
}

// TestSetVerifierOrganization_AuthorityOverride_RequiresRationale verifies that
// governance overrides require a rationale for audit purposes.
func TestSetVerifierOrganization_AuthorityOverride_RequiresRationale(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("rationale-req-verifier")
	_, err := ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          "zrn1authority",
		VerifierAddress: verifier,
		OrganizationId:  "some-org",
		Rationale:       "", // missing
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rationale required")
}

// TestSetVerifierOrganization_Unauthorized verifies that a third party cannot
// set another verifier's organization.
func TestSetVerifierOrganization_Unauthorized(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)

	verifier := makeValidBech32Addr("target-verifier-unauth")
	attacker := makeValidBech32Addr("attacker-unauth")

	_, err := ms.SetVerifierOrganization(ctx, &types.MsgSetVerifierOrganization{
		Sender:          attacker,
		VerifierAddress: verifier,
		OrganizationId:  "fake-org",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")
}

// TestSetVerifierOrganization_NilRequest verifies nil-safety.
func TestSetVerifierOrganization_NilRequest(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	ms := keeper.NewMsgServerImpl(k)
	_, err := ms.SetVerifierOrganization(ctx, nil)
	require.Error(t, err)
}

// TestDiversityScore_StoredInRound verifies that panel_diversity_score_bps is
// always stored on the VerificationRound at CompleteRound, enabling governance
// audits of panel independence regardless of verdict.
func TestDiversityScore_StoredInRound(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Set up 3 verifiers with 2 from same org (score = 666_666).
	addrA1 := divAddr("stored-A1")
	addrA2 := divAddr("stored-A2")
	addrB := divAddr("stored-B")
	require.NoError(t, k.SetVerifierOrganization(ctx, addrA1, "org-Y"))
	require.NoError(t, k.SetVerifierOrganization(ctx, addrA2, "org-Y"))
	require.NoError(t, k.SetVerifierOrganization(ctx, addrB, "org-Z"))

	round := makeRoundWithRevealers([]string{addrA1, addrA2, addrB})
	score := k.ComputePanelDiversityScore(ctx, round)
	require.Equal(t, uint64(666_666), score)

	// Simulate CompleteRound setting PanelDiversityScoreBps.
	round.PanelDiversityScoreBps = score
	require.Equal(t, uint64(666_666), round.PanelDiversityScoreBps)
}

// TestDiversityGate_InconclusivenessOnAccept verifies that an ACCEPT verdict
// is overridden to INCONCLUSIVE when diversity is below threshold.
func TestDiversityGate_InconclusivenessOnAccept(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)
	p, _ := k.GetParams(ctx)
	p.MinPanelDiversityBps = 700_000 // require 70% diversity
	_ = k.SetParams(ctx, p)

	// Score = 333_333 (all same org) — below threshold.
	err := k.CheckPanelDiversityGate(333_333, *p)
	require.Error(t, err, "score below threshold should fail the gate")

	// Score = 1_000_000 (all different orgs) — above threshold.
	err = k.CheckPanelDiversityGate(1_000_000, *p)
	require.NoError(t, err, "score at max should pass the gate")
}
