package search

import (
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// Response is the search endpoint's answer: one page of document
// branches with the blocks that matched in each.
type Response struct {
	// Total specifies the counts across every page.
	Total ResponseTotal `json:"total"`

	// NextToken specifies the token of the next page, if any.
	NextToken null.String `json:"nextToken"`

	// Results specifies the page's document branches, best first.
	Results []Result `json:"results"`
}

// NewResponse joins a page of groups with the rows of their branches. A
// group whose branch has no row is left out, since the index trails the
// database.
func NewResponse(page GroupPage, docs []document.Document) Response {
	byBranch := make(map[xid.ID]document.Document, len(docs))

	for _, doc := range docs {
		byBranch[doc.BranchID] = doc
	}

	res := Response{
		Total: ResponseTotal{
			Hits:      page.TotalHits,
			Documents: page.TotalDocuments,
			Capped:    page.Capped,
		},
		NextToken: page.NextPageToken,
		Results:   make([]Result, 0, len(page.Groups)),
	}

	for _, g := range page.Groups {
		doc, ok := byBranch[g.BranchID]
		if !ok {
			continue
		}

		r := Result{
			Document:      newResultDocument(doc),
			Hits:          make([]ResultHit, 0, len(g.Hits)),
			TotalHits:     g.TotalHits,
			NextHitsToken: g.NextHitsToken,
		}

		r.Document.TitleHTML = g.Title

		for _, b := range g.Hits {
			r.Hits = append(r.Hits, newResultHit(b))
		}

		res.Results = append(res.Results, r)
	}

	return res
}

// ResponseTotal holds the counts across every page of a search.
type ResponseTotal struct {
	// Hits specifies the number of matching blocks, document names
	// excluded.
	Hits int `json:"hits"`

	// Documents specifies the number of document branches.
	Documents int `json:"documents"`

	// Capped indicates that more matches exist than the counts include.
	Capped bool `json:"capped"`
}

// Result is one document branch with the blocks that matched in it.
type Result struct {
	// Document specifies the document branch.
	Document ResultDocument `json:"document"`

	// Hits specifies the first matching blocks, best first.
	Hits []ResultHit `json:"hits"`

	// TotalHits specifies the number of matching blocks in the branch.
	TotalHits int `json:"totalHits"`

	// NextHitsToken specifies the token of the branch search page after
	// Hits, if any.
	NextHitsToken null.String `json:"nextHitsToken"`
}

// BranchResponse is the branch search endpoint's answer: one page of the
// blocks that matched in one branch.
type BranchResponse struct {
	// TotalHits specifies the number of matching blocks across every page.
	TotalHits int `json:"totalHits"`

	// NextToken specifies the token of the next page, if any.
	NextToken null.String `json:"nextToken"`

	// Hits specifies the page's matching blocks, best first.
	Hits []ResultHit `json:"hits"`
}

// NewBranchResponse describes a page of one branch's hits.
func NewBranchResponse(page BranchPage) BranchResponse {
	res := BranchResponse{
		TotalHits: page.TotalHits,
		NextToken: page.NextPageToken,
		Hits:      make([]ResultHit, 0, len(page.Hits)),
	}

	for _, b := range page.Hits {
		res.Hits = append(res.Hits, newResultHit(b))
	}

	return res
}

// ResultDocument describes the document branch of a result.
type ResultDocument struct {
	// ID specifies the document's id.
	ID xid.ID `json:"id"`

	// Title specifies the document's name on the branch.
	Title string `json:"title"`

	// TitleHTML specifies the highlighted name, set only when it matched.
	TitleHTML null.String `json:"titleHtml"`

	// Icon specifies the document's icon on the branch.
	Icon string `json:"icon"`

	// Branch specifies the branch.
	Branch ResultBranch `json:"branch"`

	// UpdatedAt specifies when the branch was last updated.
	UpdatedAt time.Time `json:"updatedAt"`

	// UpdatedBy specifies the user who last updated the branch.
	UpdatedBy null.String `json:"updatedBy"`
}

// newResultDocument describes the document branch.
func newResultDocument(doc document.Document) ResultDocument {
	return ResultDocument{
		ID:    doc.ID,
		Title: doc.DocumentName,
		Icon:  doc.Icon,
		Branch: ResultBranch{
			ID:      doc.BranchID,
			Name:    doc.BranchName,
			Default: doc.Default,
		},
		UpdatedAt: doc.UpdatedAt,
		UpdatedBy: doc.LastUpdatedBy,
	}
}

// ResultBranch describes the branch of a result.
type ResultBranch struct {
	// ID specifies the branch's id.
	ID xid.ID `json:"id"`

	// Name specifies the branch's name.
	Name string `json:"name"`

	// Default indicates whether the branch is the document's default.
	Default bool `json:"default"`
}

// ResultHit is one matching block of a result.
type ResultHit struct {
	// ID specifies the block's uid.
	ID string `json:"id"`

	// Type specifies the block's node type.
	Type string `json:"type"`

	// Text specifies the block's text as HTML, with the matched terms
	// wrapped in mark tags.
	Text string `json:"text"`

	// Attrs specifies the block's attributes kept for its type, if any.
	Attrs map[string]string `json:"attrs,omitempty"`
}

// newResultHit describes the matching block.
func newResultHit(b Block) ResultHit {
	return ResultHit{
		ID:    b.UID(),
		Type:  b.Type,
		Text:  b.Text,
		Attrs: b.Attrs,
	}
}
