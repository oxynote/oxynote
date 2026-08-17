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
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/assistant/protocol"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
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

	man := &assistantCore.Manager{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), man, websocket.AcceptOptions{InsecureSkipVerify: true})
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.True(t, hdl.acceptOpts.InsecureSkipVerify)
	assert.Same(t, man, hdl.assistant)
}

// testHandlerHandleChatNoSession is a case of Handler_HandleChat, run as a subtest of it.
func testHandlerHandleChatNoSession(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{}

	hdl := &Handler{
		log:       slog.New(slog.DiscardHandler),
		assistant: man,
	}

	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()

	hdl.HandleChat(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"code":"account.not_authenticated","message":"not authenticated"}`, rec.Body.String())
	assert.Empty(t, man.ChatCalls())
}

// testHandlerHandleChatUpgradeFailure is a case of Handler_HandleChat, run as a subtest of it.
func testHandlerHandleChatUpgradeFailure(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{}

	hdl := &Handler{
		log:       slog.New(slog.DiscardHandler),
		assistant: man,
	}

	// a plain GET without upgrade headers cannot be accepted.
	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	}))

	rec := httptest.NewRecorder()

	hdl.HandleChat(rec, req)

	assert.NotEqual(t, http.StatusSwitchingProtocols, rec.Code)
	assert.Empty(t, man.ChatCalls())
}

// testHandlerHandleChatChatError is a case of Handler_HandleChat, run as a subtest of it.
func testHandlerHandleChatChatError(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{
		ChatFunc: func(context.Context, string, string, protocol.SessionConn) error {
			return errors.New("boom")
		},
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

	defer conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck // already closed by the server

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// a failed chat closes the socket with an internal error status.
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	assert.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err))

	ff := man.ChatCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org1", ff[0].OrgID)
	assert.Equal(t, "u1", ff[0].UserID)
	assert.NotNil(t, ff[0].Conn)
}

// testHandlerHandleChatConversationRoundTrip is a case of Handler_HandleChat, run as a subtest of it.
func testHandlerHandleChatConversationRoundTrip(t *testing.T) {
	t.Parallel()

	// the chat loop reads a message, echoes a reply, and sees the
	// clean client close as io.EOF through the adapter.
	readErrs := make(chan error, 1)
	man := &ManagerMock{
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
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

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
}

// testHandlerHandleChatAbruptClose is a case of Handler_HandleChat, run as a subtest of it.
func testHandlerHandleChatAbruptClose(t *testing.T) {
	t.Parallel()

	// an abrupt transport drop surfaces as a raw io.EOF, which counts
	// as a clean end — matching the pre-rework handler, which treated
	// io.EOF like a normal closure.
	readErrs := make(chan error, 1)
	man := &ManagerMock{
		ChatFunc: func(ctx context.Context, _, _ string, conn protocol.SessionConn) error {
			_, err := conn.Read(ctx)
			readErrs <- err

			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		},
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

	require.NoError(t, conn.CloseNow())

	select {
	case err := <-readErrs:
		assert.ErrorIs(t, err, io.EOF)
	case <-time.After(5 * time.Second):
		t.Fatal("chat loop did not observe the dropped connection")
	}
}

func Test_Handler_HandleChat(t *testing.T) {
	t.Parallel()

	t.Run("Unauthenticated request", testHandlerHandleChatNoSession)
	t.Run("Websocket upgrade failure", testHandlerHandleChatUpgradeFailure)
	t.Run("Chat failure closes the socket", testHandlerHandleChatChatError)
	t.Run("Conversation round trip", testHandlerHandleChatConversationRoundTrip)
	t.Run("Abrupt client close", testHandlerHandleChatAbruptClose)
}

// testWsConnWriteJSONCancelsOnError is a case of wsConn_WriteJSON, run as a subtest of it.
func testWsConnWriteJSONCancelsOnError(t *testing.T) {
	t.Parallel()

	conns := make(chan *wsConn, 1)
	man := &ManagerMock{
		ChatFunc: func(ctx context.Context, _, _ string, conn protocol.SessionConn) error {
			conns <- conn.(*wsConn)

			// block until the write failure cancels the context.
			<-ctx.Done()

			return nil
		},
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

	defer conn.CloseNow() //nolint:errcheck // error provides no meaningful info

	var wc *wsConn

	select {
	case wc = <-conns:
	case <-time.After(5 * time.Second):
		t.Fatal("chat was not started")
	}

	// writing on a closed connection must trip the cancel callback,
	// which unblocks the chat loop above.
	require.NoError(t, wc.conn.CloseNow())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wc.WriteJSON(ctx, struct{}{})
}

func Test_wsConn_WriteJSON(t *testing.T) {
	t.Parallel()

	t.Run("A write error cancels the connection", testWsConnWriteJSONCancelsOnError)

	type msg struct {
		N int `json:"n"`
	}

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
			log:    slog.New(slog.DiscardHandler),
			conn:   conn,
			cancel: func() {},
		}

		close(accepted)

		// hold the connection open until the client is done reading.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		conn.Read(ctx) //nolint:errcheck,gosec // the read only blocks until the client closes
	}))

	t.Cleanup(srv.Close)

	conn := dial(t, srv)

	<-accepted

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// concurrent writes must all arrive intact.
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
}
