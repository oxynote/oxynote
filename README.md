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

Requirements:

- [Docker](https://www.docker.com) — runs the dev stack (Postgres, Caddy, the
  app containers, etc.) via docker-compose; also a unit-test dependency for
  the Go database-layer tests, which start throwaway containers.
- [Go](https://go.dev) — builds the `server/core` API server and the
  `datagen` demo-data generator.
- [Node.js](https://nodejs.org) (with corepack for [pnpm](https://pnpm.io)) —
  builds and runs the `web` frontend and the `server/auth-realtime` service;
  pnpm is the package manager for both.
- [goreleaser](https://goreleaser.com) — produces the Go binaries and dev
  docker images (`make build` in `server/core` and `datagen`).
- [make](https://www.gnu.org/software/make/) — orchestrates all setup, build,
  and run commands.
- [golangci-lint](https://golangci-lint.run) — lints the Go code (part of the
  QA gates).
- [moq](https://github.com/matryer/moq) — generates the mocks used by Go
  tests (`go generate`).

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

## AI assistant

The assistant is not tied to any one vendor. Pick a provider and a model
with two environment variables in `docker/env/core.local.env`:

```sh
OXYNOTE_CORE_ASSISTANT_PROVIDER=claude
OXYNOTE_CORE_ASSISTANT_MODEL=claude-opus-4-6
OXYNOTE_CORE_ASSISTANT_API_KEY=sk-...
```

Switching providers is an env change and a restart — no rebuild.

| provider | credentials | known-good models |
| --- | --- | --- |
| `claude` | `ASSISTANT_API_KEY`, or `ASSISTANT_BEDROCK_*` / `ASSISTANT_VERTEX_*` | `claude-opus-4-6`, `claude-sonnet-4-6` |
| `openai` | `ASSISTANT_API_KEY` (+ `ASSISTANT_AZURE_API_VERSION` for Azure) | `gpt-5`, `gpt-5-mini` |
| `gemini` | `ASSISTANT_API_KEY` | `gemini-2.5-pro`, `gemini-2.5-flash` |
| `ollama` | none; set `ASSISTANT_BASE_URL` to your server | `llama3.3:70b`, `qwen3:32b` |
| `openrouter` | `ASSISTANT_API_KEY` | any tool-calling model OpenRouter offers |

`ASSISTANT_BASE_URL` also points the `openai` provider at anything that
speaks the OpenAI chat-completions protocol — a local inference server, a
gateway, or a hosted compatibility layer.

**The model must be good at tool calling.** The assistant does everything
through tools, against a large document schema, so a model that calls them
unreliably looks broken rather than merely weaker. That rules out most
small local models; prefer the sizes listed above. There is no automatic
check — an unsuitable model is a configuration mistake, not an error the
server can detect for you.

`ASSISTANT_SUMMARY_MODEL` optionally points conversation summarisation at
a cheaper model; it defaults to the chat model. See
`docker/env/core.example.env` for the full list of assistant variables.

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

- `8080` — front door (the app; APIs under `/auth-realtime` and `/core`)
- `8081` — MinIO console
- `8082` — changedetection.io
- `8083` — Grafana
- `8084` — Mailpit (dev email inbox)

Common per-component commands:

```sh
# web
make dev                            # backend containers + nuxt dev on :3000 (hot reload)
cd web && pnpm start:dev:web        # nuxt dev server only, on :3000
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
