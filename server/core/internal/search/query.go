package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2"
	bleveSearch "github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

const (
	// MaxQueryLength caps a search query in characters. Every term
	// expands to a dictionary walk inside the process, so the query size
	// bounds the work one request can ask for.
	MaxQueryLength = 200

	// _minPagedQueryLength is the fewest characters a query of the paged
	// searches holds, surrounding spaces excluded. Shorter terms match
	// too much of the dictionary to rank.
	_minPagedQueryLength = 2

	// _searchResultLimit caps the number of hits the search endpoint
	// returns.
	_searchResultLimit = 30

	// _searchTimeout bounds one search request.
	_searchTimeout = 10 * time.Second

	// _phraseBoost lifts an entry holding the whole query as a phrase
	// above one holding the same words scattered.
	_phraseBoost = 2

	// _documentNameBoost lifts an entry carrying a document name.
	_documentNameBoost = 3

	// _headingBoost lifts a heading above the other block types.
	_headingBoost = 2

	// _defaultBranchBoost lifts an entry on the document's default branch
	// above the same entry on a fork.
	_defaultBranchBoost = 1.5

	// _maxQueryTerms is how many terms of a query are searched; the rest
	// are dropped. Every term costs a dictionary walk for its fuzzy and
	// prefix candidates.
	_maxQueryTerms = 16

	// _maxTermSearchers caps the terms one search expands to across its
	// whole query tree, fuzzy and prefix candidates included, so a query
	// over a large vocabulary fails instead of pinning the process.
	_maxTermSearchers = 1024
)

// ErrInvalidQuery is returned when a search query is empty or over
// MaxQueryLength, or when a paged search query is too short.
var ErrInvalidQuery = errutil.New(http.StatusBadRequest, "document.invalid_search_query", "invalid search query")

// ValidateQuery reports whether the query is one the index will run.
func ValidateQuery(q string) error {
	if q == "" || utf8.RuneCountInString(q) > MaxQueryLength {
		return ErrInvalidQuery
	}

	return nil
}

// validatePagedQuery reports whether the query is one the paged searches
// run. On top of ValidateQuery, they refuse one under
// _minPagedQueryLength.
func validatePagedQuery(q string) error {
	if utf8.RuneCountInString(strings.TrimSpace(q)) < _minPagedQueryLength {
		return ErrInvalidQuery
	}

	return ValidateQuery(q)
}

// SearchDocuments searches the organization's entries and returns them as
// the search endpoint's JSON: the matched terms wrapped in mark tags and
// the text cropped to a fragment around the best match.
func (i *Index) SearchDocuments(ctx context.Context, organizationID, q string) ([]byte, error) {
	blocks, err := i.search(ctx, organizationID, q, _searchResultLimit, true)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(blocks)
	if err != nil {
		return nil, fmt.Errorf("marshaling search results: %w", err)
	}

	return data, nil
}

// SearchDocumentBlocks returns the raw matching blocks for the query,
// scoped to the organization. Unlike SearchDocuments it returns plain
// block values (no highlight markup or cropping) so callers, the AI
// assistant's search tool, can consume the text directly.
func (i *Index) SearchDocumentBlocks(ctx context.Context, organizationID, q string, limit int) ([]Block, error) {
	return i.search(ctx, organizationID, q, limit, false)
}

// search runs the query and decodes the hits.
func (i *Index) search(ctx context.Context, organizationID, q string, limit int, highlight bool) ([]Block, error) {
	if err := ValidateQuery(q); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, _searchTimeout)
	defer cancel()

	ctx = context.WithValue(ctx, bleveSearch.MaxTermSearchersKey, _maxTermSearchers)

	req := bleve.NewSearchRequestOptions(i.query(organizationID, q), limit, 0, false)
	req.Fields = []string{"*"}

	if highlight {
		req.Highlight = bleve.NewHighlightWithStyle(_highlighterName)
		req.Highlight.AddField(_fieldText)
	}

	res, err := i.idx.SearchInContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("searching documents: %w", err)
	}

	blocks := make([]Block, 0, len(res.Hits))

	for _, hit := range res.Hits {
		b, err := i.decodeHit(hit.ID, hit.Fields)
		if err != nil {
			return nil, err
		}

		// a hit reached only through a synonym has no matched term of its
		// own to fragment around, so it keeps its full text.
		if fragments := hit.Fragments[_fieldText]; highlight && len(fragments) != 0 {
			b.Text = fragments[0]
		}

		blocks = append(blocks, b)
	}

	return blocks, nil
}

// query composes the search: an entry must belong to the organization and
// match the text, as the whole phrase or by its terms; a document name, a
// heading and the default branch then add score on top.
func (i *Index) query(organizationID, q string) query.Query {
	org := bleve.NewTermQuery(organizationID)
	org.SetField(_fieldOrganizationID)

	var text query.Query = bleve.NewMatchNoneQuery()

	// the analyzer is the field's own, so the terms run over the stemmed
	// vocabulary the index holds.
	if terms := i.terms(q); len(terms) != 0 {
		phrase := bleve.NewPhraseQuery(terms, _fieldText)
		phrase.SetBoost(_phraseBoost)

		text = bleve.NewDisjunctionQuery(phrase, termsQuery(terms))
	}

	name := bleve.NewTermQuery("document")
	name.SetField(_fieldType)
	name.SetBoost(_documentNameBoost)

	heading := bleve.NewTermQuery("heading")
	heading.SetField(_fieldType)
	heading.SetBoost(_headingBoost)

	def := bleve.NewBoolFieldQuery(true)
	def.SetField(_fieldBranchDefault)
	def.SetBoost(_defaultBranchBoost)

	bq := bleve.NewBooleanQuery()
	bq.AddMust(org, text)
	bq.AddShould(name, heading, def)

	return bq
}

// terms analyzes the query into at most _maxQueryTerms index terms.
func (i *Index) terms(q string) []string {
	tokens := i.idx.Mapping().AnalyzerNamed(_analyzerName).Analyze([]byte(q))
	terms := make([]string, 0, min(len(tokens), _maxQueryTerms))

	for _, token := range tokens[:min(len(tokens), _maxQueryTerms)] {
		terms = append(terms, string(token.Term))
	}

	return terms
}

// termsQuery matches the terms with automatic fuzziness, the last one also
// as a prefix since it is what the user is still typing. All terms but one
// have to match: a lone unknown word does not empty the results, while a
// common word inside otherwise unknown ones does not fill them.
func termsQuery(terms []string) query.Query {
	dq := bleve.NewDisjunctionQuery()

	for n, term := range terms {
		fuzzy := bleve.NewFuzzyQuery(term)
		fuzzy.SetField(_fieldText)
		fuzzy.SetAutoFuzziness(true)

		if n != len(terms)-1 {
			dq.AddQuery(fuzzy)
			continue
		}

		prefix := bleve.NewPrefixQuery(term)
		prefix.SetField(_fieldText)

		dq.AddQuery(bleve.NewDisjunctionQuery(fuzzy, prefix))
	}

	dq.SetMin(float64(max(len(terms)-1, 1)))

	return dq
}

// decodeHit rebuilds an entry from a hit's stored fields. Every field is
// stored with the type record gives it, so one of another type is a bug;
// it is logged and left at its zero value rather than failing the search.
func (i *Index) decodeHit(id string, fields map[string]any) (Block, error) {
	str := func(field string) string {
		s, ok := fields[field].(string)
		if !ok {
			i.log.With("entry_id", id).
				With("field", field).
				Warn("search entry field is not a string")
		}

		return s
	}

	documentID, err := xid.FromString(str(_fieldDocumentID))
	if err != nil {
		return Block{}, fmt.Errorf("decoding document id of entry %s: %w", id, err)
	}

	branchID, err := xid.FromString(str(_fieldBranchID))
	if err != nil {
		return Block{}, fmt.Errorf("decoding branch id of entry %s: %w", id, err)
	}

	def, ok := fields[_fieldBranchDefault].(bool)
	if !ok {
		i.log.With("entry_id", id).
			With("field", _fieldBranchDefault).
			Warn("search entry field is not a boolean")
	}

	var attrs map[string]string

	for field := range fields {
		key, ok := strings.CutPrefix(field, _fieldAttrs+".")
		if !ok {
			continue
		}

		if attrs == nil {
			attrs = make(map[string]string)
		}

		attrs[key] = str(field)
	}

	return Block{
		ID:             id,
		OrganizationID: str(_fieldOrganizationID),
		DocumentID:     documentID,
		BranchID:       branchID,
		BranchName:     str(_fieldBranchName),
		BranchDefault:  def,
		Type:           str(_fieldType),
		Text:           str(_fieldText),
		Attrs:          attrs,
	}, nil
}
