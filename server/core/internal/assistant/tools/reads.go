package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/aiblock"
	"github.com/oxynote/purse/util/errutil"
	"github.com/rs/xid"
)

// _searchLimitDefault is the cap applied when search_documents is
// called without a limit argument.
const _searchLimitDefault = 20

// _searchLimitMax is the hard cap; larger requested values are
// silently clipped so the AI can't accidentally page through
// thousands of rows.
const _searchLimitMax = 50

// docTreeNode is the shape returned by list_documents. It mirrors
// document.Summary but uses snake_case keys so the AI consumes a
// consistent vocabulary with the rest of the tool surface.
type docTreeNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Icon     string         `json:"icon"`
	Children []docTreeNode  `json:"children,omitempty"`
}

func (m *Manager) listDocuments(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		ParentID string `json:"parent_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("list_documents: invalid input: %w", err)
	}

	var (
		tree document.Summaries
		err  error
	)

	if in.ParentID == "" {
		tree, err = m.db.FetchDocumentTree(ctx, m.orgID)
	} else {
		parentID, perr := xid.FromString(in.ParentID)
		if perr != nil {
			return nil, fmt.Errorf("list_documents: parent_id is not a valid xid: %w", perr)
		}

		tree, err = m.db.FetchDocumentTreeByDocumentParentID(ctx, null.ValueFrom(parentID), m.orgID)
	}

	if err != nil {
		return nil, fmt.Errorf("list_documents: fetch tree: %w", err)
	}

	out := struct {
		Documents []docTreeNode `json:"documents"`
	}{
		Documents: summariesToTree(tree),
	}

	return marshalResult(out)
}

func (m *Manager) getDocument(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("get_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("get_document: document_id is not a valid xid: %w", err)
	}

	doc, err := m.db.FetchDocument(ctx, docID, m.orgID, document.DefaultBranch)
	if err != nil {
		return nil, fmt.Errorf("get_document: fetch: %w", err)
	}

	out := struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Icon       string `json:"icon"`
		ParentID   string `json:"parent_id,omitempty"`
		BranchID   string `json:"branch_id"`
		BranchName string `json:"branch_name"`
		Protected  bool   `json:"protected"`
		UpdatedAt  string `json:"updated_at"`
	}{
		ID:         doc.ID.String(),
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		BranchID:   doc.BranchID.String(),
		BranchName: doc.BranchName,
		Protected:  doc.Protected,
		UpdatedAt:  doc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}

	if doc.ParentID.Valid {
		out.ParentID = doc.ParentID.V.String()
	}

	return marshalResult(out)
}

func (m *Manager) readDocumentSummary(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("read_document_summary: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("read_document_summary: document_id is not a valid xid: %w", err)
	}

	content, err := m.db.FetchMainBranchContent(ctx, docID, m.orgID)
	if err != nil {
		return nil, fmt.Errorf("read_document_summary: fetch content: %w", err)
	}

	entries := walkDocForAssistant(content.Content.Content)

	out := struct {
		DocumentID   string            `json:"document_id"`
		DocumentName string            `json:"document_name"`
		Blocks       []docSummaryEntry `json:"blocks"`
	}{
		DocumentID:   docID.String(),
		DocumentName: content.DocumentName,
		Blocks:       entries,
	}

	return marshalResult(out)
}

func (m *Manager) readBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("read_block: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("read_block: document_id is not a valid xid: %w", err)
	}

	if in.BlockUID == "" {
		return nil, errors.New("read_block: block_uid is required")
	}

	content, err := m.db.FetchMainBranchContent(ctx, docID, m.orgID)
	if err != nil {
		return nil, fmt.Errorf("read_block: fetch content: %w", err)
	}

	block, ok := findBlockByUID(content.Content.Content, in.BlockUID)
	if !ok {
		return nil, errutil.ErrNotFound
	}

	canon, err := aiblock.Compact(block)
	if err != nil {
		return nil, fmt.Errorf("read_block: compact: %w", err)
	}

	return marshalResult(canon)
}

func (m *Manager) searchDocuments(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("search_documents: invalid input: %w", err)
	}

	if in.Query == "" {
		return nil, errors.New("search_documents: query is required")
	}

	limit := in.Limit
	if limit <= 0 {
		limit = _searchLimitDefault
	}

	if limit > _searchLimitMax {
		limit = _searchLimitMax
	}

	blocks, err := m.search.SearchDocumentBlocks(ctx, m.orgID, in.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("search_documents: search: %w", err)
	}

	// The index stores block text keyed by (document, block uid) but
	// not display names; join names in from the document tree so the
	// AI can talk about hits without a follow-up lookup per document.
	names := map[string]string{}

	if tree, err := m.db.FetchDocumentTree(ctx, m.orgID); err == nil {
		collectDocumentNames(tree, names)
	}

	hits := make([]searchHit, 0, len(blocks))

	for _, b := range blocks {
		hits = append(hits, searchHit{
			DocumentID:   b.DocumentID.String(),
			DocumentName: names[b.DocumentID.String()],
			BlockUID:     b.ID,
			Text:         b.Text,
		})
	}

	out := struct {
		Hits []searchHit `json:"hits"`
	}{
		Hits: hits,
	}

	return marshalResult(out)
}

// searchHit is one search_documents result row.
type searchHit struct {
	// DocumentID is the document containing the matching block.
	DocumentID string `json:"document_id"`

	// DocumentName is the document's display name, when resolvable
	// from the tree. Empty if the document vanished between the
	// index update and the search.
	DocumentName string `json:"document_name,omitempty"`

	// BlockUID is the matching block's uid attribute, usable with
	// read_block and the edit tools.
	BlockUID string `json:"block_uid"`

	// Text is the block's indexed text.
	Text string `json:"text"`
}

// collectDocumentNames flattens the summary tree into an id → name
// map.
func collectDocumentNames(ss document.Summaries, out map[string]string) {
	for _, s := range ss {
		out[s.ID.String()] = s.DocumentName

		collectDocumentNames(s.Children, out)
	}
}

// summariesToTree converts the document package's nested Summary
// tree into the snake_case shape returned by list_documents.
func summariesToTree(ss document.Summaries) []docTreeNode {
	if len(ss) == 0 {
		return nil
	}

	out := make([]docTreeNode, 0, len(ss))

	for _, s := range ss {
		out = append(out, docTreeNode{
			ID:       s.ID.String(),
			Name:     s.DocumentName,
			Icon:     s.Icon,
			Children: summariesToTree(s.Children),
		})
	}

	return out
}

// findBlockByUID walks the document tree depth-first and returns
// the block whose uid attribute equals target.
func findBlockByUID(blocks []document.Block, target string) (document.Block, bool) {
	for _, b := range blocks {
		if uid, ok := b.UID(); ok && uid == target {
			return b, true
		}

		if found, ok := findBlockByUID(b.Content, target); ok {
			return found, true
		}
	}

	return document.Block{}, false
}

// marshalResult is the single place tool result envelopes are
// serialised; centralising it keeps the JSON shape consistent.
func marshalResult(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal tool result: %w", err)
	}

	return data, nil
}
