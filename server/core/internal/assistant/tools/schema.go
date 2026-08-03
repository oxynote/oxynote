package tools

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// AnthropicTools returns the tool definitions sent to Anthropic.
// The schemas describe canonical-layer inputs: blocks use the
// canonical (snake_case, macro) shape, never raw ProseMirror JSON.
// The full schema documentation lives inline in the system prompt
// so the model has the per-type rules without needing a separate
// discovery tool.
func AnthropicTools() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		readListDocuments(),
		readGetDocument(),
		readDocumentSummaryTool(),
		readBlockTool(),
		searchDocumentsTool(),

		writeCreateDocument(),
		writeDeleteDocument(),
		writeRenameDocument(),
		writeSetDocumentIcon(),
		writeMoveDocument(),
		writeInsertBlock(),
		writeAppendBlock(),
		writePrependBlock(),
		writeReplaceBlock(),
		writeUpdateBlockText(),
		writeUpdateBlockAttrs(),
		writeDeleteBlock(),
	}
}

func readListDocuments() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameListDocuments),
			Description: anthropic.String("List documents in the organisation. Returns the document tree (id, name, icon, children). Use parent_id to scope to one subtree; omit to list the whole org."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"parent_id": map[string]any{
						"type":        "string",
						"description": "Optional. Only return documents under this parent id. Omit for the full tree.",
					},
				},
			},
		},
	}
}

func readGetDocument() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameGetDocument),
			Description: anthropic.String("Fetch metadata for one document: name, icon, parent_id, default branch id, protected flag, updated_at."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id.",
					},
				},
				Required: []string{"document_id"},
			},
		},
	}
}

func readDocumentSummaryTool() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameReadDocumentSummary),
			Description: anthropic.String("Return an ordered, compact summary of a document: per-block uid, kind, flattened source_text, surrounding_context (ancestor headings, list intro, doc title), and tags. Use this as your default way to read a document — it's ~5-10x cheaper than fetching full content."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id.",
					},
				},
				Required: []string{"document_id"},
			},
		},
	}
}

func readBlockTool() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameReadBlock),
			Description: anthropic.String("Return the full canonical content of one block by uid, including any nested children. Use this only when read_document_summary doesn't carry enough detail (e.g. you need the full structure of a split_doc or split_doc_param_list to edit it)."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id.",
					},
					"block_uid": map[string]any{
						"type":        "string",
						"description": "The block uid to fetch.",
					},
				},
				Required: []string{"document_id", "block_uid"},
			},
		},
	}
}

func searchDocumentsTool() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameSearchDocuments),
			Description: anthropic.String("Full-text search across all documents in the organisation for blocks whose text matches the query. Returns hits with document_id, document_name, block_uid, text. Use this to find relevant context before editing or to surface candidate documents the user might be asking about."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The full-text search query (typo-tolerant, matches block text).",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of hits to return. Defaults to 20; cap is 50.",
					},
				},
				Required: []string{"query"},
			},
		},
	}
}

func writeCreateDocument() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameCreateDocument),
			Description: anthropic.String("Create a new document. Returns {document_id, branch_id}. The document starts with one empty paragraph; immediately follow up with append_block / insert_block calls to populate it."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Display name for the new document.",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "Lucide icon identifier (e.g. \"lucide:file-text\"). Defaults to \"lucide:file\" when empty.",
					},
					"parent_id": map[string]any{
						"type":        "string",
						"description": "Optional parent document id. Omit to create at the org root.",
					},
				},
				Required: []string{"name"},
			},
		},
	}
}

func writeDeleteDocument() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameDeleteDocument),
			Description: anthropic.String("Delete a document. This is destructive — the user is always asked to confirm and there is no auto-approve."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id to delete.",
					},
				},
				Required: []string{"document_id"},
			},
		},
	}
}

func writeRenameDocument() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameRenameDocument),
			Description: anthropic.String("Change a document's display name. The change is applied live via hocuspocus."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "The new display name.",
					},
				},
				Required: []string{"document_id", "name"},
			},
		},
	}
}

func writeSetDocumentIcon() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameSetDocumentIcon),
			Description: anthropic.String("Change a document's icon identifier (Lucide-style, e.g. \"lucide:rocket\")."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document id.",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "The new icon identifier.",
					},
				},
				Required: []string{"document_id", "icon"},
			},
		},
	}
}

func writeMoveDocument() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameMoveDocument),
			Description: anthropic.String("Re-parent a document. Omit new_parent_id to move the document to the org root."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The document to move.",
					},
					"new_parent_id": map[string]any{
						"type":        "string",
						"description": "Optional new parent document id; omit to move to the root.",
					},
				},
				Required: []string{"document_id"},
			},
		},
	}
}

// blockSchema is the JSON schema shared by every write tool that
// takes a canonical Block argument. The full shape is documented in
// the system prompt; this object only enumerates the top-level
// fields so Anthropic surfaces the correct argument names.
var blockSchema = map[string]any{
	"type":        "object",
	"description": "A canonical block. See the system prompt for the per-type schema.",
	"properties": map[string]any{
		"type":        map[string]any{"type": "string"},
		"uid":         map[string]any{"type": "string"},
		"text":        map[string]any{"type": "string"},
		"attrs":       map[string]any{"type": "object"},
		"items":       map[string]any{"type": "array"},
		"task_items":  map[string]any{"type": "array"},
		"left":        map[string]any{"type": "array"},
		"right":       map[string]any{"type": "array"},
		"header":      map[string]any{"type": "string"},
		"params":      map[string]any{"type": "array"},
	},
	"required": []string{"type"},
}

func writeInsertBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameInsertBlock),
			Description: anthropic.String("Insert a single canonical block before or after a referenced block in the document. The reference block stays in place. Block uid is generated server-side if omitted."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"reference_block_uid": map[string]any{
						"type":        "string",
						"description": "The uid of the block to insert relative to.",
					},
					"position": map[string]any{
						"type":        "string",
						"enum":        []string{"before", "after"},
						"description": "Insert side relative to the reference block.",
					},
					"block": blockSchema,
				},
				Required: []string{"document_id", "reference_block_uid", "position", "block"},
			},
		},
	}
}

func writeAppendBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameAppendBlock),
			Description: anthropic.String("Append a single canonical block at the end of a document."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block": blockSchema,
				},
				Required: []string{"document_id", "block"},
			},
		},
	}
}

func writePrependBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NamePrependBlock),
			Description: anthropic.String("Prepend a single canonical block at the start of a document."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block": blockSchema,
				},
				Required: []string{"document_id", "block"},
			},
		},
	}
}

func writeReplaceBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameReplaceBlock),
			Description: anthropic.String("Replace an existing block by uid with a new block. The replacement keeps the same position in its parent. Useful for changing a block's type or its full structure."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block_uid": map[string]any{
						"type":        "string",
						"description": "The uid of the block being replaced.",
					},
					"block": blockSchema,
				},
				Required: []string{"document_id", "block_uid", "block"},
			},
		},
	}
}

func writeUpdateBlockText() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameUpdateBlockText),
			Description: anthropic.String("Replace the inline text of a text-bearing block (paragraph, heading, blockquote, code, mermaid, callout-shorthand). The block's type and attrs are preserved. text uses the canonical minimal-markdown subset (**bold**, *italic*, _underline_, ~~strike~~, `code`, [label](url)). For code/mermaid blocks, text is raw and markdown is not parsed."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block_uid": map[string]any{
						"type":        "string",
						"description": "The uid of the block whose text should be replaced.",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "New inline text in canonical markdown.",
					},
				},
				Required: []string{"document_id", "block_uid", "text"},
			},
		},
	}
}

func writeUpdateBlockAttrs() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameUpdateBlockAttrs),
			Description: anthropic.String("Set or override the named attributes on an existing block. Unmentioned attributes are preserved; the uid attribute cannot be changed."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block_uid": map[string]any{
						"type":        "string",
						"description": "The uid of the block whose attrs should be updated.",
					},
					"attrs": map[string]any{
						"type":        "object",
						"description": "Attribute keys and values to set (e.g. {\"level\": 2}, {\"icon\": \"lucide:warning\"}).",
					},
				},
				Required: []string{"document_id", "block_uid", "attrs"},
			},
		},
	}
}

func writeDeleteBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameDeleteBlock),
			Description: anthropic.String("Delete a block from the document by uid. Destructive — the user is always asked to confirm."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The target document id.",
					},
					"block_uid": map[string]any{
						"type":        "string",
						"description": "The uid of the block to delete.",
					},
				},
				Required: []string{"document_id", "block_uid"},
			},
		},
	}
}
