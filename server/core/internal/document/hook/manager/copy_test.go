package manager

import (
	"context"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_CopyHooks(t *testing.T) {
	t.Parallel()

	fromBranchID, toBranchID, documentID := xid.New(), xid.New(), xid.New()

	cc := map[string]struct {
		UIDs      map[string]string
		FetchErr  error
		InsertErr error
		// Blocks are the block ids of the inserted copies, in order.
		Blocks []null.String
		Err    error
	}{
		"Error returned by tx.FetchDocumentHooksByBranchID": {
			FetchErr: assert.AnError,
			Err:      assert.AnError,
		},
		"Error returned by tx.InsertDocumentHook": {
			InsertErr: assert.AnError,
			Blocks:    []null.String{null.StringFrom("old-uid")},
			Err:       assert.AnError,
		},
		"Fork or merge keeps block ids": {
			Blocks: []null.String{null.StringFrom("old-uid"), {}, null.StringFrom("gone-uid")},
		},
		// a duplicated branch carries fresh block uids, so a hook anchored
		// to a block follows the map, a document-level hook is copied as it
		// is, and a hook whose block the map does not name is dropped.
		"Duplicate re-anchors through the uid map": {
			UIDs:   map[string]string{"old-uid": "new-uid"},
			Blocks: []null.String{null.StringFrom("new-uid"), {}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			block := stubHook(t, fromBranchID, time.Now().Add(time.Hour), time.Now())
			block.BlockID = null.StringFrom("old-uid")

			doc := stubHook(t, fromBranchID, time.Now().Add(time.Hour), time.Now())

			gone := stubHook(t, fromBranchID, time.Now().Add(time.Hour), time.Now())
			gone.BlockID = null.StringFrom("gone-uid")

			tx := &CopyTxMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hook.Hook, error) {
					return []hook.Hook{block, doc, gone}, c.FetchErr
				},
				InsertDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.InsertErr
				},
			}

			man := newTestManager(t, &DBMock{}, &fakePublisher{}, nil)

			err := man.CopyHooks(context.Background(), tx, fromBranchID, toBranchID, documentID, "org-1", c.UIDs)
			testutil.AssertEqualError(t, c.Err, err)

			if c.FetchErr == nil {
				ff := tx.FetchDocumentHooksByBranchIDCalls()
				require.Len(t, ff, 1)
				assert.Equal(t, fromBranchID, ff[0].BranchID)
				assert.Equal(t, "org-1", ff[0].OrganizationID)
			}

			ii := tx.InsertDocumentHookCalls()
			require.Len(t, ii, len(c.Blocks))

			for i, call := range ii {
				assert.Equal(t, c.Blocks[i], call.Hk.BlockID)
				assert.Equal(t, null.ValueFrom(toBranchID), call.Hk.BranchID)
				assert.Equal(t, null.ValueFrom(documentID), call.Hk.DocumentID)
				assert.False(t, call.Hk.State.Valid)
			}
		})
	}
}
