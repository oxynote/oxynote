package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// _testVariableGroupID is the custom group the stubbed tree holds.
	_testVariableGroupID = xid.New()

	// _testVariableID is the list variable the stubbed tree holds.
	_testVariableID = xid.New()

	// _testOptionID is the first option of that list variable.
	_testOptionID = xid.New()

	// _testOtherOptionID is the second option of that list variable.
	_testOtherOptionID = xid.New()

	// _builtinVariableGroupID is the built-in group the stubbed tree holds.
	_builtinVariableGroupID = xid.New()

	// _builtinVariableID is the built-in variable the stubbed tree holds.
	_builtinVariableID = xid.New()

	// _unknownVariableID names nothing in the stubbed tree.
	_unknownVariableID = xid.New()
)

// stubVariableTree returns the tree the stubbed variable manager holds:
// the built-in group with its time range, then a custom group with a
// list variable.
func stubVariableTree() variable.Tree {
	return variable.Tree{
		{
			ID:        _builtinVariableGroupID,
			GroupName: "Metrics",
			Color:     "blue",
			Builtin:   true,
			Variables: []variable.Entry{{
				ID:           _builtinVariableID,
				GroupID:      _builtinVariableGroupID,
				Key:          "metric_range",
				Name:         "Metric time range",
				Type:         variable.TypeTimeRange,
				DefaultValue: null.StringFrom("last_24_hours"),
				Builtin:      true,
				Value:        null.StringFrom("last_7_days"),
				Options:      []variable.Option{},
			}},
		},
		{
			ID:        _testVariableGroupID,
			GroupName: "Environment",
			Color:     "green",
			Variables: []variable.Entry{{
				ID:           _testVariableID,
				GroupID:      _testVariableGroupID,
				Key:          "region",
				Name:         "Region",
				Description:  null.StringFrom("Cloud region"),
				Type:         variable.TypeList,
				DefaultValue: null.StringFrom(_testOptionID.String()),
				Options: []variable.Option{
					{ID: _testOptionID, VariableID: _testVariableID, Value: "eu-west-1"},
					{ID: _testOtherOptionID, VariableID: _testVariableID, Value: "us-east-1", SortIndex: 1},
				},
			}},
		},
	}
}

// stubVariableManager answers tree reads with the stubbed tree and
// accepts every write.
func stubVariableManager() *VariableManagerMock {
	return &VariableManagerMock{
		FetchTreeFunc: func(context.Context, string, string) (variable.Tree, error) {
			return stubVariableTree(), nil
		},
		CreateGroupFunc: func(context.Context, variable.GroupCreateInput, string, string) (*variable.Group, error) {
			return &variable.Group{ID: _testVariableGroupID}, nil
		},
		CreateVariableFunc: func(context.Context, variable.Input, string, string) (*variable.Variable, error) {
			return &variable.Variable{ID: _testVariableID}, nil
		},
	}
}

// failingTreeManager is a variable manager whose tree read fails.
func failingTreeManager() *VariableManagerMock {
	return &VariableManagerMock{
		FetchTreeFunc: func(context.Context, string, string) (variable.Tree, error) {
			return nil, assert.AnError
		},
	}
}

// variableDeps returns test wiring over the variable manager.
func variableDeps(man *VariableManagerMock) *Deps {
	d := testDeps(nil, nil, nil)
	d.variableMan = man

	return d
}

func Test_listVariablesArgs_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listVariablesArgs{}.Validate())
}

func Test_listVariables_Info(t *testing.T) {
	t.Parallel()

	info := listVariables{}.Info()

	assert.Equal(t, Traits{Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameListVariables, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func Test_listVariables_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man            *VariableManagerMock
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Error returned by VariableManager.FetchTree": {
			Man:            failingTreeManager(),
			Args:           `{}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Successful listing": {
			Man:            stubVariableManager(),
			Args:           `{}`,
			FetchTreeCalls: 1,
			Result: `{"groups":[
				{"id":"` + _builtinVariableGroupID.String() + `","name":"Metrics","color":"blue","builtin":true,"variables":[
					{"id":"` + _builtinVariableID.String() + `","key":"metric_range","name":"Metric time range","description":null,"type":"time-range","builtin":true,"default":"last_24_hours","value":"last_7_days","options":[]}
				]},
				{"id":"` + _testVariableGroupID.String() + `","name":"Environment","color":"green","builtin":false,"variables":[
					{"id":"` + _testVariableID.String() + `","key":"region","name":"Region","description":"Cloud region","type":"list","builtin":false,"default":"` + _testOptionID.String() + `","value":null,"options":[
						{"id":"` + _testOptionID.String() + `","value":"eu-west-1"},
						{"id":"` + _testOtherOptionID.String() + `","value":"us-east-1"}
					]}
				]}
			]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listVariables{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameListVariables, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_createVariableGroupArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args createVariableGroupArgs
		Err  error
	}{
		"Missing name": {
			Args: createVariableGroupArgs{ColorName: "orange"},
			Err:  errors.New("name is required"),
		},
		"Missing colour": {
			Args: createVariableGroupArgs{Name: "Deploys"},
			Err:  errors.New("color is required"),
		},
		"Colour outside the palette": {
			Args: createVariableGroupArgs{Name: "Deploys", ColorName: "beige"},
			Err:  variable.ErrInvalidGroupColor,
		},
		"Complete arguments": {
			Args: createVariableGroupArgs{Name: "Deploys", ColorName: "orange"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_createVariableGroupArgs_input(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"},
		createVariableGroupArgs{Name: "Deploys", ColorName: "orange"}.input(),
	)
}

func Test_createVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := createVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameCreateVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"name", "color"}, info.Required)

	prop, ok := info.Properties["color"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, tag.ColorNames(), prop["enum"])
}

func Test_createVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   string
		Result string
		Err    error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"New group": {
			Args:   `{"name":"Deploys","color":"orange"}`,
			Result: "Create variable group Deploys (orange)",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := createVariableGroup{}.Summary(variableDeps(stubVariableManager()).newInput(context.Background(), NameCreateVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameCreateVariableGroup, Summary: c.Result}, got)
		})
	}
}

func Test_createVariableGroup_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man              *VariableManagerMock
		Args             string
		CreateGroupCalls int
		Err              error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Error returned by VariableManager.CreateGroup": {
			Man: &VariableManagerMock{
				CreateGroupFunc: func(context.Context, variable.GroupCreateInput, string, string) (*variable.Group, error) {
					return nil, assert.AnError
				},
			},
			Args:             `{"name":"Deploys","color":"orange"}`,
			CreateGroupCalls: 1,
			Err:              assert.AnError,
		},
		"Created": {
			Man:              stubVariableManager(),
			Args:             `{"name":"Deploys","color":"orange"}`,
			CreateGroupCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := createVariableGroup{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameCreateVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.CreateGroupCalls()
			require.Len(t, ff, c.CreateGroupCalls)

			if err != nil {
				return
			}

			assert.Equal(t, variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"}, ff[0].Inp)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, "user", ff[0].UserID)
			assert.JSONEq(t, `{"group_id":"`+_testVariableGroupID.String()+`"}`, res)
		})
	}
}

func Test_updateVariableGroupArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args updateVariableGroupArgs
		Err  error
	}{
		"Missing group id": {
			Args: updateVariableGroupArgs{Name: "Env"},
			Err:  errors.New("group_id is required"),
		},
		"Nothing to change": {
			Args: updateVariableGroupArgs{GroupID: _testVariableGroupID},
			Err:  errors.New("give at least one of name or color"),
		},
		"Colour outside the palette": {
			Args: updateVariableGroupArgs{GroupID: _testVariableGroupID, ColorName: "beige"},
			Err:  variable.ErrInvalidGroupColor,
		},
		"Complete arguments": {
			Args: updateVariableGroupArgs{GroupID: _testVariableGroupID, Name: "Env"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_updateVariableGroupArgs_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   updateVariableGroupArgs
		Result variable.GroupUpdateInput
	}{
		"Name given": {
			Args:   updateVariableGroupArgs{Name: "Env"},
			Result: variable.GroupUpdateInput{GroupName: null.StringFrom("Env")},
		},
		"Colour given": {
			Args:   updateVariableGroupArgs{ColorName: "orange"},
			Result: variable.GroupUpdateInput{Color: null.StringFrom("orange")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.input())
		})
	}
}

func Test_updateVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := updateVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameUpdateVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, "color")
}

func Test_updateVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	group := `"group_id":"` + _testVariableGroupID.String() + `"`

	cc := map[string]struct {
		Man            *VariableManagerMock
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Error returned by VariableManager.FetchTree": {
			Man:            failingTreeManager(),
			Args:           `{` + group + `,"name":"Env"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Unknown group": {
			Man:            stubVariableManager(),
			Args:           `{"group_id":"` + _unknownVariableID.String() + `","name":"Env"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Rename and recolour": {
			Man:            stubVariableManager(),
			Args:           `{` + group + `,"name":"Env","color":"orange"}`,
			FetchTreeCalls: 1,
			Result:         `Rename variable group Environment to "Env" and set the colour of variable group Environment to orange`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateVariableGroup{}.Summary(variableDeps(c.Man).newInput(context.Background(), NameUpdateVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameUpdateVariableGroup, Summary: c.Result}, got)
		})
	}
}

func Test_updateVariableGroup_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man              *VariableManagerMock
		Args             string
		UpdateGroupCalls int
		Err              error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Unknown group": {
			Man: &VariableManagerMock{
				UpdateGroupFunc: func(context.Context, xid.ID, variable.GroupUpdateInput, string) error {
					return errutil.ErrNotFound
				},
			},
			Args:             `{"group_id":"` + _testVariableGroupID.String() + `","name":"Env"}`,
			UpdateGroupCalls: 1,
			Err:              fmt.Errorf("variable group %s: %w", _testVariableGroupID, ErrUnknownVariableGroup),
		},
		"Updated": {
			Man:              stubVariableManager(),
			Args:             `{"group_id":"` + _testVariableGroupID.String() + `","name":"Env"}`,
			UpdateGroupCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := updateVariableGroup{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameUpdateVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.UpdateGroupCalls()
			require.Len(t, ff, c.UpdateGroupCalls)

			if err != nil {
				return
			}

			assert.Equal(t, _testVariableGroupID, ff[0].ID)
			assert.Equal(t, variable.GroupUpdateInput{GroupName: null.StringFrom("Env")}, ff[0].Inp)
			assert.JSONEq(t, `{"group_id":"`+_testVariableGroupID.String()+`"}`, res)
		})
	}
}

func Test_deleteVariableGroupArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args deleteVariableGroupArgs
		Err  error
	}{
		"Missing group id": {
			Args: deleteVariableGroupArgs{},
			Err:  errors.New("group_id is required"),
		},
		"Complete arguments": {
			Args: deleteVariableGroupArgs{GroupID: _testVariableGroupID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_deleteVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := deleteVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameDeleteVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id"}, info.Required)
}

func Test_deleteVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"Unknown group": {
			Args:           `{"group_id":"` + _unknownVariableID.String() + `"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Known group": {
			Args:           `{"group_id":"` + _testVariableGroupID.String() + `"}`,
			FetchTreeCalls: 1,
			Result:         "Delete variable group Environment",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := stubVariableManager()

			got, err := deleteVariableGroup{}.Summary(variableDeps(man).newInput(context.Background(), NameDeleteVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteVariableGroup, Summary: c.Result}, got)
		})
	}
}

func Test_deleteVariableGroup_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man              *VariableManagerMock
		Args             string
		DeleteGroupCalls int
		Err              error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Group with variables": {
			Man: &VariableManagerMock{
				DeleteGroupFunc: func(context.Context, xid.ID, string) error {
					return variable.ErrGroupNotEmpty
				},
			},
			Args:             `{"group_id":"` + _testVariableGroupID.String() + `"}`,
			DeleteGroupCalls: 1,
			Err:              variable.ErrGroupNotEmpty,
		},
		"Deleted": {
			Man:              stubVariableManager(),
			Args:             `{"group_id":"` + _testVariableGroupID.String() + `"}`,
			DeleteGroupCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteVariableGroup{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameDeleteVariableGroup, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.DeleteGroupCalls()
			require.Len(t, ff, c.DeleteGroupCalls)

			if err != nil {
				return
			}

			assert.Equal(t, _testVariableGroupID, ff[0].ID)
			assert.JSONEq(t, `{"group_id":"`+_testVariableGroupID.String()+`","deleted":true}`, res)
		})
	}
}

func Test_createVariableArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args createVariableArgs
		Err  error
	}{
		"Missing group id": {
			Args: createVariableArgs{Key: "release", Name: "Release", Type: variable.TypeText},
			Err:  errors.New("group_id is required"),
		},
		"Missing key": {
			Args: createVariableArgs{GroupID: _testVariableGroupID, Name: "Release", Type: variable.TypeText},
			Err:  errors.New("key is required"),
		},
		"Missing name": {
			Args: createVariableArgs{GroupID: _testVariableGroupID, Key: "release", Type: variable.TypeText},
			Err:  errors.New("name is required"),
		},
		"Missing type": {
			Args: createVariableArgs{GroupID: _testVariableGroupID, Key: "release", Name: "Release"},
			Err:  errors.New("type is required"),
		},
		"Invalid key": {
			Args: createVariableArgs{GroupID: _testVariableGroupID, Key: "$release", Name: "Release", Type: variable.TypeText},
			Err:  variable.ErrInvalidKey,
		},
		"Complete arguments": {
			Args: createVariableArgs{GroupID: _testVariableGroupID, Key: "release", Name: "Release", Type: variable.TypeText},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_createVariableArgs_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args   createVariableArgs
		Result variable.Input
	}{
		"List with a description": {
			Args: createVariableArgs{
				GroupID:     _testVariableGroupID,
				Key:         "region",
				Name:        "Region",
				Description: "Cloud region",
				Type:        variable.TypeList,
				Options:     []string{"eu-west-1", "us-east-1"},
			},
			Result: variable.Input{
				GroupID:     _testVariableGroupID,
				Key:         "region",
				Name:        "Region",
				Description: null.StringFrom("Cloud region"),
				Type:        variable.TypeList,
				Options:     []variable.OptionInput{{Value: "eu-west-1"}, {Value: "us-east-1"}},
			},
		},
		"Default without a description or options": {
			Args:   createVariableArgs{Type: variable.TypeNumber, Default: null.StringFrom("2")},
			Result: variable.Input{Type: variable.TypeNumber, DefaultValue: null.StringFrom("2")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.input())
		})
	}
}

func Test_createVariable_Info(t *testing.T) {
	t.Parallel()

	info := createVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameCreateVariable, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id", "key", "name", "type"}, info.Required)
	assert.Contains(t, info.Properties, "options")
	assert.Contains(t, info.Properties, "default")
}

func Test_createVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"Unknown group": {
			Args:           `{"group_id":"` + _unknownVariableID.String() + `","key":"release","name":"Release","type":"text"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Known group": {
			Args:           `{"group_id":"` + _testVariableGroupID.String() + `","key":"release","name":"Release","type":"text"}`,
			FetchTreeCalls: 1,
			Result:         "Create text variable $release in Environment",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := stubVariableManager()

			got, err := createVariable{}.Summary(variableDeps(man).newInput(context.Background(), NameCreateVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameCreateVariable, Summary: c.Result}, got)
		})
	}
}

func Test_createVariable_Execute(t *testing.T) {
	t.Parallel()

	args := `{"group_id":"` + _testVariableGroupID.String() + `","key":"region","name":"Region","type":"list","options":["eu-west-1"]}`

	cc := map[string]struct {
		Man                 *VariableManagerMock
		Args                string
		CreateVariableCalls int
		Err                 error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Unknown group": {
			Man: &VariableManagerMock{
				CreateVariableFunc: func(context.Context, variable.Input, string, string) (*variable.Variable, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Args:                args,
			CreateVariableCalls: 1,
			Err:                 fmt.Errorf("variable group %s: %w", _testVariableGroupID, ErrUnknownVariableGroup),
		},
		"Created": {
			Man:                 stubVariableManager(),
			Args:                args,
			CreateVariableCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := createVariable{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameCreateVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.CreateVariableCalls()
			require.Len(t, ff, c.CreateVariableCalls)

			if err != nil {
				return
			}

			assert.Equal(t, "region", ff[0].Inp.Key)
			assert.Equal(t, []variable.OptionInput{{Value: "eu-west-1"}}, ff[0].Inp.Options)
			assert.Equal(t, "user", ff[0].UserID)
			assert.JSONEq(t, `{"variable_id":"`+_testVariableID.String()+`"}`, res)
		})
	}
}

func Test_updateVariableArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args updateVariableArgs
		Err  error
	}{
		"Missing variable id": {
			Args: updateVariableArgs{},
			Err:  errors.New("variable_id is required"),
		},
		"Invalid type": {
			Args: updateVariableArgs{VariableID: _testVariableID, Type: "date"},
			Err:  variable.ErrInvalidType,
		},
		"Valid type": {
			Args: updateVariableArgs{VariableID: _testVariableID, Type: variable.TypeText},
		},
		"Only the variable id": {
			Args: updateVariableArgs{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_updateVariableArgs_input(t *testing.T) {
	t.Parallel()

	list := stubVariableTree()[1].Variables[0]
	builtin := stubVariableTree()[0].Variables[0]
	otherGroup := xid.New()

	cc := map[string]struct {
		Args    updateVariableArgs
		Current variable.Entry
		Result  variable.Input
	}{
		"Nothing given keeps a list as it is": {
			Current: list,
			Result: variable.Input{
				GroupID:     _testVariableGroupID,
				Key:         "region",
				Name:        "Region",
				Description: null.StringFrom("Cloud region"),
				Type:        variable.TypeList,
				Options: []variable.OptionInput{
					{ID: null.ValueFrom(_testOptionID), Value: "eu-west-1"},
					{ID: null.ValueFrom(_testOtherOptionID), Value: "us-east-1"},
				},
			},
		},
		"New options keep the ids of values the list has": {
			Args:    updateVariableArgs{GroupID: otherGroup, Key: "zone", Name: "Zone", Description: null.StringFrom(""), Options: []string{"us-east-1", "ap-south-1"}},
			Current: list,
			Result: variable.Input{
				GroupID: otherGroup,
				Key:     "zone",
				Name:    "Zone",
				Type:    variable.TypeList,
				Options: []variable.OptionInput{
					{ID: null.ValueFrom(_testOtherOptionID), Value: "us-east-1"},
					{Value: "ap-south-1"},
				},
			},
		},
		"Repeated values name one option once": {
			Args:    updateVariableArgs{Options: []string{"us-east-1", "us-east-1"}},
			Current: list,
			Result: variable.Input{
				GroupID:     _testVariableGroupID,
				Key:         "region",
				Name:        "Region",
				Description: null.StringFrom("Cloud region"),
				Type:        variable.TypeList,
				Options:     []variable.OptionInput{{ID: null.ValueFrom(_testOtherOptionID), Value: "us-east-1"}},
			},
		},
		"List turned into text drops its options": {
			Args:    updateVariableArgs{Type: variable.TypeText, Default: null.StringFrom("eu")},
			Current: list,
			Result: variable.Input{
				GroupID:      _testVariableGroupID,
				Key:          "region",
				Name:         "Region",
				Description:  null.StringFrom("Cloud region"),
				Type:         variable.TypeText,
				DefaultValue: null.StringFrom("eu"),
			},
		},
		"Same type keeps the default": {
			Args:    updateVariableArgs{Description: null.StringFrom("Window")},
			Current: builtin,
			Result: variable.Input{
				GroupID:      _builtinVariableGroupID,
				Key:          "metric_range",
				Name:         "Metric time range",
				Description:  null.StringFrom("Window"),
				Type:         variable.TypeTimeRange,
				DefaultValue: null.StringFrom("last_24_hours"),
			},
		},
		"Type change drops the old default": {
			Args:    updateVariableArgs{Type: variable.TypeText},
			Current: builtin,
			Result: variable.Input{
				GroupID: _builtinVariableGroupID,
				Key:     "metric_range",
				Name:    "Metric time range",
				Type:    variable.TypeText,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Args.input(c.Current))
		})
	}
}

func Test_updateVariable_Info(t *testing.T) {
	t.Parallel()

	info := updateVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true, Idempotent: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameUpdateVariable, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"variable_id"}, info.Required)

	// every optional argument says what it does, not only that it is
	// optional.
	for key, prop := range info.Properties {
		p, ok := prop.(map[string]any)
		require.True(t, ok)
		assert.NotEqual(t, "Optional.", p["description"], key)
	}
}

func Test_updateVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"Unknown variable": {
			Args:           `{"variable_id":"` + _unknownVariableID.String() + `"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Same type": {
			Args:           `{"variable_id":"` + _testVariableID.String() + `","name":"Zone"}`,
			FetchTreeCalls: 1,
			Result:         "Update variable $region",
		},
		"Type change": {
			Args:           `{"variable_id":"` + _testVariableID.String() + `","type":"text"}`,
			FetchTreeCalls: 1,
			Result:         "Update variable $region and make it text, resetting everyone's value",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := stubVariableManager()

			got, err := updateVariable{}.Summary(variableDeps(man).newInput(context.Background(), NameUpdateVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameUpdateVariable, Summary: c.Result}, got)
		})
	}
}

func Test_updateVariable_Execute(t *testing.T) {
	t.Parallel()

	args := `{"variable_id":"` + _testVariableID.String() + `","name":"Zone"}`

	withUpdate := func(err error) *VariableManagerMock {
		m := stubVariableManager()
		m.UpdateVariableFunc = func(context.Context, xid.ID, variable.Input, string) error {
			return err
		}

		return m
	}

	cc := map[string]struct {
		Man                 *VariableManagerMock
		Args                string
		FetchTreeCalls      int
		UpdateVariableCalls int
		Err                 error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Unknown variable": {
			Man:            stubVariableManager(),
			Args:           `{"variable_id":"` + _unknownVariableID.String() + `"}`,
			FetchTreeCalls: 1,
			Err:            fmt.Errorf("variable %s: %w", _unknownVariableID, ErrUnknownVariable),
		},
		"Built-in variable": {
			Man:                 withUpdate(variable.ErrBuiltinVariable),
			Args:                args,
			FetchTreeCalls:      1,
			UpdateVariableCalls: 1,
			Err:                 variable.ErrBuiltinVariable,
		},
		"Updated": {
			Man:                 withUpdate(nil),
			Args:                args,
			FetchTreeCalls:      1,
			UpdateVariableCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := updateVariable{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameUpdateVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), c.FetchTreeCalls)

			ff := c.Man.UpdateVariableCalls()
			require.Len(t, ff, c.UpdateVariableCalls)

			if err != nil {
				return
			}

			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.Equal(t, "Zone", ff[0].Inp.Name)
			assert.Len(t, ff[0].Inp.Options, 2)
			assert.JSONEq(t, `{"variable_id":"`+_testVariableID.String()+`"}`, res)
		})
	}
}

func Test_deleteVariableArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args deleteVariableArgs
		Err  error
	}{
		"Missing variable id": {
			Args: deleteVariableArgs{},
			Err:  errors.New("variable_id is required"),
		},
		"Complete arguments": {
			Args: deleteVariableArgs{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_deleteVariable_Info(t *testing.T) {
	t.Parallel()

	info := deleteVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameDeleteVariable, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"variable_id"}, info.Required)
}

func Test_deleteVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"Unknown variable":    {Args: `{"variable_id":"` + _unknownVariableID.String() + `"}`, FetchTreeCalls: 1, Err: assert.AnError},
		"Known variable": {
			Args:           `{"variable_id":"` + _testVariableID.String() + `"}`,
			FetchTreeCalls: 1,
			Result:         "Delete variable $region",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := stubVariableManager()

			got, err := deleteVariable{}.Summary(variableDeps(man).newInput(context.Background(), NameDeleteVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteVariable, Summary: c.Result}, got)
		})
	}
}

func Test_deleteVariable_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man                 *VariableManagerMock
		Args                string
		DeleteVariableCalls int
		Err                 error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Unknown variable": {
			Man: &VariableManagerMock{
				DeleteVariableFunc: func(context.Context, xid.ID, string) error {
					return errutil.ErrNotFound
				},
			},
			Args:                `{"variable_id":"` + _testVariableID.String() + `"}`,
			DeleteVariableCalls: 1,
			Err:                 fmt.Errorf("variable %s: %w", _testVariableID, ErrUnknownVariable),
		},
		"Deleted": {
			Man:                 stubVariableManager(),
			Args:                `{"variable_id":"` + _testVariableID.String() + `"}`,
			DeleteVariableCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteVariable{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameDeleteVariable, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.DeleteVariableCalls()
			require.Len(t, ff, c.DeleteVariableCalls)

			if err != nil {
				return
			}

			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.JSONEq(t, `{"variable_id":"`+_testVariableID.String()+`","deleted":true}`, res)
		})
	}
}

func Test_setVariableValueArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args setVariableValueArgs
		Err  error
	}{
		"Missing variable id": {
			Args: setVariableValueArgs{Value: null.StringFrom("x")},
			Err:  errors.New("variable_id is required"),
		},
		"Missing value": {
			Args: setVariableValueArgs{VariableID: _testVariableID},
			Err:  errors.New("value is required"),
		},
		"Empty default": {
			Args: setVariableValueArgs{VariableID: _testVariableID, Value: null.StringFrom(""), Default: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
}

func Test_setVariableValue_Info(t *testing.T) {
	t.Parallel()

	info := setVariableValue{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupVariables}, info.Traits)
	assert.Equal(t, NameSetVariableValue, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"variable_id", "value"}, info.Required)
}

func Test_setVariableValue_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args           string
		FetchTreeCalls int
		Result         string
		Err            error
	}{
		"Malformed arguments": {Args: `{`, Err: assert.AnError},
		"Unknown variable": {
			Args:           `{"variable_id":"` + _unknownVariableID.String() + `","value":"x"}`,
			FetchTreeCalls: 1,
			Err:            assert.AnError,
		},
		"Option id shown as its value": {
			Args:           `{"variable_id":"` + _testVariableID.String() + `","value":"` + _testOtherOptionID.String() + `"}`,
			FetchTreeCalls: 1,
			Result:         `Set your $region to "us-east-1"`,
		},
		"Plain value": {
			Args:           `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour"}`,
			FetchTreeCalls: 1,
			Result:         `Set your $metric_range to "last_1_hour"`,
		},
		"Default": {
			Args:           `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_7_days","default":true}`,
			FetchTreeCalls: 1,
			Result:         `Set the default of $metric_range to "last_7_days"`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man := stubVariableManager()

			got, err := setVariableValue{}.Summary(variableDeps(man).newInput(context.Background(), NameSetVariableValue, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.FetchTreeCalls(), c.FetchTreeCalls)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameSetVariableValue, Summary: c.Result}, got)
		})
	}
}

func Test_setVariableValue_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man          *VariableManagerMock
		Args         string
		ValueCalls   int
		DefaultCalls int
		Result       string
		Err          error
	}{
		"Malformed arguments": {Man: stubVariableManager(), Args: `{`, Err: assert.AnError},
		"Invalid value": {
			Man: &VariableManagerMock{
				SetValueFunc: func(context.Context, xid.ID, string, string, string) error {
					return variable.ErrInvalidTimeRange
				},
			},
			Args:       `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour"}`,
			ValueCalls: 1,
			Err:        variable.ErrInvalidTimeRange,
		},
		"Invalid default": {
			Man: &VariableManagerMock{
				SetDefaultValueFunc: func(context.Context, xid.ID, string, string) error {
					return variable.ErrInvalidTimeRange
				},
			},
			Args:         `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour","default":true}`,
			DefaultCalls: 1,
			Err:          variable.ErrInvalidTimeRange,
		},
		"Set": {
			Man:        stubVariableManager(),
			Args:       `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour"}`,
			ValueCalls: 1,
			Result:     `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour","default":false}`,
		},
		"Set the default": {
			Man:          stubVariableManager(),
			Args:         `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour","default":true}`,
			DefaultCalls: 1,
			Result:       `{"variable_id":"` + _builtinVariableID.String() + `","value":"last_1_hour","default":true}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := setVariableValue{}.Execute(variableDeps(c.Man).newInput(context.Background(), NameSetVariableValue, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			vv := c.Man.SetValueCalls()
			require.Len(t, vv, c.ValueCalls)

			dd := c.Man.SetDefaultValueCalls()
			require.Len(t, dd, c.DefaultCalls)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)

			for _, v := range vv {
				assert.Equal(t, _builtinVariableID, v.ID)
				assert.Equal(t, "last_1_hour", v.Value)
				assert.Equal(t, "org", v.OrganizationID)
				assert.Equal(t, "user", v.UserID)
			}

			for _, d := range dd {
				assert.Equal(t, _builtinVariableID, d.ID)
				assert.Equal(t, "last_1_hour", d.Value)
				assert.Equal(t, "org", d.OrganizationID)
			}
		})
	}
}
