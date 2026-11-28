package activity

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
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
