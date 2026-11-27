package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/rs/xid"
)

// _insertCommentRevisions copies each comment named by a sourceId in $1
// into a revision with the id next to it.
const _insertCommentRevisions = `
INSERT INTO document_comment_revisions (id, fk_organization_id, fk_document_comment_id, content, fk_user_id, created_at)
SELECT ids.id, c.fk_organization_id, c.id, c.content, c.fk_user_id, COALESCE(c.updated_at, c.created_at)
FROM jsonb_to_recordset($1::jsonb) AS ids(id TEXT, "sourceId" TEXT)
JOIN document_comments c ON c.id = ids."sourceId"`

// _insertReplyRevisions copies each reply named by a sourceId in $1 into
// a revision with the id next to it.
const _insertReplyRevisions = `
INSERT INTO document_comment_reply_revisions (id, fk_organization_id, fk_document_comment_reply_id, content, fk_user_id, created_at)
SELECT ids.id, r.fk_organization_id, r.id, r.content, r.fk_user_id, COALESCE(r.updated_at, r.created_at)
FROM jsonb_to_recordset($1::jsonb) AS ids(id TEXT, "sourceId" TEXT)
JOIN document_comment_replies r ON r.id = ids."sourceId"`

// backfillDocumentCommentRevisions gives every comment and reply one
// revision of its current text. Earlier versions were overwritten and
// cannot be rebuilt.
func backfillDocumentCommentRevisions() *goose.Migration {
	up := func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCurrentRevisions(ctx, tx, "SELECT id FROM document_comments", _insertCommentRevisions); err != nil {
			return fmt.Errorf("backfilling comment revisions: %w", err)
		}

		if err := insertCurrentRevisions(ctx, tx, "SELECT id FROM document_comment_replies", _insertReplyRevisions); err != nil {
			// NOCOV: a test cannot make this step fail without failing
			// the one above first.
			return fmt.Errorf("backfilling reply revisions: %w", err)
		}

		return nil
	}

	// the revisions written since cannot be told apart from the backfilled
	// ones, so every revision goes, as it was before this migration.
	down := func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM document_comment_revisions"); err != nil {
			return fmt.Errorf("deleting comment revisions: %w", err)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM document_comment_reply_revisions"); err != nil {
			// NOCOV: a test cannot make this statement fail without
			// failing the one above first.
			return fmt.Errorf("deleting reply revisions: %w", err)
		}

		return nil
	}

	return goose.NewGoMigration(_versionDocumentCommentRevisionsBackfill, &goose.GoFunc{RunTx: up}, &goose.GoFunc{RunTx: down})
}

// insertCurrentRevisions mints a revision id for every row the select
// returns, then copies the rows in one insert, so only ids pass through
// memory.
func insertCurrentRevisions(ctx context.Context, tx *sql.Tx, selectIDs, insert string) error {
	rows, err := tx.QueryContext(ctx, selectIDs)
	if err != nil {
		return fmt.Errorf("selecting ids: %w", err)
	}

	defer rows.Close() //nolint:errcheck // rows.Err reports what went wrong

	// the pairs travel as one JSON parameter, because the database driver
	// is wrapped in a way that takes no array parameters.
	type pair struct {
		ID       string `json:"id"`
		SourceID string `json:"sourceId"`
	}

	pairs := make([]pair, 0)

	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			// NOCOV: the column is a NOT NULL primary key.
			return fmt.Errorf("scanning an id: %w", err)
		}

		pairs = append(pairs, pair{ID: xid.New().String(), SourceID: id})
	}

	if err = rows.Err(); err != nil {
		// NOCOV: a test cannot break the connection between two rows.
		return fmt.Errorf("reading ids: %w", err)
	}

	encoded, err := json.Marshal(pairs)
	if err != nil {
		// NOCOV: a slice of string pairs always encodes.
		return fmt.Errorf("encoding ids: %w", err)
	}

	if _, err := tx.ExecContext(ctx, insert, string(encoded)); err != nil {
		// NOCOV: a test cannot make this statement fail without failing
		// the select above first.
		return fmt.Errorf("inserting revisions: %w", err)
	}

	return nil
}
