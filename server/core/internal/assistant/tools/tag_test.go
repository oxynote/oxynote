package tools

import (
	"context"
	"fmt"
	"testing"

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

	assert.Equal(t, NameListTags, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func Test_listTags_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, listTags{}.Traits())
}

func Test_listTags_Title(t *testing.T) {
	t.Parallel()

	got, err := listTags{}.Title(testInput(testDeps(nil, nil, nil), NameListTags, `{}`))
	require.NoError(t, err)
	assert.Empty(t, got)
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
				`{"id":"` + _testTagID.String() + `","name":"Production","color":"#22c55e","hidden":false,` +
				`"documents":[{"id":"` + _testDocID.String() + `","name":"Runbook","default_branch_id":"` + _stubMainBranchID.String() + `"}]},` +
				`{"id":"` + _otherTagID.String() + `","name":"Staging","color":"#f97316","hidden":true,"documents":[]}` +
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

func Test_tagInfos(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []tagInfo{}, tagInfos(nil))
	assert.Equal(t, []tagInfo{
		{ID: _testTagID, Name: _stubTagName, Color: _stubTagColor},
	}, tagInfos([]tag.Tag{{ID: _testTagID, TagName: _stubTagName, Color: _stubTagColor, OrganizationID: "org"}}))
}

func Test_createTagArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, createTagArgs{Name: "Release", Color: "#3b82f6"}, map[string]Args{
		"name":  createTagArgs{Color: "#3b82f6"},
		"color": createTagArgs{Name: "Release"},
	})

	// a colour the sidebar cannot render is refused in the domain's
	// words.
	assert.Equal(t, tag.ErrInvalidTagColor, createTagArgs{Name: "Release", Color: "3b82f6"}.Validate())
}

func Test_createTag_Info(t *testing.T) {
	t.Parallel()

	info := createTag{}.Info()

	assert.Equal(t, NameCreateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyName, _keyColor}, info.Required)
	assert.Contains(t, info.Properties, _keyName)
	assert.Contains(t, info.Properties, _keyColor)
}

func Test_createTag_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, createTag{}.Traits())
}

func Test_createTag_Title(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	got, err := createTag{}.Title(testInput(d, NameCreateTag, `{"name":"Release","color":"#3b82f6"}`))
	require.NoError(t, err)
	assert.Equal(t, "Creating tag Release", got)

	_, err = createTag{}.Title(testInput(d, NameCreateTag, `{}`))
	require.Error(t, err)
}

func Test_createTag_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)

	// a tag that does not exist yet has no id to name, and no document.
	got, err := createTag{}.Summary(testInput(d, NameCreateTag, `{"name":"Release","color":"#3b82f6"}`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{Tool: NameCreateTag, Summary: "Create tag Release (#3b82f6)"}, got)

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
		"Name is required":        {DB: &DBMock{}, Args: `{"color":"#3b82f6"}`, Err: assert.AnError},
		"Colour is required":      {DB: &DBMock{}, Args: `{"name":"Release"}`, Err: assert.AnError},
		"Malformed colour":        {DB: &DBMock{}, Args: `{"name":"Release","color":"blue"}`, Err: assert.AnError},
		"Empty name is not a tag": {DB: &DBMock{}, Args: `{"name":"","color":"#3b82f6"}`, Err: assert.AnError},
		"Name already in use": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tag.Tag) error {
					return tag.ErrDuplicateTagName
				},
			},
			Args: `{"name":"Release","color":"#3b82f6"}`,
			Err:  fmt.Errorf("create_tag: %w", tag.ErrDuplicateTagName),
		},
		"Error returned by db.InsertTag": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tag.Tag) error {
					return assert.AnError
				},
			},
			Args: `{"name":"Release","color":"#3b82f6"}`,
			Err:  assert.AnError,
		},
		"Created": {
			DB:     &DBMock{},
			Args:   `{"name":"Release","color":"#3b82f6"}`,
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
			assert.Equal(t, "#3b82f6", ff[0].T.Color)
			assert.JSONEq(t, `{"tag_id":"`+ff[0].T.ID.String()+`"}`, res)
		})
	}
}

func Test_updateTagArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, updateTagArgs{TagID: _testTagID, Name: "Release"}, map[string]Args{
		"tag_id": updateTagArgs{Name: "Release"},
	})

	assert.Equal(t, tag.ErrEmptyTagUpdate, updateTagArgs{TagID: _testTagID}.Validate())
	assert.Equal(t, tag.ErrInvalidTagColor, updateTagArgs{TagID: _testTagID, Color: "blue"}.Validate())
	require.NoError(t, updateTagArgs{TagID: _testTagID, Color: "#3b82f6"}.Validate())
}

func Test_updateTag_Info(t *testing.T) {
	t.Parallel()

	info := updateTag{}.Info()

	assert.Equal(t, NameUpdateTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyTagID}, info.Required)
	assert.Contains(t, info.Properties, _keyName)
	assert.Contains(t, info.Properties, _keyColor)
}

func Test_updateTag_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateTag{}.Traits())
}

func Test_updateTag_Title(t *testing.T) {
	t.Parallel()

	got, err := updateTag{}.Title(testInput(
		testDeps(stubTagDB(nil), nil, nil), NameUpdateTag,
		`{`+tagArgs(_testTagID)+`,"name":"Release"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating tag Production", got)

	// the tag it names has to resolve; the failure is passed on rather
	// than described around.
	_, err = updateTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameUpdateTag, `{`+tagArgs(_unknownTagID)+`,"name":"Release"}`))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameUpdateTag, `{`))
	require.Error(t, err)
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
			Args:   `{` + tagArgs(_testTagID) + `,"color":"#3b82f6"}`,
			Result: "Set the colour to #3b82f6",
		},
		"Rename and recolour": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release","color":"#3b82f6"}`,
			Result: `Rename Production to "Release" and set the colour to #3b82f6`,
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

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Result string
		Err    error
	}{
		"Malformed arguments": {DB: &DBMock{}, Args: `{`, Err: assert.AnError},
		"Tag id is required":  {DB: &DBMock{}, Args: `{"name":"Release"}`, Err: assert.AnError},
		"Nothing to change":   {DB: &DBMock{}, Args: `{` + tagArgs(_testTagID) + `}`, Err: assert.AnError},
		"Malformed colour":    {DB: &DBMock{}, Args: `{` + tagArgs(_testTagID) + `,"color":"blue"}`, Err: assert.AnError},
		"Unknown tag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return errutil.ErrNotFound
				},
			},
			Args: `{` + tagArgs(_unknownTagID) + `,"name":"Release"}`,
			Err:  fmt.Errorf("update_tag: %w", fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag)),
		},
		"Error returned by db.UpdateTag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return assert.AnError
				},
			},
			Args: `{` + tagArgs(_testTagID) + `,"name":"Release"}`,
			Err:  assert.AnError,
		},
		"Renamed": {
			DB:     &DBMock{},
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release"}`,
			Notify: 1,
			Result: `{"tag_id":"` + _testTagID.String() + `","name":"Release"}`,
		},
		"Renamed and recoloured": {
			DB:     &DBMock{},
			Args:   `{` + tagArgs(_testTagID) + `,"name":"Release","color":"#3b82f6"}`,
			Notify: 1,
			Result: `{"tag_id":"` + _testTagID.String() + `","name":"Release","color":"#3b82f6"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)

			res, err := updateTag{}.Execute(testInput(d, NameUpdateTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
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

	assert.Equal(t, NameDeleteTag, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{_keyTagID}, info.Required)
}

func Test_deleteTag_Traits(t *testing.T) {
	t.Parallel()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, deleteTag{}.Traits())
}

func Test_deleteTag_Title(t *testing.T) {
	t.Parallel()

	got, err := deleteTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameDeleteTag, `{`+tagArgs(_testTagID)+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Deleting tag Production", got)

	_, err = deleteTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameDeleteTag, `{`+tagArgs(_unknownTagID)+`}`))
	require.Error(t, err)

	_, err = deleteTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameDeleteTag, `{`))
	require.Error(t, err)
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
			Err:  fmt.Errorf("delete_tag: %w", fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag)),
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

func Test_branchTagArgs_Validate(t *testing.T) {
	t.Parallel()

	ok := branchTagArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, TagID: _testTagID}

	assertValidate(t, ok, map[string]Args{
		"document_id": branchTagArgs{docTarget: docTarget{BranchID: _stubMainBranchID}, TagID: _testTagID},
		"branch_id":   branchTagArgs{docTarget: docTarget{DocumentID: _testDocID}, TagID: _testTagID},
		"tag_id":      branchTagArgs{docTarget: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}},
	})
}

func Test_branchTagProps(t *testing.T) {
	t.Parallel()

	props := branchTagProps()

	assert.Contains(t, props, _keyDocumentID)
	assert.Contains(t, props, _keyBranchID)
	assert.Contains(t, props, _keyTagID)
}

func Test_assignTag_Info(t *testing.T) {
	t.Parallel()

	info := assignTag{}.Info()

	assert.Equal(t, NameAssignTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyBranchID, _keyTagID}, info.Required)
	assert.Contains(t, info.Properties, _keyBranchID)
}

func Test_assignTag_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, assignTag{}.Traits())
}

func Test_assignTag_Title(t *testing.T) {
	t.Parallel()

	got, err := assignTag{}.Title(testInput(
		testDeps(stubTagDB(nil), nil, nil), NameAssignTag,
		`{`+targetArgs(_stubMainBranchID)+`,`+tagArgs(_testTagID)+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Tagging Runbook", got)

	// a branch other than the default one is named.
	got, err = assignTag{}.Title(testInput(
		testDeps(stubTagDB(nil), nil, nil), NameAssignTag,
		`{`+targetArgs(_stubBranchID)+`,`+tagArgs(_testTagID)+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Tagging Runbook on branch draft", got)

	_, err = assignTag{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameAssignTag, requiredArgs(t, NameAssignTag)))
	require.Error(t, err)

	_, err = assignTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameAssignTag, `{`))
	require.Error(t, err)
}

func Test_assignTag_Summary(t *testing.T) {
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
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `}`,
			Err:  assert.AnError,
		},
		"Names the tag and the document": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Result: ActionSummary{
				Tool:         NameAssignTag,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Put tag Production on Runbook on branch draft",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := assignTag{}.Summary(testInput(testDeps(c.DB, nil, nil), NameAssignTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_assignTag_Execute(t *testing.T) {
	t.Parallel()

	unknownTag := stubTagDB(nil)
	unknownTag.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
		return errutil.ErrNotFound
	}

	failing := stubTagDB(nil)
	failing.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Tag id is required":  {DB: stubTagDB(nil), Args: `{` + targetArgs(_stubMainBranchID) + `}`, Err: assert.AnError},
		"Unknown branch": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   unknownTag,
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `}`,
			Err:  fmt.Errorf("assign_tag: %w", fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag)),
		},
		"Error returned by db.AssignBranchTag": {
			DB:   failing,
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		// the repository treats a tag the branch already carries as
		// nothing to do, so the call reads the same as a fresh one.
		"Assigned": {
			DB:     stubTagDB(nil),
			Args:   `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Notify: 1,
		},
		"Hidden tag is assigned all the same": {
			DB:     stubTagDB(nil),
			Args:   `{` + targetArgs(_stubBranchID) + `,` + tagArgs(_otherTagID) + `}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)
			inp := testInput(d, NameAssignTag, c.Args)

			res, err := assignTag{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)
			assert.Len(t, tags.NotifyBranchTagsChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			ff := c.DB.AssignBranchTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, _stubBranchID, ff[0].BranchID)

			// the header of an open document refreshes from its own topic,
			// while the sidebar refreshes from the tree's.
			bf := tags.NotifyBranchTagsChangeCalls()
			require.Len(t, bf, 1)
			assert.Equal(t, "org", bf[0].OrganizationID)
			assert.Equal(t, _testDocID, bf[0].DocumentID)
			assert.Equal(t, _stubBranchID, bf[0].BranchID)

			assert.JSONEq(t, `{"document_id":"`+_testDocID.String()+`","branch_id":"`+_stubBranchID.String()+`","tag_id":"`+ff[0].TagID.String()+`"}`, res)
			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}, inp.touched)
		})
	}
}

func Test_unassignTag_Info(t *testing.T) {
	t.Parallel()

	info := unassignTag{}.Info()

	assert.Equal(t, NameUnassignTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyDocumentID, _keyBranchID, _keyTagID}, info.Required)
	assert.Contains(t, info.Properties, _keyBranchID)
}

func Test_unassignTag_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, unassignTag{}.Traits())
}

func Test_unassignTag_Title(t *testing.T) {
	t.Parallel()

	got, err := unassignTag{}.Title(testInput(
		testDeps(stubTagDB(nil), nil, nil), NameUnassignTag,
		`{`+targetArgs(_stubMainBranchID)+`,`+tagArgs(_testTagID)+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Untagging Runbook", got)

	_, err = unassignTag{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUnassignTag, requiredArgs(t, NameUnassignTag)))
	require.Error(t, err)

	_, err = unassignTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameUnassignTag, `{`))
	require.Error(t, err)
}

func Test_unassignTag_Summary(t *testing.T) {
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
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `}`,
			Err:  assert.AnError,
		},
		"Names the tag and the document": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Result: ActionSummary{
				Tool:         NameUnassignTag,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Take tag Production off Runbook",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := unassignTag{}.Summary(testInput(testDeps(c.DB, nil, nil), NameUnassignTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_unassignTag_Execute(t *testing.T) {
	t.Parallel()

	failing := stubTagDB(nil)
	failing.UnassignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Err    error
	}{
		"Malformed arguments": {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Tag id is required":  {DB: stubTagDB(nil), Args: `{` + targetArgs(_stubMainBranchID) + `}`, Err: assert.AnError},
		"Unknown branch": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_unknownTagID) + `}`,
			Err:  fmt.Errorf("unassign_tag: %w", fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag)),
		},
		"Error returned by db.UnassignBranchTag": {
			DB:   failing,
			Args: `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Err:  assert.AnError,
		},
		// the repository treats a tag the branch does not carry as
		// nothing to do, so the call reads the same either way.
		"Unassigned": {
			DB:     stubTagDB(nil),
			Args:   `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_testTagID) + `}`,
			Notify: 1,
		},
		"Hidden tag is unassigned all the same": {
			DB:     stubTagDB(nil),
			Args:   `{` + targetArgs(_stubMainBranchID) + `,` + tagArgs(_otherTagID) + `}`,
			Notify: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)
			inp := testInput(d, NameUnassignTag, c.Args)

			res, err := unassignTag{}.Execute(inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)
			assert.Len(t, tags.NotifyBranchTagsChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			ff := c.DB.UnassignBranchTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, _stubMainBranchID, ff[0].BranchID)

			bf := tags.NotifyBranchTagsChangeCalls()
			require.Len(t, bf, 1)
			assert.Equal(t, "org", bf[0].OrganizationID)
			assert.Equal(t, _testDocID, bf[0].DocumentID)
			assert.Equal(t, _stubMainBranchID, bf[0].BranchID)

			assert.JSONEq(t, `{"document_id":"`+_testDocID.String()+`","branch_id":"`+_stubMainBranchID.String()+`","tag_id":"`+ff[0].TagID.String()+`"}`, res)
			assert.Equal(t, []Touched{{DocumentID: _testDocID, BranchID: _stubMainBranchID}}, inp.touched)
		})
	}
}

func Test_moveTagArgs_Validate(t *testing.T) {
	t.Parallel()

	var (
		first  = `{` + tagArgs(_testTagID) + `,"sort_index":0}`
		noTag  = `{"sort_index":0}`
		noSort = `{` + tagArgs(_testTagID) + `}`
	)

	// the first position is a legal answer, so presence is what is
	// checked rather than a zero value; the arguments are decoded here
	// because that is what tells the two apart.
	decode := func(t *testing.T, raw string) moveTagArgs {
		t.Helper()

		var in moveTagArgs
		require.NoError(t, testInput(testDeps(nil, nil, nil), NameMoveTag, raw).Decode(&in))

		return in
	}

	assertValidate(t, decode(t, first), nil)

	for key, raw := range map[string]string{"tag_id": noTag, "sort_index": noSort} {
		var in moveTagArgs

		err := testInput(testDeps(nil, nil, nil), NameMoveTag, raw).Decode(&in)
		require.Error(t, err, "%s should be required", key)
		assert.Contains(t, err.Error(), key+" is required")
	}
}

func Test_moveTag_Info(t *testing.T) {
	t.Parallel()

	info := moveTag{}.Info()

	assert.Equal(t, NameMoveTag, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{_keyTagID, _keySortIndex}, info.Required)
	assert.Contains(t, info.Properties, _keySortIndex)
}

func Test_moveTag_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, moveTag{}.Traits())
}

func Test_moveTag_Title(t *testing.T) {
	t.Parallel()

	got, err := moveTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`+tagArgs(_testTagID)+`,"sort_index":1}`))
	require.NoError(t, err)
	assert.Equal(t, "Moving tag Production", got)

	_, err = moveTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`+tagArgs(_unknownTagID)+`,"sort_index":1}`))
	require.Error(t, err)

	_, err = moveTag{}.Title(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`))
	require.Error(t, err)
}

func Test_moveTag_Summary(t *testing.T) {
	t.Parallel()

	got, err := moveTag{}.Summary(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`+tagArgs(_testTagID)+`,"sort_index":1}`))
	require.NoError(t, err)

	// the card counts positions from one, the way a person does.
	assert.Equal(t, ActionSummary{Tool: NameMoveTag, Summary: "Move tag Production to position 2"}, got)

	_, err = moveTag{}.Summary(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`+tagArgs(_unknownTagID)+`,"sort_index":1}`))
	require.Error(t, err)

	_, err = moveTag{}.Summary(testInput(testDeps(stubTagDB(nil), nil, nil), NameMoveTag, `{`))
	require.Error(t, err)
}

func Test_moveTag_Execute(t *testing.T) {
	t.Parallel()

	failing := stubTagDB(nil)
	failing.UpdateTagTreeFunc = func(context.Context, tag.Summaries, string) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Notify int
		Order  []xid.ID
		Err    error
	}{
		"Malformed arguments":    {DB: stubTagDB(nil), Args: `{`, Err: assert.AnError},
		"Tag id is required":     {DB: stubTagDB(nil), Args: `{"sort_index":0}`, Err: assert.AnError},
		"Sort index is required": {DB: stubTagDB(nil), Args: `{` + tagArgs(_testTagID) + `}`, Err: assert.AnError},
		"Error returned by db.FetchTagTree": {
			DB:   stubTagDB(assert.AnError),
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Err:  assert.AnError,
		},
		"Unknown tag": {
			DB:   stubTagDB(nil),
			Args: `{` + tagArgs(_unknownTagID) + `,"sort_index":1}`,
			Err:  fmt.Errorf("move_tag: %w", fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag)),
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
			DB:   failing,
			Args: `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Err:  assert.AnError,
		},
		"Moved to the end": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_testTagID) + `,"sort_index":1}`,
			Notify: 1,
			Order:  []xid.ID{_otherTagID, _testTagID},
		},
		"Moved to the front": {
			DB:     stubTagDB(nil),
			Args:   `{` + tagArgs(_otherTagID) + `,"sort_index":0}`,
			Notify: 1,
			Order:  []xid.ID{_otherTagID, _testTagID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, tags := tagDeps(c.DB)

			res, err := moveTag{}.Execute(testInput(d, NameMoveTag, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notify)

			if err != nil {
				return
			}

			// the whole tree is rewritten in its new order.
			ff := c.DB.UpdateTagTreeCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)

			order := make([]xid.ID, 0, len(ff[0].Tree))
			for _, s := range ff[0].Tree {
				order = append(order, s.ID)
			}

			assert.Equal(t, c.Order, order)
			assert.Contains(t, res, `"sort_index":`)
		})
	}
}
