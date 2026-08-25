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

	raw, err := json.Marshal(ContainerImageWatcherState{
		Status: ContainerImageWatcherStatusActive,
		Digest: digest,
	})
	require.NoError(t, err)

	return State(raw)
}

func Test_ContainerImageWatcher_Process(t *testing.T) {
	t.Parallel()

	t.Run("Unchanged digest keeps the full score", func(t *testing.T) {
		t.Parallel()

		image, digest := newFakeRegistryImage(t, false)

		ciw := ContainerImageWatcher{Image: image}

		score, state, err := ciw.Process(context.Background(), stubInput{
			state: imageWatcherState(t, digest),
		})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.NewFromInt(100)))

		var ciws ContainerImageWatcherState

		require.NoError(t, json.Unmarshal(state, &ciws))
		assert.Equal(t, ContainerImageWatcherStatusActive, ciws.Status)
		assert.Equal(t, digest, ciws.Digest)
	})

	t.Run("Changed digest drops the score to zero", func(t *testing.T) {
		t.Parallel()

		image, _ := newFakeRegistryImage(t, false)

		ciw := ContainerImageWatcher{Image: image}

		score, _, err := ciw.Process(context.Background(), stubInput{
			state: imageWatcherState(t, "sha256:old"),
		})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.Zero))
	})

	t.Run("Unauthorized registry reports the status and scores zero", func(t *testing.T) {
		t.Parallel()

		image, _ := newFakeRegistryImage(t, true)

		ciw := ContainerImageWatcher{Image: image}

		// an empty baseline digest — a hook created or reset while
		// unauthorized — must not match the empty digest of a failed fetch.
		score, state, err := ciw.Process(context.Background(), stubInput{
			state: imageWatcherState(t, ""),
		})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.Zero))

		var ciws ContainerImageWatcherState

		require.NoError(t, json.Unmarshal(state, &ciws))
		assert.Equal(t, ContainerImageWatcherStatusUnauthorized, ciws.Status)
	})

	t.Run("Invalid image reference fails", func(t *testing.T) {
		t.Parallel()

		ciw := ContainerImageWatcher{Image: "INVALID image ref"}

		_, _, err := ciw.Process(context.Background(), stubInput{
			state: imageWatcherState(t, ""),
		})
		require.Error(t, err)
	})

	t.Run("Malformed state fails", func(t *testing.T) {
		t.Parallel()

		ciw := ContainerImageWatcher{}

		_, _, err := ciw.Process(context.Background(), stubInput{state: State(`{not json`)})
		require.Error(t, err)
	})
}

func Test_ContainerImageWatcher_Reset(t *testing.T) {
	t.Parallel()

	t.Run("Reset adopts the current digest at full score", func(t *testing.T) {
		t.Parallel()

		image, digest := newFakeRegistryImage(t, false)

		ciw := ContainerImageWatcher{Image: image}

		score, state, err := ciw.Reset(context.Background(), stubInput{})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.NewFromInt(100)))

		var ciws ContainerImageWatcherState

		require.NoError(t, json.Unmarshal(state, &ciws))
		assert.Equal(t, ContainerImageWatcherStatusActive, ciws.Status)
		assert.Equal(t, digest, ciws.Digest)
	})

	// Process records an unauthorized registry as a status; the reset does
	// the same, or a hook for an image whose credentials are currently wrong
	// could never be created or updated at all.
	t.Run("Unauthorized registry resets to a status", func(t *testing.T) {
		t.Parallel()

		image, _ := newFakeRegistryImage(t, true)

		ciw := ContainerImageWatcher{Image: image}

		score, state, err := ciw.Reset(context.Background(), stubInput{})
		require.NoError(t, err)
		assert.True(t, score.IsZero())

		var ciws ContainerImageWatcherState

		require.NoError(t, json.Unmarshal(state, &ciws))
		assert.Equal(t, ContainerImageWatcherStatusUnauthorized, ciws.Status)
	})

	t.Run("Malformed state fails", func(t *testing.T) {
		t.Parallel()

		ciw := ContainerImageWatcher{}

		_, _, err := ciw.Reset(context.Background(), stubInput{state: State(`{not json`)})
		require.Error(t, err)
	})
}
