# Counterparty Risk and Escrow for Agent Transactions
*Zerone Financial Substrate — Brief 10 of 10*

---

## Methodology

Counterparty risk is the probability that the other party in a transaction will fail to fulfill their obligation. Traditional finance addresses this through clearing houses (CCP — central counterparty clearing), collateral requirements, and netting agreements. In DeFi, smart contracts replace the CCP: code enforces obligations automatically.

For zerone specifically, counterparty risk operates at three levels:
1. **Knowledge submission**: will a submitter's claim be acted upon faithfully?
2. **Dispute resolution**: will the challenger's bond be adjudicated fairly?
3. **Agent-to-agent transactions**: as zerone becomes an AI agent coordination layer, will computational work be compensated reliably?

The academic tradition:
- **Coase theorem** (1960): in the absence of transaction costs, parties will bargain to efficient outcomes; escrow reduces transaction costs by making enforcement automatic
- **Optimal contract theory** (Hart & Moore 1988, Holmström 1979): incomplete contracts create hold-up problems; escrow reduces hold-up
- **Payment channels** (Poon & Dryja 2016, Lightning Network): conditional payment via hash time-locked contracts — the template for time-locked escrow
- **ERC-4907/ERC-721** rental NFTs: temporary transfer with automatic reversion — the template for conditional access escrow

**Key sources**: Coase (1960, The Problem of Social Cost); Hart & Moore (1988, Incomplete Contracts and Renegotiation); Poon & Dryja (2016, Lightning Network); Szabo (1994, Smart Contracts); Kelkar et al. (2020, Themis — fair sequencing for smart contracts).

---

## What Zerone Already Has

### 1. Challenger Bond Escrow (`x/disputes`)

`Dispute.Bond` — the challenger stakes uzrn to open a dispute. This is held in the disputes module account. Settlement distributes it based on outcome:

| Outcome | Bond Distribution |
|---------|------------------|
| Challenger Wins | Challenger gets back bond - slash; loser (defender's staked content) loses `SlashRateLoserBps`; winner gets `RewardRateWinnerBps`; arbiters get `ArbiterRewardBps`; **remainder burns** |
| Defender Wins | Bond (challenger's): partially refunded, partially slashed; winner (defender) receives reward from slash |
| Draw | Half/half refund |
| Timed Out | Distribution on timeout (some to arbiters for participation) |

The burn-remainder mechanic is correct game theory: it eliminates the incentive to dispute with a collusive arbiter (since the burn is a dead-weight loss that benefits no one).

### 2. Stake-Weighted Arbiter Selection with Conflict Exclusion

`SelectArbiters` excludes the challenger and defender from the arbiter panel, selects by score (tier × performance), and applies stake-weighted voting. This is a proper independent tribunal structure.

Outcomes: `CHALLENGER_WINS`, `DEFENDER_WINS`, `DRAW`, `TIMED_OUT` — four possible results with distinct settlement logic.

### 3. Tier-Differentiated Bond Requirements

`TierConfig.MinBond` — each dispute tier has a minimum bond. Higher tiers require higher bonds, which:
- Deters frivolous high-stakes disputes
- Ensures the bond is economically meaningful relative to the asset under dispute
- Ties dispute cost to the tier of the fact being challenged

### 4. Vindication Escrow in Knowledge Module

`VindicationEscrowModuleName` in the knowledge module holds slashed verifier tokens. When an incorrect verifier is slashed (voted wrong), their slashed tokens are escrowed. If the fact they rejected is later vindicated (proven correct by a subsequent challenge), the escrowed tokens are returned.

This is **conditional escrow triggered by future events**: the escrow releases not on a timer but on a chain state condition (fact vindication). This is a more sophisticated escrow pattern than most DeFi protocols implement.

### 5. Evidence Commitment Scheme

`EvidenceCommitment` in disputes — evidence is submitted as a hash commitment before the reveal phase. This is the same commit-reveal pattern as knowledge module verification rounds, applied to dispute evidence.

---

## Gaps and Gaps Only

### Gap 1: No Defender Bond (One-Sided Escrow) (★★★★★)

**The most critical gap.** Only the challenger posts a bond. The defender (owner of the disputed content) has no skin-in-the-game requirement. This creates **asymmetric dispute incentives**:
- Challenger: must stake significant bond, risks losing it on a failed challenge
- Defender: no cost to defend; can delay indefinitely with no economic penalty

**Why this matters**: If the defender faces no cost for a false defense, they will always defend — even clearly invalid content. This inflates the dispute cost for legitimate challengers and creates a defensive moat for bad actors.

**Sketch**: Add `DefenderBond string` to `Dispute`. When a challenger opens a dispute, the defender must respond within `EvidencePeriod` by posting `DefenderBond = Dispute.Bond × DefenderBondRatioBps / 1_000_000` (default 50%). If the defender fails to post their bond within the period, the dispute is auto-resolved in favor of the challenger (the content is removed and the challenger's bond is returned). In `DistributeSettlement`, include `DefenderBond` in the settlement pool. This follows the Kleros two-sided deposit model.

### Gap 2: No Time-Locked Escrow for Agent Agreements (★★★★☆)

Zerone has the dispute module for content disputes, but there is no general-purpose **service escrow** for agent-to-agent agreements: "agent A pays X uzrn to agent B for completing task Y; payment releases on proof of completion; disputed via arbitration if disputed."

**Why this matters**: As zerone becomes an AI agent coordination layer, agents will need to contract with each other for computational work, data retrieval, and knowledge services. Without a general escrow primitive, these agreements require off-chain trust — which reintroduces counterparty risk.

**Sketch**: Add `x/escrow` module (or extend `x/disputes`) with `ServiceEscrow`:
```
type ServiceEscrow struct {
    Id           string
    Payer        string    // agent A
    Provider     string    // agent B
    Amount       string    // uzrn locked
    TermsHash    string    // SHA256 of service agreement
    Deadline     uint64    // block deadline for completion
    ProofHash    string    // set by provider on completion
    Status       EscrowStatus  // PENDING, COMPLETED, DISPUTED, REFUNDED
}
```
Release path: provider submits `MsgCompleteEscrow{id, proof_hash}`; payer has N blocks to dispute via `MsgDisputeEscrow`; if no dispute, payment auto-releases. If disputed, the existing disputes module handles arbitration with the escrow amount as the bond. This is the Ethereum Escrow contract pattern adapted to Cosmos modules.

### Gap 3: No Escalation Path for Failed Disputes (★★★☆☆)

The current disputes module has a single adjudication layer: arbiter panel votes, outcome is final. There is no appeal mechanism for disputed outcomes.

**Why this matters**: A corrupt or negligent arbiter panel can produce incorrect outcomes. Without appeal, the losing party has no recourse except to re-challenge the original content (which costs another bond). Appeals also improve the quality of first-instance arbitration — arbiters who know their decisions can be appealed have stronger incentives for accuracy.

**Sketch**: Add `AppealWindowBlocks uint64` to `TierConfig`. Within the appeal window after a verdict, either party can submit `MsgAppealDispute{dispute_id, appeal_bond}` where `appeal_bond = original_bond × AppealBondMultiplierBps / 1_000_000`. The appeal convenes a higher-tier arbiter panel (e.g., Guardian-only for tier-1 disputes). If the appeal reverses the verdict, the appealing party receives back their appeal bond plus the original bond. If the appeal confirms the verdict, the appeal bond goes to the winning party. Maximum 1 appeal per dispute.

### Gap 4: No Cross-Module Counterparty Risk Aggregation (★★★☆☆)

Bond requirements in disputes, claim stakes in knowledge, LP positions in liquiditypool, and validator stakes in staking are all tracked independently. There is no unified view of a single agent's total locked collateral across modules.

**Why this matters**: An agent that has staked 1000 uzrn across 5 modules simultaneously appears liquid in each module but is actually fully committed. Without aggregated collateral tracking, modules cannot enforce a maximum leverage ratio — an agent could over-commit their available capital.

**Sketch**: Add a `CollateralRegistry` (a simple module or addition to `x/auth`) that tracks all locked capital per address: `{address, module, amount, lock_reason}`. Each module that locks capital (disputes, knowledge, liquiditypool, staking) registers the lock on entry and deregisters on release. Add `GetTotalLockedCollateral(addr)` query. Use this in `MsgSubmitClaim` to enforce `lockedCollateral / totalBalance <= MaxLeverageBps / 1_000_000`.

### Gap 5: No Reputation-Adjusted Bond Requirements (★★★☆☆)

Bond requirements are fixed per tier (`TierConfig.MinBond`). A challenger with a strong track record of successful challenges should post a smaller bond (demonstrated reliability), while a new challenger with no track record should post a larger bond.

**Why this matters**: Fixed bond requirements treat experienced challengers and unknown actors identically. This under-prices risk for unknown actors (too easy to challenge frivolously) and over-prices it for experienced ones (disincentivizes legitimate challenges). Reputation-adjusted bonds are the counterparty risk equivalent of credit scoring.

**Sketch**: Add `ChallengerReputationDiscount` — computed from the challenger's dispute history (`CHALLENGER_WINS / total_disputes`) stored in a new `DisputerProfile`. Challengers with >80% win rate post `MinBond × (1 - reputation_discount)`. Challengers with no history post full `MinBond`. Challengers with <20% win rate post `MinBond × PenaltyMultiplierBps / 1_000_000`. This creates a merit-based escrow system — consistent with the truth-seeking tier system's logic of calibration over time.

---

## Convergence Notes (for Synthesis)

- **Two-sided bonds** (Gap 1) make the dispute system symmetrically fair — the same principle applies to the **knowledge module's challenger vs. defender** asymmetry (claim submitter bonds but claimee — the accepted fact — has no bond requirement beyond its established credibility).
- **Service escrow** (Gap 2) is the transaction infrastructure for the **toolbox module's agent transactions** — agents using tools via `x/toolbox` need payment guarantees. The escrow module would be the payment settlement layer.
- **Collateral aggregation** (Gap 4) directly supports **slashing safety** (Brief 3) — a validator's total collateral across all modules determines their true economic security contribution.
- **Reputation-adjusted bonds** (Gap 5) are the financial expression of the **calibrated forecasting** principle from the truth-seeking series — prior track record adjusts required collateral, exactly as Brier scores adjust verifier calibration.
- **Vindication escrow** (already implemented) is zerone's most sophisticated escrow design — it's a **conditional release on future chain state**, not just on time or party action. This is the template for designing the service escrow in Gap 2.

---

## Summary Verdict

Zerone's counterparty infrastructure is **asymmetrically designed**: challenger bond escrow is well-implemented with proper burn mechanics, stake-weighted arbitration, and vindication escrow. The disputes module is more sophisticated than most DeFi protocols at equivalent stage.

The critical structural gap is **one-sided bonding** — defenders face no economic cost for bad-faith defense. Adding `DefenderBond` with auto-resolution on non-response is the highest-leverage single change to the disputes module.

The strategic gap is **no service escrow for agent agreements**. As zerone's toolbox and agent coordination matures, agents need first-class payment settlement with automatic dispute escalation. The disputes module is already the right arbiter — it just needs a `ServiceEscrow` primitive to front-load the agreement structure.

Priority recommendation: implement `DefenderBond` in `Dispute` proto with auto-resolution on non-response (short implementation: new field + timeout check in `EndBlocker`), and begin `ServiceEscrow` proto design as the payment primitive for agent-to-agent transactions.
