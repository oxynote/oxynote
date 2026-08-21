package tools

import (
	"errors"
	"fmt"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _defaultDocumentIcon is the icon assigned to assistant-created
// documents when the model doesn't pick one.
const _defaultDocumentIcon = "lucide:file"

// listDocuments returns the organisation's document tree.
type listDocuments struct{}

// Info returns the tool's model-facing description.
func (listDocuments) Info() Info {
	return Info{
		Name:        NameListDocuments,
		Description: "List documents in the organisation. Returns the document tree (id, name, icon, children). Use parent_id to list only the direct children of that document; omit to list the whole org.",
		Properties: map[string]any{
			"parent_id": stringProp("Optional. Only return the direct children of this parent id. Omit for the full tree."),
		},
	}
}

// Traits reports a plain read.
func (listDocuments) Traits() Traits {
	return Traits{}
}

// Title returns no status line: listing is too generic to announce.
func (listDocuments) Title(_ DescribeInput) string {
	return ""
}

// Execute lists the documents the model asked for.
func (listDocuments) Execute(inp Input) (string, error) {
	var in struct {
		ParentID string `json:"parent_id"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	var (
		tree document.Summaries
		err  error
	)

	if in.ParentID == "" {
		tree, err = inp.DocumentTree()
	} else {
		parentID, perr := xid.FromString(in.ParentID)
		if perr != nil {
			return "", fmt.Errorf("list_documents: parent_id is not a valid xid: %w", perr)
		}

		tree, err = inp.DocumentChildren(null.ValueFrom(parentID))
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

// getDocument returns one document's metadata.
type getDocument struct{}

// Info returns the tool's model-facing description.
func (getDocument) Info() Info {
	return Info{
		Name:        NameGetDocument,
		Description: "Fetch metadata for one document: name, icon, parent_id, default branch id, protected flag, updated_at.",
		Properties:  documentIDProp(_descDocumentID),
		Required:    []string{_keyDocumentID},
	}
}

// Traits reports a plain read.
func (getDocument) Traits() Traits {
	return Traits{}
}

// Title returns no status line: a metadata read is too noisy to announce.
func (getDocument) Title(_ DescribeInput) string {
	return ""
}

// Execute fetches the document's metadata.
func (getDocument) Execute(inp Input) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get_document: document_id is not a valid xid: %w", err)
	}

	doc, err := inp.Document(docID)
	if err != nil {
		return "", fmt.Errorf("get_document: fetch: %w", err)
	}

	out := struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Icon       string `json:"icon"`
		ParentID   string `json:"parent_id,omitempty"`
		BranchID   string `json:"branch_id"`
		BranchName string `json:"branch_name"`
		Protected  bool   `json:"protected"`
		UpdatedAt  string `json:"updated_at"`
	}{
		ID:         doc.ID.String(),
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		BranchID:   doc.BranchID.String(),
		BranchName: doc.BranchName,
		Protected:  doc.Protected,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if doc.ParentID.Valid {
		out.ParentID = doc.ParentID.V.String()
	}

	return result(out)
}

// createDocument creates a new document in the organisation.
type createDocument struct{}

// Info returns the tool's model-facing description.
func (createDocument) Info() Info {
	return Info{
		Name:        NameCreateDocument,
		Description: "Create a new document. Returns {document_id, branch_id}. The document starts with one empty paragraph; immediately follow up with append_block / insert_block calls to populate it.",
		Properties: map[string]any{
			_keyName:          stringProp("Display name for the new document."),
			document.AttrIcon: stringProp("Lucide icon identifier (e.g. \"lucide:file-text\"). Defaults to \"lucide:file\" when empty."),
			"parent_id":       stringProp("Optional parent document id. Omit to create at the org root."),
		},
		Required: []string{_keyName},
	}
}

// Traits reports a write.
func (createDocument) Traits() Traits {
	return Traits{Write: true}
}

// Title announces the document being created.
func (createDocument) Title(inp DescribeInput) string {
	if name := createDocumentName(inp); name != "" {
		return fmt.Sprintf("Creating %q", name)
	}

	return "Creating a document"
}

// Confirm describes the document the model wants to create.
func (createDocument) Confirm(inp DescribeInput) ConfirmActionSummary {
	// a document that does not exist yet has no id to resolve, so this
	// is the one write whose summary names no target.
	out := ConfirmActionSummary{Tool: string(NameCreateDocument), Summary: "Create a new document"}

	if name := createDocumentName(inp); name != "" {
		out.Summary = fmt.Sprintf("Create document %q", name)
	}

	return out
}

// createDocumentName reads the requested display name from the call's
// arguments.
func createDocumentName(inp DescribeInput) string {
	var in struct {
		Name string `json:"name"`
	}

	inp.Probe(&in)

	return in.Name
}

// Execute creates the document, its maintainer row and its search job.
func (createDocument) Execute(inp Input) (string, error) {
	var in struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		ParentID string `json:"parent_id"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
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

		parentID = null.ValueFrom(pid)
	}

	doc := document.NewDocument(document.CreateInput{
		Name:     in.Name,
		Icon:     icon,
		ParentID: parentID,
	}, inp.OrganizationID(), inp.UserID())

	if err := inp.CreateDocument(doc); err != nil {
		return "", fmt.Errorf("create_document: %w", err)
	}

	inp.NotifyTreeChange(doc.ParentID)

	return result(struct {
		DocumentID string `json:"document_id"`
		BranchID   string `json:"branch_id"`
	}{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
	})
}

// deleteDocument removes a document from the organisation.
type deleteDocument struct{}

// Info returns the tool's model-facing description.
func (deleteDocument) Info() Info {
	return Info{
		Name:        NameDeleteDocument,
		Description: "Delete a document. This is destructive — the user is always asked to confirm and there is no auto-approve.",
		Properties:  documentIDProp("The document id to delete."),
		Required:    []string{_keyDocumentID},
	}
}

// Traits reports a destructive write, which stays outside any "approve
// all" answer.
func (deleteDocument) Traits() Traits {
	return Traits{Write: true, Destructive: true}
}

// Title announces which document is being deleted.
func (deleteDocument) Title(inp DescribeInput) string {
	return "Deleting " + inp.Subject()
}

// Confirm describes the document the model wants to delete.
func (deleteDocument) Confirm(inp DescribeInput) ConfirmActionSummary {
	return summarize(inp, NameDeleteDocument, func(subject string) string {
		return "Delete " + subject
	})
}

// Execute deletes the document and refreshes the tree.
func (deleteDocument) Execute(inp Input) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("delete_document: document_id is not a valid xid: %w", err)
	}

	// capture the parent before the row goes away so we can scope the
	// tree-change notification to the affected subtree.
	var parentID null.Value[xid.ID]

	if doc, ferr := inp.Document(docID); ferr == nil && doc != nil {
		parentID = doc.ParentID
	}

	if err := inp.DeleteDocument(docID); err != nil {
		return "", fmt.Errorf("delete_document: delete: %w", err)
	}

	inp.NotifyTreeChange(parentID)

	return result(struct {
		DocumentID string `json:"document_id"`
		Deleted    bool   `json:"deleted"`
	}{
		DocumentID: docID.String(),
		Deleted:    true,
	})
}

// renameDocument changes a document's display name.
type renameDocument struct{}

// Info returns the tool's model-facing description.
func (renameDocument) Info() Info {
	return Info{
		Name:        NameRenameDocument,
		Description: "Change a document's display name. The change is applied live via hocuspocus.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyName:       stringProp("The new display name."),
		},
		Required: []string{_keyDocumentID, _keyName},
	}
}

// Traits reports a write.
func (renameDocument) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being renamed.
func (renameDocument) Title(inp DescribeInput) string {
	return "Renaming " + inp.Subject()
}

// Confirm describes the rename the model wants to make.
func (renameDocument) Confirm(inp DescribeInput) ConfirmActionSummary {
	var in struct {
		Name string `json:"name"`
	}

	inp.Probe(&in)

	return summarize(inp, NameRenameDocument, func(subject string) string {
		if in.Name == "" {
			return "Rename " + subject
		}

		return fmt.Sprintf("Rename %s to %q", subject, in.Name)
	})
}

// Execute renames the document and refreshes the tree.
func (renameDocument) Execute(inp Input) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Name       string `json:"name"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Name == "" {
		return "", errors.New("rename_document: name is required")
	}

	out, err := inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.SetName(in.Name)})
	if err != nil {
		return "", err
	}

	inp.NotifyTreeChangeForDocument(in.DocumentID)

	return out, nil
}

// setDocumentIcon changes a document's icon identifier.
type setDocumentIcon struct{}

// Info returns the tool's model-facing description.
func (setDocumentIcon) Info() Info {
	return Info{
		Name:        NameSetDocumentIcon,
		Description: "Change a document's icon identifier (Lucide-style, e.g. \"lucide:rocket\").",
		Properties: map[string]any{
			_keyDocumentID:    stringProp(_descDocumentID),
			document.AttrIcon: stringProp("The new icon identifier."),
		},
		Required: []string{_keyDocumentID, document.AttrIcon},
	}
}

// Traits reports a write.
func (setDocumentIcon) Traits() Traits {
	return Traits{Write: true}
}

// Title returns no status line: an icon change is too minor to announce.
func (setDocumentIcon) Title(_ DescribeInput) string {
	return ""
}

// Confirm describes the icon change the model wants to make.
func (setDocumentIcon) Confirm(inp DescribeInput) ConfirmActionSummary {
	var in struct {
		Icon string `json:"icon"`
	}

	inp.Probe(&in)

	return summarize(inp, NameSetDocumentIcon, func(subject string) string {
		if in.Icon == "" {
			return "Change icon of " + subject
		}

		return fmt.Sprintf("Change icon of %s to %s", subject, in.Icon)
	})
}

// Execute sets the icon and refreshes the tree.
func (setDocumentIcon) Execute(inp Input) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Icon       string `json:"icon"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Icon == "" {
		return "", errors.New("set_document_icon: icon is required")
	}

	out, err := inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.SetIcon(in.Icon)})
	if err != nil {
		return "", err
	}

	inp.NotifyTreeChangeForDocument(in.DocumentID)

	return out, nil
}

// moveDocument re-parents a document within the organisation.
type moveDocument struct{}

// Info returns the tool's model-facing description.
func (moveDocument) Info() Info {
	return Info{
		Name:        NameMoveDocument,
		Description: "Re-parent a document. Omit new_parent_id to move the document to the org root.",
		Properties: map[string]any{
			_keyDocumentID:  stringProp("The document to move."),
			"new_parent_id": stringProp("Optional new parent document id; omit to move to the root."),
		},
		Required: []string{_keyDocumentID},
	}
}

// Traits reports a write.
func (moveDocument) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being moved.
func (moveDocument) Title(inp DescribeInput) string {
	return "Moving " + inp.Subject()
}

// Confirm describes the move the model wants to make.
func (moveDocument) Confirm(inp DescribeInput) ConfirmActionSummary {
	var in struct {
		NewParentID string `json:"new_parent_id"`
	}

	inp.Probe(&in)

	return summarize(inp, NameMoveDocument, func(subject string) string {
		if in.NewParentID == "" {
			return fmt.Sprintf("Move %s to the org root", subject)
		}

		return fmt.Sprintf("Move %s under another document", subject)
	})
}

// Execute re-parents the document, refusing to create a cycle.
func (moveDocument) Execute(inp Input) (string, error) {
	var in struct {
		DocumentID  string `json:"document_id"`
		NewParentID string `json:"new_parent_id"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("move_document: document_id is not a valid xid: %w", err)
	}

	doc, err := inp.Document(docID)
	if err != nil {
		return "", fmt.Errorf("move_document: document not found: %w", err)
	}

	oldParent := doc.ParentID

	var newParent null.Value[xid.ID]

	if in.NewParentID != "" {
		pid, perr := xid.FromString(in.NewParentID)
		if perr != nil {
			return "", fmt.Errorf("move_document: new_parent_id is not a valid xid: %w", perr)
		}

		newParent = null.ValueFrom(pid)
	}

	if err := inp.MoveDocument(docID, newParent); err != nil {
		return "", fmt.Errorf("move_document: %w", err)
	}

	// both the source and destination subtrees changed shape; tell
	// subscribers about each. When the move is within the same parent
	// the second notification would be a duplicate, so it is skipped.
	inp.NotifyTreeChange(oldParent)

	if oldParent != newParent {
		inp.NotifyTreeChange(newParent)
	}

	out := struct {
		DocumentID  string `json:"document_id"`
		NewParentID string `json:"new_parent_id,omitempty"`
	}{
		DocumentID: docID.String(),
	}

	if newParent.Valid {
		out.NewParentID = newParent.V.String()
	}

	return result(out)
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
