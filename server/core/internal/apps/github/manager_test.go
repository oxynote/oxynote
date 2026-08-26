package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDB creates a DB mock resolving organization installations to the
// given installation ID or error.
func stubDB(installationID int64, err error) *DBMock {
	return &DBMock{
		FetchGithubInstallationByOrganizationIDFunc: func(context.Context, string) (int64, error) {
			return installationID, err
		},
	}
}

// pointManagerAt points the manager's app client at a test server serving
// the given handler.
func pointManagerAt(t *testing.T, man *Manager, handler http.Handler) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	base := srv.URL + "/"

	// the base URL is fixed at construction, so the client is rebuilt
	// around the original one's transport instead of being re-pointed.
	client, err := gogithub.NewClient(
		gogithub.WithHTTPClient(man.appClient.Client()),
		gogithub.WithURLs(&base, &base),
	)
	require.NoError(t, err)

	man.appClient = client
}

// newTestInstallationClient creates an InstallationClient talking to a test
// server serving the given handler.
func newTestInstallationClient(t *testing.T, user bool, handler http.Handler) *InstallationClient {
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
		user:               user,
		owner:              "own",
		installationClient: client,
	}
}

// installationHandler serves the given GetInstallation response body.
func installationHandler(t *testing.T, body string) http.Handler {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/app/installations/42", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	})

	return mux
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

	tests := map[string]struct {
		Opt            Options
		ExpectErr      bool
		WantConfigured bool
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
			ExpectErr: true,
		},
		"App ID with missing private key fails": {
			Opt: Options{
				AppID:                     123,
				AppSlug:                   "test-app",
				SignatureSecret:           "sig",
				PrivateKeyPath:            "testdata/missing.pem",
				InstallationSigningSecret: _testSigningSecret,
			},
			ExpectErr: true,
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man, err := NewManager(nil, tc.Opt)

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.WantConfigured, man.Configured())
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

	tests := map[string]struct {
		Configured  bool
		DB          *DBMock
		Handler     func(t *testing.T) http.Handler
		ExpectedErr error
	}{
		"Unconfigured manager fails": {
			ExpectedErr: ErrNotConfigured,
		},
		"Database error is propagated": {
			Configured:  true,
			DB:          stubDB(0, assert.AnError),
			ExpectedErr: assert.AnError,
		},
		"Missing installation record fails": {
			Configured:  true,
			DB:          stubDB(0, sql.ErrNoRows),
			ExpectedErr: ErrInstallationNotFound,
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
			ExpectedErr: ErrInstallationNotFound,
		},
		"Installation without an account fails": {
			Configured: true,
			DB:         stubDB(42, nil),
			Handler: func(t *testing.T) http.Handler {
				t.Helper()

				return installationHandler(t, `{"id": 42}`)
			},
			ExpectedErr: ErrInstallationNotFound,
		},
		"Complete installation returns a client": {
			Configured: true,
			DB:         stubDB(42, nil),
			Handler: func(t *testing.T) http.Handler {
				t.Helper()

				return installationHandler(t, `{"id": 42, "account": {"login": "own", "type": "User"}}`)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var man *Manager

			if tc.Configured {
				man = newTestManager(t, tc.DB)
			} else {
				var err error

				man, err = NewManager(tc.DB, Options{})
				require.NoError(t, err)
			}

			if tc.Handler != nil {
				pointManagerAt(t, man, tc.Handler(t))
			}

			client, err := man.GetInstallationClient(context.Background(), "org-1")

			if tc.ExpectedErr != nil {
				testutil.AssertEqualError(t, tc.ExpectedErr, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "own", client.owner)
			assert.True(t, client.user)
		})
	}
}

func Test_InstallationClient_FetchIssues(t *testing.T) {
	t.Parallel()

	const issuesJSON = `{
		"total_count": 1,
		"items": [{
			"id": 7,
			"title": "Bug",
			"html_url": "https://github.com/own/repo/issues/7",
			"user": {"id": 3},
			"draft": false,
			"state": "open",
			"created_at": "2024-01-02T03:04:05Z",
			"updated_at": "2024-01-03T03:04:05Z"
		}]
	}`

	expectedIssues := []Issue{{
		ID:        7,
		Title:     "Bug",
		URL:       "https://github.com/own/repo/issues/7",
		UserID:    3,
		State:     "open",
		CreatedAt: "2024-01-02T03:04:05Z",
		UpdatedAt: "2024-01-03T03:04:05Z",
	}}

	tests := map[string]struct {
		User           bool
		Repository     string
		RepoStatus     int
		SearchStatus   int
		ExpectedQuery  string
		ExpectedErr    error
		ExpectedIssues []Issue
	}{
		"Repository-scoped search returns issues": {
			Repository:     "repo",
			ExpectedQuery:  "repo:own/repo in:title crash",
			ExpectedIssues: expectedIssues,
		},
		"Organization-wide search returns issues": {
			ExpectedQuery:  "org:own in:title crash",
			ExpectedIssues: expectedIssues,
		},
		"User-wide search returns issues": {
			User:           true,
			ExpectedQuery:  "user:own in:title crash",
			ExpectedIssues: expectedIssues,
		},
		"Missing repository fails": {
			Repository:  "repo",
			RepoStatus:  http.StatusNotFound,
			ExpectedErr: ErrRepositoryNotFound,
		},
		"Search failure is propagated": {
			SearchStatus: http.StatusInternalServerError,
			ExpectedErr:  assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var searchedQuery string

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if tc.RepoStatus != 0 {
					w.WriteHeader(tc.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
				searchedQuery = r.URL.Query().Get("q")

				if tc.SearchStatus != 0 {
					w.WriteHeader(tc.SearchStatus)

					return
				}

				_, err := w.Write([]byte(issuesJSON))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, tc.User, mux)

			issues, err := ic.FetchIssues(context.Background(), "crash", tc.Repository)

			testutil.AssertEqualError(t, tc.ExpectedErr, err)

			if tc.ExpectedErr != nil {
				return
			}

			assert.Equal(t, tc.ExpectedQuery, searchedQuery)
			assert.Equal(t, tc.ExpectedIssues, issues)
		})
	}
}

func Test_InstallationClient_FetchRepositories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Status        int
		ExpectedErr   error
		ExpectedRepos []Repository
	}{
		"Repositories are listed": {
			ExpectedRepos: []Repository{
				{Name: "repo", DefaultBranch: "main"},
			},
		},
		"Listing failure is propagated": {
			Status:      http.StatusInternalServerError,
			ExpectedErr: assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()

			// the installation's own repositories, which is what the app was
			// granted and the only form that works for a user account.
			mux.HandleFunc("/installation/repositories", func(w http.ResponseWriter, _ *http.Request) {
				if tc.Status != 0 {
					w.WriteHeader(tc.Status)

					return
				}

				_, err := w.Write([]byte(`{"total_count": 1, "repositories": [{"name": "repo", "default_branch": "main"}]}`))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, false, mux)

			repos, err := ic.FetchRepositories(context.Background())

			testutil.AssertEqualError(t, tc.ExpectedErr, err)
			assert.Equal(t, tc.ExpectedRepos, repos)
		})
	}

	t.Run("Listing follows every page", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()

		// GitHub returns 30 entries per page by default and signals more through
		// the Link header; stopping at the first page hides the rest.
		mux.HandleFunc("/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("Link", `<`+r.URL.Path+`?page=2>; rel="next"`)

				_, err := w.Write([]byte(`{"total_count": 2, "repositories": [{"name": "one", "default_branch": "main"}]}`))
				assert.NoError(t, err)

				return
			}

			_, err := w.Write([]byte(`{"total_count": 2, "repositories": [{"name": "two", "default_branch": "dev"}]}`))
			assert.NoError(t, err)
		})

		ic := newTestInstallationClient(t, false, mux)

		res, err := ic.FetchRepositories(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []Repository{
			{Name: "one", DefaultBranch: "main"},
			{Name: "two", DefaultBranch: "dev"},
		}, res)
	})
}

func Test_InstallationClient_FetchRepositoryBranches(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		RepoStatus       int
		BranchesStatus   int
		ExpectedErr      error
		ExpectedBranches []string
	}{
		"Branches are listed": {
			ExpectedBranches: []string{"main", "dev"},
		},
		"Missing repository fails": {
			RepoStatus:  http.StatusNotFound,
			ExpectedErr: ErrRepositoryNotFound,
		},
		"Listing failure is propagated": {
			BranchesStatus: http.StatusInternalServerError,
			ExpectedErr:    assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if tc.RepoStatus != 0 {
					w.WriteHeader(tc.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/branches", func(w http.ResponseWriter, _ *http.Request) {
				if tc.BranchesStatus != 0 {
					w.WriteHeader(tc.BranchesStatus)

					return
				}

				_, err := w.Write([]byte(`[{"name": "main"}, {"name": "dev"}]`))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, false, mux)

			branches, err := ic.FetchRepositoryBranches(context.Background(), "repo")

			testutil.AssertEqualError(t, tc.ExpectedErr, err)
			assert.Equal(t, tc.ExpectedBranches, branches)
		})
	}
}

func Test_InstallationClient_FetchRepositoryTree(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Branch         string
		RepoStatus     int
		BranchStatus   int
		TreeStatus     int
		Truncated      bool
		ExpectedBranch string
		ExpectedErr    error
		ExpectedTree   Tree
	}{
		"Explicit branch tree is fetched": {
			Branch:         "dev",
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
			RepoStatus:  http.StatusNotFound,
			ExpectedErr: ErrRepositoryNotFound,
		},
		"Missing branch fails": {
			Branch:       "gone",
			BranchStatus: http.StatusNotFound,
			ExpectedErr:  ErrRepositoryBranchNotFound,
		},
		"Truncated tree is reported": {
			Branch:      "dev",
			Truncated:   true,
			ExpectedErr: ErrTreeTruncated,
		},
		"Tree failure is propagated": {
			Branch:      "dev",
			TreeStatus:  http.StatusInternalServerError,
			ExpectedErr: assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var fetchedBranch string

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if tc.RepoStatus != 0 {
					w.WriteHeader(tc.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo", "default_branch": "main"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/branches/", func(w http.ResponseWriter, _ *http.Request) {
				if tc.BranchStatus != 0 {
					w.WriteHeader(tc.BranchStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "branch"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/git/trees/", func(w http.ResponseWriter, r *http.Request) {
				fetchedBranch = r.URL.Path[len("/repos/own/repo/git/trees/"):]

				if tc.TreeStatus != 0 {
					w.WriteHeader(tc.TreeStatus)

					return
				}

				_, err := w.Write([]byte(`{
					"sha": "root",
					"truncated": ` + strconv.FormatBool(tc.Truncated) + `,
					"tree": [
						{"path": "docs", "type": "tree", "sha": "sha-1"},
						{"path": "docs/guide.md", "type": "blob", "sha": "sha-2"}
					]
				}`))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, false, mux)

			tree, err := ic.FetchRepositoryTree(context.Background(), "repo", tc.Branch)

			testutil.AssertEqualError(t, tc.ExpectedErr, err)

			if tc.ExpectedErr != nil {
				return
			}

			assert.Equal(t, tc.ExpectedBranch, fetchedBranch)
			assert.Equal(t, tc.ExpectedTree, tree)
		})
	}
}

func Test_InstallationClient_FetchFileContent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Branch          string
		RepoStatus      int
		ContentsStatus  int
		ContentsBody    string
		ExpectedRef     string
		ExpectedErr     error
		ExpectedContent *FileContent
	}{
		"File content is decoded": {
			Branch: "dev",
			// aGVsbG8= is the base64 form of hello.
			ContentsBody: `{"type": "file", "path": "docs/a.md", "content": "aGVsbG8=", "encoding": "base64", "sha": "sha-1", "size": 5}`,
			ExpectedRef:  "dev",
			ExpectedContent: &FileContent{
				Path:    "docs/a.md",
				Content: "hello",
				SHA:     "sha-1",
				Size:    5,
			},
		},
		"Empty branch falls back to the default branch": {
			ContentsBody: `{"type": "file", "path": "docs/a.md", "content": "aGVsbG8=", "encoding": "base64", "sha": "sha-1", "size": 5}`,
			ExpectedRef:  "main",
			ExpectedContent: &FileContent{
				Path:    "docs/a.md",
				Content: "hello",
				SHA:     "sha-1",
				Size:    5,
			},
		},
		"Missing repository fails": {
			RepoStatus:  http.StatusNotFound,
			ExpectedErr: ErrRepositoryNotFound,
		},
		"Missing file fails": {
			ContentsStatus: http.StatusNotFound,
			ExpectedErr:    ErrResourceNotFound,
		},
		"Directory content fails": {
			ContentsBody: `[{"type": "file", "path": "docs/a.md"}]`,
			ExpectedErr:  ErrResourceNotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var fetchedRef string

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if tc.RepoStatus != 0 {
					w.WriteHeader(tc.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo", "default_branch": "main"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/repos/own/repo/contents/", func(w http.ResponseWriter, r *http.Request) {
				fetchedRef = r.URL.Query().Get("ref")

				if tc.ContentsStatus != 0 {
					w.WriteHeader(tc.ContentsStatus)

					return
				}

				_, err := w.Write([]byte(tc.ContentsBody))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, false, mux)

			fc, err := ic.FetchFileContent(context.Background(), "repo", tc.Branch, "docs/a.md")

			testutil.AssertEqualError(t, tc.ExpectedErr, err)

			if tc.ExpectedErr != nil {
				return
			}

			assert.Equal(t, tc.ExpectedRef, fetchedRef)
			assert.Equal(t, tc.ExpectedContent, fc)
		})
	}
}

func Test_InstallationClient_FetchCodeSearchResults(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		RepoStatus      int
		SearchStatus    int
		ExpectedErr     error
		ExpectedResults []CodeSearchResult
	}{
		"Code search results carry fragments": {
			ExpectedResults: []CodeSearchResult{
				{
					Name:      "a.go",
					Path:      "pkg/a.go",
					URL:       "https://github.com/own/repo/blob/main/pkg/a.go",
					Fragments: []string{"func main()"},
				},
			},
		},
		"Missing repository fails": {
			RepoStatus:  http.StatusNotFound,
			ExpectedErr: ErrRepositoryNotFound,
		},
		"Search failure is propagated": {
			SearchStatus: http.StatusInternalServerError,
			ExpectedErr:  assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var searchedQuery string

			mux := http.NewServeMux()

			mux.HandleFunc("/repos/own/repo", func(w http.ResponseWriter, _ *http.Request) {
				if tc.RepoStatus != 0 {
					w.WriteHeader(tc.RepoStatus)

					return
				}

				_, err := w.Write([]byte(`{"name": "repo"}`))
				assert.NoError(t, err)
			})

			mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
				searchedQuery = r.URL.Query().Get("q")

				if tc.SearchStatus != 0 {
					w.WriteHeader(tc.SearchStatus)

					return
				}

				_, err := w.Write([]byte(`{
					"total_count": 1,
					"items": [{
						"name": "a.go",
						"path": "pkg/a.go",
						"html_url": "https://github.com/own/repo/blob/main/pkg/a.go",
						"text_matches": [{"fragment": "func main()"}, {"fragment": ""}]
					}]
				}`))
				assert.NoError(t, err)
			})

			ic := newTestInstallationClient(t, false, mux)

			results, err := ic.FetchCodeSearchResults(context.Background(), "repo", "main")

			testutil.AssertEqualError(t, tc.ExpectedErr, err)

			if tc.ExpectedErr != nil {
				return
			}

			assert.Equal(t, "repo:own/repo main", searchedQuery)
			assert.Equal(t, tc.ExpectedResults, results)
		})
	}
}

func Test_parseGithubError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Err         error
		ExpectedErr error
	}{
		"Rate limit error maps to too many requests": {
			Err:         &gogithub.RateLimitError{},
			ExpectedErr: errutil.New(http.StatusTooManyRequests, "github.rate_limit", "rate limit exceeded"),
		},
		"Abuse rate limit error maps to too many requests": {
			Err:         &gogithub.AbuseRateLimitError{},
			ExpectedErr: errutil.New(http.StatusTooManyRequests, "github.abuse_rate_limit", "abuse rate limit exceeded"),
		},
		"Not found response maps to resource not found": {
			Err: &gogithub.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusNotFound},
			},
			ExpectedErr: ErrResourceNotFound,
		},
		"Other response errors pass through": {
			Err: &gogithub.ErrorResponse{
				Response: &http.Response{StatusCode: http.StatusInternalServerError},
			},
		},
		"Generic errors pass through": {
			Err: assert.AnError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := parseGithubError(tc.Err)

			if tc.ExpectedErr == nil {
				assert.Equal(t, tc.Err, err)

				return
			}

			assert.Equal(t, tc.ExpectedErr, err)
		})
	}
}
