package manager

import (
	"context"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

// CopyHooks copies the branch's hooks to another in the caller's
// transaction, not set up yet. A non-nil uids map re-anchors block hooks
// and drops those whose block it does not name.
func CopyHooks(
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
