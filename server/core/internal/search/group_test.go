package search

import (
	"context"
	"encoding/base64"
	"log/slog"
	"testing"

	bleveSearch "github.com/blevesearch/bleve/v2/search"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GroupQuery_fingerprint(t *testing.T) {
	t.Parallel()

	gq := GroupQuery{
		OrganizationID: "org-1",
		Query:          "shipment",
		Limit:          5,
		PageToken:      "token",
	}

	pinned := gq
	pinned.CurrentDocumentID = null.ValueFrom(xid.New())

	other := gq
	other.Query = "shipments"

	// the organization, limit and token do not change the order a token
	// points into.
	same := gq
	same.OrganizationID = "org-2"
	same.Limit = 10
	same.PageToken = ""

	assert.NotEmpty(t, gq.fingerprint())
	assert.Equal(t, gq.fingerprint(), same.fingerprint())
	assert.NotEqual(t, gq.fingerprint(), pinned.fingerprint())
	assert.NotEqual(t, gq.fingerprint(), other.fingerprint())
}

func Test_GroupPage_BranchIDs(t *testing.T) {
	t.Parallel()

	b1, b2 := xid.New(), xid.New()

	assert.Empty(t, GroupPage{}.BranchIDs())
	assert.Equal(t, []xid.ID{b1, b2}, GroupPage{
		Groups: []Group{
			{BranchID: b1},
			{BranchID: b2},
		},
	}.BranchIDs())
}

func Test_GroupPage_DataSourceIDs(t *testing.T) {
	t.Parallel()

	assert.Empty(t, GroupPage{}.DataSourceIDs())
	assert.Equal(t, []string{"ds-1", "ds-2"}, GroupPage{
		Groups: []Group{
			{Hits: []Block{{Attrs: map[string]string{"dataSourceId": "ds-1"}}, {}, {Attrs: map[string]string{"dataSourceId": "ds-2"}}}},
			{Hits: []Block{{Attrs: map[string]string{"dataSourceId": "ds-1"}}}},
		},
	}.DataSourceIDs())
}

func Test_Index_SearchGroups(t *testing.T) {
	t.Parallel()

	idx := openIndex(t, indexPath(t), &stubSource{})
	t.Cleanup(func() { require.NoError(t, idx.Close()) })

	shipments := Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	fork := shipments
	fork.BranchID = xid.New()
	fork.BranchName = "draft"

	kafka := Scope{
		OrganizationID: "org-1",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	other := Scope{
		OrganizationID: "org-2",
		DocumentID:     xid.New(),
		BranchID:       xid.New(),
		BranchName:     "main",
		BranchDefault:  true,
	}

	code := shipments.Block("c1", "codeBlock", "POST /v1/shipments")
	code.Attrs = map[string]string{"language": "http"}

	chart := shipments.Block("m1", "metricBlock", "Shipments created per minute")
	chart.Attrs = map[string]string{
		"dataSourceId":      "ds-1",
		"visualizationType": "timeseries",
	}

	forkCode := code
	forkCode.ID = fork.BranchID.String() + "-c1"
	forkCode.BranchID = fork.BranchID
	forkCode.BranchName = fork.BranchName
	forkCode.BranchDefault = false

	for branchID, entries := range map[xid.ID]map[string]Block{
		shipments.BranchID: {
			"docname": shipments.Block("docname", "document", "Shipments"),
			"h1":      shipments.Block("h1", "heading", "Create a shipment"),
			"c1":      code,
			"m1":      chart,
		},
		fork.BranchID: {
			"c1": forkCode,
		},
		kafka.BranchID: {
			"docname": kafka.Block("docname", "document", "Kafka topics"),
			"p1":      kafka.Block("p1", "paragraph", "shipment events"),
			"p2":      kafka.Block("p2", "paragraph", "a11y & <tags>"),
		},
		other.BranchID: {
			"p1": other.Block("p1", "paragraph", "shipment"),
		},
	} {
		require.NoError(t, idx.ReplaceBranch(context.Background(), branchID, entries))
	}

	// mark returns the block with its text replaced by the highlighted one.
	mark := func(b Block, text string) Block {
		b.Text = text

		return b
	}

	shipmentsGroup := Group{
		DocumentID: shipments.DocumentID,
		BranchID:   shipments.BranchID,
		Title:      null.StringFrom("<mark>Shipments</mark>"),
		Hits: []Block{
			mark(shipments.Block("h1", "heading", "Create a shipment"), "Create a <mark>shipment</mark>"),
			mark(code, "POST /v1/<mark>shipments</mark>"),
			mark(chart, "<mark>Shipments</mark> created per minute"),
		},
		TotalHits: 3,
	}

	kafkaGroup := Group{
		DocumentID: kafka.DocumentID,
		BranchID:   kafka.BranchID,
		Hits: []Block{
			mark(kafka.Block("p1", "paragraph", "shipment events"), "<mark>shipment</mark> events"),
		},
		TotalHits: 1,
	}

	forkGroup := Group{
		DocumentID: shipments.DocumentID,
		BranchID:   fork.BranchID,
		Hits: []Block{
			mark(forkCode, "POST /v1/<mark>shipments</mark>"),
		},
		TotalHits: 1,
	}

	// shipmentsGroup cut to its first two hits.
	shipmentsCut := shipmentsGroup
	shipmentsCut.Hits = shipmentsGroup.Hits[:2]
	shipmentsCut.NextHitsToken = null.StringFrom(encodePageToken(2, BranchQuery{
		BranchID: shipments.BranchID,
		Query:    "shipment",
	}.fingerprint()))

	// token returns the page token of the offset for the query.
	token := func(gq GroupQuery, offset int) string {
		return encodePageToken(offset, gq.fingerprint())
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cc := map[string]struct {
		Context context.Context
		Query   GroupQuery
		Result  GroupPage
		Err     error
	}{
		"Invalid query": {
			Query: GroupQuery{OrganizationID: "org-1"},
			Err:   ErrInvalidQuery,
		},
		"Invalid page token": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "shipment", PageToken: "%%%"},
			Err:   ErrInvalidPageToken,
		},
		"Page token of another query": {
			Query: GroupQuery{
				OrganizationID: "org-1",
				Query:          "shipment",
				PageToken:      token(GroupQuery{Query: "kafka"}, 1),
			},
			Err: ErrInvalidPageToken,
		},
		"Error returned by the index": {
			Context: cancelled,
			Query:   GroupQuery{OrganizationID: "org-1", Query: "shipment"},
			Err:     assert.AnError,
		},
		"No matches": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "zebra"},
			Result: GroupPage{
				Groups: []Group{},
			},
		},
		"Successful search": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "shipment"},
			Result: GroupPage{
				Groups:         []Group{shipmentsGroup, kafkaGroup, forkGroup},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"First page": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "shipment", Limit: 2},
			Result: GroupPage{
				Groups:         []Group{shipmentsGroup, kafkaGroup},
				TotalHits:      5,
				TotalDocuments: 3,
				NextPageToken:  null.StringFrom(token(GroupQuery{Query: "shipment"}, 2)),
			},
		},
		"Last page": {
			Query: GroupQuery{
				OrganizationID: "org-1",
				Query:          "shipment",
				Limit:          2,
				PageToken:      token(GroupQuery{Query: "shipment"}, 2),
			},
			Result: GroupPage{
				Groups:         []Group{forkGroup},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"Page past the end": {
			Query: GroupQuery{
				OrganizationID: "org-1",
				Query:          "shipment",
				PageToken:      token(GroupQuery{Query: "shipment"}, 10),
			},
			Result: GroupPage{
				Groups:         []Group{},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"Current document first": {
			Query: GroupQuery{
				OrganizationID:    "org-1",
				Query:             "shipment",
				CurrentDocumentID: null.ValueFrom(kafka.DocumentID),
			},
			Result: GroupPage{
				Groups:         []Group{kafkaGroup, shipmentsGroup, forkGroup},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"Current document keeps its branches in match order": {
			Query: GroupQuery{
				OrganizationID:    "org-1",
				Query:             "shipment",
				CurrentDocumentID: null.ValueFrom(shipments.DocumentID),
			},
			Result: GroupPage{
				Groups:         []Group{shipmentsGroup, forkGroup, kafkaGroup},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"Hits over the group limit": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "shipment", HitsLimit: 2},
			Result: GroupPage{
				Groups:         []Group{shipmentsCut, kafkaGroup, forkGroup},
				TotalHits:      5,
				TotalDocuments: 3,
			},
		},
		"Title match without hits": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "kafka"},
			Result: GroupPage{
				Groups: []Group{
					{
						DocumentID: kafka.DocumentID,
						BranchID:   kafka.BranchID,
						Title:      null.StringFrom("<mark>Kafka</mark> topics"),
						Hits:       []Block{},
					},
				},
				TotalDocuments: 1,
			},
		},
		"Text is escaped around the marks": {
			Query: GroupQuery{OrganizationID: "org-1", Query: "accessibility"},
			Result: GroupPage{
				Groups: []Group{
					{
						DocumentID: kafka.DocumentID,
						BranchID:   kafka.BranchID,
						Hits: []Block{
							mark(kafka.Block("p2", "paragraph", ""), "<mark>a11y</mark> &amp; &lt;tags&gt;"),
						},
						TotalHits: 1,
					},
				},
				TotalHits:      1,
				TotalDocuments: 1,
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

			res, err := idx.SearchGroups(ctx, c.Query)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_GroupLimit(t *testing.T) {
	cc := map[string]struct {
		Limit  int
		Result int
	}{
		"No limit":          {Limit: 0, Result: GroupLimitDefault},
		"Negative limit":    {Limit: -1, Result: GroupLimitDefault},
		"Limit within caps": {Limit: 5, Result: 5},
		"Limit over the cap": {
			Limit:  GroupLimitMax + 1,
			Result: GroupLimitMax,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, GroupLimit(c.Limit))
		})
	}
}

func Test_GroupHitsLimit(t *testing.T) {
	cc := map[string]struct {
		Limit  int
		Result int
	}{
		"No limit":          {Limit: 0, Result: GroupHitsLimitDefault},
		"Negative limit":    {Limit: -1, Result: GroupHitsLimitDefault},
		"Limit within caps": {Limit: 5, Result: 5},
		"Limit over the cap": {
			Limit:  GroupHitsLimitMax + 1,
			Result: GroupHitsLimitMax,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, GroupHitsLimit(c.Limit))
		})
	}
}

func Test_Index_decodeHighlightedHit(t *testing.T) {
	documentID, branchID := xid.New(), xid.New()

	fields := map[string]any{
		_fieldOrganizationID: "org-1",
		_fieldDocumentID:     documentID.String(),
		_fieldBranchID:       branchID.String(),
		_fieldBranchName:     "main",
		_fieldBranchDefault:  true,
		_fieldType:           "paragraph",
		_fieldText:           "a & b",
	}

	block := Block{
		ID:             "e1",
		OrganizationID: "org-1",
		DocumentID:     documentID,
		BranchID:       branchID,
		BranchName:     "main",
		BranchDefault:  true,
		Type:           "paragraph",
	}

	// withText returns the block with the text.
	withText := func(text string) Block {
		b := block
		b.Text = text

		return b
	}

	cc := map[string]struct {
		Hit    *bleveSearch.DocumentMatch
		Result Block
		Err    error
	}{
		"Error returned by decodeHit": {
			Hit: &bleveSearch.DocumentMatch{ID: "e1", Fields: map[string]any{}},
			Err: assert.AnError,
		},
		"Hit without a fragment is escaped": {
			Hit:    &bleveSearch.DocumentMatch{ID: "e1", Fields: fields},
			Result: withText("a &amp; b"),
		},
		"Hit with a fragment": {
			Hit: &bleveSearch.DocumentMatch{
				ID:        "e1",
				Fields:    fields,
				Fragments: bleveSearch.FieldFragmentMap{_fieldText: {"<mark>a</mark> &amp; b"}},
			},
			Result: withText("<mark>a</mark> &amp; b"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			idx := &Index{log: slog.New(slog.DiscardHandler)}

			res, err := idx.decodeHighlightedHit(c.Hit)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_encodePageToken(t *testing.T) {
	t.Parallel()

	token := encodePageToken(30, "fp")

	data, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	assert.JSONEq(t, `{"o":30,"f":"fp"}`, string(data))
}

func Test_decodePageToken(t *testing.T) {
	encode := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}

	cc := map[string]struct {
		Token  string
		Result int
		Err    error
	}{
		"Empty token":         {Token: "", Result: 0},
		"Invalid base64":      {Token: "%%%", Err: ErrInvalidPageToken},
		"Invalid JSON":        {Token: encode("nope"), Err: ErrInvalidPageToken},
		"Negative offset":     {Token: encode(`{"o":-1,"f":"fp"}`), Err: ErrInvalidPageToken},
		"Another fingerprint": {Token: encode(`{"o":1,"f":"other"}`), Err: ErrInvalidPageToken},
		"Valid token":         {Token: encodePageToken(30, "fp"), Result: 30},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := decodePageToken(c.Token, "fp")
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_groupHits(t *testing.T) {
	documentID, branchID, forkID := xid.New(), xid.New(), xid.New()

	// hit builds a match carrying the fields the grouping reads.
	hit := func(id, documentID, branchID, typ string) *bleveSearch.DocumentMatch {
		return &bleveSearch.DocumentMatch{
			ID: id,
			Fields: map[string]any{
				_fieldDocumentID: documentID,
				_fieldBranchID:   branchID,
				_fieldType:       typ,
			},
		}
	}

	cc := map[string]struct {
		Hits   bleveSearch.DocumentMatchCollection
		Result []*groupEntries
		Err    error
	}{
		"Invalid document id": {
			Hits: bleveSearch.DocumentMatchCollection{hit("a", "nope", branchID.String(), "paragraph")},
			Err:  assert.AnError,
		},
		"Invalid branch id": {
			Hits: bleveSearch.DocumentMatchCollection{hit("a", documentID.String(), "nope", "paragraph")},
			Err:  assert.AnError,
		},
		"No hits": {},
		"Hits grouped by branch in first appearance order": {
			Hits: bleveSearch.DocumentMatchCollection{
				hit("a", documentID.String(), branchID.String(), "heading"),
				hit("b", documentID.String(), forkID.String(), "paragraph"),
				hit("c", documentID.String(), branchID.String(), "document"),
				hit("d", documentID.String(), branchID.String(), "paragraph"),
			},
			Result: []*groupEntries{
				{documentID: documentID, branchID: branchID, titleID: "c", hitIDs: []string{"a", "d"}},
				{documentID: documentID, branchID: forkID, hitIDs: []string{"b"}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := groupHits(c.Hits)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}
