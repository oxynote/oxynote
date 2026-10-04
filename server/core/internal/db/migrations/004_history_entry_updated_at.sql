-- +migrate Up

TRUNCATE document_branch_history_entries;

ALTER TABLE document_branch_history_entries
	ADD COLUMN updated_at TIMESTAMP NOT NULL,
	ADD COLUMN checksum TEXT NOT NULL;

-- +migrate Down

ALTER TABLE document_branch_history_entries
	DROP COLUMN checksum,
	DROP COLUMN updated_at;
