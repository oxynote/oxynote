package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/rs/xid"
)

// ConfirmActionSummary is the human-readable description of a
// pending write op surfaced to the confirm UI. Built from the same
// JSON args the tool was about to receive: Summarize never executes
// the operation.
type ConfirmActionSummary struct {
	// Tool is the name of the tool being proposed.
	Tool string

	// DocumentID is the document the op targets, when known.
	DocumentID string

	// DocumentName is the document's display name, when the
	// summary helper could look it up.
	DocumentName string

	// Summary is a single-line description (e.g. "Insert callout
	// after 'Rate limit' section").
	Summary string
}

// parseToolArgs unmarshals the tool's JSON args into dst. The label
// helpers in this file are best-effort: a malformed args payload
// should degrade the label, not abort the surrounding flow. So we
// log a warning and let dst keep its zero value rather than
// propagating the error. Anthropic enforces the input schema at the
// tool-call boundary, so a failure here is unusual and worth knowing
// about.
func (m *Manager) parseToolArgs(name Name, args json.RawMessage, dst any) {
	if err := json.Unmarshal(args, dst); err != nil {
		m.log.Warn("summarize: tool args unmarshal failed",
			slog.String("tool", string(name)),
			slog.String("error", err.Error()),
		)
	}
}

// ToolStatusLabel builds a one-line pill string describing what
// the assistant is about to do (e.g. "Reading 'Cat Facts'"). Returns
// an empty string for tools that are too noisy or too generic to
// surface; those just run silently. The verb word at the start
// matches the action verb the client uses to color the pill.
func (m *Manager) ToolStatusLabel(ctx context.Context, name Name, args json.RawMessage) string {
	docName := ""
	if docID := m.pluckDocumentID(name, args); docID != "" {
		docName = m.lookupDocumentName(ctx, docID)
	}

	subject := docName
	if subject == "" {
		subject = "document"
	}

	switch name {
	case NameReadDocumentSummary:
		return fmt.Sprintf("Reading %s", subject)

	case NameReadBlock:
		return fmt.Sprintf("Reading a block in %s", subject)

	case NameSearchDocuments:
		var in struct {
			Query string `json:"query"`
		}

		m.parseToolArgs(name, args, &in)
		if in.Query == "" {
			return "Searching documents"
		}

		return fmt.Sprintf("Searching for %q", in.Query)

	case NameCreateDocument:
		var in struct {
			Name string `json:"name"`
		}

		m.parseToolArgs(name, args, &in)
		if in.Name == "" {
			return "Creating a document"
		}

		return fmt.Sprintf("Creating %q", in.Name)

	case NameDeleteDocument:
		return fmt.Sprintf("Deleting %s", subject)

	case NameRenameDocument:
		return fmt.Sprintf("Renaming %s", subject)

	case NameMoveDocument:
		return fmt.Sprintf("Moving %s", subject)

	case NameInsertBlock,
		NameAppendBlock,
		NamePrependBlock,
		NameReplaceBlock,
		NameUpdateBlockText,
		NameUpdateBlockAttrs,
		NameDeleteBlock:
		return fmt.Sprintf("Updating %s", subject)
	}

	// list_documents, get_document, set_document_icon: too noisy or
	// too generic; let them run without a pill.
	return ""
}

// Summarize builds a ConfirmActionSummary from a tool name and its
// raw JSON args. The function performs at most one DB read (to
// resolve a document name when the tool carries a document_id); on
// failure it falls back to a less specific summary rather than
// surfacing an error.
func (m *Manager) Summarize(ctx context.Context, name Name, args json.RawMessage) ConfirmActionSummary {
	out := ConfirmActionSummary{Tool: string(name)}

	docID := m.pluckDocumentID(name, args)
	if docID != "" {
		out.DocumentID = docID
		out.DocumentName = m.lookupDocumentName(ctx, docID)
	}

	out.Summary = m.describe(name, args, out.DocumentName)

	return out
}

// pluckDocumentID peeks at the JSON args for a document_id (or
// new_parent_id for moves), returning the empty string when the
// op doesn't reference an existing document (create_document) or
// the JSON is malformed (in which case parseToolArgs has already
// logged).
func (m *Manager) pluckDocumentID(name Name, args json.RawMessage) string {
	if name == NameCreateDocument {
		return ""
	}

	var probe struct {
		DocumentID string `json:"document_id"`
	}

	m.parseToolArgs(name, args, &probe)

	return probe.DocumentID
}

// lookupDocumentName fetches the document's display name. Failures
// (bad id, not found, transient) return an empty string so the
// confirm UI gracefully falls back to the id alone.
func (m *Manager) lookupDocumentName(ctx context.Context, documentID string) string {
	id, err := xid.FromString(documentID)
	if err != nil {
		return ""
	}

	doc, err := m.db.FetchDocument(ctx, id, m.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return ""
	}

	return doc.DocumentName
}

// describe builds the one-sentence Summary string for the confirm
// UI. The text never quotes a uid (uids are internal identifiers);
// instead it leans on document name plus a short description of
// the change.
func (m *Manager) describe(name Name, args json.RawMessage, docName string) string {
	subject := docName
	if subject == "" {
		subject = "document"
	}

	switch name {
	case NameCreateDocument:
		var in struct {
			Name string `json:"name"`
		}

		m.parseToolArgs(name, args, &in)
		if in.Name == "" {
			return "Create a new document"
		}

		return fmt.Sprintf("Create document %q", in.Name)

	case NameDeleteDocument:
		return fmt.Sprintf("Delete %s", subject)

	case NameRenameDocument:
		var in struct {
			Name string `json:"name"`
		}

		m.parseToolArgs(name, args, &in)
		if in.Name == "" {
			return fmt.Sprintf("Rename %s", subject)
		}

		return fmt.Sprintf("Rename %s to %q", subject, in.Name)

	case NameSetDocumentIcon:
		var in struct {
			Icon string `json:"icon"`
		}

		m.parseToolArgs(name, args, &in)

		return fmt.Sprintf("Change icon of %s to %s", subject, in.Icon)

	case NameMoveDocument:
		var in struct {
			NewParentID string `json:"new_parent_id"`
		}

		m.parseToolArgs(name, args, &in)
		if in.NewParentID == "" {
			return fmt.Sprintf("Move %s to the org root", subject)
		}

		return fmt.Sprintf("Move %s under another document", subject)

	case NameInsertBlock:
		var in struct {
			Position string `json:"position"`
			Block    struct {
				Type string `json:"type"`
			} `json:"block"`
		}

		m.parseToolArgs(name, args, &in)

		return fmt.Sprintf("Insert %s %s a block in %s",
			blockKindLabel(in.Block.Type), in.Position, subject)

	case NameAppendBlock:
		var in struct {
			Block struct {
				Type string `json:"type"`
			} `json:"block"`
		}

		m.parseToolArgs(name, args, &in)

		return fmt.Sprintf("Append %s to %s", blockKindLabel(in.Block.Type), subject)

	case NamePrependBlock:
		var in struct {
			Block struct {
				Type string `json:"type"`
			} `json:"block"`
		}

		m.parseToolArgs(name, args, &in)

		return fmt.Sprintf("Prepend %s to %s", blockKindLabel(in.Block.Type), subject)

	case NameReplaceBlock:
		var in struct {
			Block struct {
				Type string `json:"type"`
			} `json:"block"`
		}

		m.parseToolArgs(name, args, &in)

		return fmt.Sprintf("Replace a block in %s with %s",
			subject, blockKindLabel(in.Block.Type))

	case NameUpdateBlockText:
		var in struct {
			Text string `json:"text"`
		}

		m.parseToolArgs(name, args, &in)
		preview := preview(in.Text, 60)
		if preview == "" {
			return fmt.Sprintf("Update text of a block in %s", subject)
		}

		return fmt.Sprintf("Update a block in %s: %q", subject, preview)

	case NameUpdateBlockAttrs:
		var in struct {
			Attrs map[string]any `json:"attrs"`
		}

		m.parseToolArgs(name, args, &in)

		keys := make([]string, 0, len(in.Attrs))
		for k := range in.Attrs {
			keys = append(keys, k)
		}

		if len(keys) == 0 {
			return fmt.Sprintf("Update block attributes in %s", subject)
		}

		return fmt.Sprintf("Update block %s in %s", strings.Join(keys, ", "), subject)

	case NameDeleteBlock:
		return fmt.Sprintf("Delete a block in %s", subject)
	}

	return string(name)
}

// blockKindLabel turns a canonical type string into a friendlier
// label for the confirm UI. Unknown types fall through verbatim.
func blockKindLabel(kind string) string {
	switch kind {
	case "paragraph":
		return "a paragraph"
	case "heading":
		return "a heading"
	case "blockquote":
		return "a blockquote"
	case "bullet_list":
		return "a bullet list"
	case "ordered_list":
		return "an ordered list"
	case "task_list":
		return "a task list"
	case "callout":
		return "a callout"
	case "code":
		return "a code block"
	case "titled_code":
		return "a titled code block"
	case "mermaid":
		return "a mermaid diagram"
	case "horizontal_rule":
		return "a divider"
	case "image":
		return "an image"
	case "figma":
		return "a figma embed"
	case "metric":
		return "a metric"
	case "metric_grid":
		return "a metric grid"
	case "split_doc":
		return "a split documentation block"
	case "split_doc_param_list":
		return "a parameter list"
	}

	if kind == "" {
		return "a block"
	}

	return "a " + kind + " block"
}

// preview returns at most max runes of s, collapsed to a single
// line. Used for surfacing short snippets of an upcoming text edit
// in the confirm UI.
func preview(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	if max <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= max {
		return s
	}

	return string(runes[:max]) + "…"
}
