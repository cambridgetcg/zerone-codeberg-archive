# Monetary Policy and Supply Stability for ZRN
*Zerone Financial Substrate — Brief 2 of 10*

---

## Methodology

Monetary policy governs how a currency's supply expands or contracts over time, and what anchors its value. For protocol tokens, this resolves into three linked problems:

1. **Emission schedule design** — how new tokens enter circulation, and on what timetable
2. **Stability mechanisms** — what prevents hyperinflation, death spiral, or supply shock
3. **Sink-source balance** — are there systematic burn paths that offset emission?

The academic tradition draws on Friedman's monetarism (stable money growth rules), Hayek's denationalisation of money (competitive currency selection), and modern protocol token economics (Buterin's token model, Ethereum's EIP-1559 deflationary mechanism). Cosmos-native projects add the complication of staking dilution: validators earn emission-funded rewards, which creates inflation for non-stakers and incentivizes bonding.

**Key sources**: Friedman (1968, role of monetary policy); Hayek (1976, denationalisation of money); EIP-1559 (Roughgarden 2021, transaction fee mechanism design); Cosmos SDK inflation module; Ethereum economic model (Ethereum Foundation, 2022); Buterin token models; SushiSwap tokenomics post-mortems.

---

## What Zerone Already Has

### 1. Block-Linear Emission via `EmissionPeriod` (`x/tokens`)

`EmissionPeriod` is the core monetary policy primitive:

```go
type EmissionPeriod struct {
    Id             string  // deterministic: SHA256(creator:startBlock:endBlock)
    StartBlock     uint64
    EndBlock       uint64
    AmountPerBlock string  // uzrn per block, bigint-as-string
    Recipient      string  // bech32 address receiving emission
    Active         bool
    TotalEmitted   string  // running total, tracked in BeginBlocker
    TokenId        string  // if empty, mints native uzrn
}
```

`BeginBlocker` in `x/tokens/module.go` iterates all active emission periods each block, minting `AmountPerBlock` uzrn directly to the `Recipient` address via the Cosmos bank module's `MintCoins` → `SendCoinsFromModuleToAccount` path.

**Characteristics of this design:**
- **Block-linear**: emission is constant per block within a period, regardless of price or demand
- **Governance-gated**: `CreateEmissionPeriod` requires `authority` (governance module address)
- **Max supply cap**: `TokenDefinition.MaxSupply` is enforced at mint time for ZRN-20 secondary tokens; ZRN itself (`uzrn`) has no on-chain hard cap — the governance process is the soft cap
- **Multiple concurrent periods**: the iterator allows overlapping periods to co-exist, enabling complex emission schedules (staking rewards + research treasury + validator incentives simultaneously)
- **Active flag**: periods deactivate automatically at `EndBlock`

### 2. ZRN-20 Mintable/Burnable Secondary Tokens (`x/tokens`)

Secondary tokens (non-native) support:
- `TokenFeatures.Mintable` — creator or governance can `MintToken`
- `TokenFeatures.Burnable` — holders can `BurnToken`
- `TokenFeatures.Pausable` — trading can be halted
- `TokenFeatures.Wrappable` — native coins can be wrapped into ZRN-20

`MaxSupply = "0"` means unlimited. Any non-zero value is a hard cap enforced at `MintToken` time.

### 3. Wrapped Token Burn-on-Unwrap

`WrapToken` / `UnwrapToken` handlers: wrapping mints ZRN-20 equivalent against locked native coins; unwrapping burns the ZRN-20 and releases the native. This is a **neutral supply operation** — total economic value is conserved across the wrap/unwrap cycle.

### 4. Governance-Controlled Authority

All emission creation and token minting flows through `authority string` — the governance module address. This means monetary policy changes require on-chain governance votes, providing democratic legitimacy and resistance to unilateral supply manipulation.

---

## Gaps and Gaps Only

### Gap 1: No ZRN Burn Path on Main Protocol Operations (★★★★★)

**The most critical monetary policy gap.** Every protocol operation — claim submission, verification round, dispute — results in fee collection, but there is no systematic burn of any portion of those fees. Emission is inflationary; fee collection currently redistributes (to verifiers, to development fund, to research treasury) but does not reduce supply.

**Why this matters**: Sustainable token economics requires a sink to offset the source. EIP-1559's base fee burn is the canonical example: users pay gas, a portion burns, net supply pressure balances emission. Without a burn path, ZRN is perpetually inflationary, which creates long-term sell pressure for anyone receiving protocol rewards.

**Sketch**: Introduce `BurnBps uint64` on `Params` (e.g., 50_000 = 5%). In `distributeVerifierRewardsFromPool` (rounds.go), compute `burnAmount = feePool * BurnBps / 1_000_000` before distributing the remainder. Route `burnAmount` through `bankKeeper.BurnCoins`. This converts ZRN from a pure inflationary protocol token toward a fee-sink model.

### Gap 2: No Adaptive Emission Rate (★★★☆☆)

`AmountPerBlock` is fixed for the life of an `EmissionPeriod`. There is no feedback mechanism that adjusts emission based on bonded stake ratio, network utilization, or price oracle signals.

**Why this matters**: Cosmos native inflation (the SDK `x/mint` module) typically targets a bonded ratio — if staking fraction is low, inflation rises to incentivize bonding; if high, it falls. ZRN's fixed-schedule approach is simpler but less responsive to network conditions.

**Sketch**: Add an optional `TargetBondedRatioBps` field to `Params`. In `BeginBlocker`, after each epoch, query total bonded stake via `stakingKeeper.TotalBondedTokens()` and adjust `AmountPerBlock` up/down by a configurable `EmissionAdjustmentBps` within a `[MinEmissionPerBlock, MaxEmissionPerBlock]` band. This is the Cosmos SDK `x/mint` model adapted to zerone's EmissionPeriod system.

### Gap 3: No Emission Schedule with Halving (★★★☆☆)

Unlike Bitcoin's halving model or Ethereum's planned emission reduction, zerone's EmissionPeriods must be manually rotated by governance. There is no built-in schedule reduction (e.g., halve `AmountPerBlock` every N blocks).

**Why this matters**: Bitcoin's deterministic supply schedule provides strong credible commitment to scarcity. Manual governance rotations introduce uncertainty and require active governance participation to maintain the emission schedule.

**Sketch**: Add `HalvingIntervalBlocks uint64` and `HalvingFactor uint32` (as a fraction of 100) to `EmissionPeriod`. In `BeginBlocker`, at each halving interval, multiply `AmountPerBlock` by `HalvingFactor / 100`. Store updated `AmountPerBlock` back to the period. This makes the long-run supply schedule deterministic without governance intervention.

### Gap 4: No Hard ZRN Supply Cap in Protocol (★★★☆☆)

`uzrn` (native ZRN) has no `MaxSupply` field enforced at the protocol layer. Governance can create unlimited `EmissionPeriod` schedules. The only constraint is governance itself.

**Why this matters**: Hard caps are credible commitment devices. ZRN-20 secondary tokens have `MaxSupply` enforced at mint time — native ZRN does not, creating an asymmetry in supply guarantees.

**Sketch**: Add `MaxNativeSupply string` to the `tokens` module `Params`. In `BeginBlocker`, check `bankKeeper.TotalSupply("uzrn") + amountPerBlock <= maxNativeSupply` before minting. If the cap would be breached, skip the emission silently and deactivate the period. This encodes scarcity as a protocol invariant, not just a governance norm.

### Gap 5: No Multi-Recipient Emission Split (★★☆☆☆)

`EmissionPeriod.Recipient` is a single address. Routing emission to multiple recipients (e.g., 60% stakers, 30% research treasury, 10% development fund) requires multiple overlapping `EmissionPeriod` records at the correct ratios.

**Why this matters**: Managing split emission via overlapping periods is fragile — if one period expires and another doesn't, the ratio silently breaks. A first-class split structure would make multi-destination emission legible and atomic.

**Sketch**: Replace `Recipient string` with `Recipients []EmissionSplit` where `EmissionSplit = {address: string, bps: uint64}`. Require `sum(bps) == 1_000_000`. In `BeginBlocker`, iterate recipients and distribute proportional shares from `AmountPerBlock`.

---

## Convergence Notes (for Synthesis)

- **Fee burn path** (monetary policy) is the complement of **fee market design** (Brief 5) — EIP-1559's burn is only effective when combined with a congestion-responsive base fee. The two must be designed together.
- **Emission periods** are the funding mechanism for **treasury and reserve management** (Brief 4) — the research treasury's funding source is ultimately an emission split.
- **Bonded ratio targeting** (Gap 2) directly links monetary policy to **slashing and economic security** (Brief 3) — the incentive to bond is what creates the economic security budget.
- A **ZRN hard supply cap** (Gap 4) would create deflationary pressure over time, which interacts with **liquidity infrastructure** (Brief 7) — declining supply affects pool depth and LP economics.

---

## Summary Verdict

ZRN's monetary policy is **governance-gated block-linear emission** — clear, auditable, simple. The `EmissionPeriod` primitive is well-designed: governance-controlled, multi-period capable, block-precise.

The critical absence is a **burn path**. Every protocol token that achieves long-term value equilibrium does so because demand (usage fees burned) can offset supply (emission). Without a burn mechanism, ZRN's inflation is one-directional. The EIP-1559 pattern — burn the base fee, pay validators the tip — is directly applicable to zerone's claim submission fee structure.

Priority recommendation: introduce `BurnBps` on `Params`, implement fee-burn in `distributeVerifierRewardsFromPool`, and add `MaxNativeSupply` to `tokens/Params`. These three changes establish the sink-source balance that sustainable token economics requires.
