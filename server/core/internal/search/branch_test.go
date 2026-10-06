package search

import (
	"context"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BranchQuery_fingerprint(t *testing.T) {
	t.Parallel()

	bq := BranchQuery{
		OrganizationID: "org-1",
		BranchID:       xid.New(),
		Query:          "shipment",
		Limit:          5,
		PageToken:      "token",
	}

	branch := bq
	branch.BranchID = xid.New()

	other := bq
	other.Query = "shipments"

	// the organization, limit and token do not change the order a token
	// points into.
	same := bq
	same.OrganizationID = "org-2"
	same.Limit = 10
	same.PageToken = ""

	assert.NotEmpty(t, bq.fingerprint())
	assert.Equal(t, bq.fingerprint(), same.fingerprint())
	assert.NotEqual(t, bq.fingerprint(), branch.fingerprint())
	assert.NotEqual(t, bq.fingerprint(), other.fingerprint())
}

func Test_Index_SearchBranch(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, indexPath(t), &stubSource{})
	t.Cleanup(func() { require.NoError(t, idx.Close()) })

	main := Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	fork := main
	fork.BranchID = xid.New()
	fork.BranchName = "draft"
	fork.BranchDefault = false

	other := Scope{
		OrganizationID: "org-2",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	for branchID, entries := range map[xid.ID]map[string]Block{
		main.BranchID: {
			"docname": main.Block("docname", "document", "Shipments"),
			"h1":      main.Block("h1", "heading", "Shipment tracking"),
			"p1":      main.Block("p1", "paragraph", "a shipment is created"),
			"p2":      main.Block("p2", "paragraph", "every shipment has a label & a barcode"),
			"p3":      main.Block("p3", "paragraph", "each shipment arrives with a shipment note"),
			"p4":      main.Block("p4", "paragraph", "unrelated text"),
		},
		fork.BranchID: {
			"p1": fork.Block("p1", "paragraph", "a shipment is created"),
		},
		other.BranchID: {
			"p1": other.Block("p1", "paragraph", "shipment"),
		},
	} {
		require.NoError(t, idx.ReplaceBranch(context.Background(), branchID, entries))
	}

	query := BranchQuery{
		OrganizationID: "org-1",
		BranchID:       main.BranchID,
		Query:          "shipment",
	}

	all, err := idx.SearchBranch(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, all.Hits, 4)

	// with returns the query with the limit and the page token of the
	// offset.
	with := func(limit, offset int) BranchQuery {
		bq := query
		bq.Limit = limit

		if offset != 0 {
			bq.PageToken = encodePageToken(offset, query.fingerprint())
		}

		return bq
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	type tcase struct {
		Context context.Context
		Query   BranchQuery
		Result  BranchPage
		Err     error
	}

	cc := map[string]tcase{
		"Invalid query": {
			Query: BranchQuery{OrganizationID: "org-1", BranchID: main.BranchID},
			Err:   ErrInvalidQuery,
		},
		"Invalid page token": {
			Query: BranchQuery{OrganizationID: "org-1", BranchID: main.BranchID, Query: "shipment", PageToken: "%%%"},
			Err:   ErrInvalidPageToken,
		},
		"Page token of another branch": func() tcase {
			bq := query
			bq.PageToken = encodePageToken(1, BranchQuery{BranchID: fork.BranchID, Query: "shipment"}.fingerprint())

			return tcase{Query: bq, Err: ErrInvalidPageToken}
		}(),
		"Error returned by the index": {
			Context: cancelled,
			Query:   query,
			Err:     assert.AnError,
		},
		"No matches": func() tcase {
			bq := query
			bq.Query = "zebra"

			return tcase{Query: bq, Result: BranchPage{Hits: []Block{}}}
		}(),
		"Entries of another organization are never returned": func() tcase {
			bq := query
			bq.BranchID = other.BranchID

			return tcase{Query: bq, Result: BranchPage{Hits: []Block{}}}
		}(),
		"First page": {
			Query: with(2, 0),
			Result: BranchPage{
				Hits:          all.Hits[:2],
				TotalHits:     4,
				NextPageToken: null.StringFrom(encodePageToken(2, query.fingerprint())),
			},
		},
		"Last page": {
			Query: with(2, 2),
			Result: BranchPage{
				Hits:      all.Hits[2:],
				TotalHits: 4,
			},
		},
		"Page past the end": {
			Query: with(2, 10),
			Result: BranchPage{
				Hits:      []Block{},
				TotalHits: 4,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx := c.Context
			if ctx == nil {
				ctx = context.Background()
			}

			res, err := idx.SearchBranch(ctx, c.Query)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}

	t.Run("Continues the hits of a group", func(t *testing.T) {
		t.Parallel()

		gp, err := idx.SearchGroups(context.Background(), GroupQuery{
			OrganizationID: "org-1",
			Query:          "shipment",
			HitsLimit:      2,
		})
		require.NoError(t, err)

		var group Group

		for _, g := range gp.Groups {
			if g.BranchID == main.BranchID {
				group = g
			}
		}

		require.True(t, group.NextHitsToken.Valid)

		bq := query
		bq.PageToken = group.NextHitsToken.String

		res, err := idx.SearchBranch(context.Background(), bq)
		require.NoError(t, err)

		assert.Equal(t, all.Hits, append(group.Hits, res.Hits...))
	})
}

func Test_BranchLimit(t *testing.T) {
	cc := map[string]struct {
		Limit  int
		Result int
	}{
		"No limit":          {Limit: 0, Result: BranchLimitDefault},
		"Negative limit":    {Limit: -1, Result: BranchLimitDefault},
		"Limit within caps": {Limit: 5, Result: 5},
		"Limit over the cap": {
			Limit:  BranchLimitMax + 1,
			Result: BranchLimitMax,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, BranchLimit(c.Limit))
		})
	}
}
