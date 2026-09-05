# CLAUDE.md

Guidance for the backend: `server/core`, `server/auth-realtime`, and the
repo-root `datagen/`. Shared working principles and the TS/JS
comment/whitespace rules live in the root [CLAUDE.md](../CLAUDE.md). Go
engineering and testing standards (style, errors, logging, db layer, tests)
live in [core/CLAUDE.md](core/CLAUDE.md) — this file covers only architecture,
infrastructure, and the non-Go services.

## Stack overview

Three buildable components, orchestrated through `docker-compose`. What each one *is* is in the root [CLAUDE.md](../CLAUDE.md) map; what follows is the backend-specific detail.

- `server/core/` — one binary, `cmd/core` (`oxynote-core`), listening on `:8080`. Owns Postgres, Meilisearch, Valkey, object storage, the GitHub/Slack apps, the assistant, and the outbound data-source connections (`internal/datasource`). Its request surface is mapped below.
- `server/auth-realtime/` — listens on `:8081`, runs the Better Auth organization plugin and Hocuspocus in one Hono process, and forwards non-auth `/api/...` traffic to core (`OXYNOTE_AUTH_REALTIME_BACKEND_URL`).
- `datagen/` — separate Go module `github.com/oxynote/oxynote/datagen`. Synthesises demo Postgres/MariaDB content. Demo/testing only.

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

# auth-realtime — commands are in auth-realtime/CLAUDE.md
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
- `:8082` — changedetection.io
- `:8083` — Grafana (direct, not via Caddy)

So from a frontend's point of view: auth + realtime is `:8080/auth-realtime/...`, the core API is `:8080/core/api/...`. Internal-only Go routes (`/api/x/...`) are not exposed by Caddy; the GitHub and Slack callback URLs configured in those apps must point at `:8080/core/api/apps/...`, which is.

## Core request surface

`server/core/internal/server/router.go` is the canonical map. Three layers:

- `/api/...` (public): auth middleware (`internal/server/internal/auth`) validates sessions by calling auth-realtime's `/api/auth/get-session` (configured via `SERVER_AUTH_BETTER_AUTH_URL`). Routes for documents, branches, comments, reviewers, hooks, files, GitHub, Slack, notifications, data-sources, AI chat. `POST /api/documents/{id}/branches/{branchId}/blocks/{blockUid}/run` runs one block — `internal/document/blockrun` resolves it from stored branch content and dispatches on its type, which today means probing whether a simulated metric block's data has arrived. `GET /api/capabilities` is the frontend's capability signal: one boolean per optional service (`github`, `slack`, `changeDetection`, `search`), snapshotted at boot from each client's/manager's `Configured()`, plus an `aiAssistant` object (`status` + `model`) carrying the assistant's model-tier verdict decided in main.
- `/api/x/...` (internal): no auth at all — reverse proxy must firewall. Used by auth-realtime to fetch/store branch content (`/x/documents/{id}/branches`, `/x/documents/{id}/branch/{branchId}`), trigger emails, and initialize or tear down orgs.
- `/api/apps/...` (public, sessionless): where GitHub and Slack deliver. `POST /apps/github/events` and `POST /apps/slack/{events,commands,slash}` are gated by the provider's request signature; `GET /apps/slack/install` completes the direct-install OAuth exchange. These must stay outside `/api/x` — third parties reach core through the same front door as browsers, and the proxy 403s the internal subtree.
- `/api/mcp` (public, bearer-authed): the MCP surface (`internal/server/internal/mcp`), a streamable HTTP MCP server bridging the assistant's ungated tool registry (`tools.Set.Entries`) plus documents-as-resources. Bearer tokens are JWTs issued by auth-realtime's `@better-auth/mcp` OAuth provider; core validates each request against auth-realtime's internal `GET /api/internal/mcp/session` (JWKS verify + consent-row check, so revoking a client 401s immediately) and scopes the tool list by the token's `documents:read` / `documents:write` / `data-sources:read` scopes. Tokens are org-bound via an `org_id` claim minted at issuance. `SERVER_MCP_SESSION_URL` and `SERVER_MCP_RESOURCE_URL` are required env; the Caddyfile routes `/.well-known/oauth-*` and `/api/auth/*` at the front door to auth-realtime for OAuth discovery.
- WebSocket topics under `/api/ws` (routed by `wetsocks/wsserver` from the first-party `github.com/oxynote/wetsocks` library): `change@document-tree`, `change@documents.{documentId}.comments|metadata|reviewers|maintainers`, `post@slack.messages`, `creation@notifications`, `ping@version`. Topic binders live on the per-domain handler types under `internal/server/internal/...` (`Handler.BindXxx`).

Most public routes in the README (`/api/documents`, `/api/documents/tree`, etc.) are served by core; the README is the closest thing to a contract spec — when changing handlers, update it.

## The demo data source

Every organization is seeded at initialization with a Prometheus data source named "Demo" whose URL is `demo://engineering`, plus the welcome document whose charts query it, plus three tags — Production (`#22c55e`), Staging (`#f97316`) and Incidents (`#3b82f6`), in that order — with the welcome document's default branch under Production. This is unconditional — there is no env var and no way to opt out, so a fresh install shows populated charts and a tags section with nothing else deployed. The tags are inserted in the welcome document's transaction, so a failed seed rolls the whole initialization back; only the data source is written before it, for the reason given at the call site.

- `internal/datasource/demo` synthesises those metrics in-process. `runner.ensurePrepared` picks it over the real client when the URL is exactly `demo://engineering`; any other `demo://` URL is `demo.ErrUnknownSource` rather than a second demo. The data source's type stays `prometheus`, so the editor, autocomplete and charts cannot tell the difference.
- **One `demo.Client` serves the whole process, and it lives on `datasource.Manager`.** The client owns its engine, parser and registry, and the registry's walks cache the history they replay. A runner is built per call, so building the client there instead would throw that cache away on every request and re-walk every tick since the epoch.
- **The timeline is a pure function of the tick, not stored data.** A fixed epoch and a one-minute tick, values computed on demand from a per-series seed — so history never rewrites, a query over any window backfills for free, and the same window always answers identically. `walk.checkpoint` caches one value per day-long segment; without it a sample would replay every tick since the epoch.
- Queries run through the official `promql` engine over `queryable`, a `storage.Queryable` that synthesises only the requested window. Samples are spaced at the query's own step and the lookback delta is widened to match, which is what lets a year-long range answer with ~100 points per series instead of half a million.
- `registry.go` is the single declaration of what exists: ~15 gauge families driving both sample synthesis and the metadata/label/series endpoints. `Test_welcomeDocumentQueries` runs every query in `internal/document/files/available_metrics.json` against it, so a metric renamed in one place and not the other fails the build rather than emptying the welcome document silently.

## Document storage / Hocuspocus integration

- Documents are organized into **branches** (`document_branches` table). Every document has at least one branch; the oldest = the default/main branch.
- The Hocuspocus `documentName` is encoded as `"<documentId>-<branchIdentifier>"`. `branchIdentifier` may be the literal string `"default"`, in which case `resolveBranchId` in `server/auth-realtime/src/hocuspocus.ts` looks up the default branch ID. Splitting on the **first** `-` (`indexOf("-")`) is intentional; XIDs do not contain dashes but the suffix path can.
- Branch content is stored two ways in `document_branches`: structured ProseMirror JSON in `content` (JSONB) and the canonical Yjs binary state in `raw_content` (base64-encoded over the wire). `raw_content` is authoritative for CRDT continuity.
- `onLoadDocument` / `onStoreDocument` round-trip through core's internal `/api/x/documents/{id}/branch/{branchId}` endpoints. **Do not** use `Y.applyUpdate` to seed a freshly-created doc with content from another doc — the differing `clientID`s CRDT-merge and silently duplicate content. The codebase's pattern is `replaceYdocContent` in `server/auth-realtime/src/ydocument.ts`, which deep-clones `Y.XmlElement`s (preserving non-string attrs that `XmlElement.clone()` drops). Read the comments on `onLoadDocument` in `server/auth-realtime/src/hocuspocus.ts` and on `applyMergeToOpenDocument` in `server/auth-realtime/src/routes.ts` before touching this area.
- **The main branch is identified by its `"default"` flag, not by its name.** The tree join and `FetchMainBranchContent` key on the flag; `UpdateDocumentBranch` refuses to rename the default branch so the two never disagree. Anything that creates a document's first branch — `NewDocument`, `Duplicate` — must set `Default: true`, and forks must not.
- **Maintainers accumulate; nothing removes them.** The `maintainers` field on a branch update is *who edited in that persist*, not the document's maintainer set — auth-realtime's store hook sends the current session's editors and system writes send none. `UpsertDocumentMaintainers` only ever adds, so a diff against the stored set would drop everyone not editing at that moment. Removing a maintainer needs a path of its own; there is none today.
- **Fork, branch update and merge go through auth-realtime.** The web client calls `POST /documents/:documentId/branches`, `PUT /documents/:documentId/branches/:branchId` and `PUT /documents/:documentId/merge` on auth-realtime, which flushes the affected branch's pending Hocuspocus store (both sides for a merge; the `-default` alias too for the default branch) and refuses the operation if that store fails, then proxies to core's session-authed group under `/api/x` with the caller's headers. Core reads the branch's stored row, so without the flush a fork copies stale content, a merge reads it, and a protect toggle makes the pending user store be refused. After core confirms a protection change, auth-realtime drops the socket of every client on the branch's document (`resetConnections` in `hocuspocus.ts`): read-only is decided per connection in `onAuthenticate`, so each client reconnects with the permission the branch now has. Hocuspocus's own `closeConnections` is not that: it sends a close *message* on the still-open multiplexed socket, and the provider leaves the document closed until the page reloads. A merge then **directly mutates the in-memory target-branch Y.Doc** via `replaceYdocContent` (not `applyUpdate`) and immediately persists `rawContent` back through `/api/x/.../branch/:branchId` so a server restart can't reset the clientID and duplicate content on reconnect.

## External resource lifecycle (files, hooks)

Uploaded images live under `organizations/{org}/documents/{doc}/files/{blockUID}` — the file id **is** the image block's `uid` — and are tracked in `document_files`. That key is an object key in the S3-compatible store, or a path under the storage directory on a deployment running without one; `internal/storage` holds what the two backends share and `internal/storage/{s3,fs}` implement them. Hooks hold external resources too (changedetection.io watchers, GitHub tracking). Both are reclaimed by sweeps, not by request handlers:

- **The row outlives its owner.** `document_files` and `document_hooks` reference documents, branches and organizations with `ON DELETE SET NULL`, so a deleted document — or an org deleted by Better Auth — leaves rows behind rather than destroying the only record of the external resource. `document_files.storage_key` stores the object key outright so it stays reachable once the FKs are gone.
- **`internal/document/file/manager`** ticks every 5 minutes: it trims expired history entries, then reclaims files. A file is referenced if its id appears in any branch content of its document, any retained `document_branch_history_entries` row, or any comment/reply — checked in SQL (`CheckDocumentFileReferenced`), so a spurious match only ever retains. Unreferenced files are stamped `unreferenced_at` and deleted a day later; rows with NULL FKs skip the wait; files younger than a day are never touched, which covers the gap between an upload and the first content persist that mentions it.
- **`internal/document/hook/manager`** does the same for hooks: a row whose branch, document or organization went NULL gets `Hook.Delete` (external teardown) before the row is dropped. A merge replaces the target's hooks by nulling their branch and document (`DetachDocumentHooksByBranchID`) rather than soft-deleting them: the soft-delete mark means "block gone" and the sweep lifts it while the block is still in the branch, which after a merge it is.
- **Organization deletion is announced, not discovered.** Better Auth owns the organizations table, so core cannot notice a deletion on its own. `beforeDeleteOrganization` (`auth-realtime/src/auth.ts`) calls `POST /api/x/organizations/{id}/teardown` while every row still exists: core tears down the org's hooks, queues the search removal, drops the `slack_apps` and `github_installations` rows, and deletes the logo object. The hook throws if core fails, so the organization survives instead of leaving orphans nothing can address. Document files still need no action — the cascade nulls their keys and the file manager reclaims them.
- **Ordering is inverted between create and delete.** Creating writes the row before the object (upload, duplication); deleting removes the object before the row. Hooks follow the same rule from outside the transaction: `copyHooksToBranch` runs after the fork, merge or duplicate commits, because `hook.NewHook` creates the watcher as a side effect and a rollback cannot take that back; a failed insert tears the just-created watcher down again. Both leave at most a row without an object, which the sweep resolves — an object without a row is invisible to every cleanup path.
- **Duplication copies, never shares.** `Document.Duplicate` regenerates every block uid, rewrites image `src` paths to the new document (swapping only the path, never the host), and returns two old→new maps: the file ids the handler uses to copy each object server-side into the duplicate's own folder, and the block uids through which the source branch's hooks are re-anchored on the copy (a hook whose block the map does not name is dropped). Files are therefore owned by exactly one document.
- **Search jobs apply in order, per document.** A diff is a delta against the state its predecessors left, so the manager holds back every later diff of a document whose earlier one failed, rather than replaying it on top of newer work; an organization removal is global and holds everything after it. Without this a failed add replayed after a delete re-inserts the blocks of a document that no longer exists, and no later job would ever remove them.
- **Search index entries are branch-scoped.** Every branch of every document is indexed; an entry's id is `<branchId>-<blockUid>` (`<branchId>-docname` for the name entry), since a fork copies its source's uids, and it carries `documentId`, `branchId`, `branchName` and `branchDefault`. Every path that changes a branch queues its diff through `search.Jobs` in the same transaction: a persist on any branch, a fork (a full add of the new branch), a rename (every entry of the branch changes, since each carries the name), a merge (the target's diff). A branch deletion queues a `RemovedBranches` removal the manager applies by `branchId` filter, and a document deletion clears every branch at once: `agent.DeleteDocument` collects the subtree ids before the row goes away and returns them, so the manager clears the index by `documentId` filter — the descendants the cascade destroys have no other trace to clean up by. Index settings are declared in one object in `search.Client` and sent only when they differ from what the index reports, since a searchable or filterable attribute change re-indexes everything; every write task is awaited, so a batch Meilisearch rejects fails its job and is retried in order.
- History entries pin the files they reference, so `DB_MAX_DOCUMENT_HISTORY_ENTRIES` (count, trimmed on insert) and `DB_DOCUMENT_HISTORY_RETENTION` (age, trimmed in the sweep) also decide how long a removed image survives.

The ProseMirror schema is defined in `server/auth-realtime/src/schema/` (one file per block kind, `index.ts` aggregates). The Go-side mirror is `server/core/internal/document/node.go` (`RootBlock`, `Block`, `Mark`).

## Database

Postgres (image `postgres:18.6-alpine`). Migrations are embedded in the core binary from `server/core/internal/db/migrations/` and applied automatically by `db.New` on startup via `rubenv/sql-migrate`. Add new migrations as the next-numbered `NNN_<name>.sql` file — **once the product ships**. Until the first release there is only `001_initial.sql` and no deployed database to migrate, so a schema change goes into it in place rather than arriving as a rename-the-table follow-up nobody will ever need. The cost is local: an existing dev volume keeps the old schema, because sql-migrate records `001` as applied and will not re-run it, so reset the dev stack (`docker compose -p oxynote -f docker/docker-compose.dev.yaml down -v`) after editing it. The e2e stacks carry no volumes and are unaffected.

**Tags are per branch, visibility per user.** `tags` holds an organization's tags — the name is unique per organization, `sort_index` orders them and carries no uniqueness of its own — `document_branch_tags` pairs a branch with a tag, cascading from both so a deleted branch, document or tag takes its assignments with it, and `user_tag_settings` records the tags a user keeps out of their own sidebar, cascading from both users and tags, with no row reading as visible. The sidebar's tag tree (`FetchTagTree`) lists a document under a tag by its default branch; the header reads the open branch's own tags from `GET /documents/{documentId}/branches/{branchId}/tags`, which is also where a tag is assigned and removed. A fork copies the source branch's tags in its transaction, a merge makes the target carry exactly the source's the way it takes the source's name and icon, and a duplicate copies the source default branch's tags onto the copy's default branch.

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
- Every integration is optional and keyed on one variable. Setting that key with the
  rest of its group missing or invalid is a boot error; leaving it empty disables the
  feature — core boots, the feature's routes answer `<domain>.not_configured`, and its
  background work is skipped. `GET /api/capabilities` reports what ended up enabled.
- Keys: `GITHUB_APP_ID` (private key in `docker/github/`), `SLACK_CLIENT_ID`,
  `ASSISTANT_PROVIDER`, `MEILISEARCH_DSN`, `CHANGEDETECTION_API_URL`, `VALKEY_ADDRESS`,
  `S3_URL`, `EMAIL_SMTP_HOST`. `docker/env/*.example.env` lists every variable each
  service reads.
- Two of these degrade rather than switch off, and both silently confine the deployment
  to one instance. An empty `S3_URL` keeps objects in a directory on the local
  filesystem (`internal/storage/fs`, the counterpart of `internal/storage/s3`) rooted at
  `S3_LOCAL_PATH` — that path has no default and an unset one is a boot error, so objects
  never land somewhere the deployment did not choose. An empty `VALKEY_ADDRESS` holds
  the assistant's conversation history, checkpoints and pending confirmations in the
  core process (`pkg/memkit`), losing every conversation on restart. A model too weak
  for the assistant likewise disables it with a warning instead of failing boot.

## Assistant prompt

The system prompt at `server/core/internal/assistant/prompt.go` is assembled from section
constants. The block model, edit etiquette and aesthetics sections ship verbatim to both the
chat model and MCP clients, so a rule that is only true on one surface (the confirmation
flow, the Rubber Duck persona) lives in that surface's own section. Behaviour rules come
first and are restated in two lines at the end, with the block-model reference between them:
models weight the ends of a prompt over its middle.

Writing a rule:

- State the principle and the reason behind it in one or two sentences; the model
  generalises from the reason. Don't enumerate edge cases or add numbered steps.
- Say what to do rather than what to avoid, unless the rule is a hard limit.
- Keep per-tool facts (when to use a tool, what it returns, what it fails on) in the tool's
  description; the prompt carries only the workflow across tools.
- Worked examples and tables belong in the prompt only when the model genuinely needs the
  structure to anchor a concept (the block-schema table and the split_doc example qualify;
  most rules don't). Wrap an example in `<example>` tags so it can't be read as a rule.
- Plain language, no caps or MUST: current models overtrigger on emphasis.
- No em dashes and British spelling throughout model-facing text; the model copies the
  punctuation it is shown, and the product bans em dashes. `prompt_test.go` and
  `tools_test.go` enforce both.

## Assistant tools

Tools are grouped by what they act on, a few per file: `document.go` (the tree, one
document read whole, and a document's name, icon and place), `block.go` (reading one
block and editing content), `search.go`,
`datasource.go` (reads against the organisation's outbound connections). `eino.go` is
the odd one out — it holds the agent-framework adapter and `read_tool_output`, the one
tool that exists because of eino rather than because of the domain. `tools.Set` is the
only place that names every tool; adding one means writing its type and adding a line to
that list.

**Tools do not implement eino's interfaces.** A tool implements this package's own
`Tool` — `Info`, `Traits`, `Title`, `Summary`, `Execute` — and `eino.go` translates that
into what the framework calls. Nothing else in the package imports eino. A surface
outside the package reaches a tool through `Entry`: `Entry.Info` to describe it
(`Info.Schema()` states the arguments once, for every surface that has to publish them)
and `Entry.Tool`, a `Runner`, to run it.

**A call reports what it changed; it is never asked.** `Runner.Run` returns a `Result`
carrying the output and `Documents`. Every write goes through one of four `Input`
methods — `ApplyEdit`, `CreateDocument`, `MoveDocument`, `DeleteDocument` — and the
first three record the document as the mutation happens. A delete records nothing: the
document it names is gone.

**A block write reports what it wrote from the operation, never from a re-read.** The
realtime service applies an edit to the live document and persists it on a debounce, so
content read back right after a write may not show it. A write therefore expands its
block once (`expandForWrite`) to learn the uids it will land with, ships the canonical
form carrying those uids, and returns `blockRows` of the expanded tree; a write to an
existing block reads it (`Input.DocumentBlock`) before the edit and patches the change
onto that. Depth in those rows counts from the block itself.

**Branches are addressed by id, always.** Every content tool (`get_document`, `read_block`,
the block writes) requires `branch_id`; the default branch is a branch like any other, and
its id travels with every listing (`list_documents` carries `default_branch_id`, a search
hit carries the `branch_id` it was found on, `create_document` returns `branch_id`,
`get_document` lists every branch with its id). `Input.Branch`, `DocumentContent`, `DocumentBlock`, `ApplyEdit` and the
placement checks take the branch id and resolve it through `FetchDocumentByBranchID`,
refusing a branch that belongs to another document; an unknown id is refused with the
branches the document has (`ErrUnknownBranch`, each as "name (id)"), and a protected branch
reads but refuses every write, naming the unprotected branches to write to instead.
`Input.Document` is the default-branch fetch the document-level tools use to name and
change a document; they stay branch-free. Search covers every branch and each hit names
its own. No tool creates, renames, deletes or merges a branch. MCP resources name a
document and a branch (`oxynote://documents/{id}/branches/{branch_id}`); the list carries
each document on its default branch. The chat client reports the document and branch it
is viewing; the session stores both unchecked, as a hint the next call verifies.

**Every error names the next step.** The realtime service's operation errors are
rewritten at the boundary (`describeOpError`): a uid it holds no block for points at
`get_document`, and its operation kinds are named as the model's tools. Errors the
package raises itself follow the same shape (`errUnknownDocument`, `errUnknownBlock`,
`errBranchProtected`, `errUnknownParent`): what was wrong, then the tool or argument
that fixes it. The service's own messages stay as they are; it is not a model surface.

**`Decode` is the only way into a call's arguments.** There is no lenient variant:
`Title`, `Summary` and `Execute` all decode and all return an error, so no description is
built from the zero values a failed unmarshal left behind. `Decode` takes an `Args` — a
type with `Validate() error` — so a tool states what it requires next to the fields that
carry it (`errRequired(key)` → `<tool>: <key> is required`). Every tool has a named
`<tool>Args` type, read tools included.

Arguments are typed, not parsed: ids are `xid.ID` (optional ones
`null.Value[xid.ID]`), timestamps `time.Time`, enums their own self-validating type, and
`Decode` runs on `encoding/json/v2` so a rejected value is reported with its JSON path
(`within "/document_id"`). An empty string is an invalid value, never "absent". Ids stay
`xid.ID` all the way to the wire; only the protocol and MCP edges render strings.

A `Title` or `Summary` that names its target fetches it — `inp.Document`,
`inp.DataSource` — and uses the name on the row; a target that does not resolve is an
error passed on, never a placeholder. An unreadable payload ends no turn: the gate hands
the rejection back as the call's result, and `Set.Label` turns it into an empty label.

`Input` is built per call and carries its context, raw arguments, and every resource a
tool reaches through, already scoped to the session's (organisation, user) pair — so a
tool cannot reach another organisation's documents. `DescribeInput` is its read-only
half, handed to `Title` and `Confirm`. Shared work that needs dependencies goes on
`Input`; dependency-free helpers live in `util.go`.

**A write owns its invariants.** Checks that decide whether a mutation is legal live on
the `Input` method that performs it, not in the tool's `Execute` — `MoveDocument`
refuses a missing parent or a move under the document's own subtree, `CreateDocument`
refuses a missing parent. A tool parses its arguments and hands them over.

**One tool interface.** Every tool satisfies `Tool`; the ones that propose nothing embed
`plainSummary`. Whether a tool writes is asked of `Traits.Write` alone — never a marker
interface, which would be a second fact to keep in step. What a tool is, it states in one
`Traits()`:

- `Write` gates it behind user confirmation **and** protects its result from the context
  middlewares. A tool declaring it must describe the change in `Summary`, and is the only
  kind ever asked for one; `Test_New` checks that a write produces one and nothing else
  does.
- `Destructive` keeps it outside an "approve all" answer. Only `delete_document` and
  `delete_block`.
- `Overwrites` says the write replaces content the caller did not name — the target's
  nested blocks, and the uids comments and hooks hang off, go with it.
  `update_block_text` and `replace_block`. It is what MCP's destructive hint reports
  alongside `Destructive`, and is deliberately separate from it: a client deciding
  whether to auto-approve needs to know, while an approve-all inside a chat turn is
  meant to cover exactly these edits.
- `DataSource` says the tool reads an outbound connection rather than the organisation's
  documents. Over MCP those answer to their own `data-sources:read` scope and are
  annotated open-world.
- `Internal` keeps it off surfaces outside this process. Only `read_tool_output`, whose
  paths are minted by the reduction middleware during a chat turn.

The confirmation gate is applied by the registry from those traits, not by each tool, so
a write cannot skip it by forgetting to ask.

The MCP surface (`internal/server/internal/mcp`) serves the same registry **ungated** via
`Set.Entries`, minus the internal ones — MCP clients own the approval story, and the
write/destructive/overwrites facts become MCP tool annotations. It builds each `mcp.Tool` inline
from `Entry.Info` and calls `Entry.Tool.Run`, so it never touches eino; every document in
`Result.Documents` comes back as a resource link. Adding a tool to `tools.Set` extends
the MCP server for free.

### Tool descriptions

A description is what the model reads to pick a tool and fill its arguments, and it has to
stand alone: an MCP client may never show the instructions text. Every description says, in
three or more sentences for anything non-trivial:

- what the tool does and what it returns;
- when to use it, and which sibling tool to use instead;
- the precondition or failure it can hit (a protected document, an illegal placement, a
  missing uid);
- for each argument, what it means, whether it is optional, and the default.

It says nothing about confirmation or approval: the chat surface confirms writes and MCP
clients apply them at once, so approval belongs in the per-surface prompt sections and in
the MCP annotations. Plain language, no caps or MUST, no em dashes, British spelling.

`testdata/tool_schemas.golden` pins every tool's model-facing description. Regenerate it
with `UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/` and review
the diff as the change — a silently altered description degrades the assistant in ways no
other test catches.

## auth-realtime

The service has its own [CLAUDE.md](auth-realtime/CLAUDE.md) covering its
composition-root structure, zod-parsed environment, type-aware lint setup, and
testing conventions. From here:

- **`src/index.ts` is the only module with side effects.** Everything else is
  a factory taking its dependencies as an argument — which is what lets the
  service be tested without a database, a redis, or a port. A change that
  reads `process.env` or opens a connection outside `index.ts` breaks that.
- Prettier uses **tabs at width 8** (matching gofmt's convention on this side
  of the repo), no semicolons, trailing commas, double quotes. ESLint runs
  type-aware, and **ES module imports must include the explicit `.js`
  extension** (NodeNext + ESM), even when importing TypeScript files — e.g.
  `import { foo } from "./bar.js"` for a `./bar.ts` source file.

