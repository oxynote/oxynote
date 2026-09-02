// Package blockrun runs the block a request addresses, dispatching to
// whatever running means for that kind of block.
package blockrun

import (
	"context"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/simulation"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// ErrBlockNotFound is returned when the branch carries no block with the
// addressed uid.
var ErrBlockNotFound = errutil.New(
	http.StatusNotFound,
	"block.not_found",
	"No block with this uid exists on this branch.",
)

// ErrNotRunnable is returned when the addressed block is of a kind that
// has nothing to run.
var ErrNotRunnable = errutil.New(
	http.StatusBadRequest,
	"block.not_runnable",
	"This kind of block cannot be run.",
)

// Runner resolves the addressed block out of its branch's stored content
// and hands it to the runner of its kind. What a run reports back is the
// block kind's own payload, which the caller passes through unread.
type Runner struct {
	// db reads the branch content the block lives in.
	db DB

	// simulation runs a metric block, which means probing whether the
	// data it simulates has arrived.
	simulation MetricSimulation
}

// NewRunner creates a fresh instance of Runner.
func NewRunner(db DB, metrics MetricSimulation) *Runner {
	return &Runner{
		db:         db,
		simulation: metrics,
	}
}

// Run runs the addressed block and returns what its kind reports.
func (r *Runner) Run(
	ctx context.Context,
	documentID, branchID xid.ID,
	blockUID, organizationID string,
) (any, error) {
	block, err := r.fetchBlock(ctx, documentID, branchID, blockUID, organizationID)
	if err != nil {
		return nil, err
	}

	switch block.Type {
	case document.BlockNodeMetricBlock:
		res, err := r.simulation.Check(ctx, documentID, branchID, block, organizationID)
		if err != nil {
			return nil, err
		}

		return res, nil
	default:
		return nil, ErrNotRunnable
	}
}

// fetchBlock reads the addressed block out of the branch's stored content,
// which is the only version of it a run may act on: what the request
// carries is an address, never a configuration.
func (r *Runner) fetchBlock(
	ctx context.Context,
	documentID, branchID xid.ID,
	blockUID, organizationID string,
) (document.Block, error) {
	doc, err := r.db.FetchDocumentByBranchID(ctx, branchID, organizationID)
	if err != nil {
		return document.Block{}, err
	}

	// the branch is addressed by its own id, so a mismatch means the
	// caller pointed at a branch of a different document.
	if doc.ID != documentID {
		return document.Block{}, ErrBlockNotFound
	}

	block, ok := doc.Content.FindByUID(blockUID)
	if !ok {
		return document.Block{}, ErrBlockNotFound
	}

	return block, nil
}

// DB reads the branch content a block is resolved out of.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentByBranchID should retrieve the document joined
	// against the branch the id names.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)
}

// MetricSimulation runs a metric block. The datasource/simulation
// package's Checker satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t internal MetricSimulation metric_simulation
type MetricSimulation interface {
	// Check should probe the block's data source and report whether the
	// simulation was taken off the block.
	Check(ctx context.Context, documentID, branchID xid.ID, block document.Block, organizationID string) (simulation.Result, error)
}
