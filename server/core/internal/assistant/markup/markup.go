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
	"slices"

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

	// _attrStack is a grid's stacked order: the ids of its blocks, first
	// to last.
	_attrStack = "stack"
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

// _gridPlaceAttrs are the attributes that say where a grid block sits:
// its first column and row in the wide layout, and its place in the
// stacked layout.
var _gridPlaceAttrs = []string{document.AttrWidePosX, document.AttrWidePosY, document.AttrStackPos}

// _hiddenAttrs are the attributes of a metric or log block the model
// never sees. They are the uid, the block comment, a metric's simulation
// flag and the block's places in its grid. The uid shows as the id, and
// core derives the simulation flag. The editor lays out the places, so
// the model orders a grid's blocks by element order and the grid's stack
// attribute instead. The model sees the width and height as names.
var _hiddenAttrs = slices.Concat([]string{document.AttrUID, document.AttrCommentID, document.AttrSimulationActive}, _gridPlaceAttrs)

// _gridItemWidths are the named widths of a block inside a grid,
// narrowest first.
var _gridItemWidths = sizeNames(_gridItemSizes[document.AttrWidth])

// _gridItemHeights are the named heights of a block inside a grid,
// lowest first.
var _gridItemHeights = sizeNames(_gridItemSizes[document.AttrHeight])

// _gridItemSizes maps each named width to the columns it spans and each
// named height to the rows it spans. They mirror GRID_WIDTH_COLUMNS and
// GRID_HEIGHT_ROWS in packages/common/src/grid.ts. A size added there has
// to be added here, or the assistant cannot write it.
//
//nolint:mnd // the sizes are the table, so naming each one would hide it
var _gridItemSizes = map[string]map[string]int{
	document.AttrWidth:  {"compact": 8, "standard": 12, "wide": 24},
	document.AttrHeight: {"standard": 1, "tall": 2},
}
