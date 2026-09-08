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

## AI assistant and MCP server

Oxynote has a built-in AI assistant (works with models from Anthropic, OpenAI, Google, Ollama or
OpenRouter) and an MCP server that lets Claude Code and other MCP clients
work with your documents. Setup: [docs/ai.md](docs/ai.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for running Oxynote from source.

## License

[Apache 2.0](LICENSE)
