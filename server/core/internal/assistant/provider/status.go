package provider

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

// _claudeModels maps the supported Anthropic model ids to the status
// running the assistant on them yields.
var _claudeModels = map[string]Status{
	"claude-fable-5":    StatusActive,
	"claude-opus-5":     StatusActive,
	"claude-opus-4-6":   StatusActive,
	"claude-sonnet-5":   StatusActiveButWeak,
	"claude-sonnet-4-6": StatusActiveButWeak,
	"claude-haiku-4-5":  StatusInactiveTooWeak,
}

// _openAIModels maps the supported OpenAI model ids to the status
// running the assistant on them yields.
var _openAIModels = map[string]Status{
	"gpt-5.1":    StatusActive,
	"gpt-5":      StatusActive,
	"gpt-5-mini": StatusActiveButWeak,
	"gpt-5-nano": StatusInactiveTooWeak,
}

// _geminiModels maps the supported Gemini model ids to the status
// running the assistant on them yields.
var _geminiModels = map[string]Status{
	"gemini-3-pro-preview":  StatusActive,
	"gemini-2.5-pro":        StatusActive,
	"gemini-2.5-flash":      StatusActiveButWeak,
	"gemini-2.5-flash-lite": StatusInactiveTooWeak,
}

// _ollamaModels maps the supported self-hosted model ids to the status
// running the assistant on them yields. No local model reaches full
// strength: the assistant is entirely tool-driven, and none of these
// call tools against a large schema as reliably as the hosted models.
var _ollamaModels = map[string]Status{
	"llama3.3:70b": StatusActiveButWeak,
	"qwen3:32b":    StatusActiveButWeak,
}

// _modelStatuses lists, per provider, every supported model id and the
// status running the assistant on it yields. A model missing from its
// provider's list is not supported and reports StatusInactive.
var _modelStatuses = map[Provider]map[string]Status{
	ProviderClaude:     _claudeModels,
	ProviderOpenAI:     _openAIModels,
	ProviderGemini:     _geminiModels,
	ProviderOllama:     _ollamaModels,
	ProviderOpenRouter: openRouterModels(),
}

// _defaultModels names the model each provider falls back on when the
// operator does not set one: the strongest supported model, so an
// unconfigured deployment starts at full strength rather than at the
// cheapest tier.
var _defaultModels = map[Provider]string{
	ProviderClaude:     "claude-opus-5",
	ProviderOpenAI:     "gpt-5.1",
	ProviderGemini:     "gemini-2.5-pro",
	ProviderOllama:     "llama3.3:70b",
	ProviderOpenRouter: "anthropic/claude-opus-5",
}

// DefaultModel returns the model the provider defaults to when the
// operator does not set one, or an empty string for a provider this
// build does not support.
func (p Provider) DefaultModel() string {
	return _defaultModels[p]
}

// ModelStatus validates the options and classifies the configured
// model against the provider's supported list: whether the assistant
// runs at full strength, runs with limitations, or does not run at
// all. A model outside the list reports StatusInactive.
func (o Options) ModelStatus() (Status, error) {
	if err := o.validate(); err != nil {
		return StatusInactive, err
	}

	status, ok := _modelStatuses[o.Provider][o.Model]
	if !ok {
		return StatusInactive, nil
	}

	return status, nil
}

// openRouterModels derives the OpenRouter list from the vendor lists:
// OpenRouter serves the same models behind a vendor-prefixed id, so
// the support decision stays stated once, on the vendor's own list.
func openRouterModels() map[string]Status {
	vendors := map[string]map[string]Status{
		"anthropic/": _claudeModels,
		"openai/":    _openAIModels,
		"google/":    _geminiModels,
	}

	models := make(map[string]Status)

	for prefix, vendor := range vendors {
		for id, status := range vendor {
			models[prefix+id] = status
		}
	}

	return models
}
