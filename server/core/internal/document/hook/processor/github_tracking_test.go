package processor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/shopspring/decimal"
)

// githubErrInput is a test Input whose Github method always fails with the
// configured error.
type githubErrInput struct {
	state State
	err   error
}

func (i githubErrInput) State() State {
	return i.state
}

func (i githubErrInput) Github(_ context.Context) (Github, error) {
	return nil, i.err
}

func (i githubErrInput) ChangeDetection() ChangeDetection {
	return nil
}

func Test_GithubTracking_Process(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Err        error
		WantStatus GithubTrackingStatus
	}{
		"Installation not found degrades to missing installation": {
			Err:        github.ErrInstallationNotFound,
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
		"Github app not configured degrades to missing installation": {
			Err:        github.ErrNotConfigured,
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gt := &GithubTracking{
				Repository: "repo",
				Branch:     "main",
			}

			inp := githubErrInput{
				state: State("{}"),
				err:   tc.Err,
			}

			score, state, err := gt.Process(context.Background(), inp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !score.Equal(decimal.Zero) {
				t.Fatalf("score = %s, want 0", score)
			}

			var gts GithubTrackingState

			if err := json.Unmarshal(state, &gts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gts.Status != tc.WantStatus {
				t.Fatalf("status = %q, want %q", gts.Status, tc.WantStatus)
			}
		})
	}
}

func Test_GithubTracking_Reset(t *testing.T) {
	t.Parallel()

	gt := &GithubTracking{
		Repository: "repo",
		Branch:     "main",
	}

	inp := githubErrInput{
		state: State("{}"),
		err:   github.ErrNotConfigured,
	}

	score, state, err := gt.Reset(context.Background(), inp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !score.Equal(decimal.Zero) {
		t.Fatalf("score = %s, want 0", score)
	}

	var gts GithubTrackingState

	if err := json.Unmarshal(state, &gts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gts.Status != GithubTrackingStatusMissingInstallation {
		t.Fatalf("status = %q, want %q", gts.Status, GithubTrackingStatusMissingInstallation)
	}
}
