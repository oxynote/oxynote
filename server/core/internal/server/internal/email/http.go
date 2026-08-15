// Package email handles internal email-sending requests.
package email

import (
	"log/slog"
	"net/http"

	emailCore "github.com/oxynote/oxynote/server/core/internal/email"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// ErrInvalidTemplate is returned when the requested email template is
// not recognized.
var ErrInvalidTemplate = errutil.New(http.StatusBadRequest, "email.invalid_template", "Invalid email template.")

// Handler holds dependencies required for email operations.
type Handler struct {
	log    *slog.Logger
	sender Sender
}

// NewHandler creates a new email handling instance.
func NewHandler(
	log *slog.Logger,
	sender Sender,
) *Handler {
	return &Handler{
		log:    log.With("component", "email-handler"),
		sender: sender,
	}
}

// SendEmail sends an email rendered from the requested template.
func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template emailCore.Template `json:"template"`
		Data     struct {
			Email        string `json:"email"`
			Organization string `json:"organization"`
			Link         string `json:"link"`
		} `json:"data"`
	}

	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	switch req.Template {
	case emailCore.TemplateEmailVerification:
		h.sender.SendEmailVerification(req.Data.Email, req.Data.Link)
	case emailCore.TemplatePasswordReset:
		h.sender.SendPasswordReset(req.Data.Email, req.Data.Link)
	case emailCore.TemplateAccountExists:
		h.sender.SendAccountExists(req.Data.Email, req.Data.Link)
	case emailCore.TemplateSignupVerification:
		h.sender.SendSignupVerification(req.Data.Email, req.Data.Link)
	case emailCore.TemplateOrganizationInvitation:
		h.sender.SendOrganizationInvitation(req.Data.Email, req.Data.Organization, req.Data.Link)
	case emailCore.TemplateUserDeletion:
		h.sender.SendUserDeletionConfirmation(req.Data.Email, req.Data.Link)
	case emailCore.TemplateUserCreation:
		h.sender.SendUserCreation(req.Data.Email)
	default:
		httpserver.RespondError(h.log, w, ErrInvalidTemplate)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// Sender is an interface that handles email sending.
//
//go:generate ../../../../scripts/codegen/mock -t internal Sender
type Sender interface {
	// SendEmailVerification should send a new-email-address
	// verification email.
	SendEmailVerification(eml, link string)

	// SendPasswordReset should send a password reset email.
	SendPasswordReset(eml, link string)

	// SendAccountExists should send an account-exists notification
	// email.
	SendAccountExists(eml, link string)

	// SendSignupVerification should send the account-activation email
	// for a fresh signup.
	SendSignupVerification(eml, link string)

	// SendOrganizationInvitation should send an organization invitation
	// email.
	SendOrganizationInvitation(eml, org, link string)

	// SendUserDeletionConfirmation should send a user deletion
	// confirmation email.
	SendUserDeletionConfirmation(eml, link string)

	// SendUserCreation should send a welcome email to a newly
	// registered user.
	SendUserCreation(eml string)
}
