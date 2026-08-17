package slack

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeReceiver is a no-op notification receiver for tests.
type fakeReceiver struct{}

func (fakeReceiver) OnNotification(_ func(context.Context, notification.Notification)) notification.Unsubscribe {
	return func() {}
}

// newTestHandler creates a Handler backed by a Slack manager that is either
// configured with dummy credentials or fully unconfigured.
func newTestHandler(t *testing.T, configured bool) *Handler {
	t.Helper()

	log := slog.New(slog.DiscardHandler)
	opt := slack.Options{}

	if configured {
		opt = slack.Options{
			ClientID:                  "id",
			ClientSecret:              "secret",
			SignatureSecret:           "sig",
			RedirectURL:               "http://localhost/slack",
			InstallationSigningSecret: "0123456789abcdef0123456789abcdef",
		}
	}

	man, err := slack.NewManager(log, nil, nil, fakeReceiver{}, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewHandler(log, nil, nil, man)
}

// notFoundManagerDB is a slack.DB that never finds anything, keeping
// manager-created Slack clients (and their network calls) out of tests.
type notFoundManagerDB struct{}

func (notFoundManagerDB) FetchSlackAppByTeamID(context.Context, string) (*slack.App, error) {
	return nil, errutil.ErrNotFound
}

func (notFoundManagerDB) FetchSlackAppByOrganizationID(context.Context, string) (*slack.App, error) {
	return nil, errutil.ErrNotFound
}

func (notFoundManagerDB) FetchSlackUserLinkByUserID(context.Context, string, string) (*slack.UserLink, error) {
	return nil, errutil.ErrNotFound
}

// newMockedHandler builds a Handler with a mocked DB and an optional HTTP
// client for response-URL webhooks.
func newMockedHandler(t *testing.T, configured bool, db *DBMock, client *http.Client) *Handler {
	t.Helper()

	log := slog.New(slog.DiscardHandler)

	opt := slack.Options{}

	if configured {
		opt = slack.Options{
			ClientID:                  "id",
			ClientSecret:              "secret",
			SignatureSecret:           "sig",
			RedirectURL:               "http://localhost/slack",
			InstallationSigningSecret: "0123456789abcdef0123456789abcdef",
		}
	}

	man, err := slack.NewManager(log, notFoundManagerDB{}, nil, fakeReceiver{}, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewHandler(log, db, client, man)
}

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

// responseServer starts a local server standing in for Slack response
// URLs and returns it with a counter of received posts.
func responseServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()

	var hits int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++

		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(func() {
		srv.Close()
		http.DefaultClient.CloseIdleConnections()
	})

	return srv, &hits
}

func Test_Handler_CheckInstallation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/slack", http.NoBody)
	req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
		UserID:               "user",
		ActiveOrganizationID: "org",
	}))

	rec := httptest.NewRecorder()
	h.CheckInstallation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]bool

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["connected"] || body["configured"] {
		t.Fatalf("body = %v, want connected=false configured=false", body)
	}
}

func Test_Handler_RequireConfigured(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Configured bool
		WantStatus int
		WantCode   string
	}{
		"Not configured responds with slack.not_configured": {
			Configured: false,
			WantStatus: http.StatusConflict,
			WantCode:   "slack.not_configured",
		},
		"Configured passes through": {
			Configured: true,
			WantStatus: http.StatusNoContent,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t, tc.Configured)

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/slack/install", http.NoBody)
			rec := httptest.NewRecorder()

			h.RequireConfigured(next).ServeHTTP(rec, req)

			if rec.Code != tc.WantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.WantStatus)
			}

			if tc.WantCode == "" {
				return
			}

			var body map[string]string

			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if body["code"] != tc.WantCode {
				t.Fatalf("code = %q, want %q", body["code"], tc.WantCode)
			}
		})
	}
}

func Test_Handler_VerifySignature(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/slack/events", http.NoBody)
	rec := httptest.NewRecorder()

	h.VerifySignature(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func Test_Handler_InstallApp(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/slack/install?code=abc", http.NoBody)
	rec := httptest.NewRecorder()

	h.InstallApp(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// stateFrom extracts the encrypted state parameter from a manager URL.
func stateFrom(t *testing.T, rawURL string) string {
	t.Helper()

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	return u.Query().Get("state")
}

// connectedApp returns a Slack app assigned to org1.
func connectedApp() *slack.App {
	return &slack.App{
		TeamID:         "team1",
		OrganizationID: null.StringFrom("org1"),
		Token:          "tkn",
	}
}

// unassignedApp returns a Slack app without an organization.
func unassignedApp() *slack.App {
	return &slack.App{
		TeamID: "team1",
		Token:  "tkn",
	}
}

// userLink returns a stored Slack user link for u1.
func userLink() *slack.UserLink {
	return &slack.UserLink{
		SlackUserID: "slack-u1",
		TeamID:      "team1",
		UserID:      "u1",
		Settings:    slack.UserLinkSettings{Notifications: true},
	}
}

func Test_Handler_FetchInstall(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		DB         *DBMock
		NoSession  bool
		RespCode   int
	}{
		"No session in context": {
			Configured: true,
			DB:         &DBMock{},
			NoSession:  true,
			RespCode:   http.StatusUnauthorized,
		},
		"App lookup error": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Organization already connected": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			RespCode: http.StatusBadRequest,
		},
		"Slack app not configured": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusConflict,
		},
		"Successful URL creation": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchInstall(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespCode == http.StatusOK {
				assert.Contains(t, rec.Body.String(), "state=")
			}
		})
	}
}

func Test_Handler_ConnectOrganization(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		StateFor  string
		RawState  string
		RespCode  int
		Updated   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			StateFor:  "internal",
			RespCode:  http.StatusUnauthorized,
		},
		"Missing installation state": {
			DB:       &DBMock{},
			RawState: "-",
			RespCode: http.StatusBadRequest,
		},
		// a tampered or truncated state is the caller's problem, not a
		// server fault worth paging anyone over.
		"Invalid installation state": {
			DB:       &DBMock{},
			RawState: "garbage",
			RespCode: http.StatusBadRequest,
		},
		"External state organization mismatch": {
			DB:       &DBMock{},
			StateFor: "org2",
			RespCode: http.StatusBadRequest,
		},
		"External state with missing code": {
			DB:       &DBMock{},
			StateFor: "org1",
			RespCode: http.StatusBadRequest,
		},
		"Internal state app lookup error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			StateFor: "internal",
			RespCode: http.StatusInternalServerError,
		},
		"Internal state app already assigned": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			StateFor: "internal",
			RespCode: http.StatusBadRequest,
		},
		"Internal state assignment error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return unassignedApp(), nil
				},
				UpdateSlackAppOrganizationIDFunc: func(context.Context, string, string) error {
					return errors.New("boom")
				},
			},
			StateFor: "internal",
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful internal connection": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return unassignedApp(), nil
				},
			},
			StateFor: "internal",
			RespCode: http.StatusNoContent,
			Updated:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			state := c.RawState

			switch state {
			case "":
				var (
					rawURL string
					err    error
				)

				if c.StateFor == "internal" {
					rawURL, err = hdl.man.CreateInternalInstallationURL("team1")
				} else {
					rawURL, err = hdl.man.CreateExternalInstallationURL(c.StateFor)
				}

				require.NoError(t, err)

				state = stateFrom(t, rawURL)
			case "-":
				state = ""
			}

			q := url.Values{}

			if state != "" {
				q.Set("state", state)
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?"+q.Encode(), http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.ConnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UpdateSlackAppOrganizationIDCalls(), c.Updated)

			if c.Updated > 0 {
				assert.Equal(t, "team1", c.DB.UpdateSlackAppOrganizationIDCalls()[0].TeamID)
				assert.Equal(t, "org1", c.DB.UpdateSlackAppOrganizationIDCalls()[0].OrganizationID)
			}
		})
	}
}

func Test_Handler_FetchMessages(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Message fetch error": {
			DB: &DBMock{
				FetchSlackMessagesFunc: func(context.Context, string) ([]slack.Message, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchSlackMessagesFunc: func(context.Context, string) ([]slack.Message, error) {
					return []slack.Message{}, nil
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchMessages(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
		})
	}
}

func Test_Handler_DisconnectOrganization(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		RespCode   int
		Unassigned int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"App not connected": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
		},
		"Unassignment error": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				UnassignSlackAppOrganizationFunc: func(context.Context, string) error {
					return errors.New("boom")
				},
			},
			RespCode:   http.StatusInternalServerError,
			Unassigned: 1,
		},
		"Successful disconnection": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			RespCode:   http.StatusNoContent,
			Unassigned: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.DisconnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UnassignSlackAppOrganizationCalls(), c.Unassigned)
		})
	}
}

func Test_Handler_HandleEvent(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Body     string
		RespCode int
		RespJSON string
		Deleted  int
	}{
		"Unparseable event payload": {
			DB:       &DBMock{},
			Body:     "{",
			RespCode: http.StatusInternalServerError,
		},
		"URL verification challenge": {
			DB:       &DBMock{},
			Body:     `{"type":"url_verification","challenge":"chlg"}`,
			RespCode: http.StatusOK,
			RespJSON: `{"challenge":"chlg"}`,
		},
		"App uninstalled": {
			DB:       &DBMock{},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"app_uninstalled"}}`,
			RespCode: http.StatusNoContent,
			Deleted:  1,
		},
		"App uninstall deletion error": {
			DB: &DBMock{
				DeleteSlackAppFunc: func(context.Context, string) error {
					return errors.New("boom")
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"app_uninstalled"}}`,
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Channel open for connected workspace": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"im_open","user":"slack-u1","channel":"chan1"}}`,
			RespCode: http.StatusNoContent,
		},
		"Channel open app lookup error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"im_open","user":"slack-u1","channel":"chan1"}}`,
			RespCode: http.StatusInternalServerError,
		},
		"Ignored event type": {
			DB:       &DBMock{},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"reaction_added"}}`,
			RespCode: http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(c.Body))
			rec := httptest.NewRecorder()

			hdl.HandleEvent(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.DB.DeleteSlackAppCalls(), c.Deleted)
		})
	}
}

func Test_Handler_HandleCommand(t *testing.T) {
	t.Parallel()

	interaction := func(typ string) string {
		return `{"type":"` + typ + `","team":{"id":"team1"},"user":{"id":"slack-u1"},"view":{"state":{"values":{"message_input_block":{"message_input_block_action":{"value":"saved message"}}}}}}`
	}

	cc := map[string]struct {
		DB       *DBMock
		RawBody  string
		Payload  string
		RespCode int
		Inserted int
		Posted   int
	}{
		"Invalid form body": {
			DB:       &DBMock{},
			RawBody:  "payload=%zz",
			RespCode: http.StatusBadRequest,
		},
		"Invalid payload JSON": {
			DB:       &DBMock{},
			Payload:  "{",
			RespCode: http.StatusBadRequest,
		},
		"App lookup error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusInternalServerError,
		},
		"Message submission insert error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				InsertSlackMessageFunc: func(context.Context, slack.Message) error {
					return errors.New("boom")
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		"Successful message submission": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusNoContent,
			Inserted: 1,
			Posted:   1,
		},
		"Ignored interaction type": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
			},
			Payload:  interaction("shortcut"),
			RespCode: http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			var posted int

			hdl.message.postCallback = func(string, xid.ID) { posted++ }

			body := c.RawBody

			if body == "" {
				body = url.Values{"payload": {c.Payload}}.Encode()
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()

			hdl.HandleCommand(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertSlackMessageCalls(), c.Inserted)
			assert.Equal(t, c.Posted, posted)

			if c.Inserted > 0 {
				assert.Equal(t, "saved message", c.DB.InsertSlackMessageCalls()[0].Msg.Text)
				assert.Equal(t, "org1", c.DB.InsertSlackMessageCalls()[0].Msg.OrganizationID)
			}
		})
	}
}

func Test_Handler_HandleSlashCommand(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Command  string
		RespCode int
		Hits     int
		Deleted  int
	}{
		"Unknown command sends ephemeral response": {
			DB:       &DBMock{},
			Command:  "/bogus",
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Link with app lookup error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			Command:  _linkCommand,
			RespCode: http.StatusInternalServerError,
		},
		"Link on unconnected workspace": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return unassignedApp(), nil
				},
			},
			Command:  _linkCommand,
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Link with lookup error on user link": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errors.New("boom")
				},
			},
			Command:  _linkCommand,
			RespCode: http.StatusInternalServerError,
		},
		"Link when already linked": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
			},
			Command:  _linkCommand,
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Successful link invitation": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Command:  _linkCommand,
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Unlink when not linked": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Command:  _unlinkCommand,
			RespCode: http.StatusNoContent,
			Hits:     1,
		},
		"Unlink with lookup error": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errors.New("boom")
				},
			},
			Command:  _unlinkCommand,
			RespCode: http.StatusInternalServerError,
		},
		"Unlink with deletion error": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
				DeleteSlackUserLinkFunc: func(context.Context, string, string) error {
					return errors.New("boom")
				},
			},
			Command:  _unlinkCommand,
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful unlink": {
			DB: &DBMock{
				FetchSlackUserLinkFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
			},
			Command:  _unlinkCommand,
			RespCode: http.StatusNoContent,
			Hits:     1,
			Deleted:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			srv, hits := responseServer(t)

			hdl := newMockedHandler(t, true, c.DB, srv.Client())

			form := url.Values{
				"command":      {c.Command},
				"user_id":      {"slack-u1"},
				"team_id":      {"team1"},
				"response_url": {srv.URL},
			}

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rec := httptest.NewRecorder()

			hdl.HandleSlashCommand(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, c.Hits, *hits)
			assert.Len(t, c.DB.DeleteSlackUserLinkCalls(), c.Deleted)
		})
	}
}

func Test_Handler_FetchUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
		RespJSON  string
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Link fetch error": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
			},
			RespCode: http.StatusOK,
			RespJSON: `{"notifications":true}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.FetchUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}
		})
	}
}

func Test_Handler_UpdateUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		Body      string
		RespCode  int
		Updated   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			Body:      `{"notifications":false}`,
			RespCode:  http.StatusUnauthorized,
		},
		"Invalid JSON body": {
			DB:       &DBMock{},
			Body:     "{",
			RespCode: http.StatusBadRequest,
		},
		"Link fetch error": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusNotFound,
		},
		"Link update error": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
				UpdateSlackUserLinkFunc: func(context.Context, slack.UserLink) error {
					return errors.New("boom")
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful update": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
			},
			Body:     `{"notifications":false}`,
			RespCode: http.StatusOK,
			Updated:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodPut, "http://test.com/", strings.NewReader(c.Body))

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.UpdateUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UpdateSlackUserLinkCalls(), c.Updated)

			if c.Updated > 0 {
				assert.False(t, c.DB.UpdateSlackUserLinkCalls()[0].Link.Settings.Notifications)
			}
		})
	}
}

func Test_Handler_DeleteUserLink(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RespCode  int
		Deleted   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Link not found": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusBadRequest,
		},
		"Link fetch error": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return nil, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Link deletion error": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
				DeleteSlackUserLinkFunc: func(context.Context, string, string) error {
					return errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Successful deletion": {
			DB: &DBMock{
				FetchSlackUserLinkByUserIDFunc: func(context.Context, string, string) (*slack.UserLink, error) {
					return userLink(), nil
				},
			},
			RespCode: http.StatusNoContent,
			Deleted:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.DeleteUserLink(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.DeleteSlackUserLinkCalls(), c.Deleted)
		})
	}
}

func Test_Handler_LinkUser(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoSession bool
		RawState  string
		StateOrg  string
		RespCode  int
		Inserted  int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			StateOrg:  "org1",
			RespCode:  http.StatusUnauthorized,
		},
		"Missing link state": {
			DB:       &DBMock{},
			RawState: "-",
			RespCode: http.StatusBadRequest,
		},
		"Invalid link state": {
			DB:       &DBMock{},
			RawState: "garbage",
			RespCode: http.StatusInternalServerError,
		},
		"App lookup error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
		},
		"Workspace connected to another organization": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					app := connectedApp()
					app.OrganizationID = null.StringFrom("org9")

					return app, nil
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusBadRequest,
		},
		"Member fetch error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return nil, errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
		},
		"User not an organization member": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u2"}, nil
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusForbidden,
		},
		"Link insertion error": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u1"}, nil
				},
				InsertSlackUserLinkFunc: func(context.Context, slack.UserLink) error {
					return errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
			Inserted: 1,
		},
		"Successful link with unavailable confirmation client": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return connectedApp(), nil
				},
				FetchOrganizationMembersFunc: func(context.Context, string) ([]string, error) {
					return []string{"u1"}, nil
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusOK,
			Inserted: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			state := c.RawState

			switch state {
			case "":
				linkURL, err := hdl.man.CreateLinkURL("slack-u1", "team1", c.StateOrg)
				require.NoError(t, err)

				state = stateFrom(t, linkURL)
			case "-":
				state = ""
			}

			q := url.Values{}

			if state != "" {
				q.Set("state", state)
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?"+q.Encode(), http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.LinkUser(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertSlackUserLinkCalls(), c.Inserted)

			if c.RespCode == http.StatusOK {
				assert.JSONEq(t, `{"linked":true}`, rec.Body.String())
			}
		})
	}
}
