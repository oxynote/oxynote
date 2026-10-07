package search

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ViewFrom_Validate(t *testing.T) {
	cc := map[string]struct {
		ViewFrom ViewFrom
		Err      error
	}{
		"Empty":   {Err: ErrInvalidViewFrom},
		"Unknown": {ViewFrom: "tree", Err: ErrInvalidViewFrom},
		"All":     {ViewFrom: ViewFromAll},
		"Search":  {ViewFrom: ViewFromSearch},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.ViewFrom.Validate())
		})
	}
}

func Test_ViewFrom_MarshalText(t *testing.T) {
	t.Parallel()

	text, err := ViewFromSearch.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "search", string(text))
}

func Test_ViewFrom_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text     string
		ViewFrom ViewFrom
		Err      error
	}{
		"Unknown":           {Text: "tree", Err: assert.AnError},
		"Known":             {Text: "search", ViewFrom: ViewFromSearch},
		"Padded upper case": {Text: " ALL ", ViewFrom: ViewFromAll},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var vf ViewFrom

			err := vf.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.ViewFrom, vf)
		})
	}
}

func Test_NewRecentResponse(t *testing.T) {
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

	data, err := json.Marshal(NewRecentResponse(nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"results": []}`, string(data))

	data, err = json.Marshal(NewRecentResponse([]RecentDocument{{
		Document: doc,
		ViewedAt: time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC),
	}}))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"results": [
			{
				"id": "`+doc.ID.String()+`",
				"title": "Shipments",
				"titleHtml": null,
				"icon": "truck",
				"branch": {"id": "`+doc.BranchID.String()+`", "name": "main", "default": true},
				"updatedAt": "2026-10-04T09:12:41Z",
				"updatedBy": "usr-1",
				"viewedAt": "2026-10-07T08:00:00Z"
			}
		]
	}`, string(data))
}
