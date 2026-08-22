package persist

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// _offloadKeyPrefix namespaces offloaded results in the store.
const _offloadKeyPrefix = "assistant:offload:"

// Offload parks oversized tool results outside the conversation and
// hands them back on request. It satisfies the storage contract the
// reduction middleware writes through; the tool the model calls to read
// one back lives in the tools package, which owns everything the model
// can invoke.
//
// Without it a large result has to be destroyed to keep the conversation
// affordable, and whatever the model was reading is simply gone. Here
// the conversation keeps a preview and a handle, and the content stays
// retrievable for as long as the conversation does.
type Offload struct {
	// store holds the offloaded payloads, expiring with the
	// conversation.
	store BlobStore
}

// NewOffload creates a fresh instance of Offload.
func NewOffload(store BlobStore) *Offload {
	return &Offload{store: store}
}

// Write stores an offloaded tool result.
func (o *Offload) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	if req == nil {
		// NOCOV: the middleware never writes a nil request.
		return errors.New("offload write request is required")
	}

	if err := o.store.Set(ctx, offloadKey(req.FilePath), []byte(req.Content)); err != nil {
		return fmt.Errorf("offloading tool result: %w", err)
	}

	return nil
}

// Read returns a previously offloaded result. A result that has
// expired is reported as an instruction the model can act on rather
// than an opaque failure.
func (o *Offload) Read(ctx context.Context, path string) (string, error) {
	data, err := o.store.Get(ctx, offloadKey(path))

	switch {
	case err == nil:
		return string(data), nil
	case errors.Is(err, errutil.ErrNotFound):
		return "", fmt.Errorf("no stored output at %q; it has expired, so re-run the tool", path)
	default:
		return "", fmt.Errorf("reading offloaded tool result: %w", err)
	}
}

// offloadKey builds the key holding one offloaded result.
func offloadKey(path string) string {
	return _offloadKeyPrefix + strings.TrimPrefix(path, "/")
}
