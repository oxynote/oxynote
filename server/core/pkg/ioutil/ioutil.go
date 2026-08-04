// Package ioutil provides helper I/O functions.
package ioutil

import (
	"io"

	"github.com/hashicorp/go-multierror"
)

// multiCloser wraps a slice of closers.
type multiCloser struct {
	closers []io.Closer
	force   bool
}

// MultiCloser creates a closer that iterates over and closes all the
// provided closers. During closing, the iteration order will always
// be the same.
// If the force parameter is set to true, every closer will be closed,
// regardless of the error values of the previous closers.
// Otherwise, the method will exit after encountering the first error.
//
// Inspired by: https://golang.org/pkg/io/#MultiWriter
func MultiCloser(force bool, closers ...io.Closer) io.Closer {
	return &multiCloser{
		closers: closers,
		force:   force,
	}
}

// Close iterates over all the wrapped closers and calls their Close()
// methods. The iteration order will always be the same.
func (mc *multiCloser) Close() (err error) {
	for _, c := range mc.closers {
		if cerr := c.Close(); cerr != nil {
			if mc.force {
				err = multierror.Append(err, cerr)
				continue
			}

			return cerr
		}
	}

	return err
}

// CloserFunc type is an adapter to allow the
// use of error returning functions as a Closer interface.
type CloserFunc func() error

// Close invokes the underlying function.
func (cf CloserFunc) Close() error {
	return cf()
}

// AppendCloseErr appends closer's error to the provided error.
func AppendCloseErr(closer io.Closer, err error) error {
	if cerr := closer.Close(); cerr != nil {
		return multierror.Append(err, cerr)
	}

	return err
}
