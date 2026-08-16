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

	// Type is the type of the block (e.g., "paragraph", "heading").
	Type string `json:"type"`

	// Text is the text content of the block, if applicable.
	Text string `json:"text"`
}

// BlocksDifference represents the differences between two slices of Blocks.
type BlocksDifference struct {
	// Updated contains blocks that have been modified.
	Updated []Block `json:"updated"`

	// Added contains blocks that have been added.
	Added []Block `json:"added"`

	// Removed contains blocks that have been removed.
	Removed []Block `json:"removed"`
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
