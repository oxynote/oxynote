-- +migrate Up

-- History entries become restorable snapshots: they carry the branch
-- name, icon, author and hook definitions and drop the Yjs binary. The
-- existing rows predate that shape and nothing reads them, so they are
-- dropped rather than backfilled.
TRUNCATE document_branch_history_entries;

ALTER TABLE document_branch_history_entries
	DROP COLUMN raw_content,
	ADD COLUMN document_name TEXT NULL,
	ADD COLUMN icon TEXT NULL,
	ADD COLUMN fk_last_updated_by TEXT REFERENCES users ON DELETE SET NULL,
	ADD COLUMN hooks JSONB NOT NULL,
	ADD COLUMN boundary BOOLEAN NOT NULL;
CREATE INDEX document_branch_history_entries_fk_last_updated_by_idx ON document_branch_history_entries (fk_last_updated_by);

-- Entries are read per branch, newest first, and expired by age; the
-- composite index also covers the plain branch lookup it replaces.
DROP INDEX document_branch_history_entries_fk_branch_id_idx;
CREATE INDEX document_branch_history_entries_fk_branch_id_created_at_idx ON document_branch_history_entries (fk_branch_id, created_at DESC, id DESC);
CREATE INDEX document_branch_history_entries_created_at_idx ON document_branch_history_entries (created_at);

-- +migrate Down

DROP INDEX document_branch_history_entries_created_at_idx;
DROP INDEX document_branch_history_entries_fk_branch_id_created_at_idx;
CREATE INDEX document_branch_history_entries_fk_branch_id_idx ON document_branch_history_entries (fk_branch_id);
DROP INDEX document_branch_history_entries_fk_last_updated_by_idx;

TRUNCATE document_branch_history_entries;

ALTER TABLE document_branch_history_entries
	DROP COLUMN boundary,
	DROP COLUMN hooks,
	DROP COLUMN fk_last_updated_by,
	DROP COLUMN icon,
	DROP COLUMN document_name,
	ADD COLUMN raw_content BYTEA NULL;
