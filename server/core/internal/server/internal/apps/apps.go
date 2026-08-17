// Package apps provides the handler scaffolding shared by the third-party
// app integrations.
package apps

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// RequireConfigured returns a middleware that rejects requests with the given
// error when the integration is not configured on this deployment. The error
// is the caller's, since a user-facing endpoint says the integration is
// unavailable while a provider callback pretends the route does not exist.
func RequireConfigured(
	log *slog.Logger,
	configured func() bool,
	err error,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !configured() {
				httpserver.RespondError(log, w, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Connected reports whether the organization is connected to the
// integration, treating a missing row as not connected. The lookup is
// skipped entirely when the integration is not configured, since nothing
// can be connected to an integration this deployment does not have.
func Connected(ctx context.Context, configured bool, fetch func(ctx context.Context) error) (bool, error) {
	if !configured {
		return false, nil
	}

	err := fetch(ctx)

	switch {
	case err == nil:
		return true, nil
	case errutil.IsNotFound(err):
		return false, nil
	default:
		return false, err
	}
}

// RespondCapability responds with the integration's availability envelope:
// whether this deployment has it configured at all, and whether the caller's
// organization is connected to it. It always responds 200 so the frontend can
// read the flags rather than an error.
func RespondCapability(log *slog.Logger, w http.ResponseWriter, configured, connected bool) {
	httpserver.Respond(log, w, map[string]bool{
		"connected":  connected,
		"configured": configured,
	}, http.StatusOK)
}
