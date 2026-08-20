package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
)

// _defaultDocumentIcon is the icon assigned to assistant-created
// documents when the model doesn't pick one.
const _defaultDocumentIcon = "lucide:file"

// createDocument creates a new document in the organisation.
type createDocument struct{ *Input }

// Info returns the tool's model-facing description.
func (t *createDocument) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameCreateDocument,
		"Create a new document. Returns {document_id, branch_id}. The document starts with one empty paragraph; immediately follow up with append_block / insert_block calls to populate it.",
		map[string]any{
			"name":            stringProp("Display name for the new document."),
			document.AttrIcon: stringProp("Lucide icon identifier (e.g. \"lucide:file-text\"). Defaults to \"lucide:file\" when empty."),
			"parent_id":       stringProp("Optional parent document id. Omit to create at the org root."),
		},
		"name",
	)
}

// Label announces the document being created.
func (t *createDocument) Label(_ context.Context, args json.RawMessage) string {
	if name := t.name(args); name != "" {
		return fmt.Sprintf("Creating %q", name)
	}

	return "Creating a document"
}

// Confirm describes the document the model wants to create.
func (t *createDocument) Confirm(_ context.Context, args json.RawMessage) ConfirmActionSummary {
	// a document that does not exist yet has no id to resolve, so this
	// is the one write whose summary names no target.
	out := ConfirmActionSummary{Tool: string(NameCreateDocument), Summary: "Create a new document"}

	if name := t.name(args); name != "" {
		out.Summary = fmt.Sprintf("Create document %q", name)
	}

	return out
}

// name reads the requested display name from the tool's arguments.
func (t *createDocument) name(args json.RawMessage) string {
	var in struct {
		Name string `json:"name"`
	}

	t.parseToolArgs(args, &in)

	return in.Name
}

// InvokableRun creates the document, its maintainer row and its search job.
func (t *createDocument) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		ParentID string `json:"parent_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("create_document: invalid input: %w", err)
	}

	if in.Name == "" {
		return "", errors.New("create_document: name is required")
	}

	icon := in.Icon
	if icon == "" {
		icon = _defaultDocumentIcon
	}

	var parentID null.Value[xid.ID]

	if in.ParentID != "" {
		pid, err := xid.FromString(in.ParentID)
		if err != nil {
			return "", fmt.Errorf("create_document: parent_id is not a valid xid: %w", err)
		}

		if err := t.db.CheckDocumentExists(ctx, pid, t.orgID); err != nil {
			return "", fmt.Errorf("create_document: parent not found: %w", err)
		}

		parentID = null.ValueFrom(pid)
	}

	doc := document.NewDocument(document.CreateInput{
		Name:     in.Name,
		Icon:     icon,
		ParentID: parentID,
	}, t.orgID, t.userID)

	// the three writes go together: a half-created document the model
	// then retries leaves two of them behind, one without a maintainer
	// and invisible to search.
	var tx Tx

	if err := t.db.BeginTx(ctx, &tx); err != nil {
		return "", fmt.Errorf("create_document: begin: %w", err)
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := tx.InsertDocument(ctx, doc); err != nil {
		return "", fmt.Errorf("create_document: insert: %w", err)
	}

	// mirror the HTTP create path: whoever asked for the document
	// becomes its first maintainer so they own it from the start.
	if err := tx.UpsertDocumentMaintainers(ctx, doc.ID, t.orgID, []string{t.userID}); err != nil {
		return "", fmt.Errorf("create_document: upsert maintainers: %w", err)
	}

	// without this the document is invisible to search until someone
	// edits it, since only the persist path queues a job.
	if err := tx.InsertDocumentSearchJob(ctx, search.BlocksDiff(nil, doc.Search())); err != nil {
		return "", fmt.Errorf("create_document: insert search job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("create_document: commit: %w", err)
	}

	t.notifyTreeChange(doc.ParentID)

	return result(struct {
		DocumentID string `json:"document_id"`
		BranchID   string `json:"branch_id"`
	}{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
	})
}
