// Package document provides structures and methods for handling documents in a organization.
package document

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/strutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

var (
	//go:embed files/getting_started.json
	_gettingStartedContent []byte

	//go:embed files/available_metrics.json
	_availableMetrics []byte
)

// DefaultBranch is the name of the default branch for a document.
const DefaultBranch = "main"

const (

	// _aggregationDuration is the duration for which document changes are aggregated.
	_aggregationDuration = 30 * time.Minute

	// _historyEntryTimeLayout is the layout used for timestamps in history
	// entry IDs.
	_historyEntryTimeLayout = "2006-01-02T15:04:05"
)

// Branch holds branch-specific content and metadata for a document.
type Branch struct {
	// BranchID is the unique identifier for this branch row.
	BranchID xid.ID `json:"branchId" db:"branch_id"`

	// BranchName is the branch identifier (e.g. "main", "feature-x").
	BranchName string `json:"branchName" db:"branch_name"`

	// DocumentName is the document display name for this branch.
	DocumentName string `json:"documentName" db:"document_name"`

	// Icon is the icon associated with this branch.
	Icon string `json:"icon" db:"icon"`

	// Content is the content of this branch.
	Content RootBlock `json:"content" db:"content"`

	// RawContent is the raw (binary) representation of the content.
	RawContent []byte `json:"rawContent" db:"raw_content"`

	// Protected indicates whether this branch is protected from direct updates.
	Protected bool `json:"protected" db:"protected"`

	// Default indicates whether this is the default (original) branch of the document.
	Default bool `json:"default" db:"default"`

	// CreatedAt is the timestamp when this branch was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// CreatedBy is the identifier of the user who created this branch.
	CreatedBy null.String `json:"createdBy" db:"fk_created_by"`

	// UpdatedAt is the timestamp when this branch was last updated.
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`

	// LastUpdatedBy is the identifier of the user who last updated this branch.
	LastUpdatedBy null.String `json:"lastUpdatedBy" db:"fk_last_updated_by"`
}

// Document contains document information.
type Document struct {
	Branch

	// ID is the unique identifier for the document.
	ID xid.ID `json:"id" db:"id"`

	// ParentID is the identifier for the parent document, if any.
	ParentID null.Value[xid.ID] `json:"parentId" db:"fk_parent_id"`

	// OrganizationID is the identifier for the organization to which this document belongs.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`
}

// NewDocument creates a new document instance with the provided organization ID.
func NewDocument(inp CreateInput, organizationID, createdBy string) Document {
	now := timeutil.Now()

	return Document{
		ID:             xid.New(),
		ParentID:       inp.ParentID,
		OrganizationID: organizationID,
		BranchID:       xid.New(),
		BranchName:     DefaultBranch,
		DocumentName:   inp.Name,
		Icon:           inp.Icon,
		Content:        NewDocumentContent(),
		Protected:      false,
		Default:        true,
		CreatedAt:      now,
		CreatedBy:      null.StringFrom(createdBy),
		UpdatedAt:      now,
		LastUpdatedBy:  null.StringFrom(createdBy),
	}
}

// ApplyUpdate updates document instance with the input data.
func (d Document) ApplyUpdate(inp UpdateInput) (Document, error) {
	if d.Protected && !inp.System {
		return Document{}, httpserver.ErrNotPermitted
	}

	nd := d

	lastUpdatedBy := d.LastUpdatedBy

	if len(inp.Maintainers) > 0 {
		lastUpdatedBy = null.StringFrom(inp.Maintainers[len(inp.Maintainers)-1])
	}

	if !inp.System {
		nd.UpdatedAt = timeutil.Now()
	}

	nd.BranchName = inp.Branch
	nd.DocumentName = inp.Name
	nd.Icon = inp.Icon
	nd.Content = inp.Content
	nd.RawContent = inp.RawContent
	nd.LastUpdatedBy = lastUpdatedBy

	return nd, nil
}

// MergeBranch applies source branch changes onto this document's main content.
func (d Document) MergeBranch(source Branch, mergedBy string) Document {
	now := timeutil.Now()

	nd := d
	nd.Branch = Branch{
		BranchID:      d.BranchID,
		BranchName:    d.BranchName,
		DocumentName:  source.DocumentName,
		Icon:          source.Icon,
		Content:       source.Content.StripCommentMarks(),
		RawContent:    nil,
		Protected:     d.Protected,
		Default:       d.Default,
		CreatedAt:     d.CreatedAt,
		CreatedBy:     d.CreatedBy,
		UpdatedAt:     now,
		LastUpdatedBy: null.StringFrom(mergedBy),
	}

	return nd
}

// Fork returns a new branch of the document carrying this branch's content.
// Comment marks are stripped because comments belong to one branch, and the
// Yjs state is cleared so the first load rebuilds it from the stripped
// content. A fork is never protected or default, whatever its source is.
func (d Document) Fork(branchName, createdBy string) Document {
	now := timeutil.Now()

	nd := d
	nd.Branch = Branch{
		BranchID:      xid.New(),
		BranchName:    branchName,
		DocumentName:  d.DocumentName,
		Icon:          d.Icon,
		Content:       d.Content.StripCommentMarks(),
		RawContent:    nil,
		Protected:     false,
		Default:       false,
		CreatedAt:     now,
		CreatedBy:     null.StringFrom(createdBy),
		UpdatedAt:     now,
		LastUpdatedBy: null.StringFrom(createdBy),
	}

	return nd
}

// ApplyProtection sets the protection status of the document branch.
func (d Document) ApplyProtection(protected bool, updatedBy string) Document {
	nd := d
	nd.Protected = protected
	nd.LastUpdatedBy = null.StringFrom(updatedBy)

	return nd
}

// Content is a lightweight view of one document at one branch:
// just the identity, display name, and parsed content. Used by
// downstream pipelines that need to walk the content tree without
// pulling raw_content or other branch metadata.
type Content struct {
	// OrganizationID is the identifier of the organization that
	// owns this document.
	OrganizationID string `db:"fk_organization_id"`

	// DocumentID is the identifier of the document.
	DocumentID xid.ID `db:"fk_document_id"`

	// DocumentName is the document's display name on the branch
	// this content was loaded from.
	DocumentName string `db:"document_name"`

	// Content is the parsed ProseMirror tree for the branch.
	Content RootBlock `db:"content"`
}

// BranchSummary holds summary information for a document branch.
// It omits content and raw content and is used for listing branches.
type BranchSummary struct {
	// BranchID is the unique identifier for this branch row.
	BranchID xid.ID `json:"branchId" db:"branch_id"`

	// BranchName is the branch identifier (e.g. "main", "feature-x").
	BranchName string `json:"branchName" db:"branch_name"`

	// DocumentName is the document display name for this branch.
	DocumentName string `json:"documentName" db:"document_name"`

	// Icon is the icon associated with this branch.
	Icon string `json:"icon" db:"icon"`

	// Protected indicates whether this branch is protected from direct updates.
	Protected bool `json:"protected" db:"protected"`

	// Default indicates whether this is the default (original) branch of the document.
	Default bool `json:"default" db:"default"`

	// CreatedAt is the timestamp when this branch was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when this branch was last updated.
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// ApplyBranchUpdate returns a new Document with only the name and protection
// status updated. It does not modify content or the history.
func (d Document) ApplyBranchUpdate(name null.String, protected null.Bool, updatedBy string) Document {
	nd := d

	if name.Valid {
		nd.BranchName = name.String
	}

	if protected.Valid {
		nd.Protected = protected.Bool
	}

	nd.UpdatedAt = timeutil.Now()
	nd.LastUpdatedBy = null.StringFrom(updatedBy)

	return nd
}

// HistoryEntry returns a history entry for the current state of the document.
// Entries aggregate per branch and per time bucket: the branch is part of the
// ID so that two branches of one document edited within the same bucket keep
// separate entries instead of overwriting each other.
func (d Document) HistoryEntry() HistoryEntry {
	id := fmt.Sprintf(
		"%s-%s-%s",
		d.ID,
		d.BranchID,
		d.UpdatedAt.Truncate(_aggregationDuration).
			Format(_historyEntryTimeLayout),
	)

	return HistoryEntry{
		ID:         id,
		DocumentID: d.ID,
		Content:    d.Content,
		RawContent: d.RawContent,
		CreatedAt:  d.UpdatedAt,
	}
}

// Search converts the document's branch to search blocks for indexing.
func (d Document) Search() map[string]search.Block {
	scope := search.Scope{
		OrganizationID: d.OrganizationID,
		DocumentID:     d.ID,
		BranchID:       d.BranchID,
		BranchName:     d.BranchName,
		BranchDefault:  d.Default,
	}

	res := d.Content.Search(scope)

	// a special block carrying the document name. The map is keyed by the
	// document ID, which no content block shares.
	res[d.ID.String()] = scope.Block("docname", "document", d.DocumentName)

	return res
}

// HistoryEntry represents one entry in a document's history.
type HistoryEntry struct {
	// ID is the unique identifier for the entry.
	ID string `json:"id" db:"id"`

	// DocumentID is the identifier for the document associated
	// with this entry.
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// Content is the content of the document at the time of the entry.
	Content RootBlock `json:"content" db:"content"`

	// RawContent is the raw content of the document, typically in
	// a text format.
	RawContent []byte `json:"rawContent" db:"raw_content"`

	// CreatedAt is the timestamp when the entry was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// CreateInput is the input structure for document operations.
type CreateInput struct {
	// Name is the name of the document.
	Name string `json:"name"`

	// Icon is the icon associated with the document.
	Icon string `json:"icon"`

	// ParentID is the identifier for the parent document, if any.
	ParentID null.Value[xid.ID] `json:"parentId"`
}

// ImportInput is the input structure for importing a document.
type ImportInput struct {
	// Name is the name of the document.
	Name string `json:"name"`

	// Icon is the icon associated with the document.
	Icon string `json:"icon"`

	// ParentID is the identifier for the parent document, if any.
	ParentID null.Value[xid.ID] `json:"parentId"`

	// Content is the content of the document.
	Content RootBlock `json:"content"`
}

// UpdateInput is the update structure for document operations.
type UpdateInput struct {
	// Name is the name of the document.
	Name string `json:"name"`

	// Icon is the icon associated with the document.
	Icon string `json:"icon"`

	// Content is the content of the document.
	Content RootBlock `json:"content"`

	// Maintainers specifies the people who initiated this update, not the
	// document's maintainer set: a content persist carries whoever was
	// editing at the time, and the store hook sends none at all for a
	// system write. The set they join is add-only — anyone missing here
	// stays a maintainer — so this list must never be treated as
	// authoritative and diffed against the stored one.
	Maintainers []string `json:"maintainers"`

	// RawContent is the raw content of the document, typically in
	// a text format.
	RawContent []byte `json:"rawContent"`

	// System indicates whether the update is a system
	// change (for internal use only).
	System bool `json:"system"`

	// Branch is the target branch for the update (e.g. "main" or "feature-x").
	Branch string `json:"branch"`
}

// InitialDocumentContent returns the initial content for a new document.
func InitialDocumentContent(dataSourceID null.Value[xid.ID]) (RootBlock, error) {
	var drb RootBlock

	if err := json.Unmarshal(_gettingStartedContent, &drb); err != nil {
		return RootBlock{}, fmt.Errorf("failed to unmarshal getting started content: %w", err)
	}

	var mrb []Block

	if err := json.Unmarshal(_availableMetrics, &mrb); err != nil {
		return RootBlock{}, fmt.Errorf("failed to unmarshal available metrics content: %w", err)
	}

	return applyMetrics(drb, mrb, dataSourceID).RegenerateUIDs(), nil
}

// applyMetrics replaces every metricBlock in rb with a metricBlock from
// metricGrids (shuffled, without repetition) and sets all dataSourceId config
// attrs to dataSourceID.
func applyMetrics(rb RootBlock, metricGrids []Block, dataSourceID null.Value[xid.ID]) RootBlock {
	// without a data source the metrics have nothing to read, so they are
	// dropped rather than left pointing at an id that was never stored.
	if !dataSourceID.Valid {
		newContent := make([]Block, 0, len(rb.Content))

		for _, b := range rb.Content {
			if b.Type == BlockNodeMetricBlock {
				continue
			}

			newContent = append(newContent, b)
		}

		return RootBlock{
			Type:    rb.Type,
			Content: newContent,
		}
	}

	shuffled := make([]Block, len(metricGrids))
	copy(shuffled, metricGrids)
	//nolint:gosec // cosmetic variety in a new document's sample metrics, not a security choice
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	idx := 0
	newContent := make([]Block, len(rb.Content))

	for i, b := range rb.Content {
		newContent[i] = applyMetricsToBlock(b, shuffled, &idx, dataSourceID.V)
	}

	return RootBlock{
		Type:    rb.Type,
		Content: newContent,
	}
}

// applyMetricsToBlock recursively replaces metricBlock nodes with the next
// entry from the shuffled pool (wrapping if exhausted) and updates dataSourceId
// in all block attrs.
func applyMetricsToBlock(b Block, metricGrids []Block, idx *int, dataSourceID xid.ID) Block {
	if b.Type == BlockNodeMetricBlock && len(metricGrids) > 0 {
		chosen := metricGrids[*idx%len(metricGrids)]
		*idx++

		// Process the chosen block with nil metricGrids so its children are
		// kept as-is, but their dataSourceIds are still updated.
		return applyMetricsToBlock(chosen, nil, idx, dataSourceID)
	}

	newBlock := Block{
		Type:  b.Type,
		Text:  b.Text,
		Marks: b.Marks,
		Attrs: replaceDataSourceID(b.Attrs, dataSourceID),
	}

	if len(b.Content) > 0 {
		newBlock.Content = make([]Block, len(b.Content))

		for i, cb := range b.Content {
			newBlock.Content[i] = applyMetricsToBlock(cb, metricGrids, idx, dataSourceID)
		}
	}

	return newBlock
}

// replaceDataSourceID returns attrs with the dataSourceId field updated to dataSourceID.
// Returns the original attributes map if unchanged.
func replaceDataSourceID(attrs map[string]any, dataSourceID xid.ID) map[string]any {
	if len(attrs) == 0 {
		return attrs
	}

	if _, has := attrs["dataSourceId"]; !has {
		return attrs
	}

	newAttrs := make(map[string]any, len(attrs))
	maps.Copy(newAttrs, attrs)

	newAttrs["dataSourceId"] = dataSourceID.String()

	return newAttrs
}

// NewDocumentContent returns the initial content for a new document.
func NewDocumentContent() RootBlock {
	return RootBlock{
		Type: BlockNodeDoc,
		Content: []Block{
			{
				Type: "paragraph",
				Attrs: map[string]any{
					AttrUID: strutil.NanoID(),
				},
				Content: []Block{},
			},
		},
	}
}

// Duplicate creates a copy of the document with a new ID, duplicated content
// (with comment marks removed, nodeCommentId attributes removed, and uid
// attributes regenerated), and cleared raw content. The new document name
// includes a timestamp suffix. The first returned map pairs the source
// document's file ids with the ids the copy refers to them by, so the caller
// can copy the stored objects; the second pairs the source's block uids with
// the copy's, so the caller can carry over whatever is keyed by block.
func (d Document) Duplicate(duplicatedBy string) (Document, map[string]string, map[string]string) {
	tstamp := timeutil.Now()
	id := xid.New()

	content, files, uids := d.Content.Duplicate(d.ID, id)

	return Document{
		ID:             id,
		ParentID:       d.ParentID,
		OrganizationID: d.OrganizationID,
		BranchID:       xid.New(),
		BranchName:     DefaultBranch,
		Default:        true,
		DocumentName:   fmt.Sprintf("%s (%s)", d.DocumentName, tstamp.Format("2006 Jan. 02 15:04")),
		Icon:           d.Icon,
		Content:        content,
		RawContent:     nil,
		Protected:      false,
		CreatedAt:      tstamp,
		CreatedBy:      null.StringFrom(duplicatedBy),
		UpdatedAt:      tstamp,
		LastUpdatedBy:  null.StringFrom(duplicatedBy),
	}, files, uids
}
