# Settlement Finality and Irreversibility
*Zerone Financial Substrate — Brief 1 of 10*

---

## Methodology

Settlement finality answers one question: **when is an outcome permanent?**

The academic tradition spans distributed systems (Fischer-Lynch-Paterson impossibility), Byzantine fault tolerance (Castro-Liskov PBFT), and blockchain finality theory (Buterin's casper finality gadget). The practical framework distinguishes:

- **Probabilistic finality**: Bitcoin's longest-chain rule — a transaction is "final" after N confirmations, but never absolutely
- **Absolute (deterministic) finality**: PBFT and Tendermint's BFT consensus — once 2/3+ validators sign a block, it's final by protocol; there is no rollback
- **Economic finality**: slashing makes reversion prohibitively expensive even if technically possible
- **Application-layer finality**: at the smart contract / module level — once a state machine transition is committed and its side effects are distributed, the transition is semantically irreversible even if the underlying chain technically allows rollbacks

**Key sources**: Fischer-Lynch-Paterson (1985, FLP impossibility); Castro & Liskov (1999, PBFT); Buterin (2015, Ethereum 2.0 finality); Tendermint whitepaper (Kwon 2014); Lamport clocks and logical time; Brewer's CAP theorem.

---

## What Zerone Already Has

### 1. Three-Phase Commit-Reveal-Aggregation (`x/knowledge/keeper/rounds.go`)

Every claim verification runs through a deterministic block-deadline state machine:

```
COMMIT_PHASE         REVEAL_PHASE         AGGREGATION_PHASE     COMPLETE
[0, CommitDeadline) → [CommitDeadline, RevealDeadline) → [RevealDeadline, AggregationDeadline) → VerdictBlock
```

Fields on `VerificationRound`:
- `CommitDeadline = StartedAtBlock + CommitPhaseBlocks`
- `RevealDeadline = CommitDeadline + RevealPhaseBlocks`
- `AggregationDeadline = RevealDeadline + AggregationPhaseBlocks`
- `VerdictBlock` — the block height at which `CompleteRound` ran

This is **application-layer finality with block-timestamp anchoring**. The phases are structurally ordered by block height — no phase can be entered out of order.

### 2. Irreversible State Transitions on `CompleteRound`

`CompleteRound` is the finality gate. Once it runs:

| Verdict | Claim Status | Side Effects |
|---------|-------------|--------------|
| ACCEPT | ACCEPTED | Fact created; 55% verifier pool distributed; vindication escrow routes slashes; submitter calibration updated |
| REJECT | REJECTED | Challenge survival handled; contradict side-effects reversed; challenge outcome recorded |
| MALFORMED | MALFORMED | Contradictions reversed |
| INCONCLUSIVE | INSUFFICIENT | Contradictions reversed |

The verdict is written to both `round.Verdict` and `claim.Status`. Both are persisted via `SetVerificationRound` and `SetClaim`. The bank transfers (verifier rewards, challenger stakes) execute atomically within the same Cosmos SDK message handler.

This satisfies **economic finality** at the application layer: once rewards are distributed and stakes settled, there is no on-chain path to reverse them short of an `EmergencyRevertProposal` (the explicit circuit breaker).

### 3. Commit-Reveal Scheme Prevents Front-Running

The two-phase scheme (hash commitment first, plaintext reveal second) prevents verifiers from copying each other's votes mid-round. This is the application-layer equivalent of sealed-bid auctions — it provides **ordering fairness** for the verification process itself.

### 4. Vindication Escrow as Conditional Finality

`VindicationEscrowModuleName` holds slashed tokens from incorrect verifiers when a fact is ACCEPTED. These tokens can later be claimed via vindication if the fact is subsequently falsified. This creates a **two-level finality model**:
- Round-level: verdict is immediate and irreversible
- Fact-level: a fact can be challenged later — but through a new round, not by rewinding the original

This is the correct design. Facts persist until falsified through the normal submission process, not through retroactive round modification.

### 5. CREED Commitments as Finality Invariants

Commitments 3 and 4 (emitted as events in `CompleteRound`) encode the semantic finality guarantees as observable, auditable chain events — not just internal state changes. This means finality is not only structural but **publicly verifiable**.

---

## Gaps and Gaps Only

### Gap 1: No Block Hash Anchoring on Round Records (★★★★☆)

`VerificationRound` records `VerdictBlock` (height) but not the block **hash** at that height. On chains using Cosmos SDK, block hashes are available via `sdk.Context.HeaderHash()`.

**Why this matters**: A malicious validator set could theoretically construct two valid block histories diverging at the verdict block (a long-range attack). With block hash anchoring, the verdict is bound to a specific chain history.

**Sketch**: Add `VerdictBlockHash []byte` field to `VerificationRound`. In `CompleteRound`, set `round.VerdictBlockHash = sdkCtx.HeaderHash()`. Cost: one 32-byte field per completed round.

This follows the same principle as Bitcoin's block header commitment: the fact of settlement is anchored to the specific chain state that produced it.

### Gap 2: No Unbonding Period on Claim Stake Before Settlement (★★★☆☆)

The claim stake (`Claim.Stake`) is locked at submission and released at settlement — but there is no minimum lock period enforced separately from the round phases. If `CommitPhaseBlocks + RevealPhaseBlocks + AggregationPhaseBlocks` is very short, stake could cycle rapidly.

**Why this matters**: Economic finality requires the stake to be large relative to the potential profit from double-spending the outcome. Very fast rounds reduce the economic security of each individual verdict.

**Sketch**: A `MinRoundDurationBlocks` parameter, checked in `CreateVerificationRound`, enforcing a minimum total round length regardless of individual phase lengths.

### Gap 3: No Timeout Finality Path for Abandoned Rounds (★★★☆☆)

If a round passes `AggregationDeadline` with no aggregation call, the round remains open indefinitely. The claim stays in `IN_VERIFICATION` state. There is no keeper-level `EndBlock` sweep that finalizes timed-out rounds.

**Why this matters**: Stuck rounds are a liveness failure. A claim in perpetual `IN_VERIFICATION` cannot be challenged, re-submitted, or referenced.

**Sketch**: In `EndBlocker`, sweep rounds where `ctx.BlockHeight() > AggregationDeadline` and current phase is not `COMPLETE` — emit a `VERDICT_INCONCLUSIVE` to close them gracefully. Distribute verifier fees for reveals received.

### Gap 4: No Finality Receipt for Submitters (★★☆☆☆)

`CompleteRound` emits events but does not issue a cryptographically signed **finality receipt** (a message signed by the validator set attesting to the verdict). On IBC-connected chains, this receipt would allow remote chains to verify zerone's verdicts trustlessly.

**Why this matters**: As zerone integrates with other chains via IBC, its verdicts need to be verifiable externally. A finality receipt is the primitive for cross-chain truth attestation.

**Sketch**: On `CompleteRound`, emit a deterministic message `hash(round_id + verdict + verdict_block + block_hash)` as an IBC channel event. Remote chains can verify this against the IBC light client.

---

## Convergence Notes (for Synthesis)

- **Vindication escrow** (settlement finality) converges with **slashing** (economic security) — the escrow design assumes slashed tokens are recoverable if the outcome was wrong. Brief 3 (Slashing) should reference this.
- **Block hash anchoring** is foundational for **oracle resilience** (Brief 8) — TWAP manipulation resistance also depends on block-level anchoring of price data.
- **Timeout finality** for abandoned rounds is a prerequisite for **circuit breaker** design (Brief 9) — emergency halt proposals need a clear model of what "pending" means.
- The **three-phase commit-reveal** scheme is the same structural primitive used in **market microstructure** (Brief 6) for anti-front-running — they share the same information asymmetry problem.

---

## Summary Verdict

Settlement finality in zerone is **architecturally sound** at the application layer. The three-phase block-deadline state machine, the irreversible state transitions in `CompleteRound`, the commit-reveal anti-gaming scheme, and the vindication escrow model together constitute a coherent finality design.

The critical gap is **block hash anchoring** — verdicts are height-bound but not hash-bound, leaving a theoretical long-range attack surface. The practical gap is **timeout finality for abandoned rounds** — stuck rounds are a liveness risk that the current architecture doesn't resolve.

Priority recommendation: implement `VerdictBlockHash` in `VerificationRound` proto (one field, set in `CompleteRound`) and add an `EndBlocker` sweep for timed-out rounds. Both are small changes with high finality integrity value.
