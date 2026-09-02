// Package block provides HTTP handlers for operations on a single block
// of a document.
package block

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// Handler holds dependencies required for block operations.
type Handler struct {
	log    *slog.Logger
	runner Runner
}

// NewHandler creates a new handler instance with the provided logger and
// block runner.
func NewHandler(log *slog.Logger, runner Runner) *Handler {
	return &Handler{
		log:    log,
		runner: runner,
	}
}

// RunBlock handles running the addressed block, which means whatever
// running means for its kind. Anyone who can read the document may ask:
// the block is theirs to see either way, and the run reads the block's
// configuration from the stored branch rather than from the request.
func (h *Handler) RunBlock(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	blockUID, err := httpserver.ExtractParam(r, "blockUid")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := h.runner.Run(
		r.Context(),
		documentID,
		branchID,
		blockUID,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}

// Runner runs one block of a document. The document/blockrun package's
// Runner satisfies it.
//
//go:generate ../../../../scripts/codegen/mock -t internal Runner runner
type Runner interface {
	// Run should run the addressed block and return what its kind
	// reports back.
	Run(ctx context.Context, documentID, branchID xid.ID, blockUID, organizationID string) (any, error)
}
