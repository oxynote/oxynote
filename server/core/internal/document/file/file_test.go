package file

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Location_Valid(t *testing.T) {
	t.Parallel()

	assert.True(t, LocationDocument.Valid())
	assert.True(t, LocationComment.Valid())
	assert.False(t, Location("attachment").Valid())
	assert.False(t, Location("").Valid())
}

func Test_NewFile(t *testing.T) {
	t.Parallel()

	documentID := xid.New()

	f := NewFile("file-1", LocationComment, "organizations/org-1/documents/doc-1/files/file-1", documentID, "org-1")

	assert.Equal(t, "file-1", f.ID)
	assert.Equal(t, LocationComment, f.Location)
	assert.Equal(t, "organizations/org-1/documents/doc-1/files/file-1", f.StorageKey)
	assert.Equal(t, documentID, f.DocumentID.V)
	assert.Equal(t, "org-1", f.OrganizationID.String)
	assert.False(t, f.Orphaned())
	assert.False(t, f.CreatedAt.IsZero())
}

func Test_Folder(t *testing.T) {
	t.Parallel()

	documentID := xid.New()

	assert.Equal(
		t,
		"organizations/org-1/documents/"+documentID.String()+"/files",
		Folder("org-1", documentID),
	)
}

func Test_Key(t *testing.T) {
	t.Parallel()

	documentID := xid.New()

	assert.Equal(
		t,
		"organizations/org-1/documents/"+documentID.String()+"/files/file-1",
		Key("org-1", documentID, "file-1"),
	)
}

func Test_File_Orphaned(t *testing.T) {
	t.Parallel()

	f := NewFile("file-1", LocationDocument, "key", xid.New(), "org-1")
	assert.False(t, f.Orphaned())

	f.DocumentID = null.Value[xid.ID]{}
	assert.True(t, f.Orphaned())

	f = NewFile("file-1", LocationDocument, "key", xid.New(), "org-1")
	f.OrganizationID = null.String{}
	assert.True(t, f.Orphaned())
}
