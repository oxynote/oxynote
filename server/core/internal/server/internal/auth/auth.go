// Package auth validates sessions and resolves request identity.
package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// _contextKey is a custom type for context keys to avoid collisions.
type _contextKey int

// _contextKeySession is the key used to store the user ID in the context.
const _contextKeySession _contextKey = iota

// _errRateLimited is returned when the Better Auth server rate limits the
// session check, so the caller is told to retry instead of to sign in again.
var _errRateLimited = errutil.New(http.StatusTooManyRequests, "request.rate_limited", "too many requests")

// _headerForwardedFor is the header that carries the client address to the
// Better Auth server.
const _headerForwardedFor = "X-Forwarded-For"

// Options specifies options for auth.
type Options struct {
	// BetterAuthURL is the URL of the Better Auth server.
	BetterAuthURL string
}

// Session represents a Better Auth session stored in Redis.
type Session struct {
	// UserID is the ID of the user associated with the session.
	UserID string `json:"userId"`

	// ActiveOrganizationID is the ID of the active organization for the session.
	ActiveOrganizationID string `json:"activeOrganizationId"`
}

// Middleware is a middleware that extracts and validates Better Auth sessions from Redis.
// It adds the user ID to the request context if the session is valid.
func Middleware(log *slog.Logger, opt Options, client *http.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			session, err := getSession(ctx, client, opt.BetterAuthURL, r.Cookies())
			if err != nil {
				httpserver.RespondError(log, w, err)
				return
			}

			log.Debug("authenticated request", slog.Any("session", session))

			next.ServeHTTP(
				w,
				r.WithContext(AddSessionToContext(ctx, session)),
			)
		})
	}
}

// FilterOrganization returns a filter function that checks if the session's
// active organization ID matches the provided organization ID.
func FilterOrganization(organizationID string) func(context.Context, string) bool {
	return func(ctx context.Context, _ string) bool {
		session, err := ExtractSessionFromContext(ctx)
		if err != nil {
			return false
		}

		return session.ActiveOrganizationID == organizationID
	}
}

// FilterOrganizationDocument returns a filter function that checks the
// session's active organization on top of the document the subscriber's
// topic is bound to, so a publish reaches only those watching that document.
func FilterOrganizationDocument(organizationID string, documentID xid.ID) func(context.Context, string) bool {
	organization := FilterOrganization(organizationID)

	return func(ctx context.Context, rawTopic string) bool {
		if wsserver.TopicParamFromContext(ctx, "documentId") != documentID.String() {
			return false
		}

		return organization(ctx, rawTopic)
	}
}

// FilterUser returns a filter function that checks if the session's
// user ID matches the provided user ID.
func FilterUser(organizationID, userID string) func(context.Context, string) bool {
	return func(ctx context.Context, _ string) bool {
		session, err := ExtractSessionFromContext(ctx)
		if err != nil {
			return false
		}

		return session.ActiveOrganizationID == organizationID && session.UserID == userID
	}
}

// AddSessionToContext adds the session to the context.
func AddSessionToContext(ctx context.Context, session Session) context.Context {
	return context.WithValue(ctx, _contextKeySession, session)
}

// ExtractSessionFromContext extracts the session from the context.
func ExtractSessionFromContext(ctx context.Context) (Session, error) {
	session, ok := ctx.Value(_contextKeySession).(Session)
	if !ok {
		return Session{}, httpserver.ErrNotAuthenticated
	}

	return session, nil
}

// RequireSession extracts the session the middleware stored on the request
// context. When there is none, it responds not-authenticated and reports
// false, so a handler only has to bail out.
func RequireSession(log *slog.Logger, w http.ResponseWriter, r *http.Request) (Session, bool) {
	session, err := ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(log, w, err)
		return Session{}, false
	}

	return session, true
}

// RequireRole is a middleware that lets a request through only when the
// session's user holds at least the given role in the active organization.
// It reads the role on every request, so a role change applies at once.
func RequireRole(log *slog.Logger, db DB, least org.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := RequireSession(log, w, r)
			if !ok {
				return
			}

			role, err := FetchRole(r.Context(), db, session)
			if err != nil {
				httpserver.RespondError(log, w, err)
				return
			}

			if err := role.Allows(least); err != nil {
				httpserver.RespondError(log, w, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// FetchRole returns the role the session's user holds in the active
// organization. A user who is no longer a member is refused with
// org.ErrForbidden, like a role that is too low.
func FetchRole(ctx context.Context, db DB, session Session) (org.Role, error) {
	role, err := db.FetchOrganizationMemberRole(ctx, session.ActiveOrganizationID, session.UserID)
	if err != nil {
		if errutil.IsNotFound(err) {
			return "", org.ErrForbidden
		}

		return "", err
	}

	return role, nil
}

// HasActiveOrganization reports whether a validated identity may reach
// the handlers: a user together with the organization their work is
// scoped to. A session without an active organization (e.g. right after
// signup, before one is selected) must not: every query is scoped by the
// organization id, and a write would create rows under an empty one. The
// cookie session path and the MCP bearer verifier both ask it.
func HasActiveOrganization(userID, organizationID string) bool {
	return userID != "" && organizationID != ""
}

// getSession forwards the request to the Better Auth server to retrieve session information.
// The client address from the context goes along, so Better Auth rate limits
// each client on its own instead of all of them in one shared bucket.
func getSession(ctx context.Context, client *http.Client, url string, cookies []*http.Cookie) (Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return Session{}, err
	}

	for _, reqCookie := range cookies {
		req.AddCookie(reqCookie)
	}

	// Better Auth only trusts a single-value X-Forwarded-For unless it is
	// configured with trustedProxies, so only the one address chi's client IP
	// middleware picked (the last entry, set by Caddy) is sent.
	if ip := middleware.GetClientIP(ctx); ip != "" {
		req.Header.Set(_headerForwardedFor, ip)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode == http.StatusTooManyRequests {
		return Session{}, _errRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		return Session{}, httpserver.ErrNotAuthenticated
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Session{}, err
	}

	var payload struct {
		Session Session `json:"session"`
	}

	err = json.Unmarshal(data, &payload)
	if err != nil {
		return Session{}, err
	}

	if !HasActiveOrganization(payload.Session.UserID, payload.Session.ActiveOrganizationID) {
		return Session{}, httpserver.ErrNotAuthenticated
	}

	return payload.Session, nil
}

// DB is the database the role check reads.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchOrganizationMemberRole should retrieve the role a user holds
	// in an organization.
	FetchOrganizationMemberRole(ctx context.Context, organizationID, userID string) (org.Role, error)
}
