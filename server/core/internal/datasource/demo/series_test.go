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
		"A day in":             {Time: _epoch.Add(24 * time.Hour), Result: 1440},
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
		"A day in":             {Millis: _epoch.Add(24 * time.Hour).UnixMilli(), Result: 1440},
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
	for _, tick := range []int64{0, 1, 59, 1440, 1_000_000} {
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

	cc := map[string]struct {
		Seed      int64
		Mean      float64
		RelStdDev float64
		Check     func(t *testing.T, count func(tick int64) float64)
	}{
		"A count depends on its tick alone": {
			Seed:      1,
			Mean:      1_000,
			RelStdDev: 0.15,
			Check: func(t *testing.T, count func(tick int64) float64) {
				// reading it again answers the same and reading a
				// neighbour does not.
				assert.Equal(t, count(10), count(10))
				assert.NotEqual(t, count(10), count(11))
				assert.NotEqual(t, count(10), countAt(2, 10, 1_000, 0.15))
			},
		},
		"A spread wide enough to draw below zero reads zero instead": {
			Seed:      9,
			Mean:      4,
			RelStdDev: 5,
			Check: func(t *testing.T, count func(tick int64) float64) {
				// counts are whole and never negative, however wide the
				// spread.
				for tick := range int64(500) {
					v := count(tick)

					assert.GreaterOrEqual(t, v, 0.0)
					assert.Equal(t, math.Trunc(v), v)
				}
			},
		},
		"Many ticks average near the mean": {
			Seed:      4,
			Mean:      2_000,
			RelStdDev: 0.15,
			Check: func(t *testing.T, count func(tick int64) float64) {
				sum := 0.0

				for tick := range int64(5_000) {
					sum += count(tick)
				}

				assert.InDelta(t, 2_000.0, sum/5_000, 50)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Check(t, func(tick int64) float64 {
				return countAt(c.Seed, tick, c.Mean, c.RelStdDev)
			})
		})
	}
}

func Test_endpoint(t *testing.T) {
	t.Parallel()

	const draws = 4_000

	// the closed form has to agree with the walk it stands in for: a
	// segment stepped out in full ends where endpoint says it does, in
	// distribution. Both are sampled over many streams and compared on
	// mean and spread.
	var stepSum, stepSq, drawSum, drawSq float64

	for i := range int64(draws) {
		v := 100.0
		r := newRand(1, i)

		for range _segment {
			v = step(v, _testWalk, r)
		}

		d := endpoint(100, _testWalk, newRand(2, i))

		stepSum += v
		stepSq += v * v
		drawSum += d
		drawSq += d * d
	}

	sm, dm := stepSum/draws, drawSum/draws
	ss, ds := math.Sqrt(stepSq/draws-sm*sm), math.Sqrt(drawSq/draws-dm*dm)

	assert.InDelta(t, sm, dm, 2)
	assert.InDelta(t, ss, ds, ss*0.15)

	// a day is long enough for the start to be forgotten, so the walk
	// ends around the level drift and mean reversion balance at.
	assert.InDelta(t, _testWalk.Target+_testWalk.DriftPerStep/_testWalk.MeanReversion, dm, 2)

	// the draw is clamped like every step is.
	p := _testWalk
	p.Min = 10
	p.Max = 20

	assert.LessOrEqual(t, endpoint(1_000, p, newRand(1, 1)), 20.0)
	assert.GreaterOrEqual(t, endpoint(-1_000, p, newRand(1, 1)), 10.0)
}

func Test_replay(t *testing.T) {
	t.Parallel()

	// a segment is its start value, every step of it, and the end it
	// was told to reach.
	vv := replay(100, 130, _testWalk, newRand(1, 1))
	require.Len(t, vv, _segment+1)
	assert.Equal(t, 100.0, vv[0])
	assert.NotEqual(t, vv[0], vv[1])
	assert.Equal(t, 130.0, vv[_segment])

	// the same start, end and stream replay to the same values.
	assert.Equal(t, vv, replay(100, 130, _testWalk, newRand(1, 1)))

	// every value stays inside the bounds, tilt included.
	for _, v := range vv {
		assert.GreaterOrEqual(t, v, _testWalk.Min)
		assert.LessOrEqual(t, v, _testWalk.Max)
	}
}

func Test_step(t *testing.T) {
	t.Parallel()

	narrow := _testWalk
	narrow.Min = 10
	narrow.Max = 20

	cc := map[string]struct {
		Value  float64
		Params walkParams
	}{
		"A value inside the bounds": {
			Value:  100,
			Params: _testWalk,
		},
		"A value far above the bounds": {
			Value:  1_000,
			Params: narrow,
		},
		"A value far below the bounds": {
			Value:  -1_000,
			Params: narrow,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v := step(c.Value, c.Params, newRand(1, 1))

			// the same start and stream step to the same value.
			assert.Equal(t, step(c.Value, c.Params, newRand(1, 1)), v)

			// every step clamps, so a walk started far outside its bounds
			// is inside them again after one.
			assert.GreaterOrEqual(t, v, c.Params.Min)
			assert.LessOrEqual(t, v, c.Params.Max)
		})
	}
}

func Test_newWalk(t *testing.T) {
	t.Parallel()

	w := newWalk(42, _testWalk)
	require.NotNil(t, w)

	assert.Equal(t, int64(42), w.seed)
	assert.Equal(t, _testWalk, w.params)

	// nothing is computed until something is asked for.
	assert.Empty(t, w.checkpoints)
	assert.Empty(t, w.segments)
}

func Test_walk_at(t *testing.T) {
	t.Parallel()

	boundary := int64(_segment * _walkStride)

	cc := map[string]struct {
		ReadFirst []int64
		Tick      int64
		Check     func(t *testing.T, w *walk, v float64)
	}{
		"The epoch reads the start value": {
			Tick: 0,
			Check: func(t *testing.T, _ *walk, v float64) {
				assert.Equal(t, _testWalk.Start, v)
			},
		},
		"A tick before the epoch reads as the epoch": {
			Tick: -10,
			Check: func(t *testing.T, w *walk, v float64) {
				// rather than running the walk backwards.
				assert.Equal(t, w.at(0), v)
			},
		},
		"A tick between two walk steps": {
			Tick: 1,
		},
		"A tick on a walk step": {
			Tick: 100,
		},
		"A tick later in the first segment": {
			Tick: 500,
		},
		"The last tick before a segment boundary": {
			Tick: boundary - 1,
		},
		"A segment boundary joins the segments either side": {
			Tick: boundary,
			Check: func(t *testing.T, w *walk, v float64) {
				// the first tick of a segment is the value the previous
				// segment ended on.
				assert.Equal(t, w.segment(0)[_segment], v)
				assert.Equal(t, w.segment(1)[0], v)
			},
		},
		"The first tick after a segment boundary": {
			Tick: boundary + 1,
		},
		"A tick between steps three segments in": {
			Tick: 3*boundary + 7,
		},
		"A tick on a step three segments in": {
			Tick: 5_000,
		},
		"A tick read out of order answers the same as one read first": {
			ReadFirst: []int64{0, 1, 100, boundary - 1, boundary, boundary + 1, 3*boundary + 7},
			Tick:      2*boundary + 3,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w := newWalk(5, _testWalk)

			for _, tick := range c.ReadFirst {
				w.at(tick)
			}

			v := w.at(c.Tick)

			// a walk built from the same seed answers identically,
			// whatever was read from it before. That is what makes the
			// history the same on every install rather than only within
			// one process.
			assert.Equal(t, newWalk(5, _testWalk).at(c.Tick), v)

			// the walk stays inside its bounds.
			assert.GreaterOrEqual(t, v, _testWalk.Min)
			assert.LessOrEqual(t, v, _testWalk.Max)

			if c.Check != nil {
				c.Check(t, w, v)
			}
		})
	}
}

func Test_walk_atStep(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Step    int64
		Segment int64
		Offset  int64
	}{
		"The first step opens the first segment": {
			Step:    0,
			Segment: 0,
			Offset:  0,
		},
		"A step inside the first segment": {
			Step:    7,
			Segment: 0,
			Offset:  7,
		},
		"The last step of a segment": {
			Step:    _segment - 1,
			Segment: 0,
			Offset:  _segment - 1,
		},
		"The first step of a later segment": {
			Step:    2 * _segment,
			Segment: 2,
			Offset:  0,
		},
		"A step part way into a later segment": {
			Step:    3*_segment + 5,
			Segment: 3,
			Offset:  5,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w := newWalk(5, _testWalk)
			v := w.atStep(c.Step)

			// only the segment the step falls in is replayed.
			assert.Len(t, w.segments, 1)
			assert.Contains(t, w.segments, c.Segment)

			assert.Equal(t, newWalk(5, _testWalk).segment(c.Segment)[c.Offset], v)

			// step n is what tick n*_walkStride reads, with nothing to
			// interpolate.
			assert.Equal(t, w.at(c.Step*_walkStride), v)
		})
	}
}

func Test_walk_segment(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Reads int
	}{
		"A first read replays the segment and caches it": {
			Reads: 1,
		},
		"A second read is the cached slice": {
			Reads: 2,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w := newWalk(11, _testWalk)
			s := w.segment(3)

			for range c.Reads - 1 {
				assert.Equal(t, s, w.segment(3))
			}

			// reading a segment caches it and the checkpoints leading up
			// to it, and nothing else.
			require.Len(t, s, _segment+1)
			assert.Len(t, w.segments, 1)
			assert.Len(t, w.checkpoints, 4)

			// a segment starts on its checkpoint and ends on the next.
			assert.Equal(t, w.checkpoints[3], s[0])
			assert.Equal(t, w.segment(4)[0], s[_segment])
		})
	}
}

func Test_walk_checkpoint(t *testing.T) {
	t.Parallel()

	outside := _testWalk
	outside.Start = 5_000

	cc := map[string]struct {
		Params  walkParams
		Segment int64
		Result  float64
		Len     int
	}{
		"The first checkpoint is the start value": {
			Params:  _testWalk,
			Segment: 0,
			Result:  _testWalk.Start,
			Len:     1,
		},
		// each checkpoint is where the segment before it ends.
		"A later checkpoint fills in every segment before it": {
			Params:  _testWalk,
			Segment: 3,
			Result: endpoint(
				endpoint(endpoint(_testWalk.Start, _testWalk, newRand(11, 0)), _testWalk, newRand(11, 1)),
				_testWalk,
				newRand(11, 2),
			),
			Len: 4,
		},
		"A start outside the bounds is pulled inside before anything runs": {
			Params:  outside,
			Segment: 0,
			Result:  outside.Max,
			Len:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			w := newWalk(11, c.Params)

			assert.Equal(t, c.Result, w.checkpoint(c.Segment))
			assert.Len(t, w.checkpoints, c.Len)

			// asking again answers the cached value and computes nothing.
			assert.Equal(t, c.Result, w.checkpoint(c.Segment))
			assert.Len(t, w.checkpoints, c.Len)
		})
	}
}
