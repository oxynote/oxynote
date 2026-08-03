package slackhandler

import (
	"context"
	"time"

	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// Message represents a new message.
type Message struct {
	// ID is the unique identifier for the message.
	ID xid.ID `json:"id"`
}

// BindPostMessage binds the post message callback to a WebSocket topic.
func (h *Handler) BindPostMessage(tpc wsserver.Topic) {
	h.message.postCallback = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tpc.PublishMany(ctx, Message{}, nil)
	}
}
