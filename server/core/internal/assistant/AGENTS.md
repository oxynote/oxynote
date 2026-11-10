# AGENTS.md

The assistant: system prompt and tools. Go standards live in
[server/core/AGENTS.md](../../AGENTS.md), architecture in
[server/AGENTS.md](../../../AGENTS.md).

## Prompt

`prompt.go` assembles the system prompt from section constants. Workflow,
content, editing and style ship to both the chat model and MCP clients;
surface-specific rules (confirmation flow, persona) live in that surface's
section. `MCPInstructions` stays under 2 KB, the part Claude Code shows.

Writing a rule: state the principle and its reason in one or two sentences,
no edge-case lists or numbered steps; say what to do rather than what to
avoid; per-tool facts go in the tool description, the prompt carries only
cross-tool workflow; examples only where structure is needed, wrapped in
`<example>`; plain language, no caps or MUST; no em dashes and British
spelling (`prompt_test.go` and `tools_test.go` enforce both).

## Tools

`tools/`, grouped by subject: `document.go`, `block.go`, `tag.go`,
`variable.go`, `hook.go`, `search.go`, `datasource.go`; `eino.go` holds
the framework adapter and `read_tool_output`. `tools.Set` is the only list of tools;
adding one means its type plus a `register[T]()` line there, and the MCP
server picks it up for free.

- **Tools implement this package's `Tool`** (`Info`, `Validate`, `Exec`),
  never eino's interfaces; only `eino.go` imports eino. A read may add `Title`
  for its status line; every write adds `Summary`, which is also its
  status line. Other surfaces use `Entry.Info` to describe and
  `Entry.Tool.Run` to run.
- **Writes notify on `input`**: the method that changes the tree or a tag
  announces it, so a tool never does. Hook and variable writes go through
  `HookManager` and `VariableManager`, which announce the change as for
  the HTTP handler (hooks also record history); `decodeHookSettings`
  switches on type into the processor's own struct.
- **Content is markup** (`markup/`): `get_document` renders XML with ids,
  writes `markup.Build` it against the stored tree (a kept id keeps its
  uid, unchanged content keeps the stored node) and report from the build,
  never a re-read.
- **Branches are addressed by id, always.** Every content tool requires
  `branch_id`; listings carry the ids; `FetchDocumentByBranchID` refuses a
  branch of another document; an unknown id is refused naming the
  document's branches (`ErrUnknownBranch`); a protected branch reads but
  refuses writes, naming the unprotected ones. No tool creates, renames,
  deletes or merges a branch. MCP resources are
  `oxynote://documents/{id}/branches/{branch_id}`.
- **Every error names the next step**: what was wrong, then the tool or
  argument that fixes it (`edit.Result.Err` rewrites the realtime
  service's errors; `ErrUnknownDocument`, `_errUnknownBlock`,
  `_errBranchProtected`, `_errUnknownParent` follow suit).
- **A tool's fields are its arguments.** The registry decodes each call
  into the tool (json/v2 names the bad path) and runs `Validate`
  (`errRequired(key)`) before any other method. An empty string is
  invalid, never absent. `Exec` returns a value the adapter serialises.
- `Title`/`Summary` fetch what they name and use the row's name; an
  unresolvable target is no title, or an error from `Summary`. Tools reach
  everything through `Input`, a facade of per-subject parts at the bottom
  of the file using them, never `Deps`. Tool tests stub `InputMock`;
  `Test_input_*` covers the database. One call's steps share a branch
  fetch.
- **A write owns its invariants** on the `input` method that performs it
  (`MoveDocument` refuses a missing parent or a move under its own subtree),
  not in `Exec`. Block writes go through `ApplyEdit`. What core cannot
  check, placement and content order, is auth-realtime's, against the
  editor schema (`validate.ts`).
- **`Info.Traits` says what a tool is**; the zero value is a plain read.
  `Write` gates behind confirmation and protects the result from context
  middlewares (only writes have a `Summary`; `Test_New` checks).
  `Destructive` keeps it outside "approve all" (every `delete_*`).
  `Overwrites` means content the caller did not name is replaced
  (`replace_blocks`). `DataSource` reads an outbound
  connection (own MCP scope, open-world). `Internal` keeps it off other
  surfaces (`read_tool_output`). `Group` puts it behind `load_tools`
  (`middleware.Loader`) in chat, when the model expands tool references
  (Anthropic's own API). The registry applies the confirmation gate
  from traits; MCP serves the registry ungated and maps traits to scopes
  and annotations.

Tool descriptions stand alone (an MCP client may never show the prompt):
what it does and returns, when to use it and which sibling instead, and the
failure it hits. Each argument gets one sentence. Optionality, fixed
defaults and ranges are schema keywords, not prose. Nothing about
confirmation. Plain sentences, no semicolons, caps or em dashes, British
spelling. `tools/testdata/tool_schemas.golden` pins them; regenerate with
`UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/`
and review its diff.
