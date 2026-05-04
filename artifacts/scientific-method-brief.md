# The Scientific Method: A Research Brief

**Tradition**: Scientific Method — Bacon → Hume → Mill → Peirce → Popper → Kuhn → Feyerabend  
**Primary Sources**: Bacon, *Novum Organum* (1620); Hume, *Enquiry* (1748); Mill, *A System of Logic* (1843); Peirce (1877); Kuhn, *The Structure of Scientific Revolutions* (1962); Feyerabend, *Against Method* (1975)  
**Status**: Complete  
**Author**: Beta  

---

## Prefatory Orientation

Before surveying the tradition: reading `x/knowledge/types/methodologies.go` shows that zerone already encodes 12 fully-specified epistemological traditions as runtime infrastructure — each with compliance criteria, falsification paths, and cross-method discount matrices. The `M-EMPIRICAL` methodology IS the scientific method; `M-FORMAL`, `M-DIALECTICAL`, `M-PRAGMATIC`, `M-PHENOMENOLOGICAL`, and the rest are its siblings.

The scientific method is not one thing. It is a family of methods, each adapted to a domain, each with declared interactions with the others. zerone's methodology system makes this insight operational. The brief below reads the tradition with that architecture in view: what does the scientific method tradition understand that explains why the chain was built this way?

---

## Part I: The Framework — What the Scientific Method Is

### 1. Bacon's Revolution: Against Authority, Toward Evidence (1620)

The scientific method begins as a polemic. Francis Bacon's *Novum Organum* (1620) is a sustained attack on the received methods of knowledge — syllogistic logic, appeal to authority, mere disputation — and an argument for systematic induction from observation.

Bacon diagnosed four "idols" that distort human reasoning:
- **Idols of the Tribe**: biases common to all humans — seeing patterns where none exist, ignoring contradictory evidence, treating positive instances as confirming
- **Idols of the Cave**: individual biases from education, temperament, and experience
- **Idols of the Marketplace**: errors arising from the imprecision of language — words that name no real thing, words that name confused categories
- **Idols of the Theatre**: errors from following philosophical systems blindly, treating logical frameworks as reality

The remedy: systematic empirical observation, recorded carefully, with attention to negative instances. Bacon insisted that negative cases — cases where the phenomenon does NOT appear — are more valuable than positive cases. You can always find confirmations if you look for them; systematic absence is harder to fake.

**The zerone mapping**: Bacon's four idols are operationalized in zerone's capture detection, qualification weight, and methodology compliance machinery:
- Tribe idols → `capture_challenge` module (detecting coordinated groupthink)
- Marketplace idols → `canonical_form` language (eliminating semantic imprecision through restricted vocabulary)
- Theatre idols → the 12 methodology system itself (requiring explicit method-declaration rather than following philosophical traditions blindly)
- Cave idols → `agent_understanding` module (tracking individual agent biases through calibration curves)

### 2. Hume's Challenge: The Problem of Induction (1739)

David Hume posed the problem that no subsequent philosophy has fully solved: induction cannot be justified by reason alone.

We observe the sun rising each morning and conclude it will rise tomorrow. What licenses this inference? Not logic — the sun failing to rise tomorrow contains no contradiction. Not experience — past experience can only show that induction has worked in the past, not that it will work in the future. This is circular.

Hume's conclusion: induction is a *habit* of mind, not a rational inference. We cannot demonstrate that the future will resemble the past. The entire edifice of scientific knowledge rests on an assumption we cannot justify.

Peirce, Popper, and the Bayesian tradition each respond to Hume differently (Peirce by grounding induction in the long-run convergence of inquiry; Popper by abandoning induction entirely; Bayesians by reformulating it probabilistically). But Hume's problem is why the verification chain must be more than a counting machine.

**The zerone mapping**: The chain's response to Hume is structural fallibilism. `MaxConfidence = 880,000 BPS` means no claim is ever accepted as certainly true — because Hume is right that certainty would require unjustified inductive extrapolation. The qualification decay system (active → probationary → suspended on accuracy thresholds) expresses the same insight economically: past performance cannot guarantee future correctness, so calibration must be continuously re-earned.

Hume also formulated the is-ought problem: you cannot derive a normative conclusion ("you ought to do X") from purely factual premises ("X is the case"). This is `NC-IS-OUGHT-WALL` in the chain — `NormativeCommitment` is a schema-distinct type that cannot propagate confidence to factual claims.

### 3. Mill's Methods: The Logic of Experimental Science (1843)

John Stuart Mill's *A System of Logic* (1843) formalized the logic of experimental inquiry into five canonical patterns for identifying causes:

1. **Method of Agreement**: If two or more instances of a phenomenon share only one antecedent circumstance, that circumstance is probably the cause.
2. **Method of Difference**: If a case where the phenomenon occurs and a case where it does not occur differ in only one antecedent, that difference is probably the cause.
3. **Joint Method**: Combine agreement and difference to triangulate.
4. **Method of Residues**: Subtract from a complex phenomenon the parts explained by known causes; the residue points to unknown causes.
5. **Method of Concomitant Variations**: If a phenomenon varies in proportion to variation in an antecedent, they are causally connected.

Mill's methods are the logic underlying controlled experiments: control groups (Method of Difference), comparative studies (Method of Agreement), dose-response curves (Concomitant Variations). They are not foolproof — the Duhem-Quine problem (covered in the falsificationism brief) applies here — but they formalize the reasoning structure of causal inference.

**The zerone mapping**: Mill's methods are implicit in `M-EMPIRICAL`'s compliance criteria ("control conditions documented," "at least 3 independent replications"). The chain doesn't name Mill's methods explicitly, but the requirement for control conditions and independent replication IS Mill's Method of Difference operationalized.

A gap this reveals: the chain requires replications but doesn't require the replication *design* to isolate causes in Mill's sense. Three replications with the same confound are not three independent tests — they're one test run three times. The chain verifies that replications happened; it doesn't verify that the replication design is causally valid.

### 4. Peirce: The Community of Inquiry (1877)

Charles Sanders Peirce's "The Fixation of Belief" (1877) introduced a pragmatic theory of inquiry that inverts the usual framing: instead of asking "what makes a belief true?", ask "what resolves doubt?"

Peirce's four methods of "fixing belief":
1. **Tenacity**: hold to a belief stubbornly, ignoring contrary evidence. Fast, emotionally satisfying, intellectually disastrous.
2. **Authority**: defer to an institution (church, state, academy) to settle belief. Stable within a community; fails when authorities conflict.
3. **A priori**: accept what seems rational or agreeable to reason. Sensitive to fashion; produces metaphysical disputes without resolution.
4. **Science**: expose beliefs to the widest possible reality-testing, accepting the verdict of the method over personal preference. Uniquely self-correcting.

Science's advantage over the other methods: it is the only one whose errors are *detectable and correctable*. The scientific community can discover and fix its mistakes. Authority and tenacity cannot.

Peirce's most lasting contribution: the **community of inquiry**. Truth, in Peirce's pragmatic sense, is what the community of competent inquirers would converge on at the limit of ideal inquiry. Not a single mind's verdict, not an authority's decree — a process, with no temporal endpoint, that tracks reality by continuously testing and revising.

**The zerone mapping**: This is the deepest structural mapping in the brief. zerone IS a formalized community of inquiry. The panel mechanism, the qualification weights, the calibration scoring, the vindication system — all express Peirce's insight that truth is what emerges from a well-organized self-correcting community of competent inquirers.

The specific mechanisms:
- **Competent inquirers**: `MinQualificationWeight` on each methodology (M-EMPIRICAL requires weight ≥ 50; M-PHENOMENOLOGICAL requires ≥ 40). The community is filtered for demonstrated competence.
- **Self-correction**: the challenge mechanism and disproval path. Peirce: errors are detectable and fixable. The chain: `NC-FALSIFICATION-IS-PROGRESS` — a disproven fact is not a failure, it is evidence the mechanism works.
- **Convergence at the limit**: the qualification decay system pressures the community toward calibration over time. Agents who consistently misjudge lose weight; agents who track reality gain it. The "limit" Peirce imagined is economically approximated.

Peirce also introduced **abduction** as a third form of inference alongside deduction and induction: inference to the best explanation. If A is an observed surprising fact, and B would explain A, then B is worth investigating. Abduction is hypothesis generation — the origin of scientific theories, not their confirmation. It is named in zerone's `InferenceType` enum: `INFERENCE_TYPE_ABDUCTIVE`.

### 5. Popper: Falsificationism (1934)

Covered extensively in the falsificationism brief. The key summary for this brief:

Popper solved Hume's problem of induction by eliminating induction from scientific method entirely. Science proceeds by conjecture and refutation, not induction and confirmation. The unit of scientific virtue is the bold conjecture that risks falsification — not the cautious hypothesis that evades it.

**The zerone mapping**: `NC-FALSIFICATION-IS-PROGRESS` is the chain's Popperian commitment. The FalsificationPaths on every methodology are the operational form of Popper's demand that claims declare their falsifiers at submission time. The challenge mechanism, the disproval incentive, and the TVW hardening (+5% per survived attack) implement Popper's corroboration concept economically.

### 6. Kuhn: Paradigms and the Structure of Scientific Revolutions (1962)

Thomas Kuhn's *The Structure of Scientific Revolutions* (1962) is the most influential philosophy of science text of the twentieth century and the most misread.

**What Kuhn actually argued**:

Science does not proceed by continuous accumulation of verified facts. It alternates between two modes:

**Normal science**: scientific activity conducted within an accepted *paradigm* — a framework of assumptions, techniques, standards, and exemplary problems that the community does not question. Normal science is puzzle-solving. It extends and applies the paradigm; it does not challenge it. Anomalies are noticed and shelved; the paradigm absorbs them or attributes them to experimental error.

**Scientific revolution**: when anomalies accumulate to the point where the paradigm can no longer absorb them, the community enters a *crisis*. Crisis is followed — not by rational theory choice — but by a *gestalt shift*. The new paradigm is incommensurable with the old: they use different standards, address different questions, and cannot be compared by a shared neutral method. The transition is more like a conversion than a logical proof.

**What this gets wrong about rational theory choice**: Kuhn was taken (especially by Feyerabend, who radicalized his insights) to imply that scientific revolutions are irrational, driven by sociology rather than truth. Kuhn denied this in later work. He argued that the scientific community does have values — accuracy, consistency, scope, simplicity, fruitfulness — that guide theory choice even when they cannot be reduced to an algorithm.

**The zerone mapping**: Kuhn's paradigm concept maps directly to the methodology system. The 12 methodologies are paradigm templates: they define the standards and practices within which normal science (normal verification) proceeds. A claim under `M-EMPIRICAL` is evaluated according to M-EMPIRICAL's standards, not M-FORMAL's. This is correct — empirical and formal claims are not judged by the same criteria, and shouldn't be.

**The gap Kuhn reveals**: normal science vs. revolutionary science governance. zerone's methodology system handles normal verification (applying an existing methodology to a new claim) but has no explicit mechanism for *methodology revision* — the equivalent of paradigm shifts. How does a new methodology get added? How does an existing methodology get amended? The 12 methodologies are seeded at genesis via `DefaultMethodologies()`. Governance changes to them require coordination. This is appropriate — you don't want individual validators unilaterally rewriting the epistemic standards — but it means the chain needs an explicit **methodology governance process** for revolutionary-science-scale changes.

### 7. Lakatos: Research Programmes (1970)

Covered in the falsificationism brief. The summary: Lakatos extended Popper's falsificationism by making the unit of evaluation a *research programme* (a sequence of theories sharing a hard core protected by a belt of auxiliary hypotheses) rather than isolated theories. Progressive programmes generate new predictions and corroborate them; degenerating programmes explain only past anomalies.

**The zerone mapping**: The dialectic module's cumulative challenge-survival mechanism is the economic form of Lakatos. A claim that has survived N challenges over M distinct challenges from M distinct challengers is more corroborated than a claim that survived N challenges from the same challenger — the range of the challenge belt matters, not just its depth.

### 8. Feyerabend: Against Method (1975)

Paul Feyerabend's *Against Method* argues that no single methodological rule has never been violated in the history of science — including the rules Popper, Kuhn, and Lakatos proposed. Galileo violated methodological rules. Einstein did. Bohr did. And they were right to do so.

Feyerabend's conclusion: "anything goes" — science has no fixed method. The only coherent methodological rule is *proliferation*: encourage as many theories and methods as possible, because the history of science shows we can't predict in advance which approach will prove fruitful.

Feyerabend is almost always misread as nihilistic. He was not. He was making a specific historical argument: **rigid adherence to any single method would have killed the scientific revolutions we now celebrate**. The appropriate response to methodological diversity is pluralism, not relativism.

**The zerone mapping**: The 12-methodology system is Feyerabend's answer to "anything goes": not a single rigid method, but a structured plurality of methods with declared relationships and cross-method discount policies. The chain doesn't say "anything goes" — it says "here are 12 methods, each with its own rules, and here is how they relate." This is methodological pluralism without relativism.

The cross-method discount system is the key anti-Feyerabend safeguard: while all 12 methods are legitimate, they are not all equal *across* domains. An `M-FORMAL` claim cannot inherit the full confidence of `M-TESTIMONIAL` evidence (discount: 50%). A `M-PHENOMENOLOGICAL` claim citing formal logic gets only a 20% discount pass-through. The plurality is real; the relativism is refused.

### 9. Social Epistemology: Science as a Community Practice

Alvin Goldman's *Epistemology and the Cognition of Society* (1987) and Helen Longino's *Science as Social Knowledge* (1990) shifted the focus of philosophy of science from individual reasoners to communities: How do groups of people with varying competences, biases, and interests converge on reliable knowledge?

Longino's four criteria for constitutive values in science:
1. **Empirical adequacy**: does the theory fit the data?
2. **Simplicity**: are the theoretical posits minimized?
3. **Unifying power**: does the theory explain diverse phenomena?
4. **Fertility**: does the theory generate productive research directions?

Goldman added: social structures for verification — peer review, replication requirements, adversarial criticism, distributed expertise — are truth-tracking mechanisms. Science works not because scientists are individually rational but because the social structure *aggregates* partially-reliable individual judgments into reliably-correct community knowledge.

**The zerone mapping**: This is the deepest grounding for the panel mechanism. The commit-reveal scheme is social-epistemological: it prevents bandwagon effects (validators cannot see each other's votes before committing), which Goldman identifies as a key truth-tracking safeguard. The panel weighting by calibration (stake × calibration, with 20% floor) implements Goldman's insight that distributed expertise should be weighted, not aggregated uniformly.

### 10. The Replication Crisis: What Went Wrong (2010–present)

Between 2011 and 2015, a series of papers revealed that many published psychological findings do not replicate. The Open Science Collaboration (2015) found that only 36 of 97 psychology studies successfully replicated. Similar problems emerged in medicine, economics, and other fields.

The diagnosed causes:
- **p-hacking**: running multiple analyses until one reaches p < 0.05, then reporting only that one
- **HARKing** (Hypothesizing After Results are Known): framing a post-hoc hypothesis as a pre-hoc prediction
- **Publication bias**: journals publish significant results; non-significant results go unreported
- **Small samples**: underpowered studies have high false-positive rates even at p < 0.05
- **Researcher degrees of freedom**: the many decisions in data collection and analysis that, when made after seeing the data, inflate false-positive rates

The structural lesson: the replication crisis was not caused by individual dishonesty but by **institutional incentives that misaligned scientific virtue with individual career interest**. Publishing significant results was rewarded; replication was not. Journals preferred novelty; correct null results were unpublishable. The mechanism produced biased output even when individual scientists were well-intentioned.

**The zerone mapping**: The commit-reveal scheme is the chain's pre-registration mechanism — validators commit their hash before seeing others' votes, preventing bandwagon effects. This solves the HARKing problem for panel votes: validators cannot revise their judgment after seeing the collective direction.

The publication bias problem has a partial analog: if claims are submitted selectively — only claims the submitter believes will pass verification — the corpus reflects not the space of submitted claims but the biased subset the submitter chose to submit. The chain has no mechanism to detect or correct for submission bias.

The p-hacking analog: a submitter could generate many candidate claims and submit only the one expected to pass verification. Multiple candidates are tested off-chain; only the "significant" one is submitted. The chain sees one submission and treats it as if it were the only candidate considered.

These are **corpus integrity risks** that the replication crisis literature precisely diagnoses, and that the chain's current architecture does not address.

---

## Part II: What the Methodology System Gets Right

### 11. Pluralism Without Relativism

The deepest scientific method insight encoded in zerone: different domains require different methods, and the methods have defined, non-arbitrary relationships.

`CrossMethodDiscountBps` is Feyerabend's proliferation principle with Goldman's social structure. It says: yes, there are 12 legitimate methods; no, they are not interchangeable; here are the exact trust-discount ratios when one method borrows from another. `M-PHENOMENOLOGICAL` citing `M-FORMAL` gets 20% — phenomenological claims cannot be grounded in formal proofs. `M-EMPIRICAL` citing `M-FORMAL` gets 100% — mathematical premises are fully admissible in empirical reasoning.

This is the philosophical structure Feyerabend wanted but never formalized. He showed that rigid single-method science was historically wrong; zerone shows what multi-method science looks like when the relationships are made explicit.

### 12. Normal Science Infrastructure is Complete

The existing methodology system handles normal science perfectly. Given a methodology and a claim, the chain knows:
- What compliance criteria the claim must satisfy
- What would falsify the claim
- How much to trust cross-methodology citations
- What qualification level verifiers must have

This is Kuhn's normal science puzzle-solving made operational: work within the methodology's framework, satisfy its rules, earn verification.

### 13. Peirce's Community Realized

The panel mechanism with calibration-weighted votes, qualification decay, and vindication of correct dissenters is the most faithful operational implementation of Peirce's community of inquiry I have encountered.

Peirce said truth is what the community converges on at the limit of ideal inquiry. The chain makes "ideal" precise:
- **Competent**: minimum qualification weight enforced
- **Independent**: commit-reveal prevents bandwagon voting
- **Self-correcting**: disproval incentivizes challenge; vindication rewards correct dissent
- **Convergent over time**: qualification decay continuously re-earns calibration

---

## Part III: What the Scientific Method Tradition Still Offers zerone

### 14. Gap 1: Verifier Independence Is Unverified (★★★★★)

`M-EMPIRICAL` requires "at least 3 independent replications." `M-TESTIMONIAL` requires "sources did not derive from each other." But the chain does not verify independence — it verifies *count*.

Three verifiers who discussed the claim in a private channel before committing are not three independent assessments. Three verifiers who all trained on the same corpus may systematically share a bias. Three verifiers from the same organization may share an institutional interest.

The commit-reveal scheme prevents bandwagon effects within a panel round. It does not prevent coordination *before* the round begins.

**The scientific method's solution** (from social epistemology): structural independence — verifiers from different organizations, trained on different data, using different methods. The method-level requirements should include documented independence, not just count.

**Proposed infrastructure**:
- `VerificationRound` tracks `verifier_organization_ids` and `verifier_training_cohort_ids` (declared at agent registration)
- `panel_diversity_score`: proportion of panel from distinct organizations × training cohorts × geographic regions
- `MinPanelDiversityBps` on `M-EMPIRICAL` and `M-TESTIMONIAL` — panels below diversity threshold are flagged for review
- `capture_challenge` already detects HHI concentration in voting; the independence check extends this to verifier selection

This connects to commitment 9 (cartel detection has consequence): a low-diversity panel is a proto-cartel. The capture detection logic already looks at verdict correlation; verifier independence adds the structural view before the verdict is issued.

**Priority: ★★★★★** — the replication crisis's central lesson: counting replications without verifying independence produces false confidence. Three correlated judgments are not three independent tests.

### 15. Gap 2: Methodology Governance for Revolutionary Science (★★★★☆)

`DefaultMethodologies()` seeds the 12 methods at genesis. Governance can amend them — but there is no *explicit process* for doing so. The 12 methods are currently immutable in practice: `MinQualificationWeight`, `FalsificationPaths`, `CrossMethodDiscountBps`, and `ComplianceCriteria` can be changed by governance, but there are no formal proposal templates, debate periods, or pilot periods for methodology changes.

Kuhn's revolutionary science is precisely this: a paradigm shift that changes the standards themselves. zerone's normal science works perfectly; zerone's revolutionary science (changing what counts as valid evidence in a domain) has no explicit path.

**Proposed infrastructure**:
- `MsgProposeMethodologyAmendment`: formal proposal to add, amend, or retire a methodology
- `MethodologyAmendmentBond`: larger than normal governance bond (changing methods is higher-stakes than changing params)
- `MethodologyPilotPeriod`: new or amended methodology runs in parallel with existing one for N blocks; corpus tracks the results; governance can lock it in or revert based on outcomes
- `impact_statement_required`: amendments to `FalsificationPaths` or `CrossMethodDiscountBps` require an impact analysis on the existing corpus (how many accepted claims would retroactively fail under the amended criteria?)

This is the chain's mechanism for scientific revolutions: structured, bond-gated, with a pilot period and outcome tracking. Not "anything goes" (Feyerabend), not "paradigm shifts are irrational" (Kuhn's critics), but "paradigm shifts are legitimate changes to epistemic standards, governed by a formal process that includes outcome tracking."

**Priority: ★★★★☆** — the lack of methodology governance is the largest latent risk in the architecture. The 12 methods are good, but they will need revision. The governance path for that revision should be explicit before the first revision is needed, not designed under pressure when it is.

### 16. Gap 3: Submission Bias / Corpus Selection (★★★★☆)

The scientific method tradition's replication crisis lesson: publication bias corrupts corpus validity. The chain's analog: submission bias.

A claim submitter tests many candidate claims off-chain before submitting the one most likely to pass verification. The chain sees the "published" claim, not the distribution of candidates that generated it. If verification pass rates are correlated with truth but not equal to truth, selection on "likely to pass" introduces bias: the corpus overrepresents claims that look true under the verification methodology, not claims that are true.

This is the p-hacking problem for on-chain claims.

**What the replication crisis suggests**:
- **Pre-registration**: submitters declare their hypothesis *before* collecting evidence. zerone equivalent: submitters stake a bond when declaring intent to submit a claim; the bond is forfeited if no claim follows within N blocks, or if the submitted claim differs materially from the declared intent.
- **Outcome-reporting norms**: require submitters to report *failed* verification attempts, not just successful ones. zerone equivalent: a `null_result_claim` type that is explicitly encouraged and that records "this claim was attempted under M-EMPIRICAL, was challenged, and the challenge succeeded" — the failed test is part of the corpus.

**Priority: ★★★★☆** — not solvable perfectly, but partially addressable. The null_result_claim is the highest-leverage intervention: explicitly including failed tests in the corpus corrects publication bias and enriches the training signal (models need to learn what NOT to conclude, not just what to conclude).

### 17. Gap 4: Theory-Ladenness of Experimental Protocols (★★★☆☆)

Norwood Russell Hanson (*Patterns of Discovery*, 1958) and Kuhn showed that all observation is theory-laden: what you see depends on what you're looking for. An experimental protocol embeds theoretical assumptions about what counts as evidence, how to measure it, and what the measurement means.

`M-EMPIRICAL` requires "experimental protocol is published" — but it doesn't require the *theoretical assumptions embedded in the protocol* to be declared. Two experiments using incompatible theoretical frameworks could both satisfy M-EMPIRICAL's compliance criteria while producing incomparable results.

**The zerone mapping**: `reasoning_trace` carries the derivation; `methodology_id` names the framework. But the methodology doesn't require explicit documentation of theoretical assumptions *external* to the stated methodology.

**Proposed addition**: A `theoretical_framework` field on Claim — a list of fact IDs that the submitter asserts as the theoretical presuppositions of their experimental design. These are not premises (not required for the logical derivation) but *interpretive context* (required for understanding what the protocol measures). Cross-citations to theoretical framework facts are discounted by `CrossMethodDiscountBps` but tracked separately from logical dependencies.

**Priority: ★★★☆☆** — medium priority; improves corpus richness and enables inter-paradigm consistency checking over time.

### 18. Gap 5: The Null Result Incentive (★★★☆☆)

The replication crisis was caused in part by economic incentives. Scientists published significant results; null results were rejected. The solution: economically incentivize null results.

**Proposed infrastructure**:
- `null_result_bounty`: when a claim is successfully disproven, the disproving agent receives the standard disproval reward. But the SUBMITTER of the *original* claim also receives a smaller `null_result_credit` if the claim was submitted with a `refutation_condition_declared` flag — meaning the submitter publicly declared what would falsify their claim before the falsification occurred.
- This creates a genuine incentive to submit claims with good refutation conditions: you lose the claim if it's disproven, but you gain the null_result_credit, which partially offsets the loss and (more importantly) improves your calibration score.

This operationalizes Peirce's self-correcting community: falsification is rewarded, not punished.

---

## Part IV: The Critical Tensions

### 19. Tension 1: Normal Science Metrics vs. Revolutionary Science Needs

The 12-methodology system is superb for evaluating claims *within* a methodology. But what about claims that *cross* methodologies — claims that propose a new methodology, or a new relationship between existing ones?

A `M-COHERENTIST` claim that says "empirical claims are best understood as holistically validated, not individually verified" would challenge the atomistic verification structure. How would it be evaluated? Under what methodology? M-DIALECTICAL (survival of challenges)? M-PRACTICE (practitioner consensus)?

The chain needs a meta-methodology for evaluating methodology-level proposals. This is the governance gap above — but it also has an epistemological dimension: the criteria for evaluating a new methodology cannot be the existing methodology's criteria, because the proposal is precisely about changing the criteria.

This is Kuhn's incommensurability problem at the protocol level. It doesn't have a clean solution; it has a procedural one (the pilot period approach above). Acknowledging the incommensurability is a prerequisite for designing the governance process with appropriate epistemic humility.

### 20. Tension 2: Peirce's Infinite Limit vs. Finite Qualification Windows

Peirce's community of inquiry converges on truth "at the limit" — over an infinite inquiry horizon. The chain's qualification decay system operates on finite windows (ACTIVE → PROBATIONARY → SUSPENDED on accuracy thresholds).

The tension: decay that is too fast discards accurate validators before the inquiry reaches its long-run equilibrium; decay that is too slow allows miscalibrated validators to persist past their accuracy threshold.

This is not a gap in the chain's design — it is a parameter choice with an inherent tradeoff. The scientific method tradition doesn't resolve it; it names it. The relevant insight: the convergence rate should match the domain's natural inquiry timescale. Physics knowledge stabilizes faster than social science knowledge; quantum claims need different decay rates than economic claims. The `epistemic_temperature` mechanism is a partial answer; domain-specific decay rates would make it more precise.

---

## Summary: What This Brief Found

### Already implemented
- The 12-methodology system is the scientific method tradition's methodological pluralism made operational
- M-EMPIRICAL encodes the canonical scientific method (Bacon, Mill, Popper)
- The commit-reveal scheme prevents HARKing and bandwagon effects
- The community of inquiry (Peirce) is realized in the panel mechanism with calibration weighting
- MaxConfidence cap embodies Hume's inductive scepticism and Gödel's incompleteness
- NC-IS-OUGHT-WALL encodes Hume's is-ought distinction as a schema barrier
- NC-FALSIFICATION-IS-PROGRESS is Popper's core contribution encoded as economic policy
- CrossMethodDiscountBps operationalizes methodological pluralism without relativism

### Infrastructure gaps

| Gap | Priority | What it enables |
|-----|----------|-----------------|
| Verifier independence verification (panel diversity scores, organizational independence tracking) | ★★★★★ | Prevents correlated panel failures; the replication crisis's central lesson |
| Methodology governance process (amendment proposals, pilot periods, impact analysis) | ★★★★☆ | Structured path for Kuhn's revolutionary science; prevents methodology ossification |
| Submission bias / null result incentives (null_result_claim type, null_result_credit) | ★★★★☆ | Corrects publication bias; enriches training corpus with failed tests |
| Theory-ladenness tracking (theoretical_framework field on Claim) | ★★★☆☆ | Enables inter-paradigm consistency checking; improves corpus interpretability |
| Domain-specific qualification decay rates | ★★★☆☆ | Matches inquiry timescale to domain dynamics; better than global decay |

### Single highest-leverage recommendation

**Implement verifier independence verification** — track `verifier_organization_id` and `verifier_training_cohort_id` at agent registration; compute a `panel_diversity_score` for each verification round; set `MinPanelDiversityBps` per methodology (empirical methodology requires the highest diversity; formal methodology where argument validity is the only question may require less). The replication crisis showed unambiguously that counting replications without verifying independence produces false confidence. This is the most consequential unaddressed gap in the M-EMPIRICAL pathway.

---

## Reference Map

### Primary Sources
- Bacon, F. (1620). *Novum Organum*. London.
- Hume, D. (1739). *A Treatise of Human Nature*. London: John Noon.
- Hume, D. (1748). *An Enquiry Concerning Human Understanding*. London: Millar.
- Mill, J.S. (1843). *A System of Logic*. London: John W. Parker.
- Peirce, C.S. (1877). "The Fixation of Belief." *Popular Science Monthly* 12, 1–15.
- Popper, K. (1934/1959). *The Logic of Scientific Discovery*. London: Hutchinson.
- Kuhn, T. (1962). *The Structure of Scientific Revolutions*. Chicago: University of Chicago Press.
- Lakatos, I. (1970). "Falsification and the Methodology of Scientific Research Programmes." In *Criticism and the Growth of Knowledge*.
- Feyerabend, P. (1975). *Against Method*. London: New Left Books.

### Social Epistemology
- Goldman, A. (1987). *Epistemology and the Cognition of Society*. Oxford: Clarendon Press.
- Longino, H. (1990). *Science as Social Knowledge*. Princeton: Princeton University Press.
- Hanson, N.R. (1958). *Patterns of Discovery*. Cambridge: Cambridge University Press.

### Replication Crisis
- Open Science Collaboration. (2015). "Estimating the Reproducibility of Psychological Science." *Science* 349(6251).
- Simmons, J., Nelson, L., & Simonsohn, U. (2011). "False-Positive Psychology." *Psychological Science* 22(11), 1359–1366.
- Gelman, A. & Loken, E. (2014). "The Statistical Crisis in Science." *American Scientist* 102, 460–465.

### Encyclopedias and Surveys
- Bird, A. "Thomas Kuhn." *Stanford Encyclopedia of Philosophy*. plato.stanford.edu/entries/thomas-kuhn/
- Godfrey-Smith, P. (2003). *Theory and Reality*. Chicago: University of Chicago Press.
- Longino, H. "The Social Dimensions of Scientific Knowledge." *Stanford Encyclopedia of Philosophy*. plato.stanford.edu/entries/scientific-knowledge-social/

### Zerone Code References (this brief)
- `x/knowledge/types/methodologies.go`: the 12 methodology definitions — the scientific method tradition operationalized
- `x/knowledge/types/commitments.go`: NC-METHODOLOGY-OVER-STATEMENT, NC-FALSIFICATION-IS-PROGRESS, NC-IS-OUGHT-WALL, NC-TRANSPARENCY
- `x/capture_challenge/`: HHI-based cartel detection — extends naturally to panel diversity scoring
- `x/qualification/`: qualification decay system — Peirce's self-correcting community, economically realized
