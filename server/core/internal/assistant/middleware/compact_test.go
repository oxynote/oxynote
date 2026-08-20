package middleware

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	mock "github.com/oxynote/oxynote/server/core/internal/_mock"
	"github.com/oxynote/oxynote/server/core/internal/assistant/offload"
	offloadMock "github.com/oxynote/oxynote/server/core/internal/assistant/offload/_mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCompaction(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Model   model.ToolCallingChatModel
		Backend *offload.Backend
		Writes  []string
		Err     bool
	}{
		"Both middlewares are built": {
			Model:   &mock.ChatModel{},
			Backend: offload.New(&offloadMock.Store{}),
			Writes:  []string{"insert_block"},
		},
		"Summarization needs a model": {
			Backend: offload.New(&offloadMock.Store{}),
			Err:     true,
		},
		"Reduction needs a backend": {
			Model: &mock.ChatModel{},
			Err:   true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			mws, err := NewCompaction(context.Background(), c.Model, c.Backend, c.Writes)
			if c.Err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// reduction handles tool-result bloat, summarization handles
			// conversation length; both are needed.
			assert.Len(t, mws, 2)
		})
	}
}

func Test_rewriteClearedRead(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Msg      *schema.Message
		Contains []string
	}{
		"Names the tool whose output was cleared": {
			Msg: schema.AssistantMessage("", []schema.ToolCall{
				{Function: schema.FunctionCall{Name: "read_document_summary"}},
			}),
			Contains: []string{"read_document_summary", "cleared", "Call the tool again"},
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
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			msgs, err := rewriteClearedRead(context.Background(), c.Msg, nil)
			require.NoError(t, err)
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
