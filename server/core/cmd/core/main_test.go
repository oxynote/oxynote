package main

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// the Google API transport behind the Gemini client starts an
	// opencensus worker from a package init, so it is already running
	// before any test does.
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_parseOrigins(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Val    string
		Result []string
	}{
		"Unset value leaves origin checking disabled": {
			Val: "",
		},
		"Single origin": {
			Val:    "https://one.test",
			Result: []string{"https://one.test"},
		},
		"Several origins": {
			Val:    "https://one.test,https://two.test",
			Result: []string{"https://one.test", "https://two.test"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, parseOrigins(c.Val))
		})
	}
}

func Test_fail(t *testing.T) {
	cc := map[string]struct {
		CloseErr error
		Contains []string
	}{
		"Successful release": {
			Contains: []string{"cannot create a server"},
		},
		"Error returned by the closers": {
			CloseErr: assert.AnError,
			Contains: []string{"cannot create a server", "cannot release the opened resources"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				buf    = &bytes.Buffer{}
				logged string
			)

			closer := ioutil.CloserFunc(func() error {
				logged = buf.String()
				return c.CloseErr
			})

			fail(
				slog.New(slog.NewTextHandler(buf, nil)),
				[]io.Closer{closer},
				"cannot create a server",
				assert.AnError,
			)

			// the closers include the log flusher, so the failure must
			// already be logged by the time they run.
			assert.Contains(t, logged, "cannot create a server")

			for _, exp := range c.Contains {
				assert.Contains(t, buf.String(), exp)
			}
		})
	}
}

func Test_warnDisabled(t *testing.T) {
	t.Parallel()

	// a configured integration announces nothing.
	var buf bytes.Buffer

	log := slog.New(slog.NewTextHandler(&buf, nil))

	warnDisabled(log, true, "search is disabled")
	assert.Empty(t, buf.String())

	// a disabled one warns.
	warnDisabled(log, false, "search is disabled")
	assert.Contains(t, buf.String(), "search is disabled")
}
