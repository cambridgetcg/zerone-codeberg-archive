package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/zerone-chain/zerone/x/knowledge/types"
)

// VerifyProofCertificate checks a ProofCertificate at claim creation time.
//
// For PROOF_FORMAT_CANONICAL_CHAIN: verifies that every premise_fact_id
// references an accepted on-chain fact (FACT_STATUS_ACTIVE or FACT_STATUS_VERIFIED).
// Returns true only when all premises are verified — the chain can then set
// Claim.certificate_verified = true and, if bypass is enabled, skip PoT.
//
// For other formats (LEAN4, HASH_ONLY): the certificate is stored as a commitment
// only. The chain cannot verify these mechanically; returns false so the claim
// proceeds through the normal Proof-of-Truth panel.
//
// A nil certificate or one with no premises always returns false.
func (k Keeper) VerifyProofCertificate(ctx context.Context, cert *types.ProofCertificate) bool {
	if cert == nil {
		return false
	}
	if cert.Format != types.ProofFormat_PROOF_FORMAT_CANONICAL_CHAIN {
		// External formats: store-only, no on-chain verification.
		return false
	}
	if len(cert.PremiseFactIds) == 0 {
		// Empty premise set is not a valid derivation — a proof must depend on at
		// least one ground truth. (Genesis axioms are facts on-chain and count.)
		return false
	}
	for _, factID := range cert.PremiseFactIds {
		fact, found := k.GetFact(ctx, factID)
		if !found {
			return false
		}
		// Accept ACTIVE and VERIFIED as valid premises. CONTESTED and CHALLENGED
		// are excluded — a contested premise is currently under dispute and should
		// not serve as a proof foundation until the dispute resolves.
		status := fact.Status
		if status != types.FactStatus_FACT_STATUS_ACTIVE &&
			status != types.FactStatus_FACT_STATUS_VERIFIED {
			return false
		}
	}
	return true
}

// MaybeAutoAcceptCertifiedClaim checks whether a claim with a verified
// proof certificate should bypass the Proof-of-Truth panel (Tier 0 acceptance).
//
// Conditions for bypass:
//   1. cert != nil, format == CANONICAL_CHAIN, and all premises verified (certificate_verified = true).
//   2. claim.method_id == "M-FORMAL" — bypass is only meaningful for formal claims.
//   3. Params.proof_certificate_bypass_enabled == true (governance gate).
//
// When all three conditions hold, the claim is accepted directly:
//   - A fact is created from the claim at the declared confidence (clamped to MaxConfidence).
//   - An event "zerone.knowledge.proof_certificate_accepted" is emitted.
//   - Returns (true, nil) to signal that no PoT round should be created.
//
// Returns (false, nil) when the claim should proceed normally through PoT.
func (k Keeper) MaybeAutoAcceptCertifiedClaim(ctx context.Context, claim *types.Claim) (accepted bool, err error) {
	if !claim.CertificateVerified {
		return false, nil
	}
	if claim.MethodId != "M-FORMAL" {
		// Only M-FORMAL claims earn Tier 0. Other methods still need social verification.
		return false, nil
	}

	params, pErr := k.GetParams(ctx)
	if pErr != nil || params == nil {
		return false, nil
	}
	if !params.ProofCertificateBypassEnabled {
		// Governance has not yet activated bypass. Claim proceeds to PoT.
		return false, nil
	}

	// A formally proven claim earns the chain's maximum allowed confidence
	// (MaxConfidence, 880 000 BPS by default). This operationalises LOGIC-077:
	// even mechanical proofs do not claim certainty — the confidence cap is the
	// chain's structural fallibilism commitment, and certified proofs respect it.
	confidence := params.MaxConfidence

	// Create the accepted fact directly (no round needed).
	factID, err := k.createFactFromClaim(ctx, claim, nil, confidence)
	if err != nil {
		return false, fmt.Errorf("proof certificate auto-accept: createFactFromClaim: %w", err)
	}

	claim.Status = types.ClaimStatus_CLAIM_STATUS_ACCEPTED
	if saveErr := k.SetClaim(ctx, claim); saveErr != nil {
		return false, saveErr
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"zerone.knowledge.proof_certificate_accepted",
		sdk.NewAttribute("claim_id", claim.Id),
		sdk.NewAttribute("fact_id", factID),
		sdk.NewAttribute("submitter", claim.Submitter),
		sdk.NewAttribute("premise_count", fmt.Sprintf("%d", len(claim.ProofCertificate.PremiseFactIds))),
		sdk.NewAttribute("confidence_bps", fmt.Sprintf("%d", confidence)),
	))

	return true, nil
}

// RecordProofCertificateForTest is an exported wrapper for unit tests.
func (k Keeper) RecordProofCertificateForTest(ctx context.Context, cert *types.ProofCertificate) bool {
	return k.VerifyProofCertificate(ctx, cert)
}
