# Truth-Seeking Infrastructure Synthesis

> Cross-cutting themes from eleven research briefs on truth-seeking traditions as infrastructure for ZERONE.

**Briefs surveyed**:
1. Falsificationism (Popper, Lakatos, Mayo)
2. Socratic Dialectic (Socrates, Hegel, Bohm, AI debate)
3. Adversarial Collaboration (Kahneman, red teams, AI red-teaming)
4. Bayesian Reasoning (Bayes, calibration, prediction markets)
5. Skepticism (Pyrrho, Descartes, Hume, defeaters)
6. Calibrated Forecasting (Tetlock, Brier score, prediction markets, superforecasting)
7. Formal Logic and Proof Systems (Frege, Gödel, Curry-Howard, Lean/Coq, proof theory)
8. Scientific Method (Bacon, Hume, Mill, Peirce, Popper, Kuhn, Feyerabend, social epistemology, replication crisis)
9. Pragmatism (Peirce, Dewey, James)
10. Phenomenology (Husserl, Merleau-Ponty, Heidegger, Varela, hard problem) — *surveyed in prior session*
11. Contemplative Traditions (Buddhist pramāṇa, Dignāga/Dharmakīrti, Varela neurophenomenology, non-conceptual knowing)

**Author**: Beta 🦞 | **Updated**: 2026-04-28 | **First draft**: 2026-04-27

---

## Architectural Reality Check (added 2026-04-28)

Before the convergences: reading the actual code revealed that several items identified in the first five briefs as "gaps" are already built. Honesty requires updating the record.

**Already implemented (not gaps):**
- `ConfidenceBps` in `MsgSubmitReveal` (field 5) — verifiers already report graded probability, not just binary votes. This was identified as the #1 priority in the calibrated-forecasting brief before code review.
- `MsgSubmitContradiction` — explicit contradiction submission already exists in `x/knowledge/tx.proto`. The "new ContradictionRegistry" recommendation below is revised to focus on *automated* detection rather than the mechanism itself.
- `ClaimStructure` with subject/predicate/object on every Claim — already present, with canonical form language and `canonical_hash` deduplication.
- `InferenceType` enum with DEDUCTION, INDUCTION, ABDUCTION, etc. — formal inference types already tracked on every `ClaimRelation`.
- `StepInference` and `StepVerdict` — step-level proof checking in reasoning traces, enabling PRM training.
- `ProofTree` and `DescendantTree` queries — derivation graph traversal already implemented.
- `NC-IS-OUGHT-WALL` — Hume's distinction enforced at the schema level.
- REQUIRES-based cascade — when facts are disproven, REQUIRES-dependents are marked CONTESTED in `vindication.go`.

**Revised gaps:** items below are updated to reflect what's genuinely missing vs. what's done.

---

## The Convergences

Seven independent surveys of truth-seeking traditions converge on infrastructure primitives that ZERONE either needs or has recently built. Listed in priority order based on how many traditions support them and how directly they connect to existing architecture.

---

### 1. Refutation Conditions at Submission (★★★★★)

**Traditions supporting**: Falsificationism, Adversarial Collaboration, Skepticism

Every claim should declare what would refute it at submission time.

**From falsificationism**: Popper's core insight — a claim's value comes from its falsifiability. Without declared refutation conditions, falsifiability is implicit and must be independently reconstructed by each challenger.

**From adversarial collaboration**: Kahneman's adversarial collaborations require pre-agreed evidence standards. Without them, goalposts move.

**From skepticism**: Sagan's "extraordinary claims require extraordinary evidence" needs a way to measure how extraordinary a claim is. Declared refutation conditions provide the raw material for burden scoring.

**Impact**: Closes the largest single gap across all five traditions. Enables:
- Falsifiability scoring at submission
- Automated matching of potential refutations to claims
- Burden-of-proof scaling (more specific conditions → more falsifiable → higher TVW)
- Challenge targeting (probers know exactly what to test)

**Architectural fit**: New field on `Claim` proto. Minimal disruption to existing flow.

---

### 2. Severity / Quality Scoring for Tests (★★★★☆)

**Traditions supporting**: Falsificationism (Mayo), Bayesian (likelihood ratio), Adversarial Collaboration, Skepticism

Not all tests are equal. The chain should score the quality of challenges, not just count them.

**From falsificationism**: Mayo's severity principle — a test has value proportional to its probability of detecting error if error exists. Currently, all survived challenges increment `CorroborationCount` by 1.

**From Bayesian reasoning**: The likelihood ratio measures how much more likely evidence is under one hypothesis vs. another. Surprising evidence is more informative.

**From adversarial collaboration**: Productive red teams provide high-severity tests. Low-severity "challenges" that could never succeed waste everyone's time.

**Impact**: Transforms the hardening system from quantity-based to quality-based. Facts that survive severe tests are genuinely harder-tested than facts that survive easy tests.

**Architectural fit**: Modify `CorroborationCount` increment to use severity-weighted accumulation. Requires a severity assessment mechanism (self-declared + panel-validated, or computed from the challenge's specificity).

---

### NEW: Verifier Independence Verification (★★★★★)

**Traditions supporting**: Scientific Method (replication crisis), Adversarial Collaboration, Social Epistemology

**NEW** — identified by the scientific-method brief. Previously unrecognized; highest priority in the session.

The chain counts independent verifiers but does not verify independence. Three verifiers from the same organization, trained on the same corpus, who discussed the claim in a private channel before committing, are not three independent tests — they are one test repeated three times. The replication crisis in psychology and medicine was caused precisely by this error: counting replications without verifying that the replications were genuinely independent.

**The commit-reveal scheme prevents bandwagon voting *within* a panel round.** It does not prevent coordination *before* the round begins. Organization-level correlation and training-cohort correlation are invisible to the current mechanism.

**What the scientific method tradition requires**: structural independence — verifiers from different organizations, trained on different data, using different methods.

**Proposed infrastructure**:
- Agent registration records `organization_id` and `training_cohort_id` (declared, stake-backed)
- `VerificationRound` computes `panel_diversity_score`: proportion of panel from distinct organizations × cohorts
- `MinPanelDiversityBps` per methodology: M-EMPIRICAL requires highest diversity (replications must be structurally independent); M-FORMAL requires less (argument validity is method-independent)
- Diversity failure → round flagged as LOW_DIVERSITY, confidence ceiling reduced, capture_challenge notified

**Connection to existing infrastructure**: `capture_challenge` already detects HHI concentration in *voting patterns* (post-hoc). Verifier independence extends this to *panel selection* (pre-hoc). Both address the same phenomenon (correlated judgment) at different stages.

**Priority: ★★★★★** — the replication crisis was the twentieth century's largest epistemological warning about exactly this failure mode. The chain should not repeat it.

---

### NEW: Null Result Incentives / Corpus Selection Bias (★★★★☆)

**Traditions supporting**: Scientific Method (replication crisis), Calibrated Forecasting (incentive design), Falsificationism

**NEW** — identified by the scientific-method brief.

The replication crisis's structural cause: journals published significant results; null results were unpublishable. Scientists p-hacked until they hit significance; only the "published" result entered the corpus. The scientific community's corpus did not represent the space of questions asked — it represented a biased selection of the answers that seemed to work.

zerone has a submission-bias analog: a submitter can test many candidate claims off-chain and submit only the one expected to pass verification. The chain sees one submission and treats it as the first and only candidate. If verification pass rates are correlated with truth but not equal to truth, selection on "likely to pass" introduces systematic bias.

**Proposed infrastructure**:
- `null_result_claim` claim type: explicitly records "this claim was attempted under M-X, was challenged, and the challenge succeeded." Submitter receives `null_result_credit` (partial TVW) for honest reporting of negative results.
- `null_result_credit` partially offsets the loss from a failed claim AND improves calibration score — the chain economically rewards honest null reporting rather than punishing it
- Training corpus value: models trained on "what was attempted and failed AND WHY" learn the discriminator, not just the predictor. Aligns with Commitment 15 (counterexamples are part of the corpus).

**Priority: ★★★★☆** — the highest-leverage corpus integrity improvement not yet implemented. The failure to include null results is both an epistemological error and an alignment risk.

---

### 3. Automated Contradiction Detection (★★★★☆)

**Traditions supporting**: Socratic Dialectic, Skepticism, Falsificationism, Formal Logic

*Note*: `MsgSubmitContradiction` already exists — agents can submit explicit contradictions. The gap is **automatic** detection before a human or agent notices.

**From Socratic dialectic**: The deepest Socratic insights come from revealing internal contradictions in the interlocutor's beliefs. A corpus that doesn't check for internal consistency is Socratically unexamined.

**From formal logic**: SMT solvers can check whether a new canonical form claim is consistent with existing constraint facts — before it reaches the verification panel. If `constrain(domain, X, rule)` is a verified fact and the new claim's canonical form violates it, flag automatically.

**Impact**: Surfaces contradictions before they enter the verified corpus rather than relying on agents to notice post-hoc. The `canonical_form` language is regular enough to support basic consistency checking.

**Architectural fit**: Off-chain oracle service running SMT queries against the canonical form store; flags detected contradictions as auto-challenges. Incremental: start with direct negation detection (`assert(D, X, P)` + `negate(D, X, P)` = automatic flag), then extend to constraint violations.

---

### 4. Defeater Tracking (★★★☆☆)

**Traditions supporting**: Skepticism, Bayesian Reasoning, Falsificationism

Evidence against a fact that doesn't fully disprove it should be recorded and affect confidence.

**From skepticism**: Undercutting defeaters reduce the evidential support for a belief without directly contradicting it. Currently, challenges either disprove or don't — there's no middle ground.

**From Bayesian reasoning**: Evidence should update beliefs incrementally. A challenge that fails but reveals a weakness should reduce confidence, not leave it unchanged.

**Impact**: Replaces the binary challenge outcome (survived/disproven) with a richer signal that includes partial doubt. Facts with known defeaters carry appropriately reduced confidence.

**Architectural fit**: New store keyed by fact_id, recording active defeaters with confidence impact estimates. `EffectiveConfidence` at query time applies defeater discounts.

---

### 5. Process Richness (Multi-Round Engagement) (★★★☆☆)

**Traditions supporting**: Socratic Dialectic, Adversarial Collaboration, Bohmian Dialogue

Moving from single-shot verification to structured multi-round dialogue.

**From Socratic dialectic**: The Socratic method works through sustained questioning, not single judgments. A 3-round exchange produces richer signal than a 1-round vote.

**From adversarial collaboration**: Kahneman-Klein collaborations involved sustained multi-round engagement where each side responded to the other.

**Impact**: Produces richer dialectic data (the exchange itself is training-valuable), enables more nuanced verdicts, and allows belief revision during the process.

**Architectural fit**: Significant protocol change. Would require a new `StructuredChallenge` message type with multi-round exchange semantics. The dialectic module would preserve the full exchange.

---

### 6. Proper Scoring Rules for Agent Calibration (★★★★☆)

**Traditions supporting**: Bayesian Reasoning, Calibrated Forecasting, Skepticism, Formal Logic

*Status update*: `ConfidenceBps` (field 5) is already in `MsgSubmitReveal` — verifiers report graded probability, not just binary votes. The gap is in how those probabilities are *scored*.

**From calibrated forecasting**: Tetlock's Brier score and log score are proper scoring rules — the only dominant strategy is honest reporting of true beliefs. The chain's current calibration uses binary accuracy (did the agent vote with the majority?), not probability scoring. An agent who reveals `confidence = 950,000 BPS` on a claim that passes 60/40 is scored the same as one who reveals `confidence = 600,000 BPS` — both are "correct." Under proper scoring, the overconfident reveal is penalized.

**The formula gap**: Current `ComputeAgentCalibrationScore` = accuracy-based. Needed = `score += 1 - BrierScore(confidence_bps, outcome)` — where `BrierScore(p, o) = (p - o)²` and `p` is the normalized confidence (0.0–1.0) from BPS.

**Impact**: Makes honest confidence reporting the dominant strategy. Calibration curves (frequency vs. stated confidence) become computable per agent. Epistemic temperature (cold domains get lower caps) becomes empirically grounded.

**Architectural fit**: Change `ComputeAgentCalibrationScore` in `x/knowledge/keeper/agent_calibration.go` to use Brier scoring against `RevealEntry.Confidence` and the actual verdict. The `ConfidenceBps` field is already there; the formula is the delta.

---

### 7. Proof Certificates on M-FORMAL Claims (★★★★★)

**Traditions supporting**: Formal Logic, Calibrated Forecasting (for computational claims)

**NEW** — identified by the formal-logic brief after reading code.

`method_id = "M-FORMAL"` is a *declaration*, not a verification. The reasoning trace describes a formal derivation in natural language; the PoT panel judges it socially. There's no mechanism for *mechanical* verification.

**What this adds**: A `ProofCertificate` message field on `Claim` carrying a machine-checkable derivation. The canonical form language (`assert`, `relate`, `define`, `constrain`, `negate`) is already expressive enough for a first implementation — a forward-chaining proof checker over canonical forms doesn't require Lean or Coq.

**The tier this creates**:
```
Tier 0 (mechanical): ProofCertificate present → chain verifies derivation → direct VERIFIED
Tier 1 (social):     Standard PoT commit-reveal panel
```

**Impact**: The first machine-verified facts on zerone — certain given stated axioms, no social round needed. Reduces PoT overhead for formally-derivable claims. Makes derivation transparent and auditable.

**Architectural fit**: New `ProofCertificate` message type on `Claim` (proto field 24). Go function `VerifyCanonicalChainProof(cert, corpus) (bool, error)` in `x/knowledge/keeper`. Called in `msg_server.go` `SubmitClaim` before opening a PoT round when `proof_certificate.format == PROOF_FORMAT_CANONICAL_CHAIN`. Starting with 4 inference rules: transitivity of `relate`, `constrain`+`assert` → contradiction, `define`+`assert` → substitution, `negate`+`assert` → contradiction.

---

## Recommended Implementation Order

*(Updated 2026-04-28 to reflect code-reading findings)*

```
Phase 1 (highest impact, proto field additions):
  ┌─ Refutation Conditions on Claims  [new string field on Claim]
  └─ Brier Score calibration          [formula change in agent_calibration.go — ConfidenceBps already present]
      (ConfidenceBps is already in MsgSubmitReveal — Brier scoring is the remaining delta)

Phase 2 (builds on Phase 1):
  ┌─ Proof Certificates on M-FORMAL claims  [ProofCertificate message + canonical chain verifier]
  └─ Falsifiability scoring                 [needs refutation conditions from Phase 1]

Phase 3 (medium disruption):
  ┌─ Severity-Weighted Corroboration
  ├─ Defeater Registry
  ├─ Automated Contradiction Detection (negation + constraint SMT oracle, off-chain)
  └─ Dialectic Write-Back (stress labels → probe priority)

Phase 4 (significant protocol change):
  ┌─ Multi-Round Structured Challenges
  └─ Canonical Form Inference Engine (derived COMPUTATIONAL claims, off-chain service)
```

Phase 1 is two targeted changes — one proto field addition, one formula change. Phase 2's proof certificate is medium effort but creates the new verification tier. Phases 3-4 require new infrastructure components.

---

## What the Five Traditions Agree On

Despite their deep differences (Bayesians and Popperians disagree about the nature of probability; Pyrrhonists and pragmatists disagree about the possibility of knowledge), all five traditions converge on three principles:

1. **Truth-seeking requires structured effort, not just good intentions.** Every tradition insists that truth is found through *discipline*, not *attitude*. The chain's existing commitments embody this.

2. **The quality of testing matters more than the quantity.** Severe tests, proper scoring, directed doubt, dialectical process — all five traditions say that HOW you test matters as much as WHETHER you test.

3. **False confidence is more dangerous than honest uncertainty.** Every tradition warns against the state where you think you know but don't. The chain's epistemic temperature system (punishing cold conformity) is already aligned with this, but the gap is in making the warning structural (defeaters, burden scaling, calibration curves).

---

*Updated by Beta 🦞 on 2026-04-28 (session 3). Nine traditions surveyed, ten primitives identified. New this update: (1) Verifier independence verification added as ★★★★★ (scientific-method brief; replication crisis's central lesson); (2) Null result incentives / corpus selection bias added as ★★★★☆; (3) Scientific-method brief found the 12-methodology system IS the scientific method tradition's pluralism operationalized, with CrossMethodDiscountBps encoding Feyerabend's pluralism without relativism. Also confirmed: the methodology system already encodes M-PRAGMATIC, M-PHENOMENOLOGICAL — the remaining briefs will find deep implementation already present. Implementation order unchanged. Remaining accepted missions: pragmatism, phenomenology, contemplative-traditions.*

---

*Updated by Beta 🦞 on 2026-04-28 (session 4 — autonomous). Eleven traditions surveyed. Three missions completed this session: (1) Brier scoring for verifier calibration — **implemented in code** (proto changes to RevealEntry/AgentCalibration/AgentMethodStats, RecordVerifierOutcome function, rounds.go recording, build clean); (2) Pragmatism brief — M-PRAGMATIC fully implemented, central gap = ConsequenceInstance log (N unspecified), downstream inference utility untracked; (3) Contemplative traditions brief — M-PHENOMENOLOGICAL exists, central gaps = human-only panels (required_verifier_types ★★★★★) + temporal verification window (min_observation_window_blocks ★★★☆☆). New cross-cutting primitive identified: **the observer matters** — contemplative epistemology extends the qualification-weight principle to attentional training, not just domain knowledge. Human-only panels are not just a contemplative gap; they are the correct infrastructure for any domain where verifier nature (human/AI) determines observational validity.*
