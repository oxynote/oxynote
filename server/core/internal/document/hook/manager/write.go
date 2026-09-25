package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// CreateHook creates the hook's external resource, then stores the hook
// and records its branch in history, credited to updatedBy. It fails
// unless the branch belongs to the document, holds the hook's block and
// the hook can check its target.
func (m *Manager) CreateHook(
	ctx context.Context,
	ci hook.CreateInput,
	documentID xid.ID,
	organizationID string,
	updatedBy string,
) (_ *hook.Hook, err error) {
	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	// the block check and the history entry read the stored content, which
	// must hold what the editors have, such as a block just typed.
	if err = m.flusher.Flush(ctx, documentID, ci.BranchID); err != nil {
		return nil, fmt.Errorf("storing the branch's pending edits: %w", err)
	}

	doc, err := m.db.FetchDocumentByBranchID(ctx, ci.BranchID, organizationID)
	if err != nil {
		return nil, err
	}

	// a hook on another document's branch is missed by that document's
	// cleanup and cites the wrong one in its notifications.
	if doc.ID != documentID {
		return nil, hook.ErrBranchMismatch
	}

	// a hook on a block the branch does not hold would be soft-deleted by
	// the next sweep.
	if ci.BlockID.Valid && !doc.Content.HasBlock(ci.BlockID.String) {
		return nil, hook.ErrBlockNotFound
	}

	hk, err := hook.NewHook(ctx, ci, documentID, organizationID, m.input(organizationID))
	if err != nil {
		// a refusal the hook reports passes through. A service it could
		// not reach is logged here, since the caller only sees the
		// sentinel.
		if errutil.StatusCode(err, false) >= http.StatusInternalServerError {
			m.log.With("error", err).Warn("cannot reach the service a hook checks")

			return nil, hook.ErrUpstreamUnavailable
		}

		return nil, err
	}

	defer func() {
		if err != nil {
			m.undoSetup(ctx, hook.Hook{}, *hk)
		}
	}()

	var tx Tx

	if err = m.db.BeginTx(ctx, &tx); err != nil {
		return nil, err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.InsertDocumentHook(ctx, *hk); err != nil {
		return nil, err
	}

	if err = tx.RecordDocumentBranchHistoryEntry(ctx, ci.BranchID, organizationID, null.StringFrom(updatedBy), false); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	m.changeCallback(*hk)

	return hk, nil
}

// UpdateHook applies the new settings, which resets the hook's state, then
// stores the hook and records its branch in history, credited to
// updatedBy. It fails unless the hook can check its target with the new
// settings. The outside calls run before the transaction opens, so no
// connection waits on them.
func (m *Manager) UpdateHook(
	ctx context.Context,
	id xid.ID,
	organizationID string,
	ui hook.UpdateInput,
	updatedBy string,
) (_ *hook.Hook, err error) {
	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	unlock, err := m.hooks.Lock(ctx, id)
	if err != nil {
		return nil, err
	}

	defer unlock()

	hk, err := m.db.FetchDocumentHook(ctx, id, organizationID)
	if err != nil {
		return nil, err
	}

	if hk.DocumentID.Valid && hk.BranchID.Valid {
		if err = m.flusher.Flush(ctx, hk.DocumentID.V, hk.BranchID.V); err != nil {
			return nil, fmt.Errorf("storing the branch's pending edits: %w", err)
		}
	}

	prev := *hk

	defer func() {
		if err != nil {
			m.undoSetup(ctx, prev, *hk)
		}
	}()

	if err = hk.ApplyUpdate(ctx, ui, m.input(organizationID)); err != nil {
		// a refusal the hook reports passes through. A service it could
		// not reach is logged here, since the caller only sees the
		// sentinel.
		if errutil.StatusCode(err, false) >= http.StatusInternalServerError {
			m.log.With("error", err).Warn("cannot reach the service a hook checks")

			return nil, hook.ErrUpstreamUnavailable
		}

		return nil, err
	}

	var tx Tx

	if err = m.db.BeginTx(ctx, &tx); err != nil {
		return nil, err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.UpdateDocumentHook(ctx, *hk); err != nil {
		return nil, err
	}

	if hk.BranchID.Valid {
		if err = tx.RecordDocumentBranchHistoryEntry(ctx, hk.BranchID.V, organizationID, null.StringFrom(updatedBy), false); err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	m.changeCallback(*hk)

	return hk, nil
}

// DeleteHook tears down the hook's external resource, then removes the row
// and records its branch in history, credited to updatedBy.
func (m *Manager) DeleteHook(ctx context.Context, id xid.ID, organizationID, updatedBy string) error {
	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	unlock, err := m.hooks.Lock(ctx, id)
	if err != nil {
		return err
	}

	defer unlock()

	hk, err := m.db.FetchDocumentHook(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if hk.DocumentID.Valid && hk.BranchID.Valid {
		if err = m.flusher.Flush(ctx, hk.DocumentID.V, hk.BranchID.V); err != nil {
			return fmt.Errorf("storing the branch's pending edits: %w", err)
		}
	}

	if err = hk.Delete(ctx, m.input(organizationID)); err != nil {
		// a refusal the hook reports passes through. A service it could
		// not reach is logged here, since the caller only sees the
		// sentinel.
		if errutil.StatusCode(err, false) >= http.StatusInternalServerError {
			m.log.With("error", err).Warn("cannot reach the service a hook checks")

			return hook.ErrUpstreamUnavailable
		}

		return err
	}

	var tx Tx

	if err = m.db.BeginTx(ctx, &tx); err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.DeleteDocumentHook(ctx, hk.ID); err != nil {
		return err
	}

	if hk.BranchID.Valid {
		if err = tx.RecordDocumentBranchHistoryEntry(ctx, hk.BranchID.V, organizationID, null.StringFrom(updatedBy), false); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	m.changeCallback(*hk)

	return nil
}

// ResetHook restores the hook's score and state and stores them. History
// holds no watcher state, so nothing is recorded. A hook that cannot check
// its target is stored with that status, and its maintainers are told.
func (m *Manager) ResetHook(ctx context.Context, id xid.ID, organizationID string) (_ *hook.Hook, err error) {
	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	unlock, err := m.hooks.Lock(ctx, id)
	if err != nil {
		return nil, err
	}

	defer unlock()

	hk, err := m.db.FetchDocumentHook(ctx, id, organizationID)
	if err != nil {
		return nil, err
	}

	prev := *hk

	defer func() {
		if err != nil {
			m.undoSetup(ctx, prev, *hk)
		}
	}()

	if err = hk.Reset(ctx, m.input(organizationID)); err != nil {
		// a refusal the hook reports passes through. A service it could
		// not reach is logged here, since the caller only sees the
		// sentinel.
		if errutil.StatusCode(err, false) >= http.StatusInternalServerError {
			m.log.With("error", err).Warn("cannot reach the service a hook checks")

			return nil, hook.ErrUpstreamUnavailable
		}

		return nil, err
	}

	if err = m.db.UpdateDocumentHook(ctx, *hk); err != nil {
		return nil, err
	}

	m.changeCallback(*hk)
	m.notifyTransition(ctx, prev, *hk)

	return hk, nil
}
