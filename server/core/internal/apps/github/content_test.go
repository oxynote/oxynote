package github

import (
	"testing"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	cc := map[string]struct {
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

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			item, ok := c.Tree.GetItem(c.Path)

			assert.Equal(t, c.ExpectedOK, ok)
			assert.Equal(t, c.ExpectedItem, item)
		})
	}
}

func Test_ParseTreeItems(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Items    []*gogithub.TreeEntry
		Expected []TreeItem
	}{
		"Nil entries produce an empty tree": {},
		"Flat files are kept in order": {
			Items: []*gogithub.TreeEntry{
				{Path: new("readme.md"), Type: new("blob"), SHA: new("sha-1")},
				{Path: new("main.go"), Type: new("blob"), SHA: new("sha-2")},
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "readme.md", Checksum: "sha-1"},
				{Type: TreeItemTypeFile, Name: "main.go", Checksum: "sha-2"},
			},
		},
		"Unknown entry types become files": {
			Items: []*gogithub.TreeEntry{
				{Path: new("weird"), Type: new("commit"), SHA: new("sha-1")},
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "weird", Checksum: "sha-1"},
			},
		},
		"Folder children are nested": {
			Items: []*gogithub.TreeEntry{
				{Path: new("docs"), Type: new("tree"), SHA: new("sha-1")},
				{Path: new("docs/guide.md"), Type: new("blob"), SHA: new("sha-2")},
				{Path: new("docs/api"), Type: new("tree"), SHA: new("sha-3")},
				{Path: new("docs/api/spec.md"), Type: new("blob"), SHA: new("sha-4")},
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
				{Path: new("readme.md"), Type: new("blob"), SHA: new("sha-1")},
				{Path: new("missing/orphan.md"), Type: new("blob"), SHA: new("sha-2")},
			},
			Expected: []TreeItem{
				{Type: TreeItemTypeFile, Name: "readme.md", Checksum: "sha-1"},
			},
		},
		"Top-level growth does not detach earlier folders": {
			// enough top-level entries after the folder to force the
			// result slice to reallocate before the children arrive.
			Items: []*gogithub.TreeEntry{
				{Path: new("docs"), Type: new("tree"), SHA: new("sha-1")},
				{Path: new("a.md"), Type: new("blob"), SHA: new("sha-2")},
				{Path: new("b.md"), Type: new("blob"), SHA: new("sha-3")},
				{Path: new("c.md"), Type: new("blob"), SHA: new("sha-4")},
				{Path: new("d.md"), Type: new("blob"), SHA: new("sha-5")},
				{Path: new("docs/guide.md"), Type: new("blob"), SHA: new("sha-6")},
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
				{Path: new("docs"), Type: new("tree"), SHA: new("sha-1")},
				{Path: new("docs/api"), Type: new("tree"), SHA: new("sha-2")},
				{Path: new("docs/a.md"), Type: new("blob"), SHA: new("sha-3")},
				{Path: new("docs/b.md"), Type: new("blob"), SHA: new("sha-4")},
				{Path: new("docs/c.md"), Type: new("blob"), SHA: new("sha-5")},
				{Path: new("docs/api/spec.md"), Type: new("blob"), SHA: new("sha-6")},
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

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			require.NotPanics(t, func() {
				assert.Equal(t, c.Expected, ParseTreeItems(c.Items))
			})
		})
	}
}
