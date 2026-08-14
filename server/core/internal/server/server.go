// Package server provides a simple HTTP server for Oxynote.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/blang/semver/v4"
	"github.com/coder/websocket"
	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/slackapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchanges"
	"github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/aihandler"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/internal/server/datasourcehandler"
	"github.com/oxynote/oxynote/server/core/internal/server/dochandler"
	"github.com/oxynote/oxynote/server/core/internal/server/emailhandler"
	"github.com/oxynote/oxynote/server/core/internal/server/githubhandler"
	"github.com/oxynote/oxynote/server/core/internal/server/notifhandler"
	"github.com/oxynote/oxynote/server/core/internal/server/orghandler"
	"github.com/oxynote/oxynote/server/core/internal/server/slackhandler"
	"github.com/oxynote/oxynote/server/core/internal/server/userhandler"
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
		user         *userhandler.Handler
		organization *orghandler.Handler
		document     *dochandler.Handler
		github       *githubhandler.Handler
		slack        *slackhandler.Handler
		notification *notifhandler.Handler
		datasource   *datasourcehandler.Handler
		email        *emailhandler.Handler
		ai           *aihandler.Handler
	}

	opts Options
}

// NewServer creates a fresh instance of a server.
// If central auth is nil, the default users are enabled.
func NewServer(
	ctx context.Context,
	log *slog.Logger,
	opts Options,
	db DB,
	fc metricutil.Factory,
	storageClient Storer,
	assistantMan *assistant.Manager,
	githubMan *githubapp.Manager,
	slackMan *slackapp.Manager,
	webchangesClient *webchanges.Client,
	searchGateway dochandler.SearchGateway,
	notifier Notifier,
	emailSender emailhandler.Sender,
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

	srv.handlers.user = userhandler.NewHandler(log, db, storageClient, opts.PublicURL+_userImageLocationFormat)
	srv.handlers.organization = orghandler.NewHandler(log, db, storageClient, opts.PublicURL+_organizationLogoLocation, opts.DemoPrometheusURL)
	srv.handlers.document = dochandler.NewHandler(log, db, storageClient, opts.PublicURL+_documentFileLocationFormat, githubMan, webchangesClient, searchGateway, notifier)

	// The assistant's CRUD tools mutate the document tree directly
	// via the DB layer, bypassing the HTTP handlers that normally
	// fire tree-change events. Hand it the dochandler so its tools
	// can notify sidebar subscribers after create/delete/move/
	// rename/set-icon.
	assistantMan.SetTreeNotifier(srv.handlers.document)
	srv.handlers.github = githubhandler.NewHandler(log, db, client, githubMan)
	srv.handlers.slack = slackhandler.NewHandler(log, db, http.DefaultClient, slackMan)
	srv.handlers.notification = notifhandler.NewHandler(log, db, notifier)
	srv.handlers.datasource = datasourcehandler.NewHandler(log, db)
	srv.handlers.email = emailhandler.NewHandler(log, emailSender)

	wsAcceptOpts := websocket.AcceptOptions{
		OriginPatterns:     opts.Origins,
		InsecureSkipVerify: !opts.Secure,
	}
	srv.handlers.ai = aihandler.NewHandler(
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
	var (
		ctx    context.Context
		cancel context.CancelFunc
		cr     = timeutil.NewCron(s.log)
	)

	_, err := cr.AddFunc("*/5 * * * * *", func() {
		tpc.PublishMany(ctx, struct {
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
		// this context should be able to outlive this goroutine
		ctx, cancel = context.WithCancel(context.Background())

		cr.Start()
	})

	tpc.OnLastUnsub(func(_ context.Context) {
		cancel()
		cr.Stop()
	})
}

// DB is an interface that handles communication with the database.
type DB interface {
	dochandler.DB
	userhandler.DB
	orghandler.DB
	githubhandler.DB
	slackhandler.DB
	notification.DB
	datasourcehandler.DB
}

// Storer is an interface that defines methods for uploading and retrieving objects.
type Storer interface {
	orghandler.Storer
	userhandler.Storer
	dochandler.Storer
}

// Notifier is an interface that combines notification.Notifier and
// notification.NotificationPublisher.
type Notifier interface {
	notification.NotificationReceiver
	notification.NotificationPublisher
}
