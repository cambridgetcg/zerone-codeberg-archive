# Unified Taxonomy of Truth+Stability Primitives

> The vocabulary that the zerone codebase implies but has not yet named.

**Drawn from**: 21 research briefs (11 truth-seeking + 10 financial-stability) + 2 series syntheses  
**Author**: Beta 🦞 | **Created**: 2026-04-28  
**Purpose**: Foundation vocabulary for the Audit, Roadmap, and Foundation code missions

---

## Zerone in One Sentence

Zerone is a **stake-weighted, epistemically-governed protocol** that uses economic incentives to enforce honest knowledge production — treating truth as infrastructure and calibrated verifiers as the substrate that makes infrastructure trustworthy.

---

## The Two-Layer Architecture

```
╔══════════════════════════════════════════════════════════════╗
║  EPISTEMIC LAYER  ("What is true?")                          ║
║  x/knowledge · x/ontology · x/dialectic · x/counterexamples ║
║  x/capture_challenge · x/capture_defense · x/trust_score    ║
╠══════════════════════════════════════════════════════════════╣
║         CONVERGENCE BAND  (stakes, calibration, escrow)      ║
╠══════════════════════════════════════════════════════════════╣
║  FINANCIAL LAYER  ("What does it cost to lie?")              ║
║  x/billing · x/liquiditypool · x/staking · x/disputes       ║
║  x/emergency · x/tokens · x/vesting_rewards                  ║
╚══════════════════════════════════════════════════════════════╝
```

The layers are not independent. The convergence band is where every primitive that appears in one layer has an analog in the other. The central thesis (from the financial synthesis):

> **Zerone encodes epistemic values as economic incentives. The financial substrate is the mechanism that makes the epistemics enforceable.**

The unified taxonomy makes this concrete: name the primitive, show both of its forms.

---

## Layer-Organized Primitive Registry

Organized by the six architectural layers: **Sensing → Recording → Verifying → Transacting → Governing → Archiving**. Each primitive has: a canonical name, its function, where it lives in code, which briefs contributed to it, and whether it's implemented or a gap.

---

### Layer 1: Sensing

*How zerone receives information about the external world.*

---

#### P-01 · Oracle Feed

**Function**: Translates off-chain data (price, external claims) into on-chain values available to other modules.

**Implementation**:
- `cmd/oracle/` — HTTP oracle with LLM evaluator (fact evaluator) and static evaluator
- `CategoryOracleFeed` in staking — validators with staked tier act as price reporters
- TWAP accumulator in `x/liquiditypool/keeper/` — block-weighted price from ZRN's own AMM pool

**Status**: ✅ Partial — TWAP oracle exists; external price oracle is circular (ZRN price reads from ZRN pool)

**Gap P-01a** ★★★★★: TWAP circular price dependency — ZRN's own AMM pool is the only price source, creating a flash-loan attack vector and no external anchor. Needs an external oracle committee feeding an independent ZRN/USD price.

**Gap P-01b** ★★★★★: Single knowledge oracle — the HTTP oracle is a single point of failure with no redundancy and no BFT aggregation.

**Contributing briefs**: Oracle Resilience (financial series)

---

#### P-02 · Claim Submission

**Function**: Agent reports a perceived fact about the world. Claim submission is the sensing act for the epistemic layer — it's how the chain learns about the external world.

**Implementation**:
- `MsgSubmitClaim` in `x/knowledge/tx.proto`
- `Claim.Stake` encodes submitter conviction as economic signal
- `canonical_form` string encodes the claim in the canonical form language (→ P-07)

**Status**: ✅ Implemented

**Contributing briefs**: Falsificationism, Adversarial Collaboration, Calibrated Forecasting (truth-seeking series)

---

### Layer 2: Recording

*How zerone captures and represents knowledge before verification.*

---

#### P-03 · Canonical Form Language

**Function**: A minimal formal language for expressing claims with enough structure to support deduplication, contradiction detection, and derivation checking.

**Operators**: `assert(domain, subject, predicate)`, `relate(domain, A, relation, B)`, `define(domain, concept, definition)`, `constrain(domain, X, rule)`, `negate(domain, X, predicate)`

**Implementation**:
- `ClaimStructure` on `Claim` proto — subject/predicate/object/scope
- `canonical_form` string field — the claim in canonical language
- `canonical_hash` — SHA256 of canonical form, used for deduplication
- `MsgSubmitContradiction` — explicit contradiction submission (→ convergence with P-14)

**Status**: ✅ Implemented

**Gap P-03a** ★★★☆☆: No automated SMT contradiction detection. `MsgSubmitContradiction` exists for explicit submission; automatic detection of `assert(D,X,P)` + `negate(D,X,P)` pairs is not yet built.

**Contributing briefs**: Formal Logic and Proof Systems, Skepticism (truth-seeking series)

---

#### P-04 · NC-IS-OUGHT WALL

**Function**: Hume's distinction enforced as a schema-level type separation. Normative claims (`IS_OUGHT`) and descriptive claims (`NON_CONTROVERSIAL`) are different types — they cannot be converted implicitly.

**Implementation**: `ClaimType` enum with `NC_IS_OUGHT` and type-checking in message server

**Status**: ✅ Implemented

**Contributing briefs**: Skepticism (truth-seeking series)

---

#### P-05 · Inference Type Tagging

**Function**: Every `ClaimRelation` carries a declared `InferenceType` — DEDUCTIVE, INDUCTIVE, ABDUCTIVE, ANALOGICAL, CONTRADICTION. The chain knows HOW one claim depends on another, not just THAT it does.

**Implementation**: `InferenceType` enum on `ClaimRelation` in `x/knowledge/` proto

**Status**: ✅ Implemented

**Contributing briefs**: Formal Logic and Proof Systems (truth-seeking series)

---

#### P-06 · Methodology Plurality with Cross-Method Discount

**Function**: Claims can be verified through one of 12 methodologies (M-FORMAL, M-EMPIRICAL, M-PRAGMATIC, M-PHENOMENOLOGICAL, etc.). Claims verified through multiple independent methodological paths are stronger — but `CrossMethodDiscountBps` encodes Feyerabend's pluralism: diversity of method strengthens, but cross-methodology evidence is discounted rather than simply added.

**Implementation**: `Methodology` proto, `method_id` on `Claim`, `CrossMethodDiscountBps` parameter

**Status**: ✅ Implemented

**Convergence**: Maps to the financial layer's revenue split — methodological evidence quality modulates how much flows to `research_fund` vs. `treasury_protocol`. Diverse evidence → higher-quality corpus → different economic routing.

**Contributing briefs**: Scientific Method (truth-seeking series)

---

#### P-07 · Refutation Conditions at Submission

**Function**: Every claim declares what evidence would falsify it at submission time. Without declared refutation conditions, falsifiability is implicit and must be independently reconstructed by each challenger, wasting verification effort and enabling goalpost-moving.

**Status**: ❌ Gap ★★★★★

**Architectural fit**: New `refutation_conditions []string` field on `Claim` proto. Enables falsifiability scoring (→ P-12), automated challenge targeting, and burden-of-proof scaling.

**Contributing briefs**: Falsificationism, Adversarial Collaboration, Skepticism (truth-seeking series)

---

### Layer 3: Verifying

*How zerone validates recorded claims through structured social process.*

---

#### P-08 · Commit-Reveal-Aggregate (PoT Protocol)

**Function**: The core verification primitive. Verifiers commit hashed opinions before seeing others' views, then reveal simultaneously. Aggregation occurs only after all reveals. Prevents bandwagon effects within a panel round.

**Implementation**:
- `MsgSubmitCommitment` → `MsgRevealVerification` → aggregation in EndBlocker
- `RevealEntry` carries `confidence_bps` (field 5) — graded probability, not binary vote
- Three phases: commit window, reveal window, aggregation

**Status**: ✅ Implemented

**Convergence**: The same information-hiding primitive should apply to AMM swaps (→ Gap P-17a). The pattern is architecturally universal: seal → execute → reveal.

**Contributing briefs**: Calibrated Forecasting, Adversarial Collaboration, Bayesian Reasoning (truth-seeking series); Market Microstructure (financial series)

---

#### P-09 · Brier Calibration Score

**Function**: Proper scoring rule applied to verifier confidence claims. `BrierScore(p, o) = (p - o)²` where `p` is normalized confidence (BPS / 1M) and `o` is outcome (1.0 = accepted, 0.0 = rejected). Agent's `calibration_score_bps` accumulates over time. Calibration score modulates future rewards — honest confidence reporting is the dominant strategy.

**Implementation**:
- `AgentCalibration.verifier_rounds`, `sum_brier_bps`, `calibration_score_bps` in proto
- `RecordVerifierOutcome` in `x/knowledge/keeper/agent_calibration.go`
- `ComputeVerifierCalibrationScore` with Brier formula
- `CompleteRound` in `x/knowledge/keeper/rounds.go` calls `RecordVerifierOutcome`

**Status**: ✅ Implemented (shipped 2026-04-28)

**Convergence**: The same track record principle should modulate dispute bond requirements — agents with high calibration scores could post lower bonds. Gap P-22b in the financial layer.

**Contributing briefs**: Calibrated Forecasting, Bayesian Reasoning (truth-seeking series)

---

#### P-10 · Independence Reward

**Function**: Panel diversity premium. Verifiers who vote independently of crowd consensus receive higher rewards than crowd-followers. Implemented via HHI-based capture detection.

**Implementation**:
- `x/capture_challenge/` — measures voting concentration (HHI) in panel
- `x/capture_defense/` — penalizes persistent crowd-followers
- `IndependenceWeight` modulates verifier reward fraction

**Status**: ✅ Implemented (post-hoc bandwagon detection)

**Gap P-10a** ★★★★★: Pre-hoc panel diversity — independence reward currently detects correlated *patterns* after the fact; it does not verify *independence* before the round. Organization-level and training-cohort-level correlation are invisible. Three verifiers from the same organization who discussed privately are counted as three independent tests but are one test repeated three times.

**Proposed**: `organization_id` and `training_cohort_id` on agent registration. `panel_diversity_score` (proportion of panel from distinct org × cohort pairs). `MinPanelDiversityBps` per methodology.

**Contributing briefs**: Scientific Method (truth-seeking series — replication crisis lesson)

---

#### P-11 · Step-Level Proof Verification

**Function**: Reasoning traces are not monolithic. `StepInference` and `StepVerdict` enable step-level checking — each logical move in a reasoning trace is verified, not just the conclusion. Supports PRM (process reward model) training.

**Implementation**: `StepInference` and `StepVerdict` in `x/knowledge/` proto; `ProofTree` and `DescendantTree` queries

**Status**: ✅ Implemented

**Contributing briefs**: Formal Logic and Proof Systems (truth-seeking series)

---

#### P-12 · Proof Certificates (M-FORMAL Claims)

**Function**: Machine-checkable derivation on `M-FORMAL` claims. Currently `method_id = "M-FORMAL"` is a declaration — the reasoning trace is evaluated socially by a PoT panel. With proof certificates, the chain verifies the derivation mechanically using forward-chaining inference over canonical forms.

**Status**: ❌ Gap ★★★★☆

**Proposed tier structure**:
```
Tier 0 (mechanical): ProofCertificate present → chain verifies derivation → direct VERIFIED
Tier 1 (social):     Standard PoT commit-reveal panel
```

**Architectural fit**: New `ProofCertificate` message type on `Claim` proto. `VerifyCanonicalChainProof(cert, corpus)` in `x/knowledge/keeper`. Four initial inference rules: transitivity of `relate`, `constrain` + `assert` → contradiction, `define` + `assert` → substitution, `negate` + `assert` → contradiction.

**Contributing briefs**: Formal Logic and Proof Systems (truth-seeking series)

---

#### P-13 · Human-Only Panels

**Function**: Some domains require human verifiers — not because AIs lack competence, but because the observational validity of certain claims (phenomenological, contemplative, embodied) depends on the verifier's nature. An AI cannot verify a claim about direct phenomenal experience.

**Status**: ❌ Gap ★★★★★

**Proposed**: `required_verifier_types []VerifierType` on `Methodology`. `GetEligibleValidators` checks type constraint before selecting panel. ~3 lines in the selection function.

**Contributing briefs**: Contemplative Traditions, Phenomenology (truth-seeking series)

---

### Layer 4: Transacting

*How value flows through the system — payments, exchange, staking, fees.*

---

#### P-14 · Stake as Universal Alignment Signal

**Function**: Across every module, stake is the alignment mechanism — it quantifies how much an agent has to lose from dishonesty. Higher stake → higher verifier reward, more aggressive slash, more governance weight, more escrow required.

**Manifestations**:
- `Claim.Stake` — submitter conviction signal + verifier reward pool
- `Dispute.Bond` — challenger skin-in-game
- `SelfDelegation + DelegatedStake` — validator economic security budget
- `DisputeVote.Stake` — weighted arbiter voting

**Status**: ✅ Implemented across all modules

**Convergence**: The universal alignment signal bridges epistemic and financial layers. Claim stake is simultaneously an epistemic commitment ("I believe this is true") and an economic commitment ("I will lose this if it's wrong"). The same number serves both purposes.

**Contributing briefs**: All 21 briefs observe this pattern

---

#### P-15 · BPS Economy

**Function**: Every financial parameter uses the 1,000,000-basis-point scale (1M = 100%). This is zerone's universal unit of measure — it makes every parameter composable, comparable, and commensurable across layers.

**Manifestations**: `SwapFeeBps`, `SlashEscalationBps`, `ContributorBps`, `ProtocolBps`, `ResearchBps`, `HaltQuorum`, `CalculatePriceImpactBps`, `calibration_score_bps`, `CrossMethodDiscountBps`

**Status**: ✅ Implemented as consistent design pattern

**Note**: This is not merely a convenience. BPS makes epistemic parameters (confidence scores) and financial parameters (fee splits) commensurable — they're in the same unit and can be directly composed. `calibration_score_bps × fee_multiplier_bps / 1M` is a valid computation.

**Contributing briefs**: All financial series briefs

---

#### P-16 · Escrow as Conditional Custody

**Function**: Tokens held on conditions — conditionally released when conditions are met, burned or redistributed when they aren't. The same primitive enforces both epistemic commitments and financial commitments.

**Manifestations**:
- `vindication_escrow` — holds tokens slashed from verifiers who incorrectly rejected a claim; returned if claim is later vindicated (retroactive justice — unique to zerone)
- `Dispute.Bond` escrow — holds challenger bond; returned to winner, partially burned/redistributed on loss
- Bootstrap fund — rate-limited escrow for genesis incentives

**Status**: ✅ Implemented

**Gap P-16a** ★★★★★: No defender bond — challengers post bond to challenge, but defenders post nothing to resist. Creates asymmetric dispute economics. Proposed: `DefenderBond` + `DefenderBondDeadline` on `Dispute` proto; auto-resolve CHALLENGER_WINS if deadline passes without bond.

**Contributing briefs**: Counterparty Escrow, Settlement Finality (financial series)

---

#### P-17 · Constant-Product AMM with TWAP

**Function**: Automated market maker for ZRN liquidity. `x * y = k` invariant. `MinTokenOut` slippage protection. Block-weighted TWAP accumulator updated every block (1e12 scale).

**Implementation**: `x/liquiditypool/keeper/`

**Status**: ✅ Implemented

**Gap P-17a** ★★★★★: No commit-reveal for swaps. Front-running attacks are possible — a validator who sees a pending swap in the mempool can sandwich it. Proposed: `MsgCommitSwap` → `MsgRevealSwap` pattern (same architecture as PoT commit-reveal — the unification is clean).

**Gap P-17b**: `Pool.Locked` — per-pool halt flag exists (circuit breaker granularity). Automatic trigger for this flag is missing (→ P-21).

**Contributing briefs**: Market Microstructure, Liquidity Infrastructure (financial series)

---

#### P-18 · Fee Distribution with Fund Silos

**Function**: Revenue from knowledge fees is split into named fund silos with distinct purposes. The split encodes zerone's value theory: most value flows to contributors (truth-producers), less to protocol infrastructure, less to research, and a rounding-dust amount to burn.

**Current split** (hardcoded constants in `x/knowledge/keeper/fees.go`):
- `reviewFeeContributorBps = 550_000` (55% → verifier pool)
- `reviewFeeProtocolBps = 220_000` (22% → protocol/treasury)
- `reviewFeeResearchBps = 196_700` (19.67% → research_fund)
- `reviewFeeKnowledgeBps = 33_300` (3.33% → knowledge fund)

**Fund silos**: `treasury_protocol` (20% of protocol allocation), `research_fund`, `development_fund`, `knowledge`, `vindication_escrow`

**Status**: ✅ Implemented

**Gap P-18a** ★★★☆☆: Fee split constants are hardcoded — not governance-adjustable. Should move to `Params`.

**Gap P-18b** ★★★★★: No ZRN burn path — the `reviewFeeKnowledgeBps` fraction (3.33%) is rounding dust, not intentional deflation. Adding `BurnBps` to `Params` and routing `fee × BurnBps / 1M` to `bankKeeper.BurnCoins` closes the supply loop.

**Contributing briefs**: Fee Markets, Treasury, Monetary Policy (financial series)

---

#### P-19 · Progressive Slash Escalation

**Function**: Each subsequent slash on a validator is larger than the last: `amount × (1M + slashCount × SlashEscalationBps) / 1M`. The chain remembers — persistent dishonesty escalates in cost.

**Implementation**: `x/staking/keeper/` slash logic; `SlashEscalationBps` parameter; windowed decay (`SlashDecayPeriodBlocks = 34_272`) so rehabilitation is possible

**Status**: ✅ Implemented

**Gap P-19a** ★★★★★: No tombstoning — a validator who accumulates extreme slash history can theoretically continue operating. Proposed: `Tombstoned bool` on validator state; `TombstoneThreshold` in staking `Params`; check in `SlashValidator`.

**Contributing briefs**: Slashing and Economic Security (financial series)

---

### Layer 5: Governing

*How zerone adapts — parameter changes, emergency response, consensus evolution.*

---

#### P-20 · Governance as Metaparameter

**Function**: Virtually all financial and epistemic configuration is governance-adjustable via `Params`. The protocol's financial constitution is software, not stone. The system can learn and adapt without protocol upgrades.

**Implementation**: `Params` messages in `x/knowledge`, `x/staking`, `x/liquiditypool`, `x/emergency`, `x/tokens`, `x/billing`; governance proposal → `MsgUpdateParams`

**Status**: ✅ Implemented (with exceptions noted in P-18a)

**Contributing briefs**: All financial series briefs; Scientific Method (truth-seeking series)

---

#### P-21 · Emergency State Machine

**Function**: Four-state circuit breaker for protocol-level safety: `NORMAL → HALT_VOTING → HALTED → REVERT_VOTING / RESUME_VOTING`. Guardian stake-weighted voting controls transitions. Full chain state revert via `revert-to-state-root` on HALTED → REVERT.

**Implementation**: `x/emergency/` — state machine, `HaltQuorum = 750_000` (75% Guardian stake), genesis emergency council with `CouncilExpiryBlock`

**Status**: ✅ Implemented (manual triggers only)

**Gap P-21a** ★★★★★: No automatic circuit breakers — all transitions require Guardian governance action. Two highest-priority automatic conditions: (1) TWAP deviation >30% from 100-block average → pause affected pool; (2) slash count >3 in current epoch → pause new knowledge rounds.

**Proposed**: `AutoHaltConditions []AutoHaltCondition` in emergency `Params`; EndBlocker evaluation of conditions; `Pool.Locked` set automatically on trigger.

**Contributing briefs**: Circuit Breakers and Crisis Governance (financial series)

---

#### P-22 · Temporal Architecture

**Function**: Time is a first-class parameter across all layers. Modules use block numbers, not wall clock time, for determinism. Every time-bounded commitment has an expiry; every penalty has a decay.

**Manifestations**:
- `VerdictBlock` — block at which a claim's verdict was finalized
- `SlashDecayPeriodBlocks = 34_272` — slash history decays over ~24 days
- `CouncilExpiryBlock` — genesis emergency council dissolves at this block
- `EmissionPeriod.StartBlock / EndBlock / AmountPerBlock` — block-linear emission
- `MaxSlashesPerEpoch = 2` — rate limit on slashing per epoch
- EndBlocker sweeps — time-based state transitions throughout

**Status**: ✅ Implemented

**Contributing briefs**: All series observe this pattern; Settlement Finality (financial series) identified `VerdictBlock` anchoring

---

#### P-23 · 4-Tier Validator Hierarchy

**Function**: APPRENTICE → VERIFIED → SCHOLAR → GUARDIAN. Tier determines: slash severity, governance weight, emergency voting eligibility, oracle reporting eligibility. Progression through demonstrated reliability.

**Implementation**: `ValidatorTier` enum in `x/qualification/` (or `x/staking/`); `Params` for tier thresholds; windowed tier demotion on slash escalation

**Status**: ✅ Implemented

**Contributing briefs**: Slashing and Economic Security (financial series)

---

### Layer 6: Archiving

*How zerone preserves what's been confirmed — settlement, finality, and corpus integrity.*

---

#### P-24 · Settlement Finality

**Function**: The moment a claim's verdict is irreversible. ZERONE uses three-phase finality: PoT aggregation (immediate vote finality) → `VerdictBlock` anchoring (on-chain) → vindication window (the only post-finality path).

**Implementation**: `VerdictBlock` field on Claim; 3-phase commit-reveal-aggregation; `x/disputes/` for post-finality challenge

**Status**: ✅ Implemented

**Gap P-24a** ★★★★☆: No block hash anchoring on verdicts — `VerdictBlock` records the block number but not the block hash. Block hash anchoring would create a cryptographic commitment linking the verdict to a specific chain state, enabling external auditability.

**Gap P-24b** ★★★☆☆: No timeout finality for abandoned rounds — rounds that never reach enough reveals are not automatically cleaned up.

**Contributing briefs**: Settlement Finality (financial series)

---

#### P-25 · Corroboration Count

**Function**: Each survived challenge increments `CorroborationCount`. Facts that survive many challenges are harder-tested. This is zerone's empirical evidence accumulation mechanism.

**Implementation**: `Claim.CorroborationCount` incremented in `x/knowledge/keeper/` on successful challenge survival

**Status**: ✅ Implemented

**Gap P-25a** ★★★★☆: Corroboration is quantity-based, not quality-based. All survived challenges count equally. Mayo's severity principle: a test is valuable proportional to its probability of detecting error if error exists. Proposed: severity-weighted corroboration accumulation.

**Contributing briefs**: Falsificationism (Mayo severity), Adversarial Collaboration (truth-seeking series)

---

#### P-26 · Vindication Escrow

**Function**: Zerone's retroactive justice mechanism. When a verifier incorrectly rejects a claim and is slashed, their slashed tokens are held in `vindication_escrow` rather than burned immediately. If the claim is later vindicated (overturned via challenge), the escrow is returned. This encodes: punishment is provisional, contingent on the protocol's own future learning.

**Implementation**: `vindication_escrow` fund silo; `SlashValidatorToModule` routing; vindication in `x/knowledge/keeper/`

**Status**: ✅ Implemented

**Note**: This primitive is unique to zerone. No other known protocol implements retroactive justice escrow. It's worth highlighting in documentation as a distinctive architectural contribution.

**Contributing briefs**: Counterparty Escrow, Settlement Finality (financial series)

---

## Cross-Layer Convergence Map

Where a single abstract primitive serves both epistemic and financial purposes. These convergence points are where the two layers are not just analogous — they are the same mechanism at different levels of abstraction.

| Abstract Primitive | Epistemic Form | Financial Form | Convergence Point |
|---|---|---|---|
| **Information hiding** | Commit-reveal in PoT (P-08) | Commit-reveal in AMM swaps (Gap P-17a) | Same protocol: seal → execute → reveal |
| **Track record → trust** | Brier calibration score (P-09) | Calibration-adjusted bond rates (Gap, financial series) | Same history, two consequences |
| **Skin in game for refutation** | Refutation conditions on claim (Gap P-07) | Challenger bond (P-16) | Falsification requires stake |
| **Conditional custody** | Vindication escrow (P-26) | Challenger bond escrow (P-16) | Tokens held against future proof |
| **Correlated judges ≠ independent evidence** | Panel diversity scoring (Gap P-10a) | HHI capture detection (P-10) | Post-hoc detection + pre-hoc prevention |
| **Diversity of method ↔ quality of output** | CrossMethodDiscountBps (P-06) | Revenue split by evidence quality (P-18) | Methodological pluralism → economic routing |
| **Persistent behavior → escalating consequence** | Corroboration accumulation (P-25) | Progressive slash escalation (P-19) | Memory of behavior, compounding |
| **Nature of verifier matters** | Human-only panels (Gap P-13) | Arbiter tier-weighting (P-23) | Observer qualification affects vote weight |

---

## Unified Gap Inventory

All gaps from both series, ranked by priority. Each gap is tagged with layer (E = epistemic, F = financial) and the mission that addresses it if one exists.

### Tier 1 — Structural Deficiencies (Build Now)

| ID | Gap | Layer | Mission | Impact |
|---|---|---|---|---|
| P-16a | No defender bond | F | foundation-defender-bond | ★★★★★ |
| P-18b | No ZRN burn path | F | foundation-supply-burn-path | ★★★★★ |
| P-19a | No tombstoning | F | (no dedicated mission yet) | ★★★★★ |
| P-21a | No automatic circuit breakers | F | foundation-automatic-circuit-breakers | ★★★★★ |
| P-01a | TWAP circular price dependency | F | foundation-oracle-resilience-module | ★★★★★ |
| P-01b | Single knowledge oracle | F | foundation-oracle-resilience-module | ★★★★★ |
| P-07 | No refutation conditions | E | (requires new mission) | ★★★★★ |
| P-10a | No verifier independence pre-check | E | foundation-unified-claim-verification-proto | ★★★★★ |
| P-13 | No human-only panels | E | foundation-unified-claim-verification-proto | ★★★★★ |

### Tier 2 — Quality Improvements (Build Quarters 2-3)

| ID | Gap | Layer | Mission | Impact |
|---|---|---|---|---|
| P-12 | Proof certificates on M-FORMAL | E | (future mission) | ★★★★☆ |
| P-24a | No block hash anchoring on verdicts | F | (future mission) | ★★★★☆ |
| P-25a | Quantity-based corroboration only | E | (future mission) | ★★★★☆ |
| P-03a | No automated contradiction detection | E | (future mission) | ★★★★☆ |
| P-18a | Fee split constants hardcoded | F | (future mission) | ★★★☆☆ |
| P-17a | No commit-reveal for swaps | F | (future mission) | ★★★★★ |
| P-24b | No timeout finality for abandoned rounds | F | (future mission) | ★★★☆☆ |

### Tier 3 — Maturity Features (Build Year 2)

| ID | Gap | Layer | Mission | Impact |
|---|---|---|---|---|
| — | MaxSupplyUzrn not enforced at protocol layer | F | foundation-supply-burn-path | ★★★★☆ |
| — | No liquidity mining | F | (future) | ★★★★★ |
| — | Development fund spending controls | F | (future) | ★★★★☆ |
| — | Null result incentives | E | (future) | ★★★★☆ |
| — | Defeater tracking | E | (future) | ★★★☆☆ |
| — | Multi-round structured challenges | E | (future) | ★★★☆☆ |
| — | Concentrated liquidity | F | (future) | ★★★☆☆ |

---

## Implementation Dependency Graph

The nine accepted missions, with their dependencies and rationale for ordering.

```
────────────── PHASE 1: VOCABULARY ──────────────

[1] Synthesis: Unified Taxonomy (this document)
    └── enables: all phases below (common language)

────────────── PHASE 2: UNDERSTANDING ──────────────

[2] Visualization: Architecture Diagrams
    depends on: [1]
    └── enables: Audit, Roadmap (visual vocabulary)

[3] Audit: Existing Modules vs. Research Recommendations
    depends on: [1]
    output: gap-to-code map, what's real vs. theoretical

[4] Roadmap: Prioritized Plan with Dependency Graph
    depends on: [1], [2], [3]
    output: the ordered plan Yu approves

────────────── PHASE 3: CODE ──────────────

[5] Foundation: Supply Burn Path
    depends on: [4]
    touches: x/knowledge/keeper/fees.go, x/tokens/keeper/
    gaps closed: P-18b (burn path), MaxSupplyUzrn enforcement
    
[6] Foundation: Defender Bond
    depends on: [4]
    touches: x/disputes/tx.proto, x/disputes/keeper/
    gaps closed: P-16a (asymmetric dispute economics)
    
[7] Foundation: Automatic Circuit Breakers
    depends on: [4]
    touches: x/emergency/keeper/, x/liquiditypool/keeper/
    gaps closed: P-21a (EndBlocker AutoHaltConditions)
    
[8] Foundation: Oracle Resilience Module
    depends on: [4] (design); [7] (circuit breaker integration useful but not blocking)
    touches: cmd/oracle/, new oracle committee proto
    gaps closed: P-01a (circular dependency), P-01b (single oracle)
    
[9] Foundation: Unified Claim+Verification Proto Schema
    depends on: [4]
    touches: x/knowledge/tx.proto, x/knowledge/keeper/
    gaps closed: P-07 (refutation conditions), P-10a (panel diversity), P-13 (human-only panels)
    note: largest proto change; should run in parallel with [5]-[7] not after them
```

---

## Vocabulary Reference

Canonical terms used throughout this codebase, their definitions, and primary code locations.

| Term | Definition | Code Location |
|---|---|---|
| **AgentCalibration** | Per-agent track record of Brier scores across all verification rounds | `x/knowledge/keeper/agent_calibration.go` |
| **BPS / Basis Points** | Universal unit: 1,000,000 = 100%. All financial and probability parameters use this scale | Throughout |
| **CanonicalForm** | A claim expressed in the `assert/relate/define/constrain/negate` language | `Claim.canonical_form` in `x/knowledge/tx.proto` |
| **CapturePenalty** | Reward reduction for verifiers who persistently follow crowd consensus | `x/capture_challenge/`, `x/capture_defense/` |
| **ClaimStructure** | Subject/predicate/object/scope decomposition of every Claim | `x/knowledge/tx.proto` |
| **CommitReveal** | Two-phase information hiding: commit hash → reveal value → aggregate | PoT: `x/knowledge/`; AMM: gap |
| **ConfidenceBps** | Graded verifier probability (0–1M) stored in `RevealEntry` | `x/knowledge/tx.proto` field 5 |
| **CorroborationCount** | Cumulative count of survived challenges | `Claim.corroboration_count` |
| **CrossMethodDiscountBps** | Discount applied when multiple methodologies verify the same claim (Feyerabend encoded) | `x/knowledge/types/` |
| **DefenderBond** | (Gap) Bond required of dispute respondent, symmetric to challenger bond | Proposed: `x/disputes/tx.proto` |
| **EmissionPeriod** | Block-linear governance-gated ZRN emission schedule | `x/vesting_rewards/` |
| **EpistemicTemperature** | Domain-level confidence cap: cold domains (high certainty) punish overconfidence | `x/knowledge/keeper/epistemic_temperature.go` |
| **HaltQuorum** | Threshold for Guardian stake required to trigger emergency halt (750_000 BPS = 75%) | `x/emergency/types/` |
| **InferenceType** | DEDUCTIVE/INDUCTIVE/ABDUCTIVE/ANALOGICAL/CONTRADICTION on ClaimRelation | `x/knowledge/tx.proto` |
| **MaxSupplyUzrn** | Maximum native ZRN supply constant (222,222,222 ZRN) — not yet enforced at protocol layer | `x/vesting_rewards/types/constants.go` |
| **NC-IS-OUGHT-WALL** | Schema-level type separation of descriptive vs. normative claims (Hume's law) | `ClaimType` enum |
| **PoT (Proof-of-Truth)** | Zerone's consensus mechanism for claim verification: commit-reveal panel of stake-weighted verifiers | `x/knowledge/keeper/` |
| **ProgressiveSlash** | Slash amount escalates with each prior slash: `amount × (1M + count × EscalationBps) / 1M` | `x/staking/keeper/` |
| **ProofTree** | Derivation graph traversal query: all claims that derive from a given claim | `x/knowledge/` queries |
| **REQUIRES-cascade** | When a fact is disproven, all REQUIRES-dependent facts are marked CONTESTED | `x/knowledge/keeper/vindication.go` |
| **SlashDecayPeriodBlocks** | Number of blocks before slash history is discounted (34,272 ≈ 24 days) | `x/staking/types/` |
| **TWAP** | Block-weighted time-weighted average price; 1e12 scale accumulator | `x/liquiditypool/keeper/` |
| **VerdictBlock** | Block at which a claim's PoT verdict was finalized | `Claim.verdict_block` |
| **VindicationEscrow** | Fund holding slashed tokens pending possible vindication (retroactive justice) | `vindication_escrow` account in `x/knowledge/` |

---

## What Zerone Is

This taxonomy reveals zerone's fundamental character:

**Zerone is a calibrated belief market where the currency is tokens and the commodity is verified truth.**

The financial layer provides the instruments (stake, escrow, fees, AMM). The epistemic layer provides the protocol (commit-reveal, calibration, methodology). The convergence band is where they become the same thing: a claim's `Stake` is simultaneously a submitter's confession of belief-strength and the source of verifier rewards. A verifier's `calibration_score_bps` is simultaneously an epistemic track record and a determinant of future economic weight.

The protocol doesn't just incentivize truth — it *encodes epistemic values as economic structure*. The incentive and the value are one.

Three things distinguish zerone from other knowledge protocols:

1. **Retroactive justice** — `vindication_escrow` means punishment is provisional. If the chain was wrong, it pays back.
2. **Calibrated probability markets** — Brier scoring makes honest confidence reporting the dominant strategy. Overconfidence is penalized, not just uncertainty.
3. **Methodological pluralism with discount** — 12 methodologies, each with its own evidence standard, combined through `CrossMethodDiscountBps`. Feyerabend's pluralism encoded as economics.

These three are what makes zerone different. The tax is not just doing it, it's doing it so that the right behavior is the rational behavior.

---

*Synthesis complete: 21 briefs, 26 primitives across 6 layers, 8 convergence points, unified gap inventory. Generated 2026-04-28 by Beta 🦞.*
