package demo

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	var (
		startMs = _epoch.Add(time.Hour).UnixMilli()
		endMs   = _epoch.Add(2 * time.Hour).UnixMilli()
		lastMs  = timeAt(latestTick()).UnixMilli()
	)

	cc := map[string]struct {
		Start int64
		End   int64
		Check func(t *testing.T, ss []chunks.Sample)
	}{
		"A window is covered from its start to its end": {
			Start: startMs,
			End:   endMs,
			Check: func(t *testing.T, ss []chunks.Sample) {
				require.NotEmpty(t, ss)

				// one sample a step apart, each carrying the value of the
				// tick it lands in.
				assert.Len(t, ss, 61)
				assert.Equal(t, startMs, ss[0].T())
				assert.Equal(t, endMs, ss[len(ss)-1].T())

				for i, s := range ss {
					assert.Equal(t, float64(tickAtMillis(s.T())), s.F())

					if i > 0 {
						assert.Equal(t, _tickMillis, s.T()-ss[i-1].T())
					}
				}
			},
		},
		"A window starting mid tick starts on the sample before it": {
			Start: startMs + 30_000,
			End:   endMs,
			Check: func(t *testing.T, ss []chunks.Sample) {
				// the first point of the evaluation grid has something to
				// look back at.
				require.NotEmpty(t, ss)
				assert.LessOrEqual(t, ss[0].T(), startMs+30_000)
			},
		},
		"A window reaching past the present stops at it": {
			Start: lastMs - 10*_tickMillis,
			End:   lastMs + time.Hour.Milliseconds(),
			Check: func(t *testing.T, ss []chunks.Sample) {
				// the present is read after sampling, because the minute
				// may have turned since the window was built.
				require.NotEmpty(t, ss)
				assert.LessOrEqual(t, ss[len(ss)-1].T(), timeAt(latestTick()).UnixMilli())

				// it still reaches the present the window was built from.
				assert.GreaterOrEqual(t, ss[len(ss)-1].T(), lastMs)
			},
		},
		"A window before the epoch produces nothing": {
			Start: _epoch.Add(-10 * time.Hour).UnixMilli(),
			End:   _epoch.Add(-5 * time.Hour).UnixMilli(),
			Check: func(t *testing.T, ss []chunks.Sample) {
				// rather than running the timeline backwards.
				assert.Empty(t, ss)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the value is the tick itself, so a check can tell which tick
			// produced a sample.
			c.Check(t, samplesIn(func(tick int64) float64 { return float64(tick) }, c.Start, c.End, _tickMillis))
		})
	}
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
		startMs = _epoch.Add(time.Hour).UnixMilli()
		endMs   = _epoch.Add(2 * time.Hour).UnixMilli()
		name    = _namespace + "deploy_confidence_index"
	)

	q := &querier{registry: newRegistry(), mint: startMs, maxt: endMs}

	cc := map[string]struct {
		Sort   bool
		Hints  *storage.SelectHints
		Name   string
		Result []string
	}{
		"Sorted series sampled across the hinted window": {
			Sort:   true,
			Hints:  &storage.SelectHints{Start: startMs, End: endMs, Step: _tickMillis},
			Name:   name,
			Result: []string{"feeling_lucky", "nervous", "oncall_already_paged"},
		},
		"No hints fall back to the querier bounds": {
			Name:   name,
			Result: []string{"feeling_lucky", "nervous", "oncall_already_paged"},
		},
		"A matcher naming nothing yields an empty set": {
			Name: "nope",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			set := q.Select(context.Background(), c.Sort, c.Hints, labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, c.Name))

			require.NoError(t, set.Err())
			assert.Empty(t, set.Warnings())

			var got []string

			for set.Next() {
				s := set.At()
				got = append(got, s.Labels().Get("vibe"))

				assert.Equal(t, c.Name, s.Labels().Get(model.MetricNameLabel))

				// every series carries the samples of the window the
				// hints or the bounds ask about.
				var samples int

				it := s.Iterator(nil)
				for it.Next() == chunkenc.ValFloat {
					ts, _ := it.At()

					assert.GreaterOrEqual(t, ts, startMs)
					assert.LessOrEqual(t, ts, endMs)

					samples++
				}

				require.NoError(t, it.Err())
				assert.Equal(t, 61, samples)
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_querier_LabelNames(t *testing.T) {
	t.Parallel()

	names, annos, err := (&querier{registry: newRegistry()}).LabelNames(
		context.Background(),
		nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"deploy_confidence_index"),
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
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"deploy_confidence_index"),
	)

	require.NoError(t, err)
	assert.Empty(t, annos)
	assert.Equal(t, []string{"feeling_lucky", "nervous", "oncall_already_paged"}, values)
}

func Test_querier_Close(t *testing.T) {
	t.Parallel()

	assert.NoError(t, (&querier{}).Close())
}

func Test_seriesSet_Next(t *testing.T) {
	t.Parallel()

	one := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "one"), nil)
	two := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "two"), nil)

	ss := &seriesSet{series: []storage.Series{one, two}, index: -1}

	require.True(t, ss.Next())
	assert.Equal(t, 0, ss.index)

	require.True(t, ss.Next())
	assert.Equal(t, 1, ss.index)

	// the set ends, and stays ended.
	assert.False(t, ss.Next())
	assert.False(t, ss.Next())

	// an empty set reports nothing rather than panicking on the first
	// read.
	empty := &seriesSet{index: -1}
	assert.False(t, empty.Next())
}

func Test_seriesSet_At(t *testing.T) {
	t.Parallel()

	one := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "one"), nil)
	two := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "two"), nil)

	ss := &seriesSet{series: []storage.Series{one, two}, index: 0}
	assert.Same(t, one, ss.At())

	ss.index = 1
	assert.Same(t, two, ss.At())
}

func Test_seriesSet_Err(t *testing.T) {
	t.Parallel()

	assert.NoError(t, (&seriesSet{index: -1}).Err())
}

func Test_seriesSet_Warnings(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&seriesSet{index: -1}).Warnings())
}

func Test_floatSample_T(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int64(1_700_000_000_000), floatSample{t: 1_700_000_000_000, f: 42.5}.T())
}

func Test_floatSample_ST(t *testing.T) {
	t.Parallel()

	// a synthesized gauge reading carries no start timestamp.
	assert.Zero(t, floatSample{t: 1_700_000_000_000, f: 42.5}.ST())
}

func Test_floatSample_F(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 42.5, floatSample{t: 1_700_000_000_000, f: 42.5}.F())
}

func Test_floatSample_H(t *testing.T) {
	t.Parallel()

	// a gauge reading carries no histogram of either shape.
	assert.Nil(t, floatSample{t: 1_700_000_000_000, f: 42.5}.H())
}

func Test_floatSample_FH(t *testing.T) {
	t.Parallel()

	assert.Nil(t, floatSample{t: 1_700_000_000_000, f: 42.5}.FH())
}

func Test_floatSample_Type(t *testing.T) {
	t.Parallel()

	assert.Equal(t, chunkenc.ValFloat, floatSample{t: 1_700_000_000_000, f: 42.5}.Type())
}

func Test_floatSample_Copy(t *testing.T) {
	t.Parallel()

	s := floatSample{t: 1_700_000_000_000, f: 42.5}

	// it holds no references, so a copy is the value itself.
	assert.Equal(t, s, s.Copy())
}
