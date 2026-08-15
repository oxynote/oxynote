package tools

import (
	"maps"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

// pmBlock builds a document.Block with a uid attr plus optional
// extra attrs and children.
func pmBlock(tp document.BlockNodeType, uid string, attrs map[string]any, children ...document.Block) document.Block {
	all := document.Attributes{}
	if uid != "" {
		all["uid"] = uid
	}

	maps.Copy(all, attrs)

	if len(all) == 0 {
		all = nil
	}

	return document.Block{Type: tp, Attrs: all, Content: children}
}

// pmText builds an inline text node.
func pmText(text string) document.Block {
	return document.Block{Type: document.BlockNodeText, Text: text}
}

func Test_walkDocForAssistant(t *testing.T) {
	cc := map[string]struct {
		Blocks []document.Block
		Result []docSummaryEntry
	}{
		"Simple blocks surface flat": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("hello")),
				pmBlock(document.BlockNodeHeading, "h1", map[string]any{"level": 2}, pmText("Title")),
			},
			Result: []docSummaryEntry{
				{UID: "p1", Kind: "paragraph", Text: "hello"},
				{UID: "h1", Kind: "heading", Text: "Title", Attrs: map[string]any{"level": 2}},
			},
		},
		"Lists descend into their items": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeBulletList, "l1", nil,
					pmBlock(document.BlockNodeListItem, "li1", nil,
						pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("one")),
					),
				),
			},
			Result: []docSummaryEntry{
				{UID: "l1", Kind: "bullet_list", Text: "one"},
				{UID: "li1", Kind: "list_item", Text: "one", Depth: 1, ParentUID: "l1"},
			},
		},
		"Task list surfaces checked attr on items": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeTaskList, "t1", nil,
					pmBlock(document.BlockNodeTaskItem, "ti1", map[string]any{"checked": true},
						pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("done")),
					),
				),
			},
			Result: []docSummaryEntry{
				{UID: "t1", Kind: "task_list", Text: "done"},
				{UID: "ti1", Kind: "task_item", Text: "done", Depth: 1, ParentUID: "t1", Attrs: map[string]any{"checked": true}},
			},
		},
		"Split doc emits a single entry without descending": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeSplitDoc, "s1", map[string]any{"inversed": true},
					pmBlock(document.BlockNodeSplitDocLeft, "sl1", nil,
						pmBlock(document.BlockNodeHeading, "h1", map[string]any{"level": 1}, pmText("API")),
					),
				),
			},
			Result: []docSummaryEntry{
				{UID: "s1", Kind: "split_doc", Text: "API", Attrs: map[string]any{"inversed": true}},
			},
		},
		"Param list emits a single entry without descending": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeParamList, "pl1", nil,
					pmBlock(document.BlockNodeParamListHeader, "plh1", nil, pmText("Body")),
				),
			},
			Result: []docSummaryEntry{
				{UID: "pl1", Kind: "split_doc_param_list", Text: "Body"},
			},
		},
		"Blocks without uid are skipped": {
			Blocks: []document.Block{
				pmBlock(document.BlockNodeParagraph, "", nil, pmText("anonymous")),
				pmBlock(document.BlockNodeParagraph, "p1", nil, pmText("named")),
			},
			Result: []docSummaryEntry{
				{UID: "p1", Kind: "paragraph", Text: "named"},
			},
		},
		"Unknown node types are skipped": {
			Blocks: []document.Block{
				pmBlock("weirdNode", "w1", nil),
				pmBlock(document.BlockNodeCodeBlock, "c1", map[string]any{"language": "go"}, pmText("x := 1")),
			},
			Result: []docSummaryEntry{
				{UID: "c1", Kind: "code", Text: "x := 1", Attrs: map[string]any{"language": "go"}},
			},
		},
		"Empty document yields no entries": {
			Blocks: nil,
			Result: nil,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, walkDocForAssistant(c.Blocks))
		})
	}
}

func Test_canonicalKindForPM(t *testing.T) {
	cc := map[string]struct {
		PM     document.BlockNodeType
		Result string
	}{
		"Paragraph":             {PM: document.BlockNodeParagraph, Result: "paragraph"},
		"Heading":               {PM: document.BlockNodeHeading, Result: "heading"},
		"Blockquote":            {PM: document.BlockNodeBlockquote, Result: "blockquote"},
		"Bullet list":           {PM: document.BlockNodeBulletList, Result: "bullet_list"},
		"Ordered list":          {PM: document.BlockNodeOrderedList, Result: "ordered_list"},
		"Task list":             {PM: document.BlockNodeTaskList, Result: "task_list"},
		"List item":             {PM: document.BlockNodeListItem, Result: "list_item"},
		"Task item":             {PM: document.BlockNodeTaskItem, Result: "task_item"},
		"Callout":               {PM: document.BlockNodeCalloutBlock, Result: "callout"},
		"Code":                  {PM: document.BlockNodeCodeBlock, Result: "code"},
		"Titled code":           {PM: document.BlockNodeTitledCodeBlock, Result: "titled_code"},
		"Mermaid":               {PM: document.BlockNodeMermaidBlock, Result: "mermaid"},
		"Horizontal rule":       {PM: document.BlockNodeHorizontalRule, Result: "horizontal_rule"},
		"Image":                 {PM: document.BlockNodeImageBlock, Result: "image"},
		"Figma":                 {PM: document.BlockNodeFigmaBlock, Result: "figma"},
		"Metric":                {PM: document.BlockNodeMetricBlock, Result: "metric"},
		"Metric grid":           {PM: document.BlockNodeMetricGrid, Result: "metric_grid"},
		"Split doc":             {PM: document.BlockNodeSplitDoc, Result: "split_doc"},
		"Param list":            {PM: document.BlockNodeParamList, Result: "split_doc_param_list"},
		"Unknown falls through": {PM: "weirdNode", Result: "weirdNode"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, canonicalKindForPM(c.PM))
		})
	}
}

func Test_summaryAttrs(t *testing.T) {
	cc := map[string]struct {
		Input  document.Block
		Result map[string]any
	}{
		"Heading level": {
			Input:  pmBlock(document.BlockNodeHeading, "h", map[string]any{"level": 3}),
			Result: map[string]any{"level": 3},
		},
		"Heading without level": {
			Input:  pmBlock(document.BlockNodeHeading, "h", nil),
			Result: nil,
		},
		"Callout icon": {
			Input:  pmBlock(document.BlockNodeCalloutBlock, "c", map[string]any{"icon": "lucide:warning"}),
			Result: map[string]any{"icon": "lucide:warning"},
		},
		"Code language": {
			Input:  pmBlock(document.BlockNodeCodeBlock, "c", map[string]any{"language": "go"}),
			Result: map[string]any{"language": "go"},
		},
		"Code with empty language": {
			Input:  pmBlock(document.BlockNodeCodeBlock, "c", map[string]any{"language": ""}),
			Result: nil,
		},
		"Task item checked": {
			Input:  pmBlock(document.BlockNodeTaskItem, "t", map[string]any{"checked": false}),
			Result: map[string]any{"checked": false},
		},
		"Image attrs filtered": {
			Input: pmBlock(document.BlockNodeImageBlock, "i", map[string]any{
				"src": "http://x", "alt": "", "width": 100, "opaque": "dropped",
			}),
			Result: map[string]any{"src": "http://x", "width": 100},
		},
		"Image without usable attrs": {
			Input:  pmBlock(document.BlockNodeImageBlock, "i", nil),
			Result: nil,
		},
		"Figma attrs filtered": {
			Input: pmBlock(document.BlockNodeFigmaBlock, "f", map[string]any{
				"src": "http://f", "height": 200,
			}),
			Result: map[string]any{"src": "http://f", "height": 200},
		},
		"Figma without usable attrs": {
			Input:  pmBlock(document.BlockNodeFigmaBlock, "f", nil),
			Result: nil,
		},
		"Split doc inversed": {
			Input:  pmBlock(document.BlockNodeSplitDoc, "s", map[string]any{"inversed": true}),
			Result: map[string]any{"inversed": true},
		},
		"Split doc not inversed": {
			Input:  pmBlock(document.BlockNodeSplitDoc, "s", map[string]any{"inversed": false}),
			Result: nil,
		},
		"Metric attrs are opaque": {
			Input:  pmBlock(document.BlockNodeMetricBlock, "m", map[string]any{"query": "up"}),
			Result: nil,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, summaryAttrs(c.Input))
		})
	}
}
