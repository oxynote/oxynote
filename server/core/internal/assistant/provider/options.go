package provider

import (
	"errors"
	"time"
)

// _defaultRequestTimeout bounds a single model HTTP request when the
// operator does not set one. Turns stream for a while, so this is
// generous compared to ordinary API calls.
var _defaultRequestTimeout = 10 * time.Minute

var (
	// ErrMissingModel is returned when no model identifier is
	// configured. Wiring is expected to fall back to the provider's
	// DefaultModel before validating; an empty model here means even
	// that fallback had nothing to offer.
	ErrMissingModel = errors.New("assistant model is required")

	// ErrMissingAPIKey is returned when a provider that authenticates with
	// an API key was configured without one.
	ErrMissingAPIKey = errors.New("assistant api key is required")

	// ErrMissingBaseURL is returned when a self-hosted provider was
	// configured without an endpoint to reach.
	ErrMissingBaseURL = errors.New("assistant base url is required")
)

// Options configures the assistant's chat model. Values come from the
// environment and are assembled in main; nothing in this package reads
// the environment itself.
type Options struct {
	// Provider selects the vendor implementation.
	Provider Provider

	// Model is the vendor's model identifier, passed through verbatim.
	Model string

	// APIKey authenticates against the provider. Ollama ignores it;
	// Claude accepts it as an alternative to Bedrock or Vertex
	// credentials.
	APIKey string

	// BaseURL overrides the provider's default endpoint. It is required
	// for Ollama and optional elsewhere, where it addresses proxies,
	// gateways, and OpenAI-compatible servers.
	BaseURL string

	// MaxTokens caps the model's response length for a single turn.
	// Zero lets the provider apply its own default, except on Claude
	// where the API requires an explicit cap.
	MaxTokens int

	// Temperature controls sampling randomness. Zero value means the
	// provider default is used rather than a temperature of zero.
	Temperature *float32

	// RequestTimeout bounds one HTTP request to the provider. Zero
	// applies a default of ten minutes.
	RequestTimeout time.Duration

	// Azure holds Azure OpenAI settings, read only when Provider is
	// ProviderOpenAI.
	Azure AzureOptions

	// Bedrock holds AWS Bedrock settings, read only when Provider is
	// ProviderClaude.
	Bedrock BedrockOptions

	// Vertex holds Google Vertex settings, read only when Provider is
	// ProviderClaude.
	Vertex VertexOptions
}

// AzureOptions configures Azure-hosted OpenAI deployments.
type AzureOptions struct {
	// Enabled indicates whether the OpenAI provider should target Azure
	// rather than api.openai.com.
	Enabled bool

	// APIVersion is the Azure OpenAI API version, required by Azure.
	APIVersion string
}

// BedrockOptions configures Claude access through AWS Bedrock.
type BedrockOptions struct {
	// Enabled indicates whether Claude should be reached via Bedrock.
	Enabled bool

	// AccessKey is the AWS access key id. Empty falls back to the
	// ambient AWS credential chain.
	AccessKey string

	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string

	// SessionToken is the AWS session token for temporary credentials.
	SessionToken string

	// Region is the AWS region hosting the model.
	Region string
}

// VertexOptions configures Claude access through Google Vertex AI.
type VertexOptions struct {
	// Enabled indicates whether Claude should be reached via Vertex.
	Enabled bool

	// ProjectID is the Google Cloud project hosting the model.
	ProjectID string

	// Region is the Vertex region, for example us-east5.
	Region string

	// ServiceAccountJSON is a raw service-account key. Empty falls back
	// to application default credentials.
	ServiceAccountJSON []byte
}

// validate reports whether the options describe a usable model.
func (o Options) validate() error {
	if err := o.Provider.Validate(); err != nil {
		return err
	}

	if o.Model == "" {
		return ErrMissingModel
	}

	if o.Provider == ProviderOllama && o.BaseURL == "" {
		return ErrMissingBaseURL
	}

	if o.requiresAPIKey() && o.APIKey == "" {
		return ErrMissingAPIKey
	}

	return nil
}

// requiresAPIKey reports whether the configured provider authenticates
// with an API key. Ollama is unauthenticated, and Claude authenticates
// through Bedrock or Vertex when either is enabled.
func (o Options) requiresAPIKey() bool {
	if o.Provider == ProviderOllama {
		return false
	}

	if o.Provider == ProviderClaude {
		return !o.Bedrock.Enabled && !o.Vertex.Enabled
	}

	return true
}

// timeout returns the configured request timeout, or the default when
// the operator did not set one.
func (o Options) timeout() time.Duration {
	if o.RequestTimeout > 0 {
		return o.RequestTimeout
	}

	return _defaultRequestTimeout
}

// maxTokens returns the configured response cap as the pointer the
// vendor SDKs take, or nil when no cap is set and the provider default
// should apply.
func (o Options) maxTokens() *int {
	if o.MaxTokens > 0 {
		return &o.MaxTokens
	}

	return nil
}
