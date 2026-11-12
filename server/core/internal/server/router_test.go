package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jarcoal/httpmock"
	githubCore "github.com/oxynote/oxynote/server/core/internal/apps/github"
	slackCore "github.com/oxynote/oxynote/server/core/internal/apps/slack"
	orgCore "github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/github"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/slack"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _routeParam matches a chi route parameter.
var _routeParam = regexp.MustCompile(`\{[^}]+\}`)

// The mount a third-party callback lives on is the one thing a handler test
// cannot observe, and getting it wrong is invisible until a delivery is
// dropped in production: /api/x is blocked at the front door, so a webhook
// route mounted there is unreachable by the service that has to call it.
func Test_Server_httpRouter(t *testing.T) {
	t.Parallel()

	s := &Server{
		log: slog.New(slog.DiscardHandler),
		fc:  metricutil.NewFactory("test", prometheus.NewRegistry()),
	}

	// the app handlers read their manager while the routes are being
	// mounted, so they have to exist; unconfigured managers are enough,
	// since only the mount points are under test.
	githubMan, err := githubCore.NewManager(nil, githubCore.Options{})
	require.NoError(t, err)

	slackMan, err := slackCore.NewManager(s.log, nil, nil, nil, nil, slackCore.Options{})
	require.NoError(t, err)

	s.handlers.github = github.NewHandler(s.log, nil, githubMan)
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

	// MCP is off while its handler is nil, so nothing is mounted for it.
	for route := range routes {
		assert.NotContains(t, route, "/api/mcp")
	}

	// the service-to-service surface stays where it is.
	assert.True(t, routes["POST /api/x/organizations/{organizationId}/teardown"])
	assert.True(t, routes["PUT /api/x/documents/{documentId}/branch/{branchId}/"])
	assert.True(t, routes["POST /api/x/email"])

	// the branch operations are reachable only through auth-realtime,
	// which stores the editors' pending changes before core reads the
	// branch and drops the connections a delete leaves behind: a public
	// mount would let a client skip that.
	internalOnly := map[string]string{
		"PUT /api/x/documents/{documentId}/merge":                  "PUT /api/*/documents/{documentId}/merge",
		"POST /api/x/documents/{documentId}/branches":              "POST /api/*/documents/{documentId}/branches/",
		"PUT /api/x/documents/{documentId}/branches/{branchId}":    "PUT /api/*/documents/{documentId}/branches/{branchId}/",
		"DELETE /api/x/documents/{documentId}/branches/{branchId}": "DELETE /api/*/documents/{documentId}/branches/{branchId}/",
	}

	for internal, public := range internalOnly {
		assert.True(t, routes[internal], "%s must be mounted internally", internal)
		assert.False(t, routes[public], "%s must not be publicly routable", public)
	}

	// the branch reads stay public alongside them.
	assert.True(t, routes["GET /api/*/documents/{documentId}/branches/"])
}

// Every route under the session surface names the least role it needs.
// A route missing from the table fails the test, so a new route cannot
// ship without a decision.
func Test_Server_router(t *testing.T) {
	t.Parallel()

	least := map[string]orgCore.Role{
		"DELETE /data-sources/{dataSourceId}/":                                   orgCore.RoleAdmin,
		"DELETE /documents/{documentId}/":                                        orgCore.RoleEditor,
		"DELETE /documents/{documentId}/branches/{branchId}/reviewers/":          orgCore.RoleEditor,
		"DELETE /documents/{documentId}/branches/{branchId}/tags/{tagId}":        orgCore.RoleEditor,
		"DELETE /documents/{documentId}/comments/{commentId}/":                   orgCore.RoleEditor,
		"DELETE /documents/{documentId}/comments/{commentId}/replies/{replyId}/": orgCore.RoleEditor,
		"DELETE /documents/{documentId}/hooks/{hookId}/":                         orgCore.RoleEditor,
		"DELETE /github/":                                                   orgCore.RoleAdmin,
		"DELETE /slack/":                                                    orgCore.RoleAdmin,
		"DELETE /slack/users/":                                              orgCore.RoleViewer,
		"DELETE /tags/{tagId}":                                              orgCore.RoleEditor,
		"DELETE /variables/groups/{groupId}":                                orgCore.RoleEditor,
		"DELETE /variables/{variableId}/":                                   orgCore.RoleEditor,
		"DELETE /variables/{variableId}/value":                              orgCore.RoleViewer,
		"GET /ai/chat":                                                      orgCore.RoleViewer,
		"GET /capabilities":                                                 orgCore.RoleViewer,
		"GET /data-sources/":                                                orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/":                                 orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/connection":                       orgCore.RoleAdmin,
		"GET /data-sources/{dataSourceId}/logs/":                            orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/logs/fields/":                     orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/logs/fields/{field}/values":       orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/logs/volume":                      orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/prometheus/labels/":               orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/prometheus/labels/{label}/values": orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/prometheus/metadata":              orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/prometheus/series":                orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/query":                            orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/sql/metadata":                     orgCore.RoleViewer,
		"GET /data-sources/{dataSourceId}/sql/query-labels":                 orgCore.RoleViewer,
		"GET /documents/recent":                                             orgCore.RoleViewer,
		"GET /documents/search":                                             orgCore.RoleViewer,
		"GET /documents/search/legacy":                                      orgCore.RoleViewer,
		"GET /documents/tree/":                                              orgCore.RoleViewer,
		"GET /documents/{documentId}/access":                                orgCore.RoleViewer,
		"GET /documents/{documentId}/branches/":                             orgCore.RoleViewer,
		"GET /documents/{documentId}/branches/{branchId}/reviewers/":        orgCore.RoleViewer,
		"GET /documents/{documentId}/branches/{branchId}/search":            orgCore.RoleViewer,
		"GET /documents/{documentId}/branches/{branchId}/tags/":             orgCore.RoleViewer,
		"GET /documents/{documentId}/branches/{branchId}/views":             orgCore.RoleViewer,
		"GET /documents/{documentId}/comments/":                             orgCore.RoleViewer,
		"GET /documents/{documentId}/comments/{commentId}/":                 orgCore.RoleViewer,
		"GET /documents/{documentId}/files/{ref}":                           orgCore.RoleViewer,
		"GET /documents/{documentId}/hooks/":                                orgCore.RoleViewer,
		"GET /documents/{documentId}/maintainers":                           orgCore.RoleViewer,
		"GET /github/":                                   orgCore.RoleViewer,
		"GET /github/connect":                            orgCore.RoleAdmin,
		"GET /github/install":                            orgCore.RoleAdmin,
		"GET /github/repositories/":                      orgCore.RoleViewer,
		"GET /github/repositories/{repository}/branches": orgCore.RoleViewer,
		"GET /github/repositories/{repository}/tree":     orgCore.RoleViewer,
		"GET /notifications/":                            orgCore.RoleViewer,
		"GET /notifications/count":                       orgCore.RoleViewer,
		"GET /organizations/logo/":                       orgCore.RoleViewer,
		"GET /slack/":                                    orgCore.RoleViewer,
		"GET /slack/connect":                             orgCore.RoleAdmin,
		"GET /slack/install":                             orgCore.RoleAdmin,
		"GET /slack/messages":                            orgCore.RoleViewer,
		"GET /slack/users/":                              orgCore.RoleViewer,
		"GET /slack/users/link":                          orgCore.RoleViewer,
		"GET /tags/tree/":                                orgCore.RoleViewer,
		"GET /users/{userId}/image":                      orgCore.RoleViewer,
		"GET /variables/tree/":                           orgCore.RoleViewer,
		"POST /data-sources/":                            orgCore.RoleAdmin,
		"POST /documents/":                               orgCore.RoleEditor,
		"POST /documents/{documentId}/branches/{branchId}/blocks/{blockUid}/run": orgCore.RoleViewer,
		"POST /documents/{documentId}/branches/{branchId}/reviewers/":            orgCore.RoleEditor,
		"POST /documents/{documentId}/branches/{branchId}/tags/":                 orgCore.RoleEditor,
		"POST /documents/{documentId}/branches/{branchId}/views":                 orgCore.RoleViewer,
		"POST /documents/{documentId}/comments/":                                 orgCore.RoleEditor,
		"POST /documents/{documentId}/comments/{commentId}/replies/":             orgCore.RoleEditor,
		"POST /documents/{documentId}/duplicate":                                 orgCore.RoleEditor,
		"POST /documents/{documentId}/files/":                                    orgCore.RoleEditor,
		"POST /documents/{documentId}/hooks/":                                    orgCore.RoleEditor,
		"POST /tags/":                                                            orgCore.RoleEditor,
		"POST /variables/":                                                       orgCore.RoleEditor,
		"POST /variables/groups/":                                                orgCore.RoleEditor,
		"PUT /data-sources/{dataSourceId}/":                                      orgCore.RoleAdmin,
		"PUT /documents/tree/":                                                   orgCore.RoleEditor,
		"PUT /documents/{documentId}/branches/{branchId}/review-approve":         orgCore.RoleViewer,
		"PUT /documents/{documentId}/comments/{commentId}/":                      orgCore.RoleEditor,
		"PUT /documents/{documentId}/comments/{commentId}/replies/{replyId}/":    orgCore.RoleEditor,
		"PUT /documents/{documentId}/comments/{commentId}/status":                orgCore.RoleEditor,
		"PUT /documents/{documentId}/hooks/{hookId}/":                            orgCore.RoleEditor,
		"PUT /documents/{documentId}/hooks/{hookId}/reset":                       orgCore.RoleEditor,
		"PUT /notifications/read-status":                                         orgCore.RoleViewer,
		"PUT /organizations/logo/":                                               orgCore.RoleAdmin,
		"PUT /slack/users/settings":                                              orgCore.RoleViewer,
		"PUT /tags/tree/":                                                        orgCore.RoleEditor,
		"PUT /tags/{tagId}":                                                      orgCore.RoleEditor,
		"PUT /tags/{tagId}/visibility":                                           orgCore.RoleViewer,
		"PUT /users/image":                                                       orgCore.RoleViewer,
		"PUT /variables/groups/tree":                                             orgCore.RoleEditor,
		"PUT /variables/groups/{groupId}":                                        orgCore.RoleEditor,
		"PUT /variables/tree/":                                                   orgCore.RoleEditor,
		"PUT /variables/{variableId}/":                                           orgCore.RoleEditor,
		"PUT /variables/{variableId}/default":                                    orgCore.RoleEditor,
		"PUT /variables/{variableId}/value":                                      orgCore.RoleViewer,
	}

	routes := make(map[string]bool)

	require.NoError(t, chi.Walk(
		func() chi.Router {
			return stubRoleServer(t, stubRoleDB(orgCore.RoleViewer)).router()
		}(),
		func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			// the websocket upgrade takes every method and is open to
			// every member. Its topics only read.
			if route != "/ws" {
				routes[method+" "+route] = true
			}

			return nil
		},
	))

	for route := range routes {
		assert.Contains(t, least, route, "%s names no least role", route)
	}

	for route := range least {
		assert.True(t, routes[route], "%s is not routed", route)
	}

	for route, role := range least {
		t.Run(route, func(t *testing.T) {
			t.Parallel()

			assertRoleCheck(t, "/api", route, role)
		})
	}
}

// The branch operations auth-realtime forwards need an editor, the same
// as every other write.
func Test_Server_internalRouter(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Least orgCore.Role
	}{
		"PUT /documents/{documentId}/merge":                  {Least: orgCore.RoleEditor},
		"POST /documents/{documentId}/branches":              {Least: orgCore.RoleEditor},
		"PUT /documents/{documentId}/branches/{branchId}":    {Least: orgCore.RoleEditor},
		"DELETE /documents/{documentId}/branches/{branchId}": {Least: orgCore.RoleEditor},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assertRoleCheck(t, "/api/x", cn, c.Least)
		})
	}
}

// assertRoleCheck sends the route a request from the role just below its
// least role, which must be refused, and one from the least role, which
// must pass the check. A route open to viewers must not look the role up
// at all.
func assertRoleCheck(t *testing.T, prefix, route string, least orgCore.Role) {
	t.Helper()

	method, pattern, found := strings.Cut(route, " ")
	require.True(t, found, "%s names no method", route)

	path := prefix + _routeParam.ReplaceAllStringFunc(pattern, func(string) string {
		return xid.New().String()
	})

	below := map[orgCore.Role]orgCore.Role{
		orgCore.RoleEditor: orgCore.RoleViewer,
		orgCore.RoleAdmin:  orgCore.RoleEditor,
	}

	if lower, ok := below[least]; ok {
		db := stubRoleDB(lower)
		s := stubRoleServer(t, db)
		rec := httptest.NewRecorder()

		s.httpRouter().ServeHTTP(rec, httptest.NewRequest(method, "http://test.com"+path, http.NoBody))

		assert.Equal(t, http.StatusForbidden, rec.Code, "%s as %s", route, lower)
		assert.JSONEq(t, `{"code":"member.forbidden","message":"forbidden"}`, rec.Body.String())
		assert.Len(t, db.FetchOrganizationMemberRoleCalls(), 1)
	}

	db := stubRoleDB(least)
	s := stubRoleServer(t, db)
	rec := httptest.NewRecorder()

	s.httpRouter().ServeHTTP(rec, httptest.NewRequest(method, "http://test.com"+path, http.NoBody))

	assert.NotEqual(t, http.StatusForbidden, rec.Code, "%s as %s", route, least)

	calls := 1
	if least == orgCore.RoleViewer {
		calls = 0
	}

	assert.Len(t, db.FetchOrganizationMemberRoleCalls(), calls)
}

// stubRoleDB builds a database mock that answers every role lookup with
// the given role.
func stubRoleDB(role orgCore.Role) *DBMock {
	return &DBMock{
		FetchOrganizationMemberRoleFunc: func(_ context.Context, _, _ string) (orgCore.Role, error) {
			return role, nil
		},
	}
}

// stubRoleServer builds a server whose every session reads its member
// role from the given database mock. Handlers that sit before the role
// check exist, so a request reaches it. The rest stay nil, and a request
// past the check is recovered as a 500.
func stubRoleServer(t *testing.T, db *DBMock) *Server {
	t.Helper()

	client, mt := testutil.MockHTTP()
	mt.RegisterResponder(
		http.MethodGet,
		"http://auth.test/session",
		httpmock.NewStringResponder(http.StatusOK, `{"session":{"userId":"u1","activeOrganizationId":"org1"}}`),
	)

	s := &Server{
		log:    slog.New(slog.DiscardHandler),
		fc:     metricutil.NewFactory("test", prometheus.NewRegistry()),
		client: client,
		db:     db,
		opts: Options{
			Auth: auth.Options{BetterAuthURL: "http://auth.test/session"},
		},
	}

	githubMan, err := githubCore.NewManager(nil, githubCore.Options{})
	require.NoError(t, err)

	slackMan, err := slackCore.NewManager(s.log, nil, nil, nil, nil, slackCore.Options{})
	require.NoError(t, err)

	s.handlers.github = github.NewHandler(s.log, nil, githubMan)
	s.handlers.slack = slack.NewHandler(s.log, nil, nil, slackMan)
	s.handlers.document = document.NewHandler(s.log, db, nil, nil, nil, nil, nil)

	return s
}
