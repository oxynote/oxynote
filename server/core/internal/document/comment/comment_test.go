package comment

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubComment builds a fully populated comment for tests.
func stubComment() Comment {
	return Comment{
		ID:                  xid.New(),
		DocumentID:          xid.New(),
		OrganizationID:      "org-1",
		BranchID:            xid.New(),
		AnchorBlockID:       null.StringFrom("block-1"),
		UserID:              null.StringFrom("user-1"),
		Content:             Content{"text": "original"},
		DiffDeletionContext: null.ValueFrom(json.RawMessage(`{"ctx": true}`)),
		CreatedAt:           time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Replies: []Reply{
			{ID: xid.New(), UserID: null.StringFrom("user-2"), Content: Content{"text": "first reply"}},
			{ID: xid.New(), UserID: null.StringFrom("user-3"), Content: Content{"text": "second reply"}},
		},
	}
}

func Test_NewComment(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()
	diffCtx := null.ValueFrom(json.RawMessage(`{"ctx": true}`))

	c := NewComment(Input{
		Content:             Content{"text": "hello"},
		AnchorBlockID:       null.StringFrom("block-1"),
		DiffDeletionContext: diffCtx,
	}, documentID, branchID, "user-1", "org-1")

	assert.False(t, c.ID.IsZero())
	assert.Equal(t, documentID, c.DocumentID)
	assert.Equal(t, branchID, c.BranchID)
	assert.Equal(t, "org-1", c.OrganizationID)
	assert.Equal(t, null.StringFrom("block-1"), c.AnchorBlockID)
	assert.Equal(t, null.StringFrom("user-1"), c.UserID)
	assert.False(t, c.Resolved)
	assert.Equal(t, Content{"text": "hello"}, c.Content)
	assert.Equal(t, diffCtx, c.DiffDeletionContext)
	assert.False(t, c.CreatedAt.IsZero())
}

func Test_Comment_ApplyUpdate(t *testing.T) {
	t.Parallel()

	c := stubComment().Resolve("resolver")

	nc := c.ApplyUpdate(Input{
		Content:       Content{"text": "edited"},
		AnchorBlockID: null.StringFrom("block-2"),
	})

	assert.Equal(t, Content{"text": "edited"}, nc.Content)
	assert.Equal(t, null.StringFrom("block-2"), nc.AnchorBlockID)
	assert.True(t, nc.UpdatedAt.Valid)

	// everything else is preserved — including the resolver, the
	// branch, and the diff-deletion context.
	assert.Equal(t, c.ID, nc.ID)
	assert.Equal(t, c.BranchID, nc.BranchID)
	assert.Equal(t, c.UserID, nc.UserID)
	assert.True(t, nc.Resolved)
	assert.Equal(t, null.StringFrom("resolver"), nc.ResolvedBy)
	assert.Equal(t, c.DiffDeletionContext, nc.DiffDeletionContext)
	assert.Equal(t, c.CreatedAt, nc.CreatedAt)
	assert.Equal(t, c.Replies, nc.Replies)
}

func Test_Comment_Resolve(t *testing.T) {
	t.Parallel()

	c := stubComment()

	resolved := c.Resolve("resolver")
	assert.True(t, resolved.Resolved)
	assert.Equal(t, null.StringFrom("resolver"), resolved.ResolvedBy)
	assert.Equal(t, c.BranchID, resolved.BranchID)
	assert.Equal(t, c.Content, resolved.Content)
	assert.Equal(t, c.DiffDeletionContext, resolved.DiffDeletionContext)
}

func Test_Comment_Unresolve(t *testing.T) {
	t.Parallel()

	c := stubComment()

	unresolved := c.Resolve("resolver").Unresolve()
	assert.False(t, unresolved.Resolved)
	assert.False(t, unresolved.ResolvedBy.Valid)
	assert.Equal(t, c.BranchID, unresolved.BranchID)
	assert.Equal(t, c.Content, unresolved.Content)
}

func Test_Comment_Replace(t *testing.T) {
	t.Parallel()

	c := stubComment()
	head := c.Replies[0]

	nc, ok := c.Replace()
	require.True(t, ok)

	// the head reply's content and author take over the comment.
	assert.Equal(t, c.ID, nc.ID)
	assert.Equal(t, head.UserID, nc.UserID)
	assert.Equal(t, head.Content, nc.Content)
	assert.Equal(t, head.CreatedAt, nc.CreatedAt)
	assert.Equal(t, head.UpdatedAt, nc.UpdatedAt)
	assert.Equal(t, c.BranchID, nc.BranchID)

	// the promoted reply drops out of the reply list.
	require.Len(t, nc.Replies, 1)
	assert.Equal(t, c.Replies[1], nc.Replies[0])

	// a comment with no replies has nothing to promote and is returned
	// unchanged rather than panicking on an empty list.
	empty := c
	empty.Replies = nil

	same, ok := empty.Replace()
	assert.False(t, ok)
	assert.Equal(t, empty, same)
}

func Test_NewReply(t *testing.T) {
	t.Parallel()

	commentID := xid.New()

	r := NewReply(ReplyInput{Content: Content{"text": "reply"}}, commentID, "user-1", "org-1")

	assert.False(t, r.ID.IsZero())
	assert.Equal(t, commentID, r.CommentID)
	assert.Equal(t, "org-1", r.OrganizationID)
	assert.Equal(t, null.StringFrom("user-1"), r.UserID)
	assert.Equal(t, Content{"text": "reply"}, r.Content)
	assert.False(t, r.CreatedAt.IsZero())
}

func Test_Reply_ApplyUpdate(t *testing.T) {
	t.Parallel()

	r := NewReply(ReplyInput{Content: Content{"text": "reply"}}, xid.New(), "user-1", "org-1")

	nr := r.ApplyUpdate(ReplyInput{Content: Content{"text": "edited"}})

	assert.Equal(t, r.ID, nr.ID)
	assert.Equal(t, r.CommentID, nr.CommentID)
	assert.Equal(t, r.UserID, nr.UserID)
	assert.Equal(t, Content{"text": "edited"}, nr.Content)
	assert.Equal(t, r.CreatedAt, nr.CreatedAt)
	assert.True(t, nr.UpdatedAt.Valid)
}

func Test_Content_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value     any
		ExpectErr bool
		Expected  Content
	}{
		"Byte slice is decoded": {
			Value:    []byte(`{"text": "hello"}`),
			Expected: Content{"text": "hello"},
		},
		"String is decoded": {
			Value:    `{"text": "hello"}`,
			Expected: Content{"text": "hello"},
		},
		"Nil clears the content": {
			Value: nil,
		},
		"Unsupported type fails": {
			Value:     42,
			ExpectErr: true,
		},
		"Malformed JSON fails": {
			Value:     []byte(`{not json`),
			ExpectErr: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			decoded := Content{"stale": true}

			err := decoded.Scan(c.Value)

			if c.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Expected, decoded)
		})
	}
}

func Test_Content_Value(t *testing.T) {
	t.Parallel()

	t.Run("Value round-trips through Scan", func(t *testing.T) {
		t.Parallel()

		orig := Content{"text": "hello"}

		val, err := orig.Value()
		require.NoError(t, err)

		var decoded Content

		require.NoError(t, decoded.Scan(val))
		assert.Equal(t, orig, decoded)
	})

	t.Run("Nil content stores SQL NULL", func(t *testing.T) {
		t.Parallel()

		val, err := Content(nil).Value()
		require.NoError(t, err)
		assert.Nil(t, val)
	})
}
