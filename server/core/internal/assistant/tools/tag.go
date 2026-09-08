package tools

import (
	"fmt"
	"strings"

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
type listTags struct {
	plainSummary
	plainTraits
	plainTitle
}

// Info returns the tool's model-facing description.
func (listTags) Info() Info {
	return Info{
		Name:        NameListTags,
		Description: "List the organisation's tags in their sidebar order as [{id, name, color, hidden, documents}], where documents is [{id, name, default_branch_id}] for every document whose default branch carries the tag. Use it to find a tag by name before assigning it, and to learn the id that update_tag, delete_tag, move_tag, assign_tag and unassign_tag take as tag_id. A tag assigned to a non-default branch shows under tags on get_document for that branch but not here. hidden says whether the current user keeps the tag out of their own sidebar; it does not stop the tag being assigned.",
		Properties:  map[string]any{},
	}
}

// Execute lists every tag with the documents carrying it.
func (listTags) Execute(inp Input) (string, error) {
	var in listTagsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tree, err := inp.FetchTagTree()
	if err != nil {
		return "", fmt.Errorf("list_tags: fetch tags: %w", err)
	}

	out := tagListResult{Tags: make([]tagEntry, 0, len(tree))}

	for _, s := range tree {
		e := tagEntry{
			ID:        s.ID,
			Name:      s.TagName,
			Color:     s.Color,
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

	// Color is the tag's colour as a hex triplet.
	Color string `json:"color"`

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

	// Color is the new tag's colour as a hex triplet. Required.
	Color string `json:"color"`
}

// Validate checks the arguments are complete and the colour well formed.
func (a createTagArgs) Validate() error {
	if a.Name == "" {
		return errRequired("name")
	}

	if a.Color == "" {
		return errRequired("color")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's create input.
func (a createTagArgs) input() tag.CreateInput {
	return tag.CreateInput{TagName: a.Name, Color: a.Color}
}

// createTag creates a new tag in the organisation.
type createTag struct{}

// Info returns the tool's model-facing description.
func (createTag) Info() Info {
	return Info{
		Name:        NameCreateTag,
		Description: "Create a tag and return {tag_id}. name is the display name, which has to be unused in the organisation, and color a hex triplet with the leading hash; a malformed colour or a name already in use is refused. The new tag lands last in the sidebar order, so use move_tag to place it, and assign_tag to put it on a document branch.",
		Properties: map[string]any{
			"name":  map[string]any{"type": "string", "description": "Display name for the new tag; unique within the organisation."},
			"color": map[string]any{"type": "string", "description": "The tag's colour as a hex triplet with the leading hash, such as \"#22c55e\"."},
		},
		Required: []string{"name", "color"},
	}
}

// Traits reports a write.
func (createTag) Traits() Traits {
	return Traits{Write: true}
}

// Title announces the tag being created.
func (createTag) Title(inp DescribeInput) (string, error) {
	var in createTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Creating tag " + in.Name, nil
}

// Summary describes the tag the model wants to create.
func (createTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:    NameCreateTag,
		Summary: fmt.Sprintf("Create tag %s (%s)", in.Name, in.Color),
	}, nil
}

// Execute creates the tag and refreshes the tag tree.
func (createTag) Execute(inp Input) (string, error) {
	var in createTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	t := tag.NewTag(in.input(), inp.OrganizationID(), inp.UserID())

	if err := inp.CreateTag(t); err != nil {
		return "", fmt.Errorf("create_tag: %w", err)
	}

	inp.NotifyTagTreeChange()

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

	// Color is the new colour; empty leaves it unchanged.
	Color string `json:"color"`
}

// Validate checks the arguments name a tag and change something.
func (a updateTagArgs) Validate() error {
	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	return a.input().Validate()
}

// input returns the arguments as the domain's update input, with only
// the fields that were given set.
func (a updateTagArgs) input() tag.UpdateInput {
	var inp tag.UpdateInput

	if a.Name != "" {
		inp.TagName = null.StringFrom(a.Name)
	}

	if a.Color != "" {
		inp.Color = null.StringFrom(a.Color)
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

	if a.Color != "" {
		out = append(out, "set the colour to "+a.Color)
	}

	return out
}

// updateTag renames or recolours a tag, or both in one call.
type updateTag struct{}

// Info returns the tool's model-facing description.
func (updateTag) Info() Info {
	return Info{
		Name:        NameUpdateTag,
		Description: "Rename and/or recolour a tag; give only the fields to change, and the other keeps its value. name is the new display name, which has to be unused in the organisation, and color a hex triplet with the leading hash; a call with neither is refused. Returns {tag_id} with the fields that changed. To change which documents carry the tag use assign_tag and unassign_tag instead.",
		Properties: map[string]any{
			"tag_id": map[string]any{"type": "string", "description": "The tag id, as list_tags or a document's tags report it."},
			"name":   map[string]any{"type": "string", "description": "Optional. The new display name; omit to keep the current one."},
			"color":  map[string]any{"type": "string", "description": "Optional. The new colour as a hex triplet with the leading hash; omit to keep the current one."},
		},
		Required: []string{"tag_id"},
	}
}

// Traits reports a write.
func (updateTag) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which tag is being updated.
func (updateTag) Title(inp DescribeInput) (string, error) {
	var in updateTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch tag: %w", NameUpdateTag, err)
	}

	return "Updating tag " + tg.TagName, nil
}

// Summary lists exactly the changes the model asked for.
func (updateTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch tag: %w", NameUpdateTag, err)
	}

	changes := in.changes(tg.TagName)
	summary := strings.Join(changes, " and ")

	return ActionSummary{
		Tool:    NameUpdateTag,
		Summary: strings.ToUpper(summary[:1]) + summary[1:],
	}, nil
}

// Execute applies the change and refreshes the tag tree.
func (updateTag) Execute(inp Input) (string, error) {
	var in updateTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.UpdateTag(in.TagID, in.input()); err != nil {
		return "", fmt.Errorf("update_tag: %w", err)
	}

	inp.NotifyTagTreeChange()

	return result(updatedTagResult(in))
}

// updatedTagResult is what update_tag returns: the tag and the fields
// that changed.
type updatedTagResult struct {
	// TagID is the tag that was changed.
	TagID xid.ID `json:"tag_id"`

	// Name is the new display name, when one was set.
	Name string `json:"name,omitempty"`

	// Color is the new colour, when one was set.
	Color string `json:"color,omitempty"`
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
		Description: "Delete a tag. Every document carrying it loses it, on every branch, and the tag cannot be restored; use unassign_tag when the aim is to take it off one document rather than remove it. Returns {tag_id, deleted}.",
		Properties: map[string]any{
			"tag_id": map[string]any{"type": "string", "description": "The id of the tag to delete."},
		},
		Required: []string{"tag_id"},
	}
}

// Traits reports a destructive write, which stays outside any "approve
// all" answer.
func (deleteTag) Traits() Traits {
	return Traits{Write: true, Destructive: true}
}

// Title announces which tag is being deleted.
func (deleteTag) Title(inp DescribeInput) (string, error) {
	var in deleteTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch tag: %w", NameDeleteTag, err)
	}

	return "Deleting tag " + tg.TagName, nil
}

// Summary describes the tag the model wants to delete.
func (deleteTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch tag: %w", NameDeleteTag, err)
	}

	summary := "Delete tag " + tg.TagName

	// the delete takes the tag off every document, so a card naming
	// only the tag would have the user approve a change they were never
	// shown.
	if n := len(tg.Documents); n > 0 {
		summary += fmt.Sprintf(" and take it off the %s carrying it", pluralPages(n))
	}

	return ActionSummary{
		Tool:    NameDeleteTag,
		Summary: summary,
	}, nil
}

// Execute deletes the tag and refreshes the tag tree.
func (deleteTag) Execute(inp Input) (string, error) {
	var in deleteTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.DeleteTag(in.TagID); err != nil {
		return "", fmt.Errorf("delete_tag: %w", err)
	}

	inp.NotifyTagTreeChange()

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

// branchTagArgs is what assign_tag and unassign_tag are called with: a
// branch and a tag.
type branchTagArgs struct {
	docTarget

	// TagID names the tag.
	TagID xid.ID `json:"tag_id"`
}

// Validate checks the arguments name a branch and a tag.
func (a branchTagArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	return nil
}

// assignTag puts a tag on a document branch.
type assignTag struct{}

// Info returns the tool's model-facing description.
func (assignTag) Info() Info {
	return Info{
		Name:        NameAssignTag,
		Description: "Put a tag on one branch of a document, so the document is listed under the tag in the sidebar when the branch is its default one. document_id and branch_id name the branch the way the content tools do, and tag_id is the tag's id from list_tags. A tag the branch already carries is left as it is, and a hidden tag can still be assigned. Returns {document_id, branch_id, tag_id}.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": "The document id."},
			"branch_id":   map[string]any{"type": "string", "description": "The id of the branch the tag goes on or comes off: a document's default_branch_id from list_documents or list_tags, or any id from the branches get_document lists."},
			"tag_id":      map[string]any{"type": "string", "description": "The tag id, as list_tags or a document's tags report it."},
		},
		Required: []string{"document_id", "branch_id", "tag_id"},
	}
}

// Traits reports a write.
func (assignTag) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being tagged.
func (assignTag) Title(inp DescribeInput) (string, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameAssignTag, err)
	}

	return "Tagging " + doc.Title(), nil
}

// Summary names the tag and the document it goes on.
func (assignTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameAssignTag, err)
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch tag: %w", NameAssignTag, err)
	}

	return ActionSummary{
		Tool:         NameAssignTag,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Put tag %s on %s", tg.TagName, doc.Title()),
	}, nil
}

// Execute assigns the tag and refreshes the tag tree.
func (assignTag) Execute(inp Input) (string, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.AssignTag(in.DocumentID, in.BranchID, in.TagID); err != nil {
		return "", fmt.Errorf("assign_tag: %w", err)
	}

	inp.NotifyTagTreeChange()
	inp.NotifyBranchTagsChange(in.DocumentID, in.BranchID)

	return result(branchTagResult{DocumentID: in.DocumentID, BranchID: in.BranchID, TagID: in.TagID})
}

// unassignTag takes a tag off a document branch.
type unassignTag struct{}

// Info returns the tool's model-facing description.
func (unassignTag) Info() Info {
	return Info{
		Name:        NameUnassignTag,
		Description: "Take a tag off one branch of a document; the tag itself stays for other documents, so use delete_tag to remove it everywhere. document_id and branch_id name the branch the way the content tools do, and tag_id is the tag's id from the document's tags on get_document or from list_tags. A tag the branch does not carry is nothing to do, and the call still succeeds. Returns {document_id, branch_id, tag_id}.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": "The document id."},
			"branch_id":   map[string]any{"type": "string", "description": "The id of the branch the tag goes on or comes off: a document's default_branch_id from list_documents or list_tags, or any id from the branches get_document lists."},
			"tag_id":      map[string]any{"type": "string", "description": "The tag id, as list_tags or a document's tags report it."},
		},
		Required: []string{"document_id", "branch_id", "tag_id"},
	}
}

// Traits reports a write.
func (unassignTag) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being untagged.
func (unassignTag) Title(inp DescribeInput) (string, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameUnassignTag, err)
	}

	return "Untagging " + doc.Title(), nil
}

// Summary names the tag and the document it comes off.
func (unassignTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameUnassignTag, err)
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch tag: %w", NameUnassignTag, err)
	}

	return ActionSummary{
		Tool:         NameUnassignTag,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Take tag %s off %s", tg.TagName, doc.Title()),
	}, nil
}

// Execute unassigns the tag and refreshes the tag tree.
func (unassignTag) Execute(inp Input) (string, error) {
	var in branchTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.UnassignTag(in.DocumentID, in.BranchID, in.TagID); err != nil {
		return "", fmt.Errorf("unassign_tag: %w", err)
	}

	inp.NotifyTagTreeChange()
	inp.NotifyBranchTagsChange(in.DocumentID, in.BranchID)

	return result(branchTagResult{DocumentID: in.DocumentID, BranchID: in.BranchID, TagID: in.TagID})
}

// branchTagResult is what assign_tag and unassign_tag return.
type branchTagResult struct {
	// DocumentID is the document whose branch changed.
	DocumentID xid.ID `json:"document_id"`

	// BranchID is the branch the tag went on or came off.
	BranchID xid.ID `json:"branch_id"`

	// TagID is the tag.
	TagID xid.ID `json:"tag_id"`
}

// moveTagArgs is what move_tag is called with.
type moveTagArgs struct {
	// TagID names the tag being moved.
	TagID xid.ID `json:"tag_id"`

	// SortIndex is the 0-based position the tag should end up at.
	// Required, since the first position is a valid answer.
	SortIndex null.Value[int] `json:"sort_index"`
}

// Validate checks the arguments are complete.
func (a moveTagArgs) Validate() error {
	if a.TagID.IsNil() {
		return errRequired("tag_id")
	}

	if !a.SortIndex.Valid {
		return errRequired("sort_index")
	}

	return nil
}

// moveTag changes a tag's position in the sidebar order.
type moveTag struct{}

// Info returns the tool's model-facing description.
func (moveTag) Info() Info {
	return Info{
		Name:        NameMoveTag,
		Description: "Move a tag to a position in the sidebar order, which is the order list_tags returns. sort_index is the 0-based position the tag should end up at, so 0 puts it first, and a position past the last tag is refused; the other tags keep their relative order. Returns {tag_id, sort_index}.",
		Properties: map[string]any{
			"tag_id":     map[string]any{"type": "string", "description": "The tag id, as list_tags or a document's tags report it."},
			"sort_index": map[string]any{"type": "integer", "description": "The 0-based position the tag should end up at among the organisation's tags."},
		},
		Required: []string{"tag_id", "sort_index"},
	}
}

// Traits reports a write.
func (moveTag) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which tag is being moved.
func (moveTag) Title(inp DescribeInput) (string, error) {
	var in moveTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch tag: %w", NameMoveTag, err)
	}

	return "Moving tag " + tg.TagName, nil
}

// Summary names the tag and where it goes.
func (moveTag) Summary(inp DescribeInput) (ActionSummary, error) {
	var in moveTagArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	tg, err := inp.FetchTag(in.TagID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch tag: %w", NameMoveTag, err)
	}

	return ActionSummary{
		Tool:    NameMoveTag,
		Summary: fmt.Sprintf("Move tag %s to position %d", tg.TagName, in.SortIndex.V+1),
	}, nil
}

// Execute moves the tag and refreshes the tag tree.
func (moveTag) Execute(inp Input) (string, error) {
	var in moveTagArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.MoveTag(in.TagID, in.SortIndex.V); err != nil {
		return "", fmt.Errorf("move_tag: %w", err)
	}

	inp.NotifyTagTreeChange()

	return result(movedTagResult{TagID: in.TagID, SortIndex: in.SortIndex.V})
}

// movedTagResult is what move_tag returns.
type movedTagResult struct {
	// TagID is the tag that was moved.
	TagID xid.ID `json:"tag_id"`

	// SortIndex is the 0-based position it now has.
	SortIndex int `json:"sort_index"`
}
