package tools

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
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

// _blockSchema is the JSON schema shared by every write tool that takes
// a canonical Block argument. The full shape is documented in the
// system prompt; this object only enumerates the top-level fields so
// the model surfaces the correct argument names.
var _blockSchema = map[string]any{
	_keyType:        _typeObject,
	_keyDescription: "A canonical block. See the system prompt for the per-type schema.",
	"properties": map[string]any{
		_keyType:     map[string]any{_keyType: _typeString},
		"uid":        map[string]any{_keyType: _typeString},
		"text":       map[string]any{_keyType: _typeString},
		"attrs":      map[string]any{_keyType: _typeObject},
		"items":      map[string]any{_keyType: _typeArray},
		"children":   map[string]any{_keyType: _typeArray},
		"task_items": map[string]any{_keyType: _typeArray},
		"left":       map[string]any{_keyType: _typeArray},
		"right":      map[string]any{_keyType: _typeArray},
		"header":     map[string]any{_keyType: _typeString},
		"params":     map[string]any{_keyType: _typeArray},
	},
	"required": []string{_keyType},
}

// toolInfo builds a tool's model-facing description from its name, its
// prose, and the JSON-schema fragment describing its arguments.
//
// The property map is transported through JSON because it is already a
// JSON-schema fragment, which keeps each tool's schema the single
// source of truth rather than restating it in a second vocabulary.
func toolInfo(name Name, desc string, properties map[string]any, required ...string) (*schema.ToolInfo, error) {
	// an empty variadic is a non-nil empty slice, which would emit
	// "required": [] where the schema should simply not say anything.
	if len(required) == 0 {
		required = nil
	}

	raw, err := json.Marshal(map[string]any{
		_keyType:     _typeObject,
		"properties": properties,
		"required":   required,
	})
	if err != nil {
		// NOCOV: the property maps are literals of JSON-encodable types.
		return nil, fmt.Errorf("marshalling schema: %w", err)
	}

	js := &jsonschema.Schema{}
	if err := json.Unmarshal(raw, js); err != nil {
		// NOCOV: the payload was just produced by json.Marshal.
		return nil, fmt.Errorf("unmarshalling schema: %w", err)
	}

	return &schema.ToolInfo{
		Name:        string(name),
		Desc:        desc,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(js),
	}, nil
}

// stringProp builds a JSON-schema string property with a description.
func stringProp(desc string) map[string]any {
	return map[string]any{_keyType: _typeString, _keyDescription: desc}
}

// documentIDProp builds the shared document-id property.
func documentIDProp(desc string) map[string]any {
	return map[string]any{_keyDocumentID: stringProp(desc)}
}
