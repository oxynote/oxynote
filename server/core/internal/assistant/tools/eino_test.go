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
	// it in context, so it stays under about 7k tokens.
	assert.Less(t, len(data), 28000, "the tool list has grown past its budget")

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

	cc := map[string]struct {
		Tool                   Tool
		Args                   string
		FetchDocumentTreeCalls int
		Result                 string
		Err                    error
		ErrPrefix              string
	}{
		// the arguments never reach a tool that cannot read them, and
		// the failure names the tool.
		"Malformed arguments": {
			Tool:      getDocument{},
			Args:      `{`,
			Err:       assert.AnError,
			ErrPrefix: "get_document: invalid input: ",
		},
		// the tool's own output comes back as it is.
		"Successful run": {
			Tool:                   listDocuments{},
			Args:                   `{}`,
			FetchDocumentTreeCalls: 1,
			Result:                 `{"documents":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{}

			out, err := newEinoTool(c.Tool, testDeps(db, nil, nil)).
				Run(context.Background(), json.RawMessage(c.Args))
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
		Tool                   Tool
		QueryRange             func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error)
		Args                   string
		FetchDocumentTreeCalls int
		QueryCalls             int
		Result                 string
		Failure                string
	}{
		// the tool is handed an input carrying this call's args.
		"Successful run": {
			Tool:                   listDocuments{},
			Args:                   `{}`,
			FetchDocumentTreeCalls: 1,
			Result:                 `{"documents":[]}`,
		},
		// a failure comes back as the call's result rather than as an
		// error: the framework would end the whole turn on an error, and
		// arguments the tool cannot read are the model's to fix.
		"Malformed arguments": {
			Tool:    getDocument{},
			Args:    `{`,
			Failure: "invalid input",
		},
		// the same holds for a data source that rejected the model's
		// query.
		"Error returned by Prometheus.QueryRange": {
			Tool: queryDataSource{},
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
			d := dataSourceDeps(t, datasource.TypePrometheus, runner)
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
		Tool                         Tool
		Args                         string
		FetchDocumentByBranchIDCalls int
		Result                       string
	}{
		"Read announced by its own title": {
			Tool:                         getDocument{},
			Args:                         `{` + targetArgs(_stubMainBranchID) + `}`,
			FetchDocumentByBranchIDCalls: 1,
			Result:                       "Reading Runbook",
		},
		"Read without a title runs silently": {
			Tool: listDocuments{},
			Args: `{}`,
		},
		"Write announced by its summary": {
			Tool:                         deleteBlock{},
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			FetchDocumentByBranchIDCalls: 1,
			Result:                       "Delete a block in Runbook",
		},
		"Write with malformed arguments runs silently": {
			Tool: deleteBlock{},
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
		Tool                   Tool
		Args                   string
		FetchDocumentCalls     int
		FetchDocumentTreeCalls int
		Result                 ActionSummary
		Err                    error
	}{
		// a read proposes nothing, so it has nothing to describe.
		"Read proposes no change": {
			Tool: getDocument{},
			Args: `{}`,
			Err:  assert.AnError,
		},
		// arguments the tool cannot read are refused rather than
		// described: the same payload would fail on resume anyway.
		"Missing document id": {
			Tool: deleteDocument{},
			Args: `{"document_id":null}`,
			Err:  fmt.Errorf("%s: %w", NameDeleteDocument, errors.New("document_id is required")),
		},
		"Write describes its pending change": {
			Tool:                   deleteDocument{},
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

func Test_readToolOutputArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args readToolOutputArgs
		Err  error
	}{
		"File path is required": {
			Args: readToolOutputArgs{},
			Err:  errors.New("file_path is required"),
		},
		"Valid arguments": {
			Args: readToolOutputArgs{FilePath: "p"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
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
		Read      func(path string) (string, error)
		Args      string
		ReadCalls int
		Result    string
		Err       error
	}{
		"Malformed arguments": {
			Args: `{`,
			Err:  assert.AnError,
		},
		"Missing path": {
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Error returned by OffloadReader.Read": {
			Read: func(string) (string, error) {
				return "", errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`)
			},
			Args:      `{"file_path":"trunc/1"}`,
			ReadCalls: 1,
			Err:       errors.New(`no stored output at "trunc/1"; it has expired, so re-run the tool`),
		},
		"Stored output is returned": {
			Read: func(path string) (string, error) {
				if path != "trunc/1" {
					return "", assert.AnError
				}

				return "payload", nil
			},
			Args:      `{"file_path":"trunc/1"}`,
			ReadCalls: 1,
			Result:    "payload",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the offload stub records no calls, so they are counted here.
			var reads int

			d := testDeps(nil, nil, nil)
			d.offload = &offloadReaderMock{read: func(path string) (string, error) {
				reads++

				if c.Read == nil {
					return "", nil
				}

				return c.Read(path)
			}}

			res, err := readToolOutput{}.Execute(d.newInput(context.Background(), NameReadToolOutput, json.RawMessage(c.Args)))
			assert.Equal(t, c.ReadCalls, reads)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
