# Slashing and Economic Security
*Zerone Financial Substrate — Brief 3 of 10*

---

## Methodology

Economic security in proof-of-stake systems rests on a single theorem: **an attack costs at least as much as the attacker can gain**. Slashing is the mechanism that makes this theorem credible — it transforms stake from a passive holding into a bonded penalty that can be destroyed for provable misbehavior.

The academic tradition draws from:
- **Nakamoto consensus**: security as cumulative computational work (cost of rewriting history)
- **Tendermint/PBFT**: safety under 2/3 honest validators; slashing for equivocation (signing two conflicting blocks at the same height)
- **Ethereum Casper**: slashing conditions for surround votes and double votes; the "accountable safety" property
- **Principal-agent theory**: slashing resolves the agent problem — validators have skin in the game, so their interests align with protocol correctness
- **Mechanism design**: Vickrey-Clarke-Groves style — proper scoring rules for truthfulness (connects back to Brier scoring from Brief 10 of truth-seeking series)

**Key sources**: Buterin (2017, Casper the Friendly Finality Gadget); Kwon (2014, Tendermint); Garg et al. (2019, Thunderella, incentive-compatible consensus); Fudenberg & Tirole (1991, Game Theory, chapter on mechanism design); Cosmos SDK slashing module documentation.

---

## What Zerone Already Has

### 1. Progressive Slash Escalation (`x/staking/keeper/keeper.go`)

Zerone's slashing system is not flat — it escalates with repeat violations:

```
adjustedAmount = baseAmount × (1_000_000 + slashCount × SlashEscalationBps) / 1_000_000
```

**Default parameters** (`x/staking/types/genesis.go`):
- `SlashEscalationBps = 100_000` (10% per prior slash)
- `MaxSlashesPerEpoch = 2` (rate limit within an epoch)
- `SlashDecayPeriodBlocks = 34_272` (~1 day at ~2.5s/block)

This implements the correct incentive structure: the first violation costs X, the second costs 1.1X, the third 1.2X. Repeat offenders pay progressively more, not a flat penalty.

### 2. Per-Epoch Slash Cap with Windowed Decay

`SlashesThisEpoch` caps the number of slashes within `SlashDecayPeriodBlocks`. The windowed decay in `getWindowedSlashCount` (tiers.go) treats slashes older than `epochLength × slashWindowEpochs` as zeroed out for tier computation purposes — a validator's history has a half-life.

This is a thoughtful design: old misbehavior shouldn't permanently disqualify a validator who has since behaved correctly. The decay is per-tier, so higher tiers (Scholar, Guardian) have stricter windows.

### 3. Four-Tier Stake-Based Security Budget

The tier system creates **differentiated economic security**:

| Tier | Guardian | Scholar | Verified | Apprentice |
|------|----------|---------|----------|------------|
| Min Stake | Highest | Medium | Low | None |
| Slash Window | 0 tolerance | Strict | Moderate | Lenient |
| Min Accuracy | Highest | High | Moderate | Low |
| Min Verifications | Highest | High | Moderate | Low |
| Contested Multiplier | Yes | No | No | No |

Higher-tier validators provide more economic security (more stake at risk) and face stricter slashing conditions. This is the correct mechanism: the cost of corrupting a Guardian is orders of magnitude higher than corrupting an Apprentice.

### 4. Stake Layering: Self-Delegation + Delegated Stake

`SlashValidator` slashes from `SelfDelegation` first; overflow absorbs into `DelegatedStake`. This follows the Cosmos pattern: operators put skin in the game first (self-delegation), and delegators absorb only if the operator's stake is exhausted. It creates proper alignment: operators who undercapitalize their own stake expose delegators to excessive risk.

### 5. Slash Routing to Specific Module Accounts

`SlashValidatorToModule(addr, amount, destModule)` routes slashed tokens to a specified module. This enables:
- **Vindication escrow** (`VindicationEscrowModuleName`): incorrect verifiers' slashed tokens held for potential refund if the fact they rejected is later confirmed correct
- **Development fund**: default routing for non-vindication slashes

This is a sophisticated design — slashing is not simply token destruction but directed redistribution.

### 6. Autopoiesis SSI Multiplier

The autopoiesis module provides a `GetMultiplier(ctx, "ssi")` that modulates slash amounts. This allows the system's self-identity subsystem to affect economic security parameters — a governance layer above raw BPS calculations.

---

## Gaps and Gaps Only

### Gap 1: No Tombstoning (Permanent Ban) (★★★★★)

The most critical gap. Validators who accumulate extreme slash counts can still continue operating — there is no mechanism for permanent exclusion from the validator set.

**Why this matters**: In Tendermint-based systems, tombstoning is the last line of defense against a validator who is either permanently compromised or acting adversarially. Without tombstoning, a determined attacker can absorb repeated slashes if the gain exceeds the cost.

**Sketch**: Add `Tombstoned bool` and `TombstonedAt uint64` to `Validator`. In `SlashValidator`, after slash: if `val.SlashCount >= params.TombstoneThreshold`, set `val.Tombstoned = true`. Tombstoned validators are excluded from all verification panel selection and cannot self-unbond until a governance `UntombstoneValidator` proposal passes. The threshold should be governance-configurable (`TombstoneThreshold uint64` in `Params`).

### Gap 2: No Slash Reason Taxonomy (★★★☆☆)

`SlashValidator` takes a `reason string` parameter, but the system has no structured taxonomy of slash types with type-specific penalty rates.

**Why this matters**: Different misbehaviors warrant different penalties. Equivocation (signing conflicting verdicts) is catastrophic; liveness failure (missing a round) is a nuisance. Flat penalties mis-price risk. Cosmos SDK's `x/slashing` uses `MissedBlockCounterBps` and `DoubleSignFraction` as distinct parameters.

**Sketch**: Define a `SlashType` enum: `DOUBLE_VOTE`, `EQUIVOCATION`, `LIVENESS_FAULT`, `COORDINATION_VIOLATION`, `CALIBRATION_FAULT`. Add `SlashRatesByType map[SlashType]uint64` to `Params`. In `SlashValidator`, use `SlashRatesByType[reason]` instead of a single `BaseSlashBps`. This makes penalty pricing transparent and governance-adjustable per violation class.

### Gap 3: No Evidence-Based Slashing (★★★★☆)

Current slashing is **triggered by module logic** (e.g., `CompleteRound` slashes incorrect verifiers) — there is no path for an external party to submit evidence of misbehavior and trigger a slash.

**Why this matters**: In practice, some misbehavior is observable on-chain by third parties (e.g., a verifier committing to hash H but revealing a value that doesn't match). These should be slashable by anyone who submits valid evidence, not only by automated round logic.

**Sketch**: Add `MsgSubmitSlashEvidence` with fields `{validator_addr, evidence_type, evidence_hash, witness_data}`. In the handler, route to a `SlashEvidenceQueue` reviewed at end-block by an evidence keeper. Valid evidence triggers slash and awards the submitter a `EvidenceRewardBps` fraction of the slashed amount. This is the Cosmos SDK `x/evidence` pattern adapted for zerone's misbehavior taxonomy.

### Gap 4: No Slashing Insurance for Delegators (★★★☆☆)

Delegators absorb overflow when operator stake is exhausted — they have no protection against operator misbehavior. In Ethereum's post-merge model, delegated validators are slashed, but delegators can participate in insurance pools.

**Why this matters**: High slash risk discourages delegation, reducing the total bonded stake and weakening economic security.

**Sketch**: Add optional `DelegatorInsuranceFundBps uint64` to `Validator` — operators can commit to contributing N bps of slash recovery to a per-validator insurance pool. In `SlashValidator`, before absorbing overflow into `DelegatedStake`, check the insurance pool balance and apply it first. Operators who maintain insurance funds attract more delegators, creating a market mechanism for self-regulation.

### Gap 5: No Slashing-to-Burn Option (★★★☆☆)

All slashed tokens route to either the development fund or the vindication escrow. There is no option to burn slashed tokens, reducing total supply.

**Why this matters**: If slashed tokens go to the development fund, the development fund accrues value from misbehavior — which creates a perverse incentive if the development fund has governance power over slash parameters. Burning eliminates this conflict.

**Sketch**: Add `SlashBurnFractionBps uint64` to `Params`. In `SlashValidator`, compute `burnAmount = slashedAmount * SlashBurnFractionBps / 1_000_000`. Route `burnAmount` to `bankKeeper.BurnCoins`, remainder to development fund. Default 0 (no change to current behavior). This follows Ethereum's model where slashed ETH is burned.

---

## Convergence Notes (for Synthesis)

- **Vindication escrow** (slashing) and **settlement finality** (Brief 1) are deeply linked — the escrow design assumes reversibility at the fact level but not the round level. The escrow amount represents the economic stake of finality.
- **Tombstoning** (slashing) is the hard floor under **circuit breakers** (Brief 9) — a tombstoned validator should trigger automatic panel reconstitution, not a full halt.
- **Evidence-based slashing** (Gap 3) convergences with **adversarial collaboration** from the truth-seeking series — third-party evidence submission is the economic analog of the adversarial paper format.
- **Slash taxonomy** (Gap 2) feeds into **oracle resilience** (Brief 8) — oracle data providers who submit manipulated data need a distinct `ORACLE_MANIPULATION` slash type.
- **Emission as validator reward** (monetary policy, Brief 2) and **slash as validator punishment** are the two poles of the validator incentive curve. Their ratio determines whether honest validation is economically rational.

---

## Summary Verdict

Zerone's slashing system is **well-designed for its current scope**. Progressive escalation, per-epoch caps with windowed decay, four-tier differentiated security, and slash routing to specific module accounts are all correct mechanisms.

The critical absence is **tombstoning** — without a permanent ban path, the system has no hard ceiling on repeated misbehavior. The second priority is a **slash reason taxonomy** — undifferentiated penalties mis-price risk and don't signal which behaviors are most threatening to the protocol.

Priority recommendation: add `Tombstoned` field to `Validator` proto, add `TombstoneThreshold` to `Params`, and implement tombstone logic in `SlashValidator` after escalation calculation. This is a 20-line change with significant security impact.
