# Oracle Resilience and External Truth Integrity
*Zerone Financial Substrate — Brief 8 of 10*

---

## Methodology

Oracles solve the blockchain oracle problem: smart contracts are deterministic closed systems, but useful applications need data from the outside world — prices, weather, election results, real-world events. The oracle brings that data on-chain.

Zerone has a second oracle problem that is less common: not just "what is the ZRN price?" but "what is true?" — the knowledge oracle that evaluates factual claims using LLM inference. Both oracle types share the same security model: **a trusted off-chain entity makes a claim, and the chain must decide how much to trust it**.

The academic tradition:
- **Oracle manipulation attacks** (Zhao et al. 2021, Flash Boys 2.0 oracle section): TWAP manipulation via flash loans; price oracle dependencies in lending protocols (the Compound $90M exploit)
- **Decentralized oracle networks** (Chainlink whitepaper 2017; Band Protocol 2019): Byzantine fault tolerance via aggregation across multiple independent oracle operators
- **Commit-reveal for oracle data** (Kleros, Augur): oracles that use verifier-style commit-reveal to prevent coordinated manipulation
- **Oracle freshness**: stale data causes cascading failures; staleness bounds need to be enforced at consumption

**Key sources**: Breidenbach et al. (2017, Chainlink); Adams et al. (2021, Uniswap v3 oracle module); Zhao et al. (2021, Flash Boys 2.0); Angeris & Chitra (2020, TWAP oracle theory); Kleros whitepaper; Nakamoto oracle design principles.

---

## What Zerone Already Has

### 1. On-Chain TWAP Oracle (`x/liquiditypool/keeper/twap.go`)

Block-weighted TWAP with 1e12 precision, updated every block via `BeginBlock`. `GetTWAP(ctx, poolId, denom, windowBlocks)` returns a manipulation-resistant price:
- Short window: more responsive but more manipulable
- Long window: more resistant but slower to track genuine price changes

`DynamicPricingConfig` (billing) uses `TwapWindowBlocks` and `StalenessBlocks` to gate price consumption: if the TWAP accumulator hasn't been updated within `StalenessBlocks`, the price is considered stale.

### 2. Manual Override Governance Safety Valve

`DynamicPricingConfig.ManualZrnPriceUsd` — governance can override the TWAP price with a fixed USD price. Used for emergency scenarios when the TWAP is corrupted or during bootstrap before the pool has sufficient history.

This is the correct failsafe: an emergency governance lever that bypasses the oracle when the oracle is compromised.

### 3. Oracle Unavailability Graceful Fallback

In `x/toolbox/keeper/usd_pricing.go`, when `GetZRNPriceUSD` fails:
1. Emit `tool_pricing_oracle_unavailable` event
2. Fall back to `tool.PricePerCall` (fixed uzrn price)
3. Return `PricingModeFixedUzrn`

The system continues functioning at fixed prices rather than halting. The fallback is observable (event) and auditable.

### 4. Knowledge Oracle (cmd/oracle/)

HTTP service with `/evaluate` endpoint accepting `EvaluateRequest` and returning `{verdict, confidence, reasoning}`. Three evaluator tiers:
- `CombinedEvaluator` — LLM + static fusion
- `evaluator_llm.go` — LLM-based evaluation
- `evaluator_static.go` — rule-based static evaluation

The knowledge oracle is zerone's most unique oracle design: it's not a price feed but a **truth evaluator** — it translates natural language claims into binary verdicts with calibrated confidence scores.

### 5. Oracle Category in Validator System

`CategoryOracleFeed` is a recognized validator category with `0.8x` multiplier (less trusted than core protocol validators). Oracle feed providers are first-class participants in the staking tier system — they have skin in the game.

---

## Gaps and Gaps Only

### Gap 1: TWAP Oracle Has Circular Price Dependency (★★★★★)

**The structural vulnerability.** ZRN's price oracle is the TWAP from a ZRN AMM pool. The price of ZRN is derived from a ZRN-denominated pool. This creates a **circular dependency**: anyone who wants to manipulate the ZRN price oracle can do so by manipulating the ZRN AMM pool.

Flash loan attack vector:
1. Borrow large ZRN position
2. Trade into the ZRN/X pool, moving the spot price significantly
3. Wait for downstream contracts that use the TWAP to act on the manipulated price (e.g., minting tools at reduced cost)
4. Reverse the trade, repay the loan
5. Net profit: the extracted value from downstream TWAP consumers

**Why this matters**: Every protocol that relies on its own AMM TWAP for pricing has this vulnerability. The 2022 Mango Markets exploit ($114M) used exactly this attack pattern.

**Sketch**: Add support for an external price feed alongside the TWAP. Use a weighted median of three sources: (1) TWAP, (2) external Chainlink/Pyth price, (3) governance manual override. The weighted median is robust: manipulating any single source cannot shift the result if the other two sources are unaffected. Add `ExternalPriceFeedEnabled bool` and `ExternalPriceFeedAddress string` to `DynamicPricingConfig`.

### Gap 2: Single Knowledge Oracle Instance (★★★★★)

The knowledge oracle (`cmd/oracle/`) is a single HTTP service. If it's down, slow, or compromised, the verification system has no fallback.

**Why this matters**: Verification rounds have hard `RevealDeadline` and `AggregationDeadline` blocks. If verifiers cannot reach the oracle to get evaluation guidance, rounds time out inconclusively. A compromised oracle that returns adversarial verdicts corrupts the entire knowledge base.

**Sketch**: The evaluator pattern already supports multiple tiers (LLM + static). Extend to a **committee of oracle operators**: require N-of-M oracle operators to independently submit evaluations, aggregate by weighted median confidence. Each oracle operator registers on-chain with a stake and a URI. Incorrect evaluations (as measured by the final panel verdict) slash the oracle operator's stake. This is the Chainlink model: decentralize trust across independent oracle nodes, slash for misbehavior.

### Gap 3: No TWAP Circuit Breaker at Oracle Consumption Points (★★★★☆)

The TWAP's `StalenessBlocks` parameter in `DynamicPricingConfig` controls when a stale price triggers fallback in the billing module. But other TWAP consumers (future modules using `GetTWAP`) have no systematic staleness check.

**Why this matters**: Each module consuming the TWAP must implement its own staleness check independently — which means it will likely be forgotten. A single centralized `GetTWAPWithStalenessCheck` function that reverts or returns an error if `currentBlock - lastUpdateBlock > maxStalenessBlocks` would prevent all consumers from silently reading stale prices.

**Sketch**: Replace `GetTWAP` with `GetTWAPSafe(ctx, poolId, denom, windowBlocks, maxStalenessBlocks) (*big.Int, error)`. If the accumulator's `LastBlock < ctx.BlockHeight() - maxStalenessBlocks`, return `ErrTWAPStale`. All consumers call `GetTWAPSafe` with their staleness tolerance; the oracle subsystem handles the check centrally.

### Gap 4: No Oracle Data Freshness Enforcement On-Chain for Knowledge Oracle (★★★★☆)

The knowledge oracle (`cmd/oracle/`) evaluates claims and returns verdicts. But there is no on-chain mechanism that verifies the oracle's response is fresh (based on current chain state) rather than a replayed response from a previous evaluation.

**Why this matters**: A compromised or caching oracle could replay old verdicts for new claims, especially if claims are similar. On a probabilistic basis, a stale verdict is often wrong because context (related facts, prior decisions) changes.

**Sketch**: In `MsgRevealVerification`, add an `OracleRequestId string` field — a commitment to the specific oracle call the verifier made. The oracle returns a `request_id = sha256(claim_id + block_hash + verifier_addr)` as part of its response. Verifiers include this `request_id` in their reveal. The chain verifies that `request_id` encodes the current block hash (or one within the last N blocks), proving the oracle was queried recently.

### Gap 5: TWAP Manipulation via Max Swap Size (★★★☆☆)

A large swap in a single block moves the spot price and contributes to the TWAP accumulator for that block. If the block is N blocks long and the manipulation swap is worth `N × AmountPerBlock`, the entire TWAP window can be shifted.

**Why this matters**: This is the block-level version of the flash loan attack — no borrowed capital needed, just a large trade. The longer the TWAP window, the more capital required to manipulate it, but the attack is always theoretically possible if the attacker has enough capital.

**Sketch**: Implement the `MaxSwapFractionBps` pool parameter (also identified in Brief 6). At 10% maximum, a single swap can move the price by at most ~11% (from constant product), bounding the per-block TWAP manipulation. Combine with a `TWAPDeviation CircuitBreaker` that halts pool trading if the TWAP deviates from the previous N-block average by more than `MaxTWAPDeviationBps` — this is the same circuit breaker logic from Brief 9.

---

## Convergence Notes (for Synthesis)

- **Circular price dependency** (Gap 1) means the **liquidity infrastructure** (Brief 7) is itself the oracle security system — pool depth directly determines manipulation cost. Deeper pools are more expensive to manipulate.
- **Knowledge oracle committee** (Gap 2) is structurally identical to the **adversarial collaboration** model from the truth-seeking series: multiple independent evaluators, aggregated verdict. The financial and epistemic oracle patterns converge here.
- **Oracle staleness enforcement** (Gap 3) is a prerequisite for **circuit breaker** design (Brief 9) — circuit breakers that monitor oracle health need a standardized staleness interface.
- **TWAP as oracle** connects to **settlement finality** (Brief 1) via block hash anchoring — if verdicts are bound to block hashes, then oracle data consumed at verdict time is also hash-bound.
- `GetZRNPriceUSD` feeds into **fee market** (Brief 5) via `DynamicPricingConfig` — oracle integrity directly determines whether USD-stable fees are actually USD-stable.

---

## Summary Verdict

Zerone has a functional internal oracle (TWAP with staleness guard and manual override) and a specialized knowledge oracle (LLM evaluator with graceful fallback). The governance override and fallback to fixed pricing are correct failsafes.

The critical structural gap is the **circular price dependency**: zerone's price oracle reads ZRN price from ZRN's own AMM. This is manipulable in proportion to pool liquidity — shallow pools are cheap to manipulate. The defense requires either an external price feed or dramatically deeper pool liquidity.

The second critical gap is the **single knowledge oracle instance** — a single point of failure for the protocol's core function (fact evaluation). The oracle committee model (N-of-M aggregation with slash-on-fault) is directly applicable and consistent with zerone's validator design philosophy.

Priority recommendation: implement `GetTWAPSafe` as the standardized staleness-aware TWAP query, add `MaxSwapFractionBps` to pool parameters to bound manipulation, and begin the oracle committee protocol design. These together address the immediate TWAP manipulation surface while providing the foundation for decentralized oracle resilience.
