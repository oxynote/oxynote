package tools

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/rs/xid"
)

// listTagsArgs is what list_tags is called with.
type listTagsArgs struct{}

// Validate accepts every payload: nothing is required.
func (listTagsArgs) Validate() error {
	return nil
}

// listTags returns the organisation's tags with the documents carrying
// them.
type listTags struct{}

// Info returns the tool's model-facing description.
func (listTags) Info() Info {
	return Info{
		Name:        NameListTags,
		Description: "List the organisation's tags in sidebar order as [{id, name, color, hidden, documents}]. color is a palette name, or unknown for a colour outside the palette that update_tag can fix. documents are the documents whose default branch carries the tag; a tag on another branch shows only on get_document for that branch. hidden is whether the current user keeps the tag out of their sidebar, and does not stop it being assigned.",
		Properties:  map[string]any{},
	}
}

// Execute lists every tag with the documents carrying it.
func (listTags) Execute(inp *input) (string, error) {
	var in listTagsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tree, err := inp.FetchTagTree()
	if err != nil {
		return "", fmt.Errorf("fetch tags: %w", err)
	}

	out := tagListResult{Tags: make([]tagEntry, 0, len(tree))}

	for _, s := range tree {
		e := tagEntry{
			ID:        s.ID,
			Name:      s.TagName,
			ColorName: cmp.Or(tag.ColorName(s.Color), "unknown"),
			Hidden:    s.Hidden,
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

	return result(out)
}

// tagListResult is what list_tags returns.
type tagListResult struct {
	// Tags is every tag in the organisation, in sidebar order.
	Tags []tagEntry `json:"tags"`
}

// tagEntry describes one tag and the documents carrying it.
type tagEntry struct {
	// ID is the tag id, which is what a tool's tag_id takes.
	ID xid.ID `json:"id"`

	// Name is the tag's display name.
	Name string `json:"name"`

	// ColorName is the palette name of the tag's colour, or "unknown" for
	// one stored outside the palette: before the palette existed, or by
	// another browser's rounding of a shade.
	ColorName string `json:"color"`

	// Hidden indicates the current user keeps the tag out of their own
	// sidebar.
	Hidden bool `json:"hidden"`

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

// createTagArgs is what create_tag is called with.
type createTagArgs struct {
	// Name is the new tag's display name. Required.
	Name string `json:"name"`

	// ColorName is the new tag's colour as a palette name. Required.
	ColorName string `json:"color"`
}

// Validate checks the arguments are complete and the colour in the palette.
func (a createTagArgs) Validate() error {
	if a.Name == "" {
		return errRequired("name")
	}

	if a.ColorName == "" {
		return errRequired("color")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's create input. A colour
// outside the palette maps to no colour, which the input's Validate
// refuses.
func (a createTagArgs) input() tag.CreateInput {
	return tag.CreateInput{TagName: a.Name, Color: tag.ColorHex(a.ColorName)}
}

// createTag creates a new tag in the organisation.
type createTag struct{}

// Info returns the tool's model-facing description.
func (createTag) Info() Info {
	return Info{
		Name:        NameCreateTag,
		Traits:      Traits{Write: true},
		Description: "Create a tag and return {tag_id}. The name has to be unused in the organisation. The tag lands last in the sidebar order; update_tag moves it and set_tag_assignment puts it on a document.",
		Properties: map[string]any{
			"name":  map[string]any{"type": "string", "description": "The display name."},
			"color": map[string]any{"type": "string", "enum": tag.ColorNames(), "description": "The colour."},
		},
		Required: []string{"name", "color"},
	}
}

// Summary describes the tag the model wants to create.
func (createTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:    NameCreateTag,
		Summary: fmt.Sprintf("Create tag %s (%s)", in.Name, in.ColorName),
	}, nil
}

// Execute creates the tag and refreshes the tag tree.
func (createTag) Execute(inp *input) (string, error) {
	var in createTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	t := tag.NewTag(in.input(), inp.orgID, inp.userID)

	if err := inp.CreateTag(t); err != nil {
		return "", err
	}

	return result(createdTagResult{TagID: t.ID})
}

// createdTagResult is what create_tag returns.
type createdTagResult struct {
	// TagID addresses the new tag in every later call.
	TagID xid.ID `json:"tag_id"`
}

// updateTagArgs is what update_tag is called with.
type updateTagArgs struct {
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

// Validate checks the arguments name a tag and change something.
func (a updateTagArgs) Validate() error {
	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	if !a.renames() {
		if !a.SortIndex.Valid {
			return errors.New("give at least one of name, color or sort_index")
		}

		return nil
	}

	return a.input().Validate()
}

// renames reports whether the call changes the name or the colour, which
// is the tag's own update rather than a move.
func (a updateTagArgs) renames() bool {
	return a.Name != "" || a.ColorName != ""
}

// input returns the arguments as the domain's update input, with only
// the fields that were given set. A colour outside the palette maps to
// no colour, which the input's Validate refuses.
func (a updateTagArgs) input() tag.UpdateInput {
	var inp tag.UpdateInput

	if a.Name != "" {
		inp.TagName = null.StringFrom(a.Name)
	}

	if a.ColorName != "" {
		inp.Color = null.StringFrom(tag.ColorHex(a.ColorName))
	}

	return inp
}

// changes lists the requested changes in the words the confirm card
// uses, in a fixed order so the same request always reads the same.
func (a updateTagArgs) changes(currentName string) []string {
	var out []string

	if a.Name != "" {
		out = append(out, fmt.Sprintf("rename %s to %q", currentName, a.Name))
	}

	if a.ColorName != "" {
		out = append(out, "set the colour to "+a.ColorName)
	}

	if a.SortIndex.Valid {
		out = append(out, fmt.Sprintf("move it to position %d", a.SortIndex.V+1))
	}

	return out
}

// updateTag renames, recolours or moves a tag, any of them in one call.
type updateTag struct{}

// Info returns the tool's model-facing description.
func (updateTag) Info() Info {
	return Info{
		Name:        NameUpdateTag,
		Traits:      Traits{Write: true},
		Description: "Rename, recolour or move a tag in the sidebar order; give only what changes, and a call changing nothing is refused. A new name has to be unused in the organisation. Returns {tag_id} with what changed. set_tag_assignment changes which documents carry it.",
		Properties: map[string]any{
			"tag_id":     map[string]any{"type": "string", "description": _tagIDDescription},
			"name":       map[string]any{"type": "string", "description": "Optional. The new display name."},
			"color":      map[string]any{"type": "string", "enum": tag.ColorNames(), "description": "Optional. The new colour."},
			"sort_index": map[string]any{"type": "integer", "description": "Optional. The 0-based position in list_tags order; 0 is first."},
		},
		Required: []string{"tag_id"},
	}
}

// Summary lists exactly the changes the model asked for.
func (updateTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch tag: %w", err)
	}

	return ActionSummary{
		Tool:    NameUpdateTag,
		Summary: sentence(in.changes(tg.TagName)),
	}, nil
}

// Execute applies the change and refreshes the tag tree.
func (updateTag) Execute(inp *input) (string, error) {
	var in updateTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// the rename and the move are two writes. The move is tried on the
	// current order first, so a position it would refuse fails the call
	// before the rename is saved.
	if in.renames() && in.SortIndex.Valid {
		tree, err := inp.FetchTagTree()
		if err != nil {
			return "", fmt.Errorf("fetching tags: %w", err)
		}

		if _, err := tree.Swap(in.TagID, in.SortIndex.V); err != nil {
			return "", err
		}
	}

	if in.renames() {
		if err := inp.UpdateTag(in.TagID, in.input()); err != nil {
			return "", err
		}
	}

	if in.SortIndex.Valid {
		if err := inp.MoveTag(in.TagID, in.SortIndex.V); err != nil {
			return "", err
		}
	}

	out := updatedTagResult{TagID: in.TagID, Name: in.Name, ColorName: in.ColorName}
	if in.SortIndex.Valid {
		out.SortIndex = &in.SortIndex.V
	}

	return result(out)
}

// updatedTagResult is what update_tag returns: the tag and the fields
// that changed.
type updatedTagResult struct {
	// TagID is the tag that was changed.
	TagID xid.ID `json:"tag_id"`

	// Name is the new display name, when one was set.
	Name string `json:"name,omitempty"`

	// ColorName is the new colour's palette name, when one was set.
	ColorName string `json:"color,omitempty"`

	// SortIndex is the tag's new 0-based position, when it was moved.
	SortIndex *int `json:"sort_index,omitempty"`
}

// deleteTagArgs is what delete_tag is called with.
type deleteTagArgs struct {
	// TagID names the tag being deleted.
	TagID xid.ID `json:"tag_id"`
}

// Validate checks the arguments are complete.
func (a deleteTagArgs) Validate() error {
	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	return nil
}

// deleteTag removes a tag from the organisation.
type deleteTag struct{}

// Info returns the tool's model-facing description.
func (deleteTag) Info() Info {
	return Info{
		Name:        NameDeleteTag,
		Traits:      Traits{Write: true, Destructive: true},
		Description: "Delete a tag. Every document loses it, on every branch, and it cannot be restored; set_tag_assignment takes it off one document instead. Returns {tag_id, deleted}.",
		Properties: map[string]any{
			"tag_id": map[string]any{"type": "string", "description": _tagIDDescription},
		},
		Required: []string{"tag_id"},
	}
}

// Summary describes the tag the model wants to delete.
func (deleteTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch tag: %w", err)
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

// Execute deletes the tag and refreshes the tag tree.
func (deleteTag) Execute(inp *input) (string, error) {
	var in deleteTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.DeleteTag(in.TagID); err != nil {
		return "", err
	}

	return result(deletedTagResult{TagID: in.TagID, Deleted: true})
}

// deletedTagResult is what delete_tag returns.
type deletedTagResult struct {
	// TagID is the tag that was removed.
	TagID xid.ID `json:"tag_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// setTagAssignmentArgs is what set_tag_assignment is called with: a
// branch, a tag, and whether the branch carries it.
type setTagAssignmentArgs struct {
	docTarget

	// TagID names the tag.
	TagID xid.ID `json:"tag_id"`

	// Assigned says whether the branch carries the tag after the call.
	// Required, since false is an answer.
	Assigned null.Value[bool] `json:"assigned"`
}

// Validate checks the arguments name a branch and a tag and say which
// way the assignment goes.
func (a setTagAssignmentArgs) Validate() error {
	if err := a.docTarget.Validate(); err != nil {
		return err
	}

	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	if !a.Assigned.Valid {
		return errRequired("assigned")
	}

	return nil
}

// setTagAssignment puts a tag on a document branch or takes it off.
type setTagAssignment struct{}

// Info returns the tool's model-facing description.
func (setTagAssignment) Info() Info {
	return Info{
		Name:        NameSetTagAssignment,
		Traits:      Traits{Write: true},
		Description: "Put a tag on one branch of a document, or take it off. The document is listed under the tag in the sidebar while its default branch carries it. Setting what is already the case succeeds and changes nothing; delete_tag removes a tag everywhere. Returns {document_id, branch_id, tag_id, assigned}.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"tag_id":      map[string]any{"type": "string", "description": _tagIDDescription},
			"assigned":    map[string]any{"type": "boolean", "description": "true puts the tag on the branch, false takes it off."},
		},
		Required: []string{"document_id", "branch_id", "tag_id", "assigned"},
	}
}

// Summary names the tag and the document it goes on or comes off.
func (setTagAssignment) Summary(inp DescribeInput) (ActionSummary, error) {
	var in setTagAssignmentArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch tag: %w", err)
	}

	summary := fmt.Sprintf("Take tag %s off %s", tg.TagName, doc.Title())
	if in.Assigned.V {
		summary = fmt.Sprintf("Put tag %s on %s", tg.TagName, doc.Title())
	}

	return ActionSummary{
		Tool:         NameSetTagAssignment,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute assigns or unassigns the tag and refreshes the tag tree.
func (setTagAssignment) Execute(inp *input) (string, error) {
	var in setTagAssignmentArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Assigned.V {
		if err := inp.AssignTag(in.DocumentID, in.BranchID, in.TagID); err != nil {
			return "", err
		}
	} else if err := inp.UnassignTag(in.DocumentID, in.BranchID, in.TagID); err != nil {
		return "", err
	}

	return result(tagAssignmentResult{
		DocumentID: in.DocumentID,
		BranchID:   in.BranchID,
		TagID:      in.TagID,
		Assigned:   in.Assigned.V,
	})
}

// tagAssignmentResult is what set_tag_assignment returns.
type tagAssignmentResult struct {
	// DocumentID is the document whose branch changed.
	DocumentID xid.ID `json:"document_id"`

	// BranchID is the branch the tag went on or came off.
	BranchID xid.ID `json:"branch_id"`

	// TagID is the tag.
	TagID xid.ID `json:"tag_id"`

	// Assigned is whether the branch now carries the tag.
	Assigned bool `json:"assigned"`
}
