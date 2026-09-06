package storage

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReadObject(t *testing.T) {
	cc := map[string]struct {
		Reader      io.Reader
		Policy      Policy
		Data        []byte
		ContentType string
		Err         error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Policy: ImagePolicy,
			Err:    assert.AnError,
		},
		"Invalid content type": {
			Reader: strings.NewReader("plain text data"),
			Policy: ImagePolicy,
			Err:    ErrInvalidContentType,
		},
		"Size limit exceeded": {
			Reader: io.MultiReader(
				bytes.NewReader(_testPNG),
				bytes.NewReader(bytes.Repeat([]byte{4}, int(ImagePolicy.MaxSize))),
			),
			Policy: ImagePolicy,
			Err:    assert.AnError,
		},
		"Short reads still sniff the content type": {
			Reader:      &chunkReader{r: bytes.NewReader(_testPNG)},
			Policy:      ImagePolicy,
			Data:        _testPNG,
			ContentType: "image/png",
		},
		"Object shorter than the sniff length": {
			Reader:      bytes.NewReader(_testPNG[:64]),
			Policy:      ImagePolicy,
			Data:        _testPNG[:64],
			ContentType: "image/png",
		},
		"Successful PNG read": {
			Reader:      bytes.NewReader(_testPNG),
			Policy:      ImagePolicy,
			Data:        _testPNG,
			ContentType: "image/png",
		},
		"Successful JPEG read": {
			Reader:      bytes.NewReader(_testJPEG),
			Policy:      ImagePolicy,
			Data:        _testJPEG,
			ContentType: "image/jpeg",
		},
		"Successful WebP read": {
			Reader:      bytes.NewReader(_testWebP),
			Policy:      ImagePolicy,
			Data:        _testWebP,
			ContentType: "image/webp",
		},
		"File policy accepts plain text": {
			Reader:      strings.NewReader("plain text data"),
			Policy:      FilePolicy,
			Data:        []byte("plain text data"),
			ContentType: "text/plain; charset=utf-8",
		},
		"File policy accepts an object the image policy would reject by size": {
			Reader:      bytes.NewReader(_testLargeText),
			Policy:      FilePolicy,
			Data:        _testLargeText,
			ContentType: "text/plain; charset=utf-8",
		},
		"File policy size limit exceeded": {
			Reader: io.MultiReader(
				bytes.NewReader(_testLargeText),
				bytes.NewReader(bytes.Repeat([]byte{'x'}, int(FilePolicy.MaxSize-int64(len(_testLargeText))+1))),
			),
			Policy: FilePolicy,
			Err:    assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			data, ct, err := ReadObject(c.Reader, c.Policy)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Data, data)
			assert.Equal(t, c.ContentType, ct)
		})
	}
}

func Test_SniffContentType(t *testing.T) {
	cc := map[string]struct {
		Reader      io.Reader
		Prefix      []byte
		ContentType string
		Err         error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Err:    assert.AnError,
		},
		"Object shorter than the sniff length": {
			Reader:      bytes.NewReader(_testPNG[:32]),
			Prefix:      _testPNG[:32],
			ContentType: "image/png",
		},
		"Only the sniff length is consumed": {
			Reader:      bytes.NewReader(_testPNG),
			Prefix:      _testPNG[:_sniffLen],
			ContentType: "image/png",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			prefix, ct, err := SniffContentType(c.Reader)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Prefix, prefix)
			assert.Equal(t, c.ContentType, ct)
		})
	}
}

func Test_detectContentType(t *testing.T) {
	cc := map[string]struct {
		Input  []byte
		Result string
	}{
		"PNG":                                 {Input: _testPNG, Result: "image/png"},
		"Plain text":                          {Input: []byte("id,name\n1,a\n"), Result: "text/plain; charset=utf-8"},
		"Text starting with the bitmap magic": {Input: []byte("BMW,Model,Price\n3,320i,40000\n"), Result: "text/plain; charset=utf-8"},
		"Text starting with the gif magic":    {Input: []byte("GIF89a,foo\n"), Result: "text/plain; charset=utf-8"},
		"Text with a byte order mark":         {Input: []byte("\xef\xbb\xbfBMW,x\n"), Result: "text/plain; charset=utf-8"},
		"HTML stays HTML":                     {Input: []byte("<html><body>hi</body></html>"), Result: "text/html; charset=utf-8"},
		"Bitmap with binary bytes":            {Input: append([]byte("BM"), make([]byte, 64)...), Result: "image/bmp"},
		"Binary without magic":                {Input: []byte{0x00, 0x01, 0x02, 0xff}, Result: "application/octet-stream"},
		"Empty":                               {Input: nil, Result: "text/plain; charset=utf-8"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, detectContentType(c.Input))
		})
	}
}

func Test_isText(t *testing.T) {
	cc := map[string]struct {
		Input  []byte
		Result bool
	}{
		"ASCII with newlines and tabs": {Input: []byte("a,b\tc\r\n"), Result: true},
		"UTF-8 text":                   {Input: []byte("résumé"), Result: true},
		"Invalid UTF-8":                {Input: []byte{0xff, 0xfe, 'a'}},
		"Control character":            {Input: []byte("a\x00b")},
		"Escape character":             {Input: []byte("a\x1bb")},
		"Empty":                        {Input: nil, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isText(c.Input))
		})
	}
}

func Test_newLimitedReader(t *testing.T) {
	t.Parallel()

	r := strings.NewReader("data")
	lr := newLimitedReader(r, 10)

	require.NotNil(t, lr)
	assert.Equal(t, r, lr.r)
	assert.EqualValues(t, 10, lr.limit)
	assert.Zero(t, lr.read)
}

func Test_limitedReader_Read(t *testing.T) {
	cc := map[string]struct {
		Reader io.Reader
		Limit  int64
		Result []byte
		Err    error
	}{
		"Error returned by reader": {
			Reader: &errReader{},
			Limit:  10,
			Err:    assert.AnError,
		},
		"Limit exceeded": {
			Reader: strings.NewReader("oversized data"),
			Limit:  4,
			Err:    ErrSizeLimitExceeded,
		},
		"Read within limit": {
			Reader: strings.NewReader("data"),
			Limit:  10,
			Result: []byte("data"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lr := newLimitedReader(c.Reader, c.Limit)

			buf := make([]byte, 64)

			n, err := lr.Read(buf)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, buf[:n])
		})
	}
}
