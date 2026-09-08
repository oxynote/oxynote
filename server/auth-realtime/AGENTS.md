# AGENTS.md

Guidance for `server/auth-realtime`. Shared principles and TS/JS style live
in the root [AGENTS.md](../../AGENTS.md); backend architecture and the
document-storage model in [server/AGENTS.md](../AGENTS.md).

`@oxynote/auth-realtime`: a TypeScript ES-module service running Better Auth
and a Hocuspocus (Yjs) server in one Hono process on `:8081`. It owns
sessions, the realtime document connection and the service-to-service
endpoints core calls; everything about documents themselves belongs to core.

## Commands

pnpm, own workspace.

```bash
pnpm dev            # tsx watch, sentry preloaded via --import
pnpm build          # tsc -> dist/
pnpm build:bundle   # esbuild single file -> dist/bundle.mjs (what the images run)
pnpm check-lint     # check-types + eslint + prettier + knip, read-only
pnpm lint           # the fixing variant (knip --fix removes dead exports and files)
pnpm test           # vitest run --coverage;  test-watch
pnpm qa             # check-lint + test;  qa-fix = lint + test
```

## Composition root

**`src/index.ts` is the only module with side effects**: it reads the env,
opens the pool, connects to Valkey (only when
`OXYNOTE_AUTH_REALTIME_VALKEY_DSN` is set; otherwise better-auth runs with
no secondary storage) and listens. Everything else is a factory taking what
it needs. Modules: `env.ts` (zod config), `core.ts` (every call into core),
`db.ts` (the `Store`), `reporting.ts`, `logging.ts`, `headers.ts`, `auth.ts`
(`createAuth` + better-auth callbacks), `hocuspocus.ts` (`createHocuspocus`,
`createDocumentHooks`, `flushDocument`), `routes.ts` (`createRoutes` → Hono
app), `operations.ts` (pure: assistant edit ops on a Y.Doc), `ydocument.ts`
(pure: replacing Y.Doc content), `schema/` (ProseMirror schema mirroring
web's tiptap extensions), `sentry.ts` and `bundle.ts` (docker entry: sentry
+ index in one bundle).

- A module never reads `process.env`; it takes an `Env`. `loadEnv` runs once
  in `index.ts`. `src/sentry.ts` is the exception: node loads it via
  `--import` before the app graph exists, so it reads `process.env` and
  imports nothing of ours.
- A factory takes its dependencies as one argument, narrowed to what it
  uses (`createDocumentHooks` takes a one-method `SessionResolver`, not the
  better-auth instance), so a test can pass a two-line stub.
- A callback a framework invokes without handing it back is a named
  function (`createOrganizationHooks`); callbacks reachable through
  `auth.options` may stay inline.
- Module-level mutable state lives in the factory's closure, never at module
  scope.
- No query builder leaves `db.ts`. Every query is a `Store` method named for
  what it answers (`hasOAuthConsent`), so a schema rename touches one file
  and a test stubs methods rather than builder chains.
- Failures reach sentry through `reporting.ts`, not an inline `catch`:
  `reported(fn)` reports and rethrows (the frameworks swallow the cause
  otherwise), `bestEffort(fn)` reports and swallows, for effects that must
  not fail already-succeeded work. A `catch` that builds a response stays
  written out.
- Every line goes through `logging.ts` (pino at
  `OXYNOTE_AUTH_REALTIME_LOG_LEVEL`, records matching core's slog output),
  better-auth's and hocuspocus's own logging included. `createLogger` takes
  its destination; `Logger` is a four-method interface so tests hand over
  `vi.fn()`s. Nothing logs an address or a port.

## Environment

Every variable is declared in `src/env.ts` and zod-parsed once at boot. A
missing, malformed or half-configured value is a boot error: URLs are
scheme-pinned to http/https (bare `z.url()` accepts `core:8080`), counters
are positive integers, booleans accept only `"true"`/`"false"`, a social
provider needs both credential halves or neither. Docker env files deliver
unset variables as `""`; `loadEnv` drops those before parsing so a required
one reports as missing.

Adding a variable: the schema, the `Env` interface,
`docker/env/auth-realtime.example.env`, and for the production image the
launcher's `mapping.ts`.

## Formatting & TS

Prettier: tabs at width 8, no semicolons, trailing commas, double quotes.
ESLint is type-aware (`strictTypeChecked` + `stylisticTypeChecked` via
`projectService`):

- Cached in `node_modules/.cache/eslint`; after cross-file type changes
  results can be stale locally (`rm -rf` the cache). CI runs cold.
- Root config files sit outside the tsconfig project and opt out of typed
  linting; the type-aware rule overrides live in the `src/**` block because
  applying them to an untyped file crashes the run.
- Every `eslint-disable` states a reason after `--`, for false positives
  only. Stale disables are errors; `--max-warnings 0`.
- `no-explicit-any` is off; the `no-unsafe-*` family is on. Type the
  boundary rather than relax the rule: untrusted JSON is read as `unknown`
  and narrowed, tiptap's `any`-typed API gets an explicit cast per use.

TypeScript: `module: NodeNext`, `target: ESNext`, strict, `noUnusedLocals`,
`noUnusedParameters`, `noUncheckedIndexedAccess`, `noImplicitOverride`,
`verbatimModuleSyntax`, `allowUnreachableCode: false`. `tsconfig.json`
covers all of `src/` (tests included); `tsconfig.build.json` excludes tests.
**Imports carry the explicit `.js` extension** even for `.ts` sources.

knip (`knip.ts`) guards dead exports, files and dependencies.
`ignoreExportsUsedInFile` is on for types only; an exported const nobody
imports should be reported. Its cache is keyed by knip version, not config:
after editing `knip.ts`, `rm -rf node_modules/.cache/knip`.

## Testing

Layout:

- Tests are co-located 1:1 (`ydocument.ts` → `ydocument.test.ts`), all
  `.test.ts` in vitest's node environment. A test spanning two files lives
  with the one that initiates the behaviour. Top-level `describe`s follow
  the source file's order.
- Shared helpers live in `src/test-helpers.ts` (`stubCore`, `stubStore`,
  `testEnv`, `fragmentXml`), excluded from coverage and build. One-suite
  fixtures stay file-local.
- `db.test.ts` is the only suite that knows kysely chains; it stubs the
  builder to pin table and column names. Every other suite takes
  `stubStore()`.
- **A yjs type must be attached to a document before a test reads it**: a
  detached `Y.XmlElement`/`Y.XmlText` keeps its children in prelim state,
  invisible to the getters, and yjs only warns. Build fixtures through an
  `attached()` helper; `src/test-setup.ts` turns that warning into a
  failure.
- `src/schema/` is tested as one unit through `src/schema/index.test.ts`:
  round trips (prosemirror JSON → Y.Doc → JSON) prove each node is
  registered, the `UniqueID` uid list is checked against every addressable
  block, and the `data-type` strings are pinned because they exist only to
  stay identical to web's.

Naming: `describe` names the subject by its greppable identifier (every file
has a root `describe`, never bare `it`); `it` completes "it …" with
observable behaviour, present tense, no "should", specific enough to
diagnose from the failure line (`it("throws when no branch is flagged as
default")`). One level of nesting is usual, two the ceiling.

Parameterized: `it.for` (never `it.each`, which drops the test context) for
cases differing only in inputs and outputs; cases differing in setup or
assertions are separate `it`s. Case fields are `name`, `input`, `expected`;
bulky data goes in named builders above the table.

Mocks & coverage:

- Every exported function and every path (success, plus one failure per
  collaborator) is covered.
- Dependencies are injected, never module-mocked. `reporting.test.ts` is the
  one exception (sentry is global) and pays with `{ concurrent: false }` and
  a `beforeEach` reset; everywhere else `reported`/`bestEffort` are asserted
  by what they do to the caller.
- No real IO. `createAuth` is exercised against a lazily-constructed pool
  that is never dialled.
- One act per test; assert as many facets of that one outcome as needed.
- Account for every injected dependency: the calls that must have happened
  (count and arguments) and the zero counts of those that must not.
- Test through public exports; never export for tests. An internal that
  needs direct tests becomes its own module.
- Plain `expect` for preconditions; `expect.soft` only in a final
  outcome-accounting block.
- A branch that cannot be reproduced carries `// NOCOV: <lowercase reason>.`
  as its first line. It is a reviewer covenant, not wired to the coverage
  provider, and never excuses a testable path.

Independence: every test builds its own state; all tests run concurrently
(`sequence.concurrent`), so every assertion uses the context-local `expect`
(`it("…", ({ expect }) => …)`), `it.for` rows included. `restoreMocks`,
`unstubGlobals` and `unstubEnvs` are set globally.

Determinism: no `setTimeout` waiting; await the unit's promise or event.
Drive time with `vi.useFakeTimers()`/`advanceTimersByTime()`/
`vi.setSystemTime()` and assert exact values.

Coverage thresholds in `vitest.config.ts` are set from the measured baseline
and only ever raised. `src/index.ts` is excluded.

## Yjs

Never seed or merge one Y.Doc from another with `Y.applyUpdate`: differing
`clientID`s CRDT-merge and duplicate every block. Use `replaceYdocContent`
(`src/ydocument.ts`) and persist `rawContent` immediately after, so a restart
cannot rebuild the document under a new clientID. `ydocument.test.ts`,
`hocuspocus.test.ts` and `routes.test.ts` are the specification.

## Better Auth schema

Core's migrations own every table, Better Auth's included.
`sql/better_auth_schema.sql` is reference output only, never applied;
regenerate it to diff what Better Auth expects after changing `src/auth.ts`
(see the "Database" section of [server/AGENTS.md](../AGENTS.md)).
