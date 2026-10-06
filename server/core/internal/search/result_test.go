package search

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewResponse(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 10, 4, 9, 12, 41, 0, time.UTC)

	doc := document.Document{
		ID:            xid.New(),
		BranchID:      xid.New(),
		BranchName:    "main",
		DocumentName:  "Shipments",
		Icon:          "truck",
		Default:       true,
		UpdatedAt:     updatedAt,
		LastUpdatedBy: null.StringFrom("usr-1"),
	}

	fork := doc
	fork.BranchID = xid.New()
	fork.BranchName = "draft"
	fork.Default = false
	fork.LastUpdatedBy = null.String{}

	scope := Scope{
		OrganizationID: "org-1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
	}

	chart := scope.Block("m1", "metricBlock", "<mark>Shipments</mark> per minute")
	chart.Attrs = map[string]string{"dataSourceId": "ds-1", "visualizationType": "timeseries"}

	code := scope.Block("c1", "codeBlock", "POST /v1/<mark>shipments</mark>")
	code.Attrs = map[string]string{"language": "http"}

	page := GroupPage{
		Groups: []Group{
			{
				DocumentID: doc.ID,
				BranchID:   doc.BranchID,
				Title:      null.StringFrom("<mark>Shipments</mark>"),
				Hits: []Block{
					scope.Block("h1", "heading", "Create a <mark>shipment</mark>"),
					chart,
					code,
				},
				TotalHits:     6,
				NextHitsToken: null.StringFrom("more"),
			},
			// a branch without a row is left out.
			{
				DocumentID: doc.ID,
				BranchID:   xid.New(),
				Hits:       []Block{},
			},
			{
				DocumentID: fork.ID,
				BranchID:   fork.BranchID,
				Hits:       []Block{},
			},
		},
		TotalHits:      5,
		TotalDocuments: 3,
		Capped:         true,
		NextPageToken:  null.StringFrom("next"),
	}

	res := NewResponse(page, []document.Document{fork, doc})

	data, err := json.Marshal(res)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"total": {"hits": 5, "documents": 3, "capped": true},
		"nextToken": "next",
		"results": [
			{
				"document": {
					"id": "`+doc.ID.String()+`",
					"title": "Shipments",
					"titleHtml": "<mark>Shipments</mark>",
					"icon": "truck",
					"branch": {"id": "`+doc.BranchID.String()+`", "name": "main", "default": true},
					"updatedAt": "2026-10-04T09:12:41Z",
					"updatedBy": "usr-1"
				},
				"hits": [
					{"id": "h1", "type": "heading", "text": "Create a <mark>shipment</mark>"},
					{
						"id": "m1",
						"type": "metricBlock",
						"text": "<mark>Shipments</mark> per minute",
						"attrs": {"dataSourceId": "ds-1", "visualizationType": "timeseries"}
					},
					{
						"id": "c1",
						"type": "codeBlock",
						"text": "POST /v1/<mark>shipments</mark>",
						"attrs": {"language": "http"}
					}
				],
				"totalHits": 6,
				"nextHitsToken": "more"
			},
			{
				"document": {
					"id": "`+fork.ID.String()+`",
					"title": "Shipments",
					"titleHtml": null,
					"icon": "truck",
					"branch": {"id": "`+fork.BranchID.String()+`", "name": "draft", "default": false},
					"updatedAt": "2026-10-04T09:12:41Z",
					"updatedBy": null
				},
				"hits": [],
				"totalHits": 0,
				"nextHitsToken": null
			}
		]
	}`, string(data))
}

func Test_NewRecentResponse(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 10, 4, 9, 12, 41, 0, time.UTC)

	doc := document.Document{
		ID:            xid.New(),
		BranchID:      xid.New(),
		BranchName:    "main",
		DocumentName:  "Shipments",
		Icon:          "truck",
		Default:       true,
		UpdatedAt:     updatedAt,
		LastUpdatedBy: null.StringFrom("usr-1"),
	}

	data, err := json.Marshal(NewRecentResponse(nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"total": {"hits": 0, "documents": 0, "capped": false}, "nextToken": null, "results": []}`, string(data))

	data, err = json.Marshal(NewRecentResponse([]document.Document{doc}))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"total": {"hits": 0, "documents": 1, "capped": false},
		"nextToken": null,
		"results": [
			{
				"document": {
					"id": "`+doc.ID.String()+`",
					"title": "Shipments",
					"titleHtml": null,
					"icon": "truck",
					"branch": {"id": "`+doc.BranchID.String()+`", "name": "main", "default": true},
					"updatedAt": "2026-10-04T09:12:41Z",
					"updatedBy": "usr-1"
				},
				"hits": [],
				"totalHits": 0,
				"nextHitsToken": null
			}
		]
	}`, string(data))
}

func Test_NewBranchResponse(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	chart := scope.Block("m1", "metricBlock", "<mark>Lag</mark>")
	chart.Attrs = map[string]string{"dataSourceId": "ds-1", "visualizationType": "bar"}

	data, err := json.Marshal(NewBranchResponse(BranchPage{}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"totalHits": 0, "nextToken": null, "hits": []}`, string(data))

	data, err = json.Marshal(NewBranchResponse(BranchPage{
		Hits: []Block{
			scope.Block("p1", "paragraph", "<mark>lag</mark> grows"),
			chart,
		},
		TotalHits:     7,
		NextPageToken: null.StringFrom("next"),
	}))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"totalHits": 7,
		"nextToken": "next",
		"hits": [
			{"id": "p1", "type": "paragraph", "text": "<mark>lag</mark> grows"},
			{
				"id": "m1",
				"type": "metricBlock",
				"text": "<mark>Lag</mark>",
				"attrs": {"dataSourceId": "ds-1", "visualizationType": "bar"}
			}
		]
	}`, string(data))
}

func Test_newResultDocument(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		ID:            xid.New(),
		BranchID:      xid.New(),
		BranchName:    "main",
		DocumentName:  "Shipments",
		Icon:          "truck",
		Default:       true,
		UpdatedAt:     time.Date(2026, 10, 4, 9, 12, 41, 0, time.UTC),
		LastUpdatedBy: null.StringFrom("usr-1"),
	}

	assert.Equal(t, ResultDocument{
		ID:    doc.ID,
		Title: "Shipments",
		Icon:  "truck",
		Branch: ResultBranch{
			ID:      doc.BranchID,
			Name:    "main",
			Default: true,
		},
		UpdatedAt: doc.UpdatedAt,
		UpdatedBy: null.StringFrom("usr-1"),
	}, newResultDocument(doc))
}

func Test_newResultHit(t *testing.T) {
	t.Parallel()

	scope := stubScope()

	assert.Equal(t, ResultHit{ID: "p1", Type: "paragraph", Text: "hello"}, newResultHit(scope.Block("p1", "paragraph", "hello")))

	code := scope.Block("c1", "codeBlock", "echo")
	code.Attrs = map[string]string{"language": "bash"}

	assert.Equal(t, ResultHit{
		ID:    "c1",
		Type:  "codeBlock",
		Text:  "echo",
		Attrs: map[string]string{"language": "bash"},
	}, newResultHit(code))
}
