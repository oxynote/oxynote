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

---

## Project

**Oxynote** (codename **Bifrost**) — Nuxt 4 + Vue 3 frontend that ships as both a web app (Cloudflare Pages, SSR) and an Electron desktop app (static SPA).

## Common commands

Package manager is **pnpm** (workspace; uses `nodeLinker: hoisted` so Electron Forge's npm-style layout works).

```bash
pnpm install                  # installs deps; runs `prepare` (builds lezer-promql + nuxt prepare)

pnpm start:dev:web            # nuxt dev on :3000 (web build)
pnpm start:dev:desktop        # concurrently runs nuxt dev + electron-forge start with DESKTOP_BUILD=hybrid
pnpm build:web                # production web build (cloudflare_pages preset)
pnpm package:desktop          # nuxt generate (static) + electron-forge package
pnpm make:desktop             # nuxt generate (static) + electron-forge make (installers)

pnpm qa                       # check-types + check-lint + check-fmt (read-only)
pnpm qa-fix                   # check-types + lint --fix + prettier --write
pnpm check-types              # vue-tsc --noEmit
pnpm lint                     # eslint --fix .
pnpm fmt                      # prettier --write .

pnpm build:lezer-promql       # rebuild the forked PromQL grammar package
```

There is no test runner configured in this repo.

## Build modes (critical)

The `DESKTOP_BUILD` env var drives a Vite `define` that materializes `__DESKTOP_BUILD__` in the bundle. See [nuxt.config.ts:9-24](nuxt.config.ts#L9-L24) and [index.d.ts:1-9](index.d.ts#L1-L9):

- `DESKTOP_BUILD=0` / unset → web build. `__DESKTOP_BUILD__` is literal `false`. SSR enabled. Nitro preset `cloudflare_pages`.
- `DESKTOP_BUILD=1` → pure desktop build. Literal `true`. SSR off. Nitro preset `static`. Renderer is served from `oxynote://app/index.html` out of `.output/public/`.
- `DESKTOP_BUILD=hybrid` → dev-only, used by `start:dev:desktop`. One Nuxt dev server is hit by *both* the Electron renderer and the system browser opened for OAuth. `__DESKTOP_BUILD__` becomes a *runtime probe* of `window.__isElectron` (set by [electron/preload.ts](electron/preload.ts)) so each context picks the right branch from the same bundle.

When writing code that branches on platform, always use `__DESKTOP_BUILD__` — never `process.platform`, `import.meta.client`, or feature-detect on `window`. The literal-substitution is the security boundary: in the desktop bundle, no code path can carry a session cookie.

## Auth architecture

Uses **Better Auth** with the `@better-auth/electron` plugin. Two distinct flows:

- **Web**: `app/plugins/02.auth.ts` instantiates a standard `createAuthClient` with `credentials: "include"`. Session cookies flow normally.
- **Desktop**: the renderer's `createAuthClient` is forced to `credentials: "omit"`. The real session lives in **main**'s [electron/auth-client.ts](electron/auth-client.ts) backed by `electron-store` (encrypted via `safeStorage` + Electron's `EnableCookieEncryption` fuse). The renderer reaches auth only through `window.__host.auth.*` IPC bridges defined in [electron/auth-ipc.ts](electron/auth-ipc.ts) and [electron/preload.ts](electron/preload.ts). To add a new auth operation: add a handler in `auth-ipc.ts`, expose it in `preload.ts`, type it in [index.d.ts](index.d.ts), and add the desktop branch in [app/composables/useAuthSession.ts](app/composables/useAuthSession.ts).

Main's [electron/main.ts](electron/main.ts) injects the session cookie into renderer-originated requests via `webRequest.onBeforeSendHeaders` — this is why the renderer never holds the cookie. The `/api/auth/*` path is carved out of that injection because those calls must keep going through the IPC bridge.

OAuth flow on desktop: renderer calls `window.requestAuth({ provider })` → opens system browser at `${APP_BASE_URL}/login` → web flow completes → server redirects to `oxynote://` deep-link → OS focuses Electron → Better Auth's `setupMain()` exchanges the code → main fires `onAuthenticated` over the bridge → [app/plugins/electron-auth.client.ts](app/plugins/electron-auth.client.ts) refetches session and navigates.

## API clients

`app/plugins/03.api-fetch.ts` provides two `$fetch` instances:

- `$apiClient` → Go API (`NUXT_PUBLIC_GO_API_BASE_HTTP_URL`)
- `$nodejsAPIClient` → Node API (`NUXT_PUBLIC_NODEJS_API_BASE_HTTP_URL`)

Both propagate SSR request headers (captured eagerly during plugin setup — the H3 context is lost inside `onRequest` callbacks on Cloudflare workerd) and redirect to `/login` on 401.

## Data layer

**Pinia Colada** (`useQuery` / `useMutation`) is the standard data-fetching primitive. Auto-refetch plugin is configured but *disabled by default* — opt in per-query with `autoRefetch: true` (typed augmentation in [index.d.ts:34-38](index.d.ts#L34-L38)). See [colada.options.ts](colada.options.ts).

API composables live in [app/composables/api/](app/composables/api/) and are re-exported by [app/composables/index.ts](app/composables/index.ts) for auto-import. Request/response types live in [app/utils/api/](app/utils/api/) and are re-exported by [app/utils/index.ts](app/utils/index.ts).

## Editor (TipTap + Yjs)

Document editor is in [app/components/editor/](app/components/editor/). Real-time collaboration uses **Yjs** + **Hocuspocus** (`NUXT_PUBLIC_NODEJS_API_BASE_WS_URL`). Notable subsystems:

- `blocks/` — custom node types (mermaid, metrics, code-block, figma, image, callout, split-documentation)
- `comments/` — comment marks + node-comment extension
- `diff/` — branch diffing UI (compute, render, decorations)
- `drag-handle/`, `slash/`, `link/`, `ai/` — editor UX extensions
- `hooks/` — block-level integrations (github tracking, container image watcher, scheduled reminders, URL watcher)

Editor-wide state (active document, branch, locks, metric configs, diff statuses) lives in [app/stores/editor.ts](app/stores/editor.ts).

## Routing

Single dynamic page handles the workspace: [app/pages/[[organizationSlug]]/[[documentSlug]].vue](app/pages/[[organizationSlug]]/[[documentSlug]].vue). The global middleware [app/middleware/01.redirect.global.ts](app/middleware/01.redirect.global.ts) handles auth gating, onboarding, and root-path redirection to the first document.

Pages with `definePageMeta({ skipAuth: true })` are reachable when signed-out (login, signup, accept-invite, verify-email, desktop-auth).

## i18n

All user-facing text **must** live under [i18n/locales/en/](i18n/locales/en/) — never inline. The setup combines multiple JSON files into one big i18n object, so each file must have a **root namespace key** (e.g. `{ "sidebar": { ... } }`). To add a new namespace, also register the filename in `nuxt.config.ts` under `i18n.locales[0].files`.

Form validation (vee-validate) uses its own internal messages — see [README.md](README.md).

## UI

- **shadcn-vue** components live in [app/components/shadcn/ui/](app/components/shadcn/ui/) with prefix `ShadcnUi`. Configured in [components.json](components.json).
- **Tailwind v4** via `@tailwindcss/vite`, theme in [app/assets/css/main.css](app/assets/css/main.css).
- Icons via `@nuxt/icon` (CSS mode) — only icons listed by `selectableIconList()` in [app/utils/icon.ts](app/utils/icon.ts) are bundled client-side. Custom SVGs live in [app/assets/custom-icons/](app/assets/custom-icons/) and are reachable under the `custom-icons:` prefix.

## Formatting & TS

Prettier uses **tabs**, no semicolons, trailing commas — see [prettier.config.js](prettier.config.js). ESLint extends Nuxt's flat config; `@typescript-eslint/no-explicit-any` is **off**.

TypeScript strictness (set in [nuxt.config.ts:96-112](nuxt.config.ts#L96-L112)): `noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`, `noImplicitOverride`, `verbatimModuleSyntax` all on; `noImplicitAny` off. Path aliases `@/*` and `~/*` both point to `app/*`.

## Code style

### Comments

- Only comment **complex implementations, edge cases, and locations that would confuse a future reader**. Do not add comments that restate what the code does, and do not add comments just to acknowledge a prompt or show that you understood the task.
- Always use `//` single-line comments. Never use `/* */` — not even for multi-line blocks.
- Wrap comments at 80 characters. Continuation lines also start with `// `.
- Within a single comment, the **first sentence starts with a lowercase letter**; every following sentence starts with an uppercase letter. Sentences are still separated with proper punctuation.

  Example:
  ```ts
  // strip the leading "; " that getCookie always emits. Downstream code
  // expects a bare cookie pair, not a header-prefix fragment.
  ```

### Whitespace in TS/JS

Leave a blank line **before and after each logical block**: function definitions, `if`/`else` branches, `for`/`while` loops, `try`/`catch`, `switch` blocks, etc. The goal is visual separation between distinct units of logic — not blank lines between every statement.

### Vue `<script setup>` ordering

Within a Vue setup script, order the contents as follows:

1. Imports
2. `defineProps`, `defineEmits`, `defineExpose`
3. Store and composable initializations (`useFoo()`, `useStore()`, etc.)
4. `ref` and `computed` declarations
5. `onMounted` / `onUnmounted` (and other lifecycle hooks)
6. Watchers (`watch`, `watchEffect`, `watchImmediate`)
7. Functions

### Styling

Use **Tailwind utility classes** by default. When a utility expression can't express what you need, add the custom rule to [app/assets/css/main.css](app/assets/css/main.css) and apply it via a class name — do not use `<style>` blocks in components or inline `style=` attributes for non-dynamic values.

## Forked package: lezer-promql

[packages/lezer-promql/](packages/lezer-promql/) is a fork of Prometheus's PromQL grammar that adds Grafana-style dynamic duration placeholders (`$__interval`, etc.). It's aliased into Vite as `@prometheus-io/lezer-promql`. Rebuild with `pnpm build:lezer-promql`. Upstream sync procedure is documented in [packages/README.lezer-promql.md](packages/README.lezer-promql.md) — do not modify the grammar without reading it.

## Electron packaging notes

`forge.config.ts` allowlists only `/.vite` (compiled main+preload) and `/.output/public` (renderer SPA) into the package — anything else is excluded. The `oxynote://` protocol is registered at install time for OAuth deep-links. Production Fuses disable `RunAsNode`, `EnableNodeOptionsEnvironmentVariable`, and require ASAR integrity. Main and preload are forced to `.cjs` output (see [vite.electron.config.ts](vite.electron.config.ts)) because root `package.json` is `"type": "module"`.

`__API_BASE_URL__` and `__APP_BASE_URL__` are **baked in at build time** for the Electron bundle — runtime env cannot override them. The build fails if those env vars are missing.
