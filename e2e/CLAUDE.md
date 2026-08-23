# CLAUDE.md

Guidance for the `e2e/` end-to-end suite. Shared working principles and
the TS/JS comment/whitespace rules live in the root
[CLAUDE.md](../CLAUDE.md). The testing standards below are the `web/`
standards ([web/CLAUDE.md](../web/CLAUDE.md)) carried over and adapted —
where a rule differs, it differs because the tier does, and the reason is
stated.

## Project

`@oxynote/e2e` — Playwright tests that drive the composed product through
a browser, plus the docker-compose stack they run against.

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
make e2e             # one-shot: build images, start the stack, run the tests, tear down
make e2e-stack-build # build the stack's images, for iterating
make e2e-stack-stop  # stop the stack and drop its data
```

From `e2e/`:

```sh
pnpm setup       # deps + the playwright chromium browser
pnpm test        # playwright test (brings the stack up first, see below)

pnpm check-lint  # check-types + check-eslint + check-fmt + check-knip
pnpm lint        # the fixing counterpart of all four
pnpm qa          # check-lint + test; qa-fix = lint + test

pnpm check-types # tsc --noEmit
pnpm check-eslint # eslint --max-warnings 0 . (eslint / fmt are the
pnpm check-fmt   #   fixing counterparts)
pnpm check-knip  # dead exports, files and dependencies
```

`pnpm test` runs [global-setup.ts](global-setup.ts), which brings the
stack up itself, so it works whether or not the stack is already running.
**What it never does is rebuild.** Compose builds `web` and
`auth-realtime` when their images are missing outright, but a source
change after that is invisible to it — refreshing them takes an
explicit build.
Core it cannot build at all: the service has no build section, so
`make build-go` is the only thing that produces that image. Teaching the
hook to build would drop goreleaser into the inner loop of every run.

That is the division of labour, and each verb has exactly one owner.
**Make only builds** — goreleaser for core, `compose build` for the two
images compose owns — and **the setup hook is the only thing that runs
`compose up`.** So: after changing backend or frontend source,
`make e2e-stack-build`; after changing only a test, `pnpm test`. Running
`pnpm test` against source you just edited silently tests the previous
build.

Both build steps run quietly, under a ticking line, and replay their
whole log if they fail. Docker and goreleaser say a great deal that
matters only when something breaks.

**The stack has no pnpm scripts of its own**, and should not grow any.
Starting it belongs to `global-setup.ts` and building to make; anything
else — tearing it down by hand, reading logs — is plain `docker compose`
run from this directory, which discovers `docker-compose.yaml` without
being told where it is. A `pnpm stack:*` wrapper would only be a second
name for a command that is already shorter than its alias.

## The stack

[docker-compose.yaml](docker-compose.yaml) is the whole backend, built
from this repository's own source — the same images and Dockerfiles the
dev stack uses, never a published release. It deliberately differs from
`docker/docker-compose.dev.yaml`:

- **No named volumes**, so every run starts from an empty database. This
  is what makes the suite deterministic; do not add persistence.
- **No `container_name`**, and its own ports (`:18080` front door,
  `:18025` mailpit), so it runs at the same time as the dev stack.
- **Env is inlined and committed.** There is nothing secret in it, so CI
  needs no setup step. Unlike `docker/env/*.example.env`, it lists only
  the variables that matter — it is a fixture, not an operator template.
- **Every image is pinned**, including the two the dev stack floats
  (`minio` and `mailpit`). A run has to fail because this repository
  changed, not because an upstream `latest` moved overnight.
- **Only the services the tests touch.** No changedetection,
  sockpuppet-chrome, mariadb, datagen, prometheus or grafana.
- **The front door is `docker/Caddyfile` verbatim**, mounted from the dev
  stack rather than copied. E2e must exercise the routing that ships,
  including the `/api/x` and `/api/internal` blocks; a second copy would
  drift.

Two knock-on constraints worth knowing:

- **Core boots with `ASSISTANT_PROVIDER=ollama`.** Core refuses to start
  without a valid provider and model, and ollama is the only one that
  needs no API key. Nothing dials it.
- **Core cannot have a compose healthcheck** — its image is distroless,
  with no shell or http client to probe itself with. `global-setup.ts`
  polls it through the front door instead, treating any response under
  500 as ready (core answers 401 to an unauthenticated probe; Caddy
  answers 502 on its behalf while it is still starting).

**Mailpit is what makes the auth flows testable end to end.** Email
verification is mandatory (`requireEmailVerification`), so tests fetch
the real message from mailpit's REST API and follow the real link —
nothing is stubbed or short-circuited.

## Testing standards

### Layout

- **Test files live in [tests/](tests/), one per user-facing flow**,
  named after the flow as a user would name it — `signup.test.ts`,
  `login.test.ts`. Never `.spec.ts`: the repo encodes meaning in the
  suffix and `.spec` is not one of them.
- There is no mirror of the app's source tree and nothing to pair 1:1
  with. `web/` co-locates because a test there has exactly one subject
  file; an e2e test has none — it crosses four services by design, so it
  is filed by flow instead.
- **Shared code lives in [helpers/](helpers/), one module per concern**
  (`auth`, `mailpit`, `i18n`, `page`, `config`). Helpers are setup and
  plumbing; they do not carry the assertion a test exists to make.
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
  wiring holds". Reaching for one more error-path e2e is nearly always
  the wrong instinct — add it as a component test instead.

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
  element, fix the locator — do not reach for `.first()`.

### Independence & concurrency

- **Every test creates its own account** via `newCredentials()`, which
  mints a unique address. Unique-per-test state is what makes
  `fullyParallel` safe against a single shared stack — the same
  independence rule `web/` states, enforced by construction rather than
  by mock resetting.
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
- Any timeout raised above the default carries a comment saying what is
  slow and why.

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
file: its Playwright plugin reads [playwright.config.ts](playwright.config.ts),
resolves `globalSetup` from it, and treats the test files as entry
points. Nothing here has `web/`'s auto-import or vendored-directory
blind spots, so a `knip.ts` would only be a file to keep in sync.

TypeScript is strict, with `noUnusedLocals`, `noUnusedParameters`,
`noUncheckedIndexedAccess`, `noImplicitOverride` and
`verbatimModuleSyntax` on — the same posture as the other packages.

## CI

[.github/workflows/qa-e2e.yml](../.github/workflows/qa-e2e.yml) mirrors
the other QA workflows: a `lint` job running `check-lint`, and a `test`
job running `make e2e`. The test job needs more than a node toolchain —
it sets up Go and goreleaser, because the suite runs against images built
from this repository and core's is not something compose can build. On
failure it uploads the html report, which is why the CI reporter is
`github` **and** `html`: annotations on the pull request, and a full
report with traces to download.

**The triggers are asymmetric on purpose.** A merge to `main` runs the
suite for a change anywhere that can break it — `web/`, `server/`,
`docker/`, `e2e/` — because that is where regressions actually come from.
A pull request only triggers it for changes to `e2e/` itself. A full run
builds a nuxt image and costs real minutes on a private repository, and
paying that on every push of every frontend PR buys only a few minutes'
warning over the run on main. `concurrency` supersedes a pull request's
earlier runs but never cancels one on `main`, whose result is the record
for the branch.

**Layer caching is what keeps that affordable.** A runner starts with no
layers, so the workflow sets up buildx and applies
[docker-compose.ci.yaml](docker-compose.ci.yaml) through
`E2E_COMPOSE_EXTRA` — a separate file because `type=gha` is meaningless
outside a runner and would break every local build. Paired with the
dependency layer in `web/Dockerfile.dev`, a push that leaves `web/`
alone reuses the whole image instead of rebuilding it.

## Not covered yet

The desktop/Electron build. Playwright drives it through a different
entry point (`_electron`) and its auth runs over IPC rather than cookies,
so it is a separate track with its own launch mechanics — not a variant
of these tests.
