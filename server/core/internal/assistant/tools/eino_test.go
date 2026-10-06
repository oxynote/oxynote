package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
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

	// every session sends the whole tool list, and an MCP client keeps
	// it in context, so it stays under about 6k tokens.
	assert.Less(t, len(data), 22000, "the tool list has grown past its budget")

	// a description change is meant to be reviewed as a golden diff, so
	// the file is rewritten on request rather than by hand.
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile("testdata/tool_schemas.golden", data, 0o600))

		return
	}

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

func Test_einoTool_Run(t *testing.T) {
	t.Parallel()

	// error: the arguments never reach a tool that cannot read them, and
	// the failure names the tool.
	_, err := newEinoTool(getDocument{}, testDeps(nil, nil, nil)).
		Run(context.Background(), json.RawMessage(`{`))
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "get_document: invalid input: "))

	// success: the tool's own output comes back as it is
	out, err := newEinoTool(listDocuments{}, testDeps(nil, nil, nil)).
		Run(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"documents":[]}`, out)
}

func Test_einoTool_InvokableRun(t *testing.T) {
	t.Parallel()

	// the tool is handed an input carrying this call's args
	res, err := newEinoTool(listDocuments{}, testDeps(nil, nil, nil)).
		InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"documents":[]}`, res)

	// a failure comes back as the call's result rather than as an
	// error: the framework would end the whole turn on an error, and
	// arguments the tool cannot read are the model's to fix.
	res, err = newEinoTool(getDocument{}, testDeps(nil, nil, nil)).
		InvokableRun(context.Background(), `{`)
	require.NoError(t, err)
	assert.Contains(t, res, "invalid input")

	// the same holds for a data source that rejected the model's query
	failing := dataSourceDeps(t, datasource.TypePrometheus, prometheusRunner(&datasourceMock.Prometheus{
		QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
			return nil, errors.New("parse error: unexpected character")
		},
	}))

	args := `{"data_source_id":"` + _testDataSourceID.String() + `","query":"up)"}`

	res, err = newEinoTool(queryDataSource{}, failing).InvokableRun(context.Background(), args)
	require.NoError(t, err)
	assert.Contains(t, res, "parse error")
}

func Test_einoTool_Title(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	d := testDeps(stubContentDB(nil), nil, nil)

	// a read announces itself with its own title.
	assert.Equal(t, "Reading Runbook", newEinoTool(getDocument{}, d).Title(ctx, json.RawMessage(`{`+targetArgs(_stubMainBranchID)+`}`)))

	// a read without one runs silently.
	assert.Empty(t, newEinoTool(listDocuments{}, d).Title(ctx, json.RawMessage(`{}`)))

	// a write is announced by its summary, and by nothing when its
	// arguments cannot be read.
	write := newEinoTool(deleteBlock{}, d)
	assert.Equal(t, "Delete a block in Runbook", write.Title(ctx, json.RawMessage(`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a"}`)))
	assert.Empty(t, write.Title(ctx, json.RawMessage(`{`)))
}

func Test_einoTool_Summary(t *testing.T) {
	t.Parallel()

	// a write describes its pending change
	et := newEinoTool(deleteDocument{}, testDeps(stubDocumentDB(), nil, nil))

	got, err := et.Summary(context.Background(), json.RawMessage(`{"document_id":"`+_testDocID.String()+`"}`))
	require.NoError(t, err)
	assert.Equal(t, NameDeleteDocument, got.Tool)
	assert.Equal(t, "Delete Runbook", got.Summary)

	// arguments the tool cannot read are refused rather than described:
	// the same payload would fail on resume anyway.
	_, err = et.Summary(context.Background(), json.RawMessage(`{"document_id":null}`))
	assert.EqualError(t, err, "delete_document: document_id is required")

	// a read proposes nothing, so it has nothing to describe.
	_, err = newEinoTool(getDocument{}, testDeps(nil, nil, nil)).Summary(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
}

func Test_readToolOutputArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, readToolOutputArgs{FilePath: "p"}, map[string]Args{
		"file_path": readToolOutputArgs{},
	})
}

func Test_readToolOutput_Info(t *testing.T) {
	t.Parallel()

	info := readToolOutput{}.Info()

	// the paths it takes only exist inside a conversation, so it must
	// never be offered to a client that has none.
	assert.Equal(t, Traits{Internal: true}, info.Traits)

	// the tool is named for tool output, not files: sitting next to
	// get_document, a generic read_file would invite the wrong call.
	assert.Equal(t, NameReadToolOutput, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Contains(t, info.Properties, "file_path")
	assert.Equal(t, []string{"file_path"}, info.Required)
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
			Err:  assert.AnError,
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
