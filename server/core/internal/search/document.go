package search

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/rs/xid"
)

// Block represents a information block. It can be a paragraph, heading, or
// any other type of content in a document.
type Block struct {
	// ID is the unique identifier for the block.
	ID string `json:"id"`

	// OrganizationID is the identifier for the organization
	// this block belongs to.
	OrganizationID string `json:"organizationId"`

	// DocumentID is the identifier for the document this block belongs to.
	DocumentID xid.ID `json:"documentId"`

	// BranchID is the identifier for the branch this block belongs to.
	BranchID xid.ID `json:"branchId"`

	// BranchName is the name of the branch this block belongs to.
	BranchName string `json:"branchName"`

	// BranchDefault reports whether the branch is the document's default.
	BranchDefault bool `json:"branchDefault"`

	// Type is the type of the block (e.g., "paragraph", "heading").
	Type string `json:"type"`

	// Text is the text content of the block, if applicable.
	Text string `json:"text"`
}

// Scope is the branch every block of one indexing pass belongs to.
type Scope struct {
	// OrganizationID is the organization owning the document.
	OrganizationID string

	// DocumentID is the document the branch belongs to.
	DocumentID xid.ID

	// BranchID is the branch the blocks are read from.
	BranchID xid.ID

	// BranchName is the branch's name.
	BranchName string

	// BranchDefault reports whether the branch is the document's default.
	BranchDefault bool
}

// Block builds an index entry for uid within the scope. The entry id is
// prefixed with the branch id: a fork copies its source's content, uids
// included, so the uid alone names one block on every branch at once.
func (s Scope) Block(uid, typ, text string) Block {
	return Block{
		ID:             s.BranchID.String() + "-" + uid,
		OrganizationID: s.OrganizationID,
		DocumentID:     s.DocumentID,
		BranchID:       s.BranchID,
		BranchName:     s.BranchName,
		BranchDefault:  s.BranchDefault,
		Type:           typ,
		Text:           text,
	}
}

// BranchRemoval names a branch whose every block is removed. The document
// id is carried so the removal is ordered with the document's other jobs.
type BranchRemoval struct {
	// DocumentID is the document the branch belonged to.
	DocumentID xid.ID `json:"documentId"`

	// BranchID is the removed branch.
	BranchID xid.ID `json:"branchId"`
}

// BlocksDifference represents the differences between two slices of Blocks.
type BlocksDifference struct {
	// Updated contains blocks that have been modified.
	Updated []Block `json:"updated"`

	// Added contains blocks that have been added.
	Added []Block `json:"added"`

	// Removed contains blocks that have been removed.
	Removed []Block `json:"removed"`

	// RemovedBranches contains branches whose every block is removed.
	// Deleting a branch drops its content row, so the index is cleared by
	// branch instead of by block.
	RemovedBranches []BranchRemoval `json:"removedBranches"`

	// RemovedDocuments contains documents whose every block is removed.
	// Deleting a document cascades to its descendants in the database,
	// which makes enumerating their blocks impossible after the fact, so
	// the index is cleared by document instead.
	RemovedDocuments []xid.ID `json:"removedDocuments"`

	// RemovedOrganizations contains organizations whose every block is
	// removed. Organizations are deleted outside of core, so this is the
	// only handle left on the content that went with them.
	RemovedOrganizations []string `json:"removedOrganizations"`
}

// BlocksDiff compares two slices of Blocks and returns the differences
// as a BlocksDifference struct.
func BlocksDiff(a, b map[string]Block) BlocksDifference {
	var diff BlocksDifference

	// Check for added and updated blocks.
	for id, blockB := range b {
		blockA, exists := a[id]
		if !exists {
			diff.Added = append(diff.Added, blockB)
		} else if blockA != blockB {
			diff.Updated = append(diff.Updated, blockB)
		}
	}

	// Check for removed blocks.
	for id, blockA := range a {
		if _, exists := b[id]; !exists {
			diff.Removed = append(diff.Removed, blockA)
		}
	}

	return diff
}

// Scan implements the sql.Scanner interface for BlocksDifference.
func (bd *BlocksDifference) Scan(value any) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan BlocksDifference: expected []byte, got %T", value)
	}

	return json.Unmarshal(bytes, bd)
}

// Value implements the driver.Valuer interface for BlocksDifference.
func (bd BlocksDifference) Value() (driver.Value, error) {
	return json.Marshal(bd)
}

// DocumentSearchJob represents a search index update job.
type DocumentSearchJob struct {
	// ID is the unique identifier for the search job.
	ID int64 `db:"id"`

	// BlockDiff contains the differences in blocks for the document.
	BlockDiff BlocksDifference `db:"block_diff"`
}
