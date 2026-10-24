package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const (
	// _org is the organization every call is made for.
	_org = "org"

	// _user is the user every call is made for.
	_user = "user"
)

var (
	// _group is a custom group.
	_group = variable.Group{
		ID:             xid.New(),
		OrganizationID: _org,
		GroupName:      "Environment",
		Color:          "green",
	}

	// _builtinGroup is the group every organization starts with.
	_builtinGroup = variable.Group{
		ID:             xid.New(),
		OrganizationID: _org,
		GroupName:      "Metrics",
		Color:          "blue",
		Builtin:        true,
	}

	// _textVariable is a custom text variable in _group.
	_textVariable = variable.Variable{
		ID:             xid.New(),
		OrganizationID: _org,
		GroupID:        _group.ID,
		Key:            "release",
		Name:           "Release",
		Type:           variable.TypeText,
		DefaultValue:   null.StringFrom("relay-2.14.0"),
	}

	// _listVariableID identifies _listVariable.
	_listVariableID = xid.New()

	// _listOptions are the options of _listVariable, in order.
	_listOptions = []variable.Option{
		{ID: xid.New(), VariableID: _listVariableID, Value: "eu-west-1"},
		{ID: xid.New(), VariableID: _listVariableID, Value: "us-east-1", SortIndex: 1},
	}

	// _listVariable is a custom list variable in _group.
	_listVariable = variable.Variable{
		ID:             _listVariableID,
		OrganizationID: _org,
		GroupID:        _group.ID,
		Key:            "region",
		Name:           "Region",
		Type:           variable.TypeList,
		DefaultValue:   null.StringFrom(_listOptions[0].ID.String()),
		Options:        _listOptions,
	}

	// _builtinVariable is the built-in time range variable.
	_builtinVariable = variable.Variable{
		ID:             xid.New(),
		OrganizationID: _org,
		GroupID:        _builtinGroup.ID,
		Key:            "metric_range",
		Name:           "Metric time range",
		Type:           variable.TypeTimeRange,
		DefaultValue:   null.StringFrom("last_24_hours"),
		Builtin:        true,
	}
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recorder counts the announcements a manager makes.
type recorder struct {
	changes []string
	values  []string
}

// newTestManager returns a manager over the database whose announcements
// the recorder counts.
func newTestManager(db DB) (*Manager, *recorder) {
	m := NewManager(slog.New(slog.DiscardHandler), db)
	rec := &recorder{}

	m.BindChange(func(organizationID string, userID null.String) {
		if !userID.Valid {
			rec.changes = append(rec.changes, organizationID)
			return
		}

		rec.values = append(rec.values, organizationID+"/"+userID.String)
	})

	return m, rec
}

// stubFetchVariable returns a FetchVariable that answers with the
// variable or the error.
func stubFetchVariable(v variable.Variable, err error) func(context.Context, xid.ID, string) (*variable.Variable, error) {
	return func(context.Context, xid.ID, string) (*variable.Variable, error) {
		if err != nil {
			return nil, err
		}

		return new(v), nil
	}
}

// stubFetchGroup returns a FetchVariableGroup that answers with the group
// or the error.
func stubFetchGroup(g variable.Group, err error) func(context.Context, xid.ID, string) (*variable.Group, error) {
	return func(context.Context, xid.ID, string) (*variable.Group, error) {
		if err != nil {
			return nil, err
		}

		return new(g), nil
	}
}

func Test_NewManager(t *testing.T) {
	db := &DBMock{}

	m := NewManager(slog.New(slog.DiscardHandler), db)
	require.NotNil(t, m)
	assert.NotNil(t, m.log)
	assert.Equal(t, db, m.db)
	require.NotNil(t, m.changeCallback)

	m.changeCallback(_org, null.StringFrom(_user))
}

func Test_Manager_BindChange(t *testing.T) {
	var (
		gotOrg  string
		gotUser null.String
	)

	m := NewManager(slog.New(slog.DiscardHandler), &DBMock{})
	m.BindChange(func(organizationID string, userID null.String) {
		gotOrg = organizationID
		gotUser = userID
	})
	m.changeCallback(_org, null.StringFrom(_user))

	assert.Equal(t, _org, gotOrg)
	assert.Equal(t, null.StringFrom(_user), gotUser)
}

func Test_Manager_FetchTree(t *testing.T) {
	t.Parallel()

	tree := variable.Tree{{Group: _group, Variables: []variable.Entry{}}}

	cc := map[string]struct {
		DB     *DBMock
		Result variable.Tree
		Err    error
	}{
		"Error returned by DB.FetchVariableTree": {
			DB: &DBMock{
				FetchVariableTreeFunc: func(context.Context, string, string) (variable.Tree, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"Successful fetch": {
			DB: &DBMock{
				FetchVariableTreeFunc: func(context.Context, string, string) (variable.Tree, error) {
					return tree, nil
				},
			},
			Result: tree,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, _ := newTestManager(c.DB)

			res, err := m.FetchTree(context.Background(), _org, _user)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)

			calls := c.DB.FetchVariableTreeCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, _org, calls[0].OrganizationID)
			assert.Equal(t, _user, calls[0].UserID)
		})
	}
}

func Test_Manager_CreateGroup(t *testing.T) {
	t.Parallel()

	inp := variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"}

	cc := map[string]struct {
		DB      *DBMock
		Inp     variable.GroupCreateInput
		Result  *variable.Group
		Creates int
		Fetches int
		Changes int
		Err     error
	}{
		"Invalid input": {
			DB:  &DBMock{},
			Inp: variable.GroupCreateInput{Color: "orange"},
			Err: variable.ErrInvalidGroupName,
		},
		"Error returned by DB.CreateVariableGroup": {
			DB: &DBMock{
				CreateVariableGroupFunc: func(context.Context, variable.Group) error {
					return assert.AnError
				},
			},
			Inp:     inp,
			Creates: 1,
			Err:     assert.AnError,
		},
		"Error returned by DB.FetchVariableGroup": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError),
			},
			Inp:     inp,
			Creates: 1,
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Successful creation": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
			},
			Inp:     inp,
			Result:  &_group,
			Creates: 1,
			Fetches: 1,
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			res, err := m.CreateGroup(context.Background(), c.Inp, _org, _user)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
			assert.Len(t, c.DB.CreateVariableGroupCalls(), c.Creates)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), c.Fetches)
			assert.Len(t, rec.changes, c.Changes)

			if err != nil {
				return
			}

			calls := c.DB.CreateVariableGroupCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "Deploys", calls[0].G.GroupName)
			assert.Equal(t, _org, calls[0].G.OrganizationID)
			assert.Equal(t, null.StringFrom(_user), calls[0].G.CreatedBy)
			assert.Equal(t, calls[0].G.ID, c.DB.FetchVariableGroupCalls()[0].ID)
		})
	}
}

func Test_Manager_UpdateGroup(t *testing.T) {
	t.Parallel()

	inp := variable.GroupUpdateInput{GroupName: null.StringFrom("Deploys")}

	cc := map[string]struct {
		DB      *DBMock
		Inp     variable.GroupUpdateInput
		Fetches int
		Updates int
		Changes int
		Err     error
	}{
		"Invalid input": {
			DB:  &DBMock{},
			Err: variable.ErrEmptyGroupUpdate,
		},
		"Error returned by DB.FetchVariableGroup": {
			DB:      &DBMock{FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError)},
			Inp:     inp,
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Built-in group": {
			DB:      &DBMock{FetchVariableGroupFunc: stubFetchGroup(_builtinGroup, nil)},
			Inp:     inp,
			Fetches: 1,
			Err:     variable.ErrBuiltinGroup,
		},
		"Error returned by DB.UpdateVariableGroup": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
				UpdateVariableGroupFunc: func(context.Context, string, xid.ID, variable.GroupUpdateInput) error {
					return assert.AnError
				},
			},
			Inp:     inp,
			Fetches: 1,
			Updates: 1,
			Err:     assert.AnError,
		},
		"Successful update": {
			DB:      &DBMock{FetchVariableGroupFunc: stubFetchGroup(_group, nil)},
			Inp:     inp,
			Fetches: 1,
			Updates: 1,
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.UpdateGroup(context.Background(), _group.ID, c.Inp, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), c.Fetches)
			assert.Len(t, c.DB.UpdateVariableGroupCalls(), c.Updates)
			assert.Len(t, rec.changes, c.Changes)
		})
	}
}

func Test_Manager_MoveGroup(t *testing.T) {
	t.Parallel()

	g1 := variable.Group{ID: xid.New(), OrganizationID: _org}
	g2 := variable.Group{ID: xid.New(), OrganizationID: _org}

	stubGroups := func(err error) func(context.Context, string) ([]variable.Group, error) {
		return func(context.Context, string) ([]variable.Group, error) {
			if err != nil {
				return nil, err
			}

			return []variable.Group{_builtinGroup, _group, g1, g2}, nil
		}
	}

	cc := map[string]struct {
		DB      *DBMock
		Inp     variable.GroupMoveInput
		Order   []xid.ID
		Changes int
		Err     error
	}{
		"Error returned by DB.FetchVariableGroups": {
			DB:  &DBMock{FetchVariableGroupsFunc: stubGroups(assert.AnError)},
			Inp: variable.GroupMoveInput{ID: _group.ID},
			Err: assert.AnError,
		},
		"Unknown group": {
			DB:  &DBMock{FetchVariableGroupsFunc: stubGroups(nil)},
			Inp: variable.GroupMoveInput{ID: xid.New()},
			Err: errutil.ErrNotFound,
		},
		"Built-in group": {
			DB:  &DBMock{FetchVariableGroupsFunc: stubGroups(nil)},
			Inp: variable.GroupMoveInput{ID: _builtinGroup.ID, SortIndex: 1},
			Err: variable.ErrBuiltinGroup,
		},
		"Index out of range": {
			DB:  &DBMock{FetchVariableGroupsFunc: stubGroups(nil)},
			Inp: variable.GroupMoveInput{ID: _group.ID, SortIndex: 3},
			Err: variable.ErrInvalidSortIndex,
		},
		"Error returned by DB.UpdateVariableGroupOrder": {
			DB: &DBMock{
				FetchVariableGroupsFunc: stubGroups(nil),
				UpdateVariableGroupOrderFunc: func(context.Context, string, []xid.ID) error {
					return assert.AnError
				},
			},
			Inp:   variable.GroupMoveInput{ID: _group.ID, SortIndex: 2},
			Order: []xid.ID{g1.ID, g2.ID, _group.ID},
			Err:   assert.AnError,
		},
		"Successful move to the end": {
			DB:      &DBMock{FetchVariableGroupsFunc: stubGroups(nil)},
			Inp:     variable.GroupMoveInput{ID: _group.ID, SortIndex: 2},
			Order:   []xid.ID{g1.ID, g2.ID, _group.ID},
			Changes: 1,
		},
		"Successful move to the top below the built-in group": {
			DB:      &DBMock{FetchVariableGroupsFunc: stubGroups(nil)},
			Inp:     variable.GroupMoveInput{ID: g2.ID},
			Order:   []xid.ID{g2.ID, _group.ID, g1.ID},
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.MoveGroup(context.Background(), c.Inp, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableGroupsCalls(), 1)
			assert.Len(t, rec.changes, c.Changes)

			calls := c.DB.UpdateVariableGroupOrderCalls()
			if c.Order == nil {
				assert.Empty(t, calls)
				return
			}

			require.Len(t, calls, 1)
			assert.Equal(t, c.Order, calls[0].Ids)
		})
	}
}

func Test_Manager_DeleteGroup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB      *DBMock
		Deletes int
		Changes int
		Err     error
	}{
		"Error returned by DB.FetchVariableGroup": {
			DB:  &DBMock{FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Built-in group": {
			DB:  &DBMock{FetchVariableGroupFunc: stubFetchGroup(_builtinGroup, nil)},
			Err: variable.ErrBuiltinGroup,
		},
		"Group with variables": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
				DeleteVariableGroupFunc: func(context.Context, xid.ID, string) error {
					return variable.ErrGroupNotEmpty
				},
			},
			Deletes: 1,
			Err:     variable.ErrGroupNotEmpty,
		},
		"Successful deletion": {
			DB:      &DBMock{FetchVariableGroupFunc: stubFetchGroup(_group, nil)},
			Deletes: 1,
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.DeleteGroup(context.Background(), _group.ID, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), 1)
			assert.Len(t, c.DB.DeleteVariableGroupCalls(), c.Deletes)
			assert.Len(t, rec.changes, c.Changes)
		})
	}
}

func Test_Manager_CreateVariable(t *testing.T) {
	t.Parallel()

	inp := variable.Input{
		GroupID: _group.ID,
		Key:     "region",
		Name:    "Region",
		Type:    variable.TypeList,
		Options: []variable.OptionInput{{Value: "eu-west-1"}},
	}

	cc := map[string]struct {
		DB           *DBMock
		Inp          variable.Input
		GroupFetches int
		Creates      int
		Fetches      int
		Changes      int
		Err          error
	}{
		"Invalid input": {
			DB:  &DBMock{},
			Inp: variable.Input{Key: "$region"},
			Err: variable.ErrInvalidKey,
		},
		"Error returned by DB.FetchVariableGroup": {
			DB:           &DBMock{FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError)},
			Inp:          inp,
			GroupFetches: 1,
			Err:          assert.AnError,
		},
		"Built-in group": {
			DB:           &DBMock{FetchVariableGroupFunc: stubFetchGroup(_builtinGroup, nil)},
			Inp:          inp,
			GroupFetches: 1,
			Err:          variable.ErrBuiltinGroup,
		},
		"Error returned by DB.CreateVariable": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
				CreateVariableFunc: func(context.Context, variable.Variable) error {
					return assert.AnError
				},
			},
			Inp:          inp,
			GroupFetches: 1,
			Creates:      1,
			Err:          assert.AnError,
		},
		"Error returned by DB.FetchVariable": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
				FetchVariableFunc:      stubFetchVariable(variable.Variable{}, assert.AnError),
			},
			Inp:          inp,
			GroupFetches: 1,
			Creates:      1,
			Fetches:      1,
			Err:          assert.AnError,
		},
		"Successful creation": {
			DB: &DBMock{
				FetchVariableGroupFunc: stubFetchGroup(_group, nil),
				FetchVariableFunc:      stubFetchVariable(_listVariable, nil),
			},
			Inp:          inp,
			GroupFetches: 1,
			Creates:      1,
			Fetches:      1,
			Changes:      1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			res, err := m.CreateVariable(context.Background(), c.Inp, _org, _user)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), c.GroupFetches)
			assert.Len(t, c.DB.FetchVariableCalls(), c.Fetches)
			assert.Len(t, rec.changes, c.Changes)

			calls := c.DB.CreateVariableCalls()
			require.Len(t, calls, c.Creates)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			assert.Equal(t, "region", calls[0].V.Key)
			assert.Equal(t, _org, calls[0].V.OrganizationID)
			require.Len(t, calls[0].V.Options, 1)
			assert.Equal(t, "eu-west-1", calls[0].V.Options[0].Value)

			require.NotNil(t, res)
			assert.Equal(t, _listVariable, *res)
		})
	}
}

func Test_Manager_UpdateVariable(t *testing.T) {
	t.Parallel()

	otherGroup := variable.Group{ID: xid.New(), OrganizationID: _org}

	textInp := variable.Input{
		GroupID:      _group.ID,
		Key:          "release",
		Name:         "Release name",
		Type:         variable.TypeText,
		DefaultValue: null.StringFrom("relay-3.0.0"),
	}

	listInp := variable.Input{
		GroupID: _group.ID,
		Key:     "region",
		Name:    "Region",
		Type:    variable.TypeList,
		Options: []variable.OptionInput{
			{ID: null.ValueFrom(_listOptions[1].ID), Value: "us-east-1"},
			{Value: "ap-south-1"},
		},
	}

	type tcase struct {
		DB            *DBMock
		Tx            *TxMock
		Inp           variable.Input
		Fetches       int
		GroupFetches  int
		Begins        int
		Updated       bool
		ClearedValues bool
		Options       []variable.Option
		Replaced      bool
		Commits       int
		Changes       int
		Err           error
	}

	cc := map[string]tcase{
		"Invalid input": func() tcase {
			return tcase{
				DB:  &DBMock{},
				Inp: variable.Input{Key: "release"},
				Err: variable.ErrInvalidName,
			}
		}(),
		"Error returned by DB.FetchVariable": func() tcase {
			return tcase{
				DB:      &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
				Inp:     textInp,
				Fetches: 1,
				Err:     assert.AnError,
			}
		}(),
		"Built-in variable": func() tcase {
			return tcase{
				DB:      &DBMock{FetchVariableFunc: stubFetchVariable(_builtinVariable, nil)},
				Inp:     textInp,
				Fetches: 1,
				Err:     variable.ErrBuiltinVariable,
			}
		}(),
		"Error returned by DB.FetchVariableGroup": func() tcase {
			inp := textInp
			inp.GroupID = otherGroup.ID

			return tcase{
				DB: &DBMock{
					FetchVariableFunc:      stubFetchVariable(_textVariable, nil),
					FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError),
				},
				Inp:          inp,
				Fetches:      1,
				GroupFetches: 1,
				Err:          assert.AnError,
			}
		}(),
		"Moved into the built-in group": func() tcase {
			inp := textInp
			inp.GroupID = _builtinGroup.ID

			return tcase{
				DB: &DBMock{
					FetchVariableFunc:      stubFetchVariable(_textVariable, nil),
					FetchVariableGroupFunc: stubFetchGroup(_builtinGroup, nil),
				},
				Inp:          inp,
				Fetches:      1,
				GroupFetches: 1,
				Err:          variable.ErrBuiltinGroup,
			}
		}(),
		"Unknown option": func() tcase {
			inp := listInp
			inp.Options = []variable.OptionInput{{ID: null.ValueFrom(xid.New()), Value: "a"}}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_listVariable, nil),
				},
				Inp:     inp,
				Fetches: 1,
				Err:     variable.ErrUnknownOption,
			}
		}(),
		"Error returned by DB.BeginTx": func() tcase {
			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_textVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(nil, assert.AnError),
				},
				Inp:     textInp,
				Fetches: 1,
				Begins:  1,
				Err:     assert.AnError,
			}
		}(),
		"Error returned by Tx.UpdateVariable": func() tcase {
			tx := &TxMock{
				UpdateVariableFunc: func(context.Context, variable.Variable) error {
					return assert.AnError
				},
			}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_textVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:      tx,
				Inp:     textInp,
				Fetches: 1,
				Begins:  1,
				Updated: true,
				Err:     assert.AnError,
			}
		}(),
		"Error returned by Tx.DeleteVariableValues": func() tcase {
			tx := &TxMock{
				DeleteVariableValuesFunc: func(context.Context, string, xid.ID) error {
					return assert.AnError
				},
			}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_listVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:            tx,
				Inp:           textInp,
				Fetches:       1,
				Begins:        1,
				Updated:       true,
				ClearedValues: true,
				Err:           assert.AnError,
			}
		}(),
		"Error returned by Tx.ReplaceVariableOptions": func() tcase {
			tx := &TxMock{
				ReplaceVariableOptionsFunc: func(context.Context, variable.Variable) error {
					return assert.AnError
				},
			}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_listVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:       tx,
				Inp:      listInp,
				Fetches:  1,
				Begins:   1,
				Updated:  true,
				Replaced: true,
				Err:      assert.AnError,
			}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := &TxMock{
				CommitFunc: func() error {
					return assert.AnError
				},
			}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_textVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:      tx,
				Inp:     textInp,
				Fetches: 1,
				Begins:  1,
				Updated: true,
				Commits: 1,
				Err:     assert.AnError,
			}
		}(),
		"Same type keeps values and has no options": func() tcase {
			tx := &TxMock{}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_textVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:      tx,
				Inp:     textInp,
				Fetches: 1,
				Begins:  1,
				Updated: true,
				Commits: 1,
				Changes: 1,
			}
		}(),
		"List keeps values and replaces options": func() tcase {
			tx := &TxMock{}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_listVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:       tx,
				Inp:      listInp,
				Fetches:  1,
				Begins:   1,
				Updated:  true,
				Replaced: true,
				Commits:  1,
				Changes:  1,
			}
		}(),
		"List turned into text clears values and options": func() tcase {
			tx := &TxMock{}

			return tcase{
				DB: &DBMock{
					FetchVariableFunc: stubFetchVariable(_listVariable, nil),
					BeginTxFunc:       testutil.StubBeginTx(tx, nil),
				},
				Tx:            tx,
				Inp:           textInp,
				Fetches:       1,
				Begins:        1,
				Updated:       true,
				ClearedValues: true,
				Replaced:      true,
				Commits:       1,
				Changes:       1,
			}
		}(),
		"Moved to another group": func() tcase {
			tx := &TxMock{}

			inp := textInp
			inp.GroupID = otherGroup.ID

			return tcase{
				DB: &DBMock{
					FetchVariableFunc:      stubFetchVariable(_textVariable, nil),
					FetchVariableGroupFunc: stubFetchGroup(otherGroup, nil),
					BeginTxFunc:            testutil.StubBeginTx(tx, nil),
				},
				Tx:           tx,
				Inp:          inp,
				Fetches:      1,
				GroupFetches: 1,
				Begins:       1,
				Updated:      true,
				Commits:      1,
				Changes:      1,
			}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.UpdateVariable(context.Background(), _textVariable.ID, c.Inp, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), c.Fetches)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), c.GroupFetches)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begins)
			assert.Len(t, rec.changes, c.Changes)

			if c.Tx == nil {
				return
			}

			updates := c.Tx.UpdateVariableCalls()
			if c.Updated {
				require.Len(t, updates, 1)
				assert.Equal(t, c.Inp.Name, updates[0].V.Name)
				assert.Equal(t, c.Inp.GroupID, updates[0].V.GroupID)
			}

			assert.Len(t, c.Tx.DeleteVariableValuesCalls(), map[bool]int{true: 1}[c.ClearedValues])

			replaces := c.Tx.ReplaceVariableOptionsCalls()
			assert.Len(t, replaces, map[bool]int{true: 1}[c.Replaced])

			if c.Replaced && c.Inp.Type == variable.TypeList {
				require.Len(t, replaces[0].V.Options, 2)
				assert.Equal(t, _listOptions[1].ID, replaces[0].V.Options[0].ID)
				assert.Equal(t, null.StringFrom(_listOptions[1].ID.String()), updates[0].V.DefaultValue)
			}

			if c.Replaced && c.Inp.Type != variable.TypeList {
				assert.Empty(t, replaces[0].V.Options)
			}

			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), 1)
		})
	}
}

func Test_Manager_MoveVariable(t *testing.T) {
	t.Parallel()

	sibling := variable.Variable{ID: xid.New(), OrganizationID: _org, GroupID: _group.ID}

	stubSiblings := func(err error) func(context.Context, xid.ID, string) ([]variable.Variable, error) {
		return func(context.Context, xid.ID, string) ([]variable.Variable, error) {
			if err != nil {
				return nil, err
			}

			return []variable.Variable{sibling}, nil
		}
	}

	cc := map[string]struct {
		DB             *DBMock
		Inp            variable.MoveInput
		GroupFetches   int
		SiblingFetches int
		Order          []xid.ID
		Changes        int
		Err            error
	}{
		"Error returned by DB.FetchVariable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Built-in variable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(_builtinVariable, nil)},
			Err: variable.ErrBuiltinVariable,
		},
		"Error returned by DB.FetchVariableGroup": {
			DB: &DBMock{
				FetchVariableFunc:      stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc: stubFetchGroup(variable.Group{}, assert.AnError),
			},
			GroupFetches: 1,
			Err:          assert.AnError,
		},
		"Into the built-in group": {
			DB: &DBMock{
				FetchVariableFunc:      stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc: stubFetchGroup(_builtinGroup, nil),
			},
			GroupFetches: 1,
			Err:          variable.ErrBuiltinGroup,
		},
		"Error returned by DB.FetchVariablesByGroupID": {
			DB: &DBMock{
				FetchVariableFunc:           stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc:      stubFetchGroup(_group, nil),
				FetchVariablesByGroupIDFunc: stubSiblings(assert.AnError),
			},
			GroupFetches:   1,
			SiblingFetches: 1,
			Err:            assert.AnError,
		},
		"Index out of range": {
			DB: &DBMock{
				FetchVariableFunc:           stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc:      stubFetchGroup(_group, nil),
				FetchVariablesByGroupIDFunc: stubSiblings(nil),
			},
			Inp:            variable.MoveInput{ID: _textVariable.ID, GroupID: _group.ID, SortIndex: 2},
			GroupFetches:   1,
			SiblingFetches: 1,
			Err:            variable.ErrInvalidSortIndex,
		},
		"Error returned by DB.UpdateVariableOrder": {
			DB: &DBMock{
				FetchVariableFunc:           stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc:      stubFetchGroup(_group, nil),
				FetchVariablesByGroupIDFunc: stubSiblings(nil),
				UpdateVariableOrderFunc: func(context.Context, string, xid.ID, []xid.ID) error {
					return assert.AnError
				},
			},
			Inp:            variable.MoveInput{ID: _textVariable.ID, GroupID: _group.ID},
			GroupFetches:   1,
			SiblingFetches: 1,
			Order:          []xid.ID{_textVariable.ID, sibling.ID},
			Err:            assert.AnError,
		},
		"Successful move into another group": {
			DB: &DBMock{
				FetchVariableFunc:           stubFetchVariable(_textVariable, nil),
				FetchVariableGroupFunc:      stubFetchGroup(_group, nil),
				FetchVariablesByGroupIDFunc: stubSiblings(nil),
			},
			Inp:            variable.MoveInput{ID: _textVariable.ID, GroupID: _group.ID, SortIndex: 1},
			GroupFetches:   1,
			SiblingFetches: 1,
			Order:          []xid.ID{sibling.ID, _textVariable.ID},
			Changes:        1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.MoveVariable(context.Background(), c.Inp, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), 1)
			assert.Len(t, c.DB.FetchVariableGroupCalls(), c.GroupFetches)
			assert.Len(t, c.DB.FetchVariablesByGroupIDCalls(), c.SiblingFetches)
			assert.Len(t, rec.changes, c.Changes)

			calls := c.DB.UpdateVariableOrderCalls()
			if c.Order == nil {
				assert.Empty(t, calls)
				return
			}

			require.Len(t, calls, 1)
			assert.Equal(t, _group.ID, calls[0].GroupID)
			assert.Equal(t, c.Order, calls[0].Ids)
		})
	}
}

func Test_Manager_DeleteVariable(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB      *DBMock
		Deletes int
		Changes int
		Err     error
	}{
		"Error returned by DB.FetchVariable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Built-in variable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(_builtinVariable, nil)},
			Err: variable.ErrBuiltinVariable,
		},
		"Error returned by DB.DeleteVariable": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_textVariable, nil),
				DeleteVariableFunc: func(context.Context, xid.ID, string) error {
					return assert.AnError
				},
			},
			Deletes: 1,
			Err:     assert.AnError,
		},
		"Successful deletion": {
			DB:      &DBMock{FetchVariableFunc: stubFetchVariable(_textVariable, nil)},
			Deletes: 1,
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.DeleteVariable(context.Background(), _textVariable.ID, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), 1)
			assert.Len(t, c.DB.DeleteVariableCalls(), c.Deletes)
			assert.Len(t, rec.changes, c.Changes)
		})
	}
}

func Test_Manager_SetDefaultValue(t *testing.T) {
	t.Parallel()

	refresh := variable.Variable{
		ID:             xid.New(),
		OrganizationID: _org,
		GroupID:        _builtinGroup.ID,
		Key:            "metric_refresh",
		Type:           variable.TypeList,
		DefaultValue:   null.StringFrom(_listOptions[0].ID.String()),
		Builtin:        true,
		Options:        _listOptions,
	}

	movedRefresh := refresh
	movedRefresh.DefaultValue = null.StringFrom(_listOptions[1].ID.String())
	movedRefresh.Options = []variable.Option{
		{ID: _listOptions[1].ID, VariableID: _listVariable.ID, Value: "us-east-1"},
		{ID: _listOptions[0].ID, VariableID: _listVariable.ID, Value: "eu-west-1", SortIndex: 1},
	}

	weekRange := _builtinVariable
	weekRange.DefaultValue = null.StringFrom("last_7_days")

	cc := map[string]struct {
		DB      *DBMock
		Value   string
		Written []variable.Variable
		Changes int
		Err     error
	}{
		"Error returned by DB.FetchVariable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Invalid value": {
			DB:    &DBMock{FetchVariableFunc: stubFetchVariable(_builtinVariable, nil)},
			Value: "forever",
			Err:   variable.ErrInvalidTimeRange,
		},
		"Error returned by DB.UpdateVariableDefault": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_builtinVariable, nil),
				UpdateVariableDefaultFunc: func(context.Context, variable.Variable) error {
					return assert.AnError
				},
			},
			Value:   "last_7_days",
			Written: []variable.Variable{weekRange},
			Err:     assert.AnError,
		},
		"Built-in time range": {
			DB:      &DBMock{FetchVariableFunc: stubFetchVariable(_builtinVariable, nil)},
			Value:   "last_7_days",
			Written: []variable.Variable{weekRange},
			Changes: 1,
		},
		"Built-in list moves the option to the top": {
			DB:      &DBMock{FetchVariableFunc: stubFetchVariable(refresh, nil)},
			Value:   _listOptions[1].ID.String(),
			Written: []variable.Variable{movedRefresh},
			Changes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.SetDefaultValue(context.Background(), _builtinVariable.ID, c.Value, _org)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), 1)
			assert.Len(t, rec.changes, c.Changes)

			var written []variable.Variable

			for _, call := range c.DB.UpdateVariableDefaultCalls() {
				written = append(written, call.V)
			}

			assert.Equal(t, c.Written, written)
			assert.Empty(t, c.DB.UpdateVariableCalls())
		})
	}
}

func Test_Manager_SetValue(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB      *DBMock
		Value   string
		Upserts int
		Values  int
		Err     error
	}{
		"Error returned by DB.FetchVariable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Invalid value": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_listVariable, nil),
			},
			Value: "eu-west-1",
			Err:   variable.ErrUnknownOption,
		},
		"Error returned by DB.UpsertVariableValue": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_listVariable, nil),
				UpsertVariableValueFunc: func(context.Context, string, string, xid.ID, string) error {
					return assert.AnError
				},
			},
			Value:   _listOptions[1].ID.String(),
			Upserts: 1,
			Err:     assert.AnError,
		},
		"Successful set on a built-in variable": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_builtinVariable, nil),
			},
			Value:   "last_1_hour",
			Upserts: 1,
			Values:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.SetValue(context.Background(), _listVariable.ID, c.Value, _org, _user)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), 1)
			assert.Empty(t, rec.changes)
			assert.Len(t, rec.values, c.Values)

			calls := c.DB.UpsertVariableValueCalls()
			require.Len(t, calls, c.Upserts)

			if c.Upserts == 0 {
				return
			}

			assert.Equal(t, _user, calls[0].UserID)
			assert.Equal(t, c.Value, calls[0].Value)
		})
	}
}

func Test_Manager_ResetValue(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB      *DBMock
		Deletes int
		Values  []string
		Err     error
	}{
		"Error returned by DB.FetchVariable": {
			DB:  &DBMock{FetchVariableFunc: stubFetchVariable(variable.Variable{}, assert.AnError)},
			Err: assert.AnError,
		},
		"Error returned by DB.DeleteVariableValue": {
			DB: &DBMock{
				FetchVariableFunc: stubFetchVariable(_textVariable, nil),
				DeleteVariableValueFunc: func(context.Context, string, string, xid.ID) error {
					return assert.AnError
				},
			},
			Deletes: 1,
			Err:     assert.AnError,
		},
		"Successful reset": {
			DB:      &DBMock{FetchVariableFunc: stubFetchVariable(_textVariable, nil)},
			Deletes: 1,
			Values:  []string{_org + "/" + _user},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, rec := newTestManager(c.DB)

			err := m.ResetValue(context.Background(), _textVariable.ID, _org, _user)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchVariableCalls(), 1)
			assert.Len(t, c.DB.DeleteVariableValueCalls(), c.Deletes)
			assert.Empty(t, rec.changes)
			assert.Equal(t, c.Values, rec.values)
		})
	}
}
