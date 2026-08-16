package document

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
	"github.com/rs/xid"
)

const (
	_nodeCommentIDAttr   = "nodeCommentId"
	_nodeUIDAttr         = "uid"
	_nodeSrcAttr         = "src"
	_nodeCommentMarkType = "comment"
)

// FilePathFormat is the URL path format under which document files are
// served. Duplication matches stored src attributes against it, so the
// router mounts the file routes on this very format.
const FilePathFormat = "/api/documents/%s/files/%s"

// RootBlock represents a block in a document.
type RootBlock struct {
	// Type is the type of the root block (always BlockNodeText's
	// containing doc — currently "doc" — for TipTap documents).
	Type BlockNodeType `json:"type"`

	// Content is the content of the root block.
	Content []Block `json:"content,omitempty"`
}

// FindByUID recursively searches the root's subtree for the block
// with the given uid and returns it.
func (rb RootBlock) FindByUID(uid string) (Block, bool) {
	for _, b := range rb.Content {
		if found, ok := b.FindByUID(uid); ok {
			return found, true
		}
	}

	return Block{}, false
}

// HasBlock searches for a block with the given ID in the root block.
func (rb RootBlock) HasBlock(blockID string) bool {
	for _, b := range rb.Content {
		if b.HasBlock(blockID) {
			return true
		}
	}

	return false
}

// Search transforms RootBlock into a search compatible type.
func (rb RootBlock) Search(organizationID string, documentID xid.ID) map[string]search.Block {
	res := make(map[string]search.Block)

	for _, b := range rb.Content {
		content := b.Search(organizationID, documentID)

		maps.Insert(res, maps.All(content))
	}

	return res
}

// Value transforms stopper type into a database entry.
func (rb RootBlock) Value() (driver.Value, error) {
	return json.Marshal(rb)
}

// Scan transforms a database entry into a root block type.
func (rb *RootBlock) Scan(src any) error {
	val, ok := src.([]byte)
	if !ok {
		return errors.New("invalid tiptac document type")
	}

	var nrb RootBlock

	if err := json.Unmarshal(val, &nrb); err != nil {
		return err
	}

	*rb = nrb

	return nil
}

// Block represents a information block. It can be a paragraph, heading, or any other type of
// content in a document.
type Block struct {
	// Type is the ProseMirror node-type tag. Compare against the
	// BlockNode* constants; raw string comparisons won't compile.
	Type BlockNodeType `json:"type"`

	// Text is the text content of the block, if applicable.
	Text string `json:"text,omitempty"`

	// Content is the child blocks of this block, if applicable.
	Content []Block `json:"content,omitempty"`

	// Marks are the marks applied to this block, such as bold, italic, etc.
	Marks []Mark `json:"marks,omitempty"`

	// Attrs are additional attributes for the block, such as alignment, link, etc.
	Attrs Attributes `json:"attrs,omitempty"`
}

// UID returns the block's uid attribute and whether it is present.
// The bool is false when the attribute is missing or not a string;
// callers that need to distinguish "no uid" from "empty uid" should
// branch on it.
func (b Block) UID() (string, bool) {
	id, ok := b.Attrs[_nodeUIDAttr].(string)
	return id, ok
}

// Flatten returns the concatenated text of the block's entire
// subtree. Adjacent text fragments are separated by a single space.
// Used to derive a flat searchable representation from a structured
// ProseMirror tree.
func (b Block) Flatten() string {
	var buf []byte
	b.flattenInto(&buf)

	return string(buf)
}

func (b Block) flattenInto(buf *[]byte) {
	if b.Type == BlockNodeText {
		if b.Text != "" {
			if len(*buf) > 0 && (*buf)[len(*buf)-1] != ' ' {
				*buf = append(*buf, ' ')
			}

			*buf = append(*buf, []byte(b.Text)...)
		}

		return
	}

	for _, c := range b.Content {
		c.flattenInto(buf)
	}
}

// FindByUID recursively searches the block's subtree (self included)
// for the block with the given uid and returns it.
func (b Block) FindByUID(uid string) (Block, bool) {
	if id, ok := b.UID(); ok && id == uid {
		return b, true
	}

	for _, cb := range b.Content {
		if found, ok := cb.FindByUID(uid); ok {
			return found, true
		}
	}

	return Block{}, false
}

// HasBlock recursively searches for a block with the given ID.
func (b Block) HasBlock(blockID string) bool {
	_, ok := b.FindByUID(blockID)

	return ok
}

// Search transforms Block into a search compatible type.
func (b Block) Search(organizationID string, documentID xid.ID) map[string]search.Block {
	res := make(map[string]search.Block)

	var text string

	for _, cb := range b.Content {
		// All parent nodes that contain text have a content element
		// with the text type. This includes Headings, Paragraphs,
		// CodeBlock, CodeBlockTitle, ListItem and so on.
		if cb.Type == BlockNodeText {
			text = cb.Text
			continue
		}

		content := cb.Search(organizationID, documentID)

		maps.Insert(res, maps.All(content))
	}

	if text != "" {
		if id, ok := b.UID(); ok && id != "" {
			res[id] = search.Block{
				ID:             id,
				OrganizationID: organizationID,
				DocumentID:     documentID,
				Type:           string(b.Type),
				Text:           text,
			}
		}
	}

	return res
}

// Mark represents a mark, such as bold or italic.
type Mark struct {
	// Type is the type of the mark, such as "bold", "italic", etc.
	Type string `json:"type"`

	// Attrs are additional attributes for the mark, such as link URL.
	Attrs Attributes `json:"attrs,omitempty"`
}

// StripCommentMarks returns a copy of the RootBlock with all comment marks
// and nodeCommentId attributes removed. Unlike Duplicate, uid attributes are
// preserved as-is.
func (rb RootBlock) StripCommentMarks() RootBlock {
	return rb.copyStripped(nil)
}

// Duplicate creates a copy of the RootBlock with all comment marks
// removed, nodeCommentId attributes removed, and uid attributes regenerated.
// Image blocks pointing at the source document's own files have their src
// rewritten to the duplicate's, and the returned map pairs every such file's
// old id with its new one so the caller can copy the objects themselves.
func (rb RootBlock) Duplicate(oldDocumentID, newDocumentID xid.ID) (RootBlock, map[string]string) {
	dc := &duplication{
		oldDocumentID: oldDocumentID,
		newDocumentID: newDocumentID,
		files:         make(map[string]string),
	}

	return rb.copyStripped(dc), dc.files
}

// RegenerateUIDs returns a copy of the RootBlock with comment marks removed
// and every uid attribute regenerated. Unlike Duplicate it rewrites no file
// references, which suits content templates: they carry none.
func (rb RootBlock) RegenerateUIDs() RootBlock {
	return rb.copyStripped(&duplication{files: make(map[string]string)})
}

// duplication carries the state a block tree needs while it is being
// duplicated: the documents involved and the file ids collected on the way.
type duplication struct {
	// oldDocumentID is the id of the document being duplicated.
	oldDocumentID xid.ID

	// newDocumentID is the id of the document being created.
	newDocumentID xid.ID

	// files maps the source document's file ids to the ids the duplicate
	// refers to them by.
	files map[string]string
}

// copyStripped returns a copy of the RootBlock with comment marks and
// nodeCommentId attributes removed, regenerating uid attributes and
// rewriting file references when a duplication is in progress.
func (rb RootBlock) copyStripped(dc *duplication) RootBlock {
	newContent := make([]Block, len(rb.Content))

	for i, b := range rb.Content {
		newContent[i] = b.copyStripped(dc)
	}

	return RootBlock{
		Type:    rb.Type,
		Content: newContent,
	}
}

// copyStripped returns a copy of the Block with comment marks and
// nodeCommentId attributes removed, regenerating uid attributes and
// rewriting file references when a duplication is in progress.
func (b Block) copyStripped(dc *duplication) Block {
	newBlock := Block{
		Type: b.Type,
		Text: b.Text,
	}

	if len(b.Content) > 0 {
		newBlock.Content = make([]Block, len(b.Content))

		for i, cb := range b.Content {
			newBlock.Content[i] = cb.copyStripped(dc)
		}
	}

	if len(b.Marks) > 0 {
		newMarks := make([]Mark, 0, len(b.Marks))

		for _, m := range b.Marks {
			if m.Type != _nodeCommentMarkType {
				newMarks = append(newMarks, m)
			}
		}

		if len(newMarks) > 0 {
			newBlock.Marks = newMarks
		}
	}

	if len(b.Attrs) > 0 {
		newAttrs := make(map[string]any)

		for k, v := range b.Attrs {
			if k == _nodeCommentIDAttr {
				continue
			}

			if dc != nil && k == _nodeUIDAttr {
				newAttrs[k] = strutil.NanoID()
				continue
			}

			newAttrs[k] = v
		}

		if dc != nil && b.Type == BlockNodeImageBlock {
			dc.rewriteFileRef(b.Attrs, newAttrs)
		}

		if len(newAttrs) > 0 {
			newBlock.Attrs = newAttrs
		}
	}

	return newBlock
}

// rewriteFileRef points a duplicated image block at the duplicate's own copy
// of the file and records the pair. Images served from anywhere but the
// source document's file route are left as they are: an externally hosted
// image has no object to copy.
func (d *duplication) rewriteFileRef(attrs, newAttrs Attributes) {
	oldID, ok := attrs[_nodeUIDAttr].(string)
	if !ok {
		return
	}

	newID, ok := newAttrs[_nodeUIDAttr].(string)
	if !ok {
		return
	}

	src, ok := attrs[_nodeSrcAttr].(string)
	if !ok {
		return
	}

	u, err := url.Parse(src)
	if err != nil {
		return
	}

	oldPath := fmt.Sprintf(FilePathFormat, d.oldDocumentID, oldID)
	if !strings.HasSuffix(u.Path, oldPath) {
		return
	}

	// only the path is swapped: the src was built by the frontend from its
	// own api base url, which need not match this server's public url.
	u.Path = strings.TrimSuffix(u.Path, oldPath) +
		fmt.Sprintf(FilePathFormat, d.newDocumentID, newID)

	newAttrs[_nodeSrcAttr] = u.String()
	d.files[oldID] = newID
}
