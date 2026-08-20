package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_CanonicalType(t *testing.T) {
	cc := map[string]struct {
		PM     document.BlockNodeType
		Result Type
		Found  bool
	}{
		"Mapped node type": {
			PM:     document.BlockNodeParagraph,
			Result: BlockParagraph,
			Found:  true,
		},
		"Macro node type": {
			PM:     document.BlockNodeParamList,
			Result: BlockParamList,
			Found:  true,
		},
		"Wrapper node type the canonical model folds away": {
			PM: document.BlockNodeListItem,
		},
		"Unknown node type": {
			PM: document.BlockNodeType("nonsense"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := CanonicalType(c.PM)
			assert.Equal(t, c.Found, ok)
			assert.Equal(t, c.Result, res)
		})
	}
}

// every canonical type Expand knows how to build must be reachable from a
// ProseMirror type, or a block the AI writes could never be read back.
func Test_canonicalTypesCoverExpand(t *testing.T) {
	t.Parallel()

	covered := make(map[Type]bool, len(_canonicalTypes))

	for _, ct := range _canonicalTypes {
		covered[ct] = true
	}

	for _, tp := range []Type{
		BlockParagraph,
		BlockHeading,
		BlockBlockquote,
		BlockBulletList,
		BlockOrderedList,
		BlockTaskList,
		BlockCallout,
		BlockCode,
		BlockTitledCode,
		BlockMermaid,
		BlockHorizontalRule,
		BlockImage,
		BlockFigma,
		BlockMetric,
		BlockMetricGrid,
		BlockSplitDoc,
		BlockParamList,
	} {
		assert.True(t, covered[tp], "%s is not reachable from a ProseMirror type", tp)
	}
}
