# Oxynote

**The brains behind your team's technical operations.**

Oxynote is a collaborative platform where runbooks, API docs, and live
metrics live in one place — think Notion's editing experience crossed with
Grafana's view of your systems, with more of the ops stack on the roadmap.

- **Runbooks that stay alive** — real-time collaborative documents with
  branches, reviews, and merging, so operational knowledge is maintained
  like code, not lost in a wiki.
- **API docs, Stripe-style** — a block-based editor (code, diagrams,
  callouts, split documentation) built for polished, structured reference
  pages.
- **Live data where you read** — embed Prometheus metrics and SQL queries
  (PostgreSQL, MySQL/MariaDB) directly in documents, watch URLs and
  container images, track GitHub activity, and ask the built-in AI
  assistant.

## Quickstart

Requirements: Docker, Go, Node.js (with corepack for pnpm), goreleaser, make.

```sh
make setup     # install dependencies + create local env files from templates
make start     # build all images + run the dev stack in the background
```

Then open http://localhost:8080. Use `make run` instead of `make start` to run
in the foreground with logs (ctrl-c stops it), and `make stop` to stop the
background stack.

All configuration lives in `docker/env/*.local.env` (created by `make setup`,
gitignored). The defaults work out of the box; integrations (GitHub app,
Slack app, AI assistant, email) are optional and stay disabled until their
variables are set.

## Repository layout

- `web/` — Nuxt 4 + Vue 3 frontend; ships as both a web app (SSR) and an
  Electron desktop app. pnpm workspace.
- `server/core/` — Go API server: storage, search, integrations, AI
  pipelines.
- `server/auth-realtime/` — Node service running Better Auth and a
  Hocuspocus (Yjs) real-time server.
- `datagen/` — demo-data generator (Go); demo/testing only.
- `docker/` — dev docker-compose stack, Caddyfile, env templates, demo-data
  configs.

## Development

The dev stack exposes:

- `8080` — front door (the app; APIs under `/api` and `/go`)
- `8081` — MinIO console
- `8082` — changedetection.io
- `8083` — Grafana

Common per-component commands:

```sh
# web
cd web && pnpm start:dev:web        # nuxt dev server on :3000
cd web && pnpm qa                   # check-types + check-lint + check-fmt

# server/core
cd server/core && go test ./...     # needs Docker for the db package tests
cd server/core && make build        # goreleaser snapshot: binaries + images

# server/auth-realtime
cd server/auth-realtime && pnpm run qa
```

See `server/README.md` for backend details.

## License

[Apache 2.0](LICENSE)
