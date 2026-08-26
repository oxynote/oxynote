package github

import (
	"testing"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entry builds a go-github tree entry for tests.
func entry(path, tp, sha string) *gogithub.TreeEntry {
	return &gogithub.TreeEntry{
		Path: new(path),
		Type: new(tp),
		SHA:  new(sha),
	}
}

func Test_Tree_GetItem(t *testing.T) {
	t.Parallel()

	tree := Tree{
		{Type: TreeItemTypeFile, Name: "readme.md", Checksum: "sha-1"},
		{
			Type: TreeItemTypeFolder,
			Name: "docs",
			Items: []TreeItem{
				{Type: TreeItemTypeFile, Name: "docs/guide.md", Checksum: "sha-2"},
				{
					Type: TreeItemTypeFolder,
					Name: "docs/api",
					Items: []TreeItem{
						{Type: TreeItemTypeFile, Name: "docs/api/spec.md", Checksum: "sha-3"},
					},
				},
			},
		},
	}

	tests := map[string]struct {
		Tree         Tree
		Path         string
		ExpectedItem TreeItem
		ExpectedOK   bool
	}{
		"Top-level file is found": {
			Tree:         tree,
			Path:         "readme.md",
			ExpectedItem: tree[0],
			ExpectedOK:   true,
		},
		"Nested file is found": {
			Tree:         tree,
			Path:         "docs/guide.md",
			ExpectedItem: tree[1].Items[0],
			ExpectedOK:   true,
		},
		"Deeply nested file is found": {
			Tree:         tree,
			Path:         "docs/api/spec.md",
			ExpectedItem: tree[1].Items[1].Items[0],
			ExpectedOK:   true,
		},
		"Folder itself is found": {
			Tree:         tree,
			Path:         "docs/api",
			ExpectedItem: tree[1].Items[1],
			ExpectedOK:   true,
		},
		"Missing path is not found": {
			Tree: tree,
			Path: "docs/missing.md",
		},
		"Path through a file is not found": {
			Tree: tree,
			Path: "readme.md/nested",
		},
		"Empty tree finds nothing": {
			Tree: Tree{},
			Path: "readme.md",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			item, ok := tc.Tree.GetItem(tc.Path)

			assert.Equal(t, tc.ExpectedOK, ok)
			assert.Equal(t, tc.ExpectedItem, item)
		})
	}
}

func Test_ParseTreeItems(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Items    []*gogithub.TreeEntry
		Expected []TreeItem
	}{
		"Nil entries produce an empty tree": {},
		"Flat files are kept in order": {
			Items: []*gogithub.TreeEntry{
				entry("readme.md", "blob", "sha-1"),
				entry("main.go", "blob", "sha-2"),
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "readme.md", Checksum: "sha-1"},
				{Type: TreeItemTypeFile, Name: "main.go", Checksum: "sha-2"},
			},
		},
		"Unknown entry types become files": {
			Items: []*gogithub.TreeEntry{
				entry("weird", "commit", "sha-1"),
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "weird", Checksum: "sha-1"},
			},
		},
		"Folder children are nested": {
			Items: []*gogithub.TreeEntry{
				entry("docs", "tree", "sha-1"),
				entry("docs/guide.md", "blob", "sha-2"),
				entry("docs/api", "tree", "sha-3"),
				entry("docs/api/spec.md", "blob", "sha-4"),
			},
			Expected: []TreeItem{
				{
					Type: TreeItemTypeFolder, Name: "docs", Checksum: "sha-1",
					Items: []TreeItem{
						{Type: TreeItemTypeFile, Name: "docs/guide.md", Checksum: "sha-2"},
						{
							Type: TreeItemTypeFolder, Name: "docs/api", Checksum: "sha-3",
							Items: []TreeItem{
								{Type: TreeItemTypeFile, Name: "docs/api/spec.md", Checksum: "sha-4"},
							},
						},
					},
				},
			},
		},
		"Orphaned children are skipped": {
			Items: []*gogithub.TreeEntry{
				entry("readme.md", "blob", "sha-1"),
				entry("missing/orphan.md", "blob", "sha-2"),
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "readme.md", Checksum: "sha-1"},
			},
		},
		"Top-level growth does not detach earlier folders": {
			// enough top-level entries after the folder to force the
			// result slice to reallocate before the children arrive.
			Items: []*gogithub.TreeEntry{
				entry("docs", "tree", "sha-1"),
				entry("a.md", "blob", "sha-2"),
				entry("b.md", "blob", "sha-3"),
				entry("c.md", "blob", "sha-4"),
				entry("d.md", "blob", "sha-5"),
				entry("docs/guide.md", "blob", "sha-6"),
			},
			Expected: []TreeItem{
				{
					Type: TreeItemTypeFolder, Name: "docs", Checksum: "sha-1",
					Items: []TreeItem{
						{Type: TreeItemTypeFile, Name: "docs/guide.md", Checksum: "sha-6"},
					},
				},
				{Type: TreeItemTypeFile, Name: "a.md", Checksum: "sha-2"},
				{Type: TreeItemTypeFile, Name: "b.md", Checksum: "sha-3"},
				{Type: TreeItemTypeFile, Name: "c.md", Checksum: "sha-4"},
				{Type: TreeItemTypeFile, Name: "d.md", Checksum: "sha-5"},
			},
		},
		"Sibling growth does not detach nested folders": {
			// enough siblings inside "docs" after the "docs/api" folder to
			// force the folder's Items slice to reallocate before the
			// grandchildren arrive.
			Items: []*gogithub.TreeEntry{
				entry("docs", "tree", "sha-1"),
				entry("docs/api", "tree", "sha-2"),
				entry("docs/a.md", "blob", "sha-3"),
				entry("docs/b.md", "blob", "sha-4"),
				entry("docs/c.md", "blob", "sha-5"),
				entry("docs/api/spec.md", "blob", "sha-6"),
			},
			Expected: []TreeItem{
				{
					Type: TreeItemTypeFolder, Name: "docs", Checksum: "sha-1",
					Items: []TreeItem{
						{
							Type: TreeItemTypeFolder, Name: "docs/api", Checksum: "sha-2",
							Items: []TreeItem{
								{Type: TreeItemTypeFile, Name: "docs/api/spec.md", Checksum: "sha-6"},
							},
						},
						{Type: TreeItemTypeFile, Name: "docs/a.md", Checksum: "sha-3"},
						{Type: TreeItemTypeFile, Name: "docs/b.md", Checksum: "sha-4"},
						{Type: TreeItemTypeFile, Name: "docs/c.md", Checksum: "sha-5"},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NotPanics(t, func() {
				assert.Equal(t, tc.Expected, ParseTreeItems(tc.Items))
			})
		})
	}
}
