package sqlutil

import (
	"context"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	metricMock "github.com/oxynote/oxynote/server/core/pkg/metricutil/_mock"
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
	cv := &metricMock.CounterVec{
		WithFunc: func(_ prometheus.Labels) metricutil.Counter {
			return &metricMock.Counter{}
		},
	}

	h := Hooks{
		queriesCounter: cv,
	}

	ctx := context.Background()

	nctx, err := h.Before(ctx, "query")
	require.Nil(t, err)

	tstamp, ok := nctx.Value(timeKey).(time.Time)
	require.True(t, ok)

	assert.WithinDuration(t, timeutil.Now(), tstamp, time.Second*5)
	assert.Len(t, cv.WithCalls(), 1)
}

func Test_Hooks_OnError(t *testing.T) {
	h := Hooks{}

	assert.Equal(t, nil, h.OnError(context.Background(), nil, "query"))

	h.errorHandler = func(_ error) error {
		return assert.AnError
	}

	assert.Equal(t, assert.AnError, h.OnError(context.Background(), nil, "query"))
}

func Test_Hooks_After(t *testing.T) {
	hv := &metricMock.HistogramVec{
		WithFunc: func(_ prometheus.Labels) metricutil.Observer {
			return &metricMock.Observer{}
		},
	}

	h := Hooks{
		queriesDuration:   hv,
		durationThreshold: time.Hour,
	}

	ctx := context.WithValue(context.Background(), timeKey, timeutil.Now())

	_, err := h.After(ctx, "query")
	require.Nil(t, err)

	assert.Len(t, hv.WithCalls(), 0)

	ctx = context.WithValue(context.Background(), timeKey, timeutil.Now().Add(-time.Hour*2))

	_, err = h.After(ctx, "query")
	require.Nil(t, err)

	assert.Len(t, hv.WithCalls(), 1)
}
