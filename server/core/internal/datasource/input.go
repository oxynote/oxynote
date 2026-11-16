package datasource

import "github.com/oxynote/oxynote/server/core/internal/datasource/credentials"

// stateInput is an implementation of the Input interface
// that includes credentials.
type stateInput struct {
	url         string
	credentials credentials.Credentials
}

// newStateInput creates a new state input with the given credentials.
func newStateInput(
	url string,
	creds credentials.Credentials,
) *stateInput {
	return &stateInput{
		url:         url,
		credentials: creds,
	}
}

// URL returns the URL of the data source.
func (si *stateInput) URL() string {
	return si.url
}

// Credentials returns the credentials of the data source.
func (si *stateInput) Credentials() credentials.Credentials {
	return si.credentials
}
