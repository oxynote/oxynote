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
	"github.com/stretchr/testify/require"
)

// fakeGithubInput is a test Input that hands itself out as the Github
// client. Github fails with clientErr, FetchRepositoryTree returns tree and
// treeErr, and both count their calls.
type fakeGithubInput struct {
	state     State
	clientErr error
	tree      github.Tree
	treeErr   error

	githubCalls int
	fetchCalls  int
}

func (f *fakeGithubInput) State() State {
	return f.state
}

func (f *fakeGithubInput) Github(_ context.Context) (Github, error) {
	f.githubCalls++

	if f.clientErr != nil {
		return nil, f.clientErr
	}

	return f, nil
}

func (f *fakeGithubInput) ChangeDetection() ChangeDetection {
	return nil
}

func (f *fakeGithubInput) FetchRepositoryTree(_ context.Context, _, _ string) (github.Tree, error) {
	f.fetchCalls++

	return f.tree, f.treeErr
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

			testutil.AssertEqualError(t, c.Err, c.Tracking.Validate())
		})
	}
}

func Test_GithubTracking_Process(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp             *fakeGithubInput
		Paths           []string
		WantScore       decimal.Decimal
		WantStatus      Status
		WantChanged     int
		WantGithubCalls int
		WantFetchCalls  int
		Err             error
	}{
		"Installation not found is a status": {
			Inp:             &fakeGithubInput{state: State("{}"), clientErr: github.ErrInstallationNotFound},
			WantStatus:      StatusMissingInstallation,
			WantGithubCalls: 1,
		},
		"Github app not configured is a status": {
			Inp:             &fakeGithubInput{state: State("{}"), clientErr: github.ErrNotConfigured},
			WantStatus:      StatusUnconfigured,
			WantGithubCalls: 1,
		},
		"Error returned by Input.Github": {
			Inp:             &fakeGithubInput{state: State("{}"), clientErr: assert.AnError},
			WantGithubCalls: 1,
			Err:             assert.AnError,
		},
		"Malformed state fails": {
			Inp: &fakeGithubInput{state: State("{not json")},
			Err: assert.AnError,
		},
		"Unchanged path scores full": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				tree:  github.Tree{{Name: "doc.md", Checksum: "sum"}},
			},
			WantScore:       mathutil.Hundred,
			WantStatus:      StatusActive,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Changed checksum scores zero": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{"doc.md":"old"}}`),
				tree:  github.Tree{{Name: "doc.md", Checksum: "sum"}},
			},
			WantStatus:      StatusActive,
			WantChanged:     1,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Every changed path is counted": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{"doc.md":"old","api.md":"sum","gone.md":"sum"}}`),
				tree: github.Tree{
					{Name: "doc.md", Checksum: "sum"},
					{Name: "api.md", Checksum: "sum"},
				},
			},
			Paths:           []string{"doc.md", "api.md", "gone.md"},
			WantStatus:      StatusActive,
			WantChanged:     2,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Count from the previous run is replaced": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{"doc.md":"sum"},"changedPaths":3}`),
				tree:  github.Tree{{Name: "doc.md", Checksum: "sum"}},
			},
			WantScore:       mathutil.Hundred,
			WantStatus:      StatusActive,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Path deleted since reset scores zero": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{"doc.md":"sum"}}`),
				tree:  github.Tree{},
			},
			WantStatus:      StatusActive,
			WantChanged:     1,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Path absent since reset scores full": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{}}`),
				tree:  github.Tree{},
			},
			WantScore:       mathutil.Hundred,
			WantStatus:      StatusActive,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Path appeared after reset scores zero": {
			Inp: &fakeGithubInput{
				state: State(`{"pathsChecksums":{}}`),
				tree:  github.Tree{{Name: "doc.md", Checksum: "sum"}},
			},
			WantStatus:      StatusActive,
			WantChanged:     1,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			paths := c.Paths
			if paths == nil {
				paths = []string{"doc.md"}
			}

			gt := &GithubTracking{
				Repository: "repo",
				Branch:     "main",
				Paths:      paths,
			}

			res, err := gt.Process(context.Background(), c.Inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.WantGithubCalls, c.Inp.githubCalls)
			assert.Equal(t, c.WantFetchCalls, c.Inp.fetchCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.WantStatus, res.Status)
			assert.True(t, res.Score.Equal(c.WantScore), "score = %s, want %s", res.Score, c.WantScore)

			if c.WantStatus != StatusActive {
				assert.Nil(t, res.State)

				return
			}

			var gts GithubTrackingState

			require.NoError(t, json.Unmarshal(res.State, &gts))
			assert.Equal(t, c.WantChanged, gts.ChangedPaths)
		})
	}
}

func Test_GithubTracking_Reset(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp             *fakeGithubInput
		WantStatus      Status
		WantState       string
		WantGithubCalls int
		WantFetchCalls  int
		Err             error
	}{
		"Present paths are recorded and absent ones left out": {
			Inp: &fakeGithubInput{
				tree: github.Tree{{Name: "doc.md", Checksum: "sum"}},
			},
			WantStatus:      StatusActive,
			WantState:       `{"pathsChecksums":{"doc.md":"sum"},"changedPaths":0}`,
			WantGithubCalls: 1,
			WantFetchCalls:  1,
		},
		"Missing installation is a status": {
			Inp:             &fakeGithubInput{clientErr: github.ErrInstallationNotFound},
			WantStatus:      StatusMissingInstallation,
			WantGithubCalls: 1,
		},
		"Error returned by Input.Github": {
			Inp:             &fakeGithubInput{clientErr: assert.AnError},
			WantGithubCalls: 1,
			Err:             assert.AnError,
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
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.WantGithubCalls, c.Inp.githubCalls)
			assert.Equal(t, c.WantFetchCalls, c.Inp.fetchCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.WantStatus, res.Status)

			if c.WantState != "" {
				assert.JSONEq(t, c.WantState, string(res.State))
				assert.True(t, res.Score.Equal(mathutil.Hundred))
			}
		})
	}
}

func Test_GithubTracking_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		State  State
		Result any
		Err    error
	}{
		"Malformed state": {
			State: State(`{`),
			Err:   assert.AnError,
		},
		"Successful summary": {
			State:  State(`{"pathsChecksums":{"a.md":"x"},"changedPaths":2}`),
			Result: GithubTrackingSummary{ChangedPaths: 2},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := (&GithubTracking{}).Summary(c.State)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_GithubTracking_fetchTree(t *testing.T) {
	t.Parallel()

	tree := github.Tree{{Name: "doc.md", Checksum: "sum"}}

	cc := map[string]struct {
		Inp         *fakeGithubInput
		Result      github.Tree
		Status      Status
		GithubCalls int
		FetchCalls  int
		Err         error
	}{
		"Error returned by Input.Github": {
			Inp:         &fakeGithubInput{clientErr: assert.AnError},
			GithubCalls: 1,
			Err:         assert.AnError,
		},
		"Installation not found is reported as state": {
			Inp:         &fakeGithubInput{clientErr: github.ErrInstallationNotFound},
			Status:      StatusMissingInstallation,
			GithubCalls: 1,
		},
		"Github app not configured is reported as state": {
			Inp:         &fakeGithubInput{clientErr: github.ErrNotConfigured},
			Status:      StatusUnconfigured,
			GithubCalls: 1,
		},
		"Error returned by Github.FetchRepositoryTree": {
			Inp:         &fakeGithubInput{treeErr: assert.AnError},
			GithubCalls: 1,
			FetchCalls:  1,
			Err:         assert.AnError,
		},
		"Repository not found is reported as state": {
			Inp:         &fakeGithubInput{treeErr: github.ErrRepositoryNotFound},
			Status:      StatusMissingRepository,
			GithubCalls: 1,
			FetchCalls:  1,
		},
		"Branch not found is reported as state": {
			Inp:         &fakeGithubInput{treeErr: github.ErrRepositoryBranchNotFound},
			Status:      StatusMissingBranch,
			GithubCalls: 1,
			FetchCalls:  1,
		},
		"Truncated tree is reported as state": {
			Inp:         &fakeGithubInput{treeErr: github.ErrTreeTruncated},
			Status:      StatusTreeTruncated,
			GithubCalls: 1,
			FetchCalls:  1,
		},
		"Successful fetch": {
			Inp:         &fakeGithubInput{tree: tree},
			Result:      tree,
			Status:      StatusActive,
			GithubCalls: 1,
			FetchCalls:  1,
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

			assert.Equal(t, c.GithubCalls, c.Inp.githubCalls)
			assert.Equal(t, c.FetchCalls, c.Inp.fetchCalls)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Status, status)
		})
	}
}
