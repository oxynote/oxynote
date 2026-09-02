# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

**Leave no containers running.** If you start a stack or a container while
working — `make start`, `make dev`, `make prod-run`, a bare `docker compose
up`, a one-off `docker run` — stop it before you finish, including when the
task failed or you are interrupted. The `make e2e-*` targets tear their own
stack down; anything you started by hand is yours to stop. A stack left up
holds ports, burns battery and silently changes what the next command sees.
The exception is a stack that was already running when you started, or one
the user asked you to leave up.

The test: Every changed line should trace directly to the user's request.

## 4. Comments Serve the Reader, Not the Requester

**A comment explains the code. It never answers the prompt.**

The reader has never seen the conversation that produced the change, and
never will. Write only what they need to work on the code in front of them.

- **Never comment a decision to whoever asked for it.** No "deliberately
  without X", no "as requested", no note explaining why something is absent,
  removed, or named the way it is. Code says what it does; a comment is for
  what the code cannot say.
- **Never narrate the change.** A comment describes the code as it stands,
  not what it used to be or what moved where. That is what git is for.
- **Keep it to a line or two.** A paragraph of justification means either
  the code needs the work, or the comment is arguing with a reviewer who
  will never read it.
- Comment a genuine trap: a non-obvious invariant, an upstream bug, an
  ordering that looks arbitrary and is not.

The test: would this still make sense, and still earn its lines, to someone
reading the file a year from now with no idea what was asked?

The same applies to the prose in these CLAUDE.md files — write the rule, not
the story of how it came up.

## 5. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

## 6. Keep This File in Sync

**If a request contradicts CLAUDE.md, surface it. If the rule changes, update the file.**

- When the user asks for something that contradicts this file (or a nested CLAUDE.md), don't silently comply and don't silently refuse — name the conflict and ask which side wins.
- If the user confirms the new direction, update the affected CLAUDE.md section as part of the same task, so the documentation never drifts from how the code actually works.

---

## Repository map

**Oxynote** — collaborative documentation product. One repo, four buildable components:

- `web/` — Nuxt 4 + Vue 3 frontend; ships as both a web app (SSR) and an Electron desktop app. Own **pnpm** workspace. Details: [web/CLAUDE.md](web/CLAUDE.md).
- `server/core/` — Go API server (`oxynote-core`). Go module `github.com/oxynote/oxynote/server/core`. Details: [server/CLAUDE.md](server/CLAUDE.md).
- `server/auth-realtime/` — Node service (`@oxynote/auth-realtime`, **pnpm**) running Better Auth and a Hocuspocus (Yjs) server in one Hono process. Details: [server/auth-realtime/CLAUDE.md](server/auth-realtime/CLAUDE.md).
- `datagen/` — demo-data generator; separate Go module `github.com/oxynote/oxynote/datagen`. Demo/testing only.
- `e2e/` — Playwright end-to-end suite (`@oxynote/e2e`, **pnpm**) plus the two docker-compose stacks it drives: the dev stack built from this repo, and the all-in-one image `docker/prod/` produces. Not shipped; it exercises the composed product through a real backend. Every file, script and make target belonging to one stack is named `dev` or `prod` — neither is the unnamed default. Details: [e2e/CLAUDE.md](e2e/CLAUDE.md).
- `scripts/` — helpers the root Makefile calls. `run-quietly.sh` runs a build step with its output held back, replaying the log only if the step fails.
- `docker/` — dev docker-compose stack, Caddyfile, `env/` (committed `*.example.env` templates; `make setup` copies them to the gitignored `*.local.env` files the compose stack reads, and `web.example.env` also to `web/.env` for the host dev server and electron builds; `sync-env.sh` reconciles an existing `*.local.env` with its template, since `make setup` only copies when the local file is missing, and `make setup-force` copies over them regardless), `demo/` (demo-data configs for mariadb/postgres).
- `docker/prod/` — the production all-in-one image: alpine-based Dockerfile (built via `make prod-build`; core comes from goreleaser), the image's Caddyfile (sibling of `docker/Caddyfile`), the TS launcher (`@oxynote/launcher`, **pnpm**) that validates the flat public `OXYNOTE_*` env, generates internal secrets, and supervises caddy + web + core + auth-realtime, plus the example compose deployment and the local override `make prod-run` layers on it. Details: [docker/prod/CLAUDE.md](docker/prod/CLAUDE.md).

**One `.gitignore`, and it lives at the repository root.** Components do not carry their own — a rule for a nested directory is written with its path (`e2e/test-results/`, `web/coverage/`), so every exclusion in the repository is readable in one file.

## Common commands

From the repository root:

```sh
make deps      # install web/ + auth-realtime (pnpm) and go module dependencies
make setup     # deps + web prepare + creates docker/env/*.local.env from templates
make setup-force # setup, but *.local.env is rewritten from the templates, values and all
make run       # build images + run the dev stack in the foreground (ctrl-c stops it)
make start     # build images + run the dev stack in the background
make dev       # backend containers + web dev server on the host (hot reload, :3000)
make stop      # stop the dev stack
make check-env # report variables that drifted between *.example.env and *.local.env
make sync-env  # rewrite *.local.env from the templates, keeping existing values

make lint      # fix lint/format/type issues in web, auth-realtime, core, datagen, e2e, launcher
make check-lint # the same gates, verification only

make prod-build # goreleaser core binary + the all-in-one prod image
make prod-run   # build the image + run the example compose on :8080, with
                # a mailpit on :8025 so signup mail is readable
make prod-stop  # stop it
# release-only, run by .github/workflows/release.yml on a tag:
# make prod-publish RELEASE_VERSION=1.2.3  -> pushes :latest and :1.2.3

make e2e-dev             # one-shot: build, run playwright, tear the dev stack down
make e2e-dev-stack-build # build that stack's images, then iterate with `pnpm test:dev`
make e2e-dev-stack-stop  # stop it and drop its data

make e2e-prod             # the same suite against the all-in-one image
make e2e-prod-stack-build # build that image, then iterate with `pnpm test:prod`
make e2e-prod-stack-stop  # stop it and drop its data
```

The e2e dev stack listens on `:18080` (and mailpit on `:18025`) and the prod one on `:19080` (`:19025`), so all three stacks run alongside each other rather than fighting for ports.

Component build/test/qa commands are listed in the nested CLAUDE.md files.

## Cross-component contracts

- **Front door**: Caddy on host `:8080`. `/core/*` is path-stripped and proxied to core (`:8080`); `/auth-realtime/*` is path-stripped and proxied to auth-realtime (`:8081`, so `/auth-realtime/api/...` and `/auth-realtime/hocuspocus` publicly); everything else goes to the web SSR container. The frontend reaches the core API via `NUXT_PUBLIC_CORE_API_BASE_HTTP_URL` (`…:8080/core`) and auth/realtime via `NUXT_PUBLIC_AUTH_REALTIME_API_BASE_*_URL` (`…:8080/auth-realtime`).
- **Trust boundary**: core has four surfaces. `/api/...` requires a session. `/api/x/...` (core) and `/api/internal/...` (auth-realtime) have no auth and must never be exposed by the reverse proxy (the Caddyfile blocks both at the front door) — they exist for service-to-service calls (auth-realtime ↔ core). `/api/apps/...` is public and sessionless: GitHub and Slack deliver webhooks and OAuth callbacks there, and every request proves itself with the provider's signature or an encrypted state parameter. `/api/mcp` is the MCP surface: OAuth 2.1 bearer tokens issued by auth-realtime (which is the authorization server via `@better-auth/mcp`), validated per request against auth-realtime's internal MCP session endpoint; the Caddyfile also routes the front-door `/.well-known/oauth-*` discovery documents and `/api/auth/*` OAuth endpoints to auth-realtime.
- **Session validation**: core's auth middleware validates sessions by calling auth-realtime's `/api/auth/get-session`; auth-realtime owns the Better Auth schema.
- **Yjs invariant**: the Hocuspocus `documentName` is `"<documentId>-<branchIdentifier>"`, split on the first `-`. Never seed or merge one Y.Doc from another with `Y.applyUpdate` — use `replaceYdocContent` (`server/auth-realtime/src/ydocument.ts`). Read the document-storage section of [server/CLAUDE.md](server/CLAUDE.md) before touching branch content anywhere.
- **Env naming**: core reads `OXYNOTE_CORE_*` (via `buildinfo.Getenv`), auth-realtime reads `OXYNOTE_AUTH_REALTIME_*`, the frontend reads `NUXT_PUBLIC_*`.
- **`_DSN` vs `_URL`**: a variable holding a connection string with credentials in it ends in `_DSN` — `DB_DSN`, `VALKEY_DSN`, `SMTP_DSN`, `OBJECT_STORAGE_DSN` (public), `SENTRY_DSN`. A variable holding a plain address ends in `_URL`, and any credential it needs is its own variable — `MEILISEARCH_URL` + `_MASTER_KEY`, `CHANGE_DETECTION_URL`/`CHANGEDETECTION_API_URL` + its key, `OBJECT_STORAGE_URL` + key/secret on core's side, plus every address that is not a dependency at all (`PUBLIC_URL`, `BASE_APP_URL`, `TERMS_OF_SERVICE_URL`). The suffix is a claim about whether the value is a secret, so it holds across the trust boundary: the same dependency is never `_DSN` publicly and `_URL` internally.

## Code style (TS/JS — web and auth-realtime)

### Comments

- Only comment **complex implementations, edge cases, and locations that would confuse a future reader** — under the rules in "Comments Serve the Reader, Not the Requester" above, which apply to every language in the repository.
- Always use `//` single-line comments. Never use `/* */` — not even for multi-line blocks.
- **Never reference CLAUDE.md or any other agent-instruction file from a comment.** Comments stand alone for any reader, and CLAUDE.md is agent-facing documentation, not part of the codebase's own narrative. Where a comment would say "see CLAUDE.md", inline the actual rule or rationale instead. Links to real project docs (READMEs, upstream issues, specs) stay fine.
- Wrap comments at 80 characters. Continuation lines also start with `// `.
- Within a single comment, the **first sentence starts with a lowercase letter**; every following sentence starts with an uppercase letter. Sentences are still separated with proper punctuation. However, if the first word is the name of a class/function, use the casing of that class/function and don't override it.

  Example:
  ```ts
  // strip the leading "; " that getCookie always emits. Downstream code
  // expects a bare cookie pair, not a header-prefix fragment.
  ```

### Whitespace

Leave a blank line **before and after each logical block**: function definitions, `if`/`else` branches, `for`/`while` loops, `try`/`catch`, `switch` blocks, etc. The goal is visual separation between distinct units of logic — not blank lines between every statement.

Go style, Vue conventions, and per-component formatting configs live in the nested CLAUDE.md files.
