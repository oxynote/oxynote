package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
)

// UpsertDocumentMaintainers adds maintainers to a document, ignoring the ones
// already there. The set only ever grows: it accumulates everyone who has
// edited the document, and no path removes a maintainer from it.
func (a *agent) UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error {
	if len(maintainerIDs) == 0 {
		return nil
	}

	sb := a.builder.Insert("document_maintainers").
		Columns(
			"fk_document_id",
			"fk_organization_id",
			"fk_user_id",
		).Suffix(`ON CONFLICT (fk_document_id, fk_user_id) DO NOTHING`)

	for _, maintainerID := range maintainerIDs {
		sb = sb.Values(
			documentID,
			organizationID,
			maintainerID,
		)
	}

	q, args := sb.MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentMaintainers retrieves document maintainers by its ID from the database.
func (a *agent) FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error) {
	b := a.selectDocumentMaintainers(a.builder.Select()).
		Where(sq.Eq{
			"document_maintainers.fk_document_id":     documentID,
			"document_maintainers.fk_organization_id": organizationID,
		})

	q, args := b.MustSql()

	maintainers := []string{}
	if err := sqlx.SelectContext(ctx, a.sql, &maintainers, q, args...); err != nil {
		return nil, err
	}

	return maintainers, nil
}

// selectDocumentMaintainers prepares a sql select statement for fetching document maintainers.
func (a *agent) selectDocumentMaintainers(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_maintainers.fk_user_id AS "fk_user_id"`,
	).From("document_maintainers")
}
