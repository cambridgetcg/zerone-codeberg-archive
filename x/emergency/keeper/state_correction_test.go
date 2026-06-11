package keeper_test

// state_correction_test.go — Circuit Breaker Gap 5: Surgical State Recovery.
//
// Tests for the StateCorrectionProposal CRUD, guardian vote mechanics,
// quorum detection, and event emission.

import (
	"testing"

	"github.com/zerone-chain/zerone/x/emergency/types"
)

// testMutations returns a minimal valid mutation slice.
func testMutations() []*types.StateMutation {
	return []*types.StateMutation{
		{
			ModuleName:  "knowledge",
			Key:         []byte("claim/claim-001"),
			OldValue:    []byte("bad-value"),
			NewValue:    []byte("corrected-value"),
			Description: "fix incorrectly stored claim status",
		},
	}
}

// TestStateCorrection_ProposeAndGet verifies a proposal can be stored and retrieved.
func TestStateCorrection_ProposeAndGet(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	proposer := "zrn1guardian1"
	mock.addGuardian(proposer, "1000000000")

	id, err := k.ProposeStateCorrection(ctx, proposer, testMutations(), "fix claim-001 corruption")
	if err != nil {
		t.Fatalf("ProposeStateCorrection: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty proposal ID")
	}

	prop, found := k.GetStateCorrection(ctx, id)
	if !found {
		t.Fatalf("GetStateCorrection: proposal %s not found", id)
	}
	if prop.Status != "pending" {
		t.Errorf("expected status=pending, got %q", prop.Status)
	}
	if len(prop.Mutations) != 1 {
		t.Errorf("expected 1 mutation, got %d", len(prop.Mutations))
	}
	if prop.Proposer != proposer {
		t.Errorf("expected proposer=%s, got %s", proposer, prop.Proposer)
	}
}

// TestStateCorrection_ProposeNoMutations verifies ErrNoMutations is returned
// when an empty mutation slice is submitted.
func TestStateCorrection_ProposeNoMutations(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	mock.addGuardian("zrn1guardian2", "1000000000")

	_, err := k.ProposeStateCorrection(ctx, "zrn1guardian2", nil, "empty")
	if err == nil {
		t.Fatal("expected ErrNoMutations, got nil")
	}
}

// TestStateCorrection_VoteApproved verifies that a single guardian with
// >= HaltQuorum fraction of guardians (100% when 1 guardian) triggers approval.
func TestStateCorrection_VoteApproved(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	guardian := "zrn1guardian3"
	mock.addGuardian(guardian, "1000000000")

	id, err := k.ProposeStateCorrection(ctx, guardian, testMutations(), "single guardian network")
	if err != nil {
		t.Fatalf("ProposeStateCorrection: %v", err)
	}

	quorum, approved, err := k.VoteStateCorrection(ctx, guardian, id, true)
	if err != nil {
		t.Fatalf("VoteStateCorrection: %v", err)
	}
	if !quorum {
		t.Error("expected quorum_reached=true")
	}
	if !approved {
		t.Error("expected correction_approved=true")
	}

	prop, _ := k.GetStateCorrection(ctx, id)
	if prop.Status != "approved" {
		t.Errorf("expected status=approved, got %q", prop.Status)
	}
	if prop.ApprovedAtBlock == 0 {
		t.Error("expected approved_at_block to be set")
	}
}

// TestStateCorrection_VoteNotYetQuorum verifies that when votes fall short of
// quorum, the proposal stays pending and quorumReached=false is returned.
func TestStateCorrection_VoteNotYetQuorum(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	g1 := "zrn1guardian4"
	g2 := "zrn1guardian5"
	mock.addGuardian(g1, "1000000000")
	mock.addGuardian(g2, "1000000000")
	// With 2 guardians and HaltQuorum=600_000 (60%), need ceil(2 * 0.6) = 2 approvals.

	id, _ := k.ProposeStateCorrection(ctx, g1, testMutations(), "two guardian network")

	// First vote — not yet quorum (1/2 = 50% < 60%).
	quorum, approved, err := k.VoteStateCorrection(ctx, g1, id, true)
	if err != nil {
		t.Fatalf("VoteStateCorrection: %v", err)
	}
	if quorum {
		t.Error("expected quorum_reached=false after first of two votes")
	}
	if approved {
		t.Error("expected correction_approved=false")
	}

	prop, _ := k.GetStateCorrection(ctx, id)
	if prop.Status != "pending" {
		t.Errorf("expected status=pending after first vote, got %q", prop.Status)
	}
}

// TestStateCorrection_DuplicateVote verifies that voting twice returns ErrDuplicateVote.
func TestStateCorrection_DuplicateVote(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	guardian := "zrn1guardian6"
	mock.addGuardian(guardian, "1000000000")
	mock.addGuardian("zrn1guardian6b", "1000000000") // second guardian so first vote doesn't immediately approve

	id, _ := k.ProposeStateCorrection(ctx, guardian, testMutations(), "dup-vote test")

	_, _, err := k.VoteStateCorrection(ctx, guardian, id, true)
	if err != nil {
		t.Fatalf("first vote: %v", err)
	}

	_, _, err = k.VoteStateCorrection(ctx, guardian, id, true)
	if err == nil {
		t.Fatal("expected ErrDuplicateVote on second vote, got nil")
	}
}

// TestStateCorrection_NotFound verifies ErrCorrectionNotFound for unknown IDs.
func TestStateCorrection_NotFound(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	mock.addGuardian("zrn1guardian7", "1000000000")

	_, _, err := k.VoteStateCorrection(ctx, "zrn1guardian7", "sc-nonexistent", true)
	if err == nil {
		t.Fatal("expected ErrCorrectionNotFound, got nil")
	}
}

// TestStateCorrection_VoteOnApprovedProposal verifies ErrCorrectionNotPending
// when voting on an already-approved proposal.
func TestStateCorrection_VoteOnApprovedProposal(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	guardian := "zrn1guardian8"
	mock.addGuardian(guardian, "1000000000")

	id, _ := k.ProposeStateCorrection(ctx, guardian, testMutations(), "already approved")
	// Approve it (single guardian = 100% = quorum).
	k.VoteStateCorrection(ctx, guardian, id, true)

	// Try voting again — proposal is no longer pending.
	_, _, err := k.VoteStateCorrection(ctx, guardian, id, true)
	if err == nil {
		t.Fatal("expected ErrCorrectionNotPending on second vote after approval, got nil")
	}
}

// TestStateCorrection_MultipleMutations verifies proposals with several mutations
// are stored and retrieved correctly.
func TestStateCorrection_MultipleMutations(t *testing.T) {
	k, mock, ctx := setupKeeper(t)
	guardian := "zrn1guardian9"
	mock.addGuardian(guardian, "1000000000")

	mutations := []*types.StateMutation{
		{ModuleName: "knowledge", Key: []byte("k1"), OldValue: []byte("a"), NewValue: []byte("b"), Description: "fix 1"},
		{ModuleName: "liquiditypool", Key: []byte("k2"), OldValue: []byte("x"), NewValue: []byte("y"), Description: "fix 2"},
		{ModuleName: "staking", Key: []byte("k3"), OldValue: []byte("p"), NewValue: []byte("q"), Description: "fix 3"},
	}

	id, err := k.ProposeStateCorrection(ctx, guardian, mutations, "multi-mutation correction")
	if err != nil {
		t.Fatalf("ProposeStateCorrection: %v", err)
	}

	prop, found := k.GetStateCorrection(ctx, id)
	if !found {
		t.Fatalf("proposal not found")
	}
	if len(prop.Mutations) != 3 {
		t.Errorf("expected 3 mutations, got %d", len(prop.Mutations))
	}
	if prop.Mutations[1].ModuleName != "liquiditypool" {
		t.Errorf("mutation[1].module_name mismatch: %s", prop.Mutations[1].ModuleName)
	}
}
