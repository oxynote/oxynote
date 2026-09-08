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

## Before opening a pull request

Run `make check-lint`. CI runs the same gates per component, plus the unit
tests and both end-to-end suites.
