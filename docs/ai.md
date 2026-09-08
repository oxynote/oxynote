# AI assistant and MCP server

Both features run inside the [production image](../docker/prod/README.md)
and are configured through its `OXYNOTE_*` variables. The assistant is the
in-app chat; the MCP server lets external clients such as Claude Code use
the same tools against your documents. The MCP server is always on and does
not need the assistant.

## AI assistant

The assistant is disabled until `OXYNOTE_AI_ASSISTANT_PROVIDER` is set.
Setting any other `OXYNOTE_AI_ASSISTANT_*` variable without it fails the
boot. The assistant is not tied to one vendor: pick a provider, supply its
credentials and optionally a model, which defaults to the provider's
strongest supported one.

```sh
OXYNOTE_AI_ASSISTANT_PROVIDER=anthropic
OXYNOTE_AI_ASSISTANT_MODEL=claude-opus-4-6
OXYNOTE_AI_ASSISTANT_API_KEY=sk-...
```

Switching providers is an env change and a container restart.

| provider | credentials | supported models |
| --- | --- | --- |
| `anthropic` | `OXYNOTE_AI_ASSISTANT_API_KEY`, or `OXYNOTE_AI_ASSISTANT_BEDROCK_*` / `OXYNOTE_AI_ASSISTANT_VERTEX_*` | `claude-fable-5`, `claude-opus-5` (default), `claude-opus-4-6`, `claude-sonnet-5`, `claude-sonnet-4-6` |
| `openai` | `OXYNOTE_AI_ASSISTANT_API_KEY` (+ `OXYNOTE_AI_ASSISTANT_AZURE_API_VERSION` for Azure OpenAI) | `gpt-5.1` (default), `gpt-5`, `gpt-5-mini` |
| `google` | `OXYNOTE_AI_ASSISTANT_API_KEY` | `gemini-3-pro-preview`, `gemini-2.5-pro` (default), `gemini-2.5-flash` |
| `ollama` | none; set `OXYNOTE_AI_ASSISTANT_BASE_URL` to your server | `llama3.3:70b` (default), `qwen3:32b` |
| `openrouter` | `OXYNOTE_AI_ASSISTANT_API_KEY` | the models above behind their vendor prefix (`anthropic/claude-opus-5` (default), `openai/gpt-5`, `google/gemini-2.5-pro`, …) |

`OXYNOTE_AI_ASSISTANT_BASE_URL` also points the `openai` provider at
anything that speaks the OpenAI chat-completions protocol: a local
inference server, a gateway, or a hosted compatibility layer.

**The model must be good at tool calling.** The assistant does everything
through tools, against a large document schema, so a model that calls them
unreliably looks broken rather than merely weaker. The configured model is
judged at boot against the supported list above: a frontier model runs the
assistant at full strength; a mid-tier model (`claude-sonnet-*`,
`gpt-5-mini`, `gemini-2.5-flash`, and everything on ollama) runs it with a
logged warning about its limitations; any other model disables the
assistant with a warning instead of failing boot. The verdict is exposed by
`GET /core/api/capabilities` as `aiAssistant.status` — `active`,
`active-but-weak`, `inactive-too-weak`, or `inactive` — alongside the
configured model, which is how the app explains a missing chat entry point.

### All variables

| variable | meaning |
| --- | --- |
| `OXYNOTE_AI_ASSISTANT_PROVIDER` | `anthropic`, `openai`, `google`, `ollama` or `openrouter`. Unset disables the assistant. |
| `OXYNOTE_AI_ASSISTANT_MODEL` | model id from the table above; defaults to the provider's default. |
| `OXYNOTE_AI_ASSISTANT_API_KEY` | the vendor's API key. |
| `OXYNOTE_AI_ASSISTANT_BASE_URL` | overrides the provider endpoint. Required for `ollama`; with `openai` it reaches any OpenAI-compatible server. |
| `OXYNOTE_AI_ASSISTANT_MAX_TOKENS` | caps one reply. Default `64000`. |
| `OXYNOTE_AI_ASSISTANT_REQUEST_TIMEOUT` | bounds one request to the provider, as a Go duration. Default `10m`. |
| `OXYNOTE_AI_ASSISTANT_SUMMARY_MODEL` | model used to summarise long conversations, so that mechanical work can run on a cheaper model. Defaults to `_MODEL`. |
| `OXYNOTE_AI_ASSISTANT_AZURE_API_VERSION` | `openai` only: set to target Azure OpenAI. |
| `OXYNOTE_AI_ASSISTANT_BEDROCK_REGION`, `_BEDROCK_ACCESS_KEY`, `_BEDROCK_SECRET_ACCESS_KEY`, `_BEDROCK_SESSION_TOKEN` | `anthropic` only: set the region to reach Claude via AWS Bedrock. Leave the keys empty to use the ambient AWS credential chain. |
| `OXYNOTE_AI_ASSISTANT_VERTEX_PROJECT_ID`, `_VERTEX_REGION`, `_VERTEX_SERVICE_ACCOUNT_JSON` | `anthropic` only: set the project id to reach Claude via Google Vertex. Leave the service account empty to use application default credentials. |

Conversations are held in Valkey when `OXYNOTE_VALKEY_DSN` is set and in
process otherwise, where a restart discards them.

## MCP server

Oxynote is also a remote [MCP](https://modelcontextprotocol.io) server:
Claude Code, Claude Desktop and any other MCP client can read and edit
documents with the same tools the in-app assistant uses, plus browse
documents as resources. The endpoint is served over streamable HTTP at
`<OXYNOTE_PUBLIC_URL>/core/api/mcp` and is protected by OAuth 2.1: clients
register dynamically and the user approves each client on a consent page.
Tokens are bound to the user's organization and carry `documents:read`,
`documents:write` and `data-sources:read` scopes. Write tools run without a
server-side confirmation step, so approval is the client's job; destructive
tools are annotated as such.

```sh
claude mcp add --transport http oxynote https://oxynote.example.com/core/api/mcp
```

Then authenticate from Claude Code's `/mcp` menu, which opens the browser
OAuth flow against your deployment. Connected clients can be reviewed and
revoked in the app's settings.
