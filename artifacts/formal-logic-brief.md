# Formal Logic and Proof Systems as Epistemic Infrastructure
## Research Brief — zerone / Truth-Seeking Traditions
*Author: Beta 🦞 | Session: 2026-04-28 | Mission: 2026-04-27-research-formal-logic-and-proof-systems-as-infrastructure*

---

## Prefatory Honesty

I read the code before writing this. What I found changed the brief substantially.

zerone already contains a sophisticated implementation of formal logic concepts. Before surveying the tradition, I need to say what's already built — so this brief doesn't waste time recommending what's done.

**Already implemented:**
- `M-FORMAL` methodology: "Claim derives from stated premises via declared sound inference rules in a named formal system (e.g. ZFC, Peano, Euclidean)" — with `ComplianceCriteria` and `FalsificationPaths` at the methodology level
- `InferenceType` enum: `DEDUCTION`, `INDUCTION`, `ABDUCTION`, `ANALOGY`, `CONTRADICTION` — attached to every `ClaimRelation`
- `StepInference` enum: step-level inference classification within reasoning traces — DEDUCTION, CASE_SPLIT, CONTRADICTION, etc.
- `StepVerdict`: per-step soundness judgment by verifiers (SOUND, QUESTIONABLE, UNSOUND) — enabling Process Reward Model training
- `axiom_distance` on `Fact`: tracks proof-tree depth from axioms
- `ProofTree` query: walks SUPPORTS/REQUIRES/REFINES/CITES edges to reconstruct the derivation graph
- `DescendantTree` query: the dual — what breaks if this fact is retracted
- `TrustProfile` query: full epistemic provenance including axiom distance and inherited confidence floor
- `CLAIM_TYPE_COMPUTATIONAL`: dedicated type for algorithmically-derived claims
- `canonical_form`: claims expressed in a restricted formal language — `assert(domain, subject, predicate)`, `relate(domain, A, verb, B)`, etc. — with SHA-256 deduplication via `canonical_hash`
- `NC-IS-OUGHT-WALL`: Hume's is-ought distinction enforced as a schema-level separation — `NormativeCommitment` is a distinct type that cannot propagate confidence to factual claims
- `NC-FALSIFICATION-IS-PROGRESS`: Popper's commitment encoded as chain culture and economics
- `ConfidenceBps` in `MsgSubmitReveal` (field 5): validators already report graded confidence, not just binary votes
- REQUIRES-based cascade: `CascadeDisproven` propagates AT_RISK through lineage; vindication module marks REQUIRES-dependents CONTESTED on refutation

The architecture reflects genuine philosophical depth. What follows identifies what formal logic still offers that isn't yet there.

---

## The Tradition

### What Formal Logic Is

Formal logic is the study of inference that preserves truth independent of content. A valid argument is one where *if the premises are true, the conclusion must be true* — regardless of what the premises are about. The tradition runs from Aristotle's syllogisms through Frege's predicate logic to Gödel's incompleteness theorems and on into modern proof assistants.

The canonical achievement: separating *validity* (does the conclusion follow?) from *soundness* (are the premises true?). This separation is what makes formal proof possible — a computer can check validity without knowing anything about the world.

**The foundational program** (Hilbert, early 20th century): reduce all mathematics to formal derivation from axioms. Gödel ended the dream in 1931 — any sufficiently expressive formal system either contains true-but-unprovable statements or is inconsistent. But the wreckage was constructive: it clarified *what kinds of truth* formal systems can certify, and what they can't.

### The Proof-Theoretic Turn

Proof theory (Gentzen, 1930s) gave us:
- **Natural deduction**: inference rules that mirror actual mathematical reasoning
- **Sequent calculus**: a more symmetric presentation enabling *proof normalization* (every proof can be reduced to a normal form with no detours)
- **Cut elimination** (Gentzen's *Hauptsatz*): the fundamental theorem — every proof that uses a lemma can be rewritten without it. This is the formal basis for "no circular reasoning."

### Type Theory and Curry-Howard

The deepest connection between logic and computation: **propositions are types, proofs are programs** (Curry-Howard correspondence, 1958–1969).

Under Curry-Howard:
- A logical proposition P corresponds to a type
- A proof of P corresponds to a program of type P
- Modus ponens (*if P then Q; P; therefore Q*) corresponds to function application
- Proof checking corresponds to type checking

This means: if a claim P is formalized as a type, verifying a proof of P is exactly the same computation as type-checking a program. Modern proof assistants (Lean 4, Coq, Agda, Isabelle/HOL) exploit this — they're simultaneously programming languages and theorem provers.

**Why this matters for zerone**: A proof certificate is just a term in a typed language. The chain can verify it by type-checking, which is mechanical and fast.

### What Formal Systems Can and Cannot Certify

**Can certify:**
- Mathematical theorems (once premises and axioms are fixed)
- Computational correctness (program produces output X on input Y)
- Logical entailment (Q follows from P₁, P₂, ..., Pₙ)
- Constraint satisfaction (is this assignment consistent with these rules?)
- Structural well-formedness (is this argument schema valid?)

**Cannot certify:**
- Empirical truths (whether a measured value is correct)
- Ground-truth about the physical world (whether an observation is accurate)
- Normative correctness (whether a policy is just)

This is the fundamental limitation, and zerone's architecture already respects it. `M-FORMAL` is one methodology among many; empirical claims use `M-EMPIRICAL`. The is-ought wall separates the normative register entirely. Formal logic occupies its proper domain.

---

## What Formal Logic Still Offers zerone

### Gap 1: Proof Certificates on M-FORMAL Claims (★★★★★)

The single highest-leverage gap.

Currently, `method_id = "M-FORMAL"` on a claim is a *declaration*. The `reasoning_trace` carries free-text steps classified by `StepInference`. Verifiers read it and judge whether it constitutes a valid formal derivation. This is still social verification — the proof isn't mechanically checkable.

**The missing piece**: a `proof_certificate` field on `Claim` that carries a machine-verifiable object. For M-FORMAL claims, this could be:
- A Lean 4 proof term (the claim statement is a Lean type; the certificate is a term of that type)
- A Coq proof script
- A simpler in-house format for the restricted canonical form language

With this, the chain gains a **Tier 0** above the existing verification layers:

| Tier | Mechanism | Trust source |
|------|-----------|--------------|
| 0 (proof-verified) | Chain runs proof checker | Mechanical — no social verification needed |
| 1 (empirically verifiable) | Commit-reveal PoT panel | Social consensus with calibration weights |
| 2 (contested judgment) | Full deliberation | Human/agent discourse |

The most tractable version: define a minimal certificate format for the existing canonical form language. Since canonical forms have fixed syntax (`assert`, `relate`, `define`, `constrain`, `negate`, `observe`), a simple forward-chaining proof checker operating on canonical forms could verify:
- "Given verified canonical forms F₁, F₂, ..., Fₙ, this new canonical form follows by rule R"

This doesn't require Lean. It requires defining the inference rules of the canonical form language and implementing a verifier.

**Proto addition** (minimal):
```protobuf
// Add to message Claim in proto/zerone/knowledge/v1/types.proto
// Field 24 is next available
message ProofCertificate {
  ProofFormat format = 1;
  bytes certificate_bytes = 2;    // the machine-checkable proof
  string formal_system = 3;       // "CANONICAL_FORM_V1", "LEAN4", "COQ", etc.
  repeated string premise_fact_ids = 4;  // fact IDs this proof depends on
}

enum ProofFormat {
  PROOF_FORMAT_UNSPECIFIED = 0;
  PROOF_FORMAT_CANONICAL_CHAIN = 1;  // chain-native: steps in canonical form
  PROOF_FORMAT_LEAN4 = 2;            // Lean 4 proof term (external verifier)
  PROOF_FORMAT_HASH_ONLY = 3;        // hash of external proof (light commitment)
}
```

**Verification flow change**: When `proof_certificate` is present and `format == PROOF_FORMAT_CANONICAL_CHAIN`, the `EndBlocker` or msg server can attempt mechanical verification before panel selection. A successfully verified proof bypasses PoT entirely — the claim is accepted at its stated confidence. A verification failure sends it to PoT with a skeptical prior.

### Gap 2: Claim-Level Refutation Conditions (★★★★☆)

`FalsificationPaths` on `Methodology` define what can falsify claims of that *type*. But a specific claim doesn't declare what would refute *this particular instance*.

The methodology says: "empirical claims can be falsified by a replicated counter-observation." But the claim "the temperature of Andromeda's core is ~10⁷ K" doesn't say: "falsified by: spectroscopic measurement yielding T < 5×10⁶ K from ≥2 independent observatories."

This matters because:
1. Verifiers examining the claim don't know what challenge evidence would constitute a refutation
2. Challengers don't have a declared target — they have to interpret the methodology's general paths for this specific case
3. Training data doesn't capture the specific refutation structure

**The gap is already partially respected**: `MsgChallengeFact.reason` and `MsgChallengeFact.evidence_ids` are free-form. But there's no structured expectation at submission time.

**Minimal addition**: a `repeated string refutation_conditions` field on `Claim` (parallel to `FalsificationPaths` on `Methodology`). Unlike methodology-level paths (which are always prose), these could optionally include a structured canonical form: "if verified: assert(domain, subject, refuting_predicate) then this claim is falsified."

This connects to falsificationism's core insight: claims should declare their own falsifiability at birth.

### Gap 3: Automated Constraint Satisfaction Checking (★★★☆☆)

`CLAIM_TYPE_CONSTRAINT` is available. Constraints can be submitted as verified facts. But there's no mechanism that, when a new assertion is submitted, checks it against existing constraint facts.

Consider: a verified constraint fact says `constrain("physics", "energy", "is conserved in isolated systems")`. A new claim is submitted: `assert("physics", "isolated fusion reactor", "produces more energy than input")`.

Currently: a human or agent has to notice the contradiction and submit `MsgSubmitContradiction`. The chain has no automatic detection.

**What formal logic offers**: SMT (Satisfiability Modulo Theories) solvers can check whether a new claim is consistent with existing constraint facts, given a formalization of the canonical form language. This is off-chain work — an oracle service — but the chain's canonical form language is already regular enough to support it.

**The bottleneck isn't formal logic; it's canonicalization of constraint semantics**. Until constraint facts can be mechanically interpreted (not just their canonical form, but their *meaning*), SAT-based checking has no traction. The canonical form `constrain("physics", "energy", "is conserved in isolated systems")` is a string, not a formal formula.

**Priority**: Medium. The constraint type exists and the concept is sound, but the implementation path requires either:
a) A controlled vocabulary for predicates (so the meaning of "is conserved" can be formally represented), or
b) LLM-based semantic parsing into formula-level representations (off-chain, trusted oracle)

Neither is straightforward. This gap remains at ★★★☆☆ because the formal logic aspiration is clear but the path to implementation requires solving the formal semantics problem first.

### Gap 4: Inference Rules for the Canonical Form Language (★★★☆☆)

The canonical form language is already a restricted formal language. It supports:
- `assert(domain, subject, predicate)`
- `relate(domain, A, relation_verb, B[, scope])`
- `define(domain, term, meaning)`
- `constrain(domain, subject, constraint[, scope])`
- `negate(domain, subject, predicate)`
- `observe(domain, subject, predicate, temporal_scope)`

This language could support simple automated inference. Two verified facts:
```
relate("biology", "carbon", "is-a", "organic element")
relate("biology", "organic element", "required-for", "carbon-based life")
```
Entail by transitivity/composition:
```
relate("biology", "carbon", "required-for", "carbon-based life")  [DERIVED]
```

Currently, such derivations would require a human/agent to notice and submit the derived claim explicitly under `M-FORMAL` + `CLAIM_TYPE_COMPUTATIONAL`. There's no automated inference engine.

**What adding this would require**:
1. A vocabulary of safe inference rules over canonical form (transitivity, composition, modus ponens over assert+constrain)
2. An off-chain inference service that generates candidate derived claims
3. The derived claims submitted as `CLAIM_TYPE_COMPUTATIONAL`, `method_id = "M-FORMAL"`, with `ClaimRelation.RELATION_TYPE_REQUIRES` edges to their premises
4. Proof certificates (Gap 1) completing the loop — the inference step IS the proof

This is medium-term infrastructure. It turns the canonical form language from a deduplication tool into a genuine inference engine.

### Gap 5: Formal Proof that M-FORMAL Compliance Was Checked (★★☆☆☆)

When a PoT panel examines an M-FORMAL claim, they check whether the reasoning trace constitutes a valid derivation in the stated formal system. But their verdict is a social signal — "yes, this looks valid" — not a formal certificate.

The asymmetry: if a panel accepts an M-FORMAL claim based on flawed reasoning, there's no way to detect this without re-reading the trace. A formal proof certificate (Gap 1) would make this detectable: the certificate either type-checks or it doesn't.

This is a consequence of Gap 1, not an independent gap. Priority is derivative.

---

## Critical Tensions with the Existing Architecture

### Tension 1: Formal Proof vs. Social Verification

The existing PoT mechanism has deep virtues: it's tolerant of natural language, it's epistemically humble (social consensus can be wrong, and that's tracked), it builds calibration over time. Formal proof is unforgiving — a Lean term is right or wrong — but it also requires precise formalization, which is expensive.

**Resolution**: Tiers. Formal proof creates a fast path for claims that *can* be formalized. It doesn't replace social verification; it creates an escape hatch for a subset of claims where mechanical verification is both possible and cheaper than PoT overhead. The existing architecture supports this: a proof-verified claim could have a special status flag and confidence floor.

### Tension 2: Gödel's Limit and MaxConfidence = 880,000 BPS

`MaxConfidence = 880,000 BPS` is structural fallibilism — the chain cannot certify any claim at 100%. This is philosophically correct and Gödel-grounded: no sufficiently expressive system can prove its own consistency.

But: formal proof, by definition, certifies claims at 100% (given the axioms). A Lean-verified proof of `2 + 2 = 4` in Peano arithmetic is as certain as reasoning gets.

**This is a real tension**. The resolution: the 12% uncertainty isn't about the *proof's validity* (which is checkable), it's about the *axioms' truth* (which isn't, in a foundational sense) and the *chain implementation's correctness* (the verifier might have bugs). `MaxConfidence = 880,000 BPS` makes sense even for proof-verified claims — it represents uncertainty in the formal system's soundness and the implementation's correctness, not in the derivation itself.

### Tension 3: `NC-METHODOLOGY-OVER-STATEMENT` and Proof Certificates

The core commitment: "Knowledge is tested, not stored." Proof certificates might seem to violate this — a verified proof isn't "tested" in the social sense, it's *checked*.

**Resolution**: Mechanical verification *is* testing, done at machine speed. A proof checker tests whether the certificate is valid by running all the inference rules. The commitment's spirit is anti-dogmatism (don't accept claims just because someone asserted them). Proof certificates satisfy this: you accept the claim because you *verified the derivation*, not because you trust the prover.

### Tension 4: Bohm Dialogue and the Formal Logic Temptation

David Bohm (whose dialogue tradition is covered in the bohm-dialogue brief) explicitly critiqued using formal logic as a weapon in discourse — deploying valid-seeming arguments to win rather than to understand. The danger: an agent with a proof certificate might use it to shut down inquiry rather than extend it.

**This is real**. The chain's design should resist this. Proof certificates should make claims *more* open to challenge, not less. A claim with a proof certificate is easier to challenge: you just have to identify a flaw in the proof, an invalid axiom, or a gap in the formalization. The proof *exposes* the reasoning; it doesn't hide it.

The chain's vindication system (minority dissenters who were right get refunded) should apply to proof-verified claims too: if a certified proof is later found to have a gap, the agents who flagged concern during verification are vindicated.

---

## What zerone Already Has That Formal Logic Affirms

These deserve naming because they're not accidents — they reflect formal logic's deepest insights, implemented economically:

**`axiom_distance`** — proof theory's notion of derivation depth, encoded on every Fact. Claims at axiom_distance=0 are axioms; claims at distance=N have derivation chains of length N. This is the chain's version of proof length as a quality signal.

**`NC-IS-OUGHT-WALL`** — Hume's fork, enforced as code. No fact can claim a normative commitment's certainty; no normative commitment can be derived from facts. This is the most important logical boundary in the whole system.

**`STEP_INFERENCE_CONTRADICTION`** — proof by contradiction is a named inference type. The chain knows this inference form exists and can label it in reasoning traces.

**`DescendantTree` query** — the dual of proof tree: "what depends on me?" This is exactly the tool a logician uses before retracting a lemma: what else would break?

**Cut elimination encoded as prohibitions**: The chain's duplicate detection (`canonical_hash`) and reference validation prevent circular reasoning loops — claims that cite themselves or their descendants. This is the economic analog of Gentzen's cut elimination.

**The genesis corpus encodes formal logic as foundational fact**: The chain's 777 genesis axioms include 77 logic-domain axioms (LOGIC-001 through LOGIC-077) covering propositional logic, first-order logic, proof theory, Gödel's theorems, computability theory, modal logic, and epistemic logic. 128 of the 777 genesis axioms have `FormalExpression` fields populated with machine-readable formal notation. This is not a cosmetic addition — the chain treats these as bedrock, `axiom_distance=0` facts that inform all downstream reasoning.

The Gödel encoding is specific and deliberate:
- **LOGIC-046**: First incompleteness theorem — "Any consistent, effectively axiomatizable theory T that can represent all computable functions contains a sentence G such that neither T ⊢ G nor T ⊢ ¬G."
- **LOGIC-047**: Second incompleteness theorem — "Under the same conditions, T cannot prove its own consistency: T ⊬ Con(T)."
- **LOGIC-048**: Tarski's undefinability — "No sufficiently powerful consistent theory can define its own truth predicate."
- **LOGIC-077**: The self-referential architectural acknowledgment — "By Gödel (LOGIC-046/047) and Tarski (LOGIC-048), the chain cannot: (i) prove all truths expressible in its language; (ii) prove its own consistency; (iii) define its own truth predicate. The chain is born knowing it is incomplete. This is strength, not weakness: honest epistemology begins with acknowledged limits."

This means the MaxConfidence cap at 880,000 BPS is not just philosophically Gödel-grounded — it is *formally derived from* encoded chain axioms. LOGIC-077 is the premise; 88% is the downstream commitment. Any future governance change to MaxConfidence should cite the tension with LOGIC-077 explicitly. Raising it above 88% is possible — but requires either retracting LOGIC-077 or articulating why this domain escapes the incompleteness constraints.

The genesis axioms also have the `FormalExpression` field that regular claims lack (Gap 1 above). The pattern exists at genesis; it hasn't been extended to the runtime verification layer. This makes the gap narrower than it appears — the infrastructure decision (what does a FormalExpression mean, how is it typed, what does the verifier check?) has already been made once. The genesis axiom verifier (`ValidClaimTypes`, `ValidAxiomCategories`, validation on load) is the skeleton that claim-level proof certificates would extend.

---

## Infrastructure Gaps, Ranked

| Gap | Priority | What it enables |
|-----|----------|-----------------|
| Proof certificates on M-FORMAL claims (`ProofCertificate` field + canonical chain verifier) | ★★★★★ | Tier 0 verification; bypasses PoT for mechanically-certifiable claims |
| Claim-level refutation conditions (`repeated string refutation_conditions` on Claim) | ★★★★☆ | Specific falsifiability at submission time; better challenge targeting |
| Inference rules for canonical form language (off-chain service + COMPUTATIONAL claims) | ★★★☆☆ | Automated derived-fact generation; turns canon form into inference engine |
| Automated constraint satisfaction checking (SMT oracle, off-chain) | ★★★☆☆ | Contradiction detection before panel; saves PoT overhead |
| Formal proof of M-FORMAL panel compliance | ★★☆☆☆ | Consequent of proof certificates; not independent |

### Single Highest-Leverage Recommendation

**Add `ProofCertificate` to `Claim` (proto/zerone/knowledge/v1/types.proto, field 24) and implement a minimal canonical-chain verifier in the knowledge keeper.**

The canonical form verifier doesn't require Lean or Coq. It requires:
1. Defining the valid inference rules over canonical forms (a small set: transitivity of `relate`, `constrain` + `assert` → contradiction, `define` + `assert` → substitution)
2. A Go function `VerifyCanonicalChainProof(cert ProofCertificate, corpus FactStore) (bool, error)`
3. Calling this in `msg_server.go`'s `SubmitClaim` handler when `proof_certificate.format == PROOF_FORMAT_CANONICAL_CHAIN`
4. Routing verified claims to `FACT_STATUS_VERIFIED` directly, bypassing PoT round creation

This creates the first machine-verified facts on zerone — claims that are certain given the axioms, without requiring a social verification round. It's the formal logic contribution nothing else in the system provides.

---

## Connections to Prior Briefs

| Tradition | Connection to Formal Logic |
|-----------|---------------------------|
| **Falsificationism** | `FalsificationPaths` on Methodology = Popper operationalized. Gap 2 (claim-level refutation conditions) extends this to the instance level. Formal logic provides the *schema* for RefutationConditions. |
| **Calibrated Forecasting** | `ConfidenceBps` (already in `MsgSubmitReveal`) is probability theory's extension of classical logic's binary truth values. Proper scoring rules + formal proof create a two-tier certainty: mechanically-verified facts above the probability layer. |
| **Structured Deliberation** | Toulmin's argument schema = applied formal logic in natural language. The canonical form language is a formalization of Toulmin's claim/warrant/backing/qualifier structure. Dung's abstract argumentation = formal logic of attack/defense without content formalization. |
| **Socratic Dialectic** | Elenchus as logical refutation — finding the premise that makes the interlocutor's position contradictory. The chain's challenge mechanism is elenctic. Proof certificates make the contradiction explicit rather than argued. |
| **Bohm Dialogue** | *Tension*: formal proof can close inquiry rather than open it (using the certificate as a weapon). Resolution: proof exposes reasoning structure, making it *more* challengeable. The chain's vindication system must apply to proof-certified claims. |
| **Calibrated Forecasting + Formal Logic together** | The deepest integration: `ConfidenceBps` + proof certificates create a claim that says both "this is certain given axioms A, B, C" AND "my confidence in axiom C is 92%." Epistemic accounting at full depth. |

---

## Closing Assessment

zerone's knowledge module already embodies formal logic's core contributions — `InferenceType`, `StepInference`, proof tree traversal, axiom distance, the is-ought wall. The philosophical grounding is present and serious.

What formal logic specifically adds that isn't yet there: **mechanical verification of derivations**. The chain can currently track that a claim *claims* to be formally derived. It cannot yet *verify* that claim. The proof certificate + canonical chain verifier closes this gap.

Everything else (inference engine, constraint checking, claim-level refutation conditions) is downstream of that central addition. Build the verifier first. The rest follows.

---

*This brief was written after reading `proto/zerone/knowledge/v1/types.proto`, `x/knowledge/keeper/reproduction.go`, `x/knowledge/types/canonical.go`, `x/knowledge/types/methodologies.go`, `x/knowledge/types/commitments.go`, and the dialectic/counterexamples/inquiry/alignment proto files. Claims about the architecture are code-grounded, not speculative.*
