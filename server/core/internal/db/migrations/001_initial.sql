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
	issuer TEXT NOT NULL,
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
CREATE UNIQUE INDEX user_accounts_issuer_account_id_idx ON user_accounts (issuer, account_id);

-- sessions live primarily in Valkey; the table exists because the OAuth
-- provider plugin requires database-backed sessions alongside secondary
-- storage.
CREATE TABLE user_sessions (
	id TEXT NOT NULL PRIMARY KEY,
	expires_at TIMESTAMPTZ NOT NULL,
	token TEXT NOT NULL UNIQUE,
	created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
	ip_address TEXT,
	user_agent TEXT,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	active_organization_id TEXT
);
CREATE INDEX user_sessions_fk_user_id_idx ON user_sessions (fk_user_id);

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

CREATE TABLE jwks (
	id TEXT NOT NULL PRIMARY KEY,
	public_key TEXT NOT NULL,
	private_key TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	expires_at TIMESTAMPTZ,
	alg TEXT,
	crv TEXT
);

CREATE TABLE oauth_clients (
	id TEXT NOT NULL PRIMARY KEY,
	client_id TEXT NOT NULL UNIQUE,
	client_secret TEXT,
	client_discovery_id TEXT,
	disabled BOOLEAN,
	skip_consent BOOLEAN,
	enable_end_session BOOLEAN,
	subject_type TEXT,
	scopes TEXT,
	client_credentials_scopes TEXT,
	fk_user_id TEXT REFERENCES users ON DELETE CASCADE,
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ,
	name TEXT,
	uri TEXT,
	icon TEXT,
	contacts TEXT,
	tos TEXT,
	policy TEXT,
	software_id TEXT,
	software_version TEXT,
	software_statement TEXT,
	redirect_uris TEXT NOT NULL,
	post_logout_redirect_uris TEXT,
	backchannel_logout_uri TEXT,
	backchannel_logout_session_required BOOLEAN,
	token_endpoint_auth_method TEXT,
	application_type TEXT,
	jwks TEXT,
	jwks_uri TEXT,
	grant_types TEXT,
	response_types TEXT,
	require_pkce BOOLEAN,
	dpop_bound_access_tokens BOOLEAN,
	reference_id TEXT,
	metadata JSONB
);
CREATE INDEX oauth_clients_fk_user_id_idx ON oauth_clients (fk_user_id);

CREATE TABLE oauth_resources (
	id TEXT NOT NULL PRIMARY KEY,
	identifier TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	access_token_ttl INTEGER,
	refresh_token_ttl INTEGER,
	signing_algorithm TEXT,
	signing_key_id TEXT,
	allowed_scopes TEXT,
	custom_claims JSONB,
	dpop_bound_access_tokens_required BOOLEAN,
	disabled BOOLEAN,
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ,
	policy_version INTEGER,
	metadata JSONB
);

CREATE TABLE oauth_client_resources (
	id TEXT NOT NULL PRIMARY KEY,
	client_id TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
	resource_id TEXT NOT NULL REFERENCES oauth_resources(identifier) ON DELETE CASCADE,
	metadata JSONB,
	created_at TIMESTAMPTZ
);
CREATE INDEX oauth_client_resources_client_id_idx ON oauth_client_resources (client_id);
CREATE INDEX oauth_client_resources_resource_id_idx ON oauth_client_resources (resource_id);

CREATE TABLE oauth_refresh_tokens (
	id TEXT NOT NULL PRIMARY KEY,
	token TEXT NOT NULL UNIQUE,
	client_id TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
	-- a token outlives the login session that authorized it, but must
	-- stop claiming to belong to it once that session is gone.
	session_id TEXT REFERENCES user_sessions ON DELETE SET NULL,
	fk_user_id TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
	reference_id TEXT,
	authorization_code_id TEXT,
	resources TEXT,
	requested_user_info_claims TEXT,
	expires_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ,
	revoked TIMESTAMPTZ,
	rotated_at TIMESTAMPTZ,
	rotation_replay_response TEXT,
	rotation_replay_expires_at TIMESTAMPTZ,
	auth_time TIMESTAMPTZ,
	confirmation JSONB,
	scopes TEXT NOT NULL
);
CREATE INDEX oauth_refresh_tokens_client_id_idx ON oauth_refresh_tokens (client_id);
CREATE INDEX oauth_refresh_tokens_fk_user_id_idx ON oauth_refresh_tokens (fk_user_id);
CREATE INDEX oauth_refresh_tokens_session_id_idx ON oauth_refresh_tokens (session_id);
CREATE INDEX oauth_refresh_tokens_authorization_code_id_idx ON oauth_refresh_tokens (authorization_code_id);

CREATE TABLE oauth_access_tokens (
	id TEXT NOT NULL PRIMARY KEY,
	token TEXT UNIQUE,
	client_id TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
	session_id TEXT REFERENCES user_sessions ON DELETE SET NULL,
	fk_user_id TEXT REFERENCES users ON DELETE CASCADE,
	reference_id TEXT,
	authorization_code_id TEXT,
	resources TEXT,
	requested_user_info_claims TEXT,
	fk_refresh_token_id TEXT REFERENCES oauth_refresh_tokens(id) ON DELETE SET NULL,
	expires_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ,
	revoked TIMESTAMPTZ,
	confirmation JSONB,
	scopes TEXT NOT NULL
);
CREATE INDEX oauth_access_tokens_client_id_idx ON oauth_access_tokens (client_id);
CREATE INDEX oauth_access_tokens_fk_user_id_idx ON oauth_access_tokens (fk_user_id);
CREATE INDEX oauth_access_tokens_session_id_idx ON oauth_access_tokens (session_id);
CREATE INDEX oauth_access_tokens_authorization_code_id_idx ON oauth_access_tokens (authorization_code_id);
-- refresh-token rotation deletes every access token minted from the
-- token being rotated, keyed on this column, so it runs on every refresh.
CREATE INDEX oauth_access_tokens_fk_refresh_token_id_idx ON oauth_access_tokens (fk_refresh_token_id);

CREATE TABLE oauth_consents (
	id TEXT NOT NULL PRIMARY KEY,
	client_id TEXT NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
	fk_user_id TEXT REFERENCES users ON DELETE CASCADE,
	reference_id TEXT,
	resources TEXT,
	requested_user_info_claims TEXT,
	scopes TEXT NOT NULL,
	created_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ
);
CREATE INDEX oauth_consents_client_id_fk_user_id_idx ON oauth_consents (client_id, fk_user_id);

CREATE TABLE oauth_client_assertions (
	id TEXT NOT NULL PRIMARY KEY,
	expires_at TIMESTAMPTZ NOT NULL
);

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
	CONSTRAINT documents_sort_index_key
		UNIQUE NULLS NOT DISTINCT (fk_organization_id, fk_parent_id, sort_index)
		DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX documents_fk_organization_id_idx ON documents (fk_organization_id);
CREATE INDEX documents_fk_parent_id_idx ON documents (fk_parent_id);
CREATE INDEX documents_fk_created_by_idx ON documents (fk_created_by);
CREATE INDEX documents_fk_last_updated_by_idx ON documents (fk_last_updated_by);

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
CREATE INDEX document_branches_fk_created_by_idx ON document_branches (fk_created_by);
CREATE INDEX document_branches_fk_last_updated_by_idx ON document_branches (fk_last_updated_by);

CREATE TABLE document_branch_changelogs (
	id TEXT PRIMARY KEY,
	fk_document_id TEXT NOT NULL REFERENCES documents ON DELETE CASCADE,
	fk_branch_id TEXT NOT NULL REFERENCES document_branches(id) ON DELETE CASCADE,
	content JSONB NOT NULL,
	raw_content BYTEA NULL,
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX document_branch_changelogs_fk_document_id_idx ON document_branch_changelogs (fk_document_id);
CREATE INDEX document_branch_changelogs_fk_branch_id_idx ON document_branch_changelogs (fk_branch_id);

CREATE TABLE document_hooks (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	fk_document_id TEXT REFERENCES documents ON DELETE SET NULL,
	fk_organization_id TEXT REFERENCES organizations ON DELETE SET NULL,
	fk_branch_id TEXT REFERENCES document_branches(id) ON DELETE SET NULL,
	block_id TEXT,
	settings JSONB NOT NULL,
	state TEXT NOT NULL,
	score INTEGER NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP,
	soft_deleted_at TIMESTAMP
);
CREATE INDEX document_hooks_fk_document_id_idx ON document_hooks (fk_document_id);
CREATE INDEX document_hooks_fk_branch_id_idx ON document_hooks (fk_branch_id);

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
CREATE INDEX document_comments_fk_branch_id_idx ON document_comments (fk_branch_id);
CREATE INDEX document_comments_fk_user_id_idx ON document_comments (fk_user_id);
CREATE INDEX document_comments_fk_resolved_by_idx ON document_comments (fk_resolved_by);

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
CREATE INDEX document_comment_replies_fk_user_id_idx ON document_comment_replies (fk_user_id);

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
CREATE INDEX branch_reviewers_fk_organization_id_idx ON branch_reviewers (fk_organization_id);
CREATE INDEX branch_reviewers_fk_user_id_idx ON branch_reviewers (fk_user_id);

CREATE TABLE document_files (
	id TEXT PRIMARY KEY,
	location TEXT NOT NULL,
	storage_key TEXT NOT NULL,
	fk_document_id TEXT REFERENCES documents ON DELETE SET NULL,
	fk_organization_id TEXT REFERENCES organizations ON DELETE SET NULL,
	created_at TIMESTAMP NOT NULL,
	unreferenced_at TIMESTAMP
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
CREATE INDEX slack_messages_fk_organization_id_created_at_idx ON slack_messages (fk_organization_id, created_at DESC);

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
CREATE INDEX notifications_fk_user_id_idx ON notifications (fk_user_id);

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
DROP TABLE oauth_client_assertions;
DROP TABLE oauth_consents;
DROP TABLE oauth_access_tokens;
DROP TABLE oauth_refresh_tokens;
DROP TABLE oauth_client_resources;
DROP TABLE oauth_resources;
DROP TABLE oauth_clients;
DROP TABLE jwks;
DROP TABLE organization_invitations;
DROP TABLE organization_members;
DROP TABLE organizations;
DROP TABLE user_verifications;
DROP TABLE user_sessions;
DROP TABLE user_accounts;
DROP TABLE users;
