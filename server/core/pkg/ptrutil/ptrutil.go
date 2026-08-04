// Package ptrutil provides helper utilities for pointer operations.
package ptrutil

// New returns a pointer to the value passed in.
func New[T any](v T) *T {
	return &v
}
