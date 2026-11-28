package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/rs/xid"
)

// recordDocumentCommentRevision stores the revision of a comment's text.
func (a *agent) recordDocumentCommentRevision(ctx context.Context, tx *sqlx.Tx, rev comment.Revision) error {
	q, args := a.builder.Insert("document_comment_revisions").
		SetMap(map[string]any{
			"id":                     rev.ID,
			"fk_organization_id":     rev.OrganizationID,
			"fk_document_comment_id": rev.CommentID,
			"content":                rev.Content,
			"fk_user_id":             rev.UserID,
			"created_at":             rev.CreatedAt,
		}).MustSql()

	_, err := tx.ExecContext(ctx, q, args...)

	return err
}

// FetchNewestDocumentCommentRevision retrieves the newest revision of
// the comment's text.
func (a *agent) FetchNewestDocumentCommentRevision(
	ctx context.Context,
	commentID xid.ID,
	organizationID string,
) (comment.Revision, error) {
	q, args := a.builder.Select(
		"id",
		"fk_organization_id",
		"fk_document_comment_id",
		"content",
		"fk_user_id",
		"created_at",
	).
		From("document_comment_revisions").
		Where(sq.Eq{
			"fk_document_comment_id": commentID,
			"fk_organization_id":     organizationID,
		}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		MustSql()

	var rev comment.Revision

	if err := sqlx.GetContext(ctx, a.sql, &rev, q, args...); err != nil {
		return comment.Revision{}, err
	}

	return rev, nil
}

// fetchDocumentCommentRevisionsByIDs retrieves the comment revisions
// with the given ids.
func (a *agent) fetchDocumentCommentRevisionsByIDs(
	ctx context.Context,
	ids []string,
	organizationID string,
) ([]comment.Revision, error) {
	q, args := a.builder.Select(
		"id",
		"fk_organization_id",
		"fk_document_comment_id",
		"content",
		"fk_user_id",
		"created_at",
	).
		From("document_comment_revisions").
		Where(sq.Eq{
			"id":                 ids,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	revs := []comment.Revision{}

	if err := sqlx.SelectContext(ctx, a.sql, &revs, q, args...); err != nil {
		return nil, err
	}

	return revs, nil
}
