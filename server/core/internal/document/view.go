package document

import "time"

// BranchView represents the latest time a user viewed a document branch.
type BranchView struct {
	// UserID specifies the user who viewed the branch.
	UserID string `json:"userId" db:"fk_user_id"`

	// ViewedAt specifies when the user last viewed the branch.
	ViewedAt time.Time `json:"viewedAt" db:"viewed_at"`
}
