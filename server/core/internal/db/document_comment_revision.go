package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
)

// recordDocumentCommentRevision stores the comment's text as a new revision.
func (a *agent) recordDocumentCommentRevision(ctx context.Context, tx *sqlx.Tx, c comment.Comment) error {
	rev := comment.NewRevision(c)

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
