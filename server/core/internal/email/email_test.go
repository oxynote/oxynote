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
		"Error returned by invalid port": {
			Cfg: Config{
				Host: "localhost",
				Port: "not-a-port",
				TLS:  TLSModeNone,
			},
			Err: assert.AnError,
		},
		"Error returned by mail.NewClient": {
			Cfg: Config{
				Host: "localhost",
				// out of the valid port range, so the client option
				// fails inside mail.NewClient rather than in Atoi.
				Port: "99999",
				TLS:  TLSModeNone,
			},
			Err: assert.AnError,
		},
		"Error returned by invalid tls mode": {
			Cfg: Config{
				Host: "localhost",
				Port: "1025",
				TLS:  "ssl3",
			},
			Err: assert.AnError,
		},
		"Successful creation without a host": {
			Cfg: Config{
				FromAddress: "Oxynote <team@oxynote.io>",
			},
			NilClient: true,
		},
		"Successful creation with plaintext config": {
			Cfg: Config{
				Host:        "localhost",
				Port:        "1025",
				TLS:         TLSModeNone,
				FromAddress: "Oxynote <team@oxynote.io>",
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
			},
		},
		"Successful creation with implicit tls config": {
			Cfg: Config{
				Host:        "smtp.example.com",
				Port:        "465",
				TLS:         TLSModeTLS,
				FromAddress: "team@oxynote.io",
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

func Test_Sender_SendEmailVerification(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendEmailVerification("user@example.com", "https://example.com/verify")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Verify your new email address"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.Contains(t, msgBody(t, msg), "https://example.com/verify")
}

func Test_Sender_SendOrganizationInvitation(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendOrganizationInvitation("user@example.com", "Acme", "https://example.com/join")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Join Acme on Oxynote"}, msg.GetGenHeader(mail.HeaderSubject))

	body := msgBody(t, msg)
	assert.Contains(t, body, "https://example.com/join")
	assert.Contains(t, body, "Acme")
}

func Test_Sender_SendUserDeletionConfirmation(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendUserDeletionConfirmation("user@example.com", "https://example.com/delete")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Confirm your account deletion"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.Contains(t, msgBody(t, msg), "https://example.com/delete")
}

func Test_Sender_SendUserCreation(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendUserCreation("user@example.com")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Welcome to Oxynote"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.NotEmpty(t, msgBody(t, msg))
}

func Test_Sender_SendPasswordReset(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendPasswordReset("user@example.com", "https://example.com/reset")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Reset your password"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.Contains(t, msgBody(t, msg), "https://example.com/reset")
}

func Test_Sender_SendSignupVerification(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendSignupVerification("user@example.com", "https://example.com/activate")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"Confirm your email address"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.Contains(t, msgBody(t, msg), "https://example.com/activate")
}

func Test_Sender_SendAccountExists(t *testing.T) {
	t.Parallel()

	client := &clientMock{}
	s := stubSender(client)

	s.SendAccountExists("user@example.com", "https://example.com/login")

	s.supv.Wait()

	msg := sentMsg(t, client)
	assert.Equal(t, "user@example.com", msgTo(t, msg))
	assert.Equal(t, []string{"You already have an Oxynote account"}, msg.GetGenHeader(mail.HeaderSubject))
	assert.Contains(t, msgBody(t, msg), "https://example.com/login")
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

	s.SendPasswordReset("user@example.com", "https://example.com/reset")

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
