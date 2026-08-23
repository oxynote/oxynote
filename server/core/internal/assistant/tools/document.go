package tools

import (
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

// listDocumentsArgs is what list_documents is called with.
type listDocumentsArgs struct {
	// ParentID narrows the listing to one parent's children. Null
	// lists the whole tree.
	ParentID null.Value[xid.ID] `json:"parent_id"`
}

// Validate accepts every payload: nothing is required.
func (listDocumentsArgs) Validate() error {
	return nil
}

// listDocuments returns the organisation's document tree.
type listDocuments struct {
	plainSummary
}

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
func (listDocuments) Title(_ DescribeInput) (string, error) {
	return "", nil
}

// Execute lists the documents the model asked for.
func (listDocuments) Execute(inp Input) (string, error) {
	var in listDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	var (
		tree document.Summaries
		err  error
	)

	if in.ParentID.Valid {
		tree, err = inp.DocumentChildren(in.ParentID)
	} else {
		tree, err = inp.DocumentTree()
	}

	if err != nil {
		return "", fmt.Errorf("list_documents: fetch tree: %w", err)
	}

	return result(documentTreeResult{
		Documents: summariesToTree(tree),
	})
}

// documentTreeResult is what list_documents returns.
type documentTreeResult struct {
	// Documents is the organisation's document tree, or the children
	// of the parent the call named.
	Documents []docTreeNode `json:"documents"`
}

// createdDocumentResult is what create_document returns.
type createdDocumentResult struct {
	// DocumentID addresses the new document in every later call.
	DocumentID xid.ID `json:"document_id"`

	// BranchID is the new document's default branch.
	BranchID xid.ID `json:"branch_id"`
}

// deletedDocumentResult is what delete_document returns.
type deletedDocumentResult struct {
	// DocumentID is the document that was removed.
	DocumentID xid.ID `json:"document_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// getDocumentArgs is what get_document is called with.
type getDocumentArgs struct {
	// DocumentID names the document being described.
	DocumentID xid.ID `json:"document_id"`
}

// Validate checks the arguments are complete.
func (a getDocumentArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	return nil
}

// getDocument returns one document's metadata.
type getDocument struct {
	plainSummary
}

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
func (getDocument) Title(_ DescribeInput) (string, error) {
	return "", nil
}

// Execute fetches the document's metadata.
func (getDocument) Execute(inp Input) (string, error) {
	var in getDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get_document: fetch: %w", err)
	}

	return result(struct {
		ID         xid.ID             `json:"id"`
		Name       string             `json:"name"`
		Icon       string             `json:"icon"`
		ParentID   null.Value[xid.ID] `json:"parent_id,omitzero"`
		BranchID   xid.ID             `json:"branch_id"`
		BranchName string             `json:"branch_name"`
		Protected  bool               `json:"protected"`
		UpdatedAt  string             `json:"updated_at"`
	}{
		ID:         doc.ID,
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		ParentID:   doc.ParentID,
		BranchID:   doc.BranchID,
		BranchName: doc.BranchName,
		Protected:  doc.Protected,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

// createDocument creates a new document in the organisation.
type createDocument struct{}

// createDocumentArgs is what create_document is called with.
type createDocumentArgs struct {
	// Name is the new document's display name. Required.
	Name string `json:"name"`

	// Icon is the lucide icon identifier. Empty falls back to the
	// default icon.
	Icon string `json:"icon"`

	// ParentID names the parent document. Null creates at the org
	// root.
	ParentID null.Value[xid.ID] `json:"parent_id"`
}

// Validate checks the arguments are complete.
func (a createDocumentArgs) Validate() error {
	if a.Name == "" {
		return errRequired(_keyName)
	}

	return nil
}

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
func (createDocument) Title(inp DescribeInput) (string, error) {
	var in createDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return fmt.Sprintf("Creating %q", in.Name), nil
}

// Summary describes the document the model wants to create.
func (createDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	// a document that does not exist yet has no id to resolve, so this
	// is the one write whose summary names no target.
	return ActionSummary{
		Tool:    NameCreateDocument,
		Summary: fmt.Sprintf("Create document %q", in.Name),
	}, nil
}

// Execute creates the document, its maintainer row and its search job.
func (createDocument) Execute(inp Input) (string, error) {
	var in createDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	icon := in.Icon
	if icon == "" {
		icon = _defaultDocumentIcon
	}

	doc := document.NewDocument(document.CreateInput{
		Name:     in.Name,
		Icon:     icon,
		ParentID: in.ParentID,
	}, inp.OrganizationID(), inp.UserID())

	if err := inp.CreateDocument(doc); err != nil {
		return "", fmt.Errorf("create_document: %w", err)
	}

	inp.NotifyTreeChange(doc.ParentID)

	return result(createdDocumentResult{
		DocumentID: doc.ID,
		BranchID:   doc.BranchID,
	})
}

// deleteDocumentArgs is what delete_document is called with.
type deleteDocumentArgs struct {
	// DocumentID names the document being deleted.
	DocumentID xid.ID `json:"document_id"`
}

// Validate checks the arguments are complete.
func (a deleteDocumentArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	return nil
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
func (deleteDocument) Title(inp DescribeInput) (string, error) {
	var in deleteDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameDeleteDocument, err)
	}

	return "Deleting " + doc.DocumentName, nil
}

// Summary describes the document the model wants to delete.
func (deleteDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameDeleteDocument, err)
	}

	return ActionSummary{
		Tool:         NameDeleteDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Delete " + doc.DocumentName,
	}, nil
}

// Execute deletes the document and refreshes the tree.
func (deleteDocument) Execute(inp Input) (string, error) {
	var in deleteDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// capture the parent before the row goes away so we can scope the
	// tree-change notification to the affected subtree.
	var parentID null.Value[xid.ID]

	if doc, ferr := inp.Document(in.DocumentID); ferr == nil && doc != nil {
		parentID = doc.ParentID
	}

	if err := inp.DeleteDocument(in.DocumentID); err != nil {
		return "", fmt.Errorf("delete_document: delete: %w", err)
	}

	inp.NotifyTreeChange(parentID)

	return result(deletedDocumentResult{
		DocumentID: in.DocumentID,
		Deleted:    true,
	})
}

// renameDocumentArgs is what rename_document is called with.
type renameDocumentArgs struct {
	// DocumentID names the document being renamed.
	DocumentID xid.ID `json:"document_id"`

	// Name is the new display name. Required.
	Name string `json:"name"`
}

// Validate checks the arguments are complete.
func (a renameDocumentArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	if a.Name == "" {
		return errRequired(_keyName)
	}

	return nil
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
		Required: []string{
			_keyDocumentID,
			_keyName,
		},
	}
}

// Traits reports a write.
func (renameDocument) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being renamed.
func (renameDocument) Title(inp DescribeInput) (string, error) {
	var in renameDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameRenameDocument, err)
	}

	return "Renaming " + doc.DocumentName, nil
}

// Summary describes the rename the model wants to make.
func (renameDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in renameDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameRenameDocument, err)
	}

	return ActionSummary{
		Tool:         NameRenameDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Rename %s to %q", doc.DocumentName, in.Name),
	}, nil
}

// Execute renames the document and refreshes the tree.
func (renameDocument) Execute(inp Input) (string, error) {
	var in renameDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	out, err := inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.SetName(in.Name)})
	if err != nil {
		return "", err
	}

	inp.NotifyTreeChangeForDocument(in.DocumentID)

	return out, nil
}

// setDocumentIconArgs is what set_document_icon is called with.
type setDocumentIconArgs struct {
	// DocumentID names the document whose icon is being set.
	DocumentID xid.ID `json:"document_id"`

	// Icon is the new lucide icon identifier. Required.
	Icon string `json:"icon"`
}

// Validate checks the arguments are complete.
func (a setDocumentIconArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	if a.Icon == "" {
		return errRequired(document.AttrIcon)
	}

	return nil
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
		Required: []string{
			_keyDocumentID,
			document.AttrIcon,
		},
	}
}

// Traits reports a write.
func (setDocumentIcon) Traits() Traits {
	return Traits{Write: true}
}

// Title returns no status line: an icon change is too minor to announce.
func (setDocumentIcon) Title(_ DescribeInput) (string, error) {
	return "", nil
}

// Summary describes the icon change the model wants to make.
func (setDocumentIcon) Summary(inp DescribeInput) (ActionSummary, error) {
	var in setDocumentIconArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameSetDocumentIcon, err)
	}

	return ActionSummary{
		Tool:         NameSetDocumentIcon,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Change icon of %s to %s", doc.DocumentName, in.Icon),
	}, nil
}

// Execute sets the icon and refreshes the tree.
func (setDocumentIcon) Execute(inp Input) (string, error) {
	var in setDocumentIconArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	out, err := inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.SetIcon(in.Icon)})
	if err != nil {
		return "", err
	}

	inp.NotifyTreeChangeForDocument(in.DocumentID)

	return out, nil
}

// moveDocumentArgs is what move_document is called with.
type moveDocumentArgs struct {
	// DocumentID names the document being moved.
	DocumentID xid.ID `json:"document_id"`

	// NewParentID names the destination parent. Null moves the
	// document to the org root.
	NewParentID null.Value[xid.ID] `json:"new_parent_id"`
}

// Validate checks the arguments are complete.
func (a moveDocumentArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	return nil
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
func (moveDocument) Title(inp DescribeInput) (string, error) {
	var in moveDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameMoveDocument, err)
	}

	return "Moving " + doc.DocumentName, nil
}

// Summary describes the move the model wants to make.
func (moveDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in moveDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameMoveDocument, err)
	}

	summary := fmt.Sprintf("Move %s under another document", doc.DocumentName)
	if !in.NewParentID.Valid {
		summary = fmt.Sprintf("Move %s to the org root", doc.DocumentName)
	}

	return ActionSummary{
		Tool:         NameMoveDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute re-parents the document, refusing to create a cycle.
func (moveDocument) Execute(inp Input) (string, error) {
	var in moveDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("move_document: document not found: %w", err)
	}

	oldParent := doc.ParentID

	if err := inp.MoveDocument(in.DocumentID, in.NewParentID); err != nil {
		return "", fmt.Errorf("move_document: %w", err)
	}

	// both the source and destination subtrees changed shape; tell
	// subscribers about each. When the move is within the same parent
	// the second notification would be a duplicate, so it is skipped.
	inp.NotifyTreeChange(oldParent)

	if oldParent != in.NewParentID {
		inp.NotifyTreeChange(in.NewParentID)
	}

	return result(struct {
		DocumentID  xid.ID             `json:"document_id"`
		NewParentID null.Value[xid.ID] `json:"new_parent_id,omitzero"`
	}{
		DocumentID:  in.DocumentID,
		NewParentID: in.NewParentID,
	})
}

// docTreeNode is the shape returned by list_documents. It mirrors
// document.Summary but uses snake_case keys so the AI consumes a
// consistent vocabulary with the rest of the tool surface.
type docTreeNode struct {
	ID       xid.ID        `json:"id"`
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
			ID:       s.ID,
			Name:     s.DocumentName,
			Icon:     s.Icon,
			Children: summariesToTree(s.Children),
		})
	}

	return out
}
