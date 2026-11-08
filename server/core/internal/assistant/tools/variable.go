package tools

import (
	"cmp"
	"errors"
	"fmt"

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

// Validate accepts every payload: nothing is required.
func (listVariables) Validate() error {
	return nil
}

// Exec lists every group with its variables.
func (listVariables) Exec(inp Input) (any, error) {
	tree, err := inp.FetchVariableTree()
	if err != nil {
		return nil, fmt.Errorf("fetching variables: %w", err)
	}

	out := listVariablesResult{Groups: make([]variableGroupEntry, 0, len(tree))}

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

	return out, nil
}

// listVariablesResult is what list_variables returns.
type listVariablesResult struct {
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

// createVariableGroup creates a variable group in the organisation.
type createVariableGroup struct {
	// Name is the new group's display name. Required.
	Name string `json:"name"`

	// ColorName is the new group's colour as a palette name. Required.
	ColorName string `json:"color"`
}

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

// Validate checks the arguments are complete and the colour in the palette.
func (cvg createVariableGroup) Validate() error {
	if cvg.Name == "" {
		return errRequired("name")
	}

	if cvg.ColorName == "" {
		return errRequired("color")
	}

	return cvg.input().Validate()
}

// Summary describes the group the model wants to create.
func (cvg createVariableGroup) Summary(_ DescribeInput) (ActionSummary, error) {
	return ActionSummary{
		Tool:    NameCreateVariableGroup,
		Summary: fmt.Sprintf("Create variable group %s (%s)", cvg.Name, cvg.ColorName),
	}, nil
}

// Exec creates the group.
func (cvg createVariableGroup) Exec(inp Input) (any, error) {
	g, err := inp.CreateVariableGroup(cvg.input())
	if err != nil {
		return nil, err
	}

	return variableGroupResult{GroupID: g.ID}, nil
}

// input returns the arguments as the domain's create input.
func (cvg createVariableGroup) input() variable.GroupCreateInput {
	return variable.GroupCreateInput{GroupName: cvg.Name, Color: cvg.ColorName}
}

// updateVariableGroup renames or recolours a variable group.
type updateVariableGroup struct {
	// GroupID names the group being changed.
	GroupID xid.ID `json:"group_id"`

	// Name is the new display name; empty leaves it unchanged.
	Name string `json:"name"`

	// ColorName is the new colour as a palette name; empty leaves it
	// unchanged.
	ColorName string `json:"color"`
}

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

// Validate checks the arguments name a group and change something.
func (uvg updateVariableGroup) Validate() error {
	if uvg.GroupID.IsNil() {
		return errRequired("group_id")
	}

	if uvg.Name == "" && uvg.ColorName == "" {
		return errors.New("give at least one of name or color")
	}

	return uvg.input().Validate()
}

// Summary lists the changes the model asked for.
func (uvg updateVariableGroup) Summary(inp DescribeInput) (ActionSummary, error) {
	g, err := inp.FetchVariableGroup(uvg.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable group: %w", err)
	}

	var changes []string

	if uvg.Name != "" {
		changes = append(changes, fmt.Sprintf("rename variable group %s to %q", g.GroupName, uvg.Name))
	}

	if uvg.ColorName != "" {
		changes = append(changes, fmt.Sprintf("set the colour of variable group %s to %s", g.GroupName, uvg.ColorName))
	}

	return ActionSummary{
		Tool:    NameUpdateVariableGroup,
		Summary: joinClauses(changes),
	}, nil
}

// Exec applies the change.
func (uvg updateVariableGroup) Exec(inp Input) (any, error) {
	if err := inp.UpdateVariableGroup(uvg.GroupID, uvg.input()); err != nil {
		return nil, err
	}

	return variableGroupResult{GroupID: uvg.GroupID}, nil
}

// input returns the arguments as the domain's update input, with only
// the fields that were given set.
func (uvg updateVariableGroup) input() variable.GroupUpdateInput {
	var inp variable.GroupUpdateInput

	if uvg.Name != "" {
		inp.GroupName = null.StringFrom(uvg.Name)
	}

	if uvg.ColorName != "" {
		inp.Color = null.StringFrom(uvg.ColorName)
	}

	return inp
}

// variableGroupResult is what create_variable_group and
// update_variable_group return.
type variableGroupResult struct {
	// GroupID addresses the group in every later call.
	GroupID xid.ID `json:"group_id"`
}

// deleteVariableGroup removes an empty variable group.
type deleteVariableGroup struct {
	// GroupID names the group being deleted.
	GroupID xid.ID `json:"group_id"`
}

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

// Validate checks the arguments are complete.
func (dvg deleteVariableGroup) Validate() error {
	if dvg.GroupID.IsNil() {
		return errRequired("group_id")
	}

	return nil
}

// Summary names the group the model wants to delete.
func (dvg deleteVariableGroup) Summary(inp DescribeInput) (ActionSummary, error) {
	g, err := inp.FetchVariableGroup(dvg.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable group: %w", err)
	}

	return ActionSummary{
		Tool:    NameDeleteVariableGroup,
		Summary: "Delete variable group " + g.GroupName,
	}, nil
}

// Exec deletes the group.
func (dvg deleteVariableGroup) Exec(inp Input) (any, error) {
	if err := inp.DeleteVariableGroup(dvg.GroupID); err != nil {
		return nil, err
	}

	return deleteVariableGroupResult{GroupID: dvg.GroupID, Deleted: true}, nil
}

// deleteVariableGroupResult is what delete_variable_group returns.
type deleteVariableGroupResult struct {
	// GroupID is the group that was removed.
	GroupID xid.ID `json:"group_id"`

	// Deleted confirms the removal happened.
	Deleted bool `json:"deleted"`
}

// createVariable creates a variable in a group.
type createVariable struct {
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

// Validate checks the arguments are complete and form a valid variable.
func (cv createVariable) Validate() error {
	if cv.GroupID.IsNil() {
		return errRequired("group_id")
	}

	if cv.Key == "" {
		return errRequired("key")
	}

	if cv.Name == "" {
		return errRequired("name")
	}

	if cv.Type == "" {
		return errRequired("type")
	}

	return cv.input().Validate()
}

// Summary names the variable and the group it goes in.
func (cv createVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	g, err := inp.FetchVariableGroup(cv.GroupID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable group: %w", err)
	}

	return ActionSummary{
		Tool:    NameCreateVariable,
		Summary: fmt.Sprintf("Create %s variable $%s in %s", cv.Type, cv.Key, g.GroupName),
	}, nil
}

// Exec creates the variable.
func (cv createVariable) Exec(inp Input) (any, error) {
	v, err := inp.CreateVariable(cv.input())
	if err != nil {
		return nil, err
	}

	return variableResult{VariableID: v.ID}, nil
}

// input returns the arguments as the domain's variable input.
func (cv createVariable) input() variable.Input {
	inp := variable.Input{
		GroupID:      cv.GroupID,
		Key:          cv.Key,
		Name:         cv.Name,
		Type:         cv.Type,
		DefaultValue: cv.Default,
	}

	if cv.Description != "" {
		inp.Description = null.StringFrom(cv.Description)
	}

	for _, o := range cv.Options {
		inp.Options = append(inp.Options, variable.OptionInput{Value: o})
	}

	return inp
}

// updateVariable changes a variable's definition.
type updateVariable struct {
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

// Validate checks the arguments name a variable.
func (uv updateVariable) Validate() error {
	if uv.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	if uv.Type != "" {
		return uv.Type.Validate()
	}

	return nil
}

// Summary names the variable being changed.
func (uv updateVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	v, err := inp.FetchVariable(uv.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable: %w", err)
	}

	summary := "Update variable $" + v.Key

	if uv.Type != "" && uv.Type != v.Type {
		summary += fmt.Sprintf(" and make it %s, resetting everyone's value", uv.Type)
	}

	return ActionSummary{
		Tool:    NameUpdateVariable,
		Summary: summary,
	}, nil
}

// Exec applies the change over the current definition.
func (uv updateVariable) Exec(inp Input) (any, error) {
	current, err := inp.FetchVariable(uv.VariableID)
	if err != nil {
		return nil, err
	}

	if err = inp.UpdateVariable(uv.VariableID, uv.input(*current)); err != nil {
		return nil, err
	}

	return variableResult{VariableID: uv.VariableID}, nil
}

// input returns the variable's definition with the given fields laid
// over the current one. A type change drops what only the old type had:
// the options of a list and the default of anything else.
func (uv updateVariable) input(current variable.Entry) variable.Input {
	inp := variable.Input{
		GroupID:      cmp.Or(uv.GroupID, current.GroupID),
		Key:          cmp.Or(uv.Key, current.Key),
		Name:         cmp.Or(uv.Name, current.Name),
		Description:  current.Description,
		Type:         cmp.Or(uv.Type, current.Type),
		DefaultValue: current.DefaultValue,
	}

	if uv.Description.Valid {
		inp.Description = null.NewString(uv.Description.String, uv.Description.String != "")
	}

	if inp.Type != current.Type || inp.Type == variable.TypeList {
		inp.DefaultValue = null.String{}
	}

	if uv.Default.Valid {
		inp.DefaultValue = uv.Default
	}

	if inp.Type == variable.TypeList && current.Type == variable.TypeList {
		for _, o := range current.Options {
			inp.Options = append(inp.Options, variable.OptionInput{ID: null.ValueFrom(o.ID), Value: o.Value})
		}
	}

	if uv.Options != nil {
		inp.Options = make([]variable.OptionInput, 0, len(uv.Options))

		for _, value := range uv.Options {
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

// variableResult is what create_variable and update_variable return.
type variableResult struct {
	// VariableID addresses the variable in every later call.
	VariableID xid.ID `json:"variable_id"`
}

// deleteVariable removes a variable.
type deleteVariable struct {
	// VariableID names the variable being deleted.
	VariableID xid.ID `json:"variable_id"`
}

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

// Validate checks the arguments are complete.
func (dv deleteVariable) Validate() error {
	if dv.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	return nil
}

// Summary names the variable the model wants to delete.
func (dv deleteVariable) Summary(inp DescribeInput) (ActionSummary, error) {
	v, err := inp.FetchVariable(dv.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable: %w", err)
	}

	return ActionSummary{
		Tool:    NameDeleteVariable,
		Summary: "Delete variable $" + v.Key,
	}, nil
}

// Exec deletes the variable.
func (dv deleteVariable) Exec(inp Input) (any, error) {
	if err := inp.DeleteVariable(dv.VariableID); err != nil {
		return nil, err
	}

	return deleteVariableResult{VariableID: dv.VariableID, Deleted: true}, nil
}

// deleteVariableResult is what delete_variable returns.
type deleteVariableResult struct {
	// VariableID is the variable that was removed.
	VariableID xid.ID `json:"variable_id"`

	// Deleted confirms the removal happened.
	Deleted bool `json:"deleted"`
}

// setVariableValue sets the current user's own value of a variable, or
// its organisation-wide default.
type setVariableValue struct {
	// VariableID names the variable.
	VariableID xid.ID `json:"variable_id"`

	// Value is the new value. Required, though text may be empty.
	Value null.String `json:"value"`

	// Default sets the organisation's default instead of the user's own
	// value.
	Default bool `json:"default"`
}

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

// Validate checks the arguments name a variable and give a value.
func (svv setVariableValue) Validate() error {
	if svv.VariableID.IsNil() {
		return errRequired("variable_id")
	}

	if !svv.Value.Valid {
		return errRequired("value")
	}

	return nil
}

// Summary names the variable and the value it takes.
func (svv setVariableValue) Summary(inp DescribeInput) (ActionSummary, error) {
	v, err := inp.FetchVariable(svv.VariableID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching variable: %w", err)
	}

	value := svv.Value.String

	// a list's value is an option id, which means nothing on a card.
	for _, o := range v.Options {
		if o.ID.String() == value {
			value = o.Value
		}
	}

	summary := fmt.Sprintf("Set your $%s to %q", v.Key, value)
	if svv.Default {
		summary = fmt.Sprintf("Set the default of $%s to %q", v.Key, value)
	}

	return ActionSummary{
		Tool:    NameSetVariableValue,
		Summary: summary,
	}, nil
}

// Exec sets the value or the default.
func (svv setVariableValue) Exec(inp Input) (any, error) {
	set := inp.SetVariableValue
	if svv.Default {
		set = inp.SetVariableDefault
	}

	if err := set(svv.VariableID, svv.Value.String); err != nil {
		return nil, err
	}

	return setVariableValueResult{VariableID: svv.VariableID, Value: svv.Value.String, Default: svv.Default}, nil
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

// VariableInput is what the variable tools reach on top of
// DescribeInput.
type VariableInput interface {
	// FetchVariableTree should return the organisation's variables by
	// group.
	FetchVariableTree() (variable.Tree, error)

	// CreateVariableGroup should store a new variable group.
	CreateVariableGroup(inp variable.GroupCreateInput) (*variable.Group, error)

	// UpdateVariableGroup should apply the update to the group.
	UpdateVariableGroup(groupID xid.ID, inp variable.GroupUpdateInput) error

	// DeleteVariableGroup should delete the group.
	DeleteVariableGroup(groupID xid.ID) error

	// CreateVariable should store a new variable.
	CreateVariable(inp variable.Input) (*variable.Variable, error)

	// UpdateVariable should apply the update to the variable.
	UpdateVariable(variableID xid.ID, inp variable.Input) error

	// DeleteVariable should delete the variable.
	DeleteVariable(variableID xid.ID) error

	// SetVariableValue should set the user's own value of the variable.
	SetVariableValue(variableID xid.ID, value string) error

	// SetVariableDefault should set the organisation's default value of
	// the variable.
	SetVariableDefault(variableID xid.ID, value string) error
}
