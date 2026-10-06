package markup

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// _fixtures are the documents in testdata, each with a golden rendering
// beside it.
var _fixtures = []string{"all_blocks", "welcome", "scratch", "empty_parts"}

// loadFixture reads a document fixture from testdata.
func loadFixture(t *testing.T, name string) document.RootBlock {
	t.Helper()

	data, err := os.ReadFile("testdata/" + name + ".json") //nolint:gosec // the name is one of this package's own fixtures
	require.NoError(t, err)

	var root document.RootBlock
	require.NoError(t, json.Unmarshal(data, &root))

	return root
}

func Test_Render(t *testing.T) {
	t.Parallel()

	for _, name := range _fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := Render(loadFixture(t, name).Content)

			// a rendering change is meant to be reviewed as a golden
			// diff, so the file is rewritten on request.
			if os.Getenv("UPDATE_GOLDEN") != "" {
				require.NoError(t, os.WriteFile("testdata/"+name+".xml", []byte(got), 0o600))

				return
			}

			want, err := os.ReadFile("testdata/" + name + ".xml") //nolint:gosec // the name is one of this package's own fixtures
			require.NoError(t, err)
			assert.Equal(t, string(want), got)
		})
	}
}

func Test_renderer_entry(t *testing.T) {
	t.Parallel()

	// an entry whose first child is not a paragraph has no text of its
	// own, and its children render nested.
	got := Render([]document.Block{{
		Type:  document.BlockNodeBulletList,
		Attrs: document.Attributes{document.AttrUID: "l"},
		Content: []document.Block{{
			Type:  document.BlockNodeListItem,
			Attrs: document.Attributes{document.AttrUID: "li"},
			Content: []document.Block{{
				Type:    document.BlockNodeCodeBlock,
				Attrs:   document.Attributes{document.AttrUID: "c"},
				Content: []document.Block{{Type: document.BlockNodeText, Text: "x"}},
			}},
		}},
	}})

	assert.Equal(t, strings.Join([]string{
		`<ul id="l">`,
		`  <li id="li">`,
		`    <pre id="c">x</pre>`,
		`  </li>`,
		`</ul>`,
		``,
	}, "\n"), got)
}

func Test_renderer_param(t *testing.T) {
	t.Parallel()

	// a param read or moved alone renders as itself.
	row, ok := loadFixture(t, "all_blocks").FindByUID("pi1")
	require.True(t, ok)

	assert.Equal(t, "<param id=\"pi1\" name=\"email\" type=\"string\">The <b>user</b> email</param>\n", Render([]document.Block{row}))
}

func Test_attr_text(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result string
	}{
		"String":            {Value: "a", Result: "a"},
		"True":              {Value: true, Result: "true"},
		"False":             {Value: false},
		"Positive int":      {Value: 3, Result: "3"},
		"Zero int":          {Value: 0},
		"Positive float":    {Value: 640.0, Result: "640"},
		"Negative float":    {Value: -1.0},
		"Unset":             {},
		"Unsupported value": {Value: []any{"a"}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, attr{key: "k", value: c.Value}.text())
		})
	}
}

func Test_escapeRaw(t *testing.T) {
	t.Parallel()

	// every level of escaping reads back as it was written.
	for _, raw := range []string{"a </pre> b", "a &lt;/pre> b", "a &amp;lt;/pre> b", "</p> &lt;/p>", "a </PRE> &lt;/Pre> b"} {
		escaped := escapeRaw(raw, _elemCode)
		assert.NotContains(t, strings.ToLower(escaped), "</pre")
		assert.Equal(t, raw, unescapeRaw(escaped, _elemCode))
	}
}
