package tools

import (
	"errors"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/rs/xid"
)

// listTags returns the organisation's tags with the documents carrying
// them.
type listTags struct{}

// Info returns the tool's model-facing description.
func (listTags) Info() Info {
	return Info{
		Name:        NameListTags,
		Traits:      Traits{Group: GroupTags},
		Description: "List the organisation's tags in sidebar order as [{id, name, color, documents}]. documents are the documents whose default branch carries the tag. A tag on another branch shows only in get_document for that branch.",
		Properties:  map[string]any{},
	}
}

// Validate accepts every payload: nothing is required.
func (listTags) Validate() error {
	return nil
}

// Exec lists every tag with the documents carrying it.
func (listTags) Exec(inp Input) (any, error) {
	tree, err := inp.FetchTagTree()
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	out := listTagsResult{Tags: make([]tagEntry, 0, len(tree))}

	for _, s := range tree {
		e := tagEntry{
			ID:        s.ID,
			Name:      s.TagName,
			ColorName: s.Color,
			Documents: make([]tagDocument, 0, len(s.Documents)),
		}

		// the tree lists each assigned document with its own subtree;
		// the subtree is sidebar context, not an assignment.
		for _, d := range s.Documents {
			e.Documents = append(e.Documents, tagDocument{
				ID:              d.ID,
				Name:            d.DocumentName,
				DefaultBranchID: d.DefaultBranchID,
			})
		}

		out.Tags = append(out.Tags, e)
	}

	return out, nil
}

// listTagsResult is what list_tags returns.
type listTagsResult struct {
	// Tags is every tag in the organisation, in sidebar order.
	Tags []tagEntry `json:"tags"`
}

// tagEntry describes one tag and the documents carrying it.
type tagEntry struct {
	// ID is the tag id, which is what a tool's tag_id takes.
	ID xid.ID `json:"id"`

	// Name is the tag's display name.
	Name string `json:"name"`

	// ColorName is the palette name of the tag's colour.
	ColorName string `json:"color"`

	// Documents is every document whose default branch carries the tag.
	Documents []tagDocument `json:"documents"`
}

// tagDocument names one document carrying a tag.
type tagDocument struct {
	// ID is the document id.
	ID xid.ID `json:"id"`

	// Name is the document's display name.
	Name string `json:"name"`

	// DefaultBranchID is the branch the tag is on.
	DefaultBranchID xid.ID `json:"default_branch_id"`
}

// createTag creates a new tag in the organisation.
type createTag struct {
	// Name is the new tag's display name. Required.
	Name string `json:"name"`

	// ColorName is the new tag's colour as a palette name. Required.
	ColorName string `json:"color"`
}

// Info returns the tool's model-facing description.
func (createTag) Info() Info {
	return Info{
		Name:        NameCreateTag,
		Traits:      Traits{Group: GroupTags, Write: true},
		Description: "Create a tag and return {tag_id}. The tag goes last in the sidebar order. update_tag moves it and set_tag_assignment puts it on a document. The name must be unused in the organisation.",
		Properties: map[string]any{
			"name":  map[string]any{"type": "string", "description": "The display name."},
			"color": map[string]any{"type": "string", "enum": tag.ColorNames(), "description": "The colour."},
		},
		Required: []string{"name", "color"},
	}
}

// Validate checks the arguments are complete and the colour in the palette.
func (ct createTag) Validate() error {
	if ct.Name == "" {
		return errRequired("name")
	}

	if ct.ColorName == "" {
		return errRequired("color")
	}

	return ct.input().Validate()
}

// Summary describes the tag the model wants to create.
func (ct createTag) Summary(_ DescribeInput) (ActionSummary, error) {
	return ActionSummary{
		Tool:    NameCreateTag,
		Summary: fmt.Sprintf("Create tag %s (%s)", ct.Name, ct.ColorName),
	}, nil
}

// Exec creates the tag.
func (ct createTag) Exec(inp Input) (any, error) {
	t, err := tag.NewTag(ct.input(), inp.OrganizationID(), inp.UserID())
	if err != nil {
		return nil, err
	}

	if err = inp.CreateTag(t); err != nil {
		return nil, err
	}

	return createTagResult{TagID: t.ID}, nil
}

// input returns the arguments as the domain's create input.
func (ct createTag) input() tag.CreateInput {
	return tag.CreateInput{TagName: ct.Name, Color: ct.ColorName}
}

// createTagResult is what create_tag returns.
type createTagResult struct {
	// TagID addresses the new tag in every later call.
	TagID xid.ID `json:"tag_id"`
}

// updateTag renames, recolours or moves a tag, any of them in one call.
type updateTag struct {
	// TagID names the tag being changed.
	TagID xid.ID `json:"tag_id"`

	// Name is the new display name; empty leaves it unchanged.
	Name string `json:"name"`

	// ColorName is the new colour as a palette name; empty leaves it
	// unchanged.
	ColorName string `json:"color"`

	// SortIndex is the 0-based position the tag moves to; absent leaves
	// it where it is.
	SortIndex null.Value[int] `json:"sort_index"`
}

// Info returns the tool's model-facing description.
func (updateTag) Info() Info {
	return Info{
		Name:        NameUpdateTag,
		Traits:      Traits{Group: GroupTags, Write: true, Idempotent: true},
		Description: "Rename, recolour or move a tag in the sidebar order, giving only what changes. Returns {tag_id} with what changed. A new name must be unused, and a call that changes nothing is refused. set_tag_assignment changes which documents carry it.",
		Properties: map[string]any{
			"tag_id":     map[string]any{"type": "string", "description": _tagIDDescription},
			"name":       map[string]any{"type": "string", "description": "The new display name."},
			"color":      map[string]any{"type": "string", "enum": tag.ColorNames(), "description": "The new colour."},
			"sort_index": map[string]any{"type": "integer", "minimum": 0, "description": "The 0-based position in list_tags order."},
		},
		Required: []string{"tag_id"},
	}
}

// Validate checks the arguments name a tag and change something.
func (ut updateTag) Validate() error {
	if ut.TagID.IsNil() {
		return errRequired("tag_id")
	}

	if !ut.renames() {
		if !ut.SortIndex.Valid {
			return errors.New("give at least one of name, color or sort_index")
		}

		return nil
	}

	return ut.input().Validate()
}

// Summary lists exactly the changes the model asked for.
func (ut updateTag) Summary(inp DescribeInput) (ActionSummary, error) {
	tg, err := inp.FetchTag(ut.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching tag: %w", err)
	}

	return ActionSummary{
		Tool:    NameUpdateTag,
		Summary: joinClauses(ut.changes(tg.TagName)),
	}, nil
}

// Exec applies the change.
func (ut updateTag) Exec(inp Input) (any, error) {
	// the rename and the move are two writes. The move is tried on the
	// current order first, so a position it would refuse fails the call
	// before the rename is saved.
	if ut.renames() && ut.SortIndex.Valid {
		tree, err := inp.FetchTagTree()
		if err != nil {
			return nil, fmt.Errorf("fetching tags: %w", err)
		}

		if _, err := tree.Swap(ut.TagID, ut.SortIndex.V); err != nil {
			return nil, err
		}
	}

	if ut.renames() {
		if err := inp.UpdateTag(ut.TagID, ut.input()); err != nil {
			return nil, err
		}
	}

	if ut.SortIndex.Valid {
		if err := inp.MoveTag(ut.TagID, ut.SortIndex.V); err != nil {
			return nil, err
		}
	}

	out := updateTagResult{TagID: ut.TagID, Name: ut.Name, ColorName: ut.ColorName}
	if ut.SortIndex.Valid {
		out.SortIndex = &ut.SortIndex.V
	}

	return out, nil
}

// renames reports whether the call changes the name or the colour, which
// is the tag's own update rather than a move.
func (ut updateTag) renames() bool {
	return ut.Name != "" || ut.ColorName != ""
}

// input returns the arguments as the domain's update input, with only
// the fields that were given set.
func (ut updateTag) input() tag.UpdateInput {
	var inp tag.UpdateInput

	if ut.Name != "" {
		inp.TagName = null.StringFrom(ut.Name)
	}

	if ut.ColorName != "" {
		inp.Color = null.StringFrom(ut.ColorName)
	}

	return inp
}

// changes lists the requested changes in the words the confirm card
// uses, in a fixed order so the same request always reads the same.
func (ut updateTag) changes(currentName string) []string {
	var out []string

	if ut.Name != "" {
		out = append(out, fmt.Sprintf("rename %s to %q", currentName, ut.Name))
	}

	if ut.ColorName != "" {
		out = append(out, "set the colour to "+ut.ColorName)
	}

	if ut.SortIndex.Valid {
		out = append(out, fmt.Sprintf("move it to position %d", ut.SortIndex.V+1))
	}

	return out
}

// updateTagResult is what update_tag returns: the tag and the fields
// that changed.
type updateTagResult struct {
	// TagID is the tag that was changed.
	TagID xid.ID `json:"tag_id"`

	// Name is the new display name, when one was set.
	Name string `json:"name,omitempty"`

	// ColorName is the new colour's palette name, when one was set.
	ColorName string `json:"color,omitempty"`

	// SortIndex is the tag's new 0-based position, when it was moved.
	SortIndex *int `json:"sort_index,omitempty"`
}

// deleteTag removes a tag from the organisation.
type deleteTag struct {
	// TagID names the tag being deleted.
	TagID xid.ID `json:"tag_id"`
}

// Info returns the tool's model-facing description.
func (deleteTag) Info() Info {
	return Info{
		Name:        NameDeleteTag,
		Traits:      Traits{Group: GroupTags, Write: true, Destructive: true},
		Description: "Delete a tag from every document and branch. Returns {tag_id, deleted}. It cannot be restored. set_tag_assignment takes it off one document instead.",
		Properties: map[string]any{
			"tag_id": map[string]any{"type": "string", "description": _tagIDDescription},
		},
		Required: []string{"tag_id"},
	}
}

// Validate checks the arguments are complete.
func (dt deleteTag) Validate() error {
	if dt.TagID.IsNil() {
		return errRequired("tag_id")
	}

	return nil
}

// Summary describes the tag the model wants to delete.
func (dt deleteTag) Summary(inp DescribeInput) (ActionSummary, error) {
	tg, err := inp.FetchTag(dt.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching tag: %w", err)
	}

	summary := "Delete tag " + tg.TagName

	// the delete takes the tag off every document, so a card naming
	// only the tag would have the user approve a change they were never
	// shown.
	if n := len(tg.Documents); n > 0 {
		summary += fmt.Sprintf(" and take it off the %s carrying it", countPhrase(n, "page"))
	}

	return ActionSummary{
		Tool:    NameDeleteTag,
		Summary: summary,
	}, nil
}

// Exec deletes the tag.
func (dt deleteTag) Exec(inp Input) (any, error) {
	if err := inp.DeleteTag(dt.TagID); err != nil {
		return nil, err
	}

	return deleteTagResult{TagID: dt.TagID, Deleted: true}, nil
}

// deleteTagResult is what delete_tag returns.
type deleteTagResult struct {
	// TagID is the tag that was removed.
	TagID xid.ID `json:"tag_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// setTagAssignment puts a tag on a document branch or takes it off.
type setTagAssignment struct {
	docTarget

	// TagID names the tag.
	TagID xid.ID `json:"tag_id"`

	// Assigned says whether the branch carries the tag after the call.
	// Required, since false is an answer.
	Assigned null.Value[bool] `json:"assigned"`
}

// Info returns the tool's model-facing description.
func (setTagAssignment) Info() Info {
	return Info{
		Name:        NameSetTagAssignment,
		Traits:      Traits{Group: GroupTags, Write: true, Idempotent: true},
		Description: "Put a tag on one branch of a document, or take it off. Returns {document_id, branch_id, tag_id, assigned}. The sidebar lists the document under the tag while its default branch carries it. Setting what is already the case changes nothing. delete_tag removes a tag everywhere.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"tag_id":      map[string]any{"type": "string", "description": _tagIDDescription},
			"assigned":    map[string]any{"type": "boolean", "description": "true puts the tag on the branch, false takes it off."},
		},
		Required: []string{"document_id", "branch_id", "tag_id", "assigned"},
	}
}

// Validate checks the arguments name a branch and a tag and say which
// way the assignment goes.
func (sta setTagAssignment) Validate() error {
	if err := sta.docTarget.Validate(); err != nil {
		return err
	}

	if sta.TagID.IsNil() {
		return errRequired("tag_id")
	}

	if !sta.Assigned.Valid {
		return errRequired("assigned")
	}

	return nil
}

// Summary names the tag and the document it goes on or comes off.
func (sta setTagAssignment) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(sta.DocumentID, sta.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	tg, err := inp.FetchTag(sta.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching tag: %w", err)
	}

	summary := fmt.Sprintf("Take tag %s off %s", tg.TagName, doc.Title())
	if sta.Assigned.V {
		summary = fmt.Sprintf("Put tag %s on %s", tg.TagName, doc.Title())
	}

	return ActionSummary{
		Tool:         NameSetTagAssignment,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Exec assigns or unassigns the tag.
func (sta setTagAssignment) Exec(inp Input) (any, error) {
	if sta.Assigned.V {
		if err := inp.AssignTag(sta.DocumentID, sta.BranchID, sta.TagID); err != nil {
			return nil, err
		}
	} else if err := inp.UnassignTag(sta.DocumentID, sta.BranchID, sta.TagID); err != nil {
		return nil, err
	}

	return setTagAssignmentResult{
		DocumentID: sta.DocumentID,
		BranchID:   sta.BranchID,
		TagID:      sta.TagID,
		Assigned:   sta.Assigned.V,
	}, nil
}

// setTagAssignmentResult is what set_tag_assignment returns.
type setTagAssignmentResult struct {
	// DocumentID is the document whose branch changed.
	DocumentID xid.ID `json:"document_id"`

	// BranchID is the branch the tag went on or came off.
	BranchID xid.ID `json:"branch_id"`

	// TagID is the tag.
	TagID xid.ID `json:"tag_id"`

	// Assigned is whether the branch now carries the tag.
	Assigned bool `json:"assigned"`
}

// TagInput is what the tag tools reach on top of DescribeInput.
type TagInput interface {
	// FetchTagTree should return the organisation's tags.
	FetchTagTree() (tag.Summaries, error)

	// FetchBranchTags should return the tags the branch carries.
	FetchBranchTags(documentID, branchID xid.ID) ([]tag.Tag, error)

	// CreateTag should store the new tag.
	CreateTag(t tag.Tag) error

	// UpdateTag should apply the update to the tag.
	UpdateTag(tagID xid.ID, inp tag.UpdateInput) error

	// DeleteTag should delete the tag.
	DeleteTag(tagID xid.ID) error

	// AssignTag should put the tag on the branch.
	AssignTag(documentID, branchID, tagID xid.ID) error

	// UnassignTag should take the tag off the branch.
	UnassignTag(documentID, branchID, tagID xid.ID) error

	// MoveTag should move the tag to the sort index.
	MoveTag(tagID xid.ID, sortIndex int) error
}
