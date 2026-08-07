package aiblock

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError is the structured error returned by Validate.
// Path locates the offending block within a nested canonical tree
// using a slash-separated breadcrumb (e.g. "split_doc/left[0]",
// "param_list/params[2]"); Message describes the rule that failed.
type ValidationError struct {
	// Path is a slash-separated breadcrumb identifying the offending
	// node. Indices use bracketed integers (e.g. "items[3]").
	Path string

	// Message is a single-sentence description of the violated rule.
	Message string
}

// Error renders the validation error in the form
// "{path}: {message}".
func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}

	return e.Path + ": " + e.Message
}

// Validate checks that a canonical Block satisfies every constraint
// Expand and the editor schema require. It is intended to be run on
// AI-supplied input before Expand. Validate stops at the first
// violation and returns a ValidationError describing it; callers
// can surface that message to the AI to recover precisely.
//
// A nil error means Expand will produce a structurally valid
// document.Block tree (subject to runtime constraints like the
// referenced uid existing, which only matter once the operation
// reaches hocuspocus).
func Validate(b Block) error {
	return validateBlock(b, "")
}

// ValidateMany validates each block in a slice, prefixing errors
// with their index.
func ValidateMany(blocks []Block) error {
	for i, b := range blocks {
		if err := validateBlock(b, fmt.Sprintf("[%d]", i)); err != nil {
			return err
		}
	}

	return nil
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

	if !_allowedAtRoot[b.Type] {
		return verr("", fmt.Sprintf(
			"%s is not allowed at the document root; it must appear inside %s",
			b.Type, containerForType(b.Type),
		))
	}

	return nil
}

// containerForType returns a human-readable description of the
// macro that must contain a non-root block type. Used to build
// helpful error messages for ValidateAsRoot.
func containerForType(t BlockType) string {
	switch t {
	case BlockTitledCode:
		return "split_doc.right"
	case BlockMetric:
		return "metric_grid or split_doc.right"
	case BlockParamList:
		return "split_doc.left"
	}

	return "its parent macro"
}

// validateBlock dispatches on b.Type. Path is the breadcrumb to b
// from the validation root (passed in by recursive calls).
func validateBlock(b Block, path string) error {
	switch b.Type {
	case "":
		return verr(path, "block type is required")
	case BlockParagraph:
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
	case BlockCode:
		return validateCode(b, path)
	case BlockTitledCode:
		return validateTitledCode(b, path)
	case BlockMermaid:
		return validateRawText(b, path)
	case BlockHorizontalRule:
		return validateHorizontalRule(b, path)
	case BlockImage:
		return validateImage(b, path)
	case BlockFigma:
		return validateFigma(b, path)
	case BlockMetric:
		return validateMetric(b, path)
	case BlockMetricGrid:
		return validateMetricGrid(b, path)
	case BlockSplitDoc:
		return validateSplitDoc(b, path)
	case BlockParamList:
		return validateParamList(b, path)
	}

	return verr(path, fmt.Sprintf("unknown block type %q", b.Type))
}

// validateTextBearing checks that the block carries Text content
// and no compound-only fields. Used for paragraph (heading and
// blockquote layer their own rules on top of this).
func validateTextBearing(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept items", b.Type))
	}

	return validateInlineMarkdown(b.Text, joinPath(path, "text"))
}

func validateHeading(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, "heading does not accept items")
	}

	level := readIntAttr(b.Attrs, _attrLevel, 0)
	if level < 1 || level > 3 {
		return verr(joinPath(path, "attrs.level"), "heading level must be 1, 2, or 3")
	}

	return validateInlineMarkdown(b.Text, joinPath(path, "text"))
}

func validateBlockquote(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" && len(b.Items) > 0 {
		return verr(path, "blockquote accepts either text or items, not both")
	}

	if b.Text == "" && len(b.Items) == 0 {
		return verr(path, "blockquote requires text or items")
	}

	if b.Text != "" {
		return validateInlineMarkdown(b.Text, joinPath(path, "text"))
	}

	return validateItemsAllowed(b.Items, joinPath(path, "items"), _allowedBlockquoteItems)
}

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

	return validateItemsAllowed(b.Items, joinPath(path, "items"), _allowedListItemContent)
}

func validateTaskList(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "task_items"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "task_list does not accept text")
	}

	if len(b.Items) != 0 {
		return verr(path, "task_list rows live in task_items, not items")
	}

	if len(b.TaskItems) == 0 {
		return verr(path, "task_list requires at least one task_item")
	}

	for i, ti := range b.TaskItems {
		itemPath := joinPath(path, fmt.Sprintf("task_items[%d]", i))
		if err := validateBlock(ti.Block, joinPath(itemPath, "block")); err != nil {
			return err
		}

		if !_allowedListItemContent[ti.Block.Type] {
			return verr(joinPath(itemPath, "block"),
				fmt.Sprintf("task_item content must be one of %s, got %s", listAllowed(_allowedListItemContent), ti.Block.Type),
			)
		}
	}

	return nil
}

func validateCallout(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "items"); err != nil {
		return err
	}

	if b.Text != "" && len(b.Items) > 0 {
		return verr(path, "callout accepts either text or items, not both")
	}

	if b.Text == "" && len(b.Items) == 0 {
		return verr(path, "callout requires text or items")
	}

	if b.Text != "" {
		return validateInlineMarkdown(b.Text, joinPath(path, "text"))
	}

	return validateItemsAllowed(b.Items, joinPath(path, "items"), _allowedCalloutItems)
}

func validateCode(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, "code does not accept items")
	}

	return nil
}

func validateTitledCode(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, "titled_code does not accept items")
	}

	title := readStringAttr(b.Attrs, _attrTitle, "")
	if strings.TrimSpace(title) == "" {
		return verr(joinPath(path, "attrs.title"), "titled_code requires a non-empty title")
	}

	return nil
}

func validateRawText(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if len(b.Items) != 0 {
		return verr(path, fmt.Sprintf("%s does not accept items", b.Type))
	}

	return nil
}

func validateHorizontalRule(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 {
		return verr(path, "horizontal_rule does not accept content")
	}

	return nil
}

func validateImage(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 {
		return verr(path, "image does not accept content")
	}

	src := readStringAttr(b.Attrs, _attrSrc, "")
	if strings.TrimSpace(src) == "" {
		return verr(joinPath(path, "attrs.src"), "image requires a non-empty src")
	}

	return nil
}

func validateFigma(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 {
		return verr(path, "figma does not accept content")
	}

	src := readStringAttr(b.Attrs, _attrSrc, "")
	if strings.TrimSpace(src) == "" {
		return verr(joinPath(path, "attrs.src"), "figma requires a non-empty src")
	}

	return nil
}

func validateMetric(b Block, path string) error {
	if err := mustNotHaveCompoundFields(b, path); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 {
		return verr(path, "metric does not accept content")
	}

	return nil
}

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

func validateSplitDoc(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "left", "right"); err != nil {
		return err
	}

	if b.Text != "" {
		return verr(path, "split_doc does not accept text")
	}

	if len(b.Items) != 0 || len(b.TaskItems) != 0 {
		return verr(path, "split_doc uses left and right, not items")
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

	if level := readIntAttr(b.Left[0].Attrs, _attrLevel, 0); level != 1 {
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

func validateParamList(b Block, path string) error {
	if err := mustHaveNoCanonicalCompoundExcept(b, path, "header", "params"); err != nil {
		return err
	}

	if b.Text != "" || len(b.Items) != 0 || len(b.TaskItems) != 0 {
		return verr(path, "param_list uses header and params, not text or items")
	}

	if strings.TrimSpace(b.Header) == "" {
		return verr(joinPath(path, "header"), "param_list requires a non-empty header")
	}

	if len(b.Params) == 0 {
		return verr(joinPath(path, "params"), "param_list requires at least one row")
	}

	for i, p := range b.Params {
		itemPath := joinPath(path, fmt.Sprintf("params[%d]", i))

		if strings.TrimSpace(p.Name) == "" {
			return verr(joinPath(itemPath, "name"), "param row requires a non-empty name")
		}

		if err := validateInlineMarkdown(p.Description, joinPath(itemPath, "description")); err != nil {
			return err
		}
	}

	return nil
}

// validateInlineMarkdown checks that the input can be parsed without
// landing in an unrecoverable state. Today's parser is tolerant
// (unmatched delimiters become literal text), so the only true
// failure mode is empty text on a required field — which the caller
// checks separately. The function is kept as a hook so we can add
// stricter rules later without changing the callsites.
func validateInlineMarkdown(text, path string) error {
	_ = text
	_ = path

	return nil
}

// mustNotHaveCompoundFields rejects a block that sets any of the
// macro-only fields (TaskItems, Left, Right, Header, Params). Used
// by simple block types that should never see them.
func mustNotHaveCompoundFields(b Block, path string) error {
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

// validateItemsAllowed validates a slice of items, requiring each
// to be of a type in allowed.
func validateItemsAllowed(items []Block, path string, allowed map[BlockType]bool) error {
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

// Allowed child-type sets, mirroring the editor's schema rules.
var (
	// _allowedBlockquoteItems mirrors TipTap's default Blockquote
	// content (block+); we accept the same set of simple blocks
	// the AI is likely to nest there.
	_allowedBlockquoteItems = map[BlockType]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
	}

	// _allowedListItemContent is the set of block types that can sit
	// inside a list/task-list item. Mirrors the web editor's
	// ListItem default content.
	_allowedListItemContent = map[BlockType]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
	}

	// _allowedCalloutItems mirrors CalloutBlock.content in
	// web/app/components/editor/blocks/callout/index.ts.
	_allowedCalloutItems = map[BlockType]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
	}

	// _allowedSplitDocLeftBody is the set of types that may appear
	// between the leading heading and any trailing param_lists on
	// a split_doc left side.
	_allowedSplitDocLeftBody = map[BlockType]bool{
		BlockParagraph:   true,
		BlockBulletList:  true,
		BlockOrderedList: true,
		BlockTaskList:    true,
		BlockCallout:     true,
	}

	// _allowedSplitDocRight is the set of types that may appear on a
	// split_doc right side.
	_allowedSplitDocRight = map[BlockType]bool{
		BlockTitledCode: true,
		BlockMetric:     true,
	}

	// _allowedAtRoot is the set of canonical block types that may
	// sit directly under the document root. Types not in this set
	// use custom TipTap groups
	// (splitDocumentationParameterList, metricBlock,
	// titledCodeBlock) and only become valid inside their containing
	// macro.
	_allowedAtRoot = map[BlockType]bool{
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

// listAllowed returns a sorted comma-separated list of the keys in
// the allowed-set, for use in error messages.
func listAllowed(m map[BlockType]bool) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, string(k))
	}

	// Sort so error messages are deterministic for tests.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}

	return strings.Join(names, ", ")
}

// verr is a tiny ValidationError constructor.
func verr(path, message string) error {
	return &ValidationError{Path: path, Message: message}
}

// IsValidationError reports whether err is a *ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError

	return errors.As(err, &ve)
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
