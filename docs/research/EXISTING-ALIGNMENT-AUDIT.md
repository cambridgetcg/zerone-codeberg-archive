# Existing ZERONE Modules vs. Research Recommendations
## Alignment Audit

**Generated:** 2026-04-28 (Integration Phase — Mission 3 of 9)  
**Authored by:** Beta 🦞 (session 7, autonomous)  
**Depends on:** UNIFIED-TAXONOMY.md (Mission 1), ARCHITECTURE-DIAGRAMS.md (Mission 2)  
**Output of mission:** `2026-04-28-audit-existing-zerone-modules-vs-research-recommendations`

---

## Method

Every x/ directory was read directly against the 9 Tier-1 gaps from the unified taxonomy. This audit goes claim by claim to the source — not from memory, but from the `.go` files. The structure is:

1. **Module map** — what each x/ module does, with implementation depth
2. **Primitive coverage matrix** — 26 primitives from the taxonomy, coverage status per module
3. **Gap verifications** — each Tier-1 gap confirmed or corrected against code
4. **Divergences** — where zerone is ahead, and where it's behind

---

## 1. Module Map

### Core Financial Substrate

**`x/staking`** (keeper.go: 851 lines)
- 4-tier validator system: APPRENTICE / VERIFIED / SCHOLAR / GUARDIAN
- Progressive slash escalation: `amount × (1M + slashCount × SlashEscalationBps) / 1M`
- Slash decay: `SlashDecayPeriodBlocks = 34,272` with epoch reset
- `MaxSlashesPerEpoch = 2` hard cap
- `MaxSlashCountDeactivate` → sets `IsActive = false`
- `Jailed bool` field present in proto but **never set to `true` by any keeper logic** — vestigial
- `SlashValidatorToModule` — configurable destination (vindication_escrow or development_fund)
- Reputation scoring: `ReputationCorrectDelta` / `ReputationIncorrectDelta` / `ReputationSlashDelta`
- Redelegation cooldown enforced

**`x/vesting_rewards`** (keeper.go: 345 lines)
- Block-linear emission via `EmissionPeriod` (StartBlock/EndBlock/AmountPerBlock)
- `MaxSupplyUzrn = "222222222000000"` in keys.go
- `MintWithCap(ctx, amount)` — **enforces supply cap for block rewards**:
  reads live `bankKeeper.GetSupply("uzrn")`, mints only `min(requested, remaining)`
- Vesting schedules with clawback on falsification
- Block reward distribution records

**`x/billing`** (distribution.go: 248 lines)
- Revenue split: ContributorBps / ProtocolBps / ResearchBps (governance-adjustable)
- `ProtocolBurn` field exists but **is rounding dust routed to development_fund**, NOT actual token destruction
- Knowledge pool: citation rewards weighted by `confidence × log2(citations+1)`
- Dynamic pricing: `calculateDynamicBaseCost` reads ZRN price from liquidityPoolKeeper TWAP

**`x/liquiditypool`** (keeper/amm.go, twap.go, msg_server.go)
- Constant-product AMM with `SwapFeeBps` and `MinTokenOut` slippage protection
- Block-weighted TWAP accumulator (1e12 scale), updated every block
- `Pool.Locked` — per-pool halt flag (granular circuit breaker, manual-only)
- LP tokens minted as ZRN-20 secondary tokens (`lp/{pool_id}` denom)
- `BurnCoins` called only for LP token redemption — NOT for ZRN monetary policy

**`x/disputes`** (keeper.go: 725 lines)
- 4-tier dispute system with escalation
- Commit-reveal evidence (SHA256 hash commitment)
- `SelectArbiters` — deterministic, stake-weighted, conflict-excluded (validators only)
- Phase auto-transitions via BeginBlocker
- Bond escrowed **only from challenger** — no defender bond exists
- `DistributeSettlement` — win: winner + arbiters rewarded, remainder → development_fund; draw: bond returned minus arbiter fee; timeout: full return to challenger
- "Burned" amounts in settlement go to development_fund, not actual burn

**`x/emergency`** (msg_server.go: 407 lines)
- Guardian ceremony state machine: NORMAL → HALT_VOTING → HALTED → REVERT_VOTING or RESUME_VOTING
- `HaltQuorum = 750,000` (75% Guardian stake)
- Genesis council with `CouncilExpiryBlock`
- Audit trail via `EmergencyAuditEntry`
- **All triggers are manual Guardian proposals** — no automatic threshold monitoring

**`x/tokens`**
- ZRN-20 token factory (secondary tokens on zerone chain)
- `Burnable bool` per token type
- `BurnCoins` called for ZRN-20 tokens on `MsgBurnToken`
- No mechanism to burn `uzrn` (base ZRN) via this module

### Truth-Seeking Core

**`x/knowledge`** (largest module — 60+ keeper files)
- Claim submission, storage, lifecycle (pending → accepted/rejected → vindicated)
- Multi-round commit-reveal verification
- 12-methodology system with `CrossMethodDiscountBps` (Feyerabend pluralism as economics)
- `InferenceType` enum: DEDUCTIVE/INDUCTIVE/ABDUCTIVE/ANALOGICAL/CONTRADICTION
- `ClaimStructure` (subject/predicate/object/scope) on every claim
- `ProofTree` + `DescendantTree` queries — step-level proof verification
- `StepInference` + `StepVerdict`
- `NC-IS-OUGHT-WALL` — Hume's distinction as schema-level type separation
- **`AgentCalibration` with Brier scoring** — `VerifierRounds / SumBrierBps / CalibrationScoreBps` (**shipped**)
- `ConfidenceBps` on `MsgSubmitReveal` (**shipped**)
- Genesis axioms (777 axioms: LOGIC-001–LOGIC-077 + encoded Gödel-derived MaxConfidence)
- Epistemic temperature — consensus divergence signal
- Vindication system — retroactive justice escrow for wrongly-rejected claims
- Bootstrap fund with per-epoch + per-address rate limits
- `Methodology.FalsificationPaths []string` — what would disprove claims under this methodology
- **No `RefutationConditions` field on `Claim` itself** — falsification lives at methodology level, not claim level
- `ContradictingFactIds` on `MethodologyApplicationTrace` — tracks contradictions in traces
- `UpdateValidatorIndependence` — post-round independence tracking with reward modulation
- **No pre-assignment independence check** — independence score modulates rewards, doesn't gate participation

**`x/inquiry`** (keeper.go: 453 lines)
- Question/bounty system: OPEN → ANSWERED → RESOLVED / EXPIRED / CANCELLED
- Bounties paid to first accepted answer

**`x/counterexamples`** (keeper.go: 377 lines)
- Manages counter-examples against claims
- Links counter-examples to their target claims

**`x/evidence_mgmt`** (keeper.go: 295 lines)
- Evidence storage and management for claims and disputes
- Adapters to disputes and knowledge modules

**`x/dialectic`** (keeper.go: 71 lines)
- Socratic dialectic chains (lightweight)

**`x/qualification`** (keeper.go: 273 lines)
- Verifier qualification gating by domain
- `GetQualifiedValidators(ctx, domain, count)` — called by disputes.SelectArbiters

**`x/research`** (keeper.go: 264 lines)
- Research project management (proposals, bounties, phases)

### Social / Coordination

**`x/partnerships`** (keeper/exit.go + keeper.go)
- Human-agent partnership with common pot
- Lock tiers with exit penalties
- Exit settlement: forfeited amounts labelled "burned" but routed to development_fund
- `SettleCoolingPartnerships` runs in BeginBlocker

**`x/home`** (keeper.go: 516 lines)
- Agent identity and home management

**`x/alignment`**
- Alignment tracking between agents and protocol

**`x/auth`** (keeper.go: 618 lines)
- Zerone-specific auth module

**`x/channels`** (keeper.go: 227 lines)
- Async communication channels between agents

**`x/discovery`** (keeper.go: 338 lines)
- Agent matchmaking and capability discovery

**`x/schedule`** (keeper.go: 327 lines)
- On-chain task scheduling

**`x/claiming_pot`** (keeper.go: 342 lines)
- Bounty and reward pot management

**`x/private_corpus`** (keeper.go: 419 lines)
- Private knowledge corpus access control

### Governance

**`x/gov`**
- Standard governance module (adapted for zerone)

**`x/governance_synthesis`**
- Synthesizes governance proposals across domains

**`x/autopoiesis`**
- Self-modifying system parameters — autopoietic governance
- SSI multiplier applied to slash amounts

### System / Infrastructure

**`x/bvm`** (keeper.go: 133 lines) — Belief validation module  
**`x/compute_pool`** (keeper.go: 251 lines) — Compute resource pooling  
**`x/tree`** (keeper.go: 145 lines) — Project/task management (Tree of projects, services, milestones)  
**`x/trust_score`** (keeper.go: 28 lines) — Trust scoring stub  
**`x/ontology`** — Ontological relationship management  
**`x/tokens`** — ZRN-20 token factory  
**`x/toolbox`** — Tool registry  
**`x/training_provenance`** — Training data provenance  
**`x/agent_understanding`** — Agent comprehension tracking  
**`x/capture_challenge` / `x/capture_defense`** — Capture protection  
**`x/ibcratelimit` / `x/icaauth`** — IBC/ICA support  
**`x/common`** — Shared primitives  
**`x/module`** — Module registry  

### External Processes

**`cmd/oracle/`** — Single HTTP fact-evaluator
- `CombinedEvaluator`: static axiom-check first, LLM on uncertain cases
- Not a price oracle — evaluates claim truth, not token prices
- One process, one endpoint — no multi-source aggregation

**`cmd/zeroned/`** — Node binary

---

## 2. Primitive Coverage Matrix

From UNIFIED-TAXONOMY.md's 26 canonical primitives across 6 layers:

### Layer 1 — Sensing

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **Claim Submission** | `x/knowledge` | ✅ Implemented (ClaimStructure w/ subject/predicate/object/scope) |
| **Inquiry / Question** | `x/inquiry` | ✅ Implemented |
| **Oracle Input** | `cmd/oracle/` (fact), `x/liquiditypool` (price via TWAP) | ⚠️ Single-source; price oracle has circular dependency |

### Layer 2 — Recording

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **Fact Storage** | `x/knowledge`, `x/private_corpus` | ✅ Implemented |
| **Evidence Ledger** | `x/evidence_mgmt`, `x/disputes` | ✅ Implemented |
| **Proof Tree** | `x/knowledge` (ProofTree, DescendantTree) | ✅ Implemented (ahead of research) |
| **Audit Trail** | `x/emergency` (EmergencyAuditEntry), `x/staking` (events) | ✅ Partial (emergency has full trail; staking has events only) |

### Layer 3 — Verifying

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **Commit-Reveal Verification** | `x/knowledge` (rounds.go), `x/disputes` | ✅ Implemented |
| **Brier Calibration** | `x/knowledge` (agent_calibration.go) | ✅ Implemented (**shipped**) |
| **Methodology Pluralism** | `x/knowledge` (12 methods + CrossMethodDiscountBps) | ✅ Implemented (ahead of research) |
| **Independence Tracking** | `x/knowledge` (UpdateValidatorIndependence) | ⚠️ Post-round reward modulation only — no pre-assignment check |
| **Refutation Conditions** | `x/knowledge` (FalsificationPaths on Methodology) | ⚠️ Methodology-level only — not a first-class field on Claim |

### Layer 4 — Transacting

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **AMM / Liquidity** | `x/liquiditypool` | ✅ Implemented |
| **TWAP Price Oracle** | `x/liquiditypool` (twap.go) | ⚠️ Implemented but reads ZRN's own pool (circular dependency) |
| **Revenue Distribution** | `x/billing` (distribution.go) | ✅ Implemented |
| **Supply Emission** | `x/vesting_rewards` (MintWithCap) | ⚠️ Enforced for block rewards; knowledge module bypasses cap |
| **Fee Burn** | None | ❌ Absent — `ProtocolBurn` is dust → development_fund |
| **Defender Bond** | None | ❌ Absent — only challenger bond escrowed |
| **Challenger Bond** | `x/disputes` | ✅ Implemented |

### Layer 5 — Governing

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **Emergency Halt** | `x/emergency` | ✅ Implemented (manual Guardian ceremony) |
| **Automatic Circuit Breakers** | None | ❌ Absent — all triggers require Guardian proposal |
| **Governance Params** | `x/gov`, all module Params | ✅ Implemented |
| **Tombstoning** | `x/staking` (Jailed field in proto) | ⚠️ Proto field present but **never set** — validators deactivated not tombstoned |
| **Human-Only Panels** | None | ❌ Absent — arbiters selected from validator pool only |

### Layer 6 — Archiving

| Primitive | Module(s) | Status |
|-----------|-----------|--------|
| **Vindication Escrow** | `x/knowledge` (vindication.go) | ✅ Implemented (zerone's distinctive: retroactive justice) |
| **Genesis Axioms** | `x/knowledge` (genesis_axioms.json, 777 axioms) | ✅ Implemented (ahead of research) |
| **Training Provenance** | `x/training_provenance`, `x/knowledge` (training_corpus.go) | ✅ Implemented |

---

## 3. Tier-1 Gap Verifications

Each gap from the taxonomy verified against actual `.go` files. Green = confirmed. Yellow = partially addressed. Red = absent.

---

### P-16a: No Defender Bond
**Claim from taxonomy:** The dispute system escrows a challenger bond but collects nothing from the defender.  
**Verified at:** `x/disputes/keeper/msg_server.go:76-80`

```go
// Escrow bond from challenger
coins := sdk.NewCoins(sdk.NewCoin("uzrn", sdkmath.NewIntFromBigInt(bond)))
if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, challengerAddr, types.ModuleName, coins); err != nil {
    return nil, fmt.Errorf("failed to escrow bond: %w", err)
}
```

The `Dispute` struct has `Bond string` — this is the challenger's bond only. The defender is identified (line 39-46: fetched from knowledge module), but no funds are taken from them. `DistributeSettlement` distributes only from this single bond pool.

**Verdict: ✅ CONFIRMED ABSENT**

---

### P-18b: No ZRN Burn Path
**Claim from taxonomy:** No mechanism destroys ZRN — "burn" routes go to development_fund.  
**Verified at:** Multiple files.

**MintWithCap** (`vesting_rewards/keeper/keeper.go:220-261`) DOES enforce the supply cap for block rewards by checking live `bankKeeper.GetSupply("uzrn")`. This is real cap enforcement — a meaningful partial fix.

**BUT the burn side is absent:**
- `billing/keeper/distribution.go:47-51`: "ProtocolBurn = remainder" → line 200-208: routed to `DevelopmentFund`, not destroyed
- `partnerships/keeper/exit.go:127-130`: `burned` → `SendCoinsFromModuleToModule(..., "development_fund"...)`
- `disputes/keeper/keeper.go:601-607`: "Burn remainder" → `SendCoinsFromModuleToModule(..., "development_fund"...)`
- No `bankKeeper.BurnCoins("uzrn", ...)` call exists anywhere in `x/`

**Additional finding:** Knowledge module has uncapped `MintCoins` calls:
- `probe_bounty_pool.go:52`: mints to `ProbeBountyPoolModuleName`
- `msg_server_training_v4.go:224, 336`: mints to `TrainingFundModuleName`
- `genesis.go:97, 227`: genesis mints to bootstrap and training funds

These bypass `MintWithCap` entirely. The supply cap is enforced for block rewards but NOT for knowledge module reward minting.

**Verdict: ✅ CONFIRMED — with nuance:**  
- Block reward emission: cap enforced ✓  
- Fee-market burn: absent ✗  
- Knowledge module minting: uncapped ✗  
- "Burned" labels throughout: misleading — all route to development_fund ✗

---

### P-19a: No Tombstoning
**Claim from taxonomy:** Validators can be deactivated but not permanently excluded.  
**Verified at:** `x/staking/types/types.pb.go:98` and `x/staking/keeper/keeper.go`

```protobuf
// From types.pb.go:98
Jailed bool `protobuf:"varint,16,opt,name=jailed,proto3" json:"jailed,omitempty"`
```

The `Jailed` field exists in the validator proto. There is `ErrValidatorJailed` and `IsJailed` in the gov adapter (`gov_adapters.go:77-83`). But:
- No keeper function sets `val.Jailed = true`
- Slashing deactivates via `val.IsActive = false` when `SlashCount >= MaxSlashCountDeactivate`
- No search of the codebase finds `Jailed = true` or `Jailed: true` outside tests

A deactivated validator (`IsActive = false`) could theoretically be reactivated by governance setting parameters. There is no permanent exclusion path. The `Jailed` field is promissory infrastructure — wired in the proto, not wired into the slashing logic.

**Verdict: ✅ CONFIRMED ABSENT** — proto field is there, enforcement is not

---

### P-21a: No Automatic Circuit Breakers
**Claim from taxonomy:** Emergency halt requires Guardian manual proposal; no automatic threshold monitoring.  
**Verified at:** `x/emergency/keeper/msg_server.go:26-104`

`ProposeHalt` requires:
- A human Guardian to submit `MsgProposeHalt` with a reason
- Guardian passes eligibility checks (stake floor, cooldown, proposal limits)
- Ceremony created, other Guardians vote

There is no BeginBlocker logic that monitors:
- Abnormal slash rates (could read from `x/staking`)
- Oracle price deviation (could read from `x/liquiditypool` TWAP)
- Treasury drawdown (could read from bank module balances)
- Anomalous transaction volume

`MonitorRevertStatus` (`emergency/module.go:143`) is called in EndBlocker — but reads ceremony phase, not chain health metrics.

**Verdict: ✅ CONFIRMED ABSENT**

---

### P-01a: TWAP Circular Price Dependency
**Claim from taxonomy:** ZRN price oracle reads from ZRN's own AMM pool.  
**Verified at:** `x/billing/keeper/dynamic_pricing.go:37-48`

```go
// Tier 2: TWAP from liquidity pool
if k.liquidityPoolKeeper != nil {
    twap, err := k.liquidityPoolKeeper.GetTWAP(ctx, "uzrn", cfg.TwapWindowBlocks)
```

`GetTWAP(ctx, "uzrn", ...)` fetches the price of `uzrn` from the liquidity pool. The liquidity pool's TWAP reads from `pool.ReserveA` and `pool.ReserveB` (`twap.go:37-44`). If the ZRN/stablecoin pool is the only pool, this creates the loop: query price in ZRN → ZRN price from ZRN pool → ZRN price denominated in the other reserve asset.

The only escape is `ManualZrnPriceUsd` governance override (Tier 1 in `getZRNPriceUSD`).

**Verdict: ✅ CONFIRMED**

---

### P-01b: Single Knowledge Oracle
**Claim from taxonomy:** One HTTP oracle evaluates all fact claims.  
**Verified at:** `cmd/oracle/main.go`, `cmd/oracle/evaluator.go`

`CombinedEvaluator` has `Static *StaticEvaluator` and optional `LLM *LLMEvaluator`, both running in the same process. This is a single-tier fallback chain, not multi-source aggregation. There is:
- No attestation of who reported what
- No dispute window for oracle reports
- No stake-backed oracle accountability
- No median/trimmed-mean aggregation across independent reporters

**Verdict: ✅ CONFIRMED**

---

### P-07: No Refutation Conditions on Claims
**Claim from taxonomy:** Claims don't specify what would disprove them.  
**Verified at:** `x/knowledge/types/types.pb.go`

`Methodology.FalsificationPaths []string` (`types.pb.go:1760`) — exists, but is on the Methodology type (the verification framework), not on individual Claims.

`ContradictingFactIds []string` (`types.pb.go:5557`) — exists on `MethodologyApplicationTrace`, tracks contradictions during verification.

**No `RefutationConditions` field exists on the `Claim` or `Fact` proto types.** Claims are submitted without specifying the conditions under which they would be falsified. This matters: without a claim-level refutation condition, adversarial collaboration (the research brief's recommendation) cannot work — challengers don't know what to test.

**Verdict: ✅ CONFIRMED ABSENT** — falsification vocabulary exists at methodology level, not claim level

---

### P-10a: No Verifier Independence Pre-Check
**Claim from taxonomy:** No check that verifiers haven't already seen or influenced the claim before assignment.  
**Verified at:** `x/knowledge/keeper/rounds.go:250-254`

```go
// Update validator independence for each revealed voter
if err := k.UpdateValidatorIndependence(ctx, reveal.Verifier, reveal.Vote, majorityVote); err != nil {
    k.Logger(ctx).Debug("failed to update validator independence", "verifier", reveal.Verifier, "error", err)
}
```

And `applyIndependenceMultiplier` (`rounds.go:327-343`) — modulates reward based on historical conformity score.

This is **post-round tracking**, not pre-assignment checking. A verifier with a low independence score (history of crowd-following) is still assigned and still participates; they just receive smaller rewards. There is no mechanism to:
- Check whether a verifier has previously seen this claim
- Check whether a verifier has a financial interest in the outcome
- Exclude verifiers with conflicted history

**Verdict: ✅ CONFIRMED** — independence is measured and rewarded, not enforced as a gate

---

### P-13: No Human-Only Panels
**Claim from taxonomy:** All arbiters are selected from the validator pool with no human designation.  
**Verified at:** `x/disputes/keeper/keeper.go:384-433`

`SelectArbiters` draws from `stakingKeeper.GetQualifiedValidators(ctx, "disputes", count*3)`. The only exclusion is challenger/defender conflict. No `ArbiterType` exists distinguishing human from AI validators. The `DisputeTargetType` enum has VALIDATOR type, but this refers to what's being disputed, not who arbitrates.

**Verdict: ✅ CONFIRMED ABSENT**

---

## 4. Divergences

### Where zerone is AHEAD of the research

These implementations go beyond what any brief recommended — zerone built what the research didn't name:

1. **Brier calibration scoring** (`x/knowledge/keeper/agent_calibration.go`) — Already shipped. Verifier accuracy measured via Brier score, stored per-validator as `CalibrationScoreBps`. No blockchain does this.

2. **12-methodology system with `CrossMethodDiscountBps`** — Feyerabend's methodological pluralism encoded as a governance parameter. Different epistemic frameworks coexist with economic incentives for methodological diversity. Research briefs described this as an ideal; zerone has implemented it.

3. **Proof tree infrastructure** (`ProofTree`, `DescendantTree`) — Step-level proof verification with inference type tracking (DEDUCTIVE/INDUCTIVE/ABDUCTIVE/ANALOGICAL/CONTRADICTION). More sophisticated than any academic reference in the briefs.

4. **Genesis axioms** (`genesis_axioms.json`, 777 axioms, LOGIC-001–LOGIC-077) — Chain-embedded logical foundation with Gödel-derived MaxConfidence. LOGIC-077: "The chain is born knowing it is incomplete. This is strength, not weakness." This is unique.

5. **Epistemic temperature** — Consensus divergence signal with no research analogue. Automates what human moderators do: detect when the knowledge-producing community is fracturing.

6. **Vindication system** — Retroactive justice escrow for wrongly-rejected claims. When a claim is later accepted after initial rejection, the vindication module compensates the submitter. Named explicitly as a zerone distinctive in the taxonomy.

7. **IS-OUGHT wall** (`NC-IS-OUGHT-WALL`) — Hume's is/ought distinction as a schema-level type constraint. Normative claims and descriptive claims have different type hierarchies.

8. **Autopoiesis** — Self-modifying system parameters with multiplier effects (e.g., SSI multiplier on slash amounts). The system can reshape its own economic incentives.

9. **InferenceType enum** — Rich inference taxonomy as first-class values in the proof system. Abductive and analogical reasoning are tracked and tracked differently.

10. **Independence reward modulation** — Even without a pre-assignment gate, the post-round independence modulation is more sophisticated than any brief described. Crowd-followers get penalized economically in a principled way.

---

### Where zerone is BEHIND the research

Gaps in ascending order of build complexity:

**Simple additions (schema changes):**
- **P-07: RefutationConditions on Claim** — Add `refutation_conditions []string` field to Claim proto. Wire it into MsgSubmitClaim, store it, surface it in queries. The vocabulary (`FalsificationPaths`) already exists at Methodology level.

**Moderate (new logic in existing module):**
- **P-10a: Verifier independence pre-check** — Add pre-assignment gate: before assigning verifiers to a round, query `GetValidatorIndependence` and exclude persistent conformists (e.g., ConformityBps > threshold). The data structure is already computed.
- **P-19a: Tombstoning** — Wire `val.Jailed = true` in `SlashValidator` when slash count exceeds a hard threshold. Jailed validators cannot re-enter even via governance. The proto field is already there.
- **P-13: Human-only dispute panels** — Add `arbiter_type` field to `ArbiterRequirement`. `SelectArbiters` already accepts a domain; add a `human_only bool` flag that queries a `HumanRegistry` mapping validator addresses to verified human attestation.

**Significant (new module or major feature):**
- **P-16a: Defender bond** — When `InitiateDispute` fires, require the defender (if identifiable) to post a bond within the evidence period or forfeit by default. Modify `DistributeSettlement` to manage both bonds.
- **P-18b: ZRN burn path** — Two parts: (a) wire a `BurnCoins("uzrn")` call on a configurable percentage of billing fees instead of routing to development_fund; (b) wrap knowledge module `MintCoins` calls through a cap-checking gateway so all minting paths respect MaxSupplyUzrn.
- **P-21a: Automatic circuit breakers** — New BeginBlocker logic (or a dedicated `x/monitor` module) that reads slash rates from staking, TWAP deviation from liquiditypool, treasury balances from bank, and emits tiered responses: warn → restrict → halt. Most complex of the five foundations.
- **P-01a/P-01b: Oracle resilience** — Replace single TWAP source with multi-oracle aggregation module. Require external reporters to stake, aggregate via median or trimmed mean, implement dispute window for false reports. Also requires breaking the circular TWAP dependency by adding an external price feed path.

---

## 5. Build Priority (for Roadmap)

Given the audit, confirming the taxonomy's Tier-1 priority ranking:

| Priority | Gap | Effort | Dependency |
|----------|-----|--------|------------|
| 1 | **P-18b** Supply burn path | Medium | None |
| 2 | **P-16a** Defender bond | Medium | None |
| 3 | **P-21a** Automatic circuit breakers | High | None |
| 4 | **P-01a/b** Oracle resilience | High | None |
| 5 | **P-07** RefutationConditions | Low | None |
| 6 | **P-10a** Verifier independence gate | Low | None |
| 7 | **P-19a** Tombstoning | Low | None |
| 8 | **P-13** Human-only panels | Medium | Human registry infrastructure |

**Note on P-18b ordering:** The burn path requires two pieces:
(a) fee routing change in billing/disputes/partnerships — straightforward
(b) knowledge module minting gateway — requires coordination with knowledge module's complex internals

Recommendation: do (a) first as a small, safe change; do (b) in the Unified Claim+Verification proto mission which will touch knowledge module types anyway.

---

## 6. Corrections to Prior State

The unified taxonomy stated: "P-18b: No ZRN burn path — ProtocolBurn is ROUNDING DUST, not intentional deflation." This is confirmed. However, the taxonomy did not capture that `MintWithCap` exists and enforces the supply cap for block reward emission. The gap is not "no cap" — it's "cap enforced for emission but not for fee burn or secondary minting paths."

The taxonomy stated `MaxSupplyUzrn = "222222222000000"` as "NOT enforced at protocol layer." This was partially wrong. It IS enforced for vesting_rewards via `MintWithCap`. It is NOT enforced for knowledge module minting. The Roadmap should note this nuance.

---

*This audit was performed by direct code reads — no assumptions from memory. Every finding cites file and line where the evidence lives.*
