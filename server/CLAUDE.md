# CLAUDE.md

Guidance for the backend: `server/core`, `server/auth-realtime`, and the
repo-root `datagen/`. Shared working principles and the TS/JS
comment/whitespace rules live in the root [CLAUDE.md](../CLAUDE.md). Go
engineering and testing standards (style, errors, logging, db layer, tests)
live in [core/CLAUDE.md](core/CLAUDE.md) — this file covers only architecture,
infrastructure, and the non-Go services.

## Stack overview

`server/` is the backend half of the Oxynote collaborative documentation product. Paths below are relative to the repository root; three buildable components, all orchestrated through `docker-compose`:

- `server/core/` — Go module `github.com/oxynote/oxynote/server/core`. One binary: `cmd/core` (`oxynote-core`) — main API server. Listens on `:8080`. Exposes `/api/...` (auth-required), `/api/apps/...` (public, sessionless third-party callbacks), and `/api/x/...` (internal-only, no auth; the reverse proxy is expected to firewall this). Owns Postgres, Meilisearch, Valkey (Redis), RustFS, the GitHub/Slack apps, the assistant, and the outbound data-source connections (`internal/datasource`).
- `server/auth-realtime/` — `@oxynote/auth-realtime` package. TypeScript ES-module service that runs Better Auth (organization plugin) and a Hocuspocus (Yjs CRDT) server in a single Hono process on port `8081`. Forwards non-auth `/api/...` traffic to core (`OXYNOTE_AUTH_REALTIME_BACKEND_URL`).
- `datagen/` — separate Go module `github.com/oxynote/oxynote/datagen`. Synthesises demo Postgres/MariaDB content, and serves a fake metrics endpoint for a Prometheus to scrape. The demo Prometheus data source no longer uses that endpoint: core synthesises those metrics itself (`server/core/internal/datasource/demo`), so nothing in the dev stack scrapes it.

Go builds go through **goreleaser**: `make build` in `server/core` and `datagen` runs `goreleaser release --snapshot --clean`, producing binaries in `bin/` and the `ghcr.io/oxynote/{core,datagen}:dev` images via each module's `Dockerfile.dev`. The dev compose stack consumes those images; `auth-realtime` (and `web/`) are built from source by compose via their `Dockerfile.dev`. `make run` / `make start` from the repository root orchestrates both.

## Common commands

From the repository root:

```sh
docker-compose -p oxynote -f docker/docker-compose.dev.yaml up    # bring up the stack
```

Per-component:

```sh
# core
cd server/core && make build      # goreleaser release --snapshot --clean -> bin/

# datagen — same targets as core
cd datagen && make build
cd datagen && make test           # go test -race ./... (pgdemo/mariademo need docker)
cd datagen && make check-coverage # fails below COVERAGE_MIN
cd datagen && make check-lint     # golangci-lint; lint = the fixing variant

# auth-realtime (full details in auth-realtime/CLAUDE.md)
cd server/auth-realtime && pnpm run dev          # local watch mode (tsx + sentry.ts preloaded)
cd server/auth-realtime && pnpm run build        # tsc -p tsconfig.build.json
cd server/auth-realtime && pnpm run check-lint   # types + eslint + prettier + knip
cd server/auth-realtime && pnpm run lint         # the fixing variant
cd server/auth-realtime && pnpm run test         # vitest run --coverage
cd server/auth-realtime && pnpm run qa           # check-lint + test; qa-fix = lint + test
```

Both Go modules share the same test/lint workflow, documented in
[core/CLAUDE.md](core/CLAUDE.md) ("Commands & QA gates"); datagen carries its
own copy of core's golangci-lint profile. Its `pgdemo` and `mariademo` suites
start throwaway Postgres and MariaDB containers, so docker is a unit-test
dependency for those two packages the same way it is for core's `internal/db`
and `internal/datasource/processor`.

Go dependencies are fetched into the module cache at build time (`make deps` runs `go mod download`) — there is no vendoring. All dependencies, including the first-party `github.com/oxynote/wetsocks`, are public; no GOPRIVATE or git auth setup is needed.

## Caddy / port layout

`docker/Caddyfile` is the entrypoint for all client traffic:

- `:8080` — front door. `/core/*` is path-stripped and reverse-proxied to `core:8080`; `/auth-realtime/*` is path-stripped and reverse-proxied to `auth-realtime:8081` (Better Auth, Hocuspocus, the merge proxy — the service itself still serves `/api/...` and `/hocuspocus`); everything else goes to the web SSR container (`web:3000`).
- `:8081` — RustFS console
- `:8082` — changedetection.io
- `:8083` — Grafana (direct, not via Caddy)

So from a frontend's point of view: auth + realtime is `:8080/auth-realtime/...`, the core API is `:8080/core/api/...`. Internal-only Go routes (`/api/x/...`) are not exposed by Caddy; the GitHub and Slack callback URLs configured in those apps must point at `:8080/core/api/apps/...`, which is.

## Core request surface

`server/core/internal/server/router.go` is the canonical map. Three layers:

- `/api/...` (public): auth middleware (`internal/server/internal/auth`) validates sessions by calling auth-realtime's `/api/auth/get-session` (configured via `SERVER_AUTH_BETTER_AUTH_URL`). Routes for documents, branches, comments, reviewers, hooks, files, GitHub, Slack, notifications, data-sources, AI chat. `GET /api/capabilities` is the frontend's capability signal: one boolean per optional service (`github`, `slack`, `changeDetection`, `search`), snapshotted at boot from each client's/manager's `Configured()`, plus an `aiAssistant` object (`status` + `model`) carrying the assistant's model-tier verdict decided in main.
- `/api/x/...` (internal): no auth at all — reverse proxy must firewall. Used by auth-realtime to fetch/store branch content (`/x/documents/{id}/branches`, `/x/documents/{id}/branch/{branchId}`), trigger emails, and initialize or tear down orgs.
- `/api/apps/...` (public, sessionless): where GitHub and Slack deliver. `POST /apps/github/events` and `POST /apps/slack/{events,commands,slash}` are gated by the provider's request signature; `GET /apps/slack/install` completes the direct-install OAuth exchange. These must stay outside `/api/x` — third parties reach core through the same front door as browsers, and the proxy 403s the internal subtree.
- `/api/mcp` (public, bearer-authed): the MCP surface (`internal/server/internal/mcp`), a streamable HTTP MCP server bridging the assistant's ungated tool registry (`tools.Set.Entries`) plus documents-as-resources. Bearer tokens are JWTs issued by auth-realtime's `@better-auth/mcp` OAuth provider; core validates each request against auth-realtime's internal `GET /api/internal/mcp/session` (JWKS verify + consent-row check, so revoking a client 401s immediately) and scopes the tool list by the token's `documents:read` / `documents:write` / `data-sources:read` scopes. Tokens are org-bound via an `org_id` claim minted at issuance. `SERVER_MCP_SESSION_URL` and `SERVER_MCP_RESOURCE_URL` are required env; the Caddyfile routes `/.well-known/oauth-*` and `/api/auth/*` at the front door to auth-realtime for OAuth discovery.
- WebSocket topics under `/api/ws` (routed by `wetsocks/wsserver` from the first-party `github.com/oxynote/wetsocks` library): `change@document-tree`, `change@documents.{documentId}.comments|metadata|reviewers|maintainers`, `post@slack.messages`, `creation@notifications`, `ping@version`. Topic binders live on the per-domain handler types under `internal/server/internal/...` (`Handler.BindXxx`).

Most public routes in the README (`/api/documents`, `/api/documents/tree`, etc.) are served by core; the README is the closest thing to a contract spec — when changing handlers, update it.

## The demo data source

Every organization is seeded at initialization with a Prometheus data source named "Demo" whose URL is `demo://engineering`, plus the welcome document whose charts query it. This is unconditional — there is no env var and no way to opt out, so a fresh install shows populated charts with nothing else deployed.

- `internal/datasource/demo` synthesises those metrics in-process. `runner.ensurePrepared` picks it over the real client when the URL is exactly `demo://engineering`; any other `demo://` URL is `demo.ErrUnknownSource` rather than a second demo. The data source's type stays `prometheus`, so the editor, autocomplete and charts cannot tell the difference.
- **One `demo.Client` serves the whole process, and it lives on `datasource.Manager`.** The client owns its engine, parser and registry, and the registry's walks cache the history they replay. A runner is built per call, so building the client there instead would throw that cache away on every request and re-walk every tick since the epoch.
- **The timeline is a pure function of the tick, not stored data.** A fixed epoch and a one-minute tick, values computed on demand from a per-series seed — so history never rewrites, a query over any window backfills for free, and the same window always answers identically. `walk.checkpoint` caches one value per day-long segment; without it a sample would replay every tick since the epoch.
- Queries run through the official `promql` engine over `queryable`, a `storage.Queryable` that synthesises only the requested window. Samples are spaced at the query's own step and the lookback delta is widened to match, which is what lets a year-long range answer with ~100 points per series instead of half a million.
- `registry.go` is the single declaration of what exists: ~15 gauge families driving both sample synthesis and the metadata/label/series endpoints. `Test_welcomeDocumentQueries` runs every query in `internal/document/files/available_metrics.json` against it, so a metric renamed in one place and not the other fails the build rather than emptying the welcome document silently.

## Document storage / Hocuspocus integration

This is the model that's least obvious from a fresh read:

- Documents are organized into **branches** (`document_branches` table). Every document has at least one branch; the oldest = the default/main branch.
- The Hocuspocus `documentName` is encoded as `"<documentId>-<branchIdentifier>"`. `branchIdentifier` may be the literal string `"default"`, in which case `resolveBranchId` in `server/auth-realtime/src/hocuspocus.ts` looks up the default branch ID. Splitting on the **first** `-` (`indexOf("-")`) is intentional; XIDs do not contain dashes but the suffix path can.
- Branch content is stored two ways in `document_branches`: structured ProseMirror JSON in `content` (JSONB) and the canonical Yjs binary state in `raw_content` (base64-encoded over the wire). `raw_content` is authoritative for CRDT continuity.
- `onLoadDocument` / `onStoreDocument` round-trip through core's internal `/api/x/documents/{id}/branch/{branchId}` endpoints. **Do not** use `Y.applyUpdate` to seed a freshly-created doc with content from another doc — the differing `clientID`s CRDT-merge and silently duplicate content. The codebase's pattern is `replaceYdocContent` in `server/auth-realtime/src/ydocument.ts`, which deep-clones `Y.XmlElement`s (preserving non-string attrs that `XmlElement.clone()` drops). Read the comments on `onLoadDocument` in `server/auth-realtime/src/hocuspocus.ts` and on `applyMergeToOpenDocument` in `server/auth-realtime/src/routes.ts` before touching this area.
- **The main branch is identified by its `"default"` flag, not by its name.** The tree join and `FetchMainBranchContent` key on the flag; `UpdateDocumentBranch` refuses to rename the default branch so the two never disagree. Anything that creates a document's first branch — `NewDocument`, `Duplicate` — must set `Default: true`, and forks must not.
- **Maintainers accumulate; nothing removes them.** The `maintainers` field on a branch update is *who edited in that persist*, not the document's maintainer set — auth-realtime's store hook sends the current session's editors and system writes send none. `UpsertDocumentMaintainers` only ever adds, so a diff against the stored set would drop everyone not editing at that moment. Removing a maintainer needs a path of its own; there is none today.
- Branch merging: `PUT /documents/:documentId/merge` is handled by auth-realtime, which proxies to core, then **directly mutates the in-memory target-branch Y.Doc** via `replaceYdocContent` (not `applyUpdate`) and immediately persists `rawContent` back through `/api/x/.../branch/:branchId` so a server restart can't reset the clientID and duplicate content on reconnect.

## External resource lifecycle (files, hooks)

Uploaded images live in RustFS under `organizations/{org}/documents/{doc}/files/{blockUID}` — the file id **is** the image block's `uid` — and are tracked in `document_files`. Hooks hold external resources too (changedetection.io watchers, GitHub tracking). Both are reclaimed by sweeps, not by request handlers:

- **The row outlives its owner.** `document_files` and `document_hooks` reference documents, branches and organizations with `ON DELETE SET NULL`, so a deleted document — or an org deleted by Better Auth — leaves rows behind rather than destroying the only record of the external resource. `document_files.storage_key` stores the object key outright so it stays reachable once the FKs are gone.
- **`internal/document/file/manager`** ticks every 5 minutes: it trims expired changelog snapshots, then reclaims files. A file is referenced if its id appears in any branch content of its document, any retained `document_branch_changelogs` snapshot, or any comment/reply — checked in SQL (`CheckDocumentFileReferenced`), so a spurious match only ever retains. Unreferenced files are stamped `unreferenced_at` and deleted a day later; rows with NULL FKs skip the wait; files younger than a day are never touched, which covers the gap between an upload and the first content persist that mentions it.
- **`internal/document/hook/manager`** does the same for hooks: a row whose branch, document or organization went NULL gets `Hook.Delete` (external teardown) before the row is dropped.
- **Organization deletion is announced, not discovered.** Better Auth owns the organizations table, so core cannot notice a deletion on its own. `beforeDeleteOrganization` (`auth-realtime/src/auth.ts`) calls `POST /api/x/organizations/{id}/teardown` while every row still exists: core tears down the org's hooks, queues the search removal, drops the `slack_apps` and `github_installations` rows, and deletes the logo object. The hook throws if core fails, so the organization survives instead of leaving orphans nothing can address. Document files still need no action — the cascade nulls their keys and the file manager reclaims them.
- **Ordering is inverted between create and delete.** Creating writes the row before the object (upload, duplication); deleting removes the object before the row. Hooks follow the same rule from outside the transaction: `copyHooksToBranch` runs after the fork or merge commits, because `hook.NewHook` creates the watcher as a side effect and a rollback cannot take that back; a failed insert tears the just-created watcher down again. Both leave at most a row without an object, which the sweep resolves — an object without a row is invisible to every cleanup path.
- **Duplication copies, never shares.** `Document.Duplicate` regenerates every block uid, rewrites image `src` paths to the new document (swapping only the path, never the host), and returns the old→new id map the handler uses to copy each object server-side into the duplicate's own folder. Files are therefore owned by exactly one document.
- **Search jobs apply in order, per document.** A diff is a delta against the state its predecessors left, so the manager holds back every later diff of a document whose earlier one failed, rather than replaying it on top of newer work; an organization removal is global and holds everything after it. Without this a failed add replayed after a delete re-inserts the blocks of a document that no longer exists, and no later job would ever remove them.
- **Search index entries are removed from the ids the delete reports.** Only default-branch blocks are indexed, keyed by block uid with a `documentId` field. `agent.DeleteDocument` collects the subtree ids before the row goes away and returns them, and the deletion paths queue a search job carrying them through `search.Jobs` in the same transaction, so the manager clears the index by `documentId` filter — the descendants the cascade destroys have no other trace to clean up by.
- Snapshots pin the files they reference, so `DB_MAX_DOCUMENT_CHANGELOGS` (count, trimmed on insert) and `DB_DOCUMENT_CHANGELOG_RETENTION` (age, trimmed in the sweep) also decide how long a removed image survives.

The ProseMirror schema is defined in `server/auth-realtime/src/schema/` (one file per block kind, `index.ts` aggregates). The Go-side mirror is `server/core/internal/document/node.go` (`RootBlock`, `Block`, `Mark`).

## Database

Postgres (image `postgres:18.6-alpine`). Migrations are embedded in the core binary from `server/core/internal/db/migrations/` and applied automatically by `db.New` on startup via `rubenv/sql-migrate`. Add new migrations as the next-numbered `NNN_<name>.sql` file.

**The core migrations own all tables, including the Better Auth ones** (`users`, `user_accounts`, `organizations`, … are created by `001_initial.sql` with snake_case columns matching the `fields` mappings in `src/auth.ts`). The generated `better_auth_schema.sql` is **reference output only — never apply it directly**; regenerate it to diff what Better Auth expects after changing `src/auth.ts`:

```sh
# from server/auth-realtime/, with OXYNOTE_AUTH_REALTIME_DB_DSN exported and other auth env vars set
source ../../docker/env/auth-realtime.local.env && npx @better-auth/cli generate --output ./sql/better_auth_schema.sql
```

Then the generated `server/auth-realtime/sql/better_auth_schema.sql` is executed against the Postgres container manually. See the README "Better Auth" section.

## Env vars

Env lives in `docker/env/`: committed `*.example.env` templates list **every** variable each service reads, explicitly, even when empty, with dev defaults preset. `make setup` copies each template to its gitignored `*.local.env` when missing — the dev compose reads **only** the `.local.env` files, so secrets and local tweaks go there.

- core vars are prefixed `OXYNOTE_CORE_`; `buildinfo.Getenv("FOO")` reads `OXYNOTE_CORE_FOO`.
- auth-realtime vars are prefixed `OXYNOTE_AUTH_REALTIME_`.
- All integrations are optional, each keyed on one variable; a set key with the rest of its group missing or invalid is a boot error, and `GET /api/capabilities` reports what ended up enabled. The GitHub App is enabled by setting `OXYNOTE_CORE_GITHUB_APP_ID` and dropping the app's private key into `docker/github/` (mounted into the core container); an empty `OXYNOTE_CORE_GITHUB_APP_ID` disables it — core boots, GitHub routes respond `github.not_configured` (the always-200 `GET /api/github` status endpoint also reports `configured: false`), and github-tracking hooks are skipped. A set app ID with a missing/unreadable key is a boot error. Slack works the same way keyed on `OXYNOTE_CORE_SLACK_CLIENT_ID` (`slack.not_configured`, `GET /api/slack` reports `configured`); a set client ID with other `SLACK_*` values missing is a boot error. The assistant is keyed on `ASSISTANT_PROVIDER`: empty disables it (`GET /api/ai/chat` responds `assistant.not_configured` before any websocket upgrade, no model objects are built, the MCP surface keeps working, and the other `ASSISTANT_*` values are ignored); a set provider needs an `ASSISTANT_API_KEY` for every provider except ollama, and missing or unsupported values are a boot error; an empty `ASSISTANT_MODEL` defaults to the provider's strongest supported model (`Provider.DefaultModel`). The model itself is judged at boot against the provider's supported list (`provider.Options.ModelStatus`, backed by the embedded `internal/assistant/provider/models/models.json` — defaults and status-grouped model ids per provider): a full-strength model runs the assistant, a mid-tier one runs it with a logged warning, and a too-weak or unlisted one disables the assistant with a warning instead of failing boot — `GET /api/capabilities` reports the verdict as `aiAssistant.status` (`active` / `active-but-weak` / `inactive-too-weak` / `inactive`) with the configured model id. Provider-specific credentials (`ASSISTANT_BEDROCK_*`, `ASSISTANT_VERTEX_*`, `ASSISTANT_AZURE_API_VERSION`) are only read when the selected provider uses them. Search is keyed on `MEILISEARCH_DSN`: empty disables it — `GET /api/documents/search` responds `search.not_configured`, every enqueue goes through `search.Jobs` (`internal/search/jobs.go`) which drops jobs on a disabled deployment, the search-job manager is not started, and the `search_documents` tool is left out of the assistant/MCP registry; a set DSN that cannot be reached is a boot error. Changedetection.io is keyed on `CHANGEDETECTION_API_URL`: empty disables it — creating, updating or resetting a url-watcher hook responds `changedetection.not_configured`, existing url-watcher hooks are skipped by the processor and by fork/merge hook copies, and deleting one still removes the row (`webchange.Client.DeleteWatcher` is a no-op when unconfigured); `CHANGEDETECTION_API_KEY` may stay empty either way. Email is keyed on `OXYNOTE_CORE_EMAIL_SMTP_HOST`: when empty, core boots and each would-be email is logged instead of sent; with a set host, an invalid `EMAIL_SMTP_PORT` or `EMAIL_SMTP_TLS` (`none` | `starttls` | `tls`) is a boot error. The dev stack presets the mailpit container as the SMTP target (`mailpit:1025`, plaintext, no auth; web UI on host `:8025`) — the four HTML templates are embedded in the core binary from `server/core/internal/email/templates/`.

## Assistant prompt

The system prompt at `server/core/internal/assistant/prompt.go` codifies behaviour for the Rubber Duck AI chat. When fixing a behaviour bug, state the underlying principle in one or two sentences. Don't enumerate edge cases or add numbered steps — spelling every case out drowns the core guidance in noise. Worked examples and tables belong in the prompt only when the model genuinely needs the structure to anchor a concept (the block-schema table and the split_doc example qualify; most rules don't).

## Assistant tools

Tools are grouped by what they act on, a few per file: `document.go` (the tree
and one document's metadata), `block.go` (reading and editing content),
`search.go`, `datasource.go` (reads against the organisation's outbound
connections). `eino.go` is the odd one out — it holds the agent-framework
adapter and `read_tool_output`, the one tool that exists *because* of eino
rather than because of the domain. `tools.Set` is the only place that names
every tool; adding one means writing its type and adding a line to that list.

**Tools do not implement eino's interfaces.** A tool implements this package's
own `Tool` — `Info`, `Traits`, `Title`, `Summary`, `Execute` — and `eino.go`
translates that into what the framework calls. Nothing else in the package imports eino, so the tool files
stay free of `argumentsInJSON`, `...tool.Option` and `*schema.ToolInfo`.

The framework stops at this package's edge. A surface outside it reaches a
tool through `Entry`: `Entry.Info` to describe it (`Info.Schema()` states the
arguments once, for every surface that has to publish them) and
`Entry.Tool`, a `Runner`, to run it. `Info.toEino` and `einoTool.InvokableRun`
are the framework's own costume over the same two things, worn only inside
`eino.go`.

**A call reports what it changed; it is never asked.** `Runner.Run` returns a
`Result` carrying the output and `Documents`, the documents the call created or
changed. Every write in the package goes through one of four `Input` methods —
`ApplyEdit`, `CreateDocument`, `MoveDocument`, `DeleteDocument` — and the first
three record the document there, as the mutation happens. So the list is right
for a call that changes several documents and empty for one that changes none,
neither of which reading the arguments could establish. A delete records
nothing: the document it names is gone, so there is nothing left to point at.

**A tool states its arguments once.** Every write declares a named `<tool>Args`
type and decodes into it, so one payload has one Go shape instead of a partial
struct per method, and `Confirm` hands the document id it read to `summarize`
rather than having it sniffed back out of the JSON. Read tools, whose arguments
are used in one place, keep a local anonymous struct. Describing a call still
parses its arguments — `Title` and `Confirm` run *before* the call does, so
there is nothing to derive them from — but each describe now parses once.

**There is one tool interface, not two.** Every tool satisfies `Tool`; the ones
that propose nothing embed `plainSummary` for the no-op. Whether a tool writes
is asked of `Traits.Write` alone, so there is no second fact — a marker
interface — to keep in step with it, and the adapter reaches `Summary` directly
instead of type-asserting its way to it.

**`Decode` is the only way into a call's arguments.** There is no lenient
variant: `Title`, `Summary` and `Execute` all decode and all return an error, so
no description is ever built from the zero values a failed unmarshal left
behind. A `Title` or `Summary` that names its target fetches it — `inp.Document`,
`inp.DataSource` — and uses the name on the row; a target that does not resolve
is an error passed on, never a placeholder, so the model is told what it named
does not exist (`Set.Label` announces nothing, the gate returns the text as the
call's result).

`Decode` takes an `Args` — a type with `Validate() error` — and runs it once
the payload is read, so a tool states what it requires next to the fields that
carry it (`errRequired(key)` → `<tool>: <key> is required`) and `Title`,
`Summary` and `Execute` never see a payload the tool cannot act on. Every tool
therefore has a named `<tool>Args` type, read tools included. Arguments are
typed, not parsed: ids are `xid.ID` (optional ones `null.Value[xid.ID]`),
timestamps `time.Time`, enums their own self-validating type
(`processor.ChartType`, `position`), and `Decode` runs on `encoding/json/v2` so
a value one of those types rejects is reported with the argument's JSON path
(`within "/document_id"`). An empty string is never "absent" for such an
argument — it is an invalid value, and the model is told so. Ids stay `xid.ID`
all the way to the wire: result structs, `ActionSummary`, `Result.Documents`
and the edit client take ids, and only the protocol/MCP edges render strings.

Each caller then decides what an unreadable payload means. Neither ends the
turn: a bad payload is the model's to fix, and it can only fix what it is told
about. The gate hands the rejection back as the call's result rather than
parking the run — a payload `Execute` would reject is not worth spending a
user's confirmation on, and the same payload comes back unchanged on resume.
`Set.Label` turns it into an empty label: the observer announces a call it is
about to make, and that call is about to fail on the same arguments, so the
failure is the tool's to report and a status line derived from nothing would
only precede it with a lie.

`Input` is built per call and carries the call itself: its context, its raw
arguments, and every resource a tool reaches through, already scoped to the
session's (organisation, user) pair — so a tool names only what it wants and
cannot reach another organisation's documents. `DescribeInput` is its read-only
half, handed to `Title` and `Confirm`: it can probe the arguments and name the
target document but reaches nothing that mutates, so describing a pending write
cannot perform it. Shared work that needs dependencies goes on `Input`;
dependency-free helpers live in `util.go`.

What a tool *is*, it states — in one `Traits()` on its own type, not spread
across marker interfaces and schema fields:

- `Write` gates it behind user confirmation **and** protects its result from
  being cleared by the context middlewares — the model has to keep knowing what
  it changed, while a stale read can always be taken again. A tool declaring it
  must describe the change it proposes in `Summary`, and is the only kind of
  tool ever asked for one. `Test_New` checks that a write actually produces a
  summary and that nothing else does — a compiler cannot, since every tool
  satisfies the same interface.
- `Destructive` keeps it outside an "approve all" answer. Only
  `delete_document` and `delete_block`.
- `DataSource` says the tool reads an outbound data-source connection rather
  than the organisation's documents. Over MCP those tools answer to their own
  `data-sources:read` scope — querying an organisation's databases is not the
  same permission as reading its documents — and are annotated open-world,
  since what a connection points at is not Oxynote's.
- `Internal` keeps it off surfaces outside this process. Only
  `read_tool_output`: the paths it takes are minted by the reduction middleware
  during a chat turn, so a client holding none of that state would be offered a
  tool it could never call.

The confirmation gate is applied by the registry from those traits, not by each
tool, so a write cannot skip it by forgetting to ask.

**A write owns its invariants.** Checks that decide whether a mutation is legal
live on the `Input` method that performs it, not in the tool's `Execute` — so
`MoveDocument` is what refuses a missing parent or a move under the document's
own subtree, and `CreateDocument` is what refuses a missing parent. A tool
parses its arguments and hands them over; it does not re-derive the rules.

The MCP surface (`internal/server/internal/mcp`) serves the same registry
**ungated** via `Set.Entries`, minus the internal ones — MCP clients own the
approval story, and the write/destructive facts become MCP tool annotations
instead. It builds each `mcp.Tool` inline from `Entry.Info` and calls
`Entry.Tool.Run`, so it never touches eino; every document in the call's
`Result.Documents` comes back as a resource link. Adding a tool to `tools.Set` therefore extends the
MCP server for free.

`testdata/tool_schemas.golden` pins every tool's model-facing description. Any
change to a schema or a tool description has to be deliberate enough to update
that file — a silently altered description degrades the assistant in ways no
other test catches.

## auth-realtime

The service has its own [CLAUDE.md](auth-realtime/CLAUDE.md) covering its
composition-root structure, zod-parsed environment, type-aware lint setup, and
testing conventions. Two things worth knowing from here:

- **`src/index.ts` is the only module with side effects.** Everything else is
  a factory taking its dependencies as an argument — which is what lets the
  service be tested without a database, a redis, or a port. A change that
  reads `process.env` or opens a connection outside `index.ts` breaks that.
- Prettier uses **tabs at width 8** (matching gofmt's convention on this side
  of the repo), no semicolons, trailing commas, double quotes. ESLint runs
  type-aware, and **ES module imports must include the explicit `.js`
  extension** (NodeNext + ESM), even when importing TypeScript files — e.g.
  `import { foo } from "./bar.js"` for a `./bar.ts` source file.

