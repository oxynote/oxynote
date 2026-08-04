// Package wsserver provides server-side websocket handling functionality.
package wsserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jellydator/xync"
	"github.com/oxynote/wetsocks/internal/errutil"
	"github.com/oxynote/wetsocks/wsutil"
	"github.com/rs/xid"
)

const _defaultReadLimit = 1 << 15 // approx 32kb

const _wsCtxID ctxKey = "conn-id"

// Pool is an interface that is used to handle server-side websocket
// connections.
//
//go:generate ../scripts/codegen/mock Pool
type Pool interface {
	http.Handler
	ConnCloser

	// Close should close all the active connections.
	Close()
}

// ConnCloser handles individual connection closing.
type ConnCloser interface {
	// CloseConn should close the selected connections.
	// The func parameter should be called with each connection's
	// context to determine whether that connection should be closed.
	CloseConn(filter func(context.Context) bool)
}

// Server handles server-side websocket connections.
type Server struct {
	log    *slog.Logger
	router *Router
	opts   Options

	mu    sync.RWMutex
	conns map[*conn]struct{}

	supv *xync.Supervisor
}

// Options contains server-side websocket connection handling options.
type Options struct {
	websocket.AcceptOptions

	// ReadLimit specifies the max number of bytes to read for
	// a single message.
	// Default is 32kb.
	ReadLimit int64

	// ResponseWriteTimeout specifies the amount of time the response
	// writing process may take.
	// Default is 5secs.
	ResponseWriteTimeout time.Duration

	// BaseContext returns a new context for each websocket
	// connection.
	// Default is a func that returns context.Background().
	BaseContext func(*http.Request) (context.Context, error)

	// RecoveryFunc is used to handle panics.
	// An empty func is used if it's set to nil.
	RecoveryFunc func(any)
}

// New creates a fresh instance of a websocket server.
// The func parameter is used to build the base context for new websocket
// connections. Either a context or error should be returned. If an
// error is returned, the connection won't be allowed to open.
func New(
	logger *slog.Logger,
	router *Router,
	opts Options,
) *Server {
	if opts.ReadLimit == 0 {
		opts.ReadLimit = _defaultReadLimit
	}

	if opts.RecoveryFunc == nil {
		opts.RecoveryFunc = func(_ any) {}
	}

	if opts.ResponseWriteTimeout == 0 {
		opts.ResponseWriteTimeout = time.Second * 5
	}

	if opts.BaseContext == nil {
		opts.BaseContext = func(_ *http.Request) (context.Context, error) {
			return context.Background(), nil
		}
	}

	return &Server{
		log:    logger,
		router: router,
		opts:   opts,
		conns:  make(map[*conn]struct{}),
		supv:   xync.NewSupervisor(),
	}
}

// Close closes all websocket connections.
func (s *Server) Close() {
	s.CloseConn(func(_ context.Context) bool {
		return true
	})
	s.router.Close()
	s.supv.CloseAndWait()
}

// CloseConn closes the selected connections.
// The func parameter is called with each connection's
// context to determine whether that connection should be closed.
func (s *Server) CloseConn(filter func(context.Context) bool) {
	supv := xync.NewSupervisor()

	s.mu.RLock()

	for c := range s.conns {
		if !filter(c.safeContext()) {
			continue
		}

		supv.Go(func(_ context.Context) {
			s.closeConn(c)
		})
	}

	s.mu.RUnlock()

	supv.Wait()
}

// closeConn closes the websocket connection and removes it from the list.
func (s *Server) closeConn(c *conn) {
	c.conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck,gosec // most errors are usually false positives here

	c.stop()
	s.router.removeConn(c)

	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
}

// ServeHTTP opens a new websocket connection and adds it to the list.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.opts.BaseContext(r)
	if err != nil {
		s.respondError(w, err)
		return
	}

	ctx = context.WithValue(ctx, _wsCtxID, xid.New().String())

	c, err := websocket.Accept(w, r, &s.opts.AcceptOptions)
	if err != nil {
		err = errutil.Wrap(err, http.StatusInternalServerError, "general", "")
		s.respondError(w, err)

		return
	}

	c.SetReadLimit(s.opts.ReadLimit)
	nc := newConn(ctx, s.log, c)

	s.mu.Lock()
	s.conns[nc] = struct{}{}
	s.mu.Unlock()

	s.supv.Go(func(_ context.Context) {
		// startReader uses the connection's context which is
		// cancelled when Close is called
		s.startReader(nc)
	})
	s.supv.Go(func(_ context.Context) {
		// startWriter uses the connection's context which is
		// cancelled when Close is called
		s.startWriter(nc)
	})
}

// respondError sends the provided error to the client as a JSON response.
func (s *Server) respondError(w http.ResponseWriter, err error) {
	respErr := errutil.Detect(err)
	statusCode := errutil.StatusCode(respErr)

	body, merr := json.Marshal(respErr)
	if merr != nil {
		// NOCOV: the detected error structure is always valid.
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body) //nolint:errcheck,gosec // response write errors provide no meaningful info

	if statusCode >= http.StatusInternalServerError {
		s.log.Error("internal server error", "error", err)
	}
}

// startReader handles reading from the provided websocket connection.
func (s *Server) startReader(c *conn) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("cannot continue reading from a websocket connection", "panic", r)
			s.opts.RecoveryFunc(r)
		}
	}()
	defer s.closeConn(c)

	type response struct {
		ID      uint64 `json:"id"`
		Success bool   `json:"success"`
	}

	respond := func(resp response) {
		raw, err := json.Marshal(resp)
		if err != nil {
			// NOCOV: response structure is already
			// valid.
			s.log.Error("cannot encode a websocket response", "error", err)

			return
		}

		timeoutCtx, cancel := context.WithTimeout(c.safeContext(), s.opts.ResponseWriteTimeout)
		defer cancel()

		select {
		case <-timeoutCtx.Done():
		case c.writeCh <- raw:
		}
	}

	for {
		var req struct {
			ID    uint64 `json:"id"`
			Topic string `json:"topic"`
		}

		err := wsjson.Read(c.safeContext(), c.conn, &req)
		if err != nil {
			if wsutil.IsClosure(err) {
				return
			}

			wsutil.LogError(c.log, err, false)

			continue
		}

		resp := response{
			ID: req.ID,
		}

		tpc, err := wsutil.ParseTopic(req.Topic)
		if err != nil {
			respond(resp)
			continue
		}

		// unknown descriptors are reported as unsuccessful
		switch tpc.Descriptor {
		case _descriptorSub:
			resp.Success = s.router.addTopicSub(tpc, c) == nil
		case _descriptorUnsub:
			s.router.removeTopicSub(tpc, c)

			resp.Success = true
		default:
		}

		respond(resp)
	}
}

// startWriter handles writing to the provided websocket connection.
func (s *Server) startWriter(c *conn) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("cannot continue writing to a websocket connection", "panic", r)
			s.opts.RecoveryFunc(r)
		}
	}()
	defer s.closeConn(c)

	for {
		select {
		case <-c.safeContext().Done():
			return
		case data := <-c.writeCh:
			err := c.conn.Write(c.safeContext(), websocket.MessageText, data)
			if err != nil {
				if wsutil.IsClosure(err) {
					return
				}

				// NOCOV: errors unrelated to closure
				// are hard to simulate here
				wsutil.LogError(c.log, err, false)
			}
		}
	}
}

// conn wraps the real websocket connection and adds additional functionality
// that gives better control over the wrapped connection.
type conn struct {
	log  *slog.Logger
	conn *websocket.Conn

	ctxMu sync.RWMutex
	ctx   context.Context
	stop  context.CancelFunc

	writeCh chan []byte
}

// newConn creates a new connection wrapper.
func newConn(
	ctx context.Context,
	logger *slog.Logger,
	c *websocket.Conn,
) *conn {
	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // the cancel function is stored below and called on stop

	return &conn{
		log:     logger,
		conn:    c,
		ctx:     ctx,
		stop:    cancel,
		writeCh: make(chan []byte),
	}
}

// safeContext returns the connection's context in a concurrently safe way.
func (c *conn) safeContext() context.Context {
	c.ctxMu.RLock()
	defer c.ctxMu.RUnlock()

	return c.ctx
}

// publish writes the provided data to the wrapped connection.
func (c *conn) publish(ctx context.Context, data func() ([]byte, error)) {
	bb, err := data()
	if err != nil {
		c.log.Error("cannot encode websocket payload", "error", err)

		return
	}

	select {
	case <-c.safeContext().Done():
	case <-ctx.Done():
	case c.writeCh <- bb:
	}
}

// ctxKey is used to store values in a context.
type ctxKey string
