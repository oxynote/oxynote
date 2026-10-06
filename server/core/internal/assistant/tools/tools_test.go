package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allToolNames is every tool the assistant offers, in the order the
// registry builds them.
func allToolNames() []Name {
	return []Name{
		NameListDocuments,
		NameGetDocument,
		NameListTags,
		NameListHooks,
		NameSearchDocuments,
		NameListDataSources,
		NameGetDataSourceMetadata,
		NameListPrometheusLabels,
		NameListPrometheusSeries,
		NameQueryDataSource,
		NameCreateDocument,
		NameDeleteDocument,
		NameUpdateDocument,
		NameInsertBlocks,
		NameReplaceBlocks,
		NameDeleteBlock,
		NameMoveBlock,
		NameCreateTag,
		NameUpdateTag,
		NameDeleteTag,
		NameSetTagAssignment,
		NameCreateHook,
		NameUpdateHook,
		NameDeleteHook,
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
		tr := tl.Info().Traits

		// a write has to describe what it proposes, and nothing else
		// has anything to propose: this asks what a compiler cannot,
		// that every write's summary is actually there.
		s, summarizes := tl.(summarizer)
		require.Equal(t, tr.Write, summarizes, "%s declares Write=%v but summarizer=%v", name, tr.Write, summarizes)

		if summarizes {
			sum, serr := s.Summary(testInput(testDeps(stubContentDB(nil), nil, nil), name, requiredArgs(t, name)))
			require.NoError(t, serr)
			assert.NotEmpty(t, sum.Summary, "%s has an empty summary", name)
		}

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

	destructive := []Name{NameDeleteDocument, NameDeleteBlock, NameDeleteTag, NameDeleteHook}

	for i, e := range entries {
		assert.Equal(t, allToolNames()[i], e.Info.Name)

		// every model-facing string follows the rules a description keeps
		// on both surfaces: it says nothing about a confirmation flow,
		// which only the chat surface has, every argument is described,
		// and the text carries the house style (no em dash, British
		// spelling), since the model copies the punctuation it is shown.
		texts := map[string]string{"description": e.Info.Description}

		var walk func(props map[string]any, prefix string)

		walk = func(props map[string]any, prefix string) {
			for name, raw := range props {
				prop, ok := raw.(map[string]any)
				if !ok {
					continue
				}

				desc, _ := prop["description"].(string)
				texts[prefix+name] = desc

				if nested, ok := prop["properties"].(map[string]any); ok {
					walk(nested, prefix+name+".")
				}

				if items, ok := prop["items"].(map[string]any); ok {
					if nested, ok := items["properties"].(map[string]any); ok {
						walk(nested, prefix+name+"[].")
					}
				}
			}
		}

		walk(e.Info.Properties, "")

		for path, text := range texts {
			assert.NotEmpty(t, text, "%s: %s has no description", e.Info.Name, path)

			for _, banned := range []string{"confirm", "approv", "\u2014", "organization"} {
				assert.NotContains(t, strings.ToLower(text), banned, "%s: %s", e.Info.Name, path)
			}
		}

		// the entry carries the tool without its confirmation gate,
		// while the registry keeps the gated one for the chat loop.
		_, gated := e.Tool.(*confirming)
		assert.False(t, gated, "%s entry must be ungated", e.Info.Name)

		tr := e.Info.Traits
		assert.Equal(t, slices.Contains(s.WriteNames(), string(e.Info.Name)), tr.Write, "%s write flag", e.Info.Name)
		assert.Equal(t, slices.Contains(destructive, e.Info.Name), tr.Destructive, "%s destructive flag", e.Info.Name)
		assert.Equal(t, e.Info.Name == NameReadToolOutput, tr.Internal, "%s internal flag", e.Info.Name)
	}

	// mutating the returned slice must not affect the registry.
	entries[0].Info.Name = "clobbered"
	assert.Equal(t, allToolNames()[0], s.Entries()[0].Info.Name)
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
			Name: NameGetDocument,
		},
		"Unknown name": {
			Set:  New(testDeps(nil, nil, nil)),
			Name: "not_a_tool",
		},
		"Registered read tool": {
			Set:    New(testDeps(nil, nil, nil)),
			Name:   NameGetDocument,
			Result: NameGetDocument,
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

			assert.Equal(t, c.Result, e.Info.Name)

			_, gated := e.Tool.(*confirming)
			assert.False(t, gated, "%s entry must be ungated", e.Info.Name)
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
		string(NameUpdateDocument),
		string(NameInsertBlocks),
		string(NameReplaceBlocks),
		string(NameDeleteBlock),
		string(NameMoveBlock),
		string(NameCreateTag),
		string(NameUpdateTag),
		string(NameDeleteTag),
		string(NameSetTagAssignment),
		string(NameCreateHook),
		string(NameUpdateHook),
		string(NameDeleteHook),
	}, got)

	// mutating the returned slice must not affect the registry.
	got[0] = "clobbered"

	assert.NotContains(t, s.WriteNames(), "clobbered")
}

func Test_Set_Label(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
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
			Name:   NameGetDocument,
			Args:   `{` + targetArgs(_stubMainBranchID) + `}`,
			Result: "Reading Runbook",
		},
		// a write is announced by the summary its approval card shows.
		"Write is announced by its summary": {
			DB:     stubContentDB(nil),
			Name:   NameReplaceBlocks,
			Args:   `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p>t</p>"}`,
			Result: "Replace a block in Runbook with 1 block",
		},
		"Tag write is announced by its summary": {
			Name:   NameDeleteTag,
			Args:   `{"tag_id":"` + _testTagID.String() + `"}`,
			Result: "Delete tag Production and take it off the 1 page carrying it",
		},
		"Unresolvable document is not announced": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Name: NameGetDocument,
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
		},
		"Malformed arguments are not announced": {
			// the call is about to fail on these same arguments, and
			// that failure is the tool's to report.
			Name: NameGetDocument,
			Args: `{`,
		},
		"Missing arguments are not announced": {
			Name: NameGetDocument,
			Args: `{}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := c.DB
			if db == nil {
				db = stubDocumentDB()
			}

			s := New(testDeps(db, nil, nil))

			got := s.Label(context.Background(), c.Name, json.RawMessage(c.Args))
			assert.Equal(t, c.Result, got)
		})
	}
}
