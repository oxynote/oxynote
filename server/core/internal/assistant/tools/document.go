package tools

import (
	"cmp"
	"errors"
	"fmt"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/rs/xid"
)

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
type listDocuments struct{}

// Info returns the tool's model-facing description.
func (listDocuments) Info() Info {
	return Info{
		Name:        NameListDocuments,
		Description: "List the organisation's documents as a tree of {id, name, default_branch_id, children}. Use it to find a document by title; search_documents finds content. parent_id narrows it to that document's children.",
		Properties: map[string]any{
			"parent_id": map[string]any{"type": "string", "description": "Optional. A document whose direct children to list; omit it for the whole tree."},
		},
	}
}

// Execute lists the documents the model asked for.
func (listDocuments) Execute(inp *input) (string, error) {
	var in listDocumentsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	var (
		tree document.Summaries
		err  error
	)

	if in.ParentID.Valid {
		tree, err = inp.FetchDocumentChildren(in.ParentID)
	} else {
		tree, err = inp.FetchDocumentTree()
	}

	if err != nil {
		return "", fmt.Errorf("fetch tree: %w", err)
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
	docTarget

	// BlockUID narrows the content to one block. Optional.
	BlockUID string `json:"block_uid"`
}

// getDocument returns one document: its metadata and its content as
// markup.
type getDocument struct{}

// Info returns the tool's model-facing description.
func (getDocument) Info() Info {
	return Info{
		Name:        NameGetDocument,
		Description: "Read one document on one branch: its name, icon, parent_id and updated_at, every branch it has as {id, name, protected, default, updated_at}, the tags on the branch read, and content: its blocks as XML, one element per block with its id, in the format insert_blocks describes. Read a document this way before editing it. A protected branch refuses writes, so write to an unprotected one.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "Optional. Narrows content to this block and what it holds; an id inside a block, such as a search hit's, reads the block holding it."},
		},
		Required: []string{"document_id", "branch_id"},
	}
}

// Title announces which document is being read.
func (getDocument) Title(inp DescribeInput) string {
	var in getDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ""
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ""
	}

	return "Reading " + doc.Title()
}

// Execute fetches the branch and summarises its content. The document
// fetch carries the branch's content, so no second read is needed.
func (getDocument) Execute(inp *input) (string, error) {
	var in getDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}

	branches, err := inp.FetchDocumentBranches(in.DocumentID)
	if err != nil {
		return "", err
	}

	tags, err := inp.FetchBranchTags(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("fetch tags: %w", err)
	}

	blocks := doc.Content.Content

	if in.BlockUID != "" {
		b, ok := markup.Readable(blocks, in.BlockUID)
		if !ok {
			return "", fmt.Errorf("block %s: %w", in.BlockUID, errUnknownBlock)
		}

		blocks = []document.Block{b}
	}

	out := documentResult{
		DocumentID: doc.ID,
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		ParentID:   doc.ParentID,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
		Branches:   make([]branchInfo, 0, len(branches)),
		Tags:       make([]documentTag, 0, len(tags)),
		Content:    markup.Render(blocks),
	}

	for _, tg := range tags {
		out.Tags = append(out.Tags, documentTag{ID: tg.ID, Name: tg.TagName, ColorName: cmp.Or(tag.ColorName(tg.Color), "unknown")})
	}

	for _, b := range branches {
		out.Branches = append(out.Branches, branchInfo{
			ID:        b.BranchID,
			Name:      b.BranchName,
			Protected: b.Protected,
			Default:   b.Default,
			UpdatedAt: b.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	return result(out)
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

	// UpdatedAt is when the branch read last changed, as RFC3339.
	UpdatedAt string `json:"updated_at"`

	// Branches is every branch the document has, the one read among
	// them.
	Branches []branchInfo `json:"branches"`

	// Tags is every tag the branch read carries.
	Tags []documentTag `json:"tags"`

	// Content is the branch's blocks, or the one asked for, as markup.
	Content string `json:"content"`
}

// documentTag is one tag row of get_document.
type documentTag struct {
	// ID is the tag id, which is what a tool's tag_id takes.
	ID xid.ID `json:"id"`

	// Name is the tag's display name.
	Name string `json:"name"`

	// ColorName is the palette name of the tag's colour, or "unknown" as
	// list_tags reports it.
	ColorName string `json:"color"`
}

// branchInfo describes one branch of a document to the model.
type branchInfo struct {
	// ID is the branch id, which is what a tool's branch_id takes.
	ID xid.ID `json:"id"`

	// Name is the branch name, which is what a person calls it.
	Name string `json:"name"`

	// Protected indicates the branch refuses every write.
	Protected bool `json:"protected"`

	// Default indicates this is the branch a call without a branch
	// name reads or writes.
	Default bool `json:"default"`

	// UpdatedAt is when the branch last changed, as RFC3339; empty on
	// the branch read, whose time the result carries already.
	UpdatedAt string `json:"updated_at,omitempty"`
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
		return errRequired("name")
	}

	return nil
}

// Info returns the tool's model-facing description.
func (createDocument) Info() Info {
	return Info{
		Name:        NameCreateDocument,
		Traits:      Traits{Write: true},
		Description: "Create a document and return {document_id, branch_id}, the branch_id being its default branch. It starts with one empty paragraph; fill it with insert_blocks at position end on that branch.",
		Properties: map[string]any{
			"name":            map[string]any{"type": "string", "description": "The display name."},
			document.AttrIcon: map[string]any{"type": "string", "description": "Optional. An Iconify id; the sidebar uses MingCute fills, such as mingcute:file-code-fill. Defaults to mingcute:document-2-fill."},
			"parent_id":       map[string]any{"type": "string", "description": "Optional. The parent document id; omit it for the organisation root."},
		},
		Required: []string{"name"},
	}
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
func (createDocument) Execute(inp *input) (string, error) {
	var in createDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	icon := in.Icon
	if icon == "" {
		icon = "mingcute:document-2-fill"
	}

	doc := document.NewDocument(document.CreateInput{
		Name:     in.Name,
		Icon:     icon,
		ParentID: in.ParentID,
	}, inp.orgID, inp.userID)

	if err := inp.CreateDocument(doc); err != nil {
		return "", err
	}

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
		return errRequired("document_id")
	}

	return nil
}

// deleteDocument removes a document from the organisation.
type deleteDocument struct{}

// Info returns the tool's model-facing description.
func (deleteDocument) Info() Info {
	return Info{
		Name:        NameDeleteDocument,
		Traits:      Traits{Write: true, Destructive: true},
		Description: "Delete a document and every document nested under it. The whole subtree goes and cannot be restored, so check the tree with list_documents first, and use update_document when the aim is to relocate rather than remove. Returns {document_id, deleted}.",
		Properties:  map[string]any{"document_id": map[string]any{"type": "string", "description": _documentIDDescription}},
		Required:    []string{"document_id"},
	}
}

// Summary describes the document the model wants to delete.
func (deleteDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	summary := "Delete " + doc.DocumentName

	// the delete cascades, so a card naming only the document would
	// have the user approve a subtree they were never shown.
	if n, err := inp.DescendantCount(in.DocumentID); err == nil && n > 0 {
		summary += fmt.Sprintf(" and the %s nested under it", countPhrase(n, "page"))
	}

	return ActionSummary{
		Tool:         NameDeleteDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute deletes the document and refreshes the tree.
func (deleteDocument) Execute(inp *input) (string, error) {
	var in deleteDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.DeleteDocument(in.DocumentID); err != nil {
		return "", fmt.Errorf("delete: %w", err)
	}

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
		return errRequired("document_id")
	}

	if a.Name == "" && a.Icon == "" && a.ParentID == nil {
		return errors.New("give at least one of name, icon or parent_id")
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
		Traits:      Traits{Write: true},
		Description: "Change a document's name, icon or parent; give only what changes, and a call changing nothing is refused. Content is untouched, and a parent that is the document itself or one of its descendants is refused. Returns {document_id} with what changed.",
		Properties: map[string]any{
			"document_id":     map[string]any{"type": "string", "description": _documentIDDescription},
			"name":            map[string]any{"type": "string", "description": "Optional. The new display name."},
			document.AttrIcon: map[string]any{"type": "string", "description": "Optional. An Iconify id; the sidebar uses MingCute fills, such as mingcute:rocket-fill."},
			"parent_id":       map[string]any{"type": "string", "description": "Optional. The new parent document id, or an empty string for the organisation root."},
		},
		Required: []string{"document_id"},
	}
}

// Summary lists exactly the changes the model asked for.
func (updateDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	return ActionSummary{
		Tool:         NameUpdateDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      sentence(in.changes(doc.DocumentName)),
	}, nil
}

// Execute applies the name and icon through the live document, then
// re-parents it, and tells the tree subscribers what moved.
func (updateDocument) Execute(inp *input) (string, error) {
	var in updateDocumentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("fetch document: %w", err)
	}

	if err := inp.RenameDocument(doc, in.Name, in.Icon); err != nil {
		return "", err
	}

	if parent, moved := in.parent(); moved {
		if err := inp.MoveDocument(doc, parent); err != nil {
			return "", err
		}
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
	ID              xid.ID        `json:"id"`
	Name            string        `json:"name"`
	DefaultBranchID xid.ID        `json:"default_branch_id"`
	Children        []docTreeNode `json:"children,omitempty"`
}

// summariesToTree converts the document package's nested Summary tree
// into the snake_case shape returned by list_documents.
func summariesToTree(ss document.Summaries) []docTreeNode {
	out := make([]docTreeNode, 0, len(ss))

	for _, s := range ss {
		out = append(out, docTreeNode{
			ID:              s.ID,
			Name:            s.DocumentName,
			DefaultBranchID: s.DefaultBranchID,
			Children:        summariesToTree(s.Children),
		})
	}

	return out
}
