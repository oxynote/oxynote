# AGENTS.md

The assistant: system prompt and tools. Go standards live in
[server/core/AGENTS.md](../../AGENTS.md), architecture in
[server/AGENTS.md](../../../AGENTS.md).

## Prompt

`prompt.go` assembles the system prompt from section constants. Block model,
edit etiquette and aesthetics ship to both the chat model and MCP clients;
surface-specific rules (confirmation flow, persona) live in that surface's
section. Behaviour rules come first and are restated at the end.

Writing a rule: state the principle and its reason in one or two sentences,
no edge-case lists or numbered steps; say what to do rather than what to
avoid; per-tool facts go in the tool description, the prompt carries only
cross-tool workflow; examples only where structure is needed, wrapped in
`<example>`; plain language, no caps or MUST; no em dashes and British
spelling (`prompt_test.go` and `tools_test.go` enforce both).

## Tools

`tools/`, grouped by subject: `document.go`, `block.go`, `tag.go`,
`hook.go`, `search.go`, `datasource.go`; `eino.go` holds the framework
adapter and `read_tool_output`. `tools.Set` is the only list of tools;
adding one means its type plus a line there, and the MCP server picks it up
for free.

- **Tools implement this package's `Tool`** (`Info`, `Traits`, `Title`,
  `Summary`, `Execute`), never eino's interfaces; only `eino.go` imports
  eino. Other surfaces use `Entry.Info` to describe and `Entry.Tool.Run` to
  run.
- **A call reports what it changed** in `Result.Documents`; every write goes
  through an `Input` method that records the document as it mutates (a
  document delete records nothing, a hook delete records the branch,
  tag-only writes record none).
- **Tag and hook writes notify**: tag tools end with
  `Input.NotifyTagTreeChange` (`assign_tag`/`unassign_tag` also
  `NotifyBranchTagsChange`), hook writes call `NotifyHooksChange`, wired in
  `server.NewServer` beside the tree notifier. Hook writes drive the same
  `hook.NewHook`/`ApplyUpdate`/`Reset`/`Delete` lifecycle as the HTTP
  handler; `decodeHookSettings` switches on type into the processor's own
  struct.
- **A block write reports from the operation, never a re-read** (the
  realtime service persists on a debounce): `expandForWrite` once, ship the
  canonical uids, return `blockRows` of the expanded tree.
- **Branches are addressed by id, always.** Every content tool requires
  `branch_id`; listings carry the ids; `FetchDocumentByBranchID` refuses a
  branch of another document; an unknown id is refused naming the
  document's branches (`ErrUnknownBranch`); a protected branch reads but
  refuses writes, naming the unprotected ones. No tool creates, renames,
  deletes or merges a branch. MCP resources are
  `oxynote://documents/{id}/branches/{branch_id}`.
- **Every error names the next step**: what was wrong, then the tool or
  argument that fixes it (`describeOpError` rewrites the realtime service's
  errors; `errUnknownDocument`, `errUnknownBlock`, `errBranchProtected`,
  `errUnknownParent` follow suit).
- **`Decode` is the only way into arguments.** Every tool has a `<tool>Args`
  type with `Validate() error` (`errRequired(key)`); ids are `xid.ID`,
  timestamps `time.Time`, enums self-validating; `encoding/json/v2` reports
  the JSON path. An empty string is invalid, never absent.
- `Title`/`Summary` fetch what they name and use the row's name; an
  unresolvable target is an error. `Input` is built per call, scoped to the
  session's (organisation, user), with `Fetch*`-prefixed reads;
  `DescribeInput` is its read-only half.
- **A write owns its invariants** on the `Input` method that performs it
  (`MoveDocument` refuses a missing parent or a move under its own subtree),
  not in `Execute`.
- **`Traits()` says what a tool is**; the mixins
  `plainSummary`/`plainTraits`/`plainTitle` cover the defaults. `Write`
  gates behind confirmation and protects the result from context
  middlewares (only writes have a `Summary`; `Test_New` checks).
  `Destructive` keeps it outside "approve all" (the four `delete_*`).
  `Overwrites` means content the caller did not name is replaced
  (`update_block_text`, `replace_block`). `DataSource` reads an outbound
  connection (own MCP scope, open-world). `Internal` keeps it off other
  surfaces (`read_tool_output`). The registry applies the confirmation gate
  from traits; MCP serves the registry ungated and maps traits to
  annotations.

Tool descriptions stand alone (an MCP client may never show the prompt):
what it does and returns, when to use it and which sibling instead, the
failure it can hit, and each argument's meaning, optionality and default.
Nothing about confirmation. Plain language, no caps, no em dashes, British
spelling. `tools/testdata/tool_schemas.golden` pins them; regenerate with
`UPDATE_GOLDEN=1 go test -run Test_Info_toEino ./internal/assistant/tools/`
and review the diff as the change.
