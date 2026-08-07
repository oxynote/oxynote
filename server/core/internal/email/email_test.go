package email

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSender(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Cfg         Config
		WantErr     bool
		WantLogOnly bool
	}{
		"Empty host disables sending": {
			Cfg: Config{
				FromAddress: "Oxynote <team@oxynote.io>",
			},
			WantLogOnly: true,
		},
		"Valid plaintext config": {
			Cfg: Config{
				Host:        "localhost",
				Port:        "1025",
				TLS:         TLSModeNone,
				FromAddress: "Oxynote <team@oxynote.io>",
			},
		},
		"Valid starttls config with auth": {
			Cfg: Config{
				Host:        "smtp.example.com",
				Port:        "587",
				Username:    "user",
				Password:    "pass",
				TLS:         TLSModeStartTLS,
				FromAddress: "team@oxynote.io",
			},
		},
		"Valid implicit tls config": {
			Cfg: Config{
				Host:        "smtp.example.com",
				Port:        "465",
				TLS:         TLSModeTLS,
				FromAddress: "team@oxynote.io",
			},
		},
		"Invalid tls mode": {
			Cfg: Config{
				Host: "localhost",
				Port: "1025",
				TLS:  "ssl3",
			},
			WantErr: true,
		},
		"Invalid port": {
			Cfg: Config{
				Host: "localhost",
				Port: "not-a-port",
				TLS:  TLSModeNone,
			},
			WantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sender, err := NewSender(slog.Default(), tc.Cfg)

			if tc.WantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tc.WantLogOnly {
				assert.Nil(t, sender.client, "expected a log-only sender without a client")
			} else {
				assert.NotNil(t, sender.client, "expected a sender with a configured client")
			}
		})
	}
}

func Test_Sender_send(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Cfg Config
	}{
		"Log-only sender skips sending": {
			Cfg: Config{
				FromAddress: "Oxynote <team@oxynote.io>",
			},
		},
		"Unreachable host logs and returns": {
			Cfg: Config{
				// nothing listens on port 1, so the dial inside send
				// fails fast with connection refused and the failure
				// is suppressed and logged.
				Host:        "127.0.0.1",
				Port:        "1",
				TLS:         TLSModeNone,
				FromAddress: "Oxynote <team@oxynote.io>",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sender, err := NewSender(slog.Default(), tc.Cfg)
			require.NoError(t, err)

			// must not panic or block indefinitely; errors are
			// suppressed and logged by design.
			sender.SendEmailVerification("test@example.com", "https://example.com/verify")
		})
	}
}
