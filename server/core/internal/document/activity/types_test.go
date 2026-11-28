package activity

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Category_Validate(t *testing.T) {
	cc := map[string]struct {
		Category Category
		Err      error
	}{
		"Empty":    {Err: ErrInvalidCategory},
		"Unknown":  {Category: "edit", Err: ErrInvalidCategory},
		"Document": {Category: CategoryDocument},
		"Content":  {Category: CategoryContent},
		"Branch":   {Category: CategoryBranch},
		"Review":   {Category: CategoryReview},
		"Comment":  {Category: CategoryComment},
		"Hook":     {Category: CategoryHook},
		"Tag":      {Category: CategoryTag},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Category.Validate())
		})
	}
}

func Test_Category_MarshalText(t *testing.T) {
	t.Parallel()

	text, err := CategoryComment.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "comment", string(text))
}

func Test_Category_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text     string
		Category Category
		Err      error
	}{
		"Unknown":           {Text: "edit", Err: assert.AnError},
		"Known":             {Text: "comment", Category: CategoryComment},
		"Padded upper case": {Text: " REVIEW ", Category: CategoryReview},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var cat Category

			err := cat.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Category, cat)
		})
	}
}

func Test_Type_Category(t *testing.T) {
	cc := map[string]struct {
		Type   Type
		Result Category
	}{
		"Document": {Type: TypeDocumentCreated, Result: CategoryDocument},
		"Content":  {Type: TypeContentEdited, Result: CategoryContent},
		"Branch":   {Type: TypeBranchMerged, Result: CategoryBranch},
		"Review":   {Type: TypeReviewApproved, Result: CategoryReview},
		"Comment":  {Type: TypeCommentReplyEdited, Result: CategoryComment},
		"Hook":     {Type: TypeHookNeedsAttention, Result: CategoryHook},
		"Tag":      {Type: TypeTagUnassigned, Result: CategoryTag},
		"Unknown":  {Type: "comment.liked"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Type.Category())
		})
	}
}

func Test_NewContentEdited(t *testing.T) {
	t.Parallel()

	actor := Actor{Kind: ActorKindUser, UserID: null.StringFrom("user1")}
	doc := document.Document{
		BranchID:       xid.New(),
		ID:             xid.New(),
		OrganizationID: "org1",
	}
	at := timeutil.Now().Truncate(time.Second)

	e := NewContentEdited(actor, doc, at)

	assert.Equal(t, actor, e.Actor)
	assert.Equal(t, Target{}, e.Target)
	assert.False(t, e.ID.IsNil())
	assert.Equal(t, "org1", e.OrganizationID)
	assert.Equal(t, doc.ID, e.DocumentID)
	assert.Equal(t, doc.BranchID, e.BranchID)
	assert.Equal(t, TypeContentEdited, e.Type)
	assert.Equal(t, CategoryContent, e.Category)
	assert.Equal(t, Metadata{}, e.Metadata)
	assert.Equal(t, at, e.CreatedAt)
	assert.Equal(t, at, e.UpdatedAt)
}

// stamped returns the wanted entry with the id and times the constructor
// made, so the rest of the entry can be compared whole.
func stamped(t *testing.T, want, got Entry) Entry {
	t.Helper()

	assert.False(t, got.ID.IsNil())
	assert.WithinDuration(t, timeutil.Now(), got.CreatedAt, time.Minute)
	assert.Equal(t, got.CreatedAt, got.UpdatedAt)

	want.ID = got.ID
	want.CreatedAt = got.CreatedAt
	want.UpdatedAt = got.UpdatedAt

	return want
}

// testDoc returns a document on a branch of its own.
func testDoc() document.Document {
	return document.Document{
		BranchID:       xid.New(),
		BranchName:     "main",
		ID:             xid.New(),
		OrganizationID: "org1",
	}
}

func Test_NewDocumentCreated(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()

	e := NewDocumentCreated(actor, doc)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeDocumentCreated,
		Category:       CategoryDocument,
		Metadata:       Metadata{},
	}, e), e)
}

func Test_NewDocumentDuplicated(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()
	sourceID := xid.New()

	e := NewDocumentDuplicated(actor, doc, sourceID)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeDocumentDuplicated,
		Category:       CategoryDocument,
		Target:         NewTarget(TargetKindDocument, sourceID.String()),
		Metadata:       Metadata{},
	}, e), e)
}

func Test_NewBranchForked(t *testing.T) {
	t.Parallel()

	source := testDoc()
	fork := source
	fork.BranchID = xid.New()

	entries := NewBranchForked(UserActor("user1"), source, fork)

	require.Len(t, entries, 2)
	assert.Equal(t, TypeBranchForked, entries[0].Type)
	assert.Equal(t, source.BranchID, entries[0].BranchID)
	assert.Equal(t, NewTarget(TargetKindBranch, fork.BranchID.String()), entries[0].Target)
	assert.Equal(t, TypeBranchForked, entries[1].Type)
	assert.Equal(t, fork.BranchID, entries[1].BranchID)
	assert.Equal(t, NewTarget(TargetKindBranch, source.BranchID.String()), entries[1].Target)
}

func Test_NewBranchMerged(t *testing.T) {
	t.Parallel()

	source := testDoc()
	target := source
	target.BranchID = xid.New()

	entries := NewBranchMerged(UserActor("user1"), source, target)

	require.Len(t, entries, 2)
	assert.Equal(t, TypeBranchMerged, entries[0].Type)
	assert.Equal(t, source.BranchID, entries[0].BranchID)
	assert.Equal(t, TypeBranchMerged, entries[1].Type)
	assert.Equal(t, target.BranchID, entries[1].BranchID)
}

func Test_NewBranchUpdated(t *testing.T) {
	before := testDoc()

	renamed := before
	renamed.BranchName = "draft"

	protected := before
	protected.Protected = true

	both := renamed
	both.Protected = true

	cc := map[string]struct {
		Before document.Document
		After  document.Document
		Types  []Type
	}{
		"No change":   {Before: before, After: before, Types: []Type{}},
		"Renamed":     {Before: before, After: renamed, Types: []Type{TypeBranchRenamed}},
		"Protected":   {Before: before, After: protected, Types: []Type{TypeBranchProtected}},
		"Unprotected": {Before: protected, After: before, Types: []Type{TypeBranchUnprotected}},
		"Renamed and protected": {
			Before: before,
			After:  both,
			Types:  []Type{TypeBranchRenamed, TypeBranchProtected},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			entries := NewBranchUpdated(UserActor("user1"), c.Before, c.After)

			types := make([]Type, 0, len(entries))
			for _, e := range entries {
				types = append(types, e.Type)

				if e.Type == TypeBranchRenamed {
					assert.Equal(t, Metadata{MetaKeyFrom: c.Before.BranchName, MetaKeyTo: c.After.BranchName}, e.Metadata)
				}
			}

			assert.Equal(t, c.Types, types)
		})
	}
}

func Test_NewReviewRequested(t *testing.T) {
	cc := map[string]struct {
		ApprovalReset bool
		Metadata      Metadata
	}{
		"New request":                   {Metadata: Metadata{}},
		"Request that cleared approval": {ApprovalReset: true, Metadata: Metadata{MetaKeyApprovalReset: true}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			actor := UserActor("user1")
			doc := testDoc()

			e := NewReviewRequested(actor, doc, "user2", c.ApprovalReset)

			assert.Equal(t, stamped(t, Entry{
				OrganizationID: "org1",
				DocumentID:     doc.ID,
				BranchID:       doc.BranchID,
				Actor:          actor,
				Type:           TypeReviewRequested,
				Category:       CategoryReview,
				Target:         NewTarget(TargetKindUser, "user2"),
				Metadata:       c.Metadata,
			}, e), e)
		})
	}
}

func Test_NewReviewWithdrawn(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()

	e := NewReviewWithdrawn(actor, doc, "user2")

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeReviewWithdrawn,
		Category:       CategoryReview,
		Target:         NewTarget(TargetKindUser, "user2"),
		Metadata:       Metadata{},
	}, e), e)
}

func Test_NewReviewEntry(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()

	e := NewReviewEntry(actor, TypeReviewApproved, doc)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeReviewApproved,
		Category:       CategoryReview,
		Metadata:       Metadata{},
	}, e), e)
}

func Test_NewCommentEntry(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	c := comment.Comment{
		ID:             xid.New(),
		DocumentID:     xid.New(),
		OrganizationID: "org1",
		BranchID:       xid.New(),
	}
	rev := comment.NewRevision(c)

	e := NewCommentEntry(actor, TypeCommentResolved, c, rev)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     c.DocumentID,
		BranchID:       c.BranchID,
		Actor:          actor,
		Type:           TypeCommentResolved,
		Category:       CategoryComment,
		Target:         NewTarget(TargetKindCommentRevision, rev.ID.String()),
		Metadata:       Metadata{MetaKeyCommentID: c.ID.String()},
	}, e), e)
}

func Test_NewReplyEntry(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	c := comment.Comment{
		ID:             xid.New(),
		DocumentID:     xid.New(),
		OrganizationID: "org1",
		BranchID:       xid.New(),
	}
	rev := comment.NewReplyRevision(comment.Reply{ID: xid.New(), CommentID: c.ID})

	e := NewReplyEntry(actor, TypeCommentReplied, c, rev)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     c.DocumentID,
		BranchID:       c.BranchID,
		Actor:          actor,
		Type:           TypeCommentReplied,
		Category:       CategoryComment,
		Target:         NewTarget(TargetKindReplyRevision, rev.ID.String()),
		Metadata: Metadata{
			MetaKeyCommentID: c.ID.String(),
			MetaKeyReplyID:   rev.ReplyID.String(),
		},
	}, e), e)
}

func Test_NewTagEntry(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()
	tg := tag.Tag{ID: xid.New(), TagName: "Production"}

	e := NewTagEntry(actor, TypeTagAssigned, doc, tg)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeTagAssigned,
		Category:       CategoryTag,
		Target:         NewTarget(TargetKindTag, tg.ID.String()),
		Metadata:       Metadata{MetaKeyTagName: "Production"},
	}, e), e)
}

func Test_NewHookEntry(t *testing.T) {
	t.Parallel()

	h := hook.Hook{
		ID:             xid.New(),
		Type:           hook.TypeURLWatcher,
		DocumentID:     null.ValueFrom(xid.New()),
		OrganizationID: null.StringFrom("org1"),
		BranchID:       null.ValueFrom(xid.New()),
	}

	e := NewHookEntry(SystemActor(), TypeHookTriggered, h)

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     h.DocumentID.V,
		BranchID:       h.BranchID.V,
		Actor:          SystemActor(),
		Type:           TypeHookTriggered,
		Category:       CategoryHook,
		Target:         NewTarget(TargetKindHook, h.ID.String()),
		Metadata:       Metadata{MetaKeyHookType: hook.TypeURLWatcher},
	}, e), e)
}

func Test_newBranchEntry(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	doc := testDoc()
	target := NewTarget(TargetKindUser, "user2")

	e := newBranchEntry(actor, doc, TypeReviewRequested, target, Metadata{"key": "value"})

	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     doc.ID,
		BranchID:       doc.BranchID,
		Actor:          actor,
		Type:           TypeReviewRequested,
		Category:       CategoryReview,
		Target:         target,
		Metadata:       Metadata{"key": "value"},
	}, e), e)
}

func Test_newBranchPair(t *testing.T) {
	t.Parallel()

	actor := UserActor("user1")
	from := testDoc()
	to := from
	to.BranchID = xid.New()

	md := Metadata{
		MetaKeyFromBranchID: from.BranchID.String(),
		MetaKeyToBranchID:   to.BranchID.String(),
	}

	entries := newBranchPair(actor, TypeBranchMerged, from, to)

	require.Len(t, entries, 2)
	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     from.ID,
		BranchID:       from.BranchID,
		Actor:          actor,
		Type:           TypeBranchMerged,
		Category:       CategoryBranch,
		Target:         NewTarget(TargetKindBranch, to.BranchID.String()),
		Metadata:       md,
	}, entries[0]), entries[0])
	assert.Equal(t, stamped(t, Entry{
		OrganizationID: "org1",
		DocumentID:     to.ID,
		BranchID:       to.BranchID,
		Actor:          actor,
		Type:           TypeBranchMerged,
		Category:       CategoryBranch,
		Target:         NewTarget(TargetKindBranch, from.BranchID.String()),
		Metadata:       md,
	}, entries[1]), entries[1])
}
