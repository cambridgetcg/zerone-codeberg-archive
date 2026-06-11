package keeper_test

// Structured Deliberation Gap 1 — Toulmin Argument Structure.
//
// Toulmin (1958) decomposed arguments into: Claim (conclusion), Data (grounds),
// Warrant (inferential rule), Backing (support for the warrant), Qualifier
// (degree of certainty), and Rebuttal (conditions under which the warrant fails).
//
// Rebuttal is already covered by RefutationConditions (P-07, field 15).
// This gap adds the missing three:
//   - Claim.warrant       (field 35) / Fact.warrant       (field 77)
//   - Claim.backing       (field 36) / Fact.backing       (field 78)
//   - Claim.qualifier_bps (field 37) / Fact.qualifier_bps (field 79)
//
// Making warrants machine-readable enables Dung-style argument-graph attacks:
// a challenger can target the inferential bridge, not just the conclusion.
//
// Tests:
//   1.  WarrantStoredOnClaim:          warrant persists via SetClaim/GetClaim
//   2.  BackingStoredOnClaim:          backing persists via SetClaim/GetClaim
//   3.  QualifierBpsStoredOnClaim:     qualifier_bps persists via SetClaim/GetClaim
//   4.  AllFieldsRoundTrip:            all three fields round-trip together
//   5.  EmptyFieldsBackwardCompat:     empty warrant/backing/0 qualifier_bps is valid
//   6.  WarrantPropagatesOnAccept:     warrant copies to Fact on ACCEPT verdict
//   7.  BackingPropagatesOnAccept:     backing copies to Fact on ACCEPT verdict
//   8.  QualifierBpsPropagatesOnAccept: qualifier_bps copies to Fact on ACCEPT
//   9.  AllFieldsPropagateOnAccept:    all three propagate together
//   10. RejectDoesNotCreateFact:       REJECT verdict creates no Fact
//   11. SubmitClaimWiresWarrant:       MsgServer.SubmitClaim wires all three fields
//   12. QualifierBps_MaxValue:         qualifier_bps = 1_000_000 is valid
//   13. QualifierBps_Zero:             qualifier_bps = 0 is valid

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zerone-chain/zerone/x/knowledge/keeper"
	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// ── helper ───────────────────────────────────────────────────────────────────

func makeToulminClaim(id, content, warrant, backing string, qualBps uint64) *types.Claim {
	return &types.Claim{
		Id:               id,
		FactContent:      content,
		Domain:           "physics",
		Category:         "mechanics",
		Submitter:        makeValidBech32Addr("toulmin-sub"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		Warrant:          warrant,
		Backing:          backing,
		QualifierBps:     qualBps,
	}
}

// ── Test 1: WarrantStoredOnClaim ─────────────────────────────────────────────

func TestToulmin_WarrantStoredOnClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-warrant",
		"objects fall at equal rates regardless of mass",
		"Galileo's equivalence principle holds for all uniform gravitational fields",
		"",
		0,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-warrant")
	require.True(t, found)
	require.Equal(t, "Galileo's equivalence principle holds for all uniform gravitational fields", stored.Warrant)
}

// ── Test 2: BackingStoredOnClaim ─────────────────────────────────────────────

func TestToulmin_BackingStoredOnClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-backing",
		"E = mc²",
		"mass-energy equivalence follows from special relativity",
		"Einstein (1905), Annalen der Physik; confirmed by nuclear fission experiments",
		0,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-backing")
	require.True(t, found)
	require.Equal(t, "Einstein (1905), Annalen der Physik; confirmed by nuclear fission experiments", stored.Backing)
}

// ── Test 3: QualifierBpsStoredOnClaim ───────────────────────────────────────

func TestToulmin_QualifierBpsStoredOnClaim(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-qualifier",
		"dark matter accounts for 85% of all matter",
		"galactic rotation curves deviate from predictions without unseen mass",
		"",
		750_000, // 75% — strong probability but not certainty
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-qualifier")
	require.True(t, found)
	require.Equal(t, uint64(750_000), stored.QualifierBps)
}

// ── Test 4: AllFieldsRoundTrip ───────────────────────────────────────────────

func TestToulmin_AllFieldsRoundTrip(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-full-toulmin",
		"climate change is primarily caused by anthropogenic CO2 emissions",
		"radiative forcing from greenhouse gases correlates with observed temperature anomalies",
		"IPCC AR6 (2021); Arrhenius (1896); 97% scientific consensus meta-studies",
		950_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-full-toulmin")
	require.True(t, found)
	require.Equal(t, claim.Warrant, stored.Warrant)
	require.Equal(t, claim.Backing, stored.Backing)
	require.Equal(t, claim.QualifierBps, stored.QualifierBps)
}

// ── Test 5: EmptyFieldsBackwardCompat ───────────────────────────────────────

func TestToulmin_EmptyFieldsBackwardCompat(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	// Legacy claim with no Toulmin fields — must be valid and store without error.
	claim := &types.Claim{
		Id:               "legacy-claim-toulmin",
		FactContent:      "water is H2O",
		Domain:           "chemistry",
		Category:         "composition",
		Submitter:        makeValidBech32Addr("legacy-sub-t"),
		SubmittedAtBlock: 1,
		Status:           types.ClaimStatus_CLAIM_STATUS_PENDING,
		Stake:            "1000uzrn",
		// Warrant, Backing, QualifierBps all zero/empty
	}
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "legacy-claim-toulmin")
	require.True(t, found)
	require.Equal(t, "", stored.Warrant, "empty warrant is valid (optional field)")
	require.Equal(t, "", stored.Backing, "empty backing is valid (optional field)")
	require.Equal(t, uint64(0), stored.QualifierBps, "zero qualifier_bps is valid (speculation)")
}

// ── Tests 6–9: Claim → Fact propagation on ACCEPT verdict ────────────────────

func TestToulmin_WarrantPropagatesOnAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-wpa",
		"momentum is conserved in elastic collisions",
		"Newton's third law applies symmetrically to interacting bodies",
		"",
		900_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-wpa",
		ClaimId:             "claim-wpa",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 800_000}
	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found bool
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == "momentum is conserved in elastic collisions" {
			require.Equal(t, "Newton's third law applies symmetrically to interacting bodies", f.Warrant,
				"warrant must propagate from Claim to Fact on ACCEPT")
			found = true
			return true
		}
		return false
	})
	require.True(t, found, "accepted Fact must exist")
}

func TestToulmin_BackingPropagatesOnAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-bpa",
		"the speed of light is constant in all inertial frames",
		"Maxwell's equations are Lorentz-invariant",
		"Michelson-Morley (1887); special relativity confirmed by particle accelerators",
		1_000_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-bpa",
		ClaimId:             "claim-bpa",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 950_000}
	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found bool
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == "the speed of light is constant in all inertial frames" {
			require.Equal(t, "Michelson-Morley (1887); special relativity confirmed by particle accelerators", f.Backing)
			found = true
			return true
		}
		return false
	})
	require.True(t, found)
}

func TestToulmin_QualifierBpsPropagatesOnAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-qpa",
		"quantum entanglement enables non-local correlations",
		"Bell's theorem rules out local hidden-variable theories",
		"",
		850_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-qpa",
		ClaimId:             "claim-qpa",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 800_000}
	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found bool
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == "quantum entanglement enables non-local correlations" {
			require.Equal(t, uint64(850_000), f.QualifierBps)
			found = true
			return true
		}
		return false
	})
	require.True(t, found)
}

func TestToulmin_AllFieldsPropagateOnAccept(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-all-propagate",
		"entropy never decreases in an isolated system",
		"statistical mechanics: macrostates with more microstates are overwhelmingly probable",
		"Boltzmann H-theorem; Clausius (1865); confirmed by calorimetry across 150 years",
		990_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-all-propagate",
		ClaimId:             "claim-all-propagate",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_ACCEPT, Confidence: 980_000}
	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found bool
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == "entropy never decreases in an isolated system" {
			require.Equal(t, claim.Warrant, f.Warrant, "warrant must propagate")
			require.Equal(t, claim.Backing, f.Backing, "backing must propagate")
			require.Equal(t, claim.QualifierBps, f.QualifierBps, "qualifier_bps must propagate")
			found = true
			return true
		}
		return false
	})
	require.True(t, found)
}

// ── Test 10: REJECT does not propagate ───────────────────────────────────────

func TestToulmin_RejectDoesNotCreateFact(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-rejected-t",
		"phlogiston is released during combustion",
		"phlogiston theory accounts for combustion in all known materials",
		"Becher (1667), Stahl (1703)",
		200_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	round := &types.VerificationRound{
		Id:                  "round-rejected-t",
		ClaimId:             "claim-rejected-t",
		StartedAtBlock:      1,
		Phase:               types.VerificationPhase_VERIFICATION_PHASE_AGGREGATION,
		CommitDeadline:      100,
		RevealDeadline:      200,
		AggregationDeadline: 300,
	}
	require.NoError(t, k.SetVerificationRound(ctx, round))

	result := &keeper.VerificationResult{Verdict: types.Verdict_VERDICT_REJECT, Confidence: 950_000}
	advCtx := ctx.WithBlockHeight(400)
	require.NoError(t, k.CompleteRound(advCtx, round, result))

	var found bool
	k.IterateFacts(advCtx, func(f *types.Fact) bool {
		if f.Content == "phlogiston is released during combustion" {
			found = true
			return true
		}
		return false
	})
	require.False(t, found, "REJECT should not create a Fact")
}

// ── Test 11: MsgServer.SubmitClaim wires all three fields ────────────────────

func TestToulmin_SubmitClaimWiresAllFields(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)
	ms := keeper.NewMsgServerImpl(k)

	sub := makeValidBech32Addr("toulmin-msgserver")
	resp, err := ms.SubmitClaim(ctx, &types.MsgSubmitClaim{
		Submitter:    sub,
		FactContent:  "general relativity predicts gravitational time dilation",
		Domain:       "physics",
		Category:     "gravitation",
		Stake:        "1000000",
		Warrant:      "mass-energy tensor determines spacetime geometry via Einstein field equations",
		Backing:      "GPS clock corrections; Pound-Rebka (1959); LIGO (2015)",
		QualifierBps: 995_000,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClaimId)

	stored, found := k.GetClaim(ctx, resp.ClaimId)
	require.True(t, found)
	require.Equal(t, "mass-energy tensor determines spacetime geometry via Einstein field equations", stored.Warrant)
	require.Equal(t, "GPS clock corrections; Pound-Rebka (1959); LIGO (2015)", stored.Backing)
	require.Equal(t, uint64(995_000), stored.QualifierBps)
}

// ── Test 12: QualifierBps = 1_000_000 (certainty) ───────────────────────────

func TestToulmin_QualifierBps_MaxValue(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-certain-t",
		"a statement cannot be both true and false simultaneously",
		"law of non-contradiction (classical logic)",
		"Aristotle, Metaphysics Γ; Frege, Begriffsschrift",
		1_000_000,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-certain-t")
	require.True(t, found)
	require.Equal(t, uint64(1_000_000), stored.QualifierBps)
}

// ── Test 13: QualifierBps = 0 (pure speculation) ────────────────────────────

func TestToulmin_QualifierBps_Zero(t *testing.T) {
	k, ctx, _ := setupKnowledgeTestWithBank(t)

	claim := makeToulminClaim(
		"claim-speculative-t",
		"there exist parallel universes",
		"many-worlds interpretation of quantum mechanics implies branching",
		"Everett (1957); Deutsch (1985)",
		0,
	)
	require.NoError(t, k.SetClaim(ctx, claim))

	stored, found := k.GetClaim(ctx, "claim-speculative-t")
	require.True(t, found)
	require.Equal(t, uint64(0), stored.QualifierBps, "qualifier_bps=0 is valid (speculation)")
}
