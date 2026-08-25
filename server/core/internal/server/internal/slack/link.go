package slack

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	goslack "github.com/slack-go/slack"
)

const (
	// _linkCommand is the slash command that links a Slack account.
	_linkCommand = "/link"

	// _unlinkCommand is the slash command that unlinks a Slack account.
	_unlinkCommand = "/unlink"
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
	case _linkCommand:
		h.handleLinkCommand(w, r, userID, teamID, responseURL)
	case _unlinkCommand:
		h.handleUnlinkCommand(w, r, userID, teamID, responseURL)
	default:
		if err := h.sendEphemeralResponse(r.Context(), responseURL, "Unknown command."); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		httpserver.Respond(h.log, w, nil, http.StatusNoContent)
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

		httpserver.Respond(h.log, w, nil, http.StatusNoContent)

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

		httpserver.Respond(h.log, w, nil, http.StatusNoContent)

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

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// handleUnlinkCommand handles the /unlink slash command.
func (h *Handler) handleUnlinkCommand(w http.ResponseWriter, r *http.Request, userID, teamID, responseURL string) {
	link, err := h.db.FetchSlackUserLink(r.Context(), userID, teamID)
	if err != nil {
		if errutil.IsNotFound(err) {
			serr := h.sendEphemeralResponse(
				r.Context(),
				responseURL,
				"Your Slack account is not linked to a Oxynote account.",
			)
			if serr != nil {
				httpserver.RespondError(h.log, w, serr)
				return
			}

			httpserver.Respond(h.log, w, nil, http.StatusNoContent)

			return
		}

		httpserver.RespondError(h.log, w, err)

		return
	}

	if err = h.db.DeleteSlackUserLink(r.Context(), link.SlackUserID, link.TeamID); err != nil {
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

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// FetchUserLink retrieves the user's Slack link settings.
func (h *Handler) FetchUserLink(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var settings slack.UserLinkSettings

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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// LinkUser handles the completion of the user linking flow.
func (h *Handler) LinkUser(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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

	if err = h.db.InsertSlackUserLink(
		r.Context(),
		*slack.NewUserLink(ls.SlackUserID, ls.TeamID, session.UserID),
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
			goslack.MsgOptionText("Your Slack account has been successfully linked to your Oxynote account.", false),
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
	msg := goslack.WebhookMessage{
		ResponseType: "ephemeral",
		Blocks: &goslack.Blocks{
			BlockSet: []goslack.Block{
				&goslack.SectionBlock{
					Type: goslack.MBTSection,
					Text: &goslack.TextBlockObject{
						Type: goslack.MarkdownType,
						Text: "Link your Slack account to Oxynote to enable personalized features and notifications.",
					},
				},
				&goslack.ActionBlock{
					Type:    goslack.MBTAction,
					BlockID: "link_user_block",
					Elements: &goslack.BlockElements{
						ElementSet: []goslack.BlockElement{
							&goslack.ButtonBlockElement{
								ActionID: "link_user_element",
								Type:     goslack.METButton,
								Style:    goslack.StylePrimary,
								Text: &goslack.TextBlockObject{
									Type: goslack.PlainTextType,
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

	return goslack.PostWebhookContext(ctx, responseURL, &msg)
}
