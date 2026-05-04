# Market Microstructure and Fair Sequencing
*Zerone Financial Substrate — Brief 6 of 10*

---

## Methodology

Market microstructure studies how markets translate latent supply and demand into executed prices. The field was formalized by Kyle (1985) and Glosten-Milgrom (1985): market makers face adverse selection from informed traders, which is why spreads exist — they're the cost of not knowing whether the counterparty knows more than you.

In decentralized AMMs, the microstructure problem is different: there are no market makers, but there are **validators** who order transactions within a block. This creates **MEV** (maximal extractable value): value that can be captured by observing the mempool and inserting, removing, or reordering transactions. The canonical attack is the sandwich: front-run a large swap by buying the same token, then back-run by selling after the swap moves the price.

The academic and engineering tradition on this:
- **Flashbots** (Daian et al. 2019, Flash Boys 2.0): MEV taxonomy and mitigation
- **SUAVE** (Buterin 2022): fair ordering via sealed-bid transaction auctions
- **Chainlink FSS** (Fair Sequencing Services): external ordering guarantees
- **Commit-reveal schemes**: the canonical defense against front-running, adapted from zero-knowledge protocols
- **TWAP resistance**: Uniswap v3's lessons on oracle manipulation via block-level price stuffing

**Key sources**: Kyle (1985, Continuous Auctions and Insider Trading); Daian et al. (2019, Flash Boys 2.0); Buterin (2022, Notes on SUAVE); Uniswap v2/v3 whitepapers; Adams et al. (2021, Uniswap v3 core); Park (2021, What's in a Spread?).

---

## What Zerone Already Has

### 1. Constant-Product AMM with Slippage Protection (`x/liquiditypool`)

`MsgSwap.MinTokenOut` — a user-specified minimum output. If the actual output falls below `MinTokenOut`, the swap reverts:

```go
if minTokenOut.Sign() > 0 && tokenOut.Cmp(minTokenOut) < 0 {
    return nil, types.ErrSlippageExceeded
}
```

This is the **primary anti-sandwich defense**: users pre-commit to a slippage tolerance. A sandwich attack that moves the price more than the tolerance causes the victim's transaction to revert, making the attack profitless.

### 2. Price Impact Computation (`CalculatePriceImpactBps`)

The AMM exposes `CalculatePriceImpactBps` — the fraction of ideal output lost due to pool depth. This enables front-ends to warn users of high-slippage swaps before submission.

### 3. TWAP Accumulator (`x/liquiditypool/keeper/twap.go`)

Block-weighted TWAP with 1e12 precision scale. Accumulator updated at each block via `BeginBlock`. `GetTWAP` computes average over a configurable window with fallback to spot price. TWAP smooths out short-term price manipulation within a single block — a validator cannot move the TWAP significantly in one block.

### 4. Pool Locked Flag

`Pool.Locked bool` — pools can be halted (set in emergency or governance scenarios). When locked, all swaps revert with `ErrPoolLocked`. This is a coarse circuit breaker at the pool level.

### 5. Commit-Reveal Anti-Gaming in Verification Rounds

The knowledge module's commit-reveal scheme (Brief 1, settlement finality) is the same structural defense that AMMs need but don't have: verifiers commit to a hash, then reveal. Applied to swaps, this would be: users commit to a swap hash, then reveal the parameters — breaking the front-running window.

---

## Gaps and Gaps Only

### Gap 1: No Commit-Reveal for Swap Execution (★★★★★)

**The critical gap.** `MsgSwap` parameters (pool, amount, direction) are visible in the mempool before inclusion. A validator observing the mempool can insert their own swap before the victim's swap, and a reverse swap after, capturing the price impact.

`MinTokenOut` mitigates sandwich attacks but does not prevent them — it only ensures the victim doesn't execute at an extreme price. The validator still profits from the front-running leg even if the sandwich's second leg fails.

**Why this matters**: In Cosmos SDK, validators do control block contents. The Cosmos mempool provides no ordering guarantees within a block.

**Sketch**: Add `MsgCommitSwap { sender, pool_id, commitment: sha256(random_nonce + pool_id + amount + direction + min_out) }` and `MsgRevealSwap { sender, pool_id, nonce, amount, direction, min_out }`. A commit must precede its reveal by at least `CommitRevealDelayBlocks` (e.g., 2 blocks). In `MsgRevealSwap`, verify `sha256(nonce + params) == stored commitment` then execute. This breaks the front-running window: by the time the reveal is visible in the mempool, the commit is already included in a block.

This reuses exactly the same pattern as the knowledge module's verification rounds — zerone already has the commit-reveal machinery in its DNA.

### Gap 2: No Transaction Expiry / Deadline (★★★★☆)

`MsgSwap` has no `Expiry` or `Deadline` field. A swap submitted during low-latency conditions can sit in the mempool and execute much later when prices have moved significantly.

**Why this matters**: Without expiry, stale swaps are a user protection failure. A user who submits a swap at price P but the swap executes at price 0.8P (after market movement) because it sat in the mempool is harmed in a way that `MinTokenOut` may not protect against if they set a wide tolerance.

**Sketch**: Add `DeadlineBlock uint64` to `MsgSwap`. In the Swap handler, check `ctx.BlockHeight() <= msg.DeadlineBlock` and return `ErrSwapExpired` otherwise. This is the same pattern as Uniswap v2's `deadline` parameter.

### Gap 3: No MEV Redistribution to LPs (★★★☆☆)

When MEV extraction occurs (sandwiches, arbitrage), value flows to the extracting validator. LPs, who bear impermanent loss from the same large trades, receive none of the MEV. This creates an asymmetric loss structure: LPs absorb the volatility, validators capture the extractable value.

**Why this matters**: If LP returns systematically underperform MEV extraction rates, rational LPs will exit, reducing pool depth, increasing price impact, and creating a negative spiral.

**Sketch**: Implement a `MEVAuctionShareBps uint64` parameter. Require validators to bid for the right to include swap transactions in a sealed-bid auction mechanism. Auction proceeds flow to pool reserves as LP revenue. This is the Flashbots model adapted for Cosmos — SUAVE's sealed-bid auction infrastructure.

### Gap 4: No Maximum Swap Size per Block per Pool (★★★☆☆)

A single `MsgSwap` can consume an arbitrary fraction of pool reserves. A very large swap can dramatically move the spot price in one block, which:
1. Distorts the TWAP for the block(s) following the large swap
2. Creates the maximum price impact scenario for other traders in the same block

**Why this matters**: Pool depth protection is especially critical when TWAP is used as a price oracle (Brief 8). A single block with a large swap can corrupt the TWAP window's tail.

**Sketch**: Add `MaxSwapFractionBps uint64` to pool parameters (e.g., 100_000 = 10%). In `Swap`, compute `maxInput = reserveIn * MaxSwapFractionBps / 1_000_000`. If `tokenIn > maxInput`, return `ErrSwapTooLarge`. This bounds single-block price impact, protecting TWAP integrity.

### Gap 5: No Fair Sequencing Within Block (★★★☆☆)

Cosmos SDK's ABCI application determines transaction ordering within a block. By default, transactions are ordered by fee (descending). For zerone's knowledge module, this means high-fee claims are processed before low-fee claims — but the commit-reveal scheme already handles this for verifications.

For the AMM, transaction ordering matters: two concurrent swaps in the same direction will get different prices depending on order. Currently, the validator decides.

**Why this matters**: Deterministic ordering within a block (e.g., time-of-arrival FIFO) removes the validator's ability to profit from reordering AMM transactions.

**Sketch**: Implement `MsgSwap` with a `SubmissionBlock` field (the block at which the user first submitted, from the previous block's `EndBlocker` snapshot). Swaps in a block are ordered by `SubmissionBlock ASC, then tx hash ASC` — first come, first served, breaking ties with a deterministic hash. Requires a commit-reveal mechanism (Gap 1) to make the submission order unforgeable.

---

## Convergence Notes (for Synthesis)

- **Commit-reveal for swaps** (Gap 1) is the same mechanism as the knowledge module's verification commit-reveal — zerone already knows this pattern. The infrastructure exists; it's a matter of applying it to the AMM layer.
- **TWAP integrity protection** (Gap 4, max swap per block) is a prerequisite for **oracle resilience** (Brief 8) — the TWAP is the oracle; protecting it from manipulation is an oracle-integrity problem.
- **Transaction expiry** (Gap 2) is the AMM analog of **settlement finality**'s (Brief 1) `AggregationDeadline` — both impose time limits on pending operations to prevent stale execution.
- **MEV redistribution to LPs** (Gap 3) convergences with **treasury** (Brief 4) — if MEV is captured through an auction mechanism, the auction proceeds enrich pool reserves (LP revenues), not the protocol treasury. This is a policy choice with significant incentive implications.
- **Pool lock flag** (already implemented) is the AMM-level instance of the **circuit breaker** mechanism (Brief 9) — both halt operations on specific markets under adverse conditions.

---

## Summary Verdict

Zerone's AMM has the correct baseline: constant-product formula, slippage protection via `MinTokenOut`, TWAP accumulator, and a pool lock circuit breaker. The TWAP's block-level update cycle provides reasonable protection against single-block manipulation.

The critical structural gap is **no commit-reveal for swap execution**. In a validator-controlled block ordering environment, observable swap parameters are extractable value. The knowledge module's commit-reveal pattern is directly applicable and already in the codebase — the infrastructure exists to solve this.

Priority recommendation: implement `MsgCommitSwap` / `MsgRevealSwap` with a 2-block delay, add `DeadlineBlock` to `MsgSwap`, and add `MaxSwapFractionBps` to pool parameters. These three changes significantly reduce the AMM's MEV surface.
