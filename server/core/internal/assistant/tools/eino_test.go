package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func Test_readToolOutput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool readToolOutput
		Err  error
	}{
		"File path is required": {
			Tool: readToolOutput{},
			Err:  errors.New("file_path is required"),
		},
		"Valid arguments": {
			Tool: readToolOutput{FilePath: "p"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_readToolOutput_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.ReadToolOutput": {
			Input: &InputMock{
				ReadToolOutputFunc: func(string) (string, error) {
					return "", errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`)
				},
			},
			Err: errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`),
		},
		// the stored output is already serialised, so it comes back as a
		// string the adapter passes through.
		"Stored output is returned": {
			Input: &InputMock{
				ReadToolOutputFunc: func(string) (string, error) {
					return "payload", nil
				},
			},
			Result: "payload",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := readToolOutput{FilePath: "trunc/1"}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.ReadToolOutputCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "trunc/1", ff[0].Path)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_einoTool_Info(t *testing.T) {
	t.Parallel()

	info, err := newEinoTool(register[getDocument](), testDeps(nil, nil, nil)).Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, string(NameGetDocument), info.Name)
	assert.NotEmpty(t, info.Desc)
	require.NotNil(t, info.ParamsOneOf)
}

func Test_einoTool_Run(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool                   registration
		Args                   string
		FetchDocumentTreeCalls int
		Result                 string
		Err                    error
		ErrPrefix              string
	}{
		// the arguments never reach a tool that cannot read them, and
		// the failure names the tool.
		"Malformed arguments": {
			Tool:      register[getDocument](),
			Args:      `{`,
			Err:       assert.AnError,
			ErrPrefix: "get_document: invalid input: ",
		},
		// a stored result is serialised already, so it comes back as it
		// was stored rather than as a JSON string.
		"Serialised result is passed through": {
			Tool:   register[readToolOutput](),
			Args:   `{"file_path":"trunc/1"}`,
			Result: `{"stored":true}`,
		},
		// the tool's own output comes back as it is.
		"Successful run": {
			Tool:                   register[listDocuments](),
			Args:                   `{}`,
			FetchDocumentTreeCalls: 1,
			Result:                 `{"documents":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{}
			d := testDeps(db, nil, nil)
			d.offload = &offloadReaderMock{read: func(string) (string, error) { return `{"stored":true}`, nil }}

			out, err := newEinoTool(c.Tool, d).Run(context.Background(), json.RawMessage(c.Args))

			assert.Empty(t, db.FetchDocumentByBranchIDCalls())
			assert.Empty(t, db.FetchDocumentBranchesCalls())
			assert.Empty(t, db.FetchBranchTagsCalls())
			assert.Len(t, db.FetchDocumentTreeCalls(), c.FetchDocumentTreeCalls)
			assert.Empty(t, db.FetchDocumentTreeByDocumentParentIDCalls())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.True(t, strings.HasPrefix(err.Error(), c.ErrPrefix))

				return
			}

			assert.JSONEq(t, c.Result, out)
		})
	}
}

func Test_einoTool_InvokableRun(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool                   registration
		QueryRange             func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error)
		Args                   string
		FetchDocumentTreeCalls int
		QueryCalls             int
		Result                 string
		Failure                string
	}{
		// the tool is handed an input carrying this call's args.
		"Successful run": {
			Tool:                   register[listDocuments](),
			Args:                   `{}`,
			FetchDocumentTreeCalls: 1,
			Result:                 `{"documents":[]}`,
		},
		// a failure comes back as the call's result rather than as an
		// error: the framework would end the whole turn on an error, and
		// arguments the tool cannot read are the model's to fix.
		"Malformed arguments": {
			Tool:    register[getDocument](),
			Args:    `{`,
			Failure: "invalid input",
		},
		// the same holds for a data source that rejected the model's
		// query.
		"Error returned by Prometheus.QueryRange": {
			Tool: register[queryDataSource](),
			QueryRange: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				return nil, errors.New("parse error: unexpected character")
			},
			Args:       `{"data_source_id":"` + _testDataSourceID.String() + `","query":"up)"}`,
			QueryCalls: 1,
			Failure:    "parse error",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			prom := &datasourceMock.Prometheus{QueryRangeFunc: c.QueryRange}
			runner := &datasourceMock.Runner{
				TypeFunc: func() datasource.Type { return datasource.TypePrometheus },
				PrometheusFunc: func(context.Context) (datasource.Prometheus, error) {
					return prom, nil
				},
			}
			d := dataSourceDeps(t, runner)
			db := d.db.(*DBMock)

			res, err := newEinoTool(c.Tool, d).InvokableRun(context.Background(), c.Args)
			require.NoError(t, err)

			assert.Len(t, db.FetchDocumentTreeCalls(), c.FetchDocumentTreeCalls)
			assert.Empty(t, db.FetchDocumentTreeByDocumentParentIDCalls())
			assert.Empty(t, db.FetchDocumentByBranchIDCalls())
			assert.Empty(t, db.FetchDocumentBranchesCalls())
			assert.Empty(t, db.FetchBranchTagsCalls())
			assert.Len(t, db.FetchDataSourceCalls(), c.QueryCalls)
			assert.Len(t, d.runners.(*DataSourceRunnersMock).RunnerCalls(), c.QueryCalls)
			assert.Empty(t, d.variableMan.(*VariableManagerMock).FetchTreeCalls())
			assert.Len(t, runner.TypeCalls(), c.QueryCalls)
			assert.Len(t, runner.PrometheusCalls(), c.QueryCalls)
			assert.Empty(t, runner.PostgreSQLCalls())
			assert.Empty(t, runner.MySQLCalls())
			assert.Len(t, prom.QueryRangeCalls(), c.QueryCalls)

			if c.Failure != "" {
				assert.Contains(t, res, c.Failure)

				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_einoTool_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool                         registration
		Args                         string
		FetchDocumentByBranchIDCalls int
		Result                       string
	}{
		"Read announced by its own title": {
			Tool:                         register[getDocument](),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `}`,
			FetchDocumentByBranchIDCalls: 1,
			Result:                       "Reading Runbook",
		},
		"Read without a title runs silently": {
			Tool: register[listDocuments](),
			Args: `{}`,
		},
		"Write announced by its summary": {
			Tool:                         register[deleteBlock](),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			FetchDocumentByBranchIDCalls: 1,
			Result:                       "Delete a block in Runbook",
		},
		"Write with malformed arguments runs silently": {
			Tool: register[deleteBlock](),
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubContentDB(nil)

			got := newEinoTool(c.Tool, testDeps(db, nil, nil)).Title(context.Background(), json.RawMessage(c.Args))
			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, db.FetchDocumentBranchesCalls())
			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_einoTool_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool                   registration
		Args                   string
		FetchDocumentCalls     int
		FetchDocumentTreeCalls int
		Result                 ActionSummary
		Err                    error
	}{
		// a read proposes nothing, so it has nothing to describe.
		"Read proposes no change": {
			Tool: register[getDocument](),
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  errors.New("get_document: proposes no change to describe"),
		},
		// arguments the tool cannot read are refused rather than
		// described: the same payload would fail on resume anyway.
		"Missing document id": {
			Tool: register[deleteDocument](),
			Args: `{"document_id":null}`,
			Err:  fmt.Errorf("%s: %w", NameDeleteDocument, errors.New("document_id is required")),
		},
		"Write describes its pending change": {
			Tool:                   register[deleteDocument](),
			Args:                   `{"document_id":"` + _testDocID.String() + `"}`,
			FetchDocumentCalls:     1,
			FetchDocumentTreeCalls: 1,
			Result: ActionSummary{
				Tool:         NameDeleteDocument,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Delete Runbook",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubDocumentDB()

			got, err := newEinoTool(c.Tool, testDeps(db, nil, nil)).Summary(context.Background(), json.RawMessage(c.Args))
			assert.Len(t, db.FetchDocumentCalls(), c.FetchDocumentCalls)
			assert.Len(t, db.FetchDocumentTreeCalls(), c.FetchDocumentTreeCalls)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_newEinoTool(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	et := newEinoTool(register[getDocument](), d)
	require.NotNil(t, et)

	assert.Equal(t, getDocument{}.Info(), et.reg.info)
	assert.Same(t, d, et.deps)

	// the description is resolved once, because it never varies by call.
	assert.Equal(t, NameGetDocument, et.info.Name)
}

// Test_Info_toEino pins every tool's description and schema to the
// golden file, since a dropped field or a cut description breaks the
// assistant without a compile error.
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
	// it in context, so it stays under about 7k tokens.
	assert.Less(t, len(data), 31000, "the tool list has grown past its budget")

	// a description change is meant to be reviewed as a golden diff, so
	// the file is rewritten on request rather than by hand.
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile("testdata/tool_schemas.golden", data, 0o600))

		return
	}

	assert.JSONEq(t, string(want), string(data))
}

func Test_encodeResult(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result string
		Err    error
	}{
		"Value JSON cannot hold": {
			Value: make(chan int),
			Err:   assert.AnError,
		},
		// markup stays as it is, since escaped it costs the model six
		// characters a bracket.
		"Markup is not escaped": {
			Value:  map[string]any{"content": "<p>a & b</p>"},
			Result: `{"content":"<p>a & b</p>"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := encodeResult(c.Value)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
