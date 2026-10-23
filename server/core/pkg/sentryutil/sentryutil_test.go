package sentryutil

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Setup(t *testing.T) {
	cc := map[string]struct {
		Config Config
		Check  func(t *testing.T)
		Err    error
	}{
		"Error returned by sentry.Init": {
			Config: Config{
				DSN: "invalid",
			},
			Err: assert.AnError,
		},
		"Successful setup without deduplication": {
			Config: Config{},
		},
		"Successful setup with deduplication": {
			Config: Config{
				ReleaseName:      "test",
				ReleaseCommit:    "commit",
				ReleaseTimestamp: "timestamp",
				Deduplication: DeduplicationConfig{
					Interval: time.Millisecond * 100,
					Capacity: 10,
				},
			},
			Check: func(t *testing.T) {
				t.Helper()

				// We create two empty events, since deduplicator is enabled,
				// only the first one should be sent.
				event1 := sentry.CaptureEvent(sentry.NewEvent())
				sentry.CaptureEvent(sentry.NewEvent())

				require.NotNil(t, event1)
				assert.Equal(t, *event1, sentry.LastEventID())
				time.Sleep(time.Millisecond * 200)

				// After the first event is cleared, a new event should be sent.
				event2 := sentry.CaptureEvent(sentry.NewEvent())

				require.NotNil(t, event2)
				assert.Equal(t, *event2, sentry.LastEventID())
			},
		},
	}

	for cn, c := range cc {
		// sentry.Init binds the global hub, so the cases cannot run in
		// parallel.
		t.Run(cn, func(t *testing.T) {
			// the bubble's clock moves only while every goroutine in it is
			// blocked. So the deduplication interval passes without any
			// real waiting.
			synctest.Test(t, func(t *testing.T) {
				closer, err := Setup(c.Config)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					assert.Nil(t, closer)
					return
				}

				require.NotNil(t, closer)

				if c.Check != nil {
					c.Check(t)
				}

				assert.NotPanics(t, func() {
					closer()
				})

				// the closer only flushes, so the client keeps the
				// goroutines sentry.Init started. The bubble waits for them
				// to exit.
				sentry.CurrentHub().Client().Close()
			})
		})
	}
}
