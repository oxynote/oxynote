# CLAUDE.md

Guidance for `server/auth-realtime`. Shared working principles and the TS/JS
comment/whitespace rules live in the root [CLAUDE.md](../../CLAUDE.md);
backend architecture, the Caddy/port layout, and the document-storage model
live in [server/CLAUDE.md](../CLAUDE.md).

## Project

`@oxynote/auth-realtime` — a TypeScript ES-module service running Better Auth
and a Hocuspocus (Yjs) server in a single Hono process on port `8081`. It is a
thin layer: it owns sessions, the realtime document connection, and the
service-to-service endpoints core calls back into. Everything about documents
themselves belongs to core.

## Common commands

Package manager is **pnpm** (own workspace).

```bash
pnpm install                  # install dependencies
pnpm dev                      # tsx watch, sentry preloaded via --import
pnpm build                    # tsc -p tsconfig.build.json -> dist/
pnpm build:bundle             # esbuild single-file bundle -> dist/bundle.mjs;
                              # what both docker images run (sentry folded in,
                              # optional native accelerators left external)
pnpm start                    # run the built service

pnpm check-lint               # check-types + check-eslint + check-fmt +
                              # check-knip (read-only)
pnpm lint                     # fixing variant: check-types + knip --fix
                              # (removes dead exports/files!) + eslint --fix +
                              # prettier --write
pnpm test                     # vitest run --coverage
pnpm test-watch               # vitest in watch mode
pnpm qa                       # check-lint + test; qa-fix = lint + test
```

## Composition root

**`src/index.ts` is the only module with side effects.** It is the one place
that reads the environment, opens a database pool, connects to Valkey (only
when `OXYNOTE_AUTH_REALTIME_VALKEY_DSN` is set — without it better-auth is
built with no secondary storage at all), or listens on a port. Everything
else is a factory taking what it needs:

```
src/
  index.ts        composition root: env -> wiring -> listen
  sentry.ts       the sentry bootstrap, loaded before the app graph exists
  bundle.ts       the docker entry: sentry + index folded into one esbuild
                  artifact (build:bundle), so no --import preload and no
                  node_modules ship in the images
  env.ts          zod-parsed, typed configuration
  core.ts         the client for every call this service makes into core
  db.ts           the Store: every query this service runs, behind one
                  interface
  reporting.ts    reported() / bestEffort(), the two ways a failure reaches
                  sentry
  logging.ts      createLogger(level, destination): the pino logger
                  everything the service writes goes through
  headers.ts      normalizing whatever shape a transport hands over
  auth.ts         createAuth(deps) + the callbacks better-auth invokes
  hocuspocus.ts   createHocuspocus(deps) / createDocumentHooks(deps); the
                  flushDocument the branch routes run before core reads a
                  branch's stored row
  routes.ts       createRoutes(deps) -> a Hono app
  operations.ts   pure: applying the assistant's edit ops to a Y.Doc
  ydocument.ts    pure: replacing a Y.Doc's content without CRDT merging
  schema/         the ProseMirror schema, mirroring web's tiptap extensions
```

Rules that follow, and that a change must not erode:

- **A module never reaches for `process.env`.** It takes an `Env`. `loadEnv`
  is called once, in `index.ts`.
- **A factory takes its dependencies as an argument**, narrowed to what it
  actually uses. `createDocumentHooks` asks for a `SessionResolver` with one
  method rather than the whole better-auth instance, and `createRoutes` asks
  for a `DocumentRegistry` rather than the whole Hocuspocus server. That
  narrowing is what lets a test drive them with a two-line stub.
- **A callback a framework will invoke is a named function, not an inline
  arrow**, whenever the framework does not hand it back. The organization
  plugin keeps its hooks to itself, so they live in
  `createOrganizationHooks`; better-auth exposes `auth.options`, so the
  email callbacks stay inline and the tests reach them there.
- Module-level mutable state belongs in the factory's closure (the maintainer
  map in `createDocumentHooks`, the cached JWKS in `createRoutes`), never at
  module scope, so nothing carries over between servers — or between tests.
- **No query builder leaves [db.ts](src/db.ts).** Every query the service
  runs is a method on `Store`, named for what it answers rather than the SQL
  it builds (`hasOAuthConsent`, `userOrganizationId`). A caller takes the
  `Store`, so a table or column rename touches one file and a test stubs four
  methods instead of a chain of builder calls.
- **A failure reaches sentry through
  [reporting.ts](src/reporting.ts), not an inline `catch`.** `reported(fn)`
  reports and rethrows, for work whose failure the caller must still see —
  the frameworks this service hands callbacks to swallow the cause, so
  without it a failed hook leaves only a status code. `bestEffort(fn)` reports
  and swallows, for an effect whose failure must not fail work that has
  already succeeded (persisting rawContent after a document is already
  seeded, closing a direct connection). A `catch` that also builds a response
  stays written out, because neither helper can express that.

**Every line goes through [logging.ts](src/logging.ts)** — pino, at
`OXYNOTE_AUTH_REALTIME_LOG_LEVEL` (`DEBUG`/`INFO`/`WARN`/`ERROR`, INFO by
default; the production image lowers it to WARN). Its records match core's
slog output, so both services in a deployment log alike. That includes the
two libraries that would otherwise print on their own terms: better-auth
takes the matching level and a `log` callback, and hocuspocus's Logger
extension is routed to DEBUG.

`createLogger` takes its destination as an argument — writing to stdout is a
side effect, so the sink belongs to `index.ts`, and a test passes a stream
it reads back. `Logger` is a four-method interface rather than pino's own
type, so a suite hands over four `vi.fn()`s.

**Nothing logs an address or a port**, here or in core: a container's bound
port is not the one anything reaches the service on.

`src/sentry.ts` is the exception: node loads it through `--import` before the
app graph exists, so it reads `process.env` directly and must not import
anything of ours. Sentry's own docs call this file `instrument.js` — the name
comes from "instrumentation", the APM term for the monkey-patching the SDK
does at load — but nothing outside `package.json` and the Dockerfile's `CMD`
depends on the filename, and `sentry.ts` matches how the rest of the
repository names its Sentry setup (`web/sentry.client.config.ts`,
`web/sentry.server.config.ts`).

## Environment

Every variable is declared in [src/env.ts](src/env.ts) and parsed once at boot
with zod. **A missing, malformed, or half-configured value is a boot error** —
the same policy core applies to its own env — so nothing surfaces later as an
`undefined` deep inside better-auth. Specifically:

- URLs are scheme-pinned to http/https: bare `z.url()` accepts `core:8080` as
  a valid URL with a `core:` scheme, which would boot and then fail on use.
- Counters must be positive integers; a typo is an error rather than a `NaN`
  limit that compares false against every count.
- Boolean flags accept only `"true"` and `"false"`, so a flag spelled `1`
  cannot read as false while the operator believes it is set.
- A social provider needs both halves of its credentials or neither. One half
  is a boot error rather than a broken OAuth redirect a user discovers.
- Docker's env files list every variable including the blank ones, so an
  unset value arrives as `""`. `loadEnv` drops those before parsing, which is
  what makes a required variable report as missing rather than empty.

Adding a variable means adding it to the schema, to the `Env` interface, and
to `docker/env/auth-realtime.example.env`. One reaching the production image
also needs a line in the launcher's `mapping.ts`, which is where the public
`OXYNOTE_*` namespace is translated into each component's own.

## Formatting & TS

Prettier uses **tabs at width 8** (matching gofmt's convention on the backend
side), no semicolons, trailing commas, double quotes — see
[prettier.config.js](prettier.config.js).

ESLint ([eslint.config.mjs](eslint.config.mjs)) runs **type-aware**:
`eslint:recommended` plus typescript-eslint **strictTypeChecked** and
**stylisticTypeChecked**, resolved through the tsconfig by `projectService`.
Consequences worth knowing:

- Linting builds a full TS program, so a cold run takes a few seconds. Runs
  are cached (`node_modules/.cache/eslint`); after cross-file type changes a
  cached file can report stale results locally, and `rm -rf
  node_modules/.cache/eslint` forces a full run. CI always runs cold.
- Root config files (`eslint.config.mjs`, `knip.ts`, `prettier.config.js`,
  `vitest.config.ts`) sit outside the tsconfig project and opt out of typed
  linting; the two type-aware rule overrides live in the `src/**` block for
  the same reason, since applying them to an untyped file crashes the run.
- Every `eslint-disable` **must** state a reason after `--`. It is for false
  positives only, never to avoid a fix. Stale disables are errors
  (`reportUnusedDisableDirectives`), and eslint runs with `--max-warnings 0`.
- `@typescript-eslint/no-explicit-any` is off. The `no-unsafe-*` family is
  **on**, and the codebase answers it by typing the boundary rather than
  relaxing the rule: untrusted JSON is read as `unknown` and narrowed
  (`readString`, `upstreamResponse`, `seedContent`), and tiptap's
  `any`-typed extension API gets an explicit cast at each use, the same way
  [web](../../web/app/components/editor/blocks/) does it.

TypeScript is `module: NodeNext`, `target: ESNext`, fully strict, with
`noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`,
`noImplicitOverride`, `verbatimModuleSyntax`, and `allowUnreachableCode:
false`. Two configs: [tsconfig.json](tsconfig.json) covers all of `src/`
(tests included) for `check-types` and eslint;
[tsconfig.build.json](tsconfig.build.json) excludes the test files so `pnpm
build` cannot emit them into `dist/`.

**ES module imports must include the explicit `.js` extension** (NodeNext +
ESM requirement), even when importing TypeScript files — e.g. `import { foo }
from "./bar.js"` for a `./bar.ts` source file.

**knip** ([knip.ts](knip.ts)) guards dead exports, unused files, and unused
dependencies. `ignoreExportsUsedInFile` is on for interfaces and types only: a
type exported so another export in the same file can name it — a union
member, a table in the `Database` map, a factory's dependency bag — is part
of that file's contract, not a dead export. Values are deliberately left out,
because an exported const nobody imports is worth hearing about. Knip runs
with `--cache`, and the cache is keyed by knip version, **not** config —
after editing `knip.ts`, run `rm -rf node_modules/.cache/knip` or results are
stale.

## Testing

### Layout

- **Tests are co-located**, paired 1:1 with the file they test:
  `ydocument.ts` → `ydocument.test.ts`. Never a parallel `tests/` mirror tree.
- **One environment, one suffix.** The service is plain node — no DOM, no
  framework runtime — so every test is `.test.ts` running in vitest's node
  environment. There is no suffix taxonomy to learn.
- When a test exercises behaviour spanning two source files, it lives with
  the file that initiates the behaviour.
- **The test file mirrors the source file's order**: top-level `describe`
  blocks appear in the same order as the functions they test appear in the
  source, so the two files read side by side.
- Helpers used by more than one suite live in
  [src/test-helpers.ts](src/test-helpers.ts) (`stubCore`, `stubStore`,
  `testEnv`, `fragmentXml`). It is excluded from coverage and from the build.
  Fixtures used by one suite stay file-local.
- [db.test.ts](src/db.test.ts) is the only suite that knows kysely builds a
  query by chaining. It stubs the builder to pin the table and column names
  each `Store` method touches; every other suite takes a `stubStore()`.
- **A yjs type must be added to a document before a test reads from it.**
  A detached `Y.XmlElement` or `Y.XmlText` keeps its attributes and children
  in prelim state, invisible to the public getters, and yjs answers the read
  with a console warning rather than an error — so the fixture is silently
  wrong and the test still passes. Build fixtures through an `attached()`
  helper (see `ydocument.test.ts` and `operations.test.ts`).
  [src/test-setup.ts](src/test-setup.ts) turns that warning into a failure so
  it cannot be missed: vitest prints intercepted stderr only under the
  verbose reporter, so in a piped or CI run the warning is invisible.
- `src/schema/` is the one place that departs from 1:1 pairing: the whole
  directory is tested through [src/schema/index.test.ts](src/schema/index.test.ts)
  rather than a test file per node. The files are one unit — a single
  editor schema, re-exported through `index.ts` — and the tests that
  matter run against the assembled whole: a node round-tripped through
  the transformer proves it is registered, which no per-file test can.
  The per-node HTML assertions are data tables in the same file for the
  same reason.

  What the suite pins there is the contract with the two services on
  either side. **Round trips** (prosemirror JSON → Y.Doc → prosemirror
  JSON) prove the schema knows each node, because a node it does not know
  is silently dropped on the way through — including the non-string
  attributes (`queries`, `thresholds`) that `cloneXmlElement` exists to
  protect. **The uid list** on the `UniqueID` extension is checked to
  contain every addressable block and to name only extensions that
  exist, because `operations.ts` addresses blocks by uid and a block
  missing from that list is one the assistant can never edit. **The
  `data-type` strings** are pinned because this service never serializes
  to HTML — those callbacks exist purely to stay identical to web's
  definitions, so nothing else would notice them drifting apart.

### Naming

**The rule: `describe` names the subject, `it` completes the sentence
"it …" with observable behaviour.**

```ts
describe("resolveBranchId", () => {
	it("returns a concrete branch identifier without asking core")
	it("resolves 'default' to the branch core flags as default")
	it("throws when no branch is flagged as default")
})
```

Read it aloud: "resolveBranchId throws when no branch is flagged as default."
If the concatenation isn't a grammatical sentence, the name is off — reporters
print exactly that concatenation on failure.

- Every test file wraps its tests in a root `describe` naming the unit by its
  real, greppable identifier. Never bare top-level `it` calls.
- Present tense, third person, **no "should"**.
- State behaviour, not implementation: `it("propagates a failed teardown so
  the organization survives")`, not `it("calls core.teardownOrganization")`.
  The name should survive a behaviour-preserving refactor.
- Be specific enough that a failure is informative. `it("works")` and
  `it("handles errors")` are useless.
- One level of `describe` nesting is usually right, two is the ceiling.

### Parameterized tests

**Use `it.for` wherever cases differ only in inputs and expected outputs.**
The moment cases differ in setup, mock wiring, or assertions, they are
separate `it`s — divergent cases in a shared callback breed conditionals, and
neither a breakpoint nor a reporter line can target a single row.

- Case objects use the same field vocabulary everywhere: `name`, `input`,
  `expected`, and the title interpolates them (`"rejects $name"`).
- Always `it.for`, never `it.each`: `.for` passes the test context as the
  second callback argument, which the concurrency rule below requires.
- Hoist bulky case data into named builders above the table.

### Mocks & path coverage

- **Every exported function is covered** — a module's exports are its
  testable contract.
- **Every path gets a test**: the success path and each failure path,
  including one per collaborator that can fail. Name them after the
  observable behaviour (`it("propagates a failed content fetch")`).
- **Dependencies are injected, not module-mocked.** The factory pattern above
  exists so a test can pass a stub; module mocks are file-level singletons
  that cannot vary per test and force suites out of concurrent execution.
  [reporting.test.ts](src/reporting.test.ts) is the one exception, and shows
  what one costs: sentry is a global reporter rather than an injected
  dependency, so asserting that a failure reaches it needs
  `vi.mock("@sentry/node")`, which in turn needs `{ concurrent: false }` on
  every describe that counts its calls and a `beforeEach` clearing it —
  vitest's `restoreMocks` does not reset a hand-made `vi.fn()`. Everywhere
  else, `reported` and `bestEffort` are asserted through what they do to the
  caller (rethrow or swallow), not through sentry.
- **No real IO.** No network, no database, no redis, no port. `createAuth` is
  exercised against a lazily-constructed pg pool that is never dialled, and
  everything else takes a stub.
- **One act per test**: arrange once, invoke the unit once, then assert as
  many facets of that single outcome as needed. Needing a second invocation
  is the real "and" smell — split the test.
- **Account for every injected dependency**: assert the calls that must have
  happened (count and arguments) *and* the zero counts of the ones that must
  not. `expect(core.fetchBranches).toHaveBeenCalledTimes(0)` after a
  rejected connection is as load-bearing as any positive assertion.
- Test through the module's public exports. Never export something solely for
  tests — an internal complex enough to need direct tests is extracted into
  its own module (which is how `createOrganizationHooks` came to exist).
- Plain `expect` for guards and preconditions; `expect.soft` is permitted in
  a final outcome-accounting block, where it reports the whole broken
  accounting at once.
- A branch that genuinely cannot be reproduced carries a `// NOCOV: <lowercase
  reason>.` comment as the first line inside the branch body. NOCOV is a
  reviewer covenant, not a tool directive — it is deliberately not wired to
  the coverage provider (whose ignore hints need the banned `/* */` comment
  form), so coverage reports stay truthful and the marker explains the
  residual gap in place. It never excuses a testable path.

### Independence & concurrency

- **Every test is completely independent**: it creates its own fresh state (a
  new `Y.Doc`, a new stub, a new app) and never depends on execution order.
- **All tests run concurrently** (`sequence.concurrent: true`). Under
  concurrency, assertions must use the `expect` from the local test context —
  `it("…", ({ expect }) => …)` — because the global `expect` cannot reliably
  attribute a failure to the right test when tests interleave. This applies
  to every test in the suite, `it.for` rows included.
- Mock and global state cannot leak: the config sets `restoreMocks`,
  `unstubGlobals`, and `unstubEnvs`, so spies, stubbed globals, and env stubs
  are restored after every test without per-file cleanup.

### Determinism

- No `setTimeout`-based waiting. Await concrete signals — a promise the unit
  returns, an emitted event.
- Time is driven, not waited on: `vi.useFakeTimers()` +
  `advanceTimersByTime()`, `vi.setSystemTime()` for deterministic timestamps.
  Assert exact values, never wall-clock deltas.

### Coverage

`pnpm test` runs with coverage and **fails under the thresholds** in
[vitest.config.ts](vitest.config.ts). They are set from the measured baseline
and ratcheted up as suites grow — **never lowered**. 100% is not the target:
genuinely untestable branches stay visible in the report, marked with NOCOV
in place. `src/index.ts` is excluded because evaluating it *is* starting the
service; what it wires together is covered through the factories it calls.

## Yjs

Read the document-storage section of [server/CLAUDE.md](../CLAUDE.md) before
touching branch content. The rule that matters most here: **never seed or
merge one Y.Doc from another with `Y.applyUpdate`** — the differing
`clientID`s CRDT-merge and silently duplicate every block. Use
`replaceYdocContent` ([src/ydocument.ts](src/ydocument.ts)), and persist
`rawContent` immediately after replacing it, so a restart cannot rebuild the
document under a new clientID. Both invariants are pinned by tests; if you
change that code, the tests in `ydocument.test.ts`, `hocuspocus.test.ts` and
`routes.test.ts` are the specification.

## Better Auth schema

The core migrations own all tables, including Better Auth's. The generated
`sql/better_auth_schema.sql` is **reference output only — never apply it
directly**; regenerate it to diff what Better Auth expects after changing
`src/auth.ts`. See the "Database" section of [server/CLAUDE.md](../CLAUDE.md).
