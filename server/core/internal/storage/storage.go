// Package storage holds what every object storage backend shares: the
// limits and content types an object must satisfy, the metadata a
// retrieval hands back, and the Store interface the backends implement.
// The backends themselves live in the s3 and fs subpackages.
package storage

import (
	"context"
	"io"
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// _maxObjectSize is the maximum allowed size for an object (5 MB).
const _maxObjectSize = 5 * 1024 * 1024 // 5 MB

// MaxUploadBytes bounds an upload request's body: the object size limit
// plus headroom for multipart boundaries and part headers.
const MaxUploadBytes = _maxObjectSize + 64*1024

// _sniffLen is the number of leading bytes http.DetectContentType
// examines when detecting an object's content type.
const _sniffLen = 512

var (
	// ErrInvalidContentType is returned when the content type is not supported.
	ErrInvalidContentType = errutil.New(http.StatusBadRequest, "storage.invalid_content_type", "invalid content type")

	// ErrSizeLimitExceeded is returned when the reader exceeds the size limit.
	ErrSizeLimitExceeded = errutil.New(http.StatusBadRequest, "storage.size_limit_exceeded", "file size exceeds limit")
)

// ObjectInfo contains metadata about a retrieved object.
type ObjectInfo struct {
	// Body is the object's data stream.
	Body io.ReadCloser

	// ETag is the entity tag of the object.
	ETag string

	// ContentType is the MIME type of the object.
	ContentType string
}

// Store is the object storage a deployment runs on. It is implemented
// twice, by the s3 and fs subpackages, and exists so that the wiring can
// hold either one; every consumer declares the narrower interface it
// actually calls.
type Store interface {
	// Upload should upload a new object, overwriting an object already
	// stored under the same folder and id.
	Upload(ctx context.Context, folder, id string, r io.Reader) error

	// Retrieve should retrieve an object by its ID, reporting whether it
	// was there at all.
	Retrieve(ctx context.Context, folder, id string) (*ObjectInfo, bool, error)

	// Copy should copy an object within the storage.
	Copy(ctx context.Context, srcFolder, srcID, dstFolder, dstID string) error

	// Delete should delete an object by its ID, treating an object that
	// is not there as already deleted.
	Delete(ctx context.Context, folder, id string) error
}
