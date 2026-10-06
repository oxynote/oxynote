package search

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _longText holds a match well past the fragment size on both sides.
const _longText = "the migration guide starts with the prerequisites and then walks through every step of the upgrade, " +
	"including the zebra pattern used for rolling out schema changes without downtime, " +
	"before closing with the rollback procedure and the checks to run afterwards"

// seedIndex fills an index with the entries the ranking cases rely on:
// a default branch and a fork of one document in org-1, and one document
// in org-2. It returns the index and the default branch's scope.
func seedIndex(t *testing.T) (*Index, Scope) {
	t.Helper()

	idx := openIndex(t, indexPath(t), &stubSource{})
	t.Cleanup(func() { require.NoError(t, idx.Close()) })

	main := Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	fork := main
	fork.BranchID = xid.New()
	fork.BranchName = "draft"
	fork.BranchDefault = false

	other := Scope{
		OrganizationID: "org-2",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	require.NoError(t, idx.ReplaceBranch(context.Background(), main.BranchID, map[string]Block{
		"docname": main.Block("docname", "document", "Release Checklist"),
		"h1":      main.Block("h1", "heading", "Release Checklist"),
		"p1":      main.Block("p1", "paragraph", "Release Checklist"),
		"p2":      main.Block("p2", "paragraph", "we deploy the service to production every friday"),
		"p3":      main.Block("p3", "paragraph", "production traffic hits the service after we deploy on monday"),
		"p4":      main.Block("p4", "paragraph", "alpha beta"),
		"p5":      main.Block("p5", "paragraph", "alpha only"),
		"p6": func() Block {
			b := main.Block("p6", "codeBlock", "kubernetes clusters")
			b.Attrs = map[string]string{"language": "yaml"}

			return b
		}(),
		"p7":  main.Block("p7", "paragraph", "the endpoint answers 404 not found"),
		"p8":  main.Block("p8", "paragraph", "run an a11y audit before launch"),
		"p9":  main.Block("p9", "paragraph", "accessibility matters"),
		"p10": main.Block("p10", "paragraph", _longText),
	}))

	require.NoError(t, idx.ReplaceBranch(context.Background(), fork.BranchID, map[string]Block{
		"p1": fork.Block("p1", "paragraph", "Release Checklist"),
	}))

	require.NoError(t, idx.ReplaceBranch(context.Background(), other.BranchID, map[string]Block{
		"p1": other.Block("p1", "paragraph", "Release Checklist"),
	}))

	return idx, main
}

func Test_Index_SearchDocuments(t *testing.T) {
	t.Parallel()

	idx, main := seedIndex(t)

	data, err := idx.SearchDocuments(context.Background(), "org-1", "kubernetes")
	require.NoError(t, err)

	exp, err := json.Marshal([]Block{
		{
			ID:             main.BranchID.String() + "-p6",
			OrganizationID: "org-1",
			DocumentID:     main.DocumentID,
			BranchID:       main.BranchID,
			BranchName:     "main",
			BranchDefault:  true,
			Type:           "codeBlock",
			Text:           "<mark>kubernetes</mark> clusters",
			Attrs:          map[string]string{"language": "yaml"},
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(exp), string(data))

	// error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	data, err = idx.SearchDocuments(ctx, "org-1", "kubernetes")
	require.Error(t, err)
	assert.Nil(t, data)
}

func Test_Index_SearchDocumentBlocks(t *testing.T) {
	t.Parallel()

	idx, main := seedIndex(t)

	blocks, err := idx.SearchDocumentBlocks(context.Background(), "org-1", "kubernetes", 10)
	require.NoError(t, err)
	assert.Equal(t, []Block{
		{
			ID:             main.BranchID.String() + "-p6",
			OrganizationID: "org-1",
			DocumentID:     main.DocumentID,
			BranchID:       main.BranchID,
			BranchName:     "main",
			BranchDefault:  true,
			Type:           "codeBlock",
			Text:           "kubernetes clusters",
			Attrs:          map[string]string{"language": "yaml"},
		},
	}, blocks)

	// the limit caps the hits.
	blocks, err = idx.SearchDocumentBlocks(context.Background(), "org-1", "release checklist", 2)
	require.NoError(t, err)
	assert.Len(t, blocks, 2)

	// error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	blocks, err = idx.SearchDocumentBlocks(ctx, "org-1", "kubernetes", 10)
	require.Error(t, err)
	assert.Nil(t, blocks)
}

func Test_ValidateQuery(t *testing.T) {
	cc := map[string]struct {
		Query string
		Err   error
	}{
		"Empty query":      {Err: ErrInvalidQuery},
		"Overlong query":   {Query: strings.Repeat("a", MaxQueryLength+1), Err: ErrInvalidQuery},
		"Query at the cap": {Query: strings.Repeat("é", MaxQueryLength)},
		"Plain query":      {Query: "deploy"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, ValidateQuery(c.Query))
		})
	}
}

func Test_Index_search(t *testing.T) {
	t.Parallel()

	idx, main := seedIndex(t)

	cc := map[string]struct {
		OrganizationID string
		Query          string
		Limit          int
		Highlight      bool
		Check          func(t *testing.T, blocks []Block)
		Err            error
	}{
		"Overlong query": {
			OrganizationID: "org-1",
			Query:          strings.Repeat("a", MaxQueryLength+1),
			Limit:          10,
			Err:            ErrInvalidQuery,
		},
		"Stemming matches another form of the word": {
			OrganizationID: "org-1",
			Query:          "deploying",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.ElementsMatch(t, []string{
					"we deploy the service to production every friday",
					"production traffic hits the service after we deploy on monday",
				}, blockTexts(blocks))
			},
		},
		"Stop words are searchable": {
			OrganizationID: "org-1",
			Query:          "not found",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				require.NotEmpty(t, blocks)
				assert.Equal(t, "the endpoint answers 404 not found", blocks[0].Text)
			},
		},
		"Typo within the automatic fuzziness matches": {
			OrganizationID: "org-1",
			Query:          "kubernets",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.Equal(t, []string{"kubernetes clusters"}, blockTexts(blocks))
			},
		},
		"Last term matches as a prefix": {
			OrganizationID: "org-1",
			Query:          "kuber",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.Equal(t, []string{"kubernetes clusters"}, blockTexts(blocks))
			},
		},
		"Phrase ranks above scattered words": {
			OrganizationID: "org-1",
			Query:          "deploy the service",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				// "the" alone matches other entries too; they rank below both.
				require.GreaterOrEqual(t, len(blocks), 2)
				assert.Equal(t, "we deploy the service to production every friday", blocks[0].Text)
				assert.Equal(t, "production traffic hits the service after we deploy on monday", blocks[1].Text)
			},
		},
		"All terms rank above a subset, which is still returned": {
			OrganizationID: "org-1",
			Query:          "alpha beta",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 2)
				assert.Equal(t, "alpha beta", blocks[0].Text)
				assert.Equal(t, "alpha only", blocks[1].Text)
			},
		},
		"Synonym expands the query": {
			OrganizationID: "org-1",
			Query:          "accessibility",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.ElementsMatch(t, []string{
					"run an a11y audit before launch",
					"accessibility matters",
				}, blockTexts(blocks))
			},
		},
		"Synonym expands the query in the other direction": {
			OrganizationID: "org-1",
			Query:          "a11y",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.ElementsMatch(t, []string{
					"run an a11y audit before launch",
					"accessibility matters",
				}, blockTexts(blocks))
			},
		},
		"Document name, heading and default branch rank first": {
			OrganizationID: "org-1",
			Query:          "release checklist",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 4)
				assert.Equal(t, "document", blocks[0].Type)
				assert.Equal(t, "heading", blocks[1].Type)
				assert.Equal(t, "paragraph", blocks[2].Type)
				assert.True(t, blocks[2].BranchDefault)
				assert.Equal(t, "paragraph", blocks[3].Type)
				assert.False(t, blocks[3].BranchDefault)
			},
		},
		"Entries of another organization are never returned": {
			OrganizationID: "org-2",
			Query:          "release checklist",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 1)
				assert.Equal(t, "org-2", blocks[0].OrganizationID)
			},
		},
		"Unknown organization finds nothing": {
			OrganizationID: "org-3",
			Query:          "release checklist",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.Empty(t, blocks)
			},
		},
		"Limit caps the hits": {
			OrganizationID: "org-1",
			Query:          "release checklist",
			Limit:          1,
			Check: func(t *testing.T, blocks []Block) {
				assert.Len(t, blocks, 1)
			},
		},
		"Highlight marks the match inside a fragment": {
			OrganizationID: "org-1",
			Query:          "zebra",
			Limit:          10,
			Highlight:      true,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 1)
				assert.Contains(t, blocks[0].Text, "<mark>zebra</mark>")
				// the fragment size counts text only; the mark tags come on top.
				assert.Less(t, len(blocks[0].Text), _fragmentSize+len("<mark></mark>")+10)
				assert.Equal(t, main.BranchID.String()+"-p10", blocks[0].ID)
			},
		},
		"Highlight keeps the full text of a synonym-only match": {
			OrganizationID: "org-1",
			Query:          "accessibility",
			Limit:          10,
			Highlight:      true,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 2)

				for _, b := range blocks {
					assert.NotEmpty(t, b.Text)
				}
			},
		},
		"Query missing one of three terms still matches": {
			OrganizationID: "org-1",
			Query:          "deploy service friday",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				require.Len(t, blocks, 2)
				assert.Equal(t, "we deploy the service to production every friday", blocks[0].Text)
				assert.Equal(t, "production traffic hits the service after we deploy on monday", blocks[1].Text)
			},
		},
		"Common word among unknown ones matches nothing": {
			OrganizationID: "org-1",
			Query:          "zz-no-such-release-anywhere",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.Empty(t, blocks)
			},
		},
		"Query without a single token still matches nothing": {
			OrganizationID: "org-1",
			Query:          "...",
			Limit:          10,
			Check: func(t *testing.T, blocks []Block) {
				assert.Empty(t, blocks)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			blocks, err := idx.search(context.Background(), c.OrganizationID, c.Query, c.Limit, c.Highlight)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, blocks)
		})
	}
}

// blockTexts returns the text of each block, in order.
func blockTexts(blocks []Block) []string {
	res := make([]string, 0, len(blocks))

	for _, b := range blocks {
		res = append(res, b.Text)
	}

	return res
}

func Test_Index_query(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, indexPath(t), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	// a query with terms: the organization filter and the text disjunction
	// are required, the boosts optional.
	bq, ok := idx.query("org-1", "release notes").(*query.BooleanQuery)
	require.True(t, ok)

	must, ok := bq.Must.(*query.ConjunctionQuery)
	require.True(t, ok)
	require.Len(t, must.Conjuncts, 2)

	org, ok := must.Conjuncts[0].(*query.TermQuery)
	require.True(t, ok)
	assert.Equal(t, "org-1", org.Term)
	assert.Equal(t, _fieldOrganizationID, org.FieldVal)

	text, ok := must.Conjuncts[1].(*query.DisjunctionQuery)
	require.True(t, ok)
	require.Len(t, text.Disjuncts, 2)

	phrase, ok := text.Disjuncts[0].(*query.PhraseQuery)
	require.True(t, ok)
	assert.Equal(t, []string{"releas", "note"}, phrase.Terms)

	terms, ok := text.Disjuncts[1].(*query.DisjunctionQuery)
	require.True(t, ok)
	assert.Len(t, terms.Disjuncts, 2)

	should, ok := bq.Should.(*query.DisjunctionQuery)
	require.True(t, ok)
	assert.Len(t, should.Disjuncts, 3)

	// a query the analyzer reduces to nothing matches nothing.
	bq, ok = idx.query("org-1", "...").(*query.BooleanQuery)
	require.True(t, ok)

	must, ok = bq.Must.(*query.ConjunctionQuery)
	require.True(t, ok)

	_, ok = must.Conjuncts[1].(*query.MatchNoneQuery)
	assert.True(t, ok)
}

func Test_Index_terms(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, indexPath(t), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	// the terms are the analyzer's: lowercased and stemmed.
	assert.Equal(t, []string{"deploy", "releas", "note"}, idx.terms("Deploying Release Notes"))
	assert.Empty(t, idx.terms("..."))

	// a query beyond the cap keeps its first terms only.
	long := strings.TrimSpace(strings.Repeat("word ", _maxQueryTerms+5))
	assert.Len(t, idx.terms(long), _maxQueryTerms)
}

func Test_termsQuery(t *testing.T) {
	t.Parallel()

	// one term: fuzzy or prefix, and it has to match.
	dq, ok := termsQuery([]string{"kuber"}).(*query.DisjunctionQuery)
	require.True(t, ok)
	require.Len(t, dq.Disjuncts, 1)
	assert.InDelta(t, 1, dq.Min, 0)

	last, ok := dq.Disjuncts[0].(*query.DisjunctionQuery)
	require.True(t, ok)
	require.Len(t, last.Disjuncts, 2)

	fuzzy, ok := last.Disjuncts[0].(*query.FuzzyQuery)
	require.True(t, ok)
	assert.Equal(t, "kuber", fuzzy.Term)

	prefix, ok := last.Disjuncts[1].(*query.PrefixQuery)
	require.True(t, ok)
	assert.Equal(t, "kuber", prefix.Prefix)

	// three terms: the first two fuzzy only, the last also a prefix, and
	// all but one have to match.
	dq, ok = termsQuery([]string{"deploy", "releas", "note"}).(*query.DisjunctionQuery)
	require.True(t, ok)
	require.Len(t, dq.Disjuncts, 3)
	assert.InDelta(t, 2, dq.Min, 0)

	fuzzy, ok = dq.Disjuncts[0].(*query.FuzzyQuery)
	require.True(t, ok)
	assert.Equal(t, "deploy", fuzzy.Term)

	last, ok = dq.Disjuncts[2].(*query.DisjunctionQuery)
	require.True(t, ok)

	prefix, ok = last.Disjuncts[1].(*query.PrefixQuery)
	require.True(t, ok)
	assert.Equal(t, "note", prefix.Prefix)
}

func Test_Index_decodeHit(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	fields := func(documentID, branchID string) map[string]any {
		return map[string]any{
			_fieldOrganizationID: "org-1",
			_fieldDocumentID:     documentID,
			_fieldBranchID:       branchID,
			_fieldBranchName:     "draft",
			_fieldBranchDefault:  false,
			_fieldType:           "paragraph",
			_fieldText:           "hello",
		}
	}

	block := Block{
		ID:             branchID.String() + "-p1",
		OrganizationID: "org-1",
		DocumentID:     documentID,
		BranchID:       branchID,
		BranchName:     "draft",
		BranchDefault:  false,
		Type:           "paragraph",
		Text:           "hello",
	}

	type tcase struct {
		Fields map[string]any
		Result Block
		Logged string
		Err    error
	}

	cc := map[string]tcase{
		"Invalid document id": {
			Fields: fields("nope", branchID.String()),
			Err:    assert.AnError,
		},
		"Invalid branch id": {
			Fields: fields(documentID.String(), "nope"),
			Err:    assert.AnError,
		},
		"Field of another type is logged and left empty": func() tcase {
			ff := fields(documentID.String(), branchID.String())
			ff[_fieldText] = 42

			res := block
			res.Text = ""

			return tcase{Fields: ff, Result: res, Logged: "search entry field is not a string"}
		}(),
		"Missing boolean is logged and left false": func() tcase {
			ff := fields(documentID.String(), branchID.String())
			delete(ff, _fieldBranchDefault)

			return tcase{Fields: ff, Result: block, Logged: "search entry field is not a boolean"}
		}(),
		"Successful decoding": {
			Fields: fields(documentID.String(), branchID.String()),
			Result: block,
		},
		"Successful decoding with attributes": func() tcase {
			ff := fields(documentID.String(), branchID.String())
			ff[_fieldAttrs+".language"] = "go"
			ff[_fieldAttrs+".dataSourceId"] = "ds1"

			res := block
			res.Attrs = map[string]string{"language": "go", "dataSourceId": "ds1"}

			return tcase{Fields: ff, Result: res}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			idx := &Index{log: slog.New(slog.NewTextHandler(&logs, nil))}

			res, err := idx.decodeHit(branchID.String()+"-p1", c.Fields)

			if c.Err != nil {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Result, res)

			if c.Logged == "" {
				assert.Empty(t, logs.String())
				return
			}

			assert.Contains(t, logs.String(), c.Logged)
		})
	}
}
