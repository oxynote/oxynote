package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Attributes_Value(t *testing.T) {
	t.Parallel()

	attrs := Attributes{"set": 1, "null": nil}

	v, ok := attrs.Value("set")
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	// the editor writes null for a field nobody filled in, so a key
	// present with a null value is as unset as one that is absent.
	v, ok = attrs.Value("null")
	assert.False(t, ok)
	assert.Nil(t, v)

	v, ok = attrs.Value("missing")
	assert.False(t, ok)
	assert.Nil(t, v)
}

func Test_Attributes_Get(t *testing.T) {
	t.Parallel()

	a := Attributes{"src": "x"}

	assert.Equal(t, Attribute{value: "x"}, a.Get("src"))
	assert.Equal(t, Attribute{}, a.Get("missing"))
}

func Test_Attribute_Int(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result int
	}{
		"Int value":       {Value: 3, Result: 3},
		"Int64 value":     {Value: int64(4), Result: 4},
		"Float64 value":   {Value: float64(5), Result: 5},
		"String mismatch": {Value: "6", Result: 0},
		"Nil value":       {Value: nil, Result: 0},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			a := Attributes{"k": c.Value}.Get("k")
			assert.Equal(t, c.Result, a.Int())
		})
	}
}

func Test_Attribute_String(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result string
	}{
		"String value": {Value: "hi", Result: "hi"},
		"Int mismatch": {Value: 1, Result: ""},
		"Nil value":    {Value: nil, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			a := Attributes{"k": c.Value}.Get("k")
			assert.Equal(t, c.Result, a.String())
		})
	}
}

func Test_Attribute_Bool(t *testing.T) {
	cc := map[string]struct {
		Value  any
		Result bool
	}{
		"Bool value":      {Value: true, Result: true},
		"String mismatch": {Value: "true", Result: false},
		"Nil value":       {Value: nil, Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			a := Attributes{"k": c.Value}.Get("k")
			assert.Equal(t, c.Result, a.Bool())
		})
	}
}
