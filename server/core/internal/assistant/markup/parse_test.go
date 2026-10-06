package markup

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

// dump writes parsed nodes in a compact form that tests compare
// against: text as a quoted string, an element as name@line, its sorted
// attributes in parentheses and its content in brackets.
func dump(nodes []node) string {
	parts := make([]string, 0, len(nodes))

	for _, n := range nodes {
		if n.name == "" {
			parts = append(parts, strconv.Quote(n.text))

			continue
		}

		var sb strings.Builder

		sb.WriteString(n.name + "@" + strconv.Itoa(n.line))

		if len(n.attrs) > 0 {
			kv := make([]string, 0, len(n.attrs))
			for _, k := range slices.Sorted(maps.Keys(n.attrs)) {
				kv = append(kv, k+"="+n.attrs[k])
			}

			sb.WriteString("(" + strings.Join(kv, " ") + ")")
		}

		if n.text != "" {
			sb.WriteString("{" + strconv.Quote(n.text) + "}")
		}

		if len(n.children) > 0 {
			sb.WriteString("[" + dump(n.children) + "]")
		}

		parts = append(parts, sb.String())
	}

	return strings.Join(parts, " ")
}

func Test_parse(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  string
		Result string

		// Count, when set, is the number of top-level nodes expected in
		// place of Result.
		Count int

		Err error
	}{
		"Paragraph with inline tags": {
			Input:  `<p id="a">Hi <b>bold <i>both</i></b> and <a href="https://x.test/?a=1&amp;b=2">link</a></p>`,
			Result: `p@1(id=a)["Hi " b@1["bold " i@1["both"]] " and " a@1(href=https://x.test/?a=1&b=2)["link"]]`,
		},
		"Blocks on their own lines": {
			Input:  "<ul id=\"l\">\n  <li id=\"a\">One</li>\n</ul>\n<hr/>",
			Result: `ul@1(id=l)["\n  " li@2(id=a)["One"] "\n"] "\n" hr@4`,
		},
		"Raw element keeps markup and entities as written": {
			Input:  `<pre language="go">if a < b && c { return "</p>" } &amp; &lt;/pre></pre>`,
			Result: `pre@1(language=go){"if a < b && c { return \"</p>\" } &amp; </pre>"}`,
		},
		"Stray angle bracket and unknown tag are text": {
			Input:  `<p>1 < 2 and <br> and <div>x</div></p>`,
			Result: `p@1["1 < 2 and <br> and <div>x</div>"]`,
		},
		"Unknown entity is text": {
			Input:  `<p>a&nbsp;b &amp c &lt; d</p>`,
			Result: `p@1["a&nbsp;b &amp c < d"]`,
		},
		"HTML spellings of bold, italic and strike": {
			Input:  `<p><strong>x</strong> <em>y</em> <DEL>z</DEL></p>`,
			Result: `p@1[b@1["x"] " " i@1["y"] " " s@1["z"]]`,
		},
		"Bare, single quoted and self-closing attributes": {
			Input:  `<task checked id='t&quot;1'>Done</task><img src="x.png" width="2"/>`,
			Result: `task@1(checked=true id=t"1)["Done"] img@1(src=x.png width=2)`,
		},
		"Malformed tag is text": {
			Input:  `<p>see <b class=x>this</p>`,
			Result: `p@1["see <b class=x>this"]`,
		},
		"Entity longer than any known one is text": {
			Input:  `<p>a &ampersand; b</p>`,
			Result: `p@1["a &ampersand; b"]`,
		},
		"Elements nested to the limit": {
			Input:  strings.Repeat("<b>", _maxDepth) + "x" + strings.Repeat("</b>", _maxDepth),
			Result: strings.Repeat("b@1[", _maxDepth) + `"x"` + strings.Repeat("]", _maxDepth),
		},
		"Elements nested past the limit are refused": {
			Input: strings.Repeat("<b>", _maxDepth+1) + "x" + strings.Repeat("</b>", _maxDepth+1),
			Err:   errors.New("line 1: elements nest more than 64 deep"),
		},
		"Markup over the size limit is refused": {
			Input: strings.Repeat("a", _maxMarkupLength+1),
			Err:   errors.New("the markup is 1048577 bytes, over the limit of 1048576; send it in several calls"),
		},
		// many elements and many stray brackets both read in one pass.
		"Large input": {
			Input: strings.Repeat("<p>a <b>b</b> c < d</p>\n", 20000),
			Count: 40000,
		},
		"Unclosed element is refused with its line": {
			Input: "<p>a</p>\n<callout>\n<p>b</p>",
			Err:   errors.New("line 2: <callout> is never closed"),
		},
		"Raw element closes in any case and with space before >": {
			Input:  "<PRE>a</PRE>\n<p>b</p>\n<pre>c</pre >",
			Result: `pre@1{"a"} "\n" p@2["b"] "\n" pre@3{"c"}`,
		},
		"Raw element runs past a closing tag of another element": {
			Input:  "<pre>a</p></Pre>",
			Result: `pre@1{"a</p>"}`,
		},
		"Unclosed raw element is refused": {
			Input: "<pre>x",
			Err:   errors.New("line 1: <pre> is never closed"),
		},
		"Closing tag of another element is refused": {
			Input: "<p>a\n</li>",
			Err:   errors.New("line 2: </li> closes nothing open; expected </p>"),
		},
		"Closing tag with nothing open is refused": {
			Input: "a</p>",
			Err:   errors.New("line 1: </p> closes nothing open; expected nothing"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := parse(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Count > 0 {
				assert.Len(t, got, c.Count)

				return
			}

			assert.Equal(t, c.Result, dump(got))
		})
	}
}

func Test_unescapeRaw(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a </pre> &lt;/pre> &amp;lt;/pre> &lt;/p>", unescapeRaw("a &lt;/pre> &amp;lt;/pre> &amp;amp;lt;/pre> &lt;/p>", _elemCode))
	assert.Equal(t, "a </PRE> &lt;/Pre>", unescapeRaw("a &lt;/PRE> &amp;lt;/Pre>", _elemCode))
}
