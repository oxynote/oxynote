-- +migrate Up

ALTER TABLE document_branch_views ADD COLUMN viewed_from TEXT NOT NULL DEFAULT 'all';
ALTER TABLE document_branch_views ALTER COLUMN viewed_from DROP DEFAULT;
ALTER TABLE document_branch_views DROP CONSTRAINT document_branch_views_pkey;
ALTER TABLE document_branch_views ADD PRIMARY KEY (fk_user_id, fk_branch_id, viewed_from);
DROP INDEX document_branch_views_fk_user_id_viewed_at_idx;
CREATE INDEX document_branch_views_fk_user_id_viewed_from_viewed_at_idx ON document_branch_views (fk_user_id, fk_organization_id, viewed_from, viewed_at DESC);

-- +migrate Down

DELETE FROM document_branch_views WHERE viewed_from <> 'all';
DROP INDEX document_branch_views_fk_user_id_viewed_from_viewed_at_idx;
ALTER TABLE document_branch_views DROP CONSTRAINT document_branch_views_pkey;
ALTER TABLE document_branch_views ADD PRIMARY KEY (fk_user_id, fk_branch_id);
ALTER TABLE document_branch_views DROP COLUMN viewed_from;
CREATE INDEX document_branch_views_fk_user_id_viewed_at_idx ON document_branch_views (fk_user_id, fk_organization_id, viewed_at DESC);
