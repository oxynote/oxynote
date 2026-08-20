package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// the Google API transport behind the Gemini client starts an
	// opencensus worker from a package init, so it is already running
	// before any test does.
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func Test_Provider_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Provider Provider
		Err      error
	}{
		"Claude is supported":     {Provider: ProviderClaude},
		"OpenAI is supported":     {Provider: ProviderOpenAI},
		"Gemini is supported":     {Provider: ProviderGemini},
		"Ollama is supported":     {Provider: ProviderOllama},
		"OpenRouter is supported": {Provider: ProviderOpenRouter},
		"Unknown provider": {
			Provider: "cohere",
			Err:      ErrInvalidProvider,
		},
		"Empty provider": {
			Provider: "",
			Err:      ErrInvalidProvider,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Provider.Validate()
			if c.Err == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, c.Err)

			// the message must name the offending value and the
			// supported set, since documentation is the only guidance
			// an operator gets.
			assert.Contains(t, err.Error(), string(c.Provider))
			assert.Contains(t, err.Error(), Supported())
		})
	}
}

func Test_ParseProvider(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp    string
		Result Provider
	}{
		"Exact match":         {Inp: "claude", Result: ProviderClaude},
		"Mixed case":          {Inp: "OpenAI", Result: ProviderOpenAI},
		"Surrounding spaces":  {Inp: "  gemini  ", Result: ProviderGemini},
		"Upper case and tabs": {Inp: "\tOLLAMA\n", Result: ProviderOllama},
		"Unknown passes through": {
			Inp:    "cohere",
			Result: Provider("cohere"),
		},
		"Empty": {Inp: "", Result: Provider("")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, ParseProvider(c.Inp))
		})
	}
}

func Test_Supported(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "claude, openai, gemini, ollama, openrouter", Supported())
}

func Test_New(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts        Options
		Err         error
		ErrContains string
	}{
		"Invalid provider": {
			Opts: Options{Provider: "cohere", Model: "x"},
			Err:  ErrInvalidProvider,
		},
		"Missing model": {
			Opts: Options{Provider: ProviderClaude, APIKey: "k"},
			Err:  ErrMissingModel,
		},
		"Missing api key": {
			Opts: Options{Provider: ProviderOpenAI, Model: "gpt-5"},
			Err:  ErrMissingAPIKey,
		},
		"Claude": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				APIKey:   "k",
			},
		},
		"Claude via bedrock needs no api key": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "anthropic.claude-opus-4-6",
				Bedrock: BedrockOptions{
					Enabled:         true,
					Region:          "us-east-1",
					AccessKey:       "ak",
					SecretAccessKey: "sk",
				},
			},
		},
		"OpenAI": {
			Opts: Options{
				Provider: ProviderOpenAI,
				Model:    "gpt-5",
				APIKey:   "k",
			},
		},
		"OpenAI against a compatible endpoint": {
			Opts: Options{
				Provider: ProviderOpenAI,
				Model:    "llama-3.3-70b",
				APIKey:   "k",
				BaseURL:  "https://example.invalid/v1",
			},
		},
		"Gemini": {
			Opts: Options{
				Provider: ProviderGemini,
				Model:    "gemini-2.5-pro",
				APIKey:   "k",
			},
		},
		"Ollama": {
			Opts: Options{
				Provider: ProviderOllama,
				Model:    "llama3.3",
				BaseURL:  "http://localhost:11434",
			},
		},
		"OpenRouter": {
			Opts: Options{
				Provider: ProviderOpenRouter,
				Model:    "anthropic/claude-opus-4.6",
				APIKey:   "k",
			},
		},
		"Claude with optional tuning": {
			Opts: tunedOptions(ProviderClaude, "claude-opus-4-6"),
		},
		"OpenAI with optional tuning": {
			Opts: tunedOptions(ProviderOpenAI, "gpt-5"),
		},
		"Gemini with optional tuning": {
			Opts: tunedOptions(ProviderGemini, "gemini-2.5-pro"),
		},
		"OpenRouter with optional tuning": {
			Opts: tunedOptions(ProviderOpenRouter, "anthropic/claude-opus-4.6"),
		},
		"Ollama with optional tuning": {
			Opts: func() Options {
				o := tunedOptions(ProviderOllama, "llama3.3")
				o.BaseURL = "http://localhost:11434"

				return o
			}(),
		},
		"Claude via vertex without a project id": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				Vertex:   VertexOptions{Enabled: true, Region: "us-east5"},
			},
			ErrContains: "project ID",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cm, err := New(context.Background(), c.Opts)

			if c.Err != nil {
				require.ErrorIs(t, err, c.Err)
				assert.Nil(t, cm)

				return
			}

			if c.ErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.ErrContains)
				assert.Nil(t, cm)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, cm)
		})
	}
}

// tunedOptions builds options exercising every optional model setting,
// so the adapters' conditional field wiring is covered.
func tunedOptions(p Provider, model string) Options {
	temperature := float32(0.3)

	return Options{
		Provider:       p,
		Model:          model,
		APIKey:         "k",
		BaseURL:        "https://example.invalid/v1",
		MaxTokens:      1024,
		Temperature:    &temperature,
		RequestTimeout: time.Minute,
	}
}
