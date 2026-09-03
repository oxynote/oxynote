package tag

import (
	"context"
	"slices"
	"sync"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

var (
	// _stubSeedColors are the colours the seeded tags carry, in order.
	_stubSeedColors = []string{"#22c55e", "#f97316", "#3b82f6"}

	// _stubSeedNames are the names the seeded tags carry, in order.
	_stubSeedNames = []string{"Production", "Staging", "Incidents"}
)

// DocumentTreeReader reads the organization's document tree.
type DocumentTreeReader interface {
	// FetchDocumentTree should fetch the document tree of an organization.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)
}

// StubDB stands in for the tag tables until they exist.
//
// FIXME: it keeps every organization's tags in memory, so they are lost on
// restart and are not shared between instances, and it seeds an
// organization with a few tags the first time it is read so the sidebar has
// something to show.
//
// TODO: replace it with the real agent methods on internal/db:
//   - a `tags` table (id, organization_id, tag_name, color, sort_index,
//     created_at, created_by) mirroring `documents`;
//   - a `document_tags` join table (document_id, tag_id) cascading on both
//     sides, which is what makes a document reachable from several tags;
//   - a `user_tag_settings` join table (user_id, tag_id, hidden) cascading
//     on both sides. Visibility is per user, not a column on `tags`: two
//     members of one organization hide different tags;
//   - FetchTagTree as one query joining all three against the document
//     tree, left-joining the settings on the requesting user so a tag
//     nobody has touched reads as visible, and replacing the assembly this
//     type does by hand.
//
// The methods below are the contract the real agent has to keep; the
// handler is written against it and needs no change when they are.
type StubDB struct {
	docs DocumentTreeReader

	mu   sync.Mutex
	orgs map[string]*stubOrg
}

// stubOrg holds one organization's tags, their document assignments and
// each user's visibility preferences.
type stubOrg struct {
	// tags are the organization's tags, in their display order.
	tags []Tag

	// assigned maps a tag id to the documents carrying it.
	assigned map[xid.ID][]xid.ID

	// hidden maps a user id to the tags that user keeps out of their
	// sidebar. A tag missing from the set reads as visible.
	hidden map[string]map[xid.ID]bool
}

// NewStubDB creates a fresh instance of StubDB reading documents through
// the given reader.
func NewStubDB(docs DocumentTreeReader) *StubDB {
	return &StubDB{
		docs: docs,
		orgs: map[string]*stubOrg{},
	}
}

// FetchTagTree retrieves an organization's tags together with the documents
// carrying each of them.
func (sd *StubDB) FetchTagTree(ctx context.Context, organizationID, userID string) (Summaries, error) {
	tree, err := sd.docs.FetchDocumentTree(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	byID := map[xid.ID]document.Summary{}
	for _, doc := range tree.Descendants() {
		byID[doc.ID] = doc
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, tree)
	hidden := org.hidden[userID]
	out := make(Summaries, 0, len(org.tags))

	for _, t := range org.tags {
		docs := make(document.Summaries, 0, len(org.assigned[t.ID]))

		for _, docID := range org.assigned[t.ID] {
			if doc, ok := byID[docID]; ok {
				docs = append(docs, doc)
			}
		}

		out = append(out, Summary{
			ID:        t.ID,
			TagName:   t.TagName,
			Color:     t.Color,
			Hidden:    hidden[t.ID],
			Documents: docs,
		})
	}

	return out, nil
}

// InsertTag stores a new tag at the end of its organization's tags.
func (sd *StubDB) InsertTag(_ context.Context, t Tag) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(t.OrganizationID, nil)
	t.SortIndex = len(org.tags)
	org.tags = append(org.tags, t)

	return nil
}

// SetTagVisibility records whether one user keeps a tag out of their
// sidebar. It changes nothing for anybody else.
func (sd *StubDB) SetTagVisibility(
	_ context.Context,
	organizationID, userID string,
	id xid.ID,
	inp VisibilityInput,
) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, nil)

	if !slices.ContainsFunc(org.tags, func(t Tag) bool { return t.ID == id }) {
		return errutil.ErrNotFound
	}

	if !inp.Hidden {
		delete(org.hidden[userID], id)

		return nil
	}

	if org.hidden[userID] == nil {
		org.hidden[userID] = map[xid.ID]bool{}
	}

	org.hidden[userID][id] = true

	return nil
}

// DeleteTag removes a tag and every assignment of it.
func (sd *StubDB) DeleteTag(_ context.Context, id xid.ID, organizationID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, nil)

	index := slices.IndexFunc(org.tags, func(t Tag) bool {
		return t.ID == id
	})
	if index == -1 {
		return errutil.ErrNotFound
	}

	org.tags = slices.Delete(org.tags, index, index+1)
	delete(org.assigned, id)

	for _, tags := range org.hidden {
		delete(tags, id)
	}

	return nil
}

// UpdateTagTree rewrites the display order of an organization's tags to the
// order of the given tree.
func (sd *StubDB) UpdateTagTree(_ context.Context, tree Summaries, organizationID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, nil)
	reordered := make([]Tag, 0, len(org.tags))

	for _, s := range tree {
		index := slices.IndexFunc(org.tags, func(t Tag) bool {
			return t.ID == s.ID
		})
		if index == -1 {
			return errutil.ErrNotFound
		}

		t := org.tags[index]
		t.SortIndex = len(reordered)
		reordered = append(reordered, t)
	}

	org.tags = reordered

	return nil
}

// AssignDocumentTag makes a document carry a tag. Assigning a tag a
// document already carries changes nothing.
func (sd *StubDB) AssignDocumentTag(_ context.Context, organizationID string, documentID, tagID xid.ID) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, nil)

	if !slices.ContainsFunc(org.tags, func(t Tag) bool { return t.ID == tagID }) {
		return errutil.ErrNotFound
	}

	if slices.Contains(org.assigned[tagID], documentID) {
		return nil
	}

	org.assigned[tagID] = append(org.assigned[tagID], documentID)

	return nil
}

// UnassignDocumentTag stops a document carrying a tag. Removing a tag the
// document does not carry changes nothing.
func (sd *StubDB) UnassignDocumentTag(_ context.Context, organizationID string, documentID, tagID xid.ID) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	org := sd.org(organizationID, nil)

	index := slices.Index(org.assigned[tagID], documentID)
	if index == -1 {
		return nil
	}

	org.assigned[tagID] = slices.Delete(org.assigned[tagID], index, index+1)

	return nil
}

// org returns an organization's state, seeding it from the given document
// tree the first time it is touched. Callers must hold the mutex.
func (sd *StubDB) org(organizationID string, seed document.Summaries) *stubOrg {
	if org, ok := sd.orgs[organizationID]; ok {
		return org
	}

	org := &stubOrg{
		assigned: map[xid.ID][]xid.ID{},
		hidden:   map[string]map[xid.ID]bool{},
	}
	sd.orgs[organizationID] = org

	for i, name := range _stubSeedNames {
		org.tags = append(org.tags, Tag{
			ID:             xid.New(),
			OrganizationID: organizationID,
			TagName:        name,
			Color:          _stubSeedColors[i],
			SortIndex:      i,
		})
	}

	// the seeded tags carry the organization's first documents, spread
	// round-robin, so every tag has something under it.
	for i, doc := range seed {
		t := org.tags[i%len(org.tags)]
		org.assigned[t.ID] = append(org.assigned[t.ID], doc.ID)
	}

	return org
}
