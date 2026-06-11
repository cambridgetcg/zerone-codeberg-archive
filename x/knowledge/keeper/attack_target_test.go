package keeper_test

// Structured Deliberation Gap 2 — Dung Attack Target.
//
// Dung (1995) argumentation framework: AF = ⟨A, attacks⟩. To enable formal
// AF solvers to distinguish WHICH component of a Toulmin argument is being
// challenged, CONTRADICTS relations now carry an AttackTargetType:
//
//   UNSPECIFIED (0) — general contradiction; no specific Toulmin component
//   CONCLUSION  (1) — attacks the main claim/conclusion directly
//   WARRANT     (2) — attacks the inferential bridge (Toulmin Gap 1)
//   BACKING     (3) — attacks the justification behind the warrant
//   DATA        (4) — attacks the evidence/grounds of the argument
//   QUALIFIER   (5) — disputes the stated certainty level (qualifier_bps)
//
// AttackTargetType is added to both FactRelation (field 9) and ClaimRelation
// (field 6). It is only semantically meaningful when relation =
// RELATION_TYPE_CONTRADICTS; it is stored (not validated against relation
// type) — ignored by the chain for non-CONTRADICTS edges.
//
// Relations are stored separately via SetFactRelation (indexed by KV) and
// retrieved via GetFactRelations / GetIncomingRelations. ClaimRelation is
// embedded inside the Claim message and retrieved via GetClaim.
//
// Tests:
//   1.  FactRelation stores UNSPECIFIED by default
//   2.  FactRelation stores CONCLUSION attack
//   3.  FactRelation stores WARRANT attack
//   4.  FactRelation stores BACKING attack
//   5.  FactRelation stores DATA attack
//   6.  FactRelation stores QUALIFIER attack
//   7.  ClaimRelation stores all five named values (round-trip via GetClaim)
//   8.  AttackTargetType survives Claim.Relations round-trip via KV proto
//   9.  Non-CONTRADICTS relation stores WARRANT without error (permissive)
//  10.  Two distinct attack-targeted relations on one source Fact are independent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── Tests ─────────────────────────────────────────────────────────────────────

// Test 1: FactRelation stores UNSPECIFIED attack target (zero value).
func TestAttackTarget_FactRelation_DefaultUnspecified(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-unspec-a", "source fact for unspecified attack target test", "physics", "empirical", makeValidBech32Addr("sub-unspec-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-unspec-b", "target fact for unspecified attack target test", "physics", "empirical", makeValidBech32Addr("sub-unspec-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-unspec-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_UNSPECIFIED,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-unspec"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_UNSPECIFIED, outgoing[0].AttackTarget)
}

// Test 2: FactRelation stores CONCLUSION attack target.
func TestAttackTarget_FactRelation_Conclusion(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-conc-a", "source fact conclusion attack test", "physics", "empirical", makeValidBech32Addr("sub-conc-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-conc-b", "target fact conclusion attack test", "physics", "empirical", makeValidBech32Addr("sub-conc-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-conc-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_CONCLUSION,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-conc"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_CONCLUSION, outgoing[0].AttackTarget)
}

// Test 3: FactRelation stores WARRANT attack target.
func TestAttackTarget_FactRelation_Warrant(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-warr-a", "source fact warrant attack test", "physics", "empirical", makeValidBech32Addr("sub-warr-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-warr-b", "target fact warrant attack test", "physics", "empirical", makeValidBech32Addr("sub-warr-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-warr-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-warr"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT, outgoing[0].AttackTarget)
}

// Test 4: FactRelation stores BACKING attack target.
func TestAttackTarget_FactRelation_Backing(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-back-a", "source fact backing attack test", "physics", "empirical", makeValidBech32Addr("sub-back-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-back-b", "target fact backing attack test", "physics", "empirical", makeValidBech32Addr("sub-back-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-back-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_BACKING,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-back"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_BACKING, outgoing[0].AttackTarget)
}

// Test 5: FactRelation stores DATA attack target.
func TestAttackTarget_FactRelation_Data(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-data-a", "source fact data attack test", "physics", "empirical", makeValidBech32Addr("sub-data-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-data-b", "target fact data attack test", "physics", "empirical", makeValidBech32Addr("sub-data-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-data-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_DATA,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-data"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_DATA, outgoing[0].AttackTarget)
}

// Test 6: FactRelation stores QUALIFIER attack target.
func TestAttackTarget_FactRelation_Qualifier(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-qual-a", "source fact qualifier attack test", "physics", "empirical", makeValidBech32Addr("sub-qual-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-qual-b", "target fact qualifier attack test", "physics", "empirical", makeValidBech32Addr("sub-qual-b"), 800_000)

	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-qual-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_QUALIFIER,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-qual"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_QUALIFIER, outgoing[0].AttackTarget)
}

// Test 7: ClaimRelation stores all five named AttackTargetType values (round-trip via GetClaim).
func TestAttackTarget_ClaimRelation_AllValues(t *testing.T) {
	cases := []struct {
		name   string
		target types.AttackTargetType
	}{
		{"unspecified", types.AttackTargetType_ATTACK_TARGET_TYPE_UNSPECIFIED},
		{"conclusion", types.AttackTargetType_ATTACK_TARGET_TYPE_CONCLUSION},
		{"warrant", types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT},
		{"backing", types.AttackTargetType_ATTACK_TARGET_TYPE_BACKING},
		{"data", types.AttackTargetType_ATTACK_TARGET_TYPE_DATA},
		{"qualifier", types.AttackTargetType_ATTACK_TARGET_TYPE_QUALIFIER},
	}

	k, ctx, _ := setupKnowledgeTestWithBank(t)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			claimID := "claim-cr-" + tc.name
			claim := &types.Claim{
				Id:               claimID,
				FactContent:      "claim relation attack target test for " + tc.name + " value",
				Domain:           "physics",
				Category:         "test",
				Submitter:        makeValidBech32Addr("cr-" + tc.name),
				SubmittedAtBlock: 1,
				Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
				Stake:            "1000uzrn",
				Relations: []*types.ClaimRelation{
					{
						TargetFactId: "target-fact-" + tc.name,
						Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS,
						AttackTarget: tc.target,
					},
				},
			}
			require.NoError(t, k.SetClaim(ctx, claim))

			stored, found := k.GetClaim(ctx, claimID)
			require.True(t, found)
			require.Len(t, stored.Relations, 1)
			require.Equal(t, tc.target, stored.Relations[0].AttackTarget,
				"ClaimRelation.AttackTarget must round-trip through KV for %s", tc.name)
		})
	}
}

// Test 8: AttackTargetType survives Claim with two Relations — each preserves its own target.
func TestAttackTarget_ClaimTwoRelations_ProtoRoundTrip(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claimID := "claim-two-cr"
	claim := &types.Claim{
		Id:               claimID,
		FactContent:      "claim with two claim relations each attacking a different Toulmin component",
		Domain:           "physics",
		Category:         "test",
		Submitter:        makeValidBech32Addr("two-cr-sub"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Relations: []*types.ClaimRelation{
			{
				TargetFactId: "rt-target-conclusion",
				Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS,
				AttackTarget: types.AttackTargetType_ATTACK_TARGET_TYPE_CONCLUSION,
			},
			{
				TargetFactId: "rt-target-warrant",
				Relation:     types.RelationType_RELATION_TYPE_CONTRADICTS,
				AttackTarget: types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT,
			},
		},
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, claimID)
	require.True(t, found)
	require.Len(t, stored.Relations, 2)
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_CONCLUSION, stored.Relations[0].AttackTarget,
		"first ClaimRelation must preserve CONCLUSION attack target")
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT, stored.Relations[1].AttackTarget,
		"second ClaimRelation must preserve WARRANT attack target")
}

// Test 9: Non-CONTRADICTS FactRelation may carry an attack target without error
// (chain stores permissively — semantic interpretation is left to AF solvers).
func TestAttackTarget_NonContradictsRelation_PermissiveStorage(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-perm-a", "source fact for permissive storage test", "physics", "empirical", makeValidBech32Addr("sub-perm-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-perm-b", "target fact for permissive storage test", "physics", "empirical", makeValidBech32Addr("sub-perm-b"), 800_000)

	// SUPPORTS relation (not CONTRADICTS) carrying WARRANT attack target.
	rel := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-perm-b",
		Relation:       types.RelationType_RELATION_TYPE_SUPPORTS, // not CONTRADICTS
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-perm"),
	}
	require.NoError(t, k.SetFactRelation(ctx, rel))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	// Chain stores without error; semantics are consumer-side.
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT, outgoing[0].AttackTarget,
		"non-CONTRADICTS relation may carry attack_target (permissive storage)")
}

// Test 10: Two distinct attack-targeted FactRelations on one source are independent.
func TestAttackTarget_TwoRelationsOnOneFact_Independent(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	factA := makeTestFact(t, k, ctx, "fact-two-a", "source fact two distinct attacks", "physics", "empirical", makeValidBech32Addr("sub-two-a"), 900_000)
	makeTestFact(t, k, ctx, "fact-two-b", "target A for two distinct attacks test", "physics", "empirical", makeValidBech32Addr("sub-two-b"), 800_000)
	makeTestFact(t, k, ctx, "fact-two-c", "target B for two distinct attacks test", "physics", "empirical", makeValidBech32Addr("sub-two-c"), 750_000)

	// First relation: warrant attack on fact-two-b
	relA := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-two-b",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-two-a"),
	}
	// Second relation: data attack on fact-two-c
	relB := &types.FactRelation{
		SourceFactId:   factA.Id,
		TargetFactId:   "fact-two-c",
		Relation:       types.RelationType_RELATION_TYPE_CONTRADICTS,
		AttackTarget:   types.AttackTargetType_ATTACK_TARGET_TYPE_DATA,
		CreatedAtBlock: uint64(ctx.BlockHeight()),
		Creator:        makeValidBech32Addr("creator-two-b"),
	}
	require.NoError(t, k.SetFactRelation(ctx, relA))
	require.NoError(t, k.SetFactRelation(ctx, relB))

	outgoing, err := k.GetFactRelations(ctx, factA.Id)
	require.NoError(t, err)
	require.Len(t, outgoing, 2)

	// Build a map by target to make assertions order-independent.
	byTarget := make(map[string]types.AttackTargetType)
	for _, r := range outgoing {
		byTarget[r.TargetFactId] = r.AttackTarget
	}
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_WARRANT, byTarget["fact-two-b"],
		"warrant attack on fact-two-b must be preserved independently")
	require.Equal(t, types.AttackTargetType_ATTACK_TARGET_TYPE_DATA, byTarget["fact-two-c"],
		"data attack on fact-two-c must be preserved independently")
}
