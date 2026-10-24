package variable

import (
	"strings"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Type_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type Type
		Err  error
	}{
		"List":       {Type: TypeList},
		"Number":     {Type: TypeNumber},
		"Text":       {Type: TypeText},
		"Time range": {Type: TypeTimeRange},
		"Unknown":    {Type: "date", Err: ErrInvalidType},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Type.Validate())
		})
	}
}

func Test_Type_MarshalText(t *testing.T) {
	t.Parallel()

	text, err := TypeTimeRange.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "time-range", string(text))
}

func Test_Type_UnmarshalText(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Text   string
		Result Type
		Err    error
	}{
		"Unknown type": {
			Text: "date",
			Err:  assert.AnError,
		},
		"Known type with padding and capitals": {
			Text:   " Time-Range ",
			Result: TypeTimeRange,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var tp Type

			err := tp.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, tp)
		})
	}
}

func Test_NewGroup(t *testing.T) {
	t.Parallel()

	g := NewGroup(GroupCreateInput{GroupName: "Environment", Color: "green"}, "org", "user")

	assert.NotZero(t, g.ID)
	assert.NotZero(t, g.CreatedAt)
	assert.Equal(t, "org", g.OrganizationID)
	assert.Equal(t, "Environment", g.GroupName)
	assert.Equal(t, "green", g.Color)
	assert.Equal(t, null.StringFrom("user"), g.CreatedBy)
	assert.False(t, g.Builtin)
}

func Test_Group_AllowsChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Group Group
		Err   error
	}{
		"Regular group": {},
		"Built-in group": {
			Group: Group{Builtin: true},
			Err:   ErrBuiltinGroup,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Group.AllowsChange())
		})
	}
}

func Test_GroupCreateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input GroupCreateInput
		Err   error
	}{
		"Empty name": {
			Input: GroupCreateInput{Color: "green"},
			Err:   ErrInvalidGroupName,
		},
		"Off-palette colour": {
			Input: GroupCreateInput{GroupName: "Environment", Color: "#22c55e"},
			Err:   ErrInvalidGroupColor,
		},
		"Valid input": {
			Input: GroupCreateInput{GroupName: "Environment", Color: "green"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_GroupUpdateInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input GroupUpdateInput
		Err   error
	}{
		"Neither field set": {
			Err: ErrEmptyGroupUpdate,
		},
		"Empty name": {
			Input: GroupUpdateInput{GroupName: null.StringFrom("")},
			Err:   ErrInvalidGroupName,
		},
		"Off-palette colour": {
			Input: GroupUpdateInput{Color: null.StringFrom("#22c55e")},
			Err:   ErrInvalidGroupColor,
		},
		"Name only": {
			Input: GroupUpdateInput{GroupName: null.StringFrom("Environment")},
		},
		"Colour only": {
			Input: GroupUpdateInput{Color: null.StringFrom("green")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_NewVariable(t *testing.T) {
	t.Parallel()

	groupID := xid.New()

	foreign := xid.New()

	inp := Input{
		GroupID:     groupID,
		Key:         "region",
		Name:        "Region",
		Description: null.StringFrom("Cloud region"),
		Type:        TypeList,
		Options: []OptionInput{
			{ID: null.ValueFrom(foreign), Value: "eu-west-1"},
			{Value: "us-east-1"},
		},
	}

	v := NewVariable(inp, "org", "user")

	// an id named by the input is replaced, and the input left untouched.
	assert.NotEqual(t, foreign, v.Options[0].ID)
	assert.Equal(t, null.ValueFrom(foreign), inp.Options[0].ID)

	assert.NotZero(t, v.ID)
	assert.NotZero(t, v.CreatedAt)
	assert.Equal(t, "org", v.OrganizationID)
	assert.Equal(t, groupID, v.GroupID)
	assert.Equal(t, "region", v.Key)
	assert.Equal(t, "Region", v.Name)
	assert.Equal(t, null.StringFrom("Cloud region"), v.Description)
	assert.Equal(t, TypeList, v.Type)
	assert.Equal(t, null.StringFrom("user"), v.CreatedBy)
	assert.False(t, v.Builtin)

	require.Len(t, v.Options, 2)
	assert.NotZero(t, v.Options[0].ID)
	assert.Equal(t, v.ID, v.Options[0].VariableID)
	assert.Equal(t, "eu-west-1", v.Options[0].Value)
	assert.Equal(t, 0, v.Options[0].SortIndex)
	assert.Equal(t, "us-east-1", v.Options[1].Value)
	assert.Equal(t, 1, v.Options[1].SortIndex)
	assert.Equal(t, null.StringFrom(v.Options[0].ID.String()), v.DefaultValue)
}

func Test_Variable_AllowsChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Variable Variable
		Err      error
	}{
		"Regular variable": {},
		"Built-in variable": {
			Variable: Variable{Builtin: true},
			Err:      ErrBuiltinVariable,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Variable.AllowsChange())
		})
	}
}

func Test_Variable_Apply(t *testing.T) {
	t.Parallel()

	id := xid.New()
	kept := Option{ID: xid.New(), VariableID: id, Value: "eu-west-1"}

	v := Variable{
		ID:      id,
		Key:     "region",
		Type:    TypeList,
		Options: []Option{kept},
	}

	cc := map[string]struct {
		Input Input
		Check func(t *testing.T, nv Variable)
		Err   error
	}{
		"Unknown option id": {
			Input: Input{
				Type:    TypeList,
				Options: []OptionInput{{ID: null.ValueFrom(xid.New()), Value: "a"}},
			},
			Err: ErrUnknownOption,
		},
		"Kept and new options": {
			Input: Input{
				Key:  "zone",
				Type: TypeList,
				Options: []OptionInput{
					{Value: "us-east-1"},
					{ID: null.ValueFrom(kept.ID), Value: "eu-west-2"},
				},
			},
			Check: func(t *testing.T, nv Variable) {
				t.Helper()

				assert.Equal(t, v.ID, nv.ID)
				assert.Equal(t, "zone", nv.Key)
				require.Len(t, nv.Options, 2)
				assert.NotEqual(t, kept.ID, nv.Options[0].ID)
				assert.Equal(t, Option{ID: kept.ID, VariableID: v.ID, Value: "eu-west-2", SortIndex: 1}, nv.Options[1])
				assert.Equal(t, null.StringFrom(nv.Options[0].ID.String()), nv.DefaultValue)
			},
		},
		"Type change drops options": {
			Input: Input{
				Key:          "region",
				Type:         TypeText,
				DefaultValue: null.StringFrom("eu"),
			},
			Check: func(t *testing.T, nv Variable) {
				t.Helper()

				assert.Equal(t, TypeText, nv.Type)
				assert.Equal(t, null.StringFrom("eu"), nv.DefaultValue)
				assert.Empty(t, nv.Options)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nv, err := v.Apply(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, nv)
			assert.Equal(t, "region", v.Key)
			assert.Equal(t, []Option{kept}, v.Options)
		})
	}
}

func Test_Variable_apply(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input Input
		Check func(t *testing.T, v Variable)
	}{
		"Number without options": {
			Input: Input{
				Type:         TypeNumber,
				DefaultValue: null.StringFrom("2"),
			},
			Check: func(t *testing.T, v Variable) {
				t.Helper()

				assert.Equal(t, TypeNumber, v.Type)
				assert.Equal(t, null.StringFrom("2"), v.DefaultValue)
				assert.Empty(t, v.Options)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			c.Check(t, Variable{ID: xid.New()}.apply(c.Input))
		})
	}
}

func Test_Variable_ValidateValue(t *testing.T) {
	t.Parallel()

	option := Option{ID: xid.New(), Value: "eu-west-1"}

	cc := map[string]struct {
		Type  Type
		Value string
		Err   error
	}{
		"Unknown option": {
			Type:  TypeList,
			Value: xid.New().String(),
			Err:   ErrUnknownOption,
		},
		"Option value instead of id": {
			Type:  TypeList,
			Value: "eu-west-1",
			Err:   ErrUnknownOption,
		},
		"Known option": {
			Type:  TypeList,
			Value: option.ID.String(),
		},
		"Not a number": {
			Type:  TypeNumber,
			Value: "two",
			Err:   ErrInvalidNumber,
		},
		"Text at the length limit": {
			Type:  TypeText,
			Value: strings.Repeat("ą", 128),
		},
		"Text over the length limit": {
			Type:  TypeText,
			Value: strings.Repeat("a", 129),
			Err:   ErrValueTooLong,
		},
		"Empty number": {
			Type:  TypeNumber,
			Value: "",
			Err:   ErrInvalidNumber,
		},
		"Decimal number": {
			Type:  TypeNumber,
			Value: "-2.5",
		},
		"Unknown preset": {
			Type:  TypeTimeRange,
			Value: "last_3_days",
			Err:   ErrInvalidTimeRange,
		},
		"Known preset": {
			Type:  TypeTimeRange,
			Value: "this_week_so_far",
		},
		"Any text": {
			Type:  TypeText,
			Value: "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := Variable{Type: c.Type, Options: []Option{option}}.ValidateValue(c.Value)
			assert.Equal(t, c.Err, err)
		})
	}
}

func Test_Variable_WithDefault(t *testing.T) {
	t.Parallel()

	id := xid.New()

	options := []Option{
		{ID: xid.New(), VariableID: id, Value: "5m"},
		{ID: xid.New(), VariableID: id, Value: "5s", SortIndex: 1},
		{ID: xid.New(), VariableID: id, Value: "1h", SortIndex: 2},
	}

	v := Variable{ID: id, Type: TypeList, Options: options}

	cc := map[string]struct {
		Variable Variable
		Value    string
		Result   Variable
		Err      error
	}{
		"Invalid value": {
			Variable: Variable{Type: TypeNumber},
			Value:    "two",
			Err:      ErrInvalidNumber,
		},
		"Number": {
			Variable: Variable{Type: TypeNumber},
			Value:    "3",
			Result:   Variable{Type: TypeNumber, DefaultValue: null.StringFrom("3")},
		},
		"List option moves to the top": {
			Variable: v,
			Value:    options[2].ID.String(),
			Result: Variable{
				ID:           id,
				Type:         TypeList,
				DefaultValue: null.StringFrom(options[2].ID.String()),
				Options: []Option{
					{ID: options[2].ID, VariableID: id, Value: "1h"},
					{ID: options[0].ID, VariableID: id, Value: "5m", SortIndex: 1},
					{ID: options[1].ID, VariableID: id, Value: "5s", SortIndex: 2},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Variable.WithDefault(c.Value)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, 2, options[2].SortIndex)
		})
	}
}

func Test_Input_Validate(t *testing.T) {
	t.Parallel()

	base := Input{
		Key:  "region",
		Name: "Region",
		Type: TypeList,
		Options: []OptionInput{
			{Value: "eu-west-1"},
		},
	}

	dupID := xid.New()

	with := func(fn func(*Input)) Input {
		vi := base
		fn(&vi)

		return vi
	}

	cc := map[string]struct {
		Input Input
		Err   error
	}{
		"Key with a dollar": {
			Input: with(func(vi *Input) { vi.Key = "$region" }),
			Err:   ErrInvalidKey,
		},
		"Key starting with a digit": {
			Input: with(func(vi *Input) { vi.Key = "1region" }),
			Err:   ErrInvalidKey,
		},
		"Key with capitals": {
			Input: with(func(vi *Input) { vi.Key = "Region" }),
			Err:   ErrInvalidKey,
		},
		"Empty name": {
			Input: with(func(vi *Input) { vi.Name = "" }),
			Err:   ErrInvalidName,
		},
		"Unknown type": {
			Input: with(func(vi *Input) { vi.Type = "date" }),
			Err:   ErrInvalidType,
		},
		"List without options": {
			Input: with(func(vi *Input) { vi.Options = nil }),
			Err:   ErrMissingOptions,
		},
		"List with a default": {
			Input: with(func(vi *Input) { vi.DefaultValue = null.StringFrom("eu-west-1") }),
			Err:   ErrUnexpectedDefault,
		},
		"List with an empty option": {
			Input: with(func(vi *Input) { vi.Options = []OptionInput{{Value: ""}} }),
			Err:   ErrInvalidOption,
		},
		"List with an option over the length limit": {
			Input: with(func(vi *Input) { vi.Options = []OptionInput{{Value: strings.Repeat("a", 129)}} }),
			Err:   ErrValueTooLong,
		},
		"List naming one option twice": {
			Input: with(func(vi *Input) {
				vi.Options = []OptionInput{{ID: null.ValueFrom(dupID), Value: "a"}, {ID: null.ValueFrom(dupID), Value: "b"}}
			}),
			Err: ErrDuplicateOption,
		},
		"List keeping two options": {
			Input: with(func(vi *Input) {
				vi.Options = []OptionInput{{ID: null.ValueFrom(xid.New()), Value: "a"}, {ID: null.ValueFrom(xid.New()), Value: "b"}, {Value: "c"}}
			}),
		},
		"Valid list": {
			Input: base,
		},
		"Number with options": {
			Input: with(func(vi *Input) { vi.Type = TypeNumber }),
			Err:   ErrUnexpectedOptions,
		},
		"Number with an invalid default": {
			Input: with(func(vi *Input) {
				vi.Type = TypeNumber
				vi.Options = nil
				vi.DefaultValue = null.StringFrom("two")
			}),
			Err: ErrInvalidNumber,
		},
		"Number without a default": {
			Input: with(func(vi *Input) {
				vi.Type = TypeNumber
				vi.Options = nil
			}),
		},
		"Text with a default": {
			Input: with(func(vi *Input) {
				vi.Type = TypeText
				vi.Options = nil
				vi.DefaultValue = null.StringFrom("relay-2.14.0")
			}),
		},
		"Time range without a default": {
			Input: with(func(vi *Input) {
				vi.Type = TypeTimeRange
				vi.Options = nil
			}),
			Err: ErrMissingDefault,
		},
		"Time range with an unknown preset": {
			Input: with(func(vi *Input) {
				vi.Type = TypeTimeRange
				vi.Options = nil
				vi.DefaultValue = null.StringFrom("forever")
			}),
			Err: ErrInvalidTimeRange,
		},
		"Valid time range": {
			Input: with(func(vi *Input) {
				vi.Type = TypeTimeRange
				vi.Options = nil
				vi.DefaultValue = null.StringFrom("last_7_days")
			}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_NewTree(t *testing.T) {
	t.Parallel()

	g1 := Group{ID: xid.New(), GroupName: "Metrics"}
	g2 := Group{ID: xid.New(), GroupName: "Deploys"}

	v1 := Entry{ID: xid.New(), GroupID: g1.ID, Type: TypeList}
	v2 := Entry{ID: xid.New(), GroupID: g1.ID, Type: TypeText, Value: null.StringFrom("a")}

	cc := map[string]struct {
		Groups  []Group
		Entries []Entry
		Result  Tree
	}{
		"Groups with and without variables": {
			Groups:  []Group{g1, g2},
			Entries: []Entry{v1, v2},
			Result: Tree{
				{Group: g1, Variables: []Entry{v1, v2}},
				{Group: g2, Variables: []Entry{}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, NewTree(c.Groups, c.Entries))
		})
	}
}

func Test_Entry_Resolved(t *testing.T) {
	t.Parallel()

	a := Option{ID: xid.New(), Value: "prod"}
	b := Option{ID: xid.New(), Value: "staging"}

	cc := map[string]struct {
		Entry  Entry
		Result string
	}{
		"Own value": {
			Entry:  Entry{Type: TypeText, DefaultValue: null.StringFrom("x"), Value: null.StringFrom("y")},
			Result: "y",
		},
		"Default value": {
			Entry:  Entry{Type: TypeText, DefaultValue: null.StringFrom("x")},
			Result: "x",
		},
		"No value": {
			Entry: Entry{Type: TypeText},
		},
		"Number": {
			Entry:  Entry{Type: TypeNumber, DefaultValue: null.StringFrom("1.5")},
			Result: "1.5",
		},
		"Time range": {
			Entry:  Entry{Type: TypeTimeRange, DefaultValue: null.StringFrom("last_24_hours")},
			Result: "last_24_hours",
		},
		"List own option": {
			Entry:  Entry{Type: TypeList, DefaultValue: null.StringFrom(a.ID.String()), Options: []Option{a, b}, Value: null.StringFrom(b.ID.String())},
			Result: "staging",
		},
		"List default option": {
			Entry:  Entry{Type: TypeList, DefaultValue: null.StringFrom(a.ID.String()), Options: []Option{a, b}},
			Result: "prod",
		},
		"List unknown option": {
			Entry: Entry{Type: TypeList, Options: []Option{a, b}, Value: null.StringFrom(xid.New().String())},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Entry.Resolved())
		})
	}
}

func Test_Tree_Substitute(t *testing.T) {
	t.Parallel()

	env := Option{ID: xid.New(), Value: "prod"}

	tree := Tree{
		{Variables: []Entry{
			{Key: "metric_range", Type: TypeTimeRange, DefaultValue: null.StringFrom("last_24_hours")},
			{Key: "env", Type: TypeList, DefaultValue: null.StringFrom(env.ID.String()), Options: []Option{env}},
		}},
		{Variables: []Entry{
			{Key: "step", Type: TypeText, DefaultValue: null.StringFrom("5m"), Value: null.StringFrom("1m")},
			{Key: "name", Type: TypeText, Value: null.StringFrom("o'brien")},
		}},
	}

	cc := map[string]struct {
		Overrides map[string]string
		Texts     []string
		Result    []string
	}{
		"No texts": {
			Result: []string{},
		},
		"Several texts": {
			Texts:  []string{"$env", "up", "$step"},
			Result: []string{"prod", "up", "1m"},
		},
		"No reference": {
			Texts:  []string{"SELECT 1"},
			Result: []string{"SELECT 1"},
		},
		"Own value": {
			Texts:  []string{"rate(x[$step])"},
			Result: []string{"rate(x[1m])"},
		},
		"Default value": {
			Texts:  []string{"SELECT '$metric_range'"},
			Result: []string{"SELECT 'last_24_hours'"},
		},
		"List option text": {
			Texts:  []string{`up{env="$env"}`},
			Result: []string{`up{env="prod"}`},
		},
		"Raw value": {
			Texts:  []string{"WHERE name = '$name'"},
			Result: []string{"WHERE name = 'o'brien'"},
		},
		"Inside a word": {
			Texts:  []string{"requests_$env"},
			Result: []string{"requests_prod"},
		},
		"Several references": {
			Texts:  []string{"$env $step $env"},
			Result: []string{"prod 1m prod"},
		},
		"Longer word": {
			Texts:  []string{"$metric_range_x $envX"},
			Result: []string{"$metric_range_x $envX"},
		},
		"Unknown name": {
			Texts:  []string{"$other"},
			Result: []string{"$other"},
		},
		"Macros and placeholders": {
			Texts:  []string{"$1 $__from ${__from} $__timeGroup(time, $step)"},
			Result: []string{"$1 $__from ${__from} $__timeGroup(time, 1m)"},
		},
		"Unsupported syntax": {
			Texts:  []string{"${env} [[env]] ${env:sqlstring} $"},
			Result: []string{"${env} [[env]] ${env:sqlstring} $"},
		},
		"Overridden value": {
			Overrides: map[string]string{"step": "30s", "env": "dev"},
			Texts:     []string{"$step $env $metric_range"},
			Result:    []string{"30s dev last_24_hours"},
		},
		"Override for no variable": {
			Overrides: map[string]string{"other": "x"},
			Texts:     []string{"$other"},
			Result:    []string{"$other"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, tree.Substitute(c.Overrides, c.Texts...))
		})
	}
}

func Test_HasReference(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Texts  []string
		Result bool
	}{
		"No texts":           {},
		"No reference":       {Texts: []string{"up", "SELECT 1"}},
		"Macros and numbers": {Texts: []string{"$__from ${__to} $1 $Env $"}},
		"Reference":          {Texts: []string{"up", `up{env="$env"}`}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, HasReference(c.Texts...))
		})
	}
}

func Test_PlaceID(t *testing.T) {
	t.Parallel()

	a, b, c, d := xid.New(), xid.New(), xid.New(), xid.New()

	cc := map[string]struct {
		IDs    []xid.ID
		ID     xid.ID
		Index  int
		Result []xid.ID
		Err    error
	}{
		"Negative index": {
			IDs:   []xid.ID{a, b},
			ID:    a,
			Index: -1,
			Err:   ErrInvalidSortIndex,
		},
		"Index past the end of the list without the id": {
			IDs:   []xid.ID{a, b},
			ID:    a,
			Index: 2,
			Err:   ErrInvalidSortIndex,
		},
		"Move forward": {
			IDs:    []xid.ID{a, b, c},
			ID:     a,
			Index:  2,
			Result: []xid.ID{b, c, a},
		},
		"Move backward": {
			IDs:    []xid.ID{a, b, c},
			ID:     c,
			Index:  0,
			Result: []xid.ID{c, a, b},
		},
		"Insert a new id at the end": {
			IDs:    []xid.ID{a, b},
			ID:     d,
			Index:  2,
			Result: []xid.ID{a, b, d},
		},
		"Insert into an empty list": {
			IDs:    []xid.ID{},
			ID:     d,
			Index:  0,
			Result: []xid.ID{d},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := PlaceID(c.IDs, c.ID, c.Index)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
