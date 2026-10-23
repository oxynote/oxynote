package registry

import (
	"context"
	"fmt"
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
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _registryErrorBody is the registry-format error payload the transport
// parses to surface UNAUTHORIZED diagnostics.
const _registryErrorBody = `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Digest(t *testing.T) {
	t.Parallel()

	// dXNlcjpwYXNz is the base64 form of user:pass.
	const basicAuthorization = "Basic dXNlcjpwYXNz"

	cc := map[string]struct {
		Authorization string
		Diagnostic    string
		Reference     string
		Path          string
		ErrContains   string
		Err           error
	}{
		"Anonymous fetch returns the seeded digest": {},
		"Invalid image reference": {
			Reference:   "UPPERCASE not allowed",
			ErrContains: "parsing image reference",
			Err:         assert.AnError,
		},
		"Missing tag maps to ErrNotFound": {
			Path: "/test/img:v2",
			Err:  ErrNotFound,
		},
		"Missing repository maps to ErrNotFound": {
			Path: "/test/other:v1",
			Err:  ErrNotFound,
		},
		"Missing credentials map to ErrUnauthorized": {
			Authorization: basicAuthorization,
			Err:           ErrUnauthorized,
		},
		"Denied diagnostic maps to ErrUnauthorized": {
			Diagnostic: "DENIED",
			Err:        ErrUnauthorized,
		},
		"Other registry error is wrapped": {
			Diagnostic:  "UNSUPPORTED",
			ErrContains: "getting remote head",
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))

			seedSrv := httptest.NewServer(handler)
			t.Cleanup(seedSrv.Close)

			img, err := random.Image(256, 1)
			require.NoError(t, err)

			seeded, err := img.Digest()
			require.NoError(t, err)

			host := strings.TrimPrefix(seedSrv.URL, "http://")

			ref, err := name.ParseReference(host + "/test/img:v1")
			require.NoError(t, err)

			require.NoError(t, remote.Write(ref, img))

			// a set authorization puts a proxy in front that challenges with
			// Basic auth and rejects any request without exactly that header.
			if c.Authorization != "" {
				authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != c.Authorization {
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

				host = strings.TrimPrefix(authSrv.URL, "http://")
			}

			// a set diagnostic puts a proxy in front that fails every request
			// with a 400 carrying that registry error code.
			if c.Diagnostic != "" {
				diagSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)

					_, werr := fmt.Fprintf(w, `{"errors":[{"code":%q,"message":"request failed"}]}`, c.Diagnostic)
					assert.NoError(t, werr)
				}))
				t.Cleanup(diagSrv.Close)

				host = strings.TrimPrefix(diagSrv.URL, "http://")
			}

			path := c.Path
			if path == "" {
				path = "/test/img:v1"
			}

			reference := c.Reference
			if reference == "" {
				reference = host + path
			}

			digest, err := Digest(context.Background(), reference)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)

				return
			}

			assert.Equal(t, seeded.String(), digest)
		})
	}
}

func Test_ValidateReference(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Image string
		Err   error
	}{
		"Invalid reference": {
			Image: "UPPERCASE not allowed",
			Err:   assert.AnError,
		},
		"Docker Hub image": {
			Image: "nginx",
		},
		"Image of another registry": {
			Image: "ghcr.io/owner/image:v1",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, ValidateReference(c.Image))
		})
	}
}
