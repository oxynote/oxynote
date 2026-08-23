package server

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	githubCore "github.com/oxynote/oxynote/server/core/internal/apps/github"
	slackCore "github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/github"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/slack"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mount a third-party callback lives on is the one thing a handler test
// cannot observe, and getting it wrong is invisible until a delivery is
// dropped in production: /api/x is blocked at the front door, so a webhook
// route mounted there is unreachable by the service that has to call it.
func Test_Server_httpRouter(t *testing.T) {
	t.Parallel()

	s := &Server{
		log: discardLog(),
		fc:  metricutil.NewFactory("test", prometheus.NewRegistry()),
	}

	// the app handlers read their manager while the routes are being
	// mounted, so they have to exist; unconfigured managers are enough,
	// since only the mount points are under test.
	githubMan, err := githubCore.NewManager(nil, githubCore.Options{})
	require.NoError(t, err)

	slackMan, err := slackCore.NewManager(s.log, nil, nil, nil, slackCore.Options{})
	require.NoError(t, err)

	s.handlers.github = github.NewHandler(s.log, nil, nil, githubMan)
	s.handlers.slack = slack.NewHandler(s.log, nil, nil, slackMan)

	routes := make(map[string]bool)

	require.NoError(t, chi.Walk(
		s.httpRouter(),
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			routes[method+" "+route] = true

			return nil
		},
	))

	reachable := []string{
		"POST /api/apps/github/events",
		"GET /api/apps/slack/install",
		"POST /api/apps/slack/events",
		"POST /api/apps/slack/commands",
		"POST /api/apps/slack/slash",
	}

	for _, route := range reachable {
		assert.True(t, routes[route], "%s must be publicly routable", route)
	}

	blocked := []string{
		"POST /api/x/github/events",
		"GET /api/x/slack/install",
		"POST /api/x/slack/events",
		"POST /api/x/slack/commands",
		"POST /api/x/slack/slash",
	}

	for _, route := range blocked {
		assert.False(t, routes[route], "%s sits behind the front door's 403", route)
	}

	// the capability signal lives on the session-authed surface (the "/"
	// mount walks as "*").
	assert.True(t, routes["GET /api/*/capabilities"])

	// the service-to-service surface stays where it is.
	assert.True(t, routes["POST /api/x/organizations/{organizationId}/teardown"])
	assert.True(t, routes["PUT /api/x/documents/{documentId}/branch/{branchId}/"])
	assert.True(t, routes["POST /api/x/email"])
}
