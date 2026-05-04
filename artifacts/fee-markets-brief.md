# Fee Market Design Under Congestion
*Zerone Financial Substrate — Brief 5 of 10*

---

## Methodology

Fee market design solves a resource allocation problem: when demand for a constrained resource (block space, verifier capacity, oracle slots) exceeds supply, how should access be priced? The naive answer is "first come, first served" — the result is mempool spam, extractable value, and quality collapse.

The field advanced significantly with EIP-1559 (Roughgarden 2021): a dynamically-adjusting base fee that rises when blocks are full and falls when empty, with a miner tip on top. The key insight is that a price mechanism must clear the market — it cannot simply be a floor. Roughgarden's formal analysis showed EIP-1559 is incentive-compatible: users cannot profit by manipulating their bid.

For knowledge markets specifically, fee design must navigate the **demand-for-quality vs. demand-for-throughput** tension. More claims means more information produced, but verifier bandwidth is finite. Overloading verifiers degrades verification quality — which undermines the entire epistemic system.

**Key sources**: Roughgarden (2021, Transaction Fee Mechanism Design); Nakamoto (2008, Bitcoin, §6 on incentives); Ethereum EIP-1559 implementation; Cosmos SDK `x/auth` tx fee documentation; Dwork & Naor (1993, proof-of-work as spam resistance); Schelling (1960, focal points); Akerlof (1970, market for lemons — adverse selection in fee markets).

---

## What Zerone Already Has

### 1. Stake-as-Fee: Aligned Incentives at Submission

The core insight of zerone's fee design: the review fee is not separate from the claim stake — **the stake IS the fee**. When a submitter stakes `Claim.Stake`, 55% flows to verifiers, 22% to protocol treasury, 19.67% to development fund, and ~3.33% to research fund. The stake amount controls the review fee directly.

This aligns incentives at the submission layer: submitters who believe in their claim stake more, making the verification reward larger, attracting higher-quality verifiers. Low-stake claims attract less verifier attention — a natural quality filtering mechanism. This is an implicit fee market where the "price" for premium verification is the stake amount.

### 2. Bootstrap Fund: Demand-Side Subsidy

```go
type BootstrapFund parameters:
    BootstrapFundEnabled bool
    BootstrapFundFeeCap string        // max stake eligible for sponsorship
    BootstrapFundMaxPerAddress string // lifetime claims per address
    BootstrapFundMaxPerEpoch string   // rate limit per epoch
```

The bootstrap fund absorbs review fees for qualified submitters, with rate limiting on:
- Per-address lifetime usage (prevents Sybil farming)
- Per-epoch total (prevents fund drainage attacks)
- Fee cap (prevents expensive claims gaming the fund)

This is a well-designed demand-side subsidy. It lowers barriers for legitimate new submitters while structural rate limits prevent exploitation.

### 3. USD-Price-Responsive Dynamic Pricing (`x/billing`)

`DynamicPricingConfig` in the billing module adjusts query costs based on ZRN/USD price:

```
fields: Enabled, TargetQueryCostUsd, ManualZrnPriceUsd (governance override)
        TwapWindowBlocks, StalenessBlocks
        MinCostPerFact, MaxCostPerFact
```

This keeps the **USD cost** of querying the knowledge base roughly stable as ZRN price fluctuates — good UX for data consumers paying in ZRN. The TWAP window and staleness threshold protect against oracle manipulation. Floor and ceiling in uzrn prevent extreme fee excursions.

### 4. Fixed Revenue Split (55/22/19.67/3.33)

The hardcoded split constants in `x/knowledge/keeper/fees.go` create a stable, predictable distribution. Verifiers always know their share (55%) of any round; protocol treasury receives a consistent 22%. Stability matters for agent behavior — unpredictable fee splits create uncertainty about expected returns.

### 5. AMM Swap Fees (`x/liquiditypool`)

Liquidity pools have `SwapFeeBps` (per-pool, set at creation). The AMM fee structure follows Uniswap v2's model: fee deducted from input before the constant-product swap. Swap fees accrue to LP token holders (as pool reserves increase).

---

## Gaps and Gaps Only

### Gap 1: No Congestion-Based Backpressure on Claim Submission (★★★★★)

**The most critical gap.** When many claims are submitted simultaneously, verifiers are spread across more rounds simultaneously, degrading per-round attention and increasing panel formation latency. There is no mechanism that increases the minimum stake required during high-submission periods.

**Why this matters**: Without backpressure, zerone's verification quality degrades gracefully under load — but invisibly. The system accepts claims that it cannot service well. This is the knowledge-market equivalent of a hospital that admits more patients than it can treat.

**Sketch**: Add `ActiveRoundsCount uint64` to the knowledge module state, incremented at `CreateVerificationRound` and decremented at `CompleteRound`. Add `CongestionBaseFeeMultiplierBps uint64` and `CongestionThresholdRounds uint64` to `Params`. In the `MsgSubmitClaim` handler, compute `effectiveMinStake = params.MinReviewFee * (1 + (activeRounds - threshold) * multiplier / 1M)` when `activeRounds > threshold`. This is EIP-1559's base fee adjustment logic applied to claim submission.

### Gap 2: No Priority Fee for Faster Panel Assembly (★★★☆☆)

All claims at the same stake level are treated identically in verifier selection. A submitter who needs rapid verification (time-sensitive fact) has no mechanism to express this preference other than staking more.

**Why this matters**: If staking more increases the verifier reward, high-value submitters can get faster service — but this conflates fee-for-speed with fee-for-quality. An explicit priority tip would separate these signals.

**Sketch**: Add `PriorityTipBps uint64` to `MsgSubmitClaim` — an additional tip (as fraction of stake) paid to verifiers who complete their vote in the first N blocks of the commit phase. In `distributeVerifierRewardsFromPool`, route `PriorityTipPool` to verifiers who voted before `CommitDeadline/2`. This separates speed premium from quality premium.

### Gap 3: Revenue Split Is Not Governance-Adjustable in Knowledge Module (★★★☆☆)

The `reviewFeeContributorBps = 550_000` and related constants are hardcoded in `x/knowledge/keeper/fees.go`. The billing module has a governance-adjustable `RevenueSplit`, but the knowledge module's fee distribution is fixed at compile time.

**Why this matters**: If verifier rewards need adjustment (e.g., to attract more validators during bootstrap), the knowledge module requires a code upgrade rather than a governance vote. This is unnecessary rigidity.

**Sketch**: Move the fee split constants into `x/knowledge/types/Params` (`VerifierPoolBps`, `ProtocolTreasuryBps`, `DevelopmentFundBps`, remainder to research). Read from params in `distributeReviewFee`. Require `sum = 1_000_000`.

### Gap 4: No Fee Burn in the Knowledge Module (★★★☆☆)

The 55/22/19.67/3.33 split redistributes 100% of the fee — nothing burns. This compounds the monetary policy gap (Brief 2): the knowledge module is the highest-volume fee collection point in the protocol, and none of it reduces supply.

**Why this matters**: EIP-1559's burn is most effective when it targets the highest-frequency transaction type (Ethereum base gas). For zerone, claim submission fees are analogous — high volume, predictable, the correct burn point.

**Sketch**: Add `BurnBps uint64` to knowledge `Params`. In `distributeReviewFee`, compute `burnAmt = fee * BurnBps / 1M` before the split. Apply `bankKeeper.BurnCoins`. Reduce the residual distributed to research fund proportionally.

### Gap 5: No Verifier Capacity Forecasting (★★★☆☆)

Submitters have no way to know how loaded the verifier network is before submitting. They cannot estimate wait time or expected panel quality.

**Why this matters**: Information asymmetry in fee markets leads to adverse selection (Akerlof's market for lemons applied to verification). Submitters who know the system is overloaded will either over-stake or defer submission; those who don't know will submit at market rate and receive degraded service.

**Sketch**: Add a `QueryNetworkCapacity` endpoint that returns `ActiveRoundsCount`, `TotalActiveVerifiers`, `EstimatedPanelFormationBlocks`. Update every N blocks. This is a transparency tool, not a control — but transparency is a prerequisite for well-functioning fee markets.

---

## Convergence Notes (for Synthesis)

- **Congestion-based minimum stake** (Gap 1) is the direct application of EIP-1559 logic to zerone's knowledge market — it connects the fee market design to **monetary policy** (Brief 2) via the burn path, and to **settlement finality** (Brief 1) via round duration.
- **Revenue split governance** (Gap 3) should be unified with the **billing module's RevenueSplit** governance — two revenue splits for the same protocol creates confusion and divergence risk.
- **Verifier capacity as a throttle** feeds directly into **circuit breaker design** (Brief 9) — congestion metrics are the leading indicators that circuit breakers should monitor.
- **Bootstrap fund** (already implemented) is zerone's answer to the demand-side subsidy problem — it prevents fee markets from excluding legitimate small submitters during their early use of the system.
- The **AMM swap fee** (already implemented) is a separate fee market for the liquidity layer — it should be analyzed in **liquidity infrastructure** (Brief 7).

---

## Summary Verdict

Zerone's fee market is **well-designed for normal operating conditions** — stake-as-fee alignment, bootstrap fund with rate limits, USD-responsive dynamic pricing, and stable revenue splits create a coherent baseline. The system self-prices quality: higher stakes attract better verifiers.

The critical gap is **no congestion-based backpressure**. Under heavy load, the protocol accepts all submissions at standard rates — which degrades verification quality as verifier bandwidth thins. EIP-1559's base fee adjustment is the correct mechanism: auto-increase minimum stake as active rounds grow beyond the network's capacity to service them well.

Priority recommendation: add `ActiveRoundsCount` to knowledge state, implement `CongestionMultiplierBps` in `Params`, and apply it to `MinReviewFee` computation in `MsgSubmitClaim`. This is ~30 lines in the keeper and a critical defense of verification quality under load.
