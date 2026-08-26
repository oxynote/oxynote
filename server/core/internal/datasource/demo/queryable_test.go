package demo

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingValue returns a value function reporting the tick it was asked
// about, so a caller can tell which tick produced a sample.
func countingValue(tick int64) float64 {
	return float64(tick)
}

func Test_selectStep(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Hints  *storage.SelectHints
		Span   int64
		Result int64
	}{
		"No hints at all falls back to the tick": {
			Result: _tickMillis,
		},
		"A step finer than the tick is widened to it": {
			Hints:  &storage.SelectHints{Step: 15_000},
			Result: _tickMillis,
		},
		"A step coarser than the tick is kept": {
			Hints:  &storage.SelectHints{Step: 5 * _tickMillis},
			Result: 5 * _tickMillis,
		},
		"A range selector is sampled several times per window": {
			Hints:  &storage.SelectHints{Step: 100 * _tickMillis, Range: 20 * _tickMillis},
			Result: 5 * _tickMillis,
		},
		"A range shorter than the tick still samples at the tick": {
			Hints:  &storage.SelectHints{Step: 100 * _tickMillis, Range: 1_000},
			Result: _tickMillis,
		},
		"A range wider than the step leaves the step alone": {
			Hints:  &storage.SelectHints{Step: 2 * _tickMillis, Range: 1_000 * _tickMillis},
			Result: 2 * _tickMillis,
		},
		"A span too wide for the cap widens the spacing": {
			Hints:  &storage.SelectHints{Step: _tickMillis},
			Span:   _tickMillis * _maxSamplesPerSeries * 3,
			Result: _tickMillis * 3,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			step := selectStep(c.Hints, c.Span)
			assert.Equal(t, c.Result, step)

			// whatever the inputs, one series never outgrows the cap and
			// never samples finer than the timeline itself.
			assert.GreaterOrEqual(t, step, _tickMillis)
			assert.LessOrEqual(t, c.Span/step, int64(_maxSamplesPerSeries))
		})
	}
}

func Test_samplesIn(t *testing.T) {
	t.Parallel()

	start := _epoch.Add(time.Hour).UnixMilli()
	end := _epoch.Add(2 * time.Hour).UnixMilli()

	ss := samplesIn(countingValue, start, end, _tickMillis)

	require.NotEmpty(t, ss)

	// the window is covered end to end, one sample a step apart, and
	// each carries the value of the tick it lands in.
	assert.Len(t, ss, 61)
	assert.Equal(t, start, ss[0].T())
	assert.Equal(t, end, ss[len(ss)-1].T())

	for i, s := range ss {
		assert.Equal(t, float64(tickAtMillis(s.T())), s.F())

		if i > 0 {
			assert.Equal(t, _tickMillis, s.T()-ss[i-1].T())
		}
	}

	// the first sample sits at or before the window's start, so the
	// first point of the evaluation grid has something to look back at.
	ss = samplesIn(countingValue, start+30_000, end, _tickMillis)
	require.NotEmpty(t, ss)
	assert.LessOrEqual(t, ss[0].T(), start+30_000)

	// nothing past the present is ever sampled.
	last := timeAt(latestTick()).UnixMilli()

	ss = samplesIn(countingValue, last-10*_tickMillis, last+time.Hour.Milliseconds(), _tickMillis)
	require.NotEmpty(t, ss)
	assert.LessOrEqual(t, ss[len(ss)-1].T(), last)

	// a window that lands entirely before the epoch produces nothing
	// rather than running the timeline backwards.
	assert.Empty(t, samplesIn(
		countingValue,
		_epoch.Add(-10*time.Hour).UnixMilli(),
		_epoch.Add(-5*time.Hour).UnixMilli(),
		_tickMillis,
	))
}

func Test_queryable_Querier(t *testing.T) {
	t.Parallel()

	q, err := queryable{}.Querier(10, 20)
	require.NoError(t, err)
	require.NotNil(t, q)

	qr, ok := q.(*querier)
	require.True(t, ok)

	assert.Equal(t, int64(10), qr.mint)
	assert.Equal(t, int64(20), qr.maxt)
}

func Test_querier_Select(t *testing.T) {
	t.Parallel()

	var (
		start = _epoch.Add(time.Hour).UnixMilli()
		end   = _epoch.Add(2 * time.Hour).UnixMilli()
		name  = _namespace + "deploy_confidence_index"
	)

	q := &querier{registry: newRegistry(), mint: start, maxt: end}

	// the matchers narrow the set, and every series in it carries the
	// samples of the window the hints asked about.
	set := q.Select(
		context.Background(),
		true,
		&storage.SelectHints{Start: start, End: end, Step: _tickMillis},
		nameMatcher(t, name),
	)

	require.NoError(t, set.Err())
	assert.Empty(t, set.Warnings())

	var got []string

	for set.Next() {
		s := set.At()
		got = append(got, s.Labels().Get("vibe"))

		assert.Equal(t, name, s.Labels().Get(model.MetricNameLabel))

		var samples int

		it := s.Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			ts, _ := it.At()

			assert.GreaterOrEqual(t, ts, start)
			assert.LessOrEqual(t, ts, end)

			samples++
		}

		require.NoError(t, it.Err())
		assert.Equal(t, 61, samples)
	}

	// sorting was asked for, so the series arrive in label order.
	assert.Equal(t, []string{"feeling_lucky", "nervous", "oncall_already_paged"}, got)

	// with no hints the querier falls back to its own bounds rather than
	// answering with nothing.
	set = q.Select(context.Background(), false, nil, nameMatcher(t, name))

	require.True(t, set.Next())
	assert.NotNil(t, set.At())

	// a matcher naming nothing yields an empty set, not an error.
	set = q.Select(context.Background(), false, nil, nameMatcher(t, "nope"))

	assert.False(t, set.Next())
	require.NoError(t, set.Err())
}

func Test_querier_LabelNames(t *testing.T) {
	t.Parallel()

	names, annos, err := (&querier{registry: newRegistry()}).LabelNames(
		context.Background(),
		nil,
		nameMatcher(t, _namespace+"deploy_confidence_index"),
	)

	require.NoError(t, err)
	assert.Empty(t, annos)
	assert.Equal(t, []string{model.MetricNameLabel, "vibe"}, names)
}

func Test_querier_LabelValues(t *testing.T) {
	t.Parallel()

	values, annos, err := (&querier{registry: newRegistry()}).LabelValues(
		context.Background(),
		"vibe",
		nil,
		nameMatcher(t, _namespace+"deploy_confidence_index"),
	)

	require.NoError(t, err)
	assert.Empty(t, annos)
	assert.Equal(t, []string{"feeling_lucky", "nervous", "oncall_already_paged"}, values)
}

func Test_querier_Close(t *testing.T) {
	t.Parallel()

	assert.NoError(t, (&querier{}).Close())
}

func Test_seriesSet(t *testing.T) {
	t.Parallel()

	one := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "one"), nil)
	two := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "two"), nil)

	ss := &seriesSet{series: []storage.Series{one, two}, index: -1}

	require.True(t, ss.Next())
	assert.Same(t, one, ss.At())

	require.True(t, ss.Next())
	assert.Same(t, two, ss.At())

	// the set ends, and stays ended.
	assert.False(t, ss.Next())
	assert.False(t, ss.Next())

	assert.NoError(t, ss.Err())
	assert.Empty(t, ss.Warnings())

	// an empty set reports nothing rather than panicking on the first
	// read.
	empty := &seriesSet{index: -1}
	assert.False(t, empty.Next())
}

func Test_floatSample(t *testing.T) {
	t.Parallel()

	s := floatSample{t: 1_700_000_000_000, f: 42.5}

	assert.Equal(t, int64(1_700_000_000_000), s.T())
	assert.Equal(t, 42.5, s.F())
	assert.Equal(t, chunkenc.ValFloat, s.Type())

	// a synthesized gauge reading carries no start timestamp and no
	// histogram of either shape.
	assert.Zero(t, s.ST())
	assert.Nil(t, s.H())
	assert.Nil(t, s.FH())

	// it holds no references, so a copy is the value itself.
	assert.Equal(t, s, s.Copy())
}
