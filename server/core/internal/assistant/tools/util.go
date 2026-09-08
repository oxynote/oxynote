package tools

import (
	"encoding/json"
	"fmt"
)

// result is the single place tool result envelopes are serialised;
// centralising it keeps the JSON shape consistent.
func result(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshalling tool result: %w", err)
	}

	return string(data), nil
}

// errRequired reports an argument the tool cannot act without.
func errRequired(key string) error {
	return fmt.Errorf("%s is required", key)
}
