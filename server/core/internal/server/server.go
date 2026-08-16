// Package server provides a simple HTTP server for Oxynote.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/coder/websocket"
	githubCore "github.com/oxynote/oxynote/server/core/internal/apps/github"
	slackCore "github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	notificationCore "github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/document/files"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/email"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/github"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/slack"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/user"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/oxynote/wetsocks/wsserver"
)

// Options holds all server options.
type Options struct {
	// PublicURL is the public URL of the server.
	PublicURL string

	// DemoPrometheusURL is the URL of the demo Prometheus instance.
	DemoPrometheusURL string

	// Port specifies which port should be used by the server.
	Port uint

	// Origins specifies the allowed origins for CORS requests.
	Origins []string

	// Secure enables additional security-related features.
	// Note that most of the security settings should be handled
	// by a reverse proxy, this option just updates a few internal
	// processes all of which expect a reverse proxy to be working as
	// well.
	Secure bool

	// Auth contains options for authentication.
	Auth auth.Options
}

// AuthOptions aliases the internal auth package's options so that cmd
// wiring can populate them without importing an internal package.
type AuthOptions = auth.Options

// validate validates remote server options.
func (o Options) validate() error {
	if o.Port != 0 && o.Port < 1024 {
		return errors.New("invalid port")
	}

	return nil
}

// Server handles remote connections to the system.
type Server struct {
	log    *slog.Logger
	http   *http.Server
	fc     metricutil.Factory
	client *http.Client

	ws wsserver.Pool

	handlers struct {
		user         *user.Handler
		organization *org.Handler
		document     *document.Handler
		comment      *comment.Handler
		files        *files.Handler
		hook         *hook.Handler
		github       *github.Handler
		slack        *slack.Handler
		notification *notification.Handler
		datasource   *datasource.Handler
		email        *email.Handler
		ai           *assistant.Handler
	}

	opts Options
}

// NewServer creates a fresh instance of a server.
// If central auth is nil, the default users are enabled.
func NewServer(
	log *slog.Logger,
	opts Options,
	db DB,
	fc metricutil.Factory,
	storageClient Storer,
	assistantMan *assistantCore.Manager,
	githubMan *githubCore.Manager,
	slackMan *slackCore.Manager,
	webchangeClient *webchange.Client,
	searchGateway document.SearchGateway,
	notifier Notifier,
	emailSender email.Sender,
	client *http.Client,
) (*Server, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	srv := &Server{
		log:    log.With("component", "server"),
		opts:   opts,
		fc:     fc,
		client: client,
	}

	srv.handlers.user = user.NewHandler(log, db, storageClient, opts.PublicURL+_userImageLocationFormat)
	srv.handlers.organization = org.NewHandler(log, db, storageClient, opts.PublicURL+_organizationLogoLocation, opts.DemoPrometheusURL)
	srv.handlers.document = document.NewHandler(log, db, githubMan, webchangeClient, searchGateway, notifier, storageClient)
	srv.handlers.comment = comment.NewHandler(log, db, notifier)
	srv.handlers.files = files.NewHandler(log, db, storageClient, opts.PublicURL+documentCore.FilePathFormat)
	srv.handlers.hook = hook.NewHandler(log, db, githubMan, webchangeClient)

	// The assistant's CRUD tools mutate the document tree directly
	// via the DB layer, bypassing the HTTP handlers that normally
	// fire tree-change events. Hand it the document handler so its
	// tools can notify sidebar subscribers after create/delete/move/
	// rename/set-icon.
	assistantMan.SetTreeNotifier(srv.handlers.document)
	srv.handlers.github = github.NewHandler(log, db, client, githubMan)
	srv.handlers.slack = slack.NewHandler(log, db, http.DefaultClient, slackMan)
	srv.handlers.notification = notification.NewHandler(log, db, notifier)
	srv.handlers.datasource = datasource.NewHandler(log, db)
	srv.handlers.email = email.NewHandler(log, emailSender)

	wsAcceptOpts := websocket.AcceptOptions{
		OriginPatterns:     opts.Origins,
		InsecureSkipVerify: !opts.Secure,
	}
	srv.handlers.ai = assistant.NewHandler(
		log,
		assistantMan,
		wsAcceptOpts,
	)

	srv.http = &http.Server{
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: time.Minute,
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           srv.httpRouter(),
		ErrorLog:          logutil.NewDebugWriter(log, "EOF").StdLogger(),
	}

	srv.ws = wsserver.New(log, srv.wsRouter(), wsserver.Options{
		AcceptOptions: wsAcceptOpts,
		BaseContext: func(r *http.Request) (context.Context, error) {
			session, err := auth.ExtractSessionFromContext(r.Context())
			if err != nil {
				return nil, err
			}

			return auth.AddSessionToContext(context.Background(), session), nil
		},
	})

	return srv, nil
}

// Listen starts listening for incoming connections.
func (s *Server) Listen() {
	s.log.Info("listening", slog.String("address", s.Address()))

	err := s.http.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		s.log.Error("cannot terminate gracefully", slog.String("error", err.Error()))
	}
}

// Address returns the address of the server.
func (s *Server) Address() string {
	host := "localhost"
	if s.http.Addr != ":80" {
		host += s.http.Addr // port can be random
	}

	addr := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	return addr.String()
}

// Close performs clean up operations.
func (s *Server) Close() error {
	if err := s.http.Shutdown(context.Background()); err != nil {
		return err
	}

	s.log.Debug("closed")

	return nil
}

// fetchVersion retrieves the exec's version.
func (s *Server) fetchVersion(w http.ResponseWriter, _ *http.Request) {
	httpserver.Respond(s.log, w, struct {
		Version semver.Version `json:"version"`
	}{
		Version: buildinfo.Full().Version,
	}, http.StatusOK)
}

// bindVersionPing publishes version information every 5th second.
func (s *Server) bindVersionPing(tpc wsserver.Topic) {
	// the subscribe and unsubscribe callbacks are dispatched as independent
	// goroutines, so they can overlap and even arrive out of order; the
	// mutex guards the shared context and the nil check covers an
	// unsubscribe that lands before the first subscribe.
	var (
		mu     sync.Mutex
		ctx    context.Context
		cancel context.CancelFunc
		cr     = timeutil.NewCron(s.log)
	)

	_, err := cr.AddFunc("*/5 * * * * *", func() {
		mu.Lock()
		pubCtx := ctx
		mu.Unlock()

		if pubCtx == nil {
			return
		}

		tpc.PublishMany(pubCtx, struct {
			Version semver.Version `json:"version"`
		}{
			Version: buildinfo.Full().Version,
		}, nil)
	})
	if err != nil {
		// NOCOV: an error is returned only if the spec, which
		// in this case is constant, is invalid.
		s.log.Error("cannot set up cron", "error", err.Error())
		return
	}

	tpc.OnFirstSub(func(_ context.Context) {
		mu.Lock()
		defer mu.Unlock()

		// this context deliberately outlives the closure; OnLastUnsub
		// cancels it.
		ctx, cancel = context.WithCancel(context.Background()) //nolint:gosec,fatcontext // cancel is invoked by the OnLastUnsub callback

		cr.Start()
	})

	tpc.OnLastUnsub(func(_ context.Context) {
		mu.Lock()
		defer mu.Unlock()

		if cancel != nil {
			cancel()

			ctx, cancel = nil, nil //nolint:fatcontext // cleared so the next subscription starts a fresh context
		}

		cr.Stop()
	})
}

// DB is an interface that handles communication with the database.
//
//go:generate ../../scripts/codegen/mock -t internal DB db
type DB interface {
	document.DB
	comment.DB
	files.DB
	hook.DB
	user.DB
	org.DB
	github.DB
	slack.DB
	notification.DB
	datasource.DB
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../scripts/codegen/mock -t internal Storer
type Storer interface {
	org.Storer
	user.Storer
	files.Storer
	document.Storer
}

// Notifier is an interface that combines notificationCore.Notifier and
// notificationCore.Publisher.
//
//go:generate ../../scripts/codegen/mock -t internal Notifier
type Notifier interface {
	notificationCore.Receiver
	notificationCore.Publisher
}
