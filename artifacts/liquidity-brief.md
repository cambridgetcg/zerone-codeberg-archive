# Liquidity Infrastructure and Market Making
*Zerone Financial Substrate — Brief 7 of 10*

---

## Methodology

Liquidity is the capacity of a market to absorb trades without significant price impact. In traditional finance, market makers provide this: they post continuous bid-ask quotes and earn the spread. In AMMs (automated market makers), the market-making function is replaced by a mathematical formula and liquidity providers who deposit assets in exchange for LP tokens.

The academic foundation:
- **Constant-product AMMs** (Uniswap v1/v2, Adams 2018): `x × y = k` invariant; price is the ratio of reserves
- **Concentrated liquidity** (Uniswap v3, Adams et al. 2021): LPs can specify a price range, earning more fees when price stays in range but earning nothing outside it — capital efficiency at the cost of active management
- **Impermanent loss** (Pintail 2019): LPs systematically underperform passive holding when price changes significantly — the cost of providing liquidity
- **Bonding curves** (Bancor 2017): continuous liquidity via algorithmic reserves; no discrete LP deposits needed
- **Liquidity mining** (Compound 2020, SushiSwap 2020): token emission as subsidy for LP participation; teaches the market to provide liquidity before organic fee revenue can sustain it

**Key sources**: Adams et al. (2021, Uniswap v3); Evans (2021, Liquidity Provider Returns in Geometric Mean Markets); Pintail (2019, Uniswap — A Good Deal for Liquidity Providers?); Egorov (2019, StableSwap invariant); Bancor whitepaper; Evans & Angeris (2020, Liquidity and Price Discovery).

---

## What Zerone Already Has

### 1. Constant-Product AMM (`x/liquiditypool/keeper/amm.go`)

Full constant-product implementation:

```
swap output:   dy = y × dx_effective / (x + dx_effective)
               where dx_effective = dx × (1 - feeBps/1M)
               fee = dx × feeBps / 1M
```

LP deposit logic:
- Initial deposit: `LP = sqrt(amountA × amountB)` — geometric mean initialization
- Subsequent deposits: `LP = totalSupply × min(amountA/reserveA, amountB/reserveB)` — proportional
- Withdrawal: `amountA = reserveA × LP / totalSupply`, same for B — proportional redemption

Proportional deposit enforcer: `CalculateProportionalDeposit` computes actual deposit amounts to maintain pool ratio — prevents liquidity deposit from moving the price.

### 2. TWAP Accumulator (`x/liquiditypool/keeper/twap.go`)

Block-weighted cumulative price with 1e12 precision:
```
cumPriceAToB += (reserveB / reserveA) × blocksDelta × 1e12
```

`GetTWAP` computes the average over a configurable window. Falls back to spot price if insufficient history. Updated every block in `BeginBlock`.

This provides:
- **Price smoothing**: resists single-block manipulation
- **Historical price reference**: applications can query `GetTWAP(poolId, denom, windowBlocks)` for a manipulation-resistant price feed

### 3. LP Tokens as ZRN-20 Secondary Tokens

`Pool.LpDenom = "lp/{pool_id}"` — LP tokens are registered as ZRN-20 secondary tokens via the tokens module. This means:
- LP positions are transferable (not address-locked)
- LP tokens can participate in any ZRN-20 feature (wrapping, delegation, future DeFi integrations)
- LP holdings are queryable via the standard token balance API

### 4. Per-Pool Fee Configuration

`Pool.SwapFeeBps` is set at pool creation and applies to all swaps in that pool. A `DefaultSwapFeeBps` in params provides a fallback. The per-pool fee enables different pools to serve different trading profiles (e.g., a stablecoin pool at 0.05%, a volatile asset pool at 0.3%).

### 5. Pool Lock as Reentrancy Guard

`Pool.Locked bool` is used as a reentrancy guard during complex operations. While currently coarse (locks the entire pool), it provides the foundation for pool-level circuit breaker behavior.

---

## Gaps and Gaps Only

### Gap 1: No Liquidity Mining / Emission to LP Holders (★★★★★)

**The bootstrap liquidity problem.** A new protocol's pools have low liquidity because there's no LP fee revenue to attract providers. Without liquidity mining — emission-funded LP rewards — zerone's pools will remain shallow until organic fee volume creates sustainable yields.

**Why this matters**: Shallow pools mean high price impact, which deters trading, which means low fee revenue, which means LPs leave — a death spiral. Every major AMM launched with liquidity mining to break this chicken-and-egg problem.

**Sketch**: Add `LiquidityMiningEmissionId string` to `Pool` — an associated `EmissionPeriod` that distributes `AmountPerBlock` uzrn to LP token holders proportionally. In `BeginBlocker` (tokens module), when processing the emission period, iterate LP token holders via `IterateBalances(ctx, pool.LpDenom)` and distribute proportionally. This is a 3-4 function addition connecting the tokens module's `EmissionPeriod` system to the liquiditypool's LP token registry.

This is the only liquidity bootstrap mechanism that doesn't require a centralized market maker — exactly the kind of connection the system's existing primitives enable.

### Gap 2: No Concentrated Liquidity (Ranged Positions) (★★★☆☆)

All LP positions span the entire price range [0, ∞). This means:
- Most liquidity sits at prices far from the current market price — it never earns fees
- Capital efficiency is low compared to Uniswap v3's concentrated positions

**Why this matters**: Capital efficiency determines whether LPs earn competitive returns. Low efficiency means you need more capital locked to provide the same depth at the current price — a barrier to LP participation.

**Sketch**: Implement tick-based liquidity ranges (`TickLower`, `TickUpper` on LP positions). Liquidity only contributes to fee earning when price is within the position's range. This is a significant implementation — Uniswap v3's core paper describes the full math. For zerone's current stage, this is a v2 priority (after liquidity mining is working), not v1.

### Gap 3: No Impermanent Loss Protection (★★★☆☆)

LPs provide liquidity and are exposed to impermanent loss — if price diverges significantly from their deposit price, they'd be better off holding. There is no mechanism to compensate LPs for IL.

**Why this matters**: Impermanent loss is the primary deterrent to LP participation in volatile asset pools. Protocols that ignore it see chronic LP attrition.

**Sketch**: Implement IL insurance via protocol emission. At withdrawal, compute the IL experienced by the LP position (actual withdrawal value vs. hold-equivalent value at current prices). If IL exceeds `ILInsuranceThresholdBps`, compensate the excess from a dedicated `ILInsuranceFund` (funded by protocol emission or a fraction of swap fees). This is similar to Bancor v2.1's IL protection mechanism.

### Gap 4: No Bonding Curve for Single-Asset Pools (★★★☆☆)

The constant-product AMM requires both assets to seed a pool. For ZRN-only markets (e.g., staking derivatives, LP tokens), there is no single-asset bonding curve option.

**Why this matters**: Many useful markets involve pricing one asset against ZRN. A bonding curve (e.g., polynomial or logarithmic) would allow pools seeded with just ZRN to provide automatic liquidity for any ZRN-20 token.

**Sketch**: Add `PoolType` enum to `Pool`: `CONSTANT_PRODUCT` (current) and `BONDING_CURVE`. For bonding curve pools, implement the Bancor formula: `price = reserveA / (supplyB × CRR)` where `CRR` (constant reserve ratio) is set at pool creation. Initial minting and burning adjusts the curve. This enables fair-launch token distribution and ZRN-denominated price discovery for new ZRN-20 tokens.

### Gap 5: No LP Withdrawal Delay (Cooldown) (★★★☆☆)

LP tokens can be redeemed immediately after deposit. This enables **just-in-time liquidity** attacks: deposit liquidity immediately before a large expected trade (capturing fees), then withdraw immediately after.

**Why this matters**: JIT liquidity dilutes honest LPs' fee share — rational LPs see their fees captured by opportunistic flash providers. Uniswap v3 added LP withdrawal cooldowns partly in response to this.

**Sketch**: Add `MinLiquidityDurationBlocks uint64` to pool params. At `RemoveLiquidity`, check `ctx.BlockHeight() - deposit.DepositBlock >= MinLiquidityDurationBlocks`. If not, reject with `ErrWithdrawalTooEarly`. Store `DepositBlock` in a per-LP-position record. This simple time-lock prevents flash liquidity while negligibly affecting long-term LPs.

---

## Convergence Notes (for Synthesis)

- **Liquidity mining** (Gap 1) directly connects the **tokens module's `EmissionPeriod`** (monetary policy, Brief 2) to the **liquiditypool module** — it's an emission routing decision, not new infrastructure.
- **TWAP as oracle** (already implemented) is the bridge to **oracle resilience** (Brief 8) — the TWAP's integrity is the liquidity infrastructure's contribution to the protocol's external truth feed.
- **Pool lock** (already implemented) is the liquidity layer's instance of the **circuit breaker** mechanism (Brief 9).
- **Swap fee accumulation** in reserves connects to **treasury** (Brief 4) — swap fees currently accrue to LP tokens (pool reserves grow), not the protocol treasury. This is a design choice worth noting in the synthesis: protocol treasury doesn't benefit from AMM activity unless a `ProtocolFeeBps` fraction is diverted.
- **Concentrated liquidity** (Gap 2) is ultimately a capital efficiency improvement — it's the microstructure analog of **calibration accuracy** in the epistemics layer: better precision, same coverage but only in range.

---

## Summary Verdict

Zerone's liquidity infrastructure has the correct mathematical foundation — constant-product AMM, proportional LP mechanics, TWAP accumulator, and LP tokens as first-class ZRN-20 citizens. The pool locking mechanism provides emergency controls.

The critical gap is **no liquidity mining**. The `EmissionPeriod` primitive exists; the LP token registry exists; the connection between them is missing. Without it, zerone's pools will launch shallow and stay shallow. This is the single highest-leverage addition to the liquidity layer — it's an emission routing decision, implemented as ~50 lines connecting two already-existing systems.

Priority recommendation: implement `LiquidityMiningEmissionId` on `Pool`, add an LP token distribution hook in the tokens `BeginBlocker`, and add `WithdrawalCooldownBlocks` to pool params to prevent JIT attacks. Together these transform the AMM from a working primitive into a viable liquidity market.
