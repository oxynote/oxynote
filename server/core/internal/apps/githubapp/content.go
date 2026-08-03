package githubapp

import (
	"slices"
	"strings"

	"github.com/google/go-github/v72/github"
)

// TreeItemType represents the type of a tree item, either a file or a folder.
type TreeItemType string

const (
	// TreeItemTypeFile represents a file in the tree.
	TreeItemTypeFile TreeItemType = "file"

	// TreeItemTypeFolder represents a folder in the tree.
	TreeItemTypeFolder TreeItemType = "folder"
)

// Repository represents a GitHub repository.
type Repository struct {
	// Name is the name of the repository.
	Name string `json:"name"`

	// DefaultBranch is the default branch of the repository.
	DefaultBranch string `json:"defaultBranch"`
}

// Issue represents a GitHub issue.
type Issue struct {
	// ID is the unique identifier of the issue.
	ID int64 `json:"id"`

	// Title is the title of the issue.
	Title string `json:"title"`

	// URL is the URL of the issue.
	URL string `json:"url"`

	// UserID is the ID of the user who created the issue.
	UserID int64 `json:"userId"`

	// Draft indicates whether the issue is a draft.
	Draft bool `json:"draft"`

	// State is the current state of the issue, such as "open" or "closed".
	State string `json:"state"`

	// UpdatedAt is the timestamp when the issue was last updated.
	UpdatedAt string `json:"updatedAt"`

	// CreatedAt is the timestamp when the issue was created.
	CreatedAt string `json:"createdAt"`
}

// FileContent represents the content of a file in a GitHub repository.
type FileContent struct {
	// Path is the file path within the repository.
	Path string `json:"path"`

	// Content is the decoded text content of the file.
	Content string `json:"content"`

	// SHA is the git SHA of the file.
	SHA string `json:"sha"`

	// Size is the size of the file in bytes.
	Size int `json:"size"`
}

// CodeSearchResult represents a single code search result.
type CodeSearchResult struct {
	// Name is the filename.
	Name string `json:"name"`

	// Path is the file path within the repository.
	Path string `json:"path"`

	// URL is the HTML URL of the file.
	URL string `json:"url"`

	// Fragments contains text fragments showing where the match occurred.
	Fragments []string `json:"fragments,omitempty"`
}

// Tree represents a Git tree structure.
type Tree []TreeItem

// GetItem retrieves a TreeItem from the tree based on the given path.
func (t Tree) GetItem(path string) (TreeItem, bool) {
	parts := strings.Split(path, "/")

	var items = t

	for i := range parts {
		j := slices.IndexFunc(items, func(ti TreeItem) bool {
			return ti.Name == strings.Join(parts[:i+1], "/")
		})
		if j == -1 {
			return TreeItem{}, false
		}

		item := items[j]

		if i != len(parts)-1 {
			if item.Type != TreeItemTypeFolder {
				return TreeItem{}, false
			}

			items = item.Items

			continue
		}

		return item, true
	}

	return TreeItem{}, false
}

// TreeItem represents an item in a Git tree structure.
type TreeItem struct {
	// Type is the type of the tree item.
	Type TreeItemType `json:"type"`

	// Name is the file path of the tree item.
	Name string `json:"name"`

	// Items is a list of child tree items, if any.
	Items []TreeItem `json:"items,omitempty"`

	// Checksum is the checksum of the item.
	Checksum string `json:"checksum"`
}

// ParseTreeItems converts a list of GitHub TreeEntry objects into a
// list of TreeItem objects.
func ParseTreeItems(items []*github.TreeEntry) []TreeItem {
	var (
		res     []TreeItem
		parents = make(map[string]*TreeItem)
	)

	for _, item := range items {
		path := item.GetPath()
		parts := strings.Split(path, "/")

		item := &TreeItem{
			Type:     parseTreeItemType(item.GetType()),
			Name:     path,
			Checksum: item.GetSHA(),
		}

		if len(parts) == 1 {
			res = append(res, *item)

			if item.Type == TreeItemTypeFolder {
				parents[item.Name] = &res[len(res)-1]
			}

			continue
		}

		parentPath := strings.Join(parts[:len(parts)-1], "/")

		parent := parents[parentPath]
		parent.Items = append(parent.Items, *item)

		if item.Type == TreeItemTypeFolder {
			parents[path] = &parent.Items[len(parent.Items)-1]
		}
	}

	return res
}

// parseTreeItemType converts a GitHub tree entry type string to a TreeItemType.
func parseTreeItemType(tp string) TreeItemType {
	if tp == "tree" {
		return TreeItemTypeFolder
	}

	return TreeItemTypeFile
}
