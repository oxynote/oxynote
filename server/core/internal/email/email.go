// Package email provides functionality to send emails to the outside world.
package email

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/oxynote/heimdall/internal/buildinfo"
)

// All available email templates.
var (
	_mailgunTemplateOrganizationInvitationKey = buildinfo.Getenv("EMAIL_MAILGUN_TEMPLATE_ORGANIZATION_INVITATION_KEY")
	_mailgunTemplateEmailVerificationKey      = buildinfo.Getenv("EMAIL_MAILGUN_TEMPLATE_EMAIL_VERIFICATION_KEY")
	_mailgunTemplateUserDeletionKey           = buildinfo.Getenv("EMAIL_MAILGUN_TEMPLATE_USER_DELETION_KEY")
	_mailgunTemplateUserCreationKey           = buildinfo.Getenv("EMAIL_MAILGUN_TEMPLATE_USER_CREATION_KEY")
)

// _sendTimeout is the maximum allowed time for sending an email.
const _sendTimeout = 30 * time.Second

// Sender holds dependencies required for email sending.
type Sender struct {
	log    *slog.Logger
	client *mailgun.Client

	domain    string
	fromEmail string
}

// NewSender creates a fresh instance of email sender.
func NewSender(
	log *slog.Logger,
	domain, apiKey string,
) *Sender {
	return &Sender{
		log:       log,
		client:    mailgun.NewMailgun(apiKey),
		domain:    domain,
		fromEmail: buildinfo.Getenv("EMAIL_FROM_ADDRESS"),
	}
}

// send prepares an email from the specified templates and sends it
// to the destination address.
// All errors will be suppressed and logged.
func (s *Sender) send(toEmail, subject string, tmpl template, args map[string]string) {
	var templateKey string

	switch tmpl {
	case _templateOrganizationInvitation:
		templateKey = _mailgunTemplateOrganizationInvitationKey
	case _templateEmailVerification:
		templateKey = _mailgunTemplateEmailVerificationKey
	case _templateUserDeletion:
		templateKey = _mailgunTemplateUserDeletionKey
	case _templateUserCreation:
		templateKey = _mailgunTemplateUserCreationKey
	default:
		s.log.Error("unknown email template", slog.String("template", string(tmpl)))
		return
	}

	msg := mailgun.NewMessage(
		s.domain,
		s.fromEmail,
		subject,
		"",
	)

	for k, v := range args {
		err := msg.AddVariable(k, v)
		if err != nil {
			s.log.Error("cannot add email variable", slog.String("key", k), slog.String("value", v), slog.String("error", err.Error()))
			return
		}
	}

	msg.SetTemplate(templateKey)
	msg.AddRecipient(toEmail)

	ctx, cancel := context.WithTimeout(context.Background(), _sendTimeout)
	defer cancel()

	resp, err := s.client.Send(ctx, msg)
	if err != nil {
		s.log.Error(
			"cannot send an email",
			slog.String("from", s.fromEmail),
			slog.String("domain", s.domain),
			slog.String("to", toEmail),
			slog.String("subject", subject),
			slog.String("template", templateKey),
			slog.String("error", err.Error()),
		)
		return
	}

	s.log.Debug(
		"email sent",
		slog.String("to", toEmail),
		slog.String("subject", subject),
		slog.String("id", resp.ID),
		slog.String("message", resp.Message),
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
