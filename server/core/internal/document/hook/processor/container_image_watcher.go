// Package processor implements the freshness hook processors.
package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/apps/registry"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/shopspring/decimal"
)

// ContainerImageWatcherStatus represents the status of the
// Container Image watcher processor.
type ContainerImageWatcherStatus string

const (
	// ContainerImageWatcherStatusActive indicates that the Container Image watcher
	// processor is active.
	ContainerImageWatcherStatusActive ContainerImageWatcherStatus = "active"

	// ContainerImageWatcherStatusUnauthorized indicates that the Container Image
	// watcher processor is unauthorized to access the container registry.
	ContainerImageWatcherStatusUnauthorized ContainerImageWatcherStatus = "unauthorized"
)

// ContainerImageWatcher specifies a processor that watches container images for updates.
type ContainerImageWatcher struct {
	PlainDeleter

	// Image is the container image to watch.
	Image string `json:"image"`
}

// Process processes the Container Image watcher hook.
func (ciw *ContainerImageWatcher) Process(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	var ciws ContainerImageWatcherState

	if err := json.Unmarshal(inp.State(), &ciws); err != nil {
		return decimal.Zero, nil, fmt.Errorf("unmarshaling container image watcher state: %w", err)
	}

	digest, err := registry.Digest(
		ctx,
		ciw.Image,
	)
	switch {
	case err == nil:
		ciws.Status = ContainerImageWatcherStatusActive
	case errors.Is(err, registry.ErrUnauthorized):
		ciws.Status = ContainerImageWatcherStatusUnauthorized
	default:
		return decimal.Zero, nil, fmt.Errorf("fetching container image digest: %w", err)
	}

	score := mathutil.Hundred
	if ciws.Digest != digest {
		score = decimal.Zero
	}

	state, err := json.Marshal(ciws)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling container image watcher state: %w", err)
	}

	return score, state, nil
}

// Reset resets the state of the Container Image watcher processor.
func (ciw *ContainerImageWatcher) Reset(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	var ciws ContainerImageWatcherState

	if state := inp.State(); state != nil {
		if err := json.Unmarshal(state, &ciws); err != nil {
			return decimal.Zero, nil, fmt.Errorf("unmarshaling container image watcher state: %w", err)
		}
	}

	digest, err := registry.Digest(
		ctx,
		ciw.Image,
	)

	switch {
	case err == nil:
		ciws.Status = ContainerImageWatcherStatusActive
	case errors.Is(err, registry.ErrUnauthorized):
		// Process records this as a status rather than an error; refusing to
		// create the hook here would make a currently-unauthorized image
		// impossible to watch at all, even though the status is exactly what
		// tells the user why.
		ciws.Status = ContainerImageWatcherStatusUnauthorized
	default:
		return decimal.Zero, nil, fmt.Errorf("fetching container image digest: %w", err)
	}

	ciws.Digest = digest

	state, err := json.Marshal(ciws)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling container image watcher state: %w", err)
	}

	if ciws.Status == ContainerImageWatcherStatusUnauthorized {
		return decimal.Zero, state, nil
	}

	return mathutil.Hundred, state, nil
}

// ContainerImageWatcherState represents the state of the container image
// watcher processor.
type ContainerImageWatcherState struct {
	// Status is the current status of the processor.
	Status ContainerImageWatcherStatus `json:"status"`

	// Digest is the last known digest of the container image.
	Digest string `json:"digest"`
}
