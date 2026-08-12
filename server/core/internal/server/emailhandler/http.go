package emailhandler

import (
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/email"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// Handler holds dependencies required for email operations.
type Handler struct {
	log    *slog.Logger
	sender *email.Sender
}

// NewHandler creates a new email handling instance.
func NewHandler(
	log *slog.Logger,
	sender *email.Sender,
) *Handler {
	return &Handler{
		log:    log.With("component", "email-handler"),
		sender: sender,
	}
}

// SendEmailVerification sends an email verification email.
func (h *Handler) SendEmailVerification(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendEmailVerification(data.Email, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendOrganizationInvitation sends an organization invitation email.
func (h *Handler) SendOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email        string `json:"email"`
		Organization string `json:"organization"`
		Link         string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendOrganizationInvitation(data.Email, data.Organization, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendUserDeletionConfirmation sends a user deletion confirmation email.
func (h *Handler) SendUserDeletionConfirmation(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendUserDeletionConfirmation(data.Email, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendPasswordReset sends a password reset email.
func (h *Handler) SendPasswordReset(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendPasswordReset(data.Email, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendSignupVerification sends the account-activation email for a
// fresh signup.
func (h *Handler) SendSignupVerification(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendSignupVerification(data.Email, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendAccountExists sends an account-exists notification email.
func (h *Handler) SendAccountExists(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendAccountExists(data.Email, data.Link)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// SendUserCreation sends a welcome email to a newly registered user.
func (h *Handler) SendUserCreation(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email string `json:"email"`
	}

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.sender.SendUserCreation(data.Email)

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}
