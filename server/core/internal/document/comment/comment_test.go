package comment

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

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
	assert.True(t, resolved.ResolvedAt.Valid)
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
	assert.False(t, unresolved.ResolvedAt.Valid)
	assert.Equal(t, c.BranchID, unresolved.BranchID)
	assert.Equal(t, c.Content, unresolved.Content)
}

func Test_Comment_Replace(t *testing.T) {
	t.Parallel()

	cmt := stubComment()
	head := cmt.Replies[0]

	empty := cmt
	empty.Replies = nil

	cc := map[string]struct {
		Comment Comment
		Result  Comment
		OK      bool
	}{
		"Head reply takes over the comment": {
			Comment: cmt,
			Result: Comment{
				ID:                  cmt.ID,
				DocumentID:          cmt.DocumentID,
				OrganizationID:      cmt.OrganizationID,
				BranchID:            cmt.BranchID,
				AnchorBlockID:       cmt.AnchorBlockID,
				UserID:              head.UserID,
				Content:             head.Content,
				DiffDeletionContext: cmt.DiffDeletionContext,
				CreatedAt:           head.CreatedAt,
				UpdatedAt:           head.UpdatedAt,
				Replies:             []Reply{cmt.Replies[1]},
			},
			OK: true,
		},
		"Comment without replies is returned unchanged": {
			Comment: empty,
			Result:  empty,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nc, ok := c.Comment.Replace()
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, nc)
		})
	}
}

func Test_Comment_Participants(t *testing.T) {
	cc := map[string]struct {
		Comment Comment
		Result  []string
	}{
		"Author alone": {
			Comment: Comment{UserID: null.StringFrom("user-1")},
			Result:  []string{"user-1"},
		},
		"Author and repliers": {
			Comment: Comment{
				UserID: null.StringFrom("user-1"),
				Replies: []Reply{
					{UserID: null.StringFrom("user-2")},
					{UserID: null.StringFrom("user-1")},
					{UserID: null.String{}},
					{UserID: null.StringFrom("user-3")},
					{UserID: null.StringFrom("user-2")},
				},
			},
			Result: []string{"user-1", "user-2", "user-3"},
		},
		"Deleted author": {
			Comment: Comment{Replies: []Reply{{UserID: null.StringFrom("user-2")}}},
			Result:  []string{"user-2"},
		},
		"Nobody": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Comment.Participants())
		})
	}
}

func Test_Recipients(t *testing.T) {
	cc := map[string]struct {
		UserIDs []string
		Actor   string
		Result  []string
	}{
		"Actor dropped":   {UserIDs: []string{"user-1", "user-2"}, Actor: "user-1", Result: []string{"user-2"}},
		"Repeats dropped": {UserIDs: []string{"user-2", "user-3", "user-2"}, Actor: "user-1", Result: []string{"user-2", "user-3"}},
		"Actor alone":     {UserIDs: []string{"user-1"}, Actor: "user-1"},
		"Nobody":          {Actor: "user-1"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Recipients(c.UserIDs, c.Actor))
		})
	}
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

func Test_Input_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input Input
		Err   error
	}{
		"Missing content": {
			Input: Input{AnchorBlockID: null.StringFrom("block-1")},
			Err:   ErrMissingContent,
		},
		"Empty content": {
			Input: Input{Content: Content{}},
			Err:   ErrMissingContent,
		},
		"Valid input": {
			Input: Input{Content: Content{"text": "hello"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_StatusInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input StatusInput
		Err   error
	}{
		"Missing resolved flag": {
			Input: StatusInput{},
			Err:   ErrMissingStatus,
		},
		"Resolved": {
			Input: StatusInput{Resolved: null.BoolFrom(true)},
		},
		"Unresolved": {
			Input: StatusInput{Resolved: null.BoolFrom(false)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_ReplyInput_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input ReplyInput
		Err   error
	}{
		"Missing content": {
			Input: ReplyInput{},
			Err:   ErrMissingContent,
		},
		"Empty content": {
			Input: ReplyInput{Content: Content{}},
			Err:   ErrMissingContent,
		},
		"Valid input": {
			Input: ReplyInput{Content: Content{"text": "reply"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Input.Validate())
		})
	}
}

func Test_Content_Scan(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Value  any
		Result Content
		Err    error
	}{
		"Byte slice is decoded": {
			Value:  []byte(`{"text": "hello"}`),
			Result: Content{"text": "hello"},
		},
		"String is decoded": {
			Value:  `{"text": "hello"}`,
			Result: Content{"text": "hello"},
		},
		"Nil clears the content": {
			Value: nil,
		},
		"Unsupported type fails": {
			Value: 42,
			Err:   assert.AnError,
		},
		"Malformed JSON fails": {
			Value: []byte(`{not json`),
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			decoded := Content{"stale": true}

			err := decoded.Scan(c.Value)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, decoded)
		})
	}
}

func Test_Content_Value(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Content Content
		Check   func(t *testing.T, val driver.Value)
	}{
		"Value round-trips through Scan": {
			Content: Content{"text": "hello"},
			Check: func(t *testing.T, val driver.Value) {
				var decoded Content

				require.NoError(t, decoded.Scan(val))
				assert.Equal(t, Content{"text": "hello"}, decoded)
			},
		},
		"Nil content stores SQL NULL": {
			Check: func(t *testing.T, val driver.Value) {
				assert.Nil(t, val)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			val, err := c.Content.Value()
			require.NoError(t, err)

			c.Check(t, val)
		})
	}
}

func Test_Content_Excerpt(t *testing.T) {
	t.Parallel()

	paragraph := func(texts ...string) map[string]any {
		nodes := make([]any, 0, len(texts))

		for _, text := range texts {
			nodes = append(nodes, map[string]any{"type": "text", "text": text})
		}

		return map[string]any{"type": "paragraph", "content": nodes}
	}

	cc := map[string]struct {
		Content Content
		Result  string
	}{
		"Content without text is empty": {
			Content: Content{"type": "doc"},
		},
		"Marked runs join and blocks are set apart": {
			Content: Content{
				"type": "doc",
				"content": []any{
					paragraph("Worth a ", "second", " look"),
					paragraph("before the release"),
				},
			},
			Result: "Worth a second look before the release",
		},
		"Nested blocks and breaks are set apart once": {
			Content: Content{
				"type": "doc",
				"content": []any{
					map[string]any{
						"type": "bulletList",
						"content": []any{
							map[string]any{"type": "listItem", "content": []any{paragraph("first")}},
							map[string]any{"type": "listItem", "content": []any{paragraph("second")}},
						},
					},
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": "third"},
							map[string]any{"type": "hardBreak"},
							map[string]any{"type": "text", "text": "fourth"},
							"stray",
						},
					},
				},
			},
			Result: "first second third fourth",
		},
		"Long text is cut by runes": {
			Content: Content{
				"type":    "doc",
				"content": []any{paragraph(strings.Repeat("ž", _excerptLength+1))},
			},
			Result: strings.Repeat("ž", _excerptLength) + "…",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Content.Excerpt())
		})
	}
}

func Test_writeText(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Node   map[string]any
		Result string
	}{
		"Text runs join and the node is set apart": {
			Node: map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "one"},
					map[string]any{"type": "text", "text": "two"},
				},
			},
			Result: "onetwo ",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var sb strings.Builder

			writeText(&sb, c.Node)
			assert.Equal(t, c.Result, sb.String())
		})
	}
}
