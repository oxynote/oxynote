// Package provider builds the assistant's chat model from configuration,
// hiding every vendor SDK behind eino's model.ToolCallingChatModel. The
// assistant itself never learns which vendor it is talking to; swapping
// providers is an Options change, not a code change.
package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

const (
	// ProviderAnthropic specifies Anthropic's Claude models, reachable
	// directly or through AWS Bedrock and Google Vertex.
	ProviderAnthropic Provider = "anthropic"

	// ProviderOpenAI specifies OpenAI, Azure OpenAI, and any endpoint
	// speaking the OpenAI chat-completions protocol.
	ProviderOpenAI Provider = "openai"

	// ProviderGoogle specifies Google's Gemini models.
	ProviderGoogle Provider = "google"

	// ProviderOllama specifies a self-hosted Ollama server.
	ProviderOllama Provider = "ollama"

	// ProviderOpenRouter specifies OpenRouter, which brokers many vendors
	// behind one API.
	ProviderOpenRouter Provider = "openrouter"
)

// Provider specifies which vendor backs the assistant's chat model.
type Provider string

// ErrInvalidProvider is returned when the configured provider is not one
// this build supports.
var ErrInvalidProvider = errors.New("invalid assistant provider")

// _providers lists every supported provider in the order they are
// reported to the user, so the error message reads the same every time.
var _providers = []Provider{
	ProviderAnthropic,
	ProviderOpenAI,
	ProviderGoogle,
	ProviderOllama,
	ProviderOpenRouter,
}

// Validate checks whether the provider is one this build supports,
// naming the offending value and the supported set when it is not.
func (p Provider) Validate() error {
	if slices.Contains(_providers, p) {
		return nil
	}

	return fmt.Errorf("%w: %q is not supported, expected one of: %s",
		ErrInvalidProvider, string(p), supported())
}

// ParseProvider normalises a provider name as it arrives from the
// environment, tolerating surrounding whitespace and mixed case. The
// result is not validated; callers get that from Validate.
func ParseProvider(s string) Provider {
	return Provider(strings.ToLower(strings.TrimSpace(s)))
}

// supported returns the comma-separated list of provider names this build
// accepts, for error messages.
func supported() string {
	names := make([]string, 0, len(_providers))
	for _, p := range _providers {
		names = append(names, string(p))
	}

	return strings.Join(names, ", ")
}

// New builds the chat model described by the options. The returned model
// supports tool calling, which the assistant requires: every turn offers
// the model the document read and write tools.
func New(ctx context.Context, opts Options) (model.ToolCallingChatModel, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	switch opts.Provider {
	case ProviderAnthropic:
		return newClaude(ctx, opts)
	case ProviderOpenAI:
		return newOpenAI(ctx, opts)
	case ProviderGoogle:
		return newGemini(ctx, opts)
	case ProviderOllama:
		return newOllama(ctx, opts)
	case ProviderOpenRouter:
		return newOpenRouter(ctx, opts)
	default:
		// NOCOV: validate already rejected every unsupported provider.
		return nil, opts.Provider.Validate()
	}
}
