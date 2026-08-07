// Package email provides functionality to send emails to the outside world.
package email

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

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

// Sender holds dependencies required for email sending.
type Sender struct {
	log    *slog.Logger
	client *mail.Client

	fromEmail string
}

// NewSender creates a fresh instance of email sender.
// An empty cfg.Host yields a sender with sending disabled: every email
// is logged instead of sent.
func NewSender(log *slog.Logger, cfg Config) (*Sender, error) {
	sender := &Sender{
		log:       log,
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
func (s *Sender) send(toEmail, subject string, tmpl template, args map[string]string) {
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

	ctx, cancel := context.WithTimeout(context.Background(), _sendTimeout)
	defer cancel()

	err = s.client.DialAndSendWithContext(ctx, msg)
	if err != nil {
		s.log.Error(
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
		_templateEmailVerification,
		map[string]string{
			"link": link,
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
		_templateOrganizationInvitation,
		map[string]string{
			"link":         link,
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
		_templateUserDeletion,
		map[string]string{
			"link": link,
		},
	)
}

// SendUserCreation sends a welcome email to a newly registered user.
func (s *Sender) SendUserCreation(eml string) {
	s.send(
		eml,
		"Welcome to Oxynote",
		_templateUserCreation,
		nil,
	)
}
