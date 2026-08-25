package processor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
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

	cc := map[string]struct {
		Inp        Input
		WantScore  decimal.Decimal
		WantStatus GithubTrackingStatus
	}{
		"Installation not found degrades to missing installation": {
			Inp:        githubErrInput{state: State("{}"), err: github.ErrInstallationNotFound},
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
		"Github app not configured degrades to missing installation": {
			Inp:        githubErrInput{state: State("{}"), err: github.ErrNotConfigured},
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
		"Unchanged path scores full": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantScore:  mathutil.Hundred,
			WantStatus: GithubTrackingStatusActive,
		},
		"Changed checksum scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"old"}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantStatus: GithubTrackingStatusActive,
		},
		"Path deleted since reset scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				client: githubTreeClient{tree: github.Tree{}},
			},
			WantStatus: GithubTrackingStatusActive,
		},
		"Path absent since reset scores full": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{}}`),
				client: githubTreeClient{tree: github.Tree{}},
			},
			WantScore:  mathutil.Hundred,
			WantStatus: GithubTrackingStatusActive,
		},
		"Path appeared after reset scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantStatus: GithubTrackingStatusActive,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			gt := &GithubTracking{
				Repository: "repo",
				Branch:     "main",
				Paths:      []string{"doc.md"},
			}

			score, state, err := gt.Process(context.Background(), c.Inp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !score.Equal(c.WantScore) {
				t.Fatalf("score = %s, want %s", score, c.WantScore)
			}

			var gts GithubTrackingState

			if err := json.Unmarshal(state, &gts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gts.Status != c.WantStatus {
				t.Fatalf("status = %q, want %q", gts.Status, c.WantStatus)
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

// githubTreeClient is a test Github client returning the configured tree
// or error.
type githubTreeClient struct {
	tree github.Tree
	err  error
}

func (c githubTreeClient) FetchRepositoryTree(_ context.Context, _, _ string) (github.Tree, error) {
	return c.tree, c.err
}

// githubTreeInput is a test Input handing out a Github client that returns
// the configured tree or error.
type githubTreeInput struct {
	state  State
	client githubTreeClient
}

func (i githubTreeInput) State() State {
	return i.state
}

func (i githubTreeInput) Github(_ context.Context) (Github, error) {
	return i.client, nil
}

func (i githubTreeInput) ChangeDetection() ChangeDetection {
	return nil
}

func Test_GithubTracking_fetchTree(t *testing.T) {
	tree := github.Tree{{Name: "doc.md", Checksum: "sum"}}

	cc := map[string]struct {
		Inp    Input
		Result github.Tree
		Status GithubTrackingStatus
		Err    error
	}{
		"Error returned by Input.Github": {
			Inp: githubErrInput{err: assert.AnError},
			Err: assert.AnError,
		},
		"Installation not found is reported as state": {
			Inp:    githubErrInput{err: github.ErrInstallationNotFound},
			Status: GithubTrackingStatusMissingInstallation,
		},
		"Github app not configured is reported as state": {
			Inp:    githubErrInput{err: github.ErrNotConfigured},
			Status: GithubTrackingStatusMissingInstallation,
		},
		"Error returned by Github.FetchRepositoryTree": {
			Inp: githubTreeInput{client: githubTreeClient{err: assert.AnError}},
			Err: assert.AnError,
		},
		"Repository not found is reported as state": {
			Inp:    githubTreeInput{client: githubTreeClient{err: github.ErrRepositoryNotFound}},
			Status: GithubTrackingStatusRepositoryNotFound,
		},
		"Branch not found is reported as state": {
			Inp:    githubTreeInput{client: githubTreeClient{err: github.ErrRepositoryBranchNotFound}},
			Status: GithubTrackingStatusBranchNotFound,
		},
		"Truncated tree is reported as state": {
			Inp:    githubTreeInput{client: githubTreeClient{err: github.ErrTreeTruncated}},
			Status: GithubTrackingStatusTreeTruncated,
		},
		"Successful fetch": {
			Inp:    githubTreeInput{client: githubTreeClient{tree: tree}},
			Result: tree,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			gt := &GithubTracking{
				Repository: "repo",
				Branch:     "main",
			}

			res, status, err := gt.fetchTree(context.Background(), c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Status, status)
		})
	}
}
