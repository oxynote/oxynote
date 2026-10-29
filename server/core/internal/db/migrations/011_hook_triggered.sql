-- +migrate Up

ALTER TABLE document_hooks ADD COLUMN triggered BOOLEAN NOT NULL DEFAULT false;
UPDATE document_hooks SET triggered = score = 0;
ALTER TABLE document_hooks DROP COLUMN score;

-- +migrate Down

ALTER TABLE document_hooks ADD COLUMN score INTEGER NOT NULL DEFAULT 100;
UPDATE document_hooks SET score = 0 WHERE triggered;
ALTER TABLE document_hooks ALTER COLUMN score DROP DEFAULT;
ALTER TABLE document_hooks DROP COLUMN triggered;
