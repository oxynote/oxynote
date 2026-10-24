// Package variable provides the domain types for the global variables an
// organization filters its live page data by.
package variable

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sliceutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
)

// _maxValueLength is the most characters a value or an option holds, so
// a query that references one many times stays small.
const _maxValueLength = 128

// All available Type constants.
const (
	// TypeList is a variable whose value is one of its options.
	TypeList Type = "list"

	// TypeNumber is a variable whose value is a decimal number.
	TypeNumber Type = "number"

	// TypeText is a variable whose value is free text.
	TypeText Type = "text"

	// TypeTimeRange is a variable whose value is a time range preset.
	TypeTimeRange Type = "time-range"
)

var (
	// ErrInvalidGroupName is returned when a group is given an empty name.
	ErrInvalidGroupName = errutil.New(http.StatusBadRequest, "variable.invalid_group_name", "group name cannot be empty")

	// ErrInvalidGroupColor is returned when a group is given a colour
	// outside the palette.
	ErrInvalidGroupColor = errutil.New(http.StatusBadRequest, "variable.invalid_group_color", "group colour must be one of the palette")

	// ErrEmptyGroupUpdate is returned when a group update sets neither a
	// name nor a colour.
	ErrEmptyGroupUpdate = errutil.New(http.StatusBadRequest, "variable.empty_group_update", "group update needs a name or a colour")

	// ErrDuplicateGroupName is returned when a group takes a name the
	// organization already uses.
	ErrDuplicateGroupName = errutil.New(http.StatusConflict, "variable.duplicate_group_name", "group name is already in use")

	// ErrGroupNotEmpty is returned when a group that still holds variables
	// is deleted.
	ErrGroupNotEmpty = errutil.New(http.StatusConflict, "variable.group_not_empty", "group still has variables")

	// ErrBuiltinGroup is returned when the built-in group is changed or
	// given variables.
	ErrBuiltinGroup = errutil.New(http.StatusForbidden, "variable.builtin_group", "built-in group cannot be changed")

	// ErrInvalidKey is returned when a variable key is not lowercase
	// letters, digits and underscores starting with a letter.
	ErrInvalidKey = errutil.New(http.StatusBadRequest, "variable.invalid_key", "key must start with a letter and hold only lowercase letters, digits and underscores")

	// ErrDuplicateKey is returned when a variable takes a key the
	// organization already uses.
	ErrDuplicateKey = errutil.New(http.StatusConflict, "variable.duplicate_key", "key is already in use")

	// ErrInvalidName is returned when a variable is given an empty name.
	ErrInvalidName = errutil.New(http.StatusBadRequest, "variable.invalid_name", "variable name cannot be empty")

	// ErrInvalidType is returned when a variable carries an unknown type.
	ErrInvalidType = errutil.New(http.StatusBadRequest, "variable.invalid_type", "variable type must be list, number, text or time-range")

	// ErrMissingOptions is returned when a list variable has no options.
	ErrMissingOptions = errutil.New(http.StatusBadRequest, "variable.missing_options", "list variable needs at least one option")

	// ErrUnexpectedOptions is returned when a variable other than a list
	// is given options.
	ErrUnexpectedOptions = errutil.New(http.StatusBadRequest, "variable.unexpected_options", "only a list variable has options")

	// ErrInvalidOption is returned when an option is given an empty value.
	ErrInvalidOption = errutil.New(http.StatusBadRequest, "variable.invalid_option", "option value cannot be empty")

	// ErrValueTooLong is returned when a value, a default or an option
	// holds more than _maxValueLength characters.
	ErrValueTooLong = errutil.New(http.StatusBadRequest, "variable.value_too_long", "value cannot be longer than 128 characters")

	// ErrDuplicateOption is returned when a variable's input names the
	// same option twice.
	ErrDuplicateOption = errutil.New(http.StatusBadRequest, "variable.duplicate_option", "option is listed more than once")

	// ErrUnknownOption is returned when an option id does not belong to
	// the variable.
	ErrUnknownOption = errutil.New(http.StatusBadRequest, "variable.unknown_option", "option does not belong to the variable")

	// ErrUnexpectedDefault is returned when a list variable is given a
	// default value: its first option is the default.
	ErrUnexpectedDefault = errutil.New(http.StatusBadRequest, "variable.unexpected_default", "a list variable's default is its first option")

	// ErrMissingDefault is returned when a time range variable has no
	// default value.
	ErrMissingDefault = errutil.New(http.StatusBadRequest, "variable.missing_default", "time range variable needs a default value")

	// ErrInvalidNumber is returned when a number variable is given a value
	// that is not a decimal number.
	ErrInvalidNumber = errutil.New(http.StatusBadRequest, "variable.invalid_number", "value must be a decimal number")

	// ErrInvalidTimeRange is returned when a time range variable is given
	// a value that is not a known preset.
	ErrInvalidTimeRange = errutil.New(http.StatusBadRequest, "variable.invalid_time_range", "value must be a time range preset")

	// ErrBuiltinVariable is returned when anything but the default value of
	// a built-in variable is changed.
	ErrBuiltinVariable = errutil.New(http.StatusForbidden, "variable.builtin", "only the default value of a built-in variable can change")

	// ErrInvalidSortIndex is returned when a move targets a position
	// outside the list.
	ErrInvalidSortIndex = errutil.New(http.StatusBadRequest, "variable.invalid_sort_index", "sort index is out of range")
)

var (
	// _keyPattern is the shape every variable key has.
	_keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	// _referencePattern matches a $ and the whole word after it, so a
	// longer word is never cut down to a shorter key.
	_referencePattern = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

	// _keyReferencePattern matches the start of a reference that can name
	// a key. Macros such as $__from and placeholders such as $1 cannot.
	_keyReferencePattern = regexp.MustCompile(`\$[a-z]`)
)

// Type represents the kind of value a variable holds.
type Type string

// Validate reports whether the type is a known one.
func (t Type) Validate() error {
	switch t {
	case TypeList, TypeNumber, TypeText, TypeTimeRange:
		return nil
	default:
		return ErrInvalidType
	}
}

// MarshalText returns the type as text.
func (t Type) MarshalText() ([]byte, error) {
	return []byte(t), nil
}

// UnmarshalText parses a type, refusing an unknown one so a bad value is
// reported where it is decoded.
func (t *Type) UnmarshalText(text []byte) error {
	v := Type(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return fmt.Errorf("variable type must be one of %s, %s, %s, %s, got %q", TypeList, TypeNumber, TypeText, TypeTimeRange, text)
	}

	*t = v

	return nil
}

// Group represents a named, coloured set of variables.
type Group struct {
	// ID specifies the unique identifier of the group.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID specifies the organization the group belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// GroupName specifies the display name of the group.
	GroupName string `json:"groupName" db:"group_name"`

	// Color specifies the group's colour: a tag palette name, such as
	// "blue".
	Color string `json:"color" db:"color"`

	// SortIndex specifies the position of the group among its
	// organization's groups.
	SortIndex int `json:"sortIndex" db:"sort_index"`

	// Builtin indicates the group ships with every organization and
	// cannot change.
	Builtin bool `json:"builtin" db:"builtin"`

	// CreatedAt specifies the moment the group was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// CreatedBy specifies the user who created the group.
	CreatedBy null.String `json:"createdBy" db:"fk_created_by"`
}

// NewGroup creates a fresh instance of Group from the given input.
func NewGroup(inp GroupCreateInput, organizationID, userID string) Group {
	return Group{
		ID:             xid.New(),
		OrganizationID: organizationID,
		GroupName:      inp.GroupName,
		Color:          inp.Color,
		CreatedAt:      timeutil.Now(),
		CreatedBy:      null.StringFrom(userID),
	}
}

// AllowsChange reports whether the group can be renamed, recoloured,
// moved, deleted or given variables.
func (g Group) AllowsChange() error {
	if g.Builtin {
		return ErrBuiltinGroup
	}

	return nil
}

// GroupCreateInput is the input for creating a group.
type GroupCreateInput struct {
	// GroupName specifies the display name of the group.
	GroupName string `json:"groupName"`

	// Color specifies the group's colour: a tag palette name, such as
	// "blue".
	Color string `json:"color"`
}

// Validate reports whether the input can become a group.
func (gci GroupCreateInput) Validate() error {
	if gci.GroupName == "" {
		return ErrInvalidGroupName
	}

	if !tag.ValidColor(gci.Color) {
		return ErrInvalidGroupColor
	}

	return nil
}

// GroupUpdateInput is the input for renaming or recolouring a group. A
// field left unset keeps its current value.
type GroupUpdateInput struct {
	// GroupName specifies the new display name of the group.
	GroupName null.String `json:"groupName"`

	// Color specifies the new colour of the group.
	Color null.String `json:"color"`
}

// Validate reports whether the input changes a group: at least one field
// has to be set, and a set field is held to the same rules as on creation.
func (gui GroupUpdateInput) Validate() error {
	if !gui.GroupName.Valid && !gui.Color.Valid {
		return ErrEmptyGroupUpdate
	}

	if gui.GroupName.Valid && gui.GroupName.String == "" {
		return ErrInvalidGroupName
	}

	if gui.Color.Valid && !tag.ValidColor(gui.Color.String) {
		return ErrInvalidGroupColor
	}

	return nil
}

// GroupMoveInput is the input for moving a group to another position.
type GroupMoveInput struct {
	// ID specifies the group to move.
	ID xid.ID `json:"id"`

	// SortIndex specifies the position among the groups other than the
	// built-in one that the group ends up at.
	SortIndex int `json:"sortIndex"`
}

// Variable represents a named value pages filter their live data by. Its
// definition is shared by the organization.
type Variable struct {
	// ID specifies the unique identifier of the variable.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID specifies the organization the variable belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// GroupID specifies the group the variable is listed under.
	GroupID xid.ID `json:"groupId" db:"fk_variable_group_id"`

	// Key specifies the name queries and text reference the variable by,
	// without the leading $.
	Key string `json:"key" db:"variable_key"`

	// Name specifies the display name of the variable.
	Name string `json:"name" db:"variable_name"`

	// Description specifies an optional explanation of the variable.
	Description null.String `json:"description" db:"description"`

	// Type specifies the kind of value the variable holds.
	Type Type `json:"type" db:"variable_type"`

	// DefaultValue specifies the value a user who set none sees. For a
	// list it is the id of the first option.
	DefaultValue null.String `json:"defaultValue" db:"default_value"`

	// SortIndex specifies the position of the variable within its group.
	SortIndex int `json:"sortIndex" db:"sort_index"`

	// Builtin indicates the variable ships with every organization and
	// only its default value can change.
	Builtin bool `json:"builtin" db:"builtin"`

	// CreatedAt specifies the moment the variable was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// CreatedBy specifies the user who created the variable.
	CreatedBy null.String `json:"createdBy" db:"fk_created_by"`

	// Options specifies a list variable's options in order; empty for any
	// other type.
	Options []Option `json:"options" db:"-"`
}

// NewVariable creates a fresh instance of Variable from the given input,
// with its options in order. The input is expected to be valid. Every
// option gets a fresh id: an id named by the input could belong to
// another variable.
func NewVariable(inp Input, organizationID, userID string) Variable {
	v := Variable{
		ID:             xid.New(),
		OrganizationID: organizationID,
		CreatedAt:      timeutil.Now(),
		CreatedBy:      null.StringFrom(userID),
	}

	inp.Options = slices.Clone(inp.Options)

	for i := range inp.Options {
		inp.Options[i].ID = null.Value[xid.ID]{}
	}

	return v.apply(inp)
}

// AllowsChange reports whether anything but the default value of the
// variable can change.
func (v Variable) AllowsChange() error {
	if v.Builtin {
		return ErrBuiltinVariable
	}

	return nil
}

// Apply returns the variable as the input describes it. An option keeps
// its id when the input names it; naming an option the variable does not
// have is refused. The receiver is left untouched.
func (v Variable) Apply(inp Input) (Variable, error) {
	for _, oi := range inp.Options {
		if !oi.ID.Valid {
			continue
		}

		if !slices.ContainsFunc(v.Options, func(o Option) bool { return o.ID == oi.ID.V }) {
			return Variable{}, ErrUnknownOption
		}
	}

	return v.apply(inp), nil
}

// apply copies the input's fields onto a copy of the variable and builds
// its options, reusing the ids the input names.
func (v Variable) apply(inp Input) Variable {
	v.GroupID = inp.GroupID
	v.Key = inp.Key
	v.Name = inp.Name
	v.Description = inp.Description
	v.Type = inp.Type
	v.DefaultValue = inp.DefaultValue
	v.Options = make([]Option, 0, len(inp.Options))

	for i, oi := range inp.Options {
		id := oi.ID.V
		if !oi.ID.Valid {
			id = xid.New()
		}

		v.Options = append(v.Options, Option{
			ID:         id,
			VariableID: v.ID,
			Value:      oi.Value,
			SortIndex:  i,
		})
	}

	if v.Type == TypeList && len(v.Options) > 0 {
		v.DefaultValue = null.StringFrom(v.Options[0].ID.String())
	}

	return v
}

// ValidateValue reports whether the value fits the variable: one of its
// options for a list, a decimal for a number, a preset for a time range
// and anything for text.
func (v Variable) ValidateValue(value string) error {
	if utf8.RuneCountInString(value) > _maxValueLength {
		return ErrValueTooLong
	}

	switch v.Type {
	case TypeList:
		if !slices.ContainsFunc(v.Options, func(o Option) bool { return o.ID.String() == value }) {
			return ErrUnknownOption
		}
	case TypeNumber:
		if _, err := decimal.NewFromString(value); err != nil {
			return ErrInvalidNumber
		}
	case TypeTimeRange:
		if !slices.Contains(document.MetricTimeRanges(), value) {
			return ErrInvalidTimeRange
		}
	case TypeText:
	}

	return nil
}

// WithDefault returns the variable with the given default value. A list's
// default is its first option, so the chosen one moves to the top. The
// receiver is left untouched.
func (v Variable) WithDefault(value string) (Variable, error) {
	if err := v.ValidateValue(value); err != nil {
		return Variable{}, err
	}

	v.DefaultValue = null.StringFrom(value)

	if v.Type != TypeList {
		return v, nil
	}

	v.Options = slices.Clone(v.Options)

	i := slices.IndexFunc(v.Options, func(o Option) bool { return o.ID.String() == value })
	sliceutil.Move(v.Options, i, 0)

	for i := range v.Options {
		v.Options[i].SortIndex = i
	}

	return v, nil
}

// Input is the input for creating a variable or replacing its
// definition.
type Input struct {
	// GroupID specifies the group the variable is listed under.
	GroupID xid.ID `json:"groupId"`

	// Key specifies the name queries and text reference the variable by,
	// without the leading $.
	Key string `json:"key"`

	// Name specifies the display name of the variable.
	Name string `json:"name"`

	// Description specifies an optional explanation of the variable.
	Description null.String `json:"description"`

	// Type specifies the kind of value the variable holds.
	Type Type `json:"type"`

	// DefaultValue specifies the value a user who set none sees. A list
	// takes none: its first option is the default.
	DefaultValue null.String `json:"defaultValue"`

	// Options specifies a list variable's options in order.
	Options []OptionInput `json:"options"`
}

// Validate reports whether the input can become a variable.
func (vi Input) Validate() error {
	if !_keyPattern.MatchString(vi.Key) {
		return ErrInvalidKey
	}

	if vi.Name == "" {
		return ErrInvalidName
	}

	if err := vi.Type.Validate(); err != nil {
		return err
	}

	if vi.Type != TypeList {
		if len(vi.Options) > 0 {
			return ErrUnexpectedOptions
		}

		if vi.Type == TypeTimeRange && !vi.DefaultValue.Valid {
			return ErrMissingDefault
		}

		if !vi.DefaultValue.Valid {
			return nil
		}

		return Variable{Type: vi.Type}.ValidateValue(vi.DefaultValue.String)
	}

	if len(vi.Options) == 0 {
		return ErrMissingOptions
	}

	if vi.DefaultValue.Valid {
		return ErrUnexpectedDefault
	}

	seen := make(map[xid.ID]bool, len(vi.Options))

	for _, oi := range vi.Options {
		if oi.Value == "" {
			return ErrInvalidOption
		}

		if utf8.RuneCountInString(oi.Value) > _maxValueLength {
			return ErrValueTooLong
		}

		if !oi.ID.Valid {
			continue
		}

		if seen[oi.ID.V] {
			return ErrDuplicateOption
		}

		seen[oi.ID.V] = true
	}

	return nil
}

// MoveInput is the input for moving a variable to another
// position, in its own group or another one.
type MoveInput struct {
	// ID specifies the variable to move.
	ID xid.ID `json:"id"`

	// GroupID specifies the group the variable ends up in.
	GroupID xid.ID `json:"groupId"`

	// SortIndex specifies the position within that group the variable
	// ends up at.
	SortIndex int `json:"sortIndex"`
}

// ValueInput is the input for setting a variable's default value or one
// user's own value.
type ValueInput struct {
	// Value specifies the value: an option id for a list, a decimal for a
	// number, a preset for a time range, or text.
	Value string `json:"value"`
}

// Option represents one value a list variable can take.
type Option struct {
	// ID specifies the unique identifier of the option.
	ID xid.ID `json:"id" db:"id"`

	// VariableID specifies the variable the option belongs to.
	VariableID xid.ID `json:"variableId" db:"fk_variable_id"`

	// Value specifies the text the option stands for.
	Value string `json:"value" db:"option_value"`

	// SortIndex specifies the position of the option within its variable.
	SortIndex int `json:"sortIndex" db:"sort_index"`
}

// OptionInput is the input for one option of a list variable.
type OptionInput struct {
	// ID specifies the existing option this one keeps the identity of.
	// Unset creates a new option.
	ID null.Value[xid.ID] `json:"id"`

	// Value specifies the text the option stands for.
	Value string `json:"value"`
}

// GroupTree represents a group with its variables in order.
type GroupTree struct {
	Group

	// Variables specifies the group's variables in order.
	Variables []Entry `json:"variables"`
}

// Entry represents a variable with the value one user set.
type Entry struct {
	Variable

	// Value specifies the value the user who asked set, or null when they
	// set none and see the default.
	Value null.String `json:"value" db:"value"`
}

// Resolved returns the value the user sees: their own, else the default.
// A list resolves to the text of the option, not its id.
func (e Entry) Resolved() string {
	value := e.Value.ValueOrZero()
	if !e.Value.Valid {
		value = e.DefaultValue.ValueOrZero()
	}

	if e.Type != TypeList {
		return value
	}

	i := slices.IndexFunc(e.Options, func(o Option) bool { return o.ID.String() == value })
	if i < 0 {
		return ""
	}

	return e.Options[i].Value
}

// Tree represents an organization's groups in order, each with its
// variables.
type Tree []GroupTree

// NewTree arranges groups and entries, each already in order and the
// entries with their options, into a tree.
func NewTree(groups []Group, entries []Entry) Tree {
	byGroup := make(map[xid.ID][]Entry, len(groups))

	for _, e := range entries {
		byGroup[e.GroupID] = append(byGroup[e.GroupID], e)
	}

	tree := make(Tree, 0, len(groups))

	for _, g := range groups {
		vars := byGroup[g.ID]
		if vars == nil {
			vars = []Entry{}
		}

		tree = append(tree, GroupTree{
			Group:     g,
			Variables: vars,
		})
	}

	return tree
}

// Substitute returns the texts with every $key replaced by the resolved
// value of the variable with that key, or by its value in overrides when
// it has one there. The value goes in raw. A reference to no variable,
// such as $1 or the $__from macro, stays as written, and so does an
// override for no variable.
func (t Tree) Substitute(overrides map[string]string, texts ...string) []string {
	values := make(map[string]string)

	for _, g := range t {
		for _, e := range g.Variables {
			value, ok := overrides[e.Key]
			if !ok {
				value = e.Resolved()
			}

			values[e.Key] = value
		}
	}

	out := make([]string, len(texts))

	for i, text := range texts {
		out[i] = _referencePattern.ReplaceAllStringFunc(text, func(ref string) string {
			if value, ok := values[ref[1:]]; ok {
				return value
			}

			return ref
		})
	}

	return out
}

// HasReference reports whether any of the texts may reference a
// variable, so a caller can skip fetching the tree when none does.
func HasReference(texts ...string) bool {
	return slices.ContainsFunc(texts, _keyReferencePattern.MatchString)
}

// PlaceID returns a copy of ids with id at the given index, taken out of
// its old position first when the list holds it. The index counts the
// list without id, so it may be one past the last element.
func PlaceID(ids []xid.ID, id xid.ID, index int) ([]xid.ID, error) {
	placed := slices.DeleteFunc(slices.Clone(ids), func(i xid.ID) bool { return i == id })

	if index < 0 || index > len(placed) {
		return nil, ErrInvalidSortIndex
	}

	return slices.Insert(placed, index, id), nil
}
