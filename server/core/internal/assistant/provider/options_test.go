package provider

import (
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/ptrutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Options_validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts Options
		Err  error
	}{
		"Unsupported provider": {
			Opts: Options{Provider: "cohere", Model: "x", APIKey: "k"},
			Err:  assert.AnError,
		},
		"Missing model": {
			Opts: Options{Provider: ProviderOpenAI, APIKey: "k"},
			Err:  ErrMissingModel,
		},
		"Ollama without base url": {
			Opts: Options{Provider: ProviderOllama, Model: "llama3.3"},
			Err:  ErrMissingBaseURL,
		},
		"OpenAI without api key": {
			Opts: Options{Provider: ProviderOpenAI, Model: "gpt-5"},
			Err:  ErrMissingAPIKey,
		},
		"Gemini without api key": {
			Opts: Options{Provider: ProviderGemini, Model: "gemini-2.5-pro"},
			Err:  ErrMissingAPIKey,
		},
		"OpenRouter without api key": {
			Opts: Options{Provider: ProviderOpenRouter, Model: "x/y"},
			Err:  ErrMissingAPIKey,
		},
		"Claude without api key or cloud credentials": {
			Opts: Options{Provider: ProviderClaude, Model: "claude-opus-4-6"},
			Err:  ErrMissingAPIKey,
		},
		"Claude with api key": {
			Opts: Options{Provider: ProviderClaude, Model: "claude-opus-4-6", APIKey: "k"},
		},
		"Claude via bedrock": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				Bedrock:  BedrockOptions{Enabled: true},
			},
		},
		"Claude via vertex": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				Vertex:   VertexOptions{Enabled: true},
			},
		},
		"Ollama with base url and no api key": {
			Opts: Options{
				Provider: ProviderOllama,
				Model:    "llama3.3",
				BaseURL:  "http://localhost:11434",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Opts.validate()

			testutil.RequireEqualError(t, c.Err, err)
		})
	}
}

func Test_Options_requiresAPIKey(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts   Options
		Result bool
	}{
		"Ollama never does":            {Opts: Options{Provider: ProviderOllama}},
		"OpenAI does":                  {Opts: Options{Provider: ProviderOpenAI}, Result: true},
		"Gemini does":                  {Opts: Options{Provider: ProviderGemini}, Result: true},
		"OpenRouter does":              {Opts: Options{Provider: ProviderOpenRouter}, Result: true},
		"Claude does when direct":      {Opts: Options{Provider: ProviderClaude}, Result: true},
		"Claude does not with bedrock": {Opts: Options{Provider: ProviderClaude, Bedrock: BedrockOptions{Enabled: true}}},
		"Claude does not with vertex":  {Opts: Options{Provider: ProviderClaude, Vertex: VertexOptions{Enabled: true}}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Opts.requiresAPIKey())
		})
	}
}

func Test_Options_timeout(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts   Options
		Result time.Duration
	}{
		"Configured timeout is used": {
			Opts:   Options{RequestTimeout: time.Minute},
			Result: time.Minute,
		},
		"Zero falls back to the default": {
			Opts:   Options{},
			Result: _defaultRequestTimeout,
		},
		"Negative falls back to the default": {
			Opts:   Options{RequestTimeout: -time.Second},
			Result: _defaultRequestTimeout,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Opts.timeout())
		})
	}
}

func Test_Options_maxTokens(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts   Options
		Result *int
	}{
		"Configured cap is returned": {
			Opts:   Options{MaxTokens: 1024},
			Result: ptrutil.New(1024),
		},
		"Zero leaves the provider default":     {Opts: Options{}},
		"Negative leaves the provider default": {Opts: Options{MaxTokens: -1}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Opts.maxTokens())
		})
	}
}
