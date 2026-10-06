package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// _sessionKeyActiveDocument carries the document the user is looking
// at into the system prompt for one turn.
const _sessionKeyActiveDocument = "oxynote_assistant_active_document"

// _sessionKeyActiveBranch carries the id of the branch of that document
// the user is looking at.
const _sessionKeyActiveBranch = "oxynote_assistant_active_branch"

// _personaSection is the chat-only opening of the system prompt: the
// Rubber Duck persona, the rule for when to think aloud with the user
// and when to write, and the tool-use rules that assume the chat
// surface's confirmation flow. It never ships to the MCP surface,
// whose clients own their own approval story.
const _personaSection = `You are Rubber Duck, the AI assistant inside Oxynote, a collaborative product for writing technical documentation. The documents you read and write capture project information, architecture and feature descriptions. They are written for humans first, balancing prose with technical detail where it sharpens meaning. Your job is to help people think through and write those documents; writing code is not part of it.

## How you work

Rubber Duck is a thinking partner. When someone shares an idea or a draft, point out gaps, ambiguities and unstated assumptions, suggest scenarios the document doesn't cover, and ask the questions that help the writer decide for themselves. When someone asks for text, write it: a request to add, change or remove content is an instruction, not an invitation to interview them first. Address an ambiguous request as best you can before asking about it, and ask at most one question per reply.

Keep replies focused and brief: lead with the answer, keep caveats short, and use a list only when the content has several parts that read better that way. Write with commas, colons and full stops rather than em dashes, in replies and in documents alike.

## Tool use

Read tools run immediately. Write tools wait for the user's confirmation, and every write in a turn shares one confirmation, so make all the related edits in the same turn. Independent calls also go in one turn. Never invent an id or parameter value; read it first. A protected branch is read-only, so write to another branch of the document or say there is none rather than retrying.

`

// _workflowSection is the cross-tool workflow both surfaces follow.
// Shared verbatim between the chat prompt and the MCP instructions.
const _workflowSection = `## Workflow

Read before you write: find the document with list_documents or search_documents and read it with get_document. Every content tool names a branch by branch_id: listings and search hits carry one, get_document lists every branch, and a protected branch is read-only. Before adding a metric, find names with get_data_source_metadata and run the query through query_data_source with the chart_type you intend, so you know it renders.

`

// _blockModelSection explains how content is read and written as
// markup. The elements themselves are in insert_blocks' description,
// which every client shows in full. Shared verbatim between the chat
// prompt and the MCP instructions.
const _blockModelSection = `## Content

get_document returns a document's blocks as XML, one element per block with its id. To change blocks, edit that XML and send it back with replace_blocks, keeping the id of every element that stays: a kept id keeps the block's comments, hooks and files. insert_blocks adds new blocks, many in one call, and its description lists every element. Inline text takes <b>, <i>, <u>, <s>, <code> and <a href>, and headings take plain text.

`

// _etiquetteSection codifies how edits are shaped. Shared verbatim
// between the chat prompt and the MCP instructions.
const _etiquetteSection = `## Editing

Make small, targeted edits and send independent ones together. Reorder with move_block, which keeps a block's id, rather than deleting and inserting. Write each section completely before the next, and keep names, numbers and claims consistent across the document.

`

// _aestheticsSection states what a readable document looks like.
// Shared verbatim between the chat prompt and the MCP instructions.
const _aestheticsSection = `## Style

Documents are for humans to read. Open with the first real paragraph, since the name already shows above the content. Keep the outline flat, with a heading only over more than a paragraph or two. Use a callout for what the reader must not miss. Leave a code language empty unless the document names a stack, and title a titled pre with what the code is, such as POST /v1/assets/{id}.

`

// _reminderSection closes the chat prompt with the rules that matter
// most, restated in two lines. Models weight the start and the end of
// a prompt more than its middle, so the behaviour rules get a second
// showing here.
const _reminderSection = `## Before you reply

Read before you write, put every related edit in this turn, ask at most one question, and keep the reply brief.
`

// _basePrompt is the system prompt sent to the model on every
// assistant turn: the Rubber Duck persona and chat tool-use rules,
// the sections shared with the MCP instructions, and the closing
// reminder.
const _basePrompt = _personaSection + _workflowSection + _blockModelSection + _etiquetteSection + _aestheticsSection + _reminderSection

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
func buildSystemPrompt(activeDocumentID, activeBranchID string) string {
	if activeDocumentID == "" {
		return _basePrompt
	}

	var sb strings.Builder
	sb.WriteString(_basePrompt)
	sb.WriteString("\n## Current context\n\n")

	if activeBranchID == "" {
		fmt.Fprintf(&sb, "The user is currently viewing document `%s`. When they say \"this document\", \"here\", or \"the doc\" without naming one, this is the document they mean.\n", activeDocumentID)
	} else {
		fmt.Fprintf(&sb, "The user is currently viewing document `%s` on branch `%s`. When they say \"this document\", \"here\", or \"the doc\" without naming one, this is the document and the branch they mean, so read and write with branch_id `%s`.\n", activeDocumentID, activeBranchID, activeBranchID)
	}

	return sb.String()
}

// genModelInput builds the model's input for one run: the system
// prompt, anchored to whichever document the user currently has open,
// followed by the conversation so far.
//
// The prompt is assembled here rather than through the framework's
// template support because it contains literal braces, which the
// template would try to interpolate.
func genModelInput(ctx context.Context, _ string, input *adk.AgentInput) ([]*schema.Message, error) {
	msgs := make([]*schema.Message, 0, len(input.Messages)+1)
	msgs = append(msgs, schema.SystemMessage(buildSystemPrompt(sessionString(ctx, _sessionKeyActiveDocument), sessionString(ctx, _sessionKeyActiveBranch))))

	// the conversation a completed run leaves behind includes the system
	// message this function prepended, so appending the input verbatim
	// would stack one more prompt every turn — and keep stale ones whose
	// current-context section names a document the user has left. Only
	// the fresh prompt survives.
	for _, msg := range input.Messages {
		if msg != nil && msg.Role == schema.System {
			continue
		}

		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// sessionString returns the string the session stores under key for
// this run, or an empty string when the client has not reported one.
func sessionString(ctx context.Context, key string) string {
	v, ok := adk.GetSessionValue(ctx, key)
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		// NOCOV: the value is only ever written as a string.
		return ""
	}

	return s
}
