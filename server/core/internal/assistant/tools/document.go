package tools

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/icon"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _defaultDocumentIcon is the icon a document created without one gets.
const _defaultDocumentIcon = "mingcute:document-2-fill"

// listDocuments returns the organisation's document tree.
type listDocuments struct {
	// ParentID narrows the listing to one parent's children. Null
	// lists the whole tree.
	ParentID null.Value[xid.ID] `json:"parent_id"`
}

// Info returns the tool's model-facing description.
func (listDocuments) Info() Info {
	return Info{
		Name:        NameListDocuments,
		Description: "List the organisation's documents as a tree of {id, name, branches: [{id, name, protected, default}], children}. parent_id narrows it to that document's children. search_documents finds a document by title or content.",
		Properties: map[string]any{
			"parent_id": map[string]any{"type": "string", "description": "Optional. A document whose direct children to list; omit it for the whole tree."},
		},
	}
}

// Validate accepts every payload: nothing is required.
func (listDocuments) Validate() error {
	return nil
}

// Exec lists the documents the model asked for.
func (ld listDocuments) Exec(inp Input) (any, error) {
	var (
		tree document.Summaries
		err  error
	)

	if ld.ParentID.Valid {
		tree, err = inp.FetchDocumentChildren(ld.ParentID)
	} else {
		tree, err = inp.FetchDocumentTree()
	}

	if err != nil {
		return nil, fmt.Errorf("fetching tree: %w", err)
	}

	branches, err := inp.FetchBranchesByDocument()
	if err != nil {
		return nil, err
	}

	return listDocumentsResult{
		Documents: newDocTreeNodes(tree, branches),
	}, nil
}

// listDocumentsResult is what list_documents returns.
type listDocumentsResult struct {
	// Documents is the organisation's document tree, or the children
	// of the parent the call named.
	Documents []docTreeNode `json:"documents"`
}

// docTreeNode is the shape returned by list_documents: a document with
// every branch it has, so a caller can pick a branch, and see which ones
// refuse writes, without reading each document first.
type docTreeNode struct {
	ID       xid.ID        `json:"id"`
	Name     string        `json:"name"`
	Branches []branchInfo  `json:"branches"`
	Children []docTreeNode `json:"children,omitempty"`
}

// newDocTreeNodes converts the document package's nested Summary tree
// into list_documents' shape, with each document's branches taken from
// branches, keyed by document.
func newDocTreeNodes(ss document.Summaries, branches map[xid.ID][]document.BranchSummary) []docTreeNode {
	out := make([]docTreeNode, 0, len(ss))

	for _, s := range ss {
		node := docTreeNode{
			ID:       s.ID,
			Name:     s.DocumentName,
			Branches: make([]branchInfo, 0, len(branches[s.ID])),
			Children: newDocTreeNodes(s.Children, branches),
		}

		for _, b := range branches[s.ID] {
			node.Branches = append(node.Branches, branchInfo{
				ID:        b.BranchID,
				Name:      b.BranchName,
				Protected: b.Protected,
				Default:   b.Default,
			})
		}

		out = append(out, node)
	}

	return out
}

// getDocument returns one document: its metadata and its content as
// markup.
type getDocument struct {
	docTarget

	// BlockUID narrows the content to one block. Optional.
	BlockUID string `json:"block_uid"`
}

// Info returns the tool's model-facing description.
func (getDocument) Info() Info {
	return Info{
		Name:        NameGetDocument,
		Description: "Read one document on one branch: its name, icon, parent_id and updated_at, every branch it has as {id, name, protected, default, updated_at}, the tags on the branch read, and content: its blocks as XML, one element per block with its id, in the format insert_blocks describes. Read a document this way before editing it.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "Optional. Narrows content to this block and what it holds; an id inside a block, such as a search hit's, reads the block holding it."},
		},
		Required: []string{"document_id", "branch_id"},
	}
}

// Title announces which document is being read.
func (gd getDocument) Title(inp DescribeInput) string {
	doc, err := inp.FetchBranch(gd.DocumentID, gd.BranchID)
	if err != nil {
		return ""
	}

	return "Reading " + doc.Title()
}

// Exec fetches the branch and summarises its content. The document
// fetch carries the branch's content, so no second read is needed.
func (gd getDocument) Exec(inp Input) (any, error) {
	doc, err := inp.FetchBranch(gd.DocumentID, gd.BranchID)
	if err != nil {
		return nil, fmt.Errorf("fetching document: %w", err)
	}

	branches, err := inp.FetchDocumentBranches(gd.DocumentID)
	if err != nil {
		return nil, err
	}

	tags, err := inp.FetchBranchTags(gd.DocumentID, gd.BranchID)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	blocks := doc.Content.Content

	if gd.BlockUID != "" {
		b, ok := markup.Readable(blocks, gd.BlockUID)
		if !ok {
			return nil, fmt.Errorf("block %s: %w", gd.BlockUID, _errUnknownBlock)
		}

		blocks = []document.Block{b}
	}

	out := getDocumentResult{
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
		out.Tags = append(out.Tags, documentTag{ID: tg.ID, Name: tg.TagName, ColorName: tg.Color})
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

	return out, nil
}

// getDocumentResult is what get_document returns.
type getDocumentResult struct {
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

	// ColorName is the palette name of the tag's colour.
	ColorName string `json:"color"`
}

// createDocument creates a new document in the organisation.
type createDocument struct {
	// Name is the new document's display name. Required.
	Name string `json:"name"`

	// Icon is an icon id the picker offers. Empty falls back to the
	// default icon.
	Icon string `json:"icon"`

	// ParentID names the parent document. Null creates at the org
	// root.
	ParentID null.Value[xid.ID] `json:"parent_id"`
}

// Info returns the tool's model-facing description.
func (createDocument) Info() Info {
	return Info{
		Name:        NameCreateDocument,
		Traits:      Traits{Write: true},
		Description: "Create a document and return {document_id, branch_id}, the branch_id being its default branch. It starts with one empty paragraph; fill it with insert_blocks at position end on that branch.",
		Properties: map[string]any{
			"name":            map[string]any{"type": "string", "description": "The display name."},
			document.AttrIcon: map[string]any{"type": "string", "description": "Optional. An Iconify id from the icon picker, mostly MingCute fills such as mingcute:file-code-fill; another id is refused, naming the closest ones. Defaults to mingcute:document-2-fill."},
			"parent_id":       map[string]any{"type": "string", "description": "Optional. The parent document id; omit it for the organisation root."},
		},
		Required: []string{"name"},
	}
}

// Validate checks the arguments are complete.
func (cd createDocument) Validate() error {
	if cd.Name == "" {
		return errRequired("name")
	}

	if cd.Icon != "" {
		if err := icon.Validate(cd.Icon); err != nil {
			return err
		}
	}

	return cd.input().Validate()
}

// Summary describes the document the model wants to create.
func (cd createDocument) Summary(_ DescribeInput) (ActionSummary, error) {
	// a document that does not exist yet has no id to resolve, so this
	// is the one write whose summary names no target.
	return ActionSummary{
		Tool:    NameCreateDocument,
		Summary: fmt.Sprintf("Create document %q", strings.TrimSpace(cd.Name)),
	}, nil
}

// Exec creates the document, its maintainer row and its search job.
func (cd createDocument) Exec(inp Input) (any, error) {
	doc, err := document.NewDocument(cd.input(), inp.OrganizationID(), inp.UserID())
	if err != nil {
		return nil, err
	}

	if err = inp.CreateDocument(doc); err != nil {
		return nil, err
	}

	return createDocumentResult{
		DocumentID: doc.ID,
		BranchID:   doc.BranchID,
	}, nil
}

// input returns the arguments as the domain's create input, with the
// default icon when none is given.
func (cd createDocument) input() document.CreateInput {
	iconID := cd.Icon
	if iconID == "" {
		iconID = _defaultDocumentIcon
	}

	return document.CreateInput{Name: cd.Name, Icon: iconID, ParentID: cd.ParentID}
}

// createDocumentResult is what create_document returns.
type createDocumentResult struct {
	// DocumentID addresses the new document in every later call.
	DocumentID xid.ID `json:"document_id"`

	// BranchID is the new document's default branch.
	BranchID xid.ID `json:"branch_id"`
}

// deleteDocument removes a document from the organisation.
type deleteDocument struct {
	// DocumentID names the document being deleted.
	DocumentID xid.ID `json:"document_id"`
}

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

// Validate checks the arguments are complete.
func (dd deleteDocument) Validate() error {
	if dd.DocumentID.IsNil() {
		return errRequired("document_id")
	}

	return nil
}

// Summary describes the document the model wants to delete.
func (dd deleteDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchDocument(dd.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	summary := "Delete " + doc.DocumentName

	// the delete cascades, so a card naming only the document would
	// have the user approve a subtree they were never shown.
	if n, err := inp.DescendantCount(dd.DocumentID); err == nil && n > 0 {
		summary += fmt.Sprintf(" and the %s nested under it", countPhrase(n, "page"))
	}

	return ActionSummary{
		Tool:         NameDeleteDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Exec deletes the document and refreshes the tree.
func (dd deleteDocument) Exec(inp Input) (any, error) {
	if err := inp.DeleteDocument(dd.DocumentID); err != nil {
		return nil, fmt.Errorf("deleting document: %w", err)
	}

	return deleteDocumentResult{
		DocumentID: dd.DocumentID,
		Deleted:    true,
	}, nil
}

// deleteDocumentResult is what delete_document returns.
type deleteDocumentResult struct {
	// DocumentID is the document that was removed.
	DocumentID xid.ID `json:"document_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// updateDocument changes a document's name, icon or position in the
// tree, any combination in one call.
type updateDocument struct {
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

// Info returns the tool's model-facing description.
func (updateDocument) Info() Info {
	return Info{
		Name:        NameUpdateDocument,
		Traits:      Traits{Write: true, Idempotent: true},
		Description: "Change a document's name, icon or parent; give only what changes, and a call changing nothing is refused. Content is untouched, and a parent that is the document itself or one of its descendants is refused. Returns {document_id} with what changed.",
		Properties: map[string]any{
			"document_id":     map[string]any{"type": "string", "description": _documentIDDescription},
			"name":            map[string]any{"type": "string", "description": "Optional. The new display name."},
			document.AttrIcon: map[string]any{"type": "string", "description": "Optional. An Iconify id from the icon picker, mostly MingCute fills such as mingcute:rocket-fill; another id is refused, naming the closest ones."},
			"parent_id":       map[string]any{"type": "string", "description": "Optional. The new parent document id, or an empty string for the organisation root."},
		},
		Required: []string{"document_id"},
	}
}

// Validate checks the arguments are complete and consistent.
func (ud updateDocument) Validate() error {
	if ud.DocumentID.IsNil() {
		return errRequired("document_id")
	}

	if ud.Name == "" && ud.Icon == "" && ud.ParentID == nil {
		return errors.New("give at least one of name, icon or parent_id")
	}

	if ud.Name != "" {
		if err := document.ValidateName(strings.TrimSpace(ud.Name)); err != nil {
			return err
		}
	}

	if ud.Icon != "" {
		if err := icon.Validate(ud.Icon); err != nil {
			return err
		}
	}

	if ud.ParentID != nil && *ud.ParentID != "" {
		if _, err := xid.FromString(*ud.ParentID); err != nil {
			return fmt.Errorf("parent_id: %w", err)
		}
	}

	return nil
}

// Summary lists exactly the changes the model asked for.
func (ud updateDocument) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchDocument(ud.DocumentID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	return ActionSummary{
		Tool:         NameUpdateDocument,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      joinClauses(ud.changes(doc.DocumentName)),
	}, nil
}

// Exec applies the name and icon through the live document, then
// re-parents it, and tells the tree subscribers what moved.
func (ud updateDocument) Exec(inp Input) (any, error) {
	name := strings.TrimSpace(ud.Name)

	doc, err := inp.FetchDocument(ud.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("fetching document: %w", err)
	}

	if err := inp.RenameDocument(doc, name, ud.Icon); err != nil {
		return nil, err
	}

	if parent, moved := ud.parent(); moved {
		if err := inp.MoveDocument(doc, parent); err != nil {
			return nil, err
		}
	}

	out := updateDocumentResult(ud)
	out.Name = name

	return out, nil
}

// parent returns the destination the arguments name, and whether they
// name one at all.
func (ud updateDocument) parent() (null.Value[xid.ID], bool) {
	if ud.ParentID == nil {
		return null.Value[xid.ID]{}, false
	}

	if *ud.ParentID == "" {
		return null.Value[xid.ID]{}, true
	}

	// Validate has already parsed it.
	id, _ := xid.FromString(*ud.ParentID) //nolint:errcheck // the id was validated before the arguments were accepted

	return null.ValueFrom(id), true
}

// changes lists the requested changes in the words the confirm card
// uses, in a fixed order so the same request always reads the same.
func (ud updateDocument) changes(currentName string) []string {
	var out []string

	if ud.Name != "" {
		out = append(out, fmt.Sprintf("rename %s to %q", currentName, strings.TrimSpace(ud.Name)))
	}

	if ud.Icon != "" {
		out = append(out, "set the icon to "+ud.Icon)
	}

	if parent, ok := ud.parent(); ok {
		if parent.Valid {
			out = append(out, "move it under another document")
		} else {
			out = append(out, "move it to the org root")
		}
	}

	return out
}

// updateDocumentResult is what update_document returns: the document
// and the fields that changed.
type updateDocumentResult struct {
	// DocumentID is the document that was changed.
	DocumentID xid.ID `json:"document_id"`

	// Name is the new display name, when one was set.
	Name string `json:"name,omitempty"`

	// Icon is the new icon identifier, when one was set.
	Icon string `json:"icon,omitempty"`

	// ParentID is the new parent, when one was set; empty is the root.
	ParentID *string `json:"parent_id,omitempty"`
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

// DocumentInput is what the document and block tools reach on top of
// DescribeInput.
type DocumentInput interface {
	// FetchDocumentTree should return the organisation's document tree.
	FetchDocumentTree() (document.Summaries, error)

	// FetchDocumentChildren should return the direct children of the
	// parent, or the root documents when it is null.
	FetchDocumentChildren(parentID null.Value[xid.ID]) (document.Summaries, error)

	// FetchDocumentBranches should return the branches of the document.
	FetchDocumentBranches(documentID xid.ID) ([]document.BranchSummary, error)

	// FetchBranchesByDocument should return every branch of the
	// organisation, keyed by document.
	FetchBranchesByDocument() (map[xid.ID][]document.BranchSummary, error)

	// FetchDocumentBlock should return the block the uid names on the
	// branch.
	FetchDocumentBlock(documentID, branchID xid.ID, blockUID string) (document.Block, error)

	// CreateDocument should store the new document.
	CreateDocument(doc document.Document) error

	// DeleteDocument should delete the document and everything under it.
	DeleteDocument(id xid.ID) error

	// MoveDocument should move the document under the parent, or to the
	// root when it is null.
	MoveDocument(doc *document.Document, parentID null.Value[xid.ID]) error

	// RenameDocument should set the document's name and icon.
	RenameDocument(doc *document.Document, name, iconID string) error

	// ApplyEdit should apply the operations to the branch.
	ApplyEdit(documentID, branchID xid.ID, ops []edit.Operation) error
}
