package demo

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// _tick is the interval between two demo samples.
const _tick = time.Minute

// _tickMillis is that same interval in the milliseconds the storage layer
// counts in.
const _tickMillis = int64(_tick / time.Millisecond)

// _walkStride is how many ticks pass between two steps of a walk, which
// is the interval the generator these parameters come from advanced on.
// Ticks in between are interpolated, so a walk costs a fifth of what
// stepping it every tick would while reading the same at any tick.
const _walkStride = 5

// _segment is how many walk steps one replay segment spans — a day of
// them. A sample replays at most this many steps from the checkpoint
// before it, which is what keeps a query over a year from walking every
// step since the epoch.
const _segment = 288

// _seedStride separates the random streams of two series whose segment
// numbers differ by one, so no two streams overlap.
const _seedStride = 1_000_003

// _epoch is the instant the demo timeline starts. It is fixed so that a
// tick always denotes the same moment, on every install and every run.
var _epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// tickAt reports which tick covers the given instant.
func tickAt(t time.Time) int64 {
	return int64(t.Sub(_epoch) / _tick)
}

// tickAtMillis reports which tick covers the given millisecond instant.
func tickAtMillis(ms int64) int64 {
	return (ms - _epoch.UnixMilli()) / _tickMillis
}

// timeAt reports the instant the given tick starts at.
func timeAt(tick int64) time.Time {
	return _epoch.Add(time.Duration(tick) * _tick)
}

// latestTick reports the last tick the timeline has reached. Nothing past
// it is ever sampled: the demo has no more happened in the future than a
// real deployment has.
func latestTick() int64 {
	return tickAt(timeutil.Now())
}

// newRand returns the random stream the given seed and stream number
// name. Every segment of every series draws from its own stream, so a
// value never depends on which ticks were read before it.
func newRand(seed, stream int64) *rand.Rand {
	//nolint:gosec // demo data wants a cheap reproducible source, not a cryptographic one
	return rand.New(rand.NewSource(seed*_seedStride + stream))
}

// normal returns a sample from N(mean, stddev) via the Box-Muller
// transform.
func normal(r *rand.Rand, mean, stddev float64) float64 {
	u1 := r.Float64()
	u2 := r.Float64()
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)

	return mean + stddev*z
}

// clamp returns x clamped to [lo, hi].
func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}

	if x > hi {
		return hi
	}

	return x
}

// countAt returns the count the given bucket reports at the given tick.
// A count is resampled from scratch every tick, so it needs no history.
func countAt(seed, tick int64, mean, relStdDev float64) float64 {
	v := math.Round(normal(newRand(seed, tick), mean, mean*relStdDev))
	if v < 0 {
		return 0
	}

	return v
}

// walkParams configures a gauge that evolves each tick via drift and mean
// reversion.
type walkParams struct {
	// Min is the lower bound of the gauge value.
	Min float64

	// Max is the upper bound of the gauge value.
	Max float64

	// Start is the value the walk begins at, at the epoch.
	Start float64

	// DriftPerStep is a constant added each tick, modelling a slow
	// linear trend. Mean reversion absorbs it, so the walk settles
	// around Target+DriftPerStep/MeanReversion rather than running away.
	DriftPerStep float64

	// NoiseStdDev is the standard deviation of the Gaussian noise added
	// each tick.
	NoiseStdDev float64

	// SpikeChance is the probability (0–1) of an additional spike
	// occurring on any given tick.
	SpikeChance float64

	// SpikeStdDev is the standard deviation of the spike magnitude when
	// one occurs.
	SpikeStdDev float64

	// MeanReversion is the fraction (0–1) of the distance to Target
	// pulled back each tick.
	MeanReversion float64

	// Target is the attractor value for mean reversion.
	Target float64
}

// walk replays a mean-reverting random walk. The value at a tick depends
// only on the walk's seed and the tick's distance from the epoch, never on
// which ticks were read before it — so every query sees the same history,
// and a line already drawn never rewrites itself.
type walk struct {
	// params govern the walk's drift, noise and mean reversion.
	params walkParams

	// seed distinguishes this walk's random streams from every other
	// series'.
	seed int64

	// mu guards checkpoints.
	mu sync.Mutex

	// checkpoints holds the walk's value at the start of each segment,
	// grown on demand. Index i is the value at tick i*_segment.
	checkpoints []float64
}

// newWalk creates a fresh instance of walk.
func newWalk(seed int64, params walkParams) *walk {
	return &walk{
		params: params,
		seed:   seed,
	}
}

// at returns the walk's value at the given tick, interpolating between
// the two walk steps it falls between so a chart drawn at tick
// resolution is a line rather than a staircase.
func (w *walk) at(tick int64) float64 {
	if tick < 0 {
		tick = 0
	}

	step := tick / _walkStride

	from := w.atStep(step)
	if tick%_walkStride == 0 {
		return from
	}

	to := w.atStep(step + 1)

	return from + (to-from)*float64(tick%_walkStride)/_walkStride
}

// atStep returns the walk's value at the given step of its own clock.
func (w *walk) atStep(step int64) float64 {
	segment := step / _segment

	return replay(
		w.checkpoint(segment),
		w.params,
		newRand(w.seed, segment),
		int(step%_segment),
	)
}

// checkpoint returns the walk's value at the first step of the given
// segment, computing and caching every segment before it that is not
// cached yet.
func (w *walk) checkpoint(segment int64) float64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.checkpoints) == 0 {
		w.checkpoints = []float64{clamp(w.params.Start, w.params.Min, w.params.Max)}
	}

	for int64(len(w.checkpoints)) <= segment {
		last := int64(len(w.checkpoints)) - 1

		w.checkpoints = append(w.checkpoints, replay(
			w.checkpoints[last],
			w.params,
			newRand(w.seed, last),
			_segment,
		))
	}

	return w.checkpoints[segment]
}

// replay advances v by the given number of the walk's ticks.
func replay(v float64, p walkParams, r *rand.Rand, steps int) float64 {
	for range steps {
		v += p.MeanReversion * (p.Target - v)
		v += p.DriftPerStep + normal(r, 0, p.NoiseStdDev)

		if r.Float64() < p.SpikeChance {
			v += normal(r, 0, p.SpikeStdDev)
		}

		v = clamp(v, p.Min, p.Max)
	}

	return v
}
