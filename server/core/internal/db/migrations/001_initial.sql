-- +migrate Up

-- Better-auth-owned identity tables.

CREATE TABLE users (
	id TEXT NOT NULL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	email_verified BOOLEAN NOT NULL,
	image TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE user_accounts (
	id TEXT NOT NULL PRIMARY KEY,
	account_id TEXT NOT NULL,
	provider_id TEXT NOT NULL,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	access_token TEXT,
	refresh_token TEXT,
	id_token TEXT,
	access_token_expires_at TIMESTAMPTZ,
	refresh_token_expires_at TIMESTAMPTZ,
	scope TEXT,
	password TEXT,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX user_accounts_user_id_idx ON user_accounts (fk_user_id);

CREATE TABLE user_verifications (
	id TEXT NOT NULL PRIMARY KEY,
	identifier TEXT NOT NULL,
	value TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE INDEX user_verifications_identifier_idx ON user_verifications (identifier);

CREATE TABLE organizations (
	id TEXT NOT NULL PRIMARY KEY,
	name TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	logo TEXT,
	created_at TIMESTAMPTZ NOT NULL,
	metadata TEXT
);

CREATE TABLE organization_members (
	id TEXT NOT NULL PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	role TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX organization_members_user_id_idx ON organization_members (fk_user_id);
CREATE INDEX organization_members_organization_id_idx ON organization_members (fk_organization_id);

CREATE TABLE organization_invitations (
	id TEXT NOT NULL PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	email TEXT NOT NULL,
	role TEXT,
	status TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	fk_inviter_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE
);
CREATE INDEX organization_invitations_email_idx ON organization_invitations (email);
CREATE INDEX organization_invitations_organization_id_idx ON organization_invitations (fk_organization_id);

-- Data sources (Prometheus, SQL).

CREATE TABLE data_sources (
	id TEXT NOT NULL PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	url TEXT NOT NULL,
	credentials BYTEA NOT NULL,
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ,
	UNIQUE(fk_organization_id, name)
);
CREATE INDEX data_sources_fk_organization_id_idx ON data_sources (fk_organization_id);

-- Document tree. Per-branch content lives in document_branches; the
-- documents row only carries identity, parent, and audit fields.

CREATE TABLE documents (
	id TEXT PRIMARY KEY,
	sort_index INTEGER NOT NULL,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_parent_id TEXT REFERENCES documents ON DELETE CASCADE,
	created_at TIMESTAMP NOT NULL,
	fk_created_by TEXT REFERENCES users ON DELETE SET NULL,
	updated_at TIMESTAMP NOT NULL,
	fk_last_updated_by TEXT REFERENCES users ON DELETE SET NULL,
	UNIQUE(fk_parent_id, sort_index) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX documents_fk_organization_id_idx ON documents (fk_organization_id);
CREATE INDEX documents_fk_parent_id_idx ON documents (fk_parent_id);

CREATE TABLE document_branches (
	id TEXT NOT NULL,
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	branch_name TEXT NOT NULL,
	document_name TEXT NULL,
	icon TEXT NULL,
	content JSONB NULL,
	raw_content BYTEA NULL,
	protected BOOLEAN NOT NULL DEFAULT FALSE,
	"default" BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMP NOT NULL,
	fk_created_by TEXT REFERENCES users ON DELETE SET NULL,
	updated_at TIMESTAMP NOT NULL,
	fk_last_updated_by TEXT REFERENCES users ON DELETE SET NULL,
	PRIMARY KEY (id),
	UNIQUE (fk_document_id, branch_name)
);
CREATE INDEX document_branches_fk_organization_id_idx ON document_branches (fk_organization_id);

CREATE TABLE document_branch_changelogs (
	id TEXT PRIMARY KEY,
	fk_document_id TEXT NOT NULL,
	fk_branch_name TEXT NOT NULL,
	content JSONB NOT NULL,
	raw_content BYTEA NULL,
	created_at TIMESTAMP NOT NULL,
	FOREIGN KEY (fk_document_id, fk_branch_name) REFERENCES document_branches (fk_document_id, branch_name) ON DELETE CASCADE
);
CREATE INDEX document_branch_changelogs_fk_document_id_fk_branch_name_idx ON document_branch_changelogs (fk_document_id, fk_branch_name);

CREATE TABLE document_hooks (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_branch_id TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	block_id TEXT,
	settings JSONB NOT NULL,
	state TEXT NOT NULL,
	score INTEGER NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP,
	soft_deleted_at TIMESTAMP
);
CREATE INDEX document_hooks_fk_document_id_idx ON document_hooks (fk_document_id);

CREATE TABLE document_comments (
	id TEXT PRIMARY KEY,
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_branch_id TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	fk_user_id TEXT REFERENCES users ON DELETE SET NULL,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_resolved_by TEXT REFERENCES users ON DELETE SET NULL,
	anchor_block_id TEXT,
	resolved BOOLEAN NOT NULL DEFAULT FALSE,
	content JSONB NOT NULL,
	diff_deletion_context JSONB,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP
);
CREATE INDEX document_comments_fk_document_id_idx ON document_comments (fk_document_id);

CREATE TABLE document_comment_replies (
	id TEXT PRIMARY KEY,
	fk_document_comment_id TEXT NOT NULL REFERENCES document_comments ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	fk_user_id TEXT REFERENCES users ON DELETE SET NULL,
	content JSONB NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP
);
CREATE INDEX document_comment_replies_fk_document_comment_id_idx ON document_comment_replies (fk_document_comment_id);

CREATE TABLE document_maintainers (
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	PRIMARY KEY (fk_document_id, fk_user_id)
);
CREATE INDEX document_maintainers_fk_document_id_idx ON document_maintainers (fk_document_id);

CREATE TABLE branch_reviewers (
	fk_branch_id        TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	fk_user_id          TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	fk_organization_id  TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	currently_approved  BOOLEAN NOT NULL DEFAULT FALSE,
	previously_approved BOOLEAN NOT NULL DEFAULT FALSE,
	PRIMARY KEY (fk_branch_id, fk_user_id)
);
CREATE INDEX branch_reviewers_fk_branch_id_idx ON branch_reviewers (fk_branch_id);
CREATE INDEX branch_reviewers_fk_organization_id_idx ON branch_reviewers (fk_organization_id);

CREATE TABLE document_files (
	id TEXT PRIMARY KEY,
	location TEXT NOT NULL,
	fk_document_id TEXT REFERENCES documents ON DELETE SET NULL,
	fk_organization_id TEXT REFERENCES organizations ON DELETE SET NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX document_files_fk_document_id_idx ON document_files (fk_document_id);
CREATE INDEX document_files_fk_organization_id_idx ON document_files (fk_organization_id);

CREATE TABLE document_search_jobs (
	id SERIAL PRIMARY KEY,
	block_diff JSONB NOT NULL
);

-- Slack / GitHub integrations and the per-org notification queue.

CREATE TABLE slack_apps (
	team_id TEXT PRIMARY KEY,
	fk_organization_id TEXT REFERENCES organizations ON DELETE SET NULL,
	token TEXT NOT NULL
);
CREATE INDEX slack_apps_fk_organization_id_idx ON slack_apps (fk_organization_id);

CREATE TABLE slack_user_links (
	slack_user_id TEXT NOT NULL,
	fk_team_id TEXT NOT NULL REFERENCES slack_apps(team_id) ON DELETE CASCADE,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	settings BYTEA NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	PRIMARY KEY (slack_user_id, fk_team_id)
);
CREATE INDEX slack_user_links_fk_user_id_idx ON slack_user_links (fk_user_id);
CREATE INDEX slack_user_links_fk_team_id_idx ON slack_user_links (fk_team_id);
CREATE INDEX slack_user_links_fk_team_id_fk_user_id_idx ON slack_user_links (fk_team_id, fk_user_id);

CREATE TABLE github_installations (
	installation_id NUMERIC PRIMARY KEY,
	fk_organization_id TEXT REFERENCES organizations ON DELETE SET NULL
);
CREATE INDEX github_installations_fk_organization_id_idx ON github_installations (fk_organization_id);

CREATE TABLE slack_messages (
	id TEXT PRIMARY KEY,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	TEXT TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX slack_messages_fk_organization_id_idx ON slack_messages (fk_organization_id);

CREATE TABLE notifications (
	id TEXT PRIMARY KEY,
	code TEXT NOT NULL,
	metadata JSONB NOT NULL,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	fk_organization_id TEXT NOT NULL REFERENCES organizations ON DELETE CASCADE,
	read BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX notifications_fk_organization_id_fk_user_id_idx ON notifications (fk_organization_id, fk_user_id);

-- +migrate Down
DROP TABLE notifications;
DROP TABLE slack_messages;
DROP TABLE github_installations;
DROP TABLE slack_user_links;
DROP TABLE slack_apps;
DROP TABLE document_search_jobs;
DROP TABLE document_files;
DROP TABLE branch_reviewers;
DROP TABLE document_maintainers;
DROP TABLE document_comment_replies;
DROP TABLE document_comments;
DROP TABLE document_hooks;
DROP TABLE document_branch_changelogs;
DROP TABLE document_branches;
DROP TABLE documents;
DROP TABLE data_sources;
DROP TABLE organization_invitations;
DROP TABLE organization_members;
DROP TABLE organizations;
DROP TABLE user_verifications;
DROP TABLE user_accounts;
DROP TABLE users;
DROP EXTENSION IF EXISTS vector;
