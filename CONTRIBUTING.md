# Contributing

This guide takes you from a fresh clone to a running dev stack, then covers
what a pull request needs before it can merge. Oxynote is a web app built
with Nuxt and Vue, a Go API server and a Node service for sign-in and
real-time editing, all driven from the repository root with `make`.

- [Quick Start](#quick-start)
- [Repository Layout](#repository-layout)
- [Commands](#commands)
- [Tests and Lints](#tests-and-lints)
- [Code Standards and AGENTS.md](#code-standards-and-agentsmd)
- [Pull Requests](#pull-requests)
- [Adding a Data Source](#adding-a-data-source)
- [Adding a Freshness Hook](#adding-a-freshness-hook)

## Quick Start

Install these first:

| Tool | Used for |
| --- | --- |
| [Docker](https://www.docker.com) with Compose | the dev stack and the Go database tests |
| [Go](https://go.dev) 1.27+ | `server/core` and `datagen` |
| [Node.js](https://nodejs.org) 24+ | `web`, `server/auth-realtime`, `e2e` and the production launcher |
| [pnpm](https://pnpm.io) | the Node packages. Any recent version works, because pnpm switches to the one each package pins. |
| [make](https://www.gnu.org/software/make/) | every setup, build and test command |
| [goreleaser](https://goreleaser.com) | the Go binaries and images that each stack runs |
| [golangci-lint](https://golangci-lint.run) | the Go lints |
| [moq](https://github.com/matryer/moq) | only to regenerate the Go test mocks with `go generate` |

Then, from the repository root:

```sh
make setup   # install dependencies and create the local env files
make start   # build and start the dev stack in the background
```

Open http://localhost:8080 and sign up. The verification email lands in
Mailpit, the dev inbox at http://localhost:8084. `make stop` stops the
stack.

For frontend work, `make dev` runs the backend in containers and the web
app on http://localhost:3000, which reloads as you save. If `make start`
refuses to run after a pull because your env files no longer match the
templates, run `make sync-env`. It keeps your values.

## Repository Layout

- `web/`: the frontend, built with Nuxt 4 and Vue 3. It ships as a web app
  and an Electron desktop app.
- `server/core/`: the Go API server, with storage, search, integrations and
  the AI assistant.
- `server/auth-realtime/`: a Node service that runs sign-in
  ([Better Auth](https://github.com/better-auth/better-auth)) and real-time
  editing ([Hocuspocus](https://github.com/ueberdosis/hocuspocus)).
- `e2e/`: the Playwright end-to-end tests and the stacks they run against.
- `docker/`: the dev stack and its env templates. `docker/prod/` builds the
  production image.
- `datagen/`: a Go program that fills the demo databases.
- `scripts/`: helpers the root Makefile calls.

## Commands

The root `Makefile` runs everything from the repository root, and a comment
above each target says what it does. Inside a component, the commands live
in its `package.json` scripts or, for `server/core/` and `datagen/`, in its
own `Makefile`. Each component's `AGENTS.md` lists them too. These are the
ones you'll use most:

```sh
make dev         # backend in containers, web app with hot reload on :3000
make lint        # fix lint, format and type issues in every component
make test        # unit tests and coverage gates in every component
make e2e-dev     # end-to-end tests against a fresh dev stack
make prod-run    # build the production image from source and run it on :8080
make check-all   # lints, unit tests and end-to-end tests, should be executed before each commit
```

## Tests and Lints

`make check-all` must pass before each commit. It runs every lint and type
check, the unit tests with their coverage gates, and the end-to-end tests
against the production image. CI runs the same checks on each pull
request, plus the end-to-end tests against the dev stack.

Oxynote has over 14k unit, browser and integration tests: about 6.6k in
the web app, 6.7k in the Go server and 600 in auth-realtime. On top of
those, nearly 200 end-to-end tests drive a real browser through a running
stack. Line coverage is about 90% in the web app and 93 to 99% everywhere
else. Each component has a minimum, and CI fails a change that drops below
it.

The tests are what let us change things quickly. When a change breaks
something in a part of the code you didn't touch, a test fails on your
machine within minutes. Each test name also reads as a sentence about what
the code does, so the suite doubles as a description of its behavior.

Coverage on its own proves little, since a test can run a line without
checking what it does. We keep it high for two reasons. A test for a branch
has to set up the exact condition that reaches it, which confirms we know
when and why that branch runs. And with most of the code covered, the gaps
are easy to read: what's left is code that tests can't reach conveniently,
rare edge cases and experimental features. A branch that no test can reach,
such as code that runs only in the desktop build, carries a `// NOCOV:`
comment that says why.

Lints and type checks settle formatting and naming before review, so
reviews can focus on behavior. They also catch whole classes of bugs, such
as an unchecked error or a value that may be undefined, before a test runs.

## Code Standards and AGENTS.md

Each pull request meets the same bar, whether you wrote the code yourself
or with an AI agent. An agent writes plausible code quickly, and the tests
and conventions are what show whether it's right. Either way, you're
responsible for each line you submit and should be able to explain it in
review.

Our conventions live in `AGENTS.md` files: one at the repository root, one
in each component and a few in packages with rules of their own. They cover
code style, test layout, naming and the traps in each part of the code.
Coding agents read these files on their own, but they're the style guide
for people too. Read the root file and the one for the component you're
changing. The `CLAUDE.md` beside each one only points Claude Code to it, so
edit the `AGENTS.md`.

We distilled these rules over many years of work on projects such as
[ttlcache](https://github.com/jellydator/ttlcache), mostly long before AI
coding tools existed. A lot of them build on existing guides, such as the
[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).
Others we found by reading the Go standard library. Our table tests with
check functions, for example, came from its tests, such as
[`TestRecorder`](https://github.com/golang/go/blob/go1.27.1/src/net/http/httptest/recorder_test.go#L16).
Our
[`Test_Handler_FetchNotificationsCount`](server/core/internal/server/internal/notification/http_test.go#L154)
follows the same pattern.

## Pull Requests

- Bug fixes and small changes can go straight to a pull request.
- New features and UI changes start with an issue. Agree on the approach
  there first, with design mockups or screenshots for anything visual, so
  you don't build something that can't be merged.
- A pull request that changes the UI should show the result in screenshots or a
  short video.
- Keep to one issue per pull request, and fill in the template.
- Report a security vulnerability privately, as [SECURITY.md](SECURITY.md)
  describes.

## Adding a Data Source

The quickest way to add a data source is to copy the existing type closest
to yours, such as MySQL for an SQL database. This lists each file that
mentions it, tests included:

```sh
git grep -il mysql
```

Add the new type next to MySQL in each of those files, except the mocks
under `_mock/`. Those are generated, so run `go generate` in
`server/core/internal/datasource/` instead. A few parts need more than a
copy:

1. Write a processor in `server/core/internal/datasource/processor/`. It
   connects to the source, runs queries and shapes the results for charts.
   A source that speaks an existing protocol reuses that processor, the way
   MariaDB uses the MySQL one.
2. Make the processor's connection test check that the credentials can't
   write, because Oxynote only reads from a data source.
3. If the source is a database, start it in a container in the processor
   tests' `TestMain`. If those tests need a new tool, add it to the
   prerequisites at the top of this guide.
4. After you change the assistant tools, regenerate the golden file their
   descriptions are compared against, and review the diff:

   ```sh
   cd server/core
   UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/
   ```

## Adding a Freshness Hook

A freshness hook works the same way. Copy the scheduled reminder, the
simplest type, and add the new type next to it in each file this lists.
The `.?` matches each spelling, such as `scheduled-reminder` and
`ScheduledReminder`.

```sh
git grep -ilE 'scheduled.?reminder'
```

Skip the database migrations in that list, since a migration never changes
once it has run. Then:

1. Write a processor in `server/core/internal/document/hook/processor/`.
   Its struct holds the hook's settings. From them it decides when the
   hook is triggered, and it keeps the hook's state.
2. If the hook needs an outside service, the way GitHub tracking needs the
   GitHub app, it stays hidden and skipped until that service is set up.
   Add a client for the service under `server/core/internal/apps/`, then
   copy how GitHub tracking is switched off in the hook manager, the
   assistant tools, the capabilities endpoint and the web app's hook menu.
3. Regenerate the assistant's golden file, as in step 4 of
   [Adding a Data Source](#adding-a-data-source).
