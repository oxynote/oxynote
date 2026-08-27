package datasource

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newStateInput(t *testing.T) {
	t.Parallel()

	si := newStateInput(
		"http://prometheus.test",
		processor.NewCredentials([]byte(`{"username":"user"}`)),
	)

	require.NotNil(t, si)
	assert.Equal(t, "http://prometheus.test", si.url)
	assert.Equal(t, processor.NewCredentials([]byte(`{"username":"user"}`)), si.credentials)
}

func Test_stateInput_URL(t *testing.T) {
	t.Parallel()

	si := &stateInput{url: "http://prometheus.test"}
	assert.Equal(t, "http://prometheus.test", si.URL())
}

func Test_stateInput_Credentials(t *testing.T) {
	t.Parallel()

	si := &stateInput{credentials: processor.NewCredentials([]byte(`{"username":"user"}`))}
	assert.Equal(t, processor.NewCredentials([]byte(`{"username":"user"}`)), si.Credentials())
}
