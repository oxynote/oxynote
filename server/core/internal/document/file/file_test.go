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

	f := NewFile(
		"file-1",
		LocationComment,
		"organizations/org-1/documents/doc-1/files/file-1",
		documentID,
		"org-1",
		"notes.pdf",
		4096,
		"application/pdf",
	)

	assert.Equal(t, "file-1", f.ID)
	assert.Equal(t, LocationComment, f.Location)
	assert.Equal(t, "organizations/org-1/documents/doc-1/files/file-1", f.StorageKey)
	assert.Equal(t, documentID, f.DocumentID.V)
	assert.Equal(t, "org-1", f.OrganizationID.String)
	assert.Equal(t, "notes.pdf", f.Name)
	assert.EqualValues(t, 4096, f.Size)
	assert.Equal(t, "application/pdf", f.ContentType)
	assert.False(t, f.Orphaned())
	assert.False(t, f.CreatedAt.IsZero())
}

func Test_Viewable(t *testing.T) {
	cc := map[string]struct {
		ContentType string
		Result      bool
	}{
		"PDF":                       {ContentType: "application/pdf", Result: true},
		"PNG":                       {ContentType: "image/png", Result: true},
		"JPEG":                      {ContentType: "image/jpeg", Result: true},
		"GIF":                       {ContentType: "image/gif", Result: true},
		"WebP":                      {ContentType: "image/webp", Result: true},
		"Video":                     {ContentType: "video/mp4", Result: true},
		"Audio":                     {ContentType: "audio/mpeg", Result: true},
		"Plain text with a charset": {ContentType: "text/plain; charset=utf-8", Result: true},
		"HTML":                      {ContentType: "text/html; charset=utf-8"},
		"SVG":                       {ContentType: "image/svg+xml"},
		"XML":                       {ContentType: "text/xml; charset=utf-8"},
		"Script":                    {ContentType: "text/javascript"},
		"Zip":                       {ContentType: "application/zip"},
		"Octet stream":              {ContentType: "application/octet-stream"},
		"Malformed":                 {ContentType: "not a media type"},
		"Empty":                     {ContentType: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, Viewable(c.ContentType))
		})
	}
}

func Test_File_Disposition(t *testing.T) {
	cc := map[string]struct {
		Name        string
		ContentType string
		Result      string
	}{
		"Viewable type is served inline": {
			Name:        "report.pdf",
			ContentType: "application/pdf",
			Result:      `inline; filename=report.pdf`,
		},
		"Other type is served as an attachment": {
			Name:        "notes.zip",
			ContentType: "application/zip",
			Result:      `attachment; filename=notes.zip`,
		},
		"HTML is served as an attachment": {
			Name:        "evil.html",
			ContentType: "text/html; charset=utf-8",
			Result:      `attachment; filename=evil.html`,
		},
		"Quotes and separators are quoted": {
			Name:        `a "quoted"; name.txt`,
			ContentType: "text/plain",
			Result:      `inline; filename="a \"quoted\"; name.txt"`,
		},
		"Non-ASCII name is encoded": {
			Name:        "résumé.pdf",
			ContentType: "application/pdf",
			Result:      `inline; filename*=utf-8''r%C3%A9sum%C3%A9.pdf`,
		},
		"Control characters are encoded": {
			Name:        "a\r\nX-Injected: yes.txt",
			ContentType: "text/plain",
			Result:      `inline; filename*=utf-8''a%0D%0AX-Injected%3A%20yes.txt`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := File{Name: c.Name, ContentType: c.ContentType}

			assert.Equal(t, c.Result, f.Disposition())
		})
	}
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
	documentID := xid.New()

	assert.Equal(
		t,
		"organizations/org-1/documents/"+documentID.String()+"/files/file-1",
		Key("org-1", documentID, "file-1"),
	)
}

func Test_File_Orphaned(t *testing.T) {
	t.Parallel()

	f := NewFile("file-1", LocationDocument, "key", xid.New(), "org-1", "shot.png", 1024, "image/png")
	assert.False(t, f.Orphaned())

	f.DocumentID = null.Value[xid.ID]{}
	assert.True(t, f.Orphaned())

	f = NewFile("file-1", LocationDocument, "key", xid.New(), "org-1", "shot.png", 1024, "image/png")
	f.OrganizationID = null.String{}
	assert.True(t, f.Orphaned())
}
