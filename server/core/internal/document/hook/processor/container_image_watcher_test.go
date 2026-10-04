package processor

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeRegistryImage starts an in-process container registry seeded with a
// random image and returns the full image reference and its digest. When
// unauthorized is set, every request is rejected with a registry-format 401.
func newFakeRegistryImage(t *testing.T, unauthorized bool) (string, string) {
	t.Helper()

	handler := ggcrregistry.New(ggcrregistry.Logger(log.New(io.Discard, "", 0)))

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

	if !unauthorized {
		return seedHost + "/test/img:v1", digest.String()
	}

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)

		_, werr := w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`))
		assert.NoError(t, werr)
	}))
	t.Cleanup(authSrv.Close)

	return strings.TrimPrefix(authSrv.URL, "http://") + "/test/img:v1", digest.String()
}

// imageWatcherState marshals a container-image-watcher state for tests.
func imageWatcherState(t *testing.T, digest string) State {
	t.Helper()

	raw, err := json.Marshal(ContainerImageWatcherState{Digest: digest})
	require.NoError(t, err)

	return State(raw)
}

func Test_ContainerImageWatcher_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Image string
		Err   error
	}{
		"Valid image reference":     {Image: "nginx:1.27"},
		"Invalid image is rejected": {Image: "INVALID image ref", Err: ErrInvalidImage},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, (&ContainerImageWatcher{Image: c.Image}).Validate())
		})
	}
}

func Test_ContainerImageWatcher_Process(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		// Image is watched instead of the fake registry's image.
		Image        string
		Unauthorized bool
		// Missing watches a tag the fake registry does not hold.
		Missing bool
		// Stale has the state hold an old digest instead of the current one.
		Stale bool
		// State is sent instead of a state built from the digest.
		State  State
		Status Status
		Score  decimal.Decimal
		Err    error
	}{
		"Malformed state": {
			Image: "INVALID image ref",
			State: State(`{not json`),
			Err:   assert.AnError,
		},
		"Invalid image reference": {
			Image: "INVALID image ref",
			Err:   assert.AnError,
		},
		"Unauthorized registry is a status": {
			Unauthorized: true,
			Status:       StatusUnauthorized,
		},
		"Missing image is a status": {
			Missing: true,
			Status:  StatusImageNotFound,
		},
		"Changed digest drops the score to zero": {
			Stale:  true,
			Status: StatusActive,
			Score:  decimal.Zero,
		},
		"Unchanged digest keeps the full score": {
			Status: StatusActive,
			Score:  decimal.NewFromInt(100),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			image, digest := c.Image, "sha256:old"
			if image == "" {
				image, digest = newFakeRegistryImage(t, c.Unauthorized)
			}

			if c.Missing {
				image = strings.Replace(image, ":v1", ":v2", 1)
			}

			if c.Stale {
				digest = "sha256:old"
			}

			state := c.State
			if state == nil {
				state = imageWatcherState(t, digest)
			}

			res, err := (&ContainerImageWatcher{Image: image}).Process(context.Background(), stubInput{state: state})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, res.Status)
			assert.True(t, res.Score.Equal(c.Score), "score %s", res.Score)

			if c.Status != StatusActive {
				assert.Nil(t, res.State)

				return
			}

			assert.Equal(t, state, res.State)
		})
	}
}

func Test_ContainerImageWatcher_Reset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Unauthorized bool
		Status       Status
		Score        decimal.Decimal
	}{
		"Unauthorized registry is a status": {
			Unauthorized: true,
			Status:       StatusUnauthorized,
		},
		"Reset adopts the current digest at full score": {
			Status: StatusActive,
			Score:  decimal.NewFromInt(100),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			image, digest := newFakeRegistryImage(t, c.Unauthorized)

			res, err := (&ContainerImageWatcher{Image: image}).Reset(context.Background(), stubInput{})
			require.NoError(t, err)

			assert.Equal(t, c.Status, res.Status)
			assert.True(t, res.Score.Equal(c.Score), "score %s", res.Score)

			if c.Status != StatusActive {
				assert.Nil(t, res.State)

				return
			}

			assert.Equal(t, imageWatcherState(t, digest), res.State)
		})
	}
}
