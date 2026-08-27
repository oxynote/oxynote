// Package fs provides object storage backed by a directory on the local
// filesystem, for deployments running without an object store. Objects
// live on one node's disk: they are not shared between instances and
// they outlive the process only if the directory does.
package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// _dirMode is the mode the storage root and its object folders are
	// created with.
	_dirMode = 0o700

	// _fileMode is the mode an object is written with. The core process
	// is the only reader either mode has to admit.
	_fileMode = 0o600
)

var (
	// ErrPathNotConfigured is returned when no directory to keep objects
	// under was configured. There is no default: a deployment running
	// without an object store has to say where its objects live, rather
	// than have them land somewhere it never chose.
	ErrPathNotConfigured = errors.New("storage path is not configured")

	// ErrInvalidKey is returned when a folder or id names a path outside
	// the storage root. No validated caller can produce one, so it stands
	// for a programming error rather than a rejected request.
	ErrInvalidKey = errors.New("object key escapes the storage root")
)

// Client stores objects under a directory on the local filesystem.
type Client struct {
	root string
}

// NewClient creates a new client instance rooted at the given path. The
// directory is created when it does not exist, and a probe file proves it
// is writable now rather than at the deployment's first upload.
func NewClient(path string) (*Client, error) {
	root, err := rootPath(path)
	if err != nil {
		return nil, err
	}

	// this is also what fails when the path is taken by something that is
	// not a directory.
	if err = os.MkdirAll(root, _dirMode); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}

	probe, err := os.CreateTemp(root, ".probe-*")
	if err != nil {
		return nil, fmt.Errorf("writing to storage directory: %w", err)
	}

	if err := probe.Close(); err != nil {
		// NOCOV: closing a file that was just created cannot be made
		// to fail.
		return nil, fmt.Errorf("closing probe file: %w", err)
	}

	if err := os.Remove(probe.Name()); err != nil {
		// NOCOV: the probe file cannot be taken away between creating
		// it and removing it.
		return nil, fmt.Errorf("removing probe file: %w", err)
	}

	return &Client{root: root}, nil
}

// rootPath resolves the configured path into the absolute directory
// objects are kept under.
func rootPath(path string) (string, error) {
	if path == "" {
		return "", ErrPathNotConfigured
	}

	root, err := filepath.Abs(path)
	if err != nil {
		// NOCOV: the working directory a relative path resolves against
		// cannot be taken away mid-test.
		return "", fmt.Errorf("resolving storage path: %w", err)
	}

	return root, nil
}

// resolve joins an object key onto the storage root, rejecting a key that
// would name a path outside it. An S3 key is an opaque string; here it is
// a path, so containment is proven once here instead of being trusted
// from every caller.
func (c *Client) resolve(folder, id string) (string, error) {
	full := filepath.Join(c.root, folder, id)

	if !strings.HasPrefix(full, c.root+string(os.PathSeparator)) {
		return "", ErrInvalidKey
	}

	return full, nil
}
