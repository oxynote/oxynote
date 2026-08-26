package provider

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func Test_newGemini(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts Options

		// Client and Config describe the model newGemini should have
		// built; Config's Client field is filled in by the test body.
		Client *genai.ClientConfig
		Config *gemini.Config
	}{
		"Defaults": {
			Opts: Options{
				Provider: ProviderGoogle,
				Model:    "gemini-2.5-pro",
				APIKey:   "k",
			},
			Client: &genai.ClientConfig{
				APIKey:  "k",
				Backend: genai.BackendGeminiAPI,
				HTTPOptions: genai.HTTPOptions{
					Timeout:      new(_defaultRequestTimeout),
					RetryOptions: &genai.HTTPRetryOptions{},
				},
			},
			Config: &gemini.Config{
				Model:          "gemini-2.5-pro",
				ThinkingConfig: &genai.ThinkingConfig{IncludeThoughts: true},
			},
		},
		"Optional tuning": {
			Opts: tunedOptions(ProviderGoogle, "gemini-2.5-pro"),
			Client: &genai.ClientConfig{
				APIKey:  "k",
				Backend: genai.BackendGeminiAPI,
				HTTPOptions: genai.HTTPOptions{
					BaseURL:      "https://example.invalid/v1",
					Timeout:      new(time.Minute),
					RetryOptions: &genai.HTTPRetryOptions{},
				},
			},
			Config: &gemini.Config{
				Model:          "gemini-2.5-pro",
				MaxTokens:      new(1024),
				Temperature:    new(float32(0.3)),
				ThinkingConfig: &genai.ThinkingConfig{IncludeThoughts: true},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the expected model is built through the same SDK calls
			// with the configuration newGemini should assemble, then the
			// two models are compared field by field. The client's
			// envVarProvider func is ignored, since cmp never treats two
			// non-nil funcs as equal.
			client, err := genai.NewClient(context.Background(), c.Client)
			require.NoError(t, err)

			c.Config.Client = client

			exp, err := gemini.NewChatModel(context.Background(), c.Config)
			require.NoError(t, err)

			cm, err := newGemini(context.Background(), c.Opts)
			require.NoError(t, err)

			testutil.AssertFilterEqual(t, exp, cm, (func() map[string]string)(nil))
		})
	}
}
