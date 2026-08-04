package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/oxynote/purse/util/logutil"
	"github.com/oxynote/purse/util/metricutil"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server exposes metrics over HTTP.
type Server struct {
	log  *slog.Logger
	http *http.Server
	fc   metricutil.Factory
}

// New creates a fresh instance of server.
func New(
	log *slog.Logger,
	fc metricutil.Factory,
) *Server {
	srv := &Server{
		log: log,
		fc:  fc,
	}

	srv.http = &http.Server{
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: time.Minute,
		Addr:              ":8080",
		Handler:           srv.router(),
		ErrorLog:          logutil.NewDebugWriter(log).StdLogger(),
	}

	return srv
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

// router prepares routes.
func (s *Server) router() chi.Router {
	r := chi.NewRouter()

	r.Use(httpserver.Timeout(30 * time.Second))
	r.Use(httpserver.Recoverer(s.log))

	r.Handle("/api/metrics", promhttp.InstrumentMetricHandler(
		s.fc, promhttp.HandlerFor(s.fc, promhttp.HandlerOpts{}),
	))

	r.MethodNotAllowed(httpserver.MethodNotAllowed(s.log))
	r.NotFound(httpserver.NotFound(s.log))

	return r
}
