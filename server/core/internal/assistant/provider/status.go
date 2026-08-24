package provider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

const (
	// StatusActive specifies that the configured model meets the
	// assistant's recommended strength.
	StatusActive Status = "active"

	// StatusActiveButWeak specifies that the configured model runs the
	// assistant, but with limitations the operator should expect.
	StatusActiveButWeak Status = "active-but-weak"

	// StatusInactiveTooWeak specifies that the configured model is too
	// weak to run the assistant, which is therefore disabled.
	StatusInactiveTooWeak Status = "inactive-too-weak"

	// StatusInactive specifies that the assistant is disabled: either
	// no provider is configured or the configured model is not one this
	// build supports.
	StatusInactive Status = "inactive"
)

// Status specifies the assistant's availability, decided by the
// strength of the configured model.
type Status string

// Active reports whether the assistant runs at all under this status.
func (s Status) Active() bool {
	return s == StatusActive || s == StatusActiveButWeak
}

// modelList describes one provider's entry in the embedded model list.
type modelList struct {
	// Default specifies the model used when the operator does not set
	// one: the provider's strongest supported model.
	Default string `json:"default"`

	// Models groups the supported model ids by the status running the
	// assistant on them yields. A model in no group is not supported
	// and reports StatusInactive.
	Models map[Status][]string `json:"models"`
}

// status returns the status running the assistant on the given model
// yields, or StatusInactive when the model is not on the list.
func (ml modelList) status(model string) Status {
	for status, ids := range ml.Models {
		if slices.Contains(ids, model) {
			return status
		}
	}

	return StatusInactive
}

// _modelsJSON is the embedded per-provider model list: every supported
// model id with its status, and the default model, per provider. The
// OpenRouter entries mirror the vendors' own ids behind their vendor
// prefix.
//
//go:embed models/models.json
var _modelsJSON []byte

// _models holds the parsed model list; _modelsErr holds the parse
// failure, which ModelStatus reports instead of silently judging every
// model against an empty list.
var _models, _modelsErr = parseModels(_modelsJSON)

// ModelStatus validates the options and classifies the configured
// model against the provider's supported list: whether the assistant
// runs at full strength, runs with limitations, or does not run at
// all. A model outside the list reports StatusInactive.
func (o Options) ModelStatus() (Status, error) {
	if _modelsErr != nil {
		// NOCOV: the embedded file is pinned valid by the parse test.
		return StatusInactive, fmt.Errorf("parsing the embedded model list: %w", _modelsErr)
	}

	if err := o.validate(); err != nil {
		return StatusInactive, err
	}

	return _models[o.Provider].status(o.Model), nil
}

// DefaultModel returns the model the provider defaults to when the
// operator does not set one, or an empty string for a provider this
// build does not support.
func (p Provider) DefaultModel() string {
	return _models[p].Default
}

// parseModels decodes a per-provider model list from raw JSON.
func parseModels(data []byte) (map[Provider]modelList, error) {
	models := make(map[Provider]modelList)
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, err
	}

	return models, nil
}
