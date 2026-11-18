package credentials

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/common/config"
)

// Basic represents the username and password a data source authenticates
// with.
type Basic struct {
	TLS

	// Username is the username for the data source.
	Username string `json:"username"`

	// Password is the password for the data source.
	Password string `json:"password"`
}

// MarshalJSON returns the JSON representation of the credentials, with
// their kind.
func (b Basic) MarshalJSON() ([]byte, error) {
	type sub Basic

	return json.Marshal(struct {
		sub

		Kind Kind `json:"kind"`
	}{sub: sub(b), Kind: KindBasic})
}

// Kind returns the basic kind.
func (Basic) Kind() Kind {
	return KindBasic
}

// Configured reports which credentials are set, never their values.
func (b Basic) Configured() map[string]bool {
	configured := b.configured()

	if b.Username != "" {
		configured["username"] = true
	}

	if b.Password != "" {
		configured["password"] = true
	}

	return configured
}

// Authorize sends the username and password as basic auth. Credentials
// holding neither authenticate nobody.
func (b Basic) Authorize(rt http.RoundTripper) http.RoundTripper {
	if b.Username == "" && b.Password == "" {
		return rt
	}

	return config.NewBasicAuthRoundTripper(
		config.NewInlineSecret(b.Username),
		config.NewInlineSecret(b.Password),
		rt,
	)
}
