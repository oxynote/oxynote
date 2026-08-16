package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
)

// InsertSlackUserLink inserts or updates a Slack user link in the database.
func (a *agent) InsertSlackUserLink(ctx context.Context, link slack.UserLink) error {
	q, args := a.builder.Insert("slack_user_links").
		SetMap(map[string]any{
			"slack_user_id": link.SlackUserID,
			"fk_team_id":    link.TeamID,
			"fk_user_id":    link.UserID,
			"settings":      link.Settings,
			"created_at":    link.CreatedAt,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateSlackUserLink updates an existing Slack user link in the database.
func (a *agent) UpdateSlackUserLink(ctx context.Context, link slack.UserLink) error {
	q, args := a.builder.Update("slack_user_links").
		SetMap(map[string]any{
			"settings": link.Settings,
		}).
		Where(sq.Eq{
			"slack_user_id": link.SlackUserID,
			"fk_team_id":    link.TeamID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchSlackUserLink retrieves a Slack user link by Slack user ID and team ID.
func (a *agent) FetchSlackUserLink(ctx context.Context, slackUserID, teamID string) (*slack.UserLink, error) {
	q, args := a.selectSlackUserLink(a.builder.Select()).
		Where(sq.Eq{
			"slack_user_id": slackUserID,
			"fk_team_id":    teamID,
		}).
		Limit(1).
		MustSql()

	link := &slack.UserLink{}
	if err := sqlx.GetContext(ctx, a.sql, link, q, args...); err != nil {
		return nil, err
	}

	return link, nil
}

// FetchSlackUserLinkByUserID retrieves a Slack user link by user ID.
func (a *agent) FetchSlackUserLinkByUserID(ctx context.Context, userID, organizationID string) (*slack.UserLink, error) {
	q, args := a.selectSlackUserLink(a.builder.Select()).
		Where(sq.Eq{
			"fk_user_id":                    userID,
			"slack_apps.fk_organization_id": organizationID,
		}).
		Join(`slack_apps ON slack_apps.team_id = slack_user_links.fk_team_id`).
		Limit(1).
		MustSql()

	link := &slack.UserLink{}
	if err := sqlx.GetContext(ctx, a.sql, link, q, args...); err != nil {
		return nil, err
	}

	return link, nil
}

// DeleteSlackUserLink removes a Slack user link from the database.
func (a *agent) DeleteSlackUserLink(ctx context.Context, slackUserID, teamID string) error {
	q, args := a.builder.Delete("slack_user_links").
		Where(sq.Eq{
			"slack_user_id": slackUserID,
			"fk_team_id":    teamID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// selectSlackUserLink prepares a SQL select statement for fetching Slack user links.
func (a *agent) selectSlackUserLink(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`slack_user_links.slack_user_id AS "slack_user_id"`,
		`slack_user_links.fk_team_id AS "fk_team_id"`,
		`slack_user_links.fk_user_id AS "fk_user_id"`,
		`slack_user_links.settings AS "settings"`,
		`slack_user_links.created_at AS "created_at"`,
	).From("slack_user_links")
}
