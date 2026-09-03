package tools

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _defaultDocumentIcon is the icon assigned to assistant-created
// documents when the model doesn't pick one. It matches what the
// editor gives a document created by hand, so a document's origin is
// not visible in the sidebar.
const _defaultDocumentIcon = "mingcute:document-2-fill"

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
		Description: "List the organisation's documents as a tree of {id, name, children}. Use it to find a document by name or to see what sits under a parent; use search_documents when you are looking for content rather than a title. Pass parent_id to get only that document's direct children, or omit it for the whole organisation.",
		Properties: map[string]any{
			"parent_id": stringProp("Optional. Return only the direct children of this document. Omit for the full tree."),
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

// getDocument returns one document: its metadata and the ordered rows
// of its content.
type getDocument struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (getDocument) Info() Info {
	return Info{
		Name:        NameGetDocument,
		Description: "Read one document: its name, icon, parent_id, protected flag and updated_at, followed by one row per block with the block's uid, kind, flattened text, depth, parent_uid and the few attrs that matter for reading (heading level, callout icon, code language, task checked). Use it as the way to read a document before editing it; protected true means every write is refused. Rows marked has_children hold nested blocks the rows do not list, and read_block returns those when you need them.",
		Properties:  documentIDProp(_descDocumentID),
		Required:    []string{_keyDocumentID},
	}
}

// Traits reports a plain read.
func (getDocument) Traits() Traits {
	return Traits{}
}

// Title announces which document is being read.
func (getDocument) Title(inp DescribeInput) (string, error) {
	var in getDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameGetDocument, err)
	}

	return "Reading " + doc.DocumentName, nil
}

// Execute fetches the document and summarises its default branch.
func (getDocument) Execute(inp Input) (string, error) {
	var in getDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get_document: fetch: %w", err)
	}

	content, err := inp.DocumentContent(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get_document: fetch content: %w", err)
	}

	return result(documentResult{
		DocumentID: doc.ID,
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		ParentID:   doc.ParentID,
		Protected:  doc.Protected,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
		Blocks:     walkDocForAssistant(content.Content.Content),
	})
}

// documentResult is what get_document returns.
type documentResult struct {
	// DocumentID is the document described.
	DocumentID xid.ID `json:"document_id"`

	// Name is the document's display name.
	Name string `json:"name"`

	// Icon is the document's icon identifier.
	Icon string `json:"icon"`

	// ParentID is the document's parent, absent at the root.
	ParentID null.Value[xid.ID] `json:"parent_id,omitzero"`

	// Protected indicates the document refuses every write.
	Protected bool `json:"protected"`

	// UpdatedAt is when the document last changed, as RFC3339.
	UpdatedAt string `json:"updated_at"`

	// Blocks is the document's content, one row per block.
	Blocks []docSummaryEntry `json:"blocks"`
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
		Description: "Create a new document and return {document_id, branch_id}. The document starts with a single empty paragraph, so follow up with insert_block calls (position end) in the same turn to fill it. Omit parent_id to create it at the organisation root.",
		Properties: map[string]any{
			_keyName:          stringProp("Display name for the new document."),
			document.AttrIcon: stringProp("Iconify identifier, as \"collection:name\". The product's own icons are MingCute fills (e.g. \"mingcute:file-code-fill\"); prefer one so the document matches the rest of the sidebar. Defaults to \"mingcute:document-2-fill\" when empty."),
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
		Description: "Delete a document and every document nested under it. The whole subtree goes and cannot be restored, so check the tree with list_documents first, and use update_document when the aim is to relocate rather than remove. Returns {document_id, deleted}.",
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

	summary := "Delete " + doc.DocumentName

	// the delete cascades, so a card naming only the document would
	// have the user approve a subtree they were never shown.
	if n, err := inp.DescendantCount(in.DocumentID); err == nil && n > 0 {
		summary += fmt.Sprintf(" and the %s nested under it", pluralPages(n))
	}

	return ActionSummary{
		Tool:         NameDeleteDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// pluralPages renders a nested-document count for the confirm card.
func pluralPages(n int) string {
	if n == 1 {
		return "1 page"
	}

	return fmt.Sprintf("%d pages", n)
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

// updateDocumentArgs is what update_document is called with.
type updateDocumentArgs struct {
	// DocumentID names the document being changed.
	DocumentID xid.ID `json:"document_id"`

	// Name is the new display name; empty leaves it unchanged.
	Name string `json:"name"`

	// Icon is the new icon identifier; empty leaves it unchanged.
	Icon string `json:"icon"`

	// ParentID is the new parent: absent leaves the position unchanged,
	// empty moves the document to the organisation root, anything else
	// names the parent document.
	ParentID *string `json:"parent_id"`
}

// Validate checks the arguments are complete and consistent.
func (a updateDocumentArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	if a.Name == "" && a.Icon == "" && a.ParentID == nil {
		return errors.New("update_document needs at least one of name, icon or parent_id")
	}

	if a.ParentID != nil && *a.ParentID != "" {
		if _, err := xid.FromString(*a.ParentID); err != nil {
			return fmt.Errorf("parent_id: %w", err)
		}
	}

	return nil
}

// parent returns the destination the arguments name, and whether they
// name one at all.
func (a updateDocumentArgs) parent() (null.Value[xid.ID], bool) {
	if a.ParentID == nil {
		return null.Value[xid.ID]{}, false
	}

	if *a.ParentID == "" {
		return null.Value[xid.ID]{}, true
	}

	// Validate has already parsed it.
	id, _ := xid.FromString(*a.ParentID) //nolint:errcheck // the id was validated before the arguments were accepted

	return null.ValueFrom(id), true
}

// changes lists the requested changes in the words the confirm card
// uses, in a fixed order so the same request always reads the same.
func (a updateDocumentArgs) changes(currentName string) []string {
	var out []string

	if a.Name != "" {
		out = append(out, fmt.Sprintf("rename %s to %q", currentName, a.Name))
	}

	if a.Icon != "" {
		out = append(out, "set the icon to "+a.Icon)
	}

	if parent, ok := a.parent(); ok {
		if parent.Valid {
			out = append(out, "move it under another document")
		} else {
			out = append(out, "move it to the org root")
		}
	}

	return out
}

// updateDocument changes a document's name, icon or position in the
// tree, any combination in one call.
type updateDocument struct{}

// Info returns the tool's model-facing description.
func (updateDocument) Info() Info {
	return Info{
		Name:        NameUpdateDocument,
		Description: "Change a document's name, icon or place in the tree, any combination in one call; content is untouched. Give only the fields to change: name is the new display name, icon an Iconify identifier as \"collection:name\" (the product's own icons are MingCute fills, e.g. \"mingcute:rocket-fill\", so prefer one to match the sidebar), and parent_id the new parent, or an empty string for the organisation root. A call with none of the three is refused, and a parent that is the document itself or one of its descendants fails. Returns {document_id} with the fields that changed.",
		Properties: map[string]any{
			_keyDocumentID:    stringProp(_descDocumentID),
			_keyName:          stringProp("Optional. The new display name; omit to keep the current one."),
			document.AttrIcon: stringProp("Optional. The new icon identifier; omit to keep the current one."),
			"parent_id":       stringProp("Optional. The new parent document id, or an empty string to move the document to the organisation root; omit to leave it where it is."),
		},
		Required: []string{_keyDocumentID},
	}
}

// Traits reports a write.
func (updateDocument) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (updateDocument) Title(inp DescribeInput) (string, error) {
	var in updateDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameUpdateDocument, err)
	}

	return "Updating " + doc.DocumentName, nil
}

// Summary lists exactly the changes the model asked for.
func (updateDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameUpdateDocument, err)
	}

	changes := in.changes(doc.DocumentName)
	summary := strings.ToUpper(changes[0][:1]) + changes[0][1:]

	if len(changes) > 1 {
		summary = fmt.Sprintf("%s and %s", strings.Join(changes[:len(changes)-1], ", "), changes[len(changes)-1])
		summary = strings.ToUpper(summary[:1]) + summary[1:]
	}

	return ActionSummary{
		Tool:         NameUpdateDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute applies the name and icon through the live document, then
// re-parents it, and tells the tree subscribers what moved.
func (updateDocument) Execute(inp Input) (string, error) {
	var in updateDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Document(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("update_document: fetch document: %w", err)
	}

	var ops []edit.Operation

	if in.Name != "" {
		ops = append(ops, edit.SetName(in.Name))
	}

	if in.Icon != "" {
		ops = append(ops, edit.SetIcon(in.Icon))
	}

	if len(ops) > 0 {
		if err := inp.ApplyEdit(in.DocumentID, ops); err != nil {
			return "", err
		}
	}

	parent, moved := in.parent()
	if moved {
		if err := inp.MoveDocument(in.DocumentID, parent); err != nil {
			return "", fmt.Errorf("update_document: %w", err)
		}
	}

	// a rename or icon change shows in the document's own row; a move
	// changes the shape of the source and destination subtrees, and
	// when those are the same parent one notification covers both.
	switch {
	case moved && doc.ParentID != parent:
		inp.NotifyTreeChange(doc.ParentID)
		inp.NotifyTreeChange(parent)
	case moved:
		inp.NotifyTreeChange(parent)
	default:
		inp.NotifyTreeChangeForDocument(in.DocumentID)
	}

	return result(updatedDocumentResult(in))
}

// updatedDocumentResult is what update_document returns: the document
// and the fields that changed.
type updatedDocumentResult struct {
	// DocumentID is the document that was changed.
	DocumentID xid.ID `json:"document_id"`

	// Name is the new display name, when one was set.
	Name string `json:"name,omitempty"`

	// Icon is the new icon identifier, when one was set.
	Icon string `json:"icon,omitempty"`

	// ParentID is the new parent, when one was set; empty is the root.
	ParentID *string `json:"parent_id,omitempty"`
}

// docTreeNode is the shape returned by list_documents. It mirrors
// document.Summary but uses snake_case keys so the AI consumes a
// consistent vocabulary with the rest of the tool surface.
type docTreeNode struct {
	ID       xid.ID        `json:"id"`
	Name     string        `json:"name"`
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
			Children: summariesToTree(s.Children),
		})
	}

	return out
}
