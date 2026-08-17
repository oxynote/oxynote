package registry

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _registryErrorBody is the registry-format error payload the transport
// parses to surface UNAUTHORIZED diagnostics.
const _registryErrorBody = `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`

// newFakeRegistry starts an in-process container registry seeded with a
// random image under repo "test/img" tagged "v1" and returns the server
// host and the seeded image digest. A non-empty authorization value makes
// the server challenge with Basic auth and reject any request that does
// not carry exactly that Authorization header.
func newFakeRegistry(t *testing.T, authorization string) (string, string) {
	t.Helper()

	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))

	seedSrv := httptest.NewServer(handler)
	t.Cleanup(seedSrv.Close)

	img, err := random.Image(256, 1)
	require.NoError(t, err)

	digest, err := img.Digest()
	require.NoError(t, err)

	seedHost := strings.TrimPrefix(seedSrv.URL, "http://")

	ref, err := name.ParseReference(seedHost + "/test/img:v1")
	require.NoError(t, err)

	require.NoError(t, remote.Write(ref, img))

	if authorization == "" {
		return seedHost, digest.String()
	}

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != authorization {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)

			_, werr := w.Write([]byte(_registryErrorBody))
			assert.NoError(t, werr)

			return
		}

		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(authSrv.Close)

	return strings.TrimPrefix(authSrv.URL, "http://"), digest.String()
}

func Test_Digest(t *testing.T) {
	t.Parallel()

	// dXNlcjpwYXNz is the base64 form of user:pass.
	const basicAuthorization = "Basic dXNlcjpwYXNz"

	tests := map[string]struct {
		Authorization string
		Reference     string
		Opts          []DigestOption
		ExpectedErr   string
		ErrContains   string
	}{
		"Anonymous fetch returns the seeded digest": {},
		"Invalid image reference": {
			Reference:   "UPPERCASE not allowed",
			ErrContains: "parsing image reference",
		},
		"Basic auth credentials are attached": {
			Authorization: basicAuthorization,
			Opts:          []DigestOption{WithBasicAuth("user", "pass")},
		},
		"Bearer token is attached": {
			Authorization: "Bearer tok-1",
			Opts:          []DigestOption{WithBearerToken("tok-1")},
		},
		"Missing credentials map to ErrUnauthorized": {
			Authorization: basicAuthorization,
			ExpectedErr:   ErrUnauthorized.Error(),
		},
		"Wrong credentials map to ErrUnauthorized": {
			Authorization: basicAuthorization,
			Opts:          []DigestOption{WithBasicAuth("user", "wrong")},
			ExpectedErr:   ErrUnauthorized.Error(),
		},
		"Basic auth without password falls back to anonymous": {
			Authorization: basicAuthorization,
			Opts:          []DigestOption{WithBasicAuth("user", "")},
			ExpectedErr:   ErrUnauthorized.Error(),
		},
	}

	for tname, tc := range tests {
		t.Run(tname, func(t *testing.T) {
			t.Parallel()

			host, seeded := newFakeRegistry(t, tc.Authorization)

			reference := tc.Reference
			if reference == "" {
				reference = host + "/test/img:v1"
			}

			digest, err := Digest(context.Background(), reference, tc.Opts...)

			if tc.ErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.ErrContains)

				return
			}

			if tc.ExpectedErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.ExpectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, seeded, digest)
		})
	}
}
