package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
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

// stubVariableInput answers variable reads from the stubbed tree: the
// custom group and its list variable. It accepts every write.
func stubVariableInput() *InputMock {
	return &InputMock{
		FetchVariableTreeFunc: func() (variable.Tree, error) {
			return stubVariableTree(), nil
		},
		FetchVariableGroupFunc: func(xid.ID) (*variable.GroupTree, error) {
			return &stubVariableTree()[1], nil
		},
		FetchVariableFunc: func(xid.ID) (*variable.Entry, error) {
			return &stubVariableTree()[1].Variables[0], nil
		},
		CreateVariableGroupFunc: func(variable.GroupCreateInput) (*variable.Group, error) {
			return &variable.Group{ID: _testVariableGroupID}, nil
		},
		CreateVariableFunc: func(variable.Input) (*variable.Variable, error) {
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

func Test_listVariables_Info(t *testing.T) {
	t.Parallel()

	info := listVariables{}.Info()

	assert.Equal(t, Traits{Group: GroupVariables, Role: org.RoleViewer}, info.Traits)
	assert.Equal(t, NameListVariables, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func Test_listVariables_Validate(t *testing.T) {
	t.Parallel()

	assert.NoError(t, listVariables{}.Validate())
}

func Test_listVariables_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.FetchVariableTree": {
			Input: &InputMock{
				FetchVariableTreeFunc: func() (variable.Tree, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Successful listing": {
			Input: stubVariableInput(),
			Result: listVariablesResult{Groups: []variableGroupEntry{
				{
					ID:        _builtinVariableGroupID,
					Name:      "Metrics",
					ColorName: "blue",
					Builtin:   true,
					Variables: []variableEntry{{
						ID:      _builtinVariableID,
						Key:     "metric_range",
						Name:    "Metric time range",
						Type:    variable.TypeTimeRange,
						Builtin: true,
						Default: null.StringFrom("last_24_hours"),
						Value:   null.StringFrom("last_7_days"),
						Options: []variableOptionEntry{},
					}},
				},
				{
					ID:        _testVariableGroupID,
					Name:      "Environment",
					ColorName: "green",
					Variables: []variableEntry{{
						ID:          _testVariableID,
						Key:         "region",
						Name:        "Region",
						Description: null.StringFrom("Cloud region"),
						Type:        variable.TypeList,
						Default:     null.StringFrom(_testOptionID.String()),
						Options: []variableOptionEntry{
							{ID: _testOptionID, Value: "eu-west-1"},
							{ID: _testOtherOptionID, Value: "us-east-1"},
						},
					}},
				},
			}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := listVariables{}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableTreeCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := createVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
	assert.Equal(t, NameCreateVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"name", "color"}, info.Required)

	prop, ok := info.Properties["color"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, tag.ColorNames(), prop["enum"])
}

func Test_createVariableGroup_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool createVariableGroup
		Err  error
	}{
		"Missing name": {
			Tool: createVariableGroup{ColorName: "orange"},
			Err:  errors.New("name is required"),
		},
		"Missing colour": {
			Tool: createVariableGroup{Name: "Deploys"},
			Err:  errors.New("color is required"),
		},
		"Colour outside the palette": {
			Tool: createVariableGroup{Name: "Deploys", ColorName: "beige"},
			Err:  variable.ErrInvalidGroupColor,
		},
		"Complete arguments": {
			Tool: createVariableGroup{Name: "Deploys", ColorName: "orange"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_createVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	got, err := createVariableGroup{Name: "Deploys", ColorName: "orange"}.Summary(&InputMock{})
	require.NoError(t, err)

	assert.Equal(t, ActionSummary{Tool: NameCreateVariableGroup, Summary: "Create variable group Deploys (orange)"}, got)
}

func Test_createVariableGroup_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.CreateVariableGroup": {
			Input: &InputMock{
				CreateVariableGroupFunc: func(variable.GroupCreateInput) (*variable.Group, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Created": {
			Input:  stubVariableInput(),
			Result: variableGroupResult{GroupID: _testVariableGroupID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := createVariableGroup{Name: "Deploys", ColorName: "orange"}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.CreateVariableGroupCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"}, ff[0].Inp)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createVariableGroup_input(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"},
		createVariableGroup{Name: "Deploys", ColorName: "orange"}.input(),
	)
}

func Test_updateVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := updateVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
	assert.Equal(t, NameUpdateVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id"}, info.Required)
	assert.Contains(t, info.Properties, "name")
	assert.Contains(t, info.Properties, "color")
}

func Test_updateVariableGroup_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool updateVariableGroup
		Err  error
	}{
		"Missing group id": {
			Tool: updateVariableGroup{Name: "Env"},
			Err:  errors.New("group_id is required"),
		},
		"Nothing to change": {
			Tool: updateVariableGroup{GroupID: _testVariableGroupID},
			Err:  errors.New("give at least one of name or color"),
		},
		"Colour outside the palette": {
			Tool: updateVariableGroup{GroupID: _testVariableGroupID, ColorName: "beige"},
			Err:  variable.ErrInvalidGroupColor,
		},
		"Complete arguments": {
			Tool: updateVariableGroup{GroupID: _testVariableGroupID, Name: "Env"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_updateVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariableGroup": {
			Input: &InputMock{
				FetchVariableGroupFunc: func(xid.ID) (*variable.GroupTree, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Rename and recolour": {
			Input:  stubVariableInput(),
			Result: `Rename variable group Environment to "Env" and set the colour of variable group Environment to orange`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := updateVariableGroup{GroupID: _testVariableGroupID, Name: "Env", ColorName: "orange"}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableGroupCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameUpdateVariableGroup, Summary: c.Result}, got)
		})
	}
}

func Test_updateVariableGroup_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.UpdateVariableGroup": {
			Input: &InputMock{
				UpdateVariableGroupFunc: func(xid.ID, variable.GroupUpdateInput) error { return assert.AnError },
			},
			Err: assert.AnError,
		},
		"Updated": {
			Input:  &InputMock{},
			Result: variableGroupResult{GroupID: _testVariableGroupID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := updateVariableGroup{GroupID: _testVariableGroupID, Name: "Env"}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.UpdateVariableGroupCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableGroupID, ff[0].GroupID)
			assert.Equal(t, variable.GroupUpdateInput{GroupName: null.StringFrom("Env")}, ff[0].Inp)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_updateVariableGroup_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   updateVariableGroup
		Result variable.GroupUpdateInput
	}{
		"Name given": {
			Tool:   updateVariableGroup{Name: "Env"},
			Result: variable.GroupUpdateInput{GroupName: null.StringFrom("Env")},
		},
		"Colour given": {
			Tool:   updateVariableGroup{ColorName: "orange"},
			Result: variable.GroupUpdateInput{Color: null.StringFrom("orange")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.input())
		})
	}
}

func Test_deleteVariableGroup_Info(t *testing.T) {
	t.Parallel()

	info := deleteVariableGroup{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
	assert.Equal(t, NameDeleteVariableGroup, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id"}, info.Required)
}

func Test_deleteVariableGroup_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool deleteVariableGroup
		Err  error
	}{
		"Missing group id": {
			Tool: deleteVariableGroup{},
			Err:  errors.New("group_id is required"),
		},
		"Complete arguments": {
			Tool: deleteVariableGroup{GroupID: _testVariableGroupID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_deleteVariableGroup_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariableGroup": {
			Input: &InputMock{
				FetchVariableGroupFunc: func(xid.ID) (*variable.GroupTree, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Known group": {
			Input:  stubVariableInput(),
			Result: "Delete variable group Environment",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteVariableGroup{GroupID: _testVariableGroupID}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableGroupCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteVariableGroup, Summary: c.Result}, got)
		})
	}
}

func Test_deleteVariableGroup_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.DeleteVariableGroup": {
			Input: &InputMock{DeleteVariableGroupFunc: func(xid.ID) error { return assert.AnError }},
			Err:   assert.AnError,
		},
		"Deleted": {
			Input:  &InputMock{},
			Result: deleteVariableGroupResult{GroupID: _testVariableGroupID, Deleted: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteVariableGroup{GroupID: _testVariableGroupID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.DeleteVariableGroupCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableGroupID, ff[0].GroupID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createVariable_Info(t *testing.T) {
	t.Parallel()

	info := createVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
	assert.Equal(t, NameCreateVariable, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"group_id", "key", "name", "type"}, info.Required)
	assert.Contains(t, info.Properties, "options")
	assert.Contains(t, info.Properties, "default")
}

func Test_createVariable_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool createVariable
		Err  error
	}{
		"Missing group id": {
			Tool: createVariable{Key: "release", Name: "Release", Type: variable.TypeText},
			Err:  errors.New("group_id is required"),
		},
		"Missing key": {
			Tool: createVariable{GroupID: _testVariableGroupID, Name: "Release", Type: variable.TypeText},
			Err:  errors.New("key is required"),
		},
		"Missing name": {
			Tool: createVariable{GroupID: _testVariableGroupID, Key: "release", Type: variable.TypeText},
			Err:  errors.New("name is required"),
		},
		"Missing type": {
			Tool: createVariable{GroupID: _testVariableGroupID, Key: "release", Name: "Release"},
			Err:  errors.New("type is required"),
		},
		"Invalid key": {
			Tool: createVariable{GroupID: _testVariableGroupID, Key: "$release", Name: "Release", Type: variable.TypeText},
			Err:  variable.ErrInvalidKey,
		},
		"Repeated option values": {
			Tool: createVariable{GroupID: _testVariableGroupID, Key: "region", Name: "Region", Type: variable.TypeList, Options: []string{"a", "a"}},
			Err:  variable.ErrDuplicateOptionValue,
		},
		"Complete arguments": {
			Tool: createVariable{GroupID: _testVariableGroupID, Key: "release", Name: "Release", Type: variable.TypeText},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_createVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariableGroup": {
			Input: &InputMock{
				FetchVariableGroupFunc: func(xid.ID) (*variable.GroupTree, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Known group": {
			Input:  stubVariableInput(),
			Result: "Create text variable $release in Environment",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := createVariable{GroupID: _testVariableGroupID, Key: "release", Type: variable.TypeText}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableGroupCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameCreateVariable, Summary: c.Result}, got)
		})
	}
}

func Test_createVariable_Exec(t *testing.T) {
	t.Parallel()

	tool := createVariable{GroupID: _testVariableGroupID, Key: "release", Name: "Release", Type: variable.TypeText}

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.CreateVariable": {
			Input: &InputMock{
				CreateVariableFunc: func(variable.Input) (*variable.Variable, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Created": {
			Input:  stubVariableInput(),
			Result: variableResult{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.CreateVariableCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, tool.input(), ff[0].Inp)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_createVariable_input(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool   createVariable
		Result variable.Input
	}{
		"List with a description": {
			Tool: createVariable{
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
			Tool:   createVariable{Type: variable.TypeNumber, Default: null.StringFrom("2")},
			Result: variable.Input{Type: variable.TypeNumber, DefaultValue: null.StringFrom("2")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Tool.input())
		})
	}
}

func Test_updateVariable_Info(t *testing.T) {
	t.Parallel()

	info := updateVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true, Idempotent: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
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

func Test_updateVariable_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool updateVariable
		Err  error
	}{
		"Missing variable id": {
			Tool: updateVariable{},
			Err:  errors.New("variable_id is required"),
		},
		"Invalid type": {
			Tool: updateVariable{VariableID: _testVariableID, Type: "date"},
			Err:  variable.ErrInvalidType,
		},
		"Valid type": {
			Tool: updateVariable{VariableID: _testVariableID, Type: variable.TypeText},
		},
		"Only the variable id": {
			Tool: updateVariable{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_updateVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   updateVariable
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariable": {
			Input: &InputMock{
				FetchVariableFunc: func(xid.ID) (*variable.Entry, error) { return nil, assert.AnError },
			},
			Tool: updateVariable{VariableID: _testVariableID, Name: "Zone"},
			Err:  assert.AnError,
		},
		"Same type": {
			Input:  stubVariableInput(),
			Tool:   updateVariable{VariableID: _testVariableID, Type: variable.TypeList},
			Result: "Update variable $region",
		},
		"Type change": {
			Input:  stubVariableInput(),
			Tool:   updateVariable{VariableID: _testVariableID, Type: variable.TypeText},
			Result: "Update variable $region and make it text, resetting everyone's value",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameUpdateVariable, Summary: c.Result}, got)
		})
	}
}

func Test_updateVariable_Exec(t *testing.T) {
	t.Parallel()

	tool := updateVariable{VariableID: _testVariableID, Name: "Zone"}

	// stubBuiltinInput holds the built-in variable, whose default fails
	// to set with defaultErr.
	stubBuiltinInput := func(defaultErr error) *InputMock {
		inp := stubVariableInput()
		inp.FetchVariableFunc = func(xid.ID) (*variable.Entry, error) {
			return &stubVariableTree()[0].Variables[0], nil
		}
		inp.SetVariableDefaultFunc = func(xid.ID, string) error { return defaultErr }

		return inp
	}

	cc := map[string]struct {
		Input    *InputMock
		Tool     updateVariable
		Updates  int
		Defaults int
		Result   any
		Err      error
	}{
		"Error returned by Input.FetchVariable": {
			Input: &InputMock{
				FetchVariableFunc: func(xid.ID) (*variable.Entry, error) { return nil, assert.AnError },
			},
			Tool: tool,
			Err:  assert.AnError,
		},
		"Built-in variable without a default": {
			Input: stubBuiltinInput(nil),
			Tool:  updateVariable{VariableID: _builtinVariableID, Name: "Range"},
			Err:   variable.ErrBuiltinVariable,
		},
		"Built-in variable's default with another change": {
			Input: stubBuiltinInput(nil),
			Tool:  updateVariable{VariableID: _builtinVariableID, Name: "Range", Default: null.StringFrom("last_7_days")},
			Err:   variable.ErrBuiltinVariable,
		},
		"Error returned by Input.SetVariableDefault": {
			Input:    stubBuiltinInput(assert.AnError),
			Tool:     updateVariable{VariableID: _builtinVariableID, Default: null.StringFrom("last_7_days")},
			Defaults: 1,
			Err:      assert.AnError,
		},
		// a built-in variable's default is the one thing that changes on
		// it, so it goes straight to the default.
		"Built-in variable's default": {
			Input:    stubBuiltinInput(nil),
			Tool:     updateVariable{VariableID: _builtinVariableID, Default: null.StringFrom("last_7_days")},
			Defaults: 1,
			Result:   variableResult{VariableID: _builtinVariableID},
		},
		"Error returned by Input.UpdateVariable": {
			Input: func() *InputMock {
				inp := stubVariableInput()
				inp.UpdateVariableFunc = func(xid.ID, variable.Input) error { return assert.AnError }

				return inp
			}(),
			Tool:    tool,
			Updates: 1,
			Err:     assert.AnError,
		},
		// the change applies over the current definition, so the options
		// the call left out are kept.
		"Updated": {
			Input:   stubVariableInput(),
			Tool:    tool,
			Updates: 1,
			Result:  variableResult{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.UpdateVariableCalls()
			require.Len(t, ff, c.Updates)

			dd := c.Input.SetVariableDefaultCalls()
			require.Len(t, dd, c.Defaults)

			for _, d := range dd {
				assert.Equal(t, _builtinVariableID, d.VariableID)
				assert.Equal(t, "last_7_days", d.Value)
			}

			for _, f := range ff {
				assert.Equal(t, _testVariableID, f.VariableID)
				assert.Equal(t, tool.input(stubVariableTree()[1].Variables[0]), f.Inp)
			}

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_updateVariable_input(t *testing.T) {
	t.Parallel()

	list := stubVariableTree()[1].Variables[0]
	builtin := stubVariableTree()[0].Variables[0]
	otherGroup := xid.New()

	cc := map[string]struct {
		Tool    updateVariable
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
			Tool:    updateVariable{GroupID: otherGroup, Key: "zone", Name: "Zone", Description: null.StringFrom(""), Options: []string{"us-east-1", "ap-south-1"}},
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
		"Repeated values are passed on for validation to refuse": {
			Tool:    updateVariable{Options: []string{"us-east-1", "us-east-1"}},
			Current: list,
			Result: variable.Input{
				GroupID:     _testVariableGroupID,
				Key:         "region",
				Name:        "Region",
				Description: null.StringFrom("Cloud region"),
				Type:        variable.TypeList,
				Options:     []variable.OptionInput{{ID: null.ValueFrom(_testOtherOptionID), Value: "us-east-1"}, {ID: null.ValueFrom(_testOtherOptionID), Value: "us-east-1"}},
			},
		},
		"List turned into text drops its options": {
			Tool:    updateVariable{Type: variable.TypeText, Default: null.StringFrom("eu")},
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
			Tool:    updateVariable{Description: null.StringFrom("Window")},
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
			Tool:    updateVariable{Type: variable.TypeText},
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

			assert.Equal(t, c.Result, c.Tool.input(c.Current))
		})
	}
}

func Test_deleteVariable_Info(t *testing.T) {
	t.Parallel()

	info := deleteVariable{}.Info()

	assert.Equal(t, Traits{Write: true, Destructive: true, Group: GroupVariables, Role: org.RoleEditor}, info.Traits)
	assert.Equal(t, NameDeleteVariable, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"variable_id"}, info.Required)
}

func Test_deleteVariable_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool deleteVariable
		Err  error
	}{
		"Missing variable id": {
			Tool: deleteVariable{},
			Err:  errors.New("variable_id is required"),
		},
		"Complete arguments": {
			Tool: deleteVariable{VariableID: _testVariableID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_deleteVariable_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariable": {
			Input: &InputMock{
				FetchVariableFunc: func(xid.ID) (*variable.Entry, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
		"Known variable": {
			Input:  stubVariableInput(),
			Result: "Delete variable $region",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteVariable{VariableID: _testVariableID}.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameDeleteVariable, Summary: c.Result}, got)
		})
	}
}

func Test_deleteVariable_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Result any
		Err    error
	}{
		"Error returned by Input.DeleteVariable": {
			Input: &InputMock{DeleteVariableFunc: func(xid.ID) error { return assert.AnError }},
			Err:   assert.AnError,
		},
		"Deleted": {
			Input:  &InputMock{},
			Result: deleteVariableResult{VariableID: _testVariableID, Deleted: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := deleteVariable{VariableID: _testVariableID}.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Input.DeleteVariableCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableID, ff[0].VariableID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_setVariableValue_Info(t *testing.T) {
	t.Parallel()

	info := setVariableValue{}.Info()

	assert.Equal(t, Traits{Write: true, Idempotent: true, Group: GroupVariables, Role: org.RoleViewer}, info.Traits)
	assert.Equal(t, NameSetVariableValue, info.Name)
	assert.NotEmpty(t, info.Description)
	assert.Equal(t, []string{"variable_id"}, info.Required)
}

func Test_setVariableValue_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool setVariableValue
		Err  error
	}{
		"Missing variable id": {
			Tool: setVariableValue{Value: null.StringFrom("x")},
			Err:  errors.New("variable_id is required"),
		},
		"Missing value": {
			Tool: setVariableValue{VariableID: _testVariableID},
			Err:  errors.New("value is required"),
		},
		"Reset without a value": {
			Tool: setVariableValue{VariableID: _testVariableID, Default: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_setVariableValue_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   setVariableValue
		Result string
		Err    error
	}{
		"Error returned by Input.FetchVariable": {
			Input: &InputMock{
				FetchVariableFunc: func(xid.ID) (*variable.Entry, error) { return nil, assert.AnError },
			},
			Tool: setVariableValue{VariableID: _testVariableID, Value: null.StringFrom("x")},
			Err:  assert.AnError,
		},
		// a list option is named by its value, not its id.
		"Own value of a list": {
			Input:  stubVariableInput(),
			Tool:   setVariableValue{VariableID: _testVariableID, Value: null.StringFrom(_testOtherOptionID.String())},
			Result: `Set your $region to "us-east-1"`,
		},
		"Reset": {
			Input:  stubVariableInput(),
			Tool:   setVariableValue{VariableID: _testVariableID, Default: true},
			Result: "Reset your $region to the default",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchVariableCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{Tool: NameSetVariableValue, Summary: c.Result}, got)
		})
	}
}

func Test_setVariableValue_Exec(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input  *InputMock
		Tool   setVariableValue
		Values int
		Resets int
		Result any
		Err    error
	}{
		"Error returned by Input.SetVariableValue": {
			Input:  &InputMock{SetVariableValueFunc: func(xid.ID, string) error { return assert.AnError }},
			Tool:   setVariableValue{VariableID: _testVariableID, Value: null.StringFrom("x")},
			Values: 1,
			Err:    assert.AnError,
		},
		"Own value set": {
			Input:  &InputMock{},
			Tool:   setVariableValue{VariableID: _testVariableID, Value: null.StringFrom("x")},
			Values: 1,
			Result: setVariableValueResult{VariableID: _testVariableID, Value: "x"},
		},
		"Error returned by Input.ResetVariableValue": {
			Input:  &InputMock{ResetVariableValueFunc: func(xid.ID) error { return assert.AnError }},
			Tool:   setVariableValue{VariableID: _testVariableID, Default: true},
			Resets: 1,
			Err:    assert.AnError,
		},
		// a reset needs no value: the user sees the default again.
		"Reset to the default": {
			Input:  &InputMock{},
			Tool:   setVariableValue{VariableID: _testVariableID, Default: true},
			Resets: 1,
			Result: setVariableValueResult{VariableID: _testVariableID, Default: true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Tool.Exec(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.SetVariableValueCalls(), c.Values)
			assert.Len(t, c.Input.ResetVariableValueCalls(), c.Resets)

			// setting a default is update_variable's.
			assert.Empty(t, c.Input.SetVariableDefaultCalls())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
