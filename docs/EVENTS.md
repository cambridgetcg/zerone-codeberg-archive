# Zerone Event Reference

All state-changing operations on Zerone emit queryable events following the
`zerone.<module>.<action>` naming convention. Every attribute value is a string
(CometBFT requirement). Events are deterministic: identical input produces
identical events.

---

## Table of Contents

- [alignment](#alignment)
- [auth](#auth)
- [autopoiesis](#autopoiesis)
- [billing](#billing)
- [bvm](#bvm)
- [capture_challenge](#capture_challenge)
- [capture_defense](#capture_defense)
- [channels](#channels)
- [claiming_pot](#claiming_pot)
- [compute_pool](#compute_pool)
- [counterexamples](#counterexamples)
- [discovery](#discovery)
- [disputes](#disputes)
- [emergency](#emergency)
- [evidence_mgmt](#evidence_mgmt)
- [gov](#gov)
- [home](#home)
- [ibcratelimit](#ibcratelimit)
- [icaauth](#icaauth)
- [inquiry](#inquiry)
- [knowledge](#knowledge)
- [liquiditypool](#liquiditypool)
- [ontology](#ontology)
- [partnerships](#partnerships)
- [private_corpus](#private_corpus)
- [qualification](#qualification)
- [research](#research)
- [schedule](#schedule)
- [staking](#staking)
- [tokens](#tokens)
- [toolbox](#toolbox)
- [tree](#tree)
- [vesting_rewards](#vesting_rewards)
- [WebSocket Subscriptions](#websocket-subscriptions)
- [Transaction Indexing](#transaction-indexing)
- [Block Explorer Compatibility](#block-explorer-compatibility)

---

## alignment

### zerone.alignment.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.alignment.activated
Module enabled or disabled.
- `authority` -- governance address
- `enabled` -- `"true"` or `"false"`

### zerone.alignment.observation_recorded
*EndBlock.* Periodic AHI observation completed.
- `height` -- block height
- `composite_score` -- composite alignment score (BPS)
- `category` -- health category (`healthy`, `warning`, `critical`)
- `correction_count` -- number of corrections applied
- `observation_count` -- cumulative observation count

---

### zerone.alignment.correction_confidence_updated
Correction confidence recalculated for a category after an observation cycle.
- `category` -- alignment category
- `confidence_bps` -- updated confidence in basis points
- `effective_max_magnitude` -- effective max magnitude based on confidence

### zerone.alignment.correction_governance_required
Correction magnitude exceeds auto-apply threshold; governance proposal required.
- `dimension` -- alignment dimension
- `direction` -- correction direction
- `effective_max` -- effective maximum magnitude
- `magnitude` -- proposed magnitude
- `parameter` -- target parameter
- `reason` -- why governance is required

### zerone.alignment.correction_outcome_recorded
Result of a correction recorded for confidence tracking.
- `dimension` -- alignment dimension
- `height` -- block height of observation
- `magnitude` -- correction magnitude applied
- `score_after` -- score after correction
- `score_before` -- score before correction
- `successful` -- `"true"` or `"false"`

### zerone.alignment.network_health_critical
Network health composite dropped to critical level; defensive pacing activated.
- `analysis_multiplier_bps` -- pacing multiplier for analysis in BPS
- `composite` -- composite health score
- `creation_multiplier_bps` -- pacing multiplier for creation in BPS
- `height` -- block height

### zerone.alignment.network_health_degraded
Network health composite dropped below degraded threshold.
- `analysis_multiplier_bps` -- pacing multiplier for analysis in BPS
- `composite` -- composite health score
- `creation_multiplier_bps` -- pacing multiplier for creation in BPS
- `height` -- block height

### zerone.alignment.network_health_recovered
Network health composite returned to healthy range.
- `analysis_multiplier_bps` -- pacing multiplier for analysis in BPS
- `composite` -- composite health score
- `creation_multiplier_bps` -- pacing multiplier for creation in BPS
- `height` -- block height

### zerone.alignment.growth_pressure_detected
Verification backlog exceeded 150% of active facts; knowledge quality penalty applied (R31-1 Wood→Earth).
- `pending_ratio_bps` -- pending-to-active claim ratio in BPS
- `quality_penalty_applied` -- `"true"` or `"false"`

### zerone.alignment.correction_advisory
Correction magnitude is below `AdvisoryMagnitudeBps`; logged but not forwarded to autopoiesis (L7 banding).
- `dimension` -- alignment dimension
- `parameter` -- target parameter
- `direction` -- correction direction
- `magnitude` -- proposed magnitude
- `advisory_threshold` -- configured advisory ceiling

### zerone.alignment.verification_health_observed
Verification throughput and dispute rate observed by the governance sensor (R31-2 Fire→Earth).
- `throughput_bps` -- verification throughput in BPS
- `dispute_rate_bps` -- dispute rate in BPS


## auth

### zerone.auth.account_registered
New account registered on-chain.
- `address` -- bech32 address
- `did` -- decentralized identifier
- `account_type` -- `human` or `agent`
- `bootstrap_claimed` -- `"true"` or `"false"`

### zerone.auth.key_rotated
Key rotation performed.
- `sender` -- account address
- `key_type` -- key type rotated
- `version` -- new key version

### zerone.auth.session_created
Ephemeral session key created.
- `owner` -- account address
- `key_hash` -- hash of session key
- `expires_at` -- expiry block height

### zerone.auth.session_revoked
Session key revoked.
- `owner` -- account address
- `key_hash` -- hash of revoked key

### zerone.auth.account_frozen
Account frozen by authority.
- `address` -- frozen account
- `frozen_by` -- freezer address
- `reason` -- freeze reason

### zerone.auth.account_unfrozen
Account unfrozen.
- `address` -- unfrozen account
- `unfrozen_by` -- unfreezer address

### zerone.auth.recovery_config_set
Recovery configuration updated.
- `address` -- account address
- `threshold` -- recovery threshold
- `total_shards` -- total shard count

### zerone.auth.recovery_initiated
Account recovery initiated.
- `account` -- target account
- `initiated_by` -- initiator address
- `delay_expires_at` -- delay expiry block

### zerone.auth.recovery_shard_submitted
Recovery shard submitted.
- `account` -- target account
- `shard_index` -- shard index
- `shards_count` -- total shards submitted
- `status` -- recovery status

### zerone.auth.recovery_challenged
Recovery challenged.
- `account` -- target account
- `challenger` -- challenger address
- `reason` -- challenge reason

### zerone.auth.recovery_executed
Recovery executed.
- `account` -- recovered account
- `executed_by` -- executor address
- `new_key_version` -- new key version

### zerone.auth.params_updated
Governance parameter update.
- `authority` -- governance address

---

## autopoiesis

### zerone.autopoiesis.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.autopoiesis.activated
Module activated or deactivated.
- `authority` -- governance address
- `activated` -- `"true"` or `"false"`

### zerone.autopoiesis.multiplier_overridden
Multiplier force-set by governance.
- `authority` -- governance address
- `path` -- multiplier path
- `value` -- new BPS value

### zerone.autopoiesis.multiplier_frozen
Multiplier freeze toggled.
- `authority` -- governance address
- `path` -- multiplier path
- `frozen` -- `"true"` or `"false"`

### zerone.autopoiesis.epoch_processed
*EndBlock.* Epoch boundary reached, SSI computed, multipliers adjusted.
- `epoch` -- epoch number
- `ssi_score` -- system sustainability index (BPS, EWMA-smoothed per T8)
- `raw_ssi_score` -- raw SSI before smoothing
- `ssi_category` -- category (`healthy`, `caution`, `stressed`, `critical`)
- `block_height` -- block height
- `multiplier_count` -- number of multipliers in snapshot
- `oscillation_frozen` -- `"true"` if adjustments suppressed this epoch due to oscillation detection

### zerone.autopoiesis.oscillation_detected
Sign-flip count of SSI delta over the observation window exceeded the threshold (T8); multiplier adjustments are frozen for a cooling period.
- `epoch` -- epoch number
- `flips` -- number of sign flips detected in the window
- `window_epochs` -- configured observation window
- `frozen_until_epoch` -- epoch at which adjustments resume

### zerone.autopoiesis.change_budget_scaled
Total desired adjustment across all multipliers exceeded `MaxTotalChangeBpsPerEpoch`; each delta was scaled proportionally (L7).
- `epoch` -- epoch number
- `requested_total_bps` -- unscaled total absolute delta in BPS
- `budget_bps` -- configured budget

---

## billing

### zerone.billing.provider_registered
Knowledge provider registered.
- `address` -- provider address
- `stake` -- staked amount
- `domains` -- served domains

### zerone.billing.provider_deregistered
Provider deregistered.
- `address` -- provider address
- `stake_refunded` -- refunded stake

### zerone.billing.fact_queried
Single fact queried with payment.
- `caller` -- query caller
- `provider` -- serving provider
- `fact_id` -- queried fact ID
- `total_price` -- payment amount

### zerone.billing.batch_facts_queried
Batch fact query with payment.
- `caller` -- query caller
- `provider` -- serving provider
- `fact_count` -- number of facts
- `total_price` -- total payment

### zerone.billing.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.billing.oracle_price_update
Oracle price feed changed significantly (>5%).
- `denom` -- denomination
- `price_usd` -- new USD price
- `source` -- price source

### zerone.billing.development_fund_spend
Governance-approved disbursement from the development_fund module account.
Emitted by `MsgDevelopmentFundSpend` after successful transfer.
- `authority` -- governance address that submitted the proposal
- `recipient` -- destination address for the funds
- `amount` -- uzrn amount transferred (bigint string)
- `description` -- human-readable purpose of the spend
- `fund_balance_before` -- development_fund balance before the transfer (uzrn bigint string)
- `spending_limit_bps` -- per-proposal spending limit in effect at time of transfer (1M bps)

---

## bvm

### zerone.bvm.contract_deployed
BVM contract deployed.
- `address` -- contract address
- `deployer` -- deployer address
- `code_hash` -- bytecode hash

### zerone.bvm.contract_called
BVM contract called.
- `contract` -- contract address
- `caller` -- caller address
- `gas_used` -- gas consumed
- `success` -- `"true"` or `"false"`

### zerone.bvm.execution_scheduled
Execution scheduled for future block.
- `schedule_id` -- schedule ID
- `contract` -- contract address
- `execute_at_block` -- target block

### zerone.bvm.contract_scheduled
Contract scheduled.
- `schedule_id` -- schedule ID
- `contract` -- contract address
- `execute_at_block` -- target block

### zerone.bvm.schedule_cancelled
Scheduled execution cancelled.
- `schedule_id` -- schedule ID
- `caller` -- canceller

### zerone.bvm.update_contract_state
Contract state updated.
- `contract_address` -- contract address
- `caller` -- caller address

### zerone.bvm.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.bvm.schedule_executed
*BeginBlock.* Scheduled execution completed.
- `schedule_id` -- schedule ID
- `contract` -- contract address
- `block` -- execution block
- `success` -- `"true"` or `"false"`
- `gas_used` -- gas consumed

---

## capture_challenge

### zerone.capture_challenge.challenge_submitted
Capture challenge submitted.
- `challenge_id` -- challenge ID
- `challenger` -- challenger address
- `domain` -- challenged domain
- `stake` -- bond amount
- `evidence_deadline` -- evidence deadline block
- `review_deadline` -- review deadline block

### zerone.capture_challenge.evidence_added
Evidence added to challenge.
- `challenge_id` -- challenge ID
- `challenger` -- challenger address
- `evidence_count` -- total evidence count

### zerone.capture_challenge.challenge_resolved
Challenge resolved with outcome.
- `challenge_id` -- challenge ID
- `outcome` -- resolution outcome
- `reward_amount` -- reward paid
- `slash_amount` -- amount slashed

### zerone.capture_challenge.bounty_pool_funded
Bounty pool funded.
- `domain` -- domain funded
- `sender` -- funder address
- `amount` -- funded amount
- `new_balance` -- pool balance after

---

### zerone.capture_challenge.auto_challenge_submitted
Automatic capture challenge triggered by detection module.
- `challenge_id` -- unique challenge identifier
- `domain` -- flagged domain
- `hhi` -- Herfindahl-Hirschman Index value
- `risk_score` -- capture risk score

### zerone.capture_challenge.capture_confirmed
Capture challenge resolved with capture confirmed.
- `challenge_id` -- unique challenge identifier
- `domain` -- confirmed-captured domain

### zerone.capture_challenge.params_updated
Governance parameter update.
- `authority` -- governance address


## capture_defense

### zerone.capture_defense.verification_recorded
Verification round recorded.
- `domain` -- verified domain
- `round_id` -- round ID
- `validator_count` -- participating validators
- `block_height` -- block height

### zerone.capture_defense.domain_analyzed
Domain analysis completed.
- `domain` -- analyzed domain
- `risk_score` -- computed risk score
- `hhi` -- Herfindahl-Hirschman Index
- `flagged` -- `"true"` or `"false"`

---

### zerone.capture_defense.domain_formation_bonus_set
Partnership formation bonus applied to a flagged domain to encourage decentralisation.
- `bonus_bps` -- bonus in basis points
- `domain` -- target domain
- `expiry_height` -- block height when bonus expires
- `reason` -- why bonus was set

### zerone.capture_defense.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.capture_defense.structural_immunity_updated
Domain structural immunity recalculated based on partnership density.
- `adjusted_hhi` -- HHI after immunity adjustment
- `domain` -- target domain
- `formation_bonus_active` -- `"true"` or `"false"`
- `partnership_density` -- partnership count in domain
- `raw_hhi` -- HHI before adjustment

### zerone.capture_defense.activity_threshold_relaxation
Effective HHI threshold raised for a high-activity domain (R31-2 Fire→Metal).
- `domain` -- target domain
- `base_hhi_threshold` -- base HHI threshold
- `effective_hhi_threshold` -- relaxed HHI threshold actually used
- `verification_activity_bps` -- measured activity in BPS


## channels

### zerone.channels.channel_opened
Payment channel opened.
- `channel_id` -- channel ID
- `payer` -- payer address
- `receiver` -- receiver address
- `deposit` -- initial deposit

### zerone.channels.channel_deposited
Deposit added to channel.
- `channel_id` -- channel ID
- `depositor` -- depositor address
- `amount` -- deposited amount

### zerone.channels.state_updated
Channel state updated.
- `channel_id` -- channel ID
- `nonce` -- state nonce
- `spent` -- total spent

### zerone.channels.channel_closed
Channel closed cooperatively.
- `channel_id` -- channel ID
- `payer_refund` -- refund to payer
- `receiver_payout` -- payout to receiver

### zerone.channels.channel_disputed
Channel dispute opened.
- `channel_id` -- channel ID
- `disputer` -- disputer address
- `deadline` -- dispute deadline

### zerone.channels.expired_claimed
Expired channel funds claimed.
- `channel_id` -- channel ID
- `refunded` -- refunded amount

### zerone.channels.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.channels.channel_auto_settled
Channel auto-settled by keeper.
- `channel_id` -- channel ID
- `amount_to_payer` -- payer settlement
- `amount_to_receiver` -- receiver settlement

### zerone.channels.periodic_settlement
Periodic settlement executed.
- `channel_id` -- channel ID
- `settled_amount` -- settled amount

---

## claiming_pot

### zerone.claiming_pot.pot_created
Claiming pot created.
- `pot_id` -- pot ID
- `name` -- pot name
- `total_amount` -- total claimable amount

### zerone.claiming_pot.pot_claimed
Tokens claimed from pot.
- `pot_id` -- pot ID
- `claimant` -- claimant address
- `amount` -- claimed amount

### zerone.claiming_pot.update_pot_params
Governance parameter update.
- `authority` -- governance address

---

## compute_pool

### zerone.compute_pool.provider_registered
Compute provider registered.
- `address` -- provider address
- `service_type` -- service type
- `stake` -- staked amount
- `price_per_cu` -- price per compute unit

### zerone.compute_pool.provider_unbonding
Provider unbonding initiated.
- `address` -- provider address
- `unbonding_at` -- unbonding start block

### zerone.compute_pool.heartbeat
Provider heartbeat received.
- `provider` -- provider address
- `block` -- block height

### zerone.compute_pool.price_update_pending
Price update queued.
- `address` -- provider address
- `new_price` -- pending price
- `effective_at` -- effective block

### zerone.compute_pool.credits_redeemed
Compute credits redeemed for uzrn.
- `address` -- redeemer address
- `credits` -- credits redeemed
- `uzrn` -- uzrn received

### zerone.compute_pool.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.compute_pool.provider_jailed
*BeginBlock.* Provider jailed for missed heartbeat.
- `address` -- provider address
- `last_heartbeat` -- last heartbeat block
- `block_height` -- current block

### zerone.compute_pool.price_applied
*BeginBlock.* Pending price change applied.
- `address` -- provider address
- `new_price` -- applied price

### zerone.compute_pool.provider_removed
*BeginBlock.* Provider unbonding completed, removed.
- `address` -- provider address
- `stake_refunded` -- refunded stake

---

## counterexamples

### zerone.counterexamples.proposed
Counterexample proposed against an existing fact (Creed Commitment 15).
- `counterexample_id` -- counterexample ID
- `fact_id` -- target fact ID
- `author` -- author address
- `error_type` -- error classification (e.g. `COUNTEREXAMPLE_ERROR_TYPE_LOGICAL`)
- `creed_commitment` -- always `"15"`

### zerone.counterexamples.validation_recorded
Validator cast a vote on a proposed counterexample.
- `validation_id` -- sequential validation ID
- `counterexample_id` -- counterexample being validated
- `validator` -- validator address
- `affirm` -- `"true"` if affirming the counterexample, `"false"` if rejecting
- `creed_commitment` -- always `"15"`

### zerone.counterexamples.resolved
Counterexample resolved once minimum votes threshold is reached.
- `counterexample_id` -- counterexample ID
- `status` -- final status (`COUNTEREXAMPLE_STATUS_VALIDATED` or `COUNTEREXAMPLE_STATUS_REJECTED`)
- `validations` -- affirming vote count
- `rejections` -- rejecting vote count
- `resolved_at_block` -- resolution block height
- `creed_commitment` -- always `"15"`

---

## discovery

### zerone.discovery.agent_registered
Agent profile registered.
- `address` -- agent address
- `stake` -- staked amount
- `display_name` -- display name

### zerone.discovery.agent_profile_updated
Profile updated.
- `address` -- agent address

### zerone.discovery.agent_heartbeat
Agent heartbeat received.
- `address` -- agent address
- `block` -- block height

### zerone.discovery.agent_deregistered
Agent deregistered.
- `address` -- agent address
- `stake_refunded` -- refunded stake

### zerone.discovery.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.discovery.profile_expired
*BeginBlock.* Agent profile expired due to inactivity.
- `address` -- agent address
- `last_active_block` -- last active block

### zerone.discovery.qualification_match_bonus
Partnership match score boosted for complementary cross-domain qualifications (R31-5).
- `seeker` -- seeker address
- `candidate` -- candidate address
- `complementarity_bps` -- qualification complementarity in BPS
- `bonus_bps` -- bonus applied to match score in BPS

---

## disputes

### zerone.disputes.dispute_initiated
Dispute initiated.
- `dispute_id` -- dispute ID
- `challenger` -- challenger address
- `defender` -- defender address
- `target_id` -- disputed target ID
- `bond` -- bond amount
- `tier` -- dispute tier

### zerone.disputes.evidence_committed
Evidence commitment submitted.
- `dispute_id` -- dispute ID
- `submitter` -- submitter address
- `side` -- `challenger` or `defender`

### zerone.disputes.evidence_revealed
Evidence revealed.
- `dispute_id` -- dispute ID
- `submitter` -- submitter address
- `evidence_id` -- evidence ID

### zerone.disputes.arbiter_voted
Arbiter cast vote.
- `dispute_id` -- dispute ID
- `arbiter` -- arbiter address
- `vote` -- vote cast

### zerone.disputes.dispute_escalated
Dispute escalated to higher tier.
- `dispute_id` -- dispute ID
- `new_tier` -- new tier
- `additional_bond` -- additional bond
- `total_bond` -- cumulative bond

### zerone.disputes.dispute_settled
Dispute settled.
- `dispute_id` -- dispute ID
- `outcome` -- settlement outcome

---

### zerone.disputes.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.disputes.defender_bond_posted
*Tx.* Dispute defender posted a symmetric bond, opting into two-bond settlement math.
- `dispute_id` -- dispute ID
- `defender` -- defender address that posted bond
- `bond` -- amount escrowed (in uzrn)

### zerone.disputes.service_escrow_created
*Tx.* Payer locked funds into a time-locked service escrow.
- `escrow_id` -- unique escrow identifier
- `payer` -- address that deposited funds
- `provider` -- address expected to deliver the service
- `amount` -- locked amount (in uzrn)
- `deadline` -- block height by which provider must submit proof
- `terms_hash` -- caller-provided hash of off-chain service terms

### zerone.disputes.service_escrow_completed
*Tx.* Provider submitted proof of delivery; funds enter the dispute window.
- `escrow_id` -- escrow identifier
- `provider` -- provider address that submitted proof
- `proof_hash` -- hash of the off-chain proof artefact
- `dispute_window_closes` -- block height after which funds auto-release to provider

### zerone.disputes.service_escrow_disputed
*Tx.* Payer opened a dispute during the dispute window; funds frozen for arbiter resolution.
- `escrow_id` -- escrow identifier
- `payer` -- address that raised the dispute
- `provider` -- provider address named in the dispute
- `amount` -- disputed amount (in uzrn)
- `reason` -- caller-provided dispute reason string

### zerone.disputes.service_escrow_refunded
*EndBlock.* PENDING escrow expired past its deadline; funds returned to payer.
- `escrow_id` -- escrow identifier
- `payer` -- address that received the refund
- `amount` -- refunded amount (in uzrn)

### zerone.disputes.service_escrow_released
*EndBlock.* COMPLETED escrow survived the dispute window; funds released to provider.
- `escrow_id` -- escrow identifier
- `provider` -- address that received the funds
- `amount` -- released amount (in uzrn)

### zerone.disputes.appeal_filed
Challenger or defender filed an appeal against a settled verdict; appeal bond held in escrow.
- `dispute_id` -- dispute identifier
- `appealer` -- address filing the appeal
- `appeal_bond` -- bond amount posted (uzrn)
- `original_outcome` -- outcome being appealed

## emergency

### zerone.emergency.halt_proposed
Emergency halt ceremony proposed.
- `ceremony_id` -- ceremony ID
- `proposer` -- proposer address
- `reason` -- halt reason

### zerone.emergency.vote_halt
Halt vote cast.
- `ceremony_id` -- ceremony ID
- `voter` -- voter address
- `approve` -- `"true"` or `"false"`

### zerone.emergency.revert_proposed
Emergency revert ceremony proposed.
- `ceremony_id` -- ceremony ID
- `proposer` -- proposer address
- `target_block` -- target revert block

### zerone.emergency.vote_revert
Revert vote cast.
- `ceremony_id` -- ceremony ID
- `voter` -- voter address
- `approve` -- `"true"` or `"false"`

### zerone.emergency.resume_proposed
Resume ceremony proposed.
- `ceremony_id` -- ceremony ID
- `proposer` -- proposer address

### zerone.emergency.vote_resume
Resume vote cast.
- `ceremony_id` -- ceremony ID
- `voter` -- voter address
- `approve` -- `"true"` or `"false"`

### zerone.emergency.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.emergency.ceremony_advanced
Ceremony phase advanced (prevote quorum reached).
- `ceremony_id` -- ceremony ID
- `ceremony_type` -- `halt`, `revert`, or `resume`
- `phase` -- new phase
- `yes_prevote_stake` -- total yes stake

### zerone.emergency.ceremony_finalized
Ceremony finalized and executed.
- `ceremony_id` -- ceremony ID
- `ceremony_type` -- `halt`, `revert`, or `resume`
- `status` -- resulting chain status
- `block_height` -- finalization block

### zerone.emergency.revert_required
Revert ceremony finalized; operator action required.
- `ceremony_id` -- ceremony ID
- `target_height` -- rollback target height
- `target_hash` -- rollback target hash
- `action` -- operator instructions

---

## evidence_mgmt

### zerone.evidence_mgmt.evidence_submitted
Evidence submitted to chain.
- `evidence_id` -- evidence ID
- `submitter` -- submitter address
- `evidence_type` -- evidence type

### zerone.evidence_mgmt.custody_transferred
Evidence custody transferred.
- `evidence_id` -- evidence ID
- `from` -- previous custodian
- `to` -- new custodian

### zerone.evidence_mgmt.evidence_verified
Evidence verified.
- `evidence_id` -- evidence ID
- `verifier` -- verifier address
- `outcome` -- verification outcome
- `confidence` -- confidence score

### zerone.evidence_mgmt.evidence_challenged
Evidence challenged.
- `evidence_id` -- evidence ID
- `challenger` -- challenger address
- `dispute_id` -- linked dispute ID
- `bond` -- challenge bond

### zerone.evidence_mgmt.update_params
Governance parameter update.
- `authority` -- governance address

---

## gov

### zerone.gov.lip_submitted
LIP (Living Improvement Proposal) created.
- `lip_id` -- LIP identifier (e.g. `LIP-42`)
- `proposer` -- proposer address
- `category` -- proposal category
- `initial_stake` -- initial stake amount

### zerone.gov.lip_staked
Stake added to LIP.
- `lip_id` -- LIP identifier
- `staker` -- staker address
- `amount` -- staked amount
- `new_stage` -- stage after staking
- `total_staked` -- cumulative stake

### zerone.gov.lip_stage_advanced
LIP manually advanced by proposer.
- `lip_id` -- LIP identifier
- `authority` -- proposer address
- `new_stage` -- new stage

### zerone.gov.vote_cast
Vote cast on LIP in voting stage.
- `lip_id` -- LIP identifier
- `voter` -- voter address
- `option` -- `yes`, `no`, or `abstain`
- `weight` -- stake-weighted vote power

### zerone.gov.lip_withdrawn
LIP withdrawn by proposer.
- `lip_id` -- LIP identifier
- `proposer` -- proposer address

### zerone.gov.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.gov.lip_stage_transition
*BeginBlock.* Automatic LIP stage transition.
- `lip_id` -- LIP identifier
- `from_stage` -- previous stage
- `to_stage` -- new stage
- `block_height` -- transition block
- `voting_end_block` -- (only when entering voting) voting deadline

### zerone.gov.lip_tallied
*BeginBlock.* LIP voting period ended, tally computed.
- `lip_id` -- LIP identifier
- `outcome` -- `passed` or `failed`
- `yes_stake` -- total yes stake
- `no_stake` -- total no stake
- `abstain_stake` -- total abstain stake
- `unique_voters` -- number of unique voters
- `quorum_met` -- `"true"` or `"false"`

### zerone.gov.research_spend_submitted
Research spend proposal submitted.
- `proposal_id` -- proposal ID
- `proposer` -- proposer address
- `amount` -- requested amount
- `recipient` -- recipient address

### zerone.gov.research_spend_voted
Research spend proposal voted.
- `proposal_id` -- proposal ID
- `voter` -- voter address
- `vote` -- vote option
- `stage` -- proposal stage

### zerone.gov.research_voters_set
Research voter set updated.
- `authority` -- governance address
- `voter1` -- first voter
- `voter2` -- second voter

---

### zerone.gov.community_seat_expired
Community seat term ended.
- `previous_holder` -- outgoing seat holder address
- `seat_index` -- seat position index

### zerone.gov.community_seat_installed
New community seat holder installed.
- `address` -- new seat holder address
- `seat_index` -- seat position index
- `term_end_block` -- block height when term ends

### zerone.gov.community_seat_removed
Community seat holder removed before term end.
- `reason` -- removal reason
- `removed_address` -- removed seat holder address
- `seat_index` -- seat position index

### zerone.gov.community_seat_vacant
Community seat declared vacant.
- `phase` -- current governance phase
- `seat_index` -- seat position index

### zerone.gov.param_change_applied
Governance parameter change successfully applied from LIP.
- `key` -- parameter key
- `lip_id` -- originating LIP identifier
- `module` -- target module
- `value` -- new parameter value

### zerone.gov.param_change_failed
Governance parameter change from LIP failed to apply.
- `key` -- parameter key
- `lip_id` -- originating LIP identifier
- `module` -- target module
- `reason` -- failure reason

### zerone.gov.phase_transition_cancelled
Governance phase transition proposal cancelled.
- `lip_id` -- originating LIP identifier
- `reason` -- cancellation reason

### zerone.gov.phase_transition_passed
Governance phase transition approved and scheduled.
- `activation_block` -- block height for activation
- `is_rollback` -- `"true"` or `"false"`
- `lip_id` -- originating LIP identifier
- `target_phase` -- destination governance phase

### zerone.gov.research_fund_phase_rollback
Research fund governance phase rolled back.
- `block` -- current block height
- `cooldown_until` -- block height until which rollback cooldown applies
- `from_phase` -- phase before rollback
- `to_phase` -- phase after rollback

### zerone.gov.research_fund_phase_transition
Research fund governance phase transitioned.
- `block` -- current block height
- `block_height` -- transition block height
- `from_phase` -- previous phase
- `to_phase` -- new phase

### zerone.gov.seat_election_contested_resolved
Contested seat election resolved with a winner.
- `seat_index` -- seat position index
- `winner` -- winning candidate address
- `winner_stake` -- winning candidate stake

### zerone.gov.seat_election_nominated
Candidate nominated for community seat election.
- `candidate` -- nominee address
- `proposal_id` -- election proposal identifier
- `proposer` -- nominator address
- `seat_index` -- seat position index

### zerone.gov.seat_election_runoff_created
Runoff election created between top two candidates.
- `candidate_1` -- first candidate address
- `candidate_2` -- second candidate address
- `runoff_proposal_1` -- first runoff proposal ID
- `runoff_proposal_2` -- second runoff proposal ID
- `seat_index` -- seat position index

### zerone.gov.seat_election_tallied
Seat election votes tallied.
- `no_stake` -- total stake against
- `outcome` -- election result
- `proposal_id` -- election proposal identifier
- `yes_stake` -- total stake in favour

### zerone.gov.seat_election_voted
Vote cast in a seat election.
- `option` -- vote option
- `proposal_id` -- election proposal identifier
- `stake` -- voter stake weight
- `voter` -- voter address

### zerone.gov.seat_election_voting_started
Seat election voting period opened.
- `proposal_id` -- election proposal identifier
- `voting_end_block` -- block height when voting closes

### zerone.gov.seat_nomination_accepted
Seat nomination accepted by candidate.
- `candidate` -- candidate address
- `discussion_end_block` -- block height when discussion period ends
- `proposal_id` -- election proposal identifier

### zerone.gov.seat_nomination_expired
Seat nomination expired without acceptance.
- `candidate` -- candidate address
- `proposal_id` -- election proposal identifier

### zerone.gov.upgrade_plan_attached
Software upgrade plan attached to a LIP.
- `height` -- scheduled upgrade height
- `lip_id` -- originating LIP identifier
- `upgrade_name` -- upgrade plan name

### zerone.gov.upgrade_scheduled
Software upgrade scheduled for execution.
- `height` -- scheduled upgrade height
- `lip_id` -- originating LIP identifier
- `upgrade_name` -- upgrade plan name

### zerone.gov.expedited_voting
LIP voting period shortened in response to system health category (R31-2).
- `lip_id` -- LIP identifier
- `target_modules` -- comma-separated target modules
- `health_category` -- system health category at scheduling time
- `base_voting_period` -- base voting period in blocks
- `effective_voting_period` -- expedited voting period in blocks

### zerone.gov.domain_formation_freeze
Governance froze new partnership/domain formation in a domain (R31-3 Earth stability).
- `domain` -- frozen domain
- `duration_blocks` -- freeze duration in blocks
- `expiry_height` -- height at which the freeze expires
- `reason` -- governance-supplied reason


## home

### zerone.home.home_created
Agent home created.
- `home_id` -- home identifier
- `owner` -- owner address
- `name` -- home name

### zerone.home.home_updated
Home name or status updated.
- `home_id` -- home identifier
- `owner` -- owner address
- `status` -- new status

### zerone.home.memory_cid_updated
IPFS memory CID updated.
- `home_id` -- home identifier
- `owner` -- owner address
- `cid` -- new IPFS CID

### zerone.home.session_started
Session started for registered key.
- `home_id` -- home identifier
- `session_id` -- session identifier
- `key_hash` -- key hash

### zerone.home.session_ended
Session ended.
- `home_id` -- home identifier
- `session_id` -- session identifier

### zerone.home.key_registered
Key registered to home.
- `home_id` -- home identifier
- `owner` -- owner address
- `key_hash` -- key hash
- `key_type` -- key type
- `role` -- key role

### zerone.home.key_revoked
Key revoked and sessions terminated.
- `home_id` -- home identifier
- `owner` -- owner address
- `key_hash` -- revoked key hash

### zerone.home.guardian_configured
Guardian configuration updated.
- `home_id` -- home identifier
- `owner` -- owner address
- `defense_strategy` -- defense strategy
- `auto_defend` -- auto-defend enabled

### zerone.home.alert_acknowledged
Alert acknowledged by owner or guardian.
- `home_id` -- home identifier
- `alert_id` -- alert identifier
- `signer` -- acknowledger address

### zerone.home.spending_limit_set
Spending limit configured.
- `home_id` -- home identifier
- `owner` -- owner address
- `key_type` -- key type
- `max_amount` -- maximum amount per period
- `period_blocks` -- period length in blocks

---

### zerone.home.params_updated
Governance parameter update.
- `authority` -- governance address


## ibcratelimit

### zerone.ibcratelimit.rate_limit_added
IBC rate limit added.
- `channel_id` -- IBC channel ID
- `denom` -- denomination
- `max_send` -- max send per window
- `max_recv` -- max receive per window
- `window_blocks` -- window length in blocks

### zerone.ibcratelimit.rate_limit_removed
IBC rate limit removed.
- `channel_id` -- IBC channel ID
- `denom` -- denomination

### zerone.ibcratelimit.params_updated
Governance parameter update.
- `authority` -- governance address

---

## icaauth

### zerone.icaauth.account_registered
Interchain account registered.
- `owner` -- owner address
- `connection_id` -- IBC connection ID
- `port_id` -- ICA port ID

### zerone.icaauth.tx_submitted
Interchain transaction submitted.
- `owner` -- owner address
- `connection_id` -- IBC connection ID
- `msg_count` -- number of messages

### zerone.icaauth.params_updated
Governance parameter update.
- `authority` -- governance address

---

## inquiry

### zerone.inquiry.submitted
Open inquiry submitted — chain creates demand for unmapped territory (Creed Commitment 16).
- `inquiry_id` -- inquiry ID
- `asker` -- asker address
- `domain` -- knowledge domain
- `bounty` -- bounty amount (uzrn)
- `expires_at_block` -- expiry block height
- `creed_commitment` -- always `"16"`

### zerone.inquiry.answer_submitted
Answer linked to an open inquiry.
- `answer_id` -- sequential answer ID
- `inquiry_id` -- inquiry ID
- `answerer` -- answerer address
- `claim_id` -- linked claim ID
- `creed_commitment` -- always `"16"`

### zerone.inquiry.cancelled
Inquiry cancelled by the asker (only allowed while no answers are in flight).
- `inquiry_id` -- inquiry ID
- `asker` -- asker address
- `creed_commitment` -- always `"16"`

### zerone.inquiry.resolved
Inquiry resolved — bounty paid to the answerer whose claim was accepted.
- `inquiry_id` -- inquiry ID
- `winner` -- winning answerer address
- `winning_fact_id` -- accepted fact ID
- `bounty` -- bounty amount paid (uzrn)
- `resolved_at_block` -- resolution block height
- `creed_commitment` -- always `"16"`

### zerone.inquiry.expired
Inquiry expired without resolution — bounty refunded to asker.
- `inquiry_id` -- inquiry ID
- `asker` -- asker address
- `resolved_at_block` -- expiry block height
- `creed_commitment` -- always `"16"`

---

## knowledge

### zerone.knowledge.submit_claim
Knowledge claim submitted for verification.
- `claim_id` -- claim identifier
- `submitter` -- submitter address
- `domain` -- knowledge domain
- `review_fee` -- bonded stake (review fee amount, in uzrn)
- `content_hash` -- claim content hash
- `claim_type` -- claim type enum string (e.g. `CLAIM_TYPE_ASSERTION`)
- `sponsored` -- `"true"` if the bootstrap fund covered the fee, `"false"` otherwise

### zerone.knowledge.submit_commitment
Verifier commitment submitted (commit-reveal).
- `round_id` -- verification round ID
- `verifier` -- verifier address
- `committed_at_block` -- commitment block

### zerone.knowledge.submit_reveal
Verifier reveal submitted.
- `round_id` -- verification round ID
- `verifier` -- verifier address
- `vote` -- revealed vote
- `revealed_at_block` -- reveal block

### zerone.knowledge.add_fact
Fact added directly by authority.
- `fact_id` -- fact identifier
- `authority` -- authority address
- `domain` -- knowledge domain
- `category` -- fact category
- `status` -- fact status

### zerone.knowledge.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.knowledge.update_extended_params
Extended parameter update.
- `authority` -- governance address

### zerone.knowledge.challenge_fact
Verified fact challenged, new verification round created.
- `fact_id` -- fact identifier
- `challenger` -- challenger address
- `round_id` -- new verification round ID
- `stake` -- challenge stake
- `reason` -- challenge reason

### zerone.knowledge.challenge_provisional_fact
Provisional fact challenged.
- `fact_id` -- fact identifier
- `challenger` -- challenger address
- `challenge_id` -- challenge identifier
- `stake` -- challenge stake
- `reason` -- challenge reason

### zerone.knowledge.submit_contradiction
Contradiction submitted against a fact.
- `fact_id` -- contradicted fact ID
- `submitter` -- submitter address
- `counter_claim_id` -- counter-claim ID
- `domain` -- knowledge domain
- `stake` -- bonded stake

### zerone.knowledge.domain_proposed
New knowledge domain proposed.
- `name` -- domain name
- `proposer` -- proposer address

### zerone.knowledge.endorse_domain_proposal
Domain proposal endorsed.
- `proposal_id` -- proposal ID
- `endorser` -- endorser address
- `endorser_count` -- total endorsements
- `status` -- proposal status

### zerone.knowledge.domain_proposal_challenged
Domain proposal challenged.
- `domain` -- challenged domain
- `challenger` -- challenger address
- `reason` -- challenge reason

### zerone.knowledge.stratum_registered
Knowledge stratum registered.
- `name` -- stratum name
- `confidence_ceiling` -- maximum confidence (BPS)

### zerone.knowledge.patronize_fact
Fact patronized (funding for continued availability).
- `fact_id` -- fact identifier
- `patron` -- patron address
- `amount` -- patronage amount
- `duration_blocks` -- patronage duration
- `expiry_block` -- patronage expiry block

### zerone.knowledge.propose_research_fund
Research fund proposal created.
- `proposal_id` -- proposal ID
- `proposer` -- proposer address
- `title` -- proposal title
- `amount` -- requested amount
- `recipient` -- recipient address
- `voting_end_block` -- voting deadline

### zerone.knowledge.vote_research_proposal
Vote cast on research fund proposal.
- `proposal_id` -- proposal ID
- `voter` -- voter address
- `vote` -- vote option

### zerone.knowledge.research_proposal_executed
Research fund proposal executed.
- `proposal_id` -- proposal ID

### zerone.knowledge.verification_round_created
*BeginBlock.* New verification round created.
- `round_id` -- round identifier
- `claim_id` -- claim being verified
- `phase` -- initial phase (`COMMIT`)

### zerone.knowledge.verification_round_completed
*BeginBlock.* Verification round completed with verdict.
- `round_id` -- round identifier
- `claim_id` -- verified claim ID
- `verdict` -- round verdict

### zerone.knowledge.fact_created
*BeginBlock.* New fact created from verified claim.
- `fact_id` -- fact identifier
- `claim_id` -- source claim ID
- `domain` -- knowledge domain
- `confidence` -- confidence score (BPS)

### zerone.knowledge.round_phase_changed
*BeginBlock.* Verification round phase transitioned.
- `round_id` -- round identifier
- `phase` -- new phase (`REVEAL` or `AGGREGATION`)
- `reveal_count` -- (AGGREGATION only) number of reveals

### zerone.knowledge.round_expired
*BeginBlock.* Verification round expired with insufficient reveals.
- `round_id` -- round identifier
- `reveals` -- number of reveals received

### zerone.knowledge.constraint_violation_detected
*Tx (MsgSubmitClaim).* A new claim's canonical form directly contradicts a
verified fact. Only emitted when `Params.constraint_check_enabled = true` and
the claim has a canonical form whose logical inverse (`assert` ↔ `negate` with
identical domain/subject/predicate/scope) matches an active Fact's canonical
hash. Detection is informational — the claim proceeds to PoT regardless.
- `conflicting_fact_id` -- ID of the existing verified fact that contradicts this claim
- `opposing_canonical_form` -- the normalized opposing form that matched (e.g. `negate(...)`)
- `submitter` -- address of the claim submitter
- `domain` -- knowledge domain of the claim

---

### zerone.knowledge.claim_suspended
*Tx (MsgSuspendClaim).* Governance places a claim into CLAIM_STATUS_SUSPENDED —
a Bohmian mirror-state where the claim is held visible without being acted upon.
No verification round may be opened on a SUSPENDED claim. Emitted only when
`Params.suspension_enabled = true`. (Bohm Dialogue Gap 1.)
- `claim_id` -- the suspended claim ID
- `authority` -- governance authority address that performed the suspension
- `reason` -- the assumption being held in mirror-state (required non-empty)

### zerone.knowledge.claim_resumed
*Tx (MsgResumeClaim).* Governance lifts a SUSPENDED claim back to
CLAIM_STATUS_IN_VERIFICATION, allowing a new verification round to be opened.
Clears `suspended_at_block` and `suspension_reason`. (Bohm Dialogue Gap 1.)
- `claim_id` -- the resumed claim ID
- `authority` -- governance authority address that performed the resumption

### zerone.knowledge.ad_hoc_rescue_detected
*Tx (MsgSubmitClaim).* A rescue claim's `RefutationConditions` are not a
proper superset of the rescued (rejected) claim's conditions — Popper's
criterion for ad-hoc theory modification. Only emitted when
`MsgSubmitClaim.rescues_claim_id` is set, the referenced claim is REJECTED,
and the new conditions fail the superset test. Detection is informational —
`Claim.is_potentially_ad_hoc=true` is set, but the claim proceeds to PoT.
- `claim_id` -- the new rescue claim ID
- `rescues_claim_id` -- the rejected claim being rescued
- `submitter` -- address of the rescue submitter
- `new_refutation_conditions` -- count of conditions on the new claim
- `popper_criterion` -- human-readable description of the criterion

### zerone.knowledge.bootstrap_sponsored
Claim sponsored via bootstrap fund (gas-free).
- `address_claims_used` -- number of claims used by this address
- `claim_id` -- sponsored claim identifier
- `fee_amount` -- fee deducted from bootstrap fund
- `fund_balance_after` -- bootstrap fund balance after sponsorship
- `submitter` -- claim submitter address

### zerone.knowledge.bounty_claimed
Knowledge bounty successfully claimed by a fact submission.
- `bounty_id` -- bounty identifier
- `domain` -- knowledge domain
- `fact_id` -- fulfilling fact identifier
- `reward_amount` -- reward paid
- `subject` -- bounty subject
- `submitter` -- claimant address

### zerone.knowledge.bounty_created
New knowledge bounty posted for a domain.
- `bounty_id` -- bounty identifier
- `demand_count` -- number of demand reports aggregated
- `domain` -- knowledge domain
- `reward_amount` -- reward offered
- `subject` -- bounty subject

### zerone.knowledge.bounty_expired
Knowledge bounty expired without fulfilment.
- `bounty_id` -- bounty identifier
- `domain` -- knowledge domain
- `returned_amount` -- reward returned to creator
- `subject` -- bounty subject

### zerone.knowledge.common_knowledge_added
Common knowledge entry registered for a domain.
- `domain` -- knowledge domain
- `id` -- common knowledge entry identifier
- `penalty_bps` -- novelty penalty in basis points for matching claims
- `subject` -- common knowledge subject

### zerone.knowledge.common_knowledge_removed
Common knowledge entry removed from a domain.
- `domain` -- knowledge domain
- `id` -- common knowledge entry identifier
- `subject` -- common knowledge subject

### zerone.knowledge.conformity_alert
Epistemic diversity alert: domain showing excessive conformity.
- `avg_entropy` -- average entropy score
- `consecutive_epochs` -- number of consecutive low-diversity epochs
- `domain` -- knowledge domain
- `threshold` -- conformity alert threshold

### zerone.knowledge.demand_reported
User reported demand for knowledge in a topic.
- `report_count` -- total reports from this reporter
- `reporter` -- reporter address

### zerone.knowledge.domain_pressure_changed
Carrying capacity pressure updated for a domain.
- `active_count` -- number of active facts
- `capacity` -- domain carrying capacity
- `category` -- pressure category
- `domain` -- knowledge domain
- `pressure_bps` -- pressure in basis points

### zerone.knowledge.duplicate_subject_warning
Claim subject matches an existing fact.
- `existing_fact_id` -- identifier of the existing fact
- `subject` -- duplicate subject text

### zerone.knowledge.epistemic_temperature_changed
Domain epistemic temperature recalculated.
- `category` -- temperature category (hot/neutral/cold)
- `conformity_streak` -- consecutive conformity epochs
- `domain` -- knowledge domain
- `recent_vindications` -- recent vindication count
- `temperature_bps` -- temperature in basis points

### zerone.knowledge.fact_disproven
Established fact disproven via vindication.
- `challenge_claim_id` -- claim ID that disproved the fact
- `disproven_by` -- address that submitted disproving claim
- `fact_id` -- disproven fact identifier

### zerone.knowledge.fact_rated
User rated a fact as useful or not.
- `fact_id` -- rated fact identifier
- `rater` -- rater address
- `useful` -- `"true"` or `"false"`

### zerone.knowledge.fact_relation_created
Semantic relation created between two facts.
- `relation` -- relation type
- `source` -- source fact identifier
- `target` -- target fact identifier

### zerone.knowledge.fact_status_changed
Fact lifecycle status changed (e.g. active to dormant).
- `energy` -- current metabolism energy
- `epoch` -- metabolism epoch
- `fact_id` -- fact identifier
- `new_status` -- new lifecycle status
- `old_status` -- previous lifecycle status
- `reason` -- reason for status change

### zerone.knowledge.fitness_updated
Fact fitness score recalculated.
- `epoch` -- metabolism epoch
- `fact_id` -- fact identifier
- `fitness_label` -- fitness category label
- `fitness_score` -- numeric fitness score
- `query_count_epoch` -- queries received this epoch

### zerone.knowledge.lineage_cascade
Disproven parent fact triggered cascade to child facts.
- `child_at_risk` -- `"true"` or `"false"`
- `child_energy` -- child fact current energy
- `parent_disproven` -- disproven parent fact identifier

### zerone.knowledge.lineage_created
Fact lineage relationship established (child inherits from parent).
- `child_fact_id` -- child fact identifier
- `inherited_fitness` -- fitness inherited from parent
- `lineage_depth` -- depth in lineage tree
- `parent_fact_id` -- parent fact identifier

### zerone.knowledge.lineage_royalty
Royalty payment distributed up the lineage tree.
- `ancestor_fact_id` -- ancestor fact identifier
- `ancestor_submitter` -- ancestor submitter address
- `child_fact_id` -- child fact identifier
- `depth` -- lineage depth from child
- `royalty_amount` -- royalty amount paid

### zerone.knowledge.niche_challenger
New fact challenges the current niche leader.
- `current_leader` -- current niche leader fact ID
- `domain` -- knowledge domain
- `new_fact` -- challenging fact ID
- `niche` -- niche key

### zerone.knowledge.niche_displacement
Niche leader displaced by a fitter fact.
- `displaced_fact` -- displaced leader fact ID
- `domain` -- knowledge domain
- `new_leader` -- new niche leader fact ID
- `niche_key` -- niche identifier

### zerone.knowledge.niche_pruned
Low-fitness fact pruned from niche.
- `fitness` -- fact fitness score at pruning
- `niche_key` -- niche identifier
- `niche_leader_id` -- niche leader fact ID
- `pruned_fact_id` -- pruned fact identifier

### zerone.knowledge.novelty_scored
Claim novelty assessment completed.
- `common_knowledge_match` -- `"true"` or `"false"` — matches common knowledge
- `fact_id` -- assessed fact identifier
- `novelty_score` -- computed novelty score

### zerone.knowledge.qualification_fallback
Insufficient qualified verifiers for a domain; fallback used.
- `domain` -- knowledge domain
- `min_verifiers` -- minimum verifiers required
- `qualified_count` -- number of qualified verifiers found

### zerone.knowledge.review_fee_distributed
Claim review fee split across protocol components using governance-configured BPS params.
All share amounts are computed at submission time from the current `Params` values;
a governance vote changing the split takes effect on the next claim submission.
- `claim_id` -- claim identifier
- `development` -- development fund share (computed as `stake × DevelopmentFundBps / 1_000_000`)
- `fee_amount` -- total review fee (the claim stake, in uzrn)
- `protocol` -- protocol treasury share (computed as `stake × ProtocolTreasuryBps / 1_000_000`)
- `research` -- research fund share (the rounding remainder after burn + verifier pool + protocol + development)
- `verifier_pool` -- verifier pool share (computed as `stake × VerifierPoolBps / 1_000_000`; held in knowledge module account and distributed to correct verifiers at round completion)

### zerone.knowledge.role_elasticity_updated
Human/agent role elasticity recalculated for a domain.
- `agent_accuracy_bps` -- agent verification accuracy in BPS
- `agent_bonus_bps` -- agent vote weight bonus in BPS
- `domain` -- knowledge domain
- `human_accuracy_bps` -- human verification accuracy in BPS
- `human_bonus_bps` -- human vote weight bonus in BPS

### zerone.knowledge.vindication_executed
Vindication process completed; majority slashed, minority rewarded.
- `bonus_pool` -- total bonus pool distributed
- `disproven_by` -- address that disproved the fact
- `fact_id` -- vindicated fact identifier
- `majority_slashed` -- total stake slashed from majority
- `minority_count` -- number of minority verifiers rewarded

### zerone.knowledge.vindication_expired
Vindication window expired without resolution.
- `entry_count` -- number of entries in vindication queue
- `fact_id` -- fact identifier

### zerone.knowledge.capacity_penalty_applied
Domain carrying capacity reduced by capture-defense penalty (R31-1 Metal→Wood).
- `domain` -- affected domain
- `base_capacity` -- pre-penalty capacity
- `effective_capacity` -- post-penalty capacity
- `capture_penalty_bps` -- capture penalty in BPS (HHI)
- `reason` -- always `capture_flagged`

### zerone.knowledge.stratum_capacity_applied
Stratum-depth multiplier applied to a domain's carrying capacity (R31-4).
- `domain` -- affected domain
- `stratum_depth` -- ontology depth
- `capacity_multiplier_bps` -- multiplier applied in BPS
- `effective_capacity` -- capacity after multiplier

### zerone.knowledge.mentorship_dividend_applied
Domain energy and capacity boosted by a mentorship graduation (R31-5 Water→Wood).
- `domain` -- target domain
- `mentor` -- mentor address
- `mentee` -- mentee address
- `energy_added` -- energy added to domain
- `total_energy` -- domain total energy after dividend
- `graduations` -- cumulative graduation count

### zerone.knowledge.contradiction_reversed
Target fact restored from CONTESTED to VERIFIED after the contradicting claim failed and no other live contradictions remain.
- `fact_id` -- fact whose status was restored
- `reverted_by_claim` -- rejected/malformed/inconclusive claim whose side-effect was reversed

### zerone.knowledge.confidence_clamped_to_floor
On fact creation, the new fact's confidence was capped to its `dependency_confidence_floor` (ToK Wave 2). Emitted when a proof chain inherits a weaker ceiling than its own verification would give it.
- `fact_id` -- newly created fact
- `dependency_floor_bps` -- inherited floor in BPS
- `axiom_distance` -- minimum hops to a genesis axiom

### zerone.knowledge.falsification_cascade
A direct descendant of a disproven fact was flipped from VERIFIED/ACTIVE/AT_RISK to CONTESTED (ToK Wave 5). Emitted once per affected descendant.
- `descendant_fact_id` -- fact whose status was set to CONTESTED
- `disproven_fact_id` -- the fact that was disproven
- `challenge_claim_id` -- the challenge claim that triggered disproof
- `edge_relation` -- which support-bearing edge linked them (e.g. `RELATION_TYPE_REQUIRES`)

### zerone.knowledge.falsification_cascade_summary
Emitted once at the end of a falsification cascade with the total count of affected descendants (ToK Wave 5).
- `disproven_fact_id` -- the fact that was disproven
- `descendants_contested` -- how many direct descendants were flipped

### zerone.knowledge.corroboration_incremented
Popperian survival counter incremented: a fact withstood a falsification attempt (Phase 2). The fact's `corroboration_count` is epistemically meaningful in a way `confidence` is not — it names the tests the claim has already passed.
- `fact_id` -- fact that survived the challenge
- `challenge_claim_id` -- the (rejected) challenge claim
- `new_count` -- corroboration_count after increment
- `block_height` -- height at which the challenge was resolved

### zerone.knowledge.add_fact_proposed
Wave 16 guardian-veto path. Authority called MsgAddFact while a guardian set is configured and the veto window is positive — instead of materializing the fact immediately, the proposal is queued. Guardians have until `execute_at_block` to call MsgVetoFactInjection. Without veto, the BeginBlocker emits `pending_fact_materialized` when the window closes.
- `pending_id` -- id of the queued PendingFactInjection
- `authority` -- the proposing authority
- `domain`, `category` -- mirrors the proposed fact
- `execute_at_block` -- block at which the proposal materializes if not vetoed

### zerone.knowledge.fact_injection_vetoed
A registered guardian invoked MsgVetoFactInjection during the veto window. The pending fact is removed from the queue; the privileged-action log records the cancellation. The chain no longer relies on a single key being honest for the only path that bypasses the verifier panel.
- `pending_id` -- the cancelled PendingFactInjection id
- `guardian` -- guardian address that cast the veto (must appear in `Params.guardian_addresses`)
- `proposer` -- the original authority that proposed the injection
- `reason` -- audit comment

### zerone.knowledge.pending_fact_materialized
BeginBlocker emitted when a queued fact-injection's veto window expires without a veto. The actual Fact record is written at this point with `status=VERIFIED` (matching the immediate-AddFact path semantics).
- `fact_id` -- id of the now-existing fact
- `proposer` -- the originating authority
- `domain`, `category` -- fact metadata
- `proposed_at_block`, `materialized_at_block` -- proposal vs materialization heights

### zerone.knowledge.invitation_bonus_paid
Emitted when the probe bounty pool pays the flat `InvitationBonusAmount` to a prober who answered a chain-issued stress-test invitation. Fires on any verdict — the chain pays for showing up, not only for winning. Invitation was "current" (fact's `ProbeInvitedAtBlock > 0` and `LastCorroboratedBlock ≤ ProbeInvitedAtBlock`). Converts invitations from demand signals into standing offers.
- `claim_id` -- the challenge claim that answered the invitation
- `challenger` -- challenger address (bonus recipient)
- `fact_id` -- target fact whose invitation was answered
- `amount` -- uzrn paid from the pool (may be less than `InvitationBonusAmount` if the pool is underfunded)

### zerone.knowledge.probe_bounty_minted
Emitted each block that the Wave 15 BeginBlocker mints uzrn into the dedicated probe bounty pool (`knowledge_probe_bounty_pool` module account). The pool funds successful-probe bonuses via `PayProbeBountyFromPool` — decoupling epistemic-auditing budget from general governance. Minting throttles when the pool reaches `ProbeBountyMaxPoolSize`; the event carries the actual minted amount (may be less than `ProbeBountyMintPerBlock` when the cap clamps issuance).
- `amount` -- uzrn minted this block
- `block` -- block height

### zerone.knowledge.probe_invited
Emitted by the Wave 15 stress-test invitation heartbeat. Each block the chain scans for high-confidence facts that have gone idle longer than `ProbeInvitationIdleThresholdBlocks` and nominates them for external probing. The chain manufactures demand for its own epistemic audit — truth stands firm under challenge because of its nature, and the architecture now actively invites challenge rather than waiting for it.
- `fact_id` -- the fact being nominated for stress-testing
- `domain` -- fact domain (enables domain-scoped prober subscription)
- `confidence` -- current confidence (BPS)
- `corroboration_count` -- survived attacks so far
- `idle_since_block` -- block height of the last probe (or acceptance if never probed)
- `invited_at_block` -- current block height

### zerone.knowledge.challenge_settled
Emitted when a challenge claim's stake is settled after the verification round completes. The verifier reward pool (55%) is distributed separately via `verifier_reward`; this event covers the remaining 45% of the challenger's stake.

Wave 14c inverted the challenge economics to stress-test truth instead of shielding it. Wave 14d added a probe-participation reward so even failed probes earn something — encouraging continuous audit of high-confidence claims.

- `claim_id` -- the challenge claim id
- `challenger` -- challenger address
- `outcome` -- "accepted" (challenge succeeded, fact disproven) or "rejected" (challenge failed)
- `refund` -- uzrn returned to the challenger (accepted path only)
- `reward_bps` -- amplified success bonus; scales with the disproven fact's confidence, peaking at (base + 200%) for max-confidence disproofs (accepted path only)
- `participation_refund` -- 15% of the stake returned to the challenger on failed probes; the chain thanks every audit attempt, not only successful disproofs (rejected path only)
- `to_treasury` -- remainder routed to protocol treasury on failed probes (rejected path only)

### zerone.knowledge.agent_calibration_updated
Submitter's track record changed — Phase 5 feedback loop. Emitted after round outcomes, corroborations earned, and disprovals. Closes the loop between training-pipeline output and on-chain evaluation.
- `address` -- submitter address
- `account_type` -- "agent" / "human" / "hybrid"
- `total_submissions` -- lifetime submission count
- `accepted` -- lifetime accepted count
- `disproven_count` -- facts disproved post-acceptance
- `corroborations_earned` -- sum across submitter's facts
- `calibration_score_bps` -- computed score in BPS

### zerone.knowledge.model_card_registered
New ModelCard stored (Route B). A trained model has been registered against its TrainingPipeline, naming its deployment agent account and initial eval scores.
- `model_id` -- stable model identifier
- `pipeline_id` -- TrainingPipeline that produced this model
- `route` -- "openweight_fine_tune" | "from_scratch" | "distilled"
- `deployment_address` -- agent account the model runs as
- `owner_address` -- model owner

### zerone.knowledge.model_card_updated
ModelCard re-written (Route B) — eval updates or metadata amendment. Attributes mirror model_card_registered.
- `model_id`, `pipeline_id`, `route`, `deployment_address`, `owner_address`

### zerone.knowledge.model_card_retired
ModelCard flipped to inactive (Route B). Emitted when a model is formally retired from active deployment.
- `model_id`, `pipeline_id`, `route`, `deployment_address`, `owner_address`

### zerone.knowledge.training_pipeline_registered
New training pipeline declared on-chain (Route B Wave 2b). An operator has pinned a corpus snapshot, tokenizer version, and recipe hash for a new training run.
- `pipeline_id`, `operator`, `corpus_snapshot_height`, `tokenizer_version`, `recipe_hash`

### zerone.knowledge.training_pipeline_updated
Training pipeline status or completion amended by its operator (Route B Wave 2b).
- `pipeline_id`, `operator`, `new_status`

### zerone.knowledge.tokenizer_spec_amended
Governance amended the on-chain tokenizer contract (Route B Wave 3a). The caller-supplied version field is ignored; the handler auto-assigns monotonic `new_version = current+1` and pins `ratified_at_block`.
- `new_version` -- auto-assigned version (current+1)
- `canonical_serialisation_version` -- canonical serialisation version of the new spec
- `authority` -- governance authority address that submitted the amendment

### zerone.knowledge.contributions_attributed
Model owner posted the fact_ids consumed by training, creating the reverse fact→model index (Route B Wave 3b). `total_weight` sums per-fact (corroboration_count + 1) with an optional override.
- `model_id` -- ModelCard being attributed
- `attributed_by` -- owner (must equal ModelCard.owner_address)
- `fact_count` -- deduplicated fact count actually recorded
- `total_weight` -- sum of per-fact weights

### zerone.knowledge.training_attestation_posted
Pipeline operator attested training completion with off-chain telemetry (Route B Wave 3c).
- `pipeline_id` -- TrainingPipeline id
- `attester` -- operator address (must equal pipeline operator)
- `flops_estimate` -- best-effort FLOPs count
- `wallclock_seconds` -- wallclock training time
- `eval_hash` -- sha256 of the evaluation bundle

### zerone.knowledge.augmentation_bounty_created
Sponsor opened a bounty for variant reformulations of a target fact (Route B Wave 3e). Up to `max_variants` accepted variants can be paid out.
- `bounty_id` -- stable id
- `sponsor` -- sponsor address
- `target_fact_id` -- the fact whose variants are wanted
- `reward_per_variant` -- payout per accepted variant (uzrn)
- `max_variants` -- hard cap before the bounty auto-deactivates

### zerone.knowledge.augmentation_submitted
A variant was submitted against an augmentation bounty or as a volunteer (Route B Wave 3e). `bounty_id` is empty for volunteer variants.
- `augmentation_id` -- stable id
- `original_fact_id` -- the fact being reformulated
- `bounty_id` -- bounty id (empty for volunteer)
- `submitter` -- submitter address

### zerone.knowledge.augmentation_accepted
Variant accepted (Route B Wave 3e; Wave 4 realignment). Under Wave 4 the acceptance path for bounty augmentations is the finalized verifier-panel verdict (EQUIVALENT or SUPERIOR); this event still fires but the acceptor may be a system-level marker. Volunteer augmentations continue to route through the original fact submitter.
- `augmentation_id`
- `original_fact_id`
- `bounty_id` -- empty for volunteer acceptance
- `acceptor` -- sponsor or original fact submitter

### zerone.knowledge.augmentation_vote_cast
A verifier cast a verdict on a reformulation round (Route B Wave 4d). Sponsor and submitter are rejected with an error and never appear here. `finalized=true` means this vote pushed the panel past the consensus threshold.
- `augmentation_id`
- `verifier` -- voting verifier address
- `vote` -- one of PENDING / EQUIVALENT / SUPERIOR / INFERIOR / DRIFT
- `finalized` -- boolean: did this vote finalize the verdict?

### zerone.knowledge.augmentation_verdict_finalized
The verifier panel reached consensus on a reformulation (Route B Wave 4d). For EQUIVALENT/SUPERIOR the handler releases escrow to the submitter and writes a REFORMULATES relation; for DRIFT/INFERIOR the variant is archived for the drift corpus. Never fired by the sponsor or submitter directly.
- `augmentation_id`
- `original_fact_id`
- `verdict` -- final panel verdict
- `payout` -- uzrn paid (0 for DRIFT/INFERIOR/vetoed)

### zerone.knowledge.augmentation_sponsor_vetoed
Sponsor vetoed a passing verdict, forfeiting the payout amount to the research fund (Route B Wave 4d). The only sponsor-held lever — deliberately costly so it cannot silently block legitimate variants.
- `augmentation_id`
- `sponsor`
- `forfeited_amount` -- uzrn sent to the research fund
- `reason` -- free-form rationale

### zerone.knowledge.training_revenue_clawed
Disproval clawback — future training-use revenue is cut for this fact (Route B Wave 4b). Cleared `training_revenue_earned_recent` is moved to the research fund.
- `fact_id`
- `submitter`
- `cleared_recent` -- uzrn amount cleared from the 30-epoch recent window

### zerone.knowledge.contribution_challenge_opened
A fact submitter disputed a ContributionRecord and locked a bond in the training fund (Route B Wave 4e). `dispute_type` is "missing" (omitted attribution) or "fraudulent" (listed-but-unused fact).
- `challenge_id`
- `model_id`
- `challenger`
- `dispute_type`
- `bond` -- uzrn locked

### zerone.knowledge.contribution_challenge_resolved
Governance authority resolved an attribution challenge (Route B Wave 4e). A future wave will route this through a verifier panel rather than authority; the event shape is stable.
- `challenge_id`
- `status` -- "upheld" | "rejected"
- `payout` -- uzrn sent to winner (challenger on uphold; 0 on reject)
- `resolver` -- authority address

### zerone.knowledge.training_fund_disbursed
Post-hoc, calibration-gated training-fund reward paid to a pipeline operator (Route B Wave 4f). 50% released immediately; 50% held in vesting escrow with clawback rights.
- `disbursement_id`
- `model_id`
- `claimant` -- pipeline operator
- `released` -- uzrn paid immediately
- `vesting` -- uzrn held in escrow
- `vesting_end_block` -- height at which vesting completes

### zerone.knowledge.trace_schema_amended
Governance amended the MethodologyApplicationTrace serialisation contract (Route B Wave 5). Version auto-increments; caller-supplied version is ignored. The JSON Schema hash is computed by the handler when absent.
- `new_version` -- auto-assigned (current+1)
- `json_schema_hash` -- SHA-256 of the canonical JSON Schema bytes
- `authority` -- governance authority address

### zerone.knowledge.training_manifest_created
Pipeline operator materialised a DRAFT manifest by applying a CorpusSelector to current chain state (Route B Wave 7). The included ID sets are sorted and ready for Merkle commitment but the root is not yet locked.
- `manifest_id`
- `pipeline_id`
- `creator` -- pipeline operator
- `total_included` -- sum across FACTS / TRACES / PAIRS / DRIFT / NORMATIVE sets
- `tokenizer_version` -- pinned at creation
- `trace_schema_version` -- pinned at creation

### zerone.knowledge.training_manifest_finalized
Manifest Merkle root computed and committed (Route B Wave 7). Manifest transitions DRAFT → FINALIZED and becomes immutable. Clients can re-derive the root offline from the manifest's sorted ID lists.
- `manifest_id`
- `merkle_root` -- hex-encoded SHA-256 commitment
- `total_included`

### zerone.knowledge.training_manifest_attested
A FINALIZED manifest was bound to a TrainingAttestation, linking "what ran" (FLOPs + wallclock + eval hash) with "what it consumed" (Merkle-committed ID sets). Promotes the manifest to ATTESTED (Route B Wave 7).
- `manifest_id`
- `attestation_id` -- pipeline_id the attestation was keyed under
- `creator` -- pipeline operator

### zerone.knowledge.augmentation_bounty_expired
Automatic heartbeat event (Route B Wave 8). A bounty whose `expires_at_block` has passed is deactivated; the unused escrow returns to the sponsor minus the `augmentation_expiry_fee_bps` garnishment. Fires from `ProcessRouteBLifecycle` every block.
- `bounty_id`
- `sponsor`
- `refunded` -- uzrn returned to the sponsor (net of fee)
- `fee_bps` -- fee rate applied

### zerone.knowledge.training_fund_vesting_released
Automatic heartbeat event (Route B Wave 8). A disbursement whose `vesting_end_block` has arrived has its vesting portion transferred to the claimant. Idempotent: the vesting_amount is zeroed post-release so subsequent blocks don't double-credit.
- `disbursement_id`
- `model_id`
- `claimant`
- `amount` -- uzrn released from vesting escrow

### zerone.knowledge.training_manifest_superseded
Automatic heartbeat event (Route B Wave 8). An older FINALIZED manifest for a pipeline is marked SUPERSEDED when a newer FINALIZED/ATTESTED manifest exists for the same pipeline. ATTESTED manifests are never superseded.
- `manifest_id` -- the manifest being superseded
- `pipeline_id`
- `superseded_by` -- the newer manifest's id

### zerone.knowledge.incident_opened
A formal bug report was opened on-chain (Route B Wave 11). Authority-gated; stamps severity + SLA target at open time. Triage begins; no remediation yet.
- `incident_id`
- `severity` -- P0 / P1 / P2 / P3
- `title` -- one-line summary
- `sla_target_block` -- block height by which the incident should be resolved (frozen at open time)

### zerone.knowledge.incident_remediation_recorded
A remediation action was attached to an incident (Route B Wave 11). First remediation transitions the incident OPEN → MITIGATING. Subsequent remediations accrue; the lineage is append-only.
- `incident_id`
- `remediation_type` -- PARAM_AMENDMENT / NAMED_UPGRADE / EMERGENCY_HALT / EMERGENCY_RESUME / STATE_CORRECTION / SCHEMA_AMENDMENT / DOCUMENTATION
- `reference` -- mechanism-specific identifier (upgrade_name, ceremony_id, param_path, schema_version, …)
- `total_remediations` -- count after this append

### zerone.knowledge.incident_resolved
An incident transitions MITIGATING → RESOLVED (Route B Wave 11). Requires ≥1 recorded remediation; stamps post-mortem URI; records whether the SLA was met.
- `incident_id`
- `post_mortem_uri` -- IPFS or HTTPS link to post-mortem
- `sla_met` -- bool: `resolved_at_block ≤ sla_target_block`

### zerone.knowledge.incident_closed
A RESOLVED incident is permanently archived (Route B Wave 11). Terminal state; drops out of the operator dashboard but remains queryable by id.
- `incident_id`

### zerone.knowledge.module_paused
A named module's circuit breaker engaged (Route B Wave 12). Write-path handlers in that module reject until the breaker is cleared. Read-path queries remain available. Authority-gated; typically bound to an incident_id.
- `module_name`
- `reason` -- free-form; references incident_id when applicable
- `incident_id` -- empty when the pause isn't incident-driven
- `auto_unpause_at_block` -- 0 means manual unpause only

### zerone.knowledge.module_unpaused
The circuit breaker for a named module cleared (Route B Wave 12). Writes resume. Authority-gated.
- `module_name`
- `note` -- free-form; typically references completion of the fix

### zerone.knowledge.manifest_merkle_corrected
An authority-gated surgical correction recomputed and rewrote a finalized manifest's Merkle root after external-exploit corruption (Route B Wave 13). Incident-bound; requires an OPEN or MITIGATING incident at call time.
- `manifest_id`
- `incident_id` -- the audit trail binding
- `prior_root` -- what was stored before the correction
- `recomputed_root` -- the canonical root the handler derived
- `was_corrupted` -- bool: `prior_root != recomputed_root`. `false` means no-op.
- `authority` -- governance address that applied the correction

### zerone.knowledge.privileged_action_recorded
An authority-gated handler wrote an entry to the privileged-action audit log (Route B Wave 14). Fired in parallel with the action's own domain event. External monitors poll the `PrivilegedActions` query to detect anomalous bursts or unexpected invokers.
- `seq` -- monotonic sequence number
- `type` -- one of MODULE_PAUSE / MODULE_UNPAUSE / MANIFEST_CORRECT / INCIDENT_OPEN / INCIDENT_RESOLVE / INCIDENT_CLOSE / SCHEMA_AMEND_TOKENIZER / SCHEMA_AMEND_TRACE / FACT_AUTHORITY_INJECT
- `invoker` -- authority address that issued the call
- `target` -- module_name, manifest_id, incident_id, or schema@version
- `incident_id` -- audit binding when applicable (empty otherwise)

### zerone.knowledge.proof_certificate_accepted
An M-FORMAL claim with a verified proof certificate was auto-accepted at Tier 0 without a PoT round. Fired by `MaybeAutoAcceptCertifiedClaim` when `claim.CertificateVerified=true` and `claim.MethodId="M-FORMAL"`. No verification round is created.
- `claim_id` -- claim that was auto-accepted
- `fact_id` -- resulting fact ID registered in the knowledge graph
- `submitter` -- address of the claim submitter
- `premise_count` -- number of premises in the proof certificate
- `confidence_bps` -- accepted confidence level in basis points

### zerone.knowledge.null_result_credit_awarded
A null-result credit was paid to a submitter whose claim was rejected but had declared falsification conditions (Formal Logic Gap 2). Fired when `params.NullResultCreditUzrn > 0` and the rejected claim has `refutation_conditions` set.
- `submitter` -- address receiving the credit
- `amount_uzrn` -- credit amount in uzrn
- `claim_id` -- rejected claim that triggered the credit
- `refutation_conditions_count` -- number of declared falsification conditions on the claim

### zerone.knowledge.claim_pre_registered
A hypothesis was anchored on-chain before evidence collection (Scientific Method Gap 3). Fired by `MsgPreRegisterClaim` when a submitter creates a pre-registration to prove temporal priority of their hypothesis.
- `pre_registration_id` -- unique identifier for this pre-registration record
- `submitter` -- address creating the pre-registration
- `domain` -- knowledge domain the claim will be submitted under
- `methodology_id` -- expected methodology for the eventual claim
- `expiry_block` -- block height at which the pre-registration expires if unfulfilled
- `bond_amount` -- bond escrowed in uzrn (returned on fulfillment, forfeited on expiry)

### zerone.knowledge.pre_registration_expired
A pre-registration was not fulfilled before its expiry block and the bond was forfeited to the research fund (Scientific Method Gap 3). Fired by BeginBlock when processing expired pre-registrations.
- `pre_registration_id` -- identifier of the expired pre-registration
- `submitter` -- address whose bond was forfeited
- `bond_forfeited` -- forfeited bond amount in uzrn

### zerone.knowledge.attentional_practice_set
A verifier's contemplative practice hours were recorded or updated (Contemplative Traditions Gap 2). Fired by `MsgSetAttentionalPractice`. When `params.min_phenomenological_practice_hours > 0`, verifiers must meet the declared threshold to join M-PHENOMENOLOGICAL panels.
- `verifier` -- verifier address whose practice record was updated
- `practice_hours` -- total declared hours of contemplative practice
- `action` -- `"self_declared"` or `"authority_override"`
- `sender` -- address that submitted the message

### zerone.knowledge.verifier_organization_set
A verifier's organizational affiliation was recorded or updated (Scientific Method Gap 1). Fired by `MsgSetVerifierOrganization`. Used by the panel diversity gate to compute organizational independence scores.
- `verifier` -- verifier address whose affiliation was updated
- `organization_id` -- declared organizational identifier
- `action` -- `"self_declared"` or `"authority_override"`
- `sender` -- address that submitted the message

### zerone.knowledge.methodology_amended
A registered methodology was amended by governance (Scientific Method Gap 2). Fired by `MsgUpdateMethodology`. Version is auto-incremented; the amendment rationale is stored on-chain.
- `methodology_id` -- identifier of the amended methodology
- `new_version` -- incremented version number after the amendment
- `ratified_at_block` -- block height at which the amendment was ratified
- `authority` -- governance authority address that submitted the amendment
- `rationale` -- free-form rationale for the amendment


## liquiditypool

### zerone.liquiditypool.pool_created
Liquidity pool created.
- `pool_id` -- pool identifier
- `denom_a` -- first denomination
- `denom_b` -- second denomination
- `reserve_a` -- initial reserve A
- `reserve_b` -- initial reserve B
- `lp_tokens` -- LP tokens minted

### zerone.liquiditypool.swap
Token swap executed.
- `pool_id` -- pool identifier
- `sender` -- swapper address
- `token_in` -- input denomination
- `amount_in` -- input amount
- `token_out` -- output denomination
- `amount_out` -- output amount
- `fee` -- fee amount

### zerone.liquiditypool.liquidity_added
Liquidity added to pool.
- `pool_id` -- pool identifier
- `sender` -- provider address
- `amount_a` -- amount of token A
- `amount_b` -- amount of token B
- `lp_tokens` -- LP tokens minted

### zerone.liquiditypool.liquidity_removed
Liquidity removed from pool.
- `pool_id` -- pool identifier
- `sender` -- remover address
- `lp_tokens_burned` -- LP tokens burned
- `amount_a` -- returned token A
- `amount_b` -- returned token B
- `mev_reserve_payout` -- uzrn paid from pool MEV reserve (pro-rata); "0" if no reserve (Gap 3)

### zerone.liquiditypool.mev_tip_processed
Emitted by `RevealSwap` when a non-zero `priority_tip_uzrn` is processed (Market Microstructure Gap 3).
- `pool_id` -- pool receiving the LP share
- `sender` -- payer of the priority tip
- `tip_total` -- total uzrn tip paid (bigint string)
- `lp_share` -- uzrn added to pool MEV reserve (tip × mev_fee_share_bps / 1M)
- `protocol_share` -- uzrn burned as protocol fee (tip − lp_share)

### zerone.liquiditypool.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.liquiditypool.mining_period_created
New liquidity mining period created for a pool.
- `period_id` -- deterministic SHA-256 ID of the period
- `pool_id` -- pool identifier
- `amount_per_block` -- uzrn emitted per block
- `start_block` -- first block rewards accrue
- `end_block` -- last block rewards accrue (exclusive)

### zerone.liquiditypool.mining_reward_accumulated
Emitted each block by `ProcessMiningBeginBlock` when rewards are minted and added to the pool accumulator.
- `pool_id` -- pool identifier
- `period_id` -- active mining period ID
- `amount` -- uzrn minted this block
- `accumulated_reward_per_lp` -- running accumulator value (×1e18 scale)

### zerone.liquiditypool.mining_rewards_claimed
LP miner claimed their accumulated rewards.
- `pool_id` -- pool identifier
- `miner` -- claimer address
- `amount` -- uzrn transferred to the miner

### zerone.liquiditypool.bonding_curve_pool_created
New bonding curve (single-asset) pool created. Emitted by `MsgCreatePool` when
`pool_type = POOL_TYPE_BONDING_CURVE`. Liquidity Gap 4.
- `pool_id` -- assigned pool identifier
- `denom_a` -- reserve token (e.g. `uzrn`)
- `denom_b` -- continuous token denom (minted/burned on demand)
- `reserve_a` -- initial reserve amount (= `amount_a` from `MsgCreatePool`)
- `initial_supply` -- initial continuous token supply minted to creator (= `reserve_a`, 1:1)
- `constant_reserve_ratio_bps` -- CRR in basis points (must be 1,000,000 = 100% in v1)
- `creator` -- governance authority address

### zerone.liquiditypool.bonding_curve_swap
Bonding curve buy or sell executed. Emitted by `MsgSwap` on
`POOL_TYPE_BONDING_CURVE` pools. Liquidity Gap 4.
- `pool_id` -- pool identifier
- `sender` -- swapper address
- `token_in` -- denom sent by swapper
- `amount_in` -- amount sent (bigint string)
- `token_out` -- denom received by swapper
- `amount_out` -- amount received after fee deduction (bigint string)
- `fee` -- fee deducted from output (bigint string; "0" when `swap_fee_bps = 0`)
- `direction` -- `"buy"` (denom_a → denom_b, mints) or `"sell"` (denom_b → denom_a, burns)

---

## ontology

### zerone.ontology.domain_proposed
Domain proposal created.
- `proposal_id` -- proposal ID
- `domain` -- proposed domain name
- `stratum` -- target stratum
- `proposer` -- proposer address
- `stake` -- proposal stake

### zerone.ontology.domain_updated
Domain metadata updated.
- `domain` -- domain name
- `authority` -- updater address

### zerone.ontology.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.ontology.logic_zone_registered
Logic zone registered.
- `zone` -- zone name
- `complete` -- completeness flag
- `goedel_applies` -- Goedel incompleteness applies
- `max_confidence_bps` -- maximum confidence (BPS)
- `authority` -- registrant

### zerone.ontology.incompleteness_acknowledged
Incompleteness acknowledged for a fact within a zone.
- `fact_id` -- fact identifier
- `zone` -- logic zone
- `submitter` -- submitter address

### zerone.ontology.domain_activated
Domain activated after proposal passed.
- `domain` -- domain name
- `height` -- activation block

### zerone.ontology.domain_deprecated
Domain deprecated.
- `domain` -- domain name
- `height` -- deprecation block

### zerone.ontology.domain_archived
Domain archived (terminal).
- `domain` -- domain name
- `height` -- archive block

### zerone.ontology.domains_merged
Two domains merged.
- `source` -- source domain (removed)
- `target` -- target domain (kept)
- `height` -- merge block

### zerone.ontology.proposal_voted
Domain proposal voted.
- `proposal_id` -- proposal ID
- `voter` -- voter address
- `approve` -- `"true"` or `"false"`
- `votes_for` -- cumulative yes votes
- `votes_against` -- cumulative no votes

### zerone.ontology.proposal_expired
*EndBlock.* Domain proposal expired.
- `proposal_id` -- proposal ID
- `domain` -- proposed domain
- `votes_for` -- final yes votes
- `votes_against` -- final no votes

### zerone.ontology.domain_status_transition
Unified lifecycle event for any domain status change (L1). Indexers can subscribe to this single feed instead of each named event.
- `domain` -- domain name
- `from_status` -- prior status (empty string if newly proposed)
- `to_status` -- new status (`proposed`, `active`, `deprecated`, `archived`, `deleted`)
- `reason` -- free-form transition reason (e.g. `proposal_created:<id>`, `merged_into:<target>`, `proposal_expired`)
- `height` -- block height at transition

---

## partnerships

### zerone.partnerships.partnership_proposed
Partnership proposed.
- `partnership_id` -- partnership ID
- `proposer` -- proposer address
- `partner` -- partner address

### zerone.partnerships.partnership_accepted
Partnership accepted.
- `partnership_id` -- partnership ID
- `accepter` -- accepter address

### zerone.partnerships.operation_proposed
Consensus operation proposed.
- `operation_id` -- operation ID
- `partnership_id` -- partnership ID
- `op_type` -- operation type
- `proposer` -- proposer address
- `amount` -- operation amount

### zerone.partnerships.operation_approved
Consensus operation approved.
- `operation_id` -- operation ID
- `approver` -- approver address

### zerone.partnerships.operation_rejected
Consensus operation rejected.
- `operation_id` -- operation ID
- `rejecter` -- rejecter address

### zerone.partnerships.seed_partnership_created
Seed partnership created.
- `seed_id` -- seed ID
- `human` -- human participant
- `agent` -- agent participant

### zerone.partnerships.joined_formation_pool
Address joined formation pool.
- `address` -- joiner address

### zerone.partnerships.left_formation_pool
Address left formation pool.
- `address` -- leaver address

### zerone.partnerships.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.partnerships.safety_freeze_applied
Safety freeze applied to partnership.
- `partnership_id` -- partnership ID
- `frozen_by` -- freezer address
- `expires_at` -- freeze expiry block
- `freeze_count` -- cumulative freeze count

### zerone.partnerships.coercion_signal_raised
Coercion signal raised.
- `signal_id` -- signal ID
- `partnership_id` -- partnership ID
- `raised_by` -- raiser address
- `expires_at` -- signal expiry block

### zerone.partnerships.freeze_expired
*BeginBlock.* Safety freeze expired and lifted.
- `partnership_id` -- partnership ID

### zerone.partnerships.coercion_signal_expired
*BeginBlock.* Coercion signal expired and resolved.
- `signal_id` -- signal ID
- `partnership_id` -- partnership ID

### zerone.partnerships.exit_initiated
Partnership exit (dissolution) initiated.
- `partnership_id` -- partnership ID
- `initiator` -- initiator address
- `cooldown_ends_at` -- cooldown expiry block
- `human_payout` -- payout to human
- `agent_payout` -- payout to agent
- `burned` -- burned amount

---

### zerone.partnerships.formation_match_accepted
Formation match accepted by both parties; partnership created.
- `match_id` -- formation match identifier
- `partnership_id` -- created partnership identifier

### zerone.partnerships.formation_match_declined
Formation match declined by a party.
- `declined_by` -- declining party address
- `match_id` -- formation match identifier

### zerone.partnerships.formation_match_partial_accept
Formation match accepted by one party; awaiting other.
- `accepter` -- accepting party address
- `match_id` -- formation match identifier

### zerone.partnerships.formation_match_proposed
Formation matching algorithm proposed a new match.
- `addr1` -- first candidate address
- `addr2` -- second candidate address
- `match_id` -- formation match identifier
- `score` -- compatibility score

### zerone.partnerships.mentorship_accepted
Mentorship proposal accepted by mentee.
- `mentee` -- mentee address
- `mentor` -- mentor address
- `mentorship_id` -- mentorship identifier

### zerone.partnerships.mentorship_graduated
Mentee graduated from mentorship; partnership proposed.
- `domain` -- mentorship domain
- `mentee` -- graduated mentee address
- `mentor` -- mentor address
- `mentorship_id` -- mentorship identifier

### zerone.partnerships.mentorship_proposed
New mentorship proposed between mentor and mentee.
- `domain` -- mentorship domain
- `mentee` -- mentee address
- `mentor` -- mentor address
- `mentorship_id` -- mentorship identifier

### zerone.partnerships.mentorship_terminated
Mentorship terminated by a party.
- `mentorship_id` -- mentorship identifier
- `terminated_by` -- terminating party address

### zerone.partnerships.reward_distributed
Reward distributed to partnership members.
- `agent_share` -- agent member share
- `gross_amount` -- total gross reward
- `human_share` -- human member share
- `partnership_id` -- partnership identifier
- `source` -- reward source module

### zerone.partnerships.cross_stratum_bonus
Partnership formation match received cross-stratum compatibility bonus (R31-4).
- `addr1` -- first member address
- `addr2` -- second member address
- `bonus_bps` -- bonus applied in BPS

### zerone.partnerships.formation_blocked
Partnership formation rejected because the domain is under a formation freeze (R31-3).
- `domain` -- target domain
- `freeze_expiry` -- freeze expiry block height
- `freeze_reason` -- reason supplied when the freeze was set
- `requester` -- proposer address

### zerone.partnerships.social_benefit_achieved
Domain crossed the social saturation threshold (R31-5 Water effect).
- `domain` -- target domain
- `density` -- partnership density at detection
- `threshold` -- configured social saturation threshold

### zerone.partnerships.social_benefit_lost
Domain dropped below the social saturation threshold (R31-5 Water effect).
- `domain` -- target domain
- `density` -- partnership density at detection
- `threshold` -- configured social saturation threshold

---

## private_corpus

### zerone.private_corpus.vault_registered
Private corpus vault registered on-chain.
- `vault_id` -- vault ID
- `operator` -- operator address
- `display_name` -- vault display name
- `registered_at_block` -- registration block height

### zerone.private_corpus.vault_updated
Vault metadata updated.
- `vault_id` -- vault ID
- `status` -- new vault status (e.g. `VAULT_STATUS_ACTIVE`, `VAULT_STATUS_DEPRECATED`)
- `updated_at_block` -- update block height

### zerone.private_corpus.manifest_published
Corpus manifest published to a vault.
- `manifest_id` -- manifest ID
- `vault_id` -- parent vault ID
- `version` -- manifest version string
- `content_hash` -- content hash of the corpus snapshot
- `published_at_block` -- publication block height

### zerone.private_corpus.manifest_withdrawn
Published manifest withdrawn.
- `manifest_id` -- manifest ID
- `vault_id` -- parent vault ID
- `reason` -- withdrawal reason
- `withdrawn_at_block` -- withdrawal block height

### zerone.private_corpus.access_recorded
Corpus access event recorded by vault operator.
- `seq` -- sequential access record number
- `vault_id` -- vault ID
- `manifest_id` -- manifest ID (empty if vault-level access)
- `accessor` -- accessor address
- `recorded_at_block` -- recording block height

---

## qualification

### zerone.qualification.qualification_granted
Qualification granted via pathway.
- `validator` -- validator address
- `domain` -- qualified domain
- `pathway` -- `stake`, `track_record`, `cross_reference`, or `inheritance`
- `weight` -- qualification weight
- `source_domain` -- (cross_reference only) source domain
- `parent_domain` -- (inheritance only) parent domain

### zerone.qualification.endorsement_created
Qualification endorsed.
- `endorsement_id` -- endorsement ID
- `validator` -- endorsed validator
- `domain` -- endorsed domain
- `endorser` -- endorser address

### zerone.qualification.qualification_renewed
Qualification renewed.
- `validator` -- validator address
- `domain` -- renewed domain
- `expires_at` -- new expiry block

### zerone.qualification.qualification_withdrawn
Qualification voluntarily withdrawn.
- `validator` -- validator address
- `domain` -- withdrawn domain

### zerone.qualification.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.qualification.qualification_expired
*BeginBlock.* Qualification expired.
- `validator` -- validator address
- `domain` -- expired domain

### zerone.qualification.qualification_promoted
*BeginBlock.* Probationary qualification promoted to full.
- `validator` -- validator address
- `domain` -- promoted domain

### zerone.qualification.qualification_suspended
*BeginBlock.* Qualification suspended for failing probation.
- `validator` -- validator address
- `domain` -- suspended domain
- `reason` -- suspension reason

### zerone.qualification.decay_probation
*BeginBlock.* Wave 16 accuracy-based decay. ACTIVE qualification with sufficient samples whose AccuracyBps fell below `decay_probation_bps` is demoted to PROBATIONARY. Skill is current, not historical: a qualified voter who consistently votes against verified consensus loses status until accuracy recovers.
- `validator` -- validator address
- `domain` -- demoted domain
- `accuracy_bps` -- current accuracy at demotion (BPS)
- `threshold_bps` -- the probation threshold that was crossed

### zerone.qualification.decay_suspension
*BeginBlock.* Wave 16 accuracy-based decay. PROBATIONARY qualification whose accuracy fell further below `decay_suspension_bps` is suspended. Suspended qualifications carry zero panel weight; voters must re-qualify (stake / track-record / endorsement) to vote effectively again.
- `validator` -- validator address
- `domain` -- suspended domain
- `accuracy_bps`, `threshold_bps` -- same semantics as decay_probation

### zerone.qualification.decay_recovered
*BeginBlock.* Wave 16 accuracy-based decay. PROBATIONARY qualification whose accuracy climbed back above `decay_recovery_bps` is reinstated to ACTIVE. The feedback loop is bidirectional: voters who improve their record reclaim full panel weight without re-qualifying.
- `validator` -- validator address
- `domain` -- recovered domain
- `accuracy_bps`, `threshold_bps` -- same semantics as decay_probation

---

## research

### zerone.research.research_submitted
Research paper submitted.
- `research_id` -- research ID
- `submitter` -- submitter address
- `domain` -- research domain
- `stake` -- bonded stake

### zerone.research.research_challenged
Research challenged.
- `research_id` -- research ID
- `challenger` -- challenger address
- `reason` -- challenge reason

### zerone.research.research_reviewed
Research reviewed.
- `research_id` -- research ID
- `reviewer` -- reviewer address
- `verdict` -- review verdict
- `score` -- review score

### zerone.research.research_resolved
Research resolved.
- `research_id` -- research ID
- `outcome` -- resolution outcome
- `aggregate_score` -- aggregate review score

### zerone.research.bounty_created
Research bounty created.
- `bounty_id` -- bounty ID
- `creator` -- creator address
- `reward` -- bounty reward

### zerone.research.bounty_claimed
Bounty claimed by researcher.
- `bounty_id` -- bounty ID
- `claimer` -- claimer address

### zerone.research.bounty_fulfilled
Bounty fulfilled and reward paid.
- `bounty_id` -- bounty ID
- `fulfilled_by` -- fulfiller address
- `reward` -- reward paid

### zerone.research.research_funded
Research treasury funded.
- `funder` -- funder address
- `amount` -- funded amount
- `new_treasury_balance` -- new balance

### zerone.research.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.research.bounty_expired
*EndBlock.* Open bounty expired.
- `bounty_id` -- bounty ID
- `reward_returned` -- reward returned to creator
- `creator` -- creator address

### zerone.research.bounty_claim_expired
*EndBlock.* Claimed bounty expired (undelivered).
- `bounty_id` -- bounty ID
- `former_claimer` -- claimer whose claim expired

---

### zerone.research.bounty_auto_fulfilled
Research bounty automatically fulfilled by system matching.
- `bounty_id` -- bounty identifier
- `fulfilled_by` -- fulfilling submission identifier
- `reward` -- reward paid

### zerone.research.research_auto_resolved
Research submission automatically resolved after review period.
- `aggregate_score` -- aggregate reviewer score
- `outcome` -- resolution outcome
- `research_id` -- research submission identifier


## schedule

### zerone.schedule.schedule_created
Scheduled process created.
- `process_id` -- process ID
- `creator` -- creator address
- `prepaid_fee` -- prepaid fee
- `next_execute_at` -- next execution block

### zerone.schedule.schedule_paused
Schedule paused.
- `process_id` -- process ID
- `creator` -- creator address

### zerone.schedule.schedule_resumed
Schedule resumed.
- `process_id` -- process ID
- `creator` -- creator address
- `next_execute_at` -- next execution block

### zerone.schedule.schedule_cancelled
Schedule cancelled.
- `process_id` -- process ID
- `creator` -- creator address
- `refunded_amount` -- refunded prepaid fee

### zerone.schedule.schedule_funded
Schedule funded.
- `process_id` -- process ID
- `creator` -- creator address
- `amount` -- funded amount
- `new_remaining` -- remaining fee balance

### zerone.schedule.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.schedule.schedule_executed
*EndBlock.* Scheduled process executed.
- `process_id` -- process ID
- `execution_count` -- cumulative executions
- `remaining_fee` -- remaining prepaid fee
- `status` -- execution status

---

## staking

### zerone.staking.validator_registered
Validator registered.
- `operator` -- operator address
- `tier` -- initial tier
- `self_delegation` -- self-delegation amount

### zerone.staking.delegation_created
Delegation created.
- `delegator` -- delegator address
- `validator` -- validator address
- `amount` -- delegated amount

### zerone.staking.validator_tier_changed
Validator tier changed.
- `validator` -- validator address
- `new_tier` -- new tier (msg handler)
- `old_tier` -- previous tier (EndBlocker only)

### zerone.staking.tier_transitioned
Unified tier-change event (L3) emitted at every site that mutates a validator's tier. Pairs with the named event above.
- `validator` -- validator operator address
- `from_tier` -- prior tier (`apprentice`, `verified`, `scholar`, `guardian`)
- `to_tier` -- new tier
- `direction` -- `promotion` or `demotion`
- `trigger` -- transaction or condition that caused the transition (e.g. `stake_delegate`, `redelegate_src`, `update_validator_stake`)


### zerone.staking.delegation_unbonding
Delegation unbonding initiated.
- `delegator` -- delegator address
- `validator` -- validator address
- `amount` -- unbonding amount
- `completes_at` -- completion block

### zerone.staking.delegation_redelegated
Delegation redelegated.
- `delegator` -- delegator address
- `src_validator` -- source validator
- `dst_validator` -- destination validator
- `amount` -- redelegated amount

### zerone.staking.update_validator_stake
Validator self-stake updated.
- `operator` -- operator address
- `new_stake` -- new stake amount

### zerone.staking.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.staking.unbonding_completed
*BeginBlock.* Unbonding matured and tokens returned.
- `delegator` -- delegator address
- `amount` -- returned amount

### zerone.staking.validator_slashed
Validator slashed.
- `validator` -- validator address
- `amount` -- slashed amount
- `reason` -- slash reason

### zerone.staking.slash_evidence_submitted
*Tx.* Third-party slash evidence queued for end-block processing.
- `evidence_id` -- unique evidence identifier (hex, 16-byte sha256 prefix of submitter|validator|blockHeight)
- `submitter` -- address that submitted the evidence
- `validator` -- address of the accused validator
- `slash_type` -- slash type string (e.g. `double_vote`, `liveness_fault`)
- `evidence_hash` -- caller-provided hash of supporting evidence artefact

### zerone.staking.slash_evidence_processed
*EndBlock.* Queued slash evidence executed: validator slashed, tokens routed.
- `evidence_id` -- evidence identifier matching the `slash_evidence_submitted` event
- `submitter` -- evidence submitter address (may receive `evidence_reward_bps` fraction)
- `validator` -- slashed validator address
- `slash_type` -- slash type string
- `slashed` -- actual amount slashed (after escalation, in uzrn)
- `evidence_hash` -- hash of the evidence artefact

### zerone.staking.insurance_fund_contributed
A validator operator deposited uzrn into their per-validator insurance fund (Slashing Gap 4). The fund absorbs slash overflow before delegated stake is touched.
- `operator` -- validator operator address
- `amount` -- amount deposited in uzrn
- `new_balance` -- total insurance fund balance after deposit

### zerone.staking.validator_tombstoned
A validator was permanently banned from the active set due to repeated slashing (Slashing Gap 1). Tombstoning requires governance intervention (`MsgUntombstoneValidator`) to lift; it is stronger than jailing which is cleared automatically.
- `validator` -- operator address of the tombstoned validator
- `slash_count` -- cumulative slash count that triggered tombstoning
- `tombstoned_at` -- block height at which tombstoning was applied

### zerone.staking.validator_untombstoned
A tombstoned validator was reinstated by governance (Slashing Gap 1). Untombstoning clears the tombstone flag but does not automatically reactivate the validator; they must re-register or be re-elected.
- `validator` -- operator address of the reinstated validator
- `authority` -- governance authority address that submitted `MsgUntombstoneValidator`
- `reason` -- stated reason for reinstatement

---

## tokens

### zerone.tokens.token_created
Custom token created.
- `token_id` -- token identifier
- `creator` -- creator address
- `symbol` -- token symbol
- `initial_supply` -- initial supply

### zerone.tokens.token_minted
Tokens minted.
- `token_id` -- token identifier
- `to` -- recipient address
- `amount` -- minted amount

### zerone.tokens.token_burned
Tokens burned.
- `token_id` -- token identifier
- `burner` -- burner address
- `amount` -- burned amount

### zerone.tokens.token_transferred
Tokens transferred.
- `token_id` -- token identifier
- `from` -- sender address
- `to` -- recipient address
- `amount` -- transferred amount

### zerone.tokens.token_approved
Token spending approved.
- `token_id` -- token identifier
- `owner` -- owner address
- `spender` -- approved spender
- `amount` -- approved amount

### zerone.tokens.transfer_from
Approved transfer executed.
- `token_id` -- token identifier
- `spender` -- spender address
- `from` -- source address
- `to` -- destination address
- `amount` -- transferred amount

### zerone.tokens.token_paused
Token transfers paused.
- `token_id` -- token identifier
- `authority` -- pausing authority

### zerone.tokens.token_unpaused
Token transfers unpaused.
- `token_id` -- token identifier
- `authority` -- unpausing authority

### zerone.tokens.power_delegated
Voting power delegated.
- `token_id` -- token identifier
- `delegator` -- delegator address
- `delegate` -- delegate address
- `amount` -- delegated amount

### zerone.tokens.power_undelegated
Voting power undelegated.
- `token_id` -- token identifier
- `delegator` -- delegator address
- `delegate` -- delegate address
- `amount` -- undelegated amount

### zerone.tokens.token_wrapped
Token wrapped into native denom.
- `token_id` -- token identifier
- `sender` -- sender address
- `amount` -- wrapped amount
- `wrapped_denom` -- native denomination

### zerone.tokens.token_unwrapped
Token unwrapped from native denom.
- `token_id` -- token identifier
- `sender` -- sender address
- `amount` -- unwrapped amount
- `wrapped_denom` -- native denomination

### zerone.tokens.emission_period_created
Emission period created.
- `emission_id` -- emission ID
- `start_block` -- start block
- `end_block` -- end block
- `amount_per_block` -- emission rate
- `recipient` -- recipient address (legacy single-recipient; empty when multi-recipient split is used)
- `recipient_count` -- *(Gap 5)* number of recipients in the split; `"1"` for legacy single-recipient periods

### zerone.tokens.emission_period_cancelled
Emission period cancelled.
- `emission_id` -- emission ID
- `authority` -- cancelling authority

### zerone.tokens.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.tokens.emission_supply_cap_reached
*(Gap 4 — monetary policy)* Emission period deactivated because native supply cap was reached.
Fired when `max_native_supply_uzrn` is set and an emission would exceed it (including the partial-mint case where the period is deactivated after filling the remaining headroom).
- `emission_id` -- emission ID that was deactivated
- `cap` -- the `max_native_supply_uzrn` value that was hit

### zerone.tokens.emission_halving_applied
*(Gap 3 — monetary policy)* Halving applied to an emission period.
Fired when BeginBlocker reduces `amount_per_block` for a halving-enabled period.
- `emission_id` -- emission ID
- `halving_count` -- new `HalvingCount` after this halving
- `new_amount_per_block` -- reduced emission rate

### zerone.tokens.emission_scale_adjusted
*(Gap 2 — monetary policy)* Adaptive emission scale adjusted at epoch boundary.
Fired every `EmissionEpochBlocks` when both `TargetBondedRatioBps` and `EmissionAdjustmentBps` are non-zero.
The scale is applied transiently as a multiplier to each emission period's `amount_per_block` during BeginBlocker;
it is NOT written back into `EmissionPeriod.AmountPerBlock` (halvings own that field).
- `current_ratio_bps` -- actual bonded/total-supply ratio in bps at adjustment time (e.g. `"620000"` = 62%)
- `target_ratio_bps` -- configured target ratio (`TargetBondedRatioBps` param)
- `new_scale_bps` -- updated `EmissionScaleBps` after clamping to `[MinEmissionScaleBps, MaxEmissionScaleBps]`

---

## toolbox

### zerone.toolbox.tool_registered
Tool registered.
- `tool_id` -- tool identifier
- `deployer` -- deployer address
- `name` -- tool name
- `tool_type` -- tool type

### zerone.toolbox.tool_called
Tool invoked.
- `call_id` -- call identifier
- `tool_id` -- tool identifier
- `caller` -- caller address
- `payment` -- payment amount
- `success` -- `"true"` or `"false"`

### zerone.toolbox.contributor_added
Contributor added to tool.
- `tool_id` -- tool identifier
- `contributor` -- contributor address
- `role` -- contributor role

### zerone.toolbox.contributorship_accepted
Contributorship accepted.
- `tool_id` -- tool identifier
- `contributor` -- contributor address

### zerone.toolbox.tool_upgraded
Tool upgraded to new version.
- `new_tool_id` -- new tool ID
- `previous_tool_id` -- previous tool ID
- `version` -- new version

### zerone.toolbox.tool_deprecated
Tool deprecated with successor.
- `tool_id` -- deprecated tool ID
- `successor_tool_id` -- successor tool ID

### zerone.toolbox.tool_retired
Tool retired.
- `tool_id` -- tool identifier

### zerone.toolbox.shares_locked
Tool revenue shares locked.
- `tool_id` -- tool identifier
- `lock_height` -- lock height

### zerone.toolbox.dependency_updated
Tool dependency updated.
- `tool_id` -- tool identifier
- `old_dep_id` -- old dependency
- `new_dep_id` -- new dependency

### zerone.toolbox.tool_heartbeat
Tool heartbeat received.
- `tool_id` -- tool identifier
- `caller` -- caller address
- `block` -- block height

### zerone.toolbox.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.toolbox.trust_updated
Tool trust score updated.
- `tool_id` -- tool identifier
- `old_score` -- previous score
- `new_score` -- new score
- `tier` -- new trust tier

### zerone.toolbox.bvm_call
Tool called from BVM contract.
- `tool_id` -- tool identifier
- `caller_contract` -- calling contract
- `success` -- `"true"` or `"false"`

### zerone.toolbox.free_call
Free-tier tool call consumed.
- `caller` -- caller address
- `tool_id` -- tool identifier
- `category` -- tool category

### zerone.toolbox.surge_pricing
Surge pricing applied.
- `tool_id` -- tool identifier
- `base_price` -- base price
- `effective_price` -- effective price after surge
- `multiplier_bps` -- surge multiplier (BPS)
- `utilisation_bps` -- utilisation (BPS)
- `tier` -- surge tier

### zerone.toolbox.pricing_oracle_unavailable
USD pricing oracle unavailable.
- `tool_id` -- tool identifier
- `reason` -- unavailability reason

---

## tree

### zerone.tree.project_created
Project created.
- `project_id` -- project ID
- `founder` -- founder address
- `domain` -- project domain

### zerone.tree.project_proposed
Project proposed for review.
- `project_id` -- project ID

### zerone.tree.development_started
Project development started.
- `project_id` -- project ID

### zerone.tree.project_completed
Project completed.
- `project_id` -- project ID

### zerone.tree.project_paused
Project paused.
- `project_id` -- project ID
- `reason` -- pause reason

### zerone.tree.project_resumed
Project resumed.
- `project_id` -- project ID
- `resumed_to` -- resumed status

### zerone.tree.abandon_proposed
Abandonment proposed (requires consent).
- `project_id` -- project ID
- `proposed_by` -- proposer address
- `expires_at_block` -- consent deadline

### zerone.tree.project_abandoned
Project abandoned.
- `project_id` -- project ID
- `executed_by` -- executor address
- `consent_count` -- consenting parties

### zerone.tree.child_project_spawned
Child project spawned from parent.
- `child_id` -- child project ID
- `parent_id` -- parent project ID

### zerone.tree.task_added
Task added to project.
- `task_id` -- task ID
- `project_id` -- project ID
- `bounty_escrowed` -- escrowed bounty

### zerone.tree.task_assigned
Task assigned to agent.
- `task_id` -- task ID
- `assignee` -- assignee address

### zerone.tree.work_started
Work started on task.
- `task_id` -- task ID

### zerone.tree.deliverable_submitted
Deliverable submitted for review.
- `task_id` -- task ID
- `hash` -- deliverable hash

### zerone.tree.deliverable_approved
Deliverable approved.
- `task_id` -- task ID
- `approver` -- approver address

### zerone.tree.deliverable_rejected
Deliverable rejected.
- `task_id` -- task ID
- `reason` -- rejection reason
- `rejection_count` -- total rejections
- `disputed` -- whether dispute was triggered

### zerone.tree.dispute_slash
Slash applied after deliverable dispute.
- `task_id` -- task ID
- `slash_amount` -- slashed amount
- `refund_amount` -- refunded amount

### zerone.tree.task_reopened
Task reopened after rejection.
- `task_id` -- task ID

### zerone.tree.project_application
Application to join project.
- `project_id` -- project ID
- `applicant` -- applicant address

### zerone.tree.application_reviewed
Application reviewed.
- `applicant` -- applicant address
- `accepted` -- `"true"` or `"false"`

### zerone.tree.availability_set
Agent availability updated.
- `agent` -- agent address
- `available` -- `"true"` or `"false"`

### zerone.tree.service_deployed
Service deployed.
- `service_id` -- service ID
- `deployer` -- deployer address

### zerone.tree.service_paused
Service paused.
- `service_id` -- service ID

### zerone.tree.service_resumed
Service resumed.
- `service_id` -- service ID

### zerone.tree.service_retired
Service retired.
- `service_id` -- service ID

### zerone.tree.service_called
Service called.
- `service_id` -- service ID
- `caller` -- caller address
- `amount` -- payment amount
- `payment_method` -- payment method

### zerone.tree.contributor_added
Contributor added to project.
- `project_id` -- project ID
- `contributor` -- contributor address
- `role` -- contributor role

### zerone.tree.service_subscribed
Service subscription created.
- `subscription_id` -- subscription ID
- `service_id` -- service ID
- `subscriber` -- subscriber address
- `duration_blocks` -- subscription duration

### zerone.tree.seeding_begun
Project seeding begun.
- `project_id` -- project ID
- `seed_id` -- seed partnership ID
- `domain` -- project domain

### zerone.tree.opportunity_detected
Opportunity detected.
- `opportunity_id` -- opportunity ID
- `detector` -- detector address
- `domain` -- opportunity domain

### zerone.tree.opportunity_claimed
Opportunity claimed.
- `opportunity_id` -- opportunity ID
- `claimer` -- claimer address
- `stake` -- bonded stake

### zerone.tree.params_updated
Governance parameter update.
- `authority` -- governance address

### zerone.tree.service_called_via_adapter
Service called via toolbox adapter.
- `service_id` -- service ID
- `caller` -- caller address
- `via` -- adapter path (`toolbox_adapter`)

---

## vesting_rewards

### zerone.vesting_rewards.vesting_created
Vesting schedule created.
- `vesting_id` -- vesting ID
- `beneficiary` -- beneficiary address
- `amount` -- vested amount

### zerone.vesting_rewards.rewards_claimed
Vested rewards claimed.
- `claimer` -- claimer address
- `claimed_amount` -- claimed amount

### zerone.vesting_rewards.vesting_falsified
Vesting schedule falsified (stopped for cause).
- `vesting_id` -- vesting ID
- `challenger` -- challenger address
- `reason` -- falsification reason

### zerone.vesting_rewards.vesting_paused
Vesting schedule paused.
- `vesting_id` -- vesting ID
- `reason` -- pause reason

### zerone.vesting_rewards.vesting_resumed
Vesting schedule resumed.
- `vesting_id` -- vesting ID

### zerone.vesting_rewards.vesting_accelerated
Vesting schedule accelerated.
- `vesting_id` -- vesting ID
- `acceleration_type` -- acceleration type

### zerone.vesting_rewards.vesting_completed
Vesting schedule fully released.
- `vesting_id` -- vesting ID
- `released_amount` -- total released amount

### zerone.vesting_rewards.knowledge_coupling_applied
Block reward scaled by verification throughput (T9 / thesis claim 1).
- `verification_rate_bps` -- accepted/terminal ratio in BPS
- `target_bps` -- configured target rate
- `multiplier_bps` -- applied reward multiplier in BPS

### zerone.vesting_rewards.update_params
Governance parameter update.
- `authority` -- governance address

### zerone.vesting_rewards.research_fund_deposit
Research fund deposit via revenue routing.
- `source_module` -- source module
- `denom` -- denomination
- `total` -- total routed
- `research` -- research fund share
- `founder` -- founder share

### zerone.vesting_rewards.block_reward_distributed
*BeginBlock.* Block reward distributed to producer.
- `block_height` -- block height
- `producer` -- block producer address
- `total_minted` -- total newly minted tokens
- `producer_reward` -- producer's share
- `active_validators` -- active validator count

---

### zerone.vesting_rewards.vesting_paused_misbehavior
Vesting paused due to validator misbehaviour.
- `count` -- number of misbehaviours
- `recipient` -- vesting recipient address



## WebSocket Subscriptions

Subscribe to events in real time via CometBFT WebSocket:

```bash
# Connect to WebSocket
wscat -c ws://localhost:26657/websocket
```

### Subscribe to all events from a module

```json
{
  "jsonrpc": "2.0",
  "method": "subscribe",
  "id": 1,
  "params": {
    "query": "tm.event='Tx' AND zerone.knowledge.submit_claim.submitter EXISTS"
  }
}
```

### Subscribe to a specific event type

```json
{
  "jsonrpc": "2.0",
  "method": "subscribe",
  "id": 2,
  "params": {
    "query": "tm.event='Tx' AND zerone.staking.validator_registered.operator EXISTS"
  }
}
```

### Subscribe to new blocks (includes BeginBlock/EndBlock events)

```json
{
  "jsonrpc": "2.0",
  "method": "subscribe",
  "id": 3,
  "params": {
    "query": "tm.event='NewBlock'"
  }
}
```

### Subscribe to events by attribute value

```json
{
  "jsonrpc": "2.0",
  "method": "subscribe",
  "id": 4,
  "params": {
    "query": "tm.event='Tx' AND zerone.disputes.dispute_settled.outcome='challenger_wins'"
  }
}
```

### Unsubscribe

```json
{
  "jsonrpc": "2.0",
  "method": "unsubscribe",
  "id": 5,
  "params": {
    "query": "tm.event='Tx' AND zerone.knowledge.submit_claim.submitter EXISTS"
  }
}
```

---

## Transaction Indexing

### CometBFT Configuration

Ensure the KV indexer is enabled in `config.toml`:

```toml
[tx_index]
indexer = "kv"
```

### Query Historical Events

```bash
# Search for all fact verifications for a specific fact
curl "http://localhost:26657/tx_search?query=\"zerone.knowledge.fact_created.fact_id='axiom-001'\"&prove=true"

# Search for all disputes initiated by an address
curl "http://localhost:26657/tx_search?query=\"zerone.disputes.dispute_initiated.challenger='zerone1abc...'\"&prove=true"

# Search by block range
curl "http://localhost:26657/tx_search?query=\"tx.height>1000 AND tx.height<2000 AND zerone.staking.validator_registered.operator EXISTS\"&prove=true"
```

---

## Block Explorer Compatibility

Zerone events follow the standard Cosmos SDK event format and are compatible
with standard block explorers:

### Mintscan

Events use the standard `sdk.NewEvent` pattern with string-typed attribute
values. No custom decoder is needed for basic event display. Mintscan indexes
events by type and attribute key automatically.

### ping.pub

Zerone is compatible with the CosmosDirectory format used by ping.pub. Events
appear under the transaction detail view. The `zerone.<module>.<action>` naming
convention makes events easy to filter.

### Custom Explorer Notes

For Zerone-specific features, block explorers may want to:

1. **Knowledge verification timeline** -- Track `submit_claim` -> `submit_commitment` -> `submit_reveal` -> `verification_round_completed` -> `fact_created` flow
2. **Emergency ceremony progress** -- Monitor `halt_proposed` -> `vote_halt` -> `ceremony_advanced` -> `ceremony_finalized` sequence
3. **Partnership lifecycle** -- Follow `partnership_proposed` -> `partnership_accepted` -> `operation_proposed` -> `operation_approved` -> `exit_initiated` flow
4. **Alignment health** -- Display `observation_recorded` composite scores over time
5. **Autopoiesis adaptation** -- Chart `epoch_processed` SSI scores and multiplier changes
