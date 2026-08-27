package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/oxynote/oxynote/server/core/internal/storage"
)

// Upload uploads a new object.
// If the object with the same ID already exists, it is overwritten.
func (c *Client) Upload(_ context.Context, folder, id string, r io.Reader) error {
	full, err := c.resolve(folder, id)
	if err != nil {
		return err
	}

	data, _, err := storage.ReadObject(r)
	if err != nil {
		return err
	}

	// the content type is not kept: an object is sniffed again on the way
	// out, and the three types ReadObject admits are exactly the ones
	// http.DetectContentType tells apart.
	return writeObject(full, data)
}

// Retrieve retrieves an object by its ID.
func (c *Client) Retrieve(_ context.Context, folder, id string) (*storage.ObjectInfo, bool, error) {
	full, err := c.resolve(folder, id)
	if err != nil {
		return nil, false, err
	}

	f, err := os.Open(full) //nolint:gosec // resolve holds the path inside the storage root

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, os.ErrNotExist):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("opening object: %w", err)
	}

	info, ok, err := describe(f)
	if err != nil {
		return nil, false, errors.Join(err, closeObject(f))
	}

	return info, ok, nil
}

// Copy copies an object within the storage root. Uploads are capped well
// below any size that would justify streaming, so the object is read into
// memory and written back out as atomically as an upload is.
func (c *Client) Copy(_ context.Context, srcFolder, srcID, dstFolder, dstID string) error {
	src, err := c.resolve(srcFolder, srcID)
	if err != nil {
		return err
	}

	dst, err := c.resolve(dstFolder, dstID)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src) //nolint:gosec // resolve holds the path inside the storage root
	if err != nil {
		return fmt.Errorf("reading source object: %w", err)
	}

	return writeObject(dst, data)
}

// Delete deletes an object by its ID. Deleting an object that is not there
// is not an error, which is what lets a crashed upload heal: the row that
// outlived it is removed on the next cleanup pass either way.
//
// The folder the object sat in is left behind. There are no folders in an
// object store to reclaim, and pruning one races an upload arriving into
// it.
func (c *Client) Delete(_ context.Context, folder, id string) error {
	full, err := c.resolve(folder, id)
	if err != nil {
		return err
	}

	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing object: %w", err)
	}

	return nil
}

// describe reads the metadata an open object is served with: an entity
// tag standing for this revision of it, and the content type sniffed off
// the bytes that then rejoin the body.
func describe(f *os.File) (*storage.ObjectInfo, bool, error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("reading object info: %w", err)
	}

	prefix, ct, err := storage.SniffContentType(f)
	if err != nil {
		return nil, false, err
	}

	return &storage.ObjectInfo{
		Body:        &objectBody{Reader: io.MultiReader(bytes.NewReader(prefix), f), Closer: f},
		ETag:        etag(stat),
		ContentType: ct,
	}, true, nil
}

// etag derives an entity tag from the object's size and modification
// time. It is served bare and compared against If-None-Match, and an
// upload's rename bumps the modification time; the one revision it cannot
// tell apart is a same-size rewrite landing within a single filesystem
// timestamp tick.
func etag(stat os.FileInfo) string {
	return strconv.FormatInt(stat.ModTime().UnixNano(), 16) +
		"-" +
		strconv.FormatInt(stat.Size(), 16)
}

// writeObject writes the data to a temporary file in the object's folder
// and renames it into place, so a reader sees either the object that was
// there before or the whole of the new one, and a failed write leaves
// neither.
func writeObject(name string, data []byte) (err error) {
	dir := filepath.Dir(name)

	if err = os.MkdirAll(dir, _dirMode); err != nil {
		return fmt.Errorf("creating object folder: %w", err)
	}

	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary object: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}

		if rerr := os.Remove(f.Name()); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			// NOCOV: a temporary file this process just created cannot be
			// made unremovable.
			err = errors.Join(err, fmt.Errorf("removing temporary object: %w", rerr))
		}
	}()

	if _, err = f.Write(data); err != nil {
		// NOCOV: a write to a freshly created file fails on a full or
		// broken disk, which a test cannot arrange.
		return errors.Join(fmt.Errorf("writing object: %w", err), closeObject(f))
	}

	// CreateTemp already opens at _fileMode, but only as far as the
	// process umask allows; chmod is what makes the mode the same
	// everywhere.
	if err = f.Chmod(_fileMode); err != nil {
		// NOCOV: the mode of an open file this process owns cannot be
		// made unsettable.
		return errors.Join(fmt.Errorf("setting object mode: %w", err), closeObject(f))
	}

	if err = closeObject(f); err != nil {
		// NOCOV: closing a file that was written to cannot be made to
		// fail.
		return err
	}

	if err = os.Rename(f.Name(), name); err != nil {
		return fmt.Errorf("renaming object: %w", err)
	}

	return nil
}

// closeObject closes an object file, naming it in the error so a close
// failure joined onto another one still says what it was.
func closeObject(f *os.File) error {
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing object: %w", err)
	}

	return nil
}

// objectBody rejoins the bytes spent sniffing the content type with the
// rest of the open file, so the object still streams from where the sniff
// left off and the caller's Close still reaches the file.
type objectBody struct {
	io.Reader
	io.Closer
}
