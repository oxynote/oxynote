package tools

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// JSON-schema literals shared by the tool definitions below.
const (
	// _keyType is the JSON-schema type key.
	_keyType = "type"

	// _typeString is the JSON-schema string type.
	_typeString = "string"

	// _typeArray is the JSON-schema array type.
	_typeArray = "array"

	// _keyDescription is the JSON-schema description key.
	_keyDescription = "description"

	// _keyDocumentID is the shared document-id property name.
	_keyDocumentID = "document_id"

	// _keyBlockUID is the shared block-uid property name.
	_keyBlockUID = "block_uid"

	// _keyBlock is the shared canonical-block property name.
	_keyBlock = "block"

	// _descDocumentID describes the document-id property.
	_descDocumentID = "The document id."

	// _descTargetDocumentID describes a document-id property that names
	// the edit target.
	_descTargetDocumentID = "The target document id."
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
						_keyType:        _typeString,
						_keyDescription: "Optional. Only return documents under this parent id. Omit for the full tree.",
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descDocumentID,
					},
				},
				Required: []string{_keyDocumentID},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descDocumentID,
					},
				},
				Required: []string{_keyDocumentID},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descDocumentID,
					},
					_keyBlockUID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The block uid to fetch.",
					},
				},
				Required: []string{_keyDocumentID, _keyBlockUID},
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
						_keyType:        _typeString,
						_keyDescription: "The full-text search query (typo-tolerant, matches block text).",
					},
					"limit": map[string]any{
						_keyType:        "integer",
						_keyDescription: "Maximum number of hits to return. Defaults to 20; cap is 50.",
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
						_keyType:        _typeString,
						_keyDescription: "Display name for the new document.",
					},
					"icon": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "Lucide icon identifier (e.g. \"lucide:file-text\"). Defaults to \"lucide:file\" when empty.",
					},
					"parent_id": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "Optional parent document id. Omit to create at the org root.",
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The document id to delete.",
					},
				},
				Required: []string{_keyDocumentID},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descDocumentID,
					},
					"name": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The new display name.",
					},
				},
				Required: []string{_keyDocumentID, "name"},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descDocumentID,
					},
					"icon": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The new icon identifier.",
					},
				},
				Required: []string{_keyDocumentID, "icon"},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The document to move.",
					},
					"new_parent_id": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "Optional new parent document id; omit to move to the root.",
					},
				},
				Required: []string{_keyDocumentID},
			},
		},
	}
}

// _blockSchema is the JSON schema shared by every write tool that
// takes a canonical Block argument. The full shape is documented in
// the system prompt; this object only enumerates the top-level
// fields so Anthropic surfaces the correct argument names.
var _blockSchema = map[string]any{
	_keyType:        "object",
	_keyDescription: "A canonical block. See the system prompt for the per-type schema.",
	"properties": map[string]any{
		_keyType:     map[string]any{_keyType: _typeString},
		"uid":        map[string]any{_keyType: _typeString},
		"text":       map[string]any{_keyType: _typeString},
		"attrs":      map[string]any{_keyType: "object"},
		"items":      map[string]any{_keyType: _typeArray},
		"task_items": map[string]any{_keyType: _typeArray},
		"left":       map[string]any{_keyType: _typeArray},
		"right":      map[string]any{_keyType: _typeArray},
		"header":     map[string]any{_keyType: _typeString},
		"params":     map[string]any{_keyType: _typeArray},
	},
	"required": []string{_keyType},
}

func writeInsertBlock() anthropic.ToolUnionParam {
	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        string(NameInsertBlock),
			Description: anthropic.String("Insert a single canonical block before or after a referenced block in the document. The reference block stays in place. Block uid is generated server-side if omitted."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					"reference_block_uid": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The uid of the block to insert relative to.",
					},
					"position": map[string]any{
						_keyType:        _typeString,
						"enum":          []string{"before", "after"},
						_keyDescription: "Insert side relative to the reference block.",
					},
					_keyBlock: _blockSchema,
				},
				Required: []string{_keyDocumentID, "reference_block_uid", "position", _keyBlock},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlock: _blockSchema,
				},
				Required: []string{_keyDocumentID, _keyBlock},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlock: _blockSchema,
				},
				Required: []string{_keyDocumentID, _keyBlock},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlockUID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The uid of the block being replaced.",
					},
					_keyBlock: _blockSchema,
				},
				Required: []string{_keyDocumentID, _keyBlockUID, _keyBlock},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlockUID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The uid of the block whose text should be replaced.",
					},
					"text": map[string]any{
						_keyType:        _typeString,
						_keyDescription: "New inline text in canonical markdown.",
					},
				},
				Required: []string{_keyDocumentID, _keyBlockUID, "text"},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlockUID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The uid of the block whose attrs should be updated.",
					},
					"attrs": map[string]any{
						_keyType:        "object",
						_keyDescription: "Attribute keys and values to set (e.g. {\"level\": 2}, {\"icon\": \"lucide:warning\"}).",
					},
				},
				Required: []string{_keyDocumentID, _keyBlockUID, "attrs"},
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
					_keyDocumentID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: _descTargetDocumentID,
					},
					_keyBlockUID: map[string]any{
						_keyType:        _typeString,
						_keyDescription: "The uid of the block to delete.",
					},
				},
				Required: []string{_keyDocumentID, _keyBlockUID},
			},
		},
	}
}
