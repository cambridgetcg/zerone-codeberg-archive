package keeper_test

// Falsificationism Gap 2 — Ad-Hoc Rescue Detection.
//
// Popper's criterion (from Section 9 of the falsificationism brief):
//
//   "A modification is legitimate if it generates new, independently testable
//   predictions. A modification is illegitimate (ad-hoc) if it exists solely
//   to save the theory from a specific falsification without increasing
//   overall testability."
//
// When a submitter rescues a previously REJECTED claim, they declare
// MsgSubmitClaim.rescues_claim_id. The chain checks whether the new claim's
// RefutationConditions are a proper superset of the original's:
//   - Proper superset: new commits to everything original did, PLUS at least
//     one new falsifiability condition → legitimate rescue.
//   - Same set, subset, or non-superset: ad-hoc → IsPotentiallyAdHoc=true.
//
// The detection is informational and non-blocking. The chain emits
// zerone.knowledge.ad_hoc_rescue_detected when ad-hoc detected.
//
// Tests (unit — CheckAdHocRescue):
//   1. Legitimate rescue adds new conditions → not ad-hoc
//   2. Same conditions as original → ad-hoc
//   3. Fewer conditions than original → ad-hoc
//   4. Drops original condition even with others → ad-hoc
//   5. Original had zero conditions, new has some → legitimate (expansion)
//   6. Original had zero conditions, new has zero → ad-hoc
//   7. Rescue target was ACCEPTED (not rejected) → not ad-hoc
//   8. Rescue target does not exist → not ad-hoc (stale ref, non-error)
//   9. Empty rescues_claim_id → not ad-hoc (disabled)
//
// Integration tests (MsgServer.SubmitClaim):
//   10. Ad-hoc rescue: IsPotentiallyAdHoc=true stored on claim
//   11. Legitimate rescue: IsPotentiallyAdHoc=false stored on claim
//   12. No rescue declared: IsPotentiallyAdHoc=false and RescuesClaimId=""
//   13. RescuesClaimId propagated from msg to Claim

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// setRejectedClaim stores a claim with REJECTED status and given refutation conditions.
func setRejectedClaim(
	t *testing.T,
	k keeper.Keeper,
	ctx sdk.Context,
	id string,
	conditions []string,
) {
	t.Helper()
	claim := &types.Claim{
		Id:                   id,
		FactContent:          "rejected theory: " + id,
		Domain:               "physics",
		Category:             "test",
		Submitter:            makeValidBech32Addr("rejected-" + id),
		SubmittedAtBlock:     1,
		Status:               types.ClaimStatus_CLAIM_STATUS_REJECTED,
		Stake:                "1000uzrn",
		RefutationConditions: conditions,
	}
	require.NoError(t, k.SetClaim(ctx, claim))
}

// ── Unit Tests (CheckAdHocRescue) ─────────────────────────────────────────────

func TestAdHocRescue_LegitimateAddsNewConditions(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-legit", []string{"A", "B"})

	// New claim adds condition "C" → proper superset {A, B, C} ⊃ {A, B}
	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-legit", []string{"A", "B", "C"})
	require.False(t, adHoc, "proper superset of original conditions = legitimate rescue")
}

func TestAdHocRescue_SameConditionsIsAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-same", []string{"A", "B"})

	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-same", []string{"A", "B"})
	require.True(t, adHoc, "same conditions as original = ad-hoc (no new falsifiability)")
}

func TestAdHocRescue_FewerConditionsIsAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-fewer", []string{"A", "B", "C"})

	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-fewer", []string{"A", "B"})
	require.True(t, adHoc, "fewer conditions than original = ad-hoc")
}

func TestAdHocRescue_DropsOriginalConditionIsAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-drops", []string{"A", "B"})

	// New has 3 conditions but dropped "B" — not a superset
	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-drops", []string{"A", "C", "D"})
	require.True(t, adHoc, "dropping an original condition is ad-hoc even with new ones added")
}

func TestAdHocRescue_OriginalZeroConditions_NewHasSome_Legitimate(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-zero", nil) // no conditions originally

	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-zero", []string{"X", "Y"})
	require.False(t, adHoc, "original had no conditions; new adds some = legitimate (any expansion)")
}

func TestAdHocRescue_BothZeroConditions_AdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	setRejectedClaim(t, k, ctx, "orig-both-zero", nil)

	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-both-zero", nil)
	require.True(t, adHoc, "both have zero conditions = ad-hoc (no falsifiability expansion)")
}

func TestAdHocRescue_TargetAccepted_NotAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Store an ACCEPTED claim (not rejected — no rescue of a failure)
	accepted := &types.Claim{
		Id:               "orig-accepted",
		FactContent:      "accepted theory",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("accepted-sub"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_ACCEPTED,
		Stake:            "1000uzrn",
		RefutationConditions: []string{"A"},
	}
	require.NoError(t, k.SetClaim(ctx, accepted))

	// Rescue of an ACCEPTED claim should not flag as ad-hoc.
	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "orig-accepted", []string{"A"})
	require.False(t, adHoc, "rescued claim was not rejected — not an ad-hoc rescue scenario")
}

func TestAdHocRescue_TargetNotFound_NotAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Reference a claim ID that doesn't exist — stale reference, non-blocking.
	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "nonexistent-claim", []string{"A"})
	require.False(t, adHoc, "stale rescue reference = not ad-hoc (cannot evaluate, non-blocking)")
}

func TestAdHocRescue_EmptyRescuesClaimId_NotAdHoc(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	adHoc := keeper.CheckAdHocRescueForTest(k, ctx, "", []string{"A"})
	require.False(t, adHoc, "empty rescues_claim_id = rescue detection disabled")
}

// ── Integration tests (MsgServer) ─────────────────────────────────────────────

func TestAdHocRescue_IntegrationAdHocDetected(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Step 1: store a rejected claim with RefutationConditions.
	rejected := &types.Claim{
		Id:               "rej-claim-integration",
		FactContent:      "phlogiston is released during combustion",
		Domain:           "chemistry",
		Category:         "thermodynamics",
		Submitter:        makeValidBech32Addr("rej-sub"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_REJECTED,
		Stake:            "1000uzrn",
		RefutationConditions: []string{
			"if oxygen concentration drops, combustion intensity decreases",
		},
	}
	require.NoError(t, k.SetClaim(ctx, rejected))

	// Step 2: submit a rescue that has the SAME conditions (no expansion) → ad-hoc.
	sub := makeValidBech32Addr("rescue-sub")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:    sub,
		FactContent:  "phlogiston is released during combustion (revised)",
		Domain:       "chemistry",
		Category:     "thermodynamics",
		Stake:        "1000000",
		RescuesClaimId: "rej-claim-integration",
		RefutationConditions: []string{
			"if oxygen concentration drops, combustion intensity decreases",
			// Same condition — NOT a superset (identical)
		},
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "rej-claim-integration", stored.RescuesClaimId)
	require.True(t, stored.IsPotentiallyAdHoc,
		"rescue with identical conditions must be flagged as ad-hoc")
}

func TestAdHocRescue_IntegrationLegitimateRescue(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Store a rejected claim.
	rejected := &types.Claim{
		Id:               "rej-claim-legit",
		FactContent:      "Newtonian mechanics exactly describes all motion",
		Domain:           "physics",
		Category:         "mechanics",
		Submitter:        makeValidBech32Addr("rej-sub-l"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_REJECTED,
		Stake:            "1000uzrn",
		RefutationConditions: []string{
			"if velocities approach c, time dilation is predicted by classical mechanics",
		},
	}
	require.NoError(t, k.SetClaim(ctx, rejected))

	// Legitimate rescue: adds new RefutationCondition beyond the original.
	sub := makeValidBech32Addr("rescue-sub-l")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:    sub,
		FactContent:  "Newtonian mechanics is a low-velocity approximation of special relativity",
		Domain:       "physics",
		Category:     "mechanics",
		Stake:        "1000000",
		RescuesClaimId: "rej-claim-legit",
		RefutationConditions: []string{
			"if velocities approach c, time dilation is predicted by classical mechanics",
			// ↑ original condition (must retain)
			"if gravitational fields are strong, Newtonian trajectories diverge from GR predictions",
			// ↑ NEW condition — proper superset → legitimate rescue
		},
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "rej-claim-legit", stored.RescuesClaimId)
	require.False(t, stored.IsPotentiallyAdHoc,
		"rescue with proper superset of conditions must not be flagged as ad-hoc")
}

func TestAdHocRescue_NoRescueDeclared_NoFlag(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sub := makeValidBech32Addr("no-rescue-sub")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:   sub,
		FactContent: "gravity attracts masses",
		Domain:      "physics",
		Category:    "gravitation",
		Stake:       "1000000",
		// RescuesClaimId intentionally absent
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "", stored.RescuesClaimId, "no rescue declared → RescuesClaimId empty")
	require.False(t, stored.IsPotentiallyAdHoc, "no rescue declared → IsPotentiallyAdHoc=false")
}

func TestAdHocRescue_RescuesClaimIdPropagated(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	// Store a rejected claim.
	rejected := &types.Claim{
		Id:               "rej-claim-prop",
		FactContent:      "hypothesis that was rejected",
		Domain:           "biology",
		Category:         "genetics",
		Submitter:        makeValidBech32Addr("rej-sub-p"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_REJECTED,
		Stake:            "1000uzrn",
		RefutationConditions: []string{"one condition"},
	}
	require.NoError(t, k.SetClaim(ctx, rejected))

	sub := makeValidBech32Addr("rescue-sub-p")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:    sub,
		FactContent:  "improved hypothesis for the genetics domain",
		Domain:       "biology",
		Category:     "genetics",
		Stake:        "1000000",
		RescuesClaimId: "rej-claim-prop",
		RefutationConditions: []string{"one condition", "a new second condition"},
	})
	require.NoError(t, err)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "rej-claim-prop", stored.RescuesClaimId,
		"RescuesClaimId must propagate from MsgSubmitClaim to Claim")
}
