package ts

import (
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// HashToG1 hashes an arbitrary message to a point on the G1 curve.
func HashToG1(message []byte) (bn254.G1Affine, error) {
	h := sha256.New()
	h.Write(message)
	hashSum := h.Sum(nil)

	var scalar big.Int
	scalar.SetBytes(hashSum)

	_, _, G1Gen, _ := bn254.Generators()
	var point bn254.G1Affine
	point.ScalarMultiplication(&G1Gen, &scalar)
	return point, nil
}

// SignShare creates a partial signature using a node's private share for a given commitment.
func SignShare(commitment []byte, privateShare fr.Element) ([]byte, error) {
	msgPoint, err := HashToG1(commitment)
	if err != nil {
		return nil, err
	}

	var scalar big.Int
	privateShare.BigInt(&scalar)

	var share bn254.G1Affine
	share.ScalarMultiplication(&msgPoint, &scalar)
	return share.Marshal(), nil
}

// VerifyShare checks if a single peer's signature share is valid against their public key.
func VerifyShare(commitment []byte, shareBytes []byte, pubKey bn254.G2Affine) bool {
	var share bn254.G1Affine
	err := share.Unmarshal(shareBytes)
	if err != nil {
		return false
	}

	msgPoint, err := HashToG1(commitment)
	if err != nil {
		return false
	}

	var negMsgPoint bn254.G1Affine
	negMsgPoint.Neg(&msgPoint)

	_, _, _, G2Gen := bn254.Generators()

	res, err := bn254.Pair([]bn254.G1Affine{share, negMsgPoint}, []bn254.G2Affine{G2Gen, pubKey})
	if err != nil {
		return false
	}

	var one bn254.GT
	one.SetOne()
	return res.Equal(&one)
}

// Combine aggregates `t` valid shares into a single master threshold signature via Lagrange interpolation in G1.
func Combine(shares map[int][]byte, targetQuorum int) ([]byte, error) {
	if len(shares) < targetQuorum {
		return nil, fmt.Errorf("insufficient shares %d < %d", len(shares), targetQuorum)
	}

	xs := make([]fr.Element, 0, targetQuorum)
	ys := make([]bn254.G1Affine, 0, targetQuorum)

	count := 0
	for nodeIndex, shareBytes := range shares {
		if count >= targetQuorum {
			break
		}
		var x fr.Element
		x.SetUint64(uint64(nodeIndex + 1)) // Node index map directly to polynomial evaluation points (1-indexed)

		var y bn254.G1Affine
		if err := y.Unmarshal(shareBytes); err != nil {
			return nil, fmt.Errorf("invalid share format for node %d", nodeIndex)
		}

		xs = append(xs, x)
		ys = append(ys, y)
		count++
	}

	var result bn254.G1Affine

	for i := range targetQuorum {
		var num, den fr.Element
		num.SetOne()
		den.SetOne()

		for j := range targetQuorum {
			if i != j {
				// numerator *= (0 - x_j)
				var negXj fr.Element
				negXj.Neg(&xs[j])
				num.Mul(&num, &negXj)

				// denominator *= (x_i - x_j)
				var diff fr.Element
				diff.Sub(&xs[i], &xs[j])
				den.Mul(&den, &diff)
			}
		}

		var basis, invDen fr.Element
		invDen.Inverse(&den)
		basis.Mul(&num, &invDen)

		var basisBig big.Int
		basis.BigInt(&basisBig)

		var scaledShare bn254.G1Affine
		scaledShare.ScalarMultiplication(&ys[i], &basisBig)

		if i == 0 {
			result.Set(&scaledShare)
		} else {
			result.Add(&result, &scaledShare)
		}
	}

	return result.Marshal(), nil
}

// Verify checks the aggregated signature against the Master Public Key.
func Verify(commitment []byte, thresholdSignature []byte, masterPubKey bn254.G2Affine) bool {
	return VerifyShare(commitment, thresholdSignature, masterPubKey)
}
