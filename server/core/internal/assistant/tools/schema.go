package tools

import (
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _blockSchema is the block argument's schema for the write tools: one
// variant per type the assistant can write, each naming its type as a
// constant and taking only its own fields. file is absent, since it is
// only ever created by uploading in the editor.
var _blockSchema = blockSchema()

// blockSchema builds the block argument's schema, one variant per
// writable type in documentation order.
func blockSchema() map[string]any {
	types := block.Types()
	variants := make([]map[string]any, 0, len(types))

	for _, t := range types {
		if v, ok := blockVariant(block.Type(t)); ok {
			variants = append(variants, v)
		}
	}

	return map[string]any{
		"description": "A canonical block. Its type decides which fields it takes.",
		"anyOf":       variants,
	}
}

// blockVariant builds the schema of one block type: its type as a
// constant, its uid, the fields it takes and which of them are
// required, and no other field. The second result is false for a type
// the assistant cannot write.
func blockVariant(t block.Type) (map[string]any, bool) {
	var (
		props    map[string]any
		required []string
	)

	switch t {
	case block.BlockParagraph:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Inline text in minimal markdown. One block is one paragraph; a newline inside text does not start a new one."},
			"children": map[string]any{
				"type":        "array",
				"description": "Only when the paragraph is a list or task-list entry: the blocks indented under it, each a paragraph, bullet_list, ordered_list or task_list.",
				"items":       map[string]any{"type": "object"},
			},
		}
	case block.BlockHeading:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Inline text in minimal markdown. One block is one paragraph; a newline inside text does not start a new one."},
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrLevel: map[string]any{
						"type":        "integer",
						"description": "The heading level.",
						"enum":        []int{1, 2, 3},
					},
				},
				"required": []string{document.AttrLevel},
			},
		}
		required = []string{"attrs"}
	case block.BlockBlockquote:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Inline text in minimal markdown. One block is one paragraph; a newline inside text does not start a new one. Use text or items, not both."},
			"items": map[string]any{
				"type":        "array",
				"description": "The blocks it quotes, of any type legal at the document root. Use text or items, not both.",
				"items":       map[string]any{"type": "object"},
			},
		}
	case block.BlockBulletList, block.BlockOrderedList:
		props = map[string]any{
			"items": map[string]any{
				"type":        "array",
				"description": "The entries, each a paragraph block. Blocks nested under an entry go in that paragraph's children.",
				"items":       map[string]any{"type": "object"},
				"minItems":    1,
			},
		}
		required = []string{"items"}
	case block.BlockTaskList:
		props = map[string]any{
			"task_items": map[string]any{
				"type":        "array",
				"description": "The rows, each a checked flag and a paragraph block. Blocks nested under a row go in that paragraph's children.",
				"minItems":    1,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"checked": map[string]any{"type": "boolean"},
						"block":   map[string]any{"type": "object"},
					},
					"required": []string{"block"},
				},
			},
		}
		required = []string{"task_items"}
	case block.BlockCallout:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Inline text in minimal markdown. One block is one paragraph; a newline inside text does not start a new one. Use text or items, not both."},
			"items": map[string]any{
				"type":        "array",
				"description": "The blocks it holds, each a paragraph, bullet_list, ordered_list or task_list. Use text or items, not both.",
				"items":       map[string]any{"type": "object"},
			},
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrIcon: map[string]any{"type": "string", "description": "Iconify identifier; defaults to lucide:text."},
				},
			},
		}
	case block.BlockCode:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Raw source; no markdown."},
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrLanguage: map[string]any{"type": "string", "description": "Optional; leave empty rather than guess."},
				},
			},
		}
	case block.BlockTitledCode:
		props = map[string]any{
			"text": map[string]any{"type": "string", "description": "Raw source; no markdown."},
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrTitle:    map[string]any{"type": "string", "description": "Plain-text title of the block."},
					document.AttrLanguage: map[string]any{"type": "string", "description": "Optional; leave empty rather than guess."},
				},
				"required": []string{document.AttrTitle},
			},
		}
		required = []string{"attrs"}
	case block.BlockMermaid:
		props = map[string]any{"text": map[string]any{"type": "string", "description": "Raw source; no markdown."}}
	case block.BlockHorizontalRule:
		props = map[string]any{}
	case block.BlockImage:
		props = map[string]any{
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrSrc:   map[string]any{"type": "string", "description": "The image address."},
					document.AttrAlt:   map[string]any{"type": "string", "description": "Alternative text."},
					document.AttrTitle: map[string]any{"type": "string", "description": "Caption."},
					document.AttrWidth: map[string]any{"type": "integer", "description": "Display width in pixels."},
				},
				"required": []string{document.AttrSrc},
			},
		}
		required = []string{"attrs"}
	case block.BlockFigma:
		props = map[string]any{
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrSrc:    map[string]any{"type": "string", "description": "The Figma frame address."},
					document.AttrWidth:  map[string]any{"type": "integer", "description": "Display width in pixels."},
					document.AttrHeight: map[string]any{"type": "integer", "description": "Display height in pixels."},
				},
				"required": []string{document.AttrSrc},
			},
		}
		required = []string{"attrs"}
	case block.BlockMetric:
		props = map[string]any{
			"attrs": map[string]any{
				"type":        "object",
				"description": "The chart configuration: the data source, the queries, the window and the display settings, as the block model describes them. Legal only inside a metric_grid or on a split_doc right side.",
				"properties":  metricAttrProps(),
			},
		}
	case block.BlockMetricGrid:
		props = map[string]any{
			"items": map[string]any{
				"type":        "array",
				"description": "The metric blocks it holds.",
				"items":       map[string]any{"type": "object"},
				"minItems":    1,
			},
		}
		required = []string{"items"}
	case block.BlockSplitDoc:
		props = map[string]any{
			"left": map[string]any{
				"type":        "array",
				"description": "The concept side. Must start with a level-1 heading; then paragraphs, lists and callouts; then any split_doc_param_list, which is legal only here.",
				"items":       map[string]any{"type": "object"},
				"minItems":    1,
			},
			"right": map[string]any{
				"type":        "array",
				"description": "The example side. Holds titled_code or metric only, and is the one place titled_code is legal.",
				"items":       map[string]any{"type": "object"},
				"minItems":    1,
			},
			"attrs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					document.AttrInversed: map[string]any{"type": "boolean", "description": "Flips the two sides."},
				},
			},
		}
		required = []string{"left", "right"}
	case block.BlockParamList:
		props = map[string]any{
			"header": map[string]any{"type": "string", "description": "The section header, as plain text."},
			"params": map[string]any{
				"type":        "array",
				"description": "The rows.",
				"minItems":    1,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string", "description": "The parameter name, as plain text."},
						"type":        map[string]any{"type": "string", "description": "The parameter type, as plain text."},
						"description": map[string]any{"type": "string", "description": "The description, in minimal markdown."},
					},
					"required": []string{"name"},
				},
			},
		}
		required = []string{"header", "params"}
	default:
		return nil, false
	}

	props["type"] = map[string]any{"const": string(t)}
	props["uid"] = map[string]any{"type": "string", "description": "Leave unset; generated server-side."}

	return map[string]any{
		"title":                string(t),
		"type":                 "object",
		"properties":           props,
		"required":             append([]string{"type"}, required...),
		"additionalProperties": false,
	}, true
}

// _enumAttrs are the metric attributes whose legal values are a fixed
// set, listed in the order the block model introduces them.
var _enumAttrs = []struct {
	// Name is the attribute's key inside attrs.
	Name string

	// Desc says what the attribute sets.
	Desc string
}{
	{document.AttrVisualizationType, "The chart kind."},
	{document.AttrTimeRange, "The window queried."},
	{document.AttrRefreshInterval, "How often the chart re-queries."},
	{document.AttrUnitType, "The unit values are read in. With custom, put the label in unitCustom."},
	{document.AttrWidth, "The chart's size."},
	{document.AttrSimulationPreset, "Draws this generated series in place of the query's own result, for a block documenting a metric that has no real data yet. Omit it to chart the query."},
}

// metricAttrProps builds the metric attrs sub-schema. Only attributes
// with a fixed value set appear: publishing the set as an enum is what
// lets a client reject a bad value itself, rather than learning the
// values from prose it may never have been given.
func metricAttrProps() map[string]any {
	enums := block.MetricEnums()
	out := make(map[string]any, len(_enumAttrs))

	for _, a := range _enumAttrs {
		out[a.Name] = map[string]any{
			"type":        "string",
			"description": a.Desc,
			"enum":        enums[a.Name],
		}
	}

	return out
}
