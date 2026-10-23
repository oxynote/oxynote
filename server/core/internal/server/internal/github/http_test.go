package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// newTestManager creates a github.Manager for tests. When configured is
// true, a throwaway RSA key is generated so the manager builds a real app
// client.
func newTestManager(t *testing.T, configured bool, db github.DB) *github.Manager {
	t.Helper()

	opt := github.Options{}

	if configured {
		opt.InstallationSigningSecret = "0123456789abcdef0123456789abcdef"
		opt.AppSlug = "test-app"
		opt.SignatureSecret = "sig"

		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		keyPath := filepath.Join(t.TempDir(), "test-key.pem")

		data := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})

		if err := os.WriteFile(keyPath, data, 0o600); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		opt.AppID = 123
		opt.PrivateKeyPath = keyPath
	}

	man, err := github.NewManager(db, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return man
}

func Test_Handler_CheckInstallation(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		RespCode   int
		Connected  bool
		Configured bool
		Fetched    int
	}{
		"Github app not configured": {
			DB:       &DBMock{},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, false, c.DB))

			req := httptest.NewRequest(http.MethodGet, "/api/github", http.NoBody)
			req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
				UserID:               "user",
				ActiveOrganizationID: "org",
			}))

			rec := httptest.NewRecorder()
			h.CheckInstallation(rec, req)

			if rec.Code != c.RespCode {
				t.Fatalf("status = %d, want %d", rec.Code, c.RespCode)
			}

			var body map[string]bool

			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if body["connected"] != c.Connected || body["configured"] != c.Configured {
				t.Fatalf("body = %v, want connected=%t configured=%t", body, c.Connected, c.Configured)
			}

			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)
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
		"Github app not configured": {
			Configured: false,
			WantStatus: http.StatusConflict,
			WantCode:   "github.not_configured",
		},
		"Configured passes through": {
			Configured: true,
			WantStatus: http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(slog.New(slog.DiscardHandler), nil, newTestManager(t, c.Configured, nil))

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/github/install", http.NoBody)
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
		Configured bool
		RespCode   int
	}{
		"Github app not configured": {
			Configured: false,
			RespCode:   http.StatusNotFound,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(slog.New(slog.DiscardHandler), nil, newTestManager(t, c.Configured, nil))

			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/apps/github/events", http.NoBody)
			rec := httptest.NewRecorder()

			h.VerifySignature(next).ServeHTTP(rec, req)

			if rec.Code != c.RespCode {
				t.Fatalf("status = %d, want %d", rec.Code, c.RespCode)
			}
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
		"Error returned by DB.FetchGithubInstallationByOrganizationID": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Organization already connected": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 42, nil
				},
			},
			RespCode: http.StatusConflict,
			Fetched:  1,
		},
		"Github app not configured": {
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusConflict,
			Fetched:  1,
		},
		"Successful URL creation": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusOK,
			Fetched:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, c.Configured, c.DB))

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
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)

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
		StateOrg  string
		RawState  string
		OmitID    bool
		RawID     string
		RespCode  int
		Fetched   int
		Updated   int
	}{
		"No session in context": {
			DB:        &DBMock{},
			NoSession: true,
			StateOrg:  "org1",
			RespCode:  http.StatusUnauthorized,
		},
		"Missing installation ID": {
			DB:       &DBMock{},
			StateOrg: "org1",
			OmitID:   true,
			RespCode: http.StatusBadRequest,
		},
		"Missing installation state": {
			DB:       &DBMock{},
			RawState: "-",
			RespCode: http.StatusBadRequest,
		},
		"Non numeric installation ID": {
			DB:       &DBMock{},
			StateOrg: "org1",
			RawID:    "abc",
			RespCode: http.StatusBadRequest,
		},
		// a tampered or truncated state is the caller's problem, not a
		// server fault worth paging anyone over.
		"Invalid installation state": {
			DB:       &DBMock{},
			RawState: "garbage",
			RespCode: http.StatusBadRequest,
		},
		"State organization mismatch": {
			DB:       &DBMock{},
			StateOrg: "org2",
			RespCode: http.StatusBadRequest,
		},
		"Error returned by DB.FetchGithubInstallationOrganizationID": {
			DB: &DBMock{
				FetchGithubInstallationOrganizationIDFunc: func(context.Context, int64) (null.String, error) {
					return null.String{}, errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
		"Installation already assigned": {
			DB: &DBMock{
				FetchGithubInstallationOrganizationIDFunc: func(context.Context, int64) (null.String, error) {
					return null.StringFrom("org9"), nil
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusConflict,
			Fetched:  1,
		},
		"Error returned by DB.UpdateGithubInstallationOrganizationID": {
			DB: &DBMock{
				UpdateGithubInstallationOrganizationIDFunc: func(context.Context, int64, string) error {
					return errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
			Updated:  1,
		},
		"Successful connection": {
			DB:       &DBMock{},
			StateOrg: "org1",
			RespCode: http.StatusNoContent,
			Fetched:  1,
			Updated:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, true, c.DB))

			state := c.RawState

			switch state {
			case "":
				rawURL, err := hdl.man.CreateInstallationURL(c.StateOrg)
				require.NoError(t, err)

				u, err := url.Parse(rawURL)
				require.NoError(t, err)

				state = u.Query().Get("state")
			case "-":
				state = ""
			}

			q := url.Values{}

			if !c.OmitID {
				id := c.RawID
				if id == "" {
					id = "42"
				}

				q.Set("installation_id", id)
			}

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
			assert.Len(t, c.DB.FetchGithubInstallationOrganizationIDCalls(), c.Fetched)
			assert.Len(t, c.DB.UpdateGithubInstallationOrganizationIDCalls(), c.Updated)

			if c.Updated > 0 {
				assert.Equal(t, int64(42), c.DB.UpdateGithubInstallationOrganizationIDCalls()[0].InstallationID)
				assert.Equal(t, "org1", c.DB.UpdateGithubInstallationOrganizationIDCalls()[0].OrganizationID)
			}
		})
	}
}

func Test_Handler_HandleEvent(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		NoPayload bool
		EventType string
		Payload   string
		RespCode  int
		Inserted  int
		Deleted   int
	}{
		"Missing payload in context": {
			DB:        &DBMock{},
			NoPayload: true,
			RespCode:  http.StatusBadRequest,
		},
		"Unparseable event payload": {
			DB:        &DBMock{},
			EventType: "installation",
			Payload:   "{",
			RespCode:  http.StatusBadRequest,
		},
		"Installation created": {
			DB:        &DBMock{},
			EventType: "installation",
			Payload:   `{"action":"created","installation":{"id":42}}`,
			RespCode:  http.StatusNoContent,
			Inserted:  1,
		},
		"Error returned by DB.InsertGithubInstallation": {
			DB: &DBMock{
				InsertGithubInstallationFunc: func(context.Context, int64) error {
					return errors.New("boom")
				},
			},
			EventType: "installation",
			Payload:   `{"action":"created","installation":{"id":42}}`,
			RespCode:  http.StatusInternalServerError,
			Inserted:  1,
		},
		"Installation deleted": {
			DB:        &DBMock{},
			EventType: "installation",
			Payload:   `{"action":"deleted","installation":{"id":42}}`,
			RespCode:  http.StatusNoContent,
			Deleted:   1,
		},
		"Error returned by DB.DeleteGithubInstallation": {
			DB: &DBMock{
				DeleteGithubInstallationFunc: func(context.Context, int64) error {
					return errors.New("boom")
				},
			},
			EventType: "installation",
			Payload:   `{"action":"deleted","installation":{"id":42}}`,
			RespCode:  http.StatusInternalServerError,
			Deleted:   1,
		},
		"Ignored installation action": {
			DB:        &DBMock{},
			EventType: "installation",
			Payload:   `{"action":"suspend","installation":{"id":42}}`,
			RespCode:  http.StatusNoContent,
		},
		"Ignored event type": {
			DB:        &DBMock{},
			EventType: "push",
			Payload:   `{"ref":"refs/heads/main"}`,
			RespCode:  http.StatusNoContent,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, true, c.DB))

			req := httptest.NewRequest(http.MethodPost, "http://test.com/", http.NoBody)
			req.Header.Set("X-Github-Event", c.EventType)

			if !c.NoPayload {
				req = req.WithContext(context.WithValue(req.Context(), _ctxKeyPayload, []byte(c.Payload)))
			}

			rec := httptest.NewRecorder()

			hdl.HandleEvent(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.InsertGithubInstallationCalls(), c.Inserted)
			assert.Len(t, c.DB.DeleteGithubInstallationCalls(), c.Deleted)

			if c.Inserted > 0 {
				assert.Equal(t, int64(42), c.DB.InsertGithubInstallationCalls()[0].InstallationID)
			}
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
		"Installation not connected": {
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
			Fetched:  1,
		},
		"Error returned by DB.UnassignGithubInstallationOrganization": {
			DB: &DBMock{
				UnassignGithubInstallationOrganizationFunc: func(context.Context, string) error {
					return errors.New("boom")
				},
			},
			RespCode:   http.StatusInternalServerError,
			Fetched:    1,
			Unassigned: 1,
		},
		"Successful disconnection": {
			DB:         &DBMock{},
			RespCode:   http.StatusNoContent,
			Fetched:    1,
			Unassigned: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, true, c.DB))

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
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)
			assert.Len(t, c.DB.UnassignGithubInstallationOrganizationCalls(), c.Unassigned)
		})
	}
}

// installationClientCase is a case for an endpoint that needs a GitHub
// installation client.
type installationClientCase struct {
	Configured   bool
	DB           *DBMock
	NoSession    bool
	NoRepository bool
	RespCode     int
	Fetched      int
}

// installationClientCases are the manager-level failure paths shared by
// every endpoint that needs a GitHub installation client. The success
// path requires a live GitHub API and is intentionally not covered here.
func installationClientCases() map[string]installationClientCase {
	return map[string]installationClientCase{
		"Github app not configured": {
			DB:       &DBMock{},
			RespCode: http.StatusConflict,
		},
		"Installation not found": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusNotFound,
			Fetched:  1,
		},
		"Error returned by DB.FetchGithubInstallationByOrganizationID": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
			Fetched:  1,
		},
	}
}

func Test_Handler_FetchRepositories(t *testing.T) {
	t.Parallel()

	cc := map[string]installationClientCase{
		"No session in context": {
			Configured: true,
			DB:         &DBMock{},
			NoSession:  true,
			RespCode:   http.StatusUnauthorized,
		},
	}

	maps.Copy(cc, installationClientCases())

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, c.Configured, c.DB))

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(auth.AddSessionToContext(req.Context(), auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				}))
			}

			rec := httptest.NewRecorder()

			hdl.FetchRepositories(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)
		})
	}
}

func Test_Handler_FetchRepositoryBranches(t *testing.T) {
	t.Parallel()

	cc := map[string]installationClientCase{
		"Missing repository parameter": {
			Configured:   true,
			DB:           &DBMock{},
			NoRepository: true,
			RespCode:     http.StatusNotFound,
		},
	}

	maps.Copy(cc, installationClientCases())

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, c.Configured, c.DB))

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			ctx := req.Context()

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			if !c.NoRepository {
				ctx = testutil.AddChiCtx(ctx, "repository", "repo1")
			}

			rec := httptest.NewRecorder()

			hdl.FetchRepositoryBranches(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)
		})
	}
}

func Test_Handler_FetchRepositoryTree(t *testing.T) {
	t.Parallel()

	cc := map[string]installationClientCase{
		"Missing repository parameter": {
			Configured:   true,
			DB:           &DBMock{},
			NoRepository: true,
			RespCode:     http.StatusNotFound,
		},
	}

	maps.Copy(cc, installationClientCases())

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := NewHandler(slog.New(slog.DiscardHandler), c.DB, newTestManager(t, c.Configured, c.DB))

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?branch=main", http.NoBody)
			ctx := req.Context()

			if !c.NoSession {
				ctx = auth.AddSessionToContext(ctx, auth.Session{
					UserID:               "u1",
					ActiveOrganizationID: "org1",
				})
			}

			if !c.NoRepository {
				ctx = testutil.AddChiCtx(ctx, "repository", "repo1")
			}

			rec := httptest.NewRecorder()

			hdl.FetchRepositoryTree(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.Fetched)
		})
	}
}
