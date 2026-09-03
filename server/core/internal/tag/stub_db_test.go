package tag

import (
	"context"
	"errors"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDocs answers with a fixed document tree, or an error.
type fakeDocs struct {
	tree document.Summaries
	err  error
}

func (fd fakeDocs) FetchDocumentTree(context.Context, string) (document.Summaries, error) {
	return fd.tree, fd.err
}

// docTree builds a two-root tree whose first root carries a child.
func docTree(idA, idB, idChild xid.ID) document.Summaries {
	return document.Summaries{
		{
			ID:           idA,
			DocumentName: "Runbook",
			Children:     document.Summaries{{ID: idChild, DocumentName: "Rollback"}},
		},
		{ID: idB, DocumentName: "Postmortem"},
	}
}

// tagNames reads the names out of a tag tree, in order.
func tagNames(ss Summaries) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.TagName)
	}

	return out
}

func Test_NewStubDB(t *testing.T) {
	t.Parallel()

	docs := fakeDocs{}

	sd := NewStubDB(docs)
	require.NotNil(t, sd)
	assert.Equal(t, docs, sd.docs)
	assert.Empty(t, sd.orgs)
}

func Test_StubDB_FetchTagTree(t *testing.T) {
	t.Parallel()

	t.Run("Document tree error", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{err: errors.New("boom")})

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.Error(t, err)
		assert.Nil(t, tree)
	})

	t.Run("Seeds an organization on first read", func(t *testing.T) {
		t.Parallel()

		idA, idB, idChild := xid.New(), xid.New(), xid.New()
		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Equal(t, _stubSeedNames, tagNames(tree))

		// the two roots are spread over the first two tags, and the third
		// seeded tag is left holding nothing
		require.Len(t, tree[0].Documents, 1)
		assert.Equal(t, idA, tree[0].Documents[0].ID)
		require.Len(t, tree[1].Documents, 1)
		assert.Equal(t, idB, tree[1].Documents[0].ID)
		assert.Empty(t, tree[2].Documents)
	})

	t.Run("Drops an assignment whose document is gone", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		first, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		require.NotEmpty(t, first)

		require.NoError(t, sd.AssignDocumentTag(context.Background(), "org1", xid.New(), first[0].ID))

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Empty(t, tree[0].Documents)
	})
}

func Test_StubDB_InsertTag(t *testing.T) {
	t.Parallel()

	sd := NewStubDB(fakeDocs{})

	tg := NewTag(CreateInput{TagName: "Rollout", Color: "#22c55e"}, "org1", "u1")
	require.NoError(t, sd.InsertTag(context.Background(), tg))

	tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
	require.NoError(t, err)
	assert.Equal(t, append(append([]string{}, _stubSeedNames...), "Rollout"), tagNames(tree))
	assert.Equal(t, len(_stubSeedNames), sd.orgs["org1"].tags[len(_stubSeedNames)].SortIndex)
}

func Test_StubDB_SetTagVisibility(t *testing.T) {
	t.Parallel()

	t.Run("Unknown tag", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		err := sd.SetTagVisibility(
			context.Background(), "org1", "u1", xid.New(), VisibilityInput{Hidden: true},
		)
		assert.Equal(t, errutil.ErrNotFound, err)
	})

	t.Run("Hides the tag without dropping it", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		before, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.False(t, before[0].Hidden)

		require.NoError(t, sd.SetTagVisibility(
			context.Background(), "org1", "u1", before[0].ID, VisibilityInput{Hidden: true},
		))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Equal(t, _stubSeedNames, tagNames(after))
		assert.True(t, after[0].Hidden)
	})

	t.Run("Shows the tag again", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		before, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.SetTagVisibility(
			context.Background(), "org1", "u1", before[0].ID, VisibilityInput{Hidden: true},
		))
		require.NoError(t, sd.SetTagVisibility(
			context.Background(), "org1", "u1", before[0].ID, VisibilityInput{Hidden: false},
		))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.False(t, after[0].Hidden)
	})

	t.Run("Leaves every other user's view alone", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		before, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.SetTagVisibility(
			context.Background(), "org1", "u1", before[0].ID, VisibilityInput{Hidden: true},
		))

		mine, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.True(t, mine[0].Hidden)

		theirs, err := sd.FetchTagTree(context.Background(), "org1", "u2")
		require.NoError(t, err)
		assert.False(t, theirs[0].Hidden)
	})
}

func Test_StubDB_DeleteTag(t *testing.T) {
	t.Parallel()

	t.Run("Unknown tag", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		assert.Equal(t, errutil.ErrNotFound, sd.DeleteTag(context.Background(), xid.New(), "org1"))
	})

	t.Run("Drops the tag and its assignments", func(t *testing.T) {
		t.Parallel()

		idA, idB, idChild := xid.New(), xid.New(), xid.New()
		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		before, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.SetTagVisibility(
			context.Background(), "org1", "u1", before[0].ID, VisibilityInput{Hidden: true},
		))
		require.NoError(t, sd.DeleteTag(context.Background(), before[0].ID, "org1"))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Equal(t, _stubSeedNames[1:], tagNames(after))
		assert.NotContains(t, sd.orgs["org1"].assigned, before[0].ID)
		// the per-user preference goes with the tag
		assert.NotContains(t, sd.orgs["org1"].hidden["u1"], before[0].ID)
	})
}

func Test_StubDB_UpdateTagTree(t *testing.T) {
	t.Parallel()

	t.Run("Unknown tag", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		err := sd.UpdateTagTree(context.Background(), Summaries{{ID: xid.New()}}, "org1")
		assert.Equal(t, errutil.ErrNotFound, err)
	})

	t.Run("Rewrites the display order", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{})

		before, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		reversed := Summaries{before[2], before[1], before[0]}
		require.NoError(t, sd.UpdateTagTree(context.Background(), reversed, "org1"))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Equal(t, []string{_stubSeedNames[2], _stubSeedNames[1], _stubSeedNames[0]}, tagNames(after))

		for i, tg := range sd.orgs["org1"].tags {
			assert.Equal(t, i, tg.SortIndex)
		}
	})
}

func Test_StubDB_AssignDocumentTag(t *testing.T) {
	t.Parallel()

	idA, idB, idChild := xid.New(), xid.New(), xid.New()

	t.Run("Unknown tag", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		err := sd.AssignDocumentTag(context.Background(), "org1", idA, xid.New())
		assert.Equal(t, errutil.ErrNotFound, err)
	})

	t.Run("Assigning twice changes nothing", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.AssignDocumentTag(context.Background(), "org1", idChild, tree[0].ID))
		require.NoError(t, sd.AssignDocumentTag(context.Background(), "org1", idChild, tree[0].ID))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		require.Len(t, after[0].Documents, 2)
		assert.Equal(t, idChild, after[0].Documents[1].ID)
	})
}

func Test_StubDB_UnassignDocumentTag(t *testing.T) {
	t.Parallel()

	idA, idB, idChild := xid.New(), xid.New(), xid.New()

	t.Run("Removing a tag the document does not carry changes nothing", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.UnassignDocumentTag(context.Background(), "org1", idChild, tree[0].ID))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Len(t, after[0].Documents, 1)
	})

	t.Run("Drops the assignment", func(t *testing.T) {
		t.Parallel()

		sd := NewStubDB(fakeDocs{tree: docTree(idA, idB, idChild)})

		tree, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)

		require.NoError(t, sd.UnassignDocumentTag(context.Background(), "org1", idA, tree[0].ID))

		after, err := sd.FetchTagTree(context.Background(), "org1", "u1")
		require.NoError(t, err)
		assert.Empty(t, after[0].Documents)
	})
}
