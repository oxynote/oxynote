package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Info_toEino asserts every tool still presents the model the
// exact description it did before the tools were split into their own
// files. The golden file was captured from the previous implementation.
//
// This is the load-bearing test of the restructure: the schemas were
// retyped by hand, and a dropped required field or a truncated
// description degrades the assistant in a way no compile error catches.
func Test_Info_toEino(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile("testdata/tool_schemas.golden")
	require.NoError(t, err)

	s := New(testDeps(nil, nil, nil))

	got := map[string]any{}

	for _, bt := range s.Tools() {
		info, ierr := bt.Info(context.Background())
		require.NoError(t, ierr)

		raw, merr := json.Marshal(info)
		require.NoError(t, merr)

		var v any
		require.NoError(t, json.Unmarshal(raw, &v))

		got[info.Name] = v
	}

	data, err := json.Marshal(got)
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(data))
}

func Test_newEinoTool(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)
	tl := getDocument{}

	et := newEinoTool(tl, d)
	require.NotNil(t, et)

	assert.Equal(t, tl, et.tl)
	assert.Same(t, d, et.deps)

	// the description is resolved once, because it never varies by call.
	assert.Equal(t, NameGetDocument, et.info.Name)
}

func Test_einoTool_Info(t *testing.T) {
	t.Parallel()

	info, err := newEinoTool(getDocument{}, testDeps(nil, nil, nil)).Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, string(NameGetDocument), info.Name)
	assert.NotEmpty(t, info.Desc)
	require.NotNil(t, info.ParamsOneOf)
}

func Test_einoTool_InvokableRun(t *testing.T) {
	t.Parallel()

	// error: the arguments never reach a tool that cannot read them
	_, err := newEinoTool(getDocument{}, testDeps(nil, nil, nil)).
		InvokableRun(context.Background(), `{`)
	require.Error(t, err)

	// success: the tool is handed an input carrying this call's args
	res, err := newEinoTool(listDocuments{}, testDeps(nil, nil, nil)).
		InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"documents":null}`, res)
}

func Test_einoTool_Title(t *testing.T) {
	t.Parallel()

	et := newEinoTool(readDocumentSummary{}, testDeps(stubDocumentDB(), nil, nil))

	got := et.Title(context.Background(), json.RawMessage(`{"document_id":"`+_testDocID+`"}`))
	assert.Equal(t, "Reading Runbook", got)
}

func Test_einoTool_Confirm(t *testing.T) {
	t.Parallel()

	// a write describes its pending change
	et := newEinoTool(deleteDocument{}, testDeps(stubDocumentDB(), nil, nil))

	got := et.Confirm(context.Background(), json.RawMessage(`{"document_id":"`+_testDocID+`"}`))
	assert.Equal(t, string(NameDeleteDocument), got.Tool)
	assert.Equal(t, "Delete Runbook", got.Summary)

	// a read has nothing to confirm, and the gate is never applied to
	// one, so it degrades to a bare summary rather than panicking.
	read := newEinoTool(getDocument{}, testDeps(nil, nil, nil))
	assert.Equal(t, ConfirmActionSummary{Tool: string(NameGetDocument)},
		read.Confirm(context.Background(), json.RawMessage(`{}`)))
}

func Test_readToolOutput_Info(t *testing.T) {
	t.Parallel()

	info := readToolOutput{}.Info()

	// the tool is named for tool output, not files: sitting next to
	// read_block, a generic read_file would invite the wrong call.
	assert.Equal(t, NameReadToolOutput, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.Properties, _offloadPathKey)
	assert.Equal(t, []string{_offloadPathKey}, info.Required)
}

func Test_readToolOutput_Traits(t *testing.T) {
	t.Parallel()

	// the paths it takes only exist inside a conversation, so it must
	// never be offered to a client that has none.
	assert.Equal(t, Traits{Internal: true}, readToolOutput{}.Traits())
}

func Test_readToolOutput_Title(t *testing.T) {
	t.Parallel()

	assert.Empty(t, readToolOutput{}.Title(testInput(testDeps(nil, nil, nil), NameReadToolOutput, `{}`)))
}

func Test_readToolOutput_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Read   func(path string) (string, error)
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {
			Args: `{`,
			Err:  assert.AnError,
		},
		"Missing path": {
			Args: `{}`,
			Err:  errors.New("read_tool_output: file_path is required"),
		},
		"Expired output tells the model to re-run the tool": {
			Read: func(string) (string, error) {
				return "", errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`)
			},
			Args: `{"file_path":"trunc/1"}`,
			Err:  errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`),
		},
		"Stored output is returned": {
			Read: func(path string) (string, error) {
				if path != "trunc/1" {
					return "", assert.AnError
				}

				return "payload", nil
			},
			Args:   `{"file_path":"trunc/1"}`,
			Result: "payload",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := testDeps(nil, nil, nil)
			d.offload = &offloadReaderMock{read: c.Read}

			res, err := readToolOutput{}.Execute(testInput(d, NameReadToolOutput, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
