package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/shopspring/decimal"
)

// GithubTrackingStatus represents the status of the
// GitHub tracking processor.
type GithubTrackingStatus string

const (
	// GithubTrackingStatusActive indicates that the GitHub tracking
	// processor is active.
	GithubTrackingStatusActive GithubTrackingStatus = "active"

	// GithubTrackingStatusMissingInstallation indicates that the GitHub
	// App installation is missing for the organization.
	GithubTrackingStatusMissingInstallation GithubTrackingStatus = "missing_installation"

	// GithubTrackingStatusBranchNotFound indicates that the specified branch
	// was not found in the GitHub repository.
	GithubTrackingStatusBranchNotFound GithubTrackingStatus = "missing_branch"

	// GithubTrackingStatusTreeTruncated indicates that the repository tree
	// is too large for GitHub to return in full, so freshness cannot be
	// determined from it.
	GithubTrackingStatusTreeTruncated GithubTrackingStatus = "tree_truncated"

	// GithubTrackingStatusRepositoryNotFound indicates that the specified
	// repository was not found on GitHub.
	GithubTrackingStatusRepositoryNotFound GithubTrackingStatus = "missing_repository"
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

// Process calculates the inactivity duration score.
func (gt *GithubTracking) Process(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	var gts GithubTrackingState

	if err := json.Unmarshal(inp.State(), &gts); err != nil {
		return decimal.Zero, nil, fmt.Errorf("unmarshaling github tracking state: %w", err)
	}

	client, err := inp.Github(ctx)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, github.ErrInstallationNotFound),
		errors.Is(err, github.ErrNotConfigured):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusMissingInstallation)
	default:
		return decimal.Zero, nil, fmt.Errorf("getting github client: %w", err)
	}

	tree, err := client.FetchRepositoryTree(ctx, gt.Repository, gt.Branch)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, github.ErrRepositoryNotFound):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusRepositoryNotFound)
	case errors.Is(err, github.ErrRepositoryBranchNotFound):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusBranchNotFound)
	case errors.Is(err, github.ErrTreeTruncated):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusTreeTruncated)
	default:
		return decimal.Zero, nil, fmt.Errorf("fetching repository tree: %w", err)
	}

	var modified bool

	for _, path := range gt.Paths {
		item, ok := tree.GetItem(path)
		if !ok || item.Checksum != gts.PathsChecksums[path] {
			modified = true
			break
		}
	}

	score := _fullScore
	if modified {
		score = decimal.Zero
	}

	return gts.EncodeState(score, GithubTrackingStatusActive)
}

// Reset resets the state of the inactivity duration processor.
func (gt *GithubTracking) Reset(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	gts := GithubTrackingState{
		Status:         GithubTrackingStatusActive,
		PathsChecksums: make(map[string]string),
	}

	client, err := inp.Github(ctx)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, github.ErrInstallationNotFound),
		errors.Is(err, github.ErrNotConfigured):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusMissingInstallation)
	default:
		return decimal.Zero, nil, fmt.Errorf("getting github client: %w", err)
	}

	tree, err := client.FetchRepositoryTree(ctx, gt.Repository, gt.Branch)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, github.ErrRepositoryNotFound):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusRepositoryNotFound)
	case errors.Is(err, github.ErrRepositoryBranchNotFound):
		return gts.EncodeState(decimal.Zero, GithubTrackingStatusBranchNotFound)
	default:
		return decimal.Zero, nil, fmt.Errorf("fetching repository tree: %w", err)
	}

	for _, path := range gt.Paths {
		item, ok := tree.GetItem(path)
		if !ok {
			continue
		}

		gts.PathsChecksums[path] = item.Checksum
	}

	return gts.EncodeState(_fullScore, GithubTrackingStatusActive)
}

// GithubTrackingState represents the state of the
// GitHub tracking processor.
type GithubTrackingState struct {
	// Status is the current status of the processor.
	Status GithubTrackingStatus `json:"status"`

	// PathsChecksums is a map of file paths to their checksums.
	PathsChecksums map[string]string `json:"pathsChecksums"`
}

// EncodeState encodes the state with the given status and score.
func (gts *GithubTrackingState) EncodeState(score decimal.Decimal, status GithubTrackingStatus) (decimal.Decimal, State, error) {
	gts.Status = status

	state, err := json.Marshal(gts)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling github tracking state: %w", err)
	}

	return score, state, nil
}
