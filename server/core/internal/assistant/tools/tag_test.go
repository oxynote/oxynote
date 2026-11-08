package tools

import (
	"errors"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTagInput answers the tag tree and the test tag, and accepts every
// tag write.
func stubTagInput() *InputMock {
	return &InputMock{
		FetchTagTreeFunc: func() (tag.Summaries, error) {
			return stubTagTree(), nil
		},
		FetchTagFunc: func(xid.ID) (*tag.Summary, error) {
			return &stubTagTree()[0], nil
		},
		FetchBranchFunc: func(documentID, branchID xid.ID) (*document.Document, error) {
			name := document.DefaultBranch
			if branchID == _stubBranchID {
				name = _stubBranchName
			}

			return stubBranchDocument(documentID, "org", branchID, name), nil
		},
		OrganizationIDFunc: func() string { return "org" },
		UserIDFunc:         func() string { return "user" },
	}
}

func Test_listTags_Info(t *testing.T) {
	t.Parallel()

	info := listTags{}.Info()

	assert.Equal(t, Traits{Group: GroupTags}, info.Traits)

	assert.Equal(t, NameListTags, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func Test_listTags_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listTags{}.Validate())
}

func Test_listTags_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.FetchTagTree": {
			Input: &InputMock{
				FetchTagTreeFunc: func() (tag.Summaries, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"No tags": {
			Input: &InputMock{
				FetchTagTreeFunc: func() (tag.Summaries, error) {
					return tag.Summaries{}, nil
				},
			},
			Result: listTagsResult{Tags: []tagEntry{}},
		},
		// the tree lists the nested children of an assigned document as
		// sidebar context. Only the assigned document itself is listed.
		"Tags with their documents": {
			Input: stubTagInput(),
			Result: listTagsResult{Tags: []tagEntry{
				{
					ID:        _testTagID,
					Name:      _stubTagName,
					ColorName: _stubTagColor,
					Documents: []tagDocument{{ID: _testDocID, Name: _stubDocumentName, DefaultBranchID: _stubMainBranchID}},
				},
				{ID: _otherTagID, Name: "Staging", ColorName: "orange", Documents: []tagDocument{}},
			}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listTags{}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchTagTreeCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createTag_Info(t *testing.T) {
	t.Parallel()

	info := createTag{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupTags}, info.Traits)

	assert.Equal(t, NameCreateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"name", "color"}, info.Required)
	assert.Contains(t, info.Properties, "name")

	prop, ok := info.Properties["color"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, tag.ColorNames(), prop["enum"])
}

func Test_createTag_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool createTag
		Err  error
	}{
		"Name is required":   {Tool: createTag{ColorName: "blue"}, Err: errRequired("name")},
		"Colour is required": {Tool: createTag{Name: "Release"}, Err: errRequired("color")},
		// a colour outside the palette is refused in the domain's words.
		"Colour outside the palette": {Tool: createTag{Name: "Release", ColorName: "navy"}, Err: tag.ErrInvalidTagColor},
		"Valid arguments":            {Tool: createTag{Name: "Release", ColorName: "blue"}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_createTag_Summary(t *testing.T) {
	t.Parallel()

	// a tag that does not exist yet has no id to name, and no document.
	got, err := createTag{Name: "Release", ColorName: "blue"}.Summary(&InputMock{})
	require.NoError(t, err)

	assert.Equal(t, ActionSummary{Tool: NameCreateTag, Summary: "Create tag Release (blue)"}, got)
}

func Test_createTag_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input *InputMock
		Tool  createTag
		Err   error
	}{
		// the domain trims a name and refuses one left empty.
		"Name of only spaces": {
			Input: stubTagInput(),
			Tool:  createTag{Name: "  ", ColorName: "blue"},
			Err:   assert.AnError,
		},
		"Error returned by Input.CreateTag": {
			Input: func() *InputMock {
				inp := stubTagInput()
				inp.CreateTagFunc = func(tag.Tag) error { return assert.AnError }

				return inp
			}(),
			Tool: createTag{Name: "Release", ColorName: "blue"},
			Err:  assert.AnError,
		},
		"Created": {
			Input: stubTagInput(),
			Tool:  createTag{Name: "  Release  ", ColorName: "blue"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			// the tag is stamped with the session's pair and the
			// arguments, and the result names the id it got.
			ff := c.Input.CreateTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].T.OrganizationID)
			assert.Equal(t, "user", ff[0].T.CreatedBy.String)
			assert.Equal(t, "Release", ff[0].T.TagName)
			assert.Equal(t, "blue", ff[0].T.Color)
			assert.Equal(t, createTagResult{TagID: ff[0].T.ID}, res)
		})
	}
}

func Test_createTag_input(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		tag.CreateInput{TagName: "Release", Color: "blue"},
		createTag{Name: "Release", ColorName: "blue"}.input(),
	)
}

func Test_updateTag_Info(t *testing.T) {
	t.Parallel()

	info := updateTag{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupTags}, info.Traits)

	assert.Equal(t, NameUpdateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"tag_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, "sort_index")
	assert.Contains(t, info.Properties, "tag_id")

	prop, ok := info.Properties["color"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, tag.ColorNames(), prop["enum"])
}

func Test_updateTag_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool updateTag
		Err  error
	}{
		"Tag id is required":         {Tool: updateTag{Name: "Release"}, Err: errRequired("tag_id")},
		"Nothing to change":          {Tool: updateTag{TagID: _testTagID}, Err: errors.New("give at least one of name, color or sort_index")},
		"Colour outside the palette": {Tool: updateTag{TagID: _testTagID, ColorName: "navy"}, Err: tag.ErrInvalidTagColor},
		"Rename":                     {Tool: updateTag{TagID: _testTagID, Name: "Release"}},
		"Recolour":                   {Tool: updateTag{TagID: _testTagID, ColorName: "blue"}},
		// the first position is a move like any other.
		"Move to the first position": {Tool: updateTag{TagID: _testTagID, SortIndex: null.ValueFrom(0)}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_updateTag_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   updateTag
		Result ActionSummary
		Err    error
	}{
		"Error returned by Input.FetchTag": {
			Input: &InputMock{
				FetchTagFunc: func(xid.ID) (*tag.Summary, error) { return nil, assert.AnError },
			},
			Tool: updateTag{TagID: _unknownTagID, Name: "Release"},
			Err:  assert.AnError,
		},
		"Rename and move": {
			Input:  stubTagInput(),
			Tool:   updateTag{TagID: _testTagID, Name: "Release", SortIndex: null.ValueFrom(0)},
			Result: ActionSummary{Tool: NameUpdateTag, Summary: `Rename Production to "Release" and move it to position 1`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.FetchTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Tool.TagID, ff[0].TagID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_updateTag_Exec(t *testing.T) {
	t.Parallel()

	withInput := func(edit func(inp *InputMock)) *InputMock {
		inp := stubTagInput()
		edit(inp)

		return inp
	}

	cc := map[string]struct {
		Input   *InputMock
		Tool    updateTag
		Fetches int
		Renames int
		Moves   int
		Result  any
		Err     error
	}{
		"Error returned by Input.UpdateTag": {
			Input: withInput(func(inp *InputMock) {
				inp.UpdateTagFunc = func(xid.ID, tag.UpdateInput) error { return assert.AnError }
			}),
			Tool:    updateTag{TagID: _testTagID, Name: "Release"},
			Renames: 1,
			Err:     assert.AnError,
		},
		"Renamed and recoloured": {
			Input:   stubTagInput(),
			Tool:    updateTag{TagID: _testTagID, Name: "Release", ColorName: "blue"},
			Renames: 1,
			Result:  updateTagResult{TagID: _testTagID, Name: "Release", ColorName: "blue"},
		},
		"Error returned by Input.MoveTag": {
			Input: withInput(func(inp *InputMock) { inp.MoveTagFunc = func(xid.ID, int) error { return assert.AnError } }),
			Tool:  updateTag{TagID: _testTagID, SortIndex: null.ValueFrom(1)},
			Moves: 1,
			Err:   assert.AnError,
		},
		// a move alone checks nothing first, since MoveTag refuses a bad
		// position itself.
		"Moved": {
			Input:  stubTagInput(),
			Tool:   updateTag{TagID: _testTagID, SortIndex: null.ValueFrom(1)},
			Moves:  1,
			Result: updateTagResult{TagID: _testTagID, SortIndex: new(1)},
		},
		// a position the move would refuse fails the call before the
		// rename is saved.
		"Rename with a sort index past the last tag": {
			Input:   stubTagInput(),
			Tool:    updateTag{TagID: _testTagID, Name: "Release", SortIndex: null.ValueFrom(9)},
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Error returned by Input.FetchTagTree before the rename": {
			Input: withInput(func(inp *InputMock) {
				inp.FetchTagTreeFunc = func() (tag.Summaries, error) { return nil, assert.AnError }
			}),
			Tool:    updateTag{TagID: _testTagID, Name: "Release", SortIndex: null.ValueFrom(1)},
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Renamed and moved": {
			Input:   stubTagInput(),
			Tool:    updateTag{TagID: _testTagID, Name: "Release", SortIndex: null.ValueFrom(1)},
			Fetches: 1,
			Renames: 1,
			Moves:   1,
			Result:  updateTagResult{TagID: _testTagID, Name: "Release", SortIndex: new(1)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchTagTreeCalls(), c.Fetches)
			assert.Len(t, c.Input.UpdateTagCalls(), c.Renames)
			assert.Len(t, c.Input.MoveTagCalls(), c.Moves)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_updateTag_renames(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   updateTag
		Result bool
	}{
		"Move only":  {Tool: updateTag{SortIndex: null.ValueFrom(1)}},
		"New name":   {Tool: updateTag{Name: "Release"}, Result: true},
		"New colour": {Tool: updateTag{ColorName: "blue"}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.renames())
		})
	}
}

func Test_updateTag_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   updateTag
		Result tag.UpdateInput
	}{
		"Nothing given": {
			Tool:   updateTag{TagID: _testTagID},
			Result: tag.UpdateInput{},
		},
		"Name and palette colour": {
			Tool:   updateTag{TagID: _testTagID, Name: "Release", ColorName: "blue"},
			Result: tag.UpdateInput{TagName: null.StringFrom("Release"), Color: null.StringFrom("blue")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.input())
		})
	}
}

func Test_updateTag_changes(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   updateTag
		Result []string
	}{
		"Rename only":   {Tool: updateTag{Name: "Release"}, Result: []string{`rename Production to "Release"`}},
		"Recolour only": {Tool: updateTag{ColorName: "blue"}, Result: []string{"set the colour to blue"}},
		// the card counts positions from one, the way a person does.
		"Move only": {Tool: updateTag{SortIndex: null.ValueFrom(1)}, Result: []string{"move it to position 2"}},
		"Rename recolour and move": {
			Tool:   updateTag{Name: "Release", ColorName: "blue", SortIndex: null.ValueFrom(0)},
			Result: []string{`rename Production to "Release"`, "set the colour to blue", "move it to position 1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.changes(_stubTagName))
		})
	}
}

func Test_deleteTag_Info(t *testing.T) {
	t.Parallel()

	info := deleteTag{}.Info()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupTags}, info.Traits)

	assert.Equal(t, NameDeleteTag, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"tag_id"}, info.Required)
}

func Test_deleteTag_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool deleteTag
		Err  error
	}{
		"Tag id is required": {Tool: deleteTag{}, Err: errRequired("tag_id")},
		"Valid arguments":    {Tool: deleteTag{TagID: _testTagID}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_deleteTag_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchTag": {
			Input: &InputMock{
				FetchTagFunc: func(xid.ID) (*tag.Summary, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		// the delete takes the tag off every document carrying it, so
		// the card says how many.
		"Tag carried by a document": {
			Input:  stubTagInput(),
			Result: "Delete tag Production and take it off the 1 page carrying it",
		},
		"Tag carried by nothing": {
			Input: &InputMock{
				FetchTagFunc: func(xid.ID) (*tag.Summary, error) { return &stubTagTree()[1], nil },
			},
			Result: "Delete tag Staging",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteTag{TagID: _testTagID}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, c.Input.FetchTagCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteTag, Summary: c.Result}, got)
		})
	}
}

func Test_deleteTag_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.DeleteTag": {
			Input: &InputMock{DeleteTagFunc: func(xid.ID) error { return assert.AnError }},
			Err:   assert.AnError,
		},
		"Deleted": {
			Input:  &InputMock{},
			Result: deleteTagResult{TagID: _testTagID, Deleted: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteTag{TagID: _testTagID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.DeleteTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testTagID, ff[0].TagID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_setTagAssignment_Info(t *testing.T) {
	t.Parallel()

	info := setTagAssignment{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupTags}, info.Traits)

	assert.Equal(t, NameSetTagAssignment, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"document_id", "branch_id", "tag_id", "assigned"}, info.Required)
}

func Test_setTagAssignment_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool setTagAssignment
		Err  error
	}{
		"Document id is required": {
			Tool: setTagAssignment{docTarget: docTarget{BranchID: _stubMainBranchID}, TagID: _testTagID, Assigned: null.ValueFrom(true)},
			Err:  errRequired("document_id"),
		},
		"Branch id is required": {
			Tool: setTagAssignment{docTarget: docTarget{DocumentID: _testDocID}, TagID: _testTagID, Assigned: null.ValueFrom(true)},
			Err:  errRequired("branch_id"),
		},
		"Tag id is required": {
			Tool: setTagAssignment{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, Assigned: null.ValueFrom(true)},
			Err:  errRequired("tag_id"),
		},
		"Assigned is required": {
			Tool: setTagAssignment{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, TagID: _testTagID},
			Err:  errRequired("assigned"),
		},
		"Valid arguments": {
			Tool: setTagAssignment{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}, TagID: _testTagID, Assigned: null.ValueFrom(false)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_setTagAssignment_Summary(t *testing.T) {
	t.Parallel()

	tool := func(branchID xid.ID, assigned bool) setTagAssignment {
		return setTagAssignment{
			DocumentID: _testDocID, BranchID: branchID,
			TagID:    _testTagID,
			Assigned: null.ValueFrom(assigned),
		}
	}

	cc := map[string]struct {
		Input   *InputMock
		Tool    setTagAssignment
		Fetches int
		Result  ActionSummary
		Err     error
	}{
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: tool(_unknownBranchID, true),
			Err:  assert.AnError,
		},
		"Error returned by Input.FetchTag": {
			Input: func() *InputMock {
				inp := stubTagInput()
				inp.FetchTagFunc = func(xid.ID) (*tag.Summary, error) { return nil, assert.AnError }

				return inp
			}(),
			Tool:    tool(_stubMainBranchID, true),
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Putting the tag on": {
			Input:   stubTagInput(),
			Tool:    tool(_stubBranchID, true),
			Fetches: 1,
			Result: ActionSummary{
				Tool:         NameSetTagAssignment,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Put tag Production on Runbook on branch draft",
			},
		},
		"Taking the tag off": {
			Input:   stubTagInput(),
			Tool:    tool(_stubMainBranchID, false),
			Fetches: 1,
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

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)
			assert.Len(t, c.Input.FetchTagCalls(), c.Fetches)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_setTagAssignment_Exec(t *testing.T) {
	t.Parallel()

	tool := func(assigned bool) setTagAssignment {
		return setTagAssignment{
			DocumentID: _testDocID, BranchID: _stubBranchID,
			TagID:    _testTagID,
			Assigned: null.ValueFrom(assigned),
		}
	}

	cc := map[string]struct {
		Input   *InputMock
		Tool    setTagAssignment
		Assigns int
		Removes int
		Result  any
		Err     error
	}{
		"Error returned by Input.AssignTag": {
			Input:   &InputMock{AssignTagFunc: func(xid.ID, xid.ID, xid.ID) error { return assert.AnError }},
			Tool:    tool(true),
			Assigns: 1,
			Err:     assert.AnError,
		},
		"Error returned by Input.UnassignTag": {
			Input:   &InputMock{UnassignTagFunc: func(xid.ID, xid.ID, xid.ID) error { return assert.AnError }},
			Tool:    tool(false),
			Removes: 1,
			Err:     assert.AnError,
		},
		"Assigned": {
			Input:   &InputMock{},
			Tool:    tool(true),
			Assigns: 1,
			Result:  setTagAssignmentResult{DocumentID: _testDocID, BranchID: _stubBranchID, TagID: _testTagID, Assigned: true},
		},
		"Unassigned": {
			Input:   &InputMock{},
			Tool:    tool(false),
			Removes: 1,
			Result:  setTagAssignmentResult{DocumentID: _testDocID, BranchID: _stubBranchID, TagID: _testTagID, Assigned: false},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.AssignTagCalls(), c.Assigns)
			assert.Len(t, c.Input.UnassignTagCalls(), c.Removes)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
