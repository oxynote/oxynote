# AGENTS.md

Guidance for the `e2e/` suite. Shared principles and TS/JS style live in the
root [AGENTS.md](../AGENTS.md). The testing standards are `web/`'s
([web/AGENTS.md](../web/AGENTS.md)) adapted to this tier; where a rule
differs, the difference is stated.

## Project

`@oxynote/e2e`: Playwright tests driving the composed product through a
browser, plus the compose stacks they run against.

**Two suites share every test.** The dev suite drives the four containers
this repo builds; the prod suite drives the all-in-one image
([docker/prod/](../docker/prod/AGENTS.md)) and adds `prod-*.test.ts`.
**Everything belonging to one stack says which, and `dev` is never the
unnamed default**: `docker-compose.{dev,prod}.yaml`,
`playwright.{dev,prod}.config.ts`, `test:{dev,prod}`, `make e2e-{dev,prod}`,
`qa-e2e-{dev,prod}.yml`. Use `prod`, never `production`, in names.

The package lives at the repo root, not in `web/`: the tests belong to no
component, and Playwright's `test`/`expect` would collide with vitest's
inside web's toolchain. It owns its own tsconfig, eslint and prettier
configs.

## Commands

```sh
make e2e-dev  | e2e-dev-stack-build  | e2e-dev-stack-stop     # from the repo root
make e2e-prod | e2e-prod-stack-build | e2e-prod-stack-stop

pnpm setup        # deps + playwright chromium         (from e2e/)
pnpm test:dev     # full cycle against the dev stack;  test:prod likewise
pnpm check-lint   # check-types + eslint + prettier + knip;  lint = fixing variant
pnpm qa           # check-lint + test:dev;  qa-fix = lint + test:dev
```

**The playwright config owns the whole cycle**: `global-setup.ts` builds and
starts the stack, `global-teardown.ts` stops it and drops its volumes, so
every entry point (`make`, `pnpm`, the VS Code extension) runs the same
cycle and nothing tests a stale image. The build shells out to `make
e2e-*-stack-build` because compose cannot build core (goreleaser does) and
because `E2E_DEV_COMPOSE_EXTRA` is how CI layers `docker-compose.dev.ci.yaml`
over the base file.

Both configs set `E2E_STACK` as their first statement; `helpers/config.ts`
resolves ports from it and `helpers/stack.ts` picks the compose file and
make targets. Neither config sets `use.baseURL` (a config cannot influence a
module it imports), so `visit()` splices the origin on.

To iterate without a full build each run: `make e2e-dev-stack-build` once,
then `pnpm exec playwright test -c playwright.dev.config.ts`. **`-c` is
required**; neither config has playwright's default filename. You are then
responsible for rebuilding after source changes; a stale image fails
silently.

The stack has no pnpm scripts and should not grow any. Logs and container
access are plain `docker compose -f docker-compose.{dev,prod}.yaml ...` from
this directory.

## The dev stack

`docker-compose.dev.yaml` is the backend built from this repo's source,
differing from `docker/docker-compose.dev.yaml` in:

- **No named volumes**; every run starts empty. Do not add persistence.
- **No `container_name`**, own ports (`:18080` front door, `:18025`
  mailpit).
- **Env inlined and committed**; a fixture, not an operator template.
- **Every image pinned.**
- **Images tagged `:e2e-dev`** (`:e2e-prod` in the prod stack), never
  `:dev`, so a run never replaces the image `make start` runs. Core's tag
  threads through `make build-go CORE_IMAGE_TAG=e2e-dev` →
  `CORE_SNAPSHOT_TAG` in `.goreleaser.yml`; the prod image takes
  `PROD_IMAGE_TAG`.
- **Only the services the tests touch**: no changedetection, mariadb,
  datagen, prometheus, grafana.
- **`docker/Caddyfile` mounted verbatim**, so the `/api/x` and
  `/api/internal` blocks that ship are what is tested.
- **Core has no compose healthcheck** (distroless image); `global-setup.ts`
  polls it through the front door, treating any response under 500 as
  ready.

Mailpit makes the auth flows testable: verification is mandatory, so tests
fetch the real message via mailpit's REST API and follow the link. **Neither
stack runs valkey or an object store**; that is the configuration most
installs run. A test that needs either says why.

## The prod stack

`docker-compose.prod.yaml` is `docker/prod/docker-compose.example.yaml`
retuned as a fixture (`:19080`, `:19025`, inlined env, pinned images).
Compose builds nothing; the image comes from `make prod-build`, tagged
`:e2e-prod`, and CI's cache reaches it through `PROD_BUILD_EXTRA`. A `probe`
container (idle alpine on the private network) exists for the
trust-boundary tests: internal ports are not published, so only a sibling
container can tell loopback from wildcard binds.

## Testing standards

### Layout

- **Test files live in `tests/`, one per user-facing flow**, named as a user
  would (`signup`, `login`, `editor`, `collaboration`). Never `.spec.ts`.
  There is nothing to pair 1:1 with; an e2e test crosses four services.
- **`prod-*.test.ts` is the one exception to naming by flow**: each observes
  something only true of the all-in-one image. The dev config ignores them
  (`testIgnore`). A test earns the prefix only if it would be meaningless or
  misleading against the dev stack.
- **Shared code lives in `helpers/`, one module per concern.** Helpers are
  setup and plumbing, not the assertion a test exists to make. A setup
  helper does assert its own success (`signUpAndVerify` checks the account
  activated) so a broken precondition is reported where it broke.
- **`signUpWithWorkspace` is the starting point for almost everything**:
  signup, verify, login, create workspace, welcome document open. There is
  no seeding.
- Shared starting state moves into `helpers/`, never into a test another
  test depends on.

### Naming

`test.describe` names the flow as a user would; `test()` completes "it …"
with observable behaviour, present tense, no "should", naming the outcome
not the mechanics (`test("takes a verified user to workspace creation")`,
not `"redirects to /welcome"`). Specific enough to diagnose from the failure
line. One level of nesting is the ceiling.

### Parameterized tests

Playwright has no `it.for`; generate tests from a table at module scope
(`for (const c of CASES) test(\`refuses a password with ${c.name}\`, ...)`).
A table is only for cases differing in inputs and expected output;
different steps mean separate tests. Every generated title must be
distinct. Case fields: `name`, `input`, `expected`. Never call `test()`
inside a `test()`.

### Scope

This is the expensive tier. A test earns its cost only by observing what no
cheaper tier can:

- **Belongs here**: a flow crossing process boundaries (browser → Caddy →
  web → auth-realtime → core → Postgres → SMTP).
- **Does not**: validation messages, loading/empty states, disabled
  buttons, copy, component branches. Those are `web/` component tests.
- **The test: would this still fail if the backend were a mock?** If yes,
  it belongs one tier down. Coverage here is by flow, not by branch.
- **A negative case belongs here when the server decides it**: "user A
  cannot open user B's document" is answered by core alone. Access denials
  are grouped in `tests/access.test.ts`, since a feature test always runs
  as the user who has access.
- **The trust boundary is tested from outside the image** in
  `tests/prod-trust-boundary.test.ts`: the loopback binds (through `probe`)
  and caddy's prefix blocks are asserted separately, since either alone is
  a single point of failure. The bind table carries a positive control (the
  probe reaching the front door). Front-door cases use `rawRequest` over
  `node:http`, because `fetch` normalises `..`, duplicate slashes and
  encoding away; recognised spellings must be `403`, traversal attempts
  must merely never succeed.
- **A denial that does not hold yet is `test.fail()`** with a comment
  saying what is missing. Check its duration: a hanging test also registers
  as an expected failure.

### A real backend, no mocks

- Never `page.route()` an application endpoint, stub a response or seed the
  database out of band. Reach state through the product's own surface; the
  HTTP API is the fallback where the UI offers no path, never SQL.
- Mail is real (mailpit). Third-party edges (OAuth providers) have no
  doubles, so only email-password auth is covered.
- Needing a mock is the signal the case belongs in the component tier.
- **Collaboration runs as two real users**: `joinAsSecondUser` sends the
  invitation from settings and has the invitee sign up, verify, log in and
  accept in a fresh context. Presence assertions key on the caret's name
  label, never its colour (random per client).

### Locators

- Prefer role and placeholder over CSS.
- **Never hardcode a translation.** Assert through `t()` from
  `helpers/i18n.ts`, interpolation included
  (`t("onboarding.verify-email.sent-title", { email })`), and pick the key
  the component actually renders rather than grepping the locale file for
  the text.
- Overlays teleport into `#teleports`; menus and dialogs are appended to
  `<body>`. Reach them from `page` (`openSlashMenu()`,
  `getByRole("dialog")`), not from a locator scoped to the trigger's
  container.
- **Strict mode is a feature**: fix a locator that matches twice, do not
  `.first()`. `sidebarDocument()` and `openDocumentActions()` scope the
  common duplicates.
- **Read editor text through `editorText()`**: a collaborator's caret label
  is rendered inside the paragraph, so a plain text read splices the other
  user's name into the content.
- **The title and the body are two editors** (`titleEditor()`,
  `contentEditor()`); Enter in the title moves into the body.

### Independence & concurrency

- Every test mints its own account and workspace (`newCredentials()`,
  `newWorkspace()`), which is what makes `fullyParallel` safe against one
  shared stack. No test depends on another or cleans up; the database is
  thrown away wholesale.
- **Workers: four locally, two on CI. Turning workers down is not how a
  flake gets fixed**: every failure that only appeared under load was a
  real defect (a session read once, sync connections never torn down, a
  toast asserted after auto-dismiss). Chase it. The test budget is 60s;
  collaboration tests are marked slow.
- Browser contexts are per-test; share a session only through an explicit
  fixture.

### Determinism

- **Never sleep**: no `page.waitForTimeout`, no bare timeout as
  synchronisation. Wait on the condition (`toHaveURL`, `toBeVisible`); for
  state outside the page (a delivered email) use `expect.poll` with a
  `message`.
- **Interact only after hydration**: navigate with `visit()`
  (`helpers/page.ts`), since SSR markup accepts clicks before vue has
  attached.
- Assert the exact URL when a parameter carries meaning (`?verified=true`).
- **`documentPersisted()` is the one permitted `waitForTimeout`**:
  hocuspocus stores two seconds after the last change and nothing reports
  it.
- Three waits are raised above the default (post-signup redirect,
  post-login redirect, cold document load), all cross-service chains. Any
  other raised timeout carries a comment saying what is slow.

### Failure artifacts

`trace: "on-first-retry"`, `screenshot: "only-on-failure"`; `test-results/`
and `playwright-report/` are gitignored. `docker compose logs` gives the
service side. A failure only under load or in CI is almost always a wait
that was not a wait: fix the signal, do not raise the timeout.

## Formatting & TS

Prettier: tabs, no semicolons, trailing commas. ESLint is type-aware
(`strictTypeChecked` + `stylisticTypeChecked`), above all because it reports
an unawaited promise, which in Playwright is an assertion that never ran.
`--max-warnings 0`; every `eslint-disable` states its reason after `--`.
`eslint-plugin-playwright` (scoped to `tests/` and `helpers/`) enforces
`no-wait-for-timeout`, `no-focused-test`, `no-skipped-test` and
`missing-playwright-await`. knip needs no config: its Playwright plugin
resolves both configs and their global hooks. TypeScript is strict with
`noUnusedLocals`, `noUnusedParameters`, `noUncheckedIndexedAccess`,
`noImplicitOverride`, `verbatimModuleSyntax`.

## CI

`qa-e2e-dev.yml`: a `lint` job and a `test` job running `make e2e-dev` (it
sets up Go and goreleaser for core's image and uploads the html report on
failure, so the reporter is `github` and `html`). **Triggers are asymmetric
on purpose**: a merge to `main` runs it for changes anywhere that can break
it; a pull request only for changes to `e2e/`, since a full run builds a
nuxt image. `concurrency` supersedes earlier runs. Layer caching comes from
`docker-compose.dev.ci.yaml` via `E2E_DEV_COMPOSE_EXTRA` (`type=gha` would
break local builds).

`qa-e2e-prod.yml` has no trigger of its own: `release.yml` calls it and
refuses to publish unless it passes; `workflow_dispatch` re-verifies an
image without a release. Its cache reaches the build through
`PROD_BUILD_EXTRA`. Every workflow release.yml calls declares
`workflow_call`; path filters do not apply to a called workflow, so each
runs in full on a tag. The dev suite is deliberately not a gate and is the
one QA workflow without `workflow_call`.

## Not covered

The Electron build: Playwright drives it through `_electron` and its auth
runs over IPC, a separate track.
