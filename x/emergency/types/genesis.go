package types

import "fmt"

// DefaultParams returns the default emergency module parameters.
//
// These values express commitment 10 (forward-only audit) made into a
// ceremony rather than a privileged switch. See doc.go for the
// contract; the values below are the chain's pre-committed posture
// about its own emergency authority.
func DefaultParams() Params {
	return Params{
		// Quorum thresholds: halts require 75%, reverts and resumes
		// 80%. The asymmetry says: halting is the cheapest emergency
		// action because the chain keeps its history intact while
		// halted; reverts and resumes change the chain's reachable
		// state and demand stronger consent. Setting any quorum below
		// 50% would be admitting that a minority can declare an
		// emergency — which is what we believe must NOT be true.
		HaltQuorum:                      750000,  // 75% — supermajority for halt
		RevertQuorum:                    800000,  // 80% — stricter for state-change
		ResumeQuorum:                    800000,  // 80% — stricter for state-change

		// Vote phase blocks: prevote and precommit are SHORT for halt
		// (11 blocks ≈ 28s) because halt is time-sensitive. Revert
		// and resume run longer (22 blocks ≈ 55s) because they touch
		// state and deserve deliberation.
		HaltPrevoteBlocks:              11,
		HaltPrecommitBlocks:            11,
		HaltTimeoutBlocks:              44,
		RevertPrevoteBlocks:            22,
		RevertPrecommitBlocks:          22,
		RevertTimeoutBlocks:            111,
		ResumePrevoteBlocks:            22,
		ResumePrecommitBlocks:          22,
		ResumeTimeoutBlocks:            111,

		// Per-guardian and per-epoch caps: any single guardian can
		// open at most 1 proposal per epoch; the whole guardian set
		// can open at most 3. This rate-limits emergency machinery
		// from being weaponised as denial-of-service.
		MaxProposalsPerEpoch:           3,
		MaxProposalsPerGuardianPerEpoch: 1,
		CooldownBlocks:                 111,

		// Stake floor: total guardian stake must be at least 111,111
		// ZRN before halt proposals are accepted. This is the
		// "plurality is structural" floor — without enough guardians
		// committed, no single signer can declare an emergency.
		// MinDistinctVoters of 4 ensures at least four different
		// addresses participate in any quorum tally.
		MinGuardianStake:               "111111000000", // 111,111 ZRN — plurality requires committed stake
		MinDistinctVoters:              4,              // 4 distinct addresses minimum on any tally
		MaxRevertDepth:                 111111,         // sane upper bound on revert reach

		// Epoch and halt duration: 1 day at 2521ms blocks. The auto-
		// resume past MaxHaltDurationBlocks is the chain refusing to
		// be held hostage indefinitely — even an unanimous halt has a
		// time horizon. Without this, a halt could DoS the network
		// past the point where the original incident is over.
		EpochBlocks:                    34272, // ~1 day
		GenesisCouncil:                 []string{},
		CouncilExpiryBlock:             0,
		CouncilVirtualStake:            "11111000000", // 11,111 ZRN
		MaxHaltDurationBlocks:          34272,         // ~1 day auto-resume — halts cannot DoS forever

		// Circuit breakers are disabled by default — governance must explicitly
		// enable them once the chain has enough validator diversity to make the
		// threshold meaningful. Default thresholds are conservative: 10 slashes
		// in a 100-block window (~4 minutes at 2521ms). This is a coordinated
		// attack posture, not normal validator misbehaviour.
		CircuitBreakersEnabled:         false,
		CircuitSlashRateWindowBlocks:   100,  // ~4 minutes: tight enough to catch coordinated attacks
		CircuitSlashRateThreshold:      10,   // 10 slashes in 4 minutes = something is very wrong
	}
}

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	p := DefaultParams()
	return &GenesisState{
		Params: &p,
		Status: string(StatusNormal),
	}
}

// Validate validates the genesis state.
func (gs *GenesisState) Validate() error {
	if gs.Params == nil {
		return fmt.Errorf("params cannot be nil")
	}
	return gs.Params.Validate()
}

// Validate validates the params.
func (p *Params) Validate() error {
	if p.HaltQuorum > 1000000 {
		return fmt.Errorf("halt_quorum must be <= 1000000, got %d", p.HaltQuorum)
	}
	if p.RevertQuorum > 1000000 {
		return fmt.Errorf("revert_quorum must be <= 1000000, got %d", p.RevertQuorum)
	}
	if p.ResumeQuorum > 1000000 {
		return fmt.Errorf("resume_quorum must be <= 1000000, got %d", p.ResumeQuorum)
	}
	if p.HaltPrevoteBlocks == 0 {
		return fmt.Errorf("halt_prevote_blocks must be > 0")
	}
	if p.HaltPrecommitBlocks == 0 {
		return fmt.Errorf("halt_precommit_blocks must be > 0")
	}
	if p.HaltTimeoutBlocks == 0 {
		return fmt.Errorf("halt_timeout_blocks must be > 0")
	}
	if p.EpochBlocks == 0 {
		return fmt.Errorf("epoch_blocks must be > 0")
	}
	if p.MaxHaltDurationBlocks == 0 {
		return fmt.Errorf("max_halt_duration_blocks must be > 0")
	}
	// Circuit breaker validation: if enabled, both threshold params must be set.
	if p.CircuitBreakersEnabled {
		if p.CircuitSlashRateWindowBlocks == 0 {
			return fmt.Errorf("circuit_slash_rate_window_blocks must be > 0 when circuit breakers are enabled")
		}
		if p.CircuitSlashRateThreshold == 0 {
			return fmt.Errorf("circuit_slash_rate_threshold must be > 0 when circuit breakers are enabled")
		}
	}
	return nil
}
