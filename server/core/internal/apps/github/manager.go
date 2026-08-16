package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	gogithub "github.com/google/go-github/v72/github"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

var (
	// ErrResourceNotFound is a generic error returned when a GitHub repository tree resource is not found.
	ErrResourceNotFound = errutil.New(http.StatusNotFound, "github.resource_not_found", "resource not found")

	// ErrRepositoryNotFound is returned when a GitHub repository resource is not found.
	ErrRepositoryNotFound = errutil.New(http.StatusNotFound, "github.repository_not_found", "repository not found")

	// ErrRepositoryBranchNotFound is returned when a GitHub branch resource is not found.
	ErrRepositoryBranchNotFound = errutil.New(http.StatusNotFound, "github.repository_branch_not_found", "repository branch not found")

	// ErrInstallationNotFound is returned when a GitHub installation resource is not found.
	ErrInstallationNotFound = errutil.New(http.StatusNotFound, "github.installation_not_found", "installation not found")

	// ErrNotConfigured is returned when the GitHub App integration is not configured on this deployment.
	ErrNotConfigured = errutil.New(http.StatusConflict, "github.not_configured", "github app is not configured")
)

// _maxBranchRedirects specifies the maximum number of redirects followed
// when resolving a repository branch.
const _maxBranchRedirects = 3

// Options holds configuration options for the Github handler.
type Options struct {
	// AppID is the ID of the Github App.
	AppID int64

	// AppSlug is the slug of the Github App.
	AppSlug string

	// SignatureSecret is the secret used to verify Github request signatures.
	SignatureSecret string

	// PrivateKeyPath is the path to the private key used for Github App authentication.
	PrivateKeyPath string

	// InstallationSigningSecret is the secret used to sign installation tokens.
	InstallationSigningSecret string
}

// Validate checks if the required options are set for the Github manager.
func (o Options) Validate() error {
	if o.AppID == 0 {
		return errors.New("app id is required")
	}

	if o.AppSlug == "" {
		return errors.New("app slug is required")
	}

	if o.SignatureSecret == "" {
		return errors.New("signature secret is required")
	}

	if o.PrivateKeyPath == "" {
		return errors.New("private key path is required")
	}

	if o.InstallationSigningSecret == "" {
		return errors.New("installation signing secret is required")
	}

	return nil
}

// Manager represents a GitHub App clients manager.
type Manager struct {
	db        DB
	appClient *gogithub.Client
	opt       Options
}

// NewManager creates a new GitHub App client with the given app ID and private
// key path. A zero AppID means the GitHub App integration is not configured:
// the manager is still created, but Configured reports false and every method
// that talks to GitHub returns ErrNotConfigured. A non-zero AppID requires
// every other option to be set; an incomplete configuration is a
// construction error.
func NewManager(db DB, opt Options) (*Manager, error) {
	if opt.AppID == 0 {
		return &Manager{
			db:  db,
			opt: opt,
		}, nil
	}

	if err := opt.Validate(); err != nil {
		return nil, err
	}

	appClient, err := createAppClient(opt.AppID, opt.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		db:        db,
		opt:       opt,
		appClient: appClient,
	}, nil
}

// Configured reports whether the GitHub App integration is configured.
func (m *Manager) Configured() bool {
	return m.appClient != nil
}

// SignatureSecret returns the signature secret for verifying GitHub request signatures.
func (m *Manager) SignatureSecret() string {
	return m.opt.SignatureSecret
}

// HasInstallationClient checks whether InstallationClient exists for the given organization ID.
func (m *Manager) HasInstallationClient(ctx context.Context, organizationID string) (bool, error) {
	if !m.Configured() {
		return false, ErrNotConfigured
	}

	installationID, err := m.db.FetchGithubInstallationByOrganizationID(ctx, organizationID)

	switch {
	case err == nil:
		// OK.
	case errutil.IsNotFound(err):
		return false, nil
	default:
		return false, err
	}

	_, err = m.createInstallationClient(installationID)
	if err != nil {
		return false, err
	}

	inst, resp, err := m.appClient.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, parseGithubError(err)
	}

	account := inst.GetAccount()
	if account == nil || account.Login == nil || account.Type == nil {
		return false, nil
	}

	return true, nil
}

// GetInstallationClient fetches an InstallationClient for the given installation ID.
func (m *Manager) GetInstallationClient(ctx context.Context, organizationID string) (*InstallationClient, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	installationID, err := m.db.FetchGithubInstallationByOrganizationID(ctx, organizationID)

	switch {
	case err == nil:
		// OK.
	case errutil.IsNotFound(err):
		return nil, ErrInstallationNotFound
	default:
		return nil, err
	}

	installationClient, err := m.createInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	inst, resp, err := m.appClient.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrInstallationNotFound
		}

		return nil, parseGithubError(err)
	}

	account := inst.GetAccount()
	if account == nil || account.Login == nil || account.Type == nil {
		return nil, ErrInstallationNotFound
	}

	return &InstallationClient{
		user:               account.GetType() == "User",
		owner:              account.GetLogin(),
		installationClient: installationClient,
	}, nil
}

// createInstallationClient creates a new Github client using the installation transport.
func (m *Manager) createInstallationClient(installationID int64) (*gogithub.Client, error) {
	rt, err := ghinstallation.NewKeyFromFile(
		http.DefaultTransport,
		m.opt.AppID,
		installationID,
		m.opt.PrivateKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}

	return gogithub.NewClient(
		&http.Client{
			Transport: rt,
		},
	), nil
}

// InstallationClient represents a GitHub App installation client.
type InstallationClient struct {
	user               bool
	owner              string
	installationClient *gogithub.Client
}

// FetchIssues fetches issues from the GitHub repository based on the provided query and repository name.
func (ic *InstallationClient) FetchIssues(ctx context.Context, q, repository string) ([]Issue, error) {
	bq := "in:title"

	if repository != "" { //nolint:nestif // the branching is sequential and readable
		_, resp, err := ic.installationClient.Repositories.Get(ctx, ic.owner, repository)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				return nil, ErrRepositoryNotFound
			}

			return nil, parseGithubError(err)
		}

		bq = "repo:" + ic.owner + "/" + repository + " " + bq
	} else {
		if ic.user {
			bq = "user:" + ic.owner + " " + bq
		} else {
			bq = "org:" + ic.owner + " " + bq
		}
	}

	bq += " " + q

	res, _, err := ic.installationClient.Search.Issues(ctx, bq, nil)
	if err != nil {
		return nil, parseGithubError(err)
	}

	var issues []Issue

	for _, issue := range res.Issues {
		issues = append(issues, Issue{
			ID:        issue.GetID(),
			Title:     issue.GetTitle(),
			URL:       issue.GetHTMLURL(),
			UserID:    issue.GetUser().GetID(),
			Draft:     issue.GetDraft(),
			State:     issue.GetState(),
			UpdatedAt: issue.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
			CreatedAt: issue.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
		})
	}

	return issues, nil
}

// FetchRepositories fetches all repositories accessible by the GitHub App installation.
func (ic *InstallationClient) FetchRepositories(ctx context.Context) ([]Repository, error) {
	repos, _, err := ic.installationClient.Repositories.ListByOrg(
		ctx,
		ic.owner,
		nil,
	)
	if err != nil {
		return nil, parseGithubError(err)
	}

	var res []Repository

	for _, repo := range repos {
		res = append(res, Repository{
			Name:          repo.GetName(),
			DefaultBranch: repo.GetDefaultBranch(),
		})
	}

	return res, nil
}

// FetchRepositoryBranches fetches all branches of the specified repository.
func (ic *InstallationClient) FetchRepositoryBranches(ctx context.Context, repository string) ([]string, error) {
	_, resp, err := ic.installationClient.Repositories.Get(ctx, ic.owner, repository)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrRepositoryNotFound
		}

		return nil, parseGithubError(err)
	}

	branches, _, err := ic.installationClient.Repositories.ListBranches(
		ctx,
		ic.owner,
		repository,
		nil,
	)
	if err != nil {
		return nil, parseGithubError(err)
	}

	var res []string

	for _, branch := range branches {
		res = append(res, branch.GetName())
	}

	return res, nil
}

// FetchRepositoryTree fetches the full recursive file tree of the specified branch in the repository.
func (ic *InstallationClient) FetchRepositoryTree(ctx context.Context, repository, branch string) (Tree, error) {
	repo, resp, err := ic.installationClient.Repositories.Get(ctx, ic.owner, repository)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrRepositoryNotFound
		}

		return nil, parseGithubError(err)
	}

	if branch == "" {
		branch = repo.GetDefaultBranch()
	}

	_, resp, err = ic.installationClient.Repositories.GetBranch(
		ctx,
		ic.owner,
		repository,
		branch,
		_maxBranchRedirects,
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrRepositoryBranchNotFound
		}

		return nil, parseGithubError(err)
	}

	tree, _, err := ic.installationClient.Git.GetTree(
		ctx,
		ic.owner,
		repository,
		branch,
		true,
	)
	if err != nil {
		return nil, parseGithubError(err)
	}

	return ParseTreeItems(tree.Entries), nil
}

// FetchFileContent fetches the content of a file from the specified repository and branch.
func (ic *InstallationClient) FetchFileContent(ctx context.Context, repository, branch, path string) (*FileContent, error) {
	repo, resp, err := ic.installationClient.Repositories.Get(ctx, ic.owner, repository)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrRepositoryNotFound
		}

		return nil, parseGithubError(err)
	}

	if branch == "" {
		branch = repo.GetDefaultBranch()
	}

	fc, _, resp, err := ic.installationClient.Repositories.GetContents(
		ctx,
		ic.owner,
		repository,
		path,
		&gogithub.RepositoryContentGetOptions{
			Ref: branch,
		},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrResourceNotFound
		}

		return nil, parseGithubError(err)
	}

	if fc == nil {
		return nil, ErrResourceNotFound
	}

	content, err := fc.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to decode file content: %w", err)
	}

	return &FileContent{
		Path:    fc.GetPath(),
		Content: content,
		SHA:     fc.GetSHA(),
		Size:    fc.GetSize(),
	}, nil
}

// FetchCodeSearchResults searches for code in the specified repository.
func (ic *InstallationClient) FetchCodeSearchResults(ctx context.Context, repository, query string) ([]CodeSearchResult, error) {
	_, resp, err := ic.installationClient.Repositories.Get(ctx, ic.owner, repository)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrRepositoryNotFound
		}

		return nil, parseGithubError(err)
	}

	q := "repo:" + ic.owner + "/" + repository + " " + query

	res, _, err := ic.installationClient.Search.Code(ctx, q, &gogithub.SearchOptions{
		TextMatch: true,
	})
	if err != nil {
		return nil, parseGithubError(err)
	}

	var results []CodeSearchResult

	for _, cr := range res.CodeResults {
		item := CodeSearchResult{
			Name: cr.GetName(),
			Path: cr.GetPath(),
			URL:  cr.GetHTMLURL(),
		}

		for _, tm := range cr.TextMatches {
			if f := tm.GetFragment(); f != "" {
				item.Fragments = append(item.Fragments, f)
			}
		}

		results = append(results, item)
	}

	return results, nil
}

// createAppClient creates a new Github client using the app transport for GitHub Apps.
func createAppClient(appID int64, privateKeyPath string) (*gogithub.Client, error) {
	rt, err := ghinstallation.NewAppsTransportKeyFromFile(
		http.DefaultTransport,
		appID,
		privateKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}

	return gogithub.NewClient(
		&http.Client{
			Transport: rt,
		},
	), nil
}

// parseGithubError parses errors returned by the Github API and maps them to appropriate HTTP status codes.
func parseGithubError(err error) error {
	var (
		rateErr  *gogithub.RateLimitError
		abuseErr *gogithub.AbuseRateLimitError
		respErr  *gogithub.ErrorResponse
	)

	switch {
	case errors.As(err, &rateErr):
		return errutil.New(http.StatusTooManyRequests, "github.rate_limit", "rate limit exceeded")
	case errors.As(err, &abuseErr):
		return errutil.New(http.StatusTooManyRequests, "github.abuse_rate_limit", "abuse rate limit exceeded")
	case errors.As(err, &respErr):
		if respErr.Response.StatusCode == http.StatusNotFound {
			return ErrResourceNotFound
		}
	}

	return err
}

// DB is an interface that handles communication with the document database.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchGithubInstallationByOrganizationID retrieves the installation ID for a given organization ID.
	FetchGithubInstallationByOrganizationID(ctx context.Context, organizationID string) (int64, error)
}
