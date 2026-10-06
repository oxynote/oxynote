package search

import (
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _documentNameUID is the uid of the synthetic entry carrying a branch's
// document name. Content blocks never use it.
const _documentNameUID = "docname"

// Block represents a information block. It can be a paragraph, heading, or
// any other type of content in a document.
type Block struct {
	// ID is the unique identifier for the block.
	ID string `json:"id"`

	// OrganizationID is the identifier for the organization
	// this block belongs to.
	OrganizationID string `json:"organizationId"`

	// DocumentID is the identifier for the document this block belongs to.
	DocumentID xid.ID `json:"documentId"`

	// BranchID is the identifier for the branch this block belongs to.
	BranchID xid.ID `json:"branchId"`

	// BranchName is the name of the branch this block belongs to.
	BranchName string `json:"branchName"`

	// BranchDefault reports whether the branch is the document's default.
	BranchDefault bool `json:"branchDefault"`

	// Type is the type of the block (e.g., "paragraph", "heading").
	Type string `json:"type"`

	// Text is the text content of the block, if applicable.
	Text string `json:"text"`

	// Attrs holds the block's attributes worth keeping for its type, see
	// attrKeys.
	Attrs map[string]string `json:"attrs,omitempty"`
}

// record is the shape a block is written to the index in. bleve walks
// struct fields by reflection and would take an xid's byte array for a
// list of numbers, so the ids are flattened to strings first.
type record struct {
	// OrganizationID specifies the organization the block belongs to.
	OrganizationID string `json:"organizationId"`

	// DocumentID specifies the document the block belongs to.
	DocumentID string `json:"documentId"`

	// BranchID specifies the branch the block belongs to.
	BranchID string `json:"branchId"`

	// BranchName specifies the name of the branch, kept for display only.
	BranchName string `json:"branchName"`

	// BranchDefault indicates whether the branch is the document's default.
	BranchDefault bool `json:"branchDefault"`

	// Type specifies the block type (e.g., "paragraph", "heading").
	Type string `json:"type"`

	// Text specifies the searchable text of the block.
	Text string `json:"text"`

	// Attrs specifies the block's kept attributes, see attrKeys.
	Attrs map[string]string `json:"attrs,omitempty"`
}

// record flattens the block for indexing.
func (b Block) record() record {
	return record{
		OrganizationID: b.OrganizationID,
		DocumentID:     b.DocumentID.String(),
		BranchID:       b.BranchID.String(),
		BranchName:     b.BranchName,
		BranchDefault:  b.BranchDefault,
		Type:           b.Type,
		Text:           b.Text,
		Attrs:          b.Attrs,
	}
}

// Scope is the branch every block of one indexing pass belongs to.
type Scope struct {
	// OrganizationID is the organization owning the document.
	OrganizationID string

	// DocumentID is the document the branch belongs to.
	DocumentID xid.ID

	// BranchID is the branch the blocks are read from.
	BranchID xid.ID

	// BranchName is the branch's name.
	BranchName string

	// BranchDefault reports whether the branch is the document's default.
	BranchDefault bool
}

// Block builds an index entry for uid within the scope. The entry id is
// prefixed with the branch id: a fork copies its source's content, uids
// included, so the uid alone names one block on every branch at once.
func (s Scope) Block(uid, typ, text string) Block {
	return Block{
		ID:             s.BranchID.String() + "-" + uid,
		OrganizationID: s.OrganizationID,
		DocumentID:     s.DocumentID,
		BranchID:       s.BranchID,
		BranchName:     s.BranchName,
		BranchDefault:  s.BranchDefault,
		Type:           typ,
		Text:           text,
	}
}

// entries collects the index entries of the block and its descendants
// into res, keyed by block uid.
func (s Scope) entries(b document.Block, res map[string]Block) {
	var text strings.Builder

	for _, cb := range b.Content {
		// every parent node holding text has a child of the text type,
		// headings, paragraphs, code blocks and list items included.
		if cb.Type == document.BlockNodeText {
			text.WriteString(cb.Text)
			continue
		}

		s.entries(cb, res)
	}

	// a metric, file or image block has no text children; what describes
	// it sits in its attributes.
	switch b.Type { //nolint:exhaustive // the other types index their text children
	case document.BlockNodeMetricBlock:
		if title, ok := b.Attrs[document.AttrTitle].(string); ok {
			text.WriteString(title)
		}
	case document.BlockNodeFileBlock:
		if name, ok := b.Attrs[document.AttrName].(string); ok {
			text.WriteString(name)
		}
	case document.BlockNodeImageBlock:
		alt, _ := b.Attrs[document.AttrAlt].(string)
		title, _ := b.Attrs[document.AttrTitle].(string)

		text.WriteString(strings.TrimSpace(alt + " " + title))
	}

	if text.Len() == 0 {
		return
	}

	id, ok := b.UID()
	if !ok || id == "" {
		return
	}

	e := s.Block(id, string(b.Type), text.String())

	for _, key := range attrKeys(b.Type) {
		// an attribute of another type only describes the block, so it is
		// left out rather than failing the indexing.
		if v, ok := b.Attrs[key].(string); ok && v != "" {
			if e.Attrs == nil {
				e.Attrs = make(map[string]string)
			}

			e.Attrs[key] = v
		}
	}

	res[id] = e
}

// attrKeys returns the attributes an entry of the block type keeps. They
// are stored and indexed as keywords under the attrs field, so a hit can be
// described and filtered without loading its document. Bump
// _mappingVersion when the keys change, so existing entries are rebuilt.
func attrKeys(typ document.BlockNodeType) []string {
	switch typ {
	case document.BlockNodeCodeBlock:
		return []string{document.AttrLanguage}
	case document.BlockNodeMetricBlock:
		return []string{
			document.AttrDataSourceID,
			document.AttrVisualizationType,
		}
	default:
		return nil
	}
}

// Entries converts a document branch into its index entries, keyed by
// block uid. A synthetic entry carries the document name; its key is the
// document id, which no content block shares.
func Entries(doc document.Document) map[string]Block {
	scope := Scope{
		OrganizationID: doc.OrganizationID,
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		BranchName:     doc.BranchName,
		BranchDefault:  doc.Default,
	}

	res := make(map[string]Block)

	for _, b := range doc.Content.Content {
		scope.entries(b, res)
	}

	res[doc.ID.String()] = scope.Block(_documentNameUID, "document", doc.DocumentName)

	return res
}
