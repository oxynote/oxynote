// Package interpreter renders notifications into human-readable content.
package interpreter

// Message represents a human-readable notification message.
type Message struct {
	// Text specifies the message text.
	Text string `json:"text"`
}
