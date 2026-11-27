package db

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
)

// recordDocumentCommentReplyRevision stores the reply's text as a new revision.
func (a *agent) recordDocumentCommentReplyRevision(ctx context.Context, tx *sqlx.Tx, r comment.Reply) error {
	rev := comment.NewReplyRevision(r)

	q, args := a.builder.Insert("document_comment_reply_revisions").
		SetMap(map[string]any{
			"id":                           rev.ID,
			"fk_organization_id":           rev.OrganizationID,
			"fk_document_comment_reply_id": rev.ReplyID,
			"content":                      rev.Content,
			"fk_user_id":                   rev.UserID,
			"created_at":                   rev.CreatedAt,
		}).MustSql()

	_, err := tx.ExecContext(ctx, q, args...)

	return err
}
