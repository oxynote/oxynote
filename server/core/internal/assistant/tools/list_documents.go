package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// listDocuments returns the organisation's document tree.
type listDocuments struct{ *Input }

// Info returns the tool's model-facing description.
func (t *listDocuments) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameListDocuments,
		"List documents in the organisation. Returns the document tree (id, name, icon, children). Use parent_id to list only the direct children of that document; omit to list the whole org.",
		map[string]any{
			"parent_id": stringProp("Optional. Only return the direct children of this parent id. Omit for the full tree."),
		},
	)
}

// Label returns no status line: listing is too generic to announce.
func (t *listDocuments) Label(_ context.Context, _ json.RawMessage) string {
	return ""
}

// InvokableRun lists the documents the model asked for.
func (t *listDocuments) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		ParentID string `json:"parent_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("list_documents: invalid input: %w", err)
	}

	var (
		tree document.Summaries
		err  error
	)

	if in.ParentID == "" {
		tree, err = t.db.FetchDocumentTree(ctx, t.orgID)
	} else {
		parentID, perr := xid.FromString(in.ParentID)
		if perr != nil {
			return "", fmt.Errorf("list_documents: parent_id is not a valid xid: %w", perr)
		}

		tree, err = t.db.FetchDocumentTreeByDocumentParentID(ctx, null.ValueFrom(parentID), t.orgID)
	}

	if err != nil {
		return "", fmt.Errorf("list_documents: fetch tree: %w", err)
	}

	return result(struct {
		Documents []docTreeNode `json:"documents"`
	}{
		Documents: summariesToTree(tree),
	})
}

// docTreeNode is the shape returned by list_documents. It mirrors
// document.Summary but uses snake_case keys so the AI consumes a
// consistent vocabulary with the rest of the tool surface.
type docTreeNode struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Icon     string        `json:"icon"`
	Children []docTreeNode `json:"children,omitempty"`
}

// summariesToTree converts the document package's nested Summary tree
// into the snake_case shape returned by list_documents.
func summariesToTree(ss document.Summaries) []docTreeNode {
	if len(ss) == 0 {
		return nil
	}

	out := make([]docTreeNode, 0, len(ss))

	for _, s := range ss {
		out = append(out, docTreeNode{
			ID:       s.ID.String(),
			Name:     s.DocumentName,
			Icon:     s.Icon,
			Children: summariesToTree(s.Children),
		})
	}

	return out
}
