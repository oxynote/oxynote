package db

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// InsertDocumentComment inserts a new comment into the database together
// with the first revision of its text, which it returns.
func (a *agent) InsertDocumentComment(ctx context.Context, c comment.Comment) (comment.Revision, error) {
	rev := comment.NewRevision(c)

	err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Insert("document_comments").
			SetMap(map[string]any{
				"id":                    c.ID,
				"fk_document_id":        c.DocumentID,
				"fk_organization_id":    c.OrganizationID,
				"fk_user_id":            c.UserID,
				"fk_branch_id":          c.BranchID,
				"resolved":              c.Resolved,
				"fk_resolved_by":        c.ResolvedBy,
				"resolved_at":           c.ResolvedAt,
				"anchor_block_id":       c.AnchorBlockID,
				"content":               c.Content,
				"diff_deletion_context": c.DiffDeletionContext,
				"created_at":            c.CreatedAt,
				"updated_at":            c.UpdatedAt,
			}).MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		return a.recordDocumentCommentRevision(ctx, tx, rev)
	})
	if err != nil {
		return comment.Revision{}, err
	}

	return rev, nil
}

// InsertDocumentCommentReply inserts a new reply to a comment into the
// database together with the first revision of its text, which it
// returns.
func (a *agent) InsertDocumentCommentReply(ctx context.Context, r comment.Reply) (comment.ReplyRevision, error) {
	rev := comment.NewReplyRevision(r)

	err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Insert("document_comment_replies").
			SetMap(map[string]any{
				"id":                     r.ID,
				"fk_document_comment_id": r.CommentID,
				"fk_organization_id":     r.OrganizationID,
				"fk_user_id":             r.UserID,
				"content":                r.Content,
				"created_at":             r.CreatedAt,
				"updated_at":             r.UpdatedAt,
			}).MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		return a.recordDocumentCommentReplyRevision(ctx, tx, rev)
	})
	if err != nil {
		return comment.ReplyRevision{}, err
	}

	return rev, nil
}

// FetchDocumentComment retrieves a comment by its ID along with its live
// replies. A deleted comment is found only while a live reply keeps its
// thread open.
func (a *agent) FetchDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) (*comment.Comment, error) {
	q, args := a.selectDocumentComment(a.builder.Select()).
		Where(sq.Eq{
			"document_comments.id":                 id,
			"document_comments.fk_document_id":     documentID,
			"document_comments.fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	c := &comment.Comment{}

	if err := sqlx.GetContext(ctx, a.sql, c, q, args...); err != nil {
		return nil, err
	}

	q, args = a.selectDocumentCommentReply(a.builder.Select()).
		Where(sq.Eq{
			"document_comment_replies.fk_document_comment_id": id,
			"document_comment_replies.fk_organization_id":     organizationID,
			"document_comment_replies.deleted_at":             nil,
		}).
		OrderBy("document_comment_replies.created_at ASC").
		MustSql()

	replies := make([]comment.Reply, 0)

	if err := sqlx.SelectContext(ctx, a.sql, &replies, q, args...); err != nil {
		return nil, err
	}

	if c.DeletedAt.Valid && len(replies) == 0 {
		return nil, sql.ErrNoRows
	}

	c.Replies = replies

	return c, nil
}

// FetchDocumentCommentReply retrieves a live reply by its ID.
func (a *agent) FetchDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) (*comment.Reply, error) {
	q, args := a.selectDocumentCommentReply(a.builder.Select()).
		Where(sq.Eq{
			"document_comment_replies.id":                     id,
			"document_comment_replies.fk_document_comment_id": commentID,
			"document_comment_replies.fk_organization_id":     organizationID,
			"document_comment_replies.deleted_at":             nil,
		}).
		Limit(1).
		MustSql()

	r := &comment.Reply{}
	if err := sqlx.GetContext(ctx, a.sql, r, q, args...); err != nil {
		return nil, err
	}

	return r, nil
}

// UpdateDocumentComment updates an existing comment in the database and
// records a revision of its text, which it returns. A deleted comment is
// not found, so a delete that lands first wins.
func (a *agent) UpdateDocumentComment(ctx context.Context, c comment.Comment) (comment.Revision, error) {
	rev := comment.NewRevision(c)

	err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Update("document_comments").
			SetMap(map[string]any{
				"content":         c.Content,
				"resolved":        c.Resolved,
				"fk_resolved_by":  c.ResolvedBy,
				"resolved_at":     c.ResolvedAt,
				"anchor_block_id": c.AnchorBlockID,
				"updated_at":      c.UpdatedAt,
			}).
			Where(sq.Eq{
				"id":                 c.ID,
				"fk_document_id":     c.DocumentID,
				"fk_organization_id": c.OrganizationID,
				"deleted_at":         nil,
			}).MustSql()

		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if n == 0 {
			return errutil.ErrNotFound
		}

		return a.recordDocumentCommentRevision(ctx, tx, rev)
	})
	if err != nil {
		return comment.Revision{}, err
	}

	return rev, nil
}

// UpdateDocumentCommentStatus stores whether a comment is resolved, and by
// whom. The text stays, so no revision is recorded.
func (a *agent) UpdateDocumentCommentStatus(ctx context.Context, c comment.Comment) error {
	q, args := a.builder.Update("document_comments").
		SetMap(map[string]any{
			"resolved":       c.Resolved,
			"fk_resolved_by": c.ResolvedBy,
			"resolved_at":    c.ResolvedAt,
		}).
		Where(sq.Eq{
			"id":                 c.ID,
			"fk_document_id":     c.DocumentID,
			"fk_organization_id": c.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateDocumentCommentReply updates an existing reply in the database and
// records a revision of its text, which it returns. A deleted reply is
// not found.
func (a *agent) UpdateDocumentCommentReply(ctx context.Context, r comment.Reply) (comment.ReplyRevision, error) {
	rev := comment.NewReplyRevision(r)

	err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Update("document_comment_replies").
			SetMap(map[string]any{
				"content":    r.Content,
				"updated_at": r.UpdatedAt,
			}).
			Where(sq.Eq{
				"id":                     r.ID,
				"fk_document_comment_id": r.CommentID,
				"fk_organization_id":     r.OrganizationID,
				"deleted_at":             nil,
			}).MustSql()

		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if n == 0 {
			return errutil.ErrNotFound
		}

		return a.recordDocumentCommentReplyRevision(ctx, tx, rev)
	})
	if err != nil {
		return comment.ReplyRevision{}, err
	}

	return rev, nil
}

// DeleteDocumentComment soft-deletes a comment, clearing its text. A
// comment that is already deleted is not found.
func (a *agent) DeleteDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) error {
	q, args := a.builder.Update("document_comments").
		SetMap(map[string]any{
			"content":    nil,
			"deleted_at": timeutil.Now(),
		}).
		Where(sq.Eq{
			"id":                 id,
			"fk_document_id":     documentID,
			"fk_organization_id": organizationID,
			"deleted_at":         nil,
		}).MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// DeleteDocumentCommentReply soft-deletes a reply, clearing its text. A
// reply that is already deleted is not found.
func (a *agent) DeleteDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) error {
	q, args := a.builder.Update("document_comment_replies").
		SetMap(map[string]any{
			"content":    nil,
			"deleted_at": timeutil.Now(),
		}).
		Where(sq.Eq{
			"id":                     id,
			"fk_document_comment_id": commentID,
			"fk_organization_id":     organizationID,
			"deleted_at":             nil,
		}).MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// FetchDocumentCommentsByBranchID retrieves the comment threads of a
// specific branch in the given resolved state along with their live
// replies. A deleted comment is listed only while a live reply keeps its
// thread open.
func (a *agent) FetchDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string, resolved bool) ([]comment.Comment, error) {
	q, args := a.selectDocumentComment(a.builder.Select()).
		Where(sq.Eq{
			"document_comments.fk_branch_id":       branchID,
			"document_comments.fk_organization_id": organizationID,
			"document_comments.resolved":           resolved,
		}).
		OrderBy("document_comments.created_at DESC").
		MustSql()

	comments := make([]comment.Comment, 0)

	if err := sqlx.SelectContext(ctx, a.sql, &comments, q, args...); err != nil {
		return nil, err
	}

	if len(comments) == 0 {
		return comments, nil
	}

	commentIDs := make([]xid.ID, 0, len(comments))
	for _, c := range comments {
		commentIDs = append(commentIDs, c.ID)
	}

	// one query rather than one per comment: the fan-out held a pool
	// connection per comment, and would have corrupted the protocol
	// outright had this method ever run on a transaction, which is a
	// single connection.
	q, args = a.selectDocumentCommentReply(a.builder.Select()).
		Where(sq.Eq{
			"document_comment_replies.fk_document_comment_id": commentIDs,
			"document_comment_replies.fk_organization_id":     organizationID,
			"document_comment_replies.deleted_at":             nil,
		}).
		OrderBy("document_comment_replies.created_at ASC").
		MustSql()

	var replies []comment.Reply

	if err := sqlx.SelectContext(ctx, a.sql, &replies, q, args...); err != nil {
		return nil, err
	}

	byComment := make(map[xid.ID][]comment.Reply, len(comments))
	for _, r := range replies {
		byComment[r.CommentID] = append(byComment[r.CommentID], r)
	}

	// a deleted comment stays listed only while a live reply keeps its
	// thread open.
	live := comments[:0]

	for _, c := range comments {
		rr, ok := byComment[c.ID]
		if !ok && c.DeletedAt.Valid {
			continue
		}

		if !ok {
			// an empty slice rather than nil, so a comment with no
			// replies serializes as [] here the same way it does
			// from FetchDocumentComment.
			rr = make([]comment.Reply, 0)
		}

		c.Replies = rr
		live = append(live, c)
	}

	return live, nil
}

// DeleteDocumentCommentsByBranchID soft-deletes every live comment of a
// branch and every live reply in those threads, clearing their text. The
// replies go too, because a live reply would keep a deleted comment's
// thread listed.
func (a *agent) DeleteDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		deleted := map[string]any{
			"content":    nil,
			"deleted_at": timeutil.Now(),
		}

		threads := a.builder.Select("id").
			From("document_comments").
			Where(sq.Eq{
				"fk_branch_id":       branchID,
				"fk_organization_id": organizationID,
			})

		q, args := a.builder.Update("document_comment_replies").
			SetMap(deleted).
			Where(sq.Eq{
				"fk_organization_id": organizationID,
				"deleted_at":         nil,
			}).
			Where(threads.Prefix("fk_document_comment_id IN (").Suffix(")")).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		q, args = a.builder.Update("document_comments").
			SetMap(deleted).
			Where(sq.Eq{
				"fk_branch_id":       branchID,
				"fk_organization_id": organizationID,
				"deleted_at":         nil,
			}).
			MustSql()

		_, err := tx.ExecContext(ctx, q, args...)

		return err
	})
}

// selectDocumentComment prepares a sql select statement for fetching comments.
func (a *agent) selectDocumentComment(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_comments.id AS "id"`,
		`document_comments.fk_document_id AS "fk_document_id"`,
		`document_comments.fk_user_id AS "fk_user_id"`,
		`document_comments.fk_organization_id AS "fk_organization_id"`,
		`document_comments.fk_branch_id AS "fk_branch_id"`,
		`document_comments.fk_resolved_by AS "fk_resolved_by"`,
		`document_comments.resolved_at AS "resolved_at"`,
		`document_comments.resolved AS "resolved"`,
		`document_comments.anchor_block_id AS "anchor_block_id"`,
		`document_comments.content AS "content"`,
		`document_comments.diff_deletion_context AS "diff_deletion_context"`,
		`document_comments.created_at AS "created_at"`,
		`document_comments.updated_at AS "updated_at"`,
		`document_comments.deleted_at AS "deleted_at"`,
	).From("document_comments")
}

// selectDocumentCommentReply prepares a sql select statement for fetching replies.
func (a *agent) selectDocumentCommentReply(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_comment_replies.id AS "id"`,
		`document_comment_replies.fk_document_comment_id AS "fk_document_comment_id"`,
		`document_comment_replies.fk_organization_id AS "fk_organization_id"`,
		`document_comment_replies.fk_user_id AS "fk_user_id"`,
		`document_comment_replies.content AS "content"`,
		`document_comment_replies.created_at AS "created_at"`,
		`document_comment_replies.updated_at AS "updated_at"`,
		`document_comment_replies.deleted_at AS "deleted_at"`,
	).From("document_comment_replies")
}
