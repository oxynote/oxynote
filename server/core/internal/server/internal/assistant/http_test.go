package assistant

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	assistantCore "github.com/oxynote/oxynote/server/core/internal/assistant"
	assistantMock "github.com/oxynote/oxynote/server/core/internal/assistant/_mock"
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

func Test_Handler_HandleChat_NoSession(t *testing.T) {
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
	assert.Empty(t, man.NewSessionCalls())
}

func Test_Handler_HandleChat_UpgradeFailure(t *testing.T) {
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
	assert.Empty(t, man.NewSessionCalls())
}

func Test_Handler_HandleChat_SessionCreationError(t *testing.T) {
	t.Parallel()

	man := &ManagerMock{
		NewSessionFunc: func(context.Context, string, string, protocol.SessionWriter) (assistantCore.Session, error) {
			return nil, errors.New("boom")
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

	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	assert.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err))

	ff := man.NewSessionCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org1", ff[0].OrgID)
	assert.Equal(t, "u1", ff[0].UserID)
	assert.NotNil(t, ff[0].Writer)
}

func Test_Handler_HandleChat_MessageDispatch(t *testing.T) {
	t.Parallel()

	session := &assistantMock.Session{}
	man := &ManagerMock{
		NewSessionFunc: func(context.Context, string, string, protocol.SessionWriter) (assistantCore.Session, error) {
			return session, nil
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

	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"first"}`)))
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"second"}`)))

	// both messages must reach the session before closing.
	require.Eventually(t, func() bool {
		return len(session.ProcessCalls()) == 2
	}, 5*time.Second, 10*time.Millisecond)

	assert.JSONEq(t, `{"type":"first"}`, string(session.ProcessCalls()[0].Msg))
	assert.JSONEq(t, `{"type":"second"}`, string(session.ProcessCalls()[1].Msg))

	// a normal closure ends the read loop and closes the session.
	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "bye"))

	require.Eventually(t, func() bool {
		return len(session.CloseCalls()) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func Test_Handler_HandleChat_AbruptClose(t *testing.T) {
	t.Parallel()

	session := &assistantMock.Session{}
	man := &ManagerMock{
		NewSessionFunc: func(context.Context, string, string, protocol.SessionWriter) (assistantCore.Session, error) {
			return session, nil
		},
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

	// an abrupt transport close still ends the read loop and closes
	// the session, just through the error branch.
	require.NoError(t, conn.CloseNow())

	require.Eventually(t, func() bool {
		return len(session.CloseCalls()) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func Test_writer_WriteJSON(t *testing.T) {
	t.Parallel()

	type msg struct {
		N int `json:"n"`
	}

	var (
		wr       *writer
		accepted = make(chan struct{})
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}

		wr = &writer{
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
			wr.WriteJSON(ctx, msg{N: i})
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

func Test_writer_WriteJSON_CancelsOnError(t *testing.T) {
	t.Parallel()

	session := &assistantMock.Session{}
	man := &ManagerMock{
		NewSessionFunc: func(context.Context, string, string, protocol.SessionWriter) (assistantCore.Session, error) {
			return session, nil
		},
	}

	hdl := &Handler{
		log:        slog.New(slog.DiscardHandler),
		assistant:  man,
		acceptOpts: websocket.AcceptOptions{InsecureSkipVerify: true},
	}

	srv := chatServer(t, hdl)
	conn := dial(t, srv)

	require.Eventually(t, func() bool {
		return len(man.NewSessionCalls()) == 1
	}, 5*time.Second, 10*time.Millisecond)

	wr, ok := man.NewSessionCalls()[0].Writer.(*writer)
	require.True(t, ok)

	var (
		mu        sync.Mutex
		cancelled bool
	)

	wr.cancel = func() {
		mu.Lock()
		defer mu.Unlock()

		cancelled = true
	}

	// writing on a closed connection must trip the cancel callback.
	require.NoError(t, wr.conn.CloseNow())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wr.WriteJSON(ctx, struct{}{})

	mu.Lock()
	cancelledNow := cancelled
	mu.Unlock()

	assert.True(t, cancelledNow)

	// the read loop exits through its error branch and closes the
	// session.
	require.Eventually(t, func() bool {
		return len(session.CloseCalls()) == 1
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, conn.CloseNow())
}
