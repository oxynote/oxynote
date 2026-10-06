package search

import (
	"fmt"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	htmlFormatter "github.com/blevesearch/bleve/v2/search/highlight/format/html"
	simpleFragmenter "github.com/blevesearch/bleve/v2/search/highlight/fragmenter/simple"
	simpleHighlighter "github.com/blevesearch/bleve/v2/search/highlight/highlighter/simple"
	bleveIndex "github.com/blevesearch/bleve_index_api"
)

// _mappingVersion is stamped into every index built by this binary. Bump it
// whenever the analyzer, the fields, the synonyms file or anything else the
// index stores changes shape: an index carrying another version is rebuilt
// at boot.
const _mappingVersion = "2"

// _mappingVersionKey is the index-internal key holding the mapping version.
const _mappingVersionKey = "mapping-version"

// Entry field names, as they appear in Block's JSON.
const (
	_fieldText           = "text"
	_fieldOrganizationID = "organizationId"
	_fieldDocumentID     = "documentId"
	_fieldBranchID       = "branchId"
	_fieldBranchName     = "branchName"
	_fieldBranchDefault  = "branchDefault"
	_fieldType           = "type"
	_fieldAttrs          = "attrs"
)

const (
	// _analyzerName is the analyzer the text field and the synonyms share.
	_analyzerName = "text"

	// _synonymSource is the synonym source the text field expands through.
	_synonymSource = "synonyms"

	// _synonymCollection is the collection the synonym definitions are
	// indexed under.
	_synonymCollection = "synonyms"

	// _fragmenterName names the fragmenter cropping highlighted text.
	_fragmenterName = "crop"

	// _fragmentSize is the number of characters a highlighted fragment
	// keeps around the best match.
	_fragmentSize = 120

	// _highlighterName names the highlighter wrapping matches in mark tags.
	_highlighterName = "mark"
)

// _registerHighlighter guards the one-time registration of the highlighter
// in bleve's process-wide registry, which refuses a second definition.
var _registerHighlighter sync.Once

// newIndexMapping describes the entries: one text field analyzed for
// search, keyword fields to filter and boost on, the branch name kept
// only for display, and the kept attributes as keywords under attrs.
func newIndexMapping() (*mapping.IndexMappingImpl, error) {
	im := bleve.NewIndexMapping()
	im.ScoringModel = bleveIndex.BM25Scoring

	// no stop-word removal: phrases such as "not found" must stay
	// searchable as written.
	if err := im.AddCustomAnalyzer(_analyzerName, map[string]any{
		"type":          custom.Name,
		"tokenizer":     unicode.Name,
		"token_filters": []any{lowercase.Name, en.SnowballStemmerName},
	}); err != nil {
		return nil, fmt.Errorf("adding analyzer: %w", err)
	}

	if err := im.AddSynonymSource(_synonymSource, map[string]any{
		"collection": _synonymCollection,
		"analyzer":   _analyzerName,
	}); err != nil {
		return nil, fmt.Errorf("adding synonym source: %w", err)
	}

	text := bleve.NewTextFieldMapping()
	text.Analyzer = _analyzerName
	text.SynonymSource = _synonymSource

	name := bleve.NewKeywordFieldMapping()
	name.Index = false

	dm := bleve.NewDocumentStaticMapping()
	dm.AddFieldMappingsAt(_fieldText, text)
	dm.AddFieldMappingsAt(_fieldOrganizationID, bleve.NewKeywordFieldMapping())
	dm.AddFieldMappingsAt(_fieldDocumentID, bleve.NewKeywordFieldMapping())
	dm.AddFieldMappingsAt(_fieldBranchID, bleve.NewKeywordFieldMapping())
	dm.AddFieldMappingsAt(_fieldType, bleve.NewKeywordFieldMapping())
	dm.AddFieldMappingsAt(_fieldBranchDefault, bleve.NewBooleanFieldMapping())
	dm.AddFieldMappingsAt(_fieldBranchName, name)

	// every attribute attrKeys returns is mapped dynamically, as a stored
	// keyword.
	attrs := bleve.NewDocumentMapping()
	attrs.DefaultAnalyzer = keyword.Name

	dm.AddSubDocumentMapping(_fieldAttrs, attrs)

	im.DefaultMapping = dm

	return im, nil
}

// registerHighlighter defines the highlighter search results are formatted
// with. bleve keeps highlighters in a process-wide registry, so this runs
// once per process rather than once per index.
func registerHighlighter() error {
	var err error

	_registerHighlighter.Do(func() {
		_, err = bleve.Config.Cache.DefineFragmenter(_fragmenterName, map[string]any{
			"type": simpleFragmenter.Name,
			"size": float64(_fragmentSize),
		})
		if err != nil {
			err = fmt.Errorf("defining fragmenter: %w", err)
			return
		}

		_, err = bleve.Config.Cache.DefineHighlighter(_highlighterName, map[string]any{
			"type":       simpleHighlighter.Name,
			"fragmenter": _fragmenterName,
			"formatter":  htmlFormatter.Name,
		})
		if err != nil {
			err = fmt.Errorf("defining highlighter: %w", err)
		}
	})

	return err
}
