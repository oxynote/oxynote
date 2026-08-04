package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/minio/minio-go/v7"
)

var (
	// ErrInvalidContentType is returned when the content type is not supported.
	ErrInvalidContentType = errutil.New(http.StatusBadRequest, "storage.invalid_content_type", "invalid content type")

	// ErrSizeLimitExceeded is returned when the reader exceeds the size limit.
	ErrSizeLimitExceeded = errutil.New(http.StatusBadRequest, "storage.size_limit_exceeded", "file size exceeds limit")
)

// _maxObjectSize is the maximum allowed size for an object (5 MB).
const _maxObjectSize = 5 * 1024 * 1024 // 5 MB

// ObjectInfo contains metadata about a retrieved object.
type ObjectInfo struct {
	// Body is the object's data stream.
	Body io.ReadCloser

	// ETag is the entity tag of the object.
	ETag string

	// ContentType is the MIME type of the object.
	ContentType string
}

// Upload uploads a new object.
// If the object with the same ID already exists, it is overwritten.
func (c *Client) Upload(ctx context.Context, folder, id string, r io.Reader) error {
	r = newLimitedReader(r, _maxObjectSize)

	buf := make([]byte, 512)

	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading object: %w", err)
	}

	ct := http.DetectContentType(buf[:n])

	switch ct {
	case "image/jpeg", "image/png", "image/webp":
		// OK.
	default:
		return ErrInvalidContentType
	}

	key := path.Join(folder, id)

	_, err = c.client.PutObject(
		ctx,
		c.bucket,
		key,
		io.MultiReader(bytes.NewReader(buf[:n]), r), // Since we read some bytes for content detection, we need to prepend them back.
		-1,
		minio.PutObjectOptions{
			ContentType: ct,
		},
	)
	if err != nil {
		return fmt.Errorf("putting object: %w", err)
	}

	return nil
}

// Retrieve retrieves an object by its ID.
func (c *Client) Retrieve(ctx context.Context, folder, id string) (*ObjectInfo, bool, error) {
	obj, err := c.client.GetObject(
		ctx,
		c.bucket,
		path.Join(folder, id),
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, false, fmt.Errorf("getting object: %w", err)
	}

	info, err := obj.Stat()

	var merr minio.ErrorResponse

	switch {
	case err == nil:
		// OK.
	case errors.As(err, &merr):
		if merr.Code == minio.NoSuchKey {
			return nil, false, nil
		}

		fallthrough
	default:
		return nil, false, fmt.Errorf("describing object: %w", err)
	}

	return &ObjectInfo{
		Body:        obj,
		ETag:        info.ETag,
		ContentType: info.ContentType,
	}, true, nil
}

// Delete deletes an object by its ID.
func (c *Client) Delete(ctx context.Context, folder, id string) error {
	err := c.client.RemoveObject(
		ctx,
		c.bucket,
		path.Join(folder, id),
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		return fmt.Errorf("removing object: %w", err)
	}

	return nil
}

// limitedReader wraps an io.Reader and returns ErrSizeLimitExceeded when the limit is exceeded.
type limitedReader struct {
	r     io.Reader
	limit int64
	read  int64
}

// newLimitedReader creates a new LimitedReader that limits reads to the specified number of bytes.
func newLimitedReader(r io.Reader, limit int64) *limitedReader {
	return &limitedReader{r: r, limit: limit}
}

// Read reads from the underlying reader, returning ErrSizeLimitExceeded if the limit is exceeded.
func (lr *limitedReader) Read(p []byte) (n int, err error) {
	n, err = lr.r.Read(p)
	lr.read += int64(n)

	if lr.read > lr.limit {
		return n, ErrSizeLimitExceeded
	}

	return n, err
}
