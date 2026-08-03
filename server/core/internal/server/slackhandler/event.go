package slackhandler

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

// handleChannelOpenEvent handles the Slack event when a channel is opened.
func (h *Handler) handleChannelOpenEvent(ctx context.Context, teamID, userID, channelID string) error {
	app, err := h.db.FetchSlackAppByTeamID(ctx, teamID)
	if err != nil {
		return err
	}

	if app.OrganizationID.Valid {
		return nil
	}

	client, err := h.man.GetClient(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get slack client: %w", err)
	}

	user, err := client.GetUserInfo(userID)
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	if !user.IsAdmin && !user.IsOwner {
		return nil
	}

	history, err := client.GetConversationHistoryContext(
		ctx,
		&slack.GetConversationHistoryParameters{
			ChannelID: channelID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get conversation history: %w", err)
	}

	if len(history.Messages) > 0 {
		return nil
	}

	url, err := h.man.CreateInternalInstallationURL(teamID)
	if err != nil {
		return fmt.Errorf("failed to create installation URL: %w", err)
	}

	_, _, err = client.PostMessageContext(
		ctx,
		channelID,
		connectOrganizationMessage(
			fmt.Sprintf(_welcomeMessageTemplate, user.ID),
			url,
		),
	)

	return fmt.Errorf("failed to post message: %w", err)
}
