package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	emergency "github.com/zerone-chain/zerone/x/emergency"
	"github.com/zerone-chain/zerone/x/emergency/keeper"
	"github.com/zerone-chain/zerone/x/emergency/types"
)

// --- Mock Staking Keeper ---

type mockValidator struct {
	Address    string
	TotalStake string
	Tier       uint32
	IsActive   bool
}

type mockStakingKeeper struct {
	validators []mockValidator
}

func (m *mockStakingKeeper) GetValidator(_ context.Context, addr string) (*types.ValidatorInfo, bool) {
	for _, v := range m.validators {
		if v.Address == addr {
			return &types.ValidatorInfo{
				Address:    v.Address,
				TotalStake: v.TotalStake,
				Tier:       v.Tier,
				IsActive:   v.IsActive,
			}, true
		}
	}
	return nil, false
}

func (m *mockStakingKeeper) GetGuardianValidators(_ context.Context) ([]types.ValidatorInfo, error) {
	var guardians []types.ValidatorInfo
	for _, v := range m.validators {
		if v.Tier == types.TierGuardian && v.IsActive {
			guardians = append(guardians, types.ValidatorInfo{
				Address:    v.Address,
				TotalStake: v.TotalStake,
				Tier:       v.Tier,
				IsActive:   v.IsActive,
			})
		}
	}
	return guardians, nil
}

func (m *mockStakingKeeper) addGuardian(addr, stake string) {
	m.validators = append(m.validators, mockValidator{
		Address:    addr,
		TotalStake: stake,
		Tier:       types.TierGuardian,
		IsActive:   true,
	})
}

func (m *mockStakingKeeper) addNonGuardian(addr, stake string) {
	m.validators = append(m.validators, mockValidator{
		Address:    addr,
		TotalStake: stake,
		Tier:       3, // Scholar
		IsActive:   true,
	})
}

// --- Test Setup ---

func setupKeeper(t *testing.T) (keeper.Keeper, *mockStakingKeeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatalf("failed to load latest version: %v", err)
	}

	ctx := sdk.NewContext(stateStore, cmtproto.Header{Height: 100}, false, log.NewNopLogger()).
		WithBlockTime(time.Now())

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	mock := &mockStakingKeeper{}

	k := keeper.NewKeeper(runtime.NewKVStoreService(storeKey), cdc, "authority", mock)

	// Set default params with lower thresholds for testing.
	params := types.DefaultParams()
	params.MinGuardianStake = "0"        // No min stake for tests
	params.MinDistinctVoters = 1         // Allow single voter for simplicity
	params.CooldownBlocks = 0            // No cooldown for tests
	params.MaxProposalsPerEpoch = 100    // High limit for tests
	params.MaxProposalsPerGuardianPerEpoch = 100
	k.SetParams(ctx, &params)

	return k, mock, ctx
}

// --- Tests ---

func TestGuardianCheck(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	guardian := "zrn1guardian1"
	nonGuardian := "zrn1scholar1"

	mock.addGuardian(guardian, "100000000000")
	mock.addNonGuardian(nonGuardian, "50000000000")

	if !k.IsGuardian(ctx, guardian) {
		t.Error("expected guardian to be recognized")
	}
	if k.IsGuardian(ctx, nonGuardian) {
		t.Error("expected non-guardian to NOT be recognized")
	}
	if k.IsGuardian(ctx, "zrn1unknown") {
		t.Error("expected unknown address to NOT be recognized")
	}
}

func TestHaltCeremonyFullLifecycle(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "50000000000") // 50k ZRN
	mock.addGuardian(g2, "50000000000") // 50k ZRN

	msgSvr := keeper.NewMsgServerImpl(k)

	// 1. Propose halt
	resp, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "security breach detected",
	})
	if err != nil {
		t.Fatalf("ProposeHalt failed: %v", err)
	}
	proposalId := resp.ProposalId
	if proposalId == "" {
		t.Fatal("expected non-empty proposal ID")
	}

	// Status should be halt_voting
	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusHaltVoting {
		t.Fatalf("expected halt_voting, got %s", status)
	}

	// 2. Guardian 1 prevotes yes
	voteResp, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      g1,
		ProposalId: proposalId,
		Approve:    true,
	})
	if err != nil {
		t.Fatalf("VoteHalt (g1 prevote) failed: %v", err)
	}

	// After 50% prevote, not yet quorum (need 75%)
	ceremony, _ := k.GetCeremony(ctx, proposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Logf("phase after g1 prevote: %s", ceremony.Phase)
	}

	// 3. Guardian 2 prevotes yes → should reach 100% > 75% threshold → advance to precommit
	voteResp, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      g2,
		ProposalId: proposalId,
		Approve:    true,
	})
	if err != nil {
		t.Fatalf("VoteHalt (g2 prevote) failed: %v", err)
	}

	ceremony, _ = k.GetCeremony(ctx, proposalId)
	if ceremony.Phase != string(types.PhasePrecommit) {
		t.Fatalf("expected precommit phase, got %s", ceremony.Phase)
	}
	if !voteResp.QuorumReached {
		t.Error("expected quorum_reached to be true after prevote quorum")
	}

	// 4. Guardian 1 precommits
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      g1,
		ProposalId: proposalId,
		Approve:    true,
	})
	if err != nil {
		t.Fatalf("VoteHalt (g1 precommit) failed: %v", err)
	}

	// Check if we need g2 precommit (MinDistinctVoters=1, so g1 alone is enough)
	ceremony, _ = k.GetCeremony(ctx, proposalId)
	if ceremony.Phase == string(types.PhaseFinalized) {
		// Finalized with g1's precommit (50% stake >= 75%? No...)
		// Actually with MinDistinctVoters=1, but 50% < 75% threshold.
		t.Log("ceremony finalized after g1 precommit")
	}

	// 5. Guardian 2 precommits → 100% stake > 75% → finalized
	voteResp, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      g2,
		ProposalId: proposalId,
		Approve:    true,
	})
	if err != nil {
		t.Fatalf("VoteHalt (g2 precommit) failed: %v", err)
	}

	if !voteResp.ChainHalted {
		t.Error("expected chain_halted to be true")
	}

	// Status should now be halted
	status = k.GetEmergencyStatus(ctx)
	if status != types.StatusHalted {
		t.Fatalf("expected halted, got %s", status)
	}

	// Audit log should have entries
	auditLog := k.GetAuditLog(ctx)
	if len(auditLog) == 0 {
		t.Error("expected audit log entries")
	}
}

func TestResumeLifecycle(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	// Set low thresholds and 1 voter minimum
	params := k.GetParams(ctx)
	params.HaltQuorum = 500000   // 50%
	params.ResumeQuorum = 500000 // 50%
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// First halt the chain
	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "test halt",
	})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})

	if k.GetEmergencyStatus(ctx) != types.StatusHalted {
		t.Fatal("expected halted status for resume test setup")
	}

	// Now propose resume
	resumeResp, err := msgSvr.ProposeResume(ctx, &types.MsgProposeResume{
		Proposer: g1,
	})
	if err != nil {
		t.Fatalf("ProposeResume failed: %v", err)
	}

	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusResumeVoting {
		t.Fatalf("expected resume_voting, got %s", status)
	}

	// Vote yes → prevote quorum → precommit
	msgSvr.VoteResume(ctx, &types.MsgVoteResume{Voter: g1, ProposalId: resumeResp.ProposalId, Approve: true})
	// Vote again → should be precommit phase
	vr, err := msgSvr.VoteResume(ctx, &types.MsgVoteResume{Voter: g1, ProposalId: resumeResp.ProposalId, Approve: true})
	if err != nil {
		t.Fatalf("VoteResume (precommit) failed: %v", err)
	}

	if !vr.ChainResumed {
		t.Error("expected chain_resumed to be true")
	}

	status = k.GetEmergencyStatus(ctx)
	if status != types.StatusNormal {
		t.Fatalf("expected normal after resume, got %s", status)
	}
}

func TestNonGuardianRejection(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	scholar := "zrn1scholar"
	mock.addNonGuardian(scholar, "50000000000")

	msgSvr := keeper.NewMsgServerImpl(k)

	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: scholar,
		Reason:   "should fail",
	})
	if err == nil {
		t.Fatal("expected error for non-guardian proposer")
	}
}

func TestCeremonyTimeout(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.HaltTimeoutBlocks = 10
	params.HaltPrevoteBlocks = 5
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "timeout test",
	})

	// Advance past timeout deadline
	ctx = ctx.WithBlockHeight(int64(100 + params.HaltTimeoutBlocks + 1))

	// BeginBlock should detect timeout
	am := emergency.NewAppModule(nil, k)
	am.BeginBlock(ctx)

	ceremony, found := k.GetCeremony(ctx, resp.ProposalId)
	if !found {
		t.Fatal("ceremony not found after timeout")
	}
	if ceremony.Phase != string(types.PhaseFailed) {
		t.Fatalf("expected failed phase after timeout, got %s", ceremony.Phase)
	}

	// Status should revert to normal after halt failure
	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusNormal {
		t.Fatalf("expected normal after halt failure, got %s", status)
	}
}

func TestAntiAbusePerGuardianLimit(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "200000000000")

	params := k.GetParams(ctx)
	params.MaxProposalsPerGuardianPerEpoch = 1
	params.MaxProposalsPerEpoch = 100
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// First proposal succeeds
	resp1, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "first proposal",
	})
	if err != nil {
		t.Fatalf("first proposal should succeed: %v", err)
	}

	// Complete the ceremony so another can be proposed
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp1.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp1.ProposalId, Approve: true})

	// Second proposal should fail (per-guardian limit)
	_, err = msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "second proposal",
	})
	if err == nil {
		t.Fatal("second proposal should fail due to per-guardian limit")
	}
}

func TestAntiAbuseEpochLimit(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	g3 := "zrn1g3"
	g4 := "zrn1g4"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")
	mock.addGuardian(g3, "100000000000")
	mock.addGuardian(g4, "100000000000")

	params := k.GetParams(ctx)
	params.MaxProposalsPerEpoch = 2
	params.MaxProposalsPerGuardianPerEpoch = 5
	params.HaltQuorum = 200000 // 20% so each guardian alone reaches quorum
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// Proposal 1 by g1 → succeed + complete
	r1, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g1, Reason: "p1"})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: r1.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: r1.ProposalId, Approve: true})

	// Must resume before next halt proposal
	k.SetEmergencyStatus(ctx, types.StatusNormal)

	// Proposal 2 by g2 → succeed + complete
	r2, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g2, Reason: "p2"})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g2, ProposalId: r2.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g2, ProposalId: r2.ProposalId, Approve: true})

	k.SetEmergencyStatus(ctx, types.StatusNormal)

	// Proposal 3 by g3 → should fail (epoch limit = 2)
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g3, Reason: "p3"})
	if err == nil {
		t.Fatal("third proposal should fail due to epoch limit")
	}
}

func TestAutoResumeAfterMaxHaltDuration(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.MaxHaltDurationBlocks = 50
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	// Set up halted state
	k.SetEmergencyStatus(ctx, types.StatusHalted)
	k.SetHaltStartBlock(ctx, 100)
	k.SetActiveHaltCeremonyId(ctx, "test-halt")

	// Before expiry — should stay halted
	ctx = ctx.WithBlockHeight(149)
	k.CheckHaltExpiry(ctx)
	if k.GetEmergencyStatus(ctx) != types.StatusHalted {
		t.Fatal("should still be halted before max duration")
	}

	// At expiry — should auto-resume
	ctx = ctx.WithBlockHeight(150)
	k.CheckHaltExpiry(ctx)
	if k.GetEmergencyStatus(ctx) != types.StatusNormal {
		t.Fatal("should be auto-resumed after max duration")
	}
}

func TestRevertCeremony(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.RevertQuorum = 500000
	params.MaxRevertDepth = 1000
	k.SetParams(ctx, params)

	// Must be halted first
	k.SetEmergencyStatus(ctx, types.StatusHalted)
	k.SetHaltStartBlock(ctx, 50)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, err := msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 90,
		Justification:  "roll back to safe state",
	})
	if err != nil {
		t.Fatalf("ProposeRevert failed: %v", err)
	}

	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusRevertVoting {
		t.Fatalf("expected revert_voting, got %s", status)
	}

	// Vote to finalize
	msgSvr.VoteRevert(ctx, &types.MsgVoteRevert{Voter: g1, ProposalId: resp.ProposalId, Approve: true})
	vr, err := msgSvr.VoteRevert(ctx, &types.MsgVoteRevert{Voter: g1, ProposalId: resp.ProposalId, Approve: true})
	if err != nil {
		t.Fatalf("VoteRevert precommit failed: %v", err)
	}

	if !vr.RevertExecuted {
		t.Error("expected revert_executed to be true")
	}

	status = k.GetEmergencyStatus(ctx)
	if status != types.StatusReverting {
		t.Fatalf("expected reverting, got %s", status)
	}

	target, found := k.GetRevertTarget(ctx)
	if !found {
		t.Fatal("expected revert target to be set")
	}
	if target.Height != 90 {
		t.Fatalf("expected revert height 90, got %d", target.Height)
	}
}

func TestRevertDepthExceeded(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.MaxRevertDepth = 10
	k.SetParams(ctx, params)

	k.SetEmergencyStatus(ctx, types.StatusHalted)

	msgSvr := keeper.NewMsgServerImpl(k)

	// Try to revert too far back
	_, err := msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 50, // current=100, depth=50 > max=10
		Justification:  "too deep",
	})
	if err == nil {
		t.Fatal("expected error for revert depth exceeded")
	}
}

func TestDuplicateVotePrevention(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	g3 := "zrn1g3"
	// 3 guardians: each has 33% of stake, so one vote alone (33%) < 90% threshold.
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")
	mock.addGuardian(g3, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 900000 // 90% — one guardian's 33% won't reach quorum
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "dup test",
	})

	// First prevote succeeds
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("first prevote should succeed: %v", err)
	}

	// Verify still in prevote phase
	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Fatalf("expected prevote phase, got %s", ceremony.Phase)
	}

	// Second prevote from same guardian should fail (duplicate)
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err == nil {
		t.Fatal("duplicate prevote should fail")
	}
}

func TestPrecommitWithoutPrevoteRejection(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "80000000000")
	mock.addGuardian(g2, "20000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "precommit test",
	})

	// g1 prevotes yes → advances to precommit (80% > 50%)
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})

	// g2 tries to precommit without having prevoted
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g2, ProposalId: resp.ProposalId, Approve: true,
	})
	if err == nil {
		t.Fatal("precommit without prevote should fail")
	}
}

func TestGenesisRoundtrip(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	// Set some state
	k.SetEmergencyStatus(ctx, types.StatusHalted)
	k.SetHaltStartBlock(ctx, 42)
	k.AddAuditEntry(ctx, &types.EmergencyAuditEntry{
		BlockNumber: 42,
		Action:      string(types.AuditHaltExecuted),
		Actor:       "system",
		CeremonyId:  "test-ceremony",
		Details:     "test audit",
	})

	// Export
	genState := k.ExportGenesis(ctx)
	if genState.Status != string(types.StatusHalted) {
		t.Fatalf("expected halted in export, got %s", genState.Status)
	}
	if len(genState.AuditLog) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(genState.AuditLog))
	}

	// Re-init on fresh keeper
	k2, _, ctx2 := setupKeeper(t)
	k2.InitGenesis(ctx2, genState)

	status := k2.GetEmergencyStatus(ctx2)
	if status != types.StatusHalted {
		t.Fatalf("expected halted after import, got %s", status)
	}

	auditLog := k2.GetAuditLog(ctx2)
	if len(auditLog) != 1 {
		t.Fatalf("expected 1 audit entry after import, got %d", len(auditLog))
	}
}

func TestIsEmergencyMsg(t *testing.T) {
	tests := []struct {
		name     string
		msg      sdk.Msg
		expected bool
	}{
		{"ProposeHalt", &types.MsgProposeHalt{}, true},
		{"VoteHalt", &types.MsgVoteHalt{}, true},
		{"ProposeRevert", &types.MsgProposeRevert{}, true},
		{"VoteRevert", &types.MsgVoteRevert{}, true},
		{"ProposeResume", &types.MsgProposeResume{}, true},
		{"VoteResume", &types.MsgVoteResume{}, true},
		{"UpdateParams", &types.MsgUpdateParams{}, false}, // UpdateParams is not an emergency msg
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := types.IsEmergencyMsg(tt.msg); got != tt.expected {
				t.Errorf("IsEmergencyMsg(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestCooldown(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.CooldownBlocks = 50
	params.HaltQuorum = 200000
	params.MaxProposalsPerGuardianPerEpoch = 100
	params.MaxProposalsPerEpoch = 100
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// First proposal at block 100
	r1, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g1, Reason: "first"})

	// Complete it
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: r1.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: r1.ProposalId, Approve: true})
	k.SetEmergencyStatus(ctx, types.StatusNormal)

	// Try another at block 110 (within cooldown)
	ctx = ctx.WithBlockHeight(110)
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g2, Reason: "too soon"})
	if err == nil {
		t.Fatal("expected cooldown error")
	}

	// Try at block 160 (after cooldown)
	ctx = ctx.WithBlockHeight(160)
	_, err = msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g2, Reason: "after cooldown"})
	if err != nil {
		t.Fatalf("should succeed after cooldown: %v", err)
	}
}

func TestQueryStatus(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	querySvr := keeper.NewQueryServerImpl(k)

	resp, err := querySvr.Status(ctx, &types.QueryStatusRequest{})
	if err != nil {
		t.Fatalf("query Status failed: %v", err)
	}
	if resp.Status != string(types.StatusNormal) {
		t.Fatalf("expected normal, got %s", resp.Status)
	}
	if resp.IsHalted {
		t.Error("expected is_halted=false")
	}
}

func TestQueryParams(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	querySvr := keeper.NewQueryServerImpl(k)

	resp, err := querySvr.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		t.Fatalf("query Params failed: %v", err)
	}
	if resp.Params == nil {
		t.Fatal("expected non-nil params")
	}
}

func TestProposeRevertWhenNotHalted(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.MaxRevertDepth = 1000
	k.SetParams(ctx, params)

	// Status is normal (not halted) — propose revert should fail
	msgSvr := keeper.NewMsgServerImpl(k)
	_, err := msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 90,
		Justification:  "trying to revert while not halted",
	})
	if err == nil {
		t.Fatal("expected error when proposing revert while not halted")
	}
}

func TestGenesisCouncilCanPropose(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	councilMember := "zrn1council1"

	// Set params with genesis council active
	params := k.GetParams(ctx)
	params.GenesisCouncil = []string{councilMember}
	params.CouncilExpiryBlock = 1000 // expires at block 1000
	params.CouncilVirtualStake = "50000000000"
	params.MinGuardianStake = "0"
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	// Council member at block 100 (< 1000) should be treated as guardian
	if !k.IsGuardian(ctx, councilMember) {
		t.Fatal("council member should be recognized as guardian during bootstrap")
	}

	// Council member should be able to propose halt
	msgSvr := keeper.NewMsgServerImpl(k)
	resp, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: councilMember,
		Reason:   "bootstrap emergency",
	})
	if err != nil {
		t.Fatalf("council member should be able to propose halt: %v", err)
	}
	if resp.ProposalId == "" {
		t.Fatal("expected non-empty proposal ID")
	}
}

func TestCouncilExpiryBlock(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	councilMember := "zrn1council1"

	// Set council that expires at block 200
	params := k.GetParams(ctx)
	params.GenesisCouncil = []string{councilMember}
	params.CouncilExpiryBlock = 200
	params.CouncilVirtualStake = "50000000000"
	k.SetParams(ctx, params)

	// Before expiry — council is active
	if !k.IsGuardian(ctx, councilMember) {
		t.Fatal("council member should be active at block 100")
	}

	// After expiry — council is no longer active
	ctxExpired := ctx.WithBlockHeight(200)
	if k.IsGuardian(ctxExpired, councilMember) {
		t.Fatal("council member should NOT be active at or after expiry block 200")
	}

	// Proposing halt should fail after expiry
	msgSvr := keeper.NewMsgServerImpl(k)
	_, err := msgSvr.ProposeHalt(ctxExpired, &types.MsgProposeHalt{
		Proposer: councilMember,
		Reason:   "should fail after expiry",
	})
	if err == nil {
		t.Fatal("expected error for expired council member proposing halt")
	}
}

func TestMinDistinctVotersEnforcement(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	// One guardian with 100% of stake
	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 500000       // 50% — g1 alone meets this
	params.MinDistinctVoters = 3     // But we require 3 distinct voters
	params.HaltTimeoutBlocks = 5     // Short timeout
	params.HaltPrevoteBlocks = 5000  // Long prevote window
	params.HaltPrecommitBlocks = 5   // Short precommit window
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// Propose + prevote → moves to precommit (g1 has 100% stake > 50%)
	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g1, Reason: "min voters test"})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})

	// g1 precommits — has 100% stake but only 1 distinct voter < 3 required
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})

	// Ceremony should NOT be finalized due to insufficient distinct voters
	ceremony, found := k.GetCeremony(ctx, resp.ProposalId)
	if !found {
		t.Fatal("ceremony not found")
	}
	if ceremony.Phase == string(types.PhaseFinalized) {
		t.Fatal("ceremony should NOT be finalized with insufficient distinct voters")
	}

	// Advance past precommit deadline — should fail due to distinct voter check
	ctx = ctx.WithBlockHeight(int64(100 + params.HaltTimeoutBlocks + 1))
	am := emergency.NewAppModule(nil, k)
	am.BeginBlock(ctx)

	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhaseFailed) {
		t.Fatalf("expected failed phase, got %s", ceremony.Phase)
	}
	if ceremony.FailureReason == "" {
		t.Fatal("expected failure reason to be set")
	}
}

func TestEpochCounterReset(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	// Simulate some proposal counts
	k.IncrementGuardianProposalCount(ctx, "zrn1g1")
	k.IncrementGuardianProposalCount(ctx, "zrn1g2")
	k.IncrementEpochProposalCount(ctx)

	if k.GetGuardianProposalCount(ctx, "zrn1g1") != 1 {
		t.Fatal("expected count 1")
	}
	if k.GetEpochProposalCount(ctx) != 1 {
		t.Fatal("expected epoch count 1")
	}

	// Reset
	k.ResetEpochCounters(ctx)

	if k.GetGuardianProposalCount(ctx, "zrn1g1") != 0 {
		t.Fatal("expected count 0 after reset")
	}
	if k.GetEpochProposalCount(ctx) != 0 {
		t.Fatal("expected epoch count 0 after reset")
	}
}

// ============================================================
// Ported tests from legible-money prototype — Batch 1
// ============================================================

// --- Guardian / Stake Tests ---

// TestGetGuardianStake verifies total stake calculation across multiple guardians.
func TestGetGuardianStake(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	mock.addGuardian("zrn1g1", "100000000000")
	mock.addGuardian("zrn1g2", "200000000000")
	mock.addGuardian("zrn1g3", "300000000000")

	total := k.GetGuardianStake(ctx)
	if total.String() != "600000000000" {
		t.Fatalf("expected total guardian stake 600000000000, got %s", total.String())
	}
}

// TestGetGuardianStakeWithCouncil verifies council virtual stake is included during bootstrap.
func TestGetGuardianStakeWithCouncil(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	mock.addGuardian("zrn1g1", "100000000000")

	params := k.GetParams(ctx)
	params.GenesisCouncil = []string{"zrn1council1", "zrn1council2"}
	params.CouncilExpiryBlock = 1000
	params.CouncilVirtualStake = "50000000000"
	k.SetParams(ctx, params)

	total := k.GetGuardianStake(ctx)
	// 100B (guardian) + 2 * 50B (council) = 200B
	if total.String() != "200000000000" {
		t.Fatalf("expected total stake 200000000000 (including council), got %s", total.String())
	}
}

// TestGetGuardianEffectiveStake verifies individual guardian stake lookups.
func TestGetGuardianEffectiveStake(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	mock.addGuardian("zrn1g1", "100000000000")
	mock.addNonGuardian("zrn1scholar", "50000000000")

	stake := k.GetGuardianEffectiveStake(ctx, "zrn1g1")
	if stake.String() != "100000000000" {
		t.Fatalf("expected guardian stake 100000000000, got %s", stake.String())
	}

	// Non-guardian should return 0
	stake = k.GetGuardianEffectiveStake(ctx, "zrn1scholar")
	if stake.Sign() != 0 {
		t.Fatalf("expected 0 stake for non-guardian, got %s", stake.String())
	}

	// Unknown should return 0
	stake = k.GetGuardianEffectiveStake(ctx, "zrn1unknown")
	if stake.Sign() != 0 {
		t.Fatalf("expected 0 stake for unknown, got %s", stake.String())
	}
}

// --- Ceremony Validation Tests ---

// TestCannotHaltWhenAlreadyHalted verifies halt proposal when already halted is rejected.
func TestCannotHaltWhenAlreadyHalted(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	k.SetEmergencyStatus(ctx, types.StatusHalted)

	msgSvr := keeper.NewMsgServerImpl(k)
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "already halted",
	})
	if err == nil {
		t.Fatal("expected error when proposing halt while already halted")
	}
}

// TestCannotResumeWhenNotHalted verifies resume proposal when status is normal is rejected.
func TestCannotResumeWhenNotHalted(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	// Status is normal by default
	msgSvr := keeper.NewMsgServerImpl(k)
	_, err := msgSvr.ProposeResume(ctx, &types.MsgProposeResume{
		Proposer: g1,
	})
	if err == nil {
		t.Fatal("expected error when proposing resume while not halted")
	}
}

// TestCeremonyActiveBlocksNewProposal verifies active ceremony prevents new proposals.
func TestCeremonyActiveBlocksNewProposal(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.MaxProposalsPerGuardianPerEpoch = 100
	params.MaxProposalsPerEpoch = 100
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// First proposal creates an active ceremony
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "first proposal",
	})
	if err != nil {
		t.Fatalf("first proposal should succeed: %v", err)
	}

	// Second proposal should fail because ceremony is active
	_, err = msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g2,
		Reason:   "second proposal",
	})
	if err == nil {
		t.Fatal("expected error when ceremony is already active")
	}
}

// --- Voting Mechanics Tests ---

// TestPrecommitInPrevotePhaseRejected verifies that submitting a precommit during prevote phase
// is handled correctly (vote is treated as a prevote since ceremony is in prevote phase).
func TestPrecommitInPrevotePhaseRejected(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	g3 := "zrn1g3"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")
	mock.addGuardian(g3, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 900000 // 90% — need all 3 to advance to precommit
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "prevote phase test",
	})

	// g1 prevotes — still in prevote phase (33% < 90%)
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("g1 prevote should succeed: %v", err)
	}

	// Verify still in prevote phase
	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Fatalf("expected prevote phase, got %s", ceremony.Phase)
	}

	// g2 votes — this should be recorded as a prevote (phase is still prevote)
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g2, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("g2 prevote should succeed: %v", err)
	}

	// Verify g2's vote was recorded as a prevote
	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	_, hasPrevote := ceremony.GetPrevote(g2)
	if !hasPrevote {
		t.Fatal("expected g2's vote to be recorded as a prevote")
	}
}

// TestStakeWeightedVoting verifies that stake weights affect quorum calculations.
func TestStakeWeightedVoting(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	// g1 has 90%, g2 has 10%
	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "900000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 750000 // 75%
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "stake weight test",
	})

	// g2 alone votes (10% < 75%) — should NOT advance to precommit
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g2, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("g2 prevote should succeed: %v", err)
	}
	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Fatalf("expected prevote (10%% < 75%%), got %s", ceremony.Phase)
	}

	// g1 votes (90% > 75%) — should advance to precommit
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("g1 prevote should succeed: %v", err)
	}
	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrecommit) {
		t.Fatalf("expected precommit after 100%% stake voted, got %s", ceremony.Phase)
	}
}

// TestNonGuardianVoteRejected verifies a non-guardian vote attempt is rejected.
func TestNonGuardianVoteRejected(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	scholar := "zrn1scholar"
	mock.addGuardian(g1, "100000000000")
	mock.addNonGuardian(scholar, "50000000000")

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "non-guardian vote test",
	})

	// Non-guardian tries to vote
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      scholar,
		ProposalId: resp.ProposalId,
		Approve:    true,
	})
	if err == nil {
		t.Fatal("expected error for non-guardian vote attempt")
	}
}

// --- State Tests ---

// TestIsHaltedStates verifies IsHalted returns correct values for each status.
func TestIsHaltedStates(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	tests := []struct {
		status   types.EmergencyStatus
		expected bool
	}{
		{types.StatusNormal, false},
		{types.StatusHaltVoting, false},
		{types.StatusHalted, true},
		{types.StatusRevertVoting, true},
		{types.StatusReverting, true},
		{types.StatusResumeVoting, true},
	}

	for _, tt := range tests {
		k.SetEmergencyStatus(ctx, tt.status)
		got := k.IsHalted(ctx)
		if got != tt.expected {
			t.Errorf("IsHalted(%s) = %v, want %v", tt.status, got, tt.expected)
		}
	}
}

// TestAuditLogRecordsActions verifies audit entries are recorded for actions.
func TestAuditLogRecordsActions(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	msgSvr := keeper.NewMsgServerImpl(k)

	// Propose halt — should generate audit entry
	resp, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "audit test",
	})
	if err != nil {
		t.Fatalf("ProposeHalt failed: %v", err)
	}

	entries := k.GetAuditLog(ctx)
	if len(entries) == 0 {
		t.Fatal("expected audit log entries after proposal")
	}

	// Check halt_proposed audit entry
	foundProposal := false
	for _, e := range entries {
		if e.Action == string(types.AuditHaltProposed) && e.Actor == g1 {
			foundProposal = true
			break
		}
	}
	if !foundProposal {
		t.Fatal("expected halt_proposed audit entry from g1")
	}

	// Vote — should generate another audit entry
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("VoteHalt failed: %v", err)
	}

	entries = k.GetAuditLog(ctx)
	foundVote := false
	for _, e := range entries {
		if e.Action == string(types.AuditHaltPrevote) && e.Actor == g1 {
			foundVote = true
			break
		}
	}
	if !foundVote {
		t.Fatal("expected halt_prevote audit entry from g1")
	}
}

// --- Query Server Tests ---

// TestQueryActiveCeremony_None verifies query returns no active ceremony when none exists.
func TestQueryActiveCeremony_None(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	querySvr := keeper.NewQueryServerImpl(k)

	resp, err := querySvr.ActiveCeremony(ctx, &types.QueryActiveCeremonyRequest{})
	if err != nil {
		t.Fatalf("query ActiveCeremony failed: %v", err)
	}
	if resp.Found {
		t.Error("expected no active ceremony when none exists")
	}
}

// TestQueryActiveCeremony_Found verifies query finds an active ceremony.
func TestQueryActiveCeremony_Found(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	msgSvr := keeper.NewMsgServerImpl(k)
	querySvr := keeper.NewQueryServerImpl(k)

	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "query test",
	})
	if err != nil {
		t.Fatalf("ProposeHalt failed: %v", err)
	}

	resp, err := querySvr.ActiveCeremony(ctx, &types.QueryActiveCeremonyRequest{})
	if err != nil {
		t.Fatalf("query ActiveCeremony failed: %v", err)
	}
	if !resp.Found {
		t.Error("expected active ceremony to be found")
	}
	if resp.Ceremony == nil {
		t.Error("expected non-nil ceremony in response")
	}
}

// TestQueryCompletedCeremonies verifies querying completed ceremonies.
func TestQueryCompletedCeremonies(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)
	querySvr := keeper.NewQueryServerImpl(k)

	// No completed ceremonies initially
	resp, err := querySvr.CompletedCeremonies(ctx, &types.QueryCompletedCeremoniesRequest{})
	if err != nil {
		t.Fatalf("query CompletedCeremonies failed: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 completed ceremonies, got %d", resp.Total)
	}

	// Complete a halt ceremony
	haltResp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g1, Reason: "test"})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: haltResp.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: haltResp.ProposalId, Approve: true})

	// Now should have 1 completed ceremony
	resp, err = querySvr.CompletedCeremonies(ctx, &types.QueryCompletedCeremoniesRequest{})
	if err != nil {
		t.Fatalf("query CompletedCeremonies failed: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 completed ceremony, got %d", resp.Total)
	}
}

// TestQueryAuditLog verifies querying audit log entries.
func TestQueryAuditLog(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	querySvr := keeper.NewQueryServerImpl(k)

	// No audit entries initially
	resp, err := querySvr.AuditLog(ctx, &types.QueryAuditLogRequest{})
	if err != nil {
		t.Fatalf("query AuditLog failed: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 audit entries, got %d", resp.Total)
	}

	// Add an audit entry
	k.AddAuditEntry(ctx, &types.EmergencyAuditEntry{
		BlockNumber: 100,
		Action:      string(types.AuditHaltProposed),
		Actor:       "zrn1g1",
		CeremonyId:  "test-123",
		Details:     "test entry",
	})

	resp, err = querySvr.AuditLog(ctx, &types.QueryAuditLogRequest{})
	if err != nil {
		t.Fatalf("query AuditLog failed: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 audit entry, got %d", resp.Total)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry in response, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Actor != "zrn1g1" {
		t.Fatalf("expected actor zrn1g1, got %s", resp.Entries[0].Actor)
	}
}

// --- Halt Expiry Edge Cases ---

// TestCheckHaltExpiry_NotHalted verifies expiry check when not halted is a no-op.
func TestCheckHaltExpiry_NotHalted(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	// Status is normal by default
	k.CheckHaltExpiry(ctx)
	if k.GetEmergencyStatus(ctx) != types.StatusNormal {
		t.Fatal("status should remain normal after CheckHaltExpiry when not halted")
	}
}

// TestCheckHaltExpiry_NoStartBlock verifies edge case with halted but no start block.
func TestCheckHaltExpiry_NoStartBlock(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	// Halted but no start block recorded (pre-upgrade scenario)
	k.SetEmergencyStatus(ctx, types.StatusHalted)
	k.CheckHaltExpiry(ctx)
	if k.GetEmergencyStatus(ctx) != types.StatusHalted {
		t.Fatal("should not expire without start block (graceful for pre-upgrade halts)")
	}
}

// ============================================================
// Adversarial Tests (ported from openclaw_emergency_test.go)
// ============================================================

// TestOC_NonGuardianHaltAttempt verifies a non-guardian cannot vote on halt.
// Note: TestNonGuardianRejection covers proposal rejection; this tests VOTE rejection.
func TestOC_NonGuardianHaltAttempt(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	unknown := "zrn1attacker"
	mock.addGuardian(g1, "100000000000")

	msgSvr := keeper.NewMsgServerImpl(k)

	// Guardian creates a valid ceremony
	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "adversarial test",
	})

	// Completely unknown address tries to vote
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      unknown,
		ProposalId: resp.ProposalId,
		Approve:    true,
	})
	if err == nil {
		t.Fatal("OC: unknown address voting on halt should fail")
	}

	// Non-guardian (Scholar tier) tries to vote
	mock.addNonGuardian("zrn1scholar", "50000000000")
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter:      "zrn1scholar",
		ProposalId: resp.ProposalId,
		Approve:    true,
	})
	if err == nil {
		t.Fatal("OC: non-guardian voting on halt should fail")
	}
}

// TestOC_CooldownBypass verifies cooldown bypass via block height manipulation.
// Note: TestCooldown covers basic cooldown; this tests the exact boundary.
func TestOC_CooldownBypass(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.CooldownBlocks = 50
	params.HaltQuorum = 200000 // 20%
	params.MaxProposalsPerGuardianPerEpoch = 100
	params.MaxProposalsPerEpoch = 100
	params.HaltTimeoutBlocks = 5 // Short timeout
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// First proposal at block 100
	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "cooldown boundary test",
	})

	// Advance past timeout so ceremony fails, then handle failure
	ctx = ctx.WithBlockHeight(106)
	k.CheckCeremonyProgress(ctx, resp.ProposalId)
	k.HandleCeremonyFailure(ctx, resp.ProposalId)

	// Block 149 — still in cooldown (100 + 50 = 150)
	ctx = ctx.WithBlockHeight(149)
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g2,
		Reason:   "boundary minus one",
	})
	if err == nil {
		t.Fatal("OC: proposal at cooldown boundary-1 (block 149, cooldown ends at 150) should fail")
	}

	// Block 150 — exactly at cooldown end
	ctx = ctx.WithBlockHeight(150)
	_, err = msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g2,
		Reason:   "exactly at boundary",
	})
	if err != nil {
		t.Fatalf("OC: proposal at cooldown boundary (block 150) should succeed: %v", err)
	}
}

// TestOC_ProposalSpamPerEpoch verifies same guardian cannot propose twice in one epoch.
// Note: TestAntiAbusePerGuardianLimit covers basic limit; this tests with epoch boundary.
func TestOC_ProposalSpamPerEpoch(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.MaxProposalsPerGuardianPerEpoch = 1
	params.MaxProposalsPerEpoch = 100
	params.HaltQuorum = 200000
	params.CooldownBlocks = 0 // No cooldown for this test
	params.HaltTimeoutBlocks = 5
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// g1 proposes at block 100
	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "first",
	})

	// Advance past timeout, fail the ceremony
	ctx = ctx.WithBlockHeight(106)
	k.CheckCeremonyProgress(ctx, resp.ProposalId)
	k.HandleCeremonyFailure(ctx, resp.ProposalId)

	// g1 tries again — should fail (per-guardian limit)
	_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "second by same guardian",
	})
	if err == nil {
		t.Fatal("OC: same guardian proposing twice per epoch should fail")
	}

	// g2 should succeed (different guardian)
	_, err = msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g2,
		Reason:   "different guardian",
	})
	if err != nil {
		t.Fatalf("OC: different guardian should be able to propose: %v", err)
	}
}

// TestOC_PrecommitWithoutPrevote verifies precommit from a guardian who didn't prevote.
// Note: TestPrecommitWithoutPrevoteRejection exists; this tests a resume ceremony variant.
func TestOC_PrecommitWithoutPrevote(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	g3 := "zrn1g3"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")
	mock.addGuardian(g3, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 500000  // 50%
	params.ResumeQuorum = 500000
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	// Execute a halt
	haltResp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{Proposer: g1, Reason: "precommit test"})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: haltResp.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g2, ProposalId: haltResp.ProposalId, Approve: true})
	// Now in precommit phase, finalize halt
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: haltResp.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g2, ProposalId: haltResp.ProposalId, Approve: true})

	if k.GetEmergencyStatus(ctx) != types.StatusHalted {
		t.Fatal("expected halted status for precommit test setup")
	}

	// Propose resume
	resumeResp, _ := msgSvr.ProposeResume(ctx, &types.MsgProposeResume{Proposer: g1})

	// g1 and g2 prevote resume
	msgSvr.VoteResume(ctx, &types.MsgVoteResume{Voter: g1, ProposalId: resumeResp.ProposalId, Approve: true})
	msgSvr.VoteResume(ctx, &types.MsgVoteResume{Voter: g2, ProposalId: resumeResp.ProposalId, Approve: true})

	// Verify ceremony is in precommit
	ceremony, _ := k.GetCeremony(ctx, resumeResp.ProposalId)
	if ceremony.Phase != string(types.PhasePrecommit) {
		t.Fatalf("expected precommit phase, got %s", ceremony.Phase)
	}

	// g3 did NOT prevote — precommit on resume should fail
	_, err := msgSvr.VoteResume(ctx, &types.MsgVoteResume{
		Voter: g3, ProposalId: resumeResp.ProposalId, Approve: true,
	})
	if err == nil {
		t.Fatal("OC: precommit without prevote on resume ceremony should fail")
	}
}

// TestOC_DuplicateVoteInPhase verifies duplicate precommit vote is rejected.
// Note: TestDuplicateVotePrevention covers prevote duplicates; this tests precommit duplicates.
func TestOC_DuplicateVoteInPhase(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "80000000000")
	mock.addGuardian(g2, "20000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 500000 // 50%
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "dup precommit test",
	})

	// g1 prevotes (80% > 50%) — advances to precommit
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g1, ProposalId: resp.ProposalId, Approve: true})

	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrecommit) {
		t.Fatalf("expected precommit phase, got %s", ceremony.Phase)
	}

	// g1 first precommit — succeeds
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("first precommit should succeed: %v", err)
	}

	// g1 duplicate precommit — should fail
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g1, ProposalId: resp.ProposalId, Approve: true,
	})
	if err == nil {
		t.Fatal("OC: duplicate precommit should fail")
	}
}

// TestOC_HaltFromWrongStatus verifies halt proposals are rejected from non-normal statuses.
func TestOC_HaltFromWrongStatus(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	msgSvr := keeper.NewMsgServerImpl(k)

	invalidStatuses := []types.EmergencyStatus{
		types.StatusHalted,
		types.StatusResumeVoting,
		types.StatusRevertVoting,
		types.StatusReverting,
		types.StatusHaltVoting,
	}

	for _, status := range invalidStatuses {
		k.SetEmergencyStatus(ctx, status)
		_, err := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
			Proposer: g1,
			Reason:   "wrong status: " + string(status),
		})
		if err == nil {
			t.Errorf("OC: halt proposal should fail when status is %s", status)
		}
	}
}

// TestOC_RevertDepthExceeded verifies revert at exact boundary of MaxRevertDepth.
// Note: TestRevertDepthExceeded tests depth > max; this tests depth == max (boundary).
func TestOC_RevertDepthExceeded(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.MaxRevertDepth = 50
	params.HaltQuorum = 500000
	k.SetParams(ctx, params)

	// Halt the chain
	k.SetEmergencyStatus(ctx, types.StatusHalted)

	msgSvr := keeper.NewMsgServerImpl(k)

	// At block 100, revert to block 50 → depth = 50 = MaxRevertDepth (should succeed)
	_, err := msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 50,
		Justification:  "exact max depth",
	})
	if err != nil {
		t.Fatalf("OC: revert at exact max depth (50) should succeed: %v", err)
	}
}

// TestOC_RevertDepthExceededByOne verifies revert at max+1 is rejected.
func TestOC_RevertDepthExceededByOne(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.MaxRevertDepth = 50
	k.SetParams(ctx, params)

	k.SetEmergencyStatus(ctx, types.StatusHalted)

	msgSvr := keeper.NewMsgServerImpl(k)

	// At block 100, revert to block 49 → depth = 51 > MaxRevertDepth (should fail)
	_, err := msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 49,
		Justification:  "one past max depth",
	})
	if err == nil {
		t.Fatal("OC: revert at depth max+1 (51 > 50) should fail")
	}
}

// TestOC_ResumeFromNormalState verifies resume when already normal is rejected.
func TestOC_ResumeFromNormalState(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	// Status is normal by default
	if k.GetEmergencyStatus(ctx) != types.StatusNormal {
		t.Fatal("expected normal status")
	}

	msgSvr := keeper.NewMsgServerImpl(k)

	_, err := msgSvr.ProposeResume(ctx, &types.MsgProposeResume{
		Proposer: g1,
	})
	if err == nil {
		t.Fatal("OC: resume from normal state should fail")
	}

	// Also test revert from normal state
	_, err = msgSvr.ProposeRevert(ctx, &types.MsgProposeRevert{
		Proposer:       g1,
		RevertToHeight: 50,
		Justification:  "not halted",
	})
	if err == nil {
		t.Fatal("OC: revert from normal state should fail")
	}
}

// TestOC_QuorumManipulation verifies quorum advances correctly at threshold boundaries.
func TestOC_QuorumManipulation(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	// 5 guardians with equal stake (20% each)
	guardians := []string{"zrn1g1", "zrn1g2", "zrn1g3", "zrn1g4", "zrn1g5"}
	for _, g := range guardians {
		mock.addGuardian(g, "100000000000")
	}

	params := k.GetParams(ctx)
	params.HaltQuorum = 750000 // 75%
	params.MinDistinctVoters = 1
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: guardians[0],
		Reason:   "quorum manipulation test",
	})

	// 2 guardians = 40% < 75% — still prevote
	for _, g := range guardians[:2] {
		msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g, ProposalId: resp.ProposalId, Approve: true})
	}
	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Fatalf("OC: 40%% should still be prevote, got %s", ceremony.Phase)
	}

	// 3 guardians = 60% < 75% — still prevote
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[2], ProposalId: resp.ProposalId, Approve: true})
	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrevote) {
		t.Fatalf("OC: 60%% should still be prevote, got %s", ceremony.Phase)
	}

	// 4 guardians = 80% > 75% — advance to precommit
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[3], ProposalId: resp.ProposalId, Approve: true})
	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase != string(types.PhasePrecommit) {
		t.Fatalf("OC: 80%% should advance to precommit, got %s", ceremony.Phase)
	}

	// 3 precommits = 60% < 75% — not finalized
	for _, g := range guardians[:3] {
		msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: g, ProposalId: resp.ProposalId, Approve: true})
	}
	ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase == string(types.PhaseFinalized) {
		t.Fatal("OC: 60% precommit should not finalize at 75% quorum")
	}

	// 4th precommit → 80% > 75% → finalized
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[3], ProposalId: resp.ProposalId, Approve: true})

	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusHalted {
		t.Fatalf("OC: expected halted after full quorum, got %s", status)
	}
}

// TestOC_CeremonyTimeoutEnforcement verifies ceremony timeout at exact deadline boundary.
func TestOC_CeremonyTimeoutEnforcement(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.HaltPrevoteBlocks = 10
	params.HaltTimeoutBlocks = 20
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "timeout boundary test",
	})

	// At prevote deadline boundary (100 + 10 = 110): should still be active
	ctxAtDeadline := ctx.WithBlockHeight(110)
	k.CheckCeremonyProgress(ctxAtDeadline, resp.ProposalId)
	ceremony, _ := k.GetCeremony(ctxAtDeadline, resp.ProposalId)
	if ceremony.Phase == string(types.PhaseFailed) {
		t.Error("OC: ceremony should not timeout exactly at prevote deadline")
	}

	// Just past prevote deadline (111): should fail (prevote quorum not reached)
	ctxPastDeadline := ctx.WithBlockHeight(111)
	k.CheckCeremonyProgress(ctxPastDeadline, resp.ProposalId)
	ceremony, _ = k.GetCeremony(ctxPastDeadline, resp.ProposalId)
	if ceremony.Phase != string(types.PhaseFailed) {
		t.Fatalf("OC: ceremony should fail after prevote deadline, got %s", ceremony.Phase)
	}
	if ceremony.FailureReason == "" {
		t.Fatal("OC: expected failure reason to be set")
	}
}

// TestOC_VoteFlipRejected verifies that changing vote (YES→NO) in prevote phase is rejected.
func TestOC_VoteFlipRejected(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	g2 := "zrn1g2"
	mock.addGuardian(g1, "100000000000")
	mock.addGuardian(g2, "100000000000")

	params := k.GetParams(ctx)
	params.HaltQuorum = 900000 // High quorum so g1 alone can't advance
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "vote flip test",
	})

	// g2 prevotes YES
	_, err := msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g2, ProposalId: resp.ProposalId, Approve: true,
	})
	if err != nil {
		t.Fatalf("first prevote should succeed: %v", err)
	}

	// g2 tries to prevote NO (flip) — should fail as duplicate
	_, err = msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{
		Voter: g2, ProposalId: resp.ProposalId, Approve: false,
	})
	if err == nil {
		t.Fatal("OC: vote flip (YES→NO) should be rejected as duplicate")
	}
}

// TestOC_NoPrevoteRejection verifies that "no" prevotes contribute to quorum impossibility.
func TestOC_NoPrevoteRejection(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	// 4 guardians with equal stake (25% each)
	guardians := []string{"zrn1g1", "zrn1g2", "zrn1g3", "zrn1g4"}
	for _, g := range guardians {
		mock.addGuardian(g, "100000000000")
	}

	params := k.GetParams(ctx)
	params.HaltQuorum = 750000 // 75%
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: guardians[0],
		Reason:   "no vote test",
	})

	// g1 votes YES (25%), g2 votes NO (25%)
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[0], ProposalId: resp.ProposalId, Approve: true})
	msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[1], ProposalId: resp.ProposalId, Approve: false})

	// After NO exceeds (100% - 75%) = 25%, quorum should become impossible
	// g2's NO at 25% = 25% which is equal to threshold (not strictly greater), check behavior
	ceremony, _ := k.GetCeremony(ctx, resp.ProposalId)
	if ceremony.Phase == string(types.PhaseFailed) {
		t.Log("ceremony correctly failed: quorum impossible with 25% NO votes")
	}

	// g3 also votes NO (now 50% NO > 25% allowed)
	if ceremony.Phase != string(types.PhaseFailed) {
		msgSvr.VoteHalt(ctx, &types.MsgVoteHalt{Voter: guardians[2], ProposalId: resp.ProposalId, Approve: false})
		ceremony, _ = k.GetCeremony(ctx, resp.ProposalId)
		if ceremony.Phase != string(types.PhaseFailed) {
			t.Fatal("OC: ceremony should fail when quorum is impossible (50% NO at 75% threshold)")
		}
	}
}

// TestBeginBlockCeremonyTimeout verifies BeginBlock detects ceremony timeout.
func TestBeginBlockCeremonyTimeout(t *testing.T) {
	k, mock, ctx := setupKeeper(t)

	g1 := "zrn1g1"
	mock.addGuardian(g1, "100000000000")

	params := k.GetParams(ctx)
	params.HaltTimeoutBlocks = 10
	params.HaltPrevoteBlocks = 5
	k.SetParams(ctx, params)

	msgSvr := keeper.NewMsgServerImpl(k)

	resp, _ := msgSvr.ProposeHalt(ctx, &types.MsgProposeHalt{
		Proposer: g1,
		Reason:   "begin block timeout test",
	})

	// Advance past timeout deadline
	ctx = ctx.WithBlockHeight(int64(100 + params.HaltTimeoutBlocks + 1))

	// BeginBlock should detect timeout
	am := emergency.NewAppModule(nil, k)
	am.BeginBlock(ctx)

	ceremony, found := k.GetCeremony(ctx, resp.ProposalId)
	if !found {
		t.Fatal("ceremony not found after timeout")
	}
	if ceremony.Phase != string(types.PhaseFailed) {
		t.Fatalf("expected failed phase after timeout via BeginBlock, got %s", ceremony.Phase)
	}

	// Status should revert to normal after halt ceremony failure
	status := k.GetEmergencyStatus(ctx)
	if status != types.StatusNormal {
		t.Fatalf("expected normal after halt failure via BeginBlock, got %s", status)
	}
}

// --- Oracle Staleness Circuit Breaker Tests (P-21b) ---

func TestRecordOracleUpdate_StoresCurrentBlock(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	// Initially no oracle update recorded.
	if got := k.GetLastOracleUpdateBlock(ctx); got != 0 {
		t.Fatalf("expected 0 before any update, got %d", got)
	}

	// Record at block 100 (setupKeeper default height).
	k.RecordOracleUpdate(ctx)
	if got := k.GetLastOracleUpdateBlock(ctx); got != 100 {
		t.Fatalf("expected 100 after RecordOracleUpdate, got %d", got)
	}

	// Advance block and record again.
	ctx = ctx.WithBlockHeight(250)
	k.RecordOracleUpdate(ctx)
	if got := k.GetLastOracleUpdateBlock(ctx); got != 250 {
		t.Fatalf("expected 250 after second RecordOracleUpdate, got %d", got)
	}
}

func TestCheckOracleStaleness_DisabledByDefault(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	// Default params: CircuitOracleStalenessBlocks == 0 → disabled.
	// Record nothing; advance far into the future.
	ctx = ctx.WithBlockHeight(99999)

	// Must not halt even with massive staleness when disabled.
	k.CheckOracleStaleness(ctx)
	if k.IsHalted(ctx) {
		t.Fatal("chain should NOT halt when oracle staleness circuit breaker is disabled (blocks=0)")
	}
}

func TestCheckOracleStaleness_ThresholdNotMet(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 50
	k.SetParams(ctx, params)

	// Record oracle update at block 100.
	k.RecordOracleUpdate(ctx)

	// Advance to block 130: staleness = 30, threshold = 50 → no halt.
	ctx = ctx.WithBlockHeight(130)
	k.CheckOracleStaleness(ctx)
	if k.IsHalted(ctx) {
		t.Fatal("chain should NOT halt when staleness (30) < threshold (50)")
	}
}

func TestCheckOracleStaleness_ThresholdExact_NoHalt(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 50
	k.SetParams(ctx, params)

	// Record oracle update at block 100.
	k.RecordOracleUpdate(ctx)

	// Advance to block 150: staleness = 50 exactly → still within tolerance (≤ threshold).
	ctx = ctx.WithBlockHeight(150)
	k.CheckOracleStaleness(ctx)
	if k.IsHalted(ctx) {
		t.Fatal("chain should NOT halt when staleness (50) == threshold (50); halts only when staleness > threshold")
	}
}

func TestCheckOracleStaleness_ThresholdExceeded_Halts(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 50
	k.SetParams(ctx, params)

	// Record oracle update at block 100.
	k.RecordOracleUpdate(ctx)

	// Advance to block 151: staleness = 51 > threshold = 50 → halt.
	ctx = ctx.WithBlockHeight(151)
	k.CheckOracleStaleness(ctx)
	if !k.IsHalted(ctx) {
		t.Fatal("chain SHOULD halt when staleness (51) > threshold (50)")
	}

	// Status must be exactly StatusHalted.
	if status := k.GetEmergencyStatus(ctx); status != types.StatusHalted {
		t.Fatalf("expected StatusHalted, got %s", status)
	}
}

func TestCheckOracleStaleness_NoUpdateEver_DoesNotHalt(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 10
	k.SetParams(ctx, params)

	// Never call RecordOracleUpdate → last update block = 0.
	// Should not halt: 0 means "oracle keeper not yet wired".
	ctx = ctx.WithBlockHeight(9999)
	k.CheckOracleStaleness(ctx)
	if k.IsHalted(ctx) {
		t.Fatal("chain should NOT halt when no oracle update ever recorded (oracle not wired yet)")
	}
}

func TestCheckOracleStaleness_AlreadyHalted_NoDoubleHalt(t *testing.T) {
	k, _, ctx := setupKeeper(t)

	params := k.GetParams(ctx)
	params.CircuitOracleStalenessBlocks = 10
	k.SetParams(ctx, params)

	// Record at block 100, advance to block 115 (staleness=15 > threshold=10).
	k.RecordOracleUpdate(ctx)
	ctx = ctx.WithBlockHeight(115)

	// First check → halts.
	k.CheckOracleStaleness(ctx)
	if !k.IsHalted(ctx) {
		t.Fatal("expected halt after first CheckOracleStaleness")
	}

	// Manually verify status then call again → must not panic, must not change status.
	k.CheckOracleStaleness(ctx)
	if status := k.GetEmergencyStatus(ctx); status != types.StatusHalted {
		t.Fatalf("status should remain StatusHalted after second call, got %s", status)
	}
}
