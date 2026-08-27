package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// ReadObject reads an object off r, rejecting it unless it is one of the
// supported image types and stays within the size limit, and returns its
// bytes alongside the detected content type.
//
// The object is fully buffered rather than streamed: the read side is
// already capped at _maxObjectSize, and a backend handed the exact bytes
// can write them in one seekable, exactly-sized pass.
func ReadObject(r io.Reader) ([]byte, string, error) {
	r = newLimitedReader(r, _maxObjectSize)

	prefix, ct, err := SniffContentType(r)
	if err != nil {
		return nil, "", err
	}

	switch ct {
	case "image/jpeg", "image/png", "image/webp":
		// OK.
	default:
		return nil, "", ErrInvalidContentType
	}

	rest, err := io.ReadAll(r)
	if err != nil {
		return nil, "", fmt.Errorf("reading object: %w", err)
	}

	return slices.Concat(prefix, rest), ct, nil
}

// SniffContentType detects the content type of the object r holds,
// returning the bytes it had to consume to do so alongside it. A caller
// still streaming the object rejoins them to the rest of the stream.
func SniffContentType(r io.Reader) ([]byte, string, error) {
	buf := make([]byte, _sniffLen)

	// a single Read may legitimately return fewer bytes than asked for, which
	// would sniff the content type from a partial prefix and reject a valid
	// image; only a genuinely short object stops before _sniffLen.
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, "", fmt.Errorf("reading object: %w", err)
	}

	return buf[:n], http.DetectContentType(buf[:n]), nil
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
