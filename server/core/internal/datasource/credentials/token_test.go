package credentials

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Token_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Token{CACertificate: "pem", Token: "secret"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"token","caCertificate":"pem","token":"secret"}`, string(data))
}

func Test_Token_Kind(t *testing.T) {
	t.Parallel()

	assert.Equal(t, KindToken, Token{}.Kind())
}

func Test_Token_Configured(t *testing.T) {
	cc := map[string]struct {
		Creds  Token
		Result map[string]bool
	}{
		"Nothing set": {
			Result: map[string]bool{},
		},
		"Only a CA certificate set": {
			Creds:  Token{TLS: TLS{CACertificate: "pem"}},
			Result: map[string]bool{"caCertificate": true},
		},
		"Token set": {
			Creds:  Token{Token: "secret"},
			Result: map[string]bool{"token": true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Creds.Configured())
		})
	}
}

func Test_Token_Authorize(t *testing.T) {
	cc := map[string]struct {
		Creds  Token
		Header string
	}{
		"Credentials without a token": {},
		"Token": {
			Creds:  Token{Token: "secret"},
			Header: "Bearer secret",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Header, authorizationHeader(t, c.Creds))
		})
	}
}
