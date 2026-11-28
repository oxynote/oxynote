-- +goose Up

-- a deleted comment or reply keeps its row, so its thread and the activity
-- that points at it stay readable. Its text leaves the row and lives on in
-- its revisions.
ALTER TABLE document_comments
	ALTER COLUMN content DROP NOT NULL,
	ADD COLUMN deleted_at TIMESTAMP NULL;

ALTER TABLE document_comment_replies
	ALTER COLUMN content DROP NOT NULL,
	ADD COLUMN deleted_at TIMESTAMP NULL;

-- a revision is the text of a comment or a reply as one write left it.
-- Creating and editing each add one. Migration 015 gives existing rows
-- their first.
CREATE TABLE document_comment_revisions (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_document_comment_id TEXT NOT NULL REFERENCES document_comments ON DELETE CASCADE,
	content JSONB NOT NULL,
	fk_user_id TEXT NULL REFERENCES users ON DELETE SET NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX document_comment_revisions_fk_document_comment_id_created_at_idx ON document_comment_revisions (fk_document_comment_id, created_at DESC, id DESC);
CREATE INDEX document_comment_revisions_fk_organization_id_idx ON document_comment_revisions (fk_organization_id);
CREATE INDEX document_comment_revisions_fk_user_id_idx ON document_comment_revisions (fk_user_id);

CREATE TABLE document_comment_reply_revisions (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_document_comment_reply_id TEXT NOT NULL REFERENCES document_comment_replies ON DELETE CASCADE,
	content JSONB NOT NULL,
	fk_user_id TEXT NULL REFERENCES users ON DELETE SET NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX document_comment_reply_revisions_fk_document_comment_reply_id_created_at_idx ON document_comment_reply_revisions (fk_document_comment_reply_id, created_at DESC, id DESC);
CREATE INDEX document_comment_reply_revisions_fk_organization_id_idx ON document_comment_reply_revisions (fk_organization_id);
CREATE INDEX document_comment_reply_revisions_fk_user_id_idx ON document_comment_reply_revisions (fk_user_id);

-- +goose Down

DROP TABLE document_comment_reply_revisions;
DROP TABLE document_comment_revisions;

-- a deleted row has no text, so it cannot return under NOT NULL. Deleting
-- a comment also takes its replies.
DELETE FROM document_comment_replies WHERE deleted_at IS NOT NULL;
DELETE FROM document_comments WHERE deleted_at IS NOT NULL;

ALTER TABLE document_comment_replies
	DROP COLUMN deleted_at,
	ALTER COLUMN content SET NOT NULL;

ALTER TABLE document_comments
	DROP COLUMN deleted_at,
	ALTER COLUMN content SET NOT NULL;
