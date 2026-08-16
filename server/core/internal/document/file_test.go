package document

import (
	"testing"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

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

	f := NewFile("file-1", LocationComment, documentID, "org-1")

	assert.Equal(t, "file-1", f.ID)
	assert.Equal(t, LocationComment, f.Location)
	assert.Equal(t, documentID, f.DocumentID)
	assert.Equal(t, "org-1", f.OrganizationID)
	assert.False(t, f.CreatedAt.IsZero())
}
