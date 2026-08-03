package liveedit

import (
	"testing"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/oxynote/heimdall/internal/document/aiblock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Operations_ToWire(t *testing.T) {
	tests := map[string]struct {
		Op           Operation
		ExpectedKind string
		Inspect      func(t *testing.T, w wireOp)
	}{
		"InsertAfter carries position and reference uid": {
			Op:           InsertAfter("ref-uid", aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
			ExpectedKind: "insert",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "after", w.Position)
				assert.Equal(t, "ref-uid", w.ReferenceUID)
				require.NotNil(t, w.Block)
				assert.Equal(t, document.BlockNodeParagraph, w.Block.Type)
			},
		},
		"InsertBefore carries position": {
			Op:           InsertBefore("ref", aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
			ExpectedKind: "insert",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "before", w.Position)
			},
		},
		"Append has block but no reference": {
			Op:           Append(aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
			ExpectedKind: "append",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Empty(t, w.Position)
				assert.Empty(t, w.ReferenceUID)
				require.NotNil(t, w.Block)
			},
		},
		"Prepend has block but no reference": {
			Op:           Prepend(aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
			ExpectedKind: "prepend",
		},
		"Replace carries block uid and block": {
			Op:           Replace("target", aiblock.Block{Type: aiblock.BlockParagraph, Text: "hi"}),
			ExpectedKind: "replace",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "target", w.BlockUID)
				require.NotNil(t, w.Block)
			},
		},
		"UpdateText parses markdown into PM inline content": {
			Op:           UpdateText("target", "say **hi**"),
			ExpectedKind: "update_text",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "target", w.BlockUID)
				assert.Nil(t, w.Block)
				require.Len(t, w.Content, 2)
				assert.Equal(t, "say ", w.Content[0].Text)
				assert.Equal(t, "hi", w.Content[1].Text)
				require.Len(t, w.Content[1].Marks, 1)
				assert.Equal(t, "bold", w.Content[1].Marks[0].Type)
			},
		},
		"UpdateAttrs carries attrs map": {
			Op:           UpdateAttrs("target", map[string]any{"level": 3}),
			ExpectedKind: "update_attrs",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "target", w.BlockUID)
				assert.Equal(t, 3, w.Attrs["level"])
			},
		},
		"Delete carries only the uid": {
			Op:           Delete("target"),
			ExpectedKind: "delete",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "target", w.BlockUID)
				assert.Nil(t, w.Block)
			},
		},
		"SetName carries name": {
			Op:           SetName("New title"),
			ExpectedKind: "set_name",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "New title", w.Name)
			},
		},
		"SetIcon carries icon identifier": {
			Op:           SetIcon("lucide:file-text"),
			ExpectedKind: "set_icon",
			Inspect: func(t *testing.T, w wireOp) {
				assert.Equal(t, "lucide:file-text", w.Icon)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w, err := tc.Op.toWire()
			require.NoError(t, err)
			assert.Equal(t, tc.ExpectedKind, w.Kind)

			if tc.Inspect != nil {
				tc.Inspect(t, w)
			}
		})
	}
}
