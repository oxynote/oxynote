package main

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

func Test_parseServerAddress(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Val  string
		Host string
		Port uint
		Err  error
	}{
		"Unset value binds all interfaces on the default port": {
			Port: 8080,
		},
		"Loopback address": {
			Val:  "127.0.0.1:8180",
			Host: "127.0.0.1",
			Port: 8180,
		},
		"Port without a host": {
			Val:  ":9090",
			Port: 9090,
		},
		"Missing port": {
			Val: "127.0.0.1",
			Err: assert.AnError,
		},
		"Non-numeric port": {
			Val: "127.0.0.1:http",
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			host, port, err := parseServerAddress(c.Val)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Host, host)
			assert.Equal(t, c.Port, port)
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
