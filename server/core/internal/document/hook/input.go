package hook

import (
	"context"

	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchanges"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
)

// Input represents an Input that provides state information
// for processing freshness hooks.
type Input struct {
	organizationID   string
	githubMan        *githubapp.Manager
	webchangesClient *webchanges.Client
}

// NewInput creates a new Input with the given state.
func NewInput(
	organizationID string,
	githubMan *githubapp.Manager,
	webchangesClient *webchanges.Client,
) *Input {
	return &Input{
		organizationID:   organizationID,
		githubMan:        githubMan,
		webchangesClient: webchangesClient,
	}
}

// Github should return a GitHub client interface.
func (i *Input) Github(ctx context.Context) (processor.Github, error) {
	return i.githubMan.GetInstallationClient(ctx, i.organizationID)
}

// ChangeDetection should return a ChangeDetection client interface.
func (i *Input) ChangeDetection() processor.ChangeDetection {
	return i.webchangesClient
}

// stateInput is an implementation of the Input interface
// that includes a specific state.
type stateInput struct {
	*Input

	state processor.State
}

// newStateInput creates a new state input with the given state.
func newStateInput(inp *Input, state processor.State) *stateInput {
	return &stateInput{
		Input: inp,
		state: state,
	}
}

// State returns the state of the input in JSON format.
func (si *stateInput) State() processor.State {
	return si.state
}
