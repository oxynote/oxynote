package sqlutil

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	metricMock "github.com/oxynote/oxynote/server/core/pkg/metricutil/_mock"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewHooks(t *testing.T) {
	h := NewHooks(
		metricutil.NewFactory("test", nil),
		WithHookDatabaseName("db"),
		WithHookDurationThreshold(time.Second),
		WithHookErrorHandler(func(_ error) error {
			return assert.AnError
		}),
	)
	assert.NotNil(t, h.queriesCounter)
	assert.NotNil(t, h.queriesDuration)
	assert.Equal(t, time.Second, h.durationThreshold)
	assert.NotNil(t, h.errorHandler)
	assert.Equal(t, "db", h.dbName)
}

func Test_Hooks_Before(t *testing.T) {
	cnt := &metricMock.Counter{}
	cv := &metricMock.CounterVec{
		WithFunc: func(_ prometheus.Labels) metricutil.Counter {
			return cnt
		},
	}

	h := Hooks{
		queriesCounter: cv,
	}

	ctx := context.Background()

	nctx, err := h.Before(ctx, "query")
	require.NoError(t, err)

	tstamp, ok := nctx.Value(timeKey).(time.Time)
	require.True(t, ok)

	assert.WithinDuration(t, timeutil.Now(), tstamp, time.Second*5)
	assert.Len(t, cv.WithCalls(), 1)
	assert.Len(t, cnt.IncCalls(), 1)
}

func Test_Hooks_OnError(t *testing.T) {
	cc := map[string]struct {
		ErrorHandler func(error) error
		Inp          error
		Err          error
	}{
		"Without error handler": {
			Inp: sql.ErrNoRows,
			Err: sql.ErrNoRows,
		},
		"With error handler": {
			ErrorHandler: func(err error) error {
				return fmt.Errorf("handled: %w", err)
			},
			Inp: sql.ErrNoRows,
			Err: fmt.Errorf("handled: %w", sql.ErrNoRows),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := Hooks{
				errorHandler: c.ErrorHandler,
			}

			testutil.AssertEqualError(t, c.Err, h.OnError(context.Background(), c.Inp, "query"))
		})
	}
}

func Test_Hooks_After(t *testing.T) {
	cc := map[string]struct {
		Context context.Context
		Calls   int
	}{
		"Context without start time": {
			Context: context.Background(),
		},
		"Duration below threshold": {
			Context: context.WithValue(context.Background(), timeKey, timeutil.Now()),
		},
		"Duration above threshold": {
			Context: context.WithValue(context.Background(), timeKey, timeutil.Now().Add(-time.Hour*2)),
			Calls:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			obs := &metricMock.Observer{}
			hv := &metricMock.HistogramVec{
				WithFunc: func(_ prometheus.Labels) metricutil.Observer {
					return obs
				},
			}

			h := Hooks{
				queriesDuration:   hv,
				durationThreshold: time.Hour,
			}

			_, err := h.After(c.Context, "query")
			require.NoError(t, err)

			assert.Len(t, hv.WithCalls(), c.Calls)
			assert.Len(t, obs.ObserveCalls(), c.Calls)
		})
	}
}

func Test_normalizeQuery(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Query    string
		Expected string
	}{
		"Constant query is unchanged": {
			Query:    "SELECT id FROM items WHERE id = $1",
			Expected: "SELECT id FROM items WHERE id = $1",
		},
		// each slice length would otherwise mint its own label value, and
		// with it its own histogram series.
		"Placeholder list collapses": {
			Query:    "SELECT id FROM items WHERE id IN ($1,$2,$3)",
			Expected: "SELECT id FROM items WHERE id IN ($?)",
		},
		"Spaced placeholder list collapses": {
			Query:    "SELECT id FROM items WHERE id IN ($1, $2)",
			Expected: "SELECT id FROM items WHERE id IN ($?)",
		},
		"Lists collapse independently of the rest": {
			Query:    "SELECT id FROM items WHERE org = $1 AND id IN ($2, $3, $4)",
			Expected: "SELECT id FROM items WHERE org = $1 AND id IN ($?)",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, normalizeQuery(c.Query))
		})
	}
}
