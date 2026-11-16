package credentials

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/common/config"
)

// Token represents the token a data source authenticates with.
type Token struct {
	TLS

	// Token is the token for the data source.
	Token string `json:"token"`
}

// MarshalJSON returns the JSON representation of the credentials, with
// their kind.
func (t Token) MarshalJSON() ([]byte, error) {
	type sub Token

	return json.Marshal(struct {
		sub

		Kind Kind `json:"kind"`
	}{sub: sub(t), Kind: KindToken})
}

// Kind returns the token kind.
func (Token) Kind() Kind {
	return KindToken
}

// Configured reports which credentials are set, never their values.
func (t Token) Configured() map[string]bool {
	configured := t.configured()

	if t.Token != "" {
		configured["token"] = true
	}

	return configured
}

// Authorize sends the token as a bearer token. Credentials without a token
// authenticate nobody.
func (t Token) Authorize(rt http.RoundTripper) http.RoundTripper {
	if t.Token == "" {
		return rt
	}

	return config.NewAuthorizationCredentialsRoundTripper("Bearer", config.NewInlineSecret(t.Token), rt)
}
