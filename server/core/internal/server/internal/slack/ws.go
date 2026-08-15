package slack

import (
	"context"
	"time"

	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// _publishTimeout bounds each WebSocket publish triggered by a domain
// callback.
const _publishTimeout = 5 * time.Second

// Message represents a new message.
type Message struct {
	// ID is the unique identifier for the message.
	ID xid.ID `json:"id"`
}

// BindPostMessage binds the post message callback to a WebSocket topic.
func (h *Handler) BindPostMessage(tpc wsserver.Topic) {
	h.message.postCallback = func() {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, Message{}, nil)
	}
}
