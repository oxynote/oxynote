package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/rs/xid"
)

// Each select below lists the entries of one type, with the columns in
// the order backfillEntries reads them.
const (
	// _selectDocumentCreated lists each document's creation on its
	// default branch.
	_selectDocumentCreated = `
SELECT d.fk_organization_id, d.id, b.id, d.fk_created_by,
	'document', 'document.created', NULL AS target_kind, NULL AS target_id, '{}', d.created_at
FROM documents d
JOIN document_branches b ON b.fk_document_id = d.id AND b."default"`

	// _selectBranchForked lists each fork on the branch it made. A branch
	// does not store its source, so the entry names only the new branch.
	_selectBranchForked = `
SELECT fk_organization_id, fk_document_id, id, fk_created_by,
	'branch', 'branch.forked', NULL AS target_kind, NULL AS target_id, jsonb_build_object('toBranchId', id), created_at
FROM document_branches
WHERE NOT "default"`

	// _selectCommentCreated lists each comment, pointing at its oldest
	// revision.
	_selectCommentCreated = `
SELECT c.fk_organization_id, c.fk_document_id, c.fk_branch_id, c.fk_user_id,
	'comment', 'comment.created', 'comment-revision',
	(SELECT id FROM document_comment_revisions WHERE fk_document_comment_id = c.id ORDER BY created_at, id LIMIT 1),
	jsonb_build_object('commentId', c.id), c.created_at
FROM document_comments c
WHERE c.content IS NOT NULL`

	// _selectCommentReplied lists each reply, pointing at its oldest
	// revision.
	_selectCommentReplied = `
SELECT c.fk_organization_id, c.fk_document_id, c.fk_branch_id, r.fk_user_id,
	'comment', 'comment.replied', 'reply-revision',
	(SELECT id FROM document_comment_reply_revisions WHERE fk_document_comment_reply_id = r.id ORDER BY created_at, id LIMIT 1),
	jsonb_build_object('commentId', c.id, 'replyId', r.id), r.created_at
FROM document_comment_replies r
JOIN document_comments c ON c.id = r.fk_document_comment_id
WHERE r.content IS NOT NULL`

	// _selectCommentResolved lists each resolution, pointing at the
	// comment's newest revision.
	_selectCommentResolved = `
SELECT c.fk_organization_id, c.fk_document_id, c.fk_branch_id, c.fk_resolved_by,
	'comment', 'comment.resolved', 'comment-revision',
	(SELECT id FROM document_comment_revisions WHERE fk_document_comment_id = c.id ORDER BY created_at DESC, id DESC LIMIT 1),
	jsonb_build_object('commentId', c.id), c.resolved_at
FROM document_comments c
WHERE c.resolved AND c.resolved_at IS NOT NULL AND c.content IS NOT NULL`
)

// _insertEntry stores one backfilled entry. Every backfilled actor is a
// user.
const _insertEntry = `
INSERT INTO document_activity_entries (
	id, fk_organization_id, fk_document_id, fk_branch_id, actor_kind, fk_actor_id,
	category, type, target_kind, target_id, metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, 'user', $5, $6, $7, $8, $9, $10, $11, $11)`

// backfillDocumentActivityEntries rebuilds the entries that existing rows
// can tell. A deleted comment or reply has no revision, so it gets none.
func backfillDocumentActivityEntries() *goose.Migration {
	steps := []struct {
		name  string
		query string
	}{
		{"document.created", _selectDocumentCreated},
		{"branch.forked", _selectBranchForked},
		{"comment.created", _selectCommentCreated},
		{"comment.replied", _selectCommentReplied},
		{"comment.resolved", _selectCommentResolved},
	}

	up := func(ctx context.Context, tx *sql.Tx) error {
		for _, s := range steps {
			if err := backfillEntries(ctx, tx, s.query); err != nil {
				return fmt.Errorf("backfilling %s entries: %w", s.name, err)
			}
		}

		return nil
	}

	// the entries written since cannot be told apart from the backfilled
	// ones, so every entry goes.
	down := func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM document_activity_entries"); err != nil {
			return fmt.Errorf("deleting activity entries: %w", err)
		}

		return nil
	}

	return goose.NewGoMigration(_versionDocumentActivityEntriesBackfill, &goose.GoFunc{RunTx: up}, &goose.GoFunc{RunTx: down})
}

// backfillEntries stores every entry the select lists, each with a new id.
func backfillEntries(ctx context.Context, tx *sql.Tx, selectEntries string) error {
	rows, err := tx.QueryContext(ctx, selectEntries)
	if err != nil {
		return fmt.Errorf("selecting entries: %w", err)
	}

	defer rows.Close() //nolint:errcheck // rows.Err reports what went wrong

	// the rows are read in full first, because the transaction cannot run
	// an insert while they are open.
	var entries [][]any

	for rows.Next() {
		var (
			organizationID, documentID, branchID string
			actorID                              sql.NullString
			category, tp                         string
			targetKind, targetID                 sql.NullString
			metadata                             string
			createdAt                            time.Time
		)

		err = rows.Scan(
			&organizationID,
			&documentID,
			&branchID,
			&actorID,
			&category,
			&tp,
			&targetKind,
			&targetID,
			&metadata,
			&createdAt,
		)
		if err != nil {
			// NOCOV: every select lists the columns in this order.
			return fmt.Errorf("scanning an entry: %w", err)
		}

		entries = append(entries, []any{
			xid.New().String(),
			organizationID,
			documentID,
			branchID,
			actorID,
			category,
			tp,
			targetKind,
			targetID,
			metadata,
			createdAt,
		})
	}

	if err = rows.Err(); err != nil {
		// NOCOV: a test cannot break the connection between two rows.
		return fmt.Errorf("reading entries: %w", err)
	}

	for _, args := range entries {
		if _, err = tx.ExecContext(ctx, _insertEntry, args...); err != nil {
			// NOCOV: a test cannot make this insert fail without failing
			// the select above first.
			return fmt.Errorf("inserting an entry: %w", err)
		}
	}

	return nil
}
