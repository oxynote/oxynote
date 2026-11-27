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
-- Creating and editing each add one, so the newest revision always holds
-- the text the row had last.
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

-- an xid is 12 bytes in 20 base32hex characters, so the last character
-- carries a single bit and must be 0 or g.
-- +goose StatementBegin
CREATE FUNCTION pg_temp.random_xid() RETURNS TEXT LANGUAGE SQL VOLATILE AS $$
	SELECT string_agg(substr('0123456789abcdefghijklmnopqrstuv', floor(random() * 32)::INT + 1, 1), '')
		|| substr('0g', floor(random() * 2)::INT + 1, 1)
	FROM generate_series(1, 19)
$$;
-- +goose StatementEnd

-- every comment and reply starts with one revision of its current text.
-- Earlier versions were overwritten and cannot be rebuilt.
INSERT INTO document_comment_revisions (id, fk_organization_id, fk_document_comment_id, content, fk_user_id, created_at)
SELECT pg_temp.random_xid(), fk_organization_id, id, content, fk_user_id, COALESCE(updated_at, created_at)
FROM document_comments;

INSERT INTO document_comment_reply_revisions (id, fk_organization_id, fk_document_comment_reply_id, content, fk_user_id, created_at)
SELECT pg_temp.random_xid(), fk_organization_id, id, content, fk_user_id, COALESCE(updated_at, created_at)
FROM document_comment_replies;

DROP FUNCTION pg_temp.random_xid();

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
