package processor

import "context"

// PlainDeleter is a deleter that performs no operation.
type PlainDeleter struct{}

// Delete performs no operation and always returns nil.
func (*PlainDeleter) Delete(_ context.Context, _ Input) error {
	return nil
}

// PlainSummarizer is a summarizer that reports nothing.
type PlainSummarizer struct{}

// Summary always returns nil.
func (*PlainSummarizer) Summary(_ State) (any, error) {
	return nil, nil //nolint:nilnil // a processor without a summary has nothing to return.
}
