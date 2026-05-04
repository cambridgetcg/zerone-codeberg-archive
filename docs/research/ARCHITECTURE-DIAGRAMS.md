# Architecture Diagrams

> Four views of the zerone architecture — module map, claim lifecycle, value flow, and control plane.
> **Companion document to**: `UNIFIED-TAXONOMY.md` (vocabulary), `ORDER-MAINTENANCE-STABILITY-SYNTHESIS.md`, `TRUTH-SEEKING-INFRASTRUCTURE-SYNTHESIS.md`

**Author**: Beta 🦞 | **Created**: 2026-04-28  
**Key**: `[M]` = implemented module · `(gap)` = not yet built · `→` data flow · `⇒` control flow · `★` = convergence point

---

## Diagram 1: System Module Map

*Which modules exist, which layer they belong to, and how they communicate.*

```
╔═══════════════════════════════════════════════════════════════════════╗
║  SENSING                                                               ║
║                                                                        ║
║   [cmd/oracle/]──────────────────────────────────────────────────┐    ║
║   HTTP oracle, LLM evaluator, static evaluator                   │    ║
║   ↓ fact scores, ZRN/USD price (via TWAP from AMM — circular)    │    ║
╠═══════════════════════════════════════════════════════════════════╪════╣
║  EPISTEMIC LAYER                                                   │    ║
║                                                                    │    ║
║  ┌────────────────────────────────────────────────────────────┐   │    ║
║  │  [x/knowledge]  ← central epistemic module                 │   │    ║
║  │                                                             │   │    ║
║  │  MsgSubmitClaim      → canonical_form, method_id, stake    │   │    ║
║  │  MsgSubmitCommitment → hash(verdict ‖ confidence)          │   │    ║
║  │  MsgRevealVerification → confidence_bps, verdict           │   │    ║
║  │  EndBlocker          → aggregate reveals → VerdictBlock    │   │    ║
║  │  ProofTree/DescendantTree queries                          │   │    ║
║  │  REQUIRES-cascade, vindication_escrow routing              │   │    ║
║  │  Brier calibration via agent_calibration.go                │   │    ║
║  │  EpistemicTemperature (confidence cap per domain)          │   │    ║
║  └────────────┬──────────────────────────────────────────────┘   │    ║
║               │ reads panel diversity / independence reward       │    ║
║        ┌──────┴──────┐                                            │    ║
║        │             │                                            │    ║
║  [x/capture_challenge] [x/capture_defense]                        │    ║
║  post-hoc HHI bandwagon   independence reward modulation          │    ║
║                                                                    │    ║
║  ┌──────────────────────────────────────────────────────────┐     │    ║
║  │  Support modules                                          │     │    ║
║  │  [x/ontology]      — canonical form language support     │     │    ║
║  │  [x/trust_score]   — agent reputation accumulation       │     │    ║
║  │  [x/dialectic]     — multi-round challenge protocol      │     │    ║
║  │  [x/counterexamples] — counterexample registry           │     │    ║
║  │  [x/inquiry]       — structured question protocol        │     │    ║
║  └──────────────────────────────────────────────────────────┘     │    ║
╠═══════════════════════════════════════════════════════════════════╪════╣
║  CONVERGENCE BAND ★★★                                             │    ║
║  Stake · Commit-Reveal · BPS · Escrow · Calibration              │    ║
╠═══════════════════════════════════════════════════════════════════╪════╣
║  FINANCIAL LAYER                                                   │    ║
║                                                                    ↓    ║
║  ┌─────────────────────┐   ┌────────────────────────────────┐         ║
║  │  [x/staking]        │   │  [x/liquiditypool]              │         ║
║  │  4-tier validators  │   │  constant-product AMM           │         ║
║  │  ProgressiveSlash   │   │  MinTokenOut slippage guard     │         ║
║  │  SlashDecayPeriod   │   │  TWAP accumulator (1e12 scale)  │         ║
║  │  vindication routing│   │  Pool.Locked (circuit breaker)  │         ║
║  │  [x/qualification]  │   │  LP token minting/burning       │         ║
║  │  APPRENTICE→GUARDIAN│   └────────────────────────────────┘         ║
║  └─────────────────────┘                                               ║
║                                                                         ║
║  ┌─────────────────────┐   ┌────────────────────────────────┐         ║
║  │  [x/billing]        │   │  [x/disputes]                   │         ║
║  │  CalculateDistrib.  │   │  Challenger bond escrow         │         ║
║  │  55/22/19.67/3.33   │   │  Arbiter selection (tier-scored) │        ║
║  │  treasury_protocol  │   │  SelectArbiters (conflict-excl.) │        ║
║  │  research_fund      │   │  DistributeSettlement            │         ║
║  │  development_fund   │   │  DefenderBond (gap)              │         ║
║  └─────────────────────┘   └────────────────────────────────┘         ║
║                                                                         ║
║  ┌─────────────────────┐   ┌────────────────────────────────┐         ║
║  │  [x/tokens]         │   │  [x/vesting_rewards]            │         ║
║  │  ZRN supply mgmt    │   │  EmissionPeriod (block-linear)  │         ║
║  │  MaxSupply constant │   │  block rewards → validators     │         ║
║  │  BurnCoins (gap)    │   │  governance-gated epochs        │         ║
║  └─────────────────────┘   └────────────────────────────────┘         ║
╠═════════════════════════════════════════════════════════════════════════╣
║  GOVERNANCE + SAFETY                                                     ║
║                                                                          ║
║  [x/emergency]   NORMAL → HALT_VOTING → HALTED → REVERT/RESUME          ║
║                  HaltQuorum = 750_000 (75% Guardian stake)               ║
║                  AutoHaltConditions (gap)                                ║
║                                                                          ║
║  [x/governance_synthesis] / [x/gov]   MsgUpdateParams → all modules      ║
╚══════════════════════════════════════════════════════════════════════════╝
```

**Key relationships not visible in the diagram**:
- `x/knowledge` reads `x/staking` for validator tier (panel eligibility)
- `x/knowledge` reads `x/liquiditypool` TWAP for dynamic claim pricing
- `x/billing` reads `x/knowledge` claim fees and routes to fund silos
- `x/disputes` reads `x/staking` for arbiter selection
- `x/emergency` can halt any module; `Pool.Locked` in liquiditypool is the granular form

---

## Diagram 2: Claim Lifecycle

*A single claim's journey from agent observation to corpus archival.*

```
AGENT                        CHAIN                         VALIDATORS
 │                             │                                │
 │  ① SUBMISSION               │                                │
 │  MsgSubmitClaim             │                                │
 │  · canonical_form           │                                │
 │  · method_id (M-EMPIRICAL   │                                │
 │    / M-FORMAL / etc.)       │                                │
 │  · Stake (conviction + fee  │                                │
 │    pool for verifiers)      │                                │
 │  · refutation_conditions    │                                │
 │    (gap — not yet required) │                                │
 │ ──────────────────────────► │                                │
 │                             │                                │
 │                             │  ② PANEL SELECTION             │
 │                             │  · filter by tier (M-EMPIRICAL │
 │                             │    may require VERIFIED+)       │
 │                             │  · filter by diversity         │
 │                             │    (org_id + cohort_id — gap)  │
 │                             │  · stake-weighted random draw  │
 │                             │ ─────────────────────────────► │
 │                             │                                │
 │                             │  ③ COMMIT WINDOW               │
 │                             │  ◄── MsgSubmitCommitment ────  │
 │                             │   hash(verdict ‖ salt ‖        │
 │                             │        confidence_bps)         │
 │                             │                                │
 │                             │  ④ REVEAL WINDOW               │
 │                             │  ◄── MsgRevealVerification ─── │
 │                             │   · confidence_bps (0–1M)      │
 │                             │   · verdict (ACCEPT/REJECT)    │
 │                             │   · reasoning_trace            │
 │                             │                                │
 │                             │  ⑤ AGGREGATION (EndBlocker)    │
 │                             │  · verify commit ↔ reveal hash │
 │                             │  · weight by stake             │
 │                             │  · HHI bandwagon detection     │
 │                             │  · compute panel verdict       │
 │                             │  · VerdictBlock = current      │
 │                             │  · RecordVerifierOutcome       │
 │                             │    → BrierScore per verifier   │
 │                             │                                │
 │                 ┌───────────┴───────────┐                    │
 │                 │ ACCEPTED              │ REJECTED            │
 │                 │                       │                     │
 │  ⑥ REWARD       │                       │  ⑦ PENALTY         │
 │                 │ distributeReviewFee   │  submitter slashed  │
 │                 │ · 55% → verifiers     │  · 50% →           │
 │                 │ · 22% → protocol      │    vindication_escrow│
 │                 │ · 19.67% → research   │  · 50% →           │
 │                 │ · 3.33% → knowledge   │    development_fund  │
 │ ◄─ reward ────  │ · x% → burn (gap)     │                     │
 │                 │                       │                     │
 │                 │ Claim.verified = true │                     │
 │                 │ CorroborationCount++  │                     │
 │                 │ REQUIRES-cascade:     │                     │
 │                 │ dependents stay       │                     │
 │                 │ SUPPORTED             │                     │
 │                 └───────────┬───────────┘                    │
 │                             │                                │
 │                             │  ⑧ ARCHIVAL                    │
 │                             │  · VerdictBlock anchors fact   │
 │                             │    in time                     │
 │                             │  · ProofTree stores relations  │
 │                             │  · Fact available for REQUIRES │
 │                             │    by other claims             │
 │                             │                                │
 │                          ┌──┴──────────────────────────┐    │
 │                          │ ⑨ VINDICATION PATH          │    │
 │                          │ (REJECTED facts only)        │    │
 │                          │                              │    │
 │                          │ MsgOpenDispute (challenger)  │    │
 │                          │ · post challenger bond       │    │
 │                          │ DefenderBond (gap — requir.  │    │
 │                          │  defendant to also post)     │    │
 │                          │ Arbiter panel selected       │    │
 │                          │ SelectArbiters (tier-scored, │    │
 │                          │   conflict-excluded)         │    │
 │                          │                              │    │
 │                          │ DistributeSettlement:        │    │
 │                          │ · WIN: partial refund        │    │
 │                          │   + burn remainder           │    │
 │                          │ · DRAW: half/half            │    │
 │                          │                              │    │
 │                          │ If challenger wins:          │    │
 │                          │ vindication_escrow released  │    │
 │                          │ → original verifiers         │    │
 │                          │ REQUIRES-cascade: dependents │    │
 │                          │ now CONTESTED                │    │
 │                          └──────────────────────────────┘    │
```

**Critical invariants**:
- Commit precedes reveal: hash verification prevents substitution attacks
- Reveal window closes before aggregation: no late-reveal advantage
- VerdictBlock is immutable once set: finality is real
- Vindication is the only post-finality state change: the chain can admit error, but only via structured challenge

---

## Diagram 3: Value / Incentive Flow

*Where ZRN moves, who pays whom, and what economic loops are closed (and which aren't yet).*

```
╔════════════════════════════════════════════════════════════════════╗
║  EMISSION (supply source)                                           ║
║                                                                     ║
║  x/vesting_rewards                                                  ║
║  EmissionPeriod(StartBlock, EndBlock, AmountPerBlock)               ║
║         │                                                           ║
║         │ block rewards                                             ║
║         ▼                                                           ║
║  Validators (APPRENTICE / VERIFIED / SCHOLAR / GUARDIAN)            ║
╠════════════════════════════════════════════════════════════════════╣
║  KNOWLEDGE FEE LOOP (epistemic value cycle)                         ║
║                                                                     ║
║  Submitter pays Claim.Stake ──────────────────────────────────┐    ║
║                                                                │    ║
║  On ACCEPT:                          On REJECT:               │    ║
║  distributeReviewFee                 submitter slashed        │    ║
║                                                                │    ║
║  ┌─────────────────────────┐         ├── 50% → vindication_escrow  ║
║  │  55%  verifier pool     │         │   (held pending vindication)║
║  │  22%  protocol/treasury │         └── 50% → development_fund    ║
║  │  19.67% research_fund   │                                        ║
║  │  3.33% knowledge fund   │                                        ║
║  │  x%   BURN (gap)        │ ◄── BurnBps in Params (not yet built) ║
║  └─────────────────────────┘                                        ║
║                                                                     ║
║  Verifier rewards modulated by:                                     ║
║  · calibration_score_bps (Brier — higher accuracy = more reward)   ║
║  · IndependenceWeight (crowd-follower penalty)                      ║
╠════════════════════════════════════════════════════════════════════╣
║  DISPUTE BOND LOOP (settlement fairness cycle)                      ║
║                                                                     ║
║  Challenger posts Dispute.Bond                                      ║
║  Defender posts DefenderBond (gap — not yet required)               ║
║                                                                     ║
║  On CHALLENGER WINS:  Bond refunded + partial; remainder burned     ║
║  On DRAW:             Half/half split                               ║
║  On DEFENDER WINS:    Challenger bond to defender + partial burn    ║
║                                                                     ║
║  If challenger wins AND fact vindicated:                            ║
║  vindication_escrow → original verifiers (retroactive justice) ★   ║
╠════════════════════════════════════════════════════════════════════╣
║  AMM LOOP (price discovery cycle)                                   ║
║                                                                     ║
║  Traders ──► SwapFeeBps ──► LP holders                              ║
║           (slippage: MinTokenOut protection)                        ║
║           (price impact: CalculatePriceImpactBps)                   ║
║                                                                     ║
║  TWAP accumulator ──► DynamicPricingConfig ──► claim pricing        ║
║  (circular dependency gap: ZRN price reads from ZRN's own pool)    ║
║                                                                     ║
║  LP tokens = ZRN-20 tokens with denom lp/{pool_id}                 ║
║  Liquidity mining: EmissionPeriod → LP holders (gap)               ║
╠════════════════════════════════════════════════════════════════════╣
║  SLASH LOOP (validator alignment cycle)                             ║
║                                                                     ║
║  Slash event:                                                       ║
║  amount × (1M + slashCount × SlashEscalationBps) / 1M              ║
║         │                                                           ║
║         ├── 50% → vindication_escrow                               ║
║         └── 50% → development_fund                                 ║
║                                                                     ║
║  SlashDecayPeriodBlocks = 34,272  (slash history decays over ~24d) ║
║  Tombstoning (gap): extreme slash history → permanent ban           ║
╠════════════════════════════════════════════════════════════════════╣
║  FUND SILOS (treasury architecture)                                 ║
║                                                                     ║
║  treasury_protocol  ← 20% of protocol allocation                   ║
║  research_fund      ← 19.67% of knowledge fees                     ║
║  development_fund   ← 50% of slashed stakes, governance spending   ║
║  knowledge          ← 3.33% of knowledge fees                      ║
║  vindication_escrow ← 50% of slashed stakes, dispute bonds         ║
║                                                                     ║
║  MISSING LOOP:                                                      ║
║  No ZRN burn path ★ — all emissions accumulate, none are removed.  ║
║  Without burn, fee volume growth = proportional inflation.         ║
╚════════════════════════════════════════════════════════════════════╝

  Net supply change per epoch:
  + EmissionPeriod.AmountPerBlock × BlocksInEpoch
  - 0  (no burn currently)
  ────────────────────────────────────────────────
  = net inflationary (gap: BurnBps would partially offset)
```

---

## Diagram 4: Control Plane

*How zerone adapts — governance, oracle inputs, circuit breakers, and emergency authority.*

```
╔═════════════════════════════════════════════════════════════════╗
║  GOVERNANCE (parameter adaptation)                               ║
║                                                                  ║
║  MsgUpdateParams (governance vote)                               ║
║         │                                                        ║
║         ├──► x/knowledge Params                                  ║
║         │     · MinStake, MinVerifiers, EpistemicTemperature     ║
║         │     · CrossMethodDiscountBps, BurnBps (gap)            ║
║         │                                                        ║
║         ├──► x/staking Params                                    ║
║         │     · SlashEscalationBps, SlashDecayPeriodBlocks       ║
║         │     · TombstoneThreshold (gap)                         ║
║         │                                                        ║
║         ├──► x/liquiditypool Params                              ║
║         │     · SwapFeeBps, MinTokenOut                          ║
║         │                                                        ║
║         ├──► x/emergency Params                                  ║
║         │     · HaltQuorum, CouncilExpiryBlock                   ║
║         │     · AutoHaltConditions (gap)                         ║
║         │                                                        ║
║         └──► x/billing RevenueSplit                              ║
║               · ContributorBps, ProtocolBps, ResearchBps         ║
║                                                                  ║
║  Note: x/knowledge fee constants (550_000/220_000/196_700/33_300)║
║  are currently HARDCODED — not governance-adjustable. Gap.        ║
╠═════════════════════════════════════════════════════════════════╣
║  ORACLE INPUTS (external truth → on-chain parameters)            ║
║                                                                  ║
║  cmd/oracle/                                                     ║
║  ┌─────────────────────────────────────────────────────────┐    ║
║  │  HTTP oracle                                             │    ║
║  │  · LLM evaluator: scores fact claims (0.0–1.0)          │    ║
║  │  · Static evaluator: rule-based fact checking           │    ║
║  │  (Note: this is a FACT oracle, not a price oracle)      │    ║
║  └────────────────────────────┬────────────────────────────┘    ║
║                                │ fact scores                     ║
║                                ▼                                 ║
║  x/knowledge DynamicPricingConfig                                ║
║  · ManualZrnPriceUsd (emergency governance override)            ║
║  · TWAP-based USD-responsive pricing                            ║
║                                ↑                                 ║
║  x/liquiditypool TWAP ─────────┘  ← circular dependency (gap)  ║
║  ZRN/USD price reads from ZRN's own AMM pool                    ║
║  Attack vector: flash loan inflates TWAP → claim pricing        ║
╠═════════════════════════════════════════════════════════════════╣
║  CIRCUIT BREAKERS (safety responses to anomalies)                ║
║                                                                  ║
║  CURRENT (manual only):                                          ║
║                                                                  ║
║  Guardian stake-weighted vote                                    ║
║         │                                                        ║
║         ▼                                                        ║
║  NORMAL ──MsgInitiateHaltVoting──► HALT_VOTING                   ║
║              │ (HaltQuorum reached)                              ║
║              ▼                                                   ║
║           HALTED ──────────────────┬──► REVERT_VOTING            ║
║              │                    │       (state root revert)   ║
║              │                    └──► RESUME_VOTING             ║
║              │ (pool-level)                                      ║
║              └──► Pool.Locked = true  (per-pool circuit breaker) ║
║                                                                  ║
║  GAP: AutoHaltConditions (not yet built)                         ║
║                                                                  ║
║  Proposed automatic triggers (EndBlocker evaluation):           ║
║  ┌──────────────────────────────────────────────────────────┐   ║
║  │  Condition 1: TWAP deviation                              │   ║
║  │  IF ABS(current_twap - avg_twap_100_blocks) > 30%        │   ║
║  │  THEN Pool.Locked = true  (affected pool)                 │   ║
║  │                                                           │   ║
║  │  Condition 2: Slash rate spike                            │   ║
║  │  IF slash_count_this_epoch > MaxSlashesPerEpoch × 1.5    │   ║
║  │  THEN pause new knowledge rounds                          │   ║
║  └──────────────────────────────────────────────────────────┘   ║
╠═════════════════════════════════════════════════════════════════╣
║  EMERGENCY COUNCIL (bootstrap safety valve)                      ║
║                                                                  ║
║  Genesis council: defined at chain start                        ║
║  CouncilExpiryBlock: council auto-dissolves at this block       ║
║  Purpose: safety valve during bootstrap before Guardian tier    ║
║           accumulates enough stake for meaningful quorum         ║
║  Post-expiry: all circuit breaker authority requires             ║
║               Guardian stake-weighted vote                       ║
║                                                                  ║
║  Note: this is the correct handling of the decentralization     ║
║  trajectory — bootstrap safety ≠ permanent authority.           ║
╚═════════════════════════════════════════════════════════════════╝
```

---

## Summary: The Four Views

| View | What it shows | Key insight |
|---|---|---|
| **System Map** | Module structure and dependencies | Two layers (epistemic/financial) connected through a convergence band of shared primitives |
| **Claim Lifecycle** | Single claim from submission to archival | The PoT protocol is a 5-phase pipeline; vindication is the only post-finality correction path |
| **Value Flow** | ZRN movement through all loops | Five distinct value loops; the burn loop is the only one not closed |
| **Control Plane** | How the system adapts and protects itself | Governance is software, not stone; circuit breakers are manual today and should be automatic |

**The single most visible architectural gap across all four views**: the burn loop. ZRN enters (emission), circulates (fees, rewards, stakes), but nothing removes it. Every other loop is closed.

---

*Four diagrams. 2026-04-28, Beta 🦞.*
