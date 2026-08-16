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

			err := db.UpsertDocumentMaintainers(context.Background(), c.DocumentID, c.OrganizationID, c.MaintainerIDs)
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
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentMaintainers(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no maintainers
	res, err = db.FetchDocumentMaintainers(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]
	users := prepUsers(t, db, 2)

	require.NoError(t, db.UpsertDocumentMaintainers(context.Background(), doc.ID, doc.OrganizationID, users))

	res, err = db.FetchDocumentMaintainers(context.Background(), doc.ID, doc.OrganizationID)
	assert.NoError(t, err)
	assert.ElementsMatch(t, users, res)
}

func Test_agent_UpsertDocumentMaintainers_emptyList(t *testing.T) {
	t.Parallel()

	db := prepTempDB(t)
	doc := prepDocuments(t, db, 1, nil)[0]

	// squirrel refuses an INSERT with no VALUES and MustSql panics on it, so
	// an empty list has to short-circuit.
	assert.NotPanics(t, func() {
		err := db.UpsertDocumentMaintainers(context.Background(), doc.ID, doc.OrganizationID, nil)
		assert.NoError(t, err)
	})
}
