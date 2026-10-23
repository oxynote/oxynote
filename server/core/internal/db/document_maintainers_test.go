package db

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_agent_UpsertDocumentMaintainers(t *testing.T) {
	type tcase struct {
		DocumentID     xid.ID
		OrganizationID string
		MaintainerIDs  []string
		Result         []string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent user": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				DocumentID:     doc.ID,
				OrganizationID: doc.OrganizationID,
				MaintainerIDs:  []string{"non-existent-user-id"},
				Err:            assert.AnError,
			}
		},
		"Empty maintainer list is a no-op": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			// squirrel refuses an INSERT with no VALUES and MustSql panics on it, so
			// an empty list has to short-circuit.
			return tcase{
				DocumentID:     doc.ID,
				OrganizationID: doc.OrganizationID,
			}
		},
		"Successful upsert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			return tcase{
				DocumentID:     doc.ID,
				OrganizationID: doc.OrganizationID,
				MaintainerIDs:  users,
				Result:         users,
			}
		},
		"Existing maintainers are left untouched": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			require.NoError(t, db.UpsertDocumentMaintainers(
				context.Background(),
				doc.ID,
				doc.OrganizationID,
				users[:1],
			))

			return tcase{
				DocumentID:     doc.ID,
				OrganizationID: doc.OrganizationID,
				MaintainerIDs:  users,
				Result:         users,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			var err error

			require.NotPanics(t, func() {
				err = db.UpsertDocumentMaintainers(context.Background(), c.DocumentID, c.OrganizationID, c.MaintainerIDs)
			})
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentMaintainers(context.Background(), c.DocumentID, c.OrganizationID)
			require.NoError(t, err)
			assert.ElementsMatch(t, c.Result, res)
		})
	}
}

func Test_agent_FetchDocumentMaintainers(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		DocumentID       xid.ID
		OrganizationID   string
		Result           []string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				DocumentID:       xid.New(),
				OrganizationID:   "org-id",
				Err:              assert.AnError,
			}
		},
		"No maintainers": func(*testing.T, *DB) tcase {
			return tcase{
				DocumentID:     xid.New(),
				OrganizationID: "non-existent-org-id",
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			require.NoError(t, db.UpsertDocumentMaintainers(context.Background(), doc.ID, doc.OrganizationID, users))

			return tcase{
				DocumentID:     doc.ID,
				OrganizationID: doc.OrganizationID,
				Result:         users,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			res, err := db.FetchDocumentMaintainers(ctx, c.DocumentID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			assert.ElementsMatch(t, c.Result, res)
		})
	}
}
