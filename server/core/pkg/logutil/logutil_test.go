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
		CriticalSkipFrames(log, errors.New("error"), 10000).Error("test")
	})

	require.NoError(t, out.Flush())
	assert.JSONEq(t, `{
		"level":"ERROR", 
		"error":"internal error",
		"msg":"test"
	}`, b.String())

	b.Reset()

	require.NotPanics(t, func() {
		CriticalSkipFrames(log, errors.New("error"), 0).Error("test")
	})

	require.NoError(t, out.Flush())
	assert.JSONEq(t, `{
		"level":"ERROR", 
		"error":"internal error",
		"msg":"test"
	}`, b.String())
}

func Test_Recover(t *testing.T) {
	cc := map[string]struct {
		Panic        any
		Plan         *RecoveryPlan
		ShowCritical bool
		Result       string
	}{
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
		t.Run(cn, func(t *testing.T) {
			var calledBefore bool

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
					calledBefore = true
				}
			}

			ShowCritical = c.ShowCritical

			assert.NotPanics(t, func() {
				defer Recover(log, c.Plan)
				panic(c.Panic)
			})

			require.NoError(t, out.Flush())
			assert.JSONEq(t, c.Result, b.String())

			if c.Plan == nil {
				return
			}

			assert.Equal(t, c.Plan.beforeSendFn != nil, calledBefore)
		})
	}

	ShowCritical = false
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
	out, b := testutil.NewBuffer()
	log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	w := DebugWriter{log: log, exclude: []string{"test", "123"}}

	n, err := w.Write([]byte("testhello"))
	assert.NoError(t, err)
	assert.Equal(t, len("testhello"), n)
	assert.NoError(t, out.Flush())
	assert.NotContains(t, b.String(), `"level":"DEBUG"`)
	assert.NotContains(t, b.String(), `"msg":"testhello"`)

	n, err = w.Write([]byte("123hello456"))
	assert.NoError(t, err)
	assert.Equal(t, len("123hello456"), n)
	assert.NoError(t, out.Flush())
	assert.NotContains(t, b.String(), `"level":"DEBUG"`)
	assert.NotContains(t, b.String(), `"msg":"123hello456"`)

	n, err = w.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, len("hello"), n)
	assert.NoError(t, out.Flush())
	assert.Contains(t, b.String(), `"level":"DEBUG"`)
	assert.Contains(t, b.String(), `"msg":"hello"`)
}

func Test_DebugWriter_StdLogger(t *testing.T) {
	w := &DebugWriter{}
	l := w.StdLogger()
	require.NotNil(t, l)
	assert.Same(t, w, l.Writer())
}
