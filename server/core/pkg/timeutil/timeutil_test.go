package timeutil

import (
	"log/slog"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Parse(t *testing.T) {
	tim := Now().Round(time.Second)

	cc := map[string]struct {
		Inp    string
		Result time.Time
		Err    error
	}{
		"Error returned by time.Parse": {
			Inp: "F",
			Err: assert.AnError,
		},
		"Successful parse": {
			Inp:    tim.Format(time.RFC3339),
			Result: tim,
		},
		"Offset input is normalised to UTC": {
			Inp:    "2026-01-02T10:00:00+02:00",
			Result: time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := Parse(c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Zero(t, res)
				return
			}

			assert.Equal(t, time.UTC, res.Location())
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Now(t *testing.T) {
	n := Now()
	assert.NotZero(t, n)
	assert.Equal(t, time.UTC, n.Location())
}

func Test_NewCron(t *testing.T) {
	cr := NewCron(slog.New(slog.DiscardHandler))
	require.NotNil(t, cr)

	_, err := cr.AddFunc("* * * * * *", func() { panic("crash") })
	require.NoError(t, err)

	go func() {
		time.Sleep(time.Second)
		cr.Stop()
	}()

	assert.NotPanics(t, func() {
		cr.Run()
	})
}
