package credentials

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Basic_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Basic{CACertificate: "pem", Username: "user", Password: "pass"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"basic","caCertificate":"pem","username":"user","password":"pass"}`, string(data))
}

func Test_Basic_Kind(t *testing.T) {
	t.Parallel()

	assert.Equal(t, KindBasic, Basic{}.Kind())
}

func Test_Basic_Configured(t *testing.T) {
	cc := map[string]struct {
		Creds  Basic
		Result map[string]bool
	}{
		"Nothing set": {
			Result: map[string]bool{},
		},
		"Username and CA certificate set": {
			Creds:  Basic{TLS: TLS{CACertificate: "pem"}, Username: "user"},
			Result: map[string]bool{"username": true, "caCertificate": true},
		},
		"Username and password set": {
			Creds:  Basic{Username: "user", Password: "pass"},
			Result: map[string]bool{"username": true, "password": true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Creds.Configured())
		})
	}
}

func Test_Basic_Authorize(t *testing.T) {
	cc := map[string]struct {
		Creds  Basic
		Header string
	}{
		"Credentials without a username or password": {},
		"Username and password": {
			Creds:  Basic{Username: "user", Password: "pass"},
			Header: "Basic dXNlcjpwYXNz",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Header, authorizationHeader(t, c.Creds))
		})
	}
}
