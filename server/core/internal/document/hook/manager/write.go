package manager

import (
	"context"
	"net/http"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// CreateHook creates the hook's external resource, then stores the hook
// and records its branch in history, credited to updatedBy. It fails
// unless the hook can check its target. A failed store tears the resource
// down again, since no row would point at it.
func (m *Manager) CreateHook(
	ctx context.Context,
	ci hook.CreateInput,
	documentID xid.ID,
	organizationID string,
	updatedBy string,
) (*hook.Hook, error) {
	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	hk, err := hook.NewHook(ctx, ci, documentID, ci.BranchID, organizationID, m.input(organizationID))
	if err != nil {
		return nil, m.upstream(err)
	}

	err = m.commit(ctx, func(tx Tx) error {
		if ierr := tx.InsertDocumentHook(ctx, *hk); ierr != nil {
			return ierr
		}

		return m.record(ctx, tx, *hk, updatedBy)
	})
	if err != nil {
		m.teardown(ctx, hk, "cannot tear down the hook of a failed insert")

		return nil, err
	}

	return hk, nil
}

// UpdateHook applies the new settings, which resets the hook's state, then
// stores the hook and records its branch in history, credited to
// updatedBy. It fails unless the hook can check its target with the new
// settings.
func (m *Manager) UpdateHook(
	ctx context.Context,
	id xid.ID,
	organizationID string,
	ui hook.UpdateInput,
	updatedBy string,
) (*hook.Hook, error) {
	return m.change(
		ctx,
		id,
		organizationID,
		func(ctx context.Context, hk *hook.Hook) error {
			return m.upstream(hk.ApplyUpdate(ctx, ui, m.input(organizationID)))
		},
		func(ctx context.Context, tx Tx, hk hook.Hook) error {
			if err := tx.UpdateDocumentHook(ctx, hk); err != nil {
				return err
			}

			return m.record(ctx, tx, hk, updatedBy)
		},
	)
}

// DeleteHook tears down the hook's external resource, then removes the row
// and records its branch in history, credited to updatedBy.
func (m *Manager) DeleteHook(ctx context.Context, id xid.ID, organizationID, updatedBy string) error {
	_, err := m.change(
		ctx,
		id,
		organizationID,
		func(ctx context.Context, hk *hook.Hook) error {
			return m.upstream(hk.Delete(ctx, m.input(organizationID)))
		},
		func(ctx context.Context, tx Tx, hk hook.Hook) error {
			if err := tx.DeleteDocumentHook(ctx, hk.ID); err != nil {
				return err
			}

			return m.record(ctx, tx, hk, updatedBy)
		},
	)

	return err
}

// ResetHook restores the hook's score and state and stores them. History
// does not hold watcher state, so nothing is recorded. A hook that cannot
// check its target is stored with that status, since there is nothing to
// refuse, and the maintainers hear of it as from a pass.
func (m *Manager) ResetHook(ctx context.Context, id xid.ID, organizationID string) (*hook.Hook, error) {
	var prev hook.Hook

	hk, err := m.change(
		ctx,
		id,
		organizationID,
		func(ctx context.Context, hk *hook.Hook) error {
			prev = *hk

			return m.upstream(hk.Reset(ctx, m.input(organizationID)))
		},
		func(ctx context.Context, tx Tx, hk hook.Hook) error {
			return tx.UpdateDocumentHook(ctx, hk)
		},
	)
	if err != nil {
		return nil, err
	}

	m.notifyTransition(ctx, prev, *hk)

	return hk, nil
}

// change takes the hook's lock and reads the hook. It then runs the
// hook's outside calls, and only then writes the row in a transaction, so
// no connection waits on an outside service. A change that set up a hook
// never set up before and then fails tears down what the setup created.
func (m *Manager) change(
	ctx context.Context,
	id xid.ID,
	organizationID string,
	run func(context.Context, *hook.Hook) error,
	write func(context.Context, Tx, hook.Hook) error,
) (*hook.Hook, error) {
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

	err = run(ctx, hk)
	if err == nil {
		err = m.commit(ctx, func(tx Tx) error {
			return write(ctx, tx, *hk)
		})
	}

	if err != nil {
		m.discardSetup(ctx, prev, hk)

		return nil, err
	}

	return hk, nil
}

// commit begins a transaction, runs fn in it and commits.
func (m *Manager) commit(ctx context.Context, fn func(Tx) error) error {
	var tx Tx

	if err := m.db.BeginTx(ctx, &tx); err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// record records the hook's branch in history, credited to updatedBy. A
// hook whose branch is gone has no history to record in.
func (m *Manager) record(ctx context.Context, tx Tx, hk hook.Hook, updatedBy string) error {
	if !hk.BranchID.Valid {
		return nil
	}

	_, err := tx.RecordDocumentBranchHistoryEntry(
		ctx,
		hk.BranchID.V,
		hk.OrganizationID.String,
		null.StringFrom(updatedBy),
		false,
	)

	return err
}

// upstream turns a failure to reach the service a hook checks into
// hook.ErrUpstreamUnavailable. Refusals the hook reports itself pass
// through.
func (m *Manager) upstream(err error) error {
	if err == nil || errutil.StatusCode(err, false) < http.StatusInternalServerError {
		return err
	}

	m.log.With("error", err).
		Warn("reaching the service a hook checks")

	return hook.ErrUpstreamUnavailable
}
