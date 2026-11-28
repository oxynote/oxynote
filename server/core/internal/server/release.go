package server

import (
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/release"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// fetchLatestRelease responds with the latest Oxynote release, or null
// while none is known.
func (s *Server) fetchLatestRelease(w http.ResponseWriter, _ *http.Request) {
	httpserver.Respond(s.log, w, s.releases.Latest(), http.StatusOK)
}

// ReleaseChecker reports the latest Oxynote release.
//
//go:generate ../../scripts/codegen/mock -t internal ReleaseChecker release_checker
type ReleaseChecker interface {
	// Latest should return the latest release, null while none is known.
	Latest() null.Value[release.Release]
}
