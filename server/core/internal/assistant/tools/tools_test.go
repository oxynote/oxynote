package tools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allToolNames is every tool the assistant offers, in the order the
// registry builds them.
func allToolNames() []Name {
	return []Name{
		NameListDocuments,
		NameGetDocument,
		NameReadDocumentSummary,
		NameReadBlock,
		NameSearchDocuments,
		NameListDataSources,
		NameGetPrometheusMetadata,
		NameListPrometheusLabelNames,
		NameListPrometheusLabelValues,
		NameListPrometheusSeries,
		NameQueryPrometheus,
		NameGetSQLMetadata,
		NameGetSQLQueryLabels,
		NameQuerySQL,
		NameCreateDocument,
		NameDeleteDocument,
		NameRenameDocument,
		NameSetDocumentIcon,
		NameMoveDocument,
		NameInsertBlock,
		NameAppendBlock,
		NamePrependBlock,
		NameReplaceBlock,
		NameUpdateBlockText,
		NameUpdateBlockAttrs,
		NameDeleteBlock,
		NameReadToolOutput,
	}
}

// adapter returns the eino adapter underneath a registry entry, gated
// or not, so a test can reach the tool the registry wrapped.
func adapter(t *testing.T, it registryTool) *einoTool {
	t.Helper()

	if c, ok := it.(*confirming); ok {
		return c.einoTool
	}

	et, ok := it.(*einoTool)
	require.True(t, ok, "the registry holds an unexpected %T", it)

	return et
}

func Test_New(t *testing.T) {
	t.Parallel()

	s := New(testDeps(nil, nil, nil))
	require.NotNil(t, s)

	// every tool the model is told about has to be reachable by the
	// name it was told, or the agent's dispatch finds nothing.
	require.Len(t, s.tools, len(allToolNames()))

	for _, name := range allToolNames() {
		it, ok := s.tools[name]
		require.True(t, ok, "%s is missing from the registry", name)

		info, err := it.Info(context.Background())
		require.NoError(t, err)
		assert.Equal(t, string(name), info.Name)

		tl := adapter(t, it).tl
		tr := tl.Traits()

		// a write has to describe what it proposes, and nothing else
		// has anything to propose. Every tool satisfies the interface,
		// so this asks what a compiler cannot: that the summary is
		// actually there.
		sum, serr := tl.Summary(testInput(testDeps(nil, nil, nil), name, `{}`))
		require.NoError(t, serr)
		assert.Equal(t, tr.Write, sum.Summary != "",
			"%s declares Write=%v but Summary=%q", name, tr.Write, sum.Summary)

		// a destructive tool is a write first; nothing else can be
		// destructive.
		if tr.Destructive {
			assert.True(t, tr.Write, "%s is destructive without being a write", name)
		}

		// the gate is applied here rather than by each tool, so a write
		// cannot declare itself one and quietly skip the prompt.
		c, gated := it.(*confirming)
		require.Equal(t, tr.Write, gated, "%s is gated=%v but declares Write=%v", name, gated, tr.Write)

		if !gated {
			continue
		}

		// the gate carries the tool's own destructive trait: approving
		// a batch of text edits is not consent to delete.
		assert.Equal(t, tr.Destructive, c.destructive, "%s gate destructive flag", name)
	}
}

func Test_Set_Tools(t *testing.T) {
	t.Parallel()

	ts := New(testDeps(nil, nil, nil)).Tools()
	require.Len(t, ts, len(allToolNames()))

	got := make([]Name, 0, len(ts))

	for _, bt := range ts {
		info, err := bt.Info(context.Background())
		require.NoError(t, err)

		got = append(got, Name(info.Name))
	}

	// the order is the registration order, every session: a shuffled
	// tool list would defeat provider prompt caching.
	assert.Equal(t, allToolNames(), got)
}

func Test_Set_Entries(t *testing.T) {
	t.Parallel()

	s := New(testDeps(nil, nil, nil))

	entries := s.Entries()
	require.Len(t, entries, len(allToolNames()))

	destructive := []Name{NameDeleteDocument, NameDeleteBlock}

	for i, e := range entries {
		assert.Equal(t, allToolNames()[i], e.Name)

		// the entry carries the tool's own description, so a surface
		// outside this package never has to ask the tool for it.
		assert.Equal(t, e.Name, e.Info.Name)
		assert.NotEmpty(t, e.Info.Description)

		// the entry carries the tool without its confirmation gate,
		// while the registry keeps the gated one for the chat loop.
		_, gated := e.Tool.(*confirming)
		assert.False(t, gated, "%s entry must be ungated", e.Name)

		assert.Equal(t, slices.Contains(s.WriteNames(), string(e.Name)), e.Write, "%s write flag", e.Name)
		assert.Equal(t, slices.Contains(destructive, e.Name), e.Destructive, "%s destructive flag", e.Name)
		assert.Equal(t, e.Name == NameReadToolOutput, e.Internal, "%s internal flag", e.Name)
	}

	// mutating the returned slice must not affect the registry.
	entries[0].Name = "clobbered"
	assert.Equal(t, allToolNames()[0], s.Entries()[0].Name)
}

func Test_Set_Entry(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Set    *Set
		Name   Name
		Result Name
		Found  bool
	}{
		"Empty registry": {
			Set:  &Set{},
			Name: NameReadDocumentSummary,
		},
		"Unknown name": {
			Set:  New(testDeps(nil, nil, nil)),
			Name: "not_a_tool",
		},
		"Registered read tool": {
			Set:    New(testDeps(nil, nil, nil)),
			Name:   NameReadDocumentSummary,
			Result: NameReadDocumentSummary,
			Found:  true,
		},
		"Registered write tool comes back ungated": {
			Set:    New(testDeps(nil, nil, nil)),
			Name:   NameDeleteDocument,
			Result: NameDeleteDocument,
			Found:  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e, ok := c.Set.Entry(c.Name)
			assert.Equal(t, c.Found, ok)

			if !c.Found {
				assert.Equal(t, Entry{}, e)

				return
			}

			assert.Equal(t, c.Result, e.Name)

			_, gated := e.Tool.(*confirming)
			assert.False(t, gated, "%s entry must be ungated", e.Name)
		})
	}
}

func Test_Set_WriteNames(t *testing.T) {
	t.Parallel()

	s := New(testDeps(nil, nil, nil))

	got := s.WriteNames()

	// exactly the tools gated behind confirmation, so the context
	// middlewares and the gate cannot drift apart.
	assert.ElementsMatch(t, []string{
		string(NameCreateDocument),
		string(NameDeleteDocument),
		string(NameRenameDocument),
		string(NameSetDocumentIcon),
		string(NameMoveDocument),
		string(NameInsertBlock),
		string(NameAppendBlock),
		string(NamePrependBlock),
		string(NameReplaceBlock),
		string(NameUpdateBlockText),
		string(NameUpdateBlockAttrs),
		string(NameDeleteBlock),
	}, got)

	// mutating the returned slice must not affect the registry.
	got[0] = "clobbered"

	assert.NotContains(t, s.WriteNames(), "clobbered")
}

func Test_Set_Label(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name   Name
		Args   string
		Result string
	}{
		"Unknown name is not an error": {
			Name: "not_a_tool",
			Args: `{}`,
		},
		"Tool that declines to announce itself": {
			Name: NameListDocuments,
			Args: `{}`,
		},
		"Read names the document": {
			Name:   NameReadDocumentSummary,
			Args:   `{"document_id":"` + _testDocID + `"}`,
			Result: "Reading Runbook",
		},
		"Write names the document": {
			Name:   NameUpdateBlockText,
			Args:   `{"document_id":"` + _testDocID + `"}`,
			Result: "Updating Runbook",
		},
		"Unresolvable document falls back": {
			Name:   NameReadDocumentSummary,
			Args:   `{}`,
			Result: "Reading document",
		},
		"Malformed arguments are not announced": {
			// the call is about to fail on these same arguments, and
			// that failure is the tool's to report.
			Name: NameReadDocumentSummary,
			Args: `{`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			s := New(testDeps(stubDocumentDB(), nil, nil))

			got := s.Label(context.Background(), c.Name, json.RawMessage(c.Args))
			assert.Equal(t, c.Result, got)
		})
	}
}
