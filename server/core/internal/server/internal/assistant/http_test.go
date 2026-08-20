package assistant

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// chatServer serves HandleChat with a test session on every request and
// returns the server ready for WebSocket dials.
func chatServer(t *testing.T, hdl *Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdl.HandleChat(w, r.WithContext(auth.AddSessionToContext(r.Context(), auth.Session{
			UserID:               "u1",
			ActiveOrganizationID: "org1",
		})))
	}))

	t.Cleanup(srv.Close)

	return srv
}

// dial opens a WebSocket connection against the chat server.
func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, srv.URL, &websocket.DialOptions{
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	if resp != nil && resp.Body != nil {
		resp.Body.Close() //nolint:errcheck,gosec // error provides no meaningful info
	}

	return conn
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), man, websocket.AcceptOptions{InsecureSkipVerify: true})
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.True(t, hdl.acceptOpts.InsecureSkipVerify)
	assert.Same(t, man, hdl.assistant)
}

func Test_Handler_HandleChat(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Man *ManagerMock

		// Client drives the socket side of a case that completes the
		// websocket handshake; Resp inspects the response of one that
		// never gets that far. Exactly one of them is set.
		Client func(t *testing.T, conn *websocket.Conn)
		Resp   func(t *testing.T, rec *httptest.ResponseRecorder)

		NoSession bool
		Chats     int
	}

	cc := map[string]tcase{
		"Unauthenticated request": {
			Man:       &ManagerMock{},
			NoSession: true,
			Resp: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnauthorized, rec.Code)
				assert.JSONEq(t, `{"code":"account.not_authenticated","message":"not authenticated"}`, rec.Body.String())
			},
		},
		"Websocket upgrade failure": {
			Man: &ManagerMock{},
			Resp: func(t *testing.T, rec *httptest.ResponseRecorder) {
				// a plain GET without upgrade headers cannot be accepted.
				assert.NotEqual(t, http.StatusSwitchingProtocols, rec.Code)
			},
		},
		"Chat failure closes the socket": {
			Man: &ManagerMock{
				ChatFunc: func(context.Context, string, string, protocol.SessionConn) error {
					return errors.New("boom")
				},
			},
			Chats: 1,
			Client: func(t *testing.T, conn *websocket.Conn) {
				defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck // already closed by the server

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// a failed chat closes the socket with an internal error status.
				_, _, err := conn.Read(ctx)
				require.Error(t, err)
				assert.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err))
			},
		},
		// the chat loop reads a message, echoes a reply, and sees the clean
		// client close as io.EOF through the adapter.
		"Conversation round trip": func() tcase {
			readErrs := make(chan error, 1)

			return tcase{
				Man: &ManagerMock{
					ChatFunc: func(ctx context.Context, _, _ string, conn protocol.SessionConn) error {
						msg, err := conn.Read(ctx)
						if err != nil {
							return err
						}

						conn.WriteJSON(ctx, map[string]string{"echo": string(msg)})

						_, err = conn.Read(ctx)
						readErrs <- err

						if errors.Is(err, io.EOF) {
							return nil
						}

						return err
					},
				},
				Chats: 1,
				Client: func(t *testing.T, conn *websocket.Conn) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"hello"}`)))

					var reply map[string]string

					require.NoError(t, wsjson.Read(ctx, conn, &reply))
					assert.JSONEq(t, `{"type":"hello"}`, reply["echo"])

					require.NoError(t, conn.Close(websocket.StatusNormalClosure, "bye"))

					select {
					case err := <-readErrs:
						assert.ErrorIs(t, err, io.EOF)
					case <-time.After(5 * time.Second):
						t.Fatal("chat loop did not observe the client close")
					}
				},
			}
		}(),
		// an abrupt transport drop surfaces as a raw io.EOF, which counts as
		// a clean end — matching the pre-rework handler, which treated
		// io.EOF like a normal closure.
		"Abrupt client close": func() tcase {
			readErrs := make(chan error, 1)

			return tcase{
				Man: &ManagerMock{
					ChatFunc: func(ctx context.Context, _, _ string, conn protocol.SessionConn) error {
						_, err := conn.Read(ctx)
						readErrs <- err

						if errors.Is(err, io.EOF) {
							return nil
						}

						return err
					},
				},
				Chats: 1,
				Client: func(t *testing.T, conn *websocket.Conn) {
					require.NoError(t, conn.CloseNow())

					select {
					case err := <-readErrs:
						assert.ErrorIs(t, err, io.EOF)
					case <-time.After(5 * time.Second):
						t.Fatal("chat loop did not observe the dropped connection")
					}
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := &Handler{
				log:        slog.New(slog.DiscardHandler),
				assistant:  c.Man,
				acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
			}

			if c.Client != nil {
				c.Client(t, dial(t, chatServer(t, hdl)))
			} else {
				req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

				if !c.NoSession {
					req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
						UserID:               "u1",
						ActiveOrganizationID: "org1",
					}))
				}

				rec := httptest.NewRecorder()

				hdl.HandleChat(rec, req)
				c.Resp(t, rec)
			}

			ff := c.Man.ChatCalls()
			require.Len(t, ff, c.Chats)

			if c.Chats == 0 {
				return
			}

			assert.Equal(t, "org1", ff[0].OrgID)
			assert.Equal(t, "u1", ff[0].UserID)
			assert.NotNil(t, ff[0].Conn)
		})
	}
}

func Test_wsConn_Read(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Do     func(t *testing.T, conn *websocket.Conn)
		Status websocket.StatusCode
		Result []byte
		Err    error
	}{
		"Client message is returned": {
			Do: func(t *testing.T, conn *websocket.Conn) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"a":1}`)))
			},
			Result: []byte(`{"a":1}`),
		},
		"Normal closure maps to EOF": {
			Do: func(t *testing.T, conn *websocket.Conn) {
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			},
			Err: io.EOF,
		},
		"Going away maps to EOF": {
			Do: func(t *testing.T, conn *websocket.Conn) {
				require.NoError(t, conn.Close(websocket.StatusGoingAway, "tab closed"))
			},
			Err: io.EOF,
		},
		"Abnormal close passes through": {
			Do: func(t *testing.T, conn *websocket.Conn) {
				require.NoError(t, conn.Close(websocket.StatusPolicyViolation, "nope"))
			},
			Status: websocket.StatusPolicyViolation,
			Err:    assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				wcCh = make(chan *wsConn, 1)
				hold = make(chan struct{})
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
				if err != nil {
					return
				}

				wcCh <- &wsConn{
					log:    slog.New(slog.DiscardHandler),
					conn:   conn,
					cancel: func() {},
				}

				// keep the handler alive until the test read finished,
				// or returning would tear the connection down under it.
				<-hold
			}))

			t.Cleanup(srv.Close)
			t.Cleanup(func() { close(hold) })

			conn := dial(t, srv)

			t.Cleanup(func() {
				conn.CloseNow() //nolint:errcheck,gosec // error provides no meaningful info
			})

			wc := <-wcCh

			// the read runs concurrently with Do: a client-side Close
			// performs a handshake that only completes once this side
			// reads the close frame.
			type readResult struct {
				data []byte
				err  error
			}

			resCh := make(chan readResult, 1)

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				data, err := wc.Read(ctx)
				resCh <- readResult{data: data, err: err}
			}()

			c.Do(t, conn)

			res := <-resCh
			data, err := res.data, res.err
			testutil.AssertEqualError(t, c.Err, err)

			if c.Status != 0 {
				assert.Equal(t, c.Status, websocket.CloseStatus(err))
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, data)
		})
	}
}

func Test_wsConn_WriteJSON(t *testing.T) {
	t.Parallel()

	type msg struct {
		N int `json:"n"`
	}

	type tcase struct {
		// Cancelled receives one value when the connection's cancel
		// callback fires.
		Cancelled chan struct{}

		Do func(t *testing.T, wc *wsConn, conn *websocket.Conn)
	}

	cc := map[string]tcase{
		"Concurrent writes all arrive intact": {
			Cancelled: make(chan struct{}, 1),
			Do: func(t *testing.T, wc *wsConn, conn *websocket.Conn) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				var wg sync.WaitGroup

				for i := range 10 {
					wg.Go(func() {
						wc.WriteJSON(ctx, msg{N: i})
					})
				}

				seen := make(map[int]bool)

				for range 10 {
					var m msg

					require.NoError(t, wsjson.Read(ctx, conn, &m))

					seen[m.N] = true
				}

				wg.Wait()
				assert.Len(t, seen, 10)

				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			},
		},
		"A write error cancels the connection": func() tcase {
			cancelled := make(chan struct{}, 1)

			return tcase{
				Cancelled: cancelled,
				Do: func(t *testing.T, wc *wsConn, conn *websocket.Conn) {
					defer conn.CloseNow() //nolint:errcheck // error provides no meaningful info

					// writing on a closed connection must trip the cancel
					// callback, which is what ends the chat loop above it.
					require.NoError(t, wc.conn.CloseNow())

					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()

					wc.WriteJSON(ctx, struct{}{})

					select {
					case <-cancelled:
					case <-time.After(5 * time.Second):
						t.Fatal("the write failure did not cancel the connection")
					}
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				wc       *wsConn
				accepted = make(chan struct{})
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
				if err != nil {
					return
				}

				wc = &wsConn{
					log:  slog.New(slog.DiscardHandler),
					conn: conn,
					cancel: func() {
						select {
						case c.Cancelled <- struct{}{}:
						default:
						}
					},
				}

				close(accepted)

				// hold the connection open until the client is done.
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()

				conn.Read(ctx) //nolint:errcheck,gosec // the read only blocks until the client closes
			}))

			t.Cleanup(srv.Close)

			conn := dial(t, srv)

			<-accepted

			c.Do(t, wc, conn)
		})
	}
}
