-- +goose Up

ALTER TABLE document_hooks ADD COLUMN cross_branch_id TEXT NULL;
UPDATE document_hooks SET cross_branch_id = id;
ALTER TABLE document_hooks ALTER COLUMN cross_branch_id SET NOT NULL;

-- +goose Down

ALTER TABLE document_hooks DROP COLUMN cross_branch_id;
