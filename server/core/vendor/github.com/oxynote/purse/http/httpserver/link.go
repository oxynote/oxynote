package httpserver

import "fmt"

// LinkKey is used to access links in the map.
type LinkKey string

// Links holds a map of link strings ready to formatted and built with
// arguments.
// Useful for sending emails etc.
// Strings should abide standard rules of formatting, example:
// "http://yoursite.com/user/activation?token=%s"
type Links struct {
	ll map[LinkKey]string
}

// NewLinks creates a new link store.
func NewLinks(ll map[LinkKey]string) Links {
	return Links{
		ll: ll,
	}
}

// Prep finds the link by the specified key and inserts all provided arguments
// into it (if allowed).
func (l Links) Prep(k LinkKey, args ...any) string {
	if l.ll == nil {
		return ""
	}

	ls, ok := l.ll[k]
	if !ok {
		return ""
	}

	return fmt.Sprintf(ls, args...)
}

// Exist checks whether the links accessed by the specified keys exist or not.
func (l Links) Exist(kk ...LinkKey) bool {
	if l.ll == nil {
		return false
	}

	for _, k := range kk {
		if v, ok := l.ll[k]; !ok || v == "" {
			return false
		}
	}

	return true
}
