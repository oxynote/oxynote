package provider

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Status_Active(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Status Status
		Result bool
	}{
		"Active runs":             {Status: StatusActive, Result: true},
		"Active but weak runs":    {Status: StatusActiveButWeak, Result: true},
		"Too weak does not run":   {Status: StatusInactiveTooWeak},
		"Inactive does not run":   {Status: StatusInactive},
		"Zero value does not run": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Status.Active())
		})
	}
}

func Test_Options_ModelStatus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts   Options
		Result Status
		Err    error
	}{
		"Invalid options": {
			Opts: Options{Provider: ProviderClaude, Model: "claude-opus-5"},
			Err:  ErrMissingAPIKey,
		},
		"Unsupported model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-2", APIKey: "k"},
			Result: StatusInactive,
		},
		"Full strength model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-opus-5", APIKey: "k"},
			Result: StatusActive,
		},
		"Weaker model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-sonnet-5", APIKey: "k"},
			Result: StatusActiveButWeak,
		},
		"Too weak model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-haiku-4-5", APIKey: "k"},
			Result: StatusInactiveTooWeak,
		},
		"Supported ollama model": {
			Opts: Options{
				Provider: ProviderOllama,
				Model:    "llama3.3:70b",
				BaseURL:  "http://localhost:11434",
			},
			Result: StatusActiveButWeak,
		},
		"Supported openrouter model": {
			Opts:   Options{Provider: ProviderOpenRouter, Model: "anthropic/claude-opus-5", APIKey: "k"},
			Result: StatusActive,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			status, err := c.Opts.ModelStatus()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, status)
		})
	}
}

func Test_Provider_DefaultModel(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Provider Provider
		Result   string
	}{
		"Claude":           {Provider: ProviderClaude, Result: "claude-opus-5"},
		"OpenAI":           {Provider: ProviderOpenAI, Result: "gpt-5.1"},
		"Gemini":           {Provider: ProviderGemini, Result: "gemini-2.5-pro"},
		"Ollama":           {Provider: ProviderOllama, Result: "llama3.3:70b"},
		"OpenRouter":       {Provider: ProviderOpenRouter, Result: "anthropic/claude-opus-5"},
		"Unknown provider": {Provider: "cohere"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.Provider.DefaultModel()
			assert.Equal(t, c.Result, result)

			if result == "" {
				return
			}

			// a default that is not on its provider's supported list
			// would disable the assistant instead of enabling it.
			assert.Contains(t, _modelStatuses[c.Provider], result)
		})
	}
}

func Test_openRouterModels(t *testing.T) {
	t.Parallel()

	models := openRouterModels()

	assert.Len(t, models, len(_claudeModels)+len(_openAIModels)+len(_geminiModels))
	assert.Equal(t, StatusActive, models["anthropic/claude-opus-5"])
	assert.Equal(t, StatusActive, models["openai/gpt-5"])
	assert.Equal(t, StatusActive, models["google/gemini-2.5-pro"])
	assert.Equal(t, StatusInactiveTooWeak, models["anthropic/claude-haiku-4-5"])
}
