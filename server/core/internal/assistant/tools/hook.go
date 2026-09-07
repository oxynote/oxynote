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

// Hook argument names and descriptions.
const (
	// _keyHookID is the shared hook-id property name.
	_keyHookID = "hook_id"

	// _descHookID describes the hook-id property every hook write takes.
	_descHookID = "The hook id, as list_hooks reports it."

	// _keySchedule is the scheduled-reminder timestamp property name.
	_keySchedule = "schedule"

	// _keyRepository is the github-tracking repository property name.
	_keyRepository = "repository"

	// _keyBranch is the github-tracking branch property name.
	_keyBranch = "branch"

	// _keyPaths is the github-tracking paths property name.
	_keyPaths = "paths"

	// _keyURL is the url-watcher address property name.
	_keyURL = "url"

	// _keyImage is the container-image-watcher image property name.
	_keyImage = "image"

	// _durationCustom is the duration a scheduled reminder stores when
	// its schedule is a concrete date rather than a preset, so the
	// editor labels it as such.
	_durationCustom = "custom"
)

// _keySettings is the settings property name create_hook and
// update_hook take.
const _keySettings = "settings"

// hookSettingsSchema builds the settings argument's schema for the hook
// writes. Which fields apply is decided by the hook type, the way a
// block's type decides its fields, and that rule is checked server-side.
func hookSettingsSchema() map[string]any {
	return map[string]any{
		_keyType:        _typeObject,
		_keyDescription: "The hook's settings. Which fields apply depends on the hook type; supply only that type's own.",
		"properties": map[string]any{
			_keySchedule:   stringProp("scheduled-reminder: the RFC 3339 timestamp at which the reminder is due."),
			_keyRepository: stringProp("github-tracking: the repository to follow, as owner/name."),
			_keyBranch:     stringProp("github-tracking: the repository branch to follow."),
			_keyPaths: map[string]any{
				_keyType:        _typeArray,
				_keyDescription: "github-tracking: the repository paths whose changes mark the document stale; at least one.",
				_keyItems:       map[string]any{_keyType: _typeString},
			},
			_keyURL:   stringProp("url-watcher: the web address to watch for changes."),
			_keyImage: stringProp("container-image-watcher: the container image reference to watch for new digests."),
		},
	}
}

// decodeHookSettings reads the settings the model supplied as the
// processor settings of the hook type: each type decodes into its own
// struct, requires its own fields, and refuses a field of another type
// naming the type it belongs to.
func decodeHookSettings(tp hook.Type, raw json.RawMessage) (processor.Settings, error) {
	var v any

	switch tp {
	case hook.TypeScheduledReminder:
		var sr processor.ScheduledReminder

		if err := decodeSettings(raw, &sr); err != nil {
			return nil, err
		}

		if sr.Schedule.IsZero() {
			return nil, errRequired(_keySchedule)
		}

		// a concrete date is what the editor calls a custom duration.
		sr.Scale = processor.ScaleTypeLinear
		sr.Duration = null.StringFrom(_durationCustom)
		v = sr
	case hook.TypeGithubTracking:
		var gt processor.GithubTracking

		if err := decodeSettings(raw, &gt); err != nil {
			return nil, err
		}

		switch {
		case gt.Repository == "":
			return nil, errRequired(_keyRepository)
		case gt.Branch == "":
			return nil, errRequired(_keyBranch)
		case len(gt.Paths) == 0:
			return nil, errRequired(_keyPaths)
		}

		v = gt
	case hook.TypeURLWatcher:
		var uw processor.URLWatcher

		if err := decodeSettings(raw, &uw); err != nil {
			return nil, err
		}

		if uw.URL == "" {
			return nil, errRequired(_keyURL)
		}

		v = uw
	case hook.TypeContainerImageWatcher:
		var ciw processor.ContainerImageWatcher

		if err := decodeSettings(raw, &ciw); err != nil {
			return nil, err
		}

		if ciw.Image == "" {
			return nil, errRequired(_keyImage)
		}

		v = ciw
	default:
		return nil, fmt.Errorf("type %q: %w", tp, hook.ErrInvalidType)
	}

	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshalling settings: %w", err)
	}

	return processor.Settings(out), nil
}

// decodeSettings unmarshals the settings into the processor struct,
// refusing a field the struct does not have. An absent payload is an
// empty one, so the type's own check reports what is missing.
func decodeSettings(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}

	err := jsonv2.Unmarshal(raw, dst, jsonv2.RejectUnknownMembers(true))
	if err == nil {
		return nil
	}

	var serr *jsonv2.SemanticError

	if errors.As(err, &serr) && errors.Is(serr.Err, jsonv2.ErrUnknownName) {
		key := serr.JSONPointer.LastToken()

		if owner, ok := hookSettingOwner(key); ok {
			return fmt.Errorf("%s applies to %s only", key, owner)
		}

		return fmt.Errorf("%s is not a hook setting", key)
	}

	return fmt.Errorf("invalid settings: %w", err)
}

// hookSettingOwner names the hook type a setting belongs to.
func hookSettingOwner(key string) (hook.Type, bool) {
	switch key {
	case _keySchedule:
		return hook.TypeScheduledReminder, true
	case _keyRepository, _keyBranch, _keyPaths:
		return hook.TypeGithubTracking, true
	case _keyURL:
		return hook.TypeURLWatcher, true
	case _keyImage:
		return hook.TypeContainerImageWatcher, true
	default:
		return "", false
	}
}

// _descHookTypes explains the four hook types once, for the descriptions
// that list them.
const _descHookTypes = "scheduled-reminder takes schedule; github-tracking takes repository, branch and paths; url-watcher takes url; container-image-watcher takes image. github-tracking and url-watcher depend on the deployment's GitHub App and changedetection.io integrations, and are refused where those are missing."

// listHooksArgs is what list_hooks is called with.
type listHooksArgs struct {
	docTarget
}

// Validate checks the arguments name a branch.
func (a listHooksArgs) Validate() error {
	return a.validate()
}

// listHooks returns every hook on one branch of a document.
type listHooks struct {
	plainSummary
	plainTraits
	plainTitle
}

// Info returns the tool's model-facing description.
func (listHooks) Info() Info {
	return Info{
		Name:        NameListHooks,
		Description: "List the freshness hooks on one branch of a document as [{id, type, block_uid, settings, score, state, created_at, updated_at}]. A hook watches something outside the document and lowers its score as that thing changes or a date approaches; block_uid names the block it is anchored to, or is null for a hook on the document as a whole. Use it to learn the id that update_hook, reset_hook and delete_hook take as hook_id. document_id and branch_id name the branch the way the content tools do.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyBranchID:   stringProp("The id of the branch whose hooks to list: a document's default_branch_id from list_documents, or any id from the branches get_document lists."),
		},
		Required: []string{_keyDocumentID, _keyBranchID},
	}
}

// Execute lists the branch's hooks.
func (listHooks) Execute(inp Input) (string, error) {
	var in listHooksArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hooks, err := inp.FetchHooks(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("list_hooks: %w", err)
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
	// shape.
	State json.RawMessage `json:"state"`

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
		State:     json.RawMessage(hk.State),
		CreatedAt: hk.CreatedAt,
		UpdatedAt: hk.UpdatedAt,
	}
}

// createHookArgs is what create_hook is called with.
type createHookArgs struct {
	docTarget

	// Type is the hook type.
	Type hook.Type `json:"type"`

	// BlockUID anchors the hook to a block; empty puts it on the
	// document as a whole.
	BlockUID string `json:"block_uid"`

	// Settings is the hook's settings in the shape its type takes.
	Settings json.RawMessage `json:"settings"`
}

// Validate checks the arguments name a branch and a hook type, and
// carry exactly that type's settings.
func (a createHookArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.Type == "" {
		return errRequired(_keyType)
	}

	if err := a.Type.Validate(); err != nil {
		return fmt.Errorf("type %q: %w", a.Type, err)
	}

	_, err := decodeHookSettings(a.Type, a.Settings)

	return err
}

// createHook adds a hook to one branch of a document.
type createHook struct{}

// Info returns the tool's model-facing description.
func (createHook) Info() Info {
	return Info{
		Name:        NameCreateHook,
		Description: "Add a freshness hook to one branch of a document and return it as list_hooks would. Supply type and, in settings, only that type's fields: " + _descHookTypes + " block_uid anchors the hook to a block; a uid the branch does not hold is refused. A new hook starts at score 100.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyBranchID:   stringProp("The id of the branch the hook goes on: a document's default_branch_id from list_documents, or any id from the branches get_document lists."),
			_keyType: map[string]any{
				_keyType:        _typeString,
				_keyDescription: "The hook type, which decides the fields settings takes.",
				_keyEnum:        hookTypes(),
			},
			_keyBlockUID: stringProp("Optional. The uid of a block on the branch to anchor the hook to, from get_document; omit to put the hook on the document as a whole."),
			_keySettings: hookSettingsSchema(),
		},
		Required: []string{_keyDocumentID, _keyBranchID, _keyType, _keySettings},
	}
}

// Traits reports a write.
func (createHook) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document gets the hook.
func (createHook) Title(inp DescribeInput) (string, error) {
	var in createHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameCreateHook, err)
	}

	return "Adding a hook to " + docLabel(doc), nil
}

// Summary names the hook type and where it goes.
func (createHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in createHookArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameCreateHook, err)
	}

	return ActionSummary{
		Tool:         NameCreateHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Add a " + hookLabel(in.Type, null.NewString(in.BlockUID, in.BlockUID != ""), doc),
	}, nil
}

// Execute creates the hook.
func (createHook) Execute(inp Input) (string, error) {
	var in createHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// Validate already accepted the settings, so this cannot fail.
	settings, err := decodeHookSettings(in.Type, in.Settings)
	if err != nil {
		return "", fmt.Errorf("create_hook: %w", err)
	}

	hk, err := inp.CreateHook(in.DocumentID, in.BranchID, in.BlockUID, in.Type, settings)
	if err != nil {
		return "", fmt.Errorf("create_hook: %w", err)
	}

	return result(newHookRow(hk))
}

// updateHookArgs is what update_hook is called with.
type updateHookArgs struct {
	hookRefArgs

	// Settings is the hook's new settings in the shape its type takes.
	Settings json.RawMessage `json:"settings"`
}

// Validate checks the arguments name a hook and carry settings. The
// settings are checked against the hook's type once it is fetched, since
// the type is the hook's own and not an argument.
func (a updateHookArgs) Validate() error {
	if err := a.hookRefArgs.Validate(); err != nil {
		return err
	}

	if len(bytes.TrimSpace(a.Settings)) == 0 {
		return errRequired(_keySettings)
	}

	return nil
}

// updateHook replaces a hook's settings.
type updateHook struct{}

// Info returns the tool's model-facing description.
func (updateHook) Info() Info {
	return Info{
		Name:        NameUpdateHook,
		Description: "Replace the settings of a hook and reset its score and state, returning it as list_hooks would. The hook keeps its type, so give exactly that type's fields in settings: " + _descHookTypes + " To change the type, delete the hook and create another.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyHookID:     stringProp(_descHookID),
			_keySettings:   hookSettingsSchema(),
		},
		Required: []string{_keyDocumentID, _keyHookID, _keySettings},
	}
}

// Traits reports a write.
func (updateHook) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which hook is being updated.
func (updateHook) Title(inp DescribeInput) (string, error) {
	var in updateHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameUpdateHook, err)
	}

	return "Updating a hook on " + doc.DocumentName, nil
}

// Summary names the hook and checks the settings fit its type, so the
// user is not asked to approve a call that cannot run.
func (updateHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateHookArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, hk, err := describeHook(inp, NameUpdateHook, in.hookRefArgs)
	if err != nil {
		return ActionSummary{}, err
	}

	if _, err := decodeHookSettings(hk.Type, in.Settings); err != nil {
		return ActionSummary{}, fmt.Errorf("%s: %w", NameUpdateHook, err)
	}

	return ActionSummary{
		Tool:         NameUpdateHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Update the " + hookLabel(hk.Type, hk.BlockID, doc),
	}, nil
}

// Execute replaces the settings and resets the hook.
func (updateHook) Execute(inp Input) (string, error) {
	var in updateHookArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hk, err := inp.FetchHook(in.DocumentID, in.HookID)
	if err != nil {
		return "", fmt.Errorf("update_hook: %w", err)
	}

	settings, err := decodeHookSettings(hk.Type, in.Settings)
	if err != nil {
		return "", fmt.Errorf("update_hook: %w", err)
	}

	if err := inp.UpdateHook(hk, settings); err != nil {
		return "", fmt.Errorf("update_hook: %w", err)
	}

	return result(newHookRow(hk))
}

// hookRefArgs is what reset_hook and delete_hook are called with: a
// document and one of its hooks.
type hookRefArgs struct {
	// DocumentID names the document the hook is on.
	DocumentID xid.ID `json:"document_id"`

	// HookID names the hook.
	HookID xid.ID `json:"hook_id"`
}

// Validate checks the arguments name a document and a hook.
func (a hookRefArgs) Validate() error {
	if a.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	if a.HookID.IsNil() {
		return errRequired(_keyHookID)
	}

	return nil
}

// hookRefProps builds the argument schema reset_hook and delete_hook
// share.
func hookRefProps() map[string]any {
	return map[string]any{
		_keyDocumentID: stringProp(_descDocumentID),
		_keyHookID:     stringProp(_descHookID),
	}
}

// resetHook restores a hook's score and state without touching its
// settings.
type resetHook struct{}

// Info returns the tool's model-facing description.
func (resetHook) Info() Info {
	return Info{
		Name:        NameResetHook,
		Description: "Reset a hook's score to 100 and its state to what a fresh hook has, keeping its settings, and return it as list_hooks would. Use it once the document has been brought up to date with whatever the hook flagged.",
		Properties:  hookRefProps(),
		Required:    []string{_keyDocumentID, _keyHookID},
	}
}

// Traits reports a write.
func (resetHook) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which hook is being reset.
func (resetHook) Title(inp DescribeInput) (string, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameResetHook, err)
	}

	return "Resetting a hook on " + doc.DocumentName, nil
}

// Summary names the hook being reset.
func (resetHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, hk, err := describeHook(inp, NameResetHook, in)
	if err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:         NameResetHook,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Reset the " + hookLabel(hk.Type, hk.BlockID, doc),
	}, nil
}

// Execute resets the hook.
func (resetHook) Execute(inp Input) (string, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hk, err := inp.FetchHook(in.DocumentID, in.HookID)
	if err != nil {
		return "", fmt.Errorf("reset_hook: %w", err)
	}

	if err := inp.ResetHook(hk); err != nil {
		return "", fmt.Errorf("reset_hook: %w", err)
	}

	return result(newHookRow(hk))
}

// deleteHook removes a hook from its document.
type deleteHook struct{}

// Info returns the tool's model-facing description.
func (deleteHook) Info() Info {
	return Info{
		Name:        NameDeleteHook,
		Description: "Delete a hook, tearing down whatever it watches outside the document; it cannot be restored, so use reset_hook when the aim is to clear what it flagged rather than stop watching. Returns {hook_id, deleted}.",
		Properties:  hookRefProps(),
		Required:    []string{_keyDocumentID, _keyHookID},
	}
}

// Traits reports a destructive write, which stays outside any "approve
// all" answer.
func (deleteHook) Traits() Traits {
	return Traits{Write: true, Destructive: true}
}

// Title announces which hook is being deleted.
func (deleteHook) Title(inp DescribeInput) (string, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.FetchDocument(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameDeleteHook, err)
	}

	return "Deleting a hook on " + doc.DocumentName, nil
}

// Summary names the hook being deleted.
func (deleteHook) Summary(inp DescribeInput) (ActionSummary, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, hk, err := describeHook(inp, NameDeleteHook, in)
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
func (deleteHook) Execute(inp Input) (string, error) {
	var in hookRefArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	hk, err := inp.FetchHook(in.DocumentID, in.HookID)
	if err != nil {
		return "", fmt.Errorf("delete_hook: %w", err)
	}

	if err := inp.DeleteHook(hk); err != nil {
		return "", fmt.Errorf("delete_hook: %w", err)
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

// describeHook resolves the hook a description names and the document
// on the branch the hook is on, so the label can name that branch. A
// hook whose branch is gone is named on the document's default one.
func describeHook(inp DescribeInput, name Name, ref hookRefArgs) (*document.Document, *hook.Hook, error) {
	hk, err := inp.FetchHook(ref.DocumentID, ref.HookID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: fetch hook: %w", name, err)
	}

	var doc *document.Document

	if hk.BranchID.Valid {
		doc, err = inp.FetchBranch(ref.DocumentID, hk.BranchID.V)
	} else {
		doc, err = inp.FetchDocument(ref.DocumentID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("%s: fetch document: %w", name, err)
	}

	return doc, hk, nil
}

// hookLabel names a hook by its type, the document it is on and, when
// anchored, its block, in the words the confirm card uses.
func hookLabel(tp hook.Type, blockUID null.String, doc *document.Document) string {
	label := tp.HumanizedString() + " hook on " + docLabel(doc)

	if blockUID.Valid {
		label += ", block " + blockUID.String
	}

	return label
}

// hookTypes lists the hook types the schema publishes as an enum.
func hookTypes() []string {
	return []string{
		string(hook.TypeScheduledReminder),
		string(hook.TypeGithubTracking),
		string(hook.TypeURLWatcher),
		string(hook.TypeContainerImageWatcher),
	}
}
