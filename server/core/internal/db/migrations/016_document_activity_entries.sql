-- +goose Up

-- a target has no foreign key, so an entry outlives a deleted hook or tag.
CREATE TABLE document_activity_entries (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_branch_id TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	actor_kind TEXT NOT NULL,
	fk_actor_id TEXT NULL REFERENCES users ON DELETE SET NULL,
	actor_client_id TEXT NULL,
	category TEXT NOT NULL,
	type TEXT NOT NULL,
	target_kind TEXT NULL,
	target_id TEXT NULL,
	metadata JSONB NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
-- without the category index, a category with no entries scans the whole
-- branch.
CREATE INDEX document_activity_entries_fk_branch_id_created_at_idx ON document_activity_entries (fk_branch_id, created_at DESC, id DESC);
CREATE INDEX document_activity_entries_fk_branch_id_category_created_at_idx ON document_activity_entries (fk_branch_id, category, created_at DESC, id DESC);
CREATE INDEX document_activity_entries_fk_document_id_idx ON document_activity_entries (fk_document_id);
CREATE INDEX document_activity_entries_fk_organization_id_idx ON document_activity_entries (fk_organization_id);
CREATE INDEX document_activity_entries_fk_actor_id_idx ON document_activity_entries (fk_actor_id);

-- +goose Down

DROP TABLE document_activity_entries;
