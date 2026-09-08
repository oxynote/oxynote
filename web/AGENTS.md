# AGENTS.md

Guidance for the `web/` frontend. Shared principles and TS/JS style live in
the root [AGENTS.md](../AGENTS.md).

Nuxt 4 + Vue 3, shipping as a web app (SSR, Cloudflare Pages preset; the
docker image uses `node-server`) and an Electron desktop app (static SPA).

## Commands

pnpm workspace with `nodeLinker: hoisted` (Electron Forge needs the
npm-style layout).

```bash
pnpm setup                # deps + nuxt prepare + playwright chromium (browser-mode tests)
pnpm start:dev:web        # nuxt dev on :3000
pnpm start:dev:desktop    # nuxt dev + electron-forge start, DESKTOP_BUILD=hybrid
pnpm build:web            # production web build
pnpm package:desktop / make:desktop   # nuxt generate + electron-forge package / make
pnpm check-lint           # dedupe --check + check-types + check-eslint + check-fmt + check-knip
pnpm lint                 # fixing variant (knip --fix removes dead exports/deps/files)
pnpm test                 # vitest run --coverage (node + browser + nuxt projects);  test-watch
pnpm qa                   # check-lint + test;  qa-fix = lint + test
pnpm check-types          # nuxt typecheck (regenerates .nuxt, then vue-tsc -b) + tsc on electron/
```

## Build modes

`DESKTOP_BUILD` drives a Vite `define` producing `__DESKTOP_BUILD__`
([nuxt.config.ts](nuxt.config.ts), [index.d.ts](index.d.ts)):

- unset/`0`: web build, literal `false`, SSR on.
- `1`: desktop build, literal `true`, SSR off, nitro `static`, renderer
  served from `oxynote://app/index.html`.
- `hybrid`: dev only; one dev server serves both the Electron renderer and
  the browser opened for OAuth, so `__DESKTOP_BUILD__` becomes a runtime
  probe of `window.__isElectron` (set by
  [electron/preload.ts](electron/preload.ts)).

Branch on platform only through `__DESKTOP_BUILD__`, never
`process.platform`, `import.meta.client` or feature detection. The literal
substitution is the security boundary: no desktop code path can carry a
session cookie.

## Auth

Better Auth with `@better-auth/electron`.

- **Web**: `app/plugins/02.auth.ts` creates a standard client with
  `credentials: "include"`.
- **Desktop**: the renderer's client is `credentials: "omit"`. The session
  lives in main ([electron/auth-client.ts](electron/auth-client.ts),
  `electron-store` encrypted via `safeStorage`); the renderer reaches auth
  only through `window.__host.auth.*` IPC
  ([electron/auth-ipc.ts](electron/auth-ipc.ts),
  [electron/preload.ts](electron/preload.ts)). Adding an auth operation:
  handler in `auth-ipc.ts`, expose in `preload.ts`, type in `index.d.ts`,
  desktop branch in
  [app/composables/useAuthSession.ts](app/composables/useAuthSession.ts).
  Main injects the cookie into renderer requests via
  `webRequest.onBeforeSendHeaders`, except `/api/auth/*`, which must stay on
  the IPC bridge.
- Desktop OAuth: `window.requestAuth({ provider })` → system browser at
  `${APP_BASE_URL}/login` → redirect to `oxynote://` → `setupMain()`
  exchanges the code → `onAuthenticated` over the bridge →
  [app/plugins/electron-auth.client.ts](app/plugins/electron-auth.client.ts)
  refetches and navigates.

## API clients

`app/plugins/03.api-fetch.ts` provides `$coreAPIClient`
(`NUXT_PUBLIC_CORE_API_BASE_HTTP_URL`) and `$authRealtimeAPIClient`
(`NUXT_PUBLIC_AUTH_REALTIME_API_BASE_HTTP_URL`). Both propagate SSR request
headers (captured eagerly at plugin setup; the H3 context is lost inside
`onRequest` on workerd) and redirect to `/login` on 401. When
`NUXT_CORE_API_INTERNAL_HTTP_URL` / `NUXT_AUTH_REALTIME_API_INTERNAL_HTTP_URL`
are set, SSR fetches use them (needed inside a container where the public
origin is unreachable).

## Data, editor, routing

- **Pinia Colada** (`useQuery`/`useMutation`) is the data primitive.
  Auto-refetch is configured but off by default; opt in per query with
  `autoRefetch: true`. API composables: [app/composables/api/](app/composables/api/);
  request/response types: [app/utils/api/](app/utils/api/); both re-exported
  through `index.ts` for auto-import.
- **Editor**: [app/components/editor/](app/components/editor/), TipTap +
  Yjs/Hocuspocus (`NUXT_PUBLIC_AUTH_REALTIME_API_BASE_WS_URL`). `blocks/`
  (custom nodes; `upload-handler.ts` routes dropped files), `comments/`,
  `diff/`, `drag-handle/`, `slash/`, `link/`, `ai/`, `hooks/`. Editor-wide
  state is [app/stores/editor.ts](app/stores/editor.ts).
- **Routing**: one dynamic page,
  `app/pages/[[organizationSlug]]/[[documentSlug]].vue`;
  [app/middleware/01.redirect.global.ts](app/middleware/01.redirect.global.ts)
  handles auth gating, onboarding and the root redirect.
  `definePageMeta({ skipAuth: true })` marks signed-out pages.

## i18n

All user-facing text lives under [i18n/locales/en/](i18n/locales/en/), never
inline. Each JSON file has a root namespace key; a new file is registered in
`nuxt.config.ts` under `i18n.locales[0].files`. `<i18n-t>` always carries
`scope="global"` (the default `parent` scope finds no ancestor and warns per
render). vee-validate uses its own messages (see [README.md](README.md)).

## UI

- shadcn-vue components in [app/components/shadcn/ui/](app/components/shadcn/ui/),
  prefix `ShadcnUi`; Tailwind v4, theme in
  [app/assets/css/main.css](app/assets/css/main.css).
- Icons via `@nuxt/icon` (CSS mode). The desktop build bundles the
  selectable title icons (`selectableIconList()` in
  [app/utils/icon.ts](app/utils/icon.ts)); the web build resolves them via
  the server. The picker uses
  [modules/icon-picker-css.ts](modules/icon-picker-css.ts), one rule per
  icon, loaded after the page's load event. Custom SVGs:
  [app/assets/custom-icons/](app/assets/custom-icons/), prefix
  `custom-icons:`.
- **Every dialog and sheet renders a `DialogDescription`** (`sr-only` when
  nothing visible plays that role; `as-child` around `<i18n-t tag="p">` when
  interpolated), or reka-ui warns per mount. Settings action components
  render theirs inside `ActionModal`, so their tests mount through
  `mountUnderDialogRoot`.

## Formatting & TS

Prettier: tabs, no semicolons, trailing commas. ESLint is type-aware
(`*-type-checked` via `eslint.config.typescript.tsconfigPath`), plus
`eslint:recommended`, typescript-eslint strict + stylistic,
`@intlify/eslint-plugin-vue-i18n` and `eslint-plugin-vuejs-accessibility`
(shadcn wrappers are exempt from the label-association and static-element
rules).

- A cold `check-eslint` takes about a minute; the cache
  (`node_modules/.cache/eslint`) is per-file while results depend on other
  files' types, so after cross-file type changes `rm -rf` it. CI runs cold.
- ESLint cannot resolve `.vue` imports, so calls through a component ref
  carry the disable `eslint's ts program resolves .vue imports as error
  typed, vue-tsc accepts this`.
- Every `eslint-disable` states a reason after `--`, for false positives
  only. Stale disables are errors; `--max-warnings 0`.
- `no-explicit-any` and `prefer-function-type` are off.

knip ([knip.ts](knip.ts)) resolves auto-imports through `.nuxt`, so unused
components, stores and utils are detected. Blind spots: `app/composables`
are entry points (their internal exports go unreported) and
`app/components/shadcn/` is ignored. `unlisted` is off by policy (hoisted
layout, transitive imports relied on). The cache is keyed by knip version:
after editing `knip.ts`, `rm -rf node_modules/.cache/knip`.

**`pnpm dedupe --check` is a lint gate**: with `nodeLinker: hoisted` Nitro
externalizes every resolved version, so two versions of `vue` in the
lockfile ship both into the server bundle and SSR crashes (`Cannot read
properties of null` from `currentRenderingInstance`). Dependency bumps cause
the split without touching `package.json`.

TypeScript (in `nuxt.config.ts`): `noUnusedLocals`, `noUnusedParameters`,
`noUncheckedIndexedAccess`, `noImplicitOverride`, `verbatimModuleSyntax`,
`noImplicitAny`, `allowUnreachableCode: false` (mirrored in
[electron/tsconfig.json](electron/tsconfig.json)). `@/*` and `~/*` both point
to `app/*`. Under `noUncheckedIndexedAccess`, a variable filled inside a
`forEach`/`descendants` callback stays narrowed to its initializer; prefer a
sentinel or a restructure over a non-null assertion.

## Testing

### Layout

- **Tests are co-located 1:1** (`string.ts` → `string.test.ts`,
  `CalendarInput.vue` → `CalendarInput.nuxt.test.ts`), never a mirror tree.
  A test spanning two files lives with the one that initiates the
  behaviour. Top-level `describe`s follow the source file's order.
- **The suffix encodes the environment**, never `.spec.ts`, never an
  `@vitest-environment` pragma:

  | suffix | environment | for |
  | --- | --- | --- |
  | `.test.ts` | node | pure logic; `electron/` with `vi.mock("electron")` |
  | `.nuxt.test.ts` | nuxt runtime | composables/stores and all component tests (`mountSuspended`, real children, mocked network) |
  | `.browser.test.ts` | headless chromium | DOM behaviour happy-dom fakes (geometry, DOMPurify) |
  | `.test-d.ts` | typecheck | compile-time contracts (`expectTypeOf`) |
  | `.bench.ts` | `vitest bench` | hot paths, only when a regression bites |

- e2e lives in the repo-root [e2e/](../e2e/) package. There is no separate
  integration tier: component tests are the integration layer.
- Nuxt's default `ignore` excludes `**/*.{spec,test}.*` from its scanners,
  so co-location inside `app/` is safe.
- **`app/components/shadcn/` is out of scope**: vendored, regenerated by the
  CLI, excluded from collection and coverage. Rendering one as a real child,
  or importing one as a harness, is fine.

### Naming

`describe` names the subject by its greppable identifier (every file has a
root `describe`; never bare `it`); `it` completes "it …" with observable
behaviour and its condition, present tense, no "should", specific enough to
diagnose from the failure line (`it("renders the fallback avatar when the
image 404s")`). One level of nesting is usual, two the ceiling; `describe("when
X")` groups tests sharing a precondition. Name components from the user's
perspective.

### Parameterized tests

`it.for` for cases differing only in inputs and outputs; cases differing in
setup, mocks or assertions are separate `it`s. Use `%s`/`%i` or `$field`
placeholders so every row has a distinct title, or a `name` field as the
whole title when placeholders cannot form a sentence. Fields: `name`,
`input`, `expected`. Hoist bulky data into builders; store mutable case
values as thunks so rows cannot share state. Never `it.each` (it provides no
test context).

### Mocks & path coverage

- **Every exported function is covered; every path gets a test** (success,
  plus one failure per collaborator). Entry-point modules
  (`electron/main.ts`, `preload.ts`) are tested through their side effects:
  mock the boundaries, import, assert the wiring.
- `// NOCOV: <lowercase reason>.` as the first line of a branch that cannot
  be reproduced (`__DESKTOP_BUILD__` splits, SSR guards, browser quirks). It
  is a reviewer covenant, not wired to the coverage provider, and never
  excuses a testable path.
- Per-test behaviour via `vi.fn()` configured inline. `vi.mock` is hoisted
  and file-level, so anything that must differ between tests is injected.
- Repeated stub shapes become local factory closures; shared dependencies
  become `test.extend` fixtures; helpers shared across a directory live in a
  colocated `test-helpers.ts` (never re-exported for app use; excluded from
  coverage in `vitest.config.ts`).
- **One act per test**; assert every facet of that one outcome.
- **Account for every injected dependency**: the calls that must have
  happened (count and arguments) and zero counts for those that must not,
  even when the return value already proves the outcome.
- **Only e2e gets a real backend.** No network, filesystem or IPC here;
  `registerEndpoint` and stubs.
- Test through public exports; never export for tests.
- Plain `expect` for preconditions; `expect.soft` only in the final
  outcome-accounting block.

### Component tests

`mountSuspended` inside the nuxt runtime, driven like a user. Helpers:
[app/components/test-helpers.ts](app/components/test-helpers.ts).

- **Never hardcode a translation.** Assert through `t()` from the helpers,
  interpolation included (`t("sidebar.search.no-results", { query: "run"
  })`), for assertions, lookup text and props alike. Pick the key the
  component actually renders (many share an English value). `t()` needs the
  nuxt context, so call it inside a test or hook, never at module scope.
- **Context providers**: `useSidebar`, `useSidebarWidth` and the tooltip
  context are page-level; mount through `mountUnderSidebarProvider` /
  `mountUnderTooltipProvider`.
- **Overlays live in `<body>`**; reach them with `menuItem()`,
  `teleportedButton()`, `openTooltipText()`. Suites driving overlays call
  `clearTeleportedOverlays()` in `beforeEach`; sheet-based suites must not
  (removing a node vue still has mounted breaks its next patch).
- [vitest.nuxt-setup.ts](vitest.nuxt-setup.ts) stubs a signed-out session
  for the whole nuxt project (the route middleware asks for it on every
  mount) and pins `@nuxt/icon` to `provider: "none"`. Signed-in suites use
  `seedAuthSession` / `seedAuthOrganization` / `seedAuthAccounts`.
- **Register an endpoint under the url the client asks for**:
  `$coreAPIClient` has an empty base in tests (bare path),
  `$authRealtimeAPIClient` an absolute one; better-auth needs both spellings
  via `mockAuthEndpoint()`.
- **A failing endpoint throws `createError({ statusCode })`**, never a bare
  `Error` (h3 dumps non-H3Errors to stderr).
- happy-dom has no `matchMedia`; pick a side with `stubViewportMatches()`.
- Pointer/geometry behaviour: `mockNuxtImport` the vueuse primitives
  (`useDraggable`, `useMouseInElement`, `useElementBounding`) and drive the
  captured callbacks, returning **refs**, not plain objects (see
  `useSidebarDraggable.nuxt.test.ts`).
- **Settling**: `settleMutations()` for pinia-colada invalidation,
  `settleActionSubmit()` for settings action modals (mounted with
  `mountWithFrozenClock()` for their `delay(300)`). better-auth's backoff
  and vee-validate's scheduler are the documented `vi.waitFor` exceptions.
- **Shared app state forces `{ concurrent: false }`**: `vi.mock`
  singletons, `useState`, pinia stores, `usePersistentState`,
  `vi.stubGlobal`, fake timers, the teleport target. Mark each describe
  that touches them, nested ones included.
- A component that keeps watching after its test (colour-mode watcher,
  branch sync, teleported overlay) is taken down with
  `enableAutoUnmount(afterEach)`.

### Editor component tests

- Node views mount through `mountNodeView()`
  ([test-helpers/node-view.ts](app/components/editor/test-helpers/node-view.ts)),
  with `makeNode()`/`makeEditor()`; the command chain is recorded, so assert
  which commands ran.
- Anything painting theme colours (charts, metric blocks, carets) needs
  `stubThemeColorContext()`
  ([test-helpers/theme.ts](app/components/editor/test-helpers/theme.ts))
  first; happy-dom has no 2d context.
- Charts are asserted via the echarts option (`chartOption()`) with
  `vue-echarts` stubbed. Virtualized lists (`vue-virtual-scroller`) are
  mocked with a pass-through component.
- `editor.commands` is rebuilt on every access; shadow the getter with a
  recording proxy to spy.
- A live editor with real extensions when the behaviour reads the document;
  collaboration components also need a provider stand-in (`document`,
  `awareness`, `on`/`off`).

### Snapshots, independence, determinism

- Snapshots only for golden-style serialized output (ProseMirror JSON, diff
  structures); explicit assertions everywhere else.
- Every test builds its own state; repeated setup goes in `beforeEach` or a
  `test.extend` fixture, never module-level mutable state.
- All tests run concurrently (`sequence.concurrent`), so assertions and
  snapshots use the context-local `expect` (`it("…", ({ expect }) => …)`). A
  test needing sequential execution is a smell; the one exception is call
  accounting on `vi.mock` singletons under `{ concurrent: false }` with a
  comment.
- `restoreMocks`, `unstubGlobals`, `unstubEnvs` are set globally. Since
  vitest 4 `restoreMocks` does not touch hand-made `vi.fn()` singletons;
  reset those in `beforeEach`.
- Never sleep: await `nextTick()`, `flushPromises()`, an event or the unit's
  promise. Drive time with fake timers and `vi.setSystemTime()`; assert
  exact values. `vi.waitFor` is the last resort and always carries a
  justifying comment.

## Code style

- **One assignment per statement** (`no-multi-assign`): no chaining, no
  assignment inside an expression such as `const group = (groups[key] ??=
  …)`. `??=` as its own statement is fine.
- `<script setup>` order: imports; `defineProps`/`defineEmits`/`defineExpose`;
  store and composable initializations; `ref`/`computed`; lifecycle hooks;
  watchers; functions.
- Tailwind utilities by default; custom rules go in `main.css` and are
  applied by class. No `<style>` blocks or static inline `style=`.

## PromQL grammar

[@oxynote/lezer-promql](https://github.com/oxynote/lezer-promql) forks
Prometheus's grammar to add Grafana-style placeholders (`$__interval`). It is
a published package reaching the app through a pnpm override in
[pnpm-workspace.yaml](pnpm-workspace.yaml) (so `@prometheus-io/codemirror-promql`
resolves it too). A grammar change is a release from its own repo, then a
bump in the override and in `minimumReleaseAgeExclude`.

## Electron packaging

`forge.config.ts` allowlists only `/.vite` and `/.output/public`. The
`oxynote://` protocol is registered at install for OAuth deep links. Fuses
disable `RunAsNode` and `EnableNodeOptionsEnvironmentVariable` and require
ASAR integrity. Main and preload are forced to `.cjs`
([vite.electron.config.ts](vite.electron.config.ts)) because `package.json`
is `"type": "module"`. `__API_BASE_URL__` and `__APP_BASE_URL__` are baked
at build time; the build fails if they are missing.
