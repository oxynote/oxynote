package document

import "github.com/rs/xid"

// BranchReviewer represents a reviewer assigned to a specific document branch.
type BranchReviewer struct {
	// BranchID is the branch this reviewer is assigned to.
	BranchID xid.ID `json:"branchId" db:"fk_branch_id"`

	// UserID is the identifier for the user who is reviewing.
	UserID string `json:"userId" db:"fk_user_id"`

	// OrganizationID is the identifier for the organization this reviewer belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// CurrentlyApproved indicates whether the reviewer has approved the current state of this branch.
	CurrentlyApproved bool `json:"currentlyApproved" db:"currently_approved"`

	// PreviouslyApproved indicates whether the reviewer approved a previous version of this branch.
	// Used to suggest reviewers when requesting a new review.
	PreviouslyApproved bool `json:"previouslyApproved" db:"previously_approved"`
}
