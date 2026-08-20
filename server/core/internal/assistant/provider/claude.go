package provider

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
)

// _claudeDefaultMaxTokens caps a Claude response when the operator did
// not choose a cap. The Anthropic API requires the field, so unlike the
// other providers there is no "let the vendor decide" option.
const _claudeDefaultMaxTokens = 64000

// newClaude builds a Claude chat model, reaching Anthropic directly or
// through Bedrock or Vertex depending on the options.
//
// AutoCacheControl is always enabled: it places cache breakpoints on the
// system prompt and tool definitions, which is what keeps the assistant's
// large canonical-schema prompt from being re-billed on every turn.
func newClaude(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	cfg := &claude.Config{
		APIKey:                   opts.APIKey,
		Model:                    opts.Model,
		MaxTokens:                opts.MaxTokens,
		Temperature:              opts.Temperature,
		RequestTimeout:           opts.timeout(),
		AutoCacheControl:         &claude.CacheControl{TTL: claude.CacheTTL1h},
		ByBedrock:                opts.Bedrock.Enabled,
		AccessKey:                opts.Bedrock.AccessKey,
		SecretAccessKey:          opts.Bedrock.SecretAccessKey,
		SessionToken:             opts.Bedrock.SessionToken,
		Region:                   opts.Bedrock.Region,
		ByVertex:                 opts.Vertex.Enabled,
		VertexProjectID:          opts.Vertex.ProjectID,
		VertexRegion:             opts.Vertex.Region,
		VertexServiceAccountJSON: opts.Vertex.ServiceAccountJSON,
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = _claudeDefaultMaxTokens
	}

	if opts.BaseURL != "" {
		cfg.BaseURL = &opts.BaseURL
	}

	cm, err := claude.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating claude chat model: %w", err)
	}

	return cm, nil
}
