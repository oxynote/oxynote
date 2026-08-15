package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Attributes_Copy(t *testing.T) {
	// nil receiver
	out := Attributes(nil).Copy()
	require.NotNil(t, out)
	assert.Empty(t, out)

	// populated receiver
	src := Attributes{"level": 2, "icon": "lucide:text"}
	out = src.Copy()
	assert.Equal(t, src, out)

	// mutation independence
	out["level"] = 3

	assert.Equal(t, 2, src["level"])
}

func Test_Attributes_Has(t *testing.T) {
	a := Attributes{"src": "x", "empty": ""}

	assert.True(t, a.Has("src"))
	assert.True(t, a.Has("empty"))
	assert.False(t, a.Has("missing"))
	assert.False(t, Attributes(nil).Has("src"))
}

func Test_Attributes_Get(t *testing.T) {
	a := Attributes{"src": "x"}

	got, ok := a.Get("src")
	assert.True(t, ok)
	assert.Equal(t, "x", got.String())

	got, ok = a.Get("missing")
	assert.False(t, ok)
	assert.Empty(t, got.String())
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

			a, _ := Attributes{"k": c.Value}.Get("k")
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

			a, _ := Attributes{"k": c.Value}.Get("k")
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

			a, _ := Attributes{"k": c.Value}.Get("k")
			assert.Equal(t, c.Result, a.Bool())
		})
	}
}
