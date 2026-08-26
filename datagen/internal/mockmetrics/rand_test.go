package mockmetrics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Normal(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Mean   float64
		StdDev float64
	}{
		"Zero deviation always returns the mean": {
			Mean: 42,
		},
		"Non-zero deviation spreads around the mean": {
			Mean:   100,
			StdDev: 15,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := NewRand(1)

			// a zero deviation collapses the distribution onto the mean,
			// which is the only exact assertion available here.
			if c.StdDev == 0 {
				assert.InDelta(t, c.Mean, Normal(r, c.Mean, c.StdDev), 1e-9)
				return
			}

			// otherwise the sample mean of a large draw must land near the
			// requested mean, and the samples must not all be identical.
			var (
				sum   float64
				first = Normal(r, c.Mean, c.StdDev)
				same  = true
			)

			sum += first

			for range 9_999 {
				v := Normal(r, c.Mean, c.StdDev)
				if v != first {
					same = false
				}

				sum += v
			}

			assert.False(t, same)
			assert.InDelta(t, c.Mean, sum/10_000, c.StdDev)
		})
	}
}

func Test_Clamp(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		X      float64
		Lo     float64
		Hi     float64
		Result float64
	}{
		"Below the lower bound": {
			X:      -5,
			Lo:     0,
			Hi:     10,
			Result: 0,
		},
		"Above the upper bound": {
			X:      15,
			Lo:     0,
			Hi:     10,
			Result: 10,
		},
		"Within the bounds": {
			X:      5,
			Lo:     0,
			Hi:     10,
			Result: 5,
		},
		"Exactly on the lower bound": {
			X:      0,
			Lo:     0,
			Hi:     10,
			Result: 0,
		},
		"Exactly on the upper bound": {
			X:      10,
			Lo:     0,
			Hi:     10,
			Result: 10,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, c.Result, Clamp(c.X, c.Lo, c.Hi), 1e-9)
		})
	}
}

func Test_Poisson(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Lambda float64

		// Exact holds the single value every draw must produce, used for the
		// degenerate lambdas that cannot vary.
		Exact *int

		// Draws is how many samples the statistical assertions run over.
		Draws int
	}{
		"Zero lambda never produces an event": {
			Lambda: 0,
			Exact:  new(0),
		},
		"Negative lambda never produces an event": {
			Lambda: -1,
			Exact:  new(0),
		},
		"Small lambda goes through Knuth's algorithm": {
			Lambda: 5,
			Draws:  10_000,
		},
		"Lambda on the switchover boundary still uses Knuth": {
			Lambda: _poissonNormalLambda,
			Draws:  10_000,
		},
		"Large lambda goes through the normal approximation": {
			Lambda: 5_000,
			Draws:  10_000,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := NewRand(1)

			if c.Exact != nil {
				assert.Equal(t, *c.Exact, Poisson(r, c.Lambda))
				return
			}

			var sum int

			for range c.Draws {
				v := Poisson(r, c.Lambda)
				require.GreaterOrEqual(t, v, 0)

				sum += v
			}

			// the mean of a Poisson draw is lambda itself; allow a generous
			// band so the assertion never races the sampler.
			assert.InDelta(t, c.Lambda, float64(sum)/float64(c.Draws), math.Sqrt(c.Lambda))
		})
	}
}

func Test_RandCount(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Mean      float64
		RelStdDev float64

		// Exact holds the value every draw must produce when the parameters
		// leave no room for variation.
		Exact *int
	}{
		"Zero deviation returns the rounded mean": {
			Mean:  1_000,
			Exact: new(1_000),
		},
		"Negative mean is clamped to zero": {
			Mean:  -50,
			Exact: new(0),
		},
		"Spread mean stays non-negative": {
			Mean:      500,
			RelStdDev: 0.2,
		},
		"Deviation wide enough to undershoot zero is clamped": {
			Mean:      10,
			RelStdDev: 2,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := NewRand(1)

			if c.Exact != nil {
				assert.Equal(t, *c.Exact, RandCount(r, c.Mean, c.RelStdDev))
				return
			}

			var sum int

			for range 10_000 {
				v := RandCount(r, c.Mean, c.RelStdDev)
				require.GreaterOrEqual(t, v, 0)

				sum += v
			}

			assert.InDelta(t, c.Mean, float64(sum)/10_000, c.Mean*c.RelStdDev)
		})
	}
}

func Test_NewRand(t *testing.T) {
	t.Parallel()

	// the same seed replays the same sequence.
	a, b := NewRand(7), NewRand(7)
	require.NotNil(t, a)
	require.NotNil(t, b)

	for range 100 {
		assert.InDelta(t, a.Float64(), b.Float64(), 1e-12)
	}

	// a different seed does not.
	assert.NotEqual(t, NewRand(7).Float64(), NewRand(8).Float64())
}
