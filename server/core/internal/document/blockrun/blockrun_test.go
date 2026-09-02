package blockrun

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/simulation"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const (
	// _blockUID names the block every case addresses.
	_blockUID = "block-1"

	// _organizationID is the organization every case belongs to.
	_organizationID = "org-1"
)

// docWith wraps the given block in the document the runner resolves it
// out of.
func docWith(documentID xid.ID, block document.Block) *document.Document {
	doc := &document.Document{ID: documentID, OrganizationID: _organizationID}
	doc.Content = document.RootBlock{
		Type:    document.BlockNodeDoc,
		Content: []document.Block{block},
	}

	return doc
}

// blockOfType is a block of the given kind carrying the addressed uid.
func blockOfType(typ document.BlockNodeType) document.Block {
	return document.Block{
		Type:  typ,
		Attrs: document.Attributes{document.AttrUID: _blockUID},
	}
}

func Test_Runner_Run(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	cc := map[string]struct {
		Doc        *document.Document
		DocErr     bool
		SimResult  simulation.Result
		SimErr     error
		DocumentID xid.ID
		BlockUID   string
		Expected   any
		Err        error
		SimCalls   int
	}{
		"A metric block is handed to its simulation": {
			Doc:        docWith(documentID, blockOfType(document.BlockNodeMetricBlock)),
			SimResult:  simulation.Result{Cleared: true},
			DocumentID: documentID,
			BlockUID:   _blockUID,
			Expected:   simulation.Result{Cleared: true},
			SimCalls:   1,
		},
		"What the block kind reports is what comes back": {
			Doc:        docWith(documentID, blockOfType(document.BlockNodeMetricBlock)),
			SimErr:     assert.AnError,
			DocumentID: documentID,
			BlockUID:   _blockUID,
			Err:        assert.AnError,
			SimCalls:   1,
		},
		"The uid names no block on the branch": {
			Doc:        docWith(documentID, blockOfType(document.BlockNodeMetricBlock)),
			DocumentID: documentID,
			BlockUID:   "nope",
			Err:        ErrBlockNotFound,
		},
		"The branch belongs to another document": {
			Doc:        docWith(xid.New(), blockOfType(document.BlockNodeMetricBlock)),
			DocumentID: documentID,
			BlockUID:   _blockUID,
			Err:        ErrBlockNotFound,
		},
		"The block kind has nothing to run": {
			Doc:        docWith(documentID, blockOfType(document.BlockNodeParagraph)),
			DocumentID: documentID,
			BlockUID:   _blockUID,
			Err:        ErrNotRunnable,
		},
		"The branch cannot be read": {
			DocErr:     true,
			DocumentID: documentID,
			BlockUID:   _blockUID,
			Err:        assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sim := &MetricSimulationMock{
				CheckFunc: func(context.Context, xid.ID, xid.ID, document.Block, string) (simulation.Result, error) {
					return c.SimResult, c.SimErr
				},
			}
			db := &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					if c.DocErr {
						return nil, assert.AnError
					}

					return c.Doc, nil
				},
			}

			res, err := NewRunner(db, sim).Run(
				context.Background(),
				c.DocumentID,
				branchID,
				c.BlockUID,
				_organizationID,
			)

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err != nil {
				assert.Nil(t, res)
			} else {
				assert.Equal(t, c.Expected, res)
			}

			require.Len(t, sim.CheckCalls(), c.SimCalls)

			if c.SimCalls > 0 {
				call := sim.CheckCalls()[0]

				assert.Equal(t, c.DocumentID, call.DocumentID)
				assert.Equal(t, branchID, call.BranchID)
				assert.Equal(t, _organizationID, call.OrganizationID)
				assert.Equal(t, blockOfType(document.BlockNodeMetricBlock), call.Block)
			}

			// the branch is read exactly once, whatever the block is
			require.Len(t, db.FetchDocumentByBranchIDCalls(), 1)
			assert.Equal(t, branchID, db.FetchDocumentByBranchIDCalls()[0].BranchID)
			assert.Equal(t, _organizationID, db.FetchDocumentByBranchIDCalls()[0].OrganizationID)
		})
	}
}
