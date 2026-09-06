package s3

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

func Test_Client_Upload(t *testing.T) {
	cc := map[string]struct {
		Fake        *fakeS3
		Data        []byte
		ContentType string
		Err         error
	}{
		"Error returned by PutObject": {
			Fake:        &fakeS3{failUpload: true},
			Data:        _testPNG,
			ContentType: "image/png",
			Err:         assert.AnError,
		},
		"Successful PNG upload": {
			Fake:        &fakeS3{},
			Data:        _testPNG,
			ContentType: "image/png",
		},
		"Successful upload under the content type it was given": {
			Fake:        &fakeS3{},
			Data:        []byte("PK\x03\x04 zipped bytes"),
			ContentType: "application/zip",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t, c.Fake)

			err := client.Upload(context.Background(), "folder", "object-id", c.Data, c.ContentType)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Data, c.Fake.objects["folder/object-id"])
			assert.Equal(t, c.ContentType, c.Fake.contentTypes["folder/object-id"])
		})
	}
}

func Test_Client_Retrieve(t *testing.T) {
	cc := map[string]struct {
		Fake  *fakeS3
		Found bool
		Err   error
	}{
		"Error returned by GetObject": {
			Fake: &fakeS3{failGet: true},
			Err:  assert.AnError,
		},
		"Object not found": {
			Fake: &fakeS3{},
		},
		"Successful retrieval": {
			Fake: &fakeS3{
				objects:      map[string][]byte{"folder/object-id": _testPNG},
				contentTypes: map[string]string{"folder/object-id": "image/png"},
			},
			Found: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t, c.Fake)

			info, found, err := client.Retrieve(context.Background(), "folder", "object-id")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Found, found)

			if !c.Found {
				assert.Nil(t, info)

				return
			}

			require.NotNil(t, info)

			body, err := io.ReadAll(info.Body)
			require.NoError(t, err)
			require.NoError(t, info.Body.Close())

			assert.Equal(t, _testPNG, body)
			assert.Equal(t, "test-etag", info.ETag)
			assert.Equal(t, "image/png", info.ContentType)
		})
	}
}

func Test_Client_Copy(t *testing.T) {
	cc := map[string]struct {
		Fake *fakeS3
		Err  error
	}{
		"Error returned by CopyObject": {
			Fake: &fakeS3{
				objects:  map[string][]byte{"folder/object-id": _testPNG},
				failCopy: true,
			},
			Err: assert.AnError,
		},
		"Missing source object": {
			Fake: &fakeS3{objects: map[string][]byte{}},
			Err:  assert.AnError,
		},
		"Successful copy": {
			Fake: &fakeS3{
				objects: map[string][]byte{"folder/object-id": _testPNG},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t, c.Fake)

			err := client.Copy(context.Background(), "folder", "object-id", "other-folder", "copy-id")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, _testPNG, c.Fake.objects["other-folder/copy-id"])
			assert.Equal(t, _testPNG, c.Fake.objects["folder/object-id"])
		})
	}
}

func Test_Client_Delete(t *testing.T) {
	cc := map[string]struct {
		Fake *fakeS3
		Err  error
	}{
		"Error returned by RemoveObject": {
			Fake: &fakeS3{
				objects:    map[string][]byte{"folder/object-id": _testPNG},
				failDelete: true,
			},
			Err: assert.AnError,
		},
		// a file row can outlive its object, so the sweep must be able to
		// drop the row without the delete failing first.
		"Object is already gone": {
			Fake: &fakeS3{objects: map[string][]byte{}},
		},
		"Successful deletion": {
			Fake: &fakeS3{
				objects: map[string][]byte{"folder/object-id": _testPNG},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t, c.Fake)

			err := client.Delete(context.Background(), "folder", "object-id")
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.NotContains(t, c.Fake.objects, "folder/object-id")
		})
	}
}
