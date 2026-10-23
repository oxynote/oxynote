package logutil

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Critical(t *testing.T) {
	out, b := testutil.NewBuffer()
	log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "time" {
				return slog.Attr{}
			}

			return a
		},
	}))

	Critical(log, errors.New("error")).Error("test")

	require.NoError(t, out.Flush())
	assert.JSONEq(t, `{
		"level":"ERROR", 
		"msg":"test",
		"error":"internal error"
	}`, b.String())
}

func Test_CriticalSkipFrames(t *testing.T) {
	cc := map[string]struct {
		Frames uint
		Result string
	}{
		"Successful capture with more skipped frames than the stacktrace has": {
			Frames: 10000,
			Result: `{
				"level":"ERROR",
				"error":"internal error",
				"msg":"test"
			}`,
		},
		"Successful capture without skipped frames": {
			Frames: 0,
			Result: `{
				"level":"ERROR",
				"error":"internal error",
				"msg":"test"
			}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == "time" {
						return slog.Attr{}
					}

					return a
				},
			}))

			require.NotPanics(t, func() {
				CriticalSkipFrames(log, errors.New("error"), c.Frames).Error("test")
			})

			require.NoError(t, out.Flush())
			assert.JSONEq(t, c.Result, b.String())
		})
	}
}

func Test_Recover(t *testing.T) {
	cc := map[string]struct {
		Panic           any
		Plan            *RecoveryPlan
		ShowCritical    bool
		Result          string
		BeforeSendCalls int
	}{
		"Nothing logged or sent without panic": {
			Plan: &RecoveryPlan{
				msg:          "test",
				localFields:  map[string]any{"hello": "world"},
				beforeSendFn: func() {},
			},
		},
		"Successful recovery from string type panic": {
			Panic: "error",
			Plan: &RecoveryPlan{
				msg:          "test",
				localFields:  map[string]any{"hello": "world"},
				beforeSendFn: func() {},
			},
			Result: `{
				"level":"ERROR", 
				"error":"internal error", 
				"msg":"test",
				"hello": "world"
			}`,
			BeforeSendCalls: 1,
		},
		"Successful recovery from error type panic": {
			Panic: errors.New("error"),
			Plan: &RecoveryPlan{
				msg:          "test",
				localFields:  map[string]any{"hello": "world"},
				beforeSendFn: func() {},
			},
			Result: `{
				"level":"ERROR", 
				"error":"internal error", 
				"msg":"test",
				"hello": "world"
			}`,
			BeforeSendCalls: 1,
		},
		"Successful recovery from unknown type panic": {
			Panic: 123,
			Plan: &RecoveryPlan{
				msg:          "test",
				localFields:  map[string]any{"hello": "world"},
				beforeSendFn: func() {},
			},
			Result: `{
				"level":"ERROR", 
				"error":"internal error", 
				"msg":"test",
				"hello": "world"
			}`,
			BeforeSendCalls: 1,
		},
		"Successful recovery from panic without beforeSendFn": {
			Panic: "error",
			Plan: &RecoveryPlan{
				msg:         "test",
				localFields: map[string]any{"hello": "world"},
			},
			Result: `{
				"level":"ERROR",
				"error":"internal error",
				"msg":"test",
				"hello": "world"
			}`,
		},
		"Successful recovery from panic without recovery plan": {
			Panic: "error",
			Result: `{
				"level":"ERROR", 
				"error":"internal error",
				"msg":"recovered from panic"
			}`,
		},
		"Successful recovery from panic with error message shown": {
			Panic: "error123",
			Plan: &RecoveryPlan{
				msg:         "test",
				localFields: map[string]any{"hello": "world"},
			},
			ShowCritical: true,
			Result: `{
				"level":"ERROR", 
				"error":"error123", 
				"msg":"test",
				"hello": "world"
			}`,
		},
	}

	for cn, c := range cc {
		// the cases write ShowCritical, a package var that
		// CriticalSkipFrames reads, so they cannot run in parallel.
		t.Run(cn, func(t *testing.T) {
			var beforeSendCalls int

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == "time" {
						return slog.Attr{}
					}

					return a
				},
			}))

			if c.Plan != nil && c.Plan.beforeSendFn != nil {
				c.Plan.beforeSendFn = func() {
					beforeSendCalls++
				}
			}

			ShowCritical = c.ShowCritical

			assert.NotPanics(t, func() {
				defer Recover(log, c.Plan)

				if c.Panic != nil {
					panic(c.Panic)
				}
			})

			assert.Equal(t, c.BeforeSendCalls, beforeSendCalls)
			require.NoError(t, out.Flush())

			if c.Result == "" {
				assert.Empty(t, b.String())
				return
			}

			assert.JSONEq(t, c.Result, b.String())
		})
	}

	ShowCritical = false
}

func Test_RecoveryValue(t *testing.T) {
	// the nil plan branch inside the returned closure is reachable
	// only through a direct call, since Recover replaces a nil plan
	// before invoking this helper.
	cc := map[string]struct {
		Panic  any
		Result string
	}{
		"Successful recovery from panic without recovery plan": {
			Panic: "error",
			Result: `{
				"level":"ERROR",
				"error":"internal error",
				"msg":"recovered from panic"
			}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == "time" {
						return slog.Attr{}
					}

					return a
				},
			}))

			RecoveryValue(log, nil)(c.Panic)

			require.NoError(t, out.Flush())
			assert.JSONEq(t, c.Result, b.String())
		})
	}
}

func Test_NewRecoveryPlan(t *testing.T) {
	p := NewRecoveryPlan("hello")
	require.NotNil(t, p)
	assert.Equal(t, "hello", p.msg)
}

func Test_RecoveryPlan_LocalFields(t *testing.T) {
	fields := map[string]any{"hello": "world"}
	p := (&RecoveryPlan{}).LocalFields(fields)
	require.NotNil(t, p)
	assert.Equal(t, fields, p.localFields)
}

func Test_RecoveryPlan_BeforeSendFunc(t *testing.T) {
	p := (&RecoveryPlan{}).BeforeSendFunc(func() {})
	require.NotNil(t, p)
	assert.NotNil(t, p.beforeSendFn)
}

func Test_NewDebugWriter(t *testing.T) {
	w := NewDebugWriter(slog.New(slog.DiscardHandler), "hello", "123")
	require.NotNil(t, w)
	assert.NotZero(t, w.log)
	assert.Equal(t, []string{"hello", "123"}, w.exclude)
}

func Test_DebugWriter_Write(t *testing.T) {
	cc := map[string]struct {
		Inp    string
		Logged bool
	}{
		"Message excluded by the first keyword": {
			Inp: "testhello",
		},
		"Message excluded by the second keyword": {
			Inp: "123hello456",
		},
		"Successful write": {
			Inp:    "hello",
			Logged: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
			w := DebugWriter{log: log, exclude: []string{"test", "123"}}

			n, err := w.Write([]byte(c.Inp))
			assert.NoError(t, err)
			assert.Equal(t, len(c.Inp), n)
			assert.NoError(t, out.Flush())

			if !c.Logged {
				assert.NotContains(t, b.String(), `"level":"DEBUG"`)
				assert.NotContains(t, b.String(), `"msg":"`+c.Inp+`"`)

				return
			}

			assert.Contains(t, b.String(), `"level":"DEBUG"`)
			assert.Contains(t, b.String(), `"msg":"`+c.Inp+`"`)
		})
	}
}

func Test_DebugWriter_StdLogger(t *testing.T) {
	w := &DebugWriter{}
	l := w.StdLogger()
	require.NotNil(t, l)
	assert.Same(t, w, l.Writer())
}
