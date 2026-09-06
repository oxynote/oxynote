package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// ReadObject reads an object off r, rejecting it unless it satisfies the
// policy, and returns its bytes alongside the detected content type. The
// type is detected from the bytes, so a client's own claim about it never
// reaches storage.
//
// The object is fully buffered rather than streamed: the read side is
// already capped at the policy's size, and a backend handed the exact
// bytes can write them in one seekable, exactly-sized pass.
func ReadObject(r io.Reader, p Policy) ([]byte, string, error) {
	r = newLimitedReader(r, p.MaxSize)

	prefix, ct, err := SniffContentType(r)
	if err != nil {
		return nil, "", err
	}

	if len(p.ContentTypes) > 0 && !slices.Contains(p.ContentTypes, ct) {
		return nil, "", ErrInvalidContentType
	}

	data, err := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), r))
	if err != nil {
		return nil, "", fmt.Errorf("reading object: %w", err)
	}

	return data, ct, nil
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

	return buf[:n], detectContentType(buf[:n]), nil
}

// detectContentType names the type of an object by its leading bytes. A
// magic-number match is trusted only when the bytes are not plain text:
// http.DetectContentType reads a CSV whose first cell is "BMW" as a bitmap
// and one starting "GIF89a" as a gif, while a real image carries bytes no
// text file does. Text the sniffer itself names, HTML included, is left
// as it says.
func detectContentType(buf []byte) string {
	ct := http.DetectContentType(buf)

	if !strings.HasPrefix(ct, "text/") && isText(buf) {
		return "text/plain; charset=utf-8"
	}

	return ct
}

// isText reports whether the bytes are valid UTF-8 carrying no control
// characters beyond the whitespace a text file uses.
func isText(buf []byte) bool {
	if !utf8.Valid(buf) {
		return false
	}

	for _, b := range buf {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}

	return true
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
