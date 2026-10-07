package db

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
)

// UpsertDocumentBranchView records that the user viewed the branch at the
// given time, under search.ViewFromAll and, when the origin is another
// one, under that origin too. A branch viewed from the same origin before
// keeps one row with the newer time.
func (a *agent) UpsertDocumentBranchView(
	ctx context.Context,
	userID string,
	organizationID string,
	branchID xid.ID,
	from search.ViewFrom,
	viewedAt time.Time,
) error {
	b := a.builder.Insert("document_branch_views").
		Columns("fk_user_id", "fk_organization_id", "fk_branch_id", "viewed_from", "viewed_at").
		Values(userID, organizationID, branchID, search.ViewFromAll, viewedAt)

	if from != search.ViewFromAll {
		b = b.Values(userID, organizationID, branchID, from, viewedAt)
	}

	q, args := b.
		Suffix("ON CONFLICT (fk_user_id, fk_branch_id, viewed_from) DO UPDATE SET viewed_at = EXCLUDED.viewed_at").
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchRecentlyViewedDocuments fetches up to limit branches the user
// viewed from the given origin in the organization, joined against their
// document, most recent first.
func (a *agent) FetchRecentlyViewedDocuments(
	ctx context.Context,
	userID string,
	organizationID string,
	from search.ViewFrom,
	limit int,
) ([]search.RecentDocument, error) {
	q, args := a.selectDocumentBranch(a.builder.Select("dbv.viewed_at")).
		Join("document_branch_views dbv ON dbv.fk_branch_id = db.id").
		Where(sq.Eq{
			"dbv.fk_user_id":               userID,
			"dbv.fk_organization_id":       organizationID,
			"dbv.viewed_from":              from,
			"documents.fk_organization_id": organizationID,
		}).
		OrderBy("dbv.viewed_at DESC", "db.id DESC").
		Limit(uint64(limit)).
		MustSql()

	docs := []search.RecentDocument{}

	if err := sqlx.SelectContext(ctx, a.sql, &docs, q, args...); err != nil {
		return nil, err
	}

	return docs, nil
}
