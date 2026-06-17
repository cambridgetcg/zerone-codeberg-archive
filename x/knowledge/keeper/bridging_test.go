package keeper_test

// Structured Deliberation Gap 4 — Polis Bridging: Cross-Organizational Consensus.
//
// Polis (pol.is, Audrey Tang / vTaiwan): cross-organizational consensus is
// epistemically stronger than within-cluster agreement. "Bridging statements"
// are claims that achieve high agreement *across* ideologically distinct groups.
//
// On-chain encoding:
//
//	When a claim is accepted (ACCEPT verdict), ComputeBridgingOrgCount walks
//	the round's reveals and counts how many distinct organizations voted "accept".
//	The count is stored as Fact.BridgingOrgCount. When the count meets or exceeds
//	Params.BridgingMinOrgs (if non-zero), zerone.knowledge.bridging_claim_accepted
//	is emitted.
//
//	Organization tracking uses AgentCalibration.OrganizationId (same field as
//	Scientific Method Gap 1 panel diversity). Verifiers without a declared org
//	are each treated as a unique singleton — they cannot artificially deflate
//	the bridging count.
//
// Tests:
//
//  1. NoAcceptVotes — no accept reveals → BridgingOrgCount = 0.
//  2. SingleOrgAllAccept — three accepts from same org → count = 1.
//  3. TwoDistinctOrgs — two orgs each vote accept → count = 2.
//  4. OnlyAcceptVotersCounted — reject voter's org not counted even if distinct.
//  5. NoOrgDeclared — each unregistered accept voter counted as singleton.
//  6. SameOrg_MultipleAccepts — same org, multiple voters, all accept → count = 1.
//  7. Disabled_CountStoredNoEvent — BridgingMinOrgs=0 → count stored, no event.
//  8. ThresholdMet_EventEmitted — count >= BridgingMinOrgs → event with correct attrs.
//  9. ThresholdNotMet_NoEvent — count < BridgingMinOrgs → no event, count still stored.
// 10. UnknownFact_NoOp — factId that doesn't exist → no panic.

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// makeBridgingReveal creates a RevealEntry with the given verifier and vote.
func makeBridgingReveal(verifier, vote string) *types.RevealEntry {
	return &types.RevealEntry{
		Verifier: verifier,
		Vote:     vote,
	}
}

// setBridgingOrg sets the organization_id on the verifier's AgentCalibration.
func setBridgingOrg(t *testing.T, k keeper.Keeper, ctx sdk.Context, verifier, orgID string) {
	t.Helper()
	require.NoError(t, k.SetVerifierOrganization(ctx, verifier, orgID))
}

// makeBridgingFact creates and stores a minimal accepted fact, returns its ID.
func makeBridgingFact(t *testing.T, k keeper.Keeper, ctx sdk.Context, id string) string {
	t.Helper()
	fact := &types.Fact{
		Id:        id,
		Content:   "bridging test fact " + id,
		Domain:    "cosmology",
		Category:  "test",
		Submitter: makeValidBech32Addr("br-sub-" + id),
		Status:    types.FactStatus_FACT_STATUS_ACTIVE,
	}
	require.NoError(t, k.SetFact(ctx, fact))
	return id
}

// setBridgingMinOrgs sets Params.BridgingMinOrgs.
func setBridgingMinOrgs(t *testing.T, k keeper.Keeper, ctx sdk.Context, minOrgs uint64) {
	t.Helper()
	params, err := k.GetParams(ctx)
	require.NoError(t, err)
	params.BridgingMinOrgs = minOrgs
	require.NoError(t, k.SetParams(ctx, params))
}

// hasBridgingEvent returns (found, attrMap) for zerone.knowledge.bridging_claim_accepted.
func hasBridgingEvent(ctx sdk.Context) (bool, map[string]string) {
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == "zerone.knowledge.bridging_claim_accepted" {
			attrs := make(map[string]string)
			for _, a := range ev.Attributes {
				attrs[a.Key] = a.Value
			}
			return true, attrs
		}
	}
	return false, nil
}

// ── Pure unit tests: ComputeBridgingOrgCount ─────────────────────────────────

// TestBridging_NoAcceptVotes verifies that a round with no accept votes yields 0.
func TestBridging_NoAcceptVotes(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verA := makeValidBech32Addr("br-a")
	verB := makeValidBech32Addr("br-b")
	setBridgingOrg(t, k, ctx, verA, "org-alpha")
	setBridgingOrg(t, k, ctx, verB, "org-beta")

	round := &types.VerificationRound{
		Id:      "round-no-accepts",
		ClaimId: "claim-x",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(verA, "reject"),
			makeBridgingReveal(verB, "reject"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(0), count, "no accept votes → bridging_org_count must be 0")
}

// TestBridging_SingleOrgAllAccept verifies that multiple acceptors from the same
// org count as 1 distinct organization.
func TestBridging_SingleOrgAllAccept(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	ver1 := makeValidBech32Addr("br-s1")
	ver2 := makeValidBech32Addr("br-s2")
	ver3 := makeValidBech32Addr("br-s3")
	setBridgingOrg(t, k, ctx, ver1, "monolith-inc")
	setBridgingOrg(t, k, ctx, ver2, "monolith-inc")
	setBridgingOrg(t, k, ctx, ver3, "monolith-inc")

	round := &types.VerificationRound{
		Id:      "round-single-org",
		ClaimId: "claim-mono",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
			makeBridgingReveal(ver2, "accept"),
			makeBridgingReveal(ver3, "accept"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(1), count,
		"three acceptors from same org → bridging_org_count = 1")
}

// TestBridging_TwoDistinctOrgs verifies that two accept voters from different
// orgs yield count = 2.
func TestBridging_TwoDistinctOrgs(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verA := makeValidBech32Addr("br-da")
	verB := makeValidBech32Addr("br-db")
	setBridgingOrg(t, k, ctx, verA, "org-east")
	setBridgingOrg(t, k, ctx, verB, "org-west")

	round := &types.VerificationRound{
		Id:      "round-two-orgs",
		ClaimId: "claim-bridge",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(verA, "accept"),
			makeBridgingReveal(verB, "accept"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(2), count,
		"two distinct orgs both voted accept → bridging_org_count = 2")
}

// TestBridging_OnlyAcceptVotersCounted verifies that reject voter organizations
// are NOT included in the bridging count, even if they are from a distinct org.
func TestBridging_OnlyAcceptVotersCounted(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verAccept := makeValidBech32Addr("br-acc")
	verReject := makeValidBech32Addr("br-rej")
	setBridgingOrg(t, k, ctx, verAccept, "org-alpha")
	setBridgingOrg(t, k, ctx, verReject, "org-beta") // distinct org but votes reject

	round := &types.VerificationRound{
		Id:      "round-mixed-votes",
		ClaimId: "claim-mixed",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(verAccept, "accept"),
			makeBridgingReveal(verReject, "reject"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(1), count,
		"only accept voter counted; reject voter's distinct org does not inflate bridging count")
}

// TestBridging_NoOrgDeclared verifies that verifiers without a declared org are
// each treated as unique singletons, contributing 1 to the count apiece.
func TestBridging_NoOrgDeclared(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	// Three verifiers, none with org declared.
	ver1 := makeValidBech32Addr("br-anon1")
	ver2 := makeValidBech32Addr("br-anon2")
	ver3 := makeValidBech32Addr("br-anon3")

	round := &types.VerificationRound{
		Id:      "round-no-org",
		ClaimId: "claim-anon",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
			makeBridgingReveal(ver2, "accept"),
			makeBridgingReveal(ver3, "accept"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(3), count,
		"three unregistered accept voters each count as a unique singleton → 3")
}

// ── Integration tests: recordBridgingOrgCount ────────────────────────────────

// TestBridging_Disabled_CountStoredNoEvent verifies that when BridgingMinOrgs=0,
// the bridging_org_count is still computed and stored on the fact, but no event
// is emitted.
func TestBridging_Disabled_CountStoredNoEvent(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeBridgingFact(t, k, ctx, "fact-disabled-bridge")
	setBridgingMinOrgs(t, k, ctx, 0) // disabled

	ver1 := makeValidBech32Addr("br-d1")
	ver2 := makeValidBech32Addr("br-d2")
	setBridgingOrg(t, k, ctx, ver1, "org-A")
	setBridgingOrg(t, k, ctx, ver2, "org-B")

	round := &types.VerificationRound{
		Id:      "round-disabled",
		ClaimId: "claim-disabled",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
			makeBridgingReveal(ver2, "accept"),
		},
	}

	keeper.RecordBridgingOrgCountForTest(k, ctx, factID, round)

	// Count should be stored.
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(2), fact.BridgingOrgCount,
		"bridging_org_count must be stored even when BridgingMinOrgs=0")

	// No event emitted.
	evFound, _ := hasBridgingEvent(ctx)
	require.False(t, evFound, "BridgingMinOrgs=0: no event emitted")
}

// TestBridging_ThresholdMet_EventEmitted verifies that when bridging_org_count
// meets the threshold, zerone.knowledge.bridging_claim_accepted is emitted with
// the correct attributes.
func TestBridging_ThresholdMet_EventEmitted(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeBridgingFact(t, k, ctx, "fact-bridging-event")
	setBridgingMinOrgs(t, k, ctx, 2) // require 2 distinct orgs

	ver1 := makeValidBech32Addr("br-ev1")
	ver2 := makeValidBech32Addr("br-ev2")
	setBridgingOrg(t, k, ctx, ver1, "org-north")
	setBridgingOrg(t, k, ctx, ver2, "org-south")

	round := &types.VerificationRound{
		Id:      "round-event",
		ClaimId: "claim-event",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
			makeBridgingReveal(ver2, "accept"),
		},
	}

	keeper.RecordBridgingOrgCountForTest(k, ctx, factID, round)

	// Fact stored.
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(2), fact.BridgingOrgCount)

	// Event emitted with correct attributes.
	evFound, attrs := hasBridgingEvent(ctx)
	require.True(t, evFound, "bridging_org_count >= min_orgs → event must be emitted")
	require.Equal(t, factID, attrs["fact_id"])
	require.Equal(t, "claim-event", attrs["claim_id"])
	require.Equal(t, "2", attrs["bridging_org_count"])
	require.Equal(t, "2", attrs["accept_voter_count"])
}

// TestBridging_ThresholdNotMet_NoEvent verifies that when bridging_org_count is
// below the threshold, the count is stored but no event is emitted.
func TestBridging_ThresholdNotMet_NoEvent(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factID := makeBridgingFact(t, k, ctx, "fact-below-threshold")
	setBridgingMinOrgs(t, k, ctx, 3) // require 3 distinct orgs

	ver1 := makeValidBech32Addr("br-bt1")
	ver2 := makeValidBech32Addr("br-bt2")
	setBridgingOrg(t, k, ctx, ver1, "org-A")
	setBridgingOrg(t, k, ctx, ver2, "org-B")

	round := &types.VerificationRound{
		Id:      "round-below",
		ClaimId: "claim-below",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
			makeBridgingReveal(ver2, "accept"),
		},
	}

	keeper.RecordBridgingOrgCountForTest(k, ctx, factID, round)

	// Count stored.
	fact, found := k.GetFact(ctx, factID)
	require.True(t, found)
	require.Equal(t, uint64(2), fact.BridgingOrgCount,
		"count stored even when below threshold")

	// No event.
	evFound, _ := hasBridgingEvent(ctx)
	require.False(t, evFound, "bridging_org_count 2 < min_orgs 3: no event")
}

// TestBridging_UnknownFact_NoOp verifies that a non-existent factId causes
// recordBridgingOrgCount to silently no-op without panicking.
func TestBridging_UnknownFact_NoOp(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setBridgingMinOrgs(t, k, ctx, 1)

	ver1 := makeValidBech32Addr("br-ghost")
	setBridgingOrg(t, k, ctx, ver1, "org-ghost")

	round := &types.VerificationRound{
		Id:      "round-ghost",
		ClaimId: "claim-ghost",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(ver1, "accept"),
		},
	}

	// Must not panic.
	require.NotPanics(t, func() {
		keeper.RecordBridgingOrgCountForTest(k, ctx, "fact-does-not-exist", round)
	})

	// No event.
	evFound, _ := hasBridgingEvent(ctx)
	require.False(t, evFound)
}

// TestBridging_MixedAcceptRejectMalformed verifies that only "accept" votes
// count toward bridging — "reject" and "malformed" are excluded.
func TestBridging_MixedVoteTypes(t *testing.T) {
	k, ctx := setupKnowledgeTest(t)

	verAccept1 := makeValidBech32Addr("br-mv1")
	verAccept2 := makeValidBech32Addr("br-mv2")
	verReject := makeValidBech32Addr("br-mv3")
	verMalformed := makeValidBech32Addr("br-mv4")

	setBridgingOrg(t, k, ctx, verAccept1, "org-P")
	setBridgingOrg(t, k, ctx, verAccept2, "org-Q")
	setBridgingOrg(t, k, ctx, verReject, "org-R")   // distinct, but votes reject
	setBridgingOrg(t, k, ctx, verMalformed, "org-S") // distinct, but votes malformed

	round := &types.VerificationRound{
		Id:      "round-mixed-types",
		ClaimId: "claim-mixed-types",
		Reveals: []*types.RevealEntry{
			makeBridgingReveal(verAccept1, "accept"),
			makeBridgingReveal(verAccept2, "accept"),
			makeBridgingReveal(verReject, "reject"),
			makeBridgingReveal(verMalformed, "malformed"),
		},
	}

	count := k.ComputeBridgingOrgCount(ctx, round)
	require.Equal(t, uint64(2), count,
		"only accept voters (orgs P+Q) counted; reject+malformed excluded")
}
