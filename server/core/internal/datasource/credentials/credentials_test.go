package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testKeys is the keyring used by credentials crypto tests.
var _testKeys = func() *cryptoutil.Keyring {
	keys, err := cryptoutil.ParseKeyring(base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	if err != nil {
		panic("cannot parse test keyring: " + err.Error())
	}

	return keys
}()

// _testAAD is the associated data credentials crypto tests seal with.
var _testAAD = []byte("data-source-id")

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Credentials_MarshalJSON(t *testing.T) {
	cc := map[string]struct {
		Creds Credentials

		// ByValue marshals the credentials as a field held by value in
		// another struct.
		ByValue bool
		Result  string
	}{
		"Credentials with a payload": {
			Creds:  Credentials{data: []byte(`{"username":"user"}`)},
			Result: `{"username":"user"}`,
		},
		// empty credentials marshal as null rather than as the zero bytes
		// that make every enclosing struct fail with "unexpected end of JSON
		// input".
		"Empty credentials": {
			Result: `null`,
		},
		// the payload is unexported, so a struct that marshals credentials
		// it should not emits nothing of them rather than the secret
		// itself.
		"Credentials held by value": {
			Creds:   Credentials{data: []byte(`{"username":"user"}`)},
			ByValue: true,
			Result:  `{"credentials":{}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				data []byte
				err  error
			)

			if c.ByValue {
				data, err = json.Marshal(struct {
					Credentials Credentials `json:"credentials"`
				}{Credentials: c.Creds})
			} else {
				data, err = c.Creds.MarshalJSON()
			}

			require.NoError(t, err)
			assert.JSONEq(t, c.Result, string(data))
		})
	}
}

func Test_Credentials_IsValid(t *testing.T) {
	t.Parallel()

	// credentials carrying a payload, and empty ones standing for a data
	// source that authenticates through its URL, are both readable.
	c := Credentials{data: []byte(`{"username":"user"}`)}
	assert.True(t, c.IsValid())

	var empty Credentials
	assert.True(t, empty.IsValid())

	// only a decrypt that could not make sense of them says otherwise.
	unreadable := Credentials{data: []byte("not-sealed")}
	require.Error(t, unreadable.Decrypt(_testKeys, _testAAD))
	assert.False(t, unreadable.IsValid())
}

func Test_Credentials_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Src    any
		Result Credentials
		Err    error
	}{
		"Bytes straight off the column": {
			Src:    []byte(`{"username":"user"}`),
			Result: Credentials{data: []byte(`{"username":"user"}`)},
		},
		"A driver handing them over as a string": {
			Src:    `{"username":"user"}`,
			Result: Credentials{data: []byte(`{"username":"user"}`)},
		},
		"Anything else": {
			Src: 42,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var creds Credentials

			err := creds.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			// the driver may reuse its bytes for the next row.
			if src, ok := c.Src.([]byte); ok {
				clear(src)
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}

func Test_Credentials_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var c Credentials

	data := []byte(`{"username":"user"}`)

	require.NoError(t, c.UnmarshalJSON(data))

	// the decoder may reuse its buffer once the call returns.
	clear(data)

	assert.Equal(t, Credentials{data: []byte(`{"username":"user"}`)}, c)
}

func Test_Credentials_Encrypt(t *testing.T) {
	type tcase struct {
		Creds Credentials
		Err   error
	}

	cc := map[string]tcase{
		// credentials a decrypt could not read are refused, so the
		// ciphertext they came from is never overwritten by an encryption
		// of nothing.
		"Unreadable credentials": func() tcase {
			creds := Credentials{data: []byte("not-sealed")}
			require.Error(t, creds.Decrypt(_testKeys, _testAAD))

			return tcase{
				Creds: creds,
				Err:   assert.AnError,
			}
		}(),
		"Successful encryption": {
			Creds: Credentials{data: []byte(`{"username":"user"}`)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, err := c.Creds.Encrypt(_testKeys, _testAAD)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotEmpty(t, data)

			decrypted := Credentials{data: data}
			require.NoError(t, decrypted.Decrypt(_testKeys, _testAAD))
			assert.Equal(t, c.Creds, decrypted)
		})
	}
}

func Test_Credentials_Decrypt(t *testing.T) {
	original := Credentials{data: []byte(`{"username":"user"}`)}

	sealed, err := original.Encrypt(_testKeys, _testAAD)
	require.NoError(t, err)

	cc := map[string]struct {
		Data   []byte
		AAD    []byte
		Result Credentials
		Err    error
	}{
		// what it could not read is dropped rather than left as ciphertext
		// nobody can use, and says so to every later reader.
		"Error returned by Keyring.Decrypt": {
			Data:   []byte("not-sealed"),
			AAD:    _testAAD,
			Result: Credentials{unreadable: true},
			Err:    assert.AnError,
		},
		"Credentials sealed for another row": {
			Data:   sealed,
			AAD:    []byte("other-id"),
			Result: Credentials{unreadable: true},
			Err:    assert.AnError,
		},
		"Successful decryption": {
			Data:   sealed,
			AAD:    _testAAD,
			Result: original,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			creds := Credentials{data: c.Data}

			err := creds.Decrypt(_testKeys, c.AAD)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, err == nil, creds.IsValid())
			assert.Equal(t, c.Result, creds)
		})
	}
}

func Test_Kind_Validate(t *testing.T) {
	cc := map[string]struct {
		Kind Kind
		Err  error
	}{
		"Missing kind": {
			Err: ErrInvalidKind,
		},
		"Unknown kind": {
			Kind: "oauth",
			Err:  ErrInvalidKind,
		},
		"Basic kind": {
			Kind: KindBasic,
		},
		"Token kind": {
			Kind: KindToken,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Kind.Validate())
		})
	}
}

func Test_Kind_MarshalText(t *testing.T) {
	t.Parallel()

	text, err := KindToken.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("token"), text)
}

func Test_Kind_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result Kind
	}{
		"Kind with outer white space and capitals": {
			Text:   " Token ",
			Result: KindToken,
		},
		"Unknown kind is kept": {
			Text:   "OAuth",
			Result: "oauth",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var k Kind

			require.NoError(t, k.UnmarshalText([]byte(c.Text)))
			assert.Equal(t, c.Result, k)
		})
	}
}

func Test_Credentials_Read(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Result Auth
		Err    error
	}{
		"Error returned by Credentials.kind": {
			Creds: Credentials{data: []byte(`{`)},
			Err:   assert.AnError,
		},
		"Error returned by json.Unmarshal of basic credentials": {
			Creds: Credentials{data: []byte(`{"kind":"basic","username":1}`)},
			Err:   assert.AnError,
		},
		"Error returned by json.Unmarshal of token credentials": {
			Creds: Credentials{data: []byte(`{"kind":"token","token":1}`)},
			Err:   assert.AnError,
		},
		"Empty credentials": {},
		"Credentials that could not be read": {
			Creds: Credentials{unreadable: true},
		},
		"Basic credentials": {
			Creds:  Credentials{data: []byte(`{"kind":"basic","username":"user","password":"pass","caCertificate":"pem"}`)},
			Result: Basic{TLS: TLS{CACertificate: "pem"}, Username: "user", Password: "pass"},
		},
		"Token credentials": {
			Creds:  Credentials{data: []byte(`{"kind":"token","token":"secret"}`)},
			Result: Token{Token: "secret"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			auth, err := c.Creds.Read()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, auth)
		})
	}
}

func Test_Credentials_Basic(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Result Basic
		Err    error
	}{
		"Error returned by Credentials.kind": {
			Creds: Credentials{data: []byte(`{`)},
			Err:   assert.AnError,
		},
		"Error returned by json.Unmarshal": {
			Creds: Credentials{data: []byte(`{"kind":"basic","username":1}`)},
			Err:   assert.AnError,
		},
		"Token credentials": {
			Creds: Credentials{data: []byte(`{"kind":"token","token":"secret"}`)},
			Err:   ErrInvalid,
		},
		"Empty credentials": {},
		"Basic credentials": {
			Creds:  Credentials{data: []byte(`{"kind":"basic","username":"user","password":"pass","caCertificate":"pem"}`)},
			Result: Basic{TLS: TLS{CACertificate: "pem"}, Username: "user", Password: "pass"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			b, err := c.Creds.Basic()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, b)
		})
	}
}

func Test_Credentials_Public(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Result Public
		Err    error
	}{
		"Error returned by Credentials.Read": {
			Creds: Credentials{data: []byte(`{"username":"user"}`)},
			Err:   ErrInvalidKind,
		},
		"Empty credentials": {},
		"Token credentials": {
			Creds: Credentials{data: []byte(`{"kind":"token","token":"secret","caCertificate":"pem"}`)},
			Result: Public{
				Kind:       KindToken,
				Configured: map[string]bool{"token": true, "caCertificate": true},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			public, err := c.Creds.Public()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, public)
		})
	}
}

func Test_Credentials_kind(t *testing.T) {
	cc := map[string]struct {
		Creds  Credentials
		Result Kind
		Err    error
	}{
		"Error returned by json.Unmarshal": {
			Creds: Credentials{data: []byte(`[1]`)},
			Err:   assert.AnError,
		},
		"Error returned by Kind.Validate": {
			Creds: Credentials{data: []byte(`{"username":"user"}`)},
			Err:   ErrInvalidKind,
		},
		"Empty credentials": {},
		"Token credentials": {
			Creds:  Credentials{data: []byte(`{"kind":"token","token":"secret"}`)},
			Result: KindToken,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			kind, err := c.Creds.kind()
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, kind)
		})
	}
}

func Test_UpdateInput_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var ui UpdateInput

	data := []byte(`{"username":"user"}`)

	require.NoError(t, ui.UnmarshalJSON(data))

	// the decoder may reuse its buffer once the call returns.
	clear(data)

	assert.Equal(t, UpdateInput(`{"username":"user"}`), ui)
}

func Test_Update(t *testing.T) {
	both := []Kind{KindBasic, KindToken}

	cc := map[string]struct {
		Stored Credentials
		Inp    UpdateInput
		Kinds  []Kind
		Result Credentials
		Err    error
	}{
		"Error returned by json.Unmarshal of the update input": {
			Inp:   UpdateInput(`{`),
			Kinds: both,
			Err:   assert.AnError,
		},
		"Error returned by Kind.Validate": {
			Inp:   UpdateInput(`{"kind":"oauth"}`),
			Kinds: both,
			Err:   ErrInvalidKind,
		},
		"Error returned by json.Unmarshal of stored basic credentials": {
			Stored: Credentials{data: []byte(`{"kind":"basic","username":1}`)},
			Inp:    UpdateInput(`{"password":"pass"}`),
			Kinds:  both,
			Err:    assert.AnError,
		},
		"Error returned by json.Unmarshal of a basic update input": {
			Stored: Credentials{data: []byte(`{"kind":"basic","username":"user"}`)},
			Inp:    UpdateInput(`{"username":1}`),
			Kinds:  both,
			Err:    assert.AnError,
		},
		"Error returned by json.Unmarshal of stored token credentials": {
			Stored: Credentials{data: []byte(`{"kind":"token","token":1}`)},
			Inp:    UpdateInput(`{"caCertificate":"pem"}`),
			Kinds:  both,
			Err:    assert.AnError,
		},
		"Error returned by json.Unmarshal of a token update input": {
			Stored: Credentials{data: []byte(`{"kind":"token","token":"secret"}`)},
			Inp:    UpdateInput(`{"token":1}`),
			Kinds:  both,
			Err:    assert.AnError,
		},
		"No kind and nothing stored": {
			Inp:   UpdateInput(`{"username":"user"}`),
			Kinds: both,
			Err:   ErrInvalidKind,
		},
		"No kind and credentials stored without one": {
			Stored: Credentials{data: []byte(`{"username":"user"}`)},
			Inp:    UpdateInput(`{"password":"pass"}`),
			Kinds:  both,
			Err:    ErrInvalidKind,
		},
		"Kind the data source does not accept": {
			Inp:   UpdateInput(`{"kind":"token","token":"secret"}`),
			Kinds: []Kind{KindBasic},
			Err:   ErrInvalid,
		},
		"Updated username keeps the stored kind and password": {
			Stored: Credentials{data: []byte(`{"kind":"basic","username":"old","password":"secret"}`)},
			Inp:    UpdateInput(`{"username":"new"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"username":"new","password":"secret","kind":"basic"}`)},
		},
		"Updated token naming the stored kind": {
			Stored: Credentials{data: []byte(`{"kind":"token","token":"old"}`)},
			Inp:    UpdateInput(`{"kind":"token","token":"new"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"token":"new","kind":"token"}`)},
		},
		"Created credentials from scratch": {
			Inp:    UpdateInput(`{"kind":"basic","username":"user","password":"pass"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"username":"user","password":"pass","kind":"basic"}`)},
		},
		"Created credentials over unreadable ones": {
			Stored: Credentials{unreadable: true},
			Inp:    UpdateInput(`{"kind":"token","token":"secret"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"token":"secret","kind":"token"}`)},
		},
		"Created credentials over ones stored without a kind": {
			Stored: Credentials{data: []byte(`{"username":"user"}`)},
			Inp:    UpdateInput(`{"kind":"basic","password":"pass"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"username":"","password":"pass","kind":"basic"}`)},
		},
		"Switched to a token keeps only the CA certificate": {
			Stored: Credentials{data: []byte(`{"kind":"basic","username":"user","password":"pass","caCertificate":"pem"}`)},
			Inp:    UpdateInput(`{"kind":"token","token":"secret"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"caCertificate":"pem","token":"secret","kind":"token"}`)},
		},
		"Switched to basic drops the token": {
			Stored: Credentials{data: []byte(`{"kind":"token","token":"secret"}`)},
			Inp:    UpdateInput(`{"kind":"basic","username":"user"}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"username":"user","password":"","kind":"basic"}`)},
		},
		"Cleared credentials": {
			Stored: Credentials{data: []byte(`{"kind":"basic","username":"user","password":"pass"}`)},
			Inp:    UpdateInput(`{"username":"","password":""}`),
			Kinds:  both,
		},
		"Credentials holding only a CA certificate": {
			Stored: Credentials{data: []byte(`{"kind":"token","token":"secret","caCertificate":"pem"}`)},
			Inp:    UpdateInput(`{"token":""}`),
			Kinds:  both,
			Result: Credentials{data: []byte(`{"caCertificate":"pem","token":"","kind":"token"}`)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			creds, err := Update(c.Stored, c.Inp, c.Kinds...)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, creds)
		})
	}
}

// authorizationHeader returns the Authorization header a request carries
// once the credentials authorize it.
func authorizationHeader(t *testing.T, auth Auth) string {
	t.Helper()

	var header string

	rt := auth.Authorize(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		header = r.Header.Get("Authorization")

		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", http.NoBody)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return header
}

// roundTripperFunc is a function that serves as a round tripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls the function.
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
