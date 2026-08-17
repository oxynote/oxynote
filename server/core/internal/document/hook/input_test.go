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

// stubInput builds an input carrying an unconfigured GitHub manager and a
// changedetection client pointing nowhere.
func stubInput(t *testing.T) (*Input, *webchange.Client) {
	t.Helper()

	githubMan, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	webchangeClient := webchange.NewClient("http://test", "key")

	return NewInput("org-1", githubMan, webchangeClient), webchangeClient
}

func Test_NewInput(t *testing.T) {
	t.Parallel()

	inp, webchangeClient := stubInput(t)

	require.NotNil(t, inp)
	assert.Equal(t, "org-1", inp.organizationID)
	assert.Same(t, webchangeClient, inp.webchangeClient)
}

func Test_Input_Github(t *testing.T) {
	t.Parallel()

	inp, _ := stubInput(t)

	// the unconfigured manager surfaces its sentinel through the getter.
	_, err := inp.Github(context.Background())
	assert.ErrorIs(t, err, github.ErrNotConfigured)
}

func Test_Input_ChangeDetection(t *testing.T) {
	t.Parallel()

	inp, webchangeClient := stubInput(t)

	assert.Same(t, webchangeClient, inp.ChangeDetection())
}

func Test_stateInput_State(t *testing.T) {
	t.Parallel()

	state := processor.State(`{"startedAt": "2026-01-01T00:00:00Z"}`)

	si := newStateInput(NewInput("org-1", nil, nil), state)

	assert.Equal(t, state, si.State())
}
