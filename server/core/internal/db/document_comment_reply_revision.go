package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
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

// fetchDocumentCommentReplyRevisionsByIDs retrieves the reply revisions
// with the given ids.
func (a *agent) fetchDocumentCommentReplyRevisionsByIDs(
	ctx context.Context,
	ids []string,
	organizationID string,
) ([]comment.ReplyRevision, error) {
	q, args := a.builder.Select(
		"id",
		"fk_organization_id",
		"fk_document_comment_reply_id",
		"content",
		"fk_user_id",
		"created_at",
	).
		From("document_comment_reply_revisions").
		Where(sq.Eq{
			"id":                 ids,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	revs := []comment.ReplyRevision{}

	if err := sqlx.SelectContext(ctx, a.sql, &revs, q, args...); err != nil {
		return nil, err
	}

	return revs, nil
}
