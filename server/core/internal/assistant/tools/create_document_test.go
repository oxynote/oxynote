package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_createDocument_InvokableRun(t *testing.T) {
	parentID := xid.New()

	type tcase struct {
		DB          *DBMock
		Tx          *TxMock
		Tree        *TreeNotifierMock
		Args        string
		Icon        string
		NotifyCalls int
		Commits     int
		Err         error
	}

	// the writes run in one transaction, so the mock hands the tool a Tx the
	// same way the db package does.
	okDB := func(tx *TxMock, checkErr, beginErr error) *DBMock {
		return &DBMock{
			CheckDocumentExistsFunc: func(_ context.Context, _ xid.ID, _ string) error {
				return checkErr
			},
			BeginTxFunc: func(_ context.Context, dest any) error {
				if beginErr != nil {
					return beginErr
				}

				reflect.ValueOf(dest).
					Elem().
					Set(reflect.ValueOf(tx))

				return nil
			},
		}
	}

	okTx := func(insertErr, upsertErr, jobErr, commitErr error) *TxMock {
		return &TxMock{
			InsertDocumentFunc: func(_ context.Context, _ document.Document) error {
				return insertErr
			},
			UpsertDocumentMaintainersFunc: func(_ context.Context, _ xid.ID, _ string, _ []string) error {
				return upsertErr
			},
			InsertDocumentSearchJobFunc: func(_ context.Context, _ search.BlocksDifference) error {
				return jobErr
			},
			CommitFunc: func() error {
				return commitErr
			},
		}
	}

	cc := map[string]tcase{
		"Malformed args": {
			DB:   &DBMock{},
			Args: `{broken`,
			Err:  assert.AnError,
		},
		"Missing name": {
			DB:   &DBMock{},
			Args: `{}`,
			Err:  assert.AnError,
		},
		"Invalid parent id": {
			DB:   &DBMock{},
			Args: `{"name":"Doc","parent_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.CheckDocumentExists": func() tcase {
			tx := okTx(nil, nil, nil, nil)

			return tcase{
				DB:   okDB(tx, assert.AnError, nil),
				Tx:   tx,
				Args: `{"name":"Doc","parent_id":"` + parentID.String() + `"}`,
				Err:  assert.AnError,
			}
		}(),
		"Error returned by db.BeginTx": func() tcase {
			tx := okTx(nil, nil, nil, nil)

			return tcase{
				DB:   okDB(tx, nil, assert.AnError),
				Tx:   tx,
				Args: `{"name":"Doc"}`,
				Err:  assert.AnError,
			}
		}(),
		"Error returned by Tx.InsertDocument": func() tcase {
			tx := okTx(assert.AnError, nil, nil, nil)

			return tcase{
				DB:   okDB(tx, nil, nil),
				Tx:   tx,
				Args: `{"name":"Doc"}`,
				Err:  assert.AnError,
			}
		}(),
		"Error returned by Tx.UpsertDocumentMaintainers": func() tcase {
			tx := okTx(nil, assert.AnError, nil, nil)

			return tcase{
				DB:   okDB(tx, nil, nil),
				Tx:   tx,
				Args: `{"name":"Doc"}`,
				Err:  assert.AnError,
			}
		}(),
		"Error returned by Tx.InsertDocumentSearchJob": func() tcase {
			tx := okTx(nil, nil, assert.AnError, nil)

			return tcase{
				DB:   okDB(tx, nil, nil),
				Tx:   tx,
				Args: `{"name":"Doc"}`,
				Err:  assert.AnError,
			}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := okTx(nil, nil, nil, assert.AnError)

			return tcase{
				DB:      okDB(tx, nil, nil),
				Tx:      tx,
				Args:    `{"name":"Doc"}`,
				Commits: 1,
				Err:     assert.AnError,
			}
		}(),
		"Successful creation with default icon": func() tcase {
			tx := okTx(nil, nil, nil, nil)

			return tcase{
				DB:          okDB(tx, nil, nil),
				Tx:          tx,
				Tree:        &TreeNotifierMock{},
				Args:        `{"name":"Doc"}`,
				Icon:        "lucide:file",
				NotifyCalls: 1,
				Commits:     1,
			}
		}(),
		"Successful creation with explicit icon and parent": func() tcase {
			tx := okTx(nil, nil, nil, nil)

			return tcase{
				DB:          okDB(tx, nil, nil),
				Tx:          tx,
				Tree:        &TreeNotifierMock{},
				Args:        `{"name":"Doc","icon":"lucide:cat","parent_id":"` + parentID.String() + `"}`,
				Icon:        "lucide:cat",
				NotifyCalls: 1,
				Commits:     1,
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := stubEditInput(c.DB, nil, c.Tree)

			res, err := (&createDocument{inp}).InvokableRun(context.Background(), c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Tree != nil {
				assert.Len(t, c.Tree.NotifyTreeChangeCalls(), c.NotifyCalls)
			}

			// the rollback is deferred, so every case that opened the
			// transaction runs it — harmlessly after a commit.
			if c.Tx != nil && len(c.Tx.InsertDocumentCalls()) > 0 {
				assert.Len(t, c.Tx.CommitCalls(), c.Commits)
				assert.NotEmpty(t, c.Tx.RollbackCalls())
			}

			if err != nil {
				return
			}

			ff := c.Tx.InsertDocumentCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Icon, ff[0].Doc.Icon)
			assert.Equal(t, "Doc", ff[0].Doc.DocumentName)

			var out struct {
				DocumentID string `json:"document_id"`
				BranchID   string `json:"branch_id"`
			}

			require.NoError(t, json.Unmarshal([]byte(res), &out))
			assert.NotEmpty(t, out.DocumentID)
			assert.NotEmpty(t, out.BranchID)
		})
	}
}
