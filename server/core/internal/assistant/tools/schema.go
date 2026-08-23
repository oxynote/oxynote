package tools

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

// stringProp builds a JSON-schema string property with a description.
func stringProp(desc string) map[string]any {
	return map[string]any{_keyType: _typeString, _keyDescription: desc}
}

// documentIDProp builds the shared document-id property.
func documentIDProp(desc string) map[string]any {
	return map[string]any{_keyDocumentID: stringProp(desc)}
}
