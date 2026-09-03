package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/persist"
	persistMock "github.com/oxynote/oxynote/server/core/internal/assistant/persist/_mock"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCompaction(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Model   model.ToolCallingChatModel
		Backend *persist.Offload
		Writes  []string
		Err     error
	}{
		"Both middlewares are built": {
			Model:   &mock.ChatModel{},
			Backend: persist.NewOffload(&persistMock.BlobStore{}),
			Writes:  []string{"insert_block"},
		},
		"Summarization needs a model": {
			Backend: persist.NewOffload(&persistMock.BlobStore{}),
			Err:     assert.AnError,
		},
		"Reduction needs a backend": {
			Model: &mock.ChatModel{},
			Err:   errors.New("offload backend is required"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			mws, err := NewCompaction(context.Background(), c.Model, c.Backend, c.Writes)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// reduction handles tool-result bloat, summarization handles
			// conversation length; both are needed.
			assert.Len(t, mws, 2)
		})
	}
}

func Test_newClearRewriter(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Writes    []string
		Msg       *schema.Message
		Responses []*schema.Message
		Untouched bool
		Contains  []string
	}{
		"Names the tool whose output was cleared": {
			Writes: []string{"insert_block"},
			Msg: schema.AssistantMessage("", []schema.ToolCall{
				{Function: schema.FunctionCall{Name: "get_document"}},
			}),
			Contains: []string{"get_document", "cleared", "Call the tool again"},
		},
		"Names every tool in a cleared batch": {
			Msg: schema.AssistantMessage("", []schema.ToolCall{
				{Function: schema.FunctionCall{Name: "read_block"}},
				{Function: schema.FunctionCall{Name: "search_documents"}},
			}),
			Contains: []string{"read_block", "search_documents"},
		},
		"Survives a call with no tools": {
			Msg:      schema.AssistantMessage("", nil),
			Contains: []string{"cleared"},
		},
		"Round containing a write call is left untouched": {
			Writes: []string{"insert_block"},
			Msg: schema.AssistantMessage("", []schema.ToolCall{
				{ID: "1", Function: schema.FunctionCall{Name: "read_block"}},
				{ID: "2", Function: schema.FunctionCall{Name: "insert_block"}},
			}),
			Responses: []*schema.Message{
				schema.ToolMessage("read result", "1"),
				schema.ToolMessage("write result", "2"),
			},
			Untouched: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			msgs, err := newClearRewriter(c.Writes)(context.Background(), c.Msg, c.Responses)
			require.NoError(t, err)

			if c.Untouched {
				// eino invokes the rewriter for every round before it
				// consults ClearExcludeTools, so the only way a write
				// result survives clearing is the rewriter handing the
				// round back exactly as it was.
				assert.Equal(t, append([]*schema.Message{c.Msg}, c.Responses...), msgs)

				return
			}

			require.Len(t, msgs, 1)

			// the replacement is addressed to the model as a user-side
			// reminder, so it survives the same clearing pass.
			assert.Equal(t, schema.User, msgs[0].Role)

			for _, want := range c.Contains {
				assert.Contains(t, msgs[0].Content, want)
			}
		})
	}
}
