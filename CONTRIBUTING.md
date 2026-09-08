# Contributing

This guide covers running Oxynote from a source checkout and the commands
you need day to day. Everything is driven from the repository root through
`make`; the component-specific docs are linked at the end.

## Prerequisites

| Tool | Used for |
| --- | --- |
| [Docker](https://www.docker.com) with Compose | the dev stack (Postgres, Caddy, the app containers, …) and the Go database-layer tests, which start throwaway containers |
| [Go](https://go.dev) 1.27+ | `server/core` and `datagen` |
| [Node.js](https://nodejs.org) 24+ | `web`, `server/auth-realtime`, `e2e` and the production launcher |
| [pnpm](https://pnpm.io) | package manager for the Node packages. Any recent install works: pnpm switches to the version pinned in each package's `packageManager` field on its own |
| [make](https://www.gnu.org/software/make/) | orchestrates every setup, build and run command |
| [goreleaser](https://goreleaser.com) | builds the Go binaries and their docker images — needed by every target that starts a stack |
| [golangci-lint](https://golangci-lint.run) | lints the Go code (`make lint`, `make check-lint`) |
| [moq](https://github.com/matryer/moq) | only to regenerate the Go test mocks with `go generate`; the generated files are committed |

## First run

```sh
make setup     # install dependencies + create the local env files from templates
make start     # build all images + run the dev stack in the background
```

Then open http://localhost:8080 and sign up. `make stop` stops the stack.

## Everyday commands

### Running the dev stack

```sh
make run       # foreground: streams all logs, ctrl-c stops the stack
make start     # the same, in the background
make stop      # stop the background stack
make dev       # backend containers + the web dev server on the host
```

`make dev` is the frontend loop: the backend runs in containers and Nuxt
runs on the host with hot reload on http://localhost:3000. Ctrl-c stops the
dev server; `make stop` stops the containers.

All three targets rebuild the images before starting, so a code change is
picked up by simply running them again.

### Environment files

The dev stack reads `docker/env/*.local.env`, which `make setup` creates
from the committed `*.example.env` templates. The files are gitignored and
the defaults work out of the box. Integrations (GitHub app, Slack app, AI
assistant, email, changedetection.io, Meilisearch) are optional and stay
disabled until their variables are set; the frontend hides features the
deployment does not offer.

```sh
make check-env   # report variables that drifted between the templates and your local files
make sync-env    # rewrite the local files from the templates, keeping your values
make setup-force # setup, but the local files are recreated from the templates, values and all
```

`make setup` only copies a template when the local file is missing, so a
variable renamed or added in a template never reaches an existing local
file. The dev stack targets run `make check-env` first and refuse to start
on drift — `make sync-env` fixes it.

### Linting

```sh
make lint        # fix lint, format and type issues in every component
make check-lint  # the same gates, verification only (what CI runs)
```

### End-to-end tests

```sh
make e2e-dev     # build the dev stack, run the Playwright suite, tear it down
make e2e-prod    # the same suite against the production all-in-one image
```

Both are one-shot: every run builds the stack and drops it afterwards. To
iterate on tests, build the stack once with `make e2e-dev-stack-build` (or
`make e2e-prod-stack-build`), then run `pnpm test:dev` (or `pnpm test:prod`)
from `e2e/` as often as you like, and finish with
`make e2e-dev-stack-stop` (or `make e2e-prod-stack-stop`). The e2e stacks
use their own ports and throwaway state, so they run alongside the dev stack.

### Production image

```sh
make prod-build  # build the all-in-one image
make prod-run    # build it, then run the example deployment on :8080 (ctrl-c stops it)
make prod-stop   # stop it
```

`make prod-run` also starts a Mailpit on http://localhost:8025 so the
signup verification mail is readable. Stop the dev stack first — both listen
on `:8080`.

## Ports

| Port | Dev stack |
| --- | --- |
| `8080` | front door: the app, with the APIs under `/core` and `/auth-realtime` |
| `8082` | changedetection.io |
| `8083` | Grafana |
| `8084` | Mailpit (dev email inbox) |
| `3000` | the web dev server, only with `make dev` |

The e2e dev stack listens on `18080` (Mailpit on `18025`) and the e2e prod
stack on `19080` (`19025`).

## Repository layout

- `web/` — Nuxt 4 + Vue 3 frontend; ships as both a web app (SSR) and an
  Electron desktop app. See [web/README.md](web/README.md).
- `server/core/` — Go API server: storage, search, integrations, AI
  pipelines. See [server/README.md](server/README.md).
- `server/auth-realtime/` — Node service running Better Auth and a
  Hocuspocus (Yjs) real-time server.
- `datagen/` — demo-data generator (Go); demo/testing only.
- `e2e/` — Playwright end-to-end suite and the docker-compose stacks it
  drives.
- `docker/` — dev docker-compose stack, Caddyfile, env templates, demo-data
  configs.
- `docker/prod/` — the production all-in-one image and its example
  deployment. See [docker/prod/README.md](docker/prod/README.md).
- `scripts/` — helpers the root Makefile calls.

## Adding a data source

Decide first what you are adding. If the new type is compatible with an
existing processor, as MariaDB is with the MySQL one, it needs no
processor of its own, only the wiring in the steps below. Otherwise it
needs its own processor, client interface, routes and assistant tools.

1. **Create the type constant.** The type constant, the credential rules
   and the runner that opens a client for a stored data source all live
   in `server/core/internal/datasource/`. Add the constant, then follow
   an existing type through the package: the credential check, the
   runner's per-type construction and the runner method that returns the
   client, which refuses types it does not accept. A type with its own
   processor also gets its client interface there, with a mock
   regenerated by `go generate` (moq is in the prerequisites).
2. **Write the processor.** `server/core/internal/datasource/processor/`
   holds one processor per kind of source. It connects, tests the
   connection, including that the credentials cannot write (every data
   source is read-only), maps errors to a connection status, runs
   queries and shapes their results. What a new source needs ranges
   from a set of macro definitions to a whole client; reuse what the
   existing processors share, such as time-range handling, the `$__`
   macros, SQL metadata and result transformation, and add only what the
   new source does differently.
3. **Add the assistant tools.** Make sure every action the data source
   offers is also an assistant tool in
   `server/core/internal/assistant/tools/`; the MCP server exposes the
   same tools.
   - If metric blocks draw from the new type, add a case to the query
     handler in `server/core/internal/server/internal/datasource/` and
     to the simulation checker in
     `server/core/internal/datasource/simulation/`, which decides when a
     metric block stops showing generated data; without a case it never
     does.
4. **Add the type to the web app.** The frontend knows the types through
   the enum in `web/app/utils/api/data-source/`. From there, follow an
   existing type through `web/app/components/`: the status icon, the
   settings section that lists connectable sources (without it the type
   cannot be added) and the read-only warning in the connect dialog.
   Their strings live in `web/i18n/locales/`. The help and warning links
   are runtime config, declared in `web/nuxt.config.ts` and
   `web/index.d.ts` and set in `docker/env/web.example.env` and the
   production launcher's env mapping under `docker/prod/launcher/`.
   - If metric blocks draw from the new type, add the query editor's
     language support and the query help popover under
     `editor/blocks/metrics/config-modal/`.
5. **Add demo data (optional).** `datagen/` fills the demo databases and
   `docker/` runs them. Add a generator package and a block in the
   datagen command that runs it when its env variables are set, a
   compose service with its init script under `docker/demo/`, then
   `make sync-env`.
6. **Extend the tests and docs.** Every place above that switches on the
   type has a table-driven test with one row per type; extend the tables
   in the packages you touched, and the unit test beside each web
   component. The processor tests start a throwaway container per
   database, so a new one needs a container in their `TestMain` and
   Docker to run. Assistant tool descriptions are compared against a
   golden file; regenerate it from `server/core/` with
   `UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/`
   and review the diff. Update the supported-sources line in `README.md`,
   and the prerequisites above if a new container became a test
   dependency. The e2e suite creates no data sources and needs nothing.

## Adding a hook

Decide first whether the hook depends on an integration the deployment
may not have, the way GitHub tracking needs the GitHub app and the
website watcher needs changedetection.io. Such a hook also needs a
client under `server/core/internal/apps/`, a capability flag and the
gating in step 3.

1. **Create the type constant.** The type constant, its validation and
   its human-readable name (used in notification text), and the switch
   that turns a stored hook into a running processor all live in
   `server/core/internal/document/hook/`. Add the constant, then follow
   an existing type through the package. A hook with its own client gets
   it through the input that hands processors their clients, declared
   in the processor package and implemented in this one.
2. **Write the processor.** `server/core/internal/document/hook/processor/`
   holds one processor per type. Its struct is the hook's settings, and
   it computes a freshness score and a state from them, resets that
   state, and tears down whatever it holds outside the deployment.
3. **Gate on the integration.** If the hook depends on an integration,
   three places refuse or skip it when the deployment lacks it: the
   manager in `server/core/internal/document/hook/manager/` skips it
   while processing, the assistant's create refuses it, and the
   capabilities endpoint in `server/core/internal/server/` reports
   whether it is configured, so the web app can hide it.
4. **Add the assistant tools.** The hook tools in
   `server/core/internal/assistant/tools/` take settings as one object
   whose shape the type decides: add the type's schema variant and its
   decoding case.
5. **Add the type to the web app.** The frontend knows the types, their
   settings and their state through `web/app/utils/api/document.ts`.
   Add a directory with the config menu under
   `web/app/components/editor/hooks/`, then follow an existing type
   through the menu content beside it (the component and props switches
   and the add-menu entry, gated on the capability when the hook depends
   on one) and through the notification box in `web/app/components/`.
   Their strings live in `web/i18n/locales/`.
6. **Extend the tests.** Every place above that switches on the type has
   a table-driven test with one row per type; extend the tables in the
   packages you touched, and the unit test beside each web component.
   Assistant tool descriptions are compared against a golden file;
   regenerate it from `server/core/` with
   `UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/`
   and review the diff. The e2e suite creates no hooks and needs nothing.

## Before opening a pull request

Run `make check-lint`. CI runs the same gates per component, plus the unit
tests and both end-to-end suites.
