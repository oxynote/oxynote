package interpreter

import "fmt"

// SlackFormatter formats message parts using Slack's mrkdwn syntax.
type SlackFormatter struct{}

// NewSlackFormatter creates a fresh instance of the Slack formatter.
func NewSlackFormatter() *SlackFormatter {
	return &SlackFormatter{}
}

// Link renders a link in Slack's mrkdwn "<url|text>" syntax.
func (sf *SlackFormatter) Link(url, text string) string {
	return fmt.Sprintf("<%s|%s>", url, text)
}
