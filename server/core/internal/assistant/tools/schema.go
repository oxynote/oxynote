package tools

import (
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// JSON-schema literals shared by the tool schemas.
const (
	// _keyType is the JSON-schema type key.
	_keyType = "type"

	// _typeObject is the JSON-schema object type.
	_typeObject = "object"

	// _typeString is the JSON-schema string type.
	_typeString = "string"

	// _typeArray is the JSON-schema array type.
	_typeArray = "array"

	// _typeInteger is the JSON-schema integer type.
	_typeInteger = "integer"

	// _keyDescription is the JSON-schema description key.
	_keyDescription = "description"

	// _keyEnum is the JSON-schema enum key, naming the values a
	// property accepts.
	_keyEnum = "enum"

	// _keyBranchID is the shared branch-id property name.
	_keyBranchID = "branch_id"

	// _descBranchID describes the branch-id property every content tool
	// takes.
	_descBranchID = "The id of the branch to read or write: a document's default_branch_id from list_documents, a search hit's branch_id, or any id from the branches get_document lists. A protected branch can be read but refuses every write."

	// _keyDocumentID is the shared document-id property name.
	_keyDocumentID = "document_id"

	// _keyName is the shared display-name property name.
	_keyName = "name"

	// _keyBlockUID is the shared block-uid property name.
	_keyBlockUID = "block_uid"

	// _keyBlock is the shared canonical-block property name.
	_keyBlock = "block"

	// _keyQuery is the shared query property name.
	_keyQuery = "query"

	// _keyFrom is the shared range-start property name.
	_keyFrom = "from"

	// _keyTo is the shared range-end property name.
	_keyTo = "to"

	// _keyChartType is the shared chart-type property name.
	_keyChartType = "chart_type"

	// _keyMatchers is the shared Prometheus series-selector property
	// name.
	_keyMatchers = "matchers"

	// _keyLabel is the shared Prometheus label property name.
	_keyLabel = "label"

	// _keyItems is the JSON-schema array-element key.
	_keyItems = "items"

	// _descDocumentID describes the document-id property.
	_descDocumentID = "The document id."

	// _descTargetDocumentID describes a document-id property that names
	// the edit target.
	_descTargetDocumentID = "The target document id."
)

// _blockSchema is the block argument's schema for the write tools. The
// enum is every canonical type: where a block lands decides which types
// are legal, and that rule is checked server-side.
var _blockSchema = blockSchema(block.Types())

// blockSchema builds the block argument's schema with the given set of
// legal types. The per-field prose is deliberately terse: this schema is
// published to MCP clients, which may never receive the server
// instructions, and to the assistant, whose system prompt already
// carries the full block model and pays for every word on every turn.
func blockSchema(types []string) map[string]any {
	return map[string]any{
		_keyType:        _typeObject,
		_keyDescription: "A canonical block. Which fields apply depends on type; supply only that type's own.",
		"properties": map[string]any{
			_keyType: map[string]any{
				_keyType:        _typeString,
				_keyDescription: "The block's canonical type.",
				_keyEnum:        types,
			},
			"uid": map[string]any{
				_keyType:        _typeString,
				_keyDescription: "Leave unset. Uids are generated server-side and are the handle later edits use.",
			},
			"text": map[string]any{
				_keyType:        _typeString,
				_keyDescription: "Inline text for paragraph, heading, blockquote, code, titled_code, mermaid and callout. Raw inside code, titled_code and mermaid; minimal markdown elsewhere. One block is one paragraph; a newline inside text does not start a new one.",
			},
			"attrs": map[string]any{
				_keyType:        _typeObject,
				_keyDescription: "Per-type attributes. Required where the type says so: heading level, titled_code title, image and figma src. Also code language, callout icon, image alt and width, and a metric's chart configuration, whose width is compact, standard or wide.",
				"properties":    attrProps(),
			},
			"items": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "Entries of bullet_list and ordered_list, which are paragraphs; items of metric_grid, and of blockquote or callout when not using text.",
			},
			"children": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "Blocks nested under a single list entry. A list entry is a paragraph; anything else nested under it goes here, never in the entry itself and never in a second entry.",
			},
			"task_items": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "Rows of task_list, each {checked, block}.",
			},
			"left": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "split_doc concept side. Must start with a level-1 heading; split_doc_param_list is legal only here.",
			},
			"right": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "split_doc example side. Holds titled_code or metric only, and is the one place titled_code is legal.",
			},
			"header": map[string]any{
				_keyType:        _typeString,
				_keyDescription: "split_doc_param_list heading, as plain text.",
			},
			"params": map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "split_doc_param_list rows, each {name, type, description}.",
			},
		},
		"required": []string{_keyType},
	}
}

// _enumAttrs are the metric attributes whose legal values are a fixed
// set, listed in the order the block model introduces them.
//
// width is deliberately absent. attrs is one field shared by every
// block type, so a name means the same thing everywhere it appears,
// and width is a metric's size but an image's pixel width. It carries
// no enum here and stays described in the block model instead.
var _enumAttrs = []struct {
	// Name is the attribute's key inside attrs.
	Name string

	// Desc names the block the attribute belongs to and what it sets.
	Desc string
}{
	{document.AttrVisualizationType, "metric: the chart kind."},
	{document.AttrTimeRange, "metric: the window queried."},
	{document.AttrRefreshInterval, "metric: how often the chart re-queries."},
	{document.AttrUnitType, "metric: the unit values are read in. With custom, put the label in unitCustom."},
	{document.AttrSimulationPreset, "metric: draws this generated series in place of the query's own result, for a block documenting a metric that has no real data yet. Omit it to chart the query."},
}

// attrProps builds the attrs sub-schema. Only attributes with a fixed
// value set appear: publishing the set as an enum is what lets a client
// reject a bad value itself, rather than learning the values from prose
// it may never have been given.
func attrProps() map[string]any {
	enums := block.MetricEnums()

	out := map[string]any{
		document.AttrLevel: map[string]any{
			_keyType:        _typeInteger,
			_keyDescription: "heading: the level. Required on a heading.",
			_keyEnum:        []int{1, 2, 3},
		},
	}

	for _, a := range _enumAttrs {
		out[a.Name] = map[string]any{
			_keyType:        _typeString,
			_keyDescription: a.Desc,
			_keyEnum:        enums[a.Name],
		}
	}

	return out
}

// stringProp builds a JSON-schema string property with a description.
func stringProp(desc string) map[string]any {
	return map[string]any{_keyType: _typeString, _keyDescription: desc}
}

// documentIDProp builds the shared document-id property.
func documentIDProp(desc string) map[string]any {
	return map[string]any{_keyDocumentID: stringProp(desc)}
}
