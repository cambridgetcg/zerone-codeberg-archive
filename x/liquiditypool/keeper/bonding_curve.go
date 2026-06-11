package keeper

// bonding_curve.go — Liquidity Gap 4: Bonding Curve (Single-Asset Pools).
//
// Implements the Bancor-formula bonding curve as an alternative to the
// constant-product (xy=k) AMM. Bonding-curve pools hold a single reserve
// token (denom_a, typically uzrn) and mint/burn the continuous token
// (denom_b) on demand.
//
// Bancor price formula:
//   spot_price = reserveA / (supplyB × CRR/1M)
// where:
//   reserveA = pool.ReserveA (reserve token balance)
//   supplyB  = pool.LpTokenSupply (continuous token supply)
//   CRR      = pool.ConstantReserveRatioBps / 1,000,000
//
// v1 supports only CRR = 1,000,000 (100%) — the linear bonding curve.
// For CRR = 100%:
//   Buy:  tokensOut = supplyB × depositA / reserveA
//   Sell: reserveOut = reserveA × tokensSold / supplyB
//
// Future versions may support other CRR values (requires fractional-
// exponent arithmetic, e.g. Bancor power function).
//
// Error codes:
//   ErrUnsupportedCRR          — CRR ≠ 1,000,000 in v1
//   ErrBondingCurveEmptySupply — sell attempted when continuous supply = 0
//   ErrBondingCurveBadReserve  — reserve or supply is zero/negative
//
// The Buy/Sell functions return (tokensOut, err) — they do NOT mutate pool.
// The caller (msg_server) is responsible for updating pool state.

import (
	"fmt"
	"math/big"
)

const (
	// CRRLinear is the only supported CRR in v1: 100% (linear bonding curve).
	CRRLinear = uint64(1_000_000)
)

// BondingCurveBuyOut computes how many continuous tokens (denom_b) a buyer
// receives when depositing depositAmount of reserve tokens (denom_a).
//
// Formula (CRR = 100%):
//   tokensOut = supplyB × depositAmount / reserveA
//
// Parameters are bigint strings. Returns bigint string.
func BondingCurveBuyOut(reserveA, supplyB, depositAmount string, crr uint64) (string, error) {
	if crr != CRRLinear {
		return "", fmt.Errorf("unsupported CRR %d: only CRR=1,000,000 (linear) supported in v1", crr)
	}

	rA, ok := new(big.Int).SetString(reserveA, 10)
	if !ok || rA.Sign() <= 0 {
		return "", fmt.Errorf("bonding curve: invalid reserve %q", reserveA)
	}
	sB, ok := new(big.Int).SetString(supplyB, 10)
	if !ok || sB.Sign() < 0 {
		return "", fmt.Errorf("bonding curve: invalid supply %q", supplyB)
	}
	dep, ok := new(big.Int).SetString(depositAmount, 10)
	if !ok || dep.Sign() <= 0 {
		return "", fmt.Errorf("bonding curve: invalid deposit amount %q", depositAmount)
	}

	// If supply = 0 (fresh pool), initial minting: every unit of reserve gets
	// 1 unit of continuous token (price starts at 1 reserveA per tokenB).
	if sB.Sign() == 0 {
		return dep.String(), nil
	}

	// tokensOut = sB × dep / rA  (floor division)
	out := new(big.Int).Mul(sB, dep)
	out.Div(out, rA)

	if out.Sign() == 0 {
		return "", fmt.Errorf("bonding curve: deposit too small — output rounds to zero")
	}
	return out.String(), nil
}

// BondingCurveSellOut computes how many reserve tokens (denom_a) a seller
// receives when burning tokensSold of continuous tokens (denom_b).
//
// Formula (CRR = 100%):
//   reserveOut = reserveA × tokensSold / supplyB
//
// Parameters are bigint strings. Returns bigint string.
func BondingCurveSellOut(reserveA, supplyB, tokensSold string, crr uint64) (string, error) {
	if crr != CRRLinear {
		return "", fmt.Errorf("unsupported CRR %d: only CRR=1,000,000 (linear) supported in v1", crr)
	}

	rA, ok := new(big.Int).SetString(reserveA, 10)
	if !ok || rA.Sign() <= 0 {
		return "", fmt.Errorf("bonding curve: invalid reserve %q", reserveA)
	}
	sB, ok := new(big.Int).SetString(supplyB, 10)
	if !ok || sB.Sign() <= 0 {
		return "", fmt.Errorf("bonding curve: empty continuous token supply — nothing to sell")
	}
	sold, ok := new(big.Int).SetString(tokensSold, 10)
	if !ok || sold.Sign() <= 0 {
		return "", fmt.Errorf("bonding curve: invalid tokens sold %q", tokensSold)
	}
	if sold.Cmp(sB) > 0 {
		return "", fmt.Errorf("bonding curve: cannot sell more than total supply (%s > %s)", tokensSold, supplyB)
	}

	// reserveOut = rA × sold / sB  (floor division)
	out := new(big.Int).Mul(rA, sold)
	out.Div(out, sB)

	if out.Sign() == 0 {
		return "", fmt.Errorf("bonding curve: sale too small — output rounds to zero")
	}
	return out.String(), nil
}

// BondingCurveSpotPrice returns the spot price of one continuous token
// (denom_b) in reserve token (denom_a) units, as a rational (num, denom).
//
// price = reserveA / (supplyB × CRR/1M) = reserveA × 1M / (supplyB × CRR)
// For CRR = 100%:  price = reserveA / supplyB
//
// Returns (numerator, denominator) as big.Int strings for display; callers
// can compute the float as float64(num) / float64(denom).
func BondingCurveSpotPrice(reserveA, supplyB string, crr uint64) (num string, denom string, err error) {
	rA, ok := new(big.Int).SetString(reserveA, 10)
	if !ok || rA.Sign() <= 0 {
		return "", "", fmt.Errorf("bonding curve: invalid reserve %q", reserveA)
	}
	sB, ok := new(big.Int).SetString(supplyB, 10)
	if !ok || sB.Sign() <= 0 {
		return "", "", fmt.Errorf("bonding curve: invalid supply %q (no price before tokens minted)", supplyB)
	}
	crrBig := new(big.Int).SetUint64(crr)

	// price_num   = reserveA × 1_000_000
	// price_denom = supplyB × crr
	priceNum := new(big.Int).Mul(rA, big.NewInt(1_000_000))
	priceDenom := new(big.Int).Mul(sB, crrBig)

	return priceNum.String(), priceDenom.String(), nil
}
