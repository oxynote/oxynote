package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/heimdall/internal/datasource"
	"github.com/rs/xid"
)

// InsertDataSource inserts a data source into the database.
func (a *agent) InsertDataSource(ctx context.Context, ds *datasource.DataSource) error {
	safeCredentials, err := ds.Credentials.Encrypt(a.opts.DataSourceCredentialsSigningSecret)
	if err != nil {
		return err
	}

	q, args := a.builder.Insert("data_sources").
		SetMap(map[string]any{
			"id":                 ds.ID,
			"fk_organization_id": ds.OrganizationID,
			"name":               ds.Name,
			"type":               ds.Type,
			"url":                ds.URL,
			"credentials":        safeCredentials,
			"status":             ds.Status,
			"created_at":         ds.CreatedAt,
			"updated_at":         ds.UpdatedAt,
		}).
		MustSql()

	_, err = a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateDataSource updates an existing data source in the database.
func (a *agent) UpdateDataSource(ctx context.Context, ds *datasource.DataSource) error {
	safeCredentials, err := ds.Credentials.Encrypt(a.opts.DataSourceCredentialsSigningSecret)
	if err != nil {
		return err
	}

	q, args := a.builder.Update("data_sources").
		SetMap(map[string]any{
			"name":        ds.Name,
			"url":         ds.URL,
			"credentials": safeCredentials,
			"status":      ds.Status,
			"updated_at":  ds.UpdatedAt,
		}).
		Where(sq.Eq{
			"id": ds.ID,
		}).
		MustSql()

	_, err = a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteDataSource removes a data source from the database.
func (a *agent) DeleteDataSource(ctx context.Context, id xid.ID, organizationID string) error {
	q, args := a.builder.Delete("data_sources").
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDataSource retrieves a data source by ID and organization ID.
func (a *agent) FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasource.DataSource, error) {
	q, args := a.selectDataSource(a.builder.Select()).
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	ds := &datasource.DataSource{}
	if err := sqlx.GetContext(ctx, a.sql, ds, q, args...); err != nil {
		return nil, err
	}

	err := ds.Credentials.Decrypt(a.opts.DataSourceCredentialsSigningSecret)
	if err != nil {
		return nil, err
	}

	return ds, nil
}

// FetchDataSources retrieves all data sources for an organization.
func (a *agent) FetchDataSources(ctx context.Context, organizationID string) ([]datasource.DataSource, error) {
	q, args := a.selectDataSource(a.builder.Select()).
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		OrderBy("created_at DESC").
		MustSql()

	var sources []datasource.DataSource

	if err := sqlx.SelectContext(ctx, a.sql, &sources, q, args...); err != nil {
		return nil, err
	}

	for i := range sources {
		err := sources[i].Credentials.Decrypt(a.opts.DataSourceCredentialsSigningSecret)
		if err != nil {
			return nil, err
		}
	}

	return sources, nil
}

// selectDataSource prepares a SQL select statement for fetching data sources.
func (a *agent) selectDataSource(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		"id",
		"fk_organization_id",
		"name",
		"type",
		"url",
		"credentials",
		"status",
		"created_at",
		"updated_at",
	).From("data_sources")
}
