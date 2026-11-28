package db

import (
	"context"

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
