# Treasury and Reserve Management
*Zerone Financial Substrate — Brief 4 of 10*

---

## Methodology

Treasury management in protocol design addresses three connected problems:

1. **Accumulation** — how does the protocol generate surplus, and where does it flow?
2. **Governance** — who controls the surplus, and with what procedural constraints?
3. **Reserve adequacy** — how much liquidity must be on hand to meet future obligations?

The academic tradition draws from public finance theory (Musgrave's functions of fiscal policy — allocation, distribution, stabilization), corporate treasury management, and protocol-native examples: MakerDAO's Surplus Buffer, Compound's Community Treasury, Ethereum's Protocol Guild, Cosmos Hub's Community Pool. The recurring pattern is: protocols that accumulate surplus without governance controls either capture value for insiders or fail to deploy it when needed.

**Key sources**: Musgrave (1959, Theory of Public Finance); Jensen & Meckling (1976, agency costs); MakerDAO governance documentation (2021); Compound treasury design; Cosmos SDK `x/distribution` documentation; Klages-Mundt & Minca (2020, stablecoin reserve design); Thorchain treasury lessons.

---

## What Zerone Already Has

### 1. Multi-Destination Revenue Distribution via Billing (`x/billing/keeper/distribution.go`)

`CalculateDistribution` is the central revenue routing function. Every payment through the billing system is split across four destinations using governance-adjustable `RevenueSplit`:

```
Revenue = ContributorBps    → data providers (direct transfer)
        + ProtocolBps       → protocol allocation (split further below)
        + ResearchBps       → research_fund module account
        + Burn (remainder)  → rounding dust (not a real deflationary mechanism)
```

The `ProtocolBps` sub-split:
```
ProtocolAllocation = 50% → knowledge module (citation rewards, weighted by confidence × log2(citations+1))
                   + 30% → knowledge module (verification pool)
                   + 20% → treasury_protocol module account
```

Citation rewards use the formula `weight = confidence × floor(log2(citations+1) + 1)` — facts cited more often (evidence of quality) and with higher confidence earn more from the knowledge pool. This is a well-designed incentive.

### 2. Named Module Accounts as Fund Silos

Zerone uses Cosmos SDK module accounts as named fund silos:

| Module Account | Source | Purpose |
|----------------|--------|---------|
| `treasury_protocol` | 20% of protocol billing share | Governance, operations |
| `research_fund` | `ResearchBps` of billing | Protocol research |
| `development_fund` | Slashes, withheld rewards, failed bonds, vindication remainders | Development, bug bounties |
| `knowledge` | 50+30% of protocol billing | Citation rewards + verification pool |
| `vindication_escrow` | Slashed tokens (verifier mistakes, when fact accepted) | Conditional refund |

This gives a clear audit trail — balances are queryable on-chain, movements emit events.

### 3. Governance-Adjustable Revenue Split

`RevenueSplit` is part of the billing module `Params`, which means it's adjustable through governance proposals. The split (ContributorBps, ProtocolBps, ResearchBps) can be changed by governance vote without a protocol upgrade. This is the correct design — revenue allocation policy should be democratically adjustable.

### 4. Knowledge Pool Citation Economics

The knowledge pool uses `confidence × log2(citations+1)` as the weight function. This is a meaningful design choice: the log factor captures diminishing marginal returns from citation count (each additional citation is worth less), while confidence captures the epistemic quality of the underlying fact. Facts that are both highly confident and frequently cited earn the most — a reasonable proxy for knowledge quality.

### 5. Hard Supply Cap Constant

`MaxSupplyUzrn = "222,222,222,000,000"` uzrn (222.2M ZRN) is defined in `vesting_rewards/types/keys.go`. While this is a constant rather than an enforced protocol rule, it establishes a normative commitment to maximum supply.

---

## Gaps and Gaps Only

### Gap 1: `MaxSupplyUzrn` Is a Constant, Not an Enforced Protocol Rule (★★★★★)

The hard cap of 222,222,222 ZRN exists as a Go constant in `vesting_rewards/types/keys.go` — but it is not checked at mint time in `x/tokens`. A governance proposal creating an `EmissionPeriod` that would breach the cap would not be prevented by the protocol.

**Why this matters**: A hard cap that isn't enforced is not a cap — it's an aspiration. Credible commitment to scarcity requires enforcement at the protocol layer. Bitcoin's 21M cap is enforced in every node's block validation logic.

**Sketch**: Move `MaxSupplyUzrn` to `x/tokens/Params.MaxNativeSupply`. In `BeginBlocker`, before minting via `EmissionPeriod`, check `bankKeeper.GetSupply("uzrn").Amount + amountPerBlock <= params.MaxNativeSupply`. Enforce at genesis, not just at runtime.

### Gap 2: No Spending Controls on `development_fund` (★★★★☆)

The `development_fund` accumulates slashed tokens, withheld independence penalties, and vindication remainders — but there are no governance controls on how it is spent. Any module with access to `bankKeeper.SendCoinsFromModuleToModule` can route from it.

**Why this matters**: The development fund is a significant accumulation point. Slashes from incorrect verifiers, failed challenge bonds, and conformity penalties all flow here. Without spending controls, it's a governance capture risk — whoever controls module message routing controls the fund.

**Sketch**: Require all `development_fund` disbursements to go through a `MsgDevelopmentFundSpend` governance proposal with a `spending_limit_per_block` rate limit (to prevent draining the fund in a single block). Add `DevelopmentFundSpendingLimitBps uint64` to params. This follows the Cosmos `x/distribution` community pool governance pattern.

### Gap 3: `ProtocolBurn` Is Rounding Dust, Not a Deflationary Mechanism (★★★★☆)

`ProtocolBurn` in `PaymentDistribution` captures the rounding remainder after the top-level split. The comment confirms: "Burn = remainder (absorbs rounding dust)." This is not a deflationary mechanism — it's an accounting artifact.

**Why this matters**: This creates a false impression that the billing system has a burn mechanism. The monetary policy analysis (Brief 2) is correct that there is no structural burn path. Naming this field "ProtocolBurn" may obscure the distinction when reviewing the codebase.

**Sketch**: Add a proper `BurnBps uint64` to `RevenueSplit` in billing `Params`. Compute `burnAmount = totalPayment * BurnBps / 1_000_000` as a first-class top-level allocation before the remainder calculation. Route to `bankKeeper.BurnCoins`. Rename the remainder field to `RoundingDust` to clarify its nature.

### Gap 4: No Reserve Ratio Governance (★★★☆☆)

There is no minimum reserve requirement — no rule that says "the protocol treasury must hold at least X uzrn liquid before any governance spending can occur." During a crisis (price crash, oracle failure), the treasury might be empty when it's most needed.

**Why this matters**: MakerDAO's Surplus Buffer exists precisely for this reason: a capital buffer that absorbs losses before governance needs to respond. Without a minimum reserve, the protocol is structurally procyclical — it depletes its reserves exactly when it needs them most.

**Sketch**: Add `MinProtocolReserveBps uint64` to billing `Params` — expressed as a fraction of current total supply. In the `MsgDevelopmentFundSpend` governance handler, enforce that `treasury_protocol.balance / totalSupply >= MinProtocolReserveBps / 1_000_000` before any disbursement. Set default to 10_000 (1%).

### Gap 5: No Unified Reserve Dashboard (★★★☆☆)

The fund silos (`treasury_protocol`, `research_fund`, `development_fund`, `vindication_escrow`, `knowledge`) are queryable individually but there is no aggregated reserve health query. Understanding the protocol's financial position requires five separate queries.

**Why this matters**: Governance participants making decisions about spending or emission should be able to see the full protocol financial picture in one query. Fragmentation discourages informed governance.

**Sketch**: Add a `QueryProtocolReserves` endpoint in billing or a new `x/reserves` module that aggregates all protocol fund balances plus total supply, computing utilization ratios. Emit a `ProtocolFinancialHealth` event every N blocks with aggregated reserve data.

### Gap 6: Siloed Research Treasury vs. Research Fund (★★☆☆☆)

`x/research` maintains its own `TreasuryBalance` separate from the `research_fund` module account. These two accounting systems can diverge — the module account holds actual coins, but the x/research keeper has its own separate balance tracker.

**Why this matters**: Dual accounting for the same resource is a reconciliation risk. If the two diverge (due to a bug or direct bank send that bypasses the research keeper), the research module will show an incorrect balance.

**Sketch**: Consolidate — either x/research reads its balance directly from `bankKeeper.GetBalance("research_fund")` (no separate tracker needed), or the `research_fund` module is retired and x/research's keeper balance becomes the canonical source with direct bank integration.

---

## Convergence Notes (for Synthesis)

- **`MaxSupplyUzrn` enforcement** (treasury, Gap 1) should be the same enforcement point as `MaxNativeSupply` in monetary policy (Brief 2) — they're the same gap seen from two modules.
- **`development_fund` governance controls** (treasury, Gap 2) connect with **slashing routing** (Brief 3) — slashed tokens should follow the same spending governance path as other protocol funds.
- **Reserve ratio requirements** (treasury, Gap 4) are the prerequisite for **circuit breaker design** (Brief 9) — circuit breakers need to know if the protocol has sufficient reserves to execute emergency buybacks or validator compensation.
- **Knowledge pool citation economics** (`confidence × log2(citations+1)`) from treasury convergences with the **calibrated forecasting** series — citation weight is a proxy for epistemic quality, but it should eventually be supplemented with calibration scores (Brier scores from Brief 3 of the truth-seeking series).
- **Billing `RevenueSplit`** is the financial expression of the **methodology system's economic weighting** — CrossMethodDiscountBps and RevenueSplit are two faces of the same commitment: zerone encodes epistemic values as economic incentives.

---

## Summary Verdict

Zerone's treasury architecture is **more complete than it appears at first glance**. The billing distribution system, named fund silos, governance-adjustable revenue split, and citation economics are all present and coherent. The 20% protocol treasury allocation, 50% citation pool, and 30% verification pool constitute a working revenue distribution model.

The critical gaps are: **`MaxSupplyUzrn` not enforced at protocol layer** (the supply cap is normative, not mechanical), and **no spending controls on `development_fund`** (the largest sink for punitive flows lacks governance gates). The `ProtocolBurn` naming is misleading and should be corrected to prevent false assumptions about deflationary mechanics.

Priority recommendation: enforce `MaxSupplyUzrn` in the tokens `BeginBlocker`, add governance gates to `development_fund` spending, and rename `ProtocolBurn` → `RoundingDust` with a proper `BurnBps` parameter for intentional burning.
