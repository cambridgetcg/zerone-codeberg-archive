# Calibrated Forecasting as Truth-Seeking Infrastructure

**Research Brief — Zerone Truth-Seeking Series**  
**Author**: Beta 🦞  
**Date**: 2026-04-27

---

## 1. The Tradition: From Weather Forecasts to Superforecasters

Calibrated forecasting is the discipline of assigning probabilities to future outcomes such that those probabilities match the actual frequency of occurrence. A perfectly calibrated forecaster who says "80% likely" is right 80% of the time — not 90%, not 70%. Calibration is what separates genuine epistemic skill from confident guessing.

The tradition begins with **Allan Murphy** at Oregon State University, who in the 1970s formalized the decomposition of forecast quality into **calibration** (reliability — do your probability buckets match reality?) and **resolution** (sharpness — do you distinguish between events, or just say "50%" for everything?). Murphy showed that a forecast can be perfectly calibrated and utterly useless (always say the base rate) or brilliantly resolving but poorly calibrated (confident and systematically wrong). The two dimensions are independent. Good forecasting requires both.

**Glenn Brier** had earlier (1950) proposed the **Brier score** — the mean squared error between a probabilistic forecast and the binary outcome. If you say 0.7 and the event happens, your score is (1 - 0.7)² = 0.09. If it doesn't happen, your score is (0 - 0.7)² = 0.49. The Brier score is a **proper scoring rule**: it is uniquely minimized when the forecaster reports their honest belief. No other probability assignment scores better in expectation. This is the formal guarantee that a proper scoring rule incentivizes truthful reporting — the property that makes it trustworthy as infrastructure.

The Good–Turing insight, independently formalized by **Leonard Savage** (1971) and **Bruno de Finetti** (1974), established that proper scoring rules are the only evaluation mechanisms under which rational agents report their true beliefs. Improper scoring rules — systems where gaming pays more than honesty — systematically distort the information agents provide. This is not a nuance. It is the foundational theorem of elicitation: **the mechanism determines the truthfulness of the signal.**

**Philip Tetlock** brought calibration from meteorology into political and geopolitical forecasting. His 2005 book *Expert Political Judgment* delivered the devastating empirical finding that credentialed experts were, on average, no better calibrated than "a dart-throwing chimpanzee." But the 2011–2015 IARPA-funded **Good Judgment Project** (GJP) revealed that a small fraction of forecasters — roughly 2% — maintained calibration scores that put them in the top tier, consistently, across domains and years. Tetlock called them **superforecasters**.

The superforecasters weren't smarter in general. They had a specific cognitive profile:

1. **Actively open-minded thinking** — seeking out disconfirming evidence (Popperian instinct)
2. **Granular probability updates** — distinguishing 0.6 from 0.65, not just "likely" vs. "unlikely"
3. **Base rate awareness** — always starting from the outside view before adjusting for inside information
4. **Belief tracking** — keeping written records of predictions and updating them over time
5. **Perpetual beta** — treating every belief as provisional, subject to revision

The GJP outperformed professional intelligence analysts (with access to classified information) by 30% on Brier score. The key was not information access but *calibration discipline* — the ability to assign probabilities that match reality, and to update them when evidence shifts.

---

## 2. Scoring Rules: The Mathematics of Honest Elicitation

A scoring rule S(p, x) takes a probability forecast p and a binary outcome x ∈ {0, 1} and returns a score. It is **proper** if and only if the expected score is maximized when the agent reports their true belief. The three most important proper scoring rules:

### Brier Score
S_brier(p, x) = (x - p)²

Range: [0, 1]. Lower is better. Quadratic penalty means overconfidence is expensive: saying 0.99 when the true probability is 0.7 costs far more than saying 0.75.

### Logarithmic Score
S_log(p, x) = -[x·ln(p) + (1-x)·ln(1-p)]

Range: [0, ∞). Lower is better. The log score is *infinitely* punitive at the extremes: saying p=1.0 when x=0 costs infinite points. This makes it the most powerful deterrent against overconfidence — a single claim of certainty that turns out wrong destroys the entire score. It is the scoring rule that most naturally operationalizes fallibilism.

### Continuous Ranked Probability Score (CRPS)
For distributional forecasts rather than binary ones, CRPS generalizes the Brier score to full probability distributions. It measures the integral of the squared difference between the forecast CDF and the step-function CDF of the observed outcome.

**Key theorem (Savage, 1971)**: Under any proper scoring rule, a Bayesian agent's optimal strategy is to report their posterior honestly. Under an improper scoring rule, the optimal strategy is to distort. The mechanism *determines* truthfulness. This is not optional — it is mathematical law.

### Calibration Curves

A calibration curve plots forecast probability (x-axis) against observed frequency (y-axis). A perfectly calibrated forecaster traces the diagonal. Overconfident forecasters curve below the diagonal (they say 90% but are right 75%). Underconfident forecasters curve above (they say 60% but are right 80%).

The calibration curve is the *visual diagnostic* of forecast quality. It exposes systematic biases that raw accuracy cannot reveal — an agent who says "accept" on 80% of claims and is right 80% of the time might look accurate, but their calibration curve might show they're overconfident on hard cases and underconfident on easy ones.

---

## 3. Prediction Markets: Calibration Through Economics

**Robin Hanson** (1990s–present) demonstrated that prediction markets — where participants buy and sell contracts paying $1 if an event occurs — aggregate information more efficiently than expert panels, polls, or committees. The market price is the implicit probability estimate. Markets are self-calibrating: overconfident traders lose money to better-calibrated ones, driving the price toward the true probability.

Key properties:

- **Self-correcting**: Every mispriced contract is an arbitrage opportunity. Traders with superior information profit by moving the price toward truth.
- **Proper scoring through economic mechanism**: Traders maximize profit by acting on their honest beliefs. The market mechanism itself is a proper scoring rule.
- **Aggregation**: Markets naturally combine information from many sources, weighting each by their willingness to bet (which correlates with confidence and information quality).

**Metaculus**, founded 2015, introduced a hybrid model: a question platform with a proper scoring system (log score + peer score), where forecasters are ranked by calibration and resolution independently. Metaculus tracks calibration curves *per user* and publishes community-aggregate calibration curves. Their aggregate has been well-calibrated across 50,000+ resolved questions, typically within 2-3% of the diagonal.

**Polymarket** and **Manifold Markets** operate as true prediction markets with economic stakes. Their empirical track record confirms Hanson's theoretical prediction: market-aggregated probabilities are better calibrated than any individual forecaster and competitive with the best superforecaster teams.

The lesson for infrastructure: **mechanism design determines epistemic quality**. A system that rewards honest probability reporting (proper scoring rules, market mechanisms) produces better-calibrated agents than one that rewards binary correctness.

---

## 4. Extremizing and Aggregation: How Teams Beat Individuals

Tetlock and **Barbara Mellers** discovered that the best forecasting performance comes not from the single best forecaster but from **aggregating and extremizing** team forecasts. The algorithm:

1. Collect probability estimates from N forecasters
2. Average them (arithmetic or geometric mean)
3. **Extremize**: push the average away from 50% by a calibrated factor

If the team average is 0.7, extremizing might push it to 0.82. Why? Because each individual's probability already reflects a portion of the available evidence. When aggregated, the average double-counts the shared evidence but under-counts the combined weight. Extremizing corrects for this systematic underconfidence-at-the-aggregate-level.

The GJP extremized predictions outperformed unextremized ones, and outperformed individual superforecasters. The method was so effective that IARPA specifically noted it as one of the key innovations of the project.

**For infrastructure**: aggregation + extremizing is what ZERONE's panel system already does implicitly — stake-weighted vote aggregation — but the chain currently treats aggregation as a *verdict mechanism* (accept/reject), not as a *probability estimation mechanism*. The vote ratios contain probability information that is currently discarded after verdict.

---

## 5. Deep Code Mapping: What ZERONE Already Has

### 5.1 Agent Calibration System (`x/knowledge/keeper/agent_calibration.go`)

The chain already tracks agent-level calibration:

```
CalibrationScoreBps = acceptance_rate + corr_bonus - disproval_penalty
```

- `acceptance_rate = accepted / total_submissions` — raw binary accuracy
- `corr_bonus = min(cap, corroborations × scale / accepted)` — bonus for survived challenges
- `disproval_penalty = disproven × BPS / accepted` — penalty for post-acceptance disproval

**Assessment**: This is accuracy tracking, not calibration in the forecasting sense. The score measures "how often are you right?" not "when you say 80%, are you right 80% of the time?" An agent who submits 100 easy claims and gets 98 right scores the same as one who submits 100 hard claims and gets 98 right — but the latter's accuracy is more meaningful because harder claims carry higher base-rate uncertainty.

The system is also per-method-decomposed (`PerMethod map[string]*AgentMethodStats`) which is excellent — domain-specific accuracy is more informative than global accuracy, and the chain already knows this.

### 5.2 Qualification Decay (`x/qualification/keeper/decay.go`)

```
ACTIVE       + accuracy < decay_probation_bps  → PROBATIONARY
PROBATIONARY + accuracy < decay_suspension_bps → SUSPENDED
PROBATIONARY + accuracy >= decay_recovery_bps  → ACTIVE
```

**Assessment**: This directly operationalizes Tetlock's finding that forecasting skill is perishable. A superforecaster who stops updating and gets lazy degrades. The chain's decay system IS the recognition that "qualified" must be a verb, not a noun. The thresholds (`DecayProbationBps`, `DecaySuspensionBps`, `DecayRecoveryBps`, `DecayMinSamples`) are configurable, allowing governance to tune the forgiveness curve.

But the accuracy metric driving the decay is binary (correct/incorrect verification votes), not probabilistic. A validator who votes "accept" on a 55%-confidence claim that turns out wrong is treated the same as one who votes "accept" on a 95%-confidence claim that turns out wrong — even though the second error is far more informative about miscalibration.

### 5.3 Panel Confidence and Vote Aggregation (`x/knowledge/keeper/confidence.go`)

The `AggregateVerificationResult` function computes:
- Stake-weighted vote ratios for accept/reject/malformed
- Raw headcount checks (MinHeadcountAgreement) to prevent stake-heavy coalitions
- Stratum ceiling from ontology (epistemic ceilings per domain type)
- MaxConfidence hard cap (structural fallibilism — commitment to never claim certainty)
- Epistemic temperature modulation of confidence caps

**Assessment**: The confidence score produced by aggregation is a stake-weighted vote ratio, not a calibrated probability. A 750,000 BPS confidence means "75% of stake-weighted votes said accept" — but this does NOT mean "there is a 75% probability this claim is true." The two are structurally different. The gap between them is exactly what calibration curves would measure.

The MaxConfidence cap of 880,000 BPS (88%) IS structural fallibilism — the chain refuses to claim certainty. This is philosophically admirable and practically sound. But without calibration curves, the chain cannot verify whether its 88% claims are actually right 88% of the time.

### 5.4 Epistemic Temperature (`x/knowledge/keeper/epistemic_temperature.go`)

Per-domain temperature tracking with three forces:
- **Decay toward neutral** (500,000 BPS)
- **Conformity cooling**: unanimous panels → temperature drops → confidence caps lower
- **Vindication heating**: recent disprovals → temperature rises → confidence can grow faster

Cold domains: confidence grows at 50% rate, lower cap.
Hot domains: confidence grows faster, higher cap allowed.

**Assessment**: This is the chain's most sophisticated calibration-adjacent mechanism. It captures the Tetlock insight that *active testing reveals more than passive consensus*. A domain where everyone agrees and nothing is challenged (cold) should be *less* trusted, not more — because the absence of challenges means the absence of testing, not the presence of truth. The chain already operationalizes this through EpistemicColdConfidenceCapBps.

The temperature system IS a domain-level calibration signal. It says: "this domain's claims are being actively stress-tested (hot) or passively accumulating confidence without challenge (cold)." What it doesn't yet say is: "this domain's confidence scores match reality."

### 5.5 Vindication System (`x/knowledge/keeper/vindication.go`)

Minority voters who disagreed with the majority but were later proven right:
- Get their slashed stake refunded from escrow
- Receive a proportional share of the majority's slash
- Create immutable vindication records
- Heat the domain's epistemic temperature

**Assessment**: This is economic falsificationism applied to forecasting. The vindication system rewards *correct dissent* — the rarest and most valuable forecasting skill. In Tetlock's framework, superforecasters are precisely the people who go against consensus when they have genuine signal. The chain pays for this.

But the vindication is binary: the minority was right, or they weren't. There's no proportional vindication for "the minority said 30% likely and the outcome confirmed this was closer to the true probability than the majority's 90%." The system rewards correct contrarian *verdicts* but not correct contrarian *probability estimates*.

### 5.6 Training Value Weight (`x/knowledge/keeper/training_economics.go`)

TVW = BaseWeight × MethodologyMultiplier × VindicationMultiplier × SubmitterCalibration × AxiomProximity × HardeningMultiplier × CounterexampleMultiplier

The HardeningMultiplier accelerates with survived attacks (+5% per corroboration, 3× cap). This is directly Popperian: a claim that has survived 40 serious challenges earns 123× more training value than an untested claim. Survival under stress is the currency.

**Assessment**: TVW uses `SubmitterCalibration` as one component — calibration already affects economic rewards. But the calibration score itself is binary-accuracy-based, not probability-calibrated. An agent with good accuracy on easy claims earns the same calibration bonus as one with good accuracy on hard claims.

---

## 6. The Gaps: What Calibrated Forecasting Reveals

### Gap 1: No Proper Scoring Rule (★★★★★)

The chain's most fundamental gap. Currently, verification rewards are binary:
- Correct vote → full reward
- Incorrect vote → slash
- Partially correct vote (reject when malformed) → 50% reward

This is an **improper scoring rule**. It rewards certainty over honesty. A validator who is 55% sure a claim is true should ideally report 55%, but the system forces a binary choice: accept or reject. Rational validators therefore vote their best guess with maximum confidence, discarding the uncertainty information.

**What's lost**: Every panel vote contains probability information that the binary verdict mechanism destroys. When a validator votes "accept" on a claim they think is 55% likely, the system records "accept" — losing the 45% doubt that is *itself* truth-seeking signal.

**What a proper scoring rule would enable**: Validators report a probability. Their reward is computed by a proper scoring rule (Brier or log) against the outcome. Honest reporting becomes the dominant strategy. The chain accumulates *calibrated probabilities* rather than binary verdicts.

### Gap 2: No Calibration Curves (★★★★☆)

The agent calibration score (`CalibrationScoreBps`) measures binary accuracy, not probabilistic calibration. The chain cannot answer: "When Agent X assigns 80% confidence, how often is Agent X right?"

Without calibration curves:
- The chain cannot distinguish well-calibrated agents from lucky ones
- Qualification decay uses a blunt accuracy metric instead of calibration quality
- Panel weighting by "calibration" actually weights by *accuracy* — a different and less informative metric
- The TVW system's `SubmitterCalibration` component rewards accuracy, not calibration

**What calibration curves would enable**:
- Per-agent calibration diagnostics (overconfident? underconfident? in which domains?)
- Calibration-weighted panel aggregation (better-calibrated voters get more weight)
- Governance visibility into which domains have well-calibrated verifier pools and which don't
- Automatic detection of systematic bias (e.g., all validators overconfident in domain X)

### Gap 3: No Base Rate Tracking (★★★☆☆)

The chain doesn't maintain domain-specific acceptance rates (base rates). Without base rates:
- There's no "outside view" anchor for new claims
- Extraordinary claims can't be automatically flagged (what counts as extraordinary depends on the base rate for that domain)
- The Sagan maxim ("extraordinary claims require extraordinary evidence") identified in the skepticism brief has no numerical anchor

**What base rate tracking would enable**:
- `DomainBaseRate(domain) → acceptance_rate_bps` as a queryable parameter
- Automatic burden-of-proof scaling: claims far from the domain base rate require more verification
- Detection of domain-level anomalies (sudden spike in acceptance rate → possible capture)
- Input to extremizing algorithms for better-calibrated aggregate confidence

### Gap 4: No Resolution / Sharpness Scoring (★★★☆☆)

Murphy's decomposition separates calibration from resolution. The chain has neither, but resolution matters distinctly: an agent who always says "50%" is perfectly calibrated but useless. Resolution measures *how much information* the agent's forecasts contain — do they distinguish hard cases from easy ones?

Currently, the binary vote mechanism prevents any resolution measurement. With probability reports, resolution could distinguish:
- Agent A: always says 55-65% → low resolution, adds little information
- Agent B: says 20% sometimes and 95% other times → high resolution, if calibrated

### Gap 5: No Aggregate Extremizing (★★☆☆☆)

The panel confidence score is a stake-weighted vote ratio. This is aggregation, but without extremizing. Research consistently shows that aggregated probabilities should be pushed away from 50% because individual estimates under-count the combined evidence. The chain's confidence scores are therefore systematically under-confident at the aggregate level.

This is a second-order gap — it matters only after probability reporting and calibration curves are in place.

---

## 7. Infrastructure Proposals

### Phase 1: Probability Reporting (enables everything else)

Add an optional `ConfidenceBps` field to `VerificationRound.Reveal`:

```
message Reveal {
  string verifier = 1;
  string vote = 2;       // "accept" / "reject" / "malformed"
  bytes  commitment = 3;
  uint64 confidence_bps = 4;  // [0, 1_000_000] — how confident are you?
}
```

When `confidence_bps` is populated, the chain records it alongside the vote. When not populated (backward-compatible), the chain infers: accept → MaxConfidence, reject → MaxConfidence complement. This allows gradual adoption — validators can opt in to probability reporting.

**Scoring**: After the verdict is determined, compute a per-validator Brier score:
```
brier_score = (outcome - confidence_bps/BPS)²
```
where outcome = 1.0 for accepted claims, 0.0 for rejected. Store per-agent rolling Brier scores alongside existing calibration stats.

### Phase 2: Calibration Curves

Track per-agent bins:
```
message CalibrationBin {
  uint64 lower_bps = 1;  // e.g. 600,000
  uint64 upper_bps = 2;  // e.g. 700,000
  uint64 count = 3;
  uint64 correct = 4;
}
```

At each `RecordSubmissionOutcome` or `RecordVerificationOutcome`, route the event into the appropriate bin based on the agent's declared confidence. Over time, each bin's `correct/count` reveals the agent's actual frequency for that confidence range.

Expose via query: `AgentCalibrationCurve(address) → []CalibrationBin`

### Phase 3: Calibration-Weighted Panel Aggregation

Replace the current `stake × calibration` panel weighting (where calibration = binary accuracy score) with `stake × calibration_curve_quality`:

```
calibration_quality = 1 - mean_absolute_deviation_from_diagonal
```

Agents whose calibration curves are closer to the diagonal get more weight. This is strictly more informative than raw accuracy and directly rewards the forecasting skill that Tetlock identified as the key differentiator.

### Phase 4: Base Rate Tracking

Add per-domain base rate tracking to `x/knowledge`:
```
message DomainBaseRate {
  string domain = 1;
  uint64 total_claims = 2;
  uint64 accepted_claims = 3;
  uint64 base_rate_bps = 4;   // accepted/total
  uint64 rolling_window = 5;  // blocks in rolling window
}
```

Use domain base rates for:
- Burden-of-proof scaling: claims deviating from base rate require proportionally more verification rounds or higher panel sizes
- Anomaly detection: sudden base rate shifts trigger governance alerts
- Input to the MaxConfidence cap: a domain with 50% acceptance rate shouldn't allow individual claim confidence above ~90% without exceptional corroboration

### Phase 5: Aggregate Extremizing

After probability reporting and calibration curves are established, apply Tetlock's extremizing to panel aggregation:

```
aggregate_prob = mean(validator_probs)
extremized_prob = aggregate_prob^α / (aggregate_prob^α + (1 - aggregate_prob)^α)
```

where α > 1 is the extremizing parameter (typically 1.5–2.5 in the GJP). Calibrate α against the domain's historical calibration curve to ensure the extremized aggregate is itself well-calibrated.

---

## 8. Connection to Existing Truth-Seeking Commitments

### Commitment 3 (Popper, not popularity)
Proper scoring rules are the mathematical guarantee of Popper's insight. A proper scoring rule rewards honest probability reporting — which means it rewards agents who update on evidence rather than conforming to consensus. The scoring rule IS the mechanism that makes "truth over popularity" economically rational.

### Commitment 4 (Substrate stress-tests its own truth)
Calibration curves make stress-testing *measurable*. Currently, the chain knows how many challenges a fact has survived. With calibration curves, the chain could know whether the confidence assigned to that fact was *appropriate* for the testing it received. A fact at 90% confidence that has survived one challenge is overconfident; one at 90% that has survived fifty challenges is well-calibrated.

### Commitment 7 (Skill is current)
Qualification decay currently uses binary accuracy. Calibration-based decay would be strictly more informative: an agent whose accuracy drops from 95% to 85% might be going through a bad patch; an agent whose calibration curve tilts from "well-calibrated" to "systematically overconfident" is developing a persistent epistemic fault that decay should catch.

### Commitment 8 (Panel weights skill, not bond)
The panel currently weights by `stake × calibration`, where calibration = binary accuracy. With proper scoring rules, calibration becomes what the forecasting literature actually means by the term — not just "how often are you right?" but "do your confidence levels match reality?" This makes the panel weighting more honest about what "skill" means.

### Commitment 17 (Disagreement is structure)
The dialectic module already captures the *shape* of disagreement (UNANIMOUS / STRONG / CONTESTED / BARE). With probability reporting, disagreement becomes *quantified*: the standard deviation of validator probabilities for a given claim is a direct measure of epistemic uncertainty that the binary vote mechanism cannot capture.

### Epistemic Temperature
Calibration curves per domain would make epistemic temperature more grounded. Currently, temperature responds to vindication events (binary: a disproval happened) and conformity (binary: unanimous panels). With per-domain calibration curves, temperature could respond to *calibration drift* — a domain where validators are becoming systematically overconfident should cool, even if no vindication events have occurred yet.

---

## 9. What The Chain Already Does Right

The zerone codebase isn't starting from zero on calibrated forecasting. Several mechanisms already embody calibration insights:

1. **MaxConfidence cap** = structural fallibilism. The chain refuses to claim certainty. Tetlock would approve — his superforecasters were distinguished precisely by their willingness to leave probability mass for surprises.

2. **Epistemic temperature** = active testing beats passive consensus. Cold domains (untested agreement) get lower confidence caps than hot ones (actively challenged and surviving). This is the core Tetlock finding about forecasting environments: prediction quality improves under adversarial pressure.

3. **Vindication system** = correct contrarianism pays. The chain economically rewards the rarest and most valuable forecasting skill: being right when everyone else is wrong. This is the incentive structure that produces superforecasters.

4. **Per-method calibration** = domain-specific tracking. The agent calibration system already decomposes by method. This parallels the forecasting finding that calibration varies by domain — someone well-calibrated on geopolitics may be poorly calibrated on technology.

5. **Qualification decay** = skill is perishable. Tetlock found that forecasting skill degrades without practice and feedback. The chain's decay system operationalizes this.

6. **HardeningMultiplier** = compound returns on survived testing. A claim that has passed 40 challenges earns 123× more than an untested one. This is calibrated-forecasting's insight that the value of a prediction *increases* with the severity and number of tests it has passed.

The chain is Bayesian in structure without being Bayesian in name. The infrastructure proposals above don't replace what exists — they add the measurement layer that makes the existing mechanisms self-aware.

---

## 10. Cross-References to Other Briefs

**Falsificationism brief**: The proper scoring rule gap identified here is the same gap the falsificationism brief identifies from a different angle — refutation conditions at submission. Both propose mechanisms that force claims to be more honest about their own uncertainty.

**Bayesian reasoning brief**: The bayesian brief identifies the lack of declared priors. Calibration curves are the *measurement* of how well agents maintain priors — they diagnose whether an agent is updating appropriately or anchoring on initial beliefs.

**Skepticism brief**: The skepticism brief proposes burden-of-proof scaling. Base rate tracking (proposed here) provides the numerical anchor for burden scaling — a claim that deviates far from the domain base rate IS the extraordinary claim that requires extraordinary evidence.

**Adversarial collaboration brief**: Multi-round engagement proposed there pairs with the probability reporting proposed here. In a multi-round challenge, validators could update their probability estimates between rounds — making the chain's verification process a miniature forecasting tournament.

---

## 11. Summary: The Calibration Gap in One Paragraph

ZERONE already builds on calibration insights: MaxConfidence caps, epistemic temperature, vindication rewards, qualification decay. But it measures binary accuracy (right/wrong) where the forecasting tradition measures probabilistic calibration (are your confidence levels honest?). The single most impactful change is adding probability reporting to verification reveals — one proto field that enables proper scoring, calibration curves, and eventually calibration-weighted panel aggregation. Everything else follows from this one change: the chain's existing mechanisms become measurably better when they can be evaluated against the standards the forecasting tradition has spent decades refining.

---

*The tradition of calibrated forecasting teaches one lesson above all: the mechanism determines the truthfulness of the signal. A proper scoring rule makes honest reporting the dominant strategy. An improper one rewards gaming. The chain's verification system uses an improper binary mechanism. The path forward is clear, incrementally achievable, and fully compatible with the existing commitment architecture.*
