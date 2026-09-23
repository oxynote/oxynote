package email

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewSender(t *testing.T) {
	cc := map[string]struct {
		Cfg       Config
		NilClient bool
		Err       error
	}{
		"Error returned by invalid public url": {
			Cfg: Config{
				PublicURL: "::not-a-url",
			},
			Err: assert.AnError,
		},
		"Error returned by public url without a host": {
			Cfg: Config{
				PublicURL: "/core",
			},
			Err: assert.AnError,
		},
		"Error returned by invalid port": {
			Cfg: Config{
				Host:      "localhost",
				Port:      "not-a-port",
				TLS:       TLSModeNone,
				PublicURL: "https://notes.example.com/core",
			},
			Err: assert.AnError,
		},
		"Error returned by mail.NewClient": {
			Cfg: Config{
				Host: "localhost",
				// out of the valid port range, so the client option
				// fails inside mail.NewClient rather than in Atoi.
				Port:      "99999",
				TLS:       TLSModeNone,
				PublicURL: "https://notes.example.com/core",
			},
			Err: assert.AnError,
		},
		"Error returned by invalid tls mode": {
			Cfg: Config{
				Host:      "localhost",
				Port:      "1025",
				TLS:       "ssl3",
				PublicURL: "https://notes.example.com/core",
			},
			Err: assert.AnError,
		},
		"Successful creation without a host": {
			Cfg: Config{
				FromAddress: "Oxynote <team@oxynote.io>",
				PublicURL:   "https://notes.example.com/core",
			},
			NilClient: true,
		},
		"Successful creation with plaintext config": {
			Cfg: Config{
				Host:        "localhost",
				Port:        "1025",
				TLS:         TLSModeNone,
				FromAddress: "Oxynote <team@oxynote.io>",
				PublicURL:   "https://notes.example.com/core",
			},
		},
		"Successful creation with starttls config and auth": {
			Cfg: Config{
				Host:        "smtp.example.com",
				Port:        "587",
				Username:    "user",
				Password:    "pass",
				TLS:         TLSModeStartTLS,
				FromAddress: "team@oxynote.io",
				PublicURL:   "https://notes.example.com/core",
			},
		},
		"Successful creation with implicit tls config": {
			Cfg: Config{
				Host:        "smtp.example.com",
				Port:        "465",
				TLS:         TLSModeTLS,
				FromAddress: "team@oxynote.io",
				PublicURL:   "https://notes.example.com/core",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			log := slog.New(slog.DiscardHandler)

			sender, err := NewSender(log, c.Cfg)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, sender)
			assert.Equal(t, log, sender.log)
			assert.NotNil(t, sender.backoffStrategy)
			assert.Equal(t, c.Cfg.FromAddress, sender.fromEmail)
			assert.Equal(t, "notes.example.com", sender.publicHost)

			if c.NilClient {
				assert.Nil(t, sender.client)
			} else {
				assert.NotNil(t, sender.client)
			}
		})
	}
}

func Test_Sender_send(t *testing.T) {
	cc := map[string]struct {
		Client      *clientMock
		From        string
		To          string
		Template    Template
		Args        map[string]string
		WantCalls   int
		LogContains string
	}{
		"Sending disabled logs the email": {
			To:          "user@example.com",
			Template:    TemplatePasswordReset,
			Args:        map[string]string{"link": "https://example.com/reset"},
			LogContains: "email sending is not configured",
		},
		"Error returned by render": {
			Client:      &clientMock{},
			From:        "team@oxynote.io",
			To:          "user@example.com",
			Template:    Template("nonexistent"),
			LogContains: "cannot render email template",
		},
		"Invalid from address": {
			Client:      &clientMock{},
			To:          "user@example.com",
			Template:    TemplatePasswordReset,
			Args:        map[string]string{"link": "https://example.com/reset"},
			LogContains: "cannot set email from address",
		},
		"Invalid recipient address": {
			Client:      &clientMock{},
			From:        "team@oxynote.io",
			Template:    TemplatePasswordReset,
			Args:        map[string]string{"link": "https://example.com/reset"},
			LogContains: "cannot set email recipient",
		},
		"Error returned by client.DialAndSendWithContext": {
			Client: &clientMock{
				DialAndSendWithContextFunc: func(_ context.Context, _ ...*mail.Msg) error {
					return assert.AnError
				},
			},
			From:     "team@oxynote.io",
			To:       "user@example.com",
			Template: TemplatePasswordReset,
			Args:     map[string]string{"link": "https://example.com/reset"},
			// a lost password reset locks a user out, so the delivery is
			// retried before it is given up on.
			WantCalls:   1 + _maxSendRetries,
			LogContains: "cannot send an email",
		},
		"Successful send": {
			Client:      &clientMock{},
			From:        "team@oxynote.io",
			To:          "user@example.com",
			Template:    TemplatePasswordReset,
			Args:        map[string]string{"link": "https://example.com/reset"},
			WantCalls:   1,
			LogContains: "email sent",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			s := &Sender{
				log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				})),
				backoffStrategy: func() backoff.BackOff { return &backoff.ZeroBackOff{} },
				supv:            xync.NewSupervisor(),
				fromEmail:       c.From,
			}

			// a nil *clientMock must stay a nil interface so the
			// log-only branch triggers.
			if c.Client != nil {
				s.client = c.Client
			}

			s.send(c.To, "subject", c.Template, c.Args)

			// the delivery runs off the request path; Wait drains it without
			// the cancellation Close would bring.
			s.supv.Wait()

			assert.Contains(t, buf.String(), c.LogContains)

			if c.Client == nil {
				return
			}

			ff := c.Client.DialAndSendWithContextCalls()
			require.Len(t, ff, c.WantCalls)

			if c.WantCalls == 0 {
				return
			}

			assert.NotNil(t, ff[0].Ctx)
			require.Len(t, ff[0].Msgs, 1)
		})
	}
}

func Test_Sender_Send(t *testing.T) {
	cc := map[string]struct {
		Template Template
		Data     Data
		Subject  string
		Contains []string
		Err      error
	}{
		"Unknown template": {
			Template: Template("nonexistent"),
			Data:     Data{Email: "user@example.com"},
			Err:      ErrInvalidTemplate,
		},
		"Email verification": {
			Template: TemplateEmailVerification,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/verify"},
			Subject:  "Verify your new email address",
			Contains: []string{"https://example.com/verify"},
		},
		"Email change confirmation": {
			Template: TemplateEmailChangeConfirmation,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/approve"},
			Subject:  "Approve your email address change",
			Contains: []string{"https://example.com/approve"},
		},
		"Organization invitation": {
			Template: TemplateOrganizationInvitation,
			Data:     Data{Email: "user@example.com", Organization: "Acme", Link: "https://example.com/join"},
			Subject:  "You're invited to the Acme workspace",
			Contains: []string{"https://example.com/join", "Acme", "Sent by Oxynote at notes.example.com"},
		},
		"User deletion confirmation": {
			Template: TemplateUserDeletion,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/delete"},
			Subject:  "Confirm your account deletion",
			Contains: []string{"https://example.com/delete"},
		},
		"Password reset": {
			Template: TemplatePasswordReset,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/reset"},
			Subject:  "Reset your password",
			Contains: []string{"https://example.com/reset"},
		},
		"Signup verification": {
			Template: TemplateSignupVerification,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/activate"},
			Subject:  "Confirm your email address",
			Contains: []string{"https://example.com/activate"},
		},
		"Account exists": {
			Template: TemplateAccountExists,
			Data:     Data{Email: "user@example.com", Link: "https://example.com/login"},
			Subject:  "You already have an Oxynote account",
			Contains: []string{"https://example.com/login"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := &clientMock{}
			s := stubSender(client)

			err := s.Send(c.Template, c.Data)

			s.supv.Wait()

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, client.DialAndSendWithContextCalls())

				return
			}

			msg := sentMsg(t, client)
			assert.Equal(t, c.Data.Email, msgTo(t, msg))
			assert.Equal(t, []string{c.Subject}, msg.GetGenHeader(mail.HeaderSubject))

			body := msgBody(t, msg)

			for _, sub := range c.Contains {
				assert.Contains(t, body, sub)
			}
		})
	}
}

func Test_Sender_Close(t *testing.T) {
	t.Parallel()

	var ctxErr error

	client := &clientMock{
		DialAndSendWithContextFunc: func(ctx context.Context, _ ...*mail.Msg) error {
			ctxErr = ctx.Err()

			return nil
		},
	}
	s := stubSender(client)

	require.NoError(t, s.Send(TemplatePasswordReset, Data{Email: "user@example.com", Link: "https://example.com/reset"}))
	require.NoError(t, s.Close())

	// Close must drain the in-flight delivery before cancelling the
	// supervisor context, so the send observed a live context.
	require.Len(t, client.DialAndSendWithContextCalls(), 1)
	assert.NoError(t, ctxErr)
}

// stubSender creates a sender backed by the provided mocked client.
func stubSender(client *clientMock) *Sender {
	return &Sender{
		log:             slog.New(slog.DiscardHandler),
		client:          client,
		backoffStrategy: func() backoff.BackOff { return &backoff.ZeroBackOff{} },
		supv:            xync.NewSupervisor(),
		fromEmail:       "Oxynote <team@oxynote.io>",
		publicHost:      "notes.example.com",
	}
}

// sentMsg extracts the single message passed to the mocked client.
func sentMsg(t *testing.T, client *clientMock) *mail.Msg {
	t.Helper()

	ff := client.DialAndSendWithContextCalls()
	require.Len(t, ff, 1)
	require.Len(t, ff[0].Msgs, 1)

	return ff[0].Msgs[0]
}

// msgTo extracts the single recipient address of the message.
func msgTo(t *testing.T, msg *mail.Msg) string {
	t.Helper()

	addrs := msg.GetTo()
	require.Len(t, addrs, 1)

	return addrs[0].Address
}

// msgBody extracts the rendered HTML body of the message before any
// transfer encoding is applied.
func msgBody(t *testing.T, msg *mail.Msg) string {
	t.Helper()

	parts := msg.GetParts()
	require.NotEmpty(t, parts)

	body, err := parts[0].GetContent()
	require.NoError(t, err)

	return string(body)
}
