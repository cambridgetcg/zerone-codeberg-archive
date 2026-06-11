package main

import (
	"crypto/sha256"
	"encoding/hex"
)

// EvaluateRequest is the input to an Evaluator.
type EvaluateRequest struct {
	Claim     string `json:"claim"`
	Domain    string `json:"domain"`
	ClaimType string `json:"claim_type"`

	// Freshness proof fields (Oracle Resilience Brief Gap 4).
	// When provided, the oracle computes a request_id and returns it in the
	// response. Verifiers include oracle_request_id in MsgSubmitReveal so the
	// chain can verify the oracle was queried recently.
	ClaimId      string `json:"claim_id,omitempty"`      // chain claim identifier
	BlockHash    string `json:"block_hash,omitempty"`    // hex-encoded block hash at query time
	VerifierAddr string `json:"verifier_addr,omitempty"` // verifier's bech32 address
}

// EvaluateResponse is the output from an Evaluator.
type EvaluateResponse struct {
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`

	// RequestId is set when the request included claim_id, block_hash, and
	// verifier_addr. It is:
	//   sha256(claim_id + "|" + block_hash + "|" + verifier_addr)
	// where block_hash is the hex string from the request (not re-encoded).
	// Verifiers copy this value into MsgSubmitReveal.oracle_request_id.
	// Oracle Resilience Brief Gap 4.
	RequestId string `json:"request_id,omitempty"`
}

// computeRequestId returns the freshness fingerprint for an oracle query.
// Returns "" if any required input is empty.
func computeRequestId(claimId, blockHash, verifierAddr string) string {
	if claimId == "" || blockHash == "" || verifierAddr == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(claimId))
	h.Write([]byte("|"))
	h.Write([]byte(blockHash))
	h.Write([]byte("|"))
	h.Write([]byte(verifierAddr))
	return hex.EncodeToString(h.Sum(nil))
}

// Evaluator is the interface every evaluation strategy must implement.
type Evaluator interface {
	Evaluate(req EvaluateRequest) (*EvaluateResponse, error)
	Name() string
}

// CombinedEvaluator orchestrates static and LLM evaluation.
// Static evaluation runs first; LLM is used only when the static
// result is uncertain and an LLM evaluator is available.
type CombinedEvaluator struct {
	Static *StaticEvaluator
	LLM    *LLMEvaluator
}

// Evaluate implements the combined evaluation strategy:
//  1. Always run static first.
//  2. On static error, treat as uncertain (0.5 confidence).
//  3. High-confidence static verdict (>0.7, accept OR reject) → short-circuit.
//  4. Static uncertain + LLM available → use LLM (on LLM error, fall back to static).
//  5. Static uncertain + no LLM → return static result.
//  6. Always: if claim_id + block_hash + verifier_addr are provided, attach
//     request_id to the response (Oracle Resilience Brief Gap 4).
func (c *CombinedEvaluator) Evaluate(req EvaluateRequest) (*EvaluateResponse, error) {
	// Freshness proof: compute request_id from the verifier-provided inputs.
	// Done once before evaluation so all response paths carry the same id.
	requestId := computeRequestId(req.ClaimId, req.BlockHash, req.VerifierAddr)

	// Step 1: always run static first.
	staticResp, staticErr := c.Static.Evaluate(req)

	// Step 2: on static error, synthesize an uncertain result.
	if staticErr != nil {
		staticResp = &EvaluateResponse{
			Verdict:    "uncertain",
			Confidence: 0.5,
			Reasoning:  "static evaluation failed: " + staticErr.Error(),
		}
	}

	// Step 3: high-confidence static verdict → short-circuit.
	if staticResp.Confidence > 0.7 && (staticResp.Verdict == "accept" || staticResp.Verdict == "reject") {
		staticResp.RequestId = requestId
		return staticResp, nil
	}

	// Step 4: static uncertain + LLM available → delegate to LLM.
	if c.LLM != nil {
		llmResp, llmErr := c.LLM.Evaluate(req)
		if llmErr == nil {
			llmResp.RequestId = requestId
			return llmResp, nil
		}
		// LLM error → fall back to static result (step 4 fallback).
	}

	// Step 5: static uncertain + no LLM (or LLM failed) → return static.
	staticResp.RequestId = requestId
	return staticResp, nil
}

// Name returns the evaluator name.
func (c *CombinedEvaluator) Name() string {
	return "combined"
}
