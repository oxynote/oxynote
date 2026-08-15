package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewTextDeltaMessage(t *testing.T) {
	t.Parallel()

	msg := NewTextDeltaMessage("hi")
	assert.Equal(t, TextDeltaMessage{Type: ServerTypeTextDelta, Content: "hi"}, msg)
}

func Test_NewTextEndMessage(t *testing.T) {
	t.Parallel()

	msg := NewTextEndMessage(TextEndKindStatus)
	assert.Equal(t, TextEndMessage{Type: ServerTypeTextEnd, Kind: TextEndKindStatus}, msg)
}

func Test_NewToolStatusMessage(t *testing.T) {
	t.Parallel()

	msg := NewToolStatusMessage("read_block", "Reading a block")
	assert.Equal(t, ToolStatusMessage{
		Type:  ServerTypeToolStatus,
		Tool:  "read_block",
		Label: "Reading a block",
	}, msg)
}

func Test_NewDoneMessage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DoneMessage{Type: ServerTypeDone}, NewDoneMessage())
}

func Test_NewErrorMessage(t *testing.T) {
	t.Parallel()

	msg := NewErrorMessage("boom")
	assert.Equal(t, ErrorMessage{Type: ServerTypeError, Message: "boom"}, msg)
}

func Test_NewHistoryMessage(t *testing.T) {
	t.Parallel()

	entries := []HistoryEntry{{Role: "user", Content: "hi"}}

	msg := NewHistoryMessage(entries)
	assert.Equal(t, HistoryMessage{Type: ServerTypeHistory, Messages: entries}, msg)
}

func Test_NewConfirmRequest(t *testing.T) {
	t.Parallel()

	actions := []ConfirmAction{{Tool: "delete_block", Summary: "Delete a block"}}

	msg := NewConfirmRequest("turn-1", actions)
	assert.Equal(t, ConfirmRequest{
		Type:    ServerTypeConfirmRequest,
		TurnID:  "turn-1",
		Actions: actions,
	}, msg)
}
