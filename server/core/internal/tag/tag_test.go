package tag

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_CreateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input CreateInput
		Err   error
	}{
		"Empty name": {
			Input: CreateInput{TagName: "", Color: "green"},
			Err:   ErrInvalidTagName,
		},
		"Off-palette colour": {
			Input: CreateInput{TagName: "Production", Color: "navy"},
			Err:   ErrInvalidTagColor,
		},
		"Hex instead of a name": {
			Input: CreateInput{TagName: "Production", Color: "#00a63e"},
			Err:   ErrInvalidTagColor,
		},
		"Palette colour": {
			Input: CreateInput{TagName: "Production", Color: "green"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_UpdateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input UpdateInput
		Err   error
	}{
		"Neither field set": {
			Input: UpdateInput{},
			Err:   ErrEmptyTagUpdate,
		},
		"Empty name": {
			Input: UpdateInput{TagName: null.StringFrom("")},
			Err:   ErrInvalidTagName,
		},
		"Off-palette colour": {
			Input: UpdateInput{Color: null.StringFrom("navy")},
			Err:   ErrInvalidTagColor,
		},
		"Empty colour": {
			Input: UpdateInput{Color: null.StringFrom("")},
			Err:   ErrInvalidTagColor,
		},
		"Name only": {
			Input: UpdateInput{TagName: null.StringFrom("Production")},
		},
		"Colour only": {
			Input: UpdateInput{Color: null.StringFrom("green")},
		},
		"Both fields": {
			Input: UpdateInput{TagName: null.StringFrom("Production"), Color: null.StringFrom("green")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_NewTag(t *testing.T) {
	t.Parallel()

	tg := NewTag(CreateInput{TagName: "Production", Color: "green"}, "org1", "u1")

	assert.False(t, tg.ID.IsNil())
	assert.Equal(t, "org1", tg.OrganizationID)
	assert.Equal(t, "Production", tg.TagName)
	assert.Equal(t, "green", tg.Color)
	assert.Equal(t, "u1", tg.CreatedBy.String)
	assert.False(t, tg.CreatedAt.IsZero())
	assert.Zero(t, tg.SortIndex)
}

func Test_Tag_Summary(t *testing.T) {
	t.Parallel()

	id := xid.New()
	tg := Tag{ID: id, OrganizationID: "org1", TagName: "Production", Color: "green", SortIndex: 3}

	assert.Equal(t, Summary{ID: id, TagName: "Production", Color: "green"}, tg.Summary())
}

func Test_Summaries_Swap(t *testing.T) {
	t.Parallel()

	idA, idB, idC := xid.New(), xid.New(), xid.New()

	cc := map[string]struct {
		ID        xid.ID
		SortIndex int
		Err       error
		Order     []xid.ID
	}{
		"Sort index below the tree": {
			ID:        idA,
			SortIndex: -1,
			Err: errutil.New(
				400,
				"tag_summary.invalid_sort_index",
				"sort index is out of range",
			),
		},
		"Sort index beyond the tree": {
			ID:        idA,
			SortIndex: 3,
			Err: errutil.New(
				400,
				"tag_summary.invalid_sort_index",
				"sort index is out of range",
			),
		},
		"Unknown tag": {
			ID:        xid.New(),
			SortIndex: 0,
			Err:       errutil.ErrNotFound,
		},
		"Move to the top": {
			ID:        idC,
			SortIndex: 0,
			Order:     []xid.ID{idC, idA, idB},
		},
		"Move to the bottom": {
			ID:        idA,
			SortIndex: 2,
			Order:     []xid.ID{idB, idC, idA},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			original := Summaries{{ID: idA}, {ID: idB}, {ID: idC}}

			swapped, err := original.Swap(c.ID, c.SortIndex)

			if c.Err != nil {
				assert.Equal(t, c.Err, err)
				assert.Nil(t, swapped)

				return
			}

			require.NoError(t, err)

			got := make([]xid.ID, 0, len(swapped))
			for _, s := range swapped {
				got = append(got, s.ID)
			}

			assert.Equal(t, c.Order, got)
			// the receiver is left untouched
			assert.Equal(t, []xid.ID{idA, idB, idC}, []xid.ID{original[0].ID, original[1].ID, original[2].ID})
		})
	}
}
