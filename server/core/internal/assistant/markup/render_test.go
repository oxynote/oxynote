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

	cc := map[string]struct {
		Blocks []document.Block
		Result string
	}{
		"Entry without a leading paragraph has no text and nests its children": {
			Blocks: []document.Block{{
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
			}},
			Result: strings.Join([]string{
				`<ul id="l">`,
				`  <li id="li">`,
				`    <pre id="c">x</pre>`,
				`  </li>`,
				`</ul>`,
				``,
			}, "\n"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Render(c.Blocks))
		})
	}
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

	cc := map[string]struct {
		Text   string
		Result string
	}{
		"Closing tag": {
			Text:   "a </pre> b",
			Result: "a &lt;/pre> b",
		},
		"Escaped closing tag gains a level": {
			Text:   "a &lt;/pre> &amp;lt;/pre> b",
			Result: "a &amp;lt;/pre> &amp;amp;lt;/pre> b",
		},
		"Closing tags in any case": {
			Text:   "a </PRE> &lt;/Pre> b",
			Result: "a &lt;/PRE> &amp;lt;/Pre> b",
		},
		"Other closing tags stay": {
			Text:   "</p> &lt;/p>",
			Result: "</p> &lt;/p>",
		},
		"Code without entities stays": {
			Text:   "if a && b { x() }",
			Result: "if a && b { x() }",
		},
		"Entities with no angle bracket are escaped whole": {
			Text:   "x &lt; y && z",
			Result: "x &amp;lt; y &amp;&amp; z",
		},
		"Entities beside an escaped closing tag are escaped whole": {
			Text:   "</pre> &amp;",
			Result: "&lt;/pre> &amp;amp;",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			escaped := escapeRaw(c.Text, _elemCode)
			assert.Equal(t, c.Result, escaped)
			assert.NotContains(t, strings.ToLower(escaped), "</pre")
			assert.Equal(t, c.Text, unescapeRaw(escaped, _elemCode))
		})
	}
}
