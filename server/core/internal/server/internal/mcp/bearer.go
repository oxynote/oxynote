package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// _sessionExtraKey is the TokenInfo.Extra key the MCP session is
// stored under.
const _sessionExtraKey = "mcpSession"

// Session is the identity carried by a validated MCP bearer token.
// It mirrors the payload of auth-realtime's internal MCP session
// endpoint.
type Session struct {
	// UserID is the ID of the user the token was issued to.
	UserID string `json:"userId"`

	// OrganizationID is the organization the token was bound to at
	// issuance.
	OrganizationID string `json:"organizationId"`

	// ClientID is the OAuth client the token was issued for.
	ClientID string `json:"clientId"`

	// Scopes lists the OAuth scopes the token carries.
	Scopes []string `json:"scopes"`

	// ExpiresAt is the token's expiry as a Unix timestamp in seconds.
	ExpiresAt int64 `json:"expiresAt"`
}

// newVerifier builds the go-sdk token verifier for the MCP surface.
// Each bearer token is validated per request against auth-realtime's
// internal MCP session endpoint — mirroring how cookie sessions are
// validated — so a revoked client's tokens stop working immediately.
func newVerifier(client *http.Client, url string) sdkauth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		session, err := fetchSession(ctx, client, url, token)
		if err != nil {
			return nil, err
		}

		return &sdkauth.TokenInfo{
			Scopes:     session.Scopes,
			Expiration: time.Unix(session.ExpiresAt, 0),
			UserID:     session.UserID,
			Extra: map[string]any{
				_sessionExtraKey: session,
			},
		}, nil
	}
}

// sessionFromContext extracts the MCP session the bearer middleware
// stored for the request whose context this is.
func sessionFromContext(ctx context.Context) (Session, error) {
	info := sdkauth.TokenInfoFromContext(ctx)
	if info == nil {
		return Session{}, httpserver.ErrNotAuthenticated
	}

	session, ok := info.Extra[_sessionExtraKey].(Session)
	if !ok {
		return Session{}, httpserver.ErrNotAuthenticated
	}

	return session, nil
}

// fetchSession asks auth-realtime's internal MCP session endpoint to
// validate the bearer token. A 401 from the endpoint — bad signature,
// expired token, revoked consent — maps to the go-sdk's ErrInvalidToken
// so the middleware answers with the RFC 9728 challenge; every other
// failure surfaces as an internal error.
func fetchSession(ctx context.Context, client *http.Client, url, token string) (Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return Session{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode == http.StatusUnauthorized {
		return Session{}, fmt.Errorf("%w: rejected by auth service", sdkauth.ErrInvalidToken)
	}

	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("mcp session endpoint returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Session{}, err
	}

	var session Session

	err = json.Unmarshal(data, &session)
	if err != nil {
		return Session{}, err
	}

	if session.UserID == "" || session.OrganizationID == "" {
		return Session{}, fmt.Errorf("%w: incomplete session payload", sdkauth.ErrInvalidToken)
	}

	return session, nil
}
