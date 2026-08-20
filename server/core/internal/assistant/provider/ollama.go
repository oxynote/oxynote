package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
)

// newOllama builds a chat model backed by a self-hosted Ollama server.
// Ollama is unauthenticated, so the API key is ignored; the base URL
// locating the server is required instead.
func newOllama(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	cfg := &ollama.ChatModelConfig{
		BaseURL: opts.BaseURL,
		Model:   opts.Model,
		Timeout: opts.timeout(),
	}

	if opts.Temperature != nil {
		cfg.Options = &ollama.Options{Temperature: *opts.Temperature}
	}

	cm, err := ollama.NewChatModel(ctx, cfg)
	if err != nil {
		// NOCOV: the SDK constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating ollama chat model: %w", err)
	}

	return cm, nil
}
