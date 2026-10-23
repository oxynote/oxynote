package slack

import (
	"context"
	"encoding/json"
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

	man, err := slack.NewManager(log, notFoundManagerDB{}, nil, fakeReceiver{}, nil, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewHandler(log, db, client, man)
}

func Test_Handler_CheckInstallation(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB      *DBMock
		Fetched int
	}{
		"Slack app not configured": {
			DB: &DBMock{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := newMockedHandler(t, false, c.DB, nil)

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

			assert.Len(t, c.DB.FetchSlackAppByOrganizationIDCalls(), c.Fetched)
		})
	}
}

func Test_Handler_RequireConfigured(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		WantStatus int
		WantCode   string
	}{
		"Slack app not configured": {
			Configured: false,
			WantStatus: http.StatusConflict,
			WantCode:   "slack.not_configured",
		},
		"Configured passes through": {
			Configured: true,
			WantStatus: http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := newMockedHandler(t, c.Configured, &DBMock{}, nil)

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/slack/install", http.NoBody)
			rec := httptest.NewRecorder()

			h.RequireConfigured(next).ServeHTTP(rec, req)

			if rec.Code != c.WantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, c.WantStatus)
			}

			if c.WantCode == "" {
				return
			}

			var body map[string]string

			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if body["code"] != c.WantCode {
				t.Fatalf("code = %q, want %q", body["code"], c.WantCode)
			}
		})
	}
}

func Test_Handler_VerifySignature(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		RespCode int
	}{
		"Slack app not configured": {
			RespCode: http.StatusNotFound,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := newMockedHandler(t, false, &DBMock{}, nil)

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/apps/slack/events", http.NoBody)
			rec := httptest.NewRecorder()

			h.VerifySignature(next).ServeHTTP(rec, req)

			if rec.Code != c.RespCode {
				t.Fatalf("status = %d, want %d", rec.Code, c.RespCode)
			}
		})
	}
}

func Test_Handler_InstallApp(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		RespCode int
		Inserted int
	}{
		"Slack app not configured": {
			DB:       &DBMock{},
			RespCode: http.StatusNotFound,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := newMockedHandler(t, false, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/apps/slack/install?code=abc", http.NoBody)
			rec := httptest.NewRecorder()

			h.InstallApp(rec, req)

			if rec.Code != c.RespCode {
				t.Fatalf("status = %d, want %d", rec.Code, c.RespCode)
			}

			assert.Len(t, c.DB.InsertSlackAppCalls(), c.Inserted)
		})
	}
}

func Test_Handler_FetchInstall(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		DB         *DBMock
		NoSession  bool
		RespCode   int
		Fetched    int
	}{
		"No session in context": {
			Configured: true,
			DB:         &DBMock{},
			NoSession:  true,
			RespCode:   http.StatusUnauthorized,
		},
		"Error returned by DB.FetchSlackAppByOrganizationID": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Organization already connected": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Slack app not configured": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusConflict,
			Fetched:  1,
		},
		"Successful URL creation": {
			Configured: true,
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchInstall(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackAppByOrganizationIDCalls(), c.Fetched)

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
		RespJSON  string
		Fetched   int
		Updated   int
		Inserted  int
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
			RespJSON: `{"code":"slack.installation_state_mismatch","message":"installation state does not match the active organization"}`,
		},
		"External state with missing code": {
			DB:       &DBMock{},
			StateFor: "org1",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchSlackAppByTeamID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			StateFor: "internal",
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Internal state app already assigned": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			StateFor: "internal",
			RespCode: http.StatusBadRequest,
			Fetched:  1,
		},
		"Error returned by DB.UpdateSlackAppOrganizationID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", Token: "tkn"}, nil
				},
				UpdateSlackAppOrganizationIDFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			StateFor: "internal",
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Updated:  1,
		},
		"Successful internal connection": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", Token: "tkn"}, nil
				},
			},
			StateFor: "internal",
			RespCode: http.StatusNoContent,
			Fetched:  1,
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

				u, err := url.Parse(rawURL)
				require.NoError(t, err)

				state = u.Query().Get("state")
			case "-":
				state = ""
			}

			q := url.Values{}

			if state != "" {
				q.Set("state", state)
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?"+q.Encode(), http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.ConnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)

			if c.RespJSON != "" {
				assert.JSONEq(t, c.RespJSON, rec.Body.String())
			}

			assert.Len(t, c.DB.FetchSlackAppByTeamIDCalls(), c.Fetched)
			assert.Len(t, c.DB.UpdateSlackAppOrganizationIDCalls(), c.Updated)
			assert.Len(t, c.DB.InsertSlackAppCalls(), c.Inserted)

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
		Fetched   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Error returned by DB.FetchSlackMessages": {
			DB: &DBMock{
				FetchSlackMessagesFunc: func(context.Context, string) ([]slack.Message, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchSlackMessagesFunc: func(context.Context, string) ([]slack.Message, error) {
					return []slack.Message{}, nil
				},
			},
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchMessages(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackMessagesCalls(), c.Fetched)
		})
	}
}

func Test_Handler_DisconnectOrganization(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		NoSession  bool
		RespCode   int
		Fetched    int
		Unassigned int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			RespCode:  http.StatusUnauthorized,
		},
		"Error returned by DB.FetchSlackAppByOrganizationID": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"App not connected": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
			Fetched:  1,
		},
		"Error returned by DB.UnassignSlackAppOrganization": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				UnassignSlackAppOrganizationFunc: func(context.Context, string) error {
					return assert.AnError
				},
			},
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Unassigned: 1,
		},
		"Successful disconnection": {
			DB: &DBMock{
				FetchSlackAppByOrganizationIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			RespCode:   http.StatusNoContent,
			Fetched:    1,
			Unassigned: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB, nil)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.DisconnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchSlackAppByOrganizationIDCalls(), c.Fetched)
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
		Fetched  int
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
		"Error returned by DB.DeleteSlackApp": {
			DB: &DBMock{
				DeleteSlackAppFunc: func(context.Context, string) error {
					return assert.AnError
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"app_uninstalled"}}`,
			RespCode: http.StatusInternalServerError,
			Deleted:  1,
		},
		"Channel open for connected workspace": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"im_open","user":"slack-u1","channel":"chan1"}}`,
			RespCode: http.StatusNoContent,
			Fetched:  1,
		},
		"Error returned by DB.FetchSlackAppByTeamID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			Body:     `{"type":"event_callback","team_id":"team1","event":{"type":"im_open","user":"slack-u1","channel":"chan1"}}`,
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
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
			assert.Len(t, c.DB.FetchSlackAppByTeamIDCalls(), c.Fetched)
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
		Fetched  int
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
		"Error returned by DB.FetchSlackAppByTeamID": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return nil, assert.AnError
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Error returned by DB.InsertSlackMessage": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
				InsertSlackMessageFunc: func(context.Context, slack.Message) error {
					return assert.AnError
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Inserted: 1,
		},
		"Successful message submission": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			Payload:  interaction("view_submission"),
			RespCode: http.StatusNoContent,
			Fetched:  1,
			Inserted: 1,
			Posted:   1,
		},
		"Ignored interaction type": {
			DB: &DBMock{
				FetchSlackAppByTeamIDFunc: func(context.Context, string) (*slack.App, error) {
					return &slack.App{TeamID: "team1", OrganizationID: null.StringFrom("org1"), Token: "tkn"}, nil
				},
			},
			Payload:  interaction("shortcut"),
			RespCode: http.StatusNoContent,
			Fetched:  1,
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
			assert.Len(t, c.DB.FetchSlackAppByTeamIDCalls(), c.Fetched)
			assert.Len(t, c.DB.InsertSlackMessageCalls(), c.Inserted)
			assert.Equal(t, c.Posted, posted)

			if c.Inserted > 0 {
				assert.Equal(t, "saved message", c.DB.InsertSlackMessageCalls()[0].Msg.Text)
				assert.Equal(t, "org1", c.DB.InsertSlackMessageCalls()[0].Msg.OrganizationID)
			}
		})
	}
}
