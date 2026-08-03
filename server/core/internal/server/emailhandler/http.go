package emailhandler

import (
	"log/slog"
	"net/http"

	"github.com/oxynote/heimdall/internal/email"
	"github.com/oxynote/purse/http/httpserver"
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
