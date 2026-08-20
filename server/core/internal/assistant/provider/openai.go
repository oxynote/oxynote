package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// newOpenAI builds an OpenAI chat model.
//
// BaseURL is honoured whether or not Azure is enabled, so this provider
// also serves any endpoint speaking the OpenAI chat-completions protocol
// — self-hosted inference servers, gateways, and compatibility layers.
func newOpenAI(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	// MaxCompletionTokens supersedes the SDK's deprecated MaxTokens,
	// which reasoning models reject outright.
	cfg := &openai.ChatModelConfig{
		APIKey:              opts.APIKey,
		Model:               opts.Model,
		BaseURL:             opts.BaseURL,
		MaxCompletionTokens: opts.maxTokens(),
		Temperature:         opts.Temperature,
		Timeout:             opts.timeout(),
		ByAzure:             opts.Azure.Enabled,
		APIVersion:          opts.Azure.APIVersion,
	}

	cm, err := openai.NewChatModel(ctx, cfg)
	if err != nil {
		// NOCOV: the SDK constructor only rejects configuration that validate already refused.
		return nil, fmt.Errorf("creating openai chat model: %w", err)
	}

	return cm, nil
}
