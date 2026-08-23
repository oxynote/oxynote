create table "users" ("id" text not null primary key, "name" text not null, "email" text not null unique, "email_verified" boolean not null, "image" text, "created_at" timestamptz default CURRENT_TIMESTAMP not null, "updated_at" timestamptz default CURRENT_TIMESTAMP not null);

create table "user_sessions" ("id" text not null primary key, "expires_at" timestamptz not null, "token" text not null unique, "created_at" timestamptz default CURRENT_TIMESTAMP not null, "updated_at" timestamptz not null, "ip_address" text, "user_agent" text, "fk_user_id" text not null references "users" ("id") on delete cascade, "active_organization_id" text);

create table "user_accounts" ("id" text not null primary key, "account_id" text not null, "provider_id" text not null, "fk_user_id" text not null references "users" ("id") on delete cascade, "access_token" text, "refresh_token" text, "id_token" text, "access_token_expires_at" timestamptz, "refresh_token_expires_at" timestamptz, "scope" text, "password" text, "created_at" timestamptz default CURRENT_TIMESTAMP not null, "updated_at" timestamptz not null);

create table "user_verifications" ("id" text not null primary key, "identifier" text not null, "value" text not null, "expires_at" timestamptz not null, "created_at" timestamptz default CURRENT_TIMESTAMP not null, "updated_at" timestamptz default CURRENT_TIMESTAMP not null);

create table "organizations" ("id" text not null primary key, "name" text not null, "slug" text not null unique, "logo" text, "created_at" timestamptz not null, "metadata" text);

create table "organization_members" ("id" text not null primary key, "fk_organization_id" text not null references "organizations" ("id") on delete cascade, "fk_user_id" text not null references "users" ("id") on delete cascade, "role" text not null, "created_at" timestamptz not null);

create table "organization_invitations" ("id" text not null primary key, "fk_organization_id" text not null references "organizations" ("id") on delete cascade, "email" text not null, "role" text, "status" text not null, "expires_at" timestamptz not null, "created_at" timestamptz default CURRENT_TIMESTAMP not null, "fk_inviter_id" text not null references "users" ("id") on delete cascade);

create table "jwks" ("id" text not null primary key, "public_key" text not null, "private_key" text not null, "created_at" timestamptz not null, "expires_at" timestamptz, "alg" text, "crv" text);

create table "oauth_clients" ("id" text not null primary key, "client_id" text not null unique, "client_secret" text, "client_discovery_id" text, "disabled" boolean, "skip_consent" boolean, "enable_end_session" boolean, "subject_type" text, "scopes" jsonb, "client_credentials_scopes" jsonb, "fk_user_id" text references "users" ("id") on delete cascade, "created_at" timestamptz, "updated_at" timestamptz, "name" text, "uri" text, "icon" text, "contacts" jsonb, "tos" text, "policy" text, "software_id" text, "software_version" text, "software_statement" text, "redirect_uris" jsonb not null, "post_logout_redirect_uris" jsonb, "backchannel_logout_uri" text, "backchannel_logout_session_required" boolean, "token_endpoint_auth_method" text, "application_type" text, "jwks" text, "jwks_uri" text, "grant_types" jsonb, "response_types" jsonb, "require_pkce" boolean, "dpop_bound_access_tokens" boolean, "reference_id" text, "metadata" jsonb);

create table "oauth_resources" ("id" text not null primary key, "identifier" text not null unique, "name" text not null, "access_token_ttl" integer, "refresh_token_ttl" integer, "signing_algorithm" text, "signing_key_id" text, "allowed_scopes" jsonb, "custom_claims" jsonb, "dpop_bound_access_tokens_required" boolean, "disabled" boolean, "created_at" timestamptz, "updated_at" timestamptz, "policy_version" integer, "metadata" jsonb);

create table "oauth_client_resources" ("id" text not null primary key, "client_id" text not null references "oauth_clients" ("client_id") on delete cascade, "resource_id" text not null references "oauth_resources" ("identifier") on delete cascade, "metadata" jsonb, "created_at" timestamptz);

create table "oauth_refresh_tokens" ("id" text not null primary key, "token" text not null unique, "client_id" text not null references "oauth_clients" ("client_id") on delete cascade, "session_id" text references "user_sessions" ("id") on delete set null, "fk_user_id" text not null references "users" ("id") on delete cascade, "reference_id" text, "authorization_code_id" text, "resources" jsonb, "requested_user_info_claims" jsonb, "expires_at" timestamptz not null, "created_at" timestamptz not null, "revoked" timestamptz, "rotated_at" timestamptz, "rotation_replay_response" text, "rotation_replay_expires_at" timestamptz, "auth_time" timestamptz, "confirmation" jsonb, "scopes" jsonb not null);

create table "oauth_access_tokens" ("id" text not null primary key, "token" text not null unique, "client_id" text not null references "oauth_clients" ("client_id") on delete cascade, "session_id" text references "user_sessions" ("id") on delete set null, "fk_user_id" text references "users" ("id") on delete cascade, "reference_id" text, "authorization_code_id" text, "resources" jsonb, "requested_user_info_claims" jsonb, "fk_refresh_token_id" text references "oauth_refresh_tokens" ("id") on delete cascade, "expires_at" timestamptz not null, "created_at" timestamptz not null, "revoked" timestamptz, "confirmation" jsonb, "scopes" jsonb not null);

create table "oauth_consents" ("id" text not null primary key, "client_id" text not null references "oauth_clients" ("client_id") on delete cascade, "fk_user_id" text references "users" ("id") on delete cascade, "reference_id" text, "resources" jsonb, "requested_user_info_claims" jsonb, "scopes" jsonb not null, "created_at" timestamptz not null, "updated_at" timestamptz not null);

create table "oauth_client_assertions" ("id" text not null primary key, "expires_at" timestamptz not null);

create index "user_sessions_fk_user_id_idx" on "user_sessions" ("fk_user_id");

create index "user_accounts_fk_user_id_idx" on "user_accounts" ("fk_user_id");

create index "user_verifications_identifier_idx" on "user_verifications" ("identifier");

create unique index "organizations_slug_uidx" on "organizations" ("slug");

create index "organization_members_fk_organization_id_idx" on "organization_members" ("fk_organization_id");

create index "organization_members_fk_user_id_idx" on "organization_members" ("fk_user_id");

create index "organization_invitations_fk_organization_id_idx" on "organization_invitations" ("fk_organization_id");

create index "organization_invitations_email_idx" on "organization_invitations" ("email");

create index "oauth_clients_fk_user_id_idx" on "oauth_clients" ("fk_user_id");

create index "oauth_client_resources_client_id_idx" on "oauth_client_resources" ("client_id");

create index "oauth_client_resources_resource_id_idx" on "oauth_client_resources" ("resource_id");

create index "oauth_refresh_tokens_client_id_idx" on "oauth_refresh_tokens" ("client_id");

create index "oauth_refresh_tokens_session_id_idx" on "oauth_refresh_tokens" ("session_id");

create index "oauth_refresh_tokens_fk_user_id_idx" on "oauth_refresh_tokens" ("fk_user_id");

create index "oauth_refresh_tokens_authorization_code_id_idx" on "oauth_refresh_tokens" ("authorization_code_id");

create index "oauth_access_tokens_client_id_idx" on "oauth_access_tokens" ("client_id");

create index "oauth_access_tokens_session_id_idx" on "oauth_access_tokens" ("session_id");

create index "oauth_access_tokens_fk_user_id_idx" on "oauth_access_tokens" ("fk_user_id");

create index "oauth_access_tokens_authorization_code_id_idx" on "oauth_access_tokens" ("authorization_code_id");

create index "oauth_access_tokens_fk_refresh_token_id_idx" on "oauth_access_tokens" ("fk_refresh_token_id");

create index "oauth_consents_client_id_idx" on "oauth_consents" ("client_id");

create index "oauth_consents_fk_user_id_idx" on "oauth_consents" ("fk_user_id");