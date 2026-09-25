package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/shopspring/decimal"
)

var (
	// ErrInvalidRepository is returned when a GitHub tracking hook names no
	// repository, or names it with its owner.
	ErrInvalidRepository = errutil.New(http.StatusBadRequest, "document_hook.invalid_repository", "invalid repository")

	// ErrMissingPaths is returned when a GitHub tracking hook tracks no paths.
	ErrMissingPaths = errutil.New(http.StatusBadRequest, "document_hook.missing_paths", "at least one path is required")
)

const (
	// GithubTrackingStatusMissingInstallation indicates that the GitHub
	// App installation is missing for the organization.
	GithubTrackingStatusMissingInstallation Status = "missing_installation"

	// GithubTrackingStatusBranchNotFound indicates that the specified branch
	// was not found in the GitHub repository.
	GithubTrackingStatusBranchNotFound Status = "missing_branch"

	// GithubTrackingStatusTreeTruncated indicates that the repository tree
	// is too large for GitHub to return in full, so freshness cannot be
	// determined from it.
	GithubTrackingStatusTreeTruncated Status = "tree_truncated"

	// GithubTrackingStatusRepositoryNotFound indicates that the specified
	// repository was not found on GitHub.
	GithubTrackingStatusRepositoryNotFound Status = "missing_repository"
)

// GithubTracking specifies a processor that tracks the
// changes of files in a GitHub repository.
type GithubTracking struct {
	PlainDeleter

	// Repository is the GitHub repository to track.
	Repository string `json:"repository"`

	// Branch is the branch of the repository to track.
	Branch string `json:"branch"`

	// Paths are the file paths to track.
	Paths []string `json:"paths"`
}

// Validate checks that a repository and at least one path are named. The
// repository is named without its owner, which the installation provides.
func (gt *GithubTracking) Validate() error {
	if gt.Repository == "" || strings.Contains(gt.Repository, "/") {
		return ErrInvalidRepository
	}

	if len(gt.Paths) == 0 {
		return ErrMissingPaths
	}

	return nil
}

// Process scores the hook down once a tracked path changed since the reset.
func (gt *GithubTracking) Process(ctx context.Context, inp Input) (Result, error) {
	var gts GithubTrackingState

	if err := json.Unmarshal(inp.State(), &gts); err != nil {
		return Result{}, fmt.Errorf("unmarshaling github tracking state: %w", err)
	}

	tree, status, err := gt.fetchTree(ctx, inp)
	if err != nil {
		return Result{}, err
	}

	if status != StatusActive {
		return inactive(status), nil
	}

	score := mathutil.Hundred

	for _, path := range gt.Paths {
		item, ok := tree.GetItem(path)
		checksum, tracked := gts.PathsChecksums[path]

		// Reset records absence by leaving the path out of the checksum
		// map, so a path missing both then and now is unchanged; only a
		// presence flip or a differing checksum marks a modification.
		if ok != tracked || (ok && item.Checksum != checksum) {
			score = decimal.Zero
			break
		}
	}

	return active(score, gts)
}

// Reset records the checksums of the tracked paths as the baseline.
func (gt *GithubTracking) Reset(ctx context.Context, inp Input) (Result, error) {
	tree, status, err := gt.fetchTree(ctx, inp)
	if err != nil {
		return Result{}, err
	}

	if status != StatusActive {
		return inactive(status), nil
	}

	gts := GithubTrackingState{
		PathsChecksums: make(map[string]string),
	}

	for _, path := range gt.Paths {
		item, ok := tree.GetItem(path)
		if !ok {
			// a path absent at reset stays out of the checksum map;
			// Process reads the omission as recorded absence and only
			// scores the hook down when the path appears.
			continue
		}

		gts.PathsChecksums[path] = item.Checksum
	}

	return active(mathutil.Hundred, gts)
}

// fetchTree resolves the GitHub client and pulls the tracked repository's
// tree. A status other than active means the tree could not be read for a
// reason the hook reports rather than fails on.
func (gt *GithubTracking) fetchTree(ctx context.Context, inp Input) (github.Tree, Status, error) {
	client, err := inp.Github(ctx)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, github.ErrNotConfigured):
		return nil, StatusUnconfigured, nil
	case errors.Is(err, github.ErrInstallationNotFound):
		return nil, GithubTrackingStatusMissingInstallation, nil
	default:
		return nil, "", fmt.Errorf("getting github client: %w", err)
	}

	tree, err := client.FetchRepositoryTree(ctx, gt.Repository, gt.Branch)

	switch {
	case err == nil:
		return tree, StatusActive, nil
	case errors.Is(err, github.ErrRepositoryNotFound):
		return nil, GithubTrackingStatusRepositoryNotFound, nil
	case errors.Is(err, github.ErrRepositoryBranchNotFound):
		return nil, GithubTrackingStatusBranchNotFound, nil
	case errors.Is(err, github.ErrTreeTruncated):
		return nil, GithubTrackingStatusTreeTruncated, nil
	default:
		return nil, "", fmt.Errorf("fetching repository tree: %w", err)
	}
}

// GithubTrackingState represents the state of the
// GitHub tracking processor.
type GithubTrackingState struct {
	// PathsChecksums is a map of file paths to their checksums.
	PathsChecksums map[string]string `json:"pathsChecksums"`
}
