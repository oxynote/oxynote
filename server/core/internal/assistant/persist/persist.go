// Package persist holds everything one assistant conversation leaves
// outside the process: the history it resumes from, the checkpoint of a
// turn paused on a confirmation, the confirmation itself, and any tool
// result too large to keep in context.
//
// All four hang off one key from SessionKey. The checkpoint and the
// offloaded results share one byte store. Read and write failures are
// logged, not returned, so lost state costs context but never the chat.
// The checkpoint store is the exception, since its contract is the agent
// runner's.
package persist

import (
	"context"
	"fmt"
)

// SessionKey builds the key addressing one organisation/user pair's
// conversation. It doubles as the checkpoint id, so a paused turn and
// the history it belongs to are addressed the same way.
func SessionKey(orgID, userID string) string {
	return fmt.Sprintf("assistant:session:%s:%s", orgID, userID)
}

// Starter is the background maintenance a conversation store runs for
// as long as the process does.
type Starter interface {
	// Start should run the store's maintenance until ctx is cancelled.
	// It should block, so the caller owns the goroutine it runs on.
	Start(ctx context.Context)
}

// BlobStore stores a conversation's byte payloads: the checkpoint of a
// paused turn and the offloaded tool results. Offload never deletes, as
// the conversation's expiry reclaims its results.
//
//go:generate ../../../scripts/codegen/mock -t both BlobStore blob_store
type BlobStore interface {
	Starter

	// Get should return the stored payload for the key, or
	// errutil.ErrNotFound when none exists yet.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set should persist the payload under the key.
	Set(ctx context.Context, key string, value []byte) error

	// Delete should remove the stored payload for the key.
	Delete(ctx context.Context, key string) error
}
