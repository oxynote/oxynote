package document

import "maps"

// Attributes is the attribute map carried by document nodes and
// marks. It behaves as a plain map (range, index, len all work) and
// adds typed access to individual attributes via Get.
type Attributes map[string]any

// Copy returns a shallow copy of the attributes. A nil receiver
// yields an empty, non-nil map, so the result is always safe to
// mutate.
func (a Attributes) Copy() Attributes {
	out := make(Attributes, len(a)+1)

	maps.Copy(out, a)

	return out
}

// Has reports whether the named attribute is present.
func (a Attributes) Has(name string) bool {
	_, ok := a[name]

	return ok
}

// Get returns the named attribute and whether it is present. The
// zero Attribute is returned when the name is absent, so its typed
// accessors yield zero values.
func (a Attributes) Get(name string) (Attribute, bool) {
	v, ok := a[name]

	return Attribute{value: v}, ok
}

// Value returns the named attribute's raw value and whether it is set
// to anything.
//
// It differs from Get in what it calls set: the editor writes null for
// a field nobody has filled in, and a caller reading one back gets the
// null with it, so a key present with a null value is as unset here as
// a key that is absent. Get answers the narrower question of whether
// the key is there at all.
func (a Attributes) Value(name string) (any, bool) {
	v, ok := a[name]
	if !ok || v == nil {
		return nil, false
	}

	return v, true
}

// Attribute is a single attribute value with typed coercions. The
// accessors return the type's zero value when the underlying value
// does not match.
type Attribute struct {
	// value is the raw attribute value as stored in the map.
	value any
}

// Int returns the value coerced to int. It accepts the numeric
// types json.Unmarshal can produce (float64, int, int64) and
// returns 0 for anything else.
func (a Attribute) Int() int {
	switch v := a.value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// String returns the value when it is a string, "" otherwise.
func (a Attribute) String() string {
	if v, ok := a.value.(string); ok {
		return v
	}

	return ""
}

// Bool returns the value when it is a bool, false otherwise.
func (a Attribute) Bool() bool {
	if v, ok := a.value.(bool); ok {
		return v
	}

	return false
}
