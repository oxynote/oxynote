package document

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"maps"

	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
	"github.com/rs/xid"
)

const (
	_nodeCommentIDAttr   = "nodeCommentId"
	_nodeUIDAttr         = "uid"
	_nodeCommentMarkType = "comment"
)

// RootBlock represents a block in a document.
type RootBlock struct {
	// Type is the type of the root block (always BlockNodeText's
	// containing doc — currently "doc" — for TipTap documents).
	Type BlockNodeType `json:"type"`

	// Content is the content of the root block.
	Content []Block `json:"content,omitempty"`
}

// FindBlock searches for a block with the given ID in the root block.
func (rb RootBlock) FindBlock(blockID string) bool {
	for _, b := range rb.Content {
		if b.FindBlock(blockID) {
			return true
		}
	}

	return false
}

// Search transforms RootBlock into a searchgw compatible type.
func (rb RootBlock) Search(organizationID string, documentID xid.ID) map[string]searchgw.Block {
	res := make(map[string]searchgw.Block)

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
	Attrs map[string]any `json:"attrs,omitempty"`
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

// FindBlock recursively searches for a block with the given ID.
func (b Block) FindBlock(blockID string) bool {
	if id, ok := b.UID(); ok && id == blockID {
		return true
	}

	for _, cb := range b.Content {
		if cb.FindBlock(blockID) {
			return true
		}
	}

	return false
}

// Search transforms Block into a searchgw compatible type.
func (b Block) Search(organizationID string, documentID xid.ID) map[string]searchgw.Block {
	res := make(map[string]searchgw.Block)

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
			res[id] = searchgw.Block{
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
	Attrs map[string]any `json:"attrs,omitempty"`
}

// StripCommentMarks returns a copy of the RootBlock with all comment marks
// and nodeCommentId attributes removed. Unlike Duplicate, uid attributes are
// preserved as-is.
func (rb RootBlock) StripCommentMarks() RootBlock {
	newContent := make([]Block, len(rb.Content))

	for i, b := range rb.Content {
		newContent[i] = b.stripCommentMarks()
	}

	return RootBlock{
		Type:    rb.Type,
		Content: newContent,
	}
}

// stripCommentMarks returns a copy of the Block with comment marks and
// nodeCommentId attributes removed. uid attributes are preserved.
func (b Block) stripCommentMarks() Block {
	newBlock := Block{
		Type: b.Type,
		Text: b.Text,
	}

	if len(b.Content) > 0 {
		newBlock.Content = make([]Block, len(b.Content))

		for i, cb := range b.Content {
			newBlock.Content[i] = cb.stripCommentMarks()
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

			newAttrs[k] = v
		}

		if len(newAttrs) > 0 {
			newBlock.Attrs = newAttrs
		}
	}

	return newBlock
}

// Duplicate creates a copy of the RootBlock with all comment marks
// removed, nodeCommentId attributes removed, and uid attributes regenerated.
func (rb RootBlock) Duplicate() RootBlock {
	newContent := make([]Block, len(rb.Content))

	for i, b := range rb.Content {
		newContent[i] = b.duplicate()
	}

	return RootBlock{
		Type:    rb.Type,
		Content: newContent,
	}
}

// duplicate creates a copy of the Block with comment marks removed,
// nodeCommentId attributes removed, and uid attributes regenerated.
func (b Block) duplicate() Block {
	newBlock := Block{
		Type: b.Type,
		Text: b.Text,
	}

	// Process child content recursively
	if len(b.Content) > 0 {
		newBlock.Content = make([]Block, len(b.Content))

		for i, cb := range b.Content {
			newBlock.Content[i] = cb.duplicate()
		}
	}

	// Filter out comment marks
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

	// Process attributes: remove nodeCommentId, regenerate uid
	if len(b.Attrs) > 0 {
		newAttrs := make(map[string]any)

		for k, v := range b.Attrs {
			if k == _nodeCommentIDAttr {
				continue
			}

			if k == _nodeUIDAttr {
				newAttrs[k] = generateNanoID()
				continue
			}

			newAttrs[k] = v
		}

		if len(newAttrs) > 0 {
			newBlock.Attrs = newAttrs
		}
	}

	return newBlock
}
