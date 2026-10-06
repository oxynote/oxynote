-- +migrate Up

CREATE TABLE document_branch_views (
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_branch_id TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	viewed_at TIMESTAMP NOT NULL,
	PRIMARY KEY (fk_user_id, fk_branch_id)
);
CREATE INDEX document_branch_views_fk_user_id_viewed_at_idx ON document_branch_views (fk_user_id, fk_organization_id, viewed_at DESC);
CREATE INDEX document_branch_views_fk_organization_id_idx ON document_branch_views (fk_organization_id);
CREATE INDEX document_branch_views_fk_branch_id_idx ON document_branch_views (fk_branch_id);

-- +migrate Down

DROP TABLE document_branch_views;
