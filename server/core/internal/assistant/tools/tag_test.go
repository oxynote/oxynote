package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tagDeps builds session wiring over the given DB with a tag notifier
// attached, so a test can assert the broadcast a tag write ends with.
func tagDeps(db *DBMock) (*Deps, *TagNotifierMock) {
	d := testDeps(db, nil, nil)
	tags := &TagNotifierMock{}
	d.tags = tags

	return d, tags
}

// stubTagDB answers every tag and document lookup, and accepts every
// tag write; err, when set, is what the tag tree lookup fails with.
func stubTagDB(err error) *DBMock {
	db := stubDocumentDB()

	if err != nil {
		db.FetchTagTreeFunc = func(context.Context, string, string) (tag.Summaries, error) {
			return nil, err
		}
	}

	return db
}

// tagArgs renders the tag_id argument every tag write takes.
func tagArgs(tagID xid.ID) string {
	return `"tag_id":"` + tagID.String() + `"`
}

func Test_listTagsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, listTagsArgs{}, nil)
}

func Test_listTags_Info(t *testing.T) {
	t.Parallel()

	info := listTags{}.Info()

	assert.Equal(t, Traits{}, info.Traits)

	assert.Equal(t, NameListTags, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func Test_listTags_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Error returned by db.FetchTagTree": {
			DB:   stubTagDB(assert.AnError),
			Args: `{}`,
			Err:  assert.AnError,
		},
		// the tool declares no parameters, so a provider may call it with
		// no arguments at all.
		"No arguments": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tag.Summaries, error) {
					return tag.Summaries{}, nil
				},
			},
			Args:   ``,
			Result: `{"tags":[]}`,
		},
		"No tags": {
			DB: &DBMock{
				FetchTagTreeFunc: func(context.Context, string, string) (tag.Summaries, error) {
					return tag.Summaries{}, nil
				},
			},
			Args:   `{}`,
			Result: `{"tags":[]}`,
		},
		// the tree lists the nested children of an assigned document as
		// sidebar context; only the assigned document itself is listed.
		"Tags with their documents and the hidden flag": {
			DB:   stubTagDB(nil),
			Args: `{}`,
			Result: `{"tags":[` +
				`{"id":"` + _testTagID.String() + `","name":"Production","color":"green","hidden":false,` +
				`"documents":[{"id":"` + _testDocID.String() + `","name":"Runbook","default_branch_id":"` + _stubMainBranchID.String() + `"}]},` +
				`{"id":"` + _otherTagID.String() + `","name":"Staging","color":"unknown","hidden":true,"documents":[]}` +
				`]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listTags{}.Execute(testInput(testDeps(c.DB, nil, nil), NameListTags, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_createTagArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, createTagArgs{Name: "Release", ColorName: "blue"}, map[string]Args{
		"name":  createTagArgs{ColorName: "blue"},
		"color": createTagArgs{Name: "Release"},
	})

	// a colour outside the palette is refused in the domain's words.
	assert.Equal(t, tag.ErrInvalidTagColor, createTagArgs{Name: "Release", ColorName: "navy"}.Validate())
}

func Test_createTagArgs_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   createTagArgs
		Result tag.CreateInput
	}{
		"Palette colour": {
			Args:   createTagArgs{Name: "Release", ColorName: "blue"},
			Result: tag.CreateInput{TagName: "Release", Color: "#155dfc"},
		},
		"Colour outside the palette": {
			Args:   createTagArgs{Name: "Release", ColorName: "navy"},
			Result: tag.CreateInput{TagName: "Release"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.input())
		})
	}
}

func Test_createTag_Info(t *testing.T) {
	t.Parallel()

	info := createTag{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameCreateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"name", "color"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Equal(t, tag.ColorNames(), colorEnum(t, info))
}

// colorEnum returns the palette names a tool's schema lists for color.
func colorEnum(t *testing.T, info Info) []string {
	t.Helper()

	prop, ok := info.Properties["color"].(map[string]any)
	require.True(t, ok)

	names, ok := prop["enum"].([]string)
	require.True(t, ok)

	return names
}

func Test_createTag_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	// a tag that does not exist yet has no id to name, and no document.
	got, err := createTag{}.Summary(testInput(d, NameCreateTag, `{"name":"Release","color":"blue"}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{Tool: NameCreateTag, Summary: "Create tag Release (blue)"}, got)

	_, err = createTag{}.Summary(testInput(d, NameCreateTag, `{}`))
	require.Error(t, err)
}

func Test_createTag_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments":     {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Name is required":        {DB: &DBMock{}, Args: `{"color":"blue"}`, Err: assert.AnError},
		"Colour is required":      {DB: &DBMock{}, Args: `{"name":"Release"}`, Err: assert.AnError},
		"Unknown colour":          {DB: &DBMock{}, Args: `{"name":"Release","color":"navy"}`, Err: assert.AnError},
		"Empty name is not a tag": {DB: &DBMock{}, Args: `{"name":"","color":"blue"}`, Err: assert.AnError},
		"Name already in use": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tag.Tag) error {
					return tag.ErrDuplicateTagName
				},
			},
			Args: `{"name":"Release","color":"blue"}`,
			Err:  tag.ErrDuplicateTagName,
		},
		"Error returned by db.InsertTag": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tag.Tag) error {
					return assert.AnError
				},
			},
			Args: `{"name":"Release","color":"blue"}`,
			Err:  assert.AnError,
		},
		"Created": {
			DB:     &DBMock{},
			Args:   `{"name":"Release","color":"blue"}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)

			res, err := createTag{}.Execute(testInput(d, NameCreateTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// the tag is stamped with the session's pair and the
			// arguments, and the result names the id it got.
			ff := c.DB.InsertTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].T.OrganizationID)
			assert.Equal(t, "user", ff[0].T.CreatedBy.String)
			assert.Equal(t, "Release", ff[0].T.TagName)
			assert.Equal(t, "#155dfc", ff[0].T.Color)
			assert.JSONEq(t, `{"tag_id":"`+ff[0].T.ID.String()+`"}`, res)
		})
	}
}

func Test_updateTagArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, updateTagArgs{TagID: _testTagID, Name: "Release"}, map[string]Args{
		"tag_id": updateTagArgs{Name: "Release"},
	})

	testutil.AssertEqualError(t, errors.New("give at least one of name, color or sort_index"), updateTagArgs{TagID: _testTagID}.Validate())
	assert.Equal(t, tag.ErrInvalidTagColor, updateTagArgs{TagID: _testTagID, ColorName: "navy"}.Validate())
	require.NoError(t, updateTagArgs{TagID: _testTagID, ColorName: "blue"}.Validate())

	// the first position is a move like any other.
	require.NoError(t, updateTagArgs{TagID: _testTagID, SortIndex: null.ValueFrom(0)}.Validate())
}

func Test_updateTagArgs_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   updateTagArgs
		Result tag.UpdateInput
	}{
		"Nothing given": {
			Args:   updateTagArgs{TagID: _testTagID},
			Result: tag.UpdateInput{},
		},
		"Name and palette colour": {
			Args:   updateTagArgs{TagID: _testTagID, Name: "Release", ColorName: "blue"},
			Result: tag.UpdateInput{TagName: null.StringFrom("Release"), Color: null.StringFrom("#155dfc")},
		},
		"Colour outside the palette": {
			Args:   updateTagArgs{TagID: _testTagID, ColorName: "navy"},
			Result: tag.UpdateInput{Color: null.StringFrom("")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.input())
		})
	}
}

func Test_updateTag_Info(t *testing.T) {
	t.Parallel()

	info := updateTag{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameUpdateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"tag_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, "sort_index")
	assert.Contains(t, info.Properties, "sort_index")
	assert.Equal(t, tag.ColorNames(), colorEnum(t, info))
}

func Test_updateTag_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Nothing to change":   {DB: stubTagDB(nil), Args: `{` + tagArgs(_testTagID) + `}`, Err: assert.AnError},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_unknownTagID) + `,"name":"Release"}`,
			Err:  assert.AnError,
		},
		"Rename only": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release"}`,
			Result: `Rename Production to "Release"`,
		},
		"Recolour only": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"color":"blue"}`,
			Result: "Set the colour to blue",
		},
		"Rename and recolour": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release","color":"blue"}`,
			Result: `Rename Production to "Release" and set the colour to blue`,
		},
		// the card counts positions from one, the way a person does.
		"Move only": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Result: "Move it to position 2",
		},
		"Rename and move": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release","sort_index":0}`,
			Result: `Rename Production to "Release" and move it to position 1`,
		},
		"Rename, recolour and move": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release","color":"blue","sort_index":0}`,
			Result: `Rename Production to "Release", set the colour to blue and move it to position 1`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateTag{}.Summary(testInput(testDeps(c.DB, nil, nil), NameUpdateTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameUpdateTag, Summary: c.Result}, got)
		})
	}
}

func Test_updateTag_Execute(t *testing.T) {
	t.Parallel()

	failingTree := stubTagDB(nil)
	failingTree.UpdateTagTreeFunc = func(context.Context, tag.Summaries, string) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB      *DBMock
		Args    string
		Notify  int
		Renames int
		Order   []xid.ID
		Result  string
		Err     error
	}{
		"Malformed arguments": {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Tag id is required":  {DB: &DBMock{}, Args: `{"name":"Release"}`, Err: assert.AnError},
		"Nothing to change":   {DB: &DBMock{}, Args: `{` + tagArgs(_testTagID) + `}`, Err: assert.AnError},
		"Unknown colour":      {DB: &DBMock{}, Args: `{` + tagArgs(_testTagID) + `,"color":"navy"}`, Err: assert.AnError},
		"Unknown tag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return errutil.ErrNotFound
				},
			},
			Args:    `{` + tagArgs(_unknownTagID) + `,"name":"Release"}`,
			Renames: 1,
			Err:     fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Error returned by db.UpdateTag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return assert.AnError
				},
			},
			Args:    `{` + tagArgs(_testTagID) + `,"name":"Release"}`,
			Renames: 1,
			Err:     assert.AnError,
		},
		"Renamed": {
			DB:      &DBMock{},
			Args:    `{` + tagArgs(_testTagID) + `,"name":"Release"}`,
			Renames: 1,
			Notify:  1,
			Result:  `{"tag_id":"` + _testTagID.String() + `","name":"Release"}`,
		},
		"Renamed and recoloured": {
			DB:      &DBMock{},
			Args:    `{` + tagArgs(_testTagID) + `,"name":"Release","color":"blue"}`,
			Renames: 1,
			Notify:  1,
			Result:  `{"tag_id":"` + _testTagID.String() + `","name":"Release","color":"blue"}`,
		},
		"Error returned by db.FetchTagTree": {
			DB:   stubTagDB(assert.AnError),
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Err:  assert.AnError,
		},
		"Unknown tag to move": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_unknownTagID) + `,"sort_index":1}`,
			Err:  fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Sort index past the last tag": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":2}`,
			Err:  assert.AnError,
		},
		"Negative sort index": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":-1}`,
			Err:  assert.AnError,
		},
		"Error returned by db.UpdateTagTree": {
			DB:   failingTree,
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Err:  assert.AnError,
		},
		"Moved to the end": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Notify: 1,
			Order:  []xid.ID{_otherTagID, _testTagID},
			Result: `{"tag_id":"` + _testTagID.String() + `","sort_index":1}`,
		},
		"Moved to the front": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_otherTagID) + `,"sort_index":0}`,
			Notify: 1,
			Order:  []xid.ID{_otherTagID, _testTagID},
			Result: `{"tag_id":"` + _otherTagID.String() + `","sort_index":0}`,
		},
		// a position the move would refuse fails the call before the
		// rename is saved.
		"Rename with a sort index past the last tag": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_testTagID) + `,"name":"Release","sort_index":9}`,
			Err:  assert.AnError,
		},
		"Rename while the tags cannot be read": {
			DB:   stubTagDB(assert.AnError),
			Args: `{` + tagArgs(_testTagID) + `,"name":"Release","sort_index":1}`,
			Err:  assert.AnError,
		},
		"Renamed, then the move fails": {
			DB: func() *DBMock {
				db := stubTagDB(nil)
				db.UpdateTagTreeFunc = func(context.Context, tag.Summaries, string) error {
					return assert.AnError
				}

				return db
			}(),
			Args:    `{` + tagArgs(_testTagID) + `,"name":"Release","sort_index":1}`,
			Renames: 1,
			Notify:  1,
			Err:     assert.AnError,
		},
		"Renamed and moved": {
			DB:      stubTagDB(nil),
			Args:    `{` + tagArgs(_testTagID) + `,"name":"Release","sort_index":1}`,
			Renames: 1,
			Notify:  2,
			Order:   []xid.ID{_otherTagID, _testTagID},
			Result:  `{"tag_id":"` + _testTagID.String() + `","name":"Release","sort_index":1}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)

			res, err := updateTag{}.Execute(testInput(d, NameUpdateTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)
			assert.Len(t, c.DB.UpdateTagCalls(), c.Renames)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)

			// a move rewrites the whole tree in its new order.
			if c.Order == nil {
				assert.Empty(t, c.DB.UpdateTagTreeCalls())

				return
			}

			ff := c.DB.UpdateTagTreeCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)

			order := make([]xid.ID, 0, len(ff[0].Tree))
			for _, s := range ff[0].Tree {
				order = append(order, s.ID)
			}

			assert.Equal(t, c.Order, order)
		})
	}
}

func Test_deleteTagArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, deleteTagArgs{TagID: _testTagID}, map[string]Args{
		"tag_id": deleteTagArgs{},
	})
}

func Test_deleteTag_Info(t *testing.T) {
	t.Parallel()

	info := deleteTag{}.Info()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, info.Traits)

	assert.Equal(t, NameDeleteTag, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"tag_id"}, info.Required)
}

func Test_deleteTag_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Error returned by db.FetchTagTree": {
			DB:   stubTagDB(assert.AnError),
			Args: `{` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_unknownTagID) + `}`,
			Err:  assert.AnError,
		},
		// the delete takes the tag off every document carrying it, so
		// the card says how many.
		"Tag carried by a document": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `}`,
			Result: "Delete tag Production and take it off the 1 page carrying it",
		},
		"Tag carried by nothing": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_otherTagID) + `}`,
			Result: "Delete tag Staging",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteTag{}.Summary(testInput(testDeps(c.DB, nil, nil), NameDeleteTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteTag, Summary: c.Result}, got)
		})
	}
}

func Test_deleteTag_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Tag id is required":  {DB: &DBMock{}, Args: `{}`, Err: assert.AnError},
		"Unknown tag": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			Args: `{` + tagArgs(_unknownTagID) + `}`,
			Err:  fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Error returned by db.DeleteTag": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error {
					return assert.AnError
				},
			},
			Args: `{` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:     &DBMock{},
			Args:   `{` + tagArgs(_testTagID) + `}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)

			res, err := deleteTag{}.Execute(testInput(d, NameDeleteTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			ff := c.DB.DeleteTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testTagID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.JSONEq(t, `{"tag_id":"`+_testTagID.String()+`","deleted":true}`, res)
		})
	}
}

func Test_setTagAssignmentArgs_Validate(t *testing.T) {
	t.Parallel()

	ok := setTagAssignmentArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, TagID: _testTagID, Assigned: null.ValueFrom(false)}

	assertValidate(t, ok, map[string]Args{
		"document_id": setTagAssignmentArgs{docTarget: docTarget{BranchID: _stubMainBranchID}, TagID: _testTagID, Assigned: null.ValueFrom(true)},
		"branch_id":   setTagAssignmentArgs{docTarget: docTarget{DocumentID: _testDocID}, TagID: _testTagID, Assigned: null.ValueFrom(true)},
		"tag_id":      setTagAssignmentArgs{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, Assigned: null.ValueFrom(true)},
		"assigned":    setTagAssignmentArgs{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, TagID: _testTagID},
	})
}

func Test_setTagAssignment_Info(t *testing.T) {
	t.Parallel()

	info := setTagAssignment{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameSetTagAssignment, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id", "tag_id", "assigned"}, info.Required)
}

func Test_setTagAssignment_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result ActionSummary
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Unknown branch": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":true}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `,"assigned":true}`,
			Err:  assert.AnError,
		},
		"Putting the tag on": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":true}`,
			Result: ActionSummary{
				Tool:         NameSetTagAssignment,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Put tag Production on Runbook on branch draft",
			},
		},
		"Taking the tag off": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":false}`,
			Result: ActionSummary{
				Tool:         NameSetTagAssignment,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Take tag Production off Runbook",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := setTagAssignment{}.Summary(testInput(testDeps(c.DB, nil, nil), NameSetTagAssignment, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_setTagAssignment_Execute(t *testing.T) {
	t.Parallel()

	type tcase struct {
		DB       *DBMock
		Args     string
		Notify   int
		Assigns  int
		Removes  int
		BranchID xid.ID
		Result   string
		Err      error
	}

	cc := map[string]tcase{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Assigned is required": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown branch": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":true}`,
			Err:  assert.AnError,
		},
		"Unknown tag": func() tcase {
			db := stubTagDB(nil)
			db.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
				return errutil.ErrNotFound
			}

			return tcase{
				DB:      db,
				Args:    `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `,"assigned":true}`,
				Assigns: 1,
				Err:     fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
			}
		}(),
		"Error returned by db.AssignBranchTag": func() tcase {
			db := stubTagDB(nil)
			db.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
				return assert.AnError
			}

			return tcase{
				DB:      db,
				Args:    `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":true}`,
				Assigns: 1,
				Err:     assert.AnError,
			}
		}(),
		"Error returned by db.UnassignBranchTag": func() tcase {
			db := stubTagDB(nil)
			db.UnassignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
				return assert.AnError
			}

			return tcase{
				DB:      db,
				Args:    `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":false}`,
				Removes: 1,
				Err:     assert.AnError,
			}
		}(),
		// the repository treats a tag the branch already carries as
		// nothing to do, so the call reads the same as a fresh one.
		"Assigned": {
			DB:       stubTagDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":true}`,
			Notify:   1,
			Assigns:  1,
			BranchID: _stubBranchID,
			Result:   `{"document_id":"` + _testDocID.String() + `","branch_id":"` + _stubBranchID.String() + `","tag_id":"` + _testTagID.String() + `","assigned":true}`,
		},
		"Hidden tag is assigned all the same": {
			DB:       stubTagDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_otherTagID) + `,"assigned":true}`,
			Notify:   1,
			Assigns:  1,
			BranchID: _stubBranchID,
			Result:   `{"document_id":"` + _testDocID.String() + `","branch_id":"` + _stubBranchID.String() + `","tag_id":"` + _otherTagID.String() + `","assigned":true}`,
		},
		"Unassigned": {
			DB:       stubTagDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `,"assigned":false}`,
			Notify:   1,
			Removes:  1,
			BranchID: _stubMainBranchID,
			Result:   `{"document_id":"` + _testDocID.String() + `","branch_id":"` + _stubMainBranchID.String() + `","tag_id":"` + _testTagID.String() + `","assigned":false}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)
			inp := testInput(d, NameSetTagAssignment, c.Args)

			res, err := setTagAssignment{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.AssignBranchTagCalls(), c.Assigns)
			assert.Len(t, c.DB.UnassignBranchTagCalls(), c.Removes)
			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)
			assert.Len(t, tags.NotifyBranchTagsChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// the header of an open document refreshes from its own topic,
			// while the sidebar refreshes from the tree's.
			bf := tags.NotifyBranchTagsChangeCalls()
			require.Len(t, bf, 1)
			assert.Equal(t, "org", bf[0].OrganizationID)
			assert.Equal(t, _testDocID, bf[0].DocumentID)
			assert.Equal(t, c.BranchID, bf[0].BranchID)

			assert.JSONEq(t, c.Result, res)
		})
	}
}
