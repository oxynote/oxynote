# CLAUDE.md

Guidance for the `web/` frontend. Shared working principles and the TS/JS
comment/whitespace rules live in the root [CLAUDE.md](../CLAUDE.md).

## Project

**Oxynote** web frontend — Nuxt 4 + Vue 3 frontend that ships as both a web app (Cloudflare Pages, SSR) and an Electron desktop app (static SPA).

## Common commands

Package manager is **pnpm** (workspace; uses `nodeLinker: hoisted` so Electron Forge's npm-style layout works).

```bash
pnpm install                  # installs deps; runs `prepare` (builds lezer-promql + nuxt prepare)
pnpm setup                    # deps + explicit prepare (guarantees fresh lezer build + .nuxt types)

pnpm start:dev:web            # nuxt dev on :3000 (web build)
pnpm start:dev:desktop        # concurrently runs nuxt dev + electron-forge start with DESKTOP_BUILD=hybrid
pnpm build:web                # production web build (cloudflare_pages preset)
pnpm package:desktop          # nuxt generate (static) + electron-forge package
pnpm make:desktop             # nuxt generate (static) + electron-forge make (installers)

pnpm check-lint               # check-types + check-eslint + check-fmt +
                              # check-knip (read-only)
pnpm lint                     # fixing variant: check-types + knip --fix
                              # (removes dead exports/deps/files!) + eslint
                              # --fix + prettier --write
pnpm qa                       # check-lint (will grow tests later); qa-fix = lint
pnpm check-types              # nuxt typecheck (regenerates .nuxt types, then
                              # vue-tsc -b --noEmit; plain vue-tsc --noEmit on
                              # the solution-style root tsconfig checks nothing)
                              # + tsc on electron/ (own tsconfig, not part of
                              # the nuxt solution)
pnpm check-eslint             # eslint --max-warnings 0 . (eslint / fmt are the
                              # fixing counterparts)
pnpm check-knip               # dead exports/files/dependencies ([knip.ts](knip.ts))

pnpm build:lezer-promql       # rebuild the forked PromQL grammar package
```

There is no test runner configured in web/.

## Build modes (critical)

The `DESKTOP_BUILD` env var drives a Vite `define` that materializes `__DESKTOP_BUILD__` in the bundle. See [nuxt.config.ts:9-24](nuxt.config.ts#L9-L24) and [index.d.ts:1-9](index.d.ts#L1-L9):

- `DESKTOP_BUILD=0` / unset → web build. `__DESKTOP_BUILD__` is literal `false`. SSR enabled. Nitro preset `cloudflare_pages`, overridable via `NITRO_PRESET` (the docker image builds with `node-server`).
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

- `$coreAPIClient` → core API (`NUXT_PUBLIC_CORE_API_BASE_HTTP_URL`)
- `$authRealtimeAPIClient` → auth-realtime API (`NUXT_PUBLIC_AUTH_REALTIME_API_BASE_HTTP_URL`)

Both propagate SSR request headers (captured eagerly during plugin setup — the H3 context is lost inside `onRequest` callbacks on Cloudflare workerd) and redirect to `/login` on 401.

When the server-only runtime config keys `coreAPIInternalHttpURL` / `authRealtimeAPIInternalHttpURL` (`NUXT_CORE_API_INTERNAL_HTTP_URL` / `NUXT_AUTH_REALTIME_API_INTERNAL_HTTP_URL`) are set, SSR fetches (including the auth client in `app/plugins/02.auth.ts`) use them instead of the public URLs — required when the app runs inside a container where the public localhost origin is unreachable.

## Data layer

**Pinia Colada** (`useQuery` / `useMutation`) is the standard data-fetching primitive. Auto-refetch plugin is configured but *disabled by default* — opt in per-query with `autoRefetch: true` (typed augmentation in [index.d.ts:34-38](index.d.ts#L34-L38)). See [colada.options.ts](colada.options.ts).

API composables live in [app/composables/api/](app/composables/api/) and are re-exported by [app/composables/index.ts](app/composables/index.ts) for auto-import. Request/response types live in [app/utils/api/](app/utils/api/) and are re-exported by [app/utils/index.ts](app/utils/index.ts).

## Editor (TipTap + Yjs)

Document editor is in [app/components/editor/](app/components/editor/). Real-time collaboration uses **Yjs** + **Hocuspocus** (`NUXT_PUBLIC_AUTH_REALTIME_API_BASE_WS_URL`). Notable subsystems:

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

Prettier uses **tabs**, no semicolons, trailing commas — see [prettier.config.js](prettier.config.js).

ESLint ([eslint.config.mjs](eslint.config.mjs)) runs **type-aware**: `eslint.config.typescript.tsconfigPath` in `nuxt.config.ts` switches the generated Nuxt preset to the `*-type-checked` rule sets, on top of which the config adds `eslint:recommended` (the Nuxt preset ships no base JS rules), typescript-eslint **strict + stylistic**, and `@intlify/eslint-plugin-vue-i18n`. Consequences worth knowing:

- Linting builds a full TS program, so a cold `check-eslint` takes about a
  minute. Runs are cached (`node_modules/.cache/eslint`) — unchanged files
  are near-instant. Caveat: the cache is per-file while type-aware results
  depend on *other* files' types, so after cross-file type changes a cached
  file can report stale results locally. CI always runs cold and is the
  gate of record; locally, `rm -rf node_modules/.cache/eslint` forces a
  full run.
- ESLint's TS program cannot resolve `.vue` imports (only `vue-tsc` can), so calls through a component ref are reported as unsafe. Those carry an inline disable with the reason `eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this`.
- Every `eslint-disable` **must** state a reason after `--`. It is for false positives only, never to avoid a fix. Stale disables are errors (`reportUnusedDisableDirectives`), and eslint runs with `--max-warnings 0`, so warnings fail the gate too.
- `@typescript-eslint/no-explicit-any` is off; `prefer-function-type` is off (keeps the `defineEmits<{ (e: ...): void }>()` style).
- The vendored `packages/lezer-promql` fork is excluded so upstream re-syncs cannot break the gate.

**knip** ([knip.ts](knip.ts)) guards dead exports, unused files, and unused dependencies. Its nuxt plugin resolves auto-imports through the generated `.nuxt` maps, so unused components, stores, and utils **are** detected despite having no import statements. Remaining blind spots, documented in the config: `app/composables` are entry points (the auto-import map does not connect their `export default function useX` style, so their internal exports are unreported), and `app/components/shadcn/` is ignored (vendored spares are expected). Knip's `unlisted` check is **off** by policy: the hoisted node_modules layout makes transitive packages importable, that is relied on deliberately, and `package.json` declares top-level intent only. Knip runs with `--cache`, and the cache is keyed by knip version, **not** config — after editing [knip.ts](knip.ts), run `rm -rf node_modules/.cache/knip` or results are stale.

TypeScript strictness (set in [nuxt.config.ts](nuxt.config.ts)): `noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`, `noImplicitOverride`, `verbatimModuleSyntax`, and `noImplicitAny` all on; `allowUnreachableCode: false` (mirrored in [electron/tsconfig.json](electron/tsconfig.json)). Path aliases `@/*` and `~/*` both point to `app/*`.

Note `noUncheckedIndexedAccess` makes every index access `T | undefined`, and TypeScript does not track assignments made inside a callback — so a variable filled in by a `forEach`/`descendants` callback stays narrowed to its initializer afterwards. Prefer a sentinel value or a restructure over a non-null assertion when that happens.

## Testing

### Layout

- **All tests except e2e are co-located**: the test file lives in the same
  directory as the file it tests, paired 1:1 — `string.ts` →
  `string.test.ts`, `CalendarInput.vue` → `CalendarInput.nuxt.test.ts`.
  Never a parallel `tests/unit/...` mirror tree.
- When a test exercises behaviour spanning two source files, it lives
  with the file that initiates the behaviour (a round-trip test lands
  beside the file that starts the round-trip).
- **The suffix encodes the test environment** — never `.spec.ts`, never an
  `@vitest-environment` pragma comment:

  | suffix | environment | for |
  | --- | --- | --- |
  | `.test.ts` | node, no DOM | pure logic: utils, editor diff/blocks helpers, `electron/` (with `vi.mock("electron")`) |
  | `.nuxt.test.ts` | nuxt runtime | composables/stores (auto-imports, `registerEndpoint`) and **all component tests** (`mountSuspended`, real children, mocked network) |
  | `.browser.test.ts` | vitest browser mode | DOM behavior happy-dom fakes (e.g. `getBoundingClientRect` geometry in drag-handle) |
  | `.test-d.ts` | typecheck only | compile-time contracts (`expectTypeOf`), e.g. `app/utils/api/` types |
  | `.bench.ts` | `vitest bench` | perf on hot paths (diff/lcs) — add only when a regression bites |

- Only **e2e** (Playwright, incl. visual regression via `toHaveScreenshot`)
  lives outside the source tree, in `tests/`.
- There is no separate "integration" tier: component tests are the
  integration layer (real children, mocked IO); e2e covers cross-system.
- Co-location inside `app/` is safe by design: Nuxt's default `ignore`
  list excludes `**/*.{spec,test}.*` from all scanners, so a test file
  next to a plugin or middleware is never registered as one. The
  `.nuxt.test.ts` suffix is the officially documented environment opt-in;
  the vitest config still declares the env-by-suffix globs explicitly so
  the mapping is configuration, not tool default.

### Naming

**The rule: `describe` names the subject, `it` completes the sentence
"it …" with observable behaviour.**

```ts
describe("useCart", () => {
	describe("addItem", () => {
		it("increments the total by the item price")
		it("merges duplicate items into one line with summed quantity")
		it("throws when the item is out of stock")
	})
})
```

Read it aloud: "useCart addItem increments the total by the item price."
If the concatenation isn't a grammatical sentence, the name is off. This
matters practically because reporters print exactly that concatenation on
failure, and a good name lets you diagnose without opening the file.

**`describe`**

- Top level: **always required** — every test file wraps its tests in a
  root `describe` identifying the testable unit by its real identifier —
  `useCart`, `<PriceTag>`, `formatDate`. Never bare top-level `it` calls.
  Don't paraphrase ("cart logic"), use the greppable name.
- Nested level (optional): a method, prop, or condition — `addItem`,
  `when the user is anonymous`. One level of nesting is usually right,
  two is the ceiling. Deeply nested describes with `beforeEach` at each
  level are a classic readability trap: state gets assembled across four
  scopes and no single test is comprehensible alone.
- `describe("when X", ...)` grouping is good when several tests share a
  precondition; otherwise put the condition in the `it` name.

**`it`**

- Present tense, third person, no "should".
  `it("returns null for empty input")`, not `it("should return null…")` —
  "should" is eight wasted characters on every line and adds nothing.
- State behaviour + condition, not implementation:
  `it("retries the request twice before failing")`, not
  `it("calls fetchWithRetry with maxRetries=2")`. The test name should
  survive a refactor that preserves behaviour.
- Be specific enough that a failure is informative. `it("works")`,
  `it("handles errors")`, `it("renders correctly")` are useless — which
  error, what is correct? `it("renders the fallback avatar when the image
  404s")` tells you what broke.
- Edge cases worth naming explicitly: empty input, boundary values, error
  paths. A suite skeleton often reads: happy path first, then
  `it("returns [] when the list is empty")`,
  `it("throws TypeError for non-ISO strings")`, etc.

For components, name from the user's perspective where possible:
`it("shows the discount badge when price is reduced")` rather than
`it("sets showBadge to true")`.

Parameterized tests: use the format placeholders so each case gets a
distinct name — `it.for(cases)("parses %s as %i", ...)` — otherwise
failures all report the same string.

### Parameterized tests

**Use `it.for` wherever cases differ only in inputs and expected
outputs — pure data tables.** The moment cases differ in behaviour —
different mock wiring, different setup, different assertions — they are
separate `it`s inside the method's `describe` (see the next section),
never rows sharing one callback: divergent cases in a shared callback
breed conditionals, and neither a breakpoint nor a reporter line can
target a single row.

For data tables, writing a separate test per case gets repetitive;
`it.for` defines the cases as data and runs the same test logic for all
of them:

```ts
it.for([
	[1, 1, 2],
	[1, 2, 3],
	[2, 1, 3],
])("add(%i, %i) -> %i", ([a, b, expected], { expect }) => {
	expect(a + b).toBe(expected)
})
```

The placeholders `%i`, `%s`, and `%f` in the test name are replaced with
the corresponding values from each row, so the output shows
`add(1, 1) -> 2`, `add(1, 2) -> 3`, and so on.

When cases have more than two or three values, objects are more readable.
Use `$property` in the name to interpolate fields:

```ts
it.for([
	{ input: "", expected: [] },
	{ input: "a", expected: ["a"] },
	{ input: "a,b", expected: ["a", "b"] },
])("splits $input into $expected", ({ input, expected }, { expect }) => {
	expect(split(input)).toEqual(expected)
})
```

- When placeholder interpolation cannot form a grammatical sentence, give
  the case an explicit `name` field and use it as the whole title:
  `it.for(cases)("$name", …)`.
- Case objects use the same field vocabulary everywhere: `name`, `input`,
  `expected`.
- Hoist bulky case data into named builders above the table instead of
  inlining large literals. Store mutable case values as thunks
  (`makeDoc: () => …`) invoked inside the test body, so a case object
  can never leak state between tests.
- Always `it.for`, never `it.each`: `.for` passes the test context as
  the second callback argument — which the concurrency rule below
  requires for `expect` — while `.each` spreads the case and provides no
  context.

### Mocks & path coverage

- **Every path gets a test** — the success path and each failure path,
  including one per collaborator that can fail, especially in unit tests.
  A `describe` per method groups them; the linear `it`s are the rows,
  named after the observable behaviour
  (`it("propagates the error from db.createItem")`).
- A branch that genuinely cannot be reproduced in tests carries a
  `// NOCOV: <lowercase reason>.` comment as the first line inside the
  branch body. Environment-bound code is the typical legitimate case:
  `__DESKTOP_BUILD__` splits, SSR guards, browser-quirk workarounds.
- NOCOV is a reviewer covenant, not a tool directive: it is deliberately
  not wired to the coverage provider (whose ignore hints require the
  banned `/* */` comment form), so coverage reports stay truthful and
  the marker explains the residual gap in place. It never excuses a
  testable path.
- Per-test mock behaviour is configured inline with `vi.fn()`
  (`mockResolvedValue`, `mockRejectedValue`, `mockImplementation`).
  `vi.mock` module mocks are hoisted and file-level — they cannot vary
  per test, so anything that must differ between tests is injected, not
  module-mocked.
- Repeated stub shapes become local factory closures
  (`const stubDb = (err?: Error) => …`) defined next to the tests that
  use them; dependencies shared across many tests become `test.extend`
  fixtures.
- **One act per test**: arrange once, invoke the unit once, then assert
  as many facets of that single outcome as needed — result, error, and
  interactions all belong in the same test when they describe one
  scenario. Needing a second invocation of the unit is the real "and"
  smell — split the test, not the assertions.
- **Every injected dependency is accounted for in every test**: assert
  the calls that must have happened (count and arguments) and the zero
  counts of the ones that must not —
  `expect(db.createItem).toHaveBeenCalledTimes(0)` after a failed
  precondition is as load-bearing as any positive assertion. This holds
  even when the return value already proves the outcome: a call is a
  potential side effect, and output assertions cannot reveal a stray one.
- Interaction assertions at injected boundaries are behaviour, not
  implementation: they pin the unit's contract — which effects occur,
  with what, and when — and survive any behaviour-preserving refactor of
  the unit's internals.
- **Only e2e is allowed a real backend**: unit and component tests never
  perform real IO — network, filesystem, IPC — every such boundary is
  mocked (`registerEndpoint`, injected stubs).
- Test through the module's public exports. Never export something
  solely for tests — an internal complex enough to need direct tests is
  extracted into its own module.
- Plain `expect` for guards and preconditions; `expect.soft` is
  permitted in the final outcome-accounting block, where it reports the
  whole broken accounting at once instead of stopping at the first
  failed count.

### Snapshots

Snapshots are golden files, not lazy assertion dumps: use them only for
golden-style serialized outputs (ProseMirror JSON, diff structures) —
`toMatchInlineSnapshot` for small values, file snapshots for large ones,
explicit assertions everywhere else. Under concurrency they require the
context-local `expect` (see below).

### Independence & concurrency

- **Every test is completely independent**: it creates its own fresh
  state (a new instance, store, or mount per test) and never depends on
  execution order or on state left behind by another test. Each
  `describe` block focuses on one method; each test verifies one specific
  behaviour — the suite reads like a specification of the unit, and a
  failure's name plus assertion tell you exactly what broke without
  opening the file.
- Repeating the same setup in every test is a candidate for `beforeEach`
  or a `test.extend` fixture — never for module-level shared mutable
  state.
- **All tests run concurrently**: the vitest config sets
  `sequence.concurrent: true`. Test independence is what makes this safe;
  a test that needs sequential execution is a smell, not a config
  exception.
- Under concurrency, snapshots and assertions must use the `expect` from
  the local test context (`it("…", ({ expect }) => …)`) — the global
  `expect` cannot reliably attribute them to the right test when tests
  interleave. See
  [test.concurrent](https://vitest.dev/api/test#test-concurrent).
- Mock and global state cannot leak between tests by construction: the
  vitest config sets `restoreMocks: true`, `unstubGlobals: true`, and
  `unstubEnvs: true`, so spies, stubbed globals, and env stubs are
  restored after every test without per-file `afterEach` cleanup.

### Determinism

- Never sleepy: no `setTimeout`-based waiting in tests. Await concrete
  signals — `nextTick()`, `flushPromises()`, an emitted event, a promise
  returned by the unit itself.
- Time is driven, not waited on: `vi.useFakeTimers()` +
  `advanceTimersByTime()` for timer-dependent code, `vi.setSystemTime()`
  for deterministic timestamps — assert exact dates, never wall-clock
  deltas.
- `vi.waitFor` is the last resort for genuinely nondeterministic
  scheduling and always carries a comment justifying why no concrete
  signal exists.

### Prior art

Co-location is established practice across the ecosystem, including
dependencies of this app:
[Reka UI](https://github.com/unovue/reka-ui/tree/v2/packages/core/src/Dialog)
(`Dialog.test.ts` among the Dialog `.vue` components),
[Pinia Colada](https://github.com/posva/pinia-colada/tree/main/src)
(co-located specs plus `.test-d.ts` type tests),
[VueUse](https://github.com/vueuse/vueuse/tree/main/packages/core/useMouse),
[PrimeVue](https://github.com/primefaces/primevue/tree/master/packages/primevue/src/button)
(`Button.spec.js` beside `Button.vue`),
[SvelteKit](https://github.com/sveltejs/kit/tree/main/packages/kit/src/utils),
[immich](https://github.com/immich-app/immich) (co-located specs + a
dedicated `e2e/` package), and — on nuxt 4's `app/` layout specifically —
[kun-galgame-forum](https://github.com/KunMoe/kun-galgame-forum/tree/master/apps/web/app/components/editkit)
(specs beside components inside `app/`).

## Code style

### Assignments

**One assignment per statement.** Never chain them, and never assign from inside
an expression — enforced by `no-multi-assign`.

```ts
// no: the line both mutates the map and binds a local
const group = (groups[key] ??= { items: [] })

// yes
let group = groups[key]
if (!group) {
	group = { items: [] }
	groups[key] = group
}
```

`??=` itself is fine and used across the codebase; keep it as its own
statement (`elem.children ??= []`).

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
