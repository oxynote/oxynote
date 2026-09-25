package manager

import (
	"context"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

// CopyHooks copies the hooks of one branch to another inside the caller's
// transaction, so the branch's history entry lists them at commit. The
// copies are not set up: the caller runs ProcessBranch on the target once
// its transaction has committed, which creates their external resources.
//
// A branch whose content was duplicated carries fresh block uids, so the
// caller passes the old-to-new uid map and a hook anchored to a block is
// re-anchored through it; a hook whose block the map does not name has
// nothing to point at on the target and is dropped. A nil map keeps every
// block id as it is, which is right for a fork or a merge.
func (m *Manager) CopyHooks(
	ctx context.Context,
	tx CopyTx,
	fromBranchID, toBranchID, documentID xid.ID,
	organizationID string,
	uids map[string]string,
) error {
	hooks, err := tx.FetchDocumentHooksByBranchID(ctx, fromBranchID, organizationID)
	if err != nil {
		return err
	}

	for _, hk := range hooks {
		blockID := hk.BlockID

		if uids != nil && blockID.Valid {
			uid, ok := uids[blockID.String]
			if !ok {
				continue
			}

			blockID = null.StringFrom(uid)
		}

		if err := tx.InsertDocumentHook(ctx, hk.NewCopy(documentID, toBranchID, blockID)); err != nil {
			return err
		}
	}

	return nil
}
