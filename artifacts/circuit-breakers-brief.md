# Circuit Breakers and Crisis Governance
*Zerone Financial Substrate — Brief 9 of 10*

---

## Methodology

Circuit breakers are automatic or semi-automatic mechanisms that halt system operations when conditions exceed safe parameters. The name comes from electrical engineering: a breaker that trips when current exceeds a threshold, preventing equipment damage.

In financial systems, circuit breakers were introduced after Black Monday (1987): the NYSE halt rule that pauses trading when the Dow drops 7%, 13%, or 20%. In DeFi, circuit breakers were retrofitted after the events of March 2020 (Maker DAI liquidation failure), the UST depeg (2022), and the Curve reentrancy attack (2023).

The design question is always: **automatic vs. manual, and at what granularity?**
- **Automatic**: triggered by algorithm, no delay, but prone to false positives and gaming
- **Manual/Governance**: less false positives, but potentially too slow for rapidly developing crises
- **Granular**: pause only the affected module/pool; less collateral damage
- **Global**: halt everything; maximum safety, maximum disruption

**Key sources**: Duffie & Zhu (2011, Does a Central Clearing Counterparty Reduce Counterparty Risk?); NYSE circuit breaker documentation; MakerDAO post-mortem on March 2020; Euler Finance hack post-mortem (2023); Uniswap v3 emergency admin documentation; Compound governance pause functions; Cosmos SDK `x/crisis` module.

---

## What Zerone Already Has

### 1. Guardian-Governed Emergency Halt System (`x/emergency`)

A state machine with five statuses:

```
NORMAL → HALT_VOTING → HALTED → REVERT_VOTING → REVERTING
                                   ↘ RESUME_VOTING (alternative to revert)
```

**Halt parameters:**
- `HaltQuorum = 750_000` (75% of Guardian-tier stake) — supermajority required
- Guardian-tier validators and genesis council members can vote
- `EmergencyHaltProposal` carries evidence hash + suggested revert block

**Revert ceremony:**
- `EmergencyRevertProposal` carries target state root + block hash — full chain revert is possible
- This is extremely powerful — more surgical than most DeFi systems allow

### 2. Genesis Emergency Council (Bootstrap Safety Valve)

During bootstrap (before sufficient Guardian validators exist), a genesis council of known addresses holds emergency power via `CouncilVirtualStake`. The council has an expiry block (`CouncilExpiryBlock`) after which it dissolves. This handles the bootstrap problem correctly: you need an emergency mechanism before decentralization is achieved, but you also need it to expire.

### 3. Pool-Level Lock Flag (`x/liquiditypool`)

`Pool.Locked bool` — individual AMM pools can be halted without touching the rest of the system. This is a granular circuit breaker: a compromised pool can be isolated without halting all trading.

### 4. Bootstrap Fund Rate Limiting (`x/knowledge`)

The bootstrap fund's per-epoch and per-address limits are a soft circuit breaker against fund drainage attacks. They don't halt the system but slow down any attack that relies on rapid repeated transactions.

### 5. Audit Log

`k.AddAuditEntry(ctx, entry)` — emergency actions are logged to an on-chain audit trail. Every halt proposal, vote, and revert is permanently recorded. This is the accountability layer for emergency governance.

---

## Gaps and Gaps Only

### Gap 1: No Automatic Circuit Breakers (★★★★★)

**The critical gap.** All emergency mechanisms require manual Guardian proposals. A fast-developing attack (flash loan, oracle manipulation, exploit) can drain significant value before a 75%-quorum halt vote completes.

**Why this matters**: In DeFi, attacks often complete within seconds to minutes. The Euler Finance attack drained $197M in one transaction. Manual governance is too slow for this threat model.

**Sketch**: Add `AutoHaltConditions` to `Params`:
```
type AutoHaltCondition struct {
    MetricType      string   // "oracle_deviation", "slash_rate", "liquidity_drain", "fee_spike"
    ThresholdBps    uint64   // trigger threshold
    WindowBlocks    uint64   // measurement window
    HaltTarget      string   // "all", "knowledge", "liquiditypool:pool_id"
    AutoResumeBlocks uint64  // 0 = manual resume required; N = auto-resume after N blocks
}
```

In `EndBlocker`, evaluate each `AutoHaltCondition` against current metrics. If triggered, call a new `TriggerAutoHalt(ctx, condition, triggeredMetric)` function that:
1. Emits an `emergency.auto_halt_triggered` event
2. Sets the target module's halt flag
3. Notifies Guardian validators via event (they can override within a window)

Critical: the `HaltTarget` granularity means knowledge module can be halted (no new claims) without halting the AMM, and vice versa.

### Gap 2: No Granular Module-Level Pause (★★★★☆)

The emergency module's halt is a full chain halt (via `EmergencyHaltProposal`). There is no mechanism to pause only the knowledge module (claim submissions) while leaving the AMM running, or to pause new round creation while allowing existing rounds to complete.

**Why this matters**: Full chain halts are extremely disruptive. Most emergencies are localized — an oracle manipulation attack on the AMM shouldn't halt verification rounds in the knowledge module.

**Sketch**: Add a `ModulePauseRegistry` to a new `x/pause` module (or extend `x/emergency`). Modules register their pause handler at startup: `pauseRegistry.Register("knowledge", k.PauseClaimSubmission)`. Guardian governance can call `MsgPauseModule{target: "knowledge"}` without triggering a full chain halt. The `IsGuardian` check already exists — use it for per-module pause authorization.

### Gap 3: No Leading Indicator Monitoring (★★★★☆)

The emergency module responds to *reported* crises. There is no continuous monitoring of key health metrics that could trigger preemptive action before full halt is needed.

**Why this matters**: Circuit breakers are most effective as **leading indicators** — warning lights before the crisis escalates to halt. By the time a Guardian proposes a halt, the damage may be done.

**Sketch**: Add a `ProtocolHealthScore` query that aggregates:
- `oracle_freshness`: blocks since last TWAP update vs. staleness threshold
- `active_rounds_ratio`: active rounds / max capacity (from fee market Brief 5)
- `guardian_participation`: fraction of Guardians active in recent rounds
- `slash_rate_per_epoch`: recent slash events / expected

Emit a `ProtocolHealthUpdate` event every N blocks. If `HealthScore < CriticalThresholdBps`, auto-trigger the `AutoHaltCondition` evaluation (Gap 1). This creates a layered response: health score → auto-halt conditions → manual halt.

### Gap 4: No Time-Boxed Halt (Auto-Resume) (★★★☆☆)

A halted chain requires a `StatusResumeVoting` governance action to resume. There is no built-in time limit on the halt duration.

**Why this matters**: Indefinite halts are economically catastrophic — they prevent honest users from accessing funds, completing verification rounds, or withdrawing liquidity. The halt should have a maximum duration after which the chain automatically resumes (possibly at the pre-halt state if a revert occurred, or at the current state if the halt was precautionary).

**Sketch**: Add `MaxHaltDurationBlocks uint64` to `Params`. In `EndBlocker`, if `ctx.BlockHeight() > haltCeremony.ProposeAtBlock + MaxHaltDurationBlocks` and status is still `HALTED`, automatically transition to `RESUME_VOTING` with a majority-threshold quorum (lower than the 75% halt quorum). After a further N blocks with no successful resume vote, auto-resume with current state (no revert). This bounds the worst-case disruption duration.

### Gap 5: No Partial State Recovery (Hot Patching) (★★★☆☆)

The revert mechanism can revert the entire chain state to a prior block. There is no mechanism for **surgical state recovery**: correcting only specific module state (e.g., patching a specific incorrectly-credited account) without reverting everything.

**Why this matters**: Full state reverts discard all legitimate transactions since the crisis block, not just the malicious ones. Users who made valid submissions during the crisis period lose their work. A targeted state patch would be less disruptive.

**Sketch**: Add `MsgExecuteStateRecoveryScript` requiring Guardian supermajority approval. The "script" is a list of explicit state mutations `[{store: "knowledge", key: "...", old_value: "...", new_value: "..."}]` that the community verifies off-chain and submits as a governance proposal. This is Ethereum's equivalent of post-hack "refund transactions" — surgical recovery without full revert.

---

## Convergence Notes (for Synthesis)

- **Automatic circuit breakers** (Gap 1) depend on the **health metrics** defined across all financial substrate briefs: oracle freshness (Brief 8), active rounds count (Brief 5), slash rate (Brief 3), TWAP deviation (Brief 8), pool depth (Brief 7).
- **Granular module pause** (Gap 2) requires that each module implement a pause handler — this is a cross-cutting concern that should be standardized in the synthesis.
- **Leading indicator monitoring** (Gap 3) is the operational expression of the truth-seeking series' **calibrated forecasting** — health scores are probability estimates of system stability.
- **Time-boxed halts** (Gap 4) convergences with **settlement finality** (Brief 1, Gap 3: timeout finality for abandoned rounds) — both are about bounding the maximum duration of a stuck state.
- **Tombstoning** (slashing, Brief 3, Gap 1) should automatically trigger **Guardian notification** in the emergency system — repeated slashing of multiple validators is a leading indicator of a coordinated attack.

---

## Summary Verdict

Zerone's emergency module is **substantially more capable than most DeFi protocols at equivalent stage**. The Guardian-governed ceremony system, revert-to-state-root capability, genesis council bootstrap, pool-level locks, and permanent audit log constitute a genuine crisis governance infrastructure.

The critical gap is **all triggers are manual**. A 75%-quorum vote takes time — likely longer than most attacks take to execute. Automatic circuit breakers based on measurable anomalies (oracle deviation, excessive slashing, liquidity drain rate) would provide a fast-path defense.

Priority recommendation: implement `AutoHaltConditions` in `Params`, add `EndBlocker` evaluation logic, and start with two conditions: (1) TWAP deviation > 30% from 100-block moving average, and (2) slash rate > 3 in one epoch. These two automatic triggers cover the most common attack vectors (price oracle manipulation and validator coordination failure) with low false-positive risk.
