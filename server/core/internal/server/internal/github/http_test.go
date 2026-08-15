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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
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

// newTestManager creates a githubapp.Manager for tests. When configured is
// true, a throwaway RSA key is generated so the manager builds a real app
// client.
func newTestManager(t *testing.T, configured bool, db githubapp.DB) *githubapp.Manager {
	t.Helper()

	opt := githubapp.Options{}

	if configured {
		opt.InstallationSigningSecret = "0123456789abcdef0123456789abcdef"

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

	man, err := githubapp.NewManager(db, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return man
}

func newTestHandler(t *testing.T, configured bool) *Handler {
	t.Helper()

	log := slog.New(slog.DiscardHandler)

	return NewHandler(log, nil, nil, newTestManager(t, configured, nil))
}

// newMockedHandler builds a Handler whose DB (and the manager's DB) is
// the given mock.
func newMockedHandler(t *testing.T, configured bool, db *DBMock) *Handler {
	t.Helper()

	return NewHandler(slog.New(slog.DiscardHandler), db, nil, newTestManager(t, configured, db))
}

// addSession stores a test session on the request context.
func addSession(ctx context.Context) context.Context {
	return auth.AddSessionToContext(ctx, auth.Session{
		UserID:               "u1",
		ActiveOrganizationID: "org1",
	})
}

func Test_Handler_CheckInstallation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/github", http.NoBody)
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
		"Not configured responds with github.not_configured": {
			Configured: false,
			WantStatus: http.StatusConflict,
			WantCode:   "github.not_configured",
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

			req := httptest.NewRequest(http.MethodGet, "/api/github/install", http.NoBody)
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

	req := httptest.NewRequest(http.MethodPost, "/api/x/github/events", http.NoBody)
	rec := httptest.NewRecorder()

	h.VerifySignature(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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
		"Installation lookup error": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
		"Organization already connected": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 42, nil
				},
			},
			RespCode: http.StatusConflict,
		},
		"Github app not configured": {
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusConflict,
		},
		"Successful URL creation": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errutil.ErrNotFound
				},
			},
			RespCode: http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB)

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

// installationState builds a valid encrypted installation state for the
// given organization by round-tripping through the manager's URL builder.
func installationState(t *testing.T, man *githubapp.Manager, orgID string) string {
	t.Helper()

	rawURL, err := man.CreateInstallationURL(orgID)
	require.NoError(t, err)

	u, err := url.Parse(rawURL)
	require.NoError(t, err)

	return u.Query().Get("state")
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
		"Non-numeric installation ID": {
			DB:       &DBMock{},
			StateOrg: "org1",
			RawID:    "abc",
			RespCode: http.StatusBadRequest,
		},
		"Invalid installation state": {
			DB:       &DBMock{},
			RawState: "garbage",
			RespCode: http.StatusInternalServerError,
		},
		"State organization mismatch": {
			DB:       &DBMock{},
			StateOrg: "org2",
			RespCode: http.StatusBadRequest,
		},
		"Installation organization lookup error": {
			DB: &DBMock{
				FetchGithubInstallationOrganizationIDFunc: func(context.Context, int64) (null.String, error) {
					return null.String{}, errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
		},
		"Installation already assigned": {
			DB: &DBMock{
				FetchGithubInstallationOrganizationIDFunc: func(context.Context, int64) (null.String, error) {
					return null.StringFrom("org9"), nil
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusConflict,
		},
		"Assignment update error": {
			DB: &DBMock{
				UpdateGithubInstallationOrganizationIDFunc: func(context.Context, int64, string) error {
					return errors.New("boom")
				},
			},
			StateOrg: "org1",
			RespCode: http.StatusInternalServerError,
			Updated:  1,
		},
		"Successful connection": {
			DB:       &DBMock{},
			StateOrg: "org1",
			RespCode: http.StatusOK,
			Updated:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB)

			state := c.RawState

			switch state {
			case "":
				state = installationState(t, hdl.man, c.StateOrg)
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
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.ConnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
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
			RespCode:  http.StatusInternalServerError,
		},
		"Installation created": {
			DB:        &DBMock{},
			EventType: "installation",
			Payload:   `{"action":"created","installation":{"id":42}}`,
			RespCode:  http.StatusOK,
			Inserted:  1,
		},
		"Installation creation insert error": {
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
			RespCode:  http.StatusOK,
			Deleted:   1,
		},
		"Installation deletion error": {
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
			RespCode:  http.StatusOK,
		},
		"Ignored event type": {
			DB:        &DBMock{},
			EventType: "push",
			Payload:   `{"ref":"refs/heads/main"}`,
			RespCode:  http.StatusOK,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB)

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
		},
		"Unassignment error": {
			DB: &DBMock{
				UnassignGithubInstallationOrganizationFunc: func(context.Context, string) error {
					return errors.New("boom")
				},
			},
			RespCode:   http.StatusInternalServerError,
			Unassigned: 1,
		},
		"Successful disconnection": {
			DB:         &DBMock{},
			RespCode:   http.StatusOK,
			Unassigned: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, true, c.DB)

			req := httptest.NewRequest(http.MethodDelete, "http://test.com/", http.NoBody)

			if !c.NoSession {
				req = req.WithContext(addSession(req.Context()))
			}

			rec := httptest.NewRecorder()

			hdl.DisconnectOrganization(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Len(t, c.DB.UnassignGithubInstallationOrganizationCalls(), c.Unassigned)
		})
	}
}

// installationClientCases are the manager-level failure paths shared by
// every endpoint that needs a GitHub installation client. The success
// path requires a live GitHub API and is intentionally not covered here.
func installationClientCases() map[string]struct {
	Configured bool
	DB         *DBMock
	RespCode   int
} {
	return map[string]struct {
		Configured bool
		DB         *DBMock
		RespCode   int
	}{
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
		},
		"Installation lookup error": {
			Configured: true,
			DB: &DBMock{
				FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
					return 0, errors.New("boom")
				},
			},
			RespCode: http.StatusInternalServerError,
		},
	}
}

func Test_Handler_FetchRepositories(t *testing.T) {
	t.Parallel()

	t.Run("No session in context", func(t *testing.T) {
		t.Parallel()

		hdl := newMockedHandler(t, true, &DBMock{})

		rec := httptest.NewRecorder()

		hdl.FetchRepositories(rec, httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	for cn, c := range installationClientCases() {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			req = req.WithContext(addSession(req.Context()))

			rec := httptest.NewRecorder()

			hdl.FetchRepositories(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
		})
	}
}

func Test_Handler_FetchRepositoryBranches(t *testing.T) {
	t.Parallel()

	t.Run("Missing repository parameter", func(t *testing.T) {
		t.Parallel()

		hdl := newMockedHandler(t, true, &DBMock{})

		req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
		req = req.WithContext(addSession(req.Context()))

		rec := httptest.NewRecorder()

		hdl.FetchRepositoryBranches(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	for cn, c := range installationClientCases() {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			ctx := addSession(req.Context())
			ctx = testutil.AddChiCtx(ctx, "repository", "repo1")

			rec := httptest.NewRecorder()

			hdl.FetchRepositoryBranches(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
		})
	}
}

func Test_Handler_FetchIssues(t *testing.T) {
	t.Parallel()

	t.Run("Invalid form data", func(t *testing.T) {
		t.Parallel()

		hdl := newMockedHandler(t, true, &DBMock{})

		req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
		req.URL.RawQuery = "q=%zz"
		req = req.WithContext(addSession(req.Context()))

		rec := httptest.NewRecorder()

		hdl.FetchIssues(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	for cn, c := range installationClientCases() {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?q=bug", http.NoBody)
			req = req.WithContext(addSession(req.Context()))

			rec := httptest.NewRecorder()

			hdl.FetchIssues(rec, req)

			assert.Equal(t, c.RespCode, rec.Code)
		})
	}
}

func Test_Handler_FetchRepositoryTree(t *testing.T) {
	t.Parallel()

	t.Run("Missing repository parameter", func(t *testing.T) {
		t.Parallel()

		hdl := newMockedHandler(t, true, &DBMock{})

		req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
		req = req.WithContext(addSession(req.Context()))

		rec := httptest.NewRecorder()

		hdl.FetchRepositoryTree(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	for cn, c := range installationClientCases() {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := newMockedHandler(t, c.Configured, c.DB)

			req := httptest.NewRequest(http.MethodGet, "http://test.com/?branch=main", http.NoBody)
			ctx := addSession(req.Context())
			ctx = testutil.AddChiCtx(ctx, "repository", "repo1")

			rec := httptest.NewRecorder()

			hdl.FetchRepositoryTree(rec, req.WithContext(ctx))

			assert.Equal(t, c.RespCode, rec.Code)
		})
	}
}
