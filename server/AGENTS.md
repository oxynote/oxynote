# AGENTS.md

Backend architecture and cross-service invariants for `server/core`,
`server/auth-realtime` and the repo-root `datagen/`. Shared principles: root
[AGENTS.md](../AGENTS.md). Go standards: [core/AGENTS.md](core/AGENTS.md).
Core domain rules sit beside their packages:
[core/internal/document/](core/internal/document/AGENTS.md),
[core/internal/datasource/](core/internal/datasource/AGENTS.md),
[core/internal/assistant/](core/internal/assistant/AGENTS.md).
auth-realtime: [auth-realtime/AGENTS.md](auth-realtime/AGENTS.md).

## Stack

- `server/core/`: one binary, `cmd/core`, on `:8080`. Owns Postgres,
  Meilisearch, Valkey, object storage, the GitHub/Slack apps, the assistant
  and outbound data-source connections.
- `server/auth-realtime/`: `:8081`, Better Auth (organization plugin) and
  Hocuspocus in one Hono process; forwards non-auth `/api/...` to core
  (`OXYNOTE_AUTH_REALTIME_BACKEND_URL`).
- `datagen/`: separate Go module synthesising demo Postgres/MariaDB content.

Go builds go through goreleaser: `make build` in `server/core` and `datagen`
runs `goreleaser release --snapshot --clean`, producing `bin/` and the
`ghcr.io/oxynote/{core,datagen}:dev` images the dev compose stack consumes.
datagen shares core's test/lint workflow with its own copy of the
golangci-lint profile; its `pgdemo`/`mariademo` suites start throwaway
containers.

## Caddy / ports

`docker/Caddyfile`: `:8080` front door (`/core/*` → `core:8080`,
`/auth-realtime/*` → `auth-realtime:8081`, else `web:3000`), `:8082`
changedetection.io, `:8083` Grafana (direct). `/api/x/...` is not exposed;
GitHub and Slack callback URLs point at `:8080/core/api/apps/...`.

## Core request surface

`internal/server/router.go` is the canonical map.

- `/api/...`: session-authed via auth-realtime's `/api/auth/get-session`
  (`SERVER_AUTH_BETTER_AUTH_URL`). `GET /api/capabilities` reports one
  boolean per optional service (`github`, `slack`, `changeDetection`,
  `search`), snapshotted at boot from each client's `Configured()`, plus
  `aiAssistant` (`status` + `model`).
- `/api/x/...`: no auth; auth-realtime fetches/stores branch content here
  (`/x/documents/{id}/branches`, `/x/documents/{id}/branch/{branchId}`),
  triggers emails and initializes or tears down orgs.
- `/api/apps/...`: sessionless; GitHub/Slack webhooks gated by request
  signature, `GET /apps/slack/install` completes the OAuth exchange. Must
  stay outside `/api/x`, since third parties reach core through the front
  door.
- `/api/mcp`: streamable HTTP MCP server (`internal/server/internal/mcp`)
  over the assistant's ungated tool registry plus documents as resources.
  JWT bearer tokens come from auth-realtime's `@better-auth/mcp`; each
  request is validated against `GET /api/internal/mcp/session` (JWKS +
  consent row, so revocation 401s immediately) and scoped by
  `documents:read`/`documents:write`/`data-sources:read`.
- `/api/ws` topics (`wetsocks/wsserver`): `change@document-tree`,
  `change@tag-tree`,
  `change@documents.{documentId}.comments|metadata|reviewers|maintainers|tags|hooks`,
  `post@slack.messages`, `creation@notifications`, `ping@version`. Binders
  are `Handler.BindXxx` under `internal/server/internal/...`.

The README lists the public routes; update it when changing handlers.

## Document storage / Hocuspocus

- Documents have branches (`document_branches`); every document has at least
  one, and the main one is flagged `default` (see the document package's
  AGENTS.md for the branch rules).
- Hocuspocus `documentName` is `"<documentId>-<branchIdentifier>"`;
  `"default"` is resolved by `resolveBranchId`
  (`auth-realtime/src/hocuspocus.ts`). Split on the first `-` only.
- Branch content is stored twice: ProseMirror JSON in `content` (JSONB) and
  the Yjs binary in `raw_content` (base64 on the wire). `raw_content` is
  authoritative for CRDT continuity.
- `onLoadDocument`/`onStoreDocument` round-trip through
  `/api/x/documents/{id}/branch/{branchId}`. Never seed a doc from another
  with `Y.applyUpdate` (clientIDs CRDT-merge and duplicate content); use
  `replaceYdocContent` (`auth-realtime/src/ydocument.ts`), which deep-clones
  `Y.XmlElement`s preserving the non-string attrs `clone()` drops. Read the
  comments on `onLoadDocument` and on `applyMergeToOpenDocument`
  (`routes.ts`) first.
- **Fork, branch update, merge and branch delete go through auth-realtime**,
  which flushes the affected branch's pending store (both sides for a merge,
  the `-default` alias too), refuses the operation if the flush fails, then
  proxies to core's session-authed `/api/x` group with the caller's headers.
  After a protection change or a delete it drops every client socket on the
  document (`resetConnections`; hocuspocus's `closeConnections` is not
  that), since read-only is decided per connection in `onAuthenticate`. A
  merge mutates the in-memory target Y.Doc via `replaceYdocContent` and
  persists `rawContent` immediately.
- The ProseMirror schema is `auth-realtime/src/schema/`; its Go mirror is
  `core/internal/document/node.go`.

## Database

Postgres 18. Migrations are embedded from `core/internal/db/migrations/` and
applied by `db.New` (`rubenv/sql-migrate`). **Until the first release there
is only `001_initial.sql` and schema changes go into it in place**;
afterwards, add `NNN_<name>.sql`. An existing dev volume keeps the old
schema, so reset it with `docker compose -p oxynote -f
docker/docker-compose.dev.yaml down -v` after editing.

**Core migrations own the Better Auth tables** (snake_case columns matching
the `fields` mappings in `auth-realtime/src/auth.ts`).
`auth-realtime/sql/better_auth_schema.sql` is reference output, never
applied. Regenerate it to diff after changing `auth.ts`:

```sh
# from server/auth-realtime/
source ../../docker/env/auth-realtime.local.env && npx @better-auth/cli generate --output ./sql/better_auth_schema.sql
```

## Env vars

`docker/env/*.example.env` lists every variable each service reads, empty
ones included; the dev compose reads only the gitignored `*.local.env`
copies. Core reads `OXYNOTE_CORE_*` through `buildinfo.Getenv("FOO")`.

Every integration is optional and keyed on one variable (`GITHUB_APP_ID`,
`SLACK_CLIENT_ID`, `ASSISTANT_PROVIDER`, `MEILISEARCH_DSN`,
`CHANGEDETECTION_API_URL`, `VALKEY_ADDRESS`, `S3_URL`, `EMAIL_SMTP_HOST`).
Key set with the rest of its group missing is a boot error; key empty
disables the feature (routes answer `<domain>.not_configured`, background
work skipped). Two degrade instead and confine the deployment to one
instance: empty `S3_URL` stores objects under `S3_LOCAL_PATH` (no default;
unset is a boot error), empty `VALKEY_ADDRESS` keeps assistant conversations
in-process (`pkg/memkit`). A model too weak for the assistant disables it
with a warning.
