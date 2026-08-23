package server

import (
	"net/http"

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

	// Assistant reports whether the AI assistant is configured.
	Assistant bool `json:"assistant"`

	// Changedetection reports whether the changedetection.io integration
	// is configured.
	Changedetection bool `json:"changedetection"`

	// Search reports whether document search is configured.
	Search bool `json:"search"`
}

// fetchCapabilities responds with the deployment's capabilities. The
// values are fixed at boot, so they are snapshotted once in NewServer.
func (s *Server) fetchCapabilities(w http.ResponseWriter, _ *http.Request) {
	httpserver.Respond(s.log, w, s.capabilities, http.StatusOK)
}
