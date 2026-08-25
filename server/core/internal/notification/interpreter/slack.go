package interpreter

import (
	"fmt"
	"strings"
)

// _mrkdwnEscaper escapes the characters Slack's mrkdwn assigns meaning to
// (& < >). "|" has no escape sequence and would start the display text of a
// "<url|text>" link early, so it is stripped.
var _mrkdwnEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"|", "",
)

// SlackFormatter formats message parts using Slack's mrkdwn syntax.
type SlackFormatter struct{}

// NewSlackFormatter creates a fresh instance of the Slack formatter.
func NewSlackFormatter() *SlackFormatter {
	return &SlackFormatter{}
}

// Link renders a link in Slack's mrkdwn "<url|text>" syntax. Both parts are
// escaped: the display text is the user-controlled document name, which
// could otherwise inject arbitrary mrkdwn into the message.
func (sf *SlackFormatter) Link(url, text string) string {
	return fmt.Sprintf("<%s|%s>", _mrkdwnEscaper.Replace(url), _mrkdwnEscaper.Replace(text))
}
