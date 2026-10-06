package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Descriptions of the arguments several tools share, so each is worded
// once.
const (
	// _documentIDDescription describes a document_id argument.
	_documentIDDescription = "The document id."

	// _branchIDDescription describes a branch_id argument.
	_branchIDDescription = "The branch id, from list_documents, search_documents or get_document. A protected branch refuses writes."

	// _dataSourceIDDescription describes a data_source_id argument.
	_dataSourceIDDescription = "The data source id, from list_data_sources."

	// _tagIDDescription describes a tag_id argument.
	_tagIDDescription = "The tag id, from list_tags or a document's tags."

	// _hookIDDescription describes a hook_id argument.
	_hookIDDescription = "The hook id, from list_hooks."

	// _fromDescription describes the start of a time range.
	_fromDescription = "Optional. Range start, RFC 3339. Defaults to an hour before to."

	// _toDescription describes the end of a time range.
	_toDescription = "Optional. Range end, RFC 3339. Defaults to now."

	// _matchersDescription describes a list of PromQL series selectors.
	_matchersDescription = `PromQL series selectors, such as ["up", "{job=\"api\"}"].`
)

// result is the single place tool result envelopes are serialised;
// centralising it keeps the JSON shape consistent. <, > and & stay as
// they are: escaped, they cost the model six characters each, and
// document content is full of them.
func result(v any) (string, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("marshalling tool result: %w", err)
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// countPhrase phrases a count of the noun for a summary, such as
// "1 page" or "3 pages".
func countPhrase(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

// sentence joins clauses as "A, b and c", with the first letter
// capitalised.
func sentence(clauses []string) string {
	s := clauses[len(clauses)-1]
	if len(clauses) > 1 {
		s = strings.Join(clauses[:len(clauses)-1], ", ") + " and " + s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}

// errRequired reports an argument the tool cannot act without.
func errRequired(key string) error {
	return fmt.Errorf("%s is required", key)
}
