package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino/components/model"
)

// newOpenRouter builds an OpenRouter chat model. OpenRouter brokers many
// vendors behind one API, so it is the escape hatch for models this build
// has no dedicated provider for.
func newOpenRouter(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	cfg := &openrouter.Config{
		APIKey:      opts.APIKey,
		Model:       opts.Model,
		BaseURL:     opts.BaseURL,
		Temperature: opts.Temperature,
		Timeout:     opts.timeout(),
	}

	if opts.MaxTokens > 0 {
		cfg.MaxTokens = &opts.MaxTokens
	}

	cm, err := openrouter.NewChatModel(ctx, cfg)
	if err != nil {
		// NOCOV: the SDK constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating openrouter chat model: %w", err)
	}

	return cm, nil
}
