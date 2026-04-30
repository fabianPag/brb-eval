package vc

import (
	"fmt"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/kzg"
)

// HashToFr hashes a byte array to a BN254 scalar field element.
func HashToFr(data []byte) fr.Element {
	dst := []byte("VC_FR_HASH")
	elements, err := fr.Hash(data, dst, 1)

	if err != nil {
		panic("failed to hash to field: " + err.Error())
	}

	return elements[0]
}

// LagrangeInterpolate reconstructs a polynomial from n evaluating points.
func LagrangeInterpolate(xs, ys []fr.Element) ([]fr.Element, error) {
	k := len(xs)
	poly := make([]fr.Element, k)

	for i := range k {
		var denominator fr.Element
		denominator.SetOne()
		for j := range k {
			if i == j {
				continue
			}
			var diff fr.Element
			diff.Sub(&xs[i], &xs[j])
			denominator.Mul(&denominator, &diff)
		}

		var invDenom fr.Element
		invDenom.Inverse(&denominator)

		var term fr.Element
		term.Mul(&ys[i], &invDenom)

		basis := []fr.Element{{}}
		basis[0].SetOne()

		for j := 0; j < k; j++ {
			if i == j {
				continue
			}
			newBasis := make([]fr.Element, len(basis)+1)
			for b := 0; b < len(basis); b++ {
				newBasis[b+1].Add(&newBasis[b+1], &basis[b])
				var minusXj fr.Element
				minusXj.Mul(&basis[b], &xs[j])
				newBasis[b].Sub(&newBasis[b], &minusXj)
			}
			basis = newBasis
		}

		for b := 0; b < len(basis); b++ {
			var scaled fr.Element
			scaled.Mul(&basis[b], &term)
			poly[b].Add(&poly[b], &scaled)
		}
	}
	return poly, nil
}

// PolynomialFromEvaluations creates a polynomial P(x) given evaluations at x=1..n
func PolynomialFromEvaluations(evals []fr.Element) ([]fr.Element, error) {
	n := len(evals)
	xs := make([]fr.Element, n)
	for i := range n {
		xs[i].SetUint64(uint64(i + 1))
	}
	return LagrangeInterpolate(xs, evals)
}

// HashAndCommit hashes each shard into a field element, interpolates the polynomial
// with those evaluations at x=1..n, and returns the resulting KZG commitment and polynomial.
func HashAndCommit(shards [][]byte, pk kzg.ProvingKey) (kzg.Digest, []fr.Element, error) {
	if len(shards) == 0 {
		return kzg.Digest{}, nil, fmt.Errorf("no shards provided")
	}

	evals := make([]fr.Element, len(shards))
	for i, shard := range shards {
		evals[i] = HashToFr(shard)
	}

	poly, err := PolynomialFromEvaluations(evals)
	if err != nil {
		return kzg.Digest{}, nil, err
	}

	commitment, err := kzg.Commit(poly, pk)
	if err != nil {
		return kzg.Digest{}, nil, err
	}

	return commitment, poly, nil
}

// Open generates an inclusion proof for the polynomial evaluated at x=i
func Open(poly []fr.Element, index int, pk kzg.ProvingKey) (kzg.OpeningProof, error) {
	var point fr.Element
	point.SetUint64(uint64(index))
	return kzg.Open(poly, point, pk)
}

// Verify checks the validity of a fragment with respect to the vector commitment
func Verify(commitment kzg.Digest, proof kzg.OpeningProof, vk kzg.VerifyingKey, claimedValue fr.Element, index int) bool {
	if !proof.ClaimedValue.Equal(&claimedValue) {
		return false
	}

	var point fr.Element
	point.SetUint64(uint64(index))

	err := kzg.Verify(&commitment, &proof, point, vk)
	return err == nil
}
