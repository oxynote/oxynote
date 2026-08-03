// Package logutil provides helper functions for more convenient logging
// and error tracking.
package logutil

import (
	"errors"
	stdLog "log"
	"log/slog"
	"strings"

	"github.com/getsentry/sentry-go"
)

// ShowCritical determines whether critical errors should be logged by the
// logger or not.
var ShowCritical bool //nolint:gochecknoglobals // since sentry config is global, this must be too

// Critical creates an error level log event and sends the error to the
// remote error tracking service.
func Critical(log *slog.Logger, err error) *slog.Logger {
	return CriticalSkipFrames(log, err, 2)
}

// CriticalSkipFrames creates an error level log event and sends the error
// to the remote error tracking service.
// It also allows specifying the number of stacktrace frames to skip.
func CriticalSkipFrames(log *slog.Logger, err error, frames uint) *slog.Logger {
	st := sentry.NewStacktrace()
	if int(frames) > len(st.Frames) {
		st.Frames = nil
	} else {
		st.Frames = st.Frames[:len(st.Frames)-int(frames)]
	}

	e := sentry.NewEvent()
	e.Level = sentry.LevelError
	e.Exception = []sentry.Exception{{
		Type:       err.Error(),
		Stacktrace: st,
	}}
	sentry.CaptureEvent(e)

	if !ShowCritical {
		// since critical errors might expose some of the internal
		// logic, they shouldn't be logged locally
		err = errors.New("internal error")
	}

	return log.With("error", err.Error())
}

// Recover handles recoveries from panics, sends their info to the
// remote error tracking service and logs them locally.
// If no panic occurs, nothing will be logged / sent.
// Function should be called in defer.
func Recover(log *slog.Logger, plan *RecoveryPlan) {
	if r := recover(); r != nil { //nolint:revive // this function should be called in defer
		if plan == nil {
			plan = &RecoveryPlan{
				msg: "recovered from panic",
			}
		}

		plan.skipFrames = 3

		ProcessRecoveryValue(log, plan)(r)
	}
}

// ProcessRecoveryValue handles panic values, sends their info to the
// remote error tracking service and logs them locally.
func ProcessRecoveryValue(log *slog.Logger, plan *RecoveryPlan) func(any) {
	skipFrames := uint(2)
	if plan != nil && plan.skipFrames != 0 {
		skipFrames = plan.skipFrames
	}

	return func(r any) {
		var rerr error

		switch v := r.(type) {
		case string:
			rerr = errors.New(v)
		case error:
			rerr = v
		default:
			rerr = errors.New("panic of unknown type")
		}

		if plan == nil {
			plan = &RecoveryPlan{
				msg: "recovered from panic",
			}
		}

		if plan.beforeSendFn != nil {
			plan.beforeSendFn()
		}

		log = CriticalSkipFrames(log, rerr, skipFrames)

		for k, v := range plan.localFields {
			log = log.With(k, v)
		}

		log.Error(plan.msg)

		if plan.afterSendFn != nil {
			plan.afterSendFn()
		}
	}
}

// RecoveryPlan contains data that is used during panic handling
// and error event sending to the remote error tracking service.
type RecoveryPlan struct {
	msg          string
	localFields  map[string]any
	beforeSendFn func()
	afterSendFn  func()
	skipFrames   uint
}

// NewRecoveryPlan creates a fresh instance of recovery plan.
func NewRecoveryPlan(msg string) *RecoveryPlan {
	return &RecoveryPlan{msg: msg}
}

// LocalFields adds a map of local fields that will be included in the
// local log event but not sent to the remote error tracking service.
func (r *RecoveryPlan) LocalFields(fields map[string]any) *RecoveryPlan {
	r.localFields = fields
	return r
}

// BeforeSendFunc adds a function to be called before an event is sent
// to the remote error tracking service.
func (r *RecoveryPlan) BeforeSendFunc(fn func()) *RecoveryPlan {
	r.beforeSendFn = fn
	return r
}

// AfterSendFunc adds a function to be called after an event is sent
// to the remote error tracking service.
func (r *RecoveryPlan) AfterSendFunc(fn func()) *RecoveryPlan {
	r.afterSendFn = fn
	return r
}

// DebugWriter implements io.Writer and writes all incoming data
// to the wrapped logger as a debug message.
// Useful when standard logger needs to be replaced.
type DebugWriter struct {
	log     *slog.Logger
	exclude []string
}

// NewDebugWriter creates a fresh debug writer.
// A list of keywords may be supplied to filter out messages that
// contain them.
func NewDebugWriter(log *slog.Logger, exclude ...string) *DebugWriter {
	return &DebugWriter{log: log, exclude: exclude}
}

// Write writes provided data to the logger as a debug message.
func (d *DebugWriter) Write(p []byte) (n int, err error) {
	s := strings.ReplaceAll(string(p), "\n", "")

	for _, v := range d.exclude {
		if strings.Contains(s, v) {
			return len(p), nil
		}
	}

	d.log.Debug(s)

	return len(p), nil
}

// StdLogger returns a std logger with receiver set as writer.
func (d *DebugWriter) StdLogger() *stdLog.Logger {
	return stdLog.New(d, "", 0)
}

// Component creates a logger with a preset component field.
func Component(log *slog.Logger, comp string) *slog.Logger {
	return log.With("component", comp)
}
