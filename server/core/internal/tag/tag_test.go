package tag

import (
	"testing"

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
			Input: CreateInput{TagName: "", Color: "#22c55e"},
			Err:   ErrInvalidTagName,
		},
		"Colour without a hash": {
			Input: CreateInput{TagName: "Production", Color: "22c55e"},
			Err:   ErrInvalidTagColor,
		},
		"Colour of the wrong length": {
			Input: CreateInput{TagName: "Production", Color: "#22c5"},
			Err:   ErrInvalidTagColor,
		},
		"Colour with a non-hex digit": {
			Input: CreateInput{TagName: "Production", Color: "#22c55z"},
			Err:   ErrInvalidTagColor,
		},
		"Lowercase hex colour": {
			Input: CreateInput{TagName: "Production", Color: "#22c55e"},
		},
		"Uppercase hex colour": {
			Input: CreateInput{TagName: "Production", Color: "#22C55E"},
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

	tg := NewTag(CreateInput{TagName: "Production", Color: "#22c55e"}, "org1", "u1")

	assert.False(t, tg.ID.IsNil())
	assert.Equal(t, "org1", tg.OrganizationID)
	assert.Equal(t, "Production", tg.TagName)
	assert.Equal(t, "#22c55e", tg.Color)
	assert.Equal(t, "u1", tg.CreatedBy.String)
	assert.False(t, tg.CreatedAt.IsZero())
	assert.Zero(t, tg.SortIndex)
}

func Test_Summaries_Swap(t *testing.T) {
	t.Parallel()

	idA, idB, idC := xid.New(), xid.New(), xid.New()

	tree := func() Summaries {
		return Summaries{{ID: idA}, {ID: idB}, {ID: idC}}
	}

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

			original := tree()

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
