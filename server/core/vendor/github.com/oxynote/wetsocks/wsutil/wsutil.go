// Package wsutil provides helper functions for websocket-related logic.
package wsutil

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"syscall"

	"github.com/coder/websocket"
	"github.com/oxynote/wetsocks/internal/ctxutil"
	"golang.org/x/exp/slices"
)

var errInvalidTopicFormat = errors.New("invalid topic format")

// _topicRe is a regexp preset that is used to check raw topic strings.
var _topicRe = regexp.MustCompile(`^([a-zA-Z0-9-]+~|)[a-zA-Z0-9-]+@(([a-zA-Z0-9-{}_]\.?))+[^.]$`) //nolint:gocritic // it suggests rewriting the regexp into something that wouldn't work.

// LogError checks the provided websocket error and logs it accordingly.
// If critical is set to true, then too big payload errors will be logged
// as well.
func LogError(logger *slog.Logger, err error, critical bool) {
	if IsClosure(err, websocket.StatusMessageTooBig) {
		return
	}

	if critical && IsStatus(err, websocket.StatusMessageTooBig) {
		logger.Error("websocket payload too big", "error", err)
		return
	}

	logger.Error("websocket error", "error", err)
}

// IsStatus returns whether error's websocket status code is present
// in provided status codes.
// Returns false if error is not a websocket error.
func IsStatus(err error, statusCodes ...websocket.StatusCode) bool {
	status := websocket.CloseStatus(err)
	if status == -1 {
		return false
	}

	return slices.Contains(statusCodes, status)
}

// IsClosure checks whether the error indicates that the connection
// is closed.
func IsClosure(err error, skipCodes ...websocket.StatusCode) bool {
	code := websocket.CloseStatus(err)
	if code != -1 && !slices.Contains(skipCodes, code) {
		return true
	}

	// NOTE: This is a workaround for a bug in coder/websocket. It seems
	// like io.EOF error never gets wrapped, so we need to perform a
	// string check.
	return ctxutil.IsInterrupted(err) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), io.EOF.Error())
}

// Topic represents a parsed WebSocket topic.
// The general form represented is:
//
//	[descriptor~]operation@path.to.resource
type Topic struct {
	Descriptor string
	Operation  string
	Path       []string
}

// String assembles the topic into a valid topic string.
func (t Topic) String() string {
	str := fmt.Sprintf("%s@%s", t.Operation, strings.Join(t.Path, "."))
	if t.Descriptor != "" {
		str = fmt.Sprintf("%s~%s", t.Descriptor, str)
	}

	return str
}

// ParseTopic parses rawTopic into a topic structure.
func ParseTopic(rawTopic string) (Topic, error) {
	if !_topicRe.MatchString(rawTopic) {
		return Topic{}, errInvalidTopicFormat
	}

	var t Topic

	ss := strings.Split(rawTopic, "~")
	if len(ss) == 2 {
		t.Descriptor = ss[0]
		ss = ss[1:]
	}

	ss = strings.Split(ss[0], "@") // length is ensured by regexp
	t.Operation = ss[0]
	t.Path = strings.Split(ss[1], ".")

	return t, nil
}
