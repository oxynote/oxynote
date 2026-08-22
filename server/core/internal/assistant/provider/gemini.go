package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// newGemini builds a Gemini chat model. Unlike the other providers,
// eino's Gemini component takes a constructed SDK client rather than
// credentials, so the client is built here first.
func newGemini(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:  opts.APIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			Timeout: new(opts.timeout()),
		},
	}

	if opts.BaseURL != "" {
		clientCfg.HTTPOptions.BaseURL = opts.BaseURL
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		// NOCOV: the client constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating gemini client: %w", err)
	}

	cfg := &gemini.Config{
		Client:      client,
		Model:       opts.Model,
		MaxTokens:   opts.maxTokens(),
		Temperature: opts.Temperature,
	}

	cm, err := gemini.NewChatModel(ctx, cfg)
	if err != nil {
		// NOCOV: the SDK constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating gemini chat model: %w", err)
	}

	return cm, nil
}
