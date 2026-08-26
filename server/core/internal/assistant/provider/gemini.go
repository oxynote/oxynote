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
			// the SDK does not retry at all when this is nil; the empty
			// struct enables its defaults (5 attempts, exponential
			// backoff on 408/429/5xx), matching the retry behavior the
			// other providers' SDKs ship out of the box.
			RetryOptions: &genai.HTTPRetryOptions{},
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
		// eino maps thought parts to ReasoningContent, which the chat
		// protocol renders as the transient thinking stream; gemini-3
		// models additionally require thought parts to be restored on
		// the way back to the API.
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
		},
	}

	cm, err := gemini.NewChatModel(ctx, cfg)
	if err != nil {
		// NOCOV: the SDK constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating gemini chat model: %w", err)
	}

	return cm, nil
}
