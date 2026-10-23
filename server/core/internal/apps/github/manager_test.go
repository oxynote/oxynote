package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// stubDB creates a DB mock resolving organization installations to the
// given installation ID or error.
func stubDB(installationID int64, err error) *DBMock {
	return &DBMock{
		FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
			return installationID, err
		},
	}
}

// newTestInstallationClient creates an InstallationClient talking to a test
// server serving the given handler.
func newTestInstallationClient(t *testing.T, handler http.Handler) *InstallationClient {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	base := srv.URL + "/"

	client, err := gogithub.NewClient(
		gogithub.WithHTTPClient(srv.Client()),
		gogithub.WithURLs(&base, &base),
	)
	require.NoError(t, err)

	return &InstallationClient{
		owner:              "own",
		installationClient: client,
	}
}

// recordRequests wraps the handler and returns a function listing the paths
// of the requests it has received.
func recordRequests(handler http.Handler) (http.Handler, func() []string) {
	var (
		mu    sync.Mutex
		paths []string
	)

	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()

		paths = append(paths, r.URL.Path)

		mu.Unlock()

		handler.ServeHTTP(w, r)
	})

	return record, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return paths
	}
}

func Test_Options_Validate(t *testing.T) {
	t.Parallel()

	valid := Options{
		AppID:                     123,
		AppSlug:                   "test-app",
		SignatureSecret:           "sig",
		PrivateKeyPath:            "testdata/test-key.pem",
		InstallationSigningSecret: _testSigningSecret,
	}

	cc := map[string]struct {
		Mutate func(o *Options)
		Err    error
	}{
		"Full options are valid": {
			Mutate: func(*Options) {},
		},
		"Missing app ID": {
			Mutate: func(o *Options) { o.AppID = 0 },
			Err:    errors.New("app id is required"),
		},
		"Missing app slug": {
			Mutate: func(o *Options) { o.AppSlug = "" },
			Err:    errors.New("app slug is required"),
		},
		"Missing signature secret": {
			Mutate: func(o *Options) { o.SignatureSecret = "" },
			Err:    errors.New("signature secret is required"),
		},
		"Missing private key path": {
			Mutate: func(o *Options) { o.PrivateKeyPath = "" },
			Err:    errors.New("private key path is required"),
		},
		"Missing installation signing secret": {
			Mutate: func(o *Options) { o.InstallationSigningSecret = "" },
			Err:    errors.New("installation signing secret is required"),
		},
		// the secret is an AES key: a wrong-length one used to boot fine and
		// then fail every install and verify call.
		"Wrong-length installation signing secret": {
			Mutate: func(o *Options) { o.InstallationSigningSecret = "too-short" },
			Err:    fmt.Errorf("installation signing secret: %w", cryptoutil.ErrInvalidKeySize),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			opt := valid
			c.Mutate(&opt)

			testutil.AssertEqualError(t, c.Err, opt.Validate())
		})
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opt            Options
		WantConfigured bool
		Err            error
	}{
		"Zero app ID creates an unconfigured manager": {
			Opt: Options{},
		},
		"App ID with incomplete options fails": {
			Opt: Options{
				AppID:          123,
				AppSlug:        "test-app",
				PrivateKeyPath: "testdata/test-key.pem",
			},
			Err: assert.AnError,
		},
		"App ID with missing private key fails": {
			Opt: Options{
				AppID:                     123,
				AppSlug:                   "test-app",
				SignatureSecret:           "sig",
				PrivateKeyPath:            "testdata/missing.pem",
				InstallationSigningSecret: _testSigningSecret,
			},
			Err: assert.AnError,
		},
		"App ID with valid private key creates a configured manager": {
			Opt: Options{
				AppID:                     123,
				AppSlug:                   "test-app",
				SignatureSecret:           "sig",
				PrivateKeyPath:            "testdata/test-key.pem",
				InstallationSigningSecret: _testSigningSecret,
			},
			WantConfigured: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			man, err := NewManager(nil, c.Opt)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.WantConfigured, man.Configured())
		})
	}
}

func Test_Manager_SignatureSecret(t *testing.T) {
	t.Parallel()

	man, err := NewManager(nil, Options{SignatureSecret: "sig"})
	require.NoError(t, err)

	assert.Equal(t, "sig", man.SignatureSecret())
}

func Test_Manager_GetInstallationClient(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Configured bool
		DB         *DBMock
		Handler    func(t *testing.T) http.Handler
		FetchCalls int
		Requests   []string
		Err        error
	}{
		"Unconfigured manager fails": {
			DB:  &DBMock{},
			Err: ErrNotConfigured,
		},
		"Error returned by DB.FetchGithubInstallationByOrganizationID": {
			Configured: true,
			DB:         stubDB(0, assert.AnError),
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Missing installation record fails": {
			Configured: true,
			DB:         stubDB(0, sql.ErrNoRows),
			FetchCalls: 1,
			Err:        ErrInstallationNotFound,
		},
		"Missing GitHub installation fails": {
			Configured: true,
			DB:         stubDB(42, nil),
			Handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})
			},
			FetchCalls: 1,
			Requests:   []string{"/app/installations/42"},
			Err:        ErrInstallationNotFound,
		},
		"Installation without an account fails": {
			Configured: true,
			DB:         stubDB(42, nil),
			Handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, err := w.Write([]byte(`{"id": 42}`))
					assert.NoError(t, err)
				})
			},
			FetchCalls: 1,
			Requests:   []string{"/app/installations/42"},
			Err:        ErrInstallationNotFound,
		},
		"Complete installation returns a client": {
			Configured: true,
			DB:         stubDB(42, nil),
			Handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, err := w.Write([]byte(`{"id": 42, "account": {"login": "own", "type": "User"}}`))
					assert.NoError(t, err)
				})
			},
			FetchCalls: 1,
			Requests:   []string{"/app/installations/42"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			handler := http.NotFoundHandler()

			if c.Handler != nil {
				handler = c.Handler(t)
			}

			handler, requests := recordRequests(handler)

			var man *Manager

			if c.Configured {
				man = newTestManager(t, c.DB)

				srv := httptest.NewServer(handler)
				t.Cleanup(srv.Close)

				base := srv.URL + "/"

				// the base URL is fixed at construction, so the client is
				// rebuilt around the original one's transport instead of
				// being re-pointed.
				client, err := gogithub.NewClient(
					gogithub.WithHTTPClient(man.appClient.Client()),
					gogithub.WithURLs(&base, &base),
				)
				require.NoError(t, err)

				man.appClient = client
			} else {
				var err error

				man, err = NewManager(c.DB, Options{})
				require.NoError(t, err)
			}

			client, err := man.GetInstallationClient(context.Background(), "org-1")
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, c.DB.FetchGithubInstallationByOrganizationIDCalls(), c.FetchCalls)
			assert.Equal(t, c.Requests, requests())

			if err != nil {
				return
			}

			assert.Equal(t, "own", client.owner)
		})
	}
}

func Test_InstallationClient_FetchRepositories(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Handler       func(t *testing.T) http.HandlerFunc
		Requests      []string
		ExpectedRepos []Repository
		Err           error
	}{
		"Repositories are listed": {
			Handler: func(t *testing.T) http.HandlerFunc {
				t.Helper()

				return func(w http.ResponseWriter, _ *http.Request) {
					_, err := w.Write([]byte(`{"total_count": 1, "repositories": [{"name": "repo", "default_branch": "main"}]}`))
					assert.NoError(t, err)
				}
			},
			Requests: []string{"/installation/repositories"},
			ExpectedRepos: []Repository{
				{Name: "repo", DefaultBranch: "main"},
			},
		},
		"Listing follows every page": {
			Handler: func(t *testing.T) http.HandlerFunc {
				t.Helper()

				// GitHub returns 30 entries per page by default and signals
				// more through the Link header; stopping at the first page
				// hides the rest.
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Get("page") == "" {
						w.Header().Set("Link", `<`+r.URL.Path+`?page=2>; rel="next"`)

						_, err := w.Write([]byte(`{"total_count": 2, "repositories": [{"name": "one", "default_branch": "main"}]}`))
						assert.NoError(t, err)

						return
					}

					_, err := w.Write([]byte(`{"total_count": 2, "repositories": [{"name": "two", "default_branch": "dev"}]}`))
					assert.NoError(t, err)
				}
			},
			Requests: []string{"/installation/repositories", "/installation/repositories"},
			ExpectedRepos: []Repository{
				{Name: "one", DefaultBranch: "main"},
				{Name: "two", DefaultBranch: "dev"},
			},
		},
		"Error returned by Apps.ListRepos": {
			Handler: func(*testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			Requests: []string{"/installation/repositories"},
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()

			// the installation's own repositories, which is what the app was
			// granted and the only form that works for a user account.
			mux.HandleFunc("/installation/repositories", c.Handler(t))

			handler, requests := recordRequests(mux)
			ic := newTestInstallationClient(t, handler)

			repos, err := ic.FetchRepositories(context.Background())

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Requests, requests())
			assert.Equal(t, c.ExpectedRepos, repos)
		})
	}
}

func Test_InstallationClient_FetchRepositoryBranches(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		RepoStatus       int
		BranchesStatus   int
		Requests         []string
		ExpectedBranches []string
		Err              error
	}{
		"Branches are listed": {
			Requests:         []string{"/repos/own/repo", "/repos/own/repo/branches"},
			ExpectedBranches: []string{"main", "dev"},
		},
		"Missing repository fails": {
			RepoStatus: http.StatusNotFound,
			Requests:   []string{"/repos/own/repo"},
			Err:        ErrRepositoryNotFound,
		},
		"Error returned by Repositories.ListBranches": {
			BranchesStatus: http.StatusInternalServerError,
			Requests:       []string{"/repos/own/repo", "/repos/own/repo/branches"},
			Err:            assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if c.RepoStatus != 0 {
					w.WriteHeader(c.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/branches", func(w http.ResponseWriter, _ *http.Request) {
				if c.BranchesStatus != 0 {
					w.WriteHeader(c.BranchesStatus)

					return
				}

				_, err := w.Write([]byte(`[{"name": "main"}, {"name": "dev"}]`))
				assert.NoError(t, err)
			})

			handler, requests := recordRequests(mux)
			ic := newTestInstallationClient(t, handler)

			branches, err := ic.FetchRepositoryBranches(context.Background(), "repo")

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Requests, requests())
			assert.Equal(t, c.ExpectedBranches, branches)
		})
	}
}

func Test_InstallationClient_FetchRepositoryTree(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Branch         string
		RepoStatus     int
		BranchStatus   int
		TreeStatus     int
		Truncated      bool
		Requests       []string
		ExpectedBranch string
		ExpectedTree   Tree
		Err            error
	}{
		"Explicit branch tree is fetched": {
			Branch: "dev",
			Requests: []string{
				"/repos/own/repo",
				"/repos/own/repo/branches/dev",
				"/repos/own/repo/git/trees/dev",
			},
			ExpectedBranch: "dev",
			ExpectedTree: Tree{
				{
					Type: TreeItemTypeFolder, Name: "docs", Checksum: "sha-1",
					Items: []TreeItem{
						{Type: TreeItemTypeFile, Name: "docs/guide.md", Checksum: "sha-2"},
					},
				},
			},
		},
		"Empty branch falls back to the default branch": {
			Requests: []string{
				"/repos/own/repo",
				"/repos/own/repo/branches/main",
				"/repos/own/repo/git/trees/main",
			},
			ExpectedBranch: "main",
			ExpectedTree: Tree{
				{
					Type: TreeItemTypeFolder, Name: "docs", Checksum: "sha-1",
					Items: []TreeItem{
						{Type: TreeItemTypeFile, Name: "docs/guide.md", Checksum: "sha-2"},
					},
				},
			},
		},
		"Missing repository fails": {
			RepoStatus: http.StatusNotFound,
			Requests:   []string{"/repos/own/repo"},
			Err:        ErrRepositoryNotFound,
		},
		"Missing branch fails": {
			Branch:       "gone",
			BranchStatus: http.StatusNotFound,
			Requests: []string{
				"/repos/own/repo",
				"/repos/own/repo/branches/gone",
			},
			Err: ErrRepositoryBranchNotFound,
		},
		"Truncated tree is reported": {
			Branch:    "dev",
			Truncated: true,
			Requests: []string{
				"/repos/own/repo",
				"/repos/own/repo/branches/dev",
				"/repos/own/repo/git/trees/dev",
			},
			Err: ErrTreeTruncated,
		},
		"Error returned by Git.GetTree": {
			Branch:     "dev",
			TreeStatus: http.StatusInternalServerError,
			Requests: []string{
				"/repos/own/repo",
				"/repos/own/repo/branches/dev",
				"/repos/own/repo/git/trees/dev",
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var fetchedBranch string

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if c.RepoStatus != 0 {
					w.WriteHeader(c.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo", "default_branch": "main"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/branches/", func(w http.ResponseWriter, _ *http.Request) {
				if c.BranchStatus != 0 {
					w.WriteHeader(c.BranchStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "branch"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/git/trees/", func(w http.ResponseWriter, r *http.Request) {
				fetchedBranch = r.URL.Path[len("/repos/own/repo/git/trees/"):]

				if c.TreeStatus != 0 {
					w.WriteHeader(c.TreeStatus)

					return
				}

				_, err := w.Write([]byte(`{
					"sha": "root",
					"truncated": ` + strconv.FormatBool(c.Truncated) + `,
					"tree": [
						{"path": "docs", "type": "tree", "sha": "sha-1"},
						{"path": "docs/guide.md", "type": "blob", "sha": "sha-2"}
					]
				}`))
				assert.NoError(t, err)
			})

			handler, requests := recordRequests(mux)
			ic := newTestInstallationClient(t, handler)

			tree, err := ic.FetchRepositoryTree(context.Background(), "repo", c.Branch)

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Requests, requests())

			if err != nil {
				return
			}

			assert.Equal(t, c.ExpectedBranch, fetchedBranch)
			assert.Equal(t, c.ExpectedTree, tree)
		})
	}
}

func Test_parseGithubError(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Inp error
		Err error
	}{
		"Rate limit error maps to too many requests": {
			Inp: &gogithub.RateLimitError{},
			Err: errutil.New(http.StatusTooManyRequests, "github.rate_limit", "rate limit exceeded"),
		},
		"Abuse rate limit error maps to too many requests": {
			Inp: &gogithub.AbuseRateLimitError{},
			Err: errutil.New(http.StatusTooManyRequests, "github.abuse_rate_limit", "abuse rate limit exceeded"),
		},
		"Not found response maps to resource not found": {
			Inp: &gogithub.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusNotFound},
			},
			Err: ErrResourceNotFound,
		},
		"Other response errors pass through": {
			Inp: &gogithub.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusInternalServerError},
			},
			Err: &gogithub.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusInternalServerError},
			},
		},
		"Generic errors pass through": {
			Inp: errors.New("generic"),
			Err: errors.New("generic"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, parseGithubError(c.Inp))
		})
	}
}
