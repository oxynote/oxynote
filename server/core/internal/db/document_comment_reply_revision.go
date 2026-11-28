package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/rs/xid"
)

// recordDocumentCommentReplyRevision stores the revision of a reply's
// text.
func (a *agent) recordDocumentCommentReplyRevision(ctx context.Context, tx *sqlx.Tx, rev comment.ReplyRevision) error {
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

// FetchNewestDocumentCommentReplyRevision retrieves the newest revision of
// the reply's text.
func (a *agent) FetchNewestDocumentCommentReplyRevision(
	ctx context.Context,
	replyID xid.ID,
	organizationID string,
) (comment.ReplyRevision, error) {
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
			"fk_document_comment_reply_id": replyID,
			"fk_organization_id":           organizationID,
		}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		MustSql()

	var rev comment.ReplyRevision

	if err := sqlx.GetContext(ctx, a.sql, &rev, q, args...); err != nil {
		return comment.ReplyRevision{}, err
	}

	return rev, nil
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
