// Package processor implements the freshness hook processors.
package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/apps/registry"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/shopspring/decimal"
)

// ErrInvalidImage is returned when a container image watcher's image
// reference cannot be parsed.
var ErrInvalidImage = errutil.New(http.StatusBadRequest, "document_hook.invalid_image", "invalid container image reference")

// ContainerImageWatcher specifies a processor that watches container images for updates.
type ContainerImageWatcher struct {
	PlainDeleter

	// Image is the container image to watch.
	Image string `json:"image"`
}

// Validate checks that the image reference can be parsed.
func (ciw *ContainerImageWatcher) Validate() error {
	if err := registry.ValidateReference(ciw.Image); err != nil {
		return ErrInvalidImage
	}

	return nil
}

// Process scores the hook down once the image digest moved since the reset.
func (ciw *ContainerImageWatcher) Process(ctx context.Context, inp Input) (Result, error) {
	var ciws ContainerImageWatcherState

	if err := json.Unmarshal(inp.State(), &ciws); err != nil {
		return Result{}, fmt.Errorf("unmarshaling container image watcher state: %w", err)
	}

	digest, status, err := ciw.digest(ctx)
	if err != nil {
		return Result{}, err
	}

	if status != StatusActive {
		return inactive(status), nil
	}

	score := mathutil.Hundred
	if ciws.Digest != digest {
		score = decimal.Zero
	}

	return active(score, ciws)
}

// Reset records the image's current digest as the baseline.
func (ciw *ContainerImageWatcher) Reset(ctx context.Context, _ Input) (Result, error) {
	digest, status, err := ciw.digest(ctx)
	if err != nil {
		return Result{}, err
	}

	if status != StatusActive {
		return inactive(status), nil
	}

	return active(mathutil.Hundred, ContainerImageWatcherState{Digest: digest})
}

// digest fetches the image digest. A status other than active means the
// registry answered with a reason the hook reports rather than fails on.
func (ciw *ContainerImageWatcher) digest(ctx context.Context) (string, Status, error) {
	digest, err := registry.Digest(ctx, ciw.Image)

	switch {
	case err == nil:
		return digest, StatusActive, nil
	case errors.Is(err, registry.ErrUnauthorized):
		return "", StatusUnauthorized, nil
	case errors.Is(err, registry.ErrNotFound):
		return "", StatusImageNotFound, nil
	default:
		return "", "", fmt.Errorf("fetching container image digest: %w", err)
	}
}

// ContainerImageWatcherState represents the state of the container image
// watcher processor.
type ContainerImageWatcherState struct {
	// Digest is the last known digest of the container image.
	Digest string `json:"digest"`
}
