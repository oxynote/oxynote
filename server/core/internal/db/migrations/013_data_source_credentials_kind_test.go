package migrations

import (
	"encoding/base64"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_editDataSourceCredentials(t *testing.T) {
	keys, err := cryptoutil.ParseKeyring(base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	require.NoError(t, err)

	id := xid.New()

	// seal encrypts the plaintext the way a stored row holds it.
	seal := func(plaintext string) []byte {
		sealed, err := keys.Encrypt([]byte(plaintext), id.Bytes())
		require.NoError(t, err)

		return sealed
	}

	// addKind is the edit the up step makes.
	addKind := func(creds map[string]any) bool {
		if _, ok := creds["kind"]; ok {
			return false
		}

		creds["kind"] = "basic"

		return true
	}

	cc := map[string]struct {
		Sealed  []byte
		Result  string
		Changed bool
	}{
		"Credentials no key reads are reset": {
			Sealed:  []byte("garbage"),
			Changed: true,
		},
		"Empty credentials stay": {
			Sealed: seal(""),
		},
		"Credentials that are an array are reset": {
			Sealed:  seal(`[1]`),
			Changed: true,
		},
		"Credentials that are null are reset": {
			Sealed:  seal(`null`),
			Changed: true,
		},
		"Credentials the edit leaves alone stay": {
			Sealed: seal(`{"kind":"token","token":"secret"}`),
		},
		"Credentials the edit changes": {
			Sealed:  seal(`{"username":"user"}`),
			Result:  `{"kind":"basic","username":"user"}`,
			Changed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			edited, changed, err := editDataSourceCredentials(keys, id, c.Sealed, addKind)
			require.NoError(t, err)
			assert.Equal(t, c.Changed, changed)

			if c.Result == "" {
				assert.Empty(t, edited)
				return
			}

			assert.JSONEq(t, c.Result, string(edited))
		})
	}
}
