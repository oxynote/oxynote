package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
)

// InsertGithubInstallation inserts a new Github installation into the database.
func (a *agent) InsertGithubInstallation(ctx context.Context, installationID int64) error {
	q, args := a.builder.Insert("github_installations").
		SetMap(map[string]any{
			"installation_id": installationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchGithubInstallationByOrganizationID retrieves the installation ID for a given organization ID.
func (a *agent) FetchGithubInstallationByOrganizationID(ctx context.Context, organizationID string) (int64, error) {
	q, args := a.builder.Select("installation_id").
		From("github_installations").
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		MustSql()

	var installationID int64

	if err := sqlx.GetContext(ctx, a.sql, &installationID, q, args...); err != nil {
		return 0, err
	}

	return installationID, nil
}

// FetchGithubInstallationOrganizationID retrieves the organization ID for a given Github installation.
func (a *agent) FetchGithubInstallationOrganizationID(ctx context.Context, installationID int64) (null.String, error) {
	q, args := a.builder.Select("fk_organization_id").
		From("github_installations").
		Where(sq.Eq{
			"installation_id": installationID,
		}).
		MustSql()

	var organizationID null.String

	if err := sqlx.GetContext(ctx, a.sql, &organizationID, q, args...); err != nil {
		return null.String{}, err
	}

	return organizationID, nil
}

// UpdateGithubInstallationOrganizationID updates the organization ID for a given Github installation.
func (a *agent) UpdateGithubInstallationOrganizationID(ctx context.Context, installationID int64, organizationID string) error {
	q, args := a.builder.Update("github_installations").
		SetMap(map[string]any{
			"fk_organization_id": organizationID,
		}).
		Where(sq.Eq{
			"installation_id": installationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteGithubInstallation removes a Github installation from the database.
func (a *agent) DeleteGithubInstallation(ctx context.Context, installationID int64) error {
	q, args := a.builder.Delete("github_installations").
		Where(sq.Eq{
			"installation_id": installationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UnassignGithubInstallationOrganization removes the organization association from a Github installation.
func (a *agent) UnassignGithubInstallationOrganization(ctx context.Context, organizationID string) error {
	q, args := a.builder.Update("github_installations").
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
