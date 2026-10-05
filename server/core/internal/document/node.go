package document

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/strutil"
	"github.com/rs/xid"
)

// FilePathFormat is the URL path format under which document files are
// served. Duplication matches stored src attributes against it, so the
// router mounts the file routes on this very format.
const FilePathFormat = "/api/documents/%s/files/%s"

// FileIDLength is the exact length of a file id, a nanoid. The id's
// alphabet includes the dash, so a "<id>-<file name>" segment is split by
// length, never at the first dash.
const FileIDLength = strutil.NanoIDLength

// FilePath returns the URL path a document's file is served under: the
// file route with the id followed by a dash and the file name, so the
// address reads as the file it is. Every file id is a 21-character nanoid,
// which is what lets the server cut the id off the front again.
func FilePath(documentID xid.ID, id, name string) string {
	return fmt.Sprintf(FilePathFormat, documentID, id+"-"+url.PathEscape(name))
}

// ParseFileRef cuts a "<id>-<file name>" route segment, as FilePath
// builds it, back into its id and name, reporting whether the segment
// has that shape. The id must be a nanoid: its charset shuts out the
// path separators and dot segments that would otherwise let an id
// escape its storage folder once joined into an object key.
func ParseFileRef(ref string) (id, name string, ok bool) {
	if len(ref) <= FileIDLength || ref[FileIDLength] != '-' {
		return "", "", false
	}

	id = ref[:FileIDLength]
	if !strutil.IsNanoID(id) {
		return "", "", false
	}

	return id, ref[FileIDLength+1:], true
}

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

// FindParentTypeByUID recursively searches the root's subtree for the
// block with the given uid and returns the node type of the block that
// directly holds it. A top-level match reports BlockNodeDoc.
func (rb RootBlock) FindParentTypeByUID(uid string) (BlockNodeType, bool) {
	for _, b := range rb.Content {
		if id, ok := b.UID(); ok && id == uid {
			return BlockNodeDoc, true
		}

		if t, ok := b.FindParentTypeByUID(uid); ok {
			return t, true
		}
	}

	return "", false
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

// Value transforms stopper type into a database entry.
func (rb RootBlock) Value() (driver.Value, error) {
	return json.Marshal(rb)
}

// Scan transforms a database entry into a root block type.
func (rb *RootBlock) Scan(src any) error {
	var pv []byte

	switch v := src.(type) {
	case []byte:
		pv = v
	case string:
		pv = []byte(v)
	default:
		return errors.New("invalid root block type")
	}

	data := &RootBlock{}
	if err := json.Unmarshal(pv, data); err != nil {
		return err
	}

	*rb = *data

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
	id, ok := b.Attrs[AttrUID].(string)
	return id, ok
}

// Flatten returns the concatenated text of the block's entire
// subtree. Text within one block reads as written, and the text of
// separate blocks is separated by a single space. Used to derive a flat
// searchable representation from a structured ProseMirror tree.
func (b Block) Flatten() string {
	f := flattener{}
	f.walk(b)

	return string(f.buf)
}

// flattener accumulates the text Flatten returns.
type flattener struct {
	// buf holds the text written so far.
	buf []byte

	// apart indicates that a block boundary was crossed since the last
	// text, so the next text is set apart by a space.
	apart bool
}

// walk appends the text of b's subtree. Adjacent text nodes are one run
// split only by marks, so they join as written; any other node, such as
// a block or a hard break, sets the text around it apart.
func (f *flattener) walk(b Block) {
	if b.Type == BlockNodeText {
		if b.Text == "" {
			return
		}

		if f.apart && len(f.buf) > 0 && f.buf[len(f.buf)-1] != ' ' {
			f.buf = append(f.buf, ' ')
		}

		f.apart = false
		f.buf = append(f.buf, b.Text...)

		return
	}

	f.apart = true

	for _, c := range b.Content {
		f.walk(c)
	}

	f.apart = true
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

// FindParentTypeByUID recursively searches the block's children for the
// block with the given uid and returns the node type of the block that
// directly holds it — b's own type for a direct child. A match on b
// itself is not reported, since b's parent is not in view.
func (b Block) FindParentTypeByUID(uid string) (BlockNodeType, bool) {
	for _, cb := range b.Content {
		if id, ok := cb.UID(); ok && id == uid {
			return b.Type, true
		}

		if t, ok := cb.FindParentTypeByUID(uid); ok {
			return t, true
		}
	}

	return "", false
}

// HasBlock recursively searches for a block with the given ID.
func (b Block) HasBlock(blockID string) bool {
	_, ok := b.FindByUID(blockID)

	return ok
}

// Mark represents a mark, such as bold or italic.
type Mark struct {
	// Type is the type of the mark, such as "bold", "italic", etc.
	Type string `json:"type"`

	// Attrs are additional attributes for the mark, such as link URL.
	Attrs Attributes `json:"attrs,omitempty"`
}

// UnmarshalJSON decodes the mark, reading attrs that are not an object
// as none. Older documents hold marks with `true` for attrs, and one
// failing mark would keep the whole document from being stored.
func (m *Mark) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type  string          `json:"type"`
		Attrs json.RawMessage `json:"attrs"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decoding mark: %w", err)
	}

	m.Type = raw.Type
	m.Attrs = nil

	if !bytes.HasPrefix(bytes.TrimSpace(raw.Attrs), []byte("{")) {
		return nil
	}

	if err := json.Unmarshal(raw.Attrs, &m.Attrs); err != nil {
		return fmt.Errorf("decoding mark attrs: %w", err)
	}

	return nil
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
// rewritten to the duplicate's, and the first returned map pairs every such
// file's old id with its new one so the caller can copy the objects
// themselves. The second pairs every block's old uid with its new one, for
// whatever else is keyed by block.
func (rb RootBlock) Duplicate(oldDocumentID, newDocumentID xid.ID) (RootBlock, map[string]string, map[string]string) {
	dc := &duplication{
		oldDocumentID: oldDocumentID,
		newDocumentID: newDocumentID,
		files:         make(map[string]string),
		uids:          make(map[string]string),
	}

	return rb.copyStripped(dc), dc.files, dc.uids
}

// RegenerateUIDs returns a copy of the RootBlock with comment marks removed
// and every uid attribute regenerated. Unlike Duplicate it rewrites no file
// references, which suits content templates: they carry none.
func (rb RootBlock) RegenerateUIDs() RootBlock {
	return rb.copyStripped(&duplication{
		files: make(map[string]string),
		uids:  make(map[string]string),
	})
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

	// uids maps the source document's block uids to the ones the duplicate
	// carries in their place.
	uids map[string]string
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
			if m.Type != MarkComment {
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
			if k == AttrCommentID {
				continue
			}

			if dc != nil && k == AttrUID {
				newAttrs[k] = dc.regenerateUID(v)
				continue
			}

			newAttrs[k] = v
		}

		if dc != nil && (b.Type == BlockNodeImageBlock || b.Type == BlockNodeFileBlock) {
			dc.rewriteFileRef(b.Attrs, newAttrs)
		}

		if len(newAttrs) > 0 {
			newBlock.Attrs = newAttrs
		}
	}

	return newBlock
}

// regenerateUID mints the uid a duplicated block carries in place of the
// given one and records the pair.
func (d *duplication) regenerateUID(old any) string {
	uid := strutil.NanoID()

	if s, ok := old.(string); ok {
		d.uids[s] = uid
	}

	return uid
}

// rewriteFileRef points a duplicated block's src at the duplicate's own
// copy of the file and records the pair. The file id is the fixed-length
// segment after the source document's file route; a src served from
// anywhere else has no object to copy and is left as it is.
func (d *duplication) rewriteFileRef(attrs, newAttrs Attributes) {
	src, ok := attrs[AttrSrc].(string)
	if !ok {
		return
	}

	u, err := url.Parse(src)
	if err != nil {
		return
	}

	prefix, rest, ok := strings.Cut(u.Path, fmt.Sprintf(FilePathFormat, d.oldDocumentID, ""))
	if !ok {
		return
	}

	oldID, name, ok := ParseFileRef(rest)
	if !ok {
		return
	}

	newID, ok := d.files[oldID]
	if !ok {
		newID = strutil.NanoID()
		d.files[oldID] = newID
	}

	// only the path is swapped: the src was built by the frontend from its
	// own api base url, which need not match this server's public url.
	u.Path = prefix + fmt.Sprintf(FilePathFormat, d.newDocumentID, newID) + "-" + name
	u.RawPath = ""

	newAttrs[AttrSrc] = u.String()
}
