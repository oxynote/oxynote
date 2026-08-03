// Package connector implements a server that handles remote connections to the system.
package connector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/oxynote/heimdall/internal/connector/datasourcehandler"
	"github.com/oxynote/purse/util/logutil"
	"github.com/oxynote/purse/util/metricutil"
)

// Options holds all server options.
type Options struct {
	// Port specifies which port should be used by the server.
	Port uint

	// Secure enables additional security-related features.
	// Note that most of the security settings should be handled
	// by a reverse proxy, this option just updates a few internal
	// processes all of which expect a reverse proxy to be working as
	// well.
	Secure bool
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
	log  *slog.Logger
	http *http.Server
	fc   metricutil.Factory

	handlers struct {
		datasource *datasourcehandler.Handler
	}

	opts Options
}

// NewServer creates a fresh instance of a server.
// If central auth is nil, the default users are enabled.
func NewServer(
	ctx context.Context,
	log *slog.Logger,
	opts Options,
	fc metricutil.Factory,
) (*Server, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	srv := &Server{
		log:  log.With("component", "server"),
		opts: opts,
		fc:   fc,
	}

	srv.handlers.datasource = datasourcehandler.NewHandler(log)

	srv.http = &http.Server{
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: time.Minute,
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           srv.router(),
		ErrorLog:          logutil.NewDebugWriter(log, "EOF").StdLogger(),
	}

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
