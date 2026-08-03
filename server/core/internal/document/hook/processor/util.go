package processor

import "context"

// PlainDeleter is a deleter that performs no operation.
type PlainDeleter struct{}

// Delete performs no operation and always returns nil.
func (*PlainDeleter) Delete(_ context.Context, _ Input) error {
	return nil
}
