// Package history defines the restorable history entries of a document branch.
package history

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/rs/xid"
)

// Entry is a snapshot of a branch that a restore can put back.
type Entry struct {
	// ID is the unique identifier for the entry.
	ID xid.ID `json:"id" db:"id"`

	// DocumentID is the identifier of the document the entry belongs to.
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// BranchID is the identifier of the branch the entry belongs to.
	BranchID xid.ID `json:"branchId" db:"fk_branch_id"`

	// DocumentName is the name of the document at the time of the entry.
	DocumentName string `json:"documentName" db:"document_name"`

	// Icon is the icon of the document at the time of the entry.
	Icon string `json:"icon" db:"icon"`

	// Content is the content of the branch at the time of the entry.
	Content document.RootBlock `json:"content" db:"content"`

	// Hooks are the hooks the branch carried at the time of the entry.
	Hooks Hooks `json:"hooks" db:"hooks"`

	// LastUpdatedBy is the user whose change the entry records. Null
	// for a system write, which no user made.
	LastUpdatedBy null.String `json:"lastUpdatedBy" db:"fk_last_updated_by"`

	// Boundary indicates whether the entry was written by an operation
	// that replaced the whole content and therefore never aggregates.
	Boundary bool `json:"boundary" db:"boundary"`

	// CreatedAt is the timestamp when the entry was taken.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp of the last edit folded into the entry.
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// NewEntry takes a snapshot of the branch as it is now, at the given time.
// Aggregation into time buckets is decided where the entry is stored, so
// every call mints a fresh id. A boundary entry is one written by an
// operation that replaced the whole content (merge, fork); it never joins
// a bucket and closes the one it lands in.
func NewEntry(doc document.Document, at time.Time, by null.String, hooks Hooks, boundary bool) Entry {
	return Entry{
		ID:            xid.New(),
		DocumentID:    doc.ID,
		BranchID:      doc.BranchID,
		DocumentName:  doc.DocumentName,
		Icon:          doc.Icon,
		Content:       doc.Content,
		Hooks:         hooks,
		LastUpdatedBy: by,
		Boundary:      boundary,
		CreatedAt:     at,
		UpdatedAt:     at,
	}
}

// Head is what placing a new entry needs to know about the branch's
// newest one.
type Head struct {
	// ID is the unique identifier for the entry.
	ID xid.ID `db:"id"`

	// Boundary indicates whether the entry closes its bucket.
	Boundary bool `db:"boundary"`

	// CreatedAt is the timestamp when the entry was taken.
	CreatedAt time.Time `db:"created_at"`

	// Same reports whether the entry records the same name, icon, content
	// and hooks as the one being placed.
	Same bool `db:"same"`
}

// Hook is what it takes to re-create a hook: its type, the block it is
// anchored to and its settings. Watcher state is not part of it.
type Hook struct {
	// Type is the type of the hook.
	Type hook.Type `json:"type"`

	// BlockID is the block the hook is anchored to; null for a document
	// level hook.
	BlockID null.String `json:"blockId"`

	// Settings are the hook's settings in JSON format.
	Settings processor.Settings `json:"settings"`
}

// Hooks is the list of hooks stored on a history entry.
type Hooks []Hook

// NewHooks reduces hooks to what a history entry of the content records
// of them. A hook anchored to a block the content does not hold is left
// out, and one whose block is back is kept even while still soft-deleted.
// The hook sweep catches up with the content only on its next run.
func NewHooks(content document.RootBlock, hooks []hook.Hook) Hooks {
	res := make(Hooks, 0, len(hooks))

	for _, hk := range hooks {
		if hk.BlockID.Valid {
			if _, ok := content.FindByUID(hk.BlockID.String); !ok {
				continue
			}
		}

		res = append(res, Hook{
			Type:     hk.Type,
			BlockID:  hk.BlockID,
			Settings: hk.Settings,
		})
	}

	return res
}

// Value transforms the hooks into a database entry.
func (hh Hooks) Value() (driver.Value, error) {
	if hh == nil {
		return json.Marshal(Hooks{})
	}

	return json.Marshal(hh)
}

// Scan transforms a database entry into hooks.
func (hh *Hooks) Scan(src any) error {
	var pv []byte

	switch v := src.(type) {
	case []byte:
		pv = v
	case string:
		pv = []byte(v)
	default:
		return errors.New("invalid hooks type")
	}

	data := Hooks{}
	if err := json.Unmarshal(pv, &data); err != nil {
		return err
	}

	*hh = data

	return nil
}
