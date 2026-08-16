package timeutil

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Parse(t *testing.T) {
	// error
	tim, err := Parse("F")
	assert.Error(t, err)
	assert.Zero(t, tim)

	// success
	tim = Now()

	tim = tim.Round(time.Second)
	tim1, err := Parse(tim.Format(time.RFC3339))

	assert.NoError(t, err)
	assert.Equal(t, tim, tim1)
}

func Test_Parse_normalisesToUTC(t *testing.T) {
	tim, err := Parse("2026-01-02T10:00:00+02:00")
	require.NoError(t, err)
	assert.Equal(t, time.UTC, tim.Location())
	assert.Equal(t, time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC), tim)
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
