package edit

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paragraph is the block every operation that carries one is built with,
// expanded the way a tool hands it over.
func paragraph() document.Block {
	return document.Block{
		Type:    document.BlockNodeParagraph,
		Attrs:   document.Attributes{document.AttrUID: "p"},
		Content: []document.Block{{Type: document.BlockNodeText, Text: "hi"}},
	}
}

// wire runs the operation and returns the wire form it produced.
func wire(t *testing.T, op Operation) wireOp {
	t.Helper()

	w, err := op()
	require.NoError(t, err)

	return w
}

func Test_InsertAfter(t *testing.T) {
	t.Parallel()

	w := wire(t, InsertAfter("ref-uid", paragraph()))

	assert.Equal(t, "insert", w.Kind)
	assert.Equal(t, "after", w.Position)
	assert.Equal(t, "ref-uid", w.ReferenceUID)
	require.NotNil(t, w.Block)
	assert.Equal(t, document.BlockNodeParagraph, w.Block.Type)
}

func Test_InsertBefore(t *testing.T) {
	t.Parallel()

	w := wire(t, InsertBefore("ref", paragraph()))

	assert.Equal(t, "insert", w.Kind)
	assert.Equal(t, "before", w.Position)
	assert.Equal(t, "ref", w.ReferenceUID)
}

func Test_insert(t *testing.T) {
	t.Parallel()

	w := wire(t, insert("before", "ref-uid", paragraph()))

	assert.Equal(t, "insert", w.Kind)
	assert.Equal(t, "before", w.Position)
	assert.Equal(t, "ref-uid", w.ReferenceUID)
	require.NotNil(t, w.Block)
	assert.Equal(t, document.BlockNodeParagraph, w.Block.Type)

	// the block ships as handed over, uids included, so what the caller
	// reported is what lands.
	uid, ok := w.Block.UID()
	require.True(t, ok)
	assert.Equal(t, "p", uid)
}

func Test_Append(t *testing.T) {
	t.Parallel()

	w := wire(t, Append(paragraph()))

	assert.Equal(t, "append", w.Kind)
	assert.Empty(t, w.Position)
	assert.Empty(t, w.ReferenceUID)
	require.NotNil(t, w.Block)
}

func Test_Prepend(t *testing.T) {
	t.Parallel()

	w := wire(t, Prepend(paragraph()))

	assert.Equal(t, "prepend", w.Kind)
	require.NotNil(t, w.Block)
}

func Test_Replace(t *testing.T) {
	t.Parallel()

	w := wire(t, Replace("target", paragraph()))

	assert.Equal(t, "replace", w.Kind)
	assert.Equal(t, "target", w.BlockUID)
	require.NotNil(t, w.Block)
}

func Test_UpdateText(t *testing.T) {
	t.Parallel()

	w := wire(t, UpdateText("target", "say **hi**"))

	assert.Equal(t, "update_text", w.Kind)
	assert.Equal(t, "target", w.BlockUID)
	assert.Nil(t, w.Block)

	// the markdown is parsed into ProseMirror inline content.
	require.Len(t, w.Content, 2)
	assert.Equal(t, "say ", w.Content[0].Text)
	assert.Equal(t, "hi", w.Content[1].Text)
	require.Len(t, w.Content[1].Marks, 1)
	assert.Equal(t, "bold", w.Content[1].Marks[0].Type)
}

func Test_UpdateAttrs(t *testing.T) {
	t.Parallel()

	w := wire(t, UpdateAttrs("target", map[string]any{"level": 3}))

	assert.Equal(t, "update_attrs", w.Kind)
	assert.Equal(t, "target", w.BlockUID)
	assert.Equal(t, 3, w.Attrs["level"])
}

func Test_Delete(t *testing.T) {
	t.Parallel()

	w := wire(t, Delete("target"))

	assert.Equal(t, "delete", w.Kind)
	assert.Equal(t, "target", w.BlockUID)
	assert.Nil(t, w.Block)
}

func Test_MoveAfter(t *testing.T) {
	t.Parallel()

	w := wire(t, MoveAfter("target", "ref-uid"))

	assert.Equal(t, "move", w.Kind)
	assert.Equal(t, "after", w.Position)
	assert.Equal(t, "target", w.BlockUID)
	assert.Equal(t, "ref-uid", w.ReferenceUID)
	assert.Nil(t, w.Block)
}

func Test_MoveBefore(t *testing.T) {
	t.Parallel()

	w := wire(t, MoveBefore("target", "ref-uid"))

	assert.Equal(t, "move", w.Kind)
	assert.Equal(t, "before", w.Position)
	assert.Equal(t, "target", w.BlockUID)
	assert.Equal(t, "ref-uid", w.ReferenceUID)
}

func Test_move(t *testing.T) {
	t.Parallel()

	w := wire(t, move("before", "target", "ref-uid"))

	assert.Equal(t, "move", w.Kind)
	assert.Equal(t, "before", w.Position)
	assert.Equal(t, "target", w.BlockUID)
	assert.Equal(t, "ref-uid", w.ReferenceUID)
	assert.Nil(t, w.Block)
}

func Test_SetName(t *testing.T) {
	t.Parallel()

	w := wire(t, SetName("New title"))

	assert.Equal(t, "set_name", w.Kind)
	assert.Equal(t, "New title", w.Name)
}

func Test_SetIcon(t *testing.T) {
	t.Parallel()

	w := wire(t, SetIcon("lucide:file-text"))

	assert.Equal(t, "set_icon", w.Kind)
	assert.Equal(t, "lucide:file-text", w.Icon)
}
