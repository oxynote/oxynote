package edit

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
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

func Test_InsertAfter(t *testing.T) {
	t.Parallel()

	b := paragraph()

	// the block ships as handed over, uids included, so what the caller
	// reported is what lands.
	assert.Equal(t, Operation{Kind: "insert", Position: "after", ReferenceUID: "ref", Block: &b}, InsertAfter("ref", paragraph()))
}

func Test_InsertBefore(t *testing.T) {
	t.Parallel()

	b := paragraph()

	assert.Equal(t, Operation{Kind: "insert", Position: "before", ReferenceUID: "ref", Block: &b}, InsertBefore("ref", paragraph()))
}

func Test_Append(t *testing.T) {
	t.Parallel()

	b := paragraph()

	assert.Equal(t, Operation{Kind: "append", Block: &b}, Append(paragraph()))
}

func Test_Prepend(t *testing.T) {
	t.Parallel()

	b := paragraph()

	assert.Equal(t, Operation{Kind: "prepend", Block: &b}, Prepend(paragraph()))
}

func Test_Replace(t *testing.T) {
	t.Parallel()

	b := paragraph()

	assert.Equal(t, Operation{Kind: "replace", BlockUID: "target", Block: &b}, Replace("target", paragraph()))
}

func Test_UpdateAttrs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "update_attrs", BlockUID: "target", Attrs: map[string]any{"level": 3}}, UpdateAttrs("target", map[string]any{"level": 3}))
}

func Test_Delete(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "delete", BlockUID: "target"}, Delete("target"))
}

func Test_MoveAfter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "move", Position: "after", BlockUID: "target", ReferenceUID: "ref"}, MoveAfter("target", "ref"))
}

func Test_MoveBefore(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "move", Position: "before", BlockUID: "target", ReferenceUID: "ref"}, MoveBefore("target", "ref"))
}

func Test_SetName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "set_name", Name: "New title"}, SetName("New title"))
}

func Test_SetIcon(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Operation{Kind: "set_icon", Icon: "lucide:file-text"}, SetIcon("lucide:file-text"))
}
