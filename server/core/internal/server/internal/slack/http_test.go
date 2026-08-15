package slack

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/apps/slackapp"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
)

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
	opt := slackapp.Options{}

	if configured {
		opt = slackapp.Options{
			ClientID:                  "id",
			ClientSecret:              "secret",
			SignatureSecret:           "sig",
			RedirectURL:               "http://localhost/slack",
			InstallationSigningSecret: "signing",
		}
	}

	man, err := slackapp.NewManager(log, nil, nil, fakeReceiver{}, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewHandler(log, nil, nil, man)
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

	req := httptest.NewRequest(http.MethodPost, "/api/x/slack/events", http.NoBody)
	rec := httptest.NewRecorder()

	h.VerifySignature(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func Test_Handler_InstallApp(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/x/slack/install?code=abc", http.NoBody)
	rec := httptest.NewRecorder()

	h.InstallApp(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
