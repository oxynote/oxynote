package search

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource serves fixed branches, in branch id order, page by page.
type stubSource struct {
	docs  []document.Document
	err   error
	calls int
}

func (s *stubSource) FetchDocumentBranchesAfter(_ context.Context, after xid.ID, limit int) ([]document.Document, error) {
	s.calls++

	if s.err != nil {
		return nil, s.err
	}

	res := []document.Document{}

	for _, d := range s.docs {
		if d.BranchID.String() > after.String() && len(res) < limit {
			res = append(res, d)
		}
	}

	return res, nil
}

// stubDocument builds a default-branch document of org-1 with one
// paragraph. Ids come from xid.New, which increase within a process, so
// documents built in sequence are already in branch id order.
func stubDocument(name, text string) document.Document {
	return document.Document{
		ID:             xid.New(),
		OrganizationID: "org-1",
		BranchID:       xid.New(),
		BranchName:     document.DefaultBranch,
		DocumentName:   name,
		Content: document.RootBlock{
			Type: document.BlockNodeDoc,
			Content: []document.Block{
				{
					Type:  document.BlockNodeParagraph,
					Attrs: document.Attributes{"uid": "p1"},
					Content: []document.Block{
						{Type: document.BlockNodeText, Text: text},
					},
				},
			},
		},
		Default: true,
	}
}

// openIndex opens or builds an index at path from src.
func openIndex(t *testing.T, path string, src Source) *Index {
	t.Helper()

	idx, err := Open(context.Background(), slog.New(slog.DiscardHandler), path, src)
	require.NoError(t, err)
	require.NotNil(t, idx)

	return idx
}

// texts returns the text of every entry the query finds for org-1, in rank
// order.
func texts(t *testing.T, idx *Index, q string) []string {
	t.Helper()

	blocks, err := idx.SearchDocumentBlocks(context.Background(), "org-1", q, 50)
	require.NoError(t, err)

	return blockTexts(blocks)
}

func Test_Open(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Path   string
		Source *stubSource
		Err    error
		Check  func(t *testing.T, idx *Index, path string, src *stubSource)
	}

	cc := map[string]func(t *testing.T) tcase{
		"Empty path": func(*testing.T) tcase {
			return tcase{
				Path:   "",
				Source: &stubSource{},
				Err:    assert.AnError,
				Check: func(t *testing.T, _ *Index, _ string, src *stubSource) {
					assert.Equal(t, 0, src.calls)
				},
			}
		},
		"Error returned by Source.FetchDocumentBranchesAfter": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")

			return tcase{
				Path:   path,
				Source: &stubSource{err: assert.AnError},
				Err:    assert.AnError,
				Check: func(t *testing.T, _ *Index, path string, src *stubSource) {
					assert.Equal(t, 1, src.calls)

					_, err := os.Stat(path)
					assert.True(t, os.IsNotExist(err), "a failed build leaves no index behind")
				},
			}
		},
		"Missing index is built from the source": func(t *testing.T) tcase {
			return tcase{
				Path:   filepath.Join(t.TempDir(), "index"),
				Source: &stubSource{docs: []document.Document{stubDocument("Runbook", "restart the pods")}},
				Check: func(t *testing.T, idx *Index, path string, src *stubSource) {
					assert.Equal(t, 1, src.calls)
					assert.Equal(t, []string{"restart the pods"}, texts(t, idx, "pods"))

					version, err := idx.idx.GetInternal([]byte(_mappingVersionKey))
					require.NoError(t, err)
					assert.Equal(t, _mappingVersion, string(version))

					_, err = os.Stat(path + _buildSuffix)
					assert.True(t, os.IsNotExist(err), "the build directory is moved into place")
				},
			}
		},
		"Index with the current mapping version opens as is": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")

			prev := openIndex(t, path, &stubSource{docs: []document.Document{stubDocument("Runbook", "restart the pods")}})
			require.NoError(t, prev.Close())

			return tcase{
				Path:   path,
				Source: &stubSource{docs: []document.Document{stubDocument("Other", "scale the cluster")}},
				Check: func(t *testing.T, idx *Index, _ string, src *stubSource) {
					assert.Equal(t, 0, src.calls, "the source is not consulted")
					assert.Equal(t, []string{"restart the pods"}, texts(t, idx, "pods"))
					assert.Empty(t, texts(t, idx, "cluster"))
				},
			}
		},
		"Index with another mapping version is rebuilt": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")

			prev := openIndex(t, path, &stubSource{docs: []document.Document{stubDocument("Runbook", "restart the pods")}})
			require.NoError(t, prev.idx.SetInternal([]byte(_mappingVersionKey), []byte("0")))
			require.NoError(t, prev.Close())

			return tcase{
				Path:   path,
				Source: &stubSource{docs: []document.Document{stubDocument("Other", "scale the cluster")}},
				Check: func(t *testing.T, idx *Index, _ string, src *stubSource) {
					assert.Equal(t, 1, src.calls)
					assert.Empty(t, texts(t, idx, "pods"))
					assert.Equal(t, []string{"scale the cluster"}, texts(t, idx, "cluster"))
				},
			}
		},
		"Index without a mapping version is rebuilt": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")

			prev := openIndex(t, path, &stubSource{})
			require.NoError(t, prev.idx.DeleteInternal([]byte(_mappingVersionKey)))
			require.NoError(t, prev.Close())

			return tcase{
				Path:   path,
				Source: &stubSource{},
				Check: func(t *testing.T, _ *Index, _ string, src *stubSource) {
					assert.Equal(t, 1, src.calls)
				},
			}
		},
		"Unreadable index is rebuilt": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")
			require.NoError(t, os.WriteFile(path, []byte("not an index"), 0o600))

			return tcase{
				Path:   path,
				Source: &stubSource{},
				Check: func(t *testing.T, _ *Index, path string, src *stubSource) {
					assert.Equal(t, 1, src.calls)

					info, err := os.Stat(path)
					require.NoError(t, err)
					assert.True(t, info.IsDir())
				},
			}
		},
		"Stale build directory is replaced": func(t *testing.T) tcase {
			path := filepath.Join(t.TempDir(), "index")
			require.NoError(t, os.MkdirAll(path+_buildSuffix, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(path+_buildSuffix, "junk"), []byte("junk"), 0o600))

			return tcase{
				Path:   path,
				Source: &stubSource{},
				Check: func(t *testing.T, _ *Index, path string, src *stubSource) {
					assert.Equal(t, 1, src.calls)

					_, err := os.Stat(filepath.Join(path, "junk"))
					assert.True(t, os.IsNotExist(err))
				},
			}
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c := c(t)

			idx, err := Open(context.Background(), slog.New(slog.DiscardHandler), c.Path, c.Source)

			if c.Err != nil {
				require.Error(t, err)
				assert.Nil(t, idx)
			} else {
				require.NoError(t, err)
				require.NotNil(t, idx)

				defer func() { require.NoError(t, idx.Close()) }()
			}

			if c.Check != nil {
				c.Check(t, idx, c.Path, c.Source)
			}
		})
	}
}

func Test_openCurrent(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Path   string
		Opened bool
	}

	cc := map[string]tcase{
		"Missing path": {
			Path: filepath.Join(t.TempDir(), "index"),
		},
		"Path that is not an index": func() tcase {
			path := filepath.Join(t.TempDir(), "index")
			require.NoError(t, os.WriteFile(path, []byte("not an index"), 0o600))

			return tcase{Path: path}
		}(),
		"Current index": func() tcase {
			path := filepath.Join(t.TempDir(), "index")

			built := openIndex(t, path, &stubSource{})
			require.NoError(t, built.Close())

			return tcase{Path: path, Opened: true}
		}(),
		"Outdated index": func() tcase {
			path := filepath.Join(t.TempDir(), "index")

			built := openIndex(t, path, &stubSource{})
			require.NoError(t, built.idx.SetInternal([]byte(_mappingVersionKey), []byte("0")))
			require.NoError(t, built.Close())

			return tcase{Path: path}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			idx, err := openCurrent(slog.New(slog.DiscardHandler), c.Path)
			require.NoError(t, err)

			if !c.Opened {
				assert.Nil(t, idx)
				return
			}

			require.NotNil(t, idx)
			require.NoError(t, idx.Close())
		})
	}
}

func Test_build(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)

	// error
	path := filepath.Join(t.TempDir(), "index")
	src := &stubSource{err: assert.AnError}

	idx, err := build(context.Background(), log, path, src)
	require.Error(t, err)
	assert.Nil(t, idx)
	assert.Equal(t, 1, src.calls)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// success, replacing whatever was at the path
	require.NoError(t, os.WriteFile(path, []byte("previous"), 0o600))

	src = &stubSource{}

	idx, err = build(context.Background(), log, path, src)
	require.NoError(t, err)
	require.NotNil(t, idx)
	require.NoError(t, idx.Close())
	assert.Equal(t, 1, src.calls)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	_, err = os.Stat(path + _buildSuffix)
	assert.True(t, os.IsNotExist(err))
}

func Test_fill(t *testing.T) {
	t.Parallel()

	// error
	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	src := &stubSource{err: assert.AnError}

	require.Error(t, fill(context.Background(), slog.New(slog.DiscardHandler), idx.idx, src))
	assert.Equal(t, 1, src.calls)

	// success: every page is read until a short one, and the version is
	// stamped last.
	docs := make([]document.Document, 0, _buildPage+1)

	for i := range _buildPage + 1 {
		docs = append(docs, stubDocument("Doc "+strconv.Itoa(i), "page filler"))
	}

	src = &stubSource{docs: docs}

	require.NoError(t, idx.idx.DeleteInternal([]byte(_mappingVersionKey)))
	require.NoError(t, fill(context.Background(), slog.New(slog.DiscardHandler), idx.idx, src))

	assert.Equal(t, 2, src.calls)

	// one paragraph and one document-name entry per branch.
	ids, err := idx.ids(context.Background(), _fieldOrganizationID, "org-1")
	require.NoError(t, err)
	assert.Len(t, ids, 2*(_buildPage+1))

	version, err := idx.idx.GetInternal([]byte(_mappingVersionKey))
	require.NoError(t, err)
	assert.Equal(t, _mappingVersion, string(version))
}

func Test_indexSynonyms(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})

	// a build already loaded the synonyms; loading them again is an error
	// only once the index is closed.
	require.NoError(t, indexSynonyms(idx.idx))
	require.NoError(t, idx.Close())
	require.Error(t, indexSynonyms(idx.idx))
}

func Test_Index_Close(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})

	require.NoError(t, idx.Close())
}

func Test_Index_ReplaceBranch(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Index    *Index
		BranchID xid.ID
		Entries  map[string]Block
		Err      error
		Check    func(t *testing.T, idx *Index)
	}

	cc := map[string]tcase{
		// a closed index fails the lookup.
		"Error returned by ids": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			require.NoError(t, idx.Close())

			return tcase{
				Index:    idx,
				BranchID: stubScope().BranchID,
				Err:      assert.AnError,
			}
		}(),
		// an entry without an id is refused before anything is written.
		"Error returned by Batch.Index": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			t.Cleanup(func() { require.NoError(t, idx.Close()) })

			one := stubScope()

			require.NoError(t, idx.ReplaceBranch(context.Background(), one.BranchID, map[string]Block{
				"a": one.Block("a", "paragraph", "alpha"),
			}))

			return tcase{
				Index:    idx,
				BranchID: one.BranchID,
				Entries: map[string]Block{
					"e": {Text: "epsilon"},
				},
				Err: assert.AnError,
				Check: func(t *testing.T, idx *Index) {
					assert.Equal(t, []string{"alpha"}, texts(t, idx, "alpha"))
				},
			}
		}(),
		// the replacement updates, adds and removes, and leaves the other
		// branch alone.
		"Successful replacement": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			t.Cleanup(func() { require.NoError(t, idx.Close()) })

			one, two := stubScope(), stubScope()

			require.NoError(t, idx.ReplaceBranch(context.Background(), one.BranchID, map[string]Block{
				"a": one.Block("a", "paragraph", "alpha"),
				"b": one.Block("b", "paragraph", "beta"),
			}))
			require.NoError(t, idx.ReplaceBranch(context.Background(), two.BranchID, map[string]Block{
				"c": two.Block("c", "paragraph", "gamma"),
			}))

			return tcase{
				Index:    idx,
				BranchID: one.BranchID,
				Entries: map[string]Block{
					"a": one.Block("a", "paragraph", "alpha changed"),
					"d": one.Block("d", "paragraph", "delta"),
				},
				Check: func(t *testing.T, idx *Index) {
					ids, err := idx.ids(context.Background(), _fieldBranchID, one.BranchID.String())
					require.NoError(t, err)
					assert.ElementsMatch(t, []string{one.BranchID.String() + "-a", one.BranchID.String() + "-d"}, ids)
					assert.Equal(t, []string{"alpha changed"}, texts(t, idx, "alpha"))
					assert.Empty(t, texts(t, idx, "beta"))
					assert.Equal(t, []string{"gamma"}, texts(t, idx, "gamma"))
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Index.ReplaceBranch(context.Background(), c.BranchID, c.Entries)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Check != nil {
				c.Check(t, c.Index)
			}
		})
	}
}

func Test_Index_DeleteBranch(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	one, two := stubScope(), stubScope()

	require.NoError(t, idx.ReplaceBranch(context.Background(), one.BranchID, map[string]Block{"a": one.Block("a", "paragraph", "alpha")}))
	require.NoError(t, idx.ReplaceBranch(context.Background(), two.BranchID, map[string]Block{"b": two.Block("b", "paragraph", "beta")}))

	require.NoError(t, idx.DeleteBranch(context.Background(), one.BranchID))

	ids, err := idx.ids(context.Background(), _fieldBranchID, one.BranchID.String())
	require.NoError(t, err)
	assert.Empty(t, ids)

	ids, err = idx.ids(context.Background(), _fieldBranchID, two.BranchID.String())
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

func Test_Index_DeleteDocument(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	one, two := stubScope(), stubScope()
	fork := one
	fork.BranchID = xid.New()

	require.NoError(t, idx.ReplaceBranch(context.Background(), one.BranchID, map[string]Block{"a": one.Block("a", "paragraph", "alpha")}))
	require.NoError(t, idx.ReplaceBranch(context.Background(), fork.BranchID, map[string]Block{"a": fork.Block("a", "paragraph", "alpha")}))
	require.NoError(t, idx.ReplaceBranch(context.Background(), two.BranchID, map[string]Block{"b": two.Block("b", "paragraph", "beta")}))

	require.NoError(t, idx.DeleteDocument(context.Background(), one.DocumentID))

	ids, err := idx.ids(context.Background(), _fieldDocumentID, one.DocumentID.String())
	require.NoError(t, err)
	assert.Empty(t, ids)

	ids, err = idx.ids(context.Background(), _fieldDocumentID, two.DocumentID.String())
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

func Test_Index_DeleteOrganization(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	one, two := stubScope(), stubScope()
	two.OrganizationID = "org-2"

	require.NoError(t, idx.ReplaceBranch(context.Background(), one.BranchID, map[string]Block{"a": one.Block("a", "paragraph", "alpha")}))
	require.NoError(t, idx.ReplaceBranch(context.Background(), two.BranchID, map[string]Block{"b": two.Block("b", "paragraph", "beta")}))

	require.NoError(t, idx.DeleteOrganization(context.Background(), "org-1"))

	ids, err := idx.ids(context.Background(), _fieldOrganizationID, "org-1")
	require.NoError(t, err)
	assert.Empty(t, ids)

	ids, err = idx.ids(context.Background(), _fieldOrganizationID, "org-2")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

func Test_Index_deleteByTerm(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Index *Index
		Value string
		Err   error
		Check func(t *testing.T, idx *Index)
	}

	cc := map[string]tcase{
		// a closed index fails the lookup.
		"Error returned by ids": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			require.NoError(t, idx.Close())

			return tcase{
				Index: idx,
				Value: stubScope().BranchID.String(),
				Err:   assert.AnError,
			}
		}(),
		// nothing under the value: no batch is written.
		"Nothing under the value": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			t.Cleanup(func() { require.NoError(t, idx.Close()) })

			scope := stubScope()

			require.NoError(t, idx.ReplaceBranch(context.Background(), scope.BranchID, map[string]Block{"a": scope.Block("a", "paragraph", "alpha")}))

			return tcase{
				Index: idx,
				Value: "missing",
				Check: func(t *testing.T, idx *Index) {
					ids, err := idx.ids(context.Background(), _fieldBranchID, scope.BranchID.String())
					require.NoError(t, err)
					assert.Len(t, ids, 1)
				},
			}
		}(),
		"Successful deletion": func() tcase {
			idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
			t.Cleanup(func() { require.NoError(t, idx.Close()) })

			scope := stubScope()

			require.NoError(t, idx.ReplaceBranch(context.Background(), scope.BranchID, map[string]Block{"a": scope.Block("a", "paragraph", "alpha")}))

			return tcase{
				Index: idx,
				Value: scope.BranchID.String(),
				Check: func(t *testing.T, idx *Index) {
					ids, err := idx.ids(context.Background(), _fieldBranchID, scope.BranchID.String())
					require.NoError(t, err)
					assert.Empty(t, ids)
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := c.Index.deleteByTerm(context.Background(), _fieldBranchID, c.Value)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Check != nil {
				c.Check(t, c.Index)
			}
		})
	}
}

func Test_Index_ids(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, filepath.Join(t.TempDir(), "index"), &stubSource{})
	defer func() { require.NoError(t, idx.Close()) }()

	scope := stubScope()
	entries := make(map[string]Block, _idsPage+1)

	for i := range _idsPage + 1 {
		uid := "b" + strconv.Itoa(i)
		entries[uid] = scope.Block(uid, "paragraph", "filler")
	}

	require.NoError(t, idx.ReplaceBranch(context.Background(), scope.BranchID, entries))

	// every page is read, without duplicates.
	ids, err := idx.ids(context.Background(), _fieldBranchID, scope.BranchID.String())
	require.NoError(t, err)
	assert.Len(t, ids, _idsPage+1)

	seen := make(map[string]struct{}, len(ids))

	for _, id := range ids {
		seen[id] = struct{}{}
	}

	assert.Len(t, seen, _idsPage+1)

	// a cancelled context fails the lookup.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = idx.ids(ctx, _fieldBranchID, scope.BranchID.String())
	require.Error(t, err)
}
