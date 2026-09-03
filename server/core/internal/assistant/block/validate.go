package block

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Allowed child-type sets, mirroring the editor's schema rules.
var (
	// _allowedBlockquoteItems mirrors TipTap's default Blockquote
	// content (block+) — the same content expression as the document
	// root — so anything root-legal may sit inside, and Compact can
	// emit any of it back out of a stored blockquote.
	_allowedBlockquoteItems = _allowedAtRoot

	// _allowedListItemContent is the set of block types that can be
	// nested under a list or task-list entry — the "block*" half of
	// the editor's "paragraph block*" content for ListItem and a
	// nested TaskItem. The "paragraph" half is the entry itself, which
	// validateListEntry checks; the two positions take different types
	// and do not share a rule.
	_allowedListItemContent = map[Type]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
	}

	// _allowedCalloutItems mirrors CalloutBlock.content in
	// web/app/components/editor/blocks/callout/index.ts.
	_allowedCalloutItems = map[Type]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
	}

	// _allowedSplitDocLeftBody is the set of types that may appear
	// between the leading heading and any trailing param_lists on
	// a split_doc left side.
	_allowedSplitDocLeftBody = map[Type]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
		BlockCallout:     true,
	}

	// _allowedSplitDocRight is the set of types that may appear on a
	// split_doc right side.
	_allowedSplitDocRight = map[Type]bool{
		BlockTitledCode: true,
		BlockMetric:     true,
	}

	// _allowedSplitDocLeft is what a block landing on an existing
	// split_doc left side may be: the body set plus param_list. The
	// leading heading is excluded — it is created with the macro and
	// edited in place, never placed next to another block.
	_allowedSplitDocLeft = map[Type]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
		BlockCallout:     true,
		BlockParamList:   true,
	}

	// _allowedInContainer maps a ProseMirror container node to the
	// canonical types it accepts as direct children. Containers absent
	// from the map — lists, whose direct children are wrapper items,
	// and macro internals — accept no canonical block at all. The
	// document root is handled by ValidateAsRoot.
	_allowedInContainer = map[document.BlockNodeType]map[Type]bool{
		document.BlockNodeBlockquote:    _allowedBlockquoteItems,
		document.BlockNodeListItem:      _allowedListItemContent,
		document.BlockNodeTaskItem:      _allowedListItemContent,
		document.BlockNodeCalloutBlock:  _allowedCalloutItems,
		document.BlockNodeSplitDocLeft:  _allowedSplitDocLeft,
		document.BlockNodeSplitDocRight: _allowedSplitDocRight,
		document.BlockNodeMetricGrid:    {BlockMetric: true},
	}

	// _allowedAtRoot is the set of canonical block types that may
	// sit directly under the document root. Types not in this set
	// use custom TipTap groups
	// (splitDocumentationParameterList, metricBlock,
	// titledCodeBlock) and only become valid inside their containing
	// macro.
	_allowedAtRoot = map[Type]bool{
		BlockParagraph:      true,
		BlockHeading:        true,
		BlockBlockquote:     true,
		BlockBulletList:     true,
		BlockOrderedList:    true,
		BlockTaskList:       true,
		BlockCallout:        true,
		BlockCode:           true,
		BlockMermaid:        true,
		BlockHorizontalRule: true,
		BlockImage:          true,
		BlockFigma:          true,
		BlockMetricGrid:     true,
		BlockSplitDoc:       true,
	}
)

// validationError is the structured error returned by Validate.
// Path locates the offending block within a nested canonical tree
// using a slash-separated breadcrumb (e.g. "split_doc/left[0]",
// "param_list/params[2]"); Message describes the rule that failed.
type validationError struct {
	// Path is a slash-separated breadcrumb identifying the offending
	// node. Indices use bracketed integers (e.g. "items[3]").
	Path string

	// Message is a single-sentence description of the violated rule.
	Message string
}

// Error renders the validation error in the form
// "{path}: {message}".
func (e *validationError) Error() string {
	if e.Path == "" {
		return e.Message
	}

	return e.Path + ": " + e.Message
}

// Validate checks that a canonical Block satisfies every constraint
// Expand and the editor schema require. It is intended to be run on
// AI-supplied input before Expand. Validate stops at the first
// violation and returns a validationError describing it; callers
// can surface that message to the AI to recover precisely.
//
// A nil error means Expand will produce a structurally valid
// document.Block tree (subject to runtime constraints like the
// referenced uid existing, which only matter once the operation
// reaches hocuspocus).
func Validate(b Block) error {
	return validateBlock(b, "")
}

// ValidateAsRoot validates a block being inserted at the top level
// of a document. It runs Validate first and then rejects types
// whose TipTap group restricts them to a specific parent
// (titled_code, metric, param_list) — those types must reach the
// document tree via their macro container (split_doc, metric_grid)
// and never sit at the root.
func ValidateAsRoot(b Block) error {
	if err := validateBlock(b, ""); err != nil {
		return err
	}

	return AllowedInContainer(document.BlockNodeDoc, b.Type)
}

// ValidateAttrs checks the attribute rules for a block of the given
// type, asking nothing about its content. An attribute-only update
// names no content, so the content rules have nothing to say about it,
// while the attributes it does set answer to exactly the rules a whole
// block's would. Types carrying no attribute rules pass.
//
// Callers merge the update over the block's current attributes before
// calling: a rule reads the attributes the block will end up with, not
// the subset one call happened to name.
func ValidateAttrs(t Type, attrs document.Attributes) error {
	switch t {
	case BlockHeading:
		return validateHeadingAttrs(attrs, "")
	case BlockTitledCode:
		return validateTitledCodeAttrs(attrs, "")
	case BlockImage, BlockFigma:
		return validateSrcAttrs(t, attrs, "")
	case BlockMetric:
		return validateMetricAttrs(attrs, "")
	case BlockParagraph,
		BlockBlockquote,
		BlockBulletList,
		BlockOrderedList,
		BlockTaskList,
		BlockCallout,
		BlockCode,
		BlockMermaid,
		BlockHorizontalRule,
		BlockMetricGrid,
		BlockSplitDoc,
		BlockParamList:
		// these carry no attribute the canonical model constrains:
		// callout's icon and code's language are free strings, and the
		// rest have none at all.
		return nil
	}

	return nil
}

// ValidateInContainer validates a block that is about to land among the
// direct children of the given ProseMirror node. It runs Validate first
// and then checks the container's child rules: the document root takes
// the root set, macro containers take their own sets, and a node whose
// direct children are wrapper items — a list, a macro internal —
// accepts no canonical block at all.
func ValidateInContainer(container document.BlockNodeType, b Block) error {
	if err := validateBlock(b, ""); err != nil {
		return err
	}

	return AllowedInContainer(container, b.Type)
}

// AllowedInContainer reports whether a block of type t may sit among
// the direct children of the given ProseMirror node, asking nothing
// about the block's content. A move carries a block that is already in
// the document, so its content is not in question — only where it is
// allowed to land.
func AllowedInContainer(container document.BlockNodeType, t Type) error {
	if container == document.BlockNodeDoc {
		if !_allowedAtRoot[t] {
			return verr("", fmt.Sprintf(
				"%s is not allowed at the document root; it must appear inside %s",
				t, containerForType(t),
			))
		}

		return nil
	}

	allowed, ok := _allowedInContainer[container]
	if !ok {
		return verr("", fmt.Sprintf("blocks cannot be placed directly inside %s", container))
	}

	if !allowed[t] {
		return verr("", fmt.Sprintf(
			"%s is not allowed inside %s, which holds only %s",
			t, container, listAllowed(allowed),
		))
	}

	return nil
}

// containerForType returns a human-readable description of the
// macro that must contain a non-root block type. Used to build
// helpful error messages for ValidateAsRoot.
func containerForType(t Type) string {
	switch t {
	case BlockTitledCode:
		return "split_doc.right"
	case BlockMetric:
		return "metric_grid or split_doc.right"
	case BlockParamList:
		return "split_doc.left"
	default:
		return "its parent macro"
	}
}

// validateBlock dispatches on b.Type. Path is the breadcrumb to b
// from the validation root (passed in by recursive calls).
func validateBlock(b Block, path string) error {
	switch b.Type {
	case "":
		return verr(path, "block type is required")
	case BlockParagraph, BlockCode, BlockMermaid:
		return validateTextBearing(b, path)
	case BlockHeading:
		return validateHeading(b, path)
	case BlockBlockquote:
		return validateBlockquote(b, path)
	case BlockBulletList, BlockOrderedList:
		return validateList(b, path)
	case BlockTaskList:
		return validateTaskList(b, path)
	case BlockCallout:
		return validateCallout(b, path)
	case BlockTitledCode:
		return validateTitledCode(b, path)
	case BlockHorizontalRule:
		return validateContentless(b, path)
	case BlockMetric:
		return validateMetric(b, path)
	case BlockImage, BlockFigma:
		return validateAtomWithSrc(b, path)
	case BlockMetricGrid:
		return validateMetricGrid(b, path)
	case BlockSplitDoc:
		return validateSplitDoc(b, path)
	case BlockParamList:
		return validateParamList(b, path)
	}

	return verr(path, fmt.Sprintf(
		"unknown block type %q; the types are %s",
		b.Type, strings.Join(Types(), ", "),
	))
}

// validateTextBearing checks that the block carries only inline Text
// content and no compound fields. Used for paragraph, code and mermaid
// (heading, blockquote and titled_code layer their own rules on top of
// this).
func validateTextBearing(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept items", b.Type))
	}

	return nil
}

// validateHeading layers the level-attr rule on top of the
// text-bearing checks.
func validateHeading(b Block, path string) error {
	if err := validateTextBearing(b, path); err != nil {
		return err
	}

	return validateHeadingAttrs(b.Attrs, path)
}

// validateHeadingAttrs checks a heading's level.
func validateHeadingAttrs(attrs document.Attributes, path string) error {
	if a, ok := attrs.Get(document.AttrLevel); !ok || a.Int() < 1 || a.Int() > 3 {
		return verr(joinPath(path, "attrs.level"), "heading level must be 1, 2, or 3")
	}

	return nil
}

// validateBlockquote checks the text-or-items exclusivity rule and
// the allowed item types. Empty is legal: a blockquote holding one
// empty paragraph compacts to neither text nor items, and Expand
// turns the empty form back into that single paragraph.
func validateBlockquote(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" && len(b.Items) > 0 {
		return verr(path, "blockquote accepts either text or items, not both")
	}

	if b.Text != "" {
		return nil
	}

	return validateItemsAllowed(b.Items, joinPath(path, "items"), _allowedBlockquoteItems)
}

// validateList checks a bullet_list or ordered_list: no text, at
// least one item, items of allowed types.
func validateList(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, fmt.Sprintf("%s does not accept text", b.Type))
	}

	if len(b.Items) == 0 {
		return verr(path, fmt.Sprintf("%s requires at least one item", b.Type))
	}

	return validateListItems(b.Items, joinPath(path, "items"))
}

// validateTaskList checks that rows live in task_items and each
// row's content block is an allowed list-item type.
func validateTaskList(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "task_items"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "task_list does not accept text")
	}

	if len(b.TaskItems) == 0 {
		return verr(path, "task_list requires at least one task_item")
	}

	for i, ti := range b.TaskItems {
		itemPath := joinPath(path, fmt.Sprintf("task_items[%d]", i))

		children := ti.Block.Children
		ti.Block.Children = nil

		if err := validateBlock(ti.Block, joinPath(itemPath, "block")); err != nil {
			return err
		}

		if err := validateListEntry(ti.Block.Type, joinPath(itemPath, "block")); err != nil {
			return err
		}

		if err := validateItemsAllowed(
			children,
			joinPath(itemPath, "block/children"),
			_allowedListItemContent,
		); err != nil {
			return err
		}
	}

	return nil
}

// validateCallout checks the text-or-items exclusivity rule and
// the allowed item types. Empty is legal: a callout holding one
// empty paragraph compacts to neither text nor items, and Expand
// turns the empty form back into that single paragraph.
func validateCallout(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "items"); err != nil {
		return err
	}

	if b.Text != "" && len(b.Items) > 0 {
		return verr(path, "callout accepts either text or items, not both")
	}

	if b.Text != "" {
		return nil
	}

	return validateItemsAllowed(b.Items, joinPath(path, "items"), _allowedCalloutItems)
}

// validateTitledCode checks that a titled_code block carries a
// non-empty title attr and no compound content.
func validateTitledCode(b Block, path string) error {
	if err := validateTextBearing(b, path); err != nil {
		return err
	}

	return validateTitledCodeAttrs(b.Attrs, path)
}

// validateTitledCodeAttrs checks that a titled_code carries a title.
func validateTitledCodeAttrs(attrs document.Attributes, path string) error {
	if a, ok := attrs.Get(document.AttrTitle); !ok || strings.TrimSpace(a.String()) == "" {
		return verr(joinPath(path, "attrs.title"), "titled_code requires a non-empty title")
	}

	return nil
}

// validateContentless checks that the block carries no content at all,
// leaving its attrs as the only thing it can say. Used for
// horizontal_rule and metric (image and figma layer the src rule on
// top of this).
func validateContentless(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept content", b.Type))
	}

	return nil
}

// validateAtomWithSrc checks that the block carries no content and
// points somewhere. Used for image and figma.
func validateAtomWithSrc(b Block, path string) error {
	if err := validateContentless(b, path); err != nil {
		return err
	}

	return validateSrcAttrs(b.Type, b.Attrs, path)
}

// validateSrcAttrs checks that an image or figma block carries a src.
func validateSrcAttrs(t Type, attrs document.Attributes, path string) error {
	if a, ok := attrs.Get(document.AttrSrc); !ok || strings.TrimSpace(a.String()) == "" {
		return verr(joinPath(path, "attrs.src"), fmt.Sprintf("%s requires a non-empty src", t))
	}

	return nil
}

// validateMetricGrid checks that every item is a metric and there
// is at least one.
func validateMetricGrid(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "items"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "metric_grid does not accept text")
	}

	if len(b.Items) == 0 {
		return verr(path, "metric_grid requires at least one metric item")
	}

	for i, item := range b.Items {
		if item.Type != BlockMetric {
			return verr(
				joinPath(path, fmt.Sprintf("items[%d]", i)),
				fmt.Sprintf("metric_grid items must be metric, got %s", item.Type),
			)
		}

		if err := validateBlock(item, joinPath(path, fmt.Sprintf("items[%d]", i))); err != nil {
			return err
		}
	}

	return nil
}

// validateSplitDoc checks the split_doc shape: a level-1 heading
// leading the left side, body blocks before any param_lists, and
// titled_code/metric blocks on the right.
func validateSplitDoc(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "left", "right"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "split_doc does not accept text")
	}

	if len(b.Left) == 0 {
		return verr(joinPath(path, "left"), "split_doc requires at least one left block")
	}

	if len(b.Right) == 0 {
		return verr(joinPath(path, "right"), "split_doc requires at least one right block")
	}

	// First left block must be a heading at level 1; the rest may be
	// paragraphs, lists, callouts, or param_lists in that order
	// (heading first, then any number of body blocks, then any
	// number of param_lists).
	//
	// Level 1 mirrors what the web editor's split_doc.createAndFill produces
	// for a freshly inserted split_doc — the panel provides its own
	// visual emphasis, so the heading inside is always level 1
	// regardless of the surrounding outline depth.
	if b.Left[0].Type != BlockHeading {
		return verr(joinPath(path, "left[0]"), "split_doc left must begin with a heading")
	}

	if a, ok := b.Left[0].Attrs.Get(document.AttrLevel); !ok || a.Int() != 1 {
		return verr(
			joinPath(path, "left[0].attrs.level"),
			"split_doc left heading must be level 1",
		)
	}

	seenParamList := false

	for i, lb := range b.Left {
		if err := validateBlock(lb, joinPath(path, fmt.Sprintf("left[%d]", i))); err != nil {
			return err
		}

		if i == 0 {
			continue
		}

		if lb.Type == BlockParamList {
			seenParamList = true

			continue
		}

		if seenParamList {
			return verr(
				joinPath(path, fmt.Sprintf("left[%d]", i)),
				"non-param_list blocks must come before any param_list on the left side",
			)
		}

		if !_allowedSplitDocLeftBody[lb.Type] {
			return verr(
				joinPath(path, fmt.Sprintf("left[%d]", i)),
				fmt.Sprintf("split_doc left body must be one of %s, got %s",
					listAllowed(_allowedSplitDocLeftBody), lb.Type),
			)
		}
	}

	for i, rb := range b.Right {
		if err := validateBlock(rb, joinPath(path, fmt.Sprintf("right[%d]", i))); err != nil {
			return err
		}

		if !_allowedSplitDocRight[rb.Type] {
			return verr(
				joinPath(path, fmt.Sprintf("right[%d]", i)),
				fmt.Sprintf("split_doc right must be one of %s, got %s",
					listAllowed(_allowedSplitDocRight), rb.Type),
			)
		}
	}

	return nil
}

// validateParamList checks the header and per-row name
// requirements of a param_list.
func validateParamList(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "header", "params"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "param_list uses header and params, not text")
	}

	if strings.TrimSpace(b.Header) == "" {
		return verr(joinPath(path, "header"), "param_list requires a non-empty header")
	}

	if len(b.Params) == 0 {
		return verr(joinPath(path, "params"), "param_list requires at least one row")
	}

	for i, p := range b.Params {
		if strings.TrimSpace(p.Name) == "" {
			return verr(
				joinPath(path, fmt.Sprintf("params[%d]/name", i)),
				"param row requires a non-empty name",
			)
		}
	}

	return nil
}

// mustNotHaveCompoundFields rejects a block that sets any of the
// macro-only fields (TaskItems, Left, Right, Header, Params). Used
// by simple block types that should never see them.
func mustNotHaveCompoundFields(b Block, path string) error {
	if len(b.Children) != 0 {
		return verr(path, fmt.Sprintf(
			"%s does not accept children; only a list or task-list item's content block carries them",
			b.Type,
		))
	}

	if len(b.TaskItems) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept task_items", b.Type))
	}

	if len(b.Left) != 0 || len(b.Right) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept left/right", b.Type))
	}

	if b.Header != "" || len(b.Params) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept header/params", b.Type))
	}

	return nil
}

// mustHaveNoCanonicalCompoundExcept enforces that only the listed
// compound fields are populated. Each name is one of "items",
// "task_items", "left", "right", "header", "params".
func mustHaveNoCanonicalCompoundExcept(b Block, path string, allowed ...string) error {
	allow := map[string]bool{}
	for _, a := range allowed {
		allow[a] = true
	}

	if !allow["items"] && len(b.Items) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept items", b.Type))
	}

	if !allow["task_items"] && len(b.TaskItems) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept task_items", b.Type))
	}

	if !allow["left"] && len(b.Left) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept left", b.Type))
	}

	if !allow["right"] && len(b.Right) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept right", b.Type))
	}

	if !allow["header"] && b.Header != "" {
		return verr(path, fmt.Sprintf("%s does not accept header", b.Type))
	}

	if !allow["params"] && len(b.Params) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept params", b.Type))
	}

	return nil
}

// validateListItems checks the content blocks of a list or task list.
// An item's own children — a nested list, in practice — are validated
// separately, since every other position rejects them outright.
func validateListItems(items []Block, path string) error {
	for i, item := range items {
		itemPath := joinPath(path, fmt.Sprintf("[%d]", i))

		children := item.Children
		item.Children = nil

		if err := validateListEntry(item.Type, itemPath); err != nil {
			return err
		}

		if err := validateBlock(item, itemPath); err != nil {
			return err
		}

		if err := validateItemsAllowed(children, joinPath(itemPath, "children"), _allowedListItemContent); err != nil {
			return err
		}
	}

	return nil
}

// validateListEntry checks the block a list or task-list entry leads
// with. The editor's ListItem and nested TaskItem both hold
// "paragraph block*", so the entry is the paragraph and anything else
// belongs under it — expanding a list as the entry would produce an
// item with no leading paragraph, which the schema does not admit.
func validateListEntry(t Type, path string) error {
	if t != BlockParagraph {
		return verr(path, fmt.Sprintf(
			"a list entry is a %s, got %s; nest other blocks under the entry with children",
			BlockParagraph, t,
		))
	}

	return nil
}

// validateItemsAllowed validates a slice of items, requiring each
// to be of a type in allowed.
func validateItemsAllowed(items []Block, path string, allowed map[Type]bool) error {
	for i, item := range items {
		itemPath := joinPath(path, fmt.Sprintf("[%d]", i))
		if !allowed[item.Type] {
			return verr(itemPath, fmt.Sprintf("type %s not allowed here; expected one of %s",
				item.Type, listAllowed(allowed)))
		}

		if err := validateBlock(item, itemPath); err != nil {
			return err
		}
	}

	return nil
}

// listAllowed returns a sorted comma-separated list of the keys in
// the allowed-set, for use in error messages.
func listAllowed(m map[Type]bool) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, string(k))
	}

	// Sort so error messages are deterministic for tests.
	slices.Sort(names)

	return strings.Join(names, ", ")
}

// verr is a tiny validationError constructor.
func verr(path, message string) error {
	return &validationError{Path: path, Message: message}
}

// joinPath concatenates a parent breadcrumb with a child segment.
// When the child starts with '[' (an index suffix) the segments
// join without a separator; otherwise a '/' separator is used.
func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}

	if strings.HasPrefix(child, "[") {
		return parent + child
	}

	return parent + "/" + child
}
