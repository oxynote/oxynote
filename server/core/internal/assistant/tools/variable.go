package tools

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/rs/xid"
)

// _variableTypes is every type a variable takes, as the tools name them.
var _variableTypes = []string{
	string(variable.TypeList),
	string(variable.TypeNumber),
	string(variable.TypeText),
	string(variable.TypeTimeRange),
}

// listVariablesArgs is what list_variables is called with.
type listVariablesArgs struct{}

// Validate accepts every payload: nothing is required.
func (listVariablesArgs) Validate() error {
	return nil
}

// listVariables returns the organisation's variable groups with their
// variables and the user's own values.
type listVariables struct{}

// Info returns the tool's model-facing description.
func (listVariables) Info() Info {
	return Info{
		Name:        NameListVariables,
		Traits:      Traits{Group: GroupVariables},
		Description: "List the global variables by group as [{id, name, color, builtin, variables: [{id, key, name, description, type, builtin, default, value, options}]}]. Queries and metrics reference a variable as $key, so one change updates every metric using it; each user picks their own. default is what anyone without their own sees; for a list, its first option's id. value is the user's own, or null. A built-in's definition cannot change; set_variable_value sets its value or default.",
		Properties:  map[string]any{},
	}
}

// Execute lists every group with its variables.
func (listVariables) Execute(inp *input) (string, error) {
	var in listVariablesArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tree, err := inp.FetchVariableTree()
	if err != nil {
		return "", fmt.Errorf("fetch variables: %w", err)
	}

	out := variableListResult{Groups: make([]variableGroupEntry, 0, len(tree))}

	for _, g := range tree {
		ge := variableGroupEntry{
			ID:        g.ID,
			Name:      g.GroupName,
			ColorName: g.Color,
			Builtin:   g.Builtin,
			Variables: make([]variableEntry, 0, len(g.Variables)),
		}

		for _, v := range g.Variables {
			ve := variableEntry{
				ID:          v.ID,
				Key:         v.Key,
				Name:        v.Name,
				Description: v.Description,
				Type:        v.Type,
				Builtin:     v.Builtin,
				Default:     v.DefaultValue,
				Value:       v.Value,
				Options:     make([]variableOptionEntry, 0, len(v.Options)),
			}

			for _, o := range v.Options {
				ve.Options = append(ve.Options, variableOptionEntry{ID: o.ID, Value: o.Value})
			}

			ge.Variables = append(ge.Variables, ve)
		}

		out.Groups = append(out.Groups, ge)
	}

	return result(out)
}

// variableListResult is what list_variables returns.
type variableListResult struct {
	// Groups is every variable group in the organisation, in sidebar
	// order.
	Groups []variableGroupEntry `json:"groups"`
}

// variableGroupEntry describes one variable group and its variables.
type variableGroupEntry struct {
	// ID is the group id, which is what a tool's group_id takes.
	ID xid.ID `json:"id"`

	// Name is the group's display name.
	Name string `json:"name"`

	// ColorName is the palette name of the group's colour.
	ColorName string `json:"color"`

	// Builtin indicates the group ships with every organisation and
	// cannot change.
	Builtin bool `json:"builtin"`

	// Variables is every variable of the group, in order.
	Variables []variableEntry `json:"variables"`
}

// variableEntry describes one variable.
type variableEntry struct {
	// ID is the variable id, which is what a tool's variable_id takes.
	ID xid.ID `json:"id"`

	// Key is what queries and text reference the variable by, without
	// the $.
	Key string `json:"key"`

	// Name is the variable's display name.
	Name string `json:"name"`

	// Description is the optional explanation of the variable.
	Description null.String `json:"description"`

	// Type is the kind of value the variable holds.
	Type variable.Type `json:"type"`

	// Builtin indicates only the default of the variable can change.
	Builtin bool `json:"builtin"`

	// Default is the value everyone without their own value sees.
	Default null.String `json:"default"`

	// Value is the current user's own value, or null.
	Value null.String `json:"value"`

	// Options is a list variable's options in order.
	Options []variableOptionEntry `json:"options"`
}

// variableOptionEntry describes one option of a list variable.
type variableOptionEntry struct {
	// ID is the option id, which is what a list's value takes.
	ID xid.ID `json:"id"`

	// Value is the text the option stands for.
	Value string `json:"value"`
}

// createVariableGroupArgs is what create_variable_group is called with.
type createVariableGroupArgs struct {
	// Name is the new group's display name. Required.
	Name string `json:"name"`

	// ColorName is the new group's colour as a palette name. Required.
	ColorName string `json:"color"`
}

// Validate checks the arguments are complete and the colour in the palette.
func (a createVariableGroupArgs) Validate() error {
	if a.Name == "" {
		return errRequired("name")
	}

	if a.ColorName == "" {
		return errRequired("color")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's create input.
func (a createVariableGroupArgs) input() variable.GroupCreateInput {
	return variable.GroupCreateInput{GroupName: a.Name, Color: a.ColorName}
}

// createVariableGroup creates a variable group in the organisation.
type createVariableGroup struct{}

// Info returns the tool's model-facing description.
func (createVariableGroup) Info() Info {
	return Info{
		Name:        NameCreateVariableGroup,
		Traits:      Traits{Group: GroupVariables, Write: true},
		Description: "Create a variable group, last in order, and return {group_id}. The name has to be unused.",
		Properties: map[string]any{
			"name":  map[string]any{"type": "string", "description": "The display name."},
			"color": map[string]any{"type": "string", "enum": tag.ColorNames(), "description": "The colour."},
		},
		Required: []string{"name", "color"},
	}
}

// Summary describes the group the model wants to create.
func (createVariableGroup) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:    NameCreateVariableGroup,
		Summary: fmt.Sprintf("Create variable group %s (%s)", in.Name, in.ColorName),
	}, nil
}

// Execute creates the group.
func (createVariableGroup) Execute(inp *input) (string, error) {
	var in createVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	g, err := inp.CreateVariableGroup(in.input())
	if err != nil {
		return "", err
	}

	return result(variableGroupResult{GroupID: g.ID})
}

// variableGroupResult is what create_variable_group and
// update_variable_group return.
type variableGroupResult struct {
	// GroupID addresses the group in every later call.
	GroupID xid.ID `json:"group_id"`
}

// updateVariableGroupArgs is what update_variable_group is called with.
type updateVariableGroupArgs struct {
	// GroupID names the group being changed.
	GroupID xid.ID `json:"group_id"`

	// Name is the new display name; empty leaves it unchanged.
	Name string `json:"name"`

	// ColorName is the new colour as a palette name; empty leaves it
	// unchanged.
	ColorName string `json:"color"`
}

// Validate checks the arguments name a group and change something.
func (a updateVariableGroupArgs) Validate() error {
	if a.GroupID.IsNil() {
		return errRequired("group_id")
	}

	if a.Name == "" && a.ColorName == "" {
		return errors.New("give at least one of name or color")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's update input, with only
// the fields that were given set.
func (a updateVariableGroupArgs) input() variable.GroupUpdateInput {
	var inp variable.GroupUpdateInput

	if a.Name != "" {
		inp.GroupName = null.StringFrom(a.Name)
	}

	if a.ColorName != "" {
		inp.Color = null.StringFrom(a.ColorName)
	}

	return inp
}

// updateVariableGroup renames or recolours a variable group.
type updateVariableGroup struct{}

// Info returns the tool's model-facing description.
func (updateVariableGroup) Info() Info {
	return Info{
		Name:        NameUpdateVariableGroup,
		Traits:      Traits{Group: GroupVariables, Write: true, Idempotent: true},
		Description: "Rename or recolour a variable group other than the built-in one; give only what changes. Returns {group_id}.",
		Properties: map[string]any{
			"group_id": map[string]any{"type": "string", "description": _variableGroupIDDescription},
			"name":     map[string]any{"type": "string", "description": "Optional. The new display name."},
			"color":    map[string]any{"type": "string", "description": "Optional. The new colour, a create_variable_group colour."},
		},
		Required: []string{"group_id"},
	}
}

// Summary lists the changes the model asked for.
func (updateVariableGroup) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	g, err := inp.FetchVariableGroup(in.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable group: %w", err)
	}

	var changes []string

	if in.Name != "" {
		changes = append(changes, fmt.Sprintf("rename variable group %s to %q", g.GroupName, in.Name))
	}

	if in.ColorName != "" {
		changes = append(changes, fmt.Sprintf("set the colour of variable group %s to %s", g.GroupName, in.ColorName))
	}

	return ActionSummary{
		Tool:    NameUpdateVariableGroup,
		Summary: sentence(changes),
	}, nil
}

// Execute applies the change.
func (updateVariableGroup) Execute(inp *input) (string, error) {
	var in updateVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.UpdateVariableGroup(in.GroupID, in.input()); err != nil {
		return "", err
	}

	return result(variableGroupResult{GroupID: in.GroupID})
}

// deleteVariableGroupArgs is what delete_variable_group is called with.
type deleteVariableGroupArgs struct {
	// GroupID names the group being deleted.
	GroupID xid.ID `json:"group_id"`
}

// Validate checks the arguments are complete.
func (a deleteVariableGroupArgs) Validate() error {
	if a.GroupID.IsNil() {
		return errRequired("group_id")
	}

	return nil
}

// deleteVariableGroup removes an empty variable group.
type deleteVariableGroup struct{}

// Info returns the tool's model-facing description.
func (deleteVariableGroup) Info() Info {
	return Info{
		Name:        NameDeleteVariableGroup,
		Traits:      Traits{Group: GroupVariables, Write: true, Destructive: true},
		Description: "Delete an empty variable group other than the built-in one. Returns {group_id, deleted}.",
		Properties: map[string]any{
			"group_id": map[string]any{"type": "string", "description": _variableGroupIDDescription},
		},
		Required: []string{"group_id"},
	}
}

// Summary names the group the model wants to delete.
func (deleteVariableGroup) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	g, err := inp.FetchVariableGroup(in.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable group: %w", err)
	}

	return ActionSummary{
		Tool:    NameDeleteVariableGroup,
		Summary: "Delete variable group " + g.GroupName,
	}, nil
}

// Execute deletes the group.
func (deleteVariableGroup) Execute(inp *input) (string, error) {
	var in deleteVariableGroupArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.DeleteVariableGroup(in.GroupID); err != nil {
		return "", err
	}

	return result(deletedVariableGroupResult{GroupID: in.GroupID, Deleted: true})
}

// deletedVariableGroupResult is what delete_variable_group returns.
type deletedVariableGroupResult struct {
	// GroupID is the group that was removed.
	GroupID xid.ID `json:"group_id"`

	// Deleted confirms the removal happened.
	Deleted bool `json:"deleted"`
}

// createVariableArgs is what create_variable is called with.
type createVariableArgs struct {
	// GroupID names the group the variable goes in. Required.
	GroupID xid.ID `json:"group_id"`

	// Key is what queries reference the variable by. Required.
	Key string `json:"key"`

	// Name is the display name. Required.
	Name string `json:"name"`

	// Description is an optional explanation.
	Description string `json:"description"`

	// Type is the kind of value the variable holds. Required.
	Type variable.Type `json:"type"`

	// Default is the value everyone sees until they set their own; not
	// for a list.
	Default null.String `json:"default"`

	// Options are a list's option values in order.
	Options []string `json:"options"`
}

// Validate checks the arguments are complete and form a valid variable.
func (a createVariableArgs) Validate() error {
	if a.GroupID.IsNil() {
		return errRequired("group_id")
	}

	if a.Key == "" {
		return errRequired("key")
	}

	if a.Name == "" {
		return errRequired("name")
	}

	if a.Type == "" {
		return errRequired("type")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's variable input.
func (a createVariableArgs) input() variable.Input {
	inp := variable.Input{
		GroupID:      a.GroupID,
		Key:          a.Key,
		Name:         a.Name,
		Type:         a.Type,
		DefaultValue: a.Default,
	}

	if a.Description != "" {
		inp.Description = null.StringFrom(a.Description)
	}

	for _, o := range a.Options {
		inp.Options = append(inp.Options, variable.OptionInput{Value: o})
	}

	return inp
}

// createVariable creates a variable in a group.
type createVariable struct{}

// Info returns the tool's model-facing description.
func (createVariable) Info() Info {
	return Info{
		Name:        NameCreateVariable,
		Traits:      Traits{Group: GroupVariables, Write: true},
		Description: "Create a global variable, last in its group, and return {variable_id}. The key has to be unused and match ^[a-z][a-z0-9_]*$. A list takes options, the first being the default; other types take a default instead, required for a time range.",
		Properties: map[string]any{
			"group_id":    map[string]any{"type": "string", "description": _variableGroupIDDescription},
			"key":         map[string]any{"type": "string", "description": "The key, without the $."},
			"name":        map[string]any{"type": "string", "description": "The display name."},
			"description": map[string]any{"type": "string", "description": "Optional. What the variable is for."},
			"type":        map[string]any{"type": "string", "enum": _variableTypes, "description": "The kind of value."},
			"default":     map[string]any{"type": "string", "description": _variableDefaultDescription},
			"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "A list's option values in order."},
		},
		Required: []string{"group_id", "key", "name", "type"},
	}
}

// Summary names the variable and the group it goes in.
func (createVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createVariableArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	g, err := inp.FetchVariableGroup(in.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable group: %w", err)
	}

	return ActionSummary{
		Tool:    NameCreateVariable,
		Summary: fmt.Sprintf("Create %s variable $%s in %s", in.Type, in.Key, g.GroupName),
	}, nil
}

// Execute creates the variable.
func (createVariable) Execute(inp *input) (string, error) {
	var in createVariableArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	v, err := inp.CreateVariable(in.input())
	if err != nil {
		return "", err
	}

	return result(variableResult{VariableID: v.ID})
}

// variableResult is what create_variable and update_variable return.
type variableResult struct {
	// VariableID addresses the variable in every later call.
	VariableID xid.ID `json:"variable_id"`
}

// updateVariableArgs is what update_variable is called with. Every
// field but the id is optional and keeps its current value when absent.
type updateVariableArgs struct {
	// VariableID names the variable being changed.
	VariableID xid.ID `json:"variable_id"`

	// GroupID moves the variable to the end of another group.
	GroupID xid.ID `json:"group_id"`

	// Key is the new key.
	Key string `json:"key"`

	// Name is the new display name.
	Name string `json:"name"`

	// Description is the new explanation; an empty string clears it.
	Description null.String `json:"description"`

	// Type is the new type.
	Type variable.Type `json:"type"`

	// Default is the new default; not for a list.
	Default null.String `json:"default"`

	// Options replaces a list's option values, in order. A value the list
	// already has keeps its option, so the users who picked it keep it.
	Options []string `json:"options"`
}

// Validate checks the arguments name a variable.
func (a updateVariableArgs) Validate() error {
	if a.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	if a.Type != "" {
		return a.Type.Validate()
	}

	return nil
}

// input returns the variable's definition with the given fields laid
// over the current one. A type change drops what only the old type had:
// the options of a list and the default of anything else.
func (a updateVariableArgs) input(current variable.Entry) variable.Input {
	inp := variable.Input{
		GroupID:      cmp.Or(a.GroupID, current.GroupID),
		Key:          cmp.Or(a.Key, current.Key),
		Name:         cmp.Or(a.Name, current.Name),
		Description:  current.Description,
		Type:         cmp.Or(a.Type, current.Type),
		DefaultValue: current.DefaultValue,
	}

	if a.Description.Valid {
		inp.Description = null.NewString(a.Description.String, a.Description.String != "")
	}

	if inp.Type != current.Type || inp.Type == variable.TypeList {
		inp.DefaultValue = null.String{}
	}

	if a.Default.Valid {
		inp.DefaultValue = a.Default
	}

	if inp.Type == variable.TypeList && current.Type == variable.TypeList {
		for _, o := range current.Options {
			inp.Options = append(inp.Options, variable.OptionInput{ID: null.ValueFrom(o.ID), Value: o.Value})
		}
	}

	if a.Options != nil {
		inp.Options = make([]variable.OptionInput, 0, len(a.Options))

		for _, value := range a.Options {
			// a repeated value would name the same option twice.
			if slices.ContainsFunc(inp.Options, func(oi variable.OptionInput) bool { return oi.Value == value }) {
				continue
			}

			oi := variable.OptionInput{Value: value}

			for _, o := range current.Options {
				if o.Value == value {
					oi.ID = null.ValueFrom(o.ID)
				}
			}

			inp.Options = append(inp.Options, oi)
		}
	}

	return inp
}

// updateVariable changes a variable's definition.
type updateVariable struct{}

// Info returns the tool's model-facing description.
func (updateVariable) Info() Info {
	return Info{
		Name:        NameUpdateVariable,
		Traits:      Traits{Group: GroupVariables, Write: true, Overwrites: true, Idempotent: true},
		Description: "Change a global variable other than a built-in one, whose default set_variable_value sets; give only what changes. group_id moves it to the end of that group. options replace a list's; users who picked a removed value see the default again. A type change resets everyone's value. Returns {variable_id}.",
		Properties: map[string]any{
			"variable_id": map[string]any{"type": "string", "description": _variableIDDescription},
			"group_id":    map[string]any{"type": "string", "description": "Optional. Moves it to the end of this group."},
			"key":         map[string]any{"type": "string", "description": "Optional. The new key, without the $."},
			"name":        map[string]any{"type": "string", "description": "Optional. The new display name."},
			"description": map[string]any{"type": "string", "description": "Optional. Empty clears it."},
			"type":        map[string]any{"type": "string", "enum": _variableTypes, "description": "Optional. The new kind of value."},
			"default":     map[string]any{"type": "string", "description": "Optional. " + _variableDefaultDescription},
			"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional. A list's option values in order."},
		},
		Required: []string{"variable_id"},
	}
}

// Summary names the variable being changed.
func (updateVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateVariableArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	v, err := inp.FetchVariable(in.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable: %w", err)
	}

	summary := "Update variable $" + v.Key

	if in.Type != "" && in.Type != v.Type {
		summary += fmt.Sprintf(" and make it %s, resetting everyone's value", in.Type)
	}

	return ActionSummary{
		Tool:    NameUpdateVariable,
		Summary: summary,
	}, nil
}

// Execute applies the change over the current definition.
func (updateVariable) Execute(inp *input) (string, error) {
	var in updateVariableArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	current, err := inp.FetchVariable(in.VariableID)
	if err != nil {
		return "", err
	}

	if err = inp.UpdateVariable(in.VariableID, in.input(*current)); err != nil {
		return "", err
	}

	return result(variableResult{VariableID: in.VariableID})
}

// deleteVariableArgs is what delete_variable is called with.
type deleteVariableArgs struct {
	// VariableID names the variable being deleted.
	VariableID xid.ID `json:"variable_id"`
}

// Validate checks the arguments are complete.
func (a deleteVariableArgs) Validate() error {
	if a.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	return nil
}

// deleteVariable removes a variable.
type deleteVariable struct{}

// Info returns the tool's model-facing description.
func (deleteVariable) Info() Info {
	return Info{
		Name:        NameDeleteVariable,
		Traits:      Traits{Group: GroupVariables, Write: true, Destructive: true},
		Description: "Delete a global variable other than a built-in one, with everyone's value. Returns {variable_id, deleted}.",
		Properties: map[string]any{
			"variable_id": map[string]any{"type": "string", "description": _variableIDDescription},
		},
		Required: []string{"variable_id"},
	}
}

// Summary names the variable the model wants to delete.
func (deleteVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteVariableArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	v, err := inp.FetchVariable(in.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable: %w", err)
	}

	return ActionSummary{
		Tool:    NameDeleteVariable,
		Summary: "Delete variable $" + v.Key,
	}, nil
}

// Execute deletes the variable.
func (deleteVariable) Execute(inp *input) (string, error) {
	var in deleteVariableArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.DeleteVariable(in.VariableID); err != nil {
		return "", err
	}

	return result(deletedVariableResult{VariableID: in.VariableID, Deleted: true})
}

// deletedVariableResult is what delete_variable returns.
type deletedVariableResult struct {
	// VariableID is the variable that was removed.
	VariableID xid.ID `json:"variable_id"`

	// Deleted confirms the removal happened.
	Deleted bool `json:"deleted"`
}

// setVariableValueArgs is what set_variable_value is called with.
type setVariableValueArgs struct {
	// VariableID names the variable.
	VariableID xid.ID `json:"variable_id"`

	// Value is the new value. Required, though text may be empty.
	Value null.String `json:"value"`

	// Default sets the organisation's default instead of the user's own
	// value.
	Default bool `json:"default"`
}

// Validate checks the arguments name a variable and give a value.
func (a setVariableValueArgs) Validate() error {
	if a.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	if !a.Value.Valid {
		return errRequired("value")
	}

	return nil
}

// setVariableValue sets the current user's own value of a variable, or
// its organisation-wide default.
type setVariableValue struct{}

// Info returns the tool's model-facing description.
func (setVariableValue) Info() Info {
	return Info{
		Name:        NameSetVariableValue,
		Traits:      Traits{Group: GroupVariables, Write: true, Idempotent: true},
		Description: "Set the current user's own value of a global variable, built-ins included, or with default its organisation-wide default; prefer the user's own. Set metric_range to show every chart another period. A list takes an option id, a number a decimal, a time range a preset such as last_24_hours, text anything. Returns {variable_id, value, default}.",
		Properties: map[string]any{
			"variable_id": map[string]any{"type": "string", "description": _variableIDDescription},
			"value":       map[string]any{"type": "string", "description": "The new value."},
			"default":     map[string]any{"type": "boolean", "description": "Optional. True sets what everyone without their own value sees."},
		},
		Required: []string{"variable_id", "value"},
	}
}

// Summary names the variable and the value it takes.
func (setVariableValue) Summary(inp DescribeInput) (ActionSummary, error) {
	var in setVariableValueArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	v, err := inp.FetchVariable(in.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch variable: %w", err)
	}

	value := in.Value.String

	// a list's value is an option id, which means nothing on a card.
	for _, o := range v.Options {
		if o.ID.String() == value {
			value = o.Value
		}
	}

	summary := fmt.Sprintf("Set your $%s to %q", v.Key, value)
	if in.Default {
		summary = fmt.Sprintf("Set the default of $%s to %q", v.Key, value)
	}

	return ActionSummary{
		Tool:    NameSetVariableValue,
		Summary: summary,
	}, nil
}

// Execute sets the value or the default.
func (setVariableValue) Execute(inp *input) (string, error) {
	var in setVariableValueArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	set := inp.SetVariableValue
	if in.Default {
		set = inp.SetVariableDefault
	}

	if err := set(in.VariableID, in.Value.String); err != nil {
		return "", err
	}

	return result(setVariableValueResult{VariableID: in.VariableID, Value: in.Value.String, Default: in.Default})
}

// setVariableValueResult is what set_variable_value returns.
type setVariableValueResult struct {
	// VariableID is the variable whose value was set.
	VariableID xid.ID `json:"variable_id"`

	// Value is the new value.
	Value string `json:"value"`

	// Default indicates the organisation's default was set.
	Default bool `json:"default"`
}
