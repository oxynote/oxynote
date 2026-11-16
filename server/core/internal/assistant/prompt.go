package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/org"
)

// _sessionKeyActiveDocument carries the document the user is looking
// at into the system prompt for one turn.
const _sessionKeyActiveDocument = "oxynote_assistant_active_document"

// _sessionKeyActiveBranch carries the id of the branch of that document
// the user is looking at.
const _sessionKeyActiveBranch = "oxynote_assistant_active_branch"

// _sessionKeyRole carries the role the user holds. It is a plain string,
// because the checkpoint of a paused turn cannot encode a type it has
// not registered.
const _sessionKeyRole = "oxynote_assistant_role"

// _personaSection is the chat-only opening of the system prompt: the
// Rubber Duck persona, how it works with the user and the tool-use rules
// every chat follows. It never ships to the MCP surface.
const _personaSection = `You are Rubber Duck, the AI assistant inside Oxynote, a collaborative product for writing technical documentation. The documents you read and write capture project information, architecture and feature descriptions. They are written for humans first, balancing prose with technical detail where it sharpens meaning. Your job is to help people think through and write those documents, not to write code.

## How you work

Rubber Duck is a thinking partner. When someone shares an idea or a draft, point out gaps, ambiguities and unstated assumptions, suggest scenarios the document doesn't cover, and ask the questions that help the writer decide for themselves. Address an ambiguous request as best you can before asking about it, and ask at most one question per reply.

Keep replies focused and brief: lead with the answer, keep caveats short, and use a list only when the content has several parts that read better that way. Write with commas, colons and full stops rather than em dashes, in replies and in documents alike.

## Tool use

Read tools run immediately. Independent calls go in one turn. Read an id or parameter value before you use it, and never invent one.

`

// _chatWriteSection holds the chat rules for a user who may edit: when
// to write rather than interview, and the confirmation flow that writes
// wait on. Its clients own their approval story, so MCP never gets it.
const _chatWriteSection = `## Writing

When someone asks for text, write it: a request to add, change or remove content is an instruction, not an invitation to interview them first. Write tools wait for the user's confirmation, and every write in a turn shares one confirmation, so make all the related edits in the same turn. A protected branch is read-only, so write to another branch of the document or say there is none rather than retrying.

`

// _readOnlySection tells the model that the user may only read, apart
// from their own variable values.
const _readOnlySection = `## Read only

This person can read the organisation's documents but not change them. When they ask for a change, say what to change and where, and suggest an editor makes it. They can still set their own variable values.

`

// _workflowReadSection is the cross-tool workflow for reading. Every
// surface ships it.
const _workflowReadSection = `## Workflow

Find the document with list_documents or search_documents and read it with get_document. Every content tool names a branch by branch_id: listings and search hits carry one, and get_document lists every branch. Metrics follow the built-in metric_range and metric_refresh variables, so to show another period, set the variable rather than editing charts.

`

// _workflowWriteParagraph is the cross-tool workflow for writing.
const _workflowWriteParagraph = `Read before you write, since a protected branch is read-only. Before adding a metric or logs block, find names with get_data_source_metadata and run the query through query_data_source, with the chart_type you intend for a metric, so you know it renders.

`

// _workflowSection is the whole workflow, shared verbatim between the
// chat prompt and the MCP instructions.
const _workflowSection = _workflowReadSection + _workflowWriteParagraph

// _blockModelReadSection explains how content is read as markup. Every
// surface ships it.
const _blockModelReadSection = `## Content

get_document returns a document's blocks as XML, one element per block with its id.

`

// _blockModelWriteParagraph explains how content is written as markup.
// The elements themselves are in insert_blocks' description, which every
// client shows in full.
const _blockModelWriteParagraph = `To change blocks, edit that XML and send it back with replace_blocks, keeping the id of every element that stays: a kept id keeps the block's comments, hooks and files. insert_blocks adds new blocks, many in one call, and its description lists every element.

`

// _blockModelSection is the whole content model, shared verbatim between
// the chat prompt and the MCP instructions.
const _blockModelSection = _blockModelReadSection + _blockModelWriteParagraph

// _etiquetteSection codifies how edits are shaped. Shared verbatim
// between the chat prompt and the MCP instructions.
const _etiquetteSection = `## Editing

Make small, targeted edits and send independent ones together. Write each section completely before the next, and keep names, numbers and claims consistent across the document.

`

// _aestheticsSection states what a readable document looks like.
// Shared verbatim between the chat prompt and the MCP instructions.
const _aestheticsSection = `## Style

Open with the first real paragraph, since the name already shows above the content. Keep the outline flat, with a heading only over more than a paragraph or two. Use a callout for what the reader must not miss. Leave a code language empty unless the document names a stack. Write an endpoint as one split_doc: an <h1> naming the action, such as Submitting a transfer, <params> for its fields, and on its right a titled pre for the request, titled by method and path such as POST /v1/transfers, then one per response.

`

// _reminderSection closes the chat prompt with the rules that matter
// most, restated in two lines. Models weight the start and the end of
// a prompt more than its middle, so the behaviour rules get a second
// showing here.
const _reminderSection = `## Before you reply

Read before you write, put every related edit in this turn, ask at most one question, and keep the reply brief.
`

// _readOnlyReminderSection is the closing reminder for a user who may
// only read.
const _readOnlyReminderSection = `## Before you reply

Read before you answer, ask at most one question, and keep the reply brief.
`

// _basePrompt is the system prompt sent to the model on every
// assistant turn for a user who may edit: the read rules, then the write
// rules, then the closing reminder.
const _basePrompt = _personaSection + _chatWriteSection + _workflowSection + _blockModelSection + _etiquetteSection + _aestheticsSection + _reminderSection

// _readOnlyPrompt is the system prompt for a user who may only read: the
// read rules alone, with none of the write ones.
const _readOnlyPrompt = _personaSection + _readOnlySection + _workflowReadSection + _blockModelReadSection + _readOnlyReminderSection

// _mcpIntroSection frames the shared sections for an agent connecting
// over MCP: unlike the chat model, it knows nothing about Oxynote, and
// its own client owns the approval story, so writes apply immediately
// instead of waiting on a confirmation.
const _mcpIntroSection = `You are connected to Oxynote, a collaborative product for writing technical documentation, with the documents of one organisation. Documents are written for humans first, balancing prose with technical detail where it sharpens meaning. Write tools apply immediately.

`

// MCPInstructions returns the text the MCP server hands a connecting
// client in its initialize response: the MCP framing followed by the
// sections the chat prompt shares. Claude Code shows only the first 2 KB
// of it, so it has to stay below that.
func MCPInstructions() string {
	return _mcpIntroSection + _workflowSection + _blockModelSection + _etiquetteSection + _aestheticsSection
}

// buildSystemPrompt assembles the prompt sent on each turn. The
// activeDocumentID, when non-empty, hints to the model which
// document the user is currently viewing, useful for resolving
// "this document" or "here" references without forcing the user to
// spell out an id; activeBranchID names the branch the client reported.
// A role that may not edit gets the read-only prompt.
func buildSystemPrompt(activeDocumentID, activeBranchID string, role org.Role) string {
	base := _basePrompt
	if !role.CanEdit() {
		base = _readOnlyPrompt
	}

	if activeDocumentID == "" {
		return base
	}

	var sb strings.Builder
	sb.WriteString(base)
	sb.WriteString("\n## Current context\n\n")

	if activeBranchID == "" {
		fmt.Fprintf(&sb, "The user is currently viewing document `%s`. When they say \"this document\", \"here\", or \"the doc\" without naming one, this is the document they mean.\n", activeDocumentID)
	} else {
		fmt.Fprintf(&sb, "The user is currently viewing document `%s` on branch `%s`. When they say \"this document\", \"here\", or \"the doc\" without naming one, this is the document and the branch they mean, so read and write with branch_id `%s`.\n", activeDocumentID, activeBranchID, activeBranchID)
	}

	return sb.String()
}

// genModelInput builds the input for one run: the system prompt for the
// open document, then the conversation. It skips the framework's
// templates, because the prompt holds literal braces.
func genModelInput(ctx context.Context, _ string, input *adk.AgentInput) ([]*schema.Message, error) {
	msgs := make([]*schema.Message, 0, len(input.Messages)+1)
	msgs = append(msgs, schema.SystemMessage(buildSystemPrompt(
		sessionValue[string](ctx, _sessionKeyActiveDocument),
		sessionValue[string](ctx, _sessionKeyActiveBranch),
		org.Role(sessionValue[string](ctx, _sessionKeyRole)),
	)))

	// the conversation a completed run leaves behind includes the system
	// message this function prepended, so appending the input verbatim
	// would stack one more prompt every turn. It would also keep stale
	// ones whose current-context section names a document the user has
	// left. Only the fresh prompt survives.
	for _, msg := range input.Messages {
		if msg != nil && msg.Role == schema.System {
			continue
		}

		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// sessionValue returns the value the session stores under key for this
// run, or the zero value when nothing is stored there.
func sessionValue[T any](ctx context.Context, key string) T {
	var zero T

	v, ok := adk.GetSessionValue(ctx, key)
	if !ok {
		return zero
	}

	res, ok := v.(T)
	if !ok {
		// NOCOV: each key is only ever written with one type.
		return zero
	}

	return res
}
