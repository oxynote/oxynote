package fs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Client_Upload(t *testing.T) {
	cc := map[string]struct {
		// Prep lays out what is already in the storage root.
		Prep   func(t *testing.T, client *Client)
		Folder string
		ID     string
		Reader io.Reader
		Data   []byte
		Err    error
	}{
		"Error returned by resolve": {
			Folder: "../elsewhere",
			ID:     "object-id",
			Reader: bytes.NewReader(_testPNG),
			Err:    ErrInvalidKey,
		},
		"Error returned by storage.ReadObject": {
			Folder: "folder",
			ID:     "object-id",
			Reader: strings.NewReader("plain text data"),
			Err:    storage.ErrInvalidContentType,
		},
		"Error returned by writeObject": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				// the object's folder is taken by a file, so it cannot be
				// created as a directory.
				require.NoError(t, os.WriteFile(filepath.Join(client.root, "folder"), []byte("blocker"), _fileMode))
			},
			Folder: "folder",
			ID:     "object-id",
			Reader: bytes.NewReader(_testPNG),
			Err:    assert.AnError,
		},
		"Existing object is replaced": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testJPEG)
			},
			Folder: "folder",
			ID:     "object-id",
			Reader: bytes.NewReader(_testPNG),
			Data:   _testPNG,
		},
		"Successful upload into a nested folder": {
			Folder: "organizations/acme/documents/doc/files",
			ID:     "object-id",
			Reader: bytes.NewReader(_testPNG),
			Data:   _testPNG,
		},
		"Successful upload": {
			Folder: "folder",
			ID:     "object-id",
			Reader: bytes.NewReader(_testPNG),
			Data:   _testPNG,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t)

			if c.Prep != nil {
				c.Prep(t, client)
			}

			err := client.Upload(context.Background(), c.Folder, c.ID, c.Reader)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			full := filepath.Join(client.root, c.Folder, c.ID)

			data, err := os.ReadFile(full) //nolint:gosec // the path is built from this test's own temporary directory
			require.NoError(t, err)
			assert.Equal(t, c.Data, data)

			info, err := os.Stat(full)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(_fileMode), info.Mode().Perm())

			dir, err := os.Stat(filepath.Dir(full))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(_dirMode), dir.Mode().Perm())

			// the object is all its folder holds: the temporary file the
			// write went through was renamed, not left behind.
			entries, err := os.ReadDir(filepath.Dir(full))
			require.NoError(t, err)
			assert.Len(t, entries, 1)
		})
	}
}

func Test_Client_Retrieve(t *testing.T) {
	cc := map[string]struct {
		Prep   func(t *testing.T, client *Client)
		Folder string
		ID     string
		Found  bool
		Err    error
	}{
		"Error returned by resolve": {
			Folder: "../elsewhere",
			ID:     "object-id",
			Err:    ErrInvalidKey,
		},
		"Error returned by Open": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				// the folder is a file, so the object below it is not a
				// path that can be opened at all.
				require.NoError(t, os.WriteFile(filepath.Join(client.root, "folder"), []byte("blocker"), _fileMode))
			},
			Folder: "folder",
			ID:     "object-id",
			Err:    assert.AnError,
		},
		"Error returned by describe": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				require.NoError(t, os.MkdirAll(filepath.Join(client.root, "folder", "object-id"), _dirMode))
			},
			Folder: "folder",
			ID:     "object-id",
			Err:    assert.AnError,
		},
		"Object is not there": {
			Folder: "folder",
			ID:     "object-id",
		},
		"Successful retrieval": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testPNG)
			},
			Folder: "folder",
			ID:     "object-id",
			Found:  true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t)

			if c.Prep != nil {
				c.Prep(t, client)
			}

			info, found, err := client.Retrieve(context.Background(), c.Folder, c.ID)
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
			require.IsType(t, &objectBody{}, info.Body)

			body, err := io.ReadAll(info.Body)
			require.NoError(t, err)
			require.NoError(t, info.Body.Close())

			assert.Equal(t, _testPNG, body)
			assert.Equal(t, "image/png", info.ContentType)

			stat, err := os.Stat(filepath.Join(client.root, c.Folder, c.ID))
			require.NoError(t, err)
			assert.Equal(t, etag(stat), info.ETag)

			// closing the body reached the file it was reading from.
			assert.Error(t, info.Body.Close())
		})
	}
}

func Test_Client_Copy(t *testing.T) {
	cc := map[string]struct {
		Prep      func(t *testing.T, client *Client)
		SrcFolder string
		SrcID     string
		DstFolder string
		DstID     string
		Data      []byte
		Err       error
	}{
		"Error returned by resolve of the source": {
			SrcFolder: "../elsewhere",
			SrcID:     "object-id",
			DstFolder: "other-folder",
			DstID:     "copy-id",
			Err:       ErrInvalidKey,
		},
		"Error returned by resolve of the destination": {
			SrcFolder: "folder",
			SrcID:     "object-id",
			DstFolder: "../elsewhere",
			DstID:     "copy-id",
			Err:       ErrInvalidKey,
		},
		"Missing source object": {
			SrcFolder: "folder",
			SrcID:     "object-id",
			DstFolder: "other-folder",
			DstID:     "copy-id",
			Err:       assert.AnError,
		},
		"Error returned by writeObject": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testPNG)
				require.NoError(t, os.WriteFile(filepath.Join(client.root, "other-folder"), []byte("blocker"), _fileMode))
			},
			SrcFolder: "folder",
			SrcID:     "object-id",
			DstFolder: "other-folder",
			DstID:     "copy-id",
			Err:       assert.AnError,
		},
		"Existing destination object is replaced": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testPNG)
				prepObject(t, client, "other-folder", "copy-id", _testJPEG)
			},
			SrcFolder: "folder",
			SrcID:     "object-id",
			DstFolder: "other-folder",
			DstID:     "copy-id",
			Data:      _testPNG,
		},
		"Successful copy": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testPNG)
			},
			SrcFolder: "folder",
			SrcID:     "object-id",
			DstFolder: "other-folder",
			DstID:     "copy-id",
			Data:      _testPNG,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t)

			if c.Prep != nil {
				c.Prep(t, client)
			}

			err := client.Copy(context.Background(), c.SrcFolder, c.SrcID, c.DstFolder, c.DstID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			data, err := os.ReadFile(filepath.Join(client.root, c.DstFolder, c.DstID))
			require.NoError(t, err)
			assert.Equal(t, c.Data, data)

			// the source is copied, not moved.
			src, err := os.ReadFile(filepath.Join(client.root, c.SrcFolder, c.SrcID))
			require.NoError(t, err)
			assert.Equal(t, c.Data, src)
		})
	}
}

func Test_Client_Delete(t *testing.T) {
	cc := map[string]struct {
		Prep   func(t *testing.T, client *Client)
		Folder string
		ID     string
		Err    error
	}{
		"Error returned by resolve": {
			Folder: "../elsewhere",
			ID:     "object-id",
			Err:    ErrInvalidKey,
		},
		"Error returned by Remove": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				// a folder holding an object cannot be removed as if it
				// were one.
				prepObject(t, client, filepath.Join("folder", "object-id"), "nested-id", _testPNG)
			},
			Folder: "folder",
			ID:     "object-id",
			Err:    assert.AnError,
		},
		// a file row can outlive its object, so the sweep must be able to
		// drop the row without the delete failing first.
		"Object is already gone": {
			Folder: "folder",
			ID:     "object-id",
		},
		"Successful deletion": {
			Prep: func(t *testing.T, client *Client) {
				t.Helper()

				prepObject(t, client, "folder", "object-id", _testPNG)
			},
			Folder: "folder",
			ID:     "object-id",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t)

			if c.Prep != nil {
				c.Prep(t, client)
			}

			err := client.Delete(context.Background(), c.Folder, c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.NoFileExists(t, filepath.Join(client.root, c.Folder, c.ID))
		})
	}
}

func Test_describe(t *testing.T) {
	cc := map[string]struct {
		// Prep returns the open file describe is called with.
		Prep func(t *testing.T) *os.File
		Err  error
	}{
		"Error returned by Stat": {
			Prep: func(t *testing.T) *os.File {
				t.Helper()

				f := prepOpenObject(t, _testPNG)
				require.NoError(t, f.Close())

				return f
			},
			Err: assert.AnError,
		},
		"Error returned by storage.SniffContentType": {
			Prep: func(t *testing.T) *os.File {
				t.Helper()

				f, err := os.Open(t.TempDir())
				require.NoError(t, err)

				t.Cleanup(func() {
					require.NoError(t, f.Close())
				})

				return f
			},
			Err: assert.AnError,
		},
		"Successful description": {
			Prep: func(t *testing.T) *os.File {
				t.Helper()

				return prepOpenObject(t, _testPNG)
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := c.Prep(t)

			info, found, err := describe(f)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, info)
			assert.True(t, found)

			body, err := io.ReadAll(info.Body)
			require.NoError(t, err)
			assert.Equal(t, _testPNG, body)

			stat, err := f.Stat()
			require.NoError(t, err)
			assert.Equal(t, etag(stat), info.ETag)
			assert.Equal(t, "image/png", info.ContentType)
		})
	}
}

func Test_etag(t *testing.T) {
	cc := map[string]struct {
		// Change rewrites the object between the two entity tags, or
		// leaves it as it is when nil.
		Change func(t *testing.T, name string)
		Equal  bool
	}{
		"Unchanged object keeps its tag": {
			Equal: true,
		},
		"Rewritten object of the same size": {
			Change: func(t *testing.T, name string) {
				t.Helper()

				require.NoError(t, os.WriteFile(name, _testJPEG[:len(_testPNG)], _fileMode))
				touch(t, name, time.Second)
			},
		},
		"Rewritten object of a different size": {
			Change: func(t *testing.T, name string) {
				t.Helper()

				require.NoError(t, os.WriteFile(name, _testPNG[:64], _fileMode))
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			name := filepath.Join(t.TempDir(), "object-id")
			require.NoError(t, os.WriteFile(name, _testPNG, _fileMode))

			before, err := os.Stat(name)
			require.NoError(t, err)

			first := etag(before)
			assert.NotEmpty(t, first)

			if c.Change != nil {
				c.Change(t, name)
			}

			after, err := os.Stat(name)
			require.NoError(t, err)

			assert.Equal(t, c.Equal, first == etag(after))
		})
	}
}

func Test_writeObject(t *testing.T) {
	cc := map[string]struct {
		// Prep lays out the directory the object is written into and
		// returns the object's path.
		Prep func(t *testing.T) string
		Err  error
	}{
		"Error returned by MkdirAll": {
			Prep: func(t *testing.T) string {
				t.Helper()

				blocker := filepath.Join(t.TempDir(), "blocker")
				require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), _fileMode))

				return filepath.Join(blocker, "folder", "object-id")
			},
			Err: assert.AnError,
		},
		"Error returned by CreateTemp": {
			Prep: func(t *testing.T) string {
				t.Helper()

				if os.Geteuid() == 0 {
					t.Skip("a read-only directory is still writable for the superuser")
				}

				dir := t.TempDir()
				require.NoError(t, os.Chmod(dir, 0o500)) //nolint:gosec // a read-only directory is what the case needs

				t.Cleanup(func() {
					require.NoError(t, os.Chmod(dir, _dirMode))
				})

				return filepath.Join(dir, "object-id")
			},
			Err: assert.AnError,
		},
		"Error returned by Rename": {
			Prep: func(t *testing.T) string {
				t.Helper()

				name := filepath.Join(t.TempDir(), "object-id")

				// a directory holding something cannot be replaced by the
				// renamed temporary file.
				require.NoError(t, os.MkdirAll(filepath.Join(name, "nested"), _dirMode))

				return name
			},
			Err: assert.AnError,
		},
		"Successful write": {
			Prep: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "folder", "object-id")
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			name := c.Prep(t)

			err := writeObject(name, _testPNG)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				// a write that failed after the temporary file existed
				// took it with it.
				entries, rerr := os.ReadDir(filepath.Dir(name))
				if rerr == nil {
					for _, entry := range entries {
						assert.False(t, strings.HasPrefix(entry.Name(), ".tmp-"), entry.Name())
					}
				}

				return
			}

			data, err := os.ReadFile(name) //nolint:gosec // the path is built from this test's own temporary directory
			require.NoError(t, err)
			assert.Equal(t, _testPNG, data)

			info, err := os.Stat(name)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(_fileMode), info.Mode().Perm())
		})
	}
}

func Test_closeObject(t *testing.T) {
	cc := map[string]struct {
		Closed bool
		Err    error
	}{
		"Error returned by Close": {
			Closed: true,
			Err:    assert.AnError,
		},
		"Successful close": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := prepOpenObject(t, _testPNG)

			if c.Closed {
				require.NoError(t, f.Close())
			}

			testutil.AssertEqualError(t, c.Err, closeObject(f))
		})
	}
}

// prepOpenObject writes an object into a directory of this test's own and
// returns it open for reading.
func prepOpenObject(t *testing.T, data []byte) *os.File {
	t.Helper()

	name := filepath.Join(t.TempDir(), "object-id")
	require.NoError(t, os.WriteFile(name, data, _fileMode))

	f, err := os.Open(name) //nolint:gosec // the path is built from this test's own temporary directory
	require.NoError(t, err)

	return f
}

// touch moves an object's modification time forward, so a rewrite that
// kept the object's size is still a rewrite the entity tag can see.
func touch(t *testing.T, name string, d time.Duration) {
	t.Helper()

	info, err := os.Stat(name)
	require.NoError(t, err)

	require.NoError(t, os.Chtimes(name, time.Time{}, info.ModTime().Add(d)))
}
