// Package markup renders documents as the XML the assistant reads and
// writes, and builds ProseMirror blocks back from it.
//
// Each block is one element with its uid as id. Inline formatting is
// tags (b, i, u, s, code, a href), and pre, mermaid, metrics and logs
// hold raw text. Parts nobody addresses, such as a list entry's
// paragraph, have no id and keep their uids through their element.
package markup

import (
	"regexp"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Element names, as the model reads and writes them.
const (
	_elemParagraph  = "p"
	_elemBlockquote = "blockquote"
	_elemBullets    = "ul"
	_elemOrdered    = "ol"
	_elemEntry      = "li"
	_elemTasks      = "tasks"
	_elemTask       = "task"
	_elemCallout    = "callout"
	_elemCode       = "pre"
	_elemMermaid    = "mermaid"
	_elemRule       = "hr"
	_elemImage      = "img"
	_elemFigma      = "figma"
	_elemFile       = "file"
	_elemGrid       = "grid"
	_elemMetrics    = "metrics"
	_elemLogs       = "logs"
	_elemSplitDoc   = "split_doc"
	_elemLeft       = "left"
	_elemRight      = "right"
	_elemParams     = "params"
	_elemParam      = "param"

	// _elemUnsupported stands in for a node type this package does not
	// know, so a document holding one still reads. It can be kept but not
	// written.
	_elemUnsupported = "unsupported"
)

// Attribute names that are not ProseMirror attribute names.
const (
	// _attrID carries a block's uid.
	_attrID = "id"

	// _attrHeader is a parameter list's header text.
	_attrHeader = "header"

	// _attrType is a parameter's type, or an unsupported node's type.
	_attrType = "type"

	// _attrHref is a link's target.
	_attrHref = "href"
)

// Inline tags, by the ProseMirror mark they write.
const (
	_tagBold      = "b"
	_tagItalic    = "i"
	_tagUnderline = "u"
	_tagStrike    = "s"
	_tagCode      = "code"
	_tagLink      = "a"
)

// _tagMarks maps each inline tag to the ProseMirror mark it writes.
// Marks not listed, such as comments, stay hidden and are kept by the
// build step.
var _tagMarks = map[string]string{
	_tagLink:      "link",
	_tagBold:      "bold",
	_tagItalic:    "italic",
	_tagUnderline: "underline",
	_tagStrike:    "strike",
	_tagCode:      "code",
}

// _tagNesting is the order inline tags nest in when several open
// together.
var _tagNesting = []string{_tagLink, _tagBold, _tagItalic, _tagUnderline, _tagStrike, _tagCode}

// _rawElements hold raw text, up to their closing tag. Each maps to a
// match of that closing tag escaped once or more in the text, capturing
// the extra amp; levels and the name as written. A closing tag reads in
// any case, so the name matches in any case too.
var _rawElements = map[string]*regexp.Regexp{
	_elemCode:    regexp.MustCompile(`&((?:amp;)*)lt;/((?i:` + _elemCode + `))`),
	_elemMermaid: regexp.MustCompile(`&((?:amp;)*)lt;/((?i:` + _elemMermaid + `))`),
	_elemMetrics: regexp.MustCompile(`&((?:amp;)*)lt;/((?i:` + _elemMetrics + `))`),
	_elemLogs:    regexp.MustCompile(`&((?:amp;)*)lt;/((?i:` + _elemLogs + `))`),
}

// _rawCloseTags match the start of a raw element's closing tag in any
// case, which is what its text has to escape.
var _rawCloseTags = map[string]*regexp.Regexp{
	_elemCode:    regexp.MustCompile(`</((?i:` + _elemCode + `))`),
	_elemMermaid: regexp.MustCompile(`</((?i:` + _elemMermaid + `))`),
	_elemMetrics: regexp.MustCompile(`</((?i:` + _elemMetrics + `))`),
	_elemLogs:    regexp.MustCompile(`</((?i:` + _elemLogs + `))`),
}

// _hiddenAttrs are the attributes of a metric or log block the model
// never sees: the uid, which is the id, the block comment, and a
// metric's simulation flag, which core derives.
var _hiddenAttrs = map[string]bool{
	document.AttrUID:              true,
	document.AttrCommentID:        true,
	document.AttrSimulationActive: true,
}

// _gridItemWidths are the widths of a block inside a grid. They mirror
// GridItemWidth in web/app/components/editor/blocks/grid/utils.ts. A
// value added there has to be added here, or the assistant cannot write
// it.
var _gridItemWidths = []string{"compact", "standard", "wide"}
