package search

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"slices"
	"strconv"

	"github.com/blevesearch/bleve/v2"
	bleveSearch "github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

const (
	// GroupLimitDefault is the number of groups a page holds when the
	// request names no limit.
	GroupLimitDefault = 30

	// GroupLimitMax caps the number of groups one page holds.
	GroupLimitMax = 50

	// GroupHitsLimitDefault is the number of hits a group shows when the
	// request names no limit.
	GroupHitsLimitDefault = 3

	// GroupHitsLimitMax caps the number of hits one group shows.
	GroupHitsLimitMax = 10

	// _groupHitCap caps the hits one grouped search reads. The groups and
	// totals are built from these hits only.
	_groupHitCap = 1000

	// _maxPageOffset caps the offset a page token may carry. bleve adds the
	// offset to the page size unchecked, so a huge one overflows and
	// panics there.
	_maxPageOffset = 10_000
)

// ErrInvalidPageToken is returned when a page token is malformed or was
// issued for another search.
var ErrInvalidPageToken = errutil.New(http.StatusBadRequest, "document.invalid_search_page_token", "invalid search page token")

// GroupQuery describes one page of a grouped search.
type GroupQuery struct {
	// OrganizationID specifies the organization searched.
	OrganizationID string

	// Query specifies the text searched for.
	Query string

	// CurrentDocumentID specifies the document whose groups come first.
	CurrentDocumentID null.Value[xid.ID]

	// Limit specifies the number of groups on the page, see GroupLimit.
	Limit int

	// HitsLimit specifies the number of hits each group shows, see
	// GroupHitsLimit.
	HitsLimit int

	// PageToken specifies the page, empty for the first one.
	PageToken string
}

// fingerprint identifies the search a page token belongs to. A token
// replayed with another query or pin would point into another order.
func (gq GroupQuery) fingerprint() string {
	h := fnv.New64a()
	h.Write([]byte(gq.Query + "\x00" + gq.CurrentDocumentID.V.String())) //nolint:gosec // hash writes never fail

	return strconv.FormatUint(h.Sum64(), 36)
}

// Group is one branch of a document holding matches.
type Group struct {
	// DocumentID specifies the document the branch belongs to.
	DocumentID xid.ID

	// BranchID specifies the branch.
	BranchID xid.ID

	// Title specifies the highlighted document name, set only when the
	// name matched.
	Title null.String

	// Hits specifies the first matching content blocks, best first. Their
	// text is HTML with the matched terms wrapped in mark tags.
	Hits []Block

	// TotalHits specifies the number of matching content blocks in the
	// branch, counted like GroupPage.TotalHits.
	TotalHits int

	// NextHitsToken specifies the SearchBranch page token of the hits
	// after Hits, if any.
	NextHitsToken null.String
}

// GroupPage is one page of a grouped search.
type GroupPage struct {
	// Groups specifies the page's groups, best first.
	Groups []Group

	// TotalHits specifies the number of matching content blocks across
	// every page.
	TotalHits int

	// TotalDocuments specifies the number of groups across every page.
	TotalDocuments int

	// Capped indicates that more matches exist than the totals count.
	Capped bool

	// NextPageToken specifies the token of the next page, if any.
	NextPageToken null.String
}

// BranchIDs returns the ids of the page's branches.
func (gp GroupPage) BranchIDs() []xid.ID {
	ids := make([]xid.ID, 0, len(gp.Groups))

	for _, g := range gp.Groups {
		ids = append(ids, g.BranchID)
	}

	return ids
}

// pageToken is the decoded form of a page token.
type pageToken struct {
	// Offset specifies the index of the page's first group or hit.
	Offset int `json:"o"`

	// Fingerprint specifies the search the token belongs to.
	Fingerprint string `json:"f"`
}

// groupEntries collects the ids of one group's matching entries.
type groupEntries struct {
	// documentID specifies the document the branch belongs to.
	documentID xid.ID

	// branchID specifies the branch.
	branchID xid.ID

	// titleID specifies the id of the matching document name entry, if
	// any.
	titleID string

	// hitIDs specifies the ids of the matching content entries, best
	// first.
	hitIDs []string
}

// SearchGroups searches the organization's entries and returns one page
// of them grouped by branch. Groups are ordered by their best match, with
// the current document's groups first. A matching document name becomes
// the group's title instead of a hit. Each group shows its first hits
// only; SearchBranch pages through the rest.
//
// Grouping needs every match, which bleve cannot do in the index, so
// the first _groupHitCap matches are read and grouped here on every
// request. A page token is an offset into that order. Only the page's
// own entries are then read in full and highlighted.
func (i *Index) SearchGroups(ctx context.Context, gq GroupQuery) (GroupPage, error) {
	if err := ValidateQuery(gq.Query); err != nil {
		return GroupPage{}, err
	}

	fingerprint := gq.fingerprint()

	offset, err := decodePageToken(gq.PageToken, fingerprint)
	if err != nil {
		return GroupPage{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, _searchTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, bleveSearch.MaxTermSearchersKey, _maxTermSearchers)

	q := i.query(gq.OrganizationID, gq.Query)

	req := bleve.NewSearchRequestOptions(q, _groupHitCap, 0, false)
	req.Fields = []string{_fieldDocumentID, _fieldBranchID, _fieldType}
	req.SortBy([]string{"-_score", "_id"})

	res, err := i.idx.SearchInContext(ctx, req)
	if err != nil {
		return GroupPage{}, fmt.Errorf("searching documents: %w", err)
	}

	groups, err := groupHits(res.Hits)
	if err != nil {
		return GroupPage{}, err
	}

	if gq.CurrentDocumentID.Valid {
		// a stable sort keeps both parts in their match order.
		slices.SortStableFunc(groups, func(a, b *groupEntries) int {
			ac := a.documentID == gq.CurrentDocumentID.V
			bc := b.documentID == gq.CurrentDocumentID.V

			switch {
			case ac && !bc:
				return -1
			case bc && !ac:
				return 1
			default:
				return 0
			}
		})
	}

	page := GroupPage{
		TotalDocuments: len(groups),
		Capped:         res.Total > uint64(len(res.Hits)),
	}

	for _, g := range groups {
		page.TotalHits += len(g.hitIDs)
	}

	limit := GroupLimit(gq.Limit)
	start := min(offset, len(groups))
	end := min(start+limit, len(groups))

	if end < len(groups) {
		page.NextPageToken = null.StringFrom(encodePageToken(end, fingerprint))
	}

	page.Groups, err = i.highlightGroups(ctx, gq, q, groups[start:end])
	if err != nil {
		return GroupPage{}, err
	}

	return page, nil
}

// highlightGroups reads the groups' title and first hit entries in full,
// highlighted against the query. An entry removed since the groups were
// built is left out.
func (i *Index) highlightGroups(ctx context.Context, gq GroupQuery, q query.Query, groups []*groupEntries) ([]Group, error) {
	res := make([]Group, 0, len(groups))
	hitsLimit := GroupHitsLimit(gq.HitsLimit)

	var ids []string

	for _, g := range groups {
		if g.titleID != "" {
			ids = append(ids, g.titleID)
		}

		ids = append(ids, g.hitIDs[:min(hitsLimit, len(g.hitIDs))]...)
	}

	if len(ids) == 0 {
		return res, nil
	}

	req := bleve.NewSearchRequestOptions(
		bleve.NewConjunctionQuery(q, bleve.NewDocIDQuery(ids)),
		len(ids),
		0,
		false,
	)
	req.Fields = []string{"*"}
	req.Highlight = bleve.NewHighlightWithStyle(_highlighterName)
	req.Highlight.AddField(_fieldText)

	sr, err := i.idx.SearchInContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("highlighting documents: %w", err)
	}

	blocks := make(map[string]Block, len(sr.Hits))

	for _, hit := range sr.Hits {
		b, err := i.decodeHighlightedHit(hit)
		if err != nil {
			return nil, err
		}

		blocks[hit.ID] = b
	}

	for _, g := range groups {
		grp := Group{
			DocumentID: g.documentID,
			BranchID:   g.branchID,
			Hits:       []Block{},
			TotalHits:  len(g.hitIDs),
		}

		if b, ok := blocks[g.titleID]; ok {
			grp.Title = null.StringFrom(b.Text)
		}

		for _, id := range g.hitIDs[:min(hitsLimit, len(g.hitIDs))] {
			if b, ok := blocks[id]; ok {
				grp.Hits = append(grp.Hits, b)
			}
		}

		if len(g.hitIDs) > hitsLimit {
			bq := BranchQuery{
				BranchID: g.branchID,
				Query:    gq.Query,
			}

			grp.NextHitsToken = null.StringFrom(encodePageToken(hitsLimit, bq.fingerprint()))
		}

		res = append(res, grp)
	}

	return res, nil
}

// GroupLimit returns the page size for the requested limit: the default
// when none is set, and at most GroupLimitMax.
func GroupLimit(limit int) int {
	if limit <= 0 {
		return GroupLimitDefault
	}

	return min(limit, GroupLimitMax)
}

// GroupHitsLimit returns the number of hits a group shows for the
// requested limit: the default when none is set, and at most
// GroupHitsLimitMax.
func GroupHitsLimit(limit int) int {
	if limit <= 0 {
		return GroupHitsLimitDefault
	}

	return min(limit, GroupHitsLimitMax)
}

// decodeHighlightedHit rebuilds an entry from a highlighted hit. Its text
// is the best fragment, or the full text escaped like a fragment is when
// the hit has none.
func (i *Index) decodeHighlightedHit(hit *bleveSearch.DocumentMatch) (Block, error) {
	b, err := i.decodeHit(hit.ID, hit.Fields)
	if err != nil {
		return Block{}, err
	}

	b.Text = html.EscapeString(b.Text)

	if fragments := hit.Fragments[_fieldText]; len(fragments) != 0 {
		b.Text = fragments[0]
	}

	return b, nil
}

// encodePageToken encodes the offset of a page of the search.
func encodePageToken(offset int, fingerprint string) string {
	data, err := json.Marshal(pageToken{
		Offset:      offset,
		Fingerprint: fingerprint,
	})
	if err != nil {
		// NOCOV: a struct of an int and a string always marshals.
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(data)
}

// decodePageToken returns the offset a page token points at. An empty
// token is the first page.
func decodePageToken(token, fingerprint string) (int, error) {
	if token == "" {
		return 0, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidPageToken
	}

	var pt pageToken

	if err := json.Unmarshal(data, &pt); err != nil {
		return 0, ErrInvalidPageToken
	}

	if pt.Offset < 0 || pt.Offset > _maxPageOffset || pt.Fingerprint != fingerprint {
		return 0, ErrInvalidPageToken
	}

	return pt.Offset, nil
}

// groupHits groups the hits by branch, in the order each branch first
// appears.
func groupHits(hits bleveSearch.DocumentMatchCollection) ([]*groupEntries, error) {
	var groups []*groupEntries

	byBranch := make(map[string]*groupEntries)

	for _, hit := range hits {
		branch, _ := hit.Fields[_fieldBranchID].(string)

		g, ok := byBranch[branch]
		if !ok {
			doc, _ := hit.Fields[_fieldDocumentID].(string)

			documentID, err := xid.FromString(doc)
			if err != nil {
				return nil, fmt.Errorf("decoding document id of entry %s: %w", hit.ID, err)
			}

			branchID, err := xid.FromString(branch)
			if err != nil {
				return nil, fmt.Errorf("decoding branch id of entry %s: %w", hit.ID, err)
			}

			g = &groupEntries{
				documentID: documentID,
				branchID:   branchID,
			}

			byBranch[branch] = g
			groups = append(groups, g)
		}

		if typ, _ := hit.Fields[_fieldType].(string); typ == "document" {
			g.titleID = hit.ID
			continue
		}

		g.hitIDs = append(g.hitIDs, hit.ID)
	}

	return groups, nil
}
