package activity

import (
	"errors"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewTarget(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		Target{
			Kind: null.ValueFrom(TargetKindCommentRevision),
			ID:   null.StringFrom("rev1"),
		},
		NewTarget(TargetKindCommentRevision, "rev1"),
	)
}

func Test_newEntry(t *testing.T) {
	t.Parallel()

	actor := Actor{Kind: ActorKindUser, UserID: null.StringFrom("user1")}
	target := NewTarget(TargetKindCommentRevision, "rev1")
	docID, branchID := xid.New(), xid.New()
	at := timeutil.Now().Add(-time.Hour)

	e := newEntry(
		actor,
		"org1",
		docID,
		branchID,
		TypeTagAssigned,
		target,
		Metadata{"key": "value"},
		at,
	)

	assert.Equal(t, actor, e.Actor)
	assert.Equal(t, target, e.Target)
	assert.False(t, e.ID.IsNil())
	assert.True(t, at.Truncate(time.Second).Equal(e.ID.Time()))
	assert.Equal(t, "org1", e.OrganizationID)
	assert.Equal(t, docID, e.DocumentID)
	assert.Equal(t, branchID, e.BranchID)
	assert.Equal(t, TypeTagAssigned, e.Type)
	assert.Equal(t, CategoryTag, e.Category)
	assert.Equal(t, Metadata{"key": "value"}, e.Metadata)
	assert.Nil(t, e.Subject)
	assert.Equal(t, at, e.CreatedAt)
	assert.Equal(t, at, e.UpdatedAt)
}

func Test_Metadata_Value(t *testing.T) {
	t.Parallel()

	md := Metadata{
		MetaKeyCommentID: "comment1",
		MetaKeyReplyID:   "reply1",
	}

	v, err := md.Value()
	require.NoError(t, err)

	assert.JSONEq(t, `{"commentId":"comment1","replyId":"reply1"}`, string(v.([]byte)))
}

func Test_Metadata_Scan(t *testing.T) {
	cc := map[string]struct {
		Src    any
		Result Metadata
		Err    error
	}{
		"Invalid source type": {
			Src: 123,
			Err: errors.New("invalid metadata type"),
		},
		"Malformed JSON": {
			Src: []byte("{"),
			Err: assert.AnError,
		},
		"Successful scan from bytes": {
			Src:    []byte(`{"commentId":"comment1"}`),
			Result: Metadata{MetaKeyCommentID: "comment1"},
		},
		"Successful scan from string": {
			Src:    `{"commentId":"comment1"}`,
			Result: Metadata{MetaKeyCommentID: "comment1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var md Metadata

			err := md.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, md)
		})
	}
}

func Test_Query_PageSize(t *testing.T) {
	cc := map[string]struct {
		Query  Query
		Result uint64
	}{
		"No limit":            {Result: _defaultLimit},
		"Limit inside a page": {Query: Query{Limit: 10}, Result: 10},
		"Limit above the max": {Query: Query{Limit: _maxLimit + 1}, Result: _maxLimit},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Query.PageSize())
		})
	}
}
