package server

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/assistant/provider"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// Capabilities reports which optional services this deployment has
// configured. The frontend reads it to decide what to render at all —
// connect links, the chat entry point, hook types, search — before any
// per-organization state comes into play.
type Capabilities struct {
	// GitHub reports whether the GitHub App integration is configured.
	GitHub bool `json:"github"`

	// Slack reports whether the Slack App integration is configured.
	Slack bool `json:"slack"`

	// AIAssistant reports whether the AI assistant runs and how capable
	// its model is.
	AIAssistant AssistantCapability `json:"aiAssistant"`

	// ChangeDetection reports whether the changedetection.io integration
	// is configured.
	ChangeDetection bool `json:"changeDetection"`

	// Search reports whether document search is configured.
	Search bool `json:"search"`
}

// AssistantCapability describes the AI assistant's availability,
// decided at boot from the configured model's strength.
type AssistantCapability struct {
	// Status specifies whether the assistant runs at full strength,
	// runs with limitations, or is disabled.
	Status provider.Status `json:"status"`

	// Model is the configured model identifier, empty when no provider
	// is configured.
	Model string `json:"model,omitempty"`
}

// fetchCapabilities responds with the deployment's capabilities. The
// values are fixed at boot, so they are snapshotted once in NewServer.
func (s *Server) fetchCapabilities(w http.ResponseWriter, _ *http.Request) {
	httpserver.Respond(s.log, w, s.capabilities, http.StatusOK)
}
