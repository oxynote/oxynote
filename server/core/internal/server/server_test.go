package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/assistant/provider"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// the Google API transport behind the Gemini client, reached through
	// the assistant provider package, starts an opencensus worker from a
	// package init, so it is already running before any test does.
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

// stubListener blocks Accept until it is closed, and its Close returns err.
// It embeds the interface, so any other call is a test bug.
type stubListener struct {
	net.Listener

	closed chan struct{}
	err    error
}

// Accept blocks until the listener is closed.
func (l stubListener) Accept() (net.Conn, error) {
	<-l.closed

	return nil, net.ErrClosed
}

// Close unblocks Accept and returns the stubbed error.
func (l stubListener) Close() error {
	close(l.closed)

	return l.err
}

func Test_Options_validate(t *testing.T) {
	t.Parallel()

	mcpOpts := MCPOptions{
		SessionURL:  "http://auth.test/api/internal/mcp/session",
		ResourceURL: "http://test.com/core/api/mcp",
	}

	cc := map[string]struct {
		Opts Options
		Err  error
	}{
		"Invalid port": {
			Opts: Options{Port: 999},
			Err:  errors.New("invalid port"),
		},
		"Missing MCP session URL": {
			Opts: Options{MCP: MCPOptions{ResourceURL: "http://test.com/core/api/mcp"}},
			Err:  errors.New("missing mcp session url"),
		},
		"Successful validation without MCP": {
			Opts: Options{},
		},
		"Successful validation with an unset port": {
			Opts: Options{Port: 0, MCP: mcpOpts},
		},
		"Successful validation with a port": {
			Opts: Options{Port: 8080, MCP: mcpOpts},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Opts.validate())
		})
	}
}

func Test_NewServer(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	fc := metricutil.NewFactory("test", prometheus.NewRegistry())

	githubMan, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	slackMan, err := slack.NewManager(log, nil, nil, nil, nil, slack.Options{})
	require.NoError(t, err)

	db := &DBMock{}
	storer := &StorerMock{}
	notifier := &NotifierMock{}

	cc := map[string]struct {
		Opts                Options
		WebchangeURL        string
		BindHookChangeCalls int
		BindChangeCalls     int
		Capabilities        Capabilities
		Err                 error
	}{
		"Invalid options": {
			Opts: Options{Port: 999},
			Err:  errors.New("invalid port"),
		},
		"Error returned by mcp.NewHandler": {
			Opts: Options{
				Port: 8080,
				MCP: MCPOptions{
					SessionURL:  "http://auth.test/api/internal/mcp/session",
					ResourceURL: "://bad",
				},
			},
			BindHookChangeCalls: 1,
			BindChangeCalls:     1,
			Err: fmt.Errorf("building mcp handler: %w", fmt.Errorf(
				"deriving resource metadata url: %w",
				&url.Error{Op: "parse", URL: "://bad", Err: errors.New("missing protocol scheme")},
			)),
		},
		"Successful construction": {
			Opts: Options{
				PublicURL: "http://test.com",
				Port:      8080,
				Origins:   []string{"http://localhost:3000"},
				Auth:      AuthOptions{BetterAuthURL: "http://auth.test"},
				MCP: MCPOptions{
					SessionURL:  "http://auth.test/api/internal/mcp/session",
					ResourceURL: "http://test.com/core/api/mcp",
				},
			},
			BindHookChangeCalls: 1,
			BindChangeCalls:     1,
			// nothing above is configured but MCP, so every other
			// capability reports false.
			Capabilities: Capabilities{MCP: true},
		},
		"MCP off without a resource URL": {
			Opts:                Options{Port: 8080},
			BindHookChangeCalls: 1,
			BindChangeCalls:     1,
		},
		"Capabilities reflect the configured services": {
			Opts: Options{
				Port: 8080,
				Assistant: AssistantCapability{
					Status: provider.StatusActive,
					Model:  "claude-opus-5",
				},
				MCP: MCPOptions{
					SessionURL:  "http://auth.test/api/internal/mcp/session",
					ResourceURL: "http://test.com/core/api/mcp",
				},
			},
			WebchangeURL:        "http://changedetection.test",
			BindHookChangeCalls: 1,
			BindChangeCalls:     1,
			Capabilities: Capabilities{
				AIAssistant: AssistantCapability{
					Status: provider.StatusActive,
					Model:  "claude-opus-5",
				},
				ChangeDetection: true,
				MCP:             true,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hookMan := &HookManagerMock{}
			variableMan := &VariableManagerMock{}

			srv, err := NewServer(
				log,
				c.Opts,
				db,
				fc,
				storer,
				// a manager of this subtest's own: NewServer calls
				// SetTreeNotifier, which must not race another parallel
				// case's identical call on a shared manager.
				assistantCore.NewManager(log, nil, &redis.Pool{}, nil, nil, fc, nil, nil, &SearchQueueMock{}, nil, githubMan, nil, nil, nil, "claude", false),
				datasourceCore.NewManager(log, nil),
				nil,
				githubMan,
				slackMan,
				webchange.NewClient(c.WebchangeURL, ""),
				hookMan,
				variableMan,
				nil,
				&SearchQueueMock{},
				notifier,
				nil,
				http.DefaultClient,
			)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, hookMan.BindHookChangeCalls(), c.BindHookChangeCalls)
			assert.Len(t, variableMan.BindChangeCalls(), c.BindChangeCalls)

			if err != nil {
				assert.Nil(t, srv)
				return
			}

			require.NotNil(t, srv)

			t.Cleanup(func() {
				srv.ws.Close()
				assert.NoError(t, srv.Close())
			})

			assert.NotNil(t, srv.log)
			assert.Same(t, fc, srv.fc)
			assert.Same(t, http.DefaultClient, srv.client)
			assert.Equal(t, ":8080", srv.http.Addr)
			assert.NotNil(t, srv.http.Handler)
			assert.NotNil(t, srv.ws)

			assert.NotNil(t, srv.handlers.user)
			assert.NotNil(t, srv.handlers.organization)
			assert.NotNil(t, srv.handlers.document)
			assert.NotNil(t, srv.handlers.comment)
			assert.NotNil(t, srv.handlers.files)
			assert.NotNil(t, srv.handlers.hook)
			assert.NotNil(t, srv.handlers.github)
			assert.NotNil(t, srv.handlers.slack)
			assert.NotNil(t, srv.handlers.notification)
			assert.NotNil(t, srv.handlers.datasource)
			assert.NotNil(t, srv.handlers.email)
			assert.NotNil(t, srv.handlers.ai)
			assert.NotNil(t, srv.handlers.variable)

			if c.Capabilities.MCP {
				assert.NotNil(t, srv.handlers.mcp)
			} else {
				assert.Nil(t, srv.handlers.mcp)
			}

			assert.Equal(t, c.Capabilities, srv.capabilities)
		})
	}
}

func Test_Server_Listen(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Addr     string
		Shutdown bool
	}{
		"Error returned by http.Server.ListenAndServe": {
			// an invalid port makes ListenAndServe fail immediately.
			Addr: ":a",
		},
		"Successful graceful shutdown": {
			Addr:     "127.0.0.1:0",
			Shutdown: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			wr, buf := testutil.NewBuffer()

			s := Server{
				log:  slog.New(slog.NewJSONHandler(wr, nil)),
				http: &http.Server{Addr: c.Addr, ReadHeaderTimeout: time.Minute},
			}

			if c.Shutdown {
				go func() {
					// give ListenAndServe a moment to start before shutting down.
					time.Sleep(50 * time.Millisecond)

					s.http.Shutdown(context.Background()) //nolint:errcheck,gosec // error provides no meaningful info
				}()
			}

			s.Listen()

			require.NoError(t, wr.Flush())
			assert.Contains(t, buf.String(), `"msg":"listening"`)

			// a graceful shutdown ends Listen without the error log.
			if c.Shutdown {
				assert.NotContains(t, buf.String(), "cannot terminate gracefully")
				return
			}

			assert.Contains(t, buf.String(), "cannot terminate gracefully")
		})
	}
}

func Test_Server_Close(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		ListenerErr error
		Err         error
	}{
		"Error returned by http.Server.Shutdown": {
			// Shutdown returns the error of closing a listener.
			ListenerErr: assert.AnError,
			Err:         assert.AnError,
		},
		"Successful close": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			serving := make(chan struct{})

			s := Server{
				log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
				http: &http.Server{
					ReadHeaderTimeout: time.Minute,
					// Serve calls this only after it tracks the listener, so
					// Shutdown is sure to close it.
					BaseContext: func(net.Listener) context.Context {
						close(serving)

						return context.Background()
					},
				},
			}

			served := make(chan struct{})

			go func() {
				defer close(served)

				testutil.AssertEqualError(t, http.ErrServerClosed, s.http.Serve(stubListener{
					closed: make(chan struct{}),
					err:    c.ListenerErr,
				}))
			}()

			<-serving

			err := s.Close()
			testutil.AssertEqualError(t, c.Err, err)

			<-served

			if err != nil {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), "msg=closed")
		})
	}
}

func Test_Server_fetchVersion(t *testing.T) {
	t.Parallel()

	s := Server{
		log: slog.New(slog.DiscardHandler),
	}

	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()

	s.fetchVersion(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"version":"`+buildinfo.Full().Version.String()+`"}`, rec.Body.String())
}

func Test_Server_bindVersionPing(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Events lists the lifecycle callbacks in delivery order: "sub"
		// for the first subscribe, "unsub" for the last unsubscribe.
		Events    []string
		Published bool
	}{
		"Successful version ping": {
			// the first subscriber starts the 5-second cron.
			Events:    []string{"sub"},
			Published: true,
		},
		"Unsubscribe before the first subscribe": {
			// the lifecycle callbacks are dispatched as independent
			// goroutines, so an unsubscribe can land before the first
			// subscribe ever ran. The overtaken subscribe pairing with it
			// must not start the cron. A cron erroneously left running
			// here is caught by the package's goroutine-leak check.
			Events: []string{"unsub", "sub"},
		},
		"Stale unsubscribe keeps the newer subscription cron": {
			// emitted sub/unsub/sub, delivered sub/sub/unsub: the stale
			// unsubscribe pairs with the first subscribe and must leave
			// the second subscribe's cron running.
			Events:    []string{"sub", "sub", "unsub"},
			Published: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := Server{
				log: slog.New(slog.DiscardHandler),
			}

			published := make(chan struct{}, 16)
			tpc := &wsMock.Topic{
				PublishManyFunc: func(context.Context, any, func(context.Context, string) bool) {
					published <- struct{}{}
				},
			}

			s.bindVersionPing(tpc)

			require.Len(t, tpc.OnFirstSubCalls(), 1)
			require.Len(t, tpc.OnLastUnsubCalls(), 1)
			assert.Empty(t, tpc.PublishManyCalls())

			first := tpc.OnFirstSubCalls()[0].Fn
			last := tpc.OnLastUnsubCalls()[0].Fn

			for _, evt := range c.Events {
				assert.NotPanics(t, func() {
					if evt == "sub" {
						first(context.Background())
						return
					}

					last(context.Background())
				})
			}

			if !c.Published {
				assert.Empty(t, tpc.PublishManyCalls())
				return
			}

			// drain ticks that may have fired before the last event
			// landed. A fresh one can only arrive if the cron survived it.
			for len(published) > 0 {
				<-published
			}

			select {
			case <-published:
			case <-time.After(7 * time.Second):
				t.Fatal("version ping was not published")
			}

			// the balancing unsubscribe stops the cron.
			last(context.Background())

			ff := tpc.PublishManyCalls()
			require.NotEmpty(t, ff)
			assert.Equal(t, struct {
				Version semver.Version `json:"version"`
			}{
				Version: buildinfo.Full().Version,
			}, ff[0].Payload)
		})
	}
}
