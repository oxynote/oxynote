package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// checkpointStore adapts the assistant's byte store to the checkpoint
// contract the agent runner expects.
//
// A checkpoint is the whole paused turn: the conversation so far, the
// tool calls awaiting approval, and the session values that go with
// them. Holding it in Redis rather than in memory is what lets a
// pending confirmation outlive both a dropped connection and a restart
// of this process.
type checkpointStore struct {
	// store is the Redis-backed byte store holding the checkpoints.
	store BlobStore
}

// Get retrieves a checkpoint, reporting a missing one as absent rather
// than as a failure: the runner treats "no checkpoint" as a fresh run.
func (cs *checkpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	data, err := cs.store.Get(ctx, createCheckpointKey(checkPointID))

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
func (cs *checkpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	if err := cs.store.Set(ctx, createCheckpointKey(checkPointID), checkPoint); err != nil {
		return fmt.Errorf("storing checkpoint: %w", err)
	}

	return nil
}

// Delete removes a checkpoint.
//
// The runner never calls this: it only ever writes checkpoints, and
// consults the optional deleter interface from a code path this
// assistant does not use. Cleanup is therefore ours, driven from the
// points where a turn stops being resumable.
func (cs *checkpointStore) Delete(ctx context.Context, checkPointID string) error {
	if err := cs.store.Delete(ctx, createCheckpointKey(checkPointID)); err != nil {
		return fmt.Errorf("deleting checkpoint: %w", err)
	}

	return nil
}

// clearCheckpoint drops the paused state of a turn that can no longer
// be resumed. A checkpoint is only ever reached through a pending
// confirmation, so once that is gone the checkpoint is unreachable and
// would otherwise sit in Redis holding a whole conversation until it
// expired.
func (m *Manager) clearCheckpoint(ctx context.Context, key string) {
	if err := m.blobs.Delete(ctx, createCheckpointKey(key)); err != nil {
		m.log.Error("failed to clear checkpoint",
			slog.String("error", err.Error()),
			slog.String("key", key),
		)
	}
}

// createCheckpointKey builds the Redis key holding one conversation's
// checkpoint.
func createCheckpointKey(checkPointID string) string {
	return "assistant:checkpoint:" + checkPointID
}

// BlobStore is the persistence surface for the opaque byte payloads
// one conversation accumulates: the checkpoint of a paused turn, and
// any tool result too large to keep in context. The redkit BytesStore
// satisfies it.
//
//go:generate ../../scripts/codegen/mock -t internal BlobStore blob_store
type BlobStore interface {
	// Get should return the stored payload for the key, or
	// errutil.ErrNotFound when none exists yet.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set should persist the payload under the key.
	Set(ctx context.Context, key string, value []byte) error

	// Delete should remove the stored payload for the key.
	Delete(ctx context.Context, key string) error
}
