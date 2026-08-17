package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
)

// InsertSlackApp inserts or updates a Slack app document in the database.
func (a *agent) InsertSlackApp(ctx context.Context, app slack.App) error {
	q, args := a.builder.Insert("slack_apps").
		SetMap(map[string]any{
			"team_id":            app.TeamID,
			"token":              app.Token,
			"fk_organization_id": app.OrganizationID,
		}).
		Suffix(`
			ON CONFLICT (team_id)
			DO UPDATE SET
				token = EXCLUDED.token,
				fk_organization_id = COALESCE(EXCLUDED.fk_organization_id, slack_apps.fk_organization_id)
		`).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// InsertSlackMessage inserts a Slack message into the database.
func (a *agent) InsertSlackMessage(ctx context.Context, msg slack.Message) error {
	q, args := a.builder.Insert("slack_messages").
		SetMap(map[string]any{
			"id":                 msg.ID,
			"fk_organization_id": msg.OrganizationID,
			"text":               msg.Text,
			"created_at":         msg.CreatedAt,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchSlackMessages retrieves Slack messages for a given organization ID from the database.
func (a *agent) FetchSlackMessages(ctx context.Context, organizationID string) ([]slack.Message, error) {
	q, args := a.selectSlackMessage(a.builder.Select()).
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		OrderBy("created_at DESC").
		MustSql()

	var messages []slack.Message

	if err := sqlx.SelectContext(ctx, a.sql, &messages, q, args...); err != nil {
		return nil, err
	}

	return messages, nil
}

// UpdateSlackAppOrganizationID updates the Slack app organization id in the database.
func (a *agent) UpdateSlackAppOrganizationID(ctx context.Context, teamID, organizationID string) error {
	q, args := a.builder.Update("slack_apps").
		SetMap(map[string]any{
			"fk_organization_id": organizationID,
		}).
		Where(sq.Eq{
			"team_id": teamID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchSlackAppByTeamID retrieves the Slack app for a given team ID from the database.
func (a *agent) FetchSlackAppByTeamID(ctx context.Context, teamID string) (*slack.App, error) {
	q, args := a.selectSlackApp(a.builder.Select()).
		Where(sq.Eq{
			"team_id": teamID,
		}).
		Limit(1).
		MustSql()

	app := &slack.App{}
	if err := sqlx.GetContext(ctx, a.sql, app, q, args...); err != nil {
		return nil, err
	}

	return app, nil
}

// FetchSlackAppByOrganizationID retrieves the installation ID for a given organization ID.
func (a *agent) FetchSlackAppByOrganizationID(ctx context.Context, organizationID string) (*slack.App, error) {
	q, args := a.selectSlackApp(a.builder.Select()).
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	app := &slack.App{}
	if err := sqlx.GetContext(ctx, a.sql, app, q, args...); err != nil {
		return nil, err
	}

	return app, nil
}

// DeleteSlackApp removes a Slack app document from the database by team ID.
func (a *agent) DeleteSlackApp(ctx context.Context, teamID string) error {
	q, args := a.builder.Delete("slack_apps").
		Where(sq.Eq{
			"team_id": teamID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteSlackAppsByOrganizationID removes the Slack apps of an organization,
// and with them the workspace tokens they hold.
func (a *agent) DeleteSlackAppsByOrganizationID(ctx context.Context, organizationID string) error {
	q, args := a.builder.Delete("slack_apps").
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UnassignSlackAppOrganization removes the organization association from a Slack app.
func (a *agent) UnassignSlackAppOrganization(ctx context.Context, organizationID string) error {
	q, args := a.builder.Update("slack_apps").
		SetMap(map[string]any{
			"fk_organization_id": nil,
		}).
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// selectSlackApp prepares a sql select statement for fetching documents.
func (a *agent) selectSlackApp(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`slack_apps.team_id AS "team_id"`,
		`slack_apps.fk_organization_id AS "fk_organization_id"`,
		`slack_apps.token AS "token"`,
	).From("slack_apps")
}

// selectSlackMessage prepares a sql select statement for fetching Slack messages.
func (a *agent) selectSlackMessage(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`slack_messages.id AS "id"`,
		`slack_messages.fk_organization_id AS "fk_organization_id"`,
		`slack_messages.text AS "text"`,
		`slack_messages.created_at AS "created_at"`,
	).From("slack_messages")
}
