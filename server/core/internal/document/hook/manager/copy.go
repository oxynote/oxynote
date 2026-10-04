package manager

import (
	"context"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

// CopyHooks inserts copies of the branch's hooks into another branch, in
// the caller's transaction. The copies are not set up. When newBlockIDs is
// set, block hooks follow it to their new block and those it does not name
// are dropped.
func CopyHooks(
	ctx context.Context,
	tx CopyTx,
	fromBranchID, toBranchID, documentID xid.ID,
	organizationID string,
	newBlockIDs map[string]string,
) error {
	hooks, err := tx.FetchDocumentHooksByBranchID(ctx, fromBranchID, organizationID)
	if err != nil {
		return fmt.Errorf("fetching branch hooks: %w", err)
	}

	for _, hk := range hooks {
		blockID := hk.BlockID

		if newBlockIDs != nil && blockID.Valid {
			newID, ok := newBlockIDs[blockID.String]
			if !ok {
				continue
			}

			blockID = null.StringFrom(newID)
		}

		if err := tx.InsertDocumentHook(ctx, hk.NewCopy(documentID, toBranchID, blockID)); err != nil {
			return fmt.Errorf("inserting hook copy: %w", err)
		}
	}

	return nil
}
