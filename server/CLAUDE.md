# CLAUDE.md

Guidance for the backend: `server/core`, `server/auth-realtime`, and the
repo-root `datagen/`. Shared working principles and the TS/JS
comment/whitespace rules live in the root [CLAUDE.md](../CLAUDE.md).

## Stack overview

`server/` is the backend half of the Oxynote collaborative documentation product. Paths below are relative to the repository root; three buildable components, all orchestrated through `docker-compose`:

- `server/core/` — Go module `github.com/oxynote/oxynote/server/core`. Two binaries:
  - `cmd/core` (`oxynote-core`) — main API server. Listens on `:8080`. Exposes `/api/...` (auth-required) and `/api/x/...` (internal-only, no auth; the reverse proxy is expected to firewall this). Owns Postgres, Meilisearch, Valkey (Redis), MinIO, the GitHub/Slack apps and the assistant.
  - `cmd/connector` (`oxynote-connector`) — a separate, near-empty HTTP server that core calls via `CONNECTOR_URL`. Used for outbound data-source connections from a different network boundary.
- `server/auth-realtime/` — `@oxynote/auth-realtime` package. TypeScript ES-module service that runs Better Auth (organization plugin) and a Hocuspocus (Yjs CRDT) server in a single Hono process on port `8081`. Forwards non-auth `/api/...` traffic to core (`OXYNOTE_AUTH_REALTIME_BACKEND_URL`).
- `datagen/` — separate Go module `github.com/oxynote/oxynote/datagen`. Synthesises demo Postgres/MariaDB content and a fake metrics endpoint scraped by Prometheus for the demo data sources.

Go builds go through **goreleaser**: `make build` in `server/core` and `datagen` runs `goreleaser release --snapshot --clean`, producing binaries in `bin/` and the `ghcr.io/oxynote/{core,connector,datagen}:dev` images via each module's `Dockerfile.dev`. The dev compose stack consumes those images; `auth-realtime` (and `web/`) are built from source by compose via their `Dockerfile.dev`. `make run` / `make start` from the repository root orchestrates both.

## Common commands

From the repository root:

```sh
docker-compose -p oxynote -f docker/docker-compose.dev.yaml up    # bring up the stack
```

Per-component:

```sh
# core (+ connector)
cd server/core && make build      # goreleaser release --snapshot --clean -> bin/

# datagen
cd datagen && make build

# auth-realtime
cd server/auth-realtime && pnpm run dev          # local watch mode (tsx + instrument.ts)
cd server/auth-realtime && pnpm run build        # tsc only
cd server/auth-realtime && pnpm run qa           # check-types + check-lint + check-fmt
cd server/auth-realtime && pnpm run qa-fix       # check-types + lint --fix + prettier --write
```

Go tests live next to their packages. The `db` package's tests spin up a real Postgres container via `gnomock` from `TestMain`, so Docker must be running:

```sh
cd server/core && go test ./...                                # all packages
cd server/core && go test ./internal/db/...                    # db (needs Docker)
cd server/core && go test -run Test_New ./internal/db          # single test
```

Go dependencies are fetched into the module cache at build time (`make deps` runs `go mod download`) — there is no vendoring. All dependencies, including the first-party `github.com/oxynote/wetsocks`, are public; no GOPRIVATE or git auth setup is needed.

## Caddy / port layout

`docker/Caddyfile` is the entrypoint for all client traffic:

- `:8081` — front door. `/go/*` is path-stripped and reverse-proxied to `core:8080`; `/api/*` and `/hocuspocus` go to `auth-realtime:8081` (Better Auth, Hocuspocus, the merge proxy); everything else goes to the web SSR container (`web:3000`).
- `:9001` — MinIO console
- `:5001` — changedetection.io (`5001` to dodge the macOS AirPlay Receiver on `5000`)
- `:3001` — Grafana (direct, not via Caddy; host `3001` because `nuxt dev` owns `3000`)

So from a frontend's point of view: auth + realtime is `:8081/...`, the Go API is `:8081/go/api/...`. Internal-only Go routes (`/api/x/...`) are not exposed by Caddy.

## Core request surface

`server/core/internal/server/router.go` is the canonical map. Three layers:

- `/api/...` (public): auth middleware (`internal/server/auth`) validates sessions by calling auth-realtime's `/api/auth/get-session` (configured via `SERVER_AUTH_BETTER_AUTH_URL`). Routes for documents, branches, comments, reviewers, hooks, files, GitHub, Slack, notifications, data-sources, AI chat.
- `/api/x/...` (internal): no auth at all — reverse proxy must firewall. Used by auth-realtime to fetch/store branch content (`/x/documents/{id}/branches`, `/x/documents/{id}/branch/{branchId}`), trigger emails, initialize orgs, and receive GitHub/Slack webhooks.
- WebSocket topics under `/api/ws`: `change@document-tree`, `change@documents.{documentId}.comments|metadata|reviewers|maintainers`, `post@slack.messages`, `creation@notifications`, `ping@version`. Topic binders live on the per-domain handler types (`*handler.Handler.BindXxx`).

Most public routes in the README (`/api/documents`, `/api/documents/tree`, etc.) are served by core; the README is the closest thing to a contract spec — when changing handlers, update it.

## Document storage / Hocuspocus integration

This is the model that's least obvious from a fresh read:

- Documents are organized into **branches** (`document_branches` table). Every document has at least one branch; the oldest = the default/main branch.
- The Hocuspocus `documentName` is encoded as `"<documentId>-<branchIdentifier>"`. `branchIdentifier` may be the literal string `"default"`, in which case `resolveBranchId` in `server/auth-realtime/src/hocuspocus.ts` looks up the default branch ID. Splitting on the **first** `-` (`indexOf("-")`) is intentional; XIDs do not contain dashes but the suffix path can.
- Branch content is stored two ways in `document_branches`: structured ProseMirror JSON in `content` (JSONB) and the canonical Yjs binary state in `raw_content` (base64-encoded over the wire). `raw_content` is authoritative for CRDT continuity.
- `onLoadDocument` / `onStoreDocument` round-trip through core's internal `/api/x/documents/{id}/branch/{branchId}` endpoints. **Do not** use `Y.applyUpdate` to seed a freshly-created doc with content from another doc — the differing `clientID`s CRDT-merge and silently duplicate content. The codebase's pattern is `replaceYdocContent` in `server/auth-realtime/src/ydocument.ts`, which deep-clones `Y.XmlElement`s (preserving non-string attrs that `XmlElement.clone()` drops). Read the comments at `server/auth-realtime/src/hocuspocus.ts:154` and `server/auth-realtime/src/routes.ts:73` before touching this area.
- Branch merging: `PUT /documents/:documentId/merge` is handled by auth-realtime, which proxies to core, then **directly mutates the in-memory target-branch Y.Doc** via `replaceYdocContent` (not `applyUpdate`) and immediately persists `rawContent` back through `/api/x/.../branch/:branchId` so a server restart can't reset the clientID and duplicate content on reconnect.

The ProseMirror schema is defined in `server/auth-realtime/src/schema/` (one file per block kind, `index.ts` aggregates). The Go-side mirror is `server/core/internal/document/node.go` (`RootBlock`, `Block`, `Mark`).

## Database

Postgres (image `pgvector/pgvector:pg16`). Migrations are embedded in the core binary from `server/core/internal/db/migrations/` and applied automatically by `db.New` on startup via `rubenv/sql-migrate`. Add new migrations as the next-numbered `NNN_<name>.sql` file.

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
- All integrations are optional. The GitHub App is enabled by setting `OXYNOTE_CORE_GITHUB_APP_ID` and dropping the app's private key into `docker/github/` (mounted into the core container); an empty `OXYNOTE_CORE_GITHUB_APP_ID` disables it — core boots, GitHub routes respond `github.not_configured` (the always-200 `GET /api/github` status endpoint reports `configured: false` as the frontend's capability signal), and github-tracking hooks are skipped. A set app ID with a missing/unreadable key is a boot error. Slack works the same way keyed on `OXYNOTE_CORE_SLACK_CLIENT_ID` (`slack.not_configured`, `GET /api/slack` reports `configured`); a set client ID with other `SLACK_*` values missing is a boot error. The assistant needs `ANTHROPIC_API_KEY`; without it the AI chat can't complete a turn.

## House conventions worth knowing

- Errors / closers in `cmd/*/main.go` use a hand-rolled `closers []io.Closer` slice and `ioutil.MultiCloser(true, closers...).Close()` to fan-out on shutdown / wrap construction errors. New top-level resources should follow the same pattern (prepend to slice if it should close before the things that depend on it).
- Logging is `log/slog`, JSON handler, Sentry-wired via `pkg/sentryutil`. Most packages take `*slog.Logger` as their first constructor arg.
- Metrics use `pkg/metricutil` with a Prometheus registry; each subsystem has its own `Metrics` type initialised from the shared factory.
- `server/core/pkg/` holds shared utility packages: `redkit` (typed Redis streams), `metricutil`, `logutil`, `ioutil`, `sentryutil`, `redisutil`, `httpserver`, `sqlutil` and friends. `wetsocks` (`github.com/oxynote/wetsocks`) is a first-party public library; `wetsocks/wsserver` is the WebSocket router used by `/api/ws`.
- IDs are `rs/xid` everywhere on the Go side. Frontend-facing types serialise them as strings.

## Go code style

### Doc comments

Every exported symbol — struct, struct field, method, function, interface — gets a `//` doc comment. Field-level comments are required even on lightweight value types. Write comments so they stand alone: don't reference design docs by section number ("§5.4", "M1") or assume the reader has another file open.

### Private package-level globals

Unexported package-level **constants and variables** use `_camelCase` (`_pmText`, `_allowedAtRoot`, `_maxBatchSize`). Apply this on creation, never introduce a bare-lowercase `var allowedThings = …` at package level. **Functions** stay bare-lowercase.

### Tests

Test files pair 1:1 with source files: `expand.go` → `expand_test.go`, never a bundled "package_test.go". When tests span two files, they live in the one that pairs with the *primary* file driving the test (round-trip tests land with the file that initiates the round-trip).

Naming: `Test_{Func}` for free functions, `Test_{Type}_{Method}` for methods. Table-driven, with one field per line (no inline struct literals, even for short cases) and capitalised case names:

```go
tests := map[string]struct{
    In   string
    Want string
}{
    "Empty string": {
        In:   "",
        Want: "",
    },
}

for name, tc := range tests {
    t.Run(name, func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

Same one-per-line rule for multi-element slice/array literals. Single-element literals like `[]string{x}` are fine inline.

### Error checking

When a function returns a sentinel error you want to handle separately, use a **single outer `if err != nil`** with the sentinel branch nested inside:

```go
existing, err := store.FetchX(ctx, id)
if err != nil {
    if errors.Is(err, errutil.ErrNotFound) {
        return nil
    }

    return fmt.Errorf("fetch existing: %w", err)
}
```

Not two sibling ifs — `errors.Is` should never run on the success path.

Don't discard `json.Unmarshal` errors. If a parse failure is genuinely best-effort (e.g. building a label), log a `Warn` with context and continue with the zero-valued target; never `_ = json.Unmarshal(...)`.

### Background goroutines

Long-lived components own a `supv := xync.NewSupervisor()` and spawn via `supv.Go(func(ctx context.Context) { … })`. The component's `Close() error` calls `supv.CloseAndWait()` and gets wired into the cmd-level closer chain. Never `go someFunc()` in a long-lived component. Ctx-bound sleeps use `select { case <-ctx.Done(): ...; case <-time.After(d): }`, never bare `time.Sleep`.

### Persistence layer

- SQL stays in `internal/db`. Other packages define interfaces describing what they need; `db.agent` satisfies them. `db.DB` does **not** expose its underlying `*sqlx.DB`.
- Domain types live in their domain package, not in `db`. When `db` returns shaped rows, define the struct in the consuming domain package and have `db` import it. Never invent `db.XxxRow`.
- Use **squirrel** for almost all SQL: `a.builder.Insert(...).SetMap(...).Suffix("ON CONFLICT ...").MustSql()`, `sq.NotEq{"col": slice}` for `NOT IN`, `sq.Expr` for typed expressions squirrel can't build. Hand-rolled SQL strings only when squirrel can't express the query cleanly.
- Prefer nullable columns + `null.String` (`guregu/null/v5`) for genuinely optional fields rather than `NOT NULL DEFAULT ''` + an empty-string-to-NULL helper.

### HTTP handlers

Handler method names use the **domain verb**, not the HTTP verb. The router file already says `GET`/`PUT`/`POST` — the handler name doesn't repeat it:

- `HandleFetchX` for GET of a single resource
- `HandleFetchXs` for GET of a collection (not `HandleListX`)
- `HandleExtractX` / `HandleReextractX` for action endpoints (not `HandlePutX`)
- `HandleCreateX`, `HandleUpdateX`, `HandleDeleteX` for the obvious CRUD verbs

### Trimming unused exports

When restructuring, audit every exported identifier with `grep -rn "pkg\.Name"`. Anything used only inside its own package becomes lowercase (functions) or `_camelCase` (constants/vars). Doubly-unused things get deleted.

## Assistant prompt

The system prompt at `server/core/internal/assistant/prompt.go` codifies behaviour for the Rubber Duck AI chat. When fixing a behaviour bug, state the underlying principle in one or two sentences. Don't enumerate edge cases or add numbered steps — spelling every case out drowns the core guidance in noise. Worked examples and tables belong in the prompt only when the model genuinely needs the structure to anchor a concept (the block-schema table and the split_doc example qualify; most rules don't).

## Node formatting & TS

Prettier uses **tabs (width 8)**, no semicolons, trailing commas, double-quoted strings — see [auth-realtime/prettier.config.js](auth-realtime/prettier.config.js). ESLint extends `@eslint/js` recommended + `typescript-eslint` **strict** + **stylistic** + `eslint-config-prettier`; `@typescript-eslint/no-explicit-any` is **off**. See [auth-realtime/eslint.config.mjs](auth-realtime/eslint.config.mjs).

TypeScript is `module: NodeNext`, `target: ESNext`, fully strict — `strict`, `noImplicitAny`, `noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`, `noImplicitOverride`, `verbatimModuleSyntax` all on. See [auth-realtime/tsconfig.json](auth-realtime/tsconfig.json). Path alias `@/*` points to `server/auth-realtime/src/*`.

**ES module imports must include the explicit `.js` extension** (NodeNext + ESM requirement), even when importing TypeScript files — e.g. `import { foo } from "./bar.js"` for a `./bar.ts` source file.

