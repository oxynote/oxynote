# CLAUDE.md

Guidance for the `e2e/` end-to-end suite. Shared working principles and
the TS/JS comment/whitespace rules live in the root
[CLAUDE.md](../CLAUDE.md). The testing standards below are the `web/`
standards ([web/CLAUDE.md](../web/CLAUDE.md)) carried over and adapted —
where a rule differs, it differs because the tier does, and the reason is
stated.

## Project

`@oxynote/e2e` — Playwright tests that drive the composed product through
a browser, plus the docker-compose stacks they run against.

**There are two suites and they share every test.** The dev suite drives
the four containers this repository builds; the prod suite drives the
all-in-one image an operator actually installs
([docker/prod/](../docker/prod/CLAUDE.md)), and adds the `prod-*.test.ts`
files, which only mean anything there. Neither is a subset of the other:
the flow tests are the same files, run twice, because the thing under test
is a different assembly of the same product.

**Everything that belongs to one stack says which, and `dev` is never the
unnamed default.** `docker-compose.dev.yaml` and `docker-compose.prod.yaml`,
`playwright.dev.config.ts` and `playwright.prod.config.ts`, `test:dev` and
`test:prod`, `make e2e-dev` and `make e2e-prod`, `qa-e2e-dev.yml` and
`qa-e2e-prod.yml`. A bare name for one half and a qualified name for the
other reads as though the bare one were the whole suite, which is exactly
the confusion worth spending a few characters to avoid — and it costs
`-c`/`-f` on the direct commands, which is a fair price. Use `prod`, not
`production`, in every name; `production` stays a word for the real-world
thing a deployment is, as in `NODE_ENV=production`.

**It lives at the repository root, not in `web/`, for two reasons.** The
tests exercise the whole system — the web app through Caddy, real core,
real auth-realtime, real Postgres — so they belong to no single
component. And keeping them out of `web/` keeps them out of `web/`'s
toolchain: Playwright's `test`/`expect` would collide with vitest's, and
the type-aware ESLint program, the knip scan and the Nuxt tsconfig
solution would each need a carve-out for files that are not part of the
app. This package owns its own tsconfig, eslint and prettier configs
instead.

## Common commands

Package manager is **pnpm**. From the repository root:

```sh
make e2e-dev             # run the suite against the dev stack; builds and tears down
make e2e-dev-stack-build # build that stack's images, for iterating
make e2e-dev-stack-stop  # stop it and drop its data

make e2e-prod             # the same suite against the all-in-one image
make e2e-prod-stack-build # build that image
make e2e-prod-stack-stop  # stop it and drop its data
```

From `e2e/`:

```sh
pnpm setup       # deps + the playwright chromium browser
pnpm test:dev    # playwright against the dev stack (full cycle, see below)
pnpm test:prod   # the same, against the all-in-one image

pnpm check-lint  # check-types + check-eslint + check-fmt + check-knip
pnpm lint        # the fixing counterpart of all four
pnpm qa          # check-lint + test:dev; qa-fix = lint + test:dev

pnpm check-types # tsc --noEmit
pnpm check-eslint # eslint --max-warnings 0 . (eslint / fmt are the
pnpm check-fmt   #   fixing counterparts)
pnpm check-knip  # dead exports, files and dependencies
```

**The playwright config owns the whole cycle.**
[global-setup.ts](global-setup.ts) builds the stack and brings it up;
[global-teardown.ts](global-teardown.ts) stops it and drops its volumes.
Nothing has to be running first, and nothing is left behind.

That is deliberate, and the reason is that a test suite has several front
doors — `make e2e-dev`, `pnpm test:dev`, and the play button in the Playwright
VS Code extension — and only the config is common to all of them. Putting
the build in the caller meant the extension tested whatever image was
last left behind, which passes and lies. Now every entry point runs the
same cycle, so `make e2e-dev` is a one-line target that only starts the run.

The build still lives in the Makefile, and the hook shells back out to
`make e2e-dev-stack-build` rather than reimplementing it. Two reasons: the
hook cannot express it — goreleaser produces core's image, which compose
cannot build at all, since the service has no build section — and going
through make is what keeps `E2E_DEV_COMPOSE_EXTRA` working, which is how CI
layers the buildx cache config in
[docker-compose.dev.ci.yaml](docker-compose.dev.ci.yaml) over the base file.

**Both suites run through the same setup and teardown**, which is why
they take the stack they drive from `E2E_STACK` rather than from a second
copy of the hooks. Each playwright config sets it as its first statement,
before playwright loads the global setup or forks a worker — both inherit
the config process's environment — and
[helpers/config.ts](helpers/config.ts) reads it to resolve the ports while
[helpers/stack.ts](helpers/stack.ts) reads it to pick the compose file and
the make targets. Nothing else in the suite knows which stack it is on.

The knock-on is that **neither config sets `use.baseURL`**: a config is
evaluated before it can influence a module it imports, so the origin has
to come from the helpers instead. `visit()` splices it on, and every other
navigation and request in the suite was already absolute.

**The cost is that every run is a full build and teardown.** With warm
caches and no source change that is tens of seconds, not minutes, and a
dropped volume per run means each run starts from an empty database. To
iterate without paying it, build once with `make e2e-dev-stack-build` and
drive playwright directly with
`pnpm exec playwright test -c playwright.dev.config.ts`, which is the same
binary without the make wrapper — but understand that you are then
responsible for rebuilding after a source change, and that a stale image
fails silently. **`-c` is not optional**: neither config carries
playwright's default filename, so a bare `playwright test` finds nothing.
That is the price of naming both stacks rather than leaving the dev one
implicit, and it is the right trade — an unnamed default is exactly what
makes two-stack setups ambiguous.

Both build steps run quietly, under a ticking line, and replay their
whole log if they fail. Docker and goreleaser say a great deal that
matters only when something breaks.

**The stack has no pnpm scripts of its own**, and should not grow any.
Its lifecycle belongs to the setup and teardown hooks, which reach the
Makefile through [helpers/stack.ts](helpers/stack.ts); anything else —
reading logs, poking at a container — is plain `docker compose` run from
this directory with `-f docker-compose.dev.yaml` or
`-f docker-compose.prod.yaml`. Neither carries compose's default name, for
the same reason neither playwright config does: which stack a command
drives is never left to a default. A `pnpm stack:*` wrapper would only be a
second name for a command that is already shorter than its alias.

## The stack

[docker-compose.dev.yaml](docker-compose.dev.yaml) is the whole backend,
built from this repository's own source — the same images and Dockerfiles the
dev stack uses, never a published release. It deliberately differs from
`docker/docker-compose.dev.yaml`:

- **No named volumes**, so every run starts from an empty database. This
  is what makes the suite deterministic; do not add persistence.
- **No `container_name`**, and its own ports (`:18080` front door,
  `:18025` mailpit), so it runs at the same time as the dev stack.
- **Env is inlined and committed.** There is nothing secret in it, so CI
  needs no setup step. Unlike `docker/env/*.example.env`, it lists only
  the variables that matter — it is a fixture, not an operator template.
- **Every image is pinned**, as in the dev stack. A run has to fail
  because this repository changed, not because an upstream `latest`
  moved overnight.
- **The images this repo builds are tagged `:e2e-dev` here and
  `:e2e-prod` in the prod stack**, never the `:dev` the dev stack uses.
  A tag names the stack an image belongs to, and the separation is load
  bearing rather than cosmetic: sharing a tag would mean an e2e run
  silently replaced the image `make start` is running. Core's comes from
  goreleaser, so its tag is threaded through as
  `make build-go CORE_IMAGE_TAG=e2e-dev` → core's `IMAGE_TAG` →
  `CORE_SNAPSHOT_TAG`, which `.goreleaser.yml` reads as the snapshot
  version. The prod image takes `PROD_IMAGE_TAG` the same way.
- **Only the services the tests touch.** No changedetection,
  sockpuppet-chrome, mariadb, datagen, prometheus or grafana.
- **The front door is `docker/Caddyfile` verbatim**, mounted from the dev
  stack rather than copied. E2e must exercise the routing that ships,
  including the `/api/x` and `/api/internal` blocks; a second copy would
  drift.

One knock-on constraint worth knowing:

- **Core cannot have a compose healthcheck** — its image is distroless,
  with no shell or http client to probe itself with. `global-setup.ts`
  polls it through the front door instead, treating any response under
  500 as ready (core answers 401 to an unauthenticated probe; Caddy
  answers 502 on its behalf while it is still starting).

**Mailpit is what makes the auth flows testable end to end.** Email
verification is mandatory (`requireEmailVerification`), so tests fetch
the real message from mailpit's REST API and follow the real link —
nothing is stubbed or short-circuited.

**Neither stack runs valkey or an object store.** Both are optional
everywhere in the product: without them the assistant keeps its
conversations in the core process, better-auth keeps no secondary session
storage, and uploaded images live on the filesystem. That is what a
deployment does when those variables are unset, so it is the configuration
most installs actually run — and testing it here is worth more than
testing a shape that only larger deployments have. A test that needs
either one back has to say why in the same breath.

## The prod stack

[docker-compose.prod.yaml](docker-compose.prod.yaml) is
`docker/prod/docker-compose.example.yaml` — the deployment the
image's README documents — retuned as a fixture: throwaway state, its own
ports (`:19080` front door, `:19025` mailpit), inlined env, pinned images.
It runs alongside both other stacks.

Two things about it are not variations on the dev stack:

- **Compose builds nothing.** The image comes from `make
  prod-build`, because core's binary has to be produced by
  goreleaser and staged where the Dockerfile's `COPY` expects it. There is
  no build section to point compose at, which is also why the CI cache
  reaches the build through `PROD_BUILD_EXTRA` on the `docker build`
  rather than through a compose override. It is tagged `:e2e-prod`.
- **There is a `probe` container**, an idle alpine on the same private
  network. It exists for [tests/](tests/) and is the
  only honest way to ask whether a container that is not caddy can reach
  core, auth-realtime or the web server directly. Probing published host
  ports would answer nothing: the internal ports are not published, so
  every such probe passes whether the services bind loopback or the
  wildcard.

## Testing standards

### Layout

- **Test files live in [tests/](tests/), one per user-facing flow**,
  named after the flow as a user would name it — `signup`, `login`,
  `onboarding`, `editor`, `collaboration`. Never `.spec.ts`: the repo
  encodes meaning in the suffix and `.spec` is not one of them.
- There is no mirror of the app's source tree and nothing to pair 1:1
  with. `web/` co-locates because a test there has exactly one subject
  file; an e2e test has none — it crosses four services by design, so it
  is filed by flow instead.
- **A `prod-` prefix is the one exception to naming by flow**, and it is
  filed by stack rather than by flow because that is what those cases have
  in common: each observes something only true of the all-in-one image.
  The dev config ignores `prod-*.test.ts` (`testIgnore`), so the prefix is
  what decides a case runs once instead of twice. They stay in
  [tests/](tests/) beside everything else — a subdirectory would suggest a
  second tier, and there is only one. A test earns the prefix only if it
  would be meaningless or misleading against the dev stack; a flow that
  merely happens to work in both is a normal flow test, run by both suites
  already.
- **Shared code lives in [helpers/](helpers/), one module per concern**
  (`auth`, `workspace`, `editor`, `collaboration`, `mailpit`, `i18n`,
  `page`, `config`, `api`, `realtime`). Helpers are setup and plumbing;
  they do not carry the assertion a test exists to make.
- **`signUpWithWorkspace` is the starting point for almost everything.**
  It signs up, verifies, logs in and creates a workspace, and hands back
  a page with the welcome document open. Nearly every flow needs that
  state and nothing cheaper gets there — there is no seeding.
- The one exception: **a setup helper asserts its own success**, so a
  broken precondition is reported where it broke.
  `signUpAndVerify` checks the account actually activated rather than
  letting the login three steps later fail for a reason that reads like
  a login bug. Assertions about the _behaviour under test_ still belong
  in the test.
- When two flows need the same starting state, the setup moves into
  `helpers/` — never into a shared test file, and never into a test that
  another test depends on having run.

### Naming

**The rule is the repo's: the group names the subject, the test name
completes the sentence "it …" with observable behaviour.**

```ts
test.describe("signup", () => {
	test("sends a verification link that activates the new account")
	test("refuses a password shorter than the minimum")
})
```

Read aloud: "signup sends a verification link that activates the new
account." If the concatenation is not a grammatical sentence, the name is
off — and reporters print exactly that concatenation on failure.

The rule carries over from `web/`; the identifiers do not. Playwright has
no `it` at all, and `describe` is not a typed export — `test()` and
`test.describe()` are the whole API. What a vitest suite writes as
`it("returns null")`, a Playwright suite writes as `test("returns null")`,
and it still reads as "it …" once concatenated.

- **`test.describe` names the flow the way a user would** — `signup`,
  `login` — not the page component or the file.
- **Present tense, third person, no "should".**
- **Name the outcome, not the mechanics.**
  `test("takes a verified user to workspace creation")`, not
  `test("redirects to /welcome")`. The name should survive a route
  rename; the URL belongs in the assertion.
- Be specific enough that the failure line diagnoses itself:
  `test("signs in")` is useless, `test("rejects an unknown email")` is
  not.
- One level of nesting is the ceiling, as in `web/`.

### Parameterized tests

Playwright has no `it.for`. The equivalent is generating tests from a
table at collection time:

```ts
const CASES = [
	{ name: "no digit", password: "no-digits-in-here!" },
	{ name: "no symbol", password: "Passw0rdPassw0rd" },
]

for (const c of CASES) {
	test(`refuses a password with ${c.name}`, async ({ page }) => {
		// ...
	})
}
```

The rules carry over unchanged from `web/`:

- **A table is only for cases that differ in inputs and expected
  output.** The moment two cases differ in their _steps_ — a different
  page, a different sequence — they are separate tests, not rows sharing
  one callback.
- **Every generated title must be distinct.** Playwright rejects
  duplicates within a file, and a shared title makes a report
  undiagnosable.
- Case objects use the repo's vocabulary: `name`, `input`, `expected`.
- Generate at module scope. Never call `test()` inside another `test()`.

### Scope — what earns a test here

This is the expensive tier: a browser, a full stack, seconds per case
instead of milliseconds. A test earns that cost only by observing
something no cheaper tier can.

- **Belongs here:** a flow crossing process boundaries — browser →
  Caddy → web → auth-realtime → core → Postgres → SMTP. Signup qualifies
  because the verification link is minted in auth-realtime, rendered and
  sent by core, and redeemed back through the front door.
- **Does not belong here:** field validation messages, loading and empty
  states, disabled buttons, copy, component branches — anything decided
  in the browser alone. Those are component tests in `web/`, where they
  run without a stack.
- **The test to apply: would this still fail if the backend were a
  mock?** If yes, it belongs one tier down.
- **Coverage is by flow, not by branch.** `web/` owns "every exported
  function is covered" and "every path gets a test"; this tier owns "the
  wiring holds". Reaching for one more browser-decided error-path e2e is
  nearly always the wrong instinct — add it as a component test instead.
- **A negative case belongs here when the server is what decides it**,
  and it is a first-class regression harness rather than a concession.
  What the rule above excludes is the browser deciding — a validation
  message, a disabled button. A refusal that crosses the process
  boundary is the opposite: "user A cannot open user B's document" is
  answered by core and nowhere else, so it meets the mock test as
  squarely as the happy path beside it, and it guards a class of bug
  that silently reappears the day someone drops an org filter.
- **Access denials are grouped in
  [tests/access.test.ts](tests/access.test.ts)** rather than filed under
  the flow each one touches. A per-feature file never reaches them: a
  feature test always runs as the user who is supposed to have access,
  which is why a missing check survives a suite that covers every line.
  Naming follows the same rule as everywhere else — the group names the
  subject, each name completes "it …" with the denial it observes.
- **The trust boundary is tested from outside the image**, in
  [tests/prod-trust-boundary.test.ts](tests/prod-trust-boundary.test.ts).
  Two independent things keep core's `/api/x/*` and auth-realtime's
  `/api/internal/*` unreachable — the services bind the container's
  loopback, and caddy refuses the prefixes at the front door — and the
  file asserts them separately, because either one alone is a single
  point of failure and a suite that conflated them would go green with
  one of them gone. The launcher runs its own bind check at boot; this
  asks the same question from the network, so a regression that took the
  boot gate with it is still caught.
  - The bind cases go through the `probe` container, and the group carries
    a **positive control** — the same probe reaching the front door — so a
    broken network cannot pass as a loopback bind. Every table-driven
    negative here needs one; without it the whole group is one typo away
    from proving nothing.
  - The front-door cases are sent with `rawRequest`, over `node:http`,
    because the point is what caddy does with a path and `fetch` would
    normalise `..`, duplicate slashes and encoding away before the request
    ever left. Two tables: spellings caddy is expected to recognise as the
    same request must be `403`, and traversal attempts must simply never
    succeed — asserting a specific status for those would couple the suite
    to caddy's normalisation rather than to the property that matters.
- **A denial that does not hold yet is marked `test.fail()`**, with a
  comment saying what is missing and what to delete when it is fixed.
  That keeps the gap in the suite instead of in a backlog, and Playwright
  turns the annotation red the day the case starts passing. Check how
  long such a case takes when you add one: a test that hangs also
  registers as an expected failure, so a broken probe and a real gap
  look identical until you read the duration.

### A real backend, and no mocks

`web/` says only e2e is allowed a real backend. The counterpart here:
**e2e is required to use it.**

- **Never `page.route()` an application endpoint, never stub a response,
  never seed the database out of band.** A stubbed e2e proves nothing
  the component tier had not already proven more cheaply.
- **Reach state through the product's own surface.** An account exists
  because a test signed up for it. Where the UI genuinely offers no path,
  the product's HTTP API is the fallback — never SQL, which would let a
  test set up state the product itself cannot produce.
- **Mail is real.** Verification links are read out of mailpit; nothing
  marks an account verified behind the app's back.
- Needing a mock to make a case practical is the signal that the case
  belongs in the component tier.
- Third-party edges (the OAuth providers) have no doubles here, which is
  why only email-password auth is covered.
- **Collaboration runs as two real users.** `joinAsSecondUser` walks the
  whole journey a teammate takes: the owner sends the invitation from
  settings, and the invitee signs up, verifies, logs in and accepts the
  emailed invite in a fresh browser context — nothing is granted behind
  the product's back. Presence assertions key on the caret's name label
  (each side sees the *other* user's name), never its colour, which is
  random per client.

### Locators

- **Prefer role and placeholder over CSS**: `getByRole("button", { name })`,
  `getByPlaceholder(...)`. A restyle must not break a test that still
  describes the same user action.
- **Never hardcode a translation.** Assert through `t()` from
  [helpers/i18n.ts](helpers/i18n.ts), which resolves the key against the
  same locale files the app renders with, interpolation included. A
  copied english string is a second, unmaintained definition of the
  message: it keeps passing when the copy changes, and a message edit
  silently breaks a suite that has nothing to do with it. The rules that
  follow from it are the `web/` ones:
  - **Interpolated messages take their values through `t()` too** —
    `t("onboarding.verify-email.sent-title", { email })`, never a
    hand-spliced string and never a bare prefix that skips the
    interpolation.
  - **Pick the key the component actually renders.** Many messages share
    an english value, so the wrong key passes today and diverges the day
    one of them is reworded. Follow the component to its key rather than
    grepping the locale file for the text.
- **Overlays teleport out of the page's main tree** into `#teleports`.
  They are reachable from `page`, but not from a locator scoped to the
  container the trigger lives in.
- **Strict mode is a feature.** When a locator matches more than one
  element, fix the locator — do not reach for `.first()`. A document
  name is a link twice (sidebar row and breadcrumb) and every tree row
  has an "Open Page Actions" button; `sidebarDocument()` and
  `openDocumentActions()` scope those for you.
- **Read editor text through `editorText()`**, never `toHaveText` or
  `innerText` on the editor. A collaborator's caret is a decoration
  widget rendered inside the paragraph it sits in, label and all, so a
  plain text read of a shared document comes back with the other user's
  name spliced into the content.
- **The title and the body are two editors.** `titleEditor()` is its
  own tiptap instance with a one-paragraph schema and its own yjs field;
  it is not the first heading of `contentEditor()`. Enter in the title
  moves the caret into the body rather than adding a line.
- **Menus are appended to `<body>`.** The slash menu, dropdowns and
  dialogs are all outside the editor and sidebar subtrees; `openSlashMenu()`
  returns the menu, and dialogs come through `getByRole("dialog")`.

### Independence & concurrency

- **Every test creates its own account and its own workspace** via
  `newCredentials()` and `newWorkspace()`, which mint a unique email and
  a unique slug. Unique-per-test state is what makes `fullyParallel`
  safe against a single shared stack — the same independence rule
  `web/` states, enforced by construction rather than by mock resetting.
  A workspace slug is unique server-side, so two tests sharing one would
  fail on the second, exactly as two sharing an email would.
- **Workers are capped at four locally and two on CI.** Every test's
  setup is several cross-service round trips against one core and one
  postgres; past a few workers the setup starts timing out, which reads
  as flaky tests when it is only load. CI gets fewer as headroom rather
  than because of any particular runner — it carries the stack and the
  browsers on one machine, and a green suite there is worth more than a
  slightly faster one. For the same reason the test budget is 60s, well
  above what a test takes locally, and the collaboration tests are marked
  slow, tripling theirs:
  each one runs two signups, two verifications, an invitation and its
  acceptance before the first assertion.
- **Both suites run four workers, and turning that down is not how a
  flake gets fixed.** The prod stack contends harder than the dev one —
  caddy, web, core and auth-realtime share a single container there — and
  that pressure is worth keeping, because it is what surfaces the real
  defects. Every failure that only appeared at four turned out to be one:
  a form that read its session once instead of following it, sync
  connections that were never torn down when a tab closed, a dialog
  dismissed with one keypress when two were open, a toast asserted after
  it had auto-dismissed, and cross-service chains waited on with the
  default five seconds. Each was fixed where it lived. If a case fails
  only under load, that is the finding — chase it, do not widen the gap
  it hides in.
- **No test depends on another's state or on execution order**, and no
  test cleans up after itself: the stack's database is thrown away
  wholesale, which is why it carries no volumes.
- **No shared mutable module-level state.** Where several tests need the
  same starting point, each builds its own from a helper.
- Browser contexts are per-test by default. Do not carry a signed-in
  session across tests unless a fixture makes that sharing explicit.

### Determinism

The `web/` rule — "never sleepy" — applies with more force here, because
this tier has real network and real containers to be tempted by.

- **Never sleep.** No `page.waitForTimeout`, and no bare timeout used as
  a synchronisation device. Playwright's assertions already retry.
- **Wait on the condition, not the clock**: `toHaveURL`, `toBeVisible`,
  `toHaveText`. For state that lives outside the page — a delivered
  email — `expect.poll` with a `message` naming what never arrived, so
  the timeout reports the missing thing instead of a bare expiry.
- **Interact only after hydration.** Navigate with `visit()` from
  [helpers/page.ts](helpers/page.ts) rather than `page.goto` whenever the
  test then drives the page: server-rendered markup accepts clicks before
  vue has attached anything to it, and those clicks do nothing.
- **Assert the exact URL when a parameter carries meaning.**
  `toHaveURL(".../login?verified=true")` distinguishes an activated
  account from a rejected token; a regex on the path alone would pass for
  both.
- **`documentPersisted()` is the one deliberate wait on the clock.**
  Hocuspocus stores a document two seconds after its last change and
  nothing on the client reports when that happened, so a test that
  reloads to prove persistence waits out the debounce. It is the single
  permitted `waitForTimeout` in the suite and carries its disable reason.
- **Three waits are raised above the default, all for the same
  reason**: the post-signup redirect, the post-login redirect and a cold
  document load. Each is a chain across auth-realtime, core and postgres
  — and in the document's case the websocket sync and a fade-in too —
  that several workers at once can stretch past five seconds while the
  product is entirely healthy. Any other raised timeout carries a
  comment saying what is slow and why.

### Failure artifacts

- `trace: "on-first-retry"` and `screenshot: "only-on-failure"`;
  `test-results/` and `playwright-report/` are gitignored.
- `docker compose logs` from this directory gives the service side of a
  failure — the browser only ever shows one end of it.
- A failure that reproduces only under load or only in CI is almost
  always a wait that was never really a wait. Fix the signal; do not
  raise the timeout.

### Prior art

A root-level, self-contained e2e package is the norm for repositories
shaped like this one:
[immich](https://github.com/immich-app/immich/tree/main/e2e) (`e2e/`
beside `server/` and `web/`, own package.json and compose file),
[grafana](https://github.com/grafana/grafana/tree/main/e2e-playwright),
[mattermost](https://github.com/mattermost/mattermost/tree/master/e2e-tests)
(beside `server/` and `webapp/`), and
[cal.com](https://github.com/calcom/cal.com/tree/main/apps/web/playwright).
None of them nest the suite under a `tests/` tier, and none share the
frontend's toolchain.

## Formatting & TS

The same gates as `web/`, minus the ones that only make sense there.

Prettier uses **tabs, no semicolons, trailing commas**, and
[.prettierignore](.prettierignore) keeps it off the lockfile and the
markdown — the convention the sibling packages already follow.

ESLint ([eslint.config.mjs](eslint.config.mjs)) runs **type-aware**
(`strictTypeChecked` + `stylisticTypeChecked` over a `projectService`),
for one reason above all: it is what reports a promise that was never
awaited. In a Playwright suite a missing `await` is not a style problem —
the assertion never runs, and the test passes without having checked
anything. It runs with `--max-warnings 0`, and `reportUnusedDisableDirectives`
makes a stale disable an error, so every `eslint-disable` states its
reason after `--` and is removed when it stops applying.

**`eslint-plugin-playwright` is this package's counterpart to `web/`'s
`@vitest/eslint-plugin`,** and it enforces conventions this file would
otherwise only ask for politely: `no-wait-for-timeout` is the "never
sleep" rule, `no-focused-test` and `no-skipped-test` stop a `.only` or a
`.skip` reaching main, and `missing-playwright-await` catches the
unawaited assertion described above. It is scoped to
[tests/](tests/) and [helpers/](helpers/).

**knip** guards dead exports, files and dependencies, and needs no config
file: its Playwright plugin picks up both `playwright.*.config.ts` files,
resolves `globalSetup` and `globalTeardown` from them, and treats the test
files as entry points. Nothing here has `web/`'s auto-import or vendored-directory
blind spots, so a `knip.ts` would only be a file to keep in sync.

TypeScript is strict, with `noUnusedLocals`, `noUnusedParameters`,
`noUncheckedIndexedAccess`, `noImplicitOverride` and
`verbatimModuleSyntax` on — the same posture as the other packages.

## CI

[.github/workflows/qa-e2e-dev.yml](../.github/workflows/qa-e2e-dev.yml) mirrors
the other QA workflows: a `lint` job running `check-lint`, and a `test`
job running `make e2e-dev`. The test job needs more than a node toolchain —
it sets up Go and goreleaser, because the suite runs against images built
from this repository and core's is not something compose can build. On
failure it uploads the html report, which is why the CI reporter is
`github` **and** `html`: annotations on the pull request, and a full
report with traces to download.

**The triggers are asymmetric on purpose.** A merge to `main` runs the
suite for a change anywhere that can break it — `web/`, `server/`,
`docker/`, `e2e/` — because that is where regressions actually come from.
A pull request only triggers it for changes to `e2e/` itself. A full run
builds a nuxt image and costs real minutes, and paying that on every push
of every frontend PR buys only a few minutes' warning over the run on
main. `concurrency` supersedes earlier runs on
every branch, `main` included — only the latest push's result matters,
so a superseded run shows as cancelled rather than finishing.

**Layer caching is what keeps that affordable.** A runner starts with no
layers, so the workflow sets up buildx and applies
[docker-compose.dev.ci.yaml](docker-compose.dev.ci.yaml) through
`E2E_DEV_COMPOSE_EXTRA` — a separate file because `type=gha` is meaningless
outside a runner and would break every local build. Paired with the
dependency layer in `web/Dockerfile.dev`, a push that leaves `web/`
alone reuses the whole image instead of rebuilding it.

**The prod suite is
[.github/workflows/qa-e2e-prod.yml](../.github/workflows/qa-e2e-prod.yml),
and it has no trigger of its own.** No push, no pull request, no tag:
[release.yml](../.github/workflows/release.yml) calls it, and refuses to
publish the image unless it passes. `workflow_dispatch` remains for
re-verifying an image without cutting a release. Where the dev suite guards
day-to-day work, the prod suite answers a different question — does the
artifact about to be published serve the product, and is its trust boundary
intact — which is worth its minutes exactly when something is being
released. Its cache reaches the build through `PROD_BUILD_EXTRA` rather
than a compose override, because compose builds nothing in the prod stack:
the image comes from `docker build` in the Makefile. That is also why
[docker-compose.dev.ci.yaml](docker-compose.dev.ci.yaml) carries `dev` in
its name with no prod counterpart to pair with.

**Being a release gate is what `workflow_call` is for.** A job cannot
depend on a separate workflow's result, so release.yml is the one place
that runs the gates and holds the publish behind `needs` — and every
workflow it calls declares `workflow_call` alongside its own triggers. Path
filters do not apply to a called workflow, so each one runs in full on a
tag regardless of what that tag touched, which is what a release wants.
Nothing is reused from an earlier run of the same commit either: a called
workflow always executes, so a tag re-runs every gate against the tagged
tree.

**The dev suite is deliberately not a gate**, and `qa-e2e-dev.yml` is
therefore the one QA workflow that does not declare `workflow_call`. It
runs the same flow tests as the prod suite, against a different assembly of
the same product, while the prod suite runs them against the artifact
actually being published — so gating on both would spend a nuxt image build
and a browser run re-answering a question already answered about the thing
that ships.

## Not covered yet

The desktop/Electron build. Playwright drives it through a different
entry point (`_electron`) and its auth runs over IPC rather than cookies,
so it is a separate track with its own launch mechanics — not a variant
of these tests.
