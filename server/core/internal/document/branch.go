package document

import (
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

var (
	// ErrDefaultBranchRename is returned when a rename targets the default
	// branch.
	ErrDefaultBranchRename = errutil.New(http.StatusBadRequest, "document.default_branch_rename", "the default branch cannot be renamed")

	// ErrBranchSelfMerge is returned when a branch is merged into itself.
	ErrBranchSelfMerge = errutil.New(http.StatusBadRequest, "document.branch_self_merge", "cannot merge a branch into itself")

	// ErrDefaultBranchDelete is returned when a delete targets the default
	// branch.
	ErrDefaultBranchDelete = errutil.New(http.StatusConflict, "document.default_branch", "cannot delete the default branch")

	// ErrLastBranchDelete is returned when a delete targets the only branch
	// a document has left.
	ErrLastBranchDelete = errutil.New(http.StatusConflict, "document.last_branch", "cannot delete the last branch")

	// ErrBranchProtected is returned when a protected branch, or what
	// hangs off it, would change.
	ErrBranchProtected = errutil.New(http.StatusForbidden, "document.branch_protected", "a protected branch cannot be changed")
)

// AllowsBranchRename reports whether the branch may take the given name.
// The default branch is what the tree and content queries return, and its
// name is what the user sees the document under; a rename would leave the
// two out of step.
func (d Document) AllowsBranchRename(name null.String) error {
	if d.Default && name.Valid && name.String != d.BranchName {
		return ErrDefaultBranchRename
	}

	return nil
}

// AllowsBranchDelete reports whether the branch may be deleted: the
// default branch never is.
func (d Document) AllowsBranchDelete() error {
	if d.Default {
		return ErrDefaultBranchDelete
	}

	return nil
}

// AllowsDocumentChange reports whether the branch, or what hangs off it
// such as its tags and hooks, may change. A protected branch may not,
// though its hooks can still be updated or reset.
func (d Document) AllowsDocumentChange() error {
	if d.Protected {
		return ErrBranchProtected
	}

	return nil
}

// AllowsMerge reports whether one branch may be merged into another. A
// self-merge would soft-delete the branch's hooks and comments and then
// copy the hooks back from the same, now hook-less, branch, permanently
// destroying both.
func AllowsMerge(fromBranchID, toBranchID xid.ID) error {
	if fromBranchID == toBranchID {
		return ErrBranchSelfMerge
	}

	return nil
}
