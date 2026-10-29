-- generated from migrations/ by Test_Schema. Do not edit; run `make schema` in server/core.

CREATE TABLE public.branch_reviewers (
    fk_branch_id text NOT NULL,
    fk_user_id text NOT NULL,
    fk_organization_id text NOT NULL,
    currently_approved boolean DEFAULT false NOT NULL,
    previously_approved boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY public.branch_reviewers
    ADD CONSTRAINT branch_reviewers_pkey PRIMARY KEY (fk_branch_id, fk_user_id);

CREATE INDEX branch_reviewers_fk_organization_id_idx ON public.branch_reviewers USING btree (fk_organization_id);

CREATE INDEX branch_reviewers_fk_user_id_idx ON public.branch_reviewers USING btree (fk_user_id);

ALTER TABLE ONLY public.branch_reviewers
    ADD CONSTRAINT branch_reviewers_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.branch_reviewers
    ADD CONSTRAINT branch_reviewers_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.branch_reviewers
    ADD CONSTRAINT branch_reviewers_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.data_sources (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    url text NOT NULL,
    credentials bytea NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_fk_organization_id_name_key UNIQUE (fk_organization_id, name);

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.document_branch_history_entries (
    id text NOT NULL,
    fk_document_id text NOT NULL,
    fk_branch_id text NOT NULL,
    content jsonb NOT NULL,
    created_at timestamp without time zone NOT NULL,
    document_name text,
    icon text,
    fk_last_updated_by text,
    hooks jsonb NOT NULL,
    boundary boolean NOT NULL,
    updated_at timestamp without time zone NOT NULL,
    checksum text NOT NULL
);

ALTER TABLE ONLY public.document_branch_history_entries
    ADD CONSTRAINT document_branch_history_entries_pkey PRIMARY KEY (id);

CREATE INDEX document_branch_history_entries_created_at_idx ON public.document_branch_history_entries USING btree (created_at);

CREATE INDEX document_branch_history_entries_fk_branch_id_created_at_idx ON public.document_branch_history_entries USING btree (fk_branch_id, created_at DESC, id DESC);

CREATE INDEX document_branch_history_entries_fk_document_id_idx ON public.document_branch_history_entries USING btree (fk_document_id);

CREATE INDEX document_branch_history_entries_fk_last_updated_by_idx ON public.document_branch_history_entries USING btree (fk_last_updated_by);

ALTER TABLE ONLY public.document_branch_history_entries
    ADD CONSTRAINT document_branch_history_entries_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_history_entries
    ADD CONSTRAINT document_branch_history_entries_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_history_entries
    ADD CONSTRAINT document_branch_history_entries_fk_last_updated_by_fkey FOREIGN KEY (fk_last_updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

CREATE TABLE public.document_branch_tags (
    fk_branch_id text NOT NULL,
    fk_tag_id text NOT NULL,
    fk_organization_id text NOT NULL
);

ALTER TABLE ONLY public.document_branch_tags
    ADD CONSTRAINT document_branch_tags_pkey PRIMARY KEY (fk_branch_id, fk_tag_id);

CREATE INDEX document_branch_tags_fk_organization_id_idx ON public.document_branch_tags USING btree (fk_organization_id);

CREATE INDEX document_branch_tags_fk_tag_id_idx ON public.document_branch_tags USING btree (fk_tag_id);

ALTER TABLE ONLY public.document_branch_tags
    ADD CONSTRAINT document_branch_tags_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_tags
    ADD CONSTRAINT document_branch_tags_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_tags
    ADD CONSTRAINT document_branch_tags_fk_tag_id_fkey FOREIGN KEY (fk_tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;

CREATE TABLE public.document_branch_views (
    fk_user_id text NOT NULL,
    fk_organization_id text NOT NULL,
    fk_branch_id text NOT NULL,
    viewed_at timestamp without time zone NOT NULL,
    viewed_from text NOT NULL
);

ALTER TABLE ONLY public.document_branch_views
    ADD CONSTRAINT document_branch_views_pkey PRIMARY KEY (fk_user_id, fk_branch_id, viewed_from);

CREATE INDEX document_branch_views_fk_branch_id_idx ON public.document_branch_views USING btree (fk_branch_id);

CREATE INDEX document_branch_views_fk_organization_id_idx ON public.document_branch_views USING btree (fk_organization_id);

CREATE INDEX document_branch_views_fk_user_id_viewed_from_viewed_at_idx ON public.document_branch_views USING btree (fk_user_id, fk_organization_id, viewed_from, viewed_at DESC);

ALTER TABLE ONLY public.document_branch_views
    ADD CONSTRAINT document_branch_views_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_views
    ADD CONSTRAINT document_branch_views_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branch_views
    ADD CONSTRAINT document_branch_views_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.document_branches (
    id text NOT NULL,
    fk_document_id text NOT NULL,
    fk_organization_id text NOT NULL,
    branch_name text NOT NULL,
    document_name text,
    icon text,
    content jsonb,
    raw_content bytea,
    protected boolean DEFAULT false NOT NULL,
    "default" boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone NOT NULL,
    fk_created_by text,
    updated_at timestamp without time zone NOT NULL,
    fk_last_updated_by text
);

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_fk_document_id_branch_name_key UNIQUE (fk_document_id, branch_name);

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_pkey PRIMARY KEY (id);

CREATE INDEX document_branches_fk_created_by_idx ON public.document_branches USING btree (fk_created_by);

CREATE INDEX document_branches_fk_last_updated_by_idx ON public.document_branches USING btree (fk_last_updated_by);

CREATE INDEX document_branches_fk_organization_id_idx ON public.document_branches USING btree (fk_organization_id);

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_fk_created_by_fkey FOREIGN KEY (fk_created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_fk_last_updated_by_fkey FOREIGN KEY (fk_last_updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_branches
    ADD CONSTRAINT document_branches_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.document_comment_replies (
    id text NOT NULL,
    fk_document_comment_id text NOT NULL,
    fk_organization_id text NOT NULL,
    fk_user_id text,
    content jsonb NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone
);

ALTER TABLE ONLY public.document_comment_replies
    ADD CONSTRAINT document_comment_replies_pkey PRIMARY KEY (id);

CREATE INDEX document_comment_replies_fk_document_comment_id_idx ON public.document_comment_replies USING btree (fk_document_comment_id);

CREATE INDEX document_comment_replies_fk_user_id_idx ON public.document_comment_replies USING btree (fk_user_id);

ALTER TABLE ONLY public.document_comment_replies
    ADD CONSTRAINT document_comment_replies_fk_document_comment_id_fkey FOREIGN KEY (fk_document_comment_id) REFERENCES public.document_comments(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_comment_replies
    ADD CONSTRAINT document_comment_replies_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_comment_replies
    ADD CONSTRAINT document_comment_replies_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

CREATE TABLE public.document_comments (
    id text NOT NULL,
    fk_document_id text NOT NULL,
    fk_branch_id text NOT NULL,
    fk_user_id text,
    fk_organization_id text NOT NULL,
    fk_resolved_by text,
    resolved_at timestamp without time zone,
    anchor_block_id text,
    resolved boolean DEFAULT false NOT NULL,
    content jsonb NOT NULL,
    diff_deletion_context jsonb,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone
);

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_pkey PRIMARY KEY (id);

CREATE INDEX document_comments_fk_branch_id_idx ON public.document_comments USING btree (fk_branch_id);

CREATE INDEX document_comments_fk_document_id_idx ON public.document_comments USING btree (fk_document_id);

CREATE INDEX document_comments_fk_resolved_by_idx ON public.document_comments USING btree (fk_resolved_by);

CREATE INDEX document_comments_fk_user_id_idx ON public.document_comments USING btree (fk_user_id);

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_fk_resolved_by_fkey FOREIGN KEY (fk_resolved_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_comments
    ADD CONSTRAINT document_comments_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

CREATE TABLE public.document_files (
    id text NOT NULL,
    location text NOT NULL,
    storage_key text NOT NULL,
    fk_document_id text,
    fk_organization_id text,
    name text NOT NULL,
    size bigint NOT NULL,
    content_type text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    unreferenced_at timestamp without time zone
);

ALTER TABLE ONLY public.document_files
    ADD CONSTRAINT document_files_pkey PRIMARY KEY (id);

CREATE INDEX document_files_fk_document_id_idx ON public.document_files USING btree (fk_document_id);

CREATE INDEX document_files_fk_organization_id_idx ON public.document_files USING btree (fk_organization_id);

ALTER TABLE ONLY public.document_files
    ADD CONSTRAINT document_files_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_files
    ADD CONSTRAINT document_files_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;

CREATE TABLE public.document_hooks (
    id text NOT NULL,
    type text NOT NULL,
    fk_document_id text,
    fk_organization_id text,
    fk_branch_id text,
    block_id text,
    settings jsonb NOT NULL,
    state text,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone,
    soft_deleted_at timestamp without time zone,
    cross_branch_id text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    triggered boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY public.document_hooks
    ADD CONSTRAINT document_hooks_pkey PRIMARY KEY (id);

CREATE INDEX document_hooks_fk_branch_id_idx ON public.document_hooks USING btree (fk_branch_id);

CREATE INDEX document_hooks_fk_document_id_idx ON public.document_hooks USING btree (fk_document_id);

ALTER TABLE ONLY public.document_hooks
    ADD CONSTRAINT document_hooks_fk_branch_id_fkey FOREIGN KEY (fk_branch_id) REFERENCES public.document_branches(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_hooks
    ADD CONSTRAINT document_hooks_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.document_hooks
    ADD CONSTRAINT document_hooks_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;

CREATE TABLE public.document_maintainers (
    fk_document_id text NOT NULL,
    fk_user_id text NOT NULL,
    fk_organization_id text NOT NULL
);

ALTER TABLE ONLY public.document_maintainers
    ADD CONSTRAINT document_maintainers_pkey PRIMARY KEY (fk_document_id, fk_user_id);

CREATE INDEX document_maintainers_fk_document_id_idx ON public.document_maintainers USING btree (fk_document_id);

ALTER TABLE ONLY public.document_maintainers
    ADD CONSTRAINT document_maintainers_fk_document_id_fkey FOREIGN KEY (fk_document_id) REFERENCES public.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_maintainers
    ADD CONSTRAINT document_maintainers_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.document_maintainers
    ADD CONSTRAINT document_maintainers_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.documents (
    id text NOT NULL,
    sort_index integer NOT NULL,
    fk_organization_id text NOT NULL,
    fk_parent_id text,
    created_at timestamp without time zone NOT NULL,
    fk_created_by text,
    updated_at timestamp without time zone NOT NULL,
    fk_last_updated_by text
);

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_sort_index_key UNIQUE NULLS NOT DISTINCT (fk_organization_id, fk_parent_id, sort_index) DEFERRABLE;

CREATE INDEX documents_fk_created_by_idx ON public.documents USING btree (fk_created_by);

CREATE INDEX documents_fk_last_updated_by_idx ON public.documents USING btree (fk_last_updated_by);

CREATE INDEX documents_fk_organization_id_idx ON public.documents USING btree (fk_organization_id);

CREATE INDEX documents_fk_parent_id_idx ON public.documents USING btree (fk_parent_id);

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_fk_created_by_fkey FOREIGN KEY (fk_created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_fk_last_updated_by_fkey FOREIGN KEY (fk_last_updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_fk_parent_id_fkey FOREIGN KEY (fk_parent_id) REFERENCES public.documents(id) ON DELETE CASCADE;

CREATE TABLE public.github_installations (
    installation_id numeric NOT NULL,
    fk_organization_id text
);

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT github_installations_pkey PRIMARY KEY (installation_id);

CREATE INDEX github_installations_fk_organization_id_idx ON public.github_installations USING btree (fk_organization_id);

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT github_installations_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;

CREATE TABLE public.jwks (
    id text NOT NULL,
    public_key text NOT NULL,
    private_key text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    alg text,
    crv text
);

ALTER TABLE ONLY public.jwks
    ADD CONSTRAINT jwks_pkey PRIMARY KEY (id);

CREATE TABLE public.notifications (
    id text NOT NULL,
    code text NOT NULL,
    metadata jsonb NOT NULL,
    fk_user_id text NOT NULL,
    fk_organization_id text NOT NULL,
    read boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

CREATE INDEX notifications_fk_organization_id_fk_user_id_idx ON public.notifications USING btree (fk_organization_id, fk_user_id);

CREATE INDEX notifications_fk_user_id_idx ON public.notifications USING btree (fk_user_id);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.oauth_access_tokens (
    id text NOT NULL,
    token text,
    client_id text NOT NULL,
    session_id text,
    fk_user_id text,
    reference_id text,
    authorization_code_id text,
    resources text,
    requested_user_info_claims text,
    fk_refresh_token_id text,
    expires_at timestamp with time zone,
    created_at timestamp with time zone,
    revoked timestamp with time zone,
    confirmation jsonb,
    scopes text NOT NULL
);

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_token_key UNIQUE (token);

CREATE INDEX oauth_access_tokens_authorization_code_id_idx ON public.oauth_access_tokens USING btree (authorization_code_id);

CREATE INDEX oauth_access_tokens_client_id_idx ON public.oauth_access_tokens USING btree (client_id);

CREATE INDEX oauth_access_tokens_fk_refresh_token_id_idx ON public.oauth_access_tokens USING btree (fk_refresh_token_id);

CREATE INDEX oauth_access_tokens_fk_user_id_idx ON public.oauth_access_tokens USING btree (fk_user_id);

CREATE INDEX oauth_access_tokens_session_id_idx ON public.oauth_access_tokens USING btree (session_id);

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_fk_refresh_token_id_fkey FOREIGN KEY (fk_refresh_token_id) REFERENCES public.oauth_refresh_tokens(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_access_tokens
    ADD CONSTRAINT oauth_access_tokens_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.user_sessions(id) ON DELETE SET NULL;

CREATE TABLE public.oauth_client_assertions (
    id text NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.oauth_client_assertions
    ADD CONSTRAINT oauth_client_assertions_pkey PRIMARY KEY (id);

CREATE TABLE public.oauth_client_resources (
    id text NOT NULL,
    client_id text NOT NULL,
    resource_id text NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.oauth_client_resources
    ADD CONSTRAINT oauth_client_resources_pkey PRIMARY KEY (id);

CREATE INDEX oauth_client_resources_client_id_idx ON public.oauth_client_resources USING btree (client_id);

CREATE INDEX oauth_client_resources_resource_id_idx ON public.oauth_client_resources USING btree (resource_id);

ALTER TABLE ONLY public.oauth_client_resources
    ADD CONSTRAINT oauth_client_resources_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_client_resources
    ADD CONSTRAINT oauth_client_resources_resource_id_fkey FOREIGN KEY (resource_id) REFERENCES public.oauth_resources(identifier) ON DELETE CASCADE;

CREATE TABLE public.oauth_clients (
    id text NOT NULL,
    client_id text NOT NULL,
    client_secret text,
    client_discovery_id text,
    disabled boolean,
    skip_consent boolean,
    enable_end_session boolean,
    subject_type text,
    scopes text,
    client_credentials_scopes text,
    fk_user_id text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text,
    uri text,
    icon text,
    contacts text,
    tos text,
    policy text,
    software_id text,
    software_version text,
    software_statement text,
    redirect_uris text NOT NULL,
    post_logout_redirect_uris text,
    backchannel_logout_uri text,
    backchannel_logout_session_required boolean,
    token_endpoint_auth_method text,
    application_type text,
    jwks text,
    jwks_uri text,
    grant_types text,
    response_types text,
    require_pkce boolean,
    dpop_bound_access_tokens boolean,
    reference_id text,
    metadata jsonb
);

ALTER TABLE ONLY public.oauth_clients
    ADD CONSTRAINT oauth_clients_client_id_key UNIQUE (client_id);

ALTER TABLE ONLY public.oauth_clients
    ADD CONSTRAINT oauth_clients_pkey PRIMARY KEY (id);

CREATE INDEX oauth_clients_fk_user_id_idx ON public.oauth_clients USING btree (fk_user_id);

ALTER TABLE ONLY public.oauth_clients
    ADD CONSTRAINT oauth_clients_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.oauth_consents (
    id text NOT NULL,
    client_id text NOT NULL,
    fk_user_id text,
    reference_id text,
    resources text,
    requested_user_info_claims text,
    scopes text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.oauth_consents
    ADD CONSTRAINT oauth_consents_pkey PRIMARY KEY (id);

CREATE INDEX oauth_consents_client_id_fk_user_id_idx ON public.oauth_consents USING btree (client_id, fk_user_id);

ALTER TABLE ONLY public.oauth_consents
    ADD CONSTRAINT oauth_consents_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_consents
    ADD CONSTRAINT oauth_consents_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.oauth_refresh_tokens (
    id text NOT NULL,
    token text NOT NULL,
    client_id text NOT NULL,
    session_id text,
    fk_user_id text NOT NULL,
    reference_id text,
    authorization_code_id text,
    resources text,
    requested_user_info_claims text,
    expires_at timestamp with time zone,
    created_at timestamp with time zone,
    revoked timestamp with time zone,
    rotated_at timestamp with time zone,
    rotation_replay_response text,
    rotation_replay_expires_at timestamp with time zone,
    auth_time timestamp with time zone,
    confirmation jsonb,
    scopes text NOT NULL
);

ALTER TABLE ONLY public.oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_token_key UNIQUE (token);

CREATE INDEX oauth_refresh_tokens_authorization_code_id_idx ON public.oauth_refresh_tokens USING btree (authorization_code_id);

CREATE INDEX oauth_refresh_tokens_client_id_idx ON public.oauth_refresh_tokens USING btree (client_id);

CREATE INDEX oauth_refresh_tokens_fk_user_id_idx ON public.oauth_refresh_tokens USING btree (fk_user_id);

CREATE INDEX oauth_refresh_tokens_session_id_idx ON public.oauth_refresh_tokens USING btree (session_id);

ALTER TABLE ONLY public.oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.user_sessions(id) ON DELETE SET NULL;

CREATE TABLE public.oauth_resources (
    id text NOT NULL,
    identifier text NOT NULL,
    name text NOT NULL,
    access_token_ttl integer,
    refresh_token_ttl integer,
    signing_algorithm text,
    signing_key_id text,
    allowed_scopes text,
    custom_claims jsonb,
    dpop_bound_access_tokens_required boolean,
    disabled boolean,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    policy_version integer,
    metadata jsonb
);

ALTER TABLE ONLY public.oauth_resources
    ADD CONSTRAINT oauth_resources_identifier_key UNIQUE (identifier);

ALTER TABLE ONLY public.oauth_resources
    ADD CONSTRAINT oauth_resources_pkey PRIMARY KEY (id);

CREATE TABLE public.organization_invitations (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    email text NOT NULL,
    role text,
    status text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    fk_inviter_id text NOT NULL
);

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_pkey PRIMARY KEY (id);

CREATE INDEX organization_invitations_email_idx ON public.organization_invitations USING btree (email);

CREATE INDEX organization_invitations_organization_id_idx ON public.organization_invitations USING btree (fk_organization_id);

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_fk_inviter_id_fkey FOREIGN KEY (fk_inviter_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.organization_invitations
    ADD CONSTRAINT organization_invitations_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.organization_members (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    fk_user_id text NOT NULL,
    role text NOT NULL,
    created_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_pkey PRIMARY KEY (id);

CREATE INDEX organization_members_organization_id_idx ON public.organization_members USING btree (fk_organization_id);

CREATE INDEX organization_members_user_id_idx ON public.organization_members USING btree (fk_user_id);

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.organization_members
    ADD CONSTRAINT organization_members_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.organizations (
    id text NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    logo text,
    created_at timestamp with time zone NOT NULL,
    metadata text
);

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_slug_key UNIQUE (slug);

CREATE TABLE public.personal_variable_groups (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    group_name text NOT NULL,
    color text NOT NULL,
    sort_index integer NOT NULL,
    builtin boolean NOT NULL,
    created_at timestamp without time zone NOT NULL,
    fk_created_by text
);

ALTER TABLE ONLY public.personal_variable_groups
    ADD CONSTRAINT personal_variable_groups_fk_organization_id_group_name_key UNIQUE (fk_organization_id, group_name);

ALTER TABLE ONLY public.personal_variable_groups
    ADD CONSTRAINT personal_variable_groups_pkey PRIMARY KEY (id);

CREATE INDEX personal_variable_groups_fk_created_by_idx ON public.personal_variable_groups USING btree (fk_created_by);

ALTER TABLE ONLY public.personal_variable_groups
    ADD CONSTRAINT personal_variable_groups_fk_created_by_fkey FOREIGN KEY (fk_created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.personal_variable_groups
    ADD CONSTRAINT personal_variable_groups_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.personal_variable_options (
    id text NOT NULL,
    fk_variable_id text NOT NULL,
    option_value text NOT NULL,
    sort_index integer NOT NULL
);

ALTER TABLE ONLY public.personal_variable_options
    ADD CONSTRAINT personal_variable_options_pkey PRIMARY KEY (id);

CREATE INDEX personal_variable_options_fk_variable_id_idx ON public.personal_variable_options USING btree (fk_variable_id);

ALTER TABLE ONLY public.personal_variable_options
    ADD CONSTRAINT personal_variable_options_fk_variable_id_fkey FOREIGN KEY (fk_variable_id) REFERENCES public.personal_variables(id) ON DELETE CASCADE;

CREATE TABLE public.personal_variable_values (
    fk_variable_id text NOT NULL,
    fk_user_id text NOT NULL,
    fk_organization_id text NOT NULL,
    value text NOT NULL,
    updated_at timestamp without time zone NOT NULL
);

ALTER TABLE ONLY public.personal_variable_values
    ADD CONSTRAINT personal_variable_values_pkey PRIMARY KEY (fk_variable_id, fk_user_id);

CREATE INDEX personal_variable_values_fk_organization_id_idx ON public.personal_variable_values USING btree (fk_organization_id);

CREATE INDEX personal_variable_values_fk_user_id_idx ON public.personal_variable_values USING btree (fk_user_id);

ALTER TABLE ONLY public.personal_variable_values
    ADD CONSTRAINT personal_variable_values_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.personal_variable_values
    ADD CONSTRAINT personal_variable_values_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.personal_variable_values
    ADD CONSTRAINT personal_variable_values_fk_variable_id_fkey FOREIGN KEY (fk_variable_id) REFERENCES public.personal_variables(id) ON DELETE CASCADE;

CREATE TABLE public.personal_variables (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    fk_variable_group_id text NOT NULL,
    variable_key text NOT NULL,
    variable_name text NOT NULL,
    description text,
    variable_type text NOT NULL,
    default_value text,
    sort_index integer NOT NULL,
    builtin boolean NOT NULL,
    created_at timestamp without time zone NOT NULL,
    fk_created_by text,
    CONSTRAINT personal_variables_variable_type_check CHECK ((variable_type = ANY (ARRAY['list'::text, 'number'::text, 'text'::text, 'time-range'::text])))
);

ALTER TABLE ONLY public.personal_variables
    ADD CONSTRAINT personal_variables_fk_organization_id_variable_key_key UNIQUE (fk_organization_id, variable_key);

ALTER TABLE ONLY public.personal_variables
    ADD CONSTRAINT personal_variables_pkey PRIMARY KEY (id);

CREATE INDEX personal_variables_fk_created_by_idx ON public.personal_variables USING btree (fk_created_by);

CREATE INDEX personal_variables_fk_variable_group_id_idx ON public.personal_variables USING btree (fk_variable_group_id);

ALTER TABLE ONLY public.personal_variables
    ADD CONSTRAINT personal_variables_fk_created_by_fkey FOREIGN KEY (fk_created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.personal_variables
    ADD CONSTRAINT personal_variables_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.personal_variables
    ADD CONSTRAINT personal_variables_fk_variable_group_id_fkey FOREIGN KEY (fk_variable_group_id) REFERENCES public.personal_variable_groups(id);

CREATE TABLE public.slack_apps (
    team_id text NOT NULL,
    fk_organization_id text,
    token text NOT NULL
);

ALTER TABLE ONLY public.slack_apps
    ADD CONSTRAINT slack_apps_pkey PRIMARY KEY (team_id);

CREATE INDEX slack_apps_fk_organization_id_idx ON public.slack_apps USING btree (fk_organization_id);

ALTER TABLE ONLY public.slack_apps
    ADD CONSTRAINT slack_apps_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;

CREATE TABLE public.slack_messages (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    text text NOT NULL,
    created_at timestamp without time zone NOT NULL
);

ALTER TABLE ONLY public.slack_messages
    ADD CONSTRAINT slack_messages_pkey PRIMARY KEY (id);

CREATE INDEX slack_messages_fk_organization_id_created_at_idx ON public.slack_messages USING btree (fk_organization_id, created_at DESC);

ALTER TABLE ONLY public.slack_messages
    ADD CONSTRAINT slack_messages_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.slack_user_links (
    slack_user_id text NOT NULL,
    fk_team_id text NOT NULL,
    fk_user_id text NOT NULL,
    settings bytea DEFAULT '\x7b7d'::bytea NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

ALTER TABLE ONLY public.slack_user_links
    ADD CONSTRAINT slack_user_links_pkey PRIMARY KEY (slack_user_id, fk_team_id);

CREATE INDEX slack_user_links_fk_team_id_fk_user_id_idx ON public.slack_user_links USING btree (fk_team_id, fk_user_id);

CREATE INDEX slack_user_links_fk_user_id_idx ON public.slack_user_links USING btree (fk_user_id);

ALTER TABLE ONLY public.slack_user_links
    ADD CONSTRAINT slack_user_links_fk_team_id_fkey FOREIGN KEY (fk_team_id) REFERENCES public.slack_apps(team_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_user_links
    ADD CONSTRAINT slack_user_links_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.tags (
    id text NOT NULL,
    fk_organization_id text NOT NULL,
    tag_name text NOT NULL,
    color text NOT NULL,
    sort_index integer NOT NULL,
    created_at timestamp without time zone NOT NULL,
    fk_created_by text
);

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_fk_organization_id_tag_name_key UNIQUE (fk_organization_id, tag_name);

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);

CREATE INDEX tags_fk_created_by_idx ON public.tags USING btree (fk_created_by);

CREATE INDEX tags_fk_organization_id_idx ON public.tags USING btree (fk_organization_id);

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_fk_created_by_fkey FOREIGN KEY (fk_created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_fk_organization_id_fkey FOREIGN KEY (fk_organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;

CREATE TABLE public.user_accounts (
    id text NOT NULL,
    account_id text NOT NULL,
    provider_id text NOT NULL,
    fk_user_id text NOT NULL,
    access_token text,
    refresh_token text,
    id_token text,
    access_token_expires_at timestamp with time zone,
    refresh_token_expires_at timestamp with time zone,
    scope text,
    password text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.user_accounts
    ADD CONSTRAINT user_accounts_pkey PRIMARY KEY (id);

CREATE INDEX user_accounts_user_id_idx ON public.user_accounts USING btree (fk_user_id);

ALTER TABLE ONLY public.user_accounts
    ADD CONSTRAINT user_accounts_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.user_sessions (
    id text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    token text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    ip_address text,
    user_agent text,
    fk_user_id text NOT NULL,
    active_organization_id text
);

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_token_key UNIQUE (token);

CREATE INDEX user_sessions_fk_user_id_idx ON public.user_sessions USING btree (fk_user_id);

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.user_tag_settings (
    fk_user_id text NOT NULL,
    fk_tag_id text NOT NULL,
    hidden boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY public.user_tag_settings
    ADD CONSTRAINT user_tag_settings_pkey PRIMARY KEY (fk_user_id, fk_tag_id);

CREATE INDEX user_tag_settings_fk_tag_id_idx ON public.user_tag_settings USING btree (fk_tag_id);

ALTER TABLE ONLY public.user_tag_settings
    ADD CONSTRAINT user_tag_settings_fk_tag_id_fkey FOREIGN KEY (fk_tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_tag_settings
    ADD CONSTRAINT user_tag_settings_fk_user_id_fkey FOREIGN KEY (fk_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.user_verifications (
    id text NOT NULL,
    identifier text NOT NULL,
    value text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

ALTER TABLE ONLY public.user_verifications
    ADD CONSTRAINT user_verifications_pkey PRIMARY KEY (id);

CREATE INDEX user_verifications_identifier_idx ON public.user_verifications USING btree (identifier);

CREATE TABLE public.users (
    id text NOT NULL,
    name text NOT NULL,
    email text NOT NULL,
    email_verified boolean NOT NULL,
    image text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);
