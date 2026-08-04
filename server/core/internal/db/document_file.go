package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// InsertDocumentFile inserts a new document file into the database.
func (a *agent) InsertDocumentFile(ctx context.Context, f document.File) error {
	q, args := a.builder.Insert("document_files").
		SetMap(map[string]any{
			"id":                 f.ID,
			"location":           f.Location,
			"fk_document_id":     f.DocumentID,
			"fk_organization_id": f.OrganizationID,
			"created_at":         f.CreatedAt,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentFile retrieves a document file by its block ID from the database.
func (a *agent) FetchDocumentFile(ctx context.Context, id string, organizationID string) (*document.File, error) {
	q, args := a.selectDocumentFile(a.builder.Select()).
		Where(sq.Eq{
			"document_files.id":                 id,
			"document_files.fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	f := &document.File{}
	if err := sqlx.GetContext(ctx, a.sql, f, q, args...); err != nil {
		return nil, err
	}

	return f, nil
}

// selectDocumentFile prepares a sql select statement for fetching document files.
func (a *agent) selectDocumentFile(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_files.id AS "id"`,
		`document_files.location AS "location"`,
		`document_files.fk_document_id AS "fk_document_id"`,
		`document_files.fk_organization_id AS "fk_organization_id"`,
		`document_files.created_at AS "created_at"`,
	).From("document_files")
}
