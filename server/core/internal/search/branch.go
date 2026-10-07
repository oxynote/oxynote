package search

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/blevesearch/bleve/v2"
	bleveSearch "github.com/blevesearch/bleve/v2/search"
	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

const (
	// BranchLimitDefault is the number of hits a branch page holds when
	// the request names no limit.
	BranchLimitDefault = 20

	// BranchLimitMax caps the number of hits one branch page holds.
	BranchLimitMax = 50
)

// BranchQuery describes one page of the hits of one branch.
type BranchQuery struct {
	// OrganizationID specifies the organization searched.
	OrganizationID string

	// BranchID specifies the branch searched.
	BranchID xid.ID

	// Query specifies the text searched for.
	Query string

	// Limit specifies the number of hits on the page, see BranchLimit.
	Limit int

	// PageToken specifies the page, empty for the first one. A group's
	// NextHitsToken continues after the hits the group shows.
	PageToken string
}

// fingerprint identifies the search a page token belongs to. A token
// replayed with another query or branch would point into another order.
func (bq BranchQuery) fingerprint() string {
	h := fnv.New64a()
	h.Write([]byte(bq.Query + "\x00" + bq.BranchID.String())) //nolint:gosec // hash writes never fail

	return strconv.FormatUint(h.Sum64(), 36)
}

// BranchPage is one page of the hits of one branch.
type BranchPage struct {
	// Hits specifies the page's matching content blocks, best first.
	// Their text is HTML with the matched terms wrapped in mark tags.
	Hits []Block

	// TotalHits specifies the number of matching content blocks across
	// every page.
	TotalHits int

	// NextPageToken specifies the token of the next page, if any.
	NextPageToken null.String
}

// SearchBranch searches one branch's content entries and returns one page
// of them, highlighted. The hits come in the order SearchGroups shows a
// group's hits in, so a group's NextHitsToken continues the group.
func (i *Index) SearchBranch(ctx context.Context, bq BranchQuery) (BranchPage, error) {
	if err := validatePagedQuery(bq.Query); err != nil {
		return BranchPage{}, err
	}

	fingerprint := bq.fingerprint()

	offset, err := decodePageToken(bq.PageToken, fingerprint)
	if err != nil {
		return BranchPage{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, _searchTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, bleveSearch.MaxTermSearchersKey, _maxTermSearchers)

	branch := bleve.NewTermQuery(bq.BranchID.String())
	branch.SetField(_fieldBranchID)

	// a document name is a group's title, never one of its hits.
	name := bleve.NewTermQuery("document")
	name.SetField(_fieldType)

	q := bleve.NewBooleanQuery()
	q.AddMust(i.query(bq.OrganizationID, bq.Query), branch)
	q.AddMustNot(name)

	limit := BranchLimit(bq.Limit)

	req := bleve.NewSearchRequestOptions(q, limit, offset, false)
	req.Fields = []string{"*"}
	req.SortBy([]string{"-_score", "_id"})
	req.Highlight = bleve.NewHighlightWithStyle(_highlighterName)
	req.Highlight.AddField(_fieldText)

	res, err := i.idx.SearchInContext(ctx, req)
	if err != nil {
		return BranchPage{}, fmt.Errorf("searching branch: %w", err)
	}

	page := BranchPage{
		Hits:      make([]Block, 0, len(res.Hits)),
		TotalHits: int(res.Total),
	}

	for _, hit := range res.Hits {
		b, err := i.decodeHighlightedHit(hit)
		if err != nil {
			return BranchPage{}, err
		}

		page.Hits = append(page.Hits, b)
	}

	if end := offset + len(res.Hits); end < page.TotalHits {
		page.NextPageToken = null.StringFrom(encodePageToken(end, fingerprint))
	}

	return page, nil
}

// BranchLimit returns the page size for the requested limit: the default
// when none is set, and at most BranchLimitMax.
func BranchLimit(limit int) int {
	if limit <= 0 {
		return BranchLimitDefault
	}

	return min(limit, BranchLimitMax)
}
