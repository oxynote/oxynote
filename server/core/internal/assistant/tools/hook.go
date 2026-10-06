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
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
)

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
		sr.Scale = processor.ScaleTypeLinear
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
		Settings T `json:",embed"` //nolint:revive // embed is the json/v2 option revive does not know yet

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

// listHooksArgs is what list_hooks is called with.
type listHooksArgs struct {
	docTarget
}

// listHooks returns every hook on one branch of a document.
type listHooks struct{}

// Info returns the tool's model-facing description.
func (listHooks) Info() Info {
	return Info{
		Name:        NameListHooks,
		Description: "List the freshness hooks on one branch of a document as [{id, type, block_uid, settings, score, state, status, created_at, updated_at}]. A hook watches something outside the document and lowers its score from 100 as that thing changes or a date approaches. block_uid is the block it is anchored to, or null for the whole document.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
		},
		Required: []string{"document_id", "branch_id"},
	}
}

// Execute lists the branch's hooks.
func (listHooks) Execute(inp *input) (string, error) {
	var in listHooksArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hooks, err := inp.FetchHooks(in.DocumentID, in.BranchID)
	if err != nil {
		return "", err
	}

	out := hookListResult{Hooks: make([]hookRow, 0, len(hooks))}

	for i := range hooks {
		out.Hooks = append(out.Hooks, newHookRow(&hooks[i]))
	}

	return result(out)
}

// hookListResult is what list_hooks returns.
type hookListResult struct {
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

	// Score is the hook's freshness score, from 100 down to 0.
	Score decimal.Decimal `json:"score"`

	// State is what the hook has recorded so far, in its type's own
	// shape, or null for a hook not set up yet.
	State json.RawMessage `json:"state"`

	// Status tells whether the hook could check its target on its last
	// run. Score and state are left as they were while it is not active.
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
		Score:     hk.Score,
		State:     json.RawMessage(hk.State.V),
		Status:    hk.Status,
		CreatedAt: hk.CreatedAt,
		UpdatedAt: hk.UpdatedAt,
	}
}

// createHookArgs is what create_hook is called with.
type createHookArgs struct {
	docTarget

	// BlockUID anchors the hook to a block; empty puts it on the
	// document as a whole.
	BlockUID string `json:"block_uid"`

	// Settings is the hook's type and its settings in the shape that
	// type takes.
	Settings json.RawMessage `json:"settings"`
}

// Validate checks the arguments name a branch and carry settings naming
// a hook type, with exactly that type's fields.
func (a createHookArgs) Validate() error {
	if err := a.docTarget.Validate(); err != nil {
		return err
	}

	_, _, err := decodeHookSettings(a.Settings)

	return err
}

// createHook adds a hook to one branch of a document.
type createHook struct{}

// Info returns the tool's model-facing description.
func (createHook) Info() Info {
	return Info{
		Name:        NameCreateHook,
		Traits:      Traits{Write: true},
		Description: "Add a freshness hook to one branch of a document and return it as list_hooks would. github-tracking and url-watcher need the deployment's GitHub App and changedetection.io integrations, and are refused without them. A new hook starts at score 100.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "Optional. The id of a block on the branch to anchor the hook to; omit it for the whole document."},
			"settings":    hookSettingsSchema(),
		},
		Required: []string{"document_id", "branch_id", "settings"},
	}
}

// Summary names the hook type and where it goes.
func (createHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createHookArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	// Validate already accepted the settings, so this cannot fail.
	tp, _, err := decodeHookSettings(in.Settings)
	if err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:         NameCreateHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Add a " + hookLabel(tp, null.NewString(in.BlockUID, in.BlockUID != ""), doc),
	}, nil
}

// Execute creates the hook.
func (createHook) Execute(inp *input) (string, error) {
	var in createHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// Validate already accepted the settings, so this cannot fail.
	tp, settings, err := decodeHookSettings(in.Settings)
	if err != nil {
		return "", err
	}

	hk, err := inp.CreateHook(in.DocumentID, in.BranchID, in.BlockUID, tp, settings)
	if err != nil {
		return "", err
	}

	return result(newHookRow(hk))
}

// updateHookArgs is what update_hook is called with.
type updateHookArgs struct {
	hookRefArgs

	// Settings is the hook's type and its new settings in the shape
	// that type takes; absent keeps the settings and only resets the
	// hook.
	Settings json.RawMessage `json:"settings"`
}

// Validate checks the arguments name a hook and, when they carry
// settings, that those name a hook type with exactly that type's
// fields. That the type is the hook's own is checked once it is
// fetched.
func (a updateHookArgs) Validate() error {
	if err := a.hookRefArgs.Validate(); err != nil {
		return err
	}

	if !a.replacesSettings() {
		return nil
	}

	_, _, err := decodeHookSettings(a.Settings)

	return err
}

// replacesSettings reports whether the call carries new settings, as
// opposed to only resetting the hook.
func (a updateHookArgs) replacesSettings() bool {
	trimmed := bytes.TrimSpace(a.Settings)

	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

// settingsFor reads the call's settings for the hook it names, refusing
// settings of another type: a hook keeps its type for life.
func (a updateHookArgs) settingsFor(hk *hook.Hook) (processor.Settings, error) {
	tp, settings, err := decodeHookSettings(a.Settings)
	if err != nil {
		return nil, err
	}

	if tp != hk.Type {
		return nil, fmt.Errorf("the hook is a %s, not a %s; to change the type, delete it and create another", hk.Type, tp)
	}

	return settings, nil
}

// updateHook resets a hook's score and state, replacing its settings
// when new ones are given.
type updateHook struct{}

// Info returns the tool's model-facing description.
func (updateHook) Info() Info {
	return Info{
		Name:        NameUpdateHook,
		Traits:      Traits{Write: true},
		Description: "Reset a hook's score to 100 and its state to a fresh hook's, and return it as list_hooks would. Without settings the hook keeps them: use that once the document is brought up to date with what the hook flagged. With settings they replace the old ones; the hook keeps its type, so to change it delete the hook and create another.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"hook_id":     map[string]any{"type": "string", "description": _hookIDDescription},
			"settings":    map[string]any{"type": "object", "description": "Optional. New settings, shaped as create_hook's, with the hook's own type."},
		},
		Required: []string{"document_id", "hook_id"},
	}
}

// Summary names the hook and checks the settings fit its type, so the
// user is not asked to approve a call that cannot run.
func (updateHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateHookArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, hk, err := in.fetchHookAndBranch(inp)
	if err != nil {
		return ActionSummary{}, err
	}

	summary := "Reset the " + hookLabel(hk.Type, hk.BlockID, doc)

	if in.replacesSettings() {
		if _, err := in.settingsFor(hk); err != nil {
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

// Execute resets the hook, replacing its settings when new ones were
// given.
func (updateHook) Execute(inp *input) (string, error) {
	var in updateHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hk, err := inp.FetchHook(in.DocumentID, in.HookID)
	if err != nil {
		return "", err
	}

	if !in.replacesSettings() {
		if rerr := inp.ResetHook(hk); rerr != nil {
			return "", rerr
		}

		return result(newHookRow(hk))
	}

	settings, err := in.settingsFor(hk)
	if err != nil {
		return "", err
	}

	if err := inp.UpdateHook(hk, settings); err != nil {
		return "", err
	}

	return result(newHookRow(hk))
}

// hookRefArgs is a document and one of its hooks, which is what
// delete_hook is called with and update_hook starts from.
type hookRefArgs struct {
	// DocumentID names the document the hook is on.
	DocumentID xid.ID `json:"document_id"`

	// HookID names the hook.
	HookID xid.ID `json:"hook_id"`
}

// Validate checks the arguments name a document and a hook.
func (a hookRefArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired("document_id")
	}

	if a.HookID.IsNil() {
		return errRequired("hook_id")
	}

	return nil
}

// deleteHook removes a hook from its document.
type deleteHook struct{}

// Info returns the tool's model-facing description.
func (deleteHook) Info() Info {
	return Info{
		Name:        NameDeleteHook,
		Traits:      Traits{Write: true, Destructive: true},
		Description: "Delete a hook, tearing down whatever it watches outside the document. It cannot be restored; update_hook without settings clears what it flagged instead. Returns {hook_id, deleted}.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"hook_id":     map[string]any{"type": "string", "description": _hookIDDescription},
		},
		Required: []string{"document_id", "hook_id"},
	}
}

// Summary names the hook being deleted.
func (deleteHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, hk, err := in.fetchHookAndBranch(inp)
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

// Execute deletes the hook.
func (deleteHook) Execute(inp *input) (string, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hk, err := inp.FetchHook(in.DocumentID, in.HookID)
	if err != nil {
		return "", err
	}

	if err := inp.DeleteHook(hk); err != nil {
		return "", err
	}

	return result(deletedHookResult{HookID: hk.ID, Deleted: true})
}

// deletedHookResult is what delete_hook returns.
type deletedHookResult struct {
	// HookID is the hook that was removed.
	HookID xid.ID `json:"hook_id"`

	// Deleted confirms the removal happened, so the model reads an
	// outcome rather than an empty result.
	Deleted bool `json:"deleted"`
}

// fetchHookAndBranch fetches the hook the arguments name and the
// document on the branch the hook is on, so a label can name that
// branch. A hook whose branch is gone is named on the document's default
// one.
func (a hookRefArgs) fetchHookAndBranch(inp DescribeInput) (*document.Document, *hook.Hook, error) {
	hk, err := inp.FetchHook(a.DocumentID, a.HookID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch hook: %w", err)
	}

	var doc *document.Document

	if hk.BranchID.Valid {
		doc, err = inp.FetchBranch(a.DocumentID, hk.BranchID.V)
	} else {
		doc, err = inp.FetchDocument(a.DocumentID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("fetch document: %w", err)
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
