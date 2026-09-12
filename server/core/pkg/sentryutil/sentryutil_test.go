package sentryutil

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Setup(t *testing.T) {
	// Sentry init returns an error.
	closer, err := Setup(Config{
		DSN: "invalid",
	})
	assert.Error(t, err)
	assert.Nil(t, closer)

	// Success without deduplication.
	closer, err = Setup(Config{})
	require.NoError(t, err)
	require.NotNil(t, closer)
	assert.NotPanics(t, func() {
		closer()
	})

	// Success with deduplication.
	closer, err = Setup(Config{
		ReleaseName:      "test",
		ReleaseCommit:    "commit",
		ReleaseTimestamp: "timestamp",
		Deduplication: DeduplicationConfig{
			Interval: time.Millisecond * 100,
			Capacity: 10,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, closer)

	// We create two empty events, since deduplicator is enabled, only
	// the first one should be sent.
	event1 := sentry.CaptureEvent(sentry.NewEvent())
	sentry.CaptureEvent(sentry.NewEvent())

	assert.Equal(t, *event1, sentry.LastEventID())
	time.Sleep(time.Millisecond * 200)

	// After the first event is cleared, a new event should be sent.
	event2 := sentry.CaptureEvent(sentry.NewEvent())

	assert.Equal(t, *event2, sentry.LastEventID())
	assert.NotPanics(t, func() {
		closer()
	})
}
