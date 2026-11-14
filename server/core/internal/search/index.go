// Package search indexes and searches documents through an embedded bleve
// index.
package search

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _synonymsFile holds the search synonyms, embedded at build time.
//
//go:embed static/synonyms.json
var _synonymsFile []byte

const (
	// _buildSuffix is appended to the index path while a fresh index is
	// being built next to it.
	_buildSuffix = ".building"

	// _buildPage is the number of branches one rebuild step reads.
	_buildPage = 500

	// _idsPage is the number of entry ids one lookup step reads.
	_idsPage = 1000

	// _writeTimeout bounds one batch of index writes.
	_writeTimeout = 2 * time.Minute
)

// Index is the document index. Handlers search it concurrently; the
// search-job worker and the boot-time build are its only writers.
type Index struct {
	log *slog.Logger
	idx bleve.Index
}

// Open opens the index at path, or builds it from src when there is none
// or the one there was written by a binary with another mapping. The build
// happens next to path and is swapped in only once complete, so an
// interrupted build never leaves a half-written index behind.
func Open(ctx context.Context, log *slog.Logger, path string, src Source) (*Index, error) {
	if path == "" {
		return nil, errors.New("index path is empty")
	}

	if err := registerHighlighter(); err != nil {
		return nil, err
	}

	log = log.With("component", "search-index")

	idx, err := openCurrent(log, path)
	if err != nil {
		return nil, err
	}

	if idx == nil {
		idx, err = build(ctx, log, path, src)
		if err != nil {
			return nil, err
		}
	}

	return &Index{
		log: log,
		idx: idx,
	}, nil
}

// openCurrent opens the index at path when it exists and carries the
// mapping version of this binary. It returns nil otherwise, which asks for
// a build; an index that cannot be opened counts as absent, since it is
// only ever a cache of the database.
func openCurrent(log *slog.Logger, path string) (bleve.Index, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil //nolint:nilnil // no index is a valid state that asks for a build
		}

		return nil, fmt.Errorf("checking index path: %w", err)
	}

	idx, err := bleve.Open(path)
	if err != nil {
		log.With("error", err).
			Warn("cannot open the search index, rebuilding it")

		return nil, nil //nolint:nilnil // an unreadable index asks for a build
	}

	version, err := idx.GetInternal([]byte(_mappingVersionKey))
	if err != nil {
		if cerr := idx.Close(); cerr != nil {
			return nil, fmt.Errorf("closing index: %w", cerr)
		}

		return nil, fmt.Errorf("reading mapping version: %w", err)
	}

	if string(version) == _mappingVersion {
		return idx, nil
	}

	log.With("index_version", string(version)).
		With("binary_version", _mappingVersion).
		Info("search index mapping is outdated, rebuilding it")

	if err := idx.Close(); err != nil {
		return nil, fmt.Errorf("closing index: %w", err)
	}

	return nil, nil //nolint:nilnil // an outdated index asks for a build
}

// build creates a fresh index from every branch in src at path.
func build(ctx context.Context, log *slog.Logger, path string, src Source) (bleve.Index, error) {
	log.Info("building the search index")

	tmp := path + _buildSuffix

	if err := os.RemoveAll(tmp); err != nil {
		return nil, fmt.Errorf("removing stale build: %w", err)
	}

	im, err := newIndexMapping()
	if err != nil {
		return nil, err
	}

	idx, err := bleve.New(tmp, im)
	if err != nil {
		return nil, fmt.Errorf("creating index: %w", err)
	}

	if err = fill(ctx, log, idx, src); err != nil {
		if cerr := idx.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("closing index: %w", cerr))
		}

		return nil, err
	}

	if err = idx.Close(); err != nil {
		return nil, fmt.Errorf("closing built index: %w", err)
	}

	if err = os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("removing previous index: %w", err)
	}

	if err = os.Rename(tmp, path); err != nil {
		return nil, fmt.Errorf("moving built index into place: %w", err)
	}

	idx, err = bleve.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening built index: %w", err)
	}

	return idx, nil
}

// fill writes the synonyms, every branch's entries and finally the mapping
// version into a fresh index. The version goes last, so an index missing
// it is one whose build did not finish.
func fill(ctx context.Context, log *slog.Logger, idx bleve.Index, src Source) error {
	if err := indexSynonyms(idx); err != nil {
		return err
	}

	var (
		after    xid.ID
		branches int
	)

	for {
		docs, err := src.FetchDocumentBranchesAfter(ctx, after, _buildPage)
		if err != nil {
			return fmt.Errorf("fetching branches: %w", err)
		}

		batch := idx.NewBatch()

		for _, doc := range docs {
			for _, e := range Entries(doc) {
				if err := batch.Index(e.ID, e.record()); err != nil {
					return fmt.Errorf("indexing entry %s: %w", e.ID, err)
				}
			}
		}

		if err := idx.Batch(batch); err != nil {
			return fmt.Errorf("writing batch: %w", err)
		}

		branches += len(docs)

		if len(docs) < _buildPage {
			break
		}

		after = docs[len(docs)-1].BranchID
	}

	if err := idx.SetInternal([]byte(_mappingVersionKey), []byte(_mappingVersion)); err != nil {
		return fmt.Errorf("writing mapping version: %w", err)
	}

	log.With("branches", branches).
		Info("built the search index")

	return nil
}

// indexSynonyms loads the synonyms file into the index. Every entry is
// bidirectional: the key and each of its values expand to one another.
func indexSynonyms(idx bleve.Index) error {
	var synonyms map[string][]string

	if err := json.Unmarshal(_synonymsFile, &synonyms); err != nil {
		return fmt.Errorf("unmarshaling synonyms: %w", err)
	}

	batch := idx.NewBatch()

	n := 0

	for key, values := range synonyms {
		n++

		def := &bleve.SynonymDefinition{
			Synonyms: append([]string{key}, values...),
		}

		if err := batch.IndexSynonym("synonym-"+strconv.Itoa(n), _synonymCollection, def); err != nil {
			return fmt.Errorf("indexing synonym %q: %w", key, err)
		}
	}

	if err := idx.Batch(batch); err != nil {
		return fmt.Errorf("writing synonyms: %w", err)
	}

	return nil
}

// Close closes the index.
func (i *Index) Close() error {
	return i.idx.Close()
}

// ReplaceBranch makes the branch's entries exactly the given ones: what is
// there and not in entries is deleted, the rest is written, all in one
// batch.
func (i *Index) ReplaceBranch(ctx context.Context, branchID xid.ID, entries map[string]Block) error {
	ctx, cancel := context.WithTimeout(ctx, _writeTimeout)
	defer cancel()

	ids, err := i.ids(ctx, _fieldBranchID, branchID.String())
	if err != nil {
		return err
	}

	batch := i.idx.NewBatch()

	// a block that survives an edit keeps its id, so it is deleted and
	// indexed in the same batch. The batch is a map keyed by id where the
	// last write wins, which is why the deletes go in first.
	for _, id := range ids {
		batch.Delete(id)
	}

	for _, e := range entries {
		if err := batch.Index(e.ID, e.record()); err != nil {
			return fmt.Errorf("indexing entry %s: %w", e.ID, err)
		}
	}

	if err := i.idx.Batch(batch); err != nil {
		return fmt.Errorf("replacing branch %s: %w", branchID, err)
	}

	return nil
}

// DeleteBranch removes every entry of the branch.
func (i *Index) DeleteBranch(ctx context.Context, branchID xid.ID) error {
	return i.deleteByTerm(ctx, _fieldBranchID, branchID.String())
}

// DeleteDocument removes every entry of the document, on every branch.
func (i *Index) DeleteDocument(ctx context.Context, documentID xid.ID) error {
	return i.deleteByTerm(ctx, _fieldDocumentID, documentID.String())
}

// DeleteOrganization removes every entry of the organization.
func (i *Index) DeleteOrganization(ctx context.Context, organizationID string) error {
	return i.deleteByTerm(ctx, _fieldOrganizationID, organizationID)
}

// deleteByTerm removes every entry whose field holds the value.
func (i *Index) deleteByTerm(ctx context.Context, field, value string) error {
	ctx, cancel := context.WithTimeout(ctx, _writeTimeout)
	defer cancel()

	ids, err := i.ids(ctx, field, value)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	batch := i.idx.NewBatch()

	for _, id := range ids {
		batch.Delete(id)
	}

	if err := i.idx.Batch(batch); err != nil {
		return fmt.Errorf("deleting by %s: %w", field, err)
	}

	return nil
}

// ids lists the ids of every entry whose field holds the value. bleve has
// no delete-by-query, so removals go through this list.
func (i *Index) ids(ctx context.Context, field, value string) ([]string, error) {
	q := bleve.NewTermQuery(value)
	q.SetField(field)

	var ids []string

	for from := 0; ; from += _idsPage {
		req := bleve.NewSearchRequestOptions(q, _idsPage, from, false)
		req.SortBy([]string{"_id"})

		res, err := i.idx.SearchInContext(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("listing entries by %s: %w", field, err)
		}

		for _, hit := range res.Hits {
			ids = append(ids, hit.ID)
		}

		if len(res.Hits) < _idsPage {
			return ids, nil
		}
	}
}

// Source is the database surface a build reads the branches from.
type Source interface {
	// FetchDocumentBranchesAfter should return up to limit branches whose
	// id sorts after the given one, joined with their document, in id
	// order.
	FetchDocumentBranchesAfter(ctx context.Context, after xid.ID, limit int) ([]document.Document, error)
}
