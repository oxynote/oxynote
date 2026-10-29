package demo

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _logTick is a tick well inside the demo's history.
var _logTick = tickAt(time.Date(2026, 3, 14, 15, 9, 0, 0, time.UTC))

// stubLogSource returns a source whose metric reads a constant.
func stubLogSource(reading float64) logSource {
	return logSource{
		service: "deploy-bot",
		field:   "outcome",
		bucket:  "attempted",
		level:   processor.LogLevelInfo,
		scale:   10,
		value:   func(int64) float64 { return reading },
		text: func(_ *rand.Rand, fields map[string]string) string {
			fields["sha"] = "3f9a1c2"

			return "deploy started by " + fields[_logFieldUser]
		},
		seed: _logSeedBase,
	}
}

func Test_logSource_streamAt(t *testing.T) {
	t.Parallel()

	s := stubLogSource(1)
	other := s
	other.seed++

	draw := func(src logSource, tick int64) uint64 {
		return src.streamAt(tick).Uint64()
	}

	first, again := draw(s, _logTick), draw(s, _logTick)
	assert.Equal(t, first, again, "a tick draws the same every time")
	assert.NotEqual(t, draw(s, _logTick), draw(s, _logTick+1), "the next tick draws anew")
	assert.NotEqual(t, draw(s, _logTick), draw(other, _logTick), "another source draws anew")
}

func Test_logSource_lineCount(t *testing.T) {
	cc := map[string]struct {
		Reading float64
		Min     int
		Max     int
		Mean    float64
	}{
		"No reading":       {Reading: 0, Min: 0, Max: 0, Mean: 0},
		"Negative reading": {Reading: -50, Min: 0, Max: 0, Mean: 0},
		"Whole lines":      {Reading: 30, Min: 3, Max: 3, Mean: 3},
		"Fractional lines": {Reading: 25, Min: 2, Max: 3, Mean: 2.5},
		"Less than a line": {Reading: 2, Min: 0, Max: 1, Mean: 0.2},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := stubLogSource(c.Reading)

			var total int

			const ticks = 20_000

			for tick := range int64(ticks) {
				n := s.lineCount(s.streamAt(tick), tick)
				assert.GreaterOrEqual(t, n, c.Min)
				assert.LessOrEqual(t, n, c.Max)

				total += n
			}

			assert.InDelta(t, c.Mean, float64(total)/ticks, 0.02)
		})
	}
}

func Test_logSource_linesAt(t *testing.T) {
	t.Parallel()

	s := stubLogSource(45)

	lines := s.linesAt(_logTick)
	require.Len(t, lines, s.lineCount(s.streamAt(_logTick), _logTick))
	require.NotEmpty(t, lines)

	// a tick reads the same every time.
	assert.Equal(t, lines, s.linesAt(_logTick))

	start := timeAt(_logTick)

	for i, l := range lines {
		assert.False(t, l.Time.Before(start), "line %d before its tick", i)
		assert.True(t, l.Time.Before(start.Add(_tick)), "line %d after its tick", i)

		if i > 0 {
			assert.False(t, l.Time.Before(lines[i-1].Time), "line %d out of order", i)
		}

		assert.Equal(t, processor.LogLevelInfo, l.Level)
		assert.Equal(t, "deploy started by "+l.Fields[_logFieldUser], l.Line)
		assert.Equal(t, map[string]string{"service_name": "deploy-bot", "env": "production"}, l.Labels)
		assert.Equal(t, "attempted", l.Fields["outcome"])
		assert.Equal(t, "info", l.Fields[_logFieldLevel])
		assert.Equal(t, "3f9a1c2", l.Fields["sha"])
		assert.Regexp(t, "^[0-9a-f]{16}$", l.Fields[_logFieldTraceID])
		assert.Contains(t, _logUsers, l.Fields[_logFieldUser])
	}

	assert.Empty(t, stubLogSource(0).linesAt(_logTick))
}

func Test_logSource_labels(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		map[string]string{"service_name": "deploy-bot", "env": "production"},
		stubLogSource(1).labels(),
	)
}

func Test_logSource_baseFields(t *testing.T) {
	t.Parallel()

	fields := stubLogSource(1).baseFields(stubLogSource(1).streamAt(1))

	assert.Equal(t, "attempted", fields["outcome"])
	assert.Equal(t, "info", fields[_logFieldLevel])
	assert.Regexp(t, "^[0-9a-f]{16}$", fields[_logFieldTraceID])
	assert.Contains(t, _logUsers, fields[_logFieldUser])
	assert.Len(t, fields, 4)
}

func Test_logSource_fieldNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"detected_level", "outcome", "sha", "trace_id", "user"},
		stubLogSource(1).fieldNames(),
	)
}

func Test_newLogSources(t *testing.T) {
	t.Parallel()

	sources := newLogSources(newRegistry())
	require.Len(t, sources, 25)

	seeds := map[int64]bool{}
	week := tickAt(time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC))

	for _, s := range sources {
		name := s.service + "/" + s.bucket

		require.NotNil(t, s.value, "%s reads no metric", name)
		assert.False(t, seeds[s.seed], "%s shares its seed", name)

		seeds[s.seed] = true

		// every tick writes the metric's reading over the scale, or one
		// line more.
		for tick := week; tick < week+60; tick++ {
			want := math.Max(s.value(tick), 0) / s.scale
			n := len(s.linesAt(tick))

			assert.GreaterOrEqual(t, float64(n), math.Floor(want), "%s at tick %d", name, tick)
			assert.LessOrEqual(t, float64(n), math.Floor(want)+1, "%s at tick %d", name, tick)
		}

		// over a week the lines add up to the readings over the scale: the
		// rounding draws stay within about four standard deviations.
		var lines, readings float64

		const ticks = 7 * 24 * 60

		for tick := week; tick < week+ticks; tick++ {
			lines += float64(s.lineCount(s.streamAt(tick), tick))
			readings += math.Max(s.value(tick), 0) / s.scale
		}

		assert.Positive(t, lines, "%s writes nothing in a week", name)
		assert.InDelta(t, readings, lines, 2*math.Sqrt(ticks), "%s drifts from its metric", name)
	}
}
