package persist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// Checkpoints stores the paused turns of a conversation, adapting the
// byte store to the checkpoint contract the agent runner expects.
//
// A checkpoint is the whole paused turn: the conversation so far, the
// tool calls awaiting approval, and the session values that go with
// them. Holding it in Redis rather than in memory is what lets a
// pending confirmation outlive both a dropped connection and a restart
// of this process.
type Checkpoints struct {
	log *slog.Logger

	// store is the byte store holding the checkpoints.
	store BlobStore
}

// NewCheckpoints creates a fresh instance of Checkpoints.
func NewCheckpoints(log *slog.Logger, store BlobStore) *Checkpoints {
	return &Checkpoints{log: log, store: store}
}

// Get retrieves a checkpoint, reporting a missing one as absent rather
// than as a failure: the runner treats "no checkpoint" as a fresh run.
func (c *Checkpoints) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	data, err := c.store.Get(ctx, checkpointKey(checkPointID))

	switch {
	case err == nil:
		return data, true, nil
	case errors.Is(err, errutil.ErrNotFound):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("fetching checkpoint: %w", err)
	}
}

// Set stores a checkpoint under the given id.
func (c *Checkpoints) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	if err := c.store.Set(ctx, checkpointKey(checkPointID), checkPoint); err != nil {
		return fmt.Errorf("storing checkpoint: %w", err)
	}

	return nil
}

// Delete removes a checkpoint.
//
// The runner never calls this: it only ever writes checkpoints, and
// consults the optional deleter interface from a code path this
// assistant does not use. Cleanup is therefore ours, driven from the
// points where a turn stops being resumable — see Clear.
func (c *Checkpoints) Delete(ctx context.Context, checkPointID string) error {
	if err := c.store.Delete(ctx, checkpointKey(checkPointID)); err != nil {
		return fmt.Errorf("deleting checkpoint: %w", err)
	}

	return nil
}

// Clear drops the paused state of a turn that can no longer be resumed.
// A checkpoint is only ever reached through a pending confirmation, so
// once that is gone the checkpoint is unreachable and would otherwise
// sit in Redis holding a whole conversation until it expired.
func (c *Checkpoints) Clear(ctx context.Context, key string) {
	if err := c.Delete(ctx, key); err != nil {
		c.log.Error("failed to clear checkpoint",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// checkpointKey builds the key holding one conversation's checkpoint.
func checkpointKey(checkPointID string) string {
	return "assistant:checkpoint:" + checkPointID
}
