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

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

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

## 5. Keep This File in Sync

**If a request contradicts CLAUDE.md, surface it. If the rule changes, update the file.**

- When the user asks for something that contradicts this file (or a nested CLAUDE.md), don't silently comply and don't silently refuse — name the conflict and ask which side wins.
- If the user confirms the new direction, update the affected CLAUDE.md section as part of the same task, so the documentation never drifts from how the code actually works.

---

## Repository map

**Oxynote** — collaborative documentation product. One repo, four buildable components:

- `web/` — Nuxt 4 + Vue 3 frontend; ships as both a web app (SSR) and an Electron desktop app. Own **pnpm** workspace (includes the forked `packages/lezer-promql`). Details: [web/CLAUDE.md](web/CLAUDE.md).
- `server/core/` — Go API server (`oxynote-core`) plus `oxynote-connector` for outbound data-source connections. Vendored Go module `github.com/oxynote/oxynote/server/core`. Details: [server/CLAUDE.md](server/CLAUDE.md).
- `server/auth-realtime/` — Node service (`@oxynote/auth-realtime`, **npm**) running Better Auth and a Hocuspocus (Yjs) server in one Hono process. Details: [server/CLAUDE.md](server/CLAUDE.md).
- `datagen/` — demo-data generator; separate Go module `github.com/oxynote/oxynote/datagen`. Demo/testing only, ships as its own Docker image.
- `docker/` — docker-compose stack, Caddyfile, env files. Real env files (`.env.core`, `.env.auth-realtime`) are gitignored; `.env.*.example` templates are committed.

## Common commands

From the repository root:

```sh
make deps      # install web/ (pnpm) + auth-realtime (npm) dependencies
make setup     # same as deps, plus web prepare (lezer-promql build + nuxt prepare)

docker-compose -p oxynote -f docker/docker-compose.yaml up   # backend stack
```

Component build/test/qa commands are listed in the nested CLAUDE.md files.

## Cross-component contracts

- **Front door**: Caddy on `:8081`. `/go/*` is path-stripped and proxied to core (`:8080`); everything else goes to auth-realtime (`:8081`). The frontend reaches the Go API via `NUXT_PUBLIC_GO_API_BASE_HTTP_URL` (`…:8081/go`) and auth/realtime via `NUXT_PUBLIC_NODEJS_API_BASE_*_URL`.
- **Trust boundary**: core's `/api/...` routes require a session; `/api/x/...` routes have no auth and must never be exposed by the reverse proxy — they exist for service-to-service calls (auth-realtime ↔ core).
- **Session validation**: core's auth middleware validates sessions by calling auth-realtime's `/api/auth/get-session`; auth-realtime owns the Better Auth schema.
- **Yjs invariant**: the Hocuspocus `documentName` is `"<documentId>-<branchIdentifier>"`, split on the first `-`. Never seed or merge one Y.Doc from another with `Y.applyUpdate` — use `replaceYdocContent` (`server/auth-realtime/src/ydocument.ts`). Read the document-storage section of [server/CLAUDE.md](server/CLAUDE.md) before touching branch content anywhere.
- **Env naming**: core reads `OXYNOTE_CORE_*` (via `buildinfo.Getenv`), auth-realtime reads `OXYNOTE_AUTH_REALTIME_*`, the frontend reads `NUXT_PUBLIC_*`.

## Code style (TS/JS — web and auth-realtime)

### Comments

- Only comment **complex implementations, edge cases, and locations that would confuse a future reader**. Do not add comments that restate what the code does, and do not add comments just to acknowledge a prompt or show that you understood the task.
- Always use `//` single-line comments. Never use `/* */` — not even for multi-line blocks.
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
