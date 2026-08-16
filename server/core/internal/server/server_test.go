package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// discardLog returns a logger that swallows all output.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// bufferLog returns a logger writing JSON lines into the returned buffer.
func bufferLog() (*slog.Logger, *testutil.Writer, func() string) {
	wr, buf := testutil.NewBuffer()

	return slog.New(slog.NewJSONHandler(wr, nil)), wr, buf.String
}

func Test_Options_validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, Options{Port: 0}.validate())
	assert.NoError(t, Options{Port: 8080}.validate())
	assert.EqualError(t, Options{Port: 999}.validate(), "invalid port")
}

func Test_NewServer(t *testing.T) {
	t.Parallel()

	log := discardLog()
	fc := metricutil.NewFactory("test", prometheus.NewRegistry())

	assistantMan := assistantCore.NewManager(log, nil, &redis.Pool{}, "key", fc, nil, nil)

	githubMan, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	slackMan, err := slack.NewManager(log, nil, nil, nil, slack.Options{})
	require.NoError(t, err)

	db := &DBMock{}
	storer := &StorerMock{}
	notifier := &NotifierMock{}

	t.Run("Invalid options", func(t *testing.T) {
		t.Parallel()

		srv, err := NewServer(
			log,
			Options{Port: 999},
			db,
			fc,
			storer,
			assistantMan,
			githubMan,
			slackMan,
			nil,
			nil,
			notifier,
			nil,
			http.DefaultClient,
		)
		require.EqualError(t, err, "invalid port")
		assert.Nil(t, srv)
	})

	t.Run("Successful construction", func(t *testing.T) {
		t.Parallel()

		srv, err := NewServer(
			log,
			Options{
				PublicURL:         "http://test.com",
				DemoPrometheusURL: "http://prom.test",
				Port:              8080,
				Origins:           []string{"http://localhost:3000"},
				Auth:              AuthOptions{BetterAuthURL: "http://auth.test"},
			},
			db,
			fc,
			storer,
			assistantMan,
			githubMan,
			slackMan,
			nil,
			nil,
			notifier,
			nil,
			http.DefaultClient,
		)
		require.NoError(t, err)
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
	})
}

func Test_Server_Listen(t *testing.T) {
	t.Parallel()

	log, wr, content := bufferLog()

	s := Server{
		log:  log,
		http: &http.Server{Addr: ":a", ReadHeaderTimeout: time.Minute},
	}

	// an invalid port makes ListenAndServe fail immediately.
	s.Listen()

	require.NoError(t, wr.Flush())
	assert.Contains(t, content(), `"msg":"listening"`)
	assert.Contains(t, content(), "cannot terminate gracefully")

	// a graceful shutdown ends Listen without the error log.
	log2, wr2, content2 := bufferLog()
	s2 := Server{
		log:  log2,
		http: &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Minute},
	}

	go func() {
		// give ListenAndServe a moment to start before shutting down.
		time.Sleep(50 * time.Millisecond)

		s2.http.Shutdown(context.Background()) //nolint:errcheck,gosec // error provides no meaningful info
	}()

	s2.Listen()

	require.NoError(t, wr2.Flush())
	assert.Contains(t, content2(), `"msg":"listening"`)
	assert.NotContains(t, content2(), "cannot terminate gracefully")
}

func Test_Server_Address(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Addr   string
		Result string
	}{
		"Default HTTP port is omitted": {
			Addr:   ":80",
			Result: "http://localhost",
		},
		"Custom port is included": {
			Addr:   ":1234",
			Result: "http://localhost:1234",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := Server{
				http: &http.Server{Addr: c.Addr},
			}

			assert.Equal(t, c.Result, s.Address())
		})
	}
}

func Test_Server_Close(t *testing.T) {
	t.Parallel()

	s := Server{
		log:  discardLog(),
		http: &http.Server{},
	}

	assert.NoError(t, s.Close())
}

func Test_Server_fetchVersion(t *testing.T) {
	t.Parallel()

	s := Server{
		log: discardLog(),
	}

	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()

	s.fetchVersion(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"version":"`+buildinfo.Full().Version.String()+`"}`, rec.Body.String())
}

func Test_Server_bindVersionPing(t *testing.T) {
	t.Parallel()

	s := Server{
		log: discardLog(),
	}

	tpc := &wsMock.Topic{}

	published := make(chan struct{})
	tpc.PublishManyFunc = func(ctx context.Context, _ any, _ func(context.Context, string) bool) {
		// stop the cron from the first tick so the test does not
		// depend on additional firings.
		tpc.OnLastUnsubCalls()[0].Fn(ctx)
		close(published)
	}

	s.bindVersionPing(tpc)

	require.Len(t, tpc.OnFirstSubCalls(), 1)
	require.Len(t, tpc.OnLastUnsubCalls(), 1)
	assert.Empty(t, tpc.PublishManyCalls())

	// the first subscriber starts the 5-second cron.
	tpc.OnFirstSubCalls()[0].Fn(context.Background())

	select {
	case <-published:
	case <-time.After(7 * time.Second):
		t.Fatal("version ping was not published")
	}

	ff := tpc.PublishManyCalls()
	require.NotEmpty(t, ff)
	assert.Equal(t, struct {
		Version semver.Version `json:"version"`
	}{
		Version: buildinfo.Full().Version,
	}, ff[0].Payload)
}
