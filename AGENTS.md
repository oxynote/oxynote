# AGENTS.md

Guidance for AI coding agents working in this repository. Each component
directory has its own AGENTS.md; read it before working there.

## 1. Think before coding

State your assumptions. If several interpretations exist, present them
rather than picking one silently. If a simpler approach exists, say so. If
something is unclear, stop and ask.

## 2. Simplicity first

The minimum code that solves the problem: no features beyond the ask, no
abstractions for single-use code, no unrequested configurability, no error
handling for impossible cases. If 200 lines could be 50, rewrite.

## 3. Surgical changes

Touch only what the request needs. Do not improve adjacent code, comments
or formatting, do not refactor what is not broken, and match the existing
style. Mention unrelated dead code; do not delete it. Remove only the
imports, variables and functions your own change made unused.

**Leave no containers running.** Stop every stack or container you started
before finishing, including after a failure or interruption (`make e2e-*`
tears its own down). Leave up only a stack that was already running or that
the user asked to keep.

## 4. Comments serve the reader, not the requester

A comment explains the code as it stands. Never address it to whoever asked
for the change ("as requested", "deliberately without X"), never narrate
what changed or moved, never justify at paragraph length. Comment genuine
traps: a non-obvious invariant, an upstream bug, an ordering that looks
arbitrary and is not. Test: does it still earn its lines for someone reading
the file a year from now with no idea what was asked? The same test applies
to the prose in these AGENTS.md files.

## 5. Goal-driven execution

Turn the task into a verifiable goal ("fix the bug" becomes a test that
reproduces it, then passes). For multi-step work, state a short plan with a
check per step and loop until every check passes.

## 6. AGENTS.md itself

- If a request contradicts this file or a nested one, name the conflict and
  ask which side wins. Once the new direction is confirmed, update the
  affected section in the same task.
- These files hold only what cannot be learned from the code, the READMEs
  or git history: conventions, invariants, traps, and where things live.
  Before adding a line, check that it is not already stated, not readable
  from the code it describes, and not the story of one change. Write the
  rule; a reason only where the rule would otherwise look arbitrary.
- Some tools load at most 32 KiB of AGENTS.md per session: this file plus
  every AGENTS.md on the path to the working directory. This file plus any
  component's file stay under 32 KiB together; a package-level file below a
  component stays under 5 KiB (`wc -c`). When a file grows, cut before
  adding.

---

## Repository map

Oxynote is a collaborative documentation product. One repo, four buildable
components:

- `web/` — Nuxt 4 + Vue 3; ships as a web app (SSR) and an Electron desktop
  app. Own pnpm workspace. [web/AGENTS.md](web/AGENTS.md).
- `server/core/` — Go API server, module
  `github.com/oxynote/oxynote/server/core`. Architecture in
  [server/AGENTS.md](server/AGENTS.md), Go standards in
  [server/core/AGENTS.md](server/core/AGENTS.md).
- `server/auth-realtime/` — Node service: Better Auth and Hocuspocus (Yjs)
  in one Hono process. pnpm.
  [server/auth-realtime/AGENTS.md](server/auth-realtime/AGENTS.md).
- `datagen/` — demo-data generator, separate Go module.
- `e2e/` — Playwright suite plus the two compose stacks it drives, `dev`
  (built from this repo) and `prod` (the all-in-one image); neither is an
  unnamed default. [e2e/AGENTS.md](e2e/AGENTS.md).
- `docker/` — dev compose stack, Caddyfile, `env/` (committed
  `*.example.env` templates; `make setup` copies them to the gitignored
  `*.local.env` the stack reads), `demo/` data configs.
- `docker/prod/` — the production all-in-one image, its Caddyfile, the TS
  launcher and the example compose. [docker/prod/AGENTS.md](docker/prod/AGENTS.md).
- `scripts/` — helpers the root Makefile calls.

One `.gitignore`, at the repository root. Nested rules carry their path
(`e2e/test-results/`).

## Common commands

From the repository root:

```sh
make deps / setup / setup-force   # deps; + env files from templates; + overwrite them
make run / start / stop           # dev stack foreground / background / stop
make dev                          # backend containers + web dev server on :3000
make check-env / sync-env         # report / reconcile *.local.env against templates
make lint / check-lint            # fix / verify lint, format and types everywhere
make prod-build / prod-run / prod-stop         # all-in-one image on :8080 (mailpit :8025)
make e2e-{dev,prod}[-stack-build|-stack-stop]  # dev stack :18080, prod image :19080
```

`make prod-publish` is release-only (`.github/workflows/release.yml`).
Component commands are in the nested files.

## Cross-component contracts

- **Front door**: Caddy on `:8080`. `/core/*` is path-stripped to core
  (`:8080`), `/auth-realtime/*` to auth-realtime (`:8081`), everything else
  to web SSR. The frontend uses `NUXT_PUBLIC_CORE_API_BASE_HTTP_URL` and
  `NUXT_PUBLIC_AUTH_REALTIME_API_BASE_*_URL`.
- **Trust boundary**: `/api/...` needs a session. Core's `/api/x/...` and
  auth-realtime's `/api/internal/...` have no auth, exist for
  service-to-service calls, and are blocked by the Caddyfile; never expose
  them. Fork, branch update, merge and branch delete are `/api/x` routes
  that are also session-authed and reachable only through auth-realtime
  (see [server/AGENTS.md](server/AGENTS.md)). `/api/apps/...` is public and
  sessionless (GitHub/Slack webhooks and OAuth callbacks, proven by
  signature or encrypted state). `/api/mcp` takes OAuth 2.1 bearer tokens
  issued by auth-realtime and validated against its internal MCP session
  endpoint.
- **Session validation**: core calls auth-realtime's `/api/auth/get-session`;
  auth-realtime owns the Better Auth schema.
- **Yjs invariant**: the Hocuspocus `documentName` is
  `"<documentId>-<branchIdentifier>"`, split on the first `-`. Never seed or
  merge one Y.Doc from another with `Y.applyUpdate`; use `replaceYdocContent`
  (`server/auth-realtime/src/ydocument.ts`). Read the document-storage
  section of [server/AGENTS.md](server/AGENTS.md) before touching branch
  content.
- **System writes**: a protected branch accepts only core's own persists,
  marked through `edit.Client.Apply`'s `system` argument and recognised by
  `onStoreDocument` only while every change came from core. Protection is
  enforced at the connection (`onAuthenticate` makes it read-only), because
  a persist carries whole-document state.
- **Env naming**: core reads `OXYNOTE_CORE_*` (via `buildinfo.Getenv`),
  auth-realtime `OXYNOTE_AUTH_REALTIME_*`, the frontend `NUXT_PUBLIC_*`.
- **`_DSN` vs `_URL`**: a connection string carrying credentials ends in
  `_DSN`; a plain address ends in `_URL` and any credential is its own
  variable (`MEILISEARCH_URL` + `_MASTER_KEY`). The suffix says whether the
  value is a secret, so the same dependency is never `_DSN` on one side of
  the trust boundary and `_URL` on the other.

## Code style (TS/JS — web and auth-realtime)

- Comment only complex implementations, edge cases and places that would
  confuse a future reader, under section 4 above.
- `//` comments only, never `/* */`, wrapped at 80 characters.
- Never reference AGENTS.md or another agent-instruction file from a comment;
  inline the rule instead. Links to real docs (READMEs, upstream issues) are
  fine.
- Within a comment the first sentence starts lowercase and later sentences
  uppercase (`// strip the leading "; " that getCookie emits. Downstream
  code expects a bare pair.`), unless the first word is an identifier.
- A blank line before and after each logical block (function, `if`/`else`,
  loop, `try`/`catch`, `switch`), not between every statement. An `if` or
  `switch` follows the declaration of the value it checks directly, with no
  blank line between them.

Go style and Vue conventions live in the nested files.
