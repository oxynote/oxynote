package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

var (
	// ErrInvalidContentType is returned when the content type is not supported.
	ErrInvalidContentType = errutil.New(http.StatusBadRequest, "storage.invalid_content_type", "invalid content type")

	// ErrSizeLimitExceeded is returned when the reader exceeds the size limit.
	ErrSizeLimitExceeded = errutil.New(http.StatusBadRequest, "storage.size_limit_exceeded", "file size exceeds limit")
)

// _maxObjectSize is the maximum allowed size for an object (5 MB).
const _maxObjectSize = 5 * 1024 * 1024 // 5 MB

// MaxUploadBytes bounds an upload request's body: the object size limit
// plus headroom for multipart boundaries and part headers.
const MaxUploadBytes = _maxObjectSize + 64*1024

// _sniffLen is the number of leading bytes http.DetectContentType
// examines when detecting an object's content type.
const _sniffLen = 512

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

	buf := make([]byte, _sniffLen)

	// a single Read may legitimately return fewer bytes than asked for, which
	// would sniff the content type from a partial prefix and reject a valid
	// image; only a genuinely short object stops before _sniffLen.
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
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

	// the object is fully buffered rather than streamed: the read side is
	// already capped at _maxObjectSize, and a seekable reader with an exact
	// size keeps the client's transient-error retries (disabled for
	// non-seekable streams) and a single-part upload.
	rest, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading object: %w", err)
	}

	data := slices.Concat(buf[:n], rest)

	_, err = c.client.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:        aws.String(c.bucket),
			Key:           aws.String(key),
			Body:          bytes.NewReader(data),
			ContentLength: aws.Int64(int64(len(data))),
			ContentType:   aws.String(ct),
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
		&s3.GetObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(folder, id)),
		},
	)

	var nsk *types.NoSuchKey

	switch {
	case err == nil:
		// OK.
	case errors.As(err, &nsk):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("getting object: %w", err)
	}

	return &ObjectInfo{
		Body: obj.Body,
		// S3 quotes the entity tag; the value is served back as a bare
		// ETag header and compared against If-None-Match as such.
		ETag:        strings.Trim(aws.ToString(obj.ETag), `"`),
		ContentType: aws.ToString(obj.ContentType),
	}, true, nil
}

// Copy copies an object within the bucket, server-side. The object is never
// streamed through this process: uploads are capped well below the multipart
// threshold, so a copy stays a metadata-only operation.
func (c *Client) Copy(ctx context.Context, srcFolder, srcID, dstFolder, dstID string) error {
	_, err := c.client.CopyObject(
		ctx,
		&s3.CopyObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(dstFolder, dstID)),
			// the source is named as a single "<bucket>/<key>" path and
			// carries the key's own escaping.
			CopySource: aws.String(url.PathEscape(path.Join(c.bucket, srcFolder, srcID))),
		},
	)
	if err != nil {
		return fmt.Errorf("copying object: %w", err)
	}

	return nil
}

// Delete deletes an object by its ID. Deleting an object that is not there
// is not an error, which is what lets a crashed upload heal: the row that
// outlived it is removed on the next cleanup pass either way.
func (c *Client) Delete(ctx context.Context, folder, id string) error {
	_, err := c.client.DeleteObject(
		ctx,
		&s3.DeleteObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(path.Join(folder, id)),
		},
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
