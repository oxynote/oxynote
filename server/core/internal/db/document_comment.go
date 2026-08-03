package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/heimdall/internal/document/comment"
	"github.com/rs/xid"
	"golang.org/x/sync/errgroup"
)

// InsertDocumentComment inserts a new comment into the database.
func (a *agent) InsertDocumentComment(ctx context.Context, c comment.Comment) error {
	q, args := a.builder.Insert("document_comments").
		SetMap(map[string]any{
			"id":                    c.ID,
			"fk_document_id":        c.DocumentID,
			"fk_organization_id":    c.OrganizationID,
			"fk_user_id":            c.UserID,
			"fk_branch_id":          c.BranchID,
			"resolved":              c.Resolved,
			"fk_resolved_by":        c.ResolvedBy,
			"anchor_block_id":       c.AnchorBlockID,
			"content":               c.Content,
			"diff_deletion_context": c.DiffDeletionContext,
			"created_at":            c.CreatedAt,
			"updated_at":            c.UpdatedAt,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// InsertDocumentCommentReply inserts a new reply to a comment into the database.
func (a *agent) InsertDocumentCommentReply(ctx context.Context, r comment.Reply) error {
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

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentComment retrieves a comment by its ID along with all its replies.
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
		}).
		OrderBy("document_comment_replies.created_at ASC").
		MustSql()

	replies := make([]comment.Reply, 0)

	if err := sqlx.SelectContext(ctx, a.sql, &replies, q, args...); err != nil {
		return nil, err
	}

	c.Replies = replies

	return c, nil
}

// FetchDocumentCommentReply retrieves a reply by its ID.
func (a *agent) FetchDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) (*comment.Reply, error) {
	q, args := a.selectDocumentCommentReply(a.builder.Select()).
		Where(sq.Eq{
			"document_comment_replies.id":                     id,
			"document_comment_replies.fk_document_comment_id": commentID,
			"document_comment_replies.fk_organization_id":     organizationID,
		}).
		Limit(1).
		MustSql()

	r := &comment.Reply{}
	if err := sqlx.GetContext(ctx, a.sql, r, q, args...); err != nil {
		return nil, err
	}

	return r, nil
}

// UpdateDocumentComment updates an existing comment in the database.
func (a *agent) UpdateDocumentComment(ctx context.Context, c comment.Comment) error {
	q, args := a.builder.Update("document_comments").
		SetMap(map[string]any{
			"content":         c.Content,
			"resolved":        c.Resolved,
			"fk_resolved_by":  c.ResolvedBy,
			"anchor_block_id": c.AnchorBlockID,
			"updated_at":      c.UpdatedAt,
		}).
		Where(sq.Eq{
			"id":                 c.ID,
			"fk_document_id":     c.DocumentID,
			"fk_organization_id": c.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateDocumentCommentReply updates an existing reply in the database.
func (a *agent) UpdateDocumentCommentReply(ctx context.Context, r comment.Reply) error {
	q, args := a.builder.Update("document_comment_replies").
		SetMap(map[string]any{
			"content":    r.Content,
			"updated_at": r.UpdatedAt,
		}).
		Where(sq.Eq{
			"id":                     r.ID,
			"fk_document_comment_id": r.CommentID,
			"fk_organization_id":     r.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteDocumentComment deletes a comment from the database.
func (a *agent) DeleteDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) error {
	q, args := a.builder.Delete("document_comments").
		Where(sq.Eq{
			"id":                 id,
			"fk_document_id":     documentID,
			"fk_organization_id": organizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteDocumentCommentReply deletes a reply from the database.
func (a *agent) DeleteDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) error {
	q, args := a.builder.Delete("document_comment_replies").
		Where(sq.Eq{
			"id":                     id,
			"fk_document_comment_id": commentID,
			"fk_organization_id":     organizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// ReplaceDocumentComment replaces a comment's content and author with the given reply's data.
func (a *agent) ReplaceDocumentComment(ctx context.Context, c comment.Comment) error {
	q, args := a.builder.Update("document_comments").
		SetMap(map[string]any{
			"content":         c.Content,
			"fk_user_id":      c.UserID,
			"updated_at":      c.UpdatedAt,
			"anchor_block_id": c.AnchorBlockID,
			"created_at":      c.CreatedAt,
		}).
		Where(sq.Eq{
			"id":                 c.ID,
			"fk_document_id":     c.DocumentID,
			"fk_organization_id": c.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentCommentsByBranchID retrieves all comments for a specific branch along with their replies.
func (a *agent) FetchDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]comment.Comment, error) {
	q, args := a.selectDocumentComment(a.builder.Select()).
		Where(sq.Eq{
			"document_comments.fk_branch_id":       branchID,
			"document_comments.fk_organization_id": organizationID,
		}).
		OrderBy("document_comments.created_at DESC").
		MustSql()

	comments := make([]comment.Comment, 0)

	if err := sqlx.SelectContext(ctx, a.sql, &comments, q, args...); err != nil {
		return nil, err
	}

	eg, ectx := errgroup.WithContext(ctx)

	for i := range comments {
		eg.Go(func() error {
			q, args := a.selectDocumentCommentReply(a.builder.Select()).
				Where(sq.Eq{
					"document_comment_replies.fk_document_comment_id": comments[i].ID,
					"document_comment_replies.fk_organization_id":     organizationID,
				}).
				OrderBy("document_comment_replies.created_at ASC").
				MustSql()

			var replies []comment.Reply

			if err := sqlx.SelectContext(ectx, a.sql, &replies, q, args...); err != nil {
				return err
			}

			comments[i].Replies = replies

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return comments, nil
}

// DeleteDocumentCommentsByBranchID deletes all comments for a specific branch.
func (a *agent) DeleteDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error {
	q, args := a.builder.Delete("document_comments").
		Where(sq.Eq{
			"fk_branch_id":       branchID,
			"fk_organization_id": organizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
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
		`document_comments.resolved AS "resolved"`,
		`document_comments.anchor_block_id AS "anchor_block_id"`,
		`document_comments.content AS "content"`,
		`document_comments.diff_deletion_context AS "diff_deletion_context"`,
		`document_comments.created_at AS "created_at"`,
		`document_comments.updated_at AS "updated_at"`,
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
	).From("document_comment_replies")
}
