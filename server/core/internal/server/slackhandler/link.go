package slackhandler

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/apps/slackapp"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/oxynote/purse/util/errutil"
	"github.com/slack-go/slack"
)

var (
	// ErrMissingLinkState is returned when the link state is missing from the request.
	ErrMissingLinkState = errutil.New(http.StatusBadRequest, "slack.missing_link_state", "link state is required")

	// ErrUserNotInOrganization is returned when the user is not a member of the organization.
	ErrUserNotInOrganization = errutil.New(http.StatusForbidden, "slack.user_not_in_organization", "user is not a member of the organization")

	// ErrUserIsNotLinked is returned when the user's Slack account is not linked.
	ErrUserIsNotLinked = errutil.New(http.StatusBadRequest, "slack.user_not_linked", "user's slack account is not linked to an oxynote account")

	// ErrSlackNotConnected is returned when the Slack app is not connected to an organization.
	ErrSlackNotConnected = errutil.New(http.StatusBadRequest, "slack.not_connected", "slack is not connected to an organization")
)

// HandleSlashCommand handles Slack slash commands like /link and /unlink.
func (h *Handler) HandleSlashCommand(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	command := r.FormValue("command")
	userID := r.FormValue("user_id")
	teamID := r.FormValue("team_id")
	responseURL := r.FormValue("response_url")

	switch command {
	case "/link":
		h.handleLinkCommand(w, r, userID, teamID, responseURL)
	case "/unlink":
		h.handleUnlinkCommand(w, r, userID, teamID, responseURL)
	default:
		h.sendEphemeralResponse(r.Context(), responseURL, "Unknown command.")
		w.WriteHeader(http.StatusOK)
	}
}

// handleLinkCommand handles the /link slash command.
func (h *Handler) handleLinkCommand(w http.ResponseWriter, r *http.Request, userID, teamID, responseURL string) {
	app, err := h.db.FetchSlackAppByTeamID(r.Context(), teamID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !app.OrganizationID.Valid {
		err = h.sendEphemeralResponse(
			r.Context(),
			responseURL,
			"Oxynote is not connected to this Slack workspace. Please ask an admin to connect it first.",
		)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	link, err := h.db.FetchSlackUserLink(r.Context(), userID, teamID)
	if err != nil && !errutil.IsNotFound(err) {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if link != nil {
		err = h.sendEphemeralResponse(
			r.Context(),
			responseURL,
			"Your Slack account is already linked to a Oxynote account.",
		)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	linkURL, err := h.man.CreateLinkURL(userID, teamID, app.OrganizationID.String)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.sendLinkMessage(r.Context(), responseURL, linkURL)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleUnlinkCommand handles the /unlink slash command.
func (h *Handler) handleUnlinkCommand(w http.ResponseWriter, r *http.Request, userID, teamID, responseURL string) {
	link, err := h.db.FetchSlackUserLink(r.Context(), userID, teamID)
	if err != nil {
		if errutil.IsNotFound(err) {
			h.sendEphemeralResponse(
				r.Context(),
				responseURL,
				"Your Slack account is not linked to a Oxynote account.",
			)
			w.WriteHeader(http.StatusOK)
			return
		}

		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.DeleteSlackUserLink(r.Context(), link.SlackUserID, link.TeamID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.sendEphemeralResponse(
		r.Context(),
		responseURL,
		"Your Slack account has been unlinked from your Oxynote account.",
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// FetchUserLink retrieves the user's Slack link settings.
func (h *Handler) FetchUserLink(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	link, err := h.db.FetchSlackUserLinkByUserID(r.Context(), session.UserID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, link.Settings, http.StatusOK)
}

// UpdateUserLink updates the user's Slack link settings.
func (h *Handler) UpdateUserLink(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var settings slackapp.UserLinkSettings

	if err := httpserver.DecodeJSON(r, &settings); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	link, err := h.db.FetchSlackUserLinkByUserID(r.Context(), session.UserID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	link.Settings = settings

	if err := h.db.UpdateSlackUserLink(r.Context(), *link); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, link.Settings, http.StatusOK)
}

// DeleteUserLink deletes the user's Slack link.
func (h *Handler) DeleteUserLink(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	link, err := h.db.FetchSlackUserLinkByUserID(r.Context(), session.UserID, session.ActiveOrganizationID)
	if err != nil {
		if errutil.IsNotFound(err) {
			httpserver.RespondError(h.log, w, ErrUserIsNotLinked)
			return
		}

		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.DeleteSlackUserLink(r.Context(), link.SlackUserID, link.TeamID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusOK)
}

// LinkUser handles the completion of the user linking flow.
func (h *Handler) LinkUser(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	rawState := r.URL.Query().Get("state")
	if rawState == "" {
		httpserver.RespondError(h.log, w, ErrMissingLinkState)
		return
	}

	ls, err := h.man.VerifyLinkState(rawState)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	app, err := h.db.FetchSlackAppByTeamID(r.Context(), ls.TeamID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !app.OrganizationID.Valid || app.OrganizationID.String != ls.OrganizationID {
		httpserver.RespondError(h.log, w, ErrSlackNotConnected)
		return
	}

	members, err := h.db.FetchOrganizationMembers(r.Context(), ls.OrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !slices.Contains(members, session.UserID) {
		httpserver.RespondError(h.log, w, ErrUserNotInOrganization)
		return
	}

	if err := h.db.InsertSlackUserLink(
		r.Context(),
		*slackapp.NewUserLink(ls.SlackUserID, ls.TeamID, session.UserID),
	); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.man.GetClient(r.Context(), ls.TeamID)
	if err != nil {
		h.log.Error("failed to get slack client", slog.String("error", err.Error()))
	} else {
		_, _, err = client.PostMessageContext(
			r.Context(),
			ls.SlackUserID,
			slack.MsgOptionText("Your Slack account has been successfully linked to your Oxynote account.", false),
		)
		if err != nil {
			h.log.Error("failed to send confirmation DM", slog.String("error", err.Error()))
		}
	}

	httpserver.Respond(h.log, w, map[string]bool{
		"linked": true,
	}, http.StatusOK)
}

// sendLinkMessage sends an ephemeral message with a link button.
func (h *Handler) sendLinkMessage(ctx context.Context, responseURL, linkURL string) error {
	msg := slack.WebhookMessage{
		ResponseType: "ephemeral",
		Blocks: &slack.Blocks{
			BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: "Link your Slack account to Oxynote to enable personalized features and notifications.",
					},
				},
				&slack.ActionBlock{
					Type:    slack.MBTAction,
					BlockID: "link_user_block",
					Elements: &slack.BlockElements{
						ElementSet: []slack.BlockElement{
							&slack.ButtonBlockElement{
								ActionID: "link_user_element",
								Type:     slack.METButton,
								Style:    slack.StylePrimary,
								Text: &slack.TextBlockObject{
									Type: slack.PlainTextType,
									Text: "Link Account",
								},
								URL: linkURL,
							},
						},
					},
				},
			},
		},
	}

	return slack.PostWebhookContext(ctx, responseURL, &msg)
}
