package provider

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func Test_newOllama(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts Options

		// Config describes the model newOllama should have built.
		Config *ollama.ChatModelConfig
	}{
		"Defaults": {
			Opts: Options{
				Provider: ProviderOllama,
				Model:    "llama3.3",
				BaseURL:  "http://localhost:11434",
			},
			Config: &ollama.ChatModelConfig{
				BaseURL: "http://localhost:11434",
				Model:   "llama3.3",
				Timeout: _defaultRequestTimeout,
			},
		},
		"Temperature only": {
			Opts: Options{
				Provider:    ProviderOllama,
				Model:       "llama3.3",
				BaseURL:     "http://localhost:11434",
				Temperature: new(float32(0.3)),
			},
			Config: &ollama.ChatModelConfig{
				BaseURL: "http://localhost:11434",
				Model:   "llama3.3",
				Timeout: _defaultRequestTimeout,
				Options: &ollama.Options{Temperature: 0.3},
			},
		},
		"Max tokens only": {
			Opts: Options{
				Provider:  ProviderOllama,
				Model:     "llama3.3",
				BaseURL:   "http://localhost:11434",
				MaxTokens: 1024,
			},
			Config: &ollama.ChatModelConfig{
				BaseURL: "http://localhost:11434",
				Model:   "llama3.3",
				Timeout: _defaultRequestTimeout,
				Options: &ollama.Options{NumPredict: 1024},
			},
		},
		"Full tuning": {
			Opts: Options{
				Provider:       ProviderOllama,
				Model:          "llama3.3",
				BaseURL:        "http://localhost:11434",
				MaxTokens:      1024,
				Temperature:    new(float32(0.3)),
				RequestTimeout: time.Minute,
			},
			Config: &ollama.ChatModelConfig{
				BaseURL: "http://localhost:11434",
				Model:   "llama3.3",
				Timeout: time.Minute,
				Options: &ollama.Options{
					Temperature: 0.3,
					NumPredict:  1024,
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the expected model is built through the same SDK call with
			// the configuration newOllama should assemble, then the two
			// models are compared field by field.
			exp, err := ollama.NewChatModel(context.Background(), c.Config)
			require.NoError(t, err)

			cm, err := newOllama(context.Background(), c.Opts)
			require.NoError(t, err)

			testutil.AssertFilterEqual(t, exp, cm)
		})
	}
}
