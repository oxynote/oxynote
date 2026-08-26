// Package mockmetrics implements the random distributions the demo metrics
// and demo database rows are sampled from.
package mockmetrics

import (
	"math"
	"math/rand"
)

// Normal returns a sample from N(mean, stddev) via Box-Muller transform.
func Normal(r *rand.Rand, mean, stddev float64) float64 {
	u1 := r.Float64()
	u2 := r.Float64()
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)

	return mean + stddev*z
}

// Clamp returns x clamped to [lo, hi].
func Clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}

	if x > hi {
		return hi
	}

	return x
}

// _poissonNormalLambda is the lambda above which Poisson switches from
// Knuth's algorithm to a Normal approximation.
const _poissonNormalLambda = 50

// Poisson returns a Poisson-distributed sample with the given lambda.
// Knuth's algorithm is used for small lambda; above _poissonNormalLambda a
// Normal approximation (mean=lambda, stddev=sqrt(lambda)) is used instead,
// which is accurate and avoids the O(lambda) loop cost at large scale.
func Poisson(r *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}

	if lambda > _poissonNormalLambda {
		v := int(math.Round(Normal(r, lambda, math.Sqrt(lambda))))
		if v < 0 {
			// NOCOV: reaching this needs a sample more than sqrt(lambda)
			// deviations below the mean, which past lambda 50 is a
			// once-in-a-trillion draw.
			return 0
		}

		return v
	}

	limit := math.Exp(-lambda)
	k := 0
	p := 1.0

	for p > limit {
		k++
		p *= r.Float64()
	}

	return k - 1
}

// RandCount returns a random non-negative integer sampled from N(mean, mean*relStdDev).
// Use this instead of Poisson when you want visually interesting chart variation rather
// than statistically correct Poisson noise (Poisson stddev = sqrt(lambda) ≈ 1% at large
// counts, which produces near-flat charts).
func RandCount(r *rand.Rand, mean, relStdDev float64) int {
	v := int(math.Round(Normal(r, mean, mean*relStdDev)))
	if v < 0 {
		return 0
	}

	return v
}

// NewRand returns a new rand seeded with seed.
func NewRand(seed int64) *rand.Rand {
	//nolint:gosec // demo data wants a cheap reproducible source, not a cryptographic one
	return rand.New(rand.NewSource(seed))
}
