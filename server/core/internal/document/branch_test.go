package document

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
)

func Test_Document_AllowsBranchRename(t *testing.T) {
	cc := map[string]struct {
		Doc  Document
		Name null.String
		Err  error
	}{
		"Default branch renamed":        {Doc: Document{Default: true, BranchName: DefaultBranch}, Name: null.StringFrom("Renamed"), Err: ErrDefaultBranchRename},
		"Default branch keeps its name": {Doc: Document{Default: true, BranchName: DefaultBranch}, Name: null.StringFrom(DefaultBranch)},
		"Default branch without a name": {Doc: Document{Default: true, BranchName: DefaultBranch}},
		"Fork renamed":                  {Doc: Document{BranchName: "feature"}, Name: null.StringFrom("Renamed")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Doc.AllowsBranchRename(c.Name))
		})
	}
}

func Test_Document_AllowsBranchDelete(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Doc Document
		Err error
	}{
		"Default branch deleted": {Doc: Document{Default: true}, Err: ErrDefaultBranchDelete},
		"Fork deleted":           {Doc: Document{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Doc.AllowsBranchDelete())
		})
	}
}

func Test_Document_AllowsDocumentChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Doc Document
		Err error
	}{
		"Protected branch":   {Doc: Document{Protected: true}, Err: ErrBranchProtected},
		"Unprotected branch": {Doc: Document{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Doc.AllowsDocumentChange())
		})
	}
}

func Test_AllowsMerge(t *testing.T) {
	t.Parallel()

	from, to := xid.New(), xid.New()

	cc := map[string]struct {
		From xid.ID
		To   xid.ID
		Err  error
	}{
		"Branch merged into itself":      {From: from, To: from, Err: ErrBranchSelfMerge},
		"Branch merged into another one": {From: from, To: to},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, AllowsMerge(c.From, c.To))
		})
	}
}
