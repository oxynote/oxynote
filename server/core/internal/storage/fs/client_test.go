package fs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// _testPNG is a data prefix carrying the PNG magic bytes so content
// type sniffing detects image/png.
var _testPNG = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 1024)...)

// _testJPEG is a data prefix carrying the JPEG magic bytes so content
// type sniffing detects image/jpeg.
var _testJPEG = append([]byte("\xff\xd8\xff"), bytes.Repeat([]byte{2}, 1024)...)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewClient(t *testing.T) {
	cc := map[string]struct {
		// Prep lays out the filesystem the client is created over and
		// returns the path handed to the constructor.
		Prep func(t *testing.T) string
		Err  error
	}{
		"Error returned by rootPath": {
			Prep: func(t *testing.T) string {
				t.Helper()

				return ""
			},
			Err: ErrPathNotConfigured,
		},
		"Error returned by MkdirAll": {
			Prep: func(t *testing.T) string {
				t.Helper()

				blocker := filepath.Join(t.TempDir(), "blocker")
				require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), _fileMode))

				return filepath.Join(blocker, "storage")
			},
			Err: assert.AnError,
		},
		"Error returned by CreateTemp": {
			Prep: func(t *testing.T) string {
				t.Helper()

				if os.Geteuid() == 0 {
					t.Skip("a read-only directory is still writable for the superuser")
				}

				root := t.TempDir()
				require.NoError(t, os.Chmod(root, 0o500)) //nolint:gosec // a read-only directory is what the case needs

				t.Cleanup(func() {
					require.NoError(t, os.Chmod(root, _dirMode))
				})

				return root
			},
			Err: assert.AnError,
		},
		"Storage directory is created": {
			Prep: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "storage", "objects")
			},
		},
		"Existing storage directory is used": {
			Prep: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			path := c.Prep(t)

			client, err := NewClient(path)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, client)
			assert.Equal(t, path, client.root)

			info, err := os.Stat(client.root)
			require.NoError(t, err)
			assert.True(t, info.IsDir())

			// the probe the constructor wrote proves the directory is
			// writable; it must not survive it.
			entries, err := os.ReadDir(client.root)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func Test_rootPath(t *testing.T) {
	cc := map[string]struct {
		Path   string
		Result string
		Err    error
	}{
		"Empty path is refused": {
			Err: ErrPathNotConfigured,
		},
		"Absolute path is kept": {
			Path:   "/srv/objects",
			Result: "/srv/objects",
		},
		"Relative path is made absolute": {
			Path:   "objects",
			Result: filepath.Join(mustGetwd(t), "objects"),
		},
		"Path is cleaned": {
			Path:   "/srv/../srv/objects/",
			Result: "/srv/objects",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			root, err := rootPath(c.Path)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, root)
		})
	}
}

func Test_Client_resolve(t *testing.T) {
	cc := map[string]struct {
		Folder string
		ID     string
		Result string
		Err    error
	}{
		"Folder climbing out of the root": {
			Folder: "../elsewhere",
			ID:     "object-id",
			Err:    ErrInvalidKey,
		},
		"ID climbing out of the root": {
			Folder: "folder",
			ID:     "../../object-id",
			Err:    ErrInvalidKey,
		},
		"Absolute id": {
			Folder: "folder",
			ID:     "/etc/passwd",
			Result: "folder/etc/passwd",
		},
		"Key naming the root itself": {
			Folder: ".",
			ID:     ".",
			Err:    ErrInvalidKey,
		},
		"Nested folder": {
			Folder: "organizations/acme/documents/doc/files",
			ID:     "object-id",
			Result: "organizations/acme/documents/doc/files/object-id",
		},
		"Plain key": {
			Folder: "folder",
			ID:     "object-id",
			Result: "folder/object-id",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client := prepClient(t)

			full, err := client.resolve(c.Folder, c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, filepath.Join(client.root, c.Result), full)
		})
	}
}

// prepClient creates a client rooted at a directory of this test's own.
func prepClient(t *testing.T) *Client {
	t.Helper()

	client, err := NewClient(t.TempDir())
	require.NoError(t, err)

	return client
}

// prepObject writes an object straight into the client's root, bypassing
// Upload so that a test can set up what it is about to read.
func prepObject(t *testing.T, client *Client, folder, id string, data []byte) {
	t.Helper()

	full := filepath.Join(client.root, folder, id)

	require.NoError(t, os.MkdirAll(filepath.Dir(full), _dirMode))
	require.NoError(t, os.WriteFile(full, data, _fileMode))
}

// mustGetwd returns the working directory relative paths resolve against.
func mustGetwd(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	return wd
}
