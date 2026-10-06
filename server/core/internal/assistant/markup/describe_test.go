package markup

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_DescribeNode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "<p>", DescribeNode(document.BlockNodeParagraph))
	assert.Equal(t, "<left>", DescribeNode(document.BlockNodeSplitDocLeft))
	assert.Equal(t, "the document root", DescribeNode(document.BlockNodeDoc))
	assert.Equal(t, "mystery", DescribeNode("mystery"))
}

func Test_Readable(t *testing.T) {
	t.Parallel()

	root := loadFixture(t, "all_blocks")

	cc := map[string]struct {
		UID      string
		Expected string
		Found    bool
	}{
		"Block named by its id":              {UID: "p1", Expected: "p1", Found: true},
		"Nested block named by its id":       {UID: "li3", Expected: "li3", Found: true},
		"Entry paragraph reads as itself":    {UID: "li1p", Expected: "li1p", Found: true},
		"Titled pre's title reads the pre":   {UID: "tct", Expected: "tc1", Found: true},
		"Titled pre's code reads the pre":    {UID: "tcc", Expected: "tc1", Found: true},
		"Split doc side reads the split doc": {UID: "sr", Expected: "sd1", Found: true},
		"Param reads the params":             {UID: "pi1", Expected: "pl1", Found: true},
		"Param cell reads the params":        {UID: "pity", Expected: "pl1", Found: true},
		"Param description reads the params": {UID: "pip", Expected: "pl1", Found: true},
		"Params header reads the params":     {UID: "plh", Expected: "pl1", Found: true},
		"Id the document does not hold":      {UID: "nope"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, ok := Readable(root.Content, c.UID)
			assert.Equal(t, c.Found, ok)

			uid, _ := got.UID()
			assert.Equal(t, c.Expected, uid)
		})
	}
}
