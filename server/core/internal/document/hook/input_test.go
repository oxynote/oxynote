package hook

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Input(t *testing.T) {
	t.Parallel()

	githubMan, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	webchangeClient := webchange.NewClient("http://test", "key")

	inp := NewInput("org-1", githubMan, webchangeClient)

	// the unconfigured manager surfaces its sentinel through the getter.
	_, err = inp.Github(context.Background())
	assert.ErrorIs(t, err, github.ErrNotConfigured)

	assert.Same(t, webchangeClient, inp.ChangeDetection())
}

func Test_stateInput_State(t *testing.T) {
	t.Parallel()

	state := processor.State(`{"startedAt": "2026-01-01T00:00:00Z"}`)

	si := newStateInput(NewInput("org-1", nil, nil), state)

	assert.Equal(t, state, si.State())
}
