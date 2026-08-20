// Package offload parks oversized tool results outside the conversation
// and hands the model a tool to fetch them back.
//
// Without it a large result has to be destroyed to keep the conversation
// affordable, and whatever the model was reading is simply gone. Here the
// conversation keeps a preview and a handle, and the content stays
// retrievable for as long as the conversation does.
package offload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// ReadToolName is the tool the model calls to retrieve a result that was
// too large to keep in the conversation.
//
// It is deliberately not called read_file: the assistant's vocabulary is
// documents and blocks, and a generic file tool sitting next to
// read_block invites the model to reach for the wrong one.
const ReadToolName = "read_tool_output"

const (
	// _keyPrefix namespaces offloaded results in the store.
	_keyPrefix = "assistant:offload:"

	// _refKey is the argument naming which offloaded result to retrieve.
	// It matches the placeholder the reduction middleware leaves in the
	// conversation.
	_refKey = "file_path"
)

// Backend stores oversized tool results so the model can fetch them on
// demand. It satisfies the storage contract the reduction middleware
// writes through.
type Backend struct {
	// store holds the offloaded payloads, expiring with the conversation.
	store Store
}

// New creates a fresh instance of Backend.
func New(store Store) *Backend {
	return &Backend{store: store}
}

// Write stores an offloaded tool result.
func (b *Backend) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	if req == nil {
		// NOCOV: the middleware never writes a nil request.
		return errors.New("offload write request is required")
	}

	if err := b.store.Set(ctx, key(req.FilePath), []byte(req.Content)); err != nil {
		return fmt.Errorf("offloading tool result: %w", err)
	}

	return nil
}

// ReadTool exposes offloaded results to the model. The middleware only
// writes, so retrieval is ours to provide.
func (b *Backend) ReadTool() tool.InvokableTool {
	return &readTool{backend: b}
}

// read returns a previously offloaded result.
func (b *Backend) read(ctx context.Context, path string) (string, error) {
	data, err := b.store.Get(ctx, key(path))

	switch {
	case err == nil:
		return string(data), nil
	case errors.Is(err, errutil.ErrNotFound):
		return "", fmt.Errorf("no stored output at %q; it has expired, so re-run the tool", path)
	default:
		return "", fmt.Errorf("reading offloaded tool result: %w", err)
	}
}

// key builds the store key holding one offloaded result.
func key(path string) string {
	return _keyPrefix + strings.TrimPrefix(path, "/")
}

// readTool lets the model retrieve a tool result that was moved out of
// the conversation for size.
type readTool struct {
	// backend resolves the stored payload.
	backend *Backend
}

// Info describes the retrieval tool to the model.
func (rt *readTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ReadToolName,
		Desc: "Retrieve the full output of an earlier tool call that was too large to keep in the conversation. " +
			"Pass the path shown in the truncation notice.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			_refKey: {
				Type:     schema.String,
				Desc:     "The stored output path from the truncation notice.",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun returns the stored output.
func (rt *readTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		FilePath string `json:"file_path"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("%s: invalid input: %w", ReadToolName, err)
	}

	if in.FilePath == "" {
		return "", fmt.Errorf("%s: %s is required", ReadToolName, _refKey)
	}

	return rt.backend.read(ctx, in.FilePath)
}

// Store is the persistence surface for offloaded tool results.
//
//go:generate ../../../scripts/codegen/mock -t both Store store
type Store interface {
	// Get should return the payload stored under the key, or
	// errutil.ErrNotFound when none exists.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set should persist the payload under the key.
	Set(ctx context.Context, key string, value []byte) error
}
