package processor

import (
	"context"
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

func Test_GithubTracking_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tracking GithubTracking
		Err      error
	}{
		"Repository and paths are valid": {
			Tracking: GithubTracking{Repository: "repo", Paths: []string{"doc.md"}},
		},
		"Missing repository is rejected": {
			Tracking: GithubTracking{Paths: []string{"doc.md"}},
			Err:      ErrInvalidRepository,
		},
		"Repository with owner is rejected": {
			Tracking: GithubTracking{Repository: "owner/repo", Paths: []string{"doc.md"}},
			Err:      ErrInvalidRepository,
		},
		"Missing paths are rejected": {
			Tracking: GithubTracking{Repository: "repo"},
			Err:      ErrMissingPaths,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Err, c.Tracking.Validate())
		})
	}
}

func Test_GithubTracking_Process(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp        Input
		WantScore  decimal.Decimal
		WantStatus Status
		WantErr    bool
	}{
		"Installation not found is a status": {
			Inp:        githubErrInput{state: State("{}"), err: github.ErrInstallationNotFound},
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
		"Github app not configured is a status": {
			Inp:        githubErrInput{state: State("{}"), err: github.ErrNotConfigured},
			WantStatus: StatusUnconfigured,
		},
		"Client failure is propagated": {
			Inp:     githubErrInput{state: State("{}"), err: assert.AnError},
			WantErr: true,
		},
		"Malformed state fails": {
			Inp:     githubErrInput{state: State("{not json")},
			WantErr: true,
		},
		"Unchanged path scores full": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantScore:  mathutil.Hundred,
			WantStatus: StatusActive,
		},
		"Changed checksum scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"old"}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantStatus: StatusActive,
		},
		"Path deleted since reset scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				client: githubTreeClient{tree: github.Tree{}},
			},
			WantStatus: StatusActive,
		},
		"Path absent since reset scores full": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{}}`),
				client: githubTreeClient{tree: github.Tree{}},
			},
			WantScore:  mathutil.Hundred,
			WantStatus: StatusActive,
		},
		"Path appeared after reset scores zero": {
			Inp: githubTreeInput{
				state:  State(`{"pathsChecksums":{}}`),
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantStatus: StatusActive,
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

			res, err := gt.Process(context.Background(), c.Inp)
			if c.WantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, c.WantStatus, res.Status)
			assert.True(t, res.Score.Equal(c.WantScore), "score = %s, want %s", res.Score, c.WantScore)

			if c.WantStatus != StatusActive {
				assert.Nil(t, res.State)
			}
		})
	}
}

func Test_GithubTracking_Reset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp        Input
		WantStatus Status
		WantState  string
		WantErr    bool
	}{
		"Present paths are recorded, absent ones left out": {
			Inp: githubTreeInput{
				client: githubTreeClient{tree: github.Tree{{Name: "doc.md", Checksum: "sum"}}},
			},
			WantStatus: StatusActive,
			WantState:  `{"pathsChecksums":{"doc.md":"sum"}}`,
		},
		"Missing installation is a status": {
			Inp:        githubErrInput{err: github.ErrInstallationNotFound},
			WantStatus: GithubTrackingStatusMissingInstallation,
		},
		"Client failure is propagated": {
			Inp:     githubErrInput{err: assert.AnError},
			WantErr: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			gt := &GithubTracking{
				Repository: "repo",
				Branch:     "main",
				Paths:      []string{"doc.md", "gone.md"},
			}

			res, err := gt.Reset(context.Background(), c.Inp)
			if c.WantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, c.WantStatus, res.Status)

			if c.WantState != "" {
				assert.JSONEq(t, c.WantState, string(res.State))
				assert.True(t, res.Score.Equal(mathutil.Hundred))
			}
		})
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
		Status Status
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
			Status: StatusUnconfigured,
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
			Status: StatusActive,
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
