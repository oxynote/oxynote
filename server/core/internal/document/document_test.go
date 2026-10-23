package document

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDocument builds a document with a small content tree for tests.
func stubDocument() Document {
	return Document{
		ID:             xid.New(),
		OrganizationID: "org-1",
		BranchID:       xid.New(),
		BranchName:     DefaultBranch,
		DocumentName:   "Runbook",
		Icon:           "📘",
		Content:        stubMarkedTree(),
		RawContent:     []byte("raw"),
		Default:        true,
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy:      null.StringFrom("user-1"),
	}
}

func Test_NewDocument(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(xid.New())

	doc := NewDocument(CreateInput{
		Name:     "Runbook",
		Icon:     "📘",
		ParentID: parentID,
	}, "org-1", "user-1")

	assert.False(t, doc.ID.IsZero())
	assert.False(t, doc.BranchID.IsZero())
	assert.Equal(t, parentID, doc.ParentID)
	assert.Equal(t, "org-1", doc.OrganizationID)
	assert.Equal(t, DefaultBranch, doc.BranchName)
	assert.Equal(t, "Runbook", doc.DocumentName)
	assert.Equal(t, "📘", doc.Icon)
	assert.True(t, doc.Default)
	assert.False(t, doc.Protected)
	assert.Equal(t, null.StringFrom("user-1"), doc.CreatedBy)
	assert.Equal(t, null.StringFrom("user-1"), doc.LastUpdatedBy)
	assert.Equal(t, doc.CreatedAt, doc.UpdatedAt)

	// the initial content is a single empty paragraph with a uid.
	require.Len(t, doc.Content.Content, 1)
	assert.Equal(t, BlockNodeParagraph, doc.Content.Content[0].Type)

	uid, ok := doc.Content.Content[0].UID()
	assert.True(t, ok)
	assert.Len(t, uid, 21)
}

func Test_Document_ApplyUpdate(t *testing.T) {
	t.Parallel()

	newContent := stubTree()

	cc := map[string]struct {
		Protected             bool
		Input                 UpdateInput
		ExpectedErr           error
		ExpectedLastUpdatedBy null.String
	}{
		"Protected document rejects a user update": {
			Protected:   true,
			Input:       UpdateInput{Name: "New"},
			ExpectedErr: httpserver.ErrNotPermitted,
		},
		"Protected document accepts a system update": {
			Protected: true,
			Input: UpdateInput{
				Name:   "New",
				Branch: "feature-x",
				System: true,
			},
			ExpectedLastUpdatedBy: null.StringFrom("user-1"),
		},
		"Maintainers set the last updater": {
			Input: UpdateInput{
				Name:        "New",
				Branch:      "feature-x",
				Maintainers: []string{"user-2", "user-3"},
			},
			ExpectedLastUpdatedBy: null.StringFrom("user-3"),
		},
		"No maintainers keep the previous last updater": {
			Input: UpdateInput{
				Name:   "New",
				Branch: "feature-x",
			},
			ExpectedLastUpdatedBy: null.StringFrom("user-1"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc := stubDocument()
			doc.Protected = c.Protected
			doc.LastUpdatedBy = null.StringFrom("user-1")

			c.Input.Icon = "🚀"
			c.Input.Content = newContent
			c.Input.RawContent = []byte("new-raw")

			nd, err := doc.ApplyUpdate(c.Input)

			if c.ExpectedErr != nil {
				assert.Equal(t, c.ExpectedErr, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Input.Branch, nd.BranchName)
			assert.Equal(t, "New", nd.DocumentName)
			assert.Equal(t, "🚀", nd.Icon)
			assert.Equal(t, newContent, nd.Content)
			assert.Equal(t, []byte("new-raw"), nd.RawContent)
			assert.Equal(t, c.ExpectedLastUpdatedBy, nd.LastUpdatedBy)

			if c.Input.System {
				assert.Equal(t, doc.UpdatedAt, nd.UpdatedAt)
			} else {
				assert.NotEqual(t, doc.UpdatedAt, nd.UpdatedAt)
			}
		})
	}
}

func Test_Document_MergeBranch(t *testing.T) {
	t.Parallel()

	doc := stubDocument()

	source := Branch{
		BranchID:     xid.New(),
		BranchName:   "feature-x",
		DocumentName: "Runbook v2",
		Icon:         "🚀",
		Content:      stubMarkedTree(),
		RawContent:   []byte("source-raw"),
	}

	merged := doc.MergeBranch(source, "user-2")

	// identity and branch metadata stay with the target document.
	assert.Equal(t, doc.ID, merged.ID)
	assert.Equal(t, doc.BranchID, merged.BranchID)
	assert.Equal(t, doc.BranchName, merged.BranchName)
	assert.Equal(t, doc.Default, merged.Default)
	assert.Equal(t, doc.CreatedAt, merged.CreatedAt)
	assert.Equal(t, doc.CreatedBy, merged.CreatedBy)

	// content and display data come from the source, stripped of
	// comment marks, with raw content reset.
	assert.Equal(t, "Runbook v2", merged.DocumentName)
	assert.Equal(t, "🚀", merged.Icon)
	assert.Equal(t, source.Content.StripCommentMarks(), merged.Content)
	assert.Nil(t, merged.RawContent)
	assert.Equal(t, null.StringFrom("user-2"), merged.LastUpdatedBy)
}

func Test_Document_Fork(t *testing.T) {
	t.Parallel()

	doc := stubDocument()
	doc.Protected = true

	forked := doc.Fork("feature-x", "user-2")

	// the document identity stays, the branch is a new one that is
	// neither protected nor default whatever its source was.
	assert.Equal(t, doc.ID, forked.ID)
	assert.Equal(t, doc.OrganizationID, forked.OrganizationID)
	assert.NotEqual(t, doc.BranchID, forked.BranchID)
	assert.Equal(t, "feature-x", forked.BranchName)
	assert.False(t, forked.Protected)
	assert.False(t, forked.Default)

	// display data is carried over; the content loses its comment marks
	// and the raw content is left for the first load to rebuild.
	assert.Equal(t, doc.DocumentName, forked.DocumentName)
	assert.Equal(t, doc.Icon, forked.Icon)
	assert.NotEqual(t, doc.Content, forked.Content)
	assert.Equal(t, doc.Content.StripCommentMarks(), forked.Content)
	assert.Nil(t, forked.RawContent)

	assert.Equal(t, null.StringFrom("user-2"), forked.CreatedBy)
	assert.Equal(t, null.StringFrom("user-2"), forked.LastUpdatedBy)
	assert.Equal(t, forked.CreatedAt, forked.UpdatedAt)
}

func Test_Document_ApplyProtection(t *testing.T) {
	t.Parallel()

	doc := stubDocument()

	protected := doc.ApplyProtection(true, "user-2")
	assert.True(t, protected.Protected)
	assert.Equal(t, null.StringFrom("user-2"), protected.LastUpdatedBy)

	unprotected := protected.ApplyProtection(false, "user-3")
	assert.False(t, unprotected.Protected)
	assert.Equal(t, null.StringFrom("user-3"), unprotected.LastUpdatedBy)
}

func Test_Document_ApplyBranchUpdate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name              null.String
		Protected         null.Bool
		ExpectedName      string
		ExpectedProtected bool
	}{
		"Name only": {
			Name:         null.StringFrom("renamed"),
			ExpectedName: "renamed",
		},
		"Protection only": {
			Protected:         null.BoolFrom(true),
			ExpectedName:      DefaultBranch,
			ExpectedProtected: true,
		},
		"Both": {
			Name:              null.StringFrom("renamed"),
			Protected:         null.BoolFrom(true),
			ExpectedName:      "renamed",
			ExpectedProtected: true,
		},
		"Neither": {
			ExpectedName: DefaultBranch,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc := stubDocument()

			nd := doc.ApplyBranchUpdate(c.Name, c.Protected, "user-2")

			assert.Equal(t, c.ExpectedName, nd.BranchName)
			assert.Equal(t, c.ExpectedProtected, nd.Protected)
			assert.Equal(t, null.StringFrom("user-2"), nd.LastUpdatedBy)
			assert.NotEqual(t, doc.UpdatedAt, nd.UpdatedAt)
		})
	}
}

func Test_Document_Title(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		BranchName string
		Default    bool
		Result     string
	}{
		"Default branch shows the name only": {
			BranchName: DefaultBranch,
			Default:    true,
			Result:     "Runbook",
		},
		"Other branch appends the branch name": {
			BranchName: "feature-x",
			Result:     "Runbook on branch feature-x",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc := stubDocument()
			doc.BranchName = c.BranchName
			doc.Default = c.Default

			assert.Equal(t, c.Result, doc.Title())
		})
	}
}

func Test_Document_SnapshotEqual(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Mutate func(d *Document)
		Equal  bool
	}{
		"Identical": {
			Mutate: func(_ *Document) {},
			Equal:  true,
		},
		"Different name": {
			Mutate: func(d *Document) {
				d.DocumentName = "Other"
			},
		},
		"Different icon": {
			Mutate: func(d *Document) {
				d.Icon = "📕"
			},
		},
		"Different block": {
			Mutate: func(d *Document) {
				d.Content.Content = d.Content.Content[1:]
			},
		},
		"Different attribute": {
			Mutate: func(d *Document) {
				d.Content.Content[0].Attrs = map[string]any{"uid": "changed"}
			},
		},
		"Different raw content only": {
			Mutate: func(d *Document) {
				d.RawContent = []byte("other")
			},
			Equal: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc := stubDocument()
			other := stubDocument()
			other.ID = doc.ID
			other.BranchID = doc.BranchID
			c.Mutate(&other)

			assert.Equal(t, c.Equal, doc.SnapshotEqual(other))
		})
	}
}

func Test_Document_Duplicate(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Document Document
		Check    func(t *testing.T, orig, dup Document, files map[string]string)
	}

	cc := map[string]tcase{
		"Metadata is reset and the content gets fresh uids": func() tcase {
			doc := stubDocument()
			doc.Protected = true

			return tcase{
				Document: doc,
				Check: func(t *testing.T, orig, dup Document, files map[string]string) {
					assert.Empty(t, files)

					assert.NotEqual(t, orig.ID, dup.ID)
					assert.NotEqual(t, orig.BranchID, dup.BranchID)
					assert.Equal(t, orig.ParentID, dup.ParentID)
					assert.Equal(t, orig.OrganizationID, dup.OrganizationID)
					assert.Equal(t, DefaultBranch, dup.BranchName)
					assert.Contains(t, dup.DocumentName, "Runbook (")
					assert.Equal(t, orig.Icon, dup.Icon)
					assert.Nil(t, dup.RawContent)
					assert.False(t, dup.Protected)
					assert.Equal(t, null.StringFrom("user-2"), dup.CreatedBy)
					assert.Equal(t, null.StringFrom("user-2"), dup.LastUpdatedBy)

					origUID, _ := orig.Content.Content[0].UID()
					dupUID, ok := dup.Content.Content[0].UID()
					require.True(t, ok)
					assert.NotEqual(t, origUID, dupUID)
				},
			}
		}(),
		"File references are remapped to the duplicate": func() tcase {
			doc := stubDocument()
			doc.Content.Content[0] = Block{
				Type: BlockNodeImageBlock,
				Attrs: Attributes{
					"uid": "img-1-aaaaaaaaaaaaaaa",
					"src": "https://app.test/core" + FilePath(doc.ID, "img-1-aaaaaaaaaaaaaaa", "shot.png"),
				},
			}

			return tcase{
				Document: doc,
				Check: func(t *testing.T, _, dup Document, files map[string]string) {
					require.Len(t, files, 1)

					newID, ok := files["img-1-aaaaaaaaaaaaaaa"]
					require.True(t, ok)

					// the duplicate refers to its own copy under its own document.
					assert.Equal(
						t,
						"https://app.test/core"+FilePath(dup.ID, newID, "shot.png"),
						dup.Content.Content[0].Attrs["src"],
					)
				},
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			dup, files, uids := c.Document.Duplicate("user-2")

			c.Check(t, c.Document, dup, files)
			assertUIDsRemapped(t, c.Document.Content, dup.Content, uids)
		})
	}
}

func Test_NewDocumentContent(t *testing.T) {
	t.Parallel()

	rb := NewDocumentContent()

	assert.Equal(t, BlockNodeDoc, rb.Type)
	require.Len(t, rb.Content, 1)
	assert.Equal(t, BlockNodeParagraph, rb.Content[0].Type)

	uid, ok := rb.Content[0].UID()
	assert.True(t, ok)
	assert.Len(t, uid, 21)
}

func Test_UpdateInput_AddsMaintainers(t *testing.T) {
	cc := map[string]struct {
		Maintainers []string
		Stored      []string
		Result      bool
	}{
		"New maintainer":         {Maintainers: []string{"u1", "u2"}, Stored: []string{"u1"}, Result: true},
		"Known maintainers only": {Maintainers: []string{"u1"}, Stored: []string{"u1", "u2"}},
		"No maintainers":         {Stored: []string{"u1"}},
		"Nobody stored yet":      {Maintainers: []string{"u1"}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, UpdateInput{Maintainers: c.Maintainers}.AddsMaintainers(c.Stored))
		})
	}
}

func Test_NewWelcomeDocument(t *testing.T) {
	t.Parallel()

	doc, err := NewWelcomeDocument("org1", "u1", null.Value[xid.ID]{})
	require.NoError(t, err)

	assert.Equal(t, "Welcome to Oxynote!", doc.DocumentName)
	assert.Equal(t, "mingcute:flag-4-fill", doc.Icon)
	assert.Equal(t, "org1", doc.OrganizationID)
	assert.Equal(t, null.StringFrom("u1"), doc.CreatedBy)
	assert.True(t, doc.Default)
	assert.Equal(t, BlockNodeDoc, doc.Content.Type)
	assert.NotEmpty(t, doc.Content.Content)
}

func Test_InitialDocumentContent(t *testing.T) {
	t.Parallel()

	dataSourceID := xid.New()

	cc := map[string]struct {
		DataSourceID null.Value[xid.ID]
		Check        func(t *testing.T, rb RootBlock)
	}{
		"Charts read the data source": {
			DataSourceID: null.ValueFrom(dataSourceID),
			Check: func(t *testing.T, rb RootBlock) {
				assert.Equal(t, BlockNodeDoc, rb.Type)

				// every dataSourceId attribute in the tree points at the given
				// data source, and every uid is regenerated (21 chars).
				var (
					dataSourceAttrs int
					metrics         int
					checkBlock      func(b Block)
				)

				checkBlock = func(b Block) {
					if v, ok := b.Attrs["dataSourceId"]; ok {
						dataSourceAttrs++

						assert.Equal(t, dataSourceID.String(), v)
					}

					if uid, ok := b.UID(); ok {
						assert.Len(t, uid, 21)
					}

					// every chart follows the built-in metric variables.
					if b.Type == BlockNodeMetricBlock {
						metrics++

						assert.Equal(t, "$metric_range", b.Attrs[AttrTimeRange])
						assert.Equal(t, "$metric_refresh", b.Attrs[AttrRefreshInterval])
					}

					for _, cb := range b.Content {
						checkBlock(cb)
					}
				}

				for _, b := range rb.Content {
					checkBlock(b)
				}

				assert.NotZero(t, dataSourceAttrs, "the seeded content should reference the data source")
				assert.NotZero(t, metrics)
			},
		},
		// without a data source the charts have nothing to read, so they are
		// dropped instead of pointing at an id that was never stored.
		"Charts dropped without a data source": {
			Check: func(t *testing.T, rb RootBlock) {
				for _, b := range rb.Content {
					assert.NotEqual(t, BlockNodeMetricBlock, b.Type)
				}
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rb, err := InitialDocumentContent(c.DataSourceID)
			require.NoError(t, err)

			assert.NotEmpty(t, rb.Content)
			c.Check(t, rb)
		})
	}
}
