package tools

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/rs/xid"
)

// listHooks returns every hook on one branch of a document.
type listHooks struct {
	docTarget
}

// Info returns the tool's model-facing description.
func (listHooks) Info() Info {
	return Info{
		Name:        NameListHooks,
		Traits:      Traits{Group: GroupHooks, Role: org.RoleViewer},
		Description: "List the freshness hooks on one branch of a document as [{id, type, block_uid, settings, triggered, state, status, created_at, updated_at}]. A hook watches something outside the document. It triggers when that thing changes or a date passes. block_uid is the block it is anchored to, or null for the whole document.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
		},
		Required: []string{"document_id", "branch_id"},
	}
}

// Exec lists the branch's hooks.
func (lh listHooks) Exec(inp Input) (any, error) {
	hooks, err := inp.FetchHooks(lh.DocumentID, lh.BranchID)
	if err != nil {
		return nil, err
	}

	out := listHooksResult{Hooks: make([]hookRow, 0, len(hooks))}

	for i := range hooks {
		out.Hooks = append(out.Hooks, newHookRow(&hooks[i]))
	}

	return out, nil
}

// listHooksResult is what list_hooks returns.
type listHooksResult struct {
	// Hooks is every hook on the branch.
	Hooks []hookRow `json:"hooks"`
}

// hookRow describes one hook to the model.
type hookRow struct {
	// ID is the hook id, which is what a tool's hook_id takes.
	ID xid.ID `json:"id"`

	// Type is the hook type.
	Type hook.Type `json:"type"`

	// BlockUID is the block the hook is anchored to, or null for a hook
	// on the document as a whole.
	BlockUID null.String `json:"block_uid"`

	// Settings is the hook's configuration, in its type's own shape.
	Settings json.RawMessage `json:"settings"`

	// Triggered indicates that what the hook watches changed or came due
	// since its last reset.
	Triggered bool `json:"triggered"`

	// State is what the hook has recorded so far, in its type's own
	// shape, or null for a hook not set up yet.
	State json.RawMessage `json:"state"`

	// Status tells whether the hook could check its target on its last
	// run. The triggered flag and state are left as they were while it is
	// not active.
	Status processor.Status `json:"status"`

	// CreatedAt is when the hook was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the hook's settings last changed, or null.
	UpdatedAt null.Time `json:"updated_at"`
}

// newHookRow converts a hook into the model's shape.
func newHookRow(hk *hook.Hook) hookRow {
	return hookRow{
		ID:        hk.ID,
		Type:      hk.Type,
		BlockUID:  hk.BlockID,
		Settings:  json.RawMessage(hk.Settings),
		Triggered: hk.Triggered,
		State:     json.RawMessage(hk.State.V),
		Status:    hk.Status,
		CreatedAt: hk.CreatedAt,
		UpdatedAt: hk.UpdatedAt,
	}
}

// createHook adds a hook to one branch of a document.
type createHook struct {
	docTarget

	// BlockUID anchors the hook to a block; empty puts it on the
	// document as a whole.
	BlockUID string `json:"block_uid"`

	// Settings is the hook's type and its settings in the shape that
	// type takes.
	Settings json.RawMessage `json:"settings"`
}

// Info returns the tool's model-facing description.
func (createHook) Info() Info {
	return Info{
		Name:        NameCreateHook,
		Traits:      Traits{Group: GroupHooks, Write: true, Role: org.RoleEditor},
		Description: "Add a freshness hook, such as a review reminder, to one branch of a document. Returns the hook as list_hooks does. A new hook starts untriggered. github-tracking and url-watcher need the deployment's GitHub App and changedetection.io integrations, so they are refused without them.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of a block on the branch to anchor the hook to. Without it the hook covers the whole document."},
			"settings":    hookSettingsSchema(),
		},
		Required: []string{"document_id", "branch_id", "settings"},
	}
}

// Validate checks the arguments name a branch and carry settings naming
// a hook type, with exactly that type's fields.
func (ch createHook) Validate() error {
	if err := ch.docTarget.Validate(); err != nil {
		return err
	}

	_, _, err := decodeHookSettings(ch.Settings)

	return err
}

// Summary names the hook type and where it goes.
func (ch createHook) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(ch.DocumentID, ch.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	// Validate already accepted the settings, so this cannot fail.
	tp, _, err := decodeHookSettings(ch.Settings)
	if err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:         NameCreateHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Add a " + hookLabel(tp, null.NewString(ch.BlockUID, ch.BlockUID != ""), doc),
	}, nil
}

// Exec creates the hook.
func (ch createHook) Exec(inp Input) (any, error) {
	// Validate already accepted the settings, so this cannot fail.
	tp, settings, err := decodeHookSettings(ch.Settings)
	if err != nil {
		return nil, err
	}

	hk, err := inp.CreateHook(ch.DocumentID, ch.BranchID, ch.BlockUID, tp, settings)
	if err != nil {
		return nil, err
	}

	return newHookRow(hk), nil
}

// updateHook resets a hook's triggered flag and state, replacing its
// settings when new ones are given.
type updateHook struct {
	hookRef

	// Settings is the hook's type and its new settings in the shape
	// that type takes; absent keeps the settings and only resets the
	// hook.
	Settings json.RawMessage `json:"settings"`
}

// Info returns the tool's model-facing description.
func (updateHook) Info() Info {
	return Info{
		Name:        NameUpdateHook,
		Traits:      Traits{Group: GroupHooks, Write: true, Overwrites: true, Role: org.RoleEditor},
		Description: "Reset a hook to a fresh hook's state and return it as list_hooks does. It is then untriggered, but a reminder already past its schedule stays triggered until settings give it a later one. Without settings the hook keeps its own, so call it that way once the document is up to date with what the hook flagged. New settings replace the old ones whole, so send their type too. It must be the hook's own type, so to change it, delete the hook and create another.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"hook_id":     map[string]any{"type": "string", "description": _hookIDDescription},
			"settings":    map[string]any{"type": "object", "description": "New settings in create_hook's shape, type included."},
		},
		Required: []string{"document_id", "hook_id"},
	}
}

// Validate checks the arguments name a hook and, when they carry
// settings, that those name a hook type with exactly that type's
// fields. That the type is the hook's own is checked once it is
// fetched.
func (uh updateHook) Validate() error {
	if err := uh.hookRef.Validate(); err != nil {
		return err
	}

	if !uh.replacesSettings() {
		return nil
	}

	_, _, err := decodeHookSettings(uh.Settings)

	return err
}

// Summary names the hook and checks the settings fit its type, so the
// user is not asked to approve a call that cannot run.
func (uh updateHook) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, hk, err := uh.fetchHookAndBranch(inp)
	if err != nil {
		return ActionSummary{}, err
	}

	summary := "Reset the " + hookLabel(hk.Type, hk.BlockID, doc)

	if uh.replacesSettings() {
		if _, err := uh.settingsFor(hk); err != nil {
			return ActionSummary{}, err
		}

		summary = "Update the " + hookLabel(hk.Type, hk.BlockID, doc)
	}

	return ActionSummary{
		Tool:         NameUpdateHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Exec resets the hook, replacing its settings when new ones were
// given.
func (uh updateHook) Exec(inp Input) (any, error) {
	hk, err := inp.FetchHook(uh.DocumentID, uh.HookID)
	if err != nil {
		return nil, err
	}

	if !uh.replacesSettings() {
		if rerr := inp.ResetHook(hk); rerr != nil {
			return nil, rerr
		}

		return newHookRow(hk), nil
	}

	settings, err := uh.settingsFor(hk)
	if err != nil {
		return nil, err
	}

	if err := inp.UpdateHook(hk, settings); err != nil {
		return nil, err
	}

	return newHookRow(hk), nil
}

// replacesSettings reports whether the call carries new settings, as
// opposed to only resetting the hook.
func (uh updateHook) replacesSettings() bool {
	trimmed := bytes.TrimSpace(uh.Settings)

	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

// settingsFor reads the call's settings for the hook it names, refusing
// settings of another type: a hook keeps its type for life.
func (uh updateHook) settingsFor(hk *hook.Hook) (processor.Settings, error) {
	tp, settings, err := decodeHookSettings(uh.Settings)
	if err != nil {
		return nil, err
	}

	if tp != hk.Type {
		return nil, fmt.Errorf("the hook is a %s, not a %s; to change the type, delete it and create another", hk.Type, tp)
	}

	return settings, nil
}

// deleteHook removes a hook from its document.
type deleteHook struct {
	hookRef
}

// Info returns the tool's model-facing description.
func (deleteHook) Info() Info {
	return Info{
		Name:        NameDeleteHook,
		Traits:      Traits{Group: GroupHooks, Write: true, Destructive: true, Role: org.RoleEditor},
		Description: "Delete a hook and stop its watch outside the document. Returns {hook_id, deleted}. It cannot be restored. update_hook without settings clears what it flagged instead. A protected branch keeps its hooks.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"hook_id":     map[string]any{"type": "string", "description": _hookIDDescription},
		},
		Required: []string{"document_id", "hook_id"},
	}
}

// Summary names the hook being deleted.
func (dh deleteHook) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, hk, err := dh.fetchHookAndBranch(inp)
	if err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:         NameDeleteHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Delete the " + hookLabel(hk.Type, hk.BlockID, doc),
	}, nil
}

// Exec deletes the hook.
func (dh deleteHook) Exec(inp Input) (any, error) {
	hk, err := inp.FetchHook(dh.DocumentID, dh.HookID)
	if err != nil {
		return nil, err
	}

	if err := inp.DeleteHook(hk); err != nil {
		return nil, err
	}

	return deleteHookResult{HookID: hk.ID, Deleted: true}, nil
}

// deleteHookResult is what delete_hook returns.
type deleteHookResult struct {
	// HookID is the hook that was removed.
	HookID xid.ID `json:"hook_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// hookRef is a document and one of its hooks, which is what
// delete_hook is called with and update_hook starts from.
type hookRef struct {
	// DocumentID names the document the hook is on.
	DocumentID xid.ID `json:"document_id"`

	// HookID names the hook.
	HookID xid.ID `json:"hook_id"`
}

// Validate checks the arguments name a document and a hook.
func (a hookRef) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired("document_id")
	}

	if a.HookID.IsNil() {
		return errRequired("hook_id")
	}

	return nil
}

// fetchHookAndBranch fetches the hook the arguments name and the
// document on the branch the hook is on, so a label can name that
// branch. A hook whose branch is gone is named on the document's default
// one.
func (a hookRef) fetchHookAndBranch(inp DescribeInput) (*document.Document, *hook.Hook, error) {
	hk, err := inp.FetchHook(a.DocumentID, a.HookID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching hook: %w", err)
	}

	var doc *document.Document

	if hk.BranchID.Valid {
		doc, err = inp.FetchBranch(a.DocumentID, hk.BranchID.V)
	} else {
		doc, err = inp.FetchDocument(a.DocumentID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("fetching document: %w", err)
	}

	return doc, hk, nil
}

// hookLabel names a hook by its type, the document it is on and, when
// anchored, its block, in the words the confirm card uses.
func hookLabel(tp hook.Type, blockUID null.String, doc *document.Document) string {
	label := tp.HumanizedString() + " hook on " + doc.Title()

	if blockUID.Valid {
		label += ", block " + blockUID.String
	}

	return label
}

// hookSettingsSchema builds the settings argument's schema for the hook
// writes: one variant per hook type, each naming the type as a constant
// and taking only that type's own fields, so choosing a type and the
// shape of its settings is a single act.
func hookSettingsSchema() map[string]any {
	return map[string]any{
		"description": "The hook's type and its settings.",
		"anyOf": []map[string]any{
			{
				"type": "object",
				"properties": map[string]any{
					"type":     map[string]any{"const": string(hook.TypeScheduledReminder)},
					"schedule": map[string]any{"type": "string", "description": "When the reminder is due, RFC 3339."},
				},
				"required":             []string{"type", "schedule"},
				"additionalProperties": false,
			},
			{
				"type": "object",
				"properties": map[string]any{
					"type":       map[string]any{"const": string(hook.TypeGithubTracking)},
					"repository": map[string]any{"type": "string", "description": "The repository name, owned by the connected GitHub account."},
					"branch":     map[string]any{"type": "string", "description": "The branch to follow."},
					"paths": map[string]any{
						"type":        "array",
						"description": "The paths whose changes mark the document stale.",
						"items":       map[string]any{"type": "string"},
						"minItems":    1,
					},
				},
				"required":             []string{"type", "repository", "branch", "paths"},
				"additionalProperties": false,
			},
			{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{"const": string(hook.TypeURLWatcher)},
					"url":  map[string]any{"type": "string", "description": "The address to watch."},
				},
				"required":             []string{"type", "url"},
				"additionalProperties": false,
			},
			{
				"type": "object",
				"properties": map[string]any{
					"type":  map[string]any{"const": string(hook.TypeContainerImageWatcher)},
					"image": map[string]any{"type": "string", "description": "The image reference to watch for new digests."},
				},
				"required":             []string{"type", "image"},
				"additionalProperties": false,
			},
		},
	}
}

// decodeHookSettings reads the settings the model supplied as the
// processor settings of the type they name: each type decodes into its
// own struct, requires its own fields, and refuses a field of another
// type naming the type it belongs to.
func decodeHookSettings(raw json.RawMessage) (hook.Type, processor.Settings, error) {
	var head struct {
		// Type is the hook type the settings belong to.
		Type hook.Type `json:"type"`
	}

	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &head); err != nil {
			return "", nil, fmt.Errorf("invalid settings: %w", err)
		}
	}

	tp := head.Type

	if tp == "" {
		return "", nil, errRequired("settings.type")
	}

	if err := tp.Validate(); err != nil {
		return "", nil, fmt.Errorf("type %q: %w", tp, err)
	}

	var v any

	switch tp {
	case hook.TypeScheduledReminder:
		sr, err := decodeSettings[processor.ScheduledReminder](tp, raw)
		if err != nil {
			return "", nil, err
		}

		if sr.Schedule.IsZero() {
			return "", nil, errRequired("schedule")
		}

		// a concrete date is what the editor calls a custom duration.
		sr.Duration = null.StringFrom("custom")
		v = sr
	case hook.TypeGithubTracking:
		gt, err := decodeSettings[processor.GithubTracking](tp, raw)
		if err != nil {
			return "", nil, err
		}

		switch {
		case gt.Repository == "":
			return "", nil, errRequired("repository")
		case gt.Branch == "":
			return "", nil, errRequired("branch")
		case len(gt.Paths) == 0:
			return "", nil, errRequired("paths")
		}

		v = gt
	case hook.TypeURLWatcher:
		uw, err := decodeSettings[processor.URLWatcher](tp, raw)
		if err != nil {
			return "", nil, err
		}

		if uw.URL == "" {
			return "", nil, errRequired("url")
		}

		v = uw
	case hook.TypeContainerImageWatcher:
		ciw, err := decodeSettings[processor.ContainerImageWatcher](tp, raw)
		if err != nil {
			return "", nil, err
		}

		if ciw.Image == "" {
			return "", nil, errRequired("image")
		}

		v = ciw
	}

	out, err := json.Marshal(v)
	if err != nil {
		return "", nil, fmt.Errorf("marshalling settings: %w", err)
	}

	return tp, processor.Settings(out), nil
}

// decodeSettings unmarshals the settings into the processor struct of
// the given type, refusing a field the struct does not have. The type
// field sits beside the settings, so it is not refused.
func decodeSettings[T any](tp hook.Type, raw json.RawMessage) (T, error) {
	var dst struct {
		// Settings is the processor struct the settings decode into.
		Settings T `json:",embed"`

		// Type is the hook type the settings belong to.
		Type hook.Type `json:"type"`
	}

	err := jsonv2.Unmarshal(raw, &dst, jsonv2.RejectUnknownMembers(true))
	if err == nil {
		return dst.Settings, nil
	}

	var serr *jsonv2.SemanticError

	if errors.As(err, &serr) && errors.Is(serr.Err, jsonv2.ErrUnknownName) {
		return dst.Settings, fmt.Errorf("%s is not a %s setting", serr.JSONPointer.LastToken(), tp)
	}

	return dst.Settings, fmt.Errorf("invalid settings: %w", err)
}

// HookInput is what the hook tools reach on top of DescribeInput.
type HookInput interface {
	// FetchHooks should return the hooks on the branch.
	FetchHooks(documentID, branchID xid.ID) ([]hook.Hook, error)

	// CreateHook should create a hook of the type on the branch, or on
	// one block of it when blockUID is set.
	CreateHook(documentID, branchID xid.ID, blockUID string, tp hook.Type, settings processor.Settings) (*hook.Hook, error)

	// UpdateHook should replace the hook's settings.
	UpdateHook(hk *hook.Hook, settings processor.Settings) error

	// ResetHook should restore the hook's triggered flag and state,
	// keeping its settings.
	ResetHook(hk *hook.Hook) error

	// DeleteHook should delete the hook.
	DeleteHook(hk *hook.Hook) error
}
