package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// setDocumentIcon changes a document's icon identifier.
type setDocumentIcon struct{ *Input }

// Info returns the tool's model-facing description.
func (t *setDocumentIcon) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameSetDocumentIcon,
		"Change a document's icon identifier (Lucide-style, e.g. \"lucide:rocket\").",
		map[string]any{
			_keyDocumentID:    stringProp(_descDocumentID),
			document.AttrIcon: stringProp("The new icon identifier."),
		},
		_keyDocumentID, document.AttrIcon,
	)
}

// Label returns no status line: an icon change is too minor to announce.
func (t *setDocumentIcon) Label(_ context.Context, _ json.RawMessage) string {
	return ""
}

// Confirm describes the icon change the model wants to make.
func (t *setDocumentIcon) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		Icon string `json:"icon"`
	}

	t.parseToolArgs(args, &in)

	return t.summarize(ctx, NameSetDocumentIcon, args, func(subject string) string {
		if in.Icon == "" {
			return "Change icon of " + subject
		}

		return fmt.Sprintf("Change icon of %s to %s", subject, in.Icon)
	})
}

// InvokableRun sets the icon and refreshes the tree.
func (t *setDocumentIcon) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Icon       string `json:"icon"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("set_document_icon: invalid input: %w", err)
	}

	if in.Icon == "" {
		return "", errors.New("set_document_icon: icon is required")
	}

	out, err := t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.SetIcon(in.Icon)})
	if err != nil {
		return "", err
	}

	t.notifyTreeChangeForDocument(ctx, in.DocumentID)

	return out, nil
}
