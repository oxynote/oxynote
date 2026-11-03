package tag

import (
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  CreateInput
		Result Tag
		Err    error
	}{
		"Invalid input": {
			Input: CreateInput{Color: "green"},
			Err:   ErrInvalidTagName,
		},
		"Successful creation": {
			Input: CreateInput{TagName: " Production ", Color: "green"},
			Result: Tag{
				OrganizationID: "org1",
				TagName:        "Production",
				Color:          "green",
				CreatedBy:      null.StringFrom("u1"),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := NewTag(c.Input, "org1", "u1")

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.False(t, res.ID.IsNil())
			assert.False(t, res.CreatedAt.IsZero())
			testutil.AssertFilterEqual(t, c.Result, res, xid.ID{}, time.Time{})
		})
	}
}

func Test_Tag_Summary(t *testing.T) {
	t.Parallel()

	id := xid.New()
	tg := Tag{ID: id, OrganizationID: "org1", TagName: "Production", Color: "green", SortIndex: 3}

	assert.Equal(t, Summary{ID: id, TagName: "Production", Color: "green"}, tg.Summary())
}

func Test_Tag_ApplyUpdate(t *testing.T) {
	t.Parallel()

	tg := Tag{ID: xid.New(), OrganizationID: "org1", TagName: "Production", Color: "green", SortIndex: 3}

	cc := map[string]struct {
		Input  UpdateInput
		Result Tag
		Err    error
	}{
		"Invalid input": {
			Input: UpdateInput{},
			Err:   ErrEmptyTagUpdate,
		},
		"Name only": {
			Input: UpdateInput{TagName: null.StringFrom(" Staging ")},
			Result: Tag{
				ID:             tg.ID,
				OrganizationID: "org1",
				TagName:        "Staging",
				Color:          "green",
				SortIndex:      3,
			},
		},
		"Colour only": {
			Input: UpdateInput{Color: null.StringFrom("red")},
			Result: Tag{
				ID:             tg.ID,
				OrganizationID: "org1",
				TagName:        "Production",
				Color:          "red",
				SortIndex:      3,
			},
		},
		"Both fields": {
			Input: UpdateInput{TagName: null.StringFrom("Staging"), Color: null.StringFrom("red")},
			Result: Tag{
				ID:             tg.ID,
				OrganizationID: "org1",
				TagName:        "Staging",
				Color:          "red",
				SortIndex:      3,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := tg.ApplyUpdate(c.Input)

			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, "Production", tg.TagName)
		})
	}
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
		"Name of white space only": {
			Input: CreateInput{TagName: " \t\n ", Color: "green"},
			Err:   ErrInvalidTagName,
		},
		"Name over the length limit": {
			Input: CreateInput{TagName: strings.Repeat("é", _maxTagNameLength+1), Color: "green"},
			Err:   ErrTagNameTooLong,
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
		"Name at the limit once trimmed": {
			Input: CreateInput{TagName: " " + strings.Repeat("é", _maxTagNameLength) + " ", Color: "green"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_CreateInput_sanitize(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  CreateInput
		Result CreateInput
	}{
		"Name without outer white space": {
			Input:  CreateInput{TagName: "Production", Color: "green"},
			Result: CreateInput{TagName: "Production", Color: "green"},
		},
		"Name with outer white space": {
			Input:  CreateInput{TagName: " \tProduction\n ", Color: "green"},
			Result: CreateInput{TagName: "Production", Color: "green"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Input.sanitize())
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
		"Name of white space only": {
			Input: UpdateInput{TagName: null.StringFrom("   ")},
			Err:   ErrInvalidTagName,
		},
		"Name over the length limit": {
			Input: UpdateInput{TagName: null.StringFrom(strings.Repeat("é", _maxTagNameLength+1))},
			Err:   ErrTagNameTooLong,
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
			Input: UpdateInput{TagName: null.StringFrom(" Production\t")},
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

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_UpdateInput_sanitize(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  UpdateInput
		Result UpdateInput
	}{
		"Name unset": {
			Input:  UpdateInput{Color: null.StringFrom("green")},
			Result: UpdateInput{Color: null.StringFrom("green")},
		},
		"Name with outer white space": {
			Input:  UpdateInput{TagName: null.StringFrom(" Production\t"), Color: null.StringFrom("green")},
			Result: UpdateInput{TagName: null.StringFrom("Production"), Color: null.StringFrom("green")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Input.sanitize())
		})
	}
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

func Test_validateTagName(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name string
		Err  error
	}{
		"Empty name": {
			Err: ErrInvalidTagName,
		},
		"Name over the length limit": {
			Name: strings.Repeat("a", _maxTagNameLength+1),
			Err:  ErrTagNameTooLong,
		},
		"Name at the length limit": {
			Name: strings.Repeat("a", _maxTagNameLength),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, validateTagName(c.Name))
		})
	}
}
