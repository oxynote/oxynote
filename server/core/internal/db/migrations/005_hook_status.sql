-- +migrate Up

ALTER TABLE document_hooks ALTER COLUMN state DROP NOT NULL;
ALTER TABLE document_hooks ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

UPDATE document_hooks SET
	status = COALESCE(state::jsonb->>'status', 'active'),
	state = (state::jsonb - 'status')::text;

-- +migrate Down

UPDATE document_hooks SET state = '{}' WHERE state IS NULL;

UPDATE document_hooks SET state = jsonb_set(state::jsonb, '{status}', to_jsonb(status))::text
WHERE type <> 'scheduled-reminder';

ALTER TABLE document_hooks DROP COLUMN status;
ALTER TABLE document_hooks ALTER COLUMN state SET NOT NULL;
