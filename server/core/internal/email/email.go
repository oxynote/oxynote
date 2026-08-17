// Package email provides functionality to send emails to the outside world.
package email

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/wneessen/go-mail"
)

// TLSMode describes how the SMTP connection is secured.
type TLSMode string

// Available TLS modes for the SMTP connection.
const (
	// TLSModeNone connects in plaintext without any TLS.
	TLSModeNone TLSMode = "none"

	// TLSModeStartTLS connects in plaintext and requires an upgrade to
	// TLS via STARTTLS.
	TLSModeStartTLS TLSMode = "starttls"

	// TLSModeTLS connects over implicit TLS.
	TLSModeTLS TLSMode = "tls"
)

// _sendTimeout is the maximum allowed time for sending an email.
const _sendTimeout = 30 * time.Second

// _linkKey is the template argument carrying the action URL.
const _linkKey = "link"

// Config holds SMTP connection settings for the email sender.
type Config struct {
	// Host is the SMTP server host. When empty, email sending is
	// disabled and every email is logged instead.
	Host string

	// Port is the SMTP server port.
	Port string

	// Username is the SMTP auth username. Empty together with Password
	// means no authentication.
	Username string

	// Password is the SMTP auth password.
	Password string

	// TLS is the TLS mode of the SMTP connection. One of "none",
	// "starttls" or "tls".
	TLS TLSMode

	// FromAddress is the address emails are sent from, in
	// "Name <address>" or plain "address" form.
	FromAddress string
}

// _maxSendRetries caps the retries of one delivery.
const _maxSendRetries = 3

// _newBackoff creates the delivery retry strategy. Variable so tests can
// substitute a faster one.
var _newBackoff = func() backoff.BackOff {
	return backoff.NewExponentialBackOff()
}

// Sender holds dependencies required for email sending.
type Sender struct {
	log    *slog.Logger
	client client

	supv      *xync.Supervisor
	fromEmail string
}

// NewSender creates a fresh instance of email sender.
// An empty cfg.Host yields a sender with sending disabled: every email
// is logged instead of sent.
func NewSender(log *slog.Logger, cfg Config) (*Sender, error) {
	sender := &Sender{
		log:       log,
		supv:      xync.NewSupervisor(),
		fromEmail: cfg.FromAddress,
	}

	if cfg.Host == "" {
		return sender, nil
	}

	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid smtp port %q: %w", cfg.Port, err)
	}

	opts := []mail.Option{
		mail.WithPort(port),
	}

	switch cfg.TLS {
	case TLSModeNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	case TLSModeStartTLS:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	case TLSModeTLS:
		opts = append(opts, mail.WithSSL())
	default:
		return nil, fmt.Errorf("invalid smtp tls mode %q", cfg.TLS)
	}

	if cfg.Username != "" || cfg.Password != "" {
		opts = append(
			opts,
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(cfg.Username),
			mail.WithPassword(cfg.Password),
		)
	}

	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}

	sender.client = client

	return sender, nil
}

// send prepares an email from the specified template and sends it
// to the destination address.
// All errors will be suppressed and logged.
func (s *Sender) send(toEmail, subject string, tmpl Template, args map[string]string) {
	if s.client == nil {
		s.log.Info(
			"email sending is not configured, logging email instead",
			slog.String("to", toEmail),
			slog.String("subject", subject),
			slog.String("template", string(tmpl)),
			slog.Any("args", args),
		)

		return
	}

	body, err := render(tmpl, args)
	if err != nil {
		s.log.Error(
			"cannot render email template",
			slog.String("template", string(tmpl)),
			slog.String("error", err.Error()),
		)

		return
	}

	msg := mail.NewMsg()

	err = msg.From(s.fromEmail)
	if err != nil {
		s.log.Error(
			"cannot set email from address",
			slog.String("from", s.fromEmail),
			slog.String("error", err.Error()),
		)

		return
	}

	err = msg.To(toEmail)
	if err != nil {
		s.log.Error(
			"cannot set email recipient",
			slog.String("to", toEmail),
			slog.String("error", err.Error()),
		)

		return
	}

	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, body)

	err = msg.EmbedReader("logo.png", bytes.NewReader(_logoPNG), mail.WithFileContentID("logo.png"))
	if err != nil {
		// NOCOV: reading from an in-memory bytes.Reader cannot fail.
		s.log.Error(
			"cannot embed email logo",
			slog.String("error", err.Error()),
		)

		return
	}

	// the caller is an HTTP handler answering auth-realtime, so the delivery
	// runs on its own: a hanging SMTP server would otherwise stall a signup
	// or a password reset for the full send timeout.
	s.supv.Go(func(ctx context.Context) {
		s.deliver(ctx, msg, toEmail, subject, tmpl)
	})
}

// deliver dials the SMTP server and sends the message, retrying a transient
// failure. A password reset lost to a blip is a user who cannot get back in,
// so the last failure is reported rather than merely logged.
func (s *Sender) deliver(ctx context.Context, msg *mail.Msg, toEmail, subject string, tmpl Template) {
	ctx, cancel := context.WithTimeout(ctx, _sendTimeout)
	defer cancel()

	err := backoff.Retry(
		func() error {
			return s.client.DialAndSendWithContext(ctx, msg)
		},
		backoff.WithMaxRetries(
			backoff.WithContext(_newBackoff(), ctx),
			_maxSendRetries,
		),
	)
	if err != nil {
		logutil.Critical(s.log, err).Error(
			"cannot send an email",
			slog.String("from", s.fromEmail),
			slog.String("to", toEmail),
			slog.String("subject", subject),
			slog.String("template", string(tmpl)),
			slog.String("error", err.Error()),
		)

		return
	}

	s.log.Debug(
		"email sent",
		slog.String("to", toEmail),
		slog.String("subject", subject),
	)
}

// SendEmailVerification sends an email regarding new email
// verification with the token, embedded into a full URL, to the
// specified email address.
func (s *Sender) SendEmailVerification(eml, link string) {
	s.send(
		eml,
		"Verify your new email address",
		TemplateEmailVerification,
		map[string]string{
			_linkKey: link,
		},
	)
}

// SendOrganizationInvitation sends an email regarding
// organization invitation with the token, embedded into a full URL, to the
// specified email address.
func (s *Sender) SendOrganizationInvitation(eml, org, link string) {
	s.send(
		eml,
		fmt.Sprintf("Join %s on Oxynote", org),
		TemplateOrganizationInvitation,
		map[string]string{
			_linkKey:       link,
			"organization": org,
		},
	)
}

// SendUserDeletionConfirmation sends an email to confirm account deletion
// with a verification link.
func (s *Sender) SendUserDeletionConfirmation(eml, link string) {
	s.send(
		eml,
		"Confirm your account deletion",
		TemplateUserDeletion,
		map[string]string{
			_linkKey: link,
		},
	)
}

// SendUserCreation sends a welcome email to a newly registered user.
func (s *Sender) SendUserCreation(eml string) {
	s.send(
		eml,
		"Welcome to Oxynote",
		TemplateUserCreation,
		nil,
	)
}

// SendPasswordReset sends an email with a password reset link to the
// specified email address.
func (s *Sender) SendPasswordReset(eml, link string) {
	s.send(
		eml,
		"Reset your password",
		TemplatePasswordReset,
		map[string]string{
			_linkKey: link,
		},
	)
}

// SendSignupVerification sends the initial account-activation email
// with a verification link to a freshly signed-up email address.
func (s *Sender) SendSignupVerification(eml, link string) {
	s.send(
		eml,
		"Confirm your email address",
		TemplateSignupVerification,
		map[string]string{
			_linkKey: link,
		},
	)
}

// SendAccountExists notifies the owner of an existing account that a
// signup was attempted with their email address, with a login link.
func (s *Sender) SendAccountExists(eml, link string) {
	s.send(
		eml,
		"You already have an Oxynote account",
		TemplateAccountExists,
		map[string]string{
			_linkKey: link,
		},
	)
}

// client is an interface that handles communication with the SMTP
// server.
//
//go:generate ../../scripts/codegen/mock -t internal client
type client interface {
	// DialAndSendWithContext should establish a connection to the SMTP
	// server and send the provided messages.
	DialAndSendWithContext(ctx context.Context, msgs ...*mail.Msg) error
}

// Close waits for the deliveries still in flight and releases the sender.
func (s *Sender) Close() error {
	s.supv.CloseAndWait()

	return nil
}
