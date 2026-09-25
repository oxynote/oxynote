-- +migrate Up

-- an edit folded into an entry moves updated_at and leaves created_at at
-- the time the entry was taken, which retention counts from. A new entry
-- is compared with the newest by name and icon, and NULL = NULL is not
-- true, so both become NOT NULL.
ALTER TABLE document_branch_history_entries ADD COLUMN updated_at TIMESTAMP NULL;

UPDATE document_branch_history_entries SET
	updated_at = created_at,
	document_name = COALESCE(document_name, ''),
	icon = COALESCE(icon, '');

ALTER TABLE document_branch_history_entries
	ALTER COLUMN updated_at SET NOT NULL,
	ALTER COLUMN document_name SET NOT NULL,
	ALTER COLUMN icon SET NOT NULL;

-- +migrate Down

ALTER TABLE document_branch_history_entries
	ALTER COLUMN icon DROP NOT NULL,
	ALTER COLUMN document_name DROP NOT NULL,
	DROP COLUMN updated_at;
