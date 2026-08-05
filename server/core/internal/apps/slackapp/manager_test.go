package slackapp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/notification"
)

// fakeReceiver is a no-op notification receiver for tests.
type fakeReceiver struct{}

func (fakeReceiver) OnNotification(_ func(context.Context, notification.Notification)) notification.Unsubscribe {
	return func() {}
}

// newDisabledManager creates an unconfigured Manager for tests.
func newDisabledManager(t *testing.T) *Manager {
	t.Helper()

	man, err := NewManager(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		nil,
		nil,
		Options{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return man
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Opt            Options
		WantErr        bool
		WantConfigured bool
	}{
		"Empty client ID creates an unconfigured manager": {
			Opt:            Options{},
			WantErr:        false,
			WantConfigured: false,
		},
		"Client ID with missing client secret fails": {
			Opt: Options{
				ClientID: "id",
			},
			WantErr: true,
		},
		"Full options create a configured manager": {
			Opt: Options{
				ClientID:                  "id",
				ClientSecret:              "secret",
				SignatureSecret:           "sig",
				RedirectURL:               "http://localhost/slack",
				InstallationSigningSecret: "signing",
			},
			WantErr:        false,
			WantConfigured: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man, err := NewManager(
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				nil,
				nil,
				fakeReceiver{},
				tc.Opt,
			)

			if tc.WantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := man.Configured(); got != tc.WantConfigured {
				t.Fatalf("Configured() = %v, want %v", got, tc.WantConfigured)
			}
		})
	}
}

func Test_Manager_ExchangeCode(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).ExchangeCode(context.Background(), "code")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_GetClient(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).GetClient(context.Background(), "team")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
