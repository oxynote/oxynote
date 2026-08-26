package demo

import (
	"math"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testWalk is a walk whose bounds are wide enough that clamping never
// hides a mistake in the replay itself.
var _testWalk = walkParams{
	Min:           0,
	Max:           1_000,
	Start:         100,
	Target:        120,
	MeanReversion: 0.05,
	NoiseStdDev:   5,
	DriftPerStep:  0.1,
	SpikeChance:   0.05,
	SpikeStdDev:   20,
}

func Test_tickAt(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Time   time.Time
		Result int64
	}{
		"The epoch itself":     {Time: _epoch, Result: 0},
		"Part way into a tick": {Time: _epoch.Add(30 * time.Second), Result: 0},
		"The next tick":        {Time: _epoch.Add(time.Minute), Result: 1},
		"An hour in":           {Time: _epoch.Add(time.Hour), Result: 60},
		"A day in":             {Time: _epoch.Add(24 * time.Hour), Result: _segment},
		"Before the epoch": {
			Time:   _epoch.Add(-time.Hour),
			Result: -60,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, tickAt(c.Time))
		})
	}
}

func Test_tickAtMillis(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Millis int64
		Result int64
	}{
		"The epoch itself":     {Millis: _epoch.UnixMilli(), Result: 0},
		"Part way into a tick": {Millis: _epoch.UnixMilli() + 30_000, Result: 0},
		"The next tick":        {Millis: _epoch.UnixMilli() + 60_000, Result: 1},
		"A day in":             {Millis: _epoch.Add(24 * time.Hour).UnixMilli(), Result: _segment},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, tickAtMillis(c.Millis))
		})
	}
}

func Test_timeAt(t *testing.T) {
	t.Parallel()

	assert.Equal(t, _epoch, timeAt(0))
	assert.Equal(t, _epoch.Add(time.Minute), timeAt(1))

	// the two directions agree, which is what lets a sample timestamp be
	// turned back into the tick that produced its value.
	for _, tick := range []int64{0, 1, 59, _segment, 1_000_000} {
		assert.Equal(t, tick, tickAt(timeAt(tick)))
	}
}

func Test_latestTick(t *testing.T) {
	t.Parallel()

	got := latestTick()

	// nothing past the present is ever sampled, and the timeline has
	// reached the present.
	assert.Equal(t, tickAt(timeutil.Now()), got)
	assert.Positive(t, got)
	assert.False(t, timeAt(got).After(timeutil.Now()))
}

func Test_newRand(t *testing.T) {
	t.Parallel()

	// the same seed and stream replay the same numbers, which is the
	// whole basis of a frozen history.
	assert.Equal(t, newRand(1, 2).Float64(), newRand(1, 2).Float64())

	// a different seed or a different stream is a different history.
	assert.NotEqual(t, newRand(1, 2).Float64(), newRand(2, 2).Float64())
	assert.NotEqual(t, newRand(1, 2).Float64(), newRand(1, 3).Float64())

	// neighbouring series do not overlap: seed 1's stream 0 is not seed
	// 0's stream _seedStride read one segment later.
	assert.NotEqual(t, newRand(1, 0).Float64(), newRand(0, 1).Float64())
}

func Test_normal(t *testing.T) {
	t.Parallel()

	// with no spread every draw is the mean.
	assert.InDelta(t, 7.0, normal(newRand(1, 1), 7, 0), 1e-9)

	// the same stream draws the same number twice.
	assert.Equal(t, normal(newRand(1, 1), 0, 1), normal(newRand(1, 1), 0, 1))

	// over many draws the sample mean lands near the stated mean.
	r := newRand(3, 3)
	sum := 0.0

	for range 10_000 {
		sum += normal(r, 5, 2)
	}

	assert.InDelta(t, 5.0, sum/10_000, 0.1)
}

func Test_clamp(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		X      float64
		Lo     float64
		Hi     float64
		Result float64
	}{
		"Within the bounds": {X: 5, Lo: 0, Hi: 10, Result: 5},
		"Below the floor":   {X: -1, Lo: 0, Hi: 10, Result: 0},
		"Above the ceiling": {X: 11, Lo: 0, Hi: 10, Result: 10},
		"On the floor":      {X: 0, Lo: 0, Hi: 10, Result: 0},
		"On the ceiling":    {X: 10, Lo: 0, Hi: 10, Result: 10},
		"Infinite":          {X: math.Inf(1), Lo: 0, Hi: 10, Result: 10},
		"Negative infinite": {X: math.Inf(-1), Lo: 0, Hi: 10, Result: 0},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, clamp(c.X, c.Lo, c.Hi))
		})
	}
}

func Test_countAt(t *testing.T) {
	t.Parallel()

	// a count depends on its tick alone, so reading it again answers the
	// same and reading a neighbour does not.
	assert.Equal(t, countAt(1, 10, 1_000, 0.15), countAt(1, 10, 1_000, 0.15))
	assert.NotEqual(t, countAt(1, 10, 1_000, 0.15), countAt(1, 11, 1_000, 0.15))
	assert.NotEqual(t, countAt(1, 10, 1_000, 0.15), countAt(2, 10, 1_000, 0.15))

	// counts are whole and never negative, however wide the spread.
	for tick := range int64(500) {
		v := countAt(9, tick, 4, 5)

		assert.GreaterOrEqual(t, v, 0.0)
		assert.Equal(t, math.Trunc(v), v)
	}

	// across many ticks the average lands near the stated mean.
	sum := 0.0

	for tick := range int64(5_000) {
		sum += countAt(4, tick, 2_000, 0.15)
	}

	assert.InDelta(t, 2_000.0, sum/5_000, 50)
}

func Test_replay(t *testing.T) {
	t.Parallel()

	// no steps is no movement.
	assert.Equal(t, 100.0, replay(100, _testWalk, newRand(1, 1), 0))

	// the same start and stream replay to the same value.
	assert.Equal(
		t,
		replay(100, _testWalk, newRand(1, 1), 50),
		replay(100, _testWalk, newRand(1, 1), 50),
	)

	// every step clamps, so a walk started far outside its bounds is
	// inside them again after one.
	p := _testWalk
	p.Min = 10
	p.Max = 20

	assert.LessOrEqual(t, replay(1_000, p, newRand(1, 1), 1), 20.0)
	assert.GreaterOrEqual(t, replay(-1_000, p, newRand(1, 1), 1), 10.0)
}

func Test_newWalk(t *testing.T) {
	t.Parallel()

	w := newWalk(42, _testWalk)
	require.NotNil(t, w)

	assert.Equal(t, int64(42), w.seed)
	assert.Equal(t, _testWalk, w.params)

	// nothing is computed until something is asked for.
	assert.Empty(t, w.checkpoints)
}

func Test_walk_at(t *testing.T) {
	t.Parallel()

	// two walks built from the same seed answer identically, which is
	// what makes the history the same on every install rather than only
	// within one process.
	a, b := newWalk(5, _testWalk), newWalk(5, _testWalk)

	for _, tick := range []int64{0, 1, 100, _segment - 1, _segment, _segment + 1, 3*_segment + 7} {
		assert.Equal(t, a.at(tick), b.at(tick), "tick %d", tick)
	}

	// reading out of order answers the same as reading in order: a value
	// depends on its tick, not on what was read before it.
	c := newWalk(5, _testWalk)
	assert.Equal(t, a.at(2*_segment+3), c.at(2*_segment+3))

	// the walk starts where it was told to and stays inside its bounds.
	assert.Equal(t, _testWalk.Start, a.at(0))

	for _, tick := range []int64{1, 500, _segment, 5_000} {
		assert.GreaterOrEqual(t, a.at(tick), _testWalk.Min)
		assert.LessOrEqual(t, a.at(tick), _testWalk.Max)
	}

	// a tick before the epoch reads as the epoch rather than running the
	// walk backwards.
	assert.Equal(t, a.at(0), a.at(-10))

	// a segment boundary is continuous: the first tick of a segment is
	// the value the previous segment ended on.
	assert.Equal(t, a.checkpoint(1), a.at(_segment))
}

func Test_walk_checkpoint(t *testing.T) {
	t.Parallel()

	w := newWalk(11, _testWalk)

	// the first checkpoint is the start value, clamped.
	assert.Equal(t, _testWalk.Start, w.checkpoint(0))
	assert.Len(t, w.checkpoints, 1)

	// asking for a later one fills in every segment before it, once.
	assert.NotZero(t, w.checkpoint(3))
	assert.Len(t, w.checkpoints, 4)

	cached := w.checkpoints[3]
	assert.Equal(t, cached, w.checkpoint(3))
	assert.Len(t, w.checkpoints, 4)

	// a start outside the bounds is pulled inside before anything runs.
	p := _testWalk
	p.Start = 5_000

	assert.Equal(t, p.Max, newWalk(12, p).checkpoint(0))
}
