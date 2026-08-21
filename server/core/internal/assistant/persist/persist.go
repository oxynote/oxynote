// Package persist holds everything one assistant conversation leaves
// outside the process: the history it resumes from, the checkpoint of a
// turn paused on a confirmation, the confirmation itself, and any tool
// result too large to keep in context.
//
// All four hang off one key, minted by SessionKey, so a conversation is
// addressed the same way wherever it is stored. Keeping them together
// is what makes that keyspace ownable — and two of them, the checkpoint
// and the offloaded results, share a single byte store.
//
// Failures to read or write are logged and swallowed rather than
// returned. Losing a conversation's stored state costs the user context,
// never their ability to chat; the one exception is the checkpoint
// store, whose contract belongs to the agent runner.
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

// BlobStore is the persistence surface for the opaque byte payloads one
// conversation accumulates: the checkpoint of a paused turn, and any
// tool result too large to keep in context. The redkit BytesStore
// satisfies it.
//
// Both users share this one declaration because they share the store
// itself; Offload never calls Delete, since an offloaded result is
// reclaimed by the conversation's expiry rather than by us.
//
//go:generate ../../../scripts/codegen/mock -t both BlobStore blob_store
type BlobStore interface {
	// Get should return the stored payload for the key, or
	// errutil.ErrNotFound when none exists yet.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set should persist the payload under the key.
	Set(ctx context.Context, key string, value []byte) error

	// Delete should remove the stored payload for the key.
	Delete(ctx context.Context, key string) error
}
