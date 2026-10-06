package db

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// UpsertDocumentBranchView records that the user viewed the branch at the
// given time. A branch viewed before keeps one row with the newer time.
func (a *agent) UpsertDocumentBranchView(
	ctx context.Context,
	userID string,
	organizationID string,
	branchID xid.ID,
	viewedAt time.Time,
) error {
	q, args := a.builder.Insert("document_branch_views").
		SetMap(map[string]any{
			"fk_user_id":         userID,
			"fk_organization_id": organizationID,
			"fk_branch_id":       branchID,
			"viewed_at":          viewedAt,
		}).
		Suffix("ON CONFLICT (fk_user_id, fk_branch_id) DO UPDATE SET viewed_at = EXCLUDED.viewed_at").
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchRecentlyViewedDocuments fetches up to limit branches the user viewed
// in the organization, joined against their document, most recent first.
func (a *agent) FetchRecentlyViewedDocuments(
	ctx context.Context,
	userID string,
	organizationID string,
	limit int,
) ([]document.Document, error) {
	q, args := a.selectDocumentBranch(a.builder.Select()).
		Join("document_branch_views dbv ON dbv.fk_branch_id = db.id").
		Where(sq.Eq{
			"dbv.fk_user_id":               userID,
			"dbv.fk_organization_id":       organizationID,
			"documents.fk_organization_id": organizationID,
		}).
		OrderBy("dbv.viewed_at DESC", "db.id DESC").
		Limit(uint64(limit)).
		MustSql()

	docs := []document.Document{}

	if err := sqlx.SelectContext(ctx, a.sql, &docs, q, args...); err != nil {
		return nil, err
	}

	return docs, nil
}
