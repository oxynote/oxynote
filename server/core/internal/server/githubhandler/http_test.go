package githubhandler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
)

// newTestManager creates a githubapp.Manager for tests. When configured is
// true, a throwaway RSA key is generated so the manager builds a real app
// client.
func newTestManager(t *testing.T, configured bool) *githubapp.Manager {
	t.Helper()

	opt := githubapp.Options{}

	if configured {
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

	man, err := githubapp.NewManager(nil, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return man
}

func newTestHandler(t *testing.T, configured bool) *Handler {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(log, nil, nil, newTestManager(t, configured))
}

func Test_Handler_CheckInstallation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/github", nil)
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

			req := httptest.NewRequest(http.MethodGet, "/api/github/install", nil)
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

	req := httptest.NewRequest(http.MethodPost, "/api/x/github/events", nil)
	rec := httptest.NewRecorder()

	h.VerifySignature(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
