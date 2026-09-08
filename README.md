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

## AI assistant

The assistant is disabled by default: with `ASSISTANT_PROVIDER` empty the
server boots without a model and the in-app chat is unavailable (the MCP
server below keeps working). It is not tied to any one vendor — enable it
by picking a provider in `docker/env/core.local.env`; the model is
optional and defaults to the provider's strongest supported one:

```sh
OXYNOTE_CORE_ASSISTANT_PROVIDER=anthropic
OXYNOTE_CORE_ASSISTANT_MODEL=claude-opus-4-6
OXYNOTE_CORE_ASSISTANT_API_KEY=sk-...
```

Switching providers is an env change and a restart — no rebuild.

| provider | credentials | supported models |
| --- | --- | --- |
| `anthropic` | `ASSISTANT_API_KEY`, or `ASSISTANT_BEDROCK_*` / `ASSISTANT_VERTEX_*` | `claude-fable-5`, `claude-opus-5` (default), `claude-opus-4-6`, `claude-sonnet-5`, `claude-sonnet-4-6` |
| `openai` | `ASSISTANT_API_KEY` (+ `ASSISTANT_AZURE_API_VERSION` for Azure) | `gpt-5.1` (default), `gpt-5`, `gpt-5-mini` |
| `google` | `ASSISTANT_API_KEY` | `gemini-3-pro-preview`, `gemini-2.5-pro` (default), `gemini-2.5-flash` |
| `ollama` | none; set `ASSISTANT_BASE_URL` to your server | `llama3.3:70b` (default), `qwen3:32b` |
| `openrouter` | `ASSISTANT_API_KEY` | the models above behind their vendor prefix (`anthropic/claude-opus-5` (default), `openai/gpt-5`, `google/gemini-2.5-pro`, …) |

`ASSISTANT_BASE_URL` also points the `openai` provider at anything that
speaks the OpenAI chat-completions protocol — a local inference server, a
gateway, or a hosted compatibility layer.

**The model must be good at tool calling.** The assistant does everything
through tools, against a large document schema, so a model that calls them
unreliably looks broken rather than merely weaker. The server therefore
judges the configured model at boot against the supported list above: a
frontier model runs the assistant at full strength; a mid-tier model (and
everything on ollama) runs it with a logged warning about its
limitations; a small model, or any model not on the list, disables the
assistant with a warning instead of failing boot. The verdict is reported
by `GET /core/api/capabilities` as `aiAssistant.status` — `active`,
`active-but-weak`, `inactive-too-weak`, or `inactive` — alongside the
configured model, so the frontend can explain a missing chat entry point.

`ASSISTANT_SUMMARY_MODEL` optionally points conversation summarisation at
a cheaper model; it defaults to the chat model. See
`docker/env/core.example.env` for the full list of assistant variables.

## MCP server

Oxynote is also a remote [MCP](https://modelcontextprotocol.io) server:
Claude Code, Claude Desktop and any other MCP client can read and edit
documents with the same tools the in-app assistant uses, plus browse
documents as resources. The endpoint is served by core over streamable
HTTP at `/core/api/mcp` behind the front door and is protected by OAuth
2.1 — auth-realtime is the authorization server, clients register
dynamically, and the user approves each client on a consent page. Tokens
are bound to the user's organization and carry `documents:read` /
`documents:write` / `data-sources:read` scopes; write tools run without a
server-side confirmation step, so approval is the client's job
(destructive tools are annotated as such).

```sh
claude mcp add --transport http oxynote http://localhost:8080/core/api/mcp
```

Then authenticate from Claude Code's `/mcp` menu; the browser OAuth flow
completes against the dev stack. Connected clients can be reviewed and
revoked in the app's settings.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for running Oxynote from source.

## License

[Apache 2.0](LICENSE)
